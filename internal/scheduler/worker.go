package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/orchestrator"
	"github.com/swornagent/sworn/internal/router"
	"github.com/swornagent/sworn/internal/supervisor"
	"github.com/swornagent/sworn/internal/tracklog"
)

// TrackResult is the outcome of a single worker goroutine.
type TrackResult string

const (
	TrackPass    TrackResult = "pass"
	TrackFail    TrackResult = "fail"
	TrackSkipped TrackResult = "skipped"
	TrackPaused  TrackResult = "paused"
	TrackBlocked TrackResult = "blocked"
)

// ── Router interface (S59) ──────────────────────────────────────────────

// SliceDecision is the router's output for a slice — what action to take next.
type SliceDecision struct {
	// Type is the action kind: "implement", "verify", "redesign",
	// "coach_decision", "replan-release", "merge-track", "merge-release",
	// "none".
	Type string

	// Reason is a human-readable explanation.
	Reason string

	// Target is the slice ID to advance to (set when the router walks to
	// the next slice in the track after a verified slice).
	Target string
}

// SliceRouter is the interface the worker polls for route decisions.
// The production implementation wraps internal/router; tests supply
// a fake that returns scripted decisions.
type SliceRouter interface {
	Route(ctx context.Context, release, sliceID, trackID string) (SliceDecision, error)
}

// ── Pause set (S59 spec, Captain ratified) ──────────────────────────────

// pauseSet is the set of router decisions that pause a track rather than
// failing it. These surface to the human (via stderr prefix) and let other
// tracks continue.
var pauseSet = map[string]bool{
	"review":         true,
	"coach_decision": true,
	"replan-release": true,
	"merge-track":    true,
	"merge-release":  true,
}

// ── WorkerOptions ───────────────────────────────────────────────────────

// WorkerOptions configures a single-track worker goroutine.
type WorkerOptions struct {
	// ReleaseName is the release name (e.g. "2026-06-19-safe-parallelism").
	ReleaseName string

	// LogDir, when non-empty, is the directory into which the worker tees its
	// stderr narration (append-only, one <track>.log per track, versioned by
	// tracklog.FormatHeader). Empty = today's behaviour exactly (stderr only).
	// Set by RunParallel to .sworn/logs/<release>. See internal/tracklog.
	LogDir string

	// TrackInfo is the parsed board track entry.
	TrackInfo board.TrackInfo

	// ReleaseWorktreePath is the absolute path to the release worktree.
	// Must exist before the worker starts (ensured by pre-flight step).
	ReleaseWorktreePath string

	// PrimaryWorktreeRoot is the primary repo root, used as fallback
	// for git commands when the track worktree doesn't exist yet.
	PrimaryWorktreeRoot string

	// DB is the SQLite database handle for the supervisor.
	DB *sql.DB

	// EventDB is the release-specific event store database handle. When set,
	// the supervisor routes event writes to this DB instead of DB. When nil,
	// events are written to DB (backward-compatible default).
	EventDB *sql.DB

	// RunSliceFn is the function that runs a single slice's implement→verify
	// loop. Tests inject a fake; production uses run.RunSlice.
	RunSliceFn func(ctx context.Context, worktreeRoot, specPath, statusPath string) error

	// Router is the SliceRouter the worker polls for route decisions.
	// When nil, the worker falls back to static iteration for backward
	// compatibility (RunTrackLegacy behaviour).
	Router SliceRouter

	// Oracle is the committed-state reader consumed by findFirstNonTerminal
	// to seed the resume frontier from git-visible state (S07). When nil,
	// findFirstNonTerminal falls back to returning slices[0] (legacy).
	// The production path sets this from the live oracle via RunParallel.
	Oracle router.OracleReader
	// ProjectDir is the project directory name used for worktree naming.
	ProjectDir string

	// Notifier is the notification dispatcher for track-level failures.
	// When nil, notifications are skipped.
	Notifier *account.Notifier

	// PauseCh is the cooperative pause signal for this release. When this
	// channel is closed, the worker stops at the next router-poll boundary
	// (after completing any in-flight dispatch). Set via PauseEngine.PauseCh.
	// When nil, no cooperative pause is checked.
	PauseCh <-chan struct{}

	// RecordBlocked, when non-nil, is invoked exactly once when a slice
	// returns a blocked-terminal error (the orchestrator.BlockedLaneSentinel
	// shape RunSlice emits for verifier BLOCKED verdicts, implementer
	// StatusBlocked results, and terminal auth/credits driver errors). The
	// reason is the blocker text after the sentinel, verbatim, with the
	// route-directive suffix trimmed. RunParallel wires a collector here so
	// the exit report can distinguish BLOCKED lanes (replan required) from
	// FAIL lanes (retries exhausted) — the scheduler itself keeps returning
	// TrackFail for blocked lanes (S14 D3: no new TrackResult value; the
	// distinction travels via this side-channel only).
	RecordBlocked func(trackID, sliceID, reason string)

	// MergeTrackFn is invoked when a track finishes (all slices terminal).
	// It merges the track branch into the release worktree. When nil,
	// auto-merge is skipped (backward-compatible with tests and legacy
	// callers that don't wire the production merge).
	//
	// The phase barrier in RunParallel (wg.Wait per phase) guarantees that
	// dependent tracks don't start until the earlier phase's goroutines
	// have returned — so by the time finishTrack calls MergeTrackFn, the
	// release-wt HEAD has already been updated before the next phase's
	// goroutines begin. No polling loop is needed; the phase barrier is the
	// ordering mechanism. See Pin 1 in S04 design review.
	//
	// Signature: func(releaseWorktreePath, trackID, trackBranch string) error.
	// The trackID parameter is included so the merge function can update the
	// board's track state to "merged" atomically if desired.
	MergeTrackFn func(releasePath, trackID, branch string) error
}

// RunTrack executes one track's slices sequentially in its own worktree.
// It is designed to be called as a goroutine from RunParallel.
//
// If opts.Router is non-nil, the worker uses a router-driven poll loop:
// it asks the router for the next action for the current frontier slice,
// dispatches the returned action, and loops until the router returns a
// terminal or paused decision. If opts.Router is nil, the worker falls
// back to static slice-iteration (RunTrackLegacy behaviour) for backward
// compatibility.
//
// It returns TrackPass if all slices succeed, TrackFail if any slice fails,
// TrackPaused if the router returns a human-gated decision, or TrackSkipped
// if dependencies indicate the track should be skipped.
func RunTrack(ctx context.Context, opts WorkerOptions) TrackResult {
	trackID := opts.TrackInfo.ID

	// ── Durable-log tee seam (additive, surgical) ───────────────────────
	// w is the narration sink for this track: os.Stderr verbatim PLUS an
	// append-only .sworn/logs/<release>/<track>.log copy when opts.LogDir is
	// set. When LogDir == "" (tests / legacy callers) w IS os.Stderr, so the
	// 37 narration sites below are byte-for-byte unchanged. Constructed once
	// here and threaded as a plain io.Writer — no worker control-flow change.
	w, closeLog := tracklog.NewWriter(opts.LogDir, trackID)
	defer closeLog()

	// ── Check if context is already cancelled (dependency failed) ───────
	if ctx.Err() != nil {
		fmt.Fprintf(w, "[%s] skipped: depends_on failed\n", trackID)
		return TrackSkipped
	}

	fmt.Fprintf(w, "[%s] starting\n", trackID)

	// ── Supervisor acquire ──────────────────────────────────────────────
	sup := supervisor.New(opts.DB, opts.ReleaseName)
	if opts.EventDB != nil {
		sup.SetEventDB(opts.EventDB)
	}
	if err := sup.Acquire(trackID); err != nil {
		fmt.Fprintf(w, "[%s] supervisor acquire error: %v\n", trackID, err)
		return TrackFail
	}

	// Ensure release on all paths.
	releaseTrack := func(finalState string) {
		_ = sup.Release(trackID, finalState)
	}

	trackWorktreePath := opts.TrackInfo.WorktreePath
	trackBranch := opts.TrackInfo.WorktreeBranch

	// ── Materialise track worktree if absent ────────────────────────────
	if trackWorktreePath == "" {
		// internal/board normally DERIVES and populates the path (sworn#80); if it
		// is empty here, derive it from the SAME shared helper — the repo-local
		// sibling-of-release-worktree logic (eval finding 3). Never fall back to a
		// $HOME/projects convention: a wrong path would materialise a worktree on
		// another repo's tree (eval finding 3 / Rule 11), so fail closed instead.
		trackWorktreePath = board.TrackWorktreePathFrom(opts.ReleaseWorktreePath, opts.ReleaseName, trackID)
		if trackWorktreePath == "" {
			fmt.Fprintf(w, "[%s] cannot derive track worktree path: no release worktree path known\n", trackID)
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}
	}

	if !dirExists(trackWorktreePath) {
		fmt.Fprintf(w, "[%s] materialising worktree at %s\n", trackID, trackWorktreePath)

		releaseBranch := "release-wt/" + opts.ReleaseName
		cmd := exec.CommandContext(ctx, "git", "worktree", "add",
			trackWorktreePath, "-b", trackBranch,
			releaseBranch,
		)
		cmd.Dir = opts.PrimaryWorktreeRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(w, "[%s] worktree materialisation failed: %v\n  %s\n",
				trackID, err, string(output))
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}
		fmt.Fprintf(w, "[%s] worktree materialised at %s\n", trackID, trackWorktreePath)

		mergeCmd := exec.Command("git", "merge", releaseBranch, "--no-edit")
		mergeCmd.Dir = trackWorktreePath
		if mergeOut, mergeErr := mergeCmd.CombinedOutput(); mergeErr != nil {
			fmt.Fprintf(w, "[%s] forward-merge note: %s\n", trackID, string(mergeOut))
		}
	}

	// ── Fallback: no router → static iteration ─────────────────────────
	if opts.Router == nil {
		return runTrackLegacy(ctx, opts, w, trackWorktreePath, trackID, trackBranch, releaseTrack)
	}

	// ── Router-driven poll loop ─────────────────────────────────────────
	return runTrackRouter(ctx, opts, w, trackWorktreePath, trackID, trackBranch, releaseTrack)
}

// runTrackRouter is the router-driven execution loop (S59 core).
// It polls the router for the current frontier slice, dispatches the
// returned action, and loops until the router returns a terminal or
// paused decision.
func runTrackRouter(
	ctx context.Context,
	opts WorkerOptions,
	w io.Writer,
	workRoot, trackID, trackBranch string,
	releaseTrack func(string),
) TrackResult {
	specBase := filepath.Join("docs", "release", opts.ReleaseName)

	// Determine the first non-terminal slice in the track.
	currentSlice := findFirstNonTerminal(ctx, opts.Oracle, opts.ReleaseName, opts.TrackInfo.ID, opts.TrackInfo.Slices)
	if currentSlice == "" {
		// All slices already in a terminal state.
		return finishTrack(ctx, opts, w, workRoot, trackID, trackBranch, releaseTrack)
	}

	for {
		// Check context before every iteration.
		if ctx.Err() != nil {
			fmt.Fprintf(w, "[%s] cancelled at slice %s\n", trackID, currentSlice)
			releaseTrack(supervisor.StateFailed)
			return TrackSkipped
		}

		// Cooperative pause check — fires after any in-flight dispatch
		// completes, before the next router poll. The engine layer
		// (PauseEngine) closes the channel; this is a non-blocking check
		// so a nil channel (release not paused) is always a no-op.
		if opts.PauseCh != nil {
			select {
			case <-opts.PauseCh:
				fmt.Fprintf(w, "[%s] engine pause signal at slice %s — stopping\n", trackID, currentSlice)
				releaseTrack("paused")
				return TrackPaused
			default:
			}
		}

		// Poll the router for the current frontier slice.
		decision, err := opts.Router.Route(ctx, opts.ReleaseName, currentSlice, trackID)
		if err != nil {
			fmt.Fprintf(w, "[%s] router error for %s: %v\n", trackID, currentSlice, err)
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}

		fmt.Fprintf(w, "[%s] router: %s → %s (%s)\n",
			trackID, currentSlice, decision.Type, decision.Reason)

		// Record the routing decision (S02 — decision log). Best-effort:
		// a decision-log write failure must not abort the run (AC4).
		_ = supervisor.RecordDecision(opts.DB, opts.ReleaseName, currentSlice,
			decision.Type, decision.Reason)

		// Advance to the target slice BEFORE dispatching — the router's
		// Target field tells us which slice the decision applies to.
		if decision.Target != "" && decision.Target != currentSlice {
			fmt.Fprintf(w, "[%s] advanced to next slice: %s\n", trackID, decision.Target)
			currentSlice = decision.Target
		}

		switch decision.Type {
		case "implement", "verify":
			// Both implement and verify dispatch to RunSliceFn, which handles
			// the full implement→verify loop in production (run.RunSlice).
			// A separate verify-only step would be needed for a genuine
			// "implemented but not verified" resume, but RunSlice already
			// handles both phases atomically.
			specPath := filepath.Join(workRoot, specBase, currentSlice, "spec.md")
			statusPath := filepath.Join(workRoot, specBase, currentSlice, "status.json")

			fmt.Fprintf(w, "[%s] running slice %s\n", trackID, currentSlice)

			if err := opts.RunSliceFn(ctx, workRoot, specPath, statusPath); err != nil {
				// S01: interpreter INCONCLUSIVE → PAGE the Coach (pause, not fail).
				if strings.Contains(err.Error(), orchestrator.InterpreterInconclusiveSentinel) {
					fmt.Fprintf(w, "[%s] paused: interpreter inconclusive for %s — %v\n",
						trackID, currentSlice, err)
					releaseTrack("paused")
					return TrackPaused
				}
				// S03: max-turns exhaustion -> PAGE the Coach (pause, not fail).
				if strings.Contains(err.Error(), agent.MaxTurnsSentinel) {
					fmt.Fprintf(w, "[%s] paused: max turns exhausted for %s - %v\n",
						trackID, currentSlice, err)
					_ = supervisor.RecordPage(opts.DB, opts.ReleaseName, currentSlice, "max_turns")
					releaseTrack("paused")
					return TrackPaused
				}
				// S14: blocked-terminal lane — checked after the pause
				// sentinels, before breaker fingerprinting (see
				// blockedLaneTerminal). Ends the track before any subsequent
				// slice starts (AC-04); TrackFail still triggers failCancel so
				// dependent tracks skip.
				if blockedLaneTerminal(w, opts, trackID, currentSlice, err, releaseTrack) {
					return TrackFail
				}
				// S03 circuit breaker: check cross-run failure fingerprint.
				fingerprint := supervisor.Fingerprint(currentSlice, err.Error())
				_ = supervisor.RecordFailure(opts.DB, opts.ReleaseName, currentSlice, fingerprint)
				if supervisor.ShouldBreak(opts.DB, opts.ReleaseName, currentSlice, fingerprint) {
					fmt.Fprintf(w, "[%s] paused: circuit breaker for %s - %v\n",
						trackID, currentSlice, err)
					_ = supervisor.RecordPage(opts.DB, opts.ReleaseName, currentSlice, "circuit_breaker")
					releaseTrack("paused")
					return TrackPaused
				}
				fmt.Fprintf(w, "[%s] slice %s failed: %v\n", trackID, currentSlice, err)

				if opts.Notifier != nil {
					summary := err.Error()
					if len(summary) > 200 {
						summary = summary[:197] + "..."
					}
					opts.Notifier.Notify(ctx, account.NotifyEvent{
						Release:           opts.ReleaseName,
						Track:             trackID,
						SliceID:           currentSlice,
						State:             "track_failed",
						ViolationsSummary: summary,
						WorktreePath:      workRoot,
					})
				}

				releaseTrack(supervisor.StateFailed)
				return TrackFail
			}

		case "redesign":
			// Strip captain-proceed.md so the Design TL;DR gate fires again on
			// the next implement attempt. Then dispatch implement.
			stripCaptainProceed(w, workRoot, specBase, currentSlice)

			specPath := filepath.Join(workRoot, specBase, currentSlice, "spec.md")
			statusPath := filepath.Join(workRoot, specBase, currentSlice, "status.json")

			fmt.Fprintf(w, "[%s] redesign: stripped captain-proceed.md for %s, re-running\n",
				trackID, currentSlice)
			if err := opts.RunSliceFn(ctx, workRoot, specPath, statusPath); err != nil {
				// S01: interpreter INCONCLUSIVE → PAGE the Coach (pause, not fail).
				if strings.Contains(err.Error(), orchestrator.InterpreterInconclusiveSentinel) {
					fmt.Fprintf(w, "[%s] paused: interpreter inconclusive for %s — %v\n",
						trackID, currentSlice, err)
					releaseTrack("paused")
					return TrackPaused
				}
				// S03: max-turns exhaustion -> PAGE the Coach (pause, not fail).
				if strings.Contains(err.Error(), agent.MaxTurnsSentinel) {
					fmt.Fprintf(w, "[%s] paused: max turns exhausted for %s - %v\n",
						trackID, currentSlice, err)
					_ = supervisor.RecordPage(opts.DB, opts.ReleaseName, currentSlice, "max_turns")
					releaseTrack("paused")
					return TrackPaused
				}
				// S14: blocked-terminal lane (see blockedLaneTerminal).
				if blockedLaneTerminal(w, opts, trackID, currentSlice, err, releaseTrack) {
					return TrackFail
				}
				// S03 circuit breaker: check cross-run failure fingerprint.
				fingerprint := supervisor.Fingerprint(currentSlice, err.Error())
				_ = supervisor.RecordFailure(opts.DB, opts.ReleaseName, currentSlice, fingerprint)
				if supervisor.ShouldBreak(opts.DB, opts.ReleaseName, currentSlice, fingerprint) {
					fmt.Fprintf(w, "[%s] paused: circuit breaker for %s - %v\n",
						trackID, currentSlice, err)
					_ = supervisor.RecordPage(opts.DB, opts.ReleaseName, currentSlice, "circuit_breaker")
					releaseTrack("paused")
					return TrackPaused
				}
				fmt.Fprintf(w, "[%s] slice %s failed after redesign: %v\n", trackID, currentSlice, err)
				releaseTrack(supervisor.StateFailed)
				return TrackFail
			}

		case "merge-track":
			// When MergeTrackFn is wired, auto-merge (same as the terminal
			// "none" path). When nil, preserve the human-gated pause for
			// backward compatibility with callers that haven't wired it yet.
			if opts.MergeTrackFn != nil {
				return finishTrack(ctx, opts, w, workRoot, trackID, trackBranch, releaseTrack)
			}
			// Human-gated pause — surface and pause this track.
			fmt.Fprintf(w, "[%s] paused: %s — %s\n", trackID, decision.Type, decision.Reason)
			releaseTrack("paused")
			return TrackPaused

		case "review", "coach_decision", "replan-release", "merge-release":
			// Human-gated pause states — surface and pause this track.
			// "review" is the Rule 9 design gate: design.md awaits the
			// Captain's /design-review, a pause-for-human, never a failure.
			fmt.Fprintf(w, "[%s] paused: %s — %s\n", trackID, decision.Type, decision.Reason)
			releaseTrack("paused")
			return TrackPaused

		case "none": // Terminal — no more slices.
			return finishTrack(ctx, opts, w, workRoot, trackID, trackBranch, releaseTrack)

		default:
			fmt.Fprintf(w, "[%s] unrecognised router decision %q for %s: %s\n",
				trackID, decision.Type, currentSlice, decision.Reason)
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}
	}
}

// runTrackLegacy is the pre-S59 static-iteration worker, preserved for
// backward compatibility when no Router is configured.
func runTrackLegacy(
	ctx context.Context,
	opts WorkerOptions,
	w io.Writer,
	workRoot, trackID, trackBranch string,
	releaseTrack func(string),
) TrackResult {
	specBase := filepath.Join("docs", "release", opts.ReleaseName)

	for _, sliceID := range opts.TrackInfo.Slices {
		if ctx.Err() != nil {
			fmt.Fprintf(w, "[%s] cancelled at slice %s\n", trackID, sliceID)
			releaseTrack(supervisor.StateFailed)
			return TrackSkipped
		}

		fmt.Fprintf(w, "[%s] running slice %s (legacy)\n", trackID, sliceID)

		specPath := filepath.Join(workRoot, specBase, sliceID, "spec.md")
		statusPath := filepath.Join(workRoot, specBase, sliceID, "status.json")

		if err := opts.RunSliceFn(ctx, workRoot, specPath, statusPath); err != nil {
			// S01: interpreter INCONCLUSIVE → PAGE the Coach (pause, not fail).
			if strings.Contains(err.Error(), orchestrator.InterpreterInconclusiveSentinel) {
				fmt.Fprintf(w, "[%s] paused: interpreter inconclusive for %s — %v\n",
					trackID, sliceID, err)
				releaseTrack("paused")
				return TrackPaused
			}
			// S03: max-turns exhaustion -> PAGE the Coach (pause, not fail).
			if strings.Contains(err.Error(), agent.MaxTurnsSentinel) {
				fmt.Fprintf(w, "[%s] paused: max turns exhausted for %s - %v\n",
					trackID, sliceID, err)
				_ = supervisor.RecordPage(opts.DB, opts.ReleaseName, sliceID, "max_turns")
				releaseTrack("paused")
				return TrackPaused
			}
			// S14: blocked-terminal lane (see blockedLaneTerminal). Returning
			// here ends the per-slice loop before any subsequent slice in the
			// track starts (AC-04).
			if blockedLaneTerminal(w, opts, trackID, sliceID, err, releaseTrack) {
				return TrackFail
			}
			// S03 circuit breaker: check cross-run failure fingerprint.
			fingerprint := supervisor.Fingerprint(sliceID, err.Error())
			_ = supervisor.RecordFailure(opts.DB, opts.ReleaseName, sliceID, fingerprint)
			if supervisor.ShouldBreak(opts.DB, opts.ReleaseName, sliceID, fingerprint) {
				fmt.Fprintf(w, "[%s] paused: circuit breaker for %s - %v\n",
					trackID, sliceID, err)
				_ = supervisor.RecordPage(opts.DB, opts.ReleaseName, sliceID, "circuit_breaker")
				releaseTrack("paused")
				return TrackPaused
			}
			fmt.Fprintf(w, "[%s] slice %s failed: %v\n", trackID, sliceID, err)

			if opts.Notifier != nil {
				summary := err.Error()
				if len(summary) > 200 {
					summary = summary[:197] + "..."
				}
				opts.Notifier.Notify(ctx, account.NotifyEvent{
					Release:           opts.ReleaseName,
					Track:             trackID,
					SliceID:           sliceID,
					State:             "track_failed",
					ViolationsSummary: summary,
					WorktreePath:      workRoot,
				})
			}

			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}
	}

	return finishTrack(ctx, opts, w, workRoot, trackID, trackBranch, releaseTrack)
}

// finishTrack pushes the track branch, auto-merges into release-wt (when
// MergeTrackFn is wired), and releases the supervisor.
//
// ── S05 gate bypass documentation (Pin 2, S04 design review) ──────────
//
// Auto-merge bypasses the sworn merge-track CLI gate (S05). The bypass is
// intentional and each gate is accounted for:
//
// (1) verified-check: satisfied by the router — the router only emits
//
//	"merge-track" / "none" after all slices are verified.
//
// (2) invariant-4 classifier (conflict detection): bare git merge still
//
//	fails on conflict → TrackFail, which surfaces to the human.
//	Diagnostic quality is lower than S05's classifier (no file-level
//	conflict report), but the invariant-2 disjoint-touchpoints guarantee
//	makes conflicts impossible in production.
//
// (3) index.md state update to "merged": not performed by auto-merge.
//
//	The board oracle reads track state from index.md; a bare merge does
//	not update it. This is acceptable because the phase barrier is the
//	ordering mechanism, not state polling. (Per Pin 1 resolution (a):
//	waitForDependencies is dropped; the phase barrier enforces AC1.)
func finishTrack(
	ctx context.Context,
	opts WorkerOptions,
	w io.Writer,
	workRoot, trackID, trackBranch string,
	releaseTrack func(string),
) TrackResult {
	releaseTrack(supervisor.StateDone)

	pushCmd := exec.Command("git", "push", "origin", "HEAD:"+trackBranch)
	pushCmd.Dir = workRoot
	_ = pushCmd.Run()

	// Auto-merge track into release-wt when MergeTrackFn is wired.
	// The phase barrier in RunParallel (wg.Wait per phase) guarantees that
	// dependent tracks don't start until this merge completes — no polling
	// loop needed. See Pin 1 in S04 design review.
	if opts.MergeTrackFn != nil {
		fmt.Fprintf(w, "[%s] auto-merging into release-wt\n", trackID)
		if err := opts.MergeTrackFn(opts.ReleaseWorktreePath, trackID, trackBranch); err != nil {
			fmt.Fprintf(w, "[%s] auto-merge failed: %v\n", trackID, err)
			return TrackFail
		}
		fmt.Fprintf(w, "[%s] auto-merged into release-wt\n", trackID)
	}

	fmt.Fprintf(w, "[%s] done\n", trackID)
	return TrackPass
}

// blockedLaneTerminal classifies a RunSliceFn error as blocked-terminal and,
// when it is, handles the lane: logs, records it via opts.RecordBlocked, and
// releases the supervisor with StateFailed. Returns true when the caller must
// return TrackFail. Shared by the three RunSliceFn-error sites (router
// implement/verify, redesign, legacy loop), checked AFTER the pause sentinels
// and BEFORE circuit-breaker fingerprinting: a blocked lane must not accrue
// breaker pages, and the worker-level track_failed notification is skipped
// because RunSlice already emitted the blocked notification (S14 D6 — this
// also removes the pre-existing double-notify on verifier-blocked lanes).
//
// The supervisor state is StateFailed, never a bare "blocked" string —
// supervisor.Release coerces unknown states to StateDone, which would record
// a blocked (unmergeable, replan-required) track as complete. The
// blocked-vs-failed distinction travels via RecordBlocked only (S14 D3,
// Captain review pin 1).
func blockedLaneTerminal(w io.Writer, opts WorkerOptions, trackID, sliceID string, err error, releaseTrack func(string)) bool {
	if !strings.Contains(err.Error(), orchestrator.BlockedLaneSentinel) {
		return false
	}
	reason := blockedLaneReason(err.Error())
	fmt.Fprintf(w, "[%s] slice %s BLOCKED — terminal for lane (replan required): %s\n",
		trackID, sliceID, reason)
	if opts.RecordBlocked != nil {
		opts.RecordBlocked(trackID, sliceID, reason)
	}
	releaseTrack(supervisor.StateFailed)
	return true
}

// blockedLaneReason extracts the verbatim blocker text following the
// BlockedLaneSentinel, trimming the route-directive suffix RunSlice appends
// on implementer-blocked lanes so the exit report does not render the
// directive twice (S14, Captain review flag (a)).
func blockedLaneReason(errText string) string {
	idx := strings.Index(errText, orchestrator.BlockedLaneSentinel)
	reason := errText[idx+len(orchestrator.BlockedLaneSentinel):]
	reason = strings.TrimSuffix(strings.TrimSpace(reason), strings.TrimSpace(orchestrator.BlockedLaneRouteSuffix))
	return strings.TrimSpace(reason)
}

// findFirstNonTerminal returns the first slice ID in the track whose committed
// state (read via the oracle) is non-terminal per router.IsTerminal. Returns ""
// if all slices are terminal — the track is fully done and should merge.
//
// When oracle is nil, falls back to returning slices[0] (legacy behaviour).
//
// The authoritative state machine lives in the router; this function determines
// the initial frontier slice for the first Route() call.
func findFirstNonTerminal(ctx context.Context, oracle router.OracleReader, release, trackID string, slices []string) string {
	if len(slices) == 0 {
		return ""
	}

	// Legacy fallback: no oracle wired → return first slice.
	if oracle == nil {
		return slices[0]
	}

	for _, sid := range slices {
		ss, err := oracle.ReadSliceStatus(ctx, release, sid)
		if err != nil {
			// AC3: on read error (e.g. track ref doesn't exist yet), seed AT
			// this slice rather than skipping past it. Skipping re-introduces
			// the forward-only abandonment that DD-1 prevents. The oracle's
			// track→release-wt fallback handles nonexistent refs internally;
			// a hard error (malformed content) is rare and seeding at the
			// unreadable slice is the safest default.
			return sid
		}
		if !router.IsTerminal(string(ss.State)) {
			return sid
		}
	}
	return ""
}

// stripCaptainProceed removes captain-proceed.md for the given slice so the
// Design TL;DR gate fires again on the next implement dispatch.
func stripCaptainProceed(w io.Writer, workRoot, specBase, sliceID string) {
	ackPath := filepath.Join(workRoot, specBase, sliceID, "captain-proceed.md")
	if err := os.Remove(ackPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(w, "stripCaptainProceed: remove %s: %v\n", ackPath, err)
	}
}

// dirExists checks if a path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// defaultRunSliceFn is the production RunSlice wrapper.
func defaultRunSliceFn(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
	return fmt.Errorf("defaultRunSliceFn: not wired — use WorkerOptions.RunSliceFn")
}

// DefaultRunSliceFn returns the default RunSlice function (stub).
func DefaultRunSliceFn() func(context.Context, string, string, string) error {
	return defaultRunSliceFn
}
