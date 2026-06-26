// Package memory implements the sworn memory system configuration layer:
// multi-harness path discovery, global + per-project config merge, and
// sworn memory status display.
package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HarnessID identifies a known AI coding harness.
type HarnessID string

const (
	HarnessClaudeCode HarnessID = "claude-code"
	HarnessGeminiCLI  HarnessID = "gemini-cli"
	HarnessOpenCode   HarnessID = "opencode"
	HarnessCursor     HarnessID = "cursor"
	HarnessWindsurf   HarnessID = "windsurf"
	HarnessCodex      HarnessID = "codex"
	HarnessCustom     HarnessID = "custom"
)

// KnownHarnessIDs returns all valid harness IDs.
func KnownHarnessIDs() []HarnessID {
	return []HarnessID{
		HarnessClaudeCode,
		HarnessGeminiCLI,
		HarnessOpenCode,
		HarnessCursor,
		HarnessWindsurf,
		HarnessCodex,
		HarnessCustom,
	}
}

// IsValidHarnessID returns true if id is a known harness.
func IsValidHarnessID(id string) bool {
	for _, known := range KnownHarnessIDs() {
		if string(known) == id {
			return true
		}
	}
	return false
}

// EmbeddingProvider identifies a supported embedding provider.
type EmbeddingProvider string

const (
	ProviderVoyage    EmbeddingProvider = "voyage"
	ProviderOAICompat EmbeddingProvider = "oai-compat"
	ProviderOllama    EmbeddingProvider = "ollama"
)

// IsValidEmbeddingProvider returns true if p is a known provider.
func IsValidEmbeddingProvider(p string) bool {
	switch EmbeddingProvider(p) {
	case ProviderVoyage, ProviderOAICompat, ProviderOllama:
		return true
	default:
		return false
	}
}

// EmbeddingConfig holds the embedding provider configuration.
type EmbeddingConfig struct {
	Provider  EmbeddingProvider `json:"provider"`
	Model     string            `json:"model"`
	APIKeyEnv string            `json:"api_key_env"`
	BaseURL   string            `json:"base_url"`
}

// MemoryConfig represents the sworn memory system configuration.
// Arrays are replaced (not appended) by per-project overrides.
type MemoryConfig struct {
	Harnesses  []string        `json:"harnesses"`
	ExtraPaths []string        `json:"extra_paths"`
	Embedding  EmbeddingConfig `json:"embedding"`
	IndexPath  string          `json:"index_path"`

	// loadedPaths tracks which files were loaded (for status display).
	loadedPaths []string
}

// LoadedPaths returns the files loaded during Load (global + project).
func (m *MemoryConfig) LoadedPaths() []string {
	return m.loadedPaths
}

// ErrUnknownHarness is returned when a config names an unknown harness ID.
type ErrUnknownHarness struct {
	ID     string
	Knowns []string
}

func (e *ErrUnknownHarness) Error() string {
	return fmt.Sprintf("unknown harness %q; valid: %s", e.ID, strings.Join(e.Knowns, ", "))
}

// GlobalConfigPath returns the user-level global config path:
// ~/.config/sworn/memory.json.
func GlobalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "sworn", "memory.json")
}

// ProjectConfigPath returns the per-project config path: .sworn/memory.json
// in the current working directory.
func ProjectConfigPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(cwd, ".sworn", "memory.json")
}

// Defaults returns a MemoryConfig with sensible defaults. It auto-detects
// the Claude Code memory path if present at ~/.claude/projects/<encoded-cwd>/memory/.
func Defaults() (*MemoryConfig, error) {
	cfg := &MemoryConfig{
		Harnesses:  []string{string(HarnessClaudeCode)},
		ExtraPaths: []string{},
		Embedding: EmbeddingConfig{
			Provider:  ProviderVoyage,
			Model:     "voyage-code-3",
			APIKeyEnv: "VOYAGE_API_KEY",
			BaseURL:   "",
		},
		IndexPath: filepath.Join(os.Getenv("HOME"), ".sworn", "memory.db"),
	}

	// Validate default harnesses — claude-code is always valid.
	return cfg, nil
}

// EncodeProjectPath encodes an absolute project path for use in Claude Code's
// memory directory scheme. It replaces "/" with "-", matching the encoding in
// baton's captain-memory-search.py.
//
// Cross-platform: Windows backslashes are normalised to forward slashes first.
func EncodeProjectPath(path string) string {
	normalised := filepath.ToSlash(path)
	// Strip trailing slash for consistency.
	normalised = strings.TrimRight(normalised, "/")
	return strings.ReplaceAll(normalised, "/", "-")
}

// loadJSONFile reads and unmarshals a JSON config file. Returns nil, nil if
// the file does not exist (not an error — config is optional).
func loadJSONFile(path string) (*MemoryConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg MemoryConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

// validateHarnesses checks that all harness IDs in cfg.Harnesses are known.
// Returns an ErrUnknownHarness on the first unknown ID.
func validateHarnesses(cfg *MemoryConfig) error {
	knowns := KnownHarnessIDs()
	for _, id := range cfg.Harnesses {
		if !IsValidHarnessID(id) {
			knownStrs := make([]string, len(knowns))
			for i, k := range knowns {
				knownStrs[i] = string(k)
			}
			return &ErrUnknownHarness{ID: id, Knowns: knownStrs}
		}
	}
	return nil
}

// mergeOverrides applies project config as an override on top of global config.
// Arrays are replaced (not appended). The project's loadedPaths is appended.
func mergeOverrides(global, project *MemoryConfig) *MemoryConfig {
	if global == nil {
		if project == nil {
			return nil // both absent
		}
		project.loadedPaths = []string{ProjectConfigPath()}
		return project
	}
	if project == nil {
		global.loadedPaths = []string{GlobalConfigPath()}
		return global
	}

	// Merge: project wins on all scalar fields; arrays replaced.
	merged := *global

	// Arrays are replaced.
	if len(project.Harnesses) > 0 {
		merged.Harnesses = project.Harnesses
	}
	if len(project.ExtraPaths) > 0 {
		merged.ExtraPaths = project.ExtraPaths
	}
	if project.IndexPath != "" {
		merged.IndexPath = project.IndexPath
	}
	if project.Embedding.Provider != "" {
		merged.Embedding = project.Embedding
	}

	merged.loadedPaths = []string{GlobalConfigPath(), ProjectConfigPath()}
	return &merged
}

// Load reads the memory configuration with precedence: project override
// (~/.sworn/memory.json) over global (~/.config/sworn/memory.json). If neither
// exists, returns Defaults(). Validates all harness IDs.
func Load() (*MemoryConfig, error) {
	globalPath := GlobalConfigPath()
	projectPath := ProjectConfigPath()

	global, err := loadJSONFile(globalPath)
	if err != nil {
		return nil, err
	}

	project, err := loadJSONFile(projectPath)
	if err != nil {
		return nil, err
	}

	// Neither file exists — return defaults.
	if global == nil && project == nil {
		cfg, err := Defaults()
		if err != nil {
			return nil, err
		}
		cfg.loadedPaths = []string{}
		if err := validateHarnesses(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	merged := mergeOverrides(global, project)
	if err := validateHarnesses(merged); err != nil {
		return nil, err
	}
	return merged, nil
}
