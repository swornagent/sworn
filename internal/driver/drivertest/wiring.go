package drivertest

// Fixture wiring for the compiled-in drivers (AC-02). This file lives in
// drivertest — not in internal/driver's conformance_all_test.go — because
// TestNoWireImports forbids every .go file in internal/driver (test files
// included) from importing internal/model, and the in-process constructors
// take a model.ProviderConfig. The enrolment test consumes these helpers and
// stays wire-import-free.
//
// The in-process drivers are wired through the PROXY route — fake sworn
// credentials + SWORN_PROXY_URL pointing at an httptest server (Coach
// disposition 3: the landed S06 ProxyRoute predicate is the honest,
// already-tested seam; model.ProviderConfig carries no base-URL field and
// InProcess.newClient is unexported, so no production seam is added).
// t.Setenv auto-restores every mutation (Rule 11 scoped-mutation guard).

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/driver/inprocess"
	"github.com/swornagent/sworn/internal/driver/registry"
	"github.com/swornagent/sworn/internal/model"
)

// Enrolment pairs a fresh-driver factory with the Options its fake wiring
// serves. conformance_all_test.go looks every registered driver name up in a
// test-owned name→Enrolment map and fails closed on a missing entry (Coach
// disposition 4: Registry.Drivers() returns []Info, not instances, so
// enrolment is fail-closed detection, not zero-edit auto-derivation).
type Enrolment struct {
	NewDriver func() driver.Driver
	Options   Options
}

// RegisteredDriverNames enumerates the compiled-in registry's driver names
// without dispatching (Registry.Drivers is dispatch-free by construction).
func RegisteredDriverNames() []string {
	infos := registry.Default(model.ProviderConfig{}).Drivers()
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
	}
	return names
}

// conformanceResultJSON is the scripted result text every fake transport
// returns: a JSON object, so the same fixture serves both the implementer
// clause (non-empty ResultText) and the verifier clause (StructuredJSON
// parses as a JSON object).
const conformanceResultJSON = `{"verdict":"PASS","rationale":"conformance"}`

// FakeClaudeEnrolment wires ClaudeDriver to a fake `claude` CLI: a shell
// script printing a well-formed result envelope (happy) or exiting non-zero
// (failing). The script appends to a counter file so WorkCount can prove the
// Rule-11 guard fires before any spawn.
func FakeClaudeEnrolment(t *testing.T) Enrolment {
	t.Helper()
	dir := t.TempDir()
	counter := filepath.Join(dir, "spawns.log")
	envelope := `{"result":"{\"verdict\":\"PASS\",\"rationale\":\"conformance\"}","total_cost_usd":0.01,"usage":{"input_tokens":3,"output_tokens":2},"duration_ms":5,"model":"claude-conformance"}`
	happy := writeFakeBinary(t, dir, "claude-ok", counter, "printf '%s' '"+envelope+"'\n")
	failing := writeFakeBinary(t, dir, "claude-fail", counter, "echo 'scripted CLI failure' >&2\nexit 3\n")

	return Enrolment{
		NewDriver: func() driver.Driver { return &driver.ClaudeDriver{Binary: happy} },
		Options: Options{
			ModelID:    "claude-cli/conformance",
			NewFailing: func() driver.Driver { return &driver.ClaudeDriver{Binary: failing} },
			WorkCount:  lineCount(counter),
		},
	}
}

// FakeCodexEnrolment wires CodexDriver to a fake `codex` CLI emitting the
// documented JSONL event stream (happy) or exiting non-zero (failing).
func FakeCodexEnrolment(t *testing.T) Enrolment {
	t.Helper()
	dir := t.TempDir()
	counter := filepath.Join(dir, "spawns.log")
	stream := `{"type":"thread.started"}
{"type":"item.completed","item":{"type":"agent_message","text":"{\"verdict\":\"PASS\",\"rationale\":\"conformance\"}"}}
{"type":"turn.completed","usage":{"input_tokens":3,"cached_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":0}}`
	happy := writeFakeBinary(t, dir, "codex-ok", counter, "cat <<'EOF'\n"+stream+"\nEOF\n")
	failing := writeFakeBinary(t, dir, "codex-fail", counter, "echo 'scripted CLI failure' >&2\nexit 3\n")

	return Enrolment{
		NewDriver: func() driver.Driver { return &driver.CodexDriver{Binary: happy} },
		Options: Options{
			ModelID:    "codex/conformance",
			NewFailing: func() driver.Driver { return &driver.CodexDriver{Binary: failing} },
			WorkCount:  lineCount(counter),
		},
	}
}

// ProxyEnrolments wires both in-process identities (oai-inprocess and
// oai-responses-inprocess) to one httptest server through the S06 ProxyRoute
// seam: fake sworn credentials under a scratch XDG_CONFIG_HOME plus
// SWORN_PROXY_URL (t.Setenv — auto-restored). A model ID containing
// "conformance-fail" routes to a scripted 500, driving the error-path clause
// with zero production edits.
func ProxyEnrolments(t *testing.T) (chat, responses Enrolment) {
	t.Helper()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "conformance-fail") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"message":"scripted upstream failure"}}`)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			fmt.Fprint(w, `{"model":"conformance-model","choices":[{"message":{"content":"{\"verdict\":\"PASS\",\"rationale\":\"conformance\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
		case strings.HasSuffix(r.URL.Path, "/responses"):
			fmt.Fprint(w, `{"id":"resp-conformance","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"verdict\":\"PASS\",\"rationale\":\"conformance\"}"}]}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"message":"unexpected path"}}`)
		}
	}))
	t.Cleanup(srv.Close)

	// Fake sworn login: ProxyRoute requires unexpired credentials at
	// os.UserConfigDir()/sworn/credentials.json. XDG_CONFIG_HOME scopes the
	// read to a scratch dir on Linux; t.Setenv restores after the test.
	cfgHome := t.TempDir()
	swornDir := filepath.Join(cfgHome, "sworn")
	if err := os.MkdirAll(swornDir, 0o755); err != nil {
		t.Fatal(err)
	}
	creds := `{"token":"conformance-token","email":"conformance@example.invalid","tier":"test","expires_at":"2099-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(swornDir, "credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("SWORN_PROXY_URL", srv.URL)
	t.Setenv("SWORN_DIRECT", "")

	workCount := func() int { return int(hits.Load()) }

	chat = Enrolment{
		NewDriver: func() driver.Driver { return inprocess.NewOAIChat(model.ProviderConfig{}) },
		Options: Options{
			ModelID:        "deepseek/conformance",
			FailingModelID: "deepseek/conformance-fail",
			WorkCount:      workCount,
		},
	}
	responses = Enrolment{
		NewDriver: func() driver.Driver { return inprocess.NewOAIResponses(model.ProviderConfig{}) },
		Options: Options{
			ModelID:        "openai/conformance",
			FailingModelID: "openai/conformance-fail",
			WorkCount:      workCount,
		},
	}
	return chat, responses
}

// StubEnrolment enrols the transport-less reference StubDriver as the fifth
// conformance subject (design D4): the failure mode is a scripted error
// handler — the stub's injectable error hook.
func StubEnrolment(t *testing.T) Enrolment {
	t.Helper()
	return Enrolment{
		NewDriver: func() driver.Driver { return NewStub() },
		Options: Options{
			ModelID: "conformance-reference/scripted",
			NewFailing: func() driver.Driver {
				return &StubDriver{Handlers: map[driver.Role]func(driver.DispatchInput) (driver.Result, error){
					driver.RoleImplementer: func(driver.DispatchInput) (driver.Result, error) {
						return driver.Result{Status: driver.StatusError, ErrKind: "scripted"},
							fmt.Errorf("scripted reference failure")
					},
				}}
			},
		},
	}
}

// writeFakeBinary writes an executable shell script that first appends one
// line to counter, then runs body.
func writeFakeBinary(t *testing.T, dir, name, counter, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\necho spawn >> '" + counter + "'\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary %s: %v", name, err)
	}
	return path
}

// lineCount returns a WorkCount probe over an append-per-spawn counter file.
func lineCount(path string) func() int {
	return func() int {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0
		}
		trimmed := strings.TrimRight(string(data), "\n")
		if trimmed == "" {
			return 0
		}
		return len(strings.Split(trimmed, "\n"))
	}
}
