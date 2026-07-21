//go:build linux

package executor

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	requireLinuxExecutorEnvironment = "SWORN_REQUIRE_LINUX_EXECUTOR"
	shimTestSentinel                = "__sworn_executor_shim_test__"
	engineTestSentinel              = "__sworn_executor_engine_test__"
	runtimeTestSentinel             = "__sworn_content_runtime_test__"
)

func runHostContained(executor *LinuxExecutor, ctx context.Context, invocation Invocation) (RawCompletion, error) {
	return executor.runInvocation(ctx, invocation, WorkspaceReadOnly, nil, nil)
}

func TestExecutorShimProcess(t *testing.T) {
	index := argumentIndex(os.Args, shimTestSentinel)
	if index < 0 {
		return
	}
	os.Exit(RunShim(os.Args[index+1:], os.Stdin, os.Stdout, os.Stderr))
}

func TestExecutorEngineProcess(t *testing.T) {
	index := argumentIndex(os.Args, engineTestSentinel)
	if index < 0 {
		return
	}
	arguments := os.Args[index+1:]
	if len(arguments) != 4 {
		fmt.Fprintln(os.Stderr, "executor engine helper: expected runtime, workspace, digest, and id")
		os.Exit(2)
	}
	executor, err := newTestExecutor(arguments[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "executor engine helper: %v\n", err)
		os.Exit(1)
	}
	_, err = runHostContained(executor, context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              arguments[3],
		Role:            "builder",
		Workspace:       arguments[1],
		WorkspaceDigest: arguments[2],
		WorkspaceAccess: WorkspaceReadOnly,
		Argv: []string{
			"/usr/bin/python3",
			"-c",
			"import subprocess,time; subprocess.Popen(['/usr/bin/sleep','60']); time.sleep(60)",
		},
		Network: NetworkNone,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "executor engine helper: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestContentRuntimeHelperProcess(t *testing.T) {
	if argumentIndex(os.Args, runtimeTestSentinel) < 0 {
		return
	}
	contents, err := os.ReadFile("/workspace/value.txt")
	if err != nil || string(contents) != "candidate\n" {
		fmt.Fprintf(os.Stderr, "runtime helper workspace: %q, %v\n", contents, err)
		os.Exit(1)
	}
	if _, err := os.Stat("/usr/bin/python3"); !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "runtime helper observed host tool: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile("/workspace/forbidden", []byte("write"), 0o600); err == nil {
		fmt.Fprintln(os.Stderr, "runtime helper wrote read-only workspace")
		os.Exit(1)
	}
	fmt.Println("content-bound")
	os.Exit(0)
}

func TestLinuxExecutorEntryPointsBindWorkspaceAccess(t *testing.T) {
	executor := &LinuxExecutor{options: Options{Limits: DefaultLimits()}}
	invocation := Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "workspace-access-binding",
		Role:            "builder",
		Workspace:       "/work/source",
		WorkspaceDigest: testDigest("a"),
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/true"},
		Network:         NetworkNone,
		Timeout:         time.Minute,
	}
	if _, err := runHostContained(executor, context.Background(), invocation); err == nil || !strings.Contains(err.Error(), "entry point") {
		t.Fatalf("read-only entry point admitted writable access: %v", err)
	}
	invocation.WorkspaceAccess = WorkspaceReadOnly
	if _, err := executor.RunWritable(context.Background(), invocation); err == nil || !strings.Contains(err.Error(), "entry point") {
		t.Fatalf("writable entry point admitted read-only access: %v", err)
	}
}

func TestLinuxExecutorSeparatesHostAndContentRuntimeEntryPoints(t *testing.T) {
	executor := &LinuxExecutor{options: Options{Limits: DefaultLimits()}}
	invocation := Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "runtime-binding",
		Role:            "producer",
		RuntimeDigest:   testDigest("a"),
		Workspace:       "/work/source",
		WorkspaceDigest: testDigest("b"),
		WorkspaceAccess: WorkspaceReadOnly,
		Argv:            []string{"/usr/bin/true"},
		Network:         NetworkNone,
		Timeout:         time.Minute,
	}
	if _, err := runHostContained(executor, context.Background(), invocation); err == nil || !strings.Contains(err.Error(), "host-runtime") {
		t.Fatalf("host entry point accepted a runtime digest: %v", err)
	}
	if _, err := executor.RunContentBound(context.Background(), invocation, RuntimeTree{}); err == nil ||
		!strings.Contains(err.Error(), "exact runtime digest") {
		t.Fatalf("content entry point accepted a zero runtime capability: %v", err)
	}
}

func TestExecutorUnitNamesBindRuntimeRootAndInvocation(t *testing.T) {
	first := &LinuxExecutor{options: Options{RuntimeRoot: "/run/user/1000/sworn/first"}}
	second := &LinuxExecutor{options: Options{RuntimeRoot: "/run/user/1000/sworn/second"}}
	firstUnit := first.unitName("invocation-1")
	if firstUnit != first.unitName("invocation-1") {
		t.Fatal("unit name is not deterministic")
	}
	if firstUnit == first.unitName("invocation-2") || firstUnit == second.unitName("invocation-1") {
		t.Fatal("unit name does not bind runtime root and invocation")
	}
}

func TestLinuxExecutorContainsReadOnlyInvocation(t *testing.T) {
	executor := requireLinuxExecutor(t)
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "source.txt"), []byte("immutable"), 0o640)
	workspaceDigest, _, err := MeasureWorkspace(context.Background(), workspace, executor.options.Limits.InputBytes)
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(t.TempDir(), "task.json")
	inputContents := []byte(`{"task":"contain"}`)
	writeTestFile(t, inputPath, inputContents, 0o600)
	hostSecret := filepath.Join(os.TempDir(), "sworn-host-secret-"+t.Name())
	writeTestFile(t, hostSecret, []byte("not visible"), 0o600)
	t.Cleanup(func() { _ = os.Remove(hostSecret) })
	hostNetworkNamespace, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Join([]string{
		"import os, pathlib, resource",
		"assert os.getcwd() == '/workspace'",
		"assert pathlib.Path('/workspace/source.txt').read_text() == 'immutable'",
		"assert pathlib.Path('/inputs/task').read_text() == '{\"task\":\"contain\"}'",
		"assert not pathlib.Path('/workspace/.git').exists()",
		"assert not pathlib.Path(os.environ['HOST_SECRET']).exists()",
		"assert os.environ['FEATURE_FLAG'] == 'enabled'",
		"assert os.environ['HOME'] == '/home/sworn'",
		"assert os.readlink('/proc/self/ns/net') != os.environ['HOST_NET_NS']",
		"assert resource.getrlimit(resource.RLIMIT_FSIZE)[0] == 16777216",
		"assert resource.getrlimit(resource.RLIMIT_NOFILE)[0] == 1024",
		"try:",
		" pathlib.Path('/workspace/write').write_text('forbidden')",
		" raise AssertionError('workspace was writable')",
		"except OSError:",
		" pass",
		"print('contained')",
	}, "\n")
	completion, err := runHostContained(executor, context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "integration-contained",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: workspaceDigest,
		WorkspaceAccess: WorkspaceReadOnly,
		Inputs: []Input{{
			Name:   "task",
			Path:   inputPath,
			Digest: digestBytes(inputContents),
		}},
		Argv: []string{"/usr/bin/python3", "-c", program},
		Environment: map[string]string{
			"FEATURE_FLAG": "enabled",
			"HOST_SECRET":  hostSecret,
			"HOST_NET_NS":  hostNetworkNamespace,
		},
		Network: NetworkNone,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("run contained: %v; stderr=%s", err, completion.Stderr)
	}
	if completion.ExitCode != 0 || string(completion.Stdout) != "contained\n" {
		t.Fatalf("completion exit=%d stdout=%q stderr=%q", completion.ExitCode, completion.Stdout, completion.Stderr)
	}
	if completion.WorkspaceDigest != workspaceDigest || len(completion.Inputs) != 1 {
		t.Fatalf("completion bindings = %#v", completion)
	}
}

func TestLinuxExecutorRunsOnlyStagedContentRuntime(t *testing.T) {
	executor := requireLinuxExecutor(t)
	ctx := context.Background()
	runtimeSource := t.TempDir()
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	requireStaticRuntimeHelper(t, testBinary)
	binary, err := os.ReadFile(testBinary)
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeFile(t, filepath.Join(runtimeSource, "bin", "runtime-helper"), binary, 0o755)
	runtimeDigest, _, err := MeasureWorkspace(ctx, runtimeSource, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntimeTree(runtimeSource, runtimeDigest, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "value.txt"), []byte("candidate\n"), 0o600)
	workspaceDigest, _, err := MeasureWorkspace(ctx, workspace, executor.options.Limits.InputBytes)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := executor.RunContentBound(ctx, Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "integration-content-runtime",
		Role:            "producer",
		RuntimeDigest:   runtimeDigest,
		Workspace:       workspace,
		WorkspaceDigest: workspaceDigest,
		WorkspaceAccess: WorkspaceReadOnly,
		Argv: []string{
			"/usr/bin/runtime-helper", "-test.run=^TestContentRuntimeHelperProcess$", "--", runtimeTestSentinel,
		},
		Network: NetworkNone,
		Timeout: 10 * time.Second,
	}, runtime)
	if err != nil {
		t.Fatalf("run content-bound: %v; stderr=%s", err, completion.Stderr)
	}
	if completion.ExitCode != 0 || string(completion.Stdout) != "content-bound\n" ||
		completion.RuntimeDigest != runtimeDigest {
		t.Fatalf("content completion = %#v", completion)
	}
	wrongDigest := testDigest("f")
	wrongRuntime, err := NewRuntimeTree(runtimeSource, wrongDigest, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.RunContentBound(ctx, Invocation{
		SchemaVersion: InvocationSchemaVersion, ID: "integration-content-runtime-mismatch", Role: "producer",
		RuntimeDigest: wrongDigest, Workspace: workspace, WorkspaceDigest: workspaceDigest,
		WorkspaceAccess: WorkspaceReadOnly, Argv: []string{"/usr/bin/runtime-helper"},
		Network: NetworkNone, Timeout: 10 * time.Second,
	}, wrongRuntime); err == nil || !strings.Contains(err.Error(), "runtime digest mismatch") {
		t.Fatalf("wrong content runtime error = %v", err)
	}
	entries, err := os.ReadDir(executor.options.RuntimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "invocation-") {
			t.Fatalf("failed content runtime left private invocation %q", entry.Name())
		}
	}
}

func requireStaticRuntimeHelper(t *testing.T, path string) {
	t.Helper()
	binary, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer binary.Close() //nolint:errcheck
	for _, program := range binary.Progs {
		if program.Type == elf.PT_INTERP {
			if os.Getenv(requireLinuxExecutorEnvironment) == "1" {
				t.Fatal("required content runtime helper is dynamically linked")
			}
			t.Skip("content runtime integration helper is dynamically linked")
		}
	}
}

func TestLinuxExecutorBoundsOutputAndStopsInvocation(t *testing.T) {
	executor := requireLinuxExecutor(t)
	executor.options.Limits.StdoutBytes = 1024
	workspace, digest := emptyTestWorkspace(t, executor)
	started := time.Now()
	completion, err := runHostContained(executor, context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "integration-output-bound",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceReadOnly,
		Argv:            []string{"/usr/bin/python3", "-c", "import os,time; os.write(1,b'x'*4096); time.sleep(60)"},
		Network:         NetworkNone,
		Timeout:         20 * time.Second,
	})
	if err != nil {
		t.Fatalf("run bounded output: %v; stderr=%s", err, completion.Stderr)
	}
	if !completion.OutputTruncated || len(completion.Stdout) != 1024 {
		t.Fatalf("output truncation=%t, stdout bytes=%d", completion.OutputTruncated, len(completion.Stdout))
	}
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Fatalf("overflow cleanup took %s", elapsed)
	}
	assertUnitInactive(t, executor, completion.Unit, 5*time.Second)
}

func TestLinuxExecutorCancellationStopsServiceCgroup(t *testing.T) {
	executor := requireLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(250*time.Millisecond, cancel)
	started := time.Now()
	completion, err := runHostContained(executor, ctx, Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "integration-cancel",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceReadOnly,
		Argv: []string{
			"/usr/bin/python3",
			"-c",
			"import subprocess,time; subprocess.Popen(['/usr/bin/sleep','60']); time.sleep(60)",
		},
		Network: NetworkNone,
		Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("cancel invocation: %v; stderr=%s", err, completion.Stderr)
	}
	if !completion.Cancelled || completion.TimedOut {
		t.Fatalf("completion cancellation=%t timeout=%t", completion.Cancelled, completion.TimedOut)
	}
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Fatalf("cancellation cleanup took %s", elapsed)
	}
	assertUnitInactive(t, executor, completion.Unit, 5*time.Second)
}

func TestLinuxExecutorMarksInvocationTimeout(t *testing.T) {
	executor := requireLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	completion, err := runHostContained(executor, context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "integration-timeout",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceReadOnly,
		Argv:            []string{"/usr/bin/sleep", "60"},
		Network:         NetworkNone,
		Timeout:         300 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("timeout invocation: %v; stderr=%s", err, completion.Stderr)
	}
	if completion.Cancelled || !completion.TimedOut {
		t.Fatalf("completion cancellation=%t timeout=%t", completion.Cancelled, completion.TimedOut)
	}
	assertUnitInactive(t, executor, completion.Unit, 5*time.Second)
}

func TestLinuxExecutorEngineDeathStopsServiceCgroup(t *testing.T) {
	executor := requireLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	invocationID := "integration-engine-death"
	unit := executor.unitName(invocationID)
	var output bytes.Buffer
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		testBinary,
		"-test.run=^TestExecutorEngineProcess$",
		"--",
		engineTestSentinel,
		executor.options.RuntimeRoot,
		workspace,
		digest,
		invocationID,
	)
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		killUnit(executor, unit)
	})
	waitUnitActive(t, executor, unit, done, &output, 8*time.Second)
	controlGroup := unitProperty(t, executor, unit, "ControlGroup")
	assertUnitProperty(t, executor, unit, "MemoryMax", fmt.Sprint(executor.options.Limits.MemoryBytes))
	assertUnitProperty(t, executor, unit, "MemorySwapMax", fmt.Sprint(executor.options.Limits.SwapBytes))
	assertUnitProperty(t, executor, unit, "TasksMax", fmt.Sprint(executor.options.Limits.Tasks))
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill engine helper: %v", err)
	}
	if err := <-done; err == nil {
		t.Fatal("engine helper unexpectedly exited successfully after SIGKILL")
	}
	assertUnitInactive(t, executor, unit, 8*time.Second)
	assertCgroupEmpty(t, controlGroup, 8*time.Second)
}

func TestBuiltSwornBinaryProvidesExecutorShim(t *testing.T) {
	executor := requireLinuxExecutor(t)
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "sworn")
	build := exec.Command("/usr/local/go/bin/go", "build", "-o", binary, "./cmd/sworn")
	build.Dir = moduleRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sworn: %v: %s", err, output)
	}
	executor.options.ShimArgv = []string{binary, "__executor-shim"}
	workspace, digest := emptyTestWorkspace(t, executor)
	completion, err := runHostContained(executor, context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "integration-built-shim",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceReadOnly,
		Argv:            []string{"/usr/bin/true"},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err != nil || completion.ExitCode != 0 {
		t.Fatalf("built shim completion=%#v, error=%v", completion, err)
	}
}

func requireLinuxExecutor(t *testing.T) *LinuxExecutor {
	t.Helper()
	executor, err := newTestExecutor(t.TempDir())
	if err == nil {
		_, err = executor.Probe(context.Background())
	}
	if err != nil {
		if os.Getenv(requireLinuxExecutorEnvironment) == "1" {
			t.Fatalf("required Linux executor capability: %v", err)
		}
		t.Skipf("Linux executor capability unavailable: %v", err)
	}
	return executor
}

func newTestExecutor(runtimeRoot string) (*LinuxExecutor, error) {
	return newTestExecutorWithWritableRoot(runtimeRoot, "")
}

func newTestExecutorWithWritableRoot(runtimeRoot, writableRoot string) (*LinuxExecutor, error) {
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		return nil, err
	}
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		return nil, err
	}
	limits := DefaultLimits()
	limits.Runtime = 30 * time.Second
	limits.MemoryBytes = 256 << 20
	limits.Tasks = 64
	limits.FileBytes = 16 << 20
	limits.TempBytes = 32 << 20
	limits.HomeBytes = 16 << 20
	limits.InputBytes = 32 << 20
	limits.WorkspaceBytes = 32 << 20
	limits.StdoutBytes = 32 << 10
	limits.StderrBytes = 32 << 10
	return NewLinux(Options{
		RuntimeRoot:        runtimeRoot,
		WritableRoot:       writableRoot,
		ShimArgv:           []string{testBinary, "-test.run=^TestExecutorShimProcess$", "--", shimTestSentinel},
		Limits:             limits,
		AllowedEnvironment: []string{"FEATURE_FLAG", "HOST_SECRET", "HOST_NET_NS"},
	})
}

func emptyTestWorkspace(t *testing.T, executor *LinuxExecutor) (string, string) {
	t.Helper()
	workspace := t.TempDir()
	digest, _, err := MeasureWorkspace(context.Background(), workspace, executor.options.Limits.InputBytes)
	if err != nil {
		t.Fatal(err)
	}
	return workspace, digest
}

func waitUnitActive(
	t *testing.T,
	executor *LinuxExecutor,
	unit string,
	done <-chan error,
	output *bytes.Buffer,
	limit time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("engine helper exited before unit became active: %v: %s", err, output.String())
		default:
		}
		if unitActive(executor, unit) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("unit %s did not become active: %s", unit, output.String())
}

func assertUnitInactive(t *testing.T, executor *LinuxExecutor, unit string, limit time.Duration) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if !unitLive(executor, unit) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	state, _ := unitState(executor, unit)
	t.Fatalf("unit %s remained active in state %q", unit, state)
}

func unitActive(executor *LinuxExecutor, unit string) bool {
	state, err := unitState(executor, unit)
	return err == nil && state == "active"
}

func unitLive(executor *LinuxExecutor, unit string) bool {
	state, err := unitState(executor, unit)
	if err != nil {
		return false
	}
	switch state {
	case "active", "activating", "deactivating", "reloading", "refreshing":
		return true
	default:
		return false
	}
}

func unitState(executor *LinuxExecutor, unit string) (string, error) {
	command := exec.Command(executor.options.SystemctlPath, "--user", "is-active", unit)
	command.Env = controlEnvironment()
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func assertUnitProperty(t *testing.T, executor *LinuxExecutor, unit, property, want string) {
	t.Helper()
	if got := unitProperty(t, executor, unit, property); got != want {
		t.Fatalf("unit %s property %s = %q, want %q", unit, property, got, want)
	}
}

func unitProperty(t *testing.T, executor *LinuxExecutor, unit, property string) string {
	t.Helper()
	command := exec.Command(executor.options.SystemctlPath, "--user", "show", "--property="+property, "--value", unit)
	command.Env = controlEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read unit %s property %s: %v: %s", unit, property, err, output)
	}
	return strings.TrimSpace(string(output))
}

func assertCgroupEmpty(t *testing.T, controlGroup string, limit time.Duration) {
	t.Helper()
	path := filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(controlGroup, "/"), "cgroup.procs")
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if os.IsNotExist(err) || (err == nil && len(bytes.TrimSpace(contents)) == 0) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	contents, err := os.ReadFile(path)
	t.Fatalf("cgroup %s retained processes: %q, %v", controlGroup, contents, err)
}

func killUnit(executor *LinuxExecutor, unit string) {
	command := exec.Command(executor.options.SystemctlPath, "--user", "kill", "--kill-whom=all", "--signal=SIGKILL", unit)
	command.Env = controlEnvironment()
	_ = command.Run()
}

func argumentIndex(arguments []string, target string) int {
	for index, argument := range arguments {
		if argument == target {
			return index
		}
	}
	return -1
}
