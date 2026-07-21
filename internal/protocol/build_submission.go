package protocol

import (
	"bytes"
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

// MeasuredCheck is the minimum journal-owned fact needed to admit one
// policy-ordered local-check result. Check and evidence projections are derived
// from the exact policy and the resolved receipt rather than supplied by a
// caller.
type MeasuredCheck struct {
	RunID                 string
	RuntimeManifestDigest string
	Receipt               Artifact
}

type SubmissionInput struct {
	Attempt          int64
	CreatedAt        time.Time
	Plan             ExactPlan
	WorkID           string
	AuthorityReceipt Artifact
	Builder          BuilderRun
	Candidate        repo.Candidate
	MeasuredChecks   []MeasuredCheck
}

// BuildSubmission rechecks Git and every raw artifact, then constructs a
// canonical Standard submission from an exact plan, its selected policy, and
// policy-ordered journal-owned receipt facts.
// It performs no authentication, journal lookup, storage, clock read, state
// transition, or external effect.
func BuildSubmission(
	ctx context.Context,
	repository *repo.Repository,
	artifacts ArtifactReader,
	input SubmissionInput,
) (EncodedRecord, error) {
	if repository == nil || artifacts == nil {
		return EncodedRecord{}, errors.New("submission requires repository and artifact readers")
	}
	resolver := artifactResolver{reader: artifacts, cached: make(map[string]resolvedArtifact)}
	localChecks, err := resolveExactLocalChecks(ctx, &resolver, input.Plan, input.WorkID)
	if err != nil {
		return EncodedRecord{}, err
	}
	work := localChecks.contract.View()
	target := input.Plan.Target()
	if input.Candidate.RepositoryID != target.Repository || input.Candidate.TargetRef != target.Ref {
		return EncodedRecord{}, errors.New("candidate does not match the exact plan target")
	}
	if err := repository.VerifyCandidate(ctx, input.Candidate, work.Scope); err != nil {
		return EncodedRecord{}, fmt.Errorf("verify submission candidate: %w", err)
	}
	authorityBytes, err := resolver.resolve(ctx, input.AuthorityReceipt, MaximumControlReceiptBytes)
	if err != nil {
		return EncodedRecord{}, fmt.Errorf("resolve authority receipt: %w", err)
	}
	if input.AuthorityReceipt.MediaType != "application/json" {
		return EncodedRecord{}, errors.New("authority receipt must be application/json")
	}
	if err := ValidateAuthorityApprovalForBuilder(authorityBytes, input.Plan, input.Builder); err != nil {
		return EncodedRecord{}, err
	}
	if len(input.MeasuredChecks) != len(localChecks.requirements) {
		return EncodedRecord{}, errors.New("measured check receipts do not exactly cover the exact policy baseline")
	}
	snapshotDigest, err := SnapshotDigest()
	if err != nil {
		return EncodedRecord{}, fmt.Errorf("measure embedded protocol snapshot: %w", err)
	}

	orderedChecks := make([]Check, 0, len(localChecks.requirements))
	orderedEvidence := make([]Evidence, 0, len(localChecks.requirements))
	for index, baseline := range localChecks.requirements {
		checkID := baseline.CheckID
		measured := input.MeasuredChecks[index]
		if measured.Receipt.MediaType != "application/vnd.sworn.local-check-receipt+json" {
			return EncodedRecord{}, fmt.Errorf("check %q does not use a local CAS receipt", checkID)
		}
		receiptBytes, err := resolver.resolve(ctx, measured.Receipt, MaximumLocalCheckReceiptBytes)
		if err != nil {
			return EncodedRecord{}, fmt.Errorf("resolve check %q receipt: %w", checkID, err)
		}
		receipt, err := ParseLocalCheckReceipt(receiptBytes)
		if err != nil {
			return EncodedRecord{}, fmt.Errorf("parse check %q receipt: %w", checkID, err)
		}
		check, evidence, err := projectMeasuredReceipt(measured, receipt, baseline, input.Candidate)
		if err != nil {
			return EncodedRecord{}, err
		}
		for _, capture := range [...]CapturedArtifact{receipt.Stdout, receipt.Stderr} {
			contents, err := resolver.resolve(ctx, capture.Pointer(), uint64(capture.Size))
			if err != nil {
				return EncodedRecord{}, fmt.Errorf("resolve check %q output: %w", checkID, err)
			}
			if int64(len(contents)) != capture.Size {
				return EncodedRecord{}, fmt.Errorf("check %q output size does not match its receipt", checkID)
			}
		}
		environmentBytes, err := resolver.resolve(ctx, Artifact{
			Ref: receipt.Environment.Ref, MediaType: LocalEnvironmentMediaType, Digest: receipt.Environment.Ref,
		}, MaximumLocalEnvironmentBytes)
		if err != nil {
			return EncodedRecord{}, fmt.Errorf("resolve check %q environment: %w", checkID, err)
		}
		environment, err := ParseLocalEnvironment(environmentBytes)
		if err != nil {
			return EncodedRecord{}, fmt.Errorf("parse check %q environment: %w", checkID, err)
		}
		if environment.ProtocolSnapshotDigest != "sha256:"+snapshotDigest ||
			environment.SchemaVersion != ContentEnvironmentSchemaVersion ||
			environment.RuntimeManifestDigest != measured.RuntimeManifestDigest {
			return EncodedRecord{}, fmt.Errorf("check %q environment does not bind the protocol snapshot and journal runtime", checkID)
		}
		orderedChecks = append(orderedChecks, check)
		orderedEvidence = append(orderedEvidence, evidence)
	}
	slices.SortFunc(orderedEvidence, func(left, right Evidence) int { return bytes.Compare([]byte(left.ID), []byte(right.ID)) })
	if input.CreatedAt.IsZero() || input.CreatedAt.Location() != time.UTC {
		return EncodedRecord{}, errors.New("submission creation time must be an explicit UTC time")
	}
	submissionID, err := SubmissionID(input.Plan.DeliveryID(), work.ID, input.Attempt)
	if err != nil {
		return EncodedRecord{}, err
	}
	submission := Submission{
		SchemaVersion:    SubmissionSchemaVersion,
		SubmissionID:     submissionID,
		DeliveryID:       input.Plan.DeliveryID(),
		WorkID:           work.ID,
		Attempt:          input.Attempt,
		CreatedAt:        input.CreatedAt.Format(time.RFC3339Nano),
		PlanDigest:       input.Plan.Record().Digest,
		ContractDigest:   localChecks.ContractDigest(),
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
			PolicyRef:    input.Plan.Policy().Ref,
			PolicyDigest: input.Plan.Policy().Digest,
		},
		ChangedPaths: append([]string{}, input.Candidate.ChangedPaths...),
		Checks:       orderedChecks,
		Evidence:     orderedEvidence,
	}
	return EncodeSubmission(submission)
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

func (resolver *artifactResolver) resolve(
	ctx context.Context,
	pointer Artifact,
	maximumBytes uint64,
) ([]byte, error) {
	if cached, exists := resolver.cached[pointer.Digest]; exists {
		if err := validateCASArtifact(pointer); err != nil {
			return nil, err
		}
		contents, err := validateResolvedArtifact(pointer, cached.mediaType, cached.contents, maximumBytes)
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), contents...), nil
	}
	contents, err := ResolveArtifact(ctx, resolver.reader, pointer, maximumBytes)
	if err != nil {
		return nil, err
	}
	resolver.cached[pointer.Digest] = resolvedArtifact{
		mediaType: pointer.MediaType,
		contents:  append([]byte(nil), contents...),
	}
	return contents, nil
}

// ResolveArtifact resolves and verifies one bounded content-addressed artifact.
func ResolveArtifact(
	ctx context.Context,
	reader ArtifactReader,
	pointer Artifact,
	maximumBytes uint64,
) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("artifact resolution requires a reader")
	}
	if err := validateCASArtifact(pointer); err != nil {
		return nil, err
	}
	mediaType, contents, err := reader.Artifact(ctx, pointer.Digest)
	if err != nil {
		return nil, err
	}
	return validateResolvedArtifact(pointer, mediaType, contents, maximumBytes)
}

func validateCASArtifact(pointer Artifact) error {
	if err := validateArtifact(pointer, "artifact"); err != nil {
		return err
	}
	if pointer.Ref != pointer.Digest {
		return errors.New("Sworn artifact pointer is not a CAS reference")
	}
	return nil
}

func validateResolvedArtifact(
	pointer Artifact,
	mediaType string,
	contents []byte,
	maximumBytes uint64,
) ([]byte, error) {
	if uint64(len(contents)) > maximumBytes {
		return nil, errors.New("artifact exceeds byte ceiling")
	}
	if mediaType != pointer.MediaType || RawDigest(contents) != pointer.Digest {
		return nil, errors.New("artifact bytes do not match their media type and digest")
	}
	if err := ValidateArtifactContent(mediaType, contents); err != nil {
		return nil, err
	}
	return contents, nil
}

// ValidateAuthorityApprovalForBuilder proves that exact receipt bytes match the
// exact plan and precede the builder run. Authentication of those bytes remains
// the authority service and store closure's responsibility.
func ValidateAuthorityApprovalForBuilder(contents []byte, plan ExactPlan, builder BuilderRun) error {
	receipt, err := ParseAuthorityApproval(contents)
	if err != nil {
		return err
	}
	authority := plan.Authority()
	target := plan.Target()
	if receipt.PlanDigest != plan.Record().Digest || receipt.AuthorityDigest != authority.Digest ||
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
	required := []string{"inspect", "edit", "execute", "commit"}
	for index, raw := range receipt.Grants {
		grant, err := ParseAuthorityGrant(raw)
		if err != nil {
			return errors.New("authority approval contains an invalid grant")
		}
		if !bytes.Equal(grant.CanonicalJSON(), authority.Grants[index].CanonicalJSON()) {
			return errors.New("authority approval grants do not match the exact plan")
		}
		if index := slices.Index(required, grant.Action()); index >= 0 {
			required = slices.Delete(required, index, index+1)
		}
	}
	if len(required) != 0 {
		return fmt.Errorf("authority approval lacks %s workspace grant", required[0])
	}
	return nil
}

func projectMeasuredReceipt(
	measured MeasuredCheck,
	receipt LocalCheckReceipt,
	requirement LocalCheckRequirement,
	candidate repo.Candidate,
) (Check, Evidence, error) {
	definition := requirement.definition
	if receipt.Outcome != "pass" || receipt.CheckID != requirement.CheckID || receipt.RunID != measured.RunID ||
		receipt.Definition != requirement.Definition || receipt.Candidate.Repository != candidate.RepositoryID ||
		receipt.Candidate.Commit != candidate.Commit || receipt.Candidate.Tree != candidate.Tree ||
		receipt.WorkingDirectory != definition.WorkingDirectory ||
		!slices.Equal(receipt.Argv, definition.Argv) || receipt.TimeoutSeconds != definition.TimeoutSeconds ||
		receipt.ExitCode != 0 {
		return Check{}, Evidence{}, fmt.Errorf("check %q does not match its journal receipt", requirement.CheckID)
	}
	exitCode := receipt.ExitCode
	check := Check{
		ID: requirement.CheckID, Outcome: receipt.Outcome, RunID: measured.RunID,
		CandidateTree: candidate.Tree, Environment: receipt.Environment,
		StartedAt: receipt.StartedAt, CompletedAt: receipt.CompletedAt,
		ExitCode: &exitCode, Receipt: measured.Receipt,
	}
	evidence := Evidence{
		ID: definition.Evidence.ID, AcceptanceIDs: slices.Clone(definition.Evidence.AcceptanceIDs),
		Kind: "test", Boundary: definition.Evidence.Boundary, Environment: receipt.Environment,
		UsesMocks: definition.Evidence.UsesMocks, ProducerRunID: measured.RunID,
		CandidateTree: candidate.Tree, CapturedAt: receipt.CompletedAt,
		Artifact: measured.Receipt, Observed: definition.Evidence.Observed,
	}
	return check, evidence, nil
}

// SubmissionID deterministically derives Baton's bounded submission identity
// from the exact work attempt admitted by the engine.
func SubmissionID(deliveryID, workID string, attempt int64) (string, error) {
	if !ValidID(deliveryID) || !ValidID(workID) || !ValidPositiveSafeInteger(attempt) {
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

func validateAcceptanceCoverage(requirements []PlanAcceptance, evidence []Evidence) error {
	covered := make([]bool, len(requirements))
	for evidenceIndex, item := range evidence {
		for _, earlier := range evidence[:evidenceIndex] {
			if earlier.ID == item.ID {
				return fmt.Errorf("coverage inputs reuse evidence id %q", item.ID)
			}
		}
		for _, acceptanceID := range item.AcceptanceIDs {
			requirementIndex := slices.IndexFunc(requirements, func(value PlanAcceptance) bool {
				return value.ID == acceptanceID
			})
			if requirementIndex < 0 {
				return fmt.Errorf("evidence %q names unknown exact-plan acceptance %q", item.ID, acceptanceID)
			}
			requirement := requirements[requirementIndex]
			covered[requirementIndex] = covered[requirementIndex] ||
				requirement.EvidenceLevel == "component" || item.Boundary == "assembled"
		}
	}
	for index, requirement := range requirements {
		if !covered[index] {
			return fmt.Errorf("acceptance %q lacks sufficient evidence coverage", requirement.ID)
		}
	}
	return nil
}
