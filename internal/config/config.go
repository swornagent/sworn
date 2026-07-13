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

// Config is the sworn runtime configuration. All model selections are
// "provider/model" strings (e.g. "openai/gpt-4.1") as used by model.FromEnv.
//
// For UI-bearing projects, the DesignSystem field declares the project's
// design tokens source and component library, used by sworn designaudit (S09)
// for design conformance checking.
type Config struct {
	Version int `json:"version"`

	// One entry per Baton work role. Model selection lives HERE — there is no
	// env-var layer and no hardcoded default (see ResolveRoleModel).
	//
	// Planner and Captain are optional: an unset role borrows its declared
	// fallback's model (planner → implementer, captain → verifier). Before they
	// existed the planner fell back to a hardcoded "openai/gpt-4o" and the captain
	// silently rode escalation_models[0] — the cheapest rung of a RETRY ladder — so
	// the role holding Rule 9 design authority ran on the weakest model available.
	Verifier    ModelSetting `json:"verifier"`
	Implementer ModelSetting `json:"implementer,omitempty"`
	Planner     ModelSetting `json:"planner,omitempty"`
	Captain     ModelSetting `json:"captain,omitempty"`

	// OptimizeMode selects the implementer routing strategy: "quality",
	// "cost", or "balanced". When empty, defaults to "quality" (S54
	// behaviour, unchanged).
	OptimizeMode string `json:"optimize_mode,omitempty"`

	// PassRateFloor overrides the default quality floor for cost-aware
	// routing (default 0.8). Values <= 0 or > 1 use the default.
	PassRateFloor float64 `json:"pass_rate_floor,omitempty"`

	// UIBearing marks the project as UI-bearing. When true, a DesignSystem
	// declaration is required or sworn will fail closed. When false (or absent),
	// design-system requirements do not apply (CLI projects are exempt).
	UIBearing bool `json:"ui_bearing,omitempty"`

	// DesignSystem declares the project's design system: the umbrella (this
	// struct), the design tokens source of truth (TokenSource), and the coded
	// component library (ComponentLibrary). Required when UIBearing is true.
	DesignSystem *DesignSystem `json:"design_system,omitempty"`
} // DesignSystem represents a project's design system declaration.
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

// DefaultConfig returns the scaffold configuration. It carries NO model IDs.
//
// It used to pre-fill "openai/gpt-4o-mini" (implementer), ["openai/gpt-4o",
// "openai/o3"] (escalation) and "anthropic/claude-sonnet-4-6" (verifier). Load()
// returns DefaultConfig when no config file exists, so a user who had never run
// `sworn init` silently ran their implementer on a hardcoded, years-stale model and
// was never told. That is a lie with a shelf life, and it defeated every
// "not configured" error below: nothing could fail closed while the default
// quietly answered for you.
//
// Model selection is now a decision the project makes, once, in config.json.
// `sworn init` asks; nothing guesses. An unconfigured role is an error with a
// remedy, not a silent substitution.
//
// UIBearing is false by default — a CLI project is exempt from design conformance.
func DefaultConfig() Config {
	return Config{
		Version:      1,
		UIBearing:    false,
		DesignSystem: nil,
	}
} // ConfigDir returns the directory containing the config file.
// It is a thin wrapper around filepath.Dir(Path()) — one line.
// Added by S06a-sworn-login-auth (T3-commercial).
func ConfigDir() string {
	return filepath.Dir(Path())
}

// Path returns the config file path, respecting env-var overrides://
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

// Role is a Baton work role that dispatches to a model.
type Role string

const (
	RolePlanner     Role = "planner"
	RoleImplementer Role = "implementer"
	RoleVerifier    Role = "verifier"
	RoleCaptain     Role = "captain"
)

// roleFallback declares whose model a role borrows when it has none of its own.
//
// Declared ONCE, here, instead of improvised at each call site — which is what
// produced two silent defects. The planner fell back to a HARDCODED
// "openai/gpt-4o" (a tool whose thesis is capability-based selection, quietly
// assuming a provider and a model). The captain ran on escalationModels[0] — the
// FIRST entry of a list ordered cheapest-first for RETRY escalation — so the
// design-authority role, which owns Rule 9 Type-1 decisions, silently defaulted
// to the weakest model in the ladder. Neither was ever decided; both were
// artefacts of a call site improvising.
//
// The fallbacks are chosen by what the role DOES:
//   - planner  -> implementer: planning is authoring.
//   - captain  -> verifier:    design review is judgement, not authoring, and the
//     verifier is the role deliberately kept high-tier.
//
// implementer and verifier have no fallback: they are the two roles a project must
// configure, and a missing one is an error, never a guess.
var roleFallback = map[Role]Role{
	RolePlanner: RoleImplementer,
	RoleCaptain: RoleVerifier,
}

// roleSetting returns the ModelSetting a role reads from config.
func roleSetting(role Role, cfg Config) ModelSetting {
	switch role {
	case RolePlanner:
		return cfg.Planner
	case RoleImplementer:
		return cfg.Implementer
	case RoleVerifier:
		return cfg.Verifier
	case RoleCaptain:
		return cfg.Captain
	}
	return ModelSetting{}
}

// ResolveRoleModel returns the model ID for a role, in precedence order:
//
//  1. flag (an explicit, one-off CLI override)
//  2. config file (<role>.model) — the single source of truth
//  3. the role's declared fallback ROLE (see roleFallback), resolved the same way
//  4. error
//
// There is no env-var layer and no hardcoded default. Model selection lives in
// config.json, in one place, so `sworn doctor` can show it, a teammate can read it,
// and CI runs what you run. A per-role env var was a second source of truth that
// drifted: llm-check read $SWORN_MODEL while its siblings read
// $SWORN_VERIFIER_MODEL, so a fully-configured setup still got "no model
// configured".
//
// And it NEVER guesses a model. A hardcoded fallback is a lie with a shelf life:
// the planner's was "openai/gpt-4o" and the escalation ladder's first rung was
// "openai/gpt-4o-mini" — both long stale, both silently assuming an OpenAI key, in a
// tool whose thesis is capability-based selection. An unconfigured role is an error.
func ResolveRoleModel(role Role, flagModel string, cfg Config) (string, error) {
	if flagModel != "" {
		return flagModel, nil
	}
	if m := roleSetting(role, cfg).Model; m != "" {
		return m, nil
	}

	if fallback, ok := roleFallback[role]; ok {
		if m := roleSetting(fallback, cfg).Model; m != "" {
			return m, nil
		}
		return "", fmt.Errorf(
			"no model configured for the %s role, and its fallback (%s) is not configured either — "+
				"set %q.model in %s (run 'sworn init' to scaffold it)",
			role, fallback, role, Path(),
		)
	}

	return "", fmt.Errorf(
		"no model configured for the %s role — set %q.model in %s (run 'sworn init' to scaffold it)",
		role, role, Path(),
	)
}

// ResolveVerifierModel resolves the verifier's model. Thin named accessor over
// ResolveRoleModel — one implementation, read clearly at the call site.
func ResolveVerifierModel(flagModel string, cfg Config) (string, error) {
	return ResolveRoleModel(RoleVerifier, flagModel, cfg)
}

// ResolvePlannerModel resolves the planner's model (falling back to the
// implementer's — planning is authoring).
func ResolvePlannerModel(flagModel string, cfg Config) (string, error) {
	return ResolveRoleModel(RolePlanner, flagModel, cfg)
}

// ResolveCaptainModel resolves the captain's model (falling back to the
// verifier's — design review is judgement, and the verifier is kept high-tier).
func ResolveCaptainModel(flagModel string, cfg Config) (string, error) {
	return ResolveRoleModel(RoleCaptain, flagModel, cfg)
}

// There is deliberately no DefaultEscalationModels.
//
// It used to be ["openai/gpt-4o-mini", "openai/gpt-4o", "openai/o3-mini",
// "openai/o3"] — four hardcoded, now-stale OpenAI models. Two things rode on it
// silently: the escalation ladder itself, and the CAPTAIN, which took
// escalation_models[0] and therefore ran the Rule 9 design-authority role on the
// cheapest rung of a list ordered for retry.
//
// A hardcoded model list is a lie with a shelf life. When no escalation models are
// configured, the ladder is just the implementer's own model — no escalation — and
// the captain resolves through its own role (ResolveRoleModel), not by accident.

// ResolveImplementerModel returns the implementer model ID from the first
// available source, in precedence order:
//
//  1. --implementer-model flag
//  2. config file (implementer.model)
//  3. ledger recommendation for sliceKind (when corpus is confident)
//  4. first entry of config file implementer.escalation_models
//
// No env-var layer: model selection lives in config.json (see ResolveRoleModel).
//
// sliceKind is the rubric dimension (e.g. "harness", "provider"). When
// empty, the ledger lookup is skipped. ledgerPath is the path to
// docs/ledger/verdicts.jsonl; when empty, the ledger lookup is skipped.
//
// optimizeMode selects the routing strategy: "quality", "cost", or
// "balanced". When empty, defaults to "quality" (S54 behaviour unchanged).
// Precedence: optimizeMode param → config file optimize_mode field.
//
// passRateFloor overrides the quality floor for cost-aware routing
// (default 0.8). Values <= 0 or > 1 use the default. Precedence:
// passRateFloor param → config file pass_rate_floor field.
//
// Returns an error when no source provides a model.
func ResolveImplementerModel(flagModel string, cfg Config, sliceKind string, ledgerPath string, optimizeMode string, passRateFloor float64) (string, error) {
	if flagModel != "" {
		return flagModel, nil
	}
	if cfg.Implementer.Model != "" {
		return cfg.Implementer.Model, nil
	}

	// Resolve optimize mode: param → env → config → default "quality".
	mode := optimizeMode
	if mode == "" && cfg.OptimizeMode != "" {
		mode = cfg.OptimizeMode
	}
	if mode == "" {
		mode = "quality"
	}

	// Resolve pass-rate floor: param → config → default 0.
	floor := passRateFloor
	if floor <= 0 || floor > 1 {
		floor = cfg.PassRateFloor
	}
	if floor <= 0 || floor > 1 {
		floor = 0 // let RecommendModel use DefaultPassRateFloor
	}

	// Ledger-backed default: when sliceKind is non-empty and the corpus
	// has a confident recommendation, use it.
	if sliceKind != "" && ledgerPath != "" {
		records, err := ledger.Load(ledgerPath)
		if err == nil {
			obj := ledger.ParseObjective(mode)
			if rec, ok := ledger.RecommendModel(records, "implementer", sliceKind, obj, floor); ok {
				return rec.Model, nil
			}
		}
	}

	if len(cfg.Implementer.EscalationModels) > 0 {
		return cfg.Implementer.EscalationModels[0], nil
	}
	return "", fmt.Errorf(
		"no model configured for the implementer role — set \"implementer\".model in %s (run 'sworn init' to scaffold it)",
		Path(),
	)
} // ResolveEscalationModels returns the ordered escalation model list from the
// first available source, in precedence order:
//
//  1. --escalation-models flag (passed as a pre-parsed []string)
//  3. config file (implementer.escalation_models)
//  4. DefaultEscalationModels
//
// The returned slice is the raw configured value — no dedup, no filtering
// (S44-feedback-driven-retry inherits it via run.Options.EscalationModels).
func ResolveEscalationModels(flagModels []string, cfg Config) []string {
	if len(flagModels) > 0 {
		return flagModels
	}
	if len(cfg.Implementer.EscalationModels) > 0 {
		return cfg.Implementer.EscalationModels
	}
	// No ladder configured: escalate to nothing. The implementer's own model is the
	// only rung. Previously this returned four hardcoded OpenAI models — see the
	// note where DefaultEscalationModels used to live.
	if cfg.Implementer.Model != "" {
		return []string{cfg.Implementer.Model}
	}
	return nil
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
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
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
