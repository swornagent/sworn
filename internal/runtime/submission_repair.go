package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const submissionRepairVersion = "sworn.submission-repair/v1"

// productionSubmissionRepair is unverified repair input, never candidate
// admission, check PASS, or a Decision - the same disclaimer
// productionHostRepair already carries. It restores the exact outstanding
// submission refusal a killed worker or host left durable, alongside the
// checkpoint provenance needed to keep repairing the same retained code
// instead of regenerating it.
type productionSubmissionRepair struct {
	SchemaVersion string `json:"schema_version"`
	Before        string `json:"before"`
	Plan          string `json:"plan"`
	PreparedBase  string `json:"prepared_base"`
	// ProductTree is omitted when the correction loop that raised the
	// refusal never reached a durable checkpoint (a code-free correction,
	// or one made before any checkpoint was written).
	ProductTree   string `json:"product_tree,omitempty"`
	SourceEpoch   int64  `json:"source_epoch"`
	SourceTry     int64  `json:"source_try"`
	RefusalCode   string `json:"refusal_code"`
	RefusalDetail string `json:"refusal_detail,omitempty"`
}

// captureSubmissionRepair restores the exact outstanding submission refusal
// from the immediately preceding implementer attempt, when that refusal was
// never superseded by an accepted submission (Lead correction C1) - the
// journal's turn-recovery budget records it durably before it ever reaches
// the worker (internal/driver's rejectSubmission -> RecoveryStepHook), so it
// survives worker death, host death and an invocation timeout mid-correction
// loop alike. The one honestly-accepted non-coverage is documented at
// journal.recoveryBudgetAllows: when the 10,000-automatic-action or
// 1,000-correction runaway guard itself refuses the reservation, the
// triggering refusal is not captured - the same class of bound the driver's
// own 1000-correction cap already accepts as unreachable in any real
// invocation.
func captureSubmissionRepair(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	workContext *productionWorkContext,
) (*productionSubmissionRepair, error) {
	if coordinates.Responsibility != driver.ImplementerImplementation ||
		(coordinates.Try <= 1 && coordinates.Epoch <= 1) {
		return nil, nil
	}
	prior, found, err := priorAttemptCoordinates(ctx, engine, coordinates, before)
	if err != nil {
		return nil, err
	}
	if !found || workContext.Plan == nil {
		return nil, nil
	}
	dispatchWork := workIdentity(
		workIdentity(before, "git.seal"), "driver.dispatch",
	)
	lane, slice := humanTurnLane(*workContext)
	planDigest, targetDigest := recoveryAuthorityDigestsForContext(
		engine.manifest, workContext, before,
	)
	cycleID := driver.Digest(mustJSON(recoveryCycleIdentity{
		SchemaVersion:         "sworn.turn-recovery-cycle/v1",
		RunID:                 engine.manifest.value.RunID,
		LaneID:                lane,
		Slice:                 slice,
		Responsibility:        coordinates.Responsibility,
		ProtocolAttempt:       coordinates.ProtocolAttempt,
		WorkIdentity:          dispatchWork,
		PlanAuthorityDigest:   planDigest,
		TargetAuthorityDigest: targetDigest,
	}))
	binding := journal.RecoveryBinding{
		LaneID:     lane,
		CycleID:    cycleID,
		TurnID:     recoveryTurnID(cycleID, 0),
		ProgressID: dispatchWork,
	}
	budget, err := engine.journal.RecoveryBudget(
		ctx, engine.manifest.value.RunID, binding,
	)
	if err != nil {
		return nil, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	refusal := budget.LastRefusal
	if refusal == nil ||
		refusal.SourceEpoch != prior.Epoch || refusal.SourceTry != prior.Try {
		return nil, nil
	}
	superseded, err := priorTryResolvedInSession(
		ctx, engine, coordinates, before, prior.Epoch, prior.Try,
	)
	if err != nil {
		return nil, err
	}
	if superseded {
		return nil, nil
	}
	repair := &productionSubmissionRepair{
		SchemaVersion: submissionRepairVersion,
		Before:        before,
		Plan:          workContext.Plan.OID,
		PreparedBase:  workContext.Authority.TrackHead,
		SourceEpoch:   prior.Epoch,
		SourceTry:     prior.Try,
		RefusalCode:   refusal.Code,
		RefusalDetail: refusal.Detail,
	}
	cp, err := engine.journal.LatestUnverifiedCheckpoint(
		ctx, engine.manifest.value.RunID, coordinates.Slice,
	)
	if err != nil {
		return nil, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if cp != nil && cp.Release == engine.manifest.value.Release &&
		cp.PlanOID == repair.Plan && cp.PlanDigest == workContext.Plan.Digest &&
		cp.DispatchWork == dispatchWork &&
		cp.Epoch == prior.Epoch && cp.Try == prior.Try &&
		cp.PreparedBase == repair.PreparedBase {
		repair.ProductTree = cp.TreeDigest
	}
	return repair, nil
}

// priorTryResolvedInSession reports whether the implementer attempt at
// (epoch, try) resolved its own outstanding submission refusal in-session,
// over the same candidate dispatch-effect identities capturePriorSubmission's
// own per-try scan probes. That try either sealed a decodable submission, or
// its correction got past submission-time validation and reached seal
// preparation (the anchor gate, host checks or git.seal itself), which
// completes that same dispatch effect OperationalFailed with a decodable
// productionRefusalBinding or host-repair Result instead of the submission
// bytes (Lead correction C1: dispatch.go's prepareHandoff branch and
// scheduler.go's outer git.seal completion both write
// extractRefusalResult(err), never the submission, so a seal-time refusal
// must count as resolving the earlier field-level refusal exactly as an
// accepted submission does). Either shape means the next continuation must
// not be told an already-resolved refusal is still outstanding.
func priorTryResolvedInSession(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	epoch, try int64,
) (bool, error) {
	generalWork := driverWorkIdentity(
		engine.manifest.digest,
		coordinates.Slice,
		coordinates.Responsibility,
		coordinates.ProtocolAttempt,
		before,
	)
	dispatchEffect := journal.AttemptEffectID(generalWork, epoch, try)
	workID := workIdentity(before, "git.seal")
	dispatchWorkRec := workIdentity(workID, "driver.dispatch")
	dispatchEffectRec := journal.AttemptEffectID(dispatchWorkRec, epoch, try)
	priorOuterID := journal.AttemptEffectID(workID, epoch, try)
	dispatchWorkOuter := workIdentity(priorOuterID, "driver.dispatch")
	dispatchEffectOuter := journal.AttemptEffectID(dispatchWorkOuter, 1, 1)

	for _, effectID := range []string{
		dispatchEffectRec, dispatchEffectOuter, dispatchEffect, priorOuterID,
	} {
		effect, err := engine.journal.Effect(
			ctx, engine.manifest.value.RunID, effectID,
		)
		if err != nil || len(effect.Result) == 0 {
			continue
		}
		if _, decErr := driver.DecodeSubmission(effect.Result); decErr == nil {
			return true, nil
		}
		if effect.State != journal.OperationalFailed {
			continue
		}
		var hostRepair productionHostRepair
		if json.Unmarshal(effect.Result, &hostRepair) == nil &&
			hostRepair.SchemaVersion == hostRepairVersion {
			return true, nil
		}
		var refusal productionRefusalBinding
		if json.Unmarshal(effect.Result, &refusal) == nil &&
			refusal.Code != "" && len(refusal.Paths) > 0 {
			return true, nil
		}
	}
	return false, nil
}

func validateSubmissionRepair(repair productionSubmissionRepair) error {
	if repair.SchemaVersion != submissionRepairVersion ||
		!runtimeDigestPattern.MatchString(repair.Before) ||
		!validGitObjectID(repair.Plan) ||
		!validGitObjectID(repair.PreparedBase) ||
		(repair.ProductTree != "" &&
			!runtimeDigestPattern.MatchString(repair.ProductTree)) ||
		repair.SourceEpoch < 1 || repair.SourceTry < 1 ||
		!runtimeIdentityPattern.MatchString(repair.RefusalCode) ||
		!validSubmissionRepairDetail(repair.RefusalDetail) {
		return runtimeFail("SUBMISSION_REPAIR_BINDING_FAILED", nil)
	}
	return nil
}

func validSubmissionRepairDetail(detail string) bool {
	return utf8.ValidString(detail) &&
		len([]byte(detail)) <= driver.MaxSubmissionDetailBytes &&
		!strings.ContainsRune(detail, '\x00') &&
		!strings.ContainsRune(detail, '\r')
}
