package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/state"
)

// AC-05 reachability: a RunSlice dispatch on a spec.json-only slice (no spec.md)
// reaches the implement step without the sworn#97 spec.md-missing error. This
// is the engine-level integration point that owns the `sworn run --parallel`
// affordance (the scheduler worker calls RunSlice per slice); the fakeDriver
// exercises the design/captain/implement/verify legs, all of which now resolve
// the machine contract via spec.json.
func TestRunSlice_SpecJSONOnly_ReachesImplement(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)
	sliceDir := filepath.Dir(specPath)

	// Convert to a spec.json-ONLY slice: remove spec.md, author spec.json.
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	specJSON := `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S01-task",
  "release": "test-release",
  "user_outcome": "The engine implements a spec.json-only slice.",
  "covers_needs": ["N-01"],
  "in_scope": ["Read spec.json"],
  "out_of_scope": ["N/A"],
  "acceptance_criteria": [
    {"id": "AC-01", "text": "WHEN spec.json exists, THE engine SHALL read it (N-01).", "ears_pattern": "event-driven"}
  ]
}
`
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	// Commit so the worktree is clean for RunSlice.
	runCmd(t, workspaceRoot, "git", "add", "docs/")
	runCmd(t, workspaceRoot, "git", "commit", "-m", "spec.json-only slice")

	implementReached := false
	opts := RunSliceOptions{
		EscalationModels: []string{"fake/quick"},
		VerifierModel:    "fake/verifier",
		CaptainModel:     "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: DefaultImplementTimeout,
		Registry: testRegistry(&fakeDriver{
			implement: func(_ context.Context, _ driver.DispatchInput) (driver.Result, error) {
				implementReached = true
				return driver.Result{Status: driver.StatusOK, ResultText: "Done."}, nil
			},
		}),
	}

	// specPath still names spec.md (now absent); RunSlice resolves spec.json
	// from the slice directory.
	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		if strings.Contains(err.Error(), "spec.md") {
			t.Fatalf("sworn#97 regression — RunSlice hard-failed on missing spec.md: %v", err)
		}
		t.Fatalf("RunSlice on spec.json-only slice: %v", err)
	}
	if !implementReached {
		t.Fatal("implement step was never reached on a spec.json-only slice")
	}
}

// CHOICE-B: setupSlice (the `sworn run --task` on-ramp) writes an authoritative
// spec.json, not only a spec.md, so the engine reads one machine contract.
func TestSetupSlice_WritesSpecJSON(t *testing.T) {
	dir := t.TempDir()
	releaseDir, sliceDir, err := setupSlice(dir, "make the widget blue")
	if err != nil {
		t.Fatalf("setupSlice: %v", err)
	}
	_ = releaseDir
	specJSONPath := filepath.Join(dir, sliceDir, "spec.json")
	if _, err := os.Stat(specJSONPath); err != nil {
		t.Fatalf("setupSlice did not write spec.json: %v", err)
	}
	// status.SpecPath points at the authoritative spec.json.
	st, err := state.Read(filepath.Join(dir, sliceDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(st.SpecPath, "spec.json") {
		t.Fatalf("status.SpecPath should point at spec.json, got %q", st.SpecPath)
	}
	// The human spec.md is retained as the legacy artefact.
	if _, err := os.Stat(filepath.Join(dir, sliceDir, "spec.md")); err != nil {
		t.Fatalf("setupSlice should retain spec.md as the human artefact: %v", err)
	}
}
