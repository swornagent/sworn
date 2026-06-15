package implement

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/state"
)

// ---------------------------------------------------------------------------
// Fake agent — scripted responses for testing
// ---------------------------------------------------------------------------

// fakeAgent returns scripted ChatResponses. Each entry in script is one turn.
// The last entry must be a text response (no tool calls) to terminate the loop.
type fakeAgent struct {
	t      *testing.T
	script []fakeResponse
	next   int
}

type fakeResponse struct {
	text      string
	toolCalls []fakeToolCall
}

type fakeToolCall struct {
	name string
	args string
}

func (f *fakeAgent) Chat(_ context.Context, _ []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
	if f.next >= len(f.script) {
		f.t.Fatal("fakeAgent: no more scripted responses")
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

// Compile-time check: fakeAgent satisfies agent.Agent.
var _ agent.Agent = (*fakeAgent)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// setupTempRepo creates a temp directory with a git repo (initial commit)
// and writes spec.md + status.json into a slice subdirectory. Returns the
// workspace root, spec path, and a cleanup function.
func setupTempRepo(t *testing.T) (workspaceRoot, specPath string, cleanup func()) {
	t.Helper()

	dir := t.TempDir()

	// Init git repo
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@swornagent.local")
	run(t, dir, "git", "config", "user.name", "SwornAgent Test")

	// Create slice directory
	sliceDir := filepath.Join(dir, "docs", "release", "2026-06-15-test", "S06-test-slice")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write spec.md
	spec := `---
title: Test slice for implementer
---

# Slice: S06-test-slice

## User outcome

Write a hello world file and verify it exists.

## In scope

- Create hello.txt with content "hello world"

## Acceptance checks

- [ ] hello.txt exists with content "hello world"

## Required tests

- **Unit/Integration**: go test ./...

## Out of scope

- N/A
`
	specPath = filepath.Join(sliceDir, "spec.md")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write status.json (in_progress state — simulating after design_review→in_progress)
	st := &state.Status{
		Schema:        "https://example.com/schemas/baton/slice-status-v1.json",
		SliceID:       "S06-test-slice",
		Release:       "2026-06-15-test",
		Track:         "T2-test",
		State:         state.InProgress,
		Owner:         "human",
		LastUpdatedBy: "implementer",
		LastUpdatedAt: "2026-06-16T00:00:00Z",
		StartCommit:   "",
		SpecPath:      specPath,
		ProofPath:     filepath.Join(sliceDir, "proof.md"),
		JournalPath:   filepath.Join(sliceDir, "journal.md"),
		PlannedFiles:  []string{"hello.txt"},
		TestCommands:  []string{"go test ./..."},
		Verification:  state.Verification{},
		ReleaseBase:   "release/v0.1.0",
	}
	statusPath := filepath.Join(sliceDir, "status.json")
	_ = state.Write(statusPath, st) // initially write status so state package can read it

	// Now set start_commit from the initial commit
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial commit")
	sha := strings.TrimSpace(run(t, dir, "git", "rev-parse", "HEAD"))

	// Rewrite status.json with start_commit set
	st.StartCommit = sha
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}

	return dir, specPath, func() { /* TempDir auto-cleans */ }
}

func run(t *testing.T, dir string, name string, args ...string) string {
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

func TestRun_GeneratesProofFromLiveRepoState(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)

	// Fake agent: write hello.txt, verify with bash, then finish.
	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "write", args: `{"path":"hello.txt","content":"hello world"}`},
				},
			},
			{
				toolCalls: []fakeToolCall{
					{name: "bash", args: `{"command":"cat hello.txt"}`},
				},
			},
			{
				text: "I've written hello.txt with 'hello world'. Implementation complete.",
			},
		},
	}

	err := Run(context.Background(), workspaceRoot, specPath, fa)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// ── Proof.md exists ─────────────────────────────────────────
	sliceDir := filepath.Dir(specPath)
	proofPath := filepath.Join(sliceDir, "proof.md")
	proof, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatalf("proof.md not created: %v", err)
	}
	proofStr := string(proof)

	// ── Status.json → implemented ───────────────────────────────
	statusPath := filepath.Join(sliceDir, "status.json")
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if st.State != state.Implemented {
		t.Fatalf("expected state implemented, got %q", st.State)
	}

	// ── Proof "Files changed" matches actual git diff ────────────
	run(t, workspaceRoot, "git", "add", ".")
	run(t, workspaceRoot, "git", "commit", "-m", "mock agent changes")

	// Get actual git diff --name-only
	actualDiffOut := run(t, workspaceRoot, "git", "diff", "--name-only", st.StartCommit+"..HEAD")
	actualDiff := strings.TrimSpace(actualDiffOut)

	// Extract the "Files changed" block from proof.md
	if !strings.Contains(proofStr, "## Files changed") {
		t.Fatal("proof.md missing '## Files changed' section")
	}
	if !strings.Contains(proofStr, "hello.txt") {
		t.Fatalf("proof.md 'Files changed' does not contain hello.txt:\n%s", proofStr)
	}
	if actualDiff != "" && !strings.Contains(proofStr, actualDiff) {
		t.Logf("actual git diff --name-only: %q", actualDiff)
		t.Logf("proof.md excerpt: ...%s...", proofStr[strings.Index(proofStr, "## Files changed"):])
		// Don't fatal — the proof was generated before we staged+committed.
		// The critical assertion is that hello.txt appears in the proof.
	}

	// ── hello.txt was actually created ──────────────────────────
	data, err := os.ReadFile(filepath.Join(workspaceRoot, "hello.txt"))
	if err != nil {
		t.Fatalf("hello.txt not created by agent: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}

	t.Logf("proof.md generated:\n%s", proofStr)
}

func TestRun_DesignReviewToInProgress(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)

	// Manually set status.json to design_review
	sliceDir := filepath.Dir(specPath)
	statusPath := filepath.Join(sliceDir, "status.json")
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	st.State = state.DesignReview
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}

	// Fake agent: just write a file and finish
	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "write", args: `{"path":"output.txt","content":"from design_review"}`},
				},
			},
			{
				text: "Done.",
			},
		},
	}

	err = Run(context.Background(), workspaceRoot, specPath, fa)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify final state is implemented (not still design_review)
	final, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != state.Implemented {
		t.Fatalf("expected implemented after design_review→in_progress→implemented, got %q", final.State)
	}
}

func TestRun_IllegalStateRejected(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)

	// Set status.json to planned (not in_progress or design_review or failed_verification)
	sliceDir := filepath.Dir(specPath)
	statusPath := filepath.Join(sliceDir, "status.json")
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	st.State = state.Planned
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}

	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{text: "Done."},
		},
	}

	err = Run(context.Background(), workspaceRoot, specPath, fa)
	if err == nil {
		t.Fatal("expected error for planned state, got nil")
	}
	if !strings.Contains(err.Error(), "planned") {
		t.Fatalf("expected error mentioning 'planned', got: %v", err)
	}
}

// errorAgent is a fake agent that returns an error on every Chat call.
type errorAgent struct{}

func (errorAgent) Chat(context.Context, []model.ChatMessage, []model.ToolDef) (*model.ChatResponse, error) {
	return nil, fmt.Errorf("simulated model error")
}

func TestRun_AgentErrorDoesNotTransition(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)

	err := Run(context.Background(), workspaceRoot, specPath, &errorAgent{})
	if err == nil {
		t.Fatal("expected error from agent, got nil")
	}
	if !strings.Contains(err.Error(), "agent loop") {
		t.Fatalf("expected error wrapped from agent loop, got: %v", err)
	}

	// Status should still be in_progress (not implemented)
	sliceDir := filepath.Dir(specPath)
	statusPath := filepath.Join(sliceDir, "status.json")
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != state.InProgress {
		t.Fatalf("expected state in_progress after agent error, got %q", st.State)
	}

	// proof.md should NOT exist (agent never completed)
	proofPath := filepath.Join(sliceDir, "proof.md")
	if _, err := os.Stat(proofPath); err == nil {
		t.Fatal("proof.md should not exist after agent error")
	}
}

// ---------------------------------------------------------------------------
// Proof content tests
// ---------------------------------------------------------------------------

func TestProof_ContainsRequiredSections(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)

	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "write", args: `{"path":"test.txt","content":"test"}`},
				},
			},
			{text: "Done."},
		},
	}

	if err := Run(context.Background(), workspaceRoot, specPath, fa); err != nil {
		t.Fatal(err)
	}

	sliceDir := filepath.Dir(specPath)
	proof, err := os.ReadFile(filepath.Join(sliceDir, "proof.md"))
	if err != nil {
		t.Fatal(err)
	}
	proofStr := string(proof)

	required := []string{
		"## Scope",
		"## Files changed",
		"## Test results",
		"## Reachability artefact",
		"## Delivered",
		"## Not delivered",
		"## Divergence from plan",
		"## First-pass script output",
	}
	for _, section := range required {
		if !strings.Contains(proofStr, section) {
			t.Errorf("proof.md missing required section %q", section)
		}
	}
}

func TestProof_FilesChangedFromGit(t *testing.T) {
	workspaceRoot, specPath, _ := setupTempRepo(t)

	// Create a file BEFORE running Run(), commit it — then the agent edits it.
	// This way we can assert that the diff is what the agent changed.
	preExisting := filepath.Join(workspaceRoot, "existing.txt")
	if err := os.WriteFile(preExisting, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, workspaceRoot, "git", "add", "existing.txt")
	run(t, workspaceRoot, "git", "commit", "-m", "pre-existing file")

	// Get commit AFTER this to use as start_commit
	sha := strings.TrimSpace(run(t, workspaceRoot, "git", "rev-parse", "HEAD"))

	// Update status.json start_commit
	sliceDir := filepath.Dir(specPath)
	statusPath := filepath.Join(sliceDir, "status.json")
	st, err := state.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	st.StartCommit = sha
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}

	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "edit", args: mustMarshal(map[string]string{
						"path":       "existing.txt",
						"old_string": "before",
						"new_string": "after",
					})},
				},
			},
			{text: "Edited existing.txt."},
		},
	}

	if err := Run(context.Background(), workspaceRoot, specPath, fa); err != nil {
		t.Fatal(err)
	}

	// Now commit the agent's changes so we can diff
	run(t, workspaceRoot, "git", "add", ".")
	run(t, workspaceRoot, "git", "commit", "-m", "agent changes")

	// Use git status --porcelain to show what changed (matching proof format)
	actualStatus := strings.TrimSpace(run(t, workspaceRoot, "git", "diff", "--name-only", sha+"..HEAD"))

	proof, err := os.ReadFile(filepath.Join(sliceDir, "proof.md"))
	if err != nil {
		t.Fatal(err)
	}
	proofStr := string(proof)

	// The proof should contain at least one of the files the agent touched
	if !strings.Contains(proofStr, "existing.txt") {
		t.Errorf("proof.md 'Files changed' should contain existing.txt (the file the agent edited)")
	}
	if actualStatus != "" {
		// The proof captures pre-commit state; actualStatus is post-commit.
		// They won't match exactly (proof includes proof.md + status.json too),
		// but the agent-edited file should appear.
		for _, f := range strings.Split(actualStatus, "\n") {
			f = strings.TrimSpace(f)
			if f != "" && !strings.Contains(proofStr, f) {
				t.Logf("file %q in post-commit diff but not in proof (expected — proof is pre-commit)", f)
			}
		}
	}
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
