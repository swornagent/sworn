package implement

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func TestWriteProofRecord_UsesStartCommitDiff(t *testing.T) {
	dir := t.TempDir()

	// Init git repo with an initial commit.
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@swornagent.local")
	runCmd(t, dir, "git", "config", "user.name", "SwornAgent Test")

	// Create and commit an initial file.
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "initial.txt")
	runCmd(t, dir, "git", "commit", "-m", "initial commit")
	startCommit := strings.TrimSpace(runCmd(t, dir, "git", "rev-parse", "HEAD"))

	// Create another file AFTER the start commit.
	if err := os.WriteFile(filepath.Join(dir, "changed.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "changed.txt")
	runCmd(t, dir, "git", "commit", "-m", "change")

	// Create spec.md.
	spec := `---
title: Test slice
---

# Slice: S15-proof-test

## User outcome

Proof record generated with correct files_changed.

## In scope

- Create changed.txt

## Acceptance checks

- [x] files_changed uses start_commit diff

## Required tests

- **Unit**: go test ./...

## Out of scope

- N/A
`
	specPath := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create status.json with start_commit.
	status := `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S15-proof-test",
  "release": "2026-06-27-test",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "start_commit": "` + startCommit + `",
  "planned_files": ["changed.txt"],
  "test_commands": ["echo ok"],
  "open_deferrals": ["deferred-feature: not in scope (tracked: issue-DEFER-001, acknowledged: owner)"],  "verification": {"result": "pending"}
}`
	statusPath := filepath.Join(dir, "status.json")
	if err := os.WriteFile(statusPath, []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}

	sliceDir := dir
	if err := WriteProofRecord(dir, specPath, statusPath, sliceDir); err != nil {
		t.Fatalf("WriteProofRecord: %v", err)
	}

	// Verify proof.json exists and has correct content.
	proofJSONPath := filepath.Join(dir, "proof.json")
	data, err := os.ReadFile(proofJSONPath)
	if err != nil {
		t.Fatalf("proof.json not created: %v", err)
	}

	var rec proofRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("proof.json not valid JSON: %v", err)
	}

	if rec.Schema != baton.ProofSchemaURI {
		t.Errorf("$schema = %q, want %q", rec.Schema, baton.ProofSchemaURI)
	}
	if rec.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", rec.SchemaVersion)
	}

	// files_changed should contain changed.txt.
	found := false
	for _, f := range rec.FilesChanged {
		if strings.Contains(f, "changed.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("files_changed does not contain changed.txt: %v", rec.FilesChanged)
	}

	// not_delivered should reflect open_deferrals.
	if len(rec.NotDelivered) != 1 {
		t.Errorf("not_delivered length = %d, want 1", len(rec.NotDelivered))
	}
	if len(rec.NotDelivered) > 0 && !strings.Contains(rec.NotDelivered[0], "deferred-feature") {
		t.Errorf("not_delivered[0] = %q, want text about deferred-feature", rec.NotDelivered[0])
	}
}

func TestFilesChangedFromGit_FallbackToStatus(t *testing.T) {
	dir := t.TempDir()

	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@swornagent.local")
	runCmd(t, dir, "git", "config", "user.name", "SwornAgent Test")

	// Create a file and commit it.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "a.txt")
	runCmd(t, dir, "git", "commit", "-m", "initial")

	// Create a second file and commit it.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "b.txt")
	runCmd(t, dir, "git", "commit", "-m", "second")

	// Get the SHA of the first commit as start_commit.
	firstSHA := strings.TrimSpace(runCmd(t, dir, "git", "rev-parse", "HEAD~1"))

	// files_changed should use git diff --name-only <start_commit>..HEAD.
	files := filesChangedFromGit(dir, firstSHA)
	found := false
	for _, f := range files {
		if strings.Contains(f, "b.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("filesChangedFromGit with start_commit should include b.txt, got: %v", files)
	}

	// With empty start_commit, should fall back to HEAD~1..HEAD.
	filesNoSC := filesChangedFromGit(dir, "")
	if len(filesNoSC) == 0 {
		t.Error("filesChangedFromGit with empty start_commit returned no files")
	}
}

func TestDivergenceFromPlan(t *testing.T) {
	planned := []string{"a.go", "b.go"}
	actual := []string{"a.go", "c.go"}

	divs := divergenceFromPlan(planned, actual)
	if len(divs) != 2 {
		t.Fatalf("divergence length = %d, want 2", len(divs))
	}
	foundUnexpected := false
	foundMissing := false
	for _, d := range divs {
		if strings.Contains(d, "unexpected file: c.go") {
			foundUnexpected = true
		}
		if strings.Contains(d, "planned but not changed: b.go") {
			foundMissing = true
		}
	}
	if !foundUnexpected {
		t.Error("divergence missing 'unexpected file: c.go'")
	}
	if !foundMissing {
		t.Error("divergence missing 'planned but not changed: b.go'")
	}
}

func TestDeliveredFromSpec_OnlyChecked(t *testing.T) {
	spec := `## Acceptance checks

- [x] First checked AC
- [ ] Second unchecked AC
- [x] Third checked AC
`
	delivered := deliveredFromSpec(spec)
	if len(delivered) != 2 {
		t.Fatalf("delivered length = %d, want 2", len(delivered))
	}
	if delivered[0] != "First checked AC" {
		t.Errorf("delivered[0] = %q, want 'First checked AC'", delivered[0])
	}
	if delivered[1] != "Third checked AC" {
		t.Errorf("delivered[1] = %q, want 'Third checked AC'", delivered[1])
	}
}

func runCmd(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}