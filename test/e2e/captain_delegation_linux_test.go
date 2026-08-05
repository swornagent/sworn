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

	requestBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": "sworn_start_delegated",
			"arguments": map[string]any{
				"manifest_digest": envelope.ManifestDigest,
				"envelope_bytes":  envelopeBytes,
			},
		},
	})
	requestContext, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, "http://"+address+"/mcp", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Origin", "http://"+address)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || bytes.Contains(responseBody, []byte(`"isError":true`)) || !bytes.Contains(responseBody, []byte(`"state":"complete"`)) {
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
		t.Fatalf("delegated MCP start = %d %s\ncommands=%#v\neffects=%#v\nevents=%#v\nserve stderr=%s", response.StatusCode, responseBody, commands, effects, events, stderr.String())
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
	if counts["command:captain_decision"] != 2 || counts["command:planner_continuation"] != 1 || counts["command:approval"] != 1 ||
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
