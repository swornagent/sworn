package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

func TestBuildClaimWitnessFailureRollsBackAttempt(t *testing.T) {
	fixture := newBuildRetryFixture(t)
	ctx := context.Background()
	if _, err := fixture.control.db.ExecContext(ctx, `
		CREATE TEMP TRIGGER fail_build_claim_witness
		BEFORE INSERT ON effect_observations WHEN NEW.kind = 'claimed' BEGIN
			SELECT RAISE(ABORT, 'injected claimed-observation failure');
		END`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.control.ClaimNextEffect(ctx, "builder-worker"); err == nil ||
		!strings.Contains(err.Error(), "injected claimed-observation failure") {
		t.Fatalf("injected build claim error = %v", err)
	}
	pending, err := listEffects(ctx, fixture.control, EffectPending)
	if err != nil || len(pending) != 1 || pending[0].ID != fixture.effectID ||
		pending[0].Attempt != 0 || pending[0].OwnerID != "" || pending[0].StartedAtUS != 0 {
		t.Fatalf("rolled-back build claim = %+v, %v", pending, err)
	}
	if observations := buildRetryObservationCount(t, fixture.control, fixture.effectID, "claimed"); observations != 0 {
		t.Fatalf("rolled-back build claim retained %d claimed observations", observations)
	}
	if _, err := fixture.control.db.ExecContext(ctx, "DROP TRIGGER fail_build_claim_witness"); err != nil {
		t.Fatal(err)
	}
	lease, err := fixture.control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil || lease.Invocation().Attempt != 1 {
		t.Fatalf("claim after witness persistence restored = %+v, %v", lease.Invocation(), err)
	}
}

func TestNativeBuildExecutionRequiresCurrentStoreLease(t *testing.T) {
	fixture := newBuildRetryFixture(t)
	ctx := context.Background()
	lease, err := fixture.control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := OpenConfigured(ctx, fixture.controlPath, ControlConfiguration{
		LocalCheckRuntimeManifestDigest: "sha256:" + strings.Repeat("e", 64),
		BuilderDispatchDigest:           fixture.builderDispatchDigest,
		Repository:                      fixture.repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	if _, err := peer.PrepareNativeBuildExecution(ctx, lease); err == nil ||
		!strings.Contains(err.Error(), "store-issued lease") {
		t.Fatalf("foreign native build lease preparation error = %v", err)
	}
	invocation, err := fixture.control.PrepareNativeBuildExecution(ctx, lease)
	if err != nil || invocation.ID != lease.Invocation().ID || invocation.Attempt != lease.Invocation().Attempt ||
		len(invocation.Result) != 0 {
		t.Fatalf("prepared native build invocation = %+v, %v", invocation, err)
	}
	result := validBuildResultForCandidate(t, fixture.effectID, "sworn-builder/1", fixture.candidate)
	if err := fixture.control.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.control.PrepareNativeBuildExecution(ctx, lease); err == nil ||
		!strings.Contains(err.Error(), "unbound running build") {
		t.Fatalf("already-bound native build preparation error = %v", err)
	}
}

func TestBuildRetryUsesPrevalidatedLeaseAndCompositeProof(t *testing.T) {
	fixture := newBuildRetryFixture(t)
	ctx := context.Background()

	first, err := fixture.control.ClaimNextEffect(ctx, "builder-worker-1")
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity := durableBuildRetryIdentity(t, fixture.control, first)
	if firstIdentity.EffectAttempt != 1 {
		t.Fatalf("first build identity = %+v", firstIdentity)
	}
	if recovered, err := fixture.control.RecoverInterruptedEffects(ctx, "first builder process stopped"); err != nil || recovered != 1 {
		t.Fatalf("mark first attempt unknown = %d, %v", recovered, err)
	}
	assertBuildRetryUnknown(t, fixture.control, fixture.effectID, 1, nil)

	recovery, err := fixture.control.PrepareUnboundBuildRecovery(ctx, fixture.effectID, 1)
	if err != nil {
		t.Fatalf("prevalidate first build recovery: %v", err)
	}
	if recovery.Invocation().ID != fixture.effectID || recovery.Invocation().Attempt != 1 {
		t.Fatalf("first recovery lease invocation = %+v", recovery.Invocation())
	}
	if err := fixture.control.RecoverUnboundBuildEffect(
		ctx, recovery, "retry-reconciler", effects.BuildRetryProof{},
	); err == nil {
		t.Fatal("zero composite proof requeued an unknown build")
	}
	assertBuildRetryUnknown(t, fixture.control, fixture.effectID, 1, nil)

	proof, err := fixture.worker.ReconcileUnbound(ctx, recovery.Invocation(), recovery.Challenge())
	if err != nil {
		t.Fatalf("mint first build retry proof: %v", err)
	}
	if proof.InvocationID() != firstIdentity.InvocationID ||
		proof.BuilderDispatchDigest() != fixture.builderDispatchDigest {
		t.Fatalf("first build retry proof does not match claim: proof=%s identity=%+v", proof.InvocationID(), firstIdentity)
	}
	if _, err := fixture.control.db.ExecContext(ctx, `
		CREATE TEMP TRIGGER fail_build_requeue
		BEFORE UPDATE ON effects
		WHEN OLD.state = 'unknown' AND NEW.state = 'pending' BEGIN
			SELECT RAISE(ABORT, 'injected requeue failure');
		END`); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.RecoverUnboundBuildEffect(
		ctx, recovery, "retry-reconciler", proof,
	); err == nil || !strings.Contains(err.Error(), "injected requeue failure") {
		t.Fatalf("injected build requeue error = %v", err)
	}
	assertBuildRetryUnknown(t, fixture.control, fixture.effectID, 1, nil)
	if observations := buildRetryObservationCount(t, fixture.control, fixture.effectID, "not_applied"); observations != 0 {
		t.Fatalf("rolled-back build requeue retained %d not-applied observations", observations)
	}
	if _, err := fixture.control.db.ExecContext(ctx, "DROP TRIGGER fail_build_requeue"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.RecoverUnboundBuildEffect(
		ctx, recovery, "retry-reconciler", proof,
	); err != nil {
		t.Fatalf("requeue machine-proved first attempt: %v", err)
	}
	// A replay is idempotent only because the exact not-applied witness is now
	// durable for this lease and composite proof.
	if err := fixture.control.RecoverUnboundBuildEffect(
		ctx, recovery, "retry-reconciler", proof,
	); err != nil {
		t.Fatalf("replay machine-proved first attempt: %v", err)
	}
	pending, err := listEffects(ctx, fixture.control, EffectPending)
	if err != nil || len(pending) != 1 || pending[0].ID != fixture.effectID || pending[0].Attempt != 1 {
		t.Fatalf("machine-proved pending build = %+v, %v", pending, err)
	}
	if notApplied := buildRetryObservationCount(t, fixture.control, fixture.effectID, "not_applied"); notApplied != 1 {
		t.Fatalf("not-applied witnesses = %d", notApplied)
	}

	second, err := fixture.control.ClaimNextEffect(ctx, "builder-worker-2")
	if err != nil {
		t.Fatal(err)
	}
	secondIdentity := durableBuildRetryIdentity(t, fixture.control, second)
	if second.Invocation().Attempt != 2 || secondIdentity.EffectAttempt != 2 ||
		secondIdentity.InvocationID == firstIdentity.InvocationID {
		t.Fatalf("second build identity = %+v; first = %+v", secondIdentity, firstIdentity)
	}
	result := validBuildResultForCandidate(t, fixture.effectID, "sworn-builder/1", fixture.candidate)
	if err := fixture.control.BindEffectResult(ctx, first, result); err == nil {
		t.Fatal("first-attempt lease bound a result after retry")
	}
	if err := fixture.control.CompleteEffect(ctx, first); err == nil {
		t.Fatal("first-attempt lease completed the retried build")
	}
	if err := fixture.control.BindEffectResult(ctx, second, result); err != nil {
		t.Fatalf("bind second-attempt result: %v", err)
	}
	if recovered, err := fixture.control.RecoverInterruptedEffects(ctx, "second builder stopped after binding"); err != nil || recovered != 1 {
		t.Fatalf("mark bound second attempt unknown = %d, %v", recovered, err)
	}
	assertBuildRetryUnknown(t, fixture.control, fixture.effectID, 2, result)
	if _, err := fixture.control.PrepareUnboundBuildRecovery(ctx, fixture.effectID, 2); err == nil {
		t.Fatal("bound build attempt received an unbound recovery lease")
	}
	if err := fixture.control.RecoverUnboundBuildEffect(
		ctx, recovery, "retry-reconciler", proof,
	); err == nil {
		t.Fatal("stale first-attempt recovery authority requeued the bound second attempt")
	}
	assertBuildRetryUnknown(t, fixture.control, fixture.effectID, 2, result)
	if _, err := fixture.control.ClaimNextEffect(ctx, "builder-worker-3"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("rejected bound retry became claimable: %v", err)
	}
}

func TestBuildRetryPreparationLeavesCorruptNullClaimWitnessStopped(t *testing.T) {
	fixture := newBuildRetryFixture(t)
	ctx := context.Background()
	lease, err := fixture.control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil {
		t.Fatal(err)
	}
	identity := durableBuildRetryIdentity(t, fixture.control, lease)
	if recovered, err := fixture.control.RecoverInterruptedEffects(ctx, "builder process stopped"); err != nil || recovered != 1 {
		t.Fatalf("mark build attempt unknown = %d, %v", recovered, err)
	}
	attemptRoot := filepath.Join(fixture.worker.WorkspaceRoot, identity.InvocationID)
	if err := os.Mkdir(attemptRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attemptRoot, "residue"), []byte("must remain"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Drop only the test database's immutability trigger to simulate a corrupt
	// or legacy NULL claimed receipt without granting it a v7 attempt witness.
	if _, err := fixture.control.db.ExecContext(ctx, "DROP TRIGGER observations_no_update"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.control.db.ExecContext(ctx, `
		UPDATE effect_observations SET receipt_json = NULL
		WHERE effect_id = ? AND attempt = ? AND kind = 'claimed'`,
		fixture.effectID, lease.Invocation().Attempt,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := fixture.control.PrepareUnboundBuildRecovery(
		ctx, fixture.effectID, lease.Invocation().Attempt,
	); err == nil || !strings.Contains(err.Error(), "attempt witness") {
		t.Fatalf("NULL claim witness preparation error = %v", err)
	}
	assertBuildRetryUnknown(t, fixture.control, fixture.effectID, lease.Invocation().Attempt, nil)
	if contents, err := os.ReadFile(filepath.Join(attemptRoot, "residue")); err != nil || string(contents) != "must remain" {
		t.Fatalf("prevalidation failure touched builder residue: %q, %v", contents, err)
	}
	if _, err := fixture.control.ClaimNextEffect(ctx, "new-builder"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("NULL claim witness became claimable: %v", err)
	}
}

func TestBuildRecoveryLeaseCannotCrossStoreBoundary(t *testing.T) {
	first := newBuildRetryFixture(t)
	second := newBuildRetryFixture(t)
	ctx := context.Background()
	lease, err := first.control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := first.control.RecoverInterruptedEffects(ctx, "builder stopped"); err != nil || recovered != 1 {
		t.Fatalf("mark first Store attempt unknown = %d, %v", recovered, err)
	}
	recovery, err := first.control.PrepareUnboundBuildRecovery(ctx, first.effectID, lease.Invocation().Attempt)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := first.worker.ReconcileUnbound(ctx, recovery.Invocation(), recovery.Challenge())
	if err != nil {
		t.Fatal(err)
	}
	if err := second.control.RecoverUnboundBuildEffect(
		ctx, recovery, "reconciler-2", proof,
	); err == nil || !strings.Contains(err.Error(), "Store-issued lease") {
		t.Fatalf("cross-Store recovery lease error = %v", err)
	}
	assertBuildRetryUnknown(t, first.control, first.effectID, lease.Invocation().Attempt, nil)
}

func TestBuildRecoveryProofCannotCrossEquivalentStoreBoundary(t *testing.T) {
	fixture := newBuildRetryFixture(t)
	ctx := context.Background()
	lease, err := fixture.control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := fixture.control.RecoverInterruptedEffects(ctx, "builder stopped"); err != nil || recovered != 1 {
		t.Fatalf("mark equivalent Store attempt unknown = %d, %v", recovered, err)
	}
	firstRecovery, err := fixture.control.PrepareUnboundBuildRecovery(
		ctx, fixture.effectID, lease.Invocation().Attempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := OpenConfigured(ctx, fixture.controlPath, ControlConfiguration{
		LocalCheckRuntimeManifestDigest: "sha256:" + strings.Repeat("e", 64),
		BuilderDispatchDigest:           fixture.builderDispatchDigest,
		Repository:                      fixture.repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	peerRecovery, err := peer.PrepareUnboundBuildRecovery(
		ctx, fixture.effectID, lease.Invocation().Attempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if firstRecovery.Challenge() == peerRecovery.Challenge() {
		t.Fatal("independent Store recovery leases reused a challenge")
	}
	firstProof, err := fixture.worker.ReconcileUnbound(
		ctx, firstRecovery.Invocation(), firstRecovery.Challenge(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.RecoverUnboundBuildEffect(
		ctx, peerRecovery, "peer-reconciler", firstProof,
	); err == nil || !strings.Contains(err.Error(), "proof does not match") {
		t.Fatalf("cross-Store recovery proof error = %v", err)
	}
	assertBuildRetryUnknown(t, fixture.control, fixture.effectID, lease.Invocation().Attempt, nil)
	peerProof, err := fixture.worker.ReconcileUnbound(
		ctx, peerRecovery.Invocation(), peerRecovery.Challenge(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.RecoverUnboundBuildEffect(
		ctx, peerRecovery, "peer-reconciler", peerProof,
	); err != nil {
		t.Fatalf("matching peer recovery proof: %v", err)
	}
}

func TestNativeBuildCompletionPublishesAttemptBeforeSuccess(t *testing.T) {
	fixture := newBuildRetryFixture(t)
	ctx := context.Background()
	lease, err := fixture.control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil {
		t.Fatal(err)
	}
	identity := durableBuildRetryIdentity(t, fixture.control, lease)
	runAtomicAdmissionGit(t, fixture.repository.Root(), "update-ref", "-d", fixture.candidate.Ref)
	result := validBuildResultForCandidate(t, fixture.effectID, "sworn-builder/1", fixture.candidate)
	if err := fixture.control.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if refs := nativeBuildRefs(t, fixture.repository); refs != "" {
		t.Fatalf("bound native result published before completion: %s", refs)
	}
	if err := fixture.control.CompleteEffect(ctx, lease); err != nil {
		t.Fatal(err)
	}
	assertNativeBuildRefs(t, fixture.repository, fixture.candidate, identity.InvocationID)
	journal, err := fixture.control.SucceededEffect(ctx, fixture.effectID)
	if err != nil || !bytes.Equal(journal.Result, result) {
		t.Fatalf("completed native build = %+v, %v", journal, err)
	}
}

func TestNativeBuildCompletionRejectsAttemptPublicationCollision(t *testing.T) {
	fixture := newBuildRetryFixture(t)
	ctx := context.Background()
	lease, err := fixture.control.ClaimNextEffect(ctx, "builder-worker")
	if err != nil {
		t.Fatal(err)
	}
	identity := durableBuildRetryIdentity(t, fixture.control, lease)
	attemptRef := "refs/sworn/v1/attempts/" + identity.InvocationID
	runAtomicAdmissionGit(t, fixture.repository.Root(), "update-ref", attemptRef, fixture.candidate.BaseCommit)
	result := validBuildResultForCandidate(t, fixture.effectID, "sworn-builder/1", fixture.candidate)
	if err := fixture.control.BindEffectResult(ctx, lease, result); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.CompleteEffect(ctx, lease); err == nil ||
		!strings.Contains(err.Error(), "collision") {
		t.Fatalf("colliding native completion error = %v", err)
	}
	running, err := listEffects(ctx, fixture.control, EffectRunning)
	if err != nil || len(running) != 1 || running[0].ID != fixture.effectID ||
		!bytes.Equal(running[0].Result, result) {
		t.Fatalf("publication collision changed native journal = %+v, %v", running, err)
	}
}

func TestBoundNativeBuildRecoveryConvergesAcrossPublicationCrashCuts(t *testing.T) {
	for _, prepublished := range []bool{false, true} {
		name := "before publication"
		if prepublished {
			name = "after publication"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newBuildRetryFixture(t)
			ctx := context.Background()
			lease, err := fixture.control.ClaimNextEffect(ctx, "builder-worker")
			if err != nil {
				t.Fatal(err)
			}
			identity := durableBuildRetryIdentity(t, fixture.control, lease)
			result := validBuildResultForCandidate(t, fixture.effectID, "sworn-builder/1", fixture.candidate)
			if err := fixture.control.BindEffectResult(ctx, lease, result); err != nil {
				t.Fatal(err)
			}
			runAtomicAdmissionGit(t, fixture.repository.Root(), "update-ref", "-d", fixture.candidate.Ref)
			if prepublished {
				if err := fixture.repository.EnsureAttemptCandidate(
					ctx, identity.InvocationID, fixture.candidate,
				); err != nil {
					t.Fatal(err)
				}
			}
			if recovered, err := fixture.control.RecoverInterruptedEffects(
				ctx, "controller stopped after binding",
			); err != nil || recovered != 1 {
				t.Fatalf("mark bound native attempt unknown = %d, %v", recovered, err)
			}
			if err := fixture.control.RecoverBoundEffect(
				ctx, fixture.effectID, lease.Invocation().Attempt, "reconciler-1",
			); err != nil {
				t.Fatal(err)
			}
			assertNativeBuildRefs(t, fixture.repository, fixture.candidate, identity.InvocationID)
		})
	}
}

func nativeBuildRefs(t *testing.T, repository *repo.Repository) string {
	t.Helper()
	return strings.TrimSpace(runAtomicAdmissionGit(
		t, repository.Root(), "for-each-ref", "--format=%(refname)", "refs/sworn/v1",
	))
}

func assertNativeBuildRefs(
	t *testing.T,
	repository *repo.Repository,
	candidate repo.Candidate,
	invocationID string,
) {
	t.Helper()
	for _, ref := range []string{candidate.Ref, "refs/sworn/v1/attempts/" + invocationID} {
		if got := strings.TrimSpace(runAtomicAdmissionGit(t, repository.Root(), "rev-parse", ref)); got != candidate.Commit {
			t.Fatalf("native build ref %s = %s, want %s", ref, got, candidate.Commit)
		}
	}
}

type buildRetryFixture struct {
	control               *Store
	controlPath           string
	repository            *repo.Repository
	candidate             repo.Candidate
	worker                effects.BuilderWorker
	builderDispatchDigest string
	effectID              string
}

func newBuildRetryFixture(t *testing.T) buildRetryFixture {
	t.Helper()
	ctx := context.Background()
	contained := newBuildRetryExecutor(t)
	repository, candidate := atomicAdmissionCandidate(t, false)
	plan := nativeBuildRetryPlan(t)
	if plan.Target().Repository != repository.Binding().RepositoryID {
		t.Fatalf("plan repository = %q, binding = %q", plan.Target().Repository, repository.Binding().RepositoryID)
	}
	workspaceRoot := filepath.Join(t.TempDir(), "builder-attempts")
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	worker := effects.BuilderWorker{
		Control:       inertBuildRetryControl{},
		Runner:        contained,
		Repository:    repository,
		WorkspaceRoot: workspaceRoot,
		Agent:         "store-build-retry-test",
		Argv:          []string{"/usr/bin/true"},
		Timeout:       time.Minute,
	}
	builderDispatchDigest, err := worker.DispatchDigest()
	if err != nil {
		t.Fatal(err)
	}
	controlPath := filepath.Join(t.TempDir(), "control.db")
	control, err := OpenConfigured(ctx, controlPath, ControlConfiguration{
		LocalCheckRuntimeManifestDigest: "sha256:" + strings.Repeat("e", 64),
		BuilderDispatchDigest:           builderDispatchDigest,
		Repository:                      repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	worker.Control = control
	if digest, err := control.PutPlan(ctx, plan); err != nil || digest != plan.Record().Digest {
		t.Fatalf("put native builder plan = %q, %v", digest, err)
	}
	create := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref,
		Work: plan.WorkIDs(),
	})
	if result, err := control.Apply(ctx, create); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("create native builder delivery = %+v, %v", result, err)
	}
	activate := testCommand(t, "cmd-activate", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: protocol.RawDigest([]byte("native-build-retry-authority")),
	})
	if result, err := control.Apply(ctx, activate); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("activate native builder delivery = %+v, %v", result, err)
	}
	work, _ := plan.Work(plan.WorkIDs()[0])
	dispatch := testCommand(t, "cmd-dispatch", engine.CommandDispatchBuild, 1, engine.DispatchBuildPayload{
		WorkID: plan.WorkIDs()[0], DispatchDigest: work.Digest(), BuilderDispatchDigest: builderDispatchDigest,
	})
	result, err := control.Apply(ctx, dispatch)
	if err != nil || result.Outcome != OutcomeApplied || len(result.EffectIDs) != 1 {
		t.Fatalf("dispatch native builder = %+v, %v", result, err)
	}
	return buildRetryFixture{
		control: control, controlPath: controlPath, repository: repository, candidate: candidate,
		worker: worker, builderDispatchDigest: builderDispatchDigest, effectID: result.EffectIDs[0],
	}
}

type inertBuildRetryControl struct{}

func (inertBuildRetryControl) State(context.Context, string) (engine.State, error) {
	return engine.State{}, errors.New("inert builder control")
}

func (inertBuildRetryControl) Plan(context.Context, string) (protocol.ExactPlan, error) {
	return protocol.ExactPlan{}, errors.New("inert builder control")
}

func nativeBuildRetryPlan(t *testing.T) protocol.ExactPlan {
	t.Helper()
	canonical := bytes.ReplaceAll(
		exampleExactPlan(t).Record().CanonicalJSON,
		[]byte("local:example"), []byte("repo-01"),
	)
	plan, err := protocol.ParseDeliveryPlan(canonical)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func durableBuildRetryIdentity(t *testing.T, control *Store, lease EffectLease) engine.BuildAttemptIdentity {
	t.Helper()
	identity, err := loadBuildAttemptIdentity(context.Background(), control.db, lease.effect)
	if err != nil {
		t.Fatal(err)
	}
	if identity.EffectID != lease.Invocation().ID || identity.EffectAttempt != lease.Invocation().Attempt ||
		!engine.ValidID(identity.InvocationID) {
		t.Fatalf("durable build attempt identity = %+v; lease = %+v", identity, lease.Invocation())
	}
	return identity
}

func assertBuildRetryUnknown(
	t *testing.T,
	control *Store,
	effectID string,
	attempt int64,
	result []byte,
) {
	t.Helper()
	unknown, err := listEffects(context.Background(), control, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != effectID ||
		unknown[0].Attempt != attempt || !bytes.Equal(unknown[0].Result, result) {
		t.Fatalf("stopped build attempt = %+v, %v; want effect=%s attempt=%d", unknown, err, effectID, attempt)
	}
}

func buildRetryObservationCount(t *testing.T, control *Store, effectID, kind string) int {
	t.Helper()
	var count int
	if err := control.db.QueryRowContext(context.Background(), `
		SELECT count(*) FROM effect_observations WHERE effect_id = ? AND kind = ?`,
		effectID, kind,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func newBuildRetryExecutor(t *testing.T) *executor.LinuxExecutor {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("opaque writable cleanup proofs require Linux")
	}
	writableRoot, err := os.MkdirTemp("/dev/shm", "sworn-store-build-retry-")
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
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	contained, err := executor.NewLinux(executor.Options{
		RuntimeRoot: runtimeRoot, WritableRoot: writableRoot,
		BubblewrapPath: "/usr/bin/true", SystemdRunPath: "/usr/bin/true", SystemctlPath: systemctl,
		Limits: executor.DefaultLimits(),
	})
	if err != nil {
		t.Skipf("construct Linux cleanup boundary: %v", err)
	}
	return contained
}
