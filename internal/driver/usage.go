package driver

import "regexp"

const (
	CostSourceProviderReported = "provider_reported"
	UsageReported              = Availability("reported")
	UsageUnavailable           = Availability("unavailable")
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type Availability string

// CostObservation accepts only typed provider reports; W2 never estimates.
type CostObservation struct {
	MicroUnits       int64
	Currency, Source string
}
type UsageReceipt struct {
	TokenStatus    Availability `json:"token_status"`
	InputTokens    *int64       `json:"input_tokens"`
	OutputTokens   *int64       `json:"output_tokens"`
	CostStatus     Availability `json:"cost_status"`
	CostMicroUnits *int64       `json:"cost_micro_units"`
	Currency       *string      `json:"currency"`
	Source         *string      `json:"source"`
}

func NormalizeUsage(usage *Usage, cost *CostObservation) (UsageReceipt, error) {
	receipt := UsageReceipt{
		TokenStatus: UsageUnavailable,
		CostStatus:  UsageUnavailable,
	}
	if usage != nil {
		if usage.InputTokens < 0 || usage.InputTokens > MaxSafeInteger ||
			usage.OutputTokens < 0 || usage.OutputTokens > MaxSafeInteger {
			return UsageReceipt{}, fail("INVALID_USAGE")
		}
		input := usage.InputTokens
		output := usage.OutputTokens
		receipt.TokenStatus = UsageReported
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
		receipt.CostStatus = UsageReported
		receipt.CostMicroUnits = &microUnits
		receipt.Currency = &currency
		receipt.Source = &source
	}
	return receipt, nil
}
func EncodeUsageReceipt(receipt UsageReceipt) ([]byte, error) {
	if (receipt.TokenStatus != UsageReported && receipt.TokenStatus != UsageUnavailable) ||
		(receipt.TokenStatus == UsageReported) != (receipt.InputTokens != nil) ||
		(receipt.InputTokens == nil) != (receipt.OutputTokens == nil) {
		return nil, fail("PARTIAL_USAGE")
	}
	if (receipt.CostStatus != UsageReported && receipt.CostStatus != UsageUnavailable) ||
		(receipt.CostStatus == UsageReported) != (receipt.CostMicroUnits != nil) ||
		(receipt.CostMicroUnits == nil) != (receipt.Currency == nil) ||
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
