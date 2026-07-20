package control_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	controlpkg "github.com/swornagent/sworn/internal/control"
	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
	"github.com/swornagent/sworn/internal/workspace"
)

const integrationLocalCheckReceiptMediaType = "application/vnd.sworn.local-check-receipt+json"

type integrationBuilderControl struct {
	journal *store.Store
}

func (control *integrationBuilderControl) State(ctx context.Context, runID string) (engine.State, error) {
	if control.journal == nil {
		return engine.State{}, errors.New("integration control is not bound")
	}
	return control.journal.State(ctx, runID)
}

func (control *integrationBuilderControl) Plan(ctx context.Context, digest string) (protocol.ExactPlan, error) {
	if control.journal == nil {
		return protocol.ExactPlan{}, errors.New("integration control is not bound")
	}
	return control.journal.Plan(ctx, digest)
}

type integrationBuilderRunner struct {
	configurationDigest string
	limits              executor.Limits
	exportRoot          string
}

func (runner *integrationBuilderRunner) ConfigurationDigest() string {
	return runner.configurationDigest
}

func (runner *integrationBuilderRunner) EffectiveLimits() executor.Limits {
	return runner.limits
}

func (runner *integrationBuilderRunner) RunWritable(
	ctx context.Context,
	invocation executor.Invocation,
) (executor.RawCompletion, error) {
	started := time.Now().UTC()
	exportPath := filepath.Join(runner.exportRoot, invocation.ID)
	if err := os.Mkdir(exportPath, 0o700); err != nil {
		return executor.RawCompletion{}, err
	}
	if _, _, err := workspace.StageInto(ctx, invocation.Workspace, exportPath, runner.limits.InputBytes); err != nil {
		return executor.RawCompletion{}, err
	}
	changed := []byte("package main\n\nfunc ready() bool { return true }\n")
	if err := os.WriteFile(filepath.Join(exportPath, "src", "main.go"), changed, 0o644); err != nil {
		return executor.RawCompletion{}, err
	}
	digest, size, err := workspace.Measure(ctx, exportPath, runner.limits.WorkspaceBytes)
	if err != nil {
		return executor.RawCompletion{}, err
	}
	inputs := make([]executor.BoundInput, len(invocation.Inputs))
	for index, input := range invocation.Inputs {
		info, err := os.Stat(input.Path)
		if err != nil {
			return executor.RawCompletion{}, err
		}
		inputs[index] = executor.BoundInput{
			Name: input.Name, Digest: input.Digest, Size: uint64(info.Size()),
		}
	}
	return executor.RawCompletion{
		InvocationID: invocation.ID, WorkspaceDigest: invocation.WorkspaceDigest,
		WorkspaceAccess: invocation.WorkspaceAccess, Inputs: inputs,
		StartedAt: started, CompletedAt: time.Now().UTC(), ExitCode: 0,
		Export: &executor.WorkspaceExport{
			SchemaVersion: executor.WorkspaceExportSchemaVersion,
			InvocationID:  invocation.ID, Generation: strings.Repeat("a", 32),
			BaseDigest: invocation.WorkspaceDigest, Path: exportPath, Digest: digest, Bytes: size,
		},
	}, nil
}

func (runner *integrationBuilderRunner) ValidateExport(
	ctx context.Context,
	export executor.WorkspaceExport,
) error {
	digest, size, err := workspace.Measure(ctx, export.Path, runner.limits.WorkspaceBytes)
	if err != nil {
		return err
	}
	if digest != export.Digest || size != export.Bytes {
		return errors.New("integration builder export changed")
	}
	return nil
}

func (*integrationBuilderRunner) DiscardExport(_ context.Context, export executor.WorkspaceExport) error {
	return os.RemoveAll(export.Path)
}

func (*integrationBuilderRunner) ReconcileWritable(
	context.Context,
	string,
) (executor.WritableCleanup, error) {
	return executor.WritableCleanup{}, errors.New("unexpected integration builder reconciliation")
}

type integrationAuthorityResolver struct {
	sourceRef  string
	planDigest string
	source     []byte
	proof      []byte
}

func (resolver integrationAuthorityResolver) Resolve(
	_ context.Context,
	sourceRef string,
	planDigest string,
) ([]byte, []byte, error) {
	if sourceRef != resolver.sourceRef || planDigest != resolver.planDigest {
		return nil, nil, fmt.Errorf("unexpected authority resolution for %q at %q", sourceRef, planDigest)
	}
	return bytes.Clone(resolver.source), bytes.Clone(resolver.proof), nil
}

type integrationAuthoritySource struct {
	Version       int64             `json:"version"`
	SourceID      string            `json:"source_id"`
	Status        string            `json:"status"`
	Repository    string            `json:"repository"`
	TargetRef     string            `json:"target_ref"`
	MaximumGrants []json.RawMessage `json:"maximum_grants"`
	AuthorizerRef string            `json:"authorizer_ref"`
	ValidFrom     string            `json:"valid_from"`
	ValidUntil    string            `json:"valid_until"`
}

type integrationAuthorityProof struct {
	SchemaVersion   string `json:"schema_version"`
	SourceRef       string `json:"source_ref"`
	SourceDigest    string `json:"source_digest"`
	SourceVersion   int64  `json:"source_version"`
	PlanDigest      string `json:"plan_digest"`
	AuthorityDigest string `json:"authority_digest"`
	KeyID           string `json:"key_id"`
	ApprovedAt      string `json:"approved_at"`
	Signature       string `json:"signature"`
}

type integrationUnsignedAuthorityProof struct {
	SchemaVersion   string `json:"schema_version"`
	SourceRef       string `json:"source_ref"`
	SourceDigest    string `json:"source_digest"`
	SourceVersion   int64  `json:"source_version"`
	PlanDigest      string `json:"plan_digest"`
	AuthorityDigest string `json:"authority_digest"`
	KeyID           string `json:"key_id"`
	ApprovedAt      string `json:"approved_at"`
}

func TestNativeBuilderServiceFeedsChecksAndAdmission(t *testing.T) {
	ctx := context.Background()
	repository := newIntegrationRepository(t)
	workspaceRoot := t.TempDir()
	if err := os.Chmod(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	boundary := &integrationBuilderControl{}
	runner := &integrationBuilderRunner{
		configurationDigest: protocol.RawDigest([]byte("integration-builder-runner-v1")),
		limits:              executor.DefaultLimits(),
		exportRoot:          t.TempDir(),
	}
	worker := effects.BuilderWorker{
		Control: boundary, Runner: runner, Repository: repository,
		WorkspaceRoot: workspaceRoot, Agent: "integration-builder@1",
		Argv: []string{"/usr/bin/integration-builder"}, Timeout: time.Minute,
	}
	builderDispatchDigest, err := worker.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	runtimeManifestDigest := protocol.RawDigest([]byte("integration-local-runtime-v1"))
	journal, err := store.OpenConfigured(ctx, filepath.Join(t.TempDir(), "control.db"), store.ControlConfiguration{
		LocalCheckRuntimeManifestDigest: runtimeManifestDigest,
		BuilderDispatchDigest:           builderDispatchDigest,
		Repository:                      repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	boundary.journal = journal

	clock := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Second)
	plan := newIntegrationPlan(t, journal, clock)
	approval := approveIntegrationPlan(t, journal, plan, clock.Add(time.Minute))
	workID := plan.WorkIDs()[0]
	contract, exists := plan.Work(workID)
	if !exists {
		t.Fatal("integration plan lacks its work contract")
	}

	applyIntegrationCommand(t, journal, integrationCommand(t, "cmd-create", "run-native", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref, Work: plan.WorkIDs(),
	}))
	applyIntegrationCommand(t, journal, integrationCommand(t, "cmd-activate", "run-native", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: approval.Facts().ReceiptDigest,
	}))
	buildDispatch := applyIntegrationCommand(t, journal, integrationCommand(
		t, "cmd-build", "run-native", engine.CommandDispatchBuild, 1, engine.DispatchBuildPayload{
			WorkID: workID, DispatchDigest: contract.Digest(),
			BuilderDispatchDigest: builderDispatchDigest,
		},
	))
	if len(buildDispatch.EffectIDs) != 1 {
		t.Fatalf("build effect IDs = %v", buildDispatch.EffectIDs)
	}
	buildLease, err := journal.ClaimNextEffect(ctx, "builder-worker")
	if err != nil || buildLease.Invocation().ID != buildDispatch.EffectIDs[0] {
		t.Fatalf("claim native builder = %+v, %v", buildLease.Invocation(), err)
	}
	builderService, err := controlpkg.NewBuilderService(journal, worker)
	if err != nil {
		t.Fatal(err)
	}
	if err := builderService.Execute(ctx, buildLease); err != nil {
		t.Fatal(err)
	}

	buildFact, err := journal.SucceededEffect(ctx, buildDispatch.EffectIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	build, err := engine.ParseBuildEffectResult(buildFact.Result)
	if err != nil {
		t.Fatal(err)
	}
	if build.Candidate.Commit == build.Candidate.BaseCommit ||
		len(build.Candidate.ChangedPaths) != 1 || build.Candidate.ChangedPaths[0] != "src/main.go" {
		t.Fatalf("native builder candidate = %+v", build.Candidate)
	}
	identity, err := engine.BuildAttemptIdentityFor(
		buildFact.ID, buildFact.Attempt, builderDispatchDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrationRef(t, repository.Root(), build.Candidate.Ref, build.Candidate.Commit)
	assertIntegrationRef(
		t, repository.Root(), "refs/sworn/v1/attempts/"+identity.InvocationID, build.Candidate.Commit,
	)

	selection, err := protocol.ResolveExactLocalChecks(ctx, journal, plan, workID)
	if err != nil {
		t.Fatal(err)
	}
	requirements := selection.Requirements()
	if len(requirements) != 1 {
		t.Fatalf("exact checks = %+v", requirements)
	}
	checkDispatch := applyIntegrationCommand(t, journal, integrationCommand(
		t, "cmd-checks", "run-native", engine.CommandDispatchChecks, 2, engine.DispatchChecksPayload{
			WorkID: workID, BuilderEffectID: buildFact.ID,
			RuntimeManifestDigest: runtimeManifestDigest,
			Checks: []engine.CheckSelection{{
				CheckID: requirements[0].CheckID, DefinitionDigest: requirements[0].Definition.Digest,
			}},
		},
	))
	if len(checkDispatch.EffectIDs) != 1 {
		t.Fatalf("check effect IDs = %v", checkDispatch.EffectIDs)
	}
	completeIntegrationCheck(t, journal, checkDispatch.EffectIDs[0], build.Candidate)
	applyIntegrationCommand(t, journal, integrationCommand(
		t, "cmd-admit", "run-native", engine.CommandAdmitSubmission, 3,
		engine.AdmitSubmissionPayload{WorkID: workID},
	))

	state, err := journal.State(ctx, "run-native")
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 4 || len(state.Work) != 1 || state.Work[0].State != engine.WorkReviewable ||
		state.Work[0].CandidateCommit != build.Candidate.Commit {
		t.Fatalf("admitted state = %+v", state)
	}
	kind, encodedSubmission, err := journal.Record(ctx, state.Work[0].SubmissionDigest)
	if err != nil {
		t.Fatal(err)
	}
	var submission protocol.Submission
	if err := json.Unmarshal(encodedSubmission, &submission); err != nil {
		t.Fatal(err)
	}
	if kind != protocol.SubmissionSchemaVersion || submission.Builder.RunID != buildFact.ID ||
		submission.Candidate.Commit != build.Candidate.Commit || len(submission.Checks) != 1 ||
		submission.Checks[0].ID != requirements[0].CheckID {
		t.Fatalf("native submission = kind %q, %+v", kind, submission)
	}
}

func newIntegrationRepository(t *testing.T) *repo.Repository {
	t.Helper()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runIntegrationGit(t, root, "init", "-b", "main")
	runIntegrationGit(t, root, "config", "user.name", "Integration Builder")
	runIntegrationGit(t, root, "config", "user.email", "builder@example.invalid")
	writeIntegrationFile(t, filepath.Join(root, "src", "main.go"), []byte("package main\n"))
	runIntegrationGit(t, root, "add", "--all")
	runIntegrationGit(t, root, "commit", "-m", "base")
	binding, err := repo.Discover(ctx, root, "repo-01")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repo.Open(ctx, root, binding)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func newIntegrationPlan(
	t *testing.T,
	journal *store.Store,
	createdAt time.Time,
) protocol.ExactPlan {
	t.Helper()
	ctx := context.Background()
	definitionBytes, err := protocol.EncodeCanonical(protocol.LocalCheckDefinition{
		SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion,
		Argv:          []string{"/usr/bin/true"}, WorkingDirectory: ".", TimeoutSeconds: 10,
		Evidence: protocol.LocalEvidenceDefinition{
			ID: "evidence-ready", AcceptanceIDs: []string{"AC1"},
			Boundary: "assembled", Observed: "the assembled candidate reports ready",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	definitionDigest, err := journal.PutArtifact(ctx, "application/json", definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	policyBytes, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": protocol.AssurancePolicySchemaVersion,
		"policy_id":      "integration-standard",
		"checks": []any{map[string]any{
			"id": "ready", "definition": map[string]any{
				"ref": "policy/checks/ready.json", "media_type": "application/json",
				"digest": definitionDigest,
			},
		}},
		"packs": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyDigest, err := journal.PutArtifact(ctx, "application/json", policyBytes)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := protocol.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	planBytes, err := fs.ReadFile(snapshot, "examples/standard-plan.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(planBytes, &document); err != nil {
		t.Fatal(err)
	}
	document["created_at"] = createdAt.Format(time.RFC3339Nano)
	document["assurance_policy"] = map[string]any{
		"ref": "policy/assurance.json", "digest": policyDigest,
	}
	document["target"].(map[string]any)["repository"] = "repo-01"
	for _, raw := range document["authority"].(map[string]any)["grants"].([]any) {
		grant := raw.(map[string]any)
		if grant["action"] == "integrate" {
			grant["target"].(map[string]any)["repository"] = "repo-01"
		}
	}
	canonical, err := protocol.EncodeCanonical(document)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := protocol.ParseDeliveryPlan(canonical)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func approveIntegrationPlan(
	t *testing.T,
	journal *store.Store,
	plan protocol.ExactPlan,
	approvedAt time.Time,
) policy.HistoricalApproval {
	t.Helper()
	seed := sha256.Sum256([]byte("native builder integration authority"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	const authorizerRef = "identity:integration-authorizer"
	root, err := policy.NewTrustRoot(
		plan.Authority().SourceRef, authorizerRef, privateKey.Public().(ed25519.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}
	grants := make([]json.RawMessage, 0, len(plan.Authority().Grants))
	for _, grant := range plan.Authority().Grants {
		grants = append(grants, json.RawMessage(grant.CanonicalJSON()))
	}
	sourceBytes, err := protocol.EncodeCanonical(integrationAuthoritySource{
		Version: 1, SourceID: "integration-source", Status: "active",
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref,
		MaximumGrants: grants, AuthorizerRef: authorizerRef,
		ValidFrom:  approvedAt.Add(-time.Hour).Format(time.RFC3339Nano),
		ValidUntil: approvedAt.Add(24 * time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := integrationAuthorityProof{
		SchemaVersion: policy.AuthorityProofSchemaVersion,
		SourceRef:     plan.Authority().SourceRef, SourceDigest: protocol.CanonicalDigest(sourceBytes),
		SourceVersion: 1, PlanDigest: plan.Record().Digest,
		AuthorityDigest: plan.Authority().Digest, KeyID: root.KeyID(),
		ApprovedAt: approvedAt.Format(time.RFC3339Nano),
	}
	unsigned, err := protocol.EncodeCanonical(integrationUnsignedAuthorityProof{
		SchemaVersion: proof.SchemaVersion, SourceRef: proof.SourceRef,
		SourceDigest: proof.SourceDigest, SourceVersion: proof.SourceVersion,
		PlanDigest: proof.PlanDigest, AuthorityDigest: proof.AuthorityDigest,
		KeyID: proof.KeyID, ApprovedAt: proof.ApprovedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	message := append([]byte("sworn/authority-proof/v1\x00"), unsigned...)
	proof.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, message))
	proofBytes, err := protocol.EncodeCanonical(proof)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := policy.NewAuthority([]policy.TrustRoot{root}, integrationAuthorityResolver{
		sourceRef: plan.Authority().SourceRef, planDigest: plan.Record().Digest,
		source: sourceBytes, proof: proofBytes,
	}, journal)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := authority.Approve(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	return approval
}

func completeIntegrationCheck(
	t *testing.T,
	journal *store.Store,
	effectID string,
	candidate repo.Candidate,
) {
	t.Helper()
	ctx := context.Background()
	lease, err := journal.ClaimNextEffect(ctx, "check-worker")
	if err != nil || lease.Invocation().ID != effectID {
		t.Fatalf("claim exact check = %+v, %v", lease.Invocation(), err)
	}
	request, err := engine.ParseLocalCheckEffectRequest(lease.Invocation().Request)
	if err != nil {
		t.Fatal(err)
	}
	definitionType, definitionBytes, err := journal.Artifact(ctx, request.DefinitionDigest)
	if err != nil || definitionType != "application/json" {
		t.Fatalf("load exact check definition = %q, %v", definitionType, err)
	}
	definition, err := protocol.ParseLocalCheckDefinition(definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	snapshotDigest, err := protocol.SnapshotDigest()
	if err != nil {
		t.Fatal(err)
	}
	environment := protocol.LocalEnvironment{
		SchemaVersion:          protocol.ContentEnvironmentSchemaVersion,
		ProtocolSnapshotDigest: "sha256:" + snapshotDigest,
		EngineRuntime:          "go-integration", OS: "linux", Architecture: "amd64",
		Executor: protocol.LocalExecutorProbe{
			BubblewrapVersion: "bubblewrap integration", SystemdVersion: "systemd integration",
			CgroupV2: true, UserManager: "running", Controllers: []string{"cpu", "memory", "pids"},
		},
		ExecutorPolicyVersion: "sworn-linux-containment-v1",
		Limits: protocol.LocalExecutionLimits{
			RuntimeNanoseconds: 10_000_000_000, MemoryBytes: 64 << 20,
			Tasks: 16, CPUPercent: 100, FileBytes: 1 << 20, TempBytes: 1 << 20,
			HomeBytes: 1 << 20, InputBytes: 1 << 20, WorkspaceBytes: 1 << 20,
			StdoutBytes: 1 << 20, StderrBytes: 1 << 20,
		},
		RuntimeTrustRoot: "/usr", RuntimeManifestDigest: request.RuntimeManifestDigest,
		WorkspaceAccess: "read_only", Network: "none",
	}
	environmentBytes, err := protocol.EncodeCanonical(environment)
	if err != nil {
		t.Fatal(err)
	}
	environmentDigest, err := journal.PutArtifact(ctx, protocol.LocalEnvironmentMediaType, environmentBytes)
	if err != nil {
		t.Fatal(err)
	}
	stdout := putIntegrationCapture(t, journal, []byte("ok\n"))
	stderr := putIntegrationCapture(t, journal, []byte{})
	completedAt := time.Now().UTC()
	receipt, err := protocol.EncodeLocalCheckReceipt(protocol.LocalCheckReceipt{
		SchemaVersion: protocol.LocalCheckReceiptSchemaVersion,
		CheckID:       request.CheckID, RunID: lease.Invocation().ID,
		Definition: protocol.Artifact{
			Ref: request.DefinitionDigest, MediaType: "application/json", Digest: request.DefinitionDigest,
		},
		Candidate: protocol.CandidatePoint{
			Repository: candidate.RepositoryID, Commit: candidate.Commit, Tree: candidate.Tree,
		},
		WorkspaceDigest: protocol.RawDigest([]byte("integration-check-workspace")),
		Environment:     protocol.Environment{Kind: "local", Ref: environmentDigest},
		WorkspaceAccess: "read_only", WorkingDirectory: definition.WorkingDirectory,
		Argv: definition.Argv, TimeoutSeconds: definition.TimeoutSeconds, Network: "none",
		StartedAt: completedAt.Format(time.RFC3339Nano), CompletedAt: completedAt.Format(time.RFC3339Nano),
		Outcome: "pass", Stdout: stdout, Stderr: stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest, err := journal.PutArtifact(
		ctx, integrationLocalCheckReceiptMediaType, receipt.CanonicalJSON,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.EncodeLocalCheckEffectResult(engine.LocalCheckEffectResult{
		SchemaVersion: engine.LocalCheckEffectResultSchemaVersion,
		Outcome:       engine.LocalCheckOutcomePass,
		Receipt: protocol.Artifact{
			Ref: receiptDigest, MediaType: integrationLocalCheckReceiptMediaType, Digest: receiptDigest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if err := journal.CompleteEffect(ctx, lease); err != nil {
		t.Fatal(err)
	}
}

func putIntegrationCapture(
	t *testing.T,
	journal *store.Store,
	contents []byte,
) protocol.CapturedArtifact {
	t.Helper()
	digest, err := journal.PutArtifact(context.Background(), "application/octet-stream", contents)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.CapturedArtifact{
		Ref: digest, MediaType: "application/octet-stream", Digest: digest, Size: int64(len(contents)),
	}
}

func integrationCommand(
	t *testing.T,
	id string,
	runID string,
	kind engine.CommandKind,
	revision int64,
	payload any,
) engine.Command {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return engine.Command{
		ID: id, RunID: runID, Kind: kind, ExpectedRevision: revision, Payload: encoded,
	}
}

func applyIntegrationCommand(
	t *testing.T,
	journal *store.Store,
	command engine.Command,
) store.ApplyResult {
	t.Helper()
	result, err := journal.Apply(context.Background(), command)
	if err != nil || result.Outcome != store.OutcomeApplied {
		t.Fatalf("apply %s = %+v, %v", command.Kind, result, err)
	}
	return result
}

func assertIntegrationRef(t *testing.T, root, ref, want string) {
	t.Helper()
	got := strings.TrimSpace(runIntegrationGit(t, root, "rev-parse", ref))
	if got != want {
		t.Fatalf("Git ref %q = %q, want %q", ref, got, want)
	}
}

func runIntegrationGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func writeIntegrationFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
