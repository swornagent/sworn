package ledger

import (
	"testing"
)

// makeEntries builds a corpus of Record literals for routing tests.
// Every record is a terminal verdict (PASS/FAIL/BLOCKED) with
// the model, slice_kind, and attempt fields set.
func makeEntries(entries ...struct {
	model   string
	kind    string
	verdict string
	attempt int
}) []Record {
	var out []Record
	for _, e := range entries {
		out = append(out, Record{
			Model:     e.model,
			SliceKind: e.kind,
			Verdict:   e.verdict,
			Attempt:   e.attempt,
		})
	}
	return out
}

// makeEntriesWithCost is like makeEntries but also sets TotalCostUSD.
func makeEntriesWithCost(entries ...struct {
	model   string
	kind    string
	verdict string
	attempt int
	cost    float64
}) []Record {
	var out []Record
	for _, e := range entries {
		out = append(out, Record{
			Model:        e.model,
			SliceKind:    e.kind,
			Verdict:      e.verdict,
			Attempt:      e.attempt,
			TotalCostUSD: e.cost,
		})
	}
	return out
}

func TestRecommendModel_RanksByPassRate(t *testing.T) {
	// Model A: 9 pass, 1 fail → 90% pass-rate
	// Model B: 3 pass, 7 fail → 30% pass-rate
	// Both >= MinSampleSize (10)
	records := makeEntries( // Model A — harness
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "fail", 2},
		// Model B — harness
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 2},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
	)

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeQuality, 0)
	if !ok {
		t.Fatal("expected confident recommendation")
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("got model %q, want openai/gpt-4o", rec.Model)
	}
	if rec.PassRate < 0.89 || rec.PassRate > 0.91 {
		t.Errorf("got pass-rate %.2f, want ~0.90", rec.PassRate)
	}
	if rec.Sample != 10 {
		t.Errorf("got sample %d, want 10", rec.Sample)
	}
}

func TestRecommendModel_TieBreakByAttempts(t *testing.T) {
	// Both models have same pass-rate (5/10 = 50%) and same sample size.
	// Model A has best attempt = 1, Model B has best attempt = 3.
	// Model A should win (fewer attempts).
	records := makeEntries( // Model A
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o-mini", "provider", "fail", 2},
		// Model B
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
	)

	rec, ok := RecommendModel(records, "implementer", "provider", OptimizeQuality, 0)
	if !ok {
		t.Fatal("expected confident recommendation")
	}
	if rec.Model != "openai/gpt-4o-mini" {
		t.Errorf("got model %q, want openai/gpt-4o-mini (fewer attempts)", rec.Model)
	}
}

func TestRecommendModel_BelowMinSample(t *testing.T) {
	// Only 4 records for the kind — below MinSampleSize (5).
	records := makeEntries(
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "memory", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "memory", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "memory", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "memory", "fail", 2},
	)

	_, ok := RecommendModel(records, "implementer", "memory", OptimizeQuality, 0)
	if ok {
		t.Fatal("expected no recommendation below MinSampleSize")
	}
}

func TestRecommendModel_NoRecordsForKind(t *testing.T) {
	records := makeEntries(
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
	)

	// Ask for a kind with zero records.
	_, ok := RecommendModel(records, "implementer", "provider", OptimizeQuality, 0)
	if ok {
		t.Fatal("expected no recommendation for kind with no records")
	}
}

func TestRecommendModel_EmptyRecords(t *testing.T) {
	_, ok := RecommendModel(nil, "implementer", "harness", OptimizeQuality, 0)
	if ok {
		t.Fatal("expected no recommendation from empty corpus")
	}
}

func TestRecommendModel_SkipsNonTerminalVerdicts(t *testing.T) {
	// Include a "pending" record — it must be skipped.
	records := []Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		// "pending" must NOT count toward sample size.
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pending", Attempt: 0},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pending", Attempt: 0},
	}

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeQuality, 0)
	if !ok {
		t.Fatal("expected confident recommendation — sample=5 (pending skipped)")
	}
	if rec.Sample != 5 {
		t.Errorf("got sample %d, want 5 (pending excluded)", rec.Sample)
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("got model %q, want openai/gpt-4o", rec.Model)
	}
}

// ── S56 cost-aware routing tests ──────────────────────────────────────────

func TestRecommendModel_OptimizeCost_PicksCheapest(t *testing.T) {
	// Model A: 9/10 pass at $0.50/slice → clears floor (0.9 ≥ 0.8)
	// Model B: 9/10 pass at $0.05/slice → clears floor, cheaper
	// OptimizeCost must return B.
	records := makeEntriesWithCost(
		// Model A — $0.50/slice average (10 records × $0.50 = $5.00 total)
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 2, 0.50},
		// Model B — $0.05/slice average (10 records × $0.05 = $0.50 total)
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "fail", 2, 0.05},
	)

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeCost, 0)
	if !ok {
		t.Fatal("expected confident recommendation in cost mode")
	}
	if rec.Model != "anthropic/claude-haiku-4-20250514" {
		t.Errorf("OptimizeCost: got model %q, want cheaper claude-haiku", rec.Model)
	}
	if rec.MeanCostUSD < 0.04 || rec.MeanCostUSD > 0.06 {
		t.Errorf("MeanCostUSD: got %.4f, want ~0.05", rec.MeanCostUSD)
	}
	if rec.Objective != OptimizeCost {
		t.Errorf("Objective: got %v, want OptimizeCost", rec.Objective)
	}
}

func TestRecommendModel_OptimizeCost_QualityFloorExcludesCheapest(t *testing.T) {
	// Model A: 9/10 pass at $0.50/slice → clears floor (0.9)
	// Model B: 3/10 pass at $0.05/slice → below floor (0.3 < 0.8)
	// Even though B is cheaper, quality floor must exclude it — return A.
	records := makeEntriesWithCost(
		// Model A — $0.50/slice, 90% pass
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 2, 0.50},
		// Model B — $0.05/slice, 30% pass (below floor)
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-3.5-turbo", "harness", "fail", 1, 0.05},
	)

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeCost, 0)
	if !ok {
		t.Fatal("expected confident recommendation — should fall through to quality")
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("floor gate: got model %q, want openai/gpt-4o (cheapest was below floor)", rec.Model)
	}
}

func TestRecommendModel_OptimizeCost_UnpricedExcluded(t *testing.T) {
	// Model A: 9/10 pass at $0.50/slice → clears floor
	// Model B: 9/10 pass at $0.00/slice → unpriced (no signal), must be excluded
	// Even though B has $0 cost, it must NOT be selected as "free".
	records := makeEntriesWithCost(
		// Model A — priced, 90% pass
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 2, 0.50},
		// Model B — unpriced (all cost 0), 90% pass
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "pass", 1, 0},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/o4-mini", "harness", "fail", 2, 0},
	)

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeCost, 0)
	if !ok {
		t.Fatal("expected confident recommendation")
	}
	if rec.Model == "openai/o4-mini" {
		t.Errorf("unpriced model must not be selected as 'free': got %q", rec.Model)
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("unpriced exclusion: got model %q, want priced openai/gpt-4o", rec.Model)
	}
}

func TestRecommendModel_OptimizeQuality_NoRegression(t *testing.T) {
	// S54 regression: OptimizeQuality must return same result as pre-S56
	// when given the same corpus. Uses the ranks-by-pass-rate test data.
	records := makeEntries(
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "pass", 1},
		struct {
			model, kind, verdict string
			attempt              int
		}{"openai/gpt-4o", "harness", "fail", 2},
	)

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeQuality, 0)
	if !ok {
		t.Fatal("expected confident recommendation in quality mode")
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("quality mode regression: got model %q, want openai/gpt-4o", rec.Model)
	}
	if rec.Objective != OptimizeQuality {
		t.Errorf("quality mode: Objective should be OptimizeQuality, got %v", rec.Objective)
	}
}

func TestRecommendModel_OptimizeCost_AllBelowFloorFallsBack(t *testing.T) {
	// Every priced model is below the pass-rate floor (0.8). Cost mode
	// must fall back to quality mode (best available), not return nothing.
	records := makeEntriesWithCost(
		// Model A — 50% pass (below floor)
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 1, 0.50},
	)

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeCost, 0)
	if !ok {
		t.Fatal("expected fallback to quality mode, not empty (risk: all-below-floor returns nothing)")
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("all-below-floor fallback: got model %q, want openai/gpt-4o (best available)", rec.Model)
	}
}

func TestRecommendModel_OptimizeCost_HigherFloorChangesPick(t *testing.T) {
	// Model A: 9/10 pass at $0.50/slice → pass-rate 0.9, clears floor 0.8 but NOT 0.95
	// Model B: 8/10 pass at $0.05/slice → pass-rate 0.8, cheaper but below 0.85 floor
	// With floor=0.85: A (0.9) clears, B (0.8) doesn't → A wins.
	// With floor=0.75: both clear floor, B cheaper → B wins.
	records := makeEntriesWithCost(
		// Model A — $0.50/slice, 90% pass
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 2, 0.50},
		// Model B — $0.05/slice, 80% pass
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "fail", 1, 0.05},
	)

	// Floor 0.85: B (0.80) excluded by floor → A wins
	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeCost, 0.85)
	if !ok {
		t.Fatal("floor=0.85: expected recommendation (A)")
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("floor=0.85: got %q, want openai/gpt-4o (B below floor)", rec.Model)
	}

	// Floor 0.75: both clear floor → B cheaper wins
	rec, ok = RecommendModel(records, "implementer", "harness", OptimizeCost, 0.75)
	if !ok {
		t.Fatal("floor=0.75: expected recommendation (B cheaper)")
	}
	if rec.Model != "openai/gpt-4o-mini" {
		t.Errorf("floor=0.75: got %q, want openai/gpt-4o-mini (cheaper and clears floor)", rec.Model)
	}
}

func TestRecommendModel_OptimizeBalanced(t *testing.T) {
	// Model A: 9/10 pass at $0.50/slice → pass-rate/dollar = 0.9/0.5 = 1.8
	// Model B: 7/10 pass at $0.05/slice → pass-rate/dollar = 0.7/0.05 = 14.0
	// Model C: 6/10 pass at $0.02/slice → pass-rate/dollar = 0.6/0.02 = 30.0
	// Balanced mode should pick C (highest pass-rate per dollar).
	records := makeEntriesWithCost(
		// Model A
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "pass", 1, 0.50},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o", "harness", "fail", 1, 0.50},
		// Model B
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "pass", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "fail", 1, 0.05},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"openai/gpt-4o-mini", "harness", "fail", 1, 0.05},
		// Model C
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "pass", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "fail", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "fail", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "fail", 1, 0.02},
		struct {
			model, kind, verdict string
			attempt              int
			cost                 float64
		}{"anthropic/claude-haiku-4-20250514", "harness", "fail", 1, 0.02},
	)

	rec, ok := RecommendModel(records, "implementer", "harness", OptimizeBalanced, 0)
	if !ok {
		t.Fatal("expected confident recommendation in balanced mode")
	}
	if rec.Model != "anthropic/claude-haiku-4-20250514" {
		t.Errorf("OptimizeBalanced: got model %q, want claude-haiku (best pass-rate/$)", rec.Model)
	}
	if rec.Objective != OptimizeBalanced {
		t.Errorf("Objective: got %v, want OptimizeBalanced", rec.Objective)
	}
}

func TestRecommendation_FieldsRoundTrip(t *testing.T) {
	rec := Recommendation{
		Model:       "openai/gpt-4o",
		PassRate:    0.85,
		Sample:      20,
		MeanCostUSD: 0.25,
		Objective:   OptimizeCost,
	}
	if rec.Model != "openai/gpt-4o" {
		t.Errorf("Model: got %q", rec.Model)
	}
	if rec.PassRate != 0.85 {
		t.Errorf("PassRate: got %f", rec.PassRate)
	}
	if rec.Sample != 20 {
		t.Errorf("Sample: got %d", rec.Sample)
	}
	if rec.MeanCostUSD != 0.25 {
		t.Errorf("MeanCostUSD: got %f", rec.MeanCostUSD)
	}
	if rec.Objective != OptimizeCost {
		t.Errorf("Objective: got %v", rec.Objective)
	}
}

func TestParseObjective(t *testing.T) {
	tests := []struct {
		input string
		want  Objective
	}{
		{"quality", OptimizeQuality},
		{"cost", OptimizeCost},
		{"balanced", OptimizeBalanced},
		{"unknown", OptimizeQuality},
		{"", OptimizeQuality},
		{"QUALITY", OptimizeQuality}, // case-sensitive, falls back
	}
	for _, tt := range tests {
		got := ParseObjective(tt.input)
		if got != tt.want {
			t.Errorf("ParseObjective(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestObjective_String(t *testing.T) {
	tests := []struct {
		obj  Objective
		want string
	}{
		{OptimizeQuality, "quality"},
		{OptimizeCost, "cost"},
		{OptimizeBalanced, "balanced"},
	}
	for _, tt := range tests {
		if got := tt.obj.String(); got != tt.want {
			t.Errorf("Objective(%d).String() = %q, want %q", tt.obj, got, tt.want)
		}
	}
}
