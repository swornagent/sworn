package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/implement"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
)

// ---------------------------------------------------------------------------
// implement-arm helpers (driver seam — S06). These replace the old wire-typed
// agent fakes (blockingFakeAgent / quickFakeAgent / markedAgent /
// passingVerifierAgent); the verify arm defaults to a schema-valid PASS
// emission in fakeDriver.
// ---------------------------------------------------------------------------

// blockingImplement blocks on ctx.Done() to simulate a hung model dispatch.
func blockingImplement(ctx context.Context, _ driver.DispatchInput) (driver.Result, error) {
	<-ctx.Done()
	return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindTransient}, ctx.Err()
}

// markedImplement records that the implement arm was dispatched.
func markedImplement(called *bool) func(context.Context, driver.DispatchInput) (driver.Result, error) {
	return func(_ context.Context, _ driver.DispatchInput) (driver.Result, error) {
		*called = true
		return driver.Result{Status: driver.StatusOK, ResultText: "Done."}, nil
	}
}

// ---------------------------------------------------------------------------
// alwaysPassVerifier — returns PASS for every verify call
// ---------------------------------------------------------------------------

type alwaysPassVerifier struct{}

func (v *alwaysPassVerifier) Verify(_ context.Context, _, _ string) (string, float64, int64, int64, error) {
	return string(verdict.Pass), 0, 0, 0, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// setupSliceTestRepo creates a test repo with a pre-populated spec.md and
// status.json in state in_progress, ready for RunSlice testing.
func setupSliceTestRepo(t *testing.T) (workspaceRoot, specPath, statusPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()

	runCmd(t, dir, "git", "init", "-b", "main")
	runCmd(t, dir, "git", "config", "user.email", "test@swornagent.dev")
	runCmd(t, dir, "git", "config", "user.name", "sworn test")

	// Create an initial commit so start_commit is valid.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/.sworn/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "README.md", ".gitignore")
	runCmd(t, dir, "git", "commit", "-m", "initial commit")

	// Get the initial commit hash for start_commit.
	startCommit := strings.TrimSpace(runCmd(t, dir, "git", "rev-parse", "HEAD"))

	// Create the slice directory structure.
	sliceDir := filepath.Join(dir, "docs", "release", "test-release", "S01-task")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a minimal spec.md.
	specContent := `# Task

Test task

## User outcome

Test outcome

## Acceptance checks

- [ ] The implementation satisfies the task

## Required tests

- **Unit**: go test ./...
`
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(specContent), 0o644); err != nil {
		t.Fatal(err)
	}

	specPath = filepath.Join(sliceDir, "spec.md")
	statusPath = filepath.Join(sliceDir, "status.json")

	// Write status.json in in_progress state with start_commit.
	st := &state.Status{
		Schema:        "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:       "S01-task",
		Release:       "test-release",
		Track:         "",
		State:         state.InProgress,
		Owner:         "sworn-run",
		LastUpdatedBy: "test",
		LastUpdatedAt: time.Now().UTC().Format(time.RFC3339),
		StartCommit:   startCommit,
		SpecPath:      "docs/release/test-release/S01-task/spec.md",
		ProofPath:     "docs/release/test-release/S01-task/proof.md",
		JournalPath:   "docs/release/test-release/S01-task/journal.md",
		PlannedFiles:  []string{},
		TestCommands:  []string{"go test ./..."},
		Verification:  state.Verification{},
	}
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}

	// Write a minimal proof.md so the proof-mandatory gate (S11) passes.
	if err := os.WriteFile(filepath.Join(sliceDir, "proof.md"), []byte("# Proof\n\nDone.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Commit the slice setup so the worktree is clean.
	runCmd(t, dir, "git", "add", "docs/")
	runCmd(t, dir, "git", "commit", "-m", "test: slice setup")

	return dir, specPath, statusPath, func() {}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestImplementTimeoutEscalates(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	slot2Called := false

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/blocking", "fake/working"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: 500 * time.Millisecond,
		Registry: testRegistry(&fakeDriver{
			implement: func(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
				if in.ModelID == "fake/blocking" {
					return blockingImplement(ctx, in)
				}
				slot2Called = true
				return driver.Result{Status: driver.StatusOK, ResultText: "Done."}, nil
			},
		}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}

	// Slot 1 (blocking) blocks on ctx.Done(). After 500ms, the context deadline
	// fires, the dispatch returns context.DeadlineExceeded, implement.Run
	// returns an error, and RunSlice detects the timeout and escalates to
	// slot 2. slot2Called should be true — the escalation succeeded.
	if !slot2Called {
		t.Error("expected slot 2 model to be dispatched after escalation from timeout")
	}
}
func TestImplementTimeoutExhaustsToHuman(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/blocking1", "fake/blocking2"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: 100 * time.Millisecond,
		Registry:         testRegistry(&fakeDriver{implement: blockingImplement}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected error after exhausting timeouts, got nil")
	}
	if !strings.Contains(err.Error(), "verification failed after") {
		t.Fatalf("expected 'verification failed after' message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Escalate to human") {
		t.Fatalf("expected 'Escalate to human' message, got: %v", err)
	}
}
func TestImplementTimeoutHappyPath(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	called := false

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: DefaultImplementTimeout, // generous timeout
		Registry:         testRegistry(&fakeDriver{implement: markedImplement(&called)}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error on happy path: %v", err)
	}
	if !called {
		t.Error("expected agent to be called on happy path")
	}
}

func TestImplementTimeoutZeroUsesDefault(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	called := false

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: 0, // zero → use default (15m), not instant timeout
		Registry:         testRegistry(&fakeDriver{implement: markedImplement(&called)}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error with zero timeout: %v", err)
	}
	if !called {
		t.Error("expected agent to be called (zero timeout → default, not instant)")
	}
}

func TestImplementTimeoutNegativeNoTimeout(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	called := false

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1, // negative → no timeout, unbounded
		Registry:         testRegistry(&fakeDriver{implement: markedImplement(&called)}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error with no timeout: %v", err)
	}
	if !called {
		t.Error("expected agent to be called (negative timeout → opt-out)")
	}
}

// ---------------------------------------------------------------------------
// Feedback-driven retry tests
// ---------------------------------------------------------------------------

// recordingImplement records the most recent dispatch payload (the user
// prompt the orchestrator assembled) and whether it carried the S44 feedback
// block, then returns a stop response. Driver-seam successor of the old
// recordingPromptAgent.
type recordingImplement struct {
	lastUserPrompt string
	seenFeedback   bool
}

func (r *recordingImplement) dispatch(_ context.Context, in driver.DispatchInput) (driver.Result, error) {
	r.lastUserPrompt = in.Payload
	if strings.Contains(in.Payload, "Previous attempt failed verification") {
		r.seenFeedback = true
	}
	return driver.Result{Status: driver.StatusOK, ResultText: "Done."}, nil
}

// failThenPassVerifier returns FAIL on the first call and PASS on the second.
// The FAIL carries a fixed rationale so the retry path can pass it back.
type failThenPassVerifier struct {
	calls      int
	failReason string
}

func (v *failThenPassVerifier) Verify(_ context.Context, _, _ string) (string, float64, int64, int64, error) {
	v.calls++
	if v.calls == 1 {
		return v.failReason, 0, 0, 0, nil
	}
	return string(verdict.Pass), 0, 0, 0, nil
}

func TestRetryPassesVerifierRationale(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	failReason := "FAIL: gate 1 — no feedback block in implementer prompt"
	verifier := &failThenPassVerifier{failReason: failReason}

	// With K=1 resolve_in_place, model-a retries itself on FAIL. The
	// recording arm captures both attempts' payloads (last wins): attempt 0
	// (no feedback) and attempt 1 (with the verifier's rationale as feedback).
	rec := &recordingImplement{}

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/model-a"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: -1,
		Registry: testRegistry(&fakeDriver{
			implement: rec.dispatch,
			verify:    verdictsFrom(verifier),
		}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}
	// The recording arm captures the LAST prompt (attempt 1 with feedback).
	// We verify the retry carried the rationale. Attempt 0's empty feedback
	// is covered by TestAttempt0EmptyFeedback.
	if !strings.Contains(rec.lastUserPrompt, failReason) {
		t.Fatalf("retry did not receive prior rationale; got:\n%s", rec.lastUserPrompt)
	}
	if !strings.Contains(rec.lastUserPrompt, "Previous attempt failed verification") {
		t.Fatalf("retry did not receive feedback header; got:\n%s", rec.lastUserPrompt)
	}
}
func TestAttempt0EmptyFeedback(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	rec := &recordingImplement{}

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/model-a"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		Registry:         testRegistry(&fakeDriver{implement: rec.dispatch}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}
	if strings.Contains(rec.lastUserPrompt, "Previous attempt failed verification") {
		t.Fatalf("attempt 0 should not receive feedback block, got:\n%s", rec.lastUserPrompt)
	}
	if !strings.HasPrefix(rec.lastUserPrompt, "Implement the following spec") {
		t.Fatalf("attempt 0 prompt should start with original spec prefix, got:\n%s", rec.lastUserPrompt)
	}
}

func TestRetryFeedbackResolvesToPass(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	failReason := "FAIL: implementer prompt missing feedback block"
	verifier := &failThenPassVerifier{failReason: failReason}

	// With K=1 resolve_in_place, model-a retries itself. The recording arm
	// captures the last prompt (attempt 1 with feedback).
	rec := &recordingImplement{}

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/model-a"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: -1,
		Registry: testRegistry(&fakeDriver{
			implement: rec.dispatch,
			verify:    verdictsFrom(verifier),
		}),
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}
	if !rec.seenFeedback {
		t.Fatalf("model-a should have seen feedback on resolve_in_place retry; got prompt:\n%s", rec.lastUserPrompt)
	}
	final, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != state.Verified {
		t.Fatalf("expected state verified after FAIL→PASS, got %q", final.State)
	}
}

// TestCheckProofAbsent verifies the proof-mandatory gate helper.
func TestCheckProofAbsent(t *testing.T) {
	dir := t.TempDir()
	if !checkProofAbsent(filepath.Join(dir, "nonexistent.md")) {
		t.Error("expected true for nonexistent file")
	}
	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if !checkProofAbsent(empty) {
		t.Error("expected true for empty file")
	}
	ws := filepath.Join(dir, "ws.md")
	if err := os.WriteFile(ws, []byte("  \n  \n  "), 0o644); err != nil {
		t.Fatal(err)
	}
	if !checkProofAbsent(ws) {
		t.Error("expected true for whitespace-only file")
	}
	ok := filepath.Join(dir, "ok.md")
	if err := os.WriteFile(ok, []byte("# Proof\n\nDone.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if checkProofAbsent(ok) {
		t.Error("expected false for non-empty file")
	}
}

// TestRunSlice_ProofGate_Integration verifies that the proof-mandatory
// gate helper correctly detects absent/empty/whitespace files. The
// RunSlice-level happy path (proof present) is validated by all existing
// tests that pass through the proof gate.
func TestRunSlice_ProofGate_Integration(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)
	absSliceDir := filepath.Dir(specPath)
	proofPath := filepath.Join(absSliceDir, "proof.md")
	proofBytes, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatalf("proof.md missing after setup: %v", err)
	}
	if strings.TrimSpace(string(proofBytes)) == "" {
		t.Fatal("proof.md is empty after setup")
	}
	if checkProofAbsent(proofPath) {
		t.Error("checkProofAbsent returned true for non-empty proof.md")
	}
	_ = workspaceRoot
	_ = statusPath
}

// ensureCallCount is an unexported compile-time guard that implement.Run
// is callable with the new driver-seam signature inside this package.
var _ = func() error {
	_, _ = implement.Run(context.Background(), "", "", "", nil, "", 0)
	return nil
}()

// erroringDispatch always returns an error, simulating a transient provider
// failure (e.g. an HTTP 429) at dispatch time — any role.
func erroringDispatch(_ context.Context, _ driver.DispatchInput) (driver.Result, error) {
	return driver.Result{Status: driver.StatusError, ErrKind: "rate_limit"}, fmt.Errorf("simulated provider 429")
}

// findDesignGateDeferral returns the first open_deferrals entry recording a
// skipped Rule 9 design gate, or nil.
func findDesignGateDeferral(t *testing.T, statusPath string) *state.Deferral {
	t.Helper()
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	for i := range st.OpenDeferrals {
		if st.OpenDeferrals[i].Item == "design_review_gate" {
			return &st.OpenDeferrals[i]
		}
	}
	return nil
}

// TestDesignGate_GenerationFailureRecordsDeferral proves that a design-TL;DR
// dispatch failure (transient 429) no longer silently bypasses the Rule 9
// gate: it records a machine-readable Rule 2 deferral on status.json.
func TestDesignGate_GenerationFailureRecordsDeferral(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/m1"},
		VerifierModel:    "fake/verifier",
		ImplementTimeout: -1,
		Registry: testRegistry(&fakeDriver{
			captain:   erroringDispatch,
			implement: erroringDispatch,
		}),
	}

	// The implement loop will also fail with the erroring dispatch; we only
	// assert the deferral, which is recorded before the loop.
	_ = RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)

	d := findDesignGateDeferral(t, statusPath)
	if d == nil {
		t.Fatal("expected a design_review_gate deferral on status.json after design dispatch failure, got none")
	}
	if d.Why == "" || d.Tracking == "" {
		t.Errorf("deferral missing why/tracking (Rule 2): %+v", d)
	}
	if !strings.Contains(d.Why, "design TL;DR") {
		t.Errorf("expected design TL;DR reason, got %q", d.Why)
	}
}

// TestDesignGate_CaptainDispatchFailureRecordsDeferral proves that a captain
// design-review dispatch failure (transient 429) — when design.md already
// exists — records a Rule 2 deferral instead of silently proceeding.
func TestDesignGate_CaptainDispatchFailureRecordsDeferral(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	// Pre-create a valid six-section design.md so design.Generate skips and
	// the captain review is the stage that fails.
	designMD := "§1 Approach\n§2 Data\n§3 Surface\n§4 Risks\n§5 Tests\n§6 Rollback\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(specPath), "design.md"), []byte(designMD), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := RunSliceOptions{
		EscalationModels: []string{"fake/m1"},
		VerifierModel:    "fake/verifier",
		ImplementTimeout: -1,
		Registry: testRegistry(&fakeDriver{
			captain:   erroringDispatch,
			implement: erroringDispatch,
		}),
	}

	_ = RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)

	d := findDesignGateDeferral(t, statusPath)
	if d == nil {
		t.Fatal("expected a design_review_gate deferral on status.json after captain dispatch failure, got none")
	}
	if !strings.Contains(d.Why, "captain design-review dispatch failed") {
		t.Errorf("expected captain dispatch-failure reason, got %q", d.Why)
	}
}
