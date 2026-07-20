// Package engine owns Sworn's pure delivery state transitions.
package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/swornagent/sworn/internal/protocol"
)

const (
	StateSchemaVersion                    = "sworn-engine-state-v1"
	BuildEffectRequestSchemaVersion       = "sworn-build-effect-request-v2"
	LegacyBuildEffectRequestSchemaVersion = "sworn-build-effect-request-v1"
	BuildEffectResultSchemaVersion        = "sworn-build-effect-result-v1"
	BuildAttemptIdentitySchemaVersion     = "sworn-build-attempt-identity-v1"
	LocalCheckEffectRequestSchemaVersion  = "sworn-local-check-effect-request-v1"
	LocalCheckEffectResultSchemaVersion   = "sworn-local-check-effect-result-v1"
	MaximumEffectPayloadBytes             = 1 << 20
	MaximumCheckFanout                    = protocol.MaximumExactLocalChecks
	NoRevision                            = int64(-1)
)

// BuildAttemptIdentity is the durable, engine-derived identity of one claim.
// It is recorded with the claim before any builder process may start. The
// stable effect ID remains the Baton builder run ID; InvocationID separates
// executor and workspace ownership across retries.
type BuildAttemptIdentity struct {
	SchemaVersion         string `json:"schema_version"`
	EffectID              string `json:"effect_id"`
	EffectAttempt         int64  `json:"effect_attempt"`
	InvocationID          string `json:"invocation_id"`
	BuilderDispatchDigest string `json:"builder_dispatch_digest"`
}

func BuildAttemptIdentityFor(effectID string, attempt int64, builderDispatchDigest string) (BuildAttemptIdentity, error) {
	if !ValidID(effectID) || !protocol.ValidPositiveSafeInteger(attempt) || !ValidDigest(builderDispatchDigest) {
		return BuildAttemptIdentity{}, errors.New("invalid build attempt identity")
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("sworn-build-attempt-v1"))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(effectID))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(builderDispatchDigest))
	var encodedAttempt [8]byte
	binary.BigEndian.PutUint64(encodedAttempt[:], uint64(attempt))
	_, _ = hasher.Write(encodedAttempt[:])
	return BuildAttemptIdentity{
		SchemaVersion: BuildAttemptIdentitySchemaVersion,
		EffectID:      effectID, EffectAttempt: attempt,
		InvocationID:          "attempt-" + hex.EncodeToString(hasher.Sum(nil)),
		BuilderDispatchDigest: builderDispatchDigest,
	}, nil
}

type Phase string

const (
	PhasePlanned Phase = "planned"
	PhaseActive  Phase = "active"
)

type WorkState string

const (
	WorkWaiting    WorkState = "waiting"
	WorkReady      WorkState = "ready"
	WorkActive     WorkState = "active"
	WorkChecking   WorkState = "checking"
	WorkReviewable WorkState = "reviewable"
)

type NextAction string

const (
	ActionWait   NextAction = "wait"
	ActionBuild  NextAction = "build"
	ActionVerify NextAction = "verify"
)

type CommandKind string

const (
	CommandCreate          CommandKind = "delivery.create"
	CommandActivate        CommandKind = "delivery.activate"
	CommandDispatchBuild   CommandKind = "build.dispatch"
	CommandDispatchChecks  CommandKind = "checks.dispatch"
	CommandAdmitSubmission CommandKind = "submission.admit"
)

type EffectKind string

const (
	EffectBuild      EffectKind = "runner.build"
	EffectLocalCheck EffectKind = "check.local"
)

// Command is the immutable input to one reducer invocation. Payload bytes are
// bound exactly by the command idempotency digest at the store boundary.
type Command struct {
	ID               string          `json:"id"`
	RunID            string          `json:"run_id"`
	Kind             CommandKind     `json:"kind"`
	ExpectedRevision int64           `json:"expected_revision"`
	Payload          json.RawMessage `json:"payload"`
}

type CreatePayload struct {
	DeliveryID string   `json:"delivery_id"`
	PlanDigest string   `json:"plan_digest"`
	Repository string   `json:"repository_id"`
	TargetRef  string   `json:"target_ref"`
	Work       []string `json:"work"`
}

type ActivatePayload struct {
	AuthorityReceiptDigest string `json:"authority_receipt_digest"`
}

type DispatchBuildPayload struct {
	WorkID                string `json:"work_id"`
	DispatchDigest        string `json:"dispatch_digest"`
	BuilderDispatchDigest string `json:"builder_dispatch_digest,omitempty"`
}

type CheckSelection struct {
	CheckID          string `json:"check_id"`
	DefinitionDigest string `json:"definition_digest"`
}

// DispatchChecksPayload is the exact ordered check selection prepared from the
// delivery plan at the store boundary. Work attempt and effect identities remain
// reducer- and store-derived facts respectively.
type DispatchChecksPayload struct {
	WorkID                string           `json:"work_id"`
	BuilderEffectID       string           `json:"builder_effect_id"`
	RuntimeManifestDigest string           `json:"runtime_manifest_digest"`
	Checks                []CheckSelection `json:"checks"`
}

// AdmitSubmissionPayload expresses only the caller's intent. The store derives
// and revalidates the immutable submission binding before ReduceAdmission may
// expose reviewable state.
type AdmitSubmissionPayload struct {
	WorkID string `json:"work_id"`
}

// SubmissionBinding is derived by the store, never accepted in command input.
type SubmissionBinding struct {
	SubmissionID     string `json:"submission_id,omitempty"`
	SubmissionDigest string `json:"submission_digest,omitempty"`
	CandidateCommit  string `json:"candidate_commit,omitempty"`
}

type AdmissionFacts = SubmissionBinding

// BuildEffectRequest is the strict engine-owned input for one builder effect.
// Its delivery run ID is control-state identity, not the Baton builder run ID:
// the store-derived effect ID becomes that invocation identity when claimed.
type BuildEffectRequest struct {
	SchemaVersion         string `json:"schema_version"`
	DeliveryRunID         string `json:"delivery_run_id"`
	DeliveryID            string `json:"delivery_id"`
	WorkID                string `json:"work_id"`
	WorkAttempt           int64  `json:"work_attempt"`
	DispatchDigest        string `json:"dispatch_digest"`
	BuilderDispatchDigest string `json:"builder_dispatch_digest,omitempty"`
}

func ParseBuildEffectRequest(encoded json.RawMessage) (BuildEffectRequest, error) {
	if err := validateStrictJSON(encoded); err != nil {
		return BuildEffectRequest{}, fmt.Errorf("decode build effect request: %w", err)
	}
	request, err := decodePayload[BuildEffectRequest](encoded)
	if err != nil {
		return BuildEffectRequest{}, fmt.Errorf("decode build effect request: %w", err)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return BuildEffectRequest{}, fmt.Errorf("inspect build effect request members: %w", err)
	}
	_, hasBuilderDispatch := members["builder_dispatch_digest"]
	validSchema := request.SchemaVersion == BuildEffectRequestSchemaVersion &&
		hasBuilderDispatch && ValidDigest(request.BuilderDispatchDigest)
	legacySchema := request.SchemaVersion == LegacyBuildEffectRequestSchemaVersion &&
		!hasBuilderDispatch && request.BuilderDispatchDigest == ""
	if (!validSchema && !legacySchema) ||
		!ValidID(request.DeliveryRunID) || !ValidID(request.DeliveryID) || !ValidID(request.WorkID) ||
		!protocol.ValidPositiveSafeInteger(request.WorkAttempt) || !ValidDigest(request.DispatchDigest) {
		return BuildEffectRequest{}, errors.New("invalid build effect request")
	}
	return request, nil
}

type Work struct {
	ID      string    `json:"id"`
	State   WorkState `json:"state"`
	Attempt int64     `json:"attempt"`
	SubmissionBinding
	NextAction NextAction `json:"next_action"`
}

// State is the current snapshot derived from immutable events. It is persisted
// for fast reads but changes only through the reducers.
type State struct {
	SchemaVersion          string `json:"schema_version"`
	RunID                  string `json:"run_id"`
	DeliveryID             string `json:"delivery_id"`
	PlanDigest             string `json:"plan_digest"`
	Repository             string `json:"repository_id"`
	TargetRef              string `json:"target_ref"`
	Revision               int64  `json:"revision"`
	Phase                  Phase  `json:"phase"`
	AuthorityReceiptDigest string `json:"authority_receipt_digest,omitempty"`
	Work                   []Work `json:"work"`
}

type Event struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type Effect struct {
	Kind    EffectKind      `json:"kind"`
	Request json.RawMessage `json:"request"`
}

type Decision struct {
	State   State
	Event   Event
	Effects []Effect
}

// Rejection is a deterministic command result, not an infrastructure error.
type Rejection struct {
	Code    string
	Message string
}

func (r *Rejection) Error() string { return r.Code + ": " + r.Message }

func RejectionOf(err error) (*Rejection, bool) {
	var rejection *Rejection
	return rejection, errors.As(err, &rejection)
}

func ValidID(value string) bool     { return protocol.ValidID(value) }
func ValidDigest(value string) bool { return protocol.ValidDigest(value) }

func (s State) Validate() error {
	if s.SchemaVersion != StateSchemaVersion {
		return fmt.Errorf("unknown state schema %q", s.SchemaVersion)
	}
	if !ValidID(s.RunID) || !ValidID(s.DeliveryID) {
		return errors.New("invalid run or delivery id")
	}
	if !ValidDigest(s.PlanDigest) {
		return errors.New("invalid plan digest")
	}
	if strings.TrimSpace(s.Repository) == "" || len(s.Repository) > 512 {
		return errors.New("invalid repository identity")
	}
	if !strings.HasPrefix(s.TargetRef, "refs/heads/") || len(s.TargetRef) > 512 {
		return errors.New("invalid target ref")
	}
	if s.Revision < 0 {
		return errors.New("negative state revision")
	}
	if s.Phase != PhasePlanned && s.Phase != PhaseActive {
		return fmt.Errorf("unsupported phase %q", s.Phase)
	}
	if s.Phase == PhasePlanned && s.AuthorityReceiptDigest != "" {
		return errors.New("planned state carries authority receipt")
	}
	if s.Phase == PhaseActive && !ValidDigest(s.AuthorityReceiptDigest) {
		return errors.New("active state lacks authority receipt digest")
	}
	if len(s.Work) == 0 {
		return errors.New("state has no work")
	}
	seen := make(map[string]struct{}, len(s.Work))
	activeWork := 0
	for _, work := range s.Work {
		if !ValidID(work.ID) {
			return fmt.Errorf("invalid work id %q", work.ID)
		}
		if _, ok := seen[work.ID]; ok {
			return fmt.Errorf("duplicate work id %q", work.ID)
		}
		seen[work.ID] = struct{}{}
		if work.Attempt < 0 || (work.Attempt > 0 && !protocol.ValidPositiveSafeInteger(work.Attempt)) {
			return fmt.Errorf("invalid attempt for work %q", work.ID)
		}
		hasSubmissionBinding := work.SubmissionBinding != (SubmissionBinding{})
		switch work.State {
		case WorkWaiting:
			if work.NextAction != ActionWait || hasSubmissionBinding {
				return fmt.Errorf("waiting work %q must wait", work.ID)
			}
		case WorkReady:
			activeWork++
			if work.NextAction != ActionBuild || hasSubmissionBinding {
				return fmt.Errorf("ready work %q must build", work.ID)
			}
		case WorkActive, WorkChecking:
			activeWork++
			if work.NextAction != ActionWait || work.Attempt == 0 || hasSubmissionBinding {
				return fmt.Errorf("running work %q has invalid attempt or action", work.ID)
			}
		case WorkReviewable:
			activeWork++
			if work.NextAction != ActionVerify || work.Attempt == 0 ||
				!ValidID(work.SubmissionID) || !ValidDigest(work.SubmissionDigest) ||
				!objectIDPattern.MatchString(work.CandidateCommit) {
				return fmt.Errorf("reviewable work %q lacks its exact submission binding", work.ID)
			}
		default:
			return fmt.Errorf("unsupported work state %q", work.State)
		}
		if s.Phase == PhasePlanned && work.State != WorkWaiting {
			return fmt.Errorf("planned delivery has non-waiting work %q", work.ID)
		}
	}
	if s.Phase == PhaseActive && activeWork != 1 {
		return fmt.Errorf("active delivery has %d current work items, want 1", activeWork)
	}
	return nil
}

func decodePayload[T any](payload json.RawMessage) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return value, fmt.Errorf("decode trailing payload: %w", err)
		}
		return value, errors.New("payload contains trailing JSON value")
	}
	return value, nil
}
