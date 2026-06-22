package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupCatalogTemplates creates minimal template files in the temp directory's
// docs/templates/ so that materialiseCatalog can read them.
func setupCatalogTemplates(dir string) {
	os.MkdirAll(filepath.Join(dir, "docs/templates"), 0755)
	os.WriteFile(filepath.Join(dir, "docs/templates/considerations.md"),
		[]byte("# template considerations content\n"), 0644)
	os.WriteFile(filepath.Join(dir, "docs/templates/decisions.md"),
		[]byte("# template decisions content\n"), 0644)
}

// setupMinimalConfig writes a minimal config.json to the given directory so
// that cmdInit sees an existing config and skips the design-system and
// implementer-model interactive prompts (S08/S09 code paths).
func setupMinimalConfig(dir string) {
	os.MkdirAll(dir, 0755)
	cfg := map[string]interface{}{
		"version":     1,
		"ui_bearing":  false,
		"implementer": map[string]interface{}{"model": "openai/gpt-4o-mini"},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(filepath.Join(dir, "config.json"), data, 0600)
}

// feedStdinFromString writes input to a pipe connected to os.Stdin.
// Returns a cleanup function. Uses os.Pipe so calls happen in-memory.
func feedStdinFromString(t *testing.T, input string) func() {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Write in a goroutine so the pipe doesn't block if the buffer fills.
	go func() {
		w.WriteString(input)
		w.Close()
	}()
	oldStdin := os.Stdin
	os.Stdin = r
	return func() { os.Stdin = oldStdin }
}

// TestInitCreatesBothTemplates verifies that sworn init --yes creates both
// docs/considerations.md and docs/decisions.md in a fresh temp directory.
func TestInitCreatesBothTemplates(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWORN_CONFIG_PATH", configPath)

	setupCatalogTemplates(dir)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdInit([]string{"--yes"})
	if exit != 0 {
		t.Fatalf("cmdInit --yes exited %d, want 0", exit)
	}

	// Verify both files were created.
	for _, f := range []string{"docs/considerations.md", "docs/decisions.md"} {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist, but it does not", f)
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("cannot read %s: %v", f, err)
			} else if len(data) == 0 {
				t.Errorf("%s is empty", f)
			}
		}
	}
}

// TestInitSkipsBoth verifies that when the user answers 'n' to the catalog
// prompt, neither docs/considerations.md nor docs/decisions.md is created.
//
// This test pre-creates a minimal config.json so that the design-system and
// implementer-model interactive prompts (S08/S09 code paths) are skipped.
// Only the scan-confirm and catalog prompts remain.
func TestInitSkipsBoth(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWORN_CONFIG_PATH", configPath)

	setupMinimalConfig(dir)
	setupCatalogTemplates(dir)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	// "y\n" for proceed, "n\n" for catalog.
	cleanup := feedStdinFromString(t, "y\nn\n")
	defer cleanup()

	exit := cmdInit([]string{})
	if exit != 0 {
		t.Fatalf("cmdInit exited %d, want 0", exit)
	}

	// Verify neither file was created.
	for _, f := range []string{"docs/considerations.md", "docs/decisions.md"} {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("expected %s to NOT exist after catalog skip, but it exists", f)
		}
	}
}

// TestInitOverwriteGuard verifies that when both catalog files already exist,
// the user is prompted before overwriting, and answering 'n' leaves the files
// unchanged.
func TestInitOverwriteGuard(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWORN_CONFIG_PATH", configPath)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	setupMinimalConfig(dir)

	// Pre-create both target files with known content.
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	originalConsiderations := "# original considerations content"
	originalDecisions := "# original decisions content"
	os.WriteFile(filepath.Join(dir, "docs/considerations.md"), []byte(originalConsiderations), 0644)
	os.WriteFile(filepath.Join(dir, "docs/decisions.md"), []byte(originalDecisions), 0644)

	// Pre-create template source files.
	os.MkdirAll(filepath.Join(dir, "docs/templates"), 0755)
	os.WriteFile(filepath.Join(dir, "docs/templates/considerations.md"), []byte("# template considerations"), 0644)
	os.WriteFile(filepath.Join(dir, "docs/templates/decisions.md"), []byte("# template decisions"), 0644)

	// Flow: proceed (y), catalog-accept (y), overwrite-1 (n), overwrite-2 (n).
	cleanup := feedStdinFromString(t, "y\ny\nn\nn\n")
	defer cleanup()

	exit := cmdInit([]string{})
	if exit != 0 {
		t.Fatalf("cmdInit exited %d, want 0", exit)
	}

	// Verify both files are unchanged.
	gotConsiderations, _ := os.ReadFile(filepath.Join(dir, "docs/considerations.md"))
	gotDecisions, _ := os.ReadFile(filepath.Join(dir, "docs/decisions.md"))

	if strings.TrimSpace(string(gotConsiderations)) != strings.TrimSpace(originalConsiderations) {
		t.Errorf("considerations.md was overwritten — got %q, want %q",
			strings.TrimSpace(string(gotConsiderations)), originalConsiderations)
	}
	if strings.TrimSpace(string(gotDecisions)) != strings.TrimSpace(originalDecisions) {
		t.Errorf("decisions.md was overwritten — got %q, want %q",
			strings.TrimSpace(string(gotDecisions)), originalDecisions)
	}
}