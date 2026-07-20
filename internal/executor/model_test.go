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

func testDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
