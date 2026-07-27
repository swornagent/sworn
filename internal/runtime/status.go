package runtime

import (
	"context"
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
	ownerActive := ownerPresent && owner.ExpiresAt.After(s.now().UTC())
	ownerExpired := ownerPresent && !ownerActive
	result := RunStatus{SchemaVersion: "sworn.run-status/v2", RunID: runID,
		State: "new", DesiredState: control.Desired, ControlGeneration: control.Generation,
		ManifestDigest: snapshot.Run.ManifestDigest, TargetRef: snapshot.Run.TargetRef,
		Effects: make([]EffectStatus, 0, len(snapshot.Effects))}
	manifest, proposals, loadErr := loadRunSnapshot(snapshot, runID)
	if loadErr != nil {
		return RunStatus{}, loadErr
	}
	active, uncertain := false, false
	exhausted := make(map[string]struct{})
	for _, effect := range snapshot.Effects {
		if effect.ID == "runtime.owner" || effect.Kind == "runtime.control" {
			continue
		}
		result.Effects = append(result.Effects, EffectStatus{ID: effect.ID, Kind: effect.Kind,
			State: string(effect.State), ErrorCode: effect.ErrorCode})
		active = active || (effect.State == journal.Claimed && ownerActive)
		uncertain = uncertain || effect.State == journal.Uncertain ||
			(effect.State == journal.Claimed && !ownerActive)
		if effect.State == journal.OperationalFailed && strings.HasSuffix(effect.ID, "/t3") {
			parts := strings.Split(effect.ID, "/")
			if len(parts) == 4 {
				epoch, _ := strconv.ParseInt(strings.TrimPrefix(parts[2], "e"), 10, 64)
				work := "sha256:" + parts[1]
				current := control.RetryEpochs[work]
				if current == 0 {
					current = 1
				}
				if epoch == current {
					exhausted[work] = struct{}{}
				}
			}
		}
	}
	parked := len(exhausted) != 0
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
	latestRevision := int64(0)
	if proposalFound {
		latestRevision = proposal.plan.Metadata().Revision
		result.PlanDigest = proposal.plan.Digest()
		if !proposalInstalled {
			result.TargetHead = proposal.authority.TargetHead
			parked = intersectsWork(exhausted, map[string]struct{}{
				proposalInstallWork(proposal): {},
			})
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
		if parked {
			result.State = "parked"
		} else if ownerExpired {
			result.State = "takeover_required"
		} else if proposalFound && !proposalInstalled {
			result.State = "awaiting_approval"
		}
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
	parked = exhaustedWorkApplies(
		manifest, selected, proposalInstalled, state, snapshot, exhausted)
	if control.Desired == "running" && !uncertain && !parked {
		switch {
		case active:
			result.State = "running"
		case ownerExpired:
			result.State = "takeover_required"
		case state.Assembly.Outcome == "merged":
			result.State = "complete"
		case proposalFound && !proposalInstalled &&
			state.Plan.Metadata.Revision < latestRevision:
			result.State = "awaiting_approval"
		default:
			result.State = "running"
		}
	}
	return result, nil
}

func exhaustedWorkApplies(
	manifest admittedManifest,
	proposal *admittedPlanProposal,
	proposalInstalled bool,
	state baton.State,
	snapshot journal.Snapshot,
	exhausted map[string]struct{},
) bool {
	if len(exhausted) == 0 {
		return false
	}
	applicable := make(map[string]struct{})
	add := func(work string) {
		if work != "" {
			applicable[work] = struct{}{}
		}
	}
	if proposal != nil && !proposalInstalled {
		add(proposalInstallWork(*proposal))
		return intersectsWork(exhausted, applicable)
	}
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
				add(driverWorkIdentity(manifest.digest, slice.Location.Slice.ID,
					driver.ImplementerDesign, slice.Attempt, before))
			case slice.NextRole == "captain":
				add(driverWorkIdentity(manifest.digest, slice.Location.Slice.ID,
					driver.CaptainReview, slice.Attempt, before))
			case slice.NextRole == "implementer" && slice.Stage == "implement":
				add(workIdentity(before, "git.seal"))
			case slice.NextRole == "verifier":
				add(driverWorkIdentity(manifest.digest, slice.Location.Slice.ID,
					driver.WorkVerification, slice.Attempt, before))
			}
			break
		}
	}
	if ready {
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
			before := sliceFingerprint(state, input.Slice)
			if before != "" {
				add(workIdentity(
					before, "append", input.Role, input.Result, input.Candidate))
			}
		}
		return intersectsWork(exhausted, applicable)
	}
	plannerNeeded := state.Plan.TargetStale
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
		add(driverWorkIdentity(manifest.digest, "", driver.PlannerProposal,
			state.Plan.Metadata.Revision+1, before))
		return intersectsWork(exhausted, applicable)
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
	return intersectsWork(exhausted, applicable)
}

func intersectsWork(left, right map[string]struct{}) bool {
	for work := range left {
		if _, ok := right[work]; ok {
			return true
		}
	}
	return false
}
