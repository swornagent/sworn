package model

import (
	"os"
	"path/filepath"
	"testing"
)

// providerEnvCases enumerates every ProviderConfig credential field with the
// CANONICAL env var it now reads — the name the wider ecosystem already uses. There
// is no SWORN_-prefixed alias any more: a key is a key, and demanding a private
// duplicate of one you already have exported is a tax with no benefit.
var providerEnvCases = []struct {
	name      string
	canonical string
	get       func(ProviderConfig) string
}{
	{"OpenAIKey", "OPENAI_API_KEY", func(c ProviderConfig) string { return c.OpenAIKey }},
	{"DeepSeekKey", "DEEPSEEK_API_KEY", func(c ProviderConfig) string { return c.DeepSeekKey }},
	{"GroqKey", "GROQ_API_KEY", func(c ProviderConfig) string { return c.GroqKey }},
	{"MistralKey", "MISTRAL_API_KEY", func(c ProviderConfig) string { return c.MistralKey }},
	{"OpenRouterKey", "OPENROUTER_API_KEY", func(c ProviderConfig) string { return c.OpenRouterKey }},
	{"XAIKey", "XAI_API_KEY", func(c ProviderConfig) string { return c.XAIKey }},
	{"AnthropicKey", "ANTHROPIC_API_KEY", func(c ProviderConfig) string { return c.AnthropicKey }},
	{"GoogleKey", "GOOGLE_API_KEY", func(c ProviderConfig) string { return c.GoogleKey }},
	{"CloudflareKey", "CLOUDFLARE_API_KEY", func(c ProviderConfig) string { return c.CloudflareKey }},
	{"GitHubToken", "GITHUB_TOKEN", func(c ProviderConfig) string { return c.GitHubToken }},
	{"AwsAccessKey", "AWS_ACCESS_KEY_ID", func(c ProviderConfig) string { return c.AwsAccessKey }},
	{"AwsSecretKey", "AWS_SECRET_ACCESS_KEY", func(c ProviderConfig) string { return c.AwsSecretKey }},
	{"AzureAPIKey", "AZURE_OPENAI_API_KEY", func(c ProviderConfig) string { return c.AzureAPIKey }},
}

// isolateCredentials points the credential store at an empty temp file and clears
// the developer's real environment, so a test asserts against its fixture rather
// than against whatever keys happen to be exported on the machine running it.
func isolateCredentials(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir) // LegacyEnvPath() reads ~/.sworn/.env
	t.Setenv(CredentialsPathEnv, filepath.Join(dir, "credentials.json"))
	for _, tc := range providerEnvCases {
		t.Setenv(tc.canonical, "")
	}
	for _, legacy := range legacyKeyEnv {
		t.Setenv(legacy, "")
	}
	ResetCredentialsCacheForTest()
	t.Cleanup(ResetCredentialsCacheForTest)
	return dir
}

// TestProviderConfigFromEnv_CanonicalEnvIsRead — every provider reads its canonical
// env var, so a 12-factor deployment or CI can inject keys without a file.
func TestProviderConfigFromEnv_CanonicalEnvIsRead(t *testing.T) {
	for _, tc := range providerEnvCases {
		t.Run(tc.name, func(t *testing.T) {
			isolateCredentials(t)
			t.Setenv(tc.canonical, "canonical-value")

			if got := tc.get(ProviderConfigFromEnv()); got != "canonical-value" {
				t.Errorf("%s: %s was not read; got %q", tc.name, tc.canonical, got)
			}
		})
	}
}

// TestProviderConfigFromEnv_SwornPrefixIsGone is the INVERSION of the old contract.
//
// A SWORN_-prefixed environment used to light up dispatch (the "documented worker
// setup"). It no longer does. That is deliberate: the prefix was a private duplicate
// of a key the machine already had, and it was applied inconsistently — four
// providers honoured their canonical name, two of those on only ONE of three code
// paths, and the rest demanded the SWORN_ form.
//
// Failing here must be LOUD, not silent: `sworn doctor` reports legacy keys as
// stranded and `sworn init` migrates them into credentials.json.
func TestProviderConfigFromEnv_SwornPrefixIsGone(t *testing.T) {
	isolateCredentials(t)

	for _, legacy := range legacyKeyEnv {
		t.Setenv(legacy, "sworn-prefixed-value")
	}

	cfg := ProviderConfigFromEnv()
	for _, tc := range providerEnvCases {
		if got := tc.get(cfg); got == "sworn-prefixed-value" {
			t.Errorf("%s: a SWORN_-prefixed env var was still read — those names are gone; "+
				"keys belong in credentials.json or the canonical env var", tc.name)
		}
	}

	// But they must be FINDABLE, so doctor can tell the user and init can migrate.
	if len(FindLegacyCredentials()) == 0 {
		t.Error("legacy keys are unreadable AND invisible — a user would be stranded " +
			"with no idea why their configured setup stopped working")
	}
}

// TestProviderConfigFromEnv_ReadsCredentialsFile — the XDG JSON file is the other
// source, and it is what `sworn init` writes.
func TestProviderConfigFromEnv_ReadsCredentialsFile(t *testing.T) {
	dir := isolateCredentials(t)

	credPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"providers":{"openai":"from-file","anthropic":"anthropic-from-file"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	ResetCredentialsCacheForTest()

	cfg := ProviderConfigFromEnv()
	if cfg.OpenAIKey != "from-file" {
		t.Errorf("OpenAIKey = %q, want from-file — credentials.json must be read", cfg.OpenAIKey)
	}
	if cfg.AnthropicKey != "anthropic-from-file" {
		t.Errorf("AnthropicKey = %q, want anthropic-from-file", cfg.AnthropicKey)
	}
}

// TestEveryCommandSeesTheSameKeys is the guard for the defect that started this.
//
// The key store used to be loaded into the process environment by exactly ONE
// command (`sworn run` called model.LoadDotEnv; nothing else did). So a key written
// by `sworn init` was visible to the loop and invisible to llm-check, verify,
// reqverify and MCP — each resolved a model correctly and then failed for want of a
// key that was sitting on disk the whole time.
//
// The model layer now resolves keys itself, so there is no bootstrap step for a
// caller to forget. This asserts that: a key in the file is visible with NO setup.
func TestEveryCommandSeesTheSameKeys(t *testing.T) {
	dir := isolateCredentials(t)
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"),
		[]byte(`{"providers":{"openai":"sk-on-disk"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	ResetCredentialsCacheForTest()

	// No LoadDotEnv(). No os.Setenv. No command-specific bootstrap. Just ask.
	if got := ProviderKey("openai"); got != "sk-on-disk" {
		t.Errorf("ProviderKey = %q, want sk-on-disk — a key on disk must be visible to "+
			"EVERY caller without a per-command bootstrap step", got)
	}
	if got := ProviderConfigFromEnv().OpenAIKey; got != "sk-on-disk" {
		t.Errorf("ProviderConfigFromEnv().OpenAIKey = %q, want sk-on-disk", got)
	}
}
