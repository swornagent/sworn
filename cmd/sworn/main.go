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
	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/verify"
	"os"
	"strings"
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
	case "lint":
		// S01-rtm-spine / S02-ears-ac-format add this case (T1-fidelity-core).
		// Dispatches to: lint ac <release>, lint trace <release>.
		os.Exit(cmdLint(os.Args[2:]))
	case "reqverify":
		// S04-requirements-verify-gate adds this case (T1-fidelity-core).
		os.Exit(cmdReqverify(os.Args[2:]))
	case "reqvalidate":
		// S05-requirements-validate-gate adds this case (T1-fidelity-core).
		os.Exit(cmdReqvalidate(os.Args[2:]))
	case "designfit":
		// S07-design-fit-gate adds this case (T1-fidelity-core).
		os.Exit(cmdDesignfit(os.Args[2:]))
	case "journeys":
		// S11-journey-elicitation adds this case (T1-fidelity-core).
		os.Exit(cmdJourneys(os.Args[2:]))
	case "top":
		// S15-sworn-top-evidence adds this case (T4-evidence-surface).
		// Read-only evidence surface: green-board / kill-list for journey
		// validation status. Strictly read-only — no state transitions.
		os.Exit(cmdTop(os.Args[2:]))
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
	_ = fs.Parse(args) // Resolve verifier model with precedence: flag > env > config (Coach Pin 3).
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
func usage() {
	fmt.Fprint(os.Stderr, `sworn — SwornAgent's provider-neutral verification core

usage:
  sworn bench --task-set <dir> [--models <comma-sep>] [--output <dir>]
  sworn init [--api-key <key>] [--force]
  sworn journeys [--check] [project-path]
  sworn lint ac <release>
  sworn lint trace <release>
  sworn reqverify <release>
  sworn reqvalidate <release>
  sworn designfit <release>
  sworn run --task <description> [--implementer-model <m>] [--verifier-model <m>] [--base <branch>] [--retry-cap <n>]
  sworn top <release> [project-path]
  sworn verify --spec <path> --diff <path|-> [--proof <path>] [--verifier-model <provider/model>]
  sworn version
bench runs a model benchmark: iterate candidate verifier models against a task set
of slice specs with known-good diffs, record pass-rate + cost + jurisdiction, and
pick the safe-hosted default model from data.

init bootstraps SwornAgent in a repo: writes a config file, vendors the Baton
protocol into docs/baton/, and splices the seven-rule fragment into AGENTS.md.
journeys drafts critical customer journeys from the project and validates
their presence + ratification status. See 'sworn journeys --check' for the
deterministic gate, or 'sworn journeys <project>' for the elicitation loop.
lint checks a release for structural problems. Targets:
  ac     — classify every acceptance check by EARS pattern; fail closed on any
           free-form check that matches no pattern, naming the slice + line.
  trace  — build the 2-D requirements traceability matrix; fail closed on any
           broken trace (orphaned need, orphaned AC, slice with no vertical link).
reqverify grades every acceptance criterion in a release against the ISO/IEC/IEEE
29148 quality characteristics using a fresh-context model pass, fail-closed.
  See 'sworn reqverify <release>' for details.
reqvalidate checks every slice in a release for a human-ratified requirements
validation record (positive+negative scenarios + benefit hypothesis), fail-closed.
  See 'sworn reqvalidate <release>' for details.
designfit checks every slice in a release for stakes-calibrated design-fit gate
(Rule 9): fails closed when any Type-1 (high-stakes) choice lacks a recorded
human decision. No model dispatch needed.
  See 'sworn designfit <release>' for details.
run executes the full turnkey loop: implement -> verify -> (on FAIL: retry/escalate
up to N) -> gated merge on PASS only. See 'sworn run --help' for model resolution
and escalation model defaults.
top renders a read-only evidence surface for the active release: the green-board
or kill-list of journey validation status. See 'sworn top <release>' for details.

verify emits a JSON verdict (PASS/FAIL/BLOCKED) and exits 0 only on PASS,
so a CI required-check blocks the merge by default.
`)
}
