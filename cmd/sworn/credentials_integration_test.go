package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/model"
)

// TestCredentialDomainsSurviveCLIUpdates guards the real shared-file boundary:
// provider setup, login refresh, webhook configuration, and logout must update
// only the credential fields they own.
func TestCredentialDomainsSurviveCLIUpdates(t *testing.T) {
	dir := withConfigDir(t)
	model.ResetCredentialsCacheForTest()
	t.Cleanup(model.ResetCredentialsCacheForTest)

	if err := model.SaveCredentials(map[string]string{"openai": "provider-key"}); err != nil {
		t.Fatalf("save provider credential: %v", err)
	}
	session := account.Credentials{
		Token:     "session-token",
		Email:     "operator@example.test",
		Tier:      "test",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := account.SaveDefault(session); err != nil {
		t.Fatalf("save account session: %v", err)
	}
	if got := model.ProviderKey("openai"); got != "provider-key" {
		t.Fatalf("provider key after login = %q, want preserved key", got)
	}

	const webhook = "https://hooks.example.test/sworn"
	if code := cmdAccountSetWebhook([]string{webhook}); code != 0 {
		t.Fatalf("set-webhook exit = %d", code)
	}
	if err := account.SaveDefault(session); err != nil {
		t.Fatalf("refresh account session: %v", err)
	}
	if err := model.SaveCredentials(map[string]string{"mistral": "second-key"}); err != nil {
		t.Fatalf("save second provider credential: %v", err)
	}

	if code := cmdLogout(nil); code != 0 {
		t.Fatalf("logout exit = %d", code)
	}
	model.ResetCredentialsCacheForTest()
	if got := model.ProviderKey("openai"); got != "provider-key" {
		t.Fatalf("provider key after logout = %q, want preserved key", got)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"providers", "webhook_url"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("field %q did not survive the command sequence", name)
		}
	}
	for _, name := range []string{"token", "email", "tier", "expires_at"} {
		if _, ok := fields[name]; ok {
			t.Errorf("logout retained account-session field %q", name)
		}
	}

	loaded, err := account.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.WebhookURL != webhook {
		t.Fatalf("webhook after logout = %#v, want %q", loaded, webhook)
	}
}

func TestAccountCommandsHonorExactCredentialPathOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-secret-envelope.json")
	t.Setenv(model.CredentialsPathEnv, path)
	model.ResetCredentialsCacheForTest()
	t.Cleanup(model.ResetCredentialsCacheForTest)

	if err := model.SaveCredentials(map[string]string{"openai": "provider-key"}); err != nil {
		t.Fatal(err)
	}
	if err := account.SaveDefault(account.Credentials{Token: "session"}); err != nil {
		t.Fatal(err)
	}
	if code := cmdAccountSetWebhook([]string{"https://hooks.example.test/exact"}); code != 0 {
		t.Fatalf("set-webhook exit = %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("exact credential path was not updated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected default-named credential file: %v", err)
	}
}
