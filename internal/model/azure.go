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

// AzureOAI dispatches verification calls to an Azure OpenAI deployment using
// the /chat/completions endpoint with api-key auth (not Bearer). It implements
// Verifier via stdlib net/http + encoding/json — zero third-party deps.
//
// Azure OpenAI uses the same /chat/completions request body as OpenAI; it
// differs in URL structure and auth header:
//   - URL: https://<endpoint>/openai/deployments/<deployment>/chat/completions?api-version=<version>
//   - Auth: api-key: <key>  (not Authorization: Bearer <key>)
//
// This is a standalone struct — it does not embed *OAI because Azure replaces
// the URL construction and auth header entirely. Embedding would create a
// misleading type relationship (BaseURL and Authorization are meaningless for
// Azure).
type AzureOAI struct {
	Endpoint   string // e.g. myendpoint.openai.azure.com
	Deployment string // e.g. gpt-4o
	APIKey     string
	APIVersion string       // e.g. 2024-10-21
	Client     *http.Client // nil means http.DefaultClient
}

// NewAzureOAI constructs an AzureOAI driver. deployment and endpoint must be
// non-empty (endpoint is the Azure OpenAI host, without scheme or path).
// apiKey must be non-empty. apiVersion defaults to "2024-12-01-preview" if
// empty — this is the preview version specified in the acceptance checks.
// The api-version is overridable via AZURE_OPENAI_API_VERSION.
//
// Endpoint normalisation: trailing slashes are stripped and https:// is
// prepended only when no scheme is present.
func NewAzureOAI(deployment, endpoint, apiKey, apiVersion string) (*AzureOAI, error) {
	if deployment == "" {
		return nil, fmt.Errorf("model: missing Azure deployment name")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("model: missing Azure endpoint")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("model: missing Azure API key")
	}

	// Normalise endpoint: strip trailing slashes, prepend https:// if no scheme.
	endpoint = strings.TrimRight(endpoint, "/")
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}

	if apiVersion == "" {
		apiVersion = "2024-12-01-preview"
	}
	return &AzureOAI{
		Endpoint:   endpoint,
		Deployment: deployment,
		APIKey:     apiKey,
		APIVersion: apiVersion,
	}, nil
}

// Verify sends the system prompt + user payload to the Azure OpenAI
// /chat/completions endpoint. It returns the text from the first choice,
// the compute cost in USD (always 0 — Azure pricing is complex and not
// modelled here), or an error.
//
// No logging of API keys, request bodies, or response payloads — per
// AGENTS.md Security.
func (a *AzureOAI) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, int64, int64, error) {
	reqBody := chatRequest{
		Model: a.Deployment,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPayload},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
		return "", 0, 0, 0, fmt.Errorf("model: marshal request: %w", err)
	}

	// Build the Azure URL:
	// https://<endpoint>/openai/deployments/<deployment>/chat/completions?api-version=<version>
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		a.Endpoint, a.Deployment, a.APIVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("model: build request: %w", err)
	}
	req.Header.Set("api-key", a.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("model: azure dispatch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("model: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, 0, 0, NewProviderError(resp.StatusCode, "azure", a.Deployment, body)
	}

	var cr ChatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", 0, 0, 0, fmt.Errorf("model: unmarshal response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", 0, 0, 0, fmt.Errorf("model: empty choices in response")
	}

	// Azure cost is not modelled (pricing varies by deployment tier, region,
	// and commitment). Return 0 — the caller still received a verdict.
	return cr.Choices[0].Message.Content, 0, 0, 0, nil
}
