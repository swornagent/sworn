// Package board validates the structural integrity of release-board index.md
// files — the YAML frontmatter that the loop, the board reader, and every
// frontmatter-aware tool depend on.
//
// The checks are deliberately textual rather than a YAML unmarshal: the failure
// modes that actually break the board are ones a lenient YAML parser silently
// accepts (a key hidden after a `#` comment with no newline) or a strict one
// rejects with an unhelpful message (a closing `---` grafted onto a value line).
// Catching them at the text layer, with a human-readable explanation, is the
// point. Pure stdlib — no third-party dependency, consistent with the rest of
// the binary.
package board

import (
	"regexp"
	"strings"
)

var (
	// frontmatter delimited by `---` on its own line at the top of the file.
	reStrictClose = regexp.MustCompile(`(?s)\A---[\t ]*\r?\n(.*?)\r?\n---[\t ]*(?:\r?\n|\z)`)
	// a `#` comment running directly into the next `key:` with no newline, e.g.
	//   release_index: 6 # bumpedrelease_worktree_path: /path
	// the second key is invisible to frontmatter readers.
	reConcatKey = regexp.MustCompile(`#[ \t]*[a-z_]\w*:`)
	reTracksKey = regexp.MustCompile(`(?m)^tracks:`)

	reTrackID      = regexp.MustCompile(`^\s*-\s+id:\s*(\S+)`)
	reSlicesInline = regexp.MustCompile(`^\s+slices:\s*\[`)
	reSlicesBlock  = regexp.MustCompile(`^\s+slices:\s*$`)
	reSlicesNull   = regexp.MustCompile(`^\s+slices:\s*null`)
	reBullet       = regexp.MustCompile(`^\s+-\s+\S`)
	reBranch       = regexp.MustCompile(`^\s+worktree_branch:\s*\S`)
	reLegacyBranch = regexp.MustCompile(`\bbranch:\s*\S`) // legacy boards used `branch:`
)

// ValidateIndex reports structural problems with the frontmatter of a
// release-board index file. An empty result means the file is well-formed.
// name is used only as a prefix on each message (typically the file path).
func ValidateIndex(name, text string) []string {
	if !strings.HasPrefix(text, "---") {
		return []string{name + ": missing YAML frontmatter (file must start with ---)"}
	}

	m := reStrictClose.FindStringSubmatch(text)
	if m == nil {
		return []string{name + ": frontmatter closing --- is not on its own line — a blank line before --- is required"}
	}
	body := m[1]
	if strings.TrimSpace(body) == "" {
		return []string{name + ": empty YAML frontmatter"}
	}

	var errs []string
	for _, line := range strings.Split(body, "\n") {
		if hit := reConcatKey.FindString(line); hit != "" {
			trailing := strings.TrimLeft(strings.TrimPrefix(hit, "#"), " \t")
			errs = append(errs, name+": '"+trailing+"' follows a # comment on the same line without a"+
				" newline — the key is invisible to frontmatter readers; split onto its own line")
		}
	}

	// Only release boards carry a tracks: list. Capture index.md files don't,
	// and are not release boards — nothing more to check.
	if !reTracksKey.MatchString(body) {
		return errs
	}

	entries := parseTracks(body)
	if len(entries) == 0 {
		return append(errs, name+": tracks: key present but no track entries found — check YAML list indentation")
	}

	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.id] {
			errs = append(errs, name+": duplicate track id '"+e.id+"'")
		}
		seen[e.id] = true
		if !e.hasSlices {
			errs = append(errs, name+": track '"+e.id+"' has no slices: list — add slice IDs or set to []")
		}
		if !e.hasBranch {
			errs = append(errs, name+": track '"+e.id+"' has no worktree_branch: set — add the branch name")
		}
	}
	return errs
}

type trackEntry struct {
	id        string
	hasSlices bool
	hasBranch bool
}

// parseTracks walks the frontmatter body and collects one entry per `- id:`
// line, recording whether each track declares a slices list and a branch. An
// index (not a pointer) tracks the current entry so a slice re-allocation on
// append can never leave us writing through a stale reference.
func parseTracks(body string) []trackEntry {
	var entries []trackEntry
	cur := -1
	for _, line := range strings.Split(body, "\n") {
		if mm := reTrackID.FindStringSubmatch(line); mm != nil {
			entries = append(entries, trackEntry{id: mm[1]})
			cur = len(entries) - 1
			continue
		}
		if cur < 0 {
			continue
		}
		if reSlicesInline.MatchString(line) || reSlicesBlock.MatchString(line) ||
			reSlicesNull.MatchString(line) || reBullet.MatchString(line) {
			entries[cur].hasSlices = true
		}
		if reBranch.MatchString(line) || reLegacyBranch.MatchString(line) {
			entries[cur].hasBranch = true
		}
	}
	return entries
}
