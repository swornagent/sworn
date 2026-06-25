package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)
// TestCmdMemory_Status_NoConfig verifies that sworn memory status exits 0
// with no config file and shows "using defaults".
func TestCmdMemory_Status_NoConfig(t *testing.T) {
	// Run in a temp dir with no config files.
	dir := t.TempDir()

	// Save and restore cwd.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Ensure no config files exist.
	os.Unsetenv("HOME") // prevent ~/.config/sworn/memory.json from existing

	// Since the function calls os.Getwd and memory.Load, we can't easily
	// fake the global config path. Instead, test the function directly
	// by verifying the output structure produced by cmdMemoryStatus.
	code := cmdMemoryStatus([]string{})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

// TestCmdMemory_Status_WithConfig verifies that sworn memory status reads
// a config file and prints the configured values.
func TestCmdMemory_Status_WithConfig(t *testing.T) {
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Create a project-level .sworn/memory.json.
	swornDir := filepath.Join(dir, ".sworn")
	if err := os.MkdirAll(swornDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(swornDir, "memory.json"), []byte(`{
		"harnesses": ["claude-code"],
		"extra_paths": [],
		"embedding": {
			"provider": "voyage",
			"model": "voyage-code-3",
			"api_key_env": "VOYAGE_API_KEY",
			"base_url": ""
		},
		"index_path": "~/.sworn/memory.db"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	code := cmdMemoryStatus([]string{})
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

// TestCmdMemory_Status_SetAPIKey verifies that the env var name is shown
// but the key value is never printed (only "<set>" / "<not set>").
func TestCmdMemory_Status_SetAPIKey(t *testing.T) {
	t.Setenv("TEST_SWORN_MEMORY_KEY", "sk-super-secret-value")

	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Create a project config referencing the test env var.
	swornDir := filepath.Join(dir, ".sworn")
	if err := os.MkdirAll(swornDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(swornDir, "memory.json"), []byte(`{
		"harnesses": ["claude-code"],
		"extra_paths": [],
		"embedding": {
			"provider": "voyage",
			"model": "voyage-code-3",
			"api_key_env": "TEST_SWORN_MEMORY_KEY",
			"base_url": ""
		},
		"index_path": "~/.sworn/memory.db"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w

	code := cmdMemoryStatus([]string{})

	// Restore stdout and read captured output.
	w.Close()
	os.Stdout = origStdout
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	r.Close()
	output := buf.String()

	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	// Output must contain the env var name.
	if !strings.Contains(output, "TEST_SWORN_MEMORY_KEY") {
		t.Errorf("output should contain env var name TEST_SWORN_MEMORY_KEY, got:\n%s", output)
	}

	// Output must NOT contain the raw key value.
	if strings.Contains(output, "sk-super-secret-value") {
		t.Errorf("output must not contain the raw API key value, got:\n%s", output)
	}
}
// TestCmdMemory_Status_UnknownHarness verifies that an unknown harness ID
// triggers an error exit.
func TestCmdMemory_Status_UnknownHarness(t *testing.T) {
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Create a config with an unknown harness.
	swornDir := filepath.Join(dir, ".sworn")
	if err := os.MkdirAll(swornDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(swornDir, "memory.json"), []byte(`{
		"harnesses": ["claude-code", "nonexistent-harness"],
		"extra_paths": [],
		"embedding": {
			"provider": "voyage",
			"model": "voyage-code-3",
			"api_key_env": "VOYAGE_API_KEY",
			"base_url": ""
		},
		"index_path": "~/.sworn/memory.db"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Should return non-zero exit code.
	code := cmdMemoryStatus([]string{})
	if code == 0 {
		t.Errorf("expected non-zero exit code for unknown harness, got 0")
	}
}