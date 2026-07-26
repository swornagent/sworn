package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func createTrack(t *testing.T, repository *Repository, key TrackKey, head OID) {
	t.Helper()
	ref := trackHeadRef(key)
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind: CreateRef, Ref: ref, NewHead: &head,
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestTypedWorkspacesAreFreshReadOnlyAndSealBehindClaim(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "release-1", Track: "T1"}
	createTrack(t, repository, key, base)
	workspaces, err := NewWorkspaces(repository)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()

	design, err := workspaces.OpenTrack(key, DesignView)
	if err != nil {
		t.Fatal(err)
	}
	designPath := design.Path()
	info, err := os.Stat(filepath.Join(designPath, "product.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("read-only file mode = %o", info.Mode().Perm())
	}
	if err := os.WriteFile(
		filepath.Join(designPath, "product.txt"),
		[]byte("escape\n"),
		0o600,
	); err == nil {
		t.Fatal("physically read-only workspace accepted a write")
	}
	if err := design.Close(); err != nil {
		t.Fatal(err)
	}

	implementation, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	if implementation.Path() == designPath {
		t.Fatal("workspace path was reused across responsibilities")
	}
	if err := os.WriteFile(
		filepath.Join(implementation.Path(), "product.txt"),
		[]byte("candidate\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	claimed := false
	sealed, err := workspaces.SealTrackWithClaim(
		implementation,
		func(prepared SealedCandidate) error {
			claimed = true
			captured := captureRefs(t, repository, trackHeadRef(key))
			if captured[0].Head != base {
				t.Fatal("track moved before durable claim callback")
			}
			if prepared.Before != base ||
				!reflect.DeepEqual(prepared.ChangedPaths, []string{"product.txt"}) {
				t.Fatalf("prepared candidate = %#v", prepared)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("seal omitted pre-update claim callback")
	}
	if err := implementation.Close(); err != nil {
		t.Fatal(err)
	}
	captured := captureRefs(t, repository, trackHeadRef(key))
	if captured[0].Head != sealed.Candidate {
		t.Fatalf("track head = %s, want %s", captured[0].Head, sealed.Candidate)
	}
	parents, err := repository.Parents(sealed.Candidate)
	if err != nil || !reflect.DeepEqual(parents, []OID{base}) {
		t.Fatalf("candidate parents = %v, err = %v", parents, err)
	}

	workVerifier, err := workspaces.OpenCandidate(
		key,
		WorkVerifierView,
		sealed.Candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	assemblyVerifier, err := workspaces.OpenCandidate(
		key,
		AssemblyVerifierView,
		sealed.Candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workVerifier.Path() == assemblyVerifier.Path() ||
		workVerifier.Path() == designPath ||
		assemblyVerifier.Path() == designPath {
		t.Fatal("fresh verifier workspaces were not distinct")
	}
	if err := workVerifier.Close(); err != nil {
		t.Fatal(err)
	}
	if err := assemblyVerifier.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSealReconciliationClassifiesAllOldAllNewAndThirdState(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "release-2", Track: "T1"}
	createTrack(t, repository, key, base)
	workspaces, err := NewWorkspaces(repository)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()
	lease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(lease.Path(), "product.txt"),
		[]byte("candidate\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	sealed, err := workspaces.SealTrack(lease)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	disposition, err := workspaces.ReconcileSeal(key, base, sealed.Candidate)
	if err != nil || disposition != SealAllNew {
		t.Fatalf("all-new = %q, err = %v", disposition, err)
	}

	oldKey := TrackKey{Release: "release-2", Track: "T2"}
	createTrack(t, repository, oldKey, base)
	disposition, err = workspaces.ReconcileSeal(oldKey, base, sealed.Candidate)
	if err != nil || disposition != SealAllOld {
		t.Fatalf("all-old = %q, err = %v", disposition, err)
	}
	third := nextRecord(t, repository, base, "third")
	captured := captureRefs(t, repository, trackHeadRef(oldKey))
	if err := repository.ApplyRefTransaction(captured, []RefOperation{{
		Kind: UpdateRef, Ref: trackHeadRef(oldKey),
		NewHead: &third, Expected: &base,
	}}); err != nil {
		t.Fatal(err)
	}
	disposition, err = workspaces.ReconcileSeal(oldKey, base, sealed.Candidate)
	if err != nil || disposition != SealAmbiguous {
		t.Fatalf("third-state = %q, err = %v", disposition, err)
	}
}

func TestFailedClaimCallbackCannotMoveTrack(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "release-3", Track: "T1"}
	createTrack(t, repository, key, base)
	workspaces, err := NewWorkspaces(repository)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()
	lease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(lease.Path(), "product.txt"),
		[]byte("candidate\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("journal claim failed")
	if _, err := workspaces.SealTrackWithClaim(
		lease,
		func(SealedCandidate) error { return sentinel },
	); !errors.Is(err, sentinel) {
		t.Fatalf("seal error = %v", err)
	}
	captured := captureRefs(t, repository, trackHeadRef(key))
	if captured[0].Head != base {
		t.Fatal("failed claim callback moved the track")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSealRejectsBatonAuthorityChangesBeforeRefMove(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "release-4", Track: "T1"}
	createTrack(t, repository, key, base)
	workspaces, err := NewWorkspaces(repository)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()
	lease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	authority := filepath.Join(
		lease.Path(),
		".baton",
		"releases",
		"escape",
		"plan.md",
	)
	if err := os.MkdirAll(filepath.Dir(authority), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authority, []byte("escape\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := workspaces.SealTrack(lease); err == nil {
		t.Fatal("seal admitted Baton authority bytes")
	} else {
		requireGitxErrorCode(t, err, "AUTHORITY_PATH_CHANGED")
	}
	captured := captureRefs(t, repository, trackHeadRef(key))
	if captured[0].Head != base {
		t.Fatal("authority escape moved the track")
	}
}
