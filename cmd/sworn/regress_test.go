package main

import (
	"os"
	"testing"

	"github.com/swornagent/sworn/internal/command"
)

// TestRegressCommandRegistered is the reachability check: `sworn regress` must
// be wired into the command registry, else the merge-track gate can't call it.
func TestRegressCommandRegistered(t *testing.T) {
	c, ok := command.Lookup("regress")
	if !ok {
		t.Fatal("command.Lookup(\"regress\") not found — regress is not registered")
	}
	if c.Run == nil {
		t.Error("regress Run must be non-nil")
	}
}

// TestRegressWorktreeFlagFailsClosed verifies the --worktree override (used by
// the merge-track gate to run the affected-package suite in a TRACK worktree):
// a missing/non-directory worktree must fail closed (exit 2) rather than run the
// suite against the wrong place. It also proves the override bypasses index.md
// resolution — the test runs from a temp dir with no docs/release tree at all,
// so reaching the worktree-stat guard at all means index.md was skipped.
func TestRegressWorktreeFlagFailsClosed(t *testing.T) {
	origDir, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdRegress([]string{"--release", "2026-01-01-x", "--worktree", "/no/such/worktree/path"})
	if exit != 2 {
		t.Fatalf("expected fail-closed exit 2 for nonexistent --worktree, got %d", exit)
	}
}
