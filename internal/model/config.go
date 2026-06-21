package model

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/account"
)

// FromEnv resolves a Verifier from environment variables using the model ID
// format "provider/model" (e.g. "openai/gpt-4.1"). The prefix selects the
// env-var namespace; the suffix is the model name sent in the API request.
//
// Env vars:
//
//	SWORN_<UPPER_PROVIDER>_API_KEY  (required for direct provider routing)
//	SWORN_<UPPER_PROVIDER>_BASE_URL (optional; defaults vary by provider)
//	SWORN_<UPPER_PROVIDER>_MODEL    (optional; overrides the model name from the flag)
//	SWORN_DIRECT=1                  (optional; bypass proxy, use provider key directly)
//
// Proxy routing (S06b): when sworn login credentials are present and
// SWORN_DIRECT is not set, FromEnv routes through the SwornAgent proxy.
// The proxy URL is obtained from account.Endpoint(). The sworn bearer token
// is used as the API key for the proxy. When SWORN_DIRECT=1 is set, or no
// credentials are present, FromEnv falls back to direct provider routing
// (the pre-S06b behaviour).
//
// When provider is "openai" and SWORN_OPENAI_BASE_URL is unset, the default is
// https://api.openai.com/v1 — the safe-hosted default (trusted-jurisdiction). Any
// other provider requires an explicit BASE_URL.
//
// No logging of API keys. The key value is read once from the environment and
// never written to any log or stdout (per AGENTS.md Security).
func FromEnv(modelID string) (Verifier, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model: empty model ID")
	}

	provider, model, err := parseModelID(modelID)
	if err != nil {
		return nil, err
	}

	prefix := strings.ToUpper(strings.ReplaceAll(provider, "-", "_"))

	// Proxy routing: check for sworn credentials and SWORN_DIRECT override.
	// (Coach ack pin B — credential-trust boundary.)
	if os.Getenv("SWORN_DIRECT") != "1" {
		creds, credErr := account.Load(filepath.Dir(account.CredentialsPath()))
		if credErr == nil && creds != nil && account.IsLoggedIn(creds) {
			proxyURL := account.Endpoint(creds, modelID)
			if proxyURL != "" {
				return &OAI{
					BaseURL: proxyURL,
					Model:   model,
					APIKey:  creds.Token,
				}, nil
			}
		}
	}

	// Direct provider routing (pre-S06b behaviour).
	key := os.Getenv("SWORN_" + prefix + "_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("model: SWORN_%s_API_KEY not set", prefix)
	}

	baseURL := os.Getenv("SWORN_" + prefix + "_BASE_URL")
	if baseURL == "" {
		if provider == "openai" {
			baseURL = "https://api.openai.com/v1"
		} else {
			return nil, fmt.Errorf("model: SWORN_%s_BASE_URL not set (required for provider %q)", prefix, provider)
		}
	}

	if envModel := os.Getenv("SWORN_" + prefix + "_MODEL"); envModel != "" {
		model = envModel
	}

	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("model: invalid SWORN_%s_BASE_URL: %w", prefix, err)
	}

	return &OAI{
		BaseURL: baseURL,
		Model:   model,
		APIKey:  key,
	}, nil
}
// parseModelID splits "provider/model" into its parts. The first "/" is the
// separator; model names that contain "/" are not yet handled (flag for S10).
func parseModelID(modelID string) (provider, model string, err error) {
	idx := strings.IndexByte(modelID, '/')
	if idx < 0 {
		return "", "", fmt.Errorf("model: invalid model ID %q (want provider/model)", modelID)
	}
	provider = modelID[:idx]
	model = modelID[idx+1:]
	if provider == "" || model == "" {
		return "", "", fmt.Errorf("model: invalid model ID %q (provider and model required)", modelID)
	}
	return provider, model, nil
}
