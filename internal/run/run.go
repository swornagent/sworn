// Package run orchestrates the sworn run turnkey loop: implement → verify →
// (on FAIL: retry/escalate up to N) → gated merge on PASS only.
//
// It is the single-slice v0.1 engine: a task string becomes an auto-generated
// slice, the implementer model builds it, the verifier model checks it, and the
// binary merges only when the verifier returns PASS.
//
// Stdlib only — zero runtime dependencies beyond the internal packages.
package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/implement"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
	"github.com/swornagent/sworn/internal/verify"
)

// DefaultEscalationModels is the default model escalation path when none is
// provided. Each entry is a "provider/model" ID suitable for model.FromEnv.
// The list runs from cheapest to most capable; on retry the next model is used.
var DefaultEscalationModels = []string{
	"openai/gpt-4o-mini",
	"openai/gpt-4o",
	"openai/o3-mini",
	"openai/o3",
}

// Options configures the run loop.
type Options struct {
	// Task is the plain-language task description (required).
	Task string

	// ImplementerModel is the initial implementer model ID (provider/model).
	// If empty, the first entry in EscalationModels is used.
	ImplementerModel string

	// VerifierModel is the verifier model ID (provider/model). Required — the
	// run loop fails closed without a verifier.
	VerifierModel string

	// Base is the branch to merge into on PASS. Default "main".
	Base string

	// RetryCap is the maximum number of retries before escalating to the human.
	// 0 means no retries (single attempt). Use -1 to accept the default (3).
	RetryCap int

	// EscalationModels is the ordered list of model IDs to try on retry.
	// If empty, DefaultEscalationModels is used. The implementer model starts
	// at ImplementerModel (or the first entry) and advances one position on
	// each retry. The verifier model stays fixed.
	EscalationModels []string

	// WorkspaceRoot is the repo root directory. Default ".".
	WorkspaceRoot string

	// NewAgent is a factory for creating an agent.Agent from a model ID.
	// When nil, model.FromEnv is used (production path). Tests inject fakes.
	NewAgent func(modelID string) (agent.Agent, error)

	// NewVerifier is a factory for creating a model.Verifier from a model ID.
	// When nil, model.FromEnv is used (production path). Tests inject fakes.
	NewVerifier func(modelID string) (model.Verifier, error)
}

// Run executes the sworn run turnkey loop. It returns nil only when the
// implementation passed verification and was merged.
func Run(ctx context.Context, opts Options) error {
	if opts.Task == "" {
		return fmt.Errorf("run: --task is required")
	}
	if opts.Base == "" {
		opts.Base = "main"
	}
	if opts.WorkspaceRoot == "" {
		opts.WorkspaceRoot = "."
	}
	if opts.NewAgent == nil {
		opts.NewAgent = newAgentFromModel
	}
	if opts.NewVerifier == nil {
		opts.NewVerifier = newVerifierFromModel
	}

	workspaceRoot, err := filepath.Abs(opts.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("run: resolve workspace: %w", err)
	}

	repo := git.New(workspaceRoot)

	// ── Create auto-generated release + slice ─────────────────────────
	releaseDir, sliceDir, err := setupSlice(workspaceRoot, opts.Task)
	if err != nil {
		return fmt.Errorf("run: setup slice: %w", err)
	}
	absSliceDir := filepath.Join(workspaceRoot, sliceDir)
	specPath := filepath.Join(absSliceDir, "spec.md")
	statusPath := filepath.Join(absSliceDir, "status.json")

	// ── Branch off base ──────────────────────────────────────────────
	featureBranch := sanitiseBranch(opts.Task)

	if err := repo.Checkout(opts.Base); err != nil {
		return fmt.Errorf("run: checkout base %q: %w", opts.Base, err)
	}
	if err := repo.Branch(featureBranch); err != nil {
		return fmt.Errorf("run: create branch %q: %w", featureBranch, err)
	}

	// Stage and commit the auto-generated slice files.
	if err := repo.Stage(releaseDir); err != nil {
		return fmt.Errorf("run: stage slice: %w", err)
	}
	if err := repo.Commit("chore(run): auto-generated slice " + releaseDir); err != nil {
		return fmt.Errorf("run: commit slice: %w", err)
	}

	// Update start_commit so the verifier has an exact diff base.
	startCommit, err := repo.RevParse("HEAD")
	if err != nil {
		return fmt.Errorf("run: rev-parse HEAD: %w", err)
	}
	st, err := state.Read(statusPath)
	if err != nil {
		return fmt.Errorf("run: read status: %w", err)
	}
	st.StartCommit = startCommit
	if err := state.Write(statusPath, st); err != nil {
		return fmt.Errorf("run: write start_commit: %w", err)
	}

	// ── Build escalation list ────────────────────────────────────────
	escalationModels := opts.EscalationModels
	if opts.ImplementerModel != "" {
		escalationModels = append([]string{opts.ImplementerModel}, escalationModels...)
	}
	if len(escalationModels) == 0 {
		escalationModels = DefaultEscalationModels
	}

	// Determine retry cap. RetryCap < 0 means "use default" (all models).
	retryCap := opts.RetryCap
	if retryCap < 0 {
		retryCap = len(escalationModels) - 1
		if retryCap < 0 {
			retryCap = 0
		}
	}

	maxAttempts := retryCap + 1
	if maxAttempts > len(escalationModels) {
		maxAttempts = len(escalationModels)
	}
	var lastVerdict verdict.Result

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Reset slice state for retry (implement.Run rejects "implemented").
		if attempt > 0 {
			st, err := state.Read(statusPath)
			if err != nil {
				return fmt.Errorf("run: read status for retry reset: %w", err)
			}
			st.State = state.InProgress
			st.LastUpdatedBy = "run-loop"
			st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
			st.Verification = state.Verification{}
			if err := state.Write(statusPath, st); err != nil {
				return fmt.Errorf("run: reset status for retry: %w", err)
			}
		}

		implModelID := escalationModels[attempt]
		implAgent, err := opts.NewAgent(implModelID)
		if err != nil {
			return fmt.Errorf("run: create implementer agent for %q: %w", implModelID, err)
		}

		// ── Implement ────────────────────────────────────────────
		fmt.Fprintf(os.Stderr, "sworn run: attempt %d/%d — implementing with %s\n",
			attempt+1, maxAttempts, implModelID)

		if err := implement.Run(ctx, workspaceRoot, specPath, implAgent); err != nil {
			fmt.Fprintf(os.Stderr, "sworn run: implementer error: %v\n", err)
			if attempt+1 < maxAttempts {
				fmt.Fprintf(os.Stderr, "sworn run: escalating implementer model for retry\n")
				continue
			}
			return fmt.Errorf("run: implementer failed after %d attempts: %w", maxAttempts, err)
		}

		// ── Commit agent changes ─────────────────────────────────
		// implement.Run leaves changes in the working tree; commit them
		// so the diff for verification captures the agent's work.
		if err := repo.Stage("."); err != nil {
			return fmt.Errorf("run: stage agent changes: %w", err)
		}
		if err := repo.Commit(fmt.Sprintf("feat(run): implementation attempt %d", attempt+1)); err != nil {
			return fmt.Errorf("run: commit agent changes: %w", err)
		}

		// ── Compute diff ─────────────────────────────────────────
		diff, err := repo.DiffRange(startCommit, "HEAD")
		if err != nil {
			return fmt.Errorf("run: compute diff: %w", err)
		}

		// ── Verify ───────────────────────────────────────────────
		verifierModelID := opts.VerifierModel
		verifier, err := opts.NewVerifier(verifierModelID)
		if err != nil {
			return fmt.Errorf("run: create verifier for %q: %w", verifierModelID, err)
		}

		fmt.Fprintf(os.Stderr, "sworn run: verifying with %s\n", verifierModelID)

		diffPath, err := writeTempFile(workspaceRoot, "sworn-diff-*.patch", diff)
		if err != nil {
			return fmt.Errorf("run: write diff temp: %w", err)
		}

		lastVerdict = verify.Run(ctx, verify.Input{
			SpecPath:  specPath,
			DiffPath:  diffPath,
			ProofPath: filepath.Join(absSliceDir, "proof.md"),
			Model:     verifierModelID,
			Verifier:  verifier,
		})
		os.Remove(diffPath)

		fmt.Fprintf(os.Stderr, "sworn run: verdict %s (cost $%.4f)\n",
			lastVerdict.Verdict, lastVerdict.CostUSD)
		if lastVerdict.Rationale != "" {
			fmt.Fprintf(os.Stderr, "sworn run: rationale: %s\n", lastVerdict.Rationale)
		}

		switch lastVerdict.Verdict {
		case verdict.Pass:
			// ── Transition implemented → verified (Pin 2) ────────────
			st, err := state.Read(statusPath)
			if err != nil {
				return fmt.Errorf("run: read status for verified transition: %w", err)
			}
			if err := st.State.Transition(state.Verified); err != nil {
				return fmt.Errorf("run: transition to verified: %w", err)
			}
			st.State = state.Verified
			st.LastUpdatedBy = "run-loop"
			st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := state.Write(statusPath, st); err != nil {
				return fmt.Errorf("run: write verified status: %w", err)
			}

			// ── Gated merge on PASS only (AC1) ──────────────────────
			if err := repo.Stage("."); err != nil {
				return fmt.Errorf("run: stage for merge: %w", err)
			}
			if err := repo.Commit("chore(run): verified — merge to " + opts.Base); err != nil {
				return fmt.Errorf("run: commit verified state: %w", err)
			}
			if err := repo.Checkout(opts.Base); err != nil {
				return fmt.Errorf("run: checkout base for merge: %w", err)
			}
			if err := repo.Merge(featureBranch); err != nil {
				return fmt.Errorf("run: merge into %s: %w", opts.Base, err)
			}
			fmt.Fprintf(os.Stderr, "sworn run: merged %s into %s (PASS)\n", featureBranch, opts.Base)
			return nil

		case verdict.Blocked:
			return fmt.Errorf("run: verification blocked: %s", lastVerdict.Rationale)

		case verdict.Inconclusive:
			fallthrough
		default:
			if attempt+1 < maxAttempts {
				fmt.Fprintf(os.Stderr, "sworn run: verification failed — retrying with escalated implementer model\n")
				continue
			}
		}
	}

	return fmt.Errorf(
		"run: verification failed after %d attempts (last verdict: %s). "+
			"Escalate to human. Branch %q left unmerged on %s.",
		maxAttempts, lastVerdict.Verdict, featureBranch, opts.Base,
	)
}

// setupSlice creates a release directory and a single-slice directory with
// auto-generated spec.md and status.json (Pin 3). Returns the release dir and
// slice dir (both relative to workspaceRoot).
func setupSlice(workspaceRoot, task string) (releaseDir, sliceDir string, err error) {
	ts := time.Now().UTC().Format("20060102-150405")
	releaseName := "run-" + ts
	releaseDir = filepath.Join("docs", "release", releaseName)
	sliceDir = filepath.Join(releaseDir, "S01-task")

	absSlice := filepath.Join(workspaceRoot, sliceDir)

	if err := os.MkdirAll(absSlice, 0o755); err != nil {
		return "", "", err
	}

	specContent := fmt.Sprintf(`# Task

%s

## User outcome

%s

## Acceptance checks

- [ ] The implementation satisfies the task description

## Required tests

- **Unit/Integration**: go test ./...

## Out of scope

- N/A
`, task, task)
	if err := os.WriteFile(filepath.Join(absSlice, "spec.md"), []byte(specContent), 0o644); err != nil {
		return "", "", err
	}

	st := &state.Status{
		Schema:        "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:       "S01-task",
		Release:       releaseName,
		Track:         "",
		State:         state.InProgress,
		Owner:         "sworn-run",
		LastUpdatedBy: "run-loop",
		LastUpdatedAt: time.Now().UTC().Format(time.RFC3339),
		SpecPath:      filepath.Join(sliceDir, "spec.md"),
		ProofPath:     filepath.Join(sliceDir, "proof.md"),
		JournalPath:   filepath.Join(sliceDir, "journal.md"),
		PlannedFiles:  []string{},
		TestCommands:  []string{"go test ./..."},
		Verification:  state.Verification{},
		ReleaseBase:   "main",
	}
	if err := state.Write(filepath.Join(absSlice, "status.json"), st); err != nil {
		return "", "", err
	}

	return releaseDir, sliceDir, nil
}

// sanitiseBranch converts a task string into a safe branch name.
func sanitiseBranch(task string) string {
	var b strings.Builder
	b.WriteString("sworn/")
	for _, r := range task {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteRune('-')
		}
	}
	name := b.String()
	if len(name) > 50 {
		name = name[:50]
	}
	name = strings.Trim(name, "-")
	if name == "sworn" || name == "sworn/" {
		name = "sworn/task"
	}
	return name
}

func writeTempFile(dir, pattern, content string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func newAgentFromModel(modelID string) (agent.Agent, error) {
	v, err := model.FromEnv(modelID)
	if err != nil {
		return nil, err
	}
	a, ok := v.(agent.Agent)
	if !ok {
		return nil, fmt.Errorf("model %q does not support agent interface", modelID)
	}
	return a, nil
}

func newVerifierFromModel(modelID string) (model.Verifier, error) {
	return model.FromEnv(modelID)
}
