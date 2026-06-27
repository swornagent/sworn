package model

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"
)

// cliDriver dispatches verification by spawning a CLI binary (claude -p or
// codex exec) that authenticates through the user's logged-in subscription.
// It implements Verifier with no API key — the CLI's own auth session is used.
//
// Design: this is a subprocess driver, not an HTTP client. The dispatch
// contract is the same Verifier interface; callers see no difference.
type cliDriver struct {
	binary  string        // path to claude/codex (or override from env)
	model   string        // model name to pass as --model
	timeout time.Duration // subprocess deadline
}

// Capabilities returns CapVerify — the CLI driver supports verification.
// Chat is deferred (claude-code may add agentic Chat in future).
func (d *cliDriver) Capabilities() Capability { return CapVerify }

// claudeBin returns the path to the claude binary from CLAUDE_BIN env,
// defaulting to "claude" (resolved from PATH at exec time).
func claudeBin() string {
	if b := os.Getenv("CLAUDE_BIN"); b != "" {
		return b
	}
	return "claude"
}

// cliTimeout reads SWORN_CLI_TIMEOUT (a Go duration string like "300s"),
// defaulting to 300 seconds.
func cliTimeout() time.Duration {
	if s := os.Getenv("SWORN_CLI_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	return 300 * time.Second
}

// newClaudeCLI constructs a cliDriver for claude-cli. model must be non-empty
// (the per-role model, e.g. "sonnet").
func newClaudeCLI(model string) *cliDriver {
	return &cliDriver{
		binary:  claudeBin(),
		model:   model,
		timeout: cliTimeout(),
	}
}

// newCodexCLI returns a deferral error — codex support is not yet implemented.
// TODO: codex exec support (S63-deferral-1). The two CLIs have different
// invocation shapes and output normalisation; claude-cli ships first.
// Tracking: https://github.com/swornagent/sworn/issues/19.
func newCodexCLI(model string) (*cliDriver, error) {
	return nil, fmt.Errorf("%w: codex support deferred (S63-deferral-1)", ErrDriverNotRegistered)
}

// Verify dispatches the role prompt by spawning the CLI binary with the system
// prompt and user payload concatenated as a single prompt argument.
//
// Invocation: claude -p --no-session-persistence --model <model> <prompt>
//
// --no-session-persistence is mandatory to preserve the fresh-context property
// (Rule 7): each Verify call is independent and must not reuse session state.
// --model passes the user's per-role model selection to claude -p.
//
// costUSD is always 0.0 — plain-text capture from subprocess stdout gives no
// usage metadata (pin 6a: flag from Coach ack).
func (d *cliDriver) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	// Concatenate systemPrompt + userPayload as a single prompt argument.
	// (system + user concatenated as a single prompt).
	prompt := systemPrompt + "\n\n" + userPayload

	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	args := []string{"-p", "--no-session-persistence", "--model", d.model, prompt}
	cmd := exec.CommandContext(ctx, d.binary, args...)
	cmd.Stdin = nil

	stdout, err := cmd.Output()
	if err != nil {
		return "", 0, d.classifyError(ctx, err)
	}

	// costUSD is always 0 with plain-text subprocess capture.
	// The driver does no output parsing — stdout IS the verdict text.
	// Trim trailing whitespace/newlines (fmt.Println in test fakes adds \n).
	return strings.TrimSpace(string(stdout)), 0, nil
}

// classifyError maps subprocess errors to typed model.Error values.
func (d *cliDriver) classifyError(ctx context.Context, err error) *Error {
	// Deadline exceeded → the call timed out.
	if ctx.Err() == context.DeadlineExceeded {
		return &Error{
			Kind:     KindTransient,
			Provider: "claude-cli",
			Model:    d.model,
			Message:  fmt.Sprintf("claude-cli timed out after %v", d.timeout),
		}
	}

	// Missing binary — terminal error (KindOther per Coach pin 5).
	// exec.Error wraps the "not found" sentinel when LookPath fails on a PATH
	// lookup; *fs.PathError is returned by Go 1.24+ for absolute-path binaries
	// that don't exist (e.g. /nonexistent/claude). Both are terminal.
	if ee, ok := err.(*exec.Error); ok {
		return &Error{
			Kind:     KindOther,
			Provider: "claude-cli",
			Model:    d.model,
			Message:  fmt.Sprintf("claude-cli: %q not found on PATH", d.binary),
			Err:      ee,
		}
	}
	if pe, ok := err.(*fs.PathError); ok {
		return &Error{
			Kind:     KindOther,
			Provider: "claude-cli",
			Model:    d.model,
			Message:  fmt.Sprintf("claude-cli: %q not found on PATH", d.binary),
			Err:      pe,
		}
	}

	// Non-zero exit code → CLI not logged in / auth failure (coarse but
	// acceptable for v1 — Coach flag (c)).
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr := string(exitErr.Stderr)
		return &Error{
			Kind:     KindAuth,
			Provider: "claude-cli",
			Model:    d.model,
			Message:  fmt.Sprintf("claude-cli exited with code %d: %s", exitErr.ExitCode(), stderr),
			Err:      err,
		}
	}

	// Fallback — unknown error; wrap conservatively.
	return &Error{
		Kind:     KindOther,
		Provider: "claude-cli",
		Model:    d.model,
		Message:  fmt.Sprintf("claude-cli dispatch: %v", err),
		Err:      err,
	}
}
