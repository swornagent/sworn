package baton

import (
	"fmt"
	"slices"
	"strings"
)

type CandidateValidation struct {
	Base, Candidate, Tree, ProductTree, AuthorityHead string
	ProductCommits, RecordCommits                     int
}

func ValidateWorkCandidate(
	plan Plan,
	repository Repository,
	workID string,
	status Status,
	authorityHead string,
	resolveBehavioralInertness InertnessResolver,
) (CandidateValidation, error) {
	if _, err := plan.require(); err != nil {
		return CandidateValidation{}, err
	}
	if repository == nil {
		return CandidateValidation{}, recordFail("INVALID_REPOSITORY", "candidate validation requires a repository")
	}
	metadata := plan.Metadata()
	track, work, found := plan.FindWork(workID)
	if !found {
		return CandidateValidation{}, recordFail("UNKNOWN_WORK", "plan has no work "+workID)
	}
	view := status.View()
	if err := validateWorkIdentity(status, metadata, track, work); err != nil {
		return CandidateValidation{}, err
	}
	if view.Proof == nil {
		return CandidateValidation{}, recordFail("MISSING_PROOF", "Git proof validation requires status.proof")
	}
	proof := view.Proof
	if proof.Repository != metadata.Repository {
		return CandidateValidation{}, recordFail("REPOSITORY_MISMATCH", "proof repository does not match the approved plan")
	}
	for label, object := range map[string]string{
		"base": proof.BaseCommit, "candidate": proof.CandidateCommit,
		"candidate tree": proof.CandidateTree, "authority head": authorityHead,
	} {
		if err := validateRepositoryOID(repository, object, label); err != nil {
			return CandidateValidation{}, err
		}
	}
	if proof.BaseCommit == proof.CandidateCommit {
		return CandidateValidation{}, recordFail("EMPTY_CANDIDATE", "candidate must differ from its proof base")
	}
	ancestor, err := repository.IsAncestor(proof.BaseCommit, proof.CandidateCommit)
	if err != nil {
		return CandidateValidation{}, err
	}
	if !ancestor {
		return CandidateValidation{}, recordFail("INVALID_CANDIDATE", "proof base is not an ancestor of candidate")
	}
	reachable, err := repository.IsAncestor(proof.CandidateCommit, authorityHead)
	if err != nil {
		return CandidateValidation{}, err
	}
	if !reachable {
		return CandidateValidation{}, recordFail("CANDIDATE_NOT_ON_AUTHORITY", "proof candidate is not reachable from its authoritative ref head")
	}
	tree, err := repository.TreeOID(proof.CandidateCommit)
	if err != nil {
		return CandidateValidation{}, err
	}
	if tree != proof.CandidateTree {
		return CandidateValidation{}, recordFail("TREE_MISMATCH", "proof candidate tree does not match Git")
	}
	inertnessRequest := InertnessRequest{Repository: repository.Root(), RecordRoot: metadata.RecordRoot, Commit: proof.CandidateCommit}
	if err := resolveInertness(resolveBehavioralInertness, inertnessRequest); err != nil {
		return CandidateValidation{}, err
	}
	productTree, err := repository.ProductTreeIdentity(proof.CandidateCommit)
	if err != nil {
		return CandidateValidation{}, err
	}
	if productTree != proof.ProductTree {
		return CandidateValidation{}, recordFail("PRODUCT_TREE_MISMATCH", "proof product tree does not match Git")
	}
	history, err := repository.FirstParentHistory(proof.BaseCommit, proof.CandidateCommit)
	if err != nil {
		return CandidateValidation{}, err
	}
	if len(history) == 0 || history[len(history)-1] != proof.CandidateCommit {
		return CandidateValidation{}, recordFail("INVALID_HISTORY", "candidate first-parent history is incomplete")
	}
	workIndex := -1
	trackStatuses := make(map[string]Status, len(track.Work))
	for index, candidateWork := range track.Work {
		if candidateWork.ID == workID {
			workIndex = index
		}
		trackStatuses[candidateWork.ID], err = readStatusAt(repository, proof.BaseCommit, WorkStatusPath(plan, candidateWork.ID), plan)
		if err != nil {
			return CandidateValidation{}, err
		}
	}
	if workIndex < 0 {
		return CandidateValidation{}, recordFail("UNKNOWN_WORK", "owning track has no work "+workID)
	}
	currentStatus := trackStatuses[workID]
	proceedSeen := currentStatus.View().Captain != nil && currentStatus.View().Captain.Outcome == "proceed"
	previousCommit := proof.BaseCommit
	productCommits := 0
	recordCommits := 0
	for index, commit := range history {
		parents, err := repository.Parents(commit)
		if err != nil {
			return CandidateValidation{}, err
		}
		if len(parents) != 1 || parents[0] != previousCommit {
			return CandidateValidation{}, recordFail("INVALID_HISTORY", "candidate history must be direct single-parent commits")
		}
		changed, err := repository.ChangedPaths(previousCommit, commit)
		if err != nil {
			return CandidateValidation{}, err
		}
		if len(changed) == 0 {
			return CandidateValidation{}, recordFail("EMPTY_COMMIT", "candidate history contains an empty commit")
		}
		recordPaths, productPaths := splitRecordPaths(changed, metadata.RecordRoot)
		if len(recordPaths) > 0 && len(productPaths) > 0 {
			return CandidateValidation{}, recordFail("MIXED_PRODUCT_RECORD_COMMIT", "candidate history mixes Baton records and product")
		}
		if len(productPaths) > 0 {
			if !proceedSeen {
				return CandidateValidation{}, recordFail("PRODUCT_BEFORE_PROCEED", "candidate product changed before current Captain PROCEED")
			}
			for priorIndex := 0; priorIndex < workIndex; priorIndex++ {
				prior := trackStatuses[track.Work[priorIndex].ID].View()
				passed := prior.Stage == "merge" && prior.Status == "ready" && prior.NextRole == "merge" &&
					prior.Outcome == "pass" && prior.AuthorityRef == track.Ref
				transferred := prior.Stage == "merge" && prior.Status == "complete"
				if !passed && !transferred {
					return CandidateValidation{}, recordFail("OUT_OF_ORDER_WORK", "product changed before earlier work "+track.Work[priorIndex].ID+" passed")
				}
			}
			for _, changedPath := range productPaths {
				if !workAllowsPath(work, changedPath) {
					return CandidateValidation{}, recordFail("WORK_OUTSIDE_SCOPE", "candidate changes out-of-scope path "+changedPath)
				}
			}
			productCommits++
		} else {
			if err := validateCandidateRecordPaths(plan, track, work, index == 0, recordPaths); err != nil {
				return CandidateValidation{}, err
			}
			if err := assertRegularRecords(repository, commit, recordPaths); err != nil {
				return CandidateValidation{}, err
			}
			for _, candidateWork := range track.Work {
				statusPath := WorkStatusPath(plan, candidateWork.ID)
				if !slices.Contains(recordPaths, statusPath) {
					continue
				}
				previousStatus := trackStatuses[candidateWork.ID]
				nextStatus, err := readStatusAt(repository, commit, statusPath, plan)
				if err != nil {
					return CandidateValidation{}, err
				}
				if !statusesEqual(previousStatus, nextStatus) {
					results := matchingTransitions(previousStatus, nextStatus)
					if len(results) != 1 {
						return CandidateValidation{}, recordFail("INVALID_RECORDED_TRANSITION", fmt.Sprintf("record at %s for %s matches %d lifecycle transitions", commit, candidateWork.ID, len(results)))
					}
					trackStatuses[candidateWork.ID] = nextStatus
				}
			}
			currentStatus = trackStatuses[workID]
			proceedSeen = currentStatus.View().Captain != nil && currentStatus.View().Captain.Outcome == "proceed"
			recordCommits++
		}
		previousCommit = commit
	}
	if productCommits == 0 {
		return CandidateValidation{}, recordFail("EMPTY_CANDIDATE", "candidate contains no product change")
	}
	finalParents, err := repository.Parents(proof.CandidateCommit)
	if err != nil {
		return CandidateValidation{}, err
	}
	if len(finalParents) != 1 {
		return CandidateValidation{}, recordFail("INVALID_CANDIDATE", "final product candidate must be single-parent")
	}
	finalChanged, err := repository.ChangedPaths(finalParents[0], proof.CandidateCommit)
	if err != nil {
		return CandidateValidation{}, err
	}
	finalRecords, finalProduct := splitRecordPaths(finalChanged, metadata.RecordRoot)
	if len(finalRecords) != 0 || len(finalProduct) == 0 {
		return CandidateValidation{}, recordFail("NON_PRODUCT_FINAL_CANDIDATE", "final candidate commit must be product-only")
	}
	if err := ValidateWorkRecordTail(plan, repository, workID, proof.CandidateCommit, authorityHead, status); err != nil {
		return CandidateValidation{}, err
	}
	return CandidateValidation{
		Base: proof.BaseCommit, Candidate: proof.CandidateCommit, Tree: tree,
		ProductTree: productTree, AuthorityHead: authorityHead,
		ProductCommits: productCommits, RecordCommits: recordCommits,
	}, nil
}
func ValidateWorkRecordTail(plan Plan, repository Repository, workID, candidate, authorityHead string, expected Status) error {
	if candidate == authorityHead {
		return recordFail("MISSING_IMPLEMENTED_RECORD", "authority head does not contain the implemented status")
	}
	history, err := repository.FirstParentHistory(candidate, authorityHead)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		return recordFail("MISSING_IMPLEMENTED_RECORD", "authority head has no record tail")
	}
	metadata := plan.Metadata()
	_, work, found := plan.FindWork(workID)
	if !found {
		return recordFail("UNKNOWN_WORK", "plan has no work "+workID)
	}
	previous := candidate
	for _, commit := range history {
		parents, err := repository.Parents(commit)
		if err != nil {
			return err
		}
		if len(parents) != 1 || parents[0] != previous {
			return recordFail("INVALID_RECORD_TAIL", "record tail must be direct single-parent commits")
		}
		changed, err := repository.ChangedPaths(previous, commit)
		if err != nil {
			return err
		}
		recordPaths, productPaths := splitRecordPaths(changed, metadata.RecordRoot)
		if len(productPaths) != 0 || len(recordPaths) == 0 {
			return recordFail("PRODUCT_AFTER_CANDIDATE", "record tail contains hidden product change")
		}
		if err := validateCurrentWorkRecordPaths(plan, work, recordPaths); err != nil {
			return err
		}
		if err := assertRegularRecords(repository, commit, recordPaths); err != nil {
			return err
		}
		previous = commit
	}
	observed, err := readStatusAt(repository, authorityHead, WorkStatusPath(plan, workID), plan)
	if err != nil {
		return err
	}
	if !statusesEqual(observed, expected) {
		return recordFail("STATUS_MISMATCH", "authority head status does not equal the admitted status")
	}
	return validateStatusHandoffs(repository, plan, authorityHead, expected)
}
func validateStatusHandoffs(repository Repository, plan Plan, head string, status Status) error {
	view := status.View()
	if view.Design != nil {
		body, err := repository.ReadBlob(head, WorkDesignPath(plan, *view.WorkID))
		if err != nil {
			return err
		}
		if DigestBytes(body) != view.Design.Digest {
			return recordFail("HANDOFF_DIGEST_MISMATCH", "design bytes do not match status digest")
		}
	}
	if view.Proof != nil {
		var proofPath string
		if view.Kind == "work" {
			proofPath = WorkProofPath(plan, *view.WorkID)
		} else {
			proofPath = AssemblyProofPath(plan)
		}
		body, err := repository.ReadBlob(head, proofPath)
		if err != nil {
			return err
		}
		if DigestBytes(body) != view.Proof.Digest {
			return recordFail("HANDOFF_DIGEST_MISMATCH", "proof bytes do not match status digest")
		}
	}
	return nil
}
func validateRepositoryOID(repository Repository, value, label string) error {
	length := 40
	if repository.ObjectFormat() == "sha256" {
		length = 64
	}
	if len(value) != length {
		return recordFail("OBJECT_FORMAT_MISMATCH", fmt.Sprintf("%s must contain %d lowercase hex characters", label, length))
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return recordFail("INVALID_OBJECT_ID", label+" is not lowercase hexadecimal")
		}
	}
	return nil
}
func readStatusAt(repository Repository, commit, statusPath string, plan Plan) (Status, error) {
	body, err := repository.ReadBlob(commit, statusPath)
	if err != nil {
		return Status{}, err
	}
	metadata := plan.Metadata()
	return ParseStatus(body, StatusExpectation{PlanDigest: plan.Digest(), ApprovalRef: metadata.ApprovalRef})
}
func splitRecordPaths(paths []string, recordRoot string) ([]string, []string) {
	var records, product []string
	for _, changedPath := range paths {
		if changedPath == recordRoot || strings.HasPrefix(changedPath, recordRoot+"/") {
			records = append(records, changedPath)
		} else {
			product = append(product, changedPath)
		}
	}
	return records, product
}
func workAllowsPath(work Work, changedPath string) bool {
	included := false
	for _, include := range work.Scope.Include {
		if pathContains(include, changedPath) {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, exclude := range work.Scope.Exclude {
		if pathContains(exclude, changedPath) {
			return false
		}
	}
	return true
}
func validateCandidateRecordPaths(plan Plan, track Track, work Work, mayMaterialize bool, paths []string) error {
	allowed := make([]string, 0)
	workIndex := -1
	for index, candidate := range track.Work {
		if candidate.ID == work.ID {
			workIndex = index
			break
		}
	}
	for index := 0; index <= workIndex; index++ {
		candidate := track.Work[index]
		allowed = append(allowed, WorkStatusPath(plan, candidate.ID), WorkDesignPath(plan, candidate.ID), WorkProofPath(plan, candidate.ID))
	}
	if mayMaterialize {
		for _, candidate := range track.Work {
			allowed = append(allowed, WorkStatusPath(plan, candidate.ID))
		}
	}
	slices.Sort(allowed)
	for _, changedPath := range paths {
		if !slices.Contains(allowed, changedPath) {
			return recordFail("CROSS_WORK_RECORD", "candidate history changes unexpected record "+changedPath)
		}
	}
	return nil
}
func validateCurrentWorkRecordPaths(plan Plan, work Work, paths []string) error {
	allowed := []string{WorkStatusPath(plan, work.ID), WorkDesignPath(plan, work.ID), WorkProofPath(plan, work.ID)}
	for _, changedPath := range paths {
		if !slices.Contains(allowed, changedPath) {
			return recordFail("CROSS_WORK_RECORD", "record tail changes unexpected record "+changedPath)
		}
	}
	return nil
}
func matchingTransitions(previous, next Status) []TransitionResult {
	results := []TransitionResult{
		DesignWritten, Proceed, Revise, Escalate, Implemented,
		Pass, Fail, Blocked, Merged, Materialize, Rebound,
	}
	var matched []TransitionResult
	for _, result := range results {
		if ValidateTransition(previous, next, result) == nil {
			matched = append(matched, result)
		}
	}
	return matched
}
func assertRegularRecords(repository Repository, commit string, paths []string) error {
	entries, err := repository.ListTree(commit)
	if err != nil {
		return err
	}
	observed := make(map[string]RepositoryTreeEntry, len(entries))
	for _, entry := range entries {
		observed[entry.Path] = entry
	}
	for _, name := range paths {
		entry, ok := observed[name]
		if !ok || entry.Mode != "100644" || entry.Type != "blob" {
			return recordFail("NONREGULAR_RECORD", "Baton record "+name+" is not a regular file")
		}
	}
	return nil
}
