package control

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/store"
)

type serviceJournal struct {
	calls       *[]string
	unknown     []engine.JournalEffect
	prepared    engine.JournalEffect
	interrupted int
	failAt      string
}

func (journal *serviceJournal) record(call string) error {
	*journal.calls = append(*journal.calls, call)
	if journal.failAt == call {
		return errors.New(call + " failed")
	}
	return nil
}

func (journal *serviceJournal) BindEffectResult(
	_ context.Context, _ store.EffectLease, _ json.RawMessage,
) error {
	return journal.record("bind")
}

func (journal *serviceJournal) PrepareNativeBuildExecution(
	_ context.Context,
	lease store.EffectLease,
) (engine.JournalEffect, error) {
	if err := journal.record("prepare-execution"); err != nil {
		return engine.JournalEffect{}, err
	}
	if journal.prepared.ID != "" {
		return journal.prepared, nil
	}
	return lease.Invocation(), nil
}

func (journal *serviceJournal) CompleteEffect(context.Context, store.EffectLease) error {
	return journal.record("complete")
}

func (journal *serviceJournal) RecoverInterruptedEffects(context.Context, string) (int, error) {
	return journal.interrupted, journal.record("interrupt")
}

func (journal *serviceJournal) UnknownEffects(context.Context) ([]engine.JournalEffect, error) {
	if err := journal.record("unknown"); err != nil {
		return nil, err
	}
	return journal.unknown, nil
}

func (journal *serviceJournal) RecoverBoundEffect(
	_ context.Context, effectID string, _ int64, _ string,
) error {
	return journal.record("recover-bound:" + effectID)
}

func (journal *serviceJournal) RecoverUnboundBuildEffect(
	_ context.Context, _ store.BuildRecoveryLease, _ string, _ effects.BuildRetryProof,
) error {
	return journal.record("retry")
}

func (journal *serviceJournal) PrepareUnboundBuildRecovery(
	context.Context, string, int64,
) (store.BuildRecoveryLease, error) {
	if err := journal.record("prepare"); err != nil {
		return store.BuildRecoveryLease{}, err
	}
	return store.BuildRecoveryLease{}, nil
}

type serviceWorker struct {
	calls  *[]string
	result json.RawMessage
	failAt string
}

func (worker *serviceWorker) record(call string) error {
	*worker.calls = append(*worker.calls, call)
	if worker.failAt == call {
		return errors.New(call + " failed")
	}
	return nil
}

func (worker *serviceWorker) Run(
	_ context.Context, _ engine.JournalEffect,
) (json.RawMessage, error) {
	if err := worker.record("run"); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), worker.result...), nil
}

func (worker *serviceWorker) Cleanup(_ context.Context, effect engine.JournalEffect) error {
	return worker.record("cleanup:" + effect.ID)
}

func (worker *serviceWorker) ReconcileUnbound(
	_ context.Context, effect engine.JournalEffect, _ string,
) (effects.BuildRetryProof, error) {
	return effects.BuildRetryProof{}, worker.record("reconcile:" + effect.ID)
}

func TestBuilderServiceFixesPrepareBindPublishCompleteOrder(t *testing.T) {
	for _, test := range []struct {
		name string
		fail string
		want []string
	}{
		{name: "success", want: []string{"prepare-execution", "run", "bind", "complete"}},
		{name: "preparation failure", fail: "prepare-execution", want: []string{"prepare-execution"}},
		{name: "run failure", fail: "run", want: []string{"prepare-execution", "run"}},
		{name: "bind failure", fail: "bind", want: []string{"prepare-execution", "run", "bind"}},
		{name: "completion failure", fail: "complete", want: []string{"prepare-execution", "run", "bind", "complete"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			effect := engine.JournalEffect{
				ID: "effect-build", DeliveryRunID: "delivery-run", Kind: engine.EffectBuild,
				Attempt: 1, Request: json.RawMessage(`{"request":true}`),
			}
			journal := &serviceJournal{calls: &calls, failAt: test.fail, prepared: effect}
			worker := &serviceWorker{calls: &calls, result: json.RawMessage(`{"result":true}`), failAt: test.fail}
			service := BuilderService{journal: journal, worker: worker}
			err := service.Execute(context.Background(), store.EffectLease{})
			if test.fail == "" && err != nil {
				t.Fatal(err)
			}
			if test.fail != "" && (err == nil || !strings.Contains(err.Error(), test.fail+" failed")) {
				t.Fatalf("Execute error = %v", err)
			}
			if !reflect.DeepEqual(calls, test.want) {
				t.Fatalf("calls = %#v, want %#v", calls, test.want)
			}
		})
	}
}

func TestBuilderServiceReconcilesBeforeAnyRetry(t *testing.T) {
	var calls []string
	journal := &serviceJournal{
		calls: &calls, interrupted: 3,
		unknown: []engine.JournalEffect{
			{ID: "bound-build", Kind: engine.EffectBuild, Attempt: 1, Result: json.RawMessage(`{}`)},
			{ID: "bound-check", Kind: engine.EffectLocalCheck, Attempt: 2, Result: json.RawMessage(`{}`)},
			{ID: "unbound-build", Kind: engine.EffectBuild, Attempt: 3},
		},
	}
	worker := &serviceWorker{calls: &calls}
	service := BuilderService{journal: journal, worker: worker}
	report, err := service.ReconcileAfterExclusiveOwnership(
		context.Background(), "controller restarted", "reconciler-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"interrupt", "unknown", "cleanup:bound-build", "recover-bound:bound-build",
		"recover-bound:bound-check", "prepare", "reconcile:", "retry",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if report != (RecoveryReport{Interrupted: 3, Bound: 2, Retried: 1}) {
		t.Fatalf("report = %#v", report)
	}
}

func TestBuilderServiceStopsOnUnboundNonBuildEffect(t *testing.T) {
	var calls []string
	journal := &serviceJournal{
		calls: &calls,
		unknown: []engine.JournalEffect{{
			ID: "unbound-check", Kind: engine.EffectLocalCheck, Attempt: 1,
		}},
	}
	service := BuilderService{journal: journal, worker: &serviceWorker{calls: &calls}}
	_, err := service.ReconcileAfterExclusiveOwnership(
		context.Background(), "controller restarted", "reconciler-1",
	)
	if err == nil || !strings.Contains(err.Error(), "has no retry proof") {
		t.Fatalf("reconcile error = %v", err)
	}
	if want := []string{"interrupt", "unknown"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestBuilderServiceValidatesClaimBeforeBuilderCleanup(t *testing.T) {
	var calls []string
	journal := &serviceJournal{
		calls: &calls, failAt: "prepare",
		unknown: []engine.JournalEffect{{
			ID: "legacy-build", Kind: engine.EffectBuild, Attempt: 1,
		}},
	}
	service := BuilderService{journal: journal, worker: &serviceWorker{calls: &calls}}
	_, err := service.ReconcileAfterExclusiveOwnership(
		context.Background(), "controller restarted", "reconciler-1",
	)
	if err == nil || !strings.Contains(err.Error(), "prepare failed") {
		t.Fatalf("reconcile error = %v", err)
	}
	if want := []string{"interrupt", "unknown", "prepare"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
