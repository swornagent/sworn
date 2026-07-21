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

// ErrVerifierDispatchFactsRequired tells the store that a valid dispatch
// intent needs an exact plan, submission, dispatch, and verifier closure.
var ErrVerifierDispatchFactsRequired = errors.New("verifier dispatch requires store-derived facts")

// ErrVerdictAdmissionFactsRequired tells the store that a valid admission
// intent needs an exact durable verdict and its current dispatch closure.
var ErrVerdictAdmissionFactsRequired = errors.New("verdict admission requires store-derived facts")

// ErrDeliveryAttentionFactsRequired tells a controlled Store path to supply a
// stable engine attention message. Raw command payloads never carry it.
var ErrDeliveryAttentionFactsRequired = errors.New("delivery attention requires store-derived facts")

const (
	specificationBlockAttention    = "verifier reported a contract or authority block"
	verificationExhaustedAttention = "verification retry ceiling reached"
)

type reductionFacts struct {
	submissionAdmission *AdmissionFacts
	verifierDispatch    *VerifierDispatchFacts
	verdictAdmission    *VerdictAdmissionFacts
	deliveryAttention   *DeliveryAttentionFacts
}

// Reduce is pure: it performs no I/O, does not observe time, and never mutates
// current. Deterministic command failures return Rejection.
func Reduce(current *State, command Command) (Decision, error) {
	return reduce(current, command, reductionFacts{})
}

// ReduceAdmission supplies transaction-derived facts to the same pure reducer.
func ReduceAdmission(current *State, command Command, facts AdmissionFacts) (Decision, error) {
	return reduce(current, command, reductionFacts{submissionAdmission: &facts})
}

// ReduceVerifierDispatch supplies the exact transaction-derived closure for a
// verifier-dispatch intent to the same pure reducer.
func ReduceVerifierDispatch(
	current *State,
	command Command,
	facts VerifierDispatchFacts,
) (Decision, error) {
	return reduce(current, command, reductionFacts{verifierDispatch: &facts})
}

// ReduceVerdictAdmission supplies exact durable verdict facts to the same pure
// reducer. Authority-sensitive admission remains a Store boundary concern.
func ReduceVerdictAdmission(
	current *State,
	command Command,
	facts VerdictAdmissionFacts,
) (Decision, error) {
	return reduce(current, command, reductionFacts{verdictAdmission: &facts})
}

// ReduceDeliveryAttention supplies one stable Store-derived control message.
// It preserves every work/submission/verdict binding and emits no effect.
func ReduceDeliveryAttention(
	current *State,
	command Command,
	facts DeliveryAttentionFacts,
) (Decision, error) {
	return reduce(current, command, reductionFacts{deliveryAttention: &facts})
}

func reduce(current *State, command Command, facts reductionFacts) (Decision, error) {
	if !ValidID(command.ID) || !ValidID(command.RunID) {
		return Decision{}, reject("invalid_command", "invalid command or run id")
	}
	providedFacts := 0
	if facts.submissionAdmission != nil {
		providedFacts++
	}
	if facts.verifierDispatch != nil {
		providedFacts++
	}
	if facts.verdictAdmission != nil {
		providedFacts++
	}
	if facts.deliveryAttention != nil {
		providedFacts++
	}
	if providedFacts > 1 ||
		(facts.submissionAdmission != nil && command.Kind != CommandAdmitSubmission) ||
		(facts.verifierDispatch != nil && command.Kind != CommandDispatchVerifier) ||
		(facts.verdictAdmission != nil && command.Kind != CommandAdmitVerdict) ||
		(facts.deliveryAttention != nil && command.Kind != CommandRaiseDeliveryAttention) {
		return Decision{}, reject("unsupported_command", fmt.Sprintf("unsupported facts for command kind %q", command.Kind))
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
		return admitSubmission(*current, command, facts.submissionAdmission)
	case CommandDispatchVerifier:
		return dispatchVerifier(*current, command, facts.verifierDispatch)
	case CommandAdmitVerdict:
		return admitVerdict(*current, command, facts.verdictAdmission)
	case CommandRaiseDeliveryAttention:
		return raiseDeliveryAttention(*current, command, facts.deliveryAttention)
	default:
		return Decision{}, reject("unsupported_command", fmt.Sprintf("unsupported command kind %q", command.Kind))
	}
}

func raiseDeliveryAttention(
	current State,
	command Command,
	facts *DeliveryAttentionFacts,
) (Decision, error) {
	if current.Phase != PhaseActive {
		return Decision{}, reject("invalid_transition", "only an active delivery can raise attention")
	}
	if err := validateStrictJSON(command.Payload); err != nil {
		return Decision{}, reject("invalid_payload", err.Error())
	}
	payload, err := decodePayload[RaiseDeliveryAttentionPayload](command.Payload)
	if err != nil || !ValidID(payload.WorkID) || (payload.Code != "" && !ValidID(payload.Code)) {
		return Decision{}, reject("invalid_payload", "delivery attention requires valid work and code identities")
	}
	if workByID(current.Work, payload.WorkID) == nil {
		return Decision{}, reject("work_not_found", "work is not part of this delivery")
	}
	if len(current.Attention) >= MaximumDeliveryAttentionEntries {
		return Decision{}, reject("attention_exhausted", "delivery attention ceiling reached")
	}
	if facts == nil {
		return Decision{}, ErrDeliveryAttentionFactsRequired
	}
	if !protocol.ValidNonEmpty(facts.Message) || len(facts.Message) > MaximumDeliveryAttentionMessageBytes {
		return Decision{}, errors.New("invalid store-derived delivery attention facts")
	}
	next := cloneState(current)
	next.Attention = append(next.Attention, facts.Message)
	next.Revision++
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	eventData, err := json.Marshal(struct {
		RaiseDeliveryAttentionPayload
		DeliveryAttentionFacts
	}{payload, *facts})
	if err != nil {
		return Decision{}, fmt.Errorf("encode delivery attention event: %w", err)
	}
	return Decision{
		State: next,
		Event: Event{Kind: "delivery.attention_raised", Data: eventData},
	}, nil
}

func dispatchVerifier(
	current State,
	command Command,
	facts *VerifierDispatchFacts,
) (Decision, error) {
	if current.Phase != PhaseActive {
		return Decision{}, reject("invalid_transition", "only an active delivery can dispatch a verifier")
	}
	if err := validateStrictJSON(command.Payload); err != nil {
		return Decision{}, reject("invalid_payload", err.Error())
	}
	payload, err := decodePayload[DispatchVerifierPayload](command.Payload)
	if err != nil || !ValidID(payload.WorkID) {
		return Decision{}, reject("invalid_payload", "verifier dispatch requires a valid work id")
	}
	work := workByID(current.Work, payload.WorkID)
	if work == nil {
		return Decision{}, reject("work_not_found", "work is not part of this delivery")
	}
	if work.State != WorkReviewable && work.State != WorkRetry {
		return Decision{}, reject("invalid_transition", "work is not reviewable or awaiting verification retry")
	}
	if work.VerificationEpoch >= MaximumVerificationEpoch {
		return Decision{}, reject("verification_exhausted", "fresh verification epoch ceiling reached")
	}
	if facts == nil {
		return Decision{}, ErrVerifierDispatchFactsRequired
	}
	if err := validateVerifierDispatchFacts(current, *work, *facts); err != nil {
		return Decision{}, err
	}
	nextEpoch := work.VerificationEpoch + 1
	if nextEpoch <= work.VerificationEpoch || !validVerificationEpoch(nextEpoch) ||
		facts.VerificationEpoch != nextEpoch {
		return Decision{}, errors.New("invalid store-derived verifier verification epoch")
	}
	request, err := EncodeVerifierEffectRequest(VerifierEffectRequest{
		SchemaVersion: VerifierEffectRequestSchemaVersion,
		DeliveryRunID: current.RunID, DeliveryID: current.DeliveryID,
		WorkID: work.ID, WorkAttempt: work.Attempt,
		PlanDigest: facts.PlanDigest, SubmissionID: facts.SubmissionID,
		SubmissionDigest: facts.SubmissionDigest, Candidate: facts.Candidate,
		DispatchID: facts.DispatchID, DispatchReceipt: facts.DispatchReceipt,
		VerifierProfileDigest: facts.VerifierProfileDigest, Agent: facts.Agent,
		VerificationEpoch: facts.VerificationEpoch,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("encode verifier effect: %w", err)
	}
	next := cloneState(current)
	work = workByID(next.Work, payload.WorkID)
	work.VerificationDispatchID = facts.DispatchID
	work.VerificationEpoch = facts.VerificationEpoch
	next.Revision++
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	eventData, err := json.Marshal(struct {
		DispatchVerifierPayload
		VerifierDispatchFacts
	}{payload, *facts})
	if err != nil {
		return Decision{}, fmt.Errorf("encode verifier dispatch event: %w", err)
	}
	return Decision{
		State:   next,
		Event:   Event{Kind: "verifier.dispatched", Data: eventData},
		Effects: []Effect{{Kind: EffectVerifier, Request: request}},
	}, nil
}

func admitVerdict(
	current State,
	command Command,
	facts *VerdictAdmissionFacts,
) (Decision, error) {
	if current.Phase != PhaseActive {
		return Decision{}, reject("invalid_transition", "only an active delivery can admit a verdict")
	}
	if err := validateStrictJSON(command.Payload); err != nil {
		return Decision{}, reject("invalid_payload", err.Error())
	}
	payload, err := decodePayload[AdmitVerdictPayload](command.Payload)
	if err != nil || !ValidID(payload.WorkID) {
		return Decision{}, reject("invalid_payload", "verdict admission requires a valid work id")
	}
	work := workByID(current.Work, payload.WorkID)
	if work == nil {
		return Decision{}, reject("work_not_found", "work is not part of this delivery")
	}
	if work.State != WorkReviewable && work.State != WorkRetry {
		return Decision{}, reject("invalid_transition", "work is not awaiting a verifier verdict")
	}
	if facts == nil {
		return Decision{}, ErrVerdictAdmissionFactsRequired
	}
	if !ValidID(facts.DispatchID) || facts.DispatchID != work.VerificationDispatchID ||
		!validVerificationEpoch(facts.VerificationEpoch) ||
		facts.VerificationEpoch != work.VerificationEpoch || work.VerdictEpoch >= work.VerificationEpoch ||
		!ValidID(facts.VerdictID) || !ValidDigest(facts.VerdictDigest) || !validVerdictOutcome(facts.Verdict) {
		return Decision{}, errors.New("invalid store-derived verdict admission facts")
	}
	next := cloneState(current)
	// A successfully admitted verifier verdict supersedes any delivery-level
	// authority/control latch raised while that verdict was pending. Outcome-
	// specific work attention is derived independently below.
	next.Attention = nil
	work = workByID(next.Work, payload.WorkID)
	work.VerdictBinding = facts.VerdictBinding
	work.VerdictEpoch = facts.VerificationEpoch
	work.Attention = ""
	switch facts.Verdict {
	case VerdictPass:
		work.State, work.NextAction = WorkVerified, ActionReplan
	case VerdictFail:
		work.State, work.NextAction = WorkRepair, ActionRepair
	case VerdictSpecBlock:
		work.State, work.NextAction, work.Attention = WorkBlocked, ActionReplan, specificationBlockAttention
	case VerdictInconclusive:
		if facts.VerificationEpoch == MaximumVerificationEpoch {
			work.State, work.NextAction, work.Attention = WorkAttention, ActionReplan, verificationExhaustedAttention
		} else {
			work.State, work.NextAction = WorkRetry, ActionRetryVerification
		}
	}
	next.Revision++
	if err := next.Validate(); err != nil {
		return Decision{}, fmt.Errorf("reducer produced invalid state: %w", err)
	}
	eventData, err := json.Marshal(struct {
		AdmitVerdictPayload
		VerdictAdmissionFacts
	}{payload, *facts})
	if err != nil {
		return Decision{}, fmt.Errorf("encode verdict admission event: %w", err)
	}
	return Decision{
		State: next,
		Event: Event{Kind: "verdict.admitted", Data: eventData},
	}, nil
}

func validateVerifierDispatchFacts(current State, work Work, facts VerifierDispatchFacts) error {
	if facts.PlanDigest != current.PlanDigest || facts.SubmissionID != work.SubmissionID ||
		facts.SubmissionDigest != work.SubmissionDigest || facts.Candidate.Repository != current.Repository ||
		facts.Candidate.Commit != work.CandidateCommit || !ValidID(facts.DispatchID) ||
		!ValidDigest(facts.VerifierProfileDigest) || !protocol.ValidNonEmpty(facts.Agent) {
		return errors.New("invalid store-derived verifier dispatch facts")
	}
	if err := validateVerifierCandidatePoint(facts.Candidate); err != nil {
		return fmt.Errorf("invalid store-derived verifier dispatch facts: %w", err)
	}
	if err := validateVerifierArtifact(facts.DispatchReceipt, "dispatch receipt"); err != nil {
		return fmt.Errorf("invalid store-derived verifier dispatch facts: %w", err)
	}
	return nil
}

func validVerdictOutcome(outcome VerdictOutcome) bool {
	return outcome == VerdictPass || outcome == VerdictFail ||
		outcome == VerdictSpecBlock || outcome == VerdictInconclusive
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
	next.Attention = append([]string(nil), current.Attention...)
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
