//go:build linux

package adapter

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	requireCodexBoundaryEnvironment   = "SWORN_REQUIRE_CODEX_BOUNDARY"
	codexBinaryEnvironment            = "SWORN_CODEX_BINARY"
	executorShimSentinel              = "__sworn_codex_boundary_executor_shim__"
	nestedProbeSentinel               = "__sworn_codex_boundary_nested_probe__"
	testProviderCredentialEnvironment = "SWORN_CODEX_BOUNDARY_PROVIDER_TOKEN"
	testProviderCanaryPrefix          = "SWORN_TEST_PROVIDER_CANARY_"
	authFileCanaryPrefix              = "SWORN_AUTH_FILE_CANARY_"
	codexBoundaryModel                = "gpt-5.4"
	codexBoundaryCallID               = "sworn-boundary-call"
	hostileProjectCanary              = "SWORN_HOSTILE_PROJECT_CONFIG_CANARY_55fd71cc"
	proofContents                     = "nested-codex-sandbox-contained\n"
)

// TestCodexBoundaryExecutorShimProcess is the real Sworn executor shim when
// this test binary is launched by the transient user service.
func TestCodexBoundaryExecutorShimProcess(t *testing.T) {
	index := boundaryArgumentIndex(os.Args, executorShimSentinel)
	if index < 0 {
		return
	}
	os.Exit(executor.RunShim(os.Args[index+1:], os.Stdin, os.Stdout, os.Stderr))
}

// TestCodexBoundaryNestedProbeProcess is the exec_command selected by the
// scripted Responses turn. Its assertions are deliberately independent of the
// parent test process so they describe what a real Codex tool can observe.
func TestCodexBoundaryNestedProbeProcess(t *testing.T) {
	index := boundaryArgumentIndex(os.Args, nestedProbeSentinel)
	if index < 0 {
		return
	}
	os.Exit(runCodexBoundaryNestedProbe(os.Args[index+1:], os.Stdout, os.Stderr))
}

func TestRealCodexCLIBoundaryFeasibility(t *testing.T) {
	required := os.Getenv(requireCodexBoundaryEnvironment) == "1"
	codexBinary := requireExactCodexBinary(t, required)
	providerCanary := randomBoundaryCanary(t, testProviderCanaryPrefix)
	authFileCanary := randomBoundaryCanary(t, authFileCanaryPrefix)
	authFile := writeSyntheticCodexAuthFile(t, authFileCanary)
	executorInstance, runtimeRoot, writableRoot := requireCodexBoundaryExecutor(t, required, authFile)
	hostCanaryRoot := t.TempDir()
	hostCanary := filepath.Join(hostCanaryRoot, "host-only-canary")
	if err := os.WriteFile(hostCanary, []byte("host-only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceWorkspace := t.TempDir()
	if err := writeHostileProjectCanaries(sourceWorkspace); err != nil {
		t.Fatal(err)
	}
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	codexDigest, err := digestBoundaryFile(codexBinary)
	if err != nil {
		t.Fatal(err)
	}
	probeDigest, err := digestBoundaryFile(testBinary)
	if err != nil {
		t.Fatal(err)
	}
	codexInfo, err := os.Stat(codexBinary)
	if err != nil {
		t.Fatal(err)
	}
	workspaceDigest, _, err := executor.MeasureWorkspace(
		context.Background(), sourceWorkspace, executorInstance.EffectiveLimits().InputBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	var modelDiscoveryRequests atomic.Int32
	var providerRequests atomic.Int32
	var providerFailure atomic.Value
	providerBackend := httptest.NewServer(newCodexResponsesHandler(
		hostCanary,
		providerCanary,
		authFileCanary,
		&modelDiscoveryRequests,
		&providerRequests,
		&providerFailure,
	))
	t.Cleanup(providerBackend.Close)

	contextWithDeadline, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	codexArgv := codexBuilderArgvWithPrompt(
		codexBoundaryModel,
		"Run the boundary probe exactly once with exec_command, then finish.",
	)
	providerConfigured := false
	for index, argument := range codexArgv {
		if argument != `model_provider="openai"` {
			continue
		}
		codexArgv[index] = `model_provider="sworn_boundary"`
		provider := "model_providers.sworn_boundary=" + `{name="Sworn boundary",base_url=` +
			strconv.Quote(providerBackend.URL+"/v1") + `,env_key=` +
			strconv.Quote(testProviderCredentialEnvironment) + `,wire_api="responses",supports_websockets=false}`
		codexArgv = append(codexArgv[:index+1], append([]string{"-c", provider}, codexArgv[index+1:]...)...)
		providerConfigured = true
		break
	}
	if !providerConfigured {
		t.Fatal("production Codex argv did not expose its fixed provider selector")
	}
	completion, err := executorInstance.RunWritable(contextWithDeadline, executor.Invocation{
		SchemaVersion:    executor.InvocationSchemaVersion,
		ID:               "real-codex-boundary",
		Role:             "builder",
		NestedSandbox:    true,
		CredentialAccess: true,
		Workspace:        sourceWorkspace,
		WorkspaceDigest:  workspaceDigest,
		WorkspaceAccess:  executor.WorkspaceWritableExport,
		ExecutableInput:  "codex",
		Inputs: []executor.Input{{
			Name:   "codex",
			Path:   codexBinary,
			Digest: codexDigest,
		}, {
			Name:   "probe",
			Path:   testBinary,
			Digest: probeDigest,
		}},
		Argv:        codexArgv,
		Environment: map[string]string{testProviderCredentialEnvironment: providerCanary},
		Network:     executor.NetworkHost,
		Timeout:     25 * time.Second,
	})
	if err != nil {
		t.Fatalf(
			"run real Codex boundary: %v; exit=%d stdout=%q stderr=%q",
			err,
			completion.ExitCode,
			redactBoundarySecrets(completion.Stdout, providerCanary, authFileCanary),
			redactBoundarySecrets(completion.Stderr, providerCanary, authFileCanary),
		)
	}
	if completion.ExitCode != 0 || completion.Cancelled || completion.TimedOut || completion.OutputTruncated {
		t.Fatalf(
			"real Codex boundary exit=%d cancelled=%t timed_out=%t truncated=%t stdout=%q stderr=%q",
			completion.ExitCode,
			completion.Cancelled,
			completion.TimedOut,
			completion.OutputTruncated,
			redactBoundarySecrets(completion.Stdout, providerCanary, authFileCanary),
			redactBoundarySecrets(completion.Stderr, providerCanary, authFileCanary),
		)
	}
	if containsBoundarySecret(completion.Stdout, providerCanary, authFileCanary) ||
		containsBoundarySecret(completion.Stderr, providerCanary, authFileCanary) {
		t.Fatal("provider or auth-file credential canary appeared in successful Codex output")
	}
	if err := validateCodexJSONL(completion.Stdout); err != nil {
		t.Fatalf("production Codex JSONL completion contract: %v", err)
	}
	if completion.ExecutableInput != "codex" || !completion.CredentialAccess || len(completion.Inputs) != 2 {
		t.Fatalf(
			"executable binding = %q, credential_access=%t, inputs=%#v",
			completion.ExecutableInput,
			completion.CredentialAccess,
			completion.Inputs,
		)
	}
	var boundCodex *executor.BoundInput
	for index := range completion.Inputs {
		if completion.Inputs[index].Name == "codex" {
			boundCodex = &completion.Inputs[index]
		}
	}
	if boundCodex == nil || boundCodex.Digest != codexDigest || boundCodex.Size != uint64(codexInfo.Size()) {
		t.Fatalf("bound Codex input = %#v, want digest=%s size=%d", boundCodex, codexDigest, codexInfo.Size())
	}
	if !bytes.Contains(completion.Stdout, []byte("nested-codex-sandbox-contained")) {
		t.Fatalf(
			"boundary proof marker absent: stdout=%q stderr=%q",
			redactBoundarySecrets(completion.Stdout, providerCanary, authFileCanary),
			redactBoundarySecrets(completion.Stderr, providerCanary, authFileCanary),
		)
	}
	version := boundaryCodexVersion(completion.Stdout)
	if version == "" {
		t.Fatalf("Codex version absent from successful tool output: %q", redactBoundarySecrets(completion.Stdout, providerCanary, authFileCanary))
	}
	t.Logf("real Codex boundary binary: %s, %s", version, codexDigest)
	if failure := providerFailure.Load(); failure != nil {
		t.Fatalf("Responses test-provider validation: %s", failure.(string))
	}
	if got := modelDiscoveryRequests.Load(); got != 2 {
		t.Fatalf("test-provider model discovery requests = %d, want 2", got)
	}
	if got := providerRequests.Load(); got != 2 {
		t.Fatalf("Responses test-provider requests = %d, want model call plus tool output", got)
	}
	t.Logf(
		"test-provider requests: model_discovery=%d responses=%d",
		modelDiscoveryRequests.Load(),
		providerRequests.Load(),
	)
	if completion.Export == nil {
		t.Fatal("real Codex boundary did not produce a measured workspace export")
	}
	export := *completion.Export
	t.Cleanup(func() { _ = executorInstance.DiscardExport(context.Background(), export) })
	if err := executorInstance.ValidateExport(context.Background(), export); err != nil {
		t.Fatalf("validate Codex workspace export: %v", err)
	}
	proof, err := os.ReadFile(filepath.Join(export.Path, "codex-boundary-proof.txt"))
	if err != nil || string(proof) != proofContents {
		t.Fatalf("nested workspace proof = %q, %v", proof, err)
	}
	for _, name := range []string{"codex", "probe", "codex-boundary.test"} {
		if _, err := os.Lstat(filepath.Join(export.Path, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("executable residue %q entered candidate export: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(sourceWorkspace, "codex-boundary-proof.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source workspace was changed: %v", err)
	}
	if canary, err := os.ReadFile(hostCanary); err != nil || string(canary) != "host-only\n" {
		t.Fatalf("host-only canary changed: %q, %v", canary, err)
	}
	if err := executorInstance.DiscardExport(context.Background(), export); err != nil {
		t.Fatalf("discard Codex workspace export: %v", err)
	}
	if _, err := os.Lstat(export.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discarded workspace export remains: %v", err)
	}
	assertBoundaryRootEmpty(t, runtimeRoot)
	assertBoundaryRootEmpty(t, writableRoot)
}

func newCodexResponsesHandler(
	hostCanary string,
	providerCanary string,
	authFileCanary string,
	modelDiscoveryRequests *atomic.Int32,
	requests *atomic.Int32,
	failure *atomic.Value,
) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fail := func(format string, arguments ...any) {
			message := fmt.Sprintf(format, arguments...)
			failure.Store(message)
			http.Error(writer, message, http.StatusBadRequest)
		}
		if request.Header.Get("Authorization") != "Bearer "+providerCanary {
			fail("test-provider request did not carry the exact provider credential")
			return
		}
		for _, values := range request.Header {
			for _, value := range values {
				if strings.Contains(value, authFileCanary) {
					fail("ChatGPT auth-file canary reached the test provider")
					return
				}
			}
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/models" {
			modelDiscoveryRequests.Add(1)
			body, err := io.ReadAll(io.LimitReader(request.Body, 1))
			if err != nil {
				fail("read model-discovery request: %v", err)
				return
			}
			if len(body) != 0 {
				fail("model-discovery request unexpectedly carried a body")
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{"models": []any{}})
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			fail("unexpected test-provider request %s %s", request.Method, request.URL.Path)
			return
		}
		requestNumber := requests.Add(1)
		if encoding := request.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
			fail("unsupported Responses request encoding %q", encoding)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 2<<20))
		if err != nil {
			fail("read Responses request: %v", err)
			return
		}
		if bytes.Contains(body, []byte(hostileProjectCanary)) {
			fail("hostile candidate project instructions reached the model request")
			return
		}
		if containsBoundarySecret(body, providerCanary, authFileCanary) {
			fail("credential canary reached the model request body")
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.Model != codexBoundaryModel {
			fail("Responses request model=%q decode=%v", payload.Model, err)
			return
		}
		wantTools := codexBuilderToolNames()
		if got := boundaryToolNames(body); strings.Join(got, "\x00") != strings.Join(wantTools, "\x00") {
			fail("model tool allowlist=%v, want %v", got, wantTools)
			return
		}
		toolSchemaDigest, err := boundaryToolSchemaDigest(body)
		if err != nil || toolSchemaDigest != pinnedCodexToolSchemaDigest {
			fail("model tool schema digest=%q, want %q: %v", toolSchemaDigest, pinnedCodexToolSchemaDigest, err)
			return
		}
		switch requestNumber {
		case 1:
			probeArguments, err := json.Marshal(map[string]any{
				"cmd": strings.Join([]string{
					"/inputs/codex --version",
					"/usr/bin/cp /inputs/probe /tmp/codex-boundary.test",
					"/usr/bin/chmod 0700 /tmp/codex-boundary.test",
					strings.Join([]string{
						shellBoundaryQuote("/tmp/codex-boundary.test"),
						shellBoundaryQuote("-test.run=^TestCodexBoundaryNestedProbeProcess$"),
						"--",
						shellBoundaryQuote(nestedProbeSentinel),
						shellBoundaryQuote("http://" + request.Host + "/v1/responses"),
						shellBoundaryQuote(hostCanary),
					}, " "),
				}, " && "),
				"yield_time_ms":     10_000,
				"max_output_tokens": 2_000,
			})
			if err != nil {
				fail("encode shell command: %v", err)
				return
			}
			writeBoundarySSE(writer,
				map[string]any{"type": "response.created", "response": map[string]any{"id": "response-1"}},
				map[string]any{
					"type": "response.output_item.done",
					"item": map[string]any{
						"type": "function_call", "call_id": codexBoundaryCallID,
						"name": "exec_command", "arguments": string(probeArguments),
					},
				},
				boundaryCompletedEvent("response-1"),
			)
		case 2:
			if !hasBoundaryFunctionOutput(body) {
				fail("second Responses request omitted the successful boundary tool output")
				return
			}
			writeBoundarySSE(writer,
				map[string]any{"type": "response.created", "response": map[string]any{"id": "response-2"}},
				map[string]any{
					"type": "response.output_item.done",
					"item": map[string]any{
						"type": "message", "role": "assistant", "id": "message-2",
						"content": []map[string]any{{"type": "output_text", "text": "boundary complete"}},
					},
				},
				boundaryCompletedEvent("response-2"),
			)
		default:
			fail("unexpected Responses request number %d", requestNumber)
		}
	})
}

func boundaryToolNames(body []byte) []string {
	var payload struct {
		Tools []map[string]any `json:"tools"`
	}
	_ = json.Unmarshal(body, &payload)
	names := make([]string, 0, len(payload.Tools))
	for _, tool := range payload.Tools {
		kind, _ := tool["type"].(string)
		name, _ := tool["name"].(string)
		if name != "" {
			kind += ":" + name
		}
		names = append(names, kind)
	}
	sort.Strings(names)
	return names
}

func boundaryToolSchemaDigest(body []byte) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	tools, ok := payload["tools"]
	if !ok {
		return "", errors.New("Responses request omitted tools")
	}
	canonical, err := protocol.CanonicalizeJSON(tools)
	if err != nil {
		return "", err
	}
	return protocol.RawDigest(canonical), nil
}

func boundaryCodexVersion(output []byte) string {
	text := string(output)
	start := strings.Index(text, "codex-cli ")
	if start < 0 {
		return ""
	}
	rest := text[start:]
	if end := strings.IndexAny(rest, "\\\"\r\n"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func writeBoundarySSE(writer http.ResponseWriter, events ...map[string]any) {
	writer.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		payload, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event["type"], payload)
	}
}

func boundaryCompletedEvent(responseID string) map[string]any {
	return map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": responseID,
			"usage": map[string]any{
				"input_tokens": 0, "input_tokens_details": nil,
				"output_tokens": 0, "output_tokens_details": nil, "total_tokens": 0,
			},
		},
	}
}

func hasBoundaryFunctionOutput(body []byte) bool {
	var payload struct {
		Input []struct {
			Type   string          `json:"type"`
			CallID string          `json:"call_id"`
			Output json.RawMessage `json:"output"`
		} `json:"input"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return false
	}
	for _, item := range payload.Input {
		if item.Type == "function_call_output" && item.CallID == codexBoundaryCallID &&
			bytes.Contains(item.Output, []byte(strings.TrimSpace(proofContents))) &&
			bytes.Contains(item.Output, []byte("codex-cli ")) {
			return true
		}
	}
	return false
}

func shellBoundaryQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func runCodexBoundaryNestedProbe(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) != 2 {
		_, _ = fmt.Fprintln(stderr, "nested Codex boundary probe requires provider backend and host canary")
		return 2
	}
	if _, present := os.LookupEnv(testProviderCredentialEnvironment); present {
		_, _ = fmt.Fprintln(stderr, "nested tool inherited the test-provider credential")
		return 1
	}
	for _, value := range os.Environ() {
		if strings.Contains(value, testProviderCanaryPrefix) || strings.Contains(value, authFileCanaryPrefix) {
			_, _ = fmt.Fprintln(stderr, "nested tool environment exposed a credential sentinel")
			return 1
		}
	}
	if err := assertNoVisibleProcContains(testProviderCanaryPrefix, authFileCanaryPrefix); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested /proc boundary: %v\n", err)
		return 1
	}
	if err := assertCredentialCanaryUnreadable(authFileCanaryPrefix); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested credential-file boundary: %v\n", err)
		return 1
	}
	if err := assertNoOpenSocketFD(); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested socket boundary: %v\n", err)
		return 1
	}
	client := &http.Client{Timeout: 750 * time.Millisecond}
	if response, err := client.Get(arguments[0]); err == nil {
		_ = response.Body.Close()
		_, _ = fmt.Fprintln(stderr, "nested tool reached the outer provider backend")
		return 1
	}
	if info, err := os.Stat("/inputs/probe"); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		_, _ = fmt.Fprintf(stderr, "nested tool input binding: %#v, %v\n", info, err)
		return 1
	}
	if err := os.WriteFile("/workspace/codex-boundary-proof.txt", []byte(proofContents), 0o600); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested tool workspace write: %v\n", err)
		return 1
	}
	for _, path := range []string{
		"/usr/sworn-codex-boundary-forbidden",
		executor.CredentialFileTarget,
		"/inputs/codex",
		"/inputs/probe",
		"/inputs/forbidden",
	} {
		if err := os.WriteFile(path, []byte("forbidden"), 0o600); err == nil {
			_, _ = fmt.Fprintf(stderr, "nested tool wrote non-writable path %s\n", path)
			return 1
		}
	}
	if contents, err := os.ReadFile(arguments[1]); err == nil {
		_, _ = fmt.Fprintf(stderr, "nested tool reached host-only canary: %q\n", contents)
		return 1
	}
	_, _ = fmt.Fprint(stdout, proofContents)
	return 0
}

func requireExactCodexBinary(t *testing.T, required bool) string {
	t.Helper()
	path := os.Getenv(codexBinaryEnvironment)
	if path == "" {
		codexBoundaryUnavailable(t, required, "%s is unset", codexBinaryEnvironment)
	}
	if err := validatePinnedCodexBinary(context.Background(), path); err != nil {
		codexBoundaryUnavailable(t, required, "%s is not the pinned production profile: %v", codexBinaryEnvironment, err)
	}
	return path
}

func requireCodexBoundaryExecutor(
	t *testing.T,
	required bool,
	authFile string,
) (*executor.LinuxExecutor, string, string) {
	t.Helper()
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		codexBoundaryUnavailable(t, required, "XDG_RUNTIME_DIR is unavailable")
	}
	writableRoot, err := os.MkdirTemp(xdgRuntime, "sworn-codex-boundary-")
	if err != nil {
		codexBoundaryUnavailable(t, required, "create writable executor root: %v", err)
	}
	if err := os.Chmod(writableRoot, 0o700); err != nil {
		_ = os.RemoveAll(writableRoot)
		codexBoundaryUnavailable(t, required, "secure writable executor root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(writableRoot) })
	runtimeRoot := t.TempDir()
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		codexBoundaryUnavailable(t, required, "secure executor runtime root: %v", err)
	}
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	limits := executor.DefaultLimits()
	limits.Runtime = 30 * time.Second
	limits.MemoryBytes = 1 << 30
	limits.Tasks = 128
	limits.FileBytes = 64 << 20
	limits.TempBytes = 64 << 20
	limits.HomeBytes = 64 << 20
	limits.InputBytes = 768 << 20
	limits.WorkspaceBytes = 768 << 20
	limits.StdoutBytes = 256 << 10
	limits.StderrBytes = 256 << 10
	executorInstance, err := executor.NewLinux(executor.Options{
		RuntimeRoot:         runtimeRoot,
		WritableRoot:        writableRoot,
		ShimArgv:            []string{testBinary, "-test.run=^TestCodexBoundaryExecutorShimProcess$", "--", executorShimSentinel},
		Limits:              limits,
		AllowedEnvironment:  []string{testProviderCredentialEnvironment},
		AllowHostNetwork:    true,
		AllowNestedSandbox:  true,
		CredentialFile:      authFile,
		AllowCredentialFile: true,
	})
	if err == nil {
		probeContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err = executorInstance.Probe(probeContext)
	}
	if err != nil {
		codexBoundaryUnavailable(t, required, "Linux executor capability unavailable: %v", err)
	}
	return executorInstance, runtimeRoot, writableRoot
}

func codexBoundaryUnavailable(t *testing.T, required bool, format string, arguments ...any) {
	t.Helper()
	message := fmt.Sprintf(format, arguments...)
	if required {
		t.Fatal(message)
	}
	t.Skip(message)
}

func writeSyntheticCodexAuthFile(t *testing.T, canary string) string {
	t.Helper()
	authParent := filepath.Join(t.TempDir(), "codex-auth")
	if err := os.Mkdir(authParent, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	authDocument := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]string{
			"id_token": syntheticBoundaryJWT(t, map[string]any{
				"email": "sworn-boundary@example.invalid",
				"exp":   now.Add(24 * time.Hour).Unix(),
				"https://api.openai.com/auth": map[string]string{
					"chatgpt_account_id": "sworn-boundary-account",
					"chatgpt_plan_type":  "test",
				},
			}),
			"access_token": syntheticBoundaryJWT(t, map[string]any{
				"aud": []string{"https://api.openai.com/v1"},
				"exp": now.Add(24 * time.Hour).Unix(),
				"https://api.openai.com/auth": map[string]string{
					"chatgpt_account_id": "sworn-boundary-account",
				},
			}),
			"refresh_token": "rt." + canary,
			"account_id":    "sworn-boundary-account",
		},
		"last_refresh": now.Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(authDocument)
	if err != nil {
		t.Fatal(err)
	}
	authFile := filepath.Join(authParent, "auth.json")
	if err := os.WriteFile(authFile, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	parentInfo, err := os.Stat(authParent)
	if err != nil || parentInfo.Mode().Perm() != 0o700 {
		t.Fatalf("synthetic auth parent mode = %v, %v", parentInfo, err)
	}
	fileInfo, err := os.Stat(authFile)
	if err != nil || fileInfo.Mode().Perm() != 0o600 || !fileInfo.Mode().IsRegular() {
		t.Fatalf("synthetic auth file mode = %v, %v", fileInfo, err)
	}
	if contents, err := os.ReadFile(authFile); err != nil || !bytes.Contains(contents, []byte(canary)) {
		t.Fatalf("synthetic auth file omitted its credential canary: %v", err)
	}
	return authFile
}

func syntheticBoundaryJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + ".boundary"
}

func writeHostileProjectCanaries(workspace string) error {
	rules := filepath.Join(workspace, ".codex", "rules")
	if err := os.MkdirAll(rules, 0o700); err != nil {
		return err
	}
	files := map[string]string{
		filepath.Join(workspace, "AGENTS.md"):             hostileProjectCanary + "\n",
		filepath.Join(workspace, ".codex", "config.toml"): hostileProjectCanary + " = [\n",
		filepath.Join(rules, "hostile.rules"):             hostileProjectCanary + " is not valid execpolicy\n",
	}
	for path, contents := range files {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func digestBoundaryFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func randomBoundaryCanary(t *testing.T, prefix string) string {
	t.Helper()
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	return prefix + hex.EncodeToString(random[:])
}

func redactBoundarySecrets(contents []byte, credentials ...string) []byte {
	redacted := bytes.Clone(contents)
	for _, credential := range credentials {
		redacted = bytes.ReplaceAll(redacted, []byte(credential), []byte("[REDACTED]"))
	}
	return redacted
}

func containsBoundarySecret(contents []byte, credentials ...string) bool {
	for _, credential := range credentials {
		if bytes.Contains(contents, []byte(credential)) {
			return true
		}
	}
	return false
}

func assertNoVisibleProcContains(prefixes ...string) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	visible := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		for _, name := range []string{"cmdline", "environ"} {
			contents, err := os.ReadFile(filepath.Join("/proc", entry.Name(), name))
			if err != nil {
				if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
					continue
				}
				return err
			}
			visible++
			for _, prefix := range prefixes {
				if bytes.Contains(contents, []byte(prefix)) {
					return fmt.Errorf("credential canary appears in visible process %s %s", entry.Name(), name)
				}
			}
		}
	}
	if visible == 0 {
		return errors.New("no process command lines were visible")
	}
	return nil
}

func assertCredentialCanaryUnreadable(prefix string) error {
	paths := []string{
		executor.CredentialFileTarget,
		"/proc/self/root" + executor.CredentialFileTarget,
		"/proc/thread-self/root" + executor.CredentialFileTarget,
		"/proc/1/root" + executor.CredentialFileTarget,
	}
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, process := range processes {
		if !process.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(process.Name()); err != nil {
			continue
		}
		processRoot := filepath.Join("/proc", process.Name())
		paths = append(paths,
			filepath.Join(processRoot, "root")+executor.CredentialFileTarget,
			filepath.Join(processRoot, "cwd")+"/../home/sworn/.codex/auth.json",
			filepath.Join(processRoot, "cwd")+"/../../home/sworn/.codex/auth.json",
		)
		fileDescriptors, err := os.ReadDir(filepath.Join(processRoot, "fd"))
		if err != nil {
			continue
		}
		for _, descriptor := range fileDescriptors {
			paths = append(paths, filepath.Join(processRoot, "fd", descriptor.Name()))
		}
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 1<<20 {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if bytes.Contains(contents, []byte(prefix)) {
			return fmt.Errorf("credential canary was readable through %s", path)
		}
	}
	return nil
}

func assertNoOpenSocketFD() error {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if strings.HasPrefix(target, "socket:[") {
			return fmt.Errorf("socket file descriptor %s was already open", entry.Name())
		}
	}
	return nil
}

func assertBoundaryRootEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("executor residue in %s: %s", root, strings.Join(names, ", "))
	}
}

func boundaryArgumentIndex(arguments []string, target string) int {
	for index, argument := range arguments {
		if argument == target {
			return index
		}
	}
	return -1
}
