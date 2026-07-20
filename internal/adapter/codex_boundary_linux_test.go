//go:build linux

package adapter

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
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
)

const (
	requireCodexBoundaryEnvironment = "SWORN_REQUIRE_CODEX_BOUNDARY"
	codexBinaryEnvironment          = "SWORN_CODEX_BINARY"
	executorShimSentinel            = "__sworn_codex_boundary_executor_shim__"
	nestedProbeSentinel             = "__sworn_codex_boundary_nested_probe__"
	credentialCanaryPrefix          = "SWORN_CREDENTIAL_CANARY_"
	codexBoundaryModel              = "gpt-5.4"
	codexBoundaryCallID             = "sworn-boundary-call"
	hostileProjectCanary            = "SWORN_HOSTILE_PROJECT_CONFIG_CANARY_55fd71cc"
	proofContents                   = "nested-codex-sandbox-contained\n"
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
	credentialCanary := randomBoundaryCredential(t)
	executorInstance, runtimeRoot, writableRoot := requireCodexBoundaryExecutor(t, required)
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
	var controlPlaneRequests atomic.Int32
	var controlPlaneFailure atomic.Value
	controlPlane := httptest.NewServer(newCodexResponsesHandler(
		hostCanary,
		credentialCanary,
		&controlPlaneRequests,
		&controlPlaneFailure,
	))
	t.Cleanup(controlPlane.Close)

	contextWithDeadline, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completion, err := executorInstance.RunWritable(contextWithDeadline, executor.Invocation{
		SchemaVersion:   executor.InvocationSchemaVersion,
		ID:              "real-codex-boundary",
		Role:            "builder",
		NestedSandbox:   true,
		Workspace:       sourceWorkspace,
		WorkspaceDigest: workspaceDigest,
		WorkspaceAccess: executor.WorkspaceWritableExport,
		ExecutableInput: "codex",
		Inputs: []executor.Input{{
			Name:   "codex",
			Path:   codexBinary,
			Digest: codexDigest,
		}, {
			Name:   "probe",
			Path:   testBinary,
			Digest: probeDigest,
		}},
		Argv: []string{
			"/inputs/codex",
			"-a", "never",
			"-s", "workspace-write",
			"-m", codexBoundaryModel,
			"-c", `model_provider="sworn_boundary"`,
			"-c", "model_providers.sworn_boundary=" + `{name="Sworn boundary",base_url=` +
				strconv.Quote(controlPlane.URL+"/v1") + `,env_key="CODEX_API_KEY",wire_api="responses",supports_websockets=false}`,
			"-c", `web_search="disabled"`,
			"-c", `sandbox_workspace_write.network_access=false`,
			"-c", `shell_environment_policy.inherit="none"`,
			"-c", `shell_environment_policy.set={PATH="/usr/bin:/bin",HOME="/home/sworn"}`,
			"-c", `allow_login_shell=false`,
			"-c", `history.persistence="none"`,
			"-c", `check_for_update_on_startup=false`,
			"-c", `features.enable_request_compression=false`,
			"-c", `features.apps=false`,
			"-c", `features.goals=false`,
			"-c", `features.hooks=false`,
			"-c", `features.memories=false`,
			"-c", `features.multi_agent=false`,
			"-c", `features.remote_plugin=false`,
			"-c", `features.shell_snapshot=false`,
			"-c", `features.skill_mcp_dependency_install=false`,
			"-C", "/tmp",
			"exec",
			"--strict-config",
			"--ephemeral",
			"--ignore-user-config",
			"--ignore-rules",
			"--skip-git-repo-check",
			"--json",
			"--add-dir", "/workspace",
			"Run the boundary probe exactly once with exec_command, then finish.",
		},
		Environment: map[string]string{"CODEX_API_KEY": credentialCanary},
		Network:     executor.NetworkHost,
		Timeout:     25 * time.Second,
	})
	if err != nil {
		t.Fatalf(
			"run real Codex boundary: %v; exit=%d stdout=%q stderr=%q",
			err,
			completion.ExitCode,
			redactBoundarySecret(completion.Stdout, credentialCanary),
			redactBoundarySecret(completion.Stderr, credentialCanary),
		)
	}
	if completion.ExitCode != 0 || completion.Cancelled || completion.TimedOut || completion.OutputTruncated {
		t.Fatalf(
			"real Codex boundary exit=%d cancelled=%t timed_out=%t truncated=%t stdout=%q stderr=%q",
			completion.ExitCode,
			completion.Cancelled,
			completion.TimedOut,
			completion.OutputTruncated,
			redactBoundarySecret(completion.Stdout, credentialCanary),
			redactBoundarySecret(completion.Stderr, credentialCanary),
		)
	}
	if bytes.Contains(completion.Stdout, []byte(credentialCanary)) ||
		bytes.Contains(completion.Stderr, []byte(credentialCanary)) {
		t.Fatal("credential sentinel appeared in successful Codex output")
	}
	if completion.ExecutableInput != "codex" || len(completion.Inputs) != 2 {
		t.Fatalf("executable binding = %q, inputs=%#v", completion.ExecutableInput, completion.Inputs)
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
			redactBoundarySecret(completion.Stdout, credentialCanary),
			redactBoundarySecret(completion.Stderr, credentialCanary),
		)
	}
	version := boundaryCodexVersion(completion.Stdout)
	if version == "" {
		t.Fatalf("Codex version absent from successful tool output: %q", redactBoundarySecret(completion.Stdout, credentialCanary))
	}
	t.Logf("real Codex boundary binary: %s, %s", version, codexDigest)
	if failure := controlPlaneFailure.Load(); failure != nil {
		t.Fatalf("Responses control-plane validation: %s", failure.(string))
	}
	if got := controlPlaneRequests.Load(); got != 2 {
		t.Fatalf("Responses control-plane requests = %d, want model call plus tool output", got)
	}
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
	credentialCanary string,
	requests *atomic.Int32,
	failure *atomic.Value,
) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestNumber := requests.Add(1)
		fail := func(format string, arguments ...any) {
			message := fmt.Sprintf(format, arguments...)
			failure.Store(message)
			http.Error(writer, message, http.StatusBadRequest)
		}
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			fail("unexpected Responses request %s %s", request.Method, request.URL.Path)
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+credentialCanary {
			fail("Responses request did not carry the exact sentinel credential")
			return
		}
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
		if bytes.Contains(body, []byte(credentialCanary)) {
			fail("credential sentinel reached the model request body")
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.Model != codexBoundaryModel {
			fail("Responses request model=%q decode=%v", payload.Model, err)
			return
		}
		wantTools := []string{
			"custom:apply_patch",
			"function:exec_command",
			"function:request_user_input",
			"function:update_plan",
			"function:view_image",
			"function:write_stdin",
		}
		if got := boundaryToolNames(body); strings.Join(got, "\x00") != strings.Join(wantTools, "\x00") {
			fail("model tool allowlist=%v, want %v", got, wantTools)
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
		_, _ = fmt.Fprintln(stderr, "nested Codex boundary probe requires control plane and host canary")
		return 2
	}
	if _, present := os.LookupEnv("CODEX_API_KEY"); present {
		_, _ = fmt.Fprintln(stderr, "nested tool inherited CODEX_API_KEY")
		return 1
	}
	for _, value := range os.Environ() {
		if strings.Contains(value, credentialCanaryPrefix) {
			_, _ = fmt.Fprintln(stderr, "nested tool environment exposed the API-key sentinel")
			return 1
		}
	}
	if err := assertNoVisibleProcContains(credentialCanaryPrefix); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested /proc boundary: %v\n", err)
		return 1
	}
	if err := assertNoOpenSocketFD(); err != nil {
		_, _ = fmt.Fprintf(stderr, "nested socket boundary: %v\n", err)
		return 1
	}
	client := &http.Client{Timeout: 750 * time.Millisecond}
	if response, err := client.Get(arguments[0]); err == nil {
		_ = response.Body.Close()
		_, _ = fmt.Fprintln(stderr, "nested tool reached the outer control plane")
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
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		codexBoundaryUnavailable(t, required, "%s must name a clean absolute path", codexBinaryEnvironment)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		codexBoundaryUnavailable(t, required, "%s must name the exact real binary: resolved=%q error=%v", codexBinaryEnvironment, resolved, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		codexBoundaryUnavailable(t, required, "%s is not an executable regular file: %v", codexBinaryEnvironment, err)
	}
	binary, err := elf.Open(path)
	if err != nil {
		codexBoundaryUnavailable(t, required, "%s is not an ELF binary: %v", codexBinaryEnvironment, err)
	}
	defer binary.Close() //nolint:errcheck
	if binary.Type != elf.ET_DYN {
		codexBoundaryUnavailable(t, required, "%s must name a static PIE executable", codexBinaryEnvironment)
	}
	for _, program := range binary.Progs {
		if program.Type == elf.PT_INTERP {
			codexBoundaryUnavailable(t, required, "%s must not depend on an ELF interpreter", codexBinaryEnvironment)
		}
	}
	return path
}

func requireCodexBoundaryExecutor(t *testing.T, required bool) (*executor.LinuxExecutor, string, string) {
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
		RuntimeRoot:        runtimeRoot,
		WritableRoot:       writableRoot,
		ShimArgv:           []string{testBinary, "-test.run=^TestCodexBoundaryExecutorShimProcess$", "--", executorShimSentinel},
		Limits:             limits,
		AllowedEnvironment: []string{"CODEX_API_KEY"},
		AllowHostNetwork:   true,
		AllowNestedSandbox: true,
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

func randomBoundaryCredential(t *testing.T) string {
	t.Helper()
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	return credentialCanaryPrefix + hex.EncodeToString(random[:])
}

func redactBoundarySecret(contents []byte, credential string) []byte {
	return bytes.ReplaceAll(contents, []byte(credential), []byte("[REDACTED]"))
}

func assertNoVisibleProcContains(prefix string) error {
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
			if bytes.Contains(contents, []byte(prefix)) {
				return fmt.Errorf("credential canary appears in visible process %s %s", entry.Name(), name)
			}
		}
	}
	if visible == 0 {
		return errors.New("no process command lines were visible")
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
