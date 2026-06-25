package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

func makeStatus(result, model string, attempt int) *state.Status {
	sc := true
	var sid *string
	if result != "pending" {
		s := "verifier-session-1"
		sid = &s
	}
	return &state.Status{
		SliceID: "S52-ledger-projection",
		Release: "2026-06-19-safe-parallelism",
		Track:   "T16-verdict-ledger",
		State:   state.Verified,
		Verification: state.Verification{
			Result:                  result,
			Model:                   model,
			Attempt:                 attempt,
			VerifierWasFreshContext: &sc,
			VerifierSessionID:       sid,
			Violations:              nil,
		},
	}
}

func TestProject_Pass(t *testing.T) {
	st := makeStatus("pass", "claude-sonnet-4-20250514", 1)
	st.Verification.Violations = []string{}

	r, ok := Project(st, 7)
	if !ok {
		t.Fatal("expected ok=true for pass verdict")
	}
	if r.Verdict != "pass" {
		t.Errorf("Verdict: want pass, got %s", r.Verdict)
	}
	if r.Release != "2026-06-19-safe-parallelism" {
		t.Errorf("Release: got %s", r.Release)
	}
	if r.Track != "T16-verdict-ledger" {
		t.Errorf("Track: got %s", r.Track)
	}
	if r.SliceID != "S52-ledger-projection" {
		t.Errorf("SliceID: got %s", r.SliceID)
	}
	if r.SliceKind != "verdict" {
		t.Errorf("SliceKind: want verdict, got %s (T16-verdict-ledger → strip prefix, first segment 'verdict')", r.SliceKind)
	}
	if r.GateCount != 7 {
		t.Errorf("GateCount: want 7, got %d", r.GateCount)
	}
	if r.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %s", r.Model)
	}
	if r.Attempt != 1 {
		t.Errorf("Attempt: want 1, got %d", r.Attempt)
	}
	if r.State != "verified" {
		t.Errorf("State: want verified, got %s", r.State)
	}
}

func TestProject_Fail(t *testing.T) {
	st := makeStatus("fail", "claude-sonnet-4-20250514", 3)
	st.Verification.Violations = []string{"missing proof bundle", "unreachable test"}

	r, ok := Project(st, 4)
	if !ok {
		t.Fatal("expected ok=true for fail verdict")
	}
	if r.Verdict != "fail" {
		t.Errorf("Verdict: want fail, got %s", r.Verdict)
	}
	if r.GateCount != 4 {
		t.Errorf("GateCount: want 4, got %d", r.GateCount)
	}
	if r.ViolationCount != 2 {
		t.Errorf("ViolationCount: want 2, got %d", r.ViolationCount)
	}
	if len(r.Violations) != 2 {
		t.Errorf("Violations len: want 2, got %d", len(r.Violations))
	}
}

func TestProject_Blocked(t *testing.T) {
	st := makeStatus("blocked", "claude-sonnet-4-20250514", 1)
	st.State = state.FailedVerification
	st.Verification.Violations = []string{"missing spec artefact"}

	r, ok := Project(st, 5)
	if !ok {
		t.Fatal("expected ok=true for blocked verdict")
	}
	if r.Verdict != "blocked" {
		t.Errorf("Verdict: want blocked, got %s", r.Verdict)
	}
	if r.State != "failed_verification" {
		t.Errorf("State: want failed_verification, got %s", r.State)
	}
}

func TestProject_Pending_NoVerdict(t *testing.T) {
	st := makeStatus("pending", "", 0)
	_, ok := Project(st, 5)
	if ok {
		t.Error("expected ok=false for pending verdict (no terminal result)")
	}
}

func TestProject_EmptyResult_NoVerdict(t *testing.T) {
	st := makeStatus("", "", 0)
	_, ok := Project(st, 5)
	if ok {
		t.Error("expected ok=false for empty verification.result")
	}
}

func TestSliceKind(t *testing.T) {
	tests := []struct {
		track string
		want  string
	}{
		{"T5-providers", "provider"},
		{"T12-harness-hardening", "harness"},
		{"T8-memory", "memory"},
		{"T3-commercial", "commercial"},
		{"T16-verdict-ledger", "verdict"}, // first-segment rule; spec example says "ledger" — see journal
		{"T1-concurrency-core", "concurrency"},
		{"T2-monitoring", "monitoring"},
		{"T4-mcp", "mcp"},
		{"T6-provider-ux", "provider"},
		{"T7-mcp-extensions", "mcp"},
		{"T9-telemetry", "telemetry"},
		{"T10-public-readiness", "public"},
		{"T11-infra-safety", "infra"},
		{"T13-sworn-role-parity", "sworn"},
		{"T14-baton-integration", "baton"},
		{"T15-cli-registry", "cli"},
		{"", "other"},
		{"plain-track-no-prefix", "other"},
	}
	for _, tt := range tests {
		got := SliceKind(tt.track)
		if got != tt.want {
			t.Errorf("SliceKind(%q) = %q, want %q", tt.track, got, tt.want)
		}
	}
}

func TestKey(t *testing.T) {
	r1 := Record{SliceID: "S01", Verdict: "pass", Ts: "2026-01-01T00:00:00Z"}
	r2 := Record{SliceID: "S01", Verdict: "pass", Ts: "2026-01-01T00:00:00Z"}
	r3 := Record{SliceID: "S01", Verdict: "fail", Ts: "2026-01-01T00:00:00Z"}

	if Key(r1) != Key(r2) {
		t.Error("same slice+verdict+ts should produce same key")
	}
	if Key(r1) == Key(r3) {
		t.Error("different verdict should produce different key")
	}
}

func TestAppend_WritesLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")

	r1 := Record{V: 1, Ts: "2026-01-01T00:00:00Z", SliceID: "S01", Verdict: "pass"}
	r2 := Record{V: 1, Ts: "2026-01-02T00:00:00Z", SliceID: "S02", Verdict: "fail"}

	if err := Append(path, r1); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := Append(path, r2); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d; content: %s", len(lines), string(raw))
	}

	// Each line must be valid JSON.
	for i, line := range lines {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("line %d not valid JSON: %v", i, err)
		}
	}
}

func TestAppend_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")

	r := Record{V: 1, Ts: "2026-01-01T00:00:00Z", SliceID: "S01", Verdict: "pass"}

	for i := 0; i < 3; i++ {
		if err := Append(path, r); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Errorf("idempotent re-sync: want 1 line, got %d; content: %s", len(lines), string(raw))
	}
}

func TestAppend_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger", "verdicts.jsonl")

	r := Record{V: 1, Ts: "2026-01-01T00:00:00Z", SliceID: "S01", Verdict: "pass"}
	if err := Append(path, r); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// File and parent dir should exist.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}
func TestProject_V2Dispatches(t *testing.T) {
	st := makeStatus("pass", "claude-sonnet-4-20250514", 1)
	st.Verification.Dispatches = []state.Dispatch{
		{Role: "implementer", Model: "claude-sonnet-4-20250514", CostUSD: 0.0420, Attempt: 1},
		{Role: "verifier", Model: "claude-sonnet-4-20250514", CostUSD: 0.0085, Attempt: 1},
		{Role: "captain", Model: "claude-sonnet-4-20250514", CostUSD: 0.0120, Attempt: 1},
	}

	r, ok := Project(st, 7)
	if !ok {
		t.Fatal("expected ok=true for pass verdict with dispatches")
	}
	if r.V != 2 {
		t.Errorf("V: want 2, got %d", r.V)
	}
	if len(r.Dispatches) != 3 {
		t.Fatalf("Dispatches: want 3, got %d", len(r.Dispatches))
	}
	if r.Dispatches[0].Role != "implementer" {
		t.Errorf("dispatches[0].Role: want implementer, got %s", r.Dispatches[0].Role)
	}
	if r.Dispatches[0].CostUSD != 0.0420 {
		t.Errorf("dispatches[0].CostUSD: want 0.0420, got %f", r.Dispatches[0].CostUSD)
	}
	if r.Dispatches[1].Role != "verifier" {
		t.Errorf("dispatches[1].Role: want verifier, got %s", r.Dispatches[1].Role)
	}
	if r.Dispatches[2].Role != "captain" {
		t.Errorf("dispatches[2].Role: want captain, got %s", r.Dispatches[2].Role)
	}
	// TotalCostUSD should be the sum.
	expectedTotal := 0.0420 + 0.0085 + 0.0120
	if r.TotalCostUSD != expectedTotal {
		t.Errorf("TotalCostUSD: want %f, got %f", expectedTotal, r.TotalCostUSD)
	}
}

func TestProject_V2RoundTrip(t *testing.T) {
	st := makeStatus("pass", "claude-sonnet-4-20250514", 1)
	st.Verification.Dispatches = []state.Dispatch{
		{Role: "implementer", Model: "claude-sonnet-4-20250514", CostUSD: 0.050, Attempt: 2},
		{Role: "verifier", Model: "gpt-4.1", CostUSD: 0.010, Attempt: 2},
	}

	r, ok := Project(st, 5)
	if !ok {
		t.Fatal("expected ok=true")
	}

	// Marshal and unmarshal to verify JSON round-trip.
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.V != 2 {
		t.Errorf("V: want 2, got %d", got.V)
	}
	if len(got.Dispatches) != 2 {
		t.Fatalf("Dispatches: want 2, got %d", len(got.Dispatches))
	}
	if got.Dispatches[0].Role != "implementer" {
		t.Errorf("dispatches[0].Role: want implementer, got %s", got.Dispatches[0].Role)
	}
	if got.TotalCostUSD < 0.0599 || got.TotalCostUSD > 0.0601 {
		t.Errorf("TotalCostUSD: want 0.060, got %f", got.TotalCostUSD)
	}
}

func TestProject_V1BackCompat(t *testing.T) {
	// A v:1 line (no dispatches, v:1) should still load without error.
	v1JSON := `{"v":1,"ts":"2026-01-01T00:00:00Z","release":"x","track":"T1","slice_id":"S01","slice_kind":"test","role":"implementer","verdict":"pass","state":"verified","gate_count":3,"violation_count":0,"sworn_version":"0.1.0"}`

	var r Record
	if err := json.Unmarshal([]byte(v1JSON), &r); err != nil {
		t.Fatalf("v:1 line should unmarshal without error: %v", err)
	}
	// v:2 fields should be zero-valued, not panic.
	if r.V != 1 {
		t.Errorf("V: want 1, got %d", r.V)
	}
	if r.Dispatches != nil {
		t.Errorf("Dispatches: want nil for v:1 line, got %v", r.Dispatches)
	}
	if r.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD: want 0 for v:1 line, got %f", r.TotalCostUSD)
	}
	// Core v:1 fields should survive.
	if r.SliceID != "S01" {
		t.Errorf("SliceID: want S01, got %s", r.SliceID)
	}
	if r.Verdict != "pass" {
		t.Errorf("Verdict: want pass, got %s", r.Verdict)
	}
}

func TestProject_EmptyDispatches(t *testing.T) {
	// No dispatches set: Project should still produce a valid v:2 Record
	// with empty/nil Dispatches and TotalCostUSD=0.
	st := makeStatus("pass", "claude-sonnet-4-20250514", 1)
	r, ok := Project(st, 3)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.V != 2 {
		t.Errorf("V: want 2, got %d", r.V)
	}
	if r.Dispatches != nil {
		t.Errorf("Dispatches: want nil, got %v", r.Dispatches)
	}
	if r.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD: want 0, got %f", r.TotalCostUSD)
	}
}
