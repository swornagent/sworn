package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLintTraceCmd_MissingReleaseArg verifies that `sworn lint trace` without
// a release argument exits 64 (usage error).
func TestLintTraceCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdLintTrace([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestLintTraceCmd_NonexistentRelease verifies that
// `sworn lint trace <nonexistent>` exits 2.
func TestLintTraceCmd_NonexistentRelease(t *testing.T) {
	exit := cmdLintTrace([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestLintTraceCmd_FullyTracedRelease verifies that a fully-traced release
// exits 0 and prints the matrix. This is the integration test (Rule 1): it
// drives the actual command entry point (cmdLintTrace), not just the rtm
// package.
func TestLintTraceCmd_FullyTracedRelease(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	intake := `---
title: Test intake
---

# Release Intake: test-release

## Release goal

The release goal text for testing.

## Needs

- N-01: First need for testing
- N-02: Second need for testing
`
	os.WriteFile(filepath.Join(releaseDir, "intake.md"), []byte(intake), 0644)

	index := `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
    worktree_branch: track/test/T1-test
---

# Board

## Release summary

- **Goal**: the release goal from index

## Release benefit

The release delivers value to users.
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(index), 0644)

	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-01.
- [ ] WHEN a test runs, THE SYSTEM SHALL verify N-02.

## Required tests

- **Unit**: internal/rtm/rtm_test.go — basic tests
- **Integration**: exercise the command end-to-end
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)
	status := `{
  "slice_id": "S01-test-slice",
  "state": "planned",
  "release_benefit": "The release delivers value to users."
}`
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLintTrace([]string{"test-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 for fully-traced release, got %d", exit)
	}
}

// TestLintTraceCmd_OrphanedNeed verifies that an orphaned need causes
// non-zero exit.
func TestLintTraceCmd_OrphanedNeed(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	intake := `---
title: Test intake
---

# Release Intake: test-release

## Release goal

The release goal text for testing.

## Needs

- N-01: First need for testing
- N-02: Orphaned need with no AC
`
	os.WriteFile(filepath.Join(releaseDir, "intake.md"), []byte(intake), 0644)

	index := `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
    worktree_branch: track/test/T1-test
---

# Board

## Release summary

- **Goal**: the release goal from index
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(index), 0644)

	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-01.

## Required tests

- **Unit**: some test
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)
	status := `{
  "slice_id": "S01-test-slice",
  "state": "planned"
}`
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLintTrace([]string{"test-release"})
	if exit == 0 {
		t.Error("expected non-zero exit for orphaned need, got 0")
	}
}

// TestLintTraceCmd_SoloFloorNoObjective verifies that a release with no org
// objective but a release goal passes (the lightweight floor).
func TestLintTraceCmd_SoloFloorNoObjective(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	intake := `---
title: Test intake
---

# Release Intake: test-release

## Release goal

The release goal text for testing.

## Needs

- N-01: First need for testing
`
	os.WriteFile(filepath.Join(releaseDir, "intake.md"), []byte(intake), 0644)

	index := `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
    worktree_branch: track/test/T1-test
---

# Board

## Release summary

- **Goal**: the release goal from index
`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(index), 0644)

	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-01.

## Required tests

- **Unit**: some test
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)
	status := `{
  "slice_id": "S01-test-slice",
  "state": "planned"
}`
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLintTrace([]string{"test-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 for solo floor (no objective, release goal present), got %d", exit)
	}
}
