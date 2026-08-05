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
	CaptainDecisionCommandVersion = "sworn.captain-decision-command/v1"
	CaptainDecisionResultVersion  = "sworn.captain-decision-result/v1"
	CaptainDecisionEventVersion   = "sworn.captain-plan-decision-event/v1"
	CaptainRefusalEventVersion    = "sworn.captain-plan-refusal-event/v1"
	captainDecisionEffectKind     = "captain.decision"
	MaxCaptainDecisionDetailBytes = 4096
)

type CaptainDecisionCommand struct {
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
	CaptainInvocationID string `json:"captain_invocation_id"`
	CaptainWorkID       string `json:"captain_work_id"`
	CaptainAttempt      int64  `json:"captain_attempt"`
	SealedHandoffDigest string `json:"sealed_handoff_digest"`
	Outcome             string `json:"outcome"`
	Summary             string `json:"summary"`
	Detail              string `json:"detail"`
	ChildReplayKey      string `json:"child_replay_key"`
	ChildEffectID       string `json:"child_effect_id"`
}

type CaptainDecisionResult struct {
	SchemaVersion     string `json:"schema_version"`
	ReplayKey         string `json:"replay_key"`
	EffectID          string `json:"effect_id"`
	Outcome           string `json:"outcome"`
	ProposalReplayKey string `json:"proposal_replay_key"`
	ChildReplayKey    string `json:"child_replay_key"`
	ChildEffectID     string `json:"child_effect_id"`
	State             string `json:"state"`
}

type CaptainDecisionEvent struct {
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

type CaptainRefusalEvent struct {
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

type CaptainPlannerContinuationCommand struct {
	SchemaVersion               string `json:"schema_version"`
	RunID                       string `json:"run_id"`
	DecisionReplayKey           string `json:"decision_replay_key"`
	SupersededProposalReplayKey string `json:"superseded_proposal_replay_key"`
	PlanRevision                int64  `json:"plan_revision"`
	PlannerAttempt              int64  `json:"planner_attempt"`
	EnvelopeDigest              string `json:"envelope_digest"`
	EnvelopeEpoch               int64  `json:"envelope_epoch"`
}

func newCaptainDecisionCommand(
	manifest admittedManifest,
	proposal admittedPlanProposal,
	delegation CaptainDelegationState,
	submission driver.Submission,
	captainWorkID string,
	captainAttempt int64,
) (CaptainDecisionCommand, error) {
	if submission.Responsibility != driver.CaptainPlanReview || submission.Decision == nil {
		return CaptainDecisionCommand{}, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
	}
	class, err := approvalDecisionClass(proposal)
	if err != nil {
		return CaptainDecisionCommand{}, err
	}
	encoded, err := driver.EncodeSubmission(submission)
	if err != nil {
		return CaptainDecisionCommand{}, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", err)
	}
	metadata := proposal.plan.Metadata()
	command := CaptainDecisionCommand{
		SchemaVersion: CaptainDecisionCommandVersion, RunID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Project: manifest.value.Authority.Project, Release: manifest.value.Release, ReleaseRef: proposal.authority.ReleaseRef, ReleaseHead: proposal.authority.ReleaseHead,
		TargetRef: proposal.authority.TargetRef, TargetHead: proposal.authority.TargetHead, ProposalReplayKey: proposal.replayKey, ProposalByteCount: int64(len(proposal.plan.Bytes())),
		PlanDigest: proposal.plan.Digest(), PlanRevision: metadata.Revision, DecisionClass: class, PlannerAttempt: proposal.authority.PlannerAttempt, PlannerSourceWork: proposal.authority.SourceWork,
		PlannerSourceEffect: proposal.authority.SourceEffect, EnvelopeDigest: delegation.Digest, EnvelopeEpoch: delegation.Epoch, CaptainInvocationID: submission.InvocationID,
		CaptainWorkID: captainWorkID, CaptainAttempt: captainAttempt, SealedHandoffDigest: sha256Digest(encoded), Outcome: string(submission.Decision.Outcome), Summary: submission.Summary, Detail: submission.Detail,
	}
	if command.PlannerAttempt == 0 {
		command.PlannerAttempt = 1
	}
	if metadata.PreviousPlan != nil {
		command.PriorPlan = *metadata.PreviousPlan
	}
	if command.Outcome == "proceed" {
		admitted := AdmittedCaptainDelegation{Envelope: delegation.Envelope, Bytes: delegation.EnvelopeBytes, Digest: delegation.Digest}
		approval, err := approvalCommandForDelegatedProposal(manifest, proposal, admitted)
		if err != nil {
			return CaptainDecisionCommand{}, err
		}
		command.ChildReplayKey, command.ChildEffectID, _, err = approvalIdentity(approval)
		if err != nil {
			return CaptainDecisionCommand{}, err
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
	if _, err := CanonicalCaptainDecisionCommand(command); err != nil {
		return CaptainDecisionCommand{}, err
	}
	return command, nil
}

func CanonicalCaptainDecisionCommand(command CaptainDecisionCommand) ([]byte, error) {
	if command.SchemaVersion != CaptainDecisionCommandVersion || !runtimeIdentityPattern.MatchString(command.RunID) || !runtimeDigestPattern.MatchString(command.ManifestDigest) ||
		command.Project == "" || command.Release == "" || !strings.HasPrefix(command.ReleaseRef, "refs/heads/release-wt/") || !strings.HasPrefix(command.TargetRef, "refs/heads/") ||
		!validGitObjectID(command.TargetHead) || command.ProposalReplayKey == "" || command.ProposalByteCount < 1 || command.ProposalByteCount > 1_048_576 || !runtimeDigestPattern.MatchString(command.PlanDigest) || command.PlanRevision < 1 ||
		(command.DecisionClass != PlannerProposalClass && command.DecisionClass != PlannerReplanClass) || command.PlannerAttempt < 1 || command.PlannerSourceWork == "" || command.PlannerSourceEffect == "" ||
		!runtimeDigestPattern.MatchString(command.EnvelopeDigest) || command.EnvelopeEpoch < 1 || command.CaptainInvocationID == "" || !runtimeDigestPattern.MatchString(command.CaptainWorkID) || command.CaptainAttempt < 1 || !runtimeDigestPattern.MatchString(command.SealedHandoffDigest) ||
		(command.Outcome != "proceed" && command.Outcome != "revise" && command.Outcome != "escalate") || !validCaptainDecisionText(command.Summary, driver.MaxSubmissionSummaryBytes) || !validCaptainDecisionText(command.Detail, MaxCaptainDecisionDetailBytes) {
		return nil, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
	}
	if command.PlanRevision == 1 {
		if command.PriorPlan != "" || command.ReleaseHead != "" || command.DecisionClass != PlannerProposalClass {
			return nil, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
		}
	} else if command.PriorPlan == "" || !validGitObjectID(command.ReleaseHead) || command.DecisionClass != PlannerReplanClass {
		return nil, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
	}
	if command.Outcome == "escalate" {
		if command.ChildReplayKey != "" || command.ChildEffectID != "" {
			return nil, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
		}
	} else if command.ChildReplayKey == "" || command.ChildEffectID == "" {
		return nil, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
	}
	return json.Marshal(command)
}

func validCaptainDecisionText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func captainDecisionIdentity(command CaptainDecisionCommand) (string, string, []byte, error) {
	body, err := CanonicalCaptainDecisionCommand(command)
	if err != nil {
		return "", "", nil, err
	}
	digest := sha256Digest(body)
	suffix := strings.TrimPrefix(digest, "sha256:")
	return "captain-decision/" + suffix, "captain-decision/" + suffix, body, nil
}

func captainDecisionResult(command CaptainDecisionCommand) (CaptainDecisionResult, []byte, error) {
	replay, effect, _, err := captainDecisionIdentity(command)
	if err != nil {
		return CaptainDecisionResult{}, nil, err
	}
	value := CaptainDecisionResult{SchemaVersion: CaptainDecisionResultVersion, ReplayKey: replay, EffectID: effect, Outcome: command.Outcome, ProposalReplayKey: command.ProposalReplayKey, ChildReplayKey: command.ChildReplayKey, ChildEffectID: command.ChildEffectID, State: "succeeded"}
	return value, mustJSON(value), nil
}

// CaptainDecisionNotificationText is the closed, engine-owned informational
// projection for Captain decisions. Model-authored summary and detail remain in
// the bounded local command record and must never be copied into this event.
// Adding a decision class or outcome therefore requires an explicit safe
// notification mapping before completion or external projection can succeed.
func CaptainDecisionNotificationText(decisionClass, outcome string) (summary, nextAction string, ok bool) {
	switch decisionClass {
	case PlannerProposalClass:
		switch outcome {
		case "proceed":
			return "Captain authorized the exact Planner proposal.", "install_approved_plan", true
		case "revise":
			return "Captain requested a bounded revision of the Planner proposal.", "request_planner_revision", true
		case "escalate":
			return "Captain escalated the Planner proposal to external authority.", "await_external_authority", true
		}
	case PlannerReplanClass:
		switch outcome {
		case "proceed":
			return "Captain authorized the exact Planner replan.", "install_approved_plan", true
		case "revise":
			return "Captain requested another bounded Planner replan.", "request_planner_revision", true
		case "escalate":
			return "Captain escalated the Planner replan to external authority.", "await_external_authority", true
		}
	}
	return "", "", false
}

func captainDecisionEvent(command CaptainDecisionCommand, replay string) ([]byte, error) {
	summary, next, ok := CaptainDecisionNotificationText(command.DecisionClass, command.Outcome)
	if !ok {
		return nil, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
	}
	return mustJSON(CaptainDecisionEvent{SchemaVersion: CaptainDecisionEventVersion, RunID: command.RunID, Project: command.Project, Release: command.Release, DecisionClass: command.DecisionClass, Outcome: command.Outcome, ProposalReplayKey: command.ProposalReplayKey, PlanDigest: command.PlanDigest, PlanRevision: command.PlanRevision, ReleaseHead: command.ReleaseHead, TargetHead: command.TargetHead, EnvelopeDigest: command.EnvelopeDigest, EnvelopeEpoch: command.EnvelopeEpoch, DecisionReplayKey: replay, Summary: summary, NextAction: next}), nil
}

func captainRefusalEvent(manifest admittedManifest, proposal admittedPlanProposal, delegation CaptainDelegationState, code string) []byte {
	return mustJSON(CaptainRefusalEvent{
		SchemaVersion: CaptainRefusalEventVersion,
		RunID:         manifest.value.RunID, Project: manifest.value.Authority.Project,
		Release: manifest.value.Release, Code: code,
		ProposalReplayKey: proposal.replayKey, PlanDigest: proposal.plan.Digest(),
		PlanRevision:   proposal.plan.Metadata().Revision,
		EnvelopeDigest: delegation.Digest, EnvelopeEpoch: delegation.Epoch,
		NextAction: "await_external_authority",
	})
}

func (s *Service) appendCaptainRefusal(ctx context.Context, manifest admittedManifest, proposal admittedPlanProposal, delegation CaptainDelegationState, code string) error {
	body := captainRefusalEvent(manifest, proposal, delegation, code)
	digest := sha256Digest(body)
	if err := s.journal.AppendEventOnce(ctx, journal.Command{
		RunID:     manifest.value.RunID,
		ReplayKey: "captain-refusal/" + strings.TrimPrefix(digest, "sha256:"),
		Kind:      "captain_refusal", Payload: body, CreatedAt: s.now().UTC(),
	}, "captain_plan_refused", body, s.now().UTC()); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func captainHumanAuthorityRequired(snapshot journal.Snapshot, proposal admittedPlanProposal, delegation CaptainDelegationState) (bool, error) {
	if delegation.Epoch > 0 && !delegation.Active {
		return true, nil
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects[effect.ReplayKey] = effect
	}
	for _, stored := range snapshot.Commands {
		if stored.Kind != "captain_decision" {
			continue
		}
		effect, ok := effects[stored.ReplayKey]
		if !ok || effect.Kind != captainDecisionEffectKind || effect.State != journal.Succeeded {
			continue
		}
		var decision CaptainDecisionCommand
		if json.Unmarshal(stored.Payload, &decision) != nil {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		canonical, err := CanonicalCaptainDecisionCommand(decision)
		if err != nil || !bytes.Equal(canonical, stored.Payload) {
			return false, runtimeFail("CORRUPT_JOURNAL", err)
		}
		if decision.ProposalReplayKey == proposal.replayKey && decision.Outcome == "escalate" &&
			decision.EnvelopeDigest == delegation.Digest && decision.EnvelopeEpoch == delegation.Epoch {
			return true, nil
		}
	}
	for _, event := range snapshot.Events {
		if event.Kind != "captain_plan_refused" {
			continue
		}
		var refusal CaptainRefusalEvent
		if json.Unmarshal(event.Body, &refusal) != nil || !bytes.Equal(event.Body, mustJSON(refusal)) {
			continue
		}
		if refusal.SchemaVersion == CaptainRefusalEventVersion && refusal.ProposalReplayKey == proposal.replayKey &&
			refusal.PlanDigest == proposal.plan.Digest() && refusal.EnvelopeDigest == delegation.Digest &&
			refusal.EnvelopeEpoch == delegation.Epoch {
			return true, nil
		}
	}
	return false, nil
}

func validateCaptainDecisionCurrent(manifest admittedManifest, proposal admittedPlanProposal, snapshot journal.Snapshot, command CaptainDecisionCommand) (CaptainDelegationState, error) {
	state, err := currentCaptainDelegation(snapshot)
	if err != nil {
		return state, err
	}
	metadata := proposal.plan.Metadata()
	if !state.Active || state.Digest != command.EnvelopeDigest || state.Epoch != command.EnvelopeEpoch || state.Decisions >= state.Envelope.Limits.MaximumTotalCaptainDecisions ||
		(command.Outcome == "revise" && state.ReplanSpent >= state.Envelope.Limits.ReplanBudget) || command.RunID != manifest.value.RunID || command.ManifestDigest != manifest.digest || command.Project != manifest.value.Authority.Project || command.Release != manifest.value.Release || command.ReleaseRef != proposal.authority.ReleaseRef || command.ReleaseHead != proposal.authority.ReleaseHead || command.TargetRef != proposal.authority.TargetRef || command.TargetHead != proposal.authority.TargetHead || command.ProposalReplayKey != proposal.replayKey || command.ProposalByteCount != int64(len(proposal.plan.Bytes())) || command.PlanDigest != proposal.plan.Digest() || command.PlanRevision != metadata.Revision || command.PriorPlan != proposal.authority.PriorPlan || command.PlannerAttempt != max(proposal.authority.PlannerAttempt, 1) || command.PlannerSourceWork != proposal.authority.SourceWork || command.PlannerSourceEffect != proposal.authority.SourceEffect {
		return state, runtimeFail("CAPTAIN_DECISION_STALE", nil)
	}
	if err := validateCaptainDecisionDispatch(snapshot, command, state.Envelope.Limits.MaximumCaptainAttemptsPerProposal); err != nil {
		return state, err
	}
	class, err := approvalDecisionClass(proposal)
	if err != nil || class != command.DecisionClass {
		return state, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", err)
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
		return state, runtimeFail("CAPTAIN_DECISION_AUTHORITY_REFUSED", nil)
	}
	if err := ValidateCaptainPlanPolicy(state.Envelope.PlanRules, proposal.plan, nil); err != nil {
		return state, runtimeFail("CAPTAIN_DECISION_AUTHORITY_REFUSED", err)
	}
	return state, nil
}

func captainDispatchAttemptCount(snapshot journal.Snapshot, workID string) (int64, error) {
	if !runtimeDigestPattern.MatchString(workID) {
		return 0, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
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

func captainDispatchAttemptForSubmission(snapshot journal.Snapshot, workID string, submission driver.Submission) (int64, error) {
	count, err := captainDispatchAttemptCount(snapshot, workID)
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
			decoded.Responsibility == driver.CaptainPlanReview && bytes.Equal(effect.Result, mustEncodeSubmission(submission)) {
			if found != 0 {
				return 0, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			found = count
		}
	}
	if found == 0 {
		return 0, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
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

func validateCaptainDecisionDispatch(snapshot journal.Snapshot, command CaptainDecisionCommand, maximum int64) error {
	if command.CaptainAttempt < 1 || command.CaptainAttempt > maximum {
		return runtimeFail("CAPTAIN_DECISION_AUTHORITY_REFUSED", nil)
	}
	prefix := "attempt/" + strings.TrimPrefix(command.CaptainWorkID, "sha256:") + "/"
	count, err := captainDispatchAttemptCount(snapshot, command.CaptainWorkID)
	if err != nil {
		return err
	}
	found := false
	for _, effect := range snapshot.Effects {
		if !strings.HasPrefix(effect.ID, prefix) || effect.State != journal.Succeeded {
			continue
		}
		submission, decodeErr := driver.DecodeSubmission(effect.Result)
		if decodeErr != nil || submission.Responsibility != driver.CaptainPlanReview {
			return runtimeFail("CORRUPT_JOURNAL", decodeErr)
		}
		if submission.InvocationID != command.CaptainInvocationID ||
			sha256Digest(effect.Result) != command.SealedHandoffDigest {
			continue
		}
		if found || count != command.CaptainAttempt {
			return runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
		}
		found = true
	}
	if !found {
		return runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
	}
	return nil
}

func (s *Service) CaptainDecide(ctx context.Context, command CaptainDecisionCommand) (CaptainDecisionResult, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return CaptainDecisionResult{}, runtimeFail("INVALID_SERVICE", nil)
	}
	payload, err := CanonicalCaptainDecisionCommand(command)
	if err != nil {
		return CaptainDecisionResult{}, err
	}
	manifest, proposal, snapshot, err := s.currentApprovalProposal(ctx, command.RunID)
	if err != nil {
		return CaptainDecisionResult{}, err
	}
	if err = s.validateCaptainReleaseLineage(ctx, manifest, proposal, snapshot); err != nil {
		return CaptainDecisionResult{}, err
	}
	if _, err = validateCaptainDecisionCurrent(manifest, proposal, snapshot, command); err != nil {
		return CaptainDecisionResult{}, err
	}
	replay, effectID, _, _ := captainDecisionIdentity(command)
	result, resultBody, _ := captainDecisionResult(command)
	now := s.now().UTC()
	if err = s.journal.RecordCommandEffect(ctx, journal.Command{RunID: command.RunID, ReplayKey: replay, Kind: "captain_decision", Payload: payload, CreatedAt: now}, journal.Effect{RunID: command.RunID, ID: effectID, ReplayKey: replay, Kind: captainDecisionEffectKind, BeforeDigest: sha256Digest(payload), ExpectedDigest: sha256Digest(resultBody), UpdatedAt: now}); err != nil {
		return CaptainDecisionResult{}, runtimeFail("CAPTAIN_DECISION_REPLAY_CONFLICT", err)
	}
	effect, err := s.journal.Effect(ctx, command.RunID, effectID)
	if err != nil {
		return CaptainDecisionResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		if !bytes.Equal(effect.Result, resultBody) {
			return CaptainDecisionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if command.Outcome == "proceed" {
			stored, readErr := s.journal.Snapshot(ctx, command.RunID)
			if readErr != nil {
				return CaptainDecisionResult{}, runtimeFail("JOURNAL_READ_FAILED", readErr)
			}
			for _, child := range stored.Commands {
				if child.ReplayKey != command.ChildReplayKey || child.Kind != "approval" {
					continue
				}
				approval, parseErr := parseApprovalCommand(child)
				if parseErr != nil {
					return CaptainDecisionResult{}, parseErr
				}
				if _, approvalErr := s.Approve(ctx, approval); approvalErr != nil {
					return CaptainDecisionResult{}, approvalErr
				}
				return result, nil
			}
			return CaptainDecisionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return result, nil
	}
	if testCaptainCrashCut == "decision_admission" {
		return CaptainDecisionResult{}, runtimeFail("TEST_CAPTAIN_CRASH_CUT", nil)
	}
	if effect.State == journal.Pending {
		claim, claimErr := s.journal.Claim(ctx, command.RunID, effectID, now, effectLease)
		if claimErr != nil {
			return CaptainDecisionResult{}, runtimeFail("CAPTAIN_DECISION_RECOVERY_PENDING", claimErr)
		}
		effect.CurrentClaim = claim.Token
	}
	if testCaptainCrashCut == "decision_claim" {
		return CaptainDecisionResult{}, runtimeFail("TEST_CAPTAIN_CRASH_CUT", nil)
	}
	freshManifest, freshProposal, fresh, err := s.currentApprovalProposal(ctx, command.RunID)
	if err != nil {
		return CaptainDecisionResult{}, err
	}
	if err = s.validateCaptainReleaseLineage(ctx, freshManifest, freshProposal, fresh); err != nil {
		_ = s.failCaptainDecisionClaim(ctx, command, effect, "CAPTAIN_RELEASE_LINEAGE_REFUSED")
		return CaptainDecisionResult{}, err
	}
	if _, err = validateCaptainDecisionCurrent(freshManifest, freshProposal, fresh, command); err != nil {
		_ = s.failCaptainDecisionClaim(ctx, command, effect, "CAPTAIN_DECISION_STALE")
		return CaptainDecisionResult{}, err
	}
	offset := int64(0)
	if len(fresh.Events) > 0 {
		offset = fresh.Events[len(fresh.Events)-1].Offset
	}
	eventBody, eventErr := captainDecisionEvent(command, replay)
	if eventErr != nil {
		return CaptainDecisionResult{}, eventErr
	}
	completion := journal.Completion{RunID: command.RunID, EffectID: effectID, Token: effect.CurrentClaim, State: journal.Succeeded, Result: resultBody, EventKind: "captain_plan_decided", EventBody: eventBody, At: now, ExpectedEventOffset: &offset}
	var childCommand *journal.Command
	var childEffect *journal.Effect
	if command.Outcome == "proceed" {
		active, _ := ParseCaptainDelegation(freshStateEnvelopeBytes(fresh, command.EnvelopeDigest))
		approval, approvalErr := approvalCommandForDelegatedProposal(freshManifest, freshProposal, active)
		if approvalErr != nil {
			return CaptainDecisionResult{}, approvalErr
		}
		childReplay, childID, childPayload, _ := approvalIdentity(approval)
		_, childResult, _ := canonicalApprovalResult(approval)
		if childReplay != command.ChildReplayKey || childID != command.ChildEffectID {
			return CaptainDecisionResult{}, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
		}
		childCommand = &journal.Command{RunID: command.RunID, ReplayKey: childReplay, Kind: "approval", Payload: childPayload, CreatedAt: now}
		childEffect = &journal.Effect{RunID: command.RunID, ID: childID, ReplayKey: childReplay, Kind: approvalEffectKind, BeforeDigest: sha256Digest(childPayload), ExpectedDigest: sha256Digest(childResult), UpdatedAt: now}
	} else if command.Outcome == "revise" {
		continuation := CaptainPlannerContinuationCommand{SchemaVersion: "sworn.captain-planner-continuation/v1", RunID: command.RunID, DecisionReplayKey: replay, SupersededProposalReplayKey: command.ProposalReplayKey, PlanRevision: command.PlanRevision, PlannerAttempt: command.PlannerAttempt + 1, EnvelopeDigest: command.EnvelopeDigest, EnvelopeEpoch: command.EnvelopeEpoch}
		body := mustJSON(continuation)
		seed := command
		seed.ChildReplayKey = ""
		seed.ChildEffectID = ""
		suffix := strings.TrimPrefix(sha256Digest(mustJSON(seed)), "sha256:")
		if command.ChildReplayKey != "planner-continuation/"+suffix || command.ChildEffectID != "planner-continuation/"+suffix {
			return CaptainDecisionResult{}, runtimeFail("CAPTAIN_DECISION_BINDING_MISMATCH", nil)
		}
		childCommand = &journal.Command{RunID: command.RunID, ReplayKey: command.ChildReplayKey, Kind: "planner_continuation", Payload: body, CreatedAt: now}
		childEffect = &journal.Effect{RunID: command.RunID, ID: command.ChildEffectID, ReplayKey: command.ChildReplayKey, Kind: "planner.continue", BeforeDigest: sha256Digest(body), ExpectedDigest: sha256Digest([]byte("scheduled")), UpdatedAt: now}
	}
	if err = s.journal.CompleteWithChild(ctx, completion, childCommand, childEffect); err != nil {
		if journal.IsCode(err, "STALE_COMPLETION") {
			_ = s.failCaptainDecisionClaim(ctx, command, effect, "CAPTAIN_DECISION_STALE")
			return CaptainDecisionResult{}, runtimeFail("CAPTAIN_DECISION_STALE", err)
		}
		return CaptainDecisionResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	if testCaptainCrashCut == "decision_completion" ||
		(testCaptainCrashCut == "revise_completion" && command.Outcome == "revise") {
		return CaptainDecisionResult{}, runtimeFail("TEST_CAPTAIN_CRASH_CUT", nil)
	}
	if command.Outcome == "proceed" {
		approvalCommand, parseErr := parseApprovalCommand(*childCommand)
		if parseErr != nil {
			return CaptainDecisionResult{}, parseErr
		}
		if _, approvalErr := s.Approve(ctx, approvalCommand); approvalErr != nil {
			return CaptainDecisionResult{}, approvalErr
		}
	}
	return result, nil
}

func (s *Service) failCaptainDecisionClaim(ctx context.Context, command CaptainDecisionCommand, effect journal.Effect, code string) error {
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
	return s.journal.Complete(ctx, journal.Completion{RunID: command.RunID, EffectID: effect.ID, Token: current.CurrentClaim, State: journal.OperationalFailed, ErrorCode: code, EventKind: "captain_plan_refused", EventBody: []byte(code), At: s.now().UTC(), ExpectedEventOffset: &offset})
}

func freshStateEnvelopeBytes(snapshot journal.Snapshot, digest string) []byte {
	state, err := currentCaptainDelegation(snapshot)
	if err != nil || state.Digest != digest {
		return nil
	}
	return state.EnvelopeBytes
}

func (s *Service) ReconcileCaptainDecisions(ctx context.Context, runID string) error {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return runtimeFail("RUN_NOT_FOUND", err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != captainDecisionEffectKind || (effect.State != journal.Pending && effect.State != journal.Claimed) {
			continue
		}
		stored, ok := commands[effect.ReplayKey]
		if !ok {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		var command CaptainDecisionCommand
		if json.Unmarshal(stored.Payload, &command) != nil {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		canonical, canonicalErr := CanonicalCaptainDecisionCommand(command)
		if canonicalErr != nil || !bytes.Equal(canonical, stored.Payload) {
			return runtimeFail("CORRUPT_JOURNAL", canonicalErr)
		}
		if _, err := s.CaptainDecide(ctx, command); err != nil && !IsCode(err, "CAPTAIN_DECISION_STALE") {
			return err
		}
	}
	return nil
}
