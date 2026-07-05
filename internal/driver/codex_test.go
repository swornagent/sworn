package driver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCodexDispatchImplementer proves AC-01: the driver spawns
// `codex exec --json -C <dir> <prompt>` with cmd.Dir set to WorktreeRoot
// and normalizes the JSONL event stream into Result.
func TestCodexDispatchImplementer(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CODEX", "envelope")
	dir := gitWorktree(t)
	recordPath := filepath.Join(t.TempDir(), "invocation.txt")
	t.Setenv("CLI_RECORD_PATH", recordPath)

	d := &CodexDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "gpt-5-codex",
		SystemPrompt: "you are the implementer",
		Payload:      "implement the slice",
		WorktreeRoot: dir,
	})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if result.Status != StatusOK {
		t.Fatalf("Status = %s, want %s", result.Status, StatusOK)
	}
	if result.ResultText != "done" {
		t.Errorf("ResultText = %q, want %q", result.ResultText, "done")
	}
	if result.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0 (codex never reports cost)", result.CostUSD)
	}
	if result.CostSource != "provider-reported" {
		t.Errorf("CostSource = %q, want provider-reported", result.CostSource)
	}
	if result.InputTokens != 100 || result.OutputTokens != 50 {
		t.Errorf("tokens = %d/%d, want 100/50", result.InputTokens, result.OutputTokens)
	}
	if result.ModelID != "gpt-5-codex" {
		t.Errorf("ModelID = %q, want fallback to requested model %q", result.ModelID, "gpt-5-codex")
	}
	if result.DurationMS <= 0 {
		t.Errorf("DurationMS = %d, want a positive measured fallback (codex never reports duration)", result.DurationMS)
	}

	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read invocation record: %v", err)
	}
	lines := strings.SplitN(string(raw), "\n", 2)
	if lines[0] != dir {
		t.Errorf("child cmd.Dir = %q, want %q", lines[0], dir)
	}
	invocation := lines[1]
	if !strings.Contains(invocation, "exec") || !strings.Contains(invocation, "--json") ||
		!strings.Contains(invocation, "-C") || !strings.Contains(invocation, dir) {
		t.Errorf("invocation missing expected argv: %s", invocation)
	}
	if strings.Contains(invocation, "--ephemeral") {
		t.Errorf("implementer dispatch must not pass --ephemeral: %s", invocation)
	}
}

// TestCodexDispatchVerifier proves AC-02: verifier dispatches pass
// --ephemeral, include VerdictSchema in the prompt, and populate
// Result.StructuredJSON when the final agent_message parses as a JSON
// object.
func TestCodexDispatchVerifier(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CODEX", "verdict")
	dir := gitWorktree(t)
	recordPath := filepath.Join(t.TempDir(), "invocation.txt")
	t.Setenv("CLI_RECORD_PATH", recordPath)

	schema := []byte(`{"type":"object","required":["verdict"]}`)
	d := &CodexDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:          RoleVerifier,
		ModelID:       "gpt-5-codex",
		SystemPrompt:  "you are the verifier",
		Payload:       "check this diff",
		WorktreeRoot:  dir,
		VerdictSchema: schema,
	})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if result.Status != StatusOK {
		t.Fatalf("Status = %s, want %s", result.Status, StatusOK)
	}

	var verdict map[string]interface{}
	if err := json.Unmarshal(result.StructuredJSON, &verdict); err != nil {
		t.Fatalf("StructuredJSON did not parse: %v (%s)", err, result.StructuredJSON)
	}
	if verdict["verdict"] != "PASS" {
		t.Errorf("StructuredJSON verdict = %v, want PASS", verdict["verdict"])
	}

	raw, _ := os.ReadFile(recordPath)
	invocation := strings.SplitN(string(raw), "\n", 2)[1]
	if !strings.Contains(invocation, "--ephemeral") {
		t.Errorf("verifier dispatch missing --ephemeral: %s", invocation)
	}
	if !strings.Contains(invocation, string(schema)) {
		t.Errorf("verifier dispatch prompt missing VerdictSchema: %s", invocation)
	}
}

// TestCodexDispatchVerifier_ProtocolError proves the fail-closed half of
// AC-02: when the final agent_message does not parse as a JSON object,
// Dispatch returns Status=error with ErrKind=protocol.
func TestCodexDispatchVerifier_ProtocolError(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CODEX", "verdict-bad")
	dir := gitWorktree(t)

	d := &CodexDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:          RoleVerifier,
		ModelID:       "gpt-5-codex",
		WorktreeRoot:  dir,
		VerdictSchema: []byte(`{"type":"object"}`),
	})
	if err == nil {
		t.Fatal("expected error for non-JSON-object verifier result, got nil")
	}
	if result.Status != StatusError {
		t.Errorf("Status = %s, want %s", result.Status, StatusError)
	}
	if result.ErrKind != ErrKindProtocol {
		t.Errorf("ErrKind = %s, want %s", result.ErrKind, ErrKindProtocol)
	}
}

// TestCodexWorktreeGate proves the Rule-11 fail-closed target assertion: an
// invalid WorktreeRoot is rejected with ErrKind=config before any process
// is spawned.
func TestCodexWorktreeGate(t *testing.T) {
	notAWorktree := t.TempDir() // no `git init` — not a working tree

	marker := filepath.Join(t.TempDir(), "invoked.marker")
	t.Setenv("GO_TEST_FAKE_CODEX", "envelope")
	t.Setenv("CLI_RECORD_PATH", marker)

	d := &CodexDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "gpt-5-codex",
		WorktreeRoot: notAWorktree,
	})
	if err == nil {
		t.Fatal("expected error for non-worktree WorktreeRoot, got nil")
	}
	if result.Status != StatusError {
		t.Errorf("Status = %s, want %s", result.Status, StatusError)
	}
	if result.ErrKind != ErrKindConfig {
		t.Errorf("ErrKind = %s, want %s", result.ErrKind, ErrKindConfig)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("Rule 11 violation: child process was spawned despite a failed worktree assertion")
	}
}

// TestCodexErrorMapping proves AC-03, table-driven across all three
// subprocess failure classes. Non-zero exit maps to ErrKindAuth — the
// binding cross-driver contract confirmed at this slice's design review
// (design.md decision 6), not the spec.json AC-03 parenthetical's stale
// "provider" text (tracked: sworn#84).
func TestCodexErrorMapping(t *testing.T) {
	tests := []struct {
		name        string
		fakeMode    string
		binary      string
		timeout     time.Duration
		wantErrKind string
		wantSubstr  string
	}{
		{
			name:        "timeout",
			fakeMode:    "hang",
			timeout:     500 * time.Millisecond,
			wantErrKind: ErrKindTransient,
			wantSubstr:  "timed out",
		},
		{
			name:        "binary not found",
			binary:      "/nonexistent/codex-binary-xyz",
			wantErrKind: ErrKindConfig,
			wantSubstr:  "not found on PATH",
		},
		{
			name:        "non-zero exit",
			fakeMode:    "fail",
			wantErrKind: ErrKindAuth,
			wantSubstr:  "exited with code 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fakeMode != "" {
				t.Setenv("GO_TEST_FAKE_CODEX", tt.fakeMode)
			}
			binary := tt.binary
			if binary == "" {
				binary = testBinaryPath(t)
			}
			dir := gitWorktree(t)

			d := &CodexDriver{Binary: binary}
			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.name == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
			}

			result, err := d.Dispatch(ctx, DispatchInput{
				Role:         RoleImplementer,
				ModelID:      "gpt-5-codex",
				WorktreeRoot: dir,
				Timeout:      tt.timeout,
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if result.Status != StatusError {
				t.Errorf("Status = %s, want %s", result.Status, StatusError)
			}
			if result.ErrKind != tt.wantErrKind {
				t.Errorf("ErrKind = %s, want %s", result.ErrKind, tt.wantErrKind)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error %q missing substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

// TestCodexEnvHygiene proves AC-04: GOCACHE/GOMODCACHE are redirected
// outside the worktree and HOME is left untouched — the same shared
// hygieneEnv() the claude driver uses.
func TestCodexEnvHygiene(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CODEX", "record-env")
	dir := gitWorktree(t)
	recordPath := filepath.Join(t.TempDir(), "env.json")
	t.Setenv("CLI_RECORD_PATH", recordPath)
	realHome := os.Getenv("HOME")

	d := &CodexDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "gpt-5-codex",
		WorktreeRoot: dir,
	})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if result.Status != StatusOK {
		t.Fatalf("Status = %s, want %s", result.Status, StatusOK)
	}

	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read env record: %v", err)
	}
	var rec struct {
		Cwd        string `json:"cwd"`
		GOCACHE    string `json:"gocache"`
		GOMODCACHE string `json:"gomodcache"`
		HOME       string `json:"home"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal env record: %v", err)
	}

	if rec.HOME != realHome {
		t.Errorf("child HOME = %q, want unmodified %q", rec.HOME, realHome)
	}
	if strings.HasPrefix(rec.GOCACHE, dir) || strings.HasPrefix(rec.GOMODCACHE, dir) {
		t.Errorf("GOCACHE/GOMODCACHE leaked inside worktree: gocache=%q gomodcache=%q dir=%q", rec.GOCACHE, rec.GOMODCACHE, dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	for _, e := range entries {
		if e.Name() == "go-build" || e.Name() == "go-mod" || strings.Contains(e.Name(), "sworn-driver-cache") {
			t.Errorf("cache directory %q leaked inside worktree %q", e.Name(), dir)
		}
	}
}

// TestCodexEnvelopeDefaults proves the defensive-parsing fallbacks
// (design.md decision 2, corrected at design review): a stream with no
// turn.completed event at all degrades to CostSource=unknown/zero tokens,
// ModelID falls back to the requested model, and DurationMS falls back to
// the measured wall-clock time — the normal path for codex, not a rare
// edge case, since codex never reports model/duration at all.
func TestCodexEnvelopeDefaults(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CODEX", "minimal")
	dir := gitWorktree(t)

	d := &CodexDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "gpt-5-codex",
		WorktreeRoot: dir,
	})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if result.CostSource != "unknown" {
		t.Errorf("CostSource = %q, want unknown", result.CostSource)
	}
	if result.CostUSD != 0 || result.InputTokens != 0 || result.OutputTokens != 0 {
		t.Errorf("expected zero cost/tokens for a stream with no turn.completed, got CostUSD=%v InputTokens=%d OutputTokens=%d", result.CostUSD, result.InputTokens, result.OutputTokens)
	}
	if result.ModelID != "gpt-5-codex" {
		t.Errorf("ModelID = %q, want fallback to requested model %q", result.ModelID, "gpt-5-codex")
	}
	if result.DurationMS <= 0 {
		t.Errorf("DurationMS = %d, want a positive measured fallback", result.DurationMS)
	}
}

// TestCodexDispatch_OuterStreamProtocolError proves that output containing
// a line which is not valid JSON at all maps to ErrKind=protocol.
func TestCodexDispatch_OuterStreamProtocolError(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CODEX", "not-json")
	dir := gitWorktree(t)

	d := &CodexDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "gpt-5-codex",
		WorktreeRoot: dir,
	})
	if err == nil {
		t.Fatal("expected error for non-JSON output, got nil")
	}
	if result.ErrKind != ErrKindProtocol {
		t.Errorf("ErrKind = %s, want %s", result.ErrKind, ErrKindProtocol)
	}
}

// TestCodexDriver_Name_Roles proves the driver identifies itself and
// declares implementer+verifier only (mirrors claude driver's design
// decision 4 — no captain).
func TestCodexDriver_Name_Roles(t *testing.T) {
	d := NewCodexDriver()
	if d.Name() != "codex-subprocess" {
		t.Errorf("Name() = %q, want codex-subprocess", d.Name())
	}
	roles := d.Roles()
	if !roles.Has(RoleImplementer) || !roles.Has(RoleVerifier) {
		t.Errorf("Roles() = %v, want implementer+verifier", roles)
	}
	if roles.Has(RoleCaptain) {
		t.Error("Roles() must not declare captain")
	}
}
