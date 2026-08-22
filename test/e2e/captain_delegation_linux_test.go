//go:build linux

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

func TestRealBinaryDelegatedCaptainProceedInstallsRC14AndContinuesSerially(t *testing.T) {
	repository := newProductRepository(t)
	planBytes, plan := productionJourneyPlan(t, repository)
	revisedMetadata := plan.Metadata()
	revisedMetadata.Tracks[0].Slices[0].Outcome = "Deliver the exact externally admitted revised production slice A1."
	revisedMetadataBody, _ := json.MarshalIndent(revisedMetadata, "", "  ")
	revisedPlanBytes := []byte("```baton-plan-v2\n" + string(revisedMetadataBody) + "\n```\n\nDeterministic revised production journey.\n")
	revisedPlan, err := baton.ParsePlan(revisedPlanBytes)
	if err != nil {
		t.Fatal(err)
	}
	provider := &journeyProvider{
		t: t, planBytes: planBytes, replanBytes: revisedPlanBytes,
		captainPlanOutcomes: []driver.DecisionOutcome{driver.DecisionRevise, driver.DecisionProceed},
		turns:               make(map[string]int), families: make(map[string]driver.ProfileFamily),
		models: make(map[string]string), access: make(map[string]driver.WorkspaceAccess),
	}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()

	configBody, loaded := productionJourneyConfig(t, providerHTTP.URL)
	root := t.TempDir()
	driverConfigPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(driverConfigPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestBody := productionJourneyManifest(t, repository, loaded)
	manifestPath := writeManifest(t, root, manifestBody)
	journalPath := filepath.Join(root, "run.sqlite")
	address := telemetryParityAddress(t)
	operatorConfigPath := telemetryParityOperatorConfig(t, root, address, "")
	swornBinary := filepath.Join(root, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	manifestDigest := sha256.Sum256(manifestBody)
	targetHead := runGit(t, repository, "rev-parse", "main")
	_, shapeDigest, err := swornruntime.CaptainPlanStructuralProjection(plan)
	if err != nil {
		t.Fatal(err)
	}
	values, err := swornruntime.CaptainPlanValueProjection(plan)
	if err != nil {
		t.Fatal(err)
	}
	revisedValues, err := swornruntime.CaptainPlanValueProjection(revisedPlan)
	if err != nil {
		t.Fatal(err)
	}
	pointers := make([]string, 0, len(values))
	for pointer := range values {
		pointers = append(pointers, pointer)
	}
	sort.Strings(pointers)
	fieldRules := make([]swornruntime.CaptainFieldRule, 0, len(pointers))
	var delta swornruntime.CaptainDeltaOperation
	for _, pointer := range pointers {
		allowed := []string{values[pointer]}
		if revisedValues[pointer] != values[pointer] {
			allowed = append(allowed, revisedValues[pointer])
			sort.Strings(allowed)
			if delta.JSONPointer != "" {
				t.Fatal("revised fixture changed more than one admitted leaf")
			}
			delta = swornruntime.CaptainDeltaOperation{Operation: "replace", JSONPointer: pointer, FromDigest: values[pointer], ToDigest: revisedValues[pointer]}
		}
		fieldRules = append(fieldRules, swornruntime.CaptainFieldRule{
			JSONPointer: pointer, AllowedValueDigests: allowed,
		})
	}
	if delta.JSONPointer == "" {
		t.Fatal("revised fixture did not change an admitted leaf")
	}
	envelope := swornruntime.CaptainDelegation{
		SchemaVersion: swornruntime.CaptainDelegationVersion,
		RunID:         "production-journey", ManifestDigest: "sha256:" + hex.EncodeToString(manifestDigest[:]),
		Project: "acme-repo", Release: "production-journey-release",
		ReleaseRef:           "refs/heads/release-wt/production-journey-release",
		ReleaseLineageAnchor: swornruntime.CaptainLineageAnchor{State: "absent"},
		TargetRef:            "refs/heads/main", TargetHead: targetHead, DelegationEpoch: 1,
		DelegateRole: "captain", Responsibility: swornruntime.CaptainPlanReviewResponsibility,
		DecisionRules: []swornruntime.CaptainDecisionRule{
			{DecisionClass: swornruntime.PlannerProposalClass, AllowedOutcomes: []string{"escalate", "proceed", "revise"}},
			{DecisionClass: swornruntime.PlannerReplanClass, AllowedOutcomes: []string{"escalate", "proceed", "revise"}},
		},
		Limits: swornruntime.CaptainDelegationLimits{
			MinimumPlanRevision: 1, MaximumPlanRevision: 4,
			MaximumPlannerAttemptsPerRevision: 3, MaximumCaptainAttemptsPerProposal: 2,
			MaximumTotalCaptainDecisions: 8, ReplanBudget: 3,
		},
		PlanRules: swornruntime.CaptainPlanPolicy{
			SchemaVersion:  swornruntime.CaptainPlanPolicyVersion,
			AuthorityClass: "ordinary_delivery", InitialShapeDigest: shapeDigest,
			FieldRules: fieldRules,
			DeltaRules: swornruntime.CaptainDeltaRules{MaximumOperations: 1, AllowedOperations: []swornruntime.CaptainDeltaOperation{delta}},
		},
	}
	envelopeBytes, err := swornruntime.CanonicalCaptainDelegation(envelope)
	if err != nil {
		t.Fatal(err)
	}

	process := exec.Command(swornBinary, "serve", "--run", "production-journey", "--journal", journalPath,
		"--manifest", manifestPath, "--operator-config", operatorConfigPath, "--config", driverConfigPath)
	process.Env = cleanEnvironment(map[string]string{
		"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
		"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
	})
	var stdout, stderr bytes.Buffer
	process.Stdout, process.Stderr = &stdout, &stderr
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	defer func() {
		if process.Process != nil {
			_ = process.Process.Signal(syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				_ = process.Process.Kill()
			}
		}
	}()
	telemetryParityWaitHealth(t, address, func(cockpit.TelemetryHealth) bool { return true })

	// A9: the client is a coding-agent host that knows only what the server
	// advertises. It discovers the tool names, then drives the whole serial
	// loop through them, reconnecting with a fresh transport at every
	// boundary.
	advertised := journeyMCPTools(t, address)
	for _, required := range []string{
		"sworn_start_delegated", "sworn_status",
		"sworn_attentions", "sworn_answer_attention",
	} {
		if !advertised[required] {
			t.Fatalf("advertised MCP tools = %#v", advertised)
		}
	}
	diagnose := func(label string, body []byte) {
		var diagnostic journal.Snapshot
		if diagnosticStore, openErr := journal.OpenReadOnly(context.Background(), journalPath); openErr == nil {
			diagnostic, _ = diagnosticStore.Snapshot(context.Background(), "production-journey")
			_ = diagnosticStore.Close()
		}
		commands := make([]string, 0, len(diagnostic.Commands))
		for _, command := range diagnostic.Commands {
			commands = append(commands, command.Kind+":"+command.ReplayKey)
		}
		effects := make([]string, 0, len(diagnostic.Effects))
		for _, effect := range diagnostic.Effects {
			effects = append(effects, effect.Kind+":"+string(effect.State)+":"+effect.ErrorCode+":"+effect.ReplayKey)
		}
		events := make([]string, 0, len(diagnostic.Events))
		for _, event := range diagnostic.Events {
			events = append(events, event.Kind)
		}
		t.Fatalf("%s = %s\ncommands=%#v\neffects=%#v\nevents=%#v\nserve stderr=%s", label, body, commands, effects, events, stderr.String())
	}
	responseBody := journeyMCPCall(t, address, "sworn_start_delegated", map[string]any{
		"manifest_digest": envelope.ManifestDigest,
		"envelope_bytes":  envelopeBytes,
	})
	if bytes.Contains(responseBody, []byte(`"isError":true`)) ||
		!bytes.Contains(responseBody, []byte(`"state":"parked"`)) {
		diagnose("delegated MCP start", responseBody)
	}
	// Each summary turn is read and answered through a freshly connected
	// client. Answering the same turn twice must not duplicate any authority.
	// The answer returns once durable while the resident driver carries the
	// run to the next summary turn, so the second turn is awaited rather
	// than read from the answer's immediate output; an already-answered turn
	// still reading open means the drive has not resolved it yet.
	answered := map[string]bool{}
	answerDeadline := time.Now().Add(120 * time.Second)
	for len(answered) < 2 {
		if time.Now().After(answerDeadline) {
			diagnose(
				"delegated MCP human turns = "+strconv.Itoa(len(answered)),
				responseBody,
			)
		}
		attention, found := journeyMCPOpenHumanTurn(t, address)
		if !found || answered[attention] {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		answered[attention] = true
		responseBody = journeyMCPCall(t, address, "sworn_answer_attention", map[string]any{
			"run_id": "production-journey", "attention_id": attention,
			"expected_generation": 1, "answer": journeySummaryAnswer,
		})
		if bytes.Contains(responseBody, []byte(`"isError":true`)) {
			diagnose("delegated MCP answer", responseBody)
		}
		replayed := journeyMCPCall(t, address, "sworn_answer_attention", map[string]any{
			"run_id": "production-journey", "attention_id": attention,
			"expected_generation": 1, "answer": journeySummaryAnswer,
		})
		if bytes.Contains(replayed, []byte(`"isError":true`)) {
			diagnose("replayed MCP answer was not idempotent", replayed)
		}
	}
	completionDeadline := time.Now().Add(120 * time.Second)
	for {
		status := journeyMCPCall(t, address, "sworn_status", map[string]any{
			"run_id": "production-journey",
		})
		if bytes.Contains(status, []byte(`"isError":true`)) &&
			!bytes.Contains(status, []byte("SNAPSHOT_UNSTABLE")) &&
			!bytes.Contains(status, []byte("RUNTIME_UNAVAILABLE")) {
			diagnose("delegated MCP status", status)
		}
		if bytes.Contains(status, []byte(`"state":"complete"`)) {
			break
		}
		if time.Now().After(completionDeadline) {
			diagnose("delegated MCP completion deadline exceeded", status)
		}
		time.Sleep(200 * time.Millisecond)
	}

	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), "production-journey")
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, command := range snapshot.Commands {
		counts["command:"+command.Kind]++
	}
	for _, effect := range snapshot.Effects {
		if effect.State == journal.Succeeded {
			counts["effect:"+effect.Kind]++
		}
	}
	for _, event := range snapshot.Events {
		counts["event:"+event.Kind]++
	}
	if counts["command:attention.answer"] != 2 ||
		counts["command:captain_decision"] != 2 || counts["command:planner_continuation"] != 1 || counts["command:approval"] != 1 ||
		counts["effect:approval.admit"] != 1 || counts["effect:baton.install"] != 1 ||
		counts["effect:planner.continue"] != 1 || counts["event:captain_plan_decided"] != 2 {
		t.Fatalf("delegated cardinalities = %#v", counts)
	}
	captainInvocations := map[string]bool{}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" || effect.State != journal.Succeeded {
			continue
		}
		submission, decodeErr := driver.DecodeSubmission(effect.Result)
		if decodeErr == nil && submission.Responsibility == driver.CaptainPlanReview {
			captainInvocations[submission.InvocationID] = true
		}
	}
	if len(captainInvocations) != 2 {
		t.Fatalf("replacement Captain invocations = %#v", captainInvocations)
	}
	state := readBatonState(t, repository, "production-journey-release")
	if state.Plan.Digest != revisedPlan.Digest() || state.Plan.Approval.Receipt.Role != "planner" ||
		state.Plan.Approval.Receipt.Result != "approved" || state.Assembly.Outcome != "merged" {
		t.Fatalf("installed RC14 state = %#v", state.Plan)
	}
	if stdout.String() != "sworn serve: ready\n" || stderr.Len() != 0 {
		t.Fatalf("serve output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

// journeyMCPPost issues one JSON-RPC request over a freshly built transport,
// so every call in a journey behaves like a reconnecting client.
func journeyMCPPost(t *testing.T, address string, body []byte) []byte {
	t.Helper()
	requestContext, cancel := context.WithTimeout(
		context.Background(), 300*time.Second,
	)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		"http://"+address+"/mcp",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", "http://"+address)
	client := &http.Client{Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST /mcp = %d %s", response.StatusCode, responseBody)
	}
	return responseBody
}

func journeyMCPTools(t *testing.T, address string) map[string]bool {
	t.Helper()
	body := journeyMCPPost(t, address, []byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
	))
	var listing struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if json.Unmarshal(body, &listing) != nil {
		t.Fatalf("tools/list = %s", body)
	}
	advertised := make(map[string]bool, len(listing.Result.Tools))
	for _, tool := range listing.Result.Tools {
		advertised[tool.Name] = true
	}
	return advertised
}

func journeyMCPCall(
	t *testing.T,
	address, name string,
	arguments map[string]any,
) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": arguments},
	})
	if err != nil {
		t.Fatal(err)
	}
	return journeyMCPPost(t, address, body)
}

// journeyMCPOpenHumanTurn reads the one open human-only turn through the
// advertised read tool and returns its identity.
func journeyMCPOpenHumanTurn(t *testing.T, address string) (string, bool) {
	t.Helper()
	body := journeyMCPCall(t, address, "sworn_attentions", map[string]any{})
	var envelope struct {
		Result struct {
			StructuredContent struct {
				Attentions []cockpit.AttentionView `json:"attentions"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		t.Fatalf("sworn_attentions = %s", body)
	}
	var open []cockpit.AttentionView
	for _, attention := range envelope.Result.StructuredContent.Attentions {
		if attention.State != "open" || attention.HumanTurn == nil {
			continue
		}
		open = append(open, attention)
	}
	if len(open) == 0 {
		return "", false
	}
	if len(open) != 1 ||
		open[0].Question != journeySummaryQuestion ||
		open[0].HumanTurn.Responsibility != string(driver.PlannerProposal) ||
		open[0].HumanTurn.Kind != string(driver.YieldHumanConfirmation) {
		t.Fatalf("open human turns = %#v", open)
	}
	return open[0].ID, true
}
