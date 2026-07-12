package driver

import "testing"

// TestParseClaudeEnvelope_Array is the finding-P regression: current `claude -p
// --output-format json` emits a JSON ARRAY of stream events; the envelope is the
// "type":"result" element. Before the fix the parser unmarshalled the array
// into a single struct and errored.
func TestParseClaudeEnvelope_Array(t *testing.T) {
	raw := `[{"type":"system","subtype":"init"},` +
		`{"type":"assistant","message":{"role":"assistant"}},` +
		`{"type":"result","subtype":"success","result":"{\"verdict\":\"PASS\"}",` +
		`"total_cost_usd":0.42,"duration_ms":1234,"usage":{"input_tokens":10,"output_tokens":5}}]`
	env, err := parseClaudeEnvelope([]byte(raw))
	if err != nil {
		t.Fatalf("parse array: %v", err)
	}
	if env.Result != `{"verdict":"PASS"}` {
		t.Errorf("Result = %q, want the result-event's result", env.Result)
	}
	if env.costUSD() != 0.42 {
		t.Errorf("costUSD = %v, want 0.42", env.costUSD())
	}
	if env.DurationMS != 1234 {
		t.Errorf("DurationMS = %d, want 1234", env.DurationMS)
	}
}

// TestParseClaudeEnvelope_LegacyObject keeps back-compat with an older CLI that
// emitted a single result object.
func TestParseClaudeEnvelope_LegacyObject(t *testing.T) {
	env, err := parseClaudeEnvelope([]byte(`{"result":"hi","total_cost_usd":0.1,"duration_ms":5}`))
	if err != nil {
		t.Fatalf("parse object: %v", err)
	}
	if env.Result != "hi" {
		t.Errorf("Result = %q, want hi", env.Result)
	}
}

// TestExtractJSONObject is the finding-Q regression: the verifier's result text
// may be fenced (```json) and trailed by prose (an insight block); the verdict
// object must still be extracted.
func TestExtractJSONObject(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"verdict":"PASS"}`, `{"verdict":"PASS"}`},
		{"```json\n{\"verdict\":\"PASS\"}\n```", `{"verdict":"PASS"}`},
		{"```json\n{\"v\":1}\n```\n\n★ Insight ───", `{"v":1}`},
		{"  {\"a\":{\"b\":2}}  ", `{"a":{"b":2}}`},
	}
	for _, c := range cases {
		if got := extractJSONObject(c.in); got != c.want {
			t.Errorf("extractJSONObject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
