package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- AC-02: TestCatalogAnnotations — one canned-fixture case per provider
// class, asserting the exact ToolSupport value the design's per-provider
// table specifies (including the absent-field -> Unknown edge cases). ---

func TestCatalogAnnotations(t *testing.T) {
	t.Run("openrouter", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[
				{"id":"m-yes","supported_parameters":["tools","temperature"]},
				{"id":"m-no","supported_parameters":["temperature"]},
				{"id":"m-unknown"}
			]}`)
		}))
		defer srv.Close()
		restore := catalogOpenRouterBaseURL
		catalogOpenRouterBaseURL = srv.URL
		defer func() { catalogOpenRouterBaseURL = restore }()

		got, err := listOpenRouterModels(context.Background(), srv.Client(), ProviderConfig{OpenRouterKey: "k"})
		if err != nil {
			t.Fatalf("listOpenRouterModels: %v", err)
		}
		assertCatalogModels(t, got, map[string]ToolSupport{
			"m-yes": ToolSupportYes, "m-no": ToolSupportNo, "m-unknown": ToolSupportUnknown,
		})
	})

	t.Run("mistral", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[
				{"id":"m-yes","capabilities":{"function_calling":true}},
				{"id":"m-no","capabilities":{"function_calling":false}},
				{"id":"m-unknown"}
			]}`)
		}))
		defer srv.Close()
		restore := catalogMistralBaseURL
		catalogMistralBaseURL = srv.URL
		defer func() { catalogMistralBaseURL = restore }()

		got, err := listMistralModels(context.Background(), srv.Client(), ProviderConfig{MistralKey: "k"})
		if err != nil {
			t.Fatalf("listMistralModels: %v", err)
		}
		assertCatalogModels(t, got, map[string]ToolSupport{
			"m-yes": ToolSupportYes, "m-no": ToolSupportNo, "m-unknown": ToolSupportUnknown,
		})
	})

	t.Run("ollama", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
				io.WriteString(w, `{"models":[{"name":"m-yes"},{"name":"m-no"},{"name":"m-unknown"}]}`)
			case r.Method == http.MethodPost && r.URL.Path == "/api/show":
				body, _ := io.ReadAll(r.Body)
				var req struct {
					Name string `json:"name"`
				}
				json.Unmarshal(body, &req)
				switch req.Name {
				case "m-yes":
					io.WriteString(w, `{"capabilities":["completion","tools"]}`)
				case "m-no":
					io.WriteString(w, `{"capabilities":["completion"]}`)
				default:
					io.WriteString(w, `{}`)
				}
			default:
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		}))
		defer srv.Close()

		got, err := listOllamaModels(context.Background(), srv.Client(), ProviderConfig{OllamaHost: srv.URL})
		if err != nil {
			t.Fatalf("listOllamaModels: %v", err)
		}
		assertCatalogModels(t, got, map[string]ToolSupport{
			"m-yes": ToolSupportYes, "m-no": ToolSupportNo, "m-unknown": ToolSupportUnknown,
		})
	})

	t.Run("google", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1beta/models" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			// A present supportedGenerationMethods field must NOT flip the
			// annotation away from Unknown (D4 — no wire-derivable signal).
			io.WriteString(w, `{"models":[{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent"]}]}`)
		}))
		defer srv.Close()
		restore := catalogGoogleBaseURL
		catalogGoogleBaseURL = srv.URL
		defer func() { catalogGoogleBaseURL = restore }()

		got, err := listGoogleModels(context.Background(), srv.Client(), ProviderConfig{GoogleKey: "k"})
		if err != nil {
			t.Fatalf("listGoogleModels: %v", err)
		}
		if len(got) != 1 || got[0].ID != "gemini-2.5-flash" || got[0].Tools != ToolSupportUnknown {
			t.Fatalf("got %+v, want [{gemini-2.5-flash unknown}] (models/ prefix stripped)", got)
		}
	})

	for _, tc := range []struct {
		name string
		list catalogLister
		base *string
	}{
		{"openai", listOpenAIModels, &catalogOpenAIBaseURL},
		{"groq", listGroqModels, &catalogGroqBaseURL},
		{"anthropic", listAnthropicModels, &catalogAnthropicBaseURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/models" {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"data":[{"id":"m-1"},{"id":"m-2"}]}`)
			}))
			defer srv.Close()
			restore := *tc.base
			*tc.base = srv.URL
			defer func() { *tc.base = restore }()

			cfg := ProviderConfig{OpenAIKey: "k", GroqKey: "k", AnthropicKey: "k"}
			got, err := tc.list(context.Background(), srv.Client(), cfg)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			// Bare ID list, no capability field at all — always Unknown (AC-02).
			assertCatalogModels(t, got, map[string]ToolSupport{
				"m-1": ToolSupportUnknown, "m-2": ToolSupportUnknown,
			})
		})
	}
}

func assertCatalogModels(t *testing.T, got []CatalogModel, want map[string]ToolSupport) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d models, want %d: %+v", len(got), len(want), got)
	}
	for _, m := range got {
		w, ok := want[m.ID]
		if !ok {
			t.Errorf("unexpected model %q in result", m.ID)
			continue
		}
		if m.Tools != w {
			t.Errorf("model %q: Tools = %q, want %q", m.ID, m.Tools, w)
		}
	}
}

// --- AC-03: TestListCatalog_ProviderErrorIsolation ---

func newSuccessOllamaServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			io.WriteString(w, `{"models":[]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestListCatalog_ProviderErrorIsolation proves a failing provider (AC-03)
// never hides or blocks a succeeding one: groq's models/list call fails,
// anthropic's succeeds, and both still appear in the result set.
func TestListCatalog_ProviderErrorIsolation(t *testing.T) {
	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"claude-x"}]}`)
	}))
	defer anthropicSrv.Close()
	groqSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":{"message":"boom"}}`)
	}))
	defer groqSrv.Close()
	ollamaSrv := newSuccessOllamaServer(t)

	restoreAnthropic, restoreGroq := catalogAnthropicBaseURL, catalogGroqBaseURL
	catalogAnthropicBaseURL, catalogGroqBaseURL = anthropicSrv.URL, groqSrv.URL
	defer func() { catalogAnthropicBaseURL, catalogGroqBaseURL = restoreAnthropic, restoreGroq }()

	cfg := ProviderConfig{AnthropicKey: "k", GroqKey: "k", OllamaHost: ollamaSrv.URL}
	results := ListCatalog(context.Background(), cfg, nil, "")

	var anthropicResult, groqResult *CatalogResult
	for i := range results {
		switch results[i].Provider {
		case "anthropic":
			anthropicResult = &results[i]
		case "groq":
			groqResult = &results[i]
		}
	}
	if anthropicResult == nil || anthropicResult.Err != nil {
		t.Fatalf("anthropic result: %+v", anthropicResult)
	}
	if len(anthropicResult.Models) != 1 || anthropicResult.Models[0].ID != "claude-x" {
		t.Fatalf("anthropic models: %+v", anthropicResult.Models)
	}
	if groqResult == nil || groqResult.Err == nil {
		t.Fatalf("groq result: want error, got %+v", groqResult)
	}
	// The provider is carried structurally on the typed error (naming it in
	// the rendered CLI output line is renderModelsOutput's job — see
	// cmd/sworn/models.go "%s/: error: %v" — this asserts the model-layer
	// contract that makes that rendering possible).
	var me *Error
	if !AsError(groqResult.Err, &me) {
		t.Fatalf("groq error should be a *model.Error, got %T: %v", groqResult.Err, groqResult.Err)
	}
	if me.Provider != "groq" {
		t.Errorf("groq error Provider = %q, want %q", me.Provider, "groq")
	}
}

// --- AC-04: TestListCatalog_NoDispatchPaths ---

// TestListCatalog_NoDispatchPaths proves ListCatalog never requests
// anything outside each provider's documented models/list-shaped path — a
// per-provider allowlisted transport recorder fails the test immediately on
// any other request (in particular a completion/chat/generate path).
func TestListCatalog_NoDispatchPaths(t *testing.T) {
	newRecorder := func(t *testing.T, allowedPath string, body []byte) *httptest.Server {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != allowedPath {
				t.Errorf("disallowed path requested: %s %s (AC-04 violation, want only %s)", r.Method, r.URL.Path, allowedPath)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	anthropicSrv := newRecorder(t, "/models", []byte(`{"data":[]}`))
	googleSrv := newRecorder(t, "/v1beta/models", []byte(`{"models":[]}`))
	groqSrv := newRecorder(t, "/models", []byte(`{"data":[]}`))
	mistralSrv := newRecorder(t, "/models", []byte(`{"data":[]}`))
	openaiSrv := newRecorder(t, "/models", []byte(`{"data":[]}`))
	openrouterSrv := newRecorder(t, "/models", []byte(`{"data":[]}`))

	ollamaAllowed := map[string]bool{"/api/tags": true, "/api/show": true}
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ollamaAllowed[r.URL.Path] {
			t.Errorf("disallowed ollama path: %s %s (AC-04 violation)", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			io.WriteString(w, `{"models":[{"name":"m"}]}`)
			return
		}
		io.WriteString(w, `{"capabilities":["tools"]}`)
	}))
	t.Cleanup(ollamaSrv.Close)

	baseVars := []*string{
		&catalogAnthropicBaseURL, &catalogGoogleBaseURL, &catalogGroqBaseURL,
		&catalogMistralBaseURL, &catalogOpenAIBaseURL, &catalogOpenRouterBaseURL,
	}
	restore := make([]string, len(baseVars))
	for i, p := range baseVars {
		restore[i] = *p
	}
	t.Cleanup(func() {
		for i, p := range baseVars {
			*p = restore[i]
		}
	})
	catalogAnthropicBaseURL = anthropicSrv.URL
	catalogGoogleBaseURL = googleSrv.URL
	catalogGroqBaseURL = groqSrv.URL
	catalogMistralBaseURL = mistralSrv.URL
	catalogOpenAIBaseURL = openaiSrv.URL
	catalogOpenRouterBaseURL = openrouterSrv.URL

	cfg := ProviderConfig{
		AnthropicKey:  "k",
		GoogleKey:     "k",
		GroqKey:       "k",
		MistralKey:    "k",
		OpenAIKey:     "k",
		OpenRouterKey: "k",
		OllamaHost:    ollamaSrv.URL,
	}
	results := ListCatalog(context.Background(), cfg, nil, "")
	if len(results) != 7 {
		t.Fatalf("got %d results, want 7 (all 6 configured + ollama always attempted): %+v", len(results), results)
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("provider %s: unexpected error: %v", r.Provider, r.Err)
		}
	}
}

// --- D3: TestListCatalog_OllamaAlwaysAttempted ---

// TestListCatalog_OllamaAlwaysAttempted proves an empty ProviderConfig (no
// credentials configured for any provider) still yields exactly one
// CatalogResult — Ollama, always attempted per D3 — while the other 6 are
// entirely absent from the result set (not attempted, not errored).
//
// OllamaHost is pointed at an explicit closed local port rather than left at
// its env-derived default: this dev/CI host may have a real local Ollama
// daemon running (observed during implementation), which would make the
// call succeed instead of fail and turn the assertion below into an
// environment-dependent flake. The behaviour under test — Ollama is
// attempted even with zero configured credentials — is unaffected by which
// host the call is aimed at; only the deterministic-failure means changed
// from design.md's "empty ProviderConfig" (implicitly, the env-default
// host) to an explicitly unreachable one.
func TestListCatalog_OllamaAlwaysAttempted(t *testing.T) {
	cfg := ProviderConfig{OllamaHost: "http://127.0.0.1:1"}
	results := ListCatalog(context.Background(), cfg, nil, "")
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (ollama only): %+v", len(results), results)
	}
	if results[0].Provider != "ollama" {
		t.Fatalf("got provider %q, want ollama", results[0].Provider)
	}
	if results[0].Err == nil {
		t.Fatalf("want a dial error against the closed port, got nil / models=%+v", results[0].Models)
	}
}

// CatalogProviderNames must expose all 7 providers in the fixed
// alphabetical order the design decision (D1) fixes.
func TestCatalogProviderNames(t *testing.T) {
	want := []string{"anthropic", "google", "groq", "mistral", "ollama", "openai", "openrouter"}
	got := CatalogProviderNames()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
