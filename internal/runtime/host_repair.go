package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

const hostRepairVersion = "sworn.host-check-repair/v1"

// productionHostRepair is unverified repair input, never candidate admission
// or verifier evidence. The failed dispatch owns it; the existing checkpoint
// mechanism owns retention and materialization of its product files.
type productionHostRepair struct {
	SchemaVersion string            `json:"schema_version"`
	Before        string            `json:"before"`
	Plan          string            `json:"plan"`
	PreparedBase  string            `json:"prepared_base"`
	ProductTree   string            `json:"product_tree"`
	SourceEpoch   int64             `json:"source_epoch"`
	SourceTry     int64             `json:"source_try"`
	Submission    driver.Submission `json:"submission"`
	FailedCheck   hostCheckResult   `json:"failed_check"`
}

type hostCheckFailure struct {
	result hostCheckResult
	err    error
}

func (e *hostCheckFailure) Error() string { return e.err.Error() }
func (e *hostCheckFailure) Unwrap() error { return e.err }

type hostRepairError struct {
	repair productionHostRepair
	err    error
}

func (e *hostRepairError) Error() string { return e.err.Error() }
func (e *hostRepairError) Unwrap() error { return e.err }

func hostRepairResult(err error) []byte {
	var repair *hostRepairError
	if errors.As(err, &repair) {
		return mustJSON(repair.repair)
	}
	return nil
}

// priorAttemptCoordinates finds the immediately preceding implementer
// attempt under before's own authority: the same epoch's previous try, or
// (when this is a fresh epoch's first try) the latest try of the highest
// prior epoch that actually ran a git.seal for this exact before. Shared by
// captureHostRepair and captureSubmissionRepair so both repair paths locate
// the identical predecessor.
func priorAttemptCoordinates(ctx context.Context, engine *engine, coordinates dispatchCoordinates, before string) (dispatchCoordinates, bool, error) {
	work := workIdentity(before, "git.seal")
	prior := coordinates
	prior.Try--
	if coordinates.Try == 1 {
		// Explicit operator retry starts a new epoch. Keep the last exact
		// failure from the previous epoch instead of forgetting its cause.
		snapshot, err := engineSnapshot(ctx, engine)
		if err != nil {
			return dispatchCoordinates{}, false, err
		}
		prior.Epoch, prior.Try = 0, 0
		for _, effect := range snapshot.Effects {
			if effect.Kind != "git.seal" {
				continue
			}
			ownedWork, epoch, retry, err := attemptCoordinates(effect.ID)
			if err == nil && ownedWork == work && epoch < coordinates.Epoch && (epoch > prior.Epoch || (epoch == prior.Epoch && retry > prior.Try)) {
				prior.Epoch, prior.Try = epoch, retry
			}
		}
		if prior.Epoch == 0 {
			return dispatchCoordinates{}, false, nil
		}
	}
	return prior, true, nil
}

// Capture only the immediately preceding retry under identical authority.
// Missing legacy payloads fail closed instead of silently repeating work.
func captureHostRepair(ctx context.Context, engine *engine, coordinates dispatchCoordinates, before string, plan *productionPlanBinding) (*productionHostRepair, error) {
	if coordinates.Responsibility != driver.ImplementerImplementation || (coordinates.Try <= 1 && coordinates.Epoch <= 1) {
		return nil, nil
	}
	work := workIdentity(before, "git.seal")
	prior, found, err := priorAttemptCoordinates(ctx, engine, coordinates, before)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	outer, outerErr := engine.journal.Effect(ctx, engine.manifest.value.RunID, journal.AttemptEffectID(work, prior.Epoch, prior.Try))
	if outerErr != nil && !journal.IsCode(outerErr, "EFFECT_NOT_FOUND") {
		return nil, runtimeFail("JOURNAL_READ_FAILED", outerErr)
	}
	if outerErr == nil && outer.State == journal.OperationalFailed && (outer.ErrorCode == "HOST_REPAIR_UNAVAILABLE" || outer.ErrorCode == "HOST_REPAIR_BINDING_FAILED") {
		return nil, runtimeFailSite(outer.ErrorCode, "Previous repair admission failed; inspect retained host-check evidence before retrying.", nil)
	}
	dispatchID, _ := dispatchEffectCandidates(work, prior.Epoch, prior.Try)
	effect, err := engine.journal.Effect(ctx, engine.manifest.value.RunID, dispatchID)
	if journal.IsCode(err, "EFFECT_NOT_FOUND") {
		return nil, nil
	}
	if err != nil {
		return nil, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State != journal.OperationalFailed || effect.ErrorCode != "HOST_CHECK_FAILED" {
		return nil, nil
	}
	var repair productionHostRepair
	if len(effect.Result) == 0 {
		return nil, runtimeFailSite("HOST_REPAIR_UNAVAILABLE", "Previous host-check failure has no retained repair context; inspect its journaled check before retrying.", nil)
	}
	if json.Unmarshal(effect.Result, &repair) != nil || !bytesEqualCanonicalJSON(effect.Result, repair) || plan == nil || repair.Before != before || repair.Plan != plan.OID || repair.SourceEpoch != prior.Epoch || repair.SourceTry != prior.Try {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", nil)
	}
	if err := validateHostRepair(repair, dispatchInvocationID(engine.manifest.value.RunID, prior), coordinates.Slice); err != nil {
		return nil, err
	}
	check := repair.FailedCheck
	parsed, err := baton.ParsePlan(plan.body)
	if err != nil {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", err)
	}
	contractDigest, ok := parsed.Contract(coordinates.Slice)
	if !ok || contractDigest != check.ContractDigest || parsed.Digest() != plan.Digest {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", nil)
	}
	stored, err := engine.journal.Effect(ctx, engine.manifest.value.RunID, check.EffectID)
	if err != nil {
		return nil, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if stored.Kind != "check.host" || stored.State != journal.Succeeded || !bytesEqualCanonicalJSON(stored.Result, check) {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", nil)
	}
	cp, err := engine.journal.LatestUnverifiedCheckpoint(ctx, engine.manifest.value.RunID, coordinates.Slice)
	if err != nil {
		return nil, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if cp == nil || cp.Release != engine.manifest.value.Release || cp.PlanOID != repair.Plan || cp.PlanDigest != plan.Digest || cp.ContractDigest != check.ContractDigest || cp.DispatchWork != workIdentity(work, "driver.dispatch") || cp.Epoch != prior.Epoch || cp.Try != prior.Try || cp.PreparedBase != repair.PreparedBase || cp.TreeDigest != repair.ProductTree {
		return nil, runtimeFailSite("HOST_REPAIR_UNAVAILABLE", "The failed candidate has no matching retained checkpoint; preserve and inspect the recorded candidate before retrying.", nil)
	}
	oid, err := gitx.ParseOID(engine.repository.ObjectFormat(), check.Candidate)
	if err != nil {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", err)
	}
	identity, err := engine.repository.ProductTreeIdentity(oid, engine.product)
	if err != nil || identity.ProductTree != repair.ProductTree {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", err)
	}
	tree, err := engine.repository.TreeOID(oid)
	if err != nil || tree.String() != cp.TreeOID {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", err)
	}
	parents, err := engine.repository.Parents(oid)
	if err != nil || len(parents) != 1 || parents[0].String() != repair.PreparedBase {
		return nil, runtimeFail("HOST_REPAIR_BINDING_FAILED", err)
	}
	return &repair, nil
}

func validateHostRepair(repair productionHostRepair, invocation, slice string) error {
	parts := strings.Split(invocation, "/")
	if len(parts) != 6 || repair.SourceEpoch < 1 || repair.SourceTry < 1 || parts[4] != strconv.FormatInt(repair.SourceEpoch, 10) || parts[5] != strconv.FormatInt(repair.SourceTry, 10) {
		return runtimeFail("HOST_REPAIR_BINDING_FAILED", nil)
	}
	check := repair.FailedCheck
	if repair.SchemaVersion != hostRepairVersion || !runtimeDigestPattern.MatchString(repair.Before) || !validGitObjectID(repair.Plan) || !validGitObjectID(repair.PreparedBase) || !runtimeDigestPattern.MatchString(repair.ProductTree) || repair.Submission.InvocationID != invocation || repair.Submission.Responsibility != driver.ImplementerImplementation || check.Slice != slice || !validGitObjectID(check.Candidate) || !runtimeDigestPattern.MatchString(check.ContractDigest) || check.Check == "" || (check.Outcome != baton.CheckOutcomeFail && check.Outcome != baton.CheckOutcomeTimeout && check.Outcome != baton.CheckOutcomeOverflow) || check.EffectID != hostCheckEffectID(hostCheckWork(slice, check.Candidate, check.ContractDigest, check.Check)) || len(check.Output) > hostCheckOutputBytes+1024 || baton.DigestBytes([]byte(check.Output)) != check.OutputDigest {
		return runtimeFail("HOST_REPAIR_BINDING_FAILED", nil)
	}
	if _, err := driver.EncodeSubmission(repair.Submission); err != nil {
		return runtimeFail("HOST_REPAIR_BINDING_FAILED", err)
	}
	return nil
}
