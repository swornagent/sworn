package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeTrack_AllVerified builds sworn and runs merge-track against a fixture
// where all slices are verified, expecting exit 0.
func TestMergeTrack_AllVerified(t *testing.T) {
	swornBin := buildSworn(t)
	repoDir, release, trackID := setupMergeFixture(t, "all-verified")

	// Write status.json as verified.
	writeMergeStatus(t, repoDir, release, "S01-verified", "verified")

	// Commit on release-wt, then create track branch.
	commitMergeFixture(t, repoDir, "fixture: release-wt with verified slices")
	createTrackBranch(t, repoDir, release, trackID)

	// Verify the track branch has the same status.json.
	verifyTrackStatus(t, repoDir, release, trackID)

	// Run merge-track.
	cmd := exec.Command(swornBin, "merge-track", trackID, "--release", release)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()

	if exitCode != 0 {
		t.Errorf("merge-track with all verified: exit %d, want 0\noutput: %s", exitCode, string(out))
	}
	_ = out
	_ = err
}

// TestMergeTrack_UnverifiedSlice blocks when a slice is not verified.
func TestMergeTrack_UnverifiedSlice(t *testing.T) {
	swornBin := buildSworn(t)
	repoDir, release, trackID := setupMergeFixture(t, "unverified")

	// Write S01-verified (name used in board.json) with state in_progress.
	writeMergeStatus(t, repoDir, release, "S01-verified", "in_progress")
	commitMergeFixture(t, repoDir, "fixture: release-wt with unverified slice")
	createTrackBranch(t, repoDir, release, trackID)

	cmd := exec.Command(swornBin, "merge-track", trackID, "--release", release)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()

	if exitCode == 0 {
		t.Errorf("merge-track with unverified slice: exit 0, want non-zero\noutput: %s", string(out))
	}
	if !strings.Contains(string(out), "not verified") {
		t.Errorf("expected output to contain 'not verified', got: %s", string(out))
	}
	_ = err
}

// TestMergeTrack_Invariant4Conflict creates a BOTH-MODIFIED file conflict on a
// non-documented-shared file and asserts the merge is blocked with the
// invariant-4 message.
func TestMergeTrack_Invariant4Conflict(t *testing.T) {
	swornBin := buildSworn(t)
	repoDir, release, trackID := setupMergeFixture(t, "invariant4")

	// Write a file on release-wt and commit.
	conflictPath := filepath.Join(repoDir, "conflicting.go")
	os.WriteFile(conflictPath, []byte("// release-wt version A\n"), 0644)

	writeMergeStatus(t, repoDir, release, "S01-verified", "verified")
	commitMergeFixture(t, repoDir, "fixture: release-wt with conflicting.go vA")

	// Create track branch, modify the file on the track branch, commit.
	createTrackBranch(t, repoDir, release, trackID)
	trackBranch := fmt.Sprintf("track/%s/%s", release, trackID)
	runGit(t, repoDir, "checkout", trackBranch)
	os.WriteFile(conflictPath, []byte("// track version B — different\n"), 0644)
	runGit(t, repoDir, "add", "conflicting.go")
	runGit(t, repoDir, "commit", "-m", "track: modify conflicting.go to vB")

	// Back on release-wt, make a DIFFERENT change to the same file — this
	// creates a both-modified conflict when merging.
	runGit(t, repoDir, "checkout", "release-wt/"+release)
	os.WriteFile(conflictPath, []byte("// release-wt version C — also different\n"), 0644)
	runGit(t, repoDir, "add", "conflicting.go")
	runGit(t, repoDir, "commit", "-m", "release-wt: modify conflicting.go to vC")

	cmd := exec.Command(swornBin, "merge-track", trackID, "--release", release)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()

	if exitCode == 0 {
		t.Errorf("merge-track with invariant-4 conflict: exit 0, want non-zero\noutput: %s", string(out))
	}
	if !strings.Contains(string(out), "invariant-4") {
		t.Errorf("expected invariant-4 in output, got: %s", string(out))
	}
	_ = err
}

// TestMergeRelease_NoJourneys verifies the journey gate blocks merge-release
// when journeys.json does not exist.
func TestMergeRelease_NoJourneys(t *testing.T) {
	swornBin := buildSworn(t)
	repoDir, release, trackID := setupMergeFixture(t, "no-journeys")

	// All slices verified.
	writeMergeStatus(t, repoDir, release, "S01-verified", "verified")
	commitMergeFixture(t, repoDir, "fixture: release-wt with verified slices")
	createTrackBranch(t, repoDir, release, trackID)

	// Merge the track first (so it's an ancestor of release-wt).
	mergeTrackIntoRelease(t, repoDir, trackID, release)

	// Now run merge-release — journeys.json won't exist.
	cmd := exec.Command(swornBin, "merge-release", "--release", release)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()

	if exitCode == 0 {
		t.Errorf("merge-release without journeys: exit 0, want non-zero\noutput: %s", string(out))
	}
	if !strings.Contains(string(out), "Rule 10 gate") {
		t.Errorf("expected 'Rule 10 gate' in output, got: %s", string(out))
	}
	_ = err
}

// TestMergeRelease_Pass verifies merge-release succeeds when all gates pass
// (verified slices + ratified journeys.json + merged tracks).
func TestMergeRelease_Pass(t *testing.T) {
	swornBin := buildSworn(t)
	repoDir, release, trackID := setupMergeFixture(t, "release-pass")

	// All slices verified.
	writeMergeStatus(t, repoDir, release, "S01-verified", "verified")
	commitMergeFixture(t, repoDir, "fixture: release-wt with verified slices")
	createTrackBranch(t, repoDir, release, trackID)

	// Merge the track first.
	mergeTrackIntoRelease(t, repoDir, trackID, release)

	// Create ratified journeys.json (needed for journey gate).
	createRatifiedJourneys(t, repoDir)

	cmd := exec.Command(swornBin, "merge-release", "--release", release)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()

	if exitCode != 0 {
		t.Errorf("merge-release with all gates passed: exit %d, want 0\noutput: %s", exitCode, string(out))
	}
	_ = out
	_ = err
}

// TestMergeTrack_OracleRouting verifies that merge-track uses the oracle
// (reads state from track branch git ref, not working-tree files).
func TestMergeTrack_OracleRouting(t *testing.T) {
	swornBin := buildSworn(t)
	repoDir, release, trackID := setupMergeFixture(t, "oracle-routing")

	// On release-wt: S01 is planned (not verified).
	writeMergeStatus(t, repoDir, release, "S01-verified", "planned")
	commitMergeFixture(t, repoDir, "fixture: release-wt: S01 planned")

	// Create track branch and switch to it.
	createTrackBranch(t, repoDir, release, trackID)
	// createTrackBranch returns to release-wt; switch to track.
	trackBranch := fmt.Sprintf("track/%s/%s", release, trackID)
	runGit(t, repoDir, "checkout", trackBranch)

	// On track: S01 is verified.
	writeMergeStatus(t, repoDir, release, "S01-verified", "verified")
	runGit(t, repoDir, "add", "docs/release/"+release+"/S01-verified/status.json")
	runGit(t, repoDir, "commit", "-m", "track: S01 verified")

	// Back on release-wt. Working tree has planned (from release-wt commit).
	// The oracle should read from the track branch (priority 1) and find verified.
	runGit(t, repoDir, "checkout", "release-wt/"+release)

	cmd := exec.Command(swornBin, "merge-track", trackID, "--release", release)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	exitCode := cmd.ProcessState.ExitCode()

	if exitCode != 0 {
		t.Errorf("merge-track oracle routing: exit %d, want 0 (oracle should read verified from track branch)\noutput: %s", exitCode, string(out))
	}
	_ = out
	_ = err
}

// --- helpers ---

// setupMergeFixture creates a temp git repo with the release-wt branch and
// basic board.json/index.md. Returns the repo dir, release name, and track id.
func setupMergeFixture(t *testing.T, name string) (repoDir, releaseName, trackID string) {
	t.Helper()

	repoDir = t.TempDir()
	releaseName = "merge-test-" + name
	trackID = "T1-core"

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@swornagent.dev")
	runGit(t, repoDir, "config", "user.name", "sworn test")

	// Create release-wt branch.
	releaseWtBranch := "release-wt/" + releaseName
	runGit(t, repoDir, "checkout", "-b", releaseWtBranch)

	// Create docs/release/<rel>/ directory.
	releaseDir := filepath.Join(repoDir, "docs", "release", releaseName)
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	trackBranch := fmt.Sprintf("track/%s/%s", releaseName, trackID)

	// Write board.json — the oracle reads this first.
	boardContent := fmt.Sprintf(`{
  "schema_version": 1,
  "release": {"name": %q},
  "release_worktree_path": %q,
  "release_worktree_branch": "release-wt/%s",
  "tracks": [
    {
      "id": %q,
      "slices": ["S01-verified"],
      "worktree_branch": %q,
      "state": "in_progress"
    }
  ]
}`, releaseName, repoDir, releaseName, trackID, trackBranch)
	if err := os.WriteFile(filepath.Join(releaseDir, "board.json"), []byte(boardContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write index.md — needed for resolveReleaseWorktree and ParseDocumentedShared.
	// Include a minimal touchpoint matrix so ParseDocumentedShared doesn't fail
	// (with no documented-shared entries — the empty set).
	indexContent := fmt.Sprintf(`---
release_worktree_path: %s
---
# merge test

## Touchpoint matrix

| File / surface | T1-core | T2-extra |
|----------------|---------|----------|
| some-other-file.go | ✓ | |
`, repoDir)
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}
	// Create slice directory.
	sliceDir := filepath.Join(releaseDir, "S01-verified")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}

	return repoDir, releaseName, trackID
}

// writeMergeStatus writes a status.json for the given slice in the fixture.
func writeMergeStatus(t *testing.T, repoDir, release, sliceID, state string) {
	t.Helper()
	statusPath := filepath.Join(repoDir, "docs", "release", release, sliceID, "status.json")
	status := map[string]interface{}{
		"$schema":         "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id":        sliceID,
		"release":         release,
		"track":           "T1-core",
		"state":           state,
		"owner":           "agent",
		"last_updated_at": "2026-01-01T00:00:00Z",
		"verification": map[string]interface{}{
			"result":     "pass",
			"violations": []string{},
		},
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPath, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// commitMergeFixture stages and commits the fixture on the current branch.
func commitMergeFixture(t *testing.T, repoDir, msg string) {
	t.Helper()
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", msg)
}

// createTrackBranch creates the track branch from the current (release-wt) branch.
func createTrackBranch(t *testing.T, repoDir, release, trackID string) {
	t.Helper()
	trackBranch := fmt.Sprintf("track/%s/%s", release, trackID)
	runGit(t, repoDir, "checkout", "-b", trackBranch)

	// Add a track-only commit so the track branch differs from release-wt
	// (ensures the oracle's priority-1 read finds committed state).
	anchorFile := filepath.Join(repoDir, ".track-anchor")
	os.WriteFile(anchorFile, []byte(trackBranch+"\n"), 0644)
	runGit(t, repoDir, "add", ".track-anchor")
	runGit(t, repoDir, "commit", "-m", "track: anchor commit")

	// Checkout release-wt for the test runs.
	runGit(t, repoDir, "checkout", "release-wt/"+release)
}

// verifyTrackStatus confirms the track branch has the S01-verified status.json
// by reading it with `git show`.
func verifyTrackStatus(t *testing.T, repoDir, release, trackID string) {
	t.Helper()
	trackBranch := fmt.Sprintf("track/%s/%s", release, trackID)
	cmd := exec.Command("git", "show", trackBranch+":docs/release/"+release+"/S01-verified/status.json")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify track status: %v\n%s", err, out)
	}
	var s map[string]interface{}
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("parse track status: %v", err)
	}
	if s["state"] != "verified" {
		t.Fatalf("track branch status.json state = %q, want verified", s["state"])
	}
}

// mergeTrackIntoRelease does a real git merge of the track branch into release-wt.
// Used to set up the merge-release test (track must be an ancestor).
func mergeTrackIntoRelease(t *testing.T, repoDir, trackID, release string) {
	t.Helper()
	runGit(t, repoDir, "checkout", "release-wt/"+release)
	trackBranch := fmt.Sprintf("track/%s/%s", release, trackID)
	runGit(t, repoDir, "merge", "--no-ff", trackBranch, "-m", "merge: "+trackBranch)
}

// createRatifiedJourneys creates a minimal ratified journeys.json artefact.
func createRatifiedJourneys(t *testing.T, repoDir string) {
	t.Helper()
	// journeys.json lives at .sworn/journeys.json.
	swornDir := filepath.Join(repoDir, ".sworn")
	if err := os.MkdirAll(swornDir, 0755); err != nil {
		t.Fatal(err)
	}
	artefact := map[string]interface{}{
		"version": 1,
		"ratification": map[string]interface{}{
			"is_ratified": true,
			"ratified_by": "brad",
			"ratified_at": "2026-01-01T00:00:00Z",
		},
		"journeys": []map[string]interface{}{
			{
				"id":            "J01-test",
				"user_type":     "developer",
				"outcome":       "Test journey",
				"entry_surface": "CLI",
				"steps": []map[string]interface{}{
					{"order": 1, "description": "Run merge", "surface": "merge"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(artefact, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(swornDir, "journeys.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	// Commit so the journey.Check can find it.
	runGit(t, repoDir, "add", ".sworn/journeys.json")
	runGit(t, repoDir, "commit", "-m", "fixture: ratified journeys.json")
}
