package baton

import (
	"strings"
	"testing"
)

func TestReleaseEpochIgnoresPriorReceiptsWhenIdentitiesAreReused(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	oneTrack := []Track{{
		ID:        "T1",
		DependsOn: []string{},
		Slices:    []Slice{actionSlice("S1", "one.txt")},
	}}

	first, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes("epoch-a", 1, nil, oneTrack),
		Summary:   "Approve epoch A.",
		Detail:    []byte("approval A"),
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := advanceActionSlice(
		t,
		actions,
		repoPath,
		"epoch-a",
		"T1",
		"S1",
		"one.txt",
		1000002300,
		"pass",
	)
	merged, err := actions.MergePassedCandidate(MergePassedCandidateInput{
		Release: "epoch-a",
		Summary: "Merge the exact epoch A PASS.",
		Detail:  []byte("merge A"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.ResultCommit != candidate {
		t.Fatalf("epoch A result = %s, want %s", merged.ResultCommit, candidate)
	}

	second, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes("epoch-b", 1, nil, oneTrack),
		Summary:   "Approve epoch B.",
		Detail:    []byte("approval B"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Target != candidate {
		t.Fatalf("epoch B target = %s, want %s", second.Target, candidate)
	}
	state := readActionState(t, repository, "epoch-b")
	slice, ok := state.Slice("S1")
	if !ok || slice.History.MaximumAttempt != 0 ||
		len(slice.History.Entries) != 0 {
		t.Fatalf("epoch B inherited epoch A authority: %#v", slice)
	}

	movedTarget := commitActionProduct(
		t,
		repoPath,
		candidate,
		"refs/heads/main",
		"target.txt",
		"moved for epoch B revision 2\n",
		1000002350,
	)
	previous := second.Plan
	revised, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(
			"epoch-b",
			2,
			&previous,
			oneTrack,
		),
		Summary: "Approve epoch B revision 2.",
		Detail:  []byte("approval B2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Target != movedTarget {
		t.Fatalf("epoch B revision target = %s, want %s", revised.Target, movedTarget)
	}
	history, err := readReleaseReceiptHistory(
		actions.repository,
		"epoch-b",
		revised.Head,
	)
	if err != nil {
		t.Fatal(err)
	}
	if history.Boundary != candidate {
		t.Fatalf("epoch B boundary = %s, want %s", history.Boundary, candidate)
	}
	if first.Plan == second.Plan {
		t.Fatal("separate releases reused one plan object unexpectedly")
	}
}

func TestReleaseEpochFloorIgnoresMalformedReceiptBelowItButNotAboveIt(
	t *testing.T,
) {
	repoPath, repository, actions := createActionHarness(t)
	base := actionGit(
		t,
		repoPath,
		nil,
		nil,
		"rev-parse",
		"refs/heads/main",
	)
	inherited, err := actions.repository.prepareMetadata(
		base,
		[]byte("inherited malformed receipt\n\nBaton-Receipt: not-json\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	actionGit(
		t,
		repoPath,
		nil,
		nil,
		"update-ref",
		"refs/heads/main",
		inherited.Commit,
		base,
	)
	approved, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes("epoch-safe"),
		Summary:   "Approve the bounded epoch.",
		Detail:    []byte("approval"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Target != inherited.Commit {
		t.Fatalf(
			"release floor = %s, want inherited commit %s",
			approved.Target,
			inherited.Commit,
		)
	}
	_ = readActionState(t, repository, "epoch-safe")

	malformed, err := actions.repository.prepareMetadata(
		approved.Head,
		[]byte("malformed receipt inside epoch\n\nBaton-Receipt: not-json\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := releaseRef("epoch-safe")
	actionGit(
		t,
		repoPath,
		nil,
		nil,
		"update-ref",
		ref,
		malformed.Commit,
		approved.Head,
	)
	_, err = ReadState(
		UseGitRepository(repository),
		"epoch-safe",
		inertActionResolver,
	)
	if err == nil || !strings.Contains(err.Error(), "invalid receipt") {
		t.Fatalf("malformed in-epoch receipt error = %v", err)
	}
}

func TestReleaseEpochCannotBeMovedByReinstallingRevisionOne(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "epoch-replay"
	base := actionGit(
		t,
		repoPath,
		nil,
		nil,
		"rev-parse",
		"refs/heads/main",
	)
	anchor, err := actions.repository.prepareRecord(
		base,
		"test: add unrelated release record",
		map[string][]byte{
			planPath(RecordRoot, "anchor"): []byte("unrelated release record\n"),
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	actionGit(
		t,
		repoPath,
		nil,
		nil,
		"update-ref",
		"refs/heads/main",
		anchor.Commit,
		base,
	)
	planBytes := actionPlanBytes(release)
	original, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Approve the original revision-1 plan.",
		Detail:    []byte("approval"),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := planPath(RecordRoot, release)
	deleted, err := actions.repository.prepareRecord(
		original.Head,
		"test: delete the installed plan path",
		map[string][]byte{path: nil},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	reinstalled, err := actions.repository.prepareRecord(
		deleted.Commit,
		"test: reinstall the identical revision-1 plan",
		map[string][]byte{path: planBytes},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	replayed := original.Receipt.Clone()
	replayed.Binds = reinstalled.Commit
	replayed.Target = &deleted.Commit
	replayed.Summary = "Attempt to replace the original release epoch."
	message, err := RenderReceiptCommit(
		"approve the replayed revision-1 plan",
		nil,
		replayed,
	)
	if err != nil {
		t.Fatal(err)
	}
	replayApproval, err := actions.repository.prepareMetadata(
		reinstalled.Commit,
		message,
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := releaseRef(release)
	actionGit(
		t,
		repoPath,
		nil,
		nil,
		"update-ref",
		ref,
		replayApproval.Commit,
		original.Head,
	)
	_, err = ReadState(
		UseGitRepository(repository),
		release,
		inertActionResolver,
	)
	if ErrorCode(err) != "INVALID_PLAN_HISTORY" ||
		!strings.Contains(err.Error(), "already introduced") {
		t.Fatalf("replayed release epoch error = %v", err)
	}
}

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
