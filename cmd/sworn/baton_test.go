package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func TestBatonDiffExitsNonZeroOnDivergence(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "..", "internal", "baton", "testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	// Create a .git directory so RepoRoot discovery succeeds.
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create the embed directory structure.
	for _, m := range baton.AllMappings() {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Vendor to populate the embed.
	_, err = baton.Vendor(baton.VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	// Sanity: diff is clean before mutation.
	t.Run("clean_before_mutation", func(t *testing.T) {
		oldDir, _ := os.Getwd()
		if err := os.Chdir(tmpRepo); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(oldDir)

		exit := cmdBatonDiff([]string{fixture})
		if exit != 0 {
			t.Errorf("cmdBatonDiff clean exit = %d, want 0", exit)
		}
	})

	// Hand-edit an embed file.
	ruleFile := filepath.Join(tmpRepo, "internal/adopt/baton/rules/01-reachability-gate.md")
	orig, err := os.ReadFile(ruleFile)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(orig), "sworn verify", "sworn verify (FORKED)", 1)
	if err := os.WriteFile(ruleFile, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	// Diff should exit non-zero and name the file.
	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	exit := cmdBatonDiff([]string{fixture})
	if exit == 0 {
		t.Fatal("cmdBatonDiff after hand-edit exit = 0, want non-zero")
	}

	// Capture output: the command prints to stdout.
	code, out := captureStdout(t, func() int {
		return cmdBatonDiff([]string{fixture})
	})
	if code == 0 {
		t.Fatal("cmdBatonDiff after hand-edit exit = 0, want non-zero")
	}
	if !strings.Contains(out, "internal/adopt/baton/rules/01-reachability-gate.md") {
		t.Errorf("output missing divergent file path, got:\n%s", out)
	}
	if !strings.Contains(out, "content differs") {
		t.Errorf("output missing reason, got:\n%s", out)
	}
}

func TestBatonDiffExitsZeroWhenInSync(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "..", "internal", "baton", "testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, m := range baton.AllMappings() {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	_, err = baton.Vendor(baton.VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	exit := cmdBatonDiff([]string{fixture})
	if exit != 0 {
		t.Errorf("cmdBatonDiff clean exit = %d, want 0", exit)
	}
}