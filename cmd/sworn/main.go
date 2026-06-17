// Command sworn is SwornAgent's CLI: the provider-neutral verification core.
// Given a spec -> diff (-> proof) triple, it runs SwornAgent's adversarial
// verification and emits a fail-closed verdict. It makes no assumptions about
// the git host (a GitHub Action / GitLab CI / any CI invokes it the same way).
//
// Brand: SwornAgent. Binary: sworn. (Like GitHub CLI -> gh.)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/verify"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(64)
	}
	switch os.Args[1] {
	case "init":
		// S08-init-config adds this case (T3-turnkey-ux).
		os.Exit(cmdInit(os.Args[2:]))
	case "verify":
		os.Exit(cmdVerify(os.Args[2:]))
	case "run":
		// S07-run-loop adds this case (T2-orchestration).
		// Disjoint from "init": both are additive to the switch.
		os.Exit(cmdRun(os.Args[2:]))
	case "bench":
		// S10-benchmark-dogfood adds this case (T4-proof).
		os.Exit(cmdBench(os.Args[2:]))
	case "rtm":
		// S01-rtm-spine adds this case (T1-fidelity-core).
		os.Exit(cmdRtm(os.Args[2:]))
	case "ears":
		// S02-ears-ac-format adds this case (T1-fidelity-core).
		os.Exit(cmdEars(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Printf("sworn %s\nbaton-protocol %s\n", version, prompt.BatonVersion())
	case "help", "--help", "-h":
		if len(os.Args) > 2 && os.Args[2] == "run" {
			cmdRun([]string{"--help"})
			return
		}
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(64)
	}
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	spec := fs.String("spec", "", "path to the spec / acceptance criteria (required)")
	diff := fs.String("diff", "", "path to the unified diff, or - for stdin (required)")
	proof := fs.String("proof", "", "path to the proof bundle (optional in this build)")
	mdl := fs.String("verifier-model", "", "verifier model id (provider/model)")
	_ = fs.Parse(args)

	// Resolve verifier model with precedence: flag > env > config (Coach Pin 3).
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
		SpecPath:  *spec,
		DiffPath:  *diff,
		ProofPath: *proof,
		Model:     resolvedModel,
		Verifier:  v,
	})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return res.ExitCode()
}

func usage() {
	fmt.Fprint(os.Stderr, `sworn — SwornAgent's provider-neutral verification core

usage:
  sworn bench --task-set <dir> [--models <comma-sep>] [--output <dir>]
  sworn init [--api-key <key>] [--force]
  sworn ears <release>
  sworn rtm <release>
  sworn run --task <description> [--implementer-model <m>] [--verifier-model <m>] [--base <branch>] [--retry-cap <n>]  sworn verify --spec <path> --diff <path|-> [--proof <path>] [--verifier-model <provider/model>]
  sworn version
bench runs a model benchmark: iterate candidate verifier models against a task set
of slice specs with known-good diffs, record pass-rate + cost + jurisdiction, and
pick the safe-hosted default model from data.

init bootstraps SwornAgent in a repo: writes a config file, vendors the Baton
protocol into docs/baton/, and splices the seven-rule fragment into AGENTS.md.
ears classifies every acceptance check in a release by EARS pattern and
fails closed on any free-form check that matches no pattern, naming the
slice + the offending line. A release whose every AC is well-formed EARS
passes and prints the per-pattern distribution.

rtm builds the 2-D requirements traceability matrix for a release and fails
closed on any broken trace (orphaned need, orphaned AC, or slice with no
vertical link).
run executes the full turnkey loop: implement → verify → (on FAIL: retry/escalate
up to N) → gated merge on PASS only. See 'sworn run --help' for model resolution
and escalation model defaults.

verify emits a JSON verdict (PASS/FAIL/BLOCKED) and exits 0 only on PASS,
so a CI required-check blocks the merge by default.
`)
}
