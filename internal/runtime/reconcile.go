package runtime

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

// authorityFingerprint names the exact authority a checkpoint or attribution
// was captured against: the track head it branched from, and the plan and
// contract in force at that moment. Restoration compares this fingerprint by
// exact equality, never ancestry, because track bases are prepared by
// explicit rebinding rather than fast-forward-only history.
type authorityFingerprint struct {
	PreparedBase   string
	PlanOID        string
	PlanDigest     string
	ContractDigest string
}

// staleReason reports the first way checkpoint's recorded authority diverges
// from current, or "" when they match exactly. The order matters only for
// naming: target/base movement is reported as stale_base rather than a
// separate stale_target code, since a moved track base already implies the
// checkpoint branched from authority that is no longer current.
func staleReason(current, checkpoint authorityFingerprint) string {
	switch {
	case checkpoint.PreparedBase != current.PreparedBase:
		return "stale_base"
	case checkpoint.PlanOID != current.PlanOID || checkpoint.PlanDigest != current.PlanDigest:
		return "stale_plan"
	case checkpoint.ContractDigest != current.ContractDigest:
		return "stale_contract"
	default:
		return ""
	}
}

// resolveSliceAuthority derives the plan, its declared slice, and that
// slice's contract digest from an already-read state, the same derivation
// captureImplementationCheckpoint already performs before recording a
// journal checkpoint.
func resolveSliceAuthority(fresh baton.State, slice string) (baton.Plan, baton.Slice, string, error) {
	plan, err := planFromState(fresh)
	if err != nil {
		return baton.Plan{}, baton.Slice{}, "", err
	}
	_, declared, ok := plan.FindSlice(slice)
	if !ok {
		return baton.Plan{}, baton.Slice{}, "", fmt.Errorf("slice %s not found in plan", slice)
	}
	contractDigest, _ := plan.Contract(slice)
	return plan, declared, contractDigest, nil
}

// currentCheckpointStaleReason computes cp's stale reason against the run's
// present authority for display (sworn status --json and the board). It
// never mutates or re-labels the checkpoint itself; an unreadable or
// mismatched-release state simply yields no opinion rather than a false
// positive.
func currentCheckpointStaleReason(state baton.State, stateErr error, cp journal.UnverifiedCheckpoint) string {
	if stateErr != nil || state.Release != cp.Release {
		return ""
	}
	plan, err := planFromState(state)
	if err != nil {
		return ""
	}
	var trackHead string
	for _, track := range state.Refs.Tracks {
		if track.ID == cp.Track {
			trackHead = track.Head
			break
		}
	}
	if trackHead == "" {
		return ""
	}
	contractDigest, _ := plan.Contract(cp.Slice)
	current := authorityFingerprint{
		PreparedBase:   trackHead,
		PlanOID:        state.Plan.OID,
		PlanDigest:     plan.Digest(),
		ContractDigest: contractDigest,
	}
	checkpoint := authorityFingerprint{
		PreparedBase:   cp.PreparedBase,
		PlanOID:        cp.PlanOID,
		PlanDigest:     cp.PlanDigest,
		ContractDigest: cp.ContractDigest,
	}
	return staleReason(current, checkpoint)
}

// reconcileInterruptedWorkspaces runs as the first step of every owned
// cycle, immediately after openEngine's prepareRoot/recoverAbandoned pass has
// already deferred deletion of any durably attributed abandoned workspace.
// It adopts each attributed workspace under this run, preserves whatever it
// finds as a salvaged unverified checkpoint, and only then lets ordinary
// cleanup reclaim the tree - so nothing destroys interrupted production work
// between a hard process exit and this reconciliation.
func (s *Service) reconcileInterruptedWorkspaces(
	ctx context.Context, engine *engine, owner journal.OwnerLease,
) error {
	attributions, err := engine.workspaces.ListAttributions()
	if err != nil {
		return runtimeFail("WORKSPACE_CLEANUP_FAILED", err)
	}
	for _, attribution := range attributions {
		if err := s.reconcileOneWorkspace(ctx, engine, owner, attribution); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) reconcileOneWorkspace(
	ctx context.Context, engine *engine, owner journal.OwnerLease,
	attribution gitx.WorkspaceAttribution,
) error {
	key := gitx.TrackKey{Release: attribution.Release, Track: attribution.Track}
	if attribution.RunID != owner.RunID || attribution.CommonDir != engine.repository.CommonDir() {
		return fenceAttributedTree(
			engine, attribution,
			"WORKSPACE_OWNERSHIP_MISMATCH", "attribution names a foreign run or repository",
		)
	}
	head, headErr := gitx.ParseOID(engine.repository.ObjectFormat(), attribution.PreparedBase)
	if headErr != nil {
		return fenceCorruptAttribution(engine, attribution, "attributed prepared base does not parse")
	}
	if _, err := engine.repository.CommitTimestamp(head); err != nil {
		return fenceCorruptAttribution(engine, attribution, "attributed prepared base does not resolve")
	}
	workspace, adoptErr := engine.workspaces.AdoptAbandonedWorkspace(attribution.Token, key, head)
	if adoptErr != nil {
		if stableErrorCode(adoptErr) == "WORKSPACE_OWNER_ACTIVE" ||
			stableErrorCode(adoptErr) == "WORKSPACE_FENCED" {
			// A live or already-quarantined prior worker: leave the tree and
			// its attribution exactly as they are for a later owned cycle.
			return nil
		}
		return fenceCorruptAttribution(engine, attribution, "adoption failed: "+adoptErr.Error())
	}
	captureErr := s.captureSalvageCheckpoint(ctx, engine, owner, workspace, attribution)
	if captureErr != nil {
		if !workspace.IsFenced() {
			_ = workspace.Fence("CHECKPOINT_CAPTURE_FAILED", captureErr.Error(), attribution.Slice)
		}
		_ = workspace.Close()
		return nil
	}
	// A non-fenced Close deletes the now-checkpointed (or found-empty) tree
	// and clears its attribution as one step (Workspaces.remove); a fenced
	// close leaves both the tree and its quarantine marker in place, and
	// Fence already cleared the attribution when it fenced.
	return workspace.Close()
}

// fenceCorruptAttribution quarantines an attributed tree this run cannot
// safely adopt (a corrupt or foreign recovery binding), preserving the
// original bytes instead of discarding them to make the refusal disappear.
func fenceCorruptAttribution(engine *engine, attribution gitx.WorkspaceAttribution, detail string) error {
	return fenceAttributedTree(engine, attribution, "CHECKPOINT_CORRUPT", detail)
}

func fenceAttributedTree(
	engine *engine, attribution gitx.WorkspaceAttribution, reason, detail string,
) error {
	key := gitx.TrackKey{Release: attribution.Release, Track: attribution.Track}
	if err := engine.workspaces.FenceAbandonedWorkspace(
		attribution.Token, key, attribution.Slice, reason, detail,
	); err != nil {
		return runtimeFail("WORKSPACE_FENCE_FAILED", err)
	}
	return engine.workspaces.ClearAttribution(attribution.Token)
}

// captureSalvageCheckpoint captures an adopted abandoned workspace's current
// contents as a checkpoint recorded with Salvaged: true, using the authority
// and scope the attribution recorded at dispatch-open time rather than the
// run's present plan or contract - an authority change while a dispatch was
// in flight is salvaged under its original scope and surfaced as stale by
// the same comparison a normal restore uses, never silently re-scoped.
func (s *Service) captureSalvageCheckpoint(
	ctx context.Context, engine *engine, owner journal.OwnerLease,
	workspace *gitx.WorkspaceLease, attribution gitx.WorkspaceAttribution,
) error {
	scope := gitx.CheckpointScope{
		Include: attribution.ScopeInclude,
		Exclude: attribution.ScopeExclude,
	}
	var existingStagedBytes int64
	allCheckpoints, listErr := s.journal.ListUnverifiedCheckpoints(ctx, owner.RunID)
	if listErr != nil {
		return runtimeFail("CHECKPOINT_STATE_LOOKUP_FAILED", listErr)
	}
	for _, cp := range allCheckpoints {
		existingStagedBytes += cp.StagedBytes
	}
	attempt := gitx.CheckpointAttempt{
		WorkID: attribution.DispatchWork,
		Epoch:  attribution.Epoch,
		Try:    attribution.Try,
	}
	result, captureErr := engine.workspaces.CaptureCheckpoint(
		workspace, attempt, attribution.Slice, scope, existingStagedBytes, engine.product,
	)
	if captureErr != nil {
		return captureErr
	}
	if result.Empty {
		return nil
	}
	if testCrashAfterEffect == "checkpoint.bind" {
		os.Exit(86)
	}
	cp := journal.UnverifiedCheckpoint{
		SchemaVersion:  journal.UnverifiedCheckpointSchemaVersion,
		Repository:     engine.manifest.value.Repository,
		RunID:          owner.RunID,
		Release:        attribution.Release,
		Track:          attribution.Track,
		Slice:          attribution.Slice,
		PlanOID:        attribution.PlanOID,
		PlanDigest:     attribution.PlanDigest,
		ContractPath:   attribution.ContractPath,
		ContractDigest: attribution.ContractDigest,
		PreparedBase:   attribution.PreparedBase,
		DispatchWork:   attribution.DispatchWork,
		Epoch:          attribution.Epoch,
		Try:            attribution.Try,
		CheckpointRef:  result.Ref,
		CommitOID:      result.Commit.String(),
		TreeOID:        result.Tree.String(),
		TreeDigest:     result.ProductTree,
		StagedBytes:    result.StagedBytes,
		FileCount:      result.FileCount,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Salvaged:       true,
	}
	captureCtx := context.WithoutCancel(ctx)
	if recErr := s.journal.RecordUnverifiedCheckpoint(captureCtx, cp, time.Now()); recErr != nil {
		_ = workspace.Fence("CHECKPOINT_JOURNAL_FAILED", recErr.Error(), attribution.Slice)
		return runtimeFail("CHECKPOINT_JOURNAL_FAILED", recErr)
	}
	if testCrashAfterEffect == "checkpoint.publish" {
		os.Exit(86)
	}
	_, _ = engine.workspaces.PruneCheckpointGenerations(
		workspace.Key(), attempt.WorkID, gitx.MaxCheckpointGenerations, result.Ref,
	)
	return nil
}
