package observe

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	otelMaxRequestBytes  = 1024 * 1024
	otelMaxResponseBytes = 4 * 1024 * 1024
)

var errOTelRedirect = errors.New("OTLP redirect rejected")

// NewOTLP creates an explicitly configured, package-local OTLP/HTTP
// projection. It does not install global OpenTelemetry providers. Explicit
// options and a closed HTTP client prevent ambient endpoint, header, retry,
// compression, proxy, transport, or metric policy from changing outbound
// behavior. Ambient settings that would make the official constructors read
// files or use process-global OpenTelemetry state instead leave telemetry
// locally unhealthy and unavailable without controlling operator startup.
func NewOTLP(
	ctx context.Context,
	config Config,
	version string,
) (*Telemetry, error) {
	return newOTLP(
		ctx,
		config,
		version,
		newOTelHTTPClient(),
		newOTelHTTPClient(),
	)
}

func newOTLP(
	ctx context.Context,
	config Config,
	version string,
	traceClient, metricClient *http.Client,
) (*Telemetry, error) {
	if ctx == nil || !validVersion(version) {
		return nil, fail("INVALID_TELEMETRY")
	}
	if traceClient == nil || metricClient == nil {
		return nil, fail("INVALID_TELEMETRY")
	}
	canonical, traceURL, metricURL, err := canonicalConfig(config)
	if err != nil {
		return nil, err
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
		traceClient,
		metricClient,
	)
	if err != nil {
		return unavailableTelemetry("exporter_start"), nil
	}
	telemetry, err := newTelemetry(
		traceAdapter,
		metricAdapter,
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

func unavailableTelemetry(code string) *Telemetry {
	telemetry := &Telemetry{enabled: true}
	telemetry.failure(code)
	return telemetry
}

// The official v1.44 constructors read ambient TLS files before applying
// explicit options, and experimental self-observability interacts with the
// process-global OpenTelemetry provider. The operator treats the process
// environment as stable during startup and declines telemetry without opening
// those paths or using that global behavior.
func ambientOTelConstructorBlocked() bool {
	for _, name := range []string{
		"OTEL_EXPORTER_OTLP_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_CLIENT_KEY",
		"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY",
		"OTEL_EXPORTER_OTLP_METRICS_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_CERTIFICATE",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_KEY",
	} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return true
		}
	}
	return strings.EqualFold(
		strings.TrimSpace(os.Getenv("OTEL_GO_X_OBSERVABILITY")),
		"true",
	)
}

func newOfficialOTLPAdapters(
	ctx context.Context,
	traceURL, metricURL string,
	headers map[string]string,
	version string,
	traceClient, metricClient *http.Client,
) (*otelTraceAdapter, *otelMetricAdapter, error) {
	traceExporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpointURL(traceURL),
		otlptracehttp.WithHeaders(headers),
		otlptracehttp.WithCompression(
			otlptracehttp.NoCompression,
		),
		otlptracehttp.WithTimeout(telemetryExportTimeout),
		otlptracehttp.WithMaxRequestSize(otelMaxRequestBytes),
		otlptracehttp.WithRetry(
			otlptracehttp.RetryConfig{Enabled: false},
		),
		otlptracehttp.WithHTTPClient(traceClient),
	)
	if err != nil {
		return nil, nil, fail("OTEL_EXPORTER_START_FAILED")
	}
	metricExporter, err := otlpmetrichttp.New(
		ctx,
		otlpmetrichttp.WithEndpointURL(metricURL),
		otlpmetrichttp.WithHeaders(headers),
		otlpmetrichttp.WithCompression(
			otlpmetrichttp.NoCompression,
		),
		otlpmetrichttp.WithTimeout(telemetryExportTimeout),
		otlpmetrichttp.WithMaxRequestSize(otelMaxRequestBytes),
		otlpmetrichttp.WithRetry(
			otlpmetrichttp.RetryConfig{Enabled: false},
		),
		otlpmetrichttp.WithTemporalitySelector(
			sdkmetric.CumulativeTemporalitySelector,
		),
		otlpmetrichttp.WithAggregationSelector(
			sdkmetric.DefaultAggregationSelector,
		),
		otlpmetrichttp.WithHTTPClient(metricClient),
	)
	if err != nil {
		shutdownOfficialOTLPExporters(traceExporter, nil)
		return nil, nil, fail("OTEL_EXPORTER_START_FAILED")
	}
	resource, err := otelSDKResource(telemetryResource{
		serviceName:    "sworn",
		serviceVersion: version,
	})
	if err != nil {
		shutdownOfficialOTLPExporters(traceExporter, metricExporter)
		return nil, nil, fail("OTEL_EXPORTER_START_FAILED")
	}
	return &otelTraceAdapter{
			exporter: traceExporter,
			resource: resource,
		}, &otelMetricAdapter{
			exporter: metricExporter,
			resource: resource,
		}, nil
}

func shutdownOTLPAdapters(
	traceAdapter *otelTraceAdapter,
	metricAdapter *otelMetricAdapter,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	defer cancel()
	if traceAdapter != nil {
		_ = traceAdapter.Shutdown(ctx)
	}
	if metricAdapter != nil {
		_ = metricAdapter.Shutdown(ctx)
	}
}

func shutdownOfficialOTLPExporters(
	traceExporter *otlptrace.Exporter,
	metricExporter *otlpmetrichttp.Exporter,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	defer cancel()
	if traceExporter != nil {
		_ = traceExporter.Shutdown(ctx)
	}
	if metricExporter != nil {
		_ = metricExporter.Shutdown(ctx)
	}
}

func newOTelHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   telemetryExportTimeout,
		KeepAlive: -1,
	}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      false,
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxIdleConns:           0,
		MaxConnsPerHost:        1,
		IdleConnTimeout:        time.Second,
		TLSHandshakeTimeout:    telemetryExportTimeout,
		ResponseHeaderTimeout:  telemetryExportTimeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 16 * 1024,
		Protocols:              protocols,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		TLSNextProto: map[string]func(
			string,
			*tls.Conn,
		) http.RoundTripper{},
	}
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
