package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
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
				EventKind: "dispatch_completed.continuation." +
					"provider_cursor.reuse",
				FinishedAt: finished},
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
				Responsibility: "implementer_implementation",
				Transport:      "completed",
				Usage: []byte(
					`{"token_status":"reported","input_tokens":0,` +
						`"output_tokens":0,"cost_status":"reported",` +
						`"cost_micro_units":0,"currency":"USD",` +
						`"source":"provider_reported",` +
						`"cache_status":"reported","cache_read_tokens":8,` +
						`"cache_write_tokens":2,` +
						`"effort_requested":"high",` +
						`"effort_reported":"high"}`,
				),
				StartedAt:  started,
				FinishedAt: started.Add(time.Second),
			},
			{
				Kind:           journal.EvaluationAttempt,
				EffectKind:     "driver.dispatch",
				EffectState:    journal.OperationalFailed,
				Attempt:        2,
				Responsibility: "work_verification",
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
		Graph: cockpit.Graph{Nodes: []cockpit.Node{
			{ID: "slice:1", Kind: "slice", Outcome: "pass"},
			{ID: "slice:2", Kind: "slice", Outcome: "pending"},
			{ID: "assembly", Kind: "assembly", Outcome: "pass"},
		}},
		ThroughOffset: 4,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
	)
	if err != nil || !changed {
		t.Fatalf("advance = %#v, %v, %v", record, changed, err)
	}
	if record.Attempts != 2 || record.Retries != 1 ||
		record.Events != 4 ||
		record.Recovery != (RecoverySummary{
			Uncertain: 1, Reconciled: 1, Recovered: 1,
		}) ||
		!reflect.DeepEqual(
			record.Continuation,
			ContinuationSummary{
				Reused: 1,
				Counts: []ContinuationCount{{
					Mode:    "provider_cursor",
					Outcome: "reuse",
					Count:   1,
				}},
			},
		) ||
		record.Usage.InputTokens == nil || *record.Usage.InputTokens != 0 ||
		record.Usage.OutputTokens == nil || *record.Usage.OutputTokens != 0 ||
		len(record.Usage.Costs) != 1 ||
		record.Usage.Costs[0] != (CostTotal{
			Currency:   "USD",
			MicroUnits: 0,
			Source:     driver.CostSourceProviderReported,
		}) ||
		*record.Usage.TokenCoverage.Numerator != 1 ||
		*record.Usage.TokenCoverage.Denominator != 2 ||
		record.Usage.CacheReadTokens == nil ||
		*record.Usage.CacheReadTokens != 8 ||
		record.Usage.CacheWriteTokens == nil ||
		*record.Usage.CacheWriteTokens != 2 ||
		*record.Usage.CacheCoverage.Numerator != 1 ||
		*record.Usage.CacheCoverage.Denominator != 2 ||
		record.Usage.EffortRequested == nil ||
		*record.Usage.EffortRequested != "high" ||
		record.Usage.EffortReported == nil ||
		*record.Usage.EffortReported != "high" ||
		record.Usage.FinishReason != nil ||
		record.Usage.Truncated != nil ||
		len(record.Quality) != 4 ||
		record.Quality[3].Name != "verification" {
		t.Fatalf("record = %#v", record)
	}
	if len(record.Groups) != 2 ||
		record.Groups[0].Role != "implementer" ||
		record.Groups[0].Responsibility != "implementer_implementation" ||
		record.Groups[1].Role != "verifier" ||
		record.Groups[1].Responsibility != "work_verification" {
		t.Fatalf("groups = %#v", record.Groups)
	}
	groupUsage := record.Groups[0].Usage
	if groupUsage.CacheReadTokens == nil ||
		*groupUsage.CacheReadTokens != 8 ||
		groupUsage.CacheWriteTokens == nil ||
		*groupUsage.CacheWriteTokens != 2 ||
		*groupUsage.CacheCoverage.Numerator != 1 ||
		*groupUsage.CacheCoverage.Denominator != 1 ||
		groupUsage.EffortRequested == nil ||
		*groupUsage.EffortRequested != "high" ||
		groupUsage.EffortReported == nil ||
		*groupUsage.EffortReported != "high" {
		t.Fatalf("group usage = %#v", groupUsage)
	}
	verifierUsage := record.Groups[1].Usage
	if verifierUsage.CacheReadTokens != nil ||
		verifierUsage.CacheWriteTokens != nil ||
		*verifierUsage.CacheCoverage.Numerator != 0 ||
		*verifierUsage.CacheCoverage.Denominator != 1 ||
		verifierUsage.EffortRequested != nil ||
		verifierUsage.EffortReported != nil {
		t.Fatalf("verifier usage = %#v", verifierUsage)
	}
	if *record.Quality[0].Numerator != 1 ||
		*record.Quality[0].Denominator != 2 ||
		*record.Quality[1].Numerator != 1 ||
		*record.Quality[1].Denominator != 1 ||
		record.Quality[2].Numerator != nil ||
		record.Quality[2].Denominator != nil ||
		*record.Quality[3].Numerator != 1 ||
		*record.Quality[3].Denominator != 1 {
		t.Fatalf("quality = %#v", record.Quality)
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
	// S2-C1: the live record carries the unexported dispatch-span carrier,
	// which encoding/json skips. The durable body is pinned against the
	// carrier-cleared record, and the bytes themselves are pinned to the
	// sworn.eval/v3 exported schema (no new field, no version bump).
	cleared := record
	cleared.dispatchAttempts = nil
	cleared.spanJoin = spanJoin{}
	if !reflect.DeepEqual(persisted, cleared) {
		t.Fatalf("persisted = %#v\nrecord = %#v", persisted, cleared)
	}
	strict := json.NewDecoder(bytes.NewReader(store.advance.Eval[0].Body))
	strict.DisallowUnknownFields()
	var strictlyPersisted Record
	if err := strict.Decode(&strictlyPersisted); err != nil {
		t.Fatalf("persisted body carries a field outside sworn.eval/v3: %v", err)
	}
	if strictlyPersisted.SchemaVersion != EvalSchemaVersion {
		t.Fatalf("persisted schema = %q, want %q",
			strictlyPersisted.SchemaVersion, EvalSchemaVersion)
	}

	again, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
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
		Run: cockpit.RunView{
			ID: "run-1", Release: "release-1", State: "new",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
	)
	if err != nil || !changed {
		t.Fatalf("advance = %v, %v", changed, err)
	}
	if record.Usage.InputTokens != nil ||
		record.Usage.OutputTokens != nil ||
		record.Usage.Costs != nil ||
		record.Quality[2].Numerator != nil ||
		record.Quality[2].Denominator != nil ||
		*record.Usage.TokenCoverage.Numerator != 0 ||
		*record.Usage.TokenCoverage.Denominator != 0 {
		t.Fatalf("null preservation = %#v", record)
	}
	body := string(store.advance.Eval[0].Body)
	for _, expected := range []string{
		`"input_tokens":null`,
		`"output_tokens":null`,
		`"costs":null`,
		`"quality":[{"name":"delivery","numerator":0,"denominator":0}`,
		`{"name":"requirements","numerator":null,"denominator":null}`,
	} {
		if !contains(body, expected) {
			t.Fatalf("body missing %s: %s", expected, body)
		}
	}
}

func TestEvaluatorDoesNotDoubleCountTurnRecoveryAsGenericRecovery(
	t *testing.T,
) {
	t.Parallel()

	started := time.Unix(1_700_000_000, 0).UTC()
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    started.Add(time.Second),
		},
		facts: []journal.EvaluationFact{{
			Kind:        journal.EvaluationEvent,
			EventOffset: 1,
			EventKind:   "turn_recovery.outcome.recovered",
			FinishedAt:  started.Add(time.Second),
		}},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-1", Release: "release-1", State: "running",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
	)
	if err != nil || !changed ||
		record.TurnRecovery.Recovered != 1 ||
		record.Recovery.Recovered != 0 {
		t.Fatalf(
			"turn recovery classification changed=%t record=%#v error=%v",
			changed,
			record,
			err,
		)
	}
}

func TestGraphQualityUsesCurrentTruthAndKeepsUnknownRequirementsNull(
	t *testing.T,
) {
	t.Parallel()

	inProgress := graphQuality(cockpit.Graph{Nodes: []cockpit.Node{
		{Kind: "slice", Outcome: "pending"},
		{Kind: "slice", Outcome: "pending"},
		{Kind: "assembly", Outcome: "pending"},
	}})
	if *inProgress[0].Numerator != 0 ||
		*inProgress[0].Denominator != 2 ||
		*inProgress[1].Numerator != 0 ||
		*inProgress[1].Denominator != 0 ||
		inProgress[2].Numerator != nil ||
		inProgress[2].Denominator != nil ||
		*inProgress[3].Numerator != 0 ||
		*inProgress[3].Denominator != 0 {
		t.Fatalf("in-progress quality = %#v", inProgress)
	}
	repaired := graphQuality(cockpit.Graph{Nodes: []cockpit.Node{
		{Kind: "slice", Outcome: "pass"},
		{Kind: "slice", Outcome: "pass"},
		{Kind: "assembly", Outcome: "merged"},
	}})
	if *repaired[0].Numerator != 2 ||
		*repaired[0].Denominator != 2 ||
		*repaired[1].Numerator != 1 ||
		*repaired[1].Denominator != 1 ||
		*repaired[3].Numerator != 2 ||
		*repaired[3].Denominator != 2 {
		t.Fatalf("repaired quality = %#v", repaired)
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
		{Run: cockpit.RunView{ID: "run-1", Release: "release-1"}, ThroughOffset: 1},
		{Run: cockpit.RunView{ID: "run-1", Release: "release-1"}, ThroughOffset: 2},
	}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
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
					Responsibility: "planner_proposal",
					Transport:      "completed",
					Usage:          []byte(`{"not":"usage"}`),
					StartedAt:      now,
					FinishedAt:     now,
				}},
			}, &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
				Run: cockpit.RunView{
					ID: "run-1", Release: "release-1",
				},
				ThroughOffset: 1,
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
	); !IsCode(err, "JOURNAL_UNAVAILABLE") || store.cursor != 0 {
		t.Fatalf("atomic failure = %v, cursor=%d", err, store.cursor)
	}
}

func TestEvaluatorRejectsMismatchedCockpitBinding(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: now,
			},
			ThroughOffset: 1,
			ObservedAt:    now,
		},
		facts: []journal.EvaluationFact{{
			Kind: journal.EvaluationEvent, EventOffset: 1,
		}},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-1", Release: "substituted-release",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := evaluator.Advance(
		context.Background(),
		"run-1",
	); !IsCode(err, "SNAPSHOT_MISMATCH") || store.advance != nil {
		t.Fatalf("binding mismatch = %v, advance=%#v", err, store.advance)
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

// A7: reasoning_tokens rides the same path cache reads take into the eval
// record: a usage receipt carrying Gemini's thoughtsTokenCount lands on the
// record and group usage summaries, while a legacy receipt without it stays
// byte-identical through the canonical re-encode.
func TestEvaluatorSurfacesReasoningTokens(t *testing.T) {
	t.Parallel()

	started := time.Unix(1_700_000_000, 0).UTC()
	finished := started.Add(2 * time.Second)
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    finished,
		},
		facts: []journal.EvaluationFact{{
			Kind:           journal.EvaluationAttempt,
			EffectKind:     "driver.dispatch",
			EffectState:    journal.Succeeded,
			Attempt:        1,
			Responsibility: "implementer_implementation",
			Transport:      "completed",
			Usage: []byte(
				`{"token_status":"reported","input_tokens":15912,` +
					`"output_tokens":4,"cost_status":"unavailable",` +
					`"cost_micro_units":null,"currency":null,` +
					`"source":null,"cache_status":"reported",` +
					`"cache_read_tokens":12263,` +
					`"reasoning_tokens":1779}`,
			),
			StartedAt:  started,
			FinishedAt: finished,
		}},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-1", Release: "release-1", State: "running",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
	)
	if err != nil || !changed {
		t.Fatalf("advance = %v, %v", changed, err)
	}
	usage := record.Usage
	if usage.ReasoningTokens == nil || *usage.ReasoningTokens != 1779 ||
		usage.CacheReadTokens == nil || *usage.CacheReadTokens != 12263 ||
		*usage.CacheCoverage.Numerator != 1 ||
		*usage.CacheCoverage.Denominator != 1 {
		t.Fatalf("reasoning usage = %#v", usage)
	}
	if len(record.Groups) != 1 ||
		record.Groups[0].Usage.ReasoningTokens == nil ||
		*record.Groups[0].Usage.ReasoningTokens != 1779 {
		t.Fatalf("group reasoning usage = %#v", record.Groups)
	}
	if store.advance == nil || len(store.advance.Eval) != 1 ||
		!contains(
			string(store.advance.Eval[0].Body),
			`"reasoning_tokens":1779`,
		) {
		t.Fatalf("persisted body missing reasoning_tokens: %s",
			store.advance.Eval[0].Body)
	}

	// A legacy usage receipt (no reasoning_tokens) still re-encodes
	// byte-identically through the observe decode path.
	legacy := []byte(
		`{"token_status":"reported","input_tokens":7,"output_tokens":5,` +
			`"cost_status":"unavailable","cost_micro_units":null,"currency":null,` +
			`"source":null,"cache_status":"reported","cache_read_tokens":40,` +
			`"effort_requested":"high","effort_reported":"high",` +
			`"finish_reason":"length","truncated":true}`,
	)
	decoded, decodeErr := decodeUsage(legacy)
	if decodeErr != nil || decoded.ReasoningTokens != nil {
		t.Fatalf("legacy decode = %#v, %v", decoded, decodeErr)
	}
	canonical, encodeErr := driver.EncodeUsageReceipt(decoded)
	if encodeErr != nil || !bytes.Equal(canonical, legacy) {
		t.Fatalf("legacy re-encode = %s, %v", canonical, encodeErr)
	}
}

func TestEvaluatorSurfacesProviderTruncationFacts(t *testing.T) {
	t.Parallel()

	started := time.Unix(1_700_000_000, 0).UTC()
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    started.Add(time.Second),
		},
		facts: []journal.EvaluationFact{{
			Kind:           journal.EvaluationAttempt,
			EffectKind:     "driver.dispatch",
			EffectState:    journal.OperationalFailed,
			Attempt:        1,
			Responsibility: "implementer_implementation",
			Transport:      "runner_error",
			Usage: []byte(
				`{"token_status":"reported","input_tokens":4,` +
					`"output_tokens":3,"cost_status":"unavailable",` +
					`"cost_micro_units":null,"currency":null,` +
					`"source":null,"cache_status":"reported",` +
					`"cache_read_tokens":2,"cache_write_tokens":2,` +
					`"effort_requested":"high","finish_reason":"length",` +
					`"truncated":true}`,
			),
			StartedAt:  started,
			FinishedAt: started.Add(time.Second),
		}},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-1", Release: "release-1", State: "running",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(
		context.Background(),
		"run-1",
	)
	if err != nil || !changed {
		t.Fatalf("advance = %v, %v", changed, err)
	}
	usage := record.Usage
	if usage.FinishReason == nil || *usage.FinishReason != "length" ||
		usage.Truncated == nil || !*usage.Truncated ||
		usage.EffortRequested == nil || *usage.EffortRequested != "high" ||
		usage.CacheReadTokens == nil || *usage.CacheReadTokens != 2 ||
		usage.CacheWriteTokens == nil || *usage.CacheWriteTokens != 2 ||
		*usage.CacheCoverage.Numerator != 1 ||
		*usage.CacheCoverage.Denominator != 1 {
		t.Fatalf("truncation usage = %#v", usage)
	}
	if len(record.Groups) != 1 {
		t.Fatalf("groups = %#v", record.Groups)
	}
	group := record.Groups[0].Usage
	if group.FinishReason == nil || *group.FinishReason != "length" ||
		group.Truncated == nil || !*group.Truncated ||
		group.CacheReadTokens == nil || *group.CacheReadTokens != 2 {
		t.Fatalf("group truncation usage = %#v", group)
	}
}

// S2 carrier seam: the evaluator carries one entry per attempt fact in
// journal order (including non-dispatch effects, which the span builder
// later filters), plus the bounded snapshot join window the dispatch span
// needs for effect identity. The carrier is unexported and never enters
// the durable body (pinned by the strict decode in
// TestEvaluatorPersistsCanonicalCumulativeRecord).
func TestEvaluatorCarriesDispatchAttemptCarrierInJournalOrder(t *testing.T) {
	t.Parallel()

	started := time.Unix(1_700_000_000, 0).UTC()
	finished := started.Add(2 * time.Second)
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    finished,
		},
		facts: []journal.EvaluationFact{
			{
				Kind:           journal.EvaluationAttempt,
				EffectKind:     "driver.dispatch",
				EffectState:    journal.Succeeded,
				Attempt:        1,
				Responsibility: "implementer_implementation",
				Transport:      "completed",
				Usage: []byte(
					`{"token_status":"reported","input_tokens":7,` +
						`"output_tokens":3,"cost_status":"unavailable",` +
						`"cost_micro_units":null,"currency":null,` +
						`"source":null}`,
				),
				StartedAt:  started,
				FinishedAt: started.Add(time.Second),
			},
			{
				Kind:           journal.EvaluationAttempt,
				EffectKind:     "git.seal",
				EffectState:    journal.Pending,
				Attempt:        1,
				Responsibility: "implementer_implementation",
				Transport:      "completed",
				Usage: []byte(
					`{"token_status":"unavailable","input_tokens":null,` +
						`"output_tokens":null,"cost_status":"unavailable",` +
						`"cost_micro_units":null,"currency":null,"source":null}`,
				),
				StartedAt:  started,
				FinishedAt: started.Add(time.Second),
			},
			{
				Kind:           journal.EvaluationAttempt,
				EffectKind:     "driver.dispatch",
				EffectState:    journal.OperationalFailed,
				Attempt:        2,
				Responsibility: "work_verification",
				Transport:      "timeout",
				Usage: []byte(
					`{"token_status":"unavailable","input_tokens":null,` +
						`"output_tokens":null,"cost_status":"unavailable",` +
						`"cost_micro_units":null,"currency":null,"source":null}`,
				),
				StartedAt:  started,
				FinishedAt: finished,
			},
		},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-1", Release: "release-1", State: "running",
		},
		Runtime: cockpit.RuntimeView{
			Attempts: []cockpit.AttemptView{
				{
					EffectID:       "attempt/work-a/e2/t3",
					Number:         1,
					Responsibility: "implementer_implementation",
					Transport:      "completed",
					CreatedAt:      started.Add(time.Second),
				},
				{
					EffectID:       "attempt/work-b/e1/t1",
					Number:         2,
					Responsibility: "work_verification",
					Transport:      "timeout",
					CreatedAt:      finished,
				},
			},
		},
		Evidence: []cockpit.Evidence{
			{
				EffectID:  "attempt/work-a/e2/t3",
				WorkID:    "work-a",
				Slice:     "S2-genai-spans",
				CreatedAt: started.Add(time.Second),
			},
		},
		Graph: cockpit.Graph{Nodes: []cockpit.Node{
			{
				ID: "slice:S2-genai-spans", Kind: "slice",
				Track: "T1-telemetry",
			},
		}},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(context.Background(), "run-1")
	if err != nil || !changed {
		t.Fatalf("advance = %t, %v", changed, err)
	}
	if len(record.dispatchAttempts) != 3 {
		t.Fatalf("carrier = %#v", record.dispatchAttempts)
	}
	if record.dispatchAttempts[0].effectKind != "driver.dispatch" ||
		record.dispatchAttempts[1].effectKind != "git.seal" ||
		record.dispatchAttempts[2].effectKind != "driver.dispatch" {
		t.Fatalf("carrier journal order = %#v", record.dispatchAttempts)
	}
	if record.dispatchAttempts[0].effectState != "succeeded" ||
		record.dispatchAttempts[0].responsibility !=
			"implementer_implementation" ||
		record.dispatchAttempts[0].attempt != 1 ||
		record.dispatchAttempts[0].transport != "completed" ||
		record.dispatchAttempts[0].startedAt != started ||
		record.dispatchAttempts[0].finishedAt != started.Add(time.Second) {
		t.Fatalf("carrier entry = %#v", record.dispatchAttempts[0])
	}
	if _, err := decodeUsage(record.dispatchAttempts[0].usageBytes); err != nil {
		t.Fatalf("carrier usage bytes = %v", err)
	}
	if record.dispatchAttempts[2].effectState != "operational_failed" ||
		record.dispatchAttempts[2].transport != "timeout" ||
		record.dispatchAttempts[2].attempt != 2 {
		t.Fatalf("carrier entry = %#v", record.dispatchAttempts[2])
	}
	if len(record.spanJoin.attempts) != 2 ||
		len(record.spanJoin.evidence) != 1 ||
		record.spanJoin.sliceTracks["slice:S2-genai-spans"] !=
			"T1-telemetry" {
		t.Fatalf("span join = %#v", record.spanJoin)
	}
}

// A3: an aggregate over mixed coverage is never rendered as a bare total.
// Coverage rides in-band and the non-reporting surfaces are named, at the
// record level and every per-role group level.
func TestEvalCoverageNamesUnreportedSurfaces(t *testing.T) {
	t.Parallel()
	started := time.Unix(1_700_000_000, 0).UTC()
	finished := started.Add(time.Second)
	reported := driver.UsageReceipt{
		SchemaVersion: driver.UsageSchemaV2,
		Surface:       "sworn.reporting",
		TokenStatus:   driver.UsageReported,
		InputTokens:   int64Pointer(7),
		OutputTokens:  int64Pointer(3),
		CostStatus:    driver.UsageUnavailable,
		CacheStatus:   driver.UsageUnavailable,
	}
	reportedBody, err := driver.EncodeUsageReceipt(reported)
	if err != nil {
		t.Fatal(err)
	}
	facts := []journal.EvaluationFact{}
	unreported := []string{
		"sworn.s1", "sworn.s2", "sworn.s3", "sworn.s4", "sworn.s5",
		"sworn.s6", "sworn.s7",
	}
	for index := 0; index < 10; index++ {
		var body []byte
		if index < 3 {
			body = reportedBody
		} else {
			loud := reported
			loud.TokenStatus = driver.UsageUnavailable
			loud.InputTokens = nil
			loud.OutputTokens = nil
			loud.Surface = unreported[index-3]
			loud.UnavailableReason = driver.UsageReasonWireLacked
			body, err = driver.EncodeUsageReceipt(loud)
			if err != nil {
				t.Fatal(err)
			}
		}
		facts = append(facts, journal.EvaluationFact{
			Kind:           journal.EvaluationAttempt,
			EffectKind:     "driver.dispatch",
			EffectState:    journal.Succeeded,
			Attempt:        int64(index + 1),
			Responsibility: "implementer_implementation",
			Transport:      "completed",
			Usage:          body,
			StartedAt:      started,
			FinishedAt:     finished,
		})
	}
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    finished,
		},
		facts: facts,
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-1", Release: "release-1", State: "running",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(context.Background(), "run-1")
	if err != nil || !changed {
		t.Fatalf("advance = %t, %v", changed, err)
	}
	if *record.Usage.TokenCoverage.Numerator != 3 ||
		*record.Usage.TokenCoverage.Denominator != 10 {
		t.Fatalf("record coverage = %#v", record.Usage.TokenCoverage)
	}
	if len(record.Usage.UnreportedSurfaces) != 7 {
		t.Fatalf("record surfaces = %#v", record.Usage.UnreportedSurfaces)
	}
	for index, want := range unreported {
		if record.Usage.UnreportedSurfaces[index] != want {
			t.Fatalf("record surfaces = %#v", record.Usage.UnreportedSurfaces)
		}
	}
	if len(record.Groups) != 1 {
		t.Fatalf("groups = %#v", record.Groups)
	}
	groupUsage := record.Groups[0].Usage
	if *groupUsage.TokenCoverage.Numerator != 3 ||
		*groupUsage.TokenCoverage.Denominator != 10 ||
		len(groupUsage.UnreportedSurfaces) != 7 {
		t.Fatalf("group usage = %#v", groupUsage)
	}

	// A legacy silent blob contributes the honest literal "unknown".
	legacyStore := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-legacy", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    finished,
		},
		facts: []journal.EvaluationFact{{
			Kind:           journal.EvaluationAttempt,
			EffectKind:     "driver.dispatch",
			EffectState:    journal.OperationalFailed,
			Attempt:        1,
			Responsibility: "work_verification",
			Transport:      "timeout",
			Usage: []byte(
				`{"token_status":"unavailable","input_tokens":null,` +
					`"output_tokens":null,"cost_status":"unavailable",` +
					`"cost_micro_units":null,"currency":null,"source":null}`,
			),
			StartedAt:  started,
			FinishedAt: finished,
		}},
	}
	legacyProjector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-legacy", Release: "release-1", State: "running",
		},
		ThroughOffset: 1,
	}}}
	legacyEvaluator, err := NewEvaluator(
		legacyStore,
		legacyProjector,
		"0.3.0-dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	legacyRecord, changed, err := legacyEvaluator.Advance(
		context.Background(),
		"run-legacy",
	)
	if err != nil || !changed ||
		len(legacyRecord.Usage.UnreportedSurfaces) != 1 ||
		legacyRecord.Usage.UnreportedSurfaces[0] != "unknown" {
		t.Fatalf("legacy naming = %#v, %t, %v", legacyRecord.Usage, changed, err)
	}
}

// A4+A5+correction 1: two models dispatched under one role stay truthful and
// distinct. Duration, profile, certified model, cost source, and turn
// economics land in the eval record, and every group's metric series
// survives the projection with profile/model in the label set.
func TestEvalRecordCarriesAttemptFactsAndKeepsMixedModelGroupsDistinct(
	t *testing.T,
) {
	t.Parallel()
	started := time.Unix(1_700_000_000, 0).UTC()
	finished := started.Add(time.Second)
	buildReceipt := func(
		surface, profile, model string,
		durationMillis, turns, toolCalls int64,
		mix []driver.ToolCallCount,
	) []byte {
		t.Helper()
		receipt := driver.UsageReceipt{
			SchemaVersion:   driver.UsageSchemaV2,
			Surface:         surface,
			TokenStatus:     driver.UsageReported,
			InputTokens:     int64Pointer(7),
			OutputTokens:    int64Pointer(3),
			CostStatus:      driver.UsageReported,
			CostMicroUnits:  int64Pointer(10),
			Currency:        textPointer("USD"),
			Source:          textPointer(driver.CostSourceProviderReported),
			CacheStatus:     driver.UsageUnavailable,
			DurationMillis:  int64Pointer(durationMillis),
			Profile:         textPointer(profile),
			Model:           textPointer(model),
			Turns:           int64Pointer(turns),
			ToolCalls:       int64Pointer(toolCalls),
			ToolCallsByName: mix,
		}
		body, err := driver.EncodeUsageReceipt(receipt)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	mixA := []driver.ToolCallCount{
		{Name: "Bash", Count: 1},
		{Name: "Read", Count: 2},
	}
	mixB := []driver.ToolCallCount{
		{Name: "Edit", Count: 2},
	}
	facts := []journal.EvaluationFact{
		{
			Kind: journal.EvaluationAttempt, EffectKind: "driver.dispatch",
			EffectState: journal.Succeeded, Attempt: 1,
			Responsibility: "implementer_implementation",
			Transport:      "completed",
			Usage: buildReceipt(
				"sworn.a", "profile-p1", "model-m1", 100, 2, 3, mixA,
			),
			StartedAt: started, FinishedAt: finished,
		},
		{
			Kind: journal.EvaluationAttempt, EffectKind: "driver.dispatch",
			EffectState: journal.Succeeded, Attempt: 2,
			Responsibility: "implementer_implementation",
			Transport:      "completed",
			Usage: buildReceipt(
				"sworn.a", "profile-p1", "model-m1", 200, 2, 3, mixA,
			),
			StartedAt: started, FinishedAt: finished,
		},
		{
			Kind: journal.EvaluationAttempt, EffectKind: "driver.dispatch",
			EffectState: journal.Succeeded, Attempt: 3,
			Responsibility: "implementer_implementation",
			Transport:      "completed",
			Usage: buildReceipt(
				"sworn.b", "profile-p2", "model-m2", 40, 4, 2, mixB,
			),
			StartedAt: started, FinishedAt: finished,
		},
		{
			Kind: journal.EvaluationAttempt, EffectKind: "driver.dispatch",
			EffectState: journal.Succeeded, Attempt: 4,
			Responsibility: "implementer_implementation",
			Transport:      "completed",
			Usage: buildReceipt(
				"sworn.b", "profile-p2", "model-m2", 60, 4, 2, mixB,
			),
			StartedAt: started, FinishedAt: finished,
		},
	}
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID: "run-1", Release: "release-1", CreatedAt: started,
			},
			ThroughOffset: 1,
			ObservedAt:    finished,
		},
		facts: facts,
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID: "run-1", Release: "release-1", State: "running",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	record, changed, err := evaluator.Advance(context.Background(), "run-1")
	if err != nil || !changed {
		t.Fatalf("advance = %t, %v", changed, err)
	}
	if record.SchemaVersion != journal.EvalSchemaVersionV3 {
		t.Fatalf("schema = %q", record.SchemaVersion)
	}
	if len(record.Groups) != 2 {
		t.Fatalf("groups = %#v", record.Groups)
	}
	var groupOne, groupTwo AttemptGroup
	for _, group := range record.Groups {
		switch group.Profile {
		case "profile-p1":
			groupOne = group
		case "profile-p2":
			groupTwo = group
		}
	}
	if groupOne.Model != "model-m1" || groupTwo.Model != "model-m2" {
		t.Fatalf("group identity = %#v", record.Groups)
	}
	if groupOne.ObservationDurationNS.Numerator == nil ||
		*groupOne.ObservationDurationNS.Numerator != 300*1_000_000 ||
		*groupOne.ObservationDurationNS.Denominator != 2 ||
		groupTwo.ObservationDurationNS.Numerator == nil ||
		*groupTwo.ObservationDurationNS.Numerator != 100*1_000_000 ||
		*groupTwo.ObservationDurationNS.Denominator != 2 {
		t.Fatalf("observation duration = %#v / %#v",
			groupOne.ObservationDurationNS,
			groupTwo.ObservationDurationNS,
		)
	}
	if len(groupOne.Usage.Costs) != 1 ||
		groupOne.Usage.Costs[0].Source !=
			driver.CostSourceProviderReported {
		t.Fatalf("cost source = %#v", groupOne.Usage.Costs)
	}
	if groupOne.TurnEconomics.Turns == nil ||
		*groupOne.TurnEconomics.Turns != 4 ||
		groupOne.TurnEconomics.ToolCalls == nil ||
		*groupOne.TurnEconomics.ToolCalls != 6 ||
		*groupOne.TurnEconomics.ToolCallsPerTurn.Numerator != 6 ||
		*groupOne.TurnEconomics.ToolCallsPerTurn.Denominator != 4 ||
		len(groupOne.TurnEconomics.ToolCallMix) != 2 ||
		groupOne.TurnEconomics.ToolCallMix[0] !=
			(ToolCallCount{Name: "Bash", Count: 2}) ||
		groupOne.TurnEconomics.ToolCallMix[1] !=
			(ToolCallCount{Name: "Read", Count: 4}) {
		t.Fatalf("turn economics = %#v", groupOne.TurnEconomics)
	}
	if groupTwo.TurnEconomics.Turns == nil ||
		*groupTwo.TurnEconomics.Turns != 8 ||
		groupTwo.TurnEconomics.ToolCalls == nil ||
		*groupTwo.TurnEconomics.ToolCalls != 4 ||
		len(groupTwo.TurnEconomics.ToolCallMix) != 1 ||
		groupTwo.TurnEconomics.ToolCallMix[0] !=
			(ToolCallCount{Name: "Edit", Count: 4}) {
		t.Fatalf("turn economics two = %#v", groupTwo.TurnEconomics)
	}

	// Correction 1: every group's metric series survives the projection.
	traceExporter := &captureTraceExporter{started: make(chan struct{})}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		4,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(record) {
		t.Fatal("record was not accepted")
	}
	waitForProcessed(t, telemetry, 1)
	shutdownTelemetry(t, telemetry)
	metrics, _ := metricExporter.snapshot()
	profileLabels := map[string]bool{"profile-p1": false, "profile-p2": false}
	for _, metric := range metrics {
		if metric.name != "sworn.eval.attempts" {
			continue
		}
		profile, ok := metric.attributes["sworn.profile"].(string)
		if ok {
			if _, exists := profileLabels[profile]; exists {
				profileLabels[profile] = true
			}
		}
	}
	if !profileLabels["profile-p1"] || !profileLabels["profile-p2"] {
		t.Fatalf("mixed-model group series collapsed: %#v", metrics)
	}
}
