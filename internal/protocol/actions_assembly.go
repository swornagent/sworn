package protocol

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
		if input.CheckResultsFor != nil {
			return ActionResult{}, recordFail("INVALID_ACTION_INPUT", "checkResults and checkResultsFor are mutually exclusive")
		}
		if len(input.CheckResults) > MaxEvidenceBytes {
			return ActionResult{}, recordFail("INVALID_ACTION_INPUT", fmt.Sprintf("checkResults must be at most %d bytes", MaxEvidenceBytes))
		}
		checkResults = append([]byte(nil), input.CheckResults...)
	}
	state, err := a.stateFor(release)
	if err != nil {
		return ActionResult{}, err
	}
	if err := requireTargetLineage(a.repository, state); err != nil {
		return ActionResult{}, err
	}
	classification, err := classifyStateAssembly(a.repository, state)
	if err != nil {
		return ActionResult{}, err
	}

	target := state.Refs.Target.Head
	inputs := classification.Inputs
	if classification.Direct {
		candidate := *classification.DirectPass.Receipt.Candidate
		result := actionResult("prepareAssembly", false)
		result.Release, result.Direct, result.Candidate = release, true, candidate
		result.Inputs, result.ReceiptCommit =
			cloneInputs(inputs), classification.DirectPass.OID
		return result, nil
	}
	existing := state.Assembly.Candidate
	if existing != nil && existing.Receipt.Target != nil &&
		*existing.Receipt.Target == target && inputsEqual(existing.Receipt.Inputs, inputs) {
		// The reused candidate's receipt already binds its checks digest;
		// the engine still records this run's own evidence for it.
		if _, err := composedCheckResults(input, *existing.Receipt.Candidate); err != nil {
			return ActionResult{}, err
		}
		result := actionResult("prepareAssembly", false)
		result.Release, result.Direct, result.Candidate = release, false, *existing.Receipt.Candidate
		result.Inputs, result.ReceiptCommit = cloneInputs(inputs), existing.OID
		return result, nil
	}

	candidate, err := prepareClassifiedAssembly(
		a.repository, state.Refs.Target.Ref, target,
		state.Refs.Release.Head, classification,
		state.productBases.track,
	)
	if err != nil {
		return ActionResult{}, err
	}
	productTree, err := a.repository.productTree(candidate)
	if err != nil {
		return ActionResult{}, err
	}
	if composed, err := composedCheckResults(input, candidate); err != nil {
		return ActionResult{}, err
	} else if composed != nil {
		checkResults = composed
	}
	checks := ""
	if checkResults != nil {
		checks = DigestBytes(checkResults)
	} else {
		canonical, err := canonicalJSON(stringMapAny(inputs))
		if err != nil {
			return ActionResult{}, err
		}
		checks = DigestBytes(canonical)
	}
	binds := state.Refs.Release.Head
	base := target
	receipt := Receipt{
		Version: ReceiptVersion, Release: release, Role: "implementer", Result: "candidate",
		Plan: state.Plan.OID, Binds: binds, Detail: DigestBytes(nil), Summary: summary,
		Target: &target, Base: &base, Candidate: &candidate, ProductTree: &productTree,
		Inputs: cloneInputs(inputs), Checks: &checks,
	}
	message, err := RenderReceiptCommit(a.repository.commitPrefix()+"("+release+"): assembly candidate", detail, receipt)
	if err != nil {
		return ActionResult{}, err
	}
	prepared, err := a.repository.prepareMetadata(candidate, message)
	if err != nil {
		return ActionResult{}, err
	}
	snapshot, operations, err := assemblyRefCAS(state, classification, []refOperation{
		{Kind: "verify", Ref: state.Refs.Target.Ref, ExpectedHead: target},
		{
			Kind: "update", Ref: state.Refs.Release.Ref,
			NewHead: prepared.Commit, ExpectedHead: state.Refs.Release.Head,
		},
	})
	if err != nil {
		return ActionResult{}, err
	}
	if err := a.repository.updateRefs(snapshot, operations); err != nil {
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

// composedCheckResults asks the engine's CheckResultsFor hook for the
// check-results manifest of the exact composed candidate and admits it: it
// must be a well-formed sworn.check-results/v1 manifest within the evidence
// cap that binds this candidate whenever it asserts one, so the receipt's
// checks digest can never cover evidence about a different tree. A nil
// manifest means no host checks are declared and keeps the input-pin digest.
func composedCheckResults(input PrepareAssemblyInput, candidate string) ([]byte, error) {
	if input.CheckResultsFor == nil {
		return nil, nil
	}
	manifest, err := input.CheckResultsFor(candidate)
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, nil
	}
	if len(manifest) > MaxEvidenceBytes {
		return nil, recordFail("INVALID_ACTION_INPUT", fmt.Sprintf("checkResults must be at most %d bytes", MaxEvidenceBytes))
	}
	parsed, err := ParseCheckResults(manifest)
	if err != nil {
		return nil, err
	}
	if parsed.Slice != "" || (parsed.Candidate != "" && parsed.Candidate != candidate) {
		return nil, recordFail("STALE_BINDING", "assembly check results do not bind the composed candidate")
	}
	return append([]byte(nil), manifest...), nil
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
	if err := requireTargetLineage(a.repository, state); err != nil {
		return ActionResult{}, err
	}
	classification, err := classifyStateAssembly(a.repository, state)
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
	passed := applicableAssemblyPass(state, classification)
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
	message, err := RenderReceiptCommit(a.repository.commitPrefix()+"("+release+"): merge passed candidate", detail, receipt)
	if err != nil {
		return ActionResult{}, err
	}
	preparedReceipt, err := a.repository.prepareMetadata(state.Refs.Release.Head, message)
	if err != nil {
		return ActionResult{}, err
	}
	snapshot, operations, err := assemblyRefCAS(state, classification, []refOperation{
		{
			Kind: "update", Ref: state.Refs.Target.Ref,
			NewHead: prepared.Result, ExpectedHead: state.Refs.Target.Head,
		},
		{
			Kind: "update", Ref: state.Refs.Release.Ref,
			NewHead: preparedReceipt.Commit, ExpectedHead: state.Refs.Release.Head,
		},
	})
	if err != nil {
		return ActionResult{}, err
	}
	if err := a.repository.updateRefs(snapshot, operations); err != nil {
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

func assemblyRefCAS(
	state State,
	classification assemblyClassification,
	operations []refOperation,
) ([]CapturedRef, []refOperation, error) {
	snapshot := []CapturedRef{state.Refs.Release, state.Refs.Target}
	resultOperations := append([]refOperation(nil), operations...)
	for _, classified := range classification.TrackCandidates {
		captured := capturedTrackRef(state, classified.ID)
		if !directCommit(captured) {
			return nil, nil, recordFail(
				"INVALID_TRACK_TOPOLOGY",
				"track "+classified.ID+" has no exact captured head",
			)
		}
		snapshot = append(snapshot, captured)
		resultOperations = append(resultOperations, refOperation{
			Kind: "verify", Ref: captured.Ref, ExpectedHead: captured.Head,
		})
	}
	return sortedCaptured(snapshot), resultOperations, nil
}

func requireTargetLineage(repository *repository, state State) error {
	if state.Plan.Approval.Receipt.Target == nil {
		return recordFail("APPROVAL_MISSING", "current plan approval has no target")
	}
	contained, err := repository.isAncestor(
		*state.Plan.Approval.Receipt.Target,
		state.Refs.Target.Head,
	)
	if err != nil {
		return err
	}
	if !contained {
		return recordFail(
			"TARGET_DIVERGED",
			"the target no longer contains the approved starting point; reconcile its history before continuing",
		)
	}
	return nil
}

func classifyStateAssembly(
	repository *repository,
	state State,
) (assemblyClassification, error) {
	if len(state.Plan.History) == 0 {
		return assemblyClassification{}, recordFail(
			"INVALID_TRACK_TOPOLOGY",
			"release has no approved plan history",
		)
	}
	currentOID := state.Plan.History[len(state.Plan.History)-1].OID
	current := state.Plan.History[len(state.Plan.History)-1].Plan
	topology := topologyFromPlanHistory(state.Plan.History)
	evidence := assemblyEvidenceFromTracks(state.Tracks)
	planByOID := planByOIDFromHistory(state.Plan.History)
	acceptanceIdentities, err := resolveAcceptanceIdentities(
		repository, state.Release, state.Refs.Release.Head, planByOID,
	)
	if err != nil {
		return assemblyClassification{}, err
	}
	classification, err := classifyAssembly(
		repository,
		current,
		currentOID,
		planByOID,
		acceptanceIdentities,
		topology,
		evidence,
	)
	if err != nil {
		return assemblyClassification{}, err
	}
	target := state.Refs.Target.Head
	releaseHead := state.Refs.Release.Head
	if state.Assembly.Outcome == "merged" &&
		state.Assembly.CurrentReceipt != nil &&
		state.Assembly.Pass != nil &&
		state.Assembly.Pass.Receipt.Slice != nil {
		releaseHead = state.Assembly.CurrentReceipt.Parent
	}
	return withDirectAssemblyReuse(
		repository, current, topology, evidence,
		target, releaseHead, classification,
		state.productBases.track,
	)
}

func applicableAssemblyPass(
	state State,
	classification assemblyClassification,
) *ReceiptEntry {
	if classification.Direct {
		return classification.DirectPass
	}
	pass, candidate := state.Assembly.Pass, state.Assembly.Candidate
	if pass == nil || candidate == nil ||
		pass.Receipt.Role != "verifier" ||
		pass.Receipt.Result != "pass" ||
		pass.Receipt.Slice != nil ||
		pass.Receipt.Plan != state.Plan.OID ||
		pass.Receipt.Binds != candidate.OID ||
		candidate.Receipt.Role != "implementer" ||
		candidate.Receipt.Result != "candidate" ||
		candidate.Receipt.Slice != nil ||
		candidate.Receipt.Plan != state.Plan.OID ||
		candidate.Receipt.Target == nil ||
		*candidate.Receipt.Target != state.Refs.Target.Head ||
		!sameCandidate(pass.Receipt, candidate.Receipt) ||
		!inputsEqual(candidate.Receipt.Inputs, classification.Inputs) {
		return nil
	}
	return pass
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
