// Package engine owns Sworn's pure delivery state transitions.
package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	StateSchemaVersion = "sworn-engine-state-v1"
	NoRevision         = int64(-1)
)

type Phase string

const (
	PhasePlanned Phase = "planned"
	PhaseActive  Phase = "active"
)

type WorkState string

const (
	WorkWaiting WorkState = "waiting"
	WorkReady   WorkState = "ready"
	WorkActive  WorkState = "active"
)

type NextAction string

const (
	ActionWait  NextAction = "wait"
	ActionBuild NextAction = "build"
)

type CommandKind string

const (
	CommandCreate        CommandKind = "delivery.create"
	CommandActivate      CommandKind = "delivery.activate"
	CommandDispatchBuild CommandKind = "build.dispatch"
)

type EffectKind string

const EffectBuild EffectKind = "runner.build"

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
	WorkID         string `json:"work_id"`
	DispatchDigest string `json:"dispatch_digest"`
}

type Work struct {
	ID         string     `json:"id"`
	State      WorkState  `json:"state"`
	Attempt    int64      `json:"attempt"`
	NextAction NextAction `json:"next_action"`
}

// State is the current snapshot derived from immutable events. It is persisted
// for fast reads but changes only through Reduce.
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

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ValidID(value string) bool { return idPattern.MatchString(value) }

func ValidDigest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

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
	activeOrReady := 0
	for _, work := range s.Work {
		if !ValidID(work.ID) {
			return fmt.Errorf("invalid work id %q", work.ID)
		}
		if _, ok := seen[work.ID]; ok {
			return fmt.Errorf("duplicate work id %q", work.ID)
		}
		seen[work.ID] = struct{}{}
		if work.Attempt < 0 {
			return fmt.Errorf("negative attempt for work %q", work.ID)
		}
		switch work.State {
		case WorkWaiting:
			if work.NextAction != ActionWait {
				return fmt.Errorf("waiting work %q must wait", work.ID)
			}
		case WorkReady:
			activeOrReady++
			if work.NextAction != ActionBuild {
				return fmt.Errorf("ready work %q must build", work.ID)
			}
		case WorkActive:
			activeOrReady++
			if work.NextAction != ActionWait || work.Attempt == 0 {
				return fmt.Errorf("active work %q has invalid attempt or action", work.ID)
			}
		default:
			return fmt.Errorf("unsupported work state %q", work.State)
		}
		if s.Phase == PhasePlanned && work.State != WorkWaiting {
			return fmt.Errorf("planned delivery has non-waiting work %q", work.ID)
		}
	}
	if s.Phase == PhaseActive && activeOrReady != 1 {
		return fmt.Errorf("active delivery has %d runnable work items, want 1", activeOrReady)
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
