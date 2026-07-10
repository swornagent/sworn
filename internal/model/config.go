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
// format "provider/model" (e.g. "openai/gpt-4.1").
//
// Env vars (backward-compat SWORN_* namespace):
//
//	SWORN_<UPPER_PROVIDER>_API_KEY  (required for direct provider routing)
//	SWORN_<UPPER_PROVIDER>_BASE_URL (optional; overrides the preset base URL)
//	SWORN_<UPPER_PROVIDER>_MODEL    (optional; overrides the model name from the flag)
//	SWORN_DIRECT=1                  (optional; bypass proxy, use provider key directly)
//
// Env vars (new canonical namespace — see ProviderConfigFromEnv):
//
//	OPENAI_API_KEY, DEEPSEEK_API_KEY, GROQ_API_KEY, etc.
//
// Proxy routing (S06b): when sworn login credentials are present and
// SWORN_DIRECT is not set, FromEnv routes through the SwornAgent proxy.
// The proxy URL is obtained from account.Endpoint(). The sworn bearer token
// is used as the API key for the proxy. When SWORN_DIRECT=1 is set, or no
// credentials are present, FromEnv falls back to direct provider routing
// (the pre-S06b behaviour).
//
// Direct routing now delegates to NewClient() (S10-provider-foundation),
// which dispatches by model ID prefix to the correct driver with preset
// base URLs for all OAI-compat providers.
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

	// Keyless subscription driver: claude-cli authenticates via the user's
	// logged-in CLI session — no API key, no proxy routing. Return the
	// subprocess driver directly, BEFORE the proxy routing block. claude-cli
	// is the only keyless model.Verifier: codex/ is served by the subprocess
	// DRIVER via internal/driver/registry (S03/S05), not by this utility path.
	if provider == "claude-cli" {
		verifier, err := NewClient(modelID, ProviderConfig{})
		if err != nil {
			return nil, err
		}
		return verifier, nil
	}

	prefix := strings.ToUpper(strings.ReplaceAll(provider, "-", "_"))

	// Proxy routing: ONE predicate (ProxyRoute) shared with ResolveLoopClient
	// and the registry's ViaProxy enumeration (S06 D6/R-04) — the capability
	// surface and the dispatch route must evaluate literally the same
	// condition. (Coach ack pin B — credential-trust boundary.)
	if baseURL, token, ok := ProxyRoute(modelID); ok {
		return proxyClient(provider, model, baseURL, token), nil
	}

	// Direct provider routing (S10-refactored).
	// Build a backward-compat ProviderConfig from SWORN_* env vars, then
	// delegate to NewClient for provider dispatch.
	//
	// Backward compat: check that the provider's API key is set before dispatch.
	// Vertex AI uses Application Default Credentials — no API key required.
	// Google Gemini uses either GOOGLE_API_KEY (canonical) or SWORN_GOOGLE_API_KEY.
	var key string
	switch provider {
	case "vertex":
		// Vertex AI uses ADC — no API key required.
		key = "adc"
	case "bedrock":
		// Bedrock uses IAM credentials — no API key required.
		key = "iam"
	case "oci":
		// OCI uses SDK config file (~/.oci/config) and OCI_CLI_REGION —
		// no API key required. The compartment ID is checked later.
		key = "compartment"
	case "google":
		key = envOrAlias("GOOGLE_API_KEY", "SWORN_GOOGLE_API_KEY")
	case "openai-responses":
		key = envOrAlias("OPENAI_API_KEY", "SWORN_OPENAI_API_KEY")
	case "openai-completions":
		// Shares the openai key: the generic default would demand
		// SWORN_OPENAI_COMPLETIONS_API_KEY, which nothing sets and which
		// swornProviderConfig would not read into OpenAIKey.
		key = os.Getenv("SWORN_OPENAI_API_KEY")
	case "azure":
		key = envOrAlias("AZURE_OPENAI_API_KEY", "SWORN_AZURE_OPENAI_API_KEY")
	default:
		key = os.Getenv("SWORN_" + prefix + "_API_KEY")
	}
	if key == "" {
		return nil, fmt.Errorf("model: SWORN_%s_API_KEY not set", prefix)
	}
	pcfg := swornProviderConfig()
	// Apply SWORN_<PREFIX>_MODEL override before dispatch.
	if envModel := os.Getenv("SWORN_" + prefix + "_MODEL"); envModel != "" {
		model = envModel
	}

	verifier, err := NewClient(modelID, pcfg)
	if err != nil {
		// If NewClient returned a not-registered error, surface it directly.
		if errorsIs(err, ErrDriverNotImplemented) {
			return nil, err
		}
		return nil, fmt.Errorf("model: %w", err)
	}

	// Apply SWORN_*_BASE_URL override for OAI clients (backward compat).
	if oai, ok := verifier.(*OAI); ok {
		if baseURL := os.Getenv("SWORN_" + prefix + "_BASE_URL"); baseURL != "" {
			if _, err := url.Parse(baseURL); err != nil {
				return nil, fmt.Errorf("model: invalid SWORN_%s_BASE_URL: %w", prefix, err)
			}
			oai.BaseURL = baseURL
		}
	}

	return verifier, nil
}

// ProxyRoute is the single proxy-routing predicate (S06 D6, R-04): it reports
// whether modelID currently routes through the SwornAgent proxy, and if so
// the proxy base URL and bearer token to use. The condition is exactly what
// FromEnv has always routed on: SWORN_DIRECT unset, sworn login credentials
// present, and account.Endpoint yielding a proxy URL for the model ID.
//
// Every surface that answers "does this dispatch go via the proxy?" —
// FromEnv, ResolveLoopClient (the in-process drivers' client resolution), and
// the registry's ViaProxy capability enumeration — MUST delegate here so
// advertised and actual routing can never disagree (the S06b/OpenRouter
// keyless-credits journey). No logging of the token (AGENTS.md Security).
func ProxyRoute(modelID string) (baseURL, token string, ok bool) {
	if os.Getenv("SWORN_DIRECT") == "1" {
		return "", "", false
	}
	creds, err := account.Load(filepath.Dir(account.CredentialsPath()))
	if err != nil || creds == nil || !account.IsLoggedIn(creds) {
		return "", "", false
	}
	proxyURL := account.Endpoint(creds, modelID)
	if proxyURL == "" {
		return "", "", false
	}
	return proxyURL, creds.Token, true
}

// proxyClient constructs the proxy-routed client for a provider prefix.
// Coach pin 1 (S39), re-keyed for sworn#31: openai/ and its deprecated alias
// openai-responses/ are the Responses API and need the responses-API
// provider, not the OAI chat/completions adapter; everything else speaks the
// legacy chat/completions wire format. The proxy URL + token are identical;
// only the struct type differs so /v1/responses is called.
func proxyClient(provider, model, baseURL, token string) Verifier {
	if provider == "openai" || provider == "openai-responses" {
		return &OpenAIResponses{
			BaseURL:         baseURL,
			Model:           model,
			APIKey:          token,
			ReasoningEffort: "medium",
		}
	}
	return &OAI{
		BaseURL: baseURL,
		Model:   model,
		APIKey:  token,
	}
}

// ResolveLoopClient is the FromEnv-equivalent client resolution the loop's
// in-process drivers use as their default (S06 D6): proxy route when
// ProxyRoute says so, otherwise direct via NewClient(modelID, pcfg). It
// exists so a registry-dispatched loop client satisfies the exact condition
// `sworn capabilities` advertises — replacing the proxy-blind bare NewClient
// default that would have made enumeration claim proxy while dispatch went
// direct (spec R-04).
func ResolveLoopClient(modelID string, pcfg ProviderConfig) (Verifier, error) {
	provider, model, err := parseModelID(modelID)
	if err != nil {
		return nil, err
	}
	// claude-cli is keyless and never proxy-routed — same carve-out as FromEnv.
	if provider != "claude-cli" {
		if baseURL, token, ok := ProxyRoute(modelID); ok {
			return proxyClient(provider, model, baseURL, token), nil
		}
	}
	return NewClient(modelID, pcfg)
}

// errorsIs is a local helper to check if err matches target, avoiding an
// import of the errors package (which would shadow our Error type in the
// model package).
func errorsIs(err, target error) bool {
	// Simple equality check; sufficient for sentinel errors like
	// ErrDriverNotImplemented which is a constErr.
	return err.Error() == target.Error()
}

// swornProviderConfig reads the backward-compat SWORN_* env var namespace
// into a ProviderConfig. This bridges existing SWORN_*_API_KEY env vars to
// the new canonical ProviderConfig used by NewClient.
func swornProviderConfig() ProviderConfig {
	return ProviderConfig{
		OpenAIKey:           os.Getenv("SWORN_OPENAI_API_KEY"),
		DeepSeekKey:         os.Getenv("SWORN_DEEPSEEK_API_KEY"),
		GroqKey:             os.Getenv("SWORN_GROQ_API_KEY"),
		MistralKey:          os.Getenv("SWORN_MISTRAL_API_KEY"),
		OpenRouterKey:       os.Getenv("SWORN_OPENROUTER_API_KEY"),
		AnthropicKey:        os.Getenv("SWORN_ANTHROPIC_API_KEY"),
		GoogleKey:           envOrAlias("GOOGLE_API_KEY", "SWORN_GOOGLE_API_KEY"),
		GoogleCloudProject:  os.Getenv("GOOGLE_CLOUD_PROJECT"),
		GoogleCloudLocation: os.Getenv("GOOGLE_CLOUD_LOCATION"),
		CloudflareKey:       os.Getenv("SWORN_CLOUDFLARE_API_KEY"),
		GitHubToken:         os.Getenv("SWORN_GITHUB_TOKEN"),
		OllamaHost:          ollamaHost(),
		AwsAccessKey:        os.Getenv("SWORN_AWS_ACCESS_KEY_ID"),
		AwsSecretKey:        os.Getenv("SWORN_AWS_SECRET_ACCESS_KEY"),
		AwsRegion:           envOrAlias("AWS_REGION", "AWS_DEFAULT_REGION"),
		AzureAPIKey:         os.Getenv("SWORN_AZURE_OPENAI_API_KEY"),
		AzureEndpoint:       os.Getenv("SWORN_AZURE_OPENAI_ENDPOINT"),
		AzureAPIVersion:     os.Getenv("SWORN_AZURE_OPENAI_API_VERSION"),
		OCICompartmentID:    os.Getenv("OCI_COMPARTMENT_ID")}
}

// parseModelID splits "provider/model" into its parts. The first "/" is the
// separator; model names that contain "/" are passed through as-is after the
// first slash — this correctly handles OpenRouter sub-paths like
// openrouter/anthropic/claude-sonnet-4-6 where provider="openrouter" and
// model="anthropic/claude-sonnet-4-6".
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
