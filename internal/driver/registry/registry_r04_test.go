package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/model"
)

// initGitWorktree initialises a git repo in a temp dir so AssertWorktree
// passes for a real in-process Dispatch.
func initGitWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@swornagent.dev"},
		{"config", "user.name", "sworn test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// TestProxyRoutingAdvertisedEqualsActual is the BINDING R-04 reachability
// artefact (S06 D6, Coach ack pin 4 — three parts, one predicate):
//
//	(a) under the proxy login condition, `sworn capabilities` enumeration
//	    (Drivers()) advertises openai/ ViaProxy;
//	(b) a registry-resolved in-process driver's ACTUAL dispatch goes to the
//	    proxy host — observed server-side at an httptest proxy (the
//	    SWORN_PROXY_URL test-only override, credential-trust boundary);
//	(c) SWORN_DIRECT=1 flips BOTH surfaces off together: the advertisement
//	    empties AND the loop-client resolution constructs the direct route.
//
// Before S06 the two surfaces evaluated different predicates (registry
// re-implemented the login condition; the driver's client default was
// proxy-blind model.NewClient) — capabilities could claim proxy while
// dispatch went direct, regressing the keyless-credits journey.
func TestProxyRoutingAdvertisedEqualsActual(t *testing.T) {
	writeTestCreds(t)
	t.Setenv("SWORN_DIRECT", "")

	var hits atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// Any response terminates the observation — the dispatch outcome is
		// irrelevant; the routed REQUEST is the artefact.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"observed"}}`))
	}))
	defer proxy.Close()
	t.Setenv("SWORN_PROXY_URL", proxy.URL)

	// (a) Advertisement: the capabilities enumeration claims openai/ routes
	// via the proxy under the login condition.
	reg := Default(model.ProviderConfig{})
	advertised := false
	for _, info := range reg.Drivers() {
		if info.Name == "oai-responses-inprocess" {
			for _, p := range info.ViaProxy {
				if p == "openai" {
					advertised = true
				}
			}
		}
	}
	if !advertised {
		t.Fatal("(a) capabilities enumeration should advertise openai/ ViaProxy under the login condition")
	}

	// (b) Actual dispatch: resolve through the registry and dispatch — the
	// request must arrive at the proxy host, observed server-side.
	d, err := reg.Resolve("openai/gpt-4o", driver.RoleImplementer)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wt := initGitWorktree(t)
	_, _ = d.Dispatch(context.Background(), driver.DispatchInput{
		Role:         driver.RoleImplementer,
		ModelID:      "openai/gpt-4o",
		SystemPrompt: "system",
		Payload:      "payload",
		WorktreeRoot: wt,
	})
	if hits.Load() == 0 {
		t.Fatal("(b) registry-dispatched in-process driver did not hit the proxy host — advertised and actual routing disagree (R-04)")
	}

	// (c) SWORN_DIRECT=1 flips BOTH surfaces together.
	t.Setenv("SWORN_DIRECT", "1")
	for _, info := range reg.Drivers() {
		if len(info.ViaProxy) != 0 {
			t.Errorf("(c) driver %s still advertises ViaProxy=%v under SWORN_DIRECT=1", info.Name, info.ViaProxy)
		}
	}
	client, err := model.ResolveLoopClient("openai/gpt-4o", model.ProviderConfig{OpenAIKey: "sk-direct"})
	if err != nil {
		t.Fatalf("(c) ResolveLoopClient direct: %v", err)
	}
	resp, ok := client.(*model.OpenAIResponses)
	if !ok {
		t.Fatalf("(c) expected *model.OpenAIResponses for openai/ direct route, got %T", client)
	}
	if strings.Contains(resp.BaseURL, proxy.URL) || strings.Contains(resp.BaseURL, "/proxy/v1/") {
		t.Errorf("(c) direct route still points at the proxy: %q", resp.BaseURL)
	}

	// Same predicate, asserted at the source: ProxyRoute must be off.
	if _, _, on := model.ProxyRoute("openai/gpt-4o"); on {
		t.Error("(c) model.ProxyRoute still reports proxy routing under SWORN_DIRECT=1")
	}
}
