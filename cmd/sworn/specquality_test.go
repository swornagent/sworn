package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSpecqualityCmd_MissingReleaseArg verifies that without a release argument,
// the command exits 64 (usage error).
func TestSpecqualityCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdSpecquality([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestSpecqualityCmd_NonexistentRelease verifies exit 2 for nonexistent release.
func TestSpecqualityCmd_NonexistentRelease(t *testing.T) {
	exit := cmdSpecquality([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestSpecqualityCmd_Pass verifies CLI wiring at the integration point (Rule 1):
// a sound+complete example set exits 0.
func TestSpecqualityCmd_Pass(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "fixture-release")
	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Write spec.md with acceptance examples and checks.
	spec := `# S01-test-slice

## Acceptance checks

- [ ] WHEN a release has valid input sworn lint ac SHALL exit 0
- [ ] WHEN a release has invalid input sworn lint ac SHALL exit 1

## Acceptance examples

- name: "pass-case"
  input: "valid input"
  expected: "sworn lint ac exits 0"

- name: "fail-case"
  input: "invalid input"
  expected: "sworn lint ac exits 1"
`
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Write minimal status.json.
	status := `{
  "$schema": "https://example.com/schemas/baton/slice-status-v1.json",
  "slice_id": "S01-test-slice",
  "state": "planned",
  "verification": {"result": "pending"}
}`
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdSpecquality([]string{"fixture-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 for sound+complete release, got %d", exit)
	}
}

// TestSpecqualityCmd_Fail_NoExamples verifies that a slice with no acceptance
// examples causes exit 1.
func TestSpecqualityCmd_Fail_NoExamples(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "fixture-release")
	sliceDir := filepath.Join(releaseDir, "S01-no-examples")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	spec := `# S01-no-examples

## Acceptance checks

- [ ] WHEN something happens THE SYSTEM SHALL respond

## Acceptance examples

`
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	status := `{
  "slice_id": "S01-no-examples",
  "state": "planned",
  "verification": {"result": "pending"}
}`
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdSpecquality([]string{"fixture-release"})
	if exit != 1 {
		t.Errorf("expected exit 1 for missing examples, got %d", exit)
	}
}

// TestSpecqualityCmd_Fail_LowCompleteness verifies that vague examples scoring
// below the threshold cause exit 1.
func TestSpecqualityCmd_Fail_LowCompleteness(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "fixture-release")
	sliceDir := filepath.Join(releaseDir, "S01-vague")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	spec := `# S01-vague

## Acceptance checks

- [ ] WHEN input is valid THE SYSTEM SHALL work

## Acceptance examples

- name: "works"
  input: "valid input"
  expected: "works correctly"
`
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	status := `{
  "slice_id": "S01-vague",
  "state": "planned",
  "verification": {"result": "pending"}
}`
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdSpecquality([]string{"fixture-release"})
	if exit != 1 {
		t.Errorf("expected exit 1 for low completeness, got %d", exit)
	}
}