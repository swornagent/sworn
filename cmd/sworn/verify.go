package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
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
	proof := fs.String("proof", "", "path to the proof bundle (required — fail closed per Rule 6)")
	mdl := fs.String("verifier-model", "", "verifier model id (provider/model)")
	agentic := fs.Bool("agentic", false, "use agentic verifier (full verifier.md role, schema-constrained verdict) instead of the deterministic first-pass")
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
	// v remains nil when no model is configured; the default path below is
	// the deterministic first-pass and never dispatches it.

	// ── Agentic path (--agentic flag) ──────────────────────────────
	if *agentic {
		// Read spec, diff, proof content for the agentic payload. All three
		// are required and must be non-empty BEFORE the verifier is created
		// or dispatched — an empty payload must never reach the model, and a
		// missing/empty/unparseable proof must never upgrade to PASS (Rule 6).
		specContent, sErr := readFileContent(*spec)
		if sErr != nil {
			fmt.Fprintf(os.Stderr, "sworn verify: read spec: %v\n", sErr)
			return 2
		}
		if strings.TrimSpace(specContent) == "" {
			fmt.Fprintf(os.Stderr, "sworn verify: spec is required and must be non-empty (--spec) — fail closed\n")
			return 2
		}
		diffContent, dErr := readFileContent(*diff)
		if dErr != nil {
			fmt.Fprintf(os.Stderr, "sworn verify: read diff: %v\n", dErr)
			return 2
		}
		if strings.TrimSpace(diffContent) == "" {
			fmt.Fprintf(os.Stderr, "sworn verify: diff is required and must be non-empty (--diff) — fail closed\n")
			return 2
		}
		if *proof == "" {
			fmt.Fprintf(os.Stderr, "sworn verify: proof bundle is required (--proof) — fail closed (Rule 6)\n")
			return 2
		}
		proofContent, pErr := readFileContent(*proof)
		if pErr != nil {
			fmt.Fprintf(os.Stderr, "sworn verify: read proof: %v\n", pErr)
			return 2
		}
		if strings.TrimSpace(proofContent) == "" {
			fmt.Fprintf(os.Stderr, "sworn verify: proof bundle %s is empty — fail closed (Rule 6)\n", *proof)
			return 2
		}
		if strings.HasSuffix(*proof, ".json") && !json.Valid([]byte(proofContent)) {
			fmt.Fprintf(os.Stderr, "sworn verify: proof bundle %s is not valid JSON — fail closed (Rule 6)\n", *proof)
			return 2
		}

		// Create an agentic verifier (agent.Agent, not model.Verifier).
		va, vaErr := model.FromEnv(resolvedModel)
		if vaErr != nil {
			fmt.Fprintf(os.Stderr, "sworn verify: create agentic verifier: %v\n", vaErr)
			return 2
		}
		verifierAgent, ok := va.(agent.Agent)
		if !ok {
			fmt.Fprintf(os.Stderr, "sworn verify: model %q does not support agent interface\n", resolvedModel)
			return 2
		}

		result, rErr := verify.RunAgentic(context.Background(), specContent, diffContent, proofContent, verifierAgent)
		if rErr != nil {
			fmt.Fprintf(os.Stderr, "sworn verify: agentic dispatch: %v\n", rErr)
			return 2
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return result.ExitCode()
	}

	// ── Stateless path (default) ───────────────────────────────────
	// The --deferral flags are free-form "why - tracking - ack" strings; wrap
	// each into the typed carrier with the full text in Item so the boundary
	// matcher (Item+Why) sees the same text the old []string match did.
	deferrals := make([]state.Deferral, 0, len(openDeferrals))
	for _, d := range openDeferrals {
		deferrals = append(deferrals, state.Deferral{Item: d})
	}
	res := verify.RunFirstPass(context.Background(), verify.Input{
		SpecPath:  *spec,
		DiffPath:  *diff,
		ProofPath: *proof,
		// The standalone CLI is the proof-bundle gate: an absent --proof is
		// BLOCKED, never PASS (Rule 6 fail-closed).
		ProofRequired: true,
		Model:         resolvedModel,
		Verifier:      v,
		OpenDeferrals: deferrals,
	})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return res.ExitCode()
}

// readFileContent reads a file and returns its content as a string.
// If path is "-", reads from stdin. Returns empty string with no error for
// empty path.
func readFileContent(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "-" {
		b, err := os.ReadFile("/dev/stdin")
		return string(b), err
	}
	b, err := os.ReadFile(path)
	return string(b), err
}
