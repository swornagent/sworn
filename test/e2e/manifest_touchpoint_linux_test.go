//go:build linux

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// manifestTouchpointStrings converts a plain string slice into the []any
// shape encoding/json needs for a hand-built sworn.release-manifest/v1
// document.
func manifestTouchpointStrings(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

// manifestTouchpointContractRaw builds one real, self-consistent Sworn slice
// contract body: the same shape internal/baton's own manifest tests use, so
// its digest agrees with what a real sworn.release-manifest/v1 manifest can
// declare.
func manifestTouchpointContractRaw(t *testing.T, id string, touchpoints []string) []byte {
	t.Helper()
	value := map[string]any{
		"outcome": "Deliver " + id + ".",
		"scope": map[string]any{
			"include": manifestTouchpointStrings(touchpoints), "exclude": []any{},
		},
		"acceptance":  []any{map[string]any{"id": "A-" + id, "text": id + " is exact."}},
		"checks":      []any{"check " + id},
		"constraints": []any{"deterministic"},
		"depends_on":  []any{},
		"consumes":    []any{},
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// manifestTouchpointSliceEntry builds one compact sworn.release-manifest/v1
// slice entry: only the fields admission needs to build the dependency graph
// and touchpoint overlap, matching internal/baton's own manifest slice shape.
func manifestTouchpointSliceEntry(id, contractPath, digest string, touchpoints []string) map[string]any {
	return map[string]any{
		"id": id, "outcome": "Deliver " + id + ".",
		"contract_path": contractPath, "digest": digest,
		"depends_on": []any{}, "consumes": []any{},
		"touchpoints": manifestTouchpointStrings(touchpoints),
	}
}

// manifestTouchpointPlanBytes builds one real sworn.release-manifest/v1
// revision-1 plan admitting two single-slice tracks, T1/S1 and T2/S2, that
// both declare the same touchpoint path "shared/thing.go". When
// t2DependsOnT1 is false the tracks are independent and admission must fail
// closed with PARALLEL_TOUCH_CONFLICT; when true, T2's declared dependency on
// T1 orders the shared touchpoint and admission must succeed.
func manifestTouchpointPlanBytes(
	t *testing.T,
	release, targetRef string,
	t2DependsOnT1 bool,
	s1Digest, s2Digest string,
) []byte {
	t.Helper()
	t2Depends := []any{}
	if t2DependsOnT1 {
		t2Depends = []any{"T1"}
	}
	value := map[string]any{
		"schema_version": baton.ManifestVersion, "release": release, "revision": int64(1),
		"previous_plan": nil, "repository": "acme-repo", "target_ref": targetRef,
		"approval_ref": "operator://" + release + "/1",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{manifestTouchpointSliceEntry(
					"S1", "contracts/S1.json", s1Digest,
					[]string{"shared/thing.go", "one/only.txt"},
				)},
			},
			map[string]any{
				"id": "T2", "depends_on": t2Depends,
				"slices": []any{manifestTouchpointSliceEntry(
					"S2", "contracts/S2.json", s2Digest,
					[]string{"shared/thing.go", "two/only.txt"},
				)},
			},
		},
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte(
		"```sworn-release-manifest-v1\n" + string(body) +
			"\n```\n\nReal-binary touchpoint E2E plan.\n",
	)
}

// manifestTouchpointRunManifest builds the fake-driver run manifest for this
// scenario. Only PlannerProposal and AssemblyVerification are scripted:
// S1 and S2 are admitted directly through production action calls (the same
// prepared-tree/AppendReceipt pattern installAndPassComponent already uses
// in walking_skeleton_linux_test.go) before "resume" ever runs, so the real
// dispatcher never needs a scripted Implementer/Captain/Verifier response
// for either slice.
func manifestTouchpointRunManifest(
	t *testing.T,
	runID, repository, release string,
	planBytes []byte,
	fakeExecutable, fakeDigest string,
) []byte {
	t.Helper()
	var scripts []swornruntime.ScriptedAttempt
	add := func(responsibility driver.Responsibility, batonAttempt int64) {
		for try := int64(1); try <= 3; try++ {
			submission := driver.Submission{
				SchemaVersion:  driver.SubmissionSchemaVersion,
				InvocationID:   fmt.Sprintf("%s/release/%s/%d/1/%d", runID, responsibility, batonAttempt, try),
				Responsibility: responsibility,
				Summary:        "Exact " + string(responsibility) + ".",
				Detail:         "Fresh bounded E2E evidence.",
			}
			switch responsibility {
			case driver.PlannerProposal:
				submission.Plan, _ = driver.NewPlanBytes(planBytes)
			case driver.AssemblyVerification:
				submission.Checks, _ = driver.NewCheckBytes([]byte("fresh assembly checks\n"))
				submission.Decision, _ = driver.NewDecision(driver.DecisionPass)
			}
			scripts = append(scripts, swornruntime.ScriptedAttempt{
				Slice: "", Responsibility: responsibility, BatonAttempt: batonAttempt,
				Epoch: 1, Try: try, Behavior: "submit", Submission: encodedSubmission(t, submission),
			})
		}
	}
	add(driver.PlannerProposal, 1)
	add(driver.AssemblyVerification, 1)
	sort.Slice(scripts, func(i, j int) bool {
		left := fmt.Sprintf("%s/%s/%020d/%020d/%d", scripts[i].Responsibility,
			scripts[i].Slice, scripts[i].BatonAttempt, scripts[i].Epoch, scripts[i].Try)
		right := fmt.Sprintf("%s/%s/%020d/%020d/%d", scripts[j].Responsibility,
			scripts[j].Slice, scripts[j].BatonAttempt, scripts[j].Epoch, scripts[j].Try)
		return left < right
	})
	manifest := swornruntime.Manifest{
		GitIdentity:   gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
		SchemaVersion: swornruntime.ManifestVersion,
		RunID:         runID, Repository: repository, Release: release,
		TargetRef: "refs/heads/main", Intent: "Drive the exact approved touchpoint track.",
		MaxParallelTracks: 2,
		Authority: swornruntime.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		Driver: &swornruntime.FakeDriverConfig{
			Executable: fakeExecutable, Digest: fakeDigest,
			AdapterKey: "e2e-fake", Profile: "e2e-fake",
		},
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "e2e-fake", Model: "planner-model"},
			Implementer: driver.RoleSelection{Profile: "e2e-fake", Model: "implementer-model"},
			Captain:     driver.RoleSelection{Profile: "e2e-fake", Model: "captain-model"},
			Verifier:    driver.RoleSelection{Profile: "e2e-fake", Model: "verifier-model"},
		},
		Automation: &swornruntime.AutomationSelections{
			Recovery: driver.RoleSelection{Profile: "e2e-fake", Model: "recovery-model"},
		},
		Limits:  driver.Limits{TimeoutMillis: 30_000, OutputBytes: 65_536},
		Scripts: scripts,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := swornruntime.ParseManifest(body); err != nil {
		t.Fatal(err)
	}
	return body
}

// sealAndPassManifestSlice drives one slice from Implementer design through
// Verifier PASS entirely through direct production action calls and real Git
// workspace plumbing -- the exact pattern installAndPassComponent already
// uses in walking_skeleton_linux_test.go for a disjoint track -- writing its
// real candidate content to contentPath, which must stay inside the slice's
// declared touchpoints.
func sealAndPassManifestSlice(
	t *testing.T,
	actions *baton.Actions,
	workspaces *gitx.Workspaces,
	plan baton.Plan,
	gitRepository baton.GitRepository,
	release, track, slice, contentPath, content string,
) {
	t.Helper()
	for _, input := range []baton.AppendReceiptInput{
		{
			Release: release, Slice: slice, Role: "implementer", Result: "designed",
			Summary: "Design " + slice + ".", Detail: []byte(slice + " design."),
		},
		{
			Release: release, Slice: slice, Role: "captain", Result: "proceed",
			Summary: "Proceed with " + slice + ".", Detail: []byte(slice + " Captain proceed."),
		},
	} {
		if _, err := actions.AppendReceipt(input); err != nil {
			t.Fatal(err)
		}
	}
	key := gitx.TrackKey{Release: release, Track: track}
	workspace, err := workspaces.OpenTrack(key, gitx.ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(workspace.Path(), contentPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Path(), contentPath), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sealed, err := workspaces.SealTrack(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := baton.ValidateSliceCandidateScope(
		gitRepository, inertResolver, plan, slice,
		sealed.Before.String(), sealed.Candidate.String(),
	); err != nil {
		t.Fatal(err)
	}
	for _, input := range []baton.AppendReceiptInput{
		{
			Release: release, Slice: slice, Role: "implementer", Result: "candidate",
			Summary: "Seal " + slice + ".", Detail: []byte(slice + " implementation."),
			Candidate: sealed.Candidate.String(), CheckResults: []byte(slice + " checks\n"),
		},
		{
			Release: release, Slice: slice, Role: "verifier", Result: "pass",
			Summary: "Pass " + slice + ".", Detail: []byte("Fresh " + slice + " verification."),
			Candidate: sealed.Candidate.String(), CheckResults: []byte("fresh " + slice + " checks\n"),
		},
	} {
		if _, err := actions.AppendReceipt(input); err != nil {
			t.Fatal(err)
		}
	}
}

// fetchManifestTouchpointHTTPSnapshot reads the cockpit HTTP snapshot
// endpoint from a real running "sworn serve" process.
func fetchManifestTouchpointHTTPSnapshot(t *testing.T, address, runID string) cockpit.Snapshot {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 15 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for {
		response, err := client.Get("http://" + address + "/api/v2/runs/" + runID + "/snapshot")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		// A detached drive writing the journal makes the projection
		// transiently unstable; the honest 409 refusal is retried.
		if ((response.StatusCode == http.StatusConflict &&
			bytes.Contains(body, []byte("SNAPSHOT_UNSTABLE"))) ||
			(response.StatusCode == http.StatusServiceUnavailable &&
				bytes.Contains(body, []byte("RUNTIME_UNAVAILABLE")))) &&
			time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET snapshot = %d %s", response.StatusCode, body)
		}
		var snapshot cockpit.Snapshot
		if err := json.Unmarshal(body, &snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
}

// fetchManifestTouchpointMCPSnapshot calls the one read-only MCP
// sworn_status tool against a real running "sworn serve" process.
func fetchManifestTouchpointMCPSnapshot(t *testing.T, address, runID string) cockpit.Snapshot {
	t.Helper()
	requestBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "sworn_status",
			"arguments": map[string]any{"run_id": runID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 15 * time.Second}
	request, err := http.NewRequest(http.MethodPost, "http://"+address+"/mcp", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || bytes.Contains(body, []byte(`"isError":true`)) {
		t.Fatalf("POST /mcp sworn_status = %d %s", response.StatusCode, body)
	}
	var envelope struct {
		Result struct {
			StructuredContent cockpit.Snapshot `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Result.StructuredContent
}

// assertManifestTouchpointFacts checks that one cockpit Snapshot -- however
// it was obtained -- reports exactly the manifest identity, per-slice
// contract path/digest, absent evidence, and ordered shared-touchpoint
// relation that admission actually recorded.
func assertManifestTouchpointFacts(t *testing.T, snapshot cockpit.Snapshot, plan baton.Plan) {
	t.Helper()
	if snapshot.Graph.ManifestVersion != baton.ManifestVersion {
		t.Fatalf("manifest version = %q, want %q", snapshot.Graph.ManifestVersion, baton.ManifestVersion)
	}
	if snapshot.Run.PlanDigest != plan.Digest() {
		t.Fatalf("plan digest = %q, want %q", snapshot.Run.PlanDigest, plan.Digest())
	}
	nodes := make(map[string]cockpit.Node, len(snapshot.Graph.Nodes))
	for _, node := range snapshot.Graph.Nodes {
		nodes[node.ID] = node
	}
	s1, ok := nodes["slice:S1"]
	if !ok || s1.ContractPath != "contracts/S1.json" || s1.ContractDigest == "" ||
		len(s1.BoundEvidence) != 0 {
		t.Fatalf("S1 node = %#v", s1)
	}
	s2, ok := nodes["slice:S2"]
	if !ok || s2.ContractPath != "contracts/S2.json" || s2.ContractDigest == "" ||
		len(s2.BoundEvidence) != 0 {
		t.Fatalf("S2 node = %#v", s2)
	}
	if len(snapshot.Graph.Touchpoints) != 1 {
		t.Fatalf("touchpoints = %#v, want exactly one relation", snapshot.Graph.Touchpoints)
	}
	relation := snapshot.Graph.Touchpoints[0]
	if relation.Left != "S1" || relation.Right != "S2" || relation.Path != "shared/thing.go" ||
		!relation.Ordered || relation.Before != "S1" {
		t.Fatalf("touchpoint relation = %#v", relation)
	}
}

// TestRealBinaryManifestTouchpointOrderingGatesParallelConflictAndProjectsMatchingFacts
// is the S2-slice-artifacts real-built-binary E2E boundary required by the
// Verifier's bounded correction: it admits a real sworn.release-manifest/v1
// plan (contracts committed via the existing prepared-tree pattern) whose two
// tracks, T1/S1 and T2/S2, both declare the touchpoint "shared/thing.go".
// Left unordered, real admission must fail closed with
// PARALLEL_TOUCH_CONFLICT and leave no Git trace; with T2's explicit
// dependency on T1, the identical overlap is accepted and ordered. It then
// drives the real compiled sworn binary's board, serve (HTTP), and MCP
// status surfaces against that one real admitted state and proves all three
// report the identical touchpoint, contract, and evidence facts.
func TestRealBinaryManifestTouchpointOrderingGatesParallelConflictAndProjectsMatchingFacts(t *testing.T) {
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	repository := newProductRepository(t)

	// Commit both slice contracts to the target branch by an ordinary
	// commit -- the prepared-tree pattern TestRecordPlanRevisionManifestAtomicRecordAndReread
	// documents -- before any plan revision names them.
	s1ContractRaw := manifestTouchpointContractRaw(t, "S1", []string{"shared/thing.go", "one/only.txt"})
	s2ContractRaw := manifestTouchpointContractRaw(t, "S2", []string{"shared/thing.go", "two/only.txt"})
	_, s1Digest, err := baton.ParseSliceContract(s1ContractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	_, s2Digest, err := baton.ParseSliceContract(s2ContractRaw, "S2", "T2")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repository, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "contracts", "S1.json"), s1ContractRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "contracts", "S2.json"), s2ContractRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "--", "contracts/S1.json", "contracts/S2.json")
	runGit(t, repository, "commit", "--quiet", "-m", "commit slice contracts")
	contractTree := runGit(t, repository, "rev-parse", "main")

	const (
		runID   = "e2e-manifest-touchpoint"
		release = "e2e-manifest-touchpoint-release"
	)

	identity := gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"}
	openedRepository, err := gitx.Open(repository, e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	gitRepository := baton.UseGitRepository(openedRepository)
	actions, err := baton.NewActions(gitRepository, inertResolver, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Unordered: the same touchpoint overlap with no ordering dependency
	// must be rejected at real production admission with
	// PARALLEL_TOUCH_CONFLICT and must not move any Git ref.
	unorderedPlanBytes := manifestTouchpointPlanBytes(t, release, "refs/heads/main", false, s1Digest, s2Digest)
	beforeMain := runGit(t, repository, "rev-parse", "main")
	_, rejectErr := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: unorderedPlanBytes, ContractTree: contractTree,
		Summary: "Attempt unordered shared touchpoint.",
		Detail:  []byte("Independent tracks may not silently share a touchpoint."),
	})
	if baton.ErrorCode(rejectErr) != "PARALLEL_TOUCH_CONFLICT" {
		t.Fatalf("unordered admission code = %q, err = %v", baton.ErrorCode(rejectErr), rejectErr)
	}
	if after := runGit(t, repository, "rev-parse", "main"); after != beforeMain {
		t.Fatal("rejected admission moved the target ref")
	}
	if refCmd := exec.Command(
		e2eGit, "-C", repository, "show-ref", "--verify", "--quiet",
		"refs/heads/release-wt/"+release,
	); refCmd.Run() == nil {
		t.Fatal("rejected admission created a release ref")
	}

	// Ordered: T2's explicit dependency on T1 orders the identical
	// touchpoint overlap, so real admission must succeed.
	orderedPlanBytes := manifestTouchpointPlanBytes(t, release, "refs/heads/main", true, s1Digest, s2Digest)
	orderedPlan, err := baton.ParsePlan(orderedPlanBytes)
	if err != nil {
		t.Fatal(err)
	}

	manifestBody := manifestTouchpointRunManifest(
		t, runID, repository, release, orderedPlanBytes, fakeBinary, fakeDigest,
	)
	root := t.TempDir()
	manifestPath := writeManifest(t, root, manifestBody)
	journalPath := filepath.Join(root, "run.sqlite")

	stdout, _ := runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("planner output = %q", stdout)
	}

	authorizePlan(t, journalPath, runID, orderedPlan)

	result, err := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: orderedPlanBytes, ContractTree: contractTree,
		Summary: "Admit ordered shared touchpoint.",
		Detail:  []byte("T2's explicit dependency on T1 orders the shared touchpoint."),
	})
	if err != nil || !result.Changed {
		t.Fatalf("ordered admission failed: result=%#v err=%v", result, err)
	}

	workspaces, err := gitx.NewWorkspaces(openedRepository, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()

	// Both slices pass entirely through direct production action calls,
	// S1 fully before S2, exactly as installAndPassComponent already does
	// for one disjoint track -- generalized here to two, one of which is
	// dependency-ordered on the other.
	sealAndPassManifestSlice(
		t, actions, workspaces, orderedPlan, gitRepository,
		release, "T1", "S1", "one/only.txt", "s1 content\n",
	)
	sealAndPassManifestSlice(
		t, actions, workspaces, orderedPlan, gitRepository,
		release, "T2", "S2", "two/only.txt", "s2 content\n",
	)

	stdout, stderr := runBinary(
		t, swornBinary, 0, "resume", "--run", runID, "--journal", journalPath,
		"--command", "resume-1", "--generation", "0",
	)
	if stderr != "" {
		t.Fatalf("resume stderr = %q", stderr)
	}
	stdout, stderr = runBinary(
		t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("resume stdout = %q, stderr = %q", stdout, stderr)
	}

	boardBody, boardErr := runBinary(
		t, swornBinary, 0, "board", "--run", runID, "--journal", journalPath, "--json",
	)
	if boardErr != "" {
		t.Fatalf("board stderr = %q", boardErr)
	}
	var board cockpit.Snapshot
	if err := json.Unmarshal([]byte(boardBody), &board); err != nil {
		t.Fatal(err)
	}
	assertManifestTouchpointFacts(t, board, orderedPlan)

	address := telemetryParityAddress(t)
	operatorConfigPath := telemetryParityOperatorConfig(t, root, address, "")
	process := telemetryParityStartServe(t, swornBinary, runID, journalPath, manifestPath, operatorConfigPath)
	t.Cleanup(func() {
		if process.command.Process != nil {
			_ = process.command.Process.Signal(syscall.SIGTERM)
		}
	})
	telemetryParityWaitHealth(t, address, func(cockpit.TelemetryHealth) bool { return true })

	httpSnapshot := fetchManifestTouchpointHTTPSnapshot(t, address, runID)
	assertManifestTouchpointFacts(t, httpSnapshot, orderedPlan)

	mcpSnapshot := fetchManifestTouchpointMCPSnapshot(t, address, runID)
	assertManifestTouchpointFacts(t, mcpSnapshot, orderedPlan)

	if !reflect.DeepEqual(board.Graph, httpSnapshot.Graph) {
		t.Fatalf("board/HTTP graph facts diverge:\nboard=%#v\nhttp=%#v", board.Graph, httpSnapshot.Graph)
	}
	if !reflect.DeepEqual(board.Graph, mcpSnapshot.Graph) {
		t.Fatalf("board/MCP graph facts diverge:\nboard=%#v\nmcp=%#v", board.Graph, mcpSnapshot.Graph)
	}

	if exitCode := telemetryParityStopServe(t, process); exitCode != 0 {
		t.Fatalf("sworn serve exit = %d", exitCode)
	}
}
