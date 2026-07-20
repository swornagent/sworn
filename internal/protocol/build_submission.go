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
	record EncodedRecord
}

func (prepared PreparedSubmission) Record() EncodedRecord {
	record := prepared.record
	record.CanonicalJSON = append([]byte(nil), record.CanonicalJSON...)
	return record
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
	resolver := artifactResolver{reader: artifacts, cached: make(map[string]resolvedArtifact)}
	localChecks, err := resolveExactLocalChecks(ctx, &resolver, input.Plan, input.WorkID)
	if err != nil {
		return PreparedSubmission{}, err
	}
	work := localChecks.contract.View()
	target := input.Plan.Target()
	if input.Candidate.RepositoryID != target.Repository || input.Candidate.TargetRef != target.Ref {
		return PreparedSubmission{}, errors.New("candidate does not match the exact plan target")
	}
	if err := repository.VerifyCandidate(ctx, input.Candidate, work.Scope); err != nil {
		return PreparedSubmission{}, fmt.Errorf("verify submission candidate: %w", err)
	}
	planPolicy := input.Plan.Policy()
	authorityBytes, err := resolver.resolve(ctx, input.AuthorityReceipt)
	if err != nil {
		return PreparedSubmission{}, fmt.Errorf("resolve authority receipt: %w", err)
	}
	if input.AuthorityReceipt.MediaType != "application/json" {
		return PreparedSubmission{}, errors.New("authority receipt must be application/json")
	}
	if err := ValidateAuthorityApprovalForBuilder(authorityBytes, input.Plan, input.Builder); err != nil {
		return PreparedSubmission{}, err
	}

	checksByID := make(map[string]Check, len(input.Checks))
	for _, check := range input.Checks {
		if _, exists := checksByID[check.ID]; exists {
			return PreparedSubmission{}, fmt.Errorf("duplicate measured check %q", check.ID)
		}
		checksByID[check.ID] = check
	}
	if len(checksByID) != len(localChecks.requirements) {
		return PreparedSubmission{}, errors.New("measured checks do not exactly cover the exact policy baseline")
	}
	evidenceByID := make(map[string]Evidence, len(input.Evidence))
	for _, evidence := range input.Evidence {
		if _, exists := evidenceByID[evidence.ID]; exists {
			return PreparedSubmission{}, fmt.Errorf("duplicate measured evidence %q", evidence.ID)
		}
		evidenceByID[evidence.ID] = evidence
	}

	orderedChecks := make([]Check, 0, len(localChecks.requirements))
	orderedEvidence := make([]Evidence, 0, len(localChecks.requirements))
	for _, baseline := range localChecks.requirements {
		checkID := baseline.CheckID
		definitionPointer := baseline.Definition
		definition := baseline.definition
		check, exists := checksByID[checkID]
		if !exists || check.Outcome != "pass" {
			return PreparedSubmission{}, fmt.Errorf("required baseline check %q is absent or did not pass", checkID)
		}
		if check.Receipt.Ref != check.Receipt.Digest || check.Receipt.MediaType != "application/vnd.sworn.local-check-receipt+json" {
			return PreparedSubmission{}, fmt.Errorf("check %q does not use a local CAS receipt", checkID)
		}
		receiptBytes, err := resolver.resolve(ctx, check.Receipt)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q receipt: %w", checkID, err)
		}
		receipt, err := ParseLocalCheckReceipt(receiptBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q receipt: %w", checkID, err)
		}
		if err := bindCheckReceipt(check, receipt, checkID, definitionPointer, definition, input.Candidate); err != nil {
			return PreparedSubmission{}, err
		}
		for name, capture := range map[string]CapturedArtifact{"stdout": receipt.Stdout, "stderr": receipt.Stderr} {
			contents, err := resolver.resolve(ctx, capture.Pointer())
			if err != nil {
				return PreparedSubmission{}, fmt.Errorf("resolve check %q %s: %w", checkID, name, err)
			}
			if int64(len(contents)) != capture.Size {
				return PreparedSubmission{}, fmt.Errorf("check %q %s size does not match its receipt", checkID, name)
			}
		}
		if !digestPattern.MatchString(check.Environment.Ref) {
			return PreparedSubmission{}, fmt.Errorf("check %q has a non-content-addressed environment", checkID)
		}
		environmentBytes, err := resolver.resolve(ctx, Artifact{
			Ref: check.Environment.Ref, MediaType: "application/vnd.sworn.local-environment+json", Digest: check.Environment.Ref,
		})
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("resolve check %q environment: %w", checkID, err)
		}
		environment, err := ParseLocalEnvironment(environmentBytes)
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("parse check %q environment: %w", checkID, err)
		}
		snapshotDigest, err := SnapshotDigest()
		if err != nil {
			return PreparedSubmission{}, fmt.Errorf("measure embedded protocol snapshot: %w", err)
		}
		if environment.ProtocolSnapshotDigest != "sha256:"+snapshotDigest {
			return PreparedSubmission{}, fmt.Errorf("check %q environment names a different protocol snapshot", checkID)
		}
		evidence, exists := evidenceByID[definition.Evidence.ID]
		if !exists {
			return PreparedSubmission{}, fmt.Errorf("check %q lacks policy-defined evidence %q", checkID, definition.Evidence.ID)
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
	if len(evidenceByID) != len(localChecks.requirements) {
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
	return PreparedSubmission{record: record}, nil
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

// ValidateAuthorityApprovalForBuilder proves that exact receipt bytes match the
// exact plan and precede the builder run. Authentication of those bytes remains
// the authority service and store closure's responsibility.
func ValidateAuthorityApprovalForBuilder(contents []byte, plan ExactPlan, builder BuilderRun) error {
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
	required := make(map[string]int, len(requirements))
	for _, requirement := range requirements {
		required[requirement.ID] = rank[requirement.EvidenceLevel]
	}
	seenEvidence := make(map[string]struct{}, len(evidence))
	covered := make(map[string]struct{}, len(requirements))
	for _, item := range evidence {
		if _, duplicate := seenEvidence[item.ID]; duplicate {
			return fmt.Errorf("coverage inputs reuse evidence id %q", item.ID)
		}
		seenEvidence[item.ID] = struct{}{}
		for _, acceptanceID := range item.AcceptanceIDs {
			minimum, exists := required[acceptanceID]
			if !exists {
				return fmt.Errorf("evidence %q names unknown exact-plan acceptance %q", item.ID, acceptanceID)
			}
			if rank[item.Boundary] >= minimum {
				covered[acceptanceID] = struct{}{}
			}
		}
	}
	for _, requirement := range requirements {
		if _, exists := covered[requirement.ID]; !exists {
			return fmt.Errorf("acceptance %q lacks sufficient evidence coverage", requirement.ID)
		}
	}
	return nil
}

func formatRecordTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
