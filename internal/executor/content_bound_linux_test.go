//go:build linux

package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContentBoundRuntimeIdentityIsDeterministicAndDirect(t *testing.T) {
	runtimeRoot := t.TempDir()
	first := &LinuxExecutor{options: Options{RuntimeRoot: runtimeRoot}}
	second := &LinuxExecutor{options: Options{RuntimeRoot: runtimeRoot}}
	invocationID := "check-attempt-018f4f0f"
	if first.contentBoundRuntimePath(invocationID) != second.contentBoundRuntimePath(invocationID) ||
		filepath.Dir(first.contentBoundRuntimePath(invocationID)) != runtimeRoot {
		t.Fatal("same invocation did not reconstruct one direct runtime binding")
	}
	if first.contentBoundRuntimePath(invocationID) == first.contentBoundRuntimePath("check-attempt-other") {
		t.Fatal("different content-bound invocations reused a runtime binding")
	}
}

func TestReconcileContentBoundCleansExactInactiveInvocationIdempotently(t *testing.T) {
	executor, calls := newReconciliationTestExecutor(t, "printf 'inactive\\n'", 0)
	invocationID := "check-attempt-018f4f0f"
	runtimePath := createContentBoundResidue(t, executor, invocationID)
	otherPath := createContentBoundResidue(t, executor, "check-attempt-other")

	cleanup, err := executor.ReconcileContentBound(context.Background(), invocationID)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.InvocationID() != invocationID || cleanup.proof == nil {
		t.Fatalf("cleanup proof = %#v", cleanup)
	}
	assertPathsAbsent(t, runtimePath)
	assertPathsPresent(t, otherPath)

	replayed, err := executor.ReconcileContentBound(context.Background(), invocationID)
	if err != nil || replayed.InvocationID() != invocationID || replayed.proof == nil {
		t.Fatalf("idempotent cleanup = %#v, %v", replayed, err)
	}
	if zero := (ContentBoundCleanup{}); zero.InvocationID() != "" || zero.proof != nil {
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

func TestReconcileContentBoundRejectsInvalidIdentityBeforeMutation(t *testing.T) {
	executor, calls := newReconciliationTestExecutor(t, "printf 'inactive\\n'", 0)
	foreign := filepath.Join(executor.options.RuntimeRoot, "foreign")
	if err := os.Mkdir(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	cleanup, err := executor.ReconcileContentBound(context.Background(), "../foreign")
	if err == nil || cleanup.InvocationID() != "" {
		t.Fatalf("invalid cleanup = %#v, %v", cleanup, err)
	}
	assertPathsPresent(t, foreign)
	if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid cleanup reached systemctl: %v", err)
	}
}

func TestReconcileContentBoundFailsClosedWhenLiveOrOwned(t *testing.T) {
	t.Run("live", func(t *testing.T) {
		executor, _ := newReconciliationTestExecutor(t, "printf 'active\\n'", 0)
		path := createContentBoundResidue(t, executor, "check-attempt-live")
		cleanup, err := executor.ReconcileContentBound(context.Background(), "check-attempt-live")
		if err == nil || !strings.Contains(err.Error(), "still live") || cleanup.InvocationID() != "" {
			t.Fatalf("live cleanup = %#v, %v", cleanup, err)
		}
		assertPathsPresent(t, path)
	})

	t.Run("cross instance prelaunch ownership", func(t *testing.T) {
		executor, calls := newReconciliationTestExecutor(t, "printf 'inactive\\n'", 0)
		peer := &LinuxExecutor{options: cloneExecutorOptions(executor.options)}
		path := createContentBoundResidue(t, executor, "check-attempt-owned")
		ownership, err := executor.acquireContentBoundOwnership(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		released := false
		t.Cleanup(func() {
			if !released {
				_ = releaseContentBoundOwnership(ownership)
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
		defer cancel()
		cleanup, err := peer.ReconcileContentBound(ctx, "check-attempt-owned")
		if err == nil || !errors.Is(err, context.DeadlineExceeded) || cleanup.InvocationID() != "" {
			t.Fatalf("owned cleanup = %#v, %v", cleanup, err)
		}
		assertPathsPresent(t, path)
		if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned cleanup reached systemctl: %v", err)
		}
		if err := releaseContentBoundOwnership(ownership); err != nil {
			t.Fatal(err)
		}
		released = true
	})
}

func TestContentBoundRunOwnershipIsSharedAcrossInstances(t *testing.T) {
	executor, _ := newReconciliationTestExecutor(t, "printf 'inactive\\n'", 0)
	peer := &LinuxExecutor{options: cloneExecutorOptions(executor.options)}
	first, err := executor.acquireContentBoundOwnership(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseContentBoundOwnership(first) //nolint:errcheck
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	second, err := peer.acquireContentBoundOwnership(ctx, false)
	if err != nil {
		t.Fatalf("independent content-bound runs were serialized: %v", err)
	}
	if err := releaseContentBoundOwnership(second); err != nil {
		t.Fatal(err)
	}
}

func createContentBoundResidue(t *testing.T, executor *LinuxExecutor, invocationID string) string {
	t.Helper()
	path := executor.contentBoundRuntimePath(invocationID)
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
	t.Cleanup(func() { _ = removePrivateTree(path) })
	return path
}
