package driver

import (
	"regexp"
	"sort"
)

const (
	CostSourceProviderReported = "provider_reported"
	UsageReported              = Availability("reported")
	UsageUnavailable           = Availability("unavailable")
)

// UsageSchemaV2 is the additive receipt schema this release introduces.
// Receipts carrying any v2 field must declare it; receipts carrying none keep
// today's exact encoding so every historical blob re-encodes byte-identically.
const UsageSchemaV2 = "sworn.usage/v2"

// Unavailable reasons are the A2 vocabulary: a receipt built because no turn
// ever carried usage is wire-lacked-usage; a receipt the capture pipeline
// failed to carry through is capture-failed.
const (
	UsageReasonWireLacked    = "wire-lacked-usage"
	UsageReasonCaptureFailed = "capture-failed"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type Availability string

// CostObservation accepts only typed provider reports; W2 never estimates.
type CostObservation struct {
	MicroUnits int64  `json:"micro_units"`
	Currency   string `json:"currency"`
	Source     string `json:"source"`
}

// ToolCallCount is one name-count pair of the closed tool vocabulary. Counts
// are strictly positive and the slice is sorted by name, so the canonical
// JSON representation is unique.
type ToolCallCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
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
	// The v2 fields below are additive and version-gated: any of them being
	// set flips the receipt to sworn.usage/v2, which requires schema_version,
	// surface, and (when unavailable) unavailable_reason. All are omitempty
	// so legacy receipts keep today's canonical bytes exactly.
	SchemaVersion     string `json:"schema_version,omitempty"`
	Surface           string `json:"surface,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	// Turns and ToolCalls are the A5 turn economics, counted engine-side.
	// ToolCallsByName carries the per-name mix over the closed tool
	// vocabulary; nil is honest absence (e.g. surfaces whose tool names are
	// not in the closed vocabulary, or legacy receipts).
	Turns           *int64          `json:"turns,omitempty"`
	ToolCalls       *int64          `json:"tool_calls,omitempty"`
	ToolCallsByName []ToolCallCount `json:"tool_calls_by_name,omitempty"`
	// DurationMillis, Profile, and Model are the A4 attempt facts stamped at
	// the runtime's single attempt-write seam.
	DurationMillis *int64  `json:"duration_ms,omitempty"`
	Profile        *string `json:"profile,omitempty"`
	Model          *string `json:"model,omitempty"`
}

// Zero reports whether the receipt carries no fact at all, field by field.
// It replaces the struct equality that the new slice field made impossible
// while preserving today's exact semantics (including the new fields).
func (receipt UsageReceipt) Zero() bool {
	return receipt.TokenStatus == "" && receipt.InputTokens == nil &&
		receipt.OutputTokens == nil && receipt.CostStatus == "" &&
		receipt.CostMicroUnits == nil && receipt.Currency == nil &&
		receipt.Source == nil && receipt.CacheStatus == "" &&
		receipt.CacheReadTokens == nil && receipt.CacheWriteTokens == nil &&
		receipt.ReasoningTokens == nil && receipt.EffortRequested == nil &&
		receipt.EffortReported == nil && receipt.FinishReason == nil &&
		receipt.Truncated == nil && receipt.SchemaVersion == "" &&
		receipt.Surface == "" && receipt.UnavailableReason == "" &&
		receipt.Turns == nil && receipt.ToolCalls == nil &&
		receipt.ToolCallsByName == nil && receipt.DurationMillis == nil &&
		receipt.Profile == nil && receipt.Model == nil
}

// carriesV2Facts reports whether the receipt carries any v2 field. Receipts
// carrying none keep today's exact encoding rules (the v1 path) so every
// historical blob re-encodes byte-identically.
func (receipt UsageReceipt) carriesV2Facts() bool {
	return receipt.SchemaVersion != "" || receipt.Surface != "" ||
		receipt.UnavailableReason != "" || receipt.Turns != nil ||
		receipt.ToolCalls != nil || receipt.ToolCallsByName != nil ||
		receipt.DurationMillis != nil || receipt.Profile != nil ||
		receipt.Model != nil
}

func validUnavailableReason(value string) bool {
	return value == UsageReasonWireLacked ||
		value == UsageReasonCaptureFailed
}

func validToolVocabularyName(name string) bool {
	switch name {
	case "Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"sworn_submit", "sworn_yield":
		return true
	default:
		return false
	}
}

// UnavailableReceipt builds the loud v2 unavailable receipt A2 requires: the
// reporting surface (adapter id) and a stable reason ride on the receipt, so
// an attempt that genuinely cannot report says so with the surface named
// instead of defaulting silent.
func UnavailableReceipt(surface, reason string) (UsageReceipt, error) {
	if !driverIdentityPattern.MatchString(surface) ||
		!validUnavailableReason(reason) {
		return UsageReceipt{}, fail("INVALID_USAGE")
	}
	return UsageReceipt{
		SchemaVersion:     UsageSchemaV2,
		Surface:           surface,
		TokenStatus:       UsageUnavailable,
		CostStatus:        UsageUnavailable,
		CacheStatus:       UsageUnavailable,
		UnavailableReason: reason,
	}, nil
}

func NormalizeUsage(
	usage *Usage,
	cost *CostObservation,
	surface string,
) (UsageReceipt, error) {
	if !driverIdentityPattern.MatchString(surface) {
		return UsageReceipt{}, fail("INVALID_USAGE")
	}
	receipt := UsageReceipt{
		SchemaVersion: UsageSchemaV2,
		Surface:       surface,
		TokenStatus:   UsageUnavailable,
		CostStatus:    UsageUnavailable,
		CacheStatus:   UsageUnavailable,
	}
	if usage == nil {
		// No turn ever carried usage: the wire lacked it, and the receipt
		// says so by name instead of defaulting silent.
		receipt.UnavailableReason = UsageReasonWireLacked
	} else {
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

// StampAttemptFacts adds the A4 attempt identity facts (profile, certified
// model, wall-clock duration) to a v2 receipt. It is best-effort by design:
// a receipt without a surface is a legacy-shaped receipt whose bytes must
// stay untouched, and a fact that fails its bound is simply not stamped, so
// the stamp never turns a valid receipt invalid.
func StampAttemptFacts(
	receipt *UsageReceipt,
	profile, model string,
	durationMillis int64,
) {
	if receipt == nil || receipt.Surface == "" {
		return
	}
	if providerKeyPattern.MatchString(profile) &&
		validateText(model, 500, false) == nil {
		profileValue := profile
		modelValue := model
		receipt.Profile = &profileValue
		receipt.Model = &modelValue
	}
	if durationMillis >= 0 && durationMillis <= MaxSafeInteger {
		duration := durationMillis
		receipt.DurationMillis = &duration
	}
}

// applyTurnEconomics stamps the A5 turn facts onto a v2 receipt. Counts are
// engine-counted and bounded; a count outside its bound or a mix that cannot
// sum to the tool-call total is honest absence rather than a fabricated
// number, so a malformed stamp never turns a valid receipt invalid.
func applyTurnEconomics(
	receipt *UsageReceipt,
	turns, toolCalls int64,
	byName map[string]int64,
) {
	if receipt == nil || receipt.Surface == "" {
		return
	}
	if turns < 0 || turns > MaxProviderTurns ||
		toolCalls < 0 || toolCalls > turns*MaxToolCalls {
		return
	}
	turnsValue := turns
	toolCallsValue := toolCalls
	receipt.Turns = &turnsValue
	receipt.ToolCalls = &toolCallsValue
	mix := canonicalToolCallMix(byName)
	if mix == nil || sumToolCallMix(mix) != toolCalls {
		return
	}
	receipt.ToolCallsByName = mix
}

func canonicalToolCallMix(byName map[string]int64) []ToolCallCount {
	if len(byName) == 0 {
		return nil
	}
	mix := make([]ToolCallCount, 0, len(byName))
	for name, count := range byName {
		if count > 0 && validToolVocabularyName(name) {
			mix = append(mix, ToolCallCount{Name: name, Count: count})
		}
	}
	if len(mix) == 0 {
		return nil
	}
	sort.Slice(mix, func(left, right int) bool {
		return mix[left].Name < mix[right].Name
	})
	return mix
}

func sumToolCallMix(mix []ToolCallCount) int64 {
	var total int64
	for _, item := range mix {
		total += item.Count
	}
	return total
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
	if receipt.carriesV2Facts() {
		if err := validateV2UsageReceipt(receipt); err != nil {
			return nil, err
		}
	}
	return canonicalJSON(receipt)
}

// validateV2UsageReceipt enforces the A2-A5 v2 rules. A silent unavailable
// v2 receipt is unencodable, hence unjournalable: it must name the surface
// and a stable reason.
func validateV2UsageReceipt(receipt UsageReceipt) error {
	if receipt.SchemaVersion != UsageSchemaV2 {
		return fail("INVALID_USAGE")
	}
	if !driverIdentityPattern.MatchString(receipt.Surface) {
		return fail("INVALID_USAGE")
	}
	if receipt.TokenStatus == UsageUnavailable {
		if !validUnavailableReason(receipt.UnavailableReason) {
			return fail("INVALID_USAGE")
		}
	} else if receipt.UnavailableReason != "" {
		return fail("INVALID_USAGE")
	}
	if receipt.Turns != nil {
		if *receipt.Turns < 0 || *receipt.Turns > MaxProviderTurns {
			return fail("INVALID_USAGE")
		}
	}
	if receipt.ToolCalls != nil {
		if receipt.Turns == nil ||
			*receipt.ToolCalls < 0 ||
			*receipt.ToolCalls > *receipt.Turns*MaxToolCalls {
			return fail("INVALID_USAGE")
		}
	}
	if receipt.ToolCallsByName != nil {
		if receipt.ToolCalls == nil ||
			len(receipt.ToolCallsByName) > 8 {
			return fail("INVALID_USAGE")
		}
		var total int64
		previous := ""
		for _, item := range receipt.ToolCallsByName {
			if !validToolVocabularyName(item.Name) ||
				item.Count < 1 ||
				(previous != "" && item.Name <= previous) ||
				total > *receipt.ToolCalls-item.Count {
				return fail("INVALID_USAGE")
			}
			total += item.Count
			previous = item.Name
		}
		if total != *receipt.ToolCalls {
			return fail("INVALID_USAGE")
		}
	}
	if receipt.DurationMillis != nil &&
		(*receipt.DurationMillis < 0 ||
			*receipt.DurationMillis > MaxSafeInteger) {
		return fail("INVALID_USAGE")
	}
	if receipt.Profile != nil &&
		!providerKeyPattern.MatchString(*receipt.Profile) {
		return fail("INVALID_USAGE")
	}
	if receipt.Model != nil &&
		validateText(*receipt.Model, 500, false) != nil {
		return fail("INVALID_USAGE")
	}
	return nil
}
