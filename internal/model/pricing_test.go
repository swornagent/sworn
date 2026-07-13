package model

import "testing"

// TestPricing_Sonnet4_6 asserts Claude Sonnet 4.6 pricing returns non-zero
// input and output prices. Maps AC7: Sonnet 4.6 model ID returns non-zero
// input and output prices.
func TestPricing_Sonnet4_6(t *testing.T) {
	p, ok := PriceForModel("claude-sonnet-4-6")
	if !ok {
		t.Fatal("claude-sonnet-4-6 not resolvable via PriceForModel")
	}
	if p.InputPricePer1M <= 0 {
		t.Errorf("InputPricePer1M = %f, want > 0", p.InputPricePer1M)
	}
	if p.OutputPricePer1M <= 0 {
		t.Errorf("OutputPricePer1M = %f, want > 0", p.OutputPricePer1M)
	}
}

// TestPricing_Haiku4_5 asserts Claude Haiku 4.5 pricing returns non-zero
// input and output prices. Maps AC7: Haiku 4.5 model ID returns non-zero
// input and output prices.
func TestPricing_Haiku4_5(t *testing.T) {
	p, ok := PriceForModel("claude-haiku-4-5")
	if !ok {
		t.Fatal("claude-haiku-4-5 not resolvable via PriceForModel")
	}
	if p.InputPricePer1M <= 0 {
		t.Errorf("InputPricePer1M = %f, want > 0", p.InputPricePer1M)
	}
	if p.OutputPricePer1M <= 0 {
		t.Errorf("OutputPricePer1M = %f, want > 0", p.OutputPricePer1M)
	}
}

// TestPricing_Grok45 asserts xAI grok-4.5 carries a real non-zero pricing
// entry (S03 AC-04) so honest cost is CostSource=pricing-table, not the
// "unknown" the 2026-07-11 dogfood showed. The native xai/ driver dispatches
// the bare model id "grok-4.5" to PriceForModel.
func TestPricing_Grok45(t *testing.T) {
	p, ok := PriceForModel("grok-4.5")
	if !ok {
		t.Fatal("grok-4.5 not resolvable via PriceForModel")
	}
	if p.InputPricePer1M <= 0 {
		t.Errorf("InputPricePer1M = %f, want > 0", p.InputPricePer1M)
	}
	if p.OutputPricePer1M <= 0 {
		t.Errorf("OutputPricePer1M = %f, want > 0", p.OutputPricePer1M)
	}
}

// TestPricing_UnknownModelReturnsZero asserts an unknown model ID returns 0
// cost (AC-04: fail-closed, no guessed rate). Maps AC7.
func TestPricing_UnknownModelReturnsZero(t *testing.T) {
	cost := ComputeCostFromTokens("unknown-model-xyz", 1000000, 1000000)
	if cost != 0 {
		t.Errorf("ComputeCostFromTokens(unknown) = %f, want 0", cost)
	}

	if _, ok := PriceForModel("unknown-model-xyz"); ok {
		t.Error("unknown-model-xyz should not resolve via PriceForModel")
	}
}

// TestPricing_ComputeCost calculates expected cost from known token counts.
func TestPricing_ComputeCost(t *testing.T) {
	// Sonnet 4.6: $3.00/M input, $15.00/M output.
	// 1M input = $3.00, 1M output = $15.00, total = $18.00.
	cost := ComputeCostFromTokens("claude-sonnet-4-6", 1000000, 1000000)
	if cost != 18.00 {
		t.Errorf("ComputeCostFromTokens(sonnet, 1M, 1M) = %f, want 18.00", cost)
	}

	// 1000 input = $0.003, 500 output = $0.0075, total = $0.0105
	cost = ComputeCostFromTokens("claude-sonnet-4-6", 1000, 500)
	expected := float64(1000)/1_000_000*3.00 + float64(500)/1_000_000*15.00
	if cost != expected {
		t.Errorf("ComputeCostFromTokens(sonnet, 1000, 500) = %f, want %f", cost, expected)
	}
}

// TestPricing_Sonnet5 asserts Claude Sonnet 5 is priced (not the pre-S06
// fall-through to 0) at the introductory $2/$10 rate. Maps AC-01, AC-02, AC-05.
func TestPricing_Sonnet5(t *testing.T) {
	p, ok := PriceForModel("claude-sonnet-5")
	if !ok {
		t.Fatal("claude-sonnet-5 not resolvable via PriceForModel")
	}
	if p.InputPricePer1M != 2.00 {
		t.Errorf("InputPricePer1M = %f, want 2.00", p.InputPricePer1M)
	}
	if p.OutputPricePer1M != 10.00 {
		t.Errorf("OutputPricePer1M = %f, want 10.00", p.OutputPricePer1M)
	}

	// 1M input = $2.00, 1M output = $10.00, total = $12.00.
	cost := ComputeCostFromTokens("claude-sonnet-5", 1000000, 1000000)
	if cost != 12.00 {
		t.Errorf("ComputeCostFromTokens(sonnet-5, 1M, 1M) = %f, want 12.00", cost)
	}
}

// TestPricing_Opus4_8CorrectedRate asserts Claude Opus 4.8 is priced at the
// current $5/$25 rate, not the stale $15/$75 Opus 4.1 copy. Maps AC-04, AC-05.
func TestPricing_Opus4_8CorrectedRate(t *testing.T) {
	p, ok := PriceForModel("claude-opus-4-8")
	if !ok {
		t.Fatal("claude-opus-4-8 not resolvable via PriceForModel")
	}
	if p.InputPricePer1M != 5.00 {
		t.Errorf("InputPricePer1M = %f, want 5.00", p.InputPricePer1M)
	}
	if p.OutputPricePer1M != 25.00 {
		t.Errorf("OutputPricePer1M = %f, want 25.00", p.OutputPricePer1M)
	}

	// 1M input = $5.00, 1M output = $25.00, total = $30.00 (not the old $90.00).
	cost := ComputeCostFromTokens("claude-opus-4-8", 1000000, 1000000)
	if cost != 30.00 {
		t.Errorf("ComputeCostFromTokens(opus-4-8, 1M, 1M) = %f, want 30.00 (not the old 90.00)", cost)
	}
}

// TestPricing_AllKnownModelsHavePositivePrices asserts every entry across the
// four provider pricing maps has non-negative input and output prices, as
// seen through the single PriceForModel lookup path.
func TestPricing_AllKnownModelsHavePositivePrices(t *testing.T) {
	for modelID := range allPricingKeys() {
		p, ok := PriceForModel(modelID)
		if !ok {
			t.Errorf("%s: expected in PriceForModel, got not-found", modelID)
			continue
		}
		if p.InputPricePer1M < 0 {
			t.Errorf("%s: InputPricePer1M = %f, want >= 0", modelID, p.InputPricePer1M)
		}
		if p.OutputPricePer1M < 0 {
			t.Errorf("%s: OutputPricePer1M = %f, want >= 0", modelID, p.OutputPricePer1M)
		}
	}
}

// TestPricingUnified is AC-01's regression guard (R-01 mitigation): it
// enumerates every model key present in ANY of the four provider-specific
// pricing maps (modelPricing/anthropicPricing/googlePricing/bedrockPricing)
// and asserts PriceForModel resolves each one to the EXACT same
// (InputPricePer1M, OutputPricePer1M) pair the source map holds. This is the
// "one path, no drift" structural guard the now-deleted pricing.go's Pricing
// map used to (by hand) duplicate — if any of the four maps is edited without
// updating the others' shared entries, this test catches the divergence
// instead of it silently landing in a driver's recorded cost.
func TestPricingUnified(t *testing.T) {
	for modelID, want := range allPricingKeys() {
		got, ok := PriceForModel(modelID)
		if !ok {
			t.Errorf("%s: PriceForModel found no entry, want %+v", modelID, want)
			continue
		}
		if got != want {
			t.Errorf("%s: PriceForModel = %+v, want %+v (source-map drift)", modelID, got, want)
		}
	}
}

// allPricingKeys returns every model ID across the four provider pricing
// maps, each mapped to its ModelPricing pair, as read directly off the
// source maps (not through PriceForModel) so TestPricingUnified has an
// independent ground truth to compare against.
func allPricingKeys() map[string]ModelPricing {
	out := map[string]ModelPricing{}
	for id, p := range modelPricing {
		out[id] = ModelPricing{InputPricePer1M: p.promptCostPer1M, OutputPricePer1M: p.completionCostPer1M}
	}
	for id, p := range anthropicPricing {
		out[id] = ModelPricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}
	}
	for id, p := range googlePricing {
		out[id] = ModelPricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}
	}
	for id, p := range bedrockPricing {
		out[id] = ModelPricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}
	}
	return out
}
