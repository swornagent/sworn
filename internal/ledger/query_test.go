package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)
// ── Helpers ──────────────────────────────────────────────────────────────

// makeRecords returns a fixed in-memory corpus for deterministic aggregation tests.
func makeRecords() []Record {
	return []Record{
		{V: 1, Ts: "2026-01-01T00:00:00Z", Release: "rel1", Track: "T5-providers", SliceID: "S01", SliceKind: "provider", Model: "claude-sonnet", Attempt: 1, Verdict: "pass", GateCount: 5, TotalCostUSD: 0.42},
		{V: 1, Ts: "2026-01-02T00:00:00Z", Release: "rel1", Track: "T5-providers", SliceID: "S02", SliceKind: "provider", Model: "claude-sonnet", Attempt: 2, Verdict: "pass", GateCount: 3, TotalCostUSD: 0.38},
		{V: 1, Ts: "2026-01-03T00:00:00Z", Release: "rel1", Track: "T12-harness-hardening", SliceID: "S03", SliceKind: "harness", Model: "claude-sonnet", Attempt: 1, Verdict: "fail", GateCount: 7, Violations: []string{"missing proof bundle", "unreachable test"}, ViolationCount: 2, TotalCostUSD: 0.55},
		{V: 1, Ts: "2026-01-04T00:00:00Z", Release: "rel1", Track: "T8-memory", SliceID: "S04", SliceKind: "memory", Model: "gpt-5", Attempt: 3, Verdict: "pass", GateCount: 4, TotalCostUSD: 0.12},
		{V: 1, Ts: "2026-01-05T00:00:00Z", Release: "rel1", Track: "T8-memory", SliceID: "S05", SliceKind: "memory", Model: "gpt-5", Attempt: 1, Verdict: "fail", GateCount: 4, Violations: []string{"missing proof bundle"}, ViolationCount: 1, TotalCostUSD: 0.08},
		{V: 1, Ts: "2026-01-06T00:00:00Z", Release: "rel1", Track: "T3-commercial", SliceID: "S06", SliceKind: "commercial", Model: "claude-sonnet", Attempt: 1, Verdict: "blocked", GateCount: 2, TotalCostUSD: 0.30},
		{V: 1, Ts: "2026-01-07T00:00:00Z", Release: "rel1", Track: "T5-providers", SliceID: "S07", SliceKind: "provider", Model: "claude-sonnet", Attempt: 1, Verdict: "fail", GateCount: 6, Violations: []string{"spec defect"}, ViolationCount: 1, TotalCostUSD: 0.50},
	}
}

// ── PassRateByModelKind ──────────────────────────────────────────────────

func TestPassRateByModelKind(t *testing.T) {
	buckets := PassRateByModelKind(makeRecords())

	// Find the claude-sonnet / provider bucket.
	var cb *PassRateBucket
	for i := range buckets {
		if buckets[i].Model == "claude-sonnet" && buckets[i].SliceKind == "provider" {
			cb = &buckets[i]
			break
		}
	}
	if cb == nil {
		t.Fatal("expected a bucket for claude-sonnet / provider")
	}
	if cb.Pass != 2 {
		t.Errorf("Pass: want 2, got %d", cb.Pass)
	}
	if cb.Fail != 1 {
		t.Errorf("Fail: want 1 (S07), got %d", cb.Fail)
	}
	if cb.Blocked != 0 {
		t.Errorf("Blocked: want 0, got %d", cb.Blocked)
	}
	if cb.Total != 3 {
		t.Errorf("Total: want 3, got %d", cb.Total)
	}
	// 2 pass / 3 total = 66.7%
	if cb.PassRate < 0.66 || cb.PassRate > 0.67 {
		t.Errorf("PassRate: want ~0.667, got %.3f", cb.PassRate)
	}

	// claude-sonnet / harness
	foundHarness := false
	for i := range buckets {
		if buckets[i].Model == "claude-sonnet" && buckets[i].SliceKind == "harness" {
			foundHarness = true
			if buckets[i].Pass != 0 {
				t.Errorf("harness Pass: want 0, got %d", buckets[i].Pass)
			}
			if buckets[i].Fail != 1 {
				t.Errorf("harness Fail: want 1, got %d", buckets[i].Fail)
			}
		}
	}
	if !foundHarness {
		t.Error("expected claude-sonnet / harness bucket")
	}

	// gpt-5 / memory
	var gb *PassRateBucket
	for i := range buckets {
		if buckets[i].Model == "gpt-5" && buckets[i].SliceKind == "memory" {
			gb = &buckets[i]
			break
		}
	}
	if gb == nil {
		t.Fatal("expected a bucket for gpt-5 / memory")
	}
	if gb.Pass != 1 {
		t.Errorf("Pass: want 1, got %d", gb.Pass)
	}
	if gb.Fail != 1 {
		t.Errorf("Fail: want 1, got %d", gb.Fail)
	}
	if gb.Total != 2 {
		t.Errorf("Total: want 2, got %d", gb.Total)
	}
}

func TestPassRateByModelKind_Empty(t *testing.T) {
	buckets := PassRateByModelKind(nil)
	if len(buckets) != 0 {
		t.Errorf("empty input: want 0 buckets, got %d", len(buckets))
	}
}

func TestPassRateByModelKind_Sorting(t *testing.T) {
	buckets := PassRateByModelKind(makeRecords())
	for i := 1; i < len(buckets); i++ {
		prev := buckets[i-1]
		curr := buckets[i]
		if prev.Model > curr.Model {
			t.Errorf("sorting: model %s before %s", prev.Model, curr.Model)
		}
		if prev.Model == curr.Model && prev.SliceKind > curr.SliceKind {
			t.Errorf("sorting: kind %s before %s (same model)", prev.SliceKind, curr.SliceKind)
		}
	}
}

// ── AttemptsToPass ───────────────────────────────────────────────────────

func TestAttemptsToPass(t *testing.T) {
	buckets := AttemptsToPass(makeRecords())

	// Expected: attempt=1 count=1 (S01), attempt=2 count=1 (S02), attempt=3 count=1 (S04)
	byAttempt := make(map[int]int)
	for _, b := range buckets {
		byAttempt[b.Attempts] = b.Count
	}
	if byAttempt[1] != 1 {
		t.Errorf("attempt 1: want 1, got %d", byAttempt[1])
	}
	if byAttempt[2] != 1 {
		t.Errorf("attempt 2: want 1, got %d", byAttempt[2])
	}
	if byAttempt[3] != 1 {
		t.Errorf("attempt 3: want 1, got %d", byAttempt[3])
	}
}

func TestAttemptsToPass_Empty(t *testing.T) {
	buckets := AttemptsToPass(nil)
	if len(buckets) != 0 {
		t.Errorf("empty input: want 0 buckets, got %d", len(buckets))
	}
}

func TestAttemptsToPass_SkipsZeroAttempt(t *testing.T) {
	// A PASS record with attempt=0 is skipped.
	records := []Record{
		{V: 1, Ts: "2026-01-01T00:00:00Z", SliceID: "S99", Model: "m", Attempt: 0, Verdict: "pass"},
	}
	buckets := AttemptsToPass(records)
	if len(buckets) != 0 {
		t.Errorf("attempt=0 should be skipped, got %d buckets", len(buckets))
	}
}

// ── GateFailureHistogram ─────────────────────────────────────────────────

func TestGateFailureHistogram(t *testing.T) {
	buckets := GateFailureHistogram(makeRecords())

	// "missing proof bundle" appears in S03 and S05 → count 2
	// "unreachable test" appears in S03 → count 1
	// "spec defect" appears in S07 → count 1
	byV := make(map[string]int)
	for _, b := range buckets {
		byV[b.Violation] = b.Count
	}
	if byV["missing proof bundle"] != 2 {
		t.Errorf("'missing proof bundle': want 2, got %d", byV["missing proof bundle"])
	}
	if byV["unreachable test"] != 1 {
		t.Errorf("'unreachable test': want 1, got %d", byV["unreachable test"])
	}
	if byV["spec defect"] != 1 {
		t.Errorf("'spec defect': want 1, got %d", byV["spec defect"])
	}

	// First bucket should be the most common violation.
	if len(buckets) > 0 && buckets[0].Violation != "missing proof bundle" {
		t.Errorf("first bucket should be 'missing proof bundle' (count 2), got %q (count %d)", buckets[0].Violation, buckets[0].Count)
	}
}

func TestGateFailureHistogram_Empty(t *testing.T) {
	buckets := GateFailureHistogram(nil)
	if len(buckets) != 0 {
		t.Errorf("empty input: want 0 buckets, got %d", len(buckets))
	}
}

func TestGateFailureHistogram_OnlyPasses(t *testing.T) {
	records := []Record{
		{V: 1, Ts: "2026-01-01T00:00:00Z", SliceID: "S01", Verdict: "pass"},
	}
	buckets := GateFailureHistogram(records)
	if len(buckets) != 0 {
		t.Errorf("pass-only: want 0 buckets, got %d", len(buckets))
	}
}

// ── CostPerPassingSlice ──────────────────────────────────────────────────

func TestCostPerPassingSlice(t *testing.T) {
	buckets := CostPerPassingSlice(makeRecords())

	// claude-sonnet / provider: 3 records (2 pass, 1 fail), total cost = 0.42+0.38+0.50 = 1.30
	var cb *CostPerPassBucket
	for i := range buckets {
		if buckets[i].Model == "claude-sonnet" && buckets[i].SliceKind == "provider" {
			cb = &buckets[i]
			break
		}
	}
	if cb == nil {
		t.Fatal("expected cost bucket for claude-sonnet / provider")
	}
	if cb.PassCount != 2 {
		t.Errorf("PassCount: want 2, got %d", cb.PassCount)
	}
	if cb.TotalCost < 1.29 || cb.TotalCost > 1.31 {
		t.Errorf("TotalCost: want ~1.30, got %.4f", cb.TotalCost)
	}
	if cb.MeanCost < 0.43 || cb.MeanCost > 0.44 {
		t.Errorf("MeanCost: want ~0.4333, got %.4f", cb.MeanCost)
	}

	// Check sorting: model ascending, then kind ascending.
	for i := 1; i < len(buckets); i++ {
		if buckets[i].Model < buckets[i-1].Model {
			t.Errorf("CostPerPass: sorting violation at %d: %s before %s", i, buckets[i-1].Model, buckets[i].Model)
		}
	}
}

func TestCostPerPassingSlice_Empty(t *testing.T) {
	buckets := CostPerPassingSlice(nil)
	if len(buckets) != 0 {
		t.Errorf("empty input: want 0 buckets, got %d", len(buckets))
	}
}

// ── CaptainMissRate ──────────────────────────────────────────────────────

func TestCaptainMissRate_NoCaptainDispatches(t *testing.T) {
	records := makeRecords() // no Dispatches set → rate should be 0
	rate := CaptainMissRate(records)
	if rate != 0 {
		t.Errorf("no captain dispatches: want 0, got %.3f", rate)
	}
}

func TestCaptainMissRate_WithDispatches(t *testing.T) {
	// Two records with captain dispatches, one fail.
	records := []Record{
		{
			SliceID: "S01", Verdict: "pass", Model: "m1",
			Dispatches: []state.Dispatch{
				{Role: "captain", Model: "claude", CostUSD: 0.10, Attempt: 1},
			},
		},
		{
			SliceID: "S02", Verdict: "fail", Model: "m1",
			Dispatches: []state.Dispatch{
				{Role: "captain", Model: "claude", CostUSD: 0.10, Attempt: 1},
			},
		},
	}
	rate := CaptainMissRate(records)
	if rate != 0.5 {
		t.Errorf("captain miss rate: want 0.5, got %.3f", rate)
	}
}

func TestCaptainMissRate_BlockedCountsAsMiss(t *testing.T) {
	records := []Record{
		{
			SliceID: "S01", Verdict: "blocked", Model: "m1",
			Dispatches: []state.Dispatch{
				{Role: "captain", Model: "claude", CostUSD: 0.10, Attempt: 1},
			},
		},
	}
	rate := CaptainMissRate(records)
	if rate != 1.0 {
		t.Errorf("blocked should count as miss: want 1.0, got %.3f", rate)
	}
}
// ── VerifierOverturnRate ─────────────────────────────────────────────────

func TestVerifierOverturnRate_NoMultiRecordSlices(t *testing.T) {
	records := makeRecords() // one record per SliceID
	rate := VerifierOverturnRate(records)
	if rate != 0 {
		t.Errorf("single-verdict slices: want 0, got %.3f", rate)
	}
}

func TestVerifierOverturnRate_WithOverturn(t *testing.T) {
	records := []Record{
		{SliceID: "S01", Verdict: "fail", Model: "m1", Ts: "2026-01-01T00:00:00Z"},
		{SliceID: "S01", Verdict: "pass", Model: "m1", Ts: "2026-01-02T00:00:00Z"},
		{SliceID: "S02", Verdict: "pass", Model: "m1", Ts: "2026-01-03T00:00:00Z"},
		{SliceID: "S02", Verdict: "pass", Model: "m1", Ts: "2026-01-04T00:00:00Z"},
	}
	rate := VerifierOverturnRate(records)
	if rate != 0.5 {
		t.Errorf("S01 overturned, S02 stable: want 0.5, got %.3f", rate)
	}
}

// ── PerRoleQualityAll ────────────────────────────────────────────────────

func TestPerRoleQualityAll_EmptyDispatches(t *testing.T) {
	pqs := PerRoleQualityAll(makeRecords())
	// Should still return captain + verifier entries with zero sample.
	if len(pqs) < 2 {
		t.Errorf("expected at least 2 roles, got %d", len(pqs))
	}
	foundCaptain := false
	foundVerifier := false
	for _, pq := range pqs {
		if pq.Role == "captain" {
			foundCaptain = true
		}
		if pq.Role == "verifier" {
			foundVerifier = true
		}
	}
	if !foundCaptain || !foundVerifier {
		t.Errorf("missing required roles: captain=%v verifier=%v", foundCaptain, foundVerifier)
	}
}

// ── Load ─────────────────────────────────────────────────────────────────

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("want 0 records, got %d", len(records))
	}
}

func TestLoad_MissingFile(t *testing.T) {
	records, err := Load("/nonexistent/path/verdicts.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Errorf("missing file: want nil, got %d records", len(records))
	}
}

func TestLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corpus.jsonl")

	r1 := Record{V: 1, Ts: "2026-01-01T00:00:00Z", Release: "r", Track: "T5-providers", SliceID: "S01", SliceKind: "provider", Model: "claude", Attempt: 1, Verdict: "pass", GateCount: 5}
	r2 := Record{V: 1, Ts: "2026-01-02T00:00:00Z", Release: "r", Track: "T8-memory", SliceID: "S02", SliceKind: "memory", Model: "gpt-5", Attempt: 3, Verdict: "fail", GateCount: 3, Violations: []string{"bad test"}, ViolationCount: 1}

	if err := Append(path, r1); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, r2); err != nil {
		t.Fatal(err)
	}

	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}
	if records[0].SliceID != "S01" {
		t.Errorf("record 0 SliceID: want S01, got %s", records[0].SliceID)
	}
	if records[1].SliceID != "S02" {
		t.Errorf("record 1 SliceID: want S02, got %s", records[1].SliceID)
	}
}

func TestLoad_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corpus.jsonl")
	content := "this is not json\n{\"v\":1,\"ts\":\"2026-01-01T00:00:00Z\",\"slice_id\":\"S01\",\"verdict\":\"pass\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Should get 1 record (skipped the malformed first line).
	if len(records) != 1 {
		t.Fatalf("want 1 record (skipping malformed line), got %d", len(records))
	}
	if records[0].SliceID != "S01" {
		t.Errorf("SliceID: want S01, got %s", records[0].SliceID)
	}
}

// ── Report rendering (smoke test) ────────────────────────────────────────

func TestReport_Render(t *testing.T) {
	var sb strings.Builder
	var r Report
	r.Render(&sb, makeRecords())

	out := sb.String()

	// Should mention each section.
	for _, want := range []string{
		"Pass-rate by model",
		"Attempts to pass",
		"Gate-failure histogram",
		"Per-role quality",
		"claude-sonnet",
		"gpt-5",
		"provider",
		"harness",
		"memory",
		"commercial",
		"total cost",
		"COST/EA",
		"MISS_RATE",
		"OVERTURN_RATE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestReport_RenderEmpty(t *testing.T) {
	var sb strings.Builder
	var r Report
	r.Render(&sb, nil)

	out := sb.String()
	if !strings.Contains(out, "No verdict records") {
		t.Errorf("empty corpus: expected 'No verdict records', got: %s", out)
	}
}