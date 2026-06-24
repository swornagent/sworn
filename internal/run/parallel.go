package run

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/scheduler"
)

// ParallelOptions configures the RunParallel concurrent execution.
type ParallelOptions struct {
	// ReleaseName is the release name (e.g. "2026-06-19-safe-parallelism").
	ReleaseName string

	// WorkspaceRoot is the primary repo root.
	WorkspaceRoot string

	// DB is the SQLite database handle (for supervisor).
	DB *sql.DB

	// RunSliceFn is the per-slice implementation+verification function.
	// Production: run.RunSlice. Tests inject fakes.
	RunSliceFn func(ctx context.Context, worktreeRoot, specPath, statusPath string) error

	// ProjectDir is the base name of the project directory, used for
	// worktree naming conventions.
	ProjectDir string

	// Notifier is the notification dispatcher for track-level failures.
	// When nil, notifications are skipped.
	Notifier *account.Notifier
}

// RunParallel reads the release board, builds an execution plan, and runs
// all tracks concurrently according to their depends_on edges.
//
// Pre-flight: ensures release worktree exists (sequential).
// Per-phase: fans out independent tracks as goroutines.
// Phase barrier: all goroutines in a phase must finish before the next phase.
//
// Returns nil if all tracks PASS, or an error if any track FAILs.
func RunParallel(ctx context.Context, opts ParallelOptions) error {
	releaseName := opts.ReleaseName
	workspaceRoot := opts.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = "."
	}

	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("RunParallel: resolve workspace root: %w", err)
	}

	// ── Read release board ──────────────────────────────────────────────
	indexPath := filepath.Join(absRoot, "docs", "release", releaseName, "index.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("RunParallel: read index.md: %w", err)
	}

	// Parse frontmatter to extract release_worktree_path.
	fm := extractFrontmatter(string(indexData))
	if fm == "" {
		return fmt.Errorf("RunParallel: no frontmatter found in %s", indexPath)
	}

	releaseWorktreePath := extractReleaseWorktreePath(fm)
	if releaseWorktreePath == "" {
		return fmt.Errorf("RunParallel: release_worktree_path not set in frontmatter of %s", indexPath)
	}

	// Parse tracks from frontmatter.
	tracks := board.ParseTracks(fm)
	if len(tracks) == 0 {
		return fmt.Errorf("RunParallel: no tracks found in release board")
	}

	// ── Pre-flight: ensure release worktree exists ──────────────────────
	if !dirExists(releaseWorktreePath) {
		fmt.Fprintf(os.Stderr, "RunParallel: materialising release worktree at %s\n", releaseWorktreePath)
		releaseBranch := "release-wt/" + releaseName
		cmd := exec.CommandContext(ctx, "git", "worktree", "add",
			releaseWorktreePath, releaseBranch)
		cmd.Dir = absRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("RunParallel: materialise release worktree: %v\n  %s", err, string(output))
		}
	}

	// ── Build execution plan ────────────────────────────────────────────
	plan, err := scheduler.BuildPlan(tracks)
	if err != nil {
		return fmt.Errorf("RunParallel: build plan: %w", err)
	}

	fmt.Fprintf(os.Stderr, "sworn run --parallel: loaded %d tracks in %d phases\n",
		len(tracks), len(plan.Phases))

	// ── Fan out per phase ───────────────────────────────────────────────
	var outcomeMap sync.Map
	// failCtx propagates cancellation to subsequent phases when any track fails.
	// Each phase derives its own phaseCtx from failCtx so dependent tracks only
	// skip when a dependency actually failed — not when the previous phase's
	// goroutines were cleaned up.
	failCtx, failCancel := context.WithCancel(ctx)
	defer failCancel()

	for _, phase := range plan.Phases {
		phaseCtx, phaseCancel := context.WithCancel(failCtx)
		var wg sync.WaitGroup

		for _, trackInfo := range phase.Tracks {
			if phaseCtx.Err() != nil {
				outcomeMap.Store(trackInfo.ID, scheduler.TrackSkipped)
				fmt.Fprintf(os.Stderr, "[%s] skipped: depends_on failed (phase barrier)\n", trackInfo.ID)
				continue
			}

			wg.Add(1)
			t := trackInfo

			go func() {
				defer wg.Done()

				workerOpts := scheduler.WorkerOptions{
					ReleaseName:         releaseName,
					TrackInfo:           t,
					ReleaseWorktreePath: releaseWorktreePath,
					PrimaryWorktreeRoot: absRoot,
					ProjectDir:          opts.ProjectDir,
					DB:                  opts.DB,
					RunSliceFn:          opts.RunSliceFn,
					Notifier:            opts.Notifier,
				}
				result := scheduler.RunTrack(phaseCtx, workerOpts)
				outcomeMap.Store(t.ID, result)

				if result == scheduler.TrackFail {
					failCancel()
				}
			}()
		}

		wg.Wait()
		phaseCancel()
	}

	// ── Collect and report outcomes ─────────────────────────────────────
	var failedTracks []string
	var skippedTracks []string
	var pausedTracks []string

	for _, trackInfo := range tracks {
		val, ok := outcomeMap.Load(trackInfo.ID)
		if !ok {
			failedTracks = append(failedTracks, trackInfo.ID+" (no outcome)")
			continue
		}
		result := val.(scheduler.TrackResult)
		switch result {
		case scheduler.TrackPass:
			fmt.Fprintf(os.Stderr, "[%s] result: PASS\n", trackInfo.ID)
		case scheduler.TrackFail:
			failedTracks = append(failedTracks, trackInfo.ID)
			fmt.Fprintf(os.Stderr, "[%s] result: FAIL\n", trackInfo.ID)
		case scheduler.TrackSkipped:
			skippedTracks = append(skippedTracks, trackInfo.ID)
			fmt.Fprintf(os.Stderr, "[%s] result: SKIPPED\n", trackInfo.ID)
		case scheduler.TrackPaused:
			pausedTracks = append(pausedTracks, trackInfo.ID)
			fmt.Fprintf(os.Stderr, "[%s] result: PAUSED\n", trackInfo.ID)
		}
	}

	if len(failedTracks) > 0 {
		return fmt.Errorf("RunParallel: %d track(s) failed: %s",
			len(failedTracks), strings.Join(failedTracks, ", "))
	}

	fmt.Fprintf(os.Stderr, "RunParallel: all %d tracks PASS (skipped: %d, paused: %d)\n",
		len(tracks), len(skippedTracks), len(pausedTracks))
	return nil
}

// extractFrontmatter returns the content between the first --- and second ---
// in a markdown file with YAML frontmatter.
func extractFrontmatter(text string) string {
	const delim = "---"
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != delim {
		return ""
	}

	var body []string
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == delim {
			return strings.Join(body, "\n")
		}
		body = append(body, lines[i])
	}
	return ""
}

// extractReleaseWorktreePath extracts the release_worktree_path from
// frontmatter body.
func extractReleaseWorktreePath(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "release_worktree_path:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "release_worktree_path:"))
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

// dirExists checks if a path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}