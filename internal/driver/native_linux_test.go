//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	exactCodexBinary  = "/home/brad/.nvm/versions/node/v24.14.0/lib/node_modules/@openai/codex/node_modules/@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex"
	exactClaudeBinary = "/home/brad/snap/code/253/.local/share/claude/versions/2.1.208"
)

var (
	nativeProbeOnce   sync.Once
	nativeProbeBinary string
	nativeProbeError  error
)

func TestExactNativeProfilesRejectUnboundCertificationCallbacks(t *testing.T) {
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			config := exactNativeConfigFixture(t, family)
			ref := string(family) + "-credential"
			var probed SelectedProfile
			adapterValue, err := NewNativeAdapter(
				config,
				func(context.Context, string) (string, error) {
					return "/not-used-by-readiness", nil
				},
				func(
					_ context.Context,
					selected SelectedProfile,
				) (Invocation, error) {
					probed = selected
					return Invocation{}, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			adapter := adapterValue.(*nativeAdapter)
			profile := ProfileConfig{
				Key: string(family) + "-profile", Adapter: config.Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}
			if state, code := adapter.checkProfile(
				context.Background(),
				checkInspect,
				profile,
				"exact-native-model",
			); state != ReadinessPass || code != "native_closure_exact" {
				t.Fatalf("inspect = %s %s", state, code)
			}
			if state, code := adapter.checkProfile(
				context.Background(),
				checkDoctor,
				profile,
				"exact-native-model",
			); state != ReadinessPass || code != "native_binary_ready" {
				t.Fatalf("doctor = %s %s", state, code)
			}
			if state, code := adapter.checkProfile(
				context.Background(),
				checkCertify,
				profile,
				"exact-native-model",
			); state != ReadinessFail || code != "native_smoke_invalid" ||
				probed.Profile.CredentialRef == nil ||
				*probed.Profile.CredentialRef != ref ||
				probed.Model != "exact-native-model" ||
				len(adapter.certified) != 0 {
				t.Fatalf(
					"certify = %s %s, probe=%#v, certificates=%d",
					state,
					code,
					probed,
					len(adapter.certified),
				)
			}
			registry, err := NewSelectionRegistry(
				[]ProfileConfig{profile},
				[]Adapter{adapter},
			)
			if err != nil {
				t.Fatal(err)
			}
			report := registry.Inspect(
				context.Background(),
				profile.Key,
				"exact-native-model",
			)
			body, _ := json.Marshal(report)
			for _, forbidden := range []string{
				config.CLI.Path, ref, config.CredentialTarget,
			} {
				if bytes.Contains(body, []byte(forbidden)) {
					t.Fatalf("readiness report leaked %q: %s", forbidden, body)
				}
			}
		})
	}
}

func TestNativeCommandSurfacesAreExactAndCapabilityIsSingleSeam(t *testing.T) {
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			config := exactNativeConfigFixture(t, family)
			invocation, _, _ := memoryInvocationFixture(t)
			invocation.Selected.Model = "exact-native-model"
			invocation.Request.FreshContext = true
			session, err := newToolSession(invocation)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			broker, err := newNativeBroker(session)
			if err != nil {
				t.Fatal(err)
			}
			defer broker.Close()
			capability := broker.capability()
			defer clearBytes(capability)
			credentialPath := filepath.Join(t.TempDir(), "credential")
			const credentialCanary = "native-credential-canary"
			if err := os.WriteFile(
				credentialPath,
				[]byte(credentialCanary),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			credential, err := acquireFileCredential(
				credentialPath,
				invocation.HostWorkspace,
				config.MaxCredentialBytes,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer credential.Close()
			closure, err := openNativeClosure(config)
			if err != nil {
				t.Fatal(err)
			}
			defer closeNativeFiles(closure)
			configFiles, err := nativeConfigFiles(
				config,
				invocation,
				broker.URL(),
				capability,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer closeNativeFiles(configFiles)
			arguments, environment, extraFiles, err := nativeCommand(
				config,
				invocation,
				credential,
				closure,
				configFiles,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer clearEnvironment(environment)
			argumentBody := []byte(strings.Join(arguments, "\x00"))
			environmentBody := bytes.Join(environment, []byte{0})
			if bytes.Contains(argumentBody, capability) ||
				bytes.Contains(argumentBody, []byte(credentialCanary)) ||
				bytes.Contains(argumentBody, []byte(config.CLI.Path)) ||
				bytes.Contains(argumentBody, []byte(invocation.HostWorkspace)) ||
				!slicesContain(arguments, "--die-with-parent") ||
				!slicesContain(arguments, "--share-net") ||
				len(extraFiles) != len(config.RuntimeFiles)+4 {
				t.Fatalf(
					"native surface = args %q env %q extra=%d",
					arguments,
					environment,
					len(extraFiles),
				)
			}
			for index := 0; index < len(arguments)-1; index++ {
				if arguments[index] == "--ro-bind" &&
					(arguments[index+1] == "/usr" ||
						arguments[index+1] == "/bin") {
					t.Fatalf("host toolchain bind = %q", arguments[index:index+2])
				}
			}
			mcpBody := readOpenFile(t, configFiles[0])
			catalogBody := readOpenFile(t, configFiles[1])
			switch family {
			case ProfileCodex:
				if bytes.Contains(environmentBody, capability) ||
					bytes.Count(mcpBody, capability) != 1 ||
					slicesContain(arguments, "--output-schema") ||
					slicesContain(arguments, "-o") ||
					slicesContain(arguments, "-c") ||
					!containsArgumentSequence(arguments, codexArguments(
						invocation.Selected.Model,
						true,
					)) ||
					!bytes.Contains(catalogBody, []byte(`"shell_type":"disabled"`)) ||
					!bytes.Contains(catalogBody, []byte(`"input_modalities":["text"]`)) ||
					!bytes.Contains(catalogBody, []byte(`"supports_search_tool":false`)) {
					t.Fatalf(
						"Codex surface = args %q env %q mcp=%s catalog=%s",
						arguments,
						environment,
						mcpBody,
						catalogBody,
					)
				}
			case ProfileClaude:
				if bytes.Contains(environmentBody, capability) ||
					bytes.Count(mcpBody, capability) != 1 ||
					!bytes.Contains(mcpBody, []byte(`"alwaysLoad":true`)) ||
					slicesContain(arguments, "--json-schema") ||
					!containsArgumentSequence(
						arguments,
						claudeArguments(
							invocation.Selected.Model,
							invocation.Request.Workspace.Access,
						),
					) {
					t.Fatalf(
						"Claude surface = args %q env %q mcp=%s",
						arguments,
						environment,
						mcpBody,
					)
				}
			}
			clearBytes(mcpBody)
			clearBytes(catalogBody)
		})
	}
}

func TestNativeRuntimeCertificationInspectsLiveNamespaceAndDescriptors(t *testing.T) {
	probe := buildNativeProbe(t)
	digest, err := executableDigest(probe)
	if err != nil {
		t.Fatal(err)
	}
	config := exactNativeConfigFixture(t, ProfileCodex)
	config.CLI = ExecutableIdentity{Path: probe, Digest: digest}
	config.CLIVersion = "test"
	config.VersionOutput = "test"
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.Selected.Model = "native-probe-model"
	invocation.Request.Limits.TimeoutMillis = 300
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(
		credentialPath,
		[]byte(`{"token":"namespace-canary"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	observation, err := platformInvokeNative(
		context.Background(),
		invocation,
		config,
		credentialPath,
		nativeCertificateFixture(invocation, config),
	)
	if !errors.Is(err, context.DeadlineExceeded) ||
		observation.Handoff != nil ||
		time.Since(started) > 3*time.Second {
		t.Fatalf(
			"runtime certification result = %#v, elapsed=%s, error=%v",
			observation,
			time.Since(started),
			err,
		)
	}
}

func TestExactNativeCLIsEmitClosedFirstProviderRequest(t *testing.T) {
	models := map[ProfileFamily]string{
		ProfileCodex:  "sworn-capture-model",
		ProfileClaude: "claude-sonnet-4-20250514",
	}
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			config := exactNativeConfigFixture(t, family)
			invocation, _, _ := memoryInvocationFixture(t)
			invocation.Selected.Model = models[family]
			invocation.Request.Model = models[family]
			invocation.Request.Limits.TimeoutMillis = 20_000
			permission, err := NewSubmissionPermission(
				invocation.Request,
				invocation.Selected,
				ContainmentReadWrite,
				PlannerProposal,
			)
			if err != nil {
				t.Fatal(err)
			}
			invocation.Permission = permission
			certificate, err := platformCaptureNativeSurface(
				context.Background(),
				invocation,
				config,
			)
			if err != nil {
				t.Fatalf("capture = %v", err)
			}
			if err := validateNativeSurfaceCertificate(
				certificate,
				invocation,
				config,
			); err != nil ||
				certificate.CaptureEvidenceDigest == "" ||
				certificate.ToolDigest != nativeToolSurfaceDigest(ReadWrite) {
				t.Fatalf("certificate = %#v, error=%v", certificate, err)
			}
		})
	}
}

func TestCodexFirstProviderRequestRejectsToolSurfaceMutation(t *testing.T) {
	const model = "sworn-capture-model"
	validate := func(t *testing.T, request map[string]any) error {
		t.Helper()
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		defer clearBytes(body)
		_, err = validateNativeProviderRequest(
			body,
			ProfileCodex,
			model,
			ReadWrite,
		)
		return err
	}
	if err := validate(
		t,
		codexFirstProviderRequestFixture(t, model, ReadWrite),
	); err != nil {
		t.Fatalf("exact surface = %v", err)
	}
	t.Run("additional tool", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		request["tools"] = append(request["tools"].([]any), map[string]any{
			"type": "function", "name": "shell",
			"description": "ambient shell", "strict": false,
			"parameters": map[string]any{
				"type": "object", "properties": map[string]any{},
			},
		})
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("additional tool error = %v", err)
		}
	})
	t.Run("parallel calls", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		request["parallel_tool_calls"] = true
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("parallel calls error = %v", err)
		}
	})
	t.Run("missing broker tool", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		for _, raw := range request["tools"].([]any) {
			tool := raw.(map[string]any)
			if tool["type"] == "namespace" {
				tools := tool["tools"].([]any)
				tool["tools"] = tools[:len(tools)-1]
			}
		}
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("missing broker tool error = %v", err)
		}
	})
	t.Run("changed inert tool", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		for _, raw := range request["tools"].([]any) {
			tool := raw.(map[string]any)
			if tool["name"] == "update_plan" {
				tool["description"] = "changed"
			}
		}
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("changed inert tool error = %v", err)
		}
	})
}

func codexFirstProviderRequestFixture(
	t *testing.T,
	model string,
	access WorkspaceAccess,
) map[string]any {
	t.Helper()
	inert := codexInertProviderTools()
	names := []string{
		"list_mcp_resource_templates",
		"list_mcp_resources",
		"read_mcp_resource",
		"update_plan",
	}
	tools := make([]any, 0, len(names)+1)
	for _, name := range names {
		definition, present := inert[name]
		if !present {
			t.Fatalf("missing inert tool fixture %s", name)
		}
		schema, err := decodeStrict(
			definition.InputSchema,
			MaxToolArgumentBytes,
		)
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, map[string]any{
			"type": "function", "name": name,
			"description": definition.Description,
			"strict":      false,
			"parameters":  schema,
		})
	}
	namespaceTools := make([]any, 0, len(toolDefinitions(access)))
	for _, definition := range toolDefinitions(access) {
		schema, err := decodeStrict(
			definition.InputSchema,
			MaxToolArgumentBytes,
		)
		if err != nil {
			t.Fatal(err)
		}
		normalizeCodexProviderSchema(schema)
		namespaceTools = append(namespaceTools, map[string]any{
			"type": "function", "name": definition.Name,
			"description": definition.Description,
			"strict":      false,
			"parameters":  schema,
		})
	}
	tools = append(tools, map[string]any{
		"type": "namespace", "name": "mcp__sworn",
		"description": "Tools in the mcp__sworn namespace.",
		"tools":       namespaceTools,
	})
	return map[string]any{
		"model":               model,
		"tools":               tools,
		"tool_choice":         "auto",
		"parallel_tool_calls": false,
	}
}

func TestNativeInitializationCaptureRejectsAmbientCapabilities(t *testing.T) {
	t.Run("Claude", func(t *testing.T) {
		invocation, _, _ := memoryInvocationFixture(t)
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		broker, err := newNativeBroker(session)
		if err != nil {
			t.Fatal(err)
		}
		defer broker.Close()
		tools := make([]string, 0)
		for _, definition := range toolDefinitions(ReadWrite) {
			tools = append(tools, "mcp__sworn__"+definition.Name)
		}
		event := map[string]any{
			"type": "system", "subtype": "init",
			"model": "exact-native-model", "permissionMode": "dontAsk",
			"slash_commands": []any{}, "skills": []any{}, "plugins": []any{},
			"tools": tools, "capabilities": []any{
				"interrupt_receipt_v1", "msg_lifecycle_v1",
			},
			"analytics_disabled":        true,
			"product_feedback_disabled": true,
			"mcp_servers": []any{map[string]any{
				"name": "sworn", "status": "connected",
			}},
		}
		state := &nativeEventState{
			family: ProfileClaude, model: "exact-native-model",
			access: ReadWrite, broker: broker,
		}
		body, _ := json.Marshal(event)
		capability := broker.capability()
		defer clearBytes(capability)
		if err := completeBrokerHandshake(
			t,
			broker,
			capability,
			"claude-code",
			ClaudeCLIVersion,
		); err != nil {
			t.Fatal(err)
		}
		if broker.Ready() {
			t.Fatal("broker opened before Claude init event")
		}
		if err := state.accept(body); err != nil || !state.nativeSeen ||
			!broker.Ready() {
			t.Fatalf("Claude init = %v, state=%#v", err, state)
		}
		ambient := make([]any, len(tools), len(tools)+1)
		for index, name := range tools {
			ambient[index] = name
		}
		ambient = append(ambient, "StructuredOutput")
		if exactClaudeTools(ambient, ReadWrite) {
			t.Fatal("competing StructuredOutput tool was accepted")
		}
	})

	t.Run("Codex", func(t *testing.T) {
		invocation, _, _ := memoryInvocationFixture(t)
		session, err := newToolSession(invocation)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		broker, err := newNativeBroker(session)
		if err != nil {
			t.Fatal(err)
		}
		defer broker.Close()
		state := &nativeEventState{
			family: ProfileCodex, model: "exact-native-model",
			access: ReadWrite, broker: broker,
		}
		if err := state.accept([]byte(
			`{"type":"thread.started","thread_id":"thread-1"}`,
		)); err != nil || !state.nativeSeen || broker.Ready() {
			t.Fatalf("Codex init = %v, state=%#v", err, state)
		}
		capability := broker.capability()
		defer clearBytes(capability)
		if err := completeBrokerHandshake(
			t,
			broker,
			capability,
			"codex",
			CodexCLIVersion,
		); err != nil {
			t.Fatal(err)
		}
		if !broker.Ready() {
			t.Fatal("broker did not open after exact Codex event and handshake")
		}
		if err := state.accept([]byte(
			`{"type":"item.started","item":{"type":"command_execution"}}`,
		)); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("ambient Codex shell error = %v", err)
		}
	})
}

func nativeCertificateFixture(
	invocation Invocation,
	config NativeAdapterConfig,
) nativeSurfaceCertificate {
	digest := "sha256:" + strings.Repeat("a", 64)
	return nativeSurfaceCertificate{
		Family:                config.Family,
		ProfileDigest:         nativeProfileDigest(invocation.Selected.Profile),
		Model:                 invocation.Selected.Model,
		AdapterConfigDigest:   invocation.Selected.Adapter.ConfigurationDigest,
		ExecutableDigest:      config.CLI.Digest,
		CLIVersion:            config.CLIVersion,
		ToolDigest:            nativeToolSurfaceDigest(ReadWrite),
		CaptureEvidenceDigest: digest,
		Protocol:              "2025-06-18",
		ClientName:            "codex",
		ClientVersion:         CodexCLIVersion,
		InitializeDigest:      digest,
		NotificationDigest:    digest,
		ListDigest:            digest,
	}
}

func completeBrokerHandshake(
	t *testing.T,
	broker *nativeBroker,
	capability []byte,
	clientName string,
	clientVersion string,
) error {
	t.Helper()
	status, body := brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name": clientName, "version": clientVersion,
				},
			},
		},
	)
	clearBytes(body)
	if status != http.StatusOK {
		return fail("INVALID_BROKER")
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "method": "notifications/initialized",
			"params": map[string]any{},
		},
	)
	clearBytes(body)
	if status != http.StatusAccepted {
		return fail("INVALID_BROKER")
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "tools/list",
			"params": map[string]any{},
		},
	)
	clearBytes(body)
	if status != http.StatusOK {
		return fail("INVALID_BROKER")
	}
	return nil
}

func TestSecretGuardFindsCapabilityAcrossWriteBoundaries(t *testing.T) {
	t.Parallel()
	capability := []byte("capability-canary")
	guard := newSecretGuard(capability, 1_024)
	_, _ = guard.Write([]byte("prefix-capability-"))
	_, _ = guard.Write([]byte("canary-suffix"))
	if !guard.leaked() {
		t.Fatal("split capability was not detected")
	}
}

func exactNativeConfigFixture(
	t *testing.T,
	family ProfileFamily,
) NativeAdapterConfig {
	t.Helper()
	var pathValue, digest, version, versionOutput, target, key, id string
	switch family {
	case ProfileCodex:
		pathValue, digest = exactCodexBinary, CodexCLIDigest
		version, versionOutput = CodexCLIVersion, "codex-cli "+CodexCLIVersion
		target, key, id = CodexCredentialTarget, "codex-adapter", "sworn.codex"
	case ProfileClaude:
		pathValue, digest = exactClaudeBinary, ClaudeCLIDigest
		version, versionOutput = ClaudeCLIVersion, ClaudeCLIVersion+" (Claude Code)"
		target, key, id = ClaudeCredentialTarget, "claude-adapter", "sworn.claude"
	default:
		t.Fatalf("unknown native family %s", family)
	}
	if _, err := os.Stat(pathValue); err != nil {
		t.Skipf("exact %s fixture unavailable: %v", family, err)
	}
	runtimeFiles := systemRuntimeFiles(t)
	if family == ProfileClaude {
		for _, runtimeTarget := range []string{
			"/lib64/ld-linux-x86-64.so.2",
			"/lib/x86_64-linux-gnu/librt.so.1",
			"/lib/x86_64-linux-gnu/libc.so.6",
			"/lib/x86_64-linux-gnu/libpthread.so.0",
			"/lib/x86_64-linux-gnu/libdl.so.2",
			"/lib/x86_64-linux-gnu/libm.so.6",
		} {
			runtimeFiles = append(
				runtimeFiles,
				pinnedRuntimeFile(t, runtimeTarget, runtimeTarget),
			)
		}
	}
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	return NativeAdapterConfig{
		Key: key, ID: id, Version: "1.0.0", Family: family,
		CLI:        ExecutableIdentity{Path: pathValue, Digest: digest},
		CLIVersion: version, VersionOutput: versionOutput,
		RuntimeFiles: runtimeFiles, RequiredRuntimeTargets: required,
		CredentialTarget:   target,
		CredentialRefs:     []string{string(family) + "-credential"},
		MaxCredentialBytes: 1_048_576,
	}
}

func systemRuntimeFiles(t *testing.T) []PinnedRuntimeFile {
	t.Helper()
	files := make([]PinnedRuntimeFile, 0, 4)
	for _, target := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	} {
		files = append(files, pinnedRuntimeFile(t, target, target))
	}
	return files
}

func pinnedRuntimeFile(
	t *testing.T,
	source string,
	target string,
) PinnedRuntimeFile {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := executableDigest(resolved)
	if err != nil {
		t.Fatal(err)
	}
	return PinnedRuntimeFile{Path: resolved, Target: target, Digest: digest}
}

func readOpenFile(t *testing.T, file *os.File) []byte {
	t.Helper()
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	body, err := ioReadAllBounded(file, 65_536)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return body
}

func containsArgumentSequence(arguments, expected []string) bool {
	if len(expected) > len(arguments) {
		return false
	}
	for start := 0; start <= len(arguments)-len(expected); start++ {
		matches := true
		for index := range expected {
			matches = matches && arguments[start+index] == expected[index]
		}
		if matches {
			return true
		}
	}
	return false
}

func buildNativeProbe(t *testing.T) string {
	t.Helper()
	nativeProbeOnce.Do(func() {
		directory, err := os.MkdirTemp("", "sworn-native-probe-")
		if err != nil {
			nativeProbeError = err
			return
		}
		nativeProbeBinary = filepath.Join(directory, "native-probe")
		command := exec.Command(
			"go",
			"build",
			"-o",
			nativeProbeBinary,
			"./testdata/nativeprobe",
		)
		command.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
		output, err := command.CombinedOutput()
		if err != nil {
			nativeProbeError = &buildFailure{output: string(output)}
		}
	})
	if nativeProbeError != nil {
		t.Fatal(nativeProbeError)
	}
	return nativeProbeBinary
}
