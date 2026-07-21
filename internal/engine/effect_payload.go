package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

const (
	BuildOutcomeCandidateReady     = "candidate_ready"
	LocalCheckOutcomePass          = "pass"
	LocalCheckOutcomeNotAdmitted   = "not_admitted"
	LocalCheckOutcomeControlled    = "controlled"
	VerifierOutcomeAssessmentReady = "assessment_ready"

	localCheckReceiptMediaType        = "application/vnd.sworn.local-check-receipt+json"
	verifierArtifactMediaType         = "application/json"
	verifierExecutionReceiptMediaType = "application/vnd.sworn.verifier-execution-receipt+json"
)

var objectIDPattern = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)

// JournalEffect is the engine-owned projection consumed by execution adapters.
// It deliberately has no wire representation of its own.
type JournalEffect struct {
	ID            string
	DeliveryRunID string
	Kind          EffectKind
	Attempt       int64
	Request       json.RawMessage
	Result        json.RawMessage
}

// BuildEffectResult is the exact durable output of one builder effect.
type BuildEffectResult struct {
	SchemaVersion string              `json:"schema_version"`
	Outcome       string              `json:"outcome"`
	Builder       protocol.BuilderRun `json:"builder"`
	Candidate     repo.Candidate      `json:"candidate"`
}

// LocalCheckEffectRequest binds one check invocation to the exact builder,
// definition, candidate attempt, and configured runtime selected by the engine.
type LocalCheckEffectRequest struct {
	SchemaVersion         string `json:"schema_version"`
	DeliveryRunID         string `json:"delivery_run_id"`
	DeliveryID            string `json:"delivery_id"`
	WorkID                string `json:"work_id"`
	WorkAttempt           int64  `json:"work_attempt"`
	BuilderEffectID       string `json:"builder_effect_id"`
	CheckID               string `json:"check_id"`
	DefinitionDigest      string `json:"definition_digest"`
	RuntimeManifestDigest string `json:"runtime_manifest_digest"`
}

// LocalCheckEffectResult retains the receipt for every known check outcome.
// The artifact store closes the pointer over the receipt contents separately.
type LocalCheckEffectResult struct {
	SchemaVersion string            `json:"schema_version"`
	Outcome       string            `json:"outcome"`
	Receipt       protocol.Artifact `json:"receipt"`
}

// VerifierEffectRequest closes one fresh verifier invocation over exact plan,
// submission, candidate, dispatch, profile, agent, and verification-epoch
// truth. Adapters consume this value but cannot choose any of its bindings.
type VerifierEffectRequest struct {
	SchemaVersion         string                  `json:"schema_version"`
	DeliveryRunID         string                  `json:"delivery_run_id"`
	DeliveryID            string                  `json:"delivery_id"`
	WorkID                string                  `json:"work_id"`
	WorkAttempt           int64                   `json:"work_attempt"`
	PlanDigest            string                  `json:"plan_digest"`
	SubmissionID          string                  `json:"submission_id"`
	SubmissionDigest      string                  `json:"submission_digest"`
	Candidate             protocol.CandidatePoint `json:"candidate"`
	DispatchID            string                  `json:"dispatch_id"`
	DispatchReceipt       protocol.Artifact       `json:"dispatch_receipt"`
	VerifierProfileDigest string                  `json:"verifier_profile_digest"`
	Agent                 string                  `json:"agent"`
	VerificationEpoch     int64                   `json:"verification_epoch"`
}

// VerifierEffectResult retains the exact raw assessment pointer and
// adapter-recorded review timestamps. The configured verifier agent remains
// authoritative in the request rather than being repeated as adapter-selected
// result data.
type VerifierEffectResult struct {
	SchemaVersion     string            `json:"schema_version"`
	Outcome           string            `json:"outcome"`
	DispatchID        string            `json:"dispatch_id"`
	VerificationEpoch int64             `json:"verification_epoch"`
	Assessment        protocol.Artifact `json:"assessment"`
	ExecutionReceipt  protocol.Artifact `json:"execution_receipt"`
	StartedAt         string            `json:"started_at"`
	CompletedAt       string            `json:"completed_at"`
}

func EncodeBuildEffectResult(result BuildEffectResult) (json.RawMessage, error) {
	if err := validateBuildEffectResult(result); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(result, "build effect result")
}

func EncodeBuildAttemptIdentity(identity BuildAttemptIdentity) (json.RawMessage, error) {
	if err := validateBuildAttemptIdentity(identity); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(identity, "build attempt identity")
}

func ParseBuildAttemptIdentity(encoded json.RawMessage) (BuildAttemptIdentity, error) {
	identity, err := decodeCanonicalEffectPayload[BuildAttemptIdentity](encoded, "build attempt identity")
	if err != nil {
		return BuildAttemptIdentity{}, err
	}
	if err := validateBuildAttemptIdentity(identity); err != nil {
		return BuildAttemptIdentity{}, err
	}
	return identity, nil
}

func EncodeCheckAttemptIdentity(identity CheckAttemptIdentity) (json.RawMessage, error) {
	if err := validateCheckAttemptIdentity(identity); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(identity, "check attempt identity")
}

func ParseCheckAttemptIdentity(encoded json.RawMessage) (CheckAttemptIdentity, error) {
	identity, err := decodeCanonicalEffectPayload[CheckAttemptIdentity](encoded, "check attempt identity")
	if err != nil {
		return CheckAttemptIdentity{}, err
	}
	if err := validateCheckAttemptIdentity(identity); err != nil {
		return CheckAttemptIdentity{}, err
	}
	return identity, nil
}

func EncodeVerifierAttemptIdentity(identity VerifierAttemptIdentity) (json.RawMessage, error) {
	if err := validateVerifierAttemptIdentity(identity); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(identity, "verifier attempt identity")
}

func ParseVerifierAttemptIdentity(encoded json.RawMessage) (VerifierAttemptIdentity, error) {
	identity, err := decodeCanonicalEffectPayload[VerifierAttemptIdentity](encoded, "verifier attempt identity")
	if err != nil {
		return VerifierAttemptIdentity{}, err
	}
	if err := validateVerifierAttemptIdentity(identity); err != nil {
		return VerifierAttemptIdentity{}, err
	}
	return identity, nil
}

func ParseBuildEffectResult(encoded json.RawMessage) (BuildEffectResult, error) {
	result, err := decodeCanonicalEffectPayload[BuildEffectResult](encoded, "build effect result")
	if err != nil {
		return BuildEffectResult{}, err
	}
	if err := validateBuildEffectResult(result); err != nil {
		return BuildEffectResult{}, err
	}
	return result, nil
}

func EncodeLocalCheckEffectRequest(request LocalCheckEffectRequest) (json.RawMessage, error) {
	if err := validateLocalCheckEffectRequest(request); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(request, "local check effect request")
}

func ParseLocalCheckEffectRequest(encoded json.RawMessage) (LocalCheckEffectRequest, error) {
	if err := validateStrictJSON(encoded); err != nil {
		return LocalCheckEffectRequest{}, fmt.Errorf("decode local check effect request: %w", err)
	}
	request, err := decodePayload[LocalCheckEffectRequest](encoded)
	if err != nil {
		return LocalCheckEffectRequest{}, fmt.Errorf("decode local check effect request: %w", err)
	}
	if err := validateLocalCheckEffectRequest(request); err != nil {
		return LocalCheckEffectRequest{}, err
	}
	return request, nil
}

func EncodeLocalCheckEffectResult(result LocalCheckEffectResult) (json.RawMessage, error) {
	if err := validateLocalCheckEffectResult(result); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(result, "local check effect result")
}

func ParseLocalCheckEffectResult(encoded json.RawMessage) (LocalCheckEffectResult, error) {
	result, err := decodeCanonicalEffectPayload[LocalCheckEffectResult](encoded, "local check effect result")
	if err != nil {
		return LocalCheckEffectResult{}, err
	}
	if err := validateLocalCheckEffectResult(result); err != nil {
		return LocalCheckEffectResult{}, err
	}
	return result, nil
}

func EncodeVerifierEffectRequest(request VerifierEffectRequest) (json.RawMessage, error) {
	if err := validateVerifierEffectRequest(request); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(request, "verifier effect request")
}

func ParseVerifierEffectRequest(encoded json.RawMessage) (VerifierEffectRequest, error) {
	request, err := decodeCanonicalEffectPayload[VerifierEffectRequest](encoded, "verifier effect request")
	if err != nil {
		return VerifierEffectRequest{}, err
	}
	if err := validateVerifierEffectRequest(request); err != nil {
		return VerifierEffectRequest{}, err
	}
	return request, nil
}

func EncodeVerifierEffectResult(result VerifierEffectResult) (json.RawMessage, error) {
	if err := validateVerifierEffectResult(result); err != nil {
		return nil, err
	}
	return encodeCanonicalEffectPayload(result, "verifier effect result")
}

func ParseVerifierEffectResult(encoded json.RawMessage) (VerifierEffectResult, error) {
	result, err := decodeCanonicalEffectPayload[VerifierEffectResult](encoded, "verifier effect result")
	if err != nil {
		return VerifierEffectResult{}, err
	}
	if err := validateVerifierEffectResult(result); err != nil {
		return VerifierEffectResult{}, err
	}
	return result, nil
}

// ValidateEffectResult binds exact journal bytes to their declared row kind.
// Content-addressed artifact closure remains a persistence/admission concern.
func ValidateEffectResult(kind EffectKind, effectID string, request, result json.RawMessage) error {
	if !ValidID(effectID) {
		return errors.New("invalid effect id")
	}
	switch kind {
	case EffectBuild:
		if _, err := ParseBuildEffectRequest(request); err != nil {
			return err
		}
		parsed, err := ParseBuildEffectResult(result)
		if err != nil {
			return err
		}
		if parsed.Builder.RunID != effectID {
			return errors.New("builder run id does not match effect id")
		}
		return nil
	case EffectLocalCheck:
		if _, err := ParseLocalCheckEffectRequest(request); err != nil {
			return err
		}
		_, err := ParseLocalCheckEffectResult(result)
		return err
	case EffectVerifier:
		requestValue, err := ParseVerifierEffectRequest(request)
		if err != nil {
			return err
		}
		resultValue, err := ParseVerifierEffectResult(result)
		if err != nil {
			return err
		}
		if requestValue.DispatchID != effectID || resultValue.DispatchID != requestValue.DispatchID ||
			resultValue.VerificationEpoch != requestValue.VerificationEpoch {
			return errors.New("verifier request or result does not match its effect, dispatch, and verification epoch")
		}
		return nil
	default:
		return fmt.Errorf("unsupported effect kind %q", kind)
	}
}

func validateBuildEffectResult(result BuildEffectResult) error {
	if result.SchemaVersion != BuildEffectResultSchemaVersion || result.Outcome != BuildOutcomeCandidateReady {
		return errors.New("invalid build effect result schema or outcome")
	}
	if !protocol.ValidID(result.Builder.RunID) || !protocol.ValidNonEmpty(result.Builder.Agent) ||
		!protocol.ValidDateTime(result.Builder.StartedAt) || !protocol.ValidDateTime(result.Builder.CompletedAt) {
		return errors.New("invalid build effect builder")
	}
	order, err := protocol.CompareDateTimes(result.Builder.StartedAt, result.Builder.CompletedAt)
	if err != nil || order > 0 {
		return errors.New("invalid build effect builder timestamps")
	}
	return validateCandidate(result.Candidate)
}

func validateBuildAttemptIdentity(identity BuildAttemptIdentity) error {
	expected, err := BuildAttemptIdentityFor(
		identity.EffectID, identity.EffectAttempt, identity.BuilderDispatchDigest,
	)
	if err != nil || identity != expected {
		return errors.New("invalid build attempt identity")
	}
	return nil
}

func validateCheckAttemptIdentity(identity CheckAttemptIdentity) error {
	expected, err := CheckAttemptIdentityFor(
		identity.EffectID, identity.EffectAttempt, identity.RuntimeManifestDigest,
	)
	if err != nil || identity != expected {
		return errors.New("invalid check attempt identity")
	}
	return nil
}

func validateVerifierAttemptIdentity(identity VerifierAttemptIdentity) error {
	expected, err := VerifierAttemptIdentityFor(
		identity.EffectID, identity.EffectAttempt, identity.DispatchID, identity.DispatchDigest,
		identity.VerifierProfileDigest, identity.Agent, identity.VerificationEpoch,
	)
	if err != nil || identity != expected {
		return errors.New("invalid verifier attempt identity")
	}
	return nil
}

func validateLocalCheckEffectRequest(request LocalCheckEffectRequest) error {
	if request.SchemaVersion != LocalCheckEffectRequestSchemaVersion ||
		!ValidID(request.DeliveryRunID) || !ValidID(request.DeliveryID) || !ValidID(request.WorkID) ||
		!protocol.ValidPositiveSafeInteger(request.WorkAttempt) || !ValidID(request.BuilderEffectID) ||
		!ValidID(request.CheckID) || !ValidDigest(request.DefinitionDigest) ||
		!ValidDigest(request.RuntimeManifestDigest) {
		return errors.New("invalid local check effect request")
	}
	return nil
}

func validateLocalCheckEffectResult(result LocalCheckEffectResult) error {
	if result.SchemaVersion != LocalCheckEffectResultSchemaVersion ||
		(result.Outcome != LocalCheckOutcomePass && result.Outcome != LocalCheckOutcomeNotAdmitted &&
			result.Outcome != LocalCheckOutcomeControlled) {
		return errors.New("invalid local check effect result schema or outcome")
	}
	if !protocol.ValidNonEmpty(result.Receipt.Ref) || result.Receipt.MediaType != localCheckReceiptMediaType ||
		!protocol.ValidDigest(result.Receipt.Digest) {
		return errors.New("invalid local check receipt pointer")
	}
	return nil
}

func validateVerifierEffectRequest(request VerifierEffectRequest) error {
	if request.SchemaVersion != VerifierEffectRequestSchemaVersion ||
		!ValidID(request.DeliveryRunID) || !ValidID(request.DeliveryID) || !ValidID(request.WorkID) ||
		!protocol.ValidPositiveSafeInteger(request.WorkAttempt) || !ValidDigest(request.PlanDigest) ||
		!ValidID(request.SubmissionID) || !ValidDigest(request.SubmissionDigest) ||
		!ValidID(request.DispatchID) || !ValidDigest(request.VerifierProfileDigest) ||
		!protocol.ValidNonEmpty(request.Agent) || !validVerificationEpoch(request.VerificationEpoch) {
		return errors.New("invalid verifier effect request")
	}
	if err := validateVerifierCandidatePoint(request.Candidate); err != nil {
		return err
	}
	if err := validateVerifierArtifact(request.DispatchReceipt, "dispatch receipt"); err != nil {
		return err
	}
	return nil
}

func validateVerifierEffectResult(result VerifierEffectResult) error {
	if result.SchemaVersion != VerifierEffectResultSchemaVersion ||
		result.Outcome != VerifierOutcomeAssessmentReady || !ValidID(result.DispatchID) ||
		!validVerificationEpoch(result.VerificationEpoch) {
		return errors.New("invalid verifier effect result schema, outcome, or dispatch")
	}
	if err := validateVerifierArtifact(result.Assessment, "assessment"); err != nil {
		return err
	}
	if !protocol.ValidNonEmpty(result.ExecutionReceipt.Ref) ||
		result.ExecutionReceipt.MediaType != verifierExecutionReceiptMediaType ||
		!protocol.ValidDigest(result.ExecutionReceipt.Digest) {
		return errors.New("invalid verifier execution receipt pointer")
	}
	if !protocol.ValidDateTime(result.StartedAt) || !protocol.ValidDateTime(result.CompletedAt) {
		return errors.New("invalid verifier effect timestamps")
	}
	order, err := protocol.CompareDateTimes(result.StartedAt, result.CompletedAt)
	if err != nil || order > 0 {
		return errors.New("invalid verifier effect timestamp order")
	}
	return nil
}

func validateVerifierCandidatePoint(candidate protocol.CandidatePoint) error {
	if !protocol.ValidNonEmpty(candidate.Repository) || !objectIDPattern.MatchString(candidate.Commit) ||
		!objectIDPattern.MatchString(candidate.Tree) || len(candidate.Commit) != len(candidate.Tree) {
		return errors.New("invalid verifier candidate")
	}
	return nil
}

func validateVerifierArtifact(artifact protocol.Artifact, label string) error {
	if !protocol.ValidNonEmpty(artifact.Ref) || artifact.MediaType != verifierArtifactMediaType ||
		!protocol.ValidDigest(artifact.Digest) {
		return fmt.Errorf("invalid verifier %s pointer", label)
	}
	return nil
}

func validateCandidate(candidate repo.Candidate) error {
	if !protocol.ValidID(candidate.RepositoryID) || !protocol.ValidBranchRef(candidate.TargetRef) {
		return errors.New("invalid candidate repository or target")
	}
	objectLength := len(candidate.Commit)
	for _, objectID := range []string{candidate.BaseCommit, candidate.BaseTree, candidate.Commit, candidate.Tree} {
		if !objectIDPattern.MatchString(objectID) || len(objectID) != objectLength {
			return errors.New("invalid candidate object id")
		}
	}
	if candidate.Ref != "refs/sworn/v1/candidates/"+candidate.Commit || candidate.ChangedPaths == nil ||
		!slices.IsSorted(candidate.ChangedPaths) {
		return errors.New("invalid candidate retention or changed paths")
	}
	seen := make(map[string]struct{}, len(candidate.ChangedPaths))
	for _, path := range candidate.ChangedPaths {
		if !validCandidatePath(path) {
			return fmt.Errorf("invalid candidate changed path %q", path)
		}
		if _, exists := seen[path]; exists {
			return errors.New("duplicate candidate changed path")
		}
		seen[path] = struct{}{}
	}
	unchanged := candidate.Commit == candidate.BaseCommit
	if unchanged != (candidate.Tree == candidate.BaseTree) || unchanged != (len(candidate.ChangedPaths) == 0) {
		return errors.New("candidate change shape is inconsistent")
	}
	return nil
}

func validCandidatePath(path string) bool {
	if path == "" || path == "." || !utf8.ValidString(path) || strings.HasPrefix(path, "/") ||
		strings.HasSuffix(path, "/") || strings.Contains(path, "\x00") || strings.Contains(path, "//") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func validateStrictJSON(encoded json.RawMessage) error {
	if len(encoded) > MaximumEffectPayloadBytes {
		return errors.New("effect payload exceeds byte ceiling")
	}
	_, err := protocol.CanonicalizeJSON(encoded)
	return err
}

func encodeCanonicalEffectPayload(value any, label string) (json.RawMessage, error) {
	encoded, err := protocol.EncodeCanonical(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", label, err)
	}
	if len(encoded) > MaximumEffectPayloadBytes {
		return nil, fmt.Errorf("%s exceeds byte ceiling", label)
	}
	return json.RawMessage(encoded), nil
}

func decodeCanonicalEffectPayload[T any](encoded json.RawMessage, label string) (T, error) {
	var zero T
	if len(encoded) > MaximumEffectPayloadBytes {
		return zero, fmt.Errorf("%s exceeds byte ceiling", label)
	}
	canonical, err := protocol.CanonicalizeJSON(encoded)
	if err != nil {
		return zero, fmt.Errorf("decode %s: %w", label, err)
	}
	if !bytes.Equal(encoded, canonical) {
		return zero, fmt.Errorf("%s is not canonical JSON", label)
	}
	value, err := decodePayload[T](encoded)
	if err != nil {
		return zero, fmt.Errorf("decode %s: %w", label, err)
	}
	return value, nil
}
