package engine

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const (
	testMaximumSafe      = int64(9_007_199_254_740_991)
	testPlanDigest       = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testAuthorityDigest  = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testDispatchDigest   = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	testCheckRuntime     = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	testDefinitionOne    = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	testDefinitionTwo    = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	testSubmissionDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testCandidateCommit  = "2222222222222222222222222222222222222222"
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

func TestAdmissionRequiresStoreFactsAndTransitionsAtomically(t *testing.T) {
	t.Parallel()

	current := stateBeforeAdmission(t)
	before := cloneState(current)
	intent := command(
		t, "cmd-admit", current.RunID, CommandAdmitSubmission, current.Revision,
		AdmitSubmissionPayload{WorkID: "work-1"},
	)
	decision, err := Reduce(&current, intent)
	if !errors.Is(err, ErrAdmissionFactsRequired) || !reflect.DeepEqual(decision, Decision{}) {
		t.Fatalf("intent-only admission = %+v, %v", decision, err)
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatal("intent-only admission mutated its input state")
	}

	facts := AdmissionFacts{
		SubmissionID: "submission-1", SubmissionDigest: testSubmissionDigest,
		CandidateCommit: testCandidateCommit,
	}
	admitted, err := ReduceAdmission(&current, intent, facts)
	if err != nil {
		t.Fatalf("reduce admission: %v", err)
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatal("ReduceAdmission mutated its input state")
	}
	repeated, err := ReduceAdmission(&current, intent, facts)
	if err != nil || !reflect.DeepEqual(repeated, admitted) {
		t.Fatalf("repeated admission decision drifted: %v\nfirst:  %+v\nsecond: %+v", err, admitted, repeated)
	}
	work := admitted.State.Work[0]
	if admitted.State.Revision != current.Revision+1 || admitted.State.Phase != PhaseActive ||
		work.State != WorkReviewable || work.Attempt != current.Work[0].Attempt ||
		work.SubmissionID != facts.SubmissionID || work.SubmissionDigest != facts.SubmissionDigest ||
		work.CandidateCommit != facts.CandidateCommit || work.NextAction != ActionVerify {
		t.Fatalf("unexpected admitted state: %+v", admitted.State)
	}
	if admitted.Event.Kind != "submission.admitted" || len(admitted.Effects) != 0 {
		t.Fatalf("unexpected admission decision: %+v", admitted)
	}
	var event struct {
		WorkID           string `json:"work_id"`
		SubmissionID     string `json:"submission_id"`
		SubmissionDigest string `json:"submission_digest"`
		CandidateCommit  string `json:"candidate_commit"`
	}
	if err := json.Unmarshal(admitted.Event.Data, &event); err != nil || event.WorkID != "work-1" ||
		event.SubmissionID != facts.SubmissionID || event.SubmissionDigest != facts.SubmissionDigest ||
		event.CandidateCommit != facts.CandidateCommit {
		t.Fatalf("admission event = %+v, %v", event, err)
	}
}

func TestAdmissionRejectsMalformedIntentBeforeRequestingFacts(t *testing.T) {
	t.Parallel()

	current := stateBeforeAdmission(t)
	for name, payload := range map[string]json.RawMessage{
		"missing work":    json.RawMessage(`{}`),
		"invalid work":    json.RawMessage(`{"work_id":"bad id"}`),
		"unknown field":   json.RawMessage(`{"work_id":"work-1","submission_digest":"` + testSubmissionDigest + `"}`),
		"duplicate field": json.RawMessage(`{"work_id":"work-1","work_id":"work-1"}`),
	} {
		t.Run(name, func(t *testing.T) {
			before := cloneState(current)
			_, err := Reduce(&current, Command{
				ID: "cmd-admit-invalid", RunID: current.RunID, Kind: CommandAdmitSubmission,
				ExpectedRevision: current.Revision, Payload: payload,
			})
			assertRejection(t, err, "invalid_payload")
			if errors.Is(err, ErrAdmissionFactsRequired) {
				t.Fatal("malformed intent requested derived facts")
			}
			if !reflect.DeepEqual(current, before) {
				t.Fatal("rejected admission intent mutated its input state")
			}
		})
	}

	absent := command(
		t, "cmd-admit-absent", current.RunID, CommandAdmitSubmission, current.Revision,
		AdmitSubmissionPayload{WorkID: "work-absent"},
	)
	_, err := Reduce(&current, absent)
	assertRejection(t, err, "work_not_found")
}

func TestAdmissionRejectsStaleOrMismatchedIntentBeforeRequestingFacts(t *testing.T) {
	t.Parallel()

	current := stateBeforeAdmission(t)
	valid := command(
		t, "cmd-admit-stale", current.RunID, CommandAdmitSubmission, current.Revision,
		AdmitSubmissionPayload{WorkID: "work-1"},
	)
	for name, test := range map[string]struct {
		mutate func(*Command)
		code   string
	}{
		"stale revision": {
			mutate: func(command *Command) { command.ExpectedRevision++ },
			code:   "revision_mismatch",
		},
		"wrong run": {
			mutate: func(command *Command) { command.RunID = "run-other" },
			code:   "run_mismatch",
		},
	} {
		t.Run(name, func(t *testing.T) {
			intent := valid
			test.mutate(&intent)
			_, err := Reduce(&current, intent)
			assertRejection(t, err, test.code)
			if errors.Is(err, ErrAdmissionFactsRequired) {
				t.Fatal("non-current intent requested derived facts")
			}
		})
	}
}

func TestAdmissionRejectsInvalidDerivedFactsWithoutMutation(t *testing.T) {
	t.Parallel()

	current := stateBeforeAdmission(t)
	intent := command(
		t, "cmd-admit-facts", current.RunID, CommandAdmitSubmission, current.Revision,
		AdmitSubmissionPayload{WorkID: "work-1"},
	)
	valid := AdmissionFacts{
		SubmissionID: "submission-1", SubmissionDigest: testSubmissionDigest,
		CandidateCommit: testCandidateCommit,
	}
	tests := map[string]func(*AdmissionFacts){
		"missing submission id": func(value *AdmissionFacts) { value.SubmissionID = "" },
		"invalid submission id": func(value *AdmissionFacts) { value.SubmissionID = "bad id" },
		"invalid digest":        func(value *AdmissionFacts) { value.SubmissionDigest = "sha256:no" },
		"invalid candidate":     func(value *AdmissionFacts) { value.CandidateCommit = "not-an-object" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			facts := valid
			mutate(&facts)
			before := cloneState(current)
			decision, err := ReduceAdmission(&current, intent, facts)
			if err == nil || !reflect.DeepEqual(decision, Decision{}) {
				t.Fatalf("invalid derived facts = %+v, %v", decision, err)
			}
			if !reflect.DeepEqual(current, before) {
				t.Fatal("invalid derived facts mutated input state")
			}
		})
	}
}

func TestAdmissionOnlyTransitionsCheckingWork(t *testing.T) {
	t.Parallel()

	created, ready, active := statesBeforeCheckDispatch(t)
	checking := stateBeforeAdmission(t)
	reviewable := cloneState(checking)
	reviewable.Work[0].State = WorkReviewable
	reviewable.Work[0].SubmissionID = "submission-1"
	reviewable.Work[0].SubmissionDigest = testSubmissionDigest
	reviewable.Work[0].CandidateCommit = testCandidateCommit
	reviewable.Work[0].NextAction = ActionVerify
	facts := AdmissionFacts{
		SubmissionID: "submission-1", SubmissionDigest: testSubmissionDigest,
		CandidateCommit: testCandidateCommit,
	}
	for name, current := range map[string]State{
		"planned": created, "ready": ready, "active": active, "reviewable": reviewable,
	} {
		t.Run(name, func(t *testing.T) {
			intent := command(
				t, "cmd-admit-"+name, current.RunID, CommandAdmitSubmission, current.Revision,
				AdmitSubmissionPayload{WorkID: "work-1"},
			)
			_, err := Reduce(&current, intent)
			assertRejection(t, err, "invalid_transition")
			_, err = ReduceAdmission(&current, intent, facts)
			assertRejection(t, err, "invalid_transition")
		})
	}

	wrongKind := command(
		t, "cmd-not-admit", checking.RunID, CommandDispatchChecks, checking.Revision,
		validCheckDispatchPayload(),
	)
	_, err := ReduceAdmission(&checking, wrongKind, facts)
	assertRejection(t, err, "unsupported_command")
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
	reviewable := cloneState(checking)
	reviewable.Work[0].State = WorkReviewable
	reviewable.Work[0].SubmissionID = "submission-1"
	reviewable.Work[0].SubmissionDigest = testSubmissionDigest
	reviewable.Work[0].CandidateCommit = testCandidateCommit
	reviewable.Work[0].NextAction = ActionVerify
	states := map[string]State{
		"waiting":    created,
		"ready":      ready,
		"active":     active,
		"checking":   checking,
		"reviewable": reviewable,
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

func TestReviewableSubmissionBindingIsStateExclusive(t *testing.T) {
	t.Parallel()

	planned, ready, active := statesBeforeCheckDispatch(t)
	checking := stateBeforeAdmission(t)
	for stateName, current := range map[string]State{
		"waiting": planned, "ready": ready, "active": active, "checking": checking,
	} {
		t.Run(stateName, func(t *testing.T) {
			for fieldName, bind := range map[string]func(*Work){
				"submission id":     func(work *Work) { work.SubmissionID = "submission-1" },
				"submission digest": func(work *Work) { work.SubmissionDigest = testSubmissionDigest },
				"candidate commit":  func(work *Work) { work.CandidateCommit = testCandidateCommit },
			} {
				t.Run(fieldName, func(t *testing.T) {
					bound := cloneState(current)
					bind(&bound.Work[0])
					if err := bound.Validate(); err == nil {
						t.Fatalf("%s work accepted a premature %s", stateName, fieldName)
					}
				})
			}
		})
	}

	reviewable := cloneState(checking)
	reviewable.Work[0].State = WorkReviewable
	reviewable.Work[0].SubmissionID = "submission-1"
	reviewable.Work[0].SubmissionDigest = testSubmissionDigest
	reviewable.Work[0].CandidateCommit = testCandidateCommit
	reviewable.Work[0].NextAction = ActionVerify
	if err := reviewable.Validate(); err != nil {
		t.Fatalf("valid reviewable state: %v", err)
	}
	for name, invalidate := range map[string]func(*Work){
		"missing submission id":     func(work *Work) { work.SubmissionID = "" },
		"missing submission digest": func(work *Work) { work.SubmissionDigest = "" },
		"missing candidate commit":  func(work *Work) { work.CandidateCommit = "" },
		"wrong next action":         func(work *Work) { work.NextAction = ActionWait },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := cloneState(reviewable)
			invalidate(&invalid.Work[0])
			if err := invalid.Validate(); err == nil {
				t.Fatalf("reviewable work accepted %s", name)
			}
		})
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

func stateBeforeAdmission(t *testing.T) State {
	t.Helper()
	active := stateBeforeCheckDispatch(t)
	checking, err := Reduce(&active, command(
		t, "cmd-checks-fixture", active.RunID, CommandDispatchChecks, active.Revision,
		validCheckDispatchPayload(),
	))
	if err != nil {
		t.Fatal(err)
	}
	return checking.State
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
