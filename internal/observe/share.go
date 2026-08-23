package observe

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"math"
	"sort"
	"strings"
)

const (
	// ShareSchemaVersion stamps every share-channel payload so the gateway
	// can double-enforce the same versioned table.
	ShareSchemaVersion = "sworn.share-schema/v1"
	// ShareSchemaAttribute is the attribute carrying that stamp.
	ShareSchemaAttribute = "sworn.share.schema"
)

// shareAttributeSet builds one allowlist column.
func shareAttributeSet(keys ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		result[key] = struct{}{}
	}
	return result
}

// shareMetricAttributes is the sworn.share-schema/v1 metric table: for every
// allowlisted metric name, the exact set of attribute keys that may cross the
// share channel. Fleet aggregates only - no group, role, model, transport,
// profile, token-count, or per-tool identity, and no attempt-level facts.
var shareMetricAttributes = map[string]map[string]struct{}{
	"sworn.eval.events":                          shareAttributeSet("sworn.outcome"),
	"sworn.eval.continuations":                   shareAttributeSet("sworn.continuation.mode", "sworn.continuation.outcome"),
	"sworn.eval.continuation.outcomes":           shareAttributeSet("sworn.continuation.outcome"),
	"sworn.eval.attempts":                        shareAttributeSet(),
	"sworn.eval.retries":                         shareAttributeSet(),
	"sworn.eval.recoveries":                      shareAttributeSet("sworn.recovery", "sworn.outcome"),
	"sworn.eval.turn_recovery.actions":           shareAttributeSet("sworn.turn_recovery.action"),
	"sworn.eval.turn_recovery.outcomes":          shareAttributeSet("sworn.turn_recovery.outcome"),
	"sworn.eval.duration_ns.numerator":           shareAttributeSet(),
	"sworn.eval.duration_ns.denominator":         shareAttributeSet(),
	"sworn.eval.usage_coverage.numerator":        shareAttributeSet(),
	"sworn.eval.usage_coverage.denominator":      shareAttributeSet(),
	"sworn.eval.cache_coverage.numerator":        shareAttributeSet(),
	"sworn.eval.cache_coverage.denominator":      shareAttributeSet(),
	"sworn.eval.turns":                           shareAttributeSet(),
	"sworn.eval.tool_calls":                      shareAttributeSet(),
	"sworn.eval.tool_calls_per_turn.numerator":   shareAttributeSet(),
	"sworn.eval.tool_calls_per_turn.denominator": shareAttributeSet(),
	"sworn.eval.quality.numerator":               shareAttributeSet("sworn.quality"),
	"sworn.eval.quality.denominator":             shareAttributeSet("sworn.quality"),
}

// shareSpanAttributes is the sworn.share-schema/v1 span table: only the two
// fleet spans, with non-identity aggregate attributes. sworn.dispatch is
// un-allowlisted and dropped whole, so every gen_ai.*, sworn.usage.*,
// sworn.model, sworn.transport, and per-attempt attribute is structurally
// absent from the share channel.
var shareSpanAttributes = map[string]map[string]struct{}{
	"sworn.process.segment": shareAttributeSet(
		"sworn.schema",
		"sworn.run.state",
		"sworn.outcome",
		"sworn.events",
		"sworn.attempts",
		"sworn.retries",
		"sworn.elapsed_ns",
		"sworn.usage_known",
		"sworn.measurement",
	),
	"sworn.recovery": shareAttributeSet(
		"sworn.measurement",
		"sworn.recovery.uncertain",
		"sworn.recovery.reconciled",
		"sworn.recovery.recovered",
		"sworn.recovery.rolled_back",
	),
}

// NewShareOTLP creates the opt-in share-channel projection. It is the only
// constructor that produces the filtered exporters, so an unfiltered share
// path cannot exist structurally: every trace and metric batch is rebuilt
// exclusively from the versioned allowlist tables. A disabled config yields
// the no-op telemetry without touching the network. Share trace and span
// identities are a per-host random salt applied to the private identities -
// stable per logical span for the life of the host, never derivable from run
// identifiers, and fail-closed: if the salt cannot be minted, nothing is
// exported.
func NewShareOTLP(
	ctx context.Context,
	config ShareConfig,
	version string,
) (*Telemetry, error) {
	if !config.Enabled {
		return Noop(), nil
	}
	if ctx == nil || !validVersion(version) {
		return nil, fail("INVALID_TELEMETRY")
	}
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = ShareDefaultEndpoint
	}
	canonical, traceURL, metricURL, err := canonicalConfig(Config{
		SchemaVersion: OTelConfigSchemaVersion,
		Endpoint:      endpoint,
		Headers:       config.Headers,
	})
	if err != nil {
		return nil, err
	}
	salt, err := newShareIdentitySalt()
	if err != nil {
		return nil, fail("SHARE_IDENTITY_UNAVAILABLE")
	}
	if ambientOTelConstructorBlocked() {
		return unavailableTelemetry("exporter_start"), nil
	}
	traceAdapter, metricAdapter, err := newOfficialOTLPAdapters(
		ctx,
		traceURL,
		metricURL,
		canonical.Headers,
		version,
		newOTelHTTPClient(),
		newOTelHTTPClient(),
	)
	if err != nil {
		return unavailableTelemetry("exporter_start"), nil
	}
	telemetry, err := newTelemetry(
		&shareTraceExporter{inner: traceAdapter, salt: salt},
		&shareMetricExporter{inner: metricAdapter},
		version,
		telemetryQueueCapacity,
		telemetryExportInterval,
	)
	if err != nil {
		shutdownOTLPAdapters(traceAdapter, metricAdapter)
		return unavailableTelemetry("exporter_start"), nil
	}
	return telemetry, nil
}

func newShareIdentitySalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// shareTraceExporter rebuilds each span exclusively from the versioned span
// table, stamps the schema, and re-identifies every span id (and the trace
// id) through the per-host salt. Parent links ride through the same salted
// mapping, so they stay consistent. A batch with nothing left to export is a
// no-op: the inner adapter rejects empty batches, and an emptied payload must
// never surface as a permanent share-channel failure.
type shareTraceExporter struct {
	inner telemetryTraceExporter
	salt  []byte
}

func (e *shareTraceExporter) ExportSpans(
	ctx context.Context,
	spans []telemetrySpan,
) error {
	if e == nil || e.inner == nil {
		return errOTLPAdapterPayload
	}
	filtered := make([]telemetrySpan, 0, len(spans))
	for _, span := range spans {
		allowed, ok := shareSpanAttributes[span.name]
		if !ok {
			continue
		}
		rebuilt := span
		rebuilt.attributes = filterTelemetryAttributes(
			span.attributes,
			allowed,
		)
		rebuilt.attributes = append(
			rebuilt.attributes,
			stringTelemetryAttribute(
				ShareSchemaAttribute,
				ShareSchemaVersion,
			),
		)
		rebuilt.traceID = shareTraceID(e.salt, span.traceID)
		rebuilt.spanID = shareSpanID(e.salt, span.spanID)
		if span.hasParent {
			rebuilt.parentSpanID = shareSpanID(e.salt, span.parentSpanID)
		}
		filtered = append(filtered, rebuilt)
	}
	if len(filtered) == 0 {
		return nil
	}
	return e.inner.ExportSpans(ctx, filtered)
}

func (e *shareTraceExporter) Shutdown(ctx context.Context) error {
	if e == nil || e.inner == nil {
		return nil
	}
	return e.inner.Shutdown(ctx)
}

// shareMetricExporter rebuilds each metric exclusively from the versioned
// metric table, stamps the schema, and merges points whose remaining
// attribute sets collide after stripping (C1): every share-table metric is
// an additive count or ratio, so the merged point carries the sum and the
// latest observedAt. Without the merge, a backend keeps one arbitrary
// last-wins point per collided series and the share numbers would be wrong.
// An emptied payload is a no-op (C2).
type shareMetricExporter struct {
	inner telemetryMetricExporter
}

func (e *shareMetricExporter) Export(
	ctx context.Context,
	value *telemetryMetricPayload,
) error {
	if e == nil || e.inner == nil {
		return errOTLPAdapterPayload
	}
	if value == nil {
		return errOTLPAdapterPayload
	}
	filtered := &telemetryMetricPayload{resource: value.resource}
	for _, metric := range value.metrics {
		allowed, ok := shareMetricAttributes[metric.name]
		if !ok {
			continue
		}
		points := mergeShareMetricPoints(metric.points, allowed)
		if len(points) == 0 {
			continue
		}
		for index := range points {
			points[index].attributes = append(
				points[index].attributes,
				stringTelemetryAttribute(
					ShareSchemaAttribute,
					ShareSchemaVersion,
				),
			)
		}
		filtered.metrics = append(filtered.metrics, telemetryMetric{
			name:   metric.name,
			points: points,
		})
	}
	if len(filtered.metrics) == 0 {
		return nil
	}
	return e.inner.Export(ctx, filtered)
}

func (e *shareMetricExporter) Shutdown(ctx context.Context) error {
	if e == nil || e.inner == nil {
		return nil
	}
	return e.inner.Shutdown(ctx)
}

func filterTelemetryAttributes(
	values []telemetryAttribute,
	allowed map[string]struct{},
) []telemetryAttribute {
	if len(values) == 0 {
		return nil
	}
	result := make([]telemetryAttribute, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[value.key]; ok {
			result = append(result, value)
		}
	}
	return result
}

// mergeShareMetricPoints strips each point to its allowlisted attributes,
// then deterministically merges points whose remaining attribute sets are
// equal. Summing is the semantically correct fleet aggregate for every v1
// share metric (all are additive counts or ratio numerators/denominators);
// the latest observedAt survives. Values saturate at MaxInt64 rather than
// overflowing.
func mergeShareMetricPoints(
	points []telemetryMetricPoint,
	allowed map[string]struct{},
) []telemetryMetricPoint {
	type mergedPoint struct {
		point telemetryMetricPoint
		key   string
	}
	merged := make(map[string]*mergedPoint)
	order := make([]string, 0, len(points))
	for _, point := range points {
		retained := filterTelemetryAttributes(point.attributes, allowed)
		sortTelemetryAttributes(retained)
		key := telemetryAttributeKey(retained)
		if existing, ok := merged[key]; ok {
			existing.point.value = saturatingTelemetryAdd(
				existing.point.value,
				point.value,
			)
			if point.observedAt.After(existing.point.observedAt) {
				existing.point.observedAt = point.observedAt
			}
			continue
		}
		merged[key] = &mergedPoint{
			point: telemetryMetricPoint{
				attributes: retained,
				observedAt: point.observedAt,
				value:      point.value,
			},
			key: key,
		}
		order = append(order, key)
	}
	result := make([]telemetryMetricPoint, 0, len(order))
	for _, key := range order {
		result = append(result, merged[key].point)
	}
	return result
}

func saturatingTelemetryAdd(left, right int64) int64 {
	if left >= math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}

// sortTelemetryAttributes gives the merge a canonical key independent of the
// builder's attribute order. The OTLP layer stores attributes as a set, so
// reordering never changes payload meaning.
func sortTelemetryAttributes(values []telemetryAttribute) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.key != right.key {
			return left.key < right.key
		}
		if left.kind != right.kind {
			return left.kind < right.kind
		}
		switch left.kind {
		case telemetryStringAttribute:
			return left.stringValue < right.stringValue
		case telemetryInt64Attribute:
			return left.int64Value < right.int64Value
		default:
			return left.boolValue && !right.boolValue
		}
	})
}

// shareTraceID and shareSpanID map one private identity through the per-host
// salt. The mapping is deterministic inside a host (so the cumulative
// segment span re-emitted on every advance keeps one stable share identity)
// and unpredictable across hosts (a fresh random salt per process), which
// keeps share identities underivable from run identifiers.
func shareTraceID(salt []byte, private [16]byte) [16]byte {
	sum := sha256.Sum256(append(append([]byte(nil), salt...), private[:]...))
	var result [16]byte
	copy(result[:], sum[:])
	if allZero(result[:]) {
		result[len(result)-1] = 1
	}
	return result
}

func shareSpanID(salt []byte, private [8]byte) [8]byte {
	sum := sha256.Sum256(append(append([]byte(nil), salt...), private[:]...))
	var result [8]byte
	copy(result[:], sum[:])
	if allZero(result[:]) {
		result[len(result)-1] = 1
	}
	return result
}

// shareAllowlistInvariantKeys names every allowlisted span name, metric name,
// and attribute key for the loud-guard tests. A future table edit that adds a
// content-shaped key fails the invariant test instead of shipping.
func shareAllowlistInvariantKeys() ([]string, []string) {
	var names []string
	var keys []string
	seenNames := make(map[string]struct{})
	seenKeys := make(map[string]struct{})
	add := func(name string, allowed map[string]struct{}) {
		if _, ok := seenNames[name]; !ok {
			seenNames[name] = struct{}{}
			names = append(names, name)
		}
		for key := range allowed {
			if _, ok := seenKeys[key]; !ok {
				seenKeys[key] = struct{}{}
				keys = append(keys, key)
			}
		}
	}
	for name, allowed := range shareSpanAttributes {
		add(name, allowed)
	}
	for name, allowed := range shareMetricAttributes {
		add(name, allowed)
	}
	sort.Strings(names)
	sort.Strings(keys)
	return names, keys
}

// shareAllowlistForbiddenSubstrings is the privacy vocabulary guard. No
// allowlisted name or attribute key may contain any of these (the invariant
// test enforces it case-insensitively on every table edit).
var shareAllowlistForbiddenSubstrings = []string{
	"prompt",
	"response",
	"reasoning",
	"session",
	"source",
}

func shareAllowlistViolatesInvariant(name string) bool {
	lower := strings.ToLower(name)
	for _, forbidden := range shareAllowlistForbiddenSubstrings {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return false
}

var _ telemetryTraceExporter = (*shareTraceExporter)(nil)
var _ telemetryMetricExporter = (*shareMetricExporter)(nil)
