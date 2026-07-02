package driver

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AssertWorktree is the Rule-11 fail-closed target assertion available to
// every driver before it spawns work rooted at a WorktreeRoot: it names
// exactly which check failed rather than letting a bad path surface as a
// confusing downstream error once work is already underway.
//
// It deliberately shells out to `git rev-parse --is-inside-work-tree`
// instead of checking for a `.git` directory. In this project (and any repo
// using `git worktree add`), a linked worktree's `.git` is a plain file
// containing a `gitdir:` pointer back to the main repository's
// .git/worktrees/<name> — a directory-presence check would fail-closed on
// every worktree the project actually uses, which is the opposite of the
// intended fail-closed behaviour (rejecting a path that is NOT a working
// tree, not one that is).
func AssertWorktree(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("driver: AssertWorktree(%q): path does not exist: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("driver: AssertWorktree(%q): path is not a directory", path)
	}

	out, err := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree").CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return fmt.Errorf("driver: AssertWorktree(%q): not inside a git working tree: %w (%s)", path, err, trimmed)
	}
	if trimmed != "true" {
		return fmt.Errorf("driver: AssertWorktree(%q): git rev-parse --is-inside-work-tree returned %q, want \"true\"", path, trimmed)
	}
	return nil
}
