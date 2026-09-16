package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	LeadDelegationCommandVersion = "sworn.lead-delegation-command/v1"
	LeadDelegationResultVersion  = "sworn.lead-delegation-result/v1"
	leadDelegationEffectKind     = "lead.delegation"
)

type LeadDelegationCommand struct {
	SchemaVersion  string `json:"schema_version"`
	Action         string `json:"action"`
	RunID          string `json:"run_id"`
	ManifestDigest string `json:"manifest_digest"`
	ActorClass     string `json:"actor_class"`
	ActorAuthority string `json:"actor_authority"`
	CurrentEpoch   int64  `json:"current_epoch"`
	CurrentDigest  string `json:"current_digest"`
	EnvelopeDigest string `json:"envelope_digest"`
	EnvelopeBytes  []byte `json:"envelope_bytes"`
}

type LeadDelegationResult struct {
	SchemaVersion  string `json:"schema_version"`
	ReplayKey      string `json:"replay_key"`
	EffectID       string `json:"effect_id"`
	Action         string `json:"action"`
	Epoch          int64  `json:"epoch"`
	EnvelopeDigest string `json:"envelope_digest"`
	State          string `json:"state"`
}

type LeadDelegationState struct {
	Envelope      LeadDelegation `json:"envelope"`
	EnvelopeBytes []byte         `json:"envelope_bytes"`
	Digest        string         `json:"digest"`
	Epoch         int64          `json:"epoch"`
	Active        bool           `json:"active"`
	ReplanSpent   int64          `json:"replan_spent"`
	Decisions     int64          `json:"decisions"`
}

func CanonicalLeadDelegationCommand(command LeadDelegationCommand) ([]byte, error) {
	if command.SchemaVersion != LeadDelegationCommandVersion ||
		(command.Action != "admit" && command.Action != "revoke" && command.Action != "replace") ||
		!runtimeIdentityPattern.MatchString(command.RunID) || !runtimeDigestPattern.MatchString(command.ManifestDigest) ||
		command.ActorClass != LeadDelegationActorClass || command.ActorAuthority == "" {
		return nil, runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
	}
	switch command.Action {
	case "admit":
		if command.CurrentEpoch != 0 || command.CurrentDigest != "" {
			return nil, runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
		}
	case "revoke":
		if command.CurrentEpoch < 1 || !runtimeDigestPattern.MatchString(command.CurrentDigest) || command.EnvelopeDigest != "" || len(command.EnvelopeBytes) != 0 {
			return nil, runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
		}
	case "replace":
		if command.CurrentEpoch < 1 || !runtimeDigestPattern.MatchString(command.CurrentDigest) {
			return nil, runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
		}
	}
	if command.Action != "revoke" {
		envelope, err := ParseLeadDelegation(command.EnvelopeBytes)
		if err != nil || envelope.Digest != command.EnvelopeDigest || envelope.Envelope.RunID != command.RunID || envelope.Envelope.ManifestDigest != command.ManifestDigest ||
			(command.Action == "admit" && envelope.Envelope.DelegationEpoch != 1) ||
			(command.Action == "replace" && envelope.Envelope.DelegationEpoch != command.CurrentEpoch+1) {
			return nil, runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", err)
		}
	}
	return json.Marshal(command)
}

func leadDelegationIdentity(command LeadDelegationCommand) (string, string, []byte, error) {
	body, err := CanonicalLeadDelegationCommand(command)
	if err != nil {
		return "", "", nil, err
	}
	digest := sha256Digest(body)
	suffix := strings.TrimPrefix(digest, "sha256:")
	return "lead-delegation/" + command.Action + "/" + suffix, "lead-delegation/" + suffix, body, nil
}

func canonicalLeadDelegationResult(command LeadDelegationCommand) (LeadDelegationResult, []byte, error) {
	replay, effect, _, err := leadDelegationIdentity(command)
	if err != nil {
		return LeadDelegationResult{}, nil, err
	}
	epoch, digest, state := command.CurrentEpoch, command.CurrentDigest, "revoked"
	if command.Action != "revoke" {
		admitted, _ := ParseLeadDelegation(command.EnvelopeBytes)
		epoch, digest, state = admitted.Envelope.DelegationEpoch, admitted.Digest, "active"
	}
	value := LeadDelegationResult{SchemaVersion: LeadDelegationResultVersion, ReplayKey: replay, EffectID: effect, Action: command.Action, Epoch: epoch, EnvelopeDigest: digest, State: state}
	return value, mustJSON(value), nil
}

func parseLeadDelegationCommand(stored journal.Command) (LeadDelegationCommand, error) {
	var command LeadDelegationCommand
	if json.Unmarshal(stored.Payload, &command) != nil {
		return command, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	body, err := CanonicalLeadDelegationCommand(command)
	replay, _, _, identityErr := leadDelegationIdentity(command)
	if err != nil || identityErr != nil || stored.Kind != "lead_delegation" || stored.ReplayKey != replay || !bytes.Equal(stored.Payload, body) {
		return command, runtimeFail("CORRUPT_JOURNAL", err)
	}
	return command, nil
}

func currentLeadDelegation(snapshot journal.Snapshot) (LeadDelegationState, error) {
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects[effect.ReplayKey] = effect
	}
	commands := make([]LeadDelegationCommand, 0)
	for _, command := range snapshot.Commands {
		if command.Kind != "lead_delegation" {
			continue
		}
		effect, ok := effects[command.ReplayKey]
		if !ok || effect.Kind != leadDelegationEffectKind || effect.State != journal.Succeeded {
			continue
		}
		parsed, err := parseLeadDelegationCommand(command)
		if err != nil {
			return LeadDelegationState{}, err
		}
		commands = append(commands, parsed)
	}
	var state LeadDelegationState
	for _, command := range commands {
		if command.Action != "admit" {
			continue
		}
		if state.Epoch != 0 {
			return LeadDelegationState{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		admitted, err := ParseLeadDelegation(command.EnvelopeBytes)
		if err != nil {
			return LeadDelegationState{}, runtimeFail("CORRUPT_JOURNAL", err)
		}
		state.Envelope, state.EnvelopeBytes, state.Digest, state.Epoch, state.Active = admitted.Envelope, admitted.Bytes, admitted.Digest, admitted.Envelope.DelegationEpoch, true
	}
	if state.Epoch == 0 && len(commands) > 0 {
		return LeadDelegationState{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	for state.Active {
		matched := -1
		for index, command := range commands {
			if command.Action == "admit" || command.CurrentEpoch != state.Epoch || command.CurrentDigest != state.Digest {
				continue
			}
			if matched != -1 {
				return LeadDelegationState{}, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			matched = index
		}
		if matched == -1 {
			break
		}
		command := commands[matched]
		if command.Action == "revoke" {
			state.Active = false
			break
		}
		if command.Action != "replace" {
			return LeadDelegationState{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		admitted, err := ParseLeadDelegation(command.EnvelopeBytes)
		if err != nil {
			return LeadDelegationState{}, runtimeFail("CORRUPT_JOURNAL", err)
		}
		state.Envelope, state.EnvelopeBytes, state.Digest, state.Epoch, state.Active = admitted.Envelope, admitted.Bytes, admitted.Digest, admitted.Envelope.DelegationEpoch, true
	}
	for _, command := range snapshot.Commands {
		if command.Kind != "lead_decision" {
			continue
		}
		effect, ok := effects[command.ReplayKey]
		if !ok || effect.State != journal.Succeeded {
			continue
		}
		var decision LeadDecisionCommand
		if json.Unmarshal(command.Payload, &decision) != nil || decision.EnvelopeDigest != state.Digest || decision.EnvelopeEpoch != state.Epoch {
			continue
		}
		state.Decisions++
		if decision.Outcome == "revise" {
			state.ReplanSpent++
		}
	}
	return state, nil
}

func validateLeadDelegationTransition(manifest admittedManifest, snapshot journal.Snapshot, command LeadDelegationCommand) error {
	if command.RunID != manifest.value.RunID || command.ManifestDigest != manifest.digest || command.ActorClass != LeadDelegationActorClass || command.ActorAuthority != manifest.value.Authority.ExternalAuthorizer {
		return runtimeFail("LEAD_DELEGATION_AUTHORITY_REFUSED", nil)
	}
	state, err := currentLeadDelegation(snapshot)
	if err != nil {
		return err
	}
	switch command.Action {
	case "admit":
		// Initial delegation is legal only when the exact command/effect pair
		// was atomically registered with the manifest. The public management
		// path validates before it can append that pair, so a normal run can
		// never opportunistically acquire delegated authority after a crash.
		bootstrapIntent := false
		replay, effectID, payload, identityErr := leadDelegationIdentity(command)
		if identityErr == nil {
			commandFound := false
			for _, stored := range snapshot.Commands {
				commandFound = commandFound || stored.ReplayKey == replay &&
					stored.Kind == "lead_delegation" && bytes.Equal(stored.Payload, payload)
			}
			for _, effect := range snapshot.Effects {
				if commandFound && effect.ID == effectID && effect.ReplayKey == replay &&
					effect.Kind == leadDelegationEffectKind &&
					(effect.State == journal.Pending || effect.State == journal.Claimed) {
					bootstrapIntent = true
				}
			}
		}
		hasPriorWork := false
		for _, effect := range snapshot.Effects {
			if effect.Kind != leadDelegationEffectKind {
				hasPriorWork = true
			}
		}
		if state.Epoch != 0 || hasPriorWork || !bootstrapIntent {
			return runtimeFail("LEAD_DELEGATION_STALE", nil)
		}
	case "revoke", "replace":
		if !state.Active || state.Epoch != command.CurrentEpoch || state.Digest != command.CurrentDigest {
			return runtimeFail("LEAD_DELEGATION_STALE", nil)
		}
	}
	if command.Action != "revoke" {
		admitted, _ := ParseLeadDelegation(command.EnvelopeBytes)
		envelope := admitted.Envelope
		if envelope.Project != manifest.value.Authority.Project || envelope.Release != manifest.value.Release || envelope.TargetRef != manifest.value.TargetRef || envelope.ManifestDigest != manifest.digest || envelope.RunID != manifest.value.RunID {
			return runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
		}
	}
	return nil
}

func (s *Service) validateLeadDelegationGitFacts(manifest admittedManifest, envelope LeadDelegation) error {
	repository, err := gitx.Open(manifest.value.Repository, s.gitExecutable)
	if err != nil {
		return runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", err)
	}
	refs, err := repository.CaptureHeadRefs([]string{envelope.ReleaseRef, envelope.TargetRef})
	if err != nil || len(refs) != 2 {
		return runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", err)
	}
	byRef := make(map[string]gitx.RefHead, 2)
	for _, ref := range refs {
		byRef[ref.Ref] = ref
	}
	release, target := byRef[envelope.ReleaseRef], byRef[envelope.TargetRef]
	if target.State != gitx.RefDirect || target.Head.String() != envelope.TargetHead {
		return runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
	}
	anchor := envelope.ReleaseLineageAnchor
	if anchor.State == "absent" {
		if release.State != gitx.RefAbsent {
			return runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
		}
		return nil
	}
	if release.State != gitx.RefDirect || release.Head.String() != anchor.ReleaseHead {
		return runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", nil)
	}
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository, RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	state, stateErr := protocol.ReadState(protocol.UseGitRepository(repository), manifest.value.Release, inertness)
	if stateErr != nil || state.Plan.OID != anchor.PlanOID || state.Plan.Metadata.Revision != anchor.PlanRevision {
		return runtimeFail("LEAD_DELEGATION_BINDING_MISMATCH", stateErr)
	}
	return nil
}

func (s *Service) LeadDelegation(ctx context.Context, command LeadDelegationCommand) (LeadDelegationResult, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return LeadDelegationResult{}, runtimeFail("INVALID_SERVICE", nil)
	}
	return s.leadDelegationAt(ctx, command, s.now().UTC())
}

func (s *Service) leadDelegationAt(ctx context.Context, command LeadDelegationCommand, now time.Time) (LeadDelegationResult, error) {
	payload, err := CanonicalLeadDelegationCommand(command)
	if err != nil {
		return LeadDelegationResult{}, err
	}
	replay, effectID, _, _ := leadDelegationIdentity(command)
	result, resultBody, _ := canonicalLeadDelegationResult(command)
	if existing, readErr := s.journal.Effect(ctx, command.RunID, effectID); readErr == nil {
		if existing.ReplayKey != replay || existing.Kind != leadDelegationEffectKind {
			return LeadDelegationResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if existing.State == journal.Succeeded {
			if existing.ExpectedDigest != sha256Digest(resultBody) || !bytes.Equal(existing.Result, resultBody) {
				return LeadDelegationResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			return result, nil
		}
	} else if !journal.IsCode(readErr, "EFFECT_NOT_FOUND") {
		return LeadDelegationResult{}, runtimeFail("JOURNAL_READ_FAILED", readErr)
	}
	snapshot, err := s.journal.Snapshot(ctx, command.RunID)
	if err != nil {
		return LeadDelegationResult{}, runtimeFail("RUN_NOT_FOUND", err)
	}
	manifest, _, err := loadRunSnapshot(snapshot, command.RunID)
	if err != nil {
		return LeadDelegationResult{}, err
	}
	if err := validateLeadDelegationTransition(manifest, snapshot, command); err != nil {
		return LeadDelegationResult{}, err
	}
	if command.Action != "revoke" {
		admitted, _ := ParseLeadDelegation(command.EnvelopeBytes)
		if err := s.validateLeadDelegationGitFacts(manifest, admitted.Envelope); err != nil {
			return LeadDelegationResult{}, err
		}
	}
	if err := s.journal.RecordCommandEffect(ctx, journal.Command{RunID: command.RunID, ReplayKey: replay, Kind: "lead_delegation", Payload: payload, CreatedAt: now}, journal.Effect{RunID: command.RunID, ID: effectID, ReplayKey: replay, Kind: leadDelegationEffectKind, BeforeDigest: sha256Digest(payload), ExpectedDigest: sha256Digest(resultBody), UpdatedAt: now}); err != nil {
		if journal.IsCode(err, "REPLAY_CONFLICT") || journal.IsCode(err, "EFFECT_CONFLICT") {
			return LeadDelegationResult{}, runtimeFail("LEAD_DELEGATION_REPLAY_CONFLICT", err)
		}
		return LeadDelegationResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	effect, err := s.journal.Effect(ctx, command.RunID, effectID)
	if err != nil {
		return LeadDelegationResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		if effect.ExpectedDigest != sha256Digest(resultBody) || !bytes.Equal(effect.Result, resultBody) {
			return LeadDelegationResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return result, nil
	}
	if effect.State == journal.Pending {
		claim, claimErr := s.journal.Claim(ctx, command.RunID, effectID, now, effectLease)
		if claimErr != nil {
			return LeadDelegationResult{}, runtimeFail("LEAD_DELEGATION_RECOVERY_PENDING", claimErr)
		}
		effect.CurrentClaim = claim.Token
	}
	fresh, err := s.journal.Snapshot(ctx, command.RunID)
	if err != nil {
		return LeadDelegationResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	freshManifest, _, err := loadRunSnapshot(fresh, command.RunID)
	if err == nil {
		err = validateLeadDelegationTransition(freshManifest, fresh, command)
	}
	if err != nil {
		return LeadDelegationResult{}, err
	}
	offset := int64(0)
	if len(fresh.Events) > 0 {
		offset = fresh.Events[len(fresh.Events)-1].Offset
	}
	completion := journal.Completion{RunID: command.RunID, EffectID: effectID, Token: effect.CurrentClaim, State: journal.Succeeded, Result: resultBody, EventKind: "lead_delegation_" + command.Action, EventBody: []byte(result.EnvelopeDigest), At: now, ExpectedEventOffset: &offset}
	if err := s.journal.Complete(ctx, completion); err != nil {
		if journal.IsCode(err, "STALE_COMPLETION") {
			return LeadDelegationResult{}, runtimeFail("LEAD_DELEGATION_STALE", err)
		}
		return LeadDelegationResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return result, nil
}

func leadDelegationMatches(left, right LeadDelegationState) bool {
	return left.Epoch == right.Epoch && left.Digest == right.Digest && left.Active == right.Active && reflect.DeepEqual(left.Envelope, right.Envelope)
}

func (s *Service) ReconcileLeadDelegations(ctx context.Context, runID string) error {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return runtimeFail("RUN_NOT_FOUND", err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != leadDelegationEffectKind || (effect.State != journal.Pending && effect.State != journal.Claimed) {
			continue
		}
		stored, ok := commands[effect.ReplayKey]
		if !ok {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		command, parseErr := parseLeadDelegationCommand(stored)
		if parseErr != nil {
			return parseErr
		}
		if _, err := s.LeadDelegation(ctx, command); err != nil && !IsCode(err, "LEAD_DELEGATION_STALE") {
			return err
		}
	}
	return nil
}
