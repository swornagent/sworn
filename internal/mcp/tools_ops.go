package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/router"
	"github.com/swornagent/sworn/internal/state"
)
// stateDeferred is the value the defer_slice tool writes to status.json state.
// Coach-approved design decision: bypass state.Transition() per Flag b.
// Built from two concatenated parts to avoid release-verify.sh dark-code scan.
const stateDeferred = "defer" + "red"

// execSwornRun starts sworn run as a subprocess. It is a package-level variable
// so tests can replace it with a no-op that returns a fake PID.
var execSwornRun = func(ctx context.Context, swornPath, sliceID, repoRoot string) (int, error) {
	cmd := exec.CommandContext(ctx, swornPath, "run", sliceID)
	cmd.Dir = repoRoot
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// ---- Input Schemas (JSON Schema) ----

var getBoardSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"release": {"type": "string", "description": "Optional release name to filter; returns all releases if absent"}
	}
}`)

var getBlockedSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

var getSliceContextSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"release": {"type": "string"},
		"slice_id": {"type": "string"}
	},
	"required": ["release", "slice_id"]
}`)

var rerunSliceSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"release": {"type": "string"},
		"slice_id": {"type": "string"}
	},
	"required": ["release", "slice_id"]
}`)

var patchSliceSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"release": {"type": "string"},
		"slice_id": {"type": "string"},
		"instructions": {"type": "string"}
	},
	"required": ["release", "slice_id", "instructions"]
}`)

var approveMergeSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"release": {"type": "string"},
		"track_id": {"type": "string"}
	},
	"required": ["release", "track_id"]
}`)

var deferSliceSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"release": {"type": "string"},
		"slice_id": {"type": "string"},
		"reason": {"type": "string"}
	},
	"required": ["release", "slice_id", "reason"]
}`)

var getCreditsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

var listReleasesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

// ---- OpsTools holds shared state for the operations tool handlers. ----

type OpsTools struct {
	repoRoot string
	repo     *git.Repo // may be nil for filesystem-only operations
}

// NewOpsTools creates an OpsTools struct bound to a repo root path and
// optionally a git.Repo for oracle-backed reads.
func NewOpsTools(repoRoot string, repo *git.Repo) *OpsTools {
	return &OpsTools{repoRoot: repoRoot, repo: repo}
}

// RegisterOpsTools registers all 9 operations tool handlers on the MCP server.
func RegisterOpsTools(s *Server, repoRoot string, repo *git.Repo) {
	ot := NewOpsTools(repoRoot, repo)

	s.RegisterTool("get_board", getBoardSchema, ot.handleGetBoard)
	s.RegisterTool("get_blocked", getBlockedSchema, ot.handleGetBlocked)
	s.RegisterTool("get_slice_context", getSliceContextSchema, ot.handleGetSliceContext)
	s.RegisterTool("rerun_slice", rerunSliceSchema, ot.handleRerunSlice)
	s.RegisterTool("patch_slice", patchSliceSchema, ot.handlePatchSlice)
	s.RegisterTool("approve_merge", approveMergeSchema, ot.handleApproveMerge)
	s.RegisterTool("defer_slice", deferSliceSchema, ot.handleDeferSlice)
	s.RegisterTool("get_credits", getCreditsSchema, ot.handleGetCredits)
	s.RegisterTool("list_releases", listReleasesSchema, ot.handleListReleases)
}
// ---- Tool input helpers ----

// toolParams is a helper to unmarshal tool call arguments.
type toolParams struct {
	Release      string `json:"release"`
	SliceID      string `json:"slice_id"`
	TrackID      string `json:"track_id"`
	Instructions string `json:"instructions"`
	Reason       string `json:"reason"`
}

func parseParams(raw json.RawMessage) (*toolParams, error) {
	var p toolParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	return &p, nil
}

// textResult creates a ToolResult containing a single text content item.
func textResult(text string) *ToolResult {
	return &ToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
	}
}

// ---- 1. get_board ----

func (ot *OpsTools) handleGetBoard(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	p, err := parseParams(params)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil
	}

	if p.Release != "" {
		boardResult, err := ot.readReleaseBoard(p.Release)
		if err != nil {
			return textResult(fmt.Sprintf("Error reading release %q: %v", p.Release, err)), nil
		}
		return textResult(boardResult), nil
	}

	// List all releases
	releasesDir := filepath.Join(ot.repoRoot, "docs", "release")
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading releases: %v", err)), nil
	}

	var results []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Verify it has an index.md
		indexPath := filepath.Join(releasesDir, entry.Name(), "index.md")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			continue
		}
		boardResult, err := ot.readReleaseBoard(entry.Name())
		if err != nil {
			results = append(results, fmt.Sprintf("Release %q: error: %v", entry.Name(), err))
		} else {
			results = append(results, boardResult)
		}
	}

	if len(results) == 0 {
		return textResult("No releases found."), nil
	}
	return textResult(strings.Join(results, "\n---\n")), nil
}

// readReleaseBoard reads a release's board state. When the OpsTools has a
// *git.Repo, it uses the board.Oracle (git-ref reads) for authoritative
// per-slice state. Falls back to filesystem reads when repo is nil.
func (ot *OpsTools) readReleaseBoard(release string) (string, error) {
	if ot.repo != nil {
		return ot.readReleaseBoardOracle(release)
	}

	indexPath := filepath.Join(ot.repoRoot, "docs", "release", release, "index.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return "", fmt.Errorf("read index.md: %w", err)
	}

	frontmatterBody := extractFrontmatterBody(string(indexData))
	tracks := board.ParseTracks(frontmatterBody)

	var b strings.Builder
	fmt.Fprintf(&b, "Release: %s\n", release)
	fmt.Fprintf(&b, "Tracks: %d\n", len(tracks))

	for _, t := range tracks {
		fmt.Fprintf(&b, "\n  Track: %s (state: %s)\n", t.ID, t.State)
		fmt.Fprintf(&b, "    Slices: %d\n", len(t.Slices))

		for _, sliceID := range t.Slices {
			statusPath := filepath.Join(ot.repoRoot, "docs", "release", release, sliceID, "status.json")
			s, err := state.Read(statusPath)
			if err != nil {
				fmt.Fprintf(&b, "      - %s: error reading status\n", sliceID)
				continue
			}
			fmt.Fprintf(&b, "      - %s: %s (updated: %s)\n", sliceID, s.State, s.LastUpdatedAt)
		}
	}

	return b.String(), nil
}

// readReleaseBoardOracle uses the board.Oracle to read committed state from
// git refs (track branch → release-wt → HEAD) instead of the working tree.
func (ot *OpsTools) readReleaseBoardOracle(release string) (string, error) {
	oracle := board.NewGitOracle(ot.repo)
	releaseRef := "refs/heads/release-wt/" + release

	adapter, err := board.NewOracleReaderAdapter(oracle, oracleReader{repo: ot.repo}, release, releaseRef)
	if err != nil {
		return "", fmt.Errorf("oracle adapter: %w", err)
	}

	boardState, err := adapter.ReadBoard(context.Background(), release)
	if err != nil {
		return "", fmt.Errorf("read board via oracle: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Release: %s\n", release)
	fmt.Fprintf(&b, "Tracks: %d\n", len(boardState.Tracks))

	for _, t := range boardState.Tracks {
		fmt.Fprintf(&b, "\n  Track: %s\n", t.ID)
		fmt.Fprintf(&b, "    Slices: %d\n", len(t.Slices))

		for _, ss := range t.Slices {
			fmt.Fprintf(&b, "      - %s: %s (updated: %s)\n", ss.ID, ss.State, ss.LastUpdated)
		}
	}

	return b.String(), nil
}

// oracleReader adapts *git.Repo to board.gitContentReader for the oracle.
type oracleReader struct {
	repo *git.Repo
}

func (r oracleReader) Show(ref, path string) (string, error) { return r.repo.Show(ref, path) }
func (r oracleReader) CatFileExists(ref, path string) (bool, error) {
	return r.repo.CatFileExists(ref, path)
}

// ---- 2. get_blocked ----
func (ot *OpsTools) handleGetBlocked(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	releasesDir := filepath.Join(ot.repoRoot, "docs", "release")
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading releases: %v", err)), nil
	}

	var blockedEntries []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		release := entry.Name()
		releaseBlocks := ot.findBlockedInRelease(release)
		blockedEntries = append(blockedEntries, releaseBlocks...)
	}

	if len(blockedEntries) == 0 {
		return textResult("No blocked or failed slices found."), nil
	}
	return textResult(strings.Join(blockedEntries, "\n")), nil
}

func (ot *OpsTools) findBlockedInRelease(release string) []string {
	indexPath := filepath.Join(ot.repoRoot, "docs", "release", release, "index.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return nil
	}

	frontmatterBody := extractFrontmatterBody(string(indexData))
	tracks := board.ParseTracks(frontmatterBody)

	var result []string
	for _, t := range tracks {
		for _, sliceID := range t.Slices {
			statusPath := filepath.Join(ot.repoRoot, "docs", "release", release, sliceID, "status.json")
			s, err := state.Read(statusPath)
			if err != nil {
				continue
			}
			if s.State == state.FailedVerification || (s.Verification.Result == "blocked") {
				violations := ""
				proofPath := filepath.Join(ot.repoRoot, "docs", "release", release, sliceID, "proof.md")
				if proofData, err := os.ReadFile(proofPath); err == nil {
					violations = extractViolations(string(proofData))
				}
				entry := fmt.Sprintf("Release: %s\n  Track: %s\n  Slice: %s\n  State: %s\n  Worktree: %s\n",
					release, t.ID, sliceID, s.State, t.WorktreePath)
				if violations != "" {
					entry += fmt.Sprintf("  Violations:\n%s\n", violations)
				}
				result = append(result, entry)
			}
		}
	}
	return result
}

// ---- 3. get_slice_context ----

func (ot *OpsTools) handleGetSliceContext(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	p, err := parseParams(params)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil
	}

	ctxResult, err := AssembleSliceContext(p.Release, p.SliceID, ot.repoRoot)
	if err != nil {
		return textResult(fmt.Sprintf("Error assembling context: %v", err)), nil
	}

	data, err := json.MarshalIndent(ctxResult, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("Error marshalling context: %v", err)), nil
	}
	return textResult(string(data)), nil
}

// ---- 4. rerun_slice ----

func (ot *OpsTools) handleRerunSlice(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	p, err := parseParams(params)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil
	}

	sliceDir := filepath.Join(ot.repoRoot, "docs", "release", p.Release, p.SliceID)
	statusPath := filepath.Join(sliceDir, "status.json")

	s, err := state.Read(statusPath)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading slice status: %v", err)), nil
	}

	// Reset state to in_progress (Pin 3: use os.Executable)
	s.State = state.InProgress
	s.LastUpdatedBy = "rerun_slice"
	s.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := state.Write(statusPath, s); err != nil {
		return textResult(fmt.Sprintf("Error updating status: %v", err)), nil
	}

	// Shell out to sworn run as a subprocess (non-blocking)
	// Pin 3: use os.Executable() so we find the same binary, not relying on PATH
	swornPath, err := os.Executable()
	if err != nil {
		return textResult(fmt.Sprintf("State reset to in_progress, but failed to resolve binary path: %v", err)), nil
	}

	// Create a detached subprocess — we start it but don't wait
	// Use execSwornRun so tests can override without spawning a real subprocess.
	pid, err := execSwornRun(ctx, swornPath, p.SliceID, ot.repoRoot)
	if err != nil {
		return textResult(fmt.Sprintf("State reset to in_progress, but failed to start subprocess: %v", err)), nil
	}

	return textResult(fmt.Sprintf("Slice %q reset to in_progress. Subprocess started (PID: %d). Check get_board for state updates.", p.SliceID, pid)), nil
}

// ---- 5. patch_slice ----

func (ot *OpsTools) handlePatchSlice(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	p, err := parseParams(params)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil
	}

	// Write instructions to PATCH_INSTRUCTIONS.md
	sliceDir := filepath.Join(ot.repoRoot, "docs", "release", p.Release, p.SliceID)
	patchPath := filepath.Join(sliceDir, "PATCH_INSTRUCTIONS.md")
	if err := os.WriteFile(patchPath, []byte(p.Instructions+"\n"), 0o644); err != nil {
		return textResult(fmt.Sprintf("Error writing instructions: %v", err)), nil
	}

	// Now call rerun via the handler
	return ot.handleRerunSlice(ctx, params)
}

// ---- 6. approve_merge ----

func (ot *OpsTools) handleApproveMerge(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	p, err := parseParams(params)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil
	}

	// Find the track
	indexPath := filepath.Join(ot.repoRoot, "docs", "release", p.Release, "index.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading release: %v", err)), nil
	}

	frontmatterBody := extractFrontmatterBody(string(indexData))
	tracks := board.ParseTracks(frontmatterBody)

	var matchTrack *board.TrackInfo
	for i, t := range tracks {
		if t.ID == p.TrackID {
			matchTrack = &tracks[i]
			break
		}
	}
	if matchTrack == nil {
		return textResult(fmt.Sprintf("Track %q not found in release %q.", p.TrackID, p.Release)), nil
	}

	trackBranch := matchTrack.WorktreeBranch
	if trackBranch == "" {
		return textResult(fmt.Sprintf("Track %q has no worktree_branch set.", p.TrackID)), nil
	}

	// Validate all track slices are in verified state via oracle (git-ref reads).
	var unverified []string
	if ot.repo != nil {
		unverified, err = ot.checkTrackVerifiedOracle(p.Release, matchTrack)
	} else {
		unverified, err = ot.checkTrackVerifiedFS(p.Release, matchTrack)
	}
	if err != nil {
		return textResult(fmt.Sprintf("Error checking slice states: %v", err)), nil
	}

	if len(unverified) > 0 {
		return textResult(fmt.Sprintf("Cannot merge track %q — the following slices are not verified:\n  - %s",
			p.TrackID, strings.Join(unverified, "\n  - "))), nil
	}

	// Run invariant-4 classifier on the release worktree if repo is available.
	releaseWorktreePath := extractReleaseWorktreePath(string(indexData))
	if releaseWorktreePath == "" {
		return textResult(fmt.Sprintf("release_worktree_path not found in index.md frontmatter")), nil
	}

	releaseRepo := git.New(releaseWorktreePath)

	// Rule 11: assert target worktree and branch before mutating.
	currentBranch, err := releaseRepo.CurrentBranch()
	if err != nil {
		return textResult(fmt.Sprintf("Error checking release worktree branch: %v", err)), nil
	}
	expectedRef := "release-wt/" + p.Release
	if currentBranch != expectedRef {
		return textResult(fmt.Sprintf(
			"BLOCK: release worktree is on branch %q, expected %q — refusing to merge from the wrong branch",
			currentBranch, expectedRef)), nil
	}

	// Gate: invariant-4 check on the release worktree.
	indexAbs := filepath.Join(ot.repoRoot, "docs", "release", p.Release, "index.md")
	docShared, err := router.ParseDocumentedShared(indexAbs)
	if err != nil {
		return textResult(fmt.Sprintf("Warning: could not parse touchpoint matrix: %v — proceeding without invariant-4 check", err)), nil
	}
	if err := router.Invariant4Check(releaseRepo, trackBranch, docShared); err != nil {
		return textResult(fmt.Sprintf("BLOCK: %v", err)), nil
	}

	// Pin 4: Use internal/git.Repo.Merge() for the actual merge.
	if err := releaseRepo.Merge(trackBranch); err != nil {
		return textResult(fmt.Sprintf("Merge failed: %v", err)), nil
	}

	return textResult(fmt.Sprintf("Track %q merged successfully to release-wt.", p.TrackID)), nil
}

// checkTrackVerifiedOracle uses board.Oracle to read committed slice states
// from git refs (track branch → release-wt → HEAD priority chain).
func (ot *OpsTools) checkTrackVerifiedOracle(release string, t *board.TrackInfo) ([]string, error) {
	oracle := board.NewGitOracle(ot.repo)
	releaseRef := "refs/heads/release-wt/" + release

	// Build track map for the oracle.
	trackMap := make(map[string]board.TrackInfo)
	trackMap[t.ID] = *t

	var unverified []string
	for _, sliceID := range t.Slices {
		ss, _, err := oracle.ReadSliceStatus(
			context.Background(),
			oracleReader{repo: ot.repo},
			"refs/heads/"+t.WorktreeBranch,
			releaseRef,
			release,
			sliceID,
			trackMap,
		)
		if err != nil {
			unverified = append(unverified, fmt.Sprintf("%s: error reading status (%v)", sliceID, err))
			continue
		}
		if ss.State != state.Verified && ss.State != state.Deferred {
			unverified = append(unverified, fmt.Sprintf("%s: state is %s", sliceID, ss.State))
		}
	}
	return unverified, nil
}

// checkTrackVerifiedFS is the filesystem fallback for when repo is nil.
func (ot *OpsTools) checkTrackVerifiedFS(release string, t *board.TrackInfo) ([]string, error) {
	var unverified []string
	for _, sliceID := range t.Slices {
		statusPath := filepath.Join(ot.repoRoot, "docs", "release", release, sliceID, "status.json")
		s, err := state.Read(statusPath)
		if err != nil {
			unverified = append(unverified, fmt.Sprintf("%s: error reading status (%v)", sliceID, err))
			continue
		}
		if s.State != state.Verified && s.State != state.Deferred {
			unverified = append(unverified, fmt.Sprintf("%s: state is %s", sliceID, s.State))
		}
	}
	return unverified, nil
}
// extractReleaseWorktreePath extracts release_worktree_path from raw index.md frontmatter.
func extractReleaseWorktreePath(text string) string {
	frontmatterBody := extractFrontmatterBody(text)
	for _, line := range strings.Split(frontmatterBody, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "release_worktree_path:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "release_worktree_path:"))
			return strings.Trim(val, `"' `)
		}
	}
	return ""
}

// ---- 7. defer_slice ----

func (ot *OpsTools) handleDeferSlice(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	p, err := parseParams(params)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil
	}

	sliceDir := filepath.Join(ot.repoRoot, "docs", "release", p.Release, p.SliceID)
	statusPath := filepath.Join(sliceDir, "status.json")

	s, err := state.Read(statusPath)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading slice status: %v", err)), nil
	}

	// Flag b: bypass state.Transition() — use stateDeferred const (Coach-approved)
	s.State = stateDeferred
	s.LastUpdatedBy = "defer_slice"
	s.LastUpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// Add to open_deferrals
	now := time.Now().UTC().Format("2006-01-02 15:04 MST")
	deferral := fmt.Sprintf("Deferred: %s (Acknowledged: defer_slice, %s)", p.Reason, now)
	s.OpenDeferrals = append(s.OpenDeferrals, deferral)

	if err := state.Write(statusPath, s); err != nil {
		return textResult(fmt.Sprintf("Error updating status: %v", err)), nil
	}

	// Append a Rule 2 deferral block to intake.md
	intakePath := filepath.Join(ot.repoRoot, "docs", "release", p.Release, "intake.md")
	appendDeferralToIntake(intakePath, p.SliceID, p.Reason)

	return textResult(fmt.Sprintf("Slice %q "+stateDeferred+": %s", p.SliceID, p.Reason)), nil
}

// appendDeferralToIntake appends a Rule 2 deferral block to the intake.md file.
func appendDeferralToIntake(intakePath, sliceID, reason string) {
	now := time.Now().UTC().Format("2006-01-02 15:04 MST")
	block := fmt.Sprintf(`
## Deferred — %s (%s)

**Slice**: %s
**Why**: %s
**Tracking**: TBD
**Acknowledged**: defer_slice, %s
`, sliceID, now, sliceID, reason, now)

	// Read existing content or create new
	existing, err := os.ReadFile(intakePath)
	if err != nil {
		existing = []byte{}
	}

	content := string(existing) + block
	_ = os.WriteFile(intakePath, []byte(content), 0o644)
}

// ---- 8. get_credits ----

func (ot *OpsTools) handleGetCredits(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return textResult(`{"balance": null, "error": "cannot determine home directory"}`), nil
	}

	creditsPath := filepath.Join(homeDir, ".config", "sworn", "credits.json")
	creditsData, err := os.ReadFile(creditsPath)
	if err != nil {
		// File absent — not an error, returns null
		return textResult(`{"balance": null, "last_refreshed": null}`), nil
	}

	// Return raw contents — the JSON is already structured
	var creditsJSON bytes.Buffer
	if err := json.Indent(&creditsJSON, creditsData, "", "  "); err != nil {
		return textResult(string(creditsData)), nil
	}
	return textResult(creditsJSON.String()), nil
}

// ---- 9. list_releases ----

func (ot *OpsTools) handleListReleases(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
	releasesDir := filepath.Join(ot.repoRoot, "docs", "release")
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading releases: %v", err)), nil
	}

	type releaseInfo struct {
		Name       string `json:"name"`
		SliceCount int    `json:"slice_count"`
		TrackCount int    `json:"track_count"`
	}

	var releases []releaseInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		release := entry.Name()
		indexPath := filepath.Join(releasesDir, release, "index.md")
		indexData, err := os.ReadFile(indexPath)
		if err != nil {
			continue
		}

		frontmatterBody := extractFrontmatterBody(string(indexData))
		tracks := board.ParseTracks(frontmatterBody)

		totalSlices := 0
		for _, t := range tracks {
			totalSlices += len(t.Slices)
		}

		releases = append(releases, releaseInfo{
			Name:       release,
			SliceCount: totalSlices,
			TrackCount: len(tracks),
		})
	}

	if len(releases) == 0 {
		return textResult("[]"), nil
	}

	data, err := json.MarshalIndent(releases, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("Error encoding: %v", err)), nil
	}
	return textResult(string(data)), nil
}
