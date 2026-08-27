package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func (s *Service) Status(ctx context.Context, runID string) (RunStatus, error) {
	if s == nil || s.journal == nil || ctx == nil || !runtimeIdentityPattern.MatchString(runID) {
		return RunStatus{}, runtimeFail("INVALID_RUN", nil)
	}
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return RunStatus{}, runtimeFail("RUN_NOT_FOUND", err)
	}
	control, err := s.journal.ControlProjection(ctx, runID)
	if err != nil {
		return RunStatus{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	owner, ownerPresent, err := s.journal.CurrentOwner(ctx, runID)
	if err != nil {
		return RunStatus{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	// One clock read feeds every owner-liveness and recovery-guidance fact
	// in this Status, so RunStatus stays stable across a lease boundary
	// straddle instead of flapping SNAPSHOT_UNSTABLE.
	now := s.now().UTC()
	ownerActive := ownerPresent && owner.ExpiresAt.After(now)
	ownerExpired := ownerPresent && !ownerActive
	result := RunStatus{SchemaVersion: "sworn.run-status/v4", RunID: runID,
		State: "new", DesiredState: control.Desired, ControlGeneration: control.Generation,
		ManifestDigest: snapshot.Run.ManifestDigest, TargetRef: snapshot.Run.TargetRef,
		Effects: make([]EffectStatus, 0, len(snapshot.Effects))}
	manifest, proposals, loadErr := loadRunSnapshot(snapshot, runID)
	if loadErr != nil {
		return RunStatus{}, loadErr
	}
	if manifest.legacyVersion != "" {
		result.State = "migration_required"
		result.AuthorityState = "migration_required"
		return result, nil
	}
	result.Project = manifest.value.Authority.Project
	result.ExternalAuthorizer = manifest.value.Authority.ExternalAuthorizer
	delegation, delegationErr := currentCaptainDelegation(snapshot)
	if delegationErr != nil {
		return RunStatus{}, delegationErr
	}
	if delegation.Epoch > 0 {
		state := "revoked"
		if delegation.Active {
			state = "active"
		}
		result.CaptainDelegation = &CaptainDelegationView{Digest: delegation.Digest, Epoch: delegation.Epoch, State: state, Decisions: delegation.Decisions, ReplanSpent: delegation.ReplanSpent, ReplanBudget: delegation.Envelope.Limits.ReplanBudget}
	}
	authorityDigest, authorityErr := effectivePlanAuthority(manifest, snapshot)
	if authorityErr != nil {
		if IsCode(authorityErr, "AUTHORITY_CONFLICT") {
			result.State = "authority_conflict"
			result.AuthorityState = "authority_conflict"
			return result, nil
		}
		return RunStatus{}, authorityErr
	}
	result.AuthorityDigest = authorityDigest
	attentions, err := s.journal.Attentions(ctx, runID)
	if err != nil {
		return RunStatus{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	attentionWork, err := activeAttentionWork(attentions)
	if err != nil {
		return RunStatus{}, err
	}
	recoveryClaims, err := projectedRecoveryClaims(
		manifest,
		snapshot,
		attentionWork,
	)
	if err != nil {
		return RunStatus{}, err
	}
	active, uncertain := false, false
	exhausted := make(map[string]struct{})
	exhaustionRefusals := make(map[string]exhaustionRefusalFacts)
	attentionParked := false
	// Recovery guidance derives from the same snapshot, clock read, and
	// journal.RetryAdmissibleEffect predicate the control verbs evaluate.
	// The first current-epoch dispatch that admits retry wins; otherwise
	// an expired-but-present owner offers takeover; otherwise a claimed
	// dispatch inside its lease window offers resume with the lease reason.
	var recovery, recoveryResume *RecoveryAction
	retryReason := "The last dispatch of this work cannot be confirmed. " +
		"Retry it to start a fresh try."
	resumeReason := "A dispatch claim is still inside its lease window. " +
		"Resume the run after the lease expires and Sworn will recheck it."
	takeoverReason := "Take over the expired owner so Sworn can recheck " +
		"the run and continue."
	for _, attention := range attentionWork {
		if attention.State == journal.AttentionOpen {
			attentionParked = true
		}
	}
	derived := derivedWorks(snapshot)
	for _, effect := range snapshot.Effects {
		if effect.ID == "runtime.owner" || effect.Kind == "runtime.control" {
			continue
		}
		parts := strings.Split(effect.ID, "/")
		var work string
		if len(parts) == 4 {
			work = "sha256:" + parts[1]
		}
		_, isDerived := derived[work]
		result.Effects = append(result.Effects, EffectStatus{ID: effect.ID, Kind: effect.Kind,
			State: string(effect.State), ErrorCode: effect.ErrorCode, Derived: isDerived})
		if state, deliberate := recoveryClaims[effect.ID]; deliberate {
			active = active ||
				(state == journal.AttentionAnswered && ownerActive)
			continue
		}
		active = active || (effect.State == journal.Claimed && ownerActive)
		uncertain = uncertain || effect.State == journal.Uncertain ||
			(effect.State == journal.Claimed && !ownerActive)
		if effect.Kind == "driver.dispatch" {
			if recoveryWork, recoveryEpoch, _, coordErr :=
				attemptCoordinates(effect.ID); coordErr == nil {
				currentEpoch := control.RetryEpochs[recoveryWork]
				if currentEpoch == 0 {
					currentEpoch = 1
				}
				if recoveryEpoch == currentEpoch {
					if journal.RetryAdmissibleEffect(
						effect.State,
						effect.CurrentClaimExpiresAt,
						now,
						ownerActive,
					) {
						if recovery == nil {
							recovery = &RecoveryAction{
								Action:   string(journal.Retry),
								WorkID:   recoveryWork,
								Epoch:    recoveryEpoch,
								EffectID: effect.ID,
								Reason:   retryReason,
							}
						}
					} else if effect.State == journal.Claimed &&
						recoveryResume == nil {
						recoveryResume = &RecoveryAction{
							Action:   string(journal.Resume),
							WorkID:   recoveryWork,
							Epoch:    recoveryEpoch,
							EffectID: effect.ID,
							Reason:   resumeReason,
						}
					}
				}
			}
		}
		if effect.State == journal.OperationalFailed && strings.HasSuffix(effect.ID, "/t3") && !isDerived {
			if len(parts) == 4 {
				epoch, _ := strconv.ParseInt(strings.TrimPrefix(parts[2], "e"), 10, 64)
				current := control.RetryEpochs[work]
				if current == 0 {
					current = 1
				}
				if epoch == current {
					exhausted[work] = struct{}{}
					// The persisted effect's ErrorCode is the stable
					// runtime-wrapper code (e.g. CANDIDATE_SCOPE_FAILED);
					// the more specific refusal code (SLICE_OUTSIDE_SCOPE,
					// RESERVED_RECORD_ROOT_CHANGED) rides the effect's
					// journaled productionRefusalBinding result alongside
					// the named paths.
					if effect.ErrorCode == "CANDIDATE_SCOPE_FAILED" {
						if detail := scopeExhaustionDetail(effect.Result); detail != "" {
							exhaustionRefusals[work] = exhaustionRefusalFacts{
								code: effect.ErrorCode, detail: detail,
							}
						}
					}
				}
			}
		}
	}
	degradationCount := int64(len(degradationFallbacks(snapshot)))
	degradationBudgetExceeded := degradationCount > manifest.value.EffectiveDegradationBudget()
	// Economy and identical-failure guards (A1/A2) are evaluated over the
	// same journal history as every park condition, so a re-drive of a
	// journal that crossed re-parks with the same figures.
	economyCrossings := economyParkCrossings(snapshot, control)
	var economyPark *economyParkFacts
	if len(economyCrossings) != 0 {
		spentTurns, spentTokens, spentBytes, diagnosticCode, spentErr := s.economySpent(
			ctx,
			runID,
			economyCrossings[0],
		)
		if spentErr != nil {
			return RunStatus{}, spentErr
		}
		facts := economyParkFactsFor(
			economyCrossings[0],
			manifest.value.Limits,
			spentTurns,
			spentTokens,
			spentBytes,
			diagnosticCode,
		)
		economyPark = &facts
	}
	identicalCrossings := identicalFailureParkCrossings(
		manifest,
		snapshot,
		control,
		manifest.value.EffectiveIdenticalFailureParkAfter(),
	)
	var identicalPark *identicalFailureFacts
	if len(identicalCrossings) != 0 {
		identicalPark = &identicalCrossings[0]
	}
	// Raw exhaustion is fail-closed until Baton state can tell us whether the
	// exhausted work is still applicable. Recovery attention is independent.
	// The diagnostic code/detail stay empty here: they name a specific
	// scope-refused work, which is only known once exhaustion is matched
	// against an applicable work below.
	exhaustionApplies := len(exhausted) != 0
	var exhaustionCode, exhaustionDetail string
	parked := attentionParked || degradationBudgetExceeded ||
		economyPark != nil || identicalPark != nil || exhaustionApplies
	if len(snapshot.Events) != 0 {
		result.EventOffset = snapshot.Events[len(snapshot.Events)-1].Offset
	}
	repository, openErr := gitx.Open(manifest.value.Repository, s.gitExecutable)
	if openErr != nil {
		return result, nil
	}
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	state, stateErr := baton.ReadState(baton.UseGitRepository(repository),
		manifest.value.Release, inertness)
	statusEngine := &engine{
		manifest: manifest, repository: repository,
		git: baton.UseGitRepository(repository), inertness: inertness,
	}
	proposal, proposalFound, proposalInstalled, selectErr := selectPlanProposal(
		statusEngine, snapshot, proposals, state, stateErr)
	if selectErr != nil {
		return RunStatus{}, selectErr
	}
	humanAuthorityRequired := false
	if proposalFound && delegation.Epoch > 0 {
		humanAuthorityRequired, err = captainHumanAuthorityRequired(snapshot, proposal, delegation)
		if err != nil {
			return RunStatus{}, err
		}
	}
	var currentProposal *admittedPlanProposal
	if proposalFound {
		currentProposal = &proposal
	}
	authorityDigest, authorityErr = effectivePlanAuthority(
		manifest, snapshot, currentProposal)
	if authorityErr != nil {
		if IsCode(authorityErr, "AUTHORITY_CONFLICT") {
			result.State = "authority_conflict"
			result.AuthorityState = "authority_conflict"
			return result, nil
		}
		return RunStatus{}, authorityErr
	}
	result.AuthorityDigest = authorityDigest
	if proposalFound && authorityDigest == "" && (!delegation.Active || humanAuthorityRequired) {
		command, err := approvalCommandForProposal(manifest, proposal)
		if err != nil {
			return RunStatus{}, err
		}
		result.ApprovalOffer = &ApprovalOffer{
			SchemaVersion: ApprovalCommandVersion,
			Command:       command,
		}
	}
	proposalActivated := proposalActivationRecorded(
		proposal, proposalFound, proposalInstalled, state, stateErr,
		authorityDigest, snapshot,
	)
	result.AuthorityState = "awaiting_approval"
	for _, effect := range snapshot.Effects {
		if effect.Kind != "baton.install" {
			continue
		}
		switch effect.State {
		case journal.Pending, journal.Claimed:
			result.AuthorityState = "installing"
		case journal.Uncertain:
			result.AuthorityState = "reconciling_install"
		case journal.OperationalFailed:
			result.AuthorityState = "invalid_authority"
		}
	}
	if result.AuthorityState == "awaiting_approval" && authorityDigest != "" {
		switch {
		case proposalActivated:
			result.AuthorityState = "approved"
		case proposalAwaitsExactAuthority(
			proposal, proposalFound, state, stateErr, authorityDigest,
		):
			result.AuthorityState = "authority_conflict"
		case stateErr == nil:
			adopted, err := validateSavedPlanAdoption(
				statusEngine, state, authorityDigest)
			if err != nil {
				result.AuthorityState = "invalid_authority"
			} else if adopted {
				result.AuthorityState = "approved"
			}
		case proposalFound && proposal.plan.Digest() == authorityDigest:
			result.AuthorityState = "approved"
		default:
			result.AuthorityState = "authority_conflict"
		}
	}
	if proposalFound {
		result.PlanDigest = proposal.plan.Digest()
		if !proposalActivated {
			result.TargetHead = proposal.authority.TargetHead
			var exhaustionWork string
			exhaustionWork, exhaustionApplies = intersectingWork(
				exhausted, map[string]struct{}{
					proposalInstallWork(proposal): {},
				})
			exhaustionCode, exhaustionDetail = "", ""
			if facts, ok := exhaustionRefusals[exhaustionWork]; exhaustionApplies && ok {
				exhaustionCode, exhaustionDetail = facts.code, facts.detail
			}
			parked = attentionParked || degradationBudgetExceeded ||
				economyPark != nil || identicalPark != nil || exhaustionApplies
			parked = parked || humanAuthorityRequired
		}
	}
	switch {
	case uncertain:
		result.State = "uncertain"
	case control.Desired == "paused":
		if active {
			result.State = "pausing"
		} else {
			result.State = "paused"
		}
	case control.Desired == "cancelled":
		if active {
			result.State = "cancelling"
		} else {
			result.State = "cancelled"
		}
	default:
		if active {
			result.State = "running"
		} else if parked {
			result.State = "parked"
		} else if proposalFound && !proposalInstalled {
			result.State = "awaiting_approval"
		} else if ownerExpired {
			result.State = "takeover_required"
		} else {
			result.State = "running"
		}
	}
	if result.State == "parked" {
		// This early site runs before a later baton.ReadState is confirmed
		// to have succeeded, so it can never compute lane-scoped
		// PinnedWork; it only ever produces the legacy run-scoped shape
		// (B3). The final site below recomputes both Park and PinnedWork
		// from confirmed Baton state and is authoritative whenever it is
		// reached.
		result.Park = parkStatusFor(manifest, parkFacts{
			humanAuthorityRequired:    humanAuthorityRequired,
			attentionParked:           attentionParked,
			degradationBudgetExceeded: degradationBudgetExceeded,
			degradationCount:          degradationCount,
			economy:                   economyPark,
			identicalFailure:          identicalPark,
			exhaustionApplies:         exhaustionApplies,
			exhaustionCode:            exhaustionCode,
			exhaustionDetail:          exhaustionDetail,
		})
	} else {
		result.Park = nil
	}
	if recovery == nil && ownerExpired {
		recovery = &RecoveryAction{
			Action: string(journal.Takeover),
			Reason: takeoverReason,
		}
	}
	if recovery == nil {
		recovery = recoveryResume
	}
	if recovery == nil {
		// Every uncertain run names an admissible verb: resume is admitted
		// by the control gate whenever the desired state is running, so
		// the board never names a verb ApplyControl will refuse.
		recovery = &RecoveryAction{
			Action: string(journal.Resume),
			Reason: "Resume the run so Sworn can recheck the " +
				"unresolved work.",
		}
	}
	if result.State == "uncertain" {
		result.Recovery = recovery
	}
	if stateErr != nil {
		return result, nil
	}
	result.TargetHead, result.ReleaseHead = state.Refs.Target.Head, state.Refs.Release.Head
	result.Outcome = state.Assembly.Outcome
	var selected *admittedPlanProposal
	if proposalFound {
		selected = &proposal
	}
	// Lane-scoped drain (A1): the confirmed candidate lanes replace the flat
	// exhaustedWorkApplies applicability set. A lane pins when any of its
	// candidate works currently crosses an economy, identical-failure, or
	// exhaustion guard; the run parks on that basis only when at least one
	// candidate lane exists and every one of them is pinned, so a healthy
	// lane's live candidate work is never masked by another lane's park.
	lanes := readyLaneCandidates(manifest, selected, proposalActivated, state, snapshot)
	economyByOwner := make(map[string]economyParkFacts, len(economyCrossings))
	for _, crossing := range economyCrossings {
		owner := ownerWorkForDispatch(snapshot, crossing.work)
		if _, exists := economyByOwner[owner]; exists {
			continue
		}
		spentTurns, spentTokens, spentBytes, diagnosticCode, spentErr := s.economySpent(
			ctx, runID, crossing,
		)
		if spentErr != nil {
			return RunStatus{}, spentErr
		}
		economyByOwner[owner] = economyParkFactsFor(
			crossing, manifest.value.Limits, spentTurns, spentTokens, spentBytes, diagnosticCode,
		)
	}
	identicalByOwner := make(map[string]identicalFailureFacts, len(identicalCrossings))
	for _, crossing := range identicalCrossings {
		owner := ownerWorkForDispatch(snapshot, crossing.work)
		if _, exists := identicalByOwner[owner]; !exists {
			identicalByOwner[owner] = crossing
		}
	}
	pinnedWork, laneParks, allLanesPinned := resolveLanePins(
		lanes, exhausted, exhaustionRefusals, economyByOwner, identicalByOwner,
	)
	parked = humanAuthorityRequired || attentionParked ||
		degradationBudgetExceeded || (len(lanes) != 0 && allLanesPinned)
	if control.Desired == "running" && !uncertain && !parked {
		switch {
		case proposalFound && !proposalActivated:
			result.State = "awaiting_approval"
		case state.Assembly.Outcome == "merged":
			result.State = "complete"
		case active:
			result.State = "running"
		case ownerExpired:
			result.State = "takeover_required"
		default:
			result.State = "running"
		}
	}
	// PinnedWork is evidence about the affected work, visible beside the
	// still-running lanes (A2) regardless of whether the run as a whole
	// reads parked: a work can be pinned while every other lane keeps
	// dispatching. Park stays governed by the State == "parked" invariant
	// (B2): it is the run-level summary, nil whenever the run is not
	// parked, never a partial one.
	result.PinnedWork = pinnedWork
	if result.State == "parked" {
		switch {
		case humanAuthorityRequired || attentionParked || degradationBudgetExceeded:
			result.Park = parkStatusFor(manifest, parkFacts{
				humanAuthorityRequired:    humanAuthorityRequired,
				attentionParked:           attentionParked,
				degradationBudgetExceeded: degradationBudgetExceeded,
				degradationCount:          degradationCount,
			})
		case len(laneParks) != 0:
			result.Park = parkStatusFor(manifest, laneParks[0].facts)
			result.Park.Work = laneParks[0].work
		}
	} else {
		result.Park = nil
	}
	return result, nil
}

func projectedRecoveryClaims(
	manifest admittedManifest,
	snapshot journal.Snapshot,
	activeWork map[string]journal.AttentionProjection,
) (map[string]journal.AttentionState, error) {
	result := make(map[string]journal.AttentionState)
	if len(activeWork) == 0 {
		return result, nil
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	direct := make(map[string][]journal.Effect)
	for _, effect := range snapshot.Effects {
		if _, duplicate := effects[effect.ID]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effects[effect.ID] = effect
		if effect.Kind != "driver.dispatch" ||
			(effect.State != journal.Claimed &&
				effect.State != journal.Succeeded) {
			continue
		}
		work, err := attemptWorkIdentity(effect.ID)
		if err != nil {
			continue
		}
		if _, found := activeWork[work]; found {
			direct[work] = append(direct[work], effect)
		}
	}
	active, err := selectActiveImplementationCycles(
		manifest,
		snapshot.Commands,
		effects,
		activeWork,
	)
	if err != nil {
		return nil, err
	}
	selected := make(map[string]activeImplementationCycle, len(active))
	for _, cycle := range active {
		if _, duplicate := selected[cycle.cycle.DispatchWork]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		selected[cycle.cycle.DispatchWork] = cycle
	}
	for work, attention := range activeWork {
		cycle, implementation := selected[work]
		dispatches := direct[work]
		if !implementation {
			if len(dispatches) != 1 {
				return nil, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			command, found := commands[dispatches[0].ReplayKey]
			if !found {
				return nil, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if err := validateAttentionDispatchBinding(
				manifest,
				command,
				dispatches[0],
				attention,
				nil,
			); err != nil {
				return nil, err
			}
			result[dispatches[0].ID] = attention.State
			continue
		}
		if cycle.attention != attention {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		result[cycle.outer.ID] = cycle.attention.State
		if cycle.dispatch.State == journal.OperationalFailed {
			if len(dispatches) != 0 {
				return nil, runtimeFail("CORRUPT_JOURNAL", nil)
			}
		} else if len(dispatches) != 1 ||
			dispatches[0].ID != cycle.dispatch.ID {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if cycle.dispatch.State == journal.Claimed {
			result[cycle.dispatch.ID] = cycle.attention.State
		}
	}
	return result, nil
}

func validateAttentionDispatchBinding(
	manifest admittedManifest,
	command journal.Command,
	effect journal.Effect,
	attention journal.AttentionProjection,
	implementation *implementationCycle,
) error {
	if (attention.State != journal.AttentionOpen ||
		attention.Generation != 1) &&
		(attention.State != journal.AttentionAnswered ||
			attention.Generation != 2) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var submission *driver.Submission
	var dispatch driverRecoveryCommand
	var err error
	switch effect.State {
	case journal.Succeeded:
		var cached driver.Submission
		cached, dispatch, err = validateSucceededDriverResult(
			manifest,
			command,
			effect,
		)
		submission = &cached
		if attention.State != journal.AttentionAnswered {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	case journal.Claimed, journal.OperationalFailed:
		dispatch, err = validateDriverRecoveryCommand(
			manifest,
			command,
			effect,
		)
	default:
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err != nil {
		return err
	}
	work, epoch, try, err := attemptCoordinates(effect.ID)
	if err != nil ||
		attention.Attention.Recovery.ProgressID != work {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	before := ""
	var prepared preparedDriverDispatch
	var coordinates dispatchCoordinates
	if dispatch.production != nil {
		context := dispatch.production.Context
		if context.Epoch != epoch ||
			context.Try != try {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if implementation != nil {
			if err := validateImplementationDispatchBinding(
				*implementation,
				effect,
				dispatch,
			); err != nil {
				return err
			}
		} else {
			expectedWork := driverWorkIdentity(
				manifest.digest,
				context.Slice,
				context.Responsibility,
				context.Attempt,
				context.Before,
			)
			if work != expectedWork {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
		}
		prepared.productionContext = &context
		coordinates = dispatchCoordinates{
			Slice:          context.Slice,
			Responsibility: context.Responsibility,
			BatonAttempt:   context.Attempt,
			Epoch:          context.Epoch,
			Try:            context.Try,
		}
		before = context.Before
	} else {
		var matched *ScriptedAttempt
		for index := range manifest.value.Scripts {
			script := &manifest.value.Scripts[index]
			if script.Epoch != epoch ||
				script.Try != try ||
				script.Behavior != dispatch.fake.Behavior ||
				script.Submission != dispatch.fake.Submission {
				continue
			}
			if submission != nil &&
				(script.Responsibility != submission.Responsibility ||
					invocationID(manifest.value.RunID, *script) !=
						submission.InvocationID) {
				continue
			}
			if implementation != nil &&
				(script.Slice != implementation.Slice ||
					script.Responsibility !=
						driver.ImplementerImplementation) {
				continue
			}
			if implementation == nil {
				lane := script.Slice
				if lane == "" {
					lane = "release"
				}
				if lane != attention.Attention.Recovery.LaneID {
					continue
				}
			}
			if matched != nil {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			matched = script
		}
		if matched == nil || implementation == nil {
			// Legacy fake dispatches do not persist their unhashed before
			// authority. Their exact work and lane remain enforceable.
			lane := ""
			if matched != nil {
				lane = matched.Slice
				if lane == "" {
					lane = "release"
				}
			}
			if matched == nil ||
				attention.Attention.Recovery.LaneID != lane ||
				matched.Epoch != epoch ||
				matched.Try != try {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			return nil
		}
		if err := validateImplementationDispatchBinding(
			*implementation,
			effect,
			dispatch,
		); err != nil {
			return err
		}
		before = implementation.Before
		prepared.fake = true
		coordinates = dispatchCoordinates{
			Slice:          matched.Slice,
			Responsibility: matched.Responsibility,
			BatonAttempt:   matched.BatonAttempt,
			Epoch:          epoch,
			Try:            try,
		}
	}
	cycle, err := turnRecoveryCycleForDispatch(
		manifest,
		prepared,
		coordinates,
		work,
		before,
	)
	if err != nil ||
		attention.Attention.Recovery.CycleID != cycle.binding.CycleID ||
		attention.Attention.Recovery.LaneID != cycle.binding.LaneID {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	return nil
}

// laneCandidates is the currently-applicable work-identity set for one
// candidate lane: a track (keyed by track ID) or the release pseudo-lane
// (keyed by "release", carrying the planner-proposal and assembly works).
// Only a lane with at least one candidate work is returned by
// readyLaneCandidates; a track with no ready actionable slice contributes no
// lane, exactly as it contributes nothing to exhaustedWorkApplies's flat set
// today.
type laneCandidates struct {
	lane  string
	works map[string]struct{}
}

// readyLaneCandidates derives the currently-applicable work-identity set for
// every candidate lane (A1), generalizing the single flat set
// exhaustedWorkApplies used to derive: the pre-install branch stays a single
// release-lane candidate exactly as before (a pending plan proposal
// supersedes every track's current work, so nothing else is a candidate
// lane while it awaits install), and the ready-track branch now keeps each
// track's candidate works, including its folded baton.append_receipt scan
// (F4), in that track's own lane instead of one shared set. The release
// pseudo-lane carries the planner-proposal work whenever plannerNeeded,
// independent of whether any track lane is ready (driveLoop dispatches the
// planner proposal after its fan-out even when track lanes were ready), and
// carries the assembly work only when no track lane is ready and no planner
// is needed (driveLoop's own admission order for those branches).
func readyLaneCandidates(
	manifest admittedManifest,
	proposal *admittedPlanProposal,
	proposalInstalled bool,
	state baton.State,
	snapshot journal.Snapshot,
) []laneCandidates {
	if proposal != nil && !proposalInstalled {
		work := proposalInstallWork(*proposal)
		if work == "" {
			return nil
		}
		return []laneCandidates{{lane: "release", works: map[string]struct{}{work: {}}}}
	}
	var lanes []laneCandidates
	anyTrackReady := false
	for _, track := range state.Tracks {
		works := make(map[string]struct{})
		add := func(work string) {
			if work != "" {
				works[work] = struct{}{}
			}
		}
		ready := false
		for _, slice := range track.Slices {
			if slice.Status != "ready" || slice.NextRole == "none" ||
				slice.NextRole == "merge" || slice.NextRole == "planner" {
				continue
			}
			ready = true
			before := sliceFingerprint(state, slice.Location.Slice.ID)
			switch {
			case slice.NextRole == "implementer" && slice.Stage == "design":
				add(workIdentity(
					trackBaseBefore(state, slice),
					"git.prepare_track_base",
				))
				add(driverWorkIdentity(manifest.digest, slice.Location.Slice.ID,
					driver.ImplementerDesign, slice.Attempt, before))
			case slice.NextRole == "captain":
				add(driverWorkIdentity(manifest.digest, slice.Location.Slice.ID,
					driver.CaptainReview, slice.Attempt, before))
			case slice.NextRole == "implementer" && slice.Stage == "implement":
				add(workIdentity(
					trackBaseBefore(state, slice),
					"git.prepare_track_base",
				))
				add(workIdentity(before, "git.seal"))
			case slice.NextRole == "verifier":
				add(driverWorkIdentity(manifest.digest, slice.Location.Slice.ID,
					driver.WorkVerification, slice.Attempt, before))
			}
			break
		}
		if !ready {
			continue
		}
		anyTrackReady = true
		for _, command := range snapshot.Commands {
			if command.Kind != "baton.append_receipt" {
				continue
			}
			persisted, err := parseActionCommand(command.Payload)
			if err != nil {
				continue
			}
			var input baton.AppendReceiptInput
			if parseCanonicalActionInput(
				persisted.Input, &input) != nil || input.Slice == "" {
				continue
			}
			owning, ok := state.Slice(input.Slice)
			if !ok || owning.Location.Track.ID != track.ID {
				continue
			}
			before := sliceFingerprint(state, input.Slice)
			if before != "" {
				add(workIdentity(
					before, "append", input.Role, input.Result, input.Candidate))
			}
		}
		lanes = append(lanes, laneCandidates{lane: track.ID, works: works})
	}
	plannerNeeded := false
	for _, slice := range state.Slices {
		plannerNeeded = plannerNeeded || slice.NextRole == "planner"
	}
	plannerNeeded = plannerNeeded || state.Assembly.NextRole == "planner"
	if plannerNeeded {
		authority := planProposalAuthority{
			Release:     manifest.value.Release,
			PriorPlan:   state.Plan.OID,
			ReleaseRef:  state.Refs.Release.Ref,
			ReleaseHead: state.Refs.Release.Head,
			TargetRef:   state.Refs.Target.Ref,
			TargetHead:  state.Refs.Target.Head,
		}
		before := plannerAuthorityBefore(authority)
		work := driverWorkIdentity(manifest.digest, "", driver.PlannerProposal,
			state.Plan.Metadata.Revision+1, before)
		if work != "" {
			lanes = append(lanes, laneCandidates{
				lane: "release", works: map[string]struct{}{work: {}},
			})
		}
		return lanes
	}
	if anyTrackReady {
		return lanes
	}
	works := make(map[string]struct{})
	add := func(work string) {
		if work != "" {
			works[work] = struct{}{}
		}
	}
	switch {
	case state.Assembly.NextRole == "merge" && state.Assembly.Outcome != "pass":
		before := workIdentity(state.Plan.OID, state.Refs.Release.Head,
			state.Refs.Target.Head, state.Assembly.Outcome, state.Assembly.InputPins)
		add(workIdentity(before, "prepare"))
	case state.Assembly.NextRole == "verifier" && state.Assembly.Candidate != nil &&
		state.Assembly.Candidate.Receipt.Candidate != nil:
		candidate := *state.Assembly.Candidate.Receipt.Candidate
		before := workIdentity(
			state.Plan.OID, state.Refs.Release.Head, state.Refs.Target.Head, candidate)
		add(driverWorkIdentity(manifest.digest, "", driver.AssemblyVerification,
			state.Plan.Metadata.Revision, before))
		add(workIdentity(before, "assembly_verdict"))
	case state.Assembly.NextRole == "merge" && state.Assembly.Outcome == "pass" &&
		state.Assembly.Pass != nil:
		before := workIdentity(state.Plan.OID, state.Refs.Release.Head,
			state.Refs.Target.Head, state.Assembly.Pass.OID)
		add(workIdentity(before, "merge"))
	}
	if len(works) != 0 {
		lanes = append(lanes, laneCandidates{lane: "release", works: works})
	}
	return lanes
}

// lanePinFacts carries the fully-featured park facts for one pinned lane, in
// the shape parkStatusFor already knows how to render, plus the exact work
// identity those facts describe.
type lanePinFacts struct {
	work  string
	facts parkFacts
}

// resolveLanePins decides, per candidate lane, whether any of its works
// currently crosses an economy, identical-failure, or exhaustion guard
// (B1's precedence: economy first, then identical-failure, then
// exhaustion, matching parkStatusFor's own cause precedence), and returns
// the stamped PinnedWork facts for every pinned lane (A2), the same facts
// in parkStatusFor's richer shape for the single-cause Park fallback (B3),
// and whether every candidate lane is pinned.
func resolveLanePins(
	lanes []laneCandidates,
	exhausted map[string]struct{},
	exhaustionRefusals map[string]exhaustionRefusalFacts,
	economyByOwner map[string]economyParkFacts,
	identicalByOwner map[string]identicalFailureFacts,
) ([]PinnedWork, []lanePinFacts, bool) {
	var pinnedWork []PinnedWork
	var laneParks []lanePinFacts
	allPinned := true
	for _, lane := range lanes {
		pinned := false
		for work := range lane.works {
			facts, ok := economyByOwner[work]
			if !ok {
				continue
			}
			pinnedWork = append(pinnedWork, PinnedWork{
				WorkID: work, Lane: lane.lane, Cause: facts.cause,
				Code:   economyCrossingCode(facts.cause),
				Detail: economySpentDetail(facts),
			})
			laneParks = append(laneParks, lanePinFacts{
				work: work, facts: parkFacts{economy: &facts},
			})
			pinned = true
			break
		}
		if !pinned {
			for work := range lane.works {
				facts, ok := identicalByOwner[work]
				if !ok {
					continue
				}
				pinnedWork = append(pinnedWork, PinnedWork{
					WorkID: work, Lane: lane.lane, Cause: ParkCauseIdenticalFailure,
					Code: facts.code, Detail: facts.detail,
				})
				laneParks = append(laneParks, lanePinFacts{
					work: work, facts: parkFacts{identicalFailure: &facts},
				})
				pinned = true
				break
			}
		}
		if !pinned {
			if work, ok := intersectingWork(exhausted, lane.works); ok {
				refusal := exhaustionRefusals[work]
				pinnedWork = append(pinnedWork, PinnedWork{
					WorkID: work, Lane: lane.lane, Cause: ParkCauseExhaustion,
					Code: refusal.code, Detail: refusal.detail,
				})
				laneParks = append(laneParks, lanePinFacts{
					work: work, facts: parkFacts{
						exhaustionApplies: true,
						exhaustionCode:    refusal.code,
						exhaustionDetail:  refusal.detail,
					},
				})
				pinned = true
			}
		}
		if !pinned {
			allPinned = false
		}
	}
	return pinnedWork, laneParks, allPinned
}

// economyCrossingCode names the stable error code an economy PinnedWork
// entry carries, matching the top-level driver.dispatch failure code the
// crossing was proved from.
func economyCrossingCode(cause string) string {
	switch cause {
	case ParkCauseEconomyTurns:
		return "ECONOMY_TURN_BUDGET_EXCEEDED"
	case ParkCauseEconomyOutputTokens, ParkCauseEconomyOutputBytes:
		return "ECONOMY_OUTPUT_BUDGET_EXCEEDED"
	default:
		return ""
	}
}

// economySpentDetail renders the bounded, honest spent-versus-budget fact an
// economy PinnedWork entry carries. The figures are non-negative integers,
// always representable within validParkDetail's bound.
func economySpentDetail(facts economyParkFacts) string {
	return "spent " + strconv.FormatInt(facts.spent, 10) +
		" of " + strconv.FormatInt(facts.budget, 10)
}

// intersectingWork returns one work present in both sets and whether any
// intersection exists. Map iteration order is unspecified, so when more than
// one work intersects, which one is named is unspecified too; every call
// site in this package targets a single-work exhaustion.
func intersectingWork(left, right map[string]struct{}) (string, bool) {
	for work := range left {
		if _, ok := right[work]; ok {
			return work, true
		}
	}
	return "", false
}

// exhaustionRefusalFacts carries the diagnostic facts a scope-refused
// exhaustion park names: the stable runtime error code the exhausting
// effect failed with, and a bounded, validated detail rendering the
// refusal's specific code and named paths.
type exhaustionRefusalFacts struct {
	code   string
	detail string
}

// scopeExhaustionDetail renders a scope refusal's specific code
// (SLICE_OUTSIDE_SCOPE, RESERVED_RECORD_ROOT_CHANGED) and its named paths as
// one park detail string, truncating trailing paths rather than emitting a
// detail validParkDetail would reject. Empty is honest absence.
func scopeExhaustionDetail(result []byte) string {
	if len(result) == 0 {
		return ""
	}
	var refusal productionRefusalBinding
	if err := json.Unmarshal(result, &refusal); err != nil ||
		refusal.Code == "" || len(refusal.Paths) == 0 {
		return ""
	}
	paths := refusal.Paths
	detail := refusal.Code + ": " + strings.Join(paths, ", ")
	for len(paths) > 0 && !validParkDetail(detail) {
		paths = paths[:len(paths)-1]
		detail = refusal.Code + ": " + strings.Join(paths, ", ")
	}
	if !validParkDetail(detail) {
		return ""
	}
	return detail
}

// parkFacts carries every reducer-computed park condition into the park
// status naming, in precedence order.
type parkFacts struct {
	humanAuthorityRequired    bool
	attentionParked           bool
	degradationBudgetExceeded bool
	degradationCount          int64
	economy                   *economyParkFacts
	identicalFailure          *identicalFailureFacts
	exhaustionApplies         bool
	exhaustionCode            string
	exhaustionDetail          string
}

// parkStatusFor names the park cause with the same precedence the final park
// computation uses: human authority, attention, degradation, economy,
// identical failure, exhaustion. A degradation park carries the gated
// fallback count, the effective budget, and the manifest knob that unblocks
// it; an economy park carries spent-versus-budget and its knob; an
// identical-failure park carries the run length, threshold, shared failure
// code, and its durable detail. An exhaustion park whose tries died on a
// scope refusal carries the refusal's stable error code and a bounded
// detail naming its specific code and paths, in the identical-failure
// pattern; a plain exhaustion carries neither.
func parkStatusFor(
	manifest admittedManifest,
	facts parkFacts,
) *ParkStatus {
	status := &ParkStatus{}
	switch {
	case facts.humanAuthorityRequired:
		status.Cause = ParkCauseHumanAuthority
	case facts.attentionParked:
		status.Cause = ParkCauseAttention
	case facts.degradationBudgetExceeded:
		status.Cause = ParkCauseDegradation
		status.FallbackCount = facts.degradationCount
		status.Budget = manifest.value.EffectiveDegradationBudget()
		status.UnblockKnob = DegradationUnblockKnob
	case facts.economy != nil:
		status.Cause = facts.economy.cause
		status.Spent = facts.economy.spent
		status.Budget = facts.economy.budget
		status.UnblockKnob = facts.economy.knob
	case facts.identicalFailure != nil:
		status.Cause = ParkCauseIdenticalFailure
		status.Consecutive = facts.identicalFailure.consecutive
		status.Threshold = facts.identicalFailure.threshold
		status.FailureCode = facts.identicalFailure.code
		status.FailureDetail = facts.identicalFailure.detail
		status.UnblockKnob = IdenticalFailureUnblockKnob
	case facts.exhaustionApplies:
		status.Cause = ParkCauseExhaustion
		status.FailureCode = facts.exhaustionCode
		status.FailureDetail = facts.exhaustionDetail
	}
	return status
}
