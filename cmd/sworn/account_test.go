package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/account"
)

// captureStdout runs fn with os.Stdout replaced by a pipe and returns the
// captured output. It mirrors the idiom used by cmd/sworn/memory_test.go and
// doctor_test.go (os.Pipe + io.Copy to a bytes.Buffer).
func captureStdout(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	code := fn()
	w.Close()
	os.Stdout = origStdout
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	r.Close()
	return code, buf.String()
}

// withConfigDir points the account package at an isolated temp config dir by
// setting XDG_CONFIG_HOME (os.UserConfigDir reads it on Linux). configDir()
// appends "/sworn", so credentials land at <tmp>/sworn/credentials.json. It
// returns the sworn config dir so tests can read the credentials file back.
func withConfigDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	swornDir := filepath.Join(base, "sworn")
	if err := os.MkdirAll(swornDir, 0o700); err != nil {
		t.Fatalf("mkdir sworn dir: %v", err)
	}
	return swornDir
}

// TestAccountSetWebhookThenNotifications is the S07-paging AC5 round-trip
// test (the verifier's violation #2): `sworn account set-webhook <url>` must
// store the URL in the credentials file and `sworn account notifications` must
// show it. It drives both subcommands through their CLI entry functions
// (cmdAccountSetWebhook / cmdAccountNotifications) and asserts the on-disk
// credentials round-trip (Save → Load).
func TestAccountSetWebhookThenNotifications(t *testing.T) {
	swornDir := withConfigDir(t)

	webhookURL := "https://hooks.example.com/sworn"

	// 1. set-webhook — store the URL. Confirmation goes to stderr; we only
	// need the exit code here (0 = success).
	exit := cmdAccountSetWebhook([]string{webhookURL})
	if exit != 0 {
		t.Fatalf("set-webhook exit = %d, want 0", exit)
	}

	// 2. Round-trip on disk: Load the credentials file the CLI just wrote and
	// assert the WebhookURL field carries the value (AC5 first half).
	creds, err := account.Load(swornDir)
	if err != nil {
		t.Fatalf("Load after set-webhook: %v", err)
	}
	if creds == nil {
		t.Fatal("credentials file not created by set-webhook")
	}
	if creds.WebhookURL != webhookURL {
		t.Errorf("on-disk WebhookURL = %q, want %q", creds.WebhookURL, webhookURL)
	}

	// 3. notifications — must print the URL to stdout and exit 0 (AC5 second
	// half). Capture stdout the same way memory_test.go does.
	exit, out := captureStdout(t, func() int {
		return cmdAccountNotifications([]string{})
	})
	if exit != 0 {
		t.Fatalf("notifications exit = %d, want 0", exit)
	}
	if !strings.Contains(out, webhookURL) {
		t.Errorf("notifications output missing webhook URL %q\ngot:\n%s", webhookURL, out)
	}
	if !strings.Contains(out, "Webhook URL:") {
		t.Errorf("notifications output missing 'Webhook URL:' label\ngot:\n%s", out)
	}
}

// TestAccountSetWebhook_PersistsAcrossLoad confirms the stored URL survives a
// fresh Load (the credentials JSON serialises webhook_url with the omitempty
// tag, so an empty value would drop the field — this guards against a
// regression where Save omits the field).
func TestAccountSetWebhook_PersistsAcrossLoad(t *testing.T) {
	swornDir := withConfigDir(t)

	url := "https://hooks.example.com/sworn-2"
	if exit := cmdAccountSetWebhook([]string{url}); exit != 0 {
		t.Fatalf("set-webhook exit = %d, want 0", exit)
	}

	// Read the raw JSON to confirm the webhook_url key is present.
	raw, err := os.ReadFile(filepath.Join(swornDir, "credentials.json"))
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal credentials: %v", err)
	}
	val, ok := fields["webhook_url"]
	if !ok {
		t.Fatal("webhook_url key missing from credentials.json")
	}
	var got string
	if err := json.Unmarshal(val, &got); err != nil {
		t.Fatalf("unmarshal webhook_url: %v", err)
	}
	if got != url {
		t.Errorf("webhook_url = %q, want %q", got, url)
	}
}

// TestAccountNotifications_NoWebhook shows the not-configured message when no
// webhook is set (and no credentials file exists), guarding the empty path.
func TestAccountNotifications_NoWebhook(t *testing.T) {
	withConfigDir(t)

	exit, out := captureStdout(t, func() int {
		return cmdAccountNotifications([]string{})
	})
	if exit != 0 {
		t.Fatalf("notifications exit = %d, want 0", exit)
	}
	if !strings.Contains(out, "No webhook configured") {
		t.Errorf("expected 'No webhook configured' message, got:\n%s", out)
	}
}

// TestAccountSetWebhook_MissingURL asserts the usage error path returns
// exit 64 (EX_USAGE) when no URL argument is supplied.
func TestAccountSetWebhook_MissingURL(t *testing.T) {
	withConfigDir(t)
	exit := cmdAccountSetWebhook([]string{})
	if exit != 64 {
		t.Errorf("set-webhook with no URL: exit = %d, want 64", exit)
	}
}
