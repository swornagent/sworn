// Package git wraps git operations needed by the run-loop: init, branch
// create/checkout, stage, commit, rev-parse, and slice diff. It uses os/exec
// against the system git binary — zero runtime dependencies beyond the
// standard library, consistent with the project's dependency policy.
//
// All methods operate within a single repository directory, supplied at
// construction time. The package is not goroutine-safe; the caller (the
// run-loop) owns serialisation.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Repo is a handle to a local git repository.
type Repo struct {
	Dir string // working directory; must be inside an initialised git repo
}

// New returns a Repo rooted at dir. The directory must exist; Init is a
// separate call.
func New(dir string) *Repo {
	return &Repo{Dir: dir}
}

// Init runs `git init` in r.Dir. It is safe to call on an already-initialised
// repo (git init is idempotent).
func (r *Repo) Init() error {
	_, err := r.run("init")
	return err
}

// Config sets a git config key to val in the repository (equivalent to
// `git config <key> <val>`).
func (r *Repo) Config(key, val string) error {
	_, err := r.run("config", key, val)
	return err
}

// Branch creates and checks out a new branch named name.
func (r *Repo) Branch(name string) error {
	_, err := r.run("checkout", "-b", name)
	return err
}

// Checkout switches to branch name.
func (r *Repo) Checkout(name string) error {
	_, err := r.run("checkout", name)
	return err
}

// Stage stages the given paths (equivalent to `git add <paths...>`).
func (r *Repo) Stage(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := r.run(args...)
	return err
}

// Commit creates a commit with the given message. The index must already
// contain staged changes (see Stage). Uses --allow-empty so tests can create
// commits without staging real files.
func (r *Repo) Commit(msg string) error {
	_, err := r.run("commit", "--allow-empty", "-m", msg)
	return err
}

// RevParse returns the full SHA for ref (e.g. "HEAD", "start_commit").
func (r *Repo) RevParse(ref string) (string, error) {
	out, err := r.run("rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DiffRange returns the unified diff for base..head. Both are any
// git-rev-parse-able refs (branch names, SHAs, HEAD, etc.).
func (r *Repo) DiffRange(base, head string) (string, error) {
	return r.run("diff", base+".."+head)
}

// DiffRangeStat returns file names changed in base..head, one per line —
// suitable for populating actual_files in a proof bundle.
func (r *Repo) DiffRangeStat(base, head string) (string, error) {
	return r.run("diff", "--name-only", base+".."+head)
}

// Merge merges branch into the currently checked-out branch with --no-ff.
// The merge message is auto-generated.
func (r *Repo) Merge(branch string) error {
	_, err := r.run("merge", "--no-ff", branch, "-m", "merge: "+branch)
	return err
}

// Show returns the content of <ref>:<path>. Both ref and path are passed
// directly to `git show` — the caller is responsible for assembling the
// colon-separated form (e.g. "HEAD:docs/release/.../status.json").
func (r *Repo) Show(ref, path string) (string, error) {
	return r.run("show", ref+":"+path)
}

// CatFileExists returns true when <ref>:<path> exists in the git object
// database (equivalent to `git cat-file -e <ref>:<path>`). It does not
// inspect the working tree — the check is against the committed tree,
// which avoids the Fumadocs symlink trap (S57 spec, Coach pin 7).
func (r *Repo) CatFileExists(ref, path string) (bool, error) {
	_, err := r.run("cat-file", "-e", ref+":"+path)
	if err != nil {
		// git cat-file -e exits non-zero when the object does not exist
		if strings.Contains(err.Error(), "exists") ||
			strings.Contains(err.Error(), "bad file") ||
			strings.Contains(err.Error(), "Not a valid object name") ||
			strings.Contains(err.Error(), "path not found") ||
			strings.Contains(err.Error(), "fatal:") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// LastCommitTime returns the Unix epoch timestamp (seconds) of the last commit
// that touched path on ref. Returns 0 if the file has never been committed.
// Wraps `git log -1 --format=%ct ref -- path`.
func (r *Repo) LastCommitTime(ref, path string) (int64, error) {
	out, err := r.run("log", "-1", "--format=%ct", ref, "--", path)
	if err != nil {
		// When the file has never been committed or the ref doesn't exist,
		// git log exits non-zero. Treat as 0 (absent).
		if strings.Contains(err.Error(), "fatal:") ||
			strings.Contains(err.Error(), "does not have any commits") ||
			strings.Contains(err.Error(), "unknown revision") ||
			strings.Contains(err.Error(), "bad revision") {
			return 0, nil
		}
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	var ct int64
	if _, err := fmt.Sscanf(out, "%d", &ct); err != nil {
		return 0, fmt.Errorf("git log --format=%%ct: parse %q: %w", out, err)
	}
	return ct, nil
}

// IsAncestor returns true when ancestor is reachable from branch (i.e. branch
// contains ancestor). Wraps `git merge-base --is-ancestor ancestor branch`.
func (r *Repo) IsAncestor(ancestor, branch string) (bool, error) {
	_, err := r.run("merge-base", "--is-ancestor", ancestor, branch)
	if err != nil {
		// git merge-base --is-ancestor exits 0 when ancestor is reachable,
		// 1 when it is not. Both are valid outcomes.
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MergeDryRun runs `git merge --no-commit --no-ff <branch>` and returns
// conflicting file paths when the merge would produce conflicts. On a clean
// merge it returns (nil, nil) — the caller is responsible for calling
// ResetMerge() to undo the dry-run.
//
// This mutates the working tree (process-global mutation — Rule 11). The
// caller must assert the target worktree/branch is the expected one before
// calling, and must call ResetMerge or MergeAbort after.
func (r *Repo) MergeDryRun(branch string) (conflictFiles []string, err error) {
	_, err = r.run("merge", "--no-commit", "--no-ff", branch)
	if err == nil {
		// Clean merge — caller should reset.
		return nil, nil
	}

	// Merge failed — check if it's a conflict.
	if !strings.Contains(err.Error(), "CONFLICT") &&
		!strings.Contains(err.Error(), "Merge conflict") &&
		!strings.Contains(err.Error(), "Automatic merge failed") &&
		!strings.Contains(err.Error(), "exit status 1") {
		return nil, err
	}

	// Gather conflicted files.
	out, listErr := r.run("diff", "--name-only", "--diff-filter=U")
	if listErr != nil {
		return nil, fmt.Errorf("merge conflict but failed to list conflicted files: %w", listErr)
	}

	if out == "" {
		return nil, nil
	}

	for _, f := range strings.Split(out, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			conflictFiles = append(conflictFiles, f)
		}
	}
	return conflictFiles, nil
}

// ResetMerge undoes a dry-run merge: `git reset --merge HEAD`.
// Call after MergeDryRun on a clean merge (no conflicts) to restore the
// working tree.
func (r *Repo) ResetMerge() error {
	_, err := r.run("reset", "--merge", "HEAD")
	return err
}

// MergeAbort aborts an in-progress merge: `git merge --abort`.
// Call after MergeDryRun when conflicts were detected.
func (r *Repo) MergeAbort() error {
	_, err := r.run("merge", "--abort")
	return err
}

// StatusPorcelain returns the output of `git status --porcelain` for the repo.
func (r *Repo) StatusPorcelain() (string, error) {
	return r.run("status", "--porcelain")
}

// CurrentBranch returns the name of the currently checked-out branch.
func (r *Repo) CurrentBranch() (string, error) {
	out, err := r.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}
// run executes a git command in r.Dir and returns stdout (trimmed). On
// non-zero exit it returns stderr as the error.//
// It refuses to run when Dir is empty — executing git in the ambient cwd
// is the root cause of sworn#6 (track workers flipping the calling worktree
// to main). Every mutating method funnels through this single chokepoint,
// so one guard protects all mutation paths.
func (r *Repo) run(args ...string) (string, error) {
	if r.Dir == "" {
		return "", fmt.Errorf("git %s: refusing to run with empty Repo.Dir "+
			"(would operate on the ambient working directory / calling worktree)",
			strings.Join(args, " "))
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
