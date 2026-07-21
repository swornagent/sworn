package executor

import (
	"strings"
	"testing"
	"time"
)

func TestInvocationValidationAdmitsOnlyExplicitBoundary(t *testing.T) {
	t.Parallel()
	options := Options{
		Limits:             DefaultLimits(),
		AllowedEnvironment: []string{"API_TOKEN", "FEATURE_FLAG"},
	}
	valid := Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "run-1.builder",
		Role:            "builder",
		Workspace:       "/work/source",
		WorkspaceDigest: testDigest("a"),
		WorkspaceAccess: WorkspaceReadOnly,
		Argv:            []string{"/usr/bin/python3", "-c", "print('ok')"},
		Environment:     map[string]string{"FEATURE_FLAG": "enabled"},
		Network:         NetworkNone,
		Timeout:         time.Minute,
		Inputs: []Input{{
			Name:   "task",
			Path:   "/work/task.json",
			Digest: testDigest("b"),
		}},
	}
	if err := valid.validate(options); err != nil {
		t.Fatalf("validate admitted invocation: %v", err)
	}
	goTool := valid
	goTool.Argv = []string{"/usr/local/go/bin/go", "test", "./..."}
	if err := goTool.validate(options); err != nil {
		t.Fatalf("validate executable beneath mounted /usr: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Invocation, *Options)
		want   string
	}{
		{"unknown schema", func(inv *Invocation, _ *Options) { inv.SchemaVersion = "future" }, "unknown invocation schema"},
		{"relative executable", func(inv *Invocation, _ *Options) { inv.Argv[0] = "python3" }, "absolute"},
		{"outside runtime root", func(inv *Invocation, _ *Options) { inv.Argv[0] = "/opt/tool" }, "mounted /usr"},
		{"input path without selector", func(inv *Invocation, _ *Options) { inv.Argv[0] = "/inputs/task" }, "mounted /usr"},
		{"unlisted environment", func(inv *Invocation, _ *Options) { inv.Environment = map[string]string{"SECRET": "x"} }, "not allowlisted"},
		{"reserved environment", func(inv *Invocation, _ *Options) { inv.Environment = map[string]string{"HOME": "/outside"} }, "invalid invocation environment"},
		{"reserved Codex home", func(inv *Invocation, _ *Options) { inv.Environment = map[string]string{"CODEX_HOME": "/outside"} }, "invalid invocation environment"},
		{"host network", func(inv *Invocation, _ *Options) { inv.Network = NetworkHost }, "host network is not admitted"},
		{"nested sandbox", func(inv *Invocation, _ *Options) { inv.NestedSandbox = true }, "nested sandbox is not admitted"},
		{"runtime ceiling", func(inv *Invocation, _ *Options) { inv.Timeout = 6 * time.Minute }, "timeout"},
		{"unclean workspace", func(inv *Invocation, _ *Options) { inv.Workspace = "/work/../source" }, "clean absolute"},
		{"invalid digest", func(inv *Invocation, _ *Options) { inv.WorkspaceDigest = "sha256:no" }, "exact sha256"},
		{"unknown workspace access", func(inv *Invocation, _ *Options) { inv.WorkspaceAccess = "future" }, "workspace access"},
		{"duplicate input", func(inv *Invocation, _ *Options) { inv.Inputs = append(inv.Inputs, inv.Inputs[0]) }, "duplicate input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation := valid
			invocation.Argv = append([]string(nil), valid.Argv...)
			invocation.Inputs = append([]Input(nil), valid.Inputs...)
			invocation.Environment = map[string]string{"FEATURE_FLAG": "enabled"}
			candidateOptions := options
			test.mutate(&invocation, &candidateOptions)
			err := invocation.validate(candidateOptions)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestHostNetworkRequiresExecutorAndInvocationOptIn(t *testing.T) {
	t.Parallel()
	invocation := Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "run-1",
		Role:            "builder",
		Workspace:       "/work/source",
		WorkspaceDigest: testDigest("a"),
		WorkspaceAccess: WorkspaceReadOnly,
		Argv:            []string{"/usr/bin/true"},
		Network:         NetworkHost,
		Timeout:         time.Minute,
	}
	options := Options{Limits: DefaultLimits(), AllowHostNetwork: true}
	if err := invocation.validate(options); err != nil {
		t.Fatalf("validate explicit host-network opt-in: %v", err)
	}
}

func TestNestedSandboxRequiresExecutorAndInvocationOptIn(t *testing.T) {
	t.Parallel()
	invocation := Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "run-1",
		Role:            "builder",
		NestedSandbox:   true,
		Workspace:       "/work/source",
		WorkspaceDigest: testDigest("a"),
		WorkspaceAccess: WorkspaceReadOnly,
		Argv:            []string{"/usr/bin/true"},
		Network:         NetworkNone,
		Timeout:         time.Minute,
	}
	options := Options{Limits: DefaultLimits(), AllowNestedSandbox: true}
	if err := invocation.validate(options); err != nil {
		t.Fatalf("validate explicit nested-sandbox opt-in: %v", err)
	}

	invocation.NestedSandbox = false
	if err := invocation.validate(Options{Limits: DefaultLimits()}); err != nil {
		t.Fatalf("validate default non-nested invocation: %v", err)
	}
}

func TestCredentialFileRequiresExecutorAndWritableInvocationOptIn(t *testing.T) {
	t.Parallel()
	invocation := Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               "run-1",
		Role:             "builder",
		CredentialAccess: true,
		Workspace:        "/work/source",
		WorkspaceDigest:  testDigest("a"),
		WorkspaceAccess:  WorkspaceWritableExport,
		Argv:             []string{"/usr/bin/true"},
		Network:          NetworkNone,
		Timeout:          time.Minute,
	}
	options := Options{
		Limits:              DefaultLimits(),
		CredentialFile:      "/secure/codex/auth.json",
		AllowCredentialFile: true,
	}
	if err := invocation.validate(options); err != nil {
		t.Fatalf("validate explicit credential-file opt-in: %v", err)
	}

	executorOnly := invocation
	executorOnly.CredentialAccess = false
	if err := executorOnly.validate(options); err != nil {
		t.Fatalf("validate executor-only credential admission: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Invocation, *Options)
		message string
	}{
		{
			name: "invocation only",
			mutate: func(_ *Invocation, options *Options) {
				options.CredentialFile = ""
				options.AllowCredentialFile = false
			},
			message: "not admitted",
		},
		{
			name: "path without admission",
			mutate: func(_ *Invocation, options *Options) {
				options.AllowCredentialFile = false
			},
			message: "requires one configured",
		},
		{
			name: "admission without path",
			mutate: func(_ *Invocation, options *Options) {
				options.CredentialFile = ""
			},
			message: "requires one configured",
		},
		{
			name: "unclean source path",
			mutate: func(_ *Invocation, options *Options) {
				options.CredentialFile = "/secure/../auth.json"
			},
			message: "clean absolute",
		},
		{
			name: "read-only entry point",
			mutate: func(invocation *Invocation, _ *Options) {
				invocation.WorkspaceAccess = WorkspaceReadOnly
			},
			message: "writable executor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateInvocation := invocation
			candidateOptions := options
			test.mutate(&candidateInvocation, &candidateOptions)
			err := candidateInvocation.validate(candidateOptions)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("validate error = %v, want substring %q", err, test.message)
			}
		})
	}
}

func TestCredentialReadOnlyEntryPointClassRequiresExactBoundary(t *testing.T) {
	t.Parallel()
	valid := Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               "run-1",
		Role:             "verifier",
		NestedSandbox:    true,
		CredentialAccess: true,
		Workspace:        "/work/candidate",
		WorkspaceDigest:  testDigest("a"),
		WorkspaceAccess:  WorkspaceReadOnly,
		ExecutableInput:  "codex",
		Inputs: []Input{{
			Name: "codex", Path: "/opt/codex", Digest: testDigest("b"),
		}},
		Argv:    []string{"/inputs/codex", "exec"},
		Network: NetworkHost,
		Timeout: time.Minute,
	}
	options := Options{
		Limits:              DefaultLimits(),
		AllowHostNetwork:    true,
		AllowNestedSandbox:  true,
		CredentialFile:      "/secure/codex/auth.json",
		AllowCredentialFile: true,
	}
	if err := valid.validateFor(options, executionCredentialReadOnly); err != nil {
		t.Fatalf("validate credentialed read-only boundary: %v", err)
	}
	if err := valid.validate(options); err == nil || !strings.Contains(err.Error(), "writable executor") {
		t.Fatalf("generic entry point admitted credentialed read-only invocation: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Invocation, *Options)
		want   string
	}{
		{"credential absent", func(invocation *Invocation, _ *Options) { invocation.CredentialAccess = false }, "requires credential-file access"},
		{"writable workspace", func(invocation *Invocation, _ *Options) { invocation.WorkspaceAccess = WorkspaceWritableExport }, "requires a host-runtime"},
		{"content runtime", func(invocation *Invocation, _ *Options) { invocation.RuntimeDigest = testDigest("c") }, "requires a host-runtime"},
		{"nested sandbox absent", func(invocation *Invocation, _ *Options) { invocation.NestedSandbox = false }, "requires a host-runtime"},
		{"host network absent", func(invocation *Invocation, _ *Options) { invocation.Network = NetworkNone }, "requires a host-runtime"},
		{"executable input absent", func(invocation *Invocation, _ *Options) {
			invocation.ExecutableInput = ""
			invocation.Argv = []string{"/usr/bin/true"}
		}, "requires a host-runtime"},
		{"executor host network denied", func(_ *Invocation, options *Options) { options.AllowHostNetwork = false }, "host network is not admitted"},
		{"executor nested sandbox denied", func(_ *Invocation, options *Options) { options.AllowNestedSandbox = false }, "nested sandbox is not admitted"},
		{"executor credential denied", func(_ *Invocation, options *Options) { options.AllowCredentialFile = false }, "requires one configured"},
		{"credential inside workspace", func(invocation *Invocation, _ *Options) { invocation.Workspace = "/secure" }, "workspace overlaps"},
		{"workspace inside credential home", func(invocation *Invocation, _ *Options) { invocation.Workspace = "/secure/codex/candidate" }, "workspace overlaps"},
		{"credential copied as input", func(invocation *Invocation, _ *Options) {
			invocation.Inputs = append(invocation.Inputs, Input{
				Name: "secret", Path: "/secure/codex/auth.json", Digest: testDigest("c"),
			})
		}, "input \"secret\" overlaps"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation := valid
			invocation.Argv = append([]string(nil), valid.Argv...)
			invocation.Inputs = append([]Input(nil), valid.Inputs...)
			candidateOptions := options
			test.mutate(&invocation, &candidateOptions)
			err := invocation.validateFor(candidateOptions, executionCredentialReadOnly)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestExecutableInputRequiresExactExistingInputAndArgv(t *testing.T) {
	t.Parallel()
	valid := Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "run-1",
		Role:            "builder",
		Workspace:       "/work/source",
		WorkspaceDigest: testDigest("a"),
		WorkspaceAccess: WorkspaceReadOnly,
		ExecutableInput: "codex",
		Inputs: []Input{{
			Name: "codex", Path: "/opt/codex", Digest: testDigest("b"),
		}},
		Argv:    []string{"/inputs/codex", "exec"},
		Network: NetworkNone,
		Timeout: time.Minute,
	}
	if err := valid.validate(Options{Limits: DefaultLimits()}); err != nil {
		t.Fatalf("validate executable input: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Invocation)
		want   string
	}{
		{"different argv", func(invocation *Invocation) { invocation.Argv[0] = "/usr/bin/true" }, "must use"},
		{"absent input", func(invocation *Invocation) { invocation.ExecutableInput = "missing" }, "is absent"},
		{"invalid selector", func(invocation *Invocation) { invocation.ExecutableInput = "../codex" }, "valid input"},
		{"duplicate selected input", func(invocation *Invocation) { invocation.Inputs = append(invocation.Inputs, invocation.Inputs[0]) }, "duplicate input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation := valid
			invocation.Argv = append([]string(nil), valid.Argv...)
			invocation.Inputs = append([]Input(nil), valid.Inputs...)
			test.mutate(&invocation)
			err := invocation.validate(Options{Limits: DefaultLimits()})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateExecutableArgv(t *testing.T) {
	t.Parallel()
	if err := ValidateExecutableArgv("codex", []string{"/inputs/codex", "exec"}); err != nil {
		t.Fatalf("validate executable argv: %v", err)
	}
	for _, test := range []struct {
		name  string
		input string
		argv  []string
	}{
		{name: "invalid selector", input: "../codex", argv: []string{"/inputs/../codex"}},
		{name: "missing argv", input: "codex"},
		{name: "wrong entrypoint", input: "codex", argv: []string{"/usr/bin/codex"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateExecutableArgv(test.input, test.argv); err == nil {
				t.Fatal("invalid executable argv was accepted")
			}
		})
	}
}

func testDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
