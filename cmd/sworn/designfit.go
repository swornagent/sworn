package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/swornagent/sworn/internal/designfit"
)

// cmdDesignfit implements `sworn designfit <release>`.
//
// It reads every slice's status.json in the release, inspects the
// DesignDecisions field, and fails closed (non-zero exit) when:
//   - Any Type-1 (high-stakes) choice lacks a recorded human decision
//   - Any architecturally-significant choice is misclassified as Type-2
//
// This gate has NO model dispatch — it reads status.json directly.
// The human-owned design decision is recorded by the planner and checked
// here deterministically.
//
// Returns exit 0 when every slice passes, exit 1 on any violation, exit 2 on
// unrecoverable error.
func cmdDesignfit(args []string) int {
	fs := flag.NewFlagSet("designfit", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn designfit: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn designfit <release>")
		return 64
	}

	releaseName := fs.Arg(0)

	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn designfit: %v\n", err)
		return 2
	}

	report, err := designfit.Run(releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn designfit: %v\n", err)
		return 2
	}

	// Print the detailed report to stdout.
	fmt.Print(designfit.Print(report))

	// Print the compact summary to stderr for CI parsing.
	fmt.Fprintln(os.Stderr, designfit.PrintCompact(report))

	if report.HasViolations() {
		return 1
	}
	return 0
}
