//go:build linux

package e2e

import (
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
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/observe"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

const (
	recoveryE2ESecret  = "turn-recovery-e2e-secret"
	recoveryE2EAnswer  = "Use the exact approved recovery fixture value."
	recoveryE2EContent = "matched production outcome\n"
)

type recoveryE2EProvider struct {
	t         *testing.T
	planBytes []byte
	recover   bool

	mu    sync.Mutex
	turns map[string]int
}

type recoveryE2EModelPrompt struct {
	SchemaVersion  string                `json:"schema_version"`
	InvocationID   string                `json:"invocation_id"`
	Responsibility driver.Responsibility `json:"responsibility"`
	Recovery       *struct {
		Kind    driver.RecoverableInputKind `json:"kind"`
		Content string                      `json:"content"`
	} `json:"recovery,omitempty"`
}

type recoveryE2EAutomationPrompt struct {
	SchemaVersion string `json:"schema_version"`
	Operation     string `json:"operation"`
	Request       struct {
		InvocationID string `json:"invocation_id"`
	} `json:"request"`
}

func (provider *recoveryE2EProvider) serve(
	writer http.ResponseWriter,
	request *http.Request,
) {
	body, err := io.ReadAll(io.LimitReader(
		request.Body,
		driver.MaxProviderRequestBytes+1,
	))
	if err != nil || len(body) > driver.MaxProviderRequestBytes {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	defer func() {
		for index := range body {
			body[index] = 0
		}
	}()
	if request.Header.Get("Authorization") !=
		"Bearer "+recoveryE2ESecret {
		http.Error(writer, "credential mismatch", http.StatusUnauthorized)
		return
	}
	promptBody, model, err := openAIJourneyPrompt(request, body)
	if err != nil || model != "turn-recovery-model" {
		provider.t.Errorf(
			"turn recovery provider request model=%q: %v",
			model,
			err,
		)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	var header struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal([]byte(promptBody), &header); err != nil {
		provider.t.Errorf("turn recovery prompt: %v", err)
		http.Error(writer, "invalid prompt", http.StatusBadRequest)
		return
	}

	var invocationID, toolName string
	var arguments map[string]any
	switch header.SchemaVersion {
	case "sworn.model-prompt/v1":
		var prompt recoveryE2EModelPrompt
		if err := json.Unmarshal([]byte(promptBody), &prompt); err != nil ||
			prompt.InvocationID == "" || prompt.Responsibility == "" {
			provider.t.Errorf(
				"turn recovery model prompt=%q error=%v",
				promptBody,
				err,
			)
			http.Error(writer, "invalid prompt", http.StatusBadRequest)
			return
		}
		invocationID = prompt.InvocationID
		turn := provider.nextTurn(invocationID)
		toolName, arguments, err = provider.workerResponse(prompt, turn)
	case "sworn.automation-prompt/v1":
		var prompt recoveryE2EAutomationPrompt
		if err := json.Unmarshal([]byte(promptBody), &prompt); err != nil ||
			prompt.Operation != "recovery" ||
			prompt.Request.InvocationID == "" {
			provider.t.Errorf(
				"turn recovery automation prompt=%q error=%v",
				promptBody,
				err,
			)
			http.Error(writer, "invalid prompt", http.StatusBadRequest)
			return
		}
		invocationID = prompt.Request.InvocationID
		if turn := provider.nextTurn(invocationID); turn != 1 {
			err = fmt.Errorf("automation turn=%d", turn)
			break
		}
		toolName = "sworn_recovery_decide"
		arguments = map[string]any{"decision": map[string]any{
			"schema_version": driver.RecoveryDecisionSchemaVersion,
			"invocation_id":  invocationID,
			"action":         driver.RecoveryPauseForHuman,
		}}
	default:
		err = fmt.Errorf("unexpected prompt schema %q", header.SchemaVersion)
	}
	if err != nil {
		provider.t.Errorf("turn recovery response: %v", err)
		http.Error(writer, "invalid response", http.StatusInternalServerError)
		return
	}
	argumentBody, err := json.Marshal(arguments)
	if err != nil {
		provider.t.Errorf("turn recovery arguments: %v", err)
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
					"id":   journeyCallID(invocationID, provider.turn(invocationID)),
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

func (provider *recoveryE2EProvider) nextTurn(invocationID string) int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.turns[invocationID]++
	return provider.turns[invocationID]
}

func (provider *recoveryE2EProvider) turn(invocationID string) int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.turns[invocationID]
}

func (provider *recoveryE2EProvider) workerResponse(
	prompt recoveryE2EModelPrompt,
	turn int,
) (string, map[string]any, error) {
	if prompt.Responsibility != driver.ImplementerImplementation {
		if turn != 1 || prompt.Recovery != nil {
			return "", nil, fmt.Errorf(
				"unexpected %s turn=%d recovery=%t",
				prompt.Responsibility,
				turn,
				prompt.Recovery != nil,
			)
		}
		arguments, err := provider.submissionArguments(prompt)
		return "sworn_submit", arguments, err
	}
	if !provider.recover {
		switch {
		case turn == 1 && prompt.Recovery == nil:
			return "Write", map[string]any{
				"path":    "/workspace/one.txt",
				"content": recoveryE2EContent,
			}, nil
		case turn == 2 && prompt.Recovery == nil:
			arguments, err := provider.submissionArguments(prompt)
			return "sworn_submit", arguments, err
		default:
			return "", nil, fmt.Errorf(
				"direct implementation turn=%d recovery=%t",
				turn,
				prompt.Recovery != nil,
			)
		}
	}
	switch {
	case turn == 1 && prompt.Recovery == nil:
		return "sworn_yield", map[string]any{"yield": map[string]any{
			"schema_version": driver.YieldSchemaVersion,
			"invocation_id":  prompt.InvocationID,
			"kind":           driver.YieldQuestion,
			"message":        "Which exact approved value should I use?",
		}}, nil
	case prompt.Recovery == nil ||
		prompt.Recovery.Kind != driver.RecoverableInputAnswer ||
		prompt.Recovery.Content != recoveryE2EAnswer:
		return "", nil, fmt.Errorf(
			"invalid implementation recovery turn=%d recovery=%#v",
			turn,
			prompt.Recovery,
		)
	case turn == 2:
		return "Write", map[string]any{
			"path":    "/workspace/one.txt",
			"content": recoveryE2EContent,
		}, nil
	case turn == 3:
		arguments, err := provider.submissionArguments(prompt)
		return "sworn_submit", arguments, err
	default:
		return "", nil, fmt.Errorf("implementation turn=%d", turn)
	}
}

func (provider *recoveryE2EProvider) submissionArguments(
	prompt recoveryE2EModelPrompt,
) (map[string]any, error) {
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   prompt.InvocationID,
		Responsibility: prompt.Responsibility,
		Summary:        "Deterministic turn recovery fixture.",
		Detail:         "Bound to the admitted production responsibility.",
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
			[]byte("matched implementation checks\n"),
		)
	case driver.WorkVerification:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("fresh recovery verification checks\n"),
		)
		if err == nil {
			submission.Decision, err = driver.NewDecision(
				driver.DecisionPass,
			)
		}
	case driver.AssemblyVerification:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("fresh recovery assembly checks\n"),
		)
		if err == nil {
			submission.Decision, err = driver.NewDecision(
				driver.DecisionPass,
			)
		}
	default:
		err = fmt.Errorf("unknown responsibility %q", prompt.Responsibility)
	}
	if err != nil {
		return nil, err
	}
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, err
	}
	return map[string]any{"submission": value}, nil
}

func recoveryE2EPlan(t *testing.T) ([]byte, baton.Plan) {
	t.Helper()
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       "turn-recovery-release",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://turn-recovery-release/1",
		Tracks: []baton.Track{{
			ID:        "T1",
			DependsOn: []string{},
			Slices: []baton.Slice{{
				ID:      "S1",
				Outcome: "Deliver the matched production fixture.",
				Scope: baton.Scope{
					Include: []string{"one.txt"},
					Exclude: []string{},
				},
				Acceptance: []baton.Criterion{{
					ID:   "A-S1",
					Text: "The matched value is present in the exact product tree.",
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
			"\n```\n\nDeterministic paired turn-recovery E2E.\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func recoveryE2EConfig(
	t *testing.T,
	providerURL string,
) ([]byte, driver.LoadedDriverConfig) {
	t.Helper()
	credential := "turn-recovery-env"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key:       credential,
			Kind:      driver.CredentialEnvironment,
			Reference: "SWORN_TURN_RECOVERY_KEY",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key:              "turn-recovery-openai",
					ID:               "sworn.e2e.turn-recovery",
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
			Key:                 "turn-recovery",
			Adapter:             "turn-recovery-openai",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"turn-recovery-model"},
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

func recoveryE2EManifest(
	t *testing.T,
	runID string,
	repository string,
	config driver.LoadedDriverConfig,
) []byte {
	t.Helper()
	selection := driver.ModelSelection{
		Profile: "turn-recovery",
		Model:   "turn-recovery-model",
	}
	manifest := swornruntime.Manifest{
		SchemaVersion:     swornruntime.ManifestVersion,
		RunID:             runID,
		Repository:        repository,
		Release:           "turn-recovery-release",
		TargetRef:         "refs/heads/main",
		Intent:            "Prove exact production recovery across a process restart.",
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
			TimeoutMillis: 30_000,
			OutputBytes:   65_536,
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

type recoveryE2EPairedResult struct {
	record         observe.Record
	implementation observe.AttemptGroup
	productTree    string
	productContent string
}

func runDirectTurnRecoveryBaseline(
	t *testing.T,
	approvals *approvalServer,
	swornBinary string,
	planBytes []byte,
	plan baton.Plan,
) recoveryE2EPairedResult {
	t.Helper()

	approvals.mu.Lock()
	delete(approvals.comments, int64(64))
	approvals.mu.Unlock()

	repository := newProductRepository(t)
	provider := &recoveryE2EProvider{
		t: t, planBytes: planBytes, turns: make(map[string]int),
	}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()

	root := t.TempDir()
	configBody, loaded := recoveryE2EConfig(t, providerHTTP.URL)
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := writeManifest(
		t,
		root,
		recoveryE2EManifest(
			t,
			"turn-recovery-direct",
			repository,
			loaded,
		),
	)
	journalPath := filepath.Join(root, "run.sqlite")
	environment := map[string]string{
		"SWORN_TURN_RECOVERY_KEY": recoveryE2ESecret,
	}
	targetBefore := runGit(t, repository, "rev-parse", "main")
	stdout, stderr := runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("direct start stdout=%q stderr=%q", stdout, stderr)
	}
	approvals.publish(64, approvalFor(64, "turn-recovery-v1", plan))
	authorizePlan(t, journalPath, "turn-recovery", plan)
	installApprovedPlan(t, repository, planBytes)
	stdout, stderr = runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"resume",
		"--run", "turn-recovery-direct",
		"--journal", journalPath,
		"--command", "direct-resume-1",
		"--generation", "0",
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("direct resume stdout=%q stderr=%q", stdout, stderr)
	}
	finalState := readBatonState(
		t,
		repository,
		"turn-recovery-release",
	)
	productContent := runGit(t, repository, "show", "main:one.txt")
	if finalState.Assembly.Outcome != "merged" ||
		runGit(t, repository, "rev-parse", "main") == targetBefore ||
		productContent != strings.TrimSuffix(recoveryE2EContent, "\n") {
		t.Fatalf("final direct state=%#v content=%q",
			finalState.Assembly, productContent)
	}

	ctx := context.Background()
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	statusReader, err := swornruntime.OpenStatusService(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer statusReader.Close()
	stateReader, err := cockpit.NewGitStateReader(e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := cockpit.NewProjector(
		store,
		statusReader,
		stateReader,
	)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := observe.NewEvaluator(
		store,
		projector,
		"1.0.0-rc.2",
	)
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(ctx, "turn-recovery-direct")
	if err != nil || !changed {
		t.Fatalf("direct eval changed=%t error=%v record=%#v",
			changed, err, record)
	}
	var implementationGroups []observe.AttemptGroup
	for _, group := range record.Groups {
		if group.Responsibility ==
			string(driver.ImplementerImplementation) {
			implementationGroups = append(implementationGroups, group)
		}
	}
	if record.SchemaVersion != journal.EvalSchemaVersionV2 ||
		record.TurnRecovery.Recovered != 0 ||
		record.TurnRecovery.HumanEscalations != 0 ||
		record.TurnRecovery.FalseAcceptances != 0 ||
		len(implementationGroups) != 1 ||
		implementationGroups[0].Attempts != 1 ||
		implementationGroups[0].Usage.InputTokens == nil ||
		*implementationGroups[0].Usage.InputTokens != 14 ||
		implementationGroups[0].Usage.OutputTokens == nil ||
		*implementationGroups[0].Usage.OutputTokens != 10 {
		t.Fatalf(
			"direct eval=%#v implementation=%#v",
			record.TurnRecovery,
			implementationGroups,
		)
	}
	return recoveryE2EPairedResult{
		record:         record,
		implementation: implementationGroups[0],
		productTree:    runGit(t, repository, "rev-parse", "main^{tree}"),
		productContent: productContent,
	}
}

func TestProductionTurnRecoveryParksRestartsAndAccountsExactlyOnce(
	t *testing.T,
) {
	approvals := &approvalServer{
		comments: make(map[int64][]approvalComment),
	}
	approvalHTTP := httptest.NewServer(http.HandlerFunc(approvals.serve))
	defer approvalHTTP.Close()

	repository := newProductRepository(t)
	planBytes, plan := recoveryE2EPlan(t)
	provider := &recoveryE2EProvider{
		t: t, planBytes: planBytes, recover: true,
		turns: make(map[string]int),
	}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()

	root := t.TempDir()
	configBody, loaded := recoveryE2EConfig(t, providerHTTP.URL)
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := writeManifest(
		t,
		root,
		recoveryE2EManifest(t, "turn-recovery", repository, loaded),
	)
	journalPath := filepath.Join(root, "run.sqlite")
	swornBinary := filepath.Join(root, "sworn")
	buildBinary(
		t,
		swornBinary,
		"./cmd/sworn",
		"-X=github.com/swornagent/sworn/internal/runtime.githubAPIBase="+
			approvalHTTP.URL,
	)
	environment := map[string]string{
		"SWORN_TURN_RECOVERY_KEY": recoveryE2ESecret,
	}
	targetBefore := runGit(t, repository, "rev-parse", "main")
	stdout, stderr := runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("recovery start stdout=%q stderr=%q", stdout, stderr)
	}

	approvals.publish(64, approvalFor(64, "turn-recovery-v1", plan))
	installApprovedPlan(t, repository, planBytes)
	stdout, stderr = runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"resume",
		"--run", "turn-recovery",
		"--journal", journalPath,
		"--command", "resume-1",
		"--generation", "0",
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("recovery park stdout=%q stderr=%q", stdout, stderr)
	}

	statusBody, statusErr := runBinary(
		t,
		swornBinary,
		0,
		"status",
		"--run", "turn-recovery",
		"--journal", journalPath,
		"--json",
	)
	var status swornruntime.RunStatus
	if statusErr != "" || json.Unmarshal([]byte(statusBody), &status) != nil ||
		status.State != "parked" {
		t.Fatalf(
			"restarted status body=%q stderr=%q parsed=%#v",
			statusBody,
			statusErr,
			status,
		)
	}
	boardBody, boardErr := runBinary(
		t,
		swornBinary,
		0,
		"board",
		"--run", "turn-recovery",
		"--journal", journalPath,
		"--json",
	)
	var board cockpit.Snapshot
	if boardErr != "" || json.Unmarshal([]byte(boardBody), &board) != nil {
		t.Fatalf("restarted board body=%q stderr=%q", boardBody, boardErr)
	}
	var answerActions int
	for _, action := range board.Actions {
		if action.Kind == "answer_attention" {
			answerActions++
			if len(board.Runtime.Attentions) != 1 ||
				action.AttentionID != board.Runtime.Attentions[0].ID ||
				action.ExpectedGeneration != 1 {
				t.Fatalf("answer action=%#v attentions=%#v", action,
					board.Runtime.Attentions)
			}
		}
	}
	if board.Run.State != "parked" ||
		len(board.Runtime.Attentions) != 1 ||
		board.Runtime.Attentions[0].State != "open" ||
		board.Runtime.Attentions[0].Generation != 1 ||
		answerActions != 1 {
		t.Fatalf("parked board=%#v", board)
	}
	for _, attempt := range board.Runtime.Attempts {
		if attempt.Responsibility ==
			string(driver.ImplementerImplementation) {
			t.Fatalf("yield created a final implementation attempt: %#v", attempt)
		}
	}

	parkedState := readBatonState(
		t,
		repository,
		"turn-recovery-release",
	)
	parkedSlice, found := parkedState.Slice("S1")
	if !found || parkedSlice.Stage != "implement" ||
		parkedSlice.NextRole != "implementer" ||
		parkedSlice.Candidate != nil || parkedSlice.Pass != nil ||
		parkedSlice.CurrentReceipt == nil ||
		parkedSlice.CurrentReceipt.Receipt.Role != "captain" ||
		parkedSlice.CurrentReceipt.Receipt.Result != "proceed" ||
		runGit(t, repository, "rev-parse", "main") != targetBefore ||
		runGit(
			t,
			repository,
			"ls-tree",
			"--name-only",
			"refs/heads/track/turn-recovery-release/T1",
			"--",
			"one.txt",
		) != "" {
		t.Fatalf(
			"yield advanced authority slice=%#v target=%s",
			parkedSlice,
			runGit(t, repository, "rev-parse", "main"),
		)
	}

	time.Sleep(10 * time.Millisecond)
	attentionID := board.Runtime.Attentions[0].ID
	stdout, stderr = runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"answer",
		"--run", "turn-recovery",
		"--journal", journalPath,
		"--attention", attentionID,
		"--generation", "1",
		"--answer", recoveryE2EAnswer,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("recovery answer stdout=%q stderr=%q", stdout, stderr)
	}
	finalState := readBatonState(
		t,
		repository,
		"turn-recovery-release",
	)
	if finalState.Assembly.Outcome != "merged" ||
		runGit(t, repository, "rev-parse", "main") ==
			targetBefore ||
		runGit(t, repository, "show", "main:one.txt") !=
			strings.TrimSuffix(recoveryE2EContent, "\n") {
		t.Fatalf("final recovery state=%#v", finalState.Assembly)
	}

	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := store.ReadObservation(
		ctx,
		"turn-recovery",
		journal.MaxObservationAttempts,
		journal.MaxObservationEvents,
	)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	var targetAttempts int
	for _, attempt := range observation.Attempts {
		if attempt.Responsibility !=
			string(driver.ImplementerImplementation) {
			continue
		}
		targetAttempts++
		var usage driver.UsageReceipt
		if err := json.Unmarshal(attempt.Usage, &usage); err != nil ||
			usage.TokenStatus != driver.UsageReported ||
			usage.InputTokens == nil || *usage.InputTokens != 28 ||
			usage.OutputTokens == nil || *usage.OutputTokens != 20 {
			_ = store.Close()
			t.Fatalf("implementation usage=%s error=%v", attempt.Usage, err)
		}
	}
	if targetAttempts != 1 {
		_ = store.Close()
		t.Fatalf("implementation attempts=%d, want 1", targetAttempts)
	}

	var targetFacts []journal.EvaluationFact
	var parkAt time.Time
	var parks, recovered, falseAcceptances int
	window, err := store.VisitEvaluation(
		ctx,
		"turn-recovery",
		func(fact journal.EvaluationFact) {
			if fact.Kind == journal.EvaluationAttempt &&
				fact.Responsibility ==
					string(driver.ImplementerImplementation) {
				targetFacts = append(targetFacts, fact)
			}
			if fact.Kind != journal.EvaluationEvent {
				return
			}
			switch {
			case fact.EventKind == journal.RecoveryParkedEvent:
				parks++
				parkAt = fact.FinishedAt
			case strings.HasPrefix(
				fact.EventKind,
				"turn_recovery.outcome.recovered",
			):
				recovered++
			case fact.EventKind ==
				"turn_recovery.outcome.false_acceptance":
				falseAcceptances++
			}
		},
	)
	if closeErr := store.Close(); err != nil || closeErr != nil {
		t.Fatalf("visit evaluation error=%v close=%v", err, closeErr)
	}
	if parks != 1 || recovered != 1 || falseAcceptances != 0 ||
		len(targetFacts) != 1 ||
		!targetFacts[0].StartedAt.Before(parkAt) ||
		!targetFacts[0].FinishedAt.After(parkAt) {
		t.Fatalf(
			"durable recovery parks=%d recovered=%d false=%d facts=%#v park=%s",
			parks,
			recovered,
			falseAcceptances,
			targetFacts,
			parkAt,
		)
	}

	evalStore, err := journal.Open(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer evalStore.Close()
	statusReader, err := swornruntime.OpenStatusService(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer statusReader.Close()
	stateReader, err := cockpit.NewGitStateReader(e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := cockpit.NewProjector(
		evalStore,
		statusReader,
		stateReader,
	)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := observe.NewEvaluator(
		evalStore,
		projector,
		"1.0.0-rc.2",
	)
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(ctx, "turn-recovery")
	if err != nil || !changed {
		t.Fatalf("recovery eval changed=%t error=%v record=%#v",
			changed, err, record)
	}
	var implementationGroups []observe.AttemptGroup
	for _, group := range record.Groups {
		if group.Responsibility ==
			string(driver.ImplementerImplementation) {
			implementationGroups = append(implementationGroups, group)
		}
	}
	wantDuration := targetFacts[0].FinishedAt.Sub(
		targetFacts[0].StartedAt,
	).Nanoseconds()
	if record.SchemaVersion != journal.EvalSchemaVersionV2 ||
		record.TurnRecovery.Recovered != 1 ||
		record.TurnRecovery.HumanEscalations != 1 ||
		record.TurnRecovery.FalseAcceptances != 0 ||
		len(implementationGroups) != 1 ||
		implementationGroups[0].Attempts != 1 ||
		implementationGroups[0].Usage.InputTokens == nil ||
		*implementationGroups[0].Usage.InputTokens != 28 ||
		implementationGroups[0].Usage.OutputTokens == nil ||
		*implementationGroups[0].Usage.OutputTokens != 20 ||
		implementationGroups[0].DurationNS.Numerator == nil ||
		*implementationGroups[0].DurationNS.Numerator != wantDuration ||
		implementationGroups[0].DurationNS.Denominator == nil ||
		*implementationGroups[0].DurationNS.Denominator != 1 ||
		record.ElapsedNS != window.ObservedAt.Sub(
			window.Run.CreatedAt,
		).Nanoseconds() ||
		record.ElapsedNS < wantDuration {
		t.Fatalf(
			"recovery eval=%#v implementation=%#v want_duration=%d",
			record.TurnRecovery,
			implementationGroups,
			wantDuration,
		)
	}

	direct := runDirectTurnRecoveryBaseline(
		t,
		approvals,
		swornBinary,
		planBytes,
		plan,
	)
	recoveryProductTree := runGit(
		t,
		repository,
		"rev-parse",
		"main^{tree}",
	)
	directInput := *direct.implementation.Usage.InputTokens
	directOutput := *direct.implementation.Usage.OutputTokens
	recoveryInput := *implementationGroups[0].Usage.InputTokens
	recoveryOutput := *implementationGroups[0].Usage.OutputTokens
	inputDelta := recoveryInput - directInput
	outputDelta := recoveryOutput - directOutput
	elapsedDelta := record.ElapsedNS - direct.record.ElapsedNS
	if direct.implementation.DurationNS.Numerator == nil ||
		direct.implementation.DurationNS.Denominator == nil ||
		*direct.implementation.DurationNS.Denominator != 1 ||
		direct.record.RunState != record.RunState ||
		direct.record.Outcome != record.Outcome ||
		direct.productTree != recoveryProductTree ||
		direct.productContent != strings.TrimSuffix(recoveryE2EContent, "\n") ||
		inputDelta != 14 || outputDelta != 10 {
		t.Fatalf(
			"paired eval direct=%#v recovery=%#v input_delta=%+d output_delta=%+d direct_tree=%s recovery_tree=%s",
			direct.record,
			record,
			inputDelta,
			outputDelta,
			direct.productTree,
			recoveryProductTree,
		)
	}
	durationDelta := *implementationGroups[0].DurationNS.Numerator -
		*direct.implementation.DurationNS.Numerator
	t.Logf(
		"paired eval v2 direct elapsed_ns=%d recovery elapsed_ns=%d signed_delta=%+d; implementation duration_ns direct=%d recovery=%d signed_delta=%+d; tokens direct=%d/%d recovery=%d/%d signed_delta=%+d/%+d",
		direct.record.ElapsedNS,
		record.ElapsedNS,
		elapsedDelta,
		*direct.implementation.DurationNS.Numerator,
		*implementationGroups[0].DurationNS.Numerator,
		durationDelta,
		directInput,
		directOutput,
		recoveryInput,
		recoveryOutput,
		inputDelta,
		outputDelta,
	)
}
