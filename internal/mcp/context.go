package mcp

import (
	"bytes"
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
	SpecContent     string `json:"spec_content"`
	Violations      string `json:"violations"`
	Diff            string `json:"diff"`
	DiffNote        string `json:"diff_note,omitempty"`
	JournalContent  string `json:"journal_content"`
	WorktreePath    string `json:"worktree_path"`
	StartCommit     string `json:"start_commit"`
	SliceState      string `json:"slice_state"`
}

// AssembleSliceContext reads slice artefacts from the release directory and
// the track worktree, returning the assembled context. repoRoot is the
// absolute path to the repo root (e.g. the release-wt or project root).
//
// Pin 1: git diff errors are caught gracefully — on any non-zero exec exit or
// exec error, diff is set to "" and diff_note carries the reason.
func AssembleSliceContext(release, sliceID, repoRoot string) (*SliceContext, error) {
	sliceDir := filepath.Join(repoRoot, "docs", "release", release, sliceID)

	// 1. Read spec.md
	specData, err := os.ReadFile(filepath.Join(sliceDir, "spec.md"))
	if err != nil {
		return nil, fmt.Errorf("read spec.md: %w", err)
	}

	// 2. Read proof.md for violations
	var violations string
	if proofData, err := os.ReadFile(filepath.Join(sliceDir, "proof.md")); err == nil {
		violations = extractViolations(string(proofData))
	}

	// 3. Read journal.md
	var journalContent string
	if journalData, err := os.ReadFile(filepath.Join(sliceDir, "journal.md")); err == nil {
		journalContent = string(journalData)
	}

	// 4. Find the track containing this slice to get worktree_path
	indexPath := filepath.Join(repoRoot, "docs", "release", release, "index.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read index.md: %w", err)
	}

	frontmatterBody := extractFrontmatterBody(string(indexData))
	tracks := board.ParseTracks(frontmatterBody)

	var worktreePath string
	for _, t := range tracks {
		for _, s := range t.Slices {
			if s == sliceID {
				worktreePath = t.WorktreePath
				break
			}
		}
		if worktreePath != "" {
			break
		}
	}
	// 5. Read status.json for start_commit and current state
	var startCommit, sliceState string
	statusPath := filepath.Join(sliceDir, "status.json")
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

// extractViolations parses violations from proof.md content. It looks for
// "FAIL:" lines at the start and "**Violation <N>:**" section markers.
func extractViolations(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "FAIL:") || strings.HasPrefix(trimmed, "**Violation") {
			lines = append(lines, trimmed)
		}
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