package driver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrKind names a subprocess driver's failure class. This package cannot
// import internal/model (TestNoWireImports), so it declares its own
// vocabulary rather than reusing model.ErrorKind.
const (
	// ErrKindConfig means the dispatch could not even be attempted — a bad
	// WorktreeRoot or a missing CLI binary.
	ErrKindConfig = "config"
	// ErrKindTransient means the dispatch timed out — retryable.
	ErrKindTransient = "transient"
	// ErrKindAuth means the CLI exited non-zero. This deliberately matches
	// internal/model/cli.go's existing coarse-but-production-proven
	// heuristic (any non-zero exit is treated as an auth failure) rather
	// than a generic provider label, so the engine's terminal-halt-on-auth
	// fail-fast (internal/run/slice.go, model.IsTerminal/KindAuth) survives
	// the driver rewire (ratified 2026-07-03, see spec.json R-03).
	ErrKindAuth = "auth"
	// ErrKindProvider is reserved for a future driver's genuinely-distinct
	// provider-side failure. Unused by claude.go and codex.go — both map a
	// non-zero CLI exit to ErrKindAuth, the binding cross-driver contract
	// (see docs/release/2026-06-28-driver-contract/S03-codex-subprocess-driver/design.md
	// decision 6).
	ErrKindProvider = "provider"
	// ErrKindProtocol means the CLI's output did not parse as expected
	// (the outer JSON envelope, or a verifier's inner result text).
	ErrKindProtocol = "protocol"
	// ErrKindCredits means the provider reported exhausted credits/quota.
	// Promoted from the in-process driver's private errKindCredits so the
	// terminal vocabulary has a single source (S04 Coach acknowledgement,
	// T3 captain-proceed.md 2026-07-10).
	ErrKindCredits = "credits"
	// ErrKindUnsupported means the dispatch asked for a capability the resolved
	// client does not have — specifically, a schema-constrained
	// (StructuredSchema-set) dispatch to a client that cannot emit structured
	// output. It is DELIBERATELY distinct from ErrKindProtocol (a structured
	// EMISSION that failed): capability-absent is not a failure to retry but a
	// declared Rule 2 deferral the gate records (the model genuinely cannot do
	// this), whereas an emission failure stays a hard, fail-closed error
	// (S02 D3, Coach-ratified 2026-07-12; [[project_driver_contract_recut]] —
	// the ErrKind vocabulary binds for all future drivers, so subprocess-family
	// drivers map capability-absent to THIS kind too rather than folding it
	// into ErrKindProtocol). Not terminal: TerminalErrKind stays {auth,
	// credits} — an unsupported capability is neither retryable nor a
	// credentials halt, it is a portability deferral.
	ErrKindUnsupported = "unsupported"
)

// TerminalErrKind reports whether kind can never succeed on retry or model
// escalation (revoked/missing credentials, exhausted credits). The set is
// exactly {auth, credits} — the S04 Coach acknowledgement (T3
// captain-proceed.md, 2026-07-10) is the binding record: subprocess drivers
// collapse all terminal cases to auth, but the in-process driver emits
// credits as its own kind, and an auth-only check would silently lose the
// credits fail-fast. Consumed by BOTH the engine's implement leg and the
// verify leg's terminal->BLOCKED mapping (S06 spec R-03) so the fail-fast
// property cannot split across consumers.
func TerminalErrKind(kind string) bool {
	return kind == ErrKindAuth || kind == ErrKindCredits
}

// spawnCacheDir is the fixed directory subprocess Go tooling caches are
// redirected to, kept outside any slice worktree so a dispatch never leaves
// build artefacts behind in the tree it operated on.
var spawnCacheDir = filepath.Join(os.TempDir(), "sworn-driver-cache")

// hygieneEnv returns the child process environment: the parent's own
// environment (HOME included — CLI credentials live under the real home,
// unlike the in-process tool executor's HOME=root hygiene) plus GOCACHE/
// GOMODCACHE redirected outside the worktree.
func hygieneEnv() []string {
	return append(os.Environ(),
		"GOCACHE="+filepath.Join(spawnCacheDir, "go-build"),
		"GOMODCACHE="+filepath.Join(spawnCacheDir, "go-mod"),
	)
}

// spawnResult is the raw outcome of running one subprocess: its stdout, the
// wall-clock duration, and a classified error (nil on success).
type spawnResult struct {
	Stdout     []byte
	DurationMS int64
	Err        *DriverError
}

// DriverError is a classified subprocess dispatch failure. Kind is one of
// the ErrKind* constants above.
type DriverError struct {
	Kind    string
	Message string
	Err     error
}

func (e *DriverError) Error() string { return e.Message }
func (e *DriverError) Unwrap() error { return e.Err }

// spawn runs binary with args, rooted at dir, bounded by timeout, and
// returns its stdout on success or a classified DriverError on failure. It
// never panics: every exec/timeout/exit-code failure is mapped to a Kind.
// A non-zero exit classifies as ErrKindAuth — see spawnClassified for a
// driver that needs a different non-zero-exit Kind.
func spawn(ctx context.Context, binary string, args []string, dir string, timeout time.Duration) spawnResult {
	return spawnClassified(ctx, binary, args, dir, timeout, ErrKindAuth)
}

// spawnClassified is spawn's generalisation: nonZeroExitKind lets a driver
// classify a non-zero CLI exit as something other than ErrKindAuth (e.g.
// codex.go reuses ErrKindAuth too, per the binding cross-driver contract in
// docs/release/2026-06-28-driver-contract/S03-codex-subprocess-driver/design.md
// decision 6 — but the parameter exists so a future driver isn't forced to
// match). Timeout and missing-binary classification are unaffected by this
// parameter — those failure modes mean the same thing regardless of which
// CLI is being spawned; only the non-zero-exit arm varies per driver.
func spawnClassified(ctx context.Context, binary string, args []string, dir string, timeout time.Duration, nonZeroExitKind string) spawnResult {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = hygieneEnv()
	cmd.Stdin = nil

	stdout, err := cmd.Output()
	duration := time.Since(start).Milliseconds()

	if err == nil {
		return spawnResult{Stdout: stdout, DurationMS: duration}
	}
	return spawnResult{DurationMS: duration, Err: classifySpawnError(ctx, binary, timeout, err, nonZeroExitKind)}
}

// classifySpawnError maps a subprocess error to a DriverError. The ordering
// mirrors internal/model/cli.go's classifyError: deadline-exceeded first
// (it can otherwise present as any of the other cases once the process is
// killed), then binary-not-found, then non-zero exit (classified as
// nonZeroExitKind — the one axis that varies per calling driver).
func classifySpawnError(ctx context.Context, binary string, timeout time.Duration, err error, nonZeroExitKind string) *DriverError {
	if ctx.Err() == context.DeadlineExceeded {
		return &DriverError{
			Kind:    ErrKindTransient,
			Message: fmt.Sprintf("%s timed out after %v", binary, timeout),
			Err:     err,
		}
	}

	if ee, ok := err.(*exec.Error); ok {
		return &DriverError{
			Kind:    ErrKindConfig,
			Message: fmt.Sprintf("%s: %q not found on PATH", binary, binary),
			Err:     ee,
		}
	}
	if pe, ok := err.(*fs.PathError); ok {
		return &DriverError{
			Kind:    ErrKindConfig,
			Message: fmt.Sprintf("%s: %q not found on PATH", binary, binary),
			Err:     pe,
		}
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr := string(exitErr.Stderr)
		return &DriverError{
			Kind:    nonZeroExitKind,
			Message: fmt.Sprintf("%s exited with code %d: %s", binary, exitErr.ExitCode(), stderr),
			Err:     err,
		}
	}

	return &DriverError{
		Kind:    ErrKindConfig,
		Message: fmt.Sprintf("%s dispatch: %v", binary, err),
		Err:     err,
	}
}
