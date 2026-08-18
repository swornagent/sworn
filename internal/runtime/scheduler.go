package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func workIdentity(values ...any) string { return sha256Digest(mustJSON(values)) }

func validateRecoveryCommand(
	command journal.Command,
	effect journal.Effect,
	payloadExpected bool,
) error {
	if command.RunID != effect.RunID ||
		command.ReplayKey != effect.ReplayKey ||
		command.Kind != effect.Kind {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if payloadExpected &&
		effect.ExpectedDigest != sha256Digest(command.Payload) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return nil
}

const batonActionCommandVersion = "sworn.baton-action/v2"

type batonActionAuthority struct {
	Release     string `json:"release"`
	Plan        string `json:"plan,omitempty"`
	ReleaseHead string `json:"release_head,omitempty"`
	TargetRef   string `json:"target_ref"`
	TargetHead  string `json:"target_head"`
	OwnerRef    string `json:"owner_ref"`
	OwnerHead   string `json:"owner_head,omitempty"`
	Before      string `json:"before"`
	Binds       string `json:"binds,omitempty"`
	Candidate   string `json:"candidate,omitempty"`
	Attempt     int64  `json:"attempt,omitempty"`
}

type batonActionCommand struct {
	Version     string               `json:"version"`
	GitIdentity gitx.Identity        `json:"git_identity"`
	Authority   batonActionAuthority `json:"authority"`
	Input       json.RawMessage      `json:"input"`
}

type installActionInput struct {
	PlanBytes  []byte `json:"plan_bytes"`
	PlanDigest string `json:"plan_digest"`
	Reference  string `json:"reference"`
}

type actionTruth string

const (
	actionAllOld    actionTruth = "all_old"
	actionAllNew    actionTruth = "all_new"
	actionStale     actionTruth = "stale"
	actionAmbiguous actionTruth = "ambiguous"
)

func marshalActionCommand(identity gitx.Identity, authority batonActionAuthority, input any) []byte {
	return mustJSON(batonActionCommand{
		Version: batonActionCommandVersion, GitIdentity: identity, Authority: authority,
		Input: append(json.RawMessage(nil), mustJSON(input)...),
	})
}

func stateActionAuthority(state baton.State, ownerRef, ownerHead, before,
	binds, candidate string, attempt int64) batonActionAuthority {
	return batonActionAuthority{
		Release: state.Release, Plan: state.Plan.OID,
		ReleaseHead: state.Refs.Release.Head,
		TargetRef:   state.Refs.Target.Ref,
		TargetHead:  state.Refs.Target.Head,
		OwnerRef:    ownerRef,
		OwnerHead:   ownerHead,
		Before:      before,
		Binds:       binds,
		Candidate:   candidate,
		Attempt:     attempt,
	}
}

func parseActionCommand(raw []byte) (batonActionCommand, error) {
	var command batonActionCommand
	if json.Unmarshal(raw, &command) != nil ||
		!bytesEqualCanonicalJSON(raw, command) ||
		command.Version != batonActionCommandVersion ||
		command.Authority.Release == "" || command.Authority.TargetRef == "" ||
		command.Authority.TargetHead == "" || command.Authority.OwnerRef == "" ||
		command.Authority.Before == "" || len(command.Input) == 0 {
		return batonActionCommand{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := gitx.ValidateIdentity(command.GitIdentity); err != nil {
		return batonActionCommand{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	return command, nil
}

func parseCanonicalActionInput(raw []byte, value any) error {
	if json.Unmarshal(raw, value) != nil {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	canonical, err := json.Marshal(value)
	if err != nil ||
		(!bytes.Equal(raw, canonical) &&
			!bytes.Equal(raw, append(canonical, '\n'))) {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	return nil
}

func batonActionWorkIdentity(
	kind string,
	command batonActionCommand,
) (string, error) {
	authority := command.Authority
	if !runtimeDigestPattern.MatchString(authority.Before) {
		return "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	switch kind {
	case "baton.install":
		var input installActionInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return "", runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return authority.Before, nil
	case "baton.append_receipt":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Slice == "" {
			return "", runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return workIdentity(
			authority.Before,
			"append",
			input.Role,
			input.Result,
			input.Candidate,
		), nil
	case "baton.assembly_verdict":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Slice != "" {
			return "", runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return workIdentity(authority.Before, "assembly_verdict"), nil
	case "baton.prepare_assembly":
		var input baton.PrepareAssemblyInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return "", runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return workIdentity(authority.Before, "prepare"), nil
	case "baton.merge":
		var input baton.MergePassedCandidateInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return "", runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return workIdentity(authority.Before, "merge"), nil
	default:
		return "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
}

func validateBatonActionEnvelope(
	engine *engine,
	command journal.Command,
	effect journal.Effect,
	persisted batonActionCommand,
) error {
	if engine == nil ||
		persisted.Authority.Release != engine.manifest.value.Release ||
		persisted.Authority.TargetRef != engine.manifest.value.TargetRef {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("baton action manifest authority mismatch"),
		)
	}
	if err := validateRecoveryCommand(command, effect, true); err != nil {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(
				err,
				errors.New("baton action command/effect binding mismatch"),
			),
		)
	}
	work, err := batonActionWorkIdentity(effect.Kind, persisted)
	if err != nil {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(
				err,
				errors.New("baton action work identity is invalid"),
			),
		)
	}
	attemptWork, err := attemptWorkIdentity(effect.ID)
	if err != nil ||
		attemptWork != work ||
		effect.BeforeDigest != work {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(
				err,
				errors.New("baton action attempt authority mismatch"),
			),
		)
	}
	releaseRef := "refs/heads/release-wt/" +
		engine.manifest.value.Release
	if effect.Kind == "baton.append_receipt" {
		prefix := "refs/heads/track/" +
			engine.manifest.value.Release + "/"
		track := strings.TrimPrefix(
			persisted.Authority.OwnerRef, prefix)
		if track == persisted.Authority.OwnerRef ||
			!runtimeIdentityPattern.MatchString(track) {
			return runtimeFail(
				"CORRUPT_JOURNAL",
				errors.New("baton action track authority mismatch"),
			)
		}
	} else if persisted.Authority.OwnerRef != releaseRef {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("baton action release authority mismatch"),
		)
	}
	if effect.Kind != "baton.install" &&
		(persisted.Authority.Plan == "" ||
			persisted.Authority.ReleaseHead == "" ||
			persisted.Authority.Binds == "") {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("baton action required authority is absent"),
		)
	}
	if effect.Kind != "baton.install" &&
		effect.Kind != "baton.append_receipt" &&
		persisted.Authority.OwnerHead == "" {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("baton action owner head is absent"),
		)
	}
	authority := persisted.Authority
	parseOID := func(value string, optional bool) error {
		if value == "" && optional {
			return nil
		}
		if _, err := gitx.ParseOID(
			engine.repository.ObjectFormat(),
			value,
		); err != nil {
			return runtimeFail("CORRUPT_JOURNAL", err)
		}
		return nil
	}
	if err := parseOID(authority.TargetHead, false); err != nil {
		return err
	}
	switch effect.Kind {
	case "baton.install":
		var input installActionInput
		if parseCanonicalActionInput(persisted.Input, &input) != nil {
			return runtimeFail(
				"CORRUPT_JOURNAL",
				errors.New("install action input is noncanonical"),
			)
		}
		plan, err := validateInstallActionPolicy(engine.manifest, input)
		metadata := plan.Metadata()
		previousMatches := authority.Plan == "" &&
			metadata.PreviousPlan == nil
		if authority.Plan != "" {
			previousMatches = metadata.PreviousPlan != nil &&
				*metadata.PreviousPlan == authority.Plan
		}
		if err != nil ||
			plan.Metadata().Release != engine.manifest.value.Release ||
			plan.Metadata().TargetRef != engine.manifest.value.TargetRef ||
			!previousMatches ||
			authority.OwnerHead != authority.ReleaseHead ||
			authority.Binds != "" ||
			authority.Candidate != "" ||
			authority.Attempt != 0 {
			return runtimeFail(
				"CORRUPT_JOURNAL",
				errors.Join(
					err,
					errors.New("install action authority mismatch"),
				),
			)
		}
		if err := parseOID(authority.Plan, true); err != nil {
			return err
		}
		if err := parseOID(authority.ReleaseHead, true); err != nil {
			return err
		}
	case "baton.append_receipt":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(persisted.Input, &input) != nil ||
			input.Release != authority.Release ||
			input.Slice == "" ||
			authority.Attempt < 1 ||
			authority.Candidate != input.Candidate {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		for _, value := range []string{
			authority.Plan,
			authority.ReleaseHead,
			authority.Binds,
		} {
			if err := parseOID(value, false); err != nil {
				return err
			}
		}
		if err := parseOID(authority.OwnerHead, true); err != nil {
			return err
		}
		if err := parseOID(authority.Candidate, true); err != nil {
			return err
		}
	case "baton.assembly_verdict":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(persisted.Input, &input) != nil ||
			input.Release != authority.Release ||
			input.Slice != "" ||
			input.Candidate == "" ||
			authority.OwnerHead != authority.ReleaseHead ||
			authority.Candidate != input.Candidate ||
			authority.Attempt != 0 {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		for _, value := range []string{
			authority.Plan,
			authority.ReleaseHead,
			authority.Binds,
			authority.Candidate,
		} {
			if err := parseOID(value, false); err != nil {
				return err
			}
		}
	case "baton.prepare_assembly":
		var input baton.PrepareAssemblyInput
		if parseCanonicalActionInput(persisted.Input, &input) != nil ||
			input.Release != authority.Release ||
			authority.OwnerHead != authority.ReleaseHead ||
			authority.Candidate != "" ||
			authority.Attempt != 0 {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		for _, value := range []string{
			authority.Plan,
			authority.ReleaseHead,
			authority.Binds,
		} {
			if err := parseOID(value, false); err != nil {
				return err
			}
		}
	case "baton.merge":
		var input baton.MergePassedCandidateInput
		if parseCanonicalActionInput(persisted.Input, &input) != nil ||
			input.Release != authority.Release ||
			authority.OwnerHead != authority.ReleaseHead ||
			authority.Candidate == "" ||
			authority.Attempt != 0 {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		for _, value := range []string{
			authority.Plan,
			authority.ReleaseHead,
			authority.Binds,
			authority.Candidate,
		} {
			if err := parseOID(value, false); err != nil {
				return err
			}
		}
	default:
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return nil
}

func planMetadataForOID(
	state baton.State,
	oid string,
) (baton.Metadata, bool) {
	for _, history := range state.Plan.History {
		if history.OID == oid {
			return history.Plan.Metadata(), true
		}
	}
	if state.Plan.OID == oid {
		return state.Plan.Metadata, true
	}
	return baton.Metadata{}, false
}

func validateBatonActionStateAuthority(
	state baton.State,
	kind string,
	command batonActionCommand,
) error {
	if kind != "baton.append_receipt" {
		return nil
	}
	var input baton.AppendReceiptInput
	if parseCanonicalActionInput(command.Input, &input) != nil {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	metadata, ok := planMetadataForOID(state, command.Authority.Plan)
	if !ok {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	expectedTrack := ""
	for _, track := range metadata.Tracks {
		for _, slice := range track.Slices {
			if slice.ID != input.Slice {
				continue
			}
			if expectedTrack != "" {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			expectedTrack = track.ID
		}
	}
	if expectedTrack == "" ||
		command.Authority.OwnerRef !=
			"refs/heads/track/"+state.Release+"/"+expectedTrack {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return nil
}

func ownerHead(state baton.State, ref string) (string, bool) {
	if state.Refs.Release.Ref == ref {
		return state.Refs.Release.Head, true
	}
	for _, track := range state.Refs.Tracks {
		if track.Ref == ref {
			return track.Head, true
		}
	}
	return "", false
}

func actionReceiptMatches(entry *baton.ReceiptEntry, authority batonActionAuthority,
	input baton.AppendReceiptInput) bool {
	if entry == nil {
		return false
	}
	receipt := entry.Receipt
	if receipt.Plan != authority.Plan || receipt.Binds != authority.Binds ||
		receipt.Role != input.Role || receipt.Result != input.Result ||
		receipt.Summary != input.Summary || !bytes.Equal(entry.Detail, input.Detail) {
		return false
	}
	if authority.Attempt != 0 &&
		(receipt.Attempt == nil || *receipt.Attempt != authority.Attempt) {
		return false
	}
	if input.Candidate != "" &&
		(receipt.Candidate == nil || *receipt.Candidate != input.Candidate) {
		return false
	}
	if input.CheckResults != nil {
		expected := baton.DigestBytes(input.CheckResults)
		if receipt.Checks == nil || *receipt.Checks != expected {
			return false
		}
	}
	return true
}

type appliedActionEvidence struct {
	receipt    baton.ReceiptEntry
	plan       baton.PlanHistory
	ref        string
	hasReceipt bool
	hasPlan    bool
}

func appendUniqueReceipt(
	values []baton.ReceiptEntry,
	entry *baton.ReceiptEntry,
) []baton.ReceiptEntry {
	if entry == nil {
		return values
	}
	for _, value := range values {
		if value.OID == entry.OID {
			return values
		}
	}
	return append(values, entry.Clone())
}

func matchingReceipt(
	values []baton.ReceiptEntry,
	matches func(*baton.ReceiptEntry) bool,
) (baton.ReceiptEntry, bool, error) {
	var found baton.ReceiptEntry
	for index := range values {
		if !matches(&values[index]) {
			continue
		}
		if found.OID != "" && found.OID != values[index].OID {
			return baton.ReceiptEntry{}, false,
				runtimeFail("AMBIGUOUS_ACTION_HISTORY", nil)
		}
		found = values[index].Clone()
	}
	return found, found.OID != "", nil
}

func validateInstallActionInput(
	input installActionInput,
) (baton.Plan, error) {
	if input.PlanDigest == "" ||
		sha256Digest(input.PlanBytes) != input.PlanDigest ||
		input.Reference == "" {
		return baton.Plan{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	plan, err := baton.ParsePlan(input.PlanBytes)
	if err != nil ||
		plan.Digest() != input.PlanDigest ||
		plan.Metadata().ApprovalRef != input.Reference {
		return baton.Plan{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	return plan, nil
}

func validateInstallActionPolicy(
	manifest admittedManifest,
	input installActionInput,
) (baton.Plan, error) {
	plan, err := validateInstallActionInput(input)
	if err != nil {
		return baton.Plan{}, err
	}
	if plan.Metadata().Repository != manifest.value.Authority.Project ||
		plan.Metadata().Release != manifest.value.Release ||
		plan.Metadata().TargetRef != manifest.value.TargetRef ||
		validateApprovalRef(manifest, plan) != nil {
		return baton.Plan{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return plan, nil
}

func appliedBatonAction(
	state baton.State,
	kind string,
	command batonActionCommand,
) (appliedActionEvidence, bool, error) {
	authority := command.Authority
	switch kind {
	case "baton.append_receipt", "baton.assembly_verdict":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Release != authority.Release {
			return appliedActionEvidence{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		var entries []baton.ReceiptEntry
		ref := state.Refs.Release.Ref
		if input.Slice == "" {
			entries = append(entries, state.Assembly.History...)
			entries = appendUniqueReceipt(
				entries, state.Assembly.CurrentReceipt)
		} else {
			history, ok := state.HistoryForSlice(input.Slice)
			if ok {
				ref = history.Ref
				entries = append(entries, history.History.Entries...)
			} else {
				slice, current := state.Slice(input.Slice)
				if !current {
					return appliedActionEvidence{}, false, nil
				}
				track, current := state.Track(
					slice.Location.Track.ID,
				)
				if !current {
					return appliedActionEvidence{}, false,
						runtimeFail("CORRUPT_JOURNAL", nil)
				}
				ref = track.Ref
				entries = append(entries, slice.History.Entries...)
			}
			if slice, current := state.Slice(input.Slice); current {
				entries = appendUniqueReceipt(
					entries,
					slice.CurrentReceipt,
				)
			}
		}
		entry, found, err := matchingReceipt(
			entries,
			func(entry *baton.ReceiptEntry) bool {
				return actionReceiptMatches(entry, authority, input)
			},
		)
		return appliedActionEvidence{
			receipt: entry, ref: ref, hasReceipt: found,
		}, found, err
	case "baton.prepare_assembly":
		var input baton.PrepareAssemblyInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Release != authority.Release {
			return appliedActionEvidence{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		entries := append(
			[]baton.ReceiptEntry(nil), state.Assembly.History...)
		entries = appendUniqueReceipt(entries, state.Assembly.Candidate)
		entry, found, err := matchingReceipt(
			entries,
			func(entry *baton.ReceiptEntry) bool {
				receipt := entry.Receipt
				return receipt.Plan == authority.Plan &&
					receipt.Binds == authority.Binds &&
					receipt.Role == "implementer" &&
					receipt.Result == "candidate" &&
					receipt.Summary == input.Summary &&
					bytes.Equal(entry.Detail, input.Detail) &&
					receipt.Target != nil &&
					*receipt.Target == authority.TargetHead
			},
		)
		return appliedActionEvidence{
			receipt: entry, ref: state.Refs.Release.Ref,
			hasReceipt: found,
		}, found, err
	case "baton.merge":
		var input baton.MergePassedCandidateInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Release != authority.Release {
			return appliedActionEvidence{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		entries := append(
			[]baton.ReceiptEntry(nil), state.Assembly.History...)
		entries = appendUniqueReceipt(
			entries, state.Assembly.CurrentReceipt)
		entry, found, err := matchingReceipt(
			entries,
			func(entry *baton.ReceiptEntry) bool {
				receipt := entry.Receipt
				return receipt.Plan == authority.Plan &&
					receipt.Binds == authority.Binds &&
					receipt.Role == "merge" &&
					receipt.Result == "merged" &&
					receipt.Summary == input.Summary &&
					bytes.Equal(entry.Detail, input.Detail) &&
					receipt.Candidate != nil &&
					*receipt.Candidate == authority.Candidate &&
					receipt.Target != nil &&
					*receipt.Target == authority.TargetHead
			},
		)
		return appliedActionEvidence{
			receipt: entry, ref: state.Refs.Target.Ref,
			hasReceipt: found,
		}, found, err
	case "baton.install":
		var input installActionInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return appliedActionEvidence{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		plan, err := validateInstallActionInput(input)
		if err != nil {
			return appliedActionEvidence{}, false, err
		}
		history := append(
			[]baton.PlanHistory(nil), state.Plan.History...)
		if len(history) == 0 {
			history = append(history, baton.PlanHistory{
				OID:         state.Plan.OID,
				Revision:    state.Plan.Metadata.Revision,
				Approval:    state.Plan.Approval.Clone(),
				Plan:        plan,
				InstallHead: state.Plan.Approval.OID,
			})
		}
		var found baton.PlanHistory
		for _, candidate := range history {
			metadata := candidate.Plan.Metadata()
			previousMatches := authority.Plan == "" &&
				metadata.PreviousPlan == nil
			if authority.Plan != "" {
				previousMatches = metadata.PreviousPlan != nil &&
					*metadata.PreviousPlan == authority.Plan
			}
			receipt := candidate.Approval.Receipt
			if candidate.Plan.Digest() != input.PlanDigest ||
				metadata.ApprovalRef != input.Reference ||
				!previousMatches ||
				!bytes.Equal(candidate.Approval.Detail, installDetail(approvalAdmission{
					planBytes: input.PlanBytes, planDigest: input.PlanDigest,
					reference: input.Reference,
				})) ||
				receipt.Role != "planner" ||
				receipt.Result != "approved" ||
				receipt.Plan != candidate.OID ||
				receipt.Summary !=
					"Install the exact locally authorized plan." ||
				receipt.Target == nil ||
				*receipt.Target != authority.TargetHead {
				continue
			}
			if found.OID != "" &&
				found.Approval.OID != candidate.Approval.OID {
				return appliedActionEvidence{}, false,
					runtimeFail("AMBIGUOUS_ACTION_HISTORY", nil)
			}
			found = candidate
		}
		if found.OID == "" {
			return appliedActionEvidence{}, false, nil
		}
		return appliedActionEvidence{
			plan: found, ref: state.Refs.Release.Ref, hasPlan: true,
		}, true, nil
	default:
		return appliedActionEvidence{}, false,
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
}

func actionAlreadyApplied(state baton.State, kind string,
	command batonActionCommand) (bool, error) {
	_, applied, err := appliedBatonAction(state, kind, command)
	return applied, err
}

func actionBeforeMatches(state baton.State, kind string,
	command batonActionCommand) bool {
	authority := command.Authority
	switch kind {
	case "baton.append_receipt":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return false
		}
		return input.Slice != "" &&
			sliceFingerprint(state, input.Slice) == authority.Before
	case "baton.assembly_verdict":
		return workIdentity(
			state.Plan.OID, state.Refs.Release.Head,
			state.Refs.Target.Head, authority.Candidate,
		) == authority.Before
	case "baton.prepare_assembly":
		return workIdentity(
			state.Plan.OID, state.Refs.Release.Head, state.Refs.Target.Head,
			state.Assembly.Outcome, state.Assembly.InputPins,
		) == authority.Before
	case "baton.merge":
		return state.Assembly.Pass != nil && workIdentity(
			state.Plan.OID, state.Refs.Release.Head,
			state.Refs.Target.Head, state.Assembly.Pass.OID,
		) == authority.Before
	default:
		return false
	}
}

func validateBatonAllOldStateAuthority(
	state baton.State,
	kind string,
	command batonActionCommand,
) error {
	authority := command.Authority
	switch kind {
	case "baton.install":
		// First installation has no Baton state. Its exact release/target/ref
		// vector and prior-plan relationship are admitted by the install
		// envelope and classifyBatonAction.
		return nil
	case "baton.append_receipt":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		slice, ok := state.Slice(input.Slice)
		if !ok || slice.CurrentReceipt == nil ||
			slice.CurrentReceipt.OID != authority.Binds ||
			slice.Attempt != authority.Attempt ||
			sliceFingerprint(state, input.Slice) != authority.Before {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		switch {
		case input.Role == "implementer" && input.Result == "designed":
			if slice.Stage != "design" || slice.NextRole != "implementer" ||
				input.Candidate != "" {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
		case input.Role == "captain" &&
			(input.Result == "proceed" ||
				input.Result == "revise" ||
				input.Result == "escalate"):
			if slice.NextRole != "captain" || input.Candidate != "" {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
		case input.Role == "implementer" && input.Result == "candidate":
			track, trackOK := state.Track(slice.Location.Track.ID)
			if !trackOK || slice.Stage != "implement" ||
				slice.NextRole != "implementer" ||
				input.Candidate == "" ||
				input.Candidate != track.Head {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
		case input.Role == "verifier" &&
			(input.Result == "pass" ||
				input.Result == "fail" ||
				input.Result == "blocked"):
			if slice.NextRole != "verifier" ||
				slice.Candidate == nil ||
				slice.Candidate.Receipt.Candidate == nil ||
				input.Candidate !=
					*slice.Candidate.Receipt.Candidate {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
		default:
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	case "baton.assembly_verdict":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			state.Assembly.NextRole != "verifier" ||
			state.Assembly.Candidate == nil ||
			state.Assembly.Candidate.Receipt.Candidate == nil ||
			authority.Binds != state.Assembly.Candidate.OID ||
			authority.Attempt != 0 ||
			input.Candidate != *state.Assembly.Candidate.Receipt.Candidate {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	case "baton.prepare_assembly":
		expectedBinds := state.Plan.ApprovalOID
		if state.Assembly.CurrentReceipt != nil {
			expectedBinds = state.Assembly.CurrentReceipt.OID
		}
		if state.Assembly.NextRole != "merge" ||
			authority.Binds != expectedBinds ||
			authority.Attempt != 0 ||
			authority.Candidate != "" {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	case "baton.merge":
		if state.Assembly.NextRole != "merge" ||
			state.Assembly.Outcome != "pass" ||
			state.Assembly.Pass == nil ||
			state.Assembly.Pass.Receipt.Candidate == nil ||
			authority.Binds != state.Assembly.Pass.OID ||
			authority.Attempt != 0 ||
			authority.Candidate !=
				*state.Assembly.Pass.Receipt.Candidate {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	default:
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return nil
}

// installActionIdempotentlyCallable recognizes an exact plan that another
// Baton client already installed under the same still-current target
// authority. This is not proof that a previously claimed Sworn effect ran:
// Recovery must still classify different persisted authority as stale. This
// only permits a new live invocation to call Baton's idempotent
// RecordPlanRevision and journal the actual no-change result.
func installActionIdempotentlyCallable(
	state baton.State,
	command batonActionCommand,
) bool {
	var input installActionInput
	if parseCanonicalActionInput(command.Input, &input) != nil {
		return false
	}
	plan, err := validateInstallActionInput(input)
	if err != nil {
		return false
	}
	authority := command.Authority
	metadata := plan.Metadata()
	previousMatches := authority.Plan == "" &&
		metadata.PreviousPlan == nil
	if authority.Plan != "" {
		previousMatches = metadata.PreviousPlan != nil &&
			*metadata.PreviousPlan == authority.Plan
	}
	return previousMatches &&
		state.Release == authority.Release &&
		state.Plan.Digest == input.PlanDigest &&
		state.Plan.Metadata.Revision == metadata.Revision &&
		state.Plan.Metadata.ApprovalRef == input.Reference &&
		state.Refs.Release.Ref == authority.OwnerRef &&
		state.Refs.Target.Ref == authority.TargetRef &&
		state.Refs.Target.Head == authority.TargetHead &&
		!state.Plan.TargetStale &&
		state.Plan.Approval.Receipt.Target != nil &&
		*state.Plan.Approval.Receipt.Target == authority.TargetHead
}

func classifyBatonAction(engine *engine, kind string,
	command batonActionCommand) (actionTruth, baton.State, error) {
	authority := command.Authority
	state, err := baton.ReadState(engine.git, authority.Release, engine.inertness)
	if kind == "baton.install" {
		if err == nil {
			applied, matchErr := actionAlreadyApplied(state, kind, command)
			if matchErr != nil {
				return actionAmbiguous, state, matchErr
			}
			if applied {
				return actionAllNew, state, nil
			}
		}
		var input installActionInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return actionAmbiguous, state, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if _, validateErr := validateInstallActionInput(input); validateErr != nil {
			return actionAmbiguous, state, validateErr
		}
		refs, captureErr := engine.repository.CaptureHeadRefs(
			[]string{authority.OwnerRef, authority.TargetRef})
		if captureErr != nil || len(refs) != 2 {
			return actionAmbiguous, state, captureErr
		}
		ownerExact, targetExact := false, false
		for _, ref := range refs {
			switch ref.Ref {
			case authority.OwnerRef:
				if authority.OwnerHead == "" {
					ownerExact = ref.State == gitx.RefAbsent
				} else {
					ownerExact = ref.State == gitx.RefDirect &&
						ref.Head.String() == authority.OwnerHead
				}
			case authority.TargetRef:
				targetExact = ref.State == gitx.RefDirect &&
					ref.Head.String() == authority.TargetHead
			}
		}
		// Installation never writes the target. Any target movement therefore
		// proves that the persisted all-old authority has gone stale, including
		// a crash before the first install write.
		if !targetExact {
			return actionStale, state, nil
		}
		if ownerExact {
			if authority.Plan == "" && err != nil {
				return actionAllOld, state, nil
			}
			if err == nil && state.Plan.OID == authority.Plan {
				return actionAllOld, state, nil
			}
		}
		if err == nil && state.Plan.OID != authority.Plan {
			return actionStale, state, nil
		}
		if err == nil && state.Plan.OID == authority.Plan && !ownerExact {
			return actionStale, state, nil
		}
		return actionAmbiguous, state, nil
	}
	if err != nil {
		return actionAmbiguous, state, err
	}
	if err := validateBatonActionStateAuthority(
		state,
		kind,
		command,
	); err != nil {
		return actionAmbiguous, state, err
	}
	applied, err := actionAlreadyApplied(state, kind, command)
	if err != nil {
		return actionAmbiguous, state, err
	}
	if applied {
		return actionAllNew, state, nil
	}
	if state.Plan.OID != authority.Plan ||
		state.Refs.Target.Ref != authority.TargetRef ||
		state.Refs.Target.Head != authority.TargetHead ||
		state.Plan.TargetStale {
		return actionStale, state, nil
	}
	if state.Refs.Release.Head != authority.ReleaseHead {
		return actionStale, state, nil
	}
	head, ok := ownerHead(state, authority.OwnerRef)
	if !ok || head != authority.OwnerHead {
		return actionStale, state, nil
	}
	if !actionBeforeMatches(state, kind, command) {
		return actionAmbiguous, state, nil
	}
	return actionAllOld, state, nil
}

func cloneRuntimeInputs(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func reconstructAllNewBatonAction(
	state baton.State,
	kind string,
	command batonActionCommand,
) (baton.ActionResult, error) {
	evidence, applied, err := appliedBatonAction(state, kind, command)
	if err != nil || !applied {
		return baton.ActionResult{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	result := baton.ActionResult{
		Kind: "baton.action-result/v2", Changed: false, Release: state.Release,
	}
	switch kind {
	case "baton.install":
		if !evidence.hasPlan ||
			evidence.plan.OID == "" ||
			evidence.plan.Revision <= 0 ||
			evidence.plan.Approval.OID == "" ||
			evidence.plan.InstallHead == "" ||
			evidence.plan.Approval.Receipt.Target == nil {
			return baton.ActionResult{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		receipt := evidence.plan.Approval.Receipt.Clone()
		result.Action = "recordPlanRevision"
		result.Revision = evidence.plan.Revision
		result.Plan = evidence.plan.OID
		result.Ref = evidence.ref
		result.Head = evidence.plan.InstallHead
		result.Target = *receipt.Target
		result.ReceiptCommit = evidence.plan.Approval.OID
		result.Receipt = &receipt
		result.Retirements = cloneRuntimeRetirements(
			evidence.plan.Retirements)
	case "baton.append_receipt", "baton.assembly_verdict":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return baton.ActionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if !evidence.hasReceipt || evidence.receipt.OID == "" {
			return baton.ActionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		receipt := evidence.receipt.Receipt.Clone()
		result.Action = "appendReceipt"
		result.Ref = evidence.ref
		result.Slice = input.Slice
		result.ReceiptCommit = evidence.receipt.OID
		result.Receipt = &receipt
	case "baton.prepare_assembly":
		if !evidence.hasReceipt ||
			evidence.receipt.OID == "" ||
			evidence.receipt.Receipt.Candidate == nil {
			return baton.ActionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		receipt := evidence.receipt.Receipt.Clone()
		result.Action = "prepareAssembly"
		result.Direct = false
		result.Candidate = *receipt.Candidate
		result.Inputs = cloneRuntimeInputs(receipt.Inputs)
		result.ReceiptCommit = evidence.receipt.OID
		result.Receipt = &receipt
	case "baton.merge":
		if !evidence.hasReceipt ||
			evidence.receipt.OID == "" ||
			evidence.receipt.Receipt.Candidate == nil ||
			evidence.receipt.Receipt.ResultCommit == nil {
			return baton.ActionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		receipt := evidence.receipt.Receipt.Clone()
		result.Action = "mergePassedCandidate"
		result.Candidate = *receipt.Candidate
		result.Target = evidence.ref
		result.ResultCommit = *receipt.ResultCommit
		result.ReceiptCommit = evidence.receipt.OID
		result.Receipt = &receipt
	default:
		return baton.ActionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return result, nil
}

func installEvidenceFromHistory(
	engine *engine,
	state baton.State,
	command batonActionCommand,
) (appliedActionEvidence, error) {
	var input installActionInput
	if parseCanonicalActionInput(command.Input, &input) != nil {
		return appliedActionEvidence{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	plan, err := validateInstallActionPolicy(engine.manifest, input)
	if err != nil {
		return appliedActionEvidence{}, err
	}
	metadata := plan.Metadata()
	var found baton.PlanHistory
	for _, candidate := range state.Plan.History {
		candidateMetadata := candidate.Plan.Metadata()
		previousMatches := command.Authority.Plan == "" &&
			candidateMetadata.PreviousPlan == nil
		if command.Authority.Plan != "" {
			previousMatches = candidateMetadata.PreviousPlan != nil &&
				*candidateMetadata.PreviousPlan == command.Authority.Plan
		}
		receipt := candidate.Approval.Receipt
		expectedAdmission := approvalAdmission{
			planBytes: input.PlanBytes, planDigest: input.PlanDigest,
			reference: input.Reference,
		}
		if candidate.Plan.Digest() != input.PlanDigest ||
			candidateMetadata.Revision != metadata.Revision ||
			candidateMetadata.ApprovalRef != input.Reference ||
			!previousMatches ||
			receipt.Role != "planner" ||
			receipt.Result != "approved" ||
			receipt.Plan != candidate.OID ||
			receipt.Summary != "Install the exact locally authorized plan." ||
			!bytes.Equal(candidate.Approval.Detail, installDetail(expectedAdmission)) ||
			receipt.Target == nil ||
			*receipt.Target != command.Authority.TargetHead ||
			candidate.InstallHead == "" {
			continue
		}
		if found.OID != "" &&
			found.Approval.OID != candidate.Approval.OID {
			return appliedActionEvidence{},
				runtimeFail("AMBIGUOUS_ACTION_HISTORY", nil)
		}
		found = candidate
	}
	if found.OID == "" {
		return appliedActionEvidence{},
			runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	return appliedActionEvidence{
		plan:    found,
		ref:     state.Refs.Release.Ref,
		hasPlan: true,
	}, nil
}

func reconstructSucceededBatonAction(
	engine *engine,
	kind string,
	command batonActionCommand,
) (baton.ActionResult, error) {
	if kind == "baton.install" {
		state, err := baton.ReadState(
			engine.git,
			engine.manifest.value.Release,
			engine.inertness,
		)
		if err != nil {
			return baton.ActionResult{},
				runtimeFail("RECOVERY_UNCERTAIN", err)
		}
		evidence, err := installEvidenceFromHistory(
			engine,
			state,
			command,
		)
		if err != nil {
			return baton.ActionResult{}, err
		}
		result := baton.ActionResult{
			Kind: "baton.action-result/v2", Changed: false,
			Release: state.Release, Action: "recordPlanRevision",
			Revision: evidence.plan.Revision, Plan: evidence.plan.OID,
			Ref: evidence.ref, Head: evidence.plan.InstallHead,
			ReceiptCommit: evidence.plan.Approval.OID,
			Retirements: cloneRuntimeRetirements(
				evidence.plan.Retirements),
		}
		receipt := evidence.plan.Approval.Receipt.Clone()
		if receipt.Target == nil {
			return baton.ActionResult{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		result.Target = *receipt.Target
		result.Receipt = &receipt
		return result, nil
	}
	truth, state, err := classifyBatonAction(engine, kind, command)
	if err != nil || truth != actionAllNew {
		return baton.ActionResult{},
			runtimeFail("RECOVERY_UNCERTAIN", err)
	}
	return reconstructAllNewBatonAction(state, kind, command)
}

func canonicalActionResult(raw []byte) (baton.ActionResult, error) {
	var result baton.ActionResult
	if json.Unmarshal(raw, &result) != nil ||
		!bytesEqualCanonicalJSON(raw, result) {
		return baton.ActionResult{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return result, nil
}

func actionResultMatchesDurableTruth(
	actual baton.ActionResult,
	expected baton.ActionResult,
) bool {
	actual.Changed = false
	expected.Changed = false
	return bytes.Equal(mustJSON(actual), mustJSON(expected))
}

func validateSucceededBatonAction(
	engine *engine,
	command journal.Command,
	effect journal.Effect,
	persisted batonActionCommand,
) (baton.ActionResult, error) {
	if err := validateBatonActionEnvelope(
		engine,
		command,
		effect,
		persisted,
	); err != nil {
		return baton.ActionResult{}, err
	}
	stored, err := canonicalActionResult(effect.Result)
	if err != nil {
		return baton.ActionResult{}, err
	}
	expected, err := reconstructSucceededBatonAction(
		engine,
		effect.Kind,
		persisted,
	)
	if err != nil {
		return baton.ActionResult{}, err
	}
	if !actionResultMatchesDurableTruth(stored, expected) {
		return baton.ActionResult{},
			runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	return stored, nil
}

func cloneRuntimeRetirements(
	source []baton.RetirementResult,
) []baton.RetirementResult {
	if source == nil {
		return nil
	}
	result := make([]baton.RetirementResult, len(source))
	for index, value := range source {
		result[index] = value
		result[index].Receipt = value.Receipt.Clone()
	}
	return result
}

func driverWorkIdentity(manifestDigest, slice string,
	responsibility driver.Responsibility, batonAttempt int64, before string) string {
	return workIdentity(manifestDigest, slice, responsibility, batonAttempt, before)
}

func dispatchAuthorityCurrent(
	state baton.State,
	slice string,
	responsibility driver.Responsibility,
	before string,
) bool {
	if state.Plan.TargetStale {
		return false
	}
	switch {
	case slice != "":
		observed := sliceFingerprint(state, slice)
		return observed != "" && observed == before
	case responsibility == driver.AssemblyVerification:
		return state.Assembly.Candidate != nil &&
			state.Assembly.Candidate.Receipt.Candidate != nil &&
			workIdentity(
				state.Plan.OID,
				state.Refs.Release.Head,
				state.Refs.Target.Head,
				*state.Assembly.Candidate.Receipt.Candidate,
			) == before
	default:
		return false
	}
}

func (s *Service) dispatchRole(ctx context.Context, engine *engine, workspace *gitx.WorkspaceLease,
	role driver.Role, slice string, responsibility driver.Responsibility, batonAttempt int64,
	before string, owner journal.OwnerLease) (driver.Submission, error) {
	return s.dispatchRoleWithScope(ctx, engine, workspace, role, slice, responsibility, batonAttempt, before, owner, "")
}

func (s *Service) dispatchRoleWithScope(ctx context.Context, engine *engine, workspace *gitx.WorkspaceLease,
	role driver.Role, slice string, responsibility driver.Responsibility, batonAttempt int64,
	before string, owner journal.OwnerLease, invocationScope string) (driver.Submission, error) {
	if responsibility != driver.PlannerProposal && responsibility != driver.CaptainPlanReview {
		fresh, err := baton.ReadState(engine.git, engine.manifest.value.Release, engine.inertness)
		if err != nil {
			return driver.Submission{}, runtimeFail("BATON_UNAVAILABLE", err)
		}
		if !dispatchAuthorityCurrent(fresh, slice, responsibility, before) {
			return driver.Submission{}, runtimeFail("STALE_DISPATCH", nil)
		}
	}
	workID := driverWorkIdentity(
		engine.manifest.digest, slice, responsibility, batonAttempt, before)
	projection, err := s.journal.ControlProjection(ctx, engine.manifest.value.RunID)
	if err != nil {
		return driver.Submission{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	epoch := projection.RetryEpochs[workID]
	if epoch == 0 {
		epoch = 1
	}
	maximumAttempts := int64(3)
	if responsibility == driver.CaptainPlanReview {
		snapshot, snapshotErr := s.journal.Snapshot(ctx, engine.manifest.value.RunID)
		if snapshotErr != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_READ_FAILED", snapshotErr)
		}
		delegation, delegationErr := currentCaptainDelegation(snapshot)
		if delegationErr != nil || !delegation.Active {
			return driver.Submission{}, runtimeFail("CAPTAIN_DECISION_STALE", delegationErr)
		}
		maximumAttempts = delegation.Envelope.Limits.MaximumCaptainAttemptsPerProposal
	}
	for try := int64(1); try <= 3; try++ {
		if responsibility == driver.CaptainPlanReview {
			attemptID := journal.AttemptEffectID(workID, epoch, try)
			_, existingErr := s.journal.Effect(ctx, engine.manifest.value.RunID, attemptID)
			if journal.IsCode(existingErr, "EFFECT_NOT_FOUND") {
				snapshot, snapshotErr := s.journal.Snapshot(ctx, engine.manifest.value.RunID)
				if snapshotErr != nil {
					return driver.Submission{}, runtimeFail("JOURNAL_READ_FAILED", snapshotErr)
				}
				count, countErr := captainDispatchAttemptCount(snapshot, workID)
				if countErr != nil {
					return driver.Submission{}, countErr
				}
				if count >= maximumAttempts {
					return driver.Submission{}, runtimeFail("CAPTAIN_ATTEMPTS_EXHAUSTED", nil)
				}
			} else if existingErr != nil {
				return driver.Submission{}, runtimeFail("JOURNAL_READ_FAILED", existingErr)
			}
		}
		submission, err := s.runDriverEffect(
			ctx,
			engine,
			workspace,
			role,
			dispatchCoordinates{
				Slice:           slice,
				Responsibility:  responsibility,
				BatonAttempt:    batonAttempt,
				Epoch:           epoch,
				Try:             try,
				InvocationScope: invocationScope,
			},
			journal.EffectAttempt{WorkID: workID, Epoch: epoch, Try: try}, before, owner)
		if err == nil {
			return submission, nil
		}
		if IsCode(err, "CONTINUATION_CLEANUP_FAILED") {
			return driver.Submission{}, err
		}
		effect, readErr := s.journal.Effect(ctx, engine.manifest.value.RunID,
			journal.AttemptEffectID(workID, epoch, try))
		if readErr != nil || effect.State != journal.OperationalFailed {
			return driver.Submission{}, err
		}
	}
	return driver.Submission{}, runtimeFail("EFFECT_PARKED", nil)
}

func persistedBatonAction(engine *engine, kind string,
	command batonActionCommand,
) (
	func() (baton.ActionResult, error),
	func() error,
	error,
) {
	actions := engine.actions
	installer := engine.installer
	if command.GitIdentity != engine.manifest.value.GitIdentity {
		var err error
		actions, err = baton.NewActions(engine.git, engine.inertness, command.GitIdentity)
		if err != nil {
			return nil, nil, runtimeFail("CORRUPT_JOURNAL", err)
		}
		installer = newAuthorityInstaller(actions)
	}
	switch kind {
	case "baton.append_receipt", "baton.assembly_verdict":
		var input baton.AppendReceiptInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Release != command.Authority.Release {
			return nil, nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return func() (baton.ActionResult, error) {
			return actions.AppendReceipt(input)
		}, nil, nil
	case "baton.prepare_assembly":
		var input baton.PrepareAssemblyInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Release != command.Authority.Release {
			return nil, nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		var cleanupErr error
		return func() (baton.ActionResult, error) {
				result, actionErr, closeErr := withReleaseAssemblyAuthority(
					engine,
					command.Authority.Release,
					command.Authority.ReleaseHead,
					func() (baton.ActionResult, error) {
						return actions.PrepareAssembly(input)
					},
				)
				cleanupErr = errors.Join(cleanupErr, closeErr)
				return result, actionErr
			}, func() error {
				return cleanupErr
			}, nil
	case "baton.merge":
		var input baton.MergePassedCandidateInput
		if parseCanonicalActionInput(command.Input, &input) != nil ||
			input.Release != command.Authority.Release {
			return nil, nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		var cleanupErr error
		return func() (baton.ActionResult, error) {
				result, actionErr, closeErr := withReleaseAssemblyAuthority(
					engine,
					command.Authority.Release,
					command.Authority.ReleaseHead,
					func() (baton.ActionResult, error) {
						return actions.MergePassedCandidate(input)
					},
				)
				cleanupErr = errors.Join(cleanupErr, closeErr)
				return result, actionErr
			}, func() error {
				return cleanupErr
			}, nil
	case "baton.install":
		var input installActionInput
		if parseCanonicalActionInput(command.Input, &input) != nil {
			return nil, nil, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if _, err := validateInstallActionInput(input); err != nil {
			return nil, nil, err
		}
		admission := approvalAdmission{
			planBytes: input.PlanBytes, planDigest: input.PlanDigest,
			reference: input.Reference,
		}
		return func() (baton.ActionResult, error) {
			return installer.install(admission, command.Authority.TargetHead)
		}, nil, nil
	default:
		return nil, nil, runtimeFail("CORRUPT_JOURNAL", nil)
	}
}

func (s *Service) finishClaimedAction(ctx context.Context, owner journal.OwnerLease,
	effect journal.Effect, result baton.ActionResult, fresh bool) error {
	body := mustJSON(result)
	completion := journal.Completion{
		RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim,
		State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "baton_action_result", Body: body}},
		EventKind: "baton_action_recovered", EventBody: []byte(effect.Kind),
		At: s.now().UTC(),
	}
	if fresh {
		completion.EventKind = "baton_action_completed"
		if err := s.journal.CompleteOwned(
			context.WithoutCancel(ctx), owner, completion); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return nil
	}
	if err := s.journal.ReconcileOwned(
		context.WithoutCancel(ctx), owner, completion, journal.RecoveryAllNew); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) finishClaimedFailure(ctx context.Context, owner journal.OwnerLease,
	effect journal.Effect, code string) error {
	completion := journal.Completion{
		RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim,
		State: journal.OperationalFailed, ErrorCode: code,
		EventKind: "effect_operational_failure", EventBody: []byte(effect.Kind),
		At: s.now().UTC(),
	}
	if err := s.journal.CompleteOwned(
		context.WithoutCancel(ctx), owner, completion); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) reconcileClaimedBatonAction(ctx context.Context, engine *engine,
	owner journal.OwnerLease, effect journal.Effect, command batonActionCommand,
	action func() (baton.ActionResult, error), fresh, allowCrash bool,
) (actionTruth, baton.ActionResult, error) {
	truth, observed, err := classifyBatonAction(engine, effect.Kind, command)
	if err != nil && truth != actionAmbiguous {
		return truth, baton.ActionResult{}, err
	}
	if truth == actionStale &&
		fresh &&
		effect.Kind == "baton.install" &&
		installActionIdempotentlyCallable(observed, command) {
		truth = actionAllOld
	}
	switch truth {
	case actionStale:
		if err := s.finishClaimedFailure(
			ctx, owner, effect, "stale_authority"); err != nil {
			return truth, baton.ActionResult{}, err
		}
		return truth, baton.ActionResult{}, nil
	case actionAmbiguous:
		_ = s.journal.ReconcileOwned(context.WithoutCancel(ctx), owner,
			journal.Completion{
				RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim,
				EventKind: "baton_action_uncertain", EventBody: []byte(effect.Kind),
				At: s.now().UTC(),
			}, journal.RecoveryAmbiguous)
		return truth, baton.ActionResult{}, runtimeFail("RECOVERY_UNCERTAIN", err)
	case actionAllNew:
		result, err := reconstructAllNewBatonAction(
			observed, effect.Kind, command)
		if err != nil {
			_ = s.journal.ReconcileOwned(context.WithoutCancel(ctx), owner,
				journal.Completion{
					RunID: owner.RunID, EffectID: effect.ID,
					Token: effect.CurrentClaim, EventKind: "baton_action_uncertain",
					EventBody: []byte(effect.Kind), At: s.now().UTC(),
				}, journal.RecoveryAmbiguous)
			return actionAmbiguous, baton.ActionResult{},
				runtimeFail("RECOVERY_UNCERTAIN", err)
		}
		if err := s.finishClaimedAction(
			ctx, owner, effect, result, false); err != nil {
			return truth, baton.ActionResult{}, err
		}
		return actionAllNew, result, nil
	case actionAllOld:
		// Only an exact all-old authority may invoke the persisted mutation.
		if err := validateBatonAllOldStateAuthority(
			observed,
			effect.Kind,
			command,
		); err != nil {
			return actionAmbiguous, baton.ActionResult{}, err
		}
	default:
		return actionAmbiguous, baton.ActionResult{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	engine.actionMu.Lock()
	result, actionErr := action()
	engine.actionMu.Unlock()
	if actionErr != nil {
		after, afterState, classifyErr := classifyBatonAction(
			engine, effect.Kind, command)
		switch after {
		case actionStale:
			if err := s.finishClaimedFailure(
				ctx, owner, effect, "stale_authority"); err != nil {
				return after, baton.ActionResult{}, err
			}
			return after, baton.ActionResult{}, nil
		case actionAllNew:
			recovered, recoverErr := reconstructAllNewBatonAction(
				afterState, effect.Kind, command)
			if recoverErr != nil {
				_ = s.journal.ReconcileOwned(context.WithoutCancel(ctx), owner,
					journal.Completion{
						RunID: owner.RunID, EffectID: effect.ID,
						Token: effect.CurrentClaim, EventKind: "baton_action_uncertain",
						EventBody: []byte(effect.Kind), At: s.now().UTC(),
					}, journal.RecoveryAmbiguous)
				return actionAmbiguous, baton.ActionResult{},
					runtimeFail(
						"RECOVERY_UNCERTAIN",
						errors.Join(actionErr, classifyErr, recoverErr),
					)
			}
			if err := s.finishClaimedAction(
				ctx, owner, effect, recovered, false); err != nil {
				return after, baton.ActionResult{}, err
			}
			return actionAllNew, recovered, nil
		case actionAmbiguous:
			_ = s.journal.ReconcileOwned(context.WithoutCancel(ctx), owner,
				journal.Completion{
					RunID: owner.RunID, EffectID: effect.ID,
					Token: effect.CurrentClaim, EventKind: "baton_action_uncertain",
					EventBody: []byte(effect.Kind), At: s.now().UTC(),
				}, journal.RecoveryAmbiguous)
			return actionAmbiguous, baton.ActionResult{},
				runtimeFail("RECOVERY_UNCERTAIN", errors.Join(actionErr, classifyErr))
		default:
			if err := s.finishClaimedFailure(
				ctx, owner, effect, "baton_action_failed"); err != nil {
				return after, baton.ActionResult{}, err
			}
			return after, baton.ActionResult{}, actionErr
		}
	}
	expected, attestErr := reconstructSucceededBatonAction(
		engine,
		effect.Kind,
		command,
	)
	resultMismatch := !actionResultMatchesDurableTruth(result, expected)
	if attestErr != nil || resultMismatch {
		if resultMismatch {
			attestErr = errors.Join(
				attestErr,
				errors.New(
					"baton callback result does not match durable truth for "+
						effect.Kind,
				),
			)
		}
		_ = s.journal.ReconcileOwned(
			context.WithoutCancel(ctx),
			owner,
			journal.Completion{
				RunID: owner.RunID, EffectID: effect.ID,
				Token:     effect.CurrentClaim,
				EventKind: "baton_action_uncertain",
				EventBody: []byte(effect.Kind),
				At:        s.now().UTC(),
			},
			journal.RecoveryAmbiguous,
		)
		return actionAmbiguous, baton.ActionResult{},
			runtimeFail(
				"RECOVERY_UNCERTAIN",
				errors.Join(
					attestErr,
					errors.New("baton callback attestation failed"),
				),
			)
	}
	if allowCrash && testCrashAfterEffect == effect.Kind {
		os.Exit(86)
	}
	if effect.Kind == "baton.install" && testCaptainCrashCut == "baton_mutation" {
		return actionAllNew, result, runtimeFail("TEST_CAPTAIN_CRASH_CUT", nil)
	}
	if err := s.finishClaimedAction(ctx, owner, effect, result, fresh); err != nil {
		return truth, baton.ActionResult{}, err
	}
	return actionAllNew, result, nil
}

func (s *Service) runAction(ctx context.Context, engine *engine, owner journal.OwnerLease,
	workID, kind string, payload []byte,
	_ func() (baton.ActionResult, error)) (result baton.ActionResult, resultErr error) {
	persisted, err := parseActionCommand(payload)
	if err != nil {
		return baton.ActionResult{}, runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(
				err,
				errors.New("baton action command admission failed"),
			),
		)
	}
	action, cleanup, err := persistedBatonAction(engine, kind, persisted)
	if err != nil {
		return baton.ActionResult{}, err
	}
	defer func() {
		if cleanup != nil {
			resultErr = errors.Join(resultErr, cleanup())
		}
	}()
	projection, err := s.journal.ControlProjection(ctx, engine.manifest.value.RunID)
	if err != nil {
		return baton.ActionResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	epoch := projection.RetryEpochs[workID]
	if epoch == 0 {
		epoch = 1
	}
	for try := int64(1); try <= 3; try++ {
		id := journal.AttemptEffectID(workID, epoch, try)
		now := s.now().UTC()
		command := journal.Command{RunID: engine.manifest.value.RunID, ReplayKey: id,
			Kind: kind, Payload: payload, CreatedAt: now}
		effectInput := journal.Effect{RunID: command.RunID, ID: id, ReplayKey: id,
			Kind: kind, BeforeDigest: workID, ExpectedDigest: sha256Digest(payload), UpdatedAt: now}
		if err := s.journal.EnsureAttempt(ctx, command, effectInput,
			journal.EffectAttempt{WorkID: workID, Epoch: epoch, Try: try}); err != nil {
			return baton.ActionResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		effect, err := s.journal.Effect(ctx, command.RunID, id)
		if err != nil {
			return baton.ActionResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		if err := validateBatonActionEnvelope(
			engine,
			command,
			effect,
			persisted,
		); err != nil {
			return baton.ActionResult{}, runtimeFail(
				stableErrorCode(err),
				errors.Join(
					err,
					errors.New("baton action envelope admission failed"),
				),
			)
		}
		if effect.State == journal.Succeeded {
			return validateSucceededBatonAction(
				engine,
				command,
				effect,
				persisted,
			)
		}
		if effect.State == journal.Claimed {
			truth, recovered, recoverErr := s.reconcileClaimedBatonAction(
				ctx, engine, owner, effect, persisted, action, false, false)
			if recoverErr != nil {
				if truth == actionAllOld {
					continue
				}
				return baton.ActionResult{}, recoverErr
			}
			if truth == actionStale {
				return baton.ActionResult{}, runtimeFail("STALE_DISPATCH", nil)
			}
			return recovered, nil
		}
		if effect.State != journal.Pending {
			if effect.State == journal.OperationalFailed {
				continue
			}
			return baton.ActionResult{}, runtimeFail("RECOVERY_UNCERTAIN", nil)
		}
		claim, err := s.journal.ClaimOwned(ctx, owner, id, now, effectLease)
		if err != nil {
			return baton.ActionResult{}, runtimeFail("EFFECT_CLAIM_FAILED", err)
		}
		effect.State, effect.CurrentClaim = journal.Claimed, claim.Token
		if testCrashBeforeEffect == kind {
			os.Exit(86)
		}
		truth, result, actionErr := s.reconcileClaimedBatonAction(
			ctx, engine, owner, effect, persisted, action, true, true)
		if actionErr != nil {
			if truth == actionAllOld &&
				!IsCode(actionErr, "RECOVERY_UNCERTAIN") {
				continue
			}
			return baton.ActionResult{}, actionErr
		}
		if truth == actionStale {
			return baton.ActionResult{}, runtimeFail("STALE_DISPATCH", nil)
		}
		return result, nil
	}
	return baton.ActionResult{}, runtimeFail("EFFECT_PARKED", nil)
}

func sliceAttempt(state baton.State, sliceID string) int64 {
	slice, ok := state.Slice(sliceID)
	if !ok || slice.Attempt < 1 {
		return 0
	}
	return slice.Attempt
}

func planFromState(state baton.State) (baton.Plan, error) {
	for _, history := range state.Plan.History {
		if history.OID != state.Plan.OID {
			continue
		}
		return history.Plan, nil
	}
	return baton.Plan{}, runtimeFail("CORRUPT_JOURNAL", nil)
}

func sliceFingerprint(state baton.State, sliceID string) string {
	slice, ok := state.Slice(sliceID)
	if !ok {
		return ""
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		return ""
	}
	return sliceFingerprintAtTrackHead(state, sliceID, track.Head)
}

func sliceFingerprintAtTrackHead(
	state baton.State,
	sliceID string,
	trackHead string,
) string {
	return sliceFingerprintAtAuthority(
		state,
		sliceID,
		state.Refs.Target.Head,
		trackHead,
	)
}

func sliceFingerprintAtAuthority(
	state baton.State,
	sliceID string,
	targetHead string,
	trackHead string,
) string {
	slice, ok := state.Slice(sliceID)
	if !ok {
		return ""
	}
	current := ""
	if slice.CurrentReceipt != nil {
		current = slice.CurrentReceipt.OID
	}
	return workIdentity(state.Plan.OID, targetHead, trackHead, sliceID,
		slice.Stage, slice.NextRole, slice.Attempt, current, slice.InputPins)
}

func (s *Service) appendReceipt(ctx context.Context, engine *engine, owner journal.OwnerLease,
	state baton.State, expectedBefore string, input baton.AppendReceiptInput) error {
	fresh, err := baton.ReadState(engine.git, state.Release, engine.inertness)
	if err != nil {
		return runtimeFail("BATON_UNAVAILABLE", err)
	}
	if fresh.Plan.TargetStale {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	state = fresh
	before := sliceFingerprint(state, input.Slice)
	if expectedBefore != "" && before != expectedBefore {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	workID := workIdentity(before, "append", input.Role, input.Result, input.Candidate)
	action := func() (baton.ActionResult, error) { return engine.actions.AppendReceipt(input) }
	slice, ok := state.Slice(input.Slice)
	if !ok || slice.CurrentReceipt == nil {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	authority := stateActionAuthority(
		state, track.Ref, track.Head, before, slice.CurrentReceipt.OID,
		input.Candidate, slice.Attempt)
	_, err = s.runAction(ctx, engine, owner, workID, "baton.append_receipt",
		marshalActionCommand(engine.manifest.value.GitIdentity, authority, input), action)
	return err
}

func exactDesignContinuationPromotion(
	entry *retainedContinuation,
	runID string,
	state baton.State,
	slice *baton.SliceState,
	track *baton.TrackState,
) bool {
	if entry == nil || entry.handle == nil ||
		entry.designReceipt != "" ||
		entry.selectionDigest == "" ||
		!runtimeDigestPattern.MatchString(entry.before) ||
		slice == nil || track == nil ||
		slice.CurrentReceipt == nil ||
		state.Plan.TargetStale ||
		entry.binding.RunID != runID ||
		entry.binding.Release != state.Release ||
		entry.binding.Slice != slice.Location.Slice.ID ||
		entry.binding.Attempt != slice.Attempt ||
		entry.before != workIdentity(
			state.Plan.OID,
			state.Refs.Target.Head,
			entry.sourceReceipt,
			slice.Location.Slice.ID,
			"design",
			"implementer",
			slice.Attempt,
			entry.sourceReceipt,
			slice.InputPins,
		) ||
		slice.CurrentReceipt.OID != track.Head ||
		slice.CurrentReceipt.Parent != entry.sourceReceipt ||
		slice.CurrentReceipt.Receipt.Binds != entry.sourceReceipt ||
		slice.CurrentReceipt.Receipt.Role != "implementer" ||
		slice.CurrentReceipt.Receipt.Result != "designed" ||
		slice.CurrentReceipt.Receipt.Attempt == nil ||
		*slice.CurrentReceipt.Receipt.Attempt != slice.Attempt ||
		slice.CurrentReceipt.Receipt.Plan != state.Plan.OID ||
		slice.CurrentReceipt.Receipt.SliceID() !=
			slice.Location.Slice.ID ||
		track.Ref != "refs/heads/track/"+state.Release+"/"+
			slice.Location.Track.ID {
		return false
	}
	planDigest := continuationPlanDigest(
		state.Plan.OID,
		state.Plan.Digest,
		state.Plan.Metadata.Revision,
	)
	targetDigest := continuationTargetDigest(
		state.Refs.Target.Ref,
		state.Refs.Target.Head,
		slice.Location.Track.ID,
		track.Ref,
		slice.PreparedBase,
		sliceEvidence(slice.ConsumedInputs),
	)
	return entry.binding.PlanAuthorityDigest == planDigest &&
		entry.binding.TargetAuthorityDigest == targetDigest &&
		entry.binding.ToolContractDigest != ""
}

func (s *Service) promoteDesignContinuation(
	ctx context.Context,
	engine *engine,
	sliceID string,
) error {
	runID := engine.manifest.value.RunID
	entry := s.takeContinuation(runID, sliceID)
	if entry == nil {
		return nil
	}
	state, err := baton.ReadState(
		engine.git,
		engine.manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		return closeRetainedContinuation(entry)
	}
	slice, ok := state.Slice(sliceID)
	if !ok {
		return closeRetainedContinuation(entry)
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok ||
		!exactDesignContinuationPromotion(
			entry,
			runID,
			state,
			slice,
			track,
		) {
		return closeRetainedContinuation(entry)
	}
	entry.designReceipt = slice.CurrentReceipt.OID
	return s.storeContinuation(runID, sliceID, entry)
}

func (s *Service) promoteVerifierContinuation(
	engine *engine,
	sliceID string,
) error {
	runID := engine.manifest.value.RunID
	entry := s.takeRetainedContinuation(
		runID, continuationVerifier, sliceID,
	)
	if entry == nil {
		return nil
	}
	state, err := baton.ReadState(
		engine.git,
		engine.manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		return closeRetainedContinuation(entry)
	}
	slice, ok := state.Slice(sliceID)
	if !ok || slice.CurrentReceipt == nil {
		return closeRetainedContinuation(entry)
	}
	track, trackOK := state.Track(slice.Location.Track.ID)
	fail := slice.CurrentReceipt
	current := entry.binding
	current.RunID, current.Release, current.Slice =
		runID, state.Release, slice.Location.Slice.ID
	current.PlanAuthorityDigest = continuationPlanDigest(
		state.Plan.OID, state.Plan.Digest, state.Plan.Metadata.Revision,
	)
	if trackOK {
		current.TargetAuthorityDigest = continuationTargetDigest(
			state.Refs.Target.Ref, state.Refs.Target.Head,
			slice.Location.Track.ID, track.Ref, "",
			sliceEvidence(slice.ConsumedInputs),
		)
	}
	if !trackOK || state.Plan.TargetStale ||
		entry.handle == nil || entry.verifierFailReceipt != "" ||
		entry.sourceReceipt == "" ||
		fail.OID != track.Head || fail.Parent != entry.sourceReceipt ||
		fail.Receipt.Role != "verifier" || fail.Receipt.Result != "fail" ||
		fail.Receipt.Attempt == nil ||
		*fail.Receipt.Attempt != entry.binding.Attempt ||
		fail.Receipt.Binds != entry.sourceReceipt ||
		!sameStableContinuationAuthority(
			entry, current, entry.selectionDigest,
		) {
		return closeRetainedContinuation(entry)
	}
	entry.verifierFailReceipt = fail.OID
	return s.storeRetainedContinuation(
		runID, continuationVerifier, sliceID, entry,
	)
}

func (s *Service) advanceSlice(ctx context.Context, engine *engine, owner journal.OwnerLease,
	sliceID string) error {
	state, err := baton.ReadState(engine.git, engine.manifest.value.Release, engine.inertness)
	if err != nil {
		return runtimeFail("BATON_UNAVAILABLE", err)
	}
	if state.Plan.TargetStale {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	slice, ok := state.Slice(sliceID)
	if !ok || slice.Status != "ready" {
		return runtimeFail("WORK_NOT_READY", nil)
	}
	if slice.NextRole == "implementer" && slice.Stage == "design" {
		state, slice, err = s.prepareTrackBaseForSlice(
			ctx,
			engine,
			owner,
			state,
			slice,
		)
		if err != nil {
			return err
		}
	}
	key := gitx.TrackKey{Release: state.Release, Track: slice.Location.Track.ID}
	before := sliceFingerprint(state, sliceID)
	switch {
	case slice.NextRole == "implementer" && slice.Stage == "design":
		workspace, err := engine.workspaces.OpenTrack(key, gitx.DesignView)
		if err != nil {
			return runtimeFail("WORKSPACE_UNAVAILABLE", err)
		}
		submission, runErr := s.dispatchRole(ctx, engine, workspace, driver.RoleImplementer,
			sliceID, driver.ImplementerDesign, slice.Attempt, before, owner)
		closeErr := workspace.Close()
		if runErr != nil {
			if cleanupErr := s.discardContinuation(
				engine.manifest.value.RunID,
				sliceID,
			); cleanupErr != nil {
				return cleanupErr
			}
			return runErr
		}
		if closeErr != nil {
			if cleanupErr := s.discardContinuation(
				engine.manifest.value.RunID,
				sliceID,
			); cleanupErr != nil {
				return cleanupErr
			}
			return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
		}
		appendErr := s.appendReceipt(
			ctx,
			engine,
			owner,
			state,
			before,
			baton.AppendReceiptInput{
				Release: state.Release, Slice: sliceID,
				Role: "implementer", Result: "designed",
				Summary: submission.Summary,
				Detail:  []byte(submission.Detail),
			},
		)
		if appendErr != nil {
			if cleanupErr := s.discardContinuation(
				engine.manifest.value.RunID,
				sliceID,
			); cleanupErr != nil {
				return cleanupErr
			}
			return appendErr
		}
		return s.promoteDesignContinuation(ctx, engine, sliceID)
	case slice.NextRole == "captain":
		workspace, err := engine.workspaces.OpenTrack(key, gitx.CaptainView)
		if err != nil {
			return runtimeFail("WORKSPACE_UNAVAILABLE", err)
		}
		submission, runErr := s.dispatchRole(ctx, engine, workspace, driver.RoleCaptain,
			sliceID, driver.CaptainReview, slice.Attempt, before, owner)
		closeErr := workspace.Close()
		if runErr != nil {
			return runErr
		}
		if closeErr != nil {
			return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
		}
		appendErr := s.appendReceipt(
			ctx,
			engine,
			owner,
			state,
			before,
			baton.AppendReceiptInput{
				Release: state.Release, Slice: sliceID,
				Role:    "captain",
				Result:  string(submission.Decision.Outcome),
				Summary: submission.Summary,
				Detail:  []byte(submission.Detail),
			},
		)
		if appendErr != nil {
			return appendErr
		}
		if submission.Decision.Outcome != driver.DecisionProceed {
			return s.discardContinuation(
				engine.manifest.value.RunID,
				sliceID,
			)
		}
		return nil
	case slice.NextRole == "implementer" && slice.Stage == "implement":
		return s.implementSlice(ctx, engine, owner, state, slice)
	case slice.NextRole == "verifier":
		if slice.Candidate == nil || slice.Candidate.Receipt.Candidate == nil {
			return runtimeFail("CHANGED_CANDIDATE", nil)
		}
		candidate := *slice.Candidate.Receipt.Candidate
		oid, err := gitx.ParseOID(engine.repository.ObjectFormat(), candidate)
		if err != nil {
			return runtimeFail("INVALID_CANDIDATE", err)
		}
		workspace, err := engine.workspaces.OpenCandidate(key, gitx.WorkVerifierView, oid)
		if err != nil {
			return runtimeFail("WORKSPACE_UNAVAILABLE", err)
		}
		submission, runErr := s.dispatchRole(ctx, engine, workspace, driver.RoleVerifier,
			sliceID, driver.WorkVerification, slice.Attempt, before, owner)
		closeErr := workspace.Close()
		discardVerifier := func(cause error) error {
			return errors.Join(
				cause,
				s.discardRetainedContinuation(
					engine.manifest.value.RunID,
					continuationVerifier,
					sliceID,
				),
			)
		}
		if runErr != nil {
			return discardVerifier(runErr)
		}
		if closeErr != nil {
			return discardVerifier(
				runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr),
			)
		}
		checks, err := exactBytes(submission.Checks)
		if err != nil {
			return discardVerifier(err)
		}
		// For a host-check slice, the engine rebuilds the verifier's checks
		// evidence as a provenance manifest: it reuses the exactly-once
		// journaled host-boundary results (never re-running them in a worker)
		// and references the verifier's own submitted worker-runnable check
		// bytes by digest. The verifier's submission stays non-empty even when
		// the contract declares no worker-runnable checks at all, because the
		// projected host evidence is its checks bytes.
		plan, planErr := planFromState(state)
		if planErr != nil {
			return discardVerifier(planErr)
		}
		hostChecks, contractDigest, resolveErr := resolveSliceHostChecks(
			engine, plan, sliceID, state.Refs.Target.Head)
		if resolveErr != nil {
			return discardVerifier(resolveErr)
		}
		if len(hostChecks) > 0 {
			hostResults, runErr := s.runHostChecks(
				ctx, engine, owner, plan, sliceID,
				candidate, state.Refs.Target.Head)
			if runErr != nil {
				return discardVerifier(runErr)
			}
			manifest, buildErr := buildHostCheckResultsManifest(
				state.Release, sliceID, slice.Attempt, candidate,
				contractDigest, hostResults, baton.DigestBytes(checks))
			if buildErr != nil {
				return discardVerifier(buildErr)
			}
			checks = manifest
		}
		appendErr := s.appendReceipt(ctx, engine, owner, state, before, baton.AppendReceiptInput{
			Release: state.Release, Slice: sliceID, Role: "verifier",
			Result: string(submission.Decision.Outcome), Summary: submission.Summary,
			Detail: []byte(submission.Detail), Candidate: candidate, CheckResults: checks})
		if appendErr != nil {
			return discardVerifier(appendErr)
		}
		if submission.Decision.Outcome == driver.DecisionFail {
			return s.promoteVerifierContinuation(
				engine,
				sliceID,
			)
		}
		return discardVerifier(nil)
	}
	return runtimeFail("WORK_NOT_READY", nil)
}

func (s *Service) implementSlice(ctx context.Context, engine *engine, owner journal.OwnerLease,
	state baton.State, slice *baton.SliceState) error {
	sliceID := slice.Location.Slice.ID
	if recovered, err := s.recoverPendingImplementationForSlice(
		ctx, engine, owner, state, slice); recovered {
		return err
	}
	var err error
	state, slice, err = s.prepareTrackBaseForSlice(
		ctx,
		engine,
		owner,
		state,
		slice,
	)
	if err != nil {
		return err
	}
	key := gitx.TrackKey{Release: state.Release, Track: slice.Location.Track.ID}
	before := sliceFingerprint(state, sliceID)
	workID := workIdentity(before, "git.seal")
	projection, err := s.journal.ControlProjection(ctx, engine.manifest.value.RunID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	epoch := projection.RetryEpochs[workID]
	if epoch == 0 {
		epoch = 1
	}
	if slice.CurrentReceipt == nil {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	track, ok := state.Track(key.Track)
	if !ok {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	for try := int64(1); try <= 3; try++ {
		effectID := journal.AttemptEffectID(workID, epoch, try)
		dispatchWork := workIdentity(effectID, "driver.dispatch")
		preparedWork := workIdentity(effectID, "git.seal.prepared")
		childEpoch, childTry := int64(1), int64(1)
		if _, enabled := engine.manifest.value.recoverySelection(); enabled {
			dispatchWork = workIdentity(workID, "driver.dispatch")
			preparedWork = workIdentity(workID, "git.seal.prepared")
			childEpoch, childTry = epoch, try
		}
		refresh := candidateHeadRefresh(state, slice)
		cycle := implementationCycle{
			Release: state.Release, GitIdentity: engine.manifest.value.GitIdentity, Slice: sliceID,
			Binds: slice.CurrentReceipt.OID, Before: before,
			Plan: state.Plan.OID, ReleaseHead: state.Refs.Release.Head,
			TargetHead: state.Refs.Target.Head, Track: key.Track, TrackRef: track.Ref,
			TrackHead: track.Head, DispatchWork: dispatchWork,
			DispatchEffect: journal.AttemptEffectID(
				dispatchWork,
				childEpoch,
				childTry,
			),
			PreparedWork: preparedWork,
			PreparedEffect: journal.AttemptEffectID(
				preparedWork,
				childEpoch,
				childTry,
			),
		}
		if refresh {
			cycle.RefreshFrom = slice.CurrentReceipt.OID
		}
		if len(slice.Location.Slice.Consumes) > 0 {
			if slice.PreparedBase == "" ||
				(slice.PreparedBase != track.Head && !refresh) {
				return runtimeFail("STALE_DISPATCH", nil)
			}
			cycle.Base = slice.PreparedBase
		}
		now := s.now().UTC()
		payload := mustJSON(cycle)
		if err := s.journal.EnsureAttempt(ctx,
			journal.Command{RunID: owner.RunID, ReplayKey: effectID,
				Kind: "git.seal", Payload: payload, CreatedAt: now},
			journal.Effect{RunID: owner.RunID, ID: effectID, ReplayKey: effectID,
				Kind: "git.seal", BeforeDigest: workID,
				ExpectedDigest: sha256Digest(payload), UpdatedAt: now},
			journal.EffectAttempt{WorkID: workID, Epoch: epoch, Try: try}); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		effect, err := s.journal.Effect(ctx, owner.RunID, effectID)
		if err != nil {
			return runtimeFail("JOURNAL_READ_FAILED", err)
		}
		var claim journal.Claim
		resumingAnswer := false
		switch effect.State {
		case journal.Succeeded:
			var record sealedRecord
			if json.Unmarshal(effect.Result, &record) != nil ||
				!sealedRecordMatchesCycle(record, cycle) {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			return s.appendImplementationReceipt(ctx, engine, owner, cycle, record)
		case journal.Claimed:
			attention, found, attentionErr := s.attentionForWork(
				ctx,
				owner.RunID,
				cycle.DispatchWork,
			)
			if attentionErr != nil {
				return attentionErr
			}
			if found {
				dispatch, dispatchErr := s.journal.Effect(
					ctx,
					owner.RunID,
					cycle.DispatchEffect,
				)
				if dispatchErr != nil {
					return runtimeFail("JOURNAL_READ_FAILED", dispatchErr)
				}
				if dispatch.State != journal.Claimed {
					return runtimeFail("CORRUPT_JOURNAL", nil)
				}
				if attention.State == journal.AttentionOpen {
					return runtimeFail("EFFECT_PARKED", nil)
				}
				resumingAnswer = true
				claim = journal.Claim{
					RunID:    owner.RunID,
					EffectID: effectID,
					Token:    effect.CurrentClaim,
				}
				break
			}
			record, retry, recoverErr := s.recoverImplementationCycle(
				ctx, engine, owner, cycle, effect)
			if recoverErr != nil {
				return recoverErr
			}
			if retry {
				continue
			}
			return s.appendImplementationReceipt(ctx, engine, owner, cycle, record)
		case journal.OperationalFailed:
			continue
		case journal.Pending:
			// Claimed below.
		default:
			return runtimeFail("RECOVERY_UNCERTAIN", nil)
		}
		if claim.Token == "" {
			claim, err = s.journal.ClaimOwned(ctx, owner, effectID, now, effectLease)
			if err != nil {
				return runtimeFail("EFFECT_CLAIM_FAILED", err)
			}
		}
		var record sealedRecord
		record, err = s.runImplementationCycle(
			ctx,
			engine,
			owner,
			state,
			slice,
			key,
			cycle,
			dispatchCoordinates{
				Slice:          sliceID,
				Responsibility: driver.ImplementerImplementation,
				BatonAttempt:   slice.Attempt,
				Epoch:          epoch,
				Try:            try,
			},
			journal.Effect{
				RunID: owner.RunID, ID: effectID,
				State: journal.Claimed, CurrentClaim: claim.Token,
			},
		)
		if err == nil {
			body := mustJSON(record)
			if err := s.journal.CompleteOwned(context.WithoutCancel(ctx), owner,
				journal.Completion{RunID: owner.RunID, EffectID: effectID,
					Token: claim.Token, State: journal.Succeeded, Result: body,
					Receipts:  []journal.Receipt{{Kind: "git_candidate", Body: body}},
					EventKind: "candidate_sealed", EventBody: body, At: s.now().UTC()}); err != nil {
				return runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
			return s.appendImplementationReceipt(ctx, engine, owner, cycle, record)
		}
		if IsCode(err, "RECOVERY_UNCERTAIN") {
			return err
		}
		if IsCode(err, "CONTINUATION_CLEANUP_FAILED") {
			return err
		}
		if IsCode(err, "EFFECT_PARKED") {
			return err
		}
		if resumingAnswer {
			// The durable answer is the lane's wake token. A failure before
			// its dispatch is committed must leave the same claimed cycle
			// replayable; advancing to a fresh outer try would orphan it.
			return err
		}
		if completeErr := s.completeImplementationFailure(
			context.WithoutCancel(ctx), owner, effectID, claim.Token,
			stableErrorCode(err), extractRefusalResult(err)); completeErr != nil {
			return completeErr
		}
		if IsCode(err, "STALE_DISPATCH") {
			return err
		}
	}
	return runtimeFail("EFFECT_PARKED", nil)
}

func (s *Service) executeClaimedImplementationCycle(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	state baton.State,
	slice *baton.SliceState,
	active activeImplementationCycle,
) error {
	if slice == nil ||
		active.outer.State != journal.Claimed ||
		active.outer.CurrentClaim == "" {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	work, epoch, try, err := attemptCoordinates(
		active.cycle.DispatchEffect,
	)
	if err != nil || work != active.cycle.DispatchWork {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	record, err := s.runImplementationCycle(
		ctx,
		engine,
		owner,
		state,
		slice,
		gitx.TrackKey{
			Release: active.cycle.Release,
			Track:   active.cycle.Track,
		},
		active.cycle,
		dispatchCoordinates{
			Slice:          active.cycle.Slice,
			Responsibility: driver.ImplementerImplementation,
			BatonAttempt:   slice.Attempt,
			Epoch:          epoch,
			Try:            try,
		},
		active.outer,
	)
	if err != nil {
		return err
	}
	body := mustJSON(record)
	if err := s.journal.CompleteOwned(
		context.WithoutCancel(ctx),
		owner,
		journal.Completion{
			RunID:    owner.RunID,
			EffectID: active.outer.ID,
			Token:    active.outer.CurrentClaim,
			State:    journal.Succeeded,
			Result:   body,
			Receipts: []journal.Receipt{{
				Kind: "git_candidate",
				Body: body,
			}},
			EventKind: "candidate_sealed",
			EventBody: body,
			At:        s.now().UTC(),
		},
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return s.appendImplementationReceipt(
		ctx,
		engine,
		owner,
		active.cycle,
		record,
	)
}

func (s *Service) retireStaleImplementationCycle(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	active activeImplementationCycle,
	commands map[string]journal.Command,
	effects map[string]journal.Effect,
) error {
	retire := []journal.RetireRecoveryEffect{{
		EffectID:      active.outer.ID,
		ExpectedState: active.outer.State,
		ClaimToken:    active.outer.CurrentClaim,
	}, {
		EffectID:      active.dispatch.ID,
		ExpectedState: active.dispatch.State,
		ClaimToken:    active.dispatch.CurrentClaim,
	}}
	if prepared, found := effects[active.cycle.PreparedEffect]; found {
		switch prepared.State {
		case journal.Claimed, journal.Succeeded:
			command, commandFound := commands[prepared.ReplayKey]
			if !commandFound {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			record, err := validatePreparedSealEnvelope(
				command,
				prepared,
				active.cycle,
			)
			if err != nil {
				return err
			}
			if prepared.State == journal.Succeeded &&
				(!bytesEqualCanonicalJSON(prepared.Result, record) ||
					!bytes.Equal(prepared.Result, mustJSON(record))) {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if err := s.rollbackCycleCandidate(
				engine,
				active.cycle,
				&record,
			); err != nil {
				return runtimeFail("RECOVERY_UNCERTAIN", err)
			}
		case journal.OperationalFailed:
			// The prior terminalization already rolled its candidate back.
		default:
			return runtimeFail("RECOVERY_UNCERTAIN", nil)
		}
		retire = append(retire, journal.RetireRecoveryEffect{
			EffectID:      prepared.ID,
			ExpectedState: prepared.State,
			ClaimToken:    prepared.CurrentClaim,
		})
	}
	if err := s.discardRecoverableContinuation(
		owner.RunID,
		active.dispatch.ID,
	); err != nil {
		return runtimeFail("CONTINUATION_CLEANUP_FAILED", err)
	}
	if _, err := s.journal.RetireRecoveryAttentionOwned(
		context.WithoutCancel(ctx),
		owner,
		journal.RetireRecoveryAttentionCommand{
			RunID:              owner.RunID,
			Attention:          active.attention.Attention,
			ExpectedGeneration: active.attention.Generation,
			Effects:            retire,
			ErrorCode:          "stale_authority",
		},
		s.now().UTC(),
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) recoverPendingImplementationForSlice(ctx context.Context,
	engine *engine, owner journal.OwnerLease, state baton.State,
	slice *baton.SliceState) (bool, error) {
	if slice == nil || slice.CurrentReceipt == nil {
		return false, nil
	}
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		if _, duplicate := effects[effect.ID]; duplicate {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effects[effect.ID] = effect
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	attentions, err := s.journal.Attentions(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	activeWork, err := activeAttentionWork(attentions)
	if err != nil {
		return true, err
	}
	activeCycles, err := selectActiveImplementationCycles(
		engine.manifest,
		snapshot.Commands,
		effects,
		activeWork,
	)
	if err != nil {
		return true, err
	}
	for _, command := range snapshot.Commands {
		if command.Kind != "git.seal" {
			continue
		}
		effect, ok := effects[command.ReplayKey]
		if !ok {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		cycle, err := validateImplementationCycleEnvelope(
			engine.manifest,
			command,
			effect,
		)
		if err != nil {
			return true, err
		}
		if err := validateImplementationCycleObjects(
			engine.repository,
			cycle,
		); err != nil {
			return true, err
		}
		if cycle.Slice != slice.Location.Slice.ID ||
			cycle.Binds != slice.CurrentReceipt.OID ||
			cycle.Plan != state.Plan.OID ||
			cycle.TargetHead != state.Refs.Target.Head {
			continue
		}
		if active, found := activeCycles[effect.ID]; found {
			current := implementationAuthorityCurrent(state, cycle)
			if current &&
				active.dispatch.State == journal.Claimed {
				dispatchCommand, commandFound :=
					commands[active.dispatch.ReplayKey]
				if !commandFound {
					return true, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				dispatch, dispatchErr := validateDriverRecoveryCommand(
					engine.manifest,
					dispatchCommand,
					active.dispatch,
				)
				if dispatchErr != nil {
					return true, dispatchErr
				}
				contextErr := validateCurrentProductionDispatchContext(
					ctx,
					engine,
					dispatch,
				)
				switch {
				case contextErr == nil:
				case IsCode(contextErr, "STALE_DISPATCH"):
					current = false
				default:
					return true, contextErr
				}
			}
			if !current ||
				active.dispatch.State ==
					journal.OperationalFailed {
				if err := s.retireStaleImplementationCycle(
					ctx,
					engine,
					owner,
					active,
					commands,
					effects,
				); err != nil {
					return true, err
				}
				return true, nil
			}
			if active.attention.State == journal.AttentionOpen {
				if active.dispatch.State != journal.Claimed {
					return true, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				return true, runtimeFail("EFFECT_PARKED", nil)
			}
			return true, s.executeClaimedImplementationCycle(
				ctx,
				engine,
				owner,
				state,
				slice,
				active,
			)
		}
		switch effect.State {
		case journal.Succeeded:
			var record sealedRecord
			if json.Unmarshal(effect.Result, &record) != nil ||
				!bytesEqualCanonicalJSON(effect.Result, record) ||
				!sealedRecordMatchesCycle(record, cycle) {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			return true, s.appendImplementationReceipt(ctx, engine, owner, cycle, record)
		case journal.Claimed:
			record, retry, err := s.recoverImplementationCycle(
				ctx, engine, owner, cycle, effect)
			if err != nil {
				return true, err
			}
			if retry {
				return false, nil
			}
			return true, s.appendImplementationReceipt(ctx, engine, owner, cycle, record)
		}
	}
	return false, nil
}

func sealedRecordFromCandidate(candidate gitx.SealedCandidate) sealedRecord {
	record := sealedRecord{
		Before:       candidate.Before.String(),
		Candidate:    candidate.Candidate.String(),
		Tree:         candidate.Tree.String(),
		ChangedPaths: append([]string(nil), candidate.ChangedPaths...),
	}
	if !candidate.RefreshFrom.IsZero() {
		record.RefreshFrom = candidate.RefreshFrom.String()
	}
	return record
}

func sealedRecordMatchesCycle(record sealedRecord, cycle implementationCycle) bool {
	baseMatches := record.Before == cycle.TrackHead &&
		record.RefreshFrom == cycle.RefreshFrom &&
		(cycle.RefreshFrom == "" || cycle.RefreshFrom == cycle.Binds)
	return record.Slice == cycle.Slice && record.Binds == cycle.Binds &&
		baseMatches && record.Receipt.Release == cycle.Release &&
		record.Receipt.Slice == cycle.Slice && record.Receipt.Role == "implementer" &&
		record.Receipt.Result == "candidate" &&
		record.Receipt.Base == cycle.Base &&
		record.Receipt.Candidate == record.Candidate &&
		record.ProductTree != ""
}

func linearCandidateAncestry(
	repository *gitx.Repository,
	base gitx.OID,
	candidate gitx.OID,
) (bool, error) {
	if repository == nil {
		return false, nil
	}
	cursor := candidate
	for steps := 0; steps < gitx.MaxHistory; steps++ {
		if cursor == base {
			return true, nil
		}
		parents, err := repository.Parents(cursor)
		if err != nil {
			return false, err
		}
		if len(parents) != 1 {
			return false, nil
		}
		cursor = parents[0]
	}
	return false, runtimeFail("CORRUPT_JOURNAL", nil)
}

func validateSealedRecordCandidate(
	engine *engine,
	cycle implementationCycle,
	record sealedRecord,
) error {
	if engine == nil || engine.repository == nil || engine.product == nil ||
		!sealedRecordMatchesCycle(record, cycle) {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("sealed candidate envelope does not match its cycle"),
		)
	}
	format := engine.repository.ObjectFormat()
	before, beforeErr := gitx.ParseOID(format, record.Before)
	evidenceBase := before
	var refreshErr error
	if record.RefreshFrom != "" {
		evidenceBase, refreshErr = gitx.ParseOID(format, record.RefreshFrom)
	}
	candidate, candidateErr := gitx.ParseOID(format, record.Candidate)
	tree, treeErr := gitx.ParseOID(format, record.Tree)
	if beforeErr != nil || refreshErr != nil ||
		candidateErr != nil || treeErr != nil {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(beforeErr, refreshErr, candidateErr, treeErr),
		)
	}
	if record.RefreshFrom == "" && before == candidate {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("ordinary sealed candidate did not advance its physical head"),
		)
	}
	if record.RefreshFrom != "" && evidenceBase == before {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("refreshed candidate did not retain an earlier evidence base"),
		)
	}
	if candidate != before {
		parents, parentErr := engine.repository.Parents(candidate)
		if parentErr != nil || len(parents) != 1 || parents[0] != before {
			return runtimeFail(
				"CORRUPT_JOURNAL",
				errors.Join(
					parentErr,
					errors.New("sealed candidate is not one child of its physical head"),
				),
			)
		}
	}
	linear, lineageErr := linearCandidateAncestry(
		engine.repository,
		evidenceBase,
		candidate,
	)
	observedTree, observedTreeErr := engine.repository.TreeOID(candidate)
	paths, pathsErr := engine.repository.ChangedPaths(
		evidenceBase,
		candidate,
	)
	if lineageErr != nil || observedTreeErr != nil || pathsErr != nil {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(lineageErr, observedTreeErr, pathsErr),
		)
	}
	if !linear || observedTree != tree ||
		!slices.Equal(paths, record.ChangedPaths) {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.New("sealed candidate Git evidence changed"),
		)
	}
	productIdentity, productTreeErr := engine.repository.ProductTreeIdentity(
		candidate,
		engine.product,
	)
	if productTreeErr != nil ||
		productIdentity.ProductTree != record.ProductTree {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(
				productTreeErr,
				errors.New("sealed candidate product identity changed"),
			),
		)
	}
	state, err := baton.ReadState(
		engine.git,
		engine.manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		return runtimeFail("RECOVERY_UNCERTAIN", err)
	}
	if err := validateImplementationCyclePlanAuthority(
		state,
		cycle,
	); err != nil {
		return err
	}
	var plan baton.Plan
	for _, history := range state.Plan.History {
		if history.OID == cycle.Plan {
			plan = history.Plan
			break
		}
	}
	if plan.Digest() == "" {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	if err := baton.ValidateSliceCandidateScope(
		engine.git,
		engine.inertness,
		plan,
		cycle.Slice,
		evidenceBase.String(),
		record.Candidate,
	); err != nil {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	return nil
}

func (s *Service) completeImplementationFailure(ctx context.Context, owner journal.OwnerLease,
	effectID, token, code string, result ...[]byte) error {
	if code == "" {
		code = "implementation_failed"
	}
	var resultBytes []byte
	if len(result) > 0 {
		resultBytes = result[0]
	}
	if err := s.journal.CompleteOwned(ctx, owner, journal.Completion{
		RunID: owner.RunID, EffectID: effectID, Token: token,
		State: journal.OperationalFailed, ErrorCode: code,
		Result:    resultBytes,
		EventKind: "implementation_operational_failure",
		EventBody: []byte(effectID), At: s.now().UTC(),
	}); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func currentImplementationState(
	engine *engine,
	cycle implementationCycle,
) (baton.State, error) {
	fresh, err := baton.ReadState(
		engine.git,
		engine.manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		return baton.State{}, runtimeFail("BATON_UNAVAILABLE", err)
	}
	if !implementationAuthorityCurrent(fresh, cycle) {
		return baton.State{}, runtimeFail("STALE_DISPATCH", nil)
	}
	return fresh, nil
}

func implementationRefreshBase(
	engine *engine,
	state baton.State,
	cycle implementationCycle,
) (gitx.OID, bool, error) {
	if cycle.RefreshFrom == "" {
		return gitx.OID{}, false, nil
	}
	current, ok := state.Slice(cycle.Slice)
	track, trackOK := state.Track(cycle.Track)
	if !ok || !trackOK ||
		!candidateHeadRefresh(state, current) ||
		current.CurrentReceipt == nil ||
		current.CurrentReceipt.OID != cycle.Binds ||
		current.CurrentReceipt.OID != cycle.RefreshFrom ||
		track.Head != cycle.TrackHead {
		return gitx.OID{}, false, runtimeFail("STALE_DISPATCH", nil)
	}
	refreshFrom, err := gitx.ParseOID(
		engine.repository.ObjectFormat(),
		cycle.RefreshFrom,
	)
	if err != nil {
		return gitx.OID{}, false,
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	return refreshFrom, true, nil
}

func (s *Service) claimPreparedImplementation(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	state baton.State,
	cycle implementationCycle,
	submission driver.Submission,
	prepared gitx.SealedCandidate,
	requireDispatchProof bool,
) (sealedRecord, journal.Claim, error) {
	var plan baton.Plan
	for _, history := range state.Plan.History {
		if history.OID == cycle.Plan {
			plan = history.Plan
			break
		}
	}
	if plan.Digest() == "" {
		return sealedRecord{}, journal.Claim{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	scopeBase := prepared.Before.String()
	if !prepared.RefreshFrom.IsZero() {
		current, ok := state.Slice(cycle.Slice)
		if !ok ||
			!candidateHeadRefresh(state, current) ||
			prepared.RefreshFrom.String() != cycle.RefreshFrom ||
			prepared.Before.String() != cycle.TrackHead {
			return sealedRecord{}, journal.Claim{},
				runtimeFail("STALE_DISPATCH", nil)
		}
		scopeBase = prepared.RefreshFrom.String()
	} else if cycle.RefreshFrom != "" {
		return sealedRecord{}, journal.Claim{},
			runtimeFail("STALE_DISPATCH", nil)
	}
	if err := baton.ValidateSliceCandidateScope(
		engine.git,
		engine.inertness,
		plan,
		cycle.Slice,
		scopeBase,
		prepared.Candidate.String(),
	); err != nil {
		return sealedRecord{}, journal.Claim{},
			runtimeFail("CANDIDATE_SCOPE_FAILED", err)
	}
	checks, err := exactBytes(submission.Checks)
	if err != nil {
		return sealedRecord{}, journal.Claim{}, err
	}
	record := sealedRecordFromCandidate(prepared)
	record.Slice, record.Binds = cycle.Slice, cycle.Binds
	productIdentity, err := engine.repository.ProductTreeIdentity(
		prepared.Candidate,
		engine.product,
	)
	if err != nil {
		return sealedRecord{}, journal.Claim{},
			runtimeFail("CANDIDATE_SCOPE_FAILED", err)
	}
	record.ProductTree = productIdentity.ProductTree
	record.Receipt = baton.AppendReceiptInput{
		Release: state.Release, Slice: cycle.Slice, Role: "implementer",
		Result: "candidate", Summary: submission.Summary,
		Detail: []byte(submission.Detail), Candidate: record.Candidate,
		Base: cycle.Base, CheckResults: checks,
	}
	// A contract that declares containment-requiring checks is executed here
	// at the host boundary: the engine runs each declared check against the
	// exact sealed candidate, journals the results exactly-once, and binds the
	// engine-built provenance manifest as the candidate's checks evidence. A
	// failed, timed-out, or overflowed declared host check blocks the seal, so
	// it can never flow to the verifier as a pass or be absent.
	hostChecks, contractDigest, resolveErr := resolveSliceHostChecks(
		engine, plan, cycle.Slice, state.Refs.Target.Head)
	if resolveErr != nil {
		return sealedRecord{}, journal.Claim{}, resolveErr
	}
	if len(hostChecks) > 0 {
		hostResults, runErr := s.runHostChecks(
			ctx, engine, owner, plan, cycle.Slice,
			record.Candidate, state.Refs.Target.Head)
		if runErr != nil {
			return sealedRecord{}, journal.Claim{}, runErr
		}
		manifest, buildErr := buildHostCheckResultsManifest(
			state.Release, cycle.Slice, sliceAttempt(state, cycle.Slice),
			record.Candidate, contractDigest, hostResults,
			baton.DigestBytes(checks))
		if buildErr != nil {
			return sealedRecord{}, journal.Claim{}, buildErr
		}
		record.Receipt.CheckResults = manifest
	}
	if requireDispatchProof {
		if err := s.validateImplementationDispatchProof(
			ctx,
			engine,
			owner,
			cycle,
			record,
		); err != nil {
			return sealedRecord{}, journal.Claim{}, err
		}
	}
	now := s.now().UTC()
	payload := mustJSON(record)
	_, preparedEpoch, preparedTry, coordinateErr :=
		attemptCoordinates(cycle.PreparedEffect)
	if coordinateErr != nil {
		return sealedRecord{}, journal.Claim{}, coordinateErr
	}
	if err := s.journal.EnsureAttempt(ctx,
		journal.Command{RunID: owner.RunID, ReplayKey: cycle.PreparedEffect,
			Kind: "git.seal.prepared", Payload: payload, CreatedAt: now},
		journal.Effect{RunID: owner.RunID, ID: cycle.PreparedEffect,
			ReplayKey: cycle.PreparedEffect, Kind: "git.seal.prepared",
			BeforeDigest: cycle.Before, ExpectedDigest: sha256Digest(payload),
			UpdatedAt: now},
		journal.EffectAttempt{
			WorkID: cycle.PreparedWork,
			Epoch:  preparedEpoch,
			Try:    preparedTry,
		}); err != nil {
		if !requireDispatchProof {
			err = uncertainHandoffPreparation(err)
		}
		return sealedRecord{}, journal.Claim{}, err
	}
	claim, err := s.journal.ClaimOwned(
		ctx,
		owner,
		cycle.PreparedEffect,
		now,
		effectLease,
	)
	if err == nil && testCrashAfterEffect == "git.seal.prepared" {
		os.Exit(86)
	}
	if err != nil {
		if !requireDispatchProof {
			err = uncertainHandoffPreparation(err)
		}
		return sealedRecord{}, journal.Claim{}, err
	}
	return record, claim, nil
}

var errProductionCandidatePrepared = errors.New(
	"production implementation candidate prepared",
)

func (s *Service) prepareProductionImplementationCandidate(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	workspace *gitx.WorkspaceLease,
	cycle implementationCycle,
	submission driver.Submission,
) (sealedRecord, journal.Claim, error) {
	fresh, err := currentImplementationState(engine, cycle)
	if err != nil {
		return sealedRecord{}, journal.Claim{}, err
	}
	releaseHead, releaseErr := gitx.ParseOID(
		engine.repository.ObjectFormat(),
		fresh.Refs.Release.Head,
	)
	targetHead, targetErr := gitx.ParseOID(
		engine.repository.ObjectFormat(),
		cycle.TargetHead,
	)
	if releaseErr != nil || targetErr != nil {
		return sealedRecord{}, journal.Claim{}, runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(releaseErr, targetErr),
		)
	}
	refreshFrom, refreshed, refreshErr :=
		implementationRefreshBase(engine, fresh, cycle)
	if refreshErr != nil {
		return sealedRecord{}, journal.Claim{}, refreshErr
	}
	var record sealedRecord
	var preparedClaim journal.Claim
	if refreshed {
		_, prepareErr := engine.workspaces.SealTrackRefreshGuardedWithClaim(
			workspace,
			refreshFrom,
			gitx.SealAuthority{
				ReleaseHead: releaseHead,
				TargetRef:   engine.manifest.value.TargetRef,
				TargetHead:  targetHead,
				Identity:    cycle.GitIdentity,
			},
			func(prepared gitx.SealedCandidate) error {
				var claimErr error
				record, preparedClaim, claimErr =
					s.claimPreparedImplementation(
						ctx,
						engine,
						owner,
						fresh,
						cycle,
						submission,
						prepared,
						false,
					)
				if claimErr != nil {
					return claimErr
				}
				return errProductionCandidatePrepared
			},
		)
		if !errors.Is(prepareErr, errProductionCandidatePrepared) {
			return sealedRecord{}, journal.Claim{}, prepareErr
		}
		if _, err := currentImplementationState(engine, cycle); err != nil {
			return sealedRecord{}, journal.Claim{}, err
		}
		return record, preparedClaim, nil
	}
	_, prepareErr := engine.workspaces.SealTrackGuardedWithClaim(
		workspace,
		gitx.SealAuthority{
			ReleaseHead: releaseHead,
			TargetRef:   engine.manifest.value.TargetRef,
			TargetHead:  targetHead,
			Identity:    cycle.GitIdentity,
		},
		func(prepared gitx.SealedCandidate) error {
			var claimErr error
			record, preparedClaim, claimErr =
				s.claimPreparedImplementation(
					ctx,
					engine,
					owner,
					fresh,
					cycle,
					submission,
					prepared,
					false,
				)
			if claimErr != nil {
				return claimErr
			}
			return errProductionCandidatePrepared
		},
	)
	if !errors.Is(prepareErr, errProductionCandidatePrepared) {
		return sealedRecord{}, journal.Claim{}, prepareErr
	}
	if _, err := currentImplementationState(engine, cycle); err != nil {
		return sealedRecord{}, journal.Claim{}, err
	}
	return record, preparedClaim, nil
}

func (s *Service) claimedPreparedImplementation(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle implementationCycle,
) (sealedRecord, journal.Claim, bool, error) {
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return sealedRecord{}, journal.Claim{}, false,
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	var prepared journal.Effect
	var command journal.Command
	effectFound := false
	commandFound := false
	for _, effect := range snapshot.Effects {
		if effect.ID != cycle.PreparedEffect {
			continue
		}
		if effectFound {
			return sealedRecord{}, journal.Claim{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		prepared, effectFound = effect, true
	}
	for _, candidate := range snapshot.Commands {
		if candidate.ReplayKey != cycle.PreparedEffect {
			continue
		}
		if commandFound {
			return sealedRecord{}, journal.Claim{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		command, commandFound = candidate, true
	}
	if !effectFound && !commandFound {
		return sealedRecord{}, journal.Claim{}, false, nil
	}
	if !effectFound || !commandFound ||
		prepared.State != journal.Claimed ||
		prepared.CurrentClaim == "" {
		return sealedRecord{}, journal.Claim{}, false,
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	record, err := validatePreparedSealEnvelope(command, prepared, cycle)
	if err != nil {
		return sealedRecord{}, journal.Claim{}, false, err
	}
	return record, journal.Claim{
		RunID: owner.RunID, EffectID: prepared.ID,
		Token: prepared.CurrentClaim,
	}, true, nil
}

func (s *Service) runProductionImplementationDispatch(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	workspace *gitx.WorkspaceLease,
	cycle implementationCycle,
	coordinates dispatchCoordinates,
) (sealedRecord, journal.Claim, error) {
	dispatchWork, dispatchEpoch, dispatchTry, err :=
		attemptCoordinates(cycle.DispatchEffect)
	if err != nil || dispatchWork != cycle.DispatchWork {
		return sealedRecord{}, journal.Claim{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	var record sealedRecord
	var preparedClaim journal.Claim
	_, err = s.runDriverEffectWithPreparation(
		ctx,
		engine,
		workspace,
		driver.RoleImplementer,
		coordinates,
		journal.EffectAttempt{
			WorkID: cycle.DispatchWork,
			Epoch:  dispatchEpoch,
			Try:    dispatchTry,
		},
		cycle.Before,
		owner,
		func(submission driver.Submission) error {
			var found bool
			var loadErr error
			record, preparedClaim, found, loadErr =
				s.claimedPreparedImplementation(
					ctx,
					owner,
					cycle,
				)
			if loadErr != nil || found {
				return loadErr
			}
			var prepareErr error
			record, preparedClaim, prepareErr =
				s.prepareProductionImplementationCandidate(
					ctx,
					engine,
					owner,
					workspace,
					cycle,
					submission,
				)
			return prepareErr
		},
	)
	if err != nil {
		return sealedRecord{}, journal.Claim{}, err
	}
	if preparedClaim.Token == "" {
		var found bool
		record, preparedClaim, found, err =
			s.claimedPreparedImplementation(ctx, owner, cycle)
		if err != nil {
			return sealedRecord{}, journal.Claim{}, err
		}
		if !found {
			return sealedRecord{}, journal.Claim{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	if preparedClaim.Token == "" ||
		!sealedRecordMatchesCycle(record, cycle) {
		return sealedRecord{}, journal.Claim{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return record, preparedClaim, nil
}

func (s *Service) runImplementationCycle(ctx context.Context, engine *engine,
	owner journal.OwnerLease, state baton.State, slice *baton.SliceState,
	key gitx.TrackKey, cycle implementationCycle, coordinates dispatchCoordinates,
	outer journal.Effect) (sealedRecord, error) {
	fresh, err := baton.ReadState(engine.git, state.Release, engine.inertness)
	if err != nil {
		return sealedRecord{}, runtimeFail("BATON_UNAVAILABLE", err)
	}
	if fresh.Plan.TargetStale || sliceFingerprint(fresh, cycle.Slice) != cycle.Before {
		return sealedRecord{}, runtimeFail("STALE_DISPATCH", nil)
	}
	workspace, err := engine.workspaces.OpenTrack(key, gitx.ImplementationView)
	if err != nil {
		return sealedRecord{}, runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	if engine.manifest.value.production() {
		record, preparedClaim, dispatchErr :=
			s.runProductionImplementationDispatch(
				ctx,
				engine,
				owner,
				workspace,
				cycle,
				coordinates,
			)
		if dispatchErr != nil {
			_ = workspace.Close()
			return sealedRecord{}, dispatchErr
		}
		if testCrashAfterEffect == "implementation.handoff" {
			os.Exit(86)
		}
		if err := workspace.Close(); err != nil {
			return sealedRecord{}, runtimeFail("WORKSPACE_UNAVAILABLE", err)
		}
		return s.reconcilePreparedSeal(
			ctx,
			engine,
			owner,
			cycle,
			record,
			journal.Effect{
				RunID: owner.RunID, ID: cycle.PreparedEffect,
				State:        journal.Claimed,
				CurrentClaim: preparedClaim.Token,
			},
			outer,
		)
	}
	dispatchWork, dispatchEpoch, dispatchTry, dispatchErr :=
		attemptCoordinates(cycle.DispatchEffect)
	if dispatchErr != nil || dispatchWork != cycle.DispatchWork {
		_ = workspace.Close()
		return sealedRecord{},
			runtimeFail("CORRUPT_JOURNAL", dispatchErr)
	}
	submission, err := s.runDriverEffect(
		ctx,
		engine,
		workspace,
		driver.RoleImplementer,
		coordinates,
		journal.EffectAttempt{
			WorkID: cycle.DispatchWork,
			Epoch:  dispatchEpoch,
			Try:    dispatchTry,
		},
		cycle.Before,
		owner,
	)
	if err != nil {
		_ = workspace.Close()
		return sealedRecord{}, err
	}
	if testCrashAfterEffect == "implementation.handoff" {
		os.Exit(86)
	}
	fresh, err = baton.ReadState(engine.git, state.Release, engine.inertness)
	if err != nil || sliceFingerprint(fresh, cycle.Slice) != cycle.Before {
		_ = workspace.Close()
		return sealedRecord{}, runtimeFail("STALE_DISPATCH", err)
	}
	current, ok := fresh.Slice(cycle.Slice)
	if !ok || current.Status != "ready" || current.Stage != "implement" ||
		current.NextRole != "implementer" || current.CurrentReceipt == nil ||
		current.CurrentReceipt.OID != cycle.Binds {
		_ = workspace.Close()
		return sealedRecord{}, runtimeFail("STALE_DISPATCH", nil)
	}
	var preparedClaim journal.Claim
	var record sealedRecord
	releaseHead, releaseErr := gitx.ParseOID(
		engine.repository.ObjectFormat(), fresh.Refs.Release.Head)
	targetHead, targetErr := gitx.ParseOID(
		engine.repository.ObjectFormat(), cycle.TargetHead)
	if releaseErr != nil || targetErr != nil {
		_ = workspace.Close()
		return sealedRecord{}, runtimeFail(
			"CORRUPT_JOURNAL", errors.Join(releaseErr, targetErr))
	}
	refreshFrom, refreshed, refreshErr :=
		implementationRefreshBase(engine, fresh, cycle)
	if refreshErr != nil {
		_ = workspace.Close()
		return sealedRecord{}, refreshErr
	}
	authority := gitx.SealAuthority{
		ReleaseHead: releaseHead,
		TargetRef:   engine.manifest.value.TargetRef,
		TargetHead:  targetHead,
		Identity:    cycle.GitIdentity,
	}
	claimPrepared := func(prepared gitx.SealedCandidate) error {
		var claimErr error
		record, preparedClaim, claimErr =
			s.claimPreparedImplementation(
				ctx,
				engine,
				owner,
				fresh,
				cycle,
				submission,
				prepared,
				true,
			)
		return claimErr
	}
	if refreshed {
		_, err = engine.workspaces.SealTrackRefreshGuardedWithClaim(
			workspace,
			refreshFrom,
			authority,
			claimPrepared,
		)
	} else {
		_, err = engine.workspaces.SealTrackGuardedWithClaim(
			workspace,
			authority,
			claimPrepared,
		)
	}
	_ = workspace.Close()
	if err != nil {
		if preparedClaim.Token != "" {
			recovered, recoverErr := s.reconcilePreparedSeal(
				ctx, engine, owner, cycle, record,
				journal.Effect{
					RunID: owner.RunID, ID: cycle.PreparedEffect,
					State:        journal.Claimed,
					CurrentClaim: preparedClaim.Token,
				},
				outer,
			)
			if recoverErr == nil {
				return recovered, nil
			}
			return sealedRecord{}, recoverErr
		}
		return sealedRecord{}, err
	}
	if err := s.validateSealedCycle(engine, cycle, record, true); err != nil {
		if rollbackErr := s.rollbackSealedCycle(engine, cycle, record); rollbackErr != nil {
			_ = s.journal.ReconcileOwned(context.WithoutCancel(ctx), owner,
				journal.Completion{RunID: owner.RunID, EffectID: cycle.PreparedEffect,
					Token: preparedClaim.Token, EventKind: "seal_uncertain",
					EventBody: []byte(cycle.Slice), At: s.now().UTC()},
				journal.RecoveryAmbiguous)
			return sealedRecord{}, runtimeFail("RECOVERY_UNCERTAIN", rollbackErr)
		}
		_ = s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
			RunID: owner.RunID, EffectID: cycle.PreparedEffect, Token: preparedClaim.Token,
			State: journal.OperationalFailed, ErrorCode: "stale_dispatch",
			EventKind: "seal_rolled_back", EventBody: []byte(cycle.Slice), At: s.now().UTC(),
		})
		return sealedRecord{}, runtimeFail("STALE_DISPATCH", err)
	}
	if testCrashAfterEffect == "git.seal" {
		os.Exit(86)
	}
	body := mustJSON(record)
	if err := s.journal.CompleteOwned(context.WithoutCancel(ctx), owner, journal.Completion{
		RunID: owner.RunID, EffectID: cycle.PreparedEffect, Token: preparedClaim.Token,
		State: journal.Succeeded, Result: body,
		EventKind: "candidate_prepared", EventBody: body, At: s.now().UTC(),
	}); err != nil {
		return sealedRecord{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return record, nil
}

func (s *Service) recoverImplementationCycle(ctx context.Context, engine *engine,
	owner journal.OwnerLease, cycle implementationCycle,
	effect journal.Effect) (sealedRecord, bool, error) {
	prepared, preparedErr := s.journal.Effect(ctx, owner.RunID, cycle.PreparedEffect)
	if preparedErr == nil {
		snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
		if err != nil {
			return sealedRecord{}, false, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		var record sealedRecord
		found := 0
		for _, command := range snapshot.Commands {
			if command.ReplayKey == cycle.PreparedEffect {
				found++
				recovered, err := validatePreparedSealEnvelope(
					command,
					prepared,
					cycle,
				)
				if err != nil {
					return sealedRecord{}, false, err
				}
				record = recovered
			}
		}
		if found != 1 {
			return sealedRecord{}, false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if engine.manifest.value.production() {
			dispatch, dispatchErr := s.journal.Effect(
				ctx,
				owner.RunID,
				cycle.DispatchEffect,
			)
			if dispatchErr != nil {
				return sealedRecord{}, false,
					runtimeFail("JOURNAL_READ_FAILED", dispatchErr)
			}
			if dispatch.State == journal.OperationalFailed {
				if err := s.rollbackCycleCandidate(
					engine,
					cycle,
					nil,
				); err != nil {
					return sealedRecord{}, false,
						runtimeFail("RECOVERY_UNCERTAIN", err)
				}
				switch prepared.State {
				case journal.Pending, journal.Claimed:
					if err := s.terminalizeEffect(
						ctx,
						owner,
						prepared,
						"orphaned_dispatch",
					); err != nil {
						return sealedRecord{}, false, err
					}
				case journal.Uncertain:
					if err := s.journal.ResolveUncertainOwned(
						context.WithoutCancel(ctx),
						owner,
						owner.RunID,
						prepared.ID,
						"orphaned_dispatch",
						s.now().UTC(),
					); err != nil {
						return sealedRecord{}, false,
							runtimeFail("JOURNAL_WRITE_FAILED", err)
					}
				default:
					return sealedRecord{}, false,
						runtimeFail("CORRUPT_JOURNAL", nil)
				}
				return s.interruptImplementationCycle(
					ctx,
					engine,
					owner,
					cycle,
					effect,
				)
			}
			if dispatch.State != journal.Succeeded {
				if dispatch.State != journal.Claimed &&
					dispatch.State != journal.Uncertain {
					return sealedRecord{}, false,
						runtimeFail("CORRUPT_JOURNAL", nil)
				}
				if prepared.State == journal.Pending {
					claim, claimErr := s.journal.ClaimOwned(
						ctx,
						owner,
						prepared.ID,
						s.now().UTC(),
						effectLease,
					)
					if claimErr != nil {
						return sealedRecord{}, false,
							runtimeFail("EFFECT_CLAIM_FAILED", claimErr)
					}
					prepared.State = journal.Claimed
					prepared.CurrentClaim = claim.Token
				}
				completions := []journal.Completion{{
					RunID: owner.RunID, EffectID: effect.ID,
					Token:     effect.CurrentClaim,
					EventKind: "implementation_uncertain",
					EventBody: []byte(cycle.Slice), At: s.now().UTC(),
				}}
				if prepared.State == journal.Claimed {
					completions = append(completions, journal.Completion{
						RunID: owner.RunID, EffectID: prepared.ID,
						Token:     prepared.CurrentClaim,
						EventKind: "implementation_preparation_uncertain",
						EventBody: []byte(cycle.Slice), At: s.now().UTC(),
					})
				}
				if dispatch.State == journal.Claimed {
					completions = append(completions, journal.Completion{
						RunID: owner.RunID, EffectID: dispatch.ID,
						Token:     dispatch.CurrentClaim,
						EventKind: "implementation_dispatch_uncertain",
						EventBody: []byte(cycle.Slice), At: s.now().UTC(),
					})
				}
				if err := s.journal.ReconcileManyOwned(
					context.WithoutCancel(ctx),
					owner,
					completions,
					journal.RecoveryAmbiguous,
				); err != nil {
					return sealedRecord{}, false,
						runtimeFail("JOURNAL_WRITE_FAILED", err)
				}
				return sealedRecord{}, false,
					runtimeFail("RECOVERY_UNCERTAIN", nil)
			}
		}
		if err := validateImplementationDispatchProof(
			engine,
			snapshot,
			cycle,
			record,
		); err != nil {
			return sealedRecord{}, false, err
		}
		switch prepared.State {
		case journal.Pending:
			claim, err := s.journal.ClaimOwned(
				ctx, owner, prepared.ID, s.now().UTC(), effectLease)
			if err != nil {
				return sealedRecord{}, false, runtimeFail("EFFECT_CLAIM_FAILED", err)
			}
			prepared.State = journal.Claimed
			prepared.CurrentClaim = claim.Token
			record, err = s.reconcilePreparedSeal(
				ctx,
				engine,
				owner,
				cycle,
				record,
				prepared,
				effect,
			)
			if err != nil {
				if IsCode(err, "STALE_DISPATCH") {
					if terminalErr := s.terminalizeEffect(
						ctx,
						owner,
						effect,
						"stale_authority",
					); terminalErr != nil {
						return sealedRecord{}, false, terminalErr
					}
				}
				return sealedRecord{}, false, err
			}
		case journal.OperationalFailed:
			return s.interruptImplementationCycle(ctx, engine, owner, cycle, effect)
		case journal.Uncertain:
			_ = s.journal.ReconcileOwned(ctx, owner, journal.Completion{
				RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim,
				EventKind: "implementation_uncertain", EventBody: []byte(cycle.Slice),
				At: s.now().UTC(),
			}, journal.RecoveryAmbiguous)
			return sealedRecord{}, false, runtimeFail("RECOVERY_UNCERTAIN", nil)
		case journal.Claimed:
			record, err = s.reconcilePreparedSeal(
				ctx, engine, owner, cycle, record, prepared, effect)
			if err != nil {
				if IsCode(err, "STALE_DISPATCH") {
					if terminalErr := s.terminalizeEffect(
						ctx, owner, effect, "stale_authority"); terminalErr != nil {
						return sealedRecord{}, false, terminalErr
					}
				}
				return sealedRecord{}, false, err
			}
		case journal.Succeeded:
			if !bytes.Equal(prepared.Result, mustJSON(record)) {
				return sealedRecord{}, false, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if err := s.validateSealedCycle(engine, cycle, record, true); err != nil {
				return sealedRecord{}, false, runtimeFail("RECOVERY_UNCERTAIN", err)
			}
		default:
			return sealedRecord{}, false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		body := mustJSON(record)
		if err := s.journal.ReconcileOwned(ctx, owner, journal.Completion{
			RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim,
			State: journal.Succeeded, Result: body,
			Receipts:  []journal.Receipt{{Kind: "git_candidate", Body: body}},
			EventKind: "candidate_reconciled", EventBody: body, At: s.now().UTC(),
		}, journal.RecoveryAllNew); err != nil {
			return sealedRecord{}, false, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return record, false, nil
	}
	if !journal.IsCode(preparedErr, "EFFECT_NOT_FOUND") {
		return sealedRecord{}, false, runtimeFail("JOURNAL_READ_FAILED", preparedErr)
	}
	dispatch, dispatchErr := s.journal.Effect(ctx, owner.RunID, cycle.DispatchEffect)
	if dispatchErr == nil && (dispatch.State == journal.Claimed ||
		dispatch.State == journal.Uncertain) {
		completions := []journal.Completion{{
			RunID: owner.RunID, EffectID: effect.ID,
			Token:     effect.CurrentClaim,
			EventKind: "implementation_uncertain",
			EventBody: []byte(cycle.Slice), At: s.now().UTC(),
		}}
		if dispatch.State == journal.Claimed {
			completions = append(completions, journal.Completion{
				RunID: owner.RunID, EffectID: dispatch.ID,
				Token:     dispatch.CurrentClaim,
				EventKind: "implementation_dispatch_uncertain",
				EventBody: []byte(cycle.Slice), At: s.now().UTC(),
			})
		}
		if err := s.journal.ReconcileManyOwned(
			context.WithoutCancel(ctx),
			owner,
			completions,
			journal.RecoveryAmbiguous,
		); err != nil {
			return sealedRecord{}, false,
				runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return sealedRecord{}, false, runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	if dispatchErr != nil && !journal.IsCode(dispatchErr, "EFFECT_NOT_FOUND") {
		return sealedRecord{}, false, runtimeFail("JOURNAL_READ_FAILED", dispatchErr)
	}
	if dispatchErr == nil &&
		dispatch.State == journal.Succeeded &&
		engine.manifest.value.production() {
		if err := s.journal.ReconcileOwned(
			context.WithoutCancel(ctx),
			owner,
			journal.Completion{
				RunID: owner.RunID, EffectID: effect.ID,
				Token:     effect.CurrentClaim,
				EventKind: "implementation_preparation_missing",
				EventBody: []byte(cycle.Slice), At: s.now().UTC(),
			},
			journal.RecoveryAmbiguous,
		); err != nil {
			return sealedRecord{}, false,
				runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return sealedRecord{}, false, runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	return s.interruptImplementationCycle(ctx, engine, owner, cycle, effect)
}

func (s *Service) interruptImplementationCycle(ctx context.Context, engine *engine,
	owner journal.OwnerLease, cycle implementationCycle,
	effect journal.Effect) (sealedRecord, bool, error) {
	if err := s.journal.ReconcileOwned(ctx, owner, journal.Completion{
		RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim,
		EventKind: "implementation_reconciled_all_old", EventBody: []byte(cycle.Slice),
		At: s.now().UTC(),
	}, journal.RecoveryAllOld); err != nil {
		return sealedRecord{}, false, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	claim, err := s.journal.ClaimOwned(ctx, owner, effect.ID, s.now().UTC(), effectLease)
	if err != nil {
		return sealedRecord{}, false, runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	if err := s.completeImplementationFailure(ctx, owner, effect.ID, claim.Token,
		"implementation_interrupted"); err != nil {
		return sealedRecord{}, false, err
	}
	fresh, err := baton.ReadState(engine.git, engine.manifest.value.Release, engine.inertness)
	if err != nil || sliceFingerprint(fresh, cycle.Slice) != cycle.Before {
		return sealedRecord{}, false, runtimeFail("STALE_DISPATCH", err)
	}
	return sealedRecord{}, true, nil
}

func (s *Service) validateSealedCycle(engine *engine, cycle implementationCycle,
	record sealedRecord, sealed bool) error {
	if !sealedRecordMatchesCycle(record, cycle) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	state, err := baton.ReadState(
		engine.git, engine.manifest.value.Release, engine.inertness)
	if err != nil {
		return runtimeFail("BATON_UNAVAILABLE", err)
	}
	if err := validateImplementationCyclePlanAuthority(
		state,
		cycle,
	); err != nil {
		return err
	}
	current, ok := state.Slice(cycle.Slice)
	track, trackOK := state.Track(cycle.Track)
	expectedTrack := cycle.TrackHead
	if sealed {
		expectedTrack = record.Candidate
	}
	if !ok || !trackOK || state.Plan.OID != cycle.Plan ||
		state.Refs.Target.Head != cycle.TargetHead ||
		track.Ref != cycle.TrackRef || track.Head != expectedTrack ||
		current.Status != "ready" || current.Stage != "implement" ||
		current.NextRole != "implementer" || current.CurrentReceipt == nil ||
		current.CurrentReceipt.OID != cycle.Binds {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	return nil
}

func implementationAuthorityCurrent(state baton.State, cycle implementationCycle) bool {
	current, ok := state.Slice(cycle.Slice)
	track, trackOK := state.Track(cycle.Track)
	return validateImplementationCyclePlanAuthority(state, cycle) == nil &&
		ok && trackOK &&
		current.Location.Track.ID == cycle.Track &&
		sliceFingerprintAtTrackHead(
			state,
			cycle.Slice,
			cycle.TrackHead,
		) == cycle.Before &&
		!state.Plan.TargetStale &&
		state.Plan.OID == cycle.Plan &&
		state.Refs.Target.Head == cycle.TargetHead &&
		track.Ref == cycle.TrackRef &&
		current.Status == "ready" && current.Stage == "implement" &&
		current.NextRole == "implementer" && current.CurrentReceipt != nil &&
		current.CurrentReceipt.OID == cycle.Binds
}

func implementationReceiptApplied(
	state baton.State,
	cycle implementationCycle,
	record sealedRecord,
) (bool, error) {
	var entries []baton.ReceiptEntry
	if historical, ok := state.HistoryForSlice(cycle.Slice); ok {
		entries = append(entries, historical.History.Entries...)
	}
	if current, ok := state.Slice(cycle.Slice); ok {
		for index := range current.History.Entries {
			entries = appendUniqueReceipt(
				entries,
				&current.History.Entries[index],
			)
		}
		entries = appendUniqueReceipt(entries, current.CurrentReceipt)
		entries = appendUniqueReceipt(entries, current.Candidate)
		entries = appendUniqueReceipt(entries, current.Pass)
	}
	metadata, ok := planMetadataForOID(state, cycle.Plan)
	if !ok {
		return false, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	contract, ok := metadata.Contracts[cycle.Slice]
	if !ok {
		return false, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var expectedAttempt *int64
	for index := range entries {
		if entries[index].OID != cycle.Binds {
			continue
		}
		bound := entries[index].Receipt
		if bound.Attempt == nil {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		attempt := *bound.Attempt
		switch {
		case bound.Role == "captain" && bound.Result == "proceed":
		case bound.Role == "implementer" &&
			bound.Result == "candidate":
			attempt++
		case bound.Role == "verifier" &&
			(bound.Result == "pass" || bound.Result == "fail"):
			attempt++
		default:
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if expectedAttempt != nil &&
			*expectedAttempt != attempt {
			return false, runtimeFail("AMBIGUOUS_ACTION_HISTORY", nil)
		}
		expectedAttempt = &attempt
	}
	if expectedAttempt == nil {
		return false, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	expectedChecks := baton.DigestBytes(record.Receipt.CheckResults)
	expectedDetail := baton.DigestBytes(record.Receipt.Detail)
	found := ""
	for _, entry := range entries {
		receipt := entry.Receipt
		if receipt.Version == baton.ReceiptVersion &&
			receipt.Release == cycle.Release &&
			receipt.Slice != nil &&
			*receipt.Slice == cycle.Slice &&
			receipt.Role == "implementer" && receipt.Result == "candidate" &&
			receipt.Attempt != nil &&
			*receipt.Attempt == *expectedAttempt &&
			receipt.Plan == cycle.Plan && receipt.Binds == cycle.Binds &&
			receipt.Contract != nil &&
			*receipt.Contract == contract &&
			receipt.Detail == expectedDetail &&
			receipt.Candidate != nil &&
			*receipt.Candidate == record.Candidate &&
			receipt.ProductTree != nil &&
			*receipt.ProductTree == record.ProductTree &&
			receipt.Inputs != nil &&
			receipt.Target == nil &&
			((receipt.Base == nil && record.Receipt.Base == "") ||
				(receipt.Base != nil &&
					*receipt.Base == record.Receipt.Base)) &&
			receipt.ResultCommit == nil &&
			receipt.Summary == record.Receipt.Summary &&
			bytes.Equal(entry.Detail, record.Receipt.Detail) &&
			receipt.Checks != nil &&
			*receipt.Checks == expectedChecks {
			if found != "" && found != entry.OID {
				return false,
					runtimeFail("AMBIGUOUS_ACTION_HISTORY", nil)
			}
			found = entry.OID
		}
	}
	return found != "", nil
}

func (s *Service) terminalizeEffect(ctx context.Context, owner journal.OwnerLease,
	effect journal.Effect, code string) error {
	switch effect.State {
	case journal.Pending:
		claim, err := s.journal.ClaimOwned(
			context.WithoutCancel(ctx), owner, effect.ID, s.now().UTC(), effectLease)
		if err != nil {
			return runtimeFail("EFFECT_CLAIM_FAILED", err)
		}
		effect.State, effect.CurrentClaim = journal.Claimed, claim.Token
		return s.finishClaimedFailure(ctx, owner, effect, code)
	case journal.Claimed:
		return s.finishClaimedFailure(ctx, owner, effect, code)
	case journal.OperationalFailed, journal.Succeeded:
		return nil
	default:
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
}

func (s *Service) rollbackCycleCandidate(engine *engine, cycle implementationCycle,
	record *sealedRecord) error {
	if record != nil {
		if err := validateSealedRecordCandidate(
			engine,
			cycle,
			*record,
		); err != nil {
			return err
		}
	}
	before, err := gitx.ParseOID(engine.repository.ObjectFormat(), cycle.TrackHead)
	if err != nil {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	var candidate gitx.OID
	if record != nil {
		if record.Before != cycle.TrackHead {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		candidate, err = gitx.ParseOID(
			engine.repository.ObjectFormat(), record.Candidate)
		if err != nil {
			return runtimeFail("CORRUPT_JOURNAL", err)
		}
	}
	releaseRef := "refs/heads/release-wt/" + engine.manifest.value.Release
	refs, err := engine.repository.CaptureHeadRefs(
		[]string{cycle.TrackRef, releaseRef, engine.manifest.value.TargetRef})
	if err != nil || len(refs) != 3 {
		return runtimeFail("RECOVERY_FAILED", err)
	}
	var track, release, target gitx.RefHead
	for _, ref := range refs {
		switch ref.Ref {
		case cycle.TrackRef:
			track = ref
		case releaseRef:
			release = ref
		case engine.manifest.value.TargetRef:
			target = ref
		}
	}
	if track.State != gitx.RefDirect ||
		release.State != gitx.RefDirect || target.State != gitx.RefDirect {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	if track.Head == before {
		return nil
	}
	if record == nil || track.Head != candidate {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	if err := engine.repository.ApplyRefTransaction(refs, []gitx.RefOperation{
		{
			Kind: gitx.UpdateRef, Ref: cycle.TrackRef,
			NewHead: &before, Expected: &candidate,
		},
		{Kind: gitx.VerifyRef, Ref: releaseRef, Expected: &release.Head},
		{
			Kind: gitx.VerifyRef, Ref: engine.manifest.value.TargetRef,
			Expected: &target.Head,
		},
	}); err != nil {
		return runtimeFail("RECOVERY_FAILED", err)
	}
	return nil
}

func (s *Service) rollbackSealedCycle(engine *engine, cycle implementationCycle,
	record sealedRecord) error {
	return s.rollbackCycleCandidate(engine, cycle, &record)
}

func (s *Service) rejectStalePreparedSeal(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	cycle implementationCycle,
	record sealedRecord,
	claimToken string,
) (sealedRecord, error) {
	if err := s.rollbackCycleCandidate(engine, cycle, &record); err != nil {
		return sealedRecord{}, err
	}
	if err := s.terminalizeEffect(ctx, owner, journal.Effect{
		RunID: owner.RunID, ID: cycle.PreparedEffect,
		Kind: "git.seal.prepared", State: journal.Claimed,
		CurrentClaim: claimToken,
	}, "stale_authority"); err != nil {
		return sealedRecord{}, err
	}
	return sealedRecord{}, runtimeFail("STALE_DISPATCH", nil)
}

func (s *Service) reconcilePreparedSeal(ctx context.Context, engine *engine,
	owner journal.OwnerLease, cycle implementationCycle, record sealedRecord,
	prepared journal.Effect, outer journal.Effect) (sealedRecord, error) {
	if prepared.State != journal.Claimed ||
		prepared.CurrentClaim == "" {
		return sealedRecord{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := s.validateImplementationDispatchProof(
		ctx,
		engine,
		owner,
		cycle,
		record,
	); err != nil {
		return sealedRecord{}, err
	}
	if err := validateSealedRecordCandidate(
		engine,
		cycle,
		record,
	); err != nil {
		return sealedRecord{}, err
	}
	claimToken := prepared.CurrentClaim
	before, beforeErr := gitx.ParseOID(engine.repository.ObjectFormat(), record.Before)
	candidate, candidateErr := gitx.ParseOID(
		engine.repository.ObjectFormat(), record.Candidate)
	if beforeErr != nil || candidateErr != nil || !sealedRecordMatchesCycle(record, cycle) {
		return sealedRecord{}, runtimeFail(
			"CORRUPT_JOURNAL", errors.Join(beforeErr, candidateErr))
	}
	state, stateErr := baton.ReadState(
		engine.git, engine.manifest.value.Release, engine.inertness)
	if stateErr != nil {
		return sealedRecord{}, runtimeFail("BATON_UNAVAILABLE", stateErr)
	}
	if !implementationAuthorityCurrent(state, cycle) {
		return s.rejectStalePreparedSeal(
			ctx, engine, owner, cycle, record, claimToken)
	}
	if record.RefreshFrom != "" && before == candidate {
		if err := s.validateSealedCycle(
			engine,
			cycle,
			record,
			true,
		); err != nil {
			return s.rejectStalePreparedSeal(
				ctx,
				engine,
				owner,
				cycle,
				record,
				claimToken,
			)
		}
		body := mustJSON(record)
		if err := s.journal.ReconcileOwned(
			context.WithoutCancel(ctx),
			owner,
			journal.Completion{
				RunID: owner.RunID, EffectID: cycle.PreparedEffect,
				Token: claimToken, State: journal.Succeeded, Result: body,
				EventKind: "candidate_reconciled", EventBody: body,
				At: s.now().UTC(),
			},
			journal.RecoveryAllNew,
		); err != nil {
			return sealedRecord{},
				runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return record, nil
	}
	key := gitx.TrackKey{Release: engine.manifest.value.Release, Track: cycle.Track}
	disposition, err := engine.workspaces.ReconcileSeal(key, before, candidate)
	if err != nil {
		return sealedRecord{}, runtimeFail("RECOVERY_FAILED", err)
	}
	releaseAuthority := state.Refs.Release.Head
	if disposition == gitx.SealAllOld {
		if err := s.validateSealedCycle(engine, cycle, record, false); err != nil {
			return sealedRecord{}, err
		}
		prepared, err = s.reclaimAllOldPreparedSeal(
			ctx,
			owner,
			cycle,
			prepared,
		)
		if err != nil {
			return sealedRecord{}, err
		}
		claimToken = prepared.CurrentClaim
		releaseRef := "refs/heads/release-wt/" + engine.manifest.value.Release
		refs, captureErr := engine.repository.CaptureHeadRefs(
			[]string{cycle.TrackRef, releaseRef, engine.manifest.value.TargetRef})
		if captureErr != nil {
			err = captureErr
		} else {
			authorityExact := true
			for _, ref := range refs {
				switch ref.Ref {
				case cycle.TrackRef:
					authorityExact = authorityExact &&
						ref.State == gitx.RefDirect && ref.Head == before
				case releaseRef:
					release, parseErr := gitx.ParseOID(
						engine.repository.ObjectFormat(), releaseAuthority)
					authorityExact = authorityExact && parseErr == nil &&
						ref.State == gitx.RefDirect && ref.Head == release
				case engine.manifest.value.TargetRef:
					target, parseErr := gitx.ParseOID(
						engine.repository.ObjectFormat(), cycle.TargetHead)
					authorityExact = authorityExact && parseErr == nil &&
						ref.State == gitx.RefDirect && ref.Head == target
				}
			}
			if !authorityExact || len(refs) != 3 {
				fresh, freshErr := baton.ReadState(
					engine.git, engine.manifest.value.Release, engine.inertness)
				if freshErr == nil &&
					!implementationAuthorityCurrent(fresh, cycle) {
					return s.rejectStalePreparedSeal(
						ctx, engine, owner, cycle, record, claimToken)
				}
				err = runtimeFail("STALE_DISPATCH", freshErr)
			} else {
				release, releaseErr := gitx.ParseOID(
					engine.repository.ObjectFormat(), releaseAuthority)
				target, targetErr := gitx.ParseOID(
					engine.repository.ObjectFormat(), cycle.TargetHead)
				if releaseErr != nil || targetErr != nil {
					err = runtimeFail(
						"CORRUPT_JOURNAL", errors.Join(releaseErr, targetErr))
				} else {
					err = engine.repository.ApplyRefTransaction(refs, []gitx.RefOperation{
						{Kind: gitx.UpdateRef, Ref: cycle.TrackRef,
							NewHead: &candidate, Expected: &before},
						{Kind: gitx.VerifyRef, Ref: releaseRef, Expected: &release},
						{Kind: gitx.VerifyRef, Ref: engine.manifest.value.TargetRef,
							Expected: &target},
					})
				}
			}
		}
		disposition, _ = engine.workspaces.ReconcileSeal(key, before, candidate)
		if err != nil && disposition != gitx.SealAllNew {
			fresh, freshErr := baton.ReadState(
				engine.git, engine.manifest.value.Release, engine.inertness)
			if freshErr == nil &&
				!implementationAuthorityCurrent(fresh, cycle) {
				return s.rejectStalePreparedSeal(
					ctx, engine, owner, cycle, record, claimToken)
			}
			return sealedRecord{}, s.markSealCycleUncertain(
				ctx, owner, cycle, outer, prepared, err)
		}
	}
	if disposition != gitx.SealAllNew {
		return sealedRecord{}, s.markSealCycleUncertain(
			ctx, owner, cycle, outer, prepared, nil)
	}
	if err := s.validateSealedCycle(engine, cycle, record, true); err != nil {
		fresh, freshErr := baton.ReadState(
			engine.git, engine.manifest.value.Release, engine.inertness)
		if freshErr == nil && !implementationAuthorityCurrent(fresh, cycle) {
			return s.rejectStalePreparedSeal(
				ctx, engine, owner, cycle, record, claimToken)
		}
		return sealedRecord{}, s.markSealCycleUncertain(
			ctx, owner, cycle, outer, prepared, err)
	}
	body := mustJSON(record)
	if err := s.journal.ReconcileOwned(context.WithoutCancel(ctx), owner,
		journal.Completion{RunID: owner.RunID, EffectID: cycle.PreparedEffect,
			Token: claimToken, State: journal.Succeeded, Result: body,
			EventKind: "candidate_reconciled", EventBody: body, At: s.now().UTC(),
		}, journal.RecoveryAllNew); err != nil {
		return sealedRecord{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return record, nil
}

func (s *Service) reclaimAllOldPreparedSeal(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle implementationCycle,
	prepared journal.Effect,
) (journal.Effect, error) {
	if prepared.State != journal.Claimed ||
		prepared.CurrentClaim == "" ||
		prepared.ID != cycle.PreparedEffect {
		return journal.Effect{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := s.journal.ReconcileOwned(ctx, owner, journal.Completion{
		RunID: owner.RunID, EffectID: cycle.PreparedEffect,
		Token:     prepared.CurrentClaim,
		EventKind: "seal_reconciled_all_old", EventBody: []byte(cycle.Slice),
		At: s.now().UTC(),
	}, journal.RecoveryAllOld); err != nil {
		return journal.Effect{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	claim, err := s.journal.ClaimOwned(
		ctx,
		owner,
		cycle.PreparedEffect,
		s.now().UTC(),
		effectLease,
	)
	if err != nil {
		return journal.Effect{}, runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	prepared.State = journal.Claimed
	prepared.CurrentClaim = claim.Token
	return prepared, nil
}

func (s *Service) appendImplementationReceipt(ctx context.Context, engine *engine,
	owner journal.OwnerLease, cycle implementationCycle, record sealedRecord) error {
	if err := s.validateImplementationDispatchProof(
		ctx,
		engine,
		owner,
		cycle,
		record,
	); err != nil {
		return err
	}
	if err := validateSealedRecordCandidate(
		engine,
		cycle,
		record,
	); err != nil {
		return err
	}
	if err := s.validateSealedCycle(engine, cycle, record, true); err != nil {
		return err
	}
	state, err := baton.ReadState(
		engine.git, engine.manifest.value.Release, engine.inertness)
	if err != nil {
		return runtimeFail("BATON_UNAVAILABLE", err)
	}
	before := sliceFingerprint(state, cycle.Slice)
	return s.appendReceipt(ctx, engine, owner, state, before, record.Receipt)
}

func implementationRecord(
	cycle implementationCycle,
	outer journal.Effect,
	commands map[string]journal.Command,
) (sealedRecord, bool, error) {
	var record sealedRecord
	if outer.State == journal.Succeeded {
		if json.Unmarshal(outer.Result, &record) != nil ||
			!bytesEqualCanonicalJSON(outer.Result, record) ||
			!sealedRecordMatchesCycle(record, cycle) {
			return sealedRecord{}, false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return record, true, nil
	}
	prepared, ok := commands[cycle.PreparedEffect]
	if !ok {
		return sealedRecord{}, false, nil
	}
	if prepared.Kind != "git.seal.prepared" ||
		json.Unmarshal(prepared.Payload, &record) != nil ||
		!bytesEqualCanonicalJSON(prepared.Payload, record) ||
		!sealedRecordMatchesCycle(record, cycle) {
		return sealedRecord{}, false, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return record, true, nil
}

func (s *Service) completeRecoveredImplementation(ctx context.Context,
	owner journal.OwnerLease, effect journal.Effect, record sealedRecord) error {
	body := mustJSON(record)
	if err := s.journal.ReconcileOwned(context.WithoutCancel(ctx), owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim,
			State: journal.Succeeded, Result: body,
			Receipts:  []journal.Receipt{{Kind: "git_candidate", Body: body}},
			EventKind: "candidate_reconciled", EventBody: body, At: s.now().UTC(),
		}, journal.RecoveryAllNew); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) completeRecoveredPreparedSeal(
	ctx context.Context,
	owner journal.OwnerLease,
	effect journal.Effect,
	record sealedRecord,
) error {
	body := mustJSON(record)
	if err := s.journal.ReconcileOwned(
		context.WithoutCancel(ctx),
		owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: effect.ID,
			Token: effect.CurrentClaim, State: journal.Succeeded,
			Result: body, EventKind: "candidate_prepared_recovered",
			EventBody: body, At: s.now().UTC(),
		},
		journal.RecoveryAllNew,
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) markPreparedSealUncertain(
	ctx context.Context,
	owner journal.OwnerLease,
	effect journal.Effect,
	slice string,
	err error,
) error {
	if reconcileErr := s.journal.ReconcileOwned(
		context.WithoutCancel(ctx),
		owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: effect.ID,
			Token: effect.CurrentClaim, EventKind: "seal_uncertain",
			EventBody: []byte(slice), At: s.now().UTC(),
		},
		journal.RecoveryAmbiguous,
	); reconcileErr != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", reconcileErr)
	}
	return runtimeFail("RECOVERY_UNCERTAIN", err)
}

func (s *Service) markSealCycleUncertain(
	ctx context.Context,
	owner journal.OwnerLease,
	cycle implementationCycle,
	outer journal.Effect,
	prepared journal.Effect,
	err error,
) error {
	at := s.now().UTC()
	var completions []journal.Completion
	for _, effect := range []journal.Effect{prepared, outer} {
		if effect.State != journal.Claimed {
			continue
		}
		completions = append(completions, journal.Completion{
			RunID: owner.RunID, EffectID: effect.ID,
			Token:     effect.CurrentClaim,
			EventKind: "seal_cycle_uncertain",
			EventBody: []byte(cycle.Slice), At: at,
		})
	}
	if len(completions) != 0 {
		if reconcileErr := s.journal.ReconcileManyOwned(
			context.WithoutCancel(ctx),
			owner,
			completions,
			journal.RecoveryAmbiguous,
		); reconcileErr != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", reconcileErr)
		}
	}
	return runtimeFail("RECOVERY_UNCERTAIN", err)
}

func (s *Service) recoverUncertainPreparedCycle(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	cycle implementationCycle,
	outer journal.Effect,
	prepared journal.Effect,
	record sealedRecord,
) error {
	before, beforeErr := gitx.ParseOID(
		engine.repository.ObjectFormat(),
		record.Before,
	)
	candidate, candidateErr := gitx.ParseOID(
		engine.repository.ObjectFormat(),
		record.Candidate,
	)
	if beforeErr != nil || candidateErr != nil {
		return runtimeFail(
			"CORRUPT_JOURNAL",
			errors.Join(beforeErr, candidateErr),
		)
	}
	if record.RefreshFrom != "" && before == candidate {
		authorityErr := s.validateSealedCycle(
			engine,
			cycle,
			record,
			true,
		)
		if authorityErr == nil {
			if err := s.journal.RearmUncertainManyOwned(
				context.WithoutCancel(ctx),
				owner,
				owner.RunID,
				[]string{outer.ID, prepared.ID},
				s.now().UTC(),
			); err != nil {
				return runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
			return nil
		}
		if !IsCode(authorityErr, "STALE_DISPATCH") {
			return runtimeFail("RECOVERY_UNCERTAIN", authorityErr)
		}
		if err := s.journal.ResolveUncertainManyOwned(
			context.WithoutCancel(ctx),
			owner,
			owner.RunID,
			[]string{outer.ID, prepared.ID},
			"stale_authority",
			s.now().UTC(),
		); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return nil
	}
	disposition, err := engine.workspaces.ReconcileSeal(
		gitx.TrackKey{
			Release: engine.manifest.value.Release,
			Track:   cycle.Track,
		},
		before,
		candidate,
	)
	if err != nil {
		return runtimeFail("RECOVERY_FAILED", err)
	}
	if disposition == gitx.SealAmbiguous {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	sealed := disposition == gitx.SealAllNew
	authorityErr := s.validateSealedCycle(
		engine,
		cycle,
		record,
		sealed,
	)
	if authorityErr == nil {
		if err := s.journal.RearmUncertainManyOwned(
			context.WithoutCancel(ctx),
			owner,
			owner.RunID,
			[]string{outer.ID, prepared.ID},
			s.now().UTC(),
		); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return nil
	}
	if !IsCode(authorityErr, "STALE_DISPATCH") {
		return runtimeFail("RECOVERY_UNCERTAIN", authorityErr)
	}
	if disposition == gitx.SealAllNew {
		if err := s.rollbackCycleCandidate(
			engine,
			cycle,
			&record,
		); err != nil {
			return runtimeFail("RECOVERY_UNCERTAIN", err)
		}
	}
	if err := s.journal.ResolveUncertainManyOwned(
		context.WithoutCancel(ctx),
		owner,
		owner.RunID,
		[]string{outer.ID, prepared.ID},
		"stale_authority",
		s.now().UTC(),
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

// recoverImplementationClaims sweeps every historical seal cycle before any
// ready lane is selected. It deliberately does not filter by the current plan
// or slice set: removed and superseded work must still be made mechanically
// safe and terminal.
func (s *Service) recoverImplementationClaims(ctx context.Context, engine *engine,
	owner journal.OwnerLease) (bool, error) {
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if recovered, err := s.recoverHumanParkCheckpoint(
		ctx, engine, owner, snapshot,
	); recovered || err != nil {
		return recovered, err
	}
	attentions, err := s.journal.Attentions(ctx, owner.RunID)
	if err != nil {
		return true, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	parkedDispatches, err := activeAttentionWork(attentions)
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
	cyclesByOuter := make(map[string]implementationCycle)
	outerByPrepared := make(map[string]string)
	for _, command := range snapshot.Commands {
		if command.Kind != "git.seal" {
			continue
		}
		outer, ok := effects[command.ReplayKey]
		if !ok {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		cycle, err := validateImplementationCycleEnvelope(
			engine.manifest,
			command,
			outer,
		)
		if err != nil {
			return true, fmt.Errorf("recover seal envelope: %w", err)
		}
		if err := validateImplementationCycleObjects(
			engine.repository,
			cycle,
		); err != nil {
			return true, fmt.Errorf("recover seal objects: %w", err)
		}
		if prior, exists := outerByPrepared[cycle.PreparedEffect]; exists &&
			prior != command.ReplayKey {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		cyclesByOuter[command.ReplayKey] = cycle
		outerByPrepared[cycle.PreparedEffect] = command.ReplayKey
	}
	activeCycles, err := selectActiveImplementationCycles(
		engine.manifest,
		snapshot.Commands,
		effects,
		parkedDispatches,
	)
	if err != nil {
		return true, err
	}
	for _, command := range snapshot.Commands {
		if command.Kind != "git.seal" {
			continue
		}
		outer := effects[command.ReplayKey]
		if outer.State != journal.Uncertain {
			continue
		}
		cycle := cyclesByOuter[command.ReplayKey]
		prepared, present := effects[cycle.PreparedEffect]
		if !present {
			// A seal whose candidate was never prepared can be coupled to an
			// uncertain driver dispatch instead. That pair is classified by
			// recoverStaleClaimedDispatches.
			continue
		}
		if prepared.State != journal.Uncertain {
			return true, runtimeFail("RECOVERY_UNCERTAIN", nil)
		}
		preparedCommand, commandPresent := commands[prepared.ReplayKey]
		if !commandPresent {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		record, err := validatePreparedSealEnvelope(
			preparedCommand,
			prepared,
			cycle,
		)
		if err != nil {
			return true, err
		}
		incomplete, err := incompleteProductionImplementationDispatch(
			engine.manifest,
			commands,
			effects,
			cycle,
		)
		if err != nil {
			return true, err
		}
		if incomplete {
			// A production handoff that never became Succeeded has no
			// publishable dispatch proof. Its coupled ambiguity is preserved
			// and classified by recoverStaleClaimedDispatches.
			continue
		}
		if err := validateImplementationDispatchProof(
			engine,
			snapshot,
			cycle,
			record,
		); err != nil {
			return true, err
		}
		if err := validateSealedRecordCandidate(
			engine,
			cycle,
			record,
		); err != nil {
			return true, err
		}
		return true, s.recoverUncertainPreparedCycle(
			ctx,
			engine,
			owner,
			cycle,
			outer,
			prepared,
			record,
		)
	}
	for _, child := range snapshot.Effects {
		if child.Kind != "git.seal.prepared" ||
			child.State != journal.Claimed {
			continue
		}
		outerID, ok := outerByPrepared[child.ID]
		if !ok {
			return true, s.markPreparedSealUncertain(
				ctx, owner, child, "", nil)
		}
		cycle := cyclesByOuter[outerID]
		outer, ok := effects[outerID]
		childCommand, childCommandOK := commands[child.ReplayKey]
		if !ok || !childCommandOK {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		record, err := validatePreparedSealEnvelope(
			childCommand,
			child,
			cycle,
		)
		if err != nil {
			return true, fmt.Errorf("recover claimed prepared envelope: %w", err)
		}
		incomplete, err := incompleteProductionImplementationDispatch(
			engine.manifest,
			commands,
			effects,
			cycle,
		)
		if err != nil {
			return true, err
		}
		if incomplete {
			switch outer.State {
			case journal.Claimed:
				// The exact parent cycle below owns coupled recovery.
				continue
			case journal.OperationalFailed:
				if err := s.rollbackCycleCandidate(
					engine,
					cycle,
					nil,
				); err != nil {
					return true, s.markPreparedSealUncertain(
						ctx,
						owner,
						child,
						cycle.Slice,
						err,
					)
				}
				return true, s.terminalizeEffect(
					ctx,
					owner,
					child,
					"orphaned_dispatch",
				)
			default:
				return true, runtimeFail("RECOVERY_UNCERTAIN", nil)
			}
		}
		if err := validateImplementationDispatchProof(
			engine,
			snapshot,
			cycle,
			record,
		); err != nil {
			return true, fmt.Errorf("recover claimed prepared dispatch proof: %w", err)
		}
		if err := validateSealedRecordCandidate(
			engine,
			cycle,
			record,
		); err != nil {
			return true, fmt.Errorf("recover claimed prepared candidate: %w", err)
		}
		switch outer.State {
		case journal.Claimed:
			// The exact parent cycle below owns this child reconciliation.
			continue
		case journal.Pending:
			state, stateErr := baton.ReadState(
				engine.git, cycle.Release, engine.inertness)
			if stateErr != nil {
				return true, runtimeFail("BATON_UNAVAILABLE", stateErr)
			}
			applied, applyErr := implementationReceiptApplied(
				state,
				cycle,
				record,
			)
			if applyErr != nil {
				return true, applyErr
			}
			if applied {
				if err := s.completeRecoveredPreparedSeal(
					ctx, owner, child, record); err != nil {
					return true, err
				}
				return true, nil
			}
			claim, err := s.journal.ClaimOwned(
				ctx, owner, outer.ID, s.now().UTC(), effectLease)
			if err != nil {
				return true, runtimeFail("EFFECT_CLAIM_FAILED", err)
			}
			outer.State, outer.CurrentClaim = journal.Claimed, claim.Token
			recovered, retry, err := s.recoverImplementationCycle(
				ctx, engine, owner, cycle, outer)
			if err != nil {
				if IsCode(err, "STALE_DISPATCH") {
					return true, nil
				}
				return true, err
			}
			if retry {
				return true, nil
			}
			return true, s.appendImplementationReceipt(
				ctx, engine, owner, cycle, recovered)
		case journal.OperationalFailed:
			state, stateErr := baton.ReadState(
				engine.git, cycle.Release, engine.inertness)
			if stateErr != nil {
				return true, runtimeFail("BATON_UNAVAILABLE", stateErr)
			}
			applied, applyErr := implementationReceiptApplied(
				state,
				cycle,
				record,
			)
			if applyErr != nil {
				return true, applyErr
			}
			if applied {
				return true, s.completeRecoveredPreparedSeal(
					ctx, owner, child, record)
			}
			if err := s.rollbackCycleCandidate(
				engine, cycle, &record); err != nil {
				return true, s.markPreparedSealUncertain(
					ctx, owner, child, cycle.Slice, err)
			}
			return true, s.terminalizeEffect(
				ctx, owner, child, "orphaned_parent")
		case journal.Succeeded:
			// A seal parent can only succeed after its prepared child has
			// succeeded. Replaying an impossible partial ordering could publish
			// a journal-supplied candidate, so fail before any ref mutation.
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		case journal.Uncertain:
			state, stateErr := baton.ReadState(
				engine.git, cycle.Release, engine.inertness)
			applied := false
			if stateErr == nil {
				var applyErr error
				applied, applyErr = implementationReceiptApplied(
					state,
					cycle,
					record,
				)
				if applyErr != nil {
					return true, applyErr
				}
			}
			if applied {
				return true, s.completeRecoveredPreparedSeal(
					ctx, owner, child, record)
			}
			return true, s.markPreparedSealUncertain(
				ctx, owner, child, cycle.Slice, stateErr)
		default:
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	for _, command := range snapshot.Commands {
		if command.Kind != "git.seal" {
			continue
		}
		outer, ok := effects[command.ReplayKey]
		if !ok {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		cycle, err := validateImplementationCycleEnvelope(
			engine.manifest,
			command,
			outer,
		)
		if err != nil {
			return true, fmt.Errorf("recover terminal seal envelope: %w", err)
		}
		if err := validateImplementationCycleObjects(
			engine.repository,
			cycle,
		); err != nil {
			return true, fmt.Errorf("recover terminal seal objects: %w", err)
		}
		if active, parked := activeCycles[outer.ID]; parked {
			state, stateErr := baton.ReadState(
				engine.git,
				cycle.Release,
				engine.inertness,
			)
			if stateErr != nil {
				return true,
					runtimeFail("BATON_UNAVAILABLE", stateErr)
			}
			slice, present := state.Slice(cycle.Slice)
			current := present &&
				implementationAuthorityCurrent(state, cycle)
			if current &&
				active.dispatch.State == journal.Claimed {
				dispatchCommand, found := commands[active.dispatch.ReplayKey]
				if !found {
					return true, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				dispatch, dispatchErr := validateDriverRecoveryCommand(
					engine.manifest,
					dispatchCommand,
					active.dispatch,
				)
				if dispatchErr != nil {
					return true, dispatchErr
				}
				contextErr := validateCurrentProductionDispatchContext(
					ctx,
					engine,
					dispatch,
				)
				switch {
				case contextErr == nil:
				case IsCode(contextErr, "STALE_DISPATCH"):
					current = false
				default:
					return true, contextErr
				}
			}
			if !current ||
				active.dispatch.State ==
					journal.OperationalFailed {
				if err := s.retireStaleImplementationCycle(
					ctx,
					engine,
					owner,
					active,
					commands,
					effects,
				); err != nil {
					return true, err
				}
				return true, nil
			}
			if active.attention.State == journal.AttentionOpen {
				if active.dispatch.State != journal.Claimed {
					return true, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				continue
			}
			return true, s.executeClaimedImplementationCycle(
				ctx,
				engine,
				owner,
				state,
				slice,
				active,
			)
		}
		if outer.State != journal.Pending &&
			outer.State != journal.Claimed &&
			outer.State != journal.Succeeded {
			continue
		}
		if outer.State == journal.Pending {
			claim, err := s.journal.ClaimOwned(
				ctx, owner, outer.ID, s.now().UTC(), effectLease)
			if err != nil {
				return true, runtimeFail("EFFECT_CLAIM_FAILED", err)
			}
			outer.State, outer.CurrentClaim = journal.Claimed, claim.Token
		}
		record, hasRecord, err := implementationRecord(cycle, outer, commands)
		if err != nil {
			return true, err
		}
		incomplete := false
		if hasRecord {
			incomplete, err = incompleteProductionImplementationDispatch(
				engine.manifest,
				commands,
				effects,
				cycle,
			)
			if err != nil {
				return true, err
			}
		}
		if hasRecord && !incomplete {
			if err := validateImplementationDispatchProof(
				engine,
				snapshot,
				cycle,
				record,
			); err != nil {
				return true, fmt.Errorf("recover seal dispatch proof: %w", err)
			}
			if err := validateSealedRecordCandidate(
				engine,
				cycle,
				record,
			); err != nil {
				return true, fmt.Errorf("recover sealed candidate: %w", err)
			}
		}
		if prepared, ok := effects[cycle.PreparedEffect]; ok {
			preparedCommand, commandOK := commands[cycle.PreparedEffect]
			if !commandOK {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			preparedRecord, err := validatePreparedSealEnvelope(
				preparedCommand,
				prepared,
				cycle,
			)
			if err != nil {
				return true, err
			}
			if hasRecord &&
				!bytes.Equal(mustJSON(preparedRecord), mustJSON(record)) {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if outer.State == journal.Succeeded &&
				(prepared.State != journal.Succeeded ||
					!bytes.Equal(
						prepared.Result,
						mustJSON(preparedRecord),
					)) {
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
		} else if outer.State == journal.Succeeded {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		state, err := baton.ReadState(engine.git, cycle.Release, engine.inertness)
		if err != nil {
			if baton.ErrorCode(err) != "CHANGED_OWNER_HEAD" || !hasRecord {
				return true, runtimeFail("BATON_UNAVAILABLE", err)
			}
			rollbackErr := s.rollbackCycleCandidate(
				engine, cycle, &record)
			if rollbackErr != nil {
				return true, s.markSealCycleUncertain(
					ctx, owner, cycle, outer,
					effects[cycle.PreparedEffect], rollbackErr)
			}
			state, err = baton.ReadState(
				engine.git, cycle.Release, engine.inertness)
			if err != nil {
				return true, runtimeFail("BATON_UNAVAILABLE", err)
			}
		}
		applied := false
		if hasRecord {
			applied, err = implementationReceiptApplied(
				state,
				cycle,
				record,
			)
			if err != nil {
				return true, err
			}
		}
		if applied {
			if outer.State == journal.Claimed {
				if err := s.completeRecoveredImplementation(
					ctx, owner, outer, record); err != nil {
					return true, err
				}
				return true, nil
			}
			continue
		}
		if implementationAuthorityCurrent(state, cycle) {
			if outer.State == journal.Succeeded {
				if !hasRecord {
					return true, runtimeFail("CORRUPT_JOURNAL", nil)
				}
				return true, s.appendImplementationReceipt(
					ctx, engine, owner, cycle, record)
			}
			recovered, retry, err := s.recoverImplementationCycle(
				ctx, engine, owner, cycle, outer)
			if err != nil {
				if IsCode(err, "STALE_DISPATCH") {
					return true, nil
				}
				return true, err
			}
			if retry {
				return true, nil
			}
			return true, s.appendImplementationReceipt(
				ctx, engine, owner, cycle, recovered)
		}

		var recordPointer *sealedRecord
		if hasRecord && !incomplete {
			recordPointer = &record
		}
		if err := s.rollbackCycleCandidate(engine, cycle, recordPointer); err != nil {
			return true, s.markSealCycleUncertain(
				ctx, owner, cycle, outer,
				effects[cycle.PreparedEffect], err)
		}
		changed := false
		for _, childID := range []string{cycle.DispatchEffect, cycle.PreparedEffect} {
			child, ok := effects[childID]
			if !ok || (child.State != journal.Pending && child.State != journal.Claimed) {
				continue
			}
			if err := s.terminalizeEffect(
				ctx, owner, child, "stale_authority"); err != nil {
				return true, err
			}
			changed = true
		}
		if outer.State == journal.Claimed {
			if err := s.terminalizeEffect(
				ctx, owner, outer, "stale_authority"); err != nil {
				return true, err
			}
			changed = true
		}
		if changed {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) recoverClaimedEffects(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
) error {
	for {
		recovered, err := s.recoverClaimedTrackBase(
			ctx,
			engine,
			owner,
		)
		if err != nil {
			return err
		}
		if recovered {
			continue
		}
		recovered, err = s.recoverImplementationClaims(ctx, engine, owner)
		if err != nil {
			return err
		}
		if recovered {
			continue
		}
		recovered, err = s.recoverClaimedBatonAction(ctx, engine, owner)
		if err != nil {
			return err
		}
		if recovered {
			continue
		}
		recovered, err = s.recoverStaleClaimedDispatches(
			ctx, engine, owner)
		if err != nil {
			return err
		}
		if recovered {
			continue
		}
		recovered, err = s.recoverHostCheckClaims(
			ctx, engine, owner)
		if err != nil {
			return err
		}
		if recovered {
			continue
		}
		return nil
	}
}

func (s *Service) driveLoop(ctx context.Context, engine *engine, owner journal.OwnerLease,
	proposalPending bool) error {
	for {
		projection, err := s.journal.ControlProjection(ctx, owner.RunID)
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || projection.Desired != "running" {
			return err
		}
		if err := s.recoverClaimedEffects(ctx, engine, owner); err != nil {
			return err
		}
		recoveryPending, err := s.driverRecoveryPending(ctx, owner.RunID)
		if err != nil {
			return err
		}
		if recoveryPending {
			return nil
		}
		state, err := baton.ReadState(engine.git, engine.manifest.value.Release, engine.inertness)
		if err != nil {
			return runtimeFail("BATON_UNAVAILABLE", err)
		}
		if state.Plan.TargetStale {
			return nil
		}
		plannerNeeded := false
		for _, slice := range state.Slices {
			plannerNeeded = plannerNeeded || slice.NextRole == "planner"
		}
		plannerNeeded = plannerNeeded ||
			state.Assembly.NextRole == "planner"
		ready := make([]string, 0, len(state.Tracks))
		for _, track := range state.Tracks {
			for _, slice := range track.Slices {
				if slice.Status == "ready" && slice.NextRole != "none" &&
					slice.NextRole != "merge" && slice.NextRole != "planner" {
					ready = append(ready, slice.Location.Slice.ID)
					break
				}
			}
		}
		if len(ready) != 0 {
			semaphore := make(chan struct{}, engine.manifest.value.MaxParallelTracks)
			var wait sync.WaitGroup
			var mu sync.Mutex
			progress := false
			laneErrors := make([]error, len(ready))
			for index, sliceID := range ready {
				// Admission is plan-ordered. Acquiring capacity before launch
				// prevents goroutine scheduling from choosing which later
				// track enters the bounded set first.
				semaphore <- struct{}{}
				wait.Add(1)
				go func(position int, id string) {
					defer wait.Done()
					defer func() { <-semaphore }()
					err := s.advanceSlice(ctx, engine, owner, id)
					mu.Lock()
					if err == nil {
						progress = true
					} else if IsCode(err, "STALE_DISPATCH") {
						progress = true
					} else if !IsCode(err, "EFFECT_PARKED") {
						laneErrors[position] = err
					}
					mu.Unlock()
				}(index, sliceID)
			}
			wait.Wait()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if progress {
				continue
			}
			var laneErr error
			for _, err := range laneErrors {
				laneErr = errors.Join(laneErr, err)
			}
			if laneErr != nil {
				return laneErr
			}
			if plannerNeeded {
				if proposalPending {
					return nil
				}
				return s.proposeRevision(ctx, engine, owner, state)
			}
			return nil
		}
		if plannerNeeded {
			if proposalPending {
				return nil
			}
			return s.proposeRevision(ctx, engine, owner, state)
		}
		switch {
		case state.Assembly.Outcome == "merged":
			return nil
		case state.Assembly.NextRole == "merge" && state.Assembly.Outcome != "pass":
			if err := s.prepareAssembly(ctx, engine, owner, state); err != nil {
				return err
			}
		case state.Assembly.NextRole == "verifier":
			if err := s.verifyAssembly(ctx, engine, owner, state); err != nil {
				return err
			}
		case state.Assembly.NextRole == "merge" && state.Assembly.Outcome == "pass":
			if err := s.mergeAssembly(ctx, engine, owner, state); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (s *Service) recoverClaimedBatonAction(ctx context.Context, engine *engine,
	owner journal.OwnerLease) (bool, error) {
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
		if effect.State != journal.Pending &&
			effect.State != journal.Claimed &&
			effect.State != journal.Uncertain {
			continue
		}
		switch effect.Kind {
		case "baton.install", "baton.append_receipt", "baton.assembly_verdict",
			"baton.prepare_assembly", "baton.merge":
			// Reconciled below.
		default:
			continue
		}
		command, ok := commands[effect.ReplayKey]
		if !ok {
			return true, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		persisted, err := parseActionCommand(command.Payload)
		if err != nil {
			return true, err
		}
		if err := validateBatonActionEnvelope(
			engine, command, effect, persisted); err != nil {
			return true, err
		}
		if effect.State == journal.Pending {
			claim, err := s.journal.ClaimOwned(
				ctx, owner, effect.ID, s.now().UTC(), effectLease)
			if err != nil {
				return true, runtimeFail("EFFECT_CLAIM_FAILED", err)
			}
			effect.State = journal.Claimed
			effect.CurrentClaim = claim.Token
		}
		if effect.State == journal.Uncertain {
			truth, _, classifyErr := classifyBatonAction(
				engine,
				effect.Kind,
				persisted,
			)
			switch truth {
			case actionStale:
				if err := s.journal.ResolveUncertainOwned(
					context.WithoutCancel(ctx),
					owner,
					owner.RunID,
					effect.ID,
					"stale_authority",
					s.now().UTC(),
				); err != nil {
					return true,
						runtimeFail("JOURNAL_WRITE_FAILED", err)
				}
				return true, nil
			case actionAllOld, actionAllNew:
				if classifyErr != nil {
					return true,
						runtimeFail("RECOVERY_UNCERTAIN", classifyErr)
				}
				if err := s.journal.RearmUncertainOwned(
					context.WithoutCancel(ctx),
					owner,
					owner.RunID,
					effect.ID,
					s.now().UTC(),
				); err != nil {
					return true,
						runtimeFail("JOURNAL_WRITE_FAILED", err)
				}
				claim, err := s.journal.ClaimOwned(
					ctx,
					owner,
					effect.ID,
					s.now().UTC(),
					effectLease,
				)
				if err != nil {
					return true,
						runtimeFail("EFFECT_CLAIM_FAILED", err)
				}
				effect.State = journal.Claimed
				effect.CurrentClaim = claim.Token
			case actionAmbiguous:
				return true,
					runtimeFail("RECOVERY_UNCERTAIN", classifyErr)
			default:
				return true, runtimeFail("CORRUPT_JOURNAL", nil)
			}
		}
		action, cleanup, err := persistedBatonAction(
			engine,
			effect.Kind,
			persisted,
		)
		if err != nil {
			return true, err
		}
		truth, _, err := s.reconcileClaimedBatonAction(
			ctx, engine, owner, effect, persisted, action, false, false)
		var cleanupErr error
		if cleanup != nil {
			cleanupErr = cleanup()
		}
		if cleanupErr != nil {
			return true, errors.Join(err, cleanupErr)
		}
		if err != nil {
			if truth == actionAllOld && !IsCode(err, "RECOVERY_UNCERTAIN") {
				return true, nil
			}
			return true, err
		}
		if truth == actionStale {
			return true, nil
		}
		return true, nil
	}
	return false, nil
}

func plannerAuthorityBefore(authority planProposalAuthority) string {
	if authority.PlannerAttempt <= 1 && authority.ReplanDecision == "" {
		return workIdentity(authority.Release, authority.PriorPlan, authority.ReleaseRef, authority.ReleaseHead, authority.TargetRef, authority.TargetHead)
	}
	return workIdentity(
		authority.Release,
		authority.PriorPlan,
		authority.ReleaseRef,
		authority.ReleaseHead,
		authority.TargetRef,
		authority.TargetHead,
		strconv.FormatInt(authority.PlannerAttempt, 10),
		authority.ReplanDecision,
	)
}

func proposalInstallWork(proposal admittedPlanProposal) string {
	return workIdentity(
		"plan-install",
		proposal.replayKey,
		proposal.authority.Before,
		proposal.plan.Digest(),
	)
}

const planAuthorityVersion = "sworn.plan-authority/v1"

type planAuthorityCommand struct {
	Version    string `json:"version"`
	PlanDigest string `json:"plan_digest"`
}

func effectivePlanAuthority(
	manifest admittedManifest,
	snapshot journal.Snapshot,
	selected ...*admittedPlanProposal,
) (string, error) {
	digests := make(map[string]struct{})
	if manifest.value.Authority.BootstrapApprovedPlanDigest != nil {
		digests[*manifest.value.Authority.BootstrapApprovedPlanDigest] = struct{}{}
	}
	for _, command := range snapshot.Commands {
		if command.Kind != "plan_authority" {
			continue
		}
		var wire planAuthorityCommand
		if json.Unmarshal(command.Payload, &wire) != nil ||
			!bytes.Equal(command.Payload, mustJSON(wire)) ||
			wire.Version != planAuthorityVersion ||
			!runtimeDigestPattern.MatchString(wire.PlanDigest) ||
			command.ReplayKey != "plan-authority/"+
				strings.TrimPrefix(wire.PlanDigest, "sha256:") {
			return "", runtimeFail("CORRUPT_JOURNAL", nil)
		}
		digests[wire.PlanDigest] = struct{}{}
	}
	var current *admittedPlanProposal
	if len(selected) > 0 {
		current = selected[0]
	}
	if current != nil {
		expected, err := approvalCommandForProposal(manifest, *current)
		if err != nil {
			return "", err
		}
		commands := make(map[string]journal.Command, len(snapshot.Commands))
		for _, command := range snapshot.Commands {
			commands[command.ReplayKey] = command
		}
		for _, effect := range snapshot.Effects {
			if effect.Kind != approvalEffectKind || effect.State != journal.Succeeded {
				continue
			}
			stored, ok := commands[effect.ReplayKey]
			if !ok {
				return "", runtimeFail("CORRUPT_JOURNAL", nil)
			}
			command, err := parseApprovalCommand(stored)
			if err != nil {
				return "", err
			}
			boundExpected := expected
			boundExpected.ActorClass = command.ActorClass
			boundExpected.ActorAuthority = command.ActorAuthority
			if !reflect.DeepEqual(command, boundExpected) {
				// Historical approvals are deliberately inert.
				continue
			}
			if err := validateApprovalAuthorityWithSnapshot(manifest, *current, snapshot, command, false); err != nil {
				return "", err
			}
			if _, err := parseSucceededApproval(command, effect); err != nil {
				return "", err
			}
			digests[command.PlanDigest] = struct{}{}
		}
	}
	if len(digests) > 1 {
		return "", runtimeFail("AUTHORITY_CONFLICT", nil)
	}
	for digest := range digests {
		return digest, nil
	}
	return "", nil
}

func validateInstallEffectPrecedence(
	engine *engine,
	snapshot journal.Snapshot,
) error {
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "baton.install" {
			continue
		}
		command, ok := commands[effect.ReplayKey]
		if !ok {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		persisted, err := parseActionCommand(command.Payload)
		if err != nil || validateBatonActionEnvelope(
			engine, command, effect, persisted) != nil {
			return runtimeFail("CORRUPT_JOURNAL", err)
		}
		validateSucceeded, precedenceErr := installEffectPrecedence(effect.State)
		if precedenceErr != nil {
			return precedenceErr
		}
		if validateSucceeded {
			if _, err := validateSucceededBatonAction(
				engine, command, effect, persisted); err != nil {
				return err
			}
		}
	}
	return nil
}

func installEffectPrecedence(state journal.EffectState) (bool, error) {
	switch state {
	case journal.Pending, journal.Claimed, journal.Uncertain:
		return false, runtimeFail("INSTALL_RECOVERY_PENDING", nil)
	case journal.OperationalFailed:
		return false, runtimeFail("INSTALL_FAILED", nil)
	case journal.Succeeded:
		return true, nil
	default:
		return false, runtimeFail("CORRUPT_JOURNAL", nil)
	}
}

func validateSavedPlanAdoption(
	engine *engine,
	state baton.State,
	authorityDigest string,
) (bool, error) {
	if authorityDigest == "" {
		return false, nil
	}
	manifest := engine.manifest
	metadata := state.Plan.Metadata
	if state.Release != manifest.value.Release ||
		state.Repository != manifest.value.Authority.Project ||
		metadata.Repository != manifest.value.Authority.Project ||
		metadata.Release != manifest.value.Release ||
		metadata.TargetRef != manifest.value.TargetRef ||
		state.Plan.Digest != authorityDigest ||
		state.Plan.TargetStale ||
		state.Refs.Release.Ref != "refs/heads/release-wt/"+manifest.value.Release ||
		state.Refs.Release.Head == "" ||
		state.Refs.Target.Ref != manifest.value.TargetRef ||
		state.Refs.Target.Head == "" ||
		state.Plan.Approval.Receipt.Role != "planner" ||
		state.Plan.Approval.Receipt.Result != "approved" ||
		state.Plan.Approval.Receipt.Plan != state.Plan.OID ||
		state.Plan.Approval.Receipt.Target == nil ||
		*state.Plan.Approval.Receipt.Target != state.Refs.Target.Head {
		return false, runtimeFail("INVALID_AUTHORITY", nil)
	}
	found := false
	var saved baton.Plan
	for _, historical := range state.Plan.History {
		if historical.OID != state.Plan.OID {
			continue
		}
		if found || historical.Plan.Digest() != authorityDigest {
			return false, runtimeFail("INVALID_AUTHORITY", nil)
		}
		found = true
		saved = historical.Plan
	}
	if !found {
		return false, runtimeFail("INVALID_AUTHORITY", nil)
	}
	if validateApprovalRef(manifest, saved) != nil {
		return false, runtimeFail("INVALID_AUTHORITY", nil)
	}
	return true, nil
}

func captureProposalRefs(
	repository *gitx.Repository,
	manifest admittedManifest,
) (gitx.RefHead, gitx.RefHead, error) {
	releaseRef := "refs/heads/release-wt/" + manifest.value.Release
	refs, err := repository.CaptureHeadRefs(
		[]string{releaseRef, manifest.value.TargetRef})
	if err != nil || len(refs) != 2 {
		return gitx.RefHead{}, gitx.RefHead{},
			runtimeFail("INVALID_AUTHORITY_STATE", err)
	}
	byRef := make(map[string]gitx.RefHead, len(refs))
	for _, ref := range refs {
		byRef[ref.Ref] = ref
	}
	release, releaseOK := byRef[releaseRef]
	target, targetOK := byRef[manifest.value.TargetRef]
	if !releaseOK || !targetOK ||
		(target.State != gitx.RefDirect) ||
		(release.State != gitx.RefAbsent && release.State != gitx.RefDirect) {
		return gitx.RefHead{}, gitx.RefHead{},
			runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	return release, target, nil
}

func proposalMatchesPendingAuthority(
	proposal admittedPlanProposal,
	release gitx.RefHead,
	target gitx.RefHead,
	state baton.State,
	stateErr error,
) bool {
	metadata := proposal.plan.Metadata()
	authority := proposal.authority
	if authority.ReleaseRef != release.Ref ||
		authority.TargetRef != target.Ref ||
		authority.TargetHead != target.Head.String() {
		return false
	}
	if metadata.Revision == 1 {
		return stateErr != nil &&
			baton.ErrorCode(stateErr) == "REF_NOT_FOUND" &&
			release.State == gitx.RefAbsent &&
			authority.ReleaseHead == "" &&
			authority.PriorPlan == ""
	}
	return stateErr == nil &&
		release.State == gitx.RefDirect &&
		release.Head.String() == authority.ReleaseHead &&
		target.Head.String() == state.Refs.Target.Head &&
		release.Head.String() == state.Refs.Release.Head &&
		metadata.Revision == state.Plan.Metadata.Revision+1 &&
		metadata.PreviousPlan != nil &&
		*metadata.PreviousPlan == state.Plan.OID &&
		authority.PriorPlan == state.Plan.OID
}

func proposalHasInstalledEffect(
	engine *engine,
	snapshot journal.Snapshot,
	proposal admittedPlanProposal,
) (bool, error) {
	work := proposalInstallWork(proposal)
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	found := false
	for _, effect := range snapshot.Effects {
		if effect.Kind != "baton.install" ||
			effect.State != journal.Succeeded ||
			effect.BeforeDigest != work {
			continue
		}
		if found {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		command, ok := commands[effect.ReplayKey]
		if !ok {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		persisted, err := parseActionCommand(command.Payload)
		if err != nil {
			return false, err
		}
		var input installActionInput
		if parseCanonicalActionInput(persisted.Input, &input) != nil ||
			!bytes.Equal(input.PlanBytes, proposal.plan.Bytes()) ||
			input.PlanDigest != proposal.plan.Digest() ||
			input.Reference != proposal.plan.Metadata().ApprovalRef ||
			persisted.Authority.Release !=
				proposal.authority.Release ||
			persisted.Authority.Plan !=
				proposal.authority.PriorPlan ||
			persisted.Authority.ReleaseHead !=
				proposal.authority.ReleaseHead ||
			persisted.Authority.TargetRef !=
				proposal.authority.TargetRef ||
			persisted.Authority.TargetHead !=
				proposal.authority.TargetHead ||
			persisted.Authority.OwnerRef !=
				proposal.authority.ReleaseRef ||
			persisted.Authority.OwnerHead !=
				proposal.authority.ReleaseHead ||
			persisted.Authority.Before != work {
			return false, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if _, err := validateSucceededBatonAction(
			engine,
			command,
			effect,
			persisted,
		); err != nil {
			return false, err
		}
		found = true
	}
	return found, nil
}

func proposalMatchesAppliedPlan(
	proposal admittedPlanProposal,
	state baton.State,
	stateErr error,
) bool {
	if stateErr != nil ||
		state.Plan.Digest != proposal.plan.Digest() ||
		state.Plan.Metadata.Revision != proposal.plan.Metadata().Revision ||
		state.Plan.Approval.Receipt.Target == nil ||
		*state.Plan.Approval.Receipt.Target != proposal.authority.TargetHead {
		return false
	}
	return true
}

func planExecutionEffectRecorded(snapshot journal.Snapshot) bool {
	for _, effect := range snapshot.Effects {
		switch effect.Kind {
		case "git.prepare_track_base", "git.seal.prepared", "git.seal",
			"baton.append_receipt", "baton.prepare_assembly",
			"baton.assembly_verdict", "baton.merge":
			return true
		}
	}
	return false
}

func proposalActivationRecorded(
	proposal admittedPlanProposal,
	found bool,
	installed bool,
	state baton.State,
	stateErr error,
	authorityDigest string,
	snapshot journal.Snapshot,
) bool {
	if !found || authorityDigest == "" ||
		proposal.plan.Digest() != authorityDigest ||
		!proposalMatchesAppliedPlan(proposal, state, stateErr) {
		return false
	}
	return installed || planExecutionEffectRecorded(snapshot)
}

func proposalAwaitsExactAuthority(
	proposal admittedPlanProposal,
	found bool,
	state baton.State,
	stateErr error,
	authorityDigest string,
) bool {
	return found && proposalMatchesAppliedPlan(proposal, state, stateErr) &&
		proposal.plan.Digest() != authorityDigest
}

func selectPlanProposal(
	engine *engine,
	snapshot journal.Snapshot,
	proposals []admittedPlanProposal,
	state baton.State,
	stateErr error,
) (admittedPlanProposal, bool, bool, error) {
	if engine == nil {
		return admittedPlanProposal{}, false, false,
			runtimeFail("INVALID_ENGINE", nil)
	}
	release, target, err := captureProposalRefs(
		engine.repository,
		engine.manifest,
	)
	if err != nil {
		return admittedPlanProposal{}, false, false, err
	}
	var pending []admittedPlanProposal
	var applied []admittedPlanProposal
	var installed []admittedPlanProposal
	for _, proposal := range proposals {
		if proposalMatchesPendingAuthority(
			proposal, release, target, state, stateErr) {
			pending = append(pending, proposal)
		}
		if proposalMatchesAppliedPlan(proposal, state, stateErr) {
			hasInstalledEffect, err := proposalHasInstalledEffect(
				engine,
				snapshot,
				proposal,
			)
			if err != nil {
				return admittedPlanProposal{}, false, false, err
			}
			if hasInstalledEffect {
				installed = append(installed, proposal)
			} else {
				applied = append(applied, proposal)
			}
		}
	}
	if len(pending) > 1 || len(applied) > 1 || len(installed) > 1 {
		return admittedPlanProposal{}, false, false,
			runtimeFail("AMBIGUOUS_PLAN_PROPOSAL", nil)
	}
	if len(pending) == 1 {
		return pending[0], true, false, nil
	}
	if len(installed) == 1 {
		return installed[0], true, true, nil
	}
	if len(applied) == 1 {
		return applied[0], true, false, nil
	}
	return admittedPlanProposal{}, false, false, nil
}

func (s *Service) proposePlan(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	current *baton.State,
	revision int64,
) error {
	return s.proposePlanAttempt(ctx, engine, owner, current, revision, 1, "")
}

func (s *Service) proposePlanAttempt(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	current *baton.State,
	revision int64,
	plannerAttempt int64,
	replanDecision string,
) error {
	delegationSnapshot, snapshotErr := s.journal.Snapshot(ctx, owner.RunID)
	if snapshotErr != nil {
		return runtimeFail("JOURNAL_READ_FAILED", snapshotErr)
	}
	delegation, delegationErr := currentCaptainDelegation(delegationSnapshot)
	if delegationErr != nil {
		return delegationErr
	}
	if delegation.Epoch > 0 {
		limits := delegation.Envelope.Limits
		if !delegation.Active || revision < limits.MinimumPlanRevision || revision > limits.MaximumPlanRevision ||
			plannerAttempt < 1 || plannerAttempt > limits.MaximumPlannerAttemptsPerRevision ||
			delegation.Decisions >= limits.MaximumTotalCaptainDecisions ||
			(plannerAttempt > 1 && (delegation.ReplanSpent < 1 || delegation.ReplanSpent > limits.ReplanBudget)) {
			return runtimeFail("CAPTAIN_PLAN_POLICY_REFUSED", nil)
		}
	}
	releaseRef := "refs/heads/release-wt/" + engine.manifest.value.Release
	refs, err := engine.repository.CaptureHeadRefs(
		[]string{releaseRef, engine.manifest.value.TargetRef})
	if err != nil || len(refs) != 2 {
		return runtimeFail("INVALID_AUTHORITY_STATE", err)
	}
	byRef := make(map[string]gitx.RefHead, len(refs))
	for _, ref := range refs {
		byRef[ref.Ref] = ref
	}
	release, target := byRef[releaseRef], byRef[engine.manifest.value.TargetRef]
	if target.State != gitx.RefDirect {
		return runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	authority := planProposalAuthority{
		Release:        engine.manifest.value.Release,
		ReleaseRef:     releaseRef,
		TargetRef:      engine.manifest.value.TargetRef,
		TargetHead:     target.Head.String(),
		PlannerAttempt: plannerAttempt,
		ReplanDecision: replanDecision,
	}
	if plannerAttempt < 1 || (plannerAttempt == 1 && replanDecision != "") || (plannerAttempt > 1 && replanDecision == "") {
		return runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	snapshotHead := target.Head
	if current == nil {
		if revision != 1 || release.State != gitx.RefAbsent {
			return runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
	} else {
		if revision != current.Plan.Metadata.Revision+1 ||
			release.State != gitx.RefDirect ||
			release.Head.String() != current.Refs.Release.Head ||
			target.Head.String() != current.Refs.Target.Head ||
			current.Refs.Release.Ref != releaseRef ||
			current.Refs.Target.Ref != engine.manifest.value.TargetRef {
			return runtimeFail("STALE_DISPATCH", nil)
		}
		authority.PriorPlan = current.Plan.OID
		authority.ReleaseHead = release.Head.String()
		snapshotHead = release.Head
	}
	authority.Before = plannerAuthorityBefore(authority)
	authority.SourceWork = driverWorkIdentity(
		engine.manifest.digest, "", driver.PlannerProposal,
		revision, authority.Before)
	workspace, err := engine.workspaces.OpenSnapshot(snapshotHead)
	if err != nil {
		return runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	invocationScope := ""
	if plannerAttempt > 1 {
		dispatchWork := driverWorkIdentity(
			engine.manifest.digest, "", driver.PlannerProposal,
			revision, authority.Before,
		)
		invocationScope = strings.TrimPrefix(dispatchWork, "sha256:")[:12]
	}
	submission, runErr := s.dispatchRoleWithScope(
		ctx, engine, workspace, driver.RolePlanner, "",
		driver.PlannerProposal, revision, authority.Before, owner, invocationScope)
	closeErr := workspace.Close()
	if runErr != nil {
		return runErr
	}
	if closeErr != nil {
		return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	after, err := engine.repository.CaptureHeadRefs(
		[]string{releaseRef, engine.manifest.value.TargetRef})
	if err != nil || !refVectorEqual(refs, after) {
		return runtimeFail("PLANNER_MUTATED_AUTHORITY", err)
	}
	body, err := exactBytes(submission.Plan)
	if err != nil {
		return err
	}
	plan, err := baton.ParsePlan(body)
	if err != nil || validatePlanBinding(engine.manifest, plan, current) != nil {
		return runtimeFail("INVALID_PLAN", err)
	}
	return s.recordProposal(ctx, owner.RunID, plan, authority)
}

func (s *Service) proposeRevision(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	state baton.State,
) error {
	return s.proposePlan(
		ctx, engine, owner, &state, state.Plan.Metadata.Revision+1)
}

func (s *Service) reviewDelegatedProposal(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	proposal admittedPlanProposal,
	snapshot journal.Snapshot,
) (string, bool, error) {
	delegation, err := currentCaptainDelegation(snapshot)
	if err != nil {
		return "", false, err
	}
	if delegation.Epoch == 0 {
		return "", false, nil
	}
	refuse := func(code string) (string, bool, error) {
		return "", true, s.appendCaptainRefusal(
			ctx, engine.manifest, proposal, delegation, code)
	}
	if !delegation.Active {
		return refuse("CAPTAIN_DELEGATION_REVOKED")
	}
	envelope := delegation.Envelope
	metadata := proposal.plan.Metadata()
	class, classErr := approvalDecisionClass(proposal)
	var prior *baton.Plan
	var lineageState *baton.State
	if metadata.Revision > 1 {
		state, stateErr := baton.ReadState(engine.git, engine.manifest.value.Release, engine.inertness)
		if stateErr != nil {
			return "", true, runtimeFail("BATON_UNAVAILABLE", stateErr)
		}
		for _, history := range state.Plan.History {
			if history.OID == proposal.authority.PriorPlan {
				copy := history.Plan
				prior = &copy
			}
		}
		if prior == nil {
			return "", true, runtimeFail("CAPTAIN_DECISION_STALE", nil)
		}
		lineageState = &state
	}
	plannerAttempt := proposal.authority.PlannerAttempt
	if plannerAttempt == 0 {
		plannerAttempt = 1
	}
	lineageOK := envelope.ReleaseLineageAnchor.State == "absent" && metadata.Revision == 1 && proposal.authority.PriorPlan == "" && proposal.authority.ReleaseHead == ""
	if envelope.ReleaseLineageAnchor.State == "present" && lineageState != nil {
		anchor := envelope.ReleaseLineageAnchor
		for _, history := range lineageState.Plan.History {
			if history.OID == anchor.PlanOID && history.Revision == anchor.PlanRevision && history.InstallHead == anchor.ReleaseHead {
				lineageOK = true
			}
		}
	}
	if classErr != nil || !lineageOK || envelope.RunID != engine.manifest.value.RunID || envelope.ManifestDigest != engine.manifest.digest || envelope.Project != engine.manifest.value.Authority.Project || envelope.Release != engine.manifest.value.Release || envelope.ReleaseRef != proposal.authority.ReleaseRef || envelope.TargetRef != proposal.authority.TargetRef || envelope.TargetHead != proposal.authority.TargetHead || metadata.Revision < envelope.Limits.MinimumPlanRevision || metadata.Revision > envelope.Limits.MaximumPlanRevision || plannerAttempt > envelope.Limits.MaximumPlannerAttemptsPerRevision || delegation.Decisions >= envelope.Limits.MaximumTotalCaptainDecisions || delegation.ReplanSpent > envelope.Limits.ReplanBudget || ValidateCaptainPlanPolicy(envelope.PlanRules, proposal.plan, prior) != nil {
		return refuse("CAPTAIN_PLAN_POLICY_REFUSED")
	}
	if err := validateCaptainReleaseLineageWithEngine(engine, engine.manifest, proposal, snapshot, delegation); err != nil {
		return refuse("CAPTAIN_RELEASE_LINEAGE_REFUSED")
	}
	classAllowed := false
	for _, rule := range envelope.DecisionRules {
		classAllowed = classAllowed || rule.DecisionClass == class
	}
	if !classAllowed {
		return refuse("CAPTAIN_DECISION_CLASS_REFUSED")
	}
	before := captainReviewBefore(proposal, delegation)
	_, target, err := captureProposalRefs(engine.repository, engine.manifest)
	if err != nil || target.Head.String() != proposal.authority.TargetHead {
		return refuse("CAPTAIN_TARGET_DRIFT")
	}
	workspace, err := engine.workspaces.OpenSnapshot(target.Head)
	if err != nil {
		return "", true, runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	workID := driverWorkIdentity(engine.manifest.digest, "", driver.CaptainPlanReview, metadata.Revision, before)
	invocationScope := ""
	if plannerAttempt > 1 {
		invocationScope = strings.TrimPrefix(workID, "sha256:")[:12]
	}
	submission, runErr := s.dispatchRoleWithScope(ctx, engine, workspace, driver.RoleCaptain, "", driver.CaptainPlanReview, metadata.Revision, before, owner, invocationScope)
	closeErr := workspace.Close()
	if runErr != nil {
		if IsCode(runErr, "CAPTAIN_ATTEMPTS_EXHAUSTED") || IsCode(runErr, "EFFECT_PARKED") || IsCode(runErr, "RECOVERY_UNCERTAIN") {
			return refuse("CAPTAIN_ATTEMPTS_EXHAUSTED")
		}
		return "", true, runErr
	}
	if closeErr != nil {
		return "", true, runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	decisionSnapshot, snapshotErr := s.journal.Snapshot(ctx, owner.RunID)
	if snapshotErr != nil {
		return "", true, runtimeFail("JOURNAL_READ_FAILED", snapshotErr)
	}
	if testCaptainCrashCut == "sealed_submission" {
		return "", true, runtimeFail("TEST_CAPTAIN_CRASH_CUT", nil)
	}
	captainAttempt, attemptErr := captainDispatchAttemptForSubmission(decisionSnapshot, workID, submission)
	if attemptErr != nil {
		return "", true, attemptErr
	}
	command, err := newCaptainDecisionCommand(engine.manifest, proposal, delegation, submission, workID, captainAttempt)
	if err != nil {
		return "", true, err
	}
	// The public command service reopens and revalidates all current Git and
	// journal facts. Release this engine's workspace-owner lock while that
	// independent admission runs, then reacquire the same durable run workspace.
	if err := engine.workspaces.Close(); err != nil {
		return "", true, runtimeFail("WORKSPACE_CLEANUP_FAILED", err)
	}
	engine.workspaces = nil
	_, decisionErr := s.CaptainDecide(ctx, command)
	workspaces, reopenErr := gitx.NewRunWorkspaces(engine.repository, engine.manifest.value.RunID, engine.manifest.value.GitIdentity)
	if reopenErr != nil {
		return "", true, runtimeFail("WORKSPACE_UNAVAILABLE", reopenErr)
	}
	engine.workspaces = workspaces
	if decisionErr != nil {
		return "", true, decisionErr
	}
	if command.Outcome == "revise" {
		if err := s.processCaptainPlannerContinuations(ctx, engine, owner); err != nil {
			return "", true, err
		}
		freshSnapshot, err := s.journal.Snapshot(ctx, owner.RunID)
		if err != nil {
			return "", true, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		_, proposals, err := loadRunSnapshot(freshSnapshot, owner.RunID)
		if err != nil {
			return "", true, err
		}
		state, stateErr := baton.ReadState(engine.git, engine.manifest.value.Release, engine.inertness)
		replacement, found, _, err := selectPlanProposal(engine, freshSnapshot, proposals, state, stateErr)
		if err != nil || !found || replacement.replayKey == proposal.replayKey || replacement.plan.Digest() == proposal.plan.Digest() {
			return refuse("CAPTAIN_REPLAN_RECOVERY_REFUSED")
		}
		return s.reviewDelegatedProposal(ctx, engine, owner, replacement, freshSnapshot)
	}
	return command.Outcome, true, nil
}

func (s *Service) refreshPlanProposal(
	ctx context.Context,
	engine *engine,
	owner journal.OwnerLease,
	state baton.State,
	stateErr error,
) error {
	if stateErr == nil {
		return s.proposeRevision(ctx, engine, owner, state)
	}
	if baton.ErrorCode(stateErr) == "REF_NOT_FOUND" {
		return s.proposePlan(ctx, engine, owner, nil, 1)
	}
	return runtimeFail("BATON_UNAVAILABLE", stateErr)
}

func proposalPendingAuthorityCurrent(
	repository *gitx.Repository,
	manifest admittedManifest,
	proposal admittedPlanProposal,
	state baton.State,
	stateErr error,
) (bool, error) {
	release, target, err := captureProposalRefs(repository, manifest)
	if err != nil {
		return false, err
	}
	return proposalMatchesPendingAuthority(
		proposal, release, target, state, stateErr) ||
		proposalMatchesAppliedPlan(proposal, state, stateErr), nil
}

func withReleaseAssemblyAuthority(
	engine *engine,
	release string,
	releaseHead string,
	action func() (baton.ActionResult, error),
) (baton.ActionResult, error, error) {
	head, err := gitx.ParseOID(
		engine.repository.ObjectFormat(),
		releaseHead,
	)
	if err != nil {
		return baton.ActionResult{},
			runtimeFail("INVALID_AUTHORITY_STATE", err), nil
	}
	lease, err := engine.workspaces.OpenReleaseAssembly(release, head)
	if err != nil {
		return baton.ActionResult{},
			runtimeFail("WORKSPACE_UNAVAILABLE", err), nil
	}
	result, actionErr := action()
	closeErr := lease.Close()
	if closeErr != nil {
		closeErr = runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	return result, actionErr, closeErr
}

func withReleaseAssembly(
	engine *engine,
	state baton.State,
	action func() (baton.ActionResult, error),
) (baton.ActionResult, error, error) {
	return withReleaseAssemblyAuthority(
		engine,
		state.Release,
		state.Refs.Release.Head,
		action,
	)
}

func (s *Service) prepareAssembly(ctx context.Context, engine *engine, owner journal.OwnerLease, state baton.State) error {
	input := baton.PrepareAssemblyInput{Release: state.Release,
		Summary: "Compose all exact passed track candidates.",
		Detail:  []byte("Deterministic engine-owned plan-ordered composition.")}
	before := workIdentity(state.Plan.OID, state.Refs.Release.Head, state.Refs.Target.Head,
		state.Assembly.Outcome, state.Assembly.InputPins)
	binds := state.Plan.ApprovalOID
	if state.Assembly.CurrentReceipt != nil {
		binds = state.Assembly.CurrentReceipt.OID
	}
	authority := stateActionAuthority(
		state, state.Refs.Release.Ref, state.Refs.Release.Head,
		before, binds, "", 0)
	var cleanupErr error
	action := func() (baton.ActionResult, error) {
		result, actionErr, closeErr := withReleaseAssembly(
			engine,
			state,
			func() (baton.ActionResult, error) {
				return engine.actions.PrepareAssembly(input)
			},
		)
		cleanupErr = errors.Join(cleanupErr, closeErr)
		return result, actionErr
	}
	result, err := s.runAction(ctx, engine, owner, workIdentity(before, "prepare"),
		"baton.prepare_assembly", marshalActionCommand(engine.manifest.value.GitIdentity, authority, input), action)
	err = errors.Join(err, cleanupErr)
	if err == nil && result.Direct {
		return runtimeFail("DISTINCT_ASSEMBLY_VERIFICATION_REQUIRED", nil)
	}
	return err
}

func (s *Service) verifyAssembly(ctx context.Context, engine *engine, owner journal.OwnerLease, state baton.State) error {
	candidate := *state.Assembly.Candidate.Receipt.Candidate
	oid, err := gitx.ParseOID(engine.repository.ObjectFormat(), candidate)
	if err != nil {
		return runtimeFail("INVALID_ASSEMBLY_CANDIDATE", err)
	}
	key := gitx.TrackKey{Release: state.Release, Track: state.Tracks[0].ID}
	workspace, err := engine.workspaces.OpenCandidate(key, gitx.AssemblyVerifierView, oid)
	if err != nil {
		return runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	before := workIdentity(state.Plan.OID, state.Refs.Release.Head, state.Refs.Target.Head, candidate)
	submission, runErr := s.dispatchRole(ctx, engine, workspace, driver.RoleVerifier, "",
		driver.AssemblyVerification, state.Plan.Metadata.Revision, before, owner)
	closeErr := workspace.Close()
	if runErr != nil {
		return runErr
	}
	if closeErr != nil {
		return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	fresh, err := baton.ReadState(engine.git, state.Release, engine.inertness)
	if err != nil {
		return runtimeFail("BATON_UNAVAILABLE", err)
	}
	if fresh.Assembly.Candidate == nil ||
		fresh.Assembly.Candidate.Receipt.Candidate == nil ||
		*fresh.Assembly.Candidate.Receipt.Candidate != candidate ||
		workIdentity(fresh.Plan.OID, fresh.Refs.Release.Head, fresh.Refs.Target.Head, candidate) != before {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	state = fresh
	checks, err := exactBytes(submission.Checks)
	if err != nil {
		return err
	}
	input := baton.AppendReceiptInput{Release: state.Release, Role: "verifier",
		Result: string(submission.Decision.Outcome), Summary: submission.Summary,
		Detail: []byte(submission.Detail), Candidate: candidate, CheckResults: checks}
	action := func() (baton.ActionResult, error) { return engine.actions.AppendReceipt(input) }
	authority := stateActionAuthority(
		state, state.Refs.Release.Ref, state.Refs.Release.Head,
		before, state.Assembly.Candidate.OID, candidate, 0)
	_, err = s.runAction(ctx, engine, owner, workIdentity(before, "assembly_verdict"),
		"baton.assembly_verdict", marshalActionCommand(engine.manifest.value.GitIdentity, authority, input), action)
	return err
}

func (s *Service) mergeAssembly(ctx context.Context, engine *engine, owner journal.OwnerLease, state baton.State) error {
	input := baton.MergePassedCandidateInput{Release: state.Release,
		Summary: "Merge the exact independently verified assembly candidate.",
		Detail:  []byte("Deterministic Merge; no model dispatch.")}
	before := workIdentity(state.Plan.OID, state.Refs.Release.Head, state.Refs.Target.Head,
		state.Assembly.Pass.OID)
	candidate := ""
	if state.Assembly.Pass.Receipt.Candidate != nil {
		candidate = *state.Assembly.Pass.Receipt.Candidate
	}
	authority := stateActionAuthority(
		state, state.Refs.Release.Ref, state.Refs.Release.Head,
		before, state.Assembly.Pass.OID, candidate, 0)
	var cleanupErr error
	action := func() (baton.ActionResult, error) {
		result, actionErr, closeErr := withReleaseAssembly(
			engine,
			state,
			func() (baton.ActionResult, error) {
				return engine.actions.MergePassedCandidate(input)
			},
		)
		cleanupErr = errors.Join(cleanupErr, closeErr)
		return result, actionErr
	}
	_, err := s.runAction(ctx, engine, owner, workIdentity(before, "merge"),
		"baton.merge", marshalActionCommand(engine.manifest.value.GitIdentity, authority, input), action)
	return errors.Join(err, cleanupErr)
}

func (s *Service) driveOwned(
	ctx context.Context,
	runID string,
	owner journal.OwnerLease,
) (status RunStatus, resultErr error) {
	defer func() {
		if closeErr := s.closeRunContinuations(runID); closeErr != nil {
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	for {
		status, resultErr = s.driveOwnedCycle(
			ctx,
			runID,
			owner,
		)
		if IsCode(resultErr, "EFFECT_PARKED") {
			status, resultErr = s.Status(
				context.Background(),
				runID,
			)
		}
		if resultErr != nil {
			releaseErr := s.journal.ReleaseOwner(
				context.Background(),
				owner,
				s.now().UTC(),
			)
			resultErr = errors.Join(resultErr, releaseErr)
			return status, resultErr
		}
		if s.beforeOwnerRelease != nil {
			s.beforeOwnerRelease()
		}
		released, releaseErr := s.journal.ReleaseOwnerIfIdle(
			context.Background(),
			owner,
			s.now().UTC(),
		)
		if releaseErr != nil {
			fallbackErr := s.journal.ReleaseOwner(
				context.Background(),
				owner,
				s.now().UTC(),
			)
			return RunStatus{}, errors.Join(
				runtimeFail("OWNER_UNAVAILABLE", releaseErr),
				fallbackErr,
			)
		}
		if released {
			return status, nil
		}
	}
}

func (s *Service) driveOwnedCycle(ctx context.Context, runID string, owner journal.OwnerLease) (
	status RunStatus,
	resultErr error,
) {
	ownedCtx, cancelWork := context.WithCancel(ctx)
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan error, 1)
	go s.watchOwner(watchCtx, owner, cancelWork, watchDone)
	defer func() {
		cancelWork()
		stopWatch()
		watchErr := <-watchDone
		if resultErr == nil && watchErr != nil && !journal.IsCode(watchErr, "OWNER_FENCED") {
			resultErr = runtimeFail("OWNER_LOST", watchErr)
		}
	}()
	manifest, _, err := s.loadRun(ownedCtx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	if manifest.legacyVersion != "" {
		return RunStatus{}, runtimeFail("MIGRATION_REQUIRED", nil)
	}
	engine, err := s.openEngine(manifest)
	if err != nil {
		return RunStatus{}, err
	}
	defer engine.Close()
	control, err := s.journal.ControlProjection(
		context.WithoutCancel(ownedCtx),
		runID,
	)
	if err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if control.Desired != "running" {
		return s.Status(context.Background(), runID)
	}
	if err := s.processCaptainPlannerContinuations(ownedCtx, engine, owner); err != nil {
		return RunStatus{}, err
	}
	if err := s.recoverClaimedEffects(ownedCtx, engine, owner); err != nil {
		return RunStatus{}, err
	}
	recoveryPending, err := s.driverRecoveryPending(ownedCtx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	if recoveryPending {
		return s.Status(context.Background(), runID)
	}
	snapshot, err := s.journal.Snapshot(ownedCtx, runID)
	if err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	loadedManifest, proposals, err := loadRunSnapshot(snapshot, runID)
	if err != nil || loadedManifest.digest != manifest.digest {
		return RunStatus{}, runtimeFail("RUN_BINDING_MISMATCH", err)
	}
	state, stateErr := baton.ReadState(
		engine.git, manifest.value.Release, engine.inertness)
	if err := validateInstallEffectPrecedence(engine, snapshot); err != nil {
		return RunStatus{}, err
	}
	proposal, found, installed, err := selectPlanProposal(
		engine, snapshot, proposals, state, stateErr)
	if err != nil {
		return RunStatus{}, err
	}
	var currentProposal *admittedPlanProposal
	if found {
		currentProposal = &proposal
	}
	authorityDigest, err := effectivePlanAuthority(
		manifest, snapshot, currentProposal)
	if err != nil {
		return RunStatus{}, err
	}
	if found && stateErr == nil && proposal.plan.Digest() != state.Plan.Digest {
		return RunStatus{}, runtimeFail("PLAN_AUTHORITY_CONFLICT", nil)
	}
	if authorityDigest == "" {
		if !found && stateErr == nil {
			return s.Status(context.Background(), runID)
		}
		if !found && baton.ErrorCode(stateErr) != "REF_NOT_FOUND" {
			return RunStatus{}, runtimeFail("BATON_UNAVAILABLE", stateErr)
		}
	}
	if !found && stateErr != nil && authorityDigest == "" {
		if err := s.refreshPlanProposal(
			ownedCtx, engine, owner, state, stateErr); err != nil {
			return RunStatus{}, err
		}
		snapshot, err = s.journal.Snapshot(ownedCtx, runID)
		if err != nil {
			return RunStatus{}, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		_, proposals, err = loadRunSnapshot(snapshot, runID)
		if err != nil {
			return RunStatus{}, err
		}
		proposal, found, installed, err = selectPlanProposal(engine, snapshot, proposals, state, stateErr)
		if err != nil || !found {
			return RunStatus{}, runtimeFail("INVALID_PLAN", err)
		}
		currentProposal = &proposal
	}
	if found && authorityDigest == "" {
		outcome, handled, reviewErr := s.reviewDelegatedProposal(ownedCtx, engine, owner, proposal, snapshot)
		if reviewErr != nil {
			return RunStatus{}, reviewErr
		}
		if !handled {
			return s.Status(context.Background(), runID)
		}
		if handled {
			if outcome != "proceed" {
				return s.Status(context.Background(), runID)
			}
			snapshot, err = s.journal.Snapshot(ownedCtx, runID)
			if err != nil {
				return RunStatus{}, runtimeFail("JOURNAL_READ_FAILED", err)
			}
			_, proposals, err = loadRunSnapshot(snapshot, runID)
			if err != nil {
				return RunStatus{}, err
			}
			state, stateErr = baton.ReadState(
				engine.git, manifest.value.Release, engine.inertness)
			proposal, found, installed, err = selectPlanProposal(
				engine, snapshot, proposals, state, stateErr)
			if err != nil || !found {
				return RunStatus{}, runtimeFail("AUTHORITY_CONFLICT", err)
			}
			currentProposal = &proposal
			authorityDigest, err = effectivePlanAuthority(manifest, snapshot, &proposal)
			if err != nil || authorityDigest != proposal.plan.Digest() {
				return RunStatus{}, runtimeFail("AUTHORITY_CONFLICT", err)
			}
		}
	}
	if proposalActivationRecorded(
		proposal, found, installed, state, stateErr,
		authorityDigest, snapshot,
	) {
		runErr := s.driveLoop(ownedCtx, engine, owner, false)
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			return RunStatus{}, runErr
		}
		return s.Status(context.Background(), runID)
	}
	if proposalAwaitsExactAuthority(
		proposal, found, state, stateErr, authorityDigest,
	) {
		return s.Status(context.Background(), runID)
	}
	if stateErr == nil {
		adopted, adoptErr := validateSavedPlanAdoption(
			engine, state, authorityDigest)
		if adoptErr != nil {
			return RunStatus{}, adoptErr
		}
		if adopted {
			runErr := s.driveLoop(ownedCtx, engine, owner, false)
			if runErr != nil && !errors.Is(runErr, context.Canceled) {
				return RunStatus{}, runErr
			}
			return s.Status(context.Background(), runID)
		}
	}
	if !found || authorityDigest == "" || proposal.plan.Digest() != authorityDigest {
		return RunStatus{}, runtimeFail("AUTHORITY_CONFLICT", nil)
	}
	installWork := proposalInstallWork(proposal)
	{
		current, err := proposalPendingAuthorityCurrent(
			engine.repository, manifest, proposal, state, stateErr)
		if err != nil {
			return RunStatus{}, err
		}
		if !current {
			if err := s.refreshPlanProposal(
				ownedCtx, engine, owner, state, stateErr); err != nil {
				return RunStatus{}, err
			}
			return s.Status(context.Background(), runID)
		}
		freshState, freshStateErr := baton.ReadState(
			engine.git, manifest.value.Release, engine.inertness)
		current, err = proposalPendingAuthorityCurrent(
			engine.repository, manifest, proposal, freshState, freshStateErr)
		if err != nil {
			return RunStatus{}, err
		}
		if !current {
			if err := s.refreshPlanProposal(
				ownedCtx, engine, owner, freshState, freshStateErr); err != nil {
				return RunStatus{}, err
			}
			return s.Status(context.Background(), runID)
		}
		state, stateErr = freshState, freshStateErr
		admission := approvalAdmission{
			planBytes:  proposal.plan.Bytes(),
			planDigest: proposal.plan.Digest(),
			reference:  proposal.plan.Metadata().ApprovalRef,
		}
		installInput := installActionInput{
			PlanBytes: admission.planBytes, PlanDigest: admission.planDigest,
			Reference: admission.reference,
		}
		authority := batonActionAuthority{
			Release:     manifest.value.Release,
			Plan:        proposal.authority.PriorPlan,
			ReleaseHead: proposal.authority.ReleaseHead,
			TargetRef:   proposal.authority.TargetRef,
			TargetHead:  proposal.authority.TargetHead,
			OwnerRef:    proposal.authority.ReleaseRef,
			OwnerHead:   proposal.authority.ReleaseHead,
			Before:      installWork,
		}
		action := func() (baton.ActionResult, error) {
			return engine.installer.install(
				admission, proposal.authority.TargetHead,
			)
		}
		if _, err := s.runAction(
			ownedCtx, engine, owner, installWork, "baton.install",
			marshalActionCommand(engine.manifest.value.GitIdentity, authority, installInput), action,
		); err != nil {
			return RunStatus{}, err
		}
		state, stateErr = baton.ReadState(
			engine.git, manifest.value.Release, engine.inertness)
	}
	if stateErr != nil {
		return RunStatus{}, runtimeFail("BATON_UNAVAILABLE", stateErr)
	}
	runErr := s.driveLoop(ownedCtx, engine, owner, false)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return RunStatus{}, runErr
	}
	return s.Status(context.Background(), runID)
}

func (s *Service) processCaptainPlannerContinuations(ctx context.Context, engine *engine, owner journal.OwnerLease) error {
	snapshot, err := s.journal.Snapshot(ctx, owner.RunID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "planner.continue" || (effect.State != journal.Pending && effect.State != journal.Claimed) {
			continue
		}
		stored, ok := commands[effect.ReplayKey]
		if !ok || stored.Kind != "planner_continuation" {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		var continuation CaptainPlannerContinuationCommand
		if json.Unmarshal(stored.Payload, &continuation) != nil || continuation.RunID != owner.RunID {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		delegation, err := currentCaptainDelegation(snapshot)
		if err != nil || !delegation.Active || delegation.Digest != continuation.EnvelopeDigest || delegation.Epoch != continuation.EnvelopeEpoch {
			return runtimeFail("CAPTAIN_DECISION_STALE", err)
		}
		if effect.State == journal.Pending {
			if testCaptainCrashCut == "before_planner_continuation" {
				return runtimeFail("TEST_CAPTAIN_CRASH_CUT", nil)
			}
			claim, claimErr := s.journal.ClaimOwned(ctx, owner, effect.ID, s.now().UTC(), effectLease)
			if claimErr != nil {
				return runtimeFail("CAPTAIN_DECISION_RECOVERY_PENDING", claimErr)
			}
			effect.CurrentClaim = claim.Token
		}
		state, stateErr := baton.ReadState(engine.git, engine.manifest.value.Release, engine.inertness)
		var current *baton.State
		if stateErr == nil {
			current = &state
		} else if baton.ErrorCode(stateErr) != "REF_NOT_FOUND" {
			return runtimeFail("BATON_UNAVAILABLE", stateErr)
		}
		if err := s.proposePlanAttempt(ctx, engine, owner, current, continuation.PlanRevision, continuation.PlannerAttempt, continuation.DecisionReplayKey); err != nil {
			return err
		}
		if err := s.journal.CompleteOwned(ctx, owner, journal.Completion{RunID: owner.RunID, EffectID: effect.ID, Token: effect.CurrentClaim, State: journal.Succeeded, Result: []byte("scheduled"), EventKind: "planner_replan_scheduled", EventBody: []byte(continuation.SupersededProposalReplayKey), At: s.now().UTC()}); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
	}
	return nil
}

func (s *Service) watchOwner(ctx context.Context, owner journal.OwnerLease, cancel context.CancelFunc, done chan<- error) {
	interval := ownerDuration() / 6
	if interval > 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done <- nil
			return
		case <-ticker.C:
			projection, err := s.journal.ControlProjection(ctx, owner.RunID)
			if err != nil {
				if ctx.Err() != nil {
					done <- nil
					return
				}
				cancel()
				done <- err
				return
			}
			if projection.Desired != "running" {
				cancel()
			}
			if time.Until(owner.ExpiresAt) < ownerDuration()*2/3 {
				owner, err = s.journal.RenewOwner(ctx, owner, s.now().UTC(), ownerDuration())
				if err != nil {
					if ctx.Err() != nil {
						done <- nil
						return
					}
					cancel()
					done <- err
					return
				}
			}
		}
	}
}
