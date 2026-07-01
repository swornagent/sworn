package run

import (
	"context"
	"testing"
	"time"
)

// TestRunSliceDefaultsNilFactories is the S27 reachability test for the
// parallel-dispatch SIGSEGV: cmd/sworn/run.go's parallel runSliceFn constructs
// RunSliceOptions WITHOUT NewAgent/NewVerifier, so they were nil and RunSlice
// dereferenced them at the design-TL;DR dispatch (slice.go) and again at the
// verify step — a nil-pointer panic before any model was contacted. RunSlice
// must default them (mirroring run.Run) so the parallel loop can dispatch.
//
// The test runs with an unconfigured model ("bogus/none"): model.FromEnv errors
// synchronously for an unknown provider, so the default factory is exercised with
// NO network call. Before the fix this panics; after the fix it returns an error.
func TestRunSliceDefaultsNilFactories(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		ImplementerModel: "bogus/none",
		VerifierModel:    "bogus/none",
		EscalationModels: []string{}, // stay on bogus/none; never fall back to real openai defaults
		RetryCap:         1,
		ImplementTimeout: 2 * time.Second,
		// NewAgent / NewVerifier intentionally nil — RunSlice must default them,
		// not dereference nil.
	}

	// The assertion is twofold: (1) no panic (the test reaching this line proves
	// the nil-factory path is safe), (2) an unconfigured model yields an error.
	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected RunSlice to error on an unconfigured model, got nil")
	}
}
