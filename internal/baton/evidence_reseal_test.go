package baton

import "testing"

// TestEvidenceResealAdmitsCandidateEqualsBindsOnlyWhenEarned pins A2's
// deriveSliceHistory narrowing directly against the Actions API: a verifier
// fail whose durable FailScope is "evidence" lets the next implementer
// candidate bind Candidate == Binds (no new content commit) and the slice
// proceeds cleanly to a fresh verification and PASS; the identical shape
// following an ordinary (non-evidence) fail still corrupts the slice's
// derived history, because the general CHANGED_CANDIDATE rule survives for
// every other case.
func TestEvidenceResealAdmitsCandidateEqualsBindsOnlyWhenEarned(t *testing.T) {
	t.Parallel()
	t.Run("evidence_scope_fail_admits_reseal_then_passes", func(t *testing.T) {
		t.Parallel()
		repoPath, _, actions := createActionHarness(t)
		release := "evidence-reseal-admits"
		tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{actionSlice("S1", "one.txt")}}}
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
			Summary:   "Approve.", Detail: []byte("approval"),
		}); err != nil {
			t.Fatal(err)
		}
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "designed",
			Summary: "Design S1.", Detail: []byte("design"),
		})
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "captain", Result: "proceed",
			Summary: "Proceed S1.", Detail: []byte("review"),
		})
		ref := "refs/heads/track/" + release + "/T1"
		parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
		candidate := commitActionProduct(t, repoPath, parent, ref, "one.txt", "content\n", 1000000100)
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
			Summary: "Candidate S1.", Detail: []byte("implementation"),
			Candidate: candidate, CheckResults: []byte("checks\n"),
		})
		fail := appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "verifier", Result: "fail",
			Summary: "Fail evidence only.", Detail: []byte("verification"),
			Candidate: candidate, CheckResults: []byte("fresh checks\n"),
			FailScope: "evidence",
		})

		// The reseal: nothing new staged, so the new candidate receipt binds
		// to the fail receipt's own commit, exactly as SealTrackRefreshGuardedWithClaim
		// produces when the physical workspace stages no diff.
		reseal := appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
			Summary: "Reseal unchanged S1.", Detail: []byte("reseal"),
			Candidate: fail.ReceiptCommit, CheckResults: []byte("reseal checks\n"),
		})
		if reseal.Receipt == nil || reseal.Receipt.Candidate == nil ||
			*reseal.Receipt.Candidate != fail.ReceiptCommit ||
			reseal.Receipt.Binds != fail.ReceiptCommit {
			t.Fatalf("reseal receipt = %#v", reseal.Receipt)
		}

		state, err := actions.stateFor(release)
		if err != nil {
			t.Fatalf("state after reseal: %v", err)
		}
		slice, ok := state.Slice("S1")
		if !ok || slice.NextRole != "verifier" || slice.Stage != "verify" ||
			slice.Attempt != 2 {
			t.Fatalf("post-reseal slice state = %#v", slice)
		}

		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "verifier", Result: "pass",
			Summary: "Pass S1.", Detail: []byte("verification"),
			Candidate: fail.ReceiptCommit, CheckResults: []byte("final checks\n"),
		})
		final, err := actions.stateFor(release)
		if err != nil {
			t.Fatalf("state after pass: %v", err)
		}
		finalSlice, ok := final.Slice("S1")
		if !ok || finalSlice.Pass == nil {
			t.Fatalf("final slice has no PASS: %#v", finalSlice)
		}
	})

	t.Run("two_consecutive_evidence_fails_admit_reseal_chain", func(t *testing.T) {
		t.Parallel()
		repoPath, _, actions := createActionHarness(t)
		release := "evidence-reseal-chain"
		tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{actionSlice("S1", "one.txt")}}}
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
			Summary:   "Approve.", Detail: []byte("approval"),
		}); err != nil {
			t.Fatal(err)
		}
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "designed",
			Summary: "Design S1.", Detail: []byte("design"),
		})
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "captain", Result: "proceed",
			Summary: "Proceed S1.", Detail: []byte("review"),
		})
		ref := "refs/heads/track/" + release + "/T1"
		parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
		candidate := commitActionProduct(t, repoPath, parent, ref, "one.txt", "content\n", 1000000100)
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
			Summary: "Candidate S1.", Detail: []byte("implementation"),
			Candidate: candidate, CheckResults: []byte("checks\n"),
		})
		fail1 := appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "verifier", Result: "fail",
			Summary: "Fail evidence only, first.", Detail: []byte("verification"),
			Candidate: candidate, CheckResults: []byte("fresh checks\n"),
			FailScope: "evidence",
		})
		reseal1 := appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
			Summary: "Reseal unchanged S1, first.", Detail: []byte("reseal"),
			Candidate: fail1.ReceiptCommit, CheckResults: []byte("reseal checks 1\n"),
		})
		if reseal1.Receipt == nil || reseal1.Receipt.Candidate == nil ||
			*reseal1.Receipt.Candidate != fail1.ReceiptCommit {
			t.Fatalf("first reseal receipt = %#v", reseal1.Receipt)
		}
		fail2 := appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "verifier", Result: "fail",
			// A verifier binds *evidence.Candidate, which the first reseal
			// carried forward unchanged from fail1 - not reseal1's own OID.
			Summary: "Fail evidence only, second.", Detail: []byte("verification"),
			Candidate: fail1.ReceiptCommit, CheckResults: []byte("fresh checks 2\n"),
			FailScope: "evidence",
		})
		reseal2 := appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
			Summary: "Reseal unchanged S1, second.", Detail: []byte("reseal"),
			Candidate: fail2.ReceiptCommit, CheckResults: []byte("reseal checks 2\n"),
		})
		if reseal2.Receipt == nil || reseal2.Receipt.Candidate == nil ||
			*reseal2.Receipt.Candidate != fail2.ReceiptCommit {
			t.Fatalf("second reseal receipt = %#v", reseal2.Receipt)
		}
		state, err := actions.stateFor(release)
		if err != nil {
			t.Fatalf("state after second reseal: %v", err)
		}
		slice, ok := state.Slice("S1")
		if !ok || slice.NextRole != "verifier" || slice.Attempt != 3 {
			t.Fatalf("post-chain slice state = %#v", slice)
		}
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "verifier", Result: "pass",
			Summary: "Pass S1.", Detail: []byte("verification"),
			Candidate: fail2.ReceiptCommit, CheckResults: []byte("final checks\n"),
		})
		final, err := actions.stateFor(release)
		if err != nil {
			t.Fatalf("state after pass: %v", err)
		}
		finalSlice, ok := final.Slice("S1")
		if !ok || finalSlice.Pass == nil {
			t.Fatalf("final slice has no PASS: %#v", finalSlice)
		}
	})

	t.Run("code_scoped_fail_still_refuses_candidate_equals_binds", func(t *testing.T) {
		t.Parallel()
		repoPath, _, actions := createActionHarness(t)
		release := "evidence-reseal-refuses"
		tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{actionSlice("S1", "one.txt")}}}
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
			Summary:   "Approve.", Detail: []byte("approval"),
		}); err != nil {
			t.Fatal(err)
		}
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "designed",
			Summary: "Design S1.", Detail: []byte("design"),
		})
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "captain", Result: "proceed",
			Summary: "Proceed S1.", Detail: []byte("review"),
		})
		ref := "refs/heads/track/" + release + "/T1"
		parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
		candidate := commitActionProduct(t, repoPath, parent, ref, "one.txt", "content\n", 1000000100)
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
			Summary: "Candidate S1.", Detail: []byte("implementation"),
			Candidate: candidate, CheckResults: []byte("checks\n"),
		})
		// An ordinary, code-scoped fail: no FailScope.
		fail := appendActionReceipt(t, actions, AppendReceiptInput{
			Release: release, Slice: "S1", Role: "verifier", Result: "fail",
			Summary: "Fail on the code.", Detail: []byte("verification"),
			Candidate: candidate, CheckResults: []byte("fresh checks\n"),
		})
		if fail.Receipt == nil || fail.Receipt.FailScope != nil {
			t.Fatalf("ordinary fail unexpectedly carries fail_scope: %#v", fail.Receipt)
		}

		// The append itself is a physical git operation and succeeds; the
		// forged reseal is caught on the next state derivation, exactly like
		// every other CHANGED_CANDIDATE violation.
		if _, err := actions.AppendReceipt(AppendReceiptInput{
			Release: release, Slice: "S1", Role: "implementer", Result: "candidate",
			Summary: "Forged reseal S1.", Detail: []byte("reseal"),
			Candidate: fail.ReceiptCommit, CheckResults: []byte("reseal checks\n"),
		}); err != nil {
			t.Fatalf("append itself refused: %v", err)
		}
		if _, err := actions.stateFor(release); ErrorCode(err) != "CHANGED_CANDIDATE" {
			t.Fatalf("state after forged reseal: error = %v", err)
		}
	})
}
