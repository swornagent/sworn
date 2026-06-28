package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

// writeStatus writes a status.json file with the given dispatches.
func writeStatus(t *testing.T, path string, dispatches []state.Dispatch) {
	t.Helper()
	st := state.Status{
		SliceID: filepath.Base(filepath.Dir(path)),
		Release: filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path)))),
		Verification: state.Verification{
			Dispatches: dispatches,
		},
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	mustWrite(t, path, string(data))
}

func TestAggregateByModel_EmptyDispatches(t *testing.T) {
	reports := aggregateByModel(nil)
	if len(reports) != 0 {
		t.Errorf("expected 0 reports for nil input, got %d", len(reports))
	}
}

func TestAggregateByModel_SingleModel(t *testing.T) {
	dispatches := []state.Dispatch{
		{Model: "claude-sonnet-4-20250514", ModelIDConfirmed: "claude-sonnet-4-20250514", Attempt: 0, InputTokens: 100, OutputTokens: 50, DurationMS: 200, CostUSD: 0.01},
		{Model: "claude-sonnet-4-20250514", ModelIDConfirmed: "claude-sonnet-4-20250514", Attempt: 0, InputTokens: 200, OutputTokens: 100, DurationMS: 300, CostUSD: 0.02},
	}
	reports := aggregateByModel(dispatches)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	r := reports[0]
	if r.ModelID != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %s", r.ModelID)
	}
	if r.DispatchCount != 2 {
		t.Errorf("expected 2 dispatches, got %d", r.DispatchCount)
	}
	if r.ReworkRate != 0 {
		t.Errorf("expected 0%% rework rate, got %.1f%%", r.ReworkRate)
	}
	if r.MeanInputTok != 150 {
		t.Errorf("expected mean input 150, got %.1f", r.MeanInputTok)
	}
	if r.MeanOutputTok != 75 {
		t.Errorf("expected mean output 75, got %.1f", r.MeanOutputTok)
	}
	if r.MeanDurationMS != 250 {
		t.Errorf("expected mean duration 250, got %.1f", r.MeanDurationMS)
	}
	if r.TotalCostUSD != 0.03 {
		t.Errorf("expected total cost $0.03, got $%.4f", r.TotalCostUSD)
	}
}

func TestAggregateByModel_ReworkRate(t *testing.T) {
	dispatches := []state.Dispatch{
		{Model: "gpt-5", Attempt: 0},
		{Model: "gpt-5", Attempt: 1},
		{Model: "gpt-5", Attempt: 0},
		{Model: "gpt-5", Attempt: 2},
	}
	reports := aggregateByModel(dispatches)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	expectedRate := 50.0
	if reports[0].ReworkRate != expectedRate {
		t.Errorf("expected rework rate %.1f%%, got %.1f%%", expectedRate, reports[0].ReworkRate)
	}
}

func TestAggregateByModel_ZeroDurationExcluded(t *testing.T) {
	dispatches := []state.Dispatch{
		{Model: "claude", DurationMS: 0, InputTokens: 100, OutputTokens: 50},
		{Model: "claude", DurationMS: 500, InputTokens: 200, OutputTokens: 100},
	}
	reports := aggregateByModel(dispatches)
	if reports[0].MeanDurationMS != 500 {
		t.Errorf("expected mean duration 500 (0 excluded), got %.1f", reports[0].MeanDurationMS)
	}
	if reports[0].MeanInputTok != 150 {
		t.Errorf("expected mean input 150, got %.1f", reports[0].MeanInputTok)
	}
	if reports[0].MeanOutputTok != 75 {
		t.Errorf("expected mean output 75, got %.1f", reports[0].MeanOutputTok)
	}
}

func TestAggregateByModel_ModelIDConfirmedFallback(t *testing.T) {
	dispatches := []state.Dispatch{
		{Model: "alias", ModelIDConfirmed: "real-model-v1", Attempt: 0},
		{Model: "alias", ModelIDConfirmed: "", Attempt: 0},
	}
	reports := aggregateByModel(dispatches)
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports (different keys), got %d", len(reports))
	}
}

func TestAggregateByModel_EmptyModelGroupedUnderEmptyKey(t *testing.T) {
	// Empty-model dispatches are skipped in collectDispatches, not aggregateByModel.
	// When aggregateByModel receives them directly, they group under "".
	dispatches := []state.Dispatch{
		{Model: "", ModelIDConfirmed: ""},
		{Model: "claude", Attempt: 0},
	}
	reports := aggregateByModel(dispatches)
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports (empty key + claude), got %d", len(reports))
	}
}
func setupTelemetryFixture(t *testing.T, release string, slices map[string][]state.Dispatch) string {
	t.Helper()
	repoDir := t.TempDir()
	mustMkdir(t, filepath.Join(repoDir, ".git"))
	releaseDir := filepath.Join(repoDir, "docs", "release", release)
	mustMkdir(t, releaseDir)
	for sliceID, dispatches := range slices {
		sliceDir := filepath.Join(releaseDir, sliceID)
		mustMkdir(t, sliceDir)
		writeStatus(t, filepath.Join(sliceDir, "status.json"), dispatches)
	}
	return repoDir
}

func TestCollectDispatches_IntegratesWithTempDir(t *testing.T) {
	release := "test-release"
	slices := map[string][]state.Dispatch{
		"S01-alpha": {
			{Model: "claude-sonnet-4-20250514", Attempt: 0, InputTokens: 100, OutputTokens: 50, DurationMS: 200, CostUSD: 0.01},
		},
		"S02-beta": {
			{Model: "claude-sonnet-4-20250514", Attempt: 1, InputTokens: 300, OutputTokens: 150, DurationMS: 400, CostUSD: 0.03},
		},
	}
	repoDir := setupTelemetryFixture(t, release, slices)
	dispatches, err := collectDispatches(repoDir, release)
	if err != nil {
		t.Fatalf("collectDispatches: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected 2 dispatches, got %d", len(dispatches))
	}
}

func TestTelemetryReportIntegration(t *testing.T) {
	release := "2026-06-27-conformance-foundation"
	slices := map[string][]state.Dispatch{
		"S01-test-alpha": {
			{
				Model:            "claude-sonnet-4-20250514",
				ModelIDConfirmed: "claude-sonnet-4-20250514",
				Attempt:          0,
				InputTokens:      1000,
				OutputTokens:     500,
				DurationMS:       2000,
				CostUSD:          0.015,
			},
		},
		"S02-test-beta": {
			{
				Model:            "claude-sonnet-4-20250514",
				ModelIDConfirmed: "claude-sonnet-4-20250514",
				Attempt:          1,
				InputTokens:      2000,
				OutputTokens:     1000,
				DurationMS:       3000,
				CostUSD:          0.030,
			},
		},
	}
	repoDir := setupTelemetryFixture(t, release, slices)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Run("table output", func(t *testing.T) {
		dispatches, err := collectDispatches(repoDir, release)
		if err != nil {
			t.Fatalf("collectDispatches: %v", err)
		}
		if len(dispatches) != 2 {
			t.Fatalf("expected 2 dispatches, got %d", len(dispatches))
		}

		reports := aggregateByModel(dispatches)
		if len(reports) != 1 {
			t.Fatalf("expected 1 model report, got %d", len(reports))
		}

		r := reports[0]
		if r.ModelID != "claude-sonnet-4-20250514" {
			t.Errorf("ModelID: expected claude-sonnet-4-20250514, got %s", r.ModelID)
		}
		if r.DispatchCount != 2 {
			t.Errorf("DispatchCount: expected 2, got %d", r.DispatchCount)
		}
		if r.ReworkRate != 50.0 {
			t.Errorf("ReworkRate: expected 50.0%%, got %.1f%%", r.ReworkRate)
		}
		if r.MeanInputTok != 1500 {
			t.Errorf("MeanInputTok: expected 1500, got %.1f", r.MeanInputTok)
		}
		if r.MeanOutputTok != 750 {
			t.Errorf("MeanOutputTok: expected 750, got %.1f", r.MeanOutputTok)
		}
		if r.MeanDurationMS != 2500 {
			t.Errorf("MeanDurationMS: expected 2500, got %.1f", r.MeanDurationMS)
		}
		if r.TotalCostUSD != 0.045 {
			t.Errorf("TotalCostUSD: expected 0.045, got %.4f", r.TotalCostUSD)
		}
	})

	t.Run("json output", func(t *testing.T) {
		dispatches, _ := collectDispatches(repoDir, release)
		reports := aggregateByModel(dispatches)

		var buf bytes.Buffer
		r, w, _ := os.Pipe()
		oldStdout := os.Stdout
		os.Stdout = w
		exitCode := outputJSON(reports)
		w.Close()
		os.Stdout = oldStdout
		buf.ReadFrom(r)

		if exitCode != 0 {
			t.Fatalf("outputJSON returned %d", exitCode)
		}

		var rows []struct {
			Model            string   `json:"model"`
			Dispatches       int      `json:"dispatches"`
			ReworkRate       float64  `json:"rework_rate_pct"`
			MeanInputTokens  *float64 `json:"mean_input_tokens"`
			MeanOutputTokens *float64 `json:"mean_output_tokens"`
			MeanDurationMS   *float64 `json:"mean_duration_ms"`
			TotalCostUSD     float64  `json:"total_cost_usd"`
		}
		if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
			t.Fatalf("json unmarshal: %v\noutput: %s", err, buf.String())
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 JSON row, got %d", len(rows))
		}
		row := rows[0]
		if row.Model != "claude-sonnet-4-20250514" {
			t.Errorf("model: expected claude-sonnet-4-20250514, got %s", row.Model)
		}
		if row.Dispatches != 2 {
			t.Errorf("dispatches: expected 2, got %d", row.Dispatches)
		}
		if row.TotalCostUSD != 0.045 {
			t.Errorf("total_cost_usd: expected 0.045, got %.4f", row.TotalCostUSD)
		}
		if row.MeanDurationMS == nil || *row.MeanDurationMS != 2500 {
			t.Errorf("mean_duration_ms: expected 2500, got %v", row.MeanDurationMS)
		}
	})

	t.Run("zero duration excluded", func(t *testing.T) {
		slices2 := map[string][]state.Dispatch{
			"S03-gamma": {
				{Model: "claude", DurationMS: 0, InputTokens: 100, OutputTokens: 50},
			},
		}
		repoDir2 := setupTelemetryFixture(t, release+"-zero", slices2)
		dispatches, _ := collectDispatches(repoDir2, release+"-zero")
		reports := aggregateByModel(dispatches)
		if len(reports) != 1 {
			t.Fatalf("expected 1 report, got %d", len(reports))
		}
		if reports[0].MeanDurationMS != 0 {
			t.Errorf("expected mean duration 0 (all zeros excluded), got %.1f", reports[0].MeanDurationMS)
		}
	})

	t.Run("two slices one dispatch each", func(t *testing.T) {
		dispatches, _ := collectDispatches(repoDir, release)
		reports := aggregateByModel(dispatches)
		if len(reports) != 1 {
			t.Errorf("AC5: expected 1 model report, got %d", len(reports))
		}
		if reports[0].DispatchCount != 2 {
			t.Errorf("AC5: expected 2 total dispatches, got %d", reports[0].DispatchCount)
		}
	})
}

func TestTelemetryReportNoDispatches(t *testing.T) {
	release := "empty-release"
	slices := map[string][]state.Dispatch{
		"S01-empty": {},
	}
	repoDir := setupTelemetryFixture(t, release, slices)
	dispatches, err := collectDispatches(repoDir, release)
	if err != nil {
		t.Fatalf("collectDispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Errorf("expected 0 dispatches, got %d", len(dispatches))
	}
}

func TestTelemetryReport_ReworkRateFromAttempt(t *testing.T) {
	dispatches := []state.Dispatch{
		{Model: "test-model", Attempt: 0},
		{Model: "test-model", Attempt: 0},
		{Model: "test-model", Attempt: 1},
		{Model: "test-model", Attempt: 1},
		{Model: "test-model", Attempt: 3},
	}
	reports := aggregateByModel(dispatches)
	if reports[0].ReworkRate != 60.0 {
		t.Errorf("expected rework rate 60%% (3/5), got %.1f%%", reports[0].ReworkRate)
	}
}

func TestFormatVal(t *testing.T) {
	if v := formatVal(0); v != "0.0" {
		t.Errorf("expected '0.0', got %q", v)
	}
	if v := formatVal(1.5); v != "1.5" {
		t.Errorf("expected '1.5', got %q", v)
	}
	if v := formatVal(math.NaN()); v != "—" {
		t.Errorf("expected '—' for NaN, got %q", v)
	}
}

func TestOutputTable(t *testing.T) {
	reports := []modelReport{
		{ModelID: "test", DispatchCount: 1, ReworkRate: 0, TotalCostUSD: 0.01},
	}

	var buf bytes.Buffer
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w
	outputTable(reports)
	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	out := buf.String()
	required := []string{"MODEL", "DISP", "REWORK%", "MEAN_IN_TOK", "MEAN_OUT_TOK", "MEAN_DUR_MS", "COST_USD"}
	for _, col := range required {
		if !strings.Contains(out, col) {
			t.Errorf("output missing column %q:\n%s", col, out)
		}
	}
}
