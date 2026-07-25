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
	"testing"
	"time"
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
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	invocation.Selected.Provider.Executable = ExecutableIdentity{
		Path:   executable,
		Digest: digest,
	}
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
	return executable
}

type buildFailure struct {
	output string
}

func (failure *buildFailure) Error() string {
	return "build fake executable failed: " + failure.output
}

func executableConfig(t *testing.T) ProviderConfig {
	t.Helper()
	executable := buildFakeExecutable(t)
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	return ProviderConfig{
		Key:           "fake",
		DriverID:      FakeDriverID,
		DriverVersion: FakeDriverVersion,
		Executable: ExecutableIdentity{
			Path:   executable,
			Digest: digest,
		},
		Network: NetworkNone,
	}
}

func TestFakeBubblewrapArgumentsCannotSelectSharedNetwork(t *testing.T) {
	t.Parallel()
	invocation := Invocation{
		Request: Request{
			Workspace: Workspace{Path: "/workspace/project", Access: ReadOnly},
		},
		Selected: SelectedProvider{
			Provider: ProviderConfig{
				DriverID: FakeDriverID,
				Network:  NetworkNone,
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

	invocation.Selected.Provider.Network = NetworkRequired
	arguments, err = bubblewrapArguments(invocation)
	if !IsCode(err, "INVALID_NETWORK_POLICY") || arguments != nil {
		t.Fatalf("networked fake arguments = %q, error = %v", arguments, err)
	}

	invocation.Selected.Provider.DriverID = "vendor.driver"
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
	model := "fake-model-v1"
	request, err := NewRequest(
		invocationID,
		role,
		&model,
		Workspace{Path: workspace, Access: access},
		[]Input{input},
		true,
		Limits{TimeoutMillis: 5_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected := SelectedProvider{Provider: executableConfig(t), Model: model}
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(request, selected, containment, responsibility)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:     request,
		Selected:    selected,
		Permission:  permission,
		Inputs:      []InputContent{content},
		FakeProfile: FakeCompleted,
	}, workspace, submissionBody
}

func TestInvokerReleasesOnlyCompletedBoundSealedHandoff(t *testing.T) {
	submission := &Submission{
		SchemaVersion: SubmissionSchemaVersion,
		InvocationID:  "invoke-submit",
		Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
	}
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
	if observation.TextBytes == 0 || observation.TextDigest == "" ||
		observation.Usage.InputTokens == nil || *observation.Usage.InputTokens != 0 ||
		observation.Diagnostic.Code != "none" ||
		bytes.Contains(observation.Handoff.SealBytes, []byte("Fake completed response")) {
		t.Fatalf("observation leaked or lost normalized facts: %#v", observation)
	}
	after, err := captureWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !equalManifest(before, after) {
		t.Fatal("completed invocation changed the workspace outside its private mount")
	}
}

func TestNonCompletedTransportCannotReleaseAcceptedSubmission(t *testing.T) {
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
			submission := &Submission{
				SchemaVersion: SubmissionSchemaVersion,
				InvocationID:  invocationID,
				Artifacts:     []Artifact{artifactFixture(t, ArtifactPlan)},
			}
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
			if err != nil {
				t.Fatal(err)
			}
			if observation.TransportStatus != TransportStatus(profile) ||
				observation.Handoff != nil {
				t.Fatalf("observation=%#v", observation)
			}
		})
	}
}

func TestInvokerReadOnlyWriteAttemptAndCancellationFailClosed(t *testing.T) {
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
		if _, err := (Invoker{}).Invoke(context.Background(), invocation); !IsCode(err, "PROCESS_FAILED") {
			t.Fatalf("error = %v", err)
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

func TestInvokerRejectsMalformedProcessBehaviors(t *testing.T) {
	tests := []struct {
		behavior string
		code     string
	}{
		{"crash", "PROCESS_FAILED"},
		{"missing-result", "MISSING_JSON"},
		{"malformed-json", "INVALID_JSON"},
		{"nonzero-stdout", "PROTOCOL_FAILURE"},
		{"oversized-stdout", "OUTPUT_OVERFLOW"},
		{"multiple-results", "TRAILING_JSON"},
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
		if err != nil || observation.Handoff != nil ||
			observation.Diagnostic.Code != "submission_absent" ||
			observation.Diagnostic.StderrBytes != 4_096 ||
			!observation.Diagnostic.Truncated {
			t.Fatalf("observation=%#v error=%v", observation, err)
		}
	})
}

func TestParallelInvocationsCannotExchangeInputsModelsOrSeals(t *testing.T) {
	type invocationCase struct {
		id             string
		model          string
		invocation     Invocation
		workspace      string
		submissionBody []byte
	}
	cases := make([]invocationCase, 0, 2)
	for _, id := range []string{"parallel-one", "parallel-two"} {
		artifact, err := NewArtifact(ArtifactPlan, []byte("plan for "+id+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		submission := &Submission{
			SchemaVersion: SubmissionSchemaVersion,
			InvocationID:  id,
			Artifacts:     []Artifact{artifact},
		}
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
		invocation.Request.Model = pointer(invocation.Selected.Model)
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

func TestObservationDoesNotRetainEnvironmentStderrOrResultText(t *testing.T) {
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
			if err != nil {
				t.Fatal(err)
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
			if behavior == "secret-text" &&
				(observation.TextBytes == 0 ||
					observation.TextDigest != Digest([]byte("hostile-secret-sentinel"))) {
				t.Fatalf("text was not reduced to facts: %#v", observation)
			}
		})
	}
}

func TestLinuxBoundaryExposesOnlyFixedEnvironmentWorkspaceAndInputOverlay(t *testing.T) {
	invocation, _, _ := fakeInvocation(
		t,
		"invoke-isolation-canaries",
		RoleVerifier,
		WorkVerification,
		ReadOnly,
		"isolation-canaries",
		nil,
	)
	setProcessExecutable(t, &invocation, "isolation-canaries")
	observation, err := (Invoker{}).Invoke(context.Background(), invocation)
	if err != nil {
		t.Fatalf("observation=%#v error=%v", observation, err)
	}
	if observation.TransportStatus != Completed ||
		observation.Diagnostic.Code != "submission_absent" ||
		observation.Diagnostic.StderrBytes != 0 ||
		observation.Handoff != nil {
		t.Fatalf("observation=%#v", observation)
	}
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
		stdout.String() != `{"contract_version":"baton.driver/v1","driver_id":"baton.fake","driver_version":"1.0.0"}`+"\n" {
		t.Fatalf("info exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	requestBody, err := EncodeRequest(contractRequest(t, RoleMerge, nil))
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
		InvocationID:  "invocation-001",
		DriverID:      FakeDriverID,
		DriverVersion: FakeDriverVersion,
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
