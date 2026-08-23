package driver

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/gitx"
)

const (
	MaxResultEnvelopeBytes = 16_384
	MaxStderrBytes         = 65_536
	MaxStderrRetain        = 1_024
)

// Dispatcher performs one adapter attempt without lifecycle, Git, or Baton
// receipt authority.
type Dispatcher struct{}

// Driver is the single role-neutral invocation shape used by every provider.
type Driver interface {
	Invoke(context.Context, Invocation) (Observation, error)
}

var _ Driver = Dispatcher{}

// ContinuationDriver is the additive, opt-in turn contract. Driver.Invoke
// remains the complete one-shot contract for every existing caller and
// adapter.
type ContinuationDriver interface {
	Driver
	InvokeTurn(
		context.Context,
		Invocation,
		ContinuationBinding,
		*Continuation,
	) (Observation, *Continuation, ContinuationResult, error)
}

var _ ContinuationDriver = Dispatcher{}

// ContinuationPostureDriver is the additive, opt-in declaration capability:
// it tells the degradation counter whether an adapter's fresh rehydration is
// ordinary operation (fresh_by_design) or lost context (context_retaining).
// Driver.Invoke and the turn contract are unchanged; a driver that does not
// declare a posture is read as context_retaining.
type ContinuationPostureDriver interface {
	Driver
	ContinuationPosture(Invocation) ContinuationPosture
}

var _ ContinuationPostureDriver = Dispatcher{}

// ContinuationPosture returns the selected adapter's declared continuation
// posture. It consults the private opt-in adapter capability and fails closed
// to context_retaining for adapters that declare nothing.
func (Dispatcher) ContinuationPosture(invocation Invocation) ContinuationPosture {
	if declaration, ok := invocation.Selected.adapter.(interface {
		declaredContinuationPosture() ContinuationPosture
	}); ok {
		return declaration.declaredContinuationPosture()
	}
	return ContinuationPostureContextRetaining
}

// RecoverableTurnDriver resumes only the exact yielded worker responsibility.
// A nil handle starts a fresh turn; a nil handle with input explicitly
// rehydrates a lost or expired turn without granting submission authority.
type RecoverableTurnDriver interface {
	Driver
	InvokeRecoverableTurn(
		context.Context,
		Invocation,
		ContinuationBinding,
		*Continuation,
		*RecoverableTurnInput,
	) (Observation, *Continuation, ContinuationResult, error)
}

var _ RecoverableTurnDriver = Dispatcher{}

// Invoker is retained as the role-neutral dispatcher name used by early W2
// callers; it does not imply a process-only adapter.
type Invoker = Dispatcher

// Invocation is the single role-neutral invocation shape used by every provider.
type Invocation struct {
	Request          Request
	HostWorkspace    string
	Selected         SelectedProfile
	Permission       SubmissionPermission
	Inputs           []InputContent
	FakeProfile      FakeProfile
	RecoveryStepHook RecoveryStepHook
	// ToolResultHook is the runtime-provided durable callback for the
	// bounded tool-result projection. It is runtime-only authority: the
	// driver emits on it through the observer pump and never blocks or
	// fails a dispatch on it. A nil hook disables observation entirely
	// (certification, capture, fake, and automation paths).
	ToolResultHook ToolResultHook
	// MaskNames are the workspace-relative names the containment mask must
	// always protect, derived by the engine from the configured project roots
	// (records and journals) plus .git. They are computed by the engine and
	// never model-configurable; an empty value means the driver uses the
	// fixed defaults. The mask follows the configured roots so a relocated
	// records or journals root is never left unprotected. The same set also
	// reaches request admission, the input projection and the tool path guard
	// so a configured root is never admitted as an input or tool path.
	MaskNames        []string
	recoverableInput *RecoverableTurnInput
}

// reservedMaskNames returns the workspace-relative names the containment
// mask protects: the engine-computed MaskNames when present (which follow the
// configured project roots), else the fixed defaults. It lives in a shared
// file because request admission, the input projection and the tool path
// guard read it on every platform, not just the Linux containment sites.
func reservedMaskNames(invocation Invocation) []string {
	if len(invocation.MaskNames) != 0 {
		return append([]string(nil), invocation.MaskNames...)
	}
	return gitx.ReservedNames(gitx.DefaultProjectConfig())
}

// withoutGit returns the reserved set with ".git" removed, for read-only
// verifier workspaces that expose read-only git instead of masking it.
func withoutGit(names []string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		if name != ".git" {
			result = append(result, name)
		}
	}
	return result
}

type Diagnostic struct {
	Code        string `json:"code"`
	StderrBytes int64  `json:"stderr_bytes"`
	Truncated   bool   `json:"truncated"`
}
type SealedHandoff struct {
	SubmissionBytes  []byte `json:"submission_bytes"`
	SubmissionDigest string `json:"submission_digest"`
	SealBytes        []byte `json:"seal_bytes"`
	SealDigest       string `json:"seal_digest"`
}
type TerminalEvent struct {
	Sequence uint64 `json:"sequence"`
	Kind     string `json:"kind"`
}
type Observation struct {
	TransportStatus TransportStatus `json:"transport_status"`
	DurationMillis  int64           `json:"duration_ms"`
	Usage           UsageReceipt    `json:"usage"`
	Diagnostic      Diagnostic      `json:"diagnostic"`
	Handoff         *SealedHandoff  `json:"handoff"`
	Yield           *Yield          `json:"yield,omitempty"`
	Events          []TerminalEvent `json:"events"`
}

func (Dispatcher) Invoke(ctx context.Context, invocation Invocation) (Observation, error) {
	if ctx == nil {
		return Observation{}, fail("INVALID_CONTEXT")
	}
	if err := ctx.Err(); err != nil {
		return contextFailure(err)
	}
	if invocation.Request.Role == RoleVerifier &&
		!invocation.Request.FreshContext {
		return Observation{}, fail("INVALID_VERIFIER")
	}
	if err := validateInvocation(invocation); err != nil {
		return Observation{}, err
	}
	observation, err := invocation.Selected.adapter.invoke(ctx, invocation)
	return finishAdapterInvocation(invocation, observation, err)
}

func finishAdapterInvocation(
	invocation Invocation,
	observation Observation,
	err error,
) (Observation, error) {
	surface := invocation.Selected.Adapter.ID
	if err != nil {
		// A transport or adapter failure can never carry a model decision.
		return sanitizeFailedObservation(observation, surface), normalizeAdapterError(err)
	}
	if err := validateObservation(invocation, observation); err != nil {
		observation.Handoff = nil
		if IsCode(err, "MISSING_SUBMISSION") {
			if observation.Diagnostic.Code == "none" {
				observation.Diagnostic.Code = "submission_absent"
			}
			return observation, err
		}
		if IsCode(err, "INVALID_HANDOFF") {
			return failureObservation("invalid_handoff", surface), err
		}
		return invalidObservation(surface), err
	}
	return observation, nil
}

func validateObservation(invocation Invocation, observation Observation) error {
	if observation.TransportStatus != Completed ||
		observation.DurationMillis < 0 ||
		observation.DurationMillis > MaxSafeInteger ||
		!validDiagnosticCode(observation.Diagnostic.Code) ||
		observation.Diagnostic.StderrBytes < 0 ||
		observation.Diagnostic.StderrBytes > MaxSafeInteger {
		return fail("INVALID_OBSERVATION")
	}
	if _, err := EncodeUsageReceipt(observation.Usage); err != nil {
		return err
	}
	if len(observation.Events) > 1_024 {
		return fail("RESOURCE_LIMIT")
	}
	for index, event := range observation.Events {
		if event.Sequence != uint64(index+1) ||
			!validTerminalEventKind(event.Kind) {
			return fail("INVALID_OBSERVATION")
		}
	}
	if observation.Handoff == nil && observation.Yield == nil {
		return fail("MISSING_SUBMISSION")
	}
	if observation.Handoff != nil && observation.Yield != nil {
		return fail("INVALID_OBSERVATION")
	}
	if observation.Diagnostic.Code != "none" {
		return fail("INVALID_OBSERVATION")
	}
	if observation.Yield != nil {
		if err := ValidateYield(*observation.Yield); err != nil ||
			observation.Yield.InvocationID != invocation.Request.InvocationID {
			return fail("INVALID_YIELD")
		}
		return nil
	}
	handoff := observation.Handoff
	if len(handoff.SubmissionBytes) == 0 ||
		handoff.SubmissionDigest != Digest(handoff.SubmissionBytes) ||
		len(handoff.SealBytes) == 0 ||
		handoff.SealDigest != Digest(handoff.SealBytes) {
		return fail("INVALID_HANDOFF")
	}
	submission, err := DecodeSubmission(handoff.SubmissionBytes)
	if err != nil {
		return fail("INVALID_HANDOFF")
	}
	if err := invocation.Permission.validate(submission); err != nil {
		return fail("INVALID_HANDOFF")
	}
	seal, err := DecodeSeal(handoff.SealBytes)
	if err != nil || !seal.Accepted || seal.Code != "accepted" ||
		seal.InvocationID != invocation.Request.InvocationID ||
		seal.SubmissionDigest != handoff.SubmissionDigest {
		return fail("INVALID_HANDOFF")
	}
	return nil
}

func validDiagnosticCode(code string) bool {
	switch code {
	case "none",
		"submission_rejected",
		"submission_absent",
		"provider_truncated",
		"stdout_overflow",
		"post_result_stdout",
		"extra_stdout",
		"invalid_driver_result",
		"driver_transport_failed",
		"invalid_usage",
		"late_submission",
		"submission_protocol_failed",
		"process_failed",
		"submit_without_engine_stop",
		"invocation_cancelled",
		"invocation_timeout",
		"publication_gate_failed",
		"submission_binding_failed",
		"stderr_overflow",
		"process_status_failed",
		"process_not_quiescent",
		"workspace_postcheck_failed",
		"input_cleanup_failed":
		return true
	default:
		return false
	}
}

func validFatalDiagnosticCode(code string) bool {
	switch code {
	case "stdout_overflow",
		"post_result_stdout",
		"extra_stdout",
		"invalid_driver_result",
		"driver_transport_failed",
		"provider_truncated",
		"invalid_usage",
		"late_submission",
		"submission_protocol_failed",
		"process_failed",
		"submit_without_engine_stop",
		"invocation_cancelled",
		"invocation_timeout",
		"publication_gate_failed",
		"submission_binding_failed",
		"stderr_overflow",
		"process_status_failed",
		"process_not_quiescent",
		"workspace_postcheck_failed",
		"input_cleanup_failed":
		return true
	default:
		return false
	}
}

func validTerminalEventKind(kind string) bool {
	switch kind {
	case "result_completed",
		"submit_accepted_pending",
		"submit_rejected_pending",
		"submit_acknowledged",
		"engine_stop_after_submit",
		"process_waited",
		"published",
		"completed_without_handoff",
		"process_group_quiescent",
		"workspace_postcheck",
		"input_projection_removed",
		"producers_joined",
		"yield_accepted",
		"engine_stop_after_yield",
		"credential_rotated":
		return true
	default:
		const fatalPrefix = "fatal:"
		return len(kind) > len(fatalPrefix) &&
			kind[:len(fatalPrefix)] == fatalPrefix &&
			validFatalDiagnosticCode(kind[len(fatalPrefix):])
	}
}

func invalidObservation(surface string) Observation {
	return failureObservation("invalid_observation", surface)
}

// failureObservation builds the A2 loud unavailable receipt: the surface
// (adapter id) and the stable capture-failed reason ride on the receipt so
// an attempt that genuinely cannot report never defaults silent. A surface
// that fails the adapter-identity bound is impossible from validated
// invocations; the receipt then falls back to the legacy shape rather than
// inventing a name.
func failureObservation(code string, surface string) Observation {
	usage := UsageReceipt{
		TokenStatus: UsageUnavailable,
		CostStatus:  UsageUnavailable,
	}
	if loud, err := UnavailableReceipt(surface, UsageReasonCaptureFailed); err == nil {
		usage = loud
	}
	return Observation{
		TransportStatus: RunnerError,
		Usage:           usage,
		Diagnostic:      Diagnostic{Code: code},
	}
}

func sanitizeFailedObservation(
	observation Observation,
	surface string,
) Observation {
	sanitized := failureObservation("adapter_failed", surface)
	if observation.DurationMillis >= 0 &&
		observation.DurationMillis <= MaxSafeInteger {
		sanitized.DurationMillis = observation.DurationMillis
	}
	if validFatalDiagnosticCode(observation.Diagnostic.Code) &&
		observation.Diagnostic.StderrBytes >= 0 &&
		observation.Diagnostic.StderrBytes <= MaxSafeInteger {
		sanitized.Diagnostic = observation.Diagnostic
	}
	// A provider-reported truncation is the one adapter failure that carries
	// measured facts: the accumulated receipt with the provider's own finish
	// reason and cache/effort accounting. Preserve it when it is canonical so
	// the operator surfaces can evaluate what the invocation actually cost.
	if observation.Diagnostic.Code == "provider_truncated" {
		if _, err := EncodeUsageReceipt(observation.Usage); err == nil {
			sanitized.Usage = observation.Usage
		}
	}
	if len(observation.Events) <= 1_024 {
		sanitized.Events = make([]TerminalEvent, 0, len(observation.Events))
		for index, event := range observation.Events {
			if event.Sequence != uint64(index+1) ||
				!validTerminalEventKind(event.Kind) {
				sanitized.Events = nil
				sanitized.Diagnostic = Diagnostic{Code: "adapter_failed"}
				break
			}
			sanitized.Events = append(sanitized.Events, event)
		}
	}
	return sanitized
}

// normalizeAdapterError maps adapter errors to the stable dispatcher
// vocabulary. Adapter-provided wrapping text cannot escape, with one bounded
// exception recorded by the S5-provider-limit-evidence ruling: the five
// provider status codes carry the status envelope's error.message as Detail
// after it passes validateText at maxProviderErrorDetailBytes, so a provider
// limit is diagnosable from the durable failure record and the live stream.
// Every other code, and any non-conforming detail, is dropped exactly as
// before.
func normalizeAdapterError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return fail("INVOCATION_CANCELLED")
	case errors.Is(err, context.DeadlineExceeded):
		return fail("INVOCATION_TIMEOUT")
	}
	var contractErr *ContractError
	if errors.As(err, &contractErr) && validAdapterErrorCode(contractErr.Code) {
		if providerStatusCode(contractErr.Code) &&
			validateText(contractErr.Detail, maxProviderErrorDetailBytes, false) == nil {
			// Bounded, re-validated provider words ride the stable code.
			return &ContractError{
				Code:      contractErr.Code,
				Detail:    contractErr.Detail,
				HardLimit: contractErr.HardLimit,
			}
		}
		// Recreate the error so adapter-provided wrapping text cannot escape.
		return fail(contractErr.Code)
	}
	return fail("ADAPTER_FAILURE")
}

func validAdapterErrorCode(code string) bool {
	switch code {
	case "INVALID_EXECUTABLE",
		"EXECUTABLE_IDENTITY_MISMATCH",
		"INVALID_WORKSPACE",
		"WORKSPACE_INSPECTION_FAILED",
		"UNSAFE_WORKSPACE_SYMLINK",
		"RESOURCE_LIMIT",
		"INPUT_BINDING_MISMATCH",
		"INPUT_STAGE_FAILED",
		"INVALID_PRODUCTION_INPUT_PATH",
		"INVALID_PROJECTION",
		"INVALID_DIRECTORY",
		"ENDPOINT_UNAVAILABLE",
		"PROCESS_START_FAILED",
		"ISOLATION_UNAVAILABLE",
		"UNCONTAINED_DISPATCH_REFUSED",
		"INVALID_NETWORK_POLICY",
		"UNSAFE_WORKSPACE_SURFACE",
		"OUTPUT_OVERFLOW",
		"PROTOCOL_FAILURE",
		"MISSING_JSON",
		"INVALID_UTF8",
		"INVALID_UNICODE",
		"TRAILING_JSON",
		"UNKNOWN_FIELD",
		"INVALID_FIELD",
		"INVALID_JSON",
		"MISSING_FIELD",
		"INVALID_VERSION",
		"INVALID_DRIVER",
		"INVALID_MODEL",
		"INVALID_TRANSPORT_STATUS",
		"INVALID_USAGE",
		"INVALID_COST_OBSERVATION",
		"PARTIAL_USAGE",
		"PARTIAL_COST",
		"RESULT_BINDING_MISMATCH",
		"INVALID_RESULT",
		"INVALID_SUBMISSION",
		"INVALID_YIELD",
		"INVALID_YIELD_KIND",
		"INVALID_YIELD_MESSAGE",
		"INVALID_IDENTITY",
		"INVALID_RESPONSIBILITY",
		"INVALID_SUMMARY",
		"INVALID_DETAIL",
		"INVALID_EXACT_BYTES",
		"INVALID_PLAN_BYTES",
		"INVALID_DECISION",
		"NONCANONICAL_JSON",
		"DUPLICATE_NAME",
		"TRANSPORT_FAILURE",
		"SUBMISSION_REJECTED",
		"SUBMISSION_CONFLICT",
		"SUBMISSION_PROTOCOL_FAILED",
		"SUBMISSION_CORRECTIONS_EXHAUSTED",
		"SUBMISSION_SHAPE_MISMATCH",
		"YIELD_BINDING_MISMATCH",
		"PROCESS_FAILED",
		"INVOCATION_CANCELLED",
		"INVOCATION_TIMEOUT",
		"SUBMISSION_BINDING_MISMATCH",
		"PROCESS_TREE_NOT_QUIESCENT",
		"WORKSPACE_IDENTITY_CHANGED",
		"WORKSPACE_MUTATED",
		"INPUT_CLEANUP_FAILED",
		"INVALID_PACKAGE",
		"INVALID_PERMISSION",
		"UNSUPPORTED_HOST",
		"MISSING_SUBMISSION",
		"PROVIDER_ERROR",
		"PROVIDER_TRUNCATED",
		"PROVIDER_AUTHORIZATION_FAILED",
		"PROVIDER_LIMITED",
		"PROVIDER_REQUEST_REJECTED",
		"PROVIDER_UNAVAILABLE",
		"PROVIDER_TRANSPORT_FAILED",
		"INVALID_PROVIDER_REQUEST",
		"HTTP_REDIRECT_REFUSED",
		"CONTINUATION_INVALID",
		"TOOL_NOT_ALLOWED",
		"TOOL_PATH_INVALID",
		"TOOL_READ_FAILED",
		"TOOL_WRITE_FAILED",
		"TOOL_EDIT_FAILED",
		"INVALID_TOOL_ARGUMENT",
		"CREDENTIAL_UNAVAILABLE",
		"CREDENTIAL_NOT_CERTIFIED",
		"CREDENTIAL_IDENTITY_CHANGED",
		"CREDENTIAL_STALE",
		"NATIVE_NOT_CERTIFIED",
		"NATIVE_SURFACE_INVALID",
		"INVALID_BROKER",
		"BROKER_STATE_INVALID",
		"AWS_CONFIGURATION_INVALID",
		"AWS_NOT_CERTIFIED",
		"AWS_CREDENTIAL_EXPORT_INVALID",
		"AWS_RESOLUTION_FAILED",
		"AWS_SIGNING_FAILED",
		"INVALID_RECOVERY_INVOCATION",
		"INVALID_RECOVERY_DECISION",
		"INVALID_ADVISORY_INVOCATION",
		"INVALID_ADVISORY_RESULT",
		"INVALID_AUTOMATION_INVOCATION",
		"INVALID_AUTOMATION_BINDING",
		"INVALID_AUTOMATION_FACTS",
		"INVALID_AUTOMATION_MESSAGE",
		"INVALID_AUTOMATION_VALUE",
		"INVALID_AUTOMATION_OBSERVATION",
		"AUTOMATION_BINDING_MISMATCH",
		"AUTOMATION_PROTOCOL_FAILED",
		"AUTOMATION_CORRECTIONS_EXHAUSTED",
		"AUTOMATION_UNSUPPORTED",
		"RECOVERY_STEP_REFUSED",
		"INVALID_RECOVERABLE_INPUT":
		return true
	default:
		return false
	}
}

func contextFailure(err error) (Observation, error) {
	diagnostic, code := "invocation_cancelled", "INVOCATION_CANCELLED"
	if err == context.DeadlineExceeded {
		diagnostic, code = "invocation_timeout", "INVOCATION_TIMEOUT"
	}
	return Observation{Diagnostic: Diagnostic{Code: diagnostic}}, fail(code)
}
func validateInvocation(invocation Invocation) error {
	// Request admission threads the engine-computed reserved set (MaskNames),
	// which follows the configured project roots, so a relocated records or
	// journals root is rejected as an input path at dispatch. The wire-level
	// ValidateRequest (EncodeRequest below) keeps the fixed defaults because
	// the request schema carries no project configuration.
	if err := validateRequest(invocation.Request, invocation.MaskNames); err != nil {
		return err
	}
	if invocation.Request.Workspace.Path != GuestWorkspacePath ||
		validateWorkspace(Workspace{
			Path:   invocation.HostWorkspace,
			Access: invocation.Request.Workspace.Access,
		}) != nil {
		return fail("INVOCATION_WORKSPACE_MISMATCH")
	}
	if err := validateSelectedProfile(invocation.Selected); err != nil {
		return err
	}
	if invocation.Request.Profile != invocation.Selected.Profile.Key ||
		invocation.Request.Model != invocation.Selected.Model {
		return fail("INVOCATION_BINDING_MISMATCH")
	}
	if err := validateNetworkPolicy(
		invocation.Selected.Adapter.ID,
		invocation.Selected.Profile.Network,
	); err != nil {
		return err
	}
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return err
	}
	_, packageIdentity, err := admittedPackage()
	if err != nil {
		return fail("INVALID_PACKAGE")
	}
	body, err := EncodeRequest(invocation.Request)
	if err != nil {
		return err
	}
	inputBody, err := canonicalJSON(invocation.Request.Inputs)
	if err != nil {
		return err
	}
	if descriptor.InvocationID != invocation.Request.InvocationID ||
		descriptor.Package != packageIdentity ||
		descriptor.RequestDigest != Digest(body) ||
		descriptor.Role != invocation.Request.Role ||
		descriptor.OperationID != invocation.Request.Operation.ID ||
		descriptor.ProfileKey != invocation.Selected.Profile.Key ||
		descriptor.AdapterID != invocation.Selected.Adapter.ID ||
		descriptor.AdapterVersion != invocation.Selected.Adapter.Version ||
		descriptor.AdapterConfigDigest != invocation.Selected.Adapter.ConfigurationDigest ||
		descriptor.Network != invocation.Selected.Profile.Network ||
		descriptor.Model != invocation.Selected.Model ||
		descriptor.WorkspaceAccess != invocation.Request.Workspace.Access ||
		descriptor.FreshContext != invocation.Request.FreshContext ||
		descriptor.InputsDigest != Digest(inputBody) {
		return fail("PERMISSION_BINDING_MISMATCH")
	}
	if invocation.Selected.Adapter.ID == FakeDriverID {
		if !invocation.FakeProfile.valid() {
			return fail("INVALID_PROFILE")
		}
	} else if invocation.FakeProfile != "" {
		return fail("INVALID_PROFILE")
	}
	return nil
}

type boundedBuffer struct {
	mu              sync.Mutex
	maximum, retain int
	body            []byte
	total           int64
	overflow        bool
	onOverflow      func()
}

func (buffer *boundedBuffer) Write(body []byte) (int, error) {
	buffer.mu.Lock()
	buffer.total += int64(len(body))
	if len(buffer.body) < buffer.retain {
		remaining := buffer.retain - len(buffer.body)
		if remaining > len(body) {
			remaining = len(body)
		}
		buffer.body = append(buffer.body, body[:remaining]...)
	}
	if buffer.maximum > 0 && buffer.total > int64(buffer.maximum) {
		if !buffer.overflow {
			buffer.overflow = true
			overflow := buffer.onOverflow
			buffer.mu.Unlock()
			if overflow != nil {
				overflow()
			}
			return len(body), nil
		}
		buffer.mu.Unlock()
		return len(body), nil
	}
	buffer.mu.Unlock()
	return len(body), nil
}
func (buffer *boundedBuffer) snapshot() ([]byte, int64, bool) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.body...), buffer.total, buffer.overflow
}
func invocationContext(parent context.Context, timeoutMillis int64) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(timeoutMillis)*time.Millisecond)
}
