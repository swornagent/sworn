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