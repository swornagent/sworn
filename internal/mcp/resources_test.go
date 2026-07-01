package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBatonRulesResourceRead exercises the AGENTS.md-advertised URI
// sworn://baton/rules end-to-end through the JSON-RPC dispatch: the
// registered resource must serve the embedded 11-rule Baton content.
func TestBatonRulesResourceRead(t *testing.T) {
	w, r, s := testRoundTrip(t)
	RegisterResources(s, t.TempDir())

	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)

	sendRequest(t, w, "resources/read", jsonID(2), json.RawMessage(`{"uri":"sworn://baton/rules"}`))
	resp := readResponse(t, r)

	if errRaw, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/read sworn://baton/rules returned error: %s", errRaw)
	}
	var result resourcesReadResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal resources/read result: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("Contents length = %d, want 1", len(result.Contents))
	}
	got := result.Contents[0].Text
	if n := strings.Count(got, "# Rule"); n < 11 {
		t.Errorf("sworn://baton/rules content has %d '# Rule' headings, want >= 11", n)
	}
}

// TestResourcesListEnumeratesRegistered asserts resources/list reflects the
// actually-registered resources instead of a hardcoded empty array: every
// static URI must appear, and the dynamic sworn://release/ prefix pattern
// (not a readable resource itself) must not.
func TestResourcesListEnumeratesRegistered(t *testing.T) {
	w, r, s := testRoundTrip(t)
	RegisterResources(s, t.TempDir())

	sendRequest(t, w, "initialize", jsonID(1), json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`))
	readResponse(t, r)

	sendRequest(t, w, "resources/list", jsonID(2), nil)
	resp := readResponse(t, r)

	if errRaw, hasErr := resp["error"]; hasErr {
		t.Fatalf("resources/list returned error: %s", errRaw)
	}
	var result resourcesListResult
	if err := json.Unmarshal(resp["result"], &result); err != nil {
		t.Fatalf("unmarshal resources/list result: %v", err)
	}

	uris := make(map[string]bool, len(result.Resources))
	for _, res := range result.Resources {
		if res.URI == "" {
			t.Errorf("listed resource missing uri: %+v", res)
		}
		if res.Name == "" {
			t.Errorf("listed resource %q missing name", res.URI)
		}
		uris[res.URI] = true
	}

	want := []string{
		"sworn://prompts/plan",
		"sworn://prompts/implement",
		"sworn://prompts/verify",
		"sworn://baton/rules",
		"sworn://baton/track-mode",
		"sworn://baton/version",
	}
	for _, uri := range want {
		if !uris[uri] {
			t.Errorf("resources/list missing registered resource %q (got %v)", uri, uris)
		}
	}
	if uris["sworn://release/"] {
		t.Errorf("resources/list must not list the dynamic prefix pattern sworn://release/ as a readable resource")
	}
}
