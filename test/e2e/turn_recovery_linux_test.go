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
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/observe"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

const (
	recoveryE2ESecret  = "turn-recovery-e2e-secret"
	recoveryE2EAnswer  = "Use the exact approved recovery fixture value."
	recoveryE2EContent = "matched production outcome\n"
	// The production Planner's summary boundary. Every production run in
	// this file crosses it before a plan exists.
	recoveryE2ESummaryQuestion = "Summary of the result, scope, acceptance, " +
		"evidence, inputs, and limits I intend to promise."
	recoveryE2ESummaryAnswer = "Confirmed; plan exactly that."
)

// answerRecoveryPlannerSummary answers the one open production Planner summary
// turn through the real binary and returns the resulting stdout.
func answerRecoveryPlannerSummary(
	t *testing.T,
	binary, runID, journalPath, configPath string,
	environment map[string]string,
) string {
	t.Helper()
	boardBody, boardErr := runBinary(
		t, binary, 0,
		"board", "--run", runID, "--journal", journalPath, "--json",
	)
	var board cockpit.Snapshot
	if boardErr != "" ||
		json.Unmarshal([]byte(boardBody), &board) != nil {
		t.Fatalf("planner summary board=%q stderr=%q", boardBody, boardErr)
	}
	var open []cockpit.AttentionView
	for _, attention := range board.Runtime.Attentions {
		if attention.State == "open" && attention.HumanTurn != nil {
			open = append(open, attention)
		}
	}
	if len(open) != 1 ||
		open[0].Question != recoveryE2ESummaryQuestion ||
		open[0].HumanTurn.Responsibility != string(driver.PlannerProposal) {
		t.Fatalf("planner summary attentions = %#v", board.Runtime.Attentions)
	}
	stdout, stderr := runBinaryWithEnvironment(
		t, binary, 0, environment,
		"answer", "--run", runID, "--journal", journalPath,
		"--attention", open[0].ID, "--generation", "1",
		"--answer", recoveryE2ESummaryAnswer, "--config", configPath,
	)
	if stderr != "" {
		t.Fatalf("planner summary answer stdout=%q stderr=%q", stdout, stderr)
	}
	manifestPath := filepath.Join(filepath.Dir(journalPath), "manifest.json")
	driveStdout, driveStderr := runBinaryWithEnvironment(
		t, binary, 0, environment,
		"run", "--manifest", manifestPath, "--journal", journalPath,
		"--config", configPath,
	)
	if driveStderr != "" {
		t.Fatalf("planner summary drive stdout=%q stderr=%q", driveStdout, driveStderr)
	}
	return driveStdout
}

type recoveryE2EProvider struct {
	t         *testing.T
	planBytes []byte
	recover   bool
	human     bool
	question  string
	answer    string

	mu              sync.Mutex
	turns           map[string]int
	automationCalls int
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
		provider.mu.Lock()
		provider.automationCalls++
		provider.mu.Unlock()
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
	if prompt.Responsibility == driver.PlannerProposal {
		return provider.plannerResponse(prompt, turn)
	}
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
		kind := driver.YieldQuestion
		if provider.human {
			kind = driver.YieldHumanConfirmation
		}
		question := provider.question
		if question == "" {
			question = "Which exact approved value should I use?"
		}
		return "sworn_yield", map[string]any{"yield": map[string]any{
			"schema_version": driver.YieldSchemaVersion,
			"invocation_id":  prompt.InvocationID,
			"kind":           kind,
			"message":        question,
		}}, nil
	case prompt.Recovery == nil ||
		prompt.Recovery.Kind != driver.RecoverableInputAnswer ||
		prompt.Recovery.Content != provider.expectedAnswer():
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
	case turn == 3 || provider.human && turn > 3:
		arguments, err := provider.submissionArguments(prompt)
		return "sworn_submit", arguments, err
	default:
		return "", nil, fmt.Errorf("implementation turn=%d", turn)
	}
}

// plannerResponse crosses the summary-before-plan boundary: the Planner's
// first terminal is the human-only summary turn, and only the responsibility
// resumed from the answer emits plan bytes.
func (provider *recoveryE2EProvider) plannerResponse(
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

func (provider *recoveryE2EProvider) expectedAnswer() string {
	if provider.answer != "" {
		return provider.answer
	}
	return recoveryE2EAnswer
}

func (provider *recoveryE2EProvider) submissionArguments(
	prompt recoveryE2EModelPrompt,
) (map[string]any, error) {
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   prompt.InvocationID,
		Responsibility: prompt.Responsibility,
		Summary:        "Deterministic turn recovery fixture padded so every scripted responsibility this recovery journey drives clears the submission content floor for its coverage.",
		Detail:         "Bound to the admitted production responsibility, padded so every scripted responsibility this recovery journey drives clears the submission detail content floor for its coverage, well past the two-hundred-byte bound.\n",
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
		GitIdentity:       gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
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
	swornBinary string,
	planBytes []byte,
	plan baton.Plan,
) recoveryE2EPairedResult {
	t.Helper()

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
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("direct start stdout=%q stderr=%q", stdout, stderr)
	}
	stdout = answerRecoveryPlannerSummary(
		t, swornBinary, "turn-recovery-direct", journalPath, configPath,
		environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("direct summary answer stdout=%q", stdout)
	}
	authorizePlan(t, journalPath, "turn-recovery-direct", plan)
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
	if stderr != "" {
		t.Fatalf("direct resume stdout=%q stderr=%q", stdout, stderr)
	}
	stdout, stderr = runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
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
	// The baseline still crosses the Planner's summary boundary - that is one
	// human escalation every production run now has - but its implementation
	// turn is direct, which is what this baseline exists to measure.
	if record.SchemaVersion != journal.EvalSchemaVersionV3 ||
		record.TurnRecovery.Recovered != 1 ||
		record.TurnRecovery.HumanEscalations != 1 ||
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

func TestProductionHumanOnlyTurnUsesOneDurableOperatorBoundary(
	t *testing.T,
) {
	const (
		runID          = "human-only-turn"
		questionCanary = "HUMAN-QUESTION-CANARY-7f6f"
		answerCanary   = "HUMAN-ANSWER-CANARY-approval-receipt-code"
	)
	repository := newProductRepository(t)
	planBytes, plan := recoveryE2EPlan(t)
	provider := &recoveryE2EProvider{
		t: t, planBytes: planBytes, recover: true, human: true,
		question: questionCanary, answer: answerCanary,
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
		recoveryE2EManifest(t, runID, repository, loaded),
	)
	journalPath := filepath.Join(root, "run.sqlite")
	swornBinary := filepath.Join(root, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")
	environment := map[string]string{
		"SWORN_TURN_RECOVERY_KEY": recoveryE2ESecret,
	}
	stdout, stderr := runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run", "--manifest", manifestPath,
		"--journal", journalPath, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("human start stdout=%q stderr=%q", stdout, stderr)
	}
	stdout = answerRecoveryPlannerSummary(
		t, swornBinary, runID, journalPath, configPath, environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("human summary answer stdout=%q", stdout)
	}
	authorizePlan(t, journalPath, runID, plan)
	installApprovedPlan(t, repository, planBytes)
	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"resume", "--run", runID, "--journal", journalPath,
		"--command", "human-resume-1", "--generation", "0",
		"--config", configPath,
	)
	if stderr != "" {
		t.Fatalf("human resume stdout=%q stderr=%q", stdout, stderr)
	}
	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run", "--manifest", manifestPath,
		"--journal", journalPath, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("human park stdout=%q stderr=%q", stdout, stderr)
	}

	boardBody, boardErr := runBinary(
		t, swornBinary, 0,
		"board", "--run", runID, "--journal", journalPath, "--json",
	)
	var board cockpit.Snapshot
	if boardErr != "" || json.Unmarshal([]byte(boardBody), &board) != nil ||
		len(board.Runtime.Attentions) != 1 {
		t.Fatalf("human board body=%q stderr=%q", boardBody, boardErr)
	}
	attention := board.Runtime.Attentions[0]
	if attention.State != "open" || attention.Generation != 1 ||
		attention.Question != questionCanary || attention.HumanTurn == nil ||
		attention.HumanTurn.Kind != string(driver.YieldHumanConfirmation) ||
		attention.HumanTurn.RunID != runID ||
		attention.HumanTurn.Track != "T1" ||
		attention.HumanTurn.Slice != "S1" ||
		attention.HumanTurn.Role != string(driver.RoleImplementer) ||
		attention.HumanTurn.Responsibility !=
			string(driver.ImplementerImplementation) ||
		attention.HumanTurn.OpenGeneration != 1 {
		t.Fatalf("human attention=%#v", attention)
	}
	var answerActions int
	for _, action := range board.Actions {
		if action.Kind != "answer_attention" {
			continue
		}
		answerActions++
		if action.RunID != runID || action.AttentionID != attention.ID ||
			action.ExpectedGeneration != attention.Generation {
			t.Fatalf("human answer action=%#v", action)
		}
	}
	provider.mu.Lock()
	automationCalls := provider.automationCalls
	provider.mu.Unlock()
	if answerActions != 1 || automationCalls != 0 {
		t.Fatalf("answer actions=%d automation calls=%d", answerActions, automationCalls)
	}

	beforeAnswer := readBatonState(t, repository, "turn-recovery-release")
	targetBefore := runGit(t, repository, "rev-parse", "main")
	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"answer", "--run", runID, "--journal", journalPath,
		"--attention", attention.ID, "--generation", "1",
		"--answer", answerCanary, "--config", configPath,
	)
	if stderr != "" {
		t.Fatalf("human answer stdout=%q stderr=%q", stdout, stderr)
	}
	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run", "--manifest", manifestPath,
		"--journal", journalPath, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("human answer stdout=%q stderr=%q", stdout, stderr)
	}
	finalState := readBatonState(t, repository, "turn-recovery-release")
	if beforeAnswer.Plan.OID != finalState.Plan.OID ||
		finalState.Assembly.Outcome != "merged" ||
		runGit(t, repository, "rev-parse", "main") == targetBefore ||
		runGit(t, repository, "show", "main:one.txt") !=
			strings.TrimSuffix(recoveryE2EContent, "\n") {
		t.Fatalf("human final state=%#v", finalState.Assembly)
	}

	ctx := context.Background()
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var opens, answers, resolves, terminalSubmissions int
	for _, command := range snapshot.Commands {
		switch command.Kind {
		case "attention.open":
			opens++
		case "attention.answer":
			answers++
		case "attention.resolve":
			resolves++
		}
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind == "driver.dispatch" &&
			effect.State == journal.Succeeded &&
			strings.Contains(string(effect.Result),
				`"responsibility":"implementer_implementation"`) {
			terminalSubmissions++
		}
	}
	for _, event := range snapshot.Events {
		if strings.Contains(string(event.Body), questionCanary) ||
			strings.Contains(string(event.Body), answerCanary) {
			t.Fatalf("human content escaped into event %q", event.Kind)
		}
	}
	var evaluationFacts []journal.EvaluationFact
	_, err = store.VisitEvaluation(
		ctx,
		runID,
		func(fact journal.EvaluationFact) {
			evaluationFacts = append(evaluationFacts, fact)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	evaluationBody, _ := json.Marshal(evaluationFacts)
	if strings.Contains(string(evaluationBody), questionCanary) ||
		strings.Contains(string(evaluationBody), answerCanary) {
		t.Fatal("human content escaped into evaluation observation")
	}
	observation, err := store.ReadObservation(
		ctx,
		runID,
		journal.MaxObservationAttempts,
		journal.MaxObservationEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	notificationBody, _ := json.Marshal(observation.Notifications)
	if strings.Contains(string(notificationBody), questionCanary) ||
		strings.Contains(string(notificationBody), answerCanary) {
		t.Fatal("human content escaped into notification observation")
	}

	statusReader, err := swornruntime.OpenStatusService(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer statusReader.Close()
	stateReader, err := cockpit.NewGitStateReader(e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := cockpit.NewProjector(store, statusReader, stateReader)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := observe.NewEvaluator(store, projector, "1.0.0-rc.2")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(ctx, runID)
	if err != nil || !changed {
		t.Fatalf("human eval changed=%t error=%v", changed, err)
	}
	recordBody, _ := json.Marshal(record)
	if strings.Contains(string(recordBody), questionCanary) ||
		strings.Contains(string(recordBody), answerCanary) {
		t.Fatal("human content escaped into telemetry record")
	}

	var captureMu sync.Mutex
	var capturedOTLP [][]byte
	otlpServer := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		defer request.Body.Close()
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Errorf("read OTLP body: %v", readErr)
		}
		captureMu.Lock()
		capturedOTLP = append(capturedOTLP, body)
		captureMu.Unlock()
		writer.WriteHeader(http.StatusOK)
	}))
	defer otlpServer.Close()
	telemetry, err := observe.NewOTLP(
		ctx,
		observe.Config{
			SchemaVersion: observe.OTelConfigSchemaVersion,
			Endpoint:      otlpServer.URL,
			Headers:       map[string]string{},
		},
		"1.0.0-rc.2",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(record) {
		t.Fatal("human telemetry record was not accepted")
	}
	deadline := time.Now().Add(2 * time.Second)
	for telemetry.Status().Processed < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if telemetry.Status().Processed != 1 {
		t.Fatalf("human telemetry status=%#v", telemetry.Status())
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(ctx, 5*time.Second)
	defer cancelShutdown()
	if err := telemetry.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	captureMu.Lock()
	gotOTLP := append([][]byte(nil), capturedOTLP...)
	captureMu.Unlock()
	if len(gotOTLP) != 2 {
		t.Fatalf("captured OTLP requests=%d", len(gotOTLP))
	}
	for _, body := range gotOTLP {
		if strings.Contains(string(body), questionCanary) ||
			strings.Contains(string(body), answerCanary) {
			t.Fatal("human content escaped into captured OTLP payload")
		}
	}
	// The Planner's summary boundary and the implementation turn are two
	// human turns; each is opened, answered, and resolved exactly once.
	if opens != 2 || answers != 2 || resolves != 2 ||
		terminalSubmissions != 1 {
		t.Fatalf(
			"open=%d answer=%d resolve=%d terminal=%d",
			opens, answers, resolves, terminalSubmissions,
		)
	}
}

func TestProductionHumanTurnCrashBarriersReconcileExactlyOnce(
	t *testing.T,
) {
	buildRoot := t.TempDir()
	normalBinary := filepath.Join(buildRoot, "sworn-normal")
	buildBinary(t, normalBinary, "./cmd/sworn", "")
	crashBinary := filepath.Join(buildRoot, "sworn-human-crash")
	buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
	cuts := []string{
		"before_park_commit",
		"after_park_commit",
		"after_answer_commit",
		"after_owner_wake",
		"after_continuation_rehydration",
		"after_terminal_handoff",
		"after_terminal_completion",
	}
	var cases sync.WaitGroup
	for index, cut := range cuts {
		index, cut := index, cut
		cases.Add(1)
		go func() {
			defer cases.Done()
			t.Run(cut, func(t *testing.T) {
				runID := fmt.Sprintf("human-crash-%d", index+1)
				repository := newProductRepository(t)
				planBytes, plan := recoveryE2EPlan(t)
				question := "QUESTION-CANARY-" + cut
				answer := "ANSWER-CANARY-" + cut
				provider := &recoveryE2EProvider{
					t: t, planBytes: planBytes, recover: true, human: true,
					question: question, answer: answer,
					turns: make(map[string]int),
				}
				providerHTTP := httptest.NewServer(
					http.HandlerFunc(provider.serve),
				)
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
					recoveryE2EManifest(t, runID, repository, loaded),
				)
				journalPath := filepath.Join(root, "run.sqlite")
				environment := map[string]string{
					"SWORN_TURN_RECOVERY_KEY":       recoveryE2ESecret,
					"SWORN_TEST_OWNER_LEASE_MILLIS": testLeaseMillis,
					"SWORN_TEST_HUMAN_TURN_CRASH":   cut,
				}
				runBinaryWithEnvironment(
					t, normalBinary, 0, environment,
					"run", "--manifest", manifestPath,
					"--journal", journalPath, "--config", configPath,
				)
				summary := answerRecoveryPlannerSummary(
					t, normalBinary, runID, journalPath, configPath,
					environment,
				)
				if !strings.Contains(
					summary, "  state: awaiting_approval",
				) {
					t.Fatalf("summary answer stdout=%q", summary)
				}
				authorizePlan(t, journalPath, runID, plan)
				installApprovedPlan(t, repository, planBytes)

				crashDuringPark := cut == "before_park_commit" ||
					cut == "after_park_commit"
				if crashDuringPark {
					runBinaryWithEnvironment(
						t, crashBinary, 86, environment,
						"resume", "--run", runID, "--journal", journalPath,
						"--command", "resume-crash", "--generation", "0",
						"--config", configPath,
					)
				} else {
					runBinaryWithEnvironment(
						t, normalBinary, 0, environment,
						"resume", "--run", runID, "--journal", journalPath,
						"--command", "resume-park", "--generation", "0",
						"--config", configPath,
					)
					stdout, stderr := runBinaryWithEnvironment(
						t, normalBinary, 0, environment,
						"run", "--manifest", manifestPath,
						"--journal", journalPath, "--config", configPath,
					)
					if stderr != "" || !strings.Contains(stdout, "  state: parked") {
						t.Fatalf("pre-crash park stdout=%q stderr=%q", stdout, stderr)
					}
					boardBody, boardErr := runBinary(
						t, normalBinary, 0,
						"board", "--run", runID,
						"--journal", journalPath, "--json",
					)
					var board cockpit.Snapshot
					if boardErr != "" ||
						json.Unmarshal([]byte(boardBody), &board) != nil ||
						len(board.Runtime.Attentions) != 1 {
						t.Fatalf("pre-crash board=%q stderr=%q", boardBody, boardErr)
					}
					runBinaryWithEnvironment(
						t, crashBinary, 86, environment,
						"answer", "--run", runID, "--journal", journalPath,
						"--attention", board.Runtime.Attentions[0].ID,
						"--generation", "1", "--answer", answer,
						"--config", configPath,
					)
				}

				if cut == "after_answer_commit" {
					boardBody, _ := runBinary(
						t, normalBinary, 0,
						"board", "--run", runID,
						"--journal", journalPath, "--json",
					)
					var board cockpit.Snapshot
					if json.Unmarshal([]byte(boardBody), &board) != nil ||
						len(board.Runtime.Attentions) != 1 {
						t.Fatalf("answered board=%q", boardBody)
					}
					stdout, stderr := runBinaryWithEnvironment(
						t, normalBinary, 0, environment,
						"answer", "--run", runID, "--journal", journalPath,
						"--attention", board.Runtime.Attentions[0].ID,
						"--generation", "1", "--answer", answer,
						"--config", configPath,
					)
					if stderr != "" {
						t.Fatalf("answer replay stderr=%q", stderr)
					}
					stdout, stderr = runBinaryWithEnvironment(
						t, normalBinary, 0, environment,
						"run", "--manifest", manifestPath,
						"--journal", journalPath, "--config", configPath,
					)
					if stderr != "" || !strings.Contains(stdout, "  state: complete") {
						t.Fatalf("answer replay stdout=%q stderr=%q", stdout, stderr)
					}
				} else {
					leaseExpiryWait()
					boardBody, boardErr := runBinary(
						t, normalBinary, 0,
						"board", "--run", runID,
						"--journal", journalPath, "--json",
					)
					var board cockpit.Snapshot
					if boardErr != "" ||
						json.Unmarshal([]byte(boardBody), &board) != nil {
						t.Fatalf("recovery board=%q stderr=%q", boardBody, boardErr)
					}
					stdout, stderr := runBinaryWithEnvironment(
						t, normalBinary, 0, environment,
						"takeover", "--run", runID, "--journal", journalPath,
						"--command", "takeover-after-crash",
						"--generation", fmt.Sprint(board.Run.ControlGeneration),
						"--config", configPath,
					)
					if stderr != "" {
						t.Fatalf("takeover stderr=%q", stderr)
					}
					if crashDuringPark {
						stdout, stderr = runBinaryWithEnvironment(
							t, normalBinary, 0, environment,
							"run", "--manifest", manifestPath,
							"--journal", journalPath, "--config", configPath,
						)
						if stderr != "" || !strings.Contains(stdout, "  state: parked") {
							t.Fatalf("park takeover stdout=%q stderr=%q", stdout, stderr)
						}
						boardBody, _ = runBinary(
							t, normalBinary, 0,
							"board", "--run", runID,
							"--journal", journalPath, "--json",
						)
						if json.Unmarshal([]byte(boardBody), &board) != nil ||
							len(board.Runtime.Attentions) != 1 {
							t.Fatalf("parked recovery board=%q", boardBody)
						}
						stdout, stderr = runBinaryWithEnvironment(
							t, normalBinary, 0, environment,
							"answer", "--run", runID,
							"--journal", journalPath,
							"--attention", board.Runtime.Attentions[0].ID,
							"--generation", "1", "--answer", answer,
							"--config", configPath,
						)
						if stderr != "" {
							t.Fatalf("answer after park stderr=%q", stderr)
						}
					}
					stdout, stderr = runBinaryWithEnvironment(
						t, normalBinary, 0, environment,
						"run", "--manifest", manifestPath,
						"--journal", journalPath, "--config", configPath,
					)
					if stderr != "" || !strings.Contains(stdout, "  state: complete") {
						t.Fatalf("crash recovery stdout=%q stderr=%q", stdout, stderr)
					}
				}

				store, err := journal.OpenReadOnly(
					context.Background(),
					journalPath,
				)
				if err != nil {
					t.Fatal(err)
				}
				snapshot, err := store.Snapshot(context.Background(), runID)
				_ = store.Close()
				if err != nil {
					t.Fatal(err)
				}
				var opens, answers, resolves, terminal int
				for _, command := range snapshot.Commands {
					switch command.Kind {
					case "attention.open":
						opens++
					case "attention.answer":
						answers++
					case "attention.resolve":
						resolves++
					}
				}
				for _, effect := range snapshot.Effects {
					if effect.Kind == "driver.dispatch" &&
						effect.State == journal.Succeeded &&
						strings.Contains(string(effect.Result),
							`"responsibility":"implementer_implementation"`) {
						terminal++
					}
				}
				// Two human turns now exist per run: the Planner's
				// summary boundary and the implementation turn this cut
				// interrupts. Each must still be opened, answered, and
				// resolved exactly once.
				if opens != 2 || answers != 2 || resolves != 2 ||
					terminal != 1 {
					t.Fatalf(
						"cut=%s open=%d answer=%d resolve=%d terminal=%d",
						cut, opens, answers, resolves, terminal,
					)
				}
				// A crash around a human park or answer is one of the
				// interruptions the conformance profile's restart case
				// promises to survive; this is where a real binary was
				// observed doing it, through the command line and through
				// the configured driver that was re-dispatched.
				recordSwornConformance(
					t, caseRestartRecovery, surfaceCLI,
					"human-turn/"+cut+"/cli",
				)
				recordSwornConformance(
					t, caseRestartRecovery, surfaceConfiguredDriver,
					"human-turn/"+cut+"/configured-driver",
				)
			})
		}()
	}
	cases.Wait()
}

func TestProductionTurnRecoveryParksRestartsAndAccountsExactlyOnce(
	t *testing.T,
) {
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
	buildBinary(t, swornBinary, "./cmd/sworn", "")
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
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("recovery start stdout=%q stderr=%q", stdout, stderr)
	}
	stdout = answerRecoveryPlannerSummary(
		t, swornBinary, "turn-recovery", journalPath, configPath, environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("recovery summary answer stdout=%q", stdout)
	}

	authorizePlan(t, journalPath, "turn-recovery", plan)
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
	if stderr != "" {
		t.Fatalf("recovery resume stdout=%q stderr=%q", stdout, stderr)
	}
	stdout, stderr = runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
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
	if stderr != "" {
		t.Fatalf("recovery answer stdout=%q stderr=%q", stdout, stderr)
	}
	stdout, stderr = runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		environment,
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
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
			usage.InputTokens == nil || *usage.InputTokens != 21 ||
			usage.OutputTokens == nil || *usage.OutputTokens != 15 {
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
	// Two parks now occur: the Planner's summary boundary and the
	// implementation turn this test restarts across.
	if parks != 2 || recovered != 2 || falseAcceptances != 0 ||
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
	if record.SchemaVersion != journal.EvalSchemaVersionV3 ||
		record.TurnRecovery.Recovered != 2 ||
		record.TurnRecovery.HumanEscalations != 2 ||
		record.TurnRecovery.FalseAcceptances != 0 ||
		len(implementationGroups) != 1 ||
		implementationGroups[0].Attempts != 1 ||
		implementationGroups[0].Usage.InputTokens == nil ||
		*implementationGroups[0].Usage.InputTokens != 21 ||
		implementationGroups[0].Usage.OutputTokens == nil ||
		*implementationGroups[0].Usage.OutputTokens != 15 ||
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
		inputDelta != 7 || outputDelta != 5 {
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
