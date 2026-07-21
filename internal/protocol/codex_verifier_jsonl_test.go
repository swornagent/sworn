package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseNativeCodexVerifierJSONLReturnsExactAssessmentAndThread(t *testing.T) {
	t.Parallel()
	assessment := []byte(` {"schema_version":"sworn-verifier-assessment-v1"} `)
	contents := nativeCodexVerifierJSONLFixture(t, assessment)
	turn, err := ParseNativeCodexVerifierJSONL(contents)
	if err != nil {
		t.Fatal(err)
	}
	if string(turn.Assessment) != string(assessment) || turn.ThreadID != "thread-1" {
		t.Fatalf("native Codex verifier turn = %#v", turn)
	}

	command := strings.Replace(
		string(contents), `"type":"reasoning"`, `"type":"command_execution"`, 1,
	)
	if _, err := ParseNativeCodexVerifierJSONL([]byte(command)); err != nil {
		t.Fatalf("read-only command item was rejected: %v", err)
	}
	todo := strings.Replace(string(contents), `"type":"reasoning"`, `"type":"todo_list"`, 1)
	if _, err := ParseNativeCodexVerifierJSONL([]byte(todo)); err != nil {
		t.Fatalf("local todo item was rejected: %v", err)
	}
}

func TestParseNativeCodexVerifierJSONLRejectsLifecycleShapeAndToolSmuggling(t *testing.T) {
	t.Parallel()
	assessment := []byte(`{"schema_version":"sworn-verifier-assessment-v1"}`)
	valid := string(nativeCodexVerifierJSONLFixture(t, assessment))
	agentLine := nativeCodexVerifierAgentLine(t, assessment)
	terminal := `{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":1,"cache_write_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":1}}`
	lateItem := `{"type":"item.completed","item":{"id":"item-3","type":"reasoning","text":"late"}}`
	for _, test := range []struct {
		name   string
		output string
	}{
		{name: "empty", output: ""},
		{name: "malformed", output: "not-json\n"},
		{name: "duplicate key", output: `{"type":"thread.started","type":"turn.started","thread_id":"thread-1"}` + "\n"},
		{name: "extra thread field", output: strings.Replace(valid, `"thread_id":"thread-1"`, `"thread_id":"thread-1","extra":true`, 1)},
		{name: "duplicate thread", output: strings.Replace(valid, `{"type":"turn.started"}`, `{"type":"thread.started","thread_id":"thread-2"}`+"\n"+`{"type":"turn.started"}`, 1)},
		{name: "missing agent", output: strings.Replace(valid, agentLine+"\n", "", 1)},
		{name: "duplicate agent", output: strings.Replace(valid, terminal, agentLine+"\n"+terminal, 1)},
		{name: "agent started", output: strings.Replace(valid, agentLine, strings.Replace(agentLine, `"type":"item.completed"`, `"type":"item.started"`, 1), 1)},
		{name: "agent extra field", output: strings.Replace(valid, agentLine, strings.Replace(agentLine, `"text":`, `"extra":true,"text":`, 1), 1)},
		{name: "unknown event", output: strings.Replace(valid, `{"type":"turn.started"}`, `{"type":"future.event"}`, 1)},
		{name: "unknown item", output: strings.Replace(valid, `"type":"reasoning"`, `"type":"future_item"`, 1)},
		{name: "web search item", output: strings.Replace(valid, `"type":"reasoning"`, `"type":"web_search"`, 1)},
		{name: "MCP item", output: strings.Replace(valid, `"type":"reasoning"`, `"type":"mcp_tool_call"`, 1)},
		{name: "collaboration item", output: strings.Replace(valid, `"type":"reasoning"`, `"type":"collab_tool_call"`, 1)},
		{name: "file change item", output: strings.Replace(valid, `"type":"reasoning"`, `"type":"file_change"`, 1)},
		{name: "error item", output: strings.Replace(valid, `"type":"reasoning"`, `"type":"error"`, 1)},
		{name: "failed", output: strings.Replace(valid, terminal, `{"type":"turn.failed","error":{"message":"no"}}`, 1)},
		{name: "item after assessment", output: strings.Replace(valid, terminal, lateItem+"\n"+terminal, 1)},
		{name: "event after terminal", output: valid + lateItem + "\n"},
		{name: "usage missing field", output: strings.Replace(valid, `,"reasoning_output_tokens":1`, "", 1)},
		{name: "usage negative", output: strings.Replace(valid, `"output_tokens":2`, `"output_tokens":-1`, 1)},
		{name: "interior blank", output: strings.Replace(valid, `{"type":"turn.started"}`+"\n", `{"type":"turn.started"}`+"\n\n", 1)},
		{name: "event ceiling", output: strings.Repeat("{}\n", maximumNativeCodexVerifierEvents+1)},
		{name: "line ceiling", output: strings.Repeat("x", maximumNativeCodexVerifierEventLine+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseNativeCodexVerifierJSONL([]byte(test.output)); err == nil {
				t.Fatal("invalid native Codex verifier JSONL was accepted")
			}
		})
	}
}

func nativeCodexVerifierAgentLine(t testing.TB, assessment []byte) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "item-2", "type": "agent_message", "text": string(assessment)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func nativeCodexVerifierJSONLFixture(t testing.TB, assessment []byte) []byte {
	t.Helper()
	return []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"item-1","type":"reasoning","text":"reviewed"}}`,
		nativeCodexVerifierAgentLine(t, assessment),
		`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":1,"cache_write_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":1}}`,
		"",
	}, "\n"))
}
