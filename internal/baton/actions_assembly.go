package baton

import (
	"fmt"
)

func (a *Actions) PrepareAssembly(input PrepareAssemblyInput) (ActionResult, error) {
	release, err := identity(input.Release, "release")
	if err != nil {
		return ActionResult{}, recordWrap("INVALID_ACTION_INPUT", "invalid release", err)
	}
	summary, err := actionText(input.Summary, "summary", maxSummaryBytes)
	if err != nil {
		return ActionResult{}, err
	}
	detail, err := actionDetail(input.Detail)
	if err != nil {
		return ActionResult{}, err
	}
	var checkResults []byte
	if input.CheckResults != nil {
		if len(input.CheckResults) > MaxEvidenceBytes {
			return ActionResult{}, recordFail("INVALID_ACTION_INPUT", fmt.Sprintf("checkResults must be at most %d bytes", MaxEvidenceBytes))
		}
		checkResults = append([]byte(nil), input.CheckResults...)
	}
	state, err := a.stateFor(release)
	if err != nil {
		return ActionResult{}, err
	}
	if state.Plan.TargetStale {
		return ActionResult{}, recordFail(
			"TARGET_MOVED",
			fmt.Sprintf(
				"the target moved from %s to %s; revise and reapprove the plan",
				valueOrEmpty(state.Plan.Approval.Receipt.Target), state.Refs.Target.Head,
			),
		)
	}
	for _, slice := range state.Slices {
		if slice.Pass == nil {
			return ActionResult{}, recordFail("SLICE_PASS_REQUIRED", slice.Location.Slice.ID+" has no current PASS")
		}
	}

	type trackCandidate struct {
		ID          string
		Candidate   string
		ProductTree string
	}
	trackCandidates := make([]trackCandidate, 0, len(state.Tracks))
	for _, track := range state.Tracks {
		final := track.Slices[len(track.Slices)-1]
		if final.Pass == nil || final.Pass.Receipt.Candidate == nil ||
			final.Pass.Receipt.ProductTree == nil {
			return ActionResult{}, recordFail("SLICE_PASS_REQUIRED", "track "+track.ID+" has no final PASS")
		}
		for _, slice := range track.Slices {
			contained, err := a.repository.isAncestor(
				*slice.Pass.Receipt.Candidate, *final.Pass.Receipt.Candidate,
			)
			if err != nil {
				return ActionResult{}, err
			}
			if !contained {
				return ActionResult{}, recordFail("INVALID_TRACK_TOPOLOGY", "track "+track.ID+" candidates are not one serial lineage")
			}
		}
		trackCandidates = append(trackCandidates, trackCandidate{
			ID: track.ID, Candidate: *final.Pass.Receipt.Candidate,
			ProductTree: *final.Pass.Receipt.ProductTree,
		})
	}
	inputs := make(map[string]string, len(trackCandidates))
	for _, track := range trackCandidates {
		inputs[track.ID] = track.ProductTree
	}
	target := state.Refs.Target.Head
	if len(trackCandidates) == 1 {
		direct, err := a.repository.isAncestor(target, trackCandidates[0].Candidate)
		if err != nil {
			return ActionResult{}, err
		}
		if direct {
			result := actionResult("prepareAssembly", false)
			result.Release, result.Direct, result.Candidate = release, true, trackCandidates[0].Candidate
			result.Inputs = cloneInputs(inputs)
			result.ReceiptCommit = state.Tracks[0].Slices[len(state.Tracks[0].Slices)-1].Pass.OID
			return result, nil
		}
	}
	existing := state.Assembly.Candidate
	if existing != nil && existing.Receipt.Target != nil &&
		*existing.Receipt.Target == target && inputsEqual(existing.Receipt.Inputs, inputs) {
		result := actionResult("prepareAssembly", false)
		result.Release, result.Direct, result.Candidate = release, false, *existing.Receipt.Candidate
		result.Inputs, result.ReceiptCommit = cloneInputs(inputs), existing.OID
		return result, nil
	}

	candidate := target
	components := []string{state.Refs.Release.Head}
	for _, track := range trackCandidates {
		components = append(components, track.Candidate)
	}
	for _, component := range components {
		if component == candidate {
			continue
		}
		contained, err := a.repository.isAncestor(component, candidate)
		if err != nil {
			return ActionResult{}, err
		}
		if contained {
			continue
		}
		prepared, err := a.repository.prepareComposition(state.Refs.Target.Ref, candidate, component)
		if err != nil {
			return ActionResult{}, err
		}
		candidate = prepared.Result
	}
	productTree, err := a.repository.productTree(candidate)
	if err != nil {
		return ActionResult{}, err
	}
	checks := ""
	if input.CheckResults != nil {
		checks = DigestBytes(checkResults)
	} else {
		canonical, err := canonicalJSON(stringMapAny(inputs))
		if err != nil {
			return ActionResult{}, err
		}
		checks = DigestBytes(canonical)
	}
	binds := state.Plan.ApprovalOID
	if state.Assembly.CurrentReceipt != nil {
		binds = state.Assembly.CurrentReceipt.OID
	}
	base := target
	receipt := Receipt{
		Version: ReceiptVersion, Release: release, Role: "implementer", Result: "candidate",
		Plan: state.Plan.OID, Binds: binds, Detail: DigestBytes(nil), Summary: summary,
		Target: &target, Base: &base, Candidate: &candidate, ProductTree: &productTree,
		Inputs: cloneInputs(inputs), Checks: &checks,
	}
	message, err := RenderReceiptCommit("baton("+release+"): assembly candidate", detail, receipt)
	if err != nil {
		return ActionResult{}, err
	}
	prepared, err := a.repository.prepareMetadata(candidate, message)
	if err != nil {
		return ActionResult{}, err
	}
	snapshot := sortedCaptured([]CapturedRef{state.Refs.Release, state.Refs.Target})
	if err := a.repository.updateRefs(snapshot, []refOperation{
		{Kind: "verify", Ref: state.Refs.Target.Ref, ExpectedHead: target},
		{
			Kind: "update", Ref: state.Refs.Release.Ref,
			NewHead: prepared.Commit, ExpectedHead: state.Refs.Release.Head,
		},
	}); err != nil {
		return ActionResult{}, err
	}
	parsed, err := ParseReceiptCommitMessage(message)
	if err != nil {
		return ActionResult{}, err
	}
	parsedReceipt := parsed.Receipt.Clone()
	result := actionResult("prepareAssembly", true)
	result.Release, result.Direct, result.Candidate = release, false, candidate
	result.Inputs, result.ReceiptCommit, result.Receipt =
		cloneInputs(inputs), prepared.Commit, &parsedReceipt
	return result, nil
}

func (a *Actions) MergePassedCandidate(input MergePassedCandidateInput) (ActionResult, error) {
	release, err := identity(input.Release, "release")
	if err != nil {
		return ActionResult{}, recordWrap("INVALID_ACTION_INPUT", "invalid release", err)
	}
	summary, err := actionText(input.Summary, "summary", maxSummaryBytes)
	if err != nil {
		return ActionResult{}, err
	}
	detail, err := actionDetail(input.Detail)
	if err != nil {
		return ActionResult{}, err
	}
	state, err := a.stateFor(release)
	if err != nil {
		return ActionResult{}, err
	}
	if state.Assembly.Outcome == "merged" {
		receipt := state.Assembly.CurrentReceipt.Receipt.Clone()
		result := actionResult("mergePassedCandidate", false)
		result.Release = release
		result.Candidate = *state.Assembly.Candidate.Receipt.Candidate
		result.Target = state.Refs.Target.Ref
		result.ResultCommit = state.Assembly.ResultCommit
		result.ReceiptCommit, result.Receipt = state.Assembly.CurrentReceipt.OID, &receipt
		return result, nil
	}
	if state.Plan.TargetStale {
		return ActionResult{}, recordFail(
			"TARGET_MOVED",
			fmt.Sprintf(
				"the target moved from %s to %s; revise and reapprove the plan",
				valueOrEmpty(state.Plan.Approval.Receipt.Target), state.Refs.Target.Head,
			),
		)
	}
	passed := state.Assembly.Pass
	if passed == nil && len(state.Tracks) == 1 {
		final := state.Tracks[0].Slices[len(state.Tracks[0].Slices)-1].Pass
		if final != nil && final.Receipt.Candidate != nil {
			direct, err := a.repository.isAncestor(state.Refs.Target.Head, *final.Receipt.Candidate)
			if err != nil {
				return ActionResult{}, err
			}
			if direct {
				passed = final
			}
		}
	}
	if passed == nil || passed.Receipt.Candidate == nil {
		return ActionResult{}, recordFail("ASSEMBLY_PASS_REQUIRED", "the current exact candidate has no applicable PASS")
	}
	candidate := *passed.Receipt.Candidate
	prepared, err := a.repository.prepareComposition(
		state.Refs.Target.Ref, state.Refs.Target.Head, candidate,
	)
	if err != nil {
		return ActionResult{}, err
	}
	productTree, err := a.repository.productTree(prepared.Result)
	if err != nil {
		return ActionResult{}, err
	}
	target, resultCommit := state.Refs.Target.Head, prepared.Result
	receipt := Receipt{
		Version: ReceiptVersion, Release: release, Role: "merge", Result: "merged",
		Plan: state.Plan.OID, Binds: passed.OID, Detail: DigestBytes(nil),
		Summary: summary, Target: &target, Candidate: &candidate,
		ProductTree: &productTree, ResultCommit: &resultCommit,
	}
	message, err := RenderReceiptCommit("baton("+release+"): merge passed candidate", detail, receipt)
	if err != nil {
		return ActionResult{}, err
	}
	preparedReceipt, err := a.repository.prepareMetadata(state.Refs.Release.Head, message)
	if err != nil {
		return ActionResult{}, err
	}
	snapshot := sortedCaptured([]CapturedRef{state.Refs.Release, state.Refs.Target})
	if err := a.repository.updateRefs(snapshot, []refOperation{
		{
			Kind: "update", Ref: state.Refs.Target.Ref,
			NewHead: prepared.Result, ExpectedHead: state.Refs.Target.Head,
		},
		{
			Kind: "update", Ref: state.Refs.Release.Ref,
			NewHead: preparedReceipt.Commit, ExpectedHead: state.Refs.Release.Head,
		},
	}); err != nil {
		return ActionResult{}, err
	}
	parsed, err := ParseReceiptCommitMessage(message)
	if err != nil {
		return ActionResult{}, err
	}
	parsedReceipt := parsed.Receipt.Clone()
	result := actionResult("mergePassedCandidate", true)
	result.Release, result.Candidate, result.Target = release, candidate, state.Refs.Target.Ref
	result.ResultCommit, result.ReceiptCommit, result.Receipt =
		prepared.Result, preparedReceipt.Commit, &parsedReceipt
	return result, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringMapAny(value map[string]string) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
