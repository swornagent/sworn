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

	// 2. set_track — S04-mcp-oracle-migration: write through board.Oracle
	// (board.json, with lazy index.md migration) instead of mutating
	// index.md frontmatter directly. The board.json shape is what every
	// other consumer (TUI, MCP ops, merge gate) reads; the previous
	// implementation rewrote the frontmatter in a stale format and
	// silently wiped track data on a plan-mutation call.
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

		// Validate slices exist before mutating board.json.
		for _, sliceID := range p.Slices {
			sliceDir := filepath.Join(repoRoot, "docs", "release", p.Release, sliceID)
			if _, err := os.Stat(sliceDir); err != nil {
				return nil, fmt.Errorf("slice %q does not exist under release %q", sliceID, p.Release)
			}
		}

		br, err := board.ReadBoard(repoRoot, p.Release)
		if err != nil {
			return nil, fmt.Errorf("read board: %w", err)
		}

		found := false
		for i, t := range br.Tracks {
			if t.ID == p.TrackID {
				br.Tracks[i].Slices = p.Slices
				if p.DependsOn != "" {
					br.Tracks[i].DependsOn = board.StringList{p.DependsOn}
				} else {
					br.Tracks[i].DependsOn = nil
				}
				found = true
				break
			}
		}
		if !found {
			// board-v1 is a pure plan: the worktree branch (track/<release>/<track-id>)
			// and state are DERIVED on read (board.TrackWorktreeBranch / the oracle),
			// never persisted here (sworn#80).
			newTrack := board.BoardTrack{
				ID:     p.TrackID,
				Slices: p.Slices,
			}
			if p.DependsOn != "" {
				newTrack.DependsOn = board.StringList{p.DependsOn}
			}
			br.Tracks = append(br.Tracks, newTrack)
		}

		if err := board.WriteBoard(repoRoot, p.Release, br); err != nil {
			return nil, fmt.Errorf("write board: %w", err)
		}

		return &ToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: fmt.Sprintf("Track %q updated in release %s (board.json).", p.TrackID, p.Release),
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
