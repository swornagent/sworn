package observe

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const otelMaxRequestBytes = 1024 * 1024

var errOTelRedirect = errors.New("OTLP redirect rejected")

// NewOTLP creates an explicitly configured, package-local OTLP/HTTP
// projection. It does not install global OpenTelemetry providers and does not
// read endpoint, header, retry, compression, or client policy from ambient
// OTEL_* variables.
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
	traceExporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpointURL(traceURL),
		otlptracehttp.WithHeaders(canonical.Headers),
		otlptracehttp.WithCompression(otlptracehttp.NoCompression),
		otlptracehttp.WithRetry(
			otlptracehttp.RetryConfig{Enabled: false},
		),
		otlptracehttp.WithTimeout(telemetryExportTimeout),
		otlptracehttp.WithHTTPClient(traceClient),
		otlptracehttp.WithMaxRequestSize(otelMaxRequestBytes),
	)
	if err != nil {
		return nil, fail("OTEL_EXPORTER_FAILED")
	}
	metricExporter, err := otlpmetrichttp.New(
		ctx,
		otlpmetrichttp.WithEndpointURL(metricURL),
		otlpmetrichttp.WithHeaders(canonical.Headers),
		otlpmetrichttp.WithCompression(otlpmetrichttp.NoCompression),
		otlpmetrichttp.WithRetry(
			otlpmetrichttp.RetryConfig{Enabled: false},
		),
		otlpmetrichttp.WithTimeout(telemetryExportTimeout),
		otlpmetrichttp.WithHTTPClient(metricClient),
		otlpmetrichttp.WithMaxRequestSize(otelMaxRequestBytes),
		otlpmetrichttp.WithTemporalitySelector(
			sdkmetric.DefaultTemporalitySelector,
		),
		otlpmetrichttp.WithAggregationSelector(
			sdkmetric.DefaultAggregationSelector,
		),
	)
	if err != nil {
		_ = traceExporter.Shutdown(context.Background())
		return nil, fail("OTEL_EXPORTER_FAILED")
	}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		version,
		telemetryQueueCapacity,
		telemetryExportInterval,
	)
	if err != nil {
		_ = traceExporter.Shutdown(context.Background())
		_ = metricExporter.Shutdown(context.Background())
		return nil, err
	}
	return telemetry, nil
}

func newOTelHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   telemetryExportTimeout,
		KeepAlive: -1,
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      false,
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxIdleConns:           0,
		IdleConnTimeout:        time.Second,
		TLSHandshakeTimeout:    telemetryExportTimeout,
		ResponseHeaderTimeout:  telemetryExportTimeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 16 * 1024,
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
