package bench

import (
	"strings"
	"testing"
)

// TestOverclaimRateCalculation: fixture with 4 known overclaims out of 12 slices;
// assert calculated rate is 4/12 = 33.3%; assert underclaim calculation is correct.
func TestOverclaimRateCalculation(t *testing.T) {
	// 12 slices: 8 PASS, 4 FAIL.
	// Incorrect verifier: always returns PASS.
	// Overclaims: 4 (FAIL slices where verifier said PASS).
	// Underclaims: 0 (no PASS slices where verifier said FAIL).
	records := make([]SliceRecord, 12)
	for i := 0; i < 8; i++ {
		records[i] = SliceRecord{
			SliceID:          "S-pass",
			GroundTruth:      GroundTruthPass,
			SimulatedVerdict: SimulatedPass, // correct
		}
	}
	for i := 8; i < 12; i++ {
		records[i] = SliceRecord{
			SliceID:          "S-fail",
			GroundTruth:      GroundTruthFail,
			SimulatedVerdict: SimulatedPass, // incorrect → overclaim
		}
	}

	result := computeRates(records, 1)

	if result.OverclaimCount != 4 {
		t.Errorf("overclaim count: got %d, want 4", result.OverclaimCount)
	}
	if result.UnderclaimCount != 0 {
		t.Errorf("underclaim count: got %d, want 0", result.UnderclaimCount)
	}

	// Rate = 4/12 = 0.3333... = 33.3%
	expectedRate := 4.0 / 12.0
	if result.OverclaimRate != expectedRate {
		t.Errorf("overclaim rate: got %.4f, want %.4f", result.OverclaimRate, expectedRate)
	}
	if result.UnderclaimRate != 0.0 {
		t.Errorf("underclaim rate: got %.4f, want 0.0", result.UnderclaimRate)
	}
}

// TestUnderclaimRateCalculation: 2 underclaims out of 12 slices.
func TestUnderclaimRateCalculation(t *testing.T) {
	records := make([]SliceRecord, 12)
	for i := 0; i < 8; i++ {
		records[i] = SliceRecord{
			GroundTruth:      GroundTruthPass,
			SimulatedVerdict: SimulatedPass,
		}
	}
	// 2 of the 8 PASS slices get FAIL verdicts (underclaims).
	records[0].SimulatedVerdict = SimulatedFail
	records[1].SimulatedVerdict = SimulatedFail
	for i := 8; i < 12; i++ {
		records[i] = SliceRecord{
			GroundTruth:      GroundTruthFail,
			SimulatedVerdict: SimulatedFail,
		}
	}

	result := computeRates(records, 1)

	if result.UnderclaimCount != 2 {
		t.Errorf("underclaim count: got %d, want 2", result.UnderclaimCount)
	}
	expectedRate := 2.0 / 12.0
	if result.UnderclaimRate != expectedRate {
		t.Errorf("underclaim rate: got %.4f, want %.4f", result.UnderclaimRate, expectedRate)
	}
	if result.OverclaimCount != 0 {
		t.Errorf("overclaim count: got %d, want 0", result.OverclaimCount)
	}
}

// TestBenchmarkDeterministic: run the benchmark harness twice; assert identical
// results (proves no randomness or race).
func TestBenchmarkDeterministic(t *testing.T) {
	report1, err := RunOverclaimBenchmark()
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	report2, err := RunOverclaimBenchmark()
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if len(report1.Results) != len(report2.Results) {
		t.Fatalf("result count mismatch: %d vs %d", len(report1.Results), len(report2.Results))
	}

	for i := range report1.Results {
		r1 := report1.Results[i]
		r2 := report2.Results[i]
		if r1.N != r2.N {
			t.Errorf("run %d: N mismatch: %d vs %d", i, r1.N, r2.N)
		}
		if r1.OverclaimCount != r2.OverclaimCount {
			t.Errorf("N=%d: overclaim count mismatch: %d vs %d", r1.N, r1.OverclaimCount, r2.OverclaimCount)
		}
		if r1.UnderclaimCount != r2.UnderclaimCount {
			t.Errorf("N=%d: underclaim count mismatch: %d vs %d", r1.N, r1.UnderclaimCount, r2.UnderclaimCount)
		}
		if r1.OverclaimRate != r2.OverclaimRate {
			t.Errorf("N=%d: overclaim rate mismatch: %.4f vs %.4f", r1.N, r1.OverclaimRate, r2.OverclaimRate)
		}
		if r1.UnderclaimRate != r2.UnderclaimRate {
			t.Errorf("N=%d: underclaim rate mismatch: %.4f vs %.4f", r1.N, r1.UnderclaimRate, r2.UnderclaimRate)
		}
		if r1.Runs != r2.Runs {
			t.Errorf("N=%d: runs mismatch: %d vs %d", r1.N, r1.Runs, r2.Runs)
		}
	}
}

// TestZeroOverclaimWithCorrectGate: fixture where verifier always returns correct
// verdict; assert overclaim_rate == 0.0 at all N values.
func TestZeroOverclaimWithCorrectGate(t *testing.T) {
	report, err := RunOverclaimBenchmark()
	if err != nil {
		t.Fatalf("RunOverclaimBenchmark: %v", err)
	}

	if len(report.Results) != 3 {
		t.Fatalf("expected 3 results (N=1,2,4), got %d", len(report.Results))
	}

	expectedN := []int{1, 2, 4}
	for i, res := range report.Results {
		if res.N != expectedN[i] {
			t.Errorf("result %d: N = %d, want %d", i, res.N, expectedN[i])
		}
		if res.OverclaimCount != 0 {
			t.Errorf("N=%d: overclaim count = %d, want 0", res.N, res.OverclaimCount)
		}
		if res.OverclaimRate != 0.0 {
			t.Errorf("N=%d: overclaim rate = %.4f, want 0.0", res.N, res.OverclaimRate)
		}
		if res.UnderclaimCount != 0 {
			t.Errorf("N=%d: underclaim count = %d, want 0", res.N, res.UnderclaimCount)
		}
		if res.UnderclaimRate != 0.0 {
			t.Errorf("N=%d: underclaim rate = %.4f, want 0.0", res.N, res.UnderclaimRate)
		}
		if res.Runs != 5 {
			t.Errorf("N=%d: runs = %d, want 5", res.N, res.Runs)
		}
	}
}

// TestFormatMarkdownTable verifies the Markdown table has rows for N=1,2,4
// and the required columns.
func TestFormatMarkdownTable(t *testing.T) {
	report := &OverclaimReport{
		Results: []OverclaimResult{
			{N: 1, Runs: 5, OverclaimCount: 0, UnderclaimCount: 0, OverclaimRate: 0, UnderclaimRate: 0},
			{N: 2, Runs: 5, OverclaimCount: 0, UnderclaimCount: 0, OverclaimRate: 0, UnderclaimRate: 0},
			{N: 4, Runs: 5, OverclaimCount: 0, UnderclaimCount: 0, OverclaimRate: 0, UnderclaimRate: 0},
		},
	}

	md := FormatMarkdownTable(report)

	// Must contain rows for N=1, N=2, N=4.
	for _, n := range []int{1, 2, 4} {
		needle := "| 1 |"
		if n == 2 {
			needle = "| 2 |"
		} else if n == 4 {
			needle = "| 4 |"
		}
		if !strings.Contains(md, needle) {
			t.Errorf("Markdown table missing row for N=%d", n)
		}
	}

	// Must contain column headers.
	for _, header := range []string{"Overclaims", "Underclaims", "Overclaim Rate", "Underclaim Rate"} {
		if !strings.Contains(md, header) {
			t.Errorf("Markdown table missing column header: %s", header)
		}
	}
}

// TestFormatJSON verifies JSON output is valid and contains expected fields.
func TestFormatJSON(t *testing.T) {
	report := &OverclaimReport{
		Results: []OverclaimResult{
			{N: 1, Runs: 5, OverclaimCount: 0, UnderclaimCount: 0, OverclaimRate: 0, UnderclaimRate: 0},
		},
	}

	json, err := FormatJSON(report)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}

	if !strings.Contains(json, `"n": 1`) {
		t.Errorf("JSON missing n field")
	}
	if !strings.Contains(json, `"runs": 5`) {
		t.Errorf("JSON missing runs field")
	}
	if !strings.Contains(json, `"overclaim_count": 0`) {
		t.Errorf("JSON missing overclaim_count field")
	}
}
