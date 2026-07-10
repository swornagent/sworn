package run

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRunSliceDefaultsNilRegistry is the S06 successor of the S27
// nil-factory SIGSEGV reachability test: cmd/sworn's parallel runSliceFn
// constructs RunSliceOptions without a Registry, so RunSlice must default it
// to registry.Default(model.ProviderConfigFromEnv()) — never dereference nil.
// With the factories deleted, the SIGSEGV class is unrepresentable: an
// unregistered prefix now fails at upfront resolution (AC-02), by name,
// BEFORE any dispatch.
func TestRunSliceDefaultsNilRegistry(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		ImplementerModel: "bogus/none",
		VerifierModel:    "bogus/none",
		EscalationModels: []string{}, // stay on bogus/none; never fall back to real openai defaults
		RetryCap:         1,
		ImplementTimeout: 2 * time.Second,
		// Registry intentionally nil — RunSlice must default it, not
		// dereference nil.
	}

	// The assertion is twofold: (1) no panic (the test reaching this line
	// proves the nil-registry path is safe), (2) an unregistered prefix
	// yields the registry's descriptive resolution error with NO dispatch.
	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected RunSlice to error on an unregistered prefix, got nil")
	}
	if !strings.Contains(err.Error(), `"bogus/none"`) {
		t.Errorf("resolution error should name the model ID, got: %v", err)
	}
	if !strings.Contains(err.Error(), "registered prefixes") {
		t.Errorf("resolution error should enumerate registered prefixes, got: %v", err)
	}
}
