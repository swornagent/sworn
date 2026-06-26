package bench

import (
	"testing"

	"github.com/swornagent/sworn/internal/verdict"
)

func TestIsSafeHosted(t *testing.T) {
	tests := []struct {
		entry ModelEntry
		want  bool
	}{
		{ModelEntry{Provider: "openai", ModelID: "openai/gpt-4.1"}, true},
		{ModelEntry{Provider: "openai", ModelID: "openai/gpt-4o-mini"}, true},
		{ModelEntry{Provider: "anthropic", ModelID: "anthropic/claude-sonnet"}, false},
		{ModelEntry{Provider: "", ModelID: "no-provider"}, false},
	}
	for _, tc := range tests {
		got := IsSafeHosted(tc.entry)
		if got != tc.want {
			t.Errorf("IsSafeHosted(%q) = %v, want %v", tc.entry.ModelID, got, tc.want)
		}
	}
}

func TestSelectDefault(t *testing.T) {
	models := []ModelEntry{
		{ModelID: "openai/gpt-4.1", Provider: "openai"},
		{ModelID: "openai/gpt-4.1-mini", Provider: "openai"},
		{ModelID: "openai/gpt-4.1-nano", Provider: "openai"},
		{ModelID: "anthropic/claude", Provider: "anthropic"}, // not safe-hosted
	}
	taskNames := []string{"S01", "S02"}

	t.Run("highest pass-rate wins", func(t *testing.T) {
		cells := []CellResult{
			{ModelID: "openai/gpt-4.1", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.01},
			{ModelID: "openai/gpt-4.1", TaskName: "S02", Verdict: verdict.Pass, CostUSD: 0.01},
			{ModelID: "openai/gpt-4.1-mini", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.005},
			{ModelID: "openai/gpt-4.1-mini", TaskName: "S02", Verdict: verdict.Fail, CostUSD: 0.005},
		}
		got, err := SelectDefault(models, cells, taskNames)
		if err != nil {
			t.Fatal(err)
		}
		if got != "openai/gpt-4.1" {
			t.Errorf("SelectDefault = %q, want openai/gpt-4.1 (higher pass-rate)", got)
		}
	})

	t.Run("tie goes to lower cost", func(t *testing.T) {
		cells := []CellResult{
			{ModelID: "openai/gpt-4.1", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.10},
			{ModelID: "openai/gpt-4.1", TaskName: "S02", Verdict: verdict.Pass, CostUSD: 0.10},
			{ModelID: "openai/gpt-4.1-mini", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.01},
			{ModelID: "openai/gpt-4.1-mini", TaskName: "S02", Verdict: verdict.Pass, CostUSD: 0.01},
		}
		got, err := SelectDefault(models, cells, taskNames)
		if err != nil {
			t.Fatal(err)
		}
		if got != "openai/gpt-4.1-mini" {
			t.Errorf("SelectDefault = %q, want openai/gpt-4.1-mini (same pass-rate, lower cost)", got)
		}
	})

	t.Run("non-safe-hosted excluded", func(t *testing.T) {
		// anthropic has perfect pass-rate but is not safe-hosted.
		// Only gpt-4.1-nano has safe-hosted results (50% pass-rate).
		cells := []CellResult{
			{ModelID: "anthropic/claude", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.001},
			{ModelID: "anthropic/claude", TaskName: "S02", Verdict: verdict.Pass, CostUSD: 0.001},
			{ModelID: "openai/gpt-4.1-nano", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.001},
			{ModelID: "openai/gpt-4.1-nano", TaskName: "S02", Verdict: verdict.Fail, CostUSD: 0.002},
		}
		got, err := SelectDefault(models, cells, taskNames)
		if err != nil {
			t.Fatal(err)
		}
		if got != "openai/gpt-4.1-nano" {
			t.Errorf("SelectDefault = %q, want openai/gpt-4.1-nano (only safe-hosted with results)", got)
		}
	})
	t.Run("tie-break: fewest non-pass cells", func(t *testing.T) {
		cells := []CellResult{
			{ModelID: "openai/gpt-4.1", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.01},
			{ModelID: "openai/gpt-4.1", TaskName: "S02", Verdict: verdict.Fail, CostUSD: 0.01},
			{ModelID: "openai/gpt-4.1-mini", TaskName: "S01", Verdict: verdict.Fail, CostUSD: 0.01},
			{ModelID: "openai/gpt-4.1-mini", TaskName: "S02", Verdict: verdict.Fail, CostUSD: 0.01},
		}
		got, err := SelectDefault(models, cells, taskNames)
		if err != nil {
			t.Fatal(err)
		}
		if got != "openai/gpt-4.1" {
			t.Errorf("SelectDefault = %q, want openai/gpt-4.1 (same pass-rate+same cost, fewer non-pass)", got)
		}
	})

	t.Run("no safe-hosted results is error", func(t *testing.T) {
		cells := []CellResult{
			{ModelID: "anthropic/claude", TaskName: "S01", Verdict: verdict.Pass, CostUSD: 0.01},
		}
		_, err := SelectDefault(models, cells, taskNames)
		if err == nil {
			t.Error("SelectDefault should error when no safe-hosted model has results")
		}
	})
}
