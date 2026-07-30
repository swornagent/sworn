package observe

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

const (
	OTelConfigSchemaVersion = "sworn.otel-config/v1"
	MaxOTelConfigBytes      = 16 * 1024
	maxOTelHeaders          = 16
)

var headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)

type Config struct {
	SchemaVersion string            `json:"schema_version"`
	Endpoint      string            `json:"endpoint"`
	Headers       map[string]string `json:"headers"`
}

func ParseConfig(body []byte) (Config, error) {
	if len(body) < 2 || len(body) > MaxOTelConfigBytes {
		return Config{}, fail("INVALID_OTEL_CONFIG")
	}
	var result Config
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Config{}, fail("INVALID_OTEL_CONFIG")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Config{}, fail("INVALID_OTEL_CONFIG")
	}
	result, _, _, err := canonicalConfig(result)
	if err != nil {
		return Config{}, err
	}
	return result, nil
}

func canonicalConfig(
	value Config,
) (Config, string, string, error) {
	if value.SchemaVersion != OTelConfigSchemaVersion ||
		len(value.Headers) > maxOTelHeaders {
		return Config{}, "", "", fail("INVALID_OTEL_CONFIG")
	}
	traceURL, metricURL, err := signalURLs(value.Endpoint)
	if err != nil {
		return Config{}, "", "", err
	}
	canonicalHeaders := make(map[string]string, len(value.Headers))
	for name, headerValue := range value.Headers {
		canonicalName := http.CanonicalHeaderKey(name)
		if !headerNamePattern.MatchString(name) ||
			canonicalName == "" ||
			restrictedHeader(canonicalName) ||
			!validHeaderValue(headerValue) {
			return Config{}, "", "", fail("INVALID_OTEL_CONFIG")
		}
		if _, duplicate := canonicalHeaders[canonicalName]; duplicate {
			return Config{}, "", "", fail("INVALID_OTEL_CONFIG")
		}
		canonicalHeaders[canonicalName] = headerValue
	}
	value.Headers = canonicalHeaders
	return value, traceURL, metricURL, nil
}

func restrictedHeader(name string) bool {
	switch name {
	case "Host", "Content-Length", "Content-Type", "Content-Encoding",
		"Accept-Encoding", "Transfer-Encoding", "Connection", "Trailer",
		"Upgrade":
		return true
	default:
		return false
	}
}

func validHeaderValue(value string) bool {
	if len(value) < 1 || len(value) > 1024 {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}

func signalURLs(endpoint string) (string, string, error) {
	if len(endpoint) < 1 || len(endpoint) > 2048 {
		return "", "", fail("INVALID_OTEL_ENDPOINT")
	}
	value, err := url.Parse(endpoint)
	if err != nil || value.Scheme == "" || value.Host == "" ||
		value.User != nil || value.RawQuery != "" || value.Fragment != "" ||
		value.RawPath != "" {
		return "", "", fail("INVALID_OTEL_ENDPOINT")
	}
	host := value.Hostname()
	if host == "" || strings.Contains(host, "%") {
		return "", "", fail("INVALID_OTEL_ENDPOINT")
	}
	if value.Scheme != "https" {
		address := net.ParseIP(host)
		if value.Scheme != "http" || address == nil || !address.IsLoopback() {
			return "", "", fail("INVALID_OTEL_ENDPOINT")
		}
	}
	if value.Path == "" {
		value.Path = "/"
	}
	clean := path.Clean(value.Path)
	if clean != value.Path || !strings.HasPrefix(clean, "/") ||
		strings.Contains(value.Path, "//") ||
		strings.Contains(value.Path, `\`) {
		return "", "", fail("INVALID_OTEL_ENDPOINT")
	}
	value.Path = clean
	trace := *value
	trace.Path = path.Join(value.Path, "v1/traces")
	metric := *value
	metric.Path = path.Join(value.Path, "v1/metrics")
	return trace.String(), metric.String(), nil
}
