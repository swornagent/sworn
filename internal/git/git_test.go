package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	r := New(dir)
	if err := r.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Pin the initial branch to "main" regardless of the host's
	// init.defaultBranch. A fresh CI runner has no global git config, so
	// `git init` yields "master"; tests that assert branch == "main" (e.g.
	// TestEmptyDirDoesNotTouchCwd) would otherwise fail on the fixture, not
	// on real behaviour. symbolic-ref repoints the unborn HEAD across all
	// git versions.
	if _, err := r.run("symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		t.Fatalf("symbolic-ref HEAD main: %v", err)
	}
	// Configure git user for commits (needed in CI / containers).
	if _, err := r.run("config", "user.email", "test@swornagent.dev"); err != nil {
		t.Fatalf("config user.email: %v", err)
	}
	if _, err := r.run("config", "user.name", "sworn test"); err != nil {
		t.Fatalf("config user.name: %v", err)
	}
	return r
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInit(t *testing.T) {
	r := setupRepo(t)
	// .git directory must exist.
	if _, err := os.Stat(filepath.Join(r.Dir, ".git")); err != nil {
		t.Fatalf(".git missing after init: %v", err)
	}
}

func TestBranchAndCheckout(t *testing.T) {
	r := setupRepo(t)
	// Need at least one commit before HEAD is resolvable.
	writeFile(t, r.Dir, "f.txt", "x")
	r.Stage("f.txt")
	r.Commit("initial")
	initialSHA, _ := r.RevParse("HEAD")

	if err := r.Branch("feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	// Should be on feature branch, same HEAD as before (no new commits).
	head, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	if head != initialSHA {
		t.Errorf("HEAD after branch: want %s, got %s", initialSHA, head)
	}
}
func TestStageAndCommit(t *testing.T) {
	r := setupRepo(t)
	writeFile(t, r.Dir, "README.md", "# test")

	if err := r.Stage("README.md"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := r.Commit("initial commit"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	sha, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	if sha == "" || len(sha) != 40 {
		t.Errorf("rev-parse HEAD: want 40-char SHA, got %q", sha)
	}
}

func TestRevParse(t *testing.T) {
	r := setupRepo(t)
	writeFile(t, r.Dir, "a.txt", "a")
	r.Stage("a.txt")
	r.Commit("first")

	sha1, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	// Second commit.
	writeFile(t, r.Dir, "b.txt", "b")
	r.Stage("b.txt")
	r.Commit("second")

	sha2, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	if sha1 == sha2 {
		t.Error("HEAD SHA should change after a new commit")
	}

	// HEAD~1 should equal the first commit.
	parent, err := r.RevParse("HEAD~1")
	if err != nil {
		t.Fatalf("rev-parse HEAD~1: %v", err)
	}
	if parent != sha1 {
		t.Errorf("HEAD~1: want %s, got %s", sha1, parent)
	}
}

func TestDiffRange(t *testing.T) {
	r := setupRepo(t)

	writeFile(t, r.Dir, "a.txt", "line1\nline2\n")
	r.Stage("a.txt")
	if err := r.Commit("first"); err != nil {
		t.Fatalf("commit first: %v", err)
	}

	startCommit, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	writeFile(t, r.Dir, "b.txt", "new file\n")
	writeFile(t, r.Dir, "a.txt", "line1\nline2 modified\n")
	r.Stage("a.txt", "b.txt")
	if err := r.Commit("second"); err != nil {
		t.Fatalf("commit second: %v", err)
	}

	diff, err := r.DiffRange(startCommit, "HEAD")
	if err != nil {
		t.Fatalf("DiffRange: %v", err)
	}
	if diff == "" {
		t.Fatal("DiffRange: expected non-empty diff")
	}
	if !strings.Contains(diff, "b.txt") {
		t.Error("diff should mention b.txt")
	}
	if !strings.Contains(diff, "a.txt") {
		t.Error("diff should mention a.txt")
	}
}

func TestDiffRangeStat(t *testing.T) {
	r := setupRepo(t)

	writeFile(t, r.Dir, "x.go", "package x\n")
	r.Stage("x.go")
	r.Commit("first")

	startCommit, _ := r.RevParse("HEAD")

	writeFile(t, r.Dir, "y.go", "package y\n")
	r.Stage("y.go")
	r.Commit("second")

	stat, err := r.DiffRangeStat(startCommit, "HEAD")
	if err != nil {
		t.Fatalf("DiffRangeStat: %v", err)
	}
	if !strings.Contains(stat, "y.go") {
		t.Errorf("DiffRangeStat: want y.go, got %q", stat)
	}
	// Should only contain file names, no diff content.
	if strings.Contains(stat, "package") {
		t.Error("DiffRangeStat returned diff content, expected only file names")
	}
}

func TestCommit_AllowEmpty(t *testing.T) {
	r := setupRepo(t)
	// Empty commit (--allow-empty) should succeed — used for state-transition
	// commits where no production files change.
	if err := r.Commit("empty state transition"); err != nil {
		t.Fatalf("empty commit: %v", err)
	}
}

func TestDiffRange_Empty(t *testing.T) {
	r := setupRepo(t)
	writeFile(t, r.Dir, "f.txt", "content")
	r.Stage("f.txt")
	r.Commit("first")

	sha, _ := r.RevParse("HEAD")
	diff, err := r.DiffRange(sha, "HEAD")
	if err != nil {
		t.Fatalf("DiffRange: %v", err)
	}
	if diff != "" {
		t.Errorf("DiffRange base==HEAD: want empty diff, got %q", diff)
	}
}
func TestRunRejectsEmptyDir(t *testing.T) {
	// A Repo with no Dir must fail on every mutating method.
	// (AC1: Checkout, Branch, Commit — these are representative;
	//  the guard is in run() so all 9 methods are covered.)
	r := &Repo{} // zero Dir

	// Checkout
	err := r.Checkout("main")
	if err == nil {
		t.Fatal("Checkout on empty-Dir Repo: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty Repo.Dir") {
		t.Errorf("Checkout error: want mention of 'empty Repo.Dir', got: %v", err)
	}

	// Branch
	err = r.Branch("feature")
	if err == nil {
		t.Fatal("Branch on empty-Dir Repo: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty Repo.Dir") {
		t.Errorf("Branch error: want mention of 'empty Repo.Dir', got: %v", err)
	}

	// Commit (AC1 requires Commit be exercised)
	err = r.Commit("test message")
	if err == nil {
		t.Fatal("Commit on empty-Dir Repo: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty Repo.Dir") {
		t.Errorf("Commit error: want mention of 'empty Repo.Dir', got: %v", err)
	}
}

func TestEmptyDirDoesNotTouchCwd(t *testing.T) {
	// Create a temp git repo and chdir into it. Then call a mutating op on a
	// zero-Dir Repo. Assert: (1) the guard error is returned, (2) the temp
	// repo's HEAD and branch are unchanged — proving the ambient cwd was
	// never touched by the git binary.
	r := setupRepo(t)
	writeFile(t, r.Dir, "marker.txt", "initial content")
	r.Stage("marker.txt")
	r.Commit("initial")
	originalRef, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD before: %v", err)
	}

	// Chdir into the temp repo so we can detect ambient git operations.
	// t.Chdir (Go ≥1.24) restores cwd on test exit and marks the test
	// parallel-unsafe.
	t.Chdir(r.Dir)

	// Call a mutating op on a zero-Dir Repo — should fail with guard error.
	zero := &Repo{}
	err = zero.Checkout("main")
	if err == nil {
		t.Fatal("zero-Dir Checkout: expected guard error, got nil")
	}
	if !strings.Contains(err.Error(), "empty Repo.Dir") {
		t.Errorf("error: want mention of 'empty Repo.Dir', got: %v", err)
	}

	// Verify the temp repo was NOT touched — HEAD unchanged.
	currentRef, err := r.RevParse("HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD after: %v", err)
	}
	if currentRef != originalRef {
		t.Errorf("HEAD changed from %s to %s — the ambient repo was mutated", originalRef, currentRef)
	}

	// Verify the branch is still the original (no checkout).
	branchOut, err := r.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse --abbrev-ref HEAD: %v", err)
	}
	if strings.TrimSpace(branchOut) != "main" {
		t.Errorf("branch changed to %q after zero-Dir operation", strings.TrimSpace(branchOut))
	}
}

func TestMerge(t *testing.T) {
	r := setupRepo(t)
	writeFile(t, r.Dir, "f.txt", "base")
	r.Stage("f.txt")
	r.Commit("initial on main")
	initialSHA, _ := r.RevParse("HEAD")

	// Create feature branch, make a change, then merge back.
	if err := r.Branch("feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	writeFile(t, r.Dir, "f.txt", "feature change")
	r.Stage("f.txt")
	r.Commit("change on feature")

	// Switch back to the original branch.
	if err := r.Checkout("-"); err != nil {
		t.Fatalf("checkout -: %v", err)
	}
	backSHA, _ := r.RevParse("HEAD")
	if backSHA != initialSHA {
		t.Fatalf("after checkout -, want %s, got %s", initialSHA, backSHA)
	}

	// Merge feature into current branch.
	if err := r.Merge("feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// After merge, HEAD should be a merge commit (different from initial).
	mergedSHA, _ := r.RevParse("HEAD")
	if mergedSHA == initialSHA {
		t.Fatal("HEAD unchanged after merge")
	}
}

func TestShow(t *testing.T) {
	r := setupRepo(t)
	path := filepath.Join(r.Dir, "docs", "release", "r", "S01-task")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, path, "status.json", `{"slice_id":"S01-task","state":"planned"}`)
	r.Stage("docs/release/r/S01-task/status.json")
	r.Commit("add status.json")

	content, err := r.Show("HEAD", "docs/release/r/S01-task/status.json")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !strings.Contains(content, `"slice_id":"S01-task"`) {
		t.Errorf("Show: unexpected content: %s", content)
	}
}

func TestShow_RejectsEmptyDir(t *testing.T) {
	zero := &Repo{}
	_, err := zero.Show("HEAD", "any/path")
	if err == nil {
		t.Fatal("zero-Dir Show: expected guard error, got nil")
	}
	if !strings.Contains(err.Error(), "empty Repo.Dir") {
		t.Errorf("error: want mention of 'empty Repo.Dir', got: %v", err)
	}
}

func TestCatFileExists(t *testing.T) {
	r := setupRepo(t)
	path := filepath.Join(r.Dir, "docs", "release", "r", "S01-task")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, path, "status.json", `{"slice_id":"S01-task","state":"planned"}`)
	r.Stage("docs/release/r/S01-task/status.json")
	r.Commit("add status.json")

	exists, err := r.CatFileExists("HEAD", "docs/release/r/S01-task/status.json")
	if err != nil {
		t.Fatalf("CatFileExists: %v", err)
	}
	if !exists {
		t.Error("CatFileExists: expected true for committed file")
	}

	// Non-existent path should return false, not error.
	exists, err = r.CatFileExists("HEAD", "docs/release/r/S99-nonexistent/status.json")
	if err != nil {
		t.Fatalf("CatFileExists non-existent: %v", err)
	}
	if exists {
		t.Error("CatFileExists: expected false for non-existent path")
	}
}

func TestCatFileExists_RejectsEmptyDir(t *testing.T) {
	zero := &Repo{}
	_, err := zero.CatFileExists("HEAD", "any/path")
	if err == nil {
		t.Fatal("zero-Dir CatFileExists: expected guard error, got nil")
	}
	if !strings.Contains(err.Error(), "empty Repo.Dir") {
		t.Errorf("error: want mention of 'empty Repo.Dir', got: %v", err)
	}
}

func TestRepoListRefsReadOnly(t *testing.T) {
	r := setupRepo(t)
	if err := os.WriteFile(filepath.Join(r.Dir, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Stage("x"); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.run("branch", "z"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.run("branch", "a"); err != nil {
		t.Fatal(err)
	}
	before, _ := r.StatusPorcelain()
	refs, err := r.ListRefs()
	if err != nil {
		t.Fatal(err)
	}
	after, _ := r.StatusPorcelain()
	want := []string{"refs/heads/a", "refs/heads/main", "refs/heads/z"}
	if strings.Join(refs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("refs=%v want=%v", refs, want)
	}
	if before != after {
		t.Fatalf("status changed: %q -> %q", before, after)
	}
}
