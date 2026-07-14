package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/gate"
	"github.com/swornagent/sworn/internal/model"
)

// cmdLLMCheck dispatches `sworn llm-check --type <check> --slice <id> --release <name>`.
//
// Six check types are available:
//
//	ac-satisfaction      — does the code actually satisfy each AC?
//	spec-ambiguity        — are any ACs vague, incomplete, or underspecified?
//	design-review         — does the design conflict with project memory?
//	security-review       — does the change introduce vulnerabilities?
//	semantic-coverage     — do tests genuinely verify their ACs?
//	maintainability-review — is the code understandable 12 months from now?
//
// Each check calls a configured LLM (model resolved from --model, then
// $SWORN_VERIFIER_MODEL, then config.json — the same precedence as reqverify
// and the loop) with a structured prompt, parses the JSON verdict, and exits
// 0 on PASS, 1 on FAIL, 2 on configuration error.
//
// This is separate from `sworn lint` because LLM checks cost credits
// and are not run in the default lint path.
func cmdLLMCheck(args []string) int {
	fs := flag.NewFlagSet("llm-check", flag.ExitOnError)
	checkType := fs.String("type", "", "check type (ac-satisfaction|spec-ambiguity|design-review|security-review|semantic-coverage|maintainability-review)")
	sliceID := fs.String("slice", "", "slice ID (e.g. S70-llm-check)")
	releaseName := fs.String("release", "", "release name (e.g. 2026-06-19-safe-parallelism)")
	modelID := fs.String("model", "", "model ID (provider/model); default: $SWORN_VERIFIER_MODEL or config.json verifier model")
	baseRef := fs.String("base", "", "base ref for git diff (defaults to start_commit or release-wt/<release>)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	// --- argument validation ---
	if *checkType == "" || *sliceID == "" || *releaseName == "" {
		fmt.Fprintln(os.Stderr, "sworn llm-check: --type, --slice, and --release are required")
		fmt.Fprintln(os.Stderr, "usage: sworn llm-check --type <check> --slice <slice-id> --release <release> [--model <provider/model>]")
		return 64
	}

	ct := gate.CheckType(*checkType)
	if !gate.ValidCheckTypes[ct] {
		fmt.Fprintf(os.Stderr, "sworn llm-check: unknown check type %q\n", *checkType)
		fmt.Fprintf(os.Stderr, "valid types: ac-satisfaction, spec-ambiguity, design-review, security-review, semantic-coverage, maintainability-review\n")
		return 64
	}

	// --- resolve paths ---
	releaseDir, err := resolveReleaseDir(*releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn llm-check: %v\n", err)
		return 2
	}

	sliceDir := filepath.Join(releaseDir, *sliceID)
	if _, err := os.Stat(sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn llm-check: slice directory not found: %s\n", sliceDir)
		return 2
	}

	// --- resolve model (flag > $SWORN_VERIFIER_MODEL > config.json) ---
	// The same precedence as reqverify, verify and the loop. This used to read
	// env-only (--model > $SWORN_MODEL), so a fully-configured setup still got
	// "no model configured" — and it read a different env var from every sibling.
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "sworn llm-check: loading config: %v\n", cfgErr)
	}
	mid, err := config.ResolveVerifierModel(*modelID, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn llm-check: %v\n", err)
		return 2
	}

	verifier, err := model.FromEnv(mid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn llm-check: model setup: %v\n", err)
		return 2
	}

	// --- resolve base ref for diff ---
	ref := *baseRef
	if ref == "" {
		var err error
		ref, err = gate.BaseRefForSlice(sliceDir, *releaseName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn llm-check: resolve base ref: %v\n", err)
			return 2
		}
	}

	// --- get git diff ---
	diffContent, err := getDiff(ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn llm-check: git diff: %v\n", err)
		return 2
	}

	// --- run check ---
	ctx := context.Background()
	report, err := gate.RunLLMCheck(ctx, ct, sliceDir, diffContent, verifier)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn llm-check: %v\n", err)
		return 2
	}

	// --- output ---
	if *jsonOut {
		fmt.Print(gate.JSONLLMCheck(report))
	} else {
		fmt.Print(gate.PrintLLMCheck(report))
	}

	if report.HasViolations() {
		return 1
	}
	return 0
}

// getDiff runs `git diff <ref>..HEAD` and returns the output.
// If the ref is "HEAD", returns an empty diff (no changes to evaluate).
func getDiff(ref string) (string, error) {
	if ref == "HEAD" {
		return "", nil
	}

	// Use os/exec to avoid importing os/exec in gate package.
	// We invoke git directly here in the CLI layer.
	return runGitDiff(ref)
}

// runGitDiff runs git diff and returns its output.
func runGitDiff(ref string) (string, error) {
	// We use the git command. First check if there are any changes.
	cmd := exec.Command("git", "diff", ref+"..HEAD", "--", ".")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
