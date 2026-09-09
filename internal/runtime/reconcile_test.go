package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

// salvageAttribution builds a WorkspaceAttribution matching lease and owner,
// with PreparedBase defaulting to the lease's own head (a matching
// authority) unless the caller overrides a field afterward.
func salvageAttribution(
	fixture *productionImplementationRecoveryFixture,
	lease *gitx.WorkspaceLease,
	track string,
	runID string,
) gitx.WorkspaceAttribution {
	return gitx.WorkspaceAttribution{
		CommonDir:      fixture.engine.repository.CommonDir(),
		RunID:          runID,
		Release:        fixture.cycle.Release,
		Track:          track,
		Slice:          fixture.cycle.Slice,
		PlanOID:        fixture.cycle.Plan,
		PlanDigest:     "sha256:plan",
		ContractDigest: "sha256:contract",
		PreparedBase:   lease.Head().String(),
		DispatchWork:   fixture.cycle.DispatchWork,
		Epoch:          1,
		Try:            1,
		ScopeInclude:   []string{"salvage.txt"},
		Token:          lease.Token(),
	}
}

// openSecondTrack materializes a second track ref (distinct from the
// fixture's own T1) pointing at the same real track head, and opens a
// writable implementation lease against it - the standard way these tests
// get a second, genuinely registered worktree+token without racing the
// fixture's own already-open lease.
func openSecondTrack(
	t *testing.T, fixture *productionImplementationRecoveryFixture, track string,
) *gitx.WorkspaceLease {
	t.Helper()
	ref := "refs/heads/track/" + fixture.cycle.Release + "/" + track
	runRuntimeGit(t, fixture.repository, "update-ref", ref, fixture.cycle.TrackHead)
	lease, err := fixture.engine.workspaces.OpenTrack(
		gitx.TrackKey{Release: fixture.cycle.Release, Track: track},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatalf("open second track %s: %v", track, err)
	}
	return lease
}

func TestCaptureSalvageCheckpointRecordsSalvagedUnverifiedCheckpoint(t *testing.T) {
	t.Parallel()
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	defer fixture.workspace.Close()

	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "salvage.txt"),
		[]byte("interrupted production bytes"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	attribution := salvageAttribution(fixture, fixture.workspace, fixture.cycle.Track, fixture.owner.RunID)

	if err := fixture.service.captureSalvageCheckpoint(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, attribution,
	); err != nil {
		t.Fatalf("capture salvage checkpoint: %v", err)
	}

	checkpoints, err := fixture.store.ListUnverifiedCheckpoints(fixture.ctx, fixture.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("unverified checkpoints = %d, want 1: %#v", len(checkpoints), checkpoints)
	}
	cp := checkpoints[0]
	if !cp.Salvaged {
		t.Fatalf("checkpoint not marked salvaged: %#v", cp)
	}
	if cp.Slice != fixture.cycle.Slice || cp.StagedBytes == 0 || cp.FileCount != 1 {
		t.Fatalf("salvaged checkpoint = %#v", cp)
	}

	latest, err := fixture.store.LatestUnverifiedCheckpoint(fixture.ctx, fixture.owner.RunID, fixture.cycle.Slice)
	if err != nil || latest == nil || latest.CheckpointRef != cp.CheckpointRef {
		t.Fatalf("latest unverified checkpoint = %#v, %v", latest, err)
	}

	// The checkpoint object exists and is reachable, but is not itself a
	// candidate: reachable production evidence, never a passing check,
	// decision, or verdict.
	runRuntimeGit(t, fixture.engine.repository.Root(), "cat-file", "-e", cp.CommitOID)
}

func TestCaptureSalvageCheckpointNeverRecordsAnEmptyUnbindableCapture(t *testing.T) {
	t.Parallel()
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	defer fixture.workspace.Close()

	// No write happens: the workspace tree is byte-identical to the prepared
	// base, so the capture must be Empty and record nothing.
	attribution := salvageAttribution(fixture, fixture.workspace, fixture.cycle.Track, fixture.owner.RunID)

	if err := fixture.service.captureSalvageCheckpoint(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, attribution,
	); err != nil {
		t.Fatalf("capture salvage checkpoint: %v", err)
	}
	checkpoints, err := fixture.store.ListUnverifiedCheckpoints(fixture.ctx, fixture.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 0 {
		t.Fatalf(
			"an unbound, unchanged capture must never be reported as a completed checkpoint, got %#v",
			checkpoints,
		)
	}
}

func TestReconcileOneWorkspaceFencesForeignRunAttributionAndPreservesBytes(t *testing.T) {
	t.Parallel()
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	defer fixture.workspace.Close()

	foreign := openSecondTrack(t, fixture, "T-foreign")
	marker := filepath.Join(foreign.Path(), "foreign.txt")
	if err := os.WriteFile(marker, []byte("do not discard"), 0o600); err != nil {
		t.Fatal(err)
	}
	attribution := salvageAttribution(fixture, foreign, "T-foreign", "a-different-run")

	if err := fixture.service.reconcileOneWorkspace(
		fixture.ctx, fixture.engine, fixture.owner, attribution,
	); err != nil {
		t.Fatalf("reconcile one workspace: %v", err)
	}

	fenced, err := fixture.engine.workspaces.FencedWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fenced {
		if f.Token == foreign.Token() {
			if f.Reason != "WORKSPACE_OWNERSHIP_MISMATCH" {
				t.Fatalf("fence reason = %q, want WORKSPACE_OWNERSHIP_MISMATCH", f.Reason)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("foreign attribution was not fenced: %#v", fenced)
	}
	// No second writer entered the track, and the original bytes were not
	// discarded to make the refusal disappear.
	if _, err := os.Lstat(marker); err != nil {
		t.Fatalf("expected the foreign workspace bytes to survive fencing: %v", err)
	}

	listed, err := fixture.engine.workspaces.ListAttributions()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range listed {
		if a.Token == foreign.Token() {
			t.Fatalf("fenced attribution was not cleared: %#v", a)
		}
	}
}

func TestReconcileOneWorkspaceFencesUnresolvablePreparedBase(t *testing.T) {
	t.Parallel()
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	defer fixture.workspace.Close()

	corrupt := openSecondTrack(t, fixture, "T-corrupt")
	attribution := salvageAttribution(fixture, corrupt, "T-corrupt", fixture.owner.RunID)
	// Syntactically valid, but not an object this repository has ever
	// written: a corrupt recovery binding, not a live authority.
	attribution.PreparedBase = "abababababababababababababababababababab"

	if err := fixture.service.reconcileOneWorkspace(
		fixture.ctx, fixture.engine, fixture.owner, attribution,
	); err != nil {
		t.Fatalf("reconcile one workspace: %v", err)
	}
	fenced, err := fixture.engine.workspaces.FencedWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fenced {
		if f.Token == corrupt.Token() {
			if f.Reason != "CHECKPOINT_CORRUPT" {
				t.Fatalf("fence reason = %q, want CHECKPOINT_CORRUPT", f.Reason)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("corrupt attribution was not fenced: %#v", fenced)
	}
}

// TestReconcileInterruptedWorkspacesFencesEveryDurablyAttributedTrack proves
// the outer loop, not just one call: two real, durably attributed tracks (one
// foreign, one corrupt) are both discovered from disk via ListAttributions
// and both fenced in the same pass, and a second pass over the now-cleared
// attributions is a clean no-op rather than a duplicate fence or transition.
func TestReconcileInterruptedWorkspacesFencesEveryDurablyAttributedTrack(t *testing.T) {
	t.Parallel()
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	defer fixture.workspace.Close()

	foreign := openSecondTrack(t, fixture, "T-foreign2")
	foreignAttribution := salvageAttribution(fixture, foreign, "T-foreign2", "a-different-run")
	if err := fixture.engine.workspaces.AttributeWorkspace(foreign, foreignAttribution); err != nil {
		t.Fatal(err)
	}

	corrupt := openSecondTrack(t, fixture, "T-corrupt2")
	corruptAttribution := salvageAttribution(fixture, corrupt, "T-corrupt2", fixture.owner.RunID)
	corruptAttribution.PreparedBase = "abababababababababababababababababababab"
	if err := fixture.engine.workspaces.AttributeWorkspace(corrupt, corruptAttribution); err != nil {
		t.Fatal(err)
	}

	if err := fixture.service.reconcileInterruptedWorkspaces(
		fixture.ctx, fixture.engine, fixture.owner,
	); err != nil {
		t.Fatalf("reconcile interrupted workspaces: %v", err)
	}

	fenced, err := fixture.engine.workspaces.FencedWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{}
	for _, f := range fenced {
		reasons[f.Token] = f.Reason
	}
	if reasons[foreign.Token()] != "WORKSPACE_OWNERSHIP_MISMATCH" ||
		reasons[corrupt.Token()] != "CHECKPOINT_CORRUPT" {
		t.Fatalf("fenced tracks = %#v", reasons)
	}

	fencedCountBefore := len(fenced)
	if err := fixture.service.reconcileInterruptedWorkspaces(
		fixture.ctx, fixture.engine, fixture.owner,
	); err != nil {
		t.Fatalf("second reconcile pass: %v", err)
	}
	fencedAfter, err := fixture.engine.workspaces.FencedWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(fencedAfter) != fencedCountBefore {
		t.Fatalf(
			"a second reconciliation pass duplicated a transition: before=%d after=%d",
			fencedCountBefore, len(fencedAfter),
		)
	}
}

func TestRunStatusDistinguishesSalvagedAndRestoredSalvagedCheckpoints(t *testing.T) {
	t.Parallel()
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	defer fixture.workspace.Close()

	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "salvage.txt"),
		[]byte("interrupted production bytes"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	attribution := salvageAttribution(fixture, fixture.workspace, fixture.cycle.Track, fixture.owner.RunID)
	// A stale-base checkpoint: recorded against the workspace's own head, but
	// this run's live track head has since moved past it.
	attribution.PreparedBase = fixture.cycle.TrackHead

	if err := fixture.service.captureSalvageCheckpoint(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, attribution,
	); err != nil {
		t.Fatalf("capture salvage checkpoint: %v", err)
	}
	checkpoints, err := fixture.store.ListUnverifiedCheckpoints(fixture.ctx, fixture.owner.RunID)
	if err != nil || len(checkpoints) != 1 {
		t.Fatalf("checkpoints = %#v, %v", checkpoints, err)
	}
	cp := checkpoints[0]

	if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
		RunID: fixture.owner.RunID, ReplayKey: "manifest",
		Kind: "start", Payload: fixture.manifest.raw, CreatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}

	status, err := fixture.service.Status(fixture.ctx, fixture.owner.RunID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var found *CheckpointStatus
	for i := range status.Checkpoints {
		if status.Checkpoints[i].CheckpointID == cp.CheckpointRef {
			found = &status.Checkpoints[i]
		}
	}
	if found == nil || found.Status != "salvaged" {
		t.Fatalf("checkpoint status before restore = %#v", found)
	}

	if err := fixture.store.RecordCheckpointRestored(fixture.ctx, journal.CheckpointRestored{
		SchemaVersion: journal.CheckpointRestoredSchemaVersion,
		RunID:         fixture.owner.RunID,
		Release:       fixture.cycle.Release,
		Track:         fixture.cycle.Track,
		Slice:         fixture.cycle.Slice,
		CheckpointRef: cp.CheckpointRef,
		TreeOID:       cp.TreeOID,
		RestoredAt:    "2026-07-29T05:06:07Z",
	}, fixture.now); err != nil {
		t.Fatal(err)
	}

	status, err = fixture.service.Status(fixture.ctx, fixture.owner.RunID)
	if err != nil {
		t.Fatalf("status after restore: %v", err)
	}
	found = nil
	for i := range status.Checkpoints {
		if status.Checkpoints[i].CheckpointID == cp.CheckpointRef {
			found = &status.Checkpoints[i]
		}
	}
	// A salvaged, restored checkpoint reports the sudden-power-loss caveat
	// distinctly from an ordinary restored checkpoint: the guarantee is
	// weaker (partial bytes, unverified), and status must say so rather than
	// collapse the two into "restored" and imply zero-loss recovery.
	if found == nil || found.Status != "restored_salvaged" {
		t.Fatalf("checkpoint status after restore = %#v", found)
	}
}

func TestStaleReasonNamesTheFirstAuthorityDivergence(t *testing.T) {
	t.Parallel()
	current := authorityFingerprint{
		PreparedBase: "base-1", PlanOID: "plan-1",
		PlanDigest: "digest-1", ContractDigest: "contract-1",
	}
	cases := []struct {
		name       string
		checkpoint authorityFingerprint
		want       string
	}{
		{"exact match", current, ""},
		{"moved base", authorityFingerprint{
			PreparedBase: "base-0", PlanOID: "plan-1",
			PlanDigest: "digest-1", ContractDigest: "contract-1",
		}, "stale_base"},
		{"changed plan oid", authorityFingerprint{
			PreparedBase: "base-1", PlanOID: "plan-0",
			PlanDigest: "digest-1", ContractDigest: "contract-1",
		}, "stale_plan"},
		{"changed plan digest", authorityFingerprint{
			PreparedBase: "base-1", PlanOID: "plan-1",
			PlanDigest: "digest-0", ContractDigest: "contract-1",
		}, "stale_plan"},
		{"changed contract", authorityFingerprint{
			PreparedBase: "base-1", PlanOID: "plan-1",
			PlanDigest: "digest-1", ContractDigest: "contract-0",
		}, "stale_contract"},
		{"base and contract diverge: base wins the name", authorityFingerprint{
			PreparedBase: "base-0", PlanOID: "plan-1",
			PlanDigest: "digest-1", ContractDigest: "contract-0",
		}, "stale_base"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := staleReason(current, testCase.checkpoint); got != testCase.want {
				t.Fatalf("staleReason() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestLatestUnverifiedCheckpointIsReachableWithoutAnotherImplementationWrite(t *testing.T) {
	t.Parallel()
	fixture := newProductionImplementationRecoveryFixture(t, nil)
	defer fixture.workspace.Close()

	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "salvage.txt"),
		[]byte("already-written production bytes"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	plan, _, contractDigest, err := resolveSliceAuthority(fixture.state, fixture.cycle.Slice)
	if err != nil {
		t.Fatal(err)
	}
	current := authorityFingerprint{
		PreparedBase:   fixture.cycle.TrackHead,
		PlanOID:        fixture.state.Plan.OID,
		PlanDigest:     plan.Digest(),
		ContractDigest: contractDigest,
	}
	attribution := salvageAttribution(fixture, fixture.workspace, fixture.cycle.Track, fixture.owner.RunID)
	attribution.PreparedBase = current.PreparedBase
	attribution.PlanOID = current.PlanOID
	attribution.PlanDigest = current.PlanDigest
	attribution.ContractDigest = current.ContractDigest

	if err := fixture.service.captureSalvageCheckpoint(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, attribution,
	); err != nil {
		t.Fatalf("capture salvage checkpoint: %v", err)
	}

	latest, err := fixture.store.LatestUnverifiedCheckpoint(
		fixture.ctx, fixture.owner.RunID, fixture.cycle.Slice,
	)
	if err != nil || latest == nil {
		t.Fatalf("latest unverified checkpoint = %#v, %v", latest, err)
	}
	checkpointAuthority := authorityFingerprint{
		PreparedBase:   latest.PreparedBase,
		PlanOID:        latest.PlanOID,
		PlanDigest:     latest.PlanDigest,
		ContractDigest: latest.ContractDigest,
	}
	// The exact comparison a fresh implementation cycle makes before
	// deciding to restore this checkpoint into a new workspace: authority
	// matches, so the salvaged bytes are eligible for automatic restoration
	// rather than requiring a driver to rewrite them from scratch.
	if reason := staleReason(current, checkpointAuthority); reason != "" {
		t.Fatalf("salvaged checkpoint unexpectedly stale: %q", reason)
	}
}
