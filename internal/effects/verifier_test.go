package effects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
)

type verifierTestRecord struct {
	kind     string
	contents []byte
}

type verifierTestControl struct {
	plan      protocol.ExactPlan
	records   map[string]verifierTestRecord
	artifacts map[string]memoryArtifact
	builder   engine.JournalEffect
}

func (control *verifierTestControl) Plan(_ context.Context, digest string) (protocol.ExactPlan, error) {
	if digest != control.plan.Record().Digest {
		return protocol.ExactPlan{}, errors.New("test plan not found")
	}
	return control.plan, nil
}

func (control *verifierTestControl) Record(_ context.Context, digest string) (string, []byte, error) {
	record, exists := control.records[digest]
	if !exists {
		return "", nil, errors.New("test record not found")
	}
	return record.kind, bytes.Clone(record.contents), nil
}

func (control *verifierTestControl) PutArtifact(
	_ context.Context,
	mediaType string,
	contents []byte,
) (string, error) {
	digest := protocol.RawDigest(contents)
	if existing, exists := control.artifacts[digest]; exists &&
		(existing.mediaType != mediaType || !bytes.Equal(existing.contents, contents)) {
		return "", errors.New("test artifact conflict")
	}
	control.artifacts[digest] = memoryArtifact{mediaType: mediaType, contents: bytes.Clone(contents)}
	return digest, nil
}

func (control *verifierTestControl) Artifact(
	_ context.Context,
	digest string,
) (string, []byte, error) {
	artifact, exists := control.artifacts[digest]
	if !exists {
		return "", nil, errors.New("test artifact not found")
	}
	return artifact.mediaType, bytes.Clone(artifact.contents), nil
}

func (control *verifierTestControl) SucceededEffect(
	_ context.Context,
	effectID string,
) (engine.JournalEffect, error) {
	if effectID != control.builder.ID {
		return engine.JournalEffect{}, errors.New("test effect not found")
	}
	return control.builder, nil
}

func (control *verifierTestControl) putArtifact(
	t *testing.T,
	mediaType string,
	contents []byte,
) protocol.Artifact {
	t.Helper()
	digest, err := control.PutArtifact(context.Background(), mediaType, contents)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Artifact{Ref: digest, MediaType: mediaType, Digest: digest}
}

type verifierTestAdapter struct {
	profile    protocol.VerifierProfile
	assessment []byte
	parseErr   error
	parseCalls int
}

func (adapter *verifierTestAdapter) Profile() protocol.VerifierProfile {
	profile := adapter.profile
	profile.Argv = slices.Clone(profile.Argv)
	profile.EnvironmentNames = slices.Clone(profile.EnvironmentNames)
	return profile
}

func (adapter *verifierTestAdapter) Invocation(
	identity engine.VerifierAttemptIdentity,
	workspace repo.CandidateWorkspace,
	engineInputs []executor.Input,
) (executor.Invocation, error) {
	inputs := append(slices.Clone(engineInputs), executor.Input{
		Name: adapter.profile.ExecutableInput,
		Path: adapter.profile.BinaryPath, Digest: adapter.profile.BinaryDigest,
	})
	slices.SortFunc(inputs, func(left, right executor.Input) int {
		return strings.Compare(left.Name, right.Name)
	})
	return executor.Invocation{
		SchemaVersion: executor.InvocationSchemaVersion,
		ID:            identity.InvocationID, Role: "verifier",
		NestedSandbox: true, CredentialAccess: true,
		Workspace: workspace.Path(), WorkspaceDigest: workspace.Manifest(),
		WorkspaceAccess: executor.WorkspaceReadOnly,
		ExecutableInput: adapter.profile.ExecutableInput,
		Inputs:          inputs,
		Argv:            slices.Clone(adapter.profile.Argv),
		Network:         executor.NetworkHost,
		Timeout:         time.Duration(adapter.profile.TimeoutNanoseconds),
	}, nil
}

func (adapter *verifierTestAdapter) ParseCompletion(
	executor.RawCompletion,
) (VerifierAdapterCompletion, error) {
	adapter.parseCalls++
	if adapter.parseErr != nil {
		return VerifierAdapterCompletion{}, adapter.parseErr
	}
	return VerifierAdapterCompletion{
		Assessment: bytes.Clone(adapter.assessment), ThreadID: "thread-verifier-test",
	}, nil
}

type verifierTestRunner struct {
	reconciler          *executor.LinuxExecutor
	configurationDigest string
	limits              executor.Limits
	runErr              error
	reconcileErr        error
	mutate              func(*executor.RawCompletion)
	invocations         []executor.Invocation
	reconciled          []string
	events              []string
}

func (runner *verifierTestRunner) ConfigurationDigest() string { return runner.configurationDigest }
func (runner *verifierTestRunner) EffectiveLimits() executor.Limits {
	return runner.limits
}

func (runner *verifierTestRunner) RunCredentialReadOnly(
	_ context.Context,
	invocation executor.Invocation,
) (executor.RawCompletion, error) {
	runner.events = append(runner.events, "run")
	runner.invocations = append(runner.invocations, cloneVerifierInvocation(invocation))
	bound := make([]executor.BoundInput, len(invocation.Inputs))
	for index, input := range invocation.Inputs {
		info, err := os.Lstat(input.Path)
		if err != nil {
			return executor.RawCompletion{}, err
		}
		bound[index] = executor.BoundInput{Name: input.Name, Digest: input.Digest, Size: uint64(info.Size())}
	}
	completion := executor.RawCompletion{
		InvocationID:     invocation.ID,
		Unit:             "sworn-verifier-test.service",
		WorkspaceDigest:  invocation.WorkspaceDigest,
		WorkspaceAccess:  executor.WorkspaceReadOnly,
		CredentialAccess: true, ExecutableInput: invocation.ExecutableInput,
		Inputs:      bound,
		StartedAt:   time.Date(2026, 7, 20, 0, 6, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 7, 20, 0, 7, 0, 0, time.UTC),
		ExitCode:    0,
		Stdout:      []byte("model event stream\n"),
		Stderr:      []byte("diagnostic\n"),
	}
	if runner.mutate != nil {
		runner.mutate(&completion)
	}
	return completion, runner.runErr
}

func (runner *verifierTestRunner) ReconcileContentBound(
	ctx context.Context,
	invocationID string,
) (executor.ContentBoundCleanup, error) {
	runner.events = append(runner.events, "reconcile")
	runner.reconciled = append(runner.reconciled, invocationID)
	if runner.reconcileErr != nil {
		return executor.ContentBoundCleanup{}, runner.reconcileErr
	}
	return runner.reconciler.ReconcileContentBound(ctx, invocationID)
}

type verifierFixture struct {
	worker     VerifierWorker
	control    *verifierTestControl
	runner     *verifierTestRunner
	adapter    *verifierTestAdapter
	repository *repo.Repository
	candidate  repo.Candidate
	effect     engine.JournalEffect
}

func newVerifierFixture(t *testing.T) verifierFixture {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("verifier cleanup proof requires Linux")
	}
	ctx := context.Background()
	repository, candidate := effectCandidate(t)
	control := &verifierTestControl{
		records: make(map[string]verifierTestRecord), artifacts: make(map[string]memoryArtifact),
	}

	definitionBytes, err := protocol.EncodeCanonical(protocol.LocalCheckDefinition{
		SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion,
		Argv:          []string{"/usr/bin/true"}, WorkingDirectory: ".", TimeoutSeconds: 30,
		Evidence: protocol.LocalEvidenceDefinition{
			ID: "evidence-1", AcceptanceIDs: []string{"AC1"},
			Boundary: "assembled", Observed: "the candidate passed",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := control.putArtifact(t, "application/json", definitionBytes)
	policyBytes, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": protocol.AssurancePolicySchemaVersion,
		"policy_id":      "standard",
		"checks": []any{map[string]any{
			"id": "test",
			"definition": map[string]any{
				"ref": "policy/checks/test.json", "media_type": "application/json", "digest": definition.Digest,
			},
		}},
		"packs": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy := control.putArtifact(t, "application/json", policyBytes)
	grantValues := []map[string]any{
		{"action": "inspect", "target": "workspace"},
		{"action": "edit", "target": "workspace"},
		{"action": "execute", "target": "workspace"},
		{"action": "commit", "target": "workspace"},
	}
	planBytes, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": protocol.DeliveryPlanSchemaVersion,
		"delivery_id":    "delivery-1",
		"outcome":        "Produce the exact candidate.",
		"created_at":     "2026-07-20T00:00:00Z",
		"assurance_policy": map[string]any{
			"ref": "policy:standard", "digest": policy.Digest,
		},
		"target": map[string]any{"repository": candidate.RepositoryID, "ref": candidate.TargetRef},
		"authority": map[string]any{
			"ref": "authority-source", "grants": grantValues,
		},
		"work": []any{map[string]any{
			"id": "work-1", "outcome": "Produce the exact candidate.",
			"scope": map[string]any{"include": []string{"."}, "exclude": []string{}},
			"acceptance": []any{map[string]any{
				"id": "AC1", "criterion": "The exact candidate is proven.", "evidence_level": "assembled",
			}},
			"depends_on": []string{},
			"assurance":  map[string]any{"profile": "standard", "packs": []string{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := protocol.ParseDeliveryPlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	control.plan = plan
	grants := make([]json.RawMessage, len(plan.Authority().Grants))
	for index, grant := range plan.Authority().Grants {
		grants[index] = grant.CanonicalJSON()
	}
	authorityRecord, err := protocol.EncodeAuthorityApproval(protocol.AuthorityApproval{
		SchemaVersion: protocol.ControlReceiptSchemaVersion,
		Kind:          protocol.AuthorityApprovalKind,
		ReceiptID:     "authority-1",
		PlanDigest:    plan.Record().Digest, AuthorityDigest: plan.Authority().Digest,
		SourceRef: plan.Authority().SourceRef, SourceDigest: testEffectDigest("8"),
		Grants: grants, Repository: candidate.RepositoryID, TargetRef: candidate.TargetRef,
		AuthorizerRef: "identity:test", ApprovedAt: "2026-07-20T00:00:01Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	authority := control.putArtifact(t, "application/json", authorityRecord.CanonicalJSON)

	snapshotDigest, err := protocol.SnapshotDigest()
	if err != nil {
		t.Fatal(err)
	}
	runtimeDigest := testEffectDigest("9")
	limits := executor.DefaultLimits()
	environmentBytes, err := protocol.EncodeCanonical(protocol.LocalEnvironment{
		SchemaVersion:          protocol.ContentEnvironmentSchemaVersion,
		ProtocolSnapshotDigest: "sha256:" + snapshotDigest,
		EngineRuntime:          "go-test", OS: "linux", Architecture: "amd64",
		Executor: protocol.LocalExecutorProbe{
			BubblewrapVersion: "1.0", SystemdVersion: "257", CgroupV2: true,
			UserManager: "running", Controllers: []string{"cpu", "memory", "pids"},
		},
		ExecutorPolicyVersion: executor.ContainmentPolicyVersion,
		Limits: protocol.LocalExecutionLimits{
			RuntimeNanoseconds: limits.Runtime.Nanoseconds(),
			MemoryBytes:        limits.MemoryBytes, SwapBytes: limits.SwapBytes,
			Tasks: limits.Tasks, CPUPercent: limits.CPUPercent,
			FileBytes: limits.FileBytes, TempBytes: limits.TempBytes, HomeBytes: limits.HomeBytes,
			InputBytes: limits.InputBytes, WorkspaceBytes: limits.WorkspaceBytes,
			StdoutBytes: int64(limits.StdoutBytes), StderrBytes: int64(limits.StderrBytes),
		},
		RuntimeTrustRoot: "/usr", RuntimeManifestDigest: runtimeDigest,
		WorkspaceAccess: "read_only", Network: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	environment := control.putArtifact(t, protocol.LocalEnvironmentMediaType, environmentBytes)
	stdoutBytes := []byte("ok\n")
	stderrBytes := []byte{}
	stdout := control.putArtifact(t, "application/octet-stream", stdoutBytes)
	stderr := control.putArtifact(t, "application/octet-stream", stderrBytes)
	receiptRecord, err := protocol.EncodeLocalCheckReceipt(protocol.LocalCheckReceipt{
		SchemaVersion: protocol.LocalCheckReceiptSchemaVersion,
		CheckID:       "test", RunID: "check-1", Definition: definition,
		Candidate: protocol.CandidatePoint{
			Repository: candidate.RepositoryID, Commit: candidate.Commit, Tree: candidate.Tree,
		},
		WorkspaceDigest: testEffectDigest("a"),
		Environment:     protocol.Environment{Kind: "local", Ref: environment.Digest},
		WorkspaceAccess: "read_only", WorkingDirectory: ".",
		Argv: []string{"/usr/bin/true"}, TimeoutSeconds: 30, Network: "none",
		StartedAt: "2026-07-20T00:02:00Z", CompletedAt: "2026-07-20T00:03:00Z",
		ExitCode: 0, Outcome: "pass",
		Stdout: protocol.CapturedArtifact{
			Ref: stdout.Ref, MediaType: stdout.MediaType, Digest: stdout.Digest, Size: int64(len(stdoutBytes)),
		},
		Stderr: protocol.CapturedArtifact{
			Ref: stderr.Ref, MediaType: stderr.MediaType, Digest: stderr.Digest, Size: int64(len(stderrBytes)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkReceipt := control.putArtifact(
		t, "application/vnd.sworn.local-check-receipt+json", receiptRecord.CanonicalJSON,
	)
	builderRun := protocol.BuilderRun{
		RunID: "builder-1", Agent: "test-builder",
		StartedAt: "2026-07-20T00:00:02Z", CompletedAt: "2026-07-20T00:01:00Z",
	}
	submissionRecord, err := protocol.BuildSubmission(ctx, repository, control, protocol.SubmissionInput{
		Attempt: 1, CreatedAt: time.Date(2026, 7, 20, 0, 4, 0, 0, time.UTC),
		Plan: plan, WorkID: "work-1", AuthorityReceipt: authority,
		Builder: builderRun, Candidate: candidate,
		MeasuredChecks: []protocol.MeasuredCheck{{
			RunID: "check-1", RuntimeManifestDigest: runtimeDigest, Receipt: checkReceipt,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := protocol.ParseSubmission(submissionRecord.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	control.records[submissionRecord.Digest] = verifierTestRecord{
		kind: submissionRecord.Kind, contents: bytes.Clone(submissionRecord.CanonicalJSON),
	}
	contract, _ := plan.Work("work-1")
	buildRequest, err := protocol.EncodeCanonical(engine.BuildEffectRequest{
		SchemaVersion: engine.BuildEffectRequestSchemaVersion,
		DeliveryRunID: "delivery-run", DeliveryID: plan.DeliveryID(),
		WorkID: "work-1", WorkAttempt: 1, DispatchDigest: contract.Digest(),
		BuilderDispatchDigest: testEffectDigest("b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	buildResult, err := engine.EncodeBuildEffectResult(engine.BuildEffectResult{
		SchemaVersion: engine.BuildEffectResultSchemaVersion,
		Outcome:       engine.BuildOutcomeCandidateReady, Builder: builderRun, Candidate: candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	control.builder = engine.JournalEffect{
		ID: builderRun.RunID, DeliveryRunID: "delivery-run", Kind: engine.EffectBuild,
		Attempt: 1, Request: buildRequest, Result: buildResult,
	}
	dispatchRecord, err := protocol.BuildVerifierDispatch(protocol.VerifierDispatchInput{
		Submission: submission, DispatchID: "verifier-1",
		Workspace: verifierWorkspaceDescription, CreatedAt: "2026-07-20T00:05:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatch := control.putArtifact(t, "application/json", dispatchRecord.CanonicalJSON)

	root := privateBuilderRoot(t)
	reconciler := newVerifierTestReconciler(t)
	runner := &verifierTestRunner{
		reconciler: reconciler, configurationDigest: reconciler.ConfigurationDigest(), limits: limits,
	}
	binaryPath := filepath.Join(t.TempDir(), "codex")
	binaryBytes := []byte("pinned verifier test executable\n")
	if err := os.WriteFile(binaryPath, binaryBytes, 0o500); err != nil {
		t.Fatal(err)
	}
	schemaDigest, err := protocol.VerifierAssessmentOutputSchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	adapter := &verifierTestAdapter{profile: protocol.VerifierProfile{
		SchemaVersion: protocol.VerifierProfileSchemaVersion,
		Agent:         "codex-cli test", BinaryPath: binaryPath, BinaryVersion: "codex-cli test",
		BinaryDigest: protocol.RawDigest(binaryBytes), BinarySize: int64(len(binaryBytes)),
		ExecutableInput: "codex", Provider: "openai",
		Authentication: "codex-cli-chatgpt-file-v1", CredentialHome: "/home/sworn/.codex",
		PermissionProfile: "sworn_verifier", Model: "test-model",
		ToolSchemaDigest: testEffectDigest("c"),
		Argv:             protocol.CanonicalCodexVerifierArgv("test-model"), EnvironmentNames: []string{},
		PromptDigest: protocol.RawDigest([]byte(protocol.NativeCodexVerifierPrompt)), OutputSchemaDigest: schemaDigest,
		TimeoutNanoseconds: time.Minute.Nanoseconds(),
		Network:            string(executor.NetworkHost), WorkspaceAccess: string(executor.WorkspaceReadOnly),
		NestedSandbox: true, CredentialAccess: true,
	}, assessment: verifierTestAssessment(t)}
	worker := VerifierWorker{
		Control: control, Runner: runner, Adapter: adapter, Repository: repository,
		WorkspaceRoot: root, MaterializeLimits: repo.MaterializeLimits{Bytes: 1 << 20, Entries: 100},
	}
	profileDigest, err := worker.ProfileDigest()
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.EncodeVerifierEffectRequest(engine.VerifierEffectRequest{
		SchemaVersion: engine.VerifierEffectRequestSchemaVersion,
		DeliveryRunID: "delivery-run", DeliveryID: plan.DeliveryID(),
		WorkID: "work-1", WorkAttempt: 1,
		PlanDigest:   plan.Record().Digest,
		SubmissionID: submission.View().SubmissionID, SubmissionDigest: submission.Record().Digest,
		Candidate:  submission.View().Candidate,
		DispatchID: "verifier-1", DispatchReceipt: dispatch,
		VerifierProfileDigest: profileDigest, Agent: adapter.profile.Agent, VerificationEpoch: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return verifierFixture{
		worker: worker, control: control, runner: runner, adapter: adapter,
		repository: repository, candidate: candidate,
		effect: engine.JournalEffect{
			ID: "verifier-1", DeliveryRunID: "delivery-run",
			Kind: engine.EffectVerifier, Attempt: 1, Request: request,
		},
	}
}

func TestVerifierWorkerRunsExactFreshReviewAndPersistsMeasuredResult(t *testing.T) {
	fixture := newVerifierFixture(t)
	resultBytes, err := fixture.worker.run(context.Background(), fixture.effect)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.ParseVerifierEffectResult(resultBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != engine.VerifierOutcomeAssessmentReady ||
		result.DispatchID != fixture.effect.ID || fixture.adapter.parseCalls != 1 {
		t.Fatalf("verifier result = %#v, parse calls = %d", result, fixture.adapter.parseCalls)
	}
	if !slices.Equal(fixture.runner.events, []string{"run", "reconcile"}) ||
		len(fixture.runner.invocations) != 1 || len(fixture.runner.reconciled) != 1 ||
		fixture.runner.reconciled[0] != fixture.runner.invocations[0].ID {
		t.Fatalf("verifier execution order = %#v, reconciled = %#v", fixture.runner.events, fixture.runner.reconciled)
	}
	invocation := fixture.runner.invocations[0]
	if invocation.WorkspaceAccess != executor.WorkspaceReadOnly || invocation.Network != executor.NetworkHost ||
		!invocation.NestedSandbox || !invocation.CredentialAccess || invocation.RuntimeDigest != "" {
		t.Fatalf("verifier invocation boundary = %#v", invocation)
	}
	names := make([]string, len(invocation.Inputs))
	for index, input := range invocation.Inputs {
		names[index] = input.Name
	}
	for _, required := range []string{
		"assessment-schema", "codex", "dispatch", "plan", "review-authority",
		"review-check-01", "review-policy", "submission",
	} {
		if !slices.Contains(names, required) {
			t.Fatalf("verifier inputs %q omit %q", names, required)
		}
	}
	if !slices.IsSorted(names) {
		t.Fatalf("verifier inputs are not sorted: %q", names)
	}

	receiptType, receiptBytes, err := fixture.control.Artifact(
		context.Background(), result.ExecutionReceipt.Digest,
	)
	if err != nil || receiptType != protocol.VerifierExecutionReceiptMediaType {
		t.Fatalf("execution receipt artifact = %q, %v", receiptType, err)
	}
	receipt, err := protocol.ParseVerifierExecutionReceipt(receiptBytes)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Unit != "sworn-verifier-test.service" || !receipt.TargetStarted || !receipt.ServiceQuiescent ||
		receipt.AssessmentDigest != result.Assessment.Digest || len(receipt.Inputs) != len(invocation.Inputs) {
		t.Fatalf("execution receipt = %#v", receipt)
	}
	profileDigest, _ := fixture.worker.ProfileDigest()
	if artifact := fixture.control.artifacts[profileDigest]; artifact.mediaType != protocol.VerifierProfileMediaType {
		t.Fatalf("profile artifact = %#v", artifact)
	}
	schemaDigest, _ := protocol.VerifierAssessmentOutputSchemaDigest()
	if artifact := fixture.control.artifacts[schemaDigest]; artifact.mediaType != protocol.VerifierAssessmentSchemaMediaType {
		t.Fatalf("assessment schema artifact = %#v", artifact)
	}
	entries, err := os.ReadDir(fixture.worker.WorkspaceRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("verifier attempt cleanup = %#v, %v", entries, err)
	}
}

func TestVerifierWorkerReconcilesEveryEnteredAttemptAndSuppressesAmbiguousResults(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*verifierFixture)
		want   string
	}{
		{
			name: "runner error",
			mutate: func(fixture *verifierFixture) {
				fixture.runner.runErr = errors.New("transport lost")
			},
			want: "transport lost",
		},
		{
			name: "reconciliation error",
			mutate: func(fixture *verifierFixture) {
				fixture.runner.reconcileErr = errors.New("unit state unknown")
			},
			want: "unit state unknown",
		},
		{
			name: "completion mismatch",
			mutate: func(fixture *verifierFixture) {
				fixture.runner.mutate = func(completion *executor.RawCompletion) {
					completion.WorkspaceDigest = testEffectDigest("f")
				}
			},
			want: "exact invocation",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newVerifierFixture(t)
			test.mutate(&fixture)
			result, err := fixture.worker.run(context.Background(), fixture.effect)
			if err == nil || len(result) != 0 || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ambiguous verifier result = %q, %v", result, err)
			}
			if !slices.Equal(fixture.runner.events, []string{"run", "reconcile"}) ||
				len(fixture.runner.reconciled) != 1 {
				t.Fatalf("ambiguous verifier cleanup = %#v, %#v", fixture.runner.events, fixture.runner.reconciled)
			}
			if fixture.adapter.parseCalls != 0 {
				t.Fatalf("ambiguous completion reached adapter parser %d times", fixture.adapter.parseCalls)
			}
			entries, readErr := os.ReadDir(fixture.worker.WorkspaceRoot)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("ambiguous verifier local cleanup = %#v, %v", entries, readErr)
			}
		})
	}
}

func TestVerifierProfileDigestClosesExecutorRepositoryAndMaterialization(t *testing.T) {
	fixture := newVerifierFixture(t)
	baseline, err := fixture.worker.ProfileDigest()
	if err != nil {
		t.Fatal(err)
	}
	if agent, err := fixture.worker.Agent(); err != nil || agent != fixture.adapter.profile.Agent {
		t.Fatalf("verifier agent = %q, %v", agent, err)
	}
	changedLimits := fixture.worker
	changedLimits.MaterializeLimits.Bytes--
	if digest, err := changedLimits.ProfileDigest(); err != nil || digest == baseline {
		t.Fatalf("materialization profile digest = %q, %v", digest, err)
	}
	fixture.runner.configurationDigest = testEffectDigest("e")
	if digest, err := fixture.worker.ProfileDigest(); err != nil || digest == baseline {
		t.Fatalf("executor profile digest = %q, %v", digest, err)
	}
}

func TestVerifierWorkerRequiresPreparedStoreCapabilityBeforeExecution(t *testing.T) {
	fixture := newVerifierFixture(t)
	result, err := fixture.worker.Run(context.Background(), store.PreparedAuthorizedVerifierLease{})
	if err == nil || len(result) != 0 {
		t.Fatalf("zero verifier capability result = %q, %v", result, err)
	}
	if len(fixture.runner.events) != 0 || fixture.adapter.parseCalls != 0 {
		t.Fatalf("zero verifier capability reached execution: %#v", fixture.runner.events)
	}
}

func verifierTestAssessment(t *testing.T) []byte {
	t.Helper()
	contents, err := protocol.EncodeCanonical(protocol.VerifierAssessment{
		SchemaVersion: protocol.VerifierAssessmentSchemaVersion,
		Outcome:       "PASS", Summary: "The exact candidate satisfies its contract.",
		AcceptanceResults: []protocol.AcceptanceResult{{
			AcceptanceID: "AC1", Outcome: "pass", EvidenceIDs: []string{"evidence-1"}, Summary: "Proven.",
		}},
		AssuranceResults: []protocol.AssuranceResult{},
		Findings:         []protocol.Finding{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.ParseVerifierAssessment(contents); err != nil {
		t.Fatal(err)
	}
	return contents
}

func newVerifierTestReconciler(t *testing.T) *executor.LinuxExecutor {
	t.Helper()
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	systemctl := filepath.Join(t.TempDir(), "systemctl")
	if err := os.WriteFile(systemctl, []byte("#!/bin/sh\nprintf 'inactive\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contained, err := executor.NewLinux(executor.Options{
		RuntimeRoot:    runtimeRoot,
		BubblewrapPath: "/usr/bin/true", SystemdRunPath: "/usr/bin/true", SystemctlPath: systemctl,
		Limits: executor.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return contained
}

func cloneVerifierInvocation(invocation executor.Invocation) executor.Invocation {
	invocation.Argv = slices.Clone(invocation.Argv)
	invocation.Inputs = slices.Clone(invocation.Inputs)
	invocation.Environment = cloneEnvironment(invocation.Environment)
	return invocation
}
