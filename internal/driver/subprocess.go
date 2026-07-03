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
	// provider-side failure. Unused by claude.go itself.
	ErrKindProvider = "provider"
	// ErrKindProtocol means the CLI's output did not parse as expected
	// (the outer JSON envelope, or a verifier's inner result text).
	ErrKindProtocol = "protocol"
)

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
func spawn(ctx context.Context, binary string, args []string, dir string, timeout time.Duration) spawnResult {
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
	return spawnResult{DurationMS: duration, Err: classifySpawnError(ctx, binary, timeout, err)}
}

// classifySpawnError maps a subprocess error to a DriverError. The ordering
// mirrors internal/model/cli.go's classifyError: deadline-exceeded first
// (it can otherwise present as any of the other cases once the process is
// killed), then binary-not-found, then non-zero exit.
func classifySpawnError(ctx context.Context, binary string, timeout time.Duration, err error) *DriverError {
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
			Kind:    ErrKindAuth,
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
