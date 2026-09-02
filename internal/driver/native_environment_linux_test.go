package driver

import (
	"bytes"
	"strconv"
	"testing"
)

// The claude CLI caps each MCP tool call at its own default (~60s) unless
// told otherwise. Sworn must hand it a ceiling that lets the invocation
// deadline govern, or a contract check longer than a minute can never be
// literally completed inside a tool call (the first fired dogfood run's
// verifier BLOCKED on exactly this).
func TestNativeClaudeEnvironmentLiftsTheMCPToolCallCap(t *testing.T) {
	want := strconv.FormatInt(MaxTimeoutMillis, 10)
	found := map[string]string{}
	for _, entry := range nativeClaudeEnvironmentEntries("") {
		found[entry[0]] = entry[1]
	}
	for _, key := range []string{"MCP_TOOL_TIMEOUT", "MCP_TOOL_IDLE_TIMEOUT"} {
		if found[key] != want {
			t.Fatalf("%s = %q, want the maximum invocation budget %q", key, found[key], want)
		}
	}
	if _, leaked := found["ANTHROPIC_BASE_URL"]; leaked {
		t.Fatal("a production launch must not carry a capture base URL")
	}
	withCapture := nativeClaudeEnvironmentEntries("http://127.0.0.1:4545")
	last := withCapture[len(withCapture)-1]
	if last[0] != "ANTHROPIC_BASE_URL" || last[1] != "http://127.0.0.1:4545" {
		t.Fatalf("capture launch does not end with the capture base URL: %v", last)
	}
}

// The launch environment and the in-sandbox certificate derive from one
// list, so the environment sworn composes must be exactly the environment
// the certificate accepts - and the certificate stays exact: dropping the
// new entry is refused.
func TestNativeClaudeEnvironmentRoundTripsThroughTheCertificate(t *testing.T) {
	var body []byte
	for _, base := range [][2]string{
		{"HOME", "/home/sworn"}, {"TMPDIR", "/tmp"},
		{"LANG", "C.UTF-8"}, {"LC_ALL", "C.UTF-8"}, {"TZ", "UTC"},
	} {
		body = append(body, []byte(base[0]+"="+base[1])...)
		body = append(body, 0)
	}
	for _, entry := range nativeClaudeEnvironmentEntries("") {
		body = append(body, []byte(entry[0]+"="+entry[1])...)
		body = append(body, 0)
	}
	capability := []byte("capability-token-never-present")
	if err := validateNativeProcessEnvironment(body, ProfileClaude, capability, nil); err != nil {
		t.Fatalf("composed environment refused by the certificate: %v", err)
	}
	stripped := bytes.ReplaceAll(
		body, []byte("MCP_TOOL_TIMEOUT="+nativeMCPToolTimeoutMillis+"\x00"), nil,
	)
	if err := validateNativeProcessEnvironment(stripped, ProfileClaude, capability, nil); err == nil {
		t.Fatal("certificate accepted an environment missing MCP_TOOL_TIMEOUT")
	}
}
