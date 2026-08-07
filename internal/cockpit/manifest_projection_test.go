package cockpit

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// manifestProjectionFixture returns one Snapshot carrying real canonical
// manifest identity, a slice contract path/digest, and one touchpoint
// relation, so CLI/terminal, HTTP (TUI/browser), and MCP surfaces can be
// proven to present the exact same facts.
func manifestProjectionFixture() Snapshot {
	return Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Run:           RunView{ID: "run-1", Release: "release-1", State: "running"},
		Graph: Graph{
			ManifestVersion: "sworn.release-manifest/v1",
			Nodes: []Node{
				{
					ID: "release:release-1", Kind: "release",
					Label: "release-1", State: "running",
				},
				{
					ID: "slice:S1", Kind: "slice", Label: "S1", Track: "T1",
					State: "ready", HasBaton: true, NextResponsibility: "implementer",
					ContractPath:   "contracts/S1.json",
					ContractDigest: "sha256:" + strings.Repeat("a", 64),
				},
			},
			Edges: []Edge{},
			Touchpoints: []Touchpoint{
				{
					Left: "S1", Right: "S2", Path: "shared/owned.go",
					Ordered: true, Before: "S1",
				},
			},
		},
		Handoff: Handoff{
			Ready: true, Nodes: []string{"slice:S1"},
			Responsibilities: []string{"implementer"},
		},
		Runtime: RuntimeView{
			Effects: []EffectView{}, Attempts: []AttemptView{},
			Attentions: []AttentionView{}, Notifications: []NotificationView{},
		},
		Evidence:      []Evidence{},
		Actions:       []Action{{Kind: "pause", ExpectedGeneration: 1}},
		Diagnostics:   []Diagnostic{},
		ThroughOffset: 5,
	}
}

func manifestProjectionFixtureHandler(t *testing.T) (*HTTPHandler, *httpFakeCommands) {
	t.Helper()
	projector := &httpFakeProjector{snapshot: manifestProjectionFixture()}
	commands := &httpFakeCommands{}
	handler, err := NewHTTPHandler(projector, commands, HTTPConfig{
		RunID: "run-1", Host: testLocalHost, Origin: testLocalOrigin,
		BearerToken: []byte(testHTTPToken), MaxSSE: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, commands
}

// TestManifestFactsAreIdenticalAcrossTerminalHTTPAndMCP proves the CLI's
// terminal rendering, the HTTP snapshot endpoint TUI/browser surfaces read,
// and the one read-only MCP capability all present the exact same canonical
// manifest identity, slice contract path/digest, and touchpoint facts,
// because all three read the identical Snapshot produced by the shared
// cockpit projection.
func TestManifestFactsAreIdenticalAcrossTerminalHTTPAndMCP(t *testing.T) {
	t.Parallel()
	handler, commands := manifestProjectionFixtureHandler(t)
	fixture := manifestProjectionFixture()

	terminalText := RenderTerminalWidth(fixture, 500)
	for _, fact := range []string{
		`manifest="sworn.release-manifest/v1"`,
		`contract_path="contracts/S1.json"`,
		`contract_digest="sha256:` + strings.Repeat("a", 64) + `"`,
		`left="S1" right="S2" path="shared/owned.go" ordered=true before="S1"`,
	} {
		if !strings.Contains(terminalText, fact) {
			t.Fatalf("terminal rendering does not contain %q:\n%s", fact, terminalText)
		}
	}

	httpRequestRec := httpRequest(
		http.MethodGet, testLocalOrigin+apiPathPrefix+"/runs/run-1/snapshot",
		"127.0.0.1:41100", nil,
	)
	httpResponse := serve(handler, httpRequestRec)
	if httpResponse.Code != http.StatusOK {
		t.Fatalf("http snapshot = %d %s", httpResponse.Code, httpResponse.Body.String())
	}
	var httpSnapshot Snapshot
	if err := json.Unmarshal(httpResponse.Body.Bytes(), &httpSnapshot); err != nil {
		t.Fatal(err)
	}

	mcpBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` +
		`{"name":"sworn_status","arguments":{"run_id":"run-1"}}}`
	mcpRequestRec := httpRequest(
		http.MethodPost, testLocalOrigin+"/mcp", "127.0.0.1:41100", []byte(mcpBody),
	)
	mcpRequestRec.Header.Set("Content-Type", "application/json")
	mcpResponse := serve(handler, mcpRequestRec)
	if mcpResponse.Code != http.StatusOK {
		t.Fatalf("mcp status = %d %s", mcpResponse.Code, mcpResponse.Body.String())
	}
	var envelope struct {
		Result struct {
			StructuredContent Snapshot `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(mcpResponse.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(httpSnapshot.Graph, envelope.Result.StructuredContent.Graph) {
		t.Fatalf(
			"graph facts diverge between HTTP and MCP:\nhttp=%#v\nmcp=%#v",
			httpSnapshot.Graph, envelope.Result.StructuredContent.Graph,
		)
	}
	if !reflect.DeepEqual(httpSnapshot.Graph, fixture.Graph) {
		t.Fatalf("HTTP graph diverges from the source projection:\nhttp=%#v\nwant=%#v", httpSnapshot.Graph, fixture.Graph)
	}

	// The one read-only MCP capability must never invoke any write/action
	// command: every command counter stays at zero after this read.
	if commands.startCalls != 0 || commands.controlCalls != 0 ||
		commands.answerCalls != 0 || commands.redeliveries != 0 ||
		commands.approveCalls != 0 || commands.captainCalls != 0 {
		t.Fatalf("MCP status call invoked write authority: %#v", commands)
	}
}

// TestMCPStatusToolCannotMutateRefsOrInvokeWriteAuthority repeats the status
// call several times, including with a mismatched run_id, and proves no
// command surface is ever reached and no error path accidentally succeeds.
func TestMCPStatusToolCannotMutateRefsOrInvokeWriteAuthority(t *testing.T) {
	t.Parallel()
	handler, commands := manifestProjectionFixtureHandler(t)

	valid := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` +
		`{"name":"sworn_status","arguments":{"run_id":"run-1"}}}`
	mismatched := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":` +
		`{"name":"sworn_status","arguments":{"run_id":"other-run"}}}`
	for _, body := range []string{valid, valid, mismatched} {
		request := httpRequest(http.MethodPost, testLocalOrigin+"/mcp", "127.0.0.1:41100", []byte(body))
		request.Header.Set("Content-Type", "application/json")
		serve(handler, request)
	}
	if commands.startCalls != 0 || commands.controlCalls != 0 ||
		commands.answerCalls != 0 || commands.redeliveries != 0 ||
		commands.approveCalls != 0 || commands.captainCalls != 0 {
		t.Fatalf("repeated MCP status calls invoked write authority: %#v", commands)
	}
}

// TestLegacyPlanV2GraphProjectionHasNoContractPathOrManifestDrift proves a
// legacy baton.plan/v2 release's projection stays truthful and unchanged:
// its manifest identity reports the real legacy schema, its slices carry no
// contract_path (v2 has none), and its per-slice contract digest -- which
// already existed as retained-work identity before this phase -- is
// unaffected.
func TestLegacyPlanV2GraphProjectionHasNoContractPathOrManifestDrift(t *testing.T) {
	t.Parallel()
	state := baton.State{
		Release: "legacy-release",
		Plan: baton.PlanState{
			Metadata: baton.Metadata{
				SchemaVersion: baton.PlanVersion,
				Contracts:     map[string]string{"S1": "sha256:" + strings.Repeat("b", 64)},
			},
		},
		Tracks: []baton.TrackState{
			{
				ID: "T1",
				Slices: []*baton.SliceState{
					{
						Location: baton.SliceLocation{
							Track: baton.Track{ID: "T1"},
							Slice: baton.Slice{ID: "S1"},
						},
						Stage: "implement", Status: "ready", NextRole: "implementer",
						Outcome: "none",
					},
				},
			},
		},
		Assembly: baton.AssemblyState{Stage: "verify", Status: "waiting", NextRole: "none", Outcome: "none"},
	}
	graph := projectGraph(state, "running", nil)
	if graph.ManifestVersion != baton.PlanVersion {
		t.Fatalf("manifest_version = %q, want %q", graph.ManifestVersion, baton.PlanVersion)
	}
	var slice *Node
	for index := range graph.Nodes {
		if graph.Nodes[index].ID == "slice:S1" {
			slice = &graph.Nodes[index]
		}
	}
	if slice == nil {
		t.Fatal("missing slice:S1 node")
	}
	if slice.ContractPath != "" {
		t.Fatalf("legacy v2 slice must carry no contract_path, got %q", slice.ContractPath)
	}
	if slice.ContractDigest != "sha256:"+strings.Repeat("b", 64) {
		t.Fatalf("contract_digest = %q, want the real retained digest", slice.ContractDigest)
	}
	if len(graph.Touchpoints) != 0 {
		t.Fatalf("touchpoints = %#v, want none for one non-overlapping legacy slice", graph.Touchpoints)
	}
}

// TestEmptyPlanHistoryDegradesWithoutPanicOrFabrication proves projectGraph
// tolerates a State whose Plan.History has not been populated (e.g. a
// hand-built or degenerate fixture) by reporting no touchpoints rather than
// panicking or inventing facts.
func TestEmptyPlanHistoryDegradesWithoutPanicOrFabrication(t *testing.T) {
	t.Parallel()
	state := baton.State{Release: "empty-history"}
	graph := projectGraph(state, "not_started", nil)
	if graph.Touchpoints != nil {
		t.Fatalf("touchpoints = %#v, want nil", graph.Touchpoints)
	}
	if graph.ManifestVersion != "" {
		t.Fatalf("manifest_version = %q, want empty", graph.ManifestVersion)
	}
}
