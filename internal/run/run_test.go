package run

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/driver/registry"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
)

// structuredVerdictReply bridges the pre-agentic scripted verifiers (which
// return a leading PASS/FAIL/BLOCKED/INCONCLUSIVE prose token) onto the
// ADR-0011 structured authoring path: it translates that token into a
// schema-valid verifier-verdict-v1 emission, supplying a violation for
// FAIL/BLOCKED (which the schema requires). Shared by the verifier test fakes'
// ChatStructured methods.
func structuredVerdictReply(text string) string {
	// Tolerate the markdown/fence wrapping the legacy scripted verifiers used
	// (e.g. "**PASS**", "```\nPASS"); a real structured emitter never wraps, but
	// these fakes predate the structured path.
	stripped := strings.TrimSpace(text)
	stripped = strings.TrimPrefix(stripped, "```")
	stripped = strings.TrimLeft(strings.TrimSpace(stripped), "*_`")
	upper := strings.ToUpper(strings.TrimSpace(stripped))
	rationale := strings.TrimSpace(text)
	obj := map[string]any{}
	switch {
	case strings.HasPrefix(upper, "PASS"):
		obj["verdict"] = "PASS"
	case strings.HasPrefix(upper, "FAIL"):
		obj["verdict"] = "FAIL"
		obj["violations"] = []map[string]string{{"gate": "adversarial", "description": rationale}}
	case strings.HasPrefix(upper, "BLOCKED"):
		obj["verdict"] = "BLOCKED"
		obj["violations"] = []map[string]string{{"gate": "adversarial", "description": rationale}}
	default:
		obj["verdict"] = "INCONCLUSIVE"
	}
	if rationale == "" {
		rationale = obj["verdict"].(string)
	}
	obj["rationale"] = rationale
	b, _ := json.Marshal(obj)
	return string(b)
}

// ---------------------------------------------------------------------------
// fakeDriver — the S06 test seam: tests inject fake drivers through a test
// registry (AC-01), never through factories.
// ---------------------------------------------------------------------------

type fakeDriver struct {
	name  string
	roles driver.RoleSet // nil → all three roles

	// Per-role dispatch behaviour. Nil arms fall back to a benign default:
	// implement → StatusOK "Done."; captain → StatusOK prose (no §-headers,
	// so the design gate defers, matching the old fakes' behaviour);
	// verify → a schema-valid PASS emission.
	implement func(ctx context.Context, in driver.DispatchInput) (driver.Result, error)
	verify    func(ctx context.Context, in driver.DispatchInput) (driver.Result, error)
	captain   func(ctx context.Context, in driver.DispatchInput) (driver.Result, error)

	mu    sync.Mutex
	calls []driver.DispatchInput
}

func (d *fakeDriver) Name() string {
	if d.name == "" {
		return "fake-driver"
	}
	return d.name
}

func (d *fakeDriver) Roles() driver.RoleSet {
	if d.roles == nil {
		return driver.RoleSet{driver.RoleImplementer: true, driver.RoleVerifier: true, driver.RoleCaptain: true}
	}
	return d.roles
}

func (d *fakeDriver) Dispatch(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
	d.mu.Lock()
	d.calls = append(d.calls, in)
	d.mu.Unlock()
	switch in.Role {
	case driver.RoleVerifier:
		if d.verify != nil {
			return d.verify(ctx, in)
		}
		return okStructured(structuredVerdictReply("PASS")), nil
	case driver.RoleCaptain:
		if d.captain != nil {
			return d.captain(ctx, in)
		}
		return driver.Result{Status: driver.StatusOK, ResultText: "captain judgement (no sections, no pins)"}, nil
	default:
		if d.implement != nil {
			return d.implement(ctx, in)
		}
		return driver.Result{Status: driver.StatusOK, ResultText: "Done."}, nil
	}
}

// dispatchCount returns how many Dispatch calls the fake served.
func (d *fakeDriver) dispatchCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

// dispatchedRoles returns the set of roles Dispatch was called with.
func (d *fakeDriver) dispatchedRoles() map[driver.Role]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[driver.Role]int{}
	for _, c := range d.calls {
		out[c.Role]++
	}
	return out
}

// okStructured builds a StatusOK verifier result carrying the emitted verdict.
func okStructured(emitted string) driver.Result {
	return driver.Result{Status: driver.StatusOK, StructuredJSON: json.RawMessage(emitted)}
}

// testRegistry builds a registry with d registered under the "fake" prefix
// (plus any extras) — the AC-01 injection contract.
func testRegistry(d driver.Driver, prefixes ...string) *registry.Registry {
	r := registry.New()
	if len(prefixes) == 0 {
		prefixes = []string{"fake"}
	}
	r.Register(registry.Entry{Driver: d, Prefixes: prefixes})
	return r
}

// writeFileImplementer returns an implement arm that writes output.txt into
// the dispatch worktree (the driver-seam successor of the old stdoutAgent
// tool-call script).
func writeFileImplementer(content string) func(context.Context, driver.DispatchInput) (driver.Result, error) {
	return func(_ context.Context, in driver.DispatchInput) (driver.Result, error) {
		if err := os.WriteFile(filepath.Join(in.WorktreeRoot, "output.txt"), []byte(content), 0o644); err != nil {
			return driver.Result{Status: driver.StatusError, ErrKind: "other"}, err
		}
		return driver.Result{Status: driver.StatusOK, ResultText: "Implementation complete."}, nil
	}
}

// proseVerifier is the scripted-verdict seam the legacy verifier fakes
// satisfy (fakeVerifier, textVerifier, failThenPassVerifier, ...). It is
// wire-free by construction: prose in, prose out.
type proseVerifier interface {
	Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, int64, int64, error)
}

// verdictsFrom adapts a scripted proseVerifier onto the driver verify arm,
// translating its prose verdict into a schema-valid verifier-verdict-v1
// emission (the same bridge the pre-S06 verifierAwareAgent provided).
func verdictsFrom(v proseVerifier) func(context.Context, driver.DispatchInput) (driver.Result, error) {
	return func(ctx context.Context, _ driver.DispatchInput) (driver.Result, error) {
		text, cost, _, _, err := v.Verify(ctx, "", "")
		if err != nil {
			return driver.Result{Status: driver.StatusError, ErrKind: "other"}, err
		}
		res := okStructured(structuredVerdictReply(text))
		res.CostUSD = cost
		return res, nil
	}
}

// ---------------------------------------------------------------------------
// Fake verifier — returns scripted verdicts
// ---------------------------------------------------------------------------

type fakeVerifier struct {
	verdicts []verdict.Result
	next     int
}

func (f *fakeVerifier) Verify(_ context.Context, _, _ string) (string, float64, int64, int64, error) {
	if f.next >= len(f.verdicts) {
		return "PASS", 0, 0, 0, nil
	}
	v := f.verdicts[f.next]
	f.next++
	return string(v.Verdict) + ": " + v.Rationale, v.CostUSD, 0, 0, nil
}

// ---------------------------------------------------------------------------
// textVerifier — returns a fixed raw reply, optionally capturing the system
// prompt. Used for S03 reachability tests that must inspect what prompt the
// run loop wired.
// ---------------------------------------------------------------------------

type textVerifier struct {
	reply   string
	capture *string
}

func (v *textVerifier) Verify(_ context.Context, systemPrompt, _ string) (string, float64, int64, int64, error) {
	if v.capture != nil {
		*v.capture = systemPrompt
	}
	return v.reply, 0, 0, 0, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------
func setupTestRepo(t *testing.T) (workspaceRoot string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()

	runCmd(t, dir, "git", "init", "-b", "main")
	runCmd(t, dir, "git", "config", "user.email", "test@swornagent.dev")
	runCmd(t, dir, "git", "config", "user.name", "sworn test")

	// Create an initial commit so we have a base branch.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Add .gitignore with .sworn/ so the process registry DB doesn't
	// interfere with git operations during test.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/.sworn/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "README.md", ".gitignore")
	runCmd(t, dir, "git", "commit", "-m", "initial commit")
	return dir, func() {}
}

func runCmd(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRun_PassPath_Merges(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Pass, Rationale: "all good"},
		},
	}

	err := Run(context.Background(), Options{
		Task:             "Write a hello file",
		VerifierModel:    "fake/verifier",
		Base:             "main",
		RetryCap:         0,
		WorkspaceRoot:    workspaceRoot,
		EscalationModels: []string{"fake/impl"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("hello from sworn run"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify the file was created.
	data, err := os.ReadFile(filepath.Join(workspaceRoot, "output.txt"))
	if err != nil {
		t.Fatalf("output.txt not created: %v", err)
	}
	if string(data) != "hello from sworn run" {
		t.Fatalf("expected 'hello from sworn run', got %q", string(data))
	}

	// Verify status.json state is verified.
	entries, err := filepath.Glob(filepath.Join(workspaceRoot, "docs", "release", "run-*", "S01-task", "status.json"))
	if err != nil || len(entries) == 0 {
		t.Fatal("status.json not found after run")
	}
	st, err := state.Read(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if st.State != state.Verified {
		t.Fatalf("expected state verified, got %q", st.State)
	}

	// Verify merge commit exists on main.
	runCmd(t, workspaceRoot, "git", "checkout", "main")
	log := runCmd(t, workspaceRoot, "git", "log", "--oneline", "-1")
	if !strings.Contains(log, "merge:") {
		t.Fatalf("expected merge commit on main, got: %s", log)
	}
}

func TestRun_FailPath_NoMerge(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	// K=1 resolve-in-place: with 1 model and 2 FAILs, the triage
	// exhausts (first FAIL → resolve_in_place, second FAIL → halt
	// because no more models to escalate to).
	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Fail, Rationale: "missing test"},
			{Verdict: verdict.Fail, Rationale: "still missing"},
		},
	}

	err := Run(context.Background(), Options{
		Task:             "Write a file",
		VerifierModel:    "fake/verifier",
		Base:             "main",
		RetryCap:         1,
		WorkspaceRoot:    workspaceRoot,
		EscalationModels: []string{"fake/impl1"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("should not merge"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "verification failed after") {
		t.Fatalf("expected 'verification failed after', got: %v", err)
	}
	if !strings.Contains(err.Error(), "Escalate to human") {
		t.Fatalf("expected escalation message, got: %v", err)
	}

	// Verify no merge on main.
	runCmd(t, workspaceRoot, "git", "checkout", "main")
	log := runCmd(t, workspaceRoot, "git", "log", "--oneline", "-1")
	if strings.Contains(log, "merge:") {
		t.Fatal("unexpected merge commit on main after FAIL")
	}
}

func TestRun_FailThenPass_RetrySucceeds(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Fail, Rationale: "first try fail"},
			{Verdict: verdict.Pass, Rationale: "second try ok"},
		},
	}

	err := Run(context.Background(), Options{
		Task:             "Write retry file",
		VerifierModel:    "fake/verifier",
		Base:             "main",
		RetryCap:         1,
		WorkspaceRoot:    workspaceRoot,
		EscalationModels: []string{"fake/impl1", "fake/impl2"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("retry success"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Should have merged.
	runCmd(t, workspaceRoot, "git", "checkout", "main")
	log := runCmd(t, workspaceRoot, "git", "log", "--oneline", "-1")
	if !strings.Contains(log, "merge:") {
		t.Fatalf("expected merge commit on main after retry PASS, got: %s", log)
	}
}

func TestRun_Blocked_StopsImmediately(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Blocked, Rationale: "spec missing required section"},
		},
	}

	err := Run(context.Background(), Options{
		Task:             "Blocked task",
		VerifierModel:    "fake/verifier",
		Base:             "main",
		RetryCap:         3,
		WorkspaceRoot:    workspaceRoot,
		EscalationModels: []string{"fake/impl1", "fake/impl2", "fake/impl3", "fake/impl4"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("blocked test"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err == nil {
		t.Fatal("expected error for BLOCKED, got nil")
	}
	if !strings.Contains(err.Error(), "verification blocked") {
		t.Fatalf("expected 'verification blocked', got: %v", err)
	}
}

func TestSanitiseBranch(t *testing.T) {
	tests := []struct {
		task, want string
	}{
		{"hello world", "sworn/hello-world"},
		{"Write a Go test", "sworn/write-a-go-test"},
		{"UPPERCASE Task", "sworn/uppercase-task"},
		{"special!@#chars", "sworn/specialchars"},
		{"a-very-long-task-name-that-exceeds-fifty-characters-and-gets-cut", "sworn/a-very-long-task-name-that-exceeds-fifty-cha"}, {"", "sworn/task"},
	}
	for _, tt := range tests {
		got := sanitiseBranch(tt.task)
		if got != tt.want {
			t.Errorf("sanitiseBranch(%q) = %q, want %q", tt.task, got, tt.want)
		}
	}
}

func TestRun_MissingTask(t *testing.T) {
	err := Run(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected error for missing task")
	}
	if !strings.Contains(err.Error(), "task is required") {
		t.Fatalf("expected task required, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// S03 — verify reachability through the run loop
// ---------------------------------------------------------------------------

// TestRun_VerifyMarkdownPass proves that a markdown-emphasised PASS reply
// (e.g. **PASS**) still resolves through the run loop's verify gate and
// merges.  This is AC1 — the parser from S02 is wired on the run path.
func TestRun_VerifyMarkdownPass(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	verifier := &textVerifier{reply: "**PASS** — verification successful"}

	err := Run(context.Background(), Options{
		Task:             "Write a markdown pass file",
		VerifierModel:    "fake/verifier",
		Base:             "main",
		RetryCap:         0,
		WorkspaceRoot:    workspaceRoot,
		EscalationModels: []string{"fake/impl"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("markdown pass test"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify status.json state is verified.
	entries, _ := filepath.Glob(filepath.Join(workspaceRoot, "docs", "release", "run-*", "S01-task", "status.json"))
	if len(entries) == 0 {
		t.Fatal("status.json not found after run")
	}
	st, err := state.Read(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if st.State != state.Verified {
		t.Fatalf("expected state verified, got %q", st.State)
	}

	// Verify merge commit exists on main.
	runCmd(t, workspaceRoot, "git", "checkout", "main")
	log := runCmd(t, workspaceRoot, "git", "log", "--oneline", "-1")
	if !strings.Contains(log, "merge:") {
		t.Fatalf("expected merge commit on main, got: %s", log)
	}
}

// TestRun_VerifyStatelessPromptWired was REMOVED 2026-06-28. It asserted the
// verify path wired the *stateless* judge prompt ("no tools / SPEC+DIFF only")
// and explicitly forbade agentic markers ("worktree", "Baton verifier"). The
// agentic-verifier migration (S11 dispatch + S12 demote) deliberately reversed
// this: verification now dispatches verifier.md through an agent, and the
// stateless judge is demoted to the deterministic RunFirstPass gate. The test
// asserted behaviour the release intentionally removed, so it was deleted rather
// than inverted. Agentic verifier wiring is covered by the verify package tests
// and the passingVerifierAgent / verifierAwareAgent paths in this package.

// TestRun_VerifyToolCallLeakBlocks proves that a garbage verifier reply (e.g. a
// leaked <tool_call ...>) leaves the run loop NOT merged — fail-closed end-to-end
// (AC3). Under the ADR-0011 structured authoring path the verifier emits a
// schema-constrained verdict, so a reply that is not a valid verifier-verdict-v1
// object resolves to INCONCLUSIVE (fail-closed) rather than the old prose-parsed
// BLOCKED. The load-bearing invariant is unchanged: a non-determinate verdict
// never merges.
func TestRun_VerifyToolCallLeakBlocks(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	verifier := &textVerifier{reply: `<tool_call name="Bash">
{"command": "cat /etc/passwd"}
</tool_call>`}

	err := Run(context.Background(), Options{
		Task:             "Tool call leak task",
		VerifierModel:    "fake/verifier",
		Base:             "main",
		RetryCap:         0,
		WorkspaceRoot:    workspaceRoot,
		EscalationModels: []string{"fake/impl"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("tool call leak test"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err == nil {
		t.Fatal("expected error for garbage verifier reply, got nil")
	}

	// Verify no merge on main — the fail-closed invariant.
	runCmd(t, workspaceRoot, "git", "checkout", "main")
	log := runCmd(t, workspaceRoot, "git", "log", "--oneline", "-1")
	if strings.Contains(log, "merge:") {
		t.Fatal("unexpected merge commit on main after fail-closed verdict")
	}
}

// ---------------------------------------------------------------------------
// RunSlice tests (S02a)
// ---------------------------------------------------------------------------

// setupFixtureSlice creates a temp git repo with an initial commit, then
// writes a fixture spec.md and status.json in a slice directory. It returns
// the worktree root, spec path, status path, and a cleanup function.
func setupFixtureSlice(t *testing.T) (worktreeRoot, specPath, statusPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()

	runCmd(t, dir, "git", "init", "-b", "main")
	runCmd(t, dir, "git", "config", "user.email", "test@swornagent.dev")
	runCmd(t, dir, "git", "config", "user.name", "sworn test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "README.md")
	runCmd(t, dir, "git", "commit", "-m", "initial commit")

	// Create slice directory.
	sliceDir := filepath.Join(dir, "docs", "release", "test-release", "S01-task")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	specPath = filepath.Join(sliceDir, "spec.md")
	statusPath = filepath.Join(sliceDir, "status.json")

	// Write a minimal spec.
	if err := os.WriteFile(specPath, []byte("# Test slice\n\nWrite a hello file.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write status.json with start_commit set.
	startCommit := strings.TrimSpace(runCmd(t, dir, "git", "rev-parse", "HEAD"))

	st := &state.Status{
		Schema:        "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:       "S01-task",
		Release:       "test-release",
		Track:         "",
		State:         state.InProgress,
		Owner:         "test",
		LastUpdatedBy: "setup",
		LastUpdatedAt: time.Now().UTC().Format(time.RFC3339),
		StartCommit:   startCommit,
		SpecPath:      "docs/release/test-release/S01-task/spec.md",
		ProofPath:     "docs/release/test-release/S01-task/proof.md",
		JournalPath:   "docs/release/test-release/S01-task/journal.md",
		PlannedFiles:  []string{},
		TestCommands:  []string{"go test ./..."},
		Verification:  state.Verification{},
		ReleaseBase:   "main",
	}
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}

	// Stage and commit the fixture so start_commit captures the slice state.
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "fixture slice")

	// Update start_commit to point to this commit.
	startCommit2 := strings.TrimSpace(runCmd(t, dir, "git", "rev-parse", "HEAD"))
	st.StartCommit = startCommit2
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}
	_ = runCmd(t, dir, "git", "add", statusPath)
	_ = runCmd(t, dir, "git", "commit", "-m", "set start_commit")

	return dir, specPath, statusPath, func() {}
}

func TestRunSlice(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Pass, Rationale: "all good"},
		},
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		EscalationModels: []string{"fake/impl"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("hello from RunSlice"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}

	// Verify the implementation file was created.
	data, err := os.ReadFile(filepath.Join(worktreeRoot, "output.txt"))
	if err != nil {
		t.Fatalf("output.txt not created: %v", err)
	}
	if string(data) != "hello from RunSlice" {
		t.Fatalf("expected 'hello from RunSlice', got %q", string(data))
	}

	// Verify status.json is verified.
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != state.Verified {
		t.Fatalf("expected state verified, got %q", st.State)
	}
}

func TestRunSliceFail(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Fail, Rationale: "missing test"},
			{Verdict: verdict.Fail, Rationale: "still missing"},
		},
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		EscalationModels: []string{"fake/impl1"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("should not pass"),
			verify:    verdictsFrom(verifier),
		}),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsFailed(err) {
		t.Fatalf("expected IsFailed(err)=true, got false: %v", err)
	}

	// Verify status.json is failed_verification.
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != state.FailedVerification {
		t.Fatalf("expected state failed_verification, got %q", st.State)
	}
}

func TestRunSlice_MissingVerifierModel(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{})
	if err == nil {
		t.Fatal("expected error for missing VerifierModel, got nil")
	}
	if !strings.Contains(err.Error(), "VerifierModel is required") {
		t.Fatalf("expected VerifierModel required, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// S07-paging — FAIL/BLOCKED notifier integration (Required tests → Integration)
// ---------------------------------------------------------------------------

// fakeNotifier is a recording Notifier seam fake. It captures every Notify
// call so the integration test can assert the run loop fires the webhook on a
// FAIL or BLOCKED verdict transition with the correct payload. It implements
// the run.Notifier interface (one method).
type fakeNotifier struct {
	mu      sync.Mutex
	calls   []account.NotifyEvent
	webhook bool // mirrors account.Notifier's "has webhook" behaviour
}

func (f *fakeNotifier) Notify(_ context.Context, event account.NotifyEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, event)
}

func (f *fakeNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeNotifier) lastCall() (account.NotifyEvent, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return account.NotifyEvent{}, false
	}
	return f.calls[len(f.calls)-1], true
}

// TestRunSlice_FailNotifiesOnce is the S07-paging Required Integration test
// (spec "Required tests → Integration": inject a failing mock verifier; assert
// notifier.Notify is called exactly once with the correct slice ID). It
// exercises the FAIL→failed_verification wiring in slice.go (the path the
// verifier cited at slice.go:264-275) through the integration point that owns
// the affordance — RunSlice — using the run.Notifier interface seam.
func TestRunSlice_FailNotifiesOnce(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	// Failing verifier — FAILs every attempt so the retry loop exhausts and
	// transitions to failed_verification, firing the FAIL notifier.
	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Fail, Rationale: "missing test"},
			{Verdict: verdict.Fail, Rationale: "still missing"},
		},
	}

	notifier := &fakeNotifier{webhook: true}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		EscalationModels: []string{"fake/impl1"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("should not pass"),
			verify:    verdictsFrom(verifier),
		}),
		Notifier: notifier,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsFailed(err) {
		t.Fatalf("expected IsFailed(err)=true, got false: %v", err)
	}

	// AC1: Notify called exactly once with State == "failed_verification" and
	// the correct slice ID (and Release/Track from status.json).
	if got := notifier.count(); got != 1 {
		t.Fatalf("Notify called %d times, want exactly 1", got)
	}

	ev, ok := notifier.lastCall()
	if !ok {
		t.Fatal("no Notify event recorded")
	}
	if ev.SliceID != "S01-task" {
		t.Errorf("SliceID = %q, want %q", ev.SliceID, "S01-task")
	}
	if ev.Release != "test-release" {
		t.Errorf("Release = %q, want %q", ev.Release, "test-release")
	}
	if ev.State != "failed_verification" {
		t.Errorf("State = %q, want %q", ev.State, "failed_verification")
	}
	if ev.WorktreePath != worktreeRoot {
		t.Errorf("WorktreePath = %q, want %q", ev.WorktreePath, worktreeRoot)
	}
	if ev.ViolationsSummary == "" {
		t.Error("ViolationsSummary must not be empty on FAIL")
	}

	// The state must have actually transitioned (the notify fires AFTER the
	// state write, so this also guards ordering).
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != state.FailedVerification {
		t.Fatalf("status state = %q, want failed_verification", st.State)
	}
}

// TestRunSlice_BlockedNotifies exercises the BLOCKED wiring (slice.go:222-239)
// that the verifier cited as the second transition. It asserts Notify is
// called exactly once with State == "blocked" and the correct slice ID.
func TestRunSlice_BlockedNotifies(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	// BLOCKED verifier — the run loop returns immediately on the first attempt.
	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Blocked, Rationale: "spec missing required section"},
		},
	}

	notifier := &fakeNotifier{webhook: true}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		EscalationModels: []string{"fake/impl"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("blocked test"),
			verify:    verdictsFrom(verifier),
		}),
		Notifier: notifier,
	})
	if err == nil {
		t.Fatal("expected error for BLOCKED, got nil")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected IsBlocked(err)=true, got false: %v", err)
	}

	// Notify called exactly once with State == "blocked".
	if got := notifier.count(); got != 1 {
		t.Fatalf("Notify called %d times, want exactly 1", got)
	}

	ev, ok := notifier.lastCall()
	if !ok {
		t.Fatal("no Notify event recorded")
	}
	if ev.SliceID != "S01-task" {
		t.Errorf("SliceID = %q, want %q", ev.SliceID, "S01-task")
	}
	if ev.State != "blocked" {
		t.Errorf("State = %q, want %q", ev.State, "blocked")
	}
	if ev.ViolationsSummary != "BLOCKED: spec missing required section" {
		t.Errorf("ViolationsSummary = %q, want %q", ev.ViolationsSummary, "BLOCKED: spec missing required section")
	}
}

// TestRunSlice_NilNotifierNoOp confirms the nil-notifier path does not panic
// (production callers may pass nil when no webhook/account is configured).
func TestRunSlice_NilNotifierNoOp(t *testing.T) {
	worktreeRoot, specPath, statusPath, _ := setupFixtureSlice(t)

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{{Verdict: verdict.Pass, Rationale: "ok"}},
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		EscalationModels: []string{"fake/impl"},
		Registry: testRegistry(&fakeDriver{
			implement: writeFileImplementer("nil notifier test"),
			verify:    verdictsFrom(verifier),
		}),
		Notifier: nil, // no notifier — must not panic
	})
	if err != nil {
		t.Fatalf("RunSlice error: %v", err)
	}
}
