//go:build linux

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

const (
	journeyOpenAISecret = "journey-openai-secret"
	journeyGeminiSecret = "journey-gemini-secret"
)

type journeyProvider struct {
	t                   *testing.T
	planBytes           []byte
	replanBytes         []byte
	captainPlanOutcomes []driver.DecisionOutcome
	repair              bool
	// slicePaths overrides the slice -> product path table this Planner's plan
	// promises. It stays nil for the original production journey, which keeps
	// using journeySlicePaths(); a journey whose plan declares different
	// slices supplies its own table so the Implementer writes exactly the
	// product path that plan's scope admits.
	slicePaths map[string]string

	mu               sync.Mutex
	plannerFactReads int
	turns            map[string]int
	families         map[string]driver.ProfileFamily
	models           map[string]string
	access           map[string]driver.WorkspaceAccess
	httpCalls        int
	submissions      int
	captainPlanCalls int
}

type journeyPrompt struct {
	InvocationID   string                `json:"invocation_id"`
	Role           driver.Role           `json:"role"`
	Workspace      driver.Workspace      `json:"workspace"`
	Responsibility driver.Responsibility `json:"responsibility"`
	Recovery       *struct {
		Kind    driver.RecoverableInputKind `json:"kind"`
		Content string                      `json:"content"`
	} `json:"recovery,omitempty"`
}

// The journey Planner behaves the way A1 and A2 require of the production
// Planner. Its first terminal reads the repository it was handed, then
// presents a summary as a human-only turn; only the responsibility resumed
// from the answered turn emits plan bytes. journeyRepositoryCanary is a fact
// that exists only inside the repository: it reaches the plan because the
// Planner read it, and journeySummaryAnswer deliberately does not contain it,
// so a Planner that asked a person for a repository-discoverable fact instead
// of reading it would fail this journey.
const (
	journeyRepositoryCanary = "REPO-FACT-CANARY-4c1f"
	journeySummaryQuestion  = "Summary of the result, scope, acceptance, " +
		"evidence, inputs, and limits I intend to promise. Confirm or correct."
	journeySummaryAnswer = "Confirmed; plan exactly that."
)

type journeyGeminiContent struct {
	Role  string `json:"role"`
	Parts []struct {
		Text         *string `json:"text"`
		FunctionCall *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"functionCall"`
		FunctionResponse *struct {
			Name     string `json:"name"`
			Response struct {
				Result string `json:"result"`
			} `json:"response"`
		} `json:"functionResponse"`
	} `json:"parts"`
}

func (provider *journeyProvider) serve(
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

	family := driver.ProfileOpenAIHTTP
	promptBody, model, err := openAIJourneyPrompt(request, body)
	toolResults := openAIJourneyToolResults(body)
	switch {
	case strings.HasPrefix(request.URL.Path, "/openai/"):
		if request.Header.Get("Authorization") !=
			"Bearer "+journeyOpenAISecret {
			err = fmt.Errorf("OpenAI credential mismatch")
		}
	case strings.HasPrefix(request.URL.Path, "/gemini/"):
		family = driver.ProfileGemini
		promptBody, model, err = geminiJourneyPrompt(request, body)
		if request.Header.Get("x-goog-api-key") != journeyGeminiSecret {
			err = fmt.Errorf("Gemini credential mismatch")
		}
	default:
		err = fmt.Errorf("unexpected provider path %q", request.URL.Path)
	}
	if err != nil {
		provider.t.Errorf("journey provider request: %v", err)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	var prompt journeyPrompt
	if err := json.Unmarshal([]byte(promptBody), &prompt); err != nil ||
		prompt.InvocationID == "" || prompt.Responsibility == "" {
		provider.t.Errorf("journey prompt = %q, error=%v", promptBody, err)
		http.Error(writer, "invalid prompt", http.StatusBadRequest)
		return
	}
	expectedFamily, expectedModel := journeyRoleSelection(prompt.Role)
	if family != expectedFamily || model != expectedModel {
		provider.t.Errorf(
			"journey selection role=%s family=%s model=%s, want %s/%s",
			prompt.Role,
			family,
			model,
			expectedFamily,
			expectedModel,
		)
		http.Error(writer, "selection mismatch", http.StatusBadRequest)
		return
	}

	provider.mu.Lock()
	provider.httpCalls++
	provider.turns[prompt.InvocationID]++
	turn := provider.turns[prompt.InvocationID]
	if prior, exists := provider.families[prompt.InvocationID]; exists &&
		prior != family {
		provider.t.Errorf(
			"invocation %s changed family from %s to %s",
			prompt.InvocationID,
			prior,
			family,
		)
	}
	provider.families[prompt.InvocationID] = family
	provider.models[prompt.InvocationID] = model
	provider.access[prompt.InvocationID] = prompt.Workspace.Access
	provider.mu.Unlock()

	toolName := "sworn_submit"
	arguments, err := provider.submissionArguments(prompt)
	if prompt.Responsibility == driver.PlannerProposal {
		toolName, arguments, err = provider.plannerResponse(
			prompt, turn, toolResults,
		)
	} else if prompt.Responsibility == driver.ImplementerImplementation &&
		turn == 1 {
		parts := strings.Split(prompt.InvocationID, "/")
		if len(parts) != 6 {
			err = fmt.Errorf("invalid implementation invocation")
		} else {
			pathValue, ok := provider.paths()[parts[1]]
			if !ok {
				err = fmt.Errorf("unknown implementation slice %q", parts[1])
			} else {
				toolName = "Write"
				content := parts[1] + " production journey\n"
				if provider.repair && parts[1] == "A1" {
					content = parts[1] + " production journey attempt " +
						parts[3] + "\n"
				}
				arguments = map[string]any{
					"path":    "/workspace/" + pathValue,
					"content": content,
				}
			}
		}
	} else if prompt.Responsibility != driver.ImplementerImplementation &&
		turn != 1 {
		err = fmt.Errorf(
			"unexpected extra turn %d for %s",
			turn,
			prompt.Responsibility,
		)
	} else if prompt.Responsibility == driver.ImplementerImplementation &&
		turn != 2 {
		err = fmt.Errorf("implementation turn = %d", turn)
	}
	if err != nil {
		provider.t.Errorf("journey response: %v", err)
		http.Error(writer, "invalid response", http.StatusInternalServerError)
		return
	}
	if toolName == "sworn_submit" {
		provider.mu.Lock()
		provider.submissions++
		provider.mu.Unlock()
	}
	callID := journeyCallID(prompt.InvocationID, turn)
	writer.Header().Set("Content-Type", "application/json")
	if family == driver.ProfileOpenAIHTTP {
		argumentBody, _ := json.Marshal(arguments)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []any{map[string]any{
						"id": callID, "type": "function",
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
		return
	}
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{
				"role": "model",
				"parts": []any{map[string]any{
					"functionCall": map[string]any{
						"id": callID, "name": toolName, "args": arguments,
					},
				}},
			},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount":     7,
			"candidatesTokenCount": 5,
			"totalTokenCount":      12,
			// A7: the native usage vocabulary reports reasoning, and the
			// production journey's receipt assertion verifies it lands on the
			// usage receipt summed across every Gemini turn.
			"thoughtsTokenCount": 3,
		},
	})
}

func openAIJourneyPrompt(
	request *http.Request,
	body []byte,
) (string, string, error) {
	if request.Method != http.MethodPost ||
		request.URL.Path != "/openai/v1/chat/completions" {
		return "", "", fmt.Errorf("OpenAI path %s %s", request.Method, request.URL.Path)
	}
	var value struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &value); err != nil ||
		len(value.Messages) == 0 {
		return "", "", fmt.Errorf("invalid OpenAI body")
	}
	for index := len(value.Messages) - 1; index >= 0; index-- {
		if value.Messages[index].Role != "user" {
			continue
		}
		var prompt string
		if err := json.Unmarshal(
			value.Messages[index].Content,
			&prompt,
		); err != nil {
			return "", "", err
		}
		return prompt, value.Model, nil
	}
	return "", "", fmt.Errorf("OpenAI body omitted user prompt")
}

func geminiJourneyPrompt(
	request *http.Request,
	body []byte,
) (string, string, error) {
	const prefix = "/gemini/v1beta/models/"
	const suffix = ":generateContent"
	if request.Method != http.MethodPost ||
		!strings.HasPrefix(request.URL.Path, prefix) ||
		!strings.HasSuffix(request.URL.Path, suffix) {
		return "", "", fmt.Errorf("Gemini path %s %s", request.Method, request.URL.Path)
	}
	model := strings.TrimSuffix(
		strings.TrimPrefix(request.URL.Path, prefix),
		suffix,
	)
	var value struct {
		Contents []journeyGeminiContent `json:"contents"`
	}
	if err := json.Unmarshal(body, &value); err != nil ||
		len(value.Contents) == 0 {
		return "", "", fmt.Errorf("invalid Gemini body")
	}
	for content := len(value.Contents) - 1; content >= 0; content-- {
		if value.Contents[content].Role != "user" {
			continue
		}
		for part := len(value.Contents[content].Parts) - 1; part >= 0; part-- {
			if value.Contents[content].Parts[part].Text != nil {
				if err := validateGeminiJourneyContinuation(
					value.Contents,
					content,
					part,
				); err != nil {
					return "", "", err
				}
				return *value.Contents[content].Parts[part].Text, model, nil
			}
		}
	}
	return "", "", fmt.Errorf("Gemini body omitted user prompt")
}

func validateGeminiJourneyContinuation(
	contents []journeyGeminiContent,
	promptContent int,
	promptPart int,
) error {
	resume := contents[promptContent]
	if promptContent == 0 {
		return nil
	}
	if promptContent != 2 || promptPart != 1 ||
		len(contents) < 3 ||
		contents[0].Role != "user" ||
		len(contents[0].Parts) != 1 ||
		contents[0].Parts[0].Text == nil ||
		contents[1].Role != "model" ||
		len(contents[1].Parts) != 1 ||
		contents[1].Parts[0].FunctionCall == nil ||
		contents[1].Parts[0].FunctionCall.Name != "sworn_submit" ||
		resume.Role != "user" ||
		len(resume.Parts) != 2 ||
		resume.Parts[0].FunctionResponse == nil {
		return fmt.Errorf("invalid Gemini continuation prefix")
	}
	call := contents[1].Parts[0].FunctionCall
	result := resume.Parts[0].FunctionResponse
	if call.ID == "" ||
		result.Name != call.Name ||
		result.Response.Result != "accepted" {
		return fmt.Errorf("invalid Gemini accepted submission result")
	}
	return nil
}

func journeyRoleSelection(
	role driver.Role,
) (driver.ProfileFamily, string) {
	switch role {
	case driver.RolePlanner:
		return driver.ProfileOpenAIHTTP, "journey-planner"
	case driver.RoleImplementer:
		return driver.ProfileGemini, "journey-implementer"
	case driver.RoleCaptain:
		return driver.ProfileOpenAIHTTP, "journey-captain"
	case driver.RoleVerifier:
		return driver.ProfileGemini, "journey-verifier"
	default:
		return "", ""
	}
}

// plannerResponse is the journey Planner, and it behaves the way A1 and A2
// require. Turn 1 reads the repository fact through the workspace tool
// surface. Turn 2 refuses to continue unless that read actually returned the
// fact, then presents its summary as a human-only turn. Plan bytes are only
// emitted by the turn resumed from the operator's answer, and that answer must
// not be where the repository fact came from.
func (provider *journeyProvider) plannerResponse(
	prompt journeyPrompt,
	turn int,
	toolResults []string,
) (string, map[string]any, error) {
	switch {
	case turn == 1:
		if prompt.Recovery != nil {
			return "", nil, fmt.Errorf("planner turn 1 already resumed")
		}
		return "Read", map[string]any{
			"path": prompt.Workspace.Path + "/REPOSITORY-FACT.md",
		}, nil
	case turn == 2:
		read := ""
		if len(toolResults) > 0 {
			read = toolResults[len(toolResults)-1]
		}
		if !strings.Contains(read, journeyRepositoryCanary) {
			return "", nil, fmt.Errorf(
				"planner read did not return the repository fact: %q", read,
			)
		}
		provider.mu.Lock()
		provider.plannerFactReads++
		provider.mu.Unlock()
		return "sworn_yield", map[string]any{"yield": map[string]any{
			"schema_version": driver.YieldSchemaVersion,
			"invocation_id":  prompt.InvocationID,
			"kind":           string(driver.YieldHumanConfirmation),
			"message":        journeySummaryQuestion,
		}}, nil
	default:
		if prompt.Recovery == nil ||
			prompt.Recovery.Kind != driver.RecoverableInputAnswer ||
			prompt.Recovery.Content != journeySummaryAnswer {
			return "", nil, fmt.Errorf(
				"planner resume turn=%d recovery=%#v", turn, prompt.Recovery,
			)
		}
		if strings.Contains(
			prompt.Recovery.Content, journeyRepositoryCanary,
		) {
			return "", nil, fmt.Errorf(
				"the operator answer supplied a repository-discoverable fact",
			)
		}
		arguments, err := provider.submissionArguments(prompt)
		return "sworn_submit", arguments, err
	}
}

// openAIJourneyToolResults returns, in order, the tool results already visible
// to the model in this OpenAI conversation.
func openAIJourneyToolResults(body []byte) []string {
	var value struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &value) != nil {
		return nil
	}
	results := make([]string, 0, len(value.Messages))
	for _, message := range value.Messages {
		if message.Role != "tool" {
			continue
		}
		var content string
		if json.Unmarshal(message.Content, &content) != nil {
			continue
		}
		results = append(results, content)
	}
	return results
}

func (provider *journeyProvider) submissionArguments(
	prompt journeyPrompt,
) (map[string]any, error) {
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   prompt.InvocationID,
		Responsibility: prompt.Responsibility,
		Summary:        "Deterministic production journey " + string(prompt.Responsibility) + ".",
		Detail:         "Sealed through the common configured production driver registry.",
	}
	var err error
	switch prompt.Responsibility {
	case driver.PlannerProposal:
		planBytes := provider.planBytes
		if provider.replanBytes != nil && strings.Contains(prompt.InvocationID, "/release-") {
			planBytes = provider.replanBytes
		}
		submission.Plan, err = driver.NewPlanBytes(planBytes)
	case driver.ImplementerDesign:
	case driver.CaptainReview:
		submission.Decision, err = driver.NewDecision(driver.DecisionProceed)
	case driver.CaptainPlanReview:
		outcome := driver.DecisionProceed
		provider.mu.Lock()
		if provider.captainPlanCalls < len(provider.captainPlanOutcomes) {
			outcome = provider.captainPlanOutcomes[provider.captainPlanCalls]
		}
		provider.captainPlanCalls++
		provider.mu.Unlock()
		submission.Decision, err = driver.NewDecision(outcome)
	case driver.ImplementerImplementation:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("deterministic production implementation checks\n"),
		)
	case driver.WorkVerification:
		checks := "fresh deterministic production work checks\n"
		parts := strings.Split(prompt.InvocationID, "/")
		if provider.repair && len(parts) == 6 && parts[1] == "A1" {
			checks = "fresh deterministic production work checks attempt " +
				parts[3] + "\n"
		}
		submission.Checks, err = driver.NewCheckBytes(
			[]byte(checks),
		)
		if err == nil {
			decision := driver.DecisionPass
			if provider.repair && len(parts) == 6 &&
				parts[1] == "A1" && parts[3] == "1" {
				decision = driver.DecisionFail
			}
			submission.Decision, err = driver.NewDecision(decision)
		}
	case driver.AssemblyVerification:
		submission.Checks, err = driver.NewCheckBytes(
			[]byte("fresh deterministic production assembly checks\n"),
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

func journeyCallID(invocationID string, turn int) string {
	sum := sha256.Sum256([]byte(invocationID))
	return fmt.Sprintf("call-%s-%d", hex.EncodeToString(sum[:6]), turn)
}

// paths is the slice -> product path table this provider's Planner promised.
func (provider *journeyProvider) paths() map[string]string {
	if provider.slicePaths != nil {
		return provider.slicePaths
	}
	return journeySlicePaths()
}

func journeySlicePaths() map[string]string {
	return map[string]string{
		"A1": "one-a.txt",
		"A2": "one-b.txt",
		"B1": "two.txt",
		"C1": "three.txt",
	}
}

func productionJourneyPlan(
	t *testing.T,
	repository string,
) ([]byte, baton.Plan) {
	t.Helper()
	slice := func(id string) baton.Slice {
		return baton.Slice{
			ID:      id,
			Outcome: "Deliver deterministic production slice " + id + ".",
			Scope: baton.Scope{
				Include: []string{journeySlicePaths()[id]},
				Exclude: []string{},
			},
			Acceptance: []baton.Criterion{{
				ID:   "A-" + id,
				Text: id + " is present in the exact product tree.",
			}},
			Checks:      []string{"check " + id},
			Constraints: []string{"deterministic local provider"},
			DependsOn:   []string{},
			Consumes:    []string{},
		}
	}
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       "production-journey-release",
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    "acme-repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "operator://production-journey-release/1",
		Tracks: []baton.Track{
			{
				ID: "T1", DependsOn: []string{},
				Slices: []baton.Slice{slice("A1"), slice("A2")},
			},
			{
				ID: "T2", DependsOn: []string{},
				Slices: []baton.Slice{slice("B1")},
			},
			{
				ID: "T3", DependsOn: []string{"T1"},
				Slices: []baton.Slice{slice("C1")},
			},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nDeterministic production journey for " + repository +
			".\nOwned surface read from the repository: " +
			journeyRepositoryCanary + ".\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func productionJourneyConfig(
	t *testing.T,
	providerURL string,
) ([]byte, driver.LoadedDriverConfig) {
	t.Helper()
	geminiCredential := "gemini-env"
	openAICredential := "openai-env"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{
			{
				Key:       geminiCredential,
				Kind:      driver.CredentialEnvironment,
				Reference: "SWORN_JOURNEY_GEMINI_KEY",
			},
			{
				Key:       openAICredential,
				Kind:      driver.CredentialEnvironment,
				Reference: "SWORN_JOURNEY_OPENAI_KEY",
			},
		},
		Adapters: []driver.DriverAdapterConfig{
			{Gemini: &driver.HTTPProfileConfig{
				Key: "a-gemini", ID: "sworn.journey.gemini", Version: "1.0.0",
				Endpoint:         providerURL + "/gemini",
				CredentialHeader: "x-goog-api-key",
				CredentialPrefix: "",
				CredentialRefs:   []string{geminiCredential},
				ResponseBytes:    driver.MaxProviderResponseBytes,
			}},
			{OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key: "a-openai", ID: "sworn.journey.openai", Version: "1.0.0",
					Endpoint:         providerURL + "/openai/v1/chat/completions",
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{openAICredential},
					ResponseBytes:    driver.MaxProviderResponseBytes,
				},
				API: driver.OpenAIChatCompletionsAPI,
			}},
		},
		Profiles: []driver.DriverProfile{
			{
				Key: "gemini", Adapter: "a-gemini",
				Network:          driver.NetworkRequired,
				CredentialSource: &geminiCredential,
				CertificationModels: []string{
					"journey-implementer",
					"journey-verifier",
				},
			},
			{
				Key: "openai", Adapter: "a-openai",
				Network:          driver.NetworkRequired,
				CredentialSource: &openAICredential,
				CertificationModels: []string{
					"journey-captain",
					"journey-planner",
				},
			},
		},
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

func productionJourneyManifest(
	t *testing.T,
	repository string,
	config driver.LoadedDriverConfig,
) []byte {
	t.Helper()
	manifest := swornruntime.Manifest{
		GitIdentity:       gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
		SchemaVersion:     swornruntime.ManifestVersion,
		RunID:             "production-journey",
		Repository:        repository,
		Release:           "production-journey-release",
		TargetRef:         "refs/heads/main",
		Intent:            "Complete the deterministic configured production journey.",
		MaxParallelTracks: 3,
		Authority: swornruntime.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		DriverConfigDigest: config.ConfigurationDigest(),
		Roles: driver.RoleSelections{
			Planner: driver.RoleSelection{
				Profile: "openai", Model: "journey-planner",
			},
			Implementer: driver.RoleSelection{
				Profile: "gemini", Model: "journey-implementer",
			},
			Captain: driver.RoleSelection{
				Profile: "openai", Model: "journey-captain",
			},
			Verifier: driver.RoleSelection{
				Profile: "gemini", Model: "journey-verifier",
			},
		},
		Automation: &swornruntime.AutomationSelections{
			Recovery: driver.RoleSelection{Profile: "openai", Model: "journey-planner"},
		},
		Limits: driver.Limits{
			TimeoutMillis: 30_000,
			OutputBytes:   65_536,
		},
		Scripts: nil,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	parsed, err := swornruntime.ParseManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Driver != nil || parsed.Scripts != nil ||
		parsed.DriverConfigDigest != config.ConfigurationDigest() {
		t.Fatalf("production manifest = %#v", parsed)
	}
	return body
}

func TestConfiguredProductionJourneyRestartsAndIntegratesExactAssembly(
	t *testing.T,
) {
	t.Parallel()
	runConfiguredProductionJourney(t, false)
}

func TestConfiguredProductionJourneyRepairsVerifierFailWithFreshPass(
	t *testing.T,
) {
	t.Parallel()
	runConfiguredProductionJourney(t, true)
}

// openPlannerSummaryAttention reads the board through the real binary and
// returns the one open human-only Planner turn.
func openPlannerSummaryAttention(
	t *testing.T,
	swornBinary, runID, journalPath string,
) cockpit.AttentionView {
	t.Helper()
	body, stderr := runBinary(
		t, swornBinary, 0,
		"board", "--run", runID, "--journal", journalPath, "--json",
	)
	var board cockpit.Snapshot
	if stderr != "" || json.Unmarshal([]byte(body), &board) != nil {
		t.Fatalf("board body=%q stderr=%q", body, stderr)
	}
	var found []cockpit.AttentionView
	for _, attention := range board.Runtime.Attentions {
		if attention.State != "open" || attention.HumanTurn == nil {
			continue
		}
		found = append(found, attention)
	}
	if len(found) != 1 {
		t.Fatalf("open human turns = %#v", board.Runtime.Attentions)
	}
	attention := found[0]
	if attention.Generation != 1 ||
		attention.Question != journeySummaryQuestion ||
		attention.HumanTurn.Kind != string(driver.YieldHumanConfirmation) ||
		attention.HumanTurn.Responsibility !=
			string(driver.PlannerProposal) ||
		attention.HumanTurn.OpenGeneration != 1 {
		t.Fatalf("planner summary attention = %#v", attention)
	}
	if strings.Contains(attention.Question, journeyRepositoryCanary) {
		t.Fatal("the Planner asked a person for a repository-discoverable fact")
	}
	return attention
}

func runConfiguredProductionJourney(t *testing.T, repair bool) {
	t.Helper()
	repository := newProductRepository(t)
	planBytes, plan := productionJourneyPlan(t, repository)
	provider := &journeyProvider{
		t: t, planBytes: planBytes, repair: repair,
		turns:    make(map[string]int),
		families: make(map[string]driver.ProfileFamily),
		models:   make(map[string]string),
		access:   make(map[string]driver.WorkspaceAccess),
	}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()

	configBody, loaded := productionJourneyConfig(t, providerHTTP.URL)
	root := t.TempDir()
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestBody := productionJourneyManifest(t, repository, loaded)
	manifestPath := writeManifest(t, root, manifestBody)
	journalPath := filepath.Join(root, "run.sqlite")
	swornBinary := filepath.Join(root, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")
	targetBefore := runGit(t, repository, "rev-parse", "main")
	stdout, stderr := runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		map[string]string{
			"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
			"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
		},
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
		"--config", configPath,
	)
	// A2: the Planner presented its summary and stopped; no plan exists yet.
	if stderr != "" || !strings.Contains(stdout, "  state: parked") ||
		runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("production start stdout=%q stderr=%q", stdout, stderr)
	}
	attention := openPlannerSummaryAttention(
		t, swornBinary, "production-journey", journalPath,
	)
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t,
		swornBinary,
		0,
		map[string]string{
			"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
			"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
		},
		180*time.Second,
		"answer",
		"--run", "production-journey",
		"--journal", journalPath,
		"--attention", attention.ID,
		"--generation", "1",
		"--answer", journeySummaryAnswer,
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: awaiting_approval") ||
		runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("production summary answer stdout=%q stderr=%q", stdout, stderr)
	}
	// A1: the fact the plan promises came out of the repository, and the
	// operator's answer is not where it came from.
	provider.mu.Lock()
	factReads := provider.plannerFactReads
	provider.mu.Unlock()
	if factReads != 1 ||
		!bytes.Contains(planBytes, []byte(journeyRepositoryCanary)) ||
		strings.Contains(journeySummaryAnswer, journeyRepositoryCanary) {
		t.Fatalf("planner repository reads = %d", factReads)
	}
	beforeRestart, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeRestart, configBody) ||
		driver.Digest(beforeRestart) != loaded.ConfigurationDigest() {
		t.Fatal("production config identity changed before restart")
	}

	authorizePlan(t, journalPath, "production-journey", plan)
	installApprovedPlan(t, repository, planBytes)
	stdout, stderr = runBinaryWithEnvironmentTimeout(
		t,
		swornBinary,
		0,
		map[string]string{
			"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
			"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
		},
		180*time.Second,
		"resume",
		"--run", "production-journey",
		"--journal", journalPath,
		"--command", "resume-1",
		"--generation", "0",
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: complete") {
		store, _ := journal.OpenReadOnly(context.Background(), journalPath)
		snapshot, _ := store.Snapshot(context.Background(), "production-journey")
		_ = store.Close()
		var failures []string
		var effects []string
		for _, effect := range snapshot.Effects {
			effects = append(
				effects,
				effect.Kind+":"+string(effect.State)+":"+
					effect.ErrorCode+":"+effect.ID,
			)
			if effect.State == journal.OperationalFailed ||
				effect.State == journal.Uncertain ||
				effect.State == journal.Claimed {
				failures = append(
					failures,
					effect.Kind+":"+effect.ErrorCode+":"+effect.ID,
				)
			}
		}
		authority := readBatonState(
			t,
			repository,
			"production-journey-release",
		)
		var slices []string
		for _, slice := range authority.Slices {
			slices = append(
				slices,
				fmt.Sprintf(
					"%s:a%d:%s:%s:%s:history%d",
					slice.Location.Slice.ID,
					slice.Attempt,
					slice.Stage,
					slice.Status,
					slice.NextRole,
					len(slice.History.Entries),
				),
			)
		}
		t.Fatalf(
			"production resume stdout=%q stderr=%q failures=%v slices=%v effects=%v",
			stdout,
			stderr,
			failures,
			slices,
			effects,
		)
	}
	afterRestart, err := os.ReadFile(configPath)
	if err != nil || !bytes.Equal(afterRestart, beforeRestart) {
		t.Fatalf("production restart config changed: %v", err)
	}
	for _, secret := range []string{
		journeyOpenAISecret,
		journeyGeminiSecret,
	} {
		if strings.Contains(stdout, secret) || strings.Contains(stderr, secret) {
			t.Fatal("provider credential escaped command output")
		}
	}

	state := readBatonState(t, repository, "production-journey-release")
	if len(state.Tracks) != 3 ||
		state.Assembly.Outcome != "merged" ||
		state.Assembly.Candidate == nil ||
		state.Assembly.Pass == nil ||
		state.Assembly.ResultCommit == "" {
		t.Fatalf("production journey state = %#v", state.Assembly)
	}
	if len(state.Tracks[0].Slices) != 2 ||
		len(state.Tracks[2].DependsOn) != 1 ||
		state.Tracks[2].DependsOn[0] != "T1" {
		t.Fatalf("production journey topology = %#v", state.Tracks)
	}
	target := runGit(t, repository, "rev-parse", "main")
	assemblyCandidate := *state.Assembly.Candidate.Receipt.Candidate
	targetTree := runGit(t, repository, "rev-parse", target+"^{tree}")
	assemblyTree := runGit(
		t,
		repository,
		"rev-parse",
		assemblyCandidate+"^{tree}",
	)
	resultTree := runGit(
		t,
		repository,
		"rev-parse",
		state.Assembly.ResultCommit+"^{tree}",
	)
	if target != state.Assembly.ResultCommit ||
		targetTree != assemblyTree ||
		targetTree != resultTree {
		t.Fatalf(
			"target=%s/%s assembly=%s/%s result=%s/%s",
			target,
			targetTree,
			assemblyCandidate,
			assemblyTree,
			state.Assembly.ResultCommit,
			resultTree,
		)
	}
	for sliceID, pathValue := range journeySlicePaths() {
		want := sliceID + " production journey"
		if repair && sliceID == "A1" {
			want = "A1 production journey attempt 2"
		}
		if got := runGit(t, repository, "show", "main:"+pathValue); got != want {
			t.Fatalf("%s product = %q, want %q", sliceID, got, want)
		}
	}
	if repair {
		assertProductionRepairState(t, state)
		// A4's direct-repair clause: the failed attempt and the repair are
		// each recorded exactly once, and the merged product is the repaired
		// attempt's, not the failed one's.
		assertNoDuplicateVerdicts(t, state, "A1")
	}

	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), "production-journey")
	observation, observationErr := store.ReadObservation(
		context.Background(),
		"production-journey",
		journal.MaxObservationAttempts,
		journal.MaxObservationEvents,
	)
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	if observationErr != nil {
		t.Fatal(observationErr)
	}
	driverEffects := make(map[string]struct{})
	contexts := make(map[string]struct{})
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" {
			continue
		}
		if effect.State != journal.Succeeded {
			t.Fatalf("production driver effect = %#v", effect)
		}
		if _, duplicate := driverEffects[effect.ID]; duplicate {
			t.Fatalf("duplicate production driver effect %s", effect.ID)
		}
		driverEffects[effect.ID] = struct{}{}
		for _, command := range snapshot.Commands {
			if command.ReplayKey != effect.ReplayKey {
				continue
			}
			var payload struct {
				SchemaVersion       string `json:"schema_version"`
				ResumeRequestDigest string `json:"resume_request_digest"`
				Context             struct {
					SchemaVersion   string                 `json:"schema_version"`
					InvocationID    string                 `json:"invocation_id"`
					Responsibility  driver.Responsibility  `json:"responsibility"`
					WorkspaceAccess driver.WorkspaceAccess `json:"workspace_access"`
					Track           string                 `json:"track"`
					Slice           string                 `json:"slice"`
					PreparedBase    string                 `json:"prepared_base"`
					DesignReceipt   *struct {
						OID string `json:"oid"`
					} `json:"design_receipt"`
					Attempt int64 `json:"attempt"`
					Epoch   int64 `json:"epoch"`
					Try     int64 `json:"try"`
				} `json:"context"`
			}
			maxAttempt := int64(1)
			if repair {
				maxAttempt = 2
			}
			if json.Unmarshal(command.Payload, &payload) != nil ||
				payload.SchemaVersion != "sworn.production-dispatch/v2" ||
				payload.Context.SchemaVersion != "sworn.work-context/v2" ||
				payload.Context.InvocationID == "" ||
				payload.Context.Attempt < 1 ||
				payload.Context.Attempt > maxAttempt ||
				payload.Context.Epoch != 1 ||
				payload.Context.Try != 1 {
				t.Fatalf("production dispatch command = %s", command.Payload)
			}
			switch payload.Context.Responsibility {
			case driver.ImplementerImplementation:
				if payload.Context.Track == "" ||
					payload.Context.WorkspaceAccess != driver.ReadWrite {
					t.Fatalf(
						"implementation continuation command = %s",
						command.Payload,
					)
				}
				freshRepair := repair &&
					payload.Context.Slice == "A1" &&
					payload.Context.Attempt == 2
				if freshRepair &&
					(payload.Context.DesignReceipt != nil ||
						payload.ResumeRequestDigest != "") {
					t.Fatalf(
						"fresh repair command = %s",
						command.Payload,
					)
				}
				if !freshRepair &&
					(payload.Context.DesignReceipt == nil ||
						payload.Context.DesignReceipt.OID == "" ||
						payload.ResumeRequestDigest == "") {
					t.Fatalf(
						"resumable implementation command = %s",
						command.Payload,
					)
				}
			case driver.WorkVerification:
				if payload.ResumeRequestDigest == "" ||
					payload.Context.DesignReceipt != nil {
					t.Fatalf(
						"work-verifier continuation command = %s",
						command.Payload,
					)
				}
			default:
				if payload.ResumeRequestDigest != "" ||
					payload.Context.DesignReceipt != nil {
					t.Fatalf(
						"one-shot dispatch command = %s",
						command.Payload,
					)
				}
			}
			if _, duplicate := contexts[payload.Context.InvocationID]; duplicate {
				t.Fatalf(
					"duplicate production invocation %s",
					payload.Context.InvocationID,
				)
			}
			contexts[payload.Context.InvocationID] = struct{}{}
			if payload.Context.Responsibility == driver.WorkVerification ||
				payload.Context.Responsibility == driver.AssemblyVerification {
				if payload.Context.WorkspaceAccess != driver.ReadOnly {
					t.Fatalf(
						"verifier access = %s",
						payload.Context.WorkspaceAccess,
					)
				}
			}
		}
	}
	// The Planner spends two extra provider turns before its submission: one
	// to read the repository, one to present its summary as a human-only
	// turn.
	wantInvocations := 18
	wantHTTPCalls := 24
	if repair {
		wantInvocations = 20
		wantHTTPCalls = 27
	}
	if len(driverEffects) != wantInvocations ||
		len(contexts) != wantInvocations {
		t.Fatalf(
			"production dispatch cardinality effects=%d contexts=%d",
			len(driverEffects),
			len(contexts),
		)
	}
	var inputTokens, outputTokens, reasoningTokens int64
	reasoningReported := 0
	if len(observation.Attempts) != wantInvocations {
		t.Fatalf(
			"production attempts=%d want=%d",
			len(observation.Attempts),
			wantInvocations,
		)
	}
	for _, attempt := range observation.Attempts {
		var usage driver.UsageReceipt
		if json.Unmarshal(attempt.Usage, &usage) != nil ||
			usage.TokenStatus != driver.UsageReported ||
			usage.InputTokens == nil ||
			usage.OutputTokens == nil ||
			usage.CostStatus != driver.UsageUnavailable ||
			usage.CostMicroUnits != nil ||
			usage.Currency != nil ||
			usage.Source != nil {
			t.Fatalf("production usage = %s", attempt.Usage)
		}
		inputTokens += *usage.InputTokens
		outputTokens += *usage.OutputTokens
		if usage.ReasoningTokens != nil {
			reasoningReported++
			reasoningTokens += *usage.ReasoningTokens
		}
	}
	wantInputTokens, wantOutputTokens := int64(168), int64(120)
	wantReasoningTokens := int64(51)
	wantReasoningReported := 13
	if repair {
		wantInputTokens, wantOutputTokens = 189, 135
		wantReasoningTokens = 60
		wantReasoningReported = 15
	}
	if inputTokens != wantInputTokens ||
		outputTokens != wantOutputTokens ||
		reasoningTokens != wantReasoningTokens ||
		reasoningReported != wantReasoningReported {
		t.Fatalf(
			"production usage input=%d output=%d reasoning=%d/%d, want %d/%d reasoning=%d/%d",
			inputTokens,
			outputTokens,
			reasoningTokens,
			reasoningReported,
			wantInputTokens,
			wantOutputTokens,
			wantReasoningTokens,
			wantReasoningReported,
		)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.turns) != wantInvocations ||
		provider.submissions != wantInvocations ||
		provider.httpCalls != wantHTTPCalls {
		t.Fatalf(
			"provider turns=%d submissions=%d calls=%d",
			len(provider.turns),
			provider.submissions,
			provider.httpCalls,
		)
	}
	familySet := make(map[driver.ProfileFamily]struct{})
	modelSet := make(map[string]struct{})
	for invocationID, family := range provider.families {
		familySet[family] = struct{}{}
		modelSet[provider.models[invocationID]] = struct{}{}
		if strings.Contains(invocationID, "/work_verification/") ||
			strings.Contains(invocationID, "/assembly_verification/") {
			if provider.access[invocationID] != driver.ReadOnly {
				t.Fatalf(
					"provider observed verifier access=%s for %s",
					provider.access[invocationID],
					invocationID,
				)
			}
		}
	}
	if repair {
		var verifierInvocations []string
		for invocationID := range provider.families {
			if strings.Contains(
				invocationID,
				"/A1/work_verification/",
			) {
				verifierInvocations = append(
					verifierInvocations,
					invocationID,
				)
			}
		}
		sort.Strings(verifierInvocations)
		if len(verifierInvocations) != 2 ||
			verifierInvocations[0] == verifierInvocations[1] ||
			!strings.Contains(verifierInvocations[0], "/1/1/1") ||
			!strings.Contains(verifierInvocations[1], "/2/1/1") {
			t.Fatalf(
				"repair verifier invocations = %v",
				verifierInvocations,
			)
		}
	}
	if len(familySet) != 2 || len(modelSet) != 4 {
		var models []string
		for model := range modelSet {
			models = append(models, model)
		}
		sort.Strings(models)
		t.Fatalf("journey families=%v models=%v", familySet, models)
	}
}

func assertProductionRepairState(t *testing.T, state baton.State) {
	t.Helper()
	slice, ok := state.Slice("A1")
	if !ok {
		t.Fatal("repair slice A1 is absent")
	}
	candidates := make(map[int64]baton.ReceiptEntry)
	verdicts := make(map[int64]baton.ReceiptEntry)
	for _, entry := range slice.History.Entries {
		if entry.Receipt.Attempt == nil {
			continue
		}
		attempt := *entry.Receipt.Attempt
		switch {
		case entry.Receipt.Role == "implementer" &&
			entry.Receipt.Result == "candidate":
			candidates[attempt] = entry
		case entry.Receipt.Role == "verifier" &&
			(entry.Receipt.Result == "fail" ||
				entry.Receipt.Result == "pass"):
			verdicts[attempt] = entry
		}
	}
	failedCandidate, firstCandidateOK := candidates[1]
	repairedCandidate, secondCandidateOK := candidates[2]
	failedVerdict, failedOK := verdicts[1]
	passedVerdict, passedOK := verdicts[2]
	if !firstCandidateOK || !secondCandidateOK || !failedOK || !passedOK ||
		failedVerdict.Receipt.Result != "fail" ||
		passedVerdict.Receipt.Result != "pass" ||
		failedVerdict.Receipt.Binds != failedCandidate.OID ||
		passedVerdict.Receipt.Binds != repairedCandidate.OID ||
		failedCandidate.Receipt.Candidate == nil ||
		repairedCandidate.Receipt.Candidate == nil ||
		*failedCandidate.Receipt.Candidate ==
			*repairedCandidate.Receipt.Candidate ||
		failedCandidate.Receipt.ProductTree == nil ||
		repairedCandidate.Receipt.ProductTree == nil ||
		*failedCandidate.Receipt.ProductTree ==
			*repairedCandidate.Receipt.ProductTree ||
		failedVerdict.Receipt.Checks == nil ||
		passedVerdict.Receipt.Checks == nil ||
		*failedVerdict.Receipt.Checks ==
			*passedVerdict.Receipt.Checks ||
		slice.Candidate == nil ||
		slice.Candidate.OID != repairedCandidate.OID ||
		slice.Pass == nil ||
		slice.Pass.OID != passedVerdict.OID {
		t.Fatalf(
			"repair candidates=%#v verdicts=%#v final=%#v/%#v",
			candidates,
			verdicts,
			slice.Candidate,
			slice.Pass,
		)
	}
	pinnedTree, pinned := state.Assembly.InputPins["T1"]
	finalSlice, finalOK := state.Slice("A2")
	finalTree := ""
	if finalOK && finalSlice.Candidate != nil &&
		finalSlice.Candidate.Receipt.ProductTree != nil {
		finalTree = *finalSlice.Candidate.Receipt.ProductTree
	}
	assemblyInput := ""
	if state.Assembly.Candidate != nil {
		assemblyInput = state.Assembly.Candidate.Receipt.Inputs["T1"]
	}
	if !pinned || pinnedTree == nil || finalTree == "" ||
		*pinnedTree != finalTree ||
		*pinnedTree == *failedCandidate.Receipt.ProductTree ||
		assemblyInput != finalTree {
		t.Fatalf(
			"repair assembly pin=%v input=%q failed=%q repaired=%q final=%q",
			pinnedTree,
			assemblyInput,
			*failedCandidate.Receipt.ProductTree,
			*repairedCandidate.Receipt.ProductTree,
			finalTree,
		)
	}
}
