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

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

const (
	journeyOpenAISecret = "journey-openai-secret"
	journeyGeminiSecret = "journey-gemini-secret"
)

type journeyProvider struct {
	t         *testing.T
	planBytes []byte
	repair    bool

	mu          sync.Mutex
	turns       map[string]int
	families    map[string]driver.ProfileFamily
	models      map[string]string
	access      map[string]driver.WorkspaceAccess
	httpCalls   int
	submissions int
}

type journeyPrompt struct {
	InvocationID   string                `json:"invocation_id"`
	Role           driver.Role           `json:"role"`
	Workspace      driver.Workspace      `json:"workspace"`
	Responsibility driver.Responsibility `json:"responsibility"`
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
	if prompt.Responsibility == driver.ImplementerImplementation &&
		turn == 1 {
		parts := strings.Split(prompt.InvocationID, "/")
		if len(parts) != 6 {
			err = fmt.Errorf("invalid implementation invocation")
		} else {
			pathValue, ok := journeySlicePaths()[parts[1]]
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
		len(value.Messages) == 0 ||
		value.Messages[0].Role != "user" {
		return "", "", fmt.Errorf("invalid OpenAI body")
	}
	var prompt string
	if err := json.Unmarshal(value.Messages[0].Content, &prompt); err != nil {
		return "", "", err
	}
	return prompt, value.Model, nil
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
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text *string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(body, &value); err != nil ||
		len(value.Contents) == 0 ||
		value.Contents[0].Role != "user" ||
		len(value.Contents[0].Parts) == 0 ||
		value.Contents[0].Parts[0].Text == nil {
		return "", "", fmt.Errorf("invalid Gemini body")
	}
	return *value.Contents[0].Parts[0].Text, model, nil
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
		submission.Plan, err = driver.NewPlanBytes(provider.planBytes)
	case driver.ImplementerDesign:
	case driver.CaptainReview:
		submission.Decision, err = driver.NewDecision(driver.DecisionProceed)
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
		Repository:    "acme/repo",
		TargetRef:     "refs/heads/main",
		ApprovalRef:   "github://acme/repo/issues/61#production-journey-v1",
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
			"\n```\n\nDeterministic production journey for " + repository + ".\n",
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
		SchemaVersion:     swornruntime.ManifestVersion,
		RunID:             "production-journey",
		Repository:        repository,
		Release:           "production-journey-release",
		TargetRef:         "refs/heads/main",
		Intent:            "Complete the deterministic configured production journey.",
		MaxParallelTracks: 3,
		Approval: swornruntime.ApprovalPolicy{
			Repository:          "acme/repo",
			Issue:               61,
			AllowedAuthorIDs:    []int64{42},
			AllowedAssociations: []string{"MEMBER"},
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

func runConfiguredProductionJourney(t *testing.T, repair bool) {
	t.Helper()
	approvals := &approvalServer{comments: make(map[int64][]approvalComment)}
	approvalHTTP := httptest.NewServer(http.HandlerFunc(approvals.serve))
	defer approvalHTTP.Close()

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
	buildBinary(
		t,
		swornBinary,
		"./cmd/sworn",
		"-X=github.com/swornagent/sworn/internal/runtime.githubAPIBase="+
			approvalHTTP.URL,
	)
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
	if stderr != "" || !strings.Contains(stdout, "state awaiting_approval") ||
		runGit(t, repository, "rev-parse", "main") != targetBefore {
		t.Fatalf("production start stdout=%q stderr=%q", stdout, stderr)
	}
	beforeRestart, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeRestart, configBody) ||
		driver.Digest(beforeRestart) != loaded.ConfigurationDigest() {
		t.Fatal("production config identity changed before restart")
	}

	approvals.publish(61, approvalFor(61, "production-journey-v1", plan))
	installApprovedPlan(t, repository, planBytes)
	stdout, stderr = runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		map[string]string{
			"SWORN_JOURNEY_OPENAI_KEY": journeyOpenAISecret,
			"SWORN_JOURNEY_GEMINI_KEY": journeyGeminiSecret,
		},
		"resume",
		"--run", "production-journey",
		"--journal", journalPath,
		"--command", "resume-1",
		"--generation", "0",
		"--config", configPath,
	)
	if stderr != "" || !strings.Contains(stdout, "state complete") {
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
				SchemaVersion string `json:"schema_version"`
				Context       struct {
					InvocationID    string                 `json:"invocation_id"`
					Responsibility  driver.Responsibility  `json:"responsibility"`
					WorkspaceAccess driver.WorkspaceAccess `json:"workspace_access"`
					Attempt         int64                  `json:"attempt"`
					Epoch           int64                  `json:"epoch"`
					Try             int64                  `json:"try"`
				} `json:"context"`
			}
			maxAttempt := int64(1)
			if repair {
				maxAttempt = 2
			}
			if json.Unmarshal(command.Payload, &payload) != nil ||
				payload.SchemaVersion != "sworn.production-dispatch/v1" ||
				payload.Context.InvocationID == "" ||
				payload.Context.Attempt < 1 ||
				payload.Context.Attempt > maxAttempt ||
				payload.Context.Epoch != 1 ||
				payload.Context.Try != 1 {
				t.Fatalf("production dispatch command = %s", command.Payload)
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
	wantInvocations := 18
	wantHTTPCalls := 22
	if repair {
		wantInvocations = 20
		wantHTTPCalls = 25
	}
	if len(driverEffects) != wantInvocations ||
		len(contexts) != wantInvocations {
		t.Fatalf(
			"production dispatch cardinality effects=%d contexts=%d",
			len(driverEffects),
			len(contexts),
		)
	}
	var inputTokens, outputTokens int64
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
	}
	wantInputTokens, wantOutputTokens := int64(154), int64(110)
	if repair {
		wantInputTokens, wantOutputTokens = 175, 125
	}
	if inputTokens != wantInputTokens ||
		outputTokens != wantOutputTokens {
		t.Fatalf(
			"production usage input=%d output=%d, want %d/%d",
			inputTokens,
			outputTokens,
			wantInputTokens,
			wantOutputTokens,
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
