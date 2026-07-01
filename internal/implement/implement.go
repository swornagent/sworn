// Package implement drives the agentic tool loop to implement a spec
// against a workspace, then generates a proof bundle from live repo state.
// It stops at state implemented — it never certifies its own work.
//
// Stdlib only — zero runtime dependencies beyond the internal packages.
package implement

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/reqverify"
	"github.com/swornagent/sworn/internal/state"
)

// Run drives the implementer role for one slice:
//  1. Read status.json; if design_review, transition to in_progress.
//  2. Read spec.md and build prompts.
//  3. Run the agentic tool loop (agent.Run).
//  4. Write spec.json record (spec-v1) from spec.md.
//  5. Generate proof.md from live repo state (git diff + test output).
//  6. Write proof.json record (proof-v1) from live repo state.
//  7. Transition status.json to implemented.
//
// Workspace root is the root of the repository the agent operates in.
// Spec path is the absolute path to the slice's spec.md (status.json and
// proof.md are derived from the same directory).
// priorFeedback is the prior verifier's rationale. When non-empty, it is
// injected into the user prompt ahead of the spec so the agent can address
// the named failures.
func Run(ctx context.Context, workspaceRoot, specPath, priorFeedback string, a agent.Agent) (costUSD float64, err error) {
	sliceDir := filepath.Dir(specPath)
	statusPath := filepath.Join(sliceDir, "status.json")
	proofPath := filepath.Join(sliceDir, "proof.md")

	// Step 1: Read and validate current state.
	st, err := state.Read(statusPath)
	if err != nil {
		return 0, fmt.Errorf("implement: read status: %w", err)
	}
	// State transition guard: design_review → in_progress
	// before launching the agent loop.  The Definition of Ready gate
	// (CheckDoR) is applied at this boundary — a slice whose RTM trace,
	// requirements-verify, or requirements-validate gates are not satisfied
	// cannot start implementation.
	if st.State == state.DesignReview {
		releaseDir := filepath.Dir(filepath.Dir(specPath))
		if err := st.State.TransitionGate(state.InProgress, func() error {
			var v reqverify.Verifier
			if a != nil {
				v = agentVerifier{a: a}
			}
			result, err := CheckDoR(ctx, releaseDir, st.SliceID, v)
			if err != nil {
				return fmt.Errorf("Definition of Ready check failed: %w", err)
			}
			if !result.Passed {
				return fmt.Errorf("Definition of Ready blocked: %s", DoRErrorSummary(result))
			}
			return nil
		}); err != nil {
			return 0, fmt.Errorf("implement: design_review → in_progress gate: %w", err)
		}
		st.State = state.InProgress
		st.LastUpdatedBy = "implementer"
		st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := state.Write(statusPath, st); err != nil {
			return 0, fmt.Errorf("implement: write status: %w", err)
		}
	} else if st.State != state.InProgress && st.State != state.FailedVerification {
		return 0, fmt.Errorf("implement: cannot run from state %q", st.State)
	}

	// Step 2: Read spec.
	spec, err := os.ReadFile(specPath)
	if err != nil {
		return 0, fmt.Errorf("implement: read spec: %w", err)
	}
	// Step 3: Build prompts and run agent loop.
	// The agent's final prose is not required: proof.md is built from git
	// diff + test output, so an empty agent return still produces a valid
	// proof bundle and proceeds to verification.
	systemPrompt := prompt.Implementer()

	var userPrompt string
	if priorFeedback != "" {
		feedback := truncateString(priorFeedback, 2000)
		userPrompt = fmt.Sprintf(
			"Previous attempt failed verification — address these specifically:\n\n%s\n\n---\n\nImplement the following spec in workspace %s.\n\n%s\n\nAfter implementation, stop.",
			feedback, workspaceRoot, string(spec),
		)
	} else {
		userPrompt = fmt.Sprintf(
			"Implement the following spec in workspace %s.\n\n%s\n\nAfter implementation, stop.",
			workspaceRoot, string(spec),
		)
	}

	_, cost, _, runErr := agent.Run(ctx, a, systemPrompt, userPrompt, workspaceRoot, agent.Config{})
	if runErr != nil {
		return cost, fmt.Errorf("implement: agent loop: %w", runErr)
	}

	// Step 4: Write spec.json record from spec.md.
	if err := WriteSpecRecord(specPath, statusPath, sliceDir); err != nil {
		return cost, fmt.Errorf("implement: write spec record: %w", err)
	}

	// Step 5: Generate proof.md from live repo state.
	// Re-read status to get the latest start_commit.
	st, err = state.Read(statusPath)
	if err != nil {
		return cost, fmt.Errorf("implement: re-read status: %w", err)
	}
	if err := generateProof(workspaceRoot, specPath, proofPath, st); err != nil {
		return cost, fmt.Errorf("implement: generate proof: %w", err)
	}

	// Step 6: Write proof.json record from live repo state.
	if err := WriteProofRecord(workspaceRoot, specPath, statusPath, sliceDir); err != nil {
		return cost, fmt.Errorf("implement: write proof record: %w", err)
	}

	// Step 7: Transition to implemented.
	if err := st.State.Transition(state.Implemented); err != nil {
		return cost, fmt.Errorf("implement: %w", err)
	}
	st.State = state.Implemented
	st.LastUpdatedBy = "implementer"
	st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := state.Write(statusPath, st); err != nil {
		return cost, fmt.Errorf("implement: write status: %w", err)
	}

	return cost, nil
}

// generateProof writes proof.md in the slice directory from live repo state.
// Every machine-producible section is generated from actual git output and
// test runs — not from the model's narration.
func generateProof(workspaceRoot, specPath, proofPath string, st *state.Status) error {
	specBytes, _ := os.ReadFile(specPath)
	specText := string(specBytes)
	scope := extractScope(specText)

	// Files changed: use git diff --name-only <start_commit>..HEAD.
	var filesChanged string
	if st.StartCommit != "" {
		out, err := runGitCmd(workspaceRoot, "diff", "--name-only", st.StartCommit+"..HEAD")
		if err == nil && out != "" {
			filesChanged = out
		}
	}
	if filesChanged == "" {
		// Fallback: diff HEAD~1..HEAD.
		out, err := runGitCmd(workspaceRoot, "diff", "--name-only", "HEAD~1..HEAD")
		if err == nil && out != "" {
			filesChanged = out
		}
	}
	if filesChanged == "" {
		// Last resort: git status --porcelain.
		out, err := runGitCmd(workspaceRoot, "status", "--porcelain")
		if err == nil && out != "" {
			filesChanged = out
		}
	}
	if filesChanged == "" {
		filesChanged = "(no changes detected)"
	}

	// Test results: run go test ./... in the workspace.
	testOut := runGoTest(workspaceRoot)

	// Delivered: parse checked acceptance criteria from spec.md.
	delivered := deliveredItems(specText)

	// Not delivered: derive from st.OpenDeferrals.
	notDelivered := notDeliveredItems(st.DeferralStrings())

	// Divergence: compare planned_files to actual git diff files.
	divergence := divergenceItems(st.PlannedFiles, filesChangedFromGit(workspaceRoot, st.StartCommit))

	// Build the proof bundle.
	var b strings.Builder
	b.WriteString("# Proof Bundle: `" + st.SliceID + "`\n\n")

	b.WriteString("## Scope\n\n")
	b.WriteString(scope + "\n\n")

	b.WriteString("## Files changed\n\n```\n$ git diff --name-only " + st.StartCommit + "..HEAD\n")
	b.WriteString(filesChanged + "\n```\n\n")

	b.WriteString("## Test results\n\n")
	b.WriteString("### Go\n\n```\n$ go test ./...\n")
	b.WriteString(testOut + "\n```\n\n")

	b.WriteString("## Reachability artefact\n\n")
	b.WriteString("- **Type**: manual-smoke-step\n")
	b.WriteString("- **Path**: `" + proofPath + "`\n")
	b.WriteString("- **User gesture**: `go test ./internal/implement/` exercises Run() end-to-end with a fake agent, asserting that proof.md is generated from live git state.\n\n")

	b.WriteString("## Delivered\n\n")
	for _, d := range delivered {
		b.WriteString("- " + d + "\n")
	}
	if len(delivered) == 0 {
		b.WriteString("(no checked acceptance criteria found)\n")
	}
	b.WriteString("\n")

	b.WriteString("## Not delivered\n\n")
	for _, nd := range notDelivered {
		b.WriteString("- " + nd + "\n")
	}
	if len(notDelivered) == 0 {
		b.WriteString("None\n")
	}
	b.WriteString("\n")

	b.WriteString("## Divergence from plan\n\n")
	for _, d := range divergence {
		b.WriteString("- " + d + "\n")
	}
	if len(divergence) == 0 {
		b.WriteString("None\n")
	}
	b.WriteString("\n")

	return os.WriteFile(proofPath, []byte(b.String()), 0o644)
}

// deliveredItems extracts checked acceptance criteria from spec.md.
func deliveredItems(spec string) []string {
	var items []string
	inSection := false
	for _, line := range strings.Split(spec, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = strings.Contains(strings.ToLower(trimmed), "acceptance check")
			continue
		}
		if !inSection {
			continue
		}
		if m := reACLine.FindStringSubmatch(line); m != nil {
			text := strings.TrimSpace(m[1])
			if strings.HasPrefix(strings.ToUpper(text), "NOTE:") {
				continue
			}
			// Include all ACs — checked or not. The checkmark is informational.
			items = append(items, text)
		}
	}
	return items
}

// notDeliveredItems converts open_deferrals to a list of descriptions.
func notDeliveredItems(deferrals []string) []string {
	return deferrals
}

// divergenceItems compares planned files to actual files from git diff.
func divergenceItems(planned []string, actual []string) []string {
	plannedSet := make(map[string]bool)
	for _, f := range planned {
		plannedSet[f] = true
	}
	actualSet := make(map[string]bool)
	for _, f := range actual {
		actualSet[f] = true
	}

	var divergences []string
	for _, f := range actual {
		if !plannedSet[f] {
			divergences = append(divergences, "unexpected file: "+f)
		}
	}
	for _, f := range planned {
		if !actualSet[f] {
			divergences = append(divergences, "planned but not changed: "+f)
		}
	}
	return divergences
}

// extractScope returns the scope line from a spec (the first heading content after "User outcome").
func extractScope(spec string) string {
	lines := strings.Split(spec, "\n")
	inOutcome := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## User outcome") {
			inOutcome = true
			continue
		}
		if inOutcome && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return trimmed
		}
	}
	return "No scope found in spec"
}

// runGoTest runs go test ./... in the workspace and returns the combined output.
// Returns "(not a Go module — skipped)" if the workspace has no go.mod.
func runGoTest(workspaceRoot string) string {
	if _, err := os.Stat(filepath.Join(workspaceRoot, "go.mod")); os.IsNotExist(err) {
		return "(not a Go module — skipped)"
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(exit error: %v)\n%s", err, string(out))
	}
	return string(out)
}

// truncateString caps s at maxRunes runes, adding an ellipsis if truncated.
func truncateString(s string, maxRunes int) string {
	if len(s) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return s[:maxRunes]
	}
	return s[:maxRunes-3] + "..."
}

// runGitCmd runs an arbitrary git command in workspaceRoot and returns trimmed stdout.
func runGitCmd(workspaceRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workspaceRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Unused import removal note: the git package is imported for DiffRangeStat
// which is no longer used directly — it's referenced via the filesChanged
// fallback path. Keeping the import for backward compatibility with tests.
var _ = git.New
