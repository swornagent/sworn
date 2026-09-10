package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func TestHostCheckFailureRetriesWithRetainedWorkAndExactFeedback(t *testing.T) {
	t.Run("inline contract", func(t *testing.T) { testHostRepairRoundTrip(t, false) })
	t.Run("digest-addressed manifest contract", func(t *testing.T) { testHostRepairRoundTrip(t, true) })
}

func testHostRepairRoundTrip(t *testing.T, manifestPlan bool) {
	f := newHostCheckFixture(t, []string{"grep -q repaired one.txt || { echo 'one.txt needs repair'; exit 7; }"}, manifestPlan)
	calls := 0
	f.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		calls++
		var work productionWorkContext
		for _, input := range invocation.Inputs {
			if input.Input.Name == "work-context" {
				if err := json.Unmarshal(input.Bytes, &work); err != nil {
					t.Fatal(err)
				}
			}
		}
		path := filepath.Join(invocation.HostWorkspace, "one.txt")
		switch calls {
		case 1:
			if work.HostRepair != nil {
				t.Fatal("first try got repair context")
			}
			if err := os.WriteFile(path, []byte("valuable unfinished work\n"), 0600); err != nil {
				t.Fatal(err)
			}
		case 2:
			body, err := os.ReadFile(path)
			if err != nil || string(body) != "valuable unfinished work\n" {
				t.Fatalf("retry lost work: %q %v", body, err)
			}
			if work.HostRepair == nil {
				t.Fatal("retry lacks host repair context")
			}
			repair := work.HostRepair
			if repair.FailedCheck.ExitCode != 7 || repair.FailedCheck.Check != f.hostChecks[0] || !strings.Contains(repair.FailedCheck.Output, "one.txt needs repair") || repair.Submission.Summary != "Durable production candidate." {
				t.Fatalf("repair context = %#v", repair)
			}
			if work.Candidate != nil || work.HostEvidence != nil {
				t.Fatal("unverified repair became verified evidence")
			}
			priorInvocation := repair.Submission.InvocationID
			manualCoordinates := dispatchCoordinates{Slice: "S1", Responsibility: driver.ImplementerImplementation, BatonAttempt: work.Attempt, Epoch: 2, Try: 1}
			manualContext := work
			manualContext.Epoch, manualContext.Try = 2, 1
			manualContext.InvocationID = dispatchInvocationID(work.RunID, manualCoordinates)
			if err := validateProductionWorkContext(f.manifest, manualContext); err != nil {
				t.Fatalf("manual retry context refused: %v", err)
			}
			for name, mutate := range map[string]func(*productionHostRepair){
				"output":     func(r *productionHostRepair) { r.FailedCheck.Output += "altered" },
				"candidate":  func(r *productionHostRepair) { r.FailedCheck.Candidate = strings.Repeat("f", 40) },
				"slice":      func(r *productionHostRepair) { r.FailedCheck.Slice = "S2" },
				"submission": func(r *productionHostRepair) { r.Submission.InvocationID = invocation.Request.InvocationID },
				"pass":       func(r *productionHostRepair) { r.FailedCheck.Outcome = baton.CheckOutcomePass },
				"schema":     func(r *productionHostRepair) { r.SchemaVersion = "future" },
			} {
				changed := *repair
				mutate(&changed)
				if err := validateHostRepair(changed, priorInvocation, "S1"); err == nil {
					t.Fatalf("accepted substituted %s", name)
				}
			}
			for name, mutate := range map[string]func(*productionWorkContext){
				"authority": func(w *productionWorkContext) { w.Before = driver.Digest([]byte("other")) },
				"base":      func(w *productionWorkContext) { w.Authority.TrackHead = strings.Repeat("f", 40) },
				"role":      func(w *productionWorkContext) { w.Responsibility = driver.WorkVerification },
			} {
				changed := work
				mutate(&changed)
				if err := validateProductionWorkContext(f.manifest, changed); err == nil {
					t.Fatalf("accepted stale repair %s", name)
				}
			}
			if err := os.WriteFile(path, append(body, []byte("repaired\n")...), 0600); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unexpected blind retry %d", calls)
		}
		return productionImplementationObservation(t, invocation), nil
	})
	slice, _ := f.state.Slice("S1")
	if err := f.service.implementSlice(f.ctx, f.engine, f.owner, f.state, slice); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("dispatches = %d", calls)
	}
	state, err := baton.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := state.Slice("S1")
	if current.Candidate == nil || current.Pass != nil || current.NextRole != "verifier" {
		t.Fatalf("candidate bypassed independent verification: %#v", current)
	}
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "driver.dispatch" && effect.ErrorCode == "HOST_CHECK_FAILED" {
			failures++
			var repair productionHostRepair
			if effect.State != journal.OperationalFailed || json.Unmarshal(effect.Result, &repair) != nil || repair.SchemaVersion != hostRepairVersion {
				t.Fatal("failure did not durably retain repair input")
			}
		}
	}
	if failures != 1 {
		t.Fatalf("failed dispatches = %d", failures)
	}
}

func TestRepeatedHostFailuresPersistActionableParkExactlyOnce(t *testing.T) {
	f := newHostCheckFixture(t, []string{"echo 'still needs repair'; exit 7"})
	calls := 0
	f.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		calls++
		path := filepath.Join(invocation.HostWorkspace, "one.txt")
		if calls > 1 {
			if body, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(body), "retained") {
				t.Fatalf("lost prior candidate: %q %v", body, err)
			}
		}
		if err := os.WriteFile(path, []byte("retained"+strings.Repeat("x", calls)), 0600); err != nil {
			t.Fatal(err)
		}
		return productionImplementationObservation(t, invocation), nil
	})
	slice, _ := f.state.Slice("S1")
	if err := f.service.implementSlice(f.ctx, f.engine, f.owner, f.state, slice); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("repeat failure = %v", err)
	}
	if int64(calls) != f.manifest.value.EffectiveIdenticalFailureParkAfter() {
		t.Fatalf("dispatches = %d", calls)
	}
	state, err := baton.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	work := workIdentity(sliceFingerprint(state, "S1"), "git.seal")
	current, _ := state.Slice("S1")
	manualCoordinates := dispatchCoordinates{Slice: "S1", Responsibility: driver.ImplementerImplementation, BatonAttempt: current.Attempt, Epoch: 2, Try: 1}
	manualContext, _, err := captureProductionWorkContext(f.ctx, f.engine, manualCoordinates, sliceFingerprint(state, "S1"), driver.ReadWrite)
	if err != nil || manualContext.HostRepair == nil || manualContext.HostRepair.SourceEpoch != 1 || manualContext.HostRepair.SourceTry != int64(calls) {
		t.Fatalf("manual retry forgot parked failure: %#v %v", manualContext.HostRepair, err)
	}
	for i := 0; i < 2; i++ {
		if parked, err := f.service.economyGuardsParked(f.ctx, f.manifest, f.owner.RunID, work); err != nil || !parked {
			t.Fatalf("park replay = %v %v", parked, err)
		}
	}
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	parks := 0
	for _, event := range snapshot.Events {
		if event.Kind == "degradation_budget_parked" {
			parks++
			park, err := ParseDegradationParkEvent(event.Body)
			if err != nil || park.FailureCode != "HOST_CHECK_FAILED" || !strings.Contains(park.FailureDetail, "retained unverified candidate") || park.Work != work {
				t.Fatalf("actionable park = %#v %v", park, err)
			}
		}
	}
	if parks != 1 {
		t.Fatalf("durable notification-source events = %d", parks)
	}
}

func TestMissingLegacyHostRepairCannotFallThroughOnLaterRetry(t *testing.T) {
	f := newHostCheckFixture(t, []string{"exit 7"})
	before := sliceFingerprint(f.state, "S1")
	work := workIdentity(before, "git.seal")
	writeFailure := func(workID string, retry int64, kind, code string) {
		t.Helper()
		id := journal.AttemptEffectID(workID, 1, retry)
		payload := mustJSON(map[string]string{"fixture": kind})
		now := f.service.now()
		if err := f.store.EnsureAttempt(f.ctx,
			journal.Command{RunID: f.owner.RunID, ReplayKey: id, Kind: kind, Payload: payload, CreatedAt: now},
			journal.Effect{RunID: f.owner.RunID, ID: id, ReplayKey: id, Kind: kind, BeforeDigest: workID, ExpectedDigest: driver.Digest(payload), UpdatedAt: now},
			journal.EffectAttempt{WorkID: workID, Epoch: 1, Try: retry}); err != nil {
			t.Fatal(err)
		}
		claim, err := f.store.ClaimOwned(f.ctx, f.owner, id, now, effectLease)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.CompleteOwned(f.ctx, f.owner, journal.Completion{RunID: f.owner.RunID, EffectID: id, Token: claim.Token, State: journal.OperationalFailed, ErrorCode: code, EventKind: "fixture_failure", At: now}); err != nil {
			t.Fatal(err)
		}
	}
	writeFailure(workIdentity(work, "driver.dispatch"), 1, "driver.dispatch", "HOST_CHECK_FAILED")
	coordinates := dispatchCoordinates{Slice: "S1", Responsibility: driver.ImplementerImplementation, BatonAttempt: 1, Epoch: 1, Try: 2}
	plan, err := currentPlanBinding(f.state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := captureHostRepair(f.ctx, f.engine, coordinates, before, plan); !IsCode(err, "HOST_REPAIR_UNAVAILABLE") {
		t.Fatalf("legacy failure = %v", err)
	}
	// Admission refused before driver.dispatch existed for try 2. Try 3 must
	// not interpret that missing dispatch as permission to restart blindly.
	writeFailure(work, 2, "git.seal", "HOST_REPAIR_UNAVAILABLE")
	coordinates.Try = 3
	if _, err := captureHostRepair(f.ctx, f.engine, coordinates, before, plan); !IsCode(err, "HOST_REPAIR_UNAVAILABLE") {
		t.Fatalf("later retry fell through: %v", err)
	}
	coordinates.Epoch, coordinates.Try = 2, 1
	if _, err := captureHostRepair(f.ctx, f.engine, coordinates, before, plan); !IsCode(err, "HOST_REPAIR_UNAVAILABLE") {
		t.Fatalf("new epoch lost refusal: %v", err)
	}
}
