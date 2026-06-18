package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/swornagent/sworn/internal/reqvalidate"
)

// cmdReqvalidate implements `sworn reqvalidate <release>`.
//
// It reads every slice's status.json in the release, checks for a complete
// human-ratified validation record (scenarios + benefit hypothesis), and fails
// closed (non-zero exit) on any missing or model-only validation.
//
// Unlike reqverify, this gate has NO model dispatch — it reads status.json
// directly. The human-owned ratification is recorded by the planner and checked
// here deterministically.
//
// Returns exit 0 when every slice passes, exit 1 on any violation, exit 2 on
// unrecoverable error.
func cmdReqvalidate(args []string) int {
	fs := flag.NewFlagSet("reqvalidate", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn reqvalidate: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn reqvalidate <release>")
		return 64
	}

	releaseName := fs.Arg(0)

	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn reqvalidate: %v\n", err)
		return 2
	}

	report, err := reqvalidate.Run(releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn reqvalidate: %v\n", err)
		return 2
	}

	// Print the detailed report to stdout.
	fmt.Print(reqvalidate.Print(report))

	// Print the compact summary to stderr for CI parsing.
	fmt.Fprintln(os.Stderr, reqvalidate.PrintCompact(report))

	if report.HasViolations() {
		return 1
	}
	return 0
}