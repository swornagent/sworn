package model

import (
	"testing"
)

// TestPricing_Sonnet4_6 asserts Claude Sonnet 4.6 pricing returns non-zero
// input and output prices. Maps AC7: Sonnet 4.6 model ID returns non-zero
// input and output prices.
func TestPricing_Sonnet4_6(t *testing.T) {
	p, ok := Pricing["claude-sonnet-4-6"]
	if !ok {
		t.Fatal("claude-sonnet-4-6 not in Pricing map")
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
	p, ok := Pricing["claude-haiku-4-5"]
	if !ok {
		t.Fatal("claude-haiku-4-5 not in Pricing map")
	}
	if p.InputPricePer1M <= 0 {
		t.Errorf("InputPricePer1M = %f, want > 0", p.InputPricePer1M)
	}
	if p.OutputPricePer1M <= 0 {
		t.Errorf("OutputPricePer1M = %f, want > 0", p.OutputPricePer1M)
	}
}

// TestPricing_UnknownModelReturnsZero asserts an unknown model ID returns 0
// cost. Maps AC7: unknown model IDs return 0.
func TestPricing_UnknownModelReturnsZero(t *testing.T) {
	cost := ComputeCost("unknown-model-xyz", 1000000, 1000000)
	if cost != 0 {
		t.Errorf("ComputeCost(unknown) = %f, want 0", cost)
	}

	// Also verify the model is not in the Pricing map.
	_, ok := Pricing["unknown-model-xyz"]
	if ok {
		t.Error("unknown-model-xyz should not be in Pricing map")
	}
}

// TestPricing_ComputeCost calculates expected cost from known token counts.
func TestPricing_ComputeCost(t *testing.T) {
	// Sonnet 4.6: $3.00/M input, $15.00/M output.
	// 1M input = $3.00, 1M output = $15.00, total = $18.00.
	cost := ComputeCost("claude-sonnet-4-6", 1000000, 1000000)
	if cost != 18.00 {
		t.Errorf("ComputeCost(sonnet, 1M, 1M) = %f, want 18.00", cost)
	}

	// 1000 input = $0.003, 500 output = $0.0075, total = $0.0105
	cost = ComputeCost("claude-sonnet-4-6", 1000, 500)
	expected := float64(1000)/1_000_000*3.00 + float64(500)/1_000_000*15.00
	if cost != expected {
		t.Errorf("ComputeCost(sonnet, 1000, 500) = %f, want %f", cost, expected)
	}
}

// TestPricing_AllKnownModelsHavePositivePrices asserts every entry in the
// Pricing map has positive input and output prices.
func TestPricing_AllKnownModelsHavePositivePrices(t *testing.T) {
	for modelID, p := range Pricing {
		if p.InputPricePer1M < 0 {
			t.Errorf("%s: InputPricePer1M = %f, want >= 0", modelID, p.InputPricePer1M)
		}
		if p.OutputPricePer1M < 0 {
			t.Errorf("%s: OutputPricePer1M = %f, want >= 0", modelID, p.OutputPricePer1M)
		}
	}
}
