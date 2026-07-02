package run

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
)

// NOTE: TestStripInlineComment and TestExtractReleaseWorktreePath_CommentPlaceholder
// were removed with S06 — the frontmatter helpers they covered
// (stripInlineComment, extractReleaseWorktreePath) are deleted now that
// RunParallel reads tracks and the release worktree path from board.json via
// board.ReadBoard (the oracle) rather than parsing index.md frontmatter. The
// board package retains its own stripInlineComment for the lazy index.md
// migration path, covered by internal/board's tests.

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
