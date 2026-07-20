package engine

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const (
	testMaximumSafe     = int64(9_007_199_254_740_991)
	testPlanDigest      = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testAuthorityDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testDispatchDigest  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	testCheckRuntime    = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	testDefinitionOne   = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	testDefinitionTwo   = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
)

func TestReducerWalkingSkeleton(t *testing.T) {
	t.Parallel()

	createCommand := command(t, "cmd-create", "run-1", CommandCreate, NoRevision, CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: testPlanDigest,
		Repository: "sha256:repo-identity",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1", "work-2"},
	})
	created, err := Reduce(nil, createCommand)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.State.Revision != 0 || created.State.Phase != PhasePlanned || len(created.Effects) != 0 {
		t.Fatalf("unexpected created decision: %+v", created)
	}

	before := cloneState(created.State)
	activated, err := Reduce(&created.State, command(t, "cmd-activate", "run-1", CommandActivate, 0, ActivatePayload{
		AuthorityReceiptDigest: testAuthorityDigest,
	}))
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if !reflect.DeepEqual(created.State, before) {
		t.Fatal("Reduce mutated its input state")
	}
	if activated.State.Revision != 1 || activated.State.Work[0].State != WorkReady {
		t.Fatalf("unexpected activated state: %+v", activated.State)
	}

	dispatched, err := Reduce(&activated.State, command(t, "cmd-dispatch", "run-1", CommandDispatchBuild, 1, DispatchBuildPayload{
		WorkID:         "work-1",
		DispatchDigest: testDispatchDigest,
	}))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched.State.Revision != 2 || dispatched.State.Work[0].State != WorkActive || len(dispatched.Effects) != 1 {
		t.Fatalf("unexpected dispatch decision: %+v", dispatched)
	}
	if dispatched.Effects[0].Kind != EffectBuild {
		t.Fatalf("effect kind = %q", dispatched.Effects[0].Kind)
	}
	request, err := ParseBuildEffectRequest(dispatched.Effects[0].Request)
	if err != nil {
		t.Fatalf("parse build effect request: %v", err)
	}
	if request.DeliveryRunID != "run-1" || request.DeliveryID != "delivery-1" || request.WorkID != "work-1" ||
		request.WorkAttempt != 1 || request.DispatchDigest != testDispatchDigest {
		t.Fatalf("unexpected build effect request: %+v", request)
	}
	if strings.Contains(string(dispatched.Effects[0].Request), `"run_id"`) {
		t.Fatal("build request exposes an ambiguous or caller-selected invocation run id")
	}
}

func TestBuildEffectRequestIsStrictAndVersioned(t *testing.T) {
	t.Parallel()

	valid := json.RawMessage(`{"schema_version":"sworn-build-effect-request-v1","delivery_run_id":"delivery-run","delivery_id":"delivery-1","work_id":"work-1","work_attempt":2,"dispatch_digest":"` + testDispatchDigest + `"}`)
	request, err := ParseBuildEffectRequest(valid)
	if err != nil || request.WorkAttempt != 2 {
		t.Fatalf("valid request = %+v, %v", request, err)
	}
	for _, invalid := range []json.RawMessage{
		json.RawMessage(`{"schema_version":"sworn-build-effect-request-v2","delivery_run_id":"delivery-run","delivery_id":"delivery-1","work_id":"work-1","work_attempt":2,"dispatch_digest":"` + testDispatchDigest + `"}`),
		json.RawMessage(`{"schema_version":"sworn-build-effect-request-v1","delivery_run_id":"delivery-run","delivery_id":"delivery-1","work_id":"work-1","work_attempt":0,"dispatch_digest":"` + testDispatchDigest + `"}`),
		json.RawMessage(`{"schema_version":"sworn-build-effect-request-v1","delivery_run_id":"delivery-run","delivery_id":"delivery-1","work_id":"work-1","work_attempt":2,"dispatch_digest":"` + testDispatchDigest + `","builder_run_id":"caller-chosen"}`),
	} {
		if _, err := ParseBuildEffectRequest(invalid); err == nil {
			t.Fatalf("invalid build effect request was accepted: %s", invalid)
		}
	}
}

func TestCheckDispatchEmitsOrderedBatchAndMovesWorkToChecking(t *testing.T) {
	t.Parallel()

	current := stateBeforeCheckDispatch(t)
	before := cloneState(current)
	payload := validCheckDispatchPayload()
	dispatch := command(t, "cmd-checks", current.RunID, CommandDispatchChecks, current.Revision, payload)
	decision, err := Reduce(&current, dispatch)
	if err != nil {
		t.Fatalf("dispatch checks: %v", err)
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatal("check dispatch mutated its input state")
	}
	repeated, err := Reduce(&current, dispatch)
	if err != nil || !reflect.DeepEqual(repeated, decision) {
		t.Fatalf("repeated check dispatch decision drifted: %v\nfirst:  %+v\nsecond: %+v", err, decision, repeated)
	}
	if decision.State.Revision != current.Revision+1 || decision.State.Phase != PhaseActive ||
		decision.State.Work[0].State != WorkChecking || decision.State.Work[0].Attempt != current.Work[0].Attempt ||
		decision.State.Work[0].NextAction != ActionWait {
		t.Fatalf("unexpected check-dispatch state: %+v", decision.State)
	}
	if decision.Event.Kind != "checks.dispatched" || string(decision.Event.Data) != string(dispatch.Payload) {
		t.Fatalf("unexpected check-dispatch event: %+v", decision.Event)
	}
	if len(decision.Effects) != len(payload.Checks) {
		t.Fatalf("effect count = %d, want %d", len(decision.Effects), len(payload.Checks))
	}
	for index, effect := range decision.Effects {
		if effect.Kind != EffectLocalCheck {
			t.Fatalf("effect %d kind = %q", index, effect.Kind)
		}
		request, err := ParseLocalCheckEffectRequest(effect.Request)
		if err != nil {
			t.Fatalf("parse effect %d request: %v", index, err)
		}
		selection := payload.Checks[index]
		if request.DeliveryRunID != current.RunID || request.DeliveryID != current.DeliveryID ||
			request.WorkID != payload.WorkID || request.WorkAttempt != current.Work[0].Attempt ||
			request.BuilderEffectID != payload.BuilderEffectID || request.CheckID != selection.CheckID ||
			request.DefinitionDigest != selection.DefinitionDigest ||
			request.RuntimeManifestDigest != payload.RuntimeManifestDigest {
			t.Fatalf("effect %d request = %+v", index, request)
		}
	}
}

func TestCheckDispatchFanoutBoundary(t *testing.T) {
	t.Parallel()

	current := stateBeforeCheckDispatch(t)
	payload := validCheckDispatchPayload()
	payload.Checks = make([]CheckSelection, MaximumCheckFanout)
	for index := range payload.Checks {
		payload.Checks[index] = CheckSelection{
			CheckID: "check-" + strconv.Itoa(index), DefinitionDigest: testDefinitionOne,
		}
	}
	decision, err := Reduce(&current, command(
		t, "cmd-checks-max", current.RunID, CommandDispatchChecks, current.Revision, payload,
	))
	if err != nil || len(decision.Effects) != MaximumCheckFanout {
		t.Fatalf("maximum fanout = %d effects, %v", len(decision.Effects), err)
	}

	payload.Checks = append(payload.Checks, CheckSelection{
		CheckID: "check-overflow", DefinitionDigest: testDefinitionOne,
	})
	_, err = Reduce(&current, command(
		t, "cmd-checks-overflow", current.RunID, CommandDispatchChecks, current.Revision, payload,
	))
	assertRejection(t, err, "invalid_payload")
}

func TestReducerPreservesNonFirstWorkAttemptAcrossBuildAndChecks(t *testing.T) {
	t.Parallel()

	_, ready, _ := statesBeforeCheckDispatch(t)
	ready.Work[0].Attempt = 6
	built, err := Reduce(&ready, command(
		t, "cmd-build-later-attempt", ready.RunID, CommandDispatchBuild, ready.Revision,
		DispatchBuildPayload{WorkID: "work-1", DispatchDigest: testDispatchDigest},
	))
	if err != nil {
		t.Fatalf("dispatch later build attempt: %v", err)
	}
	buildRequest, err := ParseBuildEffectRequest(built.Effects[0].Request)
	if err != nil || built.State.Work[0].Attempt != 7 || buildRequest.WorkAttempt != 7 {
		t.Fatalf("later build attempt = state %d, request %d, %v", built.State.Work[0].Attempt, buildRequest.WorkAttempt, err)
	}
	checked, err := Reduce(&built.State, command(
		t, "cmd-checks-later-attempt", built.State.RunID, CommandDispatchChecks, built.State.Revision,
		validCheckDispatchPayload(),
	))
	if err != nil {
		t.Fatalf("dispatch later check attempt: %v", err)
	}
	request, err := ParseLocalCheckEffectRequest(checked.Effects[0].Request)
	if err != nil || checked.State.Work[0].Attempt != 7 || request.WorkAttempt != 7 {
		t.Fatalf("later check attempt = state %d, request %d, %v", checked.State.Work[0].Attempt, request.WorkAttempt, err)
	}
}

func TestWorkAttemptSafeIntegerBoundary(t *testing.T) {
	t.Parallel()

	created, ready, active := statesBeforeCheckDispatch(t)
	checking := cloneState(active)
	checking.Work[0].State = WorkChecking
	states := map[string]State{
		"waiting":  created,
		"ready":    ready,
		"active":   active,
		"checking": checking,
	}
	for name, state := range states {
		state.Work[0].Attempt = testMaximumSafe
		if err := state.Validate(); err != nil {
			t.Fatalf("%s state rejected maximum safe attempt: %v", name, err)
		}
		state.Work[0].Attempt++
		if err := state.Validate(); err == nil {
			t.Fatalf("%s state accepted an inexact attempt", name)
		}
	}

	ready.Work[0].Attempt = testMaximumSafe
	_, err := Reduce(&ready, command(
		t, "cmd-build-attempt-overflow", ready.RunID, CommandDispatchBuild, ready.Revision,
		DispatchBuildPayload{WorkID: "work-1", DispatchDigest: testDispatchDigest},
	))
	assertRejection(t, err, "invalid_transition")

	active.Work[0].Attempt = testMaximumSafe
	decision, err := Reduce(&active, command(
		t, "cmd-checks-maximum-attempt", active.RunID, CommandDispatchChecks, active.Revision,
		validCheckDispatchPayload(),
	))
	if err != nil {
		t.Fatalf("dispatch checks at maximum safe attempt: %v", err)
	}
	request, err := ParseLocalCheckEffectRequest(decision.Effects[0].Request)
	if err != nil || request.WorkAttempt != testMaximumSafe || decision.State.Work[0].Attempt != testMaximumSafe {
		t.Fatalf("maximum check attempt = state %d, request %d, %v", decision.State.Work[0].Attempt, request.WorkAttempt, err)
	}
}

func TestCheckDispatchRejectsInvalidOrCallerExtendedSelection(t *testing.T) {
	t.Parallel()

	current := stateBeforeCheckDispatch(t)
	tests := []struct {
		name   string
		mutate func(*DispatchChecksPayload)
	}{
		{name: "invalid work", mutate: func(value *DispatchChecksPayload) { value.WorkID = "bad id" }},
		{name: "invalid builder", mutate: func(value *DispatchChecksPayload) { value.BuilderEffectID = "bad id" }},
		{name: "invalid runtime", mutate: func(value *DispatchChecksPayload) { value.RuntimeManifestDigest = "sha256:no" }},
		{name: "empty checks", mutate: func(value *DispatchChecksPayload) { value.Checks = nil }},
		{name: "invalid check id", mutate: func(value *DispatchChecksPayload) { value.Checks[0].CheckID = "bad id" }},
		{name: "invalid definition", mutate: func(value *DispatchChecksPayload) { value.Checks[0].DefinitionDigest = "sha256:no" }},
		{name: "duplicate check", mutate: func(value *DispatchChecksPayload) { value.Checks[1].CheckID = value.Checks[0].CheckID }},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := validCheckDispatchPayload()
			test.mutate(&payload)
			before := cloneState(current)
			_, err := Reduce(&current, command(
				t, "cmd-invalid-"+strconv.Itoa(index), current.RunID,
				CommandDispatchChecks, current.Revision, payload,
			))
			assertRejection(t, err, "invalid_payload")
			if !reflect.DeepEqual(current, before) {
				t.Fatal("rejected check dispatch mutated its input state")
			}
		})
	}

	encoded, err := json.Marshal(validCheckDispatchPayload())
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := append(append([]byte(nil), encoded[:len(encoded)-1]...), []byte(`,"surprise":true}`)...)
	_, err = Reduce(&current, Command{
		ID: "cmd-unknown", RunID: current.RunID, Kind: CommandDispatchChecks,
		ExpectedRevision: current.Revision, Payload: withUnknown,
	})
	assertRejection(t, err, "invalid_payload")

	duplicateName := json.RawMessage(`{"work_id":"work-1","work_id":"work-1","builder_effect_id":"builder-effect-1","runtime_manifest_digest":"` +
		testCheckRuntime + `","checks":[{"check_id":"test","definition_digest":"` + testDefinitionOne + `"}]}`)
	_, err = Reduce(&current, Command{
		ID: "cmd-duplicate-name", RunID: current.RunID, Kind: CommandDispatchChecks,
		ExpectedRevision: current.Revision, Payload: duplicateName,
	})
	assertRejection(t, err, "invalid_payload")
}

func TestCheckDispatchRejectsWrongWorkState(t *testing.T) {
	t.Parallel()

	payload := validCheckDispatchPayload()
	created, activated, active := statesBeforeCheckDispatch(t)
	for name, current := range map[string]State{
		"planned":  created,
		"ready":    activated,
		"checking": func() State { value := active; value.Work[0].State = WorkChecking; return value }(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Reduce(&current, command(
				t, "cmd-wrong-"+name, current.RunID, CommandDispatchChecks, current.Revision, payload,
			))
			assertRejection(t, err, "invalid_transition")
		})
	}
	payload.WorkID = "work-absent"
	_, err := Reduce(&active, command(
		t, "cmd-work-absent", active.RunID, CommandDispatchChecks, active.Revision, payload,
	))
	assertRejection(t, err, "work_not_found")
}

func TestBuildDispatchPayloadCannotChooseProtocolRunID(t *testing.T) {
	t.Parallel()

	created, err := Reduce(nil, command(t, "cmd-create", "run-1", CommandCreate, NoRevision, CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: testPlanDigest,
		Repository: "repo",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	activated, err := Reduce(&created.State, command(t, "cmd-activate", "run-1", CommandActivate, 0, ActivatePayload{
		AuthorityReceiptDigest: testAuthorityDigest,
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Reduce(&activated.State, Command{
		ID:               "cmd-dispatch",
		RunID:            "run-1",
		Kind:             CommandDispatchBuild,
		ExpectedRevision: 1,
		Payload: json.RawMessage(`{"work_id":"work-1","dispatch_digest":"` + testDispatchDigest +
			`","builder_run_id":"caller-chosen"}`),
	})
	assertRejection(t, err, "invalid_payload")
}

func TestReducerRejectsWrongRevisionAndUnknownFields(t *testing.T) {
	t.Parallel()

	created, err := Reduce(nil, command(t, "cmd-create", "run-1", CommandCreate, NoRevision, CreatePayload{
		DeliveryID: "delivery-1",
		PlanDigest: testPlanDigest,
		Repository: "repo",
		TargetRef:  "refs/heads/main",
		Work:       []string{"work-1"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Reduce(&created.State, command(t, "cmd-activate", "run-1", CommandActivate, 9, ActivatePayload{
		AuthorityReceiptDigest: testAuthorityDigest,
	}))
	assertRejection(t, err, "revision_mismatch")

	_, err = Reduce(&created.State, Command{
		ID:               "cmd-activate-2",
		RunID:            "run-1",
		Kind:             CommandActivate,
		ExpectedRevision: 0,
		Payload:          json.RawMessage(`{"authority_receipt_digest":"` + testAuthorityDigest + `","surprise":true}`),
	})
	assertRejection(t, err, "invalid_payload")
}

func command(t *testing.T, id, runID string, kind CommandKind, revision int64, payload any) Command {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return Command{ID: id, RunID: runID, Kind: kind, ExpectedRevision: revision, Payload: encoded}
}

func validCheckDispatchPayload() DispatchChecksPayload {
	return DispatchChecksPayload{
		WorkID: "work-1", BuilderEffectID: "builder-effect-1",
		RuntimeManifestDigest: testCheckRuntime,
		Checks: []CheckSelection{
			{CheckID: "lint", DefinitionDigest: testDefinitionOne},
			{CheckID: "test", DefinitionDigest: testDefinitionTwo},
		},
	}
}

func stateBeforeCheckDispatch(t *testing.T) State {
	t.Helper()
	_, _, active := statesBeforeCheckDispatch(t)
	return active
}

func statesBeforeCheckDispatch(t *testing.T) (State, State, State) {
	t.Helper()
	created, err := Reduce(nil, command(t, "cmd-create-fixture", "run-1", CommandCreate, NoRevision, CreatePayload{
		DeliveryID: "delivery-1", PlanDigest: testPlanDigest, Repository: "repo",
		TargetRef: "refs/heads/main", Work: []string{"work-1"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	activated, err := Reduce(&created.State, command(
		t, "cmd-activate-fixture", "run-1", CommandActivate, created.State.Revision,
		ActivatePayload{AuthorityReceiptDigest: testAuthorityDigest},
	))
	if err != nil {
		t.Fatal(err)
	}
	active, err := Reduce(&activated.State, command(
		t, "cmd-build-fixture", "run-1", CommandDispatchBuild, activated.State.Revision,
		DispatchBuildPayload{WorkID: "work-1", DispatchDigest: testDispatchDigest},
	))
	if err != nil {
		t.Fatal(err)
	}
	return created.State, activated.State, active.State
}

func assertRejection(t *testing.T, err error, code string) {
	t.Helper()
	rejection, ok := RejectionOf(err)
	if !ok || rejection.Code != code {
		t.Fatalf("error = %v, want rejection %q", err, code)
	}
}
