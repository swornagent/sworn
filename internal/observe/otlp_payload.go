package observe

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const otelMetricPointsPerRequest = telemetryMetricCardinality

var errOTLPAdapterPayload = errors.New("OTLP adapter payload invalid")

type otelTraceAdapter struct {
	exporter *otlptrace.Exporter
	resource *resource.Resource
}

func (a *otelTraceAdapter) ExportSpans(
	ctx context.Context,
	spans []telemetrySpan,
) error {
	if a == nil || a.exporter == nil || a.resource == nil ||
		len(spans) == 0 {
		return errOTLPAdapterPayload
	}
	values := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		value, err := a.readOnlySpan(span)
		if err != nil {
			return err
		}
		values = append(values, value)
	}
	return a.exporter.ExportSpans(ctx, values)
}

func (a *otelTraceAdapter) readOnlySpan(
	value telemetrySpan,
) (sdktrace.ReadOnlySpan, error) {
	attributes, err := otelSDKAttributes(value.attributes)
	if err != nil {
		return nil, err
	}
	traceID := trace.TraceID(value.traceID)
	spanID := trace.SpanID(value.spanID)
	if !traceID.IsValid() || !spanID.IsValid() ||
		value.name == "" || value.startedAt.IsZero() ||
		value.endedAt.Before(value.startedAt) {
		return nil, errOTLPAdapterPayload
	}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	var parent trace.SpanContext
	if value.hasParent {
		parentID := trace.SpanID(value.parentSpanID)
		if !parentID.IsValid() {
			return nil, errOTLPAdapterPayload
		}
		parent = trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     parentID,
			TraceFlags: trace.FlagsSampled,
		})
	}
	return otelReadOnlySpan{
		name:        value.name,
		spanContext: spanContext,
		parent:      parent,
		startedAt:   value.startedAt,
		endedAt:     value.endedAt,
		attributes:  attributes,
		resource:    a.resource,
	}, nil
}

func (a *otelTraceAdapter) Shutdown(ctx context.Context) error {
	if a == nil || a.exporter == nil {
		return nil
	}
	return a.exporter.Shutdown(ctx)
}

type otelMetricAdapter struct {
	exporter *otlpmetrichttp.Exporter
	resource *resource.Resource
}

func (a *otelMetricAdapter) Export(
	ctx context.Context,
	value *telemetryMetricPayload,
) error {
	if a == nil || a.exporter == nil || a.resource == nil ||
		value == nil || len(value.metrics) == 0 {
		return errOTLPAdapterPayload
	}
	batches, err := a.metricBatches(value)
	if err != nil {
		return err
	}
	for index := range batches {
		if err := a.exporter.Export(ctx, &batches[index]); err != nil {
			return err
		}
	}
	return nil
}

func (a *otelMetricAdapter) metricBatches(
	value *telemetryMetricPayload,
) ([]metricdata.ResourceMetrics, error) {
	metrics := make([]metricdata.Metrics, 0, len(value.metrics))
	pointCounts := make([]int, 0, len(value.metrics))
	for _, metric := range value.metrics {
		if metric.name == "" || len(metric.points) == 0 ||
			len(metric.points) > otelMetricPointsPerRequest {
			return nil, errOTLPAdapterPayload
		}
		points := make(
			[]metricdata.DataPoint[int64],
			0,
			len(metric.points),
		)
		for _, point := range metric.points {
			attributes, err := otelSDKAttributes(point.attributes)
			if err != nil || point.observedAt.IsZero() {
				return nil, errOTLPAdapterPayload
			}
			points = append(points, metricdata.DataPoint[int64]{
				Attributes: attribute.NewSet(attributes...),
				Time:       point.observedAt,
				Value:      point.value,
			})
		}
		metrics = append(metrics, metricdata.Metrics{
			Name: metric.name,
			Data: metricdata.Gauge[int64]{DataPoints: points},
		})
		pointCounts = append(pointCounts, len(points))
	}

	scope := instrumentation.Scope{Name: "sworn.observe"}
	batches := make([]metricdata.ResourceMetrics, 0, len(metrics))
	current := make([]metricdata.Metrics, 0, len(metrics))
	currentPoints := 0
	flush := func() {
		batches = append(batches, metricdata.ResourceMetrics{
			Resource: a.resource,
			ScopeMetrics: []metricdata.ScopeMetrics{{
				Scope:   scope,
				Metrics: current,
			}},
		})
		current = nil
		currentPoints = 0
	}
	for index, metric := range metrics {
		if currentPoints != 0 &&
			currentPoints+pointCounts[index] >
				otelMetricPointsPerRequest {
			flush()
		}
		current = append(current, metric)
		currentPoints += pointCounts[index]
	}
	if len(current) != 0 {
		flush()
	}
	return batches, nil
}

func (a *otelMetricAdapter) Shutdown(ctx context.Context) error {
	if a == nil || a.exporter == nil {
		return nil
	}
	return a.exporter.Shutdown(ctx)
}

// otelReadOnlySpan deliberately embeds the SDK interface to inherit its
// private compatibility method, then defines every readable field explicitly.
// This feeds the official trace exporter without installing an SDK provider or
// ambient resource detectors.
type otelReadOnlySpan struct {
	sdktrace.ReadOnlySpan

	name        string
	spanContext trace.SpanContext
	parent      trace.SpanContext
	startedAt   time.Time
	endedAt     time.Time
	attributes  []attribute.KeyValue
	resource    *resource.Resource
}

func (s otelReadOnlySpan) Name() string {
	return s.name
}

func (s otelReadOnlySpan) SpanContext() trace.SpanContext {
	return s.spanContext
}

func (s otelReadOnlySpan) Parent() trace.SpanContext {
	return s.parent
}

func (otelReadOnlySpan) SpanKind() trace.SpanKind {
	return trace.SpanKindInternal
}

func (s otelReadOnlySpan) StartTime() time.Time {
	return s.startedAt
}

func (s otelReadOnlySpan) EndTime() time.Time {
	return s.endedAt
}

func (s otelReadOnlySpan) Attributes() []attribute.KeyValue {
	return s.attributes
}

func (otelReadOnlySpan) Links() []sdktrace.Link {
	return nil
}

func (otelReadOnlySpan) Events() []sdktrace.Event {
	return nil
}

func (otelReadOnlySpan) Status() sdktrace.Status {
	return sdktrace.Status{}
}

func (otelReadOnlySpan) InstrumentationScope() instrumentation.Scope {
	return instrumentation.Scope{Name: "sworn.observe"}
}

func (s otelReadOnlySpan) InstrumentationLibrary() instrumentation.Library {
	return s.InstrumentationScope()
}

func (s otelReadOnlySpan) Resource() *resource.Resource {
	return s.resource
}

func (otelReadOnlySpan) DroppedAttributes() int {
	return 0
}

func (otelReadOnlySpan) DroppedLinks() int {
	return 0
}

func (otelReadOnlySpan) DroppedEvents() int {
	return 0
}

func (otelReadOnlySpan) ChildSpanCount() int {
	return 0
}

func otelSDKResource(
	value telemetryResource,
) (*resource.Resource, error) {
	if value.serviceName == "" || value.serviceVersion == "" {
		return nil, errOTLPAdapterPayload
	}
	return resource.NewSchemaless(
		attribute.String("service.name", value.serviceName),
		attribute.String("service.version", value.serviceVersion),
	), nil
}

func otelSDKAttributes(
	values []telemetryAttribute,
) ([]attribute.KeyValue, error) {
	result := make([]attribute.KeyValue, 0, len(values))
	for _, value := range values {
		if value.key == "" {
			return nil, errOTLPAdapterPayload
		}
		switch value.kind {
		case telemetryStringAttribute:
			result = append(
				result,
				attribute.String(value.key, value.stringValue),
			)
		case telemetryInt64Attribute:
			result = append(
				result,
				attribute.Int64(value.key, value.int64Value),
			)
		case telemetryBoolAttribute:
			result = append(
				result,
				attribute.Bool(value.key, value.boolValue),
			)
		default:
			return nil, errOTLPAdapterPayload
		}
	}
	return result, nil
}

var (
	_ telemetryTraceExporter  = (*otelTraceAdapter)(nil)
	_ telemetryMetricExporter = (*otelMetricAdapter)(nil)
	_ sdktrace.SpanExporter   = (*otlptrace.Exporter)(nil)
	_ sdkmetric.Exporter      = (*otlpmetrichttp.Exporter)(nil)
)
