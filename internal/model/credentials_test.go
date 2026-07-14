package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyCredentials_RealMachineShape(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	t.Setenv(CredentialsPathEnv, credPath)
	// Isolate HOME: without this the test reads the DEVELOPER'S real ~/.sworn/.env
	// and "passes" against their keys, not the fixture's.
	t.Setenv("HOME", dir)
	ResetCredentialsCacheForTest()
	t.Cleanup(ResetCredentialsCacheForTest)

	// Clear any SWORN_-prefixed exports inherited from the developer's shell.
	for _, legacy := range legacyKeyEnv {
		t.Setenv(legacy, "")
	}

	// The shape found on the real machine: SWORN_-prefixed exports.
	t.Setenv("SWORN_OPENAI_API_KEY", "sk-openai")
	t.Setenv("SWORN_ANTHROPIC_API_KEY", "sk-anthropic")
	t.Setenv("SWORN_XAI_API_KEY", "sk-xai")

	// Before: the model layer cannot see them.
	if k := ProviderKey("openai"); k != "" {
		t.Fatalf("SWORN_-prefixed key was read: %q — those names are gone", k)
	}

	moved, err := MigrateLegacyCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 3 {
		t.Fatalf("migrated %v, want 3 providers", moved)
	}

	// After: resolvable, from the XDG file.
	for provider, want := range map[string]string{"openai": "sk-openai", "anthropic": "sk-anthropic", "xai": "sk-xai"} {
		if got := ProviderKey(provider); got != want {
			t.Errorf("ProviderKey(%s) = %q, want %q", provider, got, want)
		}
	}

	// 0600 — a credentials file readable by the group is a credentials file leaked.
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("credentials.json mode = %v, want 0600", info.Mode().Perm())
	}
	t.Logf("migrated %v -> %s (mode %v)", moved, credPath, info.Mode().Perm())
}

func TestProviderKey_CanonicalEnvBeatsFile(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	os.WriteFile(credPath, []byte(`{"providers":{"openai":"from-file"}}`), 0600)
	t.Setenv(CredentialsPathEnv, credPath)
	ResetCredentialsCacheForTest()
	t.Cleanup(ResetCredentialsCacheForTest)

	if got := ProviderKey("openai"); got != "from-file" {
		t.Errorf("got %q, want from-file", got)
	}
	t.Setenv("OPENAI_API_KEY", "from-env")
	if got := ProviderKey("openai"); got != "from-env" {
		t.Errorf("got %q — the canonical env var must win (CI/12-factor injection)", got)
	}
}
