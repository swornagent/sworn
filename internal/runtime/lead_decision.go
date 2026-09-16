package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	LeadDecisionCommandVersion = "sworn.lead-decision-command/v1"
	LeadDecisionResultVersion  = "sworn.lead-decision-result/v1"
	LeadDecisionEventVersion   = "sworn.lead-plan-decision-event/v1"
	LeadRefusalEventVersion    = "sworn.lead-plan-refusal-event/v1"
	leadDecisionEffectKind     = "lead.decision"
	MaxLeadDecisionDetailBytes = 1_048_576
)

type LeadDecisionCommand struct {
	SchemaVersion       string `json:"schema_version"`
	RunID               string `json:"run_id"`
	ManifestDigest      string `json:"manifest_digest"`
	Project             string `json:"project"`
	Release             string `json:"release"`
	ReleaseRef          string `json:"release_ref"`
	ReleaseHead         string `json:"release_head"`
	TargetRef           string `json:"target_ref"`
	TargetHead          string `json:"target_head"`
	ProposalReplayKey   string `json:"proposal_replay_key"`
	ProposalByteCount   int64  `json:"proposal_byte_count"`
	PlanDigest          string `json:"plan_digest"`
	PlanRevision        int64  `json:"plan_revision"`
	PriorPlan           string `json:"prior_plan"`
	DecisionClass       string `json:"decision_class"`
	PlannerAttempt      int64  `json:"planner_attempt"`
	PlannerSourceWork   string `json:"planner_source_work"`
	PlannerSourceEffect string `json:"planner_source_effect"`
	EnvelopeDigest      string `json:"envelope_digest"`
	EnvelopeEpoch       int64  `json:"envelope_epoch"`
	LeadInvocationID    string `json:"lead_invocation_id"`
	LeadWorkID          string `json:"lead_work_id"`
	LeadAttempt         int64  `json:"lead_attempt"`
	SealedHandoffDigest string `json:"sealed_handoff_digest"`
	Outcome             string `json:"outcome"`
	Summary             string `json:"summary"`
	Detail              string `json:"detail"`
	ChildReplayKey      string `json:"child_replay_key"`
	ChildEffectID       string `json:"child_effect_id"`
}

type LeadDecisionResult struct {
	SchemaVersion     string `json:"schema_version"`
	ReplayKey         string `json:"replay_key"`
	EffectID          string `json:"effect_id"`
	Outcome           string `json:"outcome"`
	ProposalReplayKey string `json:"proposal_replay_key"`
	ChildReplayKey    string `json:"child_replay_key"`
	ChildEffectID     string `json:"child_effect_id"`
	State             string `json:"state"`
}

type LeadDecisionEvent struct {
	SchemaVersion     string `json:"schema_version"`
	RunID             string `json:"run_id"`
	Project           string `json:"project"`
	Release           string `json:"release"`
	DecisionClass     string `json:"decision_class"`
	Outcome           string `json:"outcome"`
	ProposalReplayKey string `json:"proposal_replay_key"`
	PlanDigest        string `json:"plan_digest"`
	PlanRevision      int64  `json:"plan_revision"`
	ReleaseHead       string `json:"release_head"`
	TargetHead        string `json:"target_head"`
	EnvelopeDigest    string `json:"envelope_digest"`
	EnvelopeEpoch     int64  `json:"envelope_epoch"`
	DecisionReplayKey string `json:"decision_replay_key"`
	Summary           string `json:"summary"`
	NextAction        string `json:"next_action"`
}

type LeadRefusalEvent struct {
	SchemaVersion     string `json:"schema_version"`
	RunID             string `json:"run_id"`
	Project           string `json:"project"`
	Release           string `json:"release"`
	Code              string `json:"code"`
	ProposalReplayKey string `json:"proposal_replay_key"`
	PlanDigest        string `json:"plan_digest"`
	PlanRevision      int64  `json:"plan_revision"`
	EnvelopeDigest    string `json:"envelope_digest"`
	EnvelopeEpoch     int64  `json:"envelope_epoch"`
	NextAction        string `json:"next_action"`
}

type LeadPlannerContinuationCommand struct {
	SchemaVersion               string `json:"schema_version"`
	RunID                       string `json:"run_id"`
	DecisionReplayKey           string `json:"decision_replay_key"`
	SupersededProposalReplayKey string `json:"superseded_proposal_replay_key"`
	PlanRevision                int64  `json:"plan_revision"`
	PlannerAttempt              int64  `json:"planner_attempt"`
	EnvelopeDigest              string `json:"envelope_digest"`
	EnvelopeEpoch               int64  `json:"envelope_epoch"`
}

func newLeadDecisionCommand(
	manifest admittedManifest,
	proposal admittedPlanProposal,
	delegation LeadDelegationState,
	submission driver.Submission,
	leadWorkID string,
	leadAttempt int64,
) (LeadDecisionCommand, error) {
	if submission.Responsibility != driver.LeadPlanReview || submission.Decision == nil {
		return LeadDecisionCommand{}, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	class, err := approvalDecisionClass(proposal)
	if err != nil {
		return LeadDecisionCommand{}, err
	}
	encoded, err := driver.EncodeSubmission(submission)
	if err != nil {
		return LeadDecisionCommand{}, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", err)
	}
	metadata := proposal.plan.Metadata()
	command := LeadDecisionCommand{
		SchemaVersion: LeadDecisionCommandVersion, RunID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Project: manifest.value.Authority.Project, Release: manifest.value.Release, ReleaseRef: proposal.authority.ReleaseRef, ReleaseHead: proposal.authority.ReleaseHead,
		TargetRef: proposal.authority.TargetRef, TargetHead: proposal.authority.TargetHead, ProposalReplayKey: proposal.replayKey, ProposalByteCount: int64(len(proposal.plan.Bytes())),
		PlanDigest: proposal.plan.Digest(), PlanRevision: metadata.Revision, DecisionClass: class, PlannerAttempt: proposal.authority.PlannerAttempt, PlannerSourceWork: proposal.authority.SourceWork,
		PlannerSourceEffect: proposal.authority.SourceEffect, EnvelopeDigest: delegation.Digest, EnvelopeEpoch: delegation.Epoch, LeadInvocationID: submission.InvocationID,
		LeadWorkID: leadWorkID, LeadAttempt: leadAttempt, SealedHandoffDigest: sha256Digest(encoded), Outcome: string(submission.Decision.Outcome), Summary: submission.Summary, Detail: submission.Detail,
	}
	if command.PlannerAttempt == 0 {
		command.PlannerAttempt = 1
	}
	if metadata.PreviousPlan != nil {
		command.PriorPlan = *metadata.PreviousPlan
	}
	if command.Outcome == "proceed" {
		admitted := AdmittedLeadDelegation{Envelope: delegation.Envelope, Bytes: delegation.EnvelopeBytes, Digest: delegation.Digest}
		approval, err := approvalCommandForDelegatedProposal(manifest, proposal, admitted)
		if err != nil {
			return LeadDecisionCommand{}, err
		}
		command.ChildReplayKey, command.ChildEffectID, _, err = approvalIdentity(approval)
		if err != nil {
			return LeadDecisionCommand{}, err
		}
	} else if command.Outcome == "revise" {
		seed := command
		seed.ChildReplayKey = ""
		seed.ChildEffectID = ""
		body := mustJSON(seed)
		suffix := strings.TrimPrefix(sha256Digest(body), "sha256:")
		command.ChildReplayKey = "planner-continuation/" + suffix
		command.ChildEffectID = command.ChildReplayKey
	}
	if _, err := CanonicalLeadDecisionCommand(command); err != nil {
		return LeadDecisionCommand{}, err
	}
	return command, nil
}

func CanonicalLeadDecisionCommand(command LeadDecisionCommand) ([]byte, error) {
	if command.SchemaVersion != LeadDecisionCommandVersion || !runtimeIdentityPattern.MatchString(command.RunID) || !runtimeDigestPattern.MatchString(command.ManifestDigest) ||
		command.Project == "" || command.Release == "" || !strings.HasPrefix(command.ReleaseRef, "refs/heads/release-wt/") || !strings.HasPrefix(command.TargetRef, "refs/heads/") ||
		!validGitObjectID(command.TargetHead) || command.ProposalReplayKey == "" || command.ProposalByteCount < 1 || command.ProposalByteCount > 1_048_576 || !runtimeDigestPattern.MatchString(command.PlanDigest) || command.PlanRevision < 1 ||
		(command.DecisionClass != PlannerProposalClass && command.DecisionClass != PlannerReplanClass) || command.PlannerAttempt < 1 || command.PlannerSourceWork == "" || command.PlannerSourceEffect == "" ||
		!runtimeDigestPattern.MatchString(command.EnvelopeDigest) || command.EnvelopeEpoch < 1 || command.LeadInvocationID == "" || !runtimeDigestPattern.MatchString(command.LeadWorkID) || command.LeadAttempt < 1 || !runtimeDigestPattern.MatchString(command.SealedHandoffDigest) ||
		(command.Outcome != "proceed" && command.Outcome != "revise" && command.Outcome != "escalate") || !validLeadDecisionText(command.Summary, driver.MaxSubmissionSummaryBytes) || !validLeadDecisionText(command.Detail, MaxLeadDecisionDetailBytes) {
		return nil, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	if command.PlanRevision == 1 {
		if command.PriorPlan != "" || command.ReleaseHead != "" || command.DecisionClass != PlannerProposalClass {
			return nil, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
		}
	} else if command.PriorPlan == "" || !validGitObjectID(command.ReleaseHead) || command.DecisionClass != PlannerReplanClass {
		return nil, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	if command.Outcome == "escalate" {
		if command.ChildReplayKey != "" || command.ChildEffectID != "" {
			return nil, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
		}
	} else if command.ChildReplayKey == "" || command.ChildEffectID == "" {
		return nil, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	return json.Marshal(command)
}

func validLeadDecisionText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func leadDecisionIdentity(command LeadDecisionCommand) (string, string, []byte, error) {
	body, err := CanonicalLeadDecisionCommand(command)
	if err != nil {
		return "", "", nil, err
	}
	digest := sha256Digest(body)
	suffix := strings.TrimPrefix(digest, "sha256:")
	return "lead-decision/" + suffix, "lead-decision/" + suffix, body, nil
}

func leadDecisionResult(command LeadDecisionCommand) (LeadDecisionResult, []byte, error) {
	replay, effect, _, err := leadDecisionIdentity(command)
	if err != nil {
		return LeadDecisionResult{}, nil, err
	}
	value := LeadDecisionResult{SchemaVersion: LeadDecisionResultVersion, ReplayKey: replay, EffectID: effect, Outcome: command.Outcome, ProposalReplayKey: command.ProposalReplayKey, ChildReplayKey: command.ChildReplayKey, ChildEffectID: command.ChildEffectID, State: "succeeded"}
	return value, mustJSON(value), nil
}

// LeadDecisionNotificationText is the closed, engine-owned informational
// projection for Lead decisions. Model-authored summary and detail remain in
// the bounded local command record and must never be copied into this event.
// Adding a decision class or outcome therefore requires an explicit safe
// notification mapping before completion or external projection can succeed.
func LeadDecisionNotificationText(decisionClass, outcome string) (summary, nextAction string, ok bool) {
	switch decisionClass {
	case PlannerProposalClass:
		switch outcome {
		case "proceed":
			return "Lead authorized the exact Planner proposal.", "install_approved_plan", true
		case "revise":
			return "Lead requested a bounded revision of the Planner proposal.", "request_planner_revision", true
		case "escalate":
			return "Lead escalated the Planner proposal to external authority.", "await_external_authority", true
		}
	case PlannerReplanClass:
		switch outcome {
		case "proceed":
			return "Lead authorized the exact Planner replan.", "install_approved_plan", true
		case "revise":
			return "Lead requested another bounded Planner replan.", "request_planner_revision", true
		case "escalate":
			return "Lead escalated the Planner replan to external authority.", "await_external_authority", true
		}
	}
	return "", "", false
}

func leadDecisionEvent(command LeadDecisionCommand, replay string) ([]byte, error) {
	summary, next, ok := LeadDecisionNotificationText(command.DecisionClass, command.Outcome)
	if !ok {
		return nil, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	return mustJSON(LeadDecisionEvent{SchemaVersion: LeadDecisionEventVersion, RunID: command.RunID, Project: command.Project, Release: command.Release, DecisionClass: command.DecisionClass, Outcome: command.Outcome, ProposalReplayKey: command.ProposalReplayKey, PlanDigest: command.PlanDigest, PlanRevision: command.PlanRevision, ReleaseHead: command.ReleaseHead, TargetHead: command.TargetHead, EnvelopeDigest: command.EnvelopeDigest, EnvelopeEpoch: command.EnvelopeEpoch, DecisionReplayKey: replay, Summary: summary, NextAction: next}), nil
}

func leadRefusalEvent(manifest admittedManifest, proposal admittedPlanProposal, delegation LeadDelegationState, code string) []byte {
	return mustJSON(LeadRefusalEvent{
		SchemaVersion: LeadRefusalEventVersion,
		RunID:         manifest.value.RunID, Project: manifest.value.Authority.Project,
		Release: manifest.value.Release, Code: code,
		ProposalReplayKey: proposal.replayKey, PlanDigest: proposal.plan.Digest(),
		PlanRevision:   proposal.plan.Metadata().Revision,
		EnvelopeDigest: delegation.Digest, EnvelopeEpoch: delegation.Epoch,
		NextAction: "await_external_authority",
	})
}

func (s *Service) appendLeadRefusal(ctx context.Context, manifest admittedManifest, proposal admittedPlanProposal, delegation LeadDelegationState, code string) error {
	body := leadRefusalEvent(manifest, proposal, delegation, code)
	digest := sha256Digest(body)
	if err := s.journal.AppendEventOnce(ctx, journal.Command{
		RunID:     manifest.value.RunID,
		ReplayKey: "lead-refusal/" + strings.TrimPrefix(digest, "sha256:"),
		Kind:      "lead_refusal", Payload: body, CreatedAt: s.now().UTC(),
	}, "lead_plan_refused", body, s.now().UTC()); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func leadHumanAuthorityRequired(snapshot journal.Snapshot, proposal admittedPlanProposal, delegation LeadDelegationState) (bool, error) {
	if delegation.Epoch > 0 && !delegation.Active {
		return true, nil
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects[effect.ReplayKey] = effect
	}
	for _, stored := range snapshot.Commands {
		if stored.Kind != "lead_decision" {
			continue
		}
		effect, ok := effects[stored.ReplayKey]
		if !ok || effect.Kind != leadDecisionEffectKind || effect.State != journal.Succeeded {
			continue
		}
		var decision LeadDecisionCommand
		if json.Unmarshal(stored.Payload, &decision) != nil {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		canonical, err := CanonicalLeadDecisionCommand(decision)
		if err != nil || !bytes.Equal(canonical, stored.Payload) {
			return false, runtimeFail("CORRUPT_JOURNAL", err)
		}
		if decision.ProposalReplayKey == proposal.replayKey && decision.Outcome == "escalate" &&
			decision.EnvelopeDigest == delegation.Digest && decision.EnvelopeEpoch == delegation.Epoch {
			return true, nil
		}
	}
	for _, event := range snapshot.Events {
		if event.Kind != "lead_plan_refused" {
			continue
		}
		var refusal LeadRefusalEvent
		if json.Unmarshal(event.Body, &refusal) != nil || !bytes.Equal(event.Body, mustJSON(refusal)) {
			continue
		}
		if refusal.SchemaVersion == LeadRefusalEventVersion && refusal.ProposalReplayKey == proposal.replayKey &&
			refusal.PlanDigest == proposal.plan.Digest() && refusal.EnvelopeDigest == delegation.Digest &&
			refusal.EnvelopeEpoch == delegation.Epoch {
			return true, nil
		}
	}
	return false, nil
}

func validateLeadDecisionCurrent(manifest admittedManifest, proposal admittedPlanProposal, snapshot journal.Snapshot, command LeadDecisionCommand) (LeadDelegationState, error) {
	state, err := currentLeadDelegation(snapshot)
	if err != nil {
		return state, err
	}
	metadata := proposal.plan.Metadata()
	if !state.Active || state.Digest != command.EnvelopeDigest || state.Epoch != command.EnvelopeEpoch || state.Decisions >= state.Envelope.Limits.MaximumTotalLeadDecisions ||
		(command.Outcome == "revise" && state.ReplanSpent >= state.Envelope.Limits.ReplanBudget) || command.RunID != manifest.value.RunID || command.ManifestDigest != manifest.digest || command.Project != manifest.value.Authority.Project || command.Release != manifest.value.Release || command.ReleaseRef != proposal.authority.ReleaseRef || command.ReleaseHead != proposal.authority.ReleaseHead || command.TargetRef != proposal.authority.TargetRef || command.TargetHead != proposal.authority.TargetHead || command.ProposalReplayKey != proposal.replayKey || command.ProposalByteCount != int64(len(proposal.plan.Bytes())) || command.PlanDigest != proposal.plan.Digest() || command.PlanRevision != metadata.Revision || command.PriorPlan != proposal.authority.PriorPlan || command.PlannerAttempt != max(proposal.authority.PlannerAttempt, 1) || command.PlannerSourceWork != proposal.authority.SourceWork || command.PlannerSourceEffect != proposal.authority.SourceEffect {
		return state, runtimeFail("LEAD_DECISION_STALE", nil)
	}
	if err := validateLeadDecisionDispatch(snapshot, command, state.Envelope.Limits.MaximumLeadAttemptsPerProposal); err != nil {
		return state, err
	}
	class, err := approvalDecisionClass(proposal)
	if err != nil || class != command.DecisionClass {
		return state, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", err)
	}
	allowed := false
	for _, rule := range state.Envelope.DecisionRules {
		if rule.DecisionClass == class {
			for _, outcome := range rule.AllowedOutcomes {
				allowed = allowed || outcome == command.Outcome
			}
		}
	}
	if !allowed {
		return state, runtimeFail("LEAD_DECISION_AUTHORITY_REFUSED", nil)
	}
	if err := ValidateLeadPlanPolicy(state.Envelope.PlanRules, proposal.plan, nil); err != nil {
		return state, runtimeFail("LEAD_DECISION_AUTHORITY_REFUSED", err)
	}
	return state, nil
}

func leadDispatchAttemptCount(snapshot journal.Snapshot, workID string) (int64, error) {
	if !runtimeDigestPattern.MatchString(workID) {
		return 0, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	prefix := "attempt/" + strings.TrimPrefix(workID, "sha256:") + "/"
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return 0, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	var count int64
	for _, effect := range snapshot.Effects {
		if !strings.HasPrefix(effect.ID, prefix) {
			continue
		}
		command, ok := commands[effect.ReplayKey]
		if !ok || effect.ID != effect.ReplayKey || command.ReplayKey != effect.ReplayKey ||
			command.Kind != "driver.dispatch" || effect.Kind != "driver.dispatch" {
			return 0, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		count++
	}
	return count, nil
}

func leadDispatchAttemptForSubmission(snapshot journal.Snapshot, workID string, submission driver.Submission) (int64, error) {
	count, err := leadDispatchAttemptCount(snapshot, workID)
	if err != nil {
		return 0, err
	}
	prefix := "attempt/" + strings.TrimPrefix(workID, "sha256:") + "/"
	found := int64(0)
	for _, effect := range snapshot.Effects {
		if !strings.HasPrefix(effect.ID, prefix) || effect.State != journal.Succeeded {
			continue
		}
		decoded, decodeErr := driver.DecodeSubmission(effect.Result)
		if decodeErr == nil && decoded.InvocationID == submission.InvocationID &&
			decoded.Responsibility == driver.LeadPlanReview && bytes.Equal(effect.Result, mustEncodeSubmission(submission)) {
			if found != 0 {
				return 0, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			found = count
		}
	}
	if found == 0 {
		return 0, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	return found, nil
}

func mustEncodeSubmission(submission driver.Submission) []byte {
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		return nil
	}
	return body
}

func validateLeadDecisionDispatch(snapshot journal.Snapshot, command LeadDecisionCommand, maximum int64) error {
	if command.LeadAttempt < 1 || command.LeadAttempt > maximum {
		return runtimeFail("LEAD_DECISION_AUTHORITY_REFUSED", nil)
	}
	prefix := "attempt/" + strings.TrimPrefix(command.LeadWorkID, "sha256:") + "/"
	count, err := leadDispatchAttemptCount(snapshot, command.LeadWorkID)
	if err != nil {
		return err
	}
	found := false
	for _, effect := range snapshot.Effects {
		if !strings.HasPrefix(effect.ID, prefix) || effect.State != journal.Succeeded {
			continue
		}
		submission, decodeErr := driver.DecodeSubmission(effect.Result)
		if decodeErr != nil || submission.Responsibility != driver.LeadPlanReview {
			return runtimeFail("CORRUPT_JOURNAL", decodeErr)
		}
		if submission.InvocationID != command.LeadInvocationID ||
			sha256Digest(effect.Result) != command.SealedHandoffDigest {
			continue
		}
		if found || count != command.LeadAttempt {
			return runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
		}
		found = true
	}
	if !found {
		return runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
	}
	return nil
}

func (s *Service) LeadDecide(ctx context.Context, command LeadDecisionCommand) (LeadDecisionResult, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return LeadDecisionResult{}, runtimeFail("INVALID_SERVICE", nil)
	}
	payload, err := CanonicalLeadDecisionCommand(command)
	if err != nil {
		return LeadDecisionResult{}, err
	}
	manifest, proposal, snapshot, err := s.currentApprovalProposal(ctx, command.RunID)
	if err != nil {
		return LeadDecisionResult{}, err
	}
	if err = s.validateLeadReleaseLineage(ctx, manifest, proposal, snapshot); err != nil {
		return LeadDecisionResult{}, err
	}
	if _, err = validateLeadDecisionCurrent(manifest, proposal, snapshot, command); err != nil {
		return LeadDecisionResult{}, err
	}
	replay, effectID, _, _ := leadDecisionIdentity(command)
	result, resultBody, _ := leadDecisionResult(command)
	now := s.now().UTC()
	if err = s.journal.RecordCommandEffect(ctx, journal.Command{RunID: command.RunID, ReplayKey: replay, Kind: "lead_decision", Payload: payload, CreatedAt: now}, journal.Effect{RunID: command.RunID, ID: effectID, ReplayKey: replay, Kind: leadDecisionEffectKind, BeforeDigest: sha256Digest(payload), ExpectedDigest: sha256Digest(resultBody), UpdatedAt: now}); err != nil {
		return LeadDecisionResult{}, runtimeFail("LEAD_DECISION_REPLAY_CONFLICT", err)
	}
	effect, err := s.journal.Effect(ctx, command.RunID, effectID)
	if err != nil {
		return LeadDecisionResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		if !bytes.Equal(effect.Result, resultBody) {
			return LeadDecisionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if command.Outcome == "proceed" {
			stored, readErr := s.journal.Snapshot(ctx, command.RunID)
			if readErr != nil {
				return LeadDecisionResult{}, runtimeFail("JOURNAL_READ_FAILED", readErr)
			}
			for _, child := range stored.Commands {
				if child.ReplayKey != command.ChildReplayKey || child.Kind != "approval" {
					continue
				}
				approval, parseErr := parseApprovalCommand(child)
				if parseErr != nil {
					return LeadDecisionResult{}, parseErr
				}
				if _, approvalErr := s.Approve(ctx, approval); approvalErr != nil {
					return LeadDecisionResult{}, approvalErr
				}
				return result, nil
			}
			return LeadDecisionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return result, nil
	}
	if testLeadCrashCut == "decision_admission" {
		return LeadDecisionResult{}, runtimeFail("TEST_LEAD_CRASH_CUT", nil)
	}
	if effect.State == journal.Pending {
		claim, claimErr := s.journal.Claim(ctx, command.RunID, effectID, now, effectLease)
		if claimErr != nil {
			return LeadDecisionResult{}, runtimeFail("LEAD_DECISION_RECOVERY_PENDING", claimErr)
		}
		effect.CurrentClaim = claim.Token
	}
	if testLeadCrashCut == "decision_claim" {
		return LeadDecisionResult{}, runtimeFail("TEST_LEAD_CRASH_CUT", nil)
	}
	freshManifest, freshProposal, fresh, err := s.currentApprovalProposal(ctx, command.RunID)
	if err != nil {
		return LeadDecisionResult{}, err
	}
	if err = s.validateLeadReleaseLineage(ctx, freshManifest, freshProposal, fresh); err != nil {
		_ = s.failLeadDecisionClaim(ctx, command, effect, "LEAD_RELEASE_LINEAGE_REFUSED")
		return LeadDecisionResult{}, err
	}
	if _, err = validateLeadDecisionCurrent(freshManifest, freshProposal, fresh, command); err != nil {
		_ = s.failLeadDecisionClaim(ctx, command, effect, "LEAD_DECISION_STALE")
		return LeadDecisionResult{}, err
	}
	offset := int64(0)
	if len(fresh.Events) > 0 {
		offset = fresh.Events[len(fresh.Events)-1].Offset
	}
	eventBody, eventErr := leadDecisionEvent(command, replay)
	if eventErr != nil {
		return LeadDecisionResult{}, eventErr
	}
	completion := journal.Completion{RunID: command.RunID, EffectID: effectID, Token: effect.CurrentClaim, State: journal.Succeeded, Result: resultBody, EventKind: "lead_plan_decided", EventBody: eventBody, At: now, ExpectedEventOffset: &offset}
	var childCommand *journal.Command
	var childEffect *journal.Effect
	if command.Outcome == "proceed" {
		active, _ := ParseLeadDelegation(freshStateEnvelopeBytes(fresh, command.EnvelopeDigest))
		approval, approvalErr := approvalCommandForDelegatedProposal(freshManifest, freshProposal, active)
		if approvalErr != nil {
			return LeadDecisionResult{}, approvalErr
		}
		childReplay, childID, childPayload, _ := approvalIdentity(approval)
		_, childResult, _ := canonicalApprovalResult(approval)
		if childReplay != command.ChildReplayKey || childID != command.ChildEffectID {
			return LeadDecisionResult{}, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
		}
		childCommand = &journal.Command{RunID: command.RunID, ReplayKey: childReplay, Kind: "approval", Payload: childPayload, CreatedAt: now}
		childEffect = &journal.Effect{RunID: command.RunID, ID: childID, ReplayKey: childReplay, Kind: approvalEffectKind, BeforeDigest: sha256Digest(childPayload), ExpectedDigest: sha256Digest(childResult), UpdatedAt: now}
	} else if command.Outcome == "revise" {
		continuation := LeadPlannerContinuationCommand{SchemaVersion: "sworn.lead-planner-continuation/v1", RunID: command.RunID, DecisionReplayKey: replay, SupersededProposalReplayKey: command.ProposalReplayKey, PlanRevision: command.PlanRevision, PlannerAttempt: command.PlannerAttempt + 1, EnvelopeDigest: command.EnvelopeDigest, EnvelopeEpoch: command.EnvelopeEpoch}
		body := mustJSON(continuation)
		seed := command
		seed.ChildReplayKey = ""
		seed.ChildEffectID = ""
		suffix := strings.TrimPrefix(sha256Digest(mustJSON(seed)), "sha256:")
		if command.ChildReplayKey != "planner-continuation/"+suffix || command.ChildEffectID != "planner-continuation/"+suffix {
			return LeadDecisionResult{}, runtimeFail("LEAD_DECISION_BINDING_MISMATCH", nil)
		}
		childCommand = &journal.Command{RunID: command.RunID, ReplayKey: command.ChildReplayKey, Kind: "planner_continuation", Payload: body, CreatedAt: now}
		childEffect = &journal.Effect{RunID: command.RunID, ID: command.ChildEffectID, ReplayKey: command.ChildReplayKey, Kind: "planner.continue", BeforeDigest: sha256Digest(body), ExpectedDigest: sha256Digest([]byte("scheduled")), UpdatedAt: now}
	}
	if err = s.journal.CompleteWithChild(ctx, completion, childCommand, childEffect); err != nil {
		if journal.IsCode(err, "STALE_COMPLETION") {
			_ = s.failLeadDecisionClaim(ctx, command, effect, "LEAD_DECISION_STALE")
			return LeadDecisionResult{}, runtimeFail("LEAD_DECISION_STALE", err)
		}
		return LeadDecisionResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	if testLeadCrashCut == "decision_completion" ||
		(testLeadCrashCut == "revise_completion" && command.Outcome == "revise") {
		return LeadDecisionResult{}, runtimeFail("TEST_LEAD_CRASH_CUT", nil)
	}
	if command.Outcome == "proceed" {
		approvalCommand, parseErr := parseApprovalCommand(*childCommand)
		if parseErr != nil {
			return LeadDecisionResult{}, parseErr
		}
		if _, approvalErr := s.Approve(ctx, approvalCommand); approvalErr != nil {
			return LeadDecisionResult{}, approvalErr
		}
	}
	return result, nil
}

func (s *Service) failLeadDecisionClaim(ctx context.Context, command LeadDecisionCommand, effect journal.Effect, code string) error {
	current, err := s.journal.Effect(ctx, command.RunID, effect.ID)
	if err != nil {
		return err
	}
	if current.State == journal.Pending {
		claim, claimErr := s.journal.Claim(ctx, command.RunID, effect.ID, s.now().UTC(), effectLease)
		if claimErr != nil {
			return claimErr
		}
		current.CurrentClaim = claim.Token
	} else if current.State != journal.Claimed {
		return nil
	}
	snapshot, err := s.journal.Snapshot(ctx, command.RunID)
	if err != nil {
		return err
	}
	offset := int64(0)
	if len(snapshot.Events) > 0 {
		offset = snapshot.Events[len(snapshot.Events)-1].Offset
	}
	return s.journal.Complete(ctx, journal.Completion{RunID: command.RunID, EffectID: effect.ID, Token: current.CurrentClaim, State: journal.OperationalFailed, ErrorCode: code, EventKind: "lead_plan_refused", EventBody: []byte(code), At: s.now().UTC(), ExpectedEventOffset: &offset})
}

func freshStateEnvelopeBytes(snapshot journal.Snapshot, digest string) []byte {
	state, err := currentLeadDelegation(snapshot)
	if err != nil || state.Digest != digest {
		return nil
	}
	return state.EnvelopeBytes
}

func (s *Service) ReconcileLeadDecisions(ctx context.Context, runID string) error {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return runtimeFail("RUN_NOT_FOUND", err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != leadDecisionEffectKind || (effect.State != journal.Pending && effect.State != journal.Claimed) {
			continue
		}
		stored, ok := commands[effect.ReplayKey]
		if !ok {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		var command LeadDecisionCommand
		if json.Unmarshal(stored.Payload, &command) != nil {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		canonical, canonicalErr := CanonicalLeadDecisionCommand(command)
		if canonicalErr != nil || !bytes.Equal(canonical, stored.Payload) {
			return runtimeFail("CORRUPT_JOURNAL", canonicalErr)
		}
		if _, err := s.LeadDecide(ctx, command); err != nil && !IsCode(err, "LEAD_DECISION_STALE") {
			return err
		}
	}
	return nil
}
