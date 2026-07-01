package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/board"
)

// SliceContext holds the assembled context for a single slice, as returned by
// get_slice_context: spec, violations, diff, and journal content.
type SliceContext struct {
	SpecContent    string `json:"spec_content"`
	Violations     string `json:"violations"`
	Diff           string `json:"diff"`
	DiffNote       string `json:"diff_note,omitempty"`
	JournalContent string `json:"journal_content"`
	WorktreePath   string `json:"worktree_path"`
	StartCommit    string `json:"start_commit"`
	SliceState     string `json:"slice_state"`
}

// AssembleSliceContext reads slice artefacts from the release directory and
// the track worktree, returning the assembled context. repoRoot is the
// absolute path to the repo root (e.g. the release-wt or project root).
//
// Pin 1: git diff errors are caught gracefully — on any non-zero exec exit or
// exec error, diff is set to "" and diff_note carries the reason.
//
// S04-mcp-oracle-migration: track metadata is read via the board.Oracle
// (board.ReadBoard → board.json with lazy index.md migration) instead of
// parsing index.md frontmatter directly. This keeps the path resolution
// consistent with the rest of the MCP tools (board reads, get_blocked,
// approve_merge) and avoids the silently-empty-tracks bug a stale
// frontmatter parse would produce.
func AssembleSliceContext(release, sliceID, repoRoot string) (*SliceContext, error) {
	sliceDir := filepath.Join(repoRoot, "docs", "release", release, sliceID)

	// 1. Read spec (spec.json preferred; fall back to spec.md for legacy slices).
	var specData []byte
	if data, err := os.ReadFile(filepath.Join(sliceDir, "spec.json")); err == nil {
		specData = data
	} else if data, err := os.ReadFile(filepath.Join(sliceDir, "spec.md")); err == nil {
		specData = data
	} else {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	// 2. Read violations from proof.json.not_delivered (AC-02).
	violations := readProofViolations(sliceDir)

	// 3. Read journal.md
	var journalContent string
	if journalData, err := os.ReadFile(filepath.Join(sliceDir, "journal.md")); err == nil {
		journalContent = string(journalData)
	}

	// 4. Resolve the slice's track + worktree_path via the board oracle.
	//    board.ReadBoard reads board.json (or lazy-migrates index.md →
	//    board.json) so the worktree_path matches what the renderer emits
	//    and what every other MCP tool consumes. TrackID from status.json
	//    is a hint, not a filter — the only authoritative key is the
	//    slice's membership in a track's Slices list, so we accept the
	//    first matching track even when the status's track field is
	//    missing or stale.
	var worktreePath, trackID string
	statusPath := filepath.Join(sliceDir, "status.json")
	if statusData, err := os.ReadFile(statusPath); err == nil {
		trackID = extractField(string(statusData), "track")
	}
	if br, err := board.ReadBoard(repoRoot, release); err == nil {
		for _, t := range br.Tracks {
			for _, sid := range t.Slices {
				if sid == sliceID {
					worktreePath = t.WorktreePath
					break
				}
			}
			if worktreePath != "" {
				break
			}
		}
		_ = trackID // hint only — see comment above.
	}

	// 5. Read status.json for start_commit and current state
	var startCommit, sliceState string
	if statusData, err := os.ReadFile(statusPath); err == nil {
		startCommit = extractField(string(statusData), "start_commit")
		sliceState = extractField(string(statusData), "state")
	}

	// 6. Run git diff from start_commit..HEAD in the worktree (Pin 1: safe wrap)
	diff, diffNote := runDiff(worktreePath, startCommit)

	return &SliceContext{
		SpecContent:    string(specData),
		Violations:     violations,
		Diff:           diff,
		DiffNote:       diffNote,
		JournalContent: journalContent,
		WorktreePath:   worktreePath,
		StartCommit:    startCommit,
		SliceState:     sliceState,
	}, nil
}

// readProofViolations reads not_delivered from proof.json (AC-02 — the
// proof-v1 record is the sole source of truth for violations; there is no
// proof.md fallback). A slice with no proof.json, an unparseable one, or an
// empty not_delivered list has no violations to report.
func readProofViolations(sliceDir string) string {
	data, err := os.ReadFile(filepath.Join(sliceDir, "proof.json"))
	if err != nil {
		return ""
	}
	var pr struct {
		NotDelivered []string `json:"not_delivered"`
	}
	if err := json.Unmarshal(data, &pr); err != nil {
		return ""
	}
	var lines []string
	for _, nd := range pr.NotDelivered {
		lines = append(lines, "FAIL: "+nd)
	}
	return strings.Join(lines, "\n")
}

// extractFrontmatterBody returns the content between the opening and closing ---
// delimiters of a YAML frontmatter block. Returns empty string if no valid
// frontmatter is found.
func extractFrontmatterBody(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "---") {
		return ""
	}
	// Skip the opening ---
	rest := text[3:]
	// Find the closing ---
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return ""
	}
	return rest[:idx]
}

// extractField does a simple key-value extraction from a JSON blob for one
// string field. Returns the value trimmed of surrounding whitespace and quotes.
func extractField(jsonData, key string) string {
	prefix := fmt.Sprintf(`"%s"`, key)
	idx := strings.Index(jsonData, prefix)
	if idx < 0 {
		return ""
	}
	rest := jsonData[idx+len(prefix):]
	// Skip colon and whitespace
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	rest = strings.TrimSpace(rest)
	// Strip surrounding quotes — handle "value" (leading + trailing quote)
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, `"`) {
		rest = rest[1:]
		// Find the closing quote (before next delimiter)
		if end := strings.Index(rest, `"`); end >= 0 {
			rest = rest[:end]
		}
	} else {
		// Fallback for unquoted values: stop at comma, newline, or closing brace
		end := strings.IndexAny(rest, ",\n}")
		if end >= 0 {
			rest = rest[:end]
		}
	}
	return strings.TrimSpace(rest)
}

// runDiff runs git diff in the given worktree. On any error (non-zero exit,
// missing worktree, etc.) it returns diff="" and a descriptive diff_note.
// This is the Pin 1 safe-wrapping implementation.
func runDiff(worktreePath, startCommit string) (diff, note string) {
	if worktreePath == "" {
		return "", "worktree path is empty — cannot run diff"
	}
	if startCommit == "" {
		return "", "start_commit is empty — cannot compute diff"
	}

	var stderr bytes.Buffer
	cmd := exec.Command("git", "diff", startCommit+"..HEAD")
	cmd.Dir = worktreePath
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Sprintf("unavailable: %s", reason)
	}

	diff = strings.TrimSpace(string(out))
	if diff == "" {
		return "", "no changes since start_commit"
	}
	return diff, ""
}
