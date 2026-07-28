package cockpit

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

type fakeJournal struct {
	binding      journal.Run
	observations []journal.Observation
	window       journal.EventWindow
	calls        *[]string
}

func (f *fakeJournal) RunBinding(
	context.Context,
	string,
) (journal.Run, error) {
	*f.calls = append(*f.calls, "binding")
	return f.binding, nil
}

func (f *fakeJournal) ReadObservation(
	context.Context,
	string,
	int,
	int,
) (journal.Observation, error) {
	*f.calls = append(*f.calls, "observation")
	value := f.observations[0]
	if len(f.observations) > 1 {
		f.observations = f.observations[1:]
	}
	return value, nil
}

func (f *fakeJournal) EventsAfter(
	context.Context,
	string,
	int64,
	int,
) (journal.EventWindow, error) {
	*f.calls = append(*f.calls, "events")
	return f.window, nil
}

type fakeRuntime struct {
	statuses []runtimepkg.RunStatus
	calls    *[]string
}

func (f *fakeRuntime) Status(
	context.Context,
	string,
) (runtimepkg.RunStatus, error) {
	*f.calls = append(*f.calls, "status")
	value := f.statuses[0]
	if len(f.statuses) > 1 {
		f.statuses = f.statuses[1:]
	}
	return value, nil
}

type fakeStateReader struct {
	states []baton.State
	errs   []error
	calls  *[]string
}

func (f *fakeStateReader) Read(
	context.Context,
	journal.Run,
) (baton.State, error) {
	*f.calls = append(*f.calls, "state")
	value := f.states[0]
	err := f.errs[0]
	if len(f.states) > 1 {
		f.states = f.states[1:]
	}
	if len(f.errs) > 1 {
		f.errs = f.errs[1:]
	}
	return value, err
}

func projectionFixture() (
	journal.Run,
	journal.Observation,
	runtimepkg.RunStatus,
	baton.State,
) {
	now := time.Unix(1_700_100_000, 0).UTC()
	run := journal.Run{
		ID: "run-1", ManifestDigest: "sha256:" + strings.Repeat("a", 64),
		Repository: "/repository", Release: "release-1",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	observation := journal.Observation{
		Run: run,
		Control: journal.ControlProjection{
			Generation: 4,
			Desired:    "running",
			RetryEpochs: map[string]int64{
				"sha256:" + strings.Repeat("b", 64): 2,
			},
		},
		Owner: journal.OwnerLease{
			RunID: run.ID, Token: strings.Repeat("c", 64),
			Generation: 2, ExpiresAt: now.Add(time.Minute),
		},
		OwnerPresent: true,
		Attempts: []journal.AttemptFact{
			{
				EffectID: "effect-1", Number: 1,
				Responsibility: "work_verification",
				Transport:      "completed",
				Usage: []byte(
					`{"token_status":"reported","input_tokens":0,"output_tokens":0,` +
						`"cost_status":"reported","cost_micro_units":0,` +
						`"currency":"USD","source":"provider_reported"}`,
				),
				CreatedAt: now.Add(time.Second),
			},
			{
				EffectID: "effect-2", Number: 1,
				Responsibility: "captain_review",
				Transport:      "completed",
				Usage: []byte(
					`{"token_status":"unavailable","input_tokens":null,` +
						`"output_tokens":null,"cost_status":"unavailable",` +
						`"cost_micro_units":null,"currency":null,"source":null}`,
				),
				CreatedAt: now.Add(2 * time.Second),
			},
		},
		Events: []journal.EventFact{{
			Offset: 7, Kind: "dispatch_completed",
			CreatedAt: now.Add(3 * time.Second),
		}},
		Notifications: []journal.NotificationFact{{
			DestinationID: "primary", SourceEventOffset: 7,
			Sequence: 1, MessageID: "message-1",
			State: journal.NotificationDead, Attempts: 3,
			AvailableAt: now, LastErrorCode: "WEBHOOK_HTTP_REJECTED",
			CreatedAt: now, UpdatedAt: now.Add(4 * time.Second),
		}},
		EventOffset: 7,
	}
	status := runtimepkg.RunStatus{
		SchemaVersion: "sworn.run-status/v2",
		RunID:         run.ID,
		State:         "parked", DesiredState: "running",
		ControlGeneration: 4,
		ManifestDigest:    run.ManifestDigest,
		PlanDigest:        "sha256:" + strings.Repeat("d", 64),
		TargetRef:         run.TargetRef,
		TargetHead:        strings.Repeat("1", 40),
		ReleaseHead:       strings.Repeat("2", 40),
		Effects: []runtimepkg.EffectStatus{{
			ID:   "attempt/" + strings.Repeat("b", 64) + "/e2/t3",
			Kind: "driver.dispatch", State: string(journal.OperationalFailed),
			ErrorCode: "transport_error",
		}},
		EventOffset: 7,
	}
	product := "sha256:" + strings.Repeat("e", 64)
	sliceOne := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: baton.Track{ID: "T1"},
			Slice: baton.Slice{ID: "S1"},
		},
		Stage: "implement", Status: "ready", NextRole: "implementer",
		Outcome: "none", Attempt: 2,
	}
	sliceTwo := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: baton.Track{ID: "T2", DependsOn: []string{"T1"}},
			Slice: baton.Slice{
				ID: "S2", DependsOn: []string{"S1"}, Consumes: []string{"S1"},
			},
		},
		Stage: "design", Status: "waiting", NextRole: "none",
		Outcome: "none", Attempt: 1,
	}
	state := baton.State{
		Release: run.Release, Repository: run.Repository,
		Plan: baton.PlanState{
			Digest: status.PlanDigest,
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref:  "refs/heads/release-wt/release-1",
				Head: status.ReleaseHead, State: "direct",
			},
			Target: baton.CapturedRef{
				Ref: run.TargetRef, Head: status.TargetHead, State: "direct",
			},
			Tracks: []baton.TrackRefState{
				{ID: "T1", CapturedRef: baton.CapturedRef{
					Ref:  "refs/heads/track/release-1/T1",
					Head: strings.Repeat("3", 40), State: "direct",
				}},
				{ID: "T2", CapturedRef: baton.CapturedRef{
					Ref:  "refs/heads/track/release-1/T2",
					Head: strings.Repeat("4", 40), State: "direct",
				}},
			},
		},
		Tracks: []baton.TrackState{
			{ID: "T1", Slices: []*baton.SliceState{sliceOne}},
			{
				ID: "T2", DependsOn: []string{"T1"},
				Slices: []*baton.SliceState{sliceTwo},
			},
		},
		Slices: []*baton.SliceState{sliceOne, sliceTwo},
		Assembly: baton.AssemblyState{
			InputPins: map[string]*string{"T1": &product, "T2": nil},
			Stage:     "verify", Status: "waiting", NextRole: "none", Outcome: "none",
		},
	}
	return run, observation, status, state
}

func TestProjectorBuildsOneStableTruthfulGraph(t *testing.T) {
	t.Parallel()

	run, observation, status, state := projectionFixture()
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{
			binding: run, observations: []journal.Observation{observation},
			calls: &calls,
		},
		&fakeRuntime{
			statuses: []runtimepkg.RunStatus{status, status},
			calls:    &calls,
		},
		&fakeStateReader{
			states: []baton.State{state, state},
			errs:   []error{nil, nil},
			calls:  &calls,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"binding", "state", "status", "observation", "status", "state",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("projection order = %#v, want %#v", calls, wantCalls)
	}
	wantNodes := []string{
		"release:release-1", "track:T1", "slice:S1",
		"track:T2", "slice:S2", "assembly:release-1",
	}
	var gotNodes []string
	for _, node := range snapshot.Graph.Nodes {
		gotNodes = append(gotNodes, node.ID)
	}
	if !reflect.DeepEqual(gotNodes, wantNodes) {
		t.Fatalf("nodes = %#v, want %#v", gotNodes, wantNodes)
	}
	for _, expected := range []string{
		"edge:depends_on:track:T1:track:T2",
		"edge:depends_on:slice:S1:slice:S2",
		"edge:consumes:slice:S1:slice:S2",
		"edge:assembly_input:slice:S1:assembly:release-1",
		"edge:assembly_input:slice:S2:assembly:release-1",
	} {
		if !hasEdge(snapshot.Graph.Edges, expected) {
			t.Errorf("missing edge %s in %#v", expected, snapshot.Graph.Edges)
		}
	}
	if !snapshot.Graph.Nodes[2].HasBaton ||
		snapshot.Graph.Nodes[2].NextResponsibility != "implementer" {
		t.Fatalf("exact handoff = %#v", snapshot.Graph.Nodes[2])
	}
	if !snapshot.Handoff.Ready ||
		!reflect.DeepEqual(snapshot.Handoff.Nodes, []string{"slice:S1"}) ||
		!reflect.DeepEqual(
			snapshot.Handoff.Responsibilities,
			[]string{"implementer"},
		) {
		t.Fatalf("handoff ribbon = %#v", snapshot.Handoff)
	}
	if snapshot.ThroughOffset != observation.EventOffset ||
		len(snapshot.Evidence) != 1 ||
		snapshot.Evidence[0].Kind != "dispatch_completed" {
		t.Fatalf("evidence = %#v", snapshot)
	}
	if snapshot.Runtime.Attempts[0].InputTokens == nil ||
		*snapshot.Runtime.Attempts[0].InputTokens != 0 ||
		snapshot.Runtime.Attempts[0].CostMicroUnits == nil ||
		*snapshot.Runtime.Attempts[0].CostMicroUnits != 0 {
		t.Fatalf("reported zero was lost: %#v", snapshot.Runtime.Attempts[0])
	}
	if snapshot.Runtime.Attempts[1].InputTokens != nil ||
		snapshot.Runtime.Attempts[1].CostMicroUnits != nil {
		t.Fatalf("unknown usage became zero: %#v", snapshot.Runtime.Attempts[1])
	}
	if !hasAction(snapshot.Actions, "retry") ||
		!hasAction(snapshot.Actions, "pause") ||
		!hasAction(snapshot.Actions, "cancel") ||
		!hasAction(snapshot.Actions, "redeliver") ||
		len(snapshot.Runtime.Notifications) != 1 ||
		snapshot.Runtime.Notifications[0].MessageID != "message-1" {
		t.Fatalf("safe actions = %#v", snapshot.Actions)
	}
}

func TestProjectorFailsClosedWhenObservationNeverStabilizes(t *testing.T) {
	t.Parallel()

	run, observation, first, state := projectionFixture()
	second := first
	second.EventOffset++
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{
			binding: run,
			observations: []journal.Observation{
				observation,
				observation,
			},
			calls: &calls,
		},
		&fakeRuntime{
			statuses: []runtimepkg.RunStatus{
				first, second, first, second,
			},
			calls: &calls,
		},
		&fakeStateReader{
			states: []baton.State{state, state, state, state},
			errs:   []error{nil, nil, nil, nil},
			calls:  &calls,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projector.Snapshot(
		context.Background(),
		run.ID,
	); !IsCode(err, "SNAPSHOT_UNSTABLE") {
		t.Fatalf("unstable projection = %v", err)
	}
}

func TestProjectorDegradesWithoutInventingBatonProgressOrControls(t *testing.T) {
	t.Parallel()

	run, observation, status, state := projectionFixture()
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{
			binding: run, observations: []journal.Observation{observation},
			calls: &calls,
		},
		&fakeRuntime{
			statuses: []runtimepkg.RunStatus{status, status},
			calls:    &calls,
		},
		&fakeStateReader{
			states: []baton.State{state, state},
			errs: []error{
				errors.New("missing"),
				errors.New("missing"),
			},
			calls: &calls,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Graph.Nodes) != 1 ||
		snapshot.Graph.Nodes[0].ID != "release:release-1" ||
		len(snapshot.Graph.Edges) != 0 ||
		len(snapshot.Actions) != 0 ||
		len(snapshot.Diagnostics) != 1 ||
		snapshot.Diagnostics[0].Code != "BATON_UNAVAILABLE" {
		t.Fatalf("degraded snapshot = %#v", snapshot)
	}
}

func TestProjectorEventsExcludeBodies(t *testing.T) {
	t.Parallel()

	var calls []string
	projector, err := NewProjector(
		&fakeJournal{
			calls: &calls,
			window: journal.EventWindow{
				Events: []journal.EventFact{{
					Offset: 4, Kind: "effect_completed",
					CreatedAt: time.Unix(1_700_000_000, 0).UTC(),
				}},
				Through: 4, EventOffset: 5, HasMore: true,
			},
		},
		&fakeRuntime{calls: &calls},
		&fakeStateReader{calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := projector.Events(context.Background(), "run-1", 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.ThroughOffset != 4 || page.EventOffset != 5 ||
		!page.HasMore || len(page.Events) != 1 ||
		page.Events[0].Kind != "effect_completed" {
		t.Fatalf("event page = %#v", page)
	}
}

func hasEdge(edges []Edge, id string) bool {
	for _, edge := range edges {
		if edge.ID == id {
			return true
		}
	}
	return false
}

func hasAction(actions []Action, kind string) bool {
	for _, action := range actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}
