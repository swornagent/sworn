package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/rtm"
)

// cmdRtm implements the `sworn rtm` subcommand.
//
//	sworn rtm <release>
//
// Builds the 2-D requirements traceability matrix for a release and fails
// closed (non-zero exit) on any broken trace: an orphaned need, an orphaned
// acceptance criterion (no need or no test), or a slice with no vertical link.
// A fully-traced release prints the matrix and exits 0.
//
// The release argument is the release folder name under docs/release/ (e.g.
// "2026-06-16-fidelity-layer"). The command resolves the release directory
// relative to the repo root (the current working directory).
func cmdRtm(args []string) int {
	fs := flag.NewFlagSet("rtm", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn rtm: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn rtm <release>")
		return 64
	}

	releaseName := fs.Arg(0)

	// Resolve the release directory relative to the repo root.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn rtm: get cwd: %v\n", err)
		return 2
	}
	releaseDir := filepath.Join(cwd, "docs", "release", releaseName)

	// Check the directory exists.
	if _, err := os.Stat(releaseDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn rtm: release directory not found: %s\n", releaseDir)
		return 2
	}

	// Build the matrix.
	m, violations, err := rtm.Build(releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn rtm: %v\n", err)
		return 2
	}

	// Print the matrix.
	fmt.Print(rtm.Print(m))

	// Fail closed on violations.
	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d trace violation(s) found:\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  %s\n", v.String())
		}
		return 1
	}

	fmt.Printf("\nAll traces verified. %d needs, %d acceptance criteria, %d tests, %d slices.\n",
		len(m.Needs), len(m.ACs), len(m.Tests), len(m.Slices))
	return 0
}
