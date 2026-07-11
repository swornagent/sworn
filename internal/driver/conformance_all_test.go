package driver_test

// TestDriverConformance enrols every driver in the compiled-in registry into
// the exported behavioural conformance suite (S10 AC-02), plus the
// transport-less reference driver as the fifth subject.
//
// Enrolment is FAIL-CLOSED detection, not zero-edit auto-enrolment (Coach
// disposition 4): Registry.Drivers() yields names, not instances or fake
// wiring, so each registered name is looked up in the test-owned
// name→Enrolment map below and a registered driver missing from the map fails
// the suite loudly. A newly registered driver therefore cannot ship
// unenrolled silently — its author adds one map entry (the fake wiring only
// they can know) and the suite covers it.
//
// This file deliberately imports neither internal/model nor internal/agent —
// TestNoWireImports scans every .go file in this directory, test files
// included. The wiring that needs those packages lives in drivertest.

import (
	"testing"

	"github.com/swornagent/sworn/internal/driver/drivertest"
)

func TestDriverConformance(t *testing.T) {
	chat, responses := drivertest.ProxyEnrolments(t)
	enrolments := map[string]drivertest.Enrolment{
		"claude-subprocess":       drivertest.FakeClaudeEnrolment(t),
		"codex-subprocess":        drivertest.FakeCodexEnrolment(t),
		"oai-inprocess":           chat,
		"oai-responses-inprocess": responses,
		"conformance-reference":   drivertest.StubEnrolment(t),
	}

	registered := drivertest.RegisteredDriverNames()
	if len(registered) == 0 {
		t.Fatal("registry.Default enumerated zero drivers — enumeration broken")
	}
	for _, name := range registered {
		if _, ok := enrolments[name]; !ok {
			t.Fatalf("driver %q is registered in registry.Default but has no conformance enrolment — "+
				"add a drivertest.Enrolment for it in conformance_all_test.go (fail-closed, AC-02)", name)
		}
	}

	for name, e := range enrolments {
		if e.NewDriver == nil {
			t.Fatalf("enrolment %q has no factory", name)
		}
		drivertest.Run(t, e.NewDriver, e.Options)
	}
}
