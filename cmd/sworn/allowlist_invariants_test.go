package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllowlistInvariantsKeepRecordsTrackedAndRunStateIgnored is the
// executed-suite proof of the allowlist-shaped ignore rules (A5). It is
// state-independent and fixture-based: it builds a fresh repository in the
// exact post-migration shape (records under .sworn/records, authored plan and
// contracts under docs/sworn/, committed docs/sworn/sworn.json, no .baton,
// no journals or working files) and asserts what git actually tracks.
func TestAllowlistInvariantsKeepRecordsTrackedAndRunStateIgnored(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	if output, err := exec.Command(git, "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	runFixtureGit := func(args ...string) string {
		t.Helper()
		command := exec.Command(git, append([]string{"-C", root}, args...)...)
		command.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}

	// The committed product files under test, read from this repository's own
	// tree so drift between the rules and this assertion is caught.
	moduleRoot := filepath.Join("..", "..")
	readProduct := func(relative string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(moduleRoot, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		return string(body)
	}
	rootIgnore := readProduct(".gitignore")
	rootAttributes := readProduct(".gitattributes")

	// Assert the allowlist negation shape on the committed product bytes.
	for _, line := range []string{"/.sworn/*", "!/.sworn/records/"} {
		if !strings.Contains(rootIgnore, line) {
			t.Fatalf("root .gitignore lacks allowlist line %q:\n%s", line, rootIgnore)
		}
	}
	// The records roots stay export-ignored whether legacy or configured:
	// until the operator-gated migration runs, .baton/releases still holds
	// the historical records, and records under either root are never
	// package input.
	for _, line := range []string{
		"/.baton/releases export-ignore",
		"/.baton/releases/** export-ignore",
		"/.sworn/records export-ignore",
		"/.sworn/records/** export-ignore",
	} {
		if !strings.Contains(rootAttributes, line) {
			t.Fatalf("root .gitattributes lacks records export-ignore line %q:\n%s", line, rootAttributes)
		}
	}

	// Post-migration fixture shape.
	write := func(relative, body string) {
		t.Helper()
		target := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", rootIgnore)
	write(".gitattributes", rootAttributes)
	write("docs/sworn/sworn.json", `{
  "schema_version": "sworn.project-config/v1",
  "records_root": ".sworn/records",
  "journals_root": ".sworn",
  "contracts_root": "contracts",
  "commit_prefix": "sworn",
  "documents_root": "docs/sworn"
}
`)
	write("docs/sworn/2026-08-12-configurable-paths/plan.md", "authored plan\n")
	write("docs/sworn/2026-08-12-configurable-paths/contracts/S2.json", "{}\n")
	write("contracts/2026-08-12-configurable-paths/rev4/S2.json", "{}\n")
	write(".sworn/records/2026-08-12-configurable-paths/plan.md", "recorded plan\n")
	// Run state and working files that must stay untracked.
	write(".sworn/runs/run-1/journal.jsonl", "{}\n")
	write(".sworn/workspaces/abc/tmp/output.txt", "scratch\n")

	runFixtureGit("add", "-A", "--", ".")
	lsFiles := runFixtureGit("ls-files")
	tracked := make(map[string]bool)
	for _, line := range strings.Split(lsFiles, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			tracked[line] = true
		}
	}

	// (b) records at the resolved root and the plan/contract/config docs are
	// tracked.
	for _, expected := range []string{
		".sworn/records/2026-08-12-configurable-paths/plan.md",
		"docs/sworn/2026-08-12-configurable-paths/plan.md",
		"docs/sworn/2026-08-12-configurable-paths/contracts/S2.json",
		"docs/sworn/sworn.json",
		"contracts/2026-08-12-configurable-paths/rev4/S2.json",
		".gitignore",
		".gitattributes",
	} {
		if !tracked[expected] {
			t.Fatalf("expected tracked path %q, ls-files:\n%s", expected, lsFiles)
		}
	}
	// (c) nothing under .sworn/ except .sworn/records/ is tracked.
	for path := range tracked {
		if path == ".sworn" || strings.HasPrefix(path, ".sworn/") {
			if path != ".sworn/records/2026-08-12-configurable-paths/plan.md" &&
				!strings.HasPrefix(path, ".sworn/records/") {
				t.Fatalf("run state path %q is tracked:\n%s", path, lsFiles)
			}
		}
	}
	// (d) no .baton path is tracked.
	for path := range tracked {
		if path == ".baton" || strings.HasPrefix(path, ".baton/") {
			t.Fatalf("legacy path %q is tracked:\n%s", path, lsFiles)
		}
	}
}
