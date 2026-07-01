package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

// ---- Plan-tool round-trip helpers ----

func planToolRoundTrip(t *testing.T, repoRoot string) (stdinWriter io.Writer, stdoutReader *bufio.Reader, cleanup func()) {
	t.Helper()
	w, r, s := testRoundTrip(t)
	RegisterPlanTools(s, repoRoot)
	RegisterResources(s, repoRoot)
	RegisterPrompts(s)
	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)
	return w, r, func() {}
}

// ---- CreateRelease (internal function) ----

func TestCreateRelease(t *testing.T) {
	root := t.TempDir()
	paths, err := CreateRelease(root, "test-mcp-release", "test goal", "#123")
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}

	// Directory structure
	for _, key := range []string{"dir", "index", "intake", "activity", "attributes"} {
		if paths[key] == "" {
			t.Errorf("CreateRelease paths[%q] is empty", key)
		}
	}

	// intake.md contains the goal
	intakeData, err := os.ReadFile(paths["intake"])
	if err != nil {
		t.Fatalf("read intake.md: %v", err)
	}
	if !strings.Contains(string(intakeData), "test goal") {
		t.Errorf("intake.md missing goal, got: %s", string(intakeData))
	}

	// index.md exists and has frontmatter
	indexData, err := os.ReadFile(paths["index"])
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	if !strings.HasPrefix(string(indexData), "---") {
		t.Errorf("index.md missing frontmatter")
	}
	if !strings.Contains(string(indexData), "test-mcp-release") {
		t.Errorf("index.md missing release name")
	}

	// screenshots/.gitkeep exists
	gitkeep := filepath.Join(paths["dir"], "screenshots", ".gitkeep")
	if _, err := os.Stat(gitkeep); err != nil {
		t.Errorf("screenshots/.gitkeep not created: %v", err)
	}

	// activity.md exists
	if _, err := os.ReadFile(paths["activity"]); err != nil {
		t.Errorf("activity.md not created: %v", err)
	}

	// .gitattributes exists
	if _, err := os.ReadFile(paths["attributes"]); err != nil {
		t.Errorf(".gitattributes not created: %v", err)
	}
}

// ---- create_slice tool ----

func TestCreateSlice(t *testing.T) {
	root := t.TempDir()
	// Create the release directory first
	if err := os.MkdirAll(filepath.Join(root, "docs", "release", "test-rel"), 0755); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-rel", "slice_id": "S01-foo", "spec_content": "# spec content", "track_id": "T1"}`)
	result := callTool(t, w, r, "create_slice", args)

	if result.IsError {
		t.Fatalf("create_slice returned error: %s", result.Content[0].Text)
	}

	// Verify spec.md
	specPath := filepath.Join(root, "docs", "release", "test-rel", "S01-foo", "spec.md")
	specData, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec.md: %v", err)
	}
	if !strings.Contains(string(specData), "# spec content") {
		t.Errorf("spec.md content mismatch: %s", string(specData))
	}

	// Verify status.json
	statusPath := filepath.Join(root, "docs", "release", "test-rel", "S01-foo", "status.json")
	s, err := state.Read(statusPath)
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}
	if string(s.State) != "planned" {
		t.Errorf("status.state = %q, want %q", s.State, "planned")
	}
	if s.Track != "T1" {
		t.Errorf("status.track = %q, want %q", s.Track, "T1")
	}
}

func TestCreateSliceDuplicate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "release", "test-rel"), 0755); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	// First call succeeds
	args := json.RawMessage(`{"release": "test-rel", "slice_id": "S01-dup", "spec_content": "# spec", "track_id": "T1"}`)
	result := callTool(t, w, r, "create_slice", args)
	if result.IsError {
		t.Fatalf("first create_slice should succeed: %s", result.Content[0].Text)
	}

	// Second call with same slice_id should error
	result2 := callTool(t, w, r, "create_slice", args)
	if !result2.IsError {
		t.Errorf("second create_slice with same id should return error")
	}
	if !strings.Contains(result2.Content[0].Text, "already exists") {
		t.Errorf("error message should mention 'already exists', got: %s", result2.Content[0].Text)
	}
}

// ---- set_track tool ----

func TestSetTrackValidation(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write a minimal index.md
	indexContent := `---
title: 'test'
tracks: []
---

# Board
`
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	// set_track with non-existent slice_id
	args := json.RawMessage(`{"release": "test-rel", "track_id": "T1", "slices": ["S01-nonexistent"]}`)
	result := callTool(t, w, r, "set_track", args)
	if !result.IsError {
		t.Errorf("set_track with non-existent slice should return error")
	}
	if !strings.Contains(result.Content[0].Text, "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %s", result.Content[0].Text)
	}
}

func TestSetTrackUpdates(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a slice directory so validation passes
	sliceDir := filepath.Join(releaseDir, "S01-foo")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a minimal index.md with a Tracks table
	indexContent := `---
title: 'test'
tracks: []
---

# Board

## Tracks

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|

## Slices
`
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-rel", "track_id": "T1-core", "slices": ["S01-foo"]}`)
	result := callTool(t, w, r, "set_track", args)
	if result.IsError {
		t.Fatalf("set_track should succeed: %s", result.Content[0].Text)
	}

	// Verify board.json was updated (S04-mcp-oracle-migration: oracle source of truth)
	updatedData, err := os.ReadFile(filepath.Join(releaseDir, "board.json"))
	if err != nil {
		t.Fatal(err)
	}
	updated := string(updatedData)
	if !strings.Contains(updated, "T1-core") {
		t.Errorf("board.json should contain track T1-core")
	}
	if !strings.Contains(updated, "S01-foo") {
		t.Errorf("board.json should contain slice S01-foo")
	}
}

func TestSetTrackColon(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a slice directory with a colon in the name
	sliceDir := filepath.Join(releaseDir, "S01-colon: space")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}

	indexContent := `---
title: 'test'
tracks: []
---

# Board

## Tracks

| Track | Slices (in order) | Depends on | Branch | State |
|---|---|---|---|---|
`
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-rel", "track_id": "T1", "slices": ["S01-colon: space"]}`)
	result := callTool(t, w, r, "set_track", args)
	if result.IsError {
		t.Fatalf("set_track with colon-space slice should succeed: %s", result.Content[0].Text)
	}

	// Verify the board.json is valid — the track entry should be present
	updatedData, err := os.ReadFile(filepath.Join(releaseDir, "board.json"))
	if err != nil {
		t.Fatal(err)
	}
	updated := string(updatedData)
	if !strings.Contains(updated, "T1") {
		t.Errorf("board.json should contain track T1")
	}
	// The colon-space slice should appear in the board.json slices list
	if !strings.Contains(updated, "S01-colon") {
		t.Errorf("board.json should contain the slice id with colon")
	}
}

// ---- update_intake tool ----

func TestUpdateIntakeAppends(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	intakeContent := `# Intake

## Release goal

Original goal.

## Users

- User A
`
	if err := os.WriteFile(filepath.Join(releaseDir, "intake.md"), []byte(intakeContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	// First append
	args1 := json.RawMessage(`{"release": "test-rel", "section": "Release goal", "content": "First addition."}`)
	result1 := callTool(t, w, r, "update_intake", args1)
	if result1.IsError {
		t.Fatalf("first update_intake should succeed: %s", result1.Content[0].Text)
	}

	// Second append to same section
	args2 := json.RawMessage(`{"release": "test-rel", "section": "Release goal", "content": "Second addition."}`)
	result2 := callTool(t, w, r, "update_intake", args2)
	if result2.IsError {
		t.Fatalf("second update_intake should succeed: %s", result2.Content[0].Text)
	}

	// Verify both contents are present and order preserved
	data, err := os.ReadFile(filepath.Join(releaseDir, "intake.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "First addition.") {
		t.Errorf("intake.md missing first addition")
	}
	if !strings.Contains(content, "Second addition.") {
		t.Errorf("intake.md missing second addition")
	}
	// Order: first should come before second
	idx1 := strings.Index(content, "First addition.")
	idx2 := strings.Index(content, "Second addition.")
	if idx1 < 0 || idx2 < 0 || idx1 > idx2 {
		t.Errorf("order not preserved: first at %d, second at %d", idx1, idx2)
	}
}

func TestUpdateIntakeCreatesSection(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	intakeContent := `# Intake

## Existing section

Some content.
`
	if err := os.WriteFile(filepath.Join(releaseDir, "intake.md"), []byte(intakeContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	args := json.RawMessage(`{"release": "test-rel", "section": "New section", "content": "New content."}`)
	result := callTool(t, w, r, "update_intake", args)
	if result.IsError {
		t.Fatalf("update_intake should succeed: %s", result.Content[0].Text)
	}

	data, err := os.ReadFile(filepath.Join(releaseDir, "intake.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "## New section") {
		t.Errorf("intake.md should have new section heading")
	}
	if !strings.Contains(content, "New content.") {
		t.Errorf("intake.md should have new content")
	}
	// Existing content should be preserved
	if !strings.Contains(content, "Some content.") {
		t.Errorf("intake.md should still have existing content")
	}
}

// ---- Resource read tests ----

func TestResourceReadPrompt(t *testing.T) {
	root := t.TempDir()
	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	// Read sworn://prompts/plan via resources/read
	params := json.RawMessage(`{"uri": "sworn://prompts/plan"}`)
	sendRequest(t, w, "resources/read", jsonID(10), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read returned error: %s", resp["error"])
	}

	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal resources/read result: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("no contents in resources/read result")
	}
	if result.Contents[0].Text == "" {
		t.Errorf("sworn://prompts/plan returned empty content — embed should be non-empty")
	}
}

func TestResourceReadBatonVersion(t *testing.T) {
	root := t.TempDir()
	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"uri": "sworn://baton/version"}`)
	sendRequest(t, w, "resources/read", jsonID(11), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read returned error: %s", resp["error"])
	}

	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 || result.Contents[0].Text == "" {
		t.Errorf("sworn://baton/version returned empty content")
	}
	// Should be a parseable version string (starts with v or a digit)
	text := strings.TrimSpace(result.Contents[0].Text)
	if text == "" {
		t.Errorf("version string is empty")
	}
}

func TestResourceReadReleaseBoard(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "2026-06-19-safe-parallelism")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	boardContent := "# Release Board\n\nTest board content."
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(boardContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"uri": "sworn://release/2026-06-19-safe-parallelism/board"}`)
	sendRequest(t, w, "resources/read", jsonID(12), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read returned error: %s", resp["error"])
	}

	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("no contents")
	}
	if !strings.Contains(result.Contents[0].Text, "Test board content") {
		t.Errorf("board resource missing expected content, got: %s", result.Contents[0].Text)
	}
}

func TestResourceReadProofAbsent(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	sliceDir := filepath.Join(releaseDir, "S01-noproof")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write spec.md so the slice exists, but no proof.md
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte("# spec"), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"uri": "sworn://release/test-rel/S01-noproof/proof"}`)
	sendRequest(t, w, "resources/read", jsonID(13), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read for absent proof should not error: %s", resp["error"])
	}

	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("no contents")
	}
	if result.Contents[0].Text != "" {
		t.Errorf("absent proof should return empty string, got: %q", result.Contents[0].Text)
	}
}

func TestResourceReadSliceSpec(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	sliceDir := filepath.Join(releaseDir, "S01-spec")
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := "# S01-spec\n\nThe spec content."
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"uri": "sworn://release/test-rel/S01-spec/spec"}`)
	sendRequest(t, w, "resources/read", jsonID(14), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read returned error: %s", resp["error"])
	}

	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("no contents")
	}
	if !strings.Contains(result.Contents[0].Text, "The spec content") {
		t.Errorf("spec resource missing expected content, got: %s", result.Contents[0].Text)
	}
}

// ---- Prompt tests ----

func TestPromptsGetPlanner(t *testing.T) {
	root := t.TempDir()
	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"name": "planner"}`)
	sendRequest(t, w, "prompts/get", jsonID(20), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("prompts/get returned error: %s", resp["error"])
	}

	var result promptsGetResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("no messages in prompts/get result")
	}
	if result.Messages[0].Content.Text == "" {
		t.Errorf("planner prompt returned empty content")
	}
}

func TestPromptsGetImplementer(t *testing.T) {
	root := t.TempDir()
	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"name": "implementer"}`)
	sendRequest(t, w, "prompts/get", jsonID(21), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("prompts/get returned error: %s", resp["error"])
	}

	var result promptsGetResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Messages) == 0 || result.Messages[0].Content.Text == "" {
		t.Errorf("implementer prompt returned empty content")
	}
}

func TestPromptsGetVerifier(t *testing.T) {
	root := t.TempDir()
	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"name": "verifier"}`)
	sendRequest(t, w, "prompts/get", jsonID(22), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("prompts/get returned error: %s", resp["error"])
	}

	var result promptsGetResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Messages) == 0 || result.Messages[0].Content.Text == "" {
		t.Errorf("verifier prompt returned empty content")
	}
}

func TestPromptsListEnumerates(t *testing.T) {
	root := t.TempDir()
	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	sendRequest(t, w, "prompts/list", jsonID(23), nil)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("prompts/list returned error: %s", resp["error"])
	}

	var result promptsListResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Prompts) == 0 {
		t.Fatal("prompts/list should enumerate registered prompts")
	}
	// Should contain planner, implementer, verifier
	names := make(map[string]bool)
	for _, p := range result.Prompts {
		names[p.Name] = true
	}
	for _, expected := range []string{"planner", "implementer", "verifier"} {
		if !names[expected] {
			t.Errorf("prompts/list missing %q", expected)
		}
	}
}

// ---- Resource: track-mode ----

func TestResourceReadTrackMode(t *testing.T) {
	root := t.TempDir()
	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"uri": "sworn://baton/track-mode"}`)
	sendRequest(t, w, "resources/read", jsonID(30), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read returned error: %s", resp["error"])
	}

	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 || result.Contents[0].Text == "" {
		t.Errorf("sworn://baton/track-mode returned empty content")
	}
}

// ---- Resource: intake ----

func TestResourceReadIntake(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "docs", "release", "test-rel")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	intakeContent := "# Intake\n\nIntake content here."
	if err := os.WriteFile(filepath.Join(releaseDir, "intake.md"), []byte(intakeContent), 0644); err != nil {
		t.Fatal(err)
	}

	w, r, cleanup := planToolRoundTrip(t, root)
	defer cleanup()

	params := json.RawMessage(`{"uri": "sworn://release/test-rel/intake"}`)
	sendRequest(t, w, "resources/read", jsonID(31), params)
	resp := readResponse(t, r)

	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read returned error: %s", resp["error"])
	}

	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("no contents")
	}
	if !strings.Contains(result.Contents[0].Text, "Intake content here") {
		t.Errorf("intake resource missing expected content")
	}
}

// Suppress unused import in case context is not directly referenced
var _ = context.Background
