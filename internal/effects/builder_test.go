package effects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/workspace"
)

var builderCompletionTime = time.Date(2026, 7, 20, 6, 30, 0, 0, time.UTC)

type builderTestControl struct {
	state engine.State
	plan  protocol.ExactPlan
}

func (control *builderTestControl) State(_ context.Context, runID string) (engine.State, error) {
	if runID != control.state.RunID {
		return engine.State{}, errors.New("unknown test run")
	}
	return control.state, nil
}

func (control *builderTestControl) Plan(_ context.Context, digest string) (protocol.ExactPlan, error) {
	if digest != control.plan.Record().Digest {
		return protocol.ExactPlan{}, errors.New("unknown test plan")
	}
	return control.plan, nil
}

type fakeBuilderRunner struct {
	configurationDigest string
	limits              executor.Limits
	exportRoot          string
	mutate              func(string) error
	invocations         []executor.Invocation
	planInput           []byte
	dispatchInput       []byte
	discarded           int
	reconciled          []string
	reconcileErr        error
	reconcileExecutor   *executor.LinuxExecutor
}

func (runner *fakeBuilderRunner) ConfigurationDigest() string      { return runner.configurationDigest }
func (runner *fakeBuilderRunner) EffectiveLimits() executor.Limits { return runner.limits }

func (runner *fakeBuilderRunner) RunWritable(
	ctx context.Context,
	invocation executor.Invocation,
) (executor.RawCompletion, error) {
	runner.invocations = append(runner.invocations, cloneBuilderInvocation(invocation))
	for _, input := range invocation.Inputs {
		contents, err := os.ReadFile(input.Path)
		if err != nil {
			return executor.RawCompletion{}, err
		}
		switch input.Name {
		case "plan":
			runner.planInput = bytes.Clone(contents)
		case "dispatch":
			runner.dispatchInput = bytes.Clone(contents)
		}
	}
	exportPath := filepath.Join(runner.exportRoot, invocation.ID)
	if err := os.Mkdir(exportPath, 0o700); err != nil {
		return executor.RawCompletion{}, err
	}
	if _, _, err := workspace.StageInto(ctx, invocation.Workspace, exportPath, runner.limits.InputBytes); err != nil {
		return executor.RawCompletion{}, err
	}
	if runner.mutate != nil {
		if err := runner.mutate(exportPath); err != nil {
			return executor.RawCompletion{}, err
		}
	}
	digest, size, err := workspace.Measure(ctx, exportPath, runner.limits.WorkspaceBytes)
	if err != nil {
		return executor.RawCompletion{}, err
	}
	bound := make([]executor.BoundInput, len(invocation.Inputs))
	for index, input := range invocation.Inputs {
		info, err := os.Stat(input.Path)
		if err != nil {
			return executor.RawCompletion{}, err
		}
		bound[index] = executor.BoundInput{Name: input.Name, Digest: input.Digest, Size: uint64(info.Size())}
	}
	return executor.RawCompletion{
		InvocationID: invocation.ID, WorkspaceDigest: invocation.WorkspaceDigest,
		WorkspaceAccess: executor.WorkspaceWritableExport, Inputs: bound,
		StartedAt: builderCompletionTime.Add(-time.Minute), CompletedAt: builderCompletionTime,
		ExitCode: 0,
		Export: &executor.WorkspaceExport{
			SchemaVersion: executor.WorkspaceExportSchemaVersion,
			InvocationID:  invocation.ID, Generation: strings.Repeat("a", 32),
			BaseDigest: invocation.WorkspaceDigest, Path: exportPath, Digest: digest, Bytes: size,
		},
	}, nil
}

func (runner *fakeBuilderRunner) ValidateExport(ctx context.Context, export executor.WorkspaceExport) error {
	digest, size, err := workspace.Measure(ctx, export.Path, runner.limits.WorkspaceBytes)
	if err != nil {
		return err
	}
	if digest != export.Digest || size != export.Bytes {
		return errors.New("fake export changed")
	}
	return nil
}

func (runner *fakeBuilderRunner) DiscardExport(_ context.Context, export executor.WorkspaceExport) error {
	runner.discarded++
	return os.RemoveAll(export.Path)
}

func (runner *fakeBuilderRunner) ReconcileWritable(
	ctx context.Context,
	invocationID string,
) (executor.WritableCleanup, error) {
	runner.reconciled = append(runner.reconciled, invocationID)
	if runner.reconcileErr != nil {
		return executor.WritableCleanup{}, runner.reconcileErr
	}
	if runner.reconcileExecutor == nil {
		return executor.WritableCleanup{}, errors.New("fake writable reconciler is not configured")
	}
	return runner.reconcileExecutor.ReconcileWritable(ctx, invocationID)
}

type builderFixture struct {
	worker     BuilderWorker
	runner     *fakeBuilderRunner
	control    *builderTestControl
	repository *repo.Repository
	source     string
	effect     engine.JournalEffect
}

func newBuilderFixture(t *testing.T, mutate func(string) error) builderFixture {
	t.Helper()
	ctx := context.Background()
	plan := exampleBuilderPlan(t)
	source := newBuilderRepository(t)
	binding, err := repo.Discover(ctx, source, plan.Target().Repository)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repo.Open(ctx, source, binding)
	if err != nil {
		t.Fatal(err)
	}
	control := &builderTestControl{plan: plan}
	control.state = engine.State{
		SchemaVersion: engine.StateSchemaVersion,
		RunID:         "delivery-run", DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref,
		Revision: 2, Phase: engine.PhaseActive,
		AuthorityReceiptDigest: protocol.RawDigest([]byte("builder-test-authority")),
		Work: []engine.Work{{
			ID: plan.WorkIDs()[0], State: engine.WorkActive, Attempt: 1, NextAction: engine.ActionWait,
		}},
	}
	if err := control.state.Validate(); err != nil {
		t.Fatal(err)
	}
	runner := &fakeBuilderRunner{
		configurationDigest: protocol.RawDigest([]byte("fake-builder-runner-v1")),
		limits:              executor.DefaultLimits(), exportRoot: t.TempDir(), mutate: mutate,
	}
	worker := BuilderWorker{
		Control: control, Runner: runner, Repository: repository,
		WorkspaceRoot: privateBuilderRoot(t), Agent: "fake-native-cli@1",
		Argv:        []string{"/usr/bin/true", "--fixed"},
		Environment: map[string]string{"API_TOKEN": "super-secret-value"},
		Timeout:     time.Minute,
	}
	dispatchDigest, err := worker.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	request, err := protocol.EncodeCanonical(engine.BuildEffectRequest{
		SchemaVersion: engine.BuildEffectRequestSchemaVersion,
		DeliveryRunID: control.state.RunID, DeliveryID: control.state.DeliveryID,
		WorkID: control.state.Work[0].ID, WorkAttempt: control.state.Work[0].Attempt,
		DispatchDigest: dispatchDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	return builderFixture{
		worker: worker, runner: runner, control: control, repository: repository, source: source,
		effect: engine.JournalEffect{
			ID: "effect-build", DeliveryRunID: control.state.RunID,
			Kind: engine.EffectBuild, Attempt: 1, Request: request,
		},
	}
}

func TestBuilderDispatchDigestBindsEnvironmentNamesButNeverValues(t *testing.T) {
	fixture := newBuilderFixture(t, nil)
	first, err := fixture.worker.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	rotated := fixture.worker
	rotated.Environment = map[string]string{"API_TOKEN": "rotated-secret-value"}
	second, err := rotated.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("secret rotation changed dispatch digest: %s != %s", first, second)
	}
	renamed := fixture.worker
	renamed.Environment = map[string]string{"DIFFERENT_TOKEN": "super-secret-value"}
	third, err := renamed.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("environment name change did not change dispatch digest")
	}

	result, err := fixture.worker.Run(context.Background(), fixture.effect)
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{
		"effect request": fixture.effect.Request,
		"builder result": result,
		"plan input":     fixture.runner.planInput,
		"dispatch input": fixture.runner.dispatchInput,
	} {
		if bytes.Contains(contents, []byte("super-secret-value")) {
			t.Fatalf("%s persisted an environment value", name)
		}
	}
	if got := fixture.runner.invocations[0].Environment["API_TOKEN"]; got != "super-secret-value" {
		t.Fatalf("in-memory invocation environment = %q", got)
	}
}

func TestBuilderRunUsesExactInputsAndPublishesOnlyAfterBindingBoundary(t *testing.T) {
	fixture := newBuilderFixture(t, func(root string) error {
		return os.WriteFile(filepath.Join(root, "src", "generated.go"), []byte("package generated\n"), 0o644)
	})
	ctx := context.Background()
	result, err := fixture.worker.Run(ctx, fixture.effect)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := engine.ParseBuildEffectResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Builder.RunID != fixture.effect.ID || parsed.Builder.Agent != fixture.worker.Agent ||
		parsed.Builder.CompletedAt != builderCompletionTime.Format(time.RFC3339Nano) {
		t.Fatalf("engine-stamped builder = %#v", parsed.Builder)
	}
	if !reflect.DeepEqual(parsed.Candidate.ChangedPaths, []string{"src/generated.go"}) {
		t.Fatalf("candidate changed paths = %#v", parsed.Candidate.ChangedPaths)
	}
	if refs := builderGit(t, fixture.source, "for-each-ref", "--format=%(refname)", "refs/sworn/v1"); strings.TrimSpace(refs) != "" {
		t.Fatalf("Run published Git refs before binding: %s", refs)
	}
	if _, err := fixture.repository.ProveAttemptUnpublished(ctx, fixture.runner.invocations[0].ID); err != nil {
		t.Fatalf("prepared candidate was not unpublished: %v", err)
	}
	if !bytes.Equal(fixture.runner.planInput, fixture.control.plan.Record().CanonicalJSON) {
		t.Fatal("runner did not receive the exact canonical plan")
	}
	for name, contents := range map[string][]byte{"plan": fixture.runner.planInput, "dispatch": fixture.runner.dispatchInput} {
		canonical, err := protocol.CanonicalizeJSON(contents)
		if err != nil || !bytes.Equal(canonical, contents) {
			t.Fatalf("%s input is not canonical: %v", name, err)
		}
	}
	var dispatch builderDispatch
	if err := json.Unmarshal(fixture.runner.dispatchInput, &dispatch); err != nil {
		t.Fatal(err)
	}
	contract, _ := fixture.control.plan.Work(fixture.control.state.Work[0].ID)
	identity, _ := engine.BuildAttemptIdentityFor(fixture.effect.ID, fixture.effect.Attempt, dispatch.DispatchDigest)
	if dispatch.InvocationID != identity.InvocationID || dispatch.ContractDigest != contract.Digest() ||
		dispatch.BaseCommit != parsed.Candidate.BaseCommit || dispatch.BaseTree != parsed.Candidate.BaseTree {
		t.Fatalf("compact dispatch = %#v", dispatch)
	}
	invocation := fixture.runner.invocations[0]
	if invocation.Network != executor.NetworkNone || invocation.WorkspaceAccess != executor.WorkspaceWritableExport ||
		invocation.ID != identity.InvocationID || !slices.Equal(invocation.Argv, fixture.worker.Argv) {
		t.Fatalf("contained invocation = %#v", invocation)
	}
	if _, err := os.Lstat(filepath.Join(fixture.worker.WorkspaceRoot, identity.InvocationID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attempt root remained after Run: %v", err)
	}
	if fixture.runner.discarded != 1 {
		t.Fatalf("discard count = %d, want 1", fixture.runner.discarded)
	}
	if err := fixture.worker.Publish(ctx, fixture.effect, result); err == nil ||
		!strings.Contains(err.Error(), "externally bound") {
		t.Fatalf("unbound Publish error = %v", err)
	}
	if refs := builderGit(t, fixture.source, "for-each-ref", "--format=%(refname)", "refs/sworn/v1"); strings.TrimSpace(refs) != "" {
		t.Fatalf("unbound Publish created Git refs: %s", refs)
	}

	bound := fixture.effect
	bound.Result = result
	if err := fixture.worker.Publish(ctx, bound, result); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.ProveAttemptUnpublished(ctx, identity.InvocationID); err == nil {
		t.Fatal("Publish did not establish the attempt publication point")
	}
	for _, ref := range []string{parsed.Candidate.Ref, "refs/sworn/v1/attempts/" + identity.InvocationID} {
		if got := strings.TrimSpace(builderGit(t, fixture.source, "rev-parse", ref)); got != parsed.Candidate.Commit {
			t.Fatalf("published %s = %s, want %s", ref, got, parsed.Candidate.Commit)
		}
	}
}

func TestBuilderRunFailsClosedOnConfigurationDriftAndOutOfScopeEdit(t *testing.T) {
	t.Run("configuration drift", func(t *testing.T) {
		fixture := newBuilderFixture(t, nil)
		fixture.worker.Argv = []string{"/usr/bin/false"}
		if _, err := fixture.worker.Run(context.Background(), fixture.effect); err == nil ||
			!strings.Contains(err.Error(), "configured dispatch") {
			t.Fatalf("Run configuration drift error = %v", err)
		}
		if len(fixture.runner.invocations) != 0 {
			t.Fatal("configuration drift reached the runner")
		}
	})

	t.Run("out of scope", func(t *testing.T) {
		fixture := newBuilderFixture(t, func(root string) error {
			return os.WriteFile(filepath.Join(root, "README.md"), []byte("outside scope\n"), 0o644)
		})
		identity, _ := engine.BuildAttemptIdentityFor(
			fixture.effect.ID, fixture.effect.Attempt, mustBuilderDigest(t, fixture.worker),
		)
		if _, err := fixture.worker.Run(context.Background(), fixture.effect); err == nil ||
			!strings.Contains(err.Error(), "outside approved scope") {
			t.Fatalf("Run out-of-scope error = %v", err)
		}
		if refs := strings.TrimSpace(builderGit(t, fixture.source, "for-each-ref", "--format=%(refname)", "refs/sworn/v1")); refs != "" {
			t.Fatalf("out-of-scope run published refs: %s", refs)
		}
		if _, err := os.Lstat(filepath.Join(fixture.worker.WorkspaceRoot, identity.InvocationID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("out-of-scope attempt root remained: %v", err)
		}
		if fixture.runner.discarded != 1 {
			t.Fatalf("discard count = %d, want 1", fixture.runner.discarded)
		}
	})
}

func TestReconcileUnboundMintsOpaqueProofOnlyAfterAllCleanup(t *testing.T) {
	fixture := newBuilderFixture(t, nil)
	fixture.runner.reconcileExecutor = newBuilderReconcileExecutor(t)
	fixture.runner.configurationDigest = fixture.runner.reconcileExecutor.ConfigurationDigest()
	fixture.effect = builderEffectFor(t, fixture.worker, fixture.control.state)
	ctx := context.Background()
	digest := mustBuilderDigest(t, fixture.worker)
	identity, _ := engine.BuildAttemptIdentityFor(fixture.effect.ID, fixture.effect.Attempt, digest)
	attemptRoot, _, err := createBuildAttemptRoot(fixture.worker.WorkspaceRoot, identity.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attemptRoot, "residue"), []byte("residue"), 0o600); err != nil {
		t.Fatal(err)
	}
	proof, err := fixture.worker.ReconcileUnbound(ctx, fixture.effect)
	if err != nil {
		t.Fatal(err)
	}
	if proof.EffectID() != fixture.effect.ID || proof.EffectAttempt() != fixture.effect.Attempt ||
		proof.InvocationID() != identity.InvocationID || proof.DispatchDigest() != digest ||
		proof.RepositoryID() != fixture.control.state.Repository || proof.TargetRef() != fixture.control.state.TargetRef {
		t.Fatalf("retry proof = %#v", proof)
	}
	if proof.WritableCleanup().InvocationID() != identity.InvocationID ||
		proof.Unpublished().RepositoryID() != fixture.control.state.Repository ||
		proof.Unpublished().AttemptID() != identity.InvocationID {
		t.Fatal("retry proof did not retain its lower-level opaque proofs")
	}
	if (BuildRetryProof{}).EffectID() != "" || (BuildRetryProof{}).EffectAttempt() != 0 ||
		(BuildRetryProof{}).InvocationID() != "" || (BuildRetryProof{}).DispatchDigest() != "" ||
		(BuildRetryProof{}).RepositoryID() != "" || (BuildRetryProof{}).TargetRef() != "" ||
		(BuildRetryProof{}).WritableCleanup().InvocationID() != "" ||
		(BuildRetryProof{}).Unpublished().RepositoryID() != "" ||
		(BuildRetryProof{}).Unpublished().AttemptID() != "" {
		t.Fatal("zero retry proof exposed non-zero facts")
	}
	if !reflect.DeepEqual(fixture.runner.reconciled, []string{identity.InvocationID}) {
		t.Fatalf("reconciled invocations = %#v", fixture.runner.reconciled)
	}
	if _, err := os.Lstat(attemptRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reconciled local attempt root remains: %v", err)
	}

	// Publication ambiguity must stop before either executor or local cleanup.
	fixture = newBuilderFixture(t, func(root string) error {
		return os.WriteFile(filepath.Join(root, "src", "published.go"), []byte("package published\n"), 0o644)
	})
	result, err := fixture.worker.Run(ctx, fixture.effect)
	if err != nil {
		t.Fatal(err)
	}
	bound := fixture.effect
	bound.Result = result
	if err := fixture.worker.Publish(ctx, bound, result); err != nil {
		t.Fatal(err)
	}
	identity, _ = engine.BuildAttemptIdentityFor(
		fixture.effect.ID, fixture.effect.Attempt, mustBuilderDigest(t, fixture.worker),
	)
	attemptRoot, _, err = createBuildAttemptRoot(fixture.worker.WorkspaceRoot, identity.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	reconciledBefore := len(fixture.runner.reconciled)
	if proof, err := fixture.worker.ReconcileUnbound(ctx, fixture.effect); err == nil || proof != (BuildRetryProof{}) {
		t.Fatalf("published reconciliation = %#v, %v", proof, err)
	}
	if len(fixture.runner.reconciled) != reconciledBefore {
		t.Fatal("publication ambiguity reached executor cleanup")
	}
	if _, err := os.Lstat(attemptRoot); err != nil {
		t.Fatalf("publication ambiguity removed local residue: %v", err)
	}
}

func builderEffectFor(t *testing.T, worker BuilderWorker, state engine.State) engine.JournalEffect {
	t.Helper()
	dispatchDigest := mustBuilderDigest(t, worker)
	request, err := protocol.EncodeCanonical(engine.BuildEffectRequest{
		SchemaVersion: engine.BuildEffectRequestSchemaVersion,
		DeliveryRunID: state.RunID, DeliveryID: state.DeliveryID,
		WorkID: state.Work[0].ID, WorkAttempt: state.Work[0].Attempt,
		DispatchDigest: dispatchDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine.JournalEffect{
		ID: "effect-build", DeliveryRunID: state.RunID,
		Kind: engine.EffectBuild, Attempt: 1, Request: request,
	}
}

func newBuilderReconcileExecutor(t *testing.T) *executor.LinuxExecutor {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("writable cleanup proof requires Linux")
	}
	writableRoot, err := os.MkdirTemp("/dev/shm", "sworn-builder-effects-")
	if err != nil {
		t.Skipf("create tmpfs writable root: %v", err)
	}
	if err := os.Chmod(writableRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(writableRoot) })
	systemctl := filepath.Join(t.TempDir(), "systemctl")
	if err := os.WriteFile(systemctl, []byte("#!/bin/sh\nprintf 'inactive\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contained, err := executor.NewLinux(executor.Options{
		RuntimeRoot: t.TempDir(), WritableRoot: writableRoot,
		BubblewrapPath: "/usr/bin/true", SystemdRunPath: "/usr/bin/true", SystemctlPath: systemctl,
		Limits: executor.DefaultLimits(),
	})
	if err != nil {
		t.Skipf("construct Linux cleanup boundary: %v", err)
	}
	return contained
}

func cloneBuilderInvocation(invocation executor.Invocation) executor.Invocation {
	invocation.Argv = slices.Clone(invocation.Argv)
	invocation.Inputs = slices.Clone(invocation.Inputs)
	invocation.Environment = cloneEnvironment(invocation.Environment)
	return invocation
}

func exampleBuilderPlan(t *testing.T) protocol.ExactPlan {
	t.Helper()
	snapshot, err := protocol.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/standard-plan.json")
	if err != nil {
		t.Fatal(err)
	}
	contents = bytes.ReplaceAll(contents, []byte("local:example"), []byte("repo-01"))
	plan, err := protocol.ParseDeliveryPlan(contents)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func newBuilderRepository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	builderGit(t, root, "init", "-b", "main")
	builderGit(t, root, "config", "user.name", "Sworn Test")
	builderGit(t, root, "config", "user.email", "sworn@example.invalid")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "existing.go"), []byte("package existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("base readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderGit(t, root, "add", ".")
	builderGit(t, root, "commit", "-m", "base")
	return root
}

func builderGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func privateBuilderRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "attempts")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func mustBuilderDigest(t *testing.T, worker BuilderWorker) string {
	t.Helper()
	digest, err := worker.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
