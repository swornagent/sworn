package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/supervisor")

// TrackResult is the outcome of a single worker goroutine.
type TrackResult string

const (
	TrackPass    TrackResult = "pass"
	TrackFail    TrackResult = "fail"
	TrackSkipped TrackResult = "skipped"
	TrackPaused  TrackResult = "paused"
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
	"coach_decision":  true,
	"replan-release":  true,
	"merge-track":     true,
	"merge-release":   true,
}

// ── WorkerOptions ───────────────────────────────────────────────────────

// WorkerOptions configures a single-track worker goroutine.
type WorkerOptions struct {
	// ReleaseName is the release name (e.g. "2026-06-19-safe-parallelism").
	ReleaseName string

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

	// RunSliceFn is the function that runs a single slice's implement→verify
	// loop. Tests inject a fake; production uses run.RunSlice.
	RunSliceFn func(ctx context.Context, worktreeRoot, specPath, statusPath string) error

	// Router is the SliceRouter the worker polls for route decisions.
	// When nil, the worker falls back to static iteration for backward
	// compatibility (RunTrackLegacy behaviour).
	Router SliceRouter

	// ProjectDir is the project directory name used for worktree naming.
	ProjectDir string

	// Notifier is the notification dispatcher for track-level failures.
	// When nil, notifications are skipped.
	Notifier *account.Notifier
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

	// ── Check if context is already cancelled (dependency failed) ───────
	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "[%s] skipped: depends_on failed\n", trackID)
		return TrackSkipped
	}

	fmt.Fprintf(os.Stderr, "[%s] starting\n", trackID)

	// ── Supervisor acquire ──────────────────────────────────────────────
	sup := supervisor.New(opts.DB, opts.ReleaseName)
	if err := sup.Acquire(trackID); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] supervisor acquire error: %v\n", trackID, err)
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
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] cannot determine home dir: %v\n", trackID, err)
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}
		trackWorktreePath = filepath.Join(homeDir, "projects", opts.ProjectDir+"-worktrees",
			"release-"+opts.ReleaseName+"-"+trackID)
	}

	if !dirExists(trackWorktreePath) {
		fmt.Fprintf(os.Stderr, "[%s] materialising worktree at %s\n", trackID, trackWorktreePath)

		releaseBranch := "release-wt/" + opts.ReleaseName
		cmd := exec.CommandContext(ctx, "git", "worktree", "add",
			trackWorktreePath, "-b", trackBranch,
			releaseBranch,
		)
		cmd.Dir = opts.PrimaryWorktreeRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] worktree materialisation failed: %v\n  %s\n",
				trackID, err, string(output))
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}
		fmt.Fprintf(os.Stderr, "[%s] worktree materialised at %s\n", trackID, trackWorktreePath)

		mergeCmd := exec.Command("git", "merge", releaseBranch, "--no-edit")
		mergeCmd.Dir = trackWorktreePath
		if mergeOut, mergeErr := mergeCmd.CombinedOutput(); mergeErr != nil {
			fmt.Fprintf(os.Stderr, "[%s] forward-merge note: %s\n", trackID, string(mergeOut))
		}
	}

	// ── Fallback: no router → static iteration ─────────────────────────
	if opts.Router == nil {
		return runTrackLegacy(ctx, opts, trackWorktreePath, trackID, trackBranch, releaseTrack)
	}

	// ── Router-driven poll loop ─────────────────────────────────────────
	return runTrackRouter(ctx, opts, trackWorktreePath, trackID, trackBranch, releaseTrack)
}

// runTrackRouter is the router-driven execution loop (S59 core).
// It polls the router for the current frontier slice, dispatches the
// returned action, and loops until the router returns a terminal or
// paused decision.
func runTrackRouter(
	ctx context.Context,
	opts WorkerOptions,
	workRoot, trackID, trackBranch string,
	releaseTrack func(string),
) TrackResult {
	specBase := filepath.Join("docs", "release", opts.ReleaseName)

	// Determine the first non-terminal slice in the track.
	currentSlice := findFirstNonTerminal(opts.TrackInfo.Slices)
	if currentSlice == "" {
		// All slices already terminal (verified/shipped/deferred).
		return finishTrack(ctx, opts, workRoot, trackID, trackBranch, releaseTrack)
	}

	for {
		// Check context before every iteration.
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "[%s] cancelled at slice %s\n", trackID, currentSlice)
			releaseTrack(supervisor.StateFailed)
			return TrackSkipped
		}

		// Poll the router for the current frontier slice.
		decision, err := opts.Router.Route(ctx, opts.ReleaseName, currentSlice, trackID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] router error for %s: %v\n", trackID, currentSlice, err)
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}

		fmt.Fprintf(os.Stderr, "[%s] router: %s → %s (%s)\n",
			trackID, currentSlice, decision.Type, decision.Reason)

		// Advance to the target slice BEFORE dispatching — the router's
		// Target field tells us which slice the decision applies to.
		if decision.Target != "" && decision.Target != currentSlice {
			fmt.Fprintf(os.Stderr, "[%s] advanced to next slice: %s\n", trackID, decision.Target)
			currentSlice = decision.Target
		}

		switch decision.Type {		case "implement", "verify":
			// Both implement and verify dispatch to RunSliceFn, which handles
			// the full implement→verify loop in production (run.RunSlice).
			// A separate verify-only step would be needed for a genuine
			// "implemented but not verified" resume, but RunSlice already
			// handles both phases atomically.
			specPath := filepath.Join(workRoot, specBase, currentSlice, "spec.md")
			statusPath := filepath.Join(workRoot, specBase, currentSlice, "status.json")

			fmt.Fprintf(os.Stderr, "[%s] running slice %s\n", trackID, currentSlice)

			if err := opts.RunSliceFn(ctx, workRoot, specPath, statusPath); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] slice %s failed: %v\n", trackID, currentSlice, err)

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
			// Strip approved-ack.md so the Design TL;DR gate fires again on
			// the next implement attempt. Then dispatch implement.
			stripApprovedAck(workRoot, specBase, currentSlice)

			specPath := filepath.Join(workRoot, specBase, currentSlice, "spec.md")
			statusPath := filepath.Join(workRoot, specBase, currentSlice, "status.json")

			fmt.Fprintf(os.Stderr, "[%s] redesign: stripped approved-ack.md for %s, re-running\n",
				trackID, currentSlice)

			if err := opts.RunSliceFn(ctx, workRoot, specPath, statusPath); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] slice %s failed after redesign: %v\n", trackID, currentSlice, err)
				releaseTrack(supervisor.StateFailed)
				return TrackFail
			}

		case "coach_decision", "replan-release", "merge-track", "merge-release":
			// Human-gated pause states — surface and pause this track.
			fmt.Fprintf(os.Stderr, "[%s] paused: %s — %s\n", trackID, decision.Type, decision.Reason)
			releaseTrack("paused")
			return TrackPaused

		case "none":
			// Terminal — no more slices.
			return finishTrack(ctx, opts, workRoot, trackID, trackBranch, releaseTrack)

		default:
			fmt.Fprintf(os.Stderr, "[%s] unrecognised router decision %q for %s: %s\n",
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
	workRoot, trackID, trackBranch string,
	releaseTrack func(string),
) TrackResult {
	specBase := filepath.Join("docs", "release", opts.ReleaseName)

	for _, sliceID := range opts.TrackInfo.Slices {
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "[%s] cancelled at slice %s\n", trackID, sliceID)
			releaseTrack(supervisor.StateFailed)
			return TrackSkipped
		}

		fmt.Fprintf(os.Stderr, "[%s] running slice %s (legacy)\n", trackID, sliceID)

		specPath := filepath.Join(workRoot, specBase, sliceID, "spec.md")
		statusPath := filepath.Join(workRoot, specBase, sliceID, "status.json")

		if err := opts.RunSliceFn(ctx, workRoot, specPath, statusPath); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] slice %s failed: %v\n", trackID, sliceID, err)

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

	return finishTrack(ctx, opts, workRoot, trackID, trackBranch, releaseTrack)
}

// finishTrack pushes the track branch and releases the supervisor.
func finishTrack(
	_ context.Context,
	opts WorkerOptions,
	workRoot, trackID, trackBranch string,
	releaseTrack func(string),
) TrackResult {
	releaseTrack(supervisor.StateDone)

	pushCmd := exec.Command("git", "push", "origin", "HEAD:"+trackBranch)
	pushCmd.Dir = workRoot
	_ = pushCmd.Run()

	fmt.Fprintf(os.Stderr, "[%s] done\n", trackID)
	return TrackPass
}

// findFirstNonTerminal returns the first slice ID in the track that is not
// in a terminal state (verified, shipped, deferred). Returns "" if all
// slices are terminal — the track is fully done and resumability skips it.
//
// This is a best-effort helper for the worker at startup. The authoritative
// state machine lives in the router (S58); this function only determines the
// initial frontier slice for the first Route() call.
func findFirstNonTerminal(slices []string) string {
	for _, sid := range slices {
		// We can't read committed state here (no router wired yet for the
		// first call). The router will handle this on the first Route() call.
		// We just return the first slice; the router will skip terminal ones
		// and return a Target for the actual frontier.
		return sid
	}
	return ""
}

// stripApprovedAck removes approved-ack.md for the given slice so the
// Design TL;DR gate fires again on the next implement dispatch.
func stripApprovedAck(workRoot, specBase, sliceID string) {
	ackPath := filepath.Join(workRoot, specBase, sliceID, "approved-ack.md")
	if err := os.Remove(ackPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "stripApprovedAck: remove %s: %v\n", ackPath, err)
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

