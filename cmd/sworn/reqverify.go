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
// It resolves the model from flag/env/config, then delegates to
// cmdReqverifyWithVerifier for the actual work. Returns exit 0 when every
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

	// Resolve verifier model with precedence: flag > env > config.
	var v model.Verifier
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "sworn reqverify: loading config: %v\n", cfgErr)
	}

	// Validate config invariants: UI-bearing projects must declare a design system.
	// Sworn fails closed when a project marked UI-bearing has no design system.
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "sworn reqverify: %v\n", err)
		return 2
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

	return cmdReqverifyWithVerifier(releaseName, v)
}

// cmdReqverifyWithVerifier runs the reqverify business logic with an already-
// resolved verifier. Exported as a package-level function so CLI integration
// tests can inject a stub verifier and exercise the full path through the CLI
// boundary (release resolution -> AC extraction -> model dispatch -> grade
// aggregation -> exit code).
//
// Returns exit 0 when every AC passes, exit 1 on any violation, exit 2 on
// unrecoverable error.
func cmdReqverifyWithVerifier(releaseName string, v reqverify.Verifier) int {
	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn reqverify: %v\n", err)
		return 2
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
