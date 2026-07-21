//go:build linux

package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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
	verifierBoundaryNestedProbeSentinel = "__sworn_codex_verifier_boundary_nested_probe__"
	verifierBoundaryCallID              = "sworn-verifier-boundary-call"
	verifierBoundaryProof               = "native-codex-verifier-readonly-contained"
	verifierBoundaryCandidateMarker     = "SWORN_VERIFIER_CANDIDATE_VISIBLE_7f984a68"
	verifierBoundaryPlanMarker          = "SWORN_VERIFIER_PLAN_VISIBLE_3602ac92"
	verifierBoundarySubmissionMarker    = "SWORN_VERIFIER_SUBMISSION_VISIBLE_b0494eaa"
	verifierBoundaryDispatchMarker      = "SWORN_VERIFIER_DISPATCH_VISIBLE_e23d50e7"
	verifierBoundaryPolicyMarker        = "SWORN_VERIFIER_POLICY_VISIBLE_0cfe7912"
	verifierBoundaryAuthorityMarker     = "SWORN_VERIFIER_AUTHORITY_VISIBLE_9257a401"
	verifierBoundaryCheckMarker         = "SWORN_VERIFIER_CHECK_VISIBLE_f617c219"
	verifierBoundaryBuilderTranscript   = "SWORN_FORBIDDEN_BUILDER_TRANSCRIPT_0ed52d71"
)

// TestCodexVerifierBoundaryNestedProbeProcess runs as the model-directed
// exec_command child. Assertions here observe the actual nested Codex profile,
// not the trusted parent process or the Go test's host namespace.
func TestCodexVerifierBoundaryNestedProbeProcess(t *testing.T) {
	index := boundaryArgumentIndex(os.Args, verifierBoundaryNestedProbeSentinel)
	if index < 0 {
		return
	}
	os.Exit(runCodexVerifierBoundaryNestedProbe(os.Args[index+1:], os.Stdout, os.Stderr))
}

func TestRealPinnedCodexVerifierBoundary(t *testing.T) {
	required := os.Getenv(requireCodexBoundaryEnvironment) == "1"
	codexBinary := requireExactCodexBinary(t, required)
	providerCanary := randomBoundaryCanary(t, testProviderCanaryPrefix)
	authFileCanary := randomBoundaryCanary(t, authFileCanaryPrefix)
	authFile := writeSyntheticCodexAuthFile(t, authFileCanary)
	executorInstance, runtimeRoot, writableRoot := requireCodexBoundaryExecutor(t, required, authFile)

	workspace := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspace, "candidate.txt"), []byte(verifierBoundaryCandidateMarker+"\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := writeHostileProjectCanaries(workspace); err != nil {
		t.Fatal(err)
	}
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	probePath := filepath.Join(workspace, "sworn-verifier-boundary.test")
	if err := copyVerifierBoundaryExecutable(testBinary, probePath); err != nil {
		t.Fatal(err)
	}
	workspaceDigest, _, err := executor.MeasureWorkspace(
		context.Background(), workspace, executorInstance.EffectiveLimits().InputBytes,
	)
	if err != nil {
		t.Fatal(err)
	}

	builderTranscriptPath := filepath.Join(t.TempDir(), "builder-transcript")
	if err := os.WriteFile(
		builderTranscriptPath, []byte(verifierBoundaryBuilderTranscript+"\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	verifier, err := configureCodexVerifier(CodexVerifierOptions{
		BinaryPath: codexBinary, Model: codexBoundaryModel, Timeout: 25 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := verifier.Profile()
	engineInputs := writeCodexVerifierBoundaryInputs(t, profile.OutputSchemaDigest)
	inputs, err := verifierInputs(profile, engineInputs)
	if err != nil {
		t.Fatal(err)
	}

	var modelDiscoveryRequests atomic.Int32
	var providerRequests atomic.Int32
	var providerFailure atomic.Value
	providerBackend := httptest.NewServer(newCodexVerifierResponsesHandler(
		providerCanary,
		authFileCanary,
		profile.OutputSchemaDigest,
		&modelDiscoveryRequests,
		&providerRequests,
		&providerFailure,
	))
	t.Cleanup(providerBackend.Close)

	argv := codexVerifierBoundaryArgv(t, profile.Argv, providerBackend.URL)
	const invocationID = "real-pinned-codex-verifier-boundary"
	invocation := executor.Invocation{
		SchemaVersion:    executor.InvocationSchemaVersion,
		ID:               invocationID,
		Role:             "verifier",
		NestedSandbox:    true,
		CredentialAccess: true,
		Workspace:        workspace,
		WorkspaceDigest:  workspaceDigest,
		WorkspaceAccess:  executor.WorkspaceReadOnly,
		ExecutableInput:  codexExecutableInput,
		Inputs:           inputs,
		Argv:             argv,
		Environment:      map[string]string{testProviderCredentialEnvironment: providerCanary},
		Network:          executor.NetworkHost,
		Timeout:          25 * time.Second,
	}
	t.Cleanup(func() {
		_, _ = executorInstance.ReconcileContentBound(context.Background(), invocationID)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completion, err := executorInstance.RunCredentialReadOnly(ctx, invocation)
	if err != nil {
		t.Fatalf(
			"run real pinned Codex verifier: %v; exit=%d stdout=%q stderr=%q",
			err,
			completion.ExitCode,
			redactBoundarySecrets(completion.Stdout, providerCanary, authFileCanary),
			redactBoundarySecrets(completion.Stderr, providerCanary, authFileCanary),
		)
	}
	if completion.ExitCode != 0 || completion.Cancelled || completion.TimedOut ||
		completion.OutputTruncated || completion.Export != nil ||
		completion.WorkspaceAccess != executor.WorkspaceReadOnly || !completion.CredentialAccess {
		t.Fatalf("real pinned Codex verifier completion = %#v", completion)
	}
	for _, forbidden := range []string{
		providerCanary, authFileCanary, hostileProjectCanary, verifierBoundaryBuilderTranscript,
	} {
		if bytes.Contains(completion.Stdout, []byte(forbidden)) ||
			bytes.Contains(completion.Stderr, []byte(forbidden)) {
			t.Fatalf("forbidden boundary canary %q appeared in Codex output", forbidden)
		}
	}
	parsed, err := verifier.ParseCompletion(completion)
	if err != nil {
		t.Fatalf(
			"parse real pinned Codex verifier completion: %v; stdout=%q stderr=%q",
			err,
			redactBoundarySecrets(completion.Stdout, providerCanary, authFileCanary),
			redactBoundarySecrets(completion.Stderr, providerCanary, authFileCanary),
		)
	}
	if string(parsed.Assessment) != testCodexVerifierAssessment() || !protocol.ValidID(parsed.ThreadID) {
		t.Fatalf("real Codex verifier assessment=%q thread=%q", parsed.Assessment, parsed.ThreadID)
	}
	if failure := providerFailure.Load(); failure != nil {
		t.Fatalf("verifier Responses test-provider validation: %s", failure.(string))
	}
	if got := modelDiscoveryRequests.Load(); got != 2 {
		t.Fatalf("verifier model-discovery requests = %d, want 2", got)
	}
	if got := providerRequests.Load(); got != 2 {
		t.Fatalf("verifier Responses requests = %d, want model call plus tool output", got)
	}
	if !bytes.Contains(completion.Stdout, []byte(verifierBoundaryProof)) {
		t.Fatalf("nested verifier boundary proof absent from stdout: %q", completion.Stdout)
	}
	if err := assertCodexVerifierBoundaryInputs(completion.Inputs, inputs); err != nil {
		t.Fatal(err)
	}
	for _, input := range completion.Inputs {
		if strings.Contains(input.Name, "builder") || strings.Contains(input.Name, "transcript") {
			t.Fatalf("builder context entered verifier inputs: %#v", completion.Inputs)
		}
	}
	afterDigest, _, err := executor.MeasureWorkspace(
		context.Background(), workspace, executorInstance.EffectiveLimits().InputBytes,
	)
	if err != nil || afterDigest != workspaceDigest {
		t.Fatalf("read-only verifier candidate changed: before=%s after=%s error=%v", workspaceDigest, afterDigest, err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "verifier-forbidden-write")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("verifier wrote its read-only candidate: %v", err)
	}
	if transcript, err := os.ReadFile(builderTranscriptPath); err != nil ||
		string(transcript) != verifierBoundaryBuilderTranscript+"\n" {
		t.Fatalf("host builder transcript changed: %q, %v", transcript, err)
	}
	cleanup, err := executorInstance.ReconcileContentBound(context.Background(), invocationID)
	if err != nil || cleanup.InvocationID() != invocationID {
		t.Fatalf("reconcile real Codex verifier: cleanup=%#v error=%v", cleanup, err)
	}
	assertBoundaryRootEmpty(t, runtimeRoot)
	assertBoundaryRootEmpty(t, writableRoot)
}

func codexVerifierBoundaryArgv(t *testing.T, production []string, backendURL string) []string {
	t.Helper()
	argv := slices.Clone(production)
	configured := false
	for index, argument := range argv {
		if argument != `model_provider="openai"` {
			continue
		}
		argv[index] = `model_provider="sworn_boundary"`
		provider := "model_providers.sworn_boundary=" + `{name="Sworn verifier boundary",base_url=` +
			strconv.Quote(backendURL+"/v1") + `,env_key=` +
			strconv.Quote(testProviderCredentialEnvironment) + `,wire_api="responses",supports_websockets=false}`
		argv = append(argv[:index+1], append([]string{"-c", provider}, argv[index+1:]...)...)
		configured = true
		break
	}
	if !configured {
		t.Fatal("production verifier argv did not expose its fixed provider selector")
	}
	return argv
}

func writeCodexVerifierBoundaryInputs(t *testing.T, schemaDigest string) []executor.Input {
	t.Helper()
	root := t.TempDir()
	schema, err := protocol.VerifierAssessmentOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	if protocol.RawDigest(schema) != schemaDigest {
		t.Fatal("protocol verifier assessment schema digest changed while staging")
	}
	checkBundle := verifierBoundaryJSON(t, map[string]any{
		"schema_version": "sworn-verifier-review-check-v1",
		"marker":         verifierBoundaryCheckMarker,
		"stdout": map[string]string{
			"encoding": "base64", "contents": base64.StdEncoding.EncodeToString([]byte("check stdout\n")),
		},
		"stderr": map[string]string{
			"encoding": "base64", "contents": base64.StdEncoding.EncodeToString([]byte("check stderr\n")),
		},
	})
	contents := map[string][]byte{
		"assessment-schema": schema,
		"dispatch":          verifierBoundaryJSON(t, map[string]string{"marker": verifierBoundaryDispatchMarker}),
		"plan":              verifierBoundaryJSON(t, map[string]string{"marker": verifierBoundaryPlanMarker}),
		"review-authority":  verifierBoundaryJSON(t, map[string]string{"marker": verifierBoundaryAuthorityMarker}),
		"review-check-00":   checkBundle,
		"review-policy":     verifierBoundaryJSON(t, map[string]string{"marker": verifierBoundaryPolicyMarker}),
		"submission":        verifierBoundaryJSON(t, map[string]string{"marker": verifierBoundarySubmissionMarker}),
	}
	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	sort.Strings(names)
	inputs := make([]executor.Input, 0, len(names))
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, contents[name], 0o600); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, executor.Input{
			Name: name, Path: path, Digest: protocol.RawDigest(contents[name]),
		})
	}
	return inputs
}

func verifierBoundaryJSON(t *testing.T, value any) []byte {
	t.Helper()
	contents, err := protocol.EncodeCanonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func newCodexVerifierResponsesHandler(
	providerCanary string,
	authFileCanary string,
	outputSchemaDigest string,
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
			fail("verifier test-provider request omitted the exact provider credential")
			return
		}
		for _, values := range request.Header {
			for _, value := range values {
				if strings.Contains(value, authFileCanary) {
					fail("verifier ChatGPT auth-file canary reached the test provider")
					return
				}
			}
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/models" {
			modelDiscoveryRequests.Add(1)
			body, err := io.ReadAll(io.LimitReader(request.Body, 1))
			if err != nil || len(body) != 0 {
				fail("verifier model-discovery request body=%q error=%v", body, err)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{"models": []any{}})
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			fail("unexpected verifier test-provider request %s %s", request.Method, request.URL.Path)
			return
		}
		requestNumber := requests.Add(1)
		if encoding := request.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
			fail("unsupported verifier Responses encoding %q", encoding)
			return
		}
		const maximumResponsesRequestBytes = 2 << 20
		body, err := io.ReadAll(io.LimitReader(request.Body, maximumResponsesRequestBytes+1))
		if err != nil {
			fail("read verifier Responses request: %v", err)
			return
		}
		if len(body) > maximumResponsesRequestBytes {
			fail("verifier Responses request exceeded %d bytes", maximumResponsesRequestBytes)
			return
		}
		for label, forbidden := range map[string]string{
			"hostile project instructions": hostileProjectCanary,
			"provider credential":          providerCanary,
			"ChatGPT credential":           authFileCanary,
			"builder transcript":           verifierBoundaryBuilderTranscript,
			"builder prompt":               codexBuilderPrompt,
		} {
			if bytes.Contains(body, []byte(forbidden)) {
				fail("%s reached verifier model request", label)
				return
			}
		}
		if err := validateCodexVerifierBoundaryRequest(body, outputSchemaDigest); err != nil {
			fail("validate verifier Responses request: %v", err)
			return
		}
		switch requestNumber {
		case 1:
			if !bytes.Contains(body, []byte(protocol.NativeCodexVerifierPrompt)) {
				fail("initial verifier request omitted the production prompt")
				return
			}
			command := strings.Join([]string{
				shellBoundaryQuote("/workspace/sworn-verifier-boundary.test"),
				shellBoundaryQuote("-test.run=^TestCodexVerifierBoundaryNestedProbeProcess$"),
				"--",
				shellBoundaryQuote(verifierBoundaryNestedProbeSentinel),
				shellBoundaryQuote("http://" + request.Host + "/v1/responses"),
				shellBoundaryQuote(verifierBoundaryCandidateMarker),
				shellBoundaryQuote(verifierBoundaryPlanMarker),
				shellBoundaryQuote(verifierBoundarySubmissionMarker),
				shellBoundaryQuote(verifierBoundaryDispatchMarker),
				shellBoundaryQuote(verifierBoundaryPolicyMarker),
				shellBoundaryQuote(verifierBoundaryAuthorityMarker),
				shellBoundaryQuote(verifierBoundaryCheckMarker),
				shellBoundaryQuote(outputSchemaDigest),
			}, " ")
			arguments, err := json.Marshal(map[string]any{
				"cmd": command, "yield_time_ms": 10_000, "max_output_tokens": 4_000,
			})
			if err != nil {
				fail("encode verifier boundary command: %v", err)
				return
			}
			writeBoundarySSE(writer,
				map[string]any{"type": "response.created", "response": map[string]any{"id": "verifier-response-1"}},
				map[string]any{
					"type": "response.output_item.done",
					"item": map[string]any{
						"type": "function_call", "call_id": verifierBoundaryCallID,
						"name": "exec_command", "arguments": string(arguments),
					},
				},
				boundaryCompletedEvent("verifier-response-1"),
			)
		case 2:
			if !hasCodexVerifierBoundaryFunctionOutput(body) {
				fail("second verifier request omitted the successful nested boundary output")
				return
			}
			writeBoundarySSE(writer,
				map[string]any{"type": "response.created", "response": map[string]any{"id": "verifier-response-2"}},
				map[string]any{
					"type": "response.output_item.done",
					"item": map[string]any{
						"type": "message", "role": "assistant", "id": "verifier-message-2",
						"content": []map[string]any{{"type": "output_text", "text": testCodexVerifierAssessment()}},
					},
				},
				boundaryCompletedEvent("verifier-response-2"),
			)
		default:
			fail("unexpected verifier Responses request number %d", requestNumber)
		}
	})
}

func validateCodexVerifierBoundaryRequest(body []byte, outputSchemaDigest string) error {
	var payload struct {
		Model string `json:"model"`
		Text  struct {
			Format struct {
				Name   string          `json:"name"`
				Type   string          `json:"type"`
				Strict bool            `json:"strict"`
				Schema json.RawMessage `json:"schema"`
			} `json:"format"`
		} `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	if payload.Model != codexBoundaryModel || payload.Text.Format.Name != "codex_output_schema" ||
		payload.Text.Format.Type != "json_schema" || !payload.Text.Format.Strict {
		return fmt.Errorf("model=%q output format=%#v", payload.Model, payload.Text.Format)
	}
	canonical, err := protocol.CanonicalizeJSON(payload.Text.Format.Schema)
	if err != nil {
		return fmt.Errorf("canonicalize provider output schema: %w", err)
	}
	if protocol.RawDigest(canonical) != outputSchemaDigest {
		return errors.New("provider request output schema does not match the protocol schema")
	}
	wantTools := codexBuilderToolNames()
	if got := boundaryToolNames(body); !slices.Equal(got, wantTools) {
		return fmt.Errorf("model tool allowlist=%v, want %v", got, wantTools)
	}
	toolSchemaDigest, err := boundaryToolSchemaDigest(body)
	if err != nil {
		return fmt.Errorf("measure model tool schema: %w", err)
	}
	if toolSchemaDigest != pinnedCodexToolSchemaDigest {
		return fmt.Errorf("model tool schema digest=%q, want %q", toolSchemaDigest, pinnedCodexToolSchemaDigest)
	}
	return nil
}

func hasCodexVerifierBoundaryFunctionOutput(body []byte) bool {
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
		if item.Type == "function_call_output" && item.CallID == verifierBoundaryCallID &&
			bytes.Contains(item.Output, []byte(verifierBoundaryProof)) {
			return true
		}
	}
	return false
}

func runCodexVerifierBoundaryNestedProbe(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) != 9 {
		_, _ = fmt.Fprintf(stderr, "nested verifier boundary requires nine arguments, got %d\n", len(arguments))
		return 2
	}
	if _, present := os.LookupEnv(testProviderCredentialEnvironment); present {
		_, _ = fmt.Fprintln(stderr, "nested verifier tool inherited the provider credential")
		return 1
	}
	for _, value := range os.Environ() {
		if strings.Contains(value, testProviderCanaryPrefix) || strings.Contains(value, authFileCanaryPrefix) {
			_, _ = fmt.Fprintln(stderr, "nested verifier environment exposed a credential sentinel")
			return 1
		}
	}
	if err := assertNoVisibleProcContains(testProviderCanaryPrefix, authFileCanaryPrefix); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested verifier /proc boundary: %v\n", err)
		return 1
	}
	if err := assertCredentialCanaryUnreadable(authFileCanaryPrefix); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested verifier credential boundary: %v\n", err)
		return 1
	}
	if err := assertNoOpenSocketFD(); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested verifier socket boundary: %v\n", err)
		return 1
	}
	client := &http.Client{Timeout: 750 * time.Millisecond}
	if response, err := client.Get(arguments[0]); err == nil {
		_ = response.Body.Close()
		_, _ = fmt.Fprintln(stderr, "nested verifier tool reached the outer provider backend")
		return 1
	}
	wantFiles := map[string]string{
		"/workspace/candidate.txt": arguments[1],
		"/inputs/plan":             arguments[2],
		"/inputs/submission":       arguments[3],
		"/inputs/dispatch":         arguments[4],
		"/inputs/review-policy":    arguments[5],
		"/inputs/review-authority": arguments[6],
		"/inputs/review-check-00":  arguments[7],
	}
	for path, marker := range wantFiles {
		contents, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(contents, []byte(marker)) {
			_, _ = fmt.Fprintf(stderr, "nested verifier input %s omitted its exact marker: %v\n", path, err)
			return 1
		}
	}
	schemaDigest, err := digestBoundaryFile("/inputs/assessment-schema")
	if err != nil || schemaDigest != arguments[8] {
		_, _ = fmt.Fprintf(stderr, "nested verifier assessment schema=%q want=%q error=%v\n", schemaDigest, arguments[8], err)
		return 1
	}
	inputEntries, err := os.ReadDir("/inputs")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "list nested verifier inputs: %v\n", err)
		return 1
	}
	inputNames := make([]string, 0, len(inputEntries))
	for _, entry := range inputEntries {
		inputNames = append(inputNames, entry.Name())
	}
	sort.Strings(inputNames)
	wantInputNames := []string{
		"assessment-schema", "codex", "dispatch", "plan", "review-authority",
		"review-check-00", "review-policy", "submission",
	}
	if !slices.Equal(inputNames, wantInputNames) {
		_, _ = fmt.Fprintf(stderr, "nested verifier input manifest=%v want=%v\n", inputNames, wantInputNames)
		return 1
	}
	for _, path := range []string{
		"/workspace/AGENTS.md",
		"/workspace/.codex/config.toml",
		"/workspace/.codex/rules/hostile.rules",
	} {
		contents, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(contents, []byte(hostileProjectCanary)) {
			_, _ = fmt.Fprintf(
				stderr, "hostile project instructions %s absent from exact candidate: %v\n", path, err,
			)
			return 1
		}
	}
	if _, err := os.Stat("/workspace/.git"); !errors.Is(err, os.ErrNotExist) {
		_, _ = fmt.Fprintf(stderr, "nested verifier candidate exposed Git metadata: %v\n", err)
		return 1
	}
	for _, path := range []string{
		"/workspace/verifier-forbidden-write",
		"/workspace/candidate.txt",
		"/inputs/plan",
		"/inputs/assessment-schema",
		executor.CredentialFileTarget,
	} {
		if err := os.WriteFile(path, []byte("forbidden"), 0o600); err == nil {
			_, _ = fmt.Fprintf(stderr, "nested verifier tool wrote read-only path %s\n", path)
			return 1
		}
	}
	contents, err := os.ReadFile("/workspace/candidate.txt")
	if err != nil || !bytes.Contains(contents, []byte(arguments[1])) {
		_, _ = fmt.Fprintf(stderr, "nested verifier candidate changed after denied write: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, verifierBoundaryProof)
	return 0
}

func assertCodexVerifierBoundaryInputs(
	observed []executor.BoundInput,
	want []executor.Input,
) error {
	if len(observed) != len(want) {
		return fmt.Errorf("verifier bound inputs=%#v, want %d", observed, len(want))
	}
	for index, input := range want {
		info, err := os.Stat(input.Path)
		if err != nil {
			return fmt.Errorf("inspect verifier input %q: %w", input.Name, err)
		}
		if info.Size() < 0 {
			return fmt.Errorf("inspect verifier input %q: negative size", input.Name)
		}
		bound := observed[index]
		if bound.Name != input.Name || bound.Digest != input.Digest || bound.Size != uint64(info.Size()) {
			return fmt.Errorf("verifier bound input %d=%#v, want name=%q digest=%s size=%d", index, bound, input.Name, input.Digest, info.Size())
		}
	}
	return nil
}

func copyVerifierBoundaryExecutable(sourcePath, destinationPath string) (resultErr error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open verifier boundary executable: %w", err)
	}
	defer source.Close() //nolint:errcheck
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o500)
	if err != nil {
		return fmt.Errorf("create verifier boundary executable: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			resultErr = errors.Join(resultErr, destination.Close())
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("copy verifier boundary executable: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close verifier boundary executable: %w", err)
	}
	closed = true
	return nil
}
