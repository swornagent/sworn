package baton

import "testing"

func TestRC13DescendantTargetDoesNotInvalidateApprovedWork(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "rc13-descendant-target"
	plan := actionPlanBytes(release)
	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: plan,
		Summary:   "Approve RC13 descendant target behavior.",
		Detail:    []byte("approval"),
	})
	if err != nil {
		t.Fatal(err)
	}
	approvedTarget := recorded.Target
	currentTarget := commitActionProduct(
		t,
		repoPath,
		approvedTarget,
		"refs/heads/main",
		"forward.txt",
		"ordinary forward target movement\n",
		1000004000,
	)

	state := readActionState(t, repository, release)
	if state.Plan.TargetStale || state.Refs.Target.Head != currentTarget {
		t.Fatalf("forward target state = plan %#v refs %#v", state.Plan, state.Refs.Target)
	}
	for _, diagnostic := range state.Diagnostics {
		if diagnostic.Code == "TARGET_DIVERGED" {
			t.Fatalf("forward target emitted divergence: %#v", diagnostic)
		}
	}

	retry, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: plan,
		Summary:   "Approve RC13 descendant target behavior.",
		Detail:    []byte("approval"),
	})
	if err != nil || retry.Changed || retry.Target != approvedTarget {
		t.Fatalf("exact plan retry = %#v, error = %v", retry, err)
	}
}

func TestRC13DivergentTargetStopsActions(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "rc13-divergent-target"
	plan := actionPlanBytes(release)
	recorded, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: plan,
		Summary:   "Approve RC13 divergent target behavior.",
		Detail:    []byte("approval"),
	})
	if err != nil {
		t.Fatal(err)
	}
	designInput := AppendReceiptInput{
		Release: release,
		Slice:   "S1",
		Role:    "implementer",
		Result:  "designed",
		Summary: "Record work before target divergence.",
		Detail:  []byte("design"),
	}
	designed, err := actions.AppendReceipt(designInput)
	if err != nil || !designed.Changed {
		t.Fatalf("initial design = %#v, error = %v", designed, err)
	}
	tree := actionGit(
		t,
		repoPath,
		nil,
		nil,
		"rev-parse",
		recorded.Target+"^{tree}",
	)
	divergent := actionGit(
		t,
		repoPath,
		[]byte("replace target history\n"),
		nil,
		"commit-tree",
		tree,
	)
	actionGit(
		t,
		repoPath,
		nil,
		nil,
		"update-ref",
		"refs/heads/main",
		divergent,
	)

	state := readActionState(t, repository, release)
	if !state.Plan.TargetStale || len(state.Diagnostics) == 0 ||
		state.Diagnostics[0].Code != "TARGET_DIVERGED" {
		t.Fatalf("divergent target state = plan %#v diagnostics %#v", state.Plan, state.Diagnostics)
	}
	_, err = actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: plan,
		Summary:   "Approve RC13 divergent target behavior.",
		Detail:    []byte("approval"),
	})
	if ErrorCode(err) != "TARGET_DIVERGED" {
		t.Fatalf("divergent plan retry error = %v", err)
	}
	retry, err := actions.AppendReceipt(designInput)
	if err != nil || retry.Changed || retry.ReceiptCommit != designed.ReceiptCommit {
		t.Fatalf("divergent exact receipt retry = %#v, error = %v", retry, err)
	}
	_, err = actions.AppendReceipt(AppendReceiptInput{
		Release: release,
		Slice:   "S1",
		Role:    "captain",
		Result:  "proceed",
		Summary: "Do not start divergent work.",
		Detail:  []byte("review"),
	})
	if ErrorCode(err) != "TARGET_DIVERGED" {
		t.Fatalf("divergent receipt error = %v", err)
	}
}

func TestRC13TargetAdvanceRebuildsOnlyAssembly(t *testing.T) {
	repoPath, repository, actions := createActionHarness(t)
	release := "rc13-rebuild-assembly"
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release),
		Summary:   "Approve RC13 assembly rebuild behavior.",
		Detail:    []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	advanceActionSlice(
		t, actions, repoPath, release, "T1", "S1",
		"one.txt", 1000004100, "pass",
	)
	advanceActionSlice(
		t, actions, repoPath, release, "T2", "S2",
		"two.txt", 1000004200, "pass",
	)
	first, err := actions.PrepareAssembly(PrepareAssemblyInput{
		Release: release,
		Summary: "Prepare first RC13 assembly.",
		Detail:  []byte("assembly one"),
	})
	if err != nil || !first.Changed || first.Direct {
		t.Fatalf("first assembly = %#v, error = %v", first, err)
	}
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release:      release,
		Role:         "verifier",
		Result:       "pass",
		Summary:      "Pass first RC13 assembly.",
		Detail:       []byte("verification one"),
		Candidate:    first.Candidate,
		CheckResults: []byte("assembly checks one\n"),
	})

	before := readActionState(t, repository, release)
	if before.Assembly.Pass == nil {
		t.Fatal("first assembly has no PASS")
	}
	currentTarget := commitActionProduct(
		t,
		repoPath,
		before.Refs.Target.Head,
		"refs/heads/main",
		"forward.txt",
		"target advanced after assembly pass\n",
		1000004300,
	)
	stale := readActionState(t, repository, release)
	if stale.Plan.TargetStale || stale.Assembly.Outcome != "stale" {
		t.Fatalf("advanced target state = plan %#v assembly %#v", stale.Plan, stale.Assembly)
	}
	for _, slice := range stale.Slices {
		if slice.Pass == nil {
			t.Fatalf("target advance cleared %s PASS", slice.Location.Slice.ID)
		}
	}

	second, err := actions.PrepareAssembly(PrepareAssemblyInput{
		Release: release,
		Summary: "Rebuild RC13 assembly on the latest target.",
		Detail:  []byte("assembly two"),
	})
	if err != nil || !second.Changed || second.Direct ||
		second.Candidate == first.Candidate {
		t.Fatalf("rebuilt assembly = %#v, error = %v", second, err)
	}
	contained, err := actions.repository.isAncestor(currentTarget, second.Candidate)
	if err != nil || !contained {
		t.Fatalf("rebuilt assembly omits latest target: contained=%t error=%v", contained, err)
	}
}
