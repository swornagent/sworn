package model

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// Google dispatches verification calls to Google's Gemini API or Vertex AI
// using the official google.golang.org/genai SDK (v1.61.0). It implements
// Verifier.
//
// OAI-import segregation: this file imports only the genai SDK types — never
// internal/model/oai.go or any OAI struct types. The two drivers share the
// model.Error taxonomy via this package but have zero import overlap.
type Google struct {
	Client *genai.Client
	Model  string
}

// NewGoogleGemini constructs a Google driver for the Gemini API (AI Studio)
// backend. apiKey must be non-empty. The SDK client is initialised with the
// explicit API key and BackendGeminiAPI so it does not fall through to the
// env-var credential chain.
func NewGoogleGemini(modelID, apiKey string) (*Google, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("model: missing Google API key")
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("model: create Google Gemini client: %w", err)
	}
	return &Google{Client: client, Model: modelID}, nil
}

// NewGoogleVertex constructs a Google driver for the Vertex AI (GCP) backend.
// project and location must be non-empty. The SDK client uses Application
// Default Credentials — no explicit API key is required. ADC is configured
// via gcloud auth application-default login (dev) or a GCP service account
// (CI/production).
func NewGoogleVertex(modelID, project, location string) (*Google, error) {
	if project == "" {
		return nil, fmt.Errorf("model: missing Google Cloud project")
	}
	if location == "" {
		return nil, fmt.Errorf("model: missing Google Cloud location")
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  project,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("model: create Google Vertex AI client: %w", err)
	}
	return &Google{Client: client, Model: modelID}, nil
}

// Verify sends the system prompt as a SystemInstruction and userPayload as a
// single user turn to the Gemini/Vertex API. It returns the text from the
// first text part of the first candidate, the compute cost in USD, or an error.
func (g *Google) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, int64, int64, error) {
	resp, err := g.Client.Models.GenerateContent(ctx, g.Model,
		genai.Text(userPayload),
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(systemPrompt, ""),
		})
	if err != nil {
		// The genai SDK returns genai.APIError (value type) on HTTP errors.
		// The APIError type has a .Code field (int) — direct typed access,
		// no string parsing needed (unlike the Anthropic driver's heuristic).
		// Route through NewProviderError for the model.Error taxonomy.
		var apiErr genai.APIError
		if errors.As(err, &apiErr) {
			return "", 0, 0, 0, NewProviderError(apiErr.Code, "google", g.Model, nil)
		}
		// Fallback: non-HTTP error (DNS failure, TLS handshake, connection
		// refused, etc.). This error is not a *model.Error — IsTransient
		// returns true for unknown error types, so the caller's retry policy
		// will treat this as transient and retry.
		return "", 0, 0, 0, fmt.Errorf("model: google dispatch: %w", err)
	}

	// Extract the first candidate's first text part. SwornAgent uses
	// single-shot verify calls (no tools); the only content we care about
	// is the text field of the first part.
	if len(resp.Candidates) == 0 {
		return "", 0, 0, 0, fmt.Errorf("model: no candidates in Google response")
	}
	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", 0, 0, 0, fmt.Errorf("model: no content parts in Google response")
	}
	text := candidate.Content.Parts[0].Text

	cost := computeGoogleCost(g.Model, resp.UsageMetadata)
	return text, cost, 0, 0, nil
}
// googlePricing maps model IDs to USD per 1M tokens.
// Prices sourced from Google's public pricing page:
//
//	https://ai.google.dev/pricing (2026-07-08 snapshot).
//
// Gemini 2.5 Flash and 2.5 Pro prices are for prompts ≤ 128K tokens
// (the standard tier — sworn's system prompt + diff are well within this).
// Unknown models get zero cost (same posture as OAI and Anthropic).
var googlePricing = map[string]struct {
	inputPricePer1M  float64
	outputPricePer1M float64
}{
	"gemini-2.0-flash":              {0.10, 0.40},
	"gemini-2.0-flash-lite":         {0.075, 0.30},
	"gemini-2.5-flash":              {0.15, 0.60},
	"gemini-2.5-flash-lite":         {0.075, 0.30},
	"gemini-2.5-flash-lite-preview": {0.075, 0.30},
	"gemini-2.5-pro":                {1.25, 10.00},
}

// computeGoogleCost returns the USD cost for a verify call from token counts.
// Returns 0 for unknown models (the caller still received a verdict).
func computeGoogleCost(model string, usage *genai.GenerateContentResponseUsageMetadata) float64 {
	if usage == nil {
		return 0
	}
	p, ok := googlePricing[model]
	if !ok {
		return 0
	}
	inputCost := float64(usage.PromptTokenCount) / 1_000_000 * p.inputPricePer1M
	outputCost := float64(usage.CandidatesTokenCount) / 1_000_000 * p.outputPricePer1M
	return inputCost + outputCost
}
