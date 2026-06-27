package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRouteIntegration runs `sworn route` against a fixture repo and asserts
// the CLI command is reachable and produces correct JSON.
func TestRouteIntegration(t *testing.T) {
	swornBin := buildSworn(t)

	repoDir, release := setupRouteFixture(t)

	// Run sworn route for each test case.
	tests := []struct {
		sliceID      string
		expectedType string
	}{
		{"S01-planned", "implement"},
		{"S02-inprogress", "implement"},
		{"S03-implemented", "verify"},
		{"S04-failed-verif", "redesign"}, // Gate 1 → redesign
		{"S05-shipped", "none"},
		{"S06-deferred", "none"},
		{"S07-blocked", "replan-release"},
	}

	for _, tt := range tests {
		t.Run(tt.sliceID, func(t *testing.T) {
			cmd := exec.Command(swornBin, "route", tt.sliceID, release)
			cmd.Dir = repoDir
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("sworn route %s: %v", tt.sliceID, err)
			}

			var result struct {
				Next struct {
					Type string `json:"type"`
				} `json:"next"`
			}
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatalf("parse output: %v\noutput: %s", err, string(out))
			}

			if result.Next.Type != tt.expectedType {
				t.Errorf("sworn route %s: next.type=%q, expected=%q", tt.sliceID, result.Next.Type, tt.expectedType)
			}
		})
	}
}

// setupRouteFixture creates a temp git repo with fixture slices and returns
// the repo dir and release name.
func setupRouteFixture(t *testing.T) (repoDir, release string) {
	t.Helper()

	repoDir = t.TempDir()
	release = "route-test"

	rtRunGit(t, repoDir, "init")
	rtRunGit(t, repoDir, "config", "user.email", "test@swornagent.dev")
	rtRunGit(t, repoDir, "config", "user.name", "sworn test")

	// Create release-wt branch.
	releaseWtBranch := "release-wt/" + release
	rtRunGit(t, repoDir, "checkout", "-b", releaseWtBranch)

	// Create release directory.
	releaseDir := filepath.Join(repoDir, "docs", "release", release)
	rtMustMkdir(t, releaseDir)

	// Write index.md.
	indexContent := `---
release_benefit: route integration test
tracks:
  - id: T1-core
    worktree_branch: track/route-test/T1-core
    state: in_progress
    slices:
      - S01-planned
      - S02-inprogress
      - S03-implemented
      - S04-failed-verif
      - S05-shipped
      - S06-deferred
      - S07-blocked
---
# route test
`
	rtMustWriteFile(t, filepath.Join(releaseDir, "index.md"), indexContent)

	// Write per-slice status.json files.
	slices := map[string]string{
		"S01-planned":    `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"S01-planned","release":"route-test","track":"T1-core","state":"planned","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{"violations":[]}}`,
		"S02-inprogress": `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"S02-inprogress","release":"route-test","track":"T1-core","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{"violations":[]}}`,
		"S03-implemented": `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"S03-implemented","release":"route-test","track":"T1-core","state":"implemented","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{"violations":[]}}`,
		"S04-failed-verif": `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"S04-failed-verif","release":"route-test","track":"T1-core","state":"failed_verification","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{"result":"fail","violations":["Gate 1: reachability artefact missing"]}}`,
		"S05-shipped":   `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"S05-shipped","release":"route-test","track":"T1-core","state":"shipped","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{"violations":[]}}`,
		"S06-deferred":  `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"S06-deferred","release":"route-test","track":"T1-core","state":"deferred","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{"violations":[]}}`,
		"S07-blocked":   `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"S07-blocked","release":"route-test","track":"T1-core","state":"verified","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{"result":"blocked","violations":["spec defect: ambiguous AC"]}}`,
	}

	for sid, content := range slices {
		sliceDir := filepath.Join(releaseDir, sid)
		rtMustMkdir(t, sliceDir)
		rtMustWriteFile(t, filepath.Join(sliceDir, "status.json"), content)
	}

	// Stage and commit.
	rtRunGit(t, repoDir, "add", ".")
	rtRunGit(t, repoDir, "commit", "-m", "fixture")

	// Create track branch.
	trackBranch := "track/" + release + "/T1-core"
	rtRunGit(t, repoDir, "checkout", "-b", trackBranch)

	return repoDir, release
}

func buildSworn(t *testing.T) string {
	t.Helper()

	// Find the module root.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Walk up to find go.mod.
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find go.mod")
		}
		dir = parent
	}

	bin := filepath.Join(t.TempDir(), "sworn")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/sworn")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func rtMustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func rtMustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func rtRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}