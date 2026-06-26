package verify

import (
	"context"
	"sync"
	"testing"

	"github.com/swornagent/sworn/internal/verdict"
)

// TestConcurrentVerifySameInput runs N goroutines all calling verify.Run with
// the same Input and the same fakeVerifier concurrently. Every goroutine must
// return the same verdict (PASS) — the race detector is the primary assertion
// mechanism, proving no package-level state is corrupted by concurrent Run calls.
func TestConcurrentVerifySameInput(t *testing.T) {
	const goroutines = 4

	in := Input{
		SpecPath: writeTmp(t, "spec.md", "must do X"),
		DiffPath: writeTmp(t, "c.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "PASS — meets the spec", cost: 0.01},
	}

	var wg sync.WaitGroup
	results := make([]verdict.Result, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = Run(context.Background(), in)
		}(i)
	}
	wg.Wait()

	// All goroutines must return PASS (no cross-contamination, no races,
	// no panics).
	for i, r := range results {
		if r.Verdict != verdict.Pass {
			t.Errorf("goroutine %d: want PASS, got %s (exit code %d)", i, r.Verdict, r.ExitCode())
		}
		if r.ExitCode() != 0 {
			t.Errorf("goroutine %d: want exit code 0, got %d", i, r.ExitCode())
		}
	}
}

// TestConcurrentVerifyIndependentInputs runs two goroutines each with different
// specs and different mock verifiers concurrently. Each result must match its
// own expected verdict — no cross-contamination between the verification runs.
func TestConcurrentVerifyIndependentInputs(t *testing.T) {
	in1 := Input{
		SpecPath: writeTmp(t, "spec1.md", "must do X"),
		DiffPath: writeTmp(t, "diff1.diff", "+ did X"),
		Verifier: fakeVerifier{reply: "PASS — spec 1 satisfied", cost: 0.01},
	}
	in2 := Input{
		SpecPath: writeTmp(t, "spec2.md", "must do Y"),
		DiffPath: writeTmp(t, "diff2.diff", "+ did not do Y"),
		Verifier: fakeVerifier{reply: "FAIL — spec clause 3 not met", cost: 0.02},
	}

	var wg sync.WaitGroup
	var result1, result2 verdict.Result

	wg.Add(1)
	go func() {
		defer wg.Done()
		result1 = Run(context.Background(), in1)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		result2 = Run(context.Background(), in2)
	}()

	wg.Wait()

	// Result 1 must be PASS.
	if result1.Verdict != verdict.Pass {
		t.Errorf("input 1 (PASS verifier): want PASS, got %s (exit code %d)", result1.Verdict, result1.ExitCode())
	}
	if result1.ExitCode() != 0 {
		t.Errorf("input 1: want exit code 0, got %d", result1.ExitCode())
	}

	// Result 2 must be FAIL (independent failure, not cross-contaminated
	// by input 1's PASS verifier).
	if result2.Verdict != verdict.Fail {
		t.Errorf("input 2 (FAIL verifier): want FAIL, got %s (exit code %d)", result2.Verdict, result2.ExitCode())
	}
	if result2.ExitCode() != 1 {
		t.Errorf("input 2: want exit code 1, got %d", result2.ExitCode())
	}
}
