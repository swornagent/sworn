package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != 1 {
		t.Errorf("DefaultConfig version = %d, want 1", cfg.Version)
	}
	if cfg.Verifier.Model == "" {
		t.Error("DefaultConfig Verifier.Model is empty")
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
	cfg := Config{Version: 1, Verifier: ModelSetting{Model: "openai/gpt-4.1"}}

	t.Run("flag wins", func(t *testing.T) {
		m, err := ResolveVerifierModel("anthropic/claude-sonnet-4-20250514", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if m != "anthropic/claude-sonnet-4-20250514" {
			t.Errorf("got %q", m)
		}
	})

	t.Run("env wins over config", func(t *testing.T) {
		t.Setenv("SWORN_VERIFIER_MODEL", "openai/gpt-4o")
		m, err := ResolveVerifierModel("", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if m != "openai/gpt-4o" {
			t.Errorf("got %q", m)
		}
	})

	t.Run("config fallback", func(t *testing.T) {
		m, err := ResolveVerifierModel("", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if m != "openai/gpt-4.1" {
			t.Errorf("got %q", m)
		}
	})
}

func TestResolveVerifierModelMissingKey(t *testing.T) {
	// Missing-key error path (Coach Pin 5: smoke test the error).
	t.Setenv("SWORN_VERIFIER_MODEL", "")
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
	if !contains(msg, "SWORN_VERIFIER_MODEL") {
		t.Errorf("error should mention SWORN_VERIFIER_MODEL, got: %s", msg)
	}
}

func TestScaffoldIdempotent(t *testing.T) {
	// Idempotency: second Scaffold without force returns ErrConfigExists (Coach Pin 2).
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
	if cfg.Verifier.Model != "openai/gpt-4.1" {
		t.Errorf("after force overwrite: model = %q, want openai/gpt-4.1", cfg.Verifier.Model)
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