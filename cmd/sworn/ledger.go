package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/ledger"
	"github.com/swornagent/sworn/internal/state"
)

func init() {
	command.Register(command.Command{
		Name:    "ledger",
		Summary: "sync and report on the verdict corpus",
		Run:     runLedger,
	})
}

// runLedger dispatches sworn ledger (sync | report | recommend).
func runLedger(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "sworn ledger — manage the verdict ledger\n\n")
		fmt.Fprint(os.Stderr, "usage:\n")
		fmt.Fprint(os.Stderr, "  sworn ledger sync        harvest every release board into docs/ledger/verdicts.jsonl\n")
		fmt.Fprint(os.Stderr, "  sworn ledger report      print pass-rate, attempts-to-pass, gate-failure, and cost aggregates\n")
		fmt.Fprint(os.Stderr, "  sworn ledger recommend <role> <kind> [--optimize quality|cost|balanced] [--floor 0.8]\n")
		return 64
	}
	switch args[0] {
	case "sync":
		return cmdLedgerSync(args[1:])
	case "report":
		return cmdLedgerReport(args[1:])
	case "recommend":
		return cmdLedgerRecommend(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown ledger subcommand %q\n\n", args[0])
		fmt.Fprint(os.Stderr, "usage: sworn ledger sync\n")
		fmt.Fprint(os.Stderr, "usage: sworn ledger report\n")
		fmt.Fprint(os.Stderr, "usage: sworn ledger recommend <role> <kind> [--optimize quality|cost|balanced] [--floor 0.8]\n")
		return 64
	}
}

// cmdLedgerSync walks every docs/release/*/*/status.json, projects each
// terminal verdict into a Record, and appends it to docs/ledger/verdicts.jsonl.
// Idempotent: a second run adds zero records (Key dedup in ledger.Append).
func cmdLedgerSync(args []string) int {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ledger sync: %v\n", err)
		return 1
	}

	ledgerPath := filepath.Join(repoRoot, "docs", "ledger", "verdicts.jsonl")

	// Walk the release board hierarchy.
	pattern := filepath.Join(repoRoot, "docs", "release", "*", "*", "status.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ledger sync: glob: %v\n", err)
		return 1
	}

	// Count existing lines so we can report actual additions vs idempotent no-ops.
	before := ledger.CountLines(ledgerPath)

	attempted := 0
	skipped := 0
	errors := 0

	for _, statusPath := range matches {
		st, err := state.Read(statusPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ledger sync: read %s: %v\n", statusPath, err)
			errors++
			continue
		}

		// Count acceptance checks from the companion spec.md.
		gateCount := countGates(repoRoot, st.SliceID, st.Release)

		record, ok := ledger.Project(st, gateCount)
		if !ok {
			// No terminal verdict yet; skip.
			skipped++
			continue
		}

		if err := ledger.Append(ledgerPath, record); err != nil {
			fmt.Fprintf(os.Stderr, "ledger sync: append %s: %v\n", st.SliceID, err)
			errors++
			continue
		}
		attempted++
	}

	actualAdded := ledger.CountLines(ledgerPath) - before
	fmt.Printf("ledger sync: %d added, %d skipped (no terminal verdict), %d errors\n", actualAdded, skipped, errors)
	if errors > 0 {
		return 1
	}
	return 0
}

// countGates reads the spec.md for a slice and counts the number of `- [ ]`
// acceptance-check lines. Returns 0 if the spec cannot be read.
func countGates(repoRoot, sliceID, release string) int {
	if release == "" {
		return 0
	}
	specPath := filepath.Join(repoRoot, "docs", "release", release, sliceID, "spec.md")
	f, err := os.Open(specPath)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.HasPrefix(strings.TrimSpace(scanner.Text()), "- [ ]") {
			count++
		}
	}
	// scanner.Err is deliberately ignored — a partial read gives a best-effort count.
	return count
}

// cmdLedgerReport reads the verdict corpus and prints the aggregate tables
// including cost and per-role quality columns (S56).
func cmdLedgerReport(args []string) int {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ledger report: %v\n", err)
		return 1
	}

	ledgerPath := filepath.Join(repoRoot, "docs", "ledger", "verdicts.jsonl")
	records, err := ledger.Load(ledgerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ledger report: %v\n", err)
		return 1
	}

	var r ledger.Report
	r.Render(os.Stdout, records)
	return 0
}

// cmdLedgerRecommend loads the verdict corpus and prints the ranked model
// recommendation for the given (role, kind), optionally with cost-aware
// routing via --optimize and --floor flags.
//
// Usage: sworn ledger recommend <role> <kind> [--optimize quality|cost|balanced] [--floor 0.8]
func cmdLedgerRecommend(args []string) int {
	// Parse positional args: <role> <kind>
	var positional []string
	var optimize string
	floor := 0.0

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--optimize":
			i++
			if i < len(args) {
				optimize = args[i]
			}
		case "--floor":
			i++
			if i < len(args) {
				if f, err := strconv.ParseFloat(args[i], 64); err == nil {
					floor = f
				}
			}
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 2 {
		fmt.Fprint(os.Stderr, "usage: sworn ledger recommend <role> <kind> [--optimize quality|cost|balanced] [--floor 0.8]\n")
		fmt.Fprint(os.Stderr, "\n")
		fmt.Fprint(os.Stderr, "  <role> is the agent role to route (e.g. implementer)\n")
		fmt.Fprint(os.Stderr, "  <kind> is a slice-dimension label: harness, provider, commercial, memory, etc.\n")
		fmt.Fprint(os.Stderr, "\n")
		fmt.Fprint(os.Stderr, "  --optimize  quality | cost | balanced (default quality)\n")
		fmt.Fprint(os.Stderr, "  --floor     minimum pass-rate gate (default 0.8)\n")
		return 64
	}
	role := positional[0]
	kind := positional[1]

	if optimize == "" {
		optimize = "quality"
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ledger recommend: %v\n", err)
		return 1
	}

	ledgerPath := filepath.Join(repoRoot, "docs", "ledger", "verdicts.jsonl")
	records, err := ledger.Load(ledgerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ledger recommend: %v\n", err)
		return 1
	}

	if len(records) == 0 {
		fmt.Printf("No verdict records — run 'sworn ledger sync' first.\n")
		return 0
	}

	obj := ledger.ParseObjective(optimize)
	rec, ok := ledger.RecommendModel(records, role, kind, obj, floor)
	if !ok {
		fmt.Printf("No confident recommendation for %s %q (need at least %d verdicts per model).\n",
			role, kind, ledger.MinSampleSize)
		return 0
	}

	fmt.Printf("Recommendation for %s %q (--optimize %s):\n", role, kind, obj.String())
	fmt.Printf("  model:      %s\n", rec.Model)
	fmt.Printf("  pass-rate:  %.0f%%\n", rec.PassRate*100)
	fmt.Printf("  sample:     %d verdicts\n", rec.Sample)
	if rec.MeanCostUSD > 0 {
		fmt.Printf("  mean cost:  $%.4f/slice\n", rec.MeanCostUSD)
	}

	// Show all ranked candidates for transparency.
	fmt.Println()
	fmt.Println("All ranked models:")
	fmt.Println("  MODEL                                   PASS-RATE  SAMPLE  COST/EA")
	fmt.Println("  -----                                   ---------  ------  -------")
	// Re-rank with quality mode to show all models with enough sample.
	candidates := buildCandidateList(records, kind)
	for _, c := range candidates {
		costStr := "—"
		if c.meanCost > 0 {
			costStr = fmt.Sprintf("$%.4f", c.meanCost)
		}
		fmt.Printf("  %-40s %5.0f%%     %4d   %s\n", c.model, c.passRate*100, c.sample, costStr)
	}
	return 0
}

// candidateRow is a lightweight struct for display in the recommend CLI.
type candidateRow struct {
	model    string
	passRate float64
	sample   int
	meanCost float64
}

// buildCandidateList returns all models with enough sample for display.
func buildCandidateList(records []ledger.Record, kind string) []candidateRow {
	type accum struct {
		pass, fail, blocked int
		totalCost           float64
	}
	m := make(map[string]*accum)
	for _, r := range records {
		if r.SliceKind != kind {
			continue
		}
		if r.Verdict != "pass" && r.Verdict != "fail" && r.Verdict != "blocked" {
			continue
		}
		a := m[r.Model]
		if a == nil {
			a = &accum{}
			m[r.Model] = a
		}
		switch r.Verdict {
		case "pass":
			a.pass++
		case "fail":
			a.fail++
		case "blocked":
			a.blocked++
		}
		a.totalCost += r.TotalCostUSD
	}

	var out []candidateRow
	for model, a := range m {
		sample := a.pass + a.fail + a.blocked
		if sample < ledger.MinSampleSize {
			continue
		}
		rate := float64(a.pass) / float64(sample)
		meanCost := a.totalCost / float64(sample)
		out = append(out, candidateRow{model: model, passRate: rate, sample: sample, meanCost: meanCost})
	}

	// Sort by pass-rate descending.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].passRate > out[i].passRate ||
				(out[j].passRate == out[i].passRate && out[j].model < out[i].model) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
