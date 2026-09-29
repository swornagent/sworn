package runtime

import (
	"context"
	"errors"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// A run-snapshot native adapter's CLI identity is fixed once per run: the
// command replay key names the adapter, so each run records exactly one
// snapshot fact per adapter, and the informational event carries the same
// canonical body.
const (
	nativeCLISnapshotCommandKind = "native-cli-snapshot"
	nativeCLISnapshotEventKind   = "native_cli_snapshot"
)

func nativeCLISnapshotReplayKey(adapter string) string {
	return nativeCLISnapshotCommandKind + "/" + adapter
}

// recordNativeCLISnapshots runs at the start of every drive cycle, before the
// engine opens and so before any dispatch. For each run-snapshot adapter the
// manifest uses, a fact already recorded for this run is kept as it is: a
// serve restart or a retry never resolves the host binary again. Only an
// adapter with no fact yet, which no dispatch in this run can have used, is
// snapshotted and recorded.
func (s *Service) recordNativeCLISnapshots(
	ctx context.Context,
	manifest admittedManifest,
) error {
	if !manifest.value.production() || s.production == nil {
		return nil
	}
	adapters, err := s.production.nativeCLISnapshotAdapters(manifest)
	if err != nil {
		return err
	}
	runID := manifest.value.RunID
	for _, adapter := range adapters {
		replayKey := nativeCLISnapshotReplayKey(adapter)
		_, err := s.journal.Command(ctx, runID, replayKey)
		if err == nil {
			continue
		}
		if !journal.IsCode(err, "COMMAND_NOT_FOUND") {
			return runtimeFail("JOURNAL_READ_FAILED", err)
		}
		snapshot, err := s.production.config.SnapshotNativeCLI(ctx, adapter, runID)
		if err != nil {
			return nativeCLISnapshotFail(err)
		}
		body, err := driver.EncodeNativeCLISnapshot(snapshot)
		if err != nil {
			return nativeCLISnapshotFail(err)
		}
		now := s.now().UTC()
		// A concurrent start that recorded first wins: its fact stands and
		// is the one the engine loads.
		if err := s.journal.AppendEventOnce(ctx, journal.Command{
			RunID:     runID,
			ReplayKey: replayKey,
			Kind:      nativeCLISnapshotCommandKind,
			Payload:   body,
			CreatedAt: now,
		}, nativeCLISnapshotEventKind, body, now); err != nil &&
			!journal.IsCode(err, "REPLAY_CONFLICT") {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
	}
	return nil
}

// recordedNativeCLISnapshots loads and verifies this run's snapshot fact for
// every run-snapshot adapter the manifest uses. A fact that is unrecorded or
// malformed, or whose snapshot file is missing or no longer holds the
// recorded bytes, fails closed: the host binary is never consulted here.
func (s *Service) recordedNativeCLISnapshots(
	ctx context.Context,
	manifest admittedManifest,
) (map[string]driver.NativeCLISnapshot, error) {
	adapters, err := s.production.nativeCLISnapshotAdapters(manifest)
	if err != nil || len(adapters) == 0 {
		return nil, err
	}
	runID := manifest.value.RunID
	snapshots := make(map[string]driver.NativeCLISnapshot, len(adapters))
	for _, adapter := range adapters {
		command, err := s.journal.Command(
			ctx, runID, nativeCLISnapshotReplayKey(adapter),
		)
		if journal.IsCode(err, "COMMAND_NOT_FOUND") {
			return nil, runtimeFailSite(
				"NATIVE_CLI_SNAPSHOT_INVALID", "snapshot_unrecorded", err,
			)
		}
		if err != nil {
			return nil, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		snapshot, err := driver.DecodeNativeCLISnapshot(command.Payload)
		if err != nil {
			return nil, nativeCLISnapshotFail(err)
		}
		if command.Kind != nativeCLISnapshotCommandKind ||
			snapshot.RunID != runID || snapshot.Adapter != adapter {
			return nil, runtimeFailSite(
				"NATIVE_CLI_SNAPSHOT_INVALID", "fact_invalid", nil,
			)
		}
		if err := driver.VerifyNativeCLISnapshot(snapshot); err != nil {
			return nil, nativeCLISnapshotFail(err)
		}
		snapshots[adapter] = snapshot
	}
	return snapshots, nil
}

// nativeCLISnapshotFail keeps the driver's typed code and detail as the
// runtime refusal.
func nativeCLISnapshotFail(err error) error {
	var contractErr *driver.ContractError
	if errors.As(err, &contractErr) {
		return runtimeFailSite(contractErr.Code, contractErr.Detail, err)
	}
	return runtimeFail("NATIVE_CLI_SNAPSHOT_UNAVAILABLE", err)
}
