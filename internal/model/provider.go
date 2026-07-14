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
	XAIKey              string
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

// ProviderConfigFromEnv builds the provider configuration from the ONE credential
// source: the canonical env var (OPENAI_API_KEY, ANTHROPIC_API_KEY, …), then
// credentials.json (XDG). See internal/model/credentials.go.
//
// There is no SWORN_-prefixed alias any more. A key is a key: if OPENAI_API_KEY is
// already exported for every other tool on the machine, sworn reads it rather than
// demanding a private duplicate.
//
// This was one of THREE key-resolution paths that disagreed with each other — this
// one honoured canonical names, swornProviderConfig read SWORN_-only, and FromEnv's
// switch did some of each. All three now call ProviderKey.
func ProviderConfigFromEnv() ProviderConfig {
	return ProviderConfig{
		OpenAIKey:           ProviderKey("openai"),
		DeepSeekKey:         ProviderKey("deepseek"),
		GroqKey:             ProviderKey("groq"),
		MistralKey:          ProviderKey("mistral"),
		OpenRouterKey:       ProviderKey("openrouter"),
		XAIKey:              ProviderKey("xai"),
		AnthropicKey:        ProviderKey("anthropic"),
		GoogleKey:           ProviderKey("google"),
		GoogleCloudProject:  os.Getenv("GOOGLE_CLOUD_PROJECT"),
		GoogleCloudLocation: os.Getenv("GOOGLE_CLOUD_LOCATION"),
		CloudflareKey:       ProviderKey("cloudflare"),
		GitHubToken:         ProviderKey("github"),
		OllamaHost:          ollamaHost(),
		AwsAccessKey:        ProviderKey("aws-access-key"),
		AwsSecretKey:        ProviderKey("aws-secret-key"),
		AwsRegion:           envOrAlias("AWS_REGION", "AWS_DEFAULT_REGION"),
		AzureAPIKey:         ProviderKey("azure"),
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

	case "xai":
		// xAI (Grok) is OpenAI chat/completions-compatible and accepts the
		// exact strict json_schema response_format shape (docs.x.ai
		// structured-outputs), so it rides the shared OAI chat client with
		// native structured output — no bespoke SDK (ADR-0007). This is the
		// native xai/ path; openrouter/x-ai/grok-* stays as an alternate route.
		return &OAI{
			BaseURL:    "https://api.x.ai/v1",
			Model:      model,
			APIKey:     pcfg.XAIKey,
			Structured: StructuredResponseFormat, // native strict json_schema (ADR-0011)
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
