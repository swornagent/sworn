package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeProjectFile writes a file at path under dir, creating parent directories.
func writeProjectFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	path := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeProjectConfig writes a sworn config.json in the project dir with the
// given ui_bearing and design_system values.
func writeProjectConfig(t *testing.T, dir string, uiBearing bool, tokenSource, componentLibrary string) {
	t.Helper()
	type designSystem struct {
		TokenSource      string `json:"token_source,omitempty"`
		ComponentLibrary string `json:"component_library,omitempty"`
	}
	type cfg struct {
		Version      int           `json:"version"`
		UIBearing    bool          `json:"ui_bearing,omitempty"`
		DesignSystem *designSystem `json:"design_system,omitempty"`
	}
	c := cfg{Version: 1, UIBearing: uiBearing}
	if tokenSource != "" || componentLibrary != "" {
		c.DesignSystem = &designSystem{
			TokenSource:      tokenSource,
			ComponentLibrary: componentLibrary,
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, dir, "config.json", string(data))
}

// TestDesignauditCmd_HardcodedHex verifies the CLI entry point fails closed
// when UI source has a hardcoded hex colour (AC1 integration test — Rule 1).
func TestDesignauditCmd_HardcodedHex(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, true, "tokens.json", "src/ui")
	writeProjectFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeProjectFile(t, dir, "src/app/page.css", "body {\n  color: #ff0000;\n}\n")

	exit := cmdDesignaudit([]string{dir})
	if exit == 0 {
		t.Fatalf("cmdDesignaudit with hardcoded hex should exit non-zero, got %d", exit)
	}
}

// TestDesignauditCmd_CleanWithCohesion verifies the CLI exits 0 when source is
// clean and a cohesion verdict is supplied (AC4 integration test — Rule 1).
func TestDesignauditCmd_CleanWithCohesion(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, true, "tokens.json", "src/ui")
	writeProjectFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeProjectFile(t, dir, "src/app/page.css",
		"body {\n  color: var(--color-primary);\n  margin: var(--spacing-4);\n}\n")

	exit := cmdDesignaudit([]string{"--cohesion=on-brand", dir})
	if exit != 0 {
		t.Fatalf("cmdDesignaudit clean source + cohesion should exit 0, got %d", exit)
	}
}

// TestDesignauditCmd_MissingCohesion verifies that clean source without a
// cohesion verdict exits non-zero (AC5 integration test — human verdict required).
func TestDesignauditCmd_MissingCohesion(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, true, "tokens.json", "src/ui")
	writeProjectFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeProjectFile(t, dir, "src/app/page.css",
		"body {\n  color: var(--color-primary);\n}\n")

	exit := cmdDesignaudit([]string{dir}) // no --cohesion
	if exit == 0 {
		t.Fatal("cmdDesignaudit without cohesion on clean source should exit non-zero")
	}
}

// TestDesignauditCmd_NotUIBearing verifies that a non-UI-bearing project
// is exempt and exits 0.
func TestDesignauditCmd_NotUIBearing(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, false, "", "")
	writeProjectFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	exit := cmdDesignaudit([]string{dir})
	if exit != 0 {
		t.Fatalf("non-UI-bearing project should exit 0 (exempt), got %d", exit)
	}
}

// TestDesignauditCmd_NoArgs verifies that missing project-dir argument exits 64.
func TestDesignauditCmd_NoArgs(t *testing.T) {
	exit := cmdDesignaudit([]string{})
	if exit != 64 {
		t.Fatalf("no args should exit 64, got %d", exit)
	}
}
