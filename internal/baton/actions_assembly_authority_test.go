package baton

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

type assemblyCASInterleaver struct {
	armedPath string
}

func newAssemblyCASInterleavingHarness(
	t *testing.T,
) (string, *Actions, *assemblyCASInterleaver) {
	t.Helper()
	repoPath := createActionRepository(t, "sha1")
	wrapperRoot := t.TempDir()
	wrapperPath := filepath.Join(wrapperRoot, "git")
	const wrapper = `#!/bin/sh
set -eu
armed="${0%/*}/armed"
is_update_ref=false
for argument in "$@"; do
	if [ "$argument" = "update-ref" ]; then
		is_update_ref=true
	fi
done
if [ -f "$armed" ] && [ "$is_update_ref" = true ]; then
	{
		IFS= read -r repository
		IFS= read -r ref
		IFS= read -r before
		IFS= read -r after
	} < "$armed"
	/usr/bin/git -C "$repository" update-ref "$ref" "$after" "$before"
	/usr/bin/rm -f "$armed"
fi
exec /usr/bin/git "$@"
`
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	repository, err := gitx.Open(repoPath, wrapperPath)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := NewActions(UseGitRepository(repository), inertActionResolver, gitx.Identity{Name: "Test Engine", Email: "engine@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	return repoPath, actions, &assemblyCASInterleaver{
		armedPath: filepath.Join(wrapperRoot, "armed"),
	}
}

func (i *assemblyCASInterleaver) arm(
	t *testing.T,
	repository, ref, before, after string,
) {
	t.Helper()
	body := fmt.Sprintf("%s\n%s\n%s\n%s\n", repository, ref, before, after)
	if err := os.WriteFile(i.armedPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (i *assemblyCASInterleaver) requireFired(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(i.armedPath); !os.IsNotExist(err) {
		t.Fatalf("interleaver did not fire: %v", err)
	}
}

func assemblyRogueCommit(
	t *testing.T,
	repository, parent string,
	timestamp int64,
) string {
	t.Helper()
	date := fmt.Sprintf("%d +0000", timestamp)
	return actionGit(
		t,
		repository,
		[]byte("unrecorded assembly authority drift\n"),
		[]string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date},
		"commit-tree", parent+"^{tree}", "-p", parent,
	)
}

func passTwoAssemblyTracks(
	t *testing.T,
	actions *Actions,
	repository, release string,
) {
	t.Helper()
	if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: actionPlanBytes(release),
		Summary:   "Approve two-track assembly.",
		Detail:    []byte("approval"),
	}); err != nil {
		t.Fatal(err)
	}
	advanceActionSlice(
		t, actions, repository, release, "T1", "S1",
		"one.txt", 1000002300, "pass",
	)
	advanceActionSlice(
		t, actions, repository, release, "T2", "S2",
		"two.txt", 1000002400, "pass",
	)
}

func requireAssemblyCASAbort(
	t *testing.T,
	err error,
	repository, release, track, trackAfter, releaseBefore, targetBefore string,
) {
	t.Helper()
	if ErrorCode(err) != "REF_TRANSACTION_RECOVERY_REQUIRED" {
		t.Fatalf("post-classification track drift error = %v", err)
	}
	if got := actionGit(t, repository, nil, nil, "rev-parse", releaseRef(release)); got != releaseBefore {
		t.Fatalf("release moved after rejected assembly CAS: %s != %s", got, releaseBefore)
	}
	if got := actionGit(t, repository, nil, nil, "rev-parse", "refs/heads/main"); got != targetBefore {
		t.Fatalf("target moved after rejected assembly CAS: %s != %s", got, targetBefore)
	}
	if got := actionGit(t, repository, nil, nil, "rev-parse", track); got != trackAfter {
		t.Fatalf("interleaved track move was lost: %s != %s", got, trackAfter)
	}
}

func TestAssemblyTransactionsFencePostClassificationTrackDrift(t *testing.T) {
	t.Run("prepare", func(t *testing.T) {
		repository, actions, interleaver := newAssemblyCASInterleavingHarness(t)
		release := "prepare-assembly-cas"
		passTwoAssemblyTracks(t, actions, repository, release)

		// Move the non-first classified track so this proves the full vector is fenced.
		track := trackRef(release, "T2")
		trackBefore := actionGit(t, repository, nil, nil, "rev-parse", track)
		trackAfter := assemblyRogueCommit(t, repository, trackBefore, 1000002500)
		releaseBefore := actionGit(t, repository, nil, nil, "rev-parse", releaseRef(release))
		targetBefore := actionGit(t, repository, nil, nil, "rev-parse", "refs/heads/main")
		interleaver.arm(t, repository, track, trackBefore, trackAfter)

		_, err := actions.PrepareAssembly(PrepareAssemblyInput{
			Release: release,
			Summary: "Fence exact assembly inputs.",
			Detail:  []byte("post-classification CAS"),
		})
		interleaver.requireFired(t)
		requireAssemblyCASAbort(
			t, err, repository, release, track, trackAfter, releaseBefore, targetBefore,
		)
	})

	t.Run("merge", func(t *testing.T) {
		repository, actions, interleaver := newAssemblyCASInterleavingHarness(t)
		release := "merge-assembly-cas"
		passTwoAssemblyTracks(t, actions, repository, release)
		prepared, err := actions.PrepareAssembly(PrepareAssemblyInput{
			Release: release,
			Summary: "Prepare exact assembly.",
			Detail:  []byte("assembly"),
		})
		if err != nil {
			t.Fatal(err)
		}
		appendActionReceipt(t, actions, AppendReceiptInput{
			Release:      release,
			Role:         "verifier",
			Result:       "pass",
			Summary:      "Pass exact assembly.",
			Detail:       []byte("fresh verification"),
			Candidate:    prepared.Candidate,
			CheckResults: []byte("assembly checks\n"),
		})

		track := trackRef(release, "T2")
		trackBefore := actionGit(t, repository, nil, nil, "rev-parse", track)
		trackAfter := assemblyRogueCommit(t, repository, trackBefore, 1000002600)
		releaseBefore := actionGit(t, repository, nil, nil, "rev-parse", releaseRef(release))
		targetBefore := actionGit(t, repository, nil, nil, "rev-parse", "refs/heads/main")
		interleaver.arm(t, repository, track, trackBefore, trackAfter)

		_, err = actions.MergePassedCandidate(MergePassedCandidateInput{
			Release: release,
			Summary: "Fence exact passed assembly.",
			Detail:  []byte("post-classification CAS"),
		})
		interleaver.requireFired(t)
		requireAssemblyCASAbort(
			t, err, repository, release, track, trackAfter, releaseBefore, targetBefore,
		)
	})
}
