//go:build linux

package driver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// enableUncontainedDispatch turns on the link-time test-only gate for the
// duration of one test and restores the previous value afterwards. Unit-test
// builds link "" by default, which is exactly what the isolation tests depend
// on, so this helper is only ever used by non-parallel tests that own the gate
// for their own duration.
func enableUncontainedDispatch(t *testing.T) {
	t.Helper()
	previous := testUncontainedDispatch
	t.Cleanup(func() { testUncontainedDispatch = previous })
	testUncontainedDispatch = "1"
	t.Setenv(testUncontainedDispatchEnv, "1")
}

// TestUncontainedDispatchRefusedWhenGateIsNotLinked pins A4 at the unit level:
// a binary whose gate is not linked (the unit-test default) refuses the
// uncontained dispatch request before any driver or sandbox interaction, and
// the refusal surfaces as the stable contract error.
func TestUncontainedDispatchRefusedWhenGateIsNotLinked(t *testing.T) {
	previous := testUncontainedDispatch
	testUncontainedDispatch = ""
	t.Cleanup(func() { testUncontainedDispatch = previous })
	t.Setenv(testUncontainedDispatchEnv, "1")

	invocation, _, _ := fakeInvocation(
		t,
		"invoke-uncontained-refusal",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"none",
		nil,
	)
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "UNCONTAINED_DISPATCH_REFUSED") {
		t.Fatalf("error = %v, observation = %#v", err, observation)
	}
	if observation.TransportStatus != RunnerError ||
		observation.Diagnostic.Code != "adapter_failed" {
		t.Fatalf("observation = %#v", observation)
	}
	if !uncontainedDispatchRequested() || uncontainedDispatchEnabled() {
		t.Fatal("gate state did not reflect a refused request")
	}
}

// TestUncontainedDispatchExecutesFakeDriverThroughDirectExec drives the full
// uncontained dispatch end to end with the fake driver: the child resolves the
// fake script through the engine-set input override, submits through the
// endpoint, and the parent-side arbiter, process group and workspace postcheck
// machinery complete with a sealed handoff.
func TestUncontainedDispatchExecutesFakeDriverThroughDirectExec(t *testing.T) {
	enableUncontainedDispatch(t)

	submissionValue := submissionFixture(
		t,
		"invoke-uncontained-submit",
		PlannerProposal,
		"",
	)
	submission := &submissionValue
	invocation, _, submissionBody := fakeInvocation(
		t,
		"invoke-uncontained-submit",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"submit",
		submission,
	)
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if observation.TransportStatus != Completed ||
		observation.Handoff == nil ||
		!bytes.Equal(observation.Handoff.SubmissionBytes, submissionBody) ||
		observation.Handoff.SubmissionDigest != Digest(submissionBody) ||
		observation.Handoff.SealDigest != Digest(observation.Handoff.SealBytes) ||
		observation.Diagnostic.Code != "none" {
		t.Fatalf("observation = %#v", observation)
	}
	for _, kind := range []string{
		"result_completed",
		"submit_accepted_pending",
		"submit_acknowledged",
		"engine_stop_after_submit",
		"published",
		"process_group_quiescent",
		"workspace_postcheck",
		"input_projection_removed",
		"producers_joined",
	} {
		if eventIndex(observation.Events, kind) < 0 {
			t.Fatalf("uncontained dispatch missing %q: events = %#v", kind, observation.Events)
		}
	}
}

// TestUncontainedDispatchRemapsGuestWorkspaceForFakeWrites proves the
// workspace path remap: in an uncontained dispatch the fake driver's workspace
// write lands in the real host workspace, which the child can only reach
// through the engine-set SWORN_TEST_GUEST_WORKSPACE override.
func TestUncontainedDispatchRemapsGuestWorkspaceForFakeWrites(t *testing.T) {
	enableUncontainedDispatch(t)

	invocation, workspace, _ := fakeInvocation(
		t,
		"invoke-uncontained-write",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"attempt_workspace_write",
		nil,
	)
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "MISSING_SUBMISSION") {
		t.Fatalf("error = %v, observation = %#v", err, observation)
	}
	canary := filepath.Join(workspace, ".sworn-fake-write-canary")
	body, readErr := os.ReadFile(canary)
	if readErr != nil || string(body) != "write\n" {
		t.Fatalf(
			"uncontained fake write did not land in the host workspace: %v",
			readErr,
		)
	}
}

// TestUncontainedCommandUsesControlledEnvironmentAndOwnProcessGroup pins the
// uncontained command construction: direct exec of the validated driver, cwd
// and PWD at the host workspace, a fully controlled environment (nothing
// inherited from the parent), the submission protocol, the fake profile, the
// marker and the guest-path overrides, and a single extra fd for the child
// submission endpoint.
func TestUncontainedCommandUsesControlledEnvironmentAndOwnProcessGroup(t *testing.T) {
	enableUncontainedDispatch(t)

	invocation, _, _ := fakeInvocation(
		t,
		"invoke-uncontained-command",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"none",
		nil,
	)
	executablePath := buildFakeExecutable(t)
	digest, err := executableDigest(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	identity := ExecutableIdentity{Path: executablePath, Digest: digest}
	projection, err := StageInputProjection(
		invocation.HostWorkspace,
		invocation.Request.Inputs,
		invocation.Inputs,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	requestBody, err := EncodeRequest(invocation.Request)
	if err != nil {
		t.Fatal(err)
	}
	parentEndpoint, childEndpoint, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer parentEndpoint.Close()
	defer childEndpoint.Close()

	command, err := uncontainedCommand(
		invocation,
		identity,
		projection,
		requestBody,
		childEndpoint,
	)
	if err != nil {
		t.Fatal(err)
	}
	if command.Dir != invocation.HostWorkspace {
		t.Fatalf("command cwd = %q, want %q", command.Dir, invocation.HostWorkspace)
	}
	if len(command.Args) != 2 || command.Args[0] != executablePath ||
		command.Args[1] != "run" {
		t.Fatalf("command args = %q", command.Args)
	}
	if len(command.ExtraFiles) != 1 || command.ExtraFiles[0] != childEndpoint {
		t.Fatalf("extra files = %v, want the child endpoint only", command.ExtraFiles)
	}
	if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid ||
		command.SysProcAttr.Pdeathsig == 0 {
		t.Fatalf("sysprocattr = %#v", command.SysProcAttr)
	}
	expected := map[string]string{
		"HOME":                           "/home/sworn",
		"TMPDIR":                         "/tmp",
		"LANG":                           "C.UTF-8",
		"LC_ALL":                         "C.UTF-8",
		"TZ":                             "UTC",
		"PATH":                           "/usr/bin:/bin",
		"PWD":                            invocation.HostWorkspace,
		SubmissionProtocolEnvironment:    SubmissionControlVersion,
		SubmissionFDEnvironment:          "3",
		"BATON_FAKE_PROFILE":             string(invocation.FakeProfile),
		testUncontainedDispatchEnv:       "1",
		testUncontainedGuestWorkspaceEnv: invocation.HostWorkspace,
		testUncontainedGuestInputsEnv:    projection.Root(),
	}
	if len(command.Env) != len(expected) {
		t.Fatalf("environment length = %d, want %d: %q", len(command.Env), len(expected), command.Env)
	}
	for _, entry := range command.Env {
		key, value, found := strings.Cut(entry, "=")
		if !found || expected[key] != value {
			t.Fatalf("unexpected environment entry %q", entry)
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		t.Fatalf("controlled environment missing: %v", expected)
	}
}

// TestContainedBubblewrapArgumentsNeverCarryUncontainedVariables pins bounded
// correction 1: the contained branch's fixed --setenv list must never acquire
// the uncontained marker or the guest-path overrides, so the "parent
// environment never reaches the child" invariant holds on the contained path.
func TestContainedBubblewrapArgumentsNeverCarryUncontainedVariables(t *testing.T) {
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-contained-args",
		RolePlanner,
		PlannerProposal,
		ReadWrite,
		"none",
		nil,
	)
	arguments, err := bubblewrapArguments(invocation)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, "\x00")
	for _, name := range []string{
		testUncontainedDispatchEnv,
		testUncontainedGuestWorkspaceEnv,
		testUncontainedGuestInputsEnv,
	} {
		if strings.Contains(joined, name) {
			t.Fatalf("contained bwrap arguments carried %s", name)
		}
	}
}
