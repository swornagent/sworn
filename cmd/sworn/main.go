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
	"strings"
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
	//
	// Suppress it entirely for machine-readable invocations (e.g. --json):
	// the disclosure goes to stderr, but consumers that merge streams
	// (`sworn board --json 2>&1 | jq`, exec.CombinedOutput) would otherwise
	// see it prepended to the JSON on stdout and fail to parse. Skipping it
	// here leaves the neutral-state sentinel unwritten, so the disclosure
	// still shows on the next human-facing invocation — no consent lost.
	if !isMachineReadable(os.Args) {
		telemetry.ShowDisclosure(os.Stderr)
	}

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
	// telemetry events. This is handled
	// inside telemetry.Fire().
	telemetry.Fire(cmd, sub, version, time.Since(start).Milliseconds(), exitCode)

	os.Exit(exitCode)
}

// isMachineReadable reports whether the invocation requests machine-readable
// output (a --json / -json flag anywhere in the args). Used to suppress
// human-facing notices that would corrupt a machine-readable stdout stream
// when stderr is merged into stdout.
func isMachineReadable(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" {
			return true
		}
	}
	return false
}

// dispatch resolves the subcommand from the registry and runs it.
// No per-command case statements — the registry owns dispatch.
// Returns exit code (0 for success, non-zero for errors).
// Does NOT call os.Exit — the caller (main) handles that after telemetry.
func dispatch(args []string) int {
	if len(args) < 2 {
		// No subcommand — launch the TUI (S04, T2-monitoring).
		if err := tui.Run(version); err != nil {
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
	return 0
}

// cmdHelp prints usage. If the first argument is "loop" (or its deprecated
// alias "run"), it delegates to cmdRun with --help (preserving the pre-registry
// behaviour for sworn help loop).
func cmdHelp(args []string) int {
	if len(args) > 0 && (args[0] == "loop" || args[0] == "run") {
		cmdRun([]string{"--help"})
		return 0
	}
	usage()
	return 0
}

func usage() {
	var b strings.Builder
	// Header line — styled as a heading, matching cmdVersion/top.go vocabulary.
	b.WriteString(style.Heading("sworn — SwornAgent's provider-neutral verification core"))
	b.WriteString("\n\n")
	// usage: label — Bold, matching the label vocabulary in other commands.
	b.WriteString(style.Bold("usage:"))
	b.WriteString("\n")
	// Command synopsis lines. The verb (first token after "sworn ") is accented;
	// the remainder of each line stays plain so option/flag text is byte-identical.
	usageLines := []struct {
		verb string
		rest string
	}{
		{"bench", " --task-set <dir> [--models <comma-sep>] [--output <dir>]"},
		{"capabilities", ""},
		{"init", " [--api-key <key>] [--force]"},
		{"journeys", " [--check] [--impact <release>] [project-path]"},
		{"lint ac", " <release>"},
		{"lint trace", " <release>"},
		{"reqverify", " <release>"},
		{"reqvalidate", " <release>"},
		{"designfit", " <release>"},
		{"designaudit", " <project-dir> [--cohesion on-brand|off-brand]"},
		{"specquality", " <release> [--threshold <0.0-1.0>]"},
		{"loop", " --release <name> --parallel | --task <description> [--implementer-model <m>] [--verifier-model <m>] [--base <branch>] [--retry-cap <n>]"},
		{"ship", " <release> [project-root]"},
		{"telemetry on|off|status", ""},
		{"top", " <release> [project-path]"},
		{"doctor", " [--fix] [--sync-baton]"},
		{"verify", " --spec <path> --diff <path|-> [--proof <path>] [--verifier-model <provider/model>]"},
	}
	for _, l := range usageLines {
		b.WriteString("  sworn ")
		b.WriteString(style.Accent(l.verb))
		b.WriteString(l.rest)
		b.WriteString("\n")
	}
	// The last synopsis line ("sworn version") shares its line with the first
	// description ("bench runs...") — no newline separates them in the original
	// text. Reproduce that exactly: "  sworn version" + "bench runs..." on one
	// line, then the bench description continues for two more lines.
	b.WriteString("  sworn ")
	b.WriteString(style.Accent("version"))
	b.WriteString("bench runs a model benchmark: iterate candidate verifier models against a task set\n")
	b.WriteString("of slice specs with known-good diffs, record pass-rate + cost + jurisdiction, and\n")
	b.WriteString("pick the safe-hosted default model from data.\n\n")

	b.WriteString(style.Accent("capabilities"))
	b.WriteString(" lists the registered drivers from the driver registry: prefixes,\n")
	b.WriteString("roles, availability (no dispatch), and which prefixes route via the sworn proxy.\n")
	b.WriteString("Model prefixes (sworn#31): openai/ = Responses API; openai-completions/ =\n")
	b.WriteString("legacy chat/completions; openai-responses/ = deprecated alias of openai/ (one\n")
	b.WriteString("release); claude-cli/ and codex/ = subscription CLI subprocess drivers.\n")
	b.WriteString(style.Accent("init"))
	b.WriteString(" bootstraps SwornAgent in a repo: writes a config file, vendors the Baton\n")
	b.WriteString("protocol into docs/baton/, and splices the seven-rule fragment into AGENTS.md.\n")
	b.WriteString(style.Accent("journeys"))
	b.WriteString(" drafts critical customer journeys from the project, validates\n")
	b.WriteString("their presence + ratification status, and analyses which journeys a release\n")
	b.WriteString("touches. See 'sworn journeys --check' for the deterministic gate, 'sworn\n")
	b.WriteString("journeys <project>' for the elicitation loop, and 'sworn journeys --impact\n")
	b.WriteString("<release>' for per-release journey-impact analysis.\n")
	b.WriteString(style.Accent("lint"))
	b.WriteString(" checks a release for structural problems. Targets:\n")
	b.WriteString("  ac     — classify every acceptance check by EARS pattern; fail closed on any\n")
	b.WriteString("           free-form check that matches no pattern, naming the slice + line.\n")
	b.WriteString("  trace  — build the 2-D requirements traceability matrix; fail closed on any\n")
	b.WriteString("           broken trace (orphaned need, orphaned AC, slice with no vertical link).\n")
	b.WriteString(style.Accent("reqverify"))
	b.WriteString(" grades every acceptance criterion in a release against the ISO/IEC/IEEE\n")
	b.WriteString("29148 quality characteristics using a fresh-context model pass, fail-closed.\n")
	b.WriteString("  See 'sworn reqverify <release>' for details.\n")
	b.WriteString(style.Accent("reqvalidate"))
	b.WriteString(" checks every slice in a release for a human-ratified requirements\n")
	b.WriteString("validation record (positive+negative scenarios + benefit hypothesis), fail-closed.\n")
	b.WriteString("  See 'sworn reqvalidate <release>' for details.\n")
	b.WriteString(style.Accent("designfit"))
	b.WriteString(" checks every slice in a release for stakes-calibrated design-fit gate\n")
	b.WriteString("(Rule 9): fails closed when any Type-1 (high-stakes) choice lacks a recorded\n")
	b.WriteString("human decision. No model dispatch needed.\n")
	b.WriteString("  See 'sworn designfit <release>' for details.\n")
	b.WriteString(style.Accent("specquality"))
	b.WriteString(" computes soundness + completeness metrics from every slice's\n")
	b.WriteString("acceptance examples, with no source code and no model call. Fails closed when\n")
	b.WriteString("any slice falls below the completeness threshold (default 50%).\n")
	b.WriteString("  See 'sworn specquality <release> [--threshold <0.0-1.0>]' for details.\n")
	b.WriteString(style.Accent("loop"))
	b.WriteString(" runs the delivery loop: implement -> verify -> (on FAIL: retry/escalate\n")
	b.WriteString("up to N) -> gated merge on PASS only. See 'sworn loop --help' for model resolution\n")
	b.WriteString("and escalation model defaults. ('run' is a deprecated alias for 'loop'.)\n")
	b.WriteString(style.Accent("ship"))
	b.WriteString(" validates the human-walkthrough attestation gate (Rule 10/S13): fails\n")
	b.WriteString("closed unless every touched journey has a passing human attestation asserting\n")
	b.WriteString("real-infra + mocks-off. See 'sworn ship <release>' for details.\n")
	b.WriteString(style.Accent("telemetry"))
	b.WriteString(" manages anonymous usage telemetry: on (opt in), off (opt out), status\n")
	b.WriteString("(display current setting). Telemetry is opt-in only, collected during sworn init.\n")
	b.WriteString(style.Accent("top"))
	b.WriteString(" renders a read-only evidence surface for the active release: the green-board\n")
	b.WriteString("or kill-list of journey validation status. See 'sworn top <release>' for details.\n\n")

	b.WriteString(style.Accent("verify"))
	b.WriteString(" emits a JSON verdict (PASS/FAIL/BLOCKED) and exits 0 only on PASS,\n")
	b.WriteString("so a CI required-check blocks the merge by default.\n")
	b.WriteString(style.Accent("doctor"))
	b.WriteString(" runs health checks: embedded prompt integrity, legacy Baton artifact\n")
	b.WriteString("detection, local Baton sync, and dependency version freshness. Exits 0 if\n")
	b.WriteString("clean (WARN-only), 1 on any ERROR, 2 if --fix applied changes.\n")
	fmt.Fprint(os.Stderr, b.String())
}
