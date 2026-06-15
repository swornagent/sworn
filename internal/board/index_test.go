package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateIndex(t *testing.T) {
	const goodBoard = `---
title: Release board
release_index: 1
tracks:
  - id: T1-engine
    slices: [S01-core, S02-client]
    worktree_branch: track/x/T1-engine
  - id: T2-ux
    slices: [S03-init]
    worktree_branch: track/x/T2-ux
---

# Board
`
	tests := []struct {
		name      string
		text      string
		wantClean bool
		wantHint  string // substring expected in the first problem when not clean
	}{
		{name: "well-formed release board", text: goodBoard, wantClean: true},
		{
			name:      "capture index (no tracks) is fine",
			text:      "---\ntitle: Capture\ndescription: notes\n---\n\nbody\n",
			wantClean: true,
		},
		{
			name:     "missing frontmatter",
			text:     "# just a heading\n",
			wantHint: "missing YAML frontmatter",
		},
		{
			name:     "closing --- grafted onto a value line",
			text:     "---\ntitle: x\nrelease_index: 1---\nbody\n",
			wantHint: "closing --- is not on its own line",
		},
		{
			name:     "key hidden after a # comment",
			text:     "---\nrelease_index: 6 # bumpedrelease_worktree_path: /p\ntracks:\n  - id: T1\n    slices: []\n    worktree_branch: b\n---\n",
			wantHint: "follows a # comment",
		},
		{
			name:     "tracks present but no entries",
			text:     "---\ntracks:\n  garbage\n---\n",
			wantHint: "no track entries found",
		},
		{
			name:     "track missing slices",
			text:     "---\ntracks:\n  - id: T1\n    worktree_branch: b\n---\n",
			wantHint: "has no slices",
		},
		{
			name:     "track missing branch",
			text:     "---\ntracks:\n  - id: T1\n    slices: []\n---\n",
			wantHint: "has no worktree_branch",
		},
		{
			name:     "duplicate track id",
			text:     "---\ntracks:\n  - id: T1\n    slices: []\n    worktree_branch: b\n  - id: T1\n    slices: []\n    worktree_branch: c\n---\n",
			wantHint: "duplicate track id",
		},
		{
			name:      "block-style slices and legacy branch: pass",
			text:      "---\ntracks:\n  - id: T1\n    slices:\n      - S01\n      - S02\n    branch: legacy/T1\n---\n",
			wantClean: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateIndex(tc.name, tc.text)
			if tc.wantClean {
				if len(got) != 0 {
					t.Fatalf("expected no problems, got: %v", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("expected a problem containing %q, got none", tc.wantHint)
			}
			if !strings.Contains(got[0], tc.wantHint) {
				t.Fatalf("problem %q does not contain %q", got[0], tc.wantHint)
			}
		})
	}
}

// TestLiveReleaseBoardsAreValid is the regression guard: every committed
// release-board index.md must pass the same checks, so a malformed board (or a
// bad hand-edit) fails CI via `go test ./...` rather than the loop at runtime.
func TestLiveReleaseBoardsAreValid(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "docs", "release", "*", "index.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no release boards found")
	}
	for _, path := range matches {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if problems := ValidateIndex(path, string(text)); len(problems) != 0 {
			t.Errorf("%s is malformed:\n  %s", path, strings.Join(problems, "\n  "))
		}
	}
}
