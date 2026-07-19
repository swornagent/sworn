package repo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

var fixedCandidateTime = time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)

func TestBindingRejectsRepositoryRemapAndInvalidTarget(t *testing.T) {
	ctx := context.Background()
	first := newTestRepository(t)
	second := newTestRepository(t)
	binding, err := Discover(ctx, first, "repo-01")
	if err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	runTestGit(t, first, "worktree", "add", "--detach", linked, "HEAD")
	linkedBinding, err := Discover(ctx, linked, "repo-01")
	if err != nil {
		t.Fatal(err)
	}
	if linkedBinding != binding {
		t.Fatalf("linked worktree binding = %#v, want %#v", linkedBinding, binding)
	}
	if _, err := Open(ctx, linked, binding); err != nil {
		t.Fatalf("open linked worktree: %v", err)
	}
	if _, err := Open(ctx, second, binding); err == nil || !strings.Contains(err.Error(), "binding drift") {
		t.Fatalf("Open remapped repository error = %v, want binding drift", err)
	}
	repository, err := Open(ctx, first, binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"main", "HEAD", "refs/tags/main", "refs/heads/../main", "refs/heads/"} {
		if _, err := repository.BindTarget(ctx, ref); err == nil {
			t.Errorf("target ref %q was accepted", ref)
		}
	}
	target, err := repository.BindTarget(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if target.RepositoryID != "repo-01" || !repository.validOID(target.Commit) || !repository.validOID(target.Tree) {
		t.Fatalf("invalid measured target: %#v", target)
	}
}

func TestCaptureCreatesExactCandidateWithoutTouchingSourceIndex(t *testing.T) {
	ctx := context.Background()
	source := newTestRepository(t)
	repository, target := openTestRepository(t, source)
	indexPath := filepath.Join(source, ".git", "index")
	indexBefore := readFile(t, indexPath)

	// A dirty user worktree is neither a candidate input nor a reason to mutate
	// its real index. Materialization comes only from the bound Git object.
	writeFile(t, filepath.Join(source, "README.md"), []byte("dirty user bytes\n"), 0o644)
	workspacePath := filepath.Join(t.TempDir(), "candidate")
	workspace, err := repository.Materialize(ctx, target, workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(workspace.Path, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("materialized workspace exposes .git: %v", err)
	}
	if got := string(readFile(t, filepath.Join(workspace.Path, "README.md"))); got != "base readme\n" {
		t.Fatalf("materialized README = %q, want base bytes", got)
	}

	if err := os.Rename(filepath.Join(workspace.Path, "src", "old.txt"), filepath.Join(workspace.Path, "src", "new.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workspace.Path, "src", "new.txt"), []byte("candidate bytes\n"), 0o644)
	if err := os.Symlink("new.txt", filepath.Join(workspace.Path, "src", "link")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workspace.Path, "scripts", "run.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	writeFile(t, filepath.Join(workspace.Path, "scratch.log"), []byte("ignored\n"), 0o644)
	if err := os.Remove(filepath.Join(workspace.Path, "README.md")); err != nil {
		t.Fatal(err)

	}
	candidate, err := repository.Capture(ctx, workspace, CaptureOptions{
		Scope:     Scope{Include: []string{"."}},
		Timestamp: fixedCandidateTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"README.md", "scripts/run.sh", "src/link", "src/new.txt", "src/old.txt"}
	if !reflect.DeepEqual(candidate.ChangedPaths, wantPaths) {
		t.Fatalf("changed paths = %#v, want %#v", candidate.ChangedPaths, wantPaths)
	}
	if candidate.BaseCommit != target.Commit || candidate.BaseTree != target.Tree || candidate.Commit == target.Commit {
		t.Fatalf("candidate does not bind exact base: %#v", candidate)
	}
	if got := strings.TrimSpace(runTestGit(t, source, "rev-parse", candidate.Commit+"^")); got != target.Commit {
		t.Fatalf("candidate parent = %s, want %s", got, target.Commit)
	}
	if got := strings.TrimSpace(runTestGit(t, source, "rev-parse", candidate.Ref)); got != candidate.Commit {
		t.Fatalf("candidate ref = %s, want %s", got, candidate.Commit)
	}
	if got := runTestGit(t, source, "show", candidate.Commit+":src/new.txt"); got != "candidate bytes\n" {
		t.Fatalf("candidate file = %q", got)
	}
	entries := strings.Fields(runTestGit(t, source, "ls-tree", "-r", "--name-only", candidate.Commit))
	if contains(entries, "scratch.log") {
		t.Fatal("ignored scratch.log became candidate content")
	}
	if !bytes.Equal(readFile(t, indexPath), indexBefore) {
		t.Fatal("candidate capture mutated the source worktree index")
	}

	// A crash or external deletion after commit creation can be reconciled from
	// the exact recorded facts while the object still exists.
	runTestGit(t, source, "update-ref", "-d", candidate.Ref, candidate.Commit)
	if err := repository.EnsureCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runTestGit(t, source, "rev-parse", candidate.Ref)); got != candidate.Commit {
		t.Fatalf("reconciled candidate ref = %s, want %s", got, candidate.Commit)
	}
	runTestGit(t, source, "update-ref", candidate.Ref, candidate.BaseCommit, candidate.Commit)
	if err := repository.EnsureCandidate(ctx, candidate); err == nil || !strings.Contains(err.Error(), "ref collision") {
		t.Fatalf("candidate ref collision error = %v", err)
	}
	tampered := candidate
	tampered.ChangedPaths = []string{"src/new.txt"}
	if err := repository.EnsureCandidate(ctx, tampered); err == nil || !strings.Contains(err.Error(), "changed paths mismatch") {
		t.Fatalf("tampered candidate error = %v", err)
	}
}

func TestCaptureKeepsBaseForEmptyDiff(t *testing.T) {
	ctx := context.Background()
	source := newTestRepository(t)
	repository, target := openTestRepository(t, source)
	workspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "candidate"))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := repository.Capture(ctx, workspace, CaptureOptions{
		Scope:     Scope{Include: []string{"."}},
		Timestamp: fixedCandidateTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Commit != target.Commit || candidate.Tree != target.Tree || len(candidate.ChangedPaths) != 0 {
		t.Fatalf("empty candidate = %#v", candidate)
	}
	if got := strings.TrimSpace(runTestGit(t, source, "rev-parse", candidate.Ref)); got != target.Commit {
		t.Fatalf("empty candidate ref = %s, want base %s", got, target.Commit)
	}
}

func TestCaptureRejectsExcludedAndMovedTargets(t *testing.T) {
	t.Run("excluded path", func(t *testing.T) {
		ctx := context.Background()
		source := newTestRepository(t)
		repository, target := openTestRepository(t, source)
		workspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "candidate"))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(workspace.Path, "src", "allowed.txt"), []byte("allowed\n"), 0o644)
		writeFile(t, filepath.Join(workspace.Path, "src", "private", "secret.txt"), []byte("secret\n"), 0o644)
		_, err = repository.Capture(ctx, workspace, CaptureOptions{
			Scope:     Scope{Include: []string{"src"}, Exclude: []string{"src/private"}},
			Timestamp: fixedCandidateTime,
		})
		var scopeErr *ScopeError
		if !errors.As(err, &scopeErr) || !reflect.DeepEqual(scopeErr.Paths, []string{"src/private/secret.txt"}) {
			t.Fatalf("capture error = %v, want excluded path", err)
		}
		if refs := strings.TrimSpace(runTestGit(t, source, "for-each-ref", "--format=%(refname)", candidateRefPrefix)); refs != "" {
			t.Fatalf("out-of-scope capture retained refs: %s", refs)
		}
	})

	t.Run("target moved", func(t *testing.T) {
		ctx := context.Background()
		source := newTestRepository(t)
		repository, target := openTestRepository(t, source)
		workspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "candidate"))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(workspace.Path, "src", "new.txt"), []byte("candidate\n"), 0o644)
		writeFile(t, filepath.Join(source, "target-moved.txt"), []byte("new target\n"), 0o644)
		commitAll(t, source, "advance target")
		_, err = repository.Capture(ctx, workspace, CaptureOptions{
			Scope:     Scope{Include: []string{"."}},
			Timestamp: fixedCandidateTime,
		})
		if err == nil || !strings.Contains(err.Error(), "target moved") {
			t.Fatalf("capture error = %v, want target moved", err)
		}
	})
}

func TestRepositoryFiltersCannotExecuteDuringMaterializeOrCapture(t *testing.T) {
	ctx := context.Background()
	source := newTestRepository(t)
	marker := filepath.Join(t.TempDir(), "filter-executed")
	command := "/bin/sh -c 'touch " + marker + "; cat'"
	runTestGit(t, source, "config", "filter.evil.clean", command)
	runTestGit(t, source, "config", "filter.evil.smudge", command)
	runTestGit(t, source, "config", "filter.evil.required", "true")
	writeFile(t, filepath.Join(source, ".git", "info", "exclude"), []byte("info-excluded.txt\n"), 0o644)
	globalExcludes := filepath.Join(t.TempDir(), "global-excludes")
	writeFile(t, globalExcludes, []byte("configured-excluded.txt\n"), 0o644)
	runTestGit(t, source, "config", "core.excludesFile", globalExcludes)

	repository, target := openTestRepository(t, source)
	workspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "candidate"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(workspace.Path, "src", "old.txt"), []byte("changed\n"), 0o644)
	writeFile(t, filepath.Join(workspace.Path, "info-excluded.txt"), []byte("candidate\n"), 0o644)
	writeFile(t, filepath.Join(workspace.Path, "configured-excluded.txt"), []byte("candidate\n"), 0o644)
	candidate, err := repository.Capture(ctx, workspace, CaptureOptions{
		Scope:     Scope{Include: []string{"."}},
		Timestamp: fixedCandidateTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"info-excluded.txt", "configured-excluded.txt"} {
		if !contains(append([]string(nil), candidate.ChangedPaths...), path) {
			t.Errorf("repository-local exclude incorrectly hid %q", path)
		}
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository filter executed: %v", err)
	}
}

func TestCaptureRejectsUnrepresentableWorkspacePath(t *testing.T) {
	ctx := context.Background()
	source := newTestRepository(t)
	repository, target := openTestRepository(t, source)
	workspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "candidate"))
	if err != nil {
		t.Fatal(err)
	}
	invalidName := string([]byte{0xff})
	if err := os.WriteFile(filepath.Join(workspace.Path, invalidName), []byte("invalid\n"), 0o644); err != nil {
		t.Skipf("filesystem does not accept invalid UTF-8 names: %v", err)
	}
	_, err = repository.Capture(ctx, workspace, CaptureOptions{
		Scope:     Scope{Include: []string{"."}},
		Timestamp: fixedCandidateTime,
	})
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("capture error = %v, want UTF-8 rejection", err)
	}
}

func openTestRepository(t *testing.T, source string) (*Repository, Target) {
	t.Helper()
	ctx := context.Background()
	binding, err := Discover(ctx, source, "repo-01")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := Open(ctx, source, binding)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repository.BindTarget(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	return repository, target
}

func newTestRepository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "init", "-b", "main")
	runTestGit(t, root, "config", "user.name", "Test Author")
	runTestGit(t, root, "config", "user.email", "test@example.invalid")
	writeFile(t, filepath.Join(root, ".gitignore"), []byte("scratch.log\n"), 0o644)
	writeFile(t, filepath.Join(root, ".gitattributes"), []byte("*.txt filter=evil\n"), 0o644)
	writeFile(t, filepath.Join(root, "README.md"), []byte("base readme\n"), 0o644)
	writeFile(t, filepath.Join(root, "src", "old.txt"), []byte("base bytes\n"), 0o644)
	commitAll(t, root, "base")
	return root
}

func commitAll(t *testing.T, root, message string) {
	t.Helper()
	runTestGit(t, root, "add", "--all")
	runTestGit(t, root, "commit", "-m", message)
}

func runTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func writeFile(t *testing.T, path string, contents []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func contains(values []string, value string) bool {
	sort.Strings(values)
	index := sort.SearchStrings(values, value)
	return index < len(values) && values[index] == value
}
