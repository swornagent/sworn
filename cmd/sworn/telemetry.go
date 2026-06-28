package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/style"
	"github.com/swornagent/sworn/internal/supervisor"
	"github.com/swornagent/sworn/internal/telemetry"
)

// cmdTelemetry implements the "sworn telemetry" subcommand.
// Sub-subcommands: on, off, status, decisions, events, report.
func cmdTelemetry(args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry on|off|status|decisions|events|report\n")
		return 64
	}

	switch args[0] {
	case "on":
		return telemetryOn()
	case "off":
		return telemetryOff()
	case "status":
		return telemetryStatus()
	case "decisions":
		return telemetryDecisions(args[1:])
	case "events":
		return telemetryEvents(args[1:])
	case "report":
		return telemetryReport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry on|off|status|decisions|events|report\n")
		return 64
	}
}
func telemetryEvents(args []string) int {
	fs := flag.NewFlagSet("events", flag.ExitOnError)
	releaseName := fs.String("release", "", "release name (required, e.g. 2026-06-27-conformance-foundation)")
	_ = fs.Parse(args)

	if *releaseName == "" {
		fmt.Fprintf(os.Stderr, "telemetry events: --release is required\n")
		return 64
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry events: getwd: %v\n", err)
		return 1
	}

	db, err := supervisor.Open(*releaseName, wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry events: open event store: %v\n", err)
		return 1
	}
	defer db.Close()

	events, err := supervisor.QueryEvents(db, *releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry events: query: %v\n", err)
		return 1
	}

	if len(events) == 0 {
		fmt.Fprintln(os.Stdout, "No events found for release", *releaseName)
		return 0
	}

	// Print events as a simple table.
	fmt.Fprintf(os.Stdout, "%-6s %-12s %-15s %-25s %s\n", "ID", "TRACK", "EVENT", "DETAIL", "TIMESTAMP")
	for _, e := range events {
		fmt.Fprintf(os.Stdout, "%-6d %-12s %-15s %-25s %s\n", e.ID, e.TrackID, e.Event, e.Detail, e.TS)
	}
	return 0
}

// modelReport holds per-model aggregates for the telemetry report.
type modelReport struct {
	ModelID        string
	DispatchCount  int
	ReworkRate     float64
	MeanInputTok   float64
	MeanOutputTok  float64
	MeanDurationMS float64
	TotalCostUSD   float64
}

// telemetryReport implements "sworn telemetry report [--release <name>] [--json]".
// It walks every slice's status.json under docs/release/<name>/*/, reads
// verification.dispatches[], groups by model, and outputs a per-model summary
// table (default) or JSON (--json).
//
// Design decision: reads status.json files exclusively — does not open the
// supervisor DB. The events table has (track_id, release, event, detail, ts)
// with no token/duration/cost columns, and the decisions table is not yet
// built (S02). The spec's stated preference for status.json as the
// "authoritative per-slice ground truth" governs.
//
// Design decision: aggregation is separate from ledger.Project.  ledger.Project
// builds verdict-line corpus entries (pass-rate, cost per verdict) while this
// report computes dispatch-level metrics (rework rate, mean tokens/turn, mean
// duration) grouped by model — a different aggregation axis that doesn't share
// a helper with the ledger without injecting dispatch-level grouping into a
// package that deals in verdict-level records.
func telemetryReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	releaseName := fs.String("release", "", "release name (required, e.g. 2026-06-27-conformance-foundation)")
	asJSON := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	if *releaseName == "" {
		fmt.Fprintf(os.Stderr, "telemetry report: --release is required\n")
		return 64
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry report: %v\n", err)
		return 1
	}

	dispatches, err := collectDispatches(repoRoot, *releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry report: %v\n", err)
		return 1
	}

	if len(dispatches) == 0 {
		fmt.Fprintf(os.Stdout, "No dispatches found for release %s\n", *releaseName)
		return 0
	}

	reports := aggregateByModel(dispatches)

	if *asJSON {
		return outputJSON(reports)
	}
	outputTable(reports)
	return 0
}

// collectDispatches walks docs/release/<release>/*/status.json and collects
// every dispatch from every slice's verification.dispatches[] array.
func collectDispatches(repoRoot, release string) ([]state.Dispatch, error) {
	pattern := filepath.Join(repoRoot, "docs", "release", release, "*", "status.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}

	var all []state.Dispatch
	for _, path := range matches {
		st, err := state.Read(path)
		if err != nil {
			continue // skip unreadable status files
		}
		for _, d := range st.Verification.Dispatches {
			if d.Model == "" {
				continue // skip dispatches without a model — can't group
			}
			all = append(all, d)
		}
	}
	return all, nil
}

// aggregateByModel groups dispatches by model_id_confirmed (falling back to
// model when empty) and computes per-model aggregates: dispatch count, rework
// rate, mean tokens, mean duration, and total cost.
//
// Zero-valued fields (duration_ms == 0, input_tokens == 0, output_tokens == 0)
// are excluded from the respective mean to avoid skewing with pre-S24
// dispatches that didn't record those fields. cost_usd is always summed
// (0 is a valid cost).
func aggregateByModel(dispatches []state.Dispatch) []modelReport {
	type accum struct {
		count               int
		reworkCount         int
		inputSum, outputSum int64
		inputN, outputN     int
		durationSum         int64
		durationN           int
		costSum             float64
	}
	grouped := make(map[string]*accum)

	for _, d := range dispatches {
		key := d.ModelIDConfirmed
		if key == "" {
			key = d.Model
		}

		a := grouped[key]
		if a == nil {
			a = &accum{}
			grouped[key] = a
		}
		a.count++
		if d.Attempt > 0 {
			a.reworkCount++
		}
		if d.InputTokens > 0 {
			a.inputSum += d.InputTokens
			a.inputN++
		}
		if d.OutputTokens > 0 {
			a.outputSum += d.OutputTokens
			a.outputN++
		}
		if d.DurationMS > 0 {
			a.durationSum += d.DurationMS
			a.durationN++
		}
		a.costSum += d.CostUSD
	}

	var reports []modelReport
	for model, a := range grouped {
		r := modelReport{
			ModelID:       model,
			DispatchCount: a.count,
			TotalCostUSD:  a.costSum,
		}
		if a.count > 0 {
			r.ReworkRate = float64(a.reworkCount) / float64(a.count) * 100
		}
		if a.inputN > 0 {
			r.MeanInputTok = float64(a.inputSum) / float64(a.inputN)
		}
		if a.outputN > 0 {
			r.MeanOutputTok = float64(a.outputSum) / float64(a.outputN)
		}
		if a.durationN > 0 {
			r.MeanDurationMS = float64(a.durationSum) / float64(a.durationN)
		}
		reports = append(reports, r)
	}

	// Sort by model ID for deterministic output.
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].ModelID < reports[j].ModelID
	})
	return reports
}

// formatVal formats a float64 value as a string, returning "—" for NaN or Inf.
func formatVal(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	return fmt.Sprintf("%.1f", v)
}

// outputTable prints the per-model summary as a human-readable table.
func outputTable(reports []modelReport) {
	fmt.Fprintf(os.Stdout, "%-40s %6s %10s %15s %16s %16s %12s\n",
		"MODEL", "DISP", "REWORK%", "MEAN_IN_TOK", "MEAN_OUT_TOK", "MEAN_DUR_MS", "COST_USD")
	for _, r := range reports {
		costStr := fmt.Sprintf("$%.4f", r.TotalCostUSD)
		fmt.Fprintf(os.Stdout, "%-40s %6d %9.1f%% %15s %16s %16s %12s\n",
			r.ModelID, r.DispatchCount, r.ReworkRate,
			formatVal(r.MeanInputTok), formatVal(r.MeanOutputTok),
			formatVal(r.MeanDurationMS), costStr)
	}
}

// outputJSON writes the per-model summary as a JSON array to stdout.
func outputJSON(reports []modelReport) int {
	type jsonRow struct {
		Model            string   `json:"model"`
		Dispatches       int      `json:"dispatches"`
		ReworkRate       float64  `json:"rework_rate_pct"`
		MeanInputTokens  *float64 `json:"mean_input_tokens,omitempty"`
		MeanOutputTokens *float64 `json:"mean_output_tokens,omitempty"`
		MeanDurationMS   *float64 `json:"mean_duration_ms,omitempty"`
		TotalCostUSD     float64  `json:"total_cost_usd"`
	}

	var rows []jsonRow
	for _, r := range reports {
		jr := jsonRow{
			Model:        r.ModelID,
			Dispatches:   r.DispatchCount,
			ReworkRate:   r.ReworkRate,
			TotalCostUSD: r.TotalCostUSD,
		}
		if r.MeanInputTok > 0 || !math.IsNaN(r.MeanInputTok) {
			v := r.MeanInputTok
			jr.MeanInputTokens = &v
		}
		if r.MeanOutputTok > 0 || !math.IsNaN(r.MeanOutputTok) {
			v := r.MeanOutputTok
			jr.MeanOutputTokens = &v
		}
		if r.MeanDurationMS > 0 || !math.IsNaN(r.MeanDurationMS) {
			v := r.MeanDurationMS
			jr.MeanDurationMS = &v
		}
		rows = append(rows, jr)
	}
	if rows == nil {
		rows = []jsonRow{}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry report: json: %v\n", err)
		return 1
	}
	return 0
}

// telemetryDecisions outputs the decision log for a release.
// Usage: sworn telemetry decisions --release <name>
func telemetryDecisions(args []string) int {
	fs := flag.NewFlagSet("decisions", flag.ExitOnError)
	release := fs.String("release", "", "release name (required)")
	_ = fs.Parse(args)

	if *release == "" {
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry decisions --release <name>\n")
		return 64
	}

	db, err := openTelemetryDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry decisions: open database: %v\n", err)
		return 1
	}
	defer db.Close()

	rows, err := supervisor.QueryDecisions(db, *release)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry decisions: query: %v\n", err)
		return 1
	}

	if len(rows) == 0 {
		fmt.Printf("No decisions recorded for release %q.\n", *release)
		return 0
	}

	// Human-readable table.
	printDecisionsTable(rows)
	return 0
}

// openTelemetryDB opens the default sworn SQLite database (read-only path).
func openTelemetryDB() (*sql.DB, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	dbPath := wd + "/.sworn/sworn.db"
	return db.Open(dbPath)
}

// printDecisionsTable outputs a human-readable table of decision rows.
func printDecisionsTable(rows []supervisor.DecisionRow) {
	// Column headers.
	fmt.Printf("%-6s %-12s %-10s %-18s %s\n",
		"ID", "SLICE", "ROLE", "ACTION", "REASON")
	fmt.Println(strings.Repeat("-", 100))

	for _, r := range rows {
		reason := r.Reason
		if len(reason) > 55 {
			reason = reason[:52] + "..."
		}
		fmt.Printf("%-6d %-12s %-10s %-18s %s\n",
			r.ID, truncate(r.SliceID, 12), r.Role, truncate(r.Action, 18), reason)
	}

	fmt.Printf("\n%d row(s) for release %q.\n", len(rows), rows[0].Release)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func telemetryOn() int {
	dir, err := telemetry.ConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot determine config directory: %v\n", err)
		return 1
	}

	// Create .telemetry-enabled.
	enabledPath := filepath.Join(dir, ".telemetry-enabled")
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot create config directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(enabledPath, []byte{}, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot write telemetry-enabled sentinel: %v\n", err)
		return 1
	}

	// Remove .no-telemetry if present.
	noTelemetryPath := filepath.Join(dir, ".no-telemetry")
	os.Remove(noTelemetryPath) // best-effort

	fmt.Fprintln(os.Stderr, "telemetry: enabled")
	return 0
}

func telemetryOff() int {
	dir, err := telemetry.ConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot determine config directory: %v\n", err)
		return 1
	}

	// Create .no-telemetry.
	noTelemetryPath := filepath.Join(dir, ".no-telemetry")
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot create config directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(noTelemetryPath, []byte{}, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot write no-telemetry sentinel: %v\n", err)
		return 1
	}

	// Remove .telemetry-enabled if present.
	enabledPath := filepath.Join(dir, ".telemetry-enabled")
	os.Remove(enabledPath) // best-effort

	fmt.Fprintln(os.Stderr, "telemetry: disabled")
	return 0
}

func telemetryStatus() int {
	dir, err := telemetry.ConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot determine config directory: %v\n", err)
		return 1
	}

	if os.Getenv("SWORN_NO_TELEMETRY") == "1" {
		fmt.Fprintln(os.Stdout, style.Dim("telemetry: disabled (SWORN_NO_TELEMETRY env var)"))
		return 0
	}

	enabledPath := filepath.Join(dir, ".telemetry-enabled")
	noTelemetryPath := filepath.Join(dir, ".no-telemetry")

	_, enabledErr := os.Stat(enabledPath)
	_, noTelErr := os.Stat(noTelemetryPath)
	enabledExists := enabledErr == nil
	noTelemetryExists := noTelErr == nil
	if noTelemetryExists {
		fmt.Fprintln(os.Stdout, style.Dim("telemetry: disabled (opted out)"))
	} else if enabledExists {
		fmt.Fprintln(os.Stdout, style.Success("telemetry: enabled"))
	} else {
		fmt.Fprintln(os.Stdout, style.Dim("telemetry: disabled (init not run)"))
	}

	return 0
}
