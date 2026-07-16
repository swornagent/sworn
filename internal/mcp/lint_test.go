package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/swornagent/sworn/internal/board"
)

// TestRegisterLintTools_ToolList verifies all 6 lint tools are discoverable via
// the tools/list method. This is the primary acceptance check: an MCP client
// connecting to sworn mcp can see the lint tools.
func TestRegisterLintTools_ToolList(t *testing.T) {
	s := New()
	RegisterLintTools(s, ".")

	expected := []string{
		"sworn.lint",
		"sworn.lint_trace",
		"sworn.lint_coverage",
		"sworn.lint_design",
		"sworn.lint_mock",
		"sworn.llm_check",
	}

	s.mu.Lock()
	var names []string
	for name := range s.tools {
		if strings.HasPrefix(name, "sworn.") {
			names = append(names, name)
		}
	}
	s.mu.Unlock()

	sort.Strings(names)
	sort.Strings(expected)

	if len(names) != len(expected) {
		t.Fatalf("expected %d sworn.* tools, got %d: %v", len(expected), len(names), names)
	}
	for i := range names {
		if names[i] != expected[i] {
			t.Errorf("tool[%d]: want %q, got %q", i, expected[i], names[i])
		}
	}
}

// TestLintTools_RequireRelease verifies that tools requiring a release return
// an error when the release does not exist.
func TestLintTools_RequireRelease(t *testing.T) {
	s := New()
	RegisterLintTools(s, ".")

	// Test trace with missing release.
	params := json.RawMessage(`{"release": "nonexistent"}`)
	result, err := callRegisteredTool(s, "sworn.lint_trace", params)
	if err == nil {
		t.Errorf("expected error for nonexistent release, got result: %v", result)
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// TestLintTools_RequireSliceID verifies that tools requiring a slice_id return
// an error when the release exists but the slice does not.
func TestLintTools_RequireSliceID(t *testing.T) {
	fr := setupFixtureRelease(t, "2026-06-19-test-release")
	fr.writeSlice(t, "S99-dummy", `---
title: Test slice
---

# Slice: S99-dummy

## Acceptance checks

- [ ] AC1: WHEN the user clicks the button THE system SHALL respond within 100ms.
`)
	fr.writeStatus(t, "S99-dummy", `"state": "planned"`)
	fr.writeIndexContent(t, `title: test
release_worktree_path: `+fr.Root+`
release_worktree_branch: release-wt/2026-06-19-test-release
`)

	writeLintBoardJSON(t, fr.Dir, "2026-06-19-test-release", map[string][]string{
		"T1-test": {"S99-dummy"},
	}, fr.Root)

	s := New()
	RegisterLintTools(s, fr.Root)

	// Test coverage with nonexistent slice.
	params := json.RawMessage(`{"release": "2026-06-19-test-release", "slice_id": "S99-nonexistent"}`)
	_, err := callRegisteredTool(s, "sworn.lint_coverage", params)
	if err == nil {
		t.Errorf("expected error for nonexistent slice")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// TestLintTools_LLMCheckInvalidType verifies that llm_check rejects unknown check types.
func TestLintTools_LLMCheckInvalidType(t *testing.T) {
	fr := setupFixtureRelease(t, "2026-06-19-test-release")
	fr.writeSlice(t, "S99-dummy", `---
title: Test slice
---

# Slice: S99-dummy

## Acceptance checks

- [ ] AC1: WHEN the user clicks the button THE system SHALL respond within 100ms.
`)
	fr.writeStatus(t, "S99-dummy", `"state": "planned"`)
	fr.writeIndexContent(t, `title: test
release_worktree_path: `+fr.Root+`
release_worktree_branch: release-wt/2026-06-19-test-release
`)

	writeLintBoardJSON(t, fr.Dir, "2026-06-19-test-release", map[string][]string{
		"T1-test": {"S99-dummy"},
	}, fr.Root)

	s := New()
	RegisterLintTools(s, fr.Root)

	params := json.RawMessage(`{"release": "2026-06-19-test-release", "slice_id": "S99-dummy", "type": "invalid-check"}`)
	_, err := callRegisteredTool(s, "sworn.llm_check", params)
	if err == nil {
		t.Errorf("expected error for invalid check type")
	}
	if err != nil && !strings.Contains(err.Error(), "unknown check type") {
		t.Errorf("expected 'unknown check type' error, got: %v", err)
	}
}

// TestMCPGenericCheckIdentityReachability invokes the registered public MCP
// handler, not the gate directly. A wrong, missing, or unknown identity must
// remain observable in the raw response and produce MCP non-success.
func TestMCPGenericCheckIdentityReachability(t *testing.T) {
	fr := setupFixtureRelease(t, "2026-07-17-mcp-identity")
	fr.writeSlice(t, "S01-test", `# Slice: S01-test

## Acceptance checks

- [ ] THE SYSTEM SHALL preserve a model-emitted check identity.
`)

	responses := []string{
		`{"check":"design-review","verdict":"PASS","findings":[]}`,
		`{"verdict":"PASS","findings":[]}`,
		`{"check":"unknown-check","verdict":"PASS","findings":[]}`,
	}
	var structuredCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request struct {
			ResponseFormat json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode model request: %v", err)
		}
		if len(request.ResponseFormat) == 0 {
			t.Error("MCP generic check did not use schema-constrained output")
		}
		call := structuredCalls.Add(1) - 1
		if int(call) >= len(responses) {
			t.Errorf("unexpected model call %d", call+1)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": responses[call]}}},
		})
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version":1,"verifier":{"model":"openai-completions/test-model"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWORN_CONFIG_PATH", configPath)
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("SWORN_OPENAI_COMPLETIONS_BASE_URL", server.URL)

	s := New()
	RegisterLintTools(s, fr.Root)
	for _, tt := range []struct {
		name          string
		rawCheckMatch string
	}{
		{name: "wrong known identity", rawCheckMatch: `\"check\":\"design-review\"`},
		{name: "missing identity", rawCheckMatch: `\"verdict\":\"PASS\"`},
		{name: "unknown identity", rawCheckMatch: `\"check\":\"unknown-check\"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := callRegisteredTool(s, "sworn.llm_check", json.RawMessage(`{"release":"2026-07-17-mcp-identity","slice_id":"S01-test","type":"ac-satisfaction","base":"HEAD"}`))
			if err != nil {
				t.Fatalf("registered MCP handler: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("invalid identity must produce MCP non-success: %+v", result)
			}
			if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, tt.rawCheckMatch) {
				t.Fatalf("MCP handler lost or relabelled raw identity: %+v", result)
			}
		})
	}
	if structuredCalls.Load() != int32(len(responses)) {
		t.Fatalf("structured output calls = %d, want %d", structuredCalls.Load(), len(responses))
	}
}

// TestMCPGenericMaintainabilityReviewRetiredWithoutDispatch ensures the
// registered public handler stops before release/model/diff work even when the
// inputs would otherwise make those steps fail.
func TestMCPGenericMaintainabilityReviewRetiredWithoutDispatch(t *testing.T) {
	fr := setupFixtureRelease(t, "2026-07-17-mcp-retired")
	fr.writeSlice(t, "S01-test", "# Slice: S01-test\n")
	before := mcpFixtureTreeSnapshot(t, fr.Root)
	var modelCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv("SWORN_CONFIG_PATH", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("SWORN_OPENAI_COMPLETIONS_BASE_URL", server.URL)

	s := New()
	RegisterLintTools(s, fr.Root)
	result, err := callRegisteredTool(s, "sworn.llm_check", json.RawMessage(`{"release":"2026-07-17-mcp-retired","slice_id":"S01-test","type":"maintainability-review","model":"openai-completions/test-model","base":"definitely-not-a-ref"}`))
	if err != nil {
		t.Fatalf("retired MCP check returned transport error: %v", err)
	}
	if result == nil || !result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "use sworn maintainability review") {
		t.Fatalf("retired MCP check did not return dedicated non-success guidance: %+v", result)
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("retired MCP check dispatched %d model calls, want 0", modelCalls.Load())
	}
	if after := mcpFixtureTreeSnapshot(t, fr.Root); after != before {
		t.Fatalf("retired MCP check mutated the fixture tree\nbefore: %q\nafter:  %q", before, after)
	}
}

func mcpFixtureTreeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel)+"\x00"+string(contents))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return strings.Join(files, "\n")
}

// TestLintTools_LintTraceWithFixture verifies that sworn.lint_trace runs against
// a fixture release and returns a structured JSON result.
func TestLintTools_LintTraceWithFixture(t *testing.T) {
	fr := setupFixtureRelease(t, "2026-06-19-test-trace")

	// Write a minimal spec with a valid EARS AC.
	fr.writeSlice(t, "S99-dummy", `---
title: Test slice
---

# Slice: S99-dummy

## Acceptance checks

- [ ] AC1: WHEN the user clicks the button THE system SHALL respond within 100ms.
`)
	fr.writeStatus(t, "S99-dummy", `"state": "planned"`)

	// Write a minimal intake.md — required by trace.
	intakePath := filepath.Join(fr.Dir, "intake.md")
	if err := os.WriteFile(intakePath, []byte("# Release Intake\n\n## Release goal\n\nTest release.\n"), 0644); err != nil {
		t.Fatalf("write intake.md: %v", err)
	}

	// Write an index.md with frontmatter that the board parser expects.
	indexContent := fmt.Sprintf(`---title: test
release_worktree_path: %s
release_worktree_branch: release-wt/2026-06-19-test-trace
---

# Release Board

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|---|
| S99-dummy | T1-test | Test | planned | test | spec.md | — |
`, fr.Root)
	fr.writeIndex(t, indexContent)

	writeLintBoardJSON(t, fr.Dir, "2026-06-19-test-trace", map[string][]string{
		"T1-test": {"S99-dummy"},
	}, fr.Root)

	s := New()
	RegisterLintTools(s, fr.Root)

	params := json.RawMessage(`{"release": "2026-06-19-test-trace"}`)
	result, err := callRegisteredTool(s, "sworn.lint_trace", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Parse the JSON result.
	var report map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &report); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	// Basic structural assertions.
	if _, ok := report["verdict"]; !ok {
		t.Error("result missing verdict field")
	}
	if _, ok := report["total_acs_checked"]; !ok {
		t.Error("result missing total_acs_checked field")
	}
}

// TestLintTools_CompositeWithSlice verifies that sworn.lint runs per-slice
// checks when a slice_id is provided.
func TestLintTools_CompositeWithSlice(t *testing.T) {
	fr := setupFixtureRelease(t, "2026-06-19-test-comp")

	fr.writeSlice(t, "S99-dummy", `---
title: Test slice
---

# Slice: S99-dummy

## Acceptance checks

- [ ] AC1: WHEN the user clicks the button THE system SHALL respond within 100ms.
`)
	fr.writeStatus(t, "S99-dummy", `"state": "planned", "planned_files": ["internal/mcp/lint.go"]`)

	indexContent := fmt.Sprintf(`---
title: test
release_worktree_path: %s
release_worktree_branch: release-wt/2026-06-19-test-comp
---

# Release Board

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|---|
| S99-dummy | T1-test | Test | planned | test | spec.md | — |
`, fr.Root)
	fr.writeIndex(t, indexContent)

	writeLintBoardJSON(t, fr.Dir, "2026-06-19-test-comp", map[string][]string{
		"T1-test": {"S99-dummy"},
	}, fr.Root)

	s := New()
	RegisterLintTools(s, fr.Root)

	params := json.RawMessage(`{"release": "2026-06-19-test-comp", "slice_id": "S99-dummy"}`)
	result, err := callRegisteredTool(s, "sworn.lint", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &report); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	if _, ok := report["verdict"]; !ok {
		t.Error("result missing verdict field")
	}
	if _, ok := report["checks"]; !ok {
		t.Error("result missing checks field")
	}

	// At minimum, the release-level checks (ac, trace, status) should be present.
	checks, ok := report["checks"].(map[string]any)
	if !ok {
		t.Fatal("checks is not a map")
	}
	for _, name := range []string{"ac", "trace", "status"} {
		if _, ok := checks[name]; !ok {
			t.Errorf("composite result missing release-level check %q", name)
		}
	}
}

// TestLintTools_CompositeReleaseOnly verifies that sworn.lint runs only
// release-level checks when no slice_id is provided.
func TestLintTools_CompositeReleaseOnly(t *testing.T) {
	fr := setupFixtureRelease(t, "2026-06-19-test-relonly")
	fr.writeSlice(t, "S99-dummy", `---
title: Test slice
---

# Slice: S99-dummy

## Acceptance checks

- [ ] AC1: WHEN the user clicks the button THE system SHALL respond within 100ms.
`)
	fr.writeStatus(t, "S99-dummy", `"state": "planned"`)

	indexContent := fmt.Sprintf(`---
title: test
release_worktree_path: %s
release_worktree_branch: release-wt/2026-06-19-test-relonly
---

# Release Board

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|---|
| S99-dummy | T1-test | Test | planned | test | spec.md | — |
`, fr.Root)
	fr.writeIndex(t, indexContent)

	writeLintBoardJSON(t, fr.Dir, "2026-06-19-test-relonly", map[string][]string{
		"T1-test": {"S99-dummy"},
	}, fr.Root)

	s := New()
	RegisterLintTools(s, fr.Root)

	params := json.RawMessage(`{"release": "2026-06-19-test-relonly"}`)
	result, err := callRegisteredTool(s, "sworn.lint", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &report); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	checks, ok := report["checks"].(map[string]any)
	if !ok {
		t.Fatal("checks is not a map")
	}

	// Release-only: should have ac, trace, status but NOT per-slice checks.
	for _, name := range []string{"ac", "trace", "status"} {
		if _, ok := checks[name]; !ok {
			t.Errorf("composite result missing release-level check %q", name)
		}
	}
	// Per-slice checks should be absent.
	for _, name := range []string{"coverage", "design", "mock", "deps", "touchpoints", "symbols"} {
		if _, ok := checks[name]; ok {
			t.Errorf("composite result should NOT have per-slice check %q when no slice_id", name)
		}
	}

	// Verify the verdict is present.
	if _, ok := report["verdict"]; !ok {
		t.Error("result missing verdict field")
	}
}

// callRegisteredTool is a test helper that calls a named tool on a server with the given params.
func callRegisteredTool(s *Server, name string, params json.RawMessage) (*ToolResult, error) {
	s.mu.Lock()
	handler, ok := s.tools[name]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return handler(context.Background(), params)
}

// writeLintBoardJSON writes a board.json fixture for lint tests. This replaces
// the legacy tracks: YAML hand-write in index.md with the current (ADR-0009)
// board.json format — the same source of truth the renderer consumes.
func writeLintBoardJSON(t *testing.T, releaseDir, releaseName string, trackSlices map[string][]string, worktreeRoot string) {
	t.Helper()
	var tracks []board.BoardTrack
	for trackID, slices := range trackSlices {
		tracks = append(tracks, board.BoardTrack{
			ID:     trackID,
			Slices: slices,
		})
	}
	_ = worktreeRoot // pure-plan board: worktree paths are derived, not persisted
	br := &board.BoardRecord{
		Release: board.StringRelease(releaseName),
		Tracks:  tracks,
	}
	data, err := json.MarshalIndent(br, "", "  ")
	if err != nil {
		t.Fatalf("marshal board.json: %v", err)
	}
	boardPath := filepath.Join(releaseDir, "board.json")
	if err := os.WriteFile(boardPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
}
