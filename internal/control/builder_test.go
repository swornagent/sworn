package control

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/store"
)

type serviceJournal struct {
	calls       *[]string
	unknown     []engine.JournalEffect
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

func (journal *serviceJournal) BindAuthorizedBuildResult(
	_ context.Context, _ store.PreparedAuthorizedBuildLease, _ json.RawMessage,
) error {
	return journal.record("bind")
}

func (journal *serviceJournal) PrepareAuthorizedBuildExecution(
	_ context.Context,
	_ store.AuthorizedBuildLease,
) (store.PreparedAuthorizedBuildLease, error) {
	if err := journal.record("prepare-execution"); err != nil {
		return store.PreparedAuthorizedBuildLease{}, err
	}
	return store.PreparedAuthorizedBuildLease{}, nil
}

func (journal *serviceJournal) CompleteAuthorizedBuild(context.Context, store.PreparedAuthorizedBuildLease) error {
	return journal.record("complete")
}

func (journal *serviceJournal) RecoverControlledInterruptedEffects(
	context.Context, *store.ControllerOwnership, string, string,
) (int, error) {
	return journal.interrupted, journal.record("interrupt")
}

func (journal *serviceJournal) UnknownEffects(context.Context) ([]engine.JournalEffect, error) {
	if err := journal.record("unknown"); err != nil {
		return nil, err
	}
	return journal.unknown, nil
}

func (journal *serviceJournal) RecoverControlledBoundEffect(
	_ context.Context, _ *store.ControllerOwnership, _ string, effectID string, _ int64,
) error {
	return journal.record("recover-bound:" + effectID)
}

func (journal *serviceJournal) PrepareControlledBoundBuildCleanup(
	_ context.Context, _ *store.ControllerOwnership, _ string, effectID string, _ int64,
) (store.BoundBuildCleanupLease, error) {
	if err := journal.record("prepare-cleanup:" + effectID); err != nil {
		return store.BoundBuildCleanupLease{}, err
	}
	return store.BoundBuildCleanupLease{}, nil
}

func (journal *serviceJournal) RecoverControlledUnboundBuildEffect(
	_ context.Context, _ *store.ControllerOwnership, _ string,
	_ store.BuildRecoveryLease, _ store.BuildRetryProof,
) error {
	return journal.record("retry")
}

func (journal *serviceJournal) PrepareControlledUnboundBuildRecovery(
	context.Context, *store.ControllerOwnership, string, string, int64,
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

type terminatingServiceWorker struct {
	*serviceWorker
	mode string
}

func (worker *terminatingServiceWorker) Run(
	context.Context,
	store.PreparedAuthorizedBuildLease,
) (json.RawMessage, error) {
	switch worker.mode {
	case "panic":
		panic("builder panicked")
	case "goexit":
		runtime.Goexit()
	}
	return nil, errors.New("unknown termination mode")
}

func (worker *serviceWorker) record(call string) error {
	*worker.calls = append(*worker.calls, call)
	if worker.failAt == call {
		return errors.New(call + " failed")
	}
	return nil
}

func (worker *serviceWorker) Run(
	_ context.Context, _ store.PreparedAuthorizedBuildLease,
) (json.RawMessage, error) {
	if err := worker.record("run"); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), worker.result...), nil
}

func (worker *serviceWorker) Cleanup(_ context.Context, _ store.BoundBuildCleanupLease) error {
	return worker.record("cleanup")
}

func (worker *serviceWorker) ReconcileUnbound(
	_ context.Context, _ store.BuildRecoveryLease,
) (store.BuildRetryProof, error) {
	return store.BuildRetryProof{}, worker.record("reconcile")
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
			journal := &serviceJournal{calls: &calls, failAt: test.fail}
			worker := &serviceWorker{calls: &calls, result: json.RawMessage(`{"result":true}`), failAt: test.fail}
			service := BuilderService{journal: journal, worker: worker}
			err := service.execute(context.Background(), store.AuthorizedBuildLease{})
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

func TestBuilderControllerStopsForRecoveryWhenClaimedExecutionDoesNotReturnSuccess(t *testing.T) {
	type outcome struct {
		returned  bool
		recovered any
	}
	for _, mode := range []string{"panic", "goexit"} {
		t.Run(mode, func(t *testing.T) {
			var calls []string
			journal := &serviceJournal{calls: &calls}
			worker := &terminatingServiceWorker{
				serviceWorker: &serviceWorker{calls: &calls}, mode: mode,
			}
			controller := &BuilderController{
				ownership: new(store.ControllerOwnership),
				builder:   BuilderService{journal: journal, worker: worker},
			}
			finished := make(chan outcome, 1)
			go func() {
				result := outcome{}
				defer func() {
					result.recovered = recover()
					finished <- result
				}()
				_ = controller.executeClaimedBuild(context.Background(), store.AuthorizedBuildLease{})
				result.returned = true
			}()
			result := <-finished
			if result.returned {
				t.Fatal("claimed execution returned normally")
			}
			if mode == "panic" && result.recovered == nil {
				t.Fatal("builder panic did not propagate")
			}
			if mode == "goexit" && result.recovered != nil {
				t.Fatalf("Goexit recovered unexpected panic: %v", result.recovered)
			}
			if !controller.closed {
				t.Fatal("claimed execution termination left controller active")
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
	report, err := service.reconcileAfterExclusiveOwnership(
		context.Background(), new(store.ControllerOwnership), "controller restarted", "reconciler-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"interrupt", "unknown", "prepare-cleanup:bound-build", "cleanup",
		"recover-bound:bound-build", "recover-bound:bound-check", "prepare", "reconcile", "retry",
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
	_, err := service.reconcileAfterExclusiveOwnership(
		context.Background(), new(store.ControllerOwnership), "controller restarted", "reconciler-1",
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
	_, err := service.reconcileAfterExclusiveOwnership(
		context.Background(), new(store.ControllerOwnership), "controller restarted", "reconciler-1",
	)
	if err == nil || !strings.Contains(err.Error(), "prepare failed") {
		t.Fatalf("reconcile error = %v", err)
	}
	if want := []string{"interrupt", "unknown", "prepare"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestBuilderServicePreparesBoundResultBeforeCleanup(t *testing.T) {
	var calls []string
	journal := &serviceJournal{
		calls: &calls, failAt: "prepare-cleanup:bound-build",
		unknown: []engine.JournalEffect{{
			ID: "bound-build", Kind: engine.EffectBuild, Attempt: 1,
			Result: json.RawMessage(`{}`),
		}},
	}
	service := BuilderService{journal: journal, worker: &serviceWorker{calls: &calls}}
	_, err := service.reconcileAfterExclusiveOwnership(
		context.Background(), new(store.ControllerOwnership), "controller restarted", "reconciler-1",
	)
	if err == nil || !strings.Contains(err.Error(), "prepare-cleanup:bound-build failed") {
		t.Fatalf("reconcile error = %v", err)
	}
	if want := []string{"interrupt", "unknown", "prepare-cleanup:bound-build"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
