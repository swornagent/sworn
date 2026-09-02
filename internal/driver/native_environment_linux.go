package driver

import "strconv"

// nativeMCPToolTimeoutMillis is the per-tool-call ceiling handed to the
// claude CLI (MCP_TOOL_TIMEOUT / MCP_TOOL_IDLE_TIMEOUT, milliseconds). The
// CLI's own default caps every sworn tool call at roughly a minute, which is
// shorter than a real project's test suite: on the first fired dogfood run
// the verifier could not literally complete the contract's `go test ./...`
// (one package alone needs ~164s) and honestly refused to claim it. Sworn's
// invocation deadline already bounds every tool call, so the CLI's cap is
// lifted to the maximum invocation budget and the invocation governs.
var nativeMCPToolTimeoutMillis = strconv.FormatInt(MaxTimeoutMillis, 10)

// nativeClaudeEnvironmentEntries is the ONE list both the claude launch
// environment and the in-sandbox environment certificate derive from. The
// certificate compares the process environment for exact equality (entry
// count and every value), so the two sites must never be maintained apart:
// a variable added in one place and not the other fails every claude
// dispatch with runtime.environment_entry_count_mismatch.
func nativeClaudeEnvironmentEntries(captureBaseURL string) [][2]string {
	entries := [][2]string{
		{"CLAUDE_CODE_DISABLE_AUTO_MEMORY", "1"},
		{"CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY", "1"},
		{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1"},
		{"DISABLE_AUTOUPDATER", "1"},
		{"DISABLE_TELEMETRY", "1"},
		{"DISABLE_ERROR_REPORTING", "1"},
		{"DISABLE_FEEDBACK_COMMAND", "1"},
		{"MCP_TOOL_TIMEOUT", nativeMCPToolTimeoutMillis},
		{"MCP_TOOL_IDLE_TIMEOUT", nativeMCPToolTimeoutMillis},
	}
	if captureBaseURL != "" {
		entries = append(entries, [2]string{"ANTHROPIC_BASE_URL", captureBaseURL})
	}
	return entries
}
