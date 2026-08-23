package observe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestParseShareConfigCanonicalizesEnabledBlocks(t *testing.T) {
	t.Parallel()

	disabled := ShareConfig{
		SchemaVersion: ShareConfigSchemaVersion,
		Enabled:       false,
		Endpoint:      "",
	}
	body, err := jsonMarshal(disabled)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseShareConfig(body)
	if err != nil || parsed.Enabled {
		t.Fatalf("disabled share = %#v, %v", parsed, err)
	}

	enabled := ShareConfig{
		SchemaVersion: ShareConfigSchemaVersion,
		Enabled:       true,
		Endpoint:      "",
		Headers:       map[string]string{"x-tenant": "fleet"},
	}
	body, err = jsonMarshal(enabled)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = ParseShareConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Enabled || parsed.Endpoint != ShareDefaultEndpoint ||
		parsed.Headers["X-Tenant"] != "fleet" {
		t.Fatalf("enabled default share = %#v", parsed)
	}

	overridden := ShareConfig{
		SchemaVersion: ShareConfigSchemaVersion,
		Enabled:       true,
		Endpoint:      "http://127.0.0.1:4318/collector",
		Headers:       map[string]string{"Authorization": "share"},
	}
	body, err = jsonMarshal(overridden)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = ParseShareConfig(body)
	if err != nil || parsed.Endpoint != "http://127.0.0.1:4318/collector" ||
		parsed.Headers["Authorization"] != "share" {
		t.Fatalf("overridden share = %#v, %v", parsed, err)
	}

	for name, invalid := range map[string]ShareConfig{
		"schema": {
			SchemaVersion: "sworn.otel-config/v1",
			Enabled:       true,
			Endpoint:      ShareDefaultEndpoint,
		},
		"endpoint": {
			SchemaVersion: ShareConfigSchemaVersion,
			Enabled:       true,
			Endpoint:      "http://example.invalid/collector",
		},
		"header": {
			SchemaVersion: ShareConfigSchemaVersion,
			Enabled:       true,
			Endpoint:      ShareDefaultEndpoint,
			Headers:       map[string]string{"Host": "spoofed"},
		},
		"oversize": {
			SchemaVersion: ShareConfigSchemaVersion,
			Enabled:       true,
			Endpoint:      ShareDefaultEndpoint,
			Headers: map[string]string{
				"X-Long": strings.Repeat("x", 1025),
			},
		},
	} {
		body, err := jsonMarshal(invalid)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseShareConfig(body); err == nil {
			t.Fatalf("%s share config admitted: %s", name, body)
		}
	}
}

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

type recordingTraceExporter struct {
	mu     sync.Mutex
	spans  [][]telemetrySpan
	calls  int
	closed bool
}

func (r *recordingTraceExporter) ExportSpans(
	_ context.Context,
	spans []telemetrySpan,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.spans = append(r.spans, append([]telemetrySpan(nil), spans...))
	return nil
}

func (r *recordingTraceExporter) Shutdown(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

type recordingMetricExporter struct {
	mu      sync.Mutex
	payload []*telemetryMetricPayload
	calls   int
	closed  bool
}

func (r *recordingMetricExporter) Export(
	_ context.Context,
	value *telemetryMetricPayload,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.payload = append(r.payload, value)
	return nil
}

func (r *recordingMetricExporter) Shutdown(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

func TestShareFilterDropsHostileAttributesStructurally(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC)
	record := testTelemetryRecord("hostile-share")
	hostileSpan := telemetrySpan{
		name:      "sworn.process.segment",
		traceID:   runTraceID(record.RunID),
		spanID:    spanIDFor(record.RunID, "segment", ""),
		startedAt: startedAt,
		endedAt:   startedAt.Add(time.Second),
		attributes: []telemetryAttribute{
			stringTelemetryAttribute("sworn.schema", "sworn.eval/v3"),
			stringTelemetryAttribute("sworn.measurement", "cumulative"),
			stringTelemetryAttribute("sworn.run.state", "complete"),
			stringTelemetryAttribute("sworn.outcome", "pass"),
			// Hostile content-shaped keys that must never cross share.
			stringTelemetryAttribute("sworn.prompt", "PRIVATE PROMPT"),
			stringTelemetryAttribute("sworn.response", "PRIVATE RESPONSE"),
			stringTelemetryAttribute("sworn.reasoning", "PRIVATE REASONING"),
			stringTelemetryAttribute("sworn.session-id", "PRIVATE SESSION"),
			stringTelemetryAttribute("sworn.source_content", "PRIVATE SOURCE"),
		},
		resource: testTelemetryResource(),
	}
	dispatchSpan := telemetrySpan{
		name:      "sworn.dispatch",
		traceID:   runTraceID(record.RunID),
		spanID:    spanIDFor(record.RunID, "dispatch", "x"),
		startedAt: startedAt,
		endedAt:   startedAt.Add(time.Second),
		attributes: []telemetryAttribute{
			stringTelemetryAttribute("gen_ai.request.model", "PRIVATE MODEL"),
			stringTelemetryAttribute("sworn.run", record.RunID),
			stringTelemetryAttribute("sworn.prompt", "PRIVATE PROMPT"),
		},
		resource: testTelemetryResource(),
	}
	hostileMetric := telemetryMetric{
		name: "sworn.eval.events",
		points: []telemetryMetricPoint{{
			attributes: []telemetryAttribute{
				stringTelemetryAttribute("sworn.outcome", "pass"),
				stringTelemetryAttribute("sworn.prompt", "PRIVATE PROMPT"),
				stringTelemetryAttribute("sworn.response", "PRIVATE RESPONSE"),
				stringTelemetryAttribute("sworn.reasoning_tokens", "PRIVATE"),
			},
			observedAt: startedAt,
			value:      1,
		}},
	}

	innerTrace := &recordingTraceExporter{}
	innerMetric := &recordingMetricExporter{}
	shareTrace := &shareTraceExporter{inner: innerTrace, salt: make([]byte, 16)}
	shareMetric := &shareMetricExporter{inner: innerMetric}

	if err := shareTrace.ExportSpans(
		context.Background(),
		[]telemetrySpan{hostileSpan, dispatchSpan},
	); err != nil {
		t.Fatal(err)
	}
	if err := shareMetric.Export(
		context.Background(),
		&telemetryMetricPayload{
			resource: testTelemetryResource(),
			metrics:  []telemetryMetric{hostileMetric},
		},
	); err != nil {
		t.Fatal(err)
	}

	innerTrace.mu.Lock()
	if innerTrace.calls != 1 || len(innerTrace.spans) != 1 ||
		len(innerTrace.spans[0]) != 1 {
		innerTrace.mu.Unlock()
		t.Fatalf("share trace export = %#v", innerTrace)
	}
	exported := innerTrace.spans[0][0]
	innerTrace.mu.Unlock()
	if exported.name != "sworn.process.segment" {
		t.Fatalf("share span name = %q", exported.name)
	}
	for _, attribute := range exported.attributes {
		if shareAllowlistViolatesInvariant(attribute.key) ||
			strings.Contains(strings.ToLower(attribute.stringValue), "private") {
			t.Fatalf("hostile attribute crossed share: %#v", attribute)
		}
	}
	foundSchema := false
	for _, attribute := range exported.attributes {
		if attribute.key == ShareSchemaAttribute &&
			attribute.stringValue == ShareSchemaVersion {
			foundSchema = true
		}
	}
	if !foundSchema {
		t.Fatalf("share span lacks schema stamp: %#v", exported.attributes)
	}

	innerMetric.mu.Lock()
	defer innerMetric.mu.Unlock()
	if innerMetric.calls != 1 || len(innerMetric.payload) != 1 ||
		len(innerMetric.payload[0].metrics) != 1 ||
		len(innerMetric.payload[0].metrics[0].points) != 1 {
		t.Fatalf("share metric export = %#v", innerMetric.payload)
	}
	point := innerMetric.payload[0].metrics[0].points[0]
	for _, attribute := range point.attributes {
		if shareAllowlistViolatesInvariant(attribute.key) {
			t.Fatalf("hostile attribute crossed share metrics: %#v", attribute)
		}
	}
}

func TestShareAllowlistInvariantRejectsContentShapedKeys(t *testing.T) {
	t.Parallel()

	names, keys := shareAllowlistInvariantKeys()
	if len(names) == 0 || len(keys) == 0 {
		t.Fatal("empty share allowlist table")
	}
	for _, name := range names {
		if shareAllowlistViolatesInvariant(name) {
			t.Fatalf("allowlisted name violates privacy invariant: %q", name)
		}
	}
	for _, key := range keys {
		if shareAllowlistViolatesInvariant(key) {
			t.Fatalf("allowlisted attribute violates privacy invariant: %q", key)
		}
	}
	// The pinned v1 table: the two fleet spans and exactly the non-identity
	// aggregate metrics. Token metrics, by-name tool metrics, and the
	// dispatch span stay structurally absent.
	for _, forbidden := range []string{
		"sworn.dispatch",
		"sworn.eval.input_tokens",
		"sworn.eval.output_tokens",
		"sworn.eval.cache_read_tokens",
		"sworn.eval.cache_write_tokens",
		"sworn.eval.reasoning_tokens",
		"sworn.eval.tool_calls.by_name",
		"sworn.eval.observation_duration_ns.numerator",
	} {
		if _, ok := shareSpanAttributes[forbidden]; ok {
			t.Fatalf("forbidden span leaked into allowlist: %q", forbidden)
		}
		if _, ok := shareMetricAttributes[forbidden]; ok {
			t.Fatalf("forbidden metric leaked into allowlist: %q", forbidden)
		}
	}
}

func TestShareIdentityIsSaltedStablePerHostAndNeverPrivate(t *testing.T) {
	t.Parallel()

	record := testTelemetryRecord("share-identity")
	privateTrace := runTraceID(record.RunID)
	privateSegment := spanIDFor(record.RunID, "segment", "")

	saltA := []byte("salt-a-16-bytes")
	saltB := []byte("salt-b-16-bytes")
	traceA := shareTraceID(saltA, privateTrace)
	traceB := shareTraceID(saltB, privateTrace)
	spanA1 := shareSpanID(saltA, privateSegment)
	spanA2 := shareSpanID(saltA, privateSegment)
	spanB := shareSpanID(saltB, privateSegment)

	if traceA == traceB || traceA == privateTrace || traceB == privateTrace {
		t.Fatalf("share trace identity derivable or collided: %x %x %x",
			privateTrace, traceA, traceB)
	}
	if spanA1 != spanA2 {
		t.Fatal("share span identity is not stable within one host")
	}
	if spanA1 == spanB || spanA1 == privateSegment || spanB == privateSegment {
		t.Fatalf("share span identity derivable or collided: %x %x %x",
			privateSegment, spanA1, spanB)
	}
}

func TestShareMetricFilterMergesCollidedPointsDeterministically(t *testing.T) {
	t.Parallel()

	labelsA := []telemetryAttribute{
		stringTelemetryAttribute("sworn.role", "implementer"),
		stringTelemetryAttribute("sworn.model", "model-a"),
		stringTelemetryAttribute("sworn.outcome", "succeeded"),
	}
	labelsB := []telemetryAttribute{
		stringTelemetryAttribute("sworn.role", "verifier"),
		stringTelemetryAttribute("sworn.model", "model-b"),
		stringTelemetryAttribute("sworn.outcome", "succeeded"),
	}
	earlier := time.Date(2026, 7, 29, 1, 2, 0, 0, time.UTC)
	later := earlier.Add(time.Minute)
	exporter := &shareMetricExporter{inner: &recordingMetricExporter{}}
	payload := &telemetryMetricPayload{
		resource: testTelemetryResource(),
		metrics: []telemetryMetric{{
			name: "sworn.eval.attempts",
			points: []telemetryMetricPoint{
				{attributes: labelsA, observedAt: earlier, value: 3},
				{attributes: labelsB, observedAt: later, value: 5},
			},
		}},
	}
	if err := exporter.Export(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	inner := exporter.inner.(*recordingMetricExporter)
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.calls != 1 || len(inner.payload) != 1 ||
		len(inner.payload[0].metrics) != 1 ||
		len(inner.payload[0].metrics[0].points) != 1 {
		t.Fatalf("merged payload = %#v", inner.payload)
	}
	point := inner.payload[0].metrics[0].points[0]
	if point.value != 8 || !point.observedAt.Equal(later) {
		t.Fatalf("merged point = %#v", point)
	}
	keys := make([]string, 0, len(point.attributes))
	schemaStamped := false
	for _, attribute := range point.attributes {
		if attribute.key == ShareSchemaAttribute &&
			attribute.stringValue == ShareSchemaVersion {
			schemaStamped = true
			continue
		}
		keys = append(keys, attribute.key)
	}
	if len(keys) != 0 {
		t.Fatalf("group labels survived stripping: %v", keys)
	}
	if !schemaStamped {
		t.Fatalf("merged point lacks schema stamp: %#v", point.attributes)
	}
}

func TestShareExportIsNoOpWhenEverythingFiltersOut(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC)
	record := testTelemetryRecord("share-empty")
	dispatchOnly := []telemetrySpan{{
		name:      "sworn.dispatch",
		traceID:   runTraceID(record.RunID),
		spanID:    spanIDFor(record.RunID, "dispatch", "y"),
		startedAt: startedAt,
		endedAt:   startedAt.Add(time.Second),
		attributes: []telemetryAttribute{
			stringTelemetryAttribute("gen_ai.request.model", "model"),
		},
		resource: testTelemetryResource(),
	}}
	nonAllowlistedMetric := &telemetryMetricPayload{
		resource: testTelemetryResource(),
		metrics: []telemetryMetric{{
			name: "sworn.eval.input_tokens",
			points: []telemetryMetricPoint{{
				observedAt: startedAt,
				value:      120,
			}},
		}},
	}

	innerTrace := &recordingTraceExporter{}
	innerMetric := &recordingMetricExporter{}
	shareTrace := &shareTraceExporter{inner: innerTrace, salt: make([]byte, 16)}
	shareMetric := &shareMetricExporter{inner: innerMetric}

	if err := shareTrace.ExportSpans(context.Background(), dispatchOnly); err != nil {
		t.Fatalf("emptied trace payload errored: %v", err)
	}
	if err := shareMetric.Export(context.Background(), nonAllowlistedMetric); err != nil {
		t.Fatalf("emptied metric payload errored: %v", err)
	}
	innerTrace.mu.Lock()
	traceCalls := innerTrace.calls
	innerTrace.mu.Unlock()
	innerMetric.mu.Lock()
	metricCalls := innerMetric.calls
	innerMetric.mu.Unlock()
	if traceCalls != 0 || metricCalls != 0 {
		t.Fatalf("inner exporters called on emptied payloads: %d %d",
			traceCalls, metricCalls)
	}
}

type failingTraceExporter struct{ calls int }

func (f *failingTraceExporter) ExportSpans(context.Context, []telemetrySpan) error {
	f.calls++
	return errors.New("collector down")
}

func (f *failingTraceExporter) Shutdown(context.Context) error { return nil }

func TestShareTraceExportPassesInnerFailuresThrough(t *testing.T) {
	record := testTelemetryRecord("share-failure")
	startedAt := time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC)
	span := telemetrySpan{
		name:      "sworn.process.segment",
		traceID:   runTraceID(record.RunID),
		spanID:    spanIDFor(record.RunID, "segment", ""),
		startedAt: startedAt,
		endedAt:   startedAt.Add(time.Second),
		attributes: []telemetryAttribute{
			stringTelemetryAttribute("sworn.schema", "sworn.eval/v3"),
		},
		resource: testTelemetryResource(),
	}
	inner := &failingTraceExporter{}
	exporter := &shareTraceExporter{inner: inner, salt: make([]byte, 16)}
	if err := exporter.ExportSpans(context.Background(), []telemetrySpan{span}); err == nil {
		t.Fatal("inner failure swallowed")
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls = %d", inner.calls)
	}
}

// TestShareOTLPWireCarriesOnlyAllowlistedNamesAndAttributes drives a real
// NewShareOTLP instance against a fixture sink and decodes the protobuf: only
// allowlisted span names and attribute keys may cross, every span and point
// carries the schema stamp, and the dispatch span is absent even though the
// private record carries attempts.
func TestShareOTLPWireCarriesOnlyAllowlistedNamesAndAttributes(t *testing.T) {
	clearBlockedOTelConstructorEnvironment(t)
	var captured struct {
		mu      sync.Mutex
		traces  [][]byte
		metrics [][]byte
	}
	sink := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		defer request.Body.Close()
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read OTLP request: %v", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		captured.mu.Lock()
		switch request.URL.Path {
		case "/v1/traces":
			captured.traces = append(captured.traces, body)
		case "/v1/metrics":
			captured.metrics = append(captured.metrics, body)
		}
		captured.mu.Unlock()
		writer.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	telemetry, err := NewShareOTLP(
		context.Background(),
		ShareConfig{
			SchemaVersion: ShareConfigSchemaVersion,
			Enabled:       true,
			Endpoint:      sink.URL,
			Headers:       map[string]string{},
		},
		"0.3.0-dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	record := testTelemetryRecord("share-wire")
	startedAt := record.StartedAt.Add(time.Second)
	finishedAt := startedAt.Add(500 * time.Millisecond)
	record.dispatchAttempts = []dispatchAttempt{testDispatchAttempt(
		t,
		dispatchEffectKind,
		"succeeded",
		"implementer_implementation",
		1,
		"completed",
		startedAt,
		finishedAt,
		testReportedReceipt(),
	)}
	if !telemetry.TryEnqueue(record) {
		t.Fatal("record was not accepted")
	}
	waitForProcessed(t, telemetry, 1)
	shutdownTelemetry(t, telemetry)

	captured.mu.Lock()
	defer captured.mu.Unlock()
	if len(captured.traces) == 0 || len(captured.metrics) == 0 {
		t.Fatalf("share requests: traces=%d metrics=%d",
			len(captured.traces), len(captured.metrics))
	}

	var traceNames []string
	spanNames, spanKeys := shareAllowlistInvariantKeys()
	for _, body := range captured.traces {
		var request collectortracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode share trace request: %v", err)
		}
		for _, resourceSpans := range request.ResourceSpans {
			for _, scope := range resourceSpans.ScopeSpans {
				for _, span := range scope.Spans {
					if !containsString(spanNames, span.Name) {
						t.Fatalf("non-allowlisted span crossed share: %q", span.Name)
					}
					traceNames = append(traceNames, span.Name)
					attributes := protoAttributeMap(t, span.Attributes)
					for key := range attributes {
						if !containsString(spanKeys, key) &&
							key != ShareSchemaAttribute {
							t.Fatalf("non-allowlisted attribute crossed share: %q", key)
						}
					}
					if attributes[ShareSchemaAttribute] != ShareSchemaVersion {
						t.Fatalf("share span lacks schema stamp: %#v", attributes)
					}
				}
			}
		}
	}
	if containsString(traceNames, "sworn.dispatch") {
		t.Fatal("dispatch span crossed the share channel")
	}
	if !containsString(traceNames, "sworn.process.segment") {
		t.Fatalf("segment span missing from share channel: %v", traceNames)
	}

	metricNames, metricKeys := shareAllowlistInvariantKeys()
	for _, body := range captured.metrics {
		var request collectormetricspb.ExportMetricsServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode share metric request: %v", err)
		}
		for _, resourceMetrics := range request.ResourceMetrics {
			for _, scope := range resourceMetrics.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if !containsString(metricNames, metric.Name) {
						t.Fatalf("non-allowlisted metric crossed share: %q", metric.Name)
					}
					for _, point := range metric.GetGauge().GetDataPoints() {
						attributes := protoAttributeMap(t, point.Attributes)
						for key := range attributes {
							if !containsString(metricKeys, key) &&
								key != ShareSchemaAttribute {
								t.Fatalf("non-allowlisted metric attribute crossed share: %q", key)
							}
						}
						if attributes[ShareSchemaAttribute] != ShareSchemaVersion {
							t.Fatalf("share point lacks schema stamp: %#v", attributes)
						}
					}
				}
			}
		}
	}
}

func containsString(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}
