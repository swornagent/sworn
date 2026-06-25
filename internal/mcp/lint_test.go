package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
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
		t.Errorf("expected error for nonexistent release, got result: %v", result)	}
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
tracks:
  - id: T1-test
    slices: [S99-dummy]
    depends_on: null
    worktree_path: `+fr.Root+`
    worktree_branch: track/2026-06-19-test-release/T1-test
    state: planned
`)

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
tracks:
  - id: T1-test
    slices: [S99-dummy]
    depends_on: null
    worktree_path: `+fr.Root+`
    worktree_branch: track/2026-06-19-test-release/T1-test
    state: planned
`)

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
tracks:
  - id: T1-test
    slices: [S99-dummy]
    depends_on: null
    worktree_path: %s
    worktree_branch: track/2026-06-19-test-trace/T1-test
    state: planned
---

# Release Board

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| S99-dummy | T1-test | Test | planned | test | spec.md | — |
`, fr.Root, fr.Root)
	fr.writeIndex(t, indexContent)

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
tracks:
  - id: T1-test
    slices: [S99-dummy]
    depends_on: null
    worktree_path: %s
    worktree_branch: track/2026-06-19-test-comp/T1-test
    state: planned
---

# Release Board

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| S99-dummy | T1-test | Test | planned | test | spec.md | — |
`, fr.Root, fr.Root)
	fr.writeIndex(t, indexContent)

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
tracks:
  - id: T1-test
    slices: [S99-dummy]
    depends_on: null
    worktree_path: %s
    worktree_branch: track/2026-06-19-test-relonly/T1-test
    state: planned
---

# Release Board

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| S99-dummy | T1-test | Test | planned | test | spec.md | — |
`, fr.Root, fr.Root)
	fr.writeIndex(t, indexContent)

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