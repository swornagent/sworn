package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withModelConfig writes a config.json declaring every role's model and points
// config.Load() at it.
//
// Required because model selection now lives in config.json with NO hardcoded
// default: an unconfigured role is a fail-closed error. Tests that exercise a
// command's happy path must therefore CONFIGURE it, exactly as a user must.
//
// These tests previously passed by accident — twice over. On a developer machine
// they silently read the real ~/.config/sworn/config.json; in CI they fell through
// to DefaultConfig's hardcoded "openai/gpt-4o-mini". Deleting that hardcoded default
// is what surfaced them: the default had been quietly answering for the user, which
// is precisely why no "not configured" error could ever fire.
func withModelConfig(t *testing.T) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	const cfg = `{
  "version": 1,
  "verifier":    {"model": "openai/test-verifier"},
  "implementer": {"model": "openai/test-implementer"},
  "planner":     {"model": "openai/test-planner"},
  "captain":     {"model": "openai/test-captain"}
}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SWORN_CONFIG_PATH", cfgPath)
}
