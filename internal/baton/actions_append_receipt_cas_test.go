package baton

import "testing"

func TestAppendReceiptTargetMoveAfterClassificationDoesNotMutateOwner(t *testing.T) {
	t.Run("work_receipt", func(t *testing.T) {
		repoPath, _, actions := createActionHarness(t)
		release := "append-receipt-work-target-cas"
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanBytes(release),
			Summary:   "Approve work receipt CAS.", Detail: []byte("approval"),
		}); err != nil {
			t.Fatal(err)
		}
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "designed",
			Summary: "Design S1.", Detail: []byte("design"),
		})

		ownerRef := trackRef(release, "T1")
		ownerBefore := actionGit(t, repoPath, nil, nil, "rev-parse", ownerRef)
		releaseBefore := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
		targetBefore := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
		var targetAfterMove string
		_, err := actions.appendReceipt(AppendReceiptInput{
			Release: release, Slice: "S1", Role: "captain", Result: "proceed",
			Summary: "Proceed S1.", Detail: []byte("review"),
		}, func() {
			targetAfterMove = moveAppendReceiptTarget(
				t, repoPath, "move target during work receipt classification",
			)
		})
		if ErrorCode(err) != "REF_TRANSACTION_RECOVERY_REQUIRED" {
			t.Fatalf("target interleaving error = %v", err)
		}
		if targetAfterMove == "" || targetAfterMove == targetBefore ||
			actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main") != targetAfterMove {
			t.Fatal("work receipt interleaving did not move the target")
		}
		if ownerAfter := actionGit(t, repoPath, nil, nil, "rev-parse", ownerRef); ownerAfter != ownerBefore {
			t.Fatalf("work receipt moved owner ref from %s to %s", ownerBefore, ownerAfter)
		}
		if releaseAfter := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release)); releaseAfter != releaseBefore {
			t.Fatalf("work receipt moved release ref from %s to %s", releaseBefore, releaseAfter)
		}
	})

	t.Run("consumed_source", func(t *testing.T) {
		repoPath, _, actions := createActionHarness(t)
		release := "append-receipt-consumed-source-cas"
		s1 := actionSlice("S1", "one.txt")
		s2 := actionSlice("S2", "two.txt")
		s2.Consumes = []string{"S1"}
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, []Track{
				{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}},
				{ID: "T2", DependsOn: []string{}, Slices: []Slice{s2}},
			}),
			Summary: "Approve consumed source CAS.",
			Detail:  []byte("approval"),
		}); err != nil {
			t.Fatal(err)
		}
		advanceActionSlice(
			t,
			actions,
			repoPath,
			release,
			"T1",
			"S1",
			"one.txt",
			1000003200,
			"pass",
		)
		prepareActionSliceBase(t, actions, release, "S2")

		ownerRef := trackRef(release, "T2")
		ownerBefore := actionGit(
			t, repoPath, nil, nil, "rev-parse", ownerRef,
		)
		producerRef := trackRef(release, "T1")
		producerBefore := actionGit(
			t, repoPath, nil, nil, "rev-parse", producerRef,
		)
		var producerAfter string
		_, err := actions.appendReceipt(AppendReceiptInput{
			Release: release, Slice: "S2", Role: "implementer",
			Result: "designed", Summary: "Design S2.",
			Detail: []byte("design"),
		}, func() {
			tree := actionGit(
				t,
				repoPath,
				nil,
				nil,
				"rev-parse",
				producerBefore+"^{tree}",
			)
			producerAfter = actionGit(
				t,
				repoPath,
				[]byte("move consumed source\n"),
				nil,
				"commit-tree",
				tree,
				"-p",
				producerBefore,
			)
			actionGit(
				t,
				repoPath,
				nil,
				nil,
				"update-ref",
				producerRef,
				producerAfter,
				producerBefore,
			)
		})
		if ErrorCode(err) != "REF_TRANSACTION_RECOVERY_REQUIRED" {
			t.Fatalf("consumed-source interleaving error = %v", err)
		}
		if producerAfter == "" ||
			actionGit(t, repoPath, nil, nil, "rev-parse", producerRef) !=
				producerAfter {
			t.Fatal("consumed-source interleaving did not move producer")
		}
		if ownerAfter := actionGit(
			t, repoPath, nil, nil, "rev-parse", ownerRef,
		); ownerAfter != ownerBefore {
			t.Fatalf(
				"consumed-source interleaving moved owner from %s to %s",
				ownerBefore,
				ownerAfter,
			)
		}
	})

	t.Run("assembly_receipt", func(t *testing.T) {
		repoPath, _, actions := createActionHarness(t)
		release := "append-receipt-assembly-target-cas"
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanBytes(release),
			Summary:   "Approve assembly receipt CAS.", Detail: []byte("approval"),
		}); err != nil {
			t.Fatal(err)
		}
		advanceActionSlice(
			t, actions, repoPath, release, "T1", "S1",
			"one.txt", 1000003000, "pass",
		)
		advanceActionSlice(
			t, actions, repoPath, release, "T2", "S2",
			"two.txt", 1000003100, "pass",
		)
		assembly, err := actions.PrepareAssembly(PrepareAssemblyInput{
			Release: release, Summary: "Prepare assembly.", Detail: []byte("assembly"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !assembly.Changed || assembly.Direct || assembly.Candidate == "" {
			t.Fatalf("assembly candidate = %#v", assembly)
		}

		ownerRef := releaseRef(release)
		ownerBefore := actionGit(t, repoPath, nil, nil, "rev-parse", ownerRef)
		targetBefore := actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main")
		var targetAfterMove string
		_, err = actions.appendReceipt(AppendReceiptInput{
			Release: release, Role: "verifier", Result: "pass",
			Summary: "Pass assembly.", Detail: []byte("verification"),
			Candidate: assembly.Candidate, CheckResults: []byte("fresh assembly checks\n"),
		}, func() {
			targetAfterMove = moveAppendReceiptTarget(
				t, repoPath, "move target during assembly receipt classification",
			)
		})
		if ErrorCode(err) != "REF_TRANSACTION_RECOVERY_REQUIRED" {
			t.Fatalf("target interleaving error = %v", err)
		}
		if targetAfterMove == "" || targetAfterMove == targetBefore ||
			actionGit(t, repoPath, nil, nil, "rev-parse", "refs/heads/main") != targetAfterMove {
			t.Fatal("assembly receipt interleaving did not move the target")
		}
		if ownerAfter := actionGit(t, repoPath, nil, nil, "rev-parse", ownerRef); ownerAfter != ownerBefore {
			t.Fatalf("assembly receipt moved owner ref from %s to %s", ownerBefore, ownerAfter)
		}
	})
}

func moveAppendReceiptTarget(t *testing.T, repoPath, message string) string {
	t.Helper()
	targetRef := "refs/heads/main"
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", targetRef)
	tree := actionGit(t, repoPath, nil, nil, "rev-parse", parent+"^{tree}")
	next := actionGit(t, repoPath, []byte(message+"\n"), nil, "commit-tree", tree, "-p", parent)
	actionGit(t, repoPath, nil, nil, "update-ref", targetRef, next, parent)
	return next
}
