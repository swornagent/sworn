package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	trackBaseCommandVersion   = "sworn.git-prepare-track-base/v3"
	trackBaseCommandVersionV2 = "sworn.git-prepare-track-base/v2"
)

type trackBaseInputWire struct {
	Slice            string `json:"slice"`
	ProducerTrack    string `json:"producer_track"`
	SourceHead       string `json:"source_head"`
	PassReceipt      string `json:"pass_receipt"`
	CandidateReceipt string `json:"candidate_receipt"`
	Candidate        string `json:"candidate"`
	ProductTree      string `json:"product_tree"`
}

type trackBaseRequestWire struct {
	Release        string               `json:"release"`
	Plan           string               `json:"plan"`
	ReleaseHead    string               `json:"release_head"`
	TargetRef      string               `json:"target_ref"`
	TargetHead     string               `json:"target_head"`
	ApprovedTarget string               `json:"approved_target,omitempty"`
	ConsumerTrack  string               `json:"consumer_track"`
	ConsumerSlice  string               `json:"consumer_slice"`
	AuthoritySeed  string               `json:"authority_seed"`
	ConsumerBefore string               `json:"consumer_before,omitempty"`
	Inputs         []trackBaseInputWire `json:"inputs"`
}

type trackBaseCommand struct {
	Version string               `json:"version"`
	Before  string               `json:"before"`
	Request trackBaseRequestWire `json:"request"`
}

type trackBaseResultWire struct {
	Action         string               `json:"action"`
	Changed        bool                 `json:"changed"`
	ConsumerRef    string               `json:"consumer_ref"`
	ConsumerBefore string               `json:"consumer_before,omitempty"`
	Seed           string               `json:"seed"`
	SeedTree       string               `json:"seed_tree"`
	Base           string               `json:"base"`
	BaseTree       string               `json:"base_tree"`
	Inputs         []trackBaseInputWire `json:"inputs"`
}

func trackBaseBefore(state baton.State, slice *baton.SliceState) string {
	if slice == nil {
		return ""
	}
	consumerBefore := ""
	authoritySeed := ""
	if track, ok := state.Track(slice.Location.Track.ID); ok {
		consumerBefore = track.Head
		authoritySeed = track.AuthorityHead
	}
	return workIdentity(
		state.Plan.OID,
		state.Refs.Target.Ref,
		state.Refs.Target.Head,
		slice.Location.Slice.ID,
		slice.Stage,
		slice.NextRole,
		slice.Attempt,
		authoritySeed,
		slice.PreparedBase,
		consumerBefore,
		slice.ConsumedInputs,
	)
}

func candidateHeadRefresh(
	state baton.State,
	slice *baton.SliceState,
) bool {
	if slice == nil ||
		slice.Stage != "implement" ||
		slice.Status != "ready" ||
		slice.NextRole != "implementer" ||
		slice.Outcome != "stale" ||
		slice.Retained ||
		slice.Attempt != slice.History.MaximumAttempt+1 ||
		slice.CurrentReceipt == nil ||
		slice.Candidate == nil ||
		slice.CurrentReceipt.OID != slice.Candidate.OID {
		return false
	}
	track, ok := state.Track(slice.Location.Track.ID)
	return ok &&
		track.Head != "" &&
		track.Head != slice.CurrentReceipt.OID &&
		track.AuthorityHead == slice.CurrentReceipt.OID
}

func trackBaseRequestFromWire(
	engine *engine,
	wire trackBaseRequestWire,
	snapshot *baton.State,
) (gitx.PrepareTrackBaseRequest, error) {
	if engine == nil || engine.repository == nil || engine.product == nil ||
		wire.Release != engine.manifest.value.Release ||
		wire.TargetRef != engine.manifest.value.TargetRef ||
		!runtimeIdentityPattern.MatchString(wire.ConsumerTrack) ||
		!runtimeIdentityPattern.MatchString(wire.ConsumerSlice) ||
		wire.Inputs == nil ||
		len(wire.Inputs) > gitx.MaxTrackBaseInputs {
		return gitx.PrepareTrackBaseRequest{},
			runtimeFail("INVALID_TRACK_BASE", nil)
	}
	format := engine.repository.ObjectFormat()
	plan, err := gitx.ParseOID(format, wire.Plan)
	if err != nil {
		return gitx.PrepareTrackBaseRequest{}, err
	}
	releaseHead, err := gitx.ParseOID(format, wire.ReleaseHead)
	if err != nil {
		return gitx.PrepareTrackBaseRequest{}, err
	}
	targetHead, err := gitx.ParseOID(format, wire.TargetHead)
	if err != nil {
		return gitx.PrepareTrackBaseRequest{}, err
	}
	approvedTargetValue := wire.ApprovedTarget
	if approvedTargetValue == "" {
		// v2 commands predate RC13 target-lineage support, when the live and
		// approved target were required to be identical.
		approvedTargetValue = wire.TargetHead
	}
	approvedTarget, err := gitx.ParseOID(format, approvedTargetValue)
	if err != nil {
		return gitx.PrepareTrackBaseRequest{}, err
	}
	authoritySeed, err := gitx.ParseOID(format, wire.AuthoritySeed)
	if err != nil {
		return gitx.PrepareTrackBaseRequest{}, err
	}
	var before *gitx.OID
	if wire.ConsumerBefore != "" {
		value, err := gitx.ParseOID(format, wire.ConsumerBefore)
		if err != nil {
			return gitx.PrepareTrackBaseRequest{}, err
		}
		before = &value
	}
	request := gitx.PrepareTrackBaseRequest{
		Release:        wire.Release,
		Plan:           plan,
		ReleaseHead:    releaseHead,
		TargetRef:      wire.TargetRef,
		TargetHead:     targetHead,
		ApprovedTarget: approvedTarget,
		Consumer: gitx.TrackKey{
			Release: wire.Release,
			Track:   wire.ConsumerTrack,
		},
		AuthoritySeed:    authoritySeed,
		ConsumerBefore:   before,
		ProductAdmission: engine.product,
		Inputs:           make([]gitx.TrackBaseInput, 0, len(wire.Inputs)),
	}
	seen := make(map[string]bool, len(wire.Inputs))
	for _, input := range wire.Inputs {
		if !runtimeIdentityPattern.MatchString(input.Slice) ||
			!runtimeIdentityPattern.MatchString(input.ProducerTrack) ||
			!runtimeDigestPattern.MatchString(input.ProductTree) ||
			seen[input.Slice] {
			return gitx.PrepareTrackBaseRequest{},
				runtimeFail("INVALID_TRACK_BASE", nil)
		}
		seen[input.Slice] = true
		sourceHead, err := gitx.ParseOID(format, input.SourceHead)
		if err != nil {
			return gitx.PrepareTrackBaseRequest{}, err
		}
		pass, err := gitx.ParseOID(format, input.PassReceipt)
		if err != nil {
			return gitx.PrepareTrackBaseRequest{}, err
		}
		candidateReceipt, err := gitx.ParseOID(
			format,
			input.CandidateReceipt,
		)
		if err != nil {
			return gitx.PrepareTrackBaseRequest{}, err
		}
		candidate, err := gitx.ParseOID(format, input.Candidate)
		if err != nil {
			return gitx.PrepareTrackBaseRequest{}, err
		}
		request.Inputs = append(request.Inputs, gitx.TrackBaseInput{
			Slice: input.Slice,
			Producer: gitx.TrackKey{
				Release: wire.Release,
				Track:   input.ProducerTrack,
			},
			SourceHead:       sourceHead,
			PassReceipt:      pass,
			CandidateReceipt: candidateReceipt,
			Candidate:        candidate,
			ProductTree:      input.ProductTree,
		})
	}
	state := snapshot
	if state == nil {
		fresh, err := baton.ReadState(
			engine.git,
			wire.Release,
			engine.inertness,
		)
		if err != nil {
			return gitx.PrepareTrackBaseRequest{},
				runtimeFail("BATON_UNAVAILABLE", err)
		}
		state = &fresh
	}
	current, slicePresent := state.Slice(wire.ConsumerSlice)
	track, trackPresent := state.Track(wire.ConsumerTrack)
	if !slicePresent || !trackPresent ||
		state.Release != wire.Release ||
		state.Plan.OID != wire.Plan ||
		state.Refs.Release.Head != wire.ReleaseHead ||
		state.Refs.Target.Ref != wire.TargetRef ||
		state.Refs.Target.Head != wire.TargetHead ||
		state.Plan.Approval.Receipt.Target == nil ||
		*state.Plan.Approval.Receipt.Target != approvedTargetValue ||
		current.Location.Track.ID != wire.ConsumerTrack ||
		track.AuthorityHead != wire.AuthoritySeed ||
		!currentConsumedInputsMatch(
			wire.Release,
			current.ConsumedInputs,
			request.Inputs,
		) {
		return gitx.PrepareTrackBaseRequest{},
			runtimeFail("STALE_DISPATCH", nil)
	}
	resolver, err := state.BindTrackBaseProductResolver(format)
	if err != nil {
		return gitx.PrepareTrackBaseRequest{}, err
	}
	request.ResolveProductBase = resolver
	return request, nil
}

func trackBaseRequestForSlice(
	engine *engine,
	state baton.State,
	slice *baton.SliceState,
) (trackBaseCommand, gitx.PrepareTrackBaseRequest, error) {
	if engine == nil || engine.repository == nil || engine.product == nil ||
		slice == nil {
		return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{},
			runtimeFail("INVALID_TRACK_BASE", nil)
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{},
			runtimeFail("INVALID_TRACK_BASE", nil)
	}
	wire := trackBaseRequestWire{
		Release: state.Release, Plan: state.Plan.OID,
		ReleaseHead:    state.Refs.Release.Head,
		TargetRef:      state.Refs.Target.Ref,
		TargetHead:     state.Refs.Target.Head,
		ApprovedTarget: *state.Plan.Approval.Receipt.Target,
		ConsumerTrack:  slice.Location.Track.ID,
		ConsumerSlice:  slice.Location.Slice.ID,
		AuthoritySeed:  track.AuthorityHead,
		ConsumerBefore: track.Head,
		Inputs:         make([]trackBaseInputWire, 0, len(slice.ConsumedInputs)),
	}
	for _, consumed := range slice.ConsumedInputs {
		producer, ok := state.Slice(consumed.Slice)
		if !ok {
			return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{},
				runtimeFail("INVALID_TRACK_BASE", nil)
		}
		wire.Inputs = append(wire.Inputs, trackBaseInputWire{
			Slice:            consumed.Slice,
			ProducerTrack:    producer.Location.Track.ID,
			SourceHead:       consumed.SourceHead,
			PassReceipt:      consumed.PassReceipt,
			CandidateReceipt: consumed.CandidateReceipt,
			Candidate:        consumed.Candidate,
			ProductTree:      consumed.ProductTree,
		})
	}
	request, err := trackBaseRequestFromWire(engine, wire, &state)
	if err != nil {
		return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{}, err
	}
	command := trackBaseCommand{
		Version: trackBaseCommandVersion,
		Before:  trackBaseBefore(state, slice),
		Request: wire,
	}
	return command, request, nil
}

func wireTrackBaseResult(
	result gitx.PrepareTrackBaseResult,
) trackBaseResultWire {
	wire := trackBaseResultWire{
		Action: string(result.Action), Changed: result.Changed,
		ConsumerRef: result.ConsumerRef,
		Seed:        result.Seed.String(), SeedTree: result.SeedTree.String(),
		Base: result.Base.String(), BaseTree: result.BaseTree.String(),
		Inputs: make([]trackBaseInputWire, 0, len(result.Inputs)),
	}
	if result.ConsumerBefore != nil {
		wire.ConsumerBefore = result.ConsumerBefore.String()
	}
	for _, input := range result.Inputs {
		wire.Inputs = append(wire.Inputs, trackBaseInputWire{
			Slice: input.Slice, ProducerTrack: input.Producer.Track,
			SourceHead:       input.SourceHead.String(),
			PassReceipt:      input.PassReceipt.String(),
			CandidateReceipt: input.CandidateReceipt.String(),
			Candidate:        input.Candidate.String(),
			ProductTree:      input.ProductTree,
		})
	}
	return wire
}

func canonicalTrackBaseResult(raw []byte) (trackBaseResultWire, error) {
	var result trackBaseResultWire
	if json.Unmarshal(raw, &result) != nil ||
		!bytesEqualCanonicalJSON(raw, result) ||
		result.Action == "" || result.ConsumerRef == "" ||
		result.Seed == "" || result.SeedTree == "" ||
		result.Base == "" || result.BaseTree == "" ||
		result.Inputs == nil {
		return trackBaseResultWire{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return result, nil
}

func trackBaseResultEqual(
	left trackBaseResultWire,
	right trackBaseResultWire,
) bool {
	return bytes.Equal(mustJSON(left), mustJSON(right))
}

func currentConsumedInputsMatch(
	release string,
	current []baton.ConsumedInput,
	prepared []gitx.TrackBaseInput,
) bool {
	if len(current) != len(prepared) {
		return false
	}
	for index := range current {
		left, right := current[index], prepared[index]
		if left.Slice != right.Slice ||
			left.SourceRef != "refs/heads/track/"+release+"/"+right.Producer.Track ||
			left.SourceHead != right.SourceHead.String() ||
			left.PassReceipt != right.PassReceipt.String() ||
			left.CandidateReceipt != right.CandidateReceipt.String() ||
			left.Candidate != right.Candidate.String() ||
			left.ProductTree != right.ProductTree {
			return false
		}
	}
	return true
}

func sameCurrentReceipt(
	left *baton.ReceiptEntry,
	right *baton.ReceiptEntry,
) bool {
	return (left == nil && right == nil) ||
		(left != nil && right != nil && left.OID == right.OID)
}

func trackBaseCommandRequestError(err error) error {
	if IsCode(err, "STALE_DISPATCH") {
		return err
	}
	return runtimeFail("CORRUPT_JOURNAL", err)
}

func validateTrackBaseCommand(
	engine *engine,
	command journal.Command,
	effect journal.Effect,
) (trackBaseCommand, gitx.PrepareTrackBaseRequest, error) {
	if err := validateRecoveryCommand(command, effect, true); err != nil {
		return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{}, err
	}
	var persisted trackBaseCommand
	if command.Kind != "git.prepare_track_base" ||
		effect.Kind != "git.prepare_track_base" ||
		json.Unmarshal(command.Payload, &persisted) != nil ||
		!bytesEqualCanonicalJSON(command.Payload, persisted) ||
		(persisted.Version != trackBaseCommandVersion &&
			persisted.Version != trackBaseCommandVersionV2) ||
		(persisted.Version == trackBaseCommandVersion &&
			persisted.Request.ApprovedTarget == "") ||
		!runtimeDigestPattern.MatchString(persisted.Before) {
		return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	workID := workIdentity(persisted.Before, "git.prepare_track_base")
	attemptWork, err := attemptWorkIdentity(effect.ID)
	if err != nil || attemptWork != workID ||
		effect.ID != effect.ReplayKey ||
		effect.BeforeDigest != workID {
		return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	request, err := trackBaseRequestFromWire(engine, persisted.Request, nil)
	if err != nil {
		return trackBaseCommand{}, gitx.PrepareTrackBaseRequest{},
			trackBaseCommandRequestError(err)
	}
	return persisted, request, nil
}

func (s *Service) terminalizeStaleTrackBase(
	ctx context.Context,
	owner journal.OwnerLease,
	effect journal.Effect,
	err error,
) (bool, error) {
	if !IsCode(err, "STALE_DISPATCH") {
		return false, err
	}
	if finishErr := s.finishClaimedFailure(
		ctx,
		owner,
		effect,
		"stale_authority",
	); finishErr != nil {
		return true, finishErr
	}
	return true, nil
}

func (s *Service) finishTrackBaseEffect(
	ctx context.Context,
	owner journal.OwnerLease,
	effect journal.Effect,
	result gitx.PrepareTrackBaseResult,
	recovery journal.RecoveryDisposition,
) error {
	wire := wireTrackBaseResult(result)
	body := mustJSON(wire)
	completion := journal.Completion{
		RunID: owner.RunID, EffectID: effect.ID,
		Token: effect.CurrentClaim, State: journal.Succeeded,
		Result: body,
		Receipts: []journal.Receipt{{
			Kind: "git_track_base",
			Body: body,
		}},
		EventKind: "track_base_prepared",
		EventBody: body,
		At:        s.now().UTC(),
	}
	var err error
	if recovery == "" {
		err = s.journal.CompleteOwned(
			context.WithoutCancel(ctx),
			owner,
			completion,
		)
	} else {
		err = s.journal.ReconcileOwned(
			context.WithoutCancel(ctx),
			owner,
			completion,
			recovery,
		)
	}
	if err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) recoverTrackBaseEffect(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	effect journal.Effect,
	request gitx.PrepareTrackBaseRequest,
) (gitx.PrepareTrackBaseResult, error) {
	expected, err := engine.workspaces.ExpectedTrackBase(request)
	if err != nil {
		return gitx.PrepareTrackBaseResult{},
			runtimeFail("RECOVERY_UNCERTAIN", err)
	}
	disposition, err := engine.workspaces.ReconcileTrackBase(request, expected)
	if err != nil {
		return gitx.PrepareTrackBaseResult{},
			runtimeFail("RECOVERY_UNCERTAIN", err)
	}
	switch disposition {
	case gitx.TrackBaseAllOld:
		result, err := engine.workspaces.PrepareTrackBase(request)
		if err != nil {
			code := stableErrorCode(err)
			if finishErr := s.finishClaimedFailure(
				ctx,
				owner,
				effect,
				code,
			); finishErr != nil {
				return gitx.PrepareTrackBaseResult{}, finishErr
			}
			if code == "AUTHORITY_MOVED" {
				return gitx.PrepareTrackBaseResult{},
					runtimeFail("STALE_DISPATCH", err)
			}
			return gitx.PrepareTrackBaseResult{}, err
		}
		if err := s.finishTrackBaseEffect(
			ctx,
			owner,
			effect,
			result,
			journal.RecoveryAllNew,
		); err != nil {
			return gitx.PrepareTrackBaseResult{}, err
		}
		return result, nil
	case gitx.TrackBaseAllNew, gitx.TrackBaseAdvanced:
		if err := s.finishTrackBaseEffect(
			ctx,
			owner,
			effect,
			expected,
			journal.RecoveryAllNew,
		); err != nil {
			return gitx.PrepareTrackBaseResult{}, err
		}
		return expected, nil
	default:
		_ = s.journal.ReconcileOwned(
			context.WithoutCancel(ctx),
			owner,
			journal.Completion{
				RunID: owner.RunID, EffectID: effect.ID,
				Token:     effect.CurrentClaim,
				EventKind: "track_base_uncertain",
				EventBody: []byte(effect.ID),
				At:        s.now().UTC(),
			},
			journal.RecoveryAmbiguous,
		)
		return gitx.PrepareTrackBaseResult{},
			runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
}

func (s *Service) runTrackBaseEffect(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	command trackBaseCommand,
	request gitx.PrepareTrackBaseRequest,
) (gitx.PrepareTrackBaseResult, error) {
	payload := mustJSON(command)
	workID := workIdentity(command.Before, "git.prepare_track_base")
	projection, err := s.journal.ControlProjection(ctx, owner.RunID)
	if err != nil {
		return gitx.PrepareTrackBaseResult{},
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	epoch := projection.RetryEpochs[workID]
	if epoch == 0 {
		epoch = 1
	}
	for try := int64(1); try <= 3; try++ {
		id := journal.AttemptEffectID(workID, epoch, try)
		now := s.now().UTC()
		journalCommand := journal.Command{
			RunID: owner.RunID, ReplayKey: id,
			Kind:    "git.prepare_track_base",
			Payload: payload, CreatedAt: now,
		}
		effectInput := journal.Effect{
			RunID: owner.RunID, ID: id, ReplayKey: id,
			Kind:           "git.prepare_track_base",
			BeforeDigest:   workID,
			ExpectedDigest: sha256Digest(payload),
			UpdatedAt:      now,
		}
		if err := s.journal.EnsureAttempt(
			ctx,
			journalCommand,
			effectInput,
			journal.EffectAttempt{
				WorkID: workID,
				Epoch:  epoch,
				Try:    try,
			},
		); err != nil {
			return gitx.PrepareTrackBaseResult{},
				runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		effect, err := s.journal.Effect(ctx, owner.RunID, id)
		if err != nil {
			return gitx.PrepareTrackBaseResult{},
				runtimeFail("JOURNAL_READ_FAILED", err)
		}
		if err := validateRecoveryCommand(
			journalCommand,
			effect,
			true,
		); err != nil {
			return gitx.PrepareTrackBaseResult{},
				runtimeFail("CORRUPT_JOURNAL", err)
		}
		switch effect.State {
		case journal.Succeeded:
			stored, err := canonicalTrackBaseResult(effect.Result)
			if err != nil {
				return gitx.PrepareTrackBaseResult{}, err
			}
			expected, err := engine.workspaces.ExpectedTrackBase(request)
			if err != nil ||
				!trackBaseResultEqual(
					stored,
					wireTrackBaseResult(expected),
				) {
				return gitx.PrepareTrackBaseResult{},
					runtimeFail("RECOVERY_UNCERTAIN", err)
			}
			return expected, nil
		case journal.Claimed:
			return s.recoverTrackBaseEffect(
				ctx,
				engine,
				owner,
				effect,
				request,
			)
		case journal.OperationalFailed:
			continue
		case journal.Pending:
		default:
			return gitx.PrepareTrackBaseResult{},
				runtimeFail("RECOVERY_UNCERTAIN", nil)
		}
		claim, err := s.journal.ClaimOwned(
			ctx,
			owner,
			id,
			now,
			effectLease,
		)
		if err != nil {
			return gitx.PrepareTrackBaseResult{},
				runtimeFail("EFFECT_CLAIM_FAILED", err)
		}
		effect.State, effect.CurrentClaim = journal.Claimed, claim.Token
		if testCrashBeforeEffect == "git.prepare_track_base" {
			os.Exit(86)
		}
		result, actionErr := engine.workspaces.PrepareTrackBase(request)
		if actionErr == nil {
			if testCrashAfterEffect == "git.prepare_track_base" {
				os.Exit(86)
			}
			if err := s.finishTrackBaseEffect(
				ctx,
				owner,
				effect,
				result,
				"",
			); err != nil {
				return gitx.PrepareTrackBaseResult{}, err
			}
			return result, nil
		}
		if err := s.journal.CompleteOwned(
			context.WithoutCancel(ctx),
			owner,
			journal.Completion{
				RunID: owner.RunID, EffectID: id,
				Token:     claim.Token,
				State:     journal.OperationalFailed,
				ErrorCode: stableErrorCode(actionErr),
				EventKind: "track_base_operational_failure",
				EventBody: []byte(id),
				At:        s.now().UTC(),
			},
		); err != nil {
			return gitx.PrepareTrackBaseResult{},
				runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		if stableErrorCode(actionErr) == "AUTHORITY_MOVED" {
			return gitx.PrepareTrackBaseResult{},
				runtimeFail("STALE_DISPATCH", actionErr)
		}
	}
	return gitx.PrepareTrackBaseResult{},
		runtimeFail("EFFECT_PARKED", nil)
}

func (s *Service) recoverClaimedTrackBase(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
) (bool, error) {
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "git.prepare_track_base" ||
			effect.State != journal.Claimed {
			continue
		}
		command, present := commands[effect.ReplayKey]
		if !present {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		_, request, err := validateTrackBaseCommand(
			engine,
			command,
			effect,
		)
		if err != nil {
			if handled, recoveryErr := s.terminalizeStaleTrackBase(
				ctx,
				owner,
				effect,
				err,
			); handled {
				return true, recoveryErr
			}
			return true, err
		}
		_, err = s.recoverTrackBaseEffect(
			ctx,
			engine,
			owner,
			effect,
			request,
		)
		return true, err
	}
	return false, nil
}

func (s *Service) prepareTrackBaseForSlice(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	state baton.State,
	slice *baton.SliceState,
) (baton.State, *baton.SliceState, error) {
	if slice == nil {
		return state, slice, nil
	}
	if state.Plan.TargetStale || slice.Status != "ready" ||
		slice.NextRole != "implementer" ||
		(slice.Stage != "design" && slice.Stage != "implement") {
		return baton.State{}, nil, runtimeFail("STALE_DISPATCH", nil)
	}
	if candidateHeadRefresh(state, slice) {
		return state, slice, nil
	}
	command, request, err := trackBaseRequestForSlice(engine, state, slice)
	if err != nil {
		return baton.State{}, nil, err
	}
	result, err := s.runTrackBaseEffect(
		ctx,
		engine,
		owner,
		command,
		request,
	)
	if err != nil {
		return baton.State{}, nil, err
	}
	fresh, err := baton.ReadState(
		engine.git,
		state.Release,
		engine.inertness,
	)
	if err != nil {
		return baton.State{}, nil, runtimeFail("BATON_UNAVAILABLE", err)
	}
	current, ok := fresh.Slice(slice.Location.Slice.ID)
	track, trackOK := fresh.Track(slice.Location.Track.ID)
	if !ok || !trackOK || fresh.Plan.TargetStale ||
		fresh.Plan.OID != state.Plan.OID ||
		fresh.Refs.Target.Ref != state.Refs.Target.Ref ||
		fresh.Refs.Target.Head != state.Refs.Target.Head ||
		current.Location.Track.ID != slice.Location.Track.ID ||
		current.Stage != slice.Stage ||
		current.Status != slice.Status ||
		current.NextRole != slice.NextRole ||
		current.Attempt != slice.Attempt ||
		!sameCurrentReceipt(current.CurrentReceipt, slice.CurrentReceipt) ||
		current.PreparationSeed != slice.PreparationSeed ||
		(len(current.Location.Slice.Consumes) > 0 &&
			current.PreparedBase != result.Base.String()) ||
		track.Ref != result.ConsumerRef ||
		track.AuthorityHead != result.Seed.String() ||
		track.Head != result.Base.String() ||
		!currentConsumedInputsMatch(
			fresh.Release,
			current.ConsumedInputs,
			result.Inputs,
		) {
		return baton.State{}, nil, runtimeFail("STALE_DISPATCH", nil)
	}
	return fresh, current, nil
}
