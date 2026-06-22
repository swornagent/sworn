package run

import (
	"context"
	"fmt"
	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fake agent — scripted implementer
// ---------------------------------------------------------------------------

type fakeImplementer struct {
	t      *testing.T
	script []fakeAgentResponse
	next   int
}

type fakeAgentResponse struct {
	text      string
	toolCalls []fakeToolCall
}

type fakeToolCall struct {
	name string
	args string
}

func (f *fakeImplementer) Chat(_ context.Context, _ []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
	if f.next >= len(f.script) {
		// Return a simple done message if script exhausted.
		return &model.ChatResponse{
			Choices: []struct {
				Message struct {
					Content   string           `json:"content"`
					ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{{Message: struct {
				Content   string           `json:"content"`
				ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
			}{Content: "Done."}}},
		}, nil
	}
	r := f.script[f.next]
	f.next++

	cr := &model.ChatResponse{
		Choices: []struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{{}},
	}
	cr.Choices[0].Message.Content = r.text

	for i, tc := range r.toolCalls {
		cr.Choices[0].Message.ToolCalls = append(cr.Choices[0].Message.ToolCalls, model.ToolCall{
			ID:   fmt.Sprintf("call_%d_%d", f.next, i),
			Type: "function",
			Function: model.FunctionCall{
				Name:      tc.name,
				Arguments: tc.args,
			},
		})
	}
	if len(r.toolCalls) > 0 {
		cr.Choices[0].FinishReason = "tool_calls"
	} else {
		cr.Choices[0].FinishReason = "stop"
	}
	return cr, nil
}

var _ agent.Agent = (*fakeImplementer)(nil)

// ---------------------------------------------------------------------------
// Fake verifier — returns scripted verdicts
// ---------------------------------------------------------------------------

type fakeVerifier struct {
	verdicts []verdict.Result
	next     int
}

func (f *fakeVerifier) Verify(_ context.Context, _, _ string) (string, float64, error) {
	if f.next >= len(f.verdicts) {
		return "PASS", 0, nil
	}
	v := f.verdicts[f.next]
	f.next++
	return string(v.Verdict) + ": " + v.Rationale, v.CostUSD, nil
}

var _ model.Verifier = (*fakeVerifier)(nil)

// ---------------------------------------------------------------------------
// textVerifier — returns a fixed raw reply, optionally capturing the system prompt
// ---------------------------------------------------------------------------

// textVerifier returns a fixed reply text. When capture is non-nil, it records
// the system prompt it receives from verify.Run. Used for S03 reachability tests
// that must inspect what prompt the run loop wired.
type textVerifier struct {
	reply   string
	capture *string
}

func (v *textVerifier) Verify(_ context.Context, systemPrompt, _ string) (string, float64, error) {
	if v.capture != nil {
		*v.capture = systemPrompt
	}
	return v.reply, 0, nil
}

var _ model.Verifier = (*textVerifier)(nil)

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

// stdoutAgent creates a fake implementer that writes a file then exits.
func stdoutAgent(content string) *fakeImplementer {
	return &fakeImplementer{
		script: []fakeAgentResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "write", args: fmt.Sprintf(`{"path":"output.txt","content":%q}`, content)},
				},
			},
			{text: "Implementation complete."},
		},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRun_PassPath_Merges(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	impl := stdoutAgent("hello from sworn run")

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Pass, Rationale: "all good"},
		},
	}

	err := Run(context.Background(), Options{
		Task:          "Write a hello file",
		VerifierModel: "fake/verifier",
		Base:          "main",
		RetryCap:      0,
		WorkspaceRoot: workspaceRoot,
		NewAgent:      func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:   func(_ string) (model.Verifier, error) { return verifier, nil },
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

	impl := stdoutAgent("should not merge")

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Fail, Rationale: "missing test"},
			{Verdict: verdict.Fail, Rationale: "still missing"},
			{Verdict: verdict.Fail, Rationale: "nope"},
		},
	}

	err := Run(context.Background(), Options{
		Task:             "Write a file",
		VerifierModel:    "fake/verifier",
		Base:             "main",
		RetryCap:         2,
		WorkspaceRoot:    workspaceRoot,
		EscalationModels: []string{"fake/impl1", "fake/impl2", "fake/impl3"},
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
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

	impl := stdoutAgent("retry success")

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
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
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

	impl := stdoutAgent("blocked test")

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
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
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

	impl := stdoutAgent("markdown pass test")

	verifier := &textVerifier{reply: "**PASS** — verification successful"}

	err := Run(context.Background(), Options{
		Task:          "Write a markdown pass file",
		VerifierModel: "fake/verifier",
		Base:          "main",
		RetryCap:      0,
		WorkspaceRoot: workspaceRoot,
		NewAgent:      func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:   func(_ string) (model.Verifier, error) { return verifier, nil },
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

// TestRun_VerifyStatelessPromptWired proves the stateless judge prompt
// (S01's VerifyStateless) is wired on the run path — the verifier
// receives "no tools / SPEC+DIFF only / verdict-leading", not the
// agentic verifier.md role prompt.  This is AC2.
func TestRun_VerifyStatelessPromptWired(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	impl := stdoutAgent("stateless prompt test")

	var capturedPrompt string
	verifier := &textVerifier{reply: "PASS — looks good", capture: &capturedPrompt}

	err := Run(context.Background(), Options{
		Task:          "Stateless prompt check",
		VerifierModel: "fake/verifier",
		Base:          "main",
		RetryCap:      0,
		WorkspaceRoot: workspaceRoot,
		NewAgent:      func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:   func(_ string) (model.Verifier, error) { return verifier, nil },
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Must contain stateless markers.
	for _, want := range []string{"no tools", "SPEC+DIFF only", "verdict-leading"} {
		if !strings.Contains(capturedPrompt, want) {
			t.Errorf("system prompt missing stateless marker %q", want)
		}
	}
	// Must NOT contain agentic verifier instructions from verifier.md.
	for _, forbidden := range []string{"worktree", "git -C", "fresh terminal", "Baton verifier"} {
		if strings.Contains(capturedPrompt, forbidden) {
			t.Errorf("system prompt contains agentic token %q — should use stateless prompt, not verifier.md", forbidden)
		}
	}
}

// TestRun_VerifyToolCallLeakBlocks proves that a tool-call-leak reply
// from the verifier (e.g. <tool_call name="...">) leaves the run loop
// NOT merged — the parser from S02 maps it to BLOCKED/unparseable_verdict
// and the run loop stops without merging.  This is AC3 — fail-closed
// end-to-end.
func TestRun_VerifyToolCallLeakBlocks(t *testing.T) {
	workspaceRoot, _ := setupTestRepo(t)

	impl := stdoutAgent("tool call leak test")

	verifier := &textVerifier{reply: `<tool_call name="Bash">
{"command": "cat /etc/passwd"}
</tool_call>`}

	err := Run(context.Background(), Options{
		Task:          "Tool call leak task",
		VerifierModel: "fake/verifier",
		Base:          "main",
		RetryCap:      0,
		WorkspaceRoot: workspaceRoot,
		NewAgent:      func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:   func(_ string) (model.Verifier, error) { return verifier, nil },
	})
	if err == nil {
		t.Fatal("expected error for tool-call leak, got nil")
	}
	if !strings.Contains(err.Error(), "verification blocked") {
		t.Fatalf("expected 'verification blocked', got: %v", err)
	}

	// Verify no merge on main.
	runCmd(t, workspaceRoot, "git", "checkout", "main")
	log := runCmd(t, workspaceRoot, "git", "log", "--oneline", "-1")
	if strings.Contains(log, "merge:") {
		t.Fatal("unexpected merge commit on main after tool-call BLOCKED")
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

	impl := stdoutAgent("hello from RunSlice")

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Pass, Rationale: "all good"},
		},
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		EscalationModels: []string{"fake/impl"},
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
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

	impl := stdoutAgent("should not pass")

	verifier := &fakeVerifier{
		verdicts: []verdict.Result{
			{Verdict: verdict.Fail, Rationale: "missing test"},
			{Verdict: verdict.Fail, Rationale: "still missing"},
		},
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		EscalationModels: []string{"fake/impl1", "fake/impl2"},
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
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

	impl := stdoutAgent("should not pass")

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
		EscalationModels: []string{"fake/impl1", "fake/impl2"},
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
		Notifier:         notifier,
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

	impl := stdoutAgent("blocked test")

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
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
		Notifier:         notifier,
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

	impl := stdoutAgent("nil notifier test")
	verifier := &fakeVerifier{
		verdicts: []verdict.Result{{Verdict: verdict.Pass, Rationale: "ok"}},
	}

	err := RunSlice(context.Background(), worktreeRoot, specPath, statusPath, RunSliceOptions{
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		EscalationModels: []string{"fake/impl"},
		NewAgent:         func(_ string) (agent.Agent, error) { return impl, nil },
		NewVerifier:      func(_ string) (model.Verifier, error) { return verifier, nil },
		Notifier:         nil, // no notifier — must not panic
	})
	if err != nil {
		t.Fatalf("RunSlice error: %v", err)
	}
}
