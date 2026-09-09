//go:build linux

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// TestRealBinaryEconomyTurnBudgetParksGrantsAndResumes drives the compiled
// sworn binary against a real OpenAI-shaped HTTP provider (S4-resumable-
// budget-stops A1, A2, A3, A4): a manifest with a deliberately tiny
// max_turns_per_work forces the Implementer's first dispatch attempt to
// reach that budget with scoped code already written but never submitted,
// so the run parks with the file preserved uncommitted and no accepted
// candidate. The real compiled board/status surface is read back to find
// the grant action naming the exact exhausted work, unit and epoch (A4);
// the real `sworn grant` command line - never the internal Service.Control
// API and never a directly seeded journal crossing - applies an explicit
// bounded capacity grant (A2); and the run then continues on a fresh
// dispatch attempt to completion, merging the exact product (A5's assembly
// half). The journal's own per-attempt usage receipts are read back to
// confirm recorded spending across the crash-adjacent restart is additive,
// never reset (A3).
const (
	economyBudgetSecret  = "economy-budget-e2e-secret"
	economyBudgetContent = "economy budget granted content\n"
	// economyBudgetStallTurns is both the manifest's max_turns_per_work and
	// the exact turn the Implementer's first attempt writes scoped code on
	// without ever submitting, so the real driver's own turn-budget guard -
	// not a fixture shortcut - is what ends the attempt. It leaves generous
	// headroom over every other scripted responsibility's own real turn
	// count in this journey (including Verifier's occasional durable
	// evidence re-run), so only the Implementer's first attempt ever
	// crosses it.
	economyBudgetStallTurns = 10
	economyBudgetGrant      = 10
	// economyBudgetVerificationRerunCap bounds the Verifier's own
	// check_evidence_incomplete recovery loop (an occasional, disclosed
	// sandbox-start flake - the "sworn#251 class" - re-runs the exact named
	// check rather than treating a transient start failure as a product
	// defect), matching the production journey fixture's own bound, so a
	// persistent failure fails loud with a named cause instead of quietly
	// spinning into the turn budget.
	economyBudgetVerificationRerunCap = 3
)

func economyBudgetPlan(t *testing.T) ([]byte, baton.Plan) {
	t.Helper()
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       "economy-budget-release",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://economy-budget-release/1",
		Tracks: []baton.Track{{
			ID:        "T1",
			DependsOn: []string{},
			Slices: []baton.Slice{{
				ID:      "S1",
				Outcome: "Deliver the budget-grant fixture value.",
				Scope: baton.Scope{
					Include: []string{"one.txt"},
					Exclude: []string{},
				},
				Acceptance: []baton.Criterion{{
					ID:   "A-S1",
					Text: "The granted value is present in the exact product tree.",
				}},
				Checks:      []string{"check one.txt"},
				Constraints: []string{"deterministic local provider"},
				DependsOn:   []string{},
				Consumes:    []string{},
			}},
		}},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nDeterministic real-binary economy-budget E2E.\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func economyBudgetConfig(
	t *testing.T,
	providerURL string,
) ([]byte, driver.LoadedDriverConfig) {
	t.Helper()
	credential := "economy-budget-env"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key:       credential,
			Kind:      driver.CredentialEnvironment,
			Reference: "SWORN_ECONOMY_BUDGET_KEY",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key:              "economy-budget-openai",
					ID:               "sworn.e2e.economy-budget",
					Version:          "1.0.0",
					Endpoint:         providerURL + "/openai/v1/chat/completions",
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{credential},
					ResponseBytes:    driver.MaxProviderResponseBytes,
				},
				API: driver.OpenAIChatCompletionsAPI,
			},
		}},
		Profiles: []driver.DriverProfile{{
			Key:                 "economy-budget",
			Adapter:             "economy-budget-openai",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"economy-budget-model"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := driver.DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, loaded
}

func economyBudgetManifest(
	t *testing.T,
	runID, repository string,
	config driver.LoadedDriverConfig,
	maxTurnsPerWork int64,
) []byte {
	t.Helper()
	selection := driver.ModelSelection{
		Profile: "economy-budget",
		Model:   "economy-budget-model",
	}
	manifest := swornruntime.Manifest{
		GitIdentity:       gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
		SchemaVersion:     swornruntime.ManifestVersion,
		RunID:             runID,
		Repository:        repository,
		Release:           "economy-budget-release",
		TargetRef:         "refs/heads/main",
		Intent:            "Prove a real turn-budget park, an explicit grant and resumed completion.",
		MaxParallelTracks: 1,
		Authority: swornruntime.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		DriverConfigDigest: config.ConfigurationDigest(),
		Roles: driver.RoleSelections{
			Planner:     selection,
			Implementer: selection,
			Captain:     selection,
			Verifier:    selection,
		},
		Automation: &swornruntime.AutomationSelections{
			Recovery: selection,
		},
		Limits: driver.Limits{
			TimeoutMillis:   30_000,
			OutputBytes:     65_536,
			MaxTurnsPerWork: maxTurnsPerWork,
		},
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

// economyBudgetProvider is the scripted real-adapter model behind the HTTP
// endpoint the compiled binary actually dispatches to. Every responsibility
// but the Implementer submits on its first turn; the Implementer's first
// attempt (invocation epoch 1) deliberately never submits, consuming
// exactly the manifest's tiny turn budget so the driver's own economy guard
// - never a fixture shortcut - is what ends that attempt.
type economyBudgetProvider struct {
	t         *testing.T
	planBytes []byte

	mu                 sync.Mutex
	turns              map[string]int
	verificationReruns map[string]int
}

func (provider *economyBudgetProvider) nextTurn(invocationID string) int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.turns[invocationID]++
	return provider.turns[invocationID]
}

func (provider *economyBudgetProvider) serve(
	writer http.ResponseWriter,
	request *http.Request,
) {
	requestBody, err := io.ReadAll(io.LimitReader(
		request.Body, driver.MaxProviderRequestBytes+1,
	))
	if err != nil || len(requestBody) > driver.MaxProviderRequestBytes {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	if request.Header.Get("Authorization") != "Bearer "+economyBudgetSecret {
		http.Error(writer, "credential mismatch", http.StatusUnauthorized)
		return
	}
	promptBody, model, err := openAIJourneyPrompt(request, requestBody)
	if err != nil || model != "economy-budget-model" {
		provider.t.Errorf("economy budget provider request model=%q: %v", model, err)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	var prompt recoveryE2EModelPrompt
	if err := json.Unmarshal([]byte(promptBody), &prompt); err != nil ||
		prompt.InvocationID == "" || prompt.Responsibility == "" {
		provider.t.Errorf("economy budget prompt=%q error=%v", promptBody, err)
		http.Error(writer, "invalid prompt", http.StatusBadRequest)
		return
	}
	turn := provider.nextTurn(prompt.InvocationID)
	toolName, arguments, respErr := provider.respond(prompt, turn, requestBody)
	if respErr != nil {
		provider.t.Errorf("economy budget response: %v", respErr)
		http.Error(writer, "invalid response", http.StatusInternalServerError)
		return
	}
	argumentBody, err := json.Marshal(arguments)
	if err != nil {
		provider.t.Errorf("economy budget arguments: %v", err)
		http.Error(writer, "invalid response", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []any{map[string]any{
					"id":   journeyCallID(prompt.InvocationID, turn),
					"type": "function",
					"function": map[string]any{
						"name":      toolName,
						"arguments": string(argumentBody),
					},
				}},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{
			"prompt_tokens": 7, "completion_tokens": 5,
		},
	})
}

func (provider *economyBudgetProvider) respond(
	prompt recoveryE2EModelPrompt,
	turn int,
	body []byte,
) (string, map[string]any, error) {
	switch prompt.Responsibility {
	case driver.PlannerProposal:
		return provider.plannerResponse(prompt, turn)
	case driver.ImplementerImplementation:
		return provider.implementerResponse(prompt, turn)
	case driver.WorkVerification:
		return provider.verificationResponse(prompt, turn, body)
	default:
		if turn != 1 {
			return "", nil, fmt.Errorf(
				"unexpected %s turn=%d", prompt.Responsibility, turn,
			)
		}
		arguments, err := provider.submissionArguments(prompt)
		return "sworn_submit", arguments, err
	}
}

// verificationResponse runs the slice's one declared check and submits once
// its evidence is fresh. A check_evidence_incomplete refusal newer than the
// last check run re-runs that exact check (the disclosed "sworn#251 class"
// sandbox-start flake, not a product defect), bounded so a persistent
// failure fails loud instead of quietly spinning into the turn budget.
func (provider *economyBudgetProvider) verificationResponse(
	prompt recoveryE2EModelPrompt,
	turn int,
	body []byte,
) (string, map[string]any, error) {
	refusal := bytes.LastIndex(body, []byte("check_evidence_incomplete"))
	rerun := bytes.LastIndex(body, []byte("|| true"))
	if turn == 1 || refusal > rerun {
		provider.mu.Lock()
		if provider.verificationReruns == nil {
			provider.verificationReruns = map[string]int{}
		}
		if turn > 1 {
			provider.verificationReruns[prompt.InvocationID]++
		}
		reruns := provider.verificationReruns[prompt.InvocationID]
		provider.mu.Unlock()
		if reruns > economyBudgetVerificationRerunCap {
			return "", nil, fmt.Errorf(
				"verification check evidence refused %d times for %s",
				reruns, prompt.InvocationID,
			)
		}
		return "Bash", map[string]any{"script": "check one.txt || true"}, nil
	}
	arguments, err := provider.submissionArguments(prompt)
	return "sworn_submit", arguments, err
}

// plannerResponse crosses the production Planner's mandatory human-only
// summary boundary before any plan is admitted: turn 1 is a yielded
// confirmation question, and only the responsibility resumed from the
// answered turn emits plan bytes.
func (provider *economyBudgetProvider) plannerResponse(
	prompt recoveryE2EModelPrompt,
	turn int,
) (string, map[string]any, error) {
	if turn == 1 && prompt.Recovery == nil {
		return "sworn_yield", map[string]any{"yield": map[string]any{
			"schema_version": driver.YieldSchemaVersion,
			"invocation_id":  prompt.InvocationID,
			"kind":           string(driver.YieldHumanConfirmation),
			"message":        recoveryE2ESummaryQuestion,
		}}, nil
	}
	if prompt.Recovery == nil ||
		prompt.Recovery.Kind != driver.RecoverableInputAnswer ||
		prompt.Recovery.Content != recoveryE2ESummaryAnswer {
		return "", nil, fmt.Errorf(
			"planner resume turn=%d recovery=%#v", turn, prompt.Recovery,
		)
	}
	arguments, err := provider.submissionArguments(prompt)
	return "sworn_submit", arguments, err
}

// implementerResponse splits behavior by the invocation ID's own epoch
// segment (release/slice/responsibility/attempt/epoch/try): the first
// attempt (epoch 1) writes scoped code but is deliberately never allowed to
// submit within the manifest's tiny turn budget, so the driver's own
// ECONOMY_TURN_BUDGET_EXCEEDED guard - not a fixture shortcut - ends it
// (A1). Every later attempt (the fresh dispatch a grant admits) writes the
// real content and submits well inside its granted headroom.
func (provider *economyBudgetProvider) implementerResponse(
	prompt recoveryE2EModelPrompt,
	turn int,
) (string, map[string]any, error) {
	parts := strings.Split(prompt.InvocationID, "/")
	if len(parts) != 6 {
		return "", nil, fmt.Errorf("unexpected invocation id %q", prompt.InvocationID)
	}
	if parts[4] == "1" {
		switch {
		case turn < economyBudgetStallTurns:
			return "Bash", map[string]any{"script": "true"}, nil
		case turn == economyBudgetStallTurns:
			return "Write", map[string]any{
				"path":    "/workspace/one.txt",
				"content": "interim unsubmitted content, never accepted\n",
			}, nil
		default:
			return "", nil, fmt.Errorf(
				"first implementer attempt reached turn %d; the turn budget "+
					"should have stopped it at %d", turn, economyBudgetStallTurns,
			)
		}
	}
	switch turn {
	case 1:
		return "Write", map[string]any{
			"path":    "/workspace/one.txt",
			"content": economyBudgetContent,
		}, nil
	case 2:
		arguments, err := provider.submissionArguments(prompt)
		return "sworn_submit", arguments, err
	default:
		return "", nil, fmt.Errorf("granted implementer attempt reached turn %d", turn)
	}
}

func (provider *economyBudgetProvider) submissionArguments(
	prompt recoveryE2EModelPrompt,
) (map[string]any, error) {
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   prompt.InvocationID,
		Responsibility: prompt.Responsibility,
		Summary:        "Deterministic economy-budget fixture padded so every scripted responsibility this journey drives clears the submission content floor for its coverage.",
		Detail:         "Bound to the admitted production responsibility, padded so every scripted responsibility this journey drives clears the submission detail content floor for its coverage, well past the two-hundred-byte bound.\n",
	}
	var err error
	switch prompt.Responsibility {
	case driver.PlannerProposal:
		submission.Plan, err = driver.NewPlanBytes(provider.planBytes)
	case driver.ImplementerDesign:
	case driver.CaptainReview:
		submission.Decision, err = driver.NewDecision(driver.DecisionProceed)
	case driver.ImplementerImplementation:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("matched economy-budget implementation checks\n"),
		)
	case driver.WorkVerification:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("fresh economy-budget verification checks\n"),
		)
		if err == nil {
			submission.Decision, err = driver.NewDecision(driver.DecisionPass)
		}
	case driver.AssemblyVerification:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("fresh economy-budget assembly checks\n"),
		)
		if err == nil {
			submission.Decision, err = driver.NewDecision(driver.DecisionPass)
		}
	default:
		err = fmt.Errorf("unknown responsibility %q", prompt.Responsibility)
	}
	if err != nil {
		return nil, err
	}
	encoded, err := driver.EncodeSubmission(submission)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, err
	}
	return map[string]any{"submission": value}, nil
}

func TestRealBinaryEconomyTurnBudgetParksGrantsAndResumes(t *testing.T) {
	t.Parallel()
	repository := newProductRepository(t)
	planBytes, plan := economyBudgetPlan(t)
	provider := &economyBudgetProvider{t: t, planBytes: planBytes, turns: make(map[string]int)}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()

	root := t.TempDir()
	configBody, loaded := economyBudgetConfig(t, providerHTTP.URL)
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := writeManifest(
		t, root, economyBudgetManifest(
			t, "economy-budget", repository, loaded, economyBudgetStallTurns,
		),
	)
	journalPath := filepath.Join(root, "run.sqlite")
	swornBinary := filepath.Join(root, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")
	environment := map[string]string{"SWORN_ECONOMY_BUDGET_KEY": economyBudgetSecret}
	targetBefore := runGit(t, repository, "rev-parse", "main")

	stdout, stderr := runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run", "--manifest", manifestPath, "--journal", journalPath, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("initial run stdout=%q stderr=%q", stdout, stderr)
	}
	stdout = answerRecoveryPlannerSummary(
		t, swornBinary, "economy-budget", journalPath, configPath, environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("planner summary answer stdout=%q", stdout)
	}

	authorizePlan(t, journalPath, "economy-budget", plan)
	installApprovedPlan(t, repository, planBytes)

	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"resume", "--run", "economy-budget", "--journal", journalPath,
		"--command", "resume-1", "--generation", "0", "--config", configPath,
	)
	if stderr != "" {
		t.Fatalf("resume stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run", "--manifest", manifestPath, "--journal", journalPath, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf(
			"expected the implementer's first attempt to exhaust its real turn "+
				"budget and park: stdout=%q stderr=%q", stdout, stderr,
		)
	}

	statusBody, statusErr := runBinary(
		t, swornBinary, 0, "status", "--run", "economy-budget", "--journal", journalPath, "--json",
	)
	var status swornruntime.RunStatus
	if statusErr != "" || json.Unmarshal([]byte(statusBody), &status) != nil {
		t.Fatalf("status body=%q stderr=%q", statusBody, statusErr)
	}
	if status.State != "parked" || status.Park == nil ||
		status.Park.Cause != swornruntime.ParkCauseEconomyTurns ||
		status.Park.Spent != economyBudgetStallTurns ||
		status.Park.Budget != economyBudgetStallTurns ||
		status.Park.UnblockKnob != swornruntime.EconomyTurnsUnblockKnob {
		t.Fatalf("economy park status = %#v", status.Park)
	}
	if len(status.PinnedWork) != 1 ||
		status.PinnedWork[0].Cause != swornruntime.ParkCauseEconomyTurns ||
		status.PinnedWork[0].DispatchWorkID == "" {
		t.Fatalf("pinned work = %#v", status.PinnedWork)
	}
	if runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("parked run advanced target authority before any candidate was accepted")
	}

	// A4: the real compiled board surface, not a unit-level double, names
	// the exact exhausted work, its unit and its epoch.
	boardBody, boardErr := runBinary(
		t, swornBinary, 0, "board", "--run", "economy-budget", "--journal", journalPath, "--json",
	)
	var board cockpit.Snapshot
	if boardErr != "" || json.Unmarshal([]byte(boardBody), &board) != nil {
		t.Fatalf("board body=%q stderr=%q", boardBody, boardErr)
	}
	var grantAction *cockpit.Action
	for index := range board.Actions {
		if board.Actions[index].Kind == "grant" {
			grantAction = &board.Actions[index]
		}
	}
	if board.Run.State != "parked" || grantAction == nil ||
		grantAction.WorkID != status.PinnedWork[0].DispatchWorkID ||
		grantAction.Unit != swornruntime.ParkCauseEconomyTurns ||
		grantAction.ExpectedEpoch != 1 {
		t.Fatalf("board grant action = %#v pinned=%#v", grantAction, status.PinnedWork)
	}

	grantOut, grantErr := runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"grant", "--run", "economy-budget", "--journal", journalPath,
		"--command", "grant-1",
		"--generation", fmt.Sprintf("%d", grantAction.ExpectedGeneration),
		"--work", grantAction.WorkID,
		"--epoch", fmt.Sprintf("%d", grantAction.ExpectedEpoch),
		"--unit", grantAction.Unit,
		"--amount", fmt.Sprintf("%d", economyBudgetGrant), "--config", configPath,
	)
	if grantErr != "" || !strings.Contains(grantOut, "  state: running") {
		t.Fatalf("grant stdout=%q stderr=%q", grantOut, grantErr)
	}

	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run", "--manifest", manifestPath, "--journal", journalPath, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("post-grant run stdout=%q stderr=%q", stdout, stderr)
	}

	finalState := readBatonState(t, repository, "economy-budget-release")
	if finalState.Assembly.Outcome != "merged" ||
		runGit(t, repository, "rev-parse", "main") == targetBefore ||
		runGit(t, repository, "show", "main:one.txt") !=
			strings.TrimSuffix(economyBudgetContent, "\n") {
		t.Fatalf("final state=%#v", finalState.Assembly)
	}

	finalStatusBody, finalStatusErr := runBinary(
		t, swornBinary, 0, "status", "--run", "economy-budget", "--journal", journalPath, "--json",
	)
	var finalStatus swornruntime.RunStatus
	if finalStatusErr != "" || json.Unmarshal([]byte(finalStatusBody), &finalStatus) != nil {
		t.Fatalf("final status body=%q stderr=%q", finalStatusBody, finalStatusErr)
	}
	if finalStatus.Park != nil || len(finalStatus.PinnedWork) != 0 {
		t.Fatalf("final status still parked/pinned: %#v", finalStatus)
	}

	// A3: recorded spending survives the crash-adjacent park/grant/resume
	// restart rather than resetting - both attempts' engine-counted turns
	// are durably readable, additive across the retry.
	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	observation, err := store.ReadObservation(
		ctx, "economy-budget", journal.MaxObservationAttempts, journal.MaxObservationEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	var implementationTurns []int64
	for _, attempt := range observation.Attempts {
		if attempt.Responsibility != string(driver.ImplementerImplementation) {
			continue
		}
		var usage driver.UsageReceipt
		if err := json.Unmarshal(attempt.Usage, &usage); err != nil || usage.Turns == nil {
			t.Fatalf("implementation attempt usage=%s error=%v", attempt.Usage, err)
		}
		implementationTurns = append(implementationTurns, *usage.Turns)
	}
	if len(implementationTurns) != 2 ||
		implementationTurns[0] != economyBudgetStallTurns ||
		implementationTurns[1] != 2 {
		t.Fatalf(
			"implementation attempts' recorded turns = %v, want [%d 2] "+
				"(exhausted-then-granted, never reset)",
			implementationTurns, economyBudgetStallTurns,
		)
	}
}
