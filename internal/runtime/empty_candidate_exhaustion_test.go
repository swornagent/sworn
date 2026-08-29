package runtime

import (
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
)

// TestEmptyCandidateExhaustionParkNamesItsCause pins A3: an exhaustion park
// caused by three consecutive EMPTY_CANDIDATE git.seal failures on one
// slice's lane carries that code and a bounded, honest detail as
// PinnedWork, mirroring the CANDIDATE_SCOPE_FAILED exhaustion-detail
// pattern for a cause that, unlike scope failure, has no structured Result
// to render.
func TestEmptyCandidateExhaustionParkNamesItsCause(t *testing.T) {
	fixture := newDegradationStatusFixture(t)
	ctx := fixture.ctx
	manifest := fixture.manifest
	store := fixture.store
	engine := fixture.engine
	owner := fixture.owner
	now := fixture.now

	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "Design S1.", Detail: []byte("design"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "captain", Result: "proceed",
		Summary: "Proceed S1.", Detail: []byte("review"),
	}); err != nil {
		t.Fatal(err)
	}
	state, err := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, ok := state.Slice("S1")
	if !ok || slice.NextRole != "implementer" || slice.Stage != "implement" {
		t.Fatalf("S1 did not reach implement stage: %#v", slice)
	}
	before := sliceFingerprint(state, "S1")
	workID := workIdentity(before, "git.seal")

	const epoch = int64(1)
	for try := int64(1); try <= 3; try++ {
		journalOuterTry(t, ctx, store, owner, manifest.value.RunID, workID,
			epoch, try, journal.OperationalFailed, "EMPTY_CANDIDATE", now)
	}

	status, err := fixture.service.Status(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var found *PinnedWork
	for i := range status.PinnedWork {
		if status.PinnedWork[i].WorkID == workID {
			found = &status.PinnedWork[i]
		}
	}
	if found == nil {
		t.Fatalf("no PinnedWork for %s: %#v", workID, status.PinnedWork)
	}
	if found.Cause != ParkCauseExhaustion || found.Code != "EMPTY_CANDIDATE" ||
		found.Detail != "EMPTY_CANDIDATE: implementation produced no change to seal" {
		t.Fatalf("empty-candidate PinnedWork = %#v", found)
	}
	if !validParkDetail(found.Detail) {
		t.Fatalf("detail fails validParkDetail: %q", found.Detail)
	}
}
