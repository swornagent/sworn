package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/swornagent/sworn/internal/specquality"
)

// cmdSpecquality implements `sworn specquality <release>`.
//
// It computes soundness + completeness metrics from every slice's acceptance
// examples, with no source code and no model call — the defining property.
// Fails closed when any slice falls below the completeness threshold.
//
// Returns exit 0 when every slice passes, exit 1 on any violation, exit 2 on
// unrecoverable error.
func cmdSpecquality(args []string) int {
	fs := flag.NewFlagSet("specquality", flag.ExitOnError)
	threshold := fs.Float64("threshold", specquality.DefaultThreshold, "minimum completeness score (0.0–1.0)")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn specquality: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn specquality <release> [--threshold <0.0-1.0>]")
		return 64
	}

	releaseName := fs.Arg(0)

	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn specquality: %v\n", err)
		return 2
	}

	report, err := specquality.Run(releaseDir, *threshold)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn specquality: %v\n", err)
		return 2
	}

	// Print the detailed report to stdout.
	fmt.Print(specquality.Print(report))

	// Print the compact summary to stderr for CI parsing.
	fmt.Fprintln(os.Stderr, specquality.PrintCompact(report))

	if !report.Passed {
		return 1
	}
	return 0
}