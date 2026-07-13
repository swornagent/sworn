package run

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/verdict"
)

// NOTE (S06): the newAgentFromModel CapChat capability-gate cases that lived
// here are superseded by the registry's role check ("capability IS the role
// set", S05): an incapable driver is now rejected by name at Resolve time.
// The tests below are the AC-01/AC-02 reachability net for that contract at
// the RunSlice integration point.

// TestRunSliceDispatchesAllLegsViaRegistry is the AC-01 reachability test:
// every role leg — captain (design TL;DR + review), implement, verify —
// obtains its driver via Registry.Resolve and dispatches via Driver.Dispatch.
// The fake driver records every DispatchInput; no factory seam exists.
func TestRunSliceDispatchesAllLegsViaRegistry(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	verifier := &fakeVerifier{verdicts: []verdict.Result{{Verdict: verdict.Pass, Rationale: "ok"}}}
	fd := &fakeDriver{
		verify: verdictsFrom(verifier),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, RunSliceOptions{
		EscalationModels: []string{"fake/impl"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		ImplementTimeout: -1,
		Registry:         testRegistry(fd),
	})
	if err != nil {
		t.Fatalf("RunSlice: %v", err)
	}

	roles := fd.dispatchedRoles()
	if roles[driver.RoleCaptain] == 0 {
		t.Error("captain leg was not dispatched via the registry-resolved driver")
	}
	if roles[driver.RoleImplementer] == 0 {
		t.Error("implement leg was not dispatched via the registry-resolved driver")
	}
	if roles[driver.RoleVerifier] == 0 {
		t.Error("verify leg was not dispatched via the registry-resolved driver")
	}
}

// TestRunSliceResolutionFailure is the AC-02 test: a Resolve failure for the
// implement or verify role leg returns a descriptive error naming the model,
// role, and registered alternatives BEFORE any model dispatch — no nil
// dereference is possible because no code path holds an unresolved driver.
func TestRunSliceResolutionFailure(t *testing.T) {
	t.Run("unknown prefix for implementer", func(t *testing.T) {
		workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

		fd := &fakeDriver{}
		err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, RunSliceOptions{
			EscalationModels: []string{"nope/model-x"},
			VerifierModel:    "fake/verifier",
			CaptainModel:     "fake/verifier",
			ImplementTimeout: -1,
			Registry:         testRegistry(fd),
		})
		if err == nil {
			t.Fatal("expected resolution error for unknown prefix, got nil")
		}
		if !strings.Contains(err.Error(), `"nope/model-x"`) {
			t.Errorf("error should name the model ID, got: %v", err)
		}
		if !strings.Contains(err.Error(), `role "implementer"`) {
			t.Errorf("error should name the role, got: %v", err)
		}
		if !strings.Contains(err.Error(), "registered prefixes") || !strings.Contains(err.Error(), "fake/") {
			t.Errorf("error should enumerate the registered alternatives, got: %v", err)
		}
		if got := fd.dispatchCount(); got != 0 {
			t.Fatalf("expected ZERO dispatches on resolution failure, got %d", got)
		}
	})

	t.Run("role-incapable driver for implementer", func(t *testing.T) {
		workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

		// Driver declares verifier+captain only — implementer resolution
		// must fail by name, before any dispatch.
		fd := &fakeDriver{
			name:  "verify-only-driver",
			roles: driver.RoleSet{driver.RoleVerifier: true, driver.RoleCaptain: true},
		}
		err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, RunSliceOptions{
			EscalationModels: []string{"fake/impl"},
			VerifierModel:    "fake/verifier",
			CaptainModel:     "fake/verifier",
			ImplementTimeout: -1,
			Registry:         testRegistry(fd),
		})
		if err == nil {
			t.Fatal("expected role-resolution error, got nil")
		}
		if !strings.Contains(err.Error(), `"fake/impl"`) {
			t.Errorf("error should name the model ID, got: %v", err)
		}
		if !strings.Contains(err.Error(), `role "implementer"`) {
			t.Errorf("error should name the role, got: %v", err)
		}
		if !strings.Contains(err.Error(), "declared roles") {
			t.Errorf("error should name the driver's declared roles (alternatives vocabulary), got: %v", err)
		}
		if got := fd.dispatchCount(); got != 0 {
			t.Fatalf("expected ZERO dispatches on resolution failure, got %d", got)
		}
	})

	t.Run("role-incapable driver for verifier", func(t *testing.T) {
		workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

		fd := &fakeDriver{
			name:  "impl-only-driver",
			roles: driver.RoleSet{driver.RoleImplementer: true, driver.RoleCaptain: true},
		}
		err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, RunSliceOptions{
			EscalationModels: []string{"fake/impl"},
			VerifierModel:    "fake/verifier",
			CaptainModel:     "fake/verifier",
			ImplementTimeout: -1,
			Registry:         testRegistry(fd),
		})
		if err == nil {
			t.Fatal("expected verifier role-resolution error, got nil")
		}
		if !strings.Contains(err.Error(), `"fake/verifier"`) {
			t.Errorf("error should name the model ID, got: %v", err)
		}
		if !strings.Contains(err.Error(), `role "verifier"`) {
			t.Errorf("error should name the role, got: %v", err)
		}
		if got := fd.dispatchCount(); got != 0 {
			t.Fatalf("expected ZERO dispatches on resolution failure, got %d", got)
		}
	})
}

// TestRunSliceCaptainResolutionFailureDefersAndProceeds is the AC-02
// captain-leg arm (Coach decision 2026-07-10, captain-proceed.md pin 1): a
// captain-leg Resolve failure — e.g. a subprocess-only escalation head, since
// no subprocess driver declares RoleCaptain (sworn#86) — records the
// descriptive role error as a durable Rule 2 deferral via the design-gate
// deferral path and PROCEEDS; it never hard-halts the run.
func TestRunSliceCaptainResolutionFailureDefersAndProceeds(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	// Driver declares implementer+verifier only — captain resolution fails.
	fd := &fakeDriver{
		name:  "no-captain-driver",
		roles: driver.RoleSet{driver.RoleImplementer: true, driver.RoleVerifier: true},
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, RunSliceOptions{
		EscalationModels: []string{"fake/impl"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		ImplementTimeout: -1,
		Registry:         testRegistry(fd),
	})
	if err != nil {
		t.Fatalf("captain resolution failure must fail open, not halt the run: %v", err)
	}

	d := findDesignGateDeferral(t, statusPath)
	if d == nil {
		t.Fatal("expected a design_review_gate deferral recording the captain resolution failure")
	}
	if !strings.Contains(d.Why, `role "captain"`) {
		t.Errorf("deferral should embed the registry's descriptive role error, got %q", d.Why)
	}
	// The deferral names the CAPTAIN's model. It used to name the implementer's
	// escalation head, because the captain took escalationModels[0] — this
	// assertion encoded that coupling as if it were intended. The captain now
	// resolves as its own role.
	if !strings.Contains(d.Why, `"fake/verifier"`) {
		t.Errorf("deferral should name the CAPTAIN's model ID, got %q", d.Why)
	}

	// The captain role must never have been dispatched; implement+verify ran.
	roles := fd.dispatchedRoles()
	if roles[driver.RoleCaptain] != 0 {
		t.Errorf("captain must not be dispatched after resolution failure, got %d dispatches", roles[driver.RoleCaptain])
	}
	if roles[driver.RoleImplementer] == 0 || roles[driver.RoleVerifier] == 0 {
		t.Error("run should have proceeded to implement and verify legs")
	}
}
