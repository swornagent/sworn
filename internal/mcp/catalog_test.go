package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// callCatalogTool invokes a registered catalog tool by name and unmarshals the result text.
func callCatalogTool(t *testing.T, s *Server, name string, params map[string]any) string {	t.Helper()
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	s.mu.Lock()
	handler, ok := s.tools[name]
	s.mu.Unlock()
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	result, err := handler(context.Background(), paramsJSON)
	if err != nil {
		t.Fatalf("tool %q error: %v", name, err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("tool %q returned no content", name)
	}
	return result.Content[0].Text
}

// helper: register all catalog tools with a temp repo root.
func newCatalogServer(t *testing.T, repoRoot string) *Server {
	t.Helper()
	s := New()
	RegisterCatalogTools(s, repoRoot)
	return s
}

// ---- 1. plan_release ----

func TestPlanReleaseNew(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	result := callCatalogTool(t, s, "plan_release", map[string]any{
		"name": "2026-07-07-test-release",
		"goal": "test goal",
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if exists, _ := parsed["exists"].(bool); exists {
		t.Errorf("expected exists=false, got true")
	}
	paths, _ := parsed["created_paths"].([]any)
	if len(paths) == 0 {
		t.Errorf("expected created_paths to be non-empty")
	}
	// Verify files exist on disk.
	releaseDir := filepath.Join(dir, "docs", "release", "2026-07-07-test-release")
	if _, err := os.Stat(releaseDir); err != nil {
		t.Errorf("release directory not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(releaseDir, "index.md")); err != nil {
		t.Errorf("index.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(releaseDir, "intake.md")); err != nil {
		t.Errorf("intake.md not created: %v", err)
	}
}

func TestPlanReleaseExisting(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	// First call creates the release.
	callCatalogTool(t, s, "plan_release", map[string]any{
		"name": "2026-07-07-test-existing",
		"goal": "test goal",
	})

	// Second call should detect existing.
	result := callCatalogTool(t, s, "plan_release", map[string]any{
		"name": "2026-07-07-test-existing",
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if exists, _ := parsed["exists"].(bool); !exists {
		t.Errorf("expected exists=true, got false")
	}
	// The state_summary should be present.
	if _, ok := parsed["state_summary"]; !ok {
		t.Errorf("expected state_summary to be present")
	}
}

// ---- 2. get_induction_status ----

func TestGetInductionStatus_Empty(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	result := callCatalogTool(t, s, "get_induction_status", map[string]any{})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if exists, _ := parsed["catalog_exists"].(bool); exists {
		t.Errorf("expected catalog_exists=false, got true")
	}
}

func TestGetInductionStatus_Populated(t *testing.T) {
	dir := t.TempDir()
	// Create a populated considerations.md with 2 architecture patterns.
	catalog := `# Considerations Catalog

## design_system

location: https://storybook.example.com
framework: react

## architecture.patterns

- interface-first (internal/model/client.go): mock injection in tests
- repository-pattern (internal/db/db.go): data access layer

## [security]

Input validation on all endpoints.
`
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	if err := os.WriteFile(filepath.Join(dir, "docs", "considerations.md"), []byte(catalog), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	s := newCatalogServer(t, dir)
	result := callCatalogTool(t, s, "get_induction_status", map[string]any{})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if exists, _ := parsed["catalog_exists"].(bool); !exists {
		t.Errorf("expected catalog_exists=true")
	}
	if ds, _ := parsed["design_system_configured"].(bool); !ds {
		t.Errorf("expected design_system_configured=true")
	}
	pc, _ := parsed["architecture_patterns_count"].(float64)
	if pc != 2 {
		t.Errorf("expected architecture_patterns_count=2, got %v", pc)
	}
}

// ---- 3. get_considerations ----

func TestGetConsiderations_UIType(t *testing.T) {
	dir := t.TempDir()
	catalog := `# Considerations Catalog

## design_system

location: https://storybook.example.com
framework: react

## architecture.patterns

- interface-first (internal/model/client.go): mock injection in tests

## [ui]

### Components
Use shadcn/ui with custom theme.

## [security]

### Auth
Validate all inputs.

## [api]

### Design
REST with JSON.
`
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	if err := os.WriteFile(filepath.Join(dir, "docs", "considerations.md"), []byte(catalog), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	s := newCatalogServer(t, dir)
	result := callCatalogTool(t, s, "get_considerations", map[string]any{"slice_type": "ui"})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	// Should have design_system, architecture.patterns, [ui], [security].
	if _, ok := parsed["design_system"]; !ok {
		t.Errorf("expected design_system in result")
	}
	if _, ok := parsed["[ui]"]; !ok {
		t.Errorf("expected [ui] in result")
	}
	if _, ok := parsed["[security]"]; !ok {
		t.Errorf("expected [security] in result")
	}
	// [api] should NOT be present for slice_type=ui.
	if _, ok := parsed["[api]"]; ok {
		t.Errorf("expected [api] NOT in result for ui type")
	}
}

// ---- 4. search_decisions ----

func TestSearchDecisions_Hit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	// Record a decision, then search for it.
	callCatalogTool(t, s, "record_decision", map[string]any{
		"type":       "design",
		"title":      "Modal for settings",
		"decision":   "Use modal dialogs",
		"rationale":  "prevents layout shift",
		"applies_to": "settings surfaces",
	})

	result := callCatalogTool(t, s, "search_decisions", map[string]any{"keywords": "modal"})
	var entries []map[string]any
	if err := json.Unmarshal([]byte(result), &entries); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(entries) == 0 {
		t.Errorf("expected at least one match for 'modal'")
	}
	// Verify entry contains our decision.
	found := false
	for _, e := range entries {
		if text, ok := e["entry"].(string); ok {
			if strings.Contains(strings.ToLower(text), "modal for settings") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected to find 'Modal for settings' in search results")
	}
}

func TestSearchDecisions_Miss(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	// Record a decision.
	callCatalogTool(t, s, "record_decision", map[string]any{
		"type":       "design",
		"title":      "Modal for settings",
		"decision":   "Use modal dialogs",
		"rationale":  "prevents layout shift",
		"applies_to": "settings surfaces",
	})

	result := callCatalogTool(t, s, "search_decisions", map[string]any{"keywords": "nonexistent"})
	var entries []map[string]any
	if err := json.Unmarshal([]byte(result), &entries); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty array for non-matching keyword, got %d entries", len(entries))
	}
}

func TestSearchDecisions_NoCatalog(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	// No decisions.md exists — should return empty array, no error.
	result := callCatalogTool(t, s, "search_decisions", map[string]any{"keywords": "anything"})
	var entries []map[string]any
	if err := json.Unmarshal([]byte(result), &entries); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty array when no decisions.md, got %d entries", len(entries))
	}
}

// ---- 5. record_decision ----

func TestRecordDecision_WritesEntry(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	result := callCatalogTool(t, s, "record_decision", map[string]any{
		"type":       "design",
		"title":      "Modal for settings",
		"decision":   "Use modal dialogs",
		"rationale":  "prevents layout shift",
		"applies_to": "settings surfaces",
		"release":    "2026-07-07-test",
		"slice":      "S01-test",
	})

	// Check format: should contain ### DESIGN: Modal for settings
	if !strings.Contains(result, "### DESIGN: Modal for settings") {
		t.Errorf("expected '### DESIGN: Modal for settings' heading, got: %s", result)
	}
	if !strings.Contains(result, "- **Decision**: Use modal dialogs") {
		t.Errorf("expected decision field, got: %s", result)
	}
	if !strings.Contains(result, "- **Rationale**: prevents layout shift") {
		t.Errorf("expected rationale field, got: %s", result)
	}
	if !strings.Contains(result, "- **Applies to**: settings surfaces") {
		t.Errorf("expected applies_to field, got: %s", result)
	}
	if !strings.Contains(result, "- **Release**: 2026-07-07-test") {
		t.Errorf("expected release field, got: %s", result)
	}
	if !strings.Contains(result, "- **Slice**: S01-test") {
		t.Errorf("expected slice field, got: %s", result)
	}

	// Verify the file exists on disk.
	decisionsPath := filepath.Join(dir, "docs", "decisions.md")
	data, err := os.ReadFile(decisionsPath)
	if err != nil {
		t.Fatalf("read decisions.md: %v", err)
	}
	if !strings.Contains(string(data), "### DESIGN: Modal for settings") {
		t.Errorf("decisions.md missing expected entry")
	}
}

// ---- 6. check_design_system ----

func TestCheckDesignSystem_Unconfigured(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	// No catalog at all → unconfigured.
	result := callCatalogTool(t, s, "check_design_system", map[string]any{
		"component_description": "data table with sortable columns",
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	status, _ := parsed["status"].(string)
	if status != "unconfigured" {
		t.Errorf("expected status='unconfigured', got '%s'", status)
	}

	// Create a catalog with blank location → also unconfigured.
	catalog := `## design_system

location:
framework:
`
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	if err := os.WriteFile(filepath.Join(dir, "docs", "considerations.md"), []byte(catalog), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	result = callCatalogTool(t, s, "check_design_system", map[string]any{
		"component_description": "data table",
	})
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	status, _ = parsed["status"].(string)
	if status != "unconfigured" {
		t.Errorf("expected status='unconfigured' with blank location, got '%s'", status)
	}

	// Create a catalog with a real location → exists with scaffold.
	catalog = `## design_system

location: https://storybook.example.com
framework: react
component_library: shadcn/ui
`
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	if err := os.WriteFile(filepath.Join(dir, "docs", "considerations.md"), []byte(catalog), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	result = callCatalogTool(t, s, "check_design_system", map[string]any{
		"component_description": "data table",
	})
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	status, _ = parsed["status"].(string)
	if status != "exists" {
		t.Errorf("expected status='exists', got '%s'", status)
	}
	if loc, _ := parsed["location"].(string); loc != "https://storybook.example.com" {
		t.Errorf("expected location='https://storybook.example.com', got '%s'", loc)
	}
	if opts, ok := parsed["options"].([]any); !ok || len(opts) != 3 {
		t.Errorf("expected 3 options, got %v", parsed["options"])
	}
}

// ---- 7. update_design_system ----

func TestUpdateDesignSystem_Writes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	callCatalogTool(t, s, "update_design_system", map[string]any{
		"location":          "https://storybook.example.com",
		"framework":         "storybook",
		"version":           "7.0",
		"component_library": "shadcn/ui",
	})

	// Read file back.
	catalogPath := filepath.Join(dir, "docs", "considerations.md")
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read considerations.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "location: https://storybook.example.com") {
		t.Errorf("expected location in file, got: %s", content)
	}
	if !strings.Contains(content, "framework: storybook") {
		t.Errorf("expected framework in file, got: %s", content)
	}
	if !strings.Contains(content, "version: 7.0") {
		t.Errorf("expected version in file, got: %s", content)
	}
	if !strings.Contains(content, "component_library: shadcn/ui") {
		t.Errorf("expected component_library in file, got: %s", content)
	}

	// get_induction_status should now report design_system_configured=true.
	result := callCatalogTool(t, s, "get_induction_status", map[string]any{})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if ds, _ := parsed["design_system_configured"].(bool); !ds {
		t.Errorf("expected design_system_configured=true after update_design_system")
	}
}

// ---- 8. record_architecture_pattern ----

func TestRecordArchPattern_Idempotent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	s := newCatalogServer(t, dir)

	// First call — should add.
	r1 := callCatalogTool(t, s, "record_architecture_pattern", map[string]any{
		"pattern":  "interface-first",
		"location": "internal/model/client.go",
		"intent":   "mock injection in tests",
	})
	var p1 map[string]any
	if err := json.Unmarshal([]byte(r1), &p1); err != nil {
		t.Fatalf("unmarshal r1: %v", err)
	}
	if added, _ := p1["added"].(bool); !added {
		t.Errorf("expected added=true on first call")
	}

	// Second call with same pattern — should be idempotent.
	r2 := callCatalogTool(t, s, "record_architecture_pattern", map[string]any{
		"pattern":  "interface-first",
		"location": "internal/model/client.go",
		"intent":   "mock injection in tests",
	})
	var p2 map[string]any
	if err := json.Unmarshal([]byte(r2), &p2); err != nil {
		t.Fatalf("unmarshal r2: %v", err)
	}
	if added, _ := p2["added"].(bool); added {
		t.Errorf("expected added=false on duplicate call (idempotent)")
	}

	// Verify file only contains one entry for interface-first.
	catalogPath := filepath.Join(dir, "docs", "considerations.md")
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read considerations.md: %v", err)
	}
	count := strings.Count(string(data), "interface-first")
	if count != 1 {
		t.Errorf("expected exactly 1 'interface-first' entry, found %d", count)
	}
}