package model

import (
	"testing"
)

// providerEnvCases enumerates EVERY key widened in S06 D7 (Coach ack pin 5):
// each ProviderConfig field that gained a SWORN_* fallback, with a getter to
// read it back off the struct.
var providerEnvCases = []struct {
	name      string
	canonical string
	alias     string
	get       func(ProviderConfig) string
}{
	{"OpenAIKey", "OPENAI_API_KEY", "SWORN_OPENAI_API_KEY", func(c ProviderConfig) string { return c.OpenAIKey }},
	{"DeepSeekKey", "DEEPSEEK_API_KEY", "SWORN_DEEPSEEK_API_KEY", func(c ProviderConfig) string { return c.DeepSeekKey }},
	{"GroqKey", "GROQ_API_KEY", "SWORN_GROQ_API_KEY", func(c ProviderConfig) string { return c.GroqKey }},
	{"MistralKey", "MISTRAL_API_KEY", "SWORN_MISTRAL_API_KEY", func(c ProviderConfig) string { return c.MistralKey }},
	{"OpenRouterKey", "OPENROUTER_API_KEY", "SWORN_OPENROUTER_API_KEY", func(c ProviderConfig) string { return c.OpenRouterKey }},
	{"AnthropicKey", "ANTHROPIC_API_KEY", "SWORN_ANTHROPIC_API_KEY", func(c ProviderConfig) string { return c.AnthropicKey }},
	{"GoogleKey", "GOOGLE_API_KEY", "SWORN_GOOGLE_API_KEY", func(c ProviderConfig) string { return c.GoogleKey }},
	{"CloudflareKey", "CLOUDFLARE_API_KEY", "SWORN_CLOUDFLARE_API_KEY", func(c ProviderConfig) string { return c.CloudflareKey }},
	{"GitHubToken", "GITHUB_TOKEN", "SWORN_GITHUB_TOKEN", func(c ProviderConfig) string { return c.GitHubToken }},
	{"AwsAccessKey", "AWS_ACCESS_KEY_ID", "SWORN_AWS_ACCESS_KEY_ID", func(c ProviderConfig) string { return c.AwsAccessKey }},
	{"AwsSecretKey", "AWS_SECRET_ACCESS_KEY", "SWORN_AWS_SECRET_ACCESS_KEY", func(c ProviderConfig) string { return c.AwsSecretKey }},
	{"AzureAPIKey", "AZURE_OPENAI_API_KEY", "SWORN_AZURE_OPENAI_API_KEY", func(c ProviderConfig) string { return c.AzureAPIKey }},
	{"AzureEndpoint", "AZURE_OPENAI_ENDPOINT", "SWORN_AZURE_OPENAI_ENDPOINT", func(c ProviderConfig) string { return c.AzureEndpoint }},
	{"AzureAPIVersion", "AZURE_OPENAI_API_VERSION", "SWORN_AZURE_OPENAI_API_VERSION", func(c ProviderConfig) string { return c.AzureAPIVersion }},
}

// TestProviderConfigFromEnvCanonicalWins is the S06 D7 precedence proof
// (Coach ack pin 5): for EVERY widened key, when both the canonical env var
// and its SWORN_* alias are set, the canonical value wins — "strictly
// additive" is proven, not asserted.
func TestProviderConfigFromEnvCanonicalWins(t *testing.T) {
	for _, tc := range providerEnvCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.canonical, "canonical-value")
			t.Setenv(tc.alias, "sworn-fallback-value")
			cfg := ProviderConfigFromEnv()
			if got := tc.get(cfg); got != "canonical-value" {
				t.Errorf("%s: canonical %s must win over alias %s; got %q",
					tc.name, tc.canonical, tc.alias, got)
			}
		})
	}
}

// TestProviderConfigFromEnvSwornFallback proves the other half of the D7
// widening: a SWORN_*-only environment (the documented worker setup) still
// lights up direct dispatch — every widened key falls back to its SWORN_*
// alias when the canonical var is empty.
func TestProviderConfigFromEnvSwornFallback(t *testing.T) {
	for _, tc := range providerEnvCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.canonical, "")
			t.Setenv(tc.alias, "sworn-fallback-value")
			cfg := ProviderConfigFromEnv()
			if got := tc.get(cfg); got != "sworn-fallback-value" {
				t.Errorf("%s: empty canonical %s must fall back to alias %s; got %q",
					tc.name, tc.canonical, tc.alias, got)
			}
		})
	}
}
