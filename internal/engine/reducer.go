package engine

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/protocol"
)

// ErrAdmissionFactsRequired tells the store that a valid, current admission
// intent needs its transaction-derived submission binding. It is never a
// durable domain rejection and ordinary Reduce never exposes reviewable state.
var ErrAdmissionFactsRequired = errors.New("submission admission requires store-derived facts")

// Reduce is pure: it performs no I/O, does not observe time, and never mutates
// current. Deterministic command failures return Rejection.
func Reduce(current *State, command Command) (Decision, error) {
	return reduce(current, command, nil)
}

// ReduceAdmission supplies transaction-derived facts to the same pure reducer.
func ReduceAdmission(current *State, command Command, facts AdmissionFacts) (Decision, error) {
	return reduce(current, command, &facts)
}

func reduce(current *State, command Command, facts *AdmissionFacts) (Decision, error) {
	if !ValidID(command.ID) || !ValidID(command.RunID) {
		return Decision{}, reject("invalid_command", "invalid command or run id")
	}
	if facts != nil && command.Kind != CommandAdmitSubmission {
		return Decision{}, reject("unsupported_command", fmt.Sprintf("unsupported admission command kind %q", command.Kind))
	}
	if command.Kind == CommandCreate {
		return create(current, command)
	}
	if current == nil {
		return Decision{}, reject("run_not_found", "run does not exist")
	}
	if err := current.Validate(); err != nil {
		return Decision{}, fmt.Errorf("invalid current state: %w", err)
	}
	if current.RunID != command.RunID {
		return Decision{}, reject("run_mismatch", "command run does not match current state")
	}
	if command.ExpectedRevision != current.Revision {
		return Decision{}, reject(
			"revision_mismatch",
			fmt.Sprintf("expected revision %d, current revision is %d", command.ExpectedRevision, current.Revision),
		)
	}

	switch command.Kind {
	case CommandActivate:
		return activate(*current, command)
	case CommandDispatchBuild:
		return dispatchBuild(*current, command)
	case CommandDispatchChecks:
		return dispatchChecks(*current, command)
	case CommandAdmitSubmission:
		return admitSubmission(*current, command, facts)
	default:
		return Decision{}, reject("unsupported_command", fmt.Sprintf("unsupported command kind %q", command.Kind))
	}
}

func create(current *State, command Command) (Decision, error) {
	if current != nil {
		return Decision{}, reject("run_exists", "run already exists")
	}
	if command.ExpectedRevision != NoRevision {
		return Decision{}, reject("revision_mismatch", "delivery.create requires expected revision -1")
	}
	payload, err := decodePayload[CreatePayload](command.Payload)
	if err != nil {
		return Decision{}, reject("invalid_payload", err.Error())
	}
	if !ValidID(payload.DeliveryID) || !ValidDigest(payload.PlanDigest) ||
		payload.Repository == "" || len(payload.Repository) > 512 ||
		len(payload.TargetRef) <= len("refs/heads/") || len(payload.TargetRef) > 512 ||
		payload.TargetRef[:len("refs/heads/")] != "refs/heads/" || len(payload.Work) == 0 {
		return Decision{}, reject("invalid_payload", "create payload has invalid identity, digest, target, or work")
	}
	state := State{
		SchemaVersion: StateSchemaVersion,
		RunID:         command.RunID,
		DeliveryID:    payload.DeliveryID,
		PlanDigest:    payload.PlanDigest,
		Repository:    payload.Repository,
		TargetRef:     payload.TargetRef,
		Revision:      0,
		Phase:         PhasePlanned,
		Work:          make([]Work, len(payload.Work)),
	}
	for index, id := range payload.Work {
		state.Work[index] = Work{ID: id, State: WorkWaiting, NextAction: ActionWait}
	}
	if err := state.Validate(); err != nil {
		return Decision{}, reject("invalid_payload", err.Error())
	}
	return Decision{
		State: state,
		Event: Event{Kind: "delivery.planned", Data: cloneJSON(command.Payload)},
	}, nil
}

func activate(current State, command Command) (Decision, error) {
	if current.Phase != PhasePlanned {
		return Decision{}, reject("invalid_transition", "only a planned delivery can activate")
	}
	payload, err := decodePayload[ActivatePayload](command.Payload)
	if err != nil || !ValidDigest(payload.AuthorityReceiptDigest) {
		return Decision{}, reject("invalid_payload", "activate requires an authority receipt digest")
	}
	next := cloneState(current)
	next.Revision++
	next.Phase = PhaseActive
	next.AuthorityReceiptDigest = payload.AuthorityReceiptDigest
	next.Work[0].State = WorkReady
	next.Work[0].NextAction = ActionBuild
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	return Decision{
		State: next,
		Event: Event{Kind: "delivery.activated", Data: cloneJSON(command.Payload)},
	}, nil
}

func dispatchBuild(current State, command Command) (Decision, error) {
	if current.Phase != PhaseActive {
		return Decision{}, reject("invalid_transition", "only an active delivery can dispatch work")
	}
	payload, err := decodePayload[DispatchBuildPayload](command.Payload)
	if err != nil || !ValidID(payload.WorkID) || !ValidDigest(payload.DispatchDigest) ||
		(payload.BuilderDispatchDigest != "" && !ValidDigest(payload.BuilderDispatchDigest)) {
		return Decision{}, reject("invalid_payload", "build dispatch requires valid work and dispatch identities")
	}
	next := cloneState(current)
	work := workByID(next.Work, payload.WorkID)
	if work == nil {
		return Decision{}, reject("work_not_found", "work is not part of this delivery")
	}
	if work.State != WorkReady {
		return Decision{}, reject("invalid_transition", "work is not ready to build")
	}
	nextAttempt := work.Attempt + 1
	if nextAttempt <= work.Attempt || !protocol.ValidPositiveSafeInteger(nextAttempt) {
		return Decision{}, reject("invalid_transition", "work attempt exceeds the interoperable integer ceiling")
	}
	work.State, work.Attempt, work.NextAction = WorkActive, nextAttempt, ActionWait
	next.Revision++
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	schema := LegacyBuildEffectRequestSchemaVersion
	if payload.BuilderDispatchDigest != "" {
		schema = BuildEffectRequestSchemaVersion
	}
	request, err := json.Marshal(BuildEffectRequest{
		SchemaVersion:         schema,
		DeliveryRunID:         current.RunID,
		DeliveryID:            current.DeliveryID,
		WorkID:                payload.WorkID,
		WorkAttempt:           work.Attempt,
		DispatchDigest:        payload.DispatchDigest,
		BuilderDispatchDigest: payload.BuilderDispatchDigest,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("encode build effect: %w", err)
	}
	return Decision{
		State:   next,
		Event:   Event{Kind: "build.dispatched", Data: cloneJSON(command.Payload)},
		Effects: []Effect{{Kind: EffectBuild, Request: request}},
	}, nil
}

func dispatchChecks(current State, command Command) (Decision, error) {
	if current.Phase != PhaseActive {
		return Decision{}, reject("invalid_transition", "only an active delivery can dispatch checks")
	}
	if err := validateStrictJSON(command.Payload); err != nil {
		return Decision{}, reject("invalid_payload", err.Error())
	}
	payload, err := decodePayload[DispatchChecksPayload](command.Payload)
	if err != nil || !ValidID(payload.WorkID) || !ValidID(payload.BuilderEffectID) ||
		!ValidDigest(payload.RuntimeManifestDigest) || len(payload.Checks) == 0 ||
		len(payload.Checks) > MaximumCheckFanout {
		return Decision{}, reject("invalid_payload", "check dispatch requires valid work, builder, runtime, and bounded checks")
	}
	next := cloneState(current)
	work := workByID(next.Work, payload.WorkID)
	if work == nil {
		return Decision{}, reject("work_not_found", "work is not part of this delivery")
	}
	if work.State != WorkActive {
		return Decision{}, reject("invalid_transition", "work is not active for check dispatch")
	}
	work.State, work.NextAction = WorkChecking, ActionWait
	next.Revision++
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	request := LocalCheckEffectRequest{
		SchemaVersion:         LocalCheckEffectRequestSchemaVersion,
		DeliveryRunID:         current.RunID,
		DeliveryID:            current.DeliveryID,
		WorkID:                payload.WorkID,
		WorkAttempt:           work.Attempt,
		BuilderEffectID:       payload.BuilderEffectID,
		RuntimeManifestDigest: payload.RuntimeManifestDigest,
	}
	effects := make([]Effect, len(payload.Checks))
	seen := make(map[string]struct{}, len(payload.Checks))
	for index, check := range payload.Checks {
		if _, exists := seen[check.CheckID]; exists {
			return Decision{}, reject("invalid_payload", "check dispatch contains duplicate check ids")
		}
		seen[check.CheckID] = struct{}{}
		request.CheckID, request.DefinitionDigest = check.CheckID, check.DefinitionDigest
		encoded, err := EncodeLocalCheckEffectRequest(request)
		if err != nil {
			return Decision{}, reject("invalid_payload", fmt.Sprintf("invalid check selection %d: %v", index, err))
		}
		effects[index] = Effect{Kind: EffectLocalCheck, Request: encoded}
	}
	return Decision{
		State:   next,
		Event:   Event{Kind: "checks.dispatched", Data: cloneJSON(command.Payload)},
		Effects: effects,
	}, nil
}

func admitSubmission(current State, command Command, facts *AdmissionFacts) (Decision, error) {
	if current.Phase != PhaseActive {
		return Decision{}, reject("invalid_transition", "only an active delivery can admit a submission")
	}
	if err := validateStrictJSON(command.Payload); err != nil {
		return Decision{}, reject("invalid_payload", err.Error())
	}
	payload, err := decodePayload[AdmitSubmissionPayload](command.Payload)
	if err != nil || !ValidID(payload.WorkID) {
		return Decision{}, reject("invalid_payload", "submission admission requires a valid work id")
	}
	work := workByID(current.Work, payload.WorkID)
	if work == nil {
		return Decision{}, reject("work_not_found", "work is not part of this delivery")
	}
	if work.State != WorkChecking {
		return Decision{}, reject("invalid_transition", "work is not checking for submission admission")
	}
	if facts == nil {
		return Decision{}, ErrAdmissionFactsRequired
	}
	if !ValidID(facts.SubmissionID) || !ValidDigest(facts.SubmissionDigest) ||
		!objectIDPattern.MatchString(facts.CandidateCommit) {
		return Decision{}, errors.New("invalid store-derived admission facts")
	}
	next := cloneState(current)
	work = workByID(next.Work, payload.WorkID)
	work.State, work.SubmissionBinding, work.NextAction = WorkReviewable, *facts, ActionVerify
	next.Revision++
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	eventData, err := json.Marshal(struct {
		AdmitSubmissionPayload
		SubmissionBinding
	}{payload, *facts})
	if err != nil {
		return Decision{}, fmt.Errorf("encode submission admission event: %w", err)
	}
	return Decision{
		State: next,
		Event: Event{Kind: "submission.admitted", Data: eventData},
	}, nil
}

func reject(code, message string) error {
	return &Rejection{Code: code, Message: message}
}

func cloneState(current State) State {
	next := current
	next.Work = append([]Work(nil), current.Work...)
	return next
}

func workByID(work []Work, id string) *Work {
	for index := range work {
		if work[index].ID == id {
			return &work[index]
		}
	}
	return nil
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
