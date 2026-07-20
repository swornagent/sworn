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

type checkServiceJournalFixture struct {
	calls  *[]string
	failAt string
}

func (journal *checkServiceJournalFixture) record(call string) error {
	*journal.calls = append(*journal.calls, call)
	if journal.failAt == call {
		return errors.New(call + " failed")
	}
	return nil
}

func (journal *checkServiceJournalFixture) PrepareAuthorizedCheckExecution(
	context.Context,
	store.AuthorizedCheckLease,
) (store.PreparedAuthorizedCheckLease, error) {
	if err := journal.record("prepare-check"); err != nil {
		return store.PreparedAuthorizedCheckLease{}, err
	}
	return store.PreparedAuthorizedCheckLease{}, nil
}

func (journal *checkServiceJournalFixture) BindAuthorizedCheckResult(
	context.Context,
	store.PreparedAuthorizedCheckLease,
	json.RawMessage,
) error {
	return journal.record("bind-check")
}

func (journal *checkServiceJournalFixture) CompleteAuthorizedCheck(
	context.Context,
	store.PreparedAuthorizedCheckLease,
) error {
	return journal.record("complete-check")
}

func (journal *checkServiceJournalFixture) PrepareControlledUnboundCheckRecovery(
	context.Context,
	*store.ControllerOwnership,
	string,
	string,
	int64,
) (store.CheckRecoveryLease, error) {
	if err := journal.record("prepare-check-recovery"); err != nil {
		return store.CheckRecoveryLease{}, err
	}
	return store.CheckRecoveryLease{}, nil
}

func (journal *checkServiceJournalFixture) RecoverControlledUnboundCheckEffect(
	context.Context,
	*store.ControllerOwnership,
	string,
	store.CheckRecoveryLease,
	store.CheckRetryProof,
) error {
	return journal.record("recover-check")
}

type checkServiceWorkerFixture struct {
	calls  *[]string
	result json.RawMessage
	failAt string
	mode   string
}

func (worker *checkServiceWorkerFixture) record(call string) error {
	*worker.calls = append(*worker.calls, call)
	if worker.failAt == call {
		return errors.New(call + " failed")
	}
	return nil
}

func (worker *checkServiceWorkerFixture) Run(
	context.Context,
	store.PreparedAuthorizedCheckLease,
) (json.RawMessage, error) {
	if err := worker.record("run-check"); err != nil {
		return nil, err
	}
	switch worker.mode {
	case "panic":
		panic("check panicked")
	case "goexit":
		runtime.Goexit()
	}
	return append(json.RawMessage(nil), worker.result...), nil
}

func (worker *checkServiceWorkerFixture) ReconcileUnbound(
	context.Context,
	store.CheckRecoveryLease,
) (store.CheckRetryProof, error) {
	return store.CheckRetryProof{}, worker.record("reconcile-check")
}

func TestCheckServiceFixesPrepareRunBindCompleteOrder(t *testing.T) {
	for _, test := range []struct {
		name string
		fail string
		want []string
	}{
		{name: "success", want: []string{"prepare-check", "run-check", "bind-check", "complete-check"}},
		{name: "preparation failure", fail: "prepare-check", want: []string{"prepare-check"}},
		{name: "run failure", fail: "run-check", want: []string{"prepare-check", "run-check"}},
		{name: "bind failure", fail: "bind-check", want: []string{"prepare-check", "run-check", "bind-check"}},
		{name: "completion failure", fail: "complete-check", want: []string{"prepare-check", "run-check", "bind-check", "complete-check"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			journal := &checkServiceJournalFixture{calls: &calls, failAt: test.fail}
			worker := &checkServiceWorkerFixture{
				calls: &calls, result: json.RawMessage(`{"result":true}`), failAt: test.fail,
			}
			service := CheckService{journal: journal, worker: worker}
			err := service.execute(context.Background(), store.AuthorizedCheckLease{})
			if test.fail == "" && err != nil {
				t.Fatal(err)
			}
			if test.fail != "" && (err == nil || !strings.Contains(err.Error(), test.fail+" failed")) {
				t.Fatalf("execute check error = %v", err)
			}
			if !reflect.DeepEqual(calls, test.want) {
				t.Fatalf("calls = %#v, want %#v", calls, test.want)
			}
		})
	}
}

func TestControllerStopsForRecoveryWhenClaimedCheckDoesNotReturnSuccess(t *testing.T) {
	type outcome struct {
		returned  bool
		recovered any
	}
	for _, mode := range []string{"panic", "goexit"} {
		t.Run(mode, func(t *testing.T) {
			var calls []string
			journal := &checkServiceJournalFixture{calls: &calls}
			worker := &checkServiceWorkerFixture{calls: &calls, mode: mode}
			controller := &Controller{
				ownership: new(store.ControllerOwnership),
				checks:    CheckService{journal: journal, worker: worker},
			}
			finished := make(chan outcome, 1)
			go func() {
				result := outcome{}
				defer func() {
					result.recovered = recover()
					finished <- result
				}()
				_ = controller.executeClaimedCheck(context.Background(), store.AuthorizedCheckLease{})
				result.returned = true
			}()
			result := <-finished
			if result.returned {
				t.Fatal("claimed check execution returned normally")
			}
			if mode == "panic" && result.recovered == nil {
				t.Fatal("check panic did not propagate")
			}
			if mode == "goexit" && result.recovered != nil {
				t.Fatalf("Goexit recovered unexpected panic: %v", result.recovered)
			}
			if !controller.closed {
				t.Fatal("claimed check termination left controller active")
			}
			if !reflect.DeepEqual(calls, []string{"prepare-check", "run-check"}) {
				t.Fatalf("termination calls = %v", calls)
			}
		})
	}
}

func TestCheckRecoveryServiceFixesProofBeforeRequeueOrder(t *testing.T) {
	var calls []string
	journal := &checkServiceJournalFixture{calls: &calls}
	worker := &checkServiceWorkerFixture{calls: &calls}
	service := CheckService{journal: journal, worker: worker}
	err := service.reconcileUnbound(
		context.Background(), new(store.ControllerOwnership), "controller-recovery",
		engine.JournalEffect{ID: "check-effect", Kind: engine.EffectLocalCheck, Attempt: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare-check-recovery", "reconcile-check", "recover-check"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("check recovery calls = %v, want %v", calls, want)
	}
}

func TestControllerRecoveryBarrierReconcilesUnboundCheckBeforeActivation(t *testing.T) {
	var calls []string
	builderJournal := &serviceJournal{
		calls: &calls, interrupted: 1,
		unknown: []engine.JournalEffect{{
			ID: "unbound-check", Kind: engine.EffectLocalCheck, Attempt: 1,
		}},
	}
	checkJournal := &checkServiceJournalFixture{calls: &calls}
	boundStore := &store.Store{}
	checks := CheckService{
		journal: checkJournal, store: boundStore,
		worker:        &checkServiceWorkerFixture{calls: &calls},
		runtimeDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	}
	builder := BuilderService{
		journal: builderJournal,
		worker:  &serviceWorker{calls: &calls},
	}
	report, err := builder.reconcileWithChecksAfterExclusiveOwnership(
		context.Background(), new(store.ControllerOwnership),
		"controller restarted", "controller-recovery", checks,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"interrupt", "unknown", "prepare-check-recovery", "reconcile-check", "recover-check",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("controller check recovery barrier calls = %v, want %v", calls, want)
	}
	if report != (RecoveryReport{Interrupted: 1, ChecksRetried: 1}) {
		t.Fatalf("controller check recovery report = %#v", report)
	}
}
