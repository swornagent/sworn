package router

import (
	"context"
	"errors"
	"testing"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/state"
)

// ---------- fakes ----------

type fakeOracle struct {
	slices map[string]board.SliceState
	board  *board.BoardState
}

func (f *fakeOracle) ReadSliceStatus(_ context.Context, _, sliceID string) (board.SliceState, error) {
	ss, ok := f.slices[sliceID]
	if !ok {
		return board.SliceState{}, errors.New("slice not found")
	}
	return ss, nil
}

func (f *fakeOracle) ReadBoard(_ context.Context, _ string) (*board.BoardState, error) {
	if f.board == nil {
		return nil, errors.New("no board")
	}
	return f.board, nil
}

type fakeContent struct {
	commitTimes  map[string]int64
	existing     map[string]bool
	ancestors    map[string]bool // "ancestor|branch" → true
}

func (f *fakeContent) LastCommitTime(_, path string) (int64, error) {
	if f.commitTimes == nil {
		return 0, nil
	}
	return f.commitTimes[path], nil
}

func (f *fakeContent) CatFileExists(_, path string) (bool, error) {
	if f.existing == nil {
		return false, nil
	}
	return f.existing[path], nil
}

func (f *fakeContent) IsAncestor(ancestor, branch string) (bool, error) {
	if f.ancestors == nil {
		return true, nil // default: all merged
	}
	key := ancestor + "|" + branch
	return f.ancestors[key], nil
}

func defaultInput() RouteInput {
	return RouteInput{
		Release:     "test-release",
		SliceID:     "S01-test",
		TrackID:     "T1-core",
		TrackBranch: "refs/heads/track/test-release/T1-core",
		ReleaseRef:  "refs/heads/release-wt/test-release",
		DocsPrefix:  "docs",
	}
}

func s(ss board.SliceState, _ error) board.SliceState { return ss }

// ---------- tests ----------

func TestBlockedPrecedesState(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-test": {
				ID:     "S01-test",
				State:  state.Verified,
				Track:  "T1-core",
				Blocked: true,
				BlockedReason: "spec defect",
				Violations: []string{"spec defect: ambiguous AC"},
			},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextReplanRelease {
		t.Errorf("Blocked should route replan-release, got %s", d.NextType)
	}
}

func TestPlannedRoutesImplement(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-test": {ID: "S01-test", State: state.Planned, Track: "T1-core"},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextImplement {
		t.Errorf("planned should route implement, got %s", d.NextType)
	}
}

func TestInProgressRoutesImplement(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-test": {ID: "S01-test", State: state.InProgress, Track: "T1-core"},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextImplement {
		t.Errorf("in_progress should route implement, got %s", d.NextType)
	}
}

func TestImplementedRoutesVerify(t *testing.T) {
	tests := []struct {
		name             string
		verificationResult string
	}{
		{"no verdict", ""},
		{"pending", "pending"},
		{"stale fail", "fail"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oracle := &fakeOracle{
				slices: map[string]board.SliceState{
					"S01-test": {
						ID:                 "S01-test",
						State:              state.Implemented,
						Track:              "T1-core",
						VerificationResult: tt.verificationResult,
					},
				},
			}

			d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if d.NextType != NextVerify {
				t.Errorf("implemented with verification=%q should route verify, got %s", tt.verificationResult, d.NextType)
			}
		})
	}
}

func TestDesignReviewCommitTimeNewest(t *testing.T) {
	docsPrefix := "docs"
	release := "test-release"
	sliceID := "S01-test"

	t.Run("approved-ack present → implement", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				sliceID: {ID: sliceID, State: state.DesignReview, Track: "T1-core"},
			},
		}
		content := &fakeContent{
			existing: map[string]bool{
				docsPrefix + "/release/" + release + "/" + sliceID + "/approved-ack.md": true,
			},
		}

		d, err := Route(context.Background(), oracle, content, defaultInput())
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextImplement {
			t.Errorf("approved-ack present should route implement, got %s", d.NextType)
		}
	})

	t.Run("review.md newest → coach_decision", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				sliceID: {ID: sliceID, State: state.DesignReview, Track: "T1-core"},
			},
		}
		content := &fakeContent{
			commitTimes: map[string]int64{
				docsPrefix + "/release/" + release + "/" + sliceID + "/design.md":  100,
				docsPrefix + "/release/" + release + "/" + sliceID + "/review.md":  200,
				docsPrefix + "/release/" + release + "/" + sliceID + "/decline.md": 0,
			},
		}

		d, err := Route(context.Background(), oracle, content, defaultInput())
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextCoachDecision {
			t.Errorf("review newest should route coach_decision, got %s", d.NextType)
		}
	})

	t.Run("decline.md newest → implement", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				sliceID: {ID: sliceID, State: state.DesignReview, Track: "T1-core"},
			},
		}
		content := &fakeContent{
			commitTimes: map[string]int64{
				docsPrefix + "/release/" + release + "/" + sliceID + "/design.md":  100,
				docsPrefix + "/release/" + release + "/" + sliceID + "/review.md":  0,
				docsPrefix + "/release/" + release + "/" + sliceID + "/decline.md": 150,
			},
		}

		d, err := Route(context.Background(), oracle, content, defaultInput())
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextImplement {
			t.Errorf("decline newest should route implement, got %s", d.NextType)
		}
	})

	t.Run("design.md newest → review", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				sliceID: {ID: sliceID, State: state.DesignReview, Track: "T1-core"},
			},
		}
		content := &fakeContent{
			commitTimes: map[string]int64{
				docsPrefix + "/release/" + release + "/" + sliceID + "/design.md":  100,
				docsPrefix + "/release/" + release + "/" + sliceID + "/review.md":  0,
				docsPrefix + "/release/" + release + "/" + sliceID + "/decline.md": 0,
			},
		}

		d, err := Route(context.Background(), oracle, content, defaultInput())
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextReview {
			t.Errorf("design newest should route review, got %s", d.NextType)
		}
	})
}

func TestFailedVerificationGateClassification(t *testing.T) {
	t.Run("Gate 1/2/6 → redesign", func(t *testing.T) {
		for _, gate := range []string{"Gate 1", "Gate 2", "Gate 6"} {
			oracle := &fakeOracle{
				slices: map[string]board.SliceState{
					"S01-test": {
						ID:     "S01-test",
						State:  state.FailedVerification,
						Track:  "T1-core",
						Violations: []string{gate + ": missing reachability"},
					},
				},
			}

			d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if d.NextType != NextRedesign {
				t.Errorf("%s should route redesign, got %s", gate, d.NextType)
			}
		}
	})

	t.Run("Gate 3/4/5 → implement", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				"S01-test": {
					ID:     "S01-test",
					State:  state.FailedVerification,
					Track:  "T1-core",
					Violations: []string{"Gate 3: test failure", "Gate 4: missing artefact", "Gate 5: undeclared deferral"},
				},
			},
		}

		d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextImplement {
			t.Errorf("Gate 3/4/5 should route implement, got %s", d.NextType)
		}
	})
}

func TestShippedRoutesNone(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-test": {ID: "S01-test", State: "shipped", Track: "T1-core"},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextNone {
		t.Errorf("shipped should route none, got %s", d.NextType)
	}
}

func TestDeferredRoutesNone(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-test": {ID: "S01-test", State: state.Deferred, Track: "T1-core"},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextNone {
		t.Errorf("deferred should route none, got %s", d.NextType)
	}
}

func TestUnrecognisedStateRoutesNone(t *testing.T) {
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-test": {ID: "S01-test", State: "bogus", Track: "T1-core"},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, defaultInput())
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextNone {
		t.Errorf("unrecognised state should route none, got %s", d.NextType)
	}
}

func TestVerifiedWalksTrackThenMerges(t *testing.T) {
	input := defaultInput()
	input.SliceID = "S01-done"

	t.Run("next planned sibling → implement", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				"S01-done": {ID: "S01-done", State: state.Verified, Track: "T1-core"},
				"S02-next": {ID: "S02-next", State: state.Planned, Track: "T1-core"},
			},
			board: &board.BoardState{
				Release: "test-release",
				Tracks: []board.TrackState{
					{
						ID:    "T1-core",
						State: "in_progress",
						Slices: []board.SliceState{
							{ID: "S01-done", State: state.Verified, Track: "T1-core"},
							{ID: "S02-next", State: state.Planned, Track: "T1-core"},
						},
					},
				},
			},
		}

		d, err := Route(context.Background(), oracle, &fakeContent{}, input)
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextImplement {
			t.Errorf("next planned sibling should route implement, got %s", d.NextType)
		}
		if d.TargetSlice != "S02-next" {
			t.Errorf("TargetSlice should be S02-next, got %s", d.TargetSlice)
		}
	})

	t.Run("track done, others ongoing → merge-track", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				"S01-done": {ID: "S01-done", State: state.Verified, Track: "T1-core"},
				"S02-beta": {ID: "S02-beta", State: state.Planned, Track: "T2-aux"},
			},
			board: &board.BoardState{
				Release: "test-release",
				Tracks: []board.TrackState{
					{
						ID:    "T1-core",
						State: "in_progress",
						Slices: []board.SliceState{
							{ID: "S01-done", State: state.Verified, Track: "T1-core"},
						},
					},
					{
						ID:    "T2-aux",
						State: "in_progress",
						Slices: []board.SliceState{
							{ID: "S02-beta", State: state.Planned, Track: "T2-aux"},
						},
					},
				},
			},
		}

		d, err := Route(context.Background(), oracle, &fakeContent{}, input)
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextMergeTrack {
			t.Errorf("track done with others ongoing should route merge-track, got %s", d.NextType)
		}
	})

	t.Run("all terminal, all merged → merge-release", func(t *testing.T) {
		oracle := &fakeOracle{
			slices: map[string]board.SliceState{
				"S01-done": {ID: "S01-done", State: state.Verified, Track: "T1-core"},
				"S02-done": {ID: "S02-done", State: state.Verified, Track: "T1-core"},
			},
			board: &board.BoardState{
				Release: "test-release",
				Tracks: []board.TrackState{
					{
						ID:             "T1-core",
						State:          "in_progress",
						WorktreeBranch: "track/test-release/T1-core",
						Slices: []board.SliceState{
							{ID: "S01-done", State: state.Verified, Track: "T1-core"},
							{ID: "S02-done", State: state.Verified, Track: "T1-core"},
						},
					},
				},
			},
		}
		content := &fakeContent{
			ancestors: map[string]bool{
				"track/test-release/T1-core|refs/heads/release-wt/test-release": true,
			},
		}

		d, err := Route(context.Background(), oracle, content, input)
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.NextType != NextMergeRelease {
			t.Errorf("all terminal + merged should route merge-release, got %s", d.NextType)
		}
	})
}

func TestGhostSliceFiltered(t *testing.T) {
	input := defaultInput()
	input.SliceID = "S01-done"

	// S02-ghost appears in T1-core's slices but is owned by T2-aux.
	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-done":  {ID: "S01-done", State: state.Verified, Track: "T1-core"},
			"S02-ghost": {ID: "S02-ghost", State: state.Planned, Track: "T2-aux"},
			"S03-real":  {ID: "S03-real", State: state.Planned, Track: "T1-core"},
		},
		board: &board.BoardState{
			Release: "test-release",
			Tracks: []board.TrackState{
				{
					ID:    "T1-core",
					State: "in_progress",
					Slices: []board.SliceState{
						{ID: "S01-done", State: state.Verified, Track: "T1-core"},
						{ID: "S02-ghost", State: state.Planned, Track: "T2-aux"}, // ghost!
						{ID: "S03-real", State: state.Planned, Track: "T1-core"},
					},
				},
			},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, input)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextImplement {
		t.Errorf("ghost filter should skip S02-ghost and route S03-real, got %s", d.NextType)
	}
	if d.TargetSlice != "S03-real" {
		t.Errorf("TargetSlice should be S03-real (ghost skipped), got %s", d.TargetSlice)
	}
}

// TestDeferredSkippedInTrackWalk verifies pin 4: deferred is terminal in track walk.
func TestDeferredSkippedInTrackWalk(t *testing.T) {
	input := defaultInput()
	input.SliceID = "S01-done"

	oracle := &fakeOracle{
		slices: map[string]board.SliceState{
			"S01-done":     {ID: "S01-done", State: state.Verified, Track: "T1-core"},
			"S02-deferred": {ID: "S02-deferred", State: state.Deferred, Track: "T1-core"},
			"S03-next":     {ID: "S03-next", State: state.Planned, Track: "T1-core"},
		},
		board: &board.BoardState{
			Release: "test-release",
			Tracks: []board.TrackState{
				{
					ID:    "T1-core",
					State: "in_progress",
					Slices: []board.SliceState{
						{ID: "S01-done", State: state.Verified, Track: "T1-core"},
						{ID: "S02-deferred", State: state.Deferred, Track: "T1-core"},
						{ID: "S03-next", State: state.Planned, Track: "T1-core"},
					},
				},
			},
		},
	}

	d, err := Route(context.Background(), oracle, &fakeContent{}, input)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.NextType != NextImplement {
		t.Errorf("deferred should be skipped, routing to S03-next, got %s", d.NextType)
	}
	if d.TargetSlice != "S03-next" {
		t.Errorf("TargetSlice should be S03-next (deferred skipped), got %s", d.TargetSlice)
	}
}