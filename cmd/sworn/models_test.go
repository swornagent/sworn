package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/command"
)

// TestModelsCommandRegistered proves the verb is reachable through the
// integration point that owns it — the process-wide command registry that
// main.dispatch resolves from (Rule 1).
func TestModelsCommandRegistered(t *testing.T) {
	c, ok := command.Lookup("models")
	if !ok {
		t.Fatal(`command.Lookup("models") not found — init() in cmd/sworn/models.go did not register`)
	}
	if c.Summary == "" {
		t.Error("Summary must be non-empty")
	}
	if c.Run == nil {
		t.Fatal("Run must be non-nil")
	}
}

// clearModelsProviderEnv blanks every env var ProviderConfigFromEnv reads
// for this slice's 7 target providers so each test's configured-provider
// set is exactly what it sets explicitly.
func clearModelsProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OPENAI_API_KEY", "SWORN_OPENAI_API_KEY",
		"GROQ_API_KEY", "SWORN_GROQ_API_KEY",
		"MISTRAL_API_KEY", "SWORN_MISTRAL_API_KEY",
		"OPENROUTER_API_KEY", "SWORN_OPENROUTER_API_KEY",
		"ANTHROPIC_API_KEY", "SWORN_ANTHROPIC_API_KEY",
		"GOOGLE_API_KEY", "SWORN_GOOGLE_API_KEY",
		"OLLAMA_HOST",
	} {
		t.Setenv(k, "")
	}
}

// knownProviderHosts are the real production hosts internal/model/catalog.go
// dispatches to by default. hostRoutingTransport only redirects requests
// aimed at one of these — any other host (in particular Ollama's fixture,
// pointed at directly via OLLAMA_HOST) passes through unmodified.
var knownProviderHosts = map[string]bool{
	"api.anthropic.com":                 true,
	"generativelanguage.googleapis.com": true,
	"api.groq.com":                      true,
	"api.mistral.ai":                    true,
	"api.openai.com":                    true,
	"openrouter.ai":                     true,
}

// hostRoutingTransport rewrites requests aimed at a known real provider host
// to target, tagging the original host in X-Test-Origin-Host so one fixture
// server can dispatch per-provider canned responses. Test-only seam — it
// lets TestModelsCommand exercise the real cmdModels -> model.ListCatalog
// path (Rule 1) against local fixture servers instead of a live network
// call, without internal/model exporting its unexported base-URL vars.
type hostRoutingTransport struct {
	target *url.URL
}

func (h *hostRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !knownProviderHosts[req.URL.Host] {
		return http.DefaultTransport.RoundTrip(req)
	}
	origin := req.URL.Host
	out := req.Clone(req.Context())
	out.URL.Scheme = h.target.Scheme
	out.URL.Host = h.target.Host
	out.Host = h.target.Host
	out.Header.Set("X-Test-Origin-Host", origin)
	return http.DefaultTransport.RoundTrip(out)
}

type sixProviderFixtureOpts struct {
	groqFails bool
}

// newSixProviderFixture serves canned models/list responses for the 6
// HTTP-based providers (Ollama is handled separately — see
// newOllamaFixture), keyed by the X-Test-Origin-Host hostRoutingTransport
// tags each request with.
func newSixProviderFixture(t *testing.T, opts sixProviderFixtureOpts) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("X-Test-Origin-Host")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case origin == "api.anthropic.com" && r.URL.Path == "/v1/models":
			io.WriteString(w, `{"data":[{"id":"claude-x"}]}`)
		case origin == "generativelanguage.googleapis.com" && r.URL.Path == "/v1beta/models":
			io.WriteString(w, `{"models":[{"name":"models/gemini-x"}]}`)
		case origin == "api.groq.com" && r.URL.Path == "/openai/v1/models":
			if opts.groqFails {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, `{"error":{"message":"bad key"}}`)
				return
			}
			io.WriteString(w, `{"data":[{"id":"groq-x"}]}`)
		case origin == "api.mistral.ai" && r.URL.Path == "/v1/models":
			io.WriteString(w, `{"data":[{"id":"mistral-x","capabilities":{"function_calling":true}}]}`)
		case origin == "api.openai.com" && r.URL.Path == "/v1/models":
			io.WriteString(w, `{"data":[{"id":"gpt-x"}]}`)
		case origin == "openrouter.ai" && r.URL.Path == "/api/v1/models":
			io.WriteString(w, `{"data":[{"id":"or-x","supported_parameters":["tools"]}]}`)
		default:
			t.Errorf("unexpected fixture request: origin=%q path=%q (AC-04 violation or unwired provider)", origin, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newOllamaFixture serves /api/tags with the given model names and /api/show
// with an empty (no-capabilities) body for each.
func newOllamaFixture(t *testing.T, models []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			var sb strings.Builder
			sb.WriteString(`{"models":[`)
			for i, m := range models {
				if i > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(fmt.Sprintf(`{"name":%q}`, m))
			}
			sb.WriteString(`]}`)
			io.WriteString(w, sb.String())
		case "/api/show":
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected ollama fixture path %s (AC-04 violation)", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setModelsHTTPClient points modelsHTTPClient at a hostRoutingTransport
// targeting fixture, restoring nil (production behaviour) via t.Cleanup.
func setModelsHTTPClient(t *testing.T, fixture *httptest.Server) {
	t.Helper()
	target, err := url.Parse(fixture.URL)
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	modelsHTTPClient = &http.Client{Transport: &hostRoutingTransport{target: target}}
	t.Cleanup(func() { modelsHTTPClient = nil })
}

// runModelsCapture runs the registered "models" command's Run function,
// capturing stdout.
func runModelsCapture(t *testing.T, c command.Command, args []string) (string, int) {
	t.Helper()
	saved := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	code := c.Run(args)
	w.Close()
	os.Stdout = saved
	outBytes, _ := io.ReadAll(r)
	return string(outBytes), code
}

// TestModelsCommand runs `sworn models` end-to-end through the registered
// cmdModels entrypoint (Rule 1 — the integration point that owns the
// affordance, not a leaf catalog.go unit test) against fixture servers for
// all 7 providers, asserting the grouped-by-prefix output shape (AC-01) and
// the wire-sourced capability annotations (AC-02) survive the full path.
// This is the reachability artefact named in proof.json.
func TestModelsCommand(t *testing.T) {
	clearModelsProviderEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "k")
	t.Setenv("GOOGLE_API_KEY", "k")
	t.Setenv("GROQ_API_KEY", "k")
	t.Setenv("MISTRAL_API_KEY", "k")
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("OPENROUTER_API_KEY", "k")

	sixFixture := newSixProviderFixture(t, sixProviderFixtureOpts{})
	ollamaFixture := newOllamaFixture(t, []string{"llama-x"})
	t.Setenv("OLLAMA_HOST", ollamaFixture.URL)
	setModelsHTTPClient(t, sixFixture)

	c, ok := command.Lookup("models")
	if !ok {
		t.Fatal("models verb not registered")
	}

	out, code := runModelsCapture(t, c, nil)
	if code != 0 {
		t.Fatalf("exit %d, want 0\noutput:\n%s", code, out)
	}
	for _, want := range []string{
		"anthropic/ (1 models)",
		"  anthropic/claude-x   tools: unknown",
		"google/ (1 models)",
		"  google/gemini-x   tools: unknown",
		"groq/ (1 models)",
		"  groq/groq-x   tools: unknown",
		"mistral/ (1 models)",
		"  mistral/mistral-x   tools: yes",
		"ollama/ (1 models)",
		"  ollama/llama-x   tools: unknown",
		"openai/ (1 models)",
		"  openai/gpt-x   tools: unknown",
		"openrouter/ (1 models)",
		"  openrouter/or-x   tools: yes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\noutput:\n%s", want, out)
		}
	}
}

// TestModelsCommand_ProviderFilter proves --provider restricts output to
// exactly one provider (AC-01), attempted regardless of the other 6
// providers' configured state (none of their env vars are set here).
func TestModelsCommand_ProviderFilter(t *testing.T) {
	clearModelsProviderEnv(t)
	t.Setenv("MISTRAL_API_KEY", "k")

	sixFixture := newSixProviderFixture(t, sixProviderFixtureOpts{})
	setModelsHTTPClient(t, sixFixture)

	c, ok := command.Lookup("models")
	if !ok {
		t.Fatal("models verb not registered")
	}
	out, code := runModelsCapture(t, c, []string{"--provider", "mistral"})
	if code != 0 {
		t.Fatalf("exit %d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "mistral/ (1 models)") {
		t.Errorf("output missing mistral block\noutput:\n%s", out)
	}
	for _, unwanted := range []string{"anthropic/", "openai/", "groq/", "openrouter/", "google/", "ollama/"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output should contain only the mistral provider, found %q\noutput:\n%s", unwanted, out)
		}
	}
}

// TestModelsCommand_AllFailedExitsNonZero proves AC-03's exit-code rule:
// with no provider keys configured, Ollama is still the sole attempted
// provider (D3) — pointed at a closed local port for a guaranteed,
// deterministic dial failure — so the command exits 1.
func TestModelsCommand_AllFailedExitsNonZero(t *testing.T) {
	clearModelsProviderEnv(t)
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")

	c, ok := command.Lookup("models")
	if !ok {
		t.Fatal("models verb not registered")
	}
	out, code := runModelsCapture(t, c, nil)
	if code != 1 {
		t.Fatalf("exit %d, want 1 (every attempted provider failed)\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "ollama/: error:") {
		t.Errorf("output missing ollama error line\noutput:\n%s", out)
	}
}

// TestModelsCommand_PartialFailureExitsZero proves AC-03's isolation half:
// groq and ollama fail, anthropic succeeds — since not every attempted
// provider failed, exit stays 0 and all three still appear in the output.
func TestModelsCommand_PartialFailureExitsZero(t *testing.T) {
	clearModelsProviderEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "k")
	t.Setenv("GROQ_API_KEY", "k")
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")

	sixFixture := newSixProviderFixture(t, sixProviderFixtureOpts{groqFails: true})
	setModelsHTTPClient(t, sixFixture)

	c, ok := command.Lookup("models")
	if !ok {
		t.Fatal("models verb not registered")
	}
	out, code := runModelsCapture(t, c, nil)
	if code != 0 {
		t.Fatalf("exit %d, want 0 (anthropic succeeded — not every attempted provider failed)\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "anthropic/ (1 models)") {
		t.Errorf("output missing anthropic success block\noutput:\n%s", out)
	}
	if !strings.Contains(out, "groq/: error:") {
		t.Errorf("output missing groq error line\noutput:\n%s", out)
	}
	if !strings.Contains(out, "ollama/: error:") {
		t.Errorf("output missing ollama error line\noutput:\n%s", out)
	}
}

// TestModelsCommand_UnknownProviderFlag proves an unsupported --provider
// value is a usage error (exit 64) rejected before any HTTP call — no
// fixture server or modelsHTTPClient override is set up for this test, so a
// stray dispatch would surface as a real (and here, unwanted) network call.
func TestModelsCommand_UnknownProviderFlag(t *testing.T) {
	clearModelsProviderEnv(t)
	c, ok := command.Lookup("models")
	if !ok {
		t.Fatal("models verb not registered")
	}

	savedErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	out, code := runModelsCapture(t, c, []string{"--provider", "bogus"})
	w.Close()
	os.Stderr = savedErr
	stderrBytes, _ := io.ReadAll(r)

	if code != 64 {
		t.Fatalf("exit %d, want 64\nstdout:\n%sstderr:\n%s", code, out, string(stderrBytes))
	}
	if out != "" {
		t.Errorf("stdout should be empty on a usage error, got %q", out)
	}
	if !strings.Contains(string(stderrBytes), `unknown provider "bogus"`) {
		t.Errorf("stderr should name the bad provider, got %q", string(stderrBytes))
	}
}
