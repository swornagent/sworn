//go:build linux

package effects

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/workspace"
)

func TestLocalCheckWorkerReconcilesExactDeterministicWorkspace(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("true executable unavailable: %v", err)
	}
	truePath, err = filepath.Abs(truePath)
	if err != nil {
		t.Fatal(err)
	}
	systemctl := filepath.Join(t.TempDir(), "systemctl")
	if err := os.WriteFile(systemctl, []byte("#!/bin/sh\nprintf 'inactive\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := t.TempDir()
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	runner, err := executor.NewLinux(executor.Options{
		RuntimeRoot: runtimeRoot, ShimArgv: []string{truePath},
		BubblewrapPath: truePath, SystemdRunPath: truePath, SystemctlPath: systemctl,
		Limits: executor.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeSource := t.TempDir()
	writeEffectFile(t, filepath.Join(runtimeSource, "bin", "check"), []byte("runtime\n"), 0o755)
	runtimeDigest, _, err := workspace.Measure(context.Background(), runtimeSource, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTree, err := executor.NewRuntimeTree(runtimeSource, runtimeDigest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.EncodeLocalCheckEffectRequest(engine.LocalCheckEffectRequest{
		SchemaVersion: engine.LocalCheckEffectRequestSchemaVersion,
		DeliveryRunID: "delivery-run", DeliveryID: "delivery-1", WorkID: "work-1", WorkAttempt: 1,
		BuilderEffectID: "builder-effect", CheckID: "check-1",
		DefinitionDigest: testEffectDigest("a"), RuntimeManifestDigest: runtimeDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	effect := engine.JournalEffect{
		ID: "check-effect", DeliveryRunID: "delivery-run", Kind: engine.EffectLocalCheck,
		Attempt: 2, Request: request,
	}
	attempt, err := engine.CheckAttemptIdentityFor(effect.ID, effect.Attempt, runtimeDigest)
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	if err := os.Chmod(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	invocationRoot := localCheckWorkspacePath(workspaceRoot, attempt.InvocationID)
	if err := os.MkdirAll(filepath.Join(invocationRoot, "candidate"), 0o700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(workspaceRoot, "other-attempt")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	worker := LocalCheckWorker{Runner: runner, Runtime: runtimeTree, WorkspaceRoot: workspaceRoot}
	cleanup, err := worker.reconcileUnbound(context.Background(), effect)
	if err != nil || cleanup.InvocationID() != attempt.InvocationID {
		t.Fatalf("cleanup = %#v, %v", cleanup, err)
	}
	if _, err := os.Lstat(invocationRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attempt workspace remains: %v", err)
	}
	if info, err := os.Stat(other); err != nil || !info.IsDir() {
		t.Fatalf("foreign workspace changed: %v", err)
	}
	replayed, err := worker.reconcileUnbound(context.Background(), effect)
	if err != nil || replayed.InvocationID() != attempt.InvocationID {
		t.Fatalf("idempotent cleanup = %#v, %v", replayed, err)
	}
}
