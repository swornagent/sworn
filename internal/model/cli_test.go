package model

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMain intercepts re-exec of the test binary to act as a fake claude/codex
// CLI. When GO_TEST_FAKE_CLI is set, the test binary prints a canned response
// and exits — it does not run tests.
func TestMain(m *testing.M) {
	switch os.Getenv("GO_TEST_FAKE_CLI") {
	case "claude":
		fakeClaude()
		return
	case "claude-fail":
		fakeClaudeFail()
		return
	case "claude-hang":
		fakeClaudeHang()
		return
	case "codex":
		fakeCodex()
		return
	}
	os.Exit(m.Run())
}

// fakeClaude is a fake claude binary that records its invocation to a file and
// returns a canned PASS verdict on stdout.
func fakeClaude() {
	recordPath := os.Getenv("CLI_RECORD_PATH")
	if recordPath != "" {
		os.WriteFile(recordPath, []byte(strings.Join(os.Args, "\n")), 0644)
	}
	fmt.Println("PASS")
}

// fakeClaudeFail exits with code 1 — simulates auth failure (CLI not logged in).
func fakeClaudeFail() {
	recordPath := os.Getenv("CLI_RECORD_PATH")
	if recordPath != "" {
		os.WriteFile(recordPath, []byte(strings.Join(os.Args, "\n")), 0644)
	}
	fmt.Fprintln(os.Stderr, "claude: not logged in")
	os.Exit(1)
}

// fakeClaudeHang sleeps forever — simulates a hung subprocess to test timeout.
func fakeClaudeHang() {
	select {}
}

// fakeCodex is a stub for codex — not yet wired because codex is deferred.
func fakeCodex() {
	fmt.Println("CODEX_PASS")
}

// testBinaryPath returns the path to the running test binary so it can re-exec
// itself as a fake CLI.
func testBinaryPath(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return path
}

func TestClaudeCLI_NormalDispatch(t *testing.T) {
	t.Setenv("CLAUDE_BIN", testBinaryPath(t))
	t.Setenv("GO_TEST_FAKE_CLI", "claude")
	recordPath := t.TempDir() + "/invocation.txt"
	t.Setenv("CLI_RECORD_PATH", recordPath)

	d := newClaudeCLI("sonnet")
text, cost, _, _, err := d.Verify(context.Background(), "you are a verifier", "check this")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if text != "PASS" {
		t.Errorf("Verify() = %q, want %q", text, "PASS")
	}
	if cost != 0 {
		t.Errorf("cost = %f, want 0", cost)
	}

	// Assert the fake binary was invoked with the right args.
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read invocation record: %v", err)
	}
	invocation := string(raw)

	// Must include --no-session-persistence and --model sonnet.
	if !strings.Contains(invocation, "--no-session-persistence") {
		t.Errorf("invocation missing --no-session-persistence: %s", invocation)
	}
	if !strings.Contains(invocation, "--model") {
		t.Errorf("invocation missing --model: %s", invocation)
	}
	if !strings.Contains(invocation, "sonnet") {
		t.Errorf("invocation missing model name: %s", invocation)
	}
	// The prompt must contain both systemPrompt and userPayload.
	if !strings.Contains(invocation, "you are a verifier") {
		t.Errorf("invocation missing system prompt: %s", invocation)
	}
	if !strings.Contains(invocation, "check this") {
		t.Errorf("invocation missing user payload: %s", invocation)
	}
}

func TestClaudeCLI_MissingBinary(t *testing.T) {
	t.Setenv("CLAUDE_BIN", "/nonexistent/claude-binary-xyz")
	d := newClaudeCLI("sonnet")

_, _, _, _, err := d.Verify(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}

	me, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *model.Error, got %T: %v", err, err)
	}
	if me.Kind != KindOther {
		t.Errorf("missing binary Kind = %s, want KindOther", me.Kind)
	}
	if !strings.Contains(me.Message, "not found on PATH") {
		t.Errorf("message should mention PATH: %s", me.Message)
	}
}

func TestClaudeCLI_AuthFailure(t *testing.T) {
	t.Setenv("CLAUDE_BIN", testBinaryPath(t))
	t.Setenv("GO_TEST_FAKE_CLI", "claude-fail")

	d := newClaudeCLI("sonnet")
_, _, _, _, err := d.Verify(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error for auth failure, got nil")
	}

	me, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *model.Error, got %T: %v", err, err)
	}
	if me.Kind != KindAuth {
		t.Errorf("auth failure Kind = %s, want KindAuth", me.Kind)
	}
	if !strings.Contains(me.Message, "exited with code 1") {
		t.Errorf("message should mention exit code: %s", me.Message)
	}
}

func TestClaudeCLI_Timeout(t *testing.T) {
	t.Setenv("CLAUDE_BIN", testBinaryPath(t))
	t.Setenv("GO_TEST_FAKE_CLI", "claude-hang")
	t.Setenv("SWORN_CLI_TIMEOUT", "500ms")

	d := newClaudeCLI("sonnet")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

_, _, _, _, err := d.Verify(ctx, "s", "u")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	me, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *model.Error, got %T: %v", err, err)
	}
	if me.Kind != KindTransient {
		t.Errorf("timeout Kind = %s, want KindTransient", me.Kind)
	}
	if !strings.Contains(me.Message, "timed out") {
		t.Errorf("message should mention timeout: %s", me.Message)
	}
}

func TestClaudeCLI_FromEnvIntegration(t *testing.T) {
	t.Setenv("CLAUDE_BIN", testBinaryPath(t))
	t.Setenv("GO_TEST_FAKE_CLI", "claude")

	verifier, err := FromEnv("claude-cli/sonnet")
	if err != nil {
		t.Fatalf("FromEnv(claude-cli/sonnet) error: %v", err)
	}

text, cost, _, _, err := verifier.Verify(context.Background(), "sys", "payload")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if text != "PASS" {
		t.Errorf("Verify() = %q, want %q", text, "PASS")
	}
	if cost != 0 {
		t.Errorf("cost = %f, want 0", cost)
	}
}

func TestClaudeCLI_EmptyModel(t *testing.T) {
	_, err := FromEnv("claude-cli/")
	if err == nil {
		t.Fatal("expected error for empty model, got nil")
	}
	if !strings.Contains(err.Error(), "model required") || !strings.Contains(err.Error(), "provider and model required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCodex_Deferred(t *testing.T) {
	t.Run("NewClient", func(t *testing.T) {
		_, err := NewClient("codex/gpt-5", ProviderConfig{})
		if err == nil {
			t.Fatal("expected deferral error, got nil")
		}
		if !strings.Contains(err.Error(), "codex support deferred") {
			t.Errorf("error = %v, want codex support deferred", err)
		}
		if !strings.Contains(err.Error(), ErrDriverNotImplemented.Error()) {
			t.Errorf("error should wrap ErrDriverNotImplemented: %v", err)
		}
	})

	t.Run("FromEnv", func(t *testing.T) {
		_, err := FromEnv("codex/gpt-5")
		if err == nil {
			t.Fatal("expected deferral error, got nil")
		}
		if !strings.Contains(err.Error(), "codex support deferred") {
			t.Errorf("error = %v, want codex support deferred", err)
		}
	})
}

func TestClaudeCLI_NoProxyRouting(t *testing.T) {
	// When SWORN_LOGIN is present (simulating a logged-in user), claude-cli
	// must bypass proxy routing entirely — no proxy URL, no OAI wrapper.
	t.Setenv("CLAUDE_BIN", testBinaryPath(t))
	t.Setenv("GO_TEST_FAKE_CLI", "claude")

	v, err := FromEnv("claude-cli/sonnet")
	if err != nil {
		t.Fatalf("FromEnv(claude-cli/sonnet) error: %v", err)
	}

	// Verify it's a cliDriver, not an OAI wrapper.
	if _, ok := v.(*cliDriver); !ok {
		t.Errorf("FromEnv(claude-cli/sonnet) returned %T, want *cliDriver", v)
	}
}

func TestClaudeCLI_ModelPassthrough(t *testing.T) {
	t.Setenv("CLAUDE_BIN", testBinaryPath(t))
	t.Setenv("GO_TEST_FAKE_CLI", "claude")
	recordPath := t.TempDir() + "/invocation.txt"
	t.Setenv("CLI_RECORD_PATH", recordPath)

	d := newClaudeCLI("haiku")
_, _, _, _, err := d.Verify(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}

	raw, _ := os.ReadFile(recordPath)
	invocation := string(raw)

	// The model "haiku" must appear in the args (--model haiku).
	if !strings.Contains(invocation, "--model") || !strings.Contains(invocation, "haiku") {
		t.Errorf("model 'haiku' not passed through in args: %s", invocation)
	}
}

// Ensure exec.ErrNotFound is not directly referenced in cli.go — the driver
// type-asserts *exec.Error and *fs.PathError instead.
func TestExecErrorNotFound_NotImported(t *testing.T) {
	body, err := os.ReadFile("cli.go")
	if err != nil {
		t.Fatalf("read cli.go: %v", err)
	}
	if strings.Contains(string(body), "exec.ErrNotFound") {
		t.Error("cli.go must not reference exec.ErrNotFound — use *exec.Error or *fs.PathError type assertion instead")
	}
}
