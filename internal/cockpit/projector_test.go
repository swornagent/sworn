package cockpit

import (
	"context"
	"encoding/json"
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
		Attentions: []journal.AttentionProjection{func() journal.AttentionProjection {
			recovery := journal.RecoveryBinding{
				LaneID:     "T1",
				CycleID:    "sha256:" + strings.Repeat("5", 64),
				TurnID:     "sha256:" + strings.Repeat("6", 64),
				ProgressID: "sha256:" + strings.Repeat("7", 64),
			}
			return journal.AttentionProjection{
				Attention: journal.AttentionBinding{
					ID:       journal.AttentionID(recovery, 1),
					Ordinal:  1,
					Recovery: recovery,
				},
				Generation: 1,
				State:      journal.AttentionOpen,
				Question:   "Which approved path should this lane use?",
			}
		}()},
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
		SchemaVersion: "sworn.run-status/v3",
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
	status.ExternalAuthorizer = "external-authorizer"
	status.CaptainDelegation = &runtimepkg.CaptainDelegationView{Digest: "sha256:" + strings.Repeat("a", 64), Epoch: 2, State: "active", Decisions: 3, ReplanSpent: 1, ReplanBudget: 4}
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
	if snapshot.CaptainDelegation == nil || !reflect.DeepEqual(snapshot.CaptainDelegation, status.CaptainDelegation) {
		t.Fatalf("captain delegation = %#v", snapshot.CaptainDelegation)
	}
	var authorityActions []Action
	for _, action := range snapshot.Actions {
		if strings.HasPrefix(action.Kind, "captain_delegation_") {
			authorityActions = append(authorityActions, action)
		}
	}
	if len(authorityActions) != 2 {
		t.Fatalf("Captain authority actions = %#v", authorityActions)
	}
	for _, action := range authorityActions {
		binding := action.CaptainDelegation
		if binding == nil || binding.RunID != status.RunID ||
			binding.ManifestDigest != status.ManifestDigest ||
			binding.ActorClass != runtimepkg.CaptainDelegationActorClass ||
			binding.ActorAuthority != status.ExternalAuthorizer ||
			binding.CurrentEpoch != 2 || binding.CurrentDigest != status.CaptainDelegation.Digest {
			t.Fatalf("Captain authority binding = %#v", action)
		}
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
	if snapshot.Graph.Nodes[1].State != "present" ||
		snapshot.Graph.Nodes[1].RuntimeState != "parked" ||
		snapshot.Graph.Nodes[2].State != "ready" ||
		snapshot.Graph.Nodes[2].RuntimeState != "parked" ||
		!snapshot.Graph.Nodes[2].HasBaton ||
		snapshot.Graph.Nodes[2].NextResponsibility != "implementer" {
		t.Fatalf("lane-local park = %#v", snapshot.Graph.Nodes[:3])
	}
	if !snapshot.Handoff.Ready ||
		!reflect.DeepEqual(
			snapshot.Handoff.Nodes,
			[]string{"slice:S1"},
		) ||
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
		!hasAction(snapshot.Actions, "answer_attention") ||
		!hasAction(snapshot.Actions, "redeliver") ||
		len(snapshot.Runtime.Attentions) != 1 ||
		snapshot.Runtime.Attentions[0].LaneID != "T1" ||
		snapshot.Runtime.Attentions[0].Question !=
			"Which approved path should this lane use?" ||
		len(snapshot.Runtime.Notifications) != 1 ||
		snapshot.Runtime.Notifications[0].MessageID != "message-1" {
		t.Fatalf("safe actions = %#v", snapshot.Actions)
	}
}

func TestStableObservationSeparatesPlanSlugFromLocalCheckout(t *testing.T) {
	t.Parallel()

	run, observation, status, state := projectionFixture()
	state.Repository = "acme/repo"
	if !stableObservation(
		run,
		observation,
		status,
		status,
		state,
		nil,
		state,
		nil,
	) {
		t.Fatal("plan repository slug was compared with local run path")
	}
	changed := state
	changed.Refs.Target.Head = strings.Repeat("9", 40)
	if stableObservation(
		run,
		observation,
		status,
		status,
		state,
		nil,
		changed,
		nil,
	) {
		t.Fatal("ref-vector drift was accepted")
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

func TestProjectorEvidenceAssociationDisambiguationAndLegacyCleanProjection(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()

	// 1. New-format event association JSON
	assocBytes := runtimepkg.MarshalAssociation(runtimepkg.EventAssociation{
		EffectID: "effect-100",
		WorkID:   "work-100",
		Track:    "T1",
		Slice:    "S1",
	})
	newFormatEv := projectEvidence(journal.EventFact{
		Offset:    1,
		Kind:      "dispatch_completed",
		SafeBody:  assocBytes,
		CreatedAt: now,
	})
	if newFormatEv.EffectID != "effect-100" || newFormatEv.WorkID != "work-100" ||
		newFormatEv.Track != "T1" || newFormatEv.Slice != "S1" {
		t.Fatalf("new format evidence = %#v", newFormatEv)
	}
	jsonBytes, err := json.Marshal(newFormatEv)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"effect_id":"effect-100"`, `"work_id":"work-100"`, `"track":"T1"`, `"slice":"S1"`} {
		if !strings.Contains(string(jsonBytes), expected) {
			t.Fatalf("marshaled JSON missing %s: %s", expected, string(jsonBytes))
		}
	}

	// 2. Legacy string body
	legacyStringEv := projectEvidence(journal.EventFact{
		Offset:    2,
		Kind:      "dispatch_operational_failure",
		SafeBody:  []byte("work_verification"),
		CreatedAt: now,
	})
	if legacyStringEv.EffectID != "" || legacyStringEv.WorkID != "" ||
		legacyStringEv.Track != "" || legacyStringEv.Slice != "" {
		t.Fatalf("legacy string body populated association: %#v", legacyStringEv)
	}
	legacyJSON, err := json.Marshal(legacyStringEv)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"effect_id", "work_id", "track", "slice"} {
		if strings.Contains(string(legacyJSON), forbidden) {
			t.Fatalf("legacy string body JSON leaked %s: %s", forbidden, string(legacyJSON))
		}
	}

	// 3. Legacy sealedRecord JSON body (has "slice" field, but no effect_id/work_id)
	sealedRecordJSON := []byte(`{"slice":"S1","binds":"` + strings.Repeat("a", 40) + `","before":"` + strings.Repeat("b", 40) + `","candidate":"` + strings.Repeat("c", 40) + `"}`)
	legacySealedEv := projectEvidence(journal.EventFact{
		Offset:    3,
		Kind:      "candidate_sealed",
		SafeBody:  sealedRecordJSON,
		CreatedAt: now,
	})
	if legacySealedEv.EffectID != "" || legacySealedEv.WorkID != "" ||
		legacySealedEv.Track != "" || legacySealedEv.Slice != "" {
		t.Fatalf("legacy sealedRecord JSON body falsely populated association (collision): %#v", legacySealedEv)
	}

	// 4. Legacy RecoveryStepReceipt JSON body (has "step", "automatic_actions", etc.)
	recoveryReceiptJSON := []byte(`{"step":{"run_id":"run-1","step_id":"sha256:` + strings.Repeat("a", 64) + `","ordinal":1,"kind":"prose_nudge"},"automatic_actions":1,"corrections":0,"nudges":1,"advisories":0,"same_progress":0,"parked":false}`)
	legacyRecoveryEv := projectEvidence(journal.EventFact{
		Offset:    4,
		Kind:      "turn_recovery_step_reserved",
		SafeBody:  recoveryReceiptJSON,
		CreatedAt: now,
	})
	if legacyRecoveryEv.EffectID != "" || legacyRecoveryEv.WorkID != "" ||
		legacyRecoveryEv.Track != "" || legacyRecoveryEv.Slice != "" {
		t.Fatalf("legacy RecoveryStepReceipt JSON body falsely populated association: %#v", legacyRecoveryEv)
	}

	// 5. Empty / nil SafeBody
	nilEv := projectEvidence(journal.EventFact{
		Offset:    5,
		Kind:      "run_started",
		SafeBody:  nil,
		CreatedAt: now,
	})
	if nilEv.EffectID != "" || nilEv.WorkID != "" ||
		nilEv.Track != "" || nilEv.Slice != "" {
		t.Fatalf("nil SafeBody populated association: %#v", nilEv)
	}
}

func TestProjectorEventsFilteredByTrack(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{
			calls: &calls,
			window: journal.EventWindow{
				Events: []journal.EventFact{
					{
						Offset: 1, Kind: "dispatch_completed",
						SafeBody: runtimepkg.MarshalAssociation(runtimepkg.EventAssociation{
							EffectID: "eff-1", WorkID: "work-1", Track: "T1", Slice: "S1",
						}),
						CreatedAt: now.Add(time.Second),
					},
					{
						Offset: 2, Kind: "dispatch_completed",
						SafeBody: runtimepkg.MarshalAssociation(runtimepkg.EventAssociation{
							EffectID: "eff-2", WorkID: "work-2", Track: "T2", Slice: "S2",
						}),
						CreatedAt: now.Add(2 * time.Second),
					},
					{
						Offset: 3, Kind: "planner_replan_scheduled",
						SafeBody: runtimepkg.MarshalAssociation(runtimepkg.EventAssociation{
							EffectID: "eff-3", WorkID: "work-3", Track: "", Slice: "",
						}),
						CreatedAt: now.Add(3 * time.Second),
					},
					{
						Offset: 4, Kind: "captain_plan_decided",
						SafeBody:  nil, // run-scoped legacy / body-free
						CreatedAt: now.Add(4 * time.Second),
					},
					{
						Offset: 5, Kind: "candidate_prepared",
						SafeBody: runtimepkg.MarshalAssociation(runtimepkg.EventAssociation{
							EffectID: "eff-5", WorkID: "work-5", Track: "T1", Slice: "S1",
						}),
						CreatedAt: now.Add(5 * time.Second),
					},
				},
				Through: 5, EventOffset: 5, HasMore: false,
			},
		},
		&fakeRuntime{calls: &calls},
		&fakeStateReader{calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Query filtered by track T1: must receive only T1 events (1, 5) plus run-scoped (3, 4)
	pageT1, err := projector.Events(context.Background(), "run-1", 0, 10, "T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pageT1.Events) != 4 {
		t.Fatalf("T1 page event count = %d, want 4 (offsets 1, 3, 4, 5): %#v", len(pageT1.Events), pageT1.Events)
	}
	var offsetsT1 []int64
	for _, ev := range pageT1.Events {
		offsetsT1 = append(offsetsT1, ev.Offset)
		if ev.Track != "" && ev.Track != "T1" {
			t.Fatalf("T1 page included non-T1 track row: %#v", ev)
		}
	}
	if !reflect.DeepEqual(offsetsT1, []int64{1, 3, 4, 5}) {
		t.Fatalf("T1 page offsets = %v, want [1, 3, 4, 5]", offsetsT1)
	}

	// 2. Query filtered by track T2: must receive only T2 events (2) plus run-scoped (3, 4)
	pageT2, err := projector.Events(context.Background(), "run-1", 0, 10, "T2")
	if err != nil {
		t.Fatal(err)
	}
	var offsetsT2 []int64
	for _, ev := range pageT2.Events {
		offsetsT2 = append(offsetsT2, ev.Offset)
		if ev.Track != "" && ev.Track != "T2" {
			t.Fatalf("T2 page included non-T2 track row: %#v", ev)
		}
	}
	if !reflect.DeepEqual(offsetsT2, []int64{2, 3, 4}) {
		t.Fatalf("T2 page offsets = %v, want [2, 3, 4]", offsetsT2)
	}

	// 3. Query without track filter: receives all events (1, 2, 3, 4, 5)
	pageAll, err := projector.Events(context.Background(), "run-1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageAll.Events) != 5 {
		t.Fatalf("unfiltered page count = %d, want 5: %#v", len(pageAll.Events), pageAll.Events)
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

func TestProjectorSurfacesExactlyOneRetryActionForParkedCycle(t *testing.T) {
	t.Parallel()

	run, observation, status, state := projectionFixture()
	cycleWork := "sha256:" + strings.Repeat("1", 64)

	// In a parked implementation cycle, git.seal is top-level, and driver.dispatch & git.seal.prepared are derived children
	status.Effects = []runtimepkg.EffectStatus{
		{
			ID:   "attempt/" + strings.Repeat("1", 64) + "/e1/t3",
			Kind: "git.seal", State: string(journal.OperationalFailed),
			ErrorCode: "transport_error", Derived: false,
		},
		{
			ID:   "attempt/" + strings.Repeat("2", 64) + "/e1/t3",
			Kind: "driver.dispatch", State: string(journal.OperationalFailed),
			ErrorCode: "transport_error", Derived: true,
		},
		{
			ID:   "attempt/" + strings.Repeat("3", 64) + "/e1/t3",
			Kind: "git.seal.prepared", State: string(journal.OperationalFailed),
			ErrorCode: "transport_error", Derived: true,
		},
	}
	observation.Control = journal.ControlProjection{
		Generation:  4,
		Desired:     "running",
		RetryEpochs: map[string]int64{},
	}

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

	var retryActions []Action
	for _, action := range snapshot.Actions {
		if action.Kind == "retry" {
			retryActions = append(retryActions, action)
		}
	}
	if len(retryActions) != 1 {
		t.Fatalf("expected exactly 1 retry action, got %d: %#v", len(retryActions), retryActions)
	}
	if retryActions[0].WorkID != cycleWork || retryActions[0].ExpectedEpoch != 1 {
		t.Fatalf("retry action = %#v, want work=%s epoch=1", retryActions[0], cycleWork)
	}
}

func TestSafeActionsGatesTakeoverStrictlyOnTakeoverRequired(t *testing.T) {
	t.Parallel()

	control := journal.ControlProjection{Generation: 1, Desired: "running"}

	runningStatus := runtimepkg.RunStatus{
		State:             "running",
		ControlGeneration: 1,
	}
	runningActions := safeActions(runningStatus, control)
	if hasAction(runningActions, "takeover") {
		t.Fatalf("running state offered takeover action: %#v", runningActions)
	}
	if !hasAction(runningActions, "pause") || !hasAction(runningActions, "cancel") {
		t.Fatalf("running state missing pause/cancel: %#v", runningActions)
	}

	takeoverStatus := runtimepkg.RunStatus{
		State:             "takeover_required",
		ControlGeneration: 1,
	}
	takeoverActions := safeActions(takeoverStatus, control)
	if !hasAction(takeoverActions, "takeover") {
		t.Fatalf("takeover_required state missing takeover action: %#v", takeoverActions)
	}
}
