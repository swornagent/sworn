//go:build linux

package store

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

type controlledBuildStoreFixture struct {
	control       *Store
	plan          protocol.ExactPlan
	authority     *policy.Authority
	privateKey    ed25519.PrivateKey
	ownership     *ControllerOwnership
	ownerID       string
	request       policy.BuildPermitRequest
	permit        policy.CurrentBuildPermit
	command       engine.Command
	candidate     repo.Candidate
	builderDigest string
}

func newControlledBuildStoreFixture(
	t *testing.T,
	mutateSource func(map[string]any),
) *controlledBuildStoreFixture {
	t.Helper()
	ctx := context.Background()
	repository, candidate := atomicAdmissionCandidate(t, false)
	plan := nativeBuildRetryPlan(t)
	builderDigest := "sha256:" + strings.Repeat("d", 64)
	control, err := OpenConfigured(ctx, filepath.Join(t.TempDir(), "control.db"), ControlConfiguration{
		BuilderDispatchDigest: builderDigest,
		Repository:            repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &controlledBuildStoreFixture{
		control: control, plan: plan, ownerID: "controller-1",
		candidate: candidate, builderDigest: builderDigest,
	}
	t.Cleanup(func() {
		if fixture.ownership != nil {
			_ = fixture.ownership.Close()
		}
		_ = control.Close()
	})
	if digest, err := control.PutPlan(ctx, plan); err != nil || digest != plan.Record().Digest {
		t.Fatalf("put controlled plan = %q, %v", digest, err)
	}
	create := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
		Repository: plan.Target().Repository, TargetRef: plan.Target().Ref, Work: plan.WorkIDs(),
	})
	if result, err := control.Apply(ctx, create); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("create controlled delivery = %+v, %v", result, err)
	}
	activate := testCommand(t, "cmd-activate", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: protocol.RawDigest([]byte("controlled-build-test-authority")),
	})
	if result, err := control.Apply(ctx, activate); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("activate controlled delivery = %+v, %v", result, err)
	}
	fixture.authority, _, fixture.privateKey = authorityFixture(
		t, control, plan, 1, nil, false, controlledSourceMutation(plan, mutateSource),
	)
	if err := os.Chmod(filepath.Dir(control.ControlPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.ownership, err = control.AcquireControllerOwnership(fixture.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.ownership.Activate(ctx, control, fixture.ownerID); err != nil {
		t.Fatal(err)
	}
	fixture.request = fixture.permitRequest(t)
	fixture.permit, err = fixture.authority.AuthorizeBuild(ctx, plan, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.command = testCommand(t, "cmd-dispatch", engine.CommandDispatchBuild, fixture.request.StateRevision,
		engine.DispatchBuildPayload{
			WorkID: fixture.request.WorkID, DispatchDigest: fixture.request.Contract.Digest(),
			BuilderDispatchDigest: builderDigest,
		})
	return fixture
}

func (fixture *controlledBuildStoreFixture) permitRequest(t *testing.T) policy.BuildPermitRequest {
	t.Helper()
	state, err := fixture.control.State(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	workID := fixture.plan.WorkIDs()[0]
	contract, exists := fixture.plan.Work(workID)
	if !exists {
		t.Fatal("fixture work contract is absent")
	}
	attempt := state.Work[0].Attempt
	if state.Work[0].State == engine.WorkReady {
		attempt++
	}
	return policy.BuildPermitRequest{
		ControllerID: fixture.ownerID, RunID: state.RunID, StateRevision: state.Revision,
		WorkID: workID, WorkAttempt: attempt, Contract: contract,
		BuilderDispatchDigest: fixture.builderDigest,
	}
}

func (fixture *controlledBuildStoreFixture) dispatch(t *testing.T) ApplyResult {
	t.Helper()
	result, err := fixture.control.ApplyControlledBuild(
		context.Background(), fixture.ownership, fixture.authority,
		fixture.permit, fixture.request, fixture.command,
	)
	if err != nil || result.Outcome != OutcomeApplied || len(result.EffectIDs) != 1 {
		t.Fatalf("controlled dispatch = %+v, %v", result, err)
	}
	return result
}

func (fixture *controlledBuildStoreFixture) executionPermit(
	t *testing.T,
) (policy.BuildPermitRequest, policy.CurrentBuildPermit) {
	t.Helper()
	request := fixture.permitRequest(t)
	permit, err := fixture.authority.AuthorizeBuild(context.Background(), fixture.plan, request)
	if err != nil {
		t.Fatal(err)
	}
	return request, permit
}

type buildMutationSnapshot struct {
	revision, attempt, observations int64
	state, owner                    string
}

func buildSnapshot(t *testing.T, control *Store) buildMutationSnapshot {
	t.Helper()
	var snapshot buildMutationSnapshot
	var owner sql.NullString
	if err := control.db.QueryRow(`
		SELECT runs.revision, effects.state, effects.attempt, effects.owner_id,
		       (SELECT COUNT(*) FROM effect_observations)
		FROM runs JOIN effects ON effects.run_id = runs.run_id
		WHERE runs.run_id = 'run-1'`,
	).Scan(&snapshot.revision, &snapshot.state, &snapshot.attempt, &owner, &snapshot.observations); err != nil {
		t.Fatal(err)
	}
	if owner.Valid {
		snapshot.owner = owner.String
	}
	return snapshot
}

func TestPublicStoreBuildBypassesFailClosedWithoutMutation(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	commandsBefore := tableCount(t, fixture.control, "commands")
	if result, err := fixture.control.Apply(context.Background(), fixture.command); err == nil ||
		result.CommandID != "" || result.RunID != "" || len(result.EffectIDs) != 0 {
		t.Fatalf("raw build Apply = %+v, %v", result, err)
	}
	if tableCount(t, fixture.control, "commands") != commandsBefore ||
		tableCount(t, fixture.control, "effects") != 0 {
		t.Fatal("raw build Apply mutated durable control truth")
	}

	fixture.dispatch(t)
	want := buildSnapshot(t, fixture.control)
	if _, err := fixture.control.ClaimNextEffect(context.Background(), "generic-worker"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("generic claim with pending build error = %v", err)
	}
	if got := buildSnapshot(t, fixture.control); got != want {
		t.Fatalf("generic claim mutated build: got %+v, want %+v", got, want)
	}
	if _, err := fixture.control.ClaimPendingBuild(context.Background(), "run-1", "generic-worker"); err == nil {
		t.Fatal("raw build-specific claim succeeded")
	}
	if _, err := fixture.control.RecoverInterruptedEffects(context.Background(), "raw recovery"); err == nil {
		t.Fatal("raw interrupted-effect recovery succeeded")
	}
	if err := fixture.control.RecoverBoundEffect(
		context.Background(), "effect-raw", 1, "generic-worker",
	); err == nil {
		t.Fatal("raw bound-effect recovery succeeded")
	}
}

func TestControlledBuildRejectsSelectorDriftAndForeignCapabilitiesWithoutMutation(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	fixture.dispatch(t)
	request, permit := fixture.executionPermit(t)
	want := buildSnapshot(t, fixture.control)
	wrongDigest := "sha256:" + strings.Repeat("e", 64)
	for name, mutate := range map[string]func(*policy.BuildPermitRequest){
		"work":    func(value *policy.BuildPermitRequest) { value.WorkID = "work-other" },
		"attempt": func(value *policy.BuildPermitRequest) { value.WorkAttempt++ },
		"digest":  func(value *policy.BuildPermitRequest) { value.BuilderDispatchDigest = wrongDigest },
	} {
		t.Run(name, func(t *testing.T) {
			drifted := request
			mutate(&drifted)
			if _, err := fixture.control.ClaimControlledBuild(
				context.Background(), fixture.ownership, fixture.authority, permit, drifted,
			); err == nil {
				t.Fatal("drifted controlled claim succeeded")
			}
			if got := buildSnapshot(t, fixture.control); got != want {
				t.Fatalf("drifted claim mutated build: got %+v, want %+v", got, want)
			}
		})
	}
	if _, err := fixture.control.ClaimControlledBuild(
		context.Background(), nil, fixture.authority, permit, request,
	); err == nil {
		t.Fatal("nil ownership controlled claim succeeded")
	}
	foreignStore := openTestStore(t, filepath.Join(t.TempDir(), "foreign-control.db"))
	t.Cleanup(func() { _ = foreignStore.Close() })
	if err := os.Chmod(filepath.Dir(foreignStore.ControlPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	foreignOwnership, err := foreignStore.AcquireControllerOwnership("controller-foreign")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = foreignOwnership.Close() })
	if err := foreignOwnership.Activate(context.Background(), foreignStore, "controller-foreign"); err != nil {
		t.Fatal(err)
	}
	foreignRequest := request
	foreignRequest.ControllerID = "controller-foreign"
	if _, err := fixture.control.ClaimControlledBuild(
		context.Background(), foreignOwnership, fixture.authority, permit, foreignRequest,
	); err == nil {
		t.Fatal("foreign Store ownership controlled claim succeeded")
	}
	if _, err := fixture.control.ClaimControlledBuild(
		context.Background(), fixture.ownership, fixture.authority,
		policy.CurrentBuildPermit{}, request,
	); err == nil {
		t.Fatal("zero permit controlled claim succeeded")
	}
	foreignAuthority, _, _ := authorityFixture(
		t, fixture.control, fixture.plan, 1, fixture.privateKey, false,
		controlledSourceMutation(fixture.plan, nil),
	)
	foreignPermit, err := foreignAuthority.AuthorizeBuild(context.Background(), fixture.plan, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.control.ClaimControlledBuild(
		context.Background(), fixture.ownership, fixture.authority, foreignPermit, request,
	); err == nil {
		t.Fatal("foreign authority permit controlled claim succeeded")
	}
	if got := buildSnapshot(t, fixture.control); got != want {
		t.Fatalf("foreign capability checks mutated build: got %+v, want %+v", got, want)
	}
}

func TestControlledBuildApplyRejectsExactBindingDriftWithoutMutation(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	wrongDigest := "sha256:" + strings.Repeat("e", 64)
	for name, mutate := range map[string]func(*engine.Command){
		"run":      func(command *engine.Command) { command.RunID = "run-other" },
		"revision": func(command *engine.Command) { command.ExpectedRevision++ },
		"work": func(command *engine.Command) {
			command.Payload = mustBuildPayload(t, engine.DispatchBuildPayload{
				WorkID: "work-other", DispatchDigest: fixture.request.Contract.Digest(),
				BuilderDispatchDigest: fixture.builderDigest,
			})
		},
		"contract digest": func(command *engine.Command) {
			command.Payload = mustBuildPayload(t, engine.DispatchBuildPayload{
				WorkID: fixture.request.WorkID, DispatchDigest: wrongDigest,
				BuilderDispatchDigest: fixture.builderDigest,
			})
		},
		"builder digest": func(command *engine.Command) {
			command.Payload = mustBuildPayload(t, engine.DispatchBuildPayload{
				WorkID: fixture.request.WorkID, DispatchDigest: fixture.request.Contract.Digest(),
				BuilderDispatchDigest: wrongDigest,
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			command := fixture.command
			mutate(&command)
			if _, err := fixture.control.ApplyControlledBuild(
				context.Background(), fixture.ownership, fixture.authority,
				fixture.permit, fixture.request, command,
			); err == nil {
				t.Fatal("drifted controlled Apply succeeded")
			}
			if tableCount(t, fixture.control, "commands") != 2 ||
				tableCount(t, fixture.control, "effects") != 0 {
				t.Fatal("drifted controlled Apply mutated command or effect truth")
			}
		})
	}
}

func mustBuildPayload(t *testing.T, payload engine.DispatchBuildPayload) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestDurableAuthorityAdvanceInvalidatesControlledApplyAndClaim(t *testing.T) {
	t.Run("apply", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		advanceAuthorityToRevoked(t, fixture)
		if _, err := fixture.control.ApplyControlledBuild(
			context.Background(), fixture.ownership, fixture.authority,
			fixture.permit, fixture.request, fixture.command,
		); err == nil || !strings.Contains(err.Error(), "superseded") {
			t.Fatalf("superseded controlled Apply error = %v", err)
		}
		if tableCount(t, fixture.control, "effects") != 0 || tableCount(t, fixture.control, "commands") != 2 {
			t.Fatal("superseded controlled Apply mutated command or effect truth")
		}
	})

	t.Run("claim", func(t *testing.T) {
		fixture := newControlledBuildStoreFixture(t, nil)
		fixture.dispatch(t)
		request, permit := fixture.executionPermit(t)
		want := buildSnapshot(t, fixture.control)
		advanceAuthorityToRevoked(t, fixture)
		if _, err := fixture.control.ClaimControlledBuild(
			context.Background(), fixture.ownership, fixture.authority, permit, request,
		); err == nil || !strings.Contains(err.Error(), "superseded") {
			t.Fatalf("superseded controlled claim error = %v", err)
		}
		if got := buildSnapshot(t, fixture.control); got != want {
			t.Fatalf("superseded claim mutated build: got %+v, want %+v", got, want)
		}
	})
}

func advanceAuthorityToRevoked(t *testing.T, fixture *controlledBuildStoreFixture) {
	t.Helper()
	revoked, _, _ := authorityFixture(
		t, fixture.control, fixture.plan, 2, fixture.privateKey, false,
		controlledSourceMutation(fixture.plan, func(source map[string]any) {
			source["status"] = "revoked"
			source["maximum_grants"] = []any{}
		}),
	)
	if _, err := revoked.Approve(context.Background(), fixture.plan); err == nil ||
		!strings.Contains(err.Error(), "revoked") {
		t.Fatalf("persist revocation = %v", err)
	}
}

func controlledSourceMutation(
	plan protocol.ExactPlan,
	additional func(map[string]any),
) func(map[string]any) {
	return func(source map[string]any) {
		source["repository"] = plan.Target().Repository
		if grants, ok := source["maximum_grants"].([]any); ok {
			for _, rawGrant := range grants {
				grant, _ := rawGrant.(map[string]any)
				target, _ := grant["target"].(map[string]any)
				if target != nil {
					target["repository"] = plan.Target().Repository
				}
			}
		}
		if additional != nil {
			additional(source)
		}
	}
}

func TestNativeBuildLifecycleRequiresPreparedCapabilityAndSurvivesPostPreparationRevocation(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	result := fixture.dispatch(t)
	request, permit := fixture.executionPermit(t)
	lease, err := fixture.control.ClaimControlledBuild(
		context.Background(), fixture.ownership, fixture.authority, permit, request,
	)
	if err != nil {
		t.Fatal(err)
	}
	generic := lease.effectLease()
	buildResult := validBuildResultForCandidate(t, result.EffectIDs[0], "controlled-store-test", fixture.candidate)
	if err := fixture.control.BindAuthorizedBuildResult(
		context.Background(), PreparedAuthorizedBuildLease{}, buildResult,
	); err == nil {
		t.Fatal("zero prepared capability bound a build result")
	}
	if _, err := fixture.control.PrepareNativeBuildExecution(context.Background(), generic); err == nil {
		t.Fatal("generic build lease crossed native execution boundary")
	}
	if err := fixture.control.BindEffectResult(context.Background(), generic, buildResult); err == nil {
		t.Fatal("generic build lease crossed result-binding boundary")
	}
	if err := fixture.control.CompleteEffect(context.Background(), generic); err == nil {
		t.Fatal("generic build lease crossed publication boundary")
	}
	invocation, prepared, err := fixture.control.PrepareAuthorizedBuildExecution(context.Background(), lease)
	if err != nil || invocation.ID != result.EffectIDs[0] {
		t.Fatalf("prepare authorized build = %+v, %v", invocation, err)
	}
	advanceAuthorityToRevoked(t, fixture)
	if err := fixture.control.BindAuthorizedBuildResult(context.Background(), prepared, buildResult); err != nil {
		t.Fatalf("bind prepared build after post-preparation revocation: %v", err)
	}
	if err := fixture.control.CompleteAuthorizedBuild(context.Background(), prepared); err != nil {
		t.Fatalf("complete prepared build after post-preparation revocation: %v", err)
	}
	if err := fixture.control.CompleteAuthorizedBuild(context.Background(), prepared); err == nil {
		t.Fatal("stale prepared capability completed the build twice")
	}
	var state string
	if err := fixture.control.db.QueryRow(
		"SELECT state FROM effects WHERE effect_id = ?", result.EffectIDs[0],
	).Scan(&state); err != nil || state != string(EffectSucceeded) {
		t.Fatalf("completed prepared effect state = %q, %v", state, err)
	}
}

func TestReleasedOwnershipInvalidatesControlledBuild(t *testing.T) {
	fixture := newControlledBuildStoreFixture(t, nil)
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.control.ApplyControlledBuild(
		context.Background(), fixture.ownership, fixture.authority,
		fixture.permit, fixture.request, fixture.command,
	); err == nil {
		t.Fatal("released ownership authorized build dispatch")
	}
	if tableCount(t, fixture.control, "effects") != 0 {
		t.Fatal("released ownership mutated build effects")
	}
}
