package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// designDispatchWork returns the exact work identity readyLaneCandidates
// derives for the fixture's ready S1 design dispatch, with the Baton attempt
// that dispatch carries. A test that exhausts this work exhausts the lane's
// own candidate work, exactly as a real dispatch does.
func (f *economyGuardFixture) designDispatchWork(t *testing.T) (string, int64) {
	t.Helper()
	state, err := baton.ReadState(
		f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice("S1")
	if !ok || slice.NextRole != "implementer" || slice.Stage != "design" {
		t.Fatalf("S1 is not a ready design dispatch: %#v", slice)
	}
	before := sliceFingerprint(state, "S1")
	work := driverWorkIdentity(
		f.manifest.digest, "S1", driver.ImplementerDesign, slice.Attempt, before)
	if work == "" {
		t.Fatal("empty design dispatch work identity")
	}
	return work, slice.Attempt
}

// exhaustTryBudget spends one work's whole current-epoch try budget on
// operational failures. The codes deliberately alternate: an unbroken run of
// identical codes crosses the identical-failure guard first, which names its
// own park cause, and this fixture is about the exhaustion cause.
func (f *economyGuardFixture) exhaustTryBudget(
	t *testing.T,
	work string,
	attempt int64,
) {
	t.Helper()
	codes := []string{"INVOCATION_TIMEOUT", "TRANSPORT_FAILED", "INVOCATION_TIMEOUT"}
	for try := int64(1); try <= 3; try++ {
		f.contextualFailedDispatchAttempt(
			t, work, 1, try, codes[try-1],
			"S1", driver.ImplementerDesign, attempt,
		)
	}
}

// moveTargetBranch commits an unrelated file on the run's target branch. It
// journals nothing for the run: only state.Refs.Target.Head moves.
func (f *economyGuardFixture) moveTargetBranch(t *testing.T) {
	t.Helper()
	repository := f.manifest.value.Repository
	if err := os.WriteFile(
		filepath.Join(repository, "unrelated.txt"),
		[]byte("an unrelated commit on the target branch\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repository, "add", "--", "unrelated.txt")
	runRuntimeGitIdentity(
		t, repository, "commit", "--quiet", "-m", "unrelated target commit")
}

// dispatchEffectCount counts the journalled driver.dispatch effects, so a
// test can prove the drive loop started no new try.
func (f *economyGuardFixture) dispatchEffectCount(t *testing.T) int {
	t.Helper()
	snapshot, err := f.store.Snapshot(f.ctx, f.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "driver.dispatch" {
			count++
		}
	}
	return count
}

// requireExhaustionPark asserts the run reads parked on exhaustion, naming
// the exhausted work both as the run-level park and as the affected lane's
// pinned work.
func (f *economyGuardFixture) requireExhaustionPark(
	t *testing.T,
	work string,
	label string,
) {
	t.Helper()
	status, err := f.service.Status(f.ctx, f.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "parked" {
		t.Fatalf("%s: state = %q, want parked", label, status.State)
	}
	if status.Park == nil || status.Park.Cause != ParkCauseExhaustion ||
		status.Park.Work != work {
		t.Fatalf("%s: park = %#v, want exhaustion on %s", label, status.Park, work)
	}
	found := false
	for _, pinned := range status.PinnedWork {
		if pinned.WorkID == work && pinned.Lane == "T1" &&
			pinned.Cause == ParkCauseExhaustion {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s: pinned work = %#v, want T1 exhaustion on %s",
			label, status.PinnedWork, work)
	}
}

// TestExhaustionParkSurvivesTargetMove pins sworn#293: a work whose
// current-epoch try budget is spent parks the run, and that park is a
// journalled fact about the run, not a re-derivation over whatever the
// repository happens to hold at read time. Every candidate work identity
// binds the target head (sliceFingerprintAtTrackHead), so before this slice
// an unrelated commit on the target branch moved every candidate work,
// unpinned the exhausted lane, and left an inert run reading
// state=running park=null with no journal change at all.
func TestExhaustionParkSurvivesTargetMove(t *testing.T) {
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	work, attempt := fixture.designDispatchWork(t)
	fixture.exhaustTryBudget(t, work, attempt)
	fixture.requireExhaustionPark(t, work, "before the target moved")

	// The drive loop journals the typed park event exactly once and starts
	// no new try.
	dispatched := fixture.dispatchEffectCount(t)
	if err := fixture.service.driveLoop(
		fixture.ctx, fixture.engine, fixture.owner, false,
	); err != nil {
		t.Fatalf("driveLoop error = %v", err)
	}
	if got := fixture.dispatchEffectCount(t); got != dispatched {
		t.Fatalf("dispatch effects = %d, want %d: the loop started new work "+
			"for an exhausted lane", got, dispatched)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var parkEvents []journal.Event
	for _, event := range snapshot.Events {
		if event.Kind == ParkEventKind {
			parkEvents = append(parkEvents, event)
		}
	}
	if len(parkEvents) != 1 {
		t.Fatalf("park events = %d, want 1", len(parkEvents))
	}
	parsed, err := ParseDegradationParkEvent(parkEvents[0].Body)
	if err != nil {
		t.Fatalf("park event unparsable: %v", err)
	}
	if parsed.SchemaVersion != ParkEventVersion ||
		parsed.Cause != ParkCauseExhaustion || parsed.Work != work {
		t.Fatalf("typed park event = %#v", parsed)
	}

	// An unrelated commit on the target branch moves every candidate work
	// identity. The run's journal is untouched.
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	offset := status.EventOffset
	fixture.moveTargetBranch(t)
	fixture.requireExhaustionPark(t, work, "after the target moved")

	// The loop still refuses the exhausted lane rather than re-dispatching
	// it under the new work identity, and journals no second park event.
	dispatched = fixture.dispatchEffectCount(t)
	if err := fixture.service.driveLoop(
		fixture.ctx, fixture.engine, fixture.owner, false,
	); err != nil {
		t.Fatalf("driveLoop error after target move = %v", err)
	}
	if got := fixture.dispatchEffectCount(t); got != dispatched {
		t.Fatalf("dispatch effects = %d, want %d: the loop re-dispatched an "+
			"exhausted lane under a new work identity", got, dispatched)
	}
	after, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if after.EventOffset != offset {
		t.Fatalf("event offset = %d, want %d: the target move must journal "+
			"nothing for the run", after.EventOffset, offset)
	}
}

// TestExhaustionParkClearsOnRetry pins the other half of sworn#293's
// expectation: the park stands until a control command changes it. A paid
// retry bumps the work's epoch, which spends the exhaustion.
func TestExhaustionParkClearsOnRetry(t *testing.T) {
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	work, attempt := fixture.designDispatchWork(t)
	fixture.exhaustTryBudget(t, work, attempt)
	fixture.requireExhaustionPark(t, work, "before the retry")

	control, err := fixture.store.ControlProjection(
		fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyControl(fixture.ctx, journal.ControlCommand{
		ID:                 "control-retry-1",
		RunID:              fixture.manifest.value.RunID,
		Kind:               journal.Retry,
		WorkID:             work,
		ExpectedEpoch:      1,
		ExpectedGeneration: control.Generation,
	}, fixture.now); err != nil {
		t.Fatalf("retry control: %v", err)
	}
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State == "parked" || status.Park != nil {
		t.Fatalf("state = %q park = %#v, want the retry to spend the park",
			status.State, status.Park)
	}
}
