package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
)

// validateCaptainReleaseLineage proves provenance, not merely ancestry. Every
// installed plan after the admitted anchor must be backed by the exact
// delegated S2 approval that was the deterministic child of one successful
// PROCEED decision under the currently active envelope epoch.
func (s *Service) validateCaptainReleaseLineage(
	ctx context.Context,
	manifest admittedManifest,
	proposal admittedPlanProposal,
	snapshot journal.Snapshot,
) error {
	delegation, err := currentCaptainDelegation(snapshot)
	if err != nil || !delegation.Active {
		return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", err)
	}
	engine, err := s.openEngine(manifest)
	if err != nil {
		return err
	}
	defer engine.Close()
	return validateCaptainReleaseLineageWithEngine(engine, manifest, proposal, snapshot, delegation)
}

func validateCaptainReleaseLineageWithEngine(
	engine *engine,
	manifest admittedManifest,
	proposal admittedPlanProposal,
	snapshot journal.Snapshot,
	delegation CaptainDelegationState,
) error {
	state, stateErr := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if stateErr != nil {
		if baton.ErrorCode(stateErr) == "REF_NOT_FOUND" &&
			delegation.Envelope.ReleaseLineageAnchor.State == "absent" &&
			proposal.plan.Metadata().Revision == 1 && proposal.authority.ReleaseHead == "" {
			if err := ValidateCaptainPlanPolicy(delegation.Envelope.PlanRules, proposal.plan, nil); err != nil {
				return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", err)
			}
			return nil
		}
		return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", stateErr)
	}
	if proposal.authority.ReleaseHead != state.Refs.Release.Head ||
		proposal.authority.PriorPlan != state.Plan.OID ||
		proposal.plan.Metadata().Revision != state.Plan.Metadata.Revision+1 {
		return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
	}
	var prior *baton.Plan
	for _, history := range state.Plan.History {
		if history.OID == proposal.authority.PriorPlan {
			if prior != nil {
				return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
			}
			copy := history.Plan
			prior = &copy
		}
	}
	if prior == nil || ValidateCaptainPlanPolicy(delegation.Envelope.PlanRules, proposal.plan, prior) != nil {
		return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
	}

	start := -1
	anchor := delegation.Envelope.ReleaseLineageAnchor
	if anchor.State == "absent" {
		start = -1
	} else {
		for index, history := range state.Plan.History {
			if history.OID == anchor.PlanOID && history.Revision == anchor.PlanRevision &&
				history.InstallHead == anchor.ReleaseHead {
				if start != -1 {
					return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
				}
				start = index
			}
		}
		if start == -1 {
			return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
		}
	}
	proposals, err := allCaptainPlanProposals(manifest, snapshot)
	if err != nil {
		return err
	}
	for index := start + 1; index < len(state.Plan.History); index++ {
		history := state.Plan.History[index]
		var installed *admittedPlanProposal
		for proposalIndex := range proposals {
			candidate := &proposals[proposalIndex]
			if candidate.plan.Digest() != history.Plan.Digest() ||
				candidate.plan.Metadata().Revision != history.Revision {
				continue
			}
			if installed != nil {
				return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
			}
			installed = candidate
		}
		if installed == nil {
			return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
		}
		if err := validateCaptainInstalledProvenance(engine, manifest, snapshot, delegation, *installed, history.OID); err != nil {
			return err
		}
	}
	return nil
}

func allCaptainPlanProposals(manifest admittedManifest, snapshot journal.Snapshot) ([]admittedPlanProposal, error) {
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		if _, duplicate := effects[effect.ID]; duplicate {
			return nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effects[effect.ID] = effect
	}
	result := make([]admittedPlanProposal, 0)
	for _, command := range snapshot.Commands {
		if command.Kind != "planner_proposal" {
			continue
		}
		proposal, err := admitPlanProposal(manifest, command, commands, effects)
		if err != nil {
			return nil, err
		}
		result = append(result, proposal)
	}
	return result, nil
}

func validateCaptainInstalledProvenance(
	engine *engine,
	manifest admittedManifest,
	snapshot journal.Snapshot,
	delegation CaptainDelegationState,
	proposal admittedPlanProposal,
	planOID string,
) error {
	admitted := AdmittedCaptainDelegation{Envelope: delegation.Envelope, Bytes: delegation.EnvelopeBytes, Digest: delegation.Digest}
	expectedApproval, err := approvalCommandForDelegatedProposal(manifest, proposal, admitted)
	if err != nil {
		return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", err)
	}
	expectedApprovalReplay, expectedApprovalEffect, _, err := approvalIdentity(expectedApproval)
	if err != nil {
		return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", err)
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects[effect.ID] = effect
	}
	approvals := 0
	decisions := 0
	installs := 0
	for _, command := range snapshot.Commands {
		effect, ok := effects[command.ReplayKey]
		if command.Kind == "approval" {
			approval, parseErr := parseApprovalCommand(command)
			if parseErr == nil && reflect.DeepEqual(approval, expectedApproval) &&
				command.ReplayKey == expectedApprovalReplay && effect.ID == expectedApprovalEffect &&
				ok && effect.Kind == approvalEffectKind && effect.State == journal.Succeeded {
				approvals++
			}
		}
		if command.Kind == "captain_decision" {
			var decision CaptainDecisionCommand
			canonical, canonicalErr := func() ([]byte, error) {
				if json.Unmarshal(command.Payload, &decision) != nil {
					return nil, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				return CanonicalCaptainDecisionCommand(decision)
			}()
			if canonicalErr == nil && bytes.Equal(canonical, command.Payload) &&
				decision.Outcome == "proceed" && decision.ProposalReplayKey == proposal.replayKey &&
				decision.EnvelopeDigest == delegation.Digest && decision.EnvelopeEpoch == delegation.Epoch &&
				decision.ChildReplayKey == expectedApprovalReplay && decision.ChildEffectID == expectedApprovalEffect &&
				ok && effect.Kind == captainDecisionEffectKind && effect.State == journal.Succeeded {
				decisions++
			}
		}
	}
	work := proposalInstallWork(proposal)
	for _, effect := range snapshot.Effects {
		if effect.Kind != "baton.install" || effect.State != journal.Succeeded || effect.BeforeDigest != work {
			continue
		}
		command, ok := func() (journal.Command, bool) {
			for _, stored := range snapshot.Commands {
				if stored.ReplayKey == effect.ReplayKey {
					return stored, true
				}
			}
			return journal.Command{}, false
		}()
		if !ok {
			return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
		}
		persisted, parseErr := parseActionCommand(command.Payload)
		result, validateErr := validateSucceededBatonAction(engine, command, effect, persisted)
		if parseErr != nil || validateErr != nil || result.Plan != planOID {
			return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", errors.Join(parseErr, validateErr))
		}
		installs++
	}
	if approvals != 1 || decisions != 1 || installs != 1 {
		return runtimeFail("CAPTAIN_RELEASE_LINEAGE_REFUSED", nil)
	}
	return nil
}
