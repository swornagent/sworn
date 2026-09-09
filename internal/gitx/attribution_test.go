package gitx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttributeWorkspaceRoundTrip(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-attr", Track: "T1"}
	createTrack(t, repository, key, base)

	workspaces, err := NewRunWorkspaces(repository, "run-attr", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()

	lease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	attribution := WorkspaceAttribution{
		CommonDir:      repository.CommonDir(),
		RunID:          "run-attr",
		Release:        key.Release,
		Track:          key.Track,
		Slice:          "S2",
		PlanOID:        base.String(),
		PlanDigest:     "sha256:plan",
		ContractDigest: "sha256:contract",
		PreparedBase:   lease.Head().String(),
		DispatchWork:   "S2:implementer_implementation",
		Epoch:          1,
		Try:            1,
	}
	if err := workspaces.AttributeWorkspace(lease, attribution); err != nil {
		t.Fatalf("attribute workspace: %v", err)
	}

	listed, err := workspaces.ListAttributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Token != lease.Token() || listed[0].Slice != "S2" {
		t.Fatalf("unexpected attributions: %#v", listed)
	}

	if err := workspaces.ClearAttribution(lease.Token()); err != nil {
		t.Fatal(err)
	}
	listed, err = workspaces.ListAttributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected no attributions after clear, got %#v", listed)
	}

	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAttributeWorkspaceRejectsMismatchedKey(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-attr-mismatch", Track: "T1"}
	createTrack(t, repository, key, base)

	workspaces, err := NewRunWorkspaces(repository, "run-attr-mismatch", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()

	lease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()

	attribution := WorkspaceAttribution{
		CommonDir: repository.CommonDir(),
		RunID:     "run-attr-mismatch",
		Release:   "other-release",
		Track:     key.Track,
	}
	if err := workspaces.AttributeWorkspace(lease, attribution); err == nil {
		t.Fatal("expected mismatched release to be refused")
	}
}

// closeOwnerLockOnly releases the workspace root's owner flock and every
// open track-writer flock this process held, without touching any lease,
// tree, or marker on disk - modeling a killed process whose OS-level flocks
// are released when its file descriptors close on exit, but which never ran
// any of its own cleanup. It deliberately does not rely on garbage
// collection finalizers to close those descriptors: a live *os.File held by
// an in-scope lease keeps its flock until something closes it explicitly.
func closeOwnerLockOnly(t *testing.T, workspaces *Workspaces) {
	t.Helper()
	for _, lease := range workspaces.leases {
		if lease.writerLock != nil {
			_ = lease.writerLock.Close()
			lease.writerLock = nil
		}
	}
	if err := workspaces.releaseLock(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverAbandonedDefersAttributedWorkspace(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-defer", Track: "T1"}
	createTrack(t, repository, key, base)

	crashed, err := NewRunWorkspaces(repository, "run-defer", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := crashed.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	token := lease.Token()
	treePath := lease.Path()
	attribution := WorkspaceAttribution{
		CommonDir:      repository.CommonDir(),
		RunID:          "run-defer",
		Release:        key.Release,
		Track:          key.Track,
		Slice:          "S2",
		PlanOID:        base.String(),
		PlanDigest:     "sha256:plan",
		ContractDigest: "sha256:contract",
		PreparedBase:   lease.Head().String(),
		DispatchWork:   "S2:implementer_implementation",
		Epoch:          1,
		Try:            1,
	}
	if err := crashed.AttributeWorkspace(lease, attribution); err != nil {
		t.Fatal(err)
	}
	scoped := filepath.Join(treePath, "interrupted.txt")
	if err := os.WriteFile(scoped, []byte("interrupted work"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate a hard process exit: drop the owner flock only, leaving every
	// lease, tree, and attribution marker exactly as they were.
	closeOwnerLockOnly(t, crashed)

	replacement, err := NewRunWorkspaces(repository, "run-defer", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()

	if _, err := os.Lstat(treePath); err != nil {
		t.Fatalf("expected attributed tree to survive replacement owner's cleanup pass: %v", err)
	}
	if _, err := os.Lstat(scoped); err != nil {
		t.Fatalf("expected interrupted file to survive: %v", err)
	}

	listed, err := replacement.ListAttributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Token != token {
		t.Fatalf("expected the attribution to survive for adoption, got %#v", listed)
	}

	head, err := ParseOID(repository.ObjectFormat(), listed[0].PreparedBase)
	if err != nil {
		t.Fatal(err)
	}
	adopted, err := replacement.AdoptAbandonedWorkspace(listed[0].Token, key, head)
	if err != nil {
		t.Fatalf("adopt abandoned workspace: %v", err)
	}
	if adopted.Path() != treePath {
		t.Fatalf("adopted workspace path = %q, want %q", adopted.Path(), treePath)
	}
	result, err := replacement.CaptureCheckpoint(
		adopted, CheckpointAttempt{WorkID: "S2:implementer_implementation", Epoch: 1, Try: 1},
		"S2", CheckpointScope{Include: []string{"interrupted.txt"}}, 0, nil,
	)
	if err != nil {
		t.Fatalf("capture salvage checkpoint: %v", err)
	}
	if result.Empty {
		t.Fatal("expected the interrupted bytes to be captured")
	}
	if err := replacement.ClearAttribution(token); err != nil {
		t.Fatal(err)
	}
	if err := adopted.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(treePath); !os.IsNotExist(err) {
		t.Fatalf("expected the salvaged tree to be cleaned up after capture, got %v", err)
	}
}

func TestRecoverAbandonedStillDeletesUnattributedWorkspace(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-no-attr", Track: "T1"}
	createTrack(t, repository, key, base)

	crashed, err := NewRunWorkspaces(repository, "run-no-attr", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := crashed.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	treePath := lease.Path()
	closeOwnerLockOnly(t, crashed)

	replacement, err := NewRunWorkspaces(repository, "run-no-attr", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()

	if _, err := os.Lstat(treePath); !os.IsNotExist(err) {
		t.Fatalf("expected an unattributed abandoned tree to be cleaned up as before, got %v", err)
	}
}

func TestAdoptAbandonedWorkspaceRefusesHeadMismatch(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-head-mismatch", Track: "T1"}
	createTrack(t, repository, key, base)

	crashed, err := NewRunWorkspaces(repository, "run-head-mismatch", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := crashed.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	attribution := WorkspaceAttribution{
		CommonDir:    repository.CommonDir(),
		RunID:        "run-head-mismatch",
		Release:      key.Release,
		Track:        key.Track,
		Slice:        "S2",
		PreparedBase: lease.Head().String(),
	}
	if err := crashed.AttributeWorkspace(lease, attribution); err != nil {
		t.Fatal(err)
	}
	token := lease.Token()
	closeOwnerLockOnly(t, crashed)

	replacement, err := NewRunWorkspaces(repository, "run-head-mismatch", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()

	// Claim a different, but otherwise valid and resolvable, head than the
	// worktree's actual HEAD: this must be refused rather than adopted,
	// proving the attribution's provenance against the worktree's own Git
	// state rather than trusting the record.
	other := commitEmptyTree(t, repository, base)
	_, adoptErr := replacement.AdoptAbandonedWorkspace(token, key, other)
	if adoptErr == nil {
		t.Fatal("expected head mismatch to be refused")
	}
	requireGitxErrorCode(t, adoptErr, "CHECKPOINT_CORRUPT")
}

// commitEmptyTree writes a new commit on top of parent with the same tree,
// producing a distinct, valid, resolvable commit OID to use as a
// deliberately wrong claimed head in tests.
func commitEmptyTree(t *testing.T, repository *Repository, parent OID) OID {
	t.Helper()
	tree, err := repository.TreeOID(parent)
	if err != nil {
		t.Fatal(err)
	}
	timestamp, err := repository.CommitTimestamp(parent)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := repository.run(
		[]byte("gitx test: unrelated commit\n"),
		commitEnvironment(testIdentity, timestamp+1),
		"commit-tree", tree.String(), "-p", parent.String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	oid, err := repository.parseOID(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return oid
}

func TestListAttributionsFencesCorruptRecord(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-corrupt", Track: "T1"}
	createTrack(t, repository, key, base)

	crashed, err := NewRunWorkspaces(repository, "run-corrupt", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := crashed.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	token := lease.Token()
	treePath := lease.Path()
	attribution := WorkspaceAttribution{
		CommonDir:    repository.CommonDir(),
		RunID:        "run-corrupt",
		Release:      key.Release,
		Track:        key.Track,
		Slice:        "S2",
		PreparedBase: lease.Head().String(),
	}
	if err := crashed.AttributeWorkspace(lease, attribution); err != nil {
		t.Fatal(err)
	}
	closeOwnerLockOnly(t, crashed)

	// Corrupt the attribution record in place, as if the write had been
	// interrupted or the file damaged.
	attributionPath := filepath.Join(crashed.attributionsRoot, token)
	if err := os.WriteFile(attributionPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	replacement, err := NewRunWorkspaces(repository, "run-corrupt", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()

	listed, err := replacement.ListAttributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected the corrupt attribution to be excluded, got %#v", listed)
	}
	fenced, err := replacement.FencedWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fenced {
		if f.Token == token && f.Reason == "CHECKPOINT_CORRUPT" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the abandoned tree to be quarantined, got %#v", fenced)
	}
	if _, err := os.Lstat(treePath); err != nil {
		t.Fatalf("expected the quarantined tree to still be on disk: %v", err)
	}
}

func TestCloseDefersRootTeardownWhileAttributed(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-close-defer", Track: "T1"}
	createTrack(t, repository, key, base)

	crashed, err := NewRunWorkspaces(repository, "run-close-defer", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := crashed.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	attribution := WorkspaceAttribution{
		CommonDir:    repository.CommonDir(),
		RunID:        "run-close-defer",
		Release:      key.Release,
		Track:        key.Track,
		Slice:        "S2",
		PreparedBase: lease.Head().String(),
	}
	if err := crashed.AttributeWorkspace(lease, attribution); err != nil {
		t.Fatal(err)
	}
	closeOwnerLockOnly(t, crashed)

	replacement, err := NewRunWorkspaces(repository, "run-close-defer", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	// An ordinary approval-path Close (no reconciliation ever ran) must not
	// fail just because a retained attributed workspace still occupies the
	// root: it must defer teardown exactly as it already does for a fenced
	// workspace.
	if err := replacement.Close(); err != nil {
		t.Fatalf("expected Close to defer teardown while attributed, got %v", err)
	}
}
