package observe

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type otlpRequest struct {
	path            string
	method          string
	explicitHeader  string
	ambientHeader   string
	contentEncoding string
	body            []byte
}

func TestNewOTLPOverridesAmbientEnvironmentAndExcludesIdentities(
	t *testing.T,
) {
	clearBlockedOTelConstructorEnvironment(t)
	const sentinel = "PRIVATE_OTEL_ENV_RUN_ID_CREDENTIAL"
	var ambientRequests atomic.Int64
	ambient := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		ambientRequests.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer ambient.Close()

	var captureMu sync.Mutex
	var captures []otlpRequest
	explicit := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		defer request.Body.Close()
		body, err := io.ReadAll(io.LimitReader(
			request.Body,
			otelMaxRequestBytes+1,
		))
		if err != nil {
			t.Errorf("read OTLP request: %v", err)
		}
		captureMu.Lock()
		captures = append(captures, otlpRequest{
			path:            request.URL.Path,
			method:          request.Method,
			explicitHeader:  request.Header.Get("X-Sworn-Tenant"),
			ambientHeader:   request.Header.Get("X-Ambient-Secret"),
			contentEncoding: request.Header.Get("Content-Encoding"),
			body:            body,
		})
		captureMu.Unlock()
		time.Sleep(20 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	defer explicit.Close()

	for name, value := range map[string]string{
		"OTEL_SDK_DISABLED":                                 "true",
		"OTEL_SERVICE_NAME":                                 sentinel,
		"OTEL_RESOURCE_ATTRIBUTES":                          "private=" + sentinel,
		"OTEL_EXPORTER_OTLP_ENDPOINT":                       ambient.URL,
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT":                ambient.URL + "/traces",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT":               ambient.URL + "/metrics",
		"OTEL_EXPORTER_OTLP_HEADERS":                        "X-Ambient-Secret=" + sentinel,
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS":                 "X-Ambient-Secret=" + sentinel,
		"OTEL_EXPORTER_OTLP_METRICS_HEADERS":                "X-Ambient-Secret=" + sentinel,
		"OTEL_EXPORTER_OTLP_COMPRESSION":                    "gzip",
		"OTEL_EXPORTER_OTLP_TRACES_COMPRESSION":             "gzip",
		"OTEL_EXPORTER_OTLP_METRICS_COMPRESSION":            "gzip",
		"OTEL_EXPORTER_OTLP_TIMEOUT":                        "1",
		"OTEL_EXPORTER_OTLP_TRACES_TIMEOUT":                 "1",
		"OTEL_EXPORTER_OTLP_METRICS_TIMEOUT":                "1",
		"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE": "delta",
	} {
		t.Setenv(name, value)
	}
	telemetry, err := NewOTLP(
		context.Background(),
		Config{
			SchemaVersion: OTelConfigSchemaVersion,
			Endpoint:      explicit.URL + "/collector",
			Headers: map[string]string{
				"X-Sworn-Tenant": "tenant-a",
			},
		},
		"0.3.0-dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(testTelemetryRecord(sentinel)) {
		t.Fatal("record was not accepted")
	}
	waitForProcessed(t, telemetry, 1)
	shutdownTelemetry(t, telemetry)

	captureMu.Lock()
	got := append([]otlpRequest(nil), captures...)
	captureMu.Unlock()
	sort.Slice(got, func(left, right int) bool {
		return got[left].path < got[right].path
	})
	if len(got) != 2 ||
		got[0].path != "/collector/v1/metrics" ||
		got[1].path != "/collector/v1/traces" {
		t.Fatalf("requests = %#v", got)
	}
	for _, request := range got {
		if request.method != http.MethodPost ||
			request.explicitHeader != "tenant-a" ||
			request.ambientHeader != "" ||
			request.contentEncoding != "" ||
			len(request.body) == 0 ||
			len(request.body) > otelMaxRequestBytes ||
			bytes.Contains(request.body, []byte(sentinel)) {
			t.Fatalf("unsafe OTLP request = %#v", request)
		}
	}
	assertTraceWirePayload(t, got[1].body)
	assertMetricWirePayload(t, got[0].body)
	if ambientRequests.Load() != 0 {
		t.Fatalf("ambient endpoint received %d requests", ambientRequests.Load())
	}
	status := telemetry.Status()
	if status.TraceExports != 1 || status.MetricExports != 1 ||
		status.Failures != 0 {
		t.Fatalf("status = %#v", status)
	}
}

func TestOTLPAdaptersUseOfficialHTTPExportersAndPinnedModules(
	t *testing.T,
) {
	clearBlockedOTelConstructorEnvironment(t)
	transport := &staticOTLPRoundTripper{
		responses: map[string]staticOTLPResponse{
			"/v1/traces":  {status: http.StatusOK},
			"/v1/metrics": {status: http.StatusOK},
		},
		calls: make(map[string]int),
	}
	client := func() *http.Client {
		return &http.Client{
			Transport: transport,
			Timeout:   telemetryExportTimeout,
			CheckRedirect: func(
				*http.Request,
				[]*http.Request,
			) error {
				return errOTelRedirect
			},
		}
	}
	traceAdapter, metricAdapter, err := newOfficialOTLPAdapters(
		context.Background(),
		"http://127.0.0.1:4318/v1/traces",
		"http://127.0.0.1:4318/v1/metrics",
		map[string]string{},
		"0.3.0-dev",
		client(),
		client(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownOTLPAdapters(traceAdapter, metricAdapter)

	var officialTrace *otlptrace.Exporter = traceAdapter.exporter
	var officialMetric *otlpmetrichttp.Exporter = metricAdapter.exporter
	if officialTrace == nil || officialMetric == nil {
		t.Fatal("official exporter is nil")
	}
	if got := officialMetric.Temporality(
		sdkmetric.InstrumentKindCounter,
	); got != metricdata.CumulativeTemporality {
		t.Fatalf("metric temporality = %v", got)
	}
	if _, ok := officialMetric.Aggregation(
		sdkmetric.InstrumentKindCounter,
	).(sdkmetric.AggregationSum); !ok {
		t.Fatalf(
			"metric aggregation = %T",
			officialMetric.Aggregation(sdkmetric.InstrumentKindCounter),
		)
	}

	module, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, dependency := range []string{
		"go.opentelemetry.io/otel/exporters/otlp/" +
			"otlptrace/otlptracehttp v1.44.0",
		"go.opentelemetry.io/otel/exporters/otlp/" +
			"otlpmetric/otlpmetrichttp v1.44.0",
	} {
		if !bytes.Contains(module, []byte("\t"+dependency+"\n")) {
			t.Fatalf("go.mod has no direct %s requirement", dependency)
		}
	}
}

func TestOfficialExporterStartupFailureIsLocalAndNonControlling(
	t *testing.T,
) {
	tests := map[string]string{
		"OTEL_EXPORTER_OTLP_CERTIFICATE":                "unread-ca",
		"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE":         "unread-cert",
		"OTEL_EXPORTER_OTLP_CLIENT_KEY":                 "unread-key",
		"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE":         "unread-trace-ca",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE":  "unread-trace-cert",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY":          "unread-trace-key",
		"OTEL_EXPORTER_OTLP_METRICS_CERTIFICATE":        "unread-metric-ca",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_CERTIFICATE": "unread-metric-cert",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_KEY":         "unread-metric-key",
		"OTEL_GO_X_OBSERVABILITY":                       " TrUe ",
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			for environmentName := range tests {
				t.Setenv(environmentName, "")
			}
			t.Setenv(name, value)
			if !ambientOTelConstructorBlocked() {
				t.Fatalf("%s did not block official constructor", name)
			}
			telemetry, err := NewOTLP(
				context.Background(),
				Config{
					SchemaVersion: OTelConfigSchemaVersion,
					Endpoint:      "http://127.0.0.1:4318",
					Headers:       map[string]string{},
				},
				"0.3.0-dev",
			)
			if err != nil {
				t.Fatalf("exporter startup controlled caller: %v", err)
			}
			assertUnavailableTelemetry(t, telemetry)
		})
	}
}

func assertUnavailableTelemetry(t *testing.T, telemetry *Telemetry) {
	t.Helper()
	status := telemetry.Status()
	if !status.Enabled || status.QueueCapacity != 0 ||
		status.QueueDepth != 0 || status.Failures != 1 ||
		status.LastFailureAt == nil ||
		status.LastFailureCode != "exporter_start" {
		t.Fatalf("startup failure status = %#v", status)
	}
	started := time.Now()
	if telemetry.TryEnqueue(testTelemetryRecord("startup-failure")) {
		t.Fatal("unavailable exporter accepted telemetry")
	}
	if time.Since(started) > 250*time.Millisecond {
		t.Fatal("unavailable exporter blocked delivery")
	}
	if status := telemetry.Status(); status.Dropped != 1 ||
		status.Failures != 1 ||
		status.LastFailureCode != "exporter_start" {
		t.Fatalf("post-drop status = %#v", status)
	}
	for range 2 {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			100*time.Millisecond,
		)
		if err := telemetry.Shutdown(ctx); err != nil {
			cancel()
			t.Fatalf("unavailable shutdown = %v", err)
		}
		cancel()
	}
}

type boundedRequestRoundTripper struct {
	mu       sync.Mutex
	requests []otlpRequest
}

func (r *boundedRequestRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	defer request.Body.Close()
	body, err := io.ReadAll(io.LimitReader(
		request.Body,
		otelMaxRequestBytes+1,
	))
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.requests = append(r.requests, otlpRequest{
		path:            request.URL.Path,
		method:          request.Method,
		contentEncoding: request.Header.Get("Content-Encoding"),
		body:            body,
	})
	r.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    request,
	}, nil
}

func TestOfficialMetricExporterBoundsMaximumClosedCardinality(
	t *testing.T,
) {
	clearBlockedOTelConstructorEnvironment(t)
	transport := &boundedRequestRoundTripper{}
	client := func() *http.Client {
		return &http.Client{
			Transport: transport,
			Timeout:   telemetryExportTimeout,
			CheckRedirect: func(
				*http.Request,
				[]*http.Request,
			) error {
				return errOTelRedirect
			},
		}
	}
	traceAdapter, metricAdapter, err := newOfficialOTLPAdapters(
		context.Background(),
		"http://127.0.0.1:4318/v1/traces",
		"http://127.0.0.1:4318/v1/metrics",
		map[string]string{},
		"0.3.0-dev",
		client(),
		client(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownOTLPAdapters(traceAdapter, metricAdapter)

	points := maximumClosedMetricPoints()
	if len(points) != 1008 {
		t.Fatalf("closed metric points = %d", len(points))
	}
	names := []string{
		"sworn.eval.attempts",
		"sworn.eval.retries",
		"sworn.eval.duration_ns.numerator",
		"sworn.eval.duration_ns.denominator",
		"sworn.eval.input_tokens",
		"sworn.eval.output_tokens",
		"sworn.eval.usage_coverage.numerator",
		"sworn.eval.usage_coverage.denominator",
	}
	payload := telemetryMetricPayload{
		resource: telemetryResource{
			serviceName:    "sworn",
			serviceVersion: "0.3.0-dev",
		},
		metrics: make([]telemetryMetric, 0, len(names)),
	}
	for _, name := range names {
		payload.metrics = append(payload.metrics, telemetryMetric{
			name:   name,
			points: points,
		})
	}
	continuationPoints := maximumClosedContinuationMetricPoints()
	if len(continuationPoints) != 18 {
		t.Fatalf(
			"closed continuation metric points = %d",
			len(continuationPoints),
		)
	}
	payload.metrics = append(
		payload.metrics,
		telemetryMetric{
			name:   "sworn.eval.continuations",
			points: continuationPoints,
		},
		telemetryMetric{
			name: "sworn.eval.continuation.outcomes",
			points: []telemetryMetricPoint{
				continuationOutcomeMetricPoint("reuse"),
				continuationOutcomeMetricPoint("fallback"),
				continuationOutcomeMetricPoint("fallback_expired"),
			},
		},
	)
	if err := metricAdapter.Export(context.Background(), &payload); err != nil {
		t.Fatal(err)
	}

	transport.mu.Lock()
	requests := append([]otlpRequest(nil), transport.requests...)
	transport.mu.Unlock()
	if len(requests) != len(names)+1 {
		t.Fatalf(
			"metric requests = %d, want %d",
			len(requests),
			len(names)+1,
		)
	}
	for _, request := range requests {
		if request.path != "/v1/metrics" ||
			request.method != http.MethodPost ||
			request.contentEncoding != "" ||
			len(request.body) == 0 ||
			len(request.body) > otelMaxRequestBytes {
			t.Fatalf("unbounded metric request = %#v", request)
		}
	}
}

func maximumClosedMetricPoints() []telemetryMetricPoint {
	responsibilities := []struct {
		role           string
		responsibility string
	}{
		{role: "planner", responsibility: "planner_proposal"},
		{role: "implementer", responsibility: "implementer_design"},
		{role: "implementer", responsibility: "implementer_implementation"},
		{role: "captain", responsibility: "captain_review"},
		{role: "verifier", responsibility: "work_verification"},
		{role: "verifier", responsibility: "assembly_verification"},
		{role: "other", responsibility: "other"},
	}
	operations := []string{"driver.dispatch", "other"}
	transports := []string{
		"completed",
		"transport_error",
		"timeout",
		"cancelled",
		"runner_error",
		"other",
	}
	outcomes := []string{
		"pending",
		"claimed",
		"succeeded",
		"operational_failed",
		"uncertain",
		"other",
	}
	usageValues := []string{"reported", "unavailable"}
	observedAt := time.Date(2026, 7, 29, 1, 2, 33, 0, time.UTC)
	points := make(
		[]telemetryMetricPoint,
		0,
		telemetryMetricCardinality,
	)
	for _, responsibility := range responsibilities {
		for _, operation := range operations {
			for _, transport := range transports {
				for _, outcome := range outcomes {
					for _, usage := range usageValues {
						points = append(points, telemetryMetricPoint{
							attributes: []telemetryAttribute{
								stringTelemetryAttribute(
									"sworn.role",
									responsibility.role,
								),
								stringTelemetryAttribute(
									"sworn.responsibility",
									responsibility.responsibility,
								),
								stringTelemetryAttribute(
									"sworn.operation",
									operation,
								),
								stringTelemetryAttribute(
									"sworn.transport",
									transport,
								),
								stringTelemetryAttribute(
									"sworn.outcome",
									outcome,
								),
								stringTelemetryAttribute(
									"sworn.usage_known",
									usage,
								),
							},
							observedAt: observedAt,
							value:      1,
						})
					}
				}
			}
		}
	}
	return points
}

func maximumClosedContinuationMetricPoints() []telemetryMetricPoint {
	modes := []string{
		"fresh_rehydrate",
		"transcript_replay",
		"opaque_replay",
		"provider_cursor",
		"native_session",
		"compacted",
	}
	outcomes := []string{
		"reuse",
		"fallback",
		"fallback_expired",
	}
	observedAt := time.Date(2026, 7, 29, 1, 2, 33, 0, time.UTC)
	points := make([]telemetryMetricPoint, 0, len(modes)*len(outcomes))
	for _, mode := range modes {
		for _, outcome := range outcomes {
			points = append(points, telemetryMetricPoint{
				attributes: []telemetryAttribute{
					stringTelemetryAttribute(
						"sworn.continuation.mode",
						mode,
					),
					stringTelemetryAttribute(
						"sworn.continuation.outcome",
						outcome,
					),
				},
				observedAt: observedAt,
				value:      1,
			})
		}
	}
	return points
}

func continuationOutcomeMetricPoint(outcome string) telemetryMetricPoint {
	return telemetryMetricPoint{
		attributes: []telemetryAttribute{
			stringTelemetryAttribute(
				"sworn.continuation.outcome",
				outcome,
			),
		},
		observedAt: time.Date(2026, 7, 29, 1, 2, 33, 0, time.UTC),
		value:      1,
	}
}

func assertTraceWirePayload(t *testing.T, body []byte) {
	t.Helper()
	var request collectortracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &request); err != nil {
		t.Fatalf("decode trace request: %v", err)
	}
	if len(request.ResourceSpans) != 1 {
		t.Fatalf("resource spans = %d", len(request.ResourceSpans))
	}
	resourceSpans := request.ResourceSpans[0]
	assertProtoResource(t, resourceSpans.Resource)
	if resourceSpans.SchemaUrl != "" ||
		len(resourceSpans.ScopeSpans) != 1 {
		t.Fatalf("resource spans = %#v", resourceSpans)
	}
	scope := resourceSpans.ScopeSpans[0]
	if scope.Scope == nil || scope.Scope.Name != "sworn.observe" ||
		scope.Scope.Version != "" || len(scope.Scope.Attributes) != 0 ||
		scope.Scope.DroppedAttributesCount != 0 ||
		scope.SchemaUrl != "" || len(scope.Spans) != 2 {
		t.Fatalf("scope spans = %#v", scope)
	}
	sort.Slice(scope.Spans, func(left, right int) bool {
		return scope.Spans[left].Name < scope.Spans[right].Name
	})
	segment, recovery := scope.Spans[0], scope.Spans[1]
	if segment.Name != "sworn.process.segment" ||
		recovery.Name != "sworn.recovery" {
		t.Fatalf("span names = %q, %q", segment.Name, recovery.Name)
	}
	started := time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC)
	observed := started.Add(30 * time.Second)
	if segment.StartTimeUnixNano != uint64(started.UnixNano()) ||
		segment.EndTimeUnixNano != uint64(observed.UnixNano()) ||
		recovery.StartTimeUnixNano != uint64(observed.UnixNano()) ||
		recovery.EndTimeUnixNano != uint64(observed.UnixNano()) ||
		len(segment.ParentSpanId) != 0 ||
		!bytes.Equal(recovery.ParentSpanId, segment.SpanId) ||
		!bytes.Equal(segment.TraceId, recovery.TraceId) ||
		len(segment.TraceId) != 16 || len(segment.SpanId) != 8 ||
		len(recovery.SpanId) != 8 {
		t.Fatalf("span identity or chronology is invalid")
	}
	allowed := map[string]map[string]bool{
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
	for _, span := range scope.Spans {
		attributes := protoAttributeMap(t, span.Attributes)
		assertExactKeys(t, attributes, allowed[span.Name])
		if span.Kind != tracepb.Span_SPAN_KIND_INTERNAL ||
			span.Flags != 257 || span.TraceState != "" ||
			len(span.Events) != 0 || len(span.Links) != 0 ||
			span.Status == nil ||
			span.Status.Code != tracepb.Status_STATUS_CODE_UNSET ||
			span.Status.Message != "" ||
			span.DroppedAttributesCount != 0 ||
			span.DroppedEventsCount != 0 ||
			span.DroppedLinksCount != 0 {
			t.Fatalf("unexpected span fields = %#v", span)
		}
	}
	segmentAttributes := protoAttributeMap(t, segment.Attributes)
	if segmentAttributes["sworn.events"] != int64(8) ||
		segmentAttributes["sworn.attempts"] != int64(2) ||
		segmentAttributes["sworn.retries"] != int64(1) ||
		segmentAttributes["sworn.usage_known"] != true {
		t.Fatalf("segment attributes = %#v", segmentAttributes)
	}
}

func assertMetricWirePayload(t *testing.T, body []byte) {
	t.Helper()
	var request collectormetricspb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &request); err != nil {
		t.Fatalf("decode metric request: %v", err)
	}
	if len(request.ResourceMetrics) != 1 {
		t.Fatalf("resource metrics = %d", len(request.ResourceMetrics))
	}
	resourceMetrics := request.ResourceMetrics[0]
	assertProtoResource(t, resourceMetrics.Resource)
	if resourceMetrics.SchemaUrl != "" ||
		len(resourceMetrics.ScopeMetrics) != 1 {
		t.Fatalf("resource metrics = %#v", resourceMetrics)
	}
	scope := resourceMetrics.ScopeMetrics[0]
	if scope.Scope == nil || scope.Scope.Name != "sworn.observe" ||
		scope.Scope.Version != "" || len(scope.Scope.Attributes) != 0 ||
		scope.Scope.DroppedAttributesCount != 0 ||
		scope.SchemaUrl != "" {
		t.Fatalf("scope metrics = %#v", scope)
	}
	groupLabels := stringSet(
		"sworn.role",
		"sworn.responsibility",
		"sworn.operation",
		"sworn.transport",
		"sworn.outcome",
		"sworn.usage_known",
	)
	allowed := map[string]map[string]bool{
		"sworn.eval.events":                     stringSet("sworn.outcome"),
		"sworn.eval.continuations":              stringSet("sworn.continuation.mode", "sworn.continuation.outcome"),
		"sworn.eval.continuation.outcomes":      stringSet("sworn.continuation.outcome"),
		"sworn.eval.turn_recovery.actions":      stringSet("sworn.turn_recovery.action"),
		"sworn.eval.turn_recovery.outcomes":     stringSet("sworn.turn_recovery.outcome"),
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
	expectedValues := map[string][]int64{
		"sworn.eval.events":                     {8},
		"sworn.eval.continuations":              {1, 1, 1},
		"sworn.eval.continuation.outcomes":      {1, 1, 2},
		"sworn.eval.turn_recovery.actions":      {1, 1, 1},
		"sworn.eval.turn_recovery.outcomes":     {0, 1, 1},
		"sworn.eval.attempts":                   {2},
		"sworn.eval.retries":                    {1},
		"sworn.eval.recoveries":                 {0, 0, 1, 1},
		"sworn.eval.duration_ns.numerator":      {30},
		"sworn.eval.duration_ns.denominator":    {2},
		"sworn.eval.input_tokens":               {120},
		"sworn.eval.output_tokens":              {30},
		"sworn.eval.usage_coverage.numerator":   {1},
		"sworn.eval.usage_coverage.denominator": {2},
		"sworn.eval.quality.numerator":          {1, 1, 1},
		"sworn.eval.quality.denominator":        {1, 1, 1},
	}
	if len(scope.Metrics) != len(expectedValues) {
		t.Fatalf("metrics = %d", len(scope.Metrics))
	}
	observed := time.Date(2026, 7, 29, 1, 2, 33, 0, time.UTC)
	seen := make(map[string]bool, len(scope.Metrics))
	for _, metric := range scope.Metrics {
		if seen[metric.Name] || metric.Description != "" ||
			metric.Unit != "" || len(metric.Metadata) != 0 {
			t.Fatalf("metric metadata = %#v", metric)
		}
		seen[metric.Name] = true
		want, found := expectedValues[metric.Name]
		if !found || metric.GetGauge() == nil {
			t.Fatalf("unexpected metric %q", metric.Name)
		}
		points := metric.GetGauge().DataPoints
		got := make([]int64, 0, len(points))
		for _, point := range points {
			attributes := protoAttributeMap(t, point.Attributes)
			assertExactKeys(t, attributes, allowed[metric.Name])
			if point.TimeUnixNano != uint64(observed.UnixNano()) ||
				point.StartTimeUnixNano != 0 ||
				len(point.Exemplars) != 0 || point.Flags != 0 {
				t.Fatalf("metric point = %#v", point)
			}
			got = append(got, point.GetAsInt())
		}
		sort.Slice(got, func(left, right int) bool {
			return got[left] < got[right]
		})
		if len(got) != len(want) {
			t.Fatalf("%s values = %v, want %v", metric.Name, got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf(
					"%s values = %v, want %v",
					metric.Name,
					got,
					want,
				)
			}
		}
	}
}

func assertProtoResource(t *testing.T, value *resourcepb.Resource) {
	t.Helper()
	if value == nil || value.DroppedAttributesCount != 0 ||
		len(value.EntityRefs) != 0 {
		t.Fatalf("resource = %#v", value)
	}
	assertResource(t, protoAttributeMap(t, value.Attributes))
}

func protoAttributeMap(
	t *testing.T,
	values []*commonpb.KeyValue,
) map[string]any {
	t.Helper()
	result := make(map[string]any, len(values))
	for _, value := range values {
		if value == nil || value.Value == nil {
			t.Fatalf("nil protobuf attribute")
		}
		if _, duplicate := result[value.Key]; duplicate {
			t.Fatalf("duplicate protobuf attribute %q", value.Key)
		}
		switch typed := value.Value.Value.(type) {
		case *commonpb.AnyValue_StringValue:
			result[value.Key] = typed.StringValue
		case *commonpb.AnyValue_IntValue:
			result[value.Key] = typed.IntValue
		case *commonpb.AnyValue_BoolValue:
			result[value.Key] = typed.BoolValue
		default:
			t.Fatalf("unsupported protobuf attribute %q", value.Key)
		}
	}
	return result
}

type trackingResponseBody struct {
	mu     sync.Mutex
	body   *bytes.Reader
	read   int
	closed bool
}

func newTrackingResponseBody(size int) *trackingResponseBody {
	return &trackingResponseBody{
		body: bytes.NewReader(bytes.Repeat([]byte("private-error-"), size/14+1)),
	}
}

func (b *trackingResponseBody) Read(destination []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	count, err := b.body.Read(destination)
	b.read += count
	return count, err
}

func (b *trackingResponseBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

func (b *trackingResponseBody) status() (int, int64, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.read, b.body.Size(), b.closed
}

type trackingOTLPRoundTripper struct {
	mu     sync.Mutex
	calls  map[string]int
	bodies map[string]*trackingResponseBody
}

func (r *trackingOTLPRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	body := newTrackingResponseBody(
		otelMaxResponseBytes + otelMaxRequestBytes,
	)
	r.mu.Lock()
	r.calls[request.URL.Path]++
	r.bodies[request.URL.Path] = body
	r.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Status:     "503 Service Unavailable",
		Header:     make(http.Header),
		Body:       body,
		Request:    request,
	}, nil
}

func TestExplicitOTLPTransportDisablesRetriesAndBoundsResponses(
	t *testing.T,
) {
	clearBlockedOTelConstructorEnvironment(t)

	transport := &trackingOTLPRoundTripper{
		calls:  make(map[string]int),
		bodies: make(map[string]*trackingResponseBody),
	}
	client := func() *http.Client {
		return &http.Client{
			Transport: transport,
			Timeout:   telemetryExportTimeout,
			CheckRedirect: func(
				*http.Request,
				[]*http.Request,
			) error {
				return errOTelRedirect
			},
		}
	}
	telemetry, err := newOTLP(
		context.Background(),
		Config{
			SchemaVersion: OTelConfigSchemaVersion,
			Endpoint:      "http://127.0.0.1:4318/base",
			Headers:       map[string]string{},
		},
		"0.3.0-dev",
		client(),
		client(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(testTelemetryRecord("private-response")) {
		t.Fatal("record was not accepted")
	}
	waitForProcessed(t, telemetry, 1)
	shutdownTelemetry(t, telemetry)

	transport.mu.Lock()
	calls := make(map[string]int, len(transport.calls))
	bodies := make(map[string]*trackingResponseBody, len(transport.bodies))
	for path, count := range transport.calls {
		calls[path] = count
	}
	for path, body := range transport.bodies {
		bodies[path] = body
	}
	transport.mu.Unlock()
	for _, path := range []string{"/base/v1/traces", "/base/v1/metrics"} {
		if calls[path] != 1 {
			t.Errorf("%s calls = %d", path, calls[path])
		}
		body := bodies[path]
		if body == nil {
			t.Errorf("%s response body missing", path)
			continue
		}
		read, size, closed := body.status()
		if !closed || read <= 0 ||
			read > otelMaxResponseBytes+1 ||
			int64(read) >= size {
			t.Errorf(
				"%s response read/size/closed = %d/%d/%t",
				path,
				read,
				size,
				closed,
			)
		}
	}
	status := telemetry.Status()
	if status.Failures != 2 ||
		status.LastFailureCode != "metric_export" ||
		strings.Contains(status.LastFailureCode, "private-error") {
		t.Fatalf("status = %#v", status)
	}
}

type staticOTLPResponse struct {
	status int
	body   []byte
}

type staticOTLPRoundTripper struct {
	mu        sync.Mutex
	responses map[string]staticOTLPResponse
	calls     map[string]int
}

func (r *staticOTLPRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	r.mu.Lock()
	r.calls[request.URL.Path]++
	response := r.responses[request.URL.Path]
	r.mu.Unlock()
	return &http.Response{
		StatusCode: response.status,
		Status:     http.StatusText(response.status),
		Header: http.Header{
			"Content-Type": []string{"application/x-protobuf"},
		},
		Body: io.NopCloser(
			bytes.NewReader(append([]byte(nil), response.body...)),
		),
		Request: request,
	}, nil
}

func TestOTLPResponseFailuresStayFixedAndPrivate(t *testing.T) {
	clearBlockedOTelConstructorEnvironment(t)
	const sentinel = "PRIVATE_COLLECTOR_RESPONSE_SENTINEL"
	tracePartial, err := proto.Marshal(
		&collectortracepb.ExportTraceServiceResponse{
			PartialSuccess: &collectortracepb.ExportTracePartialSuccess{
				RejectedSpans: 1,
				ErrorMessage:  sentinel,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	metricPartial, err := proto.Marshal(
		&collectormetricspb.ExportMetricsServiceResponse{
			PartialSuccess: &collectormetricspb.ExportMetricsPartialSuccess{
				RejectedDataPoints: 1,
				ErrorMessage:       sentinel,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	traceMessageOnly, err := proto.Marshal(
		&collectortracepb.ExportTraceServiceResponse{
			PartialSuccess: &collectortracepb.ExportTracePartialSuccess{
				ErrorMessage: sentinel,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	metricMessageOnly, err := proto.Marshal(
		&collectormetricspb.ExportMetricsServiceResponse{
			PartialSuccess: &collectormetricspb.ExportMetricsPartialSuccess{
				ErrorMessage: sentinel,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		responses map[string]staticOTLPResponse
	}{
		{
			name: "partial success rejected counts",
			responses: map[string]staticOTLPResponse{
				"/base/v1/traces": {
					status: http.StatusOK,
					body:   tracePartial,
				},
				"/base/v1/metrics": {
					status: http.StatusOK,
					body:   metricPartial,
				},
			},
		},
		{
			name: "partial success message only",
			responses: map[string]staticOTLPResponse{
				"/base/v1/traces": {
					status: http.StatusOK,
					body:   traceMessageOnly,
				},
				"/base/v1/metrics": {
					status: http.StatusOK,
					body:   metricMessageOnly,
				},
			},
		},
		{
			name: "malformed and non-success",
			responses: map[string]staticOTLPResponse{
				"/base/v1/traces": {
					status: http.StatusOK,
					body:   []byte{0xff},
				},
				"/base/v1/metrics": {
					status: http.StatusServiceUnavailable,
					body:   []byte(sentinel),
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &staticOTLPRoundTripper{
				responses: test.responses,
				calls:     make(map[string]int),
			}
			client := func() *http.Client {
				return &http.Client{
					Transport: transport,
					Timeout:   telemetryExportTimeout,
					CheckRedirect: func(
						*http.Request,
						[]*http.Request,
					) error {
						return errOTelRedirect
					},
				}
			}
			telemetry, err := newOTLP(
				context.Background(),
				Config{
					SchemaVersion: OTelConfigSchemaVersion,
					Endpoint:      "http://127.0.0.1:4318/base",
					Headers:       map[string]string{},
				},
				"0.3.0-dev",
				client(),
				client(),
			)
			if err != nil {
				t.Fatal(err)
			}
			if !telemetry.TryEnqueue(
				testTelemetryRecord("collector-response"),
			) {
				t.Fatal("record was not accepted")
			}
			waitForProcessed(t, telemetry, 1)
			shutdownTelemetry(t, telemetry)
			status := telemetry.Status()
			if status.Failures != 2 ||
				status.LastFailureCode != "metric_export" ||
				strings.Contains(status.LastFailureCode, sentinel) {
				t.Fatalf("status = %#v", status)
			}
			transport.mu.Lock()
			traceCalls := transport.calls["/base/v1/traces"]
			metricCalls := transport.calls["/base/v1/metrics"]
			transport.mu.Unlock()
			if traceCalls != 1 || metricCalls != 1 {
				t.Fatalf(
					"trace/metric calls = %d/%d",
					traceCalls,
					metricCalls,
				)
			}
		})
	}
}

func TestOTelExplicitPolicyOverridesHostileAmbientInSubprocess(t *testing.T) {
	const helper = "SWORN_TEST_OTEL_HOSTILE_AMBIENT"
	const sentinel = "PRIVATE_HOSTILE_OTEL_SENTINEL_7d91"
	if os.Getenv(helper) == "1" {
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			defer request.Body.Close()
			_, _ = io.Copy(io.Discard, io.LimitReader(
				request.Body,
				otelMaxRequestBytes+1,
			))
			writer.WriteHeader(http.StatusOK)
		}))
		defer server.Close()
		telemetry, err := NewOTLP(
			context.Background(),
			Config{
				SchemaVersion: OTelConfigSchemaVersion,
				Endpoint:      server.URL + "/collector",
				Headers:       map[string]string{},
			},
			"0.3.0-dev",
		)
		if err != nil {
			t.Fatal(err)
		}
		assertHostileOTelEnvironmentUnchanged(t, sentinel)
		if !telemetry.TryEnqueue(testTelemetryRecord("subprocess")) {
			t.Fatal("record was not accepted")
		}
		waitForProcessed(t, telemetry, 1)
		shutdownTelemetry(t, telemetry)
		status := telemetry.Status()
		if status.TraceExports != 1 || status.MetricExports != 1 ||
			status.Failures != 0 {
			t.Fatalf("status = %#v", status)
		}
		return
	}

	hostile := map[string]string{
		helper:                                                     "1",
		"OTEL_RESOURCE_ATTRIBUTES":                                 "private=" + sentinel,
		"OTEL_SERVICE_NAME":                                        sentinel,
		"OTEL_TRACES_SAMPLER":                                      sentinel,
		"OTEL_TRACES_SAMPLER_ARG":                                  sentinel,
		"OTEL_METRICS_EXEMPLAR_FILTER":                             sentinel,
		"OTEL_GO_X_CARDINALITY_LIMIT":                              sentinel,
		"OTEL_GO_X_OBSERVABILITY":                                  "false",
		"OTEL_EXPORTER_OTLP_ENDPOINT":                              "https://ambient-" + sentinel + ".invalid/base",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT":                       "https://trace-" + sentinel + ".invalid/v1/traces",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT":                      "https://metric-" + sentinel + ".invalid/v1/metrics",
		"OTEL_EXPORTER_OTLP_HEADERS":                               "X-Ambient-Secret=" + sentinel,
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS":                        "X-Ambient-Secret=" + sentinel,
		"OTEL_EXPORTER_OTLP_METRICS_HEADERS":                       "X-Ambient-Secret=" + sentinel,
		"OTEL_EXPORTER_OTLP_COMPRESSION":                           "gzip",
		"OTEL_EXPORTER_OTLP_TRACES_COMPRESSION":                    "gzip",
		"OTEL_EXPORTER_OTLP_METRICS_COMPRESSION":                   "gzip",
		"OTEL_EXPORTER_OTLP_TIMEOUT":                               "1",
		"OTEL_EXPORTER_OTLP_TRACES_TIMEOUT":                        "1",
		"OTEL_EXPORTER_OTLP_METRICS_TIMEOUT":                       "1",
		"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE":        "delta",
		"OTEL_EXPORTER_OTLP_METRICS_DEFAULT_HISTOGRAM_AGGREGATION": "base2_exponential_bucket_histogram",
		"HTTP_PROXY":                                               "http://proxy-" + sentinel + ".invalid",
		"HTTPS_PROXY":                                              "http://proxy-" + sentinel + ".invalid",
		"NO_PROXY":                                                 "",
	}
	for _, name := range otelTLSFileEnvironmentNamesForTest() {
		hostile[name] = ""
	}
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestOTelExplicitPolicyOverridesHostileAmbientInSubprocess$",
		"-test.count=1",
		"-test.v",
	)
	command.Env = overrideEnvironment(os.Environ(), hostile)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf(
			"hostile ambient subprocess failed: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	for name, output := range map[string]string{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
	} {
		if strings.Contains(output, sentinel) {
			t.Fatalf("%s exposed hostile ambient sentinel", name)
		}
	}
}

func clearBlockedOTelConstructorEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range otelTLSFileEnvironmentNamesForTest() {
		t.Setenv(name, "")
	}
	t.Setenv("OTEL_GO_X_OBSERVABILITY", "false")
}

func otelTLSFileEnvironmentNamesForTest() []string {
	return []string{
		"OTEL_EXPORTER_OTLP_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_CLIENT_KEY",
		"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY",
		"OTEL_EXPORTER_OTLP_METRICS_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_KEY",
	}
}

func assertHostileOTelEnvironmentUnchanged(
	t *testing.T,
	sentinel string,
) {
	t.Helper()
	for _, name := range []string{
		"OTEL_RESOURCE_ATTRIBUTES",
		"OTEL_SERVICE_NAME",
		"OTEL_TRACES_SAMPLER",
		"OTEL_TRACES_SAMPLER_ARG",
		"OTEL_METRICS_EXEMPLAR_FILTER",
		"OTEL_GO_X_CARDINALITY_LIMIT",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_HEADERS",
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS",
		"OTEL_EXPORTER_OTLP_METRICS_HEADERS",
		"HTTP_PROXY",
		"HTTPS_PROXY",
	} {
		if !strings.Contains(os.Getenv(name), sentinel) {
			t.Fatalf("%s was changed", name)
		}
	}
	if os.Getenv("OTEL_GO_X_OBSERVABILITY") != "false" {
		t.Fatal("OTEL_GO_X_OBSERVABILITY was changed")
	}
}

func overrideEnvironment(
	current []string,
	overrides map[string]string,
) []string {
	result := make([]string, 0, len(current)+len(overrides))
	for _, entry := range current {
		name, _, found := strings.Cut(entry, "=")
		if _, replaced := overrides[name]; found && replaced {
			continue
		}
		result = append(result, entry)
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+overrides[name])
	}
	return result
}

func TestOTelHTTPClientHasClosedNetworkPolicy(t *testing.T) {
	t.Parallel()

	client := newOTelHTTPClient()
	if client.Timeout != telemetryExportTimeout ||
		client.CheckRedirect == nil ||
		!errors.Is(client.CheckRedirect(nil, nil), errOTelRedirect) {
		t.Fatalf("client policy = %#v", client)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil ||
		!transport.DisableCompression ||
		!transport.DisableKeepAlives ||
		transport.MaxConnsPerHost != 1 ||
		transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.MinVersion != tls.VersionTLS12 ||
		len(transport.TLSNextProto) != 0 ||
		transport.Protocols == nil ||
		!transport.Protocols.HTTP1() ||
		transport.Protocols.HTTP2() ||
		transport.Protocols.UnencryptedHTTP2() {
		t.Fatalf("transport policy = %#v", transport)
	}
}

func waitForProcessed(t *testing.T, telemetry *Telemetry, count uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if telemetry.Status().Processed >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("processed = %d, want %d", telemetry.Status().Processed, count)
}
