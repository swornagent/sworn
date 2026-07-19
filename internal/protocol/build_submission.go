package protocol

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/repo"
)

type ArtifactReader interface {
	Artifact(context.Context, string) (mediaType string, contents []byte, err error)
}

type AcceptanceRequirement struct {
	ID       string
	Boundary string
}

type EvidenceRequirement struct {
	ID            string
	AcceptanceIDs []string
	Boundary      string
	UsesMocks     bool
	Observed      string
}

type BaselineCheck struct {
	ID         string
	Definition Artifact
	Evidence   EvidenceRequirement
}

// AdmittedWork contains facts already admitted from an exact plan, contract,
// policy, and current authority source. The walking skeleton deliberately
// supports only dependency-free Standard work and local test producers.
type AdmittedWork struct {
	DeliveryID            string
	WorkID                string
	PlanDigest            string
	ContractDigest        string
	Repository            string
	TargetRef             string
	Scope                 repo.Scope
	PolicyRef             string
	PolicyDigest          string
	AuthorityDigest       string
	AuthoritySourceRef    string
	AuthoritySourceDigest string
	Acceptance            []AcceptanceRequirement
	BaselineChecks        []BaselineCheck
}

type SubmissionInput struct {
	Attempt          int64
	CreatedAt        time.Time
	Work             AdmittedWork
	AuthorityReceipt Artifact
	Builder          BuilderRun
	Candidate        repo.Candidate
	Checks           []Check
	Evidence         []Evidence
}

// PreparedSubmission is an opaque construction capability minted only after
// BuildSubmission has re-bound immutable Git and artifact bytes to
// structurally pre-admitted work, authority, and producer facts. It is not an
// authenticated authority or journal-registration proof; those later engine
// boundaries must precede reviewable/PASS state.
type PreparedSubmission struct {
	submission   Submission
	record       EncodedRecord
	dependencies []Artifact
}

func (prepared PreparedSubmission) Submission() Submission {
	return cloneSubmission(prepared.submission)
}

func (prepared PreparedSubmission) Record() EncodedRecord {
	record := prepared.record
	record.CanonicalJSON = append([]byte(nil), record.CanonicalJSON...)
	return record
}

func (prepared PreparedSubmission) Dependencies() []Artifact {
	return slices.Clone(prepared.dependencies)
}

// BuildSubmission rechecks Git and every raw artifact, then constructs a
// Standard submission from structurally pre-admitted work and producer facts.
// It performs no authentication, journal lookup, storage, clock read, state
// transition, or external effect.
func BuildSubmission(
	ctx context.Context,
	repository *repo.Repository,
	artifacts ArtifactReader,
	input SubmissionInput,
) (PreparedSubmission, error) {
	if repository == nil || artifacts == nil {
		return PreparedSubmission{}, errors.New("submission requires repository and artifact readers")
	}
	if err := validateAdmittedWork(input.Work); err != nil {
		return PreparedSubmission{}, err
	}
	if input.Candidate.RepositoryID != input.Work.Repository || input.Candidate.TargetRef != input.Work.TargetRef {
		return PreparedSubmission{}, errors.New("candidate does not match admitted repository and target")
	}
	if err := repository.VerifyCandidate(ctx, input.Candidate, input.Work.Scope); err != nil {
		return PreparedSubmission{}, fmt.Errorf("verify submission candidate: %w", err)
	}
	resolver := artifactResolver{reader: artifacts, cached: make(map[string]resolvedArtifact)}
	authorityBytes, err := resolver.resolve(ctx, input.AuthorityReceipt)
	if err != nil {
		return PreparedSubmission{}, fmt.Errorf("resolve authority receipt: %w", err)
	}
	if input.AuthorityReceipt.MediaType != "application/json" {
		return PreparedSubmission{}, errors.New("authority receipt must be application/json")
	}
	if err := validateAuthorityApproval(authorityBytes, input.Work, input.Builder); err != nil {
		return PreparedSubmission{}, err
	}

	checksByID := make(map[string]Check, len(input.Checks))
	for _, check := range input.Checks {
		if _, exists := checksByID[check.ID]; exists {
			return PreparedSubmission{}, fmt.Errorf("duplicate measured check %q", check.ID)
		}
		checksByID[check.ID] = check
	}
	if len(checksByID) != len(input.Work.BaselineChecks) {
		return PreparedSubmission{}, errors.New("measured checks do not exactly cover the admitted baseline")
	}
	evidenceByID := make(map[string]Evidence, len(input.Evidence))
	for _, evidence := range input.Evidence {
		if _, exists := evidenceByID[evidence.ID]; exists {
			return PreparedSubmission{}, fmt.Errorf("duplicate measured evidence %q", evidence.ID)
		}
		evidenceByID[evidence.ID] = evidence
	}
	if len(evidenceByID) != len(input.Work.BaselineChecks) {
		return PreparedSubmission{}, errors.New("measured evidence does not exactly cover the admitted baseline")
	}

	orderedChecks := make([]Check, 0, len(input.Work.BaselineChecks))
	orderedEvidence := make([]Evidence, 0, len(input.Work.BaselineChecks))
	for _, baseline := range input.Work.BaselineChecks {
		definitionBytes, err := resolver.resolve(ctx, baseline.Definition)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q definition: %w", baseline.ID, err)
		}
		definition, err := ParseLocalCheckDefinition(definitionBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q definition: %w", baseline.ID, err)
		}
		if err := bindBaselineDefinition(definition, baseline); err != nil {
			return PreparedSubmission{}, err
		}
		check, exists := checksByID[baseline.ID]
		if !exists || check.Outcome != "pass" {
			return PreparedSubmission{}, fmt.Errorf("required baseline check %q is absent or did not pass", baseline.ID)
		}
		if check.Receipt.Ref != check.Receipt.Digest || check.Receipt.MediaType != "application/vnd.sworn.local-check-receipt+json" {
			return PreparedSubmission{}, fmt.Errorf("check %q does not use a local CAS receipt", baseline.ID)
		}
		receiptBytes, err := resolver.resolve(ctx, check.Receipt)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q receipt: %w", baseline.ID, err)
		}
		receipt, err := ParseLocalCheckReceipt(receiptBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q receipt: %w", baseline.ID, err)
		}
		if err := bindCheckReceipt(check, receipt, baseline, definition, input.Candidate); err != nil {
			return PreparedSubmission{}, err
		}
		for name, capture := range map[string]CapturedArtifact{"stdout": receipt.Stdout, "stderr": receipt.Stderr} {
			contents, err := resolver.resolve(ctx, capture.Pointer())
			if err != nil {
				return PreparedSubmission{}, fmt.Errorf("resolve check %q %s: %w", baseline.ID, name, err)
			}
			if int64(len(contents)) != capture.Size {
				return PreparedSubmission{}, fmt.Errorf("check %q %s size does not match its receipt", baseline.ID, name)
			}
		}
		if !digestPattern.MatchString(check.Environment.Ref) {
			return PreparedSubmission{}, fmt.Errorf("check %q has a non-content-addressed environment", baseline.ID)
		}
		environmentBytes, err := resolver.resolve(ctx, Artifact{
			Ref: check.Environment.Ref, MediaType: "application/vnd.sworn.local-environment+json", Digest: check.Environment.Ref,
		})
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q environment: %w", baseline.ID, err)
		}
		environment, err := ParseLocalEnvironment(environmentBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q environment: %w", baseline.ID, err)
		}
		snapshotDigest, err := SnapshotDigest()
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("measure embedded protocol snapshot: %w", err)
		}
		if environment.ProtocolSnapshotDigest != "sha256:"+snapshotDigest {
			return PreparedSubmission{}, fmt.Errorf("check %q environment names a different protocol snapshot", baseline.ID)
		}
		evidence, exists := evidenceByID[baseline.Evidence.ID]
		if !exists {
			return PreparedSubmission{}, fmt.Errorf("check %q lacks admitted evidence %q", baseline.ID, baseline.Evidence.ID)
		}
		if err := bindEvidence(evidence, check, baseline, input.Candidate.Tree); err != nil {
			return PreparedSubmission{}, err
		}
		if _, err := resolver.resolve(ctx, evidence.Artifact); err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve evidence %q: %w", evidence.ID, err)
		}
		orderedChecks = append(orderedChecks, check)
		orderedEvidence = append(orderedEvidence, evidence)
	}
	slices.SortFunc(orderedEvidence, func(left, right Evidence) int { return bytes.Compare([]byte(left.ID), []byte(right.ID)) })
	if err := validateAcceptanceCoverage(input.Work.Acceptance, orderedEvidence); err != nil {
		return PreparedSubmission{}, err
	}
	createdAt := input.CreatedAt.UTC()
	if input.CreatedAt.IsZero() || input.CreatedAt.Location() != time.UTC {
		return PreparedSubmission{}, errors.New("submission creation time must be an explicit UTC time")
	}
	submissionID, err := deriveSubmissionID(input.Work.DeliveryID, input.Work.WorkID, input.Attempt)
	if err != nil {
		return PreparedSubmission{}, err
	}
	submission := Submission{
		SchemaVersion:    SubmissionSchemaVersion,
		SubmissionID:     submissionID,
		DeliveryID:       input.Work.DeliveryID,
		WorkID:           input.Work.WorkID,
		Attempt:          input.Attempt,
		CreatedAt:        formatRecordTime(createdAt),
		PlanDigest:       input.Work.PlanDigest,
		ContractDigest:   input.Work.ContractDigest,
		AuthorityReceipt: input.AuthorityReceipt,
		Builder:          input.Builder,
		Base: GitPoint{
			Repository: input.Candidate.RepositoryID,
			Ref:        input.Candidate.TargetRef,
			Commit:     input.Candidate.BaseCommit,
		},
		Candidate: CandidatePoint{
			Repository: input.Candidate.RepositoryID,
			Commit:     input.Candidate.Commit,
			Tree:       input.Candidate.Tree,
		},
		Assurance: Assurance{
			Profile:      "standard",
			Packs:        []string{},
			PolicyRef:    input.Work.PolicyRef,
			PolicyDigest: input.Work.PolicyDigest,
		},
		ChangedPaths: append([]string{}, input.Candidate.ChangedPaths...),
		Checks:       orderedChecks,
		Evidence:     orderedEvidence,
	}
	record, err := EncodeSubmission(submission)
	if err != nil {
		return PreparedSubmission{}, err
	}
	return PreparedSubmission{
		submission:   cloneSubmission(submission),
		record:       record,
		dependencies: resolver.dependencies(),
	}, nil
}

func cloneSubmission(submission Submission) Submission {
	submission.Assurance.Packs = slices.Clone(submission.Assurance.Packs)
	submission.ChangedPaths = slices.Clone(submission.ChangedPaths)
	submission.Checks = slices.Clone(submission.Checks)
	submission.Evidence = slices.Clone(submission.Evidence)
	for index := range submission.Evidence {
		submission.Evidence[index].AcceptanceIDs = slices.Clone(submission.Evidence[index].AcceptanceIDs)
		submission.Evidence[index].PackIDs = slices.Clone(submission.Evidence[index].PackIDs)
	}
	return submission
}

func validateAdmittedWork(work AdmittedWork) error {
	if !protocolIDPattern.MatchString(work.DeliveryID) || !protocolIDPattern.MatchString(work.WorkID) {
		return errors.New("admitted work requires valid delivery and work ids")
	}
	for name, digest := range map[string]string{
		"plan": work.PlanDigest, "contract": work.ContractDigest, "policy": work.PolicyDigest,
		"authority": work.AuthorityDigest, "authority source": work.AuthoritySourceDigest,
	} {
		if !digestPattern.MatchString(digest) {
			return fmt.Errorf("admitted work has invalid %s digest", name)
		}
	}
	if !nonEmpty(work.Repository) || !validBranchRef(work.TargetRef) || !nonEmpty(work.PolicyRef) || !nonEmpty(work.AuthoritySourceRef) {
		return errors.New("admitted work has invalid repository, target, policy, or authority source")
	}
	if err := work.Scope.Validate(); err != nil {
		return err
	}
	if len(work.Acceptance) == 0 || len(work.BaselineChecks) == 0 {
		return errors.New("admitted work requires acceptance and baseline checks")
	}
	acceptance := make(map[string]string, len(work.Acceptance))
	for _, requirement := range work.Acceptance {
		if !protocolIDPattern.MatchString(requirement.ID) || (requirement.Boundary != "component" && requirement.Boundary != "assembled") {
			return fmt.Errorf("invalid initial acceptance requirement %q", requirement.ID)
		}
		if _, exists := acceptance[requirement.ID]; exists {
			return fmt.Errorf("duplicate acceptance requirement %q", requirement.ID)
		}
		acceptance[requirement.ID] = requirement.Boundary
	}
	checks := make(map[string]struct{}, len(work.BaselineChecks))
	evidence := make(map[string]struct{}, len(work.BaselineChecks))
	for _, check := range work.BaselineChecks {
		if !protocolIDPattern.MatchString(check.ID) || check.Definition.MediaType != "application/json" {
			return fmt.Errorf("invalid admitted baseline check %q", check.ID)
		}
		if err := validateArtifact(check.Definition, "baseline check definition"); err != nil {
			return err
		}
		if _, exists := checks[check.ID]; exists {
			return fmt.Errorf("duplicate baseline check %q", check.ID)
		}
		checks[check.ID] = struct{}{}
		requirement := check.Evidence
		if !protocolIDPattern.MatchString(requirement.ID) || !nonEmpty(requirement.Observed) ||
			(requirement.Boundary != "component" && requirement.Boundary != "assembled") ||
			(requirement.UsesMocks && requirement.Boundary != "component") || len(requirement.AcceptanceIDs) == 0 {
			return fmt.Errorf("baseline check %q has invalid evidence semantics", check.ID)
		}
		if _, exists := evidence[requirement.ID]; exists {
			return fmt.Errorf("duplicate admitted evidence id %q", requirement.ID)
		}
		evidence[requirement.ID] = struct{}{}
		if duplicateStrings(requirement.AcceptanceIDs) || !slices.IsSorted(requirement.AcceptanceIDs) {
			return fmt.Errorf("baseline check %q acceptance ids must be unique and sorted", check.ID)
		}
		for _, acceptanceID := range requirement.AcceptanceIDs {
			if _, exists := acceptance[acceptanceID]; !exists {
				return fmt.Errorf("baseline check %q names unknown acceptance %q", check.ID, acceptanceID)
			}
		}
	}
	return nil
}

type resolvedArtifact struct {
	mediaType string
	contents  []byte
}

type artifactResolver struct {
	reader ArtifactReader
	cached map[string]resolvedArtifact
}

func (resolver *artifactResolver) dependencies() []Artifact {
	dependencies := make([]Artifact, 0, len(resolver.cached))
	for digest, resolved := range resolver.cached {
		dependencies = append(dependencies, Artifact{
			Ref: digest, MediaType: resolved.mediaType, Digest: digest,
		})
	}
	slices.SortFunc(dependencies, func(left, right Artifact) int {
		return cmp.Compare(left.Digest, right.Digest)
	})
	return dependencies
}

func (resolver *artifactResolver) resolve(ctx context.Context, pointer Artifact) ([]byte, error) {
	if err := validateArtifact(pointer, "artifact"); err != nil {
		return nil, err
	}
	if pointer.Ref != pointer.Digest {
		return nil, errors.New("Sworn artifact pointer is not a CAS reference")
	}
	if cached, exists := resolver.cached[pointer.Digest]; exists {
		if cached.mediaType != pointer.MediaType {
			return nil, errors.New("artifact media type conflicts with an earlier pointer")
		}
		return append([]byte(nil), cached.contents...), nil
	}
	mediaType, contents, err := resolver.reader.Artifact(ctx, pointer.Digest)
	if err != nil {
		return nil, err
	}
	if mediaType != pointer.MediaType || RawDigest(contents) != pointer.Digest {
		return nil, errors.New("artifact bytes do not match their media type and digest")
	}
	if err := ValidateArtifactContent(mediaType, contents); err != nil {
		return nil, err
	}
	resolver.cached[pointer.Digest] = resolvedArtifact{mediaType: mediaType, contents: append([]byte(nil), contents...)}
	return contents, nil
}

type authorityApproval struct {
	SchemaVersion   string            `json:"schema_version"`
	Kind            string            `json:"kind"`
	ReceiptID       string            `json:"receipt_id"`
	PlanDigest      string            `json:"plan_digest"`
	AuthorityDigest string            `json:"authority_digest"`
	SourceRef       string            `json:"source_ref"`
	SourceDigest    string            `json:"source_digest"`
	Grants          []json.RawMessage `json:"grants"`
	Repository      string            `json:"repository"`
	TargetRef       string            `json:"target_ref"`
	AuthorizerRef   string            `json:"authorizer_ref"`
	ApprovedAt      string            `json:"approved_at"`
}

func parseAuthorityApproval(contents []byte) (authorityApproval, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var receipt authorityApproval
	if err := decoder.Decode(&receipt); err != nil {
		return authorityApproval{}, fmt.Errorf("decode authority approval: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return authorityApproval{}, errors.New("authority approval has trailing input")
	}
	return receipt, nil
}

// AuthorityApprovalReceiptID extracts the write-once control identity from an
// exact approval artifact. Persistence uses it to prevent the same receipt ID
// from being rebound to different bytes.
func AuthorityApprovalReceiptID(contents []byte) (string, error) {
	receipt, err := parseAuthorityApproval(contents)
	if err != nil {
		return "", err
	}
	if receipt.SchemaVersion != "control-receipt-v1" || receipt.Kind != "authority_approval" ||
		!protocolIDPattern.MatchString(receipt.ReceiptID) {
		return "", errors.New("artifact is not an identified authority approval")
	}
	return receipt.ReceiptID, nil
}

func validateAuthorityApproval(contents []byte, work AdmittedWork, builder BuilderRun) error {
	receipt, err := parseAuthorityApproval(contents)
	if err != nil {
		return err
	}
	if receipt.SchemaVersion != "control-receipt-v1" || receipt.Kind != "authority_approval" ||
		!protocolIDPattern.MatchString(receipt.ReceiptID) || !nonEmpty(receipt.AuthorizerRef) ||
		receipt.PlanDigest != work.PlanDigest || receipt.AuthorityDigest != work.AuthorityDigest ||
		receipt.SourceRef != work.AuthoritySourceRef || receipt.SourceDigest != work.AuthoritySourceDigest ||
		receipt.Repository != work.Repository || receipt.TargetRef != work.TargetRef {
		return errors.New("authority approval does not match admitted work")
	}
	approvedAt, err := parseRecordTime(receipt.ApprovedAt, "authority approval")
	if err != nil {
		return err
	}
	builderStart, err := parseRecordTime(builder.StartedAt, "builder start")
	if err != nil || approvedAt.After(builderStart) {
		return errors.New("authority approval does not precede the builder")
	}
	required := map[string]bool{"inspect": false, "edit": false, "execute": false, "commit": false}
	seenGrants := make(map[string]struct{}, len(receipt.Grants))
	for _, raw := range receipt.Grants {
		canonicalGrant, err := CanonicalizeJSON(raw)
		if err != nil {
			return errors.New("authority approval contains an invalid grant")
		}
		if _, exists := seenGrants[string(canonicalGrant)]; exists {
			return errors.New("authority approval contains a duplicate grant")
		}
		seenGrants[string(canonicalGrant)] = struct{}{}
		var grant struct {
			Action string          `json:"action"`
			Target json.RawMessage `json:"target"`
		}
		grantDecoder := json.NewDecoder(bytes.NewReader(raw))
		grantDecoder.DisallowUnknownFields()
		if err := grantDecoder.Decode(&grant); err != nil {
			return errors.New("authority approval contains an invalid grant")
		}
		if err := grantDecoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return errors.New("authority approval grant has trailing input")
		}
		if _, exists := required[grant.Action]; exists {
			if !bytes.Equal(bytes.TrimSpace(grant.Target), []byte(`"workspace"`)) {
				return fmt.Errorf("authority %s grant has an invalid target", grant.Action)
			}
			required[grant.Action] = true
			continue
		}
		if grant.Action != "integrate" {
			return fmt.Errorf("authority approval has unknown grant action %q", grant.Action)
		}
		var target struct {
			Repository string `json:"repository"`
			Ref        string `json:"ref"`
		}
		targetDecoder := json.NewDecoder(bytes.NewReader(grant.Target))
		targetDecoder.DisallowUnknownFields()
		if err := targetDecoder.Decode(&target); err != nil || target.Repository != work.Repository || target.Ref != work.TargetRef {
			return errors.New("authority integration grant does not match the admitted target")
		}
		if err := targetDecoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return errors.New("authority integration target has trailing input")
		}
	}
	for action, present := range required {
		if !present {
			return fmt.Errorf("authority approval lacks %s workspace grant", action)
		}
	}
	return nil
}

func bindBaselineDefinition(definition LocalCheckDefinition, baseline BaselineCheck) error {
	requirement := baseline.Evidence
	evidence := definition.Evidence
	if evidence.ID != requirement.ID || !slices.Equal(evidence.AcceptanceIDs, requirement.AcceptanceIDs) ||
		evidence.Boundary != requirement.Boundary || evidence.UsesMocks != requirement.UsesMocks ||
		evidence.Observed != requirement.Observed {
		return fmt.Errorf("check %q definition does not match admitted policy semantics", baseline.ID)
	}
	return nil
}

func bindCheckReceipt(
	check Check,
	receipt LocalCheckReceipt,
	baseline BaselineCheck,
	definition LocalCheckDefinition,
	candidate repo.Candidate,
) error {
	if receipt.Outcome != "pass" || receipt.CheckID != check.ID || receipt.RunID != check.RunID ||
		receipt.Definition != baseline.Definition || receipt.Candidate.Repository != candidate.RepositoryID ||
		receipt.Candidate.Commit != candidate.Commit || receipt.Candidate.Tree != candidate.Tree ||
		receipt.Environment != check.Environment || receipt.StartedAt != check.StartedAt ||
		receipt.CompletedAt != check.CompletedAt || check.CandidateTree != candidate.Tree ||
		receipt.WorkingDirectory != definition.WorkingDirectory ||
		!slices.Equal(receipt.Argv, definition.Argv) || receipt.TimeoutSeconds != definition.TimeoutSeconds ||
		check.ExitCode == nil || *check.ExitCode != 0 {
		return fmt.Errorf("check %q does not match its measured receipt", check.ID)
	}
	return nil
}

func deriveSubmissionID(deliveryID, workID string, attempt int64) (string, error) {
	if !ValidID(deliveryID) || !ValidID(workID) || attempt < 1 || attempt > 9_007_199_254_740_991 {
		return "", errors.New("cannot derive submission id from invalid work attempt")
	}
	canonical, err := EncodeCanonical(struct {
		SchemaVersion string `json:"schema_version"`
		DeliveryID    string `json:"delivery_id"`
		WorkID        string `json:"work_id"`
		Attempt       int64  `json:"attempt"`
	}{
		SchemaVersion: "sworn-submission-identity-v1",
		DeliveryID:    deliveryID,
		WorkID:        workID,
		Attempt:       attempt,
	})
	if err != nil {
		return "", err
	}
	return "submission-" + strings.TrimPrefix(CanonicalDigest(canonical), "sha256:"), nil
}

func bindEvidence(evidence Evidence, check Check, baseline BaselineCheck, candidateTree string) error {
	requirement := baseline.Evidence
	if evidence.ID != requirement.ID || !slices.Equal(evidence.AcceptanceIDs, requirement.AcceptanceIDs) ||
		len(evidence.PackIDs) != 0 || evidence.Kind != "test" || evidence.Boundary != requirement.Boundary ||
		evidence.Environment != check.Environment || evidence.UsesMocks != requirement.UsesMocks ||
		evidence.ProducerRunID != check.RunID || evidence.CandidateTree != candidateTree ||
		evidence.CapturedAt != check.CompletedAt || evidence.Artifact != check.Receipt ||
		evidence.Observed != requirement.Observed || evidence.Notes != "" {
		return fmt.Errorf("evidence %q does not match admitted producer semantics", evidence.ID)
	}
	return nil
}

func validateAcceptanceCoverage(requirements []AcceptanceRequirement, evidence []Evidence) error {
	rank := map[string]int{"component": 0, "assembled": 1}
	for _, requirement := range requirements {
		covered := false
		for _, item := range evidence {
			if slices.Contains(item.AcceptanceIDs, requirement.ID) && rank[item.Boundary] >= rank[requirement.Boundary] {
				covered = true
				break
			}
		}
		if !covered {
			return fmt.Errorf("acceptance %q lacks sufficient measured evidence", requirement.ID)
		}
	}
	return nil
}

func formatRecordTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
