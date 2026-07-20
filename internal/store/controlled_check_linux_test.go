//go:build linux

package store

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

type controlledCheckStoreFixture struct {
	base       *atomicAdmissionFixture
	plan       protocol.ExactPlan
	authority  *policy.Authority
	privateKey ed25519.PrivateKey
	ownership  *ControllerOwnership
	ownerID    string
}

func newControlledCheckStoreFixture(t *testing.T) *controlledCheckStoreFixture {
	t.Helper()
	ctx := context.Background()
	base := newAtomicAdmissionFixture(t, atomicAdmissionOptions{pendingCheck: true})
	state, err := base.control.State(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := base.control.Plan(ctx, state.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	authority, _, privateKey := authorityFixture(
		t, base.control, plan, 1, nil, false, controlledSourceMutation(plan, nil),
	)
	if err := os.Chmod(filepath.Dir(base.control.ControlPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := &controlledCheckStoreFixture{
		base: base, plan: plan, authority: authority, privateKey: privateKey,
		ownerID: "check-controller-1",
	}
	fixture.ownership, err = base.control.AcquireControllerOwnership(fixture.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.ownership.Activate(ctx, base.control, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if fixture.ownership != nil {
			_ = fixture.ownership.Close()
		}
	})
	return fixture
}

func (fixture *controlledCheckStoreFixture) permitRequest(
	t *testing.T,
	index int,
) (protocol.ExactPlan, policy.CheckPermitRequest, error) {
	t.Helper()
	_, plan, request, err := fixture.base.control.PendingCheckPermitRequest(
		context.Background(), fixture.ownership, fixture.ownerID,
		"run-1", fixture.plan.WorkIDs()[0], fixture.base.checkEffectIDs[index],
	)
	if err == nil && plan.Record().Digest != fixture.plan.Record().Digest {
		t.Fatal("pending check selector returned a different exact plan")
	}
	return plan, request, err
}

func (fixture *controlledCheckStoreFixture) authorize(
	t *testing.T,
	index int,
) (policy.CheckPermitRequest, policy.CurrentCheckPermit) {
	t.Helper()
	plan, request, err := fixture.permitRequest(t, index)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := fixture.authority.AuthorizeCheck(context.Background(), plan, request)
	if err != nil {
		t.Fatal(err)
	}
	return request, permit
}

func (fixture *controlledCheckStoreFixture) claim(
	t *testing.T,
	index int,
) (policy.CheckPermitRequest, AuthorizedCheckLease) {
	t.Helper()
	request, permit := fixture.authorize(t, index)
	lease, err := fixture.base.control.ClaimControlledCheck(
		context.Background(), fixture.ownership, fixture.authority, permit, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request, lease
}

func (fixture *controlledCheckStoreFixture) closeOwnership(t *testing.T) {
	t.Helper()
	if fixture.ownership == nil {
		return
	}
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
}

type checkMutationSnapshot struct {
	state        string
	attempt      int64
	owner        string
	resultBytes  int64
	observations int64
}

func controlledCheckSnapshot(t *testing.T, control *Store, effectID string) checkMutationSnapshot {
	t.Helper()
	var snapshot checkMutationSnapshot
	if err := control.db.QueryRow(`
		SELECT state, attempt, COALESCE(owner_id, ''), COALESCE(length(receipt_json), 0),
		       (SELECT COUNT(*) FROM effect_observations WHERE effect_id = effects.effect_id)
		FROM effects WHERE effect_id = ?`, effectID,
	).Scan(
		&snapshot.state, &snapshot.attempt, &snapshot.owner,
		&snapshot.resultBytes, &snapshot.observations,
	); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func controlledCheckResult(
	t *testing.T,
	fixture *controlledCheckStoreFixture,
	index int,
	effectID string,
	outcome string,
) json.RawMessage {
	t.Helper()
	ctx := context.Background()
	requestEffect, err := loadEffect(ctx, fixture.base.control.db, effectID)
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.ParseLocalCheckEffectRequest(requestEffect.Request)
	if err != nil {
		t.Fatal(err)
	}
	definitionType, definitionBytes, err := fixture.base.control.Artifact(ctx, request.DefinitionDigest)
	if err != nil || definitionType != "application/json" {
		t.Fatalf("load controlled check definition = %q, %v", definitionType, err)
	}
	definition, err := protocol.ParseLocalCheckDefinition(definitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	snapshotDigest, err := protocol.SnapshotDigest()
	if err != nil {
		t.Fatal(err)
	}
	environment := validContentEnvironment(request.RuntimeManifestDigest)
	environment.ProtocolSnapshotDigest = "sha256:" + snapshotDigest
	environmentPointer := fixture.base.putJSONArtifact(t, protocol.LocalEnvironmentMediaType, environment)
	receipt := protocol.LocalCheckReceipt{
		SchemaVersion: protocol.LocalCheckReceiptSchemaVersion,
		CheckID:       request.CheckID,
		RunID:         effectID,
		Definition: protocol.Artifact{
			Ref: request.DefinitionDigest, MediaType: "application/json", Digest: request.DefinitionDigest,
		},
		Candidate: protocol.CandidatePoint{
			Repository: fixture.base.candidate.RepositoryID,
			Commit:     fixture.base.candidate.Commit, Tree: fixture.base.candidate.Tree,
		},
		WorkspaceDigest:  testLocalCheckDigest(string(rune('1' + index))),
		Environment:      protocol.Environment{Kind: "local", Ref: environmentPointer.Digest},
		WorkspaceAccess:  "read_only",
		WorkingDirectory: definition.WorkingDirectory,
		Argv:             append([]string(nil), definition.Argv...),
		TimeoutSeconds:   definition.TimeoutSeconds,
		Network:          "none",
		StartedAt:        atomicAdmissionTime.Format(time.RFC3339Nano),
		CompletedAt:      atomicAdmissionTime.Format(time.RFC3339Nano),
		Stdout:           fixture.base.putCapturedArtifact(t, []byte("ok\n")),
		Stderr:           fixture.base.putCapturedArtifact(t, nil),
	}
	if outcome == engine.LocalCheckOutcomePass {
		receipt.Outcome = "pass"
	} else {
		receipt.Outcome, receipt.ExitCode = "not_admitted", 7
	}
	encodedReceipt, err := protocol.EncodeLocalCheckReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest, err := fixture.base.control.PutArtifact(
		ctx, localCheckReceiptMediaType, encodedReceipt.CanonicalJSON,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.EncodeLocalCheckEffectResult(engine.LocalCheckEffectResult{
		SchemaVersion: engine.LocalCheckEffectResultSchemaVersion,
		Outcome:       outcome,
		Receipt: protocol.Artifact{
			Ref: receiptDigest, MediaType: localCheckReceiptMediaType, Digest: receiptDigest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func advanceControlledCheckAuthority(
	t *testing.T,
	fixture *controlledCheckStoreFixture,
	status string,
) {
	t.Helper()
	advanced, _, _ := authorityFixture(
		t, fixture.base.control, fixture.plan, 2, fixture.privateKey, false,
		controlledSourceMutation(fixture.plan, func(source map[string]any) {
			if status != "active" {
				source["status"] = status
				source["maximum_grants"] = []any{}
			}
		}),
	)
	_, err := advanced.Approve(context.Background(), fixture.plan)
	if status == "active" && err != nil {
		t.Fatal(err)
	}
	if status != "active" && (err == nil || !strings.Contains(err.Error(), status)) {
		t.Fatalf("persist %s authority head = %v", status, err)
	}
}

func TestPublicStoreCheckBypassesFailClosedWithoutMutation(t *testing.T) {
	fixture := newControlledCheckStoreFixture(t)
	ctx := context.Background()
	effectID := fixture.base.checkEffectIDs[0]
	want := controlledCheckSnapshot(t, fixture.base.control, effectID)
	if _, err := fixture.base.control.ClaimNextEffect(ctx, "generic-check-worker"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("generic claim with pending controlled checks error = %v", err)
	}
	if _, err := fixture.base.control.RecoverInterruptedEffects(ctx, "raw check recovery"); err == nil {
		t.Fatal("raw interrupted-check recovery succeeded")
	}
	if err := fixture.base.control.RecoverBoundEffect(ctx, effectID, 1, "raw-check-recovery"); err == nil {
		t.Fatal("raw bound-check recovery succeeded")
	}
	if got := controlledCheckSnapshot(t, fixture.base.control, effectID); got != want {
		t.Fatalf("raw check boundary mutated pending effect: got %+v, want %+v", got, want)
	}

	_, authorized := fixture.claim(t, 0)
	generic := authorized.effectLease()
	running := controlledCheckSnapshot(t, fixture.base.control, effectID)
	if err := fixture.base.control.BindEffectResult(ctx, generic, json.RawMessage(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "authorized lease") {
		t.Fatalf("generic check result binding error = %v", err)
	}
	if err := fixture.base.control.CompleteEffect(ctx, generic); err == nil ||
		!strings.Contains(err.Error(), "authorized lease") {
		t.Fatalf("generic check completion error = %v", err)
	}
	if err := fixture.base.control.FailEffect(ctx, generic, "generic failure"); err == nil ||
		!strings.Contains(err.Error(), "controlled effect") {
		t.Fatalf("generic check failure error = %v", err)
	}
	if got := controlledCheckSnapshot(t, fixture.base.control, effectID); got != running {
		t.Fatalf("generic running boundary mutated check: got %+v, want %+v", got, running)
	}
}

func TestControlledCheckClaimIsExactOrderedAndCopiedCapabilitiesAreOneShot(t *testing.T) {
	fixture := newControlledCheckStoreFixture(t)
	ctx := context.Background()
	if _, _, err := fixture.permitRequest(t, 1); err == nil || !strings.Contains(err.Error(), "policy-ordered") {
		t.Fatalf("later pending check selector error = %v", err)
	}
	request, authorized := fixture.claim(t, 0)
	if request.CheckEffectID != fixture.base.checkEffectIDs[0] {
		t.Fatalf("controlled claim selected %q", request.CheckEffectID)
	}
	if _, _, err := fixture.permitRequest(t, 1); err == nil || !strings.Contains(err.Error(), "policy-ordered") {
		t.Fatalf("later check selected while first running: %v", err)
	}
	if _, err := fixture.base.control.PrepareAuthorizedCheckExecution(ctx, AuthorizedCheckLease{}); err == nil {
		t.Fatal("zero authorized check capability was prepared")
	}
	copyOfAuthorized := authorized
	prepared, err := fixture.base.control.PrepareAuthorizedCheckExecution(ctx, authorized)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.base.control.PrepareAuthorizedCheckExecution(ctx, copyOfAuthorized); err == nil {
		t.Fatalf("copied authorized check preparation error = %v", err)
	}
	if err := fixture.base.control.BindAuthorizedCheckResult(ctx, prepared, json.RawMessage(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "consumed authorized check lease") {
		t.Fatalf("unconsumed prepared check binding error = %v", err)
	}
	result := controlledCheckResult(t, fixture, 0, request.CheckEffectID, engine.LocalCheckOutcomePass)
	copyOfPrepared := prepared
	var invocation engine.JournalEffect
	observed, err := prepared.RunCheck(func(effect engine.JournalEffect) (json.RawMessage, error) {
		invocation = effect
		return result, nil
	})
	if err != nil || !jsonEqual(observed, result) || invocation.ID != request.CheckEffectID || invocation.Attempt != 1 {
		t.Fatalf("run prepared check = %+v, %v", invocation, err)
	}
	copyRan := false
	if _, err := copyOfPrepared.RunCheck(func(engine.JournalEffect) (json.RawMessage, error) {
		copyRan = true
		return nil, nil
	}); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("copied prepared check execution error = %v", err)
	}
	if copyRan {
		t.Fatal("copied prepared check capability reached its callback")
	}
	if err := fixture.base.control.BindAuthorizedCheckResult(ctx, prepared, result); err != nil {
		t.Fatal(err)
	}
	if err := fixture.base.control.CompleteAuthorizedCheck(ctx, prepared); err != nil {
		t.Fatal(err)
	}
	if err := fixture.base.control.CompleteAuthorizedCheck(ctx, copyOfPrepared); err == nil {
		t.Fatal("copied prepared check completed an attempt twice")
	}
	if _, second, err := fixture.permitRequest(t, 1); err != nil || second.CheckEffectID != fixture.base.checkEffectIDs[1] {
		t.Fatalf("next ordered check = %+v, %v", second, err)
	}
	effect, err := loadEffect(ctx, fixture.base.control.db, request.CheckEffectID)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := loadCheckAttemptIdentity(ctx, fixture.base.control.db, effect)
	if err != nil || identity.EffectID != effect.ID || identity.EffectAttempt != effect.Attempt {
		t.Fatalf("durable check attempt identity = %+v, %v", identity, err)
	}
}

func TestControlledCheckRejectsStaleRevokedTamperedAndForeignPermits(t *testing.T) {
	t.Run("tampered request", func(t *testing.T) {
		fixture := newControlledCheckStoreFixture(t)
		request, permit := fixture.authorize(t, 0)
		want := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID)
		mutations := map[string]func(*policy.CheckPermitRequest){
			"revision": func(value *policy.CheckPermitRequest) { value.StateRevision++ },
			"attempt":  func(value *policy.CheckPermitRequest) { value.WorkAttempt++ },
			"builder":  func(value *policy.CheckPermitRequest) { value.BuilderEffectID = "builder-effect-forged" },
			"effect":   func(value *policy.CheckPermitRequest) { value.CheckEffectID = fixture.base.checkEffectIDs[1] },
			"check":    func(value *policy.CheckPermitRequest) { value.CheckID = "check-forged" },
			"definition": func(value *policy.CheckPermitRequest) {
				value.DefinitionDigest = testLocalCheckDigest("f")
			},
			"runtime": func(value *policy.CheckPermitRequest) {
				value.RuntimeManifestDigest = testLocalCheckDigest("f")
			},
		}
		for name, mutate := range mutations {
			t.Run(name, func(t *testing.T) {
				changed := request
				mutate(&changed)
				if _, err := fixture.base.control.ClaimControlledCheck(
					context.Background(), fixture.ownership, fixture.authority, permit, changed,
				); err == nil {
					t.Fatal("tampered controlled check claim succeeded")
				}
				if got := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID); got != want {
					t.Fatalf("tampered claim mutated check: got %+v, want %+v", got, want)
				}
			})
		}
		if _, err := fixture.base.control.ClaimControlledCheck(
			context.Background(), fixture.ownership, fixture.authority,
			policy.CurrentCheckPermit{}, request,
		); err == nil {
			t.Fatal("zero check permit claimed an effect")
		}
		foreign, _, _ := authorityFixture(
			t, fixture.base.control, fixture.plan, 1, fixture.privateKey, false,
			controlledSourceMutation(fixture.plan, nil),
		)
		plan, foreignRequest, err := fixture.permitRequest(t, 0)
		if err != nil {
			t.Fatal(err)
		}
		foreignPermit, err := foreign.AuthorizeCheck(context.Background(), plan, foreignRequest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.base.control.ClaimControlledCheck(
			context.Background(), fixture.ownership, fixture.authority, foreignPermit, foreignRequest,
		); err == nil || !strings.Contains(err.Error(), "another authority") {
			t.Fatalf("foreign authority check permit error = %v", err)
		}
	})

	for _, status := range []string{"active", "revoked"} {
		t.Run("superseded by "+status, func(t *testing.T) {
			fixture := newControlledCheckStoreFixture(t)
			request, permit := fixture.authorize(t, 0)
			want := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID)
			advanceControlledCheckAuthority(t, fixture, status)
			if _, err := fixture.base.control.ClaimControlledCheck(
				context.Background(), fixture.ownership, fixture.authority, permit, request,
			); err == nil || !strings.Contains(err.Error(), "superseded") {
				t.Fatalf("%s superseded permit error = %v", status, err)
			}
			if got := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID); got != want {
				t.Fatalf("superseded permit mutated check: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestCheckPreparationRevalidatesSupersededAndRevokedAuthority(t *testing.T) {
	for _, status := range []string{"active", "revoked"} {
		t.Run(status, func(t *testing.T) {
			fixture := newControlledCheckStoreFixture(t)
			request, authorized := fixture.claim(t, 0)
			want := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID)
			advanceControlledCheckAuthority(t, fixture, status)
			if _, err := fixture.base.control.PrepareAuthorizedCheckExecution(
				context.Background(), authorized,
			); err == nil || !strings.Contains(err.Error(), "superseded") {
				t.Fatalf("prepare check after %s authority head error = %v", status, err)
			}
			if got := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID); got != want {
				t.Fatalf("rejected check preparation mutated attempt: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestPreparedCheckBanksResultAfterRevocationAndBoundCrashRecovery(t *testing.T) {
	fixture := newControlledCheckStoreFixture(t)
	ctx := context.Background()
	request, authorized := fixture.claim(t, 0)
	prepared, err := fixture.base.control.PrepareAuthorizedCheckExecution(ctx, authorized)
	if err != nil {
		t.Fatal(err)
	}
	result := controlledCheckResult(t, fixture, 0, request.CheckEffectID, engine.LocalCheckOutcomePass)
	if _, err := prepared.RunCheck(func(engine.JournalEffect) (json.RawMessage, error) {
		return result, nil
	}); err != nil {
		t.Fatal(err)
	}
	advanceControlledCheckAuthority(t, fixture, "revoked")
	if err := fixture.base.control.BindAuthorizedCheckResult(ctx, prepared, result); err != nil {
		t.Fatalf("bind banked check after revocation: %v", err)
	}
	fixture.closeOwnership(t)

	recoveryOwnerID := "check-recovery-bound"
	recovery, err := fixture.base.control.AcquireControllerOwnership(recoveryOwnerID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovery.Close() })
	if count, err := fixture.base.control.RecoverControlledInterruptedEffects(
		ctx, recovery, recoveryOwnerID, "controller stopped after check result binding",
	); err != nil || count != 1 {
		t.Fatalf("mark bound check unknown = %d, %v", count, err)
	}
	unknown := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID)
	if unknown.state != string(EffectUnknown) || unknown.resultBytes == 0 {
		t.Fatalf("bound unknown check = %+v", unknown)
	}
	if err := fixture.base.control.RecoverControlledBoundEffect(
		ctx, recovery, recoveryOwnerID, request.CheckEffectID, 1,
	); err != nil {
		t.Fatalf("recover bound check without current authority: %v", err)
	}
	if err := fixture.base.control.RecoverControlledBoundEffect(
		ctx, recovery, recoveryOwnerID, request.CheckEffectID, 1,
	); err != nil {
		t.Fatalf("replay bound check recovery: %v", err)
	}
	if got := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID); got.state != string(EffectSucceeded) {
		t.Fatalf("recovered bound check = %+v", got)
	}
}

func TestUnboundCheckRecoveryRequiresExactOneShotProofAndRequeuesAttempt(t *testing.T) {
	fixture := newControlledCheckStoreFixture(t)
	ctx := context.Background()
	request, authorized := fixture.claim(t, 0)
	fixture.closeOwnership(t)
	recoveryOwnerID := "check-recovery-unbound"
	recoveryOwnership, err := fixture.base.control.AcquireControllerOwnership(recoveryOwnerID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recoveryOwnership.Close() })
	if count, err := fixture.base.control.RecoverControlledInterruptedEffects(
		ctx, recoveryOwnership, recoveryOwnerID, "controller stopped before check result binding",
	); err != nil || count != 1 {
		t.Fatalf("mark unbound check unknown = %d, %v", count, err)
	}
	if _, err := (CheckRecoveryLease{}).ReconcileCheck(
		func(engine.JournalEffect) (executor.ContentBoundCleanup, error) {
			return executor.ContentBoundCleanup{}, nil
		},
	); err == nil {
		t.Fatal("zero check recovery capability reached reconciliation")
	}
	mismatchLease, err := fixture.base.control.PrepareControlledUnboundCheckRecovery(
		ctx, recoveryOwnership, recoveryOwnerID, request.CheckEffectID, authorized.effect.Attempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	contained := newBuildRetryExecutor(t)
	mismatchCleanup, err := contained.ReconcileContentBound(ctx, "check-attempt-foreign")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mismatchLease.ReconcileCheck(
		func(engine.JournalEffect) (executor.ContentBoundCleanup, error) { return mismatchCleanup, nil },
	); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched check cleanup error = %v", err)
	}
	mismatchRan := false
	if _, err := mismatchLease.ReconcileCheck(
		func(engine.JournalEffect) (executor.ContentBoundCleanup, error) {
			mismatchRan = true
			return mismatchCleanup, nil
		},
	); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("reused mismatched check recovery error = %v", err)
	}
	if mismatchRan {
		t.Fatal("consumed mismatched check recovery ran twice")
	}

	lease, err := fixture.base.control.PrepareControlledUnboundCheckRecovery(
		ctx, recoveryOwnership, recoveryOwnerID, request.CheckEffectID, authorized.effect.Attempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := contained.ReconcileContentBound(ctx, lease.identity.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	copyOfLease := lease
	var invocation engine.JournalEffect
	proof, err := lease.ReconcileCheck(func(effect engine.JournalEffect) (executor.ContentBoundCleanup, error) {
		invocation = effect
		return cleanup, nil
	})
	if err != nil || invocation.ID != request.CheckEffectID || invocation.Attempt != 1 {
		t.Fatalf("reconcile exact unbound check = %+v, %v", invocation, err)
	}
	copyRan := false
	if _, err := copyOfLease.ReconcileCheck(
		func(engine.JournalEffect) (executor.ContentBoundCleanup, error) {
			copyRan = true
			return cleanup, nil
		},
	); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("copied check recovery error = %v", err)
	}
	if copyRan {
		t.Fatal("copied check recovery capability ran twice")
	}
	peer, err := fixture.base.control.PrepareControlledUnboundCheckRecovery(
		ctx, recoveryOwnership, recoveryOwnerID, request.CheckEffectID, authorized.effect.Attempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.base.control.RecoverControlledUnboundCheckEffect(
		ctx, recoveryOwnership, recoveryOwnerID, peer, proof,
	); err == nil || !strings.Contains(err.Error(), "proof does not match") {
		t.Fatalf("cross-issuance check retry proof error = %v", err)
	}
	if err := fixture.base.control.RecoverControlledUnboundCheckEffect(
		ctx, recoveryOwnership, recoveryOwnerID, lease, proof,
	); err != nil {
		t.Fatalf("recover exact unbound check: %v", err)
	}
	if err := fixture.base.control.RecoverControlledUnboundCheckEffect(
		ctx, recoveryOwnership, recoveryOwnerID, lease, proof,
	); err != nil {
		t.Fatalf("replay exact unbound check recovery: %v", err)
	}
	if got := controlledCheckSnapshot(t, fixture.base.control, request.CheckEffectID); got.state != string(EffectPending) || got.attempt != 1 {
		t.Fatalf("requeued check attempt = %+v", got)
	}
	if err := recoveryOwnership.Activate(ctx, fixture.base.control, recoveryOwnerID); err != nil {
		t.Fatal(err)
	}
	_, plan, retryRequest, err := fixture.base.control.PendingCheckPermitRequest(
		ctx, recoveryOwnership, recoveryOwnerID, "run-1", fixture.plan.WorkIDs()[0], request.CheckEffectID,
	)
	if err != nil {
		t.Fatal(err)
	}
	retryPermit, err := fixture.authority.AuthorizeCheck(ctx, plan, retryRequest)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := fixture.base.control.ClaimControlledCheck(
		ctx, recoveryOwnership, fixture.authority, retryPermit, retryRequest,
	)
	if err != nil || retry.effect.Attempt != 2 {
		t.Fatalf("claim requeued check attempt = %+v, %v", retry.effect, err)
	}
}

func TestSubmissionAdmissionIgnoresLaterCurrentAuthorityRevocation(t *testing.T) {
	fixture := newAtomicAdmissionFixture(t, atomicAdmissionOptions{})
	ctx := context.Background()
	state, err := fixture.control.State(ctx, fixture.command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := fixture.control.Plan(ctx, state.PlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	revoked, _, _ := authorityFixture(
		t, fixture.control, plan, 2, nil, false,
		controlledSourceMutation(plan, func(source map[string]any) {
			source["status"] = "revoked"
			source["maximum_grants"] = []any{}
		}),
	)
	if _, err := revoked.Approve(ctx, plan); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("persist later revocation = %v", err)
	}
	result, err := fixture.control.Apply(ctx, fixture.command)
	if err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("historically authorized submission admission = %+v, %v", result, err)
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	return string(left) == string(right)
}
