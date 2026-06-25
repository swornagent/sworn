package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
)

// fakeAgent is a test Agent that returns scripted responses. Each entry in
// script is one turn's response. The last entry should be a text response
// (no tool calls) to terminate the loop.
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

func TestRunReturnsOnEmptyStopAfterToolCalls(t *testing.T) {
	dir := t.TempDir()

	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "write", args: `{"path":"done.txt","content":"work complete"}`},
				},
			},
			{
				toolCalls: []fakeToolCall{
					{name: "bash", args: `{"command":"cat done.txt"}`},
				},
			},
			{
				// Terminal turn: empty content, no tool calls (natural stop).
			},
		},
	}

	text, _, msgs, err := Run(context.Background(), fa,
		"you are a helpful assistant", "write done.txt and verify it",
		dir, Config{MaxTurns: 10, MaxOutputBytes: 10000})
	if err != nil {
		t.Fatalf("expected nil error for empty natural stop, got: %v", err)
	}
	if text != "" {
		t.Fatalf("expected empty final text, got %q", text)
	}

	// Verify tool side effects were applied.
	data, err := os.ReadFile(filepath.Join(dir, "done.txt"))
	if err != nil {
		t.Fatalf("expected done.txt to exist: %v", err)
	}
	if string(data) != "work complete" {
		t.Fatalf("expected 'work complete', got %q", string(data))
	}

	foundBash := false
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "work complete") {
			foundBash = true
		}
	}
	if !foundBash {
		t.Fatal("expected tool-result message containing 'work complete'")
	}
}

func TestRunStillCapsOnEndlessToolCalls(t *testing.T) {
	// This is the existing turn-cap scenario under a new name.
	dir := t.TempDir()

	script := make([]fakeResponse, 50)
	for i := range script {
		script[i] = fakeResponse{
			toolCalls: []fakeToolCall{
				{name: "bash", args: `{"command":"echo turn"}`},
			},
		}
	}

	fa := &fakeAgent{t: t, script: script}

	_, _, _, err := Run(context.Background(), fa,
		"you are a helpful assistant", "run commands forever",
		dir, Config{MaxTurns: 3, MaxOutputBytes: 10000})
	if err == nil {
		t.Fatal("expected turn-cap error, got nil")
	}
	if !strings.Contains(err.Error(), "turn cap") {
		t.Fatalf("expected 'turn cap' in error, got: %v", err)
	}
}

func TestRun_SuccessPath(t *testing.T) {
	dir := t.TempDir()

	msg := `{"path":"hello.txt","content":"hello world"}`
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
				text: "I've written hello.txt and verified it contains 'hello world'. Done.",
			},
		},
	}

	_ = msg // used in script above

	text, cost, msgs, err := Run(context.Background(), fa,
		"you are a helpful assistant", "write hello.txt and verify it",
		dir, Config{MaxTurns: 10, MaxOutputBytes: 10000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "I've written hello.txt and verified it contains 'hello world'. Done." {
		t.Fatalf("unexpected final text: %q", text)
	}
	if cost != 0 {
		t.Logf("cost: %f (expected 0 for fake agent)", cost)
	}

	// Assert file was written (AC1: ≥1 file edit)
	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("expected hello.txt to exist: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}

	// Assert message history contains tool-call results
	foundBash := false
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "hello.txt") {
			foundBash = true
		}
	}
	if !foundBash {
		t.Fatal("expected tool-result message containing 'hello.txt'")
	}
}

func TestRun_ToolError_ModelAdapts(t *testing.T) {
	dir := t.TempDir()

	fa := &fakeAgent{
		t: t,
		// Script: read a nonexistent file → model gets error → writes the file → terminates.
		script: []fakeResponse{
			{
				// Turn 1: model tries to read a file that doesn't exist
				toolCalls: []fakeToolCall{
					{name: "read", args: `{"path":"missing.txt"}`},
				},
			},
			{
				// Turn 2: model receives the error, adapts by writing the file instead
				toolCalls: []fakeToolCall{
					{name: "write", args: `{"path":"missing.txt","content":"created after error"}`},
				},
			},
			{
				// Turn 3: model terminates with text
				text: "The file didn't exist, so I created it. Done.",
			},
		},
	}

	text, _, msgs, err := Run(context.Background(), fa,
		"you are a helpful assistant", "read missing.txt and if it fails, create it",
		dir, Config{MaxTurns: 10, MaxOutputBytes: 10000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "The file didn't exist, so I created it. Done." {
		t.Fatalf("unexpected final text: %q", text)
	}

	// Verify the error was communicated (tool result contains error text)
	foundError := false
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "error") && strings.Contains(m.Content, "missing.txt") {
			foundError = true
		}
	}
	if !foundError {
		t.Fatal("expected a tool-result message containing error for missing.txt")
	}

	// Verify the file was eventually created (model adapted)
	data, err := os.ReadFile(filepath.Join(dir, "missing.txt"))
	if err != nil {
		t.Fatalf("expected missing.txt to exist after adaptation: %v", err)
	}
	if string(data) != "created after error" {
		t.Fatalf("expected 'created after error', got %q", string(data))
	}
}

func TestRun_TurnCap(t *testing.T) {
	dir := t.TempDir()

	// Script: keep returning tool calls forever (non-terminating loop).
	script := make([]fakeResponse, 50)
	for i := range script {
		script[i] = fakeResponse{
			toolCalls: []fakeToolCall{
				{name: "bash", args: `{"command":"echo turn"}`},
			},
		}
	}

	fa := &fakeAgent{t: t, script: script}

	_, _, _, err := Run(context.Background(), fa,
		"you are a helpful assistant", "run commands forever",
		dir, Config{MaxTurns: 3, MaxOutputBytes: 10000})
	if err == nil {
		t.Fatal("expected turn-cap error, got nil")
	}
	if !strings.Contains(err.Error(), "turn cap") {
		t.Fatalf("expected 'turn cap' in error, got: %v", err)
	}
}

func TestRun_WorkspaceConfinement(t *testing.T) {
	dir := t.TempDir()

	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "read", args: `{"path":"/etc/passwd"}`},
				},
			},
			{
				text: "I was blocked from reading outside the workspace.",
			},
		},
	}

	text, _, msgs, err := Run(context.Background(), fa,
		"you are a helpful assistant", "read /etc/passwd",
		dir, Config{MaxTurns: 10, MaxOutputBytes: 10000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "I was blocked from reading outside the workspace." {
		t.Fatalf("unexpected text: %q", text)
	}

	// The tool result should contain a rejection message
	foundReject := false
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "rejected") {
			foundReject = true
		}
	}
	if !foundReject {
		t.Fatal("expected tool-result message containing 'rejected' for absolute path")
	}
}

func TestRun_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()

	fa := &fakeAgent{
		t: t,
		script: []fakeResponse{
			{
				toolCalls: []fakeToolCall{
					{name: "read", args: `{"path":"../../../etc/passwd"}`},
				},
			},
			{
				text: "Path traversal blocked.",
			},
		},
	}

	text, _, msgs, err := Run(context.Background(), fa,
		"you are a helpful assistant", "read ../../../etc/passwd",
		dir, Config{MaxTurns: 10, MaxOutputBytes: 10000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Path traversal blocked." {
		t.Fatalf("unexpected text: %q", text)
	}

	foundReject := false
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "rejected") {
			foundReject = true
		}
	}
	if !foundReject {
		t.Fatal("expected tool-result message containing 'rejected' for traversal")
	}
}
