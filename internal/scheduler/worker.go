package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/supervisor")

// TrackResult is the outcome of a single worker goroutine.
type TrackResult string

const (
	TrackPass   TrackResult = "pass"
	TrackFail   TrackResult = "fail"
	TrackSkipped TrackResult = "skipped"
)

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

	// ProjectDir is the project directory name used for worktree naming.
	ProjectDir string
}

// RunTrack executes one track's slices sequentially in its own worktree.
// It is designed to be called as a goroutine from RunParallel.
//
// It returns TrackPass if all slices succeed, TrackFail if any slice fails,
// or TrackSkipped if dependencies indicate the track should be skipped.
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
		// Generate a worktree path from the project dir + release + track.
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

		// Create the worktree from the release worktree branch.
		// git worktree add <path> -b <branch> <release-branch>
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

		// Forward-merge release-wt into the track so it's current.
		mergeCmd := exec.Command("git", "merge", releaseBranch, "--no-edit")
		mergeCmd.Dir = trackWorktreePath
		if mergeOut, mergeErr := mergeCmd.CombinedOutput(); mergeErr != nil {
			// Non-fatal: log but continue (merge may already be up-to-date).
			fmt.Fprintf(os.Stderr, "[%s] forward-merge note: %s\n", trackID, string(mergeOut))
		}
	}

	// ── Run each slice in the worktree ──────────────────────────────────
	// The track's slices are the slice IDs from TrackInfo.Slices. We need
	// to construct the specPath and statusPath for each.
	specBase := filepath.Join("docs", "release", opts.ReleaseName)
	workRoot := trackWorktreePath

	for _, sliceID := range opts.TrackInfo.Slices {
		// Check context before every slice.
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "[%s] cancelled at slice %s\n", trackID, sliceID)
			releaseTrack(supervisor.StateFailed)
			return TrackSkipped
		}

		fmt.Fprintf(os.Stderr, "[%s] running slice %s\n", trackID, sliceID)

		specPath := filepath.Join(workRoot, specBase, sliceID, "spec.md")
		statusPath := filepath.Join(workRoot, specBase, sliceID, "status.json")

		if err := opts.RunSliceFn(ctx, workRoot, specPath, statusPath); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] slice %s failed: %v\n", trackID, sliceID, err)
			releaseTrack(supervisor.StateFailed)
			return TrackFail
		}
	}

	// ── All slices passed ───────────────────────────────────────────────
	releaseTrack(supervisor.StateDone)

	// Push the track branch so results are durable.
	pushCmd := exec.Command("git", "push", "origin", "HEAD:"+trackBranch)
	pushCmd.Dir = trackWorktreePath
	_ = pushCmd.Run() // Best-effort; log but don't fail on push issues.

	fmt.Fprintf(os.Stderr, "[%s] done\n", trackID)
	return TrackPass
}

// dirExists checks if a path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// defaultRunSliceFn is the production RunSlice wrapper.
// In production, this calls run.RunSlice; here we provide the
// signature-compatible version. The actual wiring happens in parallel.go
// which constructs WorkerOptions with the real run.RunSlice.
func defaultRunSliceFn(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
	return fmt.Errorf("defaultRunSliceFn: not wired — use WorkerOptions.RunSliceFn")
}

// DefaultRunSliceFn returns the default RunSlice function (stub).
func DefaultRunSliceFn() func(context.Context, string, string, string) error {
	return defaultRunSliceFn
}