package cockpit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestProjectHandoffKeepsParallelNodesAndUniqueResponsibilities(
	t *testing.T,
) {
	t.Parallel()

	snapshot := parallelSameRoleSnapshot()
	want := Handoff{
		Ready:            true,
		Nodes:            []string{"slice:S1", "slice:S2"},
		Responsibilities: []string{"implementer"},
	}
	if !reflect.DeepEqual(snapshot.Handoff, want) {
		t.Fatalf("parallel handoff = %#v, want %#v", snapshot.Handoff, want)
	}
}

func TestWebValidatorStaticContractAllowsParallelResponsibilities(
	t *testing.T,
) {
	t.Parallel()

	javascript := mustEmbeddedAsset(t, "web/app.js")
	_ = parallelSameRoleSnapshotJSON(t)
	if strings.Contains(
		javascript,
		"handoff.nodes.length !== handoff.responsibilities.length",
	) {
		t.Fatal("browser validator still assumes one responsibility per node")
	}
	for _, required := range []string{
		"const batonResponsibilities = [];",
		"new Set(handoff.responsibilities).size",
		"batonResponsibilities.length !== handoff.responsibilities.length",
		"responsibility !== handoff.responsibilities[index]",
	} {
		if !strings.Contains(javascript, required) {
			t.Fatalf("browser handoff validation is missing %q", required)
		}
	}
}

func TestBrowserJavaScriptValidatorAcceptsParallelSameRoleHandoffs(
	t *testing.T,
) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	javascript := mustEmbeddedAsset(t, "web/app.js")
	input, err := json.Marshal(struct {
		Source   string          `json:"source"`
		Snapshot json.RawMessage `json:"snapshot"`
	}{
		Source:   javascript,
		Snapshot: parallelSameRoleSnapshotJSON(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(node, "-e", webValidatorHarness)
	command.Stdin = bytes.NewReader(input)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("browser handoff regression failed: %v\n%s", err, output)
	}
}

func TestWebAttentionAnswerRequiresExactAdmittedAction(t *testing.T) {
	t.Parallel()

	javascript := mustEmbeddedAsset(t, "web/app.js")
	for _, required := range []string{
		"renderAttentions(snapshot.runtime.attentions, snapshot.actions);",
		"function admittedAnswerAction(attention, actions)",
		`action.kind === "answer_attention"`,
		"action.attention_id === attention.id",
		"action.expected_generation === attention.generation",
		"admittedAnswerAction(attention, state.snapshot.actions)",
	} {
		if !strings.Contains(javascript, required) {
			t.Fatalf("attention admission is missing %q", required)
		}
	}
	if strings.Contains(
		javascript,
		"renderAttentions(snapshot.runtime.attentions);",
	) {
		t.Fatal("attention rendering still ignores admitted snapshot actions")
	}
}

func TestWebAttentionAnswerAdmissionIsExactWithoutBrowser(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	command := exec.Command(node, "-e", webAttentionAdmissionHarness)
	command.Stdin = strings.NewReader(mustEmbeddedAsset(t, "web/app.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("attention admission regression failed: %v\n%s", err, output)
	}
}

func parallelSameRoleSnapshotJSON(t *testing.T) json.RawMessage {
	t.Helper()
	handler, projector, _ := newHTTPFixture(
		t,
		testLocalHost,
		testLocalOrigin,
	)
	projector.snapshot = parallelSameRoleSnapshot()
	request := httpRequest(
		http.MethodGet,
		testLocalOrigin+"/api/v1/runs/run-1/snapshot",
		"127.0.0.1:48000",
		nil,
	)
	response := serve(handler, request)
	if response.Code != http.StatusOK {
		t.Fatalf("parallel snapshot = %d: %s", response.Code, response.Body)
	}
	return append(json.RawMessage(nil), response.Body.Bytes()...)
}

func parallelSameRoleSnapshot() Snapshot {
	snapshot := httpSnapshot()
	snapshot.Graph = Graph{
		Nodes: []Node{
			{
				ID: "release:release-1", Kind: "release",
				Label: "release-1", State: "running",
			},
			{
				ID: "slice:S1", Kind: "slice", Label: "S1",
				Track: "T1", State: "ready",
				NextResponsibility: "implementer", HasBaton: true,
			},
			{
				ID: "slice:S2", Kind: "slice", Label: "S2",
				Track: "T2", State: "ready",
				NextResponsibility: "implementer", HasBaton: true,
			},
			{
				ID: "assembly:release-1", Kind: "assembly",
				Label: "Assembly", State: "waiting",
			},
		},
		Edges: []Edge{},
	}
	snapshot.Handoff = projectHandoff(snapshot.Graph)
	return snapshot
}

const webAttentionAdmissionHarness = `
const fs = require("fs");
const vm = require("vm");
const source = fs.readFileSync(0, "utf8");
const start = source.indexOf("function admittedAnswerAction(");
const end = source.indexOf("function renderAttentions(", start);
if (start < 0 || end < 0) {
  throw new Error("attention admission source not found");
}
const helper = source.slice(start, end);
const base = {
  id: "sha256:" + "a".repeat(64),
  state: "open",
  generation: 1,
};
const exact = {
  kind: "answer_attention",
  attention_id: base.id,
  expected_generation: 1,
};
const admitted = (attention, actions) => {
  const context = { attention, actions, result: null };
  vm.runInNewContext(
    '"use strict";' + helper +
      'result = Boolean(admittedAnswerAction(attention, actions));',
    context,
    { timeout: 1_000 },
  );
  return context.result;
};
const cases = [
  ["exact action", base, [exact], true],
  ["no action", base, [], false],
  ["wrong attention", base, [{
    ...exact,
    attention_id: "sha256:" + "b".repeat(64),
  }], false],
  ["stale generation", base, [{
    ...exact,
    expected_generation: 2,
  }], false],
  ["non-answer action", base, [{
    ...exact,
    kind: "pause",
  }], false],
  ["answered attention", {
    ...base,
    state: "answered",
  }, [exact], false],
];
const failures = cases.filter(
  ([, attention, actions, want]) =>
    admitted(attention, actions) !== want,
);
if (failures.length !== 0) {
  throw new Error(
    "unexpected attention admission: " +
      failures.map(([name]) => name).join(", "),
  );
}
`

const webValidatorHarness = `
const fs = require("fs");
const vm = require("vm");
const input = JSON.parse(fs.readFileSync(0, "utf8"));
const start = input.source.indexOf("function validSnapshot(");
const end = input.source.indexOf("async function refresh(", start);
if (start < 0 || end < 0) {
  throw new Error("browser validator source not found");
}
const validators = input.source.slice(start, end);
const clone = (value) => JSON.parse(JSON.stringify(value));
const validate = (snapshot) => {
  const context = { result: null, snapshot };
  vm.runInNewContext(
    '"use strict";' +
      'const SCHEMA = "sworn.cockpit/v2";' +
      'const state = { runID: "run-1" };' +
      validators +
      'result = validSnapshot(snapshot);',
    context,
    { timeout: 1_000 },
  );
  return context.result === true;
};
const cases = [];
const add = (name, mutate, want) => {
  const snapshot = clone(input.snapshot);
  mutate(snapshot);
  cases.push({ name, snapshot, want });
};
add("parallel same responsibility", () => {}, true);
add("parallel distinct responsibilities", (snapshot) => {
  snapshot.graph.nodes[2].next_responsibility = "verifier";
  snapshot.handoff.responsibilities = ["implementer", "verifier"];
}, true);
add("duplicate handoff node", (snapshot) => {
  snapshot.handoff.nodes = ["slice:S1", "slice:S1"];
}, false);
add("unknown handoff node", (snapshot) => {
  snapshot.handoff.nodes[1] = "slice:unknown";
}, false);
add("missing handoff node", (snapshot) => {
  snapshot.handoff.nodes = ["slice:S1"];
}, false);
add("reordered handoff nodes", (snapshot) => {
  snapshot.handoff.nodes.reverse();
}, false);
add("duplicate responsibility", (snapshot) => {
  snapshot.handoff.responsibilities = ["implementer", "implementer"];
}, false);
add("mismatched responsibility", (snapshot) => {
  snapshot.handoff.responsibilities = ["verifier"];
}, false);
add("missing responsibility", (snapshot) => {
  snapshot.handoff.responsibilities = [];
}, false);
add("reordered responsibilities", (snapshot) => {
  snapshot.graph.nodes[2].next_responsibility = "verifier";
  snapshot.handoff.responsibilities = ["verifier", "implementer"];
}, false);
add("duplicate graph node identity", (snapshot) => {
  snapshot.graph.nodes[2].id = "slice:S1";
}, false);
add("baton node without responsibility", (snapshot) => {
  delete snapshot.graph.nodes[1].next_responsibility;
}, false);
add("baton node with none responsibility", (snapshot) => {
  snapshot.graph.nodes[1].next_responsibility = "none";
}, false);
add("handoff node without baton", (snapshot) => {
  snapshot.graph.nodes[2].has_baton = false;
}, false);
add("ready mismatch", (snapshot) => {
  snapshot.handoff.ready = false;
}, false);
add("non-string responsibility", (snapshot) => {
  snapshot.handoff.responsibilities = [7];
}, false);

const failures = cases.filter(
  ({ snapshot, want }) => validate(snapshot) !== want,
);
if (failures.length !== 0) {
  throw new Error(
    "unexpected browser validation: " +
      failures.map(({ name }) => name).join(", "),
  );
}
`
