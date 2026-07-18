package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/gate"
	"github.com/swornagent/sworn/internal/model"
)

// cmdLLMCheck dispatches `sworn llm-check --type <check> --slice <id> --release <name>`.
//
// Five active check types are available; the historical generic
// maintainability-review spelling is recognised only to return migration
// guidance before any release, model, or diff work:
//
//	ac-satisfaction      — does the code actually satisfy each AC?
//	spec-ambiguity        — are any ACs vague, incomplete, or underspecified?
//	design-review         — does the design conflict with project memory?
//	security-review       — does the change introduce vulnerabilities?
//	semantic-coverage     — do tests genuinely verify their ACs?
//	maintainability-review — retired; use sworn maintainability review
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
	checkType := fs.String("type", "", "check type (ac-satisfaction|spec-ambiguity|design-review|security-review|semantic-coverage; maintainability-review is retired: use sworn maintainability review)")
	sliceID := fs.String("slice", "", "slice ID (e.g. S70-llm-check)")
	releaseName := fs.String("release", "", "release name (e.g. 2026-06-19-safe-parallelism)")
	modelID := fs.String("model", "", "model ID (provider/model); default: $SWORN_VERIFIER_MODEL or config.json verifier model")
	baseRef := fs.String("base", "", "base ref for git diff (defaults to start_commit or release-wt/<release>)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	proofReceipt := fs.Bool("proof-receipt", false, "run the native, metadata-only S22 proof receipt mode")
	configuredRecovery := fs.Bool("configured-recovery", false, "run the configured-recovery S22 proof receipt attempt")
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
		fmt.Fprintf(os.Stderr, "valid types: ac-satisfaction, spec-ambiguity, design-review, security-review, semantic-coverage; maintainability-review is retired (use sworn maintainability review)\n")
		return 64
	}
	if gate.IsRetiredLLMCheck(ct) {
		fmt.Fprintf(os.Stderr, "sworn llm-check: %s\n", gate.RetiredMaintainabilityGuidance)
		return 64
	}
	if *configuredRecovery && !*proofReceipt {
		fmt.Fprintln(os.Stderr, "sworn llm-check: --configured-recovery requires --proof-receipt")
		return 64
	}
	if *proofReceipt {
		return cmdLLMCheckProofReceipt(ct, *sliceID, *releaseName, *modelID, *baseRef, *jsonOut, *configuredRecovery)
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
		// Provider and parser errors can contain response data or endpoint
		// fragments. Public CLI output is intentionally stable and non-raw;
		// the in-process report contract remains unchanged for MCP callers.
		fmt.Fprintln(os.Stderr, "sworn llm-check: check did not complete")
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

const (
	s22ProofReceiptRelease = "2026-07-15-baton-v0.16-conformance"
	s22ProofReceiptSlice   = "S22-openrouter-tool-structured-output"
	s22ProofReceiptModel   = "openrouter/z-ai/glm-5.2"
	s22ProofReceiptStart   = "a09b0e46df465862d00469d4aef2a997442b3d5b"
	s22UpstreamSlice       = "S21-openai-structured-envelope"
	s22UpstreamStart       = "ed0badf68673f0af84834458f07be0792555484f"
	s22UpstreamStatusRef   = "240a2ede9a5fd022ae403ced30a6a5f80d918747"
)

// cmdLLMCheckProofReceipt owns the one native S22 receipt lifecycle. Its
// output is deliberately limited to gate.ProofReceipt rendering; neither a
// provider error nor a model report is permitted to cross this command's
// stdout/stderr boundary.
func cmdLLMCheckProofReceipt(checkType gate.CheckType, sliceID, releaseName, requestedModel, baseRef string, jsonOut bool, configuredRecovery bool) int {
	if checkType != gate.CheckSpecAmbiguity || os.Getenv("SWORN_DIRECT") != "1" ||
		releaseName != s22ProofReceiptRelease || sliceID != s22ProofReceiptSlice || baseRef != s22ProofReceiptStart {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}

	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}
	var modelID string
	var err error
	if configuredRecovery {
		if strings.TrimSpace(requestedModel) != "" {
			fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
			return 2
		}
		modelID, err = config.ResolveVerifierModel("", cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
			return 2
		}
	} else {
		modelID, err = config.ResolveVerifierModel(requestedModel, cfg)
		if err != nil || modelID != s22ProofReceiptModel {
			fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
			return 2
		}
	}

	verifier, err := model.FromEnv(modelID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt setup failed")
		return 2
	}
	if configuredRecovery {
		capabilities, ok := verifier.(model.CapabilityProvider)
		if !ok || capabilities.Capabilities()&model.CapStructuredOutput == 0 {
			fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
			return 2
		}
	}

	releaseDir, err := resolveReleaseDir(releaseName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}
	sliceDir := filepath.Join(releaseDir, sliceID)
	if info, err := os.Stat(sliceDir); err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}

	var status struct {
		SliceID     string `json:"slice_id"`
		Release     string `json:"release"`
		State       string `json:"state"`
		StartCommit string `json:"start_commit"`
		Recovery    struct {
			CaptainReview struct {
				Required       bool   `json:"required"`
				State          string `json:"state"`
				ReviewCommit   string `json:"review_commit"`
				AcknowledgedBy string `json:"acknowledged_by"`
				AcknowledgedAt string `json:"acknowledged_at"`
			} `json:"captain_review"`
		} `json:"recovery"`
		UpstreamGate struct {
			SliceID                        string `json:"slice_id"`
			RequiredState                  string `json:"required_state"`
			RequiredVerificationResult     string `json:"required_verification_result"`
			AuthoritativeTrackStatusCommit string `json:"authoritative_track_status_commit"`
			ImmutableStartCommit           string `json:"immutable_start_commit"`
			VerifierVerdictAt              string `json:"verifier_verdict_at"`
		} `json:"upstream_gate"`
	}
	statusData, err := os.ReadFile(filepath.Join(sliceDir, "status.json"))
	if err != nil || json.Unmarshal(statusData, &status) != nil ||
		status.SliceID != sliceID || status.Release != releaseName || status.StartCommit != s22ProofReceiptStart ||
		status.UpstreamGate.SliceID != s22UpstreamSlice || status.UpstreamGate.RequiredState != "verified" ||
		status.UpstreamGate.RequiredVerificationResult != "pass" ||
		status.UpstreamGate.AuthoritativeTrackStatusCommit != s22UpstreamStatusRef ||
		status.UpstreamGate.ImmutableStartCommit != s22UpstreamStart ||
		strings.TrimSpace(status.UpstreamGate.VerifierVerdictAt) == "" {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}
	if configuredRecovery && !configuredRecoveryAuthoritiesValid(sliceDir, status.State,
		status.Recovery.CaptainReview.Required, status.Recovery.CaptainReview.State,
		status.Recovery.CaptainReview.ReviewCommit, status.Recovery.CaptainReview.AcknowledgedBy,
		status.Recovery.CaptainReview.AcknowledgedAt) {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}
	var upstream struct {
		SliceID      string `json:"slice_id"`
		Release      string `json:"release"`
		State        string `json:"state"`
		StartCommit  string `json:"start_commit"`
		Verification struct {
			Result                  string `json:"result"`
			VerifierVerdictAt       string `json:"verifier_verdict_at"`
			VerifierWasFreshContext bool   `json:"verifier_was_fresh_context"`
		} `json:"verification"`
	}
	upstreamData, err := os.ReadFile(filepath.Join(releaseDir, s22UpstreamSlice, "status.json"))
	if err != nil || json.Unmarshal(upstreamData, &upstream) != nil ||
		upstream.SliceID != status.UpstreamGate.SliceID || upstream.Release != releaseName ||
		upstream.State != status.UpstreamGate.RequiredState ||
		upstream.StartCommit != status.UpstreamGate.ImmutableStartCommit ||
		upstream.Verification.Result != status.UpstreamGate.RequiredVerificationResult ||
		upstream.Verification.VerifierVerdictAt != status.UpstreamGate.VerifierVerdictAt ||
		strings.TrimSpace(upstream.Verification.VerifierVerdictAt) == "" ||
		!upstream.Verification.VerifierWasFreshContext {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}
	recoveryBinding := gate.ProofReceiptBinding{
		Release:              releaseName,
		SliceID:              sliceID,
		CheckType:            checkType,
		ModelID:              modelID,
		ImmutableStartCommit: status.StartCommit,
	}
	historicalBinding := recoveryBinding
	historicalBinding.ModelID = s22ProofReceiptModel
	receiptDir := filepath.Join(sliceDir, "receipts")
	if configuredRecovery {
		if err := gate.RequireHistoricalAttemptOneAndTwoForConfiguredRecovery(historicalBinding, recoveryBinding, receiptDir); err != nil {
			fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
			return 2
		}
	} else if err := gate.RequireHistoricalAttemptOneForAttemptTwo(recoveryBinding, receiptDir); err != nil {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}

	// Client construction and prompt preparation are deterministic preflight.
	// They make no provider request, so a bad key or endpoint cannot consume the
	// one bounded dispatch budget.
	runner, err := gate.NewProofReceiptRunner(checkType, sliceDir, verifier)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt preflight rejected")
		return 2
	}

	var receipt gate.ProofReceipt
	if configuredRecovery {
		receipt, err = gate.RunConfiguredRecoveryProofReceipt(context.Background(), historicalBinding, recoveryBinding, receiptDir, runner)
	} else {
		receipt, err = gate.RunProofReceipt(context.Background(), recoveryBinding, receiptDir, runner)
	}
	if receipt.Attempt != 0 {
		if jsonOut {
			fmt.Print(gate.JSONProofReceipt(receipt))
		} else {
			fmt.Print(gate.PrintProofReceipt(receipt))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sworn llm-check: proof receipt did not complete")
		return 2
	}
	if code, available := receipt.ProcessExitCode.Code(); available {
		return code
	}
	return 2
}

const configuredRecoveryProofItem = "Configured-recovery deterministic preflight is current"

func configuredRecoveryAuthoritiesValid(sliceDir, state string, captainRequired bool, captainState, reviewCommit, acknowledgedBy, acknowledgedAt string) bool {
	if state != "in_progress" || !captainRequired || captainState != "acknowledged" ||
		!gateCommit(reviewCommit) || strings.TrimSpace(acknowledgedBy) == "" || strings.TrimSpace(acknowledgedAt) == "" {
		return false
	}
	review, err := os.ReadFile(filepath.Join(sliceDir, "review.md"))
	if err != nil || !bytes.Contains(review, []byte("DECISION: PROCEED")) {
		return false
	}
	var proof struct {
		SliceID     string `json:"slice_id"`
		Release     string `json:"release"`
		TestResults []struct {
			Command string `json:"command"`
			Passed  bool   `json:"passed"`
		} `json:"test_results"`
		Delivered []struct {
			Item string `json:"item"`
		} `json:"delivered"`
	}
	data, err := os.ReadFile(filepath.Join(sliceDir, "proof.json"))
	if err != nil || baton.ValidateSchema("proof-v1", data) != nil || json.Unmarshal(data, &proof) != nil ||
		proof.SliceID != s22ProofReceiptSlice || proof.Release != s22ProofReceiptRelease {
		return false
	}
	required := []string{"go test ./internal/gate", "go test ./...", "go vet ./...", "make build"}
	for _, want := range required {
		found := false
		for _, result := range proof.TestResults {
			if result.Passed && strings.Contains(result.Command, want) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, delivered := range proof.Delivered {
		if delivered.Item == configuredRecoveryProofItem {
			return true
		}
	}
	return false
}

func gateCommit(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
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
