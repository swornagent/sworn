package cockpit

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
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
	states []protocol.State
	errs   []error
	calls  *[]string
}

func (f *fakeStateReader) Read(
	context.Context,
	journal.Run,
) (protocol.State, error) {
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
	protocol.State,
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
				Responsibility: "lead_review",
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
		SchemaVersion: "sworn.run-status/v4",
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
	sliceOne := &protocol.SliceState{
		Location: protocol.SliceLocation{
			Track: protocol.Track{ID: "T1"},
			Slice: protocol.Slice{ID: "S1"},
		},
		Stage: "implement", Status: "ready", NextRole: "implementer",
		Outcome: "none", Attempt: 2,
	}
	sliceTwo := &protocol.SliceState{
		Location: protocol.SliceLocation{
			Track: protocol.Track{ID: "T2", DependsOn: []string{"T1"}},
			Slice: protocol.Slice{
				ID: "S2", DependsOn: []string{"S1"}, Consumes: []string{"S1"},
			},
		},
		Stage: "design", Status: "waiting", NextRole: "none",
		Outcome: "none", Attempt: 1,
	}
	state := protocol.State{
		Release: run.Release, Repository: run.Repository,
		Plan: protocol.PlanState{
			Digest: status.PlanDigest,
		},
		Refs: protocol.StateRefs{
			Release: protocol.CapturedRef{
				Ref:  "refs/heads/release-wt/release-1",
				Head: status.ReleaseHead, State: "direct",
			},
			Target: protocol.CapturedRef{
				Ref: run.TargetRef, Head: status.TargetHead, State: "direct",
			},
			Tracks: []protocol.TrackRefState{
				{ID: "T1", CapturedRef: protocol.CapturedRef{
					Ref:  "refs/heads/track/release-1/T1",
					Head: strings.Repeat("3", 40), State: "direct",
				}},
				{ID: "T2", CapturedRef: protocol.CapturedRef{
					Ref:  "refs/heads/track/release-1/T2",
					Head: strings.Repeat("4", 40), State: "direct",
				}},
			},
		},
		Tracks: []protocol.TrackState{
			{ID: "T1", Slices: []*protocol.SliceState{sliceOne}},
			{
				ID: "T2", DependsOn: []string{"T1"},
				Slices: []*protocol.SliceState{sliceTwo},
			},
		},
		Slices: []*protocol.SliceState{sliceOne, sliceTwo},
		Assembly: protocol.AssemblyState{
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
	status.LeadDelegation = &runtimepkg.LeadDelegationView{Digest: "sha256:" + strings.Repeat("a", 64), Epoch: 2, State: "active", Decisions: 3, ReplanSpent: 1, ReplanBudget: 4}
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
			states: []protocol.State{state, state},
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
	if snapshot.LeadDelegation == nil || !reflect.DeepEqual(snapshot.LeadDelegation, status.LeadDelegation) {
		t.Fatalf("lead delegation = %#v", snapshot.LeadDelegation)
	}
	var authorityActions []Action
	for _, action := range snapshot.Actions {
		if strings.HasPrefix(action.Kind, "lead_delegation_") {
			authorityActions = append(authorityActions, action)
		}
	}
	if len(authorityActions) != 2 {
		t.Fatalf("Lead authority actions = %#v", authorityActions)
	}
	for _, action := range authorityActions {
		binding := action.LeadDelegation
		if binding == nil || binding.RunID != status.RunID ||
			binding.ManifestDigest != status.ManifestDigest ||
			binding.ActorClass != runtimepkg.LeadDelegationActorClass ||
			binding.ActorAuthority != status.ExternalAuthorizer ||
			binding.CurrentEpoch != 2 || binding.CurrentDigest != status.LeadDelegation.Digest {
			t.Fatalf("Lead authority binding = %#v", action)
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
		!snapshot.Graph.Nodes[2].HasProtocol ||
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
			states: []protocol.State{state, state, state, state},
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

func TestProjectorDegradesWithoutInventingProtocolProgressOrControls(t *testing.T) {
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
			states: []protocol.State{state, state},
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
		snapshot.Diagnostics[0].Code != "PROTOCOL_UNAVAILABLE" {
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
						Offset: 4, Kind: "lead_plan_decided",
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
			states: []protocol.State{state, state},
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

// A3/C10: for each reconciled-uncertain recovery verb the board's action
// list offers exactly that verb, derived from the runtime's gate-admitted
// RecoveryAction.
func TestSafeActionsOffersRecoveryVerbForUncertainState(t *testing.T) {
	t.Parallel()

	control := journal.ControlProjection{Generation: 1, Desired: "running"}
	work := "sha256:" + strings.Repeat("a", 64)

	retryStatus := runtimepkg.RunStatus{
		State:             "uncertain",
		ControlGeneration: 1,
		Recovery: &runtimepkg.RecoveryAction{
			Action: string(journal.Retry), WorkID: work, Epoch: 1,
		},
	}
	retryActions := safeActions(retryStatus, control)
	var retry *Action
	for index := range retryActions {
		if retryActions[index].Kind == string(journal.Retry) {
			retry = &retryActions[index]
		}
	}
	if retry == nil || retry.WorkID != work || retry.ExpectedEpoch != 1 {
		t.Fatalf("uncertain retry action = %#v", retryActions)
	}

	takeoverStatus := runtimepkg.RunStatus{
		State:             "uncertain",
		ControlGeneration: 1,
		Recovery: &runtimepkg.RecoveryAction{
			Action: string(journal.Takeover),
		},
	}
	if !hasAction(
		safeActions(takeoverStatus, control),
		string(journal.Takeover),
	) {
		t.Fatal("uncertain takeover recovery emitted no takeover action")
	}

	resumeStatus := runtimepkg.RunStatus{
		State:             "uncertain",
		ControlGeneration: 1,
		Recovery: &runtimepkg.RecoveryAction{
			Action: string(journal.Resume), WorkID: work, Epoch: 1,
		},
	}
	if !hasAction(
		safeActions(resumeStatus, control),
		string(journal.Resume),
	) {
		t.Fatal("uncertain resume recovery emitted no resume action")
	}
}

// A4: an economy-caused pinned work offers a "grant" action naming its
// crossing's own dispatch-work identity, unit, and current owner epoch -
// never a bare "retry", which the control gate would refuse
// ECONOMY_GRANT_REQUIRED - and a non-economy (identical-failure) pinned
// work offers neither, matching economyGrantUnit's cause vocabulary.
func TestSafeActionsOffersGrantActionForEconomyPinnedWorkOnly(t *testing.T) {
	t.Parallel()

	control := journal.ControlProjection{
		Generation: 3, Desired: "running",
		RetryEpochs: map[string]int64{"sha256:" + strings.Repeat("a", 64): 2},
	}
	owner := "sha256:" + strings.Repeat("a", 64)
	dispatchWork := "sha256:" + strings.Repeat("b", 64)

	economyStatus := runtimepkg.RunStatus{
		State: "parked", ControlGeneration: 3,
		PinnedWork: []runtimepkg.PinnedWork{{
			WorkID: owner, Lane: "T1", Cause: runtimepkg.ParkCauseEconomyTurns,
			Code: "ECONOMY_TURN_BUDGET_EXCEEDED", DispatchWorkID: dispatchWork,
		}},
	}
	actions := safeActions(economyStatus, control)
	if hasAction(actions, string(journal.Retry)) {
		t.Fatalf("economy-caused pinned work offered a bare retry action: %#v", actions)
	}
	var grant *Action
	for index := range actions {
		if actions[index].Kind == string(journal.Grant) {
			grant = &actions[index]
		}
	}
	if grant == nil {
		t.Fatalf("economy-caused pinned work offered no grant action: %#v", actions)
	}
	if grant.WorkID != dispatchWork || grant.Unit != runtimepkg.ParkCauseEconomyTurns ||
		grant.ExpectedEpoch != 2 || grant.ExpectedGeneration != 3 {
		t.Fatalf(
			"grant action = %#v, want work=%s unit=%s epoch=2 generation=3",
			grant, dispatchWork, runtimepkg.ParkCauseEconomyTurns,
		)
	}

	identicalStatus := runtimepkg.RunStatus{
		State: "parked", ControlGeneration: 3,
		PinnedWork: []runtimepkg.PinnedWork{{
			WorkID: owner, Lane: "T1", Cause: "identical_failure",
		}},
	}
	identicalActions := safeActions(identicalStatus, control)
	if hasAction(identicalActions, string(journal.Grant)) {
		t.Fatalf("identical-failure pinned work offered a grant action: %#v", identicalActions)
	}
}

// TestSafeActionsOffersRetryForEconomyContextPinnedWorkAtAnyTry anchors
// S6-context-window-clamp A3's board action: economy_context_window is
// never Grant-eligible (there is no manifest Limits value a Grant could
// raise to fix a fixed driver-config context window), so a PinnedWork of
// that cause offers a bare Retry naming the owner and its current epoch -
// at try 1, not only the third - and never a grant action.
func TestSafeActionsOffersRetryForEconomyContextPinnedWorkAtAnyTry(t *testing.T) {
	t.Parallel()

	owner := "sha256:" + strings.Repeat("a", 64)
	control := journal.ControlProjection{
		Generation: 3, Desired: "running",
		RetryEpochs: map[string]int64{owner: 2},
	}
	status := runtimepkg.RunStatus{
		State: "parked", ControlGeneration: 3,
		PinnedWork: []runtimepkg.PinnedWork{{
			WorkID: owner, Lane: "T1", Cause: runtimepkg.ParkCauseEconomyContext,
			Code: "ECONOMY_CONTEXT_EXHAUSTED", Detail: "context_window_tokens=50000",
		}},
	}
	actions := safeActions(status, control)
	if hasAction(actions, string(journal.Grant)) {
		t.Fatalf("economy_context_window pinned work offered a grant action: %#v", actions)
	}
	var retries []Action
	for _, action := range actions {
		if action.Kind == string(journal.Retry) {
			retries = append(retries, action)
		}
	}
	if len(retries) != 1 {
		t.Fatalf("retry actions = %#v, want exactly 1", retries)
	}
	if retries[0].WorkID != owner || retries[0].ExpectedEpoch != 2 ||
		retries[0].ExpectedGeneration != 3 {
		t.Fatalf("retry action = %#v, want work=%s epoch=2 generation=3", retries[0], owner)
	}
}

// TestSafeActionsDedupesEconomyContextRetryAgainstTryThreeLoop proves the
// t3 exhaustedAttempt loop and the new PinnedWork branch never both offer a
// Retry for the same economy_context_window crossing: the t3 loop skips
// ECONOMY_CONTEXT_EXHAUSTED-coded effects entirely (mirroring how
// isEconomyErrorCode's codes are skipped there), so PinnedWork is this
// cause's sole source of a board action even when the crossing happens to
// land exactly on try 3.
func TestSafeActionsDedupesEconomyContextRetryAgainstTryThreeLoop(t *testing.T) {
	t.Parallel()

	owner := "sha256:" + strings.Repeat("a", 64)
	control := journal.ControlProjection{
		Generation: 1, Desired: "running",
		RetryEpochs: map[string]int64{owner: 1},
	}
	status := runtimepkg.RunStatus{
		State: "parked", ControlGeneration: 1,
		Effects: []runtimepkg.EffectStatus{{
			ID:   "attempt/" + strings.Repeat("a", 64) + "/e1/t3",
			Kind: "driver.dispatch", State: string(journal.OperationalFailed),
			ErrorCode: "ECONOMY_CONTEXT_EXHAUSTED",
		}},
		PinnedWork: []runtimepkg.PinnedWork{{
			WorkID: owner, Lane: "T1", Cause: runtimepkg.ParkCauseEconomyContext,
			Code: "ECONOMY_CONTEXT_EXHAUSTED",
		}},
	}
	actions := safeActions(status, control)
	var retries []Action
	for _, action := range actions {
		if action.Kind == string(journal.Retry) {
			retries = append(retries, action)
		}
	}
	if len(retries) != 1 {
		t.Fatalf("retry actions = %#v, want exactly 1 (deduped against the t3 loop)", retries)
	}
	if retries[0].WorkID != owner {
		t.Fatalf("retry action = %#v, want work=%s", retries[0], owner)
	}
}

// TestIsEconomyErrorCodeExcludesContextExhausted pins the Lead's explicit
// allowance: isEconomyErrorCode stays unchanged for this release, since a
// t3 economy_context_window crossing is admissible as a bare Retry (unlike
// the two Grant-gated economy codes it names), so it must never be
// classified alongside them.
func TestIsEconomyErrorCodeExcludesContextExhausted(t *testing.T) {
	t.Parallel()
	if isEconomyErrorCode("ECONOMY_CONTEXT_EXHAUSTED") {
		t.Fatal("isEconomyErrorCode(ECONOMY_CONTEXT_EXHAUSTED) = true, want false")
	}
	if economyGrantUnit(runtimepkg.ParkCauseEconomyContext) != "" {
		t.Fatalf(
			"economyGrantUnit(%s) = %q, want empty (never Grant-eligible)",
			runtimepkg.ParkCauseEconomyContext, economyGrantUnit(runtimepkg.ParkCauseEconomyContext),
		)
	}
}

// TestActivityTurnCarriesReportedInputTokens anchors A4's live-activity-
// stream half: a tool_result_observed event's input_tokens field rides
// unchanged onto the projected ActivityTurn.
func TestActivityTurnCarriesReportedInputTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "activity-input-tokens.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(1_700_300_000, 0).UTC()
	run := journal.Run{
		ID: "run-activity-input-tokens", ManifestDigest: "sha256:" + strings.Repeat("a", 64),
		Repository: t.TempDir(), Release: "release-activity-input-tokens",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"schema_version": "sworn.tool-result-turn/v1", "run_id": run.ID,
		"track": "T1", "slice": "S1", "role": "implementer",
		"responsibility": "implementer_implementation",
		"attempt":        int64(1), "epoch": int64(1), "try": int64(1),
		"work_id": "work-1", "effect_id": "attempt/work-1/e1/t1",
		"turn": int64(1), "encoding": "base64", "input_tokens": int64(4_200),
		"results": []map[string]any{{
			"sequence": int64(1), "tool_call_id": "call-1", "tool": "Read",
			"failed": false, "total_bytes": int64(2),
			"omitted_bytes": int64(0), "redacted_bytes": int64(0),
			"head": base64.StdEncoding.EncodeToString([]byte("ok")), "tail": "",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", body, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var calls []string
	projector, err := NewProjector(
		store,
		&fakeRuntime{statuses: []runtimepkg.RunStatus{{}}, calls: &calls},
		&fakeStateReader{states: []protocol.State{{}}, errs: []error{nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := projector.Activity(ctx, run.ID, 0, 128, ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 || page.Turns[0].InputTokens == nil ||
		*page.Turns[0].InputTokens != 4_200 {
		t.Fatalf("turns = %#v, want one turn with InputTokens=*4200", page.Turns)
	}
}

// A1: the activity projection joins journaled worker-turn and tool-result
// events by (effect_id, turn), merges parts, decodes spans, and pages with
// one cursor. It reads only what S1 journaled, performs no new redaction,
// and leaves the metadata-only evidence projection unchanged.
func TestProjectorActivityProjectionCoversA1(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "activity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(1_700_200_000, 0).UTC()
	run := journal.Run{
		ID: "run-activity-1", ManifestDigest: "sha256:" + strings.Repeat("a", 64),
		Repository: t.TempDir(), Release: "release-activity",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	workerBody := func(turn int64, part, parts int64, content string) []byte {
		body, _ := json.Marshal(map[string]any{
			"schema_version": "sworn.worker-turn/v1", "run_id": run.ID,
			"track": "T1", "slice": "S1", "role": "implementer",
			"responsibility": "implementer_implementation",
			"attempt":        int64(1), "epoch": int64(1), "try": int64(1),
			"work_id": "work-1", "effect_id": "attempt/work-1/e1/t1",
			"turn": turn, "part": part, "parts": parts,
			"encoding": "base64",
			"content": []map[string]any{{
				"kind": "text", "total_bytes": int64(len(content)),
				"omitted_bytes": int64(7), "redacted_bytes": int64(3),
				"head": encode(content), "tail": "",
			}},
		})
		return body
	}
	toolBody := func(turn int64, tool string, failed bool, head string) []byte {
		body, _ := json.Marshal(map[string]any{
			"schema_version": "sworn.tool-result-turn/v1", "run_id": run.ID,
			"track": "T1", "slice": "S1", "role": "implementer",
			"responsibility": "implementer_implementation",
			"attempt":        int64(1), "epoch": int64(1), "try": int64(1),
			"work_id": "work-1", "effect_id": "attempt/work-1/e1/t1",
			"turn": turn, "encoding": "base64",
			"results": []map[string]any{{
				"sequence": int64(1), "tool_call_id": "call-1", "tool": tool,
				"failed": failed, "total_bytes": int64(len(head)),
				"omitted_bytes": int64(0), "redacted_bytes": int64(0),
				"head": encode(head), "tail": "",
			}},
		})
		return body
	}
	at := now.Add(time.Second)
	// Turn 1: native turn with worker text + tool result.
	if err := store.AppendEvent(ctx, run.ID, "worker_turn_observed", workerBody(1, 0, 0, "hello worker"), at); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", toolBody(1, "Read", false, "file bytes"), at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// Turn 2: HTTP-lane turn with tool results alone (no worker-turn event).
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", toolBody(2, "Bash", true, "error bytes"), at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	// Turn 3: split worker turn in two parts sharing one (effect, turn).
	if err := store.AppendEvent(ctx, run.ID, "worker_turn_observed", workerBody(3, 1, 2, "part one"), at.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "worker_turn_observed", workerBody(3, 2, 2, "part two"), at.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	// Fail-closed: unknown kind and wrong schema must never appear.
	if err := store.AppendEvent(ctx, run.ID, "other_kind", []byte(`{"schema_version":"sworn.worker-turn/v1","run_id":"`+run.ID+`","effect_id":"attempt/work-1/e1/t1","turn":9}`), at.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	wrongSchema, _ := json.Marshal(map[string]any{
		"schema_version": "sworn.worker-turn/v9", "run_id": run.ID,
		"track": "T1", "slice": "S1", "effect_id": "attempt/work-1/e1/t1", "turn": int64(9),
		"encoding": "base64", "content": []map[string]any{},
	})
	if err := store.AppendEvent(ctx, run.ID, "worker_turn_observed", wrongSchema, at.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	// Non-UTF-8 head span: arbitrary bytes that must decode with replacement.
	badHead := base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe, 0xfd, 'o', 'k'})
	badBody, _ := json.Marshal(map[string]any{
		"schema_version": "sworn.worker-turn/v1", "run_id": run.ID,
		"track": "T1", "slice": "S1", "role": "implementer",
		"responsibility": "implementer_implementation",
		"attempt":        int64(1), "epoch": int64(1), "try": int64(1),
		"work_id": "work-1", "effect_id": "attempt/work-1/e1/t1",
		"turn": int64(4), "encoding": "base64",
		"content": []map[string]any{{
			"kind": "text", "total_bytes": int64(5),
			"omitted_bytes": int64(0), "redacted_bytes": int64(0),
			"head": badHead, "tail": "",
		}},
	})
	if err := store.AppendEvent(ctx, run.ID, "worker_turn_observed", badBody, at.Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	var calls []string
	projector, err := NewProjector(
		store,
		&fakeRuntime{statuses: []runtimepkg.RunStatus{{}}, calls: &calls},
		&fakeStateReader{states: []protocol.State{{}}, errs: []error{nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := projector.Activity(ctx, run.ID, 0, 128, ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if page.SchemaVersion != ActivitySchemaVersion || page.RunID != run.ID || page.Live {
		t.Fatalf("activity page header = %#v, want journal-only Live=false", page)
	}
	if len(page.Turns) != 4 {
		t.Fatalf("turns = %d, want 4 (turns 1,2,3,4; kinds filtered): %#v", len(page.Turns), page.Turns)
	}
	// Turns are ordered by durable offset (1,2,3,4 in journal order).
	for i, want := range []int64{1, 2, 3, 4} {
		if page.Turns[i].Turn != want {
			t.Fatalf("turn order = %v, want 1,2,3,4", page.Turns)
		}
	}
	first := page.Turns[0]
	if first.Slice != "S1" || first.Role != "implementer" || first.Responsibility != "implementer_implementation" || first.Attempt != 1 || first.Try != 1 || first.Turn != 1 {
		t.Fatalf("first turn identity = %#v", first)
	}
	if len(first.Content) != 1 || first.Content[0].Head != "hello worker" || first.Content[0].OmittedBytes != 7 || first.Content[0].RedactedBytes != 3 {
		t.Fatalf("first content = %#v", first.Content)
	}
	if len(first.Results) != 1 || first.Results[0].Tool != "Read" || first.Results[0].Failed || first.Results[0].Head != "file bytes" {
		t.Fatalf("first results = %#v", first.Results)
	}
	// HTTP-lane turn exists without a worker-turn event.
	second := page.Turns[1]
	if len(second.Content) != 0 || len(second.Results) != 1 || second.Results[0].Tool != "Bash" || !second.Results[0].Failed {
		t.Fatalf("tool-only turn = %#v", second)
	}
	// Split parts merge in part order with max parts surfaced.
	third := page.Turns[2]
	if third.Parts != 2 || len(third.Content) != 2 || third.Content[0].Head != "part one" || third.Content[1].Head != "part two" {
		t.Fatalf("split turn = %#v", third)
	}
	// Non-UTF-8 decodes with named replacement; counts stay authoritative.
	fourth := page.Turns[3]
	if len(fourth.Content) != 1 || !strings.Contains(fourth.Content[0].Head, "�") || fourth.Content[0].TotalBytes != 5 {
		t.Fatalf("non-utf8 head = %q counts=%d", fourth.Content[0].Head, fourth.Content[0].TotalBytes)
	}
	// Fail-closed: turn 9 never appears.
	for _, turn := range page.Turns {
		if turn.Turn == 9 {
			t.Fatalf("filtered kind or schema leaked: %#v", turn)
		}
	}
	// Paging: after/limit with one cursor, no gap and no duplicate.
	firstPage, err := projector.Activity(ctx, run.ID, 0, 2, ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Turns) != 2 || !firstPage.HasMore {
		t.Fatalf("first page = %#v", firstPage)
	}
	secondPage, err := projector.Activity(ctx, run.ID, firstPage.ThroughOffset, 2, ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Turns) != 2 || secondPage.HasMore {
		t.Fatalf("second page = %#v", secondPage)
	}
	seen := make(map[int64]bool)
	for _, turn := range append(firstPage.Turns, secondPage.Turns...) {
		if seen[turn.Offset] {
			t.Fatalf("duplicate offset %d across pages", turn.Offset)
		}
		seen[turn.Offset] = true
	}
	if len(seen) != 4 {
		t.Fatalf("paged offsets = %v, want 4 distinct", seen)
	}
	// Filtering: track/slice narrow, dispatch narrows to one effect.
	filtered, err := projector.Activity(ctx, run.ID, 0, 128, ActivityFilter{Track: "T1", Slice: "S1"})
	if err != nil || len(filtered.Turns) != 4 {
		t.Fatalf("track+slice filter = %#v, %v", filtered, err)
	}
	empty, err := projector.Activity(ctx, run.ID, 0, 128, ActivityFilter{Track: "T9"})
	if err != nil || len(empty.Turns) != 0 {
		t.Fatalf("non-matching track filter = %#v, %v", empty, err)
	}
	byEffect, err := projector.Activity(ctx, run.ID, 0, 128, ActivityFilter{EffectID: "attempt/work-1/e1/t1"})
	if err != nil || len(byEffect.Turns) != 4 {
		t.Fatalf("effect filter = %#v, %v", byEffect, err)
	}
	// The metadata-only evidence projection is unchanged: Events still
	// returns content-free evidence with no worker bodies. The page type
	// carries no content fields; worker text never appears in its JSON.
	events, err := projector.Events(ctx, run.ID, 0, 128)
	if err != nil {
		t.Fatal(err)
	}
	evidenceJSON, _ := json.Marshal(events)
	for _, forbidden := range []string{"hello worker", "file bytes", "part one"} {
		if strings.Contains(string(evidenceJSON), forbidden) {
			t.Fatalf("evidence page leaked worker content %q: %s", forbidden, evidenceJSON)
		}
	}
}

// A1+A2+A6 repair: a dispatch that parks on a human turn and resumes
// reuses its provider turn numbers. Two single-part tool events sharing
// one (effect_id, turn) are two distinct worker turns, not two parts of
// one. Merging them makes a served row grow after emission (live Turn 1
// with one result versus journal Turn 1 with two), breaking resume and
// the e2e live-equals-journal check. The projection must split them into
// stable instances with distinct offsets.
func TestProjectorActivitySplitsResumedTurnNumbers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "activity-resume.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(1_700_210_000, 0).UTC()
	run := journal.Run{
		ID: "run-activity-resume", ManifestDigest: "sha256:" + strings.Repeat("b", 64),
		Repository: t.TempDir(), Release: "release-activity",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	toolBody := func(turn int64, callID, tool, head string) []byte {
		body, _ := json.Marshal(map[string]any{
			"schema_version": "sworn.tool-result-turn/v1", "run_id": run.ID,
			"track": "T1", "slice": "S1", "role": "implementer",
			"responsibility": "implementer_implementation",
			"attempt":        int64(1), "epoch": int64(1), "try": int64(1),
			"work_id": "work-1", "effect_id": "attempt/work-1/e1/t1",
			"turn": turn, "encoding": "base64",
			"results": []map[string]any{{
				"sequence": int64(1), "tool_call_id": callID, "tool": tool,
				"failed": false, "total_bytes": int64(len(head)),
				"omitted_bytes": int64(0), "redacted_bytes": int64(0),
				"head": encode(head), "tail": "",
			}},
		})
		return body
	}
	at := now.Add(time.Second)
	// Turn 1 yield before the park, Turn 2 between, Turn 1 Write after the
	// resume: the two Turn 1 rows share a number but are distinct turns.
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", toolBody(1, "call-yield-1", "sworn_yield", "accepted"), at); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", toolBody(2, "call-read-1", "Read", "between"), at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", toolBody(1, "call-write-1", "Write", "ok"), at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	var calls []string
	projector, err := NewProjector(
		store,
		&fakeRuntime{statuses: []runtimepkg.RunStatus{{}}, calls: &calls},
		&fakeStateReader{states: []protocol.State{{}}, errs: []error{nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := projector.Activity(ctx, run.ID, 0, 128, ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 3 {
		t.Fatalf("turns = %d, want 3 (two Turn 1 instances + Turn 2): %#v", len(page.Turns), page.Turns)
	}
	if page.Turns[0].Turn != 1 || len(page.Turns[0].Results) != 1 || page.Turns[0].Results[0].Tool != "sworn_yield" {
		t.Fatalf("first Turn 1 = %#v", page.Turns[0])
	}
	if page.Turns[1].Turn != 2 || len(page.Turns[1].Results) != 1 {
		t.Fatalf("Turn 2 = %#v", page.Turns[1])
	}
	if page.Turns[2].Turn != 1 || len(page.Turns[2].Results) != 1 || page.Turns[2].Results[0].Tool != "Write" {
		t.Fatalf("second Turn 1 = %#v", page.Turns[2])
	}
	for i := 1; i < len(page.Turns); i++ {
		if page.Turns[i].Offset <= page.Turns[i-1].Offset {
			t.Fatalf("turns out of order: %#v", page.Turns)
		}
	}
	// Resuming after the first Turn 1 returns the later turns unchanged:
	// the second Turn 1 still carries exactly its own result, never the
	// first Turn 1's merged in.
	resumed, err := projector.Activity(ctx, run.ID, page.Turns[0].Offset, 128, ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Turns) != 2 || resumed.Turns[1].Turn != 1 || len(resumed.Turns[1].Results) != 1 || resumed.Turns[1].Results[0].Tool != "Write" {
		t.Fatalf("resumed page = %#v, want Turn 2 + tool-only Turn 1 Write", resumed.Turns)
	}
	// Paging across the split has no gap and no duplicate.
	first, err := projector.Activity(ctx, run.ID, 0, 2, ActivityFilter{})
	if err != nil || len(first.Turns) != 2 || !first.HasMore {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	second, err := projector.Activity(ctx, run.ID, first.ThroughOffset, 2, ActivityFilter{})
	if err != nil || len(second.Turns) != 1 || second.HasMore {
		t.Fatalf("second page = %#v, %v", second, err)
	}
	if second.Turns[0].Offset != page.Turns[2].Offset || len(second.Turns[0].Results) != 1 {
		t.Fatalf("paged second Turn 1 = %#v", second.Turns[0])
	}
}

// A1 native pairing: when a resumed number carries both worker and tool
// sides, each resume's worker pairs with its own tool by shared
// tool-call identity, and a worker-only turn stays alone.
func TestProjectorActivityPairsResumedWorkerAndToolByCallID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "activity-pair.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(1_700_220_000, 0).UTC()
	run := journal.Run{
		ID: "run-activity-pair", ManifestDigest: "sha256:" + strings.Repeat("c", 64),
		Repository: t.TempDir(), Release: "release-activity",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	workerBody := func(callID, text string) []byte {
		body, _ := json.Marshal(map[string]any{
			"schema_version": "sworn.worker-turn/v1", "run_id": run.ID,
			"track": "T1", "slice": "S1", "role": "implementer",
			"responsibility": "implementer_implementation",
			"attempt":        int64(1), "epoch": int64(1), "try": int64(1),
			"work_id": "work-1", "effect_id": "attempt/work-1/e1/t1",
			"turn": int64(1), "encoding": "base64",
			"content": []map[string]any{{
				"kind": "tool_call", "tool": "Read", "tool_call_id": callID,
				"total_bytes":   int64(len(text)),
				"omitted_bytes": int64(0), "redacted_bytes": int64(0),
				"head": encode(text), "tail": "",
			}},
		})
		return body
	}
	toolBody := func(callID, head string) []byte {
		body, _ := json.Marshal(map[string]any{
			"schema_version": "sworn.tool-result-turn/v1", "run_id": run.ID,
			"track": "T1", "slice": "S1", "role": "implementer",
			"responsibility": "implementer_implementation",
			"attempt":        int64(1), "epoch": int64(1), "try": int64(1),
			"work_id": "work-1", "effect_id": "attempt/work-1/e1/t1",
			"turn": int64(1), "encoding": "base64",
			"results": []map[string]any{{
				"sequence": int64(1), "tool_call_id": callID, "tool": "Read",
				"failed": false, "total_bytes": int64(len(head)),
				"omitted_bytes": int64(0), "redacted_bytes": int64(0),
				"head": encode(head), "tail": "",
			}},
		})
		return body
	}
	at := now.Add(time.Second)
	if err := store.AppendEvent(ctx, run.ID, "worker_turn_observed", workerBody("call-A", "first"), at); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", toolBody("call-A", "first-result"), at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "worker_turn_observed", workerBody("call-B", "second"), at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(ctx, run.ID, "tool_result_observed", toolBody("call-B", "second-result"), at.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	var calls []string
	projector, err := NewProjector(
		store,
		&fakeRuntime{statuses: []runtimepkg.RunStatus{{}}, calls: &calls},
		&fakeStateReader{states: []protocol.State{{}}, errs: []error{nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := projector.Activity(ctx, run.ID, 0, 128, ActivityFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 2 {
		t.Fatalf("turns = %d, want 2 paired instances: %#v", len(page.Turns), page.Turns)
	}
	for i, want := range []string{"call-A", "call-B"} {
		turn := page.Turns[i]
		if len(turn.Content) != 1 || len(turn.Results) != 1 || turn.Content[0].ToolCallID != want || turn.Results[0].ToolCallID != want {
			t.Fatalf("paired turn %d = %#v, want %s", i, turn, want)
		}
	}
}

// S3-failure-turn-context A3: the status projection exposes the bounded
// tail beside code/detail, the projector copies Effect and PinnedWork
// contexts to EffectView and the actionable Node, absent/empty/
// unavailable keep one meaning each, tampering surfaces as
// CORRUPT_JOURNAL rather than shown, and metadata-only Evidence stays
// content-free.
func TestProjectorFailureTurnContextCopiesEffectAndNode(t *testing.T) {
	t.Parallel()
	run, observation, status, state := projectionFixture()
	present := &runtimepkg.FailureTurnContext{
		SchemaVersion: "sworn.failure-turn-context/v1", Status: "present",
		Turns: []runtimepkg.FailureTurn{{
			Kind: "tool_result", Turn: 2,
			Results: []runtimepkg.FailureToolResult{{
				Sequence: 1, ToolCallID: "call-1", Tool: "Read", TotalBytes: 4, Head: "data",
			}},
		}},
		Omitted: 1, DroppedMaxVisible: 2,
	}
	empty := &runtimepkg.FailureTurnContext{
		SchemaVersion: "sworn.failure-turn-context/v1", Status: "empty", Turns: []runtimepkg.FailureTurn{},
	}
	unavailable := &runtimepkg.FailureTurnContext{
		SchemaVersion: "sworn.failure-turn-context/v1", Status: "unavailable",
		Turns: []runtimepkg.FailureTurn{}, Reason: "no_live_tail",
	}
	status.Effects = []runtimepkg.EffectStatus{
		{ID: "attempt/" + strings.Repeat("b", 64) + "/e2/t3", Kind: "driver.dispatch", State: string(journal.OperationalFailed), ErrorCode: "transport_error", FailureTurnContext: present},
		{ID: "attempt/" + strings.Repeat("c", 64) + "/e1/t1", Kind: "driver.dispatch", State: string(journal.OperationalFailed), ErrorCode: "other", FailureTurnContext: empty},
		{ID: "attempt/" + strings.Repeat("d", 64) + "/e1/t1", Kind: "driver.dispatch", State: string(journal.Uncertain), FailureTurnContext: unavailable},
		{ID: "attempt/" + strings.Repeat("e", 64) + "/e1/t1", Kind: "driver.dispatch", State: string(journal.OperationalFailed), ErrorCode: "old"},
	}
	status.PinnedWork = []runtimepkg.PinnedWork{{
		WorkID: "sha256:" + strings.Repeat("b", 64), Lane: "T1", Cause: "exhaustion",
		Code: "transport_error", FailureTurnContext: present,
	}}
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{binding: run, observations: []journal.Observation{observation}, calls: &calls},
		&fakeRuntime{statuses: []runtimepkg.RunStatus{status, status}, calls: &calls},
		&fakeStateReader{states: []protocol.State{state, state}, errs: []error{nil, nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runtime.Effects) != 4 {
		t.Fatalf("effects = %d", len(snapshot.Runtime.Effects))
	}
	if got := snapshot.Runtime.Effects[0].FailureTurnContext; got == nil || got.Status != "present" || len(got.Turns) != 1 || got.Turns[0].Results[0].Head != "data" || got.Omitted != 1 || got.DroppedMaxVisible != 2 {
		t.Fatalf("present effect = %#v", got)
	}
	if got := snapshot.Runtime.Effects[1].FailureTurnContext; got == nil || got.Status != "empty" || len(got.Turns) != 0 {
		t.Fatalf("empty effect = %#v, want explicit empty", got)
	}
	if got := snapshot.Runtime.Effects[2].FailureTurnContext; got == nil || got.Status != "unavailable" || got.Reason != "no_live_tail" {
		t.Fatalf("unavailable effect = %#v", got)
	}
	if got := snapshot.Runtime.Effects[3].FailureTurnContext; got != nil {
		t.Fatalf("absent effect = %#v, want nil (pre-S3)", got)
	}
	// PinnedWork->Node via lane->actionable-node: T1 maps to its ready
	// slice node (HasProtocol), release would map to assembly.
	var sliceNode, assemblyNode *Node
	for i := range snapshot.Graph.Nodes {
		node := &snapshot.Graph.Nodes[i]
		if node.ID == "slice:S1" {
			sliceNode = node
		}
		if node.Kind == "assembly" {
			assemblyNode = node
		}
	}
	if sliceNode == nil || sliceNode.FailureTurnContext == nil || sliceNode.FailureTurnContext.Status != "present" {
		t.Fatalf("slice node context = %#v", sliceNode)
	}
	if assemblyNode != nil && assemblyNode.FailureTurnContext != nil {
		t.Fatalf("assembly node context = %#v, want nil (release lane not pinned)", assemblyNode.FailureTurnContext)
	}
	// No leak: metadata-only Evidence carries no worker content.
	evidenceJSON, _ := json.Marshal(snapshot.Evidence)
	if strings.Contains(string(evidenceJSON), "data") {
		t.Fatalf("evidence leaked worker content: %s", evidenceJSON)
	}
}

func TestProjectorFailureTurnContextReleaseLaneMapsToAssembly(t *testing.T) {
	t.Parallel()
	run, observation, status, state := projectionFixture()
	present := &runtimepkg.FailureTurnContext{
		SchemaVersion: "sworn.failure-turn-context/v1", Status: "present",
		Turns: []runtimepkg.FailureTurn{{Kind: "tool_result", Turn: 1}},
	}
	status.PinnedWork = []runtimepkg.PinnedWork{{
		WorkID: "sha256:" + strings.Repeat("f", 64), Lane: "release", Cause: "exhaustion",
		FailureTurnContext: present,
	}}
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{binding: run, observations: []journal.Observation{observation}, calls: &calls},
		&fakeRuntime{statuses: []runtimepkg.RunStatus{status, status}, calls: &calls},
		&fakeStateReader{states: []protocol.State{state, state}, errs: []error{nil, nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range snapshot.Graph.Nodes {
		if node.Kind == "assembly" {
			if node.FailureTurnContext == nil || node.FailureTurnContext.Status != "present" {
				t.Fatalf("assembly context = %#v", node.FailureTurnContext)
			}
			return
		}
	}
	t.Fatal("assembly node missing")
}

func TestProjectorFailureTurnContextTamperFailsCorruptJournal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "failure-tamper.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	// Do not close before tampering; the store holds the only connection.
	now := time.Unix(1_700_300_000, 0).UTC()
	run := journal.Run{
		ID: "run-failure-tamper", ManifestDigest: "sha256:" + strings.Repeat("a", 64),
		Repository: t.TempDir(), Release: "release-tamper",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	assoc := runtimepkg.MarshalAssociation(runtimepkg.EventAssociation{EffectID: "attempt/work/e1/t1", WorkID: "work"})
	body, _ := json.Marshal(map[string]any{
		"effect_id": "attempt/work/e1/t1", "work_id": "work",
		"failure_turn_context": map[string]any{
			"schema_version": "sworn.failure-turn-context/v1", "status": "present",
			"turns": []any{map[string]any{"kind": "tool_result", "turn": int64(1)}},
		},
	})
	_ = assoc
	if err := store.AppendEvent(ctx, run.ID, "dispatch_operational_failure", body, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Snapshot(ctx, run.ID); err != nil {
		t.Fatalf("clean snapshot = %v", err)
	}
	// Tamper with the body without updating its digest: the digest-checked
	// read must fail CORRUPT_JOURNAL rather than show the tampered tail.
	if err := tamperJournalEventBody(t, store, run.ID, []byte(`{"effect_id":"attempt/work/e1/t1","failure_turn_context":{"schema_version":"sworn.failure-turn-context/v1","status":"present","turns":[{"kind":"tool_result","turn":99}]}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Snapshot(ctx, run.ID); !journal.IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("tampered snapshot = %v, want CORRUPT_JOURNAL", err)
	}
	if _, err := store.ReadWindow(ctx, run.ID, 0, 64); !journal.IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("tampered window = %v, want CORRUPT_JOURNAL", err)
	}
	_ = store.Close()
}

func tamperJournalEventBody(t *testing.T, store *journal.Store, runID string, tampered []byte) error {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+store.Path()+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE events SET body = ? WHERE run_id = ?`, tampered, runID); err != nil {
		return err
	}
	return nil
}

// S3-host-check-failure-facts A4: the projector copies the host-check
// failure fact from EffectStatus to EffectView and from PinnedWork to the
// actionable Node via the same lane->actionable-node rule as the failure
// turn context, with the release lane mapping to the assembly node.
// CheckOutcome copies field-for-field beside the effect state.
func TestProjectorHostCheckFailureCopiesEffectAndNode(t *testing.T) {
	t.Parallel()
	run, observation, status, state := projectionFixture()
	rerunExit := 3
	fact := &runtimepkg.HostCheckFailureFact{
		SchemaVersion: "sworn.host-check-failure-fact/v1",
		Check:         "exit 7", Outcome: "fail", ExitCode: 7,
		Reran: true, RerunOutcome: "fail", RerunExitCode: &rerunExit,
		NotRun: []string{"printf 'third\\n'"}, Excerpt: "needs repair",
		OutputDigest: "sha256:" + strings.Repeat("d", 64),
		HostEffect:   "attempt/aaa/e1/t1", RerunEffect: "attempt/bbb/e1/t1",
		Candidate: strings.Repeat("c", 40), ContractDigest: "sha256:" + strings.Repeat("e", 64),
	}
	status.Effects = []runtimepkg.EffectStatus{
		{ID: "attempt/" + strings.Repeat("b", 64) + "/e2/t3", Kind: "driver.dispatch", State: string(journal.OperationalFailed), ErrorCode: "HOST_CHECK_FAILED", HostCheckFailure: fact},
		{ID: "attempt/" + strings.Repeat("c", 64) + "/e1/t1", Kind: "driver.dispatch", State: string(journal.OperationalFailed), ErrorCode: "other"},
		{ID: "attempt/" + strings.Repeat("d", 64) + "/e1/t1", Kind: "check.host", State: string(journal.Succeeded), CheckOutcome: "fail"},
		{ID: "attempt/" + strings.Repeat("e", 64) + "/e1/t1", Kind: "check.host", State: string(journal.Succeeded), CheckOutcome: "pass"},
		{ID: "attempt/" + strings.Repeat("f", 64) + "/e1/t1", Kind: "check.host", State: string(journal.Succeeded)},
	}
	status.PinnedWork = []runtimepkg.PinnedWork{{
		WorkID: "sha256:" + strings.Repeat("b", 64), Lane: "T1", Cause: "identical_failure",
		Code: "HOST_CHECK_FAILED", HostCheckFailure: fact,
	}}
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{binding: run, observations: []journal.Observation{observation}, calls: &calls},
		&fakeRuntime{statuses: []runtimepkg.RunStatus{status, status}, calls: &calls},
		&fakeStateReader{states: []protocol.State{state, state}, errs: []error{nil, nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runtime.Effects) != 5 {
		t.Fatalf("effects = %d", len(snapshot.Runtime.Effects))
	}
	if got := snapshot.Runtime.Effects[0].HostCheckFailure; got == nil || got.Check != "exit 7" || got.ExitCode != 7 || !got.Reran || got.RerunOutcome != "fail" || got.RerunExitCode == nil || *got.RerunExitCode != 3 || len(got.NotRun) != 1 {
		t.Fatalf("effect fact = %#v", got)
	}
	if got := snapshot.Runtime.Effects[1].HostCheckFailure; got != nil {
		t.Fatalf("absent effect fact = %#v, want nil", got)
	}
	if got := snapshot.Runtime.Effects[2].CheckOutcome; got != "fail" {
		t.Fatalf("failing check outcome = %q, want fail", got)
	}
	if got := snapshot.Runtime.Effects[3].CheckOutcome; got != "pass" {
		t.Fatalf("passing check outcome = %q, want pass", got)
	}
	if got := snapshot.Runtime.Effects[4].CheckOutcome; got != "" {
		t.Fatalf("absent check outcome = %q, want empty", got)
	}
	var sliceNode, assemblyNode *Node
	for i := range snapshot.Graph.Nodes {
		node := &snapshot.Graph.Nodes[i]
		if node.ID == "slice:S1" {
			sliceNode = node
		}
		if node.Kind == "assembly" {
			assemblyNode = node
		}
	}
	if sliceNode == nil || sliceNode.HostCheckFailure == nil || sliceNode.HostCheckFailure.Check != "exit 7" {
		t.Fatalf("slice node fact = %#v", sliceNode)
	}
	if assemblyNode != nil && assemblyNode.HostCheckFailure != nil {
		t.Fatalf("assembly node fact = %#v, want nil (release lane not pinned)", assemblyNode.HostCheckFailure)
	}
}

func TestProjectorProviderStallCopiesToNode(t *testing.T) {
	t.Parallel()
	run, observation, status, state := projectionFixture()
	stall := runtimepkg.ProviderStallStatus{
		WorkID: "sha256:" + strings.Repeat("b", 64), Lane: "T1",
		FailureCode: "PROVIDER_UNAVAILABLE", Profile: "openai", Model: "gpt",
		Index: 2, NextProbeAt: run.CreatedAt, LastProbeCode: "certification_provider_unavailable",
		ElapsedMillis: 180_000, BoundMillis: 1_800_000,
	}
	status.ProviderStall = []runtimepkg.ProviderStallStatus{stall}
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{binding: run, observations: []journal.Observation{observation}, calls: &calls},
		&fakeRuntime{statuses: []runtimepkg.RunStatus{status, status}, calls: &calls},
		&fakeStateReader{states: []protocol.State{state, state}, errs: []error{nil, nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sliceNode, assemblyNode *Node
	for i := range snapshot.Graph.Nodes {
		node := &snapshot.Graph.Nodes[i]
		if node.ID == "slice:S1" {
			sliceNode = node
		}
		if node.Kind == "assembly" {
			assemblyNode = node
		}
	}
	if sliceNode == nil || sliceNode.ProviderStall == nil ||
		sliceNode.ProviderStall.FailureCode != "PROVIDER_UNAVAILABLE" ||
		sliceNode.ProviderStall.LastProbeCode != "certification_provider_unavailable" {
		t.Fatalf("slice node provider stall = %#v", sliceNode)
	}
	if assemblyNode != nil && assemblyNode.ProviderStall != nil {
		t.Fatalf("assembly node provider stall = %#v, want nil (release lane not waiting)", assemblyNode.ProviderStall)
	}
}

func TestProjectorProviderStallReleaseLaneMapsToAssembly(t *testing.T) {
	t.Parallel()
	run, observation, status, state := projectionFixture()
	status.ProviderStall = []runtimepkg.ProviderStallStatus{{
		WorkID: "sha256:" + strings.Repeat("f", 64), Lane: "release",
		FailureCode: "PROVIDER_LIMITED", Index: 1, NextProbeAt: run.CreatedAt,
	}}
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{binding: run, observations: []journal.Observation{observation}, calls: &calls},
		&fakeRuntime{statuses: []runtimepkg.RunStatus{status, status}, calls: &calls},
		&fakeStateReader{states: []protocol.State{state, state}, errs: []error{nil, nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range snapshot.Graph.Nodes {
		if node.Kind == "assembly" {
			if node.ProviderStall == nil || node.ProviderStall.FailureCode != "PROVIDER_LIMITED" {
				t.Fatalf("assembly provider stall = %#v", node.ProviderStall)
			}
			return
		}
	}
	t.Fatal("assembly node missing")
}

func TestProjectorHostCheckFailureReleaseLaneMapsToAssembly(t *testing.T) {
	t.Parallel()
	run, observation, status, state := projectionFixture()
	fact := &runtimepkg.HostCheckFailureFact{
		SchemaVersion: "sworn.host-check-failure-fact/v1",
		Check:         "exit 7", Outcome: "fail", ExitCode: 7,
		Excerpt: "out", OutputDigest: "sha256:" + strings.Repeat("d", 64),
		HostEffect: "attempt/aaa/e1/t1",
		Candidate:  strings.Repeat("c", 40), ContractDigest: "sha256:" + strings.Repeat("e", 64),
	}
	status.PinnedWork = []runtimepkg.PinnedWork{{
		WorkID: "sha256:" + strings.Repeat("f", 64), Lane: "release", Cause: "exhaustion",
		HostCheckFailure: fact,
	}}
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{binding: run, observations: []journal.Observation{observation}, calls: &calls},
		&fakeRuntime{statuses: []runtimepkg.RunStatus{status, status}, calls: &calls},
		&fakeStateReader{states: []protocol.State{state, state}, errs: []error{nil, nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range snapshot.Graph.Nodes {
		if node.Kind == "assembly" {
			if node.HostCheckFailure == nil || node.HostCheckFailure.Check != "exit 7" {
				t.Fatalf("assembly fact = %#v", node.HostCheckFailure)
			}
			return
		}
	}
	t.Fatal("assembly node missing")
}

// Lead correction 4: for a work that is failing but not pinned, the fact
// still appears on its EffectStatus/EffectView through sworn status --json
// and sworn_status.
func TestProjectorHostCheckFailureSurfacesWithoutPin(t *testing.T) {
	t.Parallel()
	run, observation, status, state := projectionFixture()
	fact := &runtimepkg.HostCheckFailureFact{
		SchemaVersion: "sworn.host-check-failure-fact/v1",
		Check:         "exit 7", Outcome: "fail", ExitCode: 7,
		Excerpt: "out", OutputDigest: "sha256:" + strings.Repeat("d", 64),
		HostEffect: "attempt/aaa/e1/t1",
		Candidate:  strings.Repeat("c", 40), ContractDigest: "sha256:" + strings.Repeat("e", 64),
	}
	status.Effects = []runtimepkg.EffectStatus{
		{ID: "attempt/" + strings.Repeat("b", 64) + "/e1/t1", Kind: "driver.dispatch", State: string(journal.OperationalFailed), ErrorCode: "HOST_CHECK_FAILED", HostCheckFailure: fact},
	}
	status.PinnedWork = nil
	var calls []string
	projector, err := NewProjector(
		&fakeJournal{binding: run, observations: []journal.Observation{observation}, calls: &calls},
		&fakeRuntime{statuses: []runtimepkg.RunStatus{status, status}, calls: &calls},
		&fakeStateReader{states: []protocol.State{state, state}, errs: []error{nil, nil}, calls: &calls},
	)
	if err != nil {
		t.Fatal(err)
	}
	projector.now = func() time.Time { return run.CreatedAt }
	snapshot, err := projector.Snapshot(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runtime.Effects) != 1 || snapshot.Runtime.Effects[0].HostCheckFailure == nil {
		t.Fatalf("unpinned failing effect lost its fact: %#v", snapshot.Runtime.Effects)
	}
	if got := snapshot.Runtime.Effects[0].HostCheckFailure.Check; got != "exit 7" {
		t.Fatalf("unpinned fact check = %q", got)
	}
}
