package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLintACCmd_MissingReleaseArg verifies that `sworn lint ac` without a
// release argument exits 64 (usage error).
func TestLintACCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdLintAC([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestLintACCmd_NonexistentRelease verifies that `sworn lint ac <nonexistent>`
// exits 2.
func TestLintACCmd_NonexistentRelease(t *testing.T) {
	exit := cmdLintAC([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestLintACCmd_AllWellFormed verifies that a release where every AC is
// well-formed EARS exits 0 and prints the pattern distribution.
// This is the integration test (Rule 1): it drives the actual command entry
// point (cmdLintAC), not just the ears package.
func TestLintACCmd_AllWellFormed(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist the form.
- [ ] WHILE the system is in maintenance mode THE SYSTEM SHALL show a banner.
- [ ] WHERE a premium feature is enabled THE SYSTEM SHALL show the export button.
- [ ] IF the database is unreachable THEN THE SYSTEM SHALL return a 503 error.
- [ ] WHEN a user clicks save WHILE the form is valid THE SYSTEM SHALL persist the form.
- [ ] NOTE: this is a deliberate non-requirement note.

## Required tests

- **Unit**: internal/ears/ears_test.go
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLintAC([]string{"test-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 for all-well-formed release, got %d", exit)
	}
}

// TestLintACCmd_FreeFormViolation verifies that a release with a free-form AC
// exits non-zero and names the slice + line.
func TestLintACCmd_FreeFormViolation(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] Make sure the form is saved.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist the form.

## Required tests

- **Unit**: some test
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLintAC([]string{"test-release"})
	if exit == 0 {
		t.Error("expected non-zero exit for free-form AC, got 0")
	}
}

// TestLintACCmd_NoteExcluded verifies that NOTE: lines are excluded and do not
// cause a violation.
func TestLintACCmd_NoteExcluded(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] NOTE: this is a deliberate non-requirement note.
- [ ] NOTE: another note that would be free-form if not for the escape.

## Required tests

- **Unit**: some test
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLintAC([]string{"test-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 (NOTEs excluded), got %d", exit)
	}
}

// TestLintACCmd_AllSixPatterns verifies that all six EARS pattern classes are
// recognised and the release passes.
func TestLintACCmd_AllSixPatterns(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)
	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist the form.
- [ ] WHILE the system is in maintenance mode THE SYSTEM SHALL show a banner.
- [ ] WHERE a premium feature is enabled THE SYSTEM SHALL show the export button.
- [ ] IF the database is unreachable THEN THE SYSTEM SHALL return a 503 error.
- [ ] WHEN a user clicks save WHILE the form is valid THE SYSTEM SHALL persist the form.

## Required tests

- **Unit**: some test
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdLintAC([]string{"test-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 for all six patterns, got %d", exit)
	}
}
