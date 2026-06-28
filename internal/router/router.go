// Package router deterministically routes a slice's current state to the next
// command. It ports the reference bash router into pure Go: the same
// decision tree, the same JSON .next output, no LLM. See spec S58-slice-router.
//
// Route is a pure function: all I/O flows through the injected interfaces,
// making the router table-testable with fakes.
package router

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
)
// NextType enumerates the kinds of next command the router can recommend.
type NextType string

const (
	NextImplement     NextType = "implement"
	NextReview        NextType = "review"
	NextVerify        NextType = "verify"
	NextMergeTrack    NextType = "merge-track"
	NextMergeRelease  NextType = "merge-release"
	NextReplanRelease NextType = "replan-release"
	NextRedesign      NextType = "redesign"
	NextCoachDecision NextType = "coach_decision"
	NextNone          NextType = "none"
)

// Decision is the router's output — a deterministic recommendation for what
// command to run next for a slice.
type Decision struct {
	NextType    NextType `json:"type"`
	NextCommand string   `json:"command"`
	NextReason  string   `json:"reason"`
	TargetSlice string   `json:"target_slice,omitempty"`
	TargetTrack string   `json:"target_track,omitempty"`
}

// OracleReader is the state-read interface the router consumes. It hides
// git-ref resolution and track-map construction behind simple signatures.
type OracleReader interface {
	ReadSliceStatus(ctx context.Context, release, sliceID string) (board.SliceState, error)
	ReadBoard(ctx context.Context, release string) (*board.BoardState, error)
}

// ContentReader is the git-object-read interface for commit-time queries,
// file existence checks, and ancestry tests. Separate from OracleReader
// per interface segregation.
type ContentReader interface {
	LastCommitTime(ref, path string) (int64, error)
	CatFileExists(ref, path string) (bool, error)
	IsAncestor(ancestor, branch string) (bool, error)
}

// RouteInput carries the resolved parameters Route needs from the CLI layer.
type RouteInput struct {
	Release     string
	SliceID     string
	TrackID     string
	TrackBranch string // e.g. "refs/heads/track/<release>/<track>"
	ReleaseRef  string // e.g. "refs/heads/release-wt/<release>"
	DocsPrefix  string // e.g. "docs" or "apps/docs/content/docs"
}

// Route deterministically computes the next command for a slice given its
// committed state. All I/O flows through the injected readers; the function
// itself is pure and table-testable.
func Route(
	ctx context.Context,
	oracle OracleReader,
	content ContentReader,
	input RouteInput,
) (Decision, error) {
	ss, err := oracle.ReadSliceStatus(ctx, input.Release, input.SliceID)
	if err != nil {
		return Decision{}, fmt.Errorf("read slice status: %w", err)
	}

	s := string(ss.State)

	// ---------- BLOCKED precedes state ----------
	if ss.Blocked {
		reason := buildVerificationReason(ss)
		return Decision{
			NextType:    NextReplanRelease,
			NextCommand: fmt.Sprintf("/replan-release %s", input.Release),
			NextReason: fmt.Sprintf(
				"Verifier BLOCKED with reason: %s. Only the Planner can clear a BLOCKED verdict — re-running /verify-slice or /implement-slice will re-enter the same loop.",
				reason,
			),
		}, nil
	}

	switch s {
	// ---------- design_review ----------
	case "design_review":
		return routeDesignReview(ctx, content, input)

	// ---------- failed_verification ----------
	case "failed_verification":
		return routeFailedVerification(ss, input)

	// ---------- implemented ----------
	case "implemented":
		return routeImplemented(ss, input)

	// ---------- in_progress ----------
	case "in_progress":
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Slice is in_progress but no implementer is live (the loop is synchronous — it only routes between dispatches). A prior /implement-slice died mid-flight and stranded the slice. Resume /implement-slice — it picks up the in_progress slice + partial work and drives to implemented. stuck_check backstops genuine non-progress.",
		}, nil

	// ---------- planned ----------
	case "planned":
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Slice in planned. Dispatch /implement-slice — the Design TL;DR gate (Step 4) will halt for Coach review before any code lands.",
		}, nil

	// ---------- shipped ----------
	case "shipped":
		return Decision{
			NextType:   NextNone,
			NextReason: "Slice is shipped (terminal). No routing needed.",
		}, nil

	// ---------- deferred (top-level fall-through) ----------
	case "deferred":
		return Decision{
			NextType:   NextNone,
			NextReason: "Slice is deferred (terminal). No routing needed — resume via /implement-slice when un-deferred.",
		}, nil

	// ---------- verified ----------
	case "verified":
		return routeVerified(ctx, oracle, content, input)

	// ---------- unrecognised ----------
	default:
		return Decision{
			NextType:   NextNone,
			NextReason: fmt.Sprintf("Slice in unrecognised state '%s'. Inspect status.json manually.", s),
		}, nil
	}
}

// routeDesignReview handles the design_review sub-state routing by commit-time-
// newest artefact (the S06 overnight-spin guard).
func routeDesignReview(ctx context.Context, content ContentReader, input RouteInput) (Decision, error) {
	trackRef := input.TrackBranch
	if !strings.HasPrefix(trackRef, "refs/heads/") {
		trackRef = "refs/heads/" + trackRef
	}

	// captain-proceed.md presence via committed-ref (CatFileExists).
	ackPath := fmt.Sprintf("%s/release/%s/%s/captain-proceed.md", input.DocsPrefix, input.Release, input.SliceID)
	ackExists, err := content.CatFileExists(trackRef, ackPath)
	if err != nil {
		return Decision{}, fmt.Errorf("check captain-proceed.md: %w", err)
	}

	if ackExists {
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Coach approved the ack (captain-proceed.md present). Resume /implement-slice — it'll read the ack, transition to in_progress, and write code.",
		}, nil
	}

	// Commit times for design/review/decline.
	designPath := fmt.Sprintf("%s/release/%s/%s/design.md", input.DocsPrefix, input.Release, input.SliceID)
	reviewPath := fmt.Sprintf("%s/release/%s/%s/review.md", input.DocsPrefix, input.Release, input.SliceID)
	declinePath := fmt.Sprintf("%s/release/%s/%s/decline.md", input.DocsPrefix, input.Release, input.SliceID)

	ctDesign, _ := content.LastCommitTime(trackRef, designPath)
	ctReview, _ := content.LastCommitTime(trackRef, reviewPath)
	ctDecline, _ := content.LastCommitTime(trackRef, declinePath)

	// Which artefact is newest? Ties resolve design < decline < review.
	newest := "design"
	max := ctDesign
	if ctDecline > max {
		newest = "decline"
		max = ctDecline
	}
	if ctReview > max {
		newest = "review"
		max = ctReview
	}

	if newest == "review" && max > 0 {
		return Decision{
			NextType:    NextCoachDecision,
			NextCommand: fmt.Sprintf("coach ack %s  OR  coach decline %s \"<reason>\"", input.SliceID, input.SliceID),
			NextReason:  "Captain has reviewed the current design (review.md is the newest artefact). Coach must approve or decline before implementation proceeds. Read review.md, then run: coach ack <slice> (accept suggested ack), coach ack <slice> --edit (edit before accepting), or coach decline <slice> \"<reason>\" (push back to implementer).",
		}, nil
	}

	if newest == "decline" && max > 0 {
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Coach declined and the design has not yet been revised (decline.md is newer than design.md). Resume /implement-slice — it'll read the push-back, address the pins, and re-commit a revised design.md.",
		}, nil
	}

	return Decision{
		NextType:    NextReview,
		NextCommand: fmt.Sprintf("/design-review %s %s", input.SliceID, input.Release),
		NextReason:  "design.md is the newest artefact (fresh, or revised after a Coach decline) and awaits Captain review. Run /design-review in a fresh terminal — Captain produces pins + a new review.md. (A decline that has been addressed routes here, not back to implement — guards the S06 overnight-spin regression.)",
	}, nil
}

// routeFailedVerification classifies violations by gate to determine re-entry.
func routeFailedVerification(ss board.SliceState, input RouteInput) (Decision, error) {
	// Check for Gate 1/2/6 violations (design-level, need re-design).
	designLevel := false
	for _, v := range ss.Violations {
		vl := strings.ToLower(v)
		if strings.Contains(vl, "gate 1") || strings.Contains(vl, "gate 2") || strings.Contains(vl, "gate 6") {
			designLevel = true
			break
		}
	}

	if designLevel {
		return Decision{
			NextType:    NextRedesign,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
			NextReason:  "Verifier FAIL includes Gate 1/2/6 (design-level) violations. The loop will remove captain-proceed.md so the Design TL;DR gate fires again — implementer rewrites design.md addressing the violations, then Captain re-reviews before code is written.",
		}, nil
	}

	return Decision{
		NextType:    NextImplement,
		NextCommand: fmt.Sprintf("/implement-slice %s %s", input.SliceID, input.Release),
		NextReason:  "Verifier FAIL on Gate 3/4/5 (mechanical). Implementer addresses violations directly in a fresh session without needing design re-review.",
	}, nil
}

// routeImplemented always routes to verify.
func routeImplemented(ss board.SliceState, input RouteInput) (Decision, error) {
	var reason string
	switch ss.VerificationResult {
	case "":
		reason = "Slice is implemented; no verifier verdict recorded. Run /verify-slice in a FRESH terminal (Rule 7)."
	case "pending":
		reason = "Slice is implemented with verification.result=pending. A prior verifier session likely crashed or was killed mid-run (stale pending). Dispatch a fresh /verify-slice — the new verdict will overwrite. If a verifier is genuinely mid-flight elsewhere, the later verdict simply wins."
	default:
		reason = fmt.Sprintf("Slice is implemented again after a prior verifier verdict (stale result=%s). Implementer addressed the violations and re-transitioned to implemented; the stale verdict no longer applies. Run /verify-slice — verifier will overwrite the stale result.", ss.VerificationResult)
	}

	return Decision{
		NextType:    NextVerify,
		NextCommand: fmt.Sprintf("/verify-slice %s %s", input.SliceID, input.Release),
		NextReason:  reason,
	}, nil
}

// routeVerified walks the track for the next non-terminal slice, or decides
// merge-track vs merge-release if the track is fully terminal.
func routeVerified(ctx context.Context, oracle OracleReader, content ContentReader, input RouteInput) (Decision, error) {
	boardState, err := oracle.ReadBoard(ctx, input.Release)
	if err != nil {
		return Decision{}, fmt.Errorf("read board: %w", err)
	}

	// Find this track.
	var thisTrack *board.TrackState
	for i := range boardState.Tracks {
		if boardState.Tracks[i].ID == input.TrackID {
			thisTrack = &boardState.Tracks[i]
			break
		}
	}
	if thisTrack == nil {
		return Decision{}, fmt.Errorf("track %s not found in board", input.TrackID)
	}

	// Walk track slices forward from the current slice.
	passedSelf := false
	for _, s := range thisTrack.Slices {
		if !passedSelf {
			if s.ID == input.SliceID {
				passedSelf = true
			}
			continue
		}

		// Ghost-slice filter: only consider slices owned by this track.
		if s.Track != input.TrackID {
			continue
		}

		// Terminal states: skip (verified, shipped, deferred).
		terminal := false
		switch string(s.State) {
		case "verified", "shipped", "deferred":
			terminal = true
		}

		if !terminal {
			return routeNextSlice(ctx, content, s, input)
		}
	}

	// No more non-terminal slices in this track. Decide merge-track vs merge-release.
	return routeMergeDecision(boardState, content, input)
}

// routeNextSlice returns the right command for the next non-terminal sibling.
func routeNextSlice(ctx context.Context, content ContentReader, s board.SliceState, input RouteInput) (Decision, error) {
	ss := string(s.State)
	switch ss {
	case "planned":
		// Check if the planned sibling has a design.md — if so, route review
		// (Design TL;DR gate fires before code).
		trackRef := input.TrackBranch
		if !strings.HasPrefix(trackRef, "refs/heads/") {
			trackRef = "refs/heads/" + trackRef
		}
		designPath := fmt.Sprintf("%s/release/%s/%s/design.md", input.DocsPrefix, input.Release, s.ID)
		designExists, err := content.CatFileExists(trackRef, designPath)
		if err != nil {
			return Decision{}, fmt.Errorf("check design.md for %s: %w", s.ID, err)
		}
		if designExists {
			return Decision{
				NextType:    NextReview,
				NextCommand: fmt.Sprintf("/design-review %s %s", s.ID, input.Release),
				NextReason:  fmt.Sprintf("%s is verified. Next slice in track (%s) is %s — it already has a design.md; review it before code is written (Design TL;DR gate).", input.SliceID, input.TrackID, s.ID),
				TargetSlice: s.ID,
				TargetTrack: input.TrackID,
			}, nil
		}
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next planned slice in track (%s) is %s.", input.SliceID, input.TrackID, s.ID),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil
	case "design_review":
		return Decision{
			NextType:    NextReview,
			NextCommand: fmt.Sprintf("/design-review %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next slice in track (%s) is %s — it already has a design.md; review it before code is written.", input.SliceID, input.TrackID, s.ID),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil

	case "in_progress", "implemented", "failed_verification":
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next slice in track (%s) is %s (currently %s) — continue where it left off.", input.SliceID, input.TrackID, s.ID, ss),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil

	default:
		return Decision{
			NextType:    NextImplement,
			NextCommand: fmt.Sprintf("/implement-slice %s %s", s.ID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified. Next slice in track (%s) is %s (state=%s).", input.SliceID, input.TrackID, s.ID, ss),
			TargetSlice: s.ID,
			TargetTrack: input.TrackID,
		}, nil
	}
}

// routeMergeDecision decides between merge-track and merge-release when a
// track has no more non-terminal slices.
func routeMergeDecision(
	boardState *board.BoardState,
	content ContentReader,
	input RouteInput,
) (Decision, error) {
	// Count undone (non-terminal) slices across the whole release.
	undone := 0
	for _, t := range boardState.Tracks {
		for _, s := range t.Slices {
			switch string(s.State) {
			case "verified", "shipped", "deferred":
				// terminal
			default:
				undone++
			}
		}
	}

	if undone > 0 {
		return Decision{
			NextType:    NextMergeTrack,
			NextCommand: fmt.Sprintf("/merge-track %s %s", input.TrackID, input.Release),
			NextReason:  fmt.Sprintf("%s is verified — track %s has no more non-terminal slices. Merge this track now; %d slice(s) remain across other tracks.", input.SliceID, input.TrackID, undone),
			TargetTrack: input.TrackID,
		}, nil
	}

	// All slices terminal. Check if any track hasn't been merged into release-wt.
	releaseRef := input.ReleaseRef
	if !strings.HasPrefix(releaseRef, "refs/heads/") {
		releaseRef = "refs/heads/" + releaseRef
	}

	unmergedTrack := findFirstUnmergedTrack(content, boardState, releaseRef)
	if unmergedTrack != "" {
		return Decision{
			NextType:    NextMergeTrack,
			NextCommand: fmt.Sprintf("/merge-track %s %s", unmergedTrack, input.Release),
			NextReason:  fmt.Sprintf("All owned slices verified across %s, but track %s has not yet been merged into release-wt (%s). Merge it before /merge-release — otherwise the release branch is missing that track's commits.", input.Release, unmergedTrack, releaseRef),
			TargetTrack: unmergedTrack,
		}, nil
	}

	return Decision{
		NextType:    NextMergeRelease,
		NextCommand: fmt.Sprintf("/merge-release %s", input.Release),
		NextReason:  fmt.Sprintf("%s is verified — every slice across %s is in a terminal state and every track has been merged into release-wt (%s). Time to merge the release.", input.SliceID, input.Release, releaseRef),
	}, nil
}

// findFirstUnmergedTrack returns the ID of the first track whose branch is not
// an ancestor of releaseRef, or "" if all tracks are merged.
func findFirstUnmergedTrack(content ContentReader, boardState *board.BoardState, releaseRef string) string {
	// Sort tracks by ID for deterministic output.
	ids := make([]string, 0, len(boardState.Tracks))
	for _, t := range boardState.Tracks {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)

	for _, tid := range ids {
		var wb string
		for _, t := range boardState.Tracks {
			if t.ID == tid {
				wb = t.WorktreeBranch
				break
			}
		}
		if wb == "" {
			continue
		}
		// Try local ref, then origin/ fallback.
		ancestor, err := content.IsAncestor(wb, releaseRef)
		if err != nil {
			originRef := "origin/" + wb
			ancestor, err = content.IsAncestor(originRef, releaseRef)
			if err != nil {
				continue
			}
		}
		if !ancestor {
			return tid
		}
	}
	return ""
}

// buildVerificationReason builds the reason string for a BLOCKED verdict.
// Uses violations joined by "; " (the .reason field was a bash-only jq
// fallback that was never populated in practice — no status.json has it).
func buildVerificationReason(ss board.SliceState) string {
	if len(ss.Violations) == 0 {
		return "no violations recorded"
	}
	return strings.Join(ss.Violations, "; ")
}

// Invariant4Check performs a dry-run merge of trackBranch into the current
// branch and checks that any conflicted files are documented shared files.
// If a conflict is on a file NOT in documentedShared, it returns an error
// naming the offending file. On a clean merge or all-documented-shared
// conflicts, it restores the working tree and returns nil.
//
// Rule 11 (process-global mutation guard): this function mutates the working
// tree. The caller must assert the target worktree and branch are expected
// before calling. The function restores the tree via abort/reset in every path.
func Invariant4Check(repo *git.Repo, trackBranch string, documentedShared map[string]bool) error {
	// Rule 11 — fail-closed target assertion: the repo must have a Dir set.
	if repo == nil || repo.Dir == "" {
		return fmt.Errorf("invariant-4: repo.Dir is empty — refusing to operate on ambient working directory")
	}

	// Gate 0: working tree must be clean.
	porcelain, err := repo.StatusPorcelain()
	if err != nil {
		return fmt.Errorf("invariant-4: pre-check: %w", err)
	}
	if strings.TrimSpace(porcelain) != "" {
		return fmt.Errorf("invariant-4: working tree is not clean — commit or stash changes before merging")
	}

	// Run the dry-run merge.
	conflictFiles, err := repo.MergeDryRun(trackBranch)
	if err != nil {
		// MergeDryRun returned a real error (not a conflict).
		return fmt.Errorf("invariant-4: dry-run merge: %w", err)
	}

	if len(conflictFiles) == 0 {
		// Clean merge — undo it.
		if resetErr := repo.ResetMerge(); resetErr != nil {
			return fmt.Errorf("invariant-4: clean merge but reset failed: %w", resetErr)
		}
		return nil
	}

	// Conflicts detected. Check each against the documented-shared set.
	var violations []string
	for _, f := range conflictFiles {
		if !isDocumentedShared(f, documentedShared) {
			violations = append(violations, f)
		}
	}

	// Abort the merge in every path.
	if abortErr := repo.MergeAbort(); abortErr != nil {
		return fmt.Errorf("invariant-4: merge abort failed: %w", abortErr)
	}

	if len(violations) > 0 {
		return fmt.Errorf("BLOCK: invariant-4 violation — conflict on %s (not a documented shared file)",
			strings.Join(violations, ", "))
	}

	// All conflicts are on documented shared files — invariant-4 satisfied.
	return nil
}

// isDocumentedShared checks whether a conflicted file path matches any entry
// in the documented-shared set. Uses prefix matching: a conflict on
// "internal/model/oai.go" matches the documented-shared entry
// "internal/model/oai.go + drivers".
func isDocumentedShared(path string, documentedShared map[string]bool) bool {
	if path == "" {
		return false
	}
	if documentedShared[path] {
		return true
	}
	// Prefix match: the documented-shared keys may be base paths.
	for key := range documentedShared {
		if strings.HasPrefix(path, key) || strings.HasPrefix(key, path) {
			return true
		}
	}
	return false
}
// touchpointRow holds a parsed row from the index.md touchpoint matrix.
type touchpointRow struct {
	filePath string
	tracks   map[string]bool // track ID → has checkmark
}

// ParseDocumentedShared reads an index.md at the given path and extracts the
// documented-shared file set from the touchpoint matrix. A file is documented
// shared when it is explicitly marked "(DOCUMENTED SHARED)" in the first
// column, OR when ≥2 tracks have a checkmark (✓) for that file.
func ParseDocumentedShared(indexPath string) (map[string]bool, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("parse documented shared: read index.md: %w", err)
	}

	rows, err := parseTouchpointMatrix(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse documented shared: %w", err)
	}

	shared := make(map[string]bool)
	for _, row := range rows {
		// Explicitly marked as DOCUMENTED SHARED.
		if strings.Contains(row.filePath, "DOCUMENTED SHARED") {
			key := normalizeFilePath(row.filePath)
			if key != "" {
				shared[key] = true
			}
			continue
		}
		// ≥2 tracks have checkmarks.
		count := 0
		for _, has := range row.tracks {
			if has {
				count++
			}
		}
		if count >= 2 {
			key := normalizeFilePath(row.filePath)
			if key != "" {
				shared[key] = true
			}
		}
	}
	return shared, nil
}

// normalizeFilePath strips backtick quoting, trims whitespace, and removes
// leading/trailing markers from a file path cell in the touchpoint matrix.
func normalizeFilePath(raw string) string {
	s := strings.TrimSpace(raw)
	// Strip leading backtick.
	s = strings.TrimPrefix(s, "`")
	// Strip trailing backtick — it may be followed by annotations.
	if idx := strings.Index(s, "`"); idx >= 0 {
		// Only strip if the backtick is at the end or followed by space/paren.
		after := s[idx+1:]
		if after == "" || after[0] == ' ' || after[0] == '(' {
			s = s[:idx] + after
		}
	}
	s = strings.TrimSpace(s)
	// Remove parenthesised annotations like "(DOCUMENTED SHARED)" or "(new)".
	s = regexp.MustCompile(`\s*\([^)]*\)`).ReplaceAllString(s, "")
	// Remove trailing annotations like " + drivers".
	s = regexp.MustCompile(`\s+\+.*$`).ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	return s
}
// parseTouchpointMatrix parses the markdown table in the Touchpoint matrix
// section of an index.md body. Returns parsed rows.
func parseTouchpointMatrix(body string) ([]touchpointRow, error) {
	// Find the Touchpoint matrix table.
	idx := strings.Index(body, "### Touchpoint matrix")
	if idx < 0 {
		idx = strings.Index(body, "## Touchpoint matrix")
	}
	if idx < 0 {
		// Try "Touchpoint matrix" anywhere.
		idx = strings.Index(body, "Touchpoint matrix")
	}
	if idx < 0 {
		return nil, fmt.Errorf("touchpoint matrix not found in index.md")
	}

	tableSection := body[idx:]

	// Find the table header line (starts with | File / surface |).
	lines := strings.Split(tableSection, "\n")
	headerIdx := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "| File") || strings.HasPrefix(strings.TrimSpace(line), "| `") {
			// If it's a separator line like |---|---|..., skip.
			if strings.Contains(line, "---") && !strings.Contains(line, "`") {
				continue
			}
			if strings.Contains(line, "File") {
				headerIdx = i
				break
			}
		}
	}
	if headerIdx < 0 {
		return nil, fmt.Errorf("touchpoint matrix header not found")
	}

	// Parse the header to extract track columns.
	headerCells := splitTableRow(lines[headerIdx])
	if len(headerCells) < 2 {
		return nil, fmt.Errorf("touchpoint matrix header has too few columns")
	}
	// Track columns start at index 1 (index 0 is "File / surface").
	trackIDs := headerCells[1:]

	// Find the separator line (next line, should be |---|...).
	sepIdx := headerIdx + 1
	if sepIdx >= len(lines) || !strings.Contains(lines[sepIdx], "---") {
		return nil, fmt.Errorf("touchpoint matrix separator not found after header")
	}

	var rows []touchpointRow
	for i := sepIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || !strings.HasPrefix(line, "|") {
			// End of table.
			break
		}
		cells := splitTableRow(line)
		if len(cells) < 2 {
			continue
		}
		filePath := cells[0]

		trackChecks := make(map[string]bool)
		for j, cell := range cells[1:] {
			if j < len(trackIDs) {
				trackChecks[trackIDs[j]] = strings.Contains(cell, "✓")
			}
		}
		rows = append(rows, touchpointRow{
			filePath: filePath,
			tracks:   trackChecks,
		})
	}

	return rows, nil
}

// splitTableRow splits a markdown table row by |, trimming whitespace.
func splitTableRow(line string) []string {
	// Remove leading and trailing |.
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")

	parts := strings.Split(s, "|")
	result := make([]string, len(parts))
	for i, p := range parts {
		result[i] = strings.TrimSpace(p)
	}
	return result
}