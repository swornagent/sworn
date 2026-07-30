package driver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	MaxContinuationSteps        = 32
	MaxCorrelationIDBytes       = 256
	MaxOpaqueFieldBytes         = 262_144
	MaxOpaqueStepBytes          = 524_288
	MaxOpaqueInvocationBytes    = 1_048_576
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
	if err := validateContinuationSource(invocation); err != nil {
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
			return failureObservation("adapter_failed"), nil, fresh, closeErr
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
	flow := continuationFlowW3
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
	if err := validateContinuationResume(invocation); err != nil {
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
	case cell.flow != continuationFlowW3:
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
		closeErr := cell.closeLocked()
		cell.mu.Unlock()
		return Observation{}, nil, freshContinuation(discardStatus), closeErr
	}
	state, mode := cell.state, cell.mode
	cell.zeroLocked()
	cell.mu.Unlock()

	var nextState continuationState
	var observation Observation
	var invokeErr error
	recoverableAdapter, retainsYield :=
		invocation.Selected.adapter.(recoverableContinuationAdapter)
	if retainsYield {
		observation, nextState, invokeErr =
			recoverableAdapter.resumeRecoverableContinuation(
				ctx,
				invocation,
				state,
				false,
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
		return failureObservation("adapter_failed"), nil, ContinuationResult{
			Mode: mode, Status: resultStatus,
		}, stateCloseErr
	}
	if IsCode(invokeErr, "CONTINUATION_INVALID") {
		_ = closeContinuationState(nextState)
		return Observation{}, nil,
			freshContinuation(ContinuationStatusMismatch), nil
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
	if closeErr := closeContinuationState(nextState); closeErr != nil {
		return failureObservation("adapter_failed"), nil, ContinuationResult{
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
	return ContinuationResult{
		Mode:   ContinuationModeFreshRehydrate,
		Status: status,
	}
}

func validateContinuationSource(invocation Invocation) error {
	return validateContinuationInvocation(
		invocation, ImplementerDesign, ReadOnly, true,
	)
}

func validateContinuationResume(invocation Invocation) error {
	return validateContinuationInvocation(
		invocation, ImplementerImplementation, ReadWrite, false,
	)
}

func validateContinuationInvocation(
	invocation Invocation,
	responsibility Responsibility,
	workspaceAccess WorkspaceAccess,
	fresh bool,
) error {
	if err := validateInvocation(invocation); err != nil {
		return err
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil ||
		invocation.Request.Role != RoleImplementer ||
		descriptor.Role != RoleImplementer ||
		descriptor.Responsibility != responsibility ||
		descriptor.WorkspaceAccess != workspaceAccess ||
		descriptor.FreshContext != fresh {
		return fail("CONTINUATION_INVALID")
	}
	return nil
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
		return empty, fail("CONTINUATION_INVALID")
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return empty, err
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
		return nil, fail("CONTINUATION_INVALID")
	}
	stepBytes := 0
	retained := make([][]byte, len(fields))
	for index, field := range fields {
		if len(field.body) > MaxOpaqueFieldBytes {
			clearRetained(retained)
			return nil, fail("CONTINUATION_INVALID")
		}
		switch field.kind {
		case opaqueText:
			if !validOpaqueText(field.body) {
				clearRetained(retained)
				return nil, fail("CONTINUATION_INVALID")
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
				return nil, fail("CONTINUATION_INVALID")
			}
			clearBytes(decoded)
		default:
			clearRetained(retained)
			return nil, fail("CONTINUATION_INVALID")
		}
		stepBytes += len(field.body)
		if stepBytes > MaxOpaqueStepBytes ||
			ledger.total > MaxOpaqueInvocationBytes-stepBytes {
			clearRetained(retained)
			return nil, fail("CONTINUATION_INVALID")
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
		return fail("CONTINUATION_INVALID")
	}
	if _, duplicate := ledger.ids[id]; duplicate {
		return fail("CONTINUATION_INVALID")
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
