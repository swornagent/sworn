package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReqvalidateCmd_MissingReleaseArg verifies that `sworn reqvalidate` without
// a release argument exits 64 (usage error).
func TestReqvalidateCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdReqvalidate([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestReqvalidateCmd_NonexistentRelease verifies that `sworn reqvalidate
// <nonexistent>` exits 2.
func TestReqvalidateCmd_NonexistentRelease(t *testing.T) {
	exit := cmdReqvalidate([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestReqvalidateCmd_WithFixtureRelease verifies CLI wiring at the integration
// point (Rule 1): a fixture release with a slice whose status.json has no
// human_ratified record should cause exit 1 (fail-closed on missing validation).
func TestReqvalidateCmd_WithFixtureRelease(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "fixture-release")
	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// status.json without human_ratified, scenarios, or benefit_hypothesis.
	status := map[string]any{
		"slice_id": "S01-test-slice",
		"release":  "fixture-release",
		"state":    "implemented",
		"validation": map[string]any{
			"human_ratified": false,
		},
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), data, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	exit := cmdReqvalidate([]string{"fixture-release"})
	if exit != 1 {
		t.Errorf("expected exit 1 for unvalidated fixture release, got %d", exit)
	}
}
