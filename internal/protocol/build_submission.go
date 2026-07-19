package protocol

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/repo"
)

type ArtifactReader interface {
	Artifact(context.Context, string) (mediaType string, contents []byte, err error)
}

type SubmissionInput struct {
	Attempt          int64
	CreatedAt        time.Time
	Plan             ExactPlan
	WorkID           string
	AuthorityReceipt Artifact
	Builder          BuilderRun
	Candidate        repo.Candidate
	Checks           []Check
	Evidence         []Evidence
}

// PreparedSubmission is an opaque construction capability minted only after
// BuildSubmission has re-bound immutable Git and artifact bytes to
// an exact plan, its selected policy, an authority receipt, and producer facts.
// It is not an authenticated authority or journal-registration proof; those
// later engine boundaries must precede reviewable/PASS state.
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
// Standard submission from an exact plan, its selected policy, and producer
// facts.
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
	planRecord := input.Plan.Record()
	if planRecord.Kind != DeliveryPlanSchemaVersion || !ValidDigest(planRecord.Digest) {
		return PreparedSubmission{}, errors.New("submission requires an exact delivery plan")
	}
	contract, exists := input.Plan.Work(input.WorkID)
	if !exists {
		return PreparedSubmission{}, fmt.Errorf("work %q is absent from the exact plan", input.WorkID)
	}
	work := contract.View()
	if err := validateInitialContract(len(input.Plan.WorkIDs()), work); err != nil {
		return PreparedSubmission{}, err
	}
	target := input.Plan.Target()
	if input.Candidate.RepositoryID != target.Repository || input.Candidate.TargetRef != target.Ref {
		return PreparedSubmission{}, errors.New("candidate does not match the exact plan target")
	}
	if err := repository.VerifyCandidate(ctx, input.Candidate, work.Scope); err != nil {
		return PreparedSubmission{}, fmt.Errorf("verify submission candidate: %w", err)
	}
	resolver := artifactResolver{reader: artifacts, cached: make(map[string]resolvedArtifact)}
	planPolicy := input.Plan.Policy()
	policyBytes, err := resolver.resolve(ctx, Artifact{
		Ref: planPolicy.Digest, MediaType: "application/json", Digest: planPolicy.Digest,
	})
	if err != nil {
		return PreparedSubmission{}, fmt.Errorf("resolve exact assurance policy: %w", err)
	}
	policy, err := parseAssurancePolicyRegistry(policyBytes)
	if err != nil {
		return PreparedSubmission{}, err
	}
	authorityBytes, err := resolver.resolve(ctx, input.AuthorityReceipt)
	if err != nil {
		return PreparedSubmission{}, fmt.Errorf("resolve authority receipt: %w", err)
	}
	if input.AuthorityReceipt.MediaType != "application/json" {
		return PreparedSubmission{}, errors.New("authority receipt must be application/json")
	}
	if err := validateAuthorityApproval(authorityBytes, input.Plan, input.Builder); err != nil {
		return PreparedSubmission{}, err
	}

	checksByID := make(map[string]Check, len(input.Checks))
	for _, check := range input.Checks {
		if _, exists := checksByID[check.ID]; exists {
			return PreparedSubmission{}, fmt.Errorf("duplicate measured check %q", check.ID)
		}
		checksByID[check.ID] = check
	}
	if len(checksByID) != len(policy.checks) {
		return PreparedSubmission{}, errors.New("measured checks do not exactly cover the exact policy baseline")
	}
	evidenceByID := make(map[string]Evidence, len(input.Evidence))
	for _, evidence := range input.Evidence {
		if _, exists := evidenceByID[evidence.ID]; exists {
			return PreparedSubmission{}, fmt.Errorf("duplicate measured evidence %q", evidence.ID)
		}
		evidenceByID[evidence.ID] = evidence
	}

	acceptance := make(map[string]PlanAcceptance, len(work.Acceptance))
	for _, requirement := range work.Acceptance {
		acceptance[requirement.ID] = requirement
	}
	usedEvidence := make(map[string]struct{}, len(policy.checks))
	orderedChecks := make([]Check, 0, len(policy.checks))
	orderedEvidence := make([]Evidence, 0, len(policy.checks))
	for _, baseline := range policy.checks {
		definitionPointer := Artifact{
			Ref: baseline.definition.Digest, MediaType: baseline.definition.MediaType,
			Digest: baseline.definition.Digest,
		}
		definitionBytes, err := resolver.resolve(ctx, definitionPointer)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q definition: %w", baseline.id, err)
		}
		definition, err := ParseLocalCheckDefinition(definitionBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q definition: %w", baseline.id, err)
		}
		for _, acceptanceID := range definition.Evidence.AcceptanceIDs {
			if _, exists := acceptance[acceptanceID]; !exists {
				return PreparedSubmission{}, fmt.Errorf("check %q names unknown exact-plan acceptance %q", baseline.id, acceptanceID)
			}
		}
		if _, duplicate := usedEvidence[definition.Evidence.ID]; duplicate {
			return PreparedSubmission{}, fmt.Errorf("exact policy checks reuse evidence id %q", definition.Evidence.ID)
		}
		usedEvidence[definition.Evidence.ID] = struct{}{}
		check, exists := checksByID[baseline.id]
		if !exists || check.Outcome != "pass" {
			return PreparedSubmission{}, fmt.Errorf("required baseline check %q is absent or did not pass", baseline.id)
		}
		if check.Receipt.Ref != check.Receipt.Digest || check.Receipt.MediaType != "application/vnd.sworn.local-check-receipt+json" {
			return PreparedSubmission{}, fmt.Errorf("check %q does not use a local CAS receipt", baseline.id)
		}
		receiptBytes, err := resolver.resolve(ctx, check.Receipt)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q receipt: %w", baseline.id, err)
		}
		receipt, err := ParseLocalCheckReceipt(receiptBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q receipt: %w", baseline.id, err)
		}
		if err := bindCheckReceipt(check, receipt, baseline.id, definitionPointer, definition, input.Candidate); err != nil {
			return PreparedSubmission{}, err
		}
		for name, capture := range map[string]CapturedArtifact{"stdout": receipt.Stdout, "stderr": receipt.Stderr} {
			contents, err := resolver.resolve(ctx, capture.Pointer())
			if err != nil {
				return PreparedSubmission{}, fmt.Errorf("resolve check %q %s: %w", baseline.id, name, err)
			}
			if int64(len(contents)) != capture.Size {
				return PreparedSubmission{}, fmt.Errorf("check %q %s size does not match its receipt", baseline.id, name)
			}
		}
		if !digestPattern.MatchString(check.Environment.Ref) {
			return PreparedSubmission{}, fmt.Errorf("check %q has a non-content-addressed environment", baseline.id)
		}
		environmentBytes, err := resolver.resolve(ctx, Artifact{
			Ref: check.Environment.Ref, MediaType: "application/vnd.sworn.local-environment+json", Digest: check.Environment.Ref,
		})
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q environment: %w", baseline.id, err)
		}
		environment, err := ParseLocalEnvironment(environmentBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q environment: %w", baseline.id, err)
		}
		snapshotDigest, err := SnapshotDigest()
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("measure embedded protocol snapshot: %w", err)
		}
		if environment.ProtocolSnapshotDigest != "sha256:"+snapshotDigest {
			return PreparedSubmission{}, fmt.Errorf("check %q environment names a different protocol snapshot", baseline.id)
		}
		evidence, exists := evidenceByID[definition.Evidence.ID]
		if !exists {
			return PreparedSubmission{}, fmt.Errorf("check %q lacks policy-defined evidence %q", baseline.id, definition.Evidence.ID)
		}
		if err := bindEvidence(evidence, check, definition.Evidence, input.Candidate.Tree); err != nil {
			return PreparedSubmission{}, err
		}
		if _, err := resolver.resolve(ctx, evidence.Artifact); err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve evidence %q: %w", evidence.ID, err)
		}
		orderedChecks = append(orderedChecks, check)
		orderedEvidence = append(orderedEvidence, evidence)
	}
	if len(evidenceByID) != len(usedEvidence) {
		return PreparedSubmission{}, errors.New("measured evidence does not exactly cover the exact policy baseline")
	}
	slices.SortFunc(orderedEvidence, func(left, right Evidence) int { return bytes.Compare([]byte(left.ID), []byte(right.ID)) })
	if err := validateAcceptanceCoverage(work.Acceptance, orderedEvidence); err != nil {
		return PreparedSubmission{}, err
	}
	createdAt := input.CreatedAt.UTC()
	if input.CreatedAt.IsZero() || input.CreatedAt.Location() != time.UTC {
		return PreparedSubmission{}, errors.New("submission creation time must be an explicit UTC time")
	}
	submissionID, err := deriveSubmissionID(input.Plan.DeliveryID(), work.ID, input.Attempt)
	if err != nil {
		return PreparedSubmission{}, err
	}
	submission := Submission{
		SchemaVersion:    SubmissionSchemaVersion,
		SubmissionID:     submissionID,
		DeliveryID:       input.Plan.DeliveryID(),
		WorkID:           work.ID,
		Attempt:          input.Attempt,
		CreatedAt:        formatRecordTime(createdAt),
		PlanDigest:       planRecord.Digest,
		ContractDigest:   contract.Digest(),
		AuthorityReceipt: input.AuthorityReceipt,
		Builder:          input.Builder,
		Base: GitPoint{
			Repository: target.Repository,
			Ref:        target.Ref,
			Commit:     input.Candidate.BaseCommit,
		},
		Candidate: CandidatePoint{
			Repository: input.Candidate.RepositoryID,
			Commit:     input.Candidate.Commit,
			Tree:       input.Candidate.Tree,
		},
		Assurance: Assurance{
			Profile:      work.Assurance.Profile,
			Packs:        slices.Clone(work.Assurance.Packs),
			PolicyRef:    planPolicy.Ref,
			PolicyDigest: planPolicy.Digest,
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

func validateInitialContract(workCount int, work PlanWorkView) error {
	if workCount != 1 || len(work.DependsOn) != 0 {
		return errors.New("submission construction supports only one dependency-free work contract")
	}
	if work.Assurance.Profile != "standard" || len(work.Assurance.Packs) != 0 {
		return errors.New("submission construction supports only Standard assurance without selected packs")
	}
	for _, acceptance := range work.Acceptance {
		if acceptance.EvidenceLevel != "component" && acceptance.EvidenceLevel != "assembled" {
			return fmt.Errorf("acceptance %q exceeds the initial local-evidence capability", acceptance.ID)
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

func validateAuthorityApproval(contents []byte, plan ExactPlan, builder BuilderRun) error {
	receipt, err := ParseAuthorityApproval(contents)
	if err != nil {
		return err
	}
	planRecord := plan.Record()
	authority := plan.Authority()
	target := plan.Target()
	if receipt.PlanDigest != planRecord.Digest || receipt.AuthorityDigest != authority.Digest ||
		receipt.SourceRef != authority.SourceRef || receipt.Repository != target.Repository ||
		receipt.TargetRef != target.Ref {
		return errors.New("authority approval does not match the exact plan")
	}
	approvedAt, err := parseRecordTime(receipt.ApprovedAt, "authority approval")
	if err != nil {
		return err
	}
	builderStart, err := parseRecordTime(builder.StartedAt, "builder start")
	if err != nil || approvedAt.After(builderStart) {
		return errors.New("authority approval does not precede the builder")
	}
	if len(receipt.Grants) != len(authority.Grants) {
		return errors.New("authority approval grants do not match the exact plan")
	}
	required := map[string]bool{"inspect": false, "edit": false, "execute": false, "commit": false}
	for index, raw := range receipt.Grants {
		grant, err := ParseAuthorityGrant(raw)
		if err != nil {
			return errors.New("authority approval contains an invalid grant")
		}
		if !bytes.Equal(grant.CanonicalJSON(), authority.Grants[index].CanonicalJSON()) {
			return errors.New("authority approval grants do not match the exact plan")
		}
		if _, exists := required[grant.Action()]; exists {
			required[grant.Action()] = true
		}
	}
	for action, present := range required {
		if !present {
			return fmt.Errorf("authority approval lacks %s workspace grant", action)
		}
	}
	return nil
}

func bindCheckReceipt(
	check Check,
	receipt LocalCheckReceipt,
	checkID string,
	definitionPointer Artifact,
	definition LocalCheckDefinition,
	candidate repo.Candidate,
) error {
	if receipt.Outcome != "pass" || receipt.CheckID != check.ID || receipt.RunID != check.RunID ||
		check.ID != checkID || receipt.Definition != definitionPointer || receipt.Candidate.Repository != candidate.RepositoryID ||
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

func bindEvidence(evidence Evidence, check Check, requirement LocalEvidenceDefinition, candidateTree string) error {
	if evidence.ID != requirement.ID || !slices.Equal(evidence.AcceptanceIDs, requirement.AcceptanceIDs) ||
		len(evidence.PackIDs) != 0 || evidence.Kind != "test" || evidence.Boundary != requirement.Boundary ||
		evidence.Environment != check.Environment || evidence.UsesMocks != requirement.UsesMocks ||
		evidence.ProducerRunID != check.RunID || evidence.CandidateTree != candidateTree ||
		evidence.CapturedAt != check.CompletedAt || evidence.Artifact != check.Receipt ||
		evidence.Observed != requirement.Observed || evidence.Notes != "" {
		return fmt.Errorf("evidence %q does not match policy-defined producer semantics", evidence.ID)
	}
	return nil
}

func validateAcceptanceCoverage(requirements []PlanAcceptance, evidence []Evidence) error {
	rank := map[string]int{"component": 0, "assembled": 1}
	for _, requirement := range requirements {
		covered := false
		for _, item := range evidence {
			if slices.Contains(item.AcceptanceIDs, requirement.ID) && rank[item.Boundary] >= rank[requirement.EvidenceLevel] {
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
