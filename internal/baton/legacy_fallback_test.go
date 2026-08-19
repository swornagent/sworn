package baton

import (
	"bytes"
	"testing"
)

// relocatePlanToLegacy rewrites one release's plan into the historical
// .baton/releases root only, producing the pre-move record shape: the plan is
// absent from the configured root and present under the legacy root at the
// release ref head.
func relocatePlanToLegacy(
	t *testing.T,
	repoPath string,
	release string,
	fromHead string,
	planBytes []byte,
) string {
	t.Helper()
	indexEnv := []string{"GIT_INDEX_FILE=" + t.TempDir() + "/index"}
	actionGit(t, repoPath, nil, indexEnv, "read-tree", fromHead+"^{tree}")
	actionGit(t, repoPath, nil, indexEnv, "update-index", "--force-remove", "--",
		planPath(RecordRoot, release))
	blob := actionGit(t, repoPath, planBytes, nil, "hash-object", "-w", "--stdin")
	actionGit(t, repoPath, nil, indexEnv, "update-index", "--add", "--cacheinfo",
		"100644,"+blob+","+planPath(LegacyRecordRoot, release))
	tree := actionGit(t, repoPath, nil, indexEnv, "write-tree")
	commit := actionGit(t, repoPath, []byte("legacy relocation\n"), nil, "commit-tree", tree, "-p", fromHead)
	actionGit(t, repoPath, nil, nil, "update-ref", releaseRef(release), commit, fromHead)
	return commit
}

// TestReadStateFallsBackToLegacyRecordRoot proves A4: a release recorded only
// under the historical .baton/releases root (the pre-move shape) stays fully
// readable — state reading resolves its exact plan and receipt history with
// the configured root empty.
func TestReadStateFallsBackToLegacyRecordRoot(t *testing.T) {
	t.Parallel()
	repoPath, repository, actions := createActionHarness(t)
	release := "legacy-only-release"
	planBytes := actionPlanBytes(release)
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Approve legacy-only release.", Detail: []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	// Relocate the plan into the historical root only (the pre-move shape).
	legacyHead := relocatePlanToLegacy(t, repoPath, release, result.Head, planBytes)

	refs, err := ListReleaseRefs(UseGitRepository(repository))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Release != release || refs[0].Head != legacyHead {
		t.Fatalf("release refs = %#v", refs)
	}
	state := readActionState(t, repository, release)
	if state.Plan.OID == "" || state.Plan.Digest != DigestBytes(planBytes) {
		t.Fatalf("legacy state plan = %#v", state.Plan)
	}
	if !state.Plan.LegacyFallback {
		t.Fatalf("expected legacy state.Plan.LegacyFallback = true, got false")
	}
	if state.Refs.Release.Head != legacyHead {
		t.Fatalf("release head = %s, want %s", state.Refs.Release.Head, legacyHead)
	}
	if len(state.Plan.History) != 1 || state.Plan.History[0].Plan.Digest() != DigestBytes(planBytes) {
		t.Fatalf("legacy plan history = %#v", state.Plan.History)
	}
}

// TestReadStateConfiguredRootWinsOverLegacy proves A4's one-authority rule: a
// release present under both roots resolves to the configured root, never two
// plans.
func TestReadStateConfiguredRootWinsOverLegacy(t *testing.T) {
	t.Parallel()
	repoPath, repository, actions := createActionHarness(t)
	release := "both-roots-release"
	planBytes := actionPlanBytes(release)
	result, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Approve both-roots release.", Detail: []byte("approval"),
	})
	if err != nil || !result.Changed {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	// Add a different, foreign plan under the legacy root on a child commit;
	// the configured root copy must win.
	indexEnv := []string{"GIT_INDEX_FILE=" + t.TempDir() + "/index"}
	actionGit(t, repoPath, nil, indexEnv, "read-tree", result.Head+"^{tree}")
	blob := actionGit(t, repoPath, []byte("foreign legacy plan\n"), nil, "hash-object", "-w", "--stdin")
	actionGit(t, repoPath, nil, indexEnv, "update-index", "--add", "--cacheinfo",
		"100644,"+blob+","+planPath(LegacyRecordRoot, release))
	tree := actionGit(t, repoPath, nil, indexEnv, "write-tree")
	commit := actionGit(t, repoPath, []byte("foreign legacy\n"), nil, "commit-tree", tree, "-p", result.Head)
	actionGit(t, repoPath, nil, nil, "update-ref", releaseRef(release), commit, result.Head)

	state := readActionState(t, repository, release)
	if state.Plan.Digest != DigestBytes(planBytes) {
		t.Fatalf("state plan digest = %s, want the configured-root plan %s",
			state.Plan.Digest, DigestBytes(planBytes))
	}
	if state.Plan.LegacyFallback {
		t.Fatalf("expected configured state.Plan.LegacyFallback = false, got true")
	}
	file, err := actions.repository.file(commit, planPath(LegacyRecordRoot, release))
	if err != nil || !file.Present || !bytes.Equal(file.Bytes, []byte("foreign legacy plan\n")) {
		t.Fatalf("legacy foreign plan = %#v, err = %v", file, err)
	}
}
