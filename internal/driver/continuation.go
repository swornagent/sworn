package driver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// MaxContinuationSteps matches MaxProviderTurns: the ledger's byte
	// guards bound the resource; the step count must never end a
	// conversation the turn budget still allows.
	MaxContinuationSteps  = 1_000
	MaxCorrelationIDBytes = 256
	// MaxOpaqueFieldBytes must never bind before MaxProviderResponseBytes:
	// a reasoning model may legitimately think for hundreds of kilobytes
	// (observed: 279KB in one GLM-5.2 turn), and capping the field kills the
	// turn over content Sworn does not even retain.
	MaxOpaqueFieldBytes = 1_048_576
	// A step must hold at least one maximal field with room beside it, or
	// the reasoning the per-field budget exists to admit is refused one
	// level up.
	//
	// There is deliberately no cumulative per-invocation budget. Retained
	// opaque fields accumulate across every turn of a conversation, so any
	// fixed total is a limit on how long a model may think before the run
	// dies mid-delivery over content Sworn does not read. The previous
	// one-megabyte total, against a thousand permitted steps, allowed about
	// a kilobyte of reasoning per step: a high-effort model exhausted it
	// within a handful of turns and every later retain failed
	// CONTINUATION_INVALID, which parked three real releases. The real
	// bounds remain and are physical rather than arbitrary: a single field
	// and a single step are bounded above, the assembled request is bounded
	// by MaxProviderRequestBytes, and continuation state is bounded by
	// maxContinuationStateBytes. Runaway growth still fails, and it fails
	// where the resource actually is.
	MaxOpaqueStepBytes          = 8_388_608
	MaxDecodedOpaqueBinaryBytes = 196_608
	maxContinuationStateBytes   = 67_108_864
	maxContinuationLifetime     = 24 * time.Hour
)

type ContinuationMode string

const (
	ContinuationModeFreshRehydrate   ContinuationMode = "fresh_rehydrate"
	ContinuationModeTranscriptReplay ContinuationMode = "transcript_replay"
	ContinuationModeOpaqueReplay     ContinuationMode = "opaque_replay"
	ContinuationModeProviderCursor   ContinuationMode = "provider_cursor"
	ContinuationModeNativeSession    ContinuationMode = "native_session"
	ContinuationModeCompacted        ContinuationMode = "compacted"
)

type ContinuationStatus string

const (
	ContinuationStatusUnsupported ContinuationStatus = "unsupported"
	ContinuationStatusSuspended   ContinuationStatus = "suspended"
	ContinuationStatusResumed     ContinuationStatus = "resumed"
	ContinuationStatusCompleted   ContinuationStatus = "completed"
	ContinuationStatusClosed      ContinuationStatus = "closed"
	ContinuationStatusMismatch    ContinuationStatus = "mismatch"
	ContinuationStatusCancelled   ContinuationStatus = "cancelled"
	ContinuationStatusExpired     ContinuationStatus = "expired"
	ContinuationStatusOverflow    ContinuationStatus = "overflow"
)

// ContinuationBinding is authority already validated by the runtime. The
// driver treats each digest as opaque and binds it without parsing Baton, Git,
// journal, or work-context content.
type ContinuationBinding struct {
	RunID                 string
	Release               string
	Slice                 string
	Attempt               int64
	PlanAuthorityDigest   string
	TargetAuthorityDigest string
	ToolContractDigest    string
}

type continuationIdentity struct {
	Contract  string
	Binding   ContinuationBinding
	Operation struct {
		ID      string
		Version string
		Digest  string
	}
	Profile ProfileConfig
	Adapter AdapterIdentity
	Model   string
	Package PackageIdentity
}

// ContinuationResult exposes only closed lifecycle facts. It contains no
// content, identifier, size, error text, or adapter state.
type ContinuationResult struct {
	Mode   ContinuationMode
	Status ContinuationStatus
	Reason string
}

// continuationState contains only adapter-owned replay material. It must not
// retain permission, workspace, Baton decision, credential, or tool-session
// authority. The dispatcher can bound and destroy it but cannot inspect it.
type continuationState interface {
	continuationMode() ContinuationMode
	continuationBytes() int64
	closeContinuation() error
}

// Continuation is a process-local, pointer-owned, single-use handle. Its only
// public operation is idempotent destruction; it deliberately has no encoding,
// text, String, observation, or content method.
type Continuation struct {
	cell func() *continuationCell
}

type continuationFlow uint8

const (
	continuationFlowW3 continuationFlow = iota + 1
	continuationFlowVerifier
	continuationFlowRecoverable
)

type continuationCell struct {
	mu               sync.Mutex
	binding          [sha256.Size]byte
	sourceInvocation [sha256.Size]byte
	expiresNano      int64
	mode             ContinuationMode
	state            continuationState
	flow             continuationFlow
	sourceRole       Role
	sourceDuty       Responsibility
	sourceAccess     WorkspaceAccess
	sourceFresh      bool
	closed           bool
}

// Close atomically consumes the handle and destroys its adapter state.
func (continuation *Continuation) Close() error {
	cell := continuationCellFor(continuation)
	if cell == nil {
		return nil
	}
	cell.mu.Lock()
	defer cell.mu.Unlock()
	return cell.closeLocked()
}

// InvokeTurn uses the optional continuation contract without changing Invoke.
// A nil handle starts a fresh turn. An unusable non-nil handle returns a zero
// Observation with fresh_rehydrate; the caller must then construct and invoke a
// newly admitted fresh Invocation rather than reusing the rejected resume.
func (Dispatcher) InvokeTurn(
	ctx context.Context,
	invocation Invocation,
	binding ContinuationBinding,
	continuation *Continuation,
) (Observation, *Continuation, ContinuationResult, error) {
	if continuation != nil {
		return resumeContinuation(ctx, invocation, binding, continuation)
	}
	return startContinuation(ctx, invocation, binding)
}

func startContinuation(
	ctx context.Context,
	invocation Invocation,
	binding ContinuationBinding,
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
	flow, err := continuationSourceFlow(invocation)
	if err != nil {
		return invokeWithoutContinuation(
			ctx, invocation, ContinuationStatusUnsupported,
		)
	}
	fingerprint, err := continuationFingerprint(binding, invocation)
	if err != nil {
		return invokeWithoutContinuation(
			ctx, invocation, ContinuationStatusMismatch,
		)
	}
	adapter, supported := invocation.Selected.adapter.(continuationAdapter)
	if !supported {
		return invokeWithoutContinuation(
			ctx, invocation, ContinuationStatusUnsupported,
		)
	}
	if flow == continuationFlowVerifier {
		if _, supported := invocation.Selected.adapter.(recoverableContinuationAdapter); !supported {
			return invokeWithoutContinuation(
				ctx, invocation, ContinuationStatusUnsupported,
			)
		}
	}

	observation, state, invokeErr := adapter.invokeContinuation(
		ctx,
		invocation,
	)
	observation, invokeErr = finishAdapterInvocation(
		invocation,
		observation,
		invokeErr,
	)
	if invokeErr != nil {
		if closeErr := closeContinuationState(state); closeErr != nil {
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
	if state == nil {
		if observation.Yield != nil {
			fresh.Status = ContinuationStatusUnsupported
		} else {
			fresh.Status = ContinuationStatusCompleted
		}
		return observation, nil, fresh, nil
	}
	if observation.Yield != nil {
		flow = continuationFlowRecoverable
	}
	handle, result, retainErr := retainedContinuation(
		fingerprint, invocation, state, flow,
	)
	return observation, handle, result, retainErr
}

func resumeContinuation(
	ctx context.Context,
	invocation Invocation,
	binding ContinuationBinding,
	continuation *Continuation,
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
	targetFlow, err := continuationResumeFlow(invocation)
	if err != nil {
		return discardContinuation(
			cell, ContinuationStatusMismatch, nil,
		)
	}
	fingerprint, err := continuationFingerprint(binding, invocation)
	if err != nil {
		return discardContinuation(
			cell, ContinuationStatusMismatch, nil,
		)
	}
	adapter, supported := invocation.Selected.adapter.(continuationAdapter)
	if !supported {
		return discardContinuation(
			cell, ContinuationStatusMismatch, nil,
		)
	}
	recoverableAdapter, retainsState :=
		invocation.Selected.adapter.(recoverableContinuationAdapter)
	if targetFlow == continuationFlowVerifier && !retainsState {
		return discardContinuation(
			cell, ContinuationStatusMismatch, nil,
		)
	}

	targetInvocation := sha256.Sum256([]byte(invocation.Request.InvocationID))
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
	case cell.flow != targetFlow:
		discardStatus = ContinuationStatusMismatch
	case time.Now().UnixNano() >= cell.expiresNano:
		discardStatus = ContinuationStatusExpired
	case cell.binding != fingerprint ||
		cell.sourceInvocation == targetInvocation:
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

	var nextState continuationState
	var observation Observation
	var invokeErr error
	if retainsState {
		observation, nextState, invokeErr =
			recoverableAdapter.resumeRecoverableContinuation(
				ctx,
				invocation,
				state,
				targetFlow == continuationFlowVerifier,
			)
	} else {
		observation, invokeErr = adapter.resumeContinuation(
			ctx,
			invocation,
			state,
		)
	}
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
	resultStatus := ContinuationStatusResumed
	if isContinuationCancellation(invokeErr) {
		resultStatus = ContinuationStatusCancelled
	}
	if stateCloseErr != nil {
		_ = closeContinuationState(nextState)
		return failureObservation(
				"adapter_failed",
				invocation.Selected.Adapter.ID,
			), nil, ContinuationResult{
				Mode: mode, Status: resultStatus,
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
		if retainErr != nil {
			return observation, nil, result, retainErr
		}
		return observation, handle, result, nil
	}
	if invokeErr == nil && observation.Handoff != nil &&
		targetFlow == continuationFlowVerifier {
		handle, result, retainErr := retainedContinuation(
			fingerprint,
			invocation,
			nextState,
			continuationFlowVerifier,
		)
		return observation, handle, result, retainErr
	}
	if closeErr := closeContinuationState(nextState); closeErr != nil {
		return failureObservation(
				"adapter_failed",
				invocation.Selected.Adapter.ID,
			), nil, ContinuationResult{
				Mode: mode, Status: resultStatus,
			}, closeErr
	}
	return observation, nil, ContinuationResult{
		Mode:   mode,
		Status: resultStatus,
	}, invokeErr
}

func discardContinuation(
	cell *continuationCell,
	status ContinuationStatus,
	err error,
) (Observation, *Continuation, ContinuationResult, error) {
	observation := Observation{}
	if err == context.Canceled || err == context.DeadlineExceeded {
		observation, err = contextFailure(err)
	}
	if closeErr := closeContinuationCell(cell); closeErr != nil {
		err = closeErr
	}
	return observation, nil, freshContinuation(status), err
}

func invokeWithoutContinuation(
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

func freshContinuation(status ContinuationStatus) ContinuationResult {
	reason := ""
	switch status {
	case ContinuationStatusExpired:
		reason = "expiry"
	case ContinuationStatusClosed, ContinuationStatusMismatch, ContinuationStatusOverflow, ContinuationStatusUnsupported:
		reason = "absence"
	}
	return ContinuationResult{
		Mode:   ContinuationModeFreshRehydrate,
		Status: status,
		Reason: reason,
	}
}

func validateContinuationSource(invocation Invocation) error {
	_, err := continuationSourceFlow(invocation)
	return err
}

func validateContinuationResume(invocation Invocation) error {
	_, err := continuationResumeFlow(invocation)
	return err
}

func continuationSourceFlow(
	invocation Invocation,
) (continuationFlow, error) {
	return continuationFlowForInvocation(invocation, true)
}

func continuationResumeFlow(
	invocation Invocation,
) (continuationFlow, error) {
	return continuationFlowForInvocation(invocation, false)
}

func continuationFlowForInvocation(
	invocation Invocation,
	source bool,
) (continuationFlow, error) {
	if err := validateInvocation(invocation); err != nil {
		return 0, failContinuation("continuation.flow.invalid_invocation")
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil || invocation.Request.Role != descriptor.Role {
		return 0, failContinuation("continuation.flow.role_descriptor_mismatch")
	}
	if descriptor.Role == RoleVerifier &&
		descriptor.Responsibility == WorkVerification &&
		descriptor.WorkspaceAccess == ReadOnly &&
		descriptor.FreshContext == source {
		return continuationFlowVerifier, nil
	}
	duty, access := ImplementerImplementation, ReadWrite
	if source {
		duty, access = ImplementerDesign, ReadOnly
	}
	if descriptor.Role == RoleImplementer &&
		descriptor.Responsibility == duty &&
		descriptor.WorkspaceAccess == access &&
		descriptor.FreshContext == source {
		return continuationFlowW3, nil
	}
	return 0, failContinuation("continuation.flow.unsupported_permission_combination")
}

func continuationFingerprint(
	binding ContinuationBinding,
	invocation Invocation,
) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	if validateIdentity(binding.RunID) != nil ||
		validateIdentity(binding.Release) != nil ||
		validateIdentity(binding.Slice) != nil ||
		binding.Attempt < 1 ||
		binding.Attempt > MaxSafeInteger ||
		!digestPattern.MatchString(binding.PlanAuthorityDigest) ||
		!digestPattern.MatchString(binding.TargetAuthorityDigest) ||
		!digestPattern.MatchString(binding.ToolContractDigest) {
		return empty, failContinuation("continuation.fingerprint.binding_invalid")
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return empty, err
	}
	// A verifier thread follows the same slice across direct repair attempts.
	// The runtime still admits each exact attempt; the driver binds the stable
	// thread authority only.
	if descriptor.Role == RoleVerifier &&
		descriptor.Responsibility == WorkVerification {
		binding.Attempt = 1
	}
	identity := continuationIdentity{
		Contract: "sworn.continuation-binding/v1",
		Binding:  binding,
		Profile:  invocation.Selected.Profile,
		Adapter:  invocation.Selected.Adapter,
		Model:    invocation.Selected.Model,
		Package:  descriptor.Package,
	}
	identity.Operation.ID = invocation.Request.Operation.ID
	identity.Operation.Version = invocation.Request.Operation.Version
	identity.Operation.Digest = invocation.Request.Operation.Digest
	body, err := canonicalJSON(identity)
	if err != nil {
		return empty, err
	}
	fingerprint := sha256.Sum256(body)
	clearBytes(body)
	return fingerprint, nil
}

func closeContinuationCell(cell *continuationCell) error {
	if cell == nil {
		return nil
	}
	cell.mu.Lock()
	defer cell.mu.Unlock()
	return cell.closeLocked()
}

func continuationCellFor(continuation *Continuation) *continuationCell {
	if continuation == nil || continuation.cell == nil {
		return nil
	}
	return continuation.cell()
}

func (cell *continuationCell) closeLocked() error {
	if cell == nil || cell.closed {
		return nil
	}
	state := cell.state
	cell.zeroLocked()
	return closeContinuationState(state)
}

func (cell *continuationCell) zeroLocked() {
	cell.binding = [sha256.Size]byte{}
	cell.sourceInvocation = [sha256.Size]byte{}
	cell.expiresNano = 0
	cell.mode = ""
	cell.state = nil
	cell.flow = 0
	cell.sourceRole = ""
	cell.sourceDuty = ""
	cell.sourceAccess = ""
	cell.sourceFresh = false
	cell.closed = true
}

func retainedContinuation(
	fingerprint [sha256.Size]byte,
	invocation Invocation,
	state continuationState,
	flow continuationFlow,
) (*Continuation, ContinuationResult, error) {
	if state == nil {
		return nil, freshContinuation(ContinuationStatusUnsupported), nil
	}
	mode, size := state.continuationMode(), state.continuationBytes()
	if !mode.validRetained() || size < 1 || size > maxContinuationStateBytes {
		closeErr := closeContinuationState(state)
		result := freshContinuation(ContinuationStatusOverflow)
		if closeErr != nil {
			return nil, result, closeErr
		}
		return nil, result, nil
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		_ = closeContinuationState(state)
		return nil, freshContinuation(ContinuationStatusMismatch), err
	}
	cell := &continuationCell{
		binding:          fingerprint,
		sourceInvocation: sha256.Sum256([]byte(invocation.Request.InvocationID)),
		expiresNano:      time.Now().Add(maxContinuationLifetime).UnixNano(),
		mode:             mode,
		state:            state,
		flow:             flow,
		sourceRole:       descriptor.Role,
		sourceDuty:       descriptor.Responsibility,
		sourceAccess:     descriptor.WorkspaceAccess,
		sourceFresh:      descriptor.FreshContext,
	}
	handle := &Continuation{cell: func() *continuationCell { return cell }}
	return handle, ContinuationResult{
		Mode: mode, Status: ContinuationStatusSuspended,
	}, nil
}

func closeContinuationState(state continuationState) error {
	if state == nil {
		return nil
	}
	if err := state.closeContinuation(); err != nil {
		return fail("CONTINUATION_CLEANUP_FAILED")
	}
	return nil
}

func (mode ContinuationMode) validRetained() bool {
	switch mode {
	case ContinuationModeTranscriptReplay,
		ContinuationModeOpaqueReplay,
		ContinuationModeProviderCursor,
		ContinuationModeNativeSession,
		ContinuationModeCompacted:
		return true
	default:
		return false
	}
}

func isContinuationCancellation(err error) bool {
	return isContextError(err) ||
		IsCode(err, "INVOCATION_CANCELLED") ||
		IsCode(err, "INVOCATION_TIMEOUT")
}

type opaqueKind uint8

const (
	opaqueText opaqueKind = iota + 1
	opaqueBase64
)

type opaqueField struct {
	kind opaqueKind
	body []byte
}

// continuationLedger owns provider-required replay bytes for exactly one
// invocation. It has no encoding or observation method by design.
type continuationLedger struct {
	steps  int
	total  int
	ids    map[string]struct{}
	fields [][]byte
	closed bool
}

func newContinuationLedger() *continuationLedger {
	return &continuationLedger{ids: make(map[string]struct{})}
}

func (ledger *continuationLedger) retain(fields ...opaqueField) ([][]byte, error) {
	if ledger == nil || ledger.closed || len(fields) == 0 ||
		ledger.steps >= MaxContinuationSteps {
		return nil, failContinuation("continuation.ledger.step_budget_exhausted")
	}
	stepBytes := 0
	retained := make([][]byte, len(fields))
	for index, field := range fields {
		if len(field.body) > MaxOpaqueFieldBytes {
			clearRetained(retained)
			return nil, failContinuation("continuation.ledger.field_bytes_exhausted")
		}
		switch field.kind {
		case opaqueText:
			if !validOpaqueText(field.body) {
				clearRetained(retained)
				return nil, failContinuation("continuation.ledger.invalid_opaque_text")
			}
		case opaqueBase64:
			decoded := make([]byte, base64.StdEncoding.DecodedLen(len(field.body)))
			count, err := base64.StdEncoding.Strict().Decode(decoded, field.body)
			if err != nil || count > MaxDecodedOpaqueBinaryBytes ||
				!bytes.Equal(
					[]byte(base64.StdEncoding.EncodeToString(decoded[:count])),
					field.body,
				) {
				clearBytes(decoded)
				clearRetained(retained)
				return nil, failContinuation("continuation.ledger.invalid_opaque_base64")
			}
			clearBytes(decoded)
		default:
			clearRetained(retained)
			return nil, failContinuation("continuation.ledger.unknown_opaque_kind")
		}
		stepBytes += len(field.body)
		if stepBytes > MaxOpaqueStepBytes {
			clearRetained(retained)
			return nil, failContinuation("continuation.ledger.step_bytes_exhausted")
		}
		retained[index] = append([]byte(nil), field.body...)
	}
	ledger.steps++
	ledger.total += stepBytes
	ledger.fields = append(ledger.fields, retained...)
	return retained, nil
}

func (ledger *continuationLedger) correlate(id string) error {
	if ledger == nil || ledger.closed ||
		validateText(id, MaxCorrelationIDBytes, false) != nil {
		return failContinuation("continuation.ledger.correlate_invalid_id")
	}
	if _, duplicate := ledger.ids[id]; duplicate {
		return failContinuation("continuation.ledger.correlate_duplicate_id")
	}
	ledger.ids[id] = struct{}{}
	return nil
}

func (ledger *continuationLedger) Close() {
	if ledger == nil || ledger.closed {
		return
	}
	ledger.closed = true
	clearRetained(ledger.fields)
	ledger.fields = nil
	clear(ledger.ids)
	ledger.total = 0
	ledger.steps = 0
}

func validOpaqueText(body []byte) bool {
	if !utf8.Valid(body) {
		return false
	}
	for _, character := range string(body) {
		if character == 0 || (character < 0x20 &&
			character != '\n' && character != '\r' && character != '\t') ||
			(character >= 0x7f && character <= 0x9f) {
			return false
		}
	}
	return true
}

func clearRetained(fields [][]byte) {
	for _, field := range fields {
		clearBytes(field)
	}
}
