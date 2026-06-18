package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

// writeDesignfitFixture creates a release directory with one slice's status.json
// for testing the designfit CLI command.
func writeDesignfitFixture(t *testing.T, dir, sliceID string, decisions []state.DesignDecision) string {
	t.Helper()
	releaseDir := filepath.Join(dir, "docs", "release", "test-designfit-release")
	sliceDir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	st := &state.Status{
		SliceID:         sliceID,
		DesignDecisions: decisions,
	}
	if err := state.Write(filepath.Join(sliceDir, "status.json"), st); err != nil {
		t.Fatal(err)
	}
	// Write minimal index.md so release dir structure looks valid.
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte("---\ntitle: Test Designfit Release\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return releaseDir
}

// TestDesignfitCmd_MissingReleaseArg verifies that `sworn designfit` without a
// release argument exits 64 (usage error).
func TestDesignfitCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdDesignfit([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestDesignfitCmd_NonexistentRelease verifies that `sworn designfit <nonexistent>`
// exits 2.
func TestDesignfitCmd_NonexistentRelease(t *testing.T) {
	exit := cmdDesignfit([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestDesignfitCmd_Type1NoDecision verifies AC1 via CLI: Type-1 without human_decision
// exits 1 and names the slice + choice.
func TestDesignfitCmd_Type1NoDecision(t *testing.T) {
	dir := t.TempDir()
	writeDesignfitFixture(t, dir, "S01-test", []state.DesignDecision{		{
			Choice:     "database-engine",
			StakeClass: state.Type1,
			Options:    []string{"PostgreSQL", "SQLite"},
			Rationale:  "migrations matter",
			// No HumanDecision — should fail
		},
	})

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdDesignfit([]string{"test-designfit-release"})
	if exit != 1 {
		t.Errorf("expected exit 1 for Type-1 without decision, got %d", exit)
	}
}

// TestDesignfitCmd_AllPass verifies AC3 via CLI: all Type-1 with human decisions
// exits 0.
func TestDesignfitCmd_AllPass(t *testing.T) {
	dir := t.TempDir()
	writeDesignfitFixture(t, dir, "S01-test", []state.DesignDecision{		{
			Choice:        "database-engine",
			StakeClass:    state.Type1,
			Options:       []string{"PostgreSQL", "SQLite"},
			HumanDecision: "PostgreSQL",
			Rationale:     "migrations matter",
		},
	})

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdDesignfit([]string{"test-designfit-release"})
	if exit != 0 {
		t.Errorf("expected exit 0 when all Type-1 have human decisions, got %d", exit)
	}
}

// TestDesignfitCmd_MultipleSlices verifies across-slice aggregation via CLI.
func TestDesignfitCmd_MultipleSlices(t *testing.T) {
	dir := t.TempDir()

	releaseDir := filepath.Join(dir, "docs", "release", "test-designfit-release")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// S01: Type-1 WITH decision -> pass
	writeReleaseSliceTest(t, releaseDir, "S01-pass", []state.DesignDecision{
		{
			Choice:        "cache-strategy",
			StakeClass:    state.Type1,
			Options:       []string{"redis", "memcached"},
			HumanDecision: "redis",
			Rationale:     "already running redis",
		},
	})

	// S02: Type-1 WITHOUT decision -> fail
	writeReleaseSliceTest(t, releaseDir, "S02-fail", []state.DesignDecision{
		{
			Choice:     "queue-provider",
			StakeClass: state.Type1,
			Options:    []string{"rabbitmq", "sqs"},
			// No HumanDecision
		},
	})

	// S03: no design decisions -> pass
	writeReleaseSliceTest(t, releaseDir, "S03-none", nil)

	// Write index.md
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte("---\ntitle: Test\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdDesignfit([]string{"test-designfit-release"})
	if exit != 1 {
		t.Errorf("expected exit 1 with one Type-1 without decision, got %d", exit)
	}
}

// writeReleaseSliceTest is a test helper for writing a slice's status.json
// in an already-created release directory.
func writeReleaseSliceTest(t *testing.T, releaseDir, sliceID string, decisions []state.DesignDecision) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	st := &state.Status{
		SliceID:         sliceID,
		DesignDecisions: decisions,
	}
	if err := state.Write(filepath.Join(sliceDir, "status.json"), st); err != nil {
		t.Fatal(err)
	}
}