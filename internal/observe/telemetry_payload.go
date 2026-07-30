package observe

import (
	"crypto/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

var telemetryMetricNames = []string{
	"sworn.eval.events",
	"sworn.eval.attempts",
	"sworn.eval.retries",
	"sworn.eval.recoveries",
	"sworn.eval.duration_ns.numerator",
	"sworn.eval.duration_ns.denominator",
	"sworn.eval.input_tokens",
	"sworn.eval.output_tokens",
	"sworn.eval.usage_coverage.numerator",
	"sworn.eval.usage_coverage.denominator",
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

func telemetrySpans(
	record Record,
	value telemetryResource,
) ([]telemetrySpan, error) {
	traceID, segmentID, recoveryID, err := telemetrySpanIDs()
	if err != nil {
		return nil, err
	}
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
	return spans, nil
}

func telemetrySpanIDs() ([16]byte, [8]byte, [8]byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return [16]byte{}, [8]byte{}, [8]byte{},
			fail("OTEL_TRACE_ID_FAILED")
	}
	var traceID [16]byte
	var segmentID, recoveryID [8]byte
	copy(traceID[:], raw[:16])
	copy(segmentID[:], raw[16:24])
	copy(recoveryID[:], raw[24:])
	if allZero(traceID[:]) {
		traceID[len(traceID)-1] = 1
	}
	if allZero(segmentID[:]) {
		segmentID[len(segmentID)-1] = 1
	}
	if allZero(recoveryID[:]) {
		recoveryID[len(recoveryID)-1] = 1
	}
	return traceID, segmentID, recoveryID, nil
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
