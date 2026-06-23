// Command sworn is SwornAgent's CLI: the provider-neutral verification core.
// Given a spec -> diff (-> proof) triple, it runs SwornAgent's adversarial
// verification and emits a fail-closed verdict. It makes no assumptions about
// the git host (a GitHub Action / GitLab CI / any CI invokes it the same way).
//
// Brand: SwornAgent. Binary: sworn. (Like GitHub CLI -> gh.)
//
// T15-owned — the command registry (internal/command) replaced the
// hand-maintained switch; per-command verbs self-register from their own files.
// Adding a new CLI command never edits this file.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/style"
	"github.com/swornagent/sworn/internal/telemetry"
	"github.com/swornagent/sworn/internal/tui"
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

// dispatch resolves the subcommand from the registry and runs it.
// No per-command case statements — the registry owns dispatch.
// Returns exit code (0 for success, non-zero for errors).
// Does NOT call os.Exit — the caller (main) handles that after telemetry.
func dispatch(args []string) int {
	if len(args) < 2 {
		// No subcommand — launch the TUI (S04, T2-monitoring).
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "sworn: %v\n", err)
			return 1
		}
		return 0
	}

	name := args[1]
	c, ok := command.Lookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", name)
		usage()
		return 64
	}
	return c.Run(args[2:])
}

// cmdVersion prints the sworn binary version and baton-protocol version.
func cmdVersion(_ []string) int {
	fmt.Println(style.Banner("sworn " + version))
	fmt.Println(style.Dim("baton-protocol " + prompt.BatonVersion()))
	return 0}

// cmdHelp prints usage. If the first argument is "run", it delegates to
// cmdRun with --help (preserving the pre-registry behaviour for sworn help run).
func cmdHelp(args []string) int {
	if len(args) > 0 && args[0] == "run" {
		cmdRun([]string{"--help"})
		return 0
	}
	usage()
	return 0
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
  sworn doctor [--fix] [--sync-baton]
  sworn verify --spec <path> --diff <path|-> [--proof <path>] [--verifier-model <provider/model>]
  sworn versionbench runs a model benchmark: iterate candidate verifier models against a task set
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
doctor runs health checks: embedded prompt integrity, legacy Baton artifact
detection, local Baton sync, and dependency version freshness. Exits 0 if
clean (WARN-only), 1 on any ERROR, 2 if --fix applied changes.
`)
}