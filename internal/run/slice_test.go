package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/implement"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/verdict"
)

// ---------------------------------------------------------------------------
// blockingFakeAgent — blocks on ctx.Done() to simulate a hung model
// ---------------------------------------------------------------------------

type blockingFakeAgent struct{}

func (b *blockingFakeAgent) Chat(ctx context.Context, _ []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

var _ agent.Agent = (*blockingFakeAgent)(nil)

// ---------------------------------------------------------------------------
// quickFakeAgent — returns a simple text response immediately
// ---------------------------------------------------------------------------

type quickFakeAgent struct{}

func (q *quickFakeAgent) Chat(_ context.Context, _ []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
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
		}{Content: "Done."}, FinishReason: "stop"}},
	}, nil
}

var _ agent.Agent = (*quickFakeAgent)(nil)

// ---------------------------------------------------------------------------
// alwaysPassVerifier — returns PASS for every verify call
// ---------------------------------------------------------------------------

type alwaysPassVerifier struct{}

func (v *alwaysPassVerifier) Verify(_ context.Context, _, _ string) (string, float64, error) {
	return string(verdict.Pass), 0, nil
}

var _ model.Verifier = (*alwaysPassVerifier)(nil)

// ---------------------------------------------------------------------------
// markedAgent — records that it was called via a pointer
// ---------------------------------------------------------------------------

type markedAgent struct {
	called *bool
}

func (m *markedAgent) Chat(_ context.Context, _ []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
	*m.called = true
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
		}{Content: "Done."}, FinishReason: "stop"}},
	}, nil
}

var _ agent.Agent = (*markedAgent)(nil)

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

	slot1Called := false
	slot2Called := false

	opts := RunSliceOptions{
		EscalationModels: []string{"blocking", "working"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: 500 * time.Millisecond,
		NewAgent: func(modelID string) (agent.Agent, error) {
			switch modelID {
			case "blocking":
				return &blockingFakeAgent{}, nil
			case "working":
				return &markedAgent{called: &slot2Called}, nil
			default:
				return nil, fmt.Errorf("unknown model: %s", modelID)
			}
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return &alwaysPassVerifier{}, nil },
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}

	// Slot 1 (blocking) blocks on ctx.Done(). After 500ms, the context deadline
	// fires, Chat returns context.DeadlineExceeded, implement.Run returns an error,
	// and RunSlice detects the timeout and escalates to slot 2.
	// slot2Called should be true — the escalation succeeded.
	_ = slot1Called // not used for blockingFakeAgent (no pointer)
	if !slot2Called {
		t.Error("expected slot 2 agent to be called after escalation from timeout")
	}
}
func TestImplementTimeoutExhaustsToHuman(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	opts := RunSliceOptions{
		EscalationModels: []string{"blocking1", "blocking2"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: 100 * time.Millisecond,
		NewAgent: func(_ string) (agent.Agent, error) {
			return &blockingFakeAgent{}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return &alwaysPassVerifier{}, nil },
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err == nil {
		t.Fatal("expected error after exhausting timeouts, got nil")
	}
	if !strings.Contains(err.Error(), "implementer failed after") {
		t.Fatalf("expected 'implementer failed after' message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Escalate to human") {
		t.Fatalf("expected 'Escalate to human' message, got: %v", err)
	}
}
func TestImplementTimeoutHappyPath(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	called := false

	opts := RunSliceOptions{
		EscalationModels: []string{"quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: DefaultImplementTimeout, // generous timeout
		NewAgent: func(_ string) (agent.Agent, error) {
			return &markedAgent{called: &called}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return &alwaysPassVerifier{}, nil },
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
		EscalationModels: []string{"quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: 0, // zero → use default (15m), not instant timeout
		NewAgent: func(_ string) (agent.Agent, error) {
			return &markedAgent{called: &called}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return &alwaysPassVerifier{}, nil },
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
		EscalationModels: []string{"quick"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1, // negative → no timeout, unbounded
		NewAgent: func(_ string) (agent.Agent, error) {
			return &markedAgent{called: &called}, nil
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return &alwaysPassVerifier{}, nil },
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

// recordingPromptAgent records the most recent user prompt it receives and
// always returns a stop response. It supports the optional "require feedback"
// behaviour: if requireFeedbackBlock is set, it returns a tool-call response
// (which never terminates) when the feedback block is absent, so the verifier
// can be driven to FAIL; when the block is present it returns "Done.".
type recordingPromptAgent struct {
	lastUserPrompt       string
	requireFeedbackBlock string
	seenFeedback         bool
}

func (r *recordingPromptAgent) Chat(_ context.Context, messages []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
	for _, m := range messages {
		if m.Role == "user" {
			r.lastUserPrompt = m.Content
			if strings.Contains(m.Content, "Previous attempt failed verification") {
				r.seenFeedback = true
			}
		}
	}
	resp := &model.ChatResponse{
		Choices: []struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{{}},
	}
	if r.requireFeedbackBlock != "" && !strings.Contains(r.lastUserPrompt, r.requireFeedbackBlock) {
		// Non-terminating tool-call: the verifier will judge the diff.
		// Use a unique call ID each turn so the agent loop keeps accepting it.
		callID := fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), len(messages))
		path := fmt.Sprintf("no-feedback-%d.txt", len(messages))
		resp.Choices[0].Message.ToolCalls = []model.ToolCall{{
			ID:   callID,
			Type: "function",
			Function: model.FunctionCall{
				Name:      "write",
				Arguments: fmt.Sprintf(`{"path":"%s","content":"no feedback"}`, path),
			},
		}}
		resp.Choices[0].FinishReason = "tool_calls"
	} else {
		resp.Choices[0].Message.Content = "Done."
		resp.Choices[0].FinishReason = "stop"
	}
	return resp, nil
}

// failThenPassVerifier returns FAIL on the first call and PASS on the second.
// The FAIL carries a fixed rationale so the retry path can pass it back.
type failThenPassVerifier struct {
	calls      int
	failReason string
}

func (v *failThenPassVerifier) Verify(_ context.Context, _, _ string) (string, float64, error) {
	v.calls++
	if v.calls == 1 {
		return v.failReason, 0, nil
	}
	return string(verdict.Pass), 0, nil
}

var _ model.Verifier = (*failThenPassVerifier)(nil)

// verdictReplyVerifier parses a recorded agent's text reply as a verdict.
type verdictReplyVerifier struct {
	expectedReply string
}

func (v *verdictReplyVerifier) Verify(_ context.Context, _, _ string) (string, float64, error) {
	return v.expectedReply, 0, nil
}

var _ model.Verifier = (*verdictReplyVerifier)(nil)

func TestRetryPassesVerifierRationale(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	failReason := "FAIL: gate 1 — no feedback block in implementer prompt"
	verifier := &failThenPassVerifier{failReason: failReason}

	var agent0, agent1 *recordingPromptAgent

	opts := RunSliceOptions{
		EscalationModels: []string{"model-a", "model-b"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: -1,
		NewAgent: func(modelID string) (agent.Agent, error) {
			switch modelID {
			case "model-a":
				agent0 = &recordingPromptAgent{requireFeedbackBlock: ""}
				return agent0, nil
			case "model-b":
				// Model B receives the feedback block on retry and stops naturally.
				// It records whether the block was present for the assertion below.
				agent1 = &recordingPromptAgent{}
				return agent1, nil
			default:
				return nil, fmt.Errorf("unknown model: %s", modelID)
			}
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return verifier, nil },
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}
	if agent0 == nil || agent1 == nil {
		t.Fatal("expected both agents to be created")
	}

	if strings.Contains(agent0.lastUserPrompt, "Previous attempt failed verification") {
		t.Fatalf("attempt 0 must not receive feedback, got:\n%s", agent0.lastUserPrompt)
	}
	if !strings.Contains(agent1.lastUserPrompt, failReason) {
		t.Fatalf("attempt 1 did not receive prior rationale; got:\n%s", agent1.lastUserPrompt)
	}
	if !strings.Contains(agent1.lastUserPrompt, "Previous attempt failed verification") {
		t.Fatalf("attempt 1 did not receive feedback header; got:\n%s", agent1.lastUserPrompt)
	}
}

func TestAttempt0EmptyFeedback(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	agent0 := &recordingPromptAgent{}

	opts := RunSliceOptions{
		EscalationModels: []string{"model-a"},
		VerifierModel:    "fake/verifier",
		RetryCap:         0,
		ImplementTimeout: -1,
		NewAgent: func(modelID string) (agent.Agent, error) {
			if modelID == "model-a" {
				return agent0, nil
			}
			return nil, fmt.Errorf("unknown model: %s", modelID)
		},
		NewVerifier: func(_ string) (model.Verifier, error) {
			return &verdictReplyVerifier{expectedReply: string(verdict.Pass)}, nil
		},
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}
	if strings.Contains(agent0.lastUserPrompt, "Previous attempt failed verification") {
		t.Fatalf("attempt 0 should not receive feedback block, got:\n%s", agent0.lastUserPrompt)
	}
	if !strings.HasPrefix(agent0.lastUserPrompt, "Implement the following spec") {
		t.Fatalf("attempt 0 prompt should start with original spec prefix, got:\n%s", agent0.lastUserPrompt)
	}
}

func TestRetryFeedbackResolvesToPass(t *testing.T) {
	workspaceRoot, specPath, statusPath, _ := setupSliceTestRepo(t)

	failReason := "FAIL: implementer prompt missing feedback block"
	verifier := &failThenPassVerifier{failReason: failReason}

	var agent0, agent1 *recordingPromptAgent

	opts := RunSliceOptions{
		EscalationModels: []string{"model-a", "model-b"},
		VerifierModel:    "fake/verifier",
		RetryCap:         1,
		ImplementTimeout: -1,
		NewAgent: func(modelID string) (agent.Agent, error) {
			switch modelID {
			case "model-a":
				agent0 = &recordingPromptAgent{}
				return agent0, nil
			case "model-b":
				// Model B receives the feedback block on retry and stops naturally.
				agent1 = &recordingPromptAgent{}
				return agent1, nil
			default:
				return nil, fmt.Errorf("unknown model: %s", modelID)
			}
		},
		NewVerifier: func(_ string) (model.Verifier, error) { return verifier, nil },
	}

	err := RunSlice(context.Background(), workspaceRoot, specPath, statusPath, opts)
	if err != nil {
		t.Fatalf("RunSlice() error: %v", err)
	}
	if agent0 == nil || agent1 == nil {
		t.Fatal("expected both agents to be created")
	}
	if agent0.seenFeedback {
		t.Fatal("model-a should not have seen feedback on attempt 0")
	}
	if !agent1.seenFeedback {
		t.Fatalf("model-b should have seen feedback on attempt 1; got prompt:\n%s", agent1.lastUserPrompt)
	}

	final, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != state.Verified {
		t.Fatalf("expected state verified after FAIL→PASS, got %q", final.State)
	}
}

// ensureCallCount is an unexported compile-time guard that implement.Run
// is callable with the new signature inside this package.
var _ = func() error {
	_ = implement.Run(context.Background(), "", "", "", nil)
	return nil
}()
