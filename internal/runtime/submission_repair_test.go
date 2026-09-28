package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// TestCaptureSubmissionRepairRestoresExactRefusalAndSkipsWhenSuperseded pins
// A2/Lead-correction C1: a durably reserved submission refusal for the
// immediately preceding try is restored as submission_repair on the next
// try (and validates as a legal work context), and the same refusal is not
// restored once that prior try's submission was actually accepted.
func TestCaptureSubmissionRepairRestoresExactRefusalAndSkipsWhenSuperseded(t *testing.T) {
	f := newHostCheckFixture(t, []string{"true"})
	before := sliceFingerprint(f.state, "S1")

	coordinates1 := dispatchCoordinates{
		Slice: "S1", Responsibility: driver.ImplementerImplementation,
		ProtocolAttempt: 1, Epoch: 1, Try: 1,
	}
	workContext1, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates1, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workContext1.SubmissionRepair != nil {
		t.Fatalf("first try got submission repair: %#v", workContext1.SubmissionRepair)
	}

	dispatchWork := workIdentity(
		workIdentity(before, "git.seal"), "driver.dispatch",
	)
	lane, slice := humanTurnLane(workContext1)
	planDigest, targetDigest := recoveryAuthorityDigestsForContext(
		f.manifest, &workContext1, before,
	)
	cycleID := driver.Digest(mustJSON(recoveryCycleIdentity{
		SchemaVersion:         "sworn.turn-recovery-cycle/v1",
		RunID:                 f.manifest.value.RunID,
		LaneID:                lane,
		Slice:                 slice,
		Responsibility:        coordinates1.Responsibility,
		ProtocolAttempt:       coordinates1.ProtocolAttempt,
		WorkIdentity:          dispatchWork,
		PlanAuthorityDigest:   planDigest,
		TargetAuthorityDigest: targetDigest,
	}))
	binding := journal.RecoveryBinding{
		LaneID: lane, CycleID: cycleID,
		TurnID: recoveryTurnID(cycleID, 0), ProgressID: dispatchWork,
	}
	step := journal.RecoveryStepCommand{
		RunID:   f.manifest.value.RunID,
		ID:      journal.RecoveryStepID(binding, 1),
		Binding: binding, Ordinal: 1, Kind: journal.RecoveryMalformedCorrection,
		Refusal: &journal.RecoveryStepRefusal{
			Code: "INVALID_DETAIL", Detail: "the exact refusal detail",
			SourceEpoch: 1, SourceTry: 1,
		},
	}
	now := f.service.now()
	if _, err := f.store.ReserveRecoveryStep(f.ctx, f.owner, step, now); err != nil {
		t.Fatal(err)
	}

	coordinates2 := coordinates1
	coordinates2.Try = 2
	workContext2, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	repair := workContext2.SubmissionRepair
	if repair == nil ||
		repair.RefusalCode != "INVALID_DETAIL" ||
		repair.RefusalDetail != "the exact refusal detail" ||
		repair.SourceEpoch != 1 || repair.SourceTry != 1 ||
		repair.Before != before {
		t.Fatalf("submission repair = %#v", repair)
	}
	if err := validateProductionWorkContext(f.manifest, workContext2); err != nil {
		t.Fatalf("submission repair context refused: %v", err)
	}

	// Now supersede it: try 1's own dispatch effect reached seal
	// preparation and was itself refused there (Lead correction C1 - a
	// seal-time refusal, not only an accepted submission, means the
	// field-level refusal that preceded it was corrected in-session), so a
	// fresh try 2 context must not be told it is still outstanding.
	dispatchEffectID := journal.AttemptEffectID(dispatchWork, 1, 1)
	payload := mustJSON(map[string]string{"fixture": "submitted"})
	if err := f.store.EnsureAttempt(f.ctx,
		journal.Command{RunID: f.owner.RunID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", Payload: payload, CreatedAt: now},
		journal.Effect{RunID: f.owner.RunID, ID: dispatchEffectID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", BeforeDigest: dispatchWork, ExpectedDigest: driver.Digest(payload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.ClaimOwned(f.ctx, f.owner, dispatchEffectID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	sealRefusal := mustJSON(productionRefusalBinding{
		Code: "ANCHOR_NOT_TOUCHED", Paths: []string{"docs/run.md"}, TotalPaths: 1,
	})
	sealEventBody := MarshalAssociation(EventAssociation{EffectID: dispatchEffectID})
	if err := f.store.CompleteOwned(f.ctx, f.owner, journal.Completion{
		RunID: f.owner.RunID, EffectID: dispatchEffectID, Token: claim.Token,
		State: journal.OperationalFailed, ErrorCode: "ANCHOR_NOT_TOUCHED",
		Result: sealRefusal, EventKind: "dispatch_preparation_failed",
		EventBody: sealEventBody, At: now,
	}); err != nil {
		t.Fatal(err)
	}

	workContext2Again, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workContext2Again.SubmissionRepair != nil {
		t.Fatalf(
			"superseded refusal still reported outstanding: %#v",
			workContext2Again.SubmissionRepair,
		)
	}
}

// TestCaptureSubmissionRepairCarriesAcrossEpochAndSkipsWhenPriorTryReachedSeal
// pins the cross-epoch half of Lead correction C1/A2: a refusal reserved at
// epoch 1/try 1 is carried into a fresh epoch's first try exactly as it
// would a same-epoch retry, and stops being carried once that same try's
// dispatch effect is completed OperationalFailed with a seal-time refusal
// Result, even though that effect never decodes as an accepted submission.
func TestCaptureSubmissionRepairCarriesAcrossEpochAndSkipsWhenPriorTryReachedSeal(t *testing.T) {
	f := newHostCheckFixture(t, []string{"true"})
	before := sliceFingerprint(f.state, "S1")

	coordinates1 := dispatchCoordinates{
		Slice: "S1", Responsibility: driver.ImplementerImplementation,
		ProtocolAttempt: 1, Epoch: 1, Try: 1,
	}
	workContext1, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates1, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}

	dispatchWork := workIdentity(
		workIdentity(before, "git.seal"), "driver.dispatch",
	)
	lane, slice := humanTurnLane(workContext1)
	planDigest, targetDigest := recoveryAuthorityDigestsForContext(
		f.manifest, &workContext1, before,
	)
	cycleID := driver.Digest(mustJSON(recoveryCycleIdentity{
		SchemaVersion:         "sworn.turn-recovery-cycle/v1",
		RunID:                 f.manifest.value.RunID,
		LaneID:                lane,
		Slice:                 slice,
		Responsibility:        coordinates1.Responsibility,
		ProtocolAttempt:       coordinates1.ProtocolAttempt,
		WorkIdentity:          dispatchWork,
		PlanAuthorityDigest:   planDigest,
		TargetAuthorityDigest: targetDigest,
	}))
	binding := journal.RecoveryBinding{
		LaneID: lane, CycleID: cycleID,
		TurnID: recoveryTurnID(cycleID, 0), ProgressID: dispatchWork,
	}
	step := journal.RecoveryStepCommand{
		RunID:   f.manifest.value.RunID,
		ID:      journal.RecoveryStepID(binding, 1),
		Binding: binding, Ordinal: 1, Kind: journal.RecoveryMalformedCorrection,
		Refusal: &journal.RecoveryStepRefusal{
			Code: "INVALID_DETAIL", Detail: "the exact refusal detail",
			SourceEpoch: 1, SourceTry: 1,
		},
	}
	now := f.service.now()
	if _, err := f.store.ReserveRecoveryStep(f.ctx, f.owner, step, now); err != nil {
		t.Fatal(err)
	}

	// The scheduler always records the outer git.seal attempt the moment it
	// visits an epoch/try, before the driver ever dispatches (scheduler.go's
	// implementSlice), so epoch 1/try 1's outer effect exists in the
	// journal even though the attempt died mid-correction and never
	// completed it. priorAttemptCoordinates' cross-epoch scan depends on
	// that outer effect's presence to find the highest prior epoch.
	outerWork := workIdentity(before, "git.seal")
	outerEffectID := journal.AttemptEffectID(outerWork, 1, 1)
	outerPayload := mustJSON(map[string]string{"fixture": "seal-attempt"})
	if err := f.store.EnsureAttempt(f.ctx,
		journal.Command{RunID: f.owner.RunID, ReplayKey: outerEffectID, Kind: "git.seal", Payload: outerPayload, CreatedAt: now},
		journal.Effect{RunID: f.owner.RunID, ID: outerEffectID, ReplayKey: outerEffectID, Kind: "git.seal", BeforeDigest: outerWork, ExpectedDigest: driver.Digest(outerPayload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: outerWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}

	// An operator retry starts a new epoch: epoch 1/try 1's own dispatch
	// effect never completed (the worker or host died mid-correction), so
	// the refusal must survive into epoch 2/try 1 exactly as a same-epoch
	// retry would receive it.
	coordinatesEpoch2 := coordinates1
	coordinatesEpoch2.Epoch, coordinatesEpoch2.Try = 2, 1
	workContextEpoch2, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinatesEpoch2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	repair := workContextEpoch2.SubmissionRepair
	if repair == nil ||
		repair.RefusalCode != "INVALID_DETAIL" ||
		repair.RefusalDetail != "the exact refusal detail" ||
		repair.SourceEpoch != 1 || repair.SourceTry != 1 ||
		repair.Before != before {
		t.Fatalf("cross-epoch submission repair = %#v", repair)
	}
	if err := validateProductionWorkContext(f.manifest, workContextEpoch2); err != nil {
		t.Fatalf("cross-epoch submission repair context refused: %v", err)
	}

	// Now supersede it: epoch 1/try 1's dispatch effect reached seal
	// preparation and was refused there. Epoch 2/try 1 must not be told
	// the field-level refusal is still outstanding.
	dispatchEffectID := journal.AttemptEffectID(dispatchWork, 1, 1)
	payload := mustJSON(map[string]string{"fixture": "submitted"})
	if err := f.store.EnsureAttempt(f.ctx,
		journal.Command{RunID: f.owner.RunID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", Payload: payload, CreatedAt: now},
		journal.Effect{RunID: f.owner.RunID, ID: dispatchEffectID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", BeforeDigest: dispatchWork, ExpectedDigest: driver.Digest(payload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.ClaimOwned(f.ctx, f.owner, dispatchEffectID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	sealRefusal := mustJSON(productionRefusalBinding{
		Code: "ANCHOR_NOT_TOUCHED", Paths: []string{"docs/run.md"}, TotalPaths: 1,
	})
	sealEventBody := MarshalAssociation(EventAssociation{EffectID: dispatchEffectID})
	if err := f.store.CompleteOwned(f.ctx, f.owner, journal.Completion{
		RunID: f.owner.RunID, EffectID: dispatchEffectID, Token: claim.Token,
		State: journal.OperationalFailed, ErrorCode: "ANCHOR_NOT_TOUCHED",
		Result: sealRefusal, EventKind: "dispatch_preparation_failed",
		EventBody: sealEventBody, At: now,
	}); err != nil {
		t.Fatal(err)
	}

	workContextEpoch2Again, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinatesEpoch2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workContextEpoch2Again.SubmissionRepair != nil {
		t.Fatalf(
			"superseded cross-epoch refusal still reported outstanding: %#v",
			workContextEpoch2Again.SubmissionRepair,
		)
	}
}

// TestCaptureSubmissionRepairSetsProductTreeOnlyWhenCheckpointMatches pins
// A2's checkpoint-provenance branch directly: captureSubmissionRepair sets
// ProductTree from the slice's latest unverified checkpoint only when that
// checkpoint's release, plan OID/digest, dispatch work, source epoch/try and
// prepared base all match the exact refusal being restored, and leaves it
// empty whenever any one of those fields disagrees.
func TestCaptureSubmissionRepairSetsProductTreeOnlyWhenCheckpointMatches(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*journal.UnverifiedCheckpoint)
		wantMatch bool
	}{
		{"matching checkpoint provenance is restored", nil, true},
		{
			"prepared base mismatch leaves product tree empty",
			func(cp *journal.UnverifiedCheckpoint) { cp.PreparedBase = strings.Repeat("f", 40) },
			false,
		},
		{
			"source try mismatch leaves product tree empty",
			func(cp *journal.UnverifiedCheckpoint) { cp.Try = 99 },
			false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			f := newHostCheckFixture(t, []string{"true"})
			before := sliceFingerprint(f.state, "S1")

			coordinates1 := dispatchCoordinates{
				Slice: "S1", Responsibility: driver.ImplementerImplementation,
				ProtocolAttempt: 1, Epoch: 1, Try: 1,
			}
			workContext1, _, err := captureProductionWorkContext(
				f.ctx, f.engine, coordinates1, before, driver.ReadWrite,
			)
			if err != nil {
				t.Fatal(err)
			}

			dispatchWork := workIdentity(
				workIdentity(before, "git.seal"), "driver.dispatch",
			)
			lane, slice := humanTurnLane(workContext1)
			planDigest, targetDigest := recoveryAuthorityDigestsForContext(
				f.manifest, &workContext1, before,
			)
			cycleID := driver.Digest(mustJSON(recoveryCycleIdentity{
				SchemaVersion:         "sworn.turn-recovery-cycle/v1",
				RunID:                 f.manifest.value.RunID,
				LaneID:                lane,
				Slice:                 slice,
				Responsibility:        coordinates1.Responsibility,
				ProtocolAttempt:       coordinates1.ProtocolAttempt,
				WorkIdentity:          dispatchWork,
				PlanAuthorityDigest:   planDigest,
				TargetAuthorityDigest: targetDigest,
			}))
			binding := journal.RecoveryBinding{
				LaneID: lane, CycleID: cycleID,
				TurnID: recoveryTurnID(cycleID, 0), ProgressID: dispatchWork,
			}
			step := journal.RecoveryStepCommand{
				RunID:   f.manifest.value.RunID,
				ID:      journal.RecoveryStepID(binding, 1),
				Binding: binding, Ordinal: 1, Kind: journal.RecoveryMalformedCorrection,
				Refusal: &journal.RecoveryStepRefusal{
					Code: "INVALID_DETAIL", Detail: "the exact refusal detail",
					SourceEpoch: 1, SourceTry: 1,
				},
			}
			now := f.service.now()
			if _, err := f.store.ReserveRecoveryStep(f.ctx, f.owner, step, now); err != nil {
				t.Fatal(err)
			}

			checkpoint := journal.UnverifiedCheckpoint{
				Repository:    f.manifest.value.Repository,
				RunID:         f.manifest.value.RunID,
				Release:       f.manifest.value.Release,
				Track:         "T1",
				Slice:         coordinates1.Slice,
				PlanOID:       workContext1.Plan.OID,
				PlanDigest:    workContext1.Plan.Digest,
				PreparedBase:  workContext1.Authority.TrackHead,
				DispatchWork:  dispatchWork,
				Epoch:         1,
				Try:           1,
				CheckpointRef: "refs/heads/checkpoints/fixture/S1-1-1",
				CommitOID:     strings.Repeat("c", 40),
				TreeOID:       strings.Repeat("d", 40),
				TreeDigest:    "sha256:" + strings.Repeat("e", 64),
			}
			if test.mutate != nil {
				test.mutate(&checkpoint)
			}
			if err := f.store.RecordUnverifiedCheckpoint(f.ctx, checkpoint, now); err != nil {
				t.Fatal(err)
			}

			coordinates2 := coordinates1
			coordinates2.Try = 2
			workContext2, _, err := captureProductionWorkContext(
				f.ctx, f.engine, coordinates2, before, driver.ReadWrite,
			)
			if err != nil {
				t.Fatal(err)
			}
			repair := workContext2.SubmissionRepair
			if repair == nil {
				t.Fatal("submission repair is nil")
			}
			if test.wantMatch {
				if repair.ProductTree != checkpoint.TreeDigest {
					t.Fatalf(
						"product tree = %q, want %q", repair.ProductTree, checkpoint.TreeDigest,
					)
				}
				return
			}
			if repair.ProductTree != "" {
				t.Fatalf("product tree = %q, want empty on mismatch", repair.ProductTree)
			}
		})
	}
}

// submissionRepairSupersedeSetup builds the common fixture the seal-
// preparation-failure and accepted-submission supersede tests share: epoch
// 1/try 1 reserves an INVALID_DETAIL field-level refusal, and try 2's work
// context is confirmed to carry it as submission_repair before the caller
// completes try 1's own dispatch effect to test whether that reported
// repair is then correctly superseded.
func submissionRepairSupersedeSetup(t *testing.T) (
	f *hostCheckFixture, before, dispatchWork string,
	coordinates2 dispatchCoordinates, now time.Time,
) {
	t.Helper()
	f = newHostCheckFixture(t, []string{"true"})
	before = sliceFingerprint(f.state, "S1")

	coordinates1 := dispatchCoordinates{
		Slice: "S1", Responsibility: driver.ImplementerImplementation,
		ProtocolAttempt: 1, Epoch: 1, Try: 1,
	}
	workContext1, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates1, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}

	dispatchWork = workIdentity(
		workIdentity(before, "git.seal"), "driver.dispatch",
	)
	lane, slice := humanTurnLane(workContext1)
	planDigest, targetDigest := recoveryAuthorityDigestsForContext(
		f.manifest, &workContext1, before,
	)
	cycleID := driver.Digest(mustJSON(recoveryCycleIdentity{
		SchemaVersion:         "sworn.turn-recovery-cycle/v1",
		RunID:                 f.manifest.value.RunID,
		LaneID:                lane,
		Slice:                 slice,
		Responsibility:        coordinates1.Responsibility,
		ProtocolAttempt:       coordinates1.ProtocolAttempt,
		WorkIdentity:          dispatchWork,
		PlanAuthorityDigest:   planDigest,
		TargetAuthorityDigest: targetDigest,
	}))
	binding := journal.RecoveryBinding{
		LaneID: lane, CycleID: cycleID,
		TurnID: recoveryTurnID(cycleID, 0), ProgressID: dispatchWork,
	}
	step := journal.RecoveryStepCommand{
		RunID:   f.manifest.value.RunID,
		ID:      journal.RecoveryStepID(binding, 1),
		Binding: binding, Ordinal: 1, Kind: journal.RecoveryMalformedCorrection,
		Refusal: &journal.RecoveryStepRefusal{
			Code: "INVALID_DETAIL", Detail: "the exact refusal detail",
			SourceEpoch: 1, SourceTry: 1,
		},
	}
	now = f.service.now()
	if _, err := f.store.ReserveRecoveryStep(f.ctx, f.owner, step, now); err != nil {
		t.Fatal(err)
	}

	coordinates2 = coordinates1
	coordinates2.Try = 2
	workContext2, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workContext2.SubmissionRepair == nil {
		t.Fatal("expected submission repair to be reported before supersede")
	}
	return f, before, dispatchWork, coordinates2, now
}

// TestCaptureSubmissionRepairSkipsWhenPriorTryReachedSealWithPathlessRefusal
// pins A1 directly against the real production shape: a prior try's own
// dispatch effect completed OperationalFailed under EMPTY_CANDIDATE, which
// carries no productionRefusalBinding Result at all (Result is nil), so
// neither the decode-success nor the host-repair branch can pass. Only the
// dispatch_preparation_failed event-kind branch (Lead correction C1)
// recognises this as having reached the seal; removing that branch fails
// this test.
func TestCaptureSubmissionRepairSkipsWhenPriorTryReachedSealWithPathlessRefusal(t *testing.T) {
	f, before, dispatchWork, coordinates2, now := submissionRepairSupersedeSetup(t)
	dispatchEffectID := journal.AttemptEffectID(dispatchWork, 1, 1)
	commandPayload := mustJSON(map[string]string{"fixture": "empty-candidate"})
	if err := f.store.EnsureAttempt(f.ctx,
		journal.Command{RunID: f.owner.RunID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", Payload: commandPayload, CreatedAt: now},
		journal.Effect{RunID: f.owner.RunID, ID: dispatchEffectID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", BeforeDigest: dispatchWork, ExpectedDigest: driver.Digest(commandPayload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.ClaimOwned(f.ctx, f.owner, dispatchEffectID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	eventBody := MarshalAssociation(EventAssociation{EffectID: dispatchEffectID})
	if err := f.store.CompleteOwned(f.ctx, f.owner, journal.Completion{
		RunID: f.owner.RunID, EffectID: dispatchEffectID, Token: claim.Token,
		State: journal.OperationalFailed, ErrorCode: "EMPTY_CANDIDATE",
		EventKind: "dispatch_preparation_failed", EventBody: eventBody, At: now,
	}); err != nil {
		t.Fatal(err)
	}

	workContext2Again, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workContext2Again.SubmissionRepair != nil {
		t.Fatalf(
			"path-less seal refusal not recognised as superseding: %#v",
			workContext2Again.SubmissionRepair,
		)
	}
}

// TestCaptureSubmissionRepairDoesNotSkipOnOperationalFailureThatNeverReachedSeal
// pins C2's negative half: a prior try's dispatch effect completed
// OperationalFailed under dispatch_operational_failure (invalid_driver_handoff,
// which by construction never passed DecodeSubmission and so never reached
// prepareHandoff) must not be treated as having resolved the refusal. This
// kills the over-broad mutant that would treat any OperationalFailed effect
// as resolving, whatever its event kind.
func TestCaptureSubmissionRepairDoesNotSkipOnOperationalFailureThatNeverReachedSeal(t *testing.T) {
	f, before, dispatchWork, coordinates2, now := submissionRepairSupersedeSetup(t)
	dispatchEffectID := journal.AttemptEffectID(dispatchWork, 1, 1)
	commandPayload := mustJSON(map[string]string{"fixture": "invalid-handoff"})
	if err := f.store.EnsureAttempt(f.ctx,
		journal.Command{RunID: f.owner.RunID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", Payload: commandPayload, CreatedAt: now},
		journal.Effect{RunID: f.owner.RunID, ID: dispatchEffectID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", BeforeDigest: dispatchWork, ExpectedDigest: driver.Digest(commandPayload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.ClaimOwned(f.ctx, f.owner, dispatchEffectID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	eventBody := MarshalAssociation(EventAssociation{EffectID: dispatchEffectID})
	if err := f.store.CompleteOwned(f.ctx, f.owner, journal.Completion{
		RunID: f.owner.RunID, EffectID: dispatchEffectID, Token: claim.Token,
		State: journal.OperationalFailed, ErrorCode: "invalid_driver_handoff",
		EventKind: "dispatch_operational_failure", EventBody: eventBody, At: now,
	}); err != nil {
		t.Fatal(err)
	}

	workContext2Again, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workContext2Again.SubmissionRepair == nil {
		t.Fatal(
			"an operational failure that never reached the seal must not " +
				"be treated as superseding the outstanding refusal",
		)
	}
}

// TestCaptureSubmissionRepairSkipsWhenPriorTryReachedSealWithAcceptedSubmission
// pins A2's accepted-submission branch directly: a prior try's dispatch
// effect completed Succeeded with an actual driver.EncodeSubmission-encoded
// Submission as its Result (no seal-preparation event at all) must count as
// resolved through the decode-success branch alone. Neither of the two
// existing seal-refusal tests exercises this branch: both complete their
// effect OperationalFailed with an ANCHOR_NOT_TOUCHED refusal, which is the
// seal-refusal branch, not this one.
func TestCaptureSubmissionRepairSkipsWhenPriorTryReachedSealWithAcceptedSubmission(t *testing.T) {
	f, before, dispatchWork, coordinates2, now := submissionRepairSupersedeSetup(t)
	dispatchEffectID := journal.AttemptEffectID(dispatchWork, 1, 1)
	invocationID := dispatchInvocationID(f.manifest.value.RunID, dispatchCoordinates{
		Slice: "S1", Responsibility: driver.ImplementerImplementation,
		ProtocolAttempt: 1, Epoch: 1, Try: 1,
	})
	checks, err := driver.NewCheckBytes([]byte("accepted submission fixture checks\n"))
	if err != nil {
		t.Fatal(err)
	}
	submission := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   invocationID,
		Responsibility: driver.ImplementerImplementation,
		Summary:        "Accepted implementer submission fixture.",
		Detail:         "Bound to the exact prior-try dispatch effect this test completes as Succeeded.",
		Checks:         checks,
	}
	submissionBody, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	commandPayload := mustJSON(map[string]string{"fixture": "accepted"})
	if err := f.store.EnsureAttempt(f.ctx,
		journal.Command{RunID: f.owner.RunID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", Payload: commandPayload, CreatedAt: now},
		journal.Effect{RunID: f.owner.RunID, ID: dispatchEffectID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch", BeforeDigest: dispatchWork, ExpectedDigest: driver.Digest(commandPayload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.ClaimOwned(f.ctx, f.owner, dispatchEffectID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.CompleteOwned(f.ctx, f.owner, journal.Completion{
		RunID: f.owner.RunID, EffectID: dispatchEffectID, Token: claim.Token,
		State: journal.Succeeded, Result: submissionBody,
		EventKind: "dispatch_completed", At: now,
	}); err != nil {
		t.Fatal(err)
	}

	workContext2Again, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates2, before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if workContext2Again.SubmissionRepair != nil {
		t.Fatalf(
			"accepted submission not recognised as superseding: %#v",
			workContext2Again.SubmissionRepair,
		)
	}
}
