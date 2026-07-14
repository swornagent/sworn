package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/ledger"
)

// TestDefaultConfig_CarriesNoModelIDs is the guard for the hardcoded-default defect.
//
// DefaultConfig used to pre-fill openai/gpt-4o-mini (implementer), gpt-4o + o3
// (escalation) and a verifier model. Load() returns DefaultConfig when no config
// file exists, so a user who had never run `sworn init` silently ran on hardcoded,
// years-stale models and was never told — and no "not configured" error could ever
// fire, because the default quietly answered for them.
func TestDefaultConfig_CarriesNoModelIDs(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != 1 {
		t.Errorf("DefaultConfig version = %d, want 1", cfg.Version)
	}
	for role, ms := range map[Role]ModelSetting{
		RoleVerifier:    cfg.Verifier,
		RoleImplementer: cfg.Implementer,
		RolePlanner:     cfg.Planner,
		RoleCaptain:     cfg.Captain,
	} {
		if ms.Model != "" {
			t.Errorf("DefaultConfig hardcodes a %s model (%q) — model selection is the "+
				"project's decision, and a hardcoded default is a lie with a shelf life", role, ms.Model)
		}
	}
	if len(cfg.Implementer.EscalationModels) != 0 {
		t.Errorf("DefaultConfig hardcodes an escalation ladder: %v", cfg.Implementer.EscalationModels)
	}
}

func TestPath(t *testing.T) {
	// With SWORN_CONFIG_PATH set, Path returns it exactly.
	t.Setenv("SWORN_CONFIG_PATH", "/tmp/test-config.json")
	if got := Path(); got != "/tmp/test-config.json" {
		t.Errorf("Path with SWORN_CONFIG_PATH = %q, want /tmp/test-config.json", got)
	}
	t.Setenv("SWORN_CONFIG_PATH", "")

	// With SWORN_HOME set, Path joins config.json under it.
	t.Setenv("SWORN_HOME", "/tmp/sworn-test-home")
	if got := Path(); got != filepath.Join("/tmp/sworn-test-home", "config.json") {
		t.Errorf("Path with SWORN_HOME = %q, want .../config.json", got)
	}
	t.Setenv("SWORN_HOME", "")
}

func TestLoadNotExistReturnsDefault(t *testing.T) {
	// Point to a path that doesn't exist.
	t.Setenv("SWORN_CONFIG_PATH", "/tmp/does-not-exist/config.json")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("Load version = %d, want 1", cfg.Version)
	}
}

func TestResolveVerifierModel(t *testing.T) {
	cfg := Config{Version: 1, Verifier: ModelSetting{Model: "anthropic/claude-sonnet-4-6"}}

	t.Run("flag wins", func(t *testing.T) {
		m, err := ResolveVerifierModel("anthropic/claude-sonnet-4-20250514", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if m != "anthropic/claude-sonnet-4-20250514" {
			t.Errorf("got %q", m)
		}
	})

	t.Run("config is the source (no env layer)", func(t *testing.T) {
		// A per-role env var was a SECOND source of truth that drifted: llm-check
		// read $SWORN_MODEL while its siblings read $SWORN_VERIFIER_MODEL, so a
		// fully-configured setup still got "no model configured". config.json is now
		// the only source, and an env var must not override it.
		t.Setenv("SWORN_VERIFIER_MODEL", "openai/should-be-ignored")
		m, err := ResolveVerifierModel("", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if m != cfg.Verifier.Model {
			t.Errorf("got %q", m)
		}
	})

	t.Run("config fallback", func(t *testing.T) {
		m, err := ResolveVerifierModel("", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if m != "anthropic/claude-sonnet-4-6" {
			t.Errorf("got %q", m)
		}
	})
}

func TestResolveVerifierModelMissingKey(t *testing.T) {
	// Missing-model error path: smoke test the error.
	cfg := Config{Version: 1} // no verifier model set
	_, err := ResolveVerifierModel("", cfg)
	if err == nil {
		t.Fatal("expected error when no verifier model is configured anywhere")
	}
	// Error message should mention 'sworn init' and the config path.
	msg := err.Error()
	if !contains(msg, "sworn init") {
		t.Errorf("error should mention 'sworn init', got: %s", msg)
	}
	if !contains(msg, "verifier") {
		t.Errorf("error should name the unconfigured role, got: %s", msg)
	}
}

func TestScaffoldIdempotent(t *testing.T) {
	// Idempotency: second Scaffold without force returns ErrConfigExists.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWORN_CONFIG_PATH", configPath)

	// First call: create file.
	p, existed, err := Scaffold(false)
	if err != nil {
		t.Fatalf("first Scaffold: %v", err)
	}
	if existed {
		t.Error("first Scaffold should not report existed = true")
	}
	if p != configPath {
		t.Errorf("path = %q, want %q", p, configPath)
	}

	// Second call without force: should return ErrConfigExists.
	_, existed, err = Scaffold(false)
	if !existed {
		t.Error("second Scaffold should report existed = true")
	}
	if err != ErrConfigExists {
		t.Errorf("second Scaffold error = %v, want ErrConfigExists", err)
	}

	// Verify file permissions.
	fi, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("config file mode = %o, want 0600", fi.Mode().Perm())
	}

	// Force overwrite.
	_, existed, err = Scaffold(true)
	if err != nil {
		t.Fatalf("Scaffold with force: %v", err)
	}
	if existed {
		t.Error("Scaffold with force on existing file should report existed = false")
	}
}

func TestScaffoldWithForce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWORN_CONFIG_PATH", configPath)

	// First create a custom config.
	_ = DefaultConfig()
	_ = os.WriteFile(configPath, []byte(`{"version":1,"verifier":{"model":"custom/model"}}`), 0600)
	// Force overwrite should replace with default.
	_, _, err := Scaffold(true)
	if err != nil {
		t.Fatalf("Scaffold force: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Verifier.Model != "" {
		t.Errorf("after force overwrite: verifier model = %q, want empty — "+
			"scaffold must not hardcode a model; `sworn init` asks", cfg.Verifier.Model)
	}
}

// --- S08 design system tests ---

func TestValidate_uiBearingWithoutDesignSystem(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "ui_bearing true without design_system fails closed",
			cfg:     Config{Version: 1, UIBearing: true, DesignSystem: nil},
			wantErr: true,
		},
		{
			name: "ui_bearing true with design_system succeeds",
			cfg: Config{
				Version:   1,
				UIBearing: true,
				DesignSystem: &DesignSystem{
					TokenSource:      "tokens.json",
					ComponentLibrary: "packages/ui",
				},
			},
			wantErr: false,
		},
		{
			name:    "ui_bearing false without design_system succeeds (exempt)",
			cfg:     Config{Version: 1, UIBearing: false, DesignSystem: nil},
			wantErr: false,
		},
		{
			name:    "default config (not ui-bearing) succeeds",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_uiBearingErrorText(t *testing.T) {
	// AC1: UI-bearing without design system fails closed with a clear message.
	err := Config{Version: 1, UIBearing: true}.Validate()
	if err == nil {
		t.Fatal("expected error for ui_bearing without design_system")
	}
	msg := err.Error()
	if !contains(msg, "ui_bearing") || !contains(msg, "design_system") {
		t.Errorf("error should mention ui_bearing and design_system, got: %s", msg)
	}
}

func TestDesignSystem_DistinguishesThreeConcepts(t *testing.T) {
	// AC4: schema distinguishes design system (umbrella), token source (atoms),
	// component library (reusables).
	ds := &DesignSystem{
		TokenSource:      "src/design/tokens.json",
		ComponentLibrary: "packages/ui",
	}

	// The DesignSystem struct itself IS the umbrella.
	// TokenSource and ComponentLibrary are its fields.
	if ds.TokenSource != "src/design/tokens.json" {
		t.Errorf("TokenSource = %q, want src/design/tokens.json", ds.TokenSource)
	}
	if ds.ComponentLibrary != "packages/ui" {
		t.Errorf("ComponentLibrary = %q, want packages/ui", ds.ComponentLibrary)
	}

	// Validate through Config as well.
	cfg := Config{
		Version:      1,
		UIBearing:    true,
		DesignSystem: ds,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should validate: %v", err)
	}
}

func TestDesignSystem_JSONRoundTrip(t *testing.T) {
	// AC3: valid design_system parses and exposes token source + component library.
	src := `{"version":1,"ui_bearing":true,"design_system":{"token_source":"tokens/dtcg.json","component_library":"packages/react-ui"}}`
	var cfg Config
	if err := json.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !cfg.UIBearing {
		t.Error("UIBearing should be true")
	}
	if cfg.DesignSystem == nil {
		t.Fatal("DesignSystem should not be nil")
	}
	if cfg.DesignSystem.TokenSource != "tokens/dtcg.json" {
		t.Errorf("TokenSource = %q, want tokens/dtcg.json", cfg.DesignSystem.TokenSource)
	}
	if cfg.DesignSystem.ComponentLibrary != "packages/react-ui" {
		t.Errorf("ComponentLibrary = %q, want packages/react-ui", cfg.DesignSystem.ComponentLibrary)
	}

	// Round-trip: marshal and unmarshal again.
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var cfg2 Config
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("Round-trip Unmarshal: %v", err)
	}
	if !cfg2.UIBearing {
		t.Error("Round-trip: UIBearing lost")
	}
	if cfg2.DesignSystem == nil || cfg2.DesignSystem.TokenSource != "tokens/dtcg.json" {
		t.Error("Round-trip: DesignSystem lost")
	}
}

func TestDefaultConfig_NotUIBearing(t *testing.T) {
	// sworn itself is a CLI tool — default should not be UI-bearing.
	cfg := DefaultConfig()
	if cfg.UIBearing {
		t.Error("DefaultConfig should have UIBearing = false")
	}
	if cfg.DesignSystem != nil {
		t.Error("DefaultConfig should have DesignSystem = nil")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig should validate: %v", err)
	}
}

func TestDesignSystem_OmitEmptyOnFalse(t *testing.T) {
	// A non-UI-bearing config should not emit ui_bearing or design_system in JSON.
	cfg := DefaultConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if contains(string(data), "ui_bearing") {
		t.Errorf("non-UI-bearing config should not contain ui_bearing in JSON: %s", data)
	}
	if contains(string(data), "design_system") {
		t.Errorf("non-UI-bearing config should not contain design_system in JSON: %s", data)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- S09 implementer model config tests ---

func TestResolveImplementerModel_FlagWins(t *testing.T) {
	cfg := DefaultConfig()
	m, err := ResolveImplementerModel("openai/gpt-4.1", cfg, "", "", "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/gpt-4.1" {
		t.Errorf("got %q, want openai/gpt-4.1", m)
	}
}

func TestResolveImplementerModel_ConfigFallback(t *testing.T) {
	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			Model: "openai/gpt-4o",
		},
	}
	m, err := ResolveImplementerModel("", cfg, "", "", "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/gpt-4o" {
		t.Errorf("got %q, want openai/gpt-4o", m)
	}
}

func TestResolveImplementerModel_EscalationFallback(t *testing.T) {
	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/o3-mini", "openai/o3"},
		},
	}
	m, err := ResolveImplementerModel("", cfg, "", "", "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/o3-mini" {
		t.Errorf("got %q, want openai/o3-mini (first escalation)", m)
	}
}

func TestResolveImplementerModel_Error(t *testing.T) {
	cfg := Config{Version: 1}
	_, err := ResolveImplementerModel("", cfg, "", "", "quality", 0)
	if err == nil {
		t.Fatal("expected error when no implementer model is configured")
	}
	msg := err.Error()
	if !contains(msg, "sworn init") {
		t.Errorf("error should mention 'sworn init', got: %s", msg)
	}
	if !contains(msg, "implementer") {
		t.Errorf("error should name the unconfigured role, got: %s", msg)
	}
}

// writeLedger creates a temp ledger file with the given records and returns its path.
func writeLedger(t *testing.T, records []ledger.Record) string {
	t.Helper()
	f, err := os.CreateTemp("", "verdicts-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, r := range records {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	return f.Name()
}

func TestResolveImplementerModel_LedgerDefault(t *testing.T) {
	// Corpus: openai/gpt-4o dominates harness with 9/10 pass.
	records := []ledger.Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2},
		// Also a worse model to ensure ranking.
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "pass", Attempt: 3},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "pass", Attempt: 3},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "pass", Attempt: 3},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "pass", Attempt: 3},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "pass", Attempt: 3},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "fail", Attempt: 1},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "fail", Attempt: 1},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "fail", Attempt: 1},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "fail", Attempt: 1},
		{Model: "anthropic/claude-sonnet-4-20250514", SliceKind: "harness", Verdict: "fail", Attempt: 1},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	// No flag, no env, no config — should pick ledger-recommended model.
	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/gpt-4o-mini"},
		},
	}
	m, err := ResolveImplementerModel("", cfg, "harness", ledgerPath, "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/gpt-4o" {
		t.Errorf("got %q, want openai/gpt-4o (ledger recommendation)", m)
	}
}

func TestResolveImplementerModel_LedgerFlagWins(t *testing.T) {
	// Even with a confident ledger recommendation, an explicit flag wins.
	records := []ledger.Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	cfg := DefaultConfig()
	m, err := ResolveImplementerModel("anthropic/claude-sonnet-4-20250514", cfg, "harness", ledgerPath, "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("flag should win over ledger: got %q", m)
	}
}

func TestResolveImplementerModel_LedgerThinCorpusFallback(t *testing.T) {
	// Only 4 records — below MinSampleSize (5). Should fall through to escalation.
	records := []ledger.Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/o3-mini"},
		},
	}
	m, err := ResolveImplementerModel("", cfg, "harness", ledgerPath, "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/o3-mini" {
		t.Errorf("thin corpus should fall through to escalation: got %q, want openai/o3-mini", m)
	}
}

func TestResolveImplementerModel_LedgerAbsentCorpusFallback(t *testing.T) {
	// No ledger file at all — should fall through to escalation (byte-for-byte same as S09).
	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/o3-mini"},
		},
	}
	m, err := ResolveImplementerModel("", cfg, "harness", "/nonexistent/path/verdicts.jsonl", "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/o3-mini" {
		t.Errorf("absent corpus should fall through to escalation: got %q, want openai/o3-mini", m)
	}
}

func TestResolveImplementerModel_LedgerEmptySliceKind(t *testing.T) {
	// When sliceKind is empty, the ledger lookup is skipped entirely.
	records := []ledger.Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/o3-mini"},
		},
	}
	m, err := ResolveImplementerModel("", cfg, "", ledgerPath, "quality", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/o3-mini" {
		t.Errorf("empty sliceKind should skip ledger: got %q, want escalation openai/o3-mini", m)
	}
}

// --- S56 cost-aware routing tests ---

func TestResolveImplementerModel_CostModePicksCheapest(t *testing.T) {
	// Model A: 9/10 pass at $0.50 → $0.50 mean cost
	// Model B: 9/10 pass at $0.05 → $0.05 mean cost (cheaper)
	// Cost mode should pick B.
	records := []ledger.Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "fail", Attempt: 2, TotalCostUSD: 0.05},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/o3-mini"},
		},
	}
	m, err := ResolveImplementerModel("", cfg, "harness", ledgerPath, "cost", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/gpt-4o-mini" {
		t.Errorf("cost mode: got %q, want openai/gpt-4o-mini (cheaper)", m)
	}
}

func TestResolveImplementerModel_CostModeFlagWins(t *testing.T) {
	// Even in cost mode, an explicit --model flag wins.
	records := []ledger.Record{
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "fail", Attempt: 2, TotalCostUSD: 0.05},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	cfg := DefaultConfig()
	m, err := ResolveImplementerModel("anthropic/claude-sonnet-4-20250514", cfg, "harness", ledgerPath, "cost", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("flag should win over cost mode: got %q", m)
	}
}

func TestResolveImplementerModel_CostModeThinCorpusFallback(t *testing.T) {
	// Thin corpus (below MinSampleSize) in cost mode should fall through
	// to escalation — byte-for-byte same as quality mode fallback.
	records := []ledger.Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2, TotalCostUSD: 0.50},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/o3-mini"},
		},
	}
	m, err := ResolveImplementerModel("", cfg, "harness", ledgerPath, "cost", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/o3-mini" {
		t.Errorf("thin corpus in cost mode should fall to escalation: got %q, want openai/o3-mini", m)
	}
}

func TestResolveImplementerModel_CostModeViaConfig(t *testing.T) {
	// Cost mode via config.OptimizeMode, not CLI param.
	records := []ledger.Record{
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o", SliceKind: "harness", Verdict: "fail", Attempt: 2, TotalCostUSD: 0.50},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "pass", Attempt: 1, TotalCostUSD: 0.05},
		{Model: "openai/gpt-4o-mini", SliceKind: "harness", Verdict: "fail", Attempt: 2, TotalCostUSD: 0.05},
	}
	ledgerPath := writeLedger(t, records)
	defer os.Remove(ledgerPath)

	cfg := Config{
		Version:      1,
		OptimizeMode: "cost",
		Implementer: ModelSetting{
			EscalationModels: []string{"openai/o3-mini"},
		},
	}
	// No CLI param, no env — should read from config.OptimizeMode.
	m, err := ResolveImplementerModel("", cfg, "harness", ledgerPath, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m != "openai/gpt-4o-mini" {
		t.Errorf("cost mode via config: got %q, want openai/gpt-4o-mini", m)
	}
}

func TestResolveEscalationModels_FlagWins(t *testing.T) {
	cfg := DefaultConfig()
	flag := []string{"custom/model1", "custom/model2"}
	got := ResolveEscalationModels(flag, cfg)
	if len(got) != 2 || got[0] != "custom/model1" || got[1] != "custom/model2" {
		t.Errorf("got %v, want [custom/model1 custom/model2]", got)
	}
}

func TestResolveEscalationModels_ConfigUsed(t *testing.T) {
	cfg := Config{
		Version: 1,
		Implementer: ModelSetting{
			EscalationModels: []string{"cfg/model1", "cfg/model2"},
		},
	}
	got := ResolveEscalationModels(nil, cfg)
	if len(got) != 2 || got[0] != "cfg/model1" || got[1] != "cfg/model2" {
		t.Errorf("got %v, want [cfg/model1 cfg/model2]", got)
	}
}

func TestResolveEscalationModels_NoHardcodedLadder(t *testing.T) {
	// It used to return ["openai/gpt-4o-mini", "openai/gpt-4o", "openai/o3-mini",
	// "openai/o3"] — four stale, hardcoded models injected whenever nothing was
	// configured. The CAPTAIN then took entry [0], so the Rule 9 design-authority
	// role silently ran on gpt-4o-mini.
	if got := ResolveEscalationModels(nil, Config{Version: 1}); len(got) != 0 {
		t.Errorf("ResolveEscalationModels returned a hardcoded ladder %v — "+
			"an unconfigured ladder must be empty, not a guess", got)
	}

	// With an implementer configured, the ladder is just that model: no escalation.
	cfg := Config{Version: 1, Implementer: ModelSetting{Model: "some/impl"}}
	got := ResolveEscalationModels(nil, cfg)
	if len(got) != 1 || got[0] != "some/impl" {
		t.Errorf("got %v, want [some/impl]", got)
	}
}

func TestResolveMaxAttempts_FlagWins(t *testing.T) {
	cfg := Config{
		Version:     1,
		Implementer: ModelSetting{MaxAttempts: 3},
	}
	n := ResolveMaxAttempts(5, cfg)
	if n != 5 {
		t.Errorf("got %d, want 5 (flag)", n)
	}
}

func TestResolveMaxAttempts_ConfigUsed(t *testing.T) {
	cfg := Config{
		Version:     1,
		Implementer: ModelSetting{MaxAttempts: 7},
	}
	n := ResolveMaxAttempts(-1, cfg)
	if n != 7 {
		t.Errorf("got %d, want 7 (config)", n)
	}
}

func TestResolveMaxAttempts_DefaultFallback(t *testing.T) {
	cfg := Config{Version: 1}
	n := ResolveMaxAttempts(-1, cfg)
	if n != 3 {
		t.Errorf("got %d, want 3 (default)", n)
	}
}

func TestConfigRoundTrip_ImplementerFields(t *testing.T) {
	cfg := Config{
		Version: 1,
		Verifier: ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
		Implementer: ModelSetting{
			Model:            "openai/gpt-4o-mini",
			EscalationModels: []string{"openai/gpt-4o", "openai/o3"},
			MaxAttempts:      4,
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var cfg2 Config
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg2.Implementer.Model != "openai/gpt-4o-mini" {
		t.Errorf("Model = %q", cfg2.Implementer.Model)
	}
	if len(cfg2.Implementer.EscalationModels) != 2 {
		t.Errorf("EscalationModels len = %d, want 2", len(cfg2.Implementer.EscalationModels))
	}
	if cfg2.Implementer.MaxAttempts != 4 {
		t.Errorf("MaxAttempts = %d, want 4", cfg2.Implementer.MaxAttempts)
	}
}

func TestDefaultConfig_ImplementerHasNoHardcodedModels(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Implementer.Model != "" {
		t.Errorf("Implementer.Model = %q, want empty — a hardcoded default silently "+
			"ran users on a stale model and defeated every not-configured error", cfg.Implementer.Model)
	}
	if len(cfg.Implementer.EscalationModels) != 0 {
		t.Errorf("Implementer.EscalationModels = %v, want empty", cfg.Implementer.EscalationModels)
	}
}

// --- S17 config Save tests ---

func TestSave_WritesFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	t.Setenv("SWORN_CONFIG_PATH", configPath)

	cfg := Config{
		Version: 1,
		Verifier: ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
		Implementer: ModelSetting{
			Model:            "openai/gpt-4o-mini",
			EscalationModels: []string{"openai/gpt-4o", "openai/o3"},
			MaxAttempts:      3,
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read back and verify round-trip.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Verifier.Model != "anthropic/claude-sonnet-4-6" {
		t.Errorf("Verifier.Model = %q, want anthropic/claude-sonnet-4-6", loaded.Verifier.Model)
	}
	if loaded.Implementer.Model != "openai/gpt-4o-mini" {
		t.Errorf("Implementer.Model = %q, want openai/gpt-4o-mini", loaded.Implementer.Model)
	}
	if len(loaded.Implementer.EscalationModels) != 2 {
		t.Errorf("EscalationModels len = %d, want 2", len(loaded.Implementer.EscalationModels))
	}
	if loaded.Implementer.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", loaded.Implementer.MaxAttempts)
	}
}

func TestSave_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	// Point config path at a subdirectory that doesn't exist.
	configPath := filepath.Join(dir, "does-not-exist", "sub", "config.json")
	t.Setenv("SWORN_CONFIG_PATH", configPath)

	cfg := Config{
		Version: 1,
		Verifier: ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save with non-existent parent dirs: %v", err)
	}

	// Verify the file was created.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file was not created at %s", configPath)
	}

	// Verify we can load it back.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save with new dirs: %v", err)
	}
	if loaded.Verifier.Model != "anthropic/claude-sonnet-4-6" {
		t.Errorf("Verifier.Model = %q", loaded.Verifier.Model)
	}
}

// TestResolveRoleModel is the guard for the unified role resolution.
//
// Four roles used to resolve four different ways, and two of them improvised:
//   - the planner ended in `return "openai/gpt-4o", nil` — a hardcoded, stale model;
//   - the captain took escalationModels[0], the cheapest rung of a RETRY ladder, so
//     the role holding Rule 9 design authority silently ran on the weakest model.
//
// Neither was ever decided. Both were artefacts of a call site improvising.
func TestResolveRoleModel(t *testing.T) {
	full := Config{
		Version:     1,
		Verifier:    ModelSetting{Model: "v/model"},
		Implementer: ModelSetting{Model: "i/model"},
		Planner:     ModelSetting{Model: "p/model"},
		Captain:     ModelSetting{Model: "c/model"},
	}
	// Only the two roles a project MUST configure.
	minimal := Config{
		Version:     1,
		Verifier:    ModelSetting{Model: "v/model"},
		Implementer: ModelSetting{Model: "i/model"},
	}

	tests := []struct {
		name string
		role Role
		flag string
		cfg  Config
		want string
	}{
		{name: "flag overrides everything", role: RoleVerifier, flag: "flag/model", cfg: full, want: "flag/model"},
		{name: "each role reads its own config", role: RoleCaptain, cfg: full, want: "c/model"},
		{name: "planner falls back to implementer (planning is authoring)", role: RolePlanner, cfg: minimal, want: "i/model"},
		{name: "captain falls back to VERIFIER, not the cheapest escalation rung", role: RoleCaptain, cfg: minimal, want: "v/model"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveRoleModel(tc.role, tc.flag, tc.cfg)
			if err != nil {
				t.Fatalf("ResolveRoleModel(%s): %v", tc.role, err)
			}
			if got != tc.want {
				t.Errorf("ResolveRoleModel(%s) = %q, want %q", tc.role, got, tc.want)
			}
		})
	}
}

// TestResolveRoleModel_FailsClosed — an unconfigured role is an ERROR with a remedy,
// never a hardcoded guess. This is what the old defaults made impossible.
func TestResolveRoleModel_FailsClosed(t *testing.T) {
	empty := Config{Version: 1}

	for _, role := range []Role{RolePlanner, RoleImplementer, RoleVerifier, RoleCaptain} {
		t.Run(string(role), func(t *testing.T) {
			got, err := ResolveRoleModel(role, "", empty)
			if err == nil {
				t.Fatalf("ResolveRoleModel(%s) returned %q with nothing configured — "+
					"it must fail closed, not guess a model", role, got)
			}
			if !contains(err.Error(), string(role)) {
				t.Errorf("error must name the unconfigured role, got: %v", err)
			}
			if !contains(err.Error(), "sworn init") {
				t.Errorf("error must name the remedy, got: %v", err)
			}
		})
	}
}

// TestResolveRoleModel_NoEnvLayer — config.json is the single source. A per-role env
// var was a second source that drifted, and drift is what made `sworn llm-check`
// report "no model configured" on a fully-configured setup.
func TestResolveRoleModel_NoEnvLayer(t *testing.T) {
	cfg := Config{Version: 1, Verifier: ModelSetting{Model: "from/config"}}

	for _, env := range []string{"SWORN_VERIFIER_MODEL", "SWORN_MODEL", "SWORN_CAPTAIN_MODEL", "SWORN_PLANNER_MODEL"} {
		t.Setenv(env, "env/should-be-ignored")
	}

	got, err := ResolveRoleModel(RoleVerifier, "", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != "from/config" {
		t.Errorf("resolved %q — an env var overrode config.json; there must be no env layer", got)
	}
}
