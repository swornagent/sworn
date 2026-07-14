package main

import (
	"os"
	"path/filepath"
	"testing"
)

// cmdVerify's default path is the deterministic first-pass — no model dispatch.
// These tests drive the CLI dispatch integration point (Rule 1) and assert the
// fail-closed proof/spec gates (Rule 6): a missing/empty/malformed proof, an
// absent proof, and an empty spec must all exit non-zero, never PASS/exit 0.
func TestCmdVerify_FailClosed(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	if err := os.WriteFile(spec, []byte("AC-1: it works."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diff, []byte("+ new line"), 0644); err != nil {
		t.Fatal(err)
	}
	emptySpec := filepath.Join(dir, "empty-spec.md")
	os.WriteFile(emptySpec, []byte("  \n"), 0644)
	emptyProof := filepath.Join(dir, "empty-proof.md")
	os.WriteFile(emptyProof, []byte(""), 0644)
	badJSONProof := filepath.Join(dir, "proof.json")
	os.WriteFile(badJSONProof, []byte("{not json"), 0644)
	goodProof := filepath.Join(dir, "proof.md")
	os.WriteFile(goodProof, []byte("# Proof\nScope: ok"), 0644)

	// Isolate config + provider resolution: nonexistent config -> DefaultConfig
	// (non-UI-bearing, Validate passes); a resolvable verifier model with a
	// present (unused, default path never dispatches) key so model.FromEnv
	// succeeds without network.
	// Model selection lives in config.json — there is no env layer.
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"version":1,"verifier":{"model":"openai/gpt-4.1-mini"}}`), 0644)
	t.Setenv("SWORN_CONFIG_PATH", cfgPath)
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("OPENAI_API_KEY", "test-key-not-dispatched")

	cases := []struct {
		name string
		args []string
	}{
		{"no_proof_flag", []string{"--spec", spec, "--diff", diff}},
		{"missing_proof_file", []string{"--spec", spec, "--diff", diff, "--proof", filepath.Join(dir, "nope.md")}},
		{"empty_proof", []string{"--spec", spec, "--diff", diff, "--proof", emptyProof}},
		{"malformed_json_proof", []string{"--spec", spec, "--diff", diff, "--proof", badJSONProof}},
		{"empty_spec", []string{"--spec", emptySpec, "--diff", diff, "--proof", goodProof}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exit := cmdVerify(tc.args)
			if exit == 0 {
				t.Fatalf("fail-open: cmdVerify %v returned exit 0 (PASS) — expected non-zero (fail closed)", tc.args)
			}
		})
	}
}

// A well-formed invocation (non-empty spec+diff, present non-empty proof) still
// PASSes the deterministic first-pass — the gate is fail-closed, not fail-shut.
func TestCmdVerify_WellFormedPasses(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	diff := filepath.Join(dir, "diff.patch")
	proof := filepath.Join(dir, "proof.md")
	os.WriteFile(spec, []byte("AC-1: it works."), 0644)
	os.WriteFile(diff, []byte("+ added"), 0644)
	os.WriteFile(proof, []byte("# Proof\nScope: ok"), 0644)

	// Model selection lives in config.json — there is no env layer.
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"version":1,"verifier":{"model":"openai/gpt-4.1-mini"}}`), 0644)
	t.Setenv("SWORN_CONFIG_PATH", cfgPath)
	t.Setenv("SWORN_DIRECT", "1")
	t.Setenv("OPENAI_API_KEY", "test-key-not-dispatched")

	exit := cmdVerify([]string{"--spec", spec, "--diff", diff, "--proof", proof})
	if exit != 0 {
		t.Fatalf("expected exit 0 (PASS) for well-formed first-pass, got %d", exit)
	}
}
