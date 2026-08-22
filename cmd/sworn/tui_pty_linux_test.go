//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

func openPTY(t *testing.T, rows, cols uint16) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	unlock := int32(0)
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK,
		uintptr(unsafe.Pointer(&unlock)),
	); errno != 0 {
		master.Close()
		t.Fatalf("unlock pty: %v", errno)
	}
	var number uint32
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN,
		uintptr(unsafe.Pointer(&number)),
	); errno != 0 {
		master.Close()
		t.Fatalf("resolve pty: %v", errno)
	}
	slave, err := os.OpenFile(
		fmt.Sprintf("/dev/pts/%d", number),
		os.O_RDWR|syscall.O_NOCTTY,
		0,
	)
	if err != nil {
		master.Close()
		t.Fatalf("open pty slave: %v", err)
	}
	size := winsize{rows: rows, cols: cols}
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&size)),
	); errno != 0 {
		master.Close()
		slave.Close()
		t.Fatalf("size pty: %v", errno)
	}
	return master, slave
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b[()][A-Z0-9]|\x1b[=>]`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

type ptySession struct {
	t       *testing.T
	master  *os.File
	command *exec.Cmd
	done    chan error

	mu     sync.Mutex
	output strings.Builder
}

func startPTYSession(
	t *testing.T,
	binary string,
	environment map[string]string,
	args ...string,
) *ptySession {
	t.Helper()
	master, slave := openPTY(t, 48, 140)
	command := exec.Command(binary, args...)
	overrides := map[string]string{"TERM": "xterm-256color"}
	for key, value := range environment {
		overrides[key] = value
	}
	command.Env = cleanEnvironment(overrides)
	command.Stdin, command.Stdout, command.Stderr = slave, slave, slave
	command.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, Setctty: true, Ctty: 0,
	}
	if err := command.Start(); err != nil {
		master.Close()
		slave.Close()
		t.Fatalf("start %v: %v", args, err)
	}
	slave.Close()

	session := &ptySession{t: t, master: master, command: command, done: make(chan error, 1)}
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := master.Read(buffer)
			if count > 0 {
				session.mu.Lock()
				session.output.Write(buffer[:count])
				session.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { session.done <- command.Wait() }()
	t.Cleanup(func() { session.stop() })
	return session
}

func (s *ptySession) screen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return stripANSI(s.output.String())
}

func (s *ptySession) waitFor(label string, timeout time.Duration, wanted ...string) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screen := s.screen()
		missing := false
		for _, fragment := range wanted {
			if !strings.Contains(screen, fragment) {
				missing = true
				break
			}
		}
		if !missing {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.t.Fatalf("tui %s did not show %v; screen:\n%s", label, wanted, s.screen())
}

func (s *ptySession) send(keys string) {
	s.t.Helper()
	if _, err := s.master.WriteString(keys); err != nil {
		s.t.Fatalf("write to tui: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
}

func (s *ptySession) openControls() {
	s.t.Helper()
	s.waitFor("loaded board", 60*time.Second, "available controls")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		s.clear()
		s.send("a")
		time.Sleep(150 * time.Millisecond)
		if strings.Contains(s.screen(), "AVAILABLE CONTROLS") {
			return
		}
	}
	s.t.Fatalf("tui did not open its controls; screen:\n%s", s.screen())
}

func (s *ptySession) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output.Reset()
}

func (s *ptySession) stop() {
	if s.command.Process != nil {
		_, _ = s.master.WriteString("q")
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			_ = s.command.Process.Kill()
			<-s.done
		}
	}
	s.master.Close()
}

func (s *ptySession) quit() {
	s.t.Helper()
	if _, err := s.master.WriteString("q"); err != nil {
		s.t.Fatalf("quit tui: %v", err)
	}
	select {
	case err := <-s.done:
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 0 {
				s.t.Fatalf("tui exit: %v\nscreen:\n%s", err, s.screen())
			}
		}
	case <-time.After(15 * time.Second):
		_ = s.command.Process.Kill()
		<-s.done
		s.t.Fatalf("tui did not exit; screen:\n%s", s.screen())
	}
	s.command.Process = nil
}

const (
	tuiAnswerTestSecret   = "tui-answer-secret"
	tuiAnswerTestQuestion = "Which exact approved fixture value should I use?"
	tuiAnswerTestAnswer   = "approved-tui-answer-canary"
)

type tuiAnswerProvider struct {
	t        *testing.T
	question string
	answer   string

	mu    sync.Mutex
	turns map[string]int
}

func (p *tuiAnswerProvider) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, driver.MaxProviderRequestBytes+1))
	if err != nil || len(body) > driver.MaxProviderRequestBytes {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+tuiAnswerTestSecret {
		http.Error(w, "credential mismatch", http.StatusUnauthorized)
		return
	}
	promptBody, _, err := openAIJourneyPrompt(r, body)
	if err != nil {
		p.t.Errorf("tui answer provider request: %v", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var header struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal([]byte(promptBody), &header); err != nil {
		p.t.Errorf("prompt header unmarshal: %v", err)
		http.Error(w, "invalid prompt", http.StatusBadRequest)
		return
	}

	var invocationID string
	var toolName string
	var arguments map[string]any

	switch header.SchemaVersion {
	case "sworn.automation-prompt/v1":
		var prompt struct {
			Request struct {
				InvocationID string `json:"invocation_id"`
			} `json:"request"`
		}
		if err := json.Unmarshal([]byte(promptBody), &prompt); err != nil || prompt.Request.InvocationID == "" {
			p.t.Errorf("invalid automation prompt: %v", err)
			http.Error(w, "invalid prompt", http.StatusBadRequest)
			return
		}
		invocationID = prompt.Request.InvocationID
		p.mu.Lock()
		p.turns[invocationID]++
		p.mu.Unlock()
		toolName = "sworn_recovery_decide"
		arguments = map[string]any{
			"decision": map[string]any{
				"schema_version": driver.RecoveryDecisionSchemaVersion,
				"invocation_id":  invocationID,
				"action":         driver.RecoveryPauseForHuman,
			},
		}
	case "sworn.model-prompt/v1":
		var prompt struct {
			InvocationID   string                `json:"invocation_id"`
			Responsibility driver.Responsibility `json:"responsibility"`
			Recovery       *struct {
				Kind    driver.RecoverableInputKind `json:"kind"`
				Content string                      `json:"content"`
			} `json:"recovery,omitempty"`
		}
		if err := json.Unmarshal([]byte(promptBody), &prompt); err != nil || prompt.InvocationID == "" {
			p.t.Errorf("invalid model prompt: %v", err)
			http.Error(w, "invalid prompt", http.StatusBadRequest)
			return
		}
		invocationID = prompt.InvocationID
		p.mu.Lock()
		p.turns[invocationID]++
		turn := p.turns[invocationID]
		p.mu.Unlock()

		switch prompt.Responsibility {
		case driver.PlannerProposal:
			toolName = "sworn_submit"
			arguments = map[string]any{
				"submission": map[string]any{
					"schema_version": driver.SubmissionSchemaVersion,
					"invocation_id":  prompt.InvocationID,
					"responsibility": prompt.Responsibility,
					"summary":        "Planner proposal",
					"detail":         "Detail",
				},
			}
		case driver.ImplementerDesign:
			toolName = "sworn_submit"
			arguments = map[string]any{
				"submission": map[string]any{
					"schema_version": driver.SubmissionSchemaVersion,
					"invocation_id":  prompt.InvocationID,
					"responsibility": prompt.Responsibility,
					"summary":        "Implementer design",
					"detail":         "Detail",
				},
			}
		case driver.CaptainReview:
			toolName = "sworn_submit"
			arguments = map[string]any{
				"submission": map[string]any{
					"schema_version": driver.SubmissionSchemaVersion,
					"invocation_id":  prompt.InvocationID,
					"responsibility": prompt.Responsibility,
					"summary":        "Captain review",
					"detail":         "Detail",
					"decision":       map[string]any{"outcome": "proceed"},
				},
			}
		case driver.ImplementerImplementation:
			if prompt.Recovery == nil {
				toolName = "sworn_yield"
				q := p.question
				if q == "" {
					q = tuiAnswerTestQuestion
				}
				arguments = map[string]any{
					"yield": map[string]any{
						"schema_version": driver.YieldSchemaVersion,
						"invocation_id":  prompt.InvocationID,
						"kind":           driver.YieldQuestion,
						"message":        q,
					},
				}
			} else {
				if turn == 2 {
					toolName = "Write"
					arguments = map[string]any{
						"path":    "/workspace/one.txt",
						"content": "tui answer product\n",
					}
				} else {
					toolName = "sworn_submit"
					rawChecks := []byte("implementation checks\n")
					sum := sha256.Sum256(rawChecks)
					arguments = map[string]any{
						"submission": map[string]any{
							"schema_version": driver.SubmissionSchemaVersion,
							"invocation_id":  prompt.InvocationID,
							"responsibility": prompt.Responsibility,
							"summary":        "Implementer implementation with answer",
							"detail":         "Detail",
							"checks": map[string]any{
								"byte_count": len(rawChecks),
								"digest":     "sha256:" + hex.EncodeToString(sum[:]),
								"bytes":      base64.StdEncoding.EncodeToString(rawChecks),
							},
						},
					}
				}
			}
		case driver.WorkVerification:
			toolName = "sworn_submit"
			rawChecks := []byte("work verification checks\n")
			sum := sha256.Sum256(rawChecks)
			arguments = map[string]any{
				"submission": map[string]any{
					"schema_version": driver.SubmissionSchemaVersion,
					"invocation_id":  prompt.InvocationID,
					"responsibility": prompt.Responsibility,
					"summary":        "Work verification pass",
					"detail":         "Detail",
					"decision":       map[string]any{"outcome": "pass"},
					"checks": map[string]any{
						"byte_count": len(rawChecks),
						"digest":     "sha256:" + hex.EncodeToString(sum[:]),
						"bytes":      base64.StdEncoding.EncodeToString(rawChecks),
					},
				},
			}
		case driver.AssemblyVerification:
			toolName = "sworn_submit"
			rawChecks := []byte("assembly verification checks\n")
			sum := sha256.Sum256(rawChecks)
			arguments = map[string]any{
				"submission": map[string]any{
					"schema_version": driver.SubmissionSchemaVersion,
					"invocation_id":  prompt.InvocationID,
					"responsibility": prompt.Responsibility,
					"summary":        "Assembly verification pass",
					"detail":         "Detail",
					"decision":       map[string]any{"outcome": "pass"},
					"checks": map[string]any{
						"byte_count": len(rawChecks),
						"digest":     "sha256:" + hex.EncodeToString(sum[:]),
						"bytes":      base64.StdEncoding.EncodeToString(rawChecks),
					},
				},
			}
		}
	default:
		p.t.Errorf("unexpected schema_version: %q", header.SchemaVersion)
		http.Error(w, "invalid schema", http.StatusBadRequest)
		return
	}

	p.mu.Lock()
	turn := p.turns[invocationID]
	p.mu.Unlock()

	callID := fmt.Sprintf("call-%s-%d", invocationID, turn)
	argumentBody, _ := json.Marshal(arguments)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []any{map[string]any{
					"id":   callID,
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

func tuiAnswerTestConfig(t *testing.T, serverURL string) ([]byte, string) {
	t.Helper()
	credential := "tui-answer-env"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key:       credential,
			Kind:      driver.CredentialEnvironment,
			Reference: "SWORN_TUI_ANSWER_KEY",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key:              "tui-answer-openai",
					ID:               "sworn.tui.answer",
					Version:          "1.0.0",
					Endpoint:         serverURL + "/openai/v1/chat/completions",
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{credential},
					ResponseBytes:    driver.MaxProviderResponseBytes,
				},
				API: driver.OpenAIChatCompletionsAPI,
			},
		}},
		Profiles: []driver.DriverProfile{{
			Key:                 "tui-answer-profile",
			Adapter:             "tui-answer-openai",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"tui-answer-model"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body, credential
}

func writeTUIAnswerManifest(
	t *testing.T,
	directory, repository, release, runID string,
	configDigest string,
) (string, baton.Plan) {
	t.Helper()
	_, plan := hostedDrivePlanFixture(t, release, repository)
	digest := plan.Digest()
	selection := driver.RoleSelection{
		Profile: "tui-answer-profile",
		Model:   "tui-answer-model",
	}
	manifest := runtimepkg.Manifest{
		GitIdentity:       gitx.Identity{Name: "TUI Test Engine", Email: "engine@example.test"},
		SchemaVersion:     runtimepkg.ManifestVersion,
		RunID:             runID,
		Repository:        repository,
		Release:           release,
		TargetRef:         "refs/heads/main",
		Intent:            "Drive the hosted TUI answer survival test.",
		MaxParallelTracks: 1,
		Authority: runtimepkg.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
			BootstrapApprovedPlanDigest: &digest,
		},
		DriverConfigDigest: configDigest,
		Roles: driver.RoleSelections{
			Planner:     selection,
			Implementer: selection,
			Captain:     selection,
			Verifier:    selection,
		},
		Automation: &runtimepkg.AutomationSelections{
			Recovery: selection,
		},
		Limits: driver.Limits{TimeoutMillis: 30_000, OutputBytes: 65_536},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	manifestPath := filepath.Join(directory, release+".json")
	if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifestPath, plan
}

func installHostedDrivePlan(t *testing.T, root string, plan baton.Plan) {
	t.Helper()
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	repoView, err := gitx.Open(root, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	identity := gitx.Identity{Name: "Test Engine", Email: "engine@example.test"}
	actions, err := baton.NewActions(baton.UseGitRepository(repoView), inertness, identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: plan.Bytes(),
		Summary:   "Install fixture plan",
		Detail:    []byte("Fixture detail"),
	}); err != nil {
		t.Fatal(err)
	}
}

// A1: Board actions no longer create and destroy the drive host per action:
// the command service lives across actions and the drive an answer starts survives the action's return.
func TestTUIActionDriveSurvivesActionReturn(t *testing.T) {
	t.Setenv("SWORN_TUI_ANSWER_KEY", tuiAnswerTestSecret)

	provider := &tuiAnswerProvider{
		t:        t,
		question: tuiAnswerTestQuestion,
		answer:   tuiAnswerTestAnswer,
		turns:    make(map[string]int),
	}
	server := httptest.NewServer(http.HandlerFunc(provider.serve))
	t.Cleanup(server.Close)

	root, _ := projectRepositoryFixture(t)
	stateDir := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configBody, _ := tuiAnswerTestConfig(t, server.URL)
	configPath := filepath.Join(stateDir, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	loadedConfig, err := driver.DecodeDriverConfig(configBody)
	if err != nil {
		t.Fatal(err)
	}

	runID := "run-tui-a1-survives"
	release := "delivery-tui-a1"
	manifestPath, plan := writeTUIAnswerManifest(t, manifestDir, root, release, runID, loadedConfig.ConfigurationDigest())
	installHostedDrivePlan(t, root, plan)
	journalPath := filepath.Join(stateDir, "sworn.db")

	swornBinary := filepath.Join(t.TempDir(), "sworn")
	buildTestBinary(t, swornBinary, "./cmd/sworn", "")
	env := map[string]string{
		"SWORN_TUI_ANSWER_KEY": tuiAnswerTestSecret,
	}

	// 1. Start run via CLI to park on human turn (answer_attention)
	startOut, startErr := runBinaryWithEnvironmentTimeout(
		t, swornBinary, 0, env, 30*time.Second,
		"run", "--manifest", manifestPath, "--journal", journalPath, "--config", configPath,
	)
	if startErr != "" || !strings.Contains(startOut, "  state: parked") {
		t.Fatalf("run start stdout=%q stderr=%q", startOut, startErr)
	}

	// 2. Open resident TUI backend
	backend := newProjectTUIBackend(root, journalPath, configPath, manifestDir)
	defer backend.Close()

	catalog, err := backend.Catalog(context.Background())
	if err != nil || len(catalog.Entries) != 1 {
		t.Fatalf("catalog entries = %#v, err = %v", catalog.Entries, err)
	}
	board, err := backend.Board(context.Background(), catalog.Entries[0].Selection)
	if err != nil {
		t.Fatalf("board failed: %v", err)
	}
	var answerAction *cockpit.Action
	for _, action := range board.Actions {
		if action.Kind == "answer_attention" {
			a := action
			answerAction = &a
			break
		}
	}
	if answerAction == nil {
		t.Fatalf("board actions omitted answer_attention: %#v", board.Actions)
	}

	// 3. Execute answer_attention through TUI backend.
	// The action returns promptly while the resident host's service carries the drive.
	executeStart := time.Now()
	if err := backend.Execute(context.Background(), catalog.Entries[0].Selection, *answerAction, tuiAnswerTestAnswer); err != nil {
		t.Fatalf("backend.Execute answer_attention failed: %v", err)
	}
	executeDuration := time.Since(executeStart)
	if executeDuration > 3*time.Second {
		t.Fatalf("backend.Execute took %v, want prompt return (< 3s)", executeDuration)
	}

	// 4. Assert drive an answer starts survives action return and completes in background
	statusReader, err := runtimepkg.OpenStatusService(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer statusReader.Close()

	deadline := time.Now().Add(30 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		status, err := statusReader.Status(context.Background(), runID)
		if err == nil && status.State == "complete" {
			completed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !completed {
		t.Fatal("background drive did not reach complete after action return")
	}

	// 5. Verify journal contains succeeded baton.merge effect
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	hasMerge := false
	for _, effect := range snapshot.Effects {
		if effect.Kind == "baton.merge" && effect.State == journal.Succeeded {
			hasMerge = true
			break
		}
	}
	if !hasMerge {
		t.Fatal("merge effect not recorded in journal")
	}

	// 6. Close backend and verify clean shutdown releases owner lease
	if err := backend.Close(); err != nil {
		t.Fatalf("backend.Close failed: %v", err)
	}

	owner, present, err := store.CurrentOwner(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatalf("owner lease remained claimed after completion: %#v", owner)
	}
}

// A2: An accepted answer is followed by observable drive progress in the same TUI session without restarting the TUI.
func TestTUIAnswerObservesSubsequentDriveProgress(t *testing.T) {
	provider := &tuiAnswerProvider{
		t:        t,
		question: tuiAnswerTestQuestion,
		answer:   tuiAnswerTestAnswer,
		turns:    make(map[string]int),
	}
	server := httptest.NewServer(http.HandlerFunc(provider.serve))
	t.Cleanup(server.Close)

	root, _ := projectRepositoryFixture(t)
	stateDir := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configBody, _ := tuiAnswerTestConfig(t, server.URL)
	configPath := filepath.Join(stateDir, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	loadedConfig, err := driver.DecodeDriverConfig(configBody)
	if err != nil {
		t.Fatal(err)
	}

	runID := "run-tui-a2-progress"
	release := "delivery-tui-a2"
	manifestPath, plan := writeTUIAnswerManifest(t, manifestDir, root, release, runID, loadedConfig.ConfigurationDigest())
	installHostedDrivePlan(t, root, plan)
	journalPath := filepath.Join(stateDir, "sworn.db")

	swornBinary := filepath.Join(t.TempDir(), "sworn")
	buildTestBinary(t, swornBinary, "./cmd/sworn", "")
	env := map[string]string{
		"SWORN_TUI_ANSWER_KEY": tuiAnswerTestSecret,
	}

	// 1. Start run via CLI to park on human turn
	startOut, startErr := runBinaryWithEnvironmentTimeout(
		t, swornBinary, 0, env, 30*time.Second,
		"run", "--manifest", manifestPath, "--journal", journalPath, "--config", configPath,
	)
	if startErr != "" || !strings.Contains(startOut, "  state: parked") {
		t.Fatalf("run start stdout=%q stderr=%q", startOut, startErr)
	}

	// 2. Open real TUI session over the parked run
	session := startPTYSession(t, swornBinary, env, "tui", "--project", root, "--journal", journalPath, "--config", configPath)

	session.waitFor("catalog", 20*time.Second, "RELEASES", release)
	session.send("\r")
	session.waitFor("board", 20*time.Second, "Needs you", "answer the question shown in Human attention")

	// 3. Open controls, navigate to answer_attention action, and submit answer
	session.openControls()
	session.waitFor("answer control", 20*time.Second, "Answer: Which exact approved fixture value should I")
	session.send("j") // move down to Cancel
	session.send("j") // move down to Answer
	session.send("\r")
	session.waitFor("answer overlay", 20*time.Second, "ctrl+s send")
	session.send(tuiAnswerTestAnswer)
	session.clear()
	session.send("\x13") // Ctrl+S to submit answer

	session.waitFor("accepted answer", 20*time.Second, "Answer: Which exact approved fixture value should I", "accepted.")

	// 4. Observable subsequent state transition to Complete occurs in the SAME TUI session
	session.waitFor("working or complete", 30*time.Second, "Status: Complete")

	// 5. Clean exit
	session.quit()

	// 6. Verify journal facts
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	hasMerge := false
	for _, effect := range snapshot.Effects {
		if effect.Kind == "baton.merge" && effect.State == journal.Succeeded {
			hasMerge = true
			break
		}
	}
	if !hasMerge {
		t.Fatal("merge effect not recorded in journal")
	}

	owner, present, err := store.CurrentOwner(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatalf("owner lease remained claimed after completion: %#v", owner)
	}
}
