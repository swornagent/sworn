package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type currentDriverAuthority struct {
	beforeDigest string
}

type implementationDispatchAuthority struct {
	cycle implementationCycle
	outer journal.Effect
}

type activeImplementationCycle struct {
	command   journal.Command
	cycle     implementationCycle
	outer     journal.Effect
	dispatch  journal.Effect
	attention journal.AttentionProjection
}

func validateImplementationCyclePlanAuthority(
	state baton.State,
	cycle implementationCycle,
) error {
	if state.Release != cycle.Release ||
		state.Refs.Target.Ref == "" {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("implementation cycle release authority is absent"),
		)
	}
	metadata, ok := planMetadataForOID(state, cycle.Plan)
	if !ok ||
		metadata.Release != cycle.Release ||
		metadata.TargetRef != state.Refs.Target.Ref {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("implementation cycle plan authority is absent"),
		)
	}
	expectedTrack := ""
	for _, track := range metadata.Tracks {
		for _, slice := range track.Slices {
			if slice.ID != cycle.Slice {
				continue
			}
			if expectedTrack != "" {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			expectedTrack = track.ID
		}
	}
	if expectedTrack == "" ||
		expectedTrack != cycle.Track ||
		cycle.TrackRef !=
			"refs/heads/track/"+cycle.Release+"/"+expectedTrack {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("implementation cycle track authority is inconsistent"),
		)
	}

	bindsFound := false
	if historical, found := state.HistoryForSlice(cycle.Slice); found {
		if historical.Track != expectedTrack ||
			historical.Ref != cycle.TrackRef {
			return runtimeFail(
				"CORRUPT_JOURNAL",
				errors.New("historical implementation track authority changed"),
			)
		}
		for _, entry := range historical.History.Entries {
			if entry.OID == cycle.Binds {
				if bindsFound {
					return runtimeFail(
						"AMBIGUOUS_ACTION_HISTORY",
						nil,
					)
				}
				bindsFound = true
			}
		}
	}
	if current, found := state.Slice(cycle.Slice); found {
		if current.Location.Track.ID != expectedTrack {
			return runtimeFail(
				"CORRUPT_JOURNAL",
				errors.New("current implementation track authority changed"),
			)
		}
		if current.CurrentReceipt != nil &&
			current.CurrentReceipt.OID == cycle.Binds {
			bindsFound = true
			if state.Plan.OID == cycle.Plan &&
				sliceFingerprintAtAuthority(
					state,
					cycle.Slice,
					cycle.TargetHead,
					cycle.TrackHead,
				) != cycle.Before {
				return runtimeFail(
					"CORRUPT_JOURNAL",
					errors.New("current implementation before-state changed"),
				)
			}
		}
	}
	if !bindsFound {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("implementation binding is absent from Baton history"),
		)
	}
	return nil
}

func validateImplementationDispatchProof(
	engine *engine,
	snapshot journal.Snapshot,
	cycle implementationCycle,
	record sealedRecord,
) error {
	if engine == nil ||
		snapshot.Run.ID != engine.manifest.value.RunID ||
		!sealedRecordMatchesCycle(record, cycle) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}

	var command journal.Command
	commandCount := 0
	for _, candidate := range snapshot.Commands {
		if candidate.ReplayKey != cycle.DispatchEffect {
			continue
		}
		command = candidate
		commandCount++
	}
	var effect journal.Effect
	effectCount := 0
	for _, candidate := range snapshot.Effects {
		if candidate.ID != cycle.DispatchEffect {
			continue
		}
		effect = candidate
		effectCount++
	}
	if commandCount != 1 ||
		effectCount != 1 ||
		effect.State != journal.Succeeded ||
		effect.ID != cycle.DispatchEffect ||
		effect.ReplayKey != cycle.DispatchEffect ||
		effect.Kind != "driver.dispatch" ||
		effect.BeforeDigest != sha256Digest([]byte(cycle.Before)) ||
		effect.ResultDigest != sha256Digest(effect.Result) ||
		effect.CurrentClaim != "" ||
		effect.ErrorCode != "" {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	submission, dispatch, err := validateSucceededDriverResult(
		engine.manifest,
		command,
		effect,
	)
	if err != nil {
		return err
	}
	if err := validateImplementationDispatchBinding(
		cycle,
		effect,
		dispatch,
	); err != nil {
		return err
	}

	if submission.Responsibility != driver.ImplementerImplementation {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if dispatch.production == nil {
		var script ScriptedAttempt
		scriptCount := 0
		for _, candidate := range engine.manifest.value.Scripts {
			if candidate.Slice != cycle.Slice ||
				candidate.Responsibility != driver.ImplementerImplementation ||
				invocationID(engine.manifest.value.RunID, candidate) !=
					submission.InvocationID {
				continue
			}
			script = candidate
			scriptCount++
		}
		if scriptCount != 1 || script.Behavior != "submit" {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		scriptSubmission, err := base64.StdEncoding.Strict().DecodeString(
			script.Submission,
		)
		if err != nil ||
			!bytes.Equal(scriptSubmission, effect.Result) ||
			!bytes.Equal(command.Payload, mustJSON(fakeScript{
				SchemaVersion: "sworn.fake-script/v1",
				Behavior:      script.Behavior,
				Submission:    script.Submission,
			})) {
			return runtimeFail("CORRUPT_JOURNAL", err)
		}
	}

	checks, err := exactBytes(submission.Checks)
	if err != nil ||
		record.Receipt.Summary != submission.Summary ||
		!bytes.Equal(record.Receipt.Detail, []byte(submission.Detail)) ||
		!bytes.Equal(record.Receipt.CheckResults, checks) {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	return nil
}

func validateImplementationDispatchBinding(
	cycle implementationCycle,
	effect journal.Effect,
	dispatch driverRecoveryCommand,
) error {
	work, _, _, err := attemptCoordinates(effect.ID)
	if err != nil ||
		work != cycle.DispatchWork ||
		effect.ID != cycle.DispatchEffect {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	if dispatch.production != nil {
		context := dispatch.production.Context
		if context.Slice != cycle.Slice ||
			context.Responsibility != driver.ImplementerImplementation ||
			context.Before != cycle.Before ||
			context.Plan == nil || context.Plan.OID != cycle.Plan ||
			context.Receipt == nil || context.Receipt.OID != cycle.Binds ||
			context.Authority.ReleaseHead != cycle.ReleaseHead ||
			context.Authority.TargetHead != cycle.TargetHead ||
			context.Authority.TrackRef != cycle.TrackRef ||
			context.Authority.TrackHead != cycle.TrackHead {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	return nil
}

func incompleteProductionImplementationDispatch(
	manifest admittedManifest,
	commands map[string]journal.Command,
	effects map[string]journal.Effect,
	cycle implementationCycle,
) (bool, error) {
	if !manifest.value.production() {
		return false, nil
	}
	command, commandOK := commands[cycle.DispatchEffect]
	effect, effectOK := effects[cycle.DispatchEffect]
	if !commandOK || !effectOK {
		return false, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if effect.State == journal.Succeeded {
		return false, nil
	}
	switch effect.State {
	case journal.Claimed, journal.Uncertain, journal.OperationalFailed:
	default:
		return false, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	dispatch, err := validateDriverRecoveryCommand(
		manifest,
		command,
		effect,
	)
	if err != nil || dispatch.production == nil {
		return false, runtimeFail("CORRUPT_JOURNAL", err)
	}
	if err := validateImplementationDispatchBinding(
		cycle,
		effect,
		dispatch,
	); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) validateImplementationDispatchProof(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	cycle implementationCycle,
	record sealedRecord,
) error {
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	return validateImplementationDispatchProof(
		engine,
		snapshot,
		cycle,
		record,
	)
}

func validateImplementationCycleEnvelope(
	manifest admittedManifest,
	command journal.Command,
	outer journal.Effect,
) (implementationCycle, error) {
	if err := validateRecoveryCommand(command, outer, true); err != nil {
		return implementationCycle{}, err
	}
	var cycle implementationCycle
	if json.Unmarshal(command.Payload, &cycle) != nil ||
		!bytesEqualCanonicalJSON(command.Payload, cycle) ||
		cycle.Release != manifest.value.Release ||
		gitx.ValidateIdentity(cycle.GitIdentity) != nil ||
		cycle.Slice == "" ||
		cycle.Binds == "" ||
		!runtimeDigestPattern.MatchString(cycle.Before) ||
		cycle.Plan == "" ||
		cycle.ReleaseHead == "" ||
		cycle.TargetHead == "" ||
		!runtimeIdentityPattern.MatchString(cycle.Track) ||
		cycle.TrackRef !=
			"refs/heads/track/"+manifest.value.Release+"/"+cycle.Track ||
		cycle.TrackHead == "" ||
		(cycle.RefreshFrom != "" && cycle.RefreshFrom != cycle.Binds) {
		return implementationCycle{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	outerWork := workIdentity(cycle.Before, "git.seal")
	attemptWork, err := attemptWorkIdentity(outer.ID)
	if err != nil ||
		attemptWork != outerWork ||
		outer.BeforeDigest != outerWork {
		return implementationCycle{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	_, epoch, try, err := attemptCoordinates(outer.ID)
	if err != nil {
		return implementationCycle{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	stableChildren :=
		cycle.DispatchWork ==
			workIdentity(outerWork, "driver.dispatch") &&
			cycle.DispatchEffect ==
				journal.AttemptEffectID(
					cycle.DispatchWork,
					epoch,
					try,
				) &&
			cycle.PreparedWork ==
				workIdentity(outerWork, "git.seal.prepared") &&
			cycle.PreparedEffect ==
				journal.AttemptEffectID(
					cycle.PreparedWork,
					epoch,
					try,
				)
	legacyChildren :=
		cycle.DispatchWork ==
			workIdentity(outer.ID, "driver.dispatch") &&
			cycle.DispatchEffect ==
				journal.AttemptEffectID(cycle.DispatchWork, 1, 1) &&
			cycle.PreparedWork ==
				workIdentity(outer.ID, "git.seal.prepared") &&
			cycle.PreparedEffect ==
				journal.AttemptEffectID(cycle.PreparedWork, 1, 1)
	if !stableChildren && !legacyChildren {
		return implementationCycle{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return cycle, nil
}

// selectActiveImplementationCycles binds each active implementation attention
// to one exact persisted outer/dispatch pair. Operationally-failed outer tries
// are retry history. Any other sibling would represent competing authority and
// is rejected independently of command iteration order.
func selectActiveImplementationCycles(
	manifest admittedManifest,
	commands []journal.Command,
	effects map[string]journal.Effect,
	activeWork map[string]journal.AttentionProjection,
) (map[string]activeImplementationCycle, error) {
	grouped := make(map[string][]activeImplementationCycle)
	commandsByReplay := make(map[string]journal.Command, len(commands))
	for _, command := range commands {
		if _, duplicate := commandsByReplay[command.ReplayKey]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commandsByReplay[command.ReplayKey] = command
		if command.Kind != "git.seal" {
			continue
		}
		var probe struct {
			DispatchWork string `json:"dispatch_work"`
		}
		if json.Unmarshal(command.Payload, &probe) != nil {
			continue
		}
		attention, active := activeWork[probe.DispatchWork]
		if !active {
			continue
		}
		outer, found := effects[command.ReplayKey]
		if !found {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		cycle, err := validateImplementationCycleEnvelope(
			manifest,
			command,
			outer,
		)
		if err != nil {
			return nil, err
		}
		if cycle.DispatchWork != probe.DispatchWork {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		grouped[cycle.DispatchWork] = append(
			grouped[cycle.DispatchWork],
			activeImplementationCycle{
				command:   command,
				cycle:     cycle,
				outer:     outer,
				attention: attention,
			},
		)
	}

	result := make(map[string]activeImplementationCycle)
	for work, candidates := range grouped {
		var selected *activeImplementationCycle
		for index := range candidates {
			candidate := &candidates[index]
			switch candidate.outer.State {
			case journal.OperationalFailed:
				continue
			case journal.Claimed:
				if selected != nil {
					return nil, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				selected = candidate
			case journal.Pending, journal.Succeeded, journal.Uncertain:
				return nil, runtimeFail("CORRUPT_JOURNAL", nil)
			default:
				return nil, runtimeFail("CORRUPT_JOURNAL", nil)
			}
		}
		if selected == nil || selected.cycle.DispatchWork != work {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		dispatch, found := effects[selected.cycle.DispatchEffect]
		if !found ||
			dispatch.ID != selected.cycle.DispatchEffect ||
			dispatch.ReplayKey != dispatch.ID ||
			dispatch.Kind != "driver.dispatch" {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		switch dispatch.State {
		case journal.Claimed:
			// Both Open and Answered attention retain the exact claim.
		case journal.Succeeded:
			if selected.attention.State != journal.AttentionAnswered {
				return nil, runtimeFail("CORRUPT_JOURNAL", nil)
			}
		case journal.OperationalFailed:
			// The exact cycle is terminalized with its attention below.
		case journal.Pending, journal.Uncertain:
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		default:
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		dispatchCommand, found := commandsByReplay[dispatch.ReplayKey]
		if !found {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if err := validateAttentionDispatchBinding(
			manifest,
			dispatchCommand,
			dispatch,
			selected.attention,
			&selected.cycle,
		); err != nil {
			return nil, err
		}
		selected.dispatch = dispatch
		result[selected.outer.ID] = *selected
	}
	return result, nil
}

func validatePreparedSealEnvelope(
	command journal.Command,
	prepared journal.Effect,
	cycle implementationCycle,
) (sealedRecord, error) {
	if err := validateRecoveryCommand(command, prepared, true); err != nil {
		return sealedRecord{}, err
	}
	if prepared.ID != cycle.PreparedEffect ||
		prepared.ReplayKey != cycle.PreparedEffect ||
		prepared.Kind != "git.seal.prepared" ||
		prepared.BeforeDigest != cycle.Before {
		return sealedRecord{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	work, err := attemptWorkIdentity(prepared.ID)
	if err != nil || work != cycle.PreparedWork {
		return sealedRecord{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	var record sealedRecord
	if json.Unmarshal(command.Payload, &record) != nil ||
		!bytesEqualCanonicalJSON(command.Payload, record) ||
		!sealedRecordMatchesCycle(record, cycle) {
		return sealedRecord{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return record, nil
}

func validateImplementationCycleObjects(
	repository *gitx.Repository,
	cycle implementationCycle,
) error {
	if repository == nil {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	for _, value := range []string{
		cycle.Binds,
		cycle.Plan,
		cycle.ReleaseHead,
		cycle.TargetHead,
		cycle.TrackHead,
	} {
		if _, err := gitx.ParseOID(
			repository.ObjectFormat(),
			value,
		); err != nil {
			return runtimeFail("CORRUPT_JOURNAL", err)
		}
	}
	if cycle.Base != "" {
		if _, err := gitx.ParseOID(
			repository.ObjectFormat(),
			cycle.Base,
		); err != nil {
			return runtimeFail("CORRUPT_JOURNAL", err)
		}
	}
	if cycle.RefreshFrom != "" {
		if _, err := gitx.ParseOID(
			repository.ObjectFormat(),
			cycle.RefreshFrom,
		); err != nil {
			return runtimeFail("CORRUPT_JOURNAL", err)
		}
	}
	return nil
}

// recoverStaleClaimedDispatches sweeps historical driver claims after the
// structured seal and Baton-action recovery passes. A claim is retained only
// when its exact work identity is still dispatchable from the current Baton
// projection, or when it is the child of the exact current implementation seal
// cycle. Everything else is terminally stale; a malformed or substituted
// command/effect binding fails closed.
func (s *Service) recoverStaleClaimedDispatches(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
) (bool, error) {
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	state, stateErr := baton.ReadState(
		engine.git,
		engine.manifest.value.Release,
		engine.inertness,
	)
	return s.recoverStaleClaimedDispatchesFromSnapshot(
		ctx,
		engine,
		owner,
		snapshot,
		state,
		stateErr,
	)
}

// driverRecoveryPending reports whether recovery deliberately preserved an
// exact-current driver handoff whose outcome is still unknown. Such a handoff
// fences the whole scheduler: starting unrelated work while it remains claimed
// or uncertain would make takeover add new effects beside an unresolved
// external side effect.
func (s *Service) driverRecoveryPending(
	ctx context.Context,
	runID string,
) (bool, error) {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return false, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	attentions, err := s.journal.Attentions(ctx, runID)
	if err != nil {
		return false, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	parked, err := activeAttentionWork(attentions)
	if err != nil {
		return false, err
	}
	return snapshotHasPendingDriverRecoveryExcept(
		snapshot,
		parked,
	), nil
}

func snapshotHasPendingDriverRecovery(snapshot journal.Snapshot) bool {
	return snapshotHasPendingDriverRecoveryExcept(snapshot, nil)
}

func snapshotHasPendingDriverRecoveryExcept(
	snapshot journal.Snapshot,
	parked map[string]journal.AttentionProjection,
) bool {
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" {
			continue
		}
		if effect.State == journal.Claimed {
			if work, err := attemptWorkIdentity(effect.ID); err == nil {
				if _, laneParked := parked[work]; laneParked {
					continue
				}
			}
			return true
		}
		if effect.State == journal.Uncertain {
			return true
		}
	}
	return false
}

func (s *Service) recoverStaleClaimedDispatchesFromSnapshot(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	snapshot journal.Snapshot,
	state baton.State,
	stateErr error,
) (bool, error) {
	current, err := currentDriverAuthorities(engine, state, stateErr)
	if err != nil {
		return true, err
	}
	control, err := s.journal.ControlProjection(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("CORRUPT_JOURNAL", err)
	}
	attentions, err := s.journal.Attentions(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	parked, err := activeAttentionWork(attentions)
	if err != nil {
		return true, err
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		if _, duplicate := effects[effect.ID]; duplicate {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effects[effect.ID] = effect
	}
	implementation, err := implementationDispatchAuthorities(
		engine,
		snapshot.Commands,
		effects,
	)
	if err != nil {
		return true, err
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" ||
			(effect.State != journal.Claimed &&
				effect.State != journal.Uncertain &&
				effect.State != journal.Succeeded) {
			continue
		}
		var work string
		var epoch int64
		if effect.State == journal.Succeeded {
			work, epoch, err = attemptIdentity(effect.ID)
			if err != nil {
				return true, err
			}
			if _, found := parked[work]; !found {
				continue
			}
		} else {
			work, epoch, err = driverRecoveryWorkIdentity(effect)
			if err != nil {
				return true, err
			}
		}
		command, ok := commands[effect.ReplayKey]
		if !ok {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		dispatch, err := validateDriverRecoveryCommand(
			engine.manifest,
			command,
			effect,
		)
		if err != nil {
			return true, err
		}
		governed, implementationGoverned := implementation[effect.ID]
		if attention, found := parked[work]; found {
			var cycle *implementationCycle
			if implementationGoverned {
				cycle = &governed.cycle
			}
			if err := validateAttentionDispatchBinding(
				engine.manifest,
				command,
				effect,
				attention,
				cycle,
			); err != nil {
				return true, err
			}
		}
		if implementationGoverned {
			if dispatch.production != nil {
				context := dispatch.production.Context
				outerWork, _, identityErr := attemptIdentity(
					governed.outer.ID,
				)
				if identityErr != nil ||
					context.Slice != governed.cycle.Slice ||
					context.Responsibility !=
						driver.ImplementerImplementation ||
					context.Before != governed.cycle.Before ||
					context.Attempt < 1 ||
					governed.outer.ID != journal.AttemptEffectID(
						outerWork,
						context.Epoch,
						context.Try,
					) {
					return true, runtimeFail(
						"CORRUPT_JOURNAL",
						identityErr,
					)
				}
			}
			if effect.BeforeDigest != sha256Digest([]byte(governed.cycle.Before)) {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			switch governed.outer.State {
			case journal.Claimed:
				if stateErr == nil &&
					implementationDispatchAuthorityCurrent(state, governed.cycle) {
					if err := validateCurrentProductionDispatchContext(
						ctx,
						engine,
						dispatch,
					); err != nil {
						return true, err
					}
					continue
				}
			case journal.Uncertain:
				if stateErr != nil ||
					implementationDispatchAuthorityCurrent(
						state,
						governed.cycle,
					) {
					if stateErr == nil {
						if err := validateCurrentProductionDispatchContext(
							ctx,
							engine,
							dispatch,
						); err != nil {
							return true, err
						}
					}
					// The exact cycle may still be live. Preserve the coupled
					// ambiguity instead of independently resolving its child.
					continue
				}
				if effect.State != journal.Uncertain {
					// Legacy mixed states cannot be safely split. A current
					// recovery writes parent and child ambiguity atomically.
					continue
				}
				coupled := []string{governed.outer.ID, effect.ID}
				if prepared, ok := effects[governed.cycle.PreparedEffect]; ok {
					if prepared.State != journal.Uncertain {
						return true, runtimeFail(
							"RECOVERY_UNCERTAIN",
							nil,
						)
					}
					coupled = append(coupled, prepared.ID)
				}
				if err := s.journal.ResolveUncertainManyOwned(
					context.WithoutCancel(ctx),
					owner,
					owner.RunID,
					coupled,
					"stale_authority",
					s.now().UTC(),
				); err != nil {
					return true,
						runtimeFail("JOURNAL_WRITE_FAILED", err)
				}
				return true, nil
			case journal.OperationalFailed:
				// The failed parent can no longer consume this child.
			case journal.Pending, journal.Succeeded:
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			default:
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if err := s.resolveStaleDriverEffect(
				ctx,
				owner,
				effect,
				"stale_authority",
			); err != nil {
				return true, err
			}
			return true, nil
		}
		if dispatch.production != nil {
			context := dispatch.production.Context
			expectedWork := driverWorkIdentity(
				engine.manifest.digest,
				context.Slice,
				context.Responsibility,
				context.Attempt,
				context.Before,
			)
			if expectedWork != work ||
				effect.ID != journal.AttemptEffectID(
					expectedWork,
					context.Epoch,
					context.Try,
				) {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
		}
		if authority, ok := current[work]; ok {
			currentEpoch := control.RetryEpochs[work]
			if currentEpoch == 0 {
				currentEpoch = 1
			}
			if epoch != currentEpoch {
				var err error
				if attention, found := parked[work]; found {
					err = s.retireStaleDriverAttention(
						ctx,
						owner,
						effect,
						attention,
						"stale_authority",
					)
				} else {
					err = s.resolveStaleDriverEffect(
						ctx,
						owner,
						effect,
						"stale_authority",
					)
				}
				if err != nil {
					return true, err
				}
				return true, nil
			}
			if effect.BeforeDigest != authority.beforeDigest {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if err := validateCurrentProductionDispatchContext(
				ctx,
				engine,
				dispatch,
			); err != nil {
				if !IsCode(err, "STALE_DISPATCH") {
					return true, err
				}
				if attention, found := parked[work]; found {
					err = s.retireStaleDriverAttention(
						ctx,
						owner,
						effect,
						attention,
						"stale_authority",
					)
				} else {
					err = s.resolveStaleDriverEffect(
						ctx,
						owner,
						effect,
						"stale_authority",
					)
				}
				if err != nil {
					return true, err
				}
				return true, nil
			}
			if effect.State == journal.Claimed {
				if _, laneParked := parked[work]; laneParked {
					continue
				}
				if err := s.journal.ReconcileOwned(
					context.WithoutCancel(ctx),
					owner,
					journal.Completion{
						RunID:     owner.RunID,
						EffectID:  effect.ID,
						Token:     effect.CurrentClaim,
						EventKind: "dispatch_uncertain",
						EventBody: []byte(command.Kind),
						At:        s.now().UTC(),
					},
					journal.RecoveryAmbiguous,
				); err != nil {
					return true,
						runtimeFail("JOURNAL_WRITE_FAILED", err)
				}
				return true, nil
			}
			continue
		}
		var resolveErr error
		if attention, found := parked[work]; found {
			resolveErr = s.retireStaleDriverAttention(
				ctx,
				owner,
				effect,
				attention,
				"stale_authority",
			)
		} else {
			resolveErr = s.resolveStaleDriverEffect(
				ctx,
				owner,
				effect,
				"stale_authority",
			)
		}
		if resolveErr != nil {
			return true, resolveErr
		}
		return true, nil
	}
	return false, nil
}

func validateCurrentProductionDispatchContext(
	ctx context.Context,
	engine *engine,
	dispatch driverRecoveryCommand,
) error {
	if dispatch.production == nil {
		return nil
	}
	persisted := dispatch.production.Context
	request, err := productionRequestForContext(
		engine.manifest,
		persisted,
	)
	if err != nil {
		return err
	}
	currentBody, err := currentPreparedProductionBody(
		ctx,
		engine,
		dispatchCoordinates{
			Slice:          persisted.Slice,
			Responsibility: persisted.Responsibility,
			BatonAttempt:   persisted.Attempt,
			Epoch:          persisted.Epoch,
			Try:            persisted.Try,
		},
		persisted.Before,
		preparedDriverDispatch{
			request:           request,
			productionContext: &persisted,
		},
	)
	if err != nil {
		return err
	}
	if !bytes.Equal(currentBody, mustJSON(persisted)) {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	return nil
}

func (s *Service) resolveStaleDriverEffect(
	ctx context.Context,
	owner journal.OwnerLease,
	effect journal.Effect,
	code string,
) error {
	if effect.State == journal.Uncertain {
		if err := s.journal.ResolveUncertainOwned(
			context.WithoutCancel(ctx),
			owner,
			owner.RunID,
			effect.ID,
			code,
			s.now().UTC(),
		); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return nil
	}
	return s.terminalizeEffect(ctx, owner, effect, code)
}

func (s *Service) retireStaleDriverAttention(
	ctx context.Context,
	owner journal.OwnerLease,
	effect journal.Effect,
	attention journal.AttentionProjection,
	code string,
) error {
	if (effect.State != journal.Claimed &&
		effect.State != journal.Succeeded) ||
		(effect.State == journal.Claimed &&
			effect.CurrentClaim == "") ||
		(effect.State == journal.Succeeded &&
			effect.CurrentClaim != "") {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := s.discardRecoverableContinuation(
		owner.RunID,
		effect.ID,
	); err != nil {
		return runtimeFail("CONTINUATION_CLEANUP_FAILED", err)
	}
	if _, err := s.journal.RetireRecoveryAttentionOwned(
		context.WithoutCancel(ctx),
		owner,
		journal.RetireRecoveryAttentionCommand{
			RunID:              owner.RunID,
			Attention:          attention.Attention,
			ExpectedGeneration: attention.Generation,
			Effects: []journal.RetireRecoveryEffect{{
				EffectID:      effect.ID,
				ExpectedState: effect.State,
				ClaimToken:    effect.CurrentClaim,
			}},
			ErrorCode: code,
		},
		s.now().UTC(),
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func currentDriverAuthorities(
	engine *engine,
	state baton.State,
	stateErr error,
) (map[string]currentDriverAuthority, error) {
	current := make(map[string]currentDriverAuthority)
	add := func(
		slice string,
		responsibility driver.Responsibility,
		attempt int64,
		before string,
	) error {
		if before == "" || attempt < 1 {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		work := driverWorkIdentity(
			engine.manifest.digest,
			slice,
			responsibility,
			attempt,
			before,
		)
		authority := currentDriverAuthority{
			beforeDigest: sha256Digest([]byte(before)),
		}
		if prior, duplicate := current[work]; duplicate && prior != authority {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		current[work] = authority
		return nil
	}
	if stateErr != nil {
		if baton.ErrorCode(stateErr) != "REF_NOT_FOUND" {
			return nil, runtimeFail("BATON_UNAVAILABLE", stateErr)
		}
		release, target, err := captureProposalRefs(
			engine.repository,
			engine.manifest,
		)
		if err != nil {
			return nil, err
		}
		if release.State != gitx.RefAbsent || target.State != gitx.RefDirect {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		authority := planProposalAuthority{
			Release:    engine.manifest.value.Release,
			ReleaseRef: release.Ref,
			TargetRef:  target.Ref,
			TargetHead: target.Head.String(),
		}
		authority.Before = plannerAuthorityBefore(authority)
		if err := add("", driver.PlannerProposal, 1, authority.Before); err != nil {
			return nil, err
		}
		return current, nil
	}
	if state.Release != engine.manifest.value.Release ||
		state.Refs.Release.Ref !=
			"refs/heads/release-wt/"+engine.manifest.value.Release ||
		state.Refs.Target.Ref != engine.manifest.value.TargetRef {
		return nil, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if state.Plan.TargetStale {
		return current, nil
	}
	plannerNeeded := false
	for _, slice := range state.Slices {
		plannerNeeded = plannerNeeded || slice.NextRole == "planner"
	}
	plannerNeeded = plannerNeeded || state.Assembly.NextRole == "planner"
	ready := false
	for _, track := range state.Tracks {
		for _, slice := range track.Slices {
			if slice.Status != "ready" || slice.NextRole == "none" ||
				slice.NextRole == "merge" || slice.NextRole == "planner" {
				continue
			}
			ready = true
			before := sliceFingerprint(state, slice.Location.Slice.ID)
			switch {
			case slice.NextRole == "implementer" && slice.Stage == "design":
				err := add(
					slice.Location.Slice.ID,
					driver.ImplementerDesign,
					slice.Attempt,
					before,
				)
				if err != nil {
					return nil, err
				}
			case slice.NextRole == "captain":
				err := add(
					slice.Location.Slice.ID,
					driver.CaptainReview,
					slice.Attempt,
					before,
				)
				if err != nil {
					return nil, err
				}
			case slice.NextRole == "implementer" && slice.Stage == "implement":
				// The implementation dispatch is nested below git.seal and is
				// admitted against its durable cycle below.
			case slice.NextRole == "verifier":
				err := add(
					slice.Location.Slice.ID,
					driver.WorkVerification,
					slice.Attempt,
					before,
				)
				if err != nil {
					return nil, err
				}
			default:
				return nil, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			// The scheduler admits at most the first ready slice in each track.
			break
		}
	}
	if ready {
		if plannerNeeded {
			return currentPlannerAuthority(engine, state, current, add)
		}
		return current, nil
	}
	if plannerNeeded {
		return currentPlannerAuthority(engine, state, current, add)
	}
	if state.Assembly.NextRole == "verifier" &&
		state.Assembly.Candidate != nil &&
		state.Assembly.Candidate.Receipt.Candidate != nil {
		before := workIdentity(
			state.Plan.OID,
			state.Refs.Release.Head,
			state.Refs.Target.Head,
			*state.Assembly.Candidate.Receipt.Candidate,
		)
		if err := add(
			"",
			driver.AssemblyVerification,
			state.Plan.Metadata.Revision,
			before,
		); err != nil {
			return nil, err
		}
	}
	return current, nil
}

func currentPlannerAuthority(
	engine *engine,
	state baton.State,
	current map[string]currentDriverAuthority,
	add func(string, driver.Responsibility, int64, string) error,
) (map[string]currentDriverAuthority, error) {
	authority := planProposalAuthority{
		Release:     engine.manifest.value.Release,
		PriorPlan:   state.Plan.OID,
		ReleaseRef:  state.Refs.Release.Ref,
		ReleaseHead: state.Refs.Release.Head,
		TargetRef:   state.Refs.Target.Ref,
		TargetHead:  state.Refs.Target.Head,
	}
	authority.Before = plannerAuthorityBefore(authority)
	if err := add(
		"",
		driver.PlannerProposal,
		state.Plan.Metadata.Revision+1,
		authority.Before,
	); err != nil {
		return nil, err
	}
	return current, nil
}

func implementationDispatchAuthorities(
	engine *engine,
	commands []journal.Command,
	effects map[string]journal.Effect,
) (map[string]implementationDispatchAuthority, error) {
	result := make(map[string]implementationDispatchAuthority)
	for _, command := range commands {
		if command.Kind != "git.seal" {
			continue
		}
		outer, ok := effects[command.ReplayKey]
		if !ok {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		cycle, err := validateImplementationCycleEnvelope(
			engine.manifest,
			command,
			outer,
		)
		if err != nil {
			return nil, err
		}
		if engine.repository != nil {
			if err := validateImplementationCycleObjects(
				engine.repository,
				cycle,
			); err != nil {
				return nil, err
			}
		}
		if _, duplicate := result[cycle.DispatchEffect]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		result[cycle.DispatchEffect] = implementationDispatchAuthority{
			cycle: cycle,
			outer: outer,
		}
	}
	return result, nil
}

func implementationDispatchAuthorityCurrent(
	state baton.State,
	cycle implementationCycle,
) bool {
	if !implementationAuthorityCurrent(state, cycle) {
		return false
	}
	track, ok := state.Track(cycle.Track)
	return ok && track.Head == cycle.TrackHead
}

func driverRecoveryWorkIdentity(
	effect journal.Effect,
) (string, int64, error) {
	if effect.Kind != "driver.dispatch" ||
		(effect.State != journal.Claimed &&
			effect.State != journal.Uncertain) ||
		effect.ID != effect.ReplayKey {
		return "", 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return attemptIdentity(effect.ID)
}

type driverRecoveryCommand struct {
	fake       *fakeScript
	production *productionDispatchCommand
}

func validateDriverRecoveryCommand(
	manifest admittedManifest,
	command journal.Command,
	effect journal.Effect,
) (driverRecoveryCommand, error) {
	if err := validateRecoveryCommand(command, effect, false); err != nil {
		return driverRecoveryCommand{}, err
	}
	if manifest.value.production() {
		persisted, err := parseProductionDispatchCommand(
			manifest,
			command.Payload,
		)
		if err != nil ||
			effect.ExpectedDigest != productionOutputExpectation ||
			effect.BeforeDigest !=
				sha256Digest([]byte(persisted.Context.Before)) {
			return driverRecoveryCommand{},
				runtimeFail("CORRUPT_JOURNAL", err)
		}
		return driverRecoveryCommand{production: &persisted}, nil
	}
	var script fakeScript
	if json.Unmarshal(command.Payload, &script) != nil ||
		!bytes.Equal(command.Payload, mustJSON(script)) ||
		script.SchemaVersion != "sworn.fake-script/v1" ||
		script.Behavior == "" {
		return driverRecoveryCommand{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var expected []byte
	if script.Submission != "" {
		var err error
		expected, err = base64.StdEncoding.Strict().DecodeString(script.Submission)
		if err != nil {
			return driverRecoveryCommand{},
				runtimeFail("CORRUPT_JOURNAL", err)
		}
	}
	if effect.ExpectedDigest != sha256Digest(expected) {
		return driverRecoveryCommand{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return driverRecoveryCommand{fake: &script}, nil
}

func validateSucceededDriverResult(
	manifest admittedManifest,
	command journal.Command,
	effect journal.Effect,
) (driver.Submission, driverRecoveryCommand, error) {
	dispatch, err := validateDriverRecoveryCommand(
		manifest,
		command,
		effect,
	)
	if err != nil ||
		effect.State != journal.Succeeded ||
		effect.ResultDigest != sha256Digest(effect.Result) {
		return driver.Submission{}, driverRecoveryCommand{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	submission, err := driver.DecodeSubmission(effect.Result)
	if err != nil {
		return driver.Submission{}, driverRecoveryCommand{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	if dispatch.production != nil {
		context := dispatch.production.Context
		if submission.InvocationID != context.InvocationID ||
			submission.Responsibility != context.Responsibility {
			return driver.Submission{}, driverRecoveryCommand{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
	} else {
		var expected []byte
		if dispatch.fake.Submission != "" {
			expected, err = base64.StdEncoding.Strict().DecodeString(
				dispatch.fake.Submission,
			)
		}
		if err != nil || !bytes.Equal(effect.Result, expected) {
			return driver.Submission{}, driverRecoveryCommand{},
				runtimeFail("CORRUPT_JOURNAL", err)
		}
	}
	return submission, dispatch, nil
}

func attemptWorkIdentity(effectID string) (string, error) {
	work, _, err := attemptIdentity(effectID)
	return work, err
}

func attemptIdentity(effectID string) (string, int64, error) {
	work, epoch, _, err := attemptCoordinates(effectID)
	return work, epoch, err
}

func attemptCoordinates(
	effectID string,
) (string, int64, int64, error) {
	parts := strings.Split(effectID, "/")
	if len(parts) != 4 ||
		parts[0] != "attempt" ||
		len(parts[1]) != 64 ||
		parts[1] != strings.ToLower(parts[1]) {
		return "", 0, 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", 0, 0, runtimeFail("CORRUPT_JOURNAL", err)
	}
	epoch, epochErr := strconv.ParseInt(
		strings.TrimPrefix(parts[2], "e"),
		10,
		64,
	)
	try, tryErr := strconv.ParseInt(
		strings.TrimPrefix(parts[3], "t"),
		10,
		64,
	)
	work := "sha256:" + parts[1]
	if epochErr != nil || tryErr != nil ||
		epoch < 1 ||
		try < 1 ||
		try > 3 ||
		effectID != journal.AttemptEffectID(work, epoch, try) {
		return "", 0, 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return work, epoch, try, nil
}

func bytesEqualCanonicalJSON(body []byte, value any) bool {
	return bytes.Equal(body, mustJSON(value))
}
