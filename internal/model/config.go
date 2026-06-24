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
	case "google":		key = envOrAlias("GOOGLE_API_KEY", "SWORN_GOOGLE_API_KEY")
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
		if errorsIs(err, ErrDriverNotRegistered) {
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

// errorsIs is a local helper to check if err matches target, avoiding an
// import of the errors package (which would shadow our Error type in the
// model package).
func errorsIs(err, target error) bool {
	// Simple equality check; sufficient for sentinel errors like
	// ErrDriverNotRegistered which is a constErr.
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
		OCICompartmentID:    os.Getenv("OCI_COMPARTMENT_ID"),	}
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
