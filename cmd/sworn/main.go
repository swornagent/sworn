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
	"github.com/swornagent/sworn/internal/telemetry"
	"github.com/swornagent/sworn/internal/verify"
	"os"
	"strings"
	"time"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	// ShowDisclosure prints the one-time telemetry disclosure if the user
	// is in a neutral state. It only prints on the first invocation.
	telemetry.ShowDisclosure(os.Stderr)

	start := time.Now()
	exitCode := dispatch(os.Args)

	// Determine cmd and sub for telemetry.
	// cmd = os.Args[1] (the top-level subcommand), sub = os.Args[2] if present.
	cmd, sub := "", ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if len(os.Args) > 2 {
		sub = os.Args[2]
	}

	// Fire telemetry event. Non-blocking — runs in a goroutine.
	// Meta-command exclusion: sworn telemetry * is excluded from firing
	// telemetry events (Coach Pin 4, option (a)). This is handled
	// inside telemetry.Fire().
	telemetry.Fire(cmd, sub, version, time.Since(start).Milliseconds(), exitCode)

	os.Exit(exitCode)
}

// dispatch parses os.Args and dispatches to the appropriate subcommand.
// Returns exit code (0 for success, non-zero for errors).
// Does NOT call os.Exit — the caller (main) handles that after telemetry.
func dispatch(args []string) int {
	if len(args) < 2 {
		usage()
		return 64
	}
	switch args[1] {
	case "init":
		// S08-init-config adds this case (T3-turnkey-ux).
		return cmdInit(args[2:])
	case "verify":
		return cmdVerify(args[2:])
	case "run":
		// S07-run-loop adds this case (T2-orchestration).
		// Disjoint from "init": both are additive to the switch.
		return cmdRun(args[2:])
	case "bench":
		// S10-benchmark-dogfood adds this case (T4-proof).
		return cmdBench(args[2:])
	case "mcp":
		// S08a-mcp-transport adds this case (T4-mcp).
		return cmdMcp(args[2:])
	case "lint":
		// S01-rtm-spine / S02-ears-ac-format add this case (T1-fidelity-core).
		// Dispatches to: lint ac <release>, lint trace <release>.
		return cmdLint(args[2:])
	case "reqverify":
		// S04-requirements-verify-gate adds this case (T1-fidelity-core).
		return cmdReqverify(args[2:])
	case "reqvalidate":
		// S05-requirements-validate-gate adds this case (T1-fidelity-core).
		return cmdReqvalidate(args[2:])
	case "designfit":
		// S07-design-fit-gate adds this case (T1-fidelity-core).
		return cmdDesignfit(args[2:])
	case "journeys":
		// S11-journey-elicitation adds this case (T1-fidelity-core).
		return cmdJourneys(args[2:])
	case "ship":
		// S13-walkthrough-attestation adds this case (T2-delivery-cutover).
		return cmdShip(args[2:])
	case "specquality":
		// S03-spec-quality-firstpass adds this case (T3-leaf-gates).
		return cmdSpecquality(args[2:])
	case "designaudit":
		// S09-design-conformance-audit adds this case (T3-leaf-gates).
		return cmdDesignaudit(args[2:])
	case "top":
		// S15-sworn-top-evidence adds this case (T4-evidence-surface).
		return cmdTop(args[2:])
	case "telemetry":
		// S26-telemetry adds this case (T9-telemetry).
		return cmdTelemetry(args[2:])
	case "version", "--version", "-v":
		fmt.Printf("sworn %s\nbaton-protocol %s\n", version, prompt.BatonVersion())
		return 0
	case "help", "--help", "-h":
		if len(args) > 2 && args[2] == "run" {
			cmdRun([]string{"--help"})
			return 0
		}
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[1])
		usage()
		return 64
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
func usage() {
	fmt.Fprint(os.Stderr, `sworn — SwornAgent's provider-neutral verification core

usage:
  sworn bench --task-set <dir> [--models <comma-sep>] [--output <dir>]
  sworn init [--api-key <key>] [--force]
  sworn journeys [--check] [--impact <release>] [project-path]
  sworn lint ac <release>
  sworn lint trace <release>
  sworn reqverify <release>
  sworn reqvalidate <release>
  sworn designfit <release>
  sworn designaudit <project-dir> [--cohesion on-brand|off-brand]
  sworn specquality <release> [--threshold <0.0-1.0>]
  sworn run --task <description> [--implementer-model <m>] [--verifier-model <m>] [--base <branch>] [--retry-cap <n>]
  sworn ship <release> [project-root]
  sworn telemetry on|off|status
  sworn top <release> [project-path]
  sworn verify --spec <path> --diff <path|-> [--proof <path>] [--verifier-model <provider/model>]
  sworn version
bench runs a model benchmark: iterate candidate verifier models against a task set
of slice specs with known-good diffs, record pass-rate + cost + jurisdiction, and
pick the safe-hosted default model from data.

init bootstraps SwornAgent in a repo: writes a config file, vendors the Baton
protocol into docs/baton/, and splices the seven-rule fragment into AGENTS.md.
journeys drafts critical customer journeys from the project, validates
their presence + ratification status, and analyses which journeys a release
touches. See 'sworn journeys --check' for the deterministic gate, 'sworn
journeys <project>' for the elicitation loop, and 'sworn journeys --impact
<release>' for per-release journey-impact analysis.
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
specquality computes soundness + completeness metrics from every slice's
acceptance examples, with no source code and no model call. Fails closed when
any slice falls below the completeness threshold (default 50%).
  See 'sworn specquality <release> [--threshold <0.0-1.0>]' for details.
run executes the full turnkey loop: implement -> verify -> (on FAIL: retry/escalate
up to N) -> gated merge on PASS only. See 'sworn run --help' for model resolution
and escalation model defaults.
ship validates the human-walkthrough attestation gate (Rule 10/S13): fails
closed unless every touched journey has a passing human attestation asserting
real-infra + mocks-off. See 'sworn ship <release>' for details.
telemetry manages anonymous usage telemetry: on (opt in), off (opt out), status
(display current setting). Telemetry is opt-in only, collected during sworn init.
top renders a read-only evidence surface for the active release: the green-board
or kill-list of journey validation status. See 'sworn top <release>' for details.

verify emits a JSON verdict (PASS/FAIL/BLOCKED) and exits 0 only on PASS,
so a CI required-check blocks the merge by default.
`)
}