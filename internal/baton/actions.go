package baton

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxSummaryBytes = 280

type Actions struct {
	repository *repository
}

func NewActions(gitRepository GitRepository, resolver InertnessResolver) (*Actions, error) {
	repository, err := newRepository(gitRepository.repository(), resolver)
	if err != nil {
		return nil, err
	}
	return &Actions{repository: repository}, nil
}

type RecordPlanRevisionInput struct {
	PlanBytes []byte
	Summary   string
	Detail    []byte
}

type AppendReceiptInput struct {
	Release      string
	Slice        string
	Role         string
	Result       string
	Summary      string
	Detail       []byte
	Base         string
	Candidate    string
	CheckResults []byte
}

type PrepareAssemblyInput struct {
	Release      string
	Summary      string
	Detail       []byte
	CheckResults []byte
}

type MergePassedCandidateInput struct {
	Release string
	Summary string
	Detail  []byte
}

type RetirementResult struct {
	Slice         string
	ReceiptCommit string
	Receipt       Receipt
}

type ActionResult struct {
	Kind          string
	Action        string
	Changed       bool
	Release       string
	Slice         string
	Revision      int64
	Plan          string
	Ref           string
	Head          string
	Target        string
	Direct        bool
	Candidate     string
	Inputs        map[string]string
	ResultCommit  string
	ReceiptCommit string
	Receipt       *Receipt
	Retirements   []RetirementResult
}

func actionResult(action string, changed bool) ActionResult {
	return ActionResult{Kind: "baton.action-result/v2", Action: action, Changed: changed}
}

func actionText(value, label string, maximum int) (string, error) {
	if !utf8.ValidString(value) || len([]byte(value)) > maximum || strings.TrimSpace(value) == "" {
		return "", recordFail("INVALID_ACTION_INPUT", fmt.Sprintf("%s must be a non-empty UTF-8 string of at most %d bytes", label, maximum))
	}
	return value, nil
}

func actionDetail(value []byte) ([]byte, error) {
	if len(value) > MaxDetailBytes {
		return nil, recordFail("INVALID_ACTION_INPUT", fmt.Sprintf("detail must be at most %d bytes", MaxDetailBytes))
	}
	return append([]byte(nil), value...), nil
}

func actionEvidence(value []byte, required bool, label string) ([]byte, error) {
	if required && value == nil {
		return nil, recordFail("INVALID_ACTION_INPUT", label+" is required")
	}
	if !required && value != nil {
		return nil, recordFail("INVALID_ACTION_INPUT", label+" is not accepted")
	}
	if len(value) > MaxEvidenceBytes {
		return nil, recordFail("INVALID_ACTION_INPUT", fmt.Sprintf("%s must be at most %d bytes", label, MaxEvidenceBytes))
	}
	return append([]byte(nil), value...), nil
}

func (a *Actions) stateFor(release string) (State, error) {
	if a == nil || a.repository == nil {
		return State{}, recordFail("INVALID_ACTION_INPUT", "actions are not admitted")
	}
	if _, err := identity(release, "release"); err != nil {
		return State{}, recordWrap("INVALID_ACTION_INPUT", "invalid release", err)
	}
	return readState(a.repository, release, "")
}

func (a *Actions) RecordPlanRevision(input RecordPlanRevisionInput) (ActionResult, error) {
	if a == nil || a.repository == nil {
		return ActionResult{}, recordFail("INVALID_ACTION_INPUT", "actions are not admitted")
	}
	parsed, err := ParsePlan(input.PlanBytes)
	if err != nil {
		return ActionResult{}, err
	}
	summary, err := actionText(input.Summary, "summary", maxSummaryBytes)
	if err != nil {
		return ActionResult{}, err
	}
	detail, err := actionDetail(input.Detail)
	if err != nil {
		return ActionResult{}, err
	}
	metadata := parsed.Metadata()
	release, targetRef, ownerRef := metadata.Release, metadata.TargetRef, releaseRef(metadata.Release)
	captured, err := a.repository.capture([]string{targetRef, ownerRef})
	if err != nil {
		return ActionResult{}, err
	}
	byRef := captureByRef(captured)
	target, prior := byRef[targetRef], byRef[ownerRef]
	if !directCommit(target) {
		if absentRef(target) {
			return ActionResult{}, recordFail("TARGET_NOT_FOUND", "target "+targetRef+" does not exist")
		}
		return ActionResult{}, recordFail("INVALID_HEAD_OBJECT", "target is not one direct commit")
	}
	if !directCommit(prior) && !absentRef(prior) {
		return ActionResult{}, recordFail("INVALID_HEAD_OBJECT", "release ref is not absent or one direct commit")
	}

	parent := target.Head
	var previous *State
	preparedTrackResets := make(map[string]string)
	if absentRef(prior) {
		if metadata.Revision != 1 || metadata.PreviousPlan != nil {
			return ActionResult{}, recordFail("INVALID_PLAN_REVISION", "a new release must begin at plan revision 1")
		}
		existing, err := a.repository.file(target.Head, planPath(release))
		if err != nil {
			return ActionResult{}, err
		}
		if existing.Present {
			return ActionResult{}, recordFail("RELEASE_ALREADY_RECORDED", "target already contains release "+release)
		}
	} else {
		state, err := a.stateFor(release)
		if err != nil {
			return ActionResult{}, err
		}
		previous = &state
		if state.Refs.Release.Head != prior.Head ||
			state.Refs.Target.Head != target.Head {
			return ActionResult{}, recordFail(
				"REF_SNAPSHOT_UNSTABLE",
				"release or target ref moved while validating the plan revision",
			)
		}
		current := state.Plan.History[len(state.Plan.History)-1].Plan
		if bytes.Equal(current.Bytes(), parsed.Bytes()) {
			approval := state.Plan.Approval
			if approval.Receipt.Target == nil {
				return ActionResult{}, recordFail("APPROVAL_MISSING", "current plan approval has no target")
			}
			contained, err := a.repository.isAncestor(
				*approval.Receipt.Target,
				target.Head,
			)
			if err != nil {
				return ActionResult{}, err
			}
			if !contained {
				return ActionResult{}, recordFail(
					"TARGET_DIVERGED",
					"the target no longer contains this plan's approved starting point; reconcile its history before continuing",
				)
			}
			receipt := approval.Receipt.Clone()
			result := actionResult("recordPlanRevision", false)
			result.Release, result.Revision, result.Plan = release, metadata.Revision, state.Plan.OID
			result.Ref, result.Head, result.Target = ownerRef, prior.Head, *approval.Receipt.Target
			result.ReceiptCommit, result.Receipt = approval.OID, &receipt
			return result, nil
		}
		for _, track := range state.Tracks {
			if track.Head == "" || track.Head == track.AuthorityHead {
				continue
			}
			resettable := false
			for _, slice := range track.Slices {
				if slice.Pass != nil {
					continue
				}
				if len(slice.Location.Slice.Consumes) == 0 ||
					slice.Stage != "design" ||
					slice.Status != "ready" ||
					slice.NextRole != "implementer" ||
					slice.Candidate != nil ||
					slice.PreparationSeed != track.AuthorityHead ||
					slice.PreparedBase != track.Head {
					break
				}
				exactBase, exactErr := preparedStateTrackBase(
					a.repository,
					state,
					slice,
				)
				if exactErr != nil {
					return ActionResult{}, exactErr
				}
				resettable = exactBase == track.Head
				break
			}
			if !resettable {
				return ActionResult{}, recordFail(
					"CHANGED_OWNER_HEAD",
					"track "+track.ID+" has unrecorded implementation work",
				)
			}
			preparedTrackResets[track.Ref] = track.AuthorityHead
		}
		if err := assertPlanRevision(current, parsed, state.Plan.OID); err != nil {
			return ActionResult{}, err
		}
		everPlanned := make(map[string]bool)
		for _, historical := range state.Plan.History {
			for id := range locations(historical.Plan) {
				everPlanned[id] = true
			}
		}
		active := locations(current)
		for id := range locations(parsed) {
			if everPlanned[id] {
				if _, retained := active[id]; !retained {
					return ActionResult{}, recordFail("INVALID_RETIREMENT", "retired slice "+id+" cannot be re-added")
				}
			}
		}
		parent = prior.Head
	}

	currentTracks := make(map[string]CapturedRef)
	names := []string{targetRef, ownerRef}
	if previous != nil {
		for _, track := range previous.Refs.Tracks {
			currentTracks[track.Ref] = track.CapturedRef
			names = append(names, track.Ref)
		}
	}
	for _, track := range metadata.Tracks {
		names = append(names, trackRef(release, track.ID))
	}
	captured, err = a.repository.capture(unique(names))
	if err != nil {
		return ActionResult{}, err
	}
	byRef = captureByRef(captured)
	if byRef[targetRef] != target || byRef[ownerRef] != prior {
		return ActionResult{}, recordFail(
			"REF_SNAPSHOT_UNSTABLE",
			"release or target ref moved before plan preparation",
		)
	}
	for ref, expected := range currentTracks {
		if byRef[ref] != expected {
			return ActionResult{}, recordFail(
				"REF_SNAPSHOT_UNSTABLE",
				"track authority moved before plan preparation",
			)
		}
	}
	for _, track := range metadata.Tracks {
		ref := trackRef(release, track.ID)
		if _, current := currentTracks[ref]; !current && !absentRef(byRef[ref]) {
			return ActionResult{}, recordFail(
				"AMBIGUOUS_AUTHORITY",
				"new track "+track.ID+" already has authority history",
			)
		}
	}

	preparedPlan, err := a.repository.prepareRecord(
		parent,
		fmt.Sprintf("baton(%s): plan revision %d", release, metadata.Revision),
		map[string][]byte{planPath(release): parsed.Bytes()},
	)
	if err != nil {
		return ActionResult{}, err
	}
	planFile, err := a.repository.file(preparedPlan.Commit, planPath(release))
	if err != nil {
		return ActionResult{}, err
	}
	if !planFile.Present || planFile.Object == "" {
		return ActionResult{}, recordFail("PLAN_NOT_FOUND", "prepared plan blob could not be resolved")
	}
	targetOID, planCommitOID := target.Head, preparedPlan.Commit
	approvalReceipt := Receipt{
		Version: ReceiptVersion, Release: release, Role: "planner", Result: "approved",
		Plan: planFile.Object, Binds: planCommitOID, Detail: DigestBytes(nil),
		Summary: summary, Target: &targetOID,
	}
	approvalMessage, err := RenderReceiptCommit("baton("+release+"): approve plan", detail, approvalReceipt)
	if err != nil {
		return ActionResult{}, err
	}
	preparedApproval, err := a.repository.prepareMetadata(preparedPlan.Commit, approvalMessage)
	if err != nil {
		return ActionResult{}, err
	}
	parsedApproval, err := ParseReceiptCommitMessage(approvalMessage)
	if err != nil {
		return ActionResult{}, err
	}
	approvalEntry := ReceiptEntry{
		OID: preparedApproval.Commit, Parent: preparedPlan.Commit,
		Tree: preparedApproval.Tree, Subject: parsedApproval.Subject,
		Detail: parsedApproval.Detail, Receipt: parsedApproval.Receipt,
	}
	nextHead := preparedApproval.Commit
	var retirements []RetirementResult
	if previous != nil {
		retained := make(map[string]bool)
		for _, track := range metadata.Tracks {
			for _, slice := range track.Slices {
				retained[slice.ID] = true
			}
		}
		for _, removed := range previous.Slices {
			sliceID := removed.Location.Slice.ID
			if retained[sliceID] {
				continue
			}
			attempt := removed.History.MaximumAttempt + 1
			contract := previous.Plan.Metadata.Contracts[sliceID]
			binds := preparedApproval.Commit
			retirement := Receipt{
				Version: ReceiptVersion, Release: release, Slice: &sliceID,
				Role: "planner", Result: "retired", Attempt: &attempt,
				Plan: planFile.Object, Contract: &contract, Binds: binds,
				Detail:  DigestBytes(nil),
				Summary: fmt.Sprintf("Retired %s under approved plan revision %d.", sliceID, metadata.Revision),
			}
			message, err := RenderReceiptCommit("baton("+release+"/"+sliceID+"): retire slice", nil, retirement)
			if err != nil {
				return ActionResult{}, err
			}
			prepared, err := a.repository.prepareMetadata(nextHead, message)
			if err != nil {
				return ActionResult{}, err
			}
			parsedMessage, err := ParseReceiptCommitMessage(message)
			if err != nil {
				return ActionResult{}, err
			}
			retirements = append(retirements, RetirementResult{
				Slice: sliceID, ReceiptCommit: prepared.Commit,
				Receipt: parsedMessage.Receipt,
			})
			nextHead = prepared.Commit
		}
	}
	operations := []refOperation{{Kind: "verify", Ref: targetRef, ExpectedHead: target.Head}}
	for _, capturedRef := range captured {
		if capturedRef.Ref != targetRef && capturedRef.Ref != ownerRef {
			if authority, reset := preparedTrackResets[capturedRef.Ref]; reset {
				resetHead := authority
				if resetHead == prior.Head {
					resetHead = nextHead
				}
				operations = append(operations, refOperation{
					Kind: "update", Ref: capturedRef.Ref,
					NewHead: resetHead, ExpectedHead: capturedRef.Head,
				})
				continue
			}
			operations = append(operations, refOperation{
				Kind: "verify", Ref: capturedRef.Ref, ExpectedHead: capturedRef.Head,
			})
		}
	}
	if absentRef(prior) {
		operations = append(operations, refOperation{Kind: "create", Ref: ownerRef, NewHead: nextHead})
	} else {
		operations = append(operations, refOperation{
			Kind: "update", Ref: ownerRef, NewHead: nextHead, ExpectedHead: prior.Head,
		})
	}
	if err := a.repository.updateRefs(captured, operations); err != nil {
		return ActionResult{}, err
	}
	receipt := approvalEntry.Receipt.Clone()
	result := actionResult("recordPlanRevision", true)
	result.Release, result.Revision, result.Plan = release, metadata.Revision, planFile.Object
	result.Ref, result.Head, result.Target = ownerRef, nextHead, target.Head
	result.ReceiptCommit, result.Receipt = preparedApproval.Commit, &receipt
	result.Retirements = retirements
	return result, nil
}

func assertPlanRevision(previous, next Plan, previousObject string) error {
	before, after := previous.Metadata(), next.Metadata()
	if after.Revision != before.Revision+1 {
		return recordFail("INVALID_PLAN_REVISION", "plan revision must advance by exactly one")
	}
	if after.PreviousPlan == nil || *after.PreviousPlan != previousObject {
		return recordFail("INVALID_PLAN_REVISION", "plan previous_plan must bind the current plan blob")
	}
	for _, item := range []struct {
		name          string
		before, after string
	}{
		{"release", before.Release, after.Release},
		{"repository", before.Repository, after.Repository},
		{"target_ref", before.TargetRef, after.TargetRef},
	} {
		if item.before != item.after {
			return recordFail("REPLACED_RELEASE_AUTHORITY", "plan revision cannot change "+item.name+"; create a new release")
		}
	}
	if after.ApprovalRef == before.ApprovalRef {
		return recordFail("STALE_APPROVAL", "plan revision requires a new protected approval reference")
	}
	beforeLocations, afterLocations := locations(previous), locations(next)
	for id, beforeLocation := range beforeLocations {
		if afterLocation, retained := afterLocations[id]; retained &&
			beforeLocation.Track.ID != afterLocation.Track.ID {
			return recordFail(
				"REPLACED_SLICE_AUTHORITY",
				"plan revision cannot move retained slice "+id+" between tracks",
			)
		}
	}
	return nil
}

func (a *Actions) AppendReceipt(input AppendReceiptInput) (ActionResult, error) {
	return a.appendReceipt(input, nil)
}

func (a *Actions) appendReceipt(
	input AppendReceiptInput,
	beforeUpdateRefs func(),
) (ActionResult, error) {
	release, err := identity(input.Release, "release")
	if err != nil {
		return ActionResult{}, recordWrap("INVALID_ACTION_INPUT", "invalid release", err)
	}
	role, err := actionText(input.Role, "role", 16)
	if err != nil {
		return ActionResult{}, err
	}
	resultName, err := actionText(input.Result, "result", 16)
	if err != nil {
		return ActionResult{}, err
	}
	summary, err := actionText(input.Summary, "summary", maxSummaryBytes)
	if err != nil {
		return ActionResult{}, err
	}
	detail, err := actionDetail(input.Detail)
	if err != nil {
		return ActionResult{}, err
	}
	sliceID := ""
	if input.Slice != "" {
		sliceID, err = identity(input.Slice, "slice")
		if err != nil {
			return ActionResult{}, recordWrap("INVALID_ACTION_INPUT", "invalid slice", err)
		}
	}
	evidenceRequired := (role == "implementer" && resultName == "candidate") || role == "verifier"
	checkBytes, err := actionEvidence(input.CheckResults, evidenceRequired, "checkResults")
	if err != nil {
		return ActionResult{}, err
	}
	checks := ""
	if evidenceRequired {
		checks = DigestBytes(checkBytes)
	}
	if input.Candidate != "" &&
		validateObjectForFormat(a.repository.objectFormat(), input.Candidate, "candidate") != nil {
		return ActionResult{}, recordFail("INVALID_ACTION_INPUT", "candidate must be one full repository-format object identity")
	}
	if input.Base != "" &&
		validateObjectForFormat(a.repository.objectFormat(), input.Base, "base") != nil {
		return ActionResult{}, recordFail("INVALID_ACTION_INPUT", "base must be one full repository-format object identity")
	}
	state, err := a.stateFor(release)
	if err != nil {
		return ActionResult{}, err
	}

	var ownerRef, ownerHead, parent string
	var receipt Receipt
	var current *ReceiptEntry
	var snapshot []CapturedRef
	var consumedSources []CapturedRef
	if sliceID != "" {
		slice, ok := state.Slice(sliceID)
		if !ok {
			return ActionResult{}, recordFail("SLICE_NOT_FOUND", "plan has no current slice "+sliceID)
		}
		track, _ := state.Track(slice.Location.Track.ID)
		ownerRef, ownerHead, current = track.Ref, track.Head, slice.CurrentReceipt
		if input.Base != "" &&
			!(role == "implementer" && resultName == "candidate") {
			return ActionResult{}, recordFail(
				"INVALID_ACTION_INPUT",
				role+"/"+resultName+" does not accept base",
			)
		}
		actionEligible :=
			(role == "implementer" && resultName == "designed" &&
				slice.NextRole == "implementer" && slice.Stage == "design") ||
				(role == "captain" &&
					(resultName == "proceed" ||
						resultName == "revise" ||
						resultName == "escalate") &&
					slice.NextRole == "captain") ||
				(role == "implementer" && resultName == "candidate" &&
					slice.NextRole == "implementer" &&
					slice.Stage == "implement") ||
				(role == "verifier" &&
					(resultName == "pass" ||
						resultName == "fail" ||
						resultName == "blocked") &&
					slice.NextRole == "verifier")
		if !actionEligible &&
			exactRetry(
				current,
				role,
				resultName,
				summary,
				detail,
				input.Base,
				input.Candidate,
				checks,
			) {
			return appendReceiptResult(false, ownerRef, *current), nil
		}
		if err := requireSlicePrerequisites(state, slice); err != nil {
			return ActionResult{}, err
		}
		contract := state.Plan.Metadata.Contracts[sliceID]
		common := Receipt{
			Version: ReceiptVersion, Release: release, Slice: &sliceID,
			Plan: state.Plan.OID, Contract: &contract, Detail: DigestBytes(nil),
			Summary: summary,
		}
		switch {
		case role == "implementer" && resultName == "designed":
			if slice.NextRole != "implementer" || slice.Stage != "design" {
				return ActionResult{}, recordFail("ROLE_NOT_ELIGIBLE", sliceID+" does not currently need an Implementer design")
			}
			attempt, binds := slice.Attempt, current.OID
			common.Role, common.Result, common.Attempt, common.Binds = role, resultName, &attempt, binds
			receipt = common
			parent = ownerHead
			if parent == "" {
				parent = state.Refs.Release.Head
			}
			exactBase, err := preparedStateTrackBase(
				a.repository,
				state,
				slice,
			)
			if err != nil {
				return ActionResult{}, err
			}
			if parent != exactBase {
				return ActionResult{}, recordFail(
					"TRACK_BASE_NOT_PREPARED",
					ownerRef+
						" does not equal the exact current approved-target and consumed-input base",
				)
			}
			consuming := len(slice.Location.Slice.Consumes) > 0
			if consuming {
				if slice.PreparationSeed == "" {
					return ActionResult{}, recordFail(
						"CHANGED_OWNER_HEAD",
						"consuming design has no preparation seed",
					)
				}
				base := slice.PreparationSeed
				common.Base = &base
				common.Inputs = cloneInputs(slice.InputPins)
				receipt = common
			}
			if ownerHead != "" && ownerHead != current.OID &&
				current.Receipt.Role != "planner" &&
				ownerHead != exactBase {
				return ActionResult{}, recordFail("CHANGED_OWNER_HEAD", ownerRef+" changed after its authoritative receipt")
			}
		case role == "captain" && (resultName == "proceed" || resultName == "revise" || resultName == "escalate"):
			if slice.NextRole != "captain" {
				return ActionResult{}, recordFail("ROLE_NOT_ELIGIBLE", sliceID+" does not currently need Captain review")
			}
			attempt, binds := *current.Receipt.Attempt, current.OID
			common.Role, common.Result, common.Attempt, common.Binds = role, resultName, &attempt, binds
			receipt, parent = common, ownerHead
			if ownerHead != current.OID {
				return ActionResult{}, recordFail("CHANGED_OWNER_HEAD", ownerRef+" changed after its design receipt")
			}
		case role == "implementer" && resultName == "candidate":
			if slice.NextRole != "implementer" || slice.Stage != "implement" {
				return ActionResult{}, recordFail("ROLE_NOT_ELIGIBLE", sliceID+" does not currently need an implementation candidate")
			}
			if input.Candidate == "" || ownerHead != input.Candidate {
				return ActionResult{}, recordFail("CHANGED_CANDIDATE", "candidate must be the exact captured track head")
			}
			exactBase, err := preparedStateTrackBase(
				a.repository,
				state,
				slice,
			)
			if err != nil {
				return ActionResult{}, err
			}
			if len(slice.Location.Slice.Consumes) > 0 {
				if input.Base == "" || input.Base != exactBase {
					return ActionResult{}, recordFail(
						"CHANGED_CANDIDATE",
						"consuming candidate must bind the exact prepared base",
					)
				}
				linear, err := linearOneParentAncestry(
					a.repository,
					input.Base,
					input.Candidate,
				)
				if err != nil {
					return ActionResult{}, err
				}
				if !linear {
					return ActionResult{}, recordFail(
						"CHANGED_CANDIDATE",
						"consuming candidate must be linear one-parent work from its prepared base",
					)
				}
				for _, consumed := range slice.ConsumedInputs {
					for _, ancestor := range []string{
						consumed.Candidate,
						consumed.CandidateReceipt,
						consumed.PassReceipt,
					} {
						contained, err := a.repository.isAncestor(
							ancestor,
							input.Candidate,
						)
						if err != nil {
							return ActionResult{}, err
						}
						if !contained {
							return ActionResult{}, recordFail(
								"CHANGED_CANDIDATE",
								"candidate omits consumed authority "+ancestor,
							)
						}
					}
				}
			} else {
				if input.Base != "" {
					return ActionResult{}, recordFail(
						"INVALID_ACTION_INPUT",
						"non-consuming candidate cannot record a prepared base",
					)
				}
				containsBase, err := a.repository.isAncestor(
					exactBase,
					input.Candidate,
				)
				if err != nil {
					return ActionResult{}, err
				}
				if !containsBase {
					return ActionResult{}, recordFail(
						"CHANGED_CANDIDATE",
						"non-consuming candidate omits the exact prepared base",
					)
				}
			}
			if err := a.repository.assertCandidateRecordRootUnchanged(
				exactBase,
				input.Candidate,
			); err != nil {
				return ActionResult{}, err
			}
			productTree, err := a.repository.productTree(input.Candidate)
			if err != nil {
				return ActionResult{}, err
			}
			attempt, binds, candidate, checksDigest := slice.Attempt, current.OID, input.Candidate, checks
			common.Role, common.Result, common.Attempt, common.Binds = role, resultName, &attempt, binds
			common.Candidate, common.ProductTree, common.Inputs, common.Checks =
				&candidate, &productTree, cloneInputs(slice.InputPins), &checksDigest
			if input.Base != "" {
				base := input.Base
				common.Base = &base
			}
			receipt, parent = common, input.Candidate
		case role == "verifier" && (resultName == "pass" || resultName == "fail" || resultName == "blocked"):
			if slice.NextRole != "verifier" {
				return ActionResult{}, recordFail("ROLE_NOT_ELIGIBLE", sliceID+" does not currently need verification")
			}
			evidence := current.Receipt
			if evidence.Candidate == nil || input.Candidate == "" || input.Candidate != *evidence.Candidate {
				return ActionResult{}, recordFail("CHANGED_CANDIDATE", "Verifier must bind the exact current candidate")
			}
			exactBase, err := preparedStateTrackBase(
				a.repository,
				state,
				slice,
			)
			if err != nil {
				return ActionResult{}, err
			}
			if err := a.repository.assertCandidateRecordRootUnchanged(
				exactBase,
				*evidence.Candidate,
			); err != nil {
				return ActionResult{}, err
			}
			attempt, binds, candidate, productTree, checksDigest :=
				*evidence.Attempt, current.OID, *evidence.Candidate, *evidence.ProductTree, checks
			common.Role, common.Result, common.Attempt, common.Binds = role, resultName, &attempt, binds
			common.Candidate, common.ProductTree, common.Inputs, common.Checks =
				&candidate, &productTree, cloneInputs(evidence.Inputs), &checksDigest
			receipt, parent = common, ownerHead
			if ownerHead != current.OID {
				return ActionResult{}, recordFail("CHANGED_OWNER_HEAD", ownerRef+" changed after its candidate receipt")
			}
		default:
			return ActionResult{}, recordFail("INVALID_ACTION_INPUT", "unsupported slice receipt "+role+"/"+resultName)
		}
		trackRefCapture := capturedTrackRef(state, track.ID)
		snapshotValues := []CapturedRef{state.Refs.Release, trackRefCapture}
		if role == "implementer" &&
			(resultName == "designed" || resultName == "candidate") {
			seen := make(map[string]bool)
			for _, consumed := range slice.ConsumedInputs {
				if consumed.SourceRef == ownerRef ||
					seen[consumed.SourceRef] {
					continue
				}
				seen[consumed.SourceRef] = true
				var source CapturedRef
				for _, candidate := range state.Refs.Tracks {
					if candidate.Ref == consumed.SourceRef {
						source = candidate.CapturedRef
						break
					}
				}
				if source.Ref == "" || source.Head != consumed.SourceHead {
					return ActionResult{}, recordFail(
						"AUTHORITY_MOVED",
						"consumed source "+consumed.SourceRef+" changed",
					)
				}
				consumedSources = append(consumedSources, source)
				snapshotValues = append(snapshotValues, source)
			}
		}
		snapshot = sortedCaptured(snapshotValues)
	} else {
		ownerRef, ownerHead, current = state.Refs.Release.Ref, state.Refs.Release.Head, state.Assembly.CurrentReceipt
		if exactRetry(current, role, resultName, summary, detail, input.Base, input.Candidate, checks) {
			return appendReceiptResult(false, ownerRef, *current), nil
		}
		if role != "verifier" || (resultName != "pass" && resultName != "fail" && resultName != "blocked") ||
			state.Assembly.NextRole != "verifier" {
			return ActionResult{}, recordFail("ROLE_NOT_ELIGIBLE", "the assembly does not currently need a Verifier verdict")
		}
		evidence := state.Assembly.Candidate
		if evidence == nil || evidence.Receipt.Candidate == nil ||
			input.Candidate == "" || *evidence.Receipt.Candidate != input.Candidate {
			return ActionResult{}, recordFail("CHANGED_CANDIDATE", "Verifier must bind the exact current assembly candidate")
		}
		candidate, productTree, checksDigest := input.Candidate, *evidence.Receipt.ProductTree, checks
		receipt = Receipt{
			Version: ReceiptVersion, Release: release, Role: role, Result: resultName,
			Plan: state.Plan.OID, Binds: evidence.OID, Detail: DigestBytes(nil),
			Summary: summary, Candidate: &candidate, ProductTree: &productTree,
			Inputs: cloneInputs(evidence.Receipt.Inputs), Checks: &checksDigest,
		}
		parent = ownerHead
		if current == nil || ownerHead != current.OID {
			return ActionResult{}, recordFail("CHANGED_OWNER_HEAD", ownerRef+" changed after its assembly candidate receipt")
		}
		snapshot = sortedCaptured([]CapturedRef{state.Refs.Release})
	}
	if err := requireTargetLineage(a.repository, state); err != nil {
		return ActionResult{}, err
	}

	subject := fmt.Sprintf("baton(%s", release)
	if sliceID != "" {
		subject += "/" + sliceID
	}
	subject += "): " + role + " " + resultName
	message, err := RenderReceiptCommit(subject, detail, receipt)
	if err != nil {
		return ActionResult{}, err
	}
	prepared, err := a.repository.prepareMetadata(parent, message)
	if err != nil {
		return ActionResult{}, err
	}
	operations := make([]refOperation, 0, 3)
	if ownerRef != state.Refs.Release.Ref {
		operations = append(operations, refOperation{
			Kind: "verify", Ref: state.Refs.Release.Ref,
			ExpectedHead: state.Refs.Release.Head,
		})
	}
	for _, source := range consumedSources {
		operations = append(operations, refOperation{
			Kind:         "verify",
			Ref:          source.Ref,
			ExpectedHead: source.Head,
		})
	}
	if ownerHead == "" {
		operations = append(operations, refOperation{Kind: "create", Ref: ownerRef, NewHead: prepared.Commit})
	} else {
		operations = append(operations, refOperation{
			Kind: "update", Ref: ownerRef, NewHead: prepared.Commit, ExpectedHead: ownerHead,
		})
	}
	if beforeUpdateRefs != nil {
		beforeUpdateRefs()
	}
	if err := a.repository.updateRefs(snapshot, operations); err != nil {
		return ActionResult{}, err
	}
	parsed, err := ParseReceiptCommitMessage(message)
	if err != nil {
		return ActionResult{}, err
	}
	entry := ReceiptEntry{
		OID: prepared.Commit, Parent: parent, Tree: prepared.Tree,
		Subject: parsed.Subject, Detail: parsed.Detail, Receipt: parsed.Receipt,
	}
	return appendReceiptResult(true, ownerRef, entry), nil
}

func exactRetry(
	entry *ReceiptEntry,
	role, result, summary string,
	detail []byte,
	base, candidate, checks string,
) bool {
	if entry == nil {
		return false
	}
	receipt := entry.Receipt
	if receipt.Role != role || receipt.Result != result || receipt.Summary != summary ||
		!bytes.Equal(entry.Detail, detail) {
		return false
	}
	evidenceRequired := (role == "implementer" && result == "candidate") ||
		role == "verifier"
	if evidenceRequired && (candidate == "" || checks == "") {
		return false
	}
	if role == "implementer" && result == "candidate" &&
		(base != "" || receipt.Base != nil) &&
		(base == "" || receipt.Base == nil || *receipt.Base != base) {
		return false
	}
	if candidate != "" &&
		(receipt.Candidate == nil || *receipt.Candidate != candidate) {
		return false
	}
	if checks != "" &&
		(receipt.Checks == nil || *receipt.Checks != checks) {
		return false
	}
	return true
}

func appendReceiptResult(changed bool, ref string, entry ReceiptEntry) ActionResult {
	receipt := entry.Receipt.Clone()
	result := actionResult("appendReceipt", changed)
	result.Release, result.Ref, result.ReceiptCommit, result.Receipt =
		receipt.Release, ref, entry.OID, &receipt
	if receipt.Slice != nil {
		result.Slice = *receipt.Slice
	}
	return result
}

func requireSlicePrerequisites(state State, slice *SliceState) error {
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		return recordFail("TRACK_NOT_FOUND", "plan has no track "+slice.Location.Track.ID)
	}
	position := -1
	for index, item := range track.Slices {
		if item.Location.Slice.ID == slice.Location.Slice.ID {
			position = index
			break
		}
	}
	required := make(map[string]bool)
	for _, trackID := range track.DependsOn {
		dependency, ok := state.Track(trackID)
		if !ok {
			return recordFail("TRACK_NOT_FOUND", "plan has no track "+trackID)
		}
		for _, item := range dependency.Slices {
			required[item.Location.Slice.ID] = true
		}
	}
	for _, prior := range track.Slices[:position] {
		required[prior.Location.Slice.ID] = true
	}
	for _, dependency := range append(
		append([]string(nil), slice.Location.Slice.DependsOn...),
		slice.Location.Slice.Consumes...,
	) {
		required[dependency] = true
	}
	for dependency := range required {
		stateSlice, ok := state.Slice(dependency)
		if !ok || stateSlice.Pass == nil {
			return recordFail("DEPENDENCIES_NOT_READY", slice.Location.Slice.ID+" is waiting for "+dependency+" PASS")
		}
	}
	return nil
}

func cloneInputs(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func captureByRef(values []CapturedRef) map[string]CapturedRef {
	result := make(map[string]CapturedRef, len(values))
	for _, value := range values {
		result[value.Ref] = value
	}
	return result
}

func sortedCaptured(values []CapturedRef) []CapturedRef {
	result := append([]CapturedRef(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Ref < result[j].Ref })
	return result
}

func capturedTrackRef(state State, trackID string) CapturedRef {
	for _, value := range state.Refs.Tracks {
		if value.ID == trackID {
			return value.CapturedRef
		}
	}
	return CapturedRef{}
}
