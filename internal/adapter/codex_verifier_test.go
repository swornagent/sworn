package adapter

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
)

const testCodexVerifierDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestConfigureCodexVerifierPinsMemorylessReadOnlyProfile(t *testing.T) {
	t.Parallel()
	options := CodexVerifierOptions{
		BinaryPath: "/opt/sworn/codex", Model: "gpt-explicit", Timeout: 4 * time.Minute,
	}
	verifier, err := configureCodexVerifier(options)
	if err != nil {
		t.Fatal(err)
	}
	profile := verifier.Profile()
	if profile.SchemaVersion != protocol.VerifierProfileSchemaVersion ||
		profile.Agent != pinnedCodexVersion || profile.BinaryVersion != pinnedCodexVersion ||
		profile.BinaryPath != options.BinaryPath || profile.BinaryDigest != pinnedCodexDigest ||
		profile.BinarySize != pinnedCodexSize || profile.ExecutableInput != codexExecutableInput ||
		profile.Provider != codexProvider || profile.Authentication != codexAuthentication ||
		profile.CredentialHome != codexHome || profile.ToolSchemaDigest != pinnedCodexToolSchemaDigest ||
		profile.Model != options.Model || profile.TimeoutNanoseconds != options.Timeout.Nanoseconds() {
		t.Fatalf("Codex verifier profile identity = %#v", profile)
	}
	if profile.Network != string(executor.NetworkHost) ||
		profile.WorkspaceAccess != string(executor.WorkspaceReadOnly) ||
		!profile.NestedSandbox || !profile.CredentialAccess ||
		profile.ModelToolNetwork || profile.ModelToolCredentialAccess {
		t.Fatalf("Codex verifier isolation profile = %#v", profile)
	}
	outputSchemaDigest, err := protocol.VerifierAssessmentOutputSchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	if profile.OutputSchemaDigest != outputSchemaDigest ||
		profile.PromptDigest != protocol.RawDigest([]byte(protocol.NativeCodexVerifierPrompt)) ||
		profile.PermissionProfile != codexVerifierPermissionProfile {
		t.Fatalf("Codex verifier schema/prompt profile = %#v", profile)
	}
	if !slices.Equal(profile.Argv, protocol.CanonicalCodexVerifierArgv(options.Model)) {
		t.Fatalf("Codex verifier argv diverged from protocol canonical argv: %#v", profile.Argv)
	}
	for _, sequence := range [][]string{
		{"-a", "never"},
		{"-m", options.Model},
		{"-c", `default_permissions="sworn_verifier"`},
		{"-c", `permissions.sworn_verifier={extends=":read-only",filesystem={"/home/sworn/.codex"="deny"},network={enabled=false}}`},
		{"-c", `history.persistence="none"`},
		{"-c", `project_doc_max_bytes=0`},
		{"-c", `features.memories=false`},
		{"-c", `memories.use_memories=false`},
		{"-c", `memories.generate_memories=false`},
		{"-C", "/tmp", "exec", "--strict-config", "--ephemeral", "--ignore-user-config", "--ignore-rules"},
		{"--json", "--output-schema", "/inputs/assessment-schema", protocol.NativeCodexVerifierPrompt},
	} {
		if !containsCodexArguments(profile.Argv, sequence) {
			t.Fatalf("Codex verifier argv omits %#v: %#v", sequence, profile.Argv)
		}
	}
	for _, forbidden := range []string{
		"--yolo", "--dangerously-bypass-approvals-and-sandbox", "--output-last-message", "-o", "resume", "--add-dir",
	} {
		if slices.Contains(profile.Argv, forbidden) {
			t.Fatalf("Codex verifier argv contains forbidden value %q: %#v", forbidden, profile.Argv)
		}
	}
	if strings.Contains(strings.Join(profile.Argv, "\x00"), `extends=":workspace"`) {
		t.Fatalf("Codex verifier argv widened the model tool workspace: %#v", profile.Argv)
	}
	joined := strings.Join(profile.Argv, "\x00")
	for _, forbidden := range []string{"CODEX_API_KEY", "OPENAI_API_KEY", "base_url", "model_providers."} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("Codex verifier argv contains provider transport or credential %q: %#v", forbidden, profile.Argv)
		}
	}
	if containsCodexArguments(profile.Argv, []string{"-s"}) {
		t.Fatalf("Codex verifier argv overrides the named permission profile: %#v", profile.Argv)
	}

	// The process-neutral worker owns these deployment facts. Closing them here
	// proves the adapter's literal argv satisfies the protocol profile gate.
	profile.ExecutorConfigurationDigest = testCodexVerifierDigest
	profile.RepositoryID = "repository-1"
	profile.WorkspaceRoot = "/var/lib/sworn/verifier"
	profile.MaterializeBytes = 1 << 20
	profile.MaterializeEntries = 100
	if _, err := protocol.EncodeVerifierProfile(profile); err != nil {
		t.Fatalf("encode completed Codex verifier profile: %v", err)
	}

	copyOfProfile := verifier.Profile()
	copyOfProfile.Argv[0] = "/forged"
	if verifier.Profile().Argv[0] != "/inputs/codex" {
		t.Fatal("Codex verifier profile exposed mutable argv")
	}
}

func TestCodexVerifierRequiresExplicitOptions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		options CodexVerifierOptions
		want    string
	}{
		{name: "binary", options: CodexVerifierOptions{Model: "gpt-explicit", Timeout: time.Minute}, want: "binary"},
		{name: "model", options: CodexVerifierOptions{BinaryPath: "/codex", Timeout: time.Minute}, want: "model"},
		{name: "model whitespace", options: CodexVerifierOptions{BinaryPath: "/codex", Model: " gpt", Timeout: time.Minute}, want: "model"},
		{name: "model internal whitespace", options: CodexVerifierOptions{BinaryPath: "/codex", Model: "gpt unsafe", Timeout: time.Minute}, want: "model"},
		{name: "model control", options: CodexVerifierOptions{BinaryPath: "/codex", Model: "gpt\nunsafe", Timeout: time.Minute}, want: "model"},
		{name: "model flag", options: CodexVerifierOptions{BinaryPath: "/codex", Model: "--yolo", Timeout: time.Minute}, want: "model"},
		{name: "timeout", options: CodexVerifierOptions{BinaryPath: "/codex", Model: "gpt-explicit"}, want: "timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := configureCodexVerifier(test.options); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("configureCodexVerifier error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCodexVerifierInputsAreExactAndDeterministic(t *testing.T) {
	t.Parallel()
	verifier := configuredTestCodexVerifier(t)
	profile := verifier.Profile()
	engineInputs := testCodexVerifierEngineInputs(t, profile.OutputSchemaDigest)
	inputs, err := verifierInputs(profile, []executor.Input{
		engineInputs[4], engineInputs[2], engineInputs[6], engineInputs[0], engineInputs[5], engineInputs[3], engineInputs[1],
	})
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		"assessment-schema", "codex", "dispatch", "plan", "review-authority", "review-check-test", "review-policy", "submission",
	}
	gotNames := make([]string, len(inputs))
	for index, input := range inputs {
		gotNames[index] = input.Name
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("Codex verifier input order = %#v, want %#v", gotNames, wantNames)
	}
	if inputs[1].Path != profile.BinaryPath || inputs[1].Digest != pinnedCodexDigest {
		t.Fatalf("Codex verifier binary input = %#v", inputs[1])
	}

	wrongSchema := slices.Clone(engineInputs)
	wrongSchema[0].Digest = testCodexVerifierDigest
	if _, err := verifierInputs(profile, wrongSchema); err == nil || !strings.Contains(err.Error(), "engine-owned schema") {
		t.Fatalf("wrong schema error = %v", err)
	}
	if _, err := verifierInputs(profile, engineInputs[:3]); err == nil {
		t.Fatal("missing engine input was accepted")
	}
	duplicate := slices.Clone(engineInputs)
	duplicate[3] = duplicate[2]
	if _, err := verifierInputs(profile, duplicate); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate input error = %v", err)
	}
	unknown := slices.Clone(engineInputs)
	unknown[5].Name = "unexpected"
	if _, err := verifierInputs(profile, unknown); err == nil || !strings.Contains(err.Error(), "not exact") {
		t.Fatalf("unknown input error = %v", err)
	}
}

func TestCodexVerifierCompletionExtractsOneStrictAssessment(t *testing.T) {
	t.Parallel()
	verifier := configuredTestCodexVerifier(t)
	assessment := testCodexVerifierAssessment()
	stdout := testCodexVerifierJSONL(t, assessment)
	started := time.Date(2026, 7, 22, 1, 2, 3, 0, time.UTC)
	completion := executor.RawCompletion{
		InvocationID:     "verifier-attempt-1",
		WorkspaceDigest:  testCodexVerifierDigest,
		WorkspaceAccess:  executor.WorkspaceReadOnly,
		CredentialAccess: true,
		ExecutableInput:  codexExecutableInput,
		Inputs:           testCodexVerifierBoundInputs(verifier.Profile()),
		StartedAt:        started,
		CompletedAt:      started.Add(time.Second),
		ExitCode:         0,
		Stdout:           stdout,
	}
	parsed, err := verifier.ParseCompletion(completion)
	if err != nil {
		t.Fatal(err)
	}
	if string(parsed.Assessment) != assessment || parsed.ThreadID != "thread-1" {
		t.Fatalf("Codex verifier completion = %#v", parsed)
	}

	for _, mutate := range []func(*executor.RawCompletion){
		func(value *executor.RawCompletion) { value.ExitCode = 1 },
		func(value *executor.RawCompletion) { value.Cancelled = true },
		func(value *executor.RawCompletion) { value.TimedOut = true },
		func(value *executor.RawCompletion) { value.OutputTruncated = true },
		func(value *executor.RawCompletion) { value.WorkspaceAccess = executor.WorkspaceWritableExport },
		func(value *executor.RawCompletion) { value.CredentialAccess = false },
		func(value *executor.RawCompletion) { value.Export = &executor.WorkspaceExport{} },
		func(value *executor.RawCompletion) { value.Inputs[0].Digest = testCodexVerifierDigest },
		func(value *executor.RawCompletion) {
			value.Inputs[0], value.Inputs[1] = value.Inputs[1], value.Inputs[0]
		},
		func(value *executor.RawCompletion) { value.Stdout = testCodexVerifierJSONL(t, `{}`) },
	} {
		changed := completion
		changed.Inputs = slices.Clone(completion.Inputs)
		mutate(&changed)
		if _, err := verifier.ParseCompletion(changed); err == nil {
			t.Fatalf("invalid Codex verifier completion was accepted: %#v", changed)
		}
	}
}

func configuredTestCodexVerifier(t *testing.T) CodexVerifier {
	t.Helper()
	verifier, err := configureCodexVerifier(CodexVerifierOptions{
		BinaryPath: "/opt/sworn/codex", Model: "gpt-explicit", Timeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func testCodexVerifierEngineInputs(t *testing.T, schemaDigest string) []executor.Input {
	t.Helper()
	root := t.TempDir()
	return []executor.Input{
		{Name: "assessment-schema", Path: filepath.Join(root, "assessment-schema"), Digest: schemaDigest},
		{Name: "plan", Path: filepath.Join(root, "plan"), Digest: testCodexVerifierDigest},
		{Name: "submission", Path: filepath.Join(root, "submission"), Digest: testCodexVerifierDigest},
		{Name: "dispatch", Path: filepath.Join(root, "dispatch"), Digest: testCodexVerifierDigest},
		{Name: "review-policy", Path: filepath.Join(root, "review-policy"), Digest: testCodexVerifierDigest},
		{Name: "review-check-test", Path: filepath.Join(root, "review-check-test"), Digest: testCodexVerifierDigest},
		{Name: "review-authority", Path: filepath.Join(root, "review-authority"), Digest: testCodexVerifierDigest},
	}
}

func testCodexVerifierBoundInputs(profile protocol.VerifierProfile) []executor.BoundInput {
	return []executor.BoundInput{
		{Name: "assessment-schema", Digest: profile.OutputSchemaDigest, Size: 100},
		{Name: "codex", Digest: pinnedCodexDigest, Size: uint64(pinnedCodexSize)},
		{Name: "dispatch", Digest: testCodexVerifierDigest, Size: 100},
		{Name: "plan", Digest: testCodexVerifierDigest, Size: 100},
		{Name: "review-authority", Digest: testCodexVerifierDigest, Size: 100},
		{Name: "review-check-test", Digest: testCodexVerifierDigest, Size: 100},
		{Name: "review-policy", Digest: testCodexVerifierDigest, Size: 100},
		{Name: "submission", Digest: testCodexVerifierDigest, Size: 100},
	}
}

func testCodexVerifierAssessment() string {
	return `{"schema_version":"sworn-verifier-assessment-v1","outcome":"PASS","summary":"ready","acceptance_results":[{"acceptance_id":"acceptance-1","outcome":"pass","evidence_ids":["evidence-1"],"summary":"proven"}],"assurance_results":[],"findings":[]}`
}

func testCodexVerifierAgentLine(t *testing.T, assessment string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "item-2", "type": "agent_message", "text": assessment},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func testCodexVerifierJSONL(t *testing.T, assessment string) []byte {
	t.Helper()
	return []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"item-1","type":"reasoning","text":"reviewed"}}`,
		testCodexVerifierAgentLine(t, assessment),
		`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":1,"cache_write_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":1}}`,
		"",
	}, "\n"))
}
