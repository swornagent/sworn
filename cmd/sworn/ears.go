package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/ears"
)

// cmdEars implements the `sworn ears` subcommand.
//
//	sworn ears <release>
//
// Classifies every acceptance check in every slice's spec.md by EARS pattern
// and fails closed (non-zero exit) on any free-form check that matches no
// EARS pattern, naming the slice + the offending line. A release whose every
// AC is well-formed EARS passes and prints the per-pattern distribution.
//
// The release argument is the release folder name under docs/release/ (e.g.
// "2026-06-16-fidelity-layer"). The command resolves the release directory
// relative to the repo root (the current working directory).
func cmdEars(args []string) int {
	fs := flag.NewFlagSet("ears", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn ears: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn ears <release>")
		return 64
	}

	releaseName := fs.Arg(0)

	// Resolve the release directory relative to the repo root.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn ears: get cwd: %v\n", err)
		return 2
	}
	releaseDir := filepath.Join(cwd, "docs", "release", releaseName)

	// Check the directory exists.
	if _, err := os.Stat(releaseDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn ears: release directory not found: %s\n", releaseDir)
		return 2
	}

	// Validate the release.
	report, err := ears.Validate(releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn ears: %v\n", err)
		return 2
	}

	// Print the report.
	fmt.Print(ears.Print(report))

	// Fail closed on violations.
	if report.HasViolations() {
		fmt.Fprintf(os.Stderr, "\n%d EARS violation(s) found:\n", len(report.Violations))
		for _, v := range report.Violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v.String())
		}
		return 1
	}

	fmt.Printf("\nAll %d acceptance checks are well-formed EARS. %d note(s) excluded.\n",
		report.TotalACs, report.TotalNotes)
	return 0
}
