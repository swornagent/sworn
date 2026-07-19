package engine

import (
	"encoding/json"
	"reflect"
	"testing"
)

const (
	testPlanDigest      = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testAuthorityDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testDispatchDigest  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
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

func assertRejection(t *testing.T, err error, code string) {
	t.Helper()
	rejection, ok := RejectionOf(err)
	if !ok || rejection.Code != code {
		t.Fatalf("error = %v, want rejection %q", err, code)
	}
}
