// Package board validates the structural integrity of release-board index.md
// files — the YAML frontmatter that the loop, the board reader, and every
// frontmatter-aware tool depend on.
//
// The track.go file extends the board package with structured track parsing
// consumed by the scheduler (S02b), the TUI (S04b), and MCP ops tools (S08b).
// Pure stdlib — zero third-party dependencies, consistent with the rest of
// the binary.
package board

import (
	"regexp"
	"strings"
)

// TrackInfo holds one track entry parsed from a release-board index.md frontmatter.
type TrackInfo struct {
	ID             string
	Slices         []string
	DependsOn      []string // empty when the top-level one is null or absent
	WorktreePath   string
	WorktreeBranch string
	State          string
}

var (
	// depends_on:   with brackets (inline list) immediately following
	reDependsOnListInline  = regexp.MustCompile(`^\s+depends_on\s*:\s*\[`)
	// depends_on:   with a non-bracket non-empty value (single string)
	reDependsOnSingle      = regexp.MustCompile(`^\s+depends_on\s*:\s*(.+)$`)
	// depends_on:   on its own line (block-style list follows)
	reDependsOnBlock       = regexp.MustCompile(`^\s+depends_on\s*:\s*$`)
	// A list item under a track:  - id: ... is handled by reTrackID; this
	// captures non-id list items like  - T1 under depends_on:
	reAnyListItem          = regexp.MustCompile(`^\s+-\s+(\S+)`)
	// worktree_path: <path>
	reTrackWorktreePath    = regexp.MustCompile(`^\s+worktree_path\s*:\s*(.*)$`)
	// worktree_branch: <branch>
	reTrackWorktreeBranch  = regexp.MustCompile(`^\s+worktree_branch\s*:\s*(\S+)`)
	// state: <state>
	reTrackState           = regexp.MustCompile(`^\s+state\s*:\s*(\S+)`)
)// ParseTracks parses the tracks section of a release-board index.md frontmatter
// body (the content between the opening and closing --- delimiters). It returns
// one TrackInfo per `- id:` line, in the order they appear in the frontmatter.
//
// Because the frontmatter is not real YAML (the board validator validates it
// textually to catch structural defects that a lenient YAML parser would miss),
// ParseTracks uses the same line-oriented approach.
func ParseTracks(body string) []TrackInfo {
	var tracks []TrackInfo
	var cur *TrackInfo
	inDependsBlock := false

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		// ---- Track start ----
		if mm := reTrackID.FindStringSubmatch(line); mm != nil {
			if cur != nil {
				tracks = append(tracks, *cur)
			}
			cur = &TrackInfo{ID: mm[1]}
			inDependsBlock = false
			continue
		}
		if cur == nil {
			continue
		}

		// ---- depends_on block-style line (just the key, list items follow) ----
		if reDependsOnBlock.MatchString(line) {
			inDependsBlock = true
			continue
		}

		// ---- depends_on inline list: depends_on: [T1, T3] ----
		if reDependsOnListInline.MatchString(line) {
			inDependsBlock = false
			rest := line
			if idx := strings.Index(rest, "["); idx >= 0 {
				rest = rest[idx+1:]
			}
			if idx := strings.LastIndex(rest, "]"); idx >= 0 {
				rest = rest[:idx]
			}
			for _, s := range strings.Split(rest, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					cur.DependsOn = append(cur.DependsOn, s)
				}
			}
			continue
		}

		// ---- depends_on single: depends_on: T1 ----
		// Must check AFTER the list-inline and block patterns to avoid
		// matching `depends_on: [T1, T3]` or `depends_on:` as a single.
		if mm := reDependsOnSingle.FindStringSubmatch(line); mm != nil {
			val := strings.TrimSpace(mm[1])
			// Only consume if it doesn't start with [ (already handled)
			if val != "" && !strings.HasPrefix(val, "[") && val != "null" {
				cur.DependsOn = append(cur.DependsOn, val)
			}
			inDependsBlock = false
			continue
		}

		// ---- depends_on block-style list items ----
		if inDependsBlock {
			if mm := reAnyListItem.FindStringSubmatch(line); mm != nil {
				val := strings.TrimSpace(mm[1])
				if val != "" && val != "null" {
					cur.DependsOn = append(cur.DependsOn, val)
				}
				continue
			}
			// A non-list-item line ends the block
			if !isTrackPropertyLine(line) {
				inDependsBlock = false
			}
		}

		// ---- slices: inline list ----
		if mm := reSlicesInline.FindStringSubmatch(line); mm != nil {
			rest := line
			if idx := strings.Index(rest, "["); idx >= 0 {
				rest = rest[idx+1:]
			}
			if idx := strings.LastIndex(rest, "]"); idx >= 0 {
				rest = rest[:idx]
			}
			for _, s := range strings.Split(rest, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					cur.Slices = append(cur.Slices, s)
				}
			}
			continue
		}

		// ---- slices: block-style list items ----
		if reSlicesBlock.MatchString(line) {
			continue
		}
		// Don't capture list items that are track IDs — those start new
		// entries. Only capture non-ID list items as slice names.
		if reBullet.MatchString(line) && !reTrackID.MatchString(line) {
			if mm := reAnyListItem.FindStringSubmatch(line); mm != nil {
				cur.Slices = append(cur.Slices, strings.TrimSpace(mm[1]))
			}
			continue
		}
		// ---- worktree_path ----
		if mm := reTrackWorktreePath.FindStringSubmatch(line); mm != nil {
			cur.WorktreePath = strings.TrimSpace(mm[1])
			continue
		}

		// ---- worktree_branch ----
		if mm := reTrackWorktreeBranch.FindStringSubmatch(line); mm != nil {
			cur.WorktreeBranch = strings.TrimSpace(mm[1])
			continue
		}

		// ---- state ----
		if mm := reTrackState.FindStringSubmatch(line); mm != nil {
			cur.State = strings.TrimSpace(mm[1])
			continue
		}
	}

	if cur != nil {
		tracks = append(tracks, *cur)
	}

	return tracks
}

// isTrackPropertyLine returns true if the line looks like a YAML property
// under a track (starts with whitespace followed by a key: value pattern).
// Used to detect the end of block-style list contexts.
func isTrackPropertyLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Empty lines end blocks
	if trimmed == "" {
		return false
	}
	// Lines starting with # are comments — skip them but don't end block
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	// Lines starting with - are list items — part of the block
	if strings.HasPrefix(trimmed, "-") {
		return true
	}
	// Lines matching `key: value` at top-level of track indentation
	if matched, _ := regexp.MatchString(`^[a-z_]\w*\s*:`, trimmed); matched {
		return true
	}
	return false
}

// ParseTrackID extracts the track ID from an index.md track entry line.
// Returns the ID plus whether one was found.
func ParseTrackID(line string) (string, bool) {
	mm := reTrackID.FindStringSubmatch(line)
	if mm != nil {
		return mm[1], true
	}
	return "", false
}