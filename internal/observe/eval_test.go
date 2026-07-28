package observe

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
)

type fakeEvaluationJournal struct {
	cursor  int64
	window  journal.EvaluationWindow
	facts   []journal.EvaluationFact
	advance *journal.ObserverAdvance
	err     error
}

func (f *fakeEvaluationJournal) ObserverCursor(
	context.Context,
	string,
	string,
) (int64, error) {
	return f.cursor, nil
}

func (f *fakeEvaluationJournal) VisitEvaluation(
	_ context.Context,
	_ string,
	visit func(journal.EvaluationFact),
) (journal.EvaluationWindow, error) {
	for _, fact := range f.facts {
		visit(fact)
	}
	return f.window, nil
}

func (f *fakeEvaluationJournal) AdvanceObserver(
	_ context.Context,
	value journal.ObserverAdvance,
) error {
	if f.err != nil {
		return f.err
	}
	copy := value
	copy.Eval = append([]journal.EvalDraft(nil), value.Eval...)
	f.advance = &copy
	f.cursor = value.ThroughOffset
	return nil
}

type fakeSnapshotProjector struct {
	snapshots []cockpit.Snapshot
	err       error
}

func (f *fakeSnapshotProjector) Snapshot(
	context.Context,
	string,
) (cockpit.Snapshot, error) {
	if f.err != nil {
		return cockpit.Snapshot{}, f.err
	}
	value := f.snapshots[0]
	if len(f.snapshots) > 1 {
		f.snapshots = f.snapshots[1:]
	}
	return value, nil
}

func TestEvaluatorPersistsCanonicalCumulativeRecord(t *testing.T) {
	t.Parallel()

	started := time.Unix(1_700_000_000, 123).UTC()
	finished := started.Add(2 * time.Second)
	zero := int64(0)
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID:        "run-1",
				Release:   "release-1",
				CreatedAt: started,
			},
			ThroughOffset: 4,
			ObservedAt:    finished,
		},
		facts: []journal.EvaluationFact{
			{Kind: journal.EvaluationEvent, EventOffset: 1,
				EventKind: "dispatch_completed", FinishedAt: finished},
			{Kind: journal.EvaluationEvent, EventOffset: 2,
				EventKind: "dispatch_uncertain", FinishedAt: finished},
			{Kind: journal.EvaluationEvent, EventOffset: 3,
				EventKind: "candidate_reconciled", FinishedAt: finished},
			{Kind: journal.EvaluationEvent, EventOffset: 4,
				EventKind: "candidate_recovered", FinishedAt: finished},
			{
				Kind:           journal.EvaluationAttempt,
				EffectKind:     "driver.dispatch",
				EffectState:    journal.Succeeded,
				Attempt:        1,
				Responsibility: "implementer",
				Transport:      "completed",
				Usage: []byte(
					`{"token_status":"reported","input_tokens":0,` +
						`"output_tokens":0,"cost_status":"reported",` +
						`"cost_micro_units":0,"currency":"USD",` +
						`"source":"provider_reported"}`,
				),
				StartedAt:  started,
				FinishedAt: started.Add(time.Second),
			},
			{
				Kind:           journal.EvaluationAttempt,
				EffectKind:     "driver.dispatch",
				EffectState:    journal.OperationalFailed,
				Attempt:        2,
				Responsibility: "verifier",
				Transport:      "timeout",
				Usage: []byte(
					`{"token_status":"unavailable","input_tokens":null,` +
						`"output_tokens":null,"cost_status":"unavailable",` +
						`"cost_micro_units":null,"currency":null,` +
						`"source":null}`,
				),
				StartedAt:  started,
				FinishedAt: finished,
			},
		},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID:      "run-1",
			Release: "release-1",
			State:   "running",
			Outcome: "pending",
		},
		ThroughOffset: 4,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
		Quality{
			Name:        "verification",
			Numerator:   &zero,
			Denominator: &zero,
		},
	)
	if err != nil || !changed {
		t.Fatalf("advance = %#v, %v, %v", record, changed, err)
	}
	if record.Attempts != 2 || record.Retries != 1 ||
		record.Events != 4 ||
		record.Recovery != (RecoverySummary{
			Uncertain: 1, Reconciled: 1, Recovered: 1,
		}) ||
		record.Usage.InputTokens == nil || *record.Usage.InputTokens != 0 ||
		record.Usage.OutputTokens == nil || *record.Usage.OutputTokens != 0 ||
		len(record.Usage.Costs) != 1 ||
		record.Usage.Costs[0] != (CostTotal{Currency: "USD", MicroUnits: 0}) ||
		*record.Usage.TokenCoverage.Numerator != 1 ||
		*record.Usage.TokenCoverage.Denominator != 2 ||
		*record.Quality.Numerator != 0 ||
		*record.Quality.Denominator != 0 {
		t.Fatalf("record = %#v", record)
	}
	if len(record.Groups) != 2 ||
		record.Groups[0].Responsibility != "implementer" ||
		record.Groups[1].Responsibility != "verifier" {
		t.Fatalf("groups = %#v", record.Groups)
	}
	if store.advance == nil ||
		store.advance.Observer != EvalObserver ||
		store.advance.ExpectedOffset != 0 ||
		store.advance.ThroughOffset != 4 ||
		len(store.advance.Eval) != 1 ||
		store.advance.Eval[0].ID != record.ID {
		t.Fatalf("advance = %#v", store.advance)
	}
	var persisted Record
	if err := json.Unmarshal(store.advance.Eval[0].Body, &persisted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, record) {
		t.Fatalf("persisted = %#v\nrecord = %#v", persisted, record)
	}

	again, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
		UnknownQuality("verification"),
	)
	if err != nil || changed || !reflect.DeepEqual(again, Record{}) {
		t.Fatalf("idempotent replay = %#v, %v, %v", again, changed, err)
	}
}

func TestEvaluatorPreservesUnknownUsageAndQualityAsNull(t *testing.T) {
	t.Parallel()

	started := time.Unix(1_700_000_000, 0).UTC()
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    started,
		},
		facts: []journal.EvaluationFact{
			{Kind: journal.EvaluationEvent, EventOffset: 1,
				EventKind: "runtime_progress", FinishedAt: started},
		},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run:           cockpit.RunView{ID: "run-1", State: "new"},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
		UnknownQuality("delivery"),
	)
	if err != nil || !changed {
		t.Fatalf("advance = %v, %v", changed, err)
	}
	if record.Usage.InputTokens != nil ||
		record.Usage.OutputTokens != nil ||
		record.Usage.Costs != nil ||
		record.Quality.Numerator != nil ||
		record.Quality.Denominator != nil ||
		*record.Usage.TokenCoverage.Numerator != 0 ||
		*record.Usage.TokenCoverage.Denominator != 0 {
		t.Fatalf("null preservation = %#v", record)
	}
	body := string(store.advance.Eval[0].Body)
	for _, expected := range []string{
		`"input_tokens":null`,
		`"output_tokens":null`,
		`"costs":null`,
		`"quality":{"name":"delivery","numerator":null,"denominator":null}`,
	} {
		if !contains(body, expected) {
			t.Fatalf("body missing %s: %s", expected, body)
		}
	}
}

func TestEvaluatorRetriesOnlyToAStableHighWater(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: now,
			},
			ThroughOffset: 2,
			ObservedAt:    now,
		},
		facts: []journal.EvaluationFact{
			{Kind: journal.EvaluationEvent, EventOffset: 1},
			{Kind: journal.EvaluationEvent, EventOffset: 2},
		},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{
		{Run: cockpit.RunView{ID: "run-1"}, ThroughOffset: 1},
		{Run: cockpit.RunView{ID: "run-1"}, ThroughOffset: 2},
	}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
		UnknownQuality("delivery"),
	); err != nil || !changed {
		t.Fatalf("stable retry = %v, %v", changed, err)
	}
}

func TestEvaluatorFailsClosedWithoutAdvancing(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	base := func() (*fakeEvaluationJournal, *fakeSnapshotProjector) {
		return &fakeEvaluationJournal{
				window: journal.EvaluationWindow{
					Run: journal.Run{
						ID: "run-1", Release: "release-1", CreatedAt: now,
					},
					ThroughOffset: 1,
					ObservedAt:    now,
				},
				facts: []journal.EvaluationFact{{
					Kind:           journal.EvaluationAttempt,
					EffectKind:     "driver.dispatch",
					EffectState:    journal.Succeeded,
					Attempt:        1,
					Responsibility: "planner",
					Transport:      "completed",
					Usage:          []byte(`{"not":"usage"}`),
					StartedAt:      now,
					FinishedAt:     now,
				}},
			}, &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
				Run: cockpit.RunView{ID: "run-1"}, ThroughOffset: 1,
			}}}
	}
	store, projector := base()
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := evaluator.Advance(
		context.Background(),
		"run-1",
		UnknownQuality("delivery"),
	); !IsCode(err, "INVALID_USAGE") || store.advance != nil {
		t.Fatalf("invalid usage = %v, advance=%#v", err, store.advance)
	}

	store, projector = base()
	store.facts = []journal.EvaluationFact{{
		Kind:        journal.EvaluationEvent,
		EventOffset: 1,
	}}
	store.err = errors.New("write unavailable")
	evaluator, _ = NewEvaluator(store, projector, "0.3.0-dev")
	if _, _, err := evaluator.Advance(
		context.Background(),
		"run-1",
		UnknownQuality("delivery"),
	); !IsCode(err, "JOURNAL_UNAVAILABLE") || store.cursor != 0 {
		t.Fatalf("atomic failure = %v, cursor=%d", err, store.cursor)
	}
}

func TestQualityRequiresBoundedPairedDenominators(t *testing.T) {
	t.Parallel()

	zero, one := int64(0), int64(1)
	for _, quality := range []Quality{
		{Name: "arbitrary"},
		{Name: "delivery", Numerator: &zero},
		{Name: "delivery", Denominator: &zero},
		{Name: "delivery", Numerator: &one, Denominator: &zero},
	} {
		if validateQuality(quality) == nil {
			t.Fatalf("accepted quality %#v", quality)
		}
	}
	if err := validateQuality(Quality{
		Name: "delivery", Numerator: &zero, Denominator: &zero,
	}); err != nil {
		t.Fatalf("0/0 quality = %v", err)
	}
}

func contains(value, pattern string) bool {
	for index := 0; index+len(pattern) <= len(value); index++ {
		if value[index:index+len(pattern)] == pattern {
			return true
		}
	}
	return false
}
