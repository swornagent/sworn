package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/command"
)

// TestCapabilitiesCommandRegistered proves the verb is reachable through the
// integration point that owns it — the process-wide command registry that
// main.dispatch resolves from (Rule 1).
func TestCapabilitiesCommandRegistered(t *testing.T) {
	c, ok := command.Lookup("capabilities")
	if !ok {
		t.Fatal(`command.Lookup("capabilities") not found — init() in cmd/sworn/capabilities.go did not register`)
	}
	if c.Summary == "" {
		t.Error("Summary must be non-empty")
	}
	if c.Run == nil {
		t.Fatal("Run must be non-nil")
	}
}

// clearProviderEnv blanks every env var ProviderConfigFromEnv reads plus the
// proxy-routing vars so the enumeration under test is deterministic.
func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OPENAI_API_KEY", "OPENAI_API_KEY", "DEEPSEEK_API_KEY",
		"GROQ_API_KEY", "MISTRAL_API_KEY", "OPENROUTER_API_KEY",
		"ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GOOGLE_API_KEY",
		"CLOUDFLARE_API_KEY", "GITHUB_TOKEN",
		"XAI_API_KEY", "XAI_API_KEY",
		"SWORN_DIRECT", "SWORN_PROXY_URL",
	} {
		t.Setenv(k, "")
	}
	// No credentials file → not logged in to the proxy.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// TestCapabilitiesRendersRegistry runs the registered verb end-to-end
// (AC-01: `sworn capabilities` renders from registry enumeration; AC-04:
// the help/prefix documentation reflects the sworn#31 rename) and asserts
// the output carries the four drivers, the prefix table, and a
// key-presence availability flip — all without any server to dispatch to.
func TestCapabilitiesRendersRegistry(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")

	c, ok := command.Lookup("capabilities")
	if !ok {
		t.Fatal("capabilities verb not registered")
	}

	saved := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	code := c.Run(nil)
	w.Close()
	os.Stdout = saved
	outBytes, _ := io.ReadAll(r)
	out := string(outBytes)

	if code != 0 {
		t.Fatalf("capabilities exited %d, want 0\noutput:\n%s", code, out)
	}

	for _, want := range []string{
		"claude-subprocess",
		"codex-subprocess",
		"oai-responses-inprocess",
		"oai-inprocess",
		"openai/",
		"openai-completions/",
		"openai-responses/ (deprecated alias of openai/)",
		"claude-cli/",
		"codex/",
		"implementer,verifier",
		// sworn#31 prefix documentation (AC-04).
		"openai/ = Responses API",
		"openai-completions/ = legacy chat/completions",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\noutput:\n%s", want, out)
		}
	}

	// Key-driven availability: deepseek key present → the chat identity is
	// available and its detail names deepseek/.
	if !strings.Contains(out, "API keys present: deepseek/") {
		t.Errorf("output should show deepseek/ key availability\noutput:\n%s", out)
	}
	// Not logged in, so no proxy routing may be advertised.
	if strings.Contains(out, "via proxy:") {
		t.Errorf("output advertises proxy routing while logged out\noutput:\n%s", out)
	}
}

// runCapabilities executes the capabilities verb and returns its stdout.
func runCapabilities(t *testing.T) string {
	t.Helper()
	c, ok := command.Lookup("capabilities")
	if !ok {
		t.Fatal("capabilities verb not registered")
	}
	saved := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	code := c.Run(nil)
	w.Close()
	os.Stdout = saved
	outBytes, _ := io.ReadAll(r)
	out := string(outBytes)
	if code != 0 {
		t.Fatalf("capabilities exited %d, want 0\noutput:\n%s", code, out)
	}
	return out
}

// TestCapabilitiesListsXAI (S03 AC-02) proves `sworn capabilities` surfaces
// the native xai/ prefix: present-and-available when XAI_API_KEY is set,
// present-but-unavailable when absent — no dispatch either way. The prefix
// appears automatically because xai joined chatPrefixes (renderCapabilities
// is registry-driven, no per-prefix edit).
func TestCapabilitiesListsXAI(t *testing.T) {
	t.Run("key present -> available", func(t *testing.T) {
		clearProviderEnv(t)
		t.Setenv("XAI_API_KEY", "sk-xai")
		out := runCapabilities(t)
		if !strings.Contains(out, "xai/") {
			t.Errorf("output missing xai/ prefix\noutput:\n%s", out)
		}
		if !strings.Contains(out, "API keys present: xai/") {
			t.Errorf("output should show xai/ key availability\noutput:\n%s", out)
		}
	})

	t.Run("key absent -> listed but not available via key", func(t *testing.T) {
		clearProviderEnv(t)
		out := runCapabilities(t)
		// The prefix is still enumerated (it is a registered driver prefix).
		if !strings.Contains(out, "xai/") {
			t.Errorf("output missing xai/ prefix when key absent\noutput:\n%s", out)
		}
		// With no key and logged out, xai/ must NOT be advertised as available.
		if strings.Contains(out, "API keys present: xai/") {
			t.Errorf("output claims xai/ key availability with no key set\noutput:\n%s", out)
		}
	})
}
