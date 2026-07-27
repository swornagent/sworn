package gitx

import (
	"errors"
	"os"
	"os/exec"
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

func containsWorkspacePath(paths []string, target string) bool {
	for _, path := range paths {
		if path == target {
			return true
		}
	}
	return false
}

func TestReleaseAssemblyLeaseBindsExactReleaseHead(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	release := "release-assembly"
	releaseRef := "refs/heads/release-wt/" + release
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind: CreateRef, Ref: releaseRef, NewHead: &base,
	}}); err != nil {
		t.Fatal(err)
	}
	workspaces, err := NewWorkspaces(repository)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()

	lease, err := workspaces.OpenReleaseAssembly(release, base)
	if err != nil {
		t.Fatal(err)
	}
	if lease.workspace == nil ||
		lease.workspace.view != ReleaseAssemblyView ||
		lease.workspace.access != WorkspaceReadOnly ||
		lease.workspace.head != base {
		t.Fatalf("release assembly lease = %#v", lease.workspace)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
	registered, err := registeredWorktreePaths(repository)
	if err != nil {
		t.Fatal(err)
	}
	baselineRegistered := len(registered)

	moved := nextRecord(t, repository, base, "release-moved")
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind: UpdateRef, Ref: releaseRef,
		NewHead: &moved, Expected: &base,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := workspaces.OpenReleaseAssembly(release, base); err == nil {
		t.Fatal("stale release authority admitted")
	} else {
		requireGitxErrorCode(t, err, "AUTHORITY_MOVED")
	}
	if _, err := workspaces.OpenReleaseAssembly("release-absent", base); err == nil {
		t.Fatal("absent release authority admitted")
	} else {
		requireGitxErrorCode(t, err, "AUTHORITY_MOVED")
	}
	symbolicRelease := "release-symbolic"
	runTestGit(
		t,
		repository.Root(),
		nil,
		"symbolic-ref",
		"refs/heads/release-wt/"+symbolicRelease,
		"refs/heads/main",
	)
	if _, err := workspaces.OpenReleaseAssembly(symbolicRelease, base); err == nil {
		t.Fatal("symbolic release authority admitted")
	} else {
		requireGitxErrorCode(t, err, "AUTHORITY_MOVED")
	}
	registered, err = registeredWorktreePaths(repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != baselineRegistered {
		t.Fatalf(
			"rejected release authority leaked worktrees: before=%d after=%d",
			baselineRegistered,
			len(registered),
		)
	}
}

func TestRunWorkspacesRejectRepositoryLocalTempBase(t *testing.T) {
	repository, _ := newRepository(t, SHA1)
	localTemp := filepath.Join(repository.Root(), "runtime-temp")
	if err := os.Mkdir(localTemp, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", localTemp)
	base, err := workspaceBase()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunWorkspaces(repository, "overlap-run"); err == nil {
		t.Fatal("repository-local workspace base was admitted")
	} else {
		requireGitxErrorCode(t, err, "INVALID_REPOSITORY")
	}
	if _, err := os.Lstat(base); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected overlapping workspace base was created: %v", err)
	}
}

func TestRunWorkspacesHardExitHelper(t *testing.T) {
	if os.Getenv("SWORN_WORKSPACE_CRASH_HELPER") != "1" {
		return
	}
	repository, err := Open(
		os.Getenv("SWORN_WORKSPACE_CRASH_REPOSITORY"),
		os.Getenv("SWORN_WORKSPACE_CRASH_GIT"),
	)
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := NewRunWorkspaces(repository, "hard-exit-run")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := workspaces.OpenTrack(
		TrackKey{Release: "release-hard-exit", Track: "T1"},
		ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(lease.Path(), "uncommitted.txt"),
		[]byte("abandoned by hard exit\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		os.Getenv("SWORN_WORKSPACE_CRASH_PATH"),
		[]byte(lease.Path()),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestRunWorkspacesRecoverAfterHardProcessExit(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "release-hard-exit", Track: "T1"}
	createTrack(t, repository, key, base)
	pathRecord := filepath.Join(t.TempDir(), "abandoned-path")
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestRunWorkspacesHardExitHelper$",
	)
	command.Env = append(
		os.Environ(),
		"SWORN_WORKSPACE_CRASH_HELPER=1",
		"SWORN_WORKSPACE_CRASH_REPOSITORY="+repository.Root(),
		"SWORN_WORKSPACE_CRASH_GIT="+repository.GitExecutable(),
		"SWORN_WORKSPACE_CRASH_PATH="+pathRecord,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("hard-exit helper: %v\n%s", err, output)
	}
	rawPath, err := os.ReadFile(pathRecord)
	if err != nil {
		t.Fatal(err)
	}
	abandonedPath := string(rawPath)
	if _, err := os.Lstat(abandonedPath); err != nil {
		t.Fatalf("hard-exit workspace was not left behind: %v", err)
	}
	registered, err := registeredWorktreePaths(repository)
	if err != nil {
		t.Fatal(err)
	}
	if !containsWorkspacePath(registered, abandonedPath) {
		t.Fatal("hard-exit workspace registration was not left behind")
	}

	replacement, err := NewRunWorkspaces(repository, "hard-exit-run")
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if _, err := os.Lstat(abandonedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement did not remove hard-exit workspace: %v", err)
	}
	registered, err = registeredWorktreePaths(repository)
	if err != nil {
		t.Fatal(err)
	}
	if containsWorkspacePath(registered, abandonedPath) {
		t.Fatal("replacement did not remove hard-exit Git registration")
	}
}

func TestRunWorkspacesRecoverOnlyAbandonedExactRun(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "release-recovery", Track: "T1"}
	createTrack(t, repository, key, base)

	crashed, err := NewRunWorkspaces(repository, "run-recovery")
	if err != nil {
		t.Fatal(err)
	}
	crashedOpen := true
	t.Cleanup(func() {
		if crashedOpen {
			_ = crashed.Close()
		}
	})
	abandoned, err := crashed.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(abandoned.Path(), "uncommitted.txt"),
		[]byte("must be removed only by the replacement owner\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	abandonedRoot, abandonedPath := crashed.root, abandoned.Path()
	if _, err := NewRunWorkspaces(repository, "run-recovery"); err == nil {
		t.Fatal("second active workspace owner was admitted")
	} else {
		requireGitxErrorCode(t, err, "WORKSPACE_OWNER_ACTIVE")
	}
	registeredBeforeCrash, err := registeredWorktreePaths(repository)
	if err != nil {
		t.Fatal(err)
	}
	if !containsWorkspacePath(registeredBeforeCrash, abandonedPath) {
		t.Fatal("rejected active owner removed the live Git registration")
	}

	otherRun, err := NewRunWorkspaces(repository, "run-other")
	if err != nil {
		t.Fatal(err)
	}
	defer otherRun.Close()
	otherLease, err := otherRun.OpenSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	otherPath := otherLease.Path()

	otherRepository, otherBase := newRepository(t, SHA1)
	otherRepositoryRun, err := NewRunWorkspaces(otherRepository, "run-recovery")
	if err != nil {
		t.Fatal(err)
	}
	defer otherRepositoryRun.Close()
	otherRepositoryLease, err := otherRepositoryRun.OpenSnapshot(otherBase)
	if err != nil {
		t.Fatal(err)
	}
	otherRepositoryPath := otherRepositoryLease.Path()

	// A hard process exit releases the kernel lock without running Close. Leave
	// the worktree and its marker intact to exercise replacement recovery.
	if err := crashed.releaseLock(); err != nil {
		t.Fatal(err)
	}
	replacement, err := NewRunWorkspaces(repository, "run-recovery")
	if err != nil {
		t.Fatal(err)
	}
	crashedOpen = false // The replacement has mechanically recovered its root.
	if replacement.root != abandonedRoot {
		t.Fatalf("replacement root = %q, want stable %q", replacement.root, abandonedRoot)
	}
	if replacement.root == otherRun.root ||
		replacement.root == otherRepositoryRun.root {
		t.Fatal("repository plus run identity did not isolate workspace roots")
	}
	if _, err := os.Lstat(abandonedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned workspace still exists: %v", err)
	}
	registered, err := registeredWorktreePaths(repository)
	if err != nil {
		t.Fatal(err)
	}
	if containsWorkspacePath(registered, abandonedPath) {
		t.Fatalf("abandoned worktree remains registered: %s", abandonedPath)
	}
	if !containsWorkspacePath(registered, otherPath) {
		t.Fatalf("other run worktree was removed: %s", otherPath)
	}
	otherRegistered, err := registeredWorktreePaths(otherRepository)
	if err != nil {
		t.Fatal(err)
	}
	if !containsWorkspacePath(otherRegistered, otherRepositoryPath) {
		t.Fatalf("other repository worktree was removed: %s", otherRepositoryPath)
	}

	fresh, err := replacement.OpenSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Path() == abandonedPath ||
		filepath.Dir(fresh.Path()) != replacement.treesRoot {
		t.Fatalf("replacement workspace path = %q", fresh.Path())
	}
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}
	if _, err := os.Lstat(abandonedRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed workspace root still exists: %v", err)
	}
	if _, err := os.Lstat(
		filepath.Join(filepath.Dir(abandonedRoot), replacement.identity+".lock"),
	); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed workspace owner lock still exists: %v", err)
	}
	if _, err := os.Lstat(otherPath); err != nil {
		t.Fatalf("other run workspace changed after close: %v", err)
	}
	if _, err := os.Lstat(otherRepositoryPath); err != nil {
		t.Fatalf("other repository workspace changed after close: %v", err)
	}
}

func TestRunWorkspacesRejectUnownedCrashStateWithoutDeleting(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		tamper func(*Workspaces, *WorkspaceLease) (string, []byte)
	}{
		{
			name: "root_marker",
			tamper: func(workspaces *Workspaces, _ *WorkspaceLease) (string, []byte) {
				path := filepath.Join(workspaces.root, "owner")
				return path, rootMarker(workspaces.identity)
			},
		},
		{
			name: "lease_marker",
			tamper: func(workspaces *Workspaces, lease *WorkspaceLease) (string, []byte) {
				path := filepath.Join(workspaces.leasesRoot, lease.token)
				return path, leaseMarker(workspaces.identity, lease.token)
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository, base := newRepository(t, SHA1)
			key := TrackKey{Release: "release-" + test.name, Track: "T1"}
			createTrack(t, repository, key, base)
			workspaces, err := NewRunWorkspaces(repository, "run-"+test.name)
			if err != nil {
				t.Fatal(err)
			}
			defer workspaces.Close()
			lease, err := workspaces.OpenTrack(key, ImplementationView)
			if err != nil {
				t.Fatal(err)
			}
			path, expected := test.tamper(workspaces, lease)
			if err := os.WriteFile(path, []byte("foreign ownership\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := workspaces.releaseLock(); err != nil {
				t.Fatal(err)
			}
			if _, err := NewRunWorkspaces(repository, "run-"+test.name); err == nil {
				t.Fatal("replacement accepted unowned workspace state")
			} else {
				requireGitxErrorCode(t, err, "WORKSPACE_OWNERSHIP_MISMATCH")
			}
			if _, err := os.Lstat(lease.Path()); err != nil {
				t.Fatalf("failed ownership check deleted workspace: %v", err)
			}
			registered, err := registeredWorktreePaths(repository)
			if err != nil {
				t.Fatal(err)
			}
			if !containsWorkspacePath(registered, lease.Path()) {
				t.Fatal("failed ownership check removed Git registration")
			}
			if err := os.WriteFile(path, expected, 0o600); err != nil {
				t.Fatal(err)
			}
			lock, err := acquireWorkspaceLock(
				filepath.Dir(workspaces.root),
				workspaces.identity,
			)
			if err != nil {
				t.Fatal(err)
			}
			workspaces.lock = lock
		})
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

func TestGuardedSealPublishesOnlyUnderExactReleaseTargetAndTrack(t *testing.T) {
	t.Parallel()

	repository, base := newRepository(t, SHA1)
	key := TrackKey{Release: "release-guarded", Track: "T1"}
	releaseRef := "refs/heads/release-wt/" + key.Release
	if err := repository.AtomicUpdateRefs([]RefOperation{{
		Kind: CreateRef, Ref: releaseRef, NewHead: &base,
	}}); err != nil {
		t.Fatal(err)
	}
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
		[]byte("guarded candidate\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	sealed, err := workspaces.SealTrackGuardedWithClaim(
		lease,
		SealAuthority{
			ReleaseHead: base,
			TargetRef:   "refs/heads/main",
			TargetHead:  base,
		},
		func(prepared SealedCandidate) error {
			if prepared.Before != base {
				t.Fatalf("prepared before = %s", prepared.Before)
			}
			for _, observed := range captureRefs(
				t, repository, releaseRef, "refs/heads/main", trackHeadRef(key),
			) {
				if observed.Head != base {
					t.Fatalf("%s moved before claim: %s", observed.Ref, observed.Head)
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if observed := captureRefs(t, repository, trackHeadRef(key)); observed[0].Head != sealed.Candidate {
		t.Fatalf("track = %s, want %s", observed[0].Head, sealed.Candidate)
	}
	for _, observed := range captureRefs(t, repository, releaseRef, "refs/heads/main") {
		if observed.Head != base {
			t.Fatalf("guard ref %s moved to %s", observed.Ref, observed.Head)
		}
	}
}

func TestGuardedSealRejectsReleaseTargetOrTrackSupersessionWithoutPublishing(t *testing.T) {
	t.Parallel()

	for _, drift := range []string{"release", "target", "track"} {
		drift := drift
		t.Run(drift, func(t *testing.T) {
			t.Parallel()
			repository, base := newRepository(t, SHA1)
			key := TrackKey{Release: "release-" + drift, Track: "T1"}
			releaseRef := "refs/heads/release-wt/" + key.Release
			if err := repository.AtomicUpdateRefs([]RefOperation{{
				Kind: CreateRef, Ref: releaseRef, NewHead: &base,
			}}); err != nil {
				t.Fatal(err)
			}
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
				[]byte("stale candidate\n"),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			next := nextRecord(t, repository, base, drift+"-supersession")
			driftRef := releaseRef
			if drift == "target" {
				driftRef = "refs/heads/main"
			} else if drift == "track" {
				driftRef = trackHeadRef(key)
			}
			_, err = workspaces.SealTrackGuardedWithClaim(
				lease,
				SealAuthority{
					ReleaseHead: base,
					TargetRef:   "refs/heads/main",
					TargetHead:  base,
				},
				func(SealedCandidate) error {
					captured := captureRefs(t, repository, driftRef)
					return repository.ApplyRefTransaction(captured, []RefOperation{{
						Kind: UpdateRef, Ref: driftRef,
						NewHead: &next, Expected: &base,
					}})
				},
			)
			wantCode := "AUTHORITY_MOVED"
			if drift == "track" {
				wantCode = "TRACK_MOVED"
			}
			requireGitxErrorCode(t, err, wantCode)
			observed := captureRefs(t, repository, trackHeadRef(key))
			want := base
			if drift == "track" {
				want = next
			}
			if observed[0].Head != want {
				t.Fatalf("stale candidate overwrote track %s, want %s",
					observed[0].Head, want)
			}
		})
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
