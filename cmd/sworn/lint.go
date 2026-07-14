package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/ears"
	"github.com/swornagent/sworn/internal/gate"
	"github.com/swornagent/sworn/internal/lint"
	"github.com/swornagent/sworn/internal/style"
)

// cmdLint dispatches `sworn lint <target> <release>`.
//
// Targets:
//
//	ac          — classify every AC by EARS pattern; fail closed on free-form checks
//	trace       — build the 2-D requirements traceability matrix; fail closed on broken traces
//	deps        — check that go.mod/go.sum changes are declared in planned_files; fail closed on undeclared
//	touchpoints — reconcile design file/package refs against planned_files + collision matrix; fail closed
//	symbols     — grep backtick identifiers from design.md against live codebase; advisory warn-only
//	status      — check that status.json timestamps are not in the future beyond 5m skew; fail closed
//	coverage    — map every AC to a test function in the slice diff; fail closed on uncovered ACs
//	design      — hardcoded colour detection + architecture rule engine (grep, touchpoints, diff-size, external)
//	mock        — no-mock-boundary enforcement: detects undeclared mock/stub/fixture usage alongside real-infra refs
func cmdLint(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sworn lint: target required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint <ac|trace|deps|touchpoints|symbols|status|coverage|design|mock> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint deps [--base <ref>] <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint touchpoints <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint symbols <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint status <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint coverage --slice <slice-id> --release <release> [--base <ref>]")
		fmt.Fprintln(os.Stderr, "       sworn lint design --slice <slice-id> --release <release> [--base <ref>]")
		fmt.Fprintln(os.Stderr, "       sworn lint mock --slice <slice-id> --release <release> [--base <ref>]")
		return 64
	}
	switch args[0] {
	case "ac":
		return cmdLintAC(args[1:])
	case "trace":
		return cmdLintTrace(args[1:])
	case "deps":
		return cmdLintDeps(args[1:])
	case "touchpoints":
		return cmdLintTouchpoints(args[1:])
	case "symbols":
		return cmdLintSymbols(args[1:])
	case "status":
		return cmdLintStatus(args[1:])
	case "coverage":
		return cmdLintCoverage(args[1:])
	case "design":
		return cmdLintDesign(args[1:])
	case "mock":
		return cmdLintMock(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "sworn lint: unknown target %q (known: ac, trace, deps, touchpoints, symbols, status, coverage, design, mock)\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: sworn lint <ac|trace|deps|touchpoints|symbols|status|coverage|design|mock> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint deps [--base <ref>] <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint touchpoints <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint symbols <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint status <release>")
		return 64
	}
}

// cmdLintAC implements `sworn lint ac <release>`.
// Classifies every acceptance check in every slice's spec.md by EARS pattern
// and fails closed (non-zero exit) on any free-form check that matches no
// pattern, naming the slice + the offending line. A release whose every AC is
// well-formed EARS passes and prints the per-pattern distribution.
func cmdLintAC(args []string) int {
	fs := flag.NewFlagSet("lint ac", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn lint ac: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint ac <release>")
		return 64
	}

	releaseName := fs.Arg(0)
	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint ac: %v\n", err)
		return 2
	}

	report, err := ears.Validate(releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint ac: %v\n", err)
		return 2
	}

	fmt.Print(ears.Print(report))

	if report.HasViolations() {
		fmt.Fprintln(os.Stderr, style.Danger(fmt.Sprintf("\n%d EARS violation(s) found:", len(report.Violations))))
		for _, v := range report.Violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v.String())
		}
		return 1
	}

	fmt.Printf("\nAll %d acceptance checks are well-formed EARS. %d note(s) excluded.\n",
		report.TotalACs, report.TotalNotes)
	return 0
}

// cmdLintTrace implements `sworn lint trace <release>`.
//
// Port of the canonical baton release-trace.sh: builds the full RTM + EARS +
// sniff-test gate for Rule 8. Verifies the full requirements-fidelity chain:
//
//	intake → slice (covers_needs) → AC (spec.md citations) → test (Required tests)
//
// Plus structural-completeness sniff-test and EARS conformance.
// Fails closed (non-zero exit) on any violation.
// A fully-traced release prints the report and exits 0.
func cmdLintTrace(args []string) int {
	fs := flag.NewFlagSet("lint trace", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn lint trace: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint trace <release>")
		return 64
	}

	releaseName := fs.Arg(0)
	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint trace: %v\n", err)
		return 2
	}

	report, err := gate.RunTrace(releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint trace: %v\n", err)
		return 2
	}

	fmt.Print(gate.PrintReport(report))

	if report.HasViolations() {
		return 1
	}
	return 0
}

// cmdLintDeps implements `sworn lint deps <slice-id> <release>`.
//
// Checks that go.mod / go.sum changes in the slice's diff are declared in the
// slice's status.json planned_files. Fails closed (exit 1) when a changed dep
// file is undeclared, naming the offending file(s).
func cmdLintDeps(args []string) int {
	fs := flag.NewFlagSet("lint deps", flag.ExitOnError)
	baseRef := fs.String("base", "", "base ref for git diff (defaults to start_commit or release-wt/<release>)")
	_ = fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "sworn lint deps: slice-id and release name are required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint deps [--base <ref>] <slice-id> <release>")
		return 64
	}
	sliceID := fs.Arg(0)
	releaseName := fs.Arg(1)

	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint deps: %v\n", err)
		return 2
	}

	sliceDir := filepath.Join(releaseDir, sliceID)
	if _, err := os.Stat(sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint deps: slice directory not found: %s\n", sliceDir)
		return 2
	}

	if err := lint.CheckDeps(sliceDir, *baseRef); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint deps: %v\n", err)
		return 1
	}

	fmt.Printf("deps: all dependency files declared in planned_files for %s\n", sliceID)
	return 0
}

// cmdLintTouchpoints implements `sworn lint touchpoints <slice-id> <release>`.
//
// Parses a slice's spec for referenced files/packages, reconciles them against
// planned_files AND the release index.md touchpoint matrix (flagging cross-slice
// file collisions), and detects duplicate migration numbers across slices.
// Fails closed (exit 1) on any undeclared touchpoint or unacknowledged collision.
func cmdLintTouchpoints(args []string) int {
	fs := flag.NewFlagSet("lint touchpoints", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "sworn lint touchpoints: slice-id and release name are required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint touchpoints <slice-id> <release>")
		return 64
	}
	sliceID := fs.Arg(0)
	releaseName := fs.Arg(1)

	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint touchpoints: %v\n", err)
		return 2
	}

	sliceDir := filepath.Join(releaseDir, sliceID)
	if _, err := os.Stat(sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint touchpoints: slice directory not found: %s\n", sliceDir)
		return 2
	}

	if err := lint.CheckTouchpoints(sliceDir, releaseDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint touchpoints: %v\n", err)
		return 1
	}

	fmt.Printf("touchpoints: all references declared, no collisions, no duplicate migrations for %s\n", sliceID)
	return 0
}

// cmdLintSymbols implements `sworn lint symbols <slice-id> <release>`.
//
// Extracts backtick-quoted identifiers from the slice's design.md, greps each
// against the live codebase (excluding docs/), and reports unresolved symbols
// as advisory warnings. Exit code 3 on unresolved symbols (advisory, distinct
// from the hard-fail exit 1 and I/O error exit 2); exit 0 when all resolve.
func cmdLintSymbols(args []string) int {
	fs := flag.NewFlagSet("lint symbols", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "sworn lint symbols: slice-id and release name are required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint symbols <slice-id> <release>")
		return 64
	}
	sliceID := fs.Arg(0)
	releaseName := fs.Arg(1)

	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint symbols: %v\n", err)
		return 2
	}

	sliceDir := filepath.Join(releaseDir, sliceID)
	if _, err := os.Stat(sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint symbols: slice directory not found: %s\n", sliceDir)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint symbols: get cwd: %v\n", err)
		return 2
	}

	if err := lint.CheckSymbols(sliceDir, cwd); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint symbols: %v\n", err)
		return 3
	}

	fmt.Printf("symbols: all identifiers in %s resolve against the live codebase\n", sliceID)
	return 0
}

// resolveReleaseDir returns the absolute path to docs/release/<name> relative
// to the current working directory, or an error if the directory does not exist.
func resolveReleaseDir(name string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	dir := filepath.Join(cwd, "docs", "release", name)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("release directory not found: %s", dir)
	}
	return dir, nil
}

// cmdLintStatus implements `sworn lint status <release>`.
//
// Walks the release directory for every slice's status.json and validates
// that last_updated_at and verification.verifier_verdict_at timestamps are
// not in the future beyond a 5-minute clock-skew allowance. Malformed
// timestamps fail closed.  Exit 0 when all timestamps are sane; exit 1
// on any future/malformed timestamp, naming the offending slice, field,
// value, and the allowed maximum.
func cmdLintStatus(args []string) int {
	fs := flag.NewFlagSet("lint status", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn lint status: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint status <release>")
		return 64
	}

	releaseName := fs.Arg(0)
	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint status: %v\n", err)
		return 2
	}

	violations := lint.CheckStatusTimestamps(releaseDir, lint.DefaultClock)
	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, style.Danger(fmt.Sprintf("%d status timestamp violation(s) found:", len(violations))))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v.String())
		}
		return 1
	}

	fmt.Printf("All status timestamps within allowed window for %s\n", releaseName)
	return 0
}

// cmdLintCoverage implements `sworn lint coverage --slice <slice-id> --release <release>`.
//
// Extracts acceptance checks from the slice's spec.md, scans the test files in
// the slice's diff for test functions (Go, TypeScript, Python patterns), and
// keyword-matches each AC against the discovered tests.  Prints a coverage map
// showing each AC mapped to its best-match test (file:line) and exits 0 when
// every AC is covered, 1 with uncovered ACs enumerated.
func cmdLintCoverage(args []string) int {
	fs := flag.NewFlagSet("lint coverage", flag.ExitOnError)
	sliceID := fs.String("slice", "", "slice ID to check (e.g. S66-lint-coverage)")
	releaseName := fs.String("release", "", "release name (e.g. 2026-06-19-safe-parallelism)")
	baseRef := fs.String("base", "", "base ref for git diff (defaults to start_commit or release-wt/<release>)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	if *sliceID == "" || *releaseName == "" {
		fmt.Fprintln(os.Stderr, "sworn lint coverage: --slice and --release are required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint coverage --slice <slice-id> --release <release> [--base <ref>]")
		return 64
	}

	releaseDir, err := resolveReleaseDir(*releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint coverage: %v\n", err)
		return 2
	}

	sliceDir := filepath.Join(releaseDir, *sliceID)
	if _, err := os.Stat(sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint coverage: slice directory not found: %s\n", sliceDir)
		return 2
	}

	ref := *baseRef
	if ref == "" {
		var err error
		ref, err = gate.BaseRefForSlice(sliceDir, *releaseName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn lint coverage: resolve base ref: %v\n", err)
			return 2
		}
	}

	report, err := gate.RunCoverage(releaseDir, *sliceID, ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint coverage: %v\n", err)
		return 2
	}

	if *jsonOut {
		fmt.Print(gate.JSONCoverage(report))
	} else {
		fmt.Print(gate.PrintCoverage(report))
	}

	if report.HasViolations() {
		return 1
	}
	return 0
}

// cmdLintDesign implements `sworn lint design --slice <slice-id> --release <release>`.
//
// Port of bin/release-audit-design.sh from bash to Go. Runs hardcoded colour
// detection in UI files from the slice's diff, then executes the architecture
// rule engine (grep, touchpoints, diff-size, external) from docs/architecture.json
// with a legacy docs/baton fallback.
// Reads docs/baton/design-fidelity.json for design token exemptions and the
// per-slice design-allowlist.json for escape-hatch suppression.
// Exits 0 on clean pass, 1 with enumerated violations.
func cmdLintDesign(args []string) int {
	fs := flag.NewFlagSet("lint design", flag.ExitOnError)
	sliceID := fs.String("slice", "", "slice ID to check (e.g. S67-lint-design)")
	releaseName := fs.String("release", "", "release name (e.g. 2026-06-19-safe-parallelism)")
	baseRef := fs.String("base", "", "base ref for git diff (defaults to start_commit or release-wt/<release>)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	if *sliceID == "" || *releaseName == "" {
		fmt.Fprintln(os.Stderr, "sworn lint design: --slice and --release are required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint design --slice <slice-id> --release <release> [--base <ref>]")
		return 64
	}

	releaseDir, err := resolveReleaseDir(*releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint design: %v\n", err)
		return 2
	}

	sliceDir := filepath.Join(releaseDir, *sliceID)
	if _, err := os.Stat(sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint design: slice directory not found: %s\n", sliceDir)
		return 2
	}

	ref := *baseRef
	if ref == "" {
		var err error
		ref, err = gate.BaseRefForSlice(sliceDir, *releaseName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn lint design: resolve base ref: %v\n", err)
			return 2
		}
	}

	report, err := gate.RunDesign(releaseDir, *sliceID, ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint design: %v\n", err)
		return 2
	}

	if *jsonOut {
		fmt.Print(gate.JSONDesign(report))
	} else {
		fmt.Print(gate.PrintDesign(report))
	}

	if report.HasViolations() {
		return 1
	}
	return 0
}

// cmdLintMock implements `sworn lint mock --slice <slice-id> --release <release>`.
//
// Port of release-mock-check.sh from bash to Go: Rule 10 no-mock boundary
// enforcement. Scans test files in the slice's diff for mock/stub/fixture/seed
// usage and detects real-infra references alongside undeclared mocks. Boundary
// declarations (@mock-boundary comment, open_deferrals entry, architecture-overrides.json)
// suppress violations.
// Exits 0 when every mock has a declared boundary, 1 with violations enumerated.
func cmdLintMock(args []string) int {
	fs := flag.NewFlagSet("lint mock", flag.ExitOnError)
	sliceID := fs.String("slice", "", "slice ID to check (e.g. S68-lint-mock)")
	releaseName := fs.String("release", "", "release name (e.g. 2026-06-19-safe-parallelism)")
	baseRef := fs.String("base", "", "base ref for git diff (defaults to start_commit or release-wt/<release>)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	if *sliceID == "" || *releaseName == "" {
		fmt.Fprintln(os.Stderr, "sworn lint mock: --slice and --release are required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint mock --slice <slice-id> --release <release> [--base <ref>]")
		return 64
	}

	releaseDir, err := resolveReleaseDir(*releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint mock: %v\n", err)
		return 2
	}

	sliceDir := filepath.Join(releaseDir, *sliceID)
	if _, err := os.Stat(sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint mock: slice directory not found: %s\n", sliceDir)
		return 2
	}

	ref := *baseRef
	if ref == "" {
		var err error
		ref, err = gate.BaseRefForSlice(sliceDir, *releaseName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn lint mock: resolve base ref: %v\n", err)
			return 2
		}
	}

	report, err := gate.RunMock(releaseDir, *sliceID, ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint mock: %v\n", err)
		return 2
	}

	if *jsonOut {
		fmt.Print(gate.JSONMock(report))
	} else {
		fmt.Print(gate.PrintMock(report))
	}

	if report.HasViolations() {
		return 1
	}
	return 0
}
