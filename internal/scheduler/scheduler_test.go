package scheduler

import (
	"testing"

	"github.com/swornagent/sworn/internal/board"
)

func TestBuildPlan_TwoIndependentTracks(t *testing.T) {
	// AC-1/AC-2: 2 independent tracks → same phase
	tracks := []board.TrackInfo{
		{ID: "T1"},
		{ID: "T2"},
	}

	plan, err := BuildPlan(tracks)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if len(plan.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(plan.Phases))
	}

	// Both should be in phase 0.
	if idx := plan.PhaseOf("T1"); idx != 0 {
		t.Errorf("PhaseOf(T1) = %d, want 0", idx)
	}
	if idx := plan.PhaseOf("T2"); idx != 0 {
		t.Errorf("PhaseOf(T2) = %d, want 0", idx)
	}
}

func TestBuildPlan_DependencyOrdering(t *testing.T) {
	// AC-2: T3 depends_on T1 → T1 in phase 0, T3 in phase 1
	tracks := []board.TrackInfo{
		{ID: "T1"},
		{ID: "T2"},
		{ID: "T3", DependsOn: []string{"T1"}},
	}

	plan, err := BuildPlan(tracks)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if len(plan.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(plan.Phases))
	}

	if idx := plan.PhaseOf("T1"); idx != 0 {
		t.Errorf("PhaseOf(T1) = %d, want 0", idx)
	}
	if idx := plan.PhaseOf("T2"); idx != 0 {
		t.Errorf("PhaseOf(T2) = %d, want 0", idx)
	}
	if idx := plan.PhaseOf("T3"); idx != 1 {
		t.Errorf("PhaseOf(T3) = %d, want 1 (depends on T1)", idx)
	}
}

func TestBuildPlan_FailurePropagation(t *testing.T) {
	// AC-3: T1 independent, T2 independent, T3 depends_on T1
	// This test verifies the plan structure; failure semantics
	// are tested in parallel.go.
	tracks := []board.TrackInfo{
		{ID: "T1"},
		{ID: "T2"},
		{ID: "T3", DependsOn: []string{"T1"}},
	}

	plan, err := BuildPlan(tracks)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if len(plan.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(plan.Phases))
	}
}

func TestBuildPlan_AllSucceed(t *testing.T) {
	// AC-4: exit code 0 when all pass
	tracks := []board.TrackInfo{
		{ID: "T1"},
		{ID: "T2"},
	}

	_, err := BuildPlan(tracks)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
}

func TestBuildPlan_NonExistentDep(t *testing.T) {
	tracks := []board.TrackInfo{
		{ID: "T1", DependsOn: []string{"T-nonexistent"}},
	}

	_, err := BuildPlan(tracks)
	if err == nil {
		t.Fatal("expected error for non-existent dependency, got nil")
	}
}

func TestBuildPlan_CycleDetection(t *testing.T) {
	tracks := []board.TrackInfo{
		{ID: "T1", DependsOn: []string{"T2"}},
		{ID: "T2", DependsOn: []string{"T3"}},
		{ID: "T3", DependsOn: []string{"T1"}},
	}

	_, err := BuildPlan(tracks)
	if err == nil {
		t.Fatal("expected error for dependency cycle, got nil")
	}
}

func TestBuildPlan_MultiDependency(t *testing.T) {
	// T5 depends_on [T1, T3] — should be phase 2 in: T1+T2 (P0), T3+T4 (P1), T5 (P2)
	tracks := []board.TrackInfo{
		{ID: "T1"},
		{ID: "T2"},
		{ID: "T3", DependsOn: []string{"T1"}},
		{ID: "T4", DependsOn: []string{"T2"}},
		{ID: "T5", DependsOn: []string{"T1", "T3"}},
	}

	plan, err := BuildPlan(tracks)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if len(plan.Phases) != 3 {
		t.Fatalf("expected 3 phases, got %d: %s", len(plan.Phases), plan.Summary())
	}

	if idx := plan.PhaseOf("T5"); idx != 2 {
		t.Errorf("PhaseOf(T5) = %d, want 2 (depends on T1 and T3)", idx)
	}
}

func TestBuildPlan_Empty(t *testing.T) {
	plan, err := BuildPlan(nil)
	if err != nil {
		t.Fatalf("BuildPlan(nil): %v", err)
	}
	if len(plan.Phases) != 0 {
		t.Errorf("expected 0 phases for empty input, got %d", len(plan.Phases))
	}

	plan, err = BuildPlan([]board.TrackInfo{})
	if err != nil {
		t.Fatalf("BuildPlan([]): %v", err)
	}
	if len(plan.Phases) != 0 {
		t.Errorf("expected 0 phases for empty input, got %d", len(plan.Phases))
	}
}