package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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

// runLedger dispatches sworn ledger (sync | report).
func runLedger(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "sworn ledger — manage the verdict ledger\n\n")
		fmt.Fprint(os.Stderr, "usage:\n")
		fmt.Fprint(os.Stderr, "  sworn ledger sync     harvest every release board into docs/ledger/verdicts.jsonl\n")
		fmt.Fprint(os.Stderr, "  sworn ledger report   print pass-rate, attempts-to-pass, and gate-failure aggregates\n")
		return 64
	}
	switch args[0] {
	case "sync":
		return cmdLedgerSync(args[1:])
	case "report":
		return cmdLedgerReport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown ledger subcommand %q\n\n", args[0])
		fmt.Fprint(os.Stderr, "usage: sworn ledger sync\n")
		fmt.Fprint(os.Stderr, "usage: sworn ledger report\n")
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

// cmdLedgerReport reads the verdict corpus and prints the three aggregate tables.
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