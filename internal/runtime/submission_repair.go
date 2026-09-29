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
// preparation (the anchor gate, host checks, contract resolution or
// git.seal itself). A seal-preparation failure is detected by the dispatch
// effect's own recorded event kind, "dispatch_preparation_failed" (Lead
// correction C1), never by decoding its Result: dispatch.go's prepareHandoff
// branch writes extractRefusalResult(err) as Result, which is empty for
// several real seal-time codes (EMPTY_CANDIDATE, ANCHOR_GATE_UNREADABLE,
// CANDIDATE_SEAL_FAILED, CONTRACT_RESOLUTION_FAILED), so a Result-shape test
// silently missed exactly those. The event kind is the accepted-handoff
// signal the dispatch already records: prepareHandoff runs, and can only
// fail there, after driver.DecodeSubmission and every earlier field-level
// check already passed. Either shape - an accepted submission or a
// seal-preparation failure - means the next continuation must not be told
// an already-resolved refusal is still outstanding.
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

	snapshot, err := engineSnapshot(ctx, engine)
	if err != nil {
		return false, err
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects[effect.ID] = effect
	}

	for _, effectID := range []string{
		dispatchEffectRec, dispatchEffectOuter, dispatchEffect, priorOuterID,
	} {
		effect, found := effects[effectID]
		if !found {
			continue
		}
		if len(effect.Result) > 0 {
			if _, decErr := driver.DecodeSubmission(effect.Result); decErr == nil {
				return true, nil
			}
		}
		if effect.State != journal.OperationalFailed {
			continue
		}
		if len(effect.Result) > 0 {
			var hostRepair productionHostRepair
			if json.Unmarshal(effect.Result, &hostRepair) == nil &&
				hostRepair.SchemaVersion == hostRepairVersion {
				return true, nil
			}
		}
		if dispatchEffectReachedSealPreparation(snapshot, effectID) {
			return true, nil
		}
	}
	return false, nil
}

// dispatchEffectReachedSealPreparation reports whether effectID's dispatch
// attempt failed inside prepareHandoff: the journal records this as an event
// whose kind carries the "dispatch_preparation_failed" prefix (a
// continuation suffix may extend it, mirroring isFailureTurnContextKind's
// own prefix test) and whose association names this exact effect. This is
// the same EventAssociation/EffectID pairing failure_turn_context.go already
// reads for an unrelated purpose, not a new signal.
func dispatchEffectReachedSealPreparation(
	snapshot journal.Snapshot, effectID string,
) bool {
	for _, event := range snapshot.Events {
		if !strings.HasPrefix(event.Kind, "dispatch_preparation_failed") {
			continue
		}
		assoc, _ := parseFailureEventBody(event.Body)
		if assoc.EffectID == effectID {
			return true
		}
	}
	return false
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
