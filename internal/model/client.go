// Package model abstracts the verification model behind a single interface.
//
// Design principle (see SwornAgent captures): the customer owns the model and
// the data path; SwornAgent owns the protocol. The model is a parameter — any
// implementation (OpenAI-compatible /chat/completions, a hosted endpoint, the
// customer's own cloud tenancy) plugs in here. The fresh-context, artefact-only,
// fail-closed protocol is enforced by the caller (package verify), not here.
package model

import "context"

// Verifier dispatches one fresh-context verification and returns the model's raw
// verdict text plus the dispatch cost in USD (0 if the provider does not report
// cost). Token counts (input/output) and the confirmed model ID from the response
// are returned for dispatch-record enrichment (S24).
type Verifier interface {
	Verify(ctx context.Context, systemPrompt, userPayload string) (text string, costUSD float64, inputTokens int64, outputTokens int64, err error)
}

// Pricing holds the USD cost per 1M tokens for a model.
type Pricing struct {
	InputPricePer1M  float64
	OutputPricePer1M float64
}

// PriceForModel returns the pricing for a model ID across all known provider
// pricing maps. Returns (0, 0, false) for unknown models.
func PriceForModel(modelID string) (Pricing, bool) {
	// Check OAI pricing map.
	if p, ok := modelPricing[modelID]; ok {
		return Pricing{InputPricePer1M: p.promptCostPer1M, OutputPricePer1M: p.completionCostPer1M}, true
	}
	// Check Anthropic pricing map.
	if p, ok := anthropicPricing[modelID]; ok {
		return Pricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}, true
	}
	// Check Google pricing map.
	if p, ok := googlePricing[modelID]; ok {
		return Pricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}, true
	}
	// Check Bedrock pricing map.
	if p, ok := bedrockPricing[modelID]; ok {
		return Pricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}, true
	}
	return Pricing{}, false
}

// ComputeCostFromTokens returns the USD cost for a model given token counts.
// Returns 0 for unknown models.
func ComputeCostFromTokens(modelID string, inputTokens, outputTokens int64) float64 {
	p, ok := PriceForModel(modelID)
	if !ok {
		return 0
	}
	inputCost := float64(inputTokens) / 1_000_000 * p.InputPricePer1M
	outputCost := float64(outputTokens) / 1_000_000 * p.OutputPricePer1M
	return inputCost + outputCost
}

// Unconfigured is the default until a provider client is wired (next slice:
// OpenAI-compatible client). It fails closed so an unconfigured gate BLOCKS
// rather than silently passing.
type Unconfigured struct{}

func (Unconfigured) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
	return "", 0, 0, 0, ErrNotConfigured
}

// ErrNotConfigured signals no verifier model/key was provided.
var ErrNotConfigured = constErr("verifier model not configured (pass --verifier-model and the provider key)")

type constErr string

func (e constErr) Error() string { return string(e) }