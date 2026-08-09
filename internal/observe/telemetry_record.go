package observe

import (
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func validTelemetryRecord(record Record) bool {
	if record.SchemaVersion != EvalSchemaVersion ||
		len(record.ID) < 1 || len(record.ID) > 256 ||
		len(record.RunID) < 1 || len(record.RunID) > 128 ||
		len(record.Release) < 1 || len(record.Release) > 256 ||
		!validVersion(record.SwornVersion) ||
		record.ThroughOffset < 1 ||
		record.StartedAt.IsZero() || record.ObservedAt.IsZero() ||
		record.ObservedAt.Before(record.StartedAt) ||
		record.ElapsedNS < 0 ||
		record.ObservedAt.Sub(record.StartedAt).Nanoseconds() !=
			record.ElapsedNS ||
		record.RunState != boundedRunState(record.RunState) ||
		record.Outcome != boundedRunOutcome(record.Outcome) ||
		record.Events < 0 || record.Attempts < 0 || record.Retries < 0 ||
		record.Retries > record.Attempts ||
		!validDurationRatio(record.DurationNS, record.Attempts) ||
		!validUsageSummary(record.Usage, record.Attempts) ||
		record.Recovery.Uncertain < 0 ||
		record.Recovery.Reconciled < 0 ||
		record.Recovery.Recovered < 0 ||
		record.Recovery.RolledBack < 0 ||
		!validContinuationSummary(record.Continuation, record.Events) ||
		!validTurnRecoverySummary(record.TurnRecovery, record.Events) ||
		len(record.Groups) > 512 {
		return false
	}
	var attempts, retries, duration int64
	for _, group := range record.Groups {
		if group.Role != roleForResponsibility(group.Responsibility) ||
			group.Responsibility != boundedResponsibility(group.Responsibility) ||
			group.Operation != boundedOperation(group.Operation) ||
			group.Transport != boundedTransport(group.Transport) ||
			group.Outcome != boundedEffectState(
				journal.EffectState(group.Outcome),
			) ||
			group.Attempts < 1 || group.Retries < 0 ||
			group.Retries > group.Attempts ||
			!validDurationRatio(group.DurationNS, group.Attempts) ||
			!validUsageSummary(group.Usage, group.Attempts) {
			return false
		}
		var err error
		attempts, err = telemetryAdd(attempts, group.Attempts)
		if err != nil {
			return false
		}
		retries, err = telemetryAdd(retries, group.Retries)
		if err != nil {
			return false
		}
		duration, err = telemetryAdd(
			duration,
			*group.DurationNS.Numerator,
		)
		if err != nil {
			return false
		}
	}
	if attempts != record.Attempts || retries != record.Retries ||
		duration != *record.DurationNS.Numerator {
		return false
	}
	if len(record.Quality) != 4 {
		return false
	}
	for index, name := range []string{
		"delivery", "integration", "requirements", "verification",
	} {
		quality := record.Quality[index]
		if quality.Name != name ||
			(quality.Numerator == nil) != (quality.Denominator == nil) ||
			(name == "requirements") != (quality.Numerator == nil) ||
			(quality.Numerator != nil &&
				(*quality.Numerator < 0 ||
					*quality.Denominator < *quality.Numerator)) {
			return false
		}
	}
	return true
}

func validDurationRatio(value Ratio, attempts int64) bool {
	return value.Numerator != nil && value.Denominator != nil &&
		*value.Numerator >= 0 && *value.Denominator == attempts
}

func validUsageSummary(value UsageSummary, attempts int64) bool {
	if !validCoverageRatio(value.TokenCoverage, attempts) ||
		!validCoverageRatio(value.CostCoverage, attempts) ||
		!validCoverageRatio(value.CacheCoverage, attempts) ||
		(value.InputTokens == nil) != (value.OutputTokens == nil) ||
		(value.InputTokens == nil) !=
			(*value.TokenCoverage.Numerator == 0) ||
		len(value.Costs) > 32 ||
		(len(value.Costs) == 0) !=
			(*value.CostCoverage.Numerator == 0) ||
		(value.CacheReadTokens == nil && value.CacheWriteTokens == nil) !=
			(*value.CacheCoverage.Numerator == 0) ||
		!validNullableText(value.EffortRequested) ||
		!validNullableText(value.EffortReported) ||
		!validNullableText(value.FinishReason) {
		return false
	}
	if value.InputTokens != nil &&
		(*value.InputTokens < 0 || *value.OutputTokens < 0) {
		return false
	}
	if value.CacheReadTokens != nil &&
		(*value.CacheReadTokens < 0 ||
			*value.CacheReadTokens > driver.MaxSafeInteger) {
		return false
	}
	if value.CacheWriteTokens != nil &&
		(*value.CacheWriteTokens < 0 ||
			*value.CacheWriteTokens > driver.MaxSafeInteger) {
		return false
	}
	for _, cost := range value.Costs {
		if len(cost.Currency) != 3 || cost.MicroUnits < 0 {
			return false
		}
		for _, character := range []byte(cost.Currency) {
			if character < 'A' || character > 'Z' {
				return false
			}
		}
	}
	return true
}

func validCoverageRatio(value Ratio, attempts int64) bool {
	return value.Numerator != nil && value.Denominator != nil &&
		*value.Numerator >= 0 &&
		*value.Numerator <= *value.Denominator &&
		*value.Denominator == attempts
}

// validNullableText bounds the canonical string facts carried on a telemetry
// record. Values originate from validated profile vocabularies and strictly
// parsed provider responses, so any non-bounded value is rejected.
func validNullableText(value *string) bool {
	if value == nil {
		return true
	}
	if len(*value) < 1 || len(*value) > 128 {
		return false
	}
	for _, character := range *value {
		if character <= 0x1f || (character >= 0x7f && character <= 0x9f) {
			return false
		}
	}
	return true
}

func telemetryAdd(left, right int64) (int64, error) {
	const maximumInt64 = int64(^uint64(0) >> 1)
	if right < 0 || left > maximumInt64-right {
		return 0, fail("TELEMETRY_OVERFLOW")
	}
	return left + right, nil
}

func cloneRecord(record Record) Record {
	result := record
	result.Usage = cloneUsage(record.Usage)
	result.Continuation.Counts = append(
		[]ContinuationCount(nil),
		record.Continuation.Counts...,
	)
	result.TurnRecovery.Actions = append(
		[]TurnRecoveryCount(nil),
		record.TurnRecovery.Actions...,
	)
	result.Groups = append([]AttemptGroup(nil), record.Groups...)
	for index := range result.Groups {
		result.Groups[index].DurationNS = cloneRatio(record.Groups[index].DurationNS)
		result.Groups[index].Usage = cloneUsage(record.Groups[index].Usage)
	}
	result.DurationNS = cloneRatio(record.DurationNS)
	result.Quality = append([]Quality(nil), record.Quality...)
	for index := range result.Quality {
		if result.Quality[index].Numerator != nil {
			result.Quality[index].Numerator =
				int64Pointer(*record.Quality[index].Numerator)
			result.Quality[index].Denominator =
				int64Pointer(*record.Quality[index].Denominator)
		}
	}
	return result
}

func cloneRatio(value Ratio) Ratio {
	result := Ratio{}
	if value.Numerator != nil {
		result.Numerator = int64Pointer(*value.Numerator)
		result.Denominator = int64Pointer(*value.Denominator)
	}
	return result
}

func cloneUsage(value UsageSummary) UsageSummary {
	result := value
	if value.InputTokens != nil {
		result.InputTokens = int64Pointer(*value.InputTokens)
		result.OutputTokens = int64Pointer(*value.OutputTokens)
	}
	if value.CacheReadTokens != nil {
		result.CacheReadTokens = int64Pointer(*value.CacheReadTokens)
	}
	if value.CacheWriteTokens != nil {
		result.CacheWriteTokens = int64Pointer(*value.CacheWriteTokens)
	}
	if value.EffortRequested != nil {
		result.EffortRequested = cloneText(value.EffortRequested)
	}
	if value.EffortReported != nil {
		result.EffortReported = cloneText(value.EffortReported)
	}
	if value.FinishReason != nil {
		result.FinishReason = cloneText(value.FinishReason)
	}
	if value.Truncated != nil {
		result.Truncated = boolPointer(*value.Truncated)
	}
	result.Costs = append([]CostTotal(nil), value.Costs...)
	result.TokenCoverage = cloneRatio(value.TokenCoverage)
	result.CostCoverage = cloneRatio(value.CostCoverage)
	result.CacheCoverage = cloneRatio(value.CacheCoverage)
	return result
}

func cloneText(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func boolPointer(value bool) *bool { return &value }

func textPointer(value string) *string { return &value }
