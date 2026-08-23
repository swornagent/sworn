package driver

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	RecoverableTurnInputSchemaVersion = "sworn.recoverable-turn-input/v1"
	MaxRecoverableInputBytes          = 16 * 1024
	recoverableTurnNudge              = "Continue the same responsibility from your last yielded turn. Use the available evidence; yield again only for a real unresolved question or block."
)

type RecoveryStepKind string

const (
	RecoveryStepSubmissionCorrection RecoveryStepKind = "submission_correction"
	RecoveryStepProseNudge           RecoveryStepKind = "prose_nudge"
)

// RecoveryStepHook lets the runtime durably reserve one bounded automatic
// action before the driver emits it. It carries no content or identity; the
// runtime closure owns those bindings and the journal transaction.
type RecoveryStepHook func(context.Context, RecoveryStepKind) error

type RecoverableInputKind string

const (
	RecoverableInputAnswer RecoverableInputKind = "answer"
	RecoverableInputNudge  RecoverableInputKind = "nudge"
)

type RecoverableTurnInput struct {
	SchemaVersion string               `json:"schema_version"`
	Kind          RecoverableInputKind `json:"kind"`
	Answer        string               `json:"answer,omitempty"`
	// TargetBinding is runtime-only authority. It never reaches an adapter;
	// an exact recovered terminal handoff may use it to retain the same
	// private state for its admitted continuation flow.
	TargetBinding *ContinuationBinding `json:"-"`
}

func ValidateRecoverableTurnInput(value RecoverableTurnInput) error {
	if value.SchemaVersion != RecoverableTurnInputSchemaVersion {
		return fail("INVALID_RECOVERABLE_INPUT")
	}
	switch value.Kind {
	case RecoverableInputAnswer:
		if !validRecoverableInputText(value.Answer) {
			return fail("INVALID_RECOVERABLE_INPUT")
		}
	case RecoverableInputNudge:
		if value.Answer != "" {
			return fail("INVALID_RECOVERABLE_INPUT")
		}
	default:
		return fail("INVALID_RECOVERABLE_INPUT")
	}
	return nil
}

func validRecoverableInputText(value string) bool {
	return utf8.ValidString(value) &&
		len([]byte(value)) <= MaxRecoverableInputBytes &&
		strings.TrimSpace(value) != "" &&
		!strings.ContainsRune(value, '\x00') &&
		!strings.ContainsRune(value, '\r')
}

func reserveRecoveryStep(
	ctx context.Context,
	hook RecoveryStepHook,
	kind RecoveryStepKind,
) error {
	if ctx == nil || hook == nil ||
		(kind != RecoveryStepSubmissionCorrection &&
			kind != RecoveryStepProseNudge) {
		return fail("RECOVERY_STEP_REFUSED")
	}
	if err := hook(ctx, kind); err != nil {
		return fail("RECOVERY_STEP_REFUSED")
	}
	return nil
}

// InvokeRecoverableTurn has three admitted shapes:
//   - nil handle and nil input starts a worker turn;
//   - handle and input resumes that exact yielded worker;
//   - nil handle and input explicitly rehydrates a lost or expired worker.
func (Dispatcher) InvokeRecoverableTurn(
	ctx context.Context,
	invocation Invocation,
	binding ContinuationBinding,
	continuation *Continuation,
	input *RecoverableTurnInput,
) (Observation, *Continuation, ContinuationResult, error) {
	var target *ContinuationBinding
	if input != nil && input.TargetBinding != nil {
		value := *input.TargetBinding
		target = &value
	}
	if continuation != nil {
		return resumeRecoverableTurn(
			ctx, invocation, binding, continuation, input, target,
		)
	}
	if target != nil {
		return Observation{}, nil,
			freshContinuation(ContinuationStatusMismatch),
			failContinuation("continuation.recoverable.fresh_turn_with_resume_target")
	}
	return startRecoverableTurn(
		ctx, invocation, binding, input, input != nil,
	)
}

func startRecoverableTurn(
	ctx context.Context,
	invocation Invocation,
	binding ContinuationBinding,
	input *RecoverableTurnInput,
	rehydrated bool,
) (Observation, *Continuation, ContinuationResult, error) {
	fresh := freshContinuation(ContinuationStatusUnsupported)
	if ctx == nil {
		return Observation{}, nil, fresh, fail("INVALID_CONTEXT")
	}
	if err := ctx.Err(); err != nil {
		observation, contextErr := contextFailure(err)
		fresh.Status = ContinuationStatusCancelled
		return observation, nil, fresh, contextErr
	}
	if invocation.Request.Role == RoleVerifier &&
		!invocation.Request.FreshContext {
		return Observation{}, nil, fresh, fail("INVALID_VERIFIER")
	}
	if err := prepareRecoverableInvocation(&invocation, input); err != nil {
		return Observation{}, nil, fresh, err
	}
	fingerprint, err := continuationFingerprint(binding, invocation)
	if err != nil {
		return invokeWithoutRecoverableContinuation(
			ctx, invocation, ContinuationStatusMismatch,
		)
	}
	adapter, supported :=
		invocation.Selected.adapter.(recoverableContinuationAdapter)
	if !supported {
		return invokeWithoutRecoverableContinuation(
			ctx, invocation, ContinuationStatusUnsupported,
		)
	}

	observation, state, invokeErr := adapter.invokeRecoverableContinuation(
		ctx,
		invocation,
	)
	observation, invokeErr = finishAdapterInvocation(
		invocation,
		observation,
		invokeErr,
	)
	if invokeErr != nil {
		closeErr := closeContinuationState(state)
		if closeErr != nil {
			return failureObservation(
				"adapter_failed",
				invocation.Selected.Adapter.ID,
			), nil, fresh, closeErr
		}
		if isContinuationCancellation(invokeErr) {
			fresh.Status = ContinuationStatusCancelled
		} else {
			fresh.Status = ContinuationStatusCompleted
		}
		return observation, nil, fresh, invokeErr
	}
	if observation.Yield == nil {
		closeErr := closeContinuationState(state)
		fresh.Status = ContinuationStatusCompleted
		if closeErr != nil {
			return failureObservation(
				"adapter_failed",
				invocation.Selected.Adapter.ID,
			), nil, fresh, closeErr
		}
		return observation, nil, fresh, nil
	}
	handle, result, retainErr := retainedContinuation(
		fingerprint,
		invocation,
		state,
		continuationFlowRecoverable,
	)
	if rehydrated {
		result.Mode = ContinuationModeFreshRehydrate
	}
	return observation, handle, result, retainErr
}

func resumeRecoverableTurn(
	ctx context.Context,
	invocation Invocation,
	binding ContinuationBinding,
	continuation *Continuation,
	input *RecoverableTurnInput,
	promotionBinding *ContinuationBinding,
) (Observation, *Continuation, ContinuationResult, error) {
	fresh := freshContinuation(ContinuationStatusClosed)
	cell := continuationCellFor(continuation)
	if cell == nil {
		return Observation{}, nil, fresh, nil
	}
	if ctx == nil {
		return discardContinuation(
			cell, ContinuationStatusMismatch, fail("INVALID_CONTEXT"),
		)
	}
	if err := ctx.Err(); err != nil {
		return discardContinuation(
			cell, ContinuationStatusCancelled, err,
		)
	}
	if input == nil {
		return discardContinuation(
			cell,
			ContinuationStatusMismatch,
			fail("INVALID_RECOVERABLE_INPUT"),
		)
	}
	if err := prepareRecoverableInvocation(&invocation, input); err != nil {
		return discardContinuation(
			cell, ContinuationStatusMismatch, err,
		)
	}
	var promotionFingerprint [sha256.Size]byte
	var promotionFlow continuationFlow
	if promotionBinding != nil {
		var promotionErr error
		promotionFlow, promotionErr = continuationFlowForInvocation(
			invocation,
			invocation.Request.Role != RoleVerifier,
		)
		if promotionErr != nil ||
			!sameContinuationScope(
				binding,
				*promotionBinding,
				promotionFlow == continuationFlowVerifier,
			) {
			return discardContinuation(
				cell, ContinuationStatusMismatch, nil,
			)
		}
		promotionFingerprint, promotionErr =
			continuationFingerprint(*promotionBinding, invocation)
		if promotionErr != nil {
			return discardContinuation(
				cell, ContinuationStatusMismatch, nil,
			)
		}
	}
	fingerprint, err := continuationFingerprint(binding, invocation)
	if err != nil {
		return discardContinuation(
			cell, ContinuationStatusMismatch, nil,
		)
	}
	adapter, supported :=
		invocation.Selected.adapter.(recoverableContinuationAdapter)
	if !supported {
		return discardContinuation(
			cell, ContinuationStatusMismatch, nil,
		)
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return discardContinuation(
			cell, ContinuationStatusMismatch, nil,
		)
	}

	cell.mu.Lock()
	stateBytes := int64(0)
	if cell.state != nil {
		stateBytes = cell.state.continuationBytes()
	}
	var discardStatus ContinuationStatus
	switch {
	case cell.closed || cell.state == nil:
		cell.mu.Unlock()
		return Observation{}, nil, fresh, nil
	case cell.flow != continuationFlowRecoverable ||
		!validRecoverableContinuationTransition(cell, descriptor):
		discardStatus = ContinuationStatusMismatch
	case time.Now().UnixNano() >= cell.expiresNano:
		discardStatus = ContinuationStatusExpired
	case cell.binding != fingerprint:
		discardStatus = ContinuationStatusMismatch
	case !cell.mode.validRetained() ||
		stateBytes < 1 ||
		stateBytes > maxContinuationStateBytes:
		discardStatus = ContinuationStatusOverflow
	}
	if discardStatus != "" {
		reason := "absence"
		if discardStatus == ContinuationStatusExpired {
			reason = "expiry"
		}
		closeErr := cell.closeLocked()
		cell.mu.Unlock()
		res := freshContinuation(discardStatus)
		res.Reason = reason
		return Observation{}, nil, res, closeErr
	}
	state, mode := cell.state, cell.mode
	cell.zeroLocked()
	cell.mu.Unlock()

	observation, nextState, invokeErr :=
		adapter.resumeRecoverableContinuation(
			ctx,
			invocation,
			state,
			promotionBinding != nil,
		)
	observation, invokeErr = finishAdapterInvocation(
		invocation,
		observation,
		invokeErr,
	)
	stateCloseErr := closeContinuationState(state)
	if invokeErr != nil {
		_ = closeContinuationState(nextState)
		nextState = nil
	}
	status := ContinuationStatusResumed
	if isContinuationCancellation(invokeErr) {
		status = ContinuationStatusCancelled
	}
	if stateCloseErr != nil {
		_ = closeContinuationState(nextState)
		return failureObservation(
				"adapter_failed",
				invocation.Selected.Adapter.ID,
			), nil, ContinuationResult{
				Mode: mode, Status: status,
			}, stateCloseErr
	}
	if IsCode(invokeErr, "CONTINUATION_INVALID") {
		_ = closeContinuationState(nextState)
		reason := ""
		var contractErr *ContractError
		if errors.As(invokeErr, &contractErr) && contractErr.Detail != "" {
			reason = contractErr.Detail
		}
		if reason == "" {
			reason = "absence"
		}
		res := freshContinuation(ContinuationStatusMismatch)
		res.Reason = reason
		return Observation{}, nil, res, nil
	}
	if invokeErr == nil && observation.Yield != nil {
		handle, result, retainErr := retainedContinuation(
			fingerprint,
			invocation,
			nextState,
			continuationFlowRecoverable,
		)
		return observation, handle, result, retainErr
	}
	if invokeErr == nil && observation.Handoff != nil &&
		promotionBinding != nil {
		handle, result, retainErr := retainedContinuation(
			promotionFingerprint,
			invocation,
			nextState,
			promotionFlow,
		)
		return observation, handle, result, retainErr
	}
	if closeErr := closeContinuationState(nextState); closeErr != nil {
		return failureObservation(
				"adapter_failed",
				invocation.Selected.Adapter.ID,
			), nil, ContinuationResult{
				Mode: mode, Status: status,
			}, closeErr
	}
	return observation, nil, ContinuationResult{
		Mode: mode, Status: status,
	}, invokeErr
}

func validRecoverableContinuationTransition(
	cell *continuationCell,
	target PermissionDescriptor,
) bool {
	if cell == nil ||
		cell.sourceRole != target.Role ||
		cell.sourceDuty != target.Responsibility ||
		cell.sourceAccess != target.WorkspaceAccess {
		return false
	}
	if target.Role == RoleVerifier &&
		target.Responsibility == WorkVerification {
		return !target.FreshContext
	}
	return cell.sourceFresh == target.FreshContext
}

func sameContinuationScope(
	source ContinuationBinding,
	target ContinuationBinding,
	allowAttemptChange bool,
) bool {
	return source.RunID == target.RunID &&
		source.Release == target.Release &&
		source.Slice == target.Slice &&
		(allowAttemptChange || source.Attempt == target.Attempt) &&
		source.PlanAuthorityDigest == target.PlanAuthorityDigest &&
		source.TargetAuthorityDigest == target.TargetAuthorityDigest
}

func invokeWithoutRecoverableContinuation(
	ctx context.Context,
	invocation Invocation,
	status ContinuationStatus,
) (Observation, *Continuation, ContinuationResult, error) {
	observation, err := (Dispatcher{}).Invoke(ctx, invocation)
	if isContinuationCancellation(err) {
		status = ContinuationStatusCancelled
	}
	return observation, nil, freshContinuation(status), err
}

func prepareRecoverableInvocation(
	invocation *Invocation,
	input *RecoverableTurnInput,
) error {
	if invocation == nil || invocation.recoverableInput != nil {
		return fail("INVALID_RECOVERABLE_INPUT")
	}
	if err := validateInvocation(*invocation); err != nil {
		return err
	}
	if input == nil {
		return nil
	}
	if err := ValidateRecoverableTurnInput(*input); err != nil {
		return err
	}
	copy := *input
	copy.TargetBinding = nil
	invocation.recoverableInput = &copy
	return nil
}
