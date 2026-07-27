package baton

import "testing"

func TestStatePreservesRetiredSliceHistories(t *testing.T) {
	for _, test := range []struct {
		name          string
		initialTracks []Track
		revisedTracks []Track
	}{
		{
			name: "slice_removed_from_retained_track",
			initialTracks: []Track{{
				ID: "T1", DependsOn: []string{},
				Slices: []Slice{
					actionSlice("S1", "one.txt"),
					actionSlice("S2", "two.txt"),
				},
			}},
			revisedTracks: []Track{{
				ID: "T1", DependsOn: []string{},
				Slices: []Slice{actionSlice("S1", "one.txt")},
			}},
		},
		{
			name: "slice_removed_with_whole_track",
			initialTracks: []Track{
				{
					ID: "T1", DependsOn: []string{},
					Slices: []Slice{actionSlice("S1", "one.txt")},
				},
				{
					ID: "T2", DependsOn: []string{},
					Slices: []Slice{actionSlice("S2", "two.txt")},
				},
			},
			revisedTracks: []Track{{
				ID: "T1", DependsOn: []string{},
				Slices: []Slice{actionSlice("S1", "one.txt")},
			}},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			repoPath, repository, actions := createActionHarness(t)
			release := "history-" + test.name
			recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(
					release, 1, nil, test.initialTracks,
				),
				Summary: "Approve historical projection.",
				Detail:  []byte("approval one"),
			})
			if err != nil {
				t.Fatal(err)
			}
			advanceActionSlice(
				t, actions, repoPath, release, "T1", "S1",
				"one.txt", 1000002400, "pass",
			)
			s2Track := "T1"
			if test.name == "slice_removed_with_whole_track" {
				s2Track = "T2"
			}
			advanceActionSlice(
				t, actions, repoPath, release, s2Track, "S2",
				"two.txt", 1000002500, "pass",
			)

			previous := recorded.Plan
			if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
				PlanBytes: actionPlanRevisionBytes(
					release, 2, &previous, test.revisedTracks,
				),
				Summary: "Retire S2.",
				Detail:  []byte("approval two"),
			}); err != nil {
				t.Fatal(err)
			}

			state := readActionState(t, repository, release)
			if _, present := state.Slice("S2"); present {
				t.Fatal("retired S2 remained in the current slice projection")
			}
			if len(state.Slices) != 1 || state.Slices[0].Location.Slice.ID != "S1" {
				t.Fatalf("current slices = %#v", state.Slices)
			}
			if len(state.Tracks) != 1 || state.Tracks[0].ID != "T1" ||
				len(state.Tracks[0].Slices) != 1 {
				t.Fatalf("current tracks = %#v", state.Tracks)
			}
			if len(state.Refs.Tracks) != 1 || state.Refs.Tracks[0].ID != "T1" {
				t.Fatalf("current track refs = %#v", state.Refs.Tracks)
			}

			history, present := state.HistoryForSlice("S2")
			if !present {
				t.Fatal("retired S2 history is not discoverable")
			}
			if history.Slice != "S2" || history.Track != s2Track ||
				history.Ref != trackRef(release, s2Track) ||
				history.History.MaximumAttempt != 1 ||
				len(history.History.Entries) != 4 {
				t.Fatalf("retired S2 history = %#v", history)
			}
			entries := history.History.Entries
			if entries[0].Receipt.Role != "implementer" ||
				entries[0].Receipt.Result != "designed" ||
				entries[1].Receipt.Role != "captain" ||
				entries[1].Receipt.Result != "proceed" ||
				entries[2].Receipt.Role != "implementer" ||
				entries[2].Receipt.Result != "candidate" ||
				entries[3].Receipt.Role != "verifier" ||
				entries[3].Receipt.Result != "pass" {
				t.Fatalf("retired S2 entries = %#v", entries)
			}
			if _, present := state.HistoryForSlice("missing"); present {
				t.Fatal("unknown slice has a historical projection")
			}
			if _, present := state.HistoryForSlice("S1"); !present ||
				len(state.SliceHistories) != 2 {
				t.Fatalf("slice histories = %#v", state.SliceHistories)
			}
			if test.name == "slice_removed_with_whole_track" {
				if _, present := state.Track("T2"); present {
					t.Fatal("retired T2 remained in the current track projection")
				}
			}
		})
	}
}
