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
	"github.com/swornagent/sworn/internal/gate"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/implement"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/orchestrator"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
	"github.com/swornagent/sworn/internal/verify")

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
// checkProofAbsent returns true when proof.md is absent or empty.
// This is the proof-mandatory gate (S11).
func checkProofAbsent(proofPath string) bool {
	proofBytes, err := os.ReadFile(proofPath)
	return err != nil || strings.TrimSpace(string(proofBytes)) == ""
}


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
	// EVAL SUPERVISOR FIX (2026-06-28): parallel mode's runSliceFn does not wire
	// NewAgent, so it was nil here → SIGSEGV at the design-TL;DR dispatch. Mirror
	// run.Run's default (run.go:107-108) so the parallel loop can dispatch agents.
	if opts.NewAgent == nil {
		opts.NewAgent = newAgentFromModel
	}
	if opts.NewVerifier == nil {
		opts.NewVerifier = newVerifierFromModel
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

	// ── Triage policy (S47) ────────────────────────────────────────────
	// The loop is driven by the triage policy, not a fixed attempt counter.
	// maxResolves (K) is the per-model resolve_in_place budget; default 1 as
	// per the S47 spec. escalate_model advances to the next model; halt
	// commits the terminal state and returns.
	maxResolves := 1
	_ = opts.RetryCap // RetryCap is superseded by the triage policy; kept for API compat.

	absSliceDir := filepath.Dir(specPath)
	if !filepath.IsAbs(specPath) {
		absSliceDir = filepath.Join(worktreeRoot, absSliceDir)
	}
	proofPath := filepath.Join(absSliceDir, "proof.md")
	// ── Resolve implement timeout ──────────────────────────────────────
	// 0 means use default; negative means no timeout; positive is used as-is.
	implementTimeout := opts.ImplementTimeout
	if implementTimeout == 0 {
		implementTimeout = DefaultImplementTimeout
	}

	// dispatches accumulates per-role dispatch costs (S55) for the verdict
	// ledger. Populated by the captain review, implement, and verify stages;
	// written to status.json at each terminal state transition.
	var dispatches []state.Dispatch

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
							// Record captain dispatch for per-role cost ledger (S55).
							dispatches = append(dispatches, state.Dispatch{
								Role:    "captain",
								Model:   firstModelID,
								CostUSD: reviewResult.CostUSD,
								Attempt: 1,
							})
						}
					}
				}
			}
		}
	}

	// ── Triage-driven implement→verify loop (S47) ─────────────────────
	// Replaces the fixed attempt-counter loop with a triage policy that
	// decides resolve_in_place / escalate_model / halt per verifier verdict.
	// Each model in the escalation list gets up to maxResolves (K) same-model
	// retries before the triage advances to the next model; BLOCKED halts
	// immediately.
	var (
		lastVerdict   verdict.Result
		modelIdx      = 0
		resolveCount  = 0
		totalAttempts = 0
		firstAttempt  = true
	)
	for {
		// ── Guard: model index out of range ─────────────────────────
		if modelIdx >= len(escalationModels) {
			// Should not happen with correct triage, but fail-safe.
			return fmt.Errorf("RunSlice: triage advanced model index %d beyond escalation list length %d",
				modelIdx, len(escalationModels))
		}

		// ── Reset slice state for retry ────────────────────────────
		if !firstAttempt {
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
		firstAttempt = false
		totalAttempts++

		implModelID := escalationModels[modelIdx]
		implAgent, err := opts.NewAgent(implModelID)
		if err != nil {
			return fmt.Errorf("RunSlice: create implementer agent for %q: %w", implModelID, err)
		}

		// ── Implement ───────────────────────────────────────────────
		fmt.Fprintf(os.Stderr, "sworn run: attempt %d (model %d/%d, resolve %d/%d) — implementing with %s\n",
			totalAttempts, modelIdx+1, len(escalationModels), resolveCount, maxResolves, implModelID)

		var implCost float64
		var implErr error
		if implementTimeout > 0 {
			implCtx, cancel := context.WithTimeout(ctx, implementTimeout)
			defer cancel() // safe: each iteration has its own defer
			implCost, implErr = implement.Run(implCtx, worktreeRoot, specPath, priorFeedback, implAgent)
		} else {
			implCost, implErr = implement.Run(ctx, worktreeRoot, specPath, priorFeedback, implAgent)
		}

		if implErr != nil {
			// Terminal errors halt immediately (S09 AC1): KindAuth and KindCredits
			// cannot succeed on retry. Return a BLOCKED verdict before the triage
			// path so the orchestrator routes to /replan-release, not retry/escalate.
			if model.IsTerminal(implErr) {
				var me *model.Error
				if model.AsError(implErr, &me) {
					kindLabel := "Kind" + strings.ToUpper(me.Kind.String()[:1]) + me.Kind.String()[1:]
					reason := fmt.Sprintf("%s: %s — halting; check provider credentials",
						kindLabel, me.UserMessage())
					fmt.Fprintf(os.Stderr, "sworn run: terminal error — %s\n", reason)
					return fmt.Errorf("%s%s", errVerdictBlockedPrefix, reason)
				}
				fmt.Fprintf(os.Stderr, "sworn run: terminal error — %v\n", implErr)
				return fmt.Errorf("%s%s", errVerdictBlockedPrefix, implErr.Error())
			}
			if errors.Is(implErr, context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "sworn run: implement attempt %d timed out after %s\n",
					totalAttempts, implementTimeout)
			} else {
				fmt.Fprintf(os.Stderr, "sworn run: implementer error: %v\n", implErr)
			}
			// Triage the implementer error: treat as FAIL for the policy.
			triageOut := orchestrator.Decide(orchestrator.Input{
				Verdict:        verdict.Fail,
				AttemptOnModel: resolveCount,
				ModelIdx:       modelIdx,
				EscalationLen:  len(escalationModels),
				MaxResolves:    maxResolves,
			})
			fmt.Fprintf(os.Stderr, "sworn run: triage (implementer error): %s — %s\n", triageOut.Action, triageOut.Reason)
			switch triageOut.Action {
			case orchestrator.ResolveInPlace:
				resolveCount++
				priorFeedback = fmt.Sprintf("implementer error: %v", implErr)
				continue
			case orchestrator.EscalateModel:
				modelIdx++
				resolveCount = 0
				priorFeedback = fmt.Sprintf("implementer error: %v", implErr)
				continue
			case orchestrator.Halt:
				// Commit failed_verification and return.
				goto haltFailedVerification
			}
		}

		// Record implementer dispatch for per-role cost ledger (S55).
		dispatches = append(dispatches, state.Dispatch{
			Role:    "implementer",
			Model:   implModelID,
			CostUSD: implCost,
			Attempt: totalAttempts,
		})
		// ── Commit agent changes ────────────────────────────────────
		if err := repo.Stage("."); err != nil {
			return fmt.Errorf("RunSlice: stage agent changes: %w", err)
		}
		if err := repo.Commit(fmt.Sprintf("feat(run): implementation attempt %d", totalAttempts)); err != nil {
			return fmt.Errorf("RunSlice: commit agent changes: %w", err)
		}

		// ── Compute diff ────────────────────────────────────────────
		diff, err := repo.DiffRange(startCommit, "HEAD")
		if err != nil {
			return fmt.Errorf("RunSlice: compute diff: %w", err)
		}

		// ── Verify (agentic) ─────────────────────────────────────────
		verifierModelID := opts.VerifierModel

		// ── Proof mandatory gate ───────────────────────────────────
		// Before dispatching the verifier, check that proof.md exists
		// and is non-empty. Absent proof -> BLOCKED immediately.
		if checkProofAbsent(proofPath) {
			lastVerdict = verdict.Result{
				Verdict:    verdict.Blocked,
				FailedGate: "proof_absent",
				Rationale:  "proof bundle absent — fail closed",
			}
			fmt.Fprintf(os.Stderr, "sworn run: BLOCKED — proof bundle absent\n")
			// Record the BLOCKED dispatch for cost ledger.
			dispatches = append(dispatches, state.Dispatch{
				Role:    "verifier",
				Model:   opts.VerifierModel,
				CostUSD: 0,
				Attempt: totalAttempts,
			})
			// Commit blocked state.
			stBlk, _ := state.Read(statusPath)
			if stBlk != nil {
				stBlk.Verification.Result = "blocked"
				stBlk.Verification.Model = opts.VerifierModel
				stBlk.Verification.Attempt = totalAttempts
				stBlk.Verification.Dispatches = dispatches
				stBlk.LastUpdatedBy = "run-slice"
				stBlk.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
				_ = state.Write(statusPath, stBlk)
				_ = repo.Stage(statusPath)
				_ = repo.Commit("chore(run): verification blocked — proof bundle absent")
			}
			// Notify on BLOCKED.
			if opts.Notifier != nil {
				stNotify, _ := state.Read(statusPath)
				if stNotify != nil {
					summary := "proof bundle absent — fail closed"
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
			return fmt.Errorf("RunSlice: verification blocked: proof bundle absent — fail closed")
		}
		// ── No-mock wiring (S10) ────────────────────────────────────
		// Run the mock lint gate before the agentic verifier dispatch.
		// Violations that are not in open_deferrals are appended as
		// warnings; undeclared mocks do not BLOCK by themselves (the
		// deferral path is the user's explicit choice).
		{
			absSliceDir := filepath.Dir(specPath)
			mockReport, mockErr := gate.RunMock(filepath.Dir(absSliceDir), filepath.Base(absSliceDir), startCommit)
			if mockErr == nil && mockReport.HasViolations() {
				fmt.Fprintf(os.Stderr, "sworn run: mock lint: %d undeclared boundary violation(s)\n",
					mockReport.TotalViolations)
				// Append violations as informational warnings to the run log.
				for _, v := range mockReport.Violations {
					fmt.Fprintf(os.Stderr, "sworn run:   - %s:%d %s\n", v.File, v.Line, v.Msg)
				}
			}
		}
		// ── First-pass deterministic gate (S12) ────────────────────
			// RunFirstPass catches structural blockers (empty spec,
			// empty diff, undeclared boundary mocks) before the expensive
			// agentic verifier is dispatched. A FAIL or BLOCKED here
			// short-circuits and prevents the agentic call entirely.
			{
				stFP, _ := state.Read(statusPath)
				var openDeferrals []string
				if stFP != nil {
					openDeferrals = stFP.OpenDeferrals
				}
				// Write diff to temp file for RunFirstPass (it reads paths).
				diffPath, tmpErr := writeTempFile("", "firstpass-diff-*.patch", diff)
				if tmpErr != nil {
					return fmt.Errorf("RunSlice: write diff for first-pass: %w", tmpErr)
				}
				defer os.Remove(diffPath)
				fpResult := verify.RunFirstPass(ctx, verify.Input{
					SpecPath:      specPath,
					DiffPath:      diffPath,
					ProofPath:     proofPath,
					OpenDeferrals: openDeferrals,
				})
				if fpResult.Verdict != verdict.Pass {
					lastVerdict = fpResult
					fmt.Fprintf(os.Stderr, "sworn run: first-pass %s — %s\n",
						fpResult.Verdict, fpResult.Rationale)
					// Record the BLOCKED/FAIL dispatch for cost ledger
					// (zero cost — deterministic).
					dispatches = append(dispatches, state.Dispatch{
						Role:    "first_pass",
						Model:   "deterministic",
						CostUSD: 0,
						Attempt: totalAttempts,
					})
					// Commit blocked/failed state and return.
					stBlk, _ := state.Read(statusPath)
					if stBlk != nil {
						stBlk.Verification.Result = "blocked"
						stBlk.Verification.Model = "first_pass"
						stBlk.Verification.Attempt = totalAttempts
						stBlk.Verification.Dispatches = dispatches
						stBlk.LastUpdatedBy = "run-slice"
						stBlk.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
						_ = state.Write(statusPath, stBlk)
						_ = repo.Stage(statusPath)
						_ = repo.Commit("chore(run): verification blocked — first-pass: " + fpResult.FailedGate)
					}
					if opts.Notifier != nil {
						stNotify, _ := state.Read(statusPath)
						if stNotify != nil {
							summary := fpResult.Rationale
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
					return fmt.Errorf("RunSlice: first-pass %s: %s", fpResult.Verdict, fpResult.Rationale)
				}
			}
			// ── Dispatch agentic verifier ───────────────────────────────		// Create an agent (not just a Verifier) for the verifier model
		// so we can dispatch the full verifier.md role prompt via Chat().
		verifierAgent, vaErr := opts.NewAgent(verifierModelID)
		if vaErr != nil {
			return fmt.Errorf("RunSlice: create agentic verifier for %q: %w", verifierModelID, vaErr)
		}

		fmt.Fprintf(os.Stderr, "sworn run: verifying (agentic) with %s\n", verifierModelID)

		// Read spec and diff content for the agentic payload.
		specContent, specErr := os.ReadFile(specPath)
		if specErr != nil {
			return fmt.Errorf("RunSlice: read spec for agentic verify: %w", specErr)
		}
		proofBytes2, _ := os.ReadFile(proofPath)
		proofStr := string(proofBytes2)
		specStr := string(specContent)

		result, runErr := verify.RunAgentic(ctx, specStr, diff, proofStr, verifierAgent)
		if runErr != nil {
			return fmt.Errorf("RunSlice: agentic verify dispatch: %w", runErr)
		}
		lastVerdict = result

		fmt.Fprintf(os.Stderr, "sworn run: verdict %s (cost $%.4f)\n",
			lastVerdict.Verdict, lastVerdict.CostUSD)
		if lastVerdict.Rationale != "" {
			fmt.Fprintf(os.Stderr, "sworn run: rationale: %s\n", lastVerdict.Rationale)
		}

		// Record verifier dispatch for per-role cost ledger (S55).
		dispatches = append(dispatches, state.Dispatch{
			Role:    "verifier",
			Model:   opts.VerifierModel,
			CostUSD: lastVerdict.CostUSD,
			Attempt: totalAttempts,
		})
		// ── PASS: transition to verified ────────────────────────────
		if lastVerdict.Verdict == verdict.Pass {
			st, err := state.Read(statusPath)
			if err != nil {
				return fmt.Errorf("RunSlice: read status for verified transition: %w", err)
			}
			if err := st.State.Transition(state.Verified); err != nil {
				return fmt.Errorf("RunSlice: transition to verified: %w", err)
			}
			st.State = state.Verified
			st.Verification.Model = opts.VerifierModel
			st.Verification.VerifierWasFreshContext = boolPtr(true)
			st.Verification.Dispatches = dispatches
			st.Verification.Attempt = totalAttempts
			st.LastUpdatedBy = "run-slice"
			st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := state.Write(statusPath, st); err != nil {
				return fmt.Errorf("RunSlice: write verified status: %w", err)
			}
			return nil
		}

	// triageVerdict label no longer needed
		// ── Non-PASS: run triage (S47) ──────────────────────────────
		triageOut := orchestrator.Decide(orchestrator.Input{
			Verdict:        lastVerdict.Verdict,
			AttemptOnModel: resolveCount,
			ModelIdx:       modelIdx,
			EscalationLen:  len(escalationModels),
			MaxResolves:    maxResolves,
		})
		fmt.Fprintf(os.Stderr, "sworn run: triage: %s — %s\n", triageOut.Action, triageOut.Reason)

		switch triageOut.Action {
		case orchestrator.ResolveInPlace:
			// Retry same model with S44 feedback (the verifier's rationale).
			resolveCount++
			priorFeedback = lastVerdict.Rationale
			continue

		case orchestrator.EscalateModel:
			// Advance to next model; reset per-model resolve counter.
			modelIdx++
			resolveCount = 0
			priorFeedback = lastVerdict.Rationale
			continue

		case orchestrator.Halt:
			// Commit terminal state.
			if lastVerdict.Verdict == verdict.Blocked {
				// ── BLOCKED: commit blocked with violations ─────────
				st, stErr := state.Read(statusPath)
				if stErr == nil {
					st.Verification.Result = "blocked"
					st.Verification.Violations = extractViolations(lastVerdict.Rationale)
					st.Verification.Model = opts.VerifierModel
					st.Verification.Attempt = totalAttempts
					st.Verification.Dispatches = dispatches
					st.LastUpdatedBy = "run-slice"
					st.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)
					_ = state.Write(statusPath, st)
					_ = repo.Stage(statusPath)
					_ = repo.Commit("chore(run): verification blocked — router (S58) will route to replan-release")
				}
				// Notify on BLOCKED.
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
			}

			// ── FAIL/Inconclusive exhausted: transition to failed_verification ─
			goto haltFailedVerification
		}
	}

haltFailedVerification:
	// ── Transition to failed_verification ────────────────────────────────
	st, stErr := state.Read(statusPath)
	if stErr == nil {
		_ = st.State.Transition(state.FailedVerification) // ignore — state may already be terminal
		st.State = state.FailedVerification
		st.Verification.Model = opts.VerifierModel
		st.Verification.Attempt = totalAttempts
		st.Verification.Dispatches = dispatches
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
		totalAttempts, lastVerdict.Verdict, worktreeRoot,
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

// extractViolations parses a verifier rationale string into a slice of
// individual violation strings. It handles numbered (1. ...) and bulleted
// (- ...) items. If no structured items are found, the entire rationale is
// treated as a single violation.
// This is used by the BLOCKED halt path (S47) to populate
// status.json → verification.violations so the S38 guard
// (ValidateBlockedViolations) passes.
func extractViolations(rationale string) []string {
	if rationale == "" {
		return []string{"(no rationale provided)"}
	}
	lines := strings.Split(rationale, "\n")
	var violations []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Match "1. ...", "2. ...", etc., or "- ...", or "* ..."
		if len(trimmed) >= 3 && trimmed[0] >= '0' && trimmed[0] <= '9' && trimmed[1] == '.' && trimmed[2] == ' ' {
			violations = append(violations, strings.TrimSpace(trimmed[2:]))
		} else if strings.HasPrefix(trimmed, "- ") {
			violations = append(violations, strings.TrimSpace(trimmed[2:]))
		} else if strings.HasPrefix(trimmed, "* ") {
			violations = append(violations, strings.TrimSpace(trimmed[2:]))
		}
	}
	if len(violations) == 0 {
		// No structured items found — use the entire rationale.
		return []string{rationale}
	}
	return violations
}

// Sentinel error string prefixes used by RunSlice. Callers can use
// strings.Contains on the returned error to distinguish exit causes.
const (	errVerdictBlockedPrefix = "RunSlice: verification blocked:"
	errVerdictFailPrefix    = "RunSlice: verification failed after"
)

// boolPtr returns a pointer to a bool value. Used for nullable bool fields
// in status.json (e.g. verifier_was_fresh_context).
func boolPtr(b bool) *bool { return &b }

// IsBlocked reports whether err is a BLOCKED-verdict error from RunSlice.
func IsBlocked(err error) bool {	return err != nil && strings.Contains(err.Error(), errVerdictBlockedPrefix)
}

// IsFailed reports whether err is a FAIL-exhausted error from RunSlice.
func IsFailed(err error) bool {
	return err != nil && strings.Contains(err.Error(), errVerdictFailPrefix)
}
