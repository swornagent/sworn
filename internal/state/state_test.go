package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		{Planned, Verified},            // skip every gate
		{Planned, Implemented},         // skip in_progress
		{InProgress, Verified},         // skip implemented
		{Verified, InProgress},         // terminal → non-terminal
		{Verified, FailedVerification}, // terminal
		{DesignReview, Verified},       // skip everything
		{Implemented, Planned},         // backward
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
		Schema: "https://baton.sawy3r.net/schemas/slice-status-v1.json", SliceID: "S05-state-and-git",
		Release:               "2026-06-15-e2e-turnkey-loop",
		Track:                 "T2-orchestration",
		State:                 InProgress,
		Owner:                 "human",
		LastUpdatedBy:         "implementer",
		LastUpdatedAt:         "2026-06-16T00:00:00Z",
		StartCommit:           "abc123",
		SpecPath:              "docs/release/x/S05/spec.md",
		ProofPath:             "docs/release/x/S05/proof.md",
		JournalPath:           "docs/release/x/S05/journal.md",
		PlannedFiles:          []string{"internal/state/", "internal/git/"},
		ActualFiles:           []string{"internal/state/state.go"},
		TestCommands:          []string{"go test ./..."},
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
		Release: "2026-06-27-conformance-foundation",
		Track:   "T4-records-as-json",
		State:   Planned,
		Verification: Verification{
			Result: "pending",
		},
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

func TestTransitionGate_PassesThroughGate(t *testing.T) {
	// Gate returns nil — transition should succeed.
	if err := Planned.TransitionGate(InProgress, func() error {
		return nil
	}); err != nil {
		t.Errorf("Planned → InProgress with passing gate: want nil, got %v", err)
	}
}

func TestTransitionGate_GateBlocksTransition(t *testing.T) {
	err := Planned.TransitionGate(InProgress, func() error {
		return fmt.Errorf("definition of ready failed: trace incomplete")
	})
	if err == nil {
		t.Fatal("Planned → InProgress with failing gate: want error, got nil")
	}
	if !strings.Contains(err.Error(), "definition of ready") {
		t.Errorf("want error mentioning 'definition of ready', got: %v", err)
	}
}

func TestTransitionGate_IllegalTransitionBeforeGate(t *testing.T) {
	// Gate should NOT be called for illegal transitions — state machine
	// rejects first.
	gateCalled := false
	err := Planned.TransitionGate(Verified, func() error {
		gateCalled = true
		return nil
	})
	if err == nil {
		t.Fatal("Planned → Verified: want error for illegal transition, got nil")
	}
	if gateCalled {
		t.Error("gate should not be called for illegal transition")
	}
}

func TestTransitionGate_NilGateSkipped(t *testing.T) {
	if err := Planned.TransitionGate(InProgress, nil); err != nil {
		t.Errorf("Planned → InProgress with nil gate: want nil, got %v", err)
	}
}

// TestTransitionFromLiveStatus ensures the state machine accepts every state// that appears in a real status.json written by other tools.
func TestTransitionFromLiveStatus(t *testing.T) {
	// The state machine must recognise all states used in real status.json files.
	for _, s := range []State{Planned, DesignReview, InProgress, Implemented, Verified, FailedVerification} {
		if _, ok := allowedTransitions[s]; !ok {
			t.Errorf("state %q is not in allowedTransitions — a live status.json may carry it", s)
		}
	}
}

// TestTraceFieldsRoundTrip ensures the RTM trace fields (need_ids,
// release_benefit, org_objective) survive a write-read cycle.
func TestTraceFieldsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	orig := Status{
		SliceID:        "S01-rtm-spine",
		Release:        "2026-06-27-conformance-foundation",
		Track:          "T4-records-as-json",
		State:          Planned,
		CoversNeeds:    []string{"N-01", "N-02"},
		ReleaseBenefit: "The release delivers value.",
		OrgObjective:   "Become the standard.",
		Verification: Verification{
			Result: "pending",
		},
	}
	if err := Write(path, &orig); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if len(got.CoversNeeds) != 2 || got.CoversNeeds[0] != "N-01" || got.CoversNeeds[1] != "N-02" {
		t.Errorf("NeedIDs: want [N-01 N-02], got %v", got.CoversNeeds)
	}
	if got.ReleaseBenefit != "The release delivers value." {
		t.Errorf("ReleaseBenefit: want %q, got %q", "The release delivers value.", got.ReleaseBenefit)
	}
	if got.OrgObjective != "Become the standard." {
		t.Errorf("OrgObjective: want %q, got %q", "Become the standard.", got.OrgObjective)
	}
}

func TestVerification_ModelAttemptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	orig := Status{
		SliceID: "S52-ledger-projection",
		Release: "2026-06-27-conformance-foundation",
		Track:   "T4-records-as-json",
		State:   Verified,
		Verification: Verification{
			Result:  "pass",
			Model:   "claude-sonnet-4-20250514",
			Attempt: 2,
		},
	}
	if err := Write(path, &orig); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got.Verification.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: want %q, got %q", "claude-sonnet-4-20250514", got.Verification.Model)
	}
	if got.Verification.Attempt != 2 {
		t.Errorf("Attempt: want 2, got %d", got.Verification.Attempt)
	}
}

func TestVerification_ModelAttemptOmitEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	// Zero-valued fields should be omitted (omitempty).
	orig := Status{
		SliceID: "S52-ledger-projection",
		Release: "2026-06-27-conformance-foundation",
		Track:   "T4-records-as-json",
		State:   Verified,
		Verification: Verification{
			Result: "pass",
		},
	}
	if err := Write(path, &orig); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"model"`) {
		t.Error("zero-valued Model should be omitted from JSON")
	}
	if strings.Contains(string(raw), `"attempt"`) {
		t.Error("zero-valued Attempt should be omitted from JSON")
	}

	// But they round-trip back as zero values.
	got, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Verification.Model != "" {
		t.Errorf("Model: want empty, got %q", got.Verification.Model)
	}
	if got.Verification.Attempt != 0 {
		t.Errorf("Attempt: want 0, got %d", got.Verification.Attempt)
	}
}

func TestDispatches_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	orig := Status{
		SliceID: "S55-ledger-multirole-cost",
		Release: "2026-06-27-conformance-foundation",
		Track:   "T4-records-as-json",
		State:   Verified,
		Verification: Verification{
			Result: "pass",
			Model:  "claude-sonnet-4-20250514",
			Dispatches: []Dispatch{
				{Role: "implementer", Model: "claude-sonnet-4-20250514", CostUSD: 0.0420, CostSource: "pricing-table", Attempt: 1, DurationMS: 4200, InputTokens: 1000, OutputTokens: 500},
				{Role: "verifier", Model: "claude-sonnet-4-20250514", CostUSD: 0.0085, CostSource: "cli", Attempt: 1, DurationMS: 850, InputTokens: 200, OutputTokens: 100, ModelIDConfirmed: "claude-sonnet-4-20250514"},
				{Role: "captain", Model: "claude-sonnet-4-20250514", CostUSD: 0.0120, Attempt: 1, DurationMS: 1200},
			},
		},
	}
	if err := Write(path, &orig); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if len(got.Verification.Dispatches) != 3 {
		t.Fatalf("Dispatches: want 3, got %d", len(got.Verification.Dispatches))
	}
	if got.Verification.Dispatches[0].Role != "implementer" {
		t.Errorf("dispatch[0].Role: want implementer, got %s", got.Verification.Dispatches[0].Role)
	}
	if got.Verification.Dispatches[0].CostUSD != 0.0420 {
		t.Errorf("dispatch[0].CostUSD: want 0.0420, got %f", got.Verification.Dispatches[0].CostUSD)
	}
	// S08 (honest cost telemetry) — cost_source round-trips per dispatch.
	if got.Verification.Dispatches[0].CostSource != "pricing-table" {
		t.Errorf("dispatch[0].CostSource: want pricing-table, got %q", got.Verification.Dispatches[0].CostSource)
	}
	if got.Verification.Dispatches[1].CostSource != "cli" {
		t.Errorf("dispatch[1].CostSource: want cli, got %q", got.Verification.Dispatches[1].CostSource)
	}
	// captain dispatch carries no CostSource (synthetic/omitted upstream) —
	// zero-value round-trips as empty string, not a fabricated value.
	if got.Verification.Dispatches[2].CostSource != "" {
		t.Errorf("dispatch[2].CostSource: want empty (zero value), got %q", got.Verification.Dispatches[2].CostSource)
	}
	if got.Verification.Dispatches[1].Role != "verifier" {
		t.Errorf("dispatch[1].Role: want verifier, got %s", got.Verification.Dispatches[1].Role)
	}
	if got.Verification.Dispatches[2].Role != "captain" {
		t.Errorf("dispatch[2].Role: want captain, got %s", got.Verification.Dispatches[2].Role)
	}
	// S24 fields — round-trip preserves new Dispatch fields.
	if got.Verification.Dispatches[0].DurationMS != 4200 {
		t.Errorf("dispatch[0].DurationMS: want 4200, got %d", got.Verification.Dispatches[0].DurationMS)
	}
	if got.Verification.Dispatches[0].InputTokens != 1000 {
		t.Errorf("dispatch[0].InputTokens: want 1000, got %d", got.Verification.Dispatches[0].InputTokens)
	}
	if got.Verification.Dispatches[0].OutputTokens != 500 {
		t.Errorf("dispatch[0].OutputTokens: want 500, got %d", got.Verification.Dispatches[0].OutputTokens)
	}
	if got.Verification.Dispatches[1].ModelIDConfirmed != "claude-sonnet-4-20250514" {
		t.Errorf("dispatch[1].ModelIDConfirmed: want claude-sonnet-4-20250514, got %s", got.Verification.Dispatches[1].ModelIDConfirmed)
	}
	// Zero-value S24 fields on captain dispatch round-trip correctly.
	if got.Verification.Dispatches[2].InputTokens != 0 {
		t.Errorf("dispatch[2].InputTokens: want 0 (zero value), got %d", got.Verification.Dispatches[2].InputTokens)
	}
}
func TestDispatches_OmitEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	orig := Status{
		SliceID: "S55-ledger-multirole-cost",
		Release: "2026-06-27-conformance-foundation",
		Track:   "T4-records-as-json",
		State:   Verified,
		Verification: Verification{
			Result: "pass",
		},
	}
	if err := Write(path, &orig); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"dispatches"`) {
		t.Error("empty Dispatches should be omitted from JSON")
	}

	// Round-trips back as nil.
	got, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Verification.Dispatches != nil {
		t.Errorf("Dispatches: want nil, got %v", got.Verification.Dispatches)
	}
}

// TestWrite_MalformedStatus verifies that Write() fails closed on a// Status that would produce invalid JSON per the embedded schema.
func TestWrite_MalformedStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	// Missing required slice_id.
	s := Status{
		SliceID: "", // deliberately empty — fails validation
		State:   Planned,
	}
	err := Write(path, &s)
	if err == nil {
		t.Fatal("Write() with empty slice_id: want validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("want 'validation failed' in error, got: %v", err)
	}

	// Confirm the file was NOT written.
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("status.json should not exist after a failed validated write")
	}
}

// TestWrite_SetsCanonicalSchema verifies that Write() always sets the
// canonical $schema URI regardless of what the caller provides.
func TestWrite_SetsCanonicalSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	s := Status{
		Schema:  "https://example.com/old-placeholder.json",
		SliceID: "S13-schema-embed-validate",
		Release: "2026-06-27-conformance-foundation",
		Track:   "T4-records-as-json",
		State:   InProgress,
		Verification: Verification{
			Result: "pending",
		},
	}
	if err := Write(path, &s); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Schema != "https://baton.sawy3r.net/schemas/slice-status-v1.json" {
		t.Errorf("Schema: want canonical URI, got %q", got.Schema)
	}

	// Also check raw JSON.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `https://baton.sawy3r.net/schemas/slice-status-v1.json`) {
		t.Error("raw JSON should contain the canonical $schema URI")
	}
	if strings.Contains(string(raw), "example.com") {
		t.Error("raw JSON should not contain example.com")
	}
}
