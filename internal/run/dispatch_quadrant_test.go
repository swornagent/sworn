package run

import (
	"context"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

// TestRunSlice_DispatchesCarryQuadrant proves the #36 → T16 link: when a slice
// carries an effort_complexity rating, every recorded dispatch is stamped with
// its quadrant, so the verdict ledger can project model fit per quadrant (the
// routing function). This is the reachability point for the eval/routing layer.
func TestRunSlice_DispatchesCarryQuadrant(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	st.EffortComplexity = &state.EffortComplexity{
		Effort: "low", Complexity: "high", Quadrant: "puzzle",
	}
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}
	runCmd(t, workspaceRoot, "git", "add", "docs/")
	runCmd(t, workspaceRoot, "git", "commit", "-m", "test: rated slice")

	called := false
	opts := RunSliceOptions{
		EscalationModels: []string{"fake/quick"},
		VerifierModel:    "fake/verifier",
		ImplementTimeout: DefaultImplementTimeout,
		Registry:         testRegistry(&fakeDriver{implement: markedImplement(&called)}),
	}
	if err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts); err != nil {
		t.Fatalf("RunSlice: %v", err)
	}

	got, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Verification.Dispatches) == 0 {
		t.Fatal("no dispatches recorded — cannot prove quadrant stamping")
	}
	for _, d := range got.Verification.Dispatches {
		if d.Quadrant != "puzzle" {
			t.Errorf("dispatch %q quadrant=%q, want puzzle", d.Role, d.Quadrant)
		}
	}
}
