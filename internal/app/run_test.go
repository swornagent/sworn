package app

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/engine"
)

func TestSelectCurrentWorkIsBoundedAndExplicit(t *testing.T) {
	t.Parallel()

	state := activeRunState(engine.WorkReady)
	state.Work = append(state.Work, engine.Work{
		ID: "work-2", State: engine.WorkWaiting, NextAction: engine.ActionWait,
	})
	if selected, err := selectCurrentWork(state, ""); err != nil || selected != "work-1" {
		t.Fatalf("select implicit current work = %q, %v", selected, err)
	}
	if selected, err := selectCurrentWork(state, "work-1"); err != nil || selected != "work-1" {
		t.Fatalf("select explicit current work = %q, %v", selected, err)
	}
	if _, err := selectCurrentWork(state, "work-2"); err == nil || !strings.Contains(err.Error(), "waiting") {
		t.Fatalf("select waiting work error = %v", err)
	}
	if _, err := selectCurrentWork(state, "work-3"); err == nil || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("select absent work error = %v", err)
	}
}

func TestDeterministicOwnerIDIsStableAndRunBound(t *testing.T) {
	t.Parallel()

	configuration := validRunConfig()
	first := deterministicOwnerID(configuration, "run-1")
	if first != deterministicOwnerID(configuration, "run-1") || !engine.ValidID(first) {
		t.Fatalf("owner id is not stable and valid: %q", first)
	}
	if first == deterministicOwnerID(configuration, "run-2") {
		t.Fatal("owner id was not bound to the selected run")
	}
}

func activeRunState(state engine.WorkState) engine.State {
	attempt := int64(1)
	next := engine.ActionWait
	if state == engine.WorkReady {
		attempt, next = 0, engine.ActionBuild
	}
	return engine.State{
		SchemaVersion: engine.StateSchemaVersion,
		RunID:         "run-1", DeliveryID: "delivery-1",
		PlanDigest: "sha256:" + strings.Repeat("a", 64),
		Repository: "repo-1", TargetRef: "refs/heads/main",
		Revision: 2, Phase: engine.PhaseActive,
		AuthorityReceiptDigest: "sha256:" + strings.Repeat("b", 64),
		Work:                   []engine.Work{{ID: "work-1", State: state, Attempt: attempt, NextAction: next}},
	}
}
