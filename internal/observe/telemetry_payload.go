package observe

import (
	"bytes"
	"crypto/sha256"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
)

var telemetryMetricNames = []string{
	"sworn.eval.events",
	"sworn.eval.continuations",
	"sworn.eval.continuation.outcomes",
	"sworn.eval.attempts",
	"sworn.eval.retries",
	"sworn.eval.recoveries",
	"sworn.eval.turn_recovery.actions",
	"sworn.eval.turn_recovery.outcomes",
	"sworn.eval.duration_ns.numerator",
	"sworn.eval.duration_ns.denominator",
	"sworn.eval.observation_duration_ns.numerator",
	"sworn.eval.observation_duration_ns.denominator",
	"sworn.eval.input_tokens",
	"sworn.eval.output_tokens",
	"sworn.eval.usage_coverage.numerator",
	"sworn.eval.usage_coverage.denominator",
	"sworn.eval.cache_read_tokens",
	"sworn.eval.cache_write_tokens",
	"sworn.eval.reasoning_tokens",
	"sworn.eval.cache_coverage.numerator",
	"sworn.eval.cache_coverage.denominator",
	"sworn.eval.turns",
	"sworn.eval.tool_calls",
	"sworn.eval.tool_calls_per_turn.numerator",
	"sworn.eval.tool_calls_per_turn.denominator",
	"sworn.eval.tool_calls.by_name",
	"sworn.eval.quality.numerator",
	"sworn.eval.quality.denominator",
}

type telemetryAttributeKind uint8

const (
	telemetryStringAttribute telemetryAttributeKind = iota + 1
	telemetryInt64Attribute
	telemetryBoolAttribute
)

type telemetryAttribute struct {
	key         string
	kind        telemetryAttributeKind
	stringValue string
	int64Value  int64
	boolValue   bool
}

func stringTelemetryAttribute(key, value string) telemetryAttribute {
	return telemetryAttribute{
		key:         key,
		kind:        telemetryStringAttribute,
		stringValue: value,
	}
}

func int64TelemetryAttribute(key string, value int64) telemetryAttribute {
	return telemetryAttribute{
		key:        key,
		kind:       telemetryInt64Attribute,
		int64Value: value,
	}
}

func boolTelemetryAttribute(key string, value bool) telemetryAttribute {
	return telemetryAttribute{
		key:       key,
		kind:      telemetryBoolAttribute,
		boolValue: value,
	}
}

type telemetryResource struct {
	serviceName    string
	serviceVersion string
}

type telemetryMetricPoint struct {
	attributes []telemetryAttribute
	observedAt time.Time
	value      int64
}

type telemetryMetric struct {
	name   string
	points []telemetryMetricPoint
}

type telemetryMetricPayload struct {
	resource telemetryResource
	metrics  []telemetryMetric
}

type metricAccumulator struct {
	series map[string]map[string]telemetryMetricPoint
}

func newMetricAccumulator() metricAccumulator {
	return metricAccumulator{
		series: make(
			map[string]map[string]telemetryMetricPoint,
			len(telemetryMetricNames),
		),
	}
}

func (a *metricAccumulator) record(
	name string,
	value int64,
	observedAt time.Time,
	attributes []telemetryAttribute,
) {
	key := telemetryAttributeKey(attributes)
	points := a.series[name]
	if points == nil {
		points = make(map[string]telemetryMetricPoint)
		a.series[name] = points
	}
	if _, exists := points[key]; !exists &&
		len(points) >= telemetryMetricCardinality {
		return
	}
	points[key] = telemetryMetricPoint{
		attributes: append([]telemetryAttribute(nil), attributes...),
		observedAt: observedAt,
		value:      value,
	}
}

func (a *metricAccumulator) payload(
	value telemetryResource,
) telemetryMetricPayload {
	metrics := make([]telemetryMetric, 0, len(telemetryMetricNames))
	for _, name := range telemetryMetricNames {
		series := a.series[name]
		if len(series) == 0 {
			continue
		}
		keys := make([]string, 0, len(series))
		for key := range series {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		points := make([]telemetryMetricPoint, 0, len(keys))
		for _, key := range keys {
			points = append(points, series[key])
		}
		metrics = append(metrics, telemetryMetric{
			name:   name,
			points: points,
		})
	}
	return telemetryMetricPayload{
		resource: value,
		metrics:  metrics,
	}
}

func telemetryAttributeKey(attributes []telemetryAttribute) string {
	var result strings.Builder
	for _, attribute := range attributes {
		result.WriteString(strconv.Itoa(len(attribute.key)))
		result.WriteByte(':')
		result.WriteString(attribute.key)
		result.WriteByte(':')
		result.WriteByte(byte(attribute.kind))
		result.WriteByte(':')
		switch attribute.kind {
		case telemetryStringAttribute:
			result.WriteString(strconv.Itoa(len(attribute.stringValue)))
			result.WriteByte(':')
			result.WriteString(attribute.stringValue)
		case telemetryInt64Attribute:
			result.WriteString(strconv.FormatInt(attribute.int64Value, 10))
		case telemetryBoolAttribute:
			result.WriteString(strconv.FormatBool(attribute.boolValue))
		}
		result.WriteByte(';')
	}
	return result.String()
}

type telemetrySpan struct {
	name         string
	traceID      [16]byte
	spanID       [8]byte
	parentSpanID [8]byte
	hasParent    bool
	startedAt    time.Time
	endedAt      time.Time
	attributes   []telemetryAttribute
	resource     telemetryResource
}

const dispatchSpanName = "sworn.dispatch"

// maxDispatchDurationMillis bounds the receipt duration stamp so the
// millisecond->nanosecond conversion can never overflow; a stamp beyond it
// cannot be a real wall-clock duration and the span is skipped instead of
// stating a false fact.
const maxDispatchDurationMillis = math.MaxInt64 / int64(time.Millisecond)

// telemetrySpans builds the span payload for one evaluation record: the
// cumulative segment span, the recovery span when present, and one
// sworn.dispatch span per not-yet-exported driver dispatch attempt in
// journal order. The exported set is checked before the snapshot join and
// attribute construction, so per-advance work is proportional to newly seen
// attempts (S2-C3).
func telemetrySpans(
	record Record,
	value telemetryResource,
	exported map[[8]byte]struct{},
) []telemetrySpan {
	traceID, segmentID, recoveryID := telemetrySpanIDs(record)
	recovery := hasRecovery(record.Recovery)
	spans := []telemetrySpan{{
		name:       "sworn.process.segment",
		traceID:    traceID,
		spanID:     segmentID,
		startedAt:  record.StartedAt,
		endedAt:    record.ObservedAt,
		attributes: segmentAttributes(record),
		resource:   value,
	}}
	if recovery {
		spans = append(spans, telemetrySpan{
			name:         "sworn.recovery",
			traceID:      traceID,
			spanID:       recoveryID,
			parentSpanID: segmentID,
			hasParent:    true,
			startedAt:    record.ObservedAt,
			endedAt:      record.ObservedAt,
			attributes:   recoveryAttributes(record.Recovery),
			resource:     value,
		})
	}
	for index := range record.dispatchAttempts {
		attempt := &record.dispatchAttempts[index]
		// S2-C4: only a driver dispatch attempt becomes a sworn.dispatch
		// span; a non-dispatch attempt row must not mint one.
		if attempt.effectKind != dispatchEffectKind {
			continue
		}
		spanID := dispatchSpanID(record.RunID, *attempt)
		if _, seen := exported[spanID]; seen {
			continue
		}
		identity := resolveDispatchIdentity(*attempt, record.spanJoin)
		span, ok := dispatchSpan(
			record,
			*attempt,
			identity,
			traceID,
			segmentID,
			spanID,
			value,
		)
		if !ok {
			continue
		}
		spans = append(spans, span)
	}
	return spans
}

// telemetrySpanIDs derives one deterministic trace identity per run and
// stable per-scope span ids, replacing the per-record randomized ids. The
// segment and recovery spans keep stable ids across cumulative
// re-evaluations inside their run's trace.
func telemetrySpanIDs(
	record Record,
) ([16]byte, [8]byte, [8]byte) {
	return runTraceID(record.RunID),
		spanIDFor(record.RunID, "segment", ""),
		spanIDFor(record.RunID, "recovery", "")
}

func runTraceID(runID string) [16]byte {
	var traceID [16]byte
	sum := sha256.Sum256(append([]byte("sworn.trace/v1\x00"), runID...))
	copy(traceID[:], sum[:])
	if allZero(traceID[:]) {
		traceID[len(traceID)-1] = 1
	}
	return traceID
}

func spanIDFor(runID, scope, identity string) [8]byte {
	input := make(
		[]byte,
		0,
		len("sworn.span/v1")+1+len(scope)+1+len(runID)+1+len(identity),
	)
	input = append(input, "sworn.span/v1\x00"...)
	input = append(input, scope...)
	input = append(input, 0)
	input = append(input, runID...)
	input = append(input, 0)
	input = append(input, identity...)
	sum := sha256.Sum256(input)
	var spanID [8]byte
	copy(spanID[:], sum[:])
	if allZero(spanID[:]) {
		spanID[len(spanID)-1] = 1
	}
	return spanID
}

// dispatchSpanID derives the attempt's stable span identity from durable
// journal columns only: responsibility, attempt number, transport, the
// attempt timestamps, and the canonical usage bytes. It deliberately
// excludes the snapshot-joined effect id, so identity cannot drift as the
// cockpit windows slide and dedupe-state eviction stays safe.
func dispatchSpanID(runID string, attempt dispatchAttempt) [8]byte {
	var identity bytes.Buffer
	identity.WriteString(attempt.responsibility)
	identity.WriteByte(0)
	identity.WriteString(strconv.FormatInt(attempt.attempt, 10))
	identity.WriteByte(0)
	identity.WriteString(attempt.transport)
	identity.WriteByte(0)
	identity.WriteString(
		attempt.startedAt.UTC().Format(time.RFC3339Nano),
	)
	identity.WriteByte(0)
	identity.WriteString(
		attempt.finishedAt.UTC().Format(time.RFC3339Nano),
	)
	identity.WriteByte(0)
	identity.Write(attempt.usageBytes)
	return spanIDFor(runID, "dispatch", identity.String())
}

// dispatchIdentity is the A3 effect-identity join result. Absent fields
// are omitted from the span rather than guessed (loud absence).
type dispatchIdentity struct {
	hasEpochTry bool
	epoch       int64
	try         int64
	hasSlice    bool
	slice       string
	hasTrack    bool
	track       string
}

// resolveDispatchIdentity joins one attempt against the bounded snapshot
// window: the attempts window yields the effect id (responsibility, attempt
// number, and the same created_at column the fact carries), epoch/try parse
// from the effect id, and evidence yields slice/track with the graph as the
// track fallback. Ambiguous or window-missed joins omit the affected
// attributes rather than guess.
func resolveDispatchIdentity(
	attempt dispatchAttempt,
	join spanJoin,
) dispatchIdentity {
	var result dispatchIdentity
	var matched *cockpit.AttemptView
	count := 0
	for index := range join.attempts {
		view := &join.attempts[index]
		if view.Responsibility != attempt.responsibility ||
			view.Number != attempt.attempt ||
			!view.CreatedAt.Equal(attempt.finishedAt) {
			continue
		}
		count++
		matched = view
		if count > 1 {
			break
		}
	}
	if count != 1 || matched == nil {
		return result
	}
	effectID := matched.EffectID
	if epoch, try, ok := parseAttemptCoordinates(effectID); ok {
		result.epoch, result.try = epoch, try
		result.hasEpochTry = true
	}
	var evidenceMatch *cockpit.Evidence
	evidenceCount := 0
	for index := range join.evidence {
		item := &join.evidence[index]
		if item.EffectID != effectID {
			continue
		}
		evidenceCount++
		evidenceMatch = item
		if evidenceCount > 1 {
			break
		}
	}
	if evidenceCount == 1 && evidenceMatch != nil {
		if evidenceMatch.Slice != "" {
			result.slice = evidenceMatch.Slice
			result.hasSlice = true
		}
		if evidenceMatch.Track != "" {
			result.track = evidenceMatch.Track
			result.hasTrack = true
		} else if result.hasSlice {
			if track, ok := join.sliceTracks["slice:"+result.slice]; ok &&
				track != "" {
				result.track = track
				result.hasTrack = true
			}
		}
	}
	return result
}

// parseAttemptCoordinates parses epoch and try from the canonical attempt
// effect id attempt/<work>/e<epoch>/t<try>, tolerating longer derived paths
// (attempt/<work>/e1/t1/human-park). Anything else fails.
func parseAttemptCoordinates(effectID string) (int64, int64, bool) {
	rest, ok := strings.CutPrefix(effectID, "attempt/")
	if !ok {
		return 0, 0, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[0] == "" {
		return 0, 0, false
	}
	epochText, ok := strings.CutPrefix(parts[1], "e")
	if !ok {
		return 0, 0, false
	}
	tryText, ok := strings.CutPrefix(parts[2], "t")
	if !ok {
		return 0, 0, false
	}
	epoch, err := strconv.ParseInt(epochText, 10, 64)
	if err != nil || epoch < 0 {
		return 0, 0, false
	}
	try, err := strconv.ParseInt(tryText, 10, 64)
	if err != nil || try < 0 {
		return 0, 0, false
	}
	return epoch, try, true
}

// dispatchSpan builds one sworn.dispatch span, skipping at construction any
// entry that cannot form a valid span (one malformed span would fail the
// whole OTLP batch).
func dispatchSpan(
	record Record,
	attempt dispatchAttempt,
	identity dispatchIdentity,
	traceID [16]byte,
	segmentID, spanID [8]byte,
	value telemetryResource,
) (telemetrySpan, bool) {
	usage, err := decodeUsage(attempt.usageBytes)
	if err != nil {
		return telemetrySpan{}, false
	}
	startedAt, endedAt, ok := dispatchSpanTiming(attempt, usage)
	if !ok {
		return telemetrySpan{}, false
	}
	return telemetrySpan{
		name:         dispatchSpanName,
		traceID:      traceID,
		spanID:       spanID,
		parentSpanID: segmentID,
		hasParent:    true,
		startedAt:    startedAt,
		endedAt:      endedAt,
		attributes:   dispatchSpanAttributes(record, attempt, identity, usage),
		resource:     value,
	}, true
}

func dispatchSpanTiming(
	attempt dispatchAttempt,
	usage driver.UsageReceipt,
) (time.Time, time.Time, bool) {
	endedAt := attempt.finishedAt
	if endedAt.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	if usage.DurationMillis != nil && *usage.DurationMillis > 0 {
		if *usage.DurationMillis > maxDispatchDurationMillis {
			// A canonical-but-pathological stamp would overflow the
			// conversion; skip rather than state a false duration.
			return time.Time{}, time.Time{}, false
		}
		startedAt := endedAt.Add(
			-time.Duration(*usage.DurationMillis) * time.Millisecond,
		)
		if startedAt.IsZero() || endedAt.Before(startedAt) {
			return time.Time{}, time.Time{}, false
		}
		return startedAt, endedAt, true
	}
	// Legacy receipts carry no duration stamp: fall back to the fact's
	// event-pair timing.
	if attempt.startedAt.IsZero() || endedAt.Before(attempt.startedAt) {
		return time.Time{}, time.Time{}, false
	}
	return attempt.startedAt, endedAt, true
}

// dispatchSpanAttributes is the closed sworn.dispatch vocabulary: the
// pinned GenAI semconv keys when usage was reported, the in-band status
// trio when it was not, cache/reasoning components only when reported, and
// the bounded sworn.* identity facts. No sworn.verdict exists anywhere, and
// no content-shaped value can ride: every attribute is a bounded enum, a
// bounded identity field, or a provider-stamped token count.
func dispatchSpanAttributes(
	record Record,
	attempt dispatchAttempt,
	identity dispatchIdentity,
	usage driver.UsageReceipt,
) []telemetryAttribute {
	result := []telemetryAttribute{
		stringTelemetryAttribute("sworn.run", record.RunID),
		stringTelemetryAttribute("sworn.release", record.Release),
		stringTelemetryAttribute(
			"sworn.role",
			roleForResponsibility(attempt.responsibility),
		),
		stringTelemetryAttribute(
			"sworn.responsibility",
			attempt.responsibility,
		),
		int64TelemetryAttribute("sworn.attempt", attempt.attempt),
	}
	if identity.hasEpochTry {
		result = append(
			result,
			int64TelemetryAttribute("sworn.epoch", identity.epoch),
			int64TelemetryAttribute("sworn.try", identity.try),
		)
	}
	if identity.hasSlice {
		result = append(
			result,
			stringTelemetryAttribute("sworn.slice", identity.slice),
		)
	}
	if identity.hasTrack {
		result = append(
			result,
			stringTelemetryAttribute("sworn.track", identity.track),
		)
	}
	result = append(
		result,
		stringTelemetryAttribute(
			"sworn.transport_status",
			attempt.transport,
		),
		stringTelemetryAttribute("sworn.outcome", attempt.effectState),
	)

	// A4: usage coverage rides in-band on every dispatch span. A defensive
	// clamp treats a reported receipt without its token pair as
	// unavailable rather than emitting a partial pair.
	tokenStatus := usage.TokenStatus
	if tokenStatus == driver.UsageReported &&
		(usage.InputTokens == nil || usage.OutputTokens == nil) {
		tokenStatus = driver.UsageUnavailable
	}
	if tokenStatus == driver.UsageReported {
		result = append(
			result,
			stringTelemetryAttribute(
				"sworn.usage_status",
				string(driver.UsageReported),
			),
		)
		if usage.Model != nil && *usage.Model != "" {
			result = append(
				result,
				stringTelemetryAttribute(
					"gen_ai.request.model",
					*usage.Model,
				),
			)
		}
		result = append(
			result,
			int64TelemetryAttribute(
				"gen_ai.usage.input_tokens",
				*usage.InputTokens,
			),
			int64TelemetryAttribute(
				"gen_ai.usage.output_tokens",
				*usage.OutputTokens,
			),
		)
	} else {
		surface := usage.Surface
		if surface == "" {
			surface = "unknown"
		}
		reason := usage.UnavailableReason
		if reason == "" {
			reason = "unknown"
		}
		result = append(
			result,
			stringTelemetryAttribute(
				"sworn.usage_status",
				string(driver.UsageUnavailable),
			),
			stringTelemetryAttribute("sworn.surface", surface),
			stringTelemetryAttribute(
				"sworn.usage_unavailable_reason",
				reason,
			),
		)
	}
	if usage.CacheStatus == driver.UsageReported {
		if usage.CacheReadTokens != nil {
			result = append(
				result,
				int64TelemetryAttribute(
					"sworn.usage.cache_read_tokens",
					*usage.CacheReadTokens,
				),
			)
		}
		if usage.CacheWriteTokens != nil {
			result = append(
				result,
				int64TelemetryAttribute(
					"sworn.usage.cache_write_tokens",
					*usage.CacheWriteTokens,
				),
			)
		}
	}
	if usage.ReasoningTokens != nil {
		result = append(
			result,
			int64TelemetryAttribute(
				"sworn.usage.reasoning_tokens",
				*usage.ReasoningTokens,
			),
		)
	}
	return result
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
