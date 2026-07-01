package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/state"
)

// RegisterPlanTools registers the planning tools.
func RegisterPlanTools(s *Server, repoRoot string) {
	// 1. create_slice
	s.RegisterTool("create_slice", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release": {"type": "string"},
			"slice_id": {"type": "string"},
			"spec_content": {"type": "string"},
			"track_id": {"type": "string"}
		},
		"required": ["release", "slice_id", "spec_content", "track_id"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release     string `json:"release"`
			SliceID     string `json:"slice_id"`
			SpecContent string `json:"spec_content"`
			TrackID     string `json:"track_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		sliceDir := filepath.Join(repoRoot, "docs", "release", p.Release, p.SliceID)
		if _, err := os.Stat(sliceDir); err == nil {
			return nil, fmt.Errorf("slice %q already exists under release %q", p.SliceID, p.Release)
		}

		if err := os.MkdirAll(sliceDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create slice directory: %w", err)
		}

		specPath := filepath.Join(sliceDir, "spec.md")
		if err := os.WriteFile(specPath, []byte(p.SpecContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to write spec.md: %w", err)
		}

		statusPath := filepath.Join(sliceDir, "status.json")
		sObj := state.Status{
			Schema:                "https://example.com/schemas/baton/slice-status-v1.json",
			SliceID:               p.SliceID,
			Release:               p.Release,
			Track:                 p.TrackID,
			State:                 "planned",
			Owner:                 "human",
			LastUpdatedBy:         "create_slice",
			LastUpdatedAt:         time.Now().UTC().Format(time.RFC3339),
			SpecPath:              filepath.Join("docs", "release", p.Release, p.SliceID, "spec.md"),
			ProofPath:             filepath.Join("docs", "release", p.Release, p.SliceID, "proof.md"),
			JournalPath:           filepath.Join("docs", "release", p.Release, p.SliceID, "journal.md"),
			PlannedFiles:          []string{},
			ActualFiles:           []string{},
			TestCommands:          []string{},
			ReachabilityArtifacts: []string{},
			OpenDeferrals:         []state.Deferral{},
			Verification: state.Verification{
				Result: "pending",
			},
		}
		if err := state.Write(statusPath, &sObj); err != nil {
			return nil, fmt.Errorf("failed to write status.json: %w", err)
		}

		return &ToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: fmt.Sprintf("Created slice %s under release %s.\nPaths:\n- %s\n- %s", p.SliceID, p.Release, specPath, statusPath),
				},
			},
		}, nil
	})

	// 2. set_track
	s.RegisterTool("set_track", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release": {"type": "string"},
			"track_id": {"type": "string"},
			"slices": {
				"type": "array",
				"items": {"type": "string"}
			},
			"depends_on": {"type": "string"}
		},
		"required": ["release", "track_id", "slices"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release   string   `json:"release"`
			TrackID   string   `json:"track_id"`
			Slices    []string `json:"slices"`
			DependsOn string   `json:"depends_on,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		// Validate slices exist
		for _, sliceID := range p.Slices {
			sliceDir := filepath.Join(repoRoot, "docs", "release", p.Release, sliceID)
			if _, err := os.Stat(sliceDir); err != nil {
				return nil, fmt.Errorf("slice %q does not exist under release %q", sliceID, p.Release)
			}
		}

		indexPath := filepath.Join(repoRoot, "docs", "release", p.Release, "index.md")
		indexData, err := os.ReadFile(indexPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read index.md: %w", err)
		}

		content := string(indexData)
		parts := strings.SplitN(content, "---", 3)
		if len(parts) < 3 {
			return nil, fmt.Errorf("malformed index.md: missing frontmatter delimiters")
		}

		frontmatter := parts[1]
		body := parts[2]

		// Parse existing tracks
		existingTracks := board.ParseTracks(frontmatter)
		found := false
		for i, t := range existingTracks {
			if t.ID == p.TrackID {
				existingTracks[i].Slices = p.Slices
				if p.DependsOn != "" {
					existingTracks[i].DependsOn = []string{p.DependsOn}
				} else {
					existingTracks[i].DependsOn = nil
				}
				found = true
				break
			}
		}

		if !found {
			newTrack := board.TrackInfo{
				ID:             p.TrackID,
				Slices:         p.Slices,
				WorktreeBranch: fmt.Sprintf("track/%s/%s", p.Release, p.TrackID),
				State:          "planned",
			}
			if p.DependsOn != "" {
				newTrack.DependsOn = []string{p.DependsOn}
			}
			existingTracks = append(existingTracks, newTrack)
		}

		// Regenerate tracks frontmatter block
		var tb strings.Builder
		tb.WriteString("tracks:\n")
		if len(existingTracks) == 0 {
			tb.WriteString("  []\n")
		} else {
			for _, t := range existingTracks {
				tb.WriteString(fmt.Sprintf("  - id: %s\n", t.ID))
				tb.WriteString(fmt.Sprintf("    slices: [%s]\n", strings.Join(t.Slices, ", ")))
				if len(t.DependsOn) == 0 {
					tb.WriteString("    depends_on: null\n")
				} else if len(t.DependsOn) == 1 {
					tb.WriteString(fmt.Sprintf("    depends_on: %s\n", t.DependsOn[0]))
				} else {
					tb.WriteString(fmt.Sprintf("    depends_on: [%s]\n", strings.Join(t.DependsOn, ", ")))
				}
				if t.WorktreePath == "" || t.WorktreePath == "null" {
					tb.WriteString("    worktree_path: null\n")
				} else {
					tb.WriteString(fmt.Sprintf("    worktree_path: %s\n", t.WorktreePath))
				}
				tb.WriteString(fmt.Sprintf("    worktree_branch: %s\n", t.WorktreeBranch))
				tb.WriteString(fmt.Sprintf("    state: %s\n", t.State))
			}
		}

		// Filter out old tracks block from frontmatter
		var newFM []string
		inTracks := false
		for _, line := range strings.Split(frontmatter, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "tracks:") {
				inTracks = true
				continue
			}
			if inTracks {
				if trimmed != "" && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") && strings.Contains(line, ":") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
					inTracks = false
				} else {
					continue
				}
			}
			newFM = append(newFM, line)
		}

		// Append new tracks block
		newFMStr := strings.Join(newFM, "\n")
		if !strings.HasSuffix(newFMStr, "\n") {
			newFMStr += "\n"
		}
		newFMStr += tb.String()

		// Rewrite Tracks table in body
		bodyLines := strings.Split(body, "\n")
		var newBody []string
		inTable := false

		for i := 0; i < len(bodyLines); i++ {
			line := bodyLines[i]
			if strings.Contains(line, "| Track | Slices (in order) |") {
				inTable = true
				newBody = append(newBody, "| Track | Slices (in order) | Depends on | Branch | State |")
				newBody = append(newBody, "|---|---|---|---|---|")
				for _, t := range existingTracks {
					slicesStr := strings.Join(t.Slices, " → ")
					depStr := "—"
					if len(t.DependsOn) > 0 {
						depStr = strings.Join(t.DependsOn, ", ")
					}
					newBody = append(newBody, fmt.Sprintf("| `%s` | %s | %s | `%s` | %s |", t.ID, slicesStr, depStr, t.WorktreeBranch, t.State))
				}
				i++ // skip separator line
				continue
			}
			if inTable {
				if strings.HasPrefix(strings.TrimSpace(line), "|") {
					continue
				} else {
					inTable = false
				}
			}
			newBody = append(newBody, line)
		}

		newContent := fmt.Sprintf("---%s---%s", newFMStr, strings.Join(newBody, "\n"))
		if err := os.WriteFile(indexPath, []byte(newContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to write index.md: %w", err)
		}

		return &ToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: newFMStr,
				},
			},
		}, nil
	})

	// 3. update_intake
	s.RegisterTool("update_intake", json.RawMessage(`{
		"type": "object",
		"properties": {
			"release": {"type": "string"},
			"section": {"type": "string"},
			"content": {"type": "string"}
		},
		"required": ["release", "section", "content"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Release string `json:"release"`
			Section string `json:"section"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		intakePath := filepath.Join(repoRoot, "docs", "release", p.Release, "intake.md")
		intakeData, err := os.ReadFile(intakePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read intake.md: %w", err)
		}

		lines := strings.Split(string(intakeData), "\n")
		var newLines []string
		found := false

		for i := 0; i < len(lines); i++ {
			line := lines[i]
			newLines = append(newLines, line)

			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## ") {
				heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
				if strings.EqualFold(heading, p.Section) {
					found = true
					insertIdx := i + 1
					for j := i + 1; j < len(lines); j++ {
						nextTrimmed := strings.TrimSpace(lines[j])
						if strings.HasPrefix(nextTrimmed, "#") {
							break
						}
						insertIdx = j + 1
					}

					var sectionLines []string
					for k := i + 1; k < insertIdx; k++ {
						sectionLines = append(sectionLines, lines[k])
					}
					sectionLines = append(sectionLines, "", p.Content)

					newLines = append(newLines, sectionLines...)
					i = insertIdx - 1
				}
			}
		}

		if !found {
			newLines = append(newLines, "", "## "+p.Section, "", p.Content)
		}

		newContent := strings.Join(newLines, "\n")
		if err := os.WriteFile(intakePath, []byte(newContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to write intake.md: %w", err)
		}

		return &ToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: p.Section,
				},
			},
		}, nil
	})
}

// CreateRelease creates a new release directory structure.
func CreateRelease(repoRoot, name, goal, trackingIssue string) (map[string]string, error) {
	releaseDir := filepath.Join(repoRoot, "docs", "release", name)
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create release directory: %w", err)
	}

	screenshotsDir := filepath.Join(releaseDir, "screenshots")
	if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create screenshots directory: %w", err)
	}

	gitkeepPath := filepath.Join(screenshotsDir, ".gitkeep")
	if err := os.WriteFile(gitkeepPath, []byte(""), 0644); err != nil {
		return nil, fmt.Errorf("failed to write .gitkeep: %w", err)
	}

	// Copy .gitattributes
	gitattributesPath := filepath.Join(releaseDir, ".gitattributes")
	gitattributesContent := `# Append-only release logs: union-merge so concurrent appends from parallel
# tracks never conflict. ` + "`" + `union` + "`" + ` is a built-in git merge driver — it keeps both
# sides' added lines instead of raising a conflict. No .git/config setup needed.
#
# These files carry independent per-track narrative/log data (NOT state derived
# from status.json), so union is exactly right: every track's appended lines
# survive the merge.
activity.md            merge=union
.captain-trial-log.md  merge=union
`
	if err := os.WriteFile(gitattributesPath, []byte(gitattributesContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write .gitattributes: %w", err)
	}

	// Copy activity.md
	activityPath := filepath.Join(releaseDir, "activity.md")
	activityContent := strings.ReplaceAll(`# Recent activity — `+name+`

> Append-only transition log for this release. Configured BACKTICKmerge=unionBACKTICK in BACKTICK.gitattributesBACKTICK, so when parallel tracks append concurrently the two sides' new lines are both kept — no manual conflict resolution. This file replaces the old BACKTICKindex.mdBACKTICK "Recent activity" section.
>
> Live per-slice state is **not** here — it lives in each slice's BACKTICKstatus.jsonBACKTICK (canonical) and is rendered by BACKTICKcoach boardBACKTICK / BACKTICKrelease-board-status.sh --jsonBACKTICK. This file is the human-readable narrative of how the release progressed.
>
> Append one line per transition at the **end** of the file. Format:
> BACKTICK- <ISO-8601 timestamp> — <slice-id> (<track-id>): <old-state> → <new-state>. <one-line note>.BACKTICK
> Track merges: BACKTICK- <ISO timestamp> — track <track-id> merged to release-wt/ (<commit>). <N> verified slice(s): <ids>.BACKTICK

- _(activity entries appended below by implementer / verifier / merge steps)_
`, "BACKTICK", "`")
	if err := os.WriteFile(activityPath, []byte(activityContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write activity.md: %w", err)
	}
	// Copy index.md
	indexPath := filepath.Join(releaseDir, "index.md")
	releaseIndex := getNextReleaseIndex(repoRoot)
	indexContent := `---
title: 'Release board: ` + name + `'
description: 'The release board — the single source of truth for slice states and track grouping across a release.'
release_worktree_path: null
release_worktree_branch: release-wt/` + name + `
release_index: ` + fmt.Sprintf("%d", releaseIndex) + `
tracks: []
---

# Release Board: ` + name + `

## Release summary

- **Goal**: ` + goal + `
- **Target version / integration branch**: release/v0.1.0
- **Started**: ` + time.Now().Format("2006-01-02") + `
- **Target ship**: uncommitted
- **Intake**: intake.md
- **Stakeholder**: human
- **Tracking issue**: ` + trackingIssue + `

## Tracks

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|

### Touchpoint matrix

| File / surface |

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|

### State legend

` + "`" + `planned` + "`" + ` → ` + "`" + `in_progress` + "`" + ` → ` + "`" + `implemented` + "`" + ` → ` + "`" + `verified` + "`" + ` → (` + "`" + `/merge-track` + "`" + ` →
` + "`" + `/merge-release` + "`" + `) → ` + "`" + `shipped` + "`" + `. ` + "`" + `failed_verification` + "`" + ` returns to the implementer.
A slice may also be moved to the ` + "`" + `defe` + `rred` + "`" + ` state (carved out per Rule 2) by the human.
Live state lives in each slice's ` + "`" + `status.json` + "`" + ` (not mirrored here).

## Aggregate state
- Planned: 0
- In progress: 0
- Implemented (awaiting verification): 0
- Verified (awaiting merge): 0
- Failed verification: 0
- Deferred: 0
- Shipped: 0

**Tracks:** Planned: 0 / In progress: 0 / Merged: 0
`
	if err := os.WriteFile(indexPath, []byte(indexContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write index.md: %w", err)
	}

	// Copy intake.md
	intakePath := filepath.Join(releaseDir, "intake.md")
	intakeContent := `---
title: 'Release intake: ` + name + `'
description: 'The discovery output document.'
---

# Release Intake: ` + name + `

## Release goal

` + goal + `

## Source of truth

- **Human stakeholder**: human
- **Tracking issue / epic**: ` + trackingIssue + `

## Users and their gestures

- **Anonymous visitor**: ...

## What's currently broken or missing

- ...

## What the human wants

- ...

## Constraints and non-negotiables

- ...

## Adjacent / out of scope

- ...

## Decisions made during planning

- ...

## Schema-vs-spec audit notes

- ...

## Proposed slice decomposition (draft)

- ...

## Open questions

- ...

## Screenshots / references

- ...
`
	if err := os.WriteFile(intakePath, []byte(intakeContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write intake.md: %w", err)
	}

	return map[string]string{
		"dir":        releaseDir,
		"index":      indexPath,
		"intake":     intakePath,
		"activity":   activityPath,
		"attributes": gitattributesPath,
	}, nil
}

func getNextReleaseIndex(repoRoot string) int {
	matches, err := filepath.Glob(filepath.Join(repoRoot, "docs", "release", "*", "index.md"))
	if err != nil {
		return 1
	}
	maxIndex := 0
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "release_index:") {
				valStr := strings.TrimSpace(strings.TrimPrefix(line, "release_index:"))
				var val int
				if _, err := fmt.Sscanf(valStr, "%d", &val); err == nil {
					if val > maxIndex {
						maxIndex = val
					}
				}
			}
		}
	}
	return maxIndex + 1
}
