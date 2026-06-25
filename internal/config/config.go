// Package config loads sworn's configuration with precedence: env > file > default.
// Config is the single source for model selections and provider settings consumed by
// sworn verify (and later sworn run). It never logs API keys.
//
// Extension points (for S17/S09):
//   - Save writes the current config to disk (creating parent dirs if needed).
//   - EnvPath returns the path to ~/.sworn/.env.
//   - LoadEnv / WriteEnv read and write provider API keys in .env format.
package config
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/swornagent/sworn/internal/ledger"
)

// Config is the sworn runtime configuration. All model selections are// "provider/model" strings (e.g. "openai/gpt-4.1") as used by model.FromEnv.
//
// For UI-bearing projects, the DesignSystem field declares the project's
// design tokens source and component library, used by sworn designaudit (S09)
// for design conformance checking.
type Config struct {
	Version     int          `json:"version"`
	Verifier    ModelSetting `json:"verifier"`
	Implementer ModelSetting `json:"implementer,omitempty"`

	// UIBearing marks the project as UI-bearing. When true, a DesignSystem
	// declaration is required or sworn will fail closed. When false (or absent),
	// design-system requirements do not apply (CLI projects are exempt).
	UIBearing bool `json:"ui_bearing,omitempty"`

	// DesignSystem declares the project's design system: the umbrella (this
	// struct), the design tokens source of truth (TokenSource), and the coded
	// component library (ComponentLibrary). Required when UIBearing is true.
	DesignSystem *DesignSystem `json:"design_system,omitempty"`
}
// DesignSystem represents a project's design system declaration.
// The three concepts are distinguished:
//   - DesignSystem (the umbrella struct)
//   - TokenSource (design tokens — the named-value source of truth)
//   - ComponentLibrary (the coded reusables)
//
// TokenSource and ComponentLibrary may be paths (relative to the project root),
// package names, or source URIs depending on the project's token format. S09's
// audit adapts to the declared format.
type DesignSystem struct {
	TokenSource      string `json:"token_source"`
	ComponentLibrary string `json:"component_library"`
}

// ErrNoDesignSystem is returned by Validate when a UI-bearing project has no
// DesignSystem declaration. Callers should surface this as a fail-closed error.
var ErrNoDesignSystem = fmt.Errorf(
	"ui_bearing is true but no design_system declared — " +
		"a design system (token source + component library) is required " +
		"for design conformance; run 'sworn init' to configure",
)
// ModelSetting holds a single role's model selection, escalation path, and
// retry cap. EscalationModels and MaxAttempts are only meaningful for the
// implementer role; the verifier ignores them (single-model, no retry).
type ModelSetting struct {
	Model            string   `json:"model"`
	EscalationModels []string `json:"escalation_models,omitempty"`
	MaxAttempts      int      `json:"max_attempts,omitempty"`
}
// Validate checks config invariants. It returns ErrNoDesignSystem when a
// UI-bearing project has no DesignSystem declaration. Unit-bearing projects
// (ui_bearing: false) are exempt.
func (c Config) Validate() error {
	if c.UIBearing && c.DesignSystem == nil {
		return ErrNoDesignSystem
	}
	return nil
}

// DefaultConfig returns the safe-hosted default configuration. The default model
// is "anthropic/claude-sonnet-4-6" — a trusted-jurisdiction default. Users must
// set SWORN_ANTHROPIC_API_KEY and, if needed, SWORN_ANTHROPIC_BASE_URL (defaults
// to Anthropic's OpenAI-compatible endpoint). Run sworn init --api-key to scaffold.
//
// By default, UIBearing is false — sworn itself is a CLI tool. UI-bearing
// projects must set UIBearing to true and declare a DesignSystem.
func DefaultConfig() Config {
	return Config{
		Version: 1,
		Verifier: ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
		Implementer: ModelSetting{
			Model:            "openai/gpt-4o-mini",
			EscalationModels: []string{"openai/gpt-4o", "openai/o3"},
			MaxAttempts:      3,
		},
		UIBearing:    false,
		DesignSystem: nil,
	}
}// ConfigDir returns the directory containing the config file.
// It is a thin wrapper around filepath.Dir(Path()) — one line.
// Added by S06a-sworn-login-auth (T3-commercial).
func ConfigDir() string {
	return filepath.Dir(Path())
}

// Path returns the config file path, respecting env-var overrides://
//	$SWORN_CONFIG_PATH — exact path to config.json
//	$SWORN_HOME        — config directory (joined with "config.json")
//	default             — XDG-compatible: $HOME/.config/sworn/config.json on Linux,
//	                      $HOME/Library/Application Support/sworn/config.json on macOS
func Path() string {
	if p := os.Getenv("SWORN_CONFIG_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if dir := os.Getenv("SWORN_HOME"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "sworn", "config.json")
	default:
		return filepath.Join(home, ".config", "sworn", "config.json")
	}
}

// Load reads the config file at its standard path (see Path). If the file does
// not exist, it returns DefaultConfig with no error — the user has not run
// sworn init yet but can still use env vars.
func Load() (Config, error) {
	p := Path()
	if p == "" {
		return Config{}, fmt.Errorf("config: cannot determine home directory; set $SWORN_CONFIG_PATH")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("config: reading %s: %w", p, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parsing %s: %w", p, err)
	}
	return cfg, nil
}

// ResolveVerifierModel returns the verifier model ID from the first available
// source, in precedence order:
//
//  1. --verifier-model flag (explicit CLI)
//  2. $SWORN_VERIFIER_MODEL env var
//  3. config file (verifier.model)
//
// Returns ("", nil) when no source is set — the caller must provide a clear
// error (not a crash).
func ResolveVerifierModel(flagModel string, cfg Config) (string, error) {
	if flagModel != "" {
		return flagModel, nil
	}
	if env := os.Getenv("SWORN_VERIFIER_MODEL"); env != "" {
		return env, nil
	}
	if cfg.Verifier.Model != "" {
		return cfg.Verifier.Model, nil
	}
	return "", fmt.Errorf(
		"verifier model not configured — run 'sworn init' to scaffold a config file (%s) or set $SWORN_VERIFIER_MODEL",
		Path(),
	)
}
// DefaultEscalationModels is the programmatic fallback when no escalation
// models are configured via flag, env, or config. Each entry is a
// "provider/model" ID suitable for model.FromEnv.
var DefaultEscalationModels = []string{
	"openai/gpt-4o-mini",
	"openai/gpt-4o",
	"openai/o3-mini",
	"openai/o3",
}

// ResolveImplementerModel returns the implementer model ID from the first
// available source, in precedence order:
//
//  1. --implementer-model flag
//  2. $SWORN_IMPLEMENTER_MODEL env var
//  3. config file (implementer.model)
//  4. ledger recommendation for sliceKind (when corpus is confident)
//  5. first entry of config file implementer.escalation_models
//
// sliceKind is the rubric dimension (e.g. "harness", "provider"). When
// empty, the ledger lookup is skipped. ledgerPath is the path to
// docs/ledger/verdicts.jsonl; when empty, the ledger lookup is skipped.
//
// Returns an error when no source provides a model.
func ResolveImplementerModel(flagModel string, cfg Config, sliceKind string, ledgerPath string) (string, error) {
	if flagModel != "" {
		return flagModel, nil
	}
	if env := os.Getenv("SWORN_IMPLEMENTER_MODEL"); env != "" {
		return env, nil
	}
	if cfg.Implementer.Model != "" {
		return cfg.Implementer.Model, nil
	}

	// Ledger-backed default: when sliceKind is non-empty and the corpus
	// has a confident recommendation, use it.
	if sliceKind != "" && ledgerPath != "" {
		records, err := ledger.Load(ledgerPath)
		if err == nil {
			if rec, ok := ledger.RecommendModel(records, sliceKind); ok {
				return rec.Model, nil
			}
		}
	}

	if len(cfg.Implementer.EscalationModels) > 0 {
		return cfg.Implementer.EscalationModels[0], nil
	}
	return "", fmt.Errorf(
		"implementer model not configured — run 'sworn init' to scaffold a config file (%s) or set $SWORN_IMPLEMENTER_MODEL",
		Path(),
	)
}
// ResolveEscalationModels returns the ordered escalation model list from the
// first available source, in precedence order:
//
//  1. --escalation-models flag (passed as a pre-parsed []string)
//  2. $SWORN_ESCALATION_MODELS env var (comma-separated)
//  3. config file (implementer.escalation_models)
//  4. DefaultEscalationModels
//
// The returned slice is the raw configured value — no dedup, no filtering
// (S44-feedback-driven-retry inherits it via run.Options.EscalationModels).
func ResolveEscalationModels(flagModels []string, cfg Config) []string {
	if len(flagModels) > 0 {
		return flagModels
	}
	if env := os.Getenv("SWORN_ESCALATION_MODELS"); env != "" {
		var models []string
		for _, m := range strings.Split(env, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				models = append(models, m)
			}
		}
		return models
	}
	if len(cfg.Implementer.EscalationModels) > 0 {
		return cfg.Implementer.EscalationModels
	}
	return DefaultEscalationModels
}

// ResolveMaxAttempts returns the maximum retry count from the first available
// source, in precedence order:
//
//  1. --retry-cap flag (>0)
//  2. config file (implementer.max_attempts >0)
//  3. default 3
func ResolveMaxAttempts(flagN int, cfg Config) int {
	if flagN > 0 {
		return flagN
	}
	if cfg.Implementer.MaxAttempts > 0 {
		return cfg.Implementer.MaxAttempts
	}
	return 3
}

// Save writes the Config as JSON to the file at Path(), creating parent
// directories if they do not exist. File permissions are 0600 (owner read/write)
// to match the security profile of Scaffold — the file may contain model
// selections that should not be world-readable. Returns an error if marshalling
// or writing fails.
func Save(cfg Config) error {
	p := Path()
	if p == "" {
		return fmt.Errorf("config: cannot determine config path; set $SWORN_CONFIG_PATH")
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config: creating parent dirs for %s: %w", p, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshalling config: %w", err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("config: writing %s: %w", p, err)
	}
	return nil
}

// EnvPath returns the path to the sworn .env file: ~/.sworn/.env.
// Does not create the file or its directory.
func EnvPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sworn", ".env")
}

// LoadEnv reads ~/.sworn/.env and returns a map of keys to values.
// Keys are uppercase (e.g. "OPENAI_API_KEY"). Returns an empty map
// (not nil) if the file does not exist.
func LoadEnv() (map[string]string, error) {
	p := EnvPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("config: reading %s: %w", p, err)
	}
	result := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])
		if key != "" {
			result[key] = val
		}
	}
	return result, nil
}

// WriteEnv writes a set of key=value pairs to ~/.sworn/.env, preserving existing
// lines not present in the updates map. If a key already exists, its line is
// replaced in-place; if a key is new, it is appended. Creates the file and parent
// directories if they do not exist.
func WriteEnv(updates map[string]string) error {
	// Read existing file.
	p := EnvPath()
	existing := map[string]int{} // key -> line index
	var lines []string
	data, err := os.ReadFile(p)
	if err == nil {
		raw := string(data)
		lines = strings.Split(raw, "\n")
		// Drop trailing empty line from split.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			eqIdx := strings.IndexByte(trimmed, '=')
			if eqIdx < 0 {
				continue
			}
			key := strings.TrimSpace(trimmed[:eqIdx])
			if key != "" {
				existing[key] = i
			}
		}
	} else {
		if !os.IsNotExist(err) {
			return fmt.Errorf("config: reading %s: %w", p, err)
		}
		lines = nil
	}

	// Update existing lines and collect new keys.
	for key, val := range updates {
		if idx, ok := existing[key]; ok {
			lines[idx] = fmt.Sprintf("%s=%s", key, val)
		} else {
			lines = append(lines, fmt.Sprintf("%s=%s", key, val))
		}
	}
	// Ensure trailing newline.
	content := strings.Join(lines, "\n") + "\n"

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config: creating .env parent dirs: %w", err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		return fmt.Errorf("config: writing %s: %w", p, err)
	}
	return nil
}