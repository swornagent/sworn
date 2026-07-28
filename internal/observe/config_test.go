package observe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseConfigCanonicalizesExplicitSafeConfiguration(t *testing.T) {
	t.Parallel()

	body := []byte(
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com/otlp",` +
			`"headers":{"x-sworn-tenant":"tenant-a"}}`,
	)
	config, err := ParseConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	if config.Endpoint != "https://collector.example.com/otlp" ||
		config.Headers["X-Sworn-Tenant"] != "tenant-a" ||
		len(config.Headers) != 1 {
		t.Fatalf("config = %#v", config)
	}
	traceURL, metricURL, err := signalURLs(config.Endpoint)
	if err != nil ||
		traceURL !=
			"https://collector.example.com/otlp/v1/traces" ||
		metricURL !=
			"https://collector.example.com/otlp/v1/metrics" {
		t.Fatalf("signal URLs = %q, %q, %v", traceURL, metricURL, err)
	}
	loopback, err := ParseConfig([]byte(
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"http://[::1]:4318","headers":{}}`,
	))
	if err != nil || loopback.Endpoint != "http://[::1]:4318" {
		t.Fatalf("loopback config = %#v, %v", loopback, err)
	}
}

func TestParseConfigRejectsAmbiguousOrUnsafeInput(t *testing.T) {
	t.Parallel()

	headerValues := make(map[string]string)
	for index := 0; index < maxOTelHeaders+1; index++ {
		headerValues["X-Test-"+string(rune('A'+index))] = "value"
	}
	tooManyHeaders, err := json.Marshal(Config{
		SchemaVersion: OTelConfigSchemaVersion,
		Endpoint:      "https://collector.example.com",
		Headers:       headerValues,
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []string{
		`{}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"http://collector.example.com","headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"http://localhost:4318","headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"ftp://127.0.0.1:4318","headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://user:secret@collector.example.com",` +
			`"headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com?a=b","headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com/#fragment",` +
			`"headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com/a/../b",` +
			`"headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com/a//b",` +
			`"headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com/a%2fb",` +
			`"headers":{}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com",` +
			`"headers":{"Content-Type":"text/plain"}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com",` +
			`"headers":{"X-Test":"one\r\ntwo"}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com",` +
			`"headers":{"X-Test":"one","x-test":"two"}}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com",` +
			`"headers":{},"unknown":true}`,
		`{"schema_version":"sworn.otel-config/v1",` +
			`"endpoint":"https://collector.example.com","headers":{}} trailing`,
		string(tooManyHeaders),
	}
	for _, body := range tests {
		body := body
		t.Run(strings.ReplaceAll(body[:min(len(body), 32)], "/", "_"), func(
			t *testing.T,
		) {
			t.Parallel()
			if _, err := ParseConfig([]byte(body)); err == nil {
				t.Fatalf("admitted %q", body)
			}
		})
	}
}

func TestCanonicalConfigDefendsConstructedValues(t *testing.T) {
	t.Parallel()

	_, _, _, err := canonicalConfig(Config{
		SchemaVersion: OTelConfigSchemaVersion,
		Endpoint:      "http://192.0.2.1:4318",
		Headers:       map[string]string{},
	})
	if !IsCode(err, "INVALID_OTEL_ENDPOINT") {
		t.Fatalf("constructed config = %v", err)
	}
}
