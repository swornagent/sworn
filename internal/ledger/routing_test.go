package ledger

import (
	"testing"
)

// makeEntries builds a corpus of Record literals for routing tests.
// Every record is a terminal verdict (PASS/FAIL/BLOCKED) with
// the model, slice_kind, and attempt fields set.
func makeEntries(entries ...struct {
	model     string
	kind      string
	verdict   string
	attempt   int
}) []Record {	var out []Record
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

func TestRecommendModel_RanksByPassRate(t *testing.T) {
	// Model A: 9 pass, 1 fail → 90% pass-rate
	// Model B: 3 pass, 7 fail → 30% pass-rate
	// Both >= MinSampleSize (10)
	records := makeEntries(		// Model A — harness
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "fail", 2},
		// Model B — harness
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 2},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "harness", "fail", 1},
	)

	rec, ok := RecommendModel(records, "harness")
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
	records := makeEntries(		// Model A
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "fail", 2},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o-mini", "provider", "fail", 2},
		// Model B
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "pass", 3},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
		struct{ model, kind, verdict string; attempt int }{"anthropic/claude-sonnet-4-20250514", "provider", "fail", 1},
	)

	rec, ok := RecommendModel(records, "provider")
	if !ok {
		t.Fatal("expected confident recommendation")
	}
	if rec.Model != "openai/gpt-4o-mini" {
		t.Errorf("got model %q, want openai/gpt-4o-mini (fewer attempts)", rec.Model)
	}
}

func TestRecommendModel_BelowMinSample(t *testing.T) {
	// Only 4 records for the kind — below MinSampleSize (5).
	records := makeEntries(		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "memory", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "memory", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "memory", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "memory", "fail", 2},
	)

	_, ok := RecommendModel(records, "memory")
	if ok {
		t.Fatal("expected no recommendation below MinSampleSize")
	}
}

func TestRecommendModel_NoRecordsForKind(t *testing.T) {
	records := makeEntries(		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
		struct{ model, kind, verdict string; attempt int }{"openai/gpt-4o", "harness", "pass", 1},
	)

	// Ask for a kind with zero records.
	_, ok := RecommendModel(records, "provider")
	if ok {
		t.Fatal("expected no recommendation for kind with no records")
	}
}

func TestRecommendModel_EmptyRecords(t *testing.T) {
	_, ok := RecommendModel(nil, "harness")
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

	rec, ok := RecommendModel(records, "harness")
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

func TestRecommendation_FieldsRoundTrip(t *testing.T) {
	rec := Recommendation{
		Model:    "openai/gpt-4o",
		PassRate: 0.85,
		Sample:   20,
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
}