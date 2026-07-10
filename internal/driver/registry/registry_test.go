package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/model"
)

// fakeDriver is a Driver whose Dispatch panics: the registry must never
// dispatch during resolution or enumeration, so any dispatch in these tests
// is an immediate loud failure.
type fakeDriver struct {
	name  string
	roles driver.RoleSet
}

func (f *fakeDriver) Name() string          { return f.name }
func (f *fakeDriver) Roles() driver.RoleSet { return f.roles }
func (f *fakeDriver) Dispatch(context.Context, driver.DispatchInput) (driver.Result, error) {
	panic("registry test: Dispatch must never be called by the registry")
}

// fullKeyConfig returns a ProviderConfig with every in-process key set so
// client construction (which fails closed on empty keys for native drivers)
// succeeds for every registered prefix.
func fullKeyConfig() model.ProviderConfig {
	return model.ProviderConfig{
		OpenAIKey:     "sk-openai",
		DeepSeekKey:   "sk-deepseek",
		GroqKey:       "sk-groq",
		MistralKey:    "sk-mistral",
		OpenRouterKey: "sk-openrouter",
		AnthropicKey:  "sk-anthropic",
		CloudflareKey: "sk-cloudflare",
		GitHubToken:   "sk-github",
	}
}

// TestDefaultRegistryTable (AC-01) pins the compiled-in table: exactly the
// four driver names, the D2 prefix sets, and Resolve returning the right
// driver by name for every registered prefix.
func TestDefaultRegistryTable(t *testing.T) {
	r := Default(fullKeyConfig())

	want := map[string][]string{
		"claude-subprocess":       {"claude-cli"},
		"codex-subprocess":        {"codex"},
		"oai-responses-inprocess": {"openai"},
		"oai-inprocess": {
			"anthropic", "cloudflare", "deepseek", "github",
			"groq", "mistral", "openai-completions", "openrouter",
		},
	}

	infos := r.Drivers()
	if len(infos) != len(want) {
		t.Fatalf("Drivers() returned %d entries, want %d", len(infos), len(want))
	}
	for _, info := range infos {
		wantPrefixes, ok := want[info.Name]
		if !ok {
			t.Errorf("unexpected registered driver %q", info.Name)
			continue
		}
		if got := strings.Join(info.Prefixes, ","); got != strings.Join(wantPrefixes, ",") {
			t.Errorf("driver %q prefixes = %s, want %s", info.Name, got, strings.Join(wantPrefixes, ","))
		}
	}

	// Resolve returns the right driver for each registered prefix.
	for name, prefixes := range want {
		for _, p := range prefixes {
			d, err := r.Resolve(p+"/some-model", driver.RoleImplementer)
			if err != nil {
				t.Errorf("Resolve(%q) error: %v", p+"/some-model", err)
				continue
			}
			if d.Name() != name {
				t.Errorf("Resolve(%q) = driver %q, want %q", p+"/some-model", d.Name(), name)
			}
		}
	}
}

// TestResolveUnknownPrefix (AC-02) asserts the unknown-prefix error names
// the unknown prefix AND contains the full registered-prefix list, aliases
// included and marked.
func TestResolveUnknownPrefix(t *testing.T) {
	r := Default(fullKeyConfig())
	_, err := r.Resolve("bedrock/claude-x", driver.RoleImplementer)
	if err == nil {
		t.Fatal("Resolve(bedrock/...) returned nil error, want unknown-prefix error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"bedrock/"`) {
		t.Errorf("error does not name the unknown prefix: %s", msg)
	}
	for _, p := range []string{
		"anthropic/", "claude-cli/", "cloudflare/", "codex/", "deepseek/",
		"github/", "groq/", "mistral/", "openai/", "openai-completions/",
		"openrouter/",
		"openai-responses/ (deprecated alias of openai/)",
	} {
		if !strings.Contains(msg, p) {
			t.Errorf("error missing registered prefix %q: %s", p, msg)
		}
	}
}

// TestResolveRoleFailFast (AC-03) asserts the role error names the driver,
// the missing role, and which registered drivers DO declare it — and that
// no fallback driver is ever returned.
func TestResolveRoleFailFast(t *testing.T) {
	// No compiled-in driver declares captain.
	r := Default(fullKeyConfig())
	d, err := r.Resolve("openai/gpt-5", driver.RoleCaptain)
	if err == nil {
		t.Fatal("Resolve(openai/..., captain) returned nil error")
	}
	if d != nil {
		t.Fatalf("Resolve returned a fallback driver %q alongside the role error", d.Name())
	}
	msg := err.Error()
	for _, part := range []string{`"oai-responses-inprocess"`, `"captain"`, "implementer,verifier", "(none)"} {
		if !strings.Contains(msg, part) {
			t.Errorf("role error missing %q: %s", part, msg)
		}
	}

	// Fake-driver variant: the error enumerates which drivers DO declare
	// the role, and the declared driver is never substituted.
	fr := New()
	fr.Register(Entry{
		Driver:   &fakeDriver{name: "no-captain", roles: driver.RoleSet{driver.RoleImplementer: true}},
		Prefixes: []string{"nocap"},
	})
	fr.Register(Entry{
		Driver:   &fakeDriver{name: "has-captain", roles: driver.RoleSet{driver.RoleCaptain: true}},
		Prefixes: []string{"hascap"},
	})
	d, err = fr.Resolve("nocap/m", driver.RoleCaptain)
	if err == nil {
		t.Fatal("Resolve(nocap/m, captain) returned nil error")
	}
	if d != nil {
		t.Fatalf("no-fallback violated: got driver %q, want nil", d.Name())
	}
	msg = err.Error()
	if !strings.Contains(msg, `"no-captain"`) || !strings.Contains(msg, "has-captain") {
		t.Errorf("role error must name the failing driver and the declaring drivers: %s", msg)
	}
}

// TestResolveMalformedID mirrors model.parseModelID's error contract.
func TestResolveMalformedID(t *testing.T) {
	r := Default(fullKeyConfig())
	for _, id := range []string{"gpt-4o", "/gpt-4o", "openai/"} {
		if _, err := r.Resolve(id, driver.RoleImplementer); err == nil {
			t.Errorf("Resolve(%q) returned nil error, want malformed-ID error", id)
		}
	}
}

// TestResolvePrefixRename (AC-04) pins the sworn#31 routing: openai/ is the
// Responses identity, openai-completions/ the chat identity, and
// openai-responses/ resolves as a deprecated alias WITH a captured warning.
func TestResolvePrefixRename(t *testing.T) {
	r := Default(fullKeyConfig())
	var warnings []string
	r.Warnf = func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	cases := []struct {
		modelID  string
		wantName string
	}{
		{"openai/gpt-5", "oai-responses-inprocess"},
		{"openai-completions/gpt-4.1", "oai-inprocess"},
		{"openai-responses/gpt-5", "oai-responses-inprocess"},
	}
	for _, c := range cases {
		d, err := r.Resolve(c.modelID, driver.RoleImplementer)
		if err != nil {
			t.Errorf("Resolve(%q) error: %v", c.modelID, err)
			continue
		}
		if d.Name() != c.wantName {
			t.Errorf("Resolve(%q) = %q, want %q", c.modelID, d.Name(), c.wantName)
		}
	}

	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 deprecation warning (for openai-responses/), got %d: %v", len(warnings), warnings)
	}
	w := warnings[0]
	if !strings.Contains(w, `"openai-responses/"`) || !strings.Contains(w, `"openai/"`) || !strings.Contains(w, "deprecated") {
		t.Errorf("deprecation warning must name old and new prefixes: %q", w)
	}
}

// TestDriversEnumeration (AC-05) proves enumeration is probe-driven and
// dispatch-free: injected fakes flip Available with no HTTP server in
// existence (there is nothing to dispatch TO), and fakeDriver.Dispatch
// panics if the registry ever tries.
func TestDriversEnumeration(t *testing.T) {
	for _, available := range []bool{true, false} {
		r := New()
		r.Register(Entry{
			Driver:   &fakeDriver{name: "probed", roles: driver.RoleSet{driver.RoleImplementer: true}},
			Prefixes: []string{"probed"},
			Probe: func() (bool, string) {
				return available, "injected probe"
			},
		})
		infos := r.Drivers()
		if len(infos) != 1 {
			t.Fatalf("Drivers() = %d entries, want 1", len(infos))
		}
		if infos[0].Available != available {
			t.Errorf("Available = %v, want %v", infos[0].Available, available)
		}
		if infos[0].Detail != "injected probe" {
			t.Errorf("Detail = %q, want injected probe", infos[0].Detail)
		}
		if infos[0].Roles.String() != "implementer" {
			t.Errorf("Roles = %s, want implementer", infos[0].Roles)
		}
	}

	// Default wiring: with only a deepseek key and the proxy forced off,
	// the chat identity is available and its detail names deepseek/.
	t.Setenv("SWORN_DIRECT", "1")
	r := Default(model.ProviderConfig{DeepSeekKey: "sk-deepseek"})
	for _, info := range r.Drivers() {
		if info.Name != "oai-inprocess" {
			continue
		}
		if !info.Available {
			t.Error("oai-inprocess should be available with a deepseek key present")
		}
		if !strings.Contains(info.Detail, "deepseek/") {
			t.Errorf("detail should name deepseek/: %q", info.Detail)
		}
		if len(info.ViaProxy) != 0 {
			t.Errorf("ViaProxy should be empty with SWORN_DIRECT=1, got %v", info.ViaProxy)
		}
	}
}

// writeTestCreds writes a logged-in sworn credentials file under a temp
// XDG_CONFIG_HOME (same fixture as internal/model's proxy tests).
func writeTestCreds(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	swornDir := filepath.Join(dir, "sworn")
	if err := os.MkdirAll(swornDir, 0700); err != nil {
		t.Fatalf("mkdir %s: %v", swornDir, err)
	}
	credsJSON := `{"token":"tok_proxy","email":"user@example.com","tier":"pro","expires_at":"2030-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(swornDir, "credentials.json"), []byte(credsJSON), 0600); err != nil {
		t.Fatalf("writing credentials: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// TestEnumerationShowsProxyRouting (AC-05, sworn#69): with an active proxy
// login and SWORN_DIRECT unset, enumeration lists which prefixes resolve
// through the proxy; SWORN_DIRECT=1 empties the list. Keyless subprocess
// prefixes never appear.
func TestEnumerationShowsProxyRouting(t *testing.T) {
	writeTestCreds(t)
	t.Setenv("SWORN_PROXY_URL", "")
	t.Setenv("SWORN_DIRECT", "")

	r := Default(model.ProviderConfig{})
	byName := map[string]Info{}
	for _, info := range r.Drivers() {
		byName[info.Name] = info
	}

	respVia := strings.Join(byName["oai-responses-inprocess"].ViaProxy, ",")
	if respVia != "openai" {
		t.Errorf("responses identity ViaProxy = %q, want openai", respVia)
	}
	chatVia := strings.Join(byName["oai-inprocess"].ViaProxy, ",")
	wantChat := "anthropic,cloudflare,deepseek,github,groq,mistral,openai-completions,openrouter"
	if chatVia != wantChat {
		t.Errorf("chat identity ViaProxy = %q, want %q", chatVia, wantChat)
	}
	for _, sub := range []string{"claude-subprocess", "codex-subprocess"} {
		if got := byName[sub].ViaProxy; len(got) != 0 {
			t.Errorf("%s ViaProxy = %v, want empty (keyless subprocess prefixes never proxy)", sub, got)
		}
	}
	// The keyless-but-logged-in identities are available via the proxy.
	if !byName["oai-inprocess"].Available || !byName["oai-responses-inprocess"].Available {
		t.Error("in-process identities should be available via proxy login with no keys")
	}

	// SWORN_DIRECT=1 makes the routing direct: ViaProxy empties.
	t.Setenv("SWORN_DIRECT", "1")
	for _, info := range r.Drivers() {
		if len(info.ViaProxy) != 0 {
			t.Errorf("driver %s ViaProxy = %v with SWORN_DIRECT=1, want empty", info.Name, info.ViaProxy)
		}
	}
}

// chatCapable is the wire-family assertion target for the chat identity:
// the constructed client must be drivable by the multi-turn agent loop.
type chatCapable interface {
	Chat(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error)
}

// TestRegistryNewClientConsistency (Coach ack pin 4): the registry table and
// model.NewClient's switch are two hand-synced prefix tables; this test
// iterates EVERY registered in-process prefix (aliases included) and asserts
// NewClient constructs the wire family the registered identity claims —
// Responses struct for the responses identity, chat-capable for every
// oai-inprocess prefix. Drift between the tables fails here, loudly.
func TestRegistryNewClientConsistency(t *testing.T) {
	cfg := fullKeyConfig()
	for _, info := range Default(cfg).Drivers() {
		var prefixes []string
		prefixes = append(prefixes, info.Prefixes...)
		for alias := range info.DeprecatedAliases {
			prefixes = append(prefixes, alias)
		}
		for _, p := range prefixes {
			modelID := p + "/m"
			switch info.Name {
			case "claude-subprocess", "codex-subprocess":
				// Subprocess drivers are not model.NewClient clients —
				// out of the consistency contract by design (D7).
				continue
			case "oai-responses-inprocess":
				c, err := model.NewClient(modelID, cfg)
				if err != nil {
					t.Errorf("NewClient(%q) error: %v", modelID, err)
					continue
				}
				if _, ok := c.(*model.OpenAIResponses); !ok {
					t.Errorf("NewClient(%q) = %T, want *model.OpenAIResponses (registry claims the Responses identity)", modelID, c)
				}
			case "oai-inprocess":
				c, err := model.NewClient(modelID, cfg)
				if err != nil {
					t.Errorf("NewClient(%q) error: %v", modelID, err)
					continue
				}
				if _, ok := c.(chatCapable); !ok {
					t.Errorf("NewClient(%q) = %T, not chat-capable (registry claims the chat identity)", modelID, c)
				}
			default:
				t.Errorf("unexpected registered driver %q — extend the consistency test", info.Name)
			}
		}
	}
}
