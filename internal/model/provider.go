package model

import (
	"fmt"
	"os"
)

// ProviderConfig holds per-provider API keys and optional overrides.
// Fields use the canonical env var names (OPENAI_API_KEY, etc.).
type ProviderConfig struct {
	OpenAIKey           string
	DeepSeekKey         string
	GroqKey             string
	MistralKey          string
	OpenRouterKey       string
	AnthropicKey        string
	GoogleKey           string
	GoogleCloudProject  string
	GoogleCloudLocation string
	CloudflareKey       string
	GitHubToken         string
	OllamaHost          string // optional, defaults to http://localhost:11434
	AwsAccessKey        string
	AwsSecretKey        string
	AwsRegion           string // AWS region for Bedrock (fallback: AWS_REGION → AWS_DEFAULT_REGION → us-east-1)
	AzureAPIKey         string
	AzureEndpoint       string
	AzureAPIVersion     string
	// OCI SDK auth env vars (key_file, fingerprint, tenancy, region) are read
	// directly by the OCI driver (S15). OCICompartmentID is a SwornAgent-specific
	// routing param — not an SDK auth var — and is stored here.
	OCICompartmentID string
}

// ProviderConfigFromEnv reads per-provider configuration from environment
// variables. The SWORN_OPENAI_API_KEY alias is checked as a fallback when
// OPENAI_API_KEY is empty (backward compatibility per spec Risk #1).
func ProviderConfigFromEnv() ProviderConfig {
	return ProviderConfig{
		OpenAIKey:           envOrAlias("OPENAI_API_KEY", "SWORN_OPENAI_API_KEY"),
		DeepSeekKey:         os.Getenv("DEEPSEEK_API_KEY"),
		GroqKey:             os.Getenv("GROQ_API_KEY"),
		MistralKey:          os.Getenv("MISTRAL_API_KEY"),
		OpenRouterKey:       os.Getenv("OPENROUTER_API_KEY"),
		AnthropicKey:        os.Getenv("ANTHROPIC_API_KEY"),
		GoogleKey:           envOrAlias("GOOGLE_API_KEY", "SWORN_GOOGLE_API_KEY"),
		GoogleCloudProject:  os.Getenv("GOOGLE_CLOUD_PROJECT"),
		GoogleCloudLocation: os.Getenv("GOOGLE_CLOUD_LOCATION"),
		CloudflareKey:       os.Getenv("CLOUDFLARE_API_KEY"),
		GitHubToken:         os.Getenv("GITHUB_TOKEN"),
		OllamaHost:          ollamaHost(),
		AwsAccessKey:        os.Getenv("AWS_ACCESS_KEY_ID"),
		AwsSecretKey:        os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AwsRegion:           envOrAlias("AWS_REGION", "AWS_DEFAULT_REGION"),
		AzureAPIKey:         os.Getenv("AZURE_OPENAI_API_KEY"),
		AzureEndpoint:       os.Getenv("AZURE_OPENAI_ENDPOINT"),
		AzureAPIVersion:     os.Getenv("AZURE_OPENAI_API_VERSION"),
		OCICompartmentID:    os.Getenv("OCI_COMPARTMENT_ID"),
	}
}

// envOrAlias returns the value of the canonical env var, or the alias if the
// canonical is empty. This implements the spec's backward-compat requirement:
// canonical key wins; SWORN_OPENAI_API_KEY is a fallback only.
func envOrAlias(canonical, alias string) string {
	if v := os.Getenv(canonical); v != "" {
		return v
	}
	return os.Getenv(alias)
}

// ollamaHost returns the OLLAMA_HOST env var, or the default localhost URL.
func ollamaHost() string {
	if h := os.Getenv("OLLAMA_HOST"); h != "" {
		return h
	}
	return "http://localhost:11434"
}

// ErrDriverNotImplemented is returned when a model ID prefix maps to no
// registered utility-path client (unknown provider).
var ErrDriverNotImplemented = constErr("driver not implemented (not yet available; see slices S11-S16)")

// NewClient dispatches a model ID like "openai/gpt-4o" or "groq/llama-3.3-70b"
// to the correct driver. OAI-compat providers get an &OAI{} with the correct
// base URL preset. Native drivers return an appropriate implementation. Model
// IDs after the provider prefix are passed through as-is — the provider needs
// the full model name.
//
// Prefix semantics (sworn#31, S05-driver-registry): "openai" is the
// Responses API (/v1/responses); "openai-completions" is the legacy
// chat/completions wire format under its new explicit name;
// "openai-responses" is a deprecated alias of "openai", kept for one
// release. NewClient is the single authority for prefix meaning — the
// driver registry (internal/driver/registry) maps the same prefixes to the
// in-process drivers that re-resolve through this function, so enumeration
// and dispatch can never disagree.
func NewClient(modelID string, pcfg ProviderConfig) (Verifier, error) {
	provider, model, err := parseModelID(modelID)
	if err != nil {
		return nil, err
	}

	switch provider {
	case "openai":
		// sworn#31: openai/ now routes to the Responses API.
		return NewOpenAIResponses(model, pcfg.OpenAIKey)

	case "openai-completions":
		// The legacy chat/completions wire format under its explicit name.
		return &OAI{
			BaseURL:    "https://api.openai.com/v1",
			Model:      model,
			APIKey:     pcfg.OpenAIKey,
			Structured: StructuredResponseFormat, // native strict json_schema (ADR-0011)
		}, nil

	case "deepseek":
		return &OAI{
			BaseURL:    "https://api.deepseek.com/v1",
			Model:      model,
			APIKey:     pcfg.DeepSeekKey,
			Structured: StructuredToolCall, // no strict response_format; forced-tool fallback
		}, nil

	case "groq":
		return &OAI{
			BaseURL: "https://api.groq.com/openai/v1",
			Model:   model,
			APIKey:  pcfg.GroqKey,
		}, nil

	case "mistral":
		return &OAI{
			BaseURL: "https://api.mistral.ai/v1",
			Model:   model,
			APIKey:  pcfg.MistralKey,
		}, nil

	case "openrouter":
		// OpenRouter model IDs contain sub-paths like
		// openrouter/anthropic/claude-sonnet-4-6. parseModelID splits on
		// the first '/', so provider="openrouter" and model is everything
		// after the first slash — exactly the sub-path OpenRouter expects.
		return &OAI{
			BaseURL: "https://openrouter.ai/api/v1",
			Model:   model,
			APIKey:  pcfg.OpenRouterKey,
		}, nil

	case "ollama":
		// Native Ollama driver — uses POST /api/chat, not the OAI-compat
		// /v1/chat/completions shim. pcfg.OllamaHost already holds the raw
		// host (no /v1 suffix) via ollamaHost().
		return NewOllama(model, pcfg.OllamaHost), nil
	case "cloudflare":
		return &OAI{
			BaseURL: "https://api.cloudflare.com/client/v4/ai/v1",
			Model:   model,
			APIKey:  pcfg.CloudflareKey,
		}, nil

	case "github":
		return &OAI{
			BaseURL: "https://models.inference.ai.azure.com",
			Model:   model,
			APIKey:  pcfg.GitHubToken,
		}, nil

	case "openai-responses":
		// Deprecated alias of "openai" (sworn#31), kept for one release.
		fmt.Fprintf(os.Stderr,
			"warning: model prefix \"openai-responses/\" is deprecated — use \"openai/\" instead (sworn#31; the alias is kept for one release)\n")
		return NewOpenAIResponses(model, pcfg.OpenAIKey)

	// Native drivers.
	case "anthropic":
		return NewAnthropic(model, pcfg.AnthropicKey)
	case "google":
		return NewGoogleGemini(model, pcfg.GoogleKey)
	case "vertex":
		return NewGoogleVertex(model, pcfg.GoogleCloudProject, pcfg.GoogleCloudLocation)
	case "bedrock":
		return NewBedrock(model, pcfg.AwsRegion)
	case "azure":
		return NewAzureOAI(model, pcfg.AzureEndpoint, pcfg.AzureAPIKey, pcfg.AzureAPIVersion)
	case "oci":
		return NewOCI(model, pcfg.OCICompartmentID)

	// Subscription-based CLI driver — no API key, authenticates via the
	// user's logged-in CLI session (claude -p). codex/ is served by the
	// subprocess DRIVER via internal/driver/registry (S03/S05, closing
	// sworn#19), not by a model.Verifier — it falls to the default
	// unknown-provider error on this utility path.
	case "claude-cli":
		return newClaudeCLI(model), nil
	default:
		return nil, fmt.Errorf("%w: unknown provider %q", ErrDriverNotImplemented, provider)
	}
}
