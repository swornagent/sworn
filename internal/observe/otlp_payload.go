package observe

import (
	"context"
	"errors"
	"net/http"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

var errOTLPPayload = errors.New("OTLP payload invalid")

type otlpTraceExporter struct {
	sender *otlpHTTPSender
}

func newOTLPTraceExporter(
	endpoint string,
	headers map[string]string,
	client *http.Client,
) *otlpTraceExporter {
	return &otlpTraceExporter{
		sender: newOTLPHTTPSender(endpoint, headers, client),
	}
}

func (e *otlpTraceExporter) ExportSpans(
	ctx context.Context,
	spans []telemetrySpan,
) error {
	request, err := otlpTraceRequest(spans)
	if err != nil {
		return err
	}
	response, err := e.sender.post(ctx, request)
	if err != nil {
		return err
	}
	rejected, err := otlpResponseRejected(response)
	if err != nil {
		return err
	}
	if rejected {
		return errOTLPResponse
	}
	return nil
}

func (e *otlpTraceExporter) Shutdown(context.Context) error {
	return e.sender.shutdown()
}

type otlpMetricExporter struct {
	sender *otlpHTTPSender
}

func newOTLPMetricExporter(
	endpoint string,
	headers map[string]string,
	client *http.Client,
) *otlpMetricExporter {
	return &otlpMetricExporter{
		sender: newOTLPHTTPSender(endpoint, headers, client),
	}
}

func (e *otlpMetricExporter) Export(
	ctx context.Context,
	value *telemetryMetricPayload,
) error {
	requests, err := otlpMetricRequests(value)
	if err != nil {
		return err
	}
	for _, request := range requests {
		response, err := e.sender.post(ctx, request)
		if err != nil {
			return err
		}
		rejected, err := otlpResponseRejected(response)
		if err != nil {
			return err
		}
		if rejected {
			return errOTLPResponse
		}
	}
	return nil
}

func (e *otlpMetricExporter) Shutdown(context.Context) error {
	return e.sender.shutdown()
}

func otlpTraceRequest(
	spans []telemetrySpan,
) (*tracepb.TracesData, error) {
	if len(spans) == 0 {
		return nil, errOTLPPayload
	}
	protoSpans := make([]*tracepb.Span, 0, len(spans))
	for _, span := range spans {
		attributes, err := otlpAttributes(span.attributes)
		if err != nil {
			return nil, err
		}
		value := &tracepb.Span{
			TraceId:           append([]byte(nil), span.traceID[:]...),
			SpanId:            append([]byte(nil), span.spanID[:]...),
			Flags:             1,
			Name:              span.name,
			Kind:              tracepb.Span_SPAN_KIND_INTERNAL,
			StartTimeUnixNano: uint64(span.startedAt.UnixNano()),
			EndTimeUnixNano:   uint64(span.endedAt.UnixNano()),
			Attributes:        attributes,
		}
		if span.hasParent {
			value.ParentSpanId = append(
				[]byte(nil),
				span.parentSpanID[:]...,
			)
		}
		protoSpans = append(protoSpans, value)
	}
	resourceValue, err := otlpResource(spans[0].resource)
	if err != nil {
		return nil, err
	}
	// TracesData is wire-identical to ExportTraceServiceRequest. The official
	// generated contract requires both envelopes to evolve together.
	return &tracepb.TracesData{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: resourceValue,
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{
					Name: "sworn.observe",
				},
				Spans: protoSpans,
			}},
		}},
	}, nil
}

func otlpMetricRequests(
	value *telemetryMetricPayload,
) ([]*metricspb.MetricsData, error) {
	if value == nil || len(value.metrics) == 0 {
		return nil, errOTLPPayload
	}
	resourceValue, err := otlpResource(value.resource)
	if err != nil {
		return nil, err
	}
	metrics := make([]*metricspb.Metric, 0, len(value.metrics))
	for _, metric := range value.metrics {
		points := make(
			[]*metricspb.NumberDataPoint,
			0,
			len(metric.points),
		)
		for _, point := range metric.points {
			attributes, err := otlpAttributes(point.attributes)
			if err != nil {
				return nil, err
			}
			points = append(points, &metricspb.NumberDataPoint{
				Attributes:   attributes,
				TimeUnixNano: uint64(point.observedAt.UnixNano()),
				Value: &metricspb.NumberDataPoint_AsInt{
					AsInt: point.value,
				},
			})
		}
		metrics = append(metrics, &metricspb.Metric{
			Name: metric.name,
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{DataPoints: points},
			},
		})
	}
	buildRequest := func(
		values []*metricspb.Metric,
	) *metricspb.MetricsData {
		// MetricsData is wire-identical to ExportMetricsServiceRequest.
		return &metricspb.MetricsData{
			ResourceMetrics: []*metricspb.ResourceMetrics{{
				Resource: resourceValue,
				ScopeMetrics: []*metricspb.ScopeMetrics{{
					Scope: &commonpb.InstrumentationScope{
						Name: "sworn.observe",
					},
					Metrics: values,
				}},
			}},
		}
	}
	requests := make(
		[]*metricspb.MetricsData,
		0,
		2,
	)
	current := make([]*metricspb.Metric, 0, len(metrics))
	for _, metric := range metrics {
		candidate := append(
			append([]*metricspb.Metric(nil), current...),
			metric,
		)
		request := buildRequest(candidate)
		if proto.Size(request) <= otelMaxRequestBytes {
			current = candidate
			continue
		}
		if len(current) == 0 {
			return nil, errOTLPRequestSize
		}
		requests = append(requests, buildRequest(current))
		current = []*metricspb.Metric{metric}
		if proto.Size(buildRequest(current)) > otelMaxRequestBytes {
			return nil, errOTLPRequestSize
		}
	}
	if len(current) != 0 {
		requests = append(requests, buildRequest(current))
	}
	return requests, nil
}

func otlpResponseRejected(body []byte) (bool, error) {
	for len(body) != 0 {
		number, wireType, tagLength := protowire.ConsumeTag(body)
		if tagLength < 0 {
			return false, errOTLPResponse
		}
		body = body[tagLength:]
		if number == 1 {
			if wireType != protowire.BytesType {
				return false, errOTLPResponse
			}
			partial, fieldLength := protowire.ConsumeBytes(body)
			if fieldLength < 0 {
				return false, errOTLPResponse
			}
			rejected, err := otlpPartialRejected(partial)
			if err != nil || rejected {
				return rejected, err
			}
			body = body[fieldLength:]
			continue
		}
		fieldLength := protowire.ConsumeFieldValue(
			number,
			wireType,
			body,
		)
		if fieldLength < 0 {
			return false, errOTLPResponse
		}
		body = body[fieldLength:]
	}
	return false, nil
}

func otlpPartialRejected(body []byte) (bool, error) {
	for len(body) != 0 {
		number, wireType, tagLength := protowire.ConsumeTag(body)
		if tagLength < 0 {
			return false, errOTLPResponse
		}
		body = body[tagLength:]
		if number == 1 {
			if wireType != protowire.VarintType {
				return false, errOTLPResponse
			}
			rejected, fieldLength := protowire.ConsumeVarint(body)
			if fieldLength < 0 {
				return false, errOTLPResponse
			}
			if rejected != 0 {
				return true, nil
			}
			body = body[fieldLength:]
			continue
		}
		fieldLength := protowire.ConsumeFieldValue(
			number,
			wireType,
			body,
		)
		if fieldLength < 0 {
			return false, errOTLPResponse
		}
		body = body[fieldLength:]
	}
	return false, nil
}

func otlpResource(value telemetryResource) (*resourcepb.Resource, error) {
	attributes, err := otlpAttributes([]telemetryAttribute{
		stringTelemetryAttribute("service.name", value.serviceName),
		stringTelemetryAttribute(
			"service.version",
			value.serviceVersion,
		),
	})
	if err != nil {
		return nil, err
	}
	return &resourcepb.Resource{Attributes: attributes}, nil
}

func otlpAttributes(
	values []telemetryAttribute,
) ([]*commonpb.KeyValue, error) {
	result := make([]*commonpb.KeyValue, 0, len(values))
	for _, value := range values {
		converted, err := otlpAttributeValue(value)
		if err != nil {
			return nil, err
		}
		result = append(result, &commonpb.KeyValue{
			Key:   value.key,
			Value: converted,
		})
	}
	return result, nil
}

func otlpAttributeValue(
	value telemetryAttribute,
) (*commonpb.AnyValue, error) {
	switch value.kind {
	case telemetryStringAttribute:
		return &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{
				StringValue: value.stringValue,
			},
		}, nil
	case telemetryInt64Attribute:
		return &commonpb.AnyValue{
			Value: &commonpb.AnyValue_IntValue{
				IntValue: value.int64Value,
			},
		}, nil
	case telemetryBoolAttribute:
		return &commonpb.AnyValue{
			Value: &commonpb.AnyValue_BoolValue{
				BoolValue: value.boolValue,
			},
		}, nil
	default:
		return nil, errOTLPPayload
	}
}
