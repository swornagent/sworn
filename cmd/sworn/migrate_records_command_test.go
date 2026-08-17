package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func migrateTestProject(t *testing.T) string {
	t.Helper()
	git, err := gitExecutablePath()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main", root},
	} {
		command := exec.Command(git, args...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	write := func(relative string, body []byte) {
		t.Helper()
		target := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("product.txt", []byte("base\n"))
	write(".baton/releases/legacy-rel/plan.md", []byte("legacy plan\n"))
	for _, args := range [][]string{
		{"-C", root, "add", "--", "product.txt", filepath.FromSlash(".baton/releases/legacy-rel/plan.md")},
		{"-C", root, "commit", "--quiet", "-m", "legacy records"},
		{"-C", root, "update-ref", "refs/heads/release-wt/legacy-rel", "HEAD"},
	} {
		command := exec.Command(git, args...)
		command.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return root
}

func gitExecutablePath() (string, error) {
	return exec.LookPath("git")
}

func TestMigrateRecordsRequiresConfirmFlag(t *testing.T) {
	root := migrateTestProject(t)
	var stdout, stderr bytes.Buffer
	code := runMigrateRecords([]string{"--project", root}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("unconfirmed migration returned success")
	}
	if !strings.Contains(stderr.String(), "CONFIRMATION_REQUIRED") {
		t.Fatalf("stderr = %q, want CONFIRMATION_REQUIRED", stderr.String())
	}
}

func TestMigrateRecordsMovesLegacyPlansAndReports(t *testing.T) {
	root := migrateTestProject(t)
	var stdout, stderr bytes.Buffer
	code := runMigrateRecords([]string{"--project", root, "--confirm"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("migration failed: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "legacy-rel") ||
		!strings.Contains(stdout.String(), "Migrated 1 release records") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	git, err := gitExecutablePath()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(git, "-C", root, "show", "HEAD:.sworn/records/legacy-rel/plan.md")
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if output, err := command.CombinedOutput(); err != nil || string(output) != "legacy plan\n" {
		t.Fatalf("relocated plan = %q, err = %v", output, err)
	}
	// Idempotent second run refuses.
	var secondOut, secondErr bytes.Buffer
	if code := runMigrateRecords([]string{"--project", root, "--confirm"}, &secondOut, &secondErr); code == 0 {
		t.Fatal("second migration returned success")
	}
	if !strings.Contains(secondErr.String(), "NOTHING_TO_MIGRATE") {
		t.Fatalf("second stderr = %q, want NOTHING_TO_MIGRATE", secondErr.String())
	}
}
