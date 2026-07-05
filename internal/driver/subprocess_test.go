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

// TestMain intercepts re-exec of the test binary to act as a fake claude
// CLI, following the same convention as internal/model/cli_test.go. When
// GO_TEST_FAKE_CLAUDE is set, the test binary emits a canned response and
// exits — it does not run tests.
func TestMain(m *testing.M) {
	switch os.Getenv("GO_TEST_FAKE_CLAUDE") {
	case "envelope":
		fakeClaudeEnvelope()
		return
	case "verdict":
		fakeClaudeVerdict()
		return
	case "verdict-bad":
		fakeClaudeVerdictBad()
		return
	case "minimal":
		fakeClaudeMinimal()
		return
	case "fail":
		fakeClaudeFail()
		return
	case "hang":
		fakeClaudeHang()
		return
	case "record-env":
		fakeClaudeRecordEnv()
		return
	case "not-json":
		fakeClaudeNotJSON()
		return
	}
	switch os.Getenv("GO_TEST_FAKE_CODEX") {
	case "envelope":
		fakeCodexEnvelope()
		return
	case "verdict":
		fakeCodexVerdict()
		return
	case "verdict-bad":
		fakeCodexVerdictBad()
		return
	case "minimal":
		fakeCodexMinimal()
		return
	case "fail":
		fakeCodexFail()
		return
	case "hang":
		fakeCodexHang()
		return
	case "record-env":
		fakeCodexRecordEnv()
		return
	case "not-json":
		fakeCodexNotJSON()
		return
	}
	os.Exit(m.Run())
}

func recordInvocation() {
	if p := os.Getenv("CLI_RECORD_PATH"); p != "" {
		cwd, _ := os.Getwd()
		os.WriteFile(p, []byte(cwd+"\n"+strings.Join(os.Args, "\n")), 0644)
	}
}

// fakeClaudeEnvelope emits a full result envelope for the implementer path.
func fakeClaudeEnvelope() {
	recordInvocation()
	writeStdoutLine(`{"result":"done","total_cost_usd":0.05,"usage":{"input_tokens":100,"output_tokens":50},"duration_ms":1234,"model":"claude-sonnet-4"}`)
}

// fakeClaudeVerdict emits an envelope whose result field is itself a JSON
// object string — the verifier-role happy path (AC-03).
func fakeClaudeVerdict() {
	recordInvocation()
	writeStdoutLine(`{"result":"{\"verdict\":\"PASS\",\"reasons\":[]}","total_cost_usd":0.02,"usage":{"input_tokens":10,"output_tokens":5},"duration_ms":500,"model":"claude-sonnet-4"}`)
}

// fakeClaudeVerdictBad emits an envelope whose result field is plain prose,
// not a JSON object — the verifier-role protocol-error path (AC-03).
func fakeClaudeVerdictBad() {
	recordInvocation()
	writeStdoutLine(`{"result":"looks fine to me","total_cost_usd":0.01}`)
}

// fakeClaudeMinimal emits an envelope with only "result" set — no cost,
// usage, duration, or model — to exercise defensive-parsing fallbacks.
func fakeClaudeMinimal() {
	recordInvocation()
	writeStdoutLine(`{"result":"ok"}`)
}

// fakeClaudeFail exits non-zero — simulates the CLI not being logged in.
func fakeClaudeFail() {
	recordInvocation()
	os.Stderr.WriteString("claude: not logged in\n")
	os.Exit(1)
}

// fakeClaudeHang sleeps far longer than any test timeout — simulates a hung
// subprocess to test timeout classification. A scheduled sleep (rather than
// a bare `select{}`) matters here: this package has no background
// goroutines of its own, so a `select{}` on the sole goroutine is a genuine
// deadlock and the Go runtime kills the process immediately with "fatal
// error: all goroutines are asleep" instead of actually hanging.
func fakeClaudeHang() {
	time.Sleep(24 * time.Hour)
}

// fakeClaudeNotJSON emits output that is not valid JSON at all — the outer
// envelope protocol-error path.
func fakeClaudeNotJSON() {
	recordInvocation()
	writeStdoutLine("not json at all")
}

// fakeClaudeRecordEnv records the child's cwd and the env vars AC-05 cares
// about, then emits a minimal valid envelope so Dispatch still succeeds.
func fakeClaudeRecordEnv() {
	p := os.Getenv("CLI_RECORD_PATH")
	cwd, _ := os.Getwd()
	rec := struct {
		Cwd        string `json:"cwd"`
		GOCACHE    string `json:"gocache"`
		GOMODCACHE string `json:"gomodcache"`
		HOME       string `json:"home"`
	}{
		Cwd:        cwd,
		GOCACHE:    os.Getenv("GOCACHE"),
		GOMODCACHE: os.Getenv("GOMODCACHE"),
		HOME:       os.Getenv("HOME"),
	}
	body, _ := json.Marshal(rec)
	os.WriteFile(p, body, 0644)
	writeStdoutLine(`{"result":"ok"}`)
}

// writeStdoutLine writes s + newline to stdout without pulling in fmt's
// formatting machinery for these trivial fakes.
func writeStdoutLine(s string) {
	os.Stdout.WriteString(s + "\n")
}

// fakeCodexEnvelope emits a full JSONL event stream for the implementer
// path: a thread.started event, one agent_message, and a terminal
// turn.completed usage object (per the documented shape confirmed at
// design review — no model/duration_ms fields anywhere in the stream).
func fakeCodexEnvelope() {
	recordInvocation()
	writeStdoutLine(`{"type":"thread.started","thread_id":"th_test"}`)
	writeStdoutLine(`{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`)
	writeStdoutLine(`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":50,"reasoning_output_tokens":10}}`)
}

// fakeCodexVerdict emits a stream whose final agent_message text is itself
// a JSON object string — the verifier-role happy path (AC-02).
func fakeCodexVerdict() {
	recordInvocation()
	writeStdoutLine(`{"type":"thread.started","thread_id":"th_test"}`)
	writeStdoutLine(`{"type":"item.completed","item":{"type":"agent_message","text":"{\"verdict\":\"PASS\",\"reasons\":[]}"}}`)
	writeStdoutLine(`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0}}`)
}

// fakeCodexVerdictBad emits a stream whose final agent_message is plain
// prose, not a JSON object — the verifier-role protocol-error path (AC-02).
func fakeCodexVerdictBad() {
	recordInvocation()
	writeStdoutLine(`{"type":"item.completed","item":{"type":"agent_message","text":"looks fine to me"}}`)
	writeStdoutLine(`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`)
}

// fakeCodexMinimal emits a stream with only an agent_message — no
// turn.completed at all — to exercise defensive-parsing fallbacks (missing
// usage -> zeros/CostSource=unknown; ModelID/DurationMS fall back).
func fakeCodexMinimal() {
	recordInvocation()
	writeStdoutLine(`{"type":"item.completed","item":{"type":"agent_message","text":"ok"}}`)
}

// fakeCodexFail exits non-zero — simulates the codex CLI not being logged
// in (or any other non-zero exit).
func fakeCodexFail() {
	recordInvocation()
	os.Stderr.WriteString("codex: not logged in\n")
	os.Exit(1)
}

// fakeCodexHang sleeps far longer than any test timeout — see
// fakeClaudeHang's doc comment for why a scheduled sleep is used instead of
// select{}.
func fakeCodexHang() {
	time.Sleep(24 * time.Hour)
}

// fakeCodexNotJSON emits output that is not valid JSON at all — the
// outer-stream protocol-error path (a line that fails to parse).
func fakeCodexNotJSON() {
	recordInvocation()
	writeStdoutLine("not json at all")
}

// fakeCodexRecordEnv records the child's cwd and the env vars AC-04 cares
// about, then emits a minimal valid event stream so Dispatch still
// succeeds.
func fakeCodexRecordEnv() {
	p := os.Getenv("CLI_RECORD_PATH")
	cwd, _ := os.Getwd()
	rec := struct {
		Cwd        string `json:"cwd"`
		GOCACHE    string `json:"gocache"`
		GOMODCACHE string `json:"gomodcache"`
		HOME       string `json:"home"`
	}{
		Cwd:        cwd,
		GOCACHE:    os.Getenv("GOCACHE"),
		GOMODCACHE: os.Getenv("GOMODCACHE"),
		HOME:       os.Getenv("HOME"),
	}
	body, _ := json.Marshal(rec)
	os.WriteFile(p, body, 0644)
	writeStdoutLine(`{"type":"item.completed","item":{"type":"agent_message","text":"ok"}}`)
}

// testBinaryPath returns the path to the running test binary so it can
// re-exec itself as a fake CLI.
func testBinaryPath(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return path
}

func TestSpawn_Success(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "envelope")
	dir := t.TempDir()

	sr := spawn(context.Background(), testBinaryPath(t), nil, dir, 5*time.Second)
	if sr.Err != nil {
		t.Fatalf("spawn() error: %v", sr.Err)
	}
	if !strings.Contains(string(sr.Stdout), `"result":"done"`) {
		t.Errorf("stdout = %q, missing expected result field", sr.Stdout)
	}
}

func TestSpawn_MissingBinary(t *testing.T) {
	sr := spawn(context.Background(), "/nonexistent/claude-binary-xyz", nil, t.TempDir(), 5*time.Second)
	if sr.Err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	if sr.Err.Kind != ErrKindConfig {
		t.Errorf("Kind = %s, want %s", sr.Err.Kind, ErrKindConfig)
	}
	if !strings.Contains(sr.Err.Message, "not found on PATH") {
		t.Errorf("message should mention PATH: %s", sr.Err.Message)
	}
}

func TestSpawn_NonZeroExit(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "fail")
	sr := spawn(context.Background(), testBinaryPath(t), nil, t.TempDir(), 5*time.Second)
	if sr.Err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if sr.Err.Kind != ErrKindAuth {
		t.Errorf("Kind = %s, want %s", sr.Err.Kind, ErrKindAuth)
	}
	if !strings.Contains(sr.Err.Message, "exited with code 1") {
		t.Errorf("message should mention exit code: %s", sr.Err.Message)
	}
}

func TestSpawn_Timeout(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sr := spawn(ctx, testBinaryPath(t), nil, t.TempDir(), 500*time.Millisecond)
	if sr.Err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if sr.Err.Kind != ErrKindTransient {
		t.Errorf("Kind = %s, want %s", sr.Err.Kind, ErrKindTransient)
	}
	if !strings.Contains(sr.Err.Message, "timed out") {
		t.Errorf("message should mention timeout: %s", sr.Err.Message)
	}
}

func TestSpawn_UsesDir(t *testing.T) {
	t.Setenv("GO_TEST_FAKE_CLAUDE", "envelope")
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "invocation.txt")
	t.Setenv("CLI_RECORD_PATH", recordPath)

	sr := spawn(context.Background(), testBinaryPath(t), nil, dir, 5*time.Second)
	if sr.Err != nil {
		t.Fatalf("spawn() error: %v", sr.Err)
	}

	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read invocation record: %v", err)
	}
	recordedCwd := strings.SplitN(string(raw), "\n", 2)[0]

	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	if recordedCwd != resolvedDir {
		t.Errorf("child cwd = %q, want %q", recordedCwd, resolvedDir)
	}
}

func TestHygieneEnv_CachesOutsideAnyDir(t *testing.T) {
	env := hygieneEnv()
	var gocache, gomodcache string
	var sawHome bool
	for _, kv := range env {
		if strings.HasPrefix(kv, "GOCACHE=") {
			gocache = strings.TrimPrefix(kv, "GOCACHE=")
		}
		if strings.HasPrefix(kv, "GOMODCACHE=") {
			gomodcache = strings.TrimPrefix(kv, "GOMODCACHE=")
		}
		if strings.HasPrefix(kv, "HOME=") {
			sawHome = true
		}
	}
	if gocache == "" || gomodcache == "" {
		t.Fatalf("hygieneEnv() missing GOCACHE/GOMODCACHE: %v", env)
	}
	if !strings.HasPrefix(gocache, spawnCacheDir) || !strings.HasPrefix(gomodcache, spawnCacheDir) {
		t.Errorf("GOCACHE/GOMODCACHE not rooted at spawnCacheDir: gocache=%q gomodcache=%q want prefix %q", gocache, gomodcache, spawnCacheDir)
	}
	if !sawHome {
		t.Error("hygieneEnv() dropped HOME — claude-cli credentials live under the real home and must not be overridden")
	}
}
