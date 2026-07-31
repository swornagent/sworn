package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func TestTrackBaseBeforeBindsCASStateAndConsumedAuthority(
	t *testing.T,
) {
	state := baton.State{
		Plan: baton.PlanState{OID: "plan"},
		Refs: baton.StateRefs{Target: baton.CapturedRef{
			Ref:  "refs/heads/main",
			Head: "target",
		}},
		Tracks: []baton.TrackState{{
			ID:   "T1",
			Head: "physical-before",
		}},
	}
	slice := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: baton.Track{ID: "T1"},
			Slice: baton.Slice{ID: "S1"},
		},
		Stage:           "design",
		NextRole:        "implementer",
		Attempt:         7,
		PreparationSeed: "seed",
		PreparedBase:    "prepared",
		ConsumedInputs: []baton.ConsumedInput{{
			Slice:            "S2",
			PassReceipt:      "pass-1",
			CandidateReceipt: "receipt-1",
			Candidate:        "candidate-1",
			ProductTree:      "sha256:same-product",
			SourceRef:        "refs/heads/track/release/T2",
			SourceHead:       "source-1",
		}},
	}
	before := trackBaseBefore(state, slice)

	state.Tracks[0].Head = "prepared"
	if after := trackBaseBefore(state, slice); after == before {
		t.Fatal("consumer CAS state did not change preparation identity")
	}
	state.Tracks[0].Head = "physical-before"

	newPass := *slice
	newPass.ConsumedInputs = append(
		[]baton.ConsumedInput(nil),
		slice.ConsumedInputs...,
	)
	newPass.ConsumedInputs[0].PassReceipt = "pass-2"
	newPass.ConsumedInputs[0].SourceHead = "source-2"
	if got := trackBaseBefore(state, &newPass); got == before {
		t.Fatal("same-product new PASS authority retained preparation identity")
	}
}

func TestCandidateHeadRefreshNeedsNoBaseMutation(t *testing.T) {
	state := baton.State{Tracks: []baton.TrackState{{
		ID:            "T1",
		Head:          "unreceipted-head",
		AuthorityHead: "candidate-receipt",
	}}}
	slice := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: baton.Track{ID: "T1"},
			Slice: baton.Slice{ID: "S1"},
		},
		History:        baton.SliceHistory{MaximumAttempt: 1},
		Stage:          "implement",
		Status:         "ready",
		NextRole:       "implementer",
		Outcome:        "stale",
		Attempt:        2,
		Retained:       false,
		StaleReason:    "track head changed before verification was recorded",
		CurrentReceipt: &baton.ReceiptEntry{OID: "candidate-receipt"},
		Candidate:      &baton.ReceiptEntry{OID: "candidate-receipt"},
	}
	if !candidateHeadRefresh(state, slice) {
		t.Fatal("exact candidate-head refresh was not recognized")
	}
	slice.Candidate = &baton.ReceiptEntry{OID: "other-receipt"}
	if candidateHeadRefresh(state, slice) {
		t.Fatal("mismatched candidate authority was recognized as a refresh")
	}
}

func TestSealedRefreshRecordMustMatchItsExplicitCycleAuthority(t *testing.T) {
	cycle := implementationCycle{
		Release:     "release-1",
		Slice:       "S1",
		Binds:       "candidate-receipt",
		TrackHead:   "physical-head",
		RefreshFrom: "candidate-receipt",
	}
	record := sealedRecord{
		Slice:       cycle.Slice,
		Binds:       cycle.Binds,
		Before:      cycle.TrackHead,
		RefreshFrom: cycle.RefreshFrom,
		Candidate:   "next-candidate",
		ProductTree: "sha256:product",
		Receipt: baton.AppendReceiptInput{
			Release:   cycle.Release,
			Slice:     cycle.Slice,
			Role:      "implementer",
			Result:    "candidate",
			Candidate: "next-candidate",
		},
	}
	if !sealedRecordMatchesCycle(record, cycle) {
		t.Fatal("exact refresh record did not match its cycle")
	}
	corrupt := cycle
	corrupt.RefreshFrom = "other-ancestor"
	record.RefreshFrom = corrupt.RefreshFrom
	if sealedRecordMatchesCycle(record, corrupt) {
		t.Fatal("cycle admitted a refresh base other than its bound receipt")
	}
	record.RefreshFrom = cycle.RefreshFrom
	if sealedRecordMatchesCycle(record, corrupt) {
		t.Fatal("record refresh authority drift matched its cycle")
	}
}

func TestStableErrorCodePreservesTrackBaseAuthorityMovement(t *testing.T) {
	err := &gitx.Error{
		Code: "AUTHORITY_MOVED",
		Op:   "prepare track base",
		Err:  errors.New("producer ref changed"),
	}
	if got := stableErrorCode(err); got != "AUTHORITY_MOVED" {
		t.Fatalf("stable code = %q, want AUTHORITY_MOVED", got)
	}
}

func TestTrackBaseRecoveryTerminalizesOnlyStaleAuthority(t *testing.T) {
	_, manifestBody, _ := fixtureManifest(t)
	manifest, err := admitManifest(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	service, store, owner, now := claimedDispatchJournal(t, manifest)
	defer store.Close()
	effect := addClaimedSealEffect(
		t,
		store,
		owner,
		now,
		"git.prepare_track_base",
		"stale-track-base",
	)

	if got := trackBaseCommandRequestError(
		runtimeFail("STALE_DISPATCH", nil),
	); !IsCode(got, "STALE_DISPATCH") {
		t.Fatalf("stale reconstruction error = %v", got)
	}
	if got := trackBaseCommandRequestError(
		runtimeFail("INVALID_TRACK_BASE", nil),
	); !IsCode(got, "CORRUPT_JOURNAL") {
		t.Fatalf("malformed reconstruction error = %v", got)
	}

	handled, err := service.terminalizeStaleTrackBase(
		context.Background(),
		owner,
		effect,
		runtimeFail("STALE_DISPATCH", nil),
	)
	if err != nil || !handled {
		t.Fatalf("stale track-base recovery = %t, %v", handled, err)
	}
	terminal, err := store.Effect(
		context.Background(),
		owner.RunID,
		effect.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State != journal.OperationalFailed ||
		terminal.ErrorCode != "stale_authority" ||
		terminal.CurrentClaim != "" {
		t.Fatalf("stale track-base effect = %#v", terminal)
	}

	handled, err = service.terminalizeStaleTrackBase(
		context.Background(),
		owner,
		terminal,
		runtimeFail("CORRUPT_JOURNAL", nil),
	)
	if handled || !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("corrupt track-base recovery = %t, %v", handled, err)
	}
}
