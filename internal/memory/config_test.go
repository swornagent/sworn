package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestEncodeProjectPath verifies that path encoding matches baton's// captain-memory-search.py: "/" → "-" substitution (pin 1 from Coach
// design review acknowledgement file).
func TestEncodeProjectPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/user/projects/sworn", "-home-user-projects-sworn"},
		{"/home/user/my-project", "-home-user-my-project"},
		{"/", ""}, // root becomes empty after TrimRight
		{"/var/www/app", "-var-www-app"},
	}

	// On Windows, backslashes are normalised to slashes by filepath.ToSlash.
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			input    string
			expected string
		}{"C:\\Users\\brad\\project", "-c-users-brad-project"})
	}

	for _, tc := range tests {
		got := EncodeProjectPath(tc.input)
		if got != tc.expected {
			t.Errorf("EncodeProjectPath(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// TestLoadMerge verifies that per-project config overrides global config:
// - Project values win on conflict (harnesses replaced)
// - Global values preserved where project doesn't override
// - Arrays are replaced (not appended)
func TestLoadMerge(t *testing.T) {
	// Create temp dirs simulating global + project config.
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	configDir := filepath.Join(home, ".config", "sworn")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(dir, "project")
	swornDir := filepath.Join(projectDir, ".sworn")
	if err := os.MkdirAll(swornDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write global config.
	globalPath := filepath.Join(configDir, "memory.json")
	if err := os.WriteFile(globalPath, []byte(`{
		"harnesses": ["claude-code", "gemini-cli"],
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

	// Write per-project override (only overrides harnesses).
	projectPath := filepath.Join(swornDir, "memory.json")
	if err := os.WriteFile(projectPath, []byte(`{
		"harnesses": ["cursor"]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Load and merge using the temp dirs via Load.
	// We need to override GlobalConfigPath/ProjectConfigPath behaviour.
	// Instead, test the mergeOverrides function directly.
	global, err := loadJSONFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	project, err := loadJSONFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}

	merged := mergeOverrides(global, project)
	if merged == nil {
		t.Fatal("mergeOverrides returned nil")
	}

	// Project harnesses should replace (not append to) global.
	if len(merged.Harnesses) != 1 {
		t.Fatalf("expected 1 harness (project override), got %d: %v", len(merged.Harnesses), merged.Harnesses)
	}
	if merged.Harnesses[0] != "cursor" {
		t.Errorf("expected harness 'cursor', got %q", merged.Harnesses[0])
	}

	// Embedding should come from global (not overridden).
	if merged.Embedding.Provider != ProviderVoyage {
		t.Errorf("expected embedding provider 'voyage', got %q", merged.Embedding.Provider)
	}
	if merged.Embedding.APIKeyEnv != "VOYAGE_API_KEY" {
		t.Errorf("expected API key env 'VOYAGE_API_KEY', got %q", merged.Embedding.APIKeyEnv)
	}
}

// TestDefaultsAutoDetect verifies that Defaults() returns a sensible config
// with claude-code as the default harness.
func TestDefaultsAutoDetect(t *testing.T) {
	cfg, err := Defaults()
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("Defaults() returned nil")
	}

	// Should include at least claude-code.
	if len(cfg.Harnesses) == 0 {
		t.Fatal("expected at least one default harness")
	}
	hasClaude := false
	for _, h := range cfg.Harnesses {
		if h == string(HarnessClaudeCode) {
			hasClaude = true
			break
		}
	}
	if !hasClaude {
		t.Errorf("defaults should include claude-code, got %v", cfg.Harnesses)
	}

	// Default embedding provider should be voyage.
	if cfg.Embedding.Provider != ProviderVoyage {
		t.Errorf("expected default embedding provider 'voyage', got %q", cfg.Embedding.Provider)
	}

	// Index path should be non-empty.
	if cfg.IndexPath == "" {
		t.Error("expected non-empty default index path")
	}
}

// TestUnknownHarness verifies that a config naming an unknown harness returns
// an ErrUnknownHarness with the list of known IDs.
func TestUnknownHarness(t *testing.T) {
	cfg := &MemoryConfig{
		Harnesses: []string{"claude-code", "nonexistent-harness"},
	}
	err := validateHarnesses(cfg)
	if err == nil {
		t.Fatal("expected error for unknown harness, got nil")
	}
	var ue *ErrUnknownHarness
	if !errors.As(err, &ue) {
		t.Fatalf("expected *ErrUnknownHarness, got %T: %v", err, err)
	}
	if ue.ID != "nonexistent-harness" {
		t.Errorf("expected ID 'nonexistent-harness', got %q", ue.ID)
	}
	if len(ue.Knowns) == 0 {
		t.Error("expected non-empty knowns list")
	}
}

// TestAPIKeyEnvNotLeaked verifies that status output contains the env var name
// but not the resolved key value even when the env var is set.
func TestAPIKeyEnvNotLeaked(t *testing.T) {
	t.Setenv("TEST_MEMORY_API_KEY", "sk-secret-value-12345")

	// Build a config with that env var name.
	cfg := &MemoryConfig{
		Harnesses: []string{string(HarnessClaudeCode)},
		Embedding: EmbeddingConfig{
			Provider:  ProviderVoyage,
			Model:     "voyage-code-3",
			APIKeyEnv: "TEST_MEMORY_API_KEY",
			BaseURL:   "",
		},
	}

	// APIKeyEnv stores the env var name, not the value.
	if cfg.Embedding.APIKeyEnv != "TEST_MEMORY_API_KEY" {
		t.Fatalf("APIKeyEnv should store env var name, got %q", cfg.Embedding.APIKeyEnv)
	}

	// Verify the env var is actually set (test setup sanity check).
	resolved := os.Getenv(cfg.Embedding.APIKeyEnv)
	if resolved != "sk-secret-value-12345" {
		t.Fatalf("test setup: expected TEST_MEMORY_API_KEY to resolve to test value")
	}

	// The MemoryConfig struct has no field that stores the resolved key value.
	// Only the env var name (APIKeyEnv) is part of the config struct.
	// Serialize to JSON and verify: env var name is present, raw value is absent.
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("TEST_MEMORY_API_KEY")) {
		t.Error("config JSON must contain the env var name TEST_MEMORY_API_KEY")
	}
	if bytes.Contains(data, []byte("sk-secret-value-12345")) {
		t.Error("config JSON must NOT contain the resolved API key value")
	}

	// The CLI layer (cmd/sworn/memory.go:apiKeyStatus) renders only
	// "<set>" or "<not set>" — this is verified at the CLI integration
	// level in cmd/sworn/memory_test.go (TestCmdMemory_Status_SetAPIKey)
	// which calls cmdMemoryStatus() with stdout capture.
}

// TestIsValidHarnessID verifies the harness ID validation.
func TestIsValidHarnessID(t *testing.T) {
	if !IsValidHarnessID("claude-code") {
		t.Error("expected claude-code to be valid")
	}
	if IsValidHarnessID("nonexistent") {
		t.Error("expected nonexistent to be invalid")
	}
}

// TestIsValidEmbeddingProvider verifies embedding provider validation.
func TestIsValidEmbeddingProvider(t *testing.T) {
	if !IsValidEmbeddingProvider("voyage") {
		t.Error("expected voyage to be valid")
	}
	if IsValidEmbeddingProvider("unknown") {
		t.Error("expected unknown to be invalid")
	}
}

// TestHarnessMemoryPath verifies that canonical paths are derived correctly.
func TestHarnessMemoryPath(t *testing.T) {
	cwd := "/home/user/project"
	// Claude Code path is dynamic (depends on home dir).
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	encoded := EncodeProjectPath(cwd)
	expected := filepath.Join(home, ".claude", "projects", encoded, "memory")
	got := HarnessMemoryPath(HarnessClaudeCode, cwd)
	if got != expected {
		t.Errorf("HarnessMemoryPath(claude-code) = %q, want %q", got, expected)
	}

	// OpenCode.
	if got := HarnessMemoryPath(HarnessOpenCode, cwd); got != filepath.Join(cwd, "AGENTS.md") {
		t.Errorf("HarnessMemoryPath(opencode) = %q, want %q", got, filepath.Join(cwd, "AGENTS.md"))
	}
}
