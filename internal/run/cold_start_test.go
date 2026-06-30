package run

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
)

// TestStripInlineComment covers the YAML inline-comment trap (eval finding 2):
// the unfilled placeholder collapses to "" and an inline note after a real value
// is trimmed, while a '#' embedded in a token is left intact.
func TestStripInlineComment(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/tmp/wt/release", "/tmp/wt/release"},
		{"# set by first /implement-slice in this release", ""},
		{"/tmp/wt  # inline note", "/tmp/wt"},
		{"a#b", "a#b"}, // embedded, not a comment
		{"  spaced  ", "spaced"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripInlineComment(c.in); got != c.want {
			t.Errorf("stripInlineComment(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

// TestExtractReleaseWorktreePath_CommentPlaceholder proves the release-board
// placeholder no longer yields the comment text as a literal worktree path.
func TestExtractReleaseWorktreePath_CommentPlaceholder(t *testing.T) {
	fm := "release: r1\nrelease_worktree_path: # set by first /implement-slice in this release\n"
	if got := extractReleaseWorktreePath(fm); got != "" {
		t.Errorf("placeholder should parse empty, got %q", got)
	}
	fm2 := "release_worktree_path: /tmp/wt/r1  # the worktree\n"
	if got := extractReleaseWorktreePath(fm2); got != "/tmp/wt/r1" {
		t.Errorf("got %q want /tmp/wt/r1", got)
	}
}

// TestRunSlice_ColdStartBootstrapsStartCommit is the reachability test for the
// self-bootstrap (eval finding 7): a freshly-planned slice with empty
// start_commit must self-bootstrap — RunSlice pins start_commit to HEAD and
// advances off planned — rather than hard-erroring ("start_commit not set") as
// it did before, which made an autonomous cold-start impossible.
func TestRunSlice_ColdStartBootstrapsStartCommit(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	// Rewrite the status into the cold-start shape: planned, no start_commit.
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	st.State = state.Planned
	st.StartCommit = ""
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}
	runCmd(t, workspaceRoot, "git", "add", "docs/")
	runCmd(t, workspaceRoot, "git", "commit", "-m", "test: cold planned slice")
	wantHead := strings.TrimSpace(runCmd(t, workspaceRoot, "git", "rev-parse", "HEAD"))

	called := false
	opts := RunSliceOptions{
		EscalationModels: []string{"quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: DefaultImplementTimeout,
		NewAgent: func(modelID string) (agent.Agent, error) {
			if modelID == "fake/verifier" {
				return &passingVerifierAgent{}, nil
			}
			return &markedAgent{called: &called}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return &alwaysPassVerifier{}, nil },
	}

	if err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts); err != nil {
		t.Fatalf("RunSlice cold-start should not error on an empty start_commit: %v", err)
	}

	got, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.StartCommit == "" {
		t.Fatal("start_commit not bootstrapped — cold-start still broken")
	}
	if got.StartCommit != wantHead {
		t.Errorf("start_commit=%q want bootstrapped HEAD %q", got.StartCommit, wantHead)
	}
	if got.State == state.Planned {
		t.Error("state still planned — the planned→in_progress bootstrap did not fire")
	}
}
