package observe

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type capturedSpan struct {
	name       string
	attributes map[string]any
	resource   map[string]any
	startedAt  time.Time
	endedAt    time.Time
	parent     bool
	events     int
	links      int
}

type captureTraceExporter struct {
	mu        sync.Mutex
	spans     []capturedSpan
	exports   int
	exportErr error
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
}

func (e *captureTraceExporter) ExportSpans(
	ctx context.Context,
	spans []telemetrySpan,
) error {
	if e.started != nil {
		e.startOnce.Do(func() { close(e.started) })
	}
	if e.release != nil {
		select {
		case <-e.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exports++
	for _, span := range spans {
		e.spans = append(e.spans, capturedSpan{
			name:       span.name,
			attributes: attributeMap(span.attributes),
			resource: resourceMap(
				span.resource,
			),
			startedAt: span.startedAt,
			endedAt:   span.endedAt,
			parent:    span.hasParent,
		})
	}
	return e.exportErr
}

func (e *captureTraceExporter) Shutdown(context.Context) error {
	return nil
}

func (e *captureTraceExporter) snapshot() ([]capturedSpan, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]capturedSpan(nil), e.spans...), e.exports
}

type capturedMetric struct {
	name       string
	attributes map[string]any
	value      int64
	resource   map[string]any
}

type captureMetricExporter struct {
	mu        sync.Mutex
	metrics   []capturedMetric
	exports   int
	exportErr error
}

func (e *captureMetricExporter) Export(
	_ context.Context,
	value *telemetryMetricPayload,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exports++
	resourceAttributes := resourceMap(value.resource)
	for _, metric := range value.metrics {
		for _, point := range metric.points {
			e.metrics = append(e.metrics, capturedMetric{
				name:       metric.name,
				attributes: attributeMap(point.attributes),
				value:      point.value,
				resource:   resourceAttributes,
			})
		}
	}
	return e.exportErr
}

func (*captureMetricExporter) Shutdown(context.Context) error {
	return nil
}

func (e *captureMetricExporter) snapshot() ([]capturedMetric, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]capturedMetric(nil), e.metrics...), e.exports
}

func attributeMap(values []telemetryAttribute) map[string]any {
	result := make(map[string]any, len(values))
	for _, value := range values {
		switch value.kind {
		case telemetryStringAttribute:
			result[value.key] = value.stringValue
		case telemetryInt64Attribute:
			result[value.key] = value.int64Value
		case telemetryBoolAttribute:
			result[value.key] = value.boolValue
		}
	}
	return result
}

func resourceMap(value telemetryResource) map[string]any {
	return map[string]any{
		"service.name":    value.serviceName,
		"service.version": value.serviceVersion,
	}
}

func testTelemetryRecord(sentinel string) Record {
	started := time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC)
	observed := started.Add(30 * time.Second)
	usage := UsageSummary{
		InputTokens:  int64Pointer(120),
		OutputTokens: int64Pointer(30),
		Costs: []CostTotal{{
			Currency:   "AUD",
			MicroUnits: 70,
		}},
		TokenCoverage: knownRatio(1, 2),
		CostCoverage:  knownRatio(1, 2),
	}
	return Record{
		SchemaVersion: EvalSchemaVersion,
		ID:            "eval-" + sentinel,
		SwornVersion:  "0.3.0-dev",
		RunID:         "run-" + sentinel,
		Release:       "release-" + sentinel,
		RunState:      "complete",
		Outcome:       "pass",
		ThroughOffset: 17,
		StartedAt:     started,
		ObservedAt:    observed,
		ElapsedNS:     observed.Sub(started).Nanoseconds(),
		Events:        8,
		Attempts:      2,
		Retries:       1,
		Recovery: RecoverySummary{
			Uncertain:  1,
			Reconciled: 1,
		},
		DurationNS: knownRatio(30, 2),
		Usage:      usage,
		Groups: []AttemptGroup{{
			Role:           "implementer",
			Responsibility: "implementer_implementation",
			Operation:      "driver.dispatch",
			Transport:      "completed",
			Outcome:        "succeeded",
			Attempts:       2,
			Retries:        1,
			DurationNS:     knownRatio(30, 2),
			Usage:          usage,
		}},
		Quality: []Quality{
			{Name: "delivery", Numerator: int64Pointer(1),
				Denominator: int64Pointer(1)},
			{Name: "integration", Numerator: int64Pointer(1),
				Denominator: int64Pointer(1)},
			{Name: "requirements"},
			{Name: "verification", Numerator: int64Pointer(1),
				Denominator: int64Pointer(1)},
		},
	}
}

func TestNoopTelemetryIsDisabledAndInert(t *testing.T) {
	t.Parallel()

	telemetry := Noop()
	if telemetry.TryEnqueue(testTelemetryRecord("private")) {
		t.Fatal("disabled telemetry accepted a record")
	}
	if status := telemetry.Status(); status.Enabled ||
		status.SchemaVersion != TelemetryStatusSchemaVersion ||
		status.QueueCapacity != 0 {
		t.Fatalf("status = %#v", status)
	}
	if err := telemetry.Shutdown(nil); err != nil {
		t.Fatalf("shutdown = %v", err)
	}
}

func TestTelemetryExportsOnlyThePositiveAllowlist(t *testing.T) {
	t.Parallel()

	const sentinel = "PRIVATE_PATH_CREDENTIAL_MODEL_RUN_ID"
	traceExporter := &captureTraceExporter{started: make(chan struct{})}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		4,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(testTelemetryRecord(sentinel)) {
		t.Fatal("record was not accepted")
	}
	select {
	case <-traceExporter.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start export")
	}
	shutdownTelemetry(t, telemetry)

	spans, traceExports := traceExporter.snapshot()
	if traceExports != 1 || len(spans) != 2 {
		t.Fatalf("spans = %#v, exports = %d", spans, traceExports)
	}
	sort.Slice(spans, func(left, right int) bool {
		return spans[left].name < spans[right].name
	})
	if spans[0].name != "sworn.process.segment" ||
		spans[1].name != "sworn.recovery" {
		t.Fatalf("span names = %q, %q", spans[0].name, spans[1].name)
	}
	allowedSpanAttributes := map[string]map[string]bool{
		"sworn.process.segment": stringSet(
			"sworn.schema",
			"sworn.measurement",
			"sworn.run.state",
			"sworn.outcome",
			"sworn.events",
			"sworn.attempts",
			"sworn.retries",
			"sworn.elapsed_ns",
			"sworn.usage_known",
			"sworn.input_tokens",
			"sworn.output_tokens",
		),
		"sworn.recovery": stringSet(
			"sworn.measurement",
			"sworn.recovery.uncertain",
			"sworn.recovery.reconciled",
			"sworn.recovery.recovered",
			"sworn.recovery.rolled_back",
		),
	}
	for _, span := range spans {
		assertExactKeys(t, span.attributes, allowedSpanAttributes[span.name])
		assertResource(t, span.resource)
		assertNoSentinel(t, sentinel, span.attributes)
		assertNoSentinel(t, sentinel, span.resource)
		if span.events != 0 || span.links != 0 {
			t.Fatalf("%s events/links = %d/%d", span.name, span.events, span.links)
		}
		if span.name == "sworn.process.segment" {
			if span.parent || !span.startedAt.Before(span.endedAt) {
				t.Fatalf("segment chronology = %#v", span)
			}
		} else if !span.parent || !span.startedAt.Equal(span.endedAt) {
			t.Fatalf("recovery chronology = %#v", span)
		}
	}

	metrics, metricExports := metricExporter.snapshot()
	if metricExports != 1 || len(metrics) == 0 {
		t.Fatalf("metrics = %#v, exports = %d", metrics, metricExports)
	}
	groupLabels := stringSet(
		"sworn.role",
		"sworn.responsibility",
		"sworn.operation",
		"sworn.transport",
		"sworn.outcome",
		"sworn.usage_known",
	)
	allowedMetricAttributes := map[string]map[string]bool{
		"sworn.eval.events":                     stringSet("sworn.outcome"),
		"sworn.eval.attempts":                   groupLabels,
		"sworn.eval.retries":                    groupLabels,
		"sworn.eval.recoveries":                 stringSet("sworn.recovery", "sworn.outcome"),
		"sworn.eval.duration_ns.numerator":      groupLabels,
		"sworn.eval.duration_ns.denominator":    groupLabels,
		"sworn.eval.input_tokens":               groupLabels,
		"sworn.eval.output_tokens":              groupLabels,
		"sworn.eval.usage_coverage.numerator":   groupLabels,
		"sworn.eval.usage_coverage.denominator": groupLabels,
		"sworn.eval.quality.numerator":          stringSet("sworn.quality"),
		"sworn.eval.quality.denominator":        stringSet("sworn.quality"),
	}
	seenNames := make(map[string]bool)
	for _, point := range metrics {
		allowed, found := allowedMetricAttributes[point.name]
		if !found {
			t.Fatalf("unknown metric %q", point.name)
		}
		seenNames[point.name] = true
		assertExactKeys(t, point.attributes, allowed)
		assertResource(t, point.resource)
		assertNoSentinel(t, sentinel, point.attributes)
		assertNoSentinel(t, sentinel, point.resource)
	}
	for name := range allowedMetricAttributes {
		if !seenNames[name] {
			t.Errorf("metric %q was not emitted", name)
		}
	}
	status := telemetry.Status()
	if status.Accepted != 1 || status.Processed != 1 ||
		status.Dropped != 0 || status.TraceExports != 1 ||
		status.MetricExports != 1 || status.Failures != 0 ||
		status.LastSuccessAt == nil || status.LastFailureAt != nil {
		t.Fatalf("status = %#v", status)
	}
}

func TestTelemetryBackpressureAndExporterFailureAreNonBlocking(
	t *testing.T,
) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	traceExporter := &captureTraceExporter{
		started: started,
		release: release,
	}
	metricExporter := &captureMetricExporter{
		exportErr: errors.New("metric unavailable"),
	}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		1,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(testTelemetryRecord("one")) {
		t.Fatal("first record was not accepted")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start export")
	}
	if !telemetry.TryEnqueue(testTelemetryRecord("two")) {
		t.Fatal("queue did not accept its bounded record")
	}
	began := time.Now()
	if telemetry.TryEnqueue(testTelemetryRecord("three")) {
		t.Fatal("full queue accepted a record")
	}
	if time.Since(began) > 250*time.Millisecond {
		t.Fatal("full queue blocked delivery")
	}
	close(release)
	shutdownTelemetry(t, telemetry)
	status := telemetry.Status()
	if status.Accepted != 2 || status.Processed != 1 ||
		status.Dropped != 2 || status.Failures != 1 ||
		status.LastFailureCode != "metric_export" {
		t.Fatalf("status = %#v", status)
	}
}

func TestTelemetryRejectsMalformedRecordsWithoutPanicking(t *testing.T) {
	t.Parallel()

	traceExporter := &captureTraceExporter{}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		2,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []Record{
		func() Record {
			value := testTelemetryRecord("bad-state")
			value.RunState = "/private/path"
			return value
		}(),
		func() Record {
			value := testTelemetryRecord("bad-ratio")
			value.Groups[0].DurationNS.Numerator = nil
			return value
		}(),
		func() Record {
			value := testTelemetryRecord("bad-label")
			value.Groups[0].Transport = "provider-error-secret"
			return value
		}(),
	}
	for _, record := range tests {
		if telemetry.TryEnqueue(record) {
			t.Fatalf("malformed record accepted: %#v", record)
		}
	}
	shutdownTelemetry(t, telemetry)
	if status := telemetry.Status(); status.Accepted != 0 ||
		status.Processed != 0 || status.Failures != 0 {
		t.Fatalf("status = %#v", status)
	}
}

func shutdownTelemetry(t *testing.T, telemetry *Telemetry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := telemetry.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown = %v", err)
	}
}

func stringSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func assertExactKeys(
	t *testing.T,
	values map[string]any,
	allowed map[string]bool,
) {
	t.Helper()
	if len(values) != len(allowed) {
		t.Fatalf("attribute keys = %#v, want %#v", values, allowed)
	}
	for key := range values {
		if !allowed[key] {
			t.Fatalf("attribute %q is not allowlisted", key)
		}
	}
}

func assertResource(t *testing.T, values map[string]any) {
	t.Helper()
	assertExactKeys(
		t,
		values,
		stringSet("service.name", "service.version"),
	)
	if values["service.name"] != "sworn" ||
		values["service.version"] != "0.3.0-dev" {
		t.Fatalf("resource = %#v", values)
	}
}

func assertNoSentinel(
	t *testing.T,
	sentinel string,
	values map[string]any,
) {
	t.Helper()
	for key, value := range values {
		if strings.Contains(key, sentinel) ||
			strings.Contains(fmt.Sprint(value), sentinel) {
			t.Fatalf("sentinel escaped in %q=%v", key, value)
		}
	}
}
