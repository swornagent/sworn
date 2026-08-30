package baton

import "testing"

// candidateReadyForVerification replays advanceActionSliceProduct's own
// design/proceed/candidate steps (actions_integration_test.go) but stops
// short of its baked-in verifier call, so a test can supply its own
// check-results bytes at the verifier step.
func candidateReadyForVerification(
	t *testing.T,
	actions *Actions,
	repoPath, release, track, slice, file string,
	timestamp int64,
) string {
	t.Helper()
	prepareActionSliceBase(t, actions, release, slice)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "implementer", Result: "designed",
		Summary: "Design " + slice + ".", Detail: []byte("design " + slice),
	})
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "captain", Result: "proceed",
		Summary: "Proceed " + slice + ".", Detail: []byte("review " + slice),
	})
	base := prepareActionSliceBase(t, actions, release, slice)
	ref := "refs/heads/track/" + release + "/" + track
	parent := actionGit(t, repoPath, nil, nil, "rev-parse", "--verify", ref)
	candidate := commitActionProduct(t, repoPath, parent, ref, file, slice+"\n", timestamp)
	appendActionReceipt(t, actions, AppendReceiptInput{
		Release: release, Slice: slice, Role: "implementer", Result: "candidate",
		Summary: "Candidate " + slice + ".", Detail: []byte("implementation " + slice),
		Base: base, Candidate: candidate, CheckResults: []byte("checks " + slice + "\n"),
	})
	return candidate
}

func roleCheckResultsManifest(t *testing.T, check, contractDigest string, covering bool) []byte {
	t.Helper()
	entryCheck := "check S1"
	if !covering {
		entryCheck = "an unrelated recorded command"
	}
	raw, err := EncodeCheckResults(CheckResults{
		SchemaVersion:  CheckResultsVersion,
		ContractDigest: contractDigest,
		Entries: []CheckResultEntry{
			{
				Check: entryCheck, Provenance: CheckProvenanceRole,
				Outcome: CheckOutcomePass, RoleDigest: "sha256:" + repeatHex("a"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func repeatHex(char string) string {
	result := ""
	for len(result) < 64 {
		result += char
	}
	return result
}

// TestAppendReceiptRefusesManifestMissingDeclaredCheckCoverage pins S5-A3's
// appendReceipt backstop: a well-formed sworn.check-results/v1 manifest with
// no entry covering the contract's declared check refuses the pass with
// CHECK_EVIDENCE_INCOMPLETE, over a legacy inline-plan fixture (S1's
// declared check is "check S1", actionSlice's own convention).
func TestAppendReceiptRefusesManifestMissingDeclaredCheckCoverage(t *testing.T) {
	repoPath, _, actions := createActionHarness(t)
	release := "check-evidence-incomplete"
	s1 := actionSlice("S1", "one.txt")
	tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve.", Detail: []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	candidate := candidateReadyForVerification(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000002000)
	manifest := roleCheckResultsManifest(t, "check S1", "", false)
	_, err := actions.AppendReceipt(AppendReceiptInput{
		Release: release, Slice: "S1", Role: "verifier", Result: "pass",
		Summary: "Pass S1.", Detail: []byte("verification S1"),
		Candidate: candidate, CheckResults: manifest,
	})
	if ErrorCode(err) != "CHECK_EVIDENCE_INCOMPLETE" {
		t.Fatalf("error = %v, want CHECK_EVIDENCE_INCOMPLETE", err)
	}
}

// TestAppendReceiptAcceptsManifestCoveringEveryDeclaredCheck is the positive
// twin: the same fixture, a manifest whose one entry does cover "check S1"
// with outcome pass, is accepted.
func TestAppendReceiptAcceptsManifestCoveringEveryDeclaredCheck(t *testing.T) {
	repoPath, _, actions := createActionHarness(t)
	release := "check-evidence-complete"
	s1 := actionSlice("S1", "one.txt")
	tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve.", Detail: []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	candidate := candidateReadyForVerification(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000002100)
	manifest := roleCheckResultsManifest(t, "check S1", "", true)
	result, err := actions.AppendReceipt(AppendReceiptInput{
		Release: release, Slice: "S1", Role: "verifier", Result: "pass",
		Summary: "Pass S1.", Detail: []byte("verification S1"),
		Candidate: candidate, CheckResults: manifest,
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

// TestAppendReceiptRefusesManifestAssertingWrongContractDigest pins the
// binding check: a manifest that does assert a contract_digest, and asserts
// the wrong one, is refused STALE_BINDING even though its entries cover
// every declared check.
func TestAppendReceiptRefusesManifestAssertingWrongContractDigest(t *testing.T) {
	repoPath, _, actions := createActionHarness(t)
	release := "check-evidence-stale-binding"
	s1 := actionSlice("S1", "one.txt")
	tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve.", Detail: []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	candidate := candidateReadyForVerification(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000002200)
	manifest := roleCheckResultsManifest(t, "check S1", "sha256:"+repeatHex("f"), true)
	_, err := actions.AppendReceipt(AppendReceiptInput{
		Release: release, Slice: "S1", Role: "verifier", Result: "pass",
		Summary: "Pass S1.", Detail: []byte("verification S1"),
		Candidate: candidate, CheckResults: manifest,
	})
	if ErrorCode(err) != "STALE_BINDING" {
		t.Fatalf("error = %v, want STALE_BINDING", err)
	}
}

// TestAppendReceiptOpaqueCheckBytesFallBackUnchanged pins finding 3: bytes
// that do not parse as a sworn.check-results/v1 manifest - the shape every
// pre-existing fixture in this tree (and every internal/runtime fixture)
// carries - are accepted exactly as they were before this slice, whether or
// not they cover the declared check.
func TestAppendReceiptOpaqueCheckBytesFallBackUnchanged(t *testing.T) {
	repoPath, _, actions := createActionHarness(t)
	release := "check-evidence-opaque-unchanged"
	s1 := actionSlice("S1", "one.txt")
	tracks := []Track{{ID: "T1", DependsOn: []string{}, Slices: []Slice{s1}}}
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanRevisionBytes(release, 1, nil, tracks),
		Summary:   "Approve.", Detail: []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	candidate := candidateReadyForVerification(t, actions, repoPath, release, "T1", "S1", "one.txt", 1000002300)
	result, err := actions.AppendReceipt(AppendReceiptInput{
		Release: release, Slice: "S1", Role: "verifier", Result: "pass",
		Summary: "Pass S1.", Detail: []byte("verification S1"),
		Candidate: candidate, CheckResults: []byte("fresh checks S1\n"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}
