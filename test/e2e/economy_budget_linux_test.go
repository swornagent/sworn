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
	"os/exec"
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
	// economyOutputTokenBudgetTurns, economyOutputTokenPerStallTurn and
	// economyOutputTokenBudget drive TestRealBinaryEconomyOutputTokenBudget-
	// ParksGrantsAndResumes's separate real-adapter A1 case: the same first
	// Implementer attempt writes scoped code and never submits, but this
	// time the real driver's own ECONOMY_OUTPUT_BUDGET_EXCEEDED cumulative-
	// output-token guard ends it, not the turn guard - so the manifest's own
	// max_turns_per_work stays generous (economyOutputTokenGenerousTurns)
	// while max_output_tokens_per_work is the tiny, real ceiling. Three
	// requests of economyOutputTokenPerStallTurn each cross
	// economyOutputTokenBudget on the loop-top check before a fourth
	// request would ever be built, leaving every other scripted
	// responsibility (each reporting the fixture's small default per-turn
	// count) generous headroom under the same ceiling.
	economyOutputTokenBudgetTurns   = 3
	economyOutputTokenPerStallTurn  = 3_000
	economyOutputTokenBudget        = 8_000
	economyOutputTokenGenerousTurns = 500
	economyOutputTokenGrant         = 2_000
	// economyIndependentTrackContent is the content the second, independent
	// track's Implementer writes and submits on its own first attempt in
	// TestRealBinaryEconomyTurnBudgetParksOneTrackWhileIndependentTrack-
	// Completes (S4-resumable-budget-stops A5): it never touches the
	// economy-parked track's own scope path, so both tracks' real
	// production-adapter cases share one HTTP provider without colliding.
	economyIndependentTrackContent = "economy budget independent track content\n"
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

// economyBudgetTwoTrackPlan adds a second, wholly independent track T2/S2
// (scoped to two.txt, never one.txt) beside economyBudgetPlan's T1/S1, in
// the same shape the pre-existing walking-skeleton/topology real-binary
// fixtures already use for two independent tracks (e2ePlan): neither track
// depends on the other, so S4-resumable-budget-stops A5's economy-parked-
// track-beside-a-finishing-track scenario needs no new scheduling behavior,
// only a second scope for the same real HTTP-provider journey to drive.
func economyBudgetTwoTrackPlan(t *testing.T) ([]byte, baton.Plan) {
	t.Helper()
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       "economy-budget-two-track-release",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://economy-budget-two-track-release/1",
		Tracks: []baton.Track{
			{
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
			},
			{
				ID:        "T2",
				DependsOn: []string{},
				Slices: []baton.Slice{{
					ID:      "S2",
					Outcome: "Deliver the independent-track fixture value.",
					Scope: baton.Scope{
						Include: []string{"two.txt"},
						Exclude: []string{},
					},
					Acceptance: []baton.Criterion{{
						ID:   "A-S2",
						Text: "The independent value is present in the exact product tree.",
					}},
					Checks:      []string{"check two.txt"},
					Constraints: []string{"deterministic local provider"},
					DependsOn:   []string{},
					Consumes:    []string{},
				}},
			},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nDeterministic real-binary economy-budget two-track E2E.\n",
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
	runID, repository, release string,
	config driver.LoadedDriverConfig,
	maxTurnsPerWork, maxOutputTokensPerWork int64,
	maxParallelTracks int,
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
		Release:           release,
		TargetRef:         "refs/heads/main",
		Intent:            "Prove a real turn-budget park, an explicit grant and resumed completion.",
		MaxParallelTracks: maxParallelTracks,
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
			TimeoutMillis:          30_000,
			OutputBytes:            65_536,
			MaxTurnsPerWork:        maxTurnsPerWork,
			MaxOutputTokensPerWork: maxOutputTokensPerWork,
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
	// stallTurns overrides economyBudgetStallTurns for the first
	// Implementer attempt's own park-triggering turn count. Zero keeps the
	// turn-budget default; a caller driving a different economy unit (e.g.
	// output tokens) names the number of real requests that unit's own
	// crossing takes to cross its own, separately configured budget.
	stallTurns int
	// stallTokensPerTurn, when set, is the completion_tokens the first
	// Implementer attempt's own requests report - the real per-request
	// signal an output-token budget crossing is measured from - instead of
	// the fixture's default per-turn token count every other responsibility
	// and every other attempt keeps reporting.
	stallTokensPerTurn int64

	mu                 sync.Mutex
	turns              map[string]int
	verificationReruns map[string]int
}

func (provider *economyBudgetProvider) effectiveStallTurns() int {
	if provider.stallTurns > 0 {
		return provider.stallTurns
	}
	return economyBudgetStallTurns
}

// completionTokens is the real per-request completion_tokens the scripted
// HTTP provider reports. Only the first Implementer attempt (invocation
// epoch 1, which never submits) reports the fixture's raised per-turn
// count when one is configured; every other attempt and responsibility
// keeps the small fixed count the turn-budget scenario already relies on
// staying well under any economy ceiling this journey configures.
func (provider *economyBudgetProvider) completionTokens(
	prompt recoveryE2EModelPrompt,
) int64 {
	if provider.stallTokensPerTurn > 0 &&
		prompt.Responsibility == driver.ImplementerImplementation {
		parts := strings.Split(prompt.InvocationID, "/")
		if len(parts) == 6 && parts[4] == "1" {
			return provider.stallTokensPerTurn
		}
	}
	return 5
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
			"prompt_tokens": 7, "completion_tokens": provider.completionTokens(prompt),
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
		parts := strings.Split(prompt.InvocationID, "/")
		checkPath := "one.txt"
		if len(parts) == 6 && parts[1] != "S1" {
			checkPath = "two.txt"
		}
		return "Bash", map[string]any{"script": "check " + checkPath + " || true"}, nil
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
// real content and submits well inside its granted headroom. A dispatch for
// any slice other than S1 (only present in
// TestRealBinaryEconomyTurnBudgetParksOneTrackWhileIndependentTrackCompletes'
// second, independent track) never stalls regardless of epoch: A5 needs
// that track to finish on its own while S1 is parked, not to reach its own
// economy ceiling.
func (provider *economyBudgetProvider) implementerResponse(
	prompt recoveryE2EModelPrompt,
	turn int,
) (string, map[string]any, error) {
	parts := strings.Split(prompt.InvocationID, "/")
	if len(parts) != 6 {
		return "", nil, fmt.Errorf("unexpected invocation id %q", prompt.InvocationID)
	}
	if parts[1] != "S1" {
		switch turn {
		case 1:
			return "Write", map[string]any{
				"path":    "/workspace/two.txt",
				"content": economyIndependentTrackContent,
			}, nil
		case 2:
			arguments, err := provider.submissionArguments(prompt)
			return "sworn_submit", arguments, err
		default:
			return "", nil, fmt.Errorf("independent track implementer attempt reached turn %d", turn)
		}
	}
	if parts[4] == "1" {
		stallTurns := provider.effectiveStallTurns()
		switch {
		case turn < stallTurns:
			return "Bash", map[string]any{"script": "true"}, nil
		case turn == stallTurns:
			return "Write", map[string]any{
				"path":    "/workspace/one.txt",
				"content": "interim unsubmitted content, never accepted\n",
			}, nil
		default:
			return "", nil, fmt.Errorf(
				"first implementer attempt reached turn %d; its economy "+
					"budget should have stopped it at %d", turn, stallTurns,
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
			t, "economy-budget", repository, "economy-budget-release", loaded,
			economyBudgetStallTurns, 0, 1,
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

	// A1/S1-durable-unverified-checkpoints: the affected work parks "with
	// that code durably retained" - not merely un-submitted. Read the
	// journal's own unverified checkpoint back and confirm the first
	// attempt's uncommitted /workspace/one.txt content is durably reachable
	// under the checkpoint ref in the real target repository, distinct from
	// the granted attempt's later fresh write (which this test asserts
	// separately, after the grant, as the accepted final content).
	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checkpoints, err := store.ListUnverifiedCheckpoints(ctx, "economy-budget")
	if err != nil || len(checkpoints) == 0 {
		t.Fatalf("no unverified checkpoints recorded in journal: %v (%d)", err, len(checkpoints))
	}
	preservedCheckpoint := checkpoints[len(checkpoints)-1]
	if preservedCheckpoint.Slice != "S1" ||
		preservedCheckpoint.DispatchWork != status.PinnedWork[0].DispatchWorkID ||
		preservedCheckpoint.Epoch != 1 || preservedCheckpoint.CheckpointRef == "" {
		t.Fatalf("checkpoint = %#v, want the parked attempt's own", preservedCheckpoint)
	}
	runGit(t, repository, "rev-parse", "--verify", preservedCheckpoint.CheckpointRef)
	preservedContent, preserveErr := exec.Command(
		e2eGit, "-C", repository, "show", preservedCheckpoint.CheckpointRef+":one.txt",
	).Output()
	if preserveErr != nil ||
		string(preservedContent) != "interim unsubmitted content, never accepted\n" {
		t.Fatalf(
			"checkpoint one.txt = %q, error = %v; the parked attempt's "+
				"uncommitted work was not durably preserved",
			preservedContent, preserveErr,
		)
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

// TestRealBinaryEconomyOutputTokenBudgetParksGrantsAndResumes drives the
// same compiled sworn binary and real OpenAI-shaped HTTP provider as
// TestRealBinaryEconomyTurnBudgetParksGrantsAndResumes, but crosses a
// separate real economy unit (S4-resumable-budget-stops A1's second
// anchor): a manifest with a generous turn budget but a tiny
// max_output_tokens_per_work forces the Implementer's first attempt to
// reach that ceiling - measured from the real per-request completion_tokens
// counts the driver's own conversation loop actually accumulates, never a
// fixture-injected receipt - with scoped code already written but never
// submitted. The park, its checkpoint, the board's named unit, the grant
// and the resumed completion all reuse the identical real surfaces the
// turn-budget scenario proves, this time under ParkCauseEconomyOutputTokens.
func TestRealBinaryEconomyOutputTokenBudgetParksGrantsAndResumes(t *testing.T) {
	t.Parallel()
	repository := newProductRepository(t)
	planBytes, plan := economyBudgetPlan(t)
	provider := &economyBudgetProvider{
		t: t, planBytes: planBytes, turns: make(map[string]int),
		stallTurns:         economyOutputTokenBudgetTurns,
		stallTokensPerTurn: economyOutputTokenPerStallTurn,
	}
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
			t, "economy-output-tokens", repository, "economy-budget-release", loaded,
			economyOutputTokenGenerousTurns, economyOutputTokenBudget, 1,
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
		t, swornBinary, "economy-output-tokens", journalPath, configPath, environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("planner summary answer stdout=%q", stdout)
	}

	authorizePlan(t, journalPath, "economy-output-tokens", plan)
	installApprovedPlan(t, repository, planBytes)

	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"resume", "--run", "economy-output-tokens", "--journal", journalPath,
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
			"expected the implementer's first attempt to exhaust its real "+
				"output-token budget and park: stdout=%q stderr=%q", stdout, stderr,
		)
	}

	statusBody, statusErr := runBinary(
		t, swornBinary, 0, "status", "--run", "economy-output-tokens", "--journal", journalPath, "--json",
	)
	var status swornruntime.RunStatus
	if statusErr != "" || json.Unmarshal([]byte(statusBody), &status) != nil {
		t.Fatalf("status body=%q stderr=%q", statusBody, statusErr)
	}
	spentTokens := int64(economyOutputTokenBudgetTurns) * economyOutputTokenPerStallTurn
	if status.State != "parked" || status.Park == nil ||
		status.Park.Cause != swornruntime.ParkCauseEconomyOutputTokens ||
		status.Park.Spent != spentTokens ||
		status.Park.Budget != economyOutputTokenBudget ||
		status.Park.UnblockKnob != swornruntime.EconomyOutputTokensUnblockKnob {
		t.Fatalf("economy output-token park status = %#v", status.Park)
	}
	if len(status.PinnedWork) != 1 ||
		status.PinnedWork[0].Cause != swornruntime.ParkCauseEconomyOutputTokens ||
		status.PinnedWork[0].DispatchWorkID == "" {
		t.Fatalf("pinned work = %#v", status.PinnedWork)
	}
	if runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("parked run advanced target authority before any candidate was accepted")
	}

	// A1/S1-durable-unverified-checkpoints: the same durable-preservation
	// fact the turn-budget scenario proves, now for the output-token unit -
	// the first attempt's uncommitted /workspace/one.txt is durably
	// reachable under the real journal's checkpoint ref in the real target
	// repository, distinct from the granted attempt's later fresh write.
	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checkpoints, err := store.ListUnverifiedCheckpoints(ctx, "economy-output-tokens")
	if err != nil || len(checkpoints) == 0 {
		t.Fatalf("no unverified checkpoints recorded in journal: %v (%d)", err, len(checkpoints))
	}
	preservedCheckpoint := checkpoints[len(checkpoints)-1]
	if preservedCheckpoint.Slice != "S1" ||
		preservedCheckpoint.DispatchWork != status.PinnedWork[0].DispatchWorkID ||
		preservedCheckpoint.Epoch != 1 || preservedCheckpoint.CheckpointRef == "" {
		t.Fatalf("checkpoint = %#v, want the parked attempt's own", preservedCheckpoint)
	}
	runGit(t, repository, "rev-parse", "--verify", preservedCheckpoint.CheckpointRef)
	preservedContent, preserveErr := exec.Command(
		e2eGit, "-C", repository, "show", preservedCheckpoint.CheckpointRef+":one.txt",
	).Output()
	if preserveErr != nil ||
		string(preservedContent) != "interim unsubmitted content, never accepted\n" {
		t.Fatalf(
			"checkpoint one.txt = %q, error = %v; the parked attempt's "+
				"uncommitted work was not durably preserved",
			preservedContent, preserveErr,
		)
	}

	// A4: the real compiled board surface names the exact exhausted work,
	// its unit (economy_output_tokens, not the turns cause) and its epoch.
	boardBody, boardErr := runBinary(
		t, swornBinary, 0, "board", "--run", "economy-output-tokens", "--journal", journalPath, "--json",
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
		grantAction.Unit != swornruntime.ParkCauseEconomyOutputTokens ||
		grantAction.ExpectedEpoch != 1 {
		t.Fatalf("board grant action = %#v pinned=%#v", grantAction, status.PinnedWork)
	}

	grantOut, grantErr := runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"grant", "--run", "economy-output-tokens", "--journal", journalPath,
		"--command", "grant-1",
		"--generation", fmt.Sprintf("%d", grantAction.ExpectedGeneration),
		"--work", grantAction.WorkID,
		"--epoch", fmt.Sprintf("%d", grantAction.ExpectedEpoch),
		"--unit", grantAction.Unit,
		"--amount", fmt.Sprintf("%d", economyOutputTokenGrant), "--config", configPath,
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
		t, swornBinary, 0, "status", "--run", "economy-output-tokens", "--journal", journalPath, "--json",
	)
	var finalStatus swornruntime.RunStatus
	if finalStatusErr != "" || json.Unmarshal([]byte(finalStatusBody), &finalStatus) != nil {
		t.Fatalf("final status body=%q stderr=%q", finalStatusBody, finalStatusErr)
	}
	if finalStatus.Park != nil || len(finalStatus.PinnedWork) != 0 {
		t.Fatalf("final status still parked/pinned: %#v", finalStatus)
	}

	// A3: recorded output-token spending survives the restart additively -
	// the exhausted first attempt's real per-request count, then the
	// granted attempt's own small real count, never reset.
	observation, err := store.ReadObservation(
		ctx, "economy-output-tokens", journal.MaxObservationAttempts, journal.MaxObservationEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	var implementationOutputTokens []int64
	for _, attempt := range observation.Attempts {
		if attempt.Responsibility != string(driver.ImplementerImplementation) {
			continue
		}
		var usage driver.UsageReceipt
		if err := json.Unmarshal(attempt.Usage, &usage); err != nil || usage.OutputTokens == nil {
			t.Fatalf("implementation attempt usage=%s error=%v", attempt.Usage, err)
		}
		implementationOutputTokens = append(implementationOutputTokens, *usage.OutputTokens)
	}
	if len(implementationOutputTokens) != 2 ||
		implementationOutputTokens[0] != spentTokens ||
		implementationOutputTokens[1] != 10 {
		t.Fatalf(
			"implementation attempts' recorded output tokens = %v, want [%d 10] "+
				"(exhausted-then-granted, never reset)",
			implementationOutputTokens, spentTokens,
		)
	}
}

// TestRealBinaryEconomyTurnBudgetParksOneTrackWhileIndependentTrackCompletes
// drives the same compiled sworn binary and real OpenAI-shaped HTTP
// provider as the turn-budget and output-token scenarios above, this time
// over economyBudgetTwoTrackPlan's two wholly independent tracks
// (S4-resumable-budget-stops A5): T1/S1 reaches the real turn budget on its
// first Implementer attempt and parks with its uncommitted code durably
// retained, exactly as the single-track scenario proves, while T2/S2's own
// Implementer never stalls and its slice reaches an independent pass in the
// very same drive - so the run only reports parked once T1 is the sole
// remaining admissible work, not because T2 was blocked by it. Only after
// an explicit grant unblocks T1 does the release reach assembly, which
// integrates both tracks' exact passed products - S1's granted content and
// S2's already-independently-passed content - into one real merged
// candidate, proving A5's "subsequent authorized continuation, independent
// verification and assembly integrate the exact passed product" over the
// real compiled binary and real target repository, not a unit-level double.
func TestRealBinaryEconomyTurnBudgetParksOneTrackWhileIndependentTrackCompletes(t *testing.T) {
	t.Parallel()
	repository := newProductRepository(t)
	planBytes, plan := economyBudgetTwoTrackPlan(t)
	provider := &economyBudgetProvider{t: t, planBytes: planBytes, turns: make(map[string]int)}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()

	root := t.TempDir()
	configBody, loaded := economyBudgetConfig(t, providerHTTP.URL)
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	const runID, release = "economy-budget-two-track", "economy-budget-two-track-release"
	manifestPath := writeManifest(
		t, root, economyBudgetManifest(
			t, runID, repository, release, loaded, economyBudgetStallTurns, 0, 2,
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
		t, swornBinary, runID, journalPath, configPath, environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("planner summary answer stdout=%q", stdout)
	}

	authorizePlan(t, journalPath, runID, plan)
	installApprovedPlan(t, repository, planBytes)

	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"resume", "--run", runID, "--journal", journalPath,
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
			"expected T1's first attempt to exhaust its real turn budget and "+
				"park while T2 finishes independently: stdout=%q stderr=%q", stdout, stderr,
		)
	}

	// A5: the exhausted track (S1) has not passed and no assembly candidate
	// exists yet, while the independent track (S2) has already reached its
	// own pass - proving T1's park neither stalled nor was gated on T2, and
	// T2's completion did not itself trigger assembly ahead of T1.
	state := readBatonState(t, repository, release)
	s1, ok1 := state.Slice("S1")
	s2, ok2 := state.Slice("S2")
	if !ok1 || !ok2 || s1.Pass != nil || s2.Pass == nil || state.Assembly.Candidate != nil {
		t.Fatalf("two-track parking isolation: S1=%#v S2=%#v assembly=%#v", s1, s2, state.Assembly)
	}

	statusBody, statusErr := runBinary(
		t, swornBinary, 0, "status", "--run", runID, "--journal", journalPath, "--json",
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

	// A1/S1-durable-unverified-checkpoints: T1's own uncommitted one.txt is
	// durably preserved under the journal's checkpoint ref, distinct from
	// T2's own already-passed two.txt content, which this test asserts
	// separately after assembly.
	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checkpoints, err := store.ListUnverifiedCheckpoints(ctx, runID)
	if err != nil || len(checkpoints) == 0 {
		t.Fatalf("no unverified checkpoints recorded in journal: %v (%d)", err, len(checkpoints))
	}
	preservedCheckpoint := checkpoints[len(checkpoints)-1]
	if preservedCheckpoint.Slice != "S1" ||
		preservedCheckpoint.DispatchWork != status.PinnedWork[0].DispatchWorkID ||
		preservedCheckpoint.Epoch != 1 || preservedCheckpoint.CheckpointRef == "" {
		t.Fatalf("checkpoint = %#v, want T1's own parked attempt", preservedCheckpoint)
	}
	runGit(t, repository, "rev-parse", "--verify", preservedCheckpoint.CheckpointRef)
	preservedContent, preserveErr := exec.Command(
		e2eGit, "-C", repository, "show", preservedCheckpoint.CheckpointRef+":one.txt",
	).Output()
	if preserveErr != nil ||
		string(preservedContent) != "interim unsubmitted content, never accepted\n" {
		t.Fatalf(
			"checkpoint one.txt = %q, error = %v; T1's uncommitted work was "+
				"not durably preserved beside T2's independent completion",
			preservedContent, preserveErr,
		)
	}

	boardBody, boardErr := runBinary(
		t, swornBinary, 0, "board", "--run", runID, "--journal", journalPath, "--json",
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
		"grant", "--run", runID, "--journal", journalPath,
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

	// A5: assembly, once T1's grant admits its completion, integrates the
	// exact product both tracks passed - S1's freshly granted one.txt and
	// S2's already-independent two.txt - into one real merged candidate.
	finalState := readBatonState(t, repository, release)
	if finalState.Assembly.Outcome != "merged" ||
		runGit(t, repository, "rev-parse", "main") == targetBefore ||
		runGit(t, repository, "show", "main:one.txt") !=
			strings.TrimSuffix(economyBudgetContent, "\n") ||
		runGit(t, repository, "show", "main:two.txt") !=
			strings.TrimSuffix(economyIndependentTrackContent, "\n") {
		t.Fatalf("final state=%#v", finalState.Assembly)
	}

	finalStatusBody, finalStatusErr := runBinary(
		t, swornBinary, 0, "status", "--run", runID, "--journal", journalPath, "--json",
	)
	var finalStatus swornruntime.RunStatus
	if finalStatusErr != "" || json.Unmarshal([]byte(finalStatusBody), &finalStatus) != nil {
		t.Fatalf("final status body=%q stderr=%q", finalStatusBody, finalStatusErr)
	}
	if finalStatus.Park != nil || len(finalStatus.PinnedWork) != 0 {
		t.Fatalf("final status still parked/pinned: %#v", finalStatus)
	}

	// A3: T1's recorded turns survive the restart additively, unaffected by
	// T2's own concurrent, unrelated attempt.
	observation, err := store.ReadObservation(
		ctx, runID, journal.MaxObservationAttempts, journal.MaxObservationEvents,
	)
	if err != nil {
		t.Fatal(err)
	}
	s1EffectPrefix := "attempt/" + strings.TrimPrefix(status.PinnedWork[0].DispatchWorkID, "sha256:") + "/"
	var s1ImplementationTurns []int64
	for _, attempt := range observation.Attempts {
		if attempt.Responsibility != string(driver.ImplementerImplementation) ||
			!strings.HasPrefix(attempt.EffectID, s1EffectPrefix) {
			continue
		}
		var usage driver.UsageReceipt
		if err := json.Unmarshal(attempt.Usage, &usage); err != nil || usage.Turns == nil {
			t.Fatalf("S1 implementation attempt usage=%s error=%v", attempt.Usage, err)
		}
		s1ImplementationTurns = append(s1ImplementationTurns, *usage.Turns)
	}
	if len(s1ImplementationTurns) != 2 ||
		s1ImplementationTurns[0] != economyBudgetStallTurns ||
		s1ImplementationTurns[1] != 2 {
		t.Fatalf(
			"S1 implementation attempts' recorded turns = %v, want [%d 2] "+
				"(exhausted-then-granted, never reset, independent of T2)",
			s1ImplementationTurns, economyBudgetStallTurns,
		)
	}
}

// TestRealBinaryEconomyGrantSurvivesCrashBeforeResumedExecution drives the
// same compiled sworn binary and real OpenAI-shaped HTTP provider as
// TestRealBinaryEconomyTurnBudgetParksGrantsAndResumes up through the real
// turn-budget park and the board's grant action, then kills the operator's
// own `sworn grant` process at the real named crash cut
// internal/runtime/service.go's Control applies for Grant
// (S4-resumable-budget-stops A3): the grant is already durably journaled
// when the process exits 86, before it can report success or start any
// resumed execution. A wholly independent `status` process must find that
// recorded grant on its own; an exact replay of the identical grant command
// - the retry an operator makes after never seeing its own confirmation -
// must be the idempotent, once-admitted grant A2 promises, never a second
// one; and the resumed attempt reached by a fresh `run` process afterward
// must still show the pre-crash and post-grant turns recorded additively,
// never reset or doubled by the crash.
func TestRealBinaryEconomyGrantSurvivesCrashBeforeResumedExecution(t *testing.T) {
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
	const runID, release = "economy-budget-grant-crash", "economy-budget-release"
	manifestPath := writeManifest(
		t, root, economyBudgetManifest(
			t, runID, repository, release, loaded, economyBudgetStallTurns, 0, 1,
		),
	)
	journalPath := filepath.Join(root, "run.sqlite")
	buildRoot := t.TempDir()
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")
	crashBinary := filepath.Join(buildRoot, "sworn-grant-crash")
	buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags)
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
		t, swornBinary, runID, journalPath, configPath, environment,
	)
	if !strings.Contains(stdout, "  state: awaiting_approval") {
		t.Fatalf("planner summary answer stdout=%q", stdout)
	}

	authorizePlan(t, journalPath, runID, plan)
	installApprovedPlan(t, repository, planBytes)

	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"resume", "--run", runID, "--journal", journalPath,
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

	boardBody, boardErr := runBinary(
		t, swornBinary, 0, "board", "--run", runID, "--journal", journalPath, "--json",
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
	if board.Run.State != "parked" || grantAction == nil {
		t.Fatalf("board grant action = %#v", grantAction)
	}

	grantArgs := []string{
		"grant", "--run", runID, "--journal", journalPath,
		"--command", "grant-1",
		"--generation", fmt.Sprintf("%d", grantAction.ExpectedGeneration),
		"--work", grantAction.WorkID,
		"--epoch", fmt.Sprintf("%d", grantAction.ExpectedEpoch),
		"--unit", grantAction.Unit,
		"--amount", fmt.Sprintf("%d", economyBudgetGrant), "--config", configPath,
	}
	crashEnvironment := map[string]string{
		"SWORN_ECONOMY_BUDGET_KEY":    economyBudgetSecret,
		"SWORN_TEST_HUMAN_TURN_CRASH": "after_grant_recorded",
	}
	runBinaryWithEnvironment(t, crashBinary, 86, crashEnvironment, grantArgs...)

	// A3: the grant was already durably journaled before the crash - a
	// wholly independent process reads the recorded grant back and reports
	// no remaining park or pinned work, before any resumed execution has
	// happened at all.
	statusBody, statusErr := runBinary(
		t, swornBinary, 0, "status", "--run", runID, "--journal", journalPath, "--json",
	)
	var status swornruntime.RunStatus
	if statusErr != "" || json.Unmarshal([]byte(statusBody), &status) != nil {
		t.Fatalf("status body=%q stderr=%q", statusBody, statusErr)
	}
	if status.State == "parked" || status.Park != nil || len(status.PinnedWork) != 0 {
		t.Fatalf("status after crashed-but-recorded grant = %#v", status)
	}

	// The same grant command, replayed by a normal process after the
	// crashed one never reported success, is an exact idempotent replay -
	// not a second admitted grant - matching A2's "duplicate replay grants
	// once".
	replayOut, replayErr := runBinaryWithEnvironment(
		t, swornBinary, 0, environment, grantArgs...,
	)
	if replayErr != "" || !strings.Contains(replayOut, "  state: running") {
		t.Fatalf("grant replay stdout=%q stderr=%q", replayOut, replayErr)
	}

	stdout, stderr = runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run", "--manifest", manifestPath, "--journal", journalPath, "--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		t.Fatalf("resumed execution after crashed grant stdout=%q stderr=%q", stdout, stderr)
	}

	finalState := readBatonState(t, repository, release)
	if finalState.Assembly.Outcome != "merged" ||
		runGit(t, repository, "rev-parse", "main") == targetBefore ||
		runGit(t, repository, "show", "main:one.txt") !=
			strings.TrimSuffix(economyBudgetContent, "\n") {
		t.Fatalf("final state=%#v", finalState.Assembly)
	}

	// A3: recorded spending survives the crash rather than resetting or
	// doubling - the pre-crash exhausted attempt and the post-grant
	// completion are both durably readable, additive, exactly as an
	// uninterrupted grant proves.
	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	observation, err := store.ReadObservation(
		ctx, runID, journal.MaxObservationAttempts, journal.MaxObservationEvents,
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
				"(exhausted-then-granted through a real crash, never reset, never doubled)",
			implementationTurns, economyBudgetStallTurns,
		)
	}
}
