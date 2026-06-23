package lint

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupDepTestRepo creates a temporary git repo with a go.mod, commits it,
// and returns the repo dir. The caller can then modify go.mod, commit (or not),
// and call CheckDeps.
func setupDepTestRepo(t *testing.T, plannedFiles []string) (repoDir, sliceDir string) {
	t.Helper()

	repoDir = t.TempDir()
	sliceDir = filepath.Join(repoDir, "docs", "release", "test-release", "S01-test")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatalf("mkdir slice dir: %v", err)
	}

	// Init git repo.
	for _, cmd := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		if err := exec.Command("git", append([]string{"-C", repoDir}, cmd...)...).Run(); err != nil {
			t.Fatalf("git %v: %v", cmd, err)
		}
	}

	// Write initial go.mod and commit.
	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module test\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := exec.Command("git", "-C", repoDir, "add", "go.mod").Run(); err != nil {
		t.Fatalf("git add go.mod: %v", err)
	}
	if err := exec.Command("git", "-C", repoDir, "commit", "-m", "initial").Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Write status.json with the given planned_files.
	statusJSON := `{
  "slice_id": "S01-test",
  "release": "test-release",
  "state": "in_progress",
  "planned_files": [` + plannedFilesJSON(plannedFiles) + `],
  "start_commit": ""
}`
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(statusJSON), 0o644); err != nil {
		t.Fatalf("write status.json: %v", err)
	}

	return repoDir, sliceDir
}

func plannedFilesJSON(files []string) string {
	if len(files) == 0 {
		return ""
	}
	var parts []string
	for _, f := range files {
		parts = append(parts, `"`+f+`"`)
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// commitGoModChange modifies go.mod and commits it, so the diff between
// HEAD~1 and HEAD includes go.mod.
func commitGoModChange(t *testing.T, repoDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module test\n\ngo 1.26\n\nrequire (\n\tgithub.com/foo/bar v1.0.0\n)\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := exec.Command("git", "-C", repoDir, "add", "go.mod").Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := exec.Command("git", "-C", repoDir, "commit", "-m", "add dep").Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func TestDepsUndeclaredFails(t *testing.T) {
	repoDir, sliceDir := setupDepTestRepo(t, []string{"internal/foo.go"})
	commitGoModChange(t, repoDir)

	// Diff against the first commit (HEAD~1) so only the go.mod change shows.
	err := CheckDeps(sliceDir, "HEAD~1")
	if err == nil {
		t.Fatal("expected error for undeclared go.mod change, got nil")
	}
	if !contains(err.Error(), "go.mod") {
		t.Fatalf("error should name go.mod, got: %v", err)
	}
}

func TestDepsDeclaredPasses(t *testing.T) {
	repoDir, sliceDir := setupDepTestRepo(t, []string{"go.mod", "internal/foo.go"})
	commitGoModChange(t, repoDir)

	err := CheckDeps(sliceDir, "HEAD~1")
	if err != nil {
		t.Fatalf("expected nil for declared go.mod change, got: %v", err)
	}
}

func TestDepsNoChangePasses(t *testing.T) {
	_, sliceDir := setupDepTestRepo(t, []string{"internal/foo.go"})

	// No go.mod change between HEAD~1 and HEAD (only one commit).
	// Use HEAD as baseRef — diff is empty.
	err := CheckDeps(sliceDir, "HEAD")
	if err != nil {
		t.Fatalf("expected nil when no dep files changed, got: %v", err)
	}
}
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
