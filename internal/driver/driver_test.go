package driver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoleSet(t *testing.T) {
	t.Run("membership", func(t *testing.T) {
		s := RoleSet{RoleImplementer: true, RoleVerifier: true}
		if !s.Has(RoleImplementer) {
			t.Error("expected Has(RoleImplementer) true")
		}
		if !s.Has(RoleVerifier) {
			t.Error("expected Has(RoleVerifier) true")
		}
		if s.Has(RoleCaptain) {
			t.Error("expected Has(RoleCaptain) false")
		}
	})

	t.Run("empty set", func(t *testing.T) {
		var s RoleSet
		if s.Has(RoleImplementer) {
			t.Error("expected empty RoleSet to have no members")
		}
		if got := s.String(); got != "(none)" {
			t.Errorf("String() on empty set = %q, want %q", got, "(none)")
		}
	})

	t.Run("String names declared roles in fixed order", func(t *testing.T) {
		s := RoleSet{RoleCaptain: true, RoleImplementer: true, RoleVerifier: true}
		got := s.String()
		want := "implementer,verifier,captain"
		if got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	})

	t.Run("String with a single declared role", func(t *testing.T) {
		s := RoleSet{RoleVerifier: true}
		if got := s.String(); got != "verifier" {
			t.Errorf("String() = %q, want %q", got, "verifier")
		}
	})
}

func TestAssertWorktree(t *testing.T) {
	t.Run("success: plain checkout", func(t *testing.T) {
		repo := t.TempDir()
		runGit(t, repo, "init", "-q")
		if err := AssertWorktree(repo); err != nil {
			t.Errorf("AssertWorktree(%q) = %v, want nil", repo, err)
		}
	})

	t.Run("success: linked worktree (.git is a file, not a directory)", func(t *testing.T) {
		repo := t.TempDir()
		runGit(t, repo, "init", "-q")
		runGit(t, repo, "-c", "user.email=t@t.com", "-c", "user.name=t", "commit", "--allow-empty", "-q", "-m", "init")

		linked := filepath.Join(t.TempDir(), "linked")
		runGit(t, repo, "worktree", "add", "-q", linked)

		gitPath := filepath.Join(linked, ".git")
		info, err := os.Stat(gitPath)
		if err != nil {
			t.Fatalf("stat %q: %v", gitPath, err)
		}
		if info.IsDir() {
			t.Fatalf("test setup invariant broken: %q is a directory, expected a linked-worktree gitdir file", gitPath)
		}

		if err := AssertWorktree(linked); err != nil {
			t.Errorf("AssertWorktree(%q) = %v, want nil (linked worktrees must pass)", linked, err)
		}
	})

	t.Run("failure: path does not exist", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		err := AssertWorktree(missing)
		if err == nil {
			t.Fatal("expected error for missing path, got nil")
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("error %q does not name the path %q", err.Error(), missing)
		}
	})

	t.Run("failure: path is not a directory", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "not-a-dir")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		err := AssertWorktree(file)
		if err == nil {
			t.Fatal("expected error for non-directory path, got nil")
		}
		if !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), "not a directory") {
			t.Errorf("error %q does not name the path/check", err.Error())
		}
	})

	t.Run("failure: directory not inside a git working tree", func(t *testing.T) {
		dir := t.TempDir()
		err := AssertWorktree(dir)
		if err == nil {
			t.Fatal("expected error for non-git directory, got nil")
		}
		if !strings.Contains(err.Error(), dir) {
			t.Errorf("error %q does not name the path %q", err.Error(), dir)
		}
	})
}

// TestCostSourceVocabulary pins the persisted string value of every
// CostSource* constant (S08, honest cost telemetry — sworn#70; amended AC-02,
// Coach decision 2026-07-10, S08 verifier round-1 Gate 3). slice-status-v1 is
// additionalProperties:true, so a typo in one of these strings would
// otherwise be schema-valid and drift silently; this test makes that drift a
// compile-visible + test-visible failure instead. It specifically covers
// CostSourceProviderReported == "provider" — the reserved vocabulary member
// with zero live emission sites — so the reserved value has a contract test
// even though no producer currently emits it.
func TestCostSourceVocabulary(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"CostSourceProviderReported", CostSourceProviderReported, "provider"},
		{"CostSourcePricingTable", CostSourcePricingTable, "pricing-table"},
		{"CostSourceCLI", CostSourceCLI, "cli"},
		{"CostSourceSubscription", CostSourceSubscription, "subscription"},
		{"CostSourceUnknown", CostSourceUnknown, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.value, tc.want)
			}
		})
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
