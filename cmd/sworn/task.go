package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/implement"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/run"
	"github.com/swornagent/sworn/internal/state"
)

// plannerFromEnv is the planner model factory. In production it delegates to
// model.FromEnv; tests replace it with a mock to avoid real API calls.
var plannerFromEnv = model.FromEnv

// cmdRunTask implements the real planner-assist single-slice quickstart path// for `sworn run --task`. It dispatches the planner role to draft a concrete
// spec.md with EARS ACs, then runs implement+verify over that spec.
//
// S21-sworn-run-task: direction C — honest demo/on-ramp replacing the stub
// that auto-generated a fake spec. The planner dispatch is a single-shot
// Verify() call (no tool-use or agent loop) — any OAI-compatible driver that
// supports Verify() suffices.
func cmdRunTask(
	task string,
	implModel string,
	verifierModel string,
	escalationModels []string,
	retryCap int,
	implementTimeout time.Duration,
	dryRun bool,
	notifier *account.Notifier,
) int {
	if task == "" {
		fmt.Fprintln(os.Stderr, "sworn run: --task is required")
		return 64
	}

	// ── Dry-run: verify code compiles and planner dispatch path is reachable ──
	if dryRun {
		// Resolve the planner model to confirm it's valid (without dispatching).
		plannerID, err := resolvePlannerModel(implModel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn run: planner model resolution: %v\n", err)
			return 2
		}
		fmt.Printf("sworn run --task: planner dispatch would be called\n")
		fmt.Printf("  task:         %s\n", task)
		fmt.Printf("  planner model: %s\n", plannerID)
		fmt.Printf("  verifier model: %s\n", verifierModel)
		return 0
	}

	// ── Resolve workspace root ────────────────────────────────────────────
	workspaceRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: getwd: %v\n", err)
		return 1
	}

	// ── 1. Create task-run directory ──────────────────────────────────────
	ts := time.Now().UTC().Format("20060102-150405")
	taskRoot := filepath.Join(workspaceRoot, ".sworn", "task-runs", ts)
	sliceDir := filepath.Join(taskRoot, "S01-task")

	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: create task dir: %v\n", err)
		return 1
	}

	// ── 2. Resolve planner model ──────────────────────────────────────────
	plannerModelID, err := resolvePlannerModel(implModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: %v\n", err)
		os.RemoveAll(taskRoot)
		return 2
	}

	// ── 3. Dispatch planner ───────────────────────────────────────────────
	plannerV, err := plannerFromEnv(plannerModelID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: create planner model: %v\n", err)
		os.RemoveAll(taskRoot)
		return 2
	}

	systemPrompt := prompt.Planner()
	// The planner prompt expects a discovery/intake message. For single-slice
	// task mode we provide a focused prompt asking for exactly one spec.
	userMsg := fmt.Sprintf(
		"# Single-slice task\n\n"+
			"Create a single-slice release spec for the following task. "+
			"Return the spec.md content (frontmatter with title/description, "+
			"User outcome, In scope, Out of scope, Acceptance checks as - [ ] items, "+
			"Required tests, and Risks). Use EARS notation for acceptance checks.\n\n"+
			"Task: %s\n\n"+
			"The slice ID is S01-task and the release is 'task-%s' (ephemeral). "+
			"The output will be implemented and verified by the sworn run loop.",
		task, ts,
	)

	fmt.Fprintf(os.Stderr, "sworn run: dispatching planner with %s...\n", plannerModelID)
	reply, _, _, _, err := plannerV.Verify(context.Background(), systemPrompt, userMsg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: planner dispatch error: %v\n", err)
		// Keep artefacts for inspection.
		os.WriteFile(filepath.Join(sliceDir, "planner-error.txt"),
			[]byte(fmt.Sprintf("error: %v\n", err)), 0o644)
		return 1
	}

	// ── 4. Extract spec content from reply ────────────────────────────────
	specContent := extractSpecFromReply(reply)

	// ── 5. Validate ACs ───────────────────────────────────────────────────
	if !hasAcceptanceChecks(specContent) {
		fmt.Fprintf(os.Stderr, "sworn run: planner output contained no acceptance criteria — cannot implement\n")
		// Keep artefacts for inspection.
		os.WriteFile(filepath.Join(sliceDir, "planner-output.txt"), []byte(reply), 0o644)
		return 2
	}

	// ── 6. Write spec.md ──────────────────────────────────────────────────
	specPath := filepath.Join(sliceDir, "spec.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: write spec: %v\n", err)
		return 1
	}

	// ── 7. Write status.json ──────────────────────────────────────────────
	statusPath := filepath.Join(sliceDir, "status.json")
	releaseName := "task-" + ts
	st := &state.Status{
		Schema:      "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		SliceID:     "S01-task",
		Release:     releaseName,
		Track:       "",
		CoversNeeds: []string{"N/A-task-mode"},
		State:       state.InProgress, Owner: "sworn-run",
		LastUpdatedBy: "planner-dispatch",
		LastUpdatedAt: time.Now().UTC().Format(time.RFC3339),
		SpecPath:      "S01-task/spec.md",
		ProofPath:     "S01-task/proof.md",
		JournalPath:   "S01-task/journal.md",
		PlannedFiles:  []string{},
		TestCommands:  []string{"go test ./..."},
		Verification:  state.Verification{},
		ReleaseBase:   "main",
	}
	if err := state.Write(statusPath, st); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: write status: %v\n", err)
		return 1
	}

	// ── 7b. Synthesise authoritative spec.json ────────────────────────────
	// Pin 2 / CHOICE-B: the on-ramp must keep the engine on one machine-contract
	// path. Emit an authoritative spec.json from the planner's spec.md so every
	// downstream read site (implement/verify/gates) reads spec.json rather than
	// re-introducing the legacy spec.md contract (S01-spec-json-read-conformance).
	// spec.md is retained as the human artefact / legacy fallback.
	if err := implement.WriteSpecRecord(specPath, statusPath, sliceDir); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: write spec.json: %v\n", err)
		return 1
	}

	// ── 8. Init git repo in task-runs directory and commit ────────────────
	repo := git.New(taskRoot)
	if err := repo.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: git init: %v\n", err)
		return 1
	}

	// Configure git user for clean commits in this ephemeral repo.
	repo.Config("user.email", "sworn@localhost")
	repo.Config("user.name", "sworn")

	if err := repo.Stage("."); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: stage: %v\n", err)
		return 1
	}
	if err := repo.Commit("chore(task): auto-generated slice from planner dispatch — " + task); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: commit: %v\n", err)
		return 1
	}

	startCommit, err := repo.RevParse("HEAD")
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: rev-parse: %v\n", err)
		return 1
	}

	st.StartCommit = startCommit
	if err := state.Write(statusPath, st); err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: update start_commit: %v\n", err)
		return 1
	}

	// ── 9. Run implement+verify ───────────────────────────────────────────
	fmt.Fprintf(os.Stderr, "sworn run: running implement+verify for S01-task...\n")

	err = run.RunSlice(context.Background(), taskRoot,
		"S01-task/spec.md",
		"S01-task/status.json",
		run.RunSliceOptions{
			ImplementerModel: implModel,
			VerifierModel:    verifierModel,
			EscalationModels: escalationModels,
			RetryCap:         retryCap,
			ImplementTimeout: implementTimeout,
			Notifier:         notifier,
		})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn run: %v\n", err)
		proofPath := filepath.Join(taskRoot, "S01-task", "proof.md")
		if _, statErr := os.Stat(proofPath); statErr == nil {
			fmt.Fprintf(os.Stderr, "proof bundle: %s\n", proofPath)
		}
		return 1
	}

	proofPath := filepath.Join(taskRoot, "S01-task", "proof.md")
	fmt.Fprintf(os.Stderr, "sworn run: PASS — proof bundle at %s\n", proofPath)
	fmt.Printf("proof bundle: %s\n", proofPath)
	return 0
}

// resolvePlannerModel returns the model ID to use for the planner dispatch.
// It defaults to the configured implementer model, then to a hardcoded default.
func resolvePlannerModel(implModel string) (string, error) {
	if implModel != "" {
		return implModel, nil
	}
	cfg, err := config.Load()
	if err == nil {
		id, err := config.ResolveImplementerModel("", cfg, "", "", "quality", 0)
		if err == nil {
			return id, nil
		}
	}
	// Hardcoded fallback — the planner can work with any Verify-capable model.
	return "openai/gpt-4o", nil
}

// hasAcceptanceChecks returns true if the spec content contains at least one
// acceptance check line in markdown checkbox format (- [ ]).
func hasAcceptanceChecks(content string) bool {
	return strings.Contains(content, "- [ ]")
}

// extractSpecFromReply extracts spec.md content from the planner's raw reply.
// The planner may return the spec wrapped in various formats:
//   - Bare spec with frontmatter starting with "---"
//   - Spec inside a ```markdown code block
//   - Prose with the spec embedded after a heading
func extractSpecFromReply(reply string) string {
	reply = strings.TrimSpace(reply)

	// Case 1: Reply starts with frontmatter — use as-is.
	if strings.HasPrefix(reply, "---") {
		return reply
	}

	// Case 2: Spec inside a markdown code block.
	if idx := strings.Index(reply, "```markdown"); idx >= 0 {
		start := idx + len("```markdown")
		// Skip to next line.
		if nl := strings.IndexByte(reply[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		if end := strings.Index(reply[start:], "```"); end >= 0 {
			content := strings.TrimSpace(reply[start : start+end])
			if strings.HasPrefix(content, "---") {
				return content
			}
		}
	}

	// Case 3: Spec inside a generic ``` code block.
	if idx := strings.Index(reply, "```"); idx >= 0 {
		start := idx + 3
		// Skip language identifier line if present.
		if nl := strings.IndexByte(reply[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		if end := strings.Index(reply[start:], "```"); end >= 0 {
			content := strings.TrimSpace(reply[start : start+end])
			if strings.HasPrefix(content, "---") {
				return content
			}
		}
	}

	// Case 4: Look for frontmatter starting anywhere in the reply (after "---\ntitle:").
	if idx := strings.Index(reply, "\n---\n"); idx >= 0 {
		after := strings.TrimSpace(reply[idx+1:]) // skip the leading \n
		if strings.HasPrefix(after, "---\n") {
			// Find the closing --- and include everything.
			if end := strings.Index(after[4:], "\n---\n"); end >= 0 {
				return strings.TrimSpace(after[:end+4])
			}
		}
	}

	// Case 5: Look for any "## Acceptance checks" or "## Acceptance Checks" section
	// and treat the entire reply from that point backward as the spec.
	if idx := findAcceptanceChecksHeading(reply); idx >= 0 {
		// Try to find frontmatter before this heading.
		for start := idx; start >= 0; start-- {
			if strings.HasPrefix(reply[start:], "---\n") {
				return strings.TrimSpace(reply[start:])
			}
		}
	}

	// Fallback: return the entire reply.
	return reply
}

// findAcceptanceChecksHeading returns the index of "## Acceptance checks" or
// "## Acceptance Checks" in s, or -1 if not found.
func findAcceptanceChecksHeading(s string) int {
	if idx := strings.Index(s, "## Acceptance checks"); idx >= 0 {
		return idx
	}
	if idx := strings.Index(s, "## Acceptance Checks"); idx >= 0 {
		return idx
	}
	return -1
}
