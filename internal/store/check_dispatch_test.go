package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

const dispatchRuntimeDigest = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func TestCheckDispatchCommitsExactPolicyOrderAndReplays(t *testing.T) {
	t.Parallel()
	fixture := newCheckDispatchFixture(t, checkDispatchFixtureOptions{configured: true, completeBuilder: true})
	ctx := context.Background()

	result, err := fixture.control.Apply(ctx, fixture.command)
	if err != nil || result.Revision != 3 || len(result.EffectIDs) != 2 {
		t.Fatalf("checks.dispatch = %+v, %v", result, err)
	}
	replayed, err := fixture.control.Apply(ctx, fixture.command)
	if err != nil || !replayed.Replayed || replayed.Revision != result.Revision ||
		strings.Join(replayed.EffectIDs, ",") != strings.Join(result.EffectIDs, ",") {
		t.Fatalf("checks.dispatch replay = %+v, %v; want %+v", replayed, err, result)
	}
	state, err := fixture.control.State(ctx, "run-1")
	if err != nil || state.Work[0].State != engine.WorkChecking || state.Work[0].Attempt != 1 {
		t.Fatalf("state after checks.dispatch = %+v, %v", state, err)
	}

	first, err := fixture.control.ClaimNextEffect(ctx, "check-worker-1")
	if err != nil || first.Invocation().ID != result.EffectIDs[0] {
		t.Fatalf("first policy-ordered claim = %q, %v; want %q", first.Invocation().ID, err, result.EffectIDs[0])
	}
	if lease, err := fixture.control.ClaimNextEffect(ctx, "check-worker-2"); !errors.Is(err, ErrNoPendingEffect) {
		t.Fatalf("later policy check leased while first was running: %+v, %v", lease, err)
	}
	second, err := loadEffect(ctx, fixture.control.db, result.EffectIDs[1])
	if err != nil || second.State != EffectPending {
		t.Fatalf("second policy-ordered effect = %+v, %v", second, err)
	}
	requests := []json.RawMessage{first.Invocation().Request, second.Request}
	for index, raw := range requests {
		request, err := engine.ParseLocalCheckEffectRequest(raw)
		if err != nil || request.CheckID != fixture.requirements[index].CheckID ||
			request.DefinitionDigest != fixture.requirements[index].Definition.Digest ||
			request.RuntimeManifestDigest != dispatchRuntimeDigest ||
			request.BuilderEffectID != fixture.builderEffectID {
			t.Fatalf("claimed check %d = %+v, %v", index, request, err)
		}
	}
}

func TestCheckDispatchPreconditionsRollBackWithoutDurableRejection(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T, *checkDispatchFixture){
		"runtime not configured": func(t *testing.T, fixture *checkDispatchFixture) {},
		"builder still pending":  func(t *testing.T, fixture *checkDispatchFixture) {},
		"runtime drift": func(t *testing.T, fixture *checkDispatchFixture) {
			payload := fixture.payload(t)
			payload.RuntimeManifestDigest = testLocalCheckDigest("f")
			fixture.replacePayload(t, payload)
		},
		"policy selection drift": func(t *testing.T, fixture *checkDispatchFixture) {
			payload := fixture.payload(t)
			payload.Checks[0].DefinitionDigest = testLocalCheckDigest("f")
			fixture.replacePayload(t, payload)
		},
		"policy order drift": func(t *testing.T, fixture *checkDispatchFixture) {
			payload := fixture.payload(t)
			payload.Checks[0], payload.Checks[1] = payload.Checks[1], payload.Checks[0]
			fixture.replacePayload(t, payload)
		},
		"missing policy selection": func(t *testing.T, fixture *checkDispatchFixture) {
			payload := fixture.payload(t)
			payload.Checks = payload.Checks[:1]
			fixture.replacePayload(t, payload)
		},
		"extra policy selection": func(t *testing.T, fixture *checkDispatchFixture) {
			payload := fixture.payload(t)
			payload.Checks = append(payload.Checks, engine.CheckSelection{
				CheckID: "extra", DefinitionDigest: testLocalCheckDigest("f"),
			})
			fixture.replacePayload(t, payload)
		},
		"missing historical authority":  func(t *testing.T, fixture *checkDispatchFixture) {},
		"authority after builder start": func(t *testing.T, fixture *checkDispatchFixture) {},
		"state work projection drift":   func(t *testing.T, fixture *checkDispatchFixture) {},
	}

	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			options := checkDispatchFixtureOptions{
				configured: name != "runtime not configured", completeBuilder: name != "builder still pending",
			}
			if name == "missing historical authority" {
				options.missingAuthority = true
			}
			if name == "authority after builder start" {
				options.builderStartedAt = "2026-07-19T00:00:29Z"
			}
			if name == "state work projection drift" {
				options.extraStateWork = true
			}
			fixture := newCheckDispatchFixture(t, options)
			mutate(t, fixture)
			if result, err := fixture.control.Apply(context.Background(), fixture.command); err == nil {
				t.Fatalf("drifted checks.dispatch applied: %+v", result)
			}
			fixture.assertBeforeDispatch(t)
		})
	}
}

func TestCheckDispatchEffectInsertionFailureRollsBackWholeCommand(t *testing.T) {
	t.Parallel()
	fixture := newCheckDispatchFixture(t, checkDispatchFixtureOptions{configured: true, completeBuilder: true})
	ctx := context.Background()
	if _, err := fixture.control.db.Exec(`
		CREATE TEMP TRIGGER fail_second_check
		BEFORE INSERT ON effects
		WHEN NEW.kind = 'check.local' AND NEW.ordinal = 1
		BEGIN
			SELECT RAISE(ABORT, 'injected second check failure');
		END`); err != nil {
		t.Fatal(err)
	}
	if result, err := fixture.control.Apply(ctx, fixture.command); err == nil {
		t.Fatalf("checks.dispatch survived injected effect failure: %+v", result)
	}
	fixture.assertBeforeDispatch(t)
	if _, err := fixture.control.db.Exec("DROP TRIGGER fail_second_check"); err != nil {
		t.Fatal(err)
	}
	if result, err := fixture.control.Apply(ctx, fixture.command); err != nil || len(result.EffectIDs) != 2 {
		t.Fatalf("checks.dispatch retry = %+v, %v", result, err)
	}
}

type checkDispatchFixture struct {
	control         *Store
	plan            protocol.ExactPlan
	builderEffectID string
	requirements    []protocol.LocalCheckRequirement
	command         engine.Command
}

type checkDispatchFixtureOptions struct {
	configured       bool
	completeBuilder  bool
	missingAuthority bool
	builderStartedAt string
	extraStateWork   bool
}

func newCheckDispatchFixture(t *testing.T, options checkDispatchFixtureOptions) *checkDispatchFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	repository, candidate := atomicAdmissionCandidate(t, false)
	var control *Store
	var err error
	if options.configured {
		control, err = OpenConfigured(ctx, path, ControlConfiguration{
			LocalCheckRuntimeManifestDigest: dispatchRuntimeDigest,
			BuilderDispatchDigest:           dispatchDigest,
			Repository:                      repository,
		})
	} else {
		control, err = OpenConfigured(ctx, path, ControlConfiguration{
			BuilderDispatchDigest: dispatchDigest, Repository: repository,
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })

	plan := multiCheckExactPlan(t, control)
	authority, _, _ := authorityFixture(t, control, plan, 1, nil, false, func(source map[string]any) {
		source["repository"] = "repo-01"
		for _, raw := range source["maximum_grants"].([]any) {
			grant := raw.(map[string]any)
			if grant["action"] == "integrate" {
				grant["target"].(map[string]any)["repository"] = "repo-01"
			}
		}
	})
	approval, err := authority.Approve(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	target := plan.Target()
	workIDs := plan.WorkIDs()
	if options.extraStateWork {
		workIDs = append(workIDs, "forged-work")
	}
	create := testCommand(t, "cmd-create", engine.CommandCreate, engine.NoRevision, engine.CreatePayload{
		DeliveryID: plan.DeliveryID(), PlanDigest: plan.Record().Digest,
		Repository: target.Repository, TargetRef: target.Ref, Work: workIDs,
	})
	if result, err := control.Apply(ctx, create); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("create exact delivery = %+v, %v", result, err)
	}
	authorityReceiptDigest := approval.Facts().ReceiptDigest
	if options.missingAuthority {
		authorityReceiptDigest = testLocalCheckDigest("9")
	}
	activate := testCommand(t, "cmd-activate", engine.CommandActivate, 0, engine.ActivatePayload{
		AuthorityReceiptDigest: authorityReceiptDigest,
	})
	if result, err := control.Apply(ctx, activate); err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("activate exact delivery = %+v, %v", result, err)
	}
	contract, _ := plan.Work(plan.WorkIDs()[0])
	build := testCommand(t, "cmd-build", engine.CommandDispatchBuild, 1, engine.DispatchBuildPayload{
		WorkID: plan.WorkIDs()[0], DispatchDigest: contract.Digest(),
		BuilderDispatchDigest: dispatchDigest,
	})
	buildResult, err := control.Apply(ctx, build)
	if err != nil || len(buildResult.EffectIDs) != 1 {
		t.Fatalf("dispatch exact builder = %+v, %v", buildResult, err)
	}
	builderEffectID := buildResult.EffectIDs[0]
	if options.completeBuilder {
		lease, err := control.ClaimNextEffect(ctx, "builder-worker")
		if err != nil || lease.Invocation().ID != builderEffectID {
			t.Fatalf("claim exact builder = %q, %v", lease.Invocation().ID, err)
		}
		encoded := exactBuildResult(t, builderEffectID, candidate, options.builderStartedAt)
		if err := control.BindEffectResult(ctx, lease, encoded); err != nil {
			t.Fatal(err)
		}
		if err := control.CompleteEffect(ctx, lease); err != nil {
			t.Fatal(err)
		}
	}
	selection, err := protocol.ResolveExactLocalChecks(ctx, control, plan, plan.WorkIDs()[0])
	if err != nil {
		t.Fatal(err)
	}
	requirements := selection.Requirements()
	checks := make([]engine.CheckSelection, len(requirements))
	for index, requirement := range requirements {
		checks[index] = engine.CheckSelection{
			CheckID: requirement.CheckID, DefinitionDigest: requirement.Definition.Digest,
		}
	}
	payload := engine.DispatchChecksPayload{
		WorkID: plan.WorkIDs()[0], BuilderEffectID: builderEffectID,
		RuntimeManifestDigest: dispatchRuntimeDigest, Checks: checks,
	}
	command := testCommand(t, "cmd-checks", engine.CommandDispatchChecks, 2, payload)
	return &checkDispatchFixture{
		control: control, plan: plan, builderEffectID: builderEffectID,
		requirements: requirements, command: command,
	}
}

func (fixture *checkDispatchFixture) payload(t *testing.T) engine.DispatchChecksPayload {
	t.Helper()
	var payload engine.DispatchChecksPayload
	if err := json.Unmarshal(fixture.command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func (fixture *checkDispatchFixture) replacePayload(t *testing.T, payload engine.DispatchChecksPayload) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	fixture.command.Payload = encoded
}

func (fixture *checkDispatchFixture) assertBeforeDispatch(t *testing.T) {
	t.Helper()
	state, err := fixture.control.State(context.Background(), "run-1")
	if err != nil || state.Revision != 2 || state.Work[0].State != engine.WorkActive {
		t.Fatalf("state changed after rejected check dispatch: %+v, %v", state, err)
	}
	assertCount(t, fixture.control, "commands", 3)
	assertCount(t, fixture.control, "events", 3)
	assertCount(t, fixture.control, "effects", 1)
}

func multiCheckExactPlan(t *testing.T, control *Store) protocol.ExactPlan {
	t.Helper()
	ctx := context.Background()
	definitions := make([]protocol.Artifact, 2)
	names := []string{"zeta", "alpha"}
	for index, name := range names {
		definition, err := protocol.EncodeCanonical(protocol.LocalCheckDefinition{
			SchemaVersion: protocol.LocalCheckDefinitionSchemaVersion,
			Argv:          []string{"/usr/bin/true", name}, WorkingDirectory: ".", TimeoutSeconds: 10,
			Evidence: protocol.LocalEvidenceDefinition{
				ID: "evidence-" + name, AcceptanceIDs: []string{"AC1"},
				Boundary: "assembled", Observed: "the assembled candidate passed " + name,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		digest, err := control.PutArtifact(ctx, "application/json", definition)
		if err != nil {
			t.Fatal(err)
		}
		definitions[index] = protocol.Artifact{Ref: digest, MediaType: "application/json", Digest: digest}
	}
	checks := make([]any, len(definitions))
	for index, definition := range definitions {
		checks[index] = map[string]any{
			"id": names[index],
			"definition": map[string]any{
				"ref":        "policy/checks/" + names[index] + ".json",
				"media_type": definition.MediaType, "digest": definition.Digest,
			},
		}
	}
	policy, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": protocol.AssurancePolicySchemaVersion,
		"policy_id":      "standard", "checks": checks, "packs": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyDigest, err := control.PutArtifact(ctx, "application/json", policy)
	if err != nil {
		t.Fatal(err)
	}

	base := exampleExactPlan(t)
	var document map[string]any
	if err := json.Unmarshal(base.Record().CanonicalJSON, &document); err != nil {
		t.Fatal(err)
	}
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

func exactBuildResult(
	t *testing.T,
	effectID string,
	candidate repo.Candidate,
	builderStartedAt string,
) json.RawMessage {
	t.Helper()
	if builderStartedAt == "" {
		builderStartedAt = "2026-07-20T00:01:00Z"
	}
	result, err := engine.EncodeBuildEffectResult(engine.BuildEffectResult{
		SchemaVersion: engine.BuildEffectResultSchemaVersion,
		Outcome:       engine.BuildOutcomeCandidateReady,
		Builder: protocol.BuilderRun{
			RunID: effectID, Agent: "sworn-builder/1",
			StartedAt: builderStartedAt, CompletedAt: "2026-07-20T00:01:01Z",
		},
		Candidate: candidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
