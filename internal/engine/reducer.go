package engine

import (
	"encoding/json"
	"fmt"
)

// Reduce is pure: it performs no I/O, does not observe time, and never mutates
// current. Deterministic command failures return Rejection.
func Reduce(current *State, command Command) (Decision, error) {
	if !ValidID(command.ID) || !ValidID(command.RunID) {
		return Decision{}, reject("invalid_command", "invalid command or run id")
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
	if err != nil || !ValidID(payload.WorkID) || !ValidDigest(payload.DispatchDigest) {
		return Decision{}, reject("invalid_payload", "build dispatch requires valid work and dispatch identities")
	}
	next := cloneState(current)
	found := false
	var workAttempt int64
	for index := range next.Work {
		if next.Work[index].ID != payload.WorkID {
			continue
		}
		found = true
		if next.Work[index].State != WorkReady {
			return Decision{}, reject("invalid_transition", "work is not ready to build")
		}
		next.Work[index].State = WorkActive
		next.Work[index].Attempt++
		next.Work[index].NextAction = ActionWait
		workAttempt = next.Work[index].Attempt
	}
	if !found {
		return Decision{}, reject("work_not_found", "work is not part of this delivery")
	}
	next.Revision++
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	request, err := json.Marshal(BuildEffectRequest{
		SchemaVersion:  BuildEffectRequestSchemaVersion,
		DeliveryRunID:  current.RunID,
		DeliveryID:     current.DeliveryID,
		WorkID:         payload.WorkID,
		WorkAttempt:    workAttempt,
		DispatchDigest: payload.DispatchDigest,
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

func reject(code, message string) error {
	return &Rejection{Code: code, Message: message}
}

func cloneState(current State) State {
	next := current
	next.Work = append([]Work(nil), current.Work...)
	return next
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
