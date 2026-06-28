package model

import (
	"errors"
	"os"
	"testing"
)

func TestNewClient_OAICompat(t *testing.T) {
	cfg := ProviderConfig{
		OpenAIKey:     "sk-test",
		DeepSeekKey:   "ds-test",
		GroqKey:       "groq-test",
		MistralKey:    "mistral-test",
		OpenRouterKey: "or-test",
		CloudflareKey: "cf-test",
		GitHubToken:   "gh-test",
	}

	tests := []struct {
		modelID string
		wantURL string
		wantKey string
	}{
		{"openai/gpt-4o", "https://api.openai.com/v1", "sk-test"},
		{"deepseek/deepseek-chat", "https://api.deepseek.com/v1", "ds-test"},
		{"groq/llama-3.3-70b", "https://api.groq.com/openai/v1", "groq-test"},
		{"mistral/mistral-large", "https://api.mistral.ai/v1", "mistral-test"},
		{"openrouter/anthropic/claude-sonnet-4-6", "https://openrouter.ai/api/v1", "or-test"},
		{"cloudflare/@cf/meta/llama-3", "https://api.cloudflare.com/client/v4/ai/v1", "cf-test"},
		{"github/gpt-4o", "https://models.inference.ai.azure.com", "gh-test"},
	}

	for _, tt := range tests {
		v, err := NewClient(tt.modelID, cfg)
		if err != nil {
			t.Errorf("NewClient(%q) error: %v", tt.modelID, err)
			continue
		}
		oai, ok := v.(*OAI)
		if !ok {
			t.Errorf("NewClient(%q) returned %T, want *OAI", tt.modelID, v)
			continue
		}
		if oai.BaseURL != tt.wantURL {
			t.Errorf("NewClient(%q) BaseURL = %q, want %q", tt.modelID, oai.BaseURL, tt.wantURL)
		}
		if oai.APIKey != tt.wantKey {
			t.Errorf("NewClient(%q) APIKey = %q, want %q", tt.modelID, oai.APIKey, tt.wantKey)
		}
		if oai.Model == "" {
			t.Errorf("NewClient(%q) Model is empty", tt.modelID)
		}
	}
}

func TestNewClient_Ollama(t *testing.T) {
	// With explicit OllamaHost set.
	cfg := ProviderConfig{OllamaHost: "http://ollama.local:11434"}
	v, err := NewClient("ollama/llama3", cfg)
	if err != nil {
		t.Fatalf("NewClient(ollama/llama3) error: %v", err)
	}
	o, ok := v.(*Ollama)
	if !ok {
		t.Fatalf("NewClient(ollama/llama3) returned %T, want *Ollama", v)
	}
	if o.Host != "http://ollama.local:11434" {
		t.Errorf("Host = %q, want http://ollama.local:11434", o.Host)
	}
	if o.Model != "llama3" {
		t.Errorf("Model = %q, want llama3", o.Model)
	}

	// With default OllamaHost (empty cfg). NewOllama resolves $OLLAMA_HOST
	// or falls back to http://localhost:11434.
	os.Unsetenv("OLLAMA_HOST")
	cfg2 := ProviderConfig{}
	v2, err := NewClient("ollama/llama3", cfg2)
	if err != nil {
		t.Fatalf("NewClient(ollama/llama3, default) error: %v", err)
	}
	o2, ok := v2.(*Ollama)
	if !ok {
		t.Fatalf("NewClient(ollama/llama3, default) returned %T, want *Ollama", v2)
	}
	if o2.Host != "http://localhost:11434" {
		t.Errorf("Default Host = %q, want http://localhost:11434", o2.Host)
	}
}
func TestNewClient_NativeStub(t *testing.T) {
	cfg := ProviderConfig{}
	// All native drivers are now registered (S10-S15). This list exists to
	// catch future additions — add a not-yet-implemented provider ID here.
	nativeProviders := []string{}
	for _, modelID := range nativeProviders {
		_, err := NewClient(modelID, cfg)
		if err == nil {
			t.Errorf("NewClient(%q) returned nil error, want ErrDriverNotImplemented", modelID)
			continue
		}
		if !errors.Is(err, ErrDriverNotImplemented) {
			t.Errorf("NewClient(%q) error = %v, want ErrDriverNotImplemented", modelID, err)
		}
	}
}

func TestNewClient_Unknown(t *testing.T) {
	_, err := NewClient("unknown/model", ProviderConfig{})
	if err == nil {
		t.Fatal("NewClient(unknown/model) returned nil error")
	}
	if !errors.Is(err, ErrDriverNotImplemented) {
		t.Errorf("error = %v, want ErrDriverNotImplemented", err)
	}
}

func TestNewClient_OpenRouterSubPath(t *testing.T) {
	// OpenRouter model IDs contain sub-paths. Verify the model is passed
	// through verbatim after stripping the openrouter/ prefix.
	cfg := ProviderConfig{OpenRouterKey: "or-test"}
	v, err := NewClient("openrouter/anthropic/claude-sonnet-4-6", cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	oai := v.(*OAI)
	// The model should be the full sub-path.
	if oai.Model != "anthropic/claude-sonnet-4-6" {
		t.Errorf("Model = %q, want anthropic/claude-sonnet-4-6", oai.Model)
	}
}

func TestProviderConfigFromEnv(t *testing.T) {
	// Clear all provider env vars first to avoid real env leak.
	for _, k := range []string{
		"OPENAI_API_KEY", "DEEPSEEK_API_KEY", "GROQ_API_KEY", "MISTRAL_API_KEY",
		"OPENROUTER_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY",
		"CLOUDFLARE_API_KEY", "GITHUB_TOKEN", "OLLAMA_HOST",
		"SWORN_OPENAI_API_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
		"AZURE_OPENAI_API_KEY",
	} {
		t.Setenv(k, "")
	}

	// Set a subset of env vars.
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("GROQ_API_KEY", "gsk-groq")
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")
	t.Setenv("OLLAMA_HOST", "http://ollama:12345")

	cfg := ProviderConfigFromEnv()
	if cfg.OpenAIKey != "sk-openai" {
		t.Errorf("OpenAIKey = %q, want sk-openai", cfg.OpenAIKey)
	}
	if cfg.GroqKey != "gsk-groq" {
		t.Errorf("GroqKey = %q, want gsk-groq", cfg.GroqKey)
	}
	if cfg.DeepSeekKey != "sk-deepseek" {
		t.Errorf("DeepSeekKey = %q, want sk-deepseek", cfg.DeepSeekKey)
	}
	if cfg.OllamaHost != "http://ollama:12345" {
		t.Errorf("OllamaHost = %q, want http://ollama:12345", cfg.OllamaHost)
	}

	// Unset keys should be empty.
	if cfg.MistralKey != "" {
		t.Errorf("MistralKey = %q, want empty", cfg.MistralKey)
	}
}

func TestProviderConfigFromEnv_SwornOpenAIAlias(t *testing.T) {
	// When OPENAI_API_KEY is empty, SWORN_OPENAI_API_KEY should be used.
	t.Setenv("OPENAI_API_KEY", "") // Ensure canonical is empty.
	t.Setenv("SWORN_OPENAI_API_KEY", "sk-sworn-alias")
	cfg := ProviderConfigFromEnv()

	if cfg.OpenAIKey != "sk-sworn-alias" {
		t.Errorf("OpenAIKey = %q, want sk-sworn-alias (alias fallback)", cfg.OpenAIKey)
	}
}

func TestProviderConfigFromEnv_CanonicalWins(t *testing.T) {
	// When both canonical and alias are set, canonical wins.
	t.Setenv("OPENAI_API_KEY", "sk-canonical")
	t.Setenv("SWORN_OPENAI_API_KEY", "sk-alias")
	cfg := ProviderConfigFromEnv()

	if cfg.OpenAIKey != "sk-canonical" {
		t.Errorf("OpenAIKey = %q, want sk-canonical (canonical should win)", cfg.OpenAIKey)
	}
}

func TestNewClient_EmptyModelID(t *testing.T) {
	_, err := NewClient("", ProviderConfig{})
	if err == nil {
		t.Fatal("NewClient(\"\") returned nil error")
	}
}

func TestNewClient_InvalidFormat(t *testing.T) {
	_, err := NewClient("no-slash", ProviderConfig{})
	if err == nil {
		t.Fatal("NewClient(\"no-slash\") returned nil error")
	}

	_, err = NewClient("/modelonly", ProviderConfig{})
	if err == nil {
		t.Fatal("NewClient(\"/modelonly\") returned nil error")
	}
}

func TestOllamaHostDefault(t *testing.T) {
	// When OLLAMA_HOST is unset, the default should be localhost.
	os.Unsetenv("OLLAMA_HOST")
	host := ollamaHost()
	if host != "http://localhost:11434" {
		t.Errorf("ollamaHost() = %q, want http://localhost:11434", host)
	}
}

func TestOllamaHostCustom(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://gpu-box:9999")
	host := ollamaHost()
	if host != "http://gpu-box:9999" {
		t.Errorf("ollamaHost() = %q, want http://gpu-box:9999", host)
	}
}
