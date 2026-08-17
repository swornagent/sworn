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
	MicroUnits int64  `json:"micro_units"`
	Currency   string `json:"currency"`
	Source     string `json:"source"`
}
type UsageReceipt struct {
	TokenStatus      Availability `json:"token_status"`
	InputTokens      *int64       `json:"input_tokens"`
	OutputTokens     *int64       `json:"output_tokens"`
	CostStatus       Availability `json:"cost_status"`
	CostMicroUnits   *int64       `json:"cost_micro_units"`
	Currency         *string      `json:"currency"`
	Source           *string      `json:"source"`
	CacheStatus      Availability `json:"cache_status,omitempty"`
	CacheReadTokens  *int64       `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64       `json:"cache_write_tokens,omitempty"`
	// ReasoningTokens is the single additive receipt field this release
	// authorizes (A7): omitempty, backward-compatible, propagated on the same
	// path cache_read_tokens takes. A nil value is honest absence and is
	// omitted on re-encode, so legacy receipts stay byte-identical.
	ReasoningTokens *int64  `json:"reasoning_tokens,omitempty"`
	EffortRequested *string `json:"effort_requested,omitempty"`
	EffortReported  *string `json:"effort_reported,omitempty"`
	FinishReason    *string `json:"finish_reason,omitempty"`
	Truncated       *bool   `json:"truncated,omitempty"`
}

func NormalizeUsage(usage *Usage, cost *CostObservation) (UsageReceipt, error) {
	receipt := UsageReceipt{
		TokenStatus: UsageUnavailable,
		CostStatus:  UsageUnavailable,
		CacheStatus: UsageUnavailable,
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
		if usage.CacheReadTokens != nil || usage.CacheWriteTokens != nil {
			if usage.CacheReadTokens != nil {
				if *usage.CacheReadTokens < 0 || *usage.CacheReadTokens > MaxSafeInteger {
					return UsageReceipt{}, fail("INVALID_USAGE")
				}
				read := *usage.CacheReadTokens
				receipt.CacheReadTokens = &read
			}
			if usage.CacheWriteTokens != nil {
				if *usage.CacheWriteTokens < 0 || *usage.CacheWriteTokens > MaxSafeInteger {
					return UsageReceipt{}, fail("INVALID_USAGE")
				}
				write := *usage.CacheWriteTokens
				receipt.CacheWriteTokens = &write
			}
			receipt.CacheStatus = UsageReported
		}
		if usage.ReasoningTokens != nil {
			if *usage.ReasoningTokens < 0 || *usage.ReasoningTokens > MaxSafeInteger {
				return UsageReceipt{}, fail("INVALID_USAGE")
			}
			reasoning := *usage.ReasoningTokens
			receipt.ReasoningTokens = &reasoning
		}
	}
	if cost != nil {
		if err := validateCostObservation(*cost); err != nil {
			return UsageReceipt{}, err
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

func validateCostObservation(cost CostObservation) error {
	if cost.MicroUnits < 0 || cost.MicroUnits > MaxSafeInteger ||
		!currencyPattern.MatchString(cost.Currency) ||
		cost.Source != CostSourceProviderReported {
		return fail("INVALID_COST_OBSERVATION")
	}
	return nil
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
	// The cache family admits three canonical states: absent (empty status
	// with nil values, used by legacy journals and non-provider construction
	// sites), explicitly unavailable (status with nil values), and reported
	// (status with at least one non-nil value; a vocabulary that has no
	// write side, such as Gemini or the Responses API, reports read only and
	// the missing side stays nil rather than becoming a measured zero).
	switch receipt.CacheStatus {
	case "":
		if receipt.CacheReadTokens != nil || receipt.CacheWriteTokens != nil {
			return nil, fail("PARTIAL_CACHE")
		}
	case UsageUnavailable:
		if receipt.CacheReadTokens != nil || receipt.CacheWriteTokens != nil {
			return nil, fail("PARTIAL_CACHE")
		}
	case UsageReported:
		if receipt.CacheReadTokens == nil && receipt.CacheWriteTokens == nil {
			return nil, fail("PARTIAL_CACHE")
		}
		if receipt.CacheReadTokens != nil &&
			(*receipt.CacheReadTokens < 0 || *receipt.CacheReadTokens > MaxSafeInteger) {
			return nil, fail("INVALID_USAGE")
		}
		if receipt.CacheWriteTokens != nil &&
			(*receipt.CacheWriteTokens < 0 || *receipt.CacheWriteTokens > MaxSafeInteger) {
			return nil, fail("INVALID_USAGE")
		}
	default:
		return nil, fail("PARTIAL_CACHE")
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
	if receipt.ReasoningTokens != nil &&
		(*receipt.ReasoningTokens < 0 ||
			*receipt.ReasoningTokens > MaxSafeInteger) {
		return nil, fail("INVALID_USAGE")
	}
	for _, value := range []*string{
		receipt.EffortRequested, receipt.EffortReported, receipt.FinishReason,
	} {
		if value != nil && validateText(*value, MaxOpaqueFieldBytes, false) != nil {
			return nil, fail("INVALID_OBSERVATION")
		}
	}
	return canonicalJSON(receipt)
}
