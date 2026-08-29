package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
)

func evidenceResealVerifierFailState() (baton.State, *baton.SliceState) {
	state := baton.State{Tracks: []baton.TrackState{{
		ID:   "T1",
		Head: "fail-receipt",
	}}}
	slice := &baton.SliceState{
		Location: baton.SliceLocation{
			Track: baton.Track{ID: "T1"},
			Slice: baton.Slice{ID: "S1"},
		},
		History:  baton.SliceHistory{MaximumAttempt: 1},
		Stage:    "implement",
		Status:   "ready",
		NextRole: "implementer",
		Outcome:  "fail",
		Attempt:  2,
		Retained: false,
		CurrentReceipt: &baton.ReceiptEntry{
			OID: "fail-receipt",
			Receipt: baton.Receipt{
				Role: "verifier", Result: "fail",
				FailScope: strPtr("evidence"),
			},
		},
		Candidate: &baton.ReceiptEntry{OID: "candidate-receipt"},
	}
	return state, slice
}

func strPtr(value string) *string { return &value }

func TestEvidenceOnlyResealMatchesExactlyTheVerifierFailShape(t *testing.T) {
	state, slice := evidenceResealVerifierFailState()
	if !evidenceOnlyReseal(state, slice) {
		t.Fatal("exact evidence-only verifier/fail state was not recognized")
	}
	// Mutually exclusive with candidateHeadRefresh: that gate needs
	// CurrentReceipt.OID == Candidate.OID, which never holds here.
	if candidateHeadRefresh(state, slice) {
		t.Fatal("verifier/fail state was misrecognized as a candidate head refresh")
	}

	for name, mutate := range map[string]func(baton.State, *baton.SliceState) (baton.State, *baton.SliceState){
		"code-scoped fail": func(s baton.State, sl *baton.SliceState) (baton.State, *baton.SliceState) {
			sl.CurrentReceipt.Receipt.FailScope = nil
			return s, sl
		},
		"unspecified scope": func(s baton.State, sl *baton.SliceState) (baton.State, *baton.SliceState) {
			sl.CurrentReceipt.Receipt.FailScope = strPtr("")
			return s, sl
		},
		"track moved since the fail receipt": func(s baton.State, sl *baton.SliceState) (baton.State, *baton.SliceState) {
			s.Tracks[0].Head = "some-later-commit"
			return s, sl
		},
		"retained across a plan revision": func(s baton.State, sl *baton.SliceState) (baton.State, *baton.SliceState) {
			sl.Retained = true
			return s, sl
		},
		"stale outcome, not fail": func(s baton.State, sl *baton.SliceState) (baton.State, *baton.SliceState) {
			sl.Outcome = "stale"
			return s, sl
		},
		"no candidate": func(s baton.State, sl *baton.SliceState) (baton.State, *baton.SliceState) {
			sl.Candidate = nil
			return s, sl
		},
	} {
		t.Run(name, func(t *testing.T) {
			state, slice := evidenceResealVerifierFailState()
			state, slice = mutate(state, slice)
			if evidenceOnlyReseal(state, slice) {
				t.Fatalf("%s was recognized as an earned evidence-only reseal", name)
			}
		})
	}
}

func TestSealedRefreshRecordAdmitsEvidenceResealBeyondBinds(t *testing.T) {
	cycle := implementationCycle{GitIdentity: runtimeTestGitIdentity,
		Release:         "release-1",
		Slice:           "S1",
		Binds:           "fail-receipt",
		TrackHead:       "fail-receipt",
		RefreshFrom:     "pre-change-base",
		RefreshEvidence: true,
	}
	record := sealedRecord{
		Slice:       cycle.Slice,
		Binds:       cycle.Binds,
		Before:      cycle.TrackHead,
		RefreshFrom: cycle.RefreshFrom,
		Candidate:   "fail-receipt",
		ProductTree: "sha256:product",
		Receipt: baton.AppendReceiptInput{
			Release:   cycle.Release,
			Slice:     cycle.Slice,
			Role:      "implementer",
			Result:    "candidate",
			Candidate: "fail-receipt",
		},
	}
	if !sealedRecordMatchesCycle(record, cycle) {
		t.Fatal("evidence-reseal record with RefreshFrom older than Binds did not match its cycle")
	}
	// Without the earned flag, the same shape - RefreshFrom != Binds - is
	// refused: this is cycle self-consistency, not the authority check, and
	// the ordinary candidateHeadRefresh invariant (RefreshFrom == Binds)
	// must still hold when no evidence reseal is in play.
	notEarned := cycle
	notEarned.RefreshEvidence = false
	if sealedRecordMatchesCycle(record, notEarned) {
		t.Fatal("RefreshFrom older than Binds matched without the earned flag")
	}
	// A record whose RefreshFrom disagrees with the cycle's is still refused
	// even when RefreshEvidence is set.
	drifted := record
	drifted.RefreshFrom = "other-base"
	if sealedRecordMatchesCycle(drifted, cycle) {
		t.Fatal("drifted record RefreshFrom matched an evidence-reseal cycle")
	}
}

func runRuntimeGitIdentity(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	return runRuntimeGit(t, repository, append([]string{
		"-c", "user.name=Runtime Test",
		"-c", "user.email=test@example.invalid",
	}, arguments...)...)
}

// TestEvidenceResealBaseWalksBackPastChainOfReseals pins Captain correction
// 1: after two consecutive evidence-only reseals, evidenceResealBase must
// resolve the parent of the real content commit, not the parent of an
// intervening metadata (receipt) commit that shares the content's tree -
// that intervening parent produces an empty diff and re-deadlocks
// EMPTY_CANDIDATE one attempt later.
func TestEvidenceResealBaseWalksBackPastChainOfReseals(t *testing.T) {
	repoPath := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	repoView, err := gitx.Open(repoPath, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	eng := &engine{repository: repoView}

	base := runRuntimeGit(t, repoPath, "rev-parse", "HEAD")
	if err := os.WriteFile(
		filepath.Join(repoPath, "content.txt"),
		[]byte("content\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repoPath, "add", "--", "content.txt")
	runRuntimeGitIdentity(t, repoPath, "commit", "--quiet", "-m", "content")
	content := runRuntimeGit(t, repoPath, "rev-parse", "HEAD")
	contentTree := runRuntimeGit(t, repoPath, "rev-parse", "HEAD^{tree}")

	commitTree := func(parent string) string {
		return runRuntimeGitIdentity(t, repoPath, "commit-tree", contentTree, "-p", parent, "-m", "receipt")
	}
	i1 := commitTree(content)
	v1 := commitTree(i1)
	i2 := commitTree(v1) // first reseal: Candidate == Binds == v1
	v2 := commitTree(i2)
	i3 := commitTree(v2) // second reseal: Candidate == Binds == v2

	entry := func(oid, parent, role, result string, candidate *string, binds string) baton.ReceiptEntry {
		return baton.ReceiptEntry{
			OID: oid, Parent: parent,
			Receipt: baton.Receipt{Role: role, Result: result, Candidate: candidate, Binds: binds},
		}
	}
	contentCopy, v1Copy, v2Copy := content, v1, v2
	entries := []baton.ReceiptEntry{
		entry(i1, content, "implementer", "candidate", &contentCopy, "captain-proceed-placeholder"),
		entry(v1, i1, "verifier", "fail", &contentCopy, i1),
		entry(i2, v1, "implementer", "candidate", &v1Copy, v1),
		entry(v2, i2, "verifier", "fail", &v1Copy, i2),
		entry(i3, v2, "implementer", "candidate", &v2Copy, v2),
	}
	slice := &baton.SliceState{
		History:   baton.SliceHistory{Entries: entries},
		Candidate: &entries[4],
	}

	got, err := evidenceResealBase(eng, slice)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != base {
		t.Fatalf("evidenceResealBase = %s, want the pre-content base %s", got.String(), base)
	}
}
