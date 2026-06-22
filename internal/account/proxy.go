package account

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// defaultProxyHost is the production SwornAgent proxy host. Overridden at
// build time via -ldflags "-X main.defaultProxyHost=..." (mirrors S06a's
// authURL pattern). At runtime SWORN_PROXY_URL is a test-only override
// (Coach ack pin B).
var defaultProxyHost = "https://api.swornagent.com"

// ErrInsufficientCredits is returned when the proxy responds 402 Payment
// Required. The error message directs the user to `sworn account buy`.
// (Coach ack pin C — never silently downgrade to direct provider calls.)
var ErrInsufficientCredits = fmt.Errorf("out of SwornAgent credits — run `sworn account buy` to add more")

// Endpoint returns the SwornAgent proxy base URL for the given model ID
// when credentials are present. Returns "" when credentials are nil,
// meaning the caller should use direct provider routing.
//
// The proxy URL format is:
//
//	<host>/proxy/v1/<modelID>
//
// where <host> is the compiled-in default unless SWORN_PROXY_URL is set
// (test-only override, Coach ack pin B). When SWORN_PROXY_URL is set,
// a stderr warning is emitted because the sworn bearer token will be
// sent to a non-default host.
func Endpoint(creds *Credentials, modelID string) string {
	if creds == nil || creds.Token == "" {
		return ""
	}

	host := defaultProxyHost
	if override := os.Getenv("SWORN_PROXY_URL"); override != "" {
		fmt.Fprintf(os.Stderr,
			"warning: SWORN_PROXY_URL is set — sworn credentials will be routed to %s (non-default host)\n",
			override)
		host = strings.TrimRight(override, "/")
	}

	// Build the proxy URL: <host>/proxy/v1/<modelID>
	// modelID may contain a slash (e.g. "openai/gpt-4.1"); URL-encode the
	// model portion so the path is well-formed.
	encodedModel := url.PathEscape(modelID)
	return fmt.Sprintf("%s/proxy/v1/%s", host, encodedModel)
}

// IsProxyOverrideSet returns true when SWORN_PROXY_URL is explicitly set,
// indicating the bearer token will be sent to a non-default host. Used by
// tests to verify the credential-trust boundary (pin B).
func IsProxyOverrideSet() bool {
	return os.Getenv("SWORN_PROXY_URL") != ""
}
