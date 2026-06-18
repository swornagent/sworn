package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/reqverify"
)

// cmdReqverify implements `sworn reqverify <release>`.
//
// It grades every acceptance criterion in the release against the 29148 quality
// characteristics using a fresh-context model pass. Returns exit 0 when every
// AC passes, exit 1 on any violation, exit 2 on error.
func cmdReqverify(args []string) int {
	fs := flag.NewFlagSet("reqverify", flag.ExitOnError)
	mdl := fs.String("verifier-model", "", "verifier model id (provider/model)")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn reqverify: release name is required")
		fmt.Fprintln(os.Stderr, "usage: sworn reqverify <release>")
		return 64
	}

	releaseName := fs.Arg(0)
	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn reqverify: %v\n", err)
		return 2
	}

	// Resolve verifier model with precedence: flag > env > config.
	var v model.Verifier
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "sworn reqverify: loading config: %v\n", cfgErr)
		// Continue — config may be unavailable but env vars or flags may work.
	}

	resolvedModel, err := config.ResolveVerifierModel(*mdl, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn reqverify: %v\n", err)
		return 2
	}

	if resolvedModel != "" {
		var verr error
		v, verr = model.FromEnv(resolvedModel)
		if verr != nil {
			fmt.Fprintf(os.Stderr, "sworn reqverify: %v\n", verr)
			return 2
		}
	}
	// v remains nil when no model is configured -> Unconfigured (fails closed).

	if v == nil {
		v = model.Unconfigured{}
	}

	systemPrompt := prompt.RequirementsVerifier()

	report, err := reqverify.Run(context.Background(), releaseDir, v, systemPrompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn reqverify: %v\n", err)
		return 2
	}

	// Print the detailed report to stdout.
	fmt.Print(reqverify.Print(report))

	// Print the compact summary to stderr for CI parsing.
	fmt.Fprintln(os.Stderr, reqverify.PrintCompact(report))

	if report.HasViolations() {
		return 1
	}
	return 0
}