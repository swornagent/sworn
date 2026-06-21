package account

import (
	"strings"
	"testing"
	"time"
)

// TestProxyEndpointWithCreds verifies that when credentials are present,
// Endpoint returns a proxy URL containing the model ID.
func TestProxyEndpointWithCreds(t *testing.T) {
	t.Setenv("SWORN_PROXY_URL", "")

	creds := &Credentials{
		Token:     "tok_test",
		Email:     "user@example.com",
		Tier:      "pro",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	got := Endpoint(creds, "openai/gpt-4.1")
	if got == "" {
		t.Fatal("expected non-empty proxy URL, got empty string")
	}
	if !strings.Contains(got, "openai") {
		t.Errorf("proxy URL should contain model ID, got %q", got)
	}
	// Should use the compiled-in default host
	if !strings.HasPrefix(got, "https://api.swornagent.com") {
		t.Errorf("expected default host, got %q", got)
	}
}

// TestProxyEndpointNoCreds verifies that nil credentials returns empty string.
func TestProxyEndpointNoCreds(t *testing.T) {
	got := Endpoint(nil, "openai/gpt-4.1")
	if got != "" {
		t.Errorf("expected empty string for nil creds, got %q", got)
	}

	// Also test empty token
	creds := &Credentials{Token: ""}
	got = Endpoint(creds, "openai/gpt-4.1")
	if got != "" {
		t.Errorf("expected empty string for empty token, got %q", got)
	}
}

// TestProxyEndpointOverrideWarns verifies that when SWORN_PROXY_URL is set,
// the proxy URL uses the override host (pin B).
func TestProxyEndpointOverrideWarns(t *testing.T) {
	t.Setenv("SWORN_PROXY_URL", "http://localhost:9999")

	creds := &Credentials{
		Token:     "tok_test",
		Email:     "user@example.com",
		Tier:      "pro",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	got := Endpoint(creds, "openai/gpt-4.1")
	if !strings.HasPrefix(got, "http://localhost:9999") {
		t.Errorf("expected override host in proxy URL, got %q", got)
	}
}

// TestProxyEndpointModelIDEscaped verifies the model ID is properly
// URL-escaped in the proxy URL path.
func TestProxyEndpointModelIDEscaped(t *testing.T) {
	t.Setenv("SWORN_PROXY_URL", "")

	creds := &Credentials{
		Token:     "tok_test",
		Email:     "user@example.com",
		Tier:      "pro",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	got := Endpoint(creds, "openai/gpt-4.1")
	// The slash in "openai/gpt-4.1" should be escaped to %2F
	if !strings.Contains(got, "openai%2Fgpt-4.1") {
		t.Errorf("expected escaped model ID in proxy URL, got %q", got)
	}
}