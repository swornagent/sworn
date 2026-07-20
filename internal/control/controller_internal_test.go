package control

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/swornagent/sworn/internal/store"
)

func TestControlledBuildConvergenceContextOutlivesCanceledCaller(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	probe, cancelProbe := controlledBuildConvergenceContext(parent)
	defer cancelProbe()
	if probe.Err() != nil {
		t.Fatalf("detached convergence context = %v", probe.Err())
	}
	if _, bounded := probe.Deadline(); !bounded {
		t.Fatal("detached convergence context has no deadline")
	}
}

func TestResolveControlledBuildApplyError(t *testing.T) {
	applyErr := errors.New("ambiguous apply")
	probeErr := errors.New("probe failed")
	durable := store.ApplyResult{
		CommandID: "cmd-build", RunID: "run-1", Outcome: store.OutcomeApplied,
		Revision: 2, EventID: "event-1", EffectIDs: []string{"effect-1"}, Replayed: true,
	}

	result, err := resolveControlledBuildApplyError(applyErr, durable, true, nil)
	if err != nil || !reflect.DeepEqual(result, durable) {
		t.Fatalf("durable result did not resolve ambiguous apply: %+v, %v", result, err)
	}
	result, err = resolveControlledBuildApplyError(applyErr, store.ApplyResult{}, false, nil)
	if !reflect.DeepEqual(result, store.ApplyResult{}) || !errors.Is(err, applyErr) {
		t.Fatalf("absent outcome did not preserve apply error: %+v, %v", result, err)
	}
	result, err = resolveControlledBuildApplyError(applyErr, durable, true, probeErr)
	if !reflect.DeepEqual(result, store.ApplyResult{}) ||
		!errors.Is(err, applyErr) || !errors.Is(err, probeErr) {
		t.Fatalf("probe failure did not preserve both errors: %+v, %v", result, err)
	}
}
