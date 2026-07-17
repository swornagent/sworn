package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// setupBoardFixture creates a temp git repo with a release-wt branch
// containing index.md and per-slice status.json files. Returns the repo
// dir and the sworn binary path.
func setupBoardFixture(t *testing.T) (repoDir, swornBin string) {
	t.Helper()

	repoDir = t.TempDir()

	// Init git repo.
	runGit(t, repoDir, "init", "-b", "main")
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
		"S01-alpha": `{"slice_id":"S01-alpha","release":"test-release","state":"in_progress","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`,
		"S02-beta":  `{"slice_id":"S02-beta","release":"test-release","state":"verified","owner":"human","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"pending"}}`,
		"S03-gamma": `{"slice_id":"S03-gamma","release":"test-release","state":"planned","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T2-aux","verification":{"result":"pending"}}`,
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
		build := exec.Command("go", "build", "-buildvcs=false", "-o", realSworn, ".")
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
	build := exec.Command("go", "build", "-buildvcs=false", "-o", swornBin, ".")
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

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
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

func buildSwornBinary(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "sworn")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, ".")
	build.Dir = cwd
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sworn: %v\n%s", err, out)
	}
	return bin
}

func runBoard(t *testing.T, bin, repoDir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"board"}, args...)...)
	cmd.Dir = repoDir
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err == nil {
		return out.String(), errOut.String(), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run sworn board: %v", err)
	}
	return out.String(), errOut.String(), exitErr.ExitCode()
}

func setupAllRefsCatalogFixture(t *testing.T) (repoDir, swornBin string) {
	t.Helper()
	repoDir = t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "config", "user.email", "test@swornagent.dev")
	runGit(t, repoDir, "config", "user.name", "sworn test")
	mustWrite(t, filepath.Join(repoDir, "README.md"), "consumer\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "consumer head")

	writeRelease := func(release, track, slice, state string) {
		dir := filepath.Join(repoDir, "docs", "release", release)
		mustMkdir(t, filepath.Join(dir, slice))
		mustWrite(t, filepath.Join(dir, "board.json"), `{"$schema":"board-v1","release":{"name":"`+release+`"},"tracks":[{"id":"`+track+`","slices":["`+slice+`"]}]}`)
		mustWrite(t, filepath.Join(dir, slice, "status.json"), `{"slice_id":"`+slice+`","release":"`+release+`","track":"`+track+`","state":"`+state+`","last_updated_at":"2026-01-01T00:00:00Z","verification":{"result":"pending"}}`)
		runGit(t, repoDir, "add", "docs")
		runGit(t, repoDir, "commit", "-m", "add "+release)
		runGit(t, repoDir, "branch", "release-wt/"+release)
		runGit(t, repoDir, "reset", "--hard", "HEAD^")
	}
	writeRelease("alpha-release", "T1-alpha", "S01-alpha", "implemented")
	writeRelease("beta-release", "T1-beta", "S01-beta", "planned")
	for i := 0; i < 64; i++ {
		runGit(t, repoDir, "branch", fmt.Sprintf("irrelevant-%02d", i), "main")
	}

	// A non-topology track ref carries farther state evidence for alpha.
	runGit(t, repoDir, "checkout", "-b", "track/alpha-release/T1-alpha", "release-wt/alpha-release")
	status := filepath.Join(repoDir, "docs", "release", "alpha-release", "S01-alpha", "status.json")
	mustWrite(t, status, `{"slice_id":"S01-alpha","release":"alpha-release","track":"T1-alpha","state":"verified","last_updated_at":"2026-01-02T00:00:00Z","verification":{"result":"pending"}}`)
	runGit(t, repoDir, "add", "docs")
	runGit(t, repoDir, "commit", "-m", "verify alpha")
	runGit(t, repoDir, "checkout", "main")

	return repoDir, buildSwornBinary(t)
}

type repoSnapshot struct {
	Head, Branch, Refs, Status, CWD string
}

func snapshotRepo(t *testing.T, repoDir string) repoSnapshot {
	t.Helper()
	refs := strings.Fields(gitOutput(t, repoDir, "for-each-ref", "--format=%(refname)"))
	sort.Strings(refs)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return repoSnapshot{
		Head:   strings.TrimSpace(gitOutput(t, repoDir, "rev-parse", "HEAD")),
		Branch: strings.TrimSpace(gitOutput(t, repoDir, "branch", "--show-current")),
		Refs:   strings.Join(refs, "\n"),
		Status: gitOutput(t, repoDir, "status", "--porcelain"),
		CWD:    cwd,
	}
}

func TestBoardCLIAllRefsCatalogStateEvidenceReachability(t *testing.T) {
	repoDir, bin := setupAllRefsCatalogFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "board", "--json")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("sworn board --json exceeded 10s with many refs: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("sworn board --json: %v\n%s", err, out)
	}
	var got struct {
		Releases map[string]struct {
			Release, SourceRef string
			Tracks             []struct {
				Slices []struct{ ID, State, StateSource, StateDurability string }
			}
		} `json:"releases"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(got.Releases) != 2 {
		t.Fatalf("releases=%d, want 2: %s", len(got.Releases), out)
	}
	alphaPos := bytes.Index(out, []byte(`"alpha-release"`))
	betaPos := bytes.Index(out, []byte(`"beta-release"`))
	if alphaPos < 0 || betaPos < 0 || alphaPos >= betaPos {
		t.Fatalf("catalog keys are not bytewise sorted: %s", out)
	}
	alpha := got.Releases["alpha-release"]
	if alpha.Release != "alpha-release" {
		t.Fatalf("alpha release=%q", alpha.Release)
	}
	if alpha.SourceRef != "refs/heads/release-wt/alpha-release" {
		t.Errorf("sourceRef=%q", alpha.SourceRef)
	}
	s := alpha.Tracks[0].Slices[0]
	if s.State != "verified" || s.StateSource != "refs/heads/track/alpha-release/T1-alpha" || s.StateDurability != "committed" {
		t.Fatalf("alpha evidence=%+v", s)
	}
}

func TestBoardCLIAllRefsCatalogSourceRef(t *testing.T) {
	const release = "ranked-release"
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "config", "user.email", "test@swornagent.dev")
	runGit(t, repoDir, "config", "user.name", "sworn test")
	mustWrite(t, filepath.Join(repoDir, "README.md"), "consumer\n")
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "consumer head")

	dir := filepath.Join(repoDir, "docs", "release", release)
	mustMkdir(t, filepath.Join(dir, "S01-alpha"))
	mustWrite(t, filepath.Join(dir, "board.json"), `{"$schema":"board-v1","release":{"name":"ranked-release"},"tracks":[{"id":"T1","slices":["S01-alpha"]}]}`)
	mustWrite(t, filepath.Join(dir, "S01-alpha", "status.json"), `{"slice_id":"S01-alpha","release":"ranked-release","track":"T1","state":"planned","last_updated_at":"2026-01-01T00:00:00Z","verification":{"result":"pending"}}`)
	runGit(t, repoDir, "add", "docs")
	runGit(t, repoDir, "commit", "-m", "topology")
	topology := strings.TrimSpace(gitOutput(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "reset", "--hard", "HEAD^")

	for _, ref := range []string{
		"refs/heads/release-wt/" + release,
		"refs/remotes/a/release-wt/" + release,
		"refs/remotes/z/release-wt/" + release,
		"refs/heads/a-fallback",
		"refs/heads/z-fallback",
		"refs/remotes/a/topic",
		"refs/remotes/z/topic",
	} {
		runGit(t, repoDir, "update-ref", ref, topology)
	}

	bin := buildSwornBinary(t)
	steps := []struct {
		want   string
		remove []string
	}{
		{want: "refs/heads/release-wt/" + release, remove: []string{"refs/heads/release-wt/" + release}},
		{want: "refs/remotes/a/release-wt/" + release, remove: []string{"refs/remotes/a/release-wt/" + release, "refs/remotes/z/release-wt/" + release}},
		{want: "refs/heads/a-fallback", remove: []string{"refs/heads/a-fallback", "refs/heads/z-fallback"}},
		{want: "refs/remotes/a/topic"},
	}
	for _, step := range steps {
		stdout, stderr, code := runBoard(t, bin, repoDir, "--json")
		if code != 0 {
			t.Fatalf("sworn board --json exit=%d stderr=%s", code, stderr)
		}
		var result struct {
			Releases map[string]struct {
				SourceRef string `json:"sourceRef"`
			} `json:"releases"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("parse catalog: %v\n%s", err, stdout)
		}
		if got := result.Releases[release].SourceRef; got != step.want {
			t.Fatalf("sourceRef=%q, want %q", got, step.want)
		}
		for _, ref := range step.remove {
			runGit(t, repoDir, "update-ref", "-d", ref)
		}
	}
}

func TestBoardCLIAllRefsCatalogCanonicalSkewFailsClosed(t *testing.T) {
	bin := buildSwornBinary(t)
	for _, scope := range []struct {
		name, canonicalPrefix string
	}{
		{name: "local", canonicalPrefix: "refs/heads/release-wt/"},
		{name: "remote", canonicalPrefix: "refs/remotes/origin/release-wt/"},
	} {
		for _, defect := range []string{"missing", "malformed", "identity-mismatch"} {
			t.Run(scope.name+"-"+defect, func(t *testing.T) {
				release := scope.name + "-" + defect
				repoDir := t.TempDir()
				runGit(t, repoDir, "init", "-b", "main")
				runGit(t, repoDir, "config", "user.email", "test@swornagent.dev")
				runGit(t, repoDir, "config", "user.name", "sworn test")
				mustWrite(t, filepath.Join(repoDir, "README.md"), "consumer\n")
				runGit(t, repoDir, "add", "README.md")
				runGit(t, repoDir, "commit", "-m", "consumer head")
				base := strings.TrimSpace(gitOutput(t, repoDir, "rev-parse", "HEAD"))

				lowerDir := filepath.Join(repoDir, "docs", "release", release)
				mustMkdir(t, filepath.Join(lowerDir, "S01-alpha"))
				mustWrite(t, filepath.Join(lowerDir, "board.json"), `{"$schema":"board-v1","release":{"name":"`+release+`"},"tracks":[{"id":"T1","slices":["S01-alpha"]}]}`)
				mustWrite(t, filepath.Join(lowerDir, "S01-alpha", "status.json"), `{"slice_id":"S01-alpha","release":"`+release+`","track":"T1","state":"planned","last_updated_at":"2026-01-01T00:00:00Z","verification":{"result":"pending"}}`)
				runGit(t, repoDir, "add", "docs")
				runGit(t, repoDir, "commit", "-m", "valid lower topology")
				lower := strings.TrimSpace(gitOutput(t, repoDir, "rev-parse", "HEAD"))
				runGit(t, repoDir, "reset", "--hard", base)
				runGit(t, repoDir, "update-ref", "refs/heads/topic", lower)

				canonical := base
				if defect != "missing" {
					canonicalDir := filepath.Join(repoDir, "docs", "release", release)
					mustMkdir(t, canonicalDir)
					if defect == "malformed" {
						mustWrite(t, filepath.Join(canonicalDir, "board.json"), `{`)
					} else {
						mustWrite(t, filepath.Join(canonicalDir, "board.json"), `{"$schema":"board-v1","release":{"name":"wrong-release"},"tracks":[]}`)
					}
					runGit(t, repoDir, "add", "docs")
					runGit(t, repoDir, "commit", "-m", "broken canonical topology")
					canonical = strings.TrimSpace(gitOutput(t, repoDir, "rev-parse", "HEAD"))
					runGit(t, repoDir, "reset", "--hard", base)
				}
				canonicalRef := scope.canonicalPrefix + release
				runGit(t, repoDir, "update-ref", canonicalRef, canonical)

				stdout, stderr, code := runBoard(t, bin, repoDir, "--json")
				if code != 2 {
					t.Fatalf("exit=%d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
				}
				if stdout != "" {
					t.Fatalf("successful aggregate output must be empty, got %q", stdout)
				}
				if !strings.Contains(stderr, `release "`+release+`"`) || !strings.Contains(stderr, canonicalRef) {
					t.Fatalf("stderr must name release and canonical ref: %q", stderr)
				}
			})
		}
	}
}

func TestBoardCLIAllRefsCatalogSkipsInvalidNoncanonicalTopology(t *testing.T) {
	repoDir, bin := setupAllRefsCatalogFixture(t)

	// An arbitrary historical branch can carry records that no longer satisfy
	// the canonical board schema. It must neither abort an unrelated catalog
	// nor outrank a valid lower-priority noncanonical candidate for its own
	// release.
	runGit(t, repoDir, "checkout", "-b", "audit/legacy-topology", "main")
	for _, release := range []string{"fallback-release", "legacy-only"} {
		dir := filepath.Join(repoDir, "docs", "release", release)
		mustMkdir(t, dir)
		mustWrite(t, filepath.Join(dir, "board.json"), `{"release":"`+release+`","tracks":[]}`)
	}
	runGit(t, repoDir, "add", "docs")
	runGit(t, repoDir, "commit", "-m", "add legacy noncanonical boards")
	runGit(t, repoDir, "checkout", "main")

	runGit(t, repoDir, "checkout", "-b", "topic/fallback-valid", "main")
	validDir := filepath.Join(repoDir, "docs", "release", "fallback-release", "S01-fallback")
	mustMkdir(t, validDir)
	mustWrite(t, filepath.Join(repoDir, "docs", "release", "fallback-release", "board.json"), `{"$schema":"board-v1","release":{"name":"fallback-release"},"tracks":[{"id":"T1-fallback","slices":["S01-fallback"]}]}`)
	mustWrite(t, filepath.Join(validDir, "status.json"), `{"slice_id":"S01-fallback","release":"fallback-release","track":"T1-fallback","state":"planned","last_updated_at":"2026-01-01T00:00:00Z","verification":{"result":"pending"}}`)
	runGit(t, repoDir, "add", "docs")
	runGit(t, repoDir, "commit", "-m", "add valid fallback board")
	runGit(t, repoDir, "checkout", "main")

	stdout, stderr, code := runBoard(t, bin, repoDir, "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("aggregate board exit=%d stderr=%q stdout=%s", code, stderr, stdout)
	}
	var catalog struct {
		Releases map[string]struct {
			SourceRef string `json:"sourceRef"`
		} `json:"releases"`
	}
	if err := json.Unmarshal([]byte(stdout), &catalog); err != nil {
		t.Fatalf("parse aggregate: %v\n%s", err, stdout)
	}
	if _, found := catalog.Releases["legacy-only"]; found {
		t.Fatalf("invalid-only noncanonical release must be omitted: %s", stdout)
	}
	if got := catalog.Releases["fallback-release"].SourceRef; got != "refs/heads/topic/fallback-valid" {
		t.Fatalf("fallback sourceRef=%q, want valid noncanonical ref", got)
	}

	named, namedErr, namedCode := runBoard(t, bin, repoDir, "--release", "alpha-release", "--json")
	if namedCode != 0 || namedErr != "" {
		t.Fatalf("named board exit=%d stderr=%q stdout=%s", namedCode, namedErr, named)
	}
	var single struct {
		Release string `json:"release"`
	}
	if err := json.Unmarshal([]byte(named), &single); err != nil {
		t.Fatalf("parse named board: %v\n%s", err, named)
	}
	if single.Release != "alpha-release" {
		t.Fatalf("named release=%q", single.Release)
	}
}

func TestBoardCLIStateEvidenceProvenance(t *testing.T) {
	repoDir, bin := setupAllRefsCatalogFixture(t)
	decode := func(stdout string) (state, source, durability string, blocked bool, reason string) {
		t.Helper()
		var result struct {
			Tracks []struct {
				Slices []struct {
					State, StateSource, StateDurability string
					Blocked                             bool
					BlockedReason                       string `json:"blocked_reason"`
				}
			} `json:"tracks"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("parse named board: %v\n%s", err, stdout)
		}
		if len(result.Tracks) != 1 || len(result.Tracks[0].Slices) != 1 {
			t.Fatalf("unexpected board shape: %s", stdout)
		}
		s := result.Tracks[0].Slices[0]
		return s.State, s.StateSource, s.StateDurability, s.Blocked, s.BlockedReason
	}
	runNamed := func() (string, string, string, bool, string) {
		stdout, stderr, code := runBoard(t, bin, repoDir, "--release", "alpha-release", "--json")
		if code != 0 {
			t.Fatalf("named board exit=%d stderr=%s", code, stderr)
		}
		return decode(stdout)
	}

	state, source, durability, blocked, reason := runNamed()
	if state != "verified" || source != "refs/heads/track/alpha-release/T1-alpha" || durability != "committed" || blocked {
		t.Fatalf("farthest committed state = %q %q %q blocked=%v", state, source, durability, blocked)
	}

	statusPath := filepath.Join(repoDir, "docs", "release", "alpha-release", "S01-alpha", "status.json")
	mustMkdir(t, filepath.Dir(statusPath))
	// Exact normal-state/timestamp ties preserve the committed evidence.
	mustWrite(t, statusPath, `{"slice_id":"S01-alpha","release":"alpha-release","track":"T1-alpha","state":"verified","owner":"dirty","last_updated_at":"2026-01-02T00:00:00Z","verification":{"result":"pending"}}`)
	state, source, durability, _, _ = runNamed()
	if state != "verified" || source != "refs/heads/track/alpha-release/T1-alpha" || durability != "committed" {
		t.Fatalf("committed tie winner = %q %q %q", state, source, durability)
	}

	mustWrite(t, statusPath, `{"slice_id":"S01-alpha","release":"alpha-release","track":"T1-alpha","state":"shipped","last_updated_at":"2026-01-03T00:00:00Z","verification":{"result":"pending"}}`)
	state, source, durability, _, _ = runNamed()
	if state != "shipped" || source != "working-tree" || durability != "uncommitted" {
		t.Fatalf("dirty high-water state = %q %q %q", state, source, durability)
	}
	text, stderr, code := runBoard(t, bin, repoDir, "--release", "alpha-release")
	if code != 0 || stderr != "" || !strings.Contains(text, "[uncommitted]") {
		t.Fatalf("uncommitted text output=%q stderr=%q exit=%d", text, stderr, code)
	}

	mustWrite(t, statusPath, `{"slice_id":"S01-alpha","release":"alpha-release","track":"T1-alpha","state":"implemented","last_updated_at":"2026-01-04T00:00:00Z","verification":{"result":"blocked","violations":["late attention"],"routing":"needs_planner"}}`)
	state, source, durability, blocked, reason = runNamed()
	if state != "implemented" || source != "working-tree" || durability != "uncommitted" || !blocked || reason != "late attention" {
		t.Fatalf("attention winner = %q %q %q blocked=%v reason=%q", state, source, durability, blocked, reason)
	}
	text, stderr, code = runBoard(t, bin, repoDir, "--release", "alpha-release")
	if code != 0 || stderr != "" || !strings.Contains(text, "[uncommitted]") {
		t.Fatalf("blocked uncommitted text output=%q stderr=%q exit=%d", text, stderr, code)
	}
}

func TestBoardCLINamedReleaseJSONShapeCompatibility(t *testing.T) {
	repoDir, bin := setupAllRefsCatalogFixture(t)
	aggregate, stderr, code := runBoard(t, bin, repoDir, "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("aggregate board exit=%d stderr=%s", code, stderr)
	}
	named, stderr, code := runBoard(t, bin, repoDir, "--release", "alpha-release", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("named board exit=%d stderr=%s", code, stderr)
	}

	var namedShape map[string]json.RawMessage
	if err := json.Unmarshal([]byte(named), &namedShape); err != nil {
		t.Fatalf("parse named shape: %v\n%s", err, named)
	}
	if len(namedShape) != 2 || namedShape["release"] == nil || namedShape["tracks"] == nil || namedShape["releases"] != nil || namedShape["sourceRef"] != nil {
		t.Fatalf("named top-level shape changed: %s", named)
	}

	var catalog struct {
		Releases map[string]struct {
			Tracks []struct {
				Slices []struct{ StateSource, StateDurability string }
			}
		} `json:"releases"`
	}
	var single struct {
		Release string `json:"release"`
		Tracks  []struct {
			Slices []struct{ StateSource, StateDurability string }
		} `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(aggregate), &catalog); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(named), &single); err != nil {
		t.Fatal(err)
	}
	fromCatalog := catalog.Releases["alpha-release"].Tracks[0].Slices[0]
	fromNamed := single.Tracks[0].Slices[0]
	if single.Release != "alpha-release" || fromCatalog.StateSource != fromNamed.StateSource || fromCatalog.StateDurability != fromNamed.StateDurability {
		t.Fatalf("catalog/named provenance differs: catalog=%+v named=%+v release=%q", fromCatalog, fromNamed, single.Release)
	}
}

func TestBoardCLINamedAndCatalogStateEvidenceAgree(t *testing.T) {
	repoDir, bin := setupAllRefsCatalogFixture(t)
	aggregate, stderr, code := runBoard(t, bin, repoDir, "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("aggregate board exit=%d stderr=%s", code, stderr)
	}
	named, stderr, code := runBoard(t, bin, repoDir, "--release", "alpha-release", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("named board exit=%d stderr=%s", code, stderr)
	}
	var catalog struct {
		Releases map[string]struct {
			Tracks []struct {
				Slices []struct{ State, StateSource, StateDurability string }
			}
		} `json:"releases"`
	}
	var single struct {
		Tracks []struct {
			Slices []struct{ State, StateSource, StateDurability string }
		} `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(aggregate), &catalog); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(named), &single); err != nil {
		t.Fatal(err)
	}
	if got, want := single.Tracks[0].Slices[0], catalog.Releases["alpha-release"].Tracks[0].Slices[0]; got != want {
		t.Fatalf("named evidence=%+v, catalog evidence=%+v", got, want)
	}
}

func TestBoardCLIAllRefsCatalogReadOnly(t *testing.T) {
	repoDir, bin := setupAllRefsCatalogFixture(t)
	before := snapshotRepo(t, repoDir)
	stdout, stderr, code := runBoard(t, bin, repoDir, "--json")
	if code != 0 || stderr != "" || stdout == "" {
		t.Fatalf("board output exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	after := snapshotRepo(t, repoDir)
	if before != after {
		t.Fatalf("read-only snapshot changed:\n before=%+v\n after=%+v", before, after)
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
		`{"slice_id":"S01-alpha","release":"test-release","state":"implemented","owner":"agent","last_updated_at":"2026-01-01T00:00:00Z","track":"T1-core","verification":{"result":"blocked","violations":["spec defect: missing acceptance check"],"routing":"needs_planner"}}`)
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
	}
}
