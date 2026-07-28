package runtime

import (
	"errors"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
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
