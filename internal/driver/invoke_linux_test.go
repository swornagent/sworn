//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/gitx"
)

var (
	testFakeBinary    string
	testProcessBinary string
	testBuildError    error
	testBuildOnce     sync.Once
)

func buildFakeExecutable(t *testing.T) string {
	t.Helper()
	testBuildOnce.Do(func() {
		directory, err := os.MkdirTemp("", "sworn-fake-driver-test-")
		if err != nil {
			testBuildError = err
			return
		}
		testFakeBinary = filepath.Join(directory, "baton-fake")
		command := exec.Command("go", "build", "-o", testFakeBinary, "./testdata/fake")
		command.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
		output, err := command.CombinedOutput()
		if err != nil {
			testBuildError = &buildFailure{output: string(output)}
			return
		}
		testProcessBinary = filepath.Join(directory, "process-fixture")
		command = exec.Command("go", "build", "-o", testProcessBinary, "./testdata/process")
		command.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
		output, err = command.CombinedOutput()
		if err != nil {
			testBuildError = &buildFailure{output: string(output)}
		}
	})
	if testBuildError != nil {
		t.Fatal(testBuildError)
	}
	return testFakeBinary
}

func processExecutable(t *testing.T, behavior string) string {
	t.Helper()
	_ = buildFakeExecutable(t)
	body, err := os.ReadFile(testProcessBinary)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), behavior)
	if err := os.WriteFile(target, body, 0o700); err != nil {
		t.Fatal(err)
	}
	return target
}

func setProcessExecutable(t *testing.T, invocation *Invocation, behavior string) string {
	t.Helper()
	executable := processExecutable(t, behavior)
	setProcessExecutablePath(t, invocation, executable)
	return executable
}

func setProcessExecutablePath(t *testing.T, invocation *Invocation, executable string) {
	t.Helper()
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewProcessAdapter(
		invocation.Selected.Adapter.Key,
		invocation.Selected.Adapter.ID,
		invocation.Selected.Adapter.Version,
		ExecutableIdentity{Path: executable, Digest: digest},
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation.Selected.Adapter = adapter.Identity()
	invocation.Selected.adapter = adapter
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		invocation.Request,
		invocation.Selected,
		descriptor.Containment,
		descriptor.Responsibility,
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation.Permission = permission
}

type buildFailure struct {
	output string
}

func (failure *buildFailure) Error() string {
	return "build fake executable failed: " + failure.output
}

func executableSelection(t *testing.T) SelectedProfile {
	t.Helper()
	executable := buildFakeExecutable(t)
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewProcessAdapter(
		"fake-adapter",
		FakeDriverID,
		FakeDriverVersion,
		ExecutableIdentity{Path: executable, Digest: digest},
	)
	if err != nil {
		t.Fatal(err)
	}
	return SelectedProfile{
		Profile: ProfileConfig{
			Key:     "fake-profile",
			Adapter: adapter.Identity().Key,
			Network: NetworkNone,
		},
		Adapter: adapter.Identity(),
		Model:   "fake-model-v1",
		adapter: adapter,
	}
}

func TestFakeBubblewrapArgumentsCannotSelectSharedNetwork(t *testing.T) {
	t.Parallel()
	invocation := Invocation{
		Request: Request{
			Workspace: Workspace{Path: GuestWorkspacePath, Access: ReadOnly},
		},
		Selected: SelectedProfile{
			Profile: ProfileConfig{
				Key:     "fake-profile",
				Adapter: "fake-adapter",
				Network: NetworkNone,
			},
			Adapter: AdapterIdentity{
				Key:                 "fake-adapter",
				ID:                  FakeDriverID,
				Version:             FakeDriverVersion,
				ConfigurationDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
		FakeProfile: FakeCompleted,
	}
	arguments, err := bubblewrapArguments(invocation)
	if err != nil {
		t.Fatal(err)
	}
	if slicesContain(arguments, "--share-net") {
		t.Fatal("networkless fake selected --share-net")
	}

	invocation.Selected.Profile.Network = NetworkRequired
	arguments, err = bubblewrapArguments(invocation)
	if !IsCode(err, "INVALID_NETWORK_POLICY") || arguments != nil {
		t.Fatalf("networked fake arguments = %q, error = %v", arguments, err)
	}

	invocation.Selected.Adapter.ID = "vendor.driver"
	arguments, err = bubblewrapArguments(invocation)
	if err != nil || !slicesContain(arguments, "--share-net") {
		t.Fatalf("networked vendor arguments = %q, error = %v", arguments, err)
	}
}

func slicesContain(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func eventIndex(events []TerminalEvent, kind string) int {
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			return -1
		}
		if event.Kind == kind {
			return index
		}
	}
	return -1
}

func fakeInvocation(
	t *testing.T,
	invocationID string,
	role Role,
	responsibility Responsibility,
	access WorkspaceAccess,
	behavior string,
	submission *Submission,
) (Invocation, string, []byte) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "workspace-canary"), []byte("unchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := fakeScript{
		SchemaVersion: "sworn.fake-script/v1",
		Behavior:      behavior,
	}
	var submissionBody []byte
	if submission != nil {
		var err error
		submissionBody, err = EncodeSubmission(*submission)
		if err != nil {
			t.Fatal(err)
		}
		script.Submission = base64.StdEncoding.EncodeToString(submissionBody)
	}
	scriptBody, err := json.Marshal(script)
	if err != nil {
		t.Fatal(err)
	}
	scriptBody = append(scriptBody, '\n')
	input, content := projectionInput("fake-script", "fake-script.json", scriptBody)
	request, err := NewRequest(
		invocationID,
		role,
		"fake-profile",
		"fake-model-v1",
		Workspace{Path: GuestWorkspacePath, Access: access},
		[]Input{input},
		true,
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected := executableSelection(t)
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(request, selected, containment, responsibility)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:       request,
		HostWorkspace: workspace,
		Selected:      selected,
		Permission:    permission,
		Inputs:        []InputContent{content},
		FakeProfile:   FakeCompleted,
	}, workspace, submissionBody
}

// fakeInvocationReserved is fakeInvocation plus a scripted reserved-name list
// for the reserved-canary process behavior, so a worker can prove from inside
// containment that a configured records/journals root is masked.
func fakeInvocationReserved(
	t *testing.T,
	invocationID string,
	role Role,
	responsibility Responsibility,
	access WorkspaceAccess,
	behavior string,
	reserved []string,
	submission *Submission,
) (Invocation, string) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "workspace-canary"), []byte("unchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A prepared workspace carries its reserved roots; the mask applies to
	// what exists (an absent root cannot be mounted without mutating the
	// host bind). The fixture models the real prepared-workspace shape.
	for _, name := range reserved {
		if err := os.Mkdir(filepath.Join(workspace, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := fakeScript{
		SchemaVersion: "sworn.fake-script/v1",
		Behavior:      behavior,
		Reserved:      append([]string(nil), reserved...),
	}
	if submission != nil {
		submissionBody, err := EncodeSubmission(*submission)
		if err != nil {
			t.Fatal(err)
		}
		script.Submission = base64.StdEncoding.EncodeToString(submissionBody)
	}
	scriptBody, err := json.Marshal(script)
	if err != nil {
		t.Fatal(err)
	}
	scriptBody = append(scriptBody, '\n')
	input, content := projectionInput("fake-script", "fake-script.json", scriptBody)
	request, err := NewRequest(
		invocationID,
		role,
		"fake-profile",
		"fake-model-v1",
		Workspace{Path: GuestWorkspacePath, Access: access},
		[]Input{input},
		true,
		Limits{TimeoutMillis: 15_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected := executableSelection(t)
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(request, selected, containment, responsibility)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:       request,
		HostWorkspace: workspace,
		Selected:      selected,
		Permission:    permission,
		Inputs:        []InputContent{content},
		FakeProfile:   FakeCompleted,
	}, workspace
}

// TestConfiguredRecordsRootMaskedFromWorker is the A3 proof: a project that
// configures an unusual records root has that root masked from every
// model-directed worker. The worker is run inside real containment and, from
// the guest, verifies the configured root (and the always-reserved legacy
// root) is an empty read-only surface it cannot write to.
func TestConfiguredRecordsRootMaskedFromWorker(t *testing.T) {
	requireTrustedContainment(t)

	configured := gitx.ProjectConfig{
		SchemaVersion: gitx.ProjectConfigSchemaVersion,
		RecordsRoot:   ".secret-records",
		JournalsRoot:  ".secret-journals",
		ContractsRoot: "contracts",
		CommitPrefix:  "sworn",
		DocumentsRoot: "docs/sworn",
	}
	reserved := gitx.ReservedNames(configured)
	for _, name := range []string{".secret-records", ".secret-journals", ".baton", ".git"} {
		found := false
		for _, candidate := range reserved {
			if candidate == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("reserved names %v omit %s", reserved, name)
		}
	}

	submissionValue := submissionFixture(
		t, "configured-root-mask", ImplementerImplementation, "",
	)
	invocation, _ := fakeInvocationReserved(
		t,
		"configured-root-mask",
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
		"reserved-canary",
		reserved,
		&submissionValue,
	)
	// The engine derives the mask from the configured project roots; here the
	// test supplies exactly what the engine would compute for this config.
	invocation.MaskNames = reserved
	// The fixture is invoked under the "driver" basename so its fake-script
	// input (behavior + reserved names) is parsed before dispatch.
	setProcessExecutable(t, &invocation, "driver")

	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if observation.TransportStatus != Completed {
		t.Fatalf("reserved-canary transport = %s (diagnostic %s)",
			observation.TransportStatus, observation.Diagnostic.Code)
	}
	if observation.Diagnostic.Code != "none" {
		t.Fatalf("reserved-canary diagnostic = %s", observation.Diagnostic.Code)
	}
	if observation.Handoff == nil {
		t.Fatal("reserved-canary released no sealed handoff")
	}
}

func TestInvokerReleasesOnlyCompletedBoundSealedHandoff(t *testing.T) {
	requireTrustedContainment(t)
	submissionValue := submissionFixture(t, "invoke-submit", PlannerProposal, "")
	submission := &submissionValue
	invocation, workspace, submissionBody := fakeInvocation(
		t,
		"invoke-submit",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"submit",
		submission,
	)
	before, err := captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if observation.TransportStatus != Completed || observation.Handoff == nil ||
		!bytes.Equal(observation.Handoff.SubmissionBytes, submissionBody) ||
		observation.Handoff.SubmissionDigest != Digest(submissionBody) ||
		observation.Handoff.SealDigest != Digest(observation.Handoff.SealBytes) {
		t.Fatalf("observation = %#v", observation)
	}
	if observation.Usage.TokenStatus != UsageReported ||
		observation.Usage.CostStatus != UsageUnavailable ||
		observation.Usage.InputTokens == nil || *observation.Usage.InputTokens != 0 ||
		observation.Diagnostic.Code != "none" ||
		bytes.Contains(observation.Handoff.SealBytes, []byte("Fake completed response")) {
		t.Fatalf("observation leaked or lost normalized facts: %#v", observation)
	}
	resultEvent := eventIndex(observation.Events, "result_completed")
	submitEvent := eventIndex(observation.Events, "submit_accepted_pending")
	ackEvent := eventIndex(observation.Events, "submit_acknowledged")
	stopEvent := eventIndex(observation.Events, "engine_stop_after_submit")
	publishEvent := eventIndex(observation.Events, "published")
	if resultEvent < 0 || submitEvent < 0 || ackEvent < resultEvent ||
		ackEvent < submitEvent || stopEvent != ackEvent+1 ||
		publishEvent <= stopEvent {
		t.Fatalf("terminal events = %#v", observation.Events)
	}
	t.Logf("accepted terminal_event_sequence=%v", observation.Events)
	after, err := captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !equalManifest(before, after) {
		t.Fatal("completed invocation changed the workspace outside its private mount")
	}
}

func TestRejectedSubmissionRequiresResultThenStopsWithoutHandoff(t *testing.T) {
	requireTrustedContainment(t)
	submissionValue := submissionFixture(t, "wrong-invocation", PlannerProposal, "")
	submission := &submissionValue
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-rejected",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"submit",
		submission,
	)
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "MISSING_SUBMISSION") || observation.Handoff != nil ||
		observation.Diagnostic.Code != "submission_rejected" ||
		eventIndex(observation.Events, "result_completed") < 0 ||
		eventIndex(observation.Events, "submit_rejected_pending") < 0 ||
		eventIndex(observation.Events, "engine_stop_after_submit") < 0 {
		t.Fatalf("observation=%#v error=%v", observation, err)
	}
}

func TestAcceptedSubmissionIntentionallyStopsAndQuiescesDescendants(t *testing.T) {
	requireTrustedContainment(t)
	submissionValue := submissionFixture(
		t,
		"invoke-submit-descendant",
		PlannerProposal,
		"",
	)
	submission := &submissionValue
	invocation, _, submissionBody := fakeInvocation(
		t,
		"invoke-submit-descendant",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"submit-descendant",
		submission,
	)
	executable := setProcessExecutable(t, &invocation, "driver")
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		!bytes.Equal(observation.Handoff.SubmissionBytes, submissionBody) ||
		eventIndex(observation.Events, "engine_stop_after_submit") < 0 ||
		eventIndex(observation.Events, "process_group_quiescent") < 0 {
		t.Fatalf("observation=%#v error=%v", observation, err)
	}
	if processUsesExecutable(executable) {
		t.Fatal("intentional submit stop left a descendant running")
	}
	t.Logf("intentional_stop descendant_quiescent=true events=%v", observation.Events)
}

func TestLinuxParentDeathQuiescesSandbox(t *testing.T) {
	requireTrustedContainment(t)
	const helperEnvironment = "SWORN_PARENT_DEATH_HELPER"
	const executableEnvironment = "SWORN_PARENT_DEATH_EXECUTABLE"
	if os.Getenv(helperEnvironment) == "1" {
		executable := os.Getenv(executableEnvironment)
		invocation, _, _ := fakeInvocation(
			t,
			"invoke-parent-death",
			RoleVerifier,
			WorkVerification,
			ReadOnly,
			"block-descendant",
			nil,
		)
		setProcessExecutablePath(t, &invocation, executable)
		_, _ = (Invoker{}).Invoke(context.Background(), invocation)
		t.Fatal("parent-death helper invocation unexpectedly returned")
	}

	executable := processExecutable(t, "parent-death")
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestLinuxParentDeathQuiescesSandbox$",
	)
	command.Env = append(
		os.Environ(),
		helperEnvironment+"=1",
		executableEnvironment+"="+executable,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	startDeadline := time.Now().Add(5 * time.Second)
	for !processUsesExecutable(executable) && time.Now().Before(startDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !processUsesExecutable(executable) {
		t.Fatal("parent-death helper did not start the sandboxed provider")
	}
	if err := command.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("parent-death helper unexpectedly exited successfully")
	}
	quiescenceDeadline := time.Now().Add(2 * time.Second)
	for processUsesExecutable(executable) && time.Now().Before(quiescenceDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processUsesExecutable(executable) {
		t.Fatal("sandboxed provider survived abrupt parent death")
	}
}

func TestLinuxSandboxProcessAttributesRequireParentDeath(t *testing.T) {
	attributes := linuxSandboxProcessAttributes()
	if attributes == nil || !attributes.Setpgid ||
		attributes.Pdeathsig != syscall.SIGKILL {
		t.Fatalf("sandbox process attributes = %#v", attributes)
	}
}

func TestSpontaneousNonzeroExitCannotMasqueradeAsEngineStop(t *testing.T) {
	requireTrustedContainment(t)
	submissionValue := submissionFixture(
		t,
		"invoke-submit-exit-17",
		PlannerProposal,
		"",
	)
	submission := &submissionValue
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-submit-exit-17",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"submit-exit-17",
		submission,
	)
	setProcessExecutable(t, &invocation, "driver")
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "PROCESS_FAILED") || observation.Handoff != nil ||
		eventIndex(observation.Events, "submit_acknowledged") < 0 ||
		eventIndex(observation.Events, "fatal:process_failed") < 0 {
		t.Fatalf("observation=%#v error=%v", observation, err)
	}
	t.Logf("spontaneous_exit_refused=true events=%v", observation.Events)
}

func TestSubmitWithoutCompletedResultStaysBlockedUntilDeadline(t *testing.T) {
	requireTrustedContainment(t)
	submissionValue := submissionFixture(
		t,
		"invoke-submit-no-result",
		PlannerProposal,
		"",
	)
	submission := &submissionValue
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-submit-no-result",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"submit-no-result",
		submission,
	)
	setProcessExecutable(t, &invocation, "driver")
	invocation.Request.Limits.TimeoutMillis = 250
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
	started := time.Now()
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "INVOCATION_TIMEOUT") || observation.Handoff != nil ||
		time.Since(started) < 200*time.Millisecond {
		t.Fatalf("observation=%#v elapsed=%s error=%v", observation, time.Since(started), err)
	}
}

func TestNonCompletedTransportCannotReleaseAcceptedSubmission(t *testing.T) {
	requireTrustedContainment(t)
	profiles := []FakeProfile{
		FakeTransportError,
		FakeTimeout,
		FakeCancelled,
		FakeRunnerError,
	}
	for _, profile := range profiles {
		profile := profile
		t.Run(string(profile), func(t *testing.T) {
			invocationID := "invoke-transport-" + strings.ReplaceAll(string(profile), "_", "-")
			submissionValue := submissionFixture(t, invocationID, PlannerProposal, "")
			submission := &submissionValue
			invocation, _, _ := fakeInvocation(
				t,
				invocationID,
				RolePlanner,
				PlannerProposal,
				ReadWrite,
				"submit",
				submission,
			)
			invocation.FakeProfile = profile
			observation, err := (Invoker{}).Invoke(context.Background(), invocation)
			if err == nil || observation.Handoff != nil ||
				(!IsCode(err, "TRANSPORT_FAILURE") &&
					!IsCode(err, "SUBMISSION_PROTOCOL_FAILED")) {
				t.Fatalf("observation=%#v error=%v", observation, err)
			}
		})
	}
}

func TestInvokerReadOnlyWriteAttemptAndCancellationFailClosed(t *testing.T) {
	requireTrustedContainment(t)
	t.Run("already cancelled", func(t *testing.T) {
		invocation, workspace, _ := fakeInvocation(
			t,
			"invoke-already-cancelled",
			RoleVerifier,
			WorkVerification,
			ReadOnly,
			"none",
			nil,
		)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := (Invoker{}).Invoke(ctx, invocation); !IsCode(err, "INVOCATION_CANCELLED") {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Lstat(filepath.Join(workspace, ".sworn-inputs")); !os.IsNotExist(err) {
			t.Fatal("cancelled-before-start invocation staged an input mount")
		}
	})
	t.Run("write", func(t *testing.T) {
		invocation, workspace, _ := fakeInvocation(
			t,
			"invoke-write-attempt",
			RoleVerifier,
			WorkVerification,
			ReadOnly,
			"attempt_workspace_write",
			nil,
		)
		if observation, err := (Invoker{}).Invoke(context.Background(), invocation); err == nil ||
			observation.Handoff != nil {
			t.Fatalf("observation=%#v error=%v", observation, err)
		}
		if _, err := os.Lstat(filepath.Join(workspace, ".sworn-fake-write-canary")); !os.IsNotExist(err) {
			t.Fatal("read-only child changed the workspace")
		}
	})
	t.Run("cancel blocking child", func(t *testing.T) {
		invocation, workspace, _ := fakeInvocation(
			t,
			"invoke-block",
			RoleVerifier,
			WorkVerification,
			ReadOnly,
			"block",
			nil,
		)
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		started := time.Now()
		observation, err := (Invoker{}).Invoke(ctx, invocation)
		if !IsCode(err, "INVOCATION_TIMEOUT") {
			t.Fatalf("observation = %#v, error = %v", observation, err)
		}
		if time.Since(started) > 3*time.Second {
			t.Fatal("process tree did not terminate within the bounded grace interval")
		}
		if _, err := os.Lstat(filepath.Join(workspace, ".sworn-fake-write-canary")); !os.IsNotExist(err) {
			t.Fatal("cancelled child changed the workspace")
		}
	})
	t.Run("cancel blocking descendant", func(t *testing.T) {
		invocation, _, _ := fakeInvocation(
			t,
			"invoke-block-descendant",
			RoleVerifier,
			WorkVerification,
			ReadOnly,
			"block-descendant",
			nil,
		)
		executable := setProcessExecutable(t, &invocation, "block-descendant")
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if _, err := (Invoker{}).Invoke(ctx, invocation); !IsCode(err, "INVOCATION_TIMEOUT") {
			t.Fatalf("error = %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for processUsesExecutable(executable) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if processUsesExecutable(executable) {
			t.Fatal("a descendant survived process-group cancellation")
		}
	})
}

func processUsesExecutable(executable string) bool {
	expected, err := os.Stat(executable)
	if err != nil {
		return false
	}
	entries, err := filepath.Glob("/proc/[0-9]*/exe")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		observed, err := os.Stat(entry)
		if err == nil && os.SameFile(expected, observed) {
			return true
		}
	}
	return false
}

func TestInvokerRejectsMalformedSubmissionChannel(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-malformed-frame",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"malformed_submission_frame",
		nil,
	)
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "SUBMISSION_PROTOCOL_FAILED") ||
		observation.Diagnostic.Code != "submission_protocol_failed" ||
		observation.Handoff != nil {
		t.Fatalf("observation = %#v, error = %v", observation, err)
	}
}

func TestMalformedControlTerminatesBlockingProcessImmediately(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-malformed-control-block",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"malformed-control-block",
		nil,
	)
	setProcessExecutable(t, &invocation, "driver")
	started := time.Now()
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	elapsed := time.Since(started)
	if !IsCode(err, "SUBMISSION_PROTOCOL_FAILED") || observation.Handoff != nil ||
		elapsed >= 2*time.Second {
		t.Fatalf("observation=%#v error=%v elapsed=%s", observation, err, elapsed)
	}
	t.Logf("malformed_control_immediate_termination=true elapsed=%s events=%v", elapsed, observation.Events)
}

func TestInvokerRejectsMalformedProcessBehaviors(t *testing.T) {
	requireTrustedContainment(t)
	tests := []struct {
		behavior string
		code     string
	}{
		{"crash", "INVALID_RESULT"},
		{"missing-result", "INVALID_RESULT"},
		{"malformed-json", "INVALID_JSON"},
		{"nonzero-stdout", "MISSING_FIELD"},
		{"oversized-stdout", "OUTPUT_OVERFLOW"},
		{"oversized-stderr", "OUTPUT_OVERFLOW"},
		{"multiple-results", "PROTOCOL_FAILURE"},
		{"wrong-binding", "RESULT_BINDING_MISMATCH"},
	}
	for _, test := range tests {
		t.Run(test.behavior, func(t *testing.T) {
			invocation, _, _ := fakeInvocation(
				t,
				"invoke-"+strings.ReplaceAll(test.behavior, "_", "-"),
				RolePlanner,
				PlannerProposal,
				ReadWrite,
				test.behavior,
				nil,
			)
			setProcessExecutable(t, &invocation, test.behavior)
			observation, err := (Invoker{}).Invoke(context.Background(), invocation)
			if !IsCode(err, test.code) || observation.Handoff != nil {
				t.Fatalf("observation=%#v error=%v, want %s", observation, err, test.code)
			}
			if strings.Contains(test.behavior, "oversized") {
				t.Logf("%s hard_counter_terminated=true events=%v", test.behavior, observation.Events)
			}
		})
	}
	t.Run("bounded stderr", func(t *testing.T) {
		invocation, _, _ := fakeInvocation(
			t,
			"invoke-stderr-noise",
			RolePlanner,
			PlannerProposal,
			ReadWrite,
			"stderr-noise",
			nil,
		)
		setProcessExecutable(t, &invocation, "stderr-noise")
		observation, err := (Invoker{}).Invoke(context.Background(), invocation)
		if !IsCode(err, "MISSING_SUBMISSION") || observation.Handoff != nil ||
			observation.Diagnostic.Code != "submission_absent" ||
			observation.Diagnostic.StderrBytes != 4_096 ||
			!observation.Diagnostic.Truncated {
			t.Fatalf("observation=%#v error=%v", observation, err)
		}
	})
}

func TestParallelInvocationsCannotExchangeInputsModelsOrSeals(t *testing.T) {
	requireTrustedContainment(t)
	type invocationCase struct {
		id             string
		model          string
		invocation     Invocation
		workspace      string
		submissionBody []byte
	}
	cases := make([]invocationCase, 0, 2)
	for _, id := range []string{"parallel-one", "parallel-two"} {
		submissionValue := submissionFixture(t, id, PlannerProposal, "")
		submission := &submissionValue
		invocation, workspace, submissionBody := fakeInvocation(
			t,
			id,
			RolePlanner,
			PlannerProposal,
			ReadWrite,
			"submit",
			submission,
		)
		invocation.Selected.Model = "model-" + id
		invocation.Request.Model = invocation.Selected.Model
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
		cases = append(cases, invocationCase{
			id:             id,
			model:          invocation.Selected.Model,
			invocation:     invocation,
			workspace:      workspace,
			submissionBody: submissionBody,
		})
	}
	type result struct {
		observation Observation
		err         error
	}
	results := make([]result, len(cases))
	var wait sync.WaitGroup
	for index := range cases {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index].observation, results[index].err =
				(Invoker{}).Invoke(context.Background(), cases[index].invocation)
		}()
	}
	wait.Wait()
	for index, result := range results {
		if result.err != nil {
			t.Fatalf("%s error = %v", cases[index].id, result.err)
		}
		handoff := result.observation.Handoff
		if handoff == nil ||
			!bytes.Equal(handoff.SubmissionBytes, cases[index].submissionBody) ||
			(index == 0 && bytes.Equal(handoff.SubmissionBytes, cases[1].submissionBody)) ||
			(index == 1 && bytes.Equal(handoff.SubmissionBytes, cases[0].submissionBody)) {
			t.Fatalf("%s handoff = %#v", cases[index].id, handoff)
		}
		if _, err := os.Lstat(filepath.Join(cases[index].workspace, ".sworn-inputs")); !os.IsNotExist(err) {
			t.Fatalf("%s retained another invocation's projection", cases[index].id)
		}
	}
}

func TestObservationDoesNotRetainEnvironmentStderrOrRawTranscript(t *testing.T) {
	requireTrustedContainment(t)
	t.Setenv("SWORN_PARENT_SECRET", "parent-secret-sentinel")
	for _, behavior := range []string{"environment-canary", "secret-text"} {
		t.Run(behavior, func(t *testing.T) {
			invocation, _, _ := fakeInvocation(
				t,
				"invoke-"+behavior,
				RolePlanner,
				PlannerProposal,
				ReadWrite,
				behavior,
				nil,
			)
			setProcessExecutable(t, &invocation, behavior)
			observation, err := (Invoker{}).Invoke(context.Background(), invocation)
			if behavior == "secret-text" {
				if !IsCode(err, "UNKNOWN_FIELD") || observation.Handoff != nil {
					t.Fatalf("raw transcript observation=%#v error=%v", observation, err)
				}
			} else if !IsCode(err, "MISSING_SUBMISSION") {
				t.Fatalf("observation=%#v error=%v", observation, err)
			}
			serialized, err := json.Marshal(observation)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{
				"parent-secret-sentinel",
				"hostile-secret-sentinel",
				"Fake completed response",
			} {
				if bytes.Contains(serialized, []byte(forbidden)) {
					t.Fatalf("observation retained %q: %s", forbidden, serialized)
				}
			}
			if behavior == "environment-canary" &&
				observation.Diagnostic.StderrBytes != 0 {
				t.Fatalf("parent environment reached child: %#v", observation)
			}
		})
	}
}

func TestCompletedResultKeepsOmittedUsageExplicitlyUnavailable(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-usage-unavailable",
		RolePlanner,
		PlannerProposal,
		ReadOnly,
		"usage_unavailable",
		nil,
	)
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "MISSING_SUBMISSION") ||
		observation.TransportStatus != Completed ||
		observation.Usage.TokenStatus != UsageUnavailable ||
		observation.Usage.CostStatus != UsageUnavailable ||
		observation.Usage.InputTokens != nil ||
		observation.Usage.CostMicroUnits != nil {
		t.Fatalf("observation=%#v error=%v", observation, err)
	}
}

func TestLinuxBoundaryExposesOnlyFixedEnvironmentWorkspaceAndInputOverlay(t *testing.T) {
	requireTrustedContainment(t)
	invocation, workspace, _ := fakeInvocation(
		t,
		"invoke-isolation-canaries",
		RoleVerifier,
		WorkVerification,
		ReadOnly,
		"isolation-canaries",
		nil,
	)
	markedWorkspace := filepath.Join(workspace, "sworn-host-path-canary")
	if err := os.Mkdir(markedWorkspace, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace = markedWorkspace
	invocation.HostWorkspace = workspace
	if err := os.WriteFile(filepath.Join(workspace, ".git"), []byte("git-canary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, reserved := range []string{".baton/releases", ".sworn"} {
		if err := os.MkdirAll(filepath.Join(workspace, reserved), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, reserved, "authority-canary"), []byte("forbidden\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setProcessExecutable(t, &invocation, "isolation-canaries")
	bwrap, err := trustedBubblewrap()
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := bubblewrapArguments(invocation)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(arguments, "\x00"), workspace) {
		t.Fatal("canonical host workspace leaked into child argv")
	}
	requestBody, err := EncodeRequest(invocation.Request)
	if err != nil || bytes.Contains(requestBody, []byte(workspace)) {
		t.Fatal("canonical host workspace leaked into child request")
	}
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "MISSING_SUBMISSION") {
		t.Fatalf("observation=%#v error=%v", observation, err)
	}
	if observation.TransportStatus != Completed ||
		observation.Diagnostic.Code != "submission_absent" ||
		observation.Diagnostic.StderrBytes != 0 ||
		observation.Handoff != nil {
		t.Fatalf("observation=%#v", observation)
	}
	observationBody, err := json.Marshal(observation)
	if err != nil || bytes.Contains(observationBody, []byte(workspace)) {
		t.Fatal("canonical host workspace leaked into diagnostics")
	}
	t.Logf(
		"bubblewrap=%s trusted_root_owned=true common_features=fd-bind,private-pid,drop-caps fake_mount_map=%q host_path_absent=request,argv,environment,diagnostics,mountinfo reserved_canaries_hidden=true production_profile_certification=false",
		bwrap,
		arguments,
	)
}

func TestFakeCommandHasExactCommandsAndBoundedDiagnostics(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := RunFakeCommand(
		[]string{"info"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
		FakeCompleted,
	); exit != 0 || stderr.Len() != 0 ||
		stdout.String() != `{"contract_version":"sworn.driver/v1","adapter_id":"baton.fake","adapter_version":"1.0.0"}`+"\n" {
		t.Fatalf("info exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	requestBody, err := EncodeRequest(contractRequest(t, RolePlanner))
	if err != nil {
		t.Fatal(err)
	}
	if exit := RunFakeCommand(
		[]string{"run"},
		bytes.NewReader(requestBody),
		&stdout,
		&stderr,
		FakeCompleted,
	); exit != 0 || stderr.Len() != 0 {
		t.Fatalf("run exit=%d stderr=%q", exit, stderr.String())
	}
	if _, err := DecodeResult(stdout.Bytes(), ResultBinding{
		InvocationID:   "invocation-001",
		AdapterID:      FakeDriverID,
		AdapterVersion: FakeDriverVersion,
	}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if exit := RunFakeCommand(
		[]string{"complete"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
		FakeCompleted,
	); exit == 0 || stdout.Len() != 0 || stderr.Len() > MaxStderrRetain {
		t.Fatalf("invalid exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
}
