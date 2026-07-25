package driver

import (
	"context"
	"sync"
	"time"
)

const (
	MaxStdoutBytes         = 8_388_608
	MaxResultEnvelopeBytes = 16_384
	MaxStderrBytes         = 65_536
	MaxStderrRetain        = 1_024
)

// Invoker performs one process attempt without lifecycle or Baton authority.
type Invoker struct{}

// Driver is the single role-neutral invocation shape used by every provider.
type Driver interface {
	Invoke(context.Context, Invocation) (Observation, error)
}

var _ Driver = Invoker{}

type Invocation struct {
	Request       Request
	HostWorkspace string
	Selected      SelectedProvider
	Permission    SubmissionPermission
	Inputs        []InputContent
	FakeProfile   FakeProfile
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
	TextBytes       int64           `json:"text_bytes"`
	TextDigest      string          `json:"text_digest"`
	Usage           UsageReceipt    `json:"usage"`
	Diagnostic      Diagnostic      `json:"diagnostic"`
	Handoff         *SealedHandoff  `json:"handoff"`
	Events          []TerminalEvent `json:"events"`
}

func (Invoker) Invoke(ctx context.Context, invocation Invocation) (Observation, error) {
	if ctx == nil {
		return Observation{}, fail("INVALID_CONTEXT")
	}
	if err := ctx.Err(); err != nil {
		return contextFailure(err)
	}
	if err := validateInvocation(invocation); err != nil {
		return Observation{}, err
	}
	return platformInvoke(ctx, invocation)
}
func contextFailure(err error) (Observation, error) {
	diagnostic, code := "invocation_cancelled", "INVOCATION_CANCELLED"
	if err == context.DeadlineExceeded {
		diagnostic, code = "invocation_timeout", "INVOCATION_TIMEOUT"
	}
	return Observation{Diagnostic: Diagnostic{Code: diagnostic}}, fail(code)
}
func validateInvocation(invocation Invocation) error {
	if err := ValidateRequest(invocation.Request); err != nil {
		return err
	}
	if invocation.Request.Workspace.Path != GuestWorkspacePath ||
		validateWorkspace(Workspace{
			Path:   invocation.HostWorkspace,
			Access: invocation.Request.Workspace.Access,
		}) != nil {
		return fail("INVOCATION_WORKSPACE_MISMATCH")
	}
	if invocation.Request.Role == RoleMerge || invocation.Request.Model == nil ||
		*invocation.Request.Model != invocation.Selected.Model {
		return fail("INVOCATION_BINDING_MISMATCH")
	}
	if invocation.Selected.Provider.DriverID == "" ||
		invocation.Selected.Provider.DriverVersion == "" {
		return fail("INVOCATION_BINDING_MISMATCH")
	}
	if err := validateNetworkPolicy(
		invocation.Selected.Provider.DriverID,
		invocation.Selected.Provider.Network,
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
		descriptor.ProviderKey != invocation.Selected.Provider.Key ||
		descriptor.DriverID != invocation.Selected.Provider.DriverID ||
		descriptor.DriverVersion != invocation.Selected.Provider.DriverVersion ||
		descriptor.ExecutableDigest != invocation.Selected.Provider.Executable.Digest ||
		descriptor.Network != invocation.Selected.Provider.Network ||
		descriptor.Model != invocation.Selected.Model ||
		descriptor.WorkspaceAccess != invocation.Request.Workspace.Access ||
		descriptor.FreshContext != invocation.Request.FreshContext ||
		descriptor.InputsDigest != Digest(inputBody) {
		return fail("PERMISSION_BINDING_MISMATCH")
	}
	if invocation.Selected.Provider.DriverID == FakeDriverID {
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
