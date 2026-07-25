package driver

import "regexp"

const CostSourceProviderReported = "provider_reported"

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// CostObservation is accepted only from an adapter that received a real,
// typed provider report. W2 has no pricing table and performs no estimation.
type CostObservation struct {
	MicroUnits int64
	Currency   string
	Source     string
}

type UsageReceipt struct {
	InputTokens    *int64  `json:"input_tokens"`
	OutputTokens   *int64  `json:"output_tokens"`
	CostMicroUnits *int64  `json:"cost_micro_units"`
	Currency       *string `json:"currency"`
	Source         *string `json:"source"`
}

func NormalizeUsage(usage *Usage, cost *CostObservation) (UsageReceipt, error) {
	var receipt UsageReceipt
	if usage != nil {
		if usage.InputTokens < 0 || usage.InputTokens > MaxSafeInteger ||
			usage.OutputTokens < 0 || usage.OutputTokens > MaxSafeInteger {
			return UsageReceipt{}, fail("INVALID_USAGE")
		}
		input := usage.InputTokens
		output := usage.OutputTokens
		receipt.InputTokens = &input
		receipt.OutputTokens = &output
	}
	if cost != nil {
		if cost.MicroUnits < 0 || cost.MicroUnits > MaxSafeInteger ||
			!currencyPattern.MatchString(cost.Currency) ||
			cost.Source != CostSourceProviderReported {
			return UsageReceipt{}, fail("INVALID_COST_OBSERVATION")
		}
		microUnits := cost.MicroUnits
		currency := cost.Currency
		source := cost.Source
		receipt.CostMicroUnits = &microUnits
		receipt.Currency = &currency
		receipt.Source = &source
	}
	return receipt, nil
}

func EncodeUsageReceipt(receipt UsageReceipt) ([]byte, error) {
	if (receipt.InputTokens == nil) != (receipt.OutputTokens == nil) {
		return nil, fail("PARTIAL_USAGE")
	}
	if (receipt.CostMicroUnits == nil) != (receipt.Currency == nil) ||
		(receipt.CostMicroUnits == nil) != (receipt.Source == nil) {
		return nil, fail("PARTIAL_COST")
	}
	if receipt.InputTokens != nil {
		if *receipt.InputTokens < 0 || *receipt.InputTokens > MaxSafeInteger ||
			*receipt.OutputTokens < 0 || *receipt.OutputTokens > MaxSafeInteger {
			return nil, fail("INVALID_USAGE")
		}
	}
	if receipt.CostMicroUnits != nil {
		if *receipt.CostMicroUnits < 0 || *receipt.CostMicroUnits > MaxSafeInteger ||
			!currencyPattern.MatchString(*receipt.Currency) ||
			*receipt.Source != CostSourceProviderReported {
			return nil, fail("INVALID_COST_OBSERVATION")
		}
	}
	return canonicalJSON(receipt)
}
