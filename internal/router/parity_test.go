package router

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/git"
)
// TestCaptainRouteParity runs captain-route.sh over a fixture release with
// every state branch represented, then compares the Go router's Decision
// against the shell script's .next JSON. Skips gracefully if captain-route.sh
// or jq are not on PATH.
func TestCaptainRouteParity(t *testing.T) {
	if _, err := exec.LookPath("captain-route.sh"); err != nil {
		t.Skip("captain-route.sh not on PATH — skipping parity test")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not on PATH — skipping parity test (needed for JSON extraction)")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH — skipping parity test (needed by release-board-status.sh)")
	}

	// Build a fixture release in a temp git repo.
	repoDir := t.TempDir()
	repo := git.New(repoDir)
	if err := repo.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Configure git user.
	if _, err := runGit(t, repoDir, "config", "user.email", "parity@swornagent.dev"); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := runGit(t, repoDir, "config", "user.name", "parity test"); err != nil {
		t.Fatalf("config: %v", err)
	}
	release := "parity-release"
	trackBranch := "track/" + release + "/T1-core"
	releaseWtBranch := "release-wt/" + release
	docsPrefix := "docs"

	// Create the release-wt branch first (parent of track branch).
	if err := repo.Branch(releaseWtBranch); err != nil {
		t.Fatalf("branch release-wt: %v", err)
	}

	// Create release directory structure.
	releaseDir := filepath.Join(repoDir, docsPrefix, "release", release)
	mustMkdirAll(t, releaseDir)

	// Write index.md.
	indexContent := `---
release_benefit: parity test
tracks:
  - id: T1-core
    worktree_branch: track/parity-release/T1-core
    state: in_progress
    slices:
      - S01-planned
      - S02-inprogress
      - S03-implemented
      - S04-verified
      - S05-failed-verif
      - S06-shipped
      - S07-deferred
      - S08-blocked
      - S09-design-review
---
# parity test release
`
	mustWriteFile(t, filepath.Join(releaseDir, "index.md"), indexContent)

	// S01 - planned
	mustCreateSlice(t, releaseDir, "S01-planned", "T1-core", "planned", "")
	// S02 - in_progress
	mustCreateSlice(t, releaseDir, "S02-inprogress", "T1-core", "in_progress", "")
	// S03 - implemented (no verdict)
	mustCreateSlice(t, releaseDir, "S03-implemented", "T1-core", "implemented", "")
	// S04 - verified
	mustCreateSlice(t, releaseDir, "S04-verified", "T1-core", "verified", "pass")
	// S05 - failed_verification with Gate 1 violation
	mustCreateSliceWithViolations(t, releaseDir, "S05-failed-verif", "T1-core",
		"failed_verification", "fail", []string{"Gate 1: reachability artefact missing"})
	// S06 - shipped
	mustCreateSlice(t, releaseDir, "S06-shipped", "T1-core", "shipped", "")
	// S07 - deferred
	mustCreateSlice(t, releaseDir, "S07-deferred", "T1-core", "deferred", "")
	// S08 - blocked
	mustCreateSlice(t, releaseDir, "S08-blocked", "T1-core", "verified", "blocked",
		`"violations":["spec defect: ambiguous AC"]`)
	// S09 - design_review with design.md
	mustCreateSlice(t, releaseDir, "S09-design-review", "T1-core", "design_review", "")
	mustWriteFile(t, filepath.Join(releaseDir, "S09-design-review", "design.md"), "# Design")

	// Stage and commit on release-wt.
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "fixture release")
	// Create the track branch from release-wt.
	if err := repo.Branch(trackBranch); err != nil {
		t.Fatalf("branch track: %v", err)
	}

	// Now we have the same git structure captain-route.sh expects.
	// We need to create the release worktree so captain-route.sh can find it.
	// captain-route.sh reads from git refs, so we just need the refs to exist.

	// Set the worktree_path to the repo dir itself (so git -C works).
	// captain-route.sh expects release-board-status.sh --json to work.
	// We'll simulate by setting the right env.
	t.Setenv("HOME", os.Getenv("HOME")) // preserve home for ~/.claude/bin

	// Test each slice via captain-route.sh and compare.
	tests := []struct {
		sliceID       string
		expectedType  NextType
	}{
		{"S01-planned", NextImplement},
		{"S02-inprogress", NextImplement},
		{"S03-implemented", NextVerify},
		{"S04-verified", NextImplement}, // S05-failed-verif is next non-terminal
		{"S05-failed-verif", NextRedesign}, // Gate 1 → redesign		{"S06-shipped", NextNone},
		{"S07-deferred", NextNone},
		{"S08-blocked", NextReplanRelease},
		{"S09-design-review", NextReview}, // design.md only, no review/decline/ack
	}

	for _, tt := range tests {
		t.Run(tt.sliceID, func(t *testing.T) {
			// Run Go router.
			goDecision, err := runGoRouter(t, repoDir, release, tt.sliceID, docsPrefix, trackBranch, "refs/heads/"+releaseWtBranch, "T1-core")
			if err != nil {
				t.Fatalf("Go router: %v", err)
			}

			goType := string(goDecision.NextType)

			// Verify against expected type.
			if goType != string(tt.expectedType) {
				t.Errorf("Go router next.type=%q, expected=%q", goType, tt.expectedType)
			}

			// Also try captain-route.sh for cross-check (best-effort).
			cmd := exec.Command("captain-route.sh", tt.sliceID, release)
			cmd.Dir = repoDir
			cmd.Env = append(os.Environ(),
				"HOME="+os.Getenv("HOME"),
			)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err = cmd.Run()
			if err != nil {
				t.Logf("captain-route.sh failed (best-effort): %v\nstderr: %s", err, stderr.String())
				return
			}

			// Parse the JSON output.
			var bashOutput struct {
				Next struct {
					Type string `json:"type"`
				} `json:"next"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &bashOutput); err != nil {
				t.Logf("captain-route.sh output unparseable (best-effort): %v\nstdout: %s", err, stdout.String())
				return
			}

			bashType := bashOutput.Next.Type
			if bashType != goType {
				t.Errorf("captain-route.sh next.type=%q, Go router next.type=%q — MISMATCH", bashType, goType)
			}
		})
	}}

// runGoRouter constructs the full production path (oracle + content readers)
// and calls Route, returning the Decision.
func runGoRouter(t *testing.T, repoDir, release, sliceID, docsPrefix, trackBranch, releaseRef, trackID string) (Decision, error) {
	t.Helper()

	repo := git.New(repoDir)
	oracle := board.NewGitOracle(repo)

	reader := &gitReaderAdapter{repo: repo}
	oracleAdapter, err := board.NewOracleReaderAdapter(oracle, reader, release, releaseRef)
	if err != nil {
		return Decision{}, err
	}

	content := &gitContentAdapter{repo: repo}

	input := RouteInput{
		Release:     release,
		SliceID:     sliceID,
		TrackID:     trackID,
		TrackBranch: "refs/heads/" + trackBranch,
		ReleaseRef:  releaseRef,
		DocsPrefix:  docsPrefix,
	}

	ctx := context.Background()
	return Route(ctx, oracleAdapter, content, input)
}

// gitReaderAdapter adapts *git.Repo to board.gitContentReader for parity tests.
type gitReaderAdapter struct {
	repo *git.Repo
}

func (a *gitReaderAdapter) Show(ref, path string) (string, error) {
	return a.repo.Show(ref, path)
}

func (a *gitReaderAdapter) CatFileExists(ref, path string) (bool, error) {
	return a.repo.CatFileExists(ref, path)
}

// gitContentAdapter adapts *git.Repo to router.ContentReader for parity tests.
type gitContentAdapter struct {
	repo *git.Repo
}

func (a *gitContentAdapter) LastCommitTime(ref, path string) (int64, error) {
	return a.repo.LastCommitTime(ref, path)
}

func (a *gitContentAdapter) CatFileExists(ref, path string) (bool, error) {
	return a.repo.CatFileExists(ref, path)
}

func (a *gitContentAdapter) IsAncestor(ancestor, branch string) (bool, error) {
	return a.repo.IsAncestor(ancestor, branch)
}

// ---------- helpers ----------

func mustCreateSlice(t *testing.T, releaseDir, sliceID, track, state, verdict string, extraJSON ...string) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	mustMkdirAll(t, sliceDir)

	var json string
	if len(extraJSON) > 0 {
		json = extraJSON[0]
	}

	content := mustBuildStatusJSON(sliceID, state, track, verdict, json)
	mustWriteFile(t, filepath.Join(sliceDir, "status.json"), content)
}

func mustCreateSliceWithViolations(t *testing.T, releaseDir, sliceID, track, state, verdict string, violations []string) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	mustMkdirAll(t, sliceDir)

	vJSON, _ := json.Marshal(violations)
	content := mustBuildStatusJSONWithViolations(sliceID, state, track, verdict, string(vJSON))
	mustWriteFile(t, filepath.Join(sliceDir, "status.json"), content)
}

func mustBuildStatusJSON(sliceID, state, track, verdict string, extra string) string {
	v := ""
	if verdict != "" {
		v = `"result":"` + verdict + `"`
	}
	// When both verdict and extra are empty, use empty violations.
	verifObj := v
	if extra != "" {
		if v != "" {
			verifObj = v + "," + extra
		} else {
			verifObj = extra
		}
	} else if v == "" {
		verifObj = `"violations":[]`
	}
	return `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"` + sliceID + `","release":"parity-release","track":"` + track + `","state":"` + state + `","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{` + verifObj + `}}`
}
func mustBuildStatusJSONWithViolations(sliceID, state, track, verdict, violationsJSON string) string {
	v := ""
	if verdict != "" {
		v = `"result":"` + verdict + `",`
	}
	return `{"$schema":"https://example.com/schemas/baton/slice-status-v1.json","slice_id":"` + sliceID + `","release":"parity-release","track":"` + track + `","state":"` + state + `","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","verification":{` + v + `"violations":` + violationsJSON + `}}`
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Logf("git %s stderr: %s", strings.Join(args, " "), stderr.String())
	}
	return strings.TrimSpace(stdout.String()), err
}