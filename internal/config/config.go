// Package config loads sworn's configuration with precedence: env > file > default.
// Config is the single source for model selections and provider settings consumed by
// sworn verify (and later sworn run). It never logs API keys.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config is the sworn runtime configuration. All model selections are
// "provider/model" strings (e.g. "openai/gpt-4.1") as used by model.FromEnv.
//
// For UI-bearing projects, the DesignSystem field declares the project's
// design tokens source and component library, used by sworn designaudit (S09)
// for design conformance checking.
type Config struct {
	Version  int          `json:"version"`
	Verifier ModelSetting `json:"verifier"`

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
// ModelSetting holds a single role's model selection.
type ModelSetting struct {
	Model string `json:"model"`
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
// is "openai/gpt-4.1" — a trusted-jurisdiction default. Users must set at least
// the API key via env var (SWORN_OPENAI_API_KEY) or through sworn init --api-key.
// This default is a provisional safe-hosted selection. The production default
// will be ratified by the S10-benchmark-dogfood slice (tracked in this release
// board). If the benchmark picks a different model, the default changes there.
//
// By default, UIBearing is false — sworn itself is a CLI tool. UI-bearing
// projects must set UIBearing to true and declare a DesignSystem.
func DefaultConfig() Config {
	return Config{
		Version: 1,
		Verifier: ModelSetting{
			Model: "openai/gpt-4.1",
		},
		UIBearing:    false,
		DesignSystem: nil,
	}
}
// Path returns the config file path, respecting env-var overrides:
//
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