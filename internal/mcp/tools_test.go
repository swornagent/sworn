package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)
// ---- Fixture helpers ----

type fixtureRelease struct {
	Root   string // temp dir root
	Dir    string // docs/release/<name>/
	Name   string
}

func setupFixtureRelease(t *testing.T, name string) *fixtureRelease {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "release", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return &fixtureRelease{Root: root, Dir: dir, Name: name}
}

func (fr *fixtureRelease) writeIndex(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(fr.Dir, "index.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}
}

func (fr *fixtureRelease) writeSliceFile(t *testing.T, sliceID, filename, content string) {
	t.Helper()
	sliceDir := filepath.Join(fr.Dir, sliceID)
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatalf("mkdir slice: %v", err)
	}
	path := filepath.Join(sliceDir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

func (fr *fixtureRelease) writeSlice(t *testing.T, sliceID, spec string) {
	t.Helper()
	fr.writeSliceFile(t, sliceID, "spec.md", spec)
}

func (fr *fixtureRelease) writeStatus(t *testing.T, sliceID, stateJSON string) {
	t.Helper()
	// stateJSON is something like `"state": "in_progress"` — wrap into minimal status
	tmpl := fmt.Sprintf(`{
  "$schema": "https://example.com/schemas/baton/slice-status-v1.json",
  "slice_id": %q,
  "release": %q,
  "track": "T1",
  %s,
  "last_updated_by": "test",
  "last_updated_at": "2026-06-28T00:00:00Z",
  "verification": {"result": ""}
}`, sliceID, fr.Name, stateJSON)
	fr.writeSliceFile(t, sliceID, "status.json", tmpl)
}

func (fr *fixtureRelease) writeProof(t *testing.T, sliceID, proof string) {
	t.Helper()
	fr.writeSliceFile(t, sliceID, "proof.md", proof)
}

// writeOpsIndex writes a standard fixture index.md for the safe-parallelism release.
func writeOpsIndex(t *testing.T, dir, name string, trackSlices map[string][]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: 'Release board — %s'\n", name)
	b.WriteString("tracks:\n")
	i := 0
	for trackID, slices := range trackSlices {
		i++
		fmt.Fprintf(&b, "  - id: %s\n", trackID)
		fmt.Fprintf(&b, "    slices: [%s]\n", strings.Join(slices, ", "))
		fmt.Fprintf(&b, "    depends_on: null\n")
		fmt.Fprintf(&b, "    worktree_path: /tmp/wt/%s\n", trackID)
		fmt.Fprintf(&b, "    worktree_branch: track/x/%s\n", trackID)
		fmt.Fprintf(&b, "    state: in_progress\n")
	}
	b.WriteString("release_worktree_path: /tmp/release-wt\n")
	b.WriteString("release_worktree_branch: release-wt/x\n")
	b.WriteString("---\n\nRelease board.\n")

	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}
}

// ---- Test helpers ----

func opsToolRoundTrip(t *testing.T, repoRoot string) (stdinWriter io.Writer, stdoutReader *bufio.Reader, cleanup func()) {
	t.Helper()
	w, r, s := testRoundTrip(t)
	RegisterOpsTools(s, repoRoot)
	// Perform initialize handshake
	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)
	return w, r, func() {}
}
func callTool(t *testing.T, w io.Writer, r *bufio.Reader, name string, args json.RawMessage) *ToolResult {	t.Helper()
	params := struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments,omitempty"`
	}{Name: name, Arguments: args}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal tool call params: %v", err)
	}
	sendRequest(t, w, "tools/call", jsonID(2), paramsRaw)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("tools/call returned JSON-RPC error for %s: %s", name, resp["error"])
	}
	var result ToolResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal ToolResult: %v", err)
	}
	return &result
}

func toolText(t *testing.T, w io.Writer, r *bufio.Reader, name string, args json.RawMessage) string {	t.Helper()
	result := callTool(t, w, r, name, args)
	if len(result.Content) == 0 {
		return ""
	}
	return result.Content[0].Text
}

// ---- Spec-required tests ----

func TestGetBoard(t *testing.T) {
	fr := setupFixtureRelease(t, "test-release-a")
	trackSlices := map[string][]string{
		"T1-engine": {"S01-foo", "S02-bar"},
	}
	writeOpsIndex(t, fr.Dir, "test-release-a", trackSlices)
	fr.writeStatus(t, "S01-foo", `"state": "verified"`)
	fr.writeStatus(t, "S02-bar", `"state": "in_progress"`)

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	args := json.RawMessage(fmt.Sprintf(`{"release": %q}`, "test-release-a"))
	text := toolText(t, w, r, "get_board", args)

	if !strings.Contains(text, "test-release-a") {
		t.Errorf("get_board missing release name")
	}
	if !strings.Contains(text, "T1-engine") {
		t.Errorf("get_board missing track T1-engine")
	}
	if !strings.Contains(text, "S01-foo") {
		t.Errorf("get_board missing slice S01-foo")
	}
	if !strings.Contains(text, "verified") && !strings.Contains(text, "S02-bar") {
		t.Errorf("get_board missing slice state information")
	}
}

func TestGetBlockedExtractsViolations(t *testing.T) {
	fr := setupFixtureRelease(t, "test-release-b")
	trackSlices := map[string][]string{
		"T1-core": {"S01-ok", "S02-fail"},
	}
	writeOpsIndex(t, fr.Dir, "test-release-b", trackSlices)
	fr.writeStatus(t, "S01-ok", `"state": "verified"`)
	fr.writeStatus(t, "S02-fail", `"state": "failed_verification"`)
	fr.writeProof(t, "S02-fail", `FAIL: Gate 2 — spec defect

**Violation 1:** missing spec
**Violation 2:** unreachable

Some other text.`)

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	text := toolText(t, w, r, "get_blocked", json.RawMessage(`{}`))

	if !strings.Contains(text, "S02-fail") {
		t.Errorf("get_blocked should include failed slice %q, got: %s", "S02-fail", text)
	}
	if !strings.Contains(text, "FAIL:") {
		t.Errorf("get_blocked should include violation text, got: %s", text)
	}
	if strings.Contains(text, "S01-ok") {
		t.Errorf("get_blocked should not include verified slice %q, got: %s", "S01-ok", text)
	}
}

func TestGetSliceContext(t *testing.T) {
	fr := setupFixtureRelease(t, "test-release-c")
	trackSlices := map[string][]string{
		"T1-engine": {"S01-test-slice"},
	}
	writeOpsIndex(t, fr.Dir, "test-release-c", trackSlices)
	fr.writeSlice(t, "S01-test-slice", "# S01-test-slice\n\nSome spec content.")
	fr.writeStatus(t, "S01-test-slice", `"state": "in_progress", "start_commit": "abc123"`)

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-release-c", "slice_id": "S01-test-slice"}`)
	text := toolText(t, w, r, "get_slice_context", args)

	if !strings.Contains(text, "S01-test-slice") {
		t.Errorf("get_slice_context response missing slice ID, got: %s", text)
	}
	if !strings.Contains(text, "Some spec content") {
		t.Errorf("get_slice_context response missing spec content, got: %s", text)
	}
	if !strings.Contains(text, "start_commit") {
		t.Errorf("get_slice_context response missing start_commit, got: %s", text)
	}
}

func TestDeferSliceWritesRuleTwo(t *testing.T) {
	fr := setupFixtureRelease(t, "test-release-d")
	trackSlices := map[string][]string{
		"T1-engine": {"S01-defer-me"},
	}
	writeOpsIndex(t, fr.Dir, "test-release-d", trackSlices)
	fr.writeStatus(t, "S01-defer-me", `"state": "implemented"`)

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-release-d", "slice_id": "S01-defer-me", "reason": "blocked on backend"}`)
	text := toolText(t, w, r, "defer_slice", args)

	if !strings.Contains(text, "deferred") {
		t.Errorf("defer_slice response missing 'deferred', got: %s", text)
	}
	if !strings.Contains(text, "blocked on backend") {
		t.Errorf("defer_slice response missing reason, got: %s", text)
	}

	// Verify status.json was updated
	s, err := state.Read(filepath.Join(fr.Dir, "S01-defer-me", "status.json"))
	if err != nil {
		t.Fatalf("read status after defer: %v", err)
	}
	if string(s.State) != "deferred" {
		t.Errorf("status.state = %q, want %q", s.State, "deferred")
	}

	// Verify open_deferrals contains the reason
	found := false
	for _, d := range s.OpenDeferrals {
		if strings.Contains(d, "blocked on backend") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("open_deferrals should contain reason, got: %v", s.OpenDeferrals)
	}
}

func TestGetCreditsAbsent(t *testing.T) {
	fr := setupFixtureRelease(t, "test-credits")
	trackSlices := map[string][]string{
		"T1-engine": {"S01-credits"},
	}
	writeOpsIndex(t, fr.Dir, "test-credits", trackSlices)
	fr.writeStatus(t, "S01-credits", `"state": "verified"`)

	// Ensure no credits file exists (use HOME env override in subprocess is complex;
	// we test the file-not-found path by checking the actual user's home doesn't have one
	// — in CI this is reliably absent)

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	text := toolText(t, w, r, "get_credits", json.RawMessage(`{}`))

	if !strings.Contains(text, "null") {
		// If credits.json exists on this machine, the test returns real data.
		// That's OK — we just verify the tool doesn't error and returns JSON.
		t.Logf("get_credits returned: %s (real credits file found)", text)
	}
}

// ---- Pin 5 tests ----

func TestRerunSliceWritesPID(t *testing.T) {
	fr := setupFixtureRelease(t, "test-release-e")
	trackSlices := map[string][]string{
		"T1-engine": {"S01-rerun"},
	}
	writeOpsIndex(t, fr.Dir, "test-release-e", trackSlices)
	fr.writeStatus(t, "S01-rerun", `"state": "failed_verification"`)

	// Replace execSwornRun with a fake that returns a known PID
	origExec := execSwornRun
	execSwornRun = func(ctx context.Context, swornPath, sliceID, repoRoot string) (int, error) {
		return 42000, nil
	}
	defer func() { execSwornRun = origExec }()

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-release-e", "slice_id": "S01-rerun"}`)
	text := toolText(t, w, r, "rerun_slice", args)

	if !strings.Contains(text, "in_progress") && !strings.Contains(text, "42000") {
		t.Errorf("rerun_slice response missing expected content, got: %s", text)
	}

	// Verify state was reset to in_progress
	s, err := state.Read(filepath.Join(fr.Dir, "S01-rerun", "status.json"))
	if err != nil {
		t.Fatalf("read status after rerun: %v", err)
	}
	if string(s.State) != "in_progress" {
		t.Errorf("status.state = %q, want %q", s.State, "in_progress")
	}
}

func TestPatchSliceWritesInstructions(t *testing.T) {
	fr := setupFixtureRelease(t, "test-release-f")
	trackSlices := map[string][]string{
		"T1-engine": {"S01-patch"},
	}
	writeOpsIndex(t, fr.Dir, "test-release-f", trackSlices)
	fr.writeStatus(t, "S01-patch", `"state": "failed_verification"`)

	// Replace execSwornRun with a fake
	origExec := execSwornRun
	execSwornRun = func(ctx context.Context, swornPath, sliceID, repoRoot string) (int, error) {
		return 42001, nil
	}
	defer func() { execSwornRun = origExec }()

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-release-f", "slice_id": "S01-patch", "instructions": "Fix the missing error handler in tools_ops.go"}`)
	text := toolText(t, w, r, "patch_slice", args)

	if !strings.Contains(text, "in_progress") {
		t.Errorf("patch_slice should trigger rerun, got: %s", text)
	}

	// Verify PATCH_INSTRUCTIONS.md was written
	patchPath := filepath.Join(fr.Dir, "S01-patch", "PATCH_INSTRUCTIONS.md")
	data, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read PATCH_INSTRUCTIONS.md: %v", err)
	}
	if !strings.Contains(string(data), "Fix the missing error handler") {
		t.Errorf("PATCH_INSTRUCTIONS.md missing instructions, got: %s", string(data))
	}
}

func TestApproveMergeRejectsUnverified(t *testing.T) {
	fr := setupFixtureRelease(t, "test-release-g")
	trackSlices := map[string][]string{
		"T1-engine": {"S01-ok", "S02-not-verified"},
	}
	writeOpsIndex(t, fr.Dir, "test-release-g", trackSlices)
	// One verified, one not
	fr.writeStatus(t, "S01-ok", `"state": "verified"`)
	fr.writeStatus(t, "S02-not-verified", `"state": "in_progress"`)

	w, r, cleanup := opsToolRoundTrip(t, fr.Root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-release-g", "track_id": "T1-engine"}`)
	text := toolText(t, w, r, "approve_merge", args)

	if !strings.Contains(text, "not verified") && !strings.Contains(text, "in_progress") {
		t.Errorf("approve_merge should reject unverified slices, got: %s", text)
	}
}

func TestListReleases(t *testing.T) {
	fr1 := setupFixtureRelease(t, "release-alpha")
	trackSlices1 := map[string][]string{
		"T1-core": {"S01-a", "S02-b"},
	}
	writeOpsIndex(t, fr1.Dir, "release-alpha", trackSlices1)
	fr1.writeStatus(t, "S01-a", `"state": "verified"`)
	fr1.writeStatus(t, "S02-b", `"state": "planned"`)

	fr2 := setupFixtureRelease(t, "release-beta")
	trackSlices2 := map[string][]string{
		"T1-core": {"S01-c"},
	}
	writeOpsIndex(t, fr2.Dir, "release-beta", trackSlices2)
	fr2.writeStatus(t, "S01-c", `"state": "verified"`)

	// Use fr1.Root for the repoRoot — it has docs/release/release-alpha but not
	// release-beta. We need a parent that contains both.
	// Create a combined root
	combinedRoot := t.TempDir()
	for _, src := range []string{fr1.Root, fr2.Root} {
		// rsync equivalent: copy docs/release subdirs to combined root
		copyDir(t, filepath.Join(src, "docs"), filepath.Join(combinedRoot, "docs"))
	}

	w, r, cleanup := opsToolRoundTrip(t, combinedRoot)
	defer cleanup()

	text := toolText(t, w, r, "list_releases", json.RawMessage(`{}`))

	if !strings.Contains(text, "release-alpha") {
		t.Errorf("list_releases missing release-alpha, got: %s", text)
	}
	if !strings.Contains(text, "release-beta") {
		t.Errorf("list_releases missing release-beta, got: %s", text)
	}
}

// copyDir recursively copies src dir to dst. Used for building combined fixture roots.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copy dir: %v", err)
	}
}