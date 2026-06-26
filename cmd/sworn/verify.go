package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/verify"
)

// openDeferralsFlag implements flag.Value to accept repeated --deferral flags.
type openDeferralsFlag []string

func (f *openDeferralsFlag) String() string { return strings.Join(*f, "; ") }
func (f *openDeferralsFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	spec := fs.String("spec", "", "path to the spec / acceptance criteria (required)")
	diff := fs.String("diff", "", "path to the unified diff, or - for stdin (required)")
	proof := fs.String("proof", "", "path to the proof bundle (optional in this build)")
	mdl := fs.String("verifier-model", "", "verifier model id (provider/model)")
	var openDeferrals openDeferralsFlag
	fs.Var(&openDeferrals, "deferral", "declared Rule-2 deferral (repeatable: 'why - tracking - ack')")
	_ = fs.Parse(args) // Resolve verifier model with precedence: flag > env > config.
	var v model.Verifier
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "sworn verify: loading config: %v\n", cfgErr)
		// Continue — config may be unavailable but env vars or flags may work.
	}

	resolvedModel, err := config.ResolveVerifierModel(*mdl, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn verify: %v\n", err)
		return 2
	}

	// Validate config invariants: UI-bearing projects must declare a design system.
	// Sworn fails closed when a project marked UI-bearing has no design system.
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "sworn verify: %v\n", err)
		return 2
	}

	if resolvedModel != "" {
		var verr error
		v, verr = model.FromEnv(resolvedModel)
		if verr != nil {
			fmt.Fprintf(os.Stderr, "sworn verify: %v\n", verr)
			return 2
		}
	}
	// v remains nil when no model is configured -> Unconfigured (fail-closed).

	res := verify.Run(context.Background(), verify.Input{
		SpecPath:      *spec,
		DiffPath:      *diff,
		ProofPath:     *proof,
		Model:         resolvedModel,
		Verifier:      v,
		OpenDeferrals: openDeferrals,
	})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return res.ExitCode()
}
