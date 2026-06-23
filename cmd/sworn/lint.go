package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/ears"
	"github.com/swornagent/sworn/internal/lint"
	"github.com/swornagent/sworn/internal/rtm"
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
func cmdLint(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sworn lint: target required")
		fmt.Fprintln(os.Stderr, "usage: sworn lint <ac|trace|deps|touchpoints|symbols> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint deps [--base <ref>] <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint touchpoints <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint symbols <slice-id> <release>")
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
	default:
		fmt.Fprintf(os.Stderr, "sworn lint: unknown target %q (known: ac, trace, deps, touchpoints, symbols)\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: sworn lint <ac|trace|deps|touchpoints|symbols> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint deps [--base <ref>] <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint touchpoints <slice-id> <release>")
		fmt.Fprintln(os.Stderr, "       sworn lint symbols <slice-id> <release>")
		return 64
	}
} // cmdLintAC implements `sworn lint ac <release>`.
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
// Builds the 2-D requirements traceability matrix for a release and fails
// closed (non-zero exit) on any broken trace: an orphaned need, an orphaned
// acceptance criterion (no need or no test), or a slice with no vertical link.
// A fully-traced release prints the matrix and exits 0.
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

	m, violations, err := rtm.Build(releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn lint trace: %v\n", err)
		return 2
	}

	fmt.Print(rtm.Print(m))

	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, style.Danger(fmt.Sprintf("\n%d trace violation(s) found:", len(violations))))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v.String())
		}
		return 1
	}

	fmt.Printf("\nAll traces verified. %d needs, %d acceptance criteria, %d tests, %d slices.\n",
		len(m.Needs), len(m.ACs), len(m.Tests), len(m.Slices))
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
