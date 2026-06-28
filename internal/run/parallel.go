package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/router"
	"github.com/swornagent/sworn/internal/scheduler"
)

// ParallelOptions configures the RunParallel concurrent execution.
type ParallelOptions struct {
	// ReleaseName is the release name (e.g. "2026-06-19-safe-parallelism").
	ReleaseName string

	// WorkspaceRoot is the primary repo root.
	WorkspaceRoot string

	// DB is the SQLite database handle (for supervisor process ownership).
	DB *sql.DB

	// EventDB is the release-specific event store database handle. When set,
	// events written by the supervisor are routed to this DB instead of DB.
	// This separates process-ownership state (sworn.db) from durable event
	// storage (supervisor-<release>.db). When nil, events are written to DB.
	EventDB *sql.DB
	// RunSliceFn is the per-slice implementation+verification function.
	// Production: run.RunSlice. Tests inject fakes.
	RunSliceFn func(ctx context.Context, worktreeRoot, specPath, statusPath string) error

	// ProjectDir is the base name of the project directory, used for
	// worktree naming conventions.
	ProjectDir string

	// Notifier is the notification dispatcher for track-level failures.
	// When nil, notifications are skipped.
	Notifier *account.Notifier

	// Router is the SliceRouter workers poll for route decisions. When nil,
	// RunParallel auto-constructs a production router backed by internal/router
	// and internal/board. Tests inject fakes to exercise the router-driven path
	// without real git state.
	Router scheduler.SliceRouter

	// PauseEngine manages cooperative pause signals. When nil, defaults to
	// scheduler.DefaultPauseEngine (the process-global engine shared by CLI,
	// TUI, and MCP). Tests may supply their own to avoid global state.
	PauseEngine *scheduler.PauseEngine

	// MergeTrackFn is invoked when a track finishes to auto-merge the track
	// branch into the release worktree. When nil, auto-merge is skipped
	// (tests and legacy paths). The production CLI sets this to
	// ProductionMergeTrack.
	MergeTrackFn func(releasePath, trackID, branch string) error

	// PlannedFilesFn returns the union of planned_files across all slices
	// in a track, read from committed status.json on the release-wt ref.
	// When nil, RunParallel constructs a default reader that uses git show
	// against the release-wt/<release> ref. Tests inject a fake to exercise
	// invariant-2 enforcement (S06) without real git state.
	PlannedFilesFn func(ctx context.Context, trackID string) ([]string, error)
} // productionSliceRouter wraps internal/router.Route to satisfy scheduler.SliceRouter.
// Constructed by RunParallel when no Router is injected via ParallelOptions.
type productionSliceRouter struct {
	oracle     router.OracleReader
	content    router.ContentReader
	trackInfos []board.TrackInfo
}

func (p *productionSliceRouter) Route(ctx context.Context, release, sliceID, trackID string) (scheduler.SliceDecision, error) {
	var trackBranch string
	for _, ti := range p.trackInfos {
		if ti.ID == trackID {
			trackBranch = ti.WorktreeBranch
			break
		}
	}

	dec, err := router.Route(ctx, p.oracle, p.content, router.RouteInput{
		Release:     release,
		SliceID:     sliceID,
		TrackID:     trackID,
		TrackBranch: trackBranch,
		ReleaseRef:  "release-wt/" + release,
		DocsPrefix:  "docs",
	})
	if err != nil {
		return scheduler.SliceDecision{}, err
	}
	return scheduler.SliceDecision{
		Type:   string(dec.NextType),
		Reason: dec.NextReason,
		Target: dec.TargetSlice,
	}, nil
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

	// ── Auto-construct production router when none injected ─────────────
	// Tests inject fakes via opts.Router; production gets a router backed by
	// the live oracle (board.OracleReaderAdapter) and git content reader.
	// Soft-fail: if the git repo or release ref is unavailable (e.g. in
	// unit tests that operate outside a real repo), opts.Router stays nil
	// and workers fall back to the legacy static-iteration path via RunTrack.
	// In production (real git repo + release branch) the construction always
	// succeeds and the router-driven loop is the live path.
	var ora router.OracleReader
	if opts.Router == nil {
		repo := git.New(absRoot)
		releaseRef := "release-wt/" + releaseName
		if o, oraErr := board.NewOracleReaderAdapterFromRepo(repo, releaseName, releaseRef); oraErr == nil {
			ora = o
			opts.Router = &productionSliceRouter{
				oracle:     ora,
				content:    repo,
				trackInfos: tracks,
			}
		}
	}

	// ── Resolve pause engine ────────────────────────────────────────────
	pauseEngine := opts.PauseEngine
	if pauseEngine == nil {
		pauseEngine = scheduler.DefaultPauseEngine
	}

	// ── Resolve planned-files reader ────────────────────────────────────
	plannedFilesFn := opts.PlannedFilesFn
	if plannedFilesFn == nil {
		plannedFilesFn = makePlannedFilesReader(absRoot, releaseName, tracks)
	}

	// ── Parse documented shared files ────────────────────────────────────
	// Extract from the markdown body (after frontmatter close), NOT the
	// frontmatter — the DOCUMENTED SHARED touchpoint matrix is a table in
	// the body. (Captain pin 2)
	docShared := parseDocumentedSharedFiles(string(indexData))

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

		// runningFiles accumulates the planned_files of all tracks launched
		// in this phase, for invariant-2 enforcement. fileOwner maps each
		// file to the track ID that launched it — used to produce the
		// T_a identifier in the INVARIANT-2 message.
		// Both reset per-phase so blocked tracks re-check cleanly in the
		// follow-up phase.
		runningFiles := make(map[string]bool)
		fileOwner := make(map[string]string)
		var blockedTracks []board.TrackInfo

		for _, trackInfo := range phase.Tracks {
			if phaseCtx.Err() != nil {
				outcomeMap.Store(trackInfo.ID, scheduler.TrackSkipped)
				fmt.Fprintf(os.Stderr, "[%s] skipped: depends_on failed (phase barrier)\n", trackInfo.ID)
				continue
			}

			// ── Invariant-2: disjointness check (S06) ─────────────────
			planned, err := plannedFilesFn(phaseCtx, trackInfo.ID)
			if err != nil {
				planned = nil // fail open (AC-4)
			}

			// Check against already-running tracks in this phase.
			running := make([]string, 0, len(runningFiles))
			for f := range runningFiles {
				running = append(running, f)
			}
			overlaps := checkDisjointness(planned, running, docShared)
			if len(overlaps) > 0 {
				// Find the already-launched track that owns the first
				// overlapping file for the INVARIANT-2 message.
				tA := fileOwner[overlaps[0]]
				fmt.Fprintf(os.Stderr, "INVARIANT-2: tracks %s and %s both write %s — blocked %s until %s merges\n",
					tA, trackInfo.ID, overlaps[0], trackInfo.ID, tA)
				blockedTracks = append(blockedTracks, trackInfo)
				outcomeMap.Store(trackInfo.ID, scheduler.TrackBlocked)
				continue
			}

			// Track passes invariant-2 — add its files to running set.
			for _, f := range planned {
				runningFiles[f] = true
				fileOwner[f] = trackInfo.ID
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
					EventDB:             opts.EventDB,
					RunSliceFn:          opts.RunSliceFn,
					Notifier:            opts.Notifier,
					Router:              opts.Router,
					Oracle:              ora,
					PauseCh:             pauseEngine.PauseCh(releaseName),
					MergeTrackFn:        opts.MergeTrackFn,
				}
				result := scheduler.RunTrack(phaseCtx, workerOpts)
				outcomeMap.Store(t.ID, result)
				if result == scheduler.TrackFail {
					failCancel()
				}
			}()
		}

		wg.Wait()

		// ── Follow-up phase: retry blocked tracks (S06 AC-2) ──────────
		// After all launched tracks finish (and auto-merge via finishTrack),
		// re-check blocked tracks. The conflicting track has merged, so the
		// disjointness re-check passes — same retry mechanic as S04's
		// phase barrier (depends_on wait).
		if len(blockedTracks) > 0 {
			retryRunningFiles := make(map[string]bool)
			retryFileOwner := make(map[string]string)
			var retryWg sync.WaitGroup

			for _, t := range blockedTracks {
				if phaseCtx.Err() != nil {
					break
				}

				planned, _ := plannedFilesFn(phaseCtx, t.ID)
				running := make([]string, 0, len(retryRunningFiles))
				for f := range retryRunningFiles {
					running = append(running, f)
				}
				overlaps := checkDisjointness(planned, running, docShared)
				if len(overlaps) > 0 {
					tA := retryFileOwner[overlaps[0]]
					fmt.Fprintf(os.Stderr, "INVARIANT-2: tracks %s and %s both write %s — blocked %s after retry (merge did not resolve)\n",
						tA, t.ID, overlaps[0], t.ID)
					outcomeMap.Store(t.ID, scheduler.TrackBlocked)
					continue
				}

				for _, f := range planned {
					retryRunningFiles[f] = true
					retryFileOwner[f] = t.ID
				}

				retryWg.Add(1)
				tt := t
				go func() {
					defer retryWg.Done()
					workerOpts := scheduler.WorkerOptions{
						ReleaseName:         releaseName,
						TrackInfo:           tt,
						ReleaseWorktreePath: releaseWorktreePath,
						PrimaryWorktreeRoot: absRoot,
						ProjectDir:          opts.ProjectDir,
						DB:                  opts.DB,
						EventDB:             opts.EventDB,
						RunSliceFn:          opts.RunSliceFn,
						Notifier:            opts.Notifier,
						Router:              opts.Router,
						Oracle:              ora,
						PauseCh:             pauseEngine.PauseCh(releaseName),
						MergeTrackFn:        opts.MergeTrackFn,
					}
					result := scheduler.RunTrack(phaseCtx, workerOpts)
					outcomeMap.Store(tt.ID, result)
					if result == scheduler.TrackFail {
						failCancel()
					}
				}()
			}
			retryWg.Wait()
		}
		phaseCancel()
	}
	// ── Collect and report outcomes ─────────────────────────────────────
	var failedTracks []string
	var skippedTracks []string
	var pausedTracks []string
	var blockedTracksList []string

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
		case scheduler.TrackBlocked:
			blockedTracksList = append(blockedTracksList, trackInfo.ID)
			fmt.Fprintf(os.Stderr, "[%s] result: BLOCKED (invariant-2)\n", trackInfo.ID)
		}
	}

	if len(failedTracks) > 0 {
		return fmt.Errorf("RunParallel: %d track(s) failed: %s",
			len(failedTracks), strings.Join(failedTracks, ", "))
	}

	// AC-6: a paused track is a non-zero outcome — a human decision is
	// required before the release can proceed. Surface which tracks are paused
	// so the caller (CLI / TUI) can route to the appropriate command.
	if len(pausedTracks) > 0 {
		return fmt.Errorf("RunParallel: %d track(s) paused (human decision required): %s",
			len(pausedTracks), strings.Join(pausedTracks, ", "))
	}

	// TrackBlocked: invariant-2 blocked tracks that could not be resolved
	// after retry. The release cannot proceed.
	if len(blockedTracksList) > 0 {
		return fmt.Errorf("RunParallel: %d track(s) blocked (invariant-2): %s",
			len(blockedTracksList), strings.Join(blockedTracksList, ", "))
	}

	fmt.Fprintf(os.Stderr, "RunParallel: all %d tracks PASS (skipped: %d, blocked: %d)\n",
		len(tracks), len(skippedTracks), len(blockedTracksList))
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

// ProductionMergeTrack merges a track branch into the release worktree.
// Called from finishTrack when MergeTrackFn is wired in WorkerOptions.
//
// Strategy: try a local merge first (the branch may already be reachable from
// the release worktree without a fetch — common in test scenarios and when
// the release worktree shares object storage). If that fails, fetch the branch
// from origin (just pushed by finishTrack) and retry with origin/<branch>.
func ProductionMergeTrack(releasePath, trackID, branch string) error {
	// Guard: if the release path is not a git worktree, skip the merge.
	// This happens in tests (temp dirs) and is harmless — the merge is a
	// production-only operation.
	if !dirExists(filepath.Join(releasePath, ".git")) {
		return nil
	}

	// Attempt 1: merge the local branch name directly.
	mergeCmd := exec.Command("git", "merge", "--no-ff", branch, "--no-edit")
	mergeCmd.Dir = releasePath
	_, err := mergeCmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// Attempt 2: fetch from origin, then merge origin/<branch>.
	fetchCmd := exec.Command("git", "fetch", "origin", branch)
	fetchCmd.Dir = releasePath
	if out, fetchErr := fetchCmd.CombinedOutput(); fetchErr != nil {
		return fmt.Errorf("ProductionMergeTrack: merge %s: %v (local)\n  fetch %s: %v\n  %s",
			branch, err, branch, fetchErr, string(out))
	}

	mergeCmd2 := exec.Command("git", "merge", "--no-ff", "origin/"+branch, "--no-edit")
	mergeCmd2.Dir = releasePath
	output2, err2 := mergeCmd2.CombinedOutput()
	if err2 != nil {
		return fmt.Errorf("ProductionMergeTrack: merge %s: %v (local)\n  merge origin/%s: %v\n  %s",
			branch, err, branch, err2, string(output2))
	}
	return nil
}

// dirExists checks if a path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ── Invariant-2 enforcement (S06) ─────────────────────────────────────────

// plannedFilesKey is a track-id → planned_files map key used in the default
// planned-files reader closure.
type plannedFilesKey struct {
	absRoot       string
	releaseName   string
	slicesByTrack map[string][]string
}

// parseDocumentedSharedFiles extracts file paths from the DOCUMENTED SHARED
// rows in the index.md markdown body (after the closing --- delimiter).
// The touchpoint matrix is a markdown table in the body, NOT the frontmatter.
//
// Format: | `path/to/file.go` (DOCUMENTED SHARED) | ...
// The function extracts the first backtick-quoted path from any row containing
// "(DOCUMENTED SHARED)".
func parseDocumentedSharedFiles(indexData string) map[string]bool {
	// Find the closing frontmatter delimiter — the body starts after the second ---.
	// The first --- is at position 0; find the second --- on its own line.
	const delim = "\n---"
	bodyStart := strings.Index(indexData, delim)
	if bodyStart < 0 {
		return nil
	}
	// Skip past the closing --- (len(delim) bytes) plus the newline.
	body := indexData[bodyStart+len(delim):]
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	} else if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	}

	result := make(map[string]bool)
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "(DOCUMENTED SHARED)") {
			continue
		}
		// Extract the first backtick-quoted path.
		start := strings.Index(line, "`")
		if start < 0 {
			continue
		}
		end := strings.Index(line[start+1:], "`")
		if end < 0 {
			continue
		}
		path := line[start+1 : start+1+end]
		if path != "" {
			result[path] = true
		}
	}
	return result
}

// checkDisjointness returns the set of files that appear in both a and b,
// excluding any files in docShared. An empty result means the two sets are
// disjoint (no invariant-2 violation).
func checkDisjointness(a, b []string, docShared map[string]bool) []string {
	bSet := make(map[string]bool, len(b))
	for _, f := range b {
		bSet[f] = true
	}
	var overlaps []string
	for _, f := range a {
		if bSet[f] && !docShared[f] {
			overlaps = append(overlaps, f)
		}
	}
	return overlaps
}

// makePlannedFilesReader builds the default PlannedFilesFn that reads
// each slice's status.json from the release-wt ref via git show, extracts
// planned_files, and returns the union across all slices in the track.
// The closure captures absRoot, releaseName, and the track→slices map.
func makePlannedFilesReader(absRoot, releaseName string, tracks []board.TrackInfo) func(ctx context.Context, trackID string) ([]string, error) {
	// Build track→slices lookup.
	slicesByTrack := make(map[string][]string, len(tracks))
	for _, ti := range tracks {
		slicesByTrack[ti.ID] = ti.Slices
	}
	repo := git.New(absRoot)
	ref := "release-wt/" + releaseName

	return func(ctx context.Context, trackID string) ([]string, error) {
		slices, ok := slicesByTrack[trackID]
		if !ok {
			return nil, nil // track not found → empty (fail open)
		}

		var allFiles []string
		for _, sliceID := range slices {
			// Read status.json from release-wt ref.
			// Path: docs/release/<release>/<slice>/status.json
			path := fmt.Sprintf("docs/release/%s/%s/status.json", releaseName, sliceID)
			raw, err := repo.Show(ref, path)
			if err != nil {
				continue // fail open (AC-4)
			}

			var st struct {
				PlannedFiles []string `json:"planned_files"`
			}
			if err := json.Unmarshal([]byte(raw), &st); err != nil {
				continue // fail open (AC-4)
			}
			allFiles = append(allFiles, st.PlannedFiles...)
		}
		return allFiles, nil
	}
}
