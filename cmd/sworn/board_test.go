package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupBoardFixture creates a temp git repo with a release-wt branch
// containing index.md and per-slice status.json files. Returns the repo
// dir and the sworn binary path.
func setupBoardFixture(t *testing.T) (repoDir, swornBin string) {
	t.Helper()

	repoDir = t.TempDir()

	// Init git repo.
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@swornagent.dev")
	runGit(t, repoDir, "config", "user.name", "sworn test")

	// Create release directory structure under docs/release/<name>/.
	release := "test-release"
	releaseDir := filepath.Join(repoDir, "docs", "release", release)
	mustMkdir(t, releaseDir)

	// Write index.md with frontmatter.
	indexContent := `---
release_benefit: test release
tracks:
  - id: T1-core
    worktree_branch: track/test-release/T1-core
    state: in_progress
    slices:
      - S01-alpha
      - S02-beta
  - id: T2-aux
    worktree_branch: track/test-release/T2-aux
    state: planned
    depends_on:
      - T1-core
    slices:
      - S03-gamma
---
# test release
`
	mustWrite(t, filepath.Join(releaseDir, "index.md"), indexContent)

	// Write per-slice status.json files.
	slices := map[string]string{
		"S01-alpha": `{"slice_id":"S01-alpha","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`,
		"S02-beta":  `{"slice_id":"S02-beta","state":"verified","owner":"human","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`,
		"S03-gamma": `{"slice_id":"S03-gamma","state":"planned","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T2-aux","verification":{"result":"pending"}}`,
	}
	for sid, content := range slices {
		sliceDir := filepath.Join(releaseDir, sid)
		mustMkdir(t, sliceDir)
		mustWrite(t, filepath.Join(sliceDir, "status.json"), content)
	}

	// Commit everything on a release-wt branch.
	runGit(t, repoDir, "add", "docs/")
	runGit(t, repoDir, "commit", "-m", "initial release board")
	runGit(t, repoDir, "branch", "release-wt/test-release")

	// Build the sworn binary.
	swornBin = filepath.Join(t.TempDir(), "sworn")
	buildCmd := exec.Command("go", "build", "-o", swornBin, ".")
	buildCmd.Dir = repoDir
	// We can't build from the temp repo — it doesn't have the source.
	// Instead, build from the real project and copy.
	// For test simplicity, use `go run` or build a binary from the module root.
	_ = buildCmd

	// Actually, build from the module root (cwd of test).
	realSworn, err := exec.LookPath("sworn")
	if err != nil {
		// Build from source.
		cwd, _ := os.Getwd()
		realSworn = filepath.Join(t.TempDir(), "sworn-built")
		build := exec.Command("go", "build", "-o", realSworn, ".")
		build.Dir = cwd
		out, err := build.CombinedOutput()
		if err != nil {
			t.Fatalf("build sworn: %v\n%s", err, out)
		}
	}
	_ = realSworn

	// We need the binary accessible. Let's just build it.
	cwd, _ := os.Getwd()
	swornBin = filepath.Join(t.TempDir(), "sworn")
	build := exec.Command("go", "build", "-o", swornBin, ".")
	build.Dir = cwd
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build sworn: %v\n%s", err, out)
	}

	return repoDir, swornBin
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestBoardCLI_JSON(t *testing.T) {
	repoDir, swornBin := setupBoardFixture(t)

	// Run sworn board --release test-release --json from the repo dir.
	cmd := exec.Command(swornBin, "board", "--release", "test-release", "--json")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sworn board: %v\n%s", err, out)
	}

	// Parse JSON output.
	var result struct {
		Release string `json:"release"`
		Tracks  []struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Slices []struct {
				ID              string   `json:"id"`
				State           string   `json:"state"`
				Track           string   `json:"track"`
				Actionable      bool     `json:"actionable"`
				DependsOnTracks []string `json:"dependsOnTracks"`
				Blocked         bool     `json:"blocked"`
			} `json:"slices"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}

	if result.Release != "test-release" {
		t.Errorf("release: want test-release, got %s", result.Release)
	}
	if len(result.Tracks) != 2 {
		t.Fatalf("tracks: want 2, got %d", len(result.Tracks))
	}

	// T1-core should have S01-alpha (in_progress) and S02-beta (verified).
	t1 := result.Tracks[0]
	if t1.ID != "T1-core" {
		t.Errorf("track 0 id: want T1-core, got %s", t1.ID)
	}
	if len(t1.Slices) != 2 {
		t.Errorf("T1-core slices: want 2, got %d", len(t1.Slices))
	}

	// Find S02-beta — should be verified.
	for _, s := range t1.Slices {
		if s.ID == "S02-beta" && s.State != "verified" {
			t.Errorf("S02-beta state: want verified, got %s", s.State)
		}
	}
}
func TestBoardCLI_Text(t *testing.T) {
	repoDir, swornBin := setupBoardFixture(t)

	cmd := exec.Command(swornBin, "board", "--release", "test-release")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sworn board: %v\n%s", err, out)
	}

	text := string(out)
	if !strings.Contains(text, "T1-core") {
		t.Error("text output missing T1-core")
	}
	if !strings.Contains(text, "S01-alpha") {
		t.Error("text output missing S01-alpha")
	}
}

func TestBoardCLI_BlockedVisibility(t *testing.T) {
	repoDir, swornBin := setupBoardFixture(t)

	// Overwrite S01-alpha with a blocked verdict.
	s01Dir := filepath.Join(repoDir, "docs", "release", "test-release", "S01-alpha")
	mustWrite(t, filepath.Join(s01Dir, "status.json"),
		`{"slice_id":"S01-alpha","state":"implemented","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"blocked","violations":["spec defect: missing acceptance check"],"routing":"needs_planner"}}`)
	runGit(t, repoDir, "add", "docs/")
	runGit(t, repoDir, "commit", "-m", "blocked S01-alpha")
	// Update release-wt branch.
	runGit(t, repoDir, "branch", "-f", "release-wt/test-release")

	cmd := exec.Command(swornBin, "board", "--release", "test-release", "--json")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sworn board: %v\n%s", err, out)
	}

	var result struct {
		Tracks []struct {
			Slices []struct {
				ID            string `json:"id"`
				State         string `json:"state"`
				Blocked       bool   `json:"blocked"`
				BlockedReason string `json:"blocked_reason"`
				BlockedOwner  string `json:"blocked_owner"`
			} `json:"slices"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}

	for _, tr := range result.Tracks {
		for _, s := range tr.Slices {
			if s.ID == "S01-alpha" {
				if !s.Blocked {
					t.Error("S01-alpha: expected blocked=true")
				}
				if s.BlockedReason != "spec defect: missing acceptance check" {
					t.Errorf("blocked reason: got %q", s.BlockedReason)
				}
				if s.BlockedOwner != "needs_planner" {
					t.Errorf("blocked owner: got %q", s.BlockedOwner)
				}
				// State should still be "implemented" — blocked does not change state.
				if s.State != "implemented" {
					t.Errorf("state: want implemented, got %s", s.State)
				}
			}
		}
	}}