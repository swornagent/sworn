package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReqverifyCmd_MissingReleaseArg verifies that `sworn reqverify` without a
// release argument exits 64 (usage error).
func TestReqverifyCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdReqverify([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestReqverifyCmd_NonexistentRelease verifies that `sworn reqverify <nonexistent>`
// exits 2.
func TestReqverifyCmd_NonexistentRelease(t *testing.T) {
	exit := cmdReqverify([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestReqverifyCmd_NoModelConfigured verifies that `sworn reqverify` with a valid
// release exits 2 when no model is configured (model resolution happens before
// the reqverify Run, so even an empty release reaches this error).
func TestReqverifyCmd_NoModelConfigured(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	// Create an index.md so the release dir looks valid.
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte("---\ntitle: Test\n---"), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdReqverify([]string{"test-release"})
	// No model configured — should exit 2.
	if exit != 2 {
		t.Errorf("expected exit 2 when no model configured, got %d", exit)
	}
}

// TestReqverifyCmd_WithFixtureRelease verifies CLI wiring with a fixture release
// that has ACs. No model configured — exits 2 on model resolution error.
func TestReqverifyCmd_WithFixtureRelease(t *testing.T) {
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

- [ ] THE SYSTEM SHALL do something.
- [ ] THE SYSTEM SHALL do something else.

## Required tests

- **Unit**: internal/reqverify/reqverify_test.go
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdReqverify([]string{"test-release"})
	// No model configured — should exit 2.
	if exit != 2 {
		t.Errorf("expected exit 2 for unconfigured model, got %d", exit)
	}
}