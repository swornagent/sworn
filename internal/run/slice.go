package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/captain"
	"github.com/swornagent/sworn/internal/design"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/implement"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
	"github.com/swornagent/sworn/internal/verify"
)
// DefaultImplementTimeout is the per-attempt deadline applied to the implement
// step inside RunSlice when no explicit timeout is configured. 15 minutes is
// generous enough for most implement steps but prevents a hung agent from
// blocking the escalation loop indefinitely. It lives in this package (not
// internal/config) so S42 does not collide with config.go ownership.
const DefaultImplementTimeout = 15 * time.Minute

// RunSliceOptions configure the RunSlice retry loop. These are a subset of
// Options — setup-level concerns (Task, Base, WorkspaceRoot) live in Options
// and are handled by Run() or the scheduler worker (S02b).
type RunSliceOptions struct {
	// ImplementerModel is the initial implementer model ID (provider/model).
	// If empty, the first entry in EscalationModels is used.
	ImplementerModel string

	// VerifierModel is the verifier model ID (provider/model). Required.
	VerifierModel string

	// EscalationModels is the ordered list of model IDs to try on retry.
	// If empty, DefaultEscalationModels is used.
	EscalationModels []string

	// RetryCap is the maximum number of retries. 0 = single attempt.
	RetryCap int

	// ImplementTimeout is the per-attempt deadline for the implement step.
	// 0 means use DefaultImplementTimeout.
	// A negative value means no timeout (opt-out).
	ImplementTimeout time.Duration
	// NewAgent is a factory for creating an agent.Agent from a model ID.
	// When nil, model.FromEnv is used (production path).
	NewAgent func(modelID string) (agent.Agent, error)

	// NewVerifier is a factory for creating a model.Verifier from a model ID.
	// When nil, model.FromEnv is used (production path).
	NewVerifier func(modelID string) (model.Verifier, error)

	// Notifier is the notification dispatcher for FAIL/BLOCKED verdicts.
	// When nil, notifications are skipped (test path).
	//
	// This is a one-method interface seam so internal/run tests can inject a
	// recording fake without depending on a live *account.Notifier. The
	// production *account.Notifier satisfies it implicitly (S07-paging AC1
	// integration test).
	Notifier Notifier

	// RegenerateDesign forces regeneration of the design-TL;DR (design.md)
	// even if it already exists. When false, an existing design.md is left
	// untouched (S45-design-tldr AC2).
	RegenerateDesign bool
}

// Notifier is the one-method seam for dispatching FAIL/BLOCKED notifications.
// *account.Notifier satisfies it; tests supply fakes. Declared in the consumer
// package (internal/run) rather than account so the test injection point lives
// next to the wiring it exercises (Rule 1 reachability).
type Notifier interface {
	Notify(ctx context.Context, event account.NotifyEvent)
}

// RunSlice executes the implement→verify retry loop for one slice in an
// existing worktree. It assumes the worktree exists, spec.md is at specPath,
// status.json is at statusPath, and the branch is already checked out.
//
// RunSlice owns: the implement→verify retry loop, verdict handling, and state
// transitions (→ verified on PASS, → failed_verification on FAIL exhausted).
//
// RunSlice does NOT: create branches, commit the merge, or manage git-level
// setup — those remain in Run() for the turnkey path and will be handled by
// the scheduler worker in S02b.
//
// On verifier PASS: transitions status.json to verified and returns nil.
// On verifier BLOCKED: returns error immediately (no state change).
// On verifier FAIL after all retries: transitions to failed_verification and
// returns a non-nil error.
func RunSlice(ctx context.Context, worktreeRoot, specPath, statusPath string, opts RunSliceOptions) error {
	// ── Validate mandatory options ────────────────────────────────────
	if specPath == "" {
		return fmt.Errorf("RunSlice: specPath is required")
	}
	if statusPath == "" {
		return fmt.Errorf("RunSlice: statusPath is required")
	}
	if opts.VerifierModel == "" {
		return fmt.Errorf("RunSlice: VerifierModel is required")
	}

	repo := git.New(worktreeRoot)

	// ── Read start_commit from status.json (canonical source) ─────────
	st, err := state.Read(statusPath)
	if err != nil {
		return fmt.Errorf("RunSlice: read status: %w", err)
	}
	startCommit := st.StartCommit
	if startCommit == "" {
		return fmt.Errorf("RunSlice: start_commit not set in %s", statusPath)
	}

	// ── Build escalation list ─────────────────────────────────────────
	escalationModels := opts.EscalationModels
	if opts.ImplementerModel != "" {
		escalationModels = append([]string{opts.ImplementerModel}, escalationModels...)
	}
	if len(escalationModels) == 0 {
		escalationModels = DefaultEscalationModels
	}

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

	absSliceDir := filepath.Join(worktreeRoot, filepath.Dir(specPath))
	proofPath := filepath.Join(absSliceDir, "proof.md")

	// ── Resolve implement timeout ──────────────────────────────────────
	// 0 means use default; negative means no timeout; positive is used as-is.
	implementTimeout := opts.ImplementTimeout
	if implementTimeout == 0 {
		implementTimeout = DefaultImplementTimeout
	}

	// ── Design TL;DR (S45) ────────────────────────────────────────────
	// Generate design.md before the implement loop so the captain review
	// stage (S46) has an artefact to gate on. The TL;DR uses the first
	// implementer model, bounded by the same timeout as the implement
	// step. A hung TL;DR call must not wedge the run: on timeout, warn
	// and proceed without design.md.
	{
		spec, specErr := os.ReadFile(specPath)
		if specErr == nil {
			firstModelID := escalationModels[0]
			designAgent, daErr := opts.NewAgent(firstModelID)
			if daErr == nil {
				designCtx := ctx
				var designCancel context.CancelFunc
				if implementTimeout > 0 {
					designCtx, designCancel = context.WithTimeout(ctx, implementTimeout)
					defer designCancel()
				}
				fmt.Fprintf(os.Stderr, "sworn run: generating design TL;DR with %s\n", firstModelID)
				_, genErr := design.Generate(designCtx, absSliceDir, string(spec), designAgent,
					design.GenerateOptions{Regenerate: opts.RegenerateDesign})
				if genErr != nil {
					if errors.Is(genErr, context.DeadlineExceeded) {
						fmt.Fprintf(os.Stderr, "sworn run: design TL;DR timed out after %s — proceeding without design.md\n", implementTimeout)
					} else {
						fmt.Fprintf(os.Stderr, "sworn run: design TL;DR: %v — proceeding without design.md\n", genErr)
					}
				} else {
					fmt.Fprintf(os.Stderr, "sworn run: design TL;DR written to %s\n",
						filepath.Join(absSliceDir, "design.md"))
				}
			}
		}
	}

	// ── Captain Review (S46) ──────────────────────────────────────────
	// After design TL;DR, run the captain design-review. Escalate pins
	// halt the run; mechanical/memory-cited pins feed the implementer.
	var priorFeedback string
	{
		designPath := filepath.Join(absSliceDir, "design.md")
		if designBytes, err := os.ReadFile(designPath); err == nil {
			specBytes, specErr := os.ReadFile(specPath)
			if specErr == nil {
				firstModelID := escalationModels[0]
				captainAgent, caErr := opts.NewAgent(firstModelID)
				if caErr == nil {
					// Transition to DesignReview before the review.
					stReview, _ := state.Read(statusPath)
					if stReview != nil && stReview.State != state.DesignReview {
						_ = stReview.State.Transition(state.DesignReview)
						stReview.State = state.DesignReview
						stReview.LastUpdatedBy = "run-slice"
						stReview.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
						_ = state.Write(statusPath, stReview)
					}

					reviewCtx := ctx
					var reviewCancel context.CancelFunc
					if implementTimeout > 0 {
						reviewCtx, reviewCancel = context.WithTimeout(ctx, implementTimeout)
						defer reviewCancel()
					}
					fmt.Fprintf(os.Stderr, "sworn run: running captain design-review with %s\n", firstModelID)
					reviewResult, revErr := captain.Review(reviewCtx, absSliceDir, string(specBytes), string(designBytes), captainAgent, worktreeRoot)
					if revErr != nil {
						if errors.Is(revErr, context.DeadlineExceeded) {
							fmt.Fprintf(os.Stderr, "sworn run: captain review timed out — proceeding without review\n")
						} else {
							fmt.Fprintf(os.Stderr, "sworn run: captain review error: %v — proceeding without review\n", revErr)
						}
					} else if reviewResult.HasEscalatePins {
						fmt.Fprintf(os.Stderr, "sworn run: captain review halted — %d escalate pins in %s\n",
							reviewResult.EscalateCount, filepath.Join(absSliceDir, "review.md"))
						return fmt.Errorf("RunSlice: captain review found %d escalate pins — review at %s. Resolve and re-run.",
							reviewResult.EscalateCount, filepath.Join(absSliceDir, "review.md"))
					} else {
						// Inject mechanical/memory-cited pins into the implementer
						// prompt. The S44 mechanism (priorFeedback) carries these
						// into the first implement attempt.
						if fb := reviewResult.FormatPinsAsFeedback(); fb != "" {
							priorFeedback = fb
						}
					}
				}
			}
		}
	}

	var lastVerdict verdict.Result
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// ── Reset slice state for retry ────────────────────────────────
		// Capture the prior verdict's rationale before clearing verification
		// so the next implement attempt can receive actionable feedback.
		if attempt > 0 {
			priorFeedback = lastVerdict.Rationale

			st, err := state.Read(statusPath)
			if err != nil {
				return fmt.Errorf("RunSlice: read status for retry reset: %w", err)
			}
			st.State = state.InProgress
			st.LastUpdatedBy = "run-slice"
			st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
			st.Verification = state.Verification{}
			if err := state.Write(statusPath, st); err != nil {
				return fmt.Errorf("RunSlice: reset status for retry: %w", err)
			}
		}
		implModelID := escalationModels[attempt]
		implAgent, err := opts.NewAgent(implModelID)
		if err != nil {
			return fmt.Errorf("RunSlice: create implementer agent for %q: %w", implModelID, err)
		}

		// ── Implement ──────────────────────────────────────────────────
		fmt.Fprintf(os.Stderr, "sworn run: attempt %d/%d — implementing with %s\n",
			attempt+1, maxAttempts, implModelID)

		var implErr error
		if implementTimeout > 0 {
			implCtx, cancel := context.WithTimeout(ctx, implementTimeout)
			defer cancel() // safe: each iteration has its own defer
			implErr = implement.Run(implCtx, worktreeRoot, specPath, priorFeedback, implAgent)
		} else {
			implErr = implement.Run(ctx, worktreeRoot, specPath, priorFeedback, implAgent)
		}

		if implErr != nil {
			if errors.Is(implErr, context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "sworn run: implement attempt %d timed out after %s — escalating\n",
					attempt+1, implementTimeout)
			} else {
				fmt.Fprintf(os.Stderr, "sworn run: implementer error: %v\n", implErr)
			}
			if attempt+1 < maxAttempts {
				fmt.Fprintf(os.Stderr, "sworn run: escalating implementer model for retry\n")
				continue
			}
			return fmt.Errorf("RunSlice: implementer failed after %d attempts (last error: %w). "+
				"Escalate to human.", maxAttempts, implErr)
		}
		// ── Commit agent changes ───────────────────────────────────────
		if err := repo.Stage("."); err != nil {
			return fmt.Errorf("RunSlice: stage agent changes: %w", err)
		}
		if err := repo.Commit(fmt.Sprintf("feat(run): implementation attempt %d", attempt+1)); err != nil {
			return fmt.Errorf("RunSlice: commit agent changes: %w", err)
		}

		// ── Compute diff ───────────────────────────────────────────────
		diff, err := repo.DiffRange(startCommit, "HEAD")
		if err != nil {
			return fmt.Errorf("RunSlice: compute diff: %w", err)
		}

		// ── Verify ─────────────────────────────────────────────────────
		verifierModelID := opts.VerifierModel
		verifier, err := opts.NewVerifier(verifierModelID)
		if err != nil {
			return fmt.Errorf("RunSlice: create verifier for %q: %w", verifierModelID, err)
		}

		fmt.Fprintf(os.Stderr, "sworn run: verifying with %s\n", verifierModelID)

		diffPath, err := writeTempFile(worktreeRoot, "sworn-diff-*.patch", diff)
		if err != nil {
			return fmt.Errorf("RunSlice: write diff temp: %w", err)
		}

		// Read open_deferrals from status.json for boundary-mock check (S10).
		status, stErr := state.Read(statusPath)
		var openDeferrals []string
		if stErr == nil {
			openDeferrals = status.OpenDeferrals
		}

		lastVerdict = verify.Run(ctx, verify.Input{
			SpecPath:      specPath,
			DiffPath:      diffPath,
			ProofPath:     proofPath,
			Model:         verifierModelID,
			Verifier:      verifier,
			OpenDeferrals: openDeferrals,
		})
		os.Remove(diffPath)

		fmt.Fprintf(os.Stderr, "sworn run: verdict %s (cost $%.4f)\n",
			lastVerdict.Verdict, lastVerdict.CostUSD)
		if lastVerdict.Rationale != "" {
			fmt.Fprintf(os.Stderr, "sworn run: rationale: %s\n", lastVerdict.Rationale)
		}

		switch lastVerdict.Verdict {
		case verdict.Pass:
			// ── Transition implemented → verified ──────────────────────
			st, err := state.Read(statusPath)
			if err != nil {
				return fmt.Errorf("RunSlice: read status for verified transition: %w", err)
			}
			if err := st.State.Transition(state.Verified); err != nil {
				return fmt.Errorf("RunSlice: transition to verified: %w", err)
			}
			st.State = state.Verified
			st.LastUpdatedBy = "run-slice"
			st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := state.Write(statusPath, st); err != nil {
				return fmt.Errorf("RunSlice: write verified status: %w", err)
			}
			return nil

		case verdict.Blocked:
			if opts.Notifier != nil {
				stNotify, _ := state.Read(statusPath)
				if stNotify != nil {
					summary := lastVerdict.Rationale
					if len(summary) > 200 {
						summary = summary[:197] + "..."
					}
					opts.Notifier.Notify(ctx, account.NotifyEvent{
						Release:           stNotify.Release,
						Track:             stNotify.Track,
						SliceID:           stNotify.SliceID,
						State:             "blocked",
						ViolationsSummary: summary,
						WorktreePath:      worktreeRoot,
					})
				}
			}
			return fmt.Errorf("RunSlice: verification blocked: %s", lastVerdict.Rationale)
		case verdict.Inconclusive:
			fallthrough
		default:
			if attempt+1 < maxAttempts {
				fmt.Fprintf(os.Stderr, "sworn run: verification failed — retrying with escalated implementer model\n")
				continue
			}
		}
	}

	// ── All attempts exhausted: transition to failed_verification ─────
	st, stErr := state.Read(statusPath)
	if stErr == nil {
		_ = st.State.Transition(state.FailedVerification) // ignore — state may already be terminal
		st.State = state.FailedVerification
		st.LastUpdatedBy = "run-slice"
		st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = state.Write(statusPath, st)
		// Commit the state transition so the working tree stays clean
		// for the caller (e.g. for checkout or further git operations).
		_ = repo.Stage(statusPath)
		_ = repo.Commit("chore(run): transition to failed_verification")

		// Notify on FAIL verdict after state is written.
		if opts.Notifier != nil {
			summary := account.ViolationsSummary(proofPath, len(st.Verification.Violations))
			opts.Notifier.Notify(ctx, account.NotifyEvent{
				Release:           st.Release,
				Track:             st.Track,
				SliceID:           st.SliceID,
				State:             "failed_verification",
				ViolationsSummary: summary,
				WorktreePath:      worktreeRoot,
			})
		}
	}
	return fmt.Errorf(
		"RunSlice: verification failed after %d attempts (last verdict: %s). "+
			"Escalate to human. Slice reached failed_verification on worktree %s.",
		maxAttempts, lastVerdict.Verdict, worktreeRoot,
	)
}

// writeTempFile writes content to a temporary file in dir matching pattern.
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

// Sentinel error string prefixes used by RunSlice. Callers can
// strings.Contains on the returned error to distinguish exit causes.
const (
	errVerdictBlockedPrefix = "RunSlice: verification blocked:"
	errVerdictFailPrefix    = "RunSlice: verification failed after"
)

// IsBlocked reports whether err is a BLOCKED-verdict error from RunSlice.
func IsBlocked(err error) bool {
	return err != nil && strings.Contains(err.Error(), errVerdictBlockedPrefix)
}

// IsFailed reports whether err is a FAIL-exhausted error from RunSlice.
func IsFailed(err error) bool {
	return err != nil && strings.Contains(err.Error(), errVerdictFailPrefix)
}
