package observe

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	if ambientRequests.Load() != 0 {
		t.Fatalf("ambient endpoint received %d requests", ambientRequests.Load())
	}
	status := telemetry.Status()
	if status.TraceExports != 1 || status.MetricExports != 1 ||
		status.Failures != 0 {
		t.Fatalf("status = %#v", status)
	}
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
	body := newTrackingResponseBody(5 * otelMaxRequestBytes)
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

func TestOfficialOTLPClientsDisableRetriesAndBoundAndCloseResponses(
	t *testing.T,
) {
	t.Parallel()

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
		if !closed || read <= 0 || int64(read) >= size {
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
		transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.MinVersion != tls.VersionTLS12 ||
		len(transport.TLSNextProto) != 0 {
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
