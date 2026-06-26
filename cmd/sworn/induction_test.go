package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Phase 0 tests
// ---------------------------------------------------------------------------

func TestInductionPhase0ReadsGoMod(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Write a go.mod with a require block.
	goMod := `module example.com/test

go 1.21

require (
	github.com/foo/bar v1.2.3
	github.com/baz/qux v0.5.0
)

require github.com/solo/pkg v2.0.0
`
	if err := os.WriteFile("go.mod", []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Create docs/templates/considerations.md for the template.
	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmpl := `---
version: 1
project: test
design_system:
  location: ''
  framework: ''
  component_library: ''
architecture:
  language: ''
  patterns: []
enabled_dimensions: [security]
---

# Project Considerations

## [dependencies]

project_pinned:
  # Example entries

## [security]

required_for: all
`
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// Run phase 0.
	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}
	phase0DependencyDiscovery(catalogPath)

	// Read the catalog and check project_pinned.
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	if !strings.Contains(content, "github.com/foo/bar") {
		t.Error("expected github.com/foo/bar in catalog")
	}
	if !strings.Contains(content, "v1.2.3") {
		t.Error("expected v1.2.3 in catalog")
	}
	if !strings.Contains(content, "github.com/baz/qux") {
		t.Error("expected github.com/baz/qux in catalog")
	}
}

func TestInductionPhase0NoDepsFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create the template.
	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte("---\nenabled_dimensions: []\n---\n\n## [dependencies]\nproject_pinned:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}

	// Should not error even with no dependency files.
	phase0DependencyDiscovery(catalogPath)

	b, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	if !strings.Contains(content, "project_pinned:") {
		t.Error("expected project_pinned section in catalog")
	}
}

func TestInductionPhase0UpdateAppends(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// First run: create catalog with one pinned dep already present.
	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmpl := `---
version: 1
enabled_dimensions: []
---
## [dependencies]
project_pinned:
  - module: github.com/existing/pkg
    version: v1.0.0
    pinned_by: go.mod
`
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}

	// Now write go.mod with both the existing dep and a new one.
	goMod := `module example.com/test

go 1.21

require (
	github.com/existing/pkg v1.0.0
	github.com/new/pkg v2.0.0
)
`
	if err := os.WriteFile("go.mod", []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	phase0DependencyDiscovery(catalogPath)

	b, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	// New dep should be present.
	if !strings.Contains(content, "github.com/new/pkg") {
		t.Error("expected new dep github.com/new/pkg to be appended")
	}

	// Existing dep should not be duplicated.
	count := strings.Count(content, "github.com/existing/pkg")
	if count > 1 {
		t.Errorf("existing dep duplicated: found %d occurrences", count)
	}
}

// ---------------------------------------------------------------------------
// Phase 1 tests
// ---------------------------------------------------------------------------

func TestInductionWritesDesignSystem(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create template and catalog.
	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte("---\ndesign_system:\n  location: ''\n  framework: ''\n  component_library: ''\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}

	// Simulate piped stdin: framework=shadcn, location=https://ui.shadcn.com, comp_lib=@repo/ui
	input := "y\nshadcn\nhttps://ui.shadcn.com\n@repo/ui\n"
	tmpIn, err := os.CreateTemp("", "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpIn.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := tmpIn.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = tmpIn
	defer func() { os.Stdin = oldStdin }()

	phase1DesignSystem(catalogPath)

	b, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	if !strings.Contains(content, "shadcn") {
		t.Error("expected framework 'shadcn' in catalog")
	}
	if !strings.Contains(content, "https://ui.shadcn.com") {
		t.Error("expected location in catalog")
	}
}

// ---------------------------------------------------------------------------
// Phase 2 tests
// ---------------------------------------------------------------------------

func TestInductionWritesPatterns(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create a catalog with empty patterns.
	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmpl := `---
architecture:
  language: ''
  patterns: []
enabled_dimensions: []
---
`
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a file with an interface in internal/model/
	modelDir := filepath.Join("internal", "model")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "client.go"), []byte("package model\n\ntype Client interface {\n\tDo() error\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}

	// Simulate piped stdin: accept all (empty = default = y).
	input := "\n"
	tmpIn, err := os.CreateTemp("", "stdin")
	if err != nil {
		t.Fatal(err)
	}
	tmpIn.WriteString(input)
	tmpIn.Seek(0, 0)
	oldStdin := os.Stdin
	os.Stdin = tmpIn
	defer func() { os.Stdin = oldStdin }()

	phase2ArchitecturePatterns(catalogPath, false, false)

	b, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	if !strings.Contains(content, "interface-first design") {
		t.Error("expected 'interface-first design' pattern in catalog")
	}
}

func TestInductionSkipPath(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmpl := `---
architecture:
  language: ''
  patterns: []
enabled_dimensions: []
---
`
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a file with no patterns to infer (no interface, no stdlib HTTP, no table-driven tests).
	modelDir := filepath.Join("internal", "model")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "empty.go"), []byte("package model\n\nfunc Foo() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}

	// phase2ArchitecturePatterns with no patterns inferred — should not error, empty catalog stays empty.
	phase2ArchitecturePatterns(catalogPath, false, false)

	b, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	// Should still have empty patterns.
	if strings.Contains(string(b), "interface-first") {
		t.Error("catalog should not contain patterns when none were inferred")
	}
}

func TestInductionIdempotent(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmpl := `---
architecture:
  language: ''
  patterns:
    - pattern: existing-pattern
      location: some/file.go
      intent: already here
enabled_dimensions: []
---
`
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}

	// Read patterns from the catalog — should get the existing one.
	patterns, _ := readPatternsFromCatalog(catalogPath)
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].Pattern != "existing-pattern" {
		t.Errorf("expected 'existing-pattern', got %q", patterns[0].Pattern)
	}

	// Simulate writing the same pattern again — should not duplicate.
	writePatterns(catalogPath, []pattern{{Pattern: "existing-pattern", Location: "some/file.go", Intent: "already here"}})

	patterns2, _ := readPatternsFromCatalog(catalogPath)
	if len(patterns2) != 1 {
		t.Errorf("expected 1 pattern after idempotent write, got %d", len(patterns2))
	}
}

func TestInductionUpdateShowsOnlyNew(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	tmplDir := filepath.Join("docs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	tmpl := `---
architecture:
  language: ''
  patterns:
    - pattern: interface-first design
      location: internal/model/client.go
      intent: enables mock injection
enabled_dimensions: []
---
`
	if err := os.WriteFile(filepath.Join(tmplDir, "considerations.md"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	// Create files so that pattern inference works.
	modelDir := filepath.Join("internal", "model")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "client.go"), []byte("package model\nimport \"net/http\"\n\ntype Client interface {\n\tDo() error\n}\nfunc (c *client) Do() error { return nil }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	catalogPath := considerationsPath()
	if err := initializeCatalog(catalogPath); err != nil {
		t.Fatal(err)
	}

	// In update mode, with "interface-first design" already present, it should
	// skip it and only show new patterns.
	// We run phase2ArchitecturePatterns in update mode (second arg true).
	// Since "interface-first design" already exists, it shouldn't be proposed again.
	// But "stdlib HTTP" might be found.
	input := "y\n"
	tmpIn, err := os.CreateTemp("", "stdin")
	if err != nil {
		t.Fatal(err)
	}
	tmpIn.WriteString(input)
	tmpIn.Seek(0, 0)
	oldStdin := os.Stdin
	os.Stdin = tmpIn
	defer func() { os.Stdin = oldStdin }()

	phase2ArchitecturePatterns(catalogPath, true, false)

	patterns, _ := readPatternsFromCatalog(catalogPath)
	// Should not have duplicated interface-first design.
	count := 0
	for _, p := range patterns {
		if p.Pattern == "interface-first design" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("interface-first design duplicated: %d occurrences", count)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for parseGoMod
// ---------------------------------------------------------------------------

func TestParseGoMod_RequireBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.mod")
	content := `module example.com/test

go 1.21

require (
	github.com/foo/bar v1.2.3
	github.com/baz/qux v0.5.0
)
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	deps := parseGoMod(path)
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
	if deps[0].module != "github.com/foo/bar" || deps[0].version != "v1.2.3" {
		t.Errorf("unexpected dep[0]: %s %s", deps[0].module, deps[0].version)
	}
	if deps[1].module != "github.com/baz/qux" || deps[1].version != "v0.5.0" {
		t.Errorf("unexpected dep[1]: %s %s", deps[1].module, deps[1].version)
	}
}

func TestParseGoMod_SingleRequire(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.mod")
	content := `module example.com/test

go 1.21

require github.com/solo/pkg v2.0.0
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	deps := parseGoMod(path)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].module != "github.com/solo/pkg" || deps[0].version != "v2.0.0" {
		t.Errorf("unexpected dep: %s %s", deps[0].module, deps[0].version)
	}
}

func TestParseGoMod_NoGoMod(t *testing.T) {
	deps := parseGoMod("/nonexistent/go.mod")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestFindInterfacePattern(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// No internal/model dir -> should return false.
	found, _ := findInterfacePattern()
	if found {
		t.Error("expected false when internal/model doesn't exist")
	}

	// Create internal/model with an interface.
	modelDir := filepath.Join("internal", "model")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "client.go"), []byte("package model\n\ntype Client interface {\n\tDo() error\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	found, loc := findInterfacePattern()
	if !found {
		t.Error("expected true when interface exists")
	}
	if loc != filepath.Join("internal/model", "client.go") {
		t.Errorf("unexpected location: %s", loc)
	}
}
