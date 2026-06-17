package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRtmCmd_MissingReleaseArg verifies that `sworn rtm` without a release
// argument exits 64 (usage error).
func TestRtmCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdRtm([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestRtmCmd_NonexistentRelease verifies that `sworn rtm <nonexistent>` exits 2.
func TestRtmCmd_NonexistentRelease(t *testing.T) {
	exit := cmdRtm([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestRtmCmd_FullyTracedRelease verifies that a fully-traced release exits 0
// and prints the matrix. This is the integration test (Rule 1): it drives the
// actual command entry point (cmdRtm), not just the rtm package.
func TestRtmCmd_FullyTracedRelease(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	// intake.md
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

	// index.md
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

	// S01-test-slice
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

	// Change to the temp dir so cmdRtm can resolve docs/release/test-release.
	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdRtm([]string{"test-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 for fully-traced release, got %d", exit)
	}
}

// TestRtmCmd_OrphanedNeed verifies that an orphaned need causes non-zero exit.
func TestRtmCmd_OrphanedNeed(t *testing.T) {
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

	exit := cmdRtm([]string{"test-release"})
	if exit == 0 {
		t.Error("expected non-zero exit for orphaned need, got 0")
	}
}

// TestRtmCmd_SoloFloorNoObjective verifies that a release with no org
// objective but a release goal passes (the lightweight floor).
func TestRtmCmd_SoloFloorNoObjective(t *testing.T) {
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

	exit := cmdRtm([]string{"test-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 for solo floor (no objective, release goal present), got %d", exit)
	}
}
