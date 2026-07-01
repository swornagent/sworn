package model

// PricingEntry holds per-model token prices in USD per million tokens.
type PricingEntry struct {
	InputPricePer1M  float64
	OutputPricePer1M float64
}

// Pricing is the canonical per-model pricing table, used by every driver to
// compute dispatch cost from token counts. Model IDs that are not in this
// table default to zero cost (the caller still receives the verdict).
//
// Prices are sourced from public API pricing pages (USD, 2026-06 snapshot).
// Anthropic models: https://www.anthropic.com/pricing
// OpenAI models: https://openai.com/api/pricing/
var Pricing = map[string]PricingEntry{
	// Anthropic models.
	"claude-opus-4-8": {5.00, 25.00},
	// claude-sonnet-5: introductory $2/$10 per MTok through 2026-08-31 (ratified,
	// Anthropic models-overview footnote 4). Standard rate $3/$15 applies AFTER
	// 2026-08-31 — FLIP this entry to {3.00, 15.00} then. Tracked: sworn#41.
	"claude-sonnet-5":   {2.00, 10.00},
	"claude-sonnet-4-6": {3.00, 15.00},
	"claude-haiku-4-5":  {1.00, 5.00},

	// OpenAI models.
	"gpt-4.1":       {2.00, 8.00},
	"gpt-4.1-mini":  {0.30, 0.80},
	"gpt-4.1-nano":  {0.10, 0.40},
	"gpt-4o":        {2.50, 10.00},
	"gpt-4o-mini":   {0.15, 0.60},
	"o4-mini":       {1.10, 4.40},
	"o3":            {10.00, 40.00},
	"o3-mini":       {1.10, 4.40},
	"gpt-5.5":       {1.25, 10.00},
	"gpt-5.5-pro":   {2.50, 20.00},
	"gpt-5.3-codex": {3.00, 12.00},
}

// ComputeCost returns the USD cost for a dispatch from token counts and the
// Pricing table. Returns 0 for unknown models (the caller still received a
// verdict — failing the gate because we don't know the price would be worse
// than reporting 0).
func ComputeCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := Pricing[model]
	if !ok {
		return 0
	}
	inputCost := float64(inputTokens) / 1_000_000 * p.InputPricePer1M
	outputCost := float64(outputTokens) / 1_000_000 * p.OutputPricePer1M
	return inputCost + outputCost
}
