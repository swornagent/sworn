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
	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/router"
	"github.com/swornagent/sworn/internal/scheduler"
	"github.com/swornagent/sworn/internal/tracklog"
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

	// DocsPrefix is the git-ref path prefix for release boards (default "docs").
	// On monorepos where docs/ is a symlink (e.g. fired: docs → apps/docs/content/docs),
	// the on-disk readers resolve through the symlink but git-ref readers (the
	// router and the planned-files reader) cannot, so they need the real prefix
	// (e.g. "apps/docs/content/docs"). The oracle auto-detects independently.
	DocsPrefix string

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
	docsPrefix string
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
		DocsPrefix:  p.docsPrefix,
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

	// Resolve the docs prefix for git-ref readers (router, planned-files). The
	// on-disk readers resolve through a symlinked docs/ dir, but git-ref readers
	// need the real path on monorepos. Default "docs".
	docsPrefix := opts.DocsPrefix
	if docsPrefix == "" {
		docsPrefix = "docs"
	}

	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("RunParallel: resolve workspace root: %w", err)
	}

	// ── Durable log dir + coordinator tee (feat: live logs) ─────────────
	// logDir is .sworn/logs/<release>: per-track <track>.log files are written
	// by each worker (WorkerOptions.LogDir), and loop.log captures this
	// coordinator goroutine's own narration. EnsureSelfIgnore stamps
	// .sworn/.gitignore ("*") so logs are never git-tracked even on the
	// (never-in-production) path where a log write races ahead of db.Open — in
	// every real run db.Open has already stamped it. MkdirAll failure is
	// non-fatal: NewWriter fails open to stderr-only.
	logDir := filepath.Join(absRoot, db.DefaultDir, "logs", releaseName)
	_ = os.MkdirAll(logDir, 0o755)
	db.EnsureSelfIgnore(filepath.Join(absRoot, db.DefaultDir))
	lw, closeLoop := tracklog.NewWriter(logDir, "loop")
	defer closeLoop()

	// ── Read release board (board.json via the oracle) ──────────────────
	// board.ReadBoard reads docs/release/<release>/board.json, lazily migrating
	// from index.md frontmatter when board.json is absent (legacy releases). This
	// replaces the former hand-rolled index.md frontmatter parse (extractFrontmatter
	// + board.ParseTracks), which returned zero tracks for any current-format
	// release and hard-errored "no tracks found", silently disabling dispatch
	// (AC-01).
	br, err := board.ReadBoard(absRoot, releaseName)
	if err != nil {
		return fmt.Errorf("RunParallel: read release board: %w", err)
	}

	releaseWorktreePath := br.ReleaseWorktreePath
	if releaseWorktreePath == "" {
		// Cold-start (eval finding 1): this default now fires ONLY when board.json
		// genuinely records no worktree path — not unconditionally, as the old
		// index.md parser did for every current-format release (AC-02). Default to
		// a REPO-LOCAL path (a <repo>-worktrees sibling) so the engine
		// self-bootstraps the release worktree below. The track worktrees then
		// default to siblings of this path (worker.go).
		releaseWorktreePath = filepath.Join(filepath.Dir(absRoot),
			filepath.Base(absRoot)+"-worktrees", "release-"+releaseName)
		fmt.Fprintf(lw, "RunParallel: release_worktree_path unset — defaulting to %s (cold-start)\n", releaseWorktreePath)
	}

	tracks := trackInfosFromBoardTracks(br.Tracks)
	if len(tracks) == 0 {
		return fmt.Errorf("RunParallel: no tracks found in release board")
	}

	// indexPath is still needed for the documented-shared touchpoint-matrix parse
	// below (router.ParseDocumentedShared reads the rendered index.md body).
	indexPath := filepath.Join(absRoot, docsPrefix, "release", releaseName, "index.md")

	// ── Pre-flight: ensure release worktree exists ──────────────────────
	// Self-bootstrap (eval finding 1): a freshly-planned release has no
	// release-wt/<name> branch yet — Driver-1 (/implement-slice) used to create
	// it. When the branch is absent, create it with `-b` from HEAD so the engine
	// can cold-start without manual scaffolding; otherwise check out the existing
	// branch into the worktree.
	if !dirExists(releaseWorktreePath) {
		fmt.Fprintf(lw, "RunParallel: materialising release worktree at %s\n", releaseWorktreePath)
		releaseBranch := "release-wt/" + releaseName
		args := []string{"worktree", "add"}
		if branchExists(ctx, absRoot, releaseBranch) {
			args = append(args, releaseWorktreePath, releaseBranch)
		} else {
			fmt.Fprintf(lw, "RunParallel: branch %s absent — creating it from HEAD (cold-start bootstrap)\n", releaseBranch)
			args = append(args, "-b", releaseBranch, releaseWorktreePath)
		}
		cmd := exec.CommandContext(ctx, "git", args...)
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

	fmt.Fprintf(lw, "sworn run --parallel: loaded %d tracks in %d phases\n",
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
				docsPrefix: docsPrefix,
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
		plannedFilesFn = makePlannedFilesReader(absRoot, releaseName, docsPrefix, tracks)
	}

	// ── Parse documented shared files ────────────────────────────────────
	// Delegate to router.ParseDocumentedShared — the canonical touchpoint-matrix
	// parser (explicit "(DOCUMENTED SHARED)" marker AND ≥2-checkmark inference).
	// The former local parseDocumentedSharedFiles matched ONLY the explicit
	// marker, so a genuinely-shared file that a rendered index.md marks with ≥2
	// checkmarks (but no annotation) was silently dropped and then wrongly treated
	// as an invariant-2 collision (AC-03). Fail open: a release with no touchpoint
	// matrix (single-track, or an unrendered index.md) has no documented-shared
	// exemptions, which is not fatal — matching the oracle-read fail-open
	// precedent (planned-files reader) in this same function.
	docShared, err := router.ParseDocumentedShared(indexPath)
	if err != nil {
		fmt.Fprintf(lw, "RunParallel: parse documented shared files: %v (treating as no exemptions)\n", err)
		docShared = nil
	}

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
				fmt.Fprintf(lw, "[%s] skipped: depends_on failed (phase barrier)\n", trackInfo.ID)
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
				fmt.Fprintf(lw, "INVARIANT-2: tracks %s and %s both write %s — blocked %s until %s merges\n",
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
					LogDir:              logDir,
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
				// Run on the parent ctx, NOT phaseCtx (#33): a sibling track's
				// failCancel() must not cancel this track mid-run. Tracks in a
				// phase are independent — a failure is recorded in outcomeMap and
				// only gates dependent tracks in *later* phases (the phaseCtx.Err()
				// check at launch, after the wg.Wait barrier).
				result := scheduler.RunTrack(ctx, workerOpts)
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
					fmt.Fprintf(lw, "INVARIANT-2: tracks %s and %s both write %s — blocked %s after retry (merge did not resolve)\n",
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
						LogDir:              logDir,
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
					// Run on the parent ctx, NOT phaseCtx (#33): a sibling track's
					// failCancel() must not cancel this track mid-run. Tracks in a
					// phase are independent — a failure is recorded in outcomeMap and
					// only gates dependent tracks in *later* phases (the phaseCtx.Err()
					// check at launch, after the wg.Wait barrier).
					result := scheduler.RunTrack(ctx, workerOpts)
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
			fmt.Fprintf(lw, "[%s] result: PASS\n", trackInfo.ID)
		case scheduler.TrackFail:
			failedTracks = append(failedTracks, trackInfo.ID)
			fmt.Fprintf(lw, "[%s] result: FAIL\n", trackInfo.ID)
		case scheduler.TrackSkipped:
			skippedTracks = append(skippedTracks, trackInfo.ID)
			fmt.Fprintf(lw, "[%s] result: SKIPPED\n", trackInfo.ID)
		case scheduler.TrackPaused:
			pausedTracks = append(pausedTracks, trackInfo.ID)
			fmt.Fprintf(lw, "[%s] result: PAUSED\n", trackInfo.ID)
		case scheduler.TrackBlocked:
			blockedTracksList = append(blockedTracksList, trackInfo.ID)
			fmt.Fprintf(lw, "[%s] result: BLOCKED (invariant-2)\n", trackInfo.ID)
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

	fmt.Fprintf(lw, "RunParallel: all %d tracks PASS (skipped: %d, blocked: %d)\n",
		len(tracks), len(skippedTracks), len(blockedTracksList))
	return nil
}

// trackInfosFromBoardTracks converts board.json BoardTrack records into the
// board.TrackInfo shape the scheduler and router consume. Kept local to
// internal/run rather than added to internal/board because internal/board is
// T1-drift-guard's exclusive touchpoint this release — this is a field-for-field
// copy, not new shared-library surface. DependsOn narrows board.StringList to a
// plain []string.
func trackInfosFromBoardTracks(bts []board.BoardTrack) []board.TrackInfo {
	tis := make([]board.TrackInfo, len(bts))
	for i, bt := range bts {
		tis[i] = board.TrackInfo{
			ID:             bt.ID,
			Slices:         bt.Slices,
			DependsOn:      []string(bt.DependsOn),
			WorktreePath:   bt.WorktreePath,
			WorktreeBranch: bt.WorktreeBranch,
			State:          bt.State,
		}
	}
	return tis
}

// ProductionMergeTrack merges a track branch into the release worktree.
// Called from finishTrack when MergeTrackFn is wired in WorkerOptions.
//
// Strategy: try a local merge first (the branch may already be reachable from
// the release worktree without a fetch — common in test scenarios and when
// the release worktree shares object storage). If that fails, fetch the branch
// from origin (just pushed by finishTrack) and retry with origin/<branch>.
func ProductionMergeTrack(releasePath, trackID, branch string) error {
	// Rule 11 target assertion, fail closed: the release path must be a git
	// worktree. A linked worktree from `git worktree add` (how RunParallel
	// bootstraps release-wt) has a .git FILE (gitdir pointer), a primary
	// checkout has a .git directory — accept either. When neither exists the
	// target is not a worktree: return an error rather than nil, because a
	// nil return is reported upstream by finishTrack as "auto-merged".
	if _, err := os.Stat(filepath.Join(releasePath, ".git")); err != nil {
		return fmt.Errorf("ProductionMergeTrack: release path %s is not a git worktree (.git not found): %v", releasePath, err)
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

// branchExists reports whether a local git branch exists in the repo rooted at
// dir. Used by the cold-start bootstrap to decide between checking out an
// existing release-wt/<name> branch and creating it with `-b`.
func branchExists(ctx context.Context, dir, branch string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// ── Invariant-2 enforcement (S06) ─────────────────────────────────────────

// plannedFilesKey is a track-id → planned_files map key used in the default
// planned-files reader closure.
type plannedFilesKey struct {
	absRoot       string
	releaseName   string
	slicesByTrack map[string][]string
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
func makePlannedFilesReader(absRoot, releaseName, docsPrefix string, tracks []board.TrackInfo) func(ctx context.Context, trackID string) ([]string, error) {
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
			// Path: <docsPrefix>/release/<release>/<slice>/status.json
			path := fmt.Sprintf("%s/release/%s/%s/status.json", docsPrefix, releaseName, sliceID)
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
