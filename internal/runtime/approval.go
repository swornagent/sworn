package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	ApprovalCommandVersion = "sworn.approval-command/v1"
	ApprovalResultVersion  = "sworn.approval-result/v1"
	ApprovalDecision       = "approve"
	ApprovalActorClass     = "external_authorizer"
	PlannerProposalClass   = "planner_proposal"
	PlannerReplanClass     = "planner_replan"
	approvalEffectKind     = "approval.admit"
)

// ApprovalCommand is the sole model shared unchanged by the TUI, product MCP,
// and CLI adapters. ActorAuthority is an exact manifest binding, not a
// credential or identity proof.
type ApprovalCommand struct {
	SchemaVersion     string `json:"schema_version"`
	RunID             string `json:"run_id"`
	ManifestDigest    string `json:"manifest_digest"`
	Project           string `json:"project"`
	Release           string `json:"release"`
	ReleaseRef        string `json:"release_ref"`
	ReleaseHead       string `json:"release_head,omitempty"`
	ProposalReplayKey string `json:"proposal_replay_key"`
	PlanRevision      int64  `json:"plan_revision"`
	PriorPlan         string `json:"prior_plan,omitempty"`
	PlanDigest        string `json:"plan_digest"`
	TargetRef         string `json:"target_ref"`
	TargetHead        string `json:"target_head"`
	DecisionClass     string `json:"decision_class"`
	Decision          string `json:"decision"`
	ActorClass        string `json:"actor_class"`
	ActorAuthority    string `json:"actor_authority"`
}

type ApprovalOffer struct {
	SchemaVersion string          `json:"schema_version"`
	Command       ApprovalCommand `json:"command"`
}

type ApprovalResult struct {
	SchemaVersion  string `json:"schema_version"`
	ReplayKey      string `json:"replay_key"`
	EffectID       string `json:"effect_id"`
	CommandDigest  string `json:"command_digest"`
	PlanDigest     string `json:"plan_digest"`
	DecisionClass  string `json:"decision_class"`
	Decision       string `json:"decision"`
	AdmissionState string `json:"admission_state"`
}

func CanonicalApprovalCommand(command ApprovalCommand) ([]byte, error) {
	if command.SchemaVersion != ApprovalCommandVersion ||
		!runtimeIdentityPattern.MatchString(command.RunID) ||
		!runtimeDigestPattern.MatchString(command.ManifestDigest) ||
		command.Project == "" || command.Release == "" ||
		!strings.HasPrefix(command.ReleaseRef, "refs/heads/release-wt/") ||
		command.ProposalReplayKey == "" || command.PlanRevision < 1 ||
		!runtimeDigestPattern.MatchString(command.PlanDigest) ||
		!strings.HasPrefix(command.TargetRef, "refs/heads/") ||
		command.TargetHead == "" ||
		(command.DecisionClass != PlannerProposalClass &&
			command.DecisionClass != PlannerReplanClass) ||
		command.Decision != ApprovalDecision ||
		command.ActorClass != ApprovalActorClass ||
		command.ActorAuthority == "" {
		return nil, runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	if command.PlanRevision == 1 {
		if command.PriorPlan != "" || command.ReleaseHead != "" ||
			command.DecisionClass != PlannerProposalClass {
			return nil, runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
		}
	} else if command.PriorPlan == "" || command.ReleaseHead == "" ||
		command.DecisionClass != PlannerReplanClass {
		return nil, runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	return json.Marshal(command)
}

func approvalDecisionClass(proposal admittedPlanProposal) (string, error) {
	metadata := proposal.plan.Metadata()
	if metadata.Revision == 1 && metadata.PreviousPlan == nil &&
		proposal.authority.PriorPlan == "" {
		return PlannerProposalClass, nil
	}
	if metadata.Revision >= 2 && metadata.PreviousPlan != nil &&
		*metadata.PreviousPlan == proposal.authority.PriorPlan &&
		proposal.authority.PriorPlan != "" {
		return PlannerReplanClass, nil
	}
	return "", runtimeFail("APPROVAL_AMBIGUOUS", nil)
}

func approvalCommandForProposal(
	manifest admittedManifest,
	proposal admittedPlanProposal,
) (ApprovalCommand, error) {
	decisionClass, err := approvalDecisionClass(proposal)
	if err != nil {
		return ApprovalCommand{}, err
	}
	metadata := proposal.plan.Metadata()
	command := ApprovalCommand{
		SchemaVersion: ApprovalCommandVersion,
		RunID:         manifest.value.RunID, ManifestDigest: manifest.digest,
		Project:           manifest.value.Authority.Project,
		Release:           manifest.value.Release,
		ReleaseRef:        proposal.authority.ReleaseRef,
		ReleaseHead:       proposal.authority.ReleaseHead,
		ProposalReplayKey: proposal.replayKey,
		PlanRevision:      metadata.Revision,
		PriorPlan:         proposal.authority.PriorPlan,
		PlanDigest:        proposal.plan.Digest(),
		TargetRef:         proposal.authority.TargetRef,
		TargetHead:        proposal.authority.TargetHead,
		DecisionClass:     decisionClass,
		Decision:          ApprovalDecision,
		ActorClass:        ApprovalActorClass,
		ActorAuthority:    manifest.value.Authority.ExternalAuthorizer,
	}
	if _, err := CanonicalApprovalCommand(command); err != nil {
		return ApprovalCommand{}, err
	}
	return command, nil
}

func approvalIdentity(command ApprovalCommand) (string, string, []byte, error) {
	body, err := CanonicalApprovalCommand(command)
	if err != nil {
		return "", "", nil, err
	}
	digest := sha256Digest(body)
	suffix := strings.TrimPrefix(digest, "sha256:")
	return "approval/" + command.DecisionClass + "/" + suffix,
		"approval/" + suffix, body, nil
}

func canonicalApprovalResult(command ApprovalCommand) (ApprovalResult, []byte, error) {
	replayKey, effectID, body, err := approvalIdentity(command)
	if err != nil {
		return ApprovalResult{}, nil, err
	}
	result := ApprovalResult{
		SchemaVersion: ApprovalResultVersion,
		ReplayKey:     replayKey, EffectID: effectID,
		CommandDigest: sha256Digest(body), PlanDigest: command.PlanDigest,
		DecisionClass: command.DecisionClass, Decision: command.Decision,
		AdmissionState: "succeeded",
	}
	return result, mustJSON(result), nil
}

func parseApprovalCommand(command journal.Command) (ApprovalCommand, error) {
	var value ApprovalCommand
	if json.Unmarshal(command.Payload, &value) != nil {
		return ApprovalCommand{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	canonical, err := CanonicalApprovalCommand(value)
	if err != nil || !bytes.Equal(command.Payload, canonical) {
		return ApprovalCommand{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	replayKey, _, _, err := approvalIdentity(value)
	if err != nil || command.Kind != "approval" || command.ReplayKey != replayKey {
		return ApprovalCommand{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	return value, nil
}

func validateApprovalAuthority(
	manifest admittedManifest,
	proposal admittedPlanProposal,
	command ApprovalCommand,
) error {
	expected, err := approvalCommandForProposal(manifest, proposal)
	if err != nil {
		return err
	}
	if command.DecisionClass != expected.DecisionClass {
		return runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	if command.ActorAuthority == "" {
		return runtimeFail("APPROVAL_AUTHORITY_MISSING", nil)
	}
	if command.ActorClass != ApprovalActorClass {
		return runtimeFail("APPROVAL_AUTHORITY_INSUFFICIENT", nil)
	}
	if command.ActorAuthority != manifest.value.Authority.ExternalAuthorizer {
		return runtimeFail("APPROVAL_AUTHORITY_CONFLICT", nil)
	}
	if !reflect.DeepEqual(command, expected) {
		return runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	return nil
}

func (s *Service) currentApprovalProposal(
	ctx context.Context,
	runID string,
) (admittedManifest, admittedPlanProposal, journal.Snapshot, error) {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{},
			runtimeFail("RUN_NOT_FOUND", err)
	}
	manifest, proposals, err := loadRunSnapshot(snapshot, runID)
	if err != nil || manifest.legacyVersion != "" {
		return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{},
			runtimeFail("APPROVAL_BINDING_MISMATCH", err)
	}
	engine, err := s.openEngine(manifest)
	if err != nil {
		return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{}, err
	}
	defer engine.Close()
	state, stateErr := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
	proposal, found, _, err := selectPlanProposal(engine, snapshot, proposals, state, stateErr)
	if err != nil {
		if IsCode(err, "AMBIGUOUS_PLAN_PROPOSAL") {
			return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{},
				runtimeFail("APPROVAL_AMBIGUOUS", err)
		}
		return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{}, err
	}
	if !found {
		return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{},
			runtimeFail("APPROVAL_STALE", nil)
	}
	legacyAuthority, err := effectivePlanAuthority(manifest, snapshot)
	if err != nil {
		return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{}, err
	}
	if legacyAuthority != "" && legacyAuthority != proposal.plan.Digest() {
		return admittedManifest{}, admittedPlanProposal{}, journal.Snapshot{},
			runtimeFail("APPROVAL_AUTHORITY_CONFLICT", nil)
	}
	return manifest, proposal, snapshot, nil
}

func (s *Service) ApprovalOffer(ctx context.Context, runID string) (ApprovalOffer, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return ApprovalOffer{}, runtimeFail("INVALID_SERVICE", nil)
	}
	manifest, proposal, _, err := s.currentApprovalProposal(ctx, runID)
	if err != nil {
		return ApprovalOffer{}, err
	}
	command, err := approvalCommandForProposal(manifest, proposal)
	if err != nil {
		return ApprovalOffer{}, err
	}
	return ApprovalOffer{SchemaVersion: ApprovalCommandVersion, Command: command}, nil
}

func (s *Service) completeApprovalAdmission(
	ctx context.Context,
	command ApprovalCommand,
	effect journal.Effect,
) (ApprovalResult, error) {
	result, resultBody, err := canonicalApprovalResult(command)
	if err != nil {
		return ApprovalResult{}, err
	}
	manifest, proposal, _, validationErr := s.currentApprovalProposal(ctx, command.RunID)
	if validationErr == nil {
		validationErr = validateApprovalAuthority(manifest, proposal, command)
	}
	completion := journal.Completion{
		RunID: command.RunID, EffectID: effect.ID,
		Token: effect.CurrentClaim, At: s.now().UTC(),
		EventKind: "approval_admitted", EventBody: []byte(command.PlanDigest),
	}
	if validationErr != nil {
		completion.State = journal.OperationalFailed
		completion.ErrorCode = approvalErrorCode(validationErr)
		completion.EventKind = "approval_rejected"
		if err := s.journal.Complete(ctx, completion); err != nil {
			if journal.IsCode(err, "STALE_COMPLETION") {
				current, readErr := s.journal.Effect(ctx, command.RunID, effect.ID)
				if readErr == nil && current.State == journal.OperationalFailed {
					return ApprovalResult{}, runtimeFail(current.ErrorCode, nil)
				}
			}
			return ApprovalResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return ApprovalResult{}, validationErr
	}
	completion.State = journal.Succeeded
	completion.Result = resultBody
	if err := s.journal.Complete(ctx, completion); err != nil {
		if journal.IsCode(err, "STALE_COMPLETION") {
			current, readErr := s.journal.Effect(ctx, command.RunID, effect.ID)
			if readErr == nil && current.State == journal.Succeeded {
				return parseSucceededApproval(command, current)
			}
		}
		return ApprovalResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return result, nil
}

func approvalErrorCode(err error) string {
	for _, code := range []string{
		"APPROVAL_BINDING_MISMATCH", "APPROVAL_STALE", "APPROVAL_AMBIGUOUS",
		"APPROVAL_AUTHORITY_MISSING", "APPROVAL_AUTHORITY_INSUFFICIENT",
		"APPROVAL_AUTHORITY_CONFLICT", "APPROVAL_REPLAY_CONFLICT",
	} {
		if IsCode(err, code) {
			return code
		}
	}
	return "APPROVAL_STALE"
}

func parseSucceededApproval(
	command ApprovalCommand,
	effect journal.Effect,
) (ApprovalResult, error) {
	expected, body, err := canonicalApprovalResult(command)
	if err != nil || effect.Kind != approvalEffectKind ||
		effect.State != journal.Succeeded ||
		effect.ExpectedDigest != sha256Digest(body) ||
		!bytes.Equal(effect.Result, body) {
		return ApprovalResult{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	return expected, nil
}

func (s *Service) processApprovalEffect(
	ctx context.Context,
	command ApprovalCommand,
	effect journal.Effect,
) (ApprovalResult, error) {
	switch effect.State {
	case journal.Succeeded:
		return parseSucceededApproval(command, effect)
	case journal.OperationalFailed:
		return ApprovalResult{}, runtimeFail(effect.ErrorCode, nil)
	case journal.Uncertain:
		return ApprovalResult{}, runtimeFail("APPROVAL_RECOVERY_PENDING", nil)
	case journal.Pending:
		claim, err := s.journal.Claim(
			ctx, command.RunID, effect.ID, s.now().UTC(), effectLease)
		if err != nil {
			return ApprovalResult{}, runtimeFail("APPROVAL_RECOVERY_PENDING", err)
		}
		effect.State, effect.CurrentClaim = journal.Claimed, claim.Token
	case journal.Claimed:
		// Admission has no external side effect. Repeating exact validation and
		// completing with the original claim is safe after any crash cut and
		// fences a concurrent loser at completion.
	default:
		return ApprovalResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return s.completeApprovalAdmission(ctx, command, effect)
}

func (s *Service) Approve(
	ctx context.Context,
	command ApprovalCommand,
) (ApprovalResult, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return ApprovalResult{}, runtimeFail("INVALID_SERVICE", nil)
	}
	manifest, proposal, _, err := s.currentApprovalProposal(ctx, command.RunID)
	if err != nil {
		return ApprovalResult{}, err
	}
	if err := validateApprovalAuthority(manifest, proposal, command); err != nil {
		return ApprovalResult{}, err
	}
	replayKey, effectID, payload, err := approvalIdentity(command)
	if err != nil {
		return ApprovalResult{}, err
	}
	_, resultBody, err := canonicalApprovalResult(command)
	if err != nil {
		return ApprovalResult{}, err
	}
	now := s.now().UTC()
	err = s.journal.RecordCommandEffect(ctx, journal.Command{
		RunID: command.RunID, ReplayKey: replayKey, Kind: "approval",
		Payload: payload, CreatedAt: now,
	}, journal.Effect{
		RunID: command.RunID, ID: effectID, ReplayKey: replayKey,
		Kind: approvalEffectKind, BeforeDigest: sha256Digest(payload),
		ExpectedDigest: sha256Digest(resultBody), UpdatedAt: now,
	})
	if err != nil {
		if journal.IsCode(err, "REPLAY_CONFLICT") ||
			journal.IsCode(err, "EFFECT_CONFLICT") {
			return ApprovalResult{}, runtimeFail("APPROVAL_REPLAY_CONFLICT", err)
		}
		return ApprovalResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	effect, err := s.journal.Effect(ctx, command.RunID, effectID)
	if err != nil {
		return ApprovalResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	result, err := s.processApprovalEffect(ctx, command, effect)
	if err != nil {
		return ApprovalResult{}, err
	}
	// Admission is already durable. Waking the run is best effort here; exact
	// replay and operator startup perform the same reconciliation.
	_ = s.wakeApprovedRun(ctx, command.RunID)
	return result, nil
}

func (s *Service) wakeApprovedRun(ctx context.Context, runID string) error {
	control, err := s.journal.ControlProjection(ctx, runID)
	if err != nil || control.Desired != "running" {
		return err
	}
	now := s.now().UTC()
	owner, present, err := s.journal.CurrentOwner(ctx, runID)
	if err != nil {
		return err
	}
	if present && owner.ExpiresAt.After(now) {
		_, _ = s.Status(ctx, runID)
		owner, present, err = s.journal.CurrentOwner(ctx, runID)
		if err != nil || (present && owner.ExpiresAt.After(s.now().UTC())) {
			return err
		}
	}
	if present {
		return runtimeFail("APPROVAL_RECOVERY_PENDING", nil)
	}
	owner, err = s.journal.AcquireOwner(ctx, runID, s.now().UTC(), ownerDuration(), false)
	if journal.IsCode(err, "OWNER_ACTIVE") {
		return nil
	}
	if err != nil {
		return runtimeFail("APPROVAL_RECOVERY_PENDING", err)
	}
	_, err = s.driveOwned(ctx, runID, owner)
	return err
}

// ReconcileApprovals is called at operator startup. It covers pending and
// claimed admission, plus succeeded admission whose plan was not installed.
func (s *Service) ReconcileApprovals(ctx context.Context, runID string) error {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return runtimeFail("RUN_NOT_FOUND", err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	wake := false
	for _, effect := range snapshot.Effects {
		if effect.Kind != approvalEffectKind ||
			(effect.State != journal.Pending && effect.State != journal.Claimed &&
				effect.State != journal.Succeeded) {
			continue
		}
		stored, ok := commands[effect.ReplayKey]
		if !ok {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		command, err := parseApprovalCommand(stored)
		if err != nil {
			return err
		}
		if _, err := s.processApprovalEffect(ctx, command, effect); err != nil &&
			!IsCode(err, "APPROVAL_STALE") &&
			!IsCode(err, "APPROVAL_BINDING_MISMATCH") {
			return err
		} else if err == nil {
			wake = true
		}
	}
	if !wake {
		return nil
	}
	return s.wakeApprovedRun(ctx, runID)
}

// approvalAdmission is local, exact authority for one plan. It deliberately
// contains no hosted identity, credential, URL, issue or external evidence.
type approvalAdmission struct {
	planBytes  []byte
	planDigest string
	reference  string
}

func validateApprovalRef(manifest admittedManifest, plan baton.Plan) error {
	metadata := plan.Metadata()
	expected := manifest.value.Authority.ExternalAuthorizer + "://" +
		manifest.value.Release + "/" + strconv.FormatInt(metadata.Revision, 10)
	if metadata.ApprovalRef != expected {
		return runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	return nil
}

type authorityInstaller struct {
	actions *baton.Actions
}

func newAuthorityInstaller(actions *baton.Actions) *authorityInstaller {
	return &authorityInstaller{actions: actions}
}

func installDetail(admission approvalAdmission) []byte {
	return []byte(fmt.Sprintf(
		"Sworn project authority admitted %s via %s.\n",
		admission.planDigest,
		admission.reference,
	))
}

// install is the only runtime path authorized to call RecordPlanRevision.
func (i *authorityInstaller) install(admission approvalAdmission) (baton.ActionResult, error) {
	if i == nil || i.actions == nil ||
		!runtimeDigestPattern.MatchString(admission.planDigest) ||
		sha256Digest(admission.planBytes) != admission.planDigest ||
		admission.reference == "" {
		return baton.ActionResult{}, runtimeFail("APPROVAL_ADMISSION_REQUIRED", nil)
	}
	return i.actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: admission.planBytes,
		Summary:   "Install the exact locally authorized plan.",
		Detail:    installDetail(admission),
	})
}
