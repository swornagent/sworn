// catalog.go — S09-model-catalog: per-provider models/list clients feeding
// `sworn models`. No completion/dispatch/probe calls are made anywhere in
// this file (AC-04) — every request targets a provider's models/list-shaped
// endpoint only. Capability annotation is sourced exclusively from
// wire-reported metadata (AC-02): a field the provider's own list response
// carries, never a heuristic derived from a model ID string. "unknown" is
// the fail-closed default whenever a provider's wire shape carries no
// explicit tools signal — callers must never coerce it to yes or no.
//
// D1 (design.md): availability ("configured") is determined by this file's
// own no-dispatch credential-presence check against ProviderConfig, for all
// 7 target providers uniformly — not by a call into
// internal/driver/registry.Drivers(), which by its own documented design
// only enumerates 4 of the 7 (Google and Ollama are structurally absent).
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ToolSupport is a fail-closed tri-state capability annotation. Unknown is
// never treated as capable (AC-02) — callers must not coerce it to false or
// true.
type ToolSupport string

const (
	ToolSupportYes     ToolSupport = "yes"
	ToolSupportNo      ToolSupport = "no"
	ToolSupportUnknown ToolSupport = "unknown"
)

// CatalogModel is one model entry from a provider's models/list endpoint,
// normalised across heterogeneous wire shapes. ID is the bare model name as
// the provider reports it (no resolution prefix — the caller prepends it).
type CatalogModel struct {
	ID    string
	Tools ToolSupport
}

// CatalogResult is one provider's outcome from ListCatalog: either a model
// list or a per-provider error (AC-03 — a provider failure never blocks the
// others).
type CatalogResult struct {
	Provider string // canonical resolution prefix, e.g. "openrouter"
	Models   []CatalogModel
	Err      error
}

// Default base URLs for each provider's models/list endpoint. These are
// package-level variables (not constants) so tests can redirect requests to
// an httptest fixture server without changing ListCatalog's exported
// signature — restored via t.Cleanup in each test that overrides one.
var (
	catalogAnthropicBaseURL  = "https://api.anthropic.com/v1"
	catalogGoogleBaseURL     = "https://generativelanguage.googleapis.com"
	catalogGroqBaseURL       = "https://api.groq.com/openai/v1"
	catalogMistralBaseURL    = "https://api.mistral.ai/v1"
	catalogOpenAIBaseURL     = "https://api.openai.com/v1"
	catalogOpenRouterBaseURL = "https://openrouter.ai/api/v1"
)

// catalogLister fetches and normalises one provider's model list.
type catalogLister func(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error)

// catalogProviderDef binds a provider's canonical resolution prefix to its
// no-dispatch availability check and its models/list client.
type catalogProviderDef struct {
	name       string
	configured func(cfg ProviderConfig) bool
	list       catalogLister
}

// catalogProviderDefs is the fixed alphabetical iteration order — diff-stable
// output, mirroring capabilities.go's sort.Slice discipline. D3: Ollama's
// configured func always returns true (keyless local daemon, mirrors the
// driver registry's own claude-cli treatment — binary/daemon presence, not
// key presence, gates availability; a daemon-down failure surfaces as a
// normal AC-03 per-provider error, not a silent skip).
var catalogProviderDefs = []catalogProviderDef{
	{"anthropic", func(cfg ProviderConfig) bool { return cfg.AnthropicKey != "" }, listAnthropicModels},
	{"google", func(cfg ProviderConfig) bool { return cfg.GoogleKey != "" }, listGoogleModels},
	{"groq", func(cfg ProviderConfig) bool { return cfg.GroqKey != "" }, listGroqModels},
	{"mistral", func(cfg ProviderConfig) bool { return cfg.MistralKey != "" }, listMistralModels},
	{"ollama", func(cfg ProviderConfig) bool { return true }, listOllamaModels},
	{"openai", func(cfg ProviderConfig) bool { return cfg.OpenAIKey != "" }, listOpenAIModels},
	{"openrouter", func(cfg ProviderConfig) bool { return cfg.OpenRouterKey != "" }, listOpenRouterModels},
	{"xai", func(cfg ProviderConfig) bool { return cfg.XAIKey != "" }, listXAIModels},
}

// CatalogProviderNames returns the ordered list of provider prefixes
// ListCatalog understands. Callers (the `sworn models --provider` flag
// validator) use this to reject an unsupported prefix before any HTTP call.
func CatalogProviderNames() []string {
	names := make([]string, len(catalogProviderDefs))
	for i, d := range catalogProviderDefs {
		names[i] = d.name
	}
	return names
}

// ListCatalog queries the models/list endpoint of every provider in cfg
// that has credentials configured (Ollama always attempted — D3), plus an
// optional single-provider filter. No completion/dispatch/probe calls are
// made (AC-04). client defaults to http.DefaultClient when nil.
//
// filter, when non-empty, restricts the listing to one provider regardless
// of its configured state (an explicit --provider request is attempted even
// without credentials — the resulting auth failure is a normal per-provider
// AC-03 error, not a silent skip). filter is expected to already be
// validated against CatalogProviderNames by the caller; an unrecognised
// filter simply yields zero results.
func ListCatalog(ctx context.Context, cfg ProviderConfig, client *http.Client, filter string) []CatalogResult {
	if client == nil {
		client = http.DefaultClient
	}

	var results []CatalogResult
	for _, def := range catalogProviderDefs {
		if filter != "" {
			if def.name != filter {
				continue
			}
		} else if !def.configured(cfg) {
			continue
		}
		models, err := def.list(ctx, client, cfg)
		results = append(results, CatalogResult{Provider: def.name, Models: models, Err: err})
	}
	return results
}

// catalogDoGet dispatches req and returns the response body on 2xx. On a
// non-2xx status it returns a *model.Error (NewProviderError) carrying the
// classified ErrorKind; on a transport failure it returns a wrapped error.
// Shared by every provider's list client (R-01: defensive parsing — a
// failure here degrades that one provider to an AC-03 error, never panics,
// never fails the whole command).
func catalogDoGet(client *http.Client, req *http.Request, provider string) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: dispatch: %w", provider, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", provider, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewProviderError(resp.StatusCode, provider, "", body)
	}
	return body, nil
}

// --- Anthropic (bare ID list — always Unknown, AC-02) ---

type catalogBareIDResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func listAnthropicModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	url := strings.TrimRight(catalogAnthropicBaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("x-api-key", cfg.AnthropicKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	body, err := catalogDoGet(client, req, "anthropic")
	if err != nil {
		return nil, err
	}
	return parseCatalogBareIDList(body, "anthropic")
}

// --- OpenAI (bare ID list — always Unknown, AC-02) ---

func listOpenAIModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	url := strings.TrimRight(catalogOpenAIBaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAIKey)

	body, err := catalogDoGet(client, req, "openai")
	if err != nil {
		return nil, err
	}
	return parseCatalogBareIDList(body, "openai")
}

// --- Groq (bare ID list — always Unknown, AC-02) ---

func listGroqModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	url := strings.TrimRight(catalogGroqBaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("groq: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.GroqKey)

	body, err := catalogDoGet(client, req, "groq")
	if err != nil {
		return nil, err
	}
	return parseCatalogBareIDList(body, "groq")
}

func parseCatalogBareIDList(body []byte, provider string) ([]CatalogModel, error) {
	var parsed catalogBareIDResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%s: unmarshal response: %w", provider, err)
	}
	models := make([]CatalogModel, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		models = append(models, CatalogModel{ID: m.ID, Tools: ToolSupportUnknown})
	}
	return models, nil
}

// --- OpenRouter (supported_parameters wire signal) ---

type openrouterModelsResponse struct {
	Data []struct {
		ID                  string   `json:"id"`
		SupportedParameters []string `json:"supported_parameters"`
	} `json:"data"`
}

func listOpenRouterModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	url := strings.TrimRight(catalogOpenRouterBaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.OpenRouterKey)

	body, err := catalogDoGet(client, req, "openrouter")
	if err != nil {
		return nil, err
	}
	var parsed openrouterModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("openrouter: unmarshal response: %w", err)
	}
	models := make([]CatalogModel, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		models = append(models, CatalogModel{ID: m.ID, Tools: annotateStringListTools(m.SupportedParameters)})
	}
	return models, nil
}

// --- Mistral (capabilities.function_calling wire signal) ---

type mistralCapabilities struct {
	FunctionCalling bool `json:"function_calling"`
}

type mistralModelsResponse struct {
	Data []struct {
		ID           string               `json:"id"`
		Capabilities *mistralCapabilities `json:"capabilities"`
	} `json:"data"`
}

func listMistralModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	url := strings.TrimRight(catalogMistralBaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("mistral: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.MistralKey)

	body, err := catalogDoGet(client, req, "mistral")
	if err != nil {
		return nil, err
	}
	var parsed mistralModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("mistral: unmarshal response: %w", err)
	}
	models := make([]CatalogModel, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		models = append(models, CatalogModel{ID: m.ID, Tools: annotateMistralTools(m.Capabilities)})
	}
	return models, nil
}

func annotateMistralTools(caps *mistralCapabilities) ToolSupport {
	if caps == nil {
		return ToolSupportUnknown
	}
	if caps.FunctionCalling {
		return ToolSupportYes
	}
	return ToolSupportNo
}

// --- Google (supportedGenerationMethods carries no explicit tool-support
// signal — D4: always Unknown, never derived from the field's contents) ---

type googleModelsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func listGoogleModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	url := strings.TrimRight(catalogGoogleBaseURL, "/") + "/v1beta/models?key=" + cfg.GoogleKey
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("google: build request: %w", err)
	}

	body, err := catalogDoGet(client, req, "google")
	if err != nil {
		return nil, err
	}
	var parsed googleModelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("google: unmarshal response: %w", err)
	}
	models := make([]CatalogModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		// D4: unconditionally Unknown — no wire-derivable Yes/No exists in
		// this field (spec.json rationale: "partial — tool support not
		// explicit"). Strip the "models/" resource-name prefix Google's wire
		// format carries so the bare ID matches what the caller resolves
		// (e.g. "gemini-2.5-flash", not "models/gemini-2.5-flash").
		id := strings.TrimPrefix(m.Name, "models/")
		models = append(models, CatalogModel{ID: id, Tools: ToolSupportUnknown})
	}
	return models, nil
}

// --- Ollama (D2: /api/tags for names, then /api/show per model for the
// capabilities wire signal — N+1 against the local daemon) ---

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type ollamaShowResponse struct {
	Capabilities []string `json:"capabilities"`
}

func listOllamaModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]CatalogModel, error) {
	host := cfg.OllamaHost
	if host == "" {
		host = ollamaHost()
	}
	host = strings.TrimRight(host, "/")

	tagsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	tagsBody, err := catalogDoGet(client, tagsReq, "ollama")
	if err != nil {
		return nil, err
	}
	var tags ollamaTagsResponse
	if err := json.Unmarshal(tagsBody, &tags); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal /api/tags response: %w", err)
	}

	models := make([]CatalogModel, 0, len(tags.Models))
	for _, m := range tags.Models {
		tools := ollamaModelTools(ctx, client, host, m.Name)
		models = append(models, CatalogModel{ID: m.Name, Tools: tools})
	}
	return models, nil
}

// ollamaModelTools makes the per-model /api/show call (D2). A failure at
// this per-model step (network error, non-2xx, unparseable body) degrades
// that one model's annotation to Unknown (R-01) rather than failing the
// whole Ollama listing — the model already came back from /api/tags and
// stays in the result set.
func ollamaModelTools(ctx context.Context, client *http.Client, host, modelName string) ToolSupport {
	reqBody, err := json.Marshal(struct {
		Name string `json:"name"`
	}{Name: modelName})
	if err != nil {
		return ToolSupportUnknown
	}
	showReq, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/api/show", bytes.NewReader(reqBody))
	if err != nil {
		return ToolSupportUnknown
	}
	showReq.Header.Set("Content-Type", "application/json")

	body, err := catalogDoGet(client, showReq, "ollama")
	if err != nil {
		return ToolSupportUnknown
	}
	var show ollamaShowResponse
	if err := json.Unmarshal(body, &show); err != nil {
		return ToolSupportUnknown
	}
	return annotateStringListTools(show.Capabilities)
}

// annotateStringListTools implements the shared "field present & contains
// 'tools' -> Yes; field present & absent 'tools' -> No; field missing ->
// Unknown" rule (OpenRouter's supported_parameters, Ollama's capabilities).
// encoding/json leaves a []string field nil when the JSON key is entirely
// absent, and non-nil (possibly zero-length) when the key is present as an
// array — that distinction is exactly the "missing" vs "present" signal
// this rule needs.
func annotateStringListTools(fields []string) ToolSupport {
	if fields == nil {
		return ToolSupportUnknown
	}
	for _, f := range fields {
		if f == "tools" {
			return ToolSupportYes
		}
	}
	return ToolSupportNo
}
