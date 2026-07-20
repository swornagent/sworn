//go:build linux

package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWritableIdentityIsDeterministicAndCollisionFailsClosed(t *testing.T) {
	runtimeRoot := t.TempDir()
	writableRoot := t.TempDir()
	first := &LinuxExecutor{options: Options{RuntimeRoot: runtimeRoot, WritableRoot: writableRoot}}
	second := &LinuxExecutor{options: Options{RuntimeRoot: runtimeRoot, WritableRoot: writableRoot}}
	invocationID := "effect-018f4f0f-attempt-1"
	generation := writableWorkspaceGeneration(invocationID)
	if generation != writableWorkspaceGeneration(invocationID) || !validWorkspaceGeneration(generation) {
		t.Fatalf("generation = %q", generation)
	}
	if first.unitName(invocationID) != second.unitName(invocationID) ||
		first.writableRuntimePath(invocationID) != second.writableRuntimePath(invocationID) ||
		first.writableWorkspacePath(invocationID, generation) != second.writableWorkspacePath(invocationID, generation) {
		t.Fatal("same invocation identity did not reconstruct the same executor bindings")
	}
	if filepath.Dir(first.writableRuntimePath(invocationID)) != runtimeRoot ||
		filepath.Dir(first.writableWorkspacePath(invocationID, generation)) != writableRoot {
		t.Fatal("writable residue is not an opaque direct child of its configured root")
	}
	otherID := "effect-018f4f0f-attempt-2"
	if first.unitName(invocationID) == first.unitName(otherID) ||
		first.writableRuntimePath(invocationID) == first.writableRuntimePath(otherID) ||
		first.writableWorkspacePath(invocationID, generation) ==
			first.writableWorkspacePath(otherID, writableWorkspaceGeneration(otherID)) {
		t.Fatal("different invocation identities reused an executor binding")
	}

	workspacePath, observedGeneration, err := first.createWritableWorkspace(invocationID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = removePrivateTree(workspacePath) })
	sentinel := filepath.Join(workspacePath, "sentinel")
	if err := os.WriteFile(sentinel, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := second.createWritableWorkspace(invocationID); err == nil ||
		!strings.Contains(err.Error(), "unreconciled workspace residue") {
		t.Fatalf("workspace identity collision error = %v", err)
	}
	if observedGeneration != generation {
		t.Fatalf("workspace generation = %q, want %q", observedGeneration, generation)
	}
	if contents, err := os.ReadFile(sentinel); err != nil || string(contents) != "owned" {
		t.Fatalf("collision changed existing residue: %q, %v", contents, err)
	}
}

func TestReconcileWritableCleansExactInactiveInvocationIdempotently(t *testing.T) {
	executor, calls := newReconciliationTestExecutor(t, "printf 'inactive\\n'", 0)
	invocationID := "effect-018f4f0f-attempt-7"
	runtimePath, workspacePath := createWritableResidue(t, executor, invocationID)
	otherRuntime, otherWorkspace := createWritableResidue(t, executor, "effect-018f4f0f-attempt-8")

	cleanup, err := executor.ReconcileWritable(context.Background(), invocationID)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.InvocationID() != invocationID || cleanup.proof == nil {
		t.Fatalf("cleanup proof = %#v", cleanup)
	}
	assertPathsAbsent(t, runtimePath, workspacePath)
	assertPathsPresent(t, otherRuntime, otherWorkspace)

	replayed, err := executor.ReconcileWritable(context.Background(), invocationID)
	if err != nil {
		t.Fatalf("idempotent reconciliation: %v", err)
	}
	if replayed.InvocationID() != invocationID || replayed.proof == nil {
		t.Fatalf("replayed cleanup proof = %#v", replayed)
	}
	if zero := (WritableCleanup{}); zero.InvocationID() != "" || zero.proof != nil {
		t.Fatalf("zero cleanup unexpectedly valid: %#v", zero)
	}

	lines := readNonemptyLines(t, calls)
	wantCall := "--user is-active " + executor.unitName(invocationID)
	if len(lines) != 4 {
		t.Fatalf("systemctl calls = %q, want four", lines)
	}
	for _, line := range lines {
		if line != wantCall {
			t.Fatalf("systemctl call = %q, want %q", line, wantCall)
		}
	}
}

func TestReconcileWritableFailsClosedWithoutQuiescence(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		exitStatus int
		want       string
	}{
		{"active", "printf 'active\\n'", 0, "still live"},
		{"activating", "printf 'activating\\n'", 0, "still live"},
		{"deactivating", "printf 'deactivating\\n'", 0, "still live"},
		{"unrecognized state", "printf 'surprising\\n'", 0, "unknown writable workspace service state"},
		{"control failure", "printf 'control failed\\n'", 7, "resolve writable workspace service state"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor, _ := newReconciliationTestExecutor(t, test.body, test.exitStatus)
			runtimePath, workspacePath := createWritableResidue(t, executor, "effect-fail-closed")
			cleanup, err := executor.ReconcileWritable(context.Background(), "effect-fail-closed")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("cleanup=%#v, error=%v; want %q", cleanup, err, test.want)
			}
			if cleanup.InvocationID() != "" || cleanup.proof != nil {
				t.Fatalf("failure minted cleanup proof: %#v", cleanup)
			}
			assertPathsPresent(t, runtimePath, workspacePath)
		})
	}
}

func TestReconcileWritableRejectsInvalidIdentityBeforeMutation(t *testing.T) {
	executor, calls := newReconciliationTestExecutor(t, "printf 'inactive\\n'", 0)
	runtimePath, workspacePath := createWritableResidue(t, executor, "valid-identity")
	cleanup, err := executor.ReconcileWritable(context.Background(), "../valid-identity")
	if err == nil || !strings.Contains(err.Error(), "valid writable invocation id") {
		t.Fatalf("cleanup=%#v, error=%v", cleanup, err)
	}
	assertPathsPresent(t, runtimePath, workspacePath)
	if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid identity reached systemctl: %v", err)
	}
}

func TestReconcileWritableWaitsForCrossInstancePrelaunchOwnership(t *testing.T) {
	executor, calls := newReconciliationTestExecutor(t, "printf 'inactive\\n'", 0)
	peer := &LinuxExecutor{options: cloneExecutorOptions(executor.options)}
	invocationID := "effect-owned-prelaunch"
	runtimePath, workspacePath := createWritableResidue(t, executor, invocationID)
	ownership, err := executor.acquireWritableOwnership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = releaseWritableOwnership(ownership)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	cleanup, err := peer.ReconcileWritable(ctx, invocationID)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("owned prelaunch cleanup=%#v, error=%v", cleanup, err)
	}
	assertPathsPresent(t, runtimePath, workspacePath)
	if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned prelaunch reached systemctl: %v", err)
	}
	if err := releaseWritableOwnership(ownership); err != nil {
		t.Fatal(err)
	}
	released = true
	cleanup, err = peer.ReconcileWritable(context.Background(), invocationID)
	if err != nil || cleanup.InvocationID() != invocationID {
		t.Fatalf("released prelaunch cleanup=%#v, error=%v", cleanup, err)
	}
	assertPathsAbsent(t, runtimePath, workspacePath)
}

func TestNewLinuxCopiesConfigurationSlices(t *testing.T) {
	binary, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("true executable unavailable: %v", err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	shimArgv := []string{binary, "shim-original"}
	allowedEnvironment := []string{"FIRST", "SECOND"}
	runtimeRoot := t.TempDir()
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	executor, err := NewLinux(Options{
		RuntimeRoot:        runtimeRoot,
		ShimArgv:           shimArgv,
		BubblewrapPath:     binary,
		SystemdRunPath:     binary,
		SystemctlPath:      binary,
		Limits:             DefaultLimits(),
		AllowedEnvironment: allowedEnvironment,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := executor.ConfigurationDigest()
	shimArgv[1] = "shim-mutated"
	allowedEnvironment[0] = "MUTATED"
	if got := executor.ConfigurationDigest(); got != wantDigest {
		t.Fatalf("caller slice mutation changed executor digest: got %q, want %q", got, wantDigest)
	}
	if executor.options.ShimArgv[1] != "shim-original" || executor.options.AllowedEnvironment[0] != "FIRST" {
		t.Fatalf("executor retained caller-owned slices: argv=%q environment=%q", executor.options.ShimArgv, executor.options.AllowedEnvironment)
	}
}

func newReconciliationTestExecutor(t *testing.T, body string, exitStatus int) (*LinuxExecutor, string) {
	t.Helper()
	calls := filepath.Join(t.TempDir(), "systemctl.calls")
	systemctl := filepath.Join(t.TempDir(), "systemctl")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shellSingleQuote(calls) + "\n" + body + "\nexit " + strconv.Itoa(exitStatus) + "\n"
	if err := os.WriteFile(systemctl, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return &LinuxExecutor{options: Options{
		RuntimeRoot:   t.TempDir(),
		WritableRoot:  t.TempDir(),
		SystemctlPath: systemctl,
	}}, calls
}

func createWritableResidue(t *testing.T, executor *LinuxExecutor, invocationID string) (string, string) {
	t.Helper()
	runtimePath := executor.writableRuntimePath(invocationID)
	workspacePath := executor.writableWorkspacePath(invocationID, writableWorkspaceGeneration(invocationID))
	for _, path := range []string{runtimePath, workspacePath} {
		nested := filepath.Join(path, "nested")
		if err := os.MkdirAll(nested, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "residue"), []byte("owned"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(nested, 0o000); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = removePrivateTree(runtimePath)
		_ = removePrivateTree(workspacePath)
	})
	return runtimePath, workspacePath
}

func assertPathsAbsent(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("path %q remains: %v", path, err)
		}
	}
}

func assertPathsPresent(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("path %q is absent: %v", path, err)
		}
	}
}

func readNonemptyLines(t *testing.T, path string) []string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for _, line := range strings.Split(string(contents), "\n") {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
