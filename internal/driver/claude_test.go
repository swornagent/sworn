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

// gitWorktree creates a real git working tree in a temp dir and returns its
// path, so AssertWorktree passes.
func gitWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	return resolved
}

// TestClaudeDispatchImplementer proves AC-01: the driver spawns
// `claude -p --output-format json --model <model>` with cmd.Dir set to
// WorktreeRoot and normalizes the JSON envelope into Result.
func TestClaudeDispatchImplementer(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "envelope")
	dir := gitWorktree(t)
	recordPath := filepath.Join(t.TempDir(), "invocation.txt")
	t.Setenv("CLI_RECORD_PATH", recordPath)

	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "sonnet",
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
	if result.CostUSD != 0.05 {
		t.Errorf("CostUSD = %v, want 0.05", result.CostUSD)
	}
	if result.CostSource != CostSourceCLI {
		t.Errorf("CostSource = %q, want %q", result.CostSource, CostSourceCLI)
	}
	if result.InputTokens != 100 || result.OutputTokens != 50 {
		t.Errorf("tokens = %d/%d, want 100/50", result.InputTokens, result.OutputTokens)
	}
	if result.ModelID != "claude-sonnet-4" {
		t.Errorf("ModelID = %q, want claude-sonnet-4", result.ModelID)
	}
	if result.DurationMS != 1234 {
		t.Errorf("DurationMS = %d, want 1234 (from envelope)", result.DurationMS)
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
	if !strings.Contains(invocation, "-p") || !strings.Contains(invocation, "--output-format") ||
		!strings.Contains(invocation, "json") || !strings.Contains(invocation, "--model") ||
		!strings.Contains(invocation, "sonnet") {
		t.Errorf("invocation missing expected argv: %s", invocation)
	}
	if strings.Contains(invocation, "--no-session-persistence") {
		t.Errorf("implementer dispatch must not pass --no-session-persistence: %s", invocation)
	}
}

// TestClaudeDispatchVerifier proves AC-03: verifier dispatches pass
// --no-session-persistence, include StructuredSchema in the prompt, and
// populate Result.StructuredJSON when the CLI's result text is a JSON
// object.
func TestClaudeDispatchVerifier(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "verdict")
	dir := gitWorktree(t)
	recordPath := filepath.Join(t.TempDir(), "invocation.txt")
	t.Setenv("CLI_RECORD_PATH", recordPath)

	schema := []byte(`{"type":"object","required":["verdict"]}`)
	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:             RoleVerifier,
		ModelID:          "sonnet",
		SystemPrompt:     "you are the verifier",
		Payload:          "check this diff",
		WorktreeRoot:     dir,
		StructuredSchema: schema,
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
	if !strings.Contains(invocation, "--no-session-persistence") {
		t.Errorf("verifier dispatch missing --no-session-persistence: %s", invocation)
	}
	if !strings.Contains(invocation, string(schema)) {
		t.Errorf("verifier dispatch prompt missing StructuredSchema: %s", invocation)
	}
}

// TestClaudeDispatchVerifier_ProtocolError proves the fail-closed half of
// AC-03: when the CLI's result text does not parse as a JSON object,
// Dispatch returns Status=error with ErrKind=protocol.
func TestClaudeDispatchVerifier_ProtocolError(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "verdict-bad")
	dir := gitWorktree(t)

	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:             RoleVerifier,
		ModelID:          "sonnet",
		WorktreeRoot:     dir,
		StructuredSchema: []byte(`{"type":"object"}`),
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

// TestClaudeWorktreeGate proves AC-02: an invalid WorktreeRoot is rejected
// with ErrKind=config before any process is spawned.
func TestClaudeWorktreeGate(t *testing.T) {
	notAWorktree := t.TempDir() // no `git init` — not a working tree

	// Point Binary at a marker-writing fake; if it is ever invoked, the
	// marker file will exist after Dispatch returns.
	marker := filepath.Join(t.TempDir(), "invoked.marker")
	t.Setenv("GO_TEST_FAKE_CLAUDE", "envelope")
	t.Setenv("CLI_RECORD_PATH", marker)

	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "sonnet",
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

// TestClaudeErrorMapping proves AC-04, table-driven across all three
// subprocess failure classes.
func TestClaudeErrorMapping(t *testing.T) {
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
			binary:      "/nonexistent/claude-binary-xyz",
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
				t.Setenv("GO_TEST_FAKE_CLAUDE", tt.fakeMode)
			}
			binary := tt.binary
			if binary == "" {
				binary = testBinaryPath(t)
			}
			dir := gitWorktree(t)

			d := &ClaudeDriver{Binary: binary}
			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.name == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
			}

			result, err := d.Dispatch(ctx, DispatchInput{
				Role:         RoleImplementer,
				ModelID:      "sonnet",
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

// TestClaudeEnvHygiene proves AC-05: GOCACHE/GOMODCACHE are redirected
// outside the worktree and HOME is left untouched.
func TestClaudeEnvHygiene(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "record-env")
	dir := gitWorktree(t)
	recordPath := filepath.Join(t.TempDir(), "env.json")
	t.Setenv("CLI_RECORD_PATH", recordPath)
	realHome := os.Getenv("HOME")

	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "sonnet",
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

	// No cache directory should appear inside the worktree after dispatch.
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

// TestClaudeEnvelopeDefaults proves the defensive-parsing fallbacks from
// design.md decision 6: a minimal envelope (only "result" set) degrades to
// CostSource=unknown/zero tokens, ModelID falls back to the requested
// model, and DurationMS falls back to the measured wall-clock time.
func TestClaudeEnvelopeDefaults(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "minimal")
	dir := gitWorktree(t)

	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "sonnet",
		WorktreeRoot: dir,
	})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if result.CostSource != CostSourceUnknown {
		t.Errorf("CostSource = %q, want %q", result.CostSource, CostSourceUnknown)
	}
	if result.CostUSD != 0 || result.InputTokens != 0 || result.OutputTokens != 0 {
		t.Errorf("expected zero cost/tokens for a minimal envelope, got CostUSD=%v InputTokens=%d OutputTokens=%d", result.CostUSD, result.InputTokens, result.OutputTokens)
	}
	if result.ModelID != "sonnet" {
		t.Errorf("ModelID = %q, want fallback to requested model %q", result.ModelID, "sonnet")
	}
	if result.DurationMS <= 0 {
		t.Errorf("DurationMS = %d, want a positive measured fallback", result.DurationMS)
	}
}

// TestClaudeEnvelopeExplicitZeroCostIsUnknown proves design_decision D1
// (Coach-ratified, S08 status.json): an EXPLICIT reported zero
// (total_cost_usd: 0) is NOT treated as a positively identified subscription
// marker — it classifies CostSourceUnknown, the same as a missing field,
// because an explicit zero is equally consistent with a genuinely free turn
// or an envelope quirk as with subscription coverage, and claudeEnvelope
// carries no field that distinguishes the two. This is the fail-closed
// posture the Coach ratified in place of a TotalCostUSD==0 -> "subscription"
// inference (Rule 2 note: no positively identified marker exists in the
// current CLI output — see proof.json not_delivered).
func TestClaudeEnvelopeExplicitZeroCostIsUnknown(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "zero-cost")
	dir := gitWorktree(t)

	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "sonnet",
		WorktreeRoot: dir,
	})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}
	if result.CostSource != CostSourceUnknown {
		t.Errorf("CostSource = %q, want %q (D1: explicit zero is not a subscription marker)", result.CostSource, CostSourceUnknown)
	}
	if result.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0", result.CostUSD)
	}
	// Tokens/model/duration still parse normally — only cost classification
	// is affected by the ambiguous zero.
	if result.InputTokens != 10 || result.OutputTokens != 5 {
		t.Errorf("tokens = %d/%d, want 10/5", result.InputTokens, result.OutputTokens)
	}
}

// TestClaudeDispatch_OuterEnvelopeProtocolError proves that output which is
// not valid JSON at all (not even the outer envelope) maps to ErrKind=
// protocol, per design.md decision 6.
func TestClaudeDispatch_OuterEnvelopeProtocolError(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "not-json")
	dir := gitWorktree(t)

	d := &ClaudeDriver{Binary: testBinaryPath(t)}
	result, err := d.Dispatch(context.Background(), DispatchInput{
		Role:         RoleImplementer,
		ModelID:      "sonnet",
		WorktreeRoot: dir,
	})
	if err == nil {
		t.Fatal("expected error for non-JSON output, got nil")
	}
	if result.ErrKind != ErrKindProtocol {
		t.Errorf("ErrKind = %s, want %s", result.ErrKind, ErrKindProtocol)
	}
}

// TestClaudeDriver_Name_Roles proves the driver identifies itself and
// declares implementer+verifier only (design decision 4 — no captain).
func TestClaudeDriver_Name_Roles(t *testing.T) {
	d := NewClaudeDriver()
	if d.Name() != "claude-subprocess" {
		t.Errorf("Name() = %q, want claude-subprocess", d.Name())
	}
	roles := d.Roles()
	if !roles.Has(RoleImplementer) || !roles.Has(RoleVerifier) {
		t.Errorf("Roles() = %v, want implementer+verifier", roles)
	}
	if roles.Has(RoleCaptain) {
		t.Error("Roles() must not declare captain (design decision 4)")
	}
}
