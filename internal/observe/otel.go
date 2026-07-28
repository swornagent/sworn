package observe

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	otelMaxRequestBytes  = 1024 * 1024
	otelMaxResponseBytes = 4 * 1024 * 1024
)

var (
	errOTelRedirect     = errors.New("OTLP redirect rejected")
	errOTLPRequest      = errors.New("OTLP request failed")
	errOTLPRequestSize  = errors.New("OTLP request too large")
	errOTLPResponse     = errors.New("OTLP response failed")
	errOTLPResponseSize = errors.New("OTLP response too large")
)

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
	traceExporter := newOTLPTraceExporter(
		traceURL,
		canonical.Headers,
		traceClient,
	)
	metricExporter := newOTLPMetricExporter(
		metricURL,
		canonical.Headers,
		metricClient,
	)
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

type otlpHTTPSender struct {
	endpoint string
	headers  map[string]string
	client   *http.Client
}

func newOTLPHTTPSender(
	endpoint string,
	headers map[string]string,
	client *http.Client,
) *otlpHTTPSender {
	copiedHeaders := make(map[string]string, len(headers))
	for name, value := range headers {
		copiedHeaders[name] = value
	}
	return &otlpHTTPSender{
		endpoint: endpoint,
		headers:  copiedHeaders,
		client:   client,
	}
}

func (s *otlpHTTPSender) post(
	ctx context.Context,
	requestMessage proto.Message,
) ([]byte, error) {
	body, err := proto.MarshalOptions{Deterministic: true}.Marshal(
		requestMessage,
	)
	if err != nil {
		return nil, errOTLPRequest
	}
	if len(body) > otelMaxRequestBytes {
		return nil, errOTLPRequestSize
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, errOTLPRequest
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Accept", "application/x-protobuf")
	for name, value := range s.headers {
		request.Header.Set(name, value)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, errOTLPRequest
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(
		response.Body,
		otelMaxResponseBytes+1,
	))
	if err != nil {
		return nil, errOTLPResponse
	}
	if len(responseBody) > otelMaxResponseBytes {
		return nil, errOTLPResponseSize
	}
	if response.StatusCode != http.StatusOK {
		return nil, errOTLPResponse
	}
	return responseBody, nil
}

func (s *otlpHTTPSender) shutdown() error {
	if s == nil || s.client == nil {
		return nil
	}
	s.client.CloseIdleConnections()
	return nil
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
