package model

import (
	"fmt"
	"os"
	"strings"
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
	OllamaHost          string // optional, defaults to http://localhost:11434/v1
	AwsAccessKey        string
	AwsSecretKey        string
	AwsRegion           string // AWS region for Bedrock (fallback: AWS_REGION → AWS_DEFAULT_REGION → us-east-1)
	AzureOpenAIKey      string
	// OCI SDK env vars are read directly by the OCI driver (S15); not stored here.
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
		AzureOpenAIKey:      os.Getenv("AZURE_OPENAI_API_KEY"),	}
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

// ErrDriverNotRegistered is returned when a model ID prefix maps to a native
// driver (Anthropic, Google, Bedrock, Azure, OCI) that has not yet been
// implemented. The error message names the slice that will add the driver.
var ErrDriverNotRegistered = constErr("driver not registered (not yet implemented; see slices S11-S16)")

// NewClient dispatches a model ID like "openai/gpt-4o" or "groq/llama-3.3-70b"
// to the correct driver. OAI-compat providers get an &OAI{} with the correct
// base URL preset. Native drivers return ErrDriverNotRegistered until their
// implementation slice lands (S11-S16). Model IDs after the provider prefix
// are passed through as-is — the provider needs the full model name.
func NewClient(modelID string, pcfg ProviderConfig) (Verifier, error) {
	provider, model, err := parseModelID(modelID)
	if err != nil {
		return nil, err
	}

	switch provider {
	case "openai":
		return &OAI{
			BaseURL: "https://api.openai.com/v1",
			Model:   model,
			APIKey:  pcfg.OpenAIKey,
		}, nil

	case "deepseek":
		return &OAI{
			BaseURL: "https://api.deepseek.com/v1",
			Model:   model,
			APIKey:  pcfg.DeepSeekKey,
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
		base := pcfg.OllamaHost
		if base == "" {
			base = "http://localhost:11434"
		}
		return &OAI{
			BaseURL: strings.TrimRight(base, "/") + "/v1",
			Model:   model,
			APIKey:  "ollama", // Ollama doesn't require auth by default
		}, nil

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

	// Native drivers — not yet registered. The error message names the
	// slice that will add each driver so users know what's missing.
	case "anthropic":
		return NewAnthropic(model, pcfg.AnthropicKey)
	case "google":
		return NewGoogleGemini(model, pcfg.GoogleKey)
	case "vertex":
		return NewGoogleVertex(model, pcfg.GoogleCloudProject, pcfg.GoogleCloudLocation)
	case "bedrock":
		return NewBedrock(model, pcfg.AwsRegion)
	case "azure":		return nil, fmt.Errorf("%w: azure driver lands in S14-azure-driver", ErrDriverNotRegistered)
	case "oci":
		return nil, fmt.Errorf("%w: oci driver lands in S15-oci-driver", ErrDriverNotRegistered)

	default:
		return nil, fmt.Errorf("%w: unknown provider %q", ErrDriverNotRegistered, provider)
	}
}
