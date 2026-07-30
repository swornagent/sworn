package observe

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
)

func TestContinuationAggregationCoversTheExactClosedCardinality(
	t *testing.T,
) {
	t.Parallel()

	modes := []string{
		"fresh_rehydrate",
		"transcript_replay",
		"opaque_replay",
		"provider_cursor",
		"native_session",
		"compacted",
	}
	outcomes := []string{
		"reuse",
		"fallback",
		"fallback_expired",
	}
	var aggregate continuationAggregate
	for _, mode := range modes {
		for _, outcome := range outcomes {
			if err := aggregate.add(
				"dispatch_completed.continuation." + mode + "." + outcome,
			); err != nil {
				t.Fatalf("%s/%s: %v", mode, outcome, err)
			}
		}
	}
	summary := aggregate.summary()
	if summary.Reused != 6 || summary.Fallback != 12 ||
		summary.Expired != 6 || len(summary.Counts) != 18 ||
		!validContinuationSummary(summary, 18) {
		t.Fatalf("summary = %#v", summary)
	}
	for index, count := range summary.Counts {
		if count.Count != 1 ||
			(index != 0 && !continuationKeyLess(
				continuationKey{
					mode:    summary.Counts[index-1].Mode,
					outcome: summary.Counts[index-1].Outcome,
				},
				continuationKey{
					mode:    count.Mode,
					outcome: count.Outcome,
				},
			)) {
			t.Fatalf("non-canonical counts = %#v", summary.Counts)
		}
	}
}

func TestEvaluatorStripsEventBaseAndRejectsMalformedContinuationMarkers(
	t *testing.T,
) {
	t.Parallel()

	const sentinel = "PRIVATE_SESSION_CURSOR_CREDENTIAL"
	valid := evaluatorForContinuationEvent(
		t,
		sentinel+".continuation.native_session.reuse",
	)
	record, changed, err := valid.evaluator.Advance(
		context.Background(),
		"run-1",
	)
	if err != nil || !changed ||
		record.Continuation.Reused != 1 ||
		len(record.Continuation.Counts) != 1 ||
		record.Continuation.Counts[0] != (ContinuationCount{
			Mode:    "native_session",
			Outcome: "reuse",
			Count:   1,
		}) {
		t.Fatalf("valid continuation = %#v, %v, %v", record, changed, err)
	}
	body := valid.store.advance.Eval[0].Body
	if strings.Contains(string(body), sentinel) {
		t.Fatalf("event base escaped into eval: %s", body)
	}

	for _, kind := range []string{
		"continuation.provider_cursor.reuse",
		"dispatch_completed..continuation.provider_cursor.reuse",
		"dispatch_completed.continuation.provider_cursor",
		"dispatch_completed.continuation.PRIVATE_MODE.reuse",
		"dispatch_completed.continuation.provider_cursor.PRIVATE_OUTCOME",
		"dispatch_completed.continuation.provider_cursor.reuse.extra",
		"dispatch_completed.continuation.provider_cursor.reuse." +
			"continuation.native_session.reuse",
	} {
		fixture := evaluatorForContinuationEvent(t, kind)
		if _, _, err := fixture.evaluator.Advance(
			context.Background(),
			"run-1",
		); !IsCode(err, "INVALID_EVALUATION_FACT") ||
			fixture.store.advance != nil {
			t.Errorf("%q = %v, advance=%#v", kind, err, fixture.store.advance)
		}
	}
}

type continuationEvaluatorFixture struct {
	evaluator *Evaluator
	store     *fakeEvaluationJournal
}

func evaluatorForContinuationEvent(
	t *testing.T,
	kind string,
) continuationEvaluatorFixture {
	t.Helper()
	now := time.Unix(1_700_000_000, 0).UTC()
	store := &fakeEvaluationJournal{
		window: journal.EvaluationWindow{
			Run: journal.Run{
				ID:        "run-1",
				Release:   "release-1",
				CreatedAt: now,
			},
			ThroughOffset: 1,
			ObservedAt:    now,
		},
		facts: []journal.EvaluationFact{{
			Kind:        journal.EvaluationEvent,
			EventOffset: 1,
			EventKind:   kind,
			FinishedAt:  now,
		}},
	}
	projector := &fakeSnapshotProjector{snapshots: []cockpit.Snapshot{{
		Run: cockpit.RunView{
			ID:      "run-1",
			Release: "release-1",
			State:   "running",
		},
		ThroughOffset: 1,
	}}}
	evaluator, err := NewEvaluator(store, projector, "0.3.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	return continuationEvaluatorFixture{evaluator: evaluator, store: store}
}

func TestContinuationSummaryJSONContainsOnlyClosedFacts(t *testing.T) {
	t.Parallel()

	const sentinel = "PRIVATE_TRANSCRIPT_REASONING_PROVIDER_CURSOR"
	var aggregate continuationAggregate
	if err := aggregate.add(
		sentinel + ".continuation.provider_cursor.fallback_expired",
	); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(aggregate.summary())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), sentinel) ||
		string(body) !=
			`{"reused":0,"fallback":1,"expired":1,`+
				`"counts":[{"mode":"provider_cursor",`+
				`"outcome":"fallback_expired","count":1}]}` {
		t.Fatalf("summary JSON = %s", body)
	}
}
