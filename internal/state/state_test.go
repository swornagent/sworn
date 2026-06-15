package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTransition_LegalMoves(t *testing.T) {
	tests := []struct {
		from, to State
	}{
		{Planned, DesignReview},
		{Planned, InProgress},
		{DesignReview, InProgress},
		{InProgress, Implemented},
		{Implemented, Verified},
		{Implemented, FailedVerification},
		{FailedVerification, InProgress},
	}
	for _, tt := range tests {
		if err := tt.from.Transition(tt.to); err != nil {
			t.Errorf("%s → %s: want nil, got %v", tt.from, tt.to, err)
		}
	}
}

func TestTransition_IllegalMoves(t *testing.T) {
	tests := []struct {
		from, to State
	}{
		{Planned, Verified},           // skip every gate
		{Planned, Implemented},        // skip in_progress
		{InProgress, Verified},        // skip implemented
		{Verified, InProgress},        // terminal → non-terminal
		{Verified, FailedVerification}, // terminal
		{DesignReview, Verified},      // skip everything
		{Implemented, Planned},        // backward
		{FailedVerification, Verified}, // skip implemented
	}
	for _, tt := range tests {
		if err := tt.from.Transition(tt.to); err == nil {
			t.Errorf("%s → %s: want error, got nil", tt.from, tt.to)
		}
	}
}

func TestTransition_UnknownState(t *testing.T) {
	if err := State("bogus").Transition(InProgress); err == nil {
		t.Error("unknown state: want error, got nil")
	}
}

func TestReadWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	orig := Status{
		Schema:          "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:         "S05-state-and-git",
		Release:         "2026-06-15-e2e-turnkey-loop",
		Track:           "T2-orchestration",
		State:           InProgress,
		Owner:           "human",
		LastUpdatedBy:   "implementer",
		LastUpdatedAt:   "2026-06-16T00:00:00Z",
		StartCommit:     "abc123",
		SpecPath:        "docs/release/x/S05/spec.md",
		ProofPath:       "docs/release/x/S05/proof.md",
		JournalPath:     "docs/release/x/S05/journal.md",
		PlannedFiles:    []string{"internal/state/", "internal/git/"},
		ActualFiles:     []string{"internal/state/state.go"},
		TestCommands:    []string{"go test ./..."},
		ReachabilityArtifacts: []string{"proof.md"},
		Verification: Verification{
			Result: "pending",
		},
		ReleaseBase: "release/v0.1.0",
	}

	if err := Write(path, &orig); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got.SliceID != orig.SliceID {
		t.Errorf("SliceID: want %q, got %q", orig.SliceID, got.SliceID)
	}
	if got.State != orig.State {
		t.Errorf("State: want %q, got %q", orig.State, got.State)
	}
	if got.StartCommit != orig.StartCommit {
		t.Errorf("StartCommit: want %q, got %q", orig.StartCommit, got.StartCommit)
	}
	if len(got.PlannedFiles) != len(orig.PlannedFiles) {
		t.Errorf("PlannedFiles: want %d, got %d", len(orig.PlannedFiles), len(got.PlannedFiles))
	}
	if got.Verification.Result != orig.Verification.Result {
		t.Errorf("Verification.Result: want %q, got %q", orig.Verification.Result, got.Verification.Result)
	}
}

func TestRead_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	_, err := Read(path)
	if err == nil {
		t.Fatal("want error for missing file, got nil")
	}
}

func TestRead_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
}

func TestWrite_RoundTripPreservesJSONShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	s := Status{
		Schema:  "v1",
		SliceID: "S01",
		State:   Planned,
	}
	if err := Write(path, &s); err != nil {
		t.Fatal(err)
	}

	// Read back raw bytes and check key fields are present.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["state"] != string(Planned) {
		t.Errorf("state: want %q, got %v", Planned, m["state"])
	}
	if m["slice_id"] != "S01" {
		t.Errorf("slice_id: want S01, got %v", m["slice_id"])
	}
}

// TestTransitionFromLiveStatus ensures the state machine accepts every state
// that appears in a real status.json written by other tools.
func TestTransitionFromLiveStatus(t *testing.T) {
	// The state machine must recognise all states used in real status.json files.
	for _, s := range []State{Planned, DesignReview, InProgress, Implemented, Verified, FailedVerification} {
		if _, ok := allowedTransitions[s]; !ok {
			t.Errorf("state %q is not in allowedTransitions — a live status.json may carry it", s)
		}
	}
}