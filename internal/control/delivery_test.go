package control

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/store"
)

func TestReviewableCommandIDsForIsStableAndDomainSeparated(t *testing.T) {
	t.Parallel()

	first, err := ReviewableCommandIDsFor("run-1", "work-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReviewableCommandIDsFor("run-1", "work-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("stable identities differ: %#v != %#v", first, second)
	}
	if err := first.validate(); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{first.BuildDispatch, first.CheckDispatch, first.Admission} {
		if !engine.ValidID(identity) || !strings.HasPrefix(identity, "cmd-") {
			t.Fatalf("derived command identity %q is invalid", identity)
		}
	}
	next, err := ReviewableCommandIDsFor("run-1", "work-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.BuildDispatch == next.BuildDispatch || first.CheckDispatch == next.CheckDispatch ||
		first.Admission == next.Admission {
		t.Fatalf("attempt did not change identities: %#v -> %#v", first, next)
	}
	if _, err := ReviewableCommandIDsFor("run-1", "work-1", 0); err == nil {
		t.Fatal("zero attempt produced command identities")
	}
}

func TestAdvanceToReviewableConvergesEveryReachablePreAdmissionState(t *testing.T) {
	t.Parallel()

	for _, starting := range []engine.WorkState{
		engine.WorkReady,
		engine.WorkActive,
		engine.WorkChecking,
	} {
		starting := starting
		t.Run(string(starting), func(t *testing.T) {
			t.Parallel()
			fixture := newDeliveryStepsFixture(starting)
			identities, err := ReviewableCommandIDsFor("run-1", "work-1", 1)
			if err != nil {
				t.Fatal(err)
			}
			result, err := advanceToReviewable(
				context.Background(), fixture, "run-1", "work-1", identities,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.State.Work[0].State != engine.WorkReviewable ||
				result.BuildEffectID != "effect-build" ||
				!reflect.DeepEqual(result.CheckEffectIDs, []string{"effect-check-1", "effect-check-2"}) {
				t.Fatalf("reviewable result = %#v", result)
			}
			wantCalls := []string{
				"state", "build:" + identities.BuildDispatch,
				"dispatch-checks:effect-build:" + identities.CheckDispatch,
				"execute-checks:effect-check-1,effect-check-2",
				"admit:" + identities.Admission, "state",
			}
			if !reflect.DeepEqual(fixture.calls, wantCalls) {
				t.Fatalf("calls = %v, want %v", fixture.calls, wantCalls)
			}
		})
	}
}

func TestAdvanceToReviewableIsAlreadyReviewableNoOp(t *testing.T) {
	t.Parallel()

	fixture := newDeliveryStepsFixture(engine.WorkReviewable)
	identities, err := ReviewableCommandIDsFor("run-1", "work-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := advanceToReviewable(
		context.Background(), fixture, "run-1", "work-1", identities,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.State.Work[0].State != engine.WorkReviewable ||
		!reflect.DeepEqual(fixture.calls, []string{"state"}) {
		t.Fatalf("already-reviewable result = %#v, calls = %v", result, fixture.calls)
	}
}

func TestAdvanceToReviewableStopsBeforeLaterEdges(t *testing.T) {
	t.Parallel()

	fixture := newDeliveryStepsFixture(engine.WorkReady)
	fixture.emptyCheckBatch = true
	identities, err := ReviewableCommandIDsFor("run-1", "work-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := advanceToReviewable(
		context.Background(), fixture, "run-1", "work-1", identities,
	)
	if err == nil || !strings.Contains(err.Error(), "applied batch") {
		t.Fatalf("empty check batch error = %v", err)
	}
	if result.BuildEffectID != "effect-build" || len(result.CheckEffectIDs) != 0 {
		t.Fatalf("partial result = %#v", result)
	}
	for _, call := range fixture.calls {
		if strings.HasPrefix(call, "execute-checks") || strings.HasPrefix(call, "admit") {
			t.Fatalf("later edge ran after invalid dispatch: %v", fixture.calls)
		}
	}
}

func TestAdvanceToReviewableRejectsWaitingWorkAndAmbiguousIDs(t *testing.T) {
	t.Parallel()

	identities, err := ReviewableCommandIDsFor("run-1", "work-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newDeliveryStepsFixture(engine.WorkWaiting)
	if _, err := advanceToReviewable(
		context.Background(), fixture, "run-1", "work-1", identities,
	); err == nil || !strings.Contains(err.Error(), "cannot converge") {
		t.Fatalf("waiting work error = %v", err)
	}
	identities.CheckDispatch = identities.BuildDispatch
	if _, err := advanceToReviewable(
		context.Background(), fixture, "run-1", "work-1", identities,
	); err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("duplicate command identity error = %v", err)
	}
}

type deliveryStepsFixture struct {
	current         engine.State
	calls           []string
	emptyCheckBatch bool
}

func newDeliveryStepsFixture(state engine.WorkState) *deliveryStepsFixture {
	attempt := int64(1)
	next := engine.ActionWait
	binding := engine.SubmissionBinding{}
	if state == engine.WorkReady {
		attempt = 0
		next = engine.ActionBuild
	}
	if state == engine.WorkReviewable {
		next = engine.ActionVerify
		binding = engine.SubmissionBinding{
			SubmissionID: "submission-1", SubmissionDigest: testControlDigest("b"),
			CandidateCommit: strings.Repeat("c", 40),
		}
	}
	work := []engine.Work{{
		ID: "work-1", State: state, Attempt: attempt,
		SubmissionBinding: binding, NextAction: next,
	}}
	if state == engine.WorkWaiting {
		work = append(work, engine.Work{
			ID: "work-2", State: engine.WorkReady, NextAction: engine.ActionBuild,
		})
	}
	return &deliveryStepsFixture{current: engine.State{
		SchemaVersion:          engine.StateSchemaVersion,
		RunID:                  "run-1",
		DeliveryID:             "delivery-1",
		PlanDigest:             testControlDigest("a"),
		Repository:             "repo-1",
		TargetRef:              "refs/heads/main",
		Revision:               2,
		Phase:                  engine.PhaseActive,
		AuthorityReceiptDigest: testControlDigest("d"),
		Work:                   work,
	}}
}

func (fixture *deliveryStepsFixture) state(
	_ context.Context,
	runID string,
) (engine.State, error) {
	fixture.calls = append(fixture.calls, "state")
	if runID != fixture.current.RunID {
		return engine.State{}, nil
	}
	return fixture.current, nil
}

func (fixture *deliveryStepsFixture) ensureBuild(
	_ context.Context,
	_, _ string,
	commandID string,
) (store.ApplyResult, error) {
	fixture.calls = append(fixture.calls, "build:"+commandID)
	work := &fixture.current.Work[0]
	if work.State == engine.WorkReady {
		work.State, work.Attempt, work.NextAction = engine.WorkActive, 1, engine.ActionWait
		fixture.current.Revision++
	}
	return store.ApplyResult{
		CommandID: commandID, RunID: fixture.current.RunID,
		Outcome: store.OutcomeApplied, Revision: fixture.current.Revision,
		EffectIDs: []string{"effect-build"},
	}, nil
}

func (fixture *deliveryStepsFixture) dispatchChecks(
	_ context.Context,
	_, _, builderEffectID, commandID string,
) (store.ApplyResult, error) {
	fixture.calls = append(fixture.calls, "dispatch-checks:"+builderEffectID+":"+commandID)
	if fixture.current.Work[0].State == engine.WorkActive {
		fixture.current.Work[0].State = engine.WorkChecking
		fixture.current.Revision++
	}
	effects := []string{"effect-check-1", "effect-check-2"}
	if fixture.emptyCheckBatch {
		effects = nil
	}
	return store.ApplyResult{
		CommandID: commandID, RunID: fixture.current.RunID,
		Outcome: store.OutcomeApplied, Revision: fixture.current.Revision,
		EffectIDs: effects,
	}, nil
}

func (fixture *deliveryStepsFixture) executeChecks(
	_ context.Context,
	_, _ string,
	effectIDs []string,
) error {
	fixture.calls = append(fixture.calls, "execute-checks:"+strings.Join(effectIDs, ","))
	return nil
}

func (fixture *deliveryStepsFixture) admitSubmission(
	_ context.Context,
	_, _ string,
	commandID string,
) (store.ApplyResult, error) {
	fixture.calls = append(fixture.calls, "admit:"+commandID)
	work := &fixture.current.Work[0]
	work.State, work.NextAction = engine.WorkReviewable, engine.ActionVerify
	work.SubmissionBinding = engine.SubmissionBinding{
		SubmissionID: "submission-1", SubmissionDigest: testControlDigest("e"),
		CandidateCommit: strings.Repeat("f", 40),
	}
	fixture.current.Revision++
	return store.ApplyResult{
		CommandID: commandID, RunID: fixture.current.RunID,
		Outcome: store.OutcomeApplied, Revision: fixture.current.Revision,
	}, nil
}

func testControlDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
