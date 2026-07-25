package baton

import (
	"fmt"
	"reflect"
)

func (a *Actions) trackIndex(trackID string) (int, Track, error) {
	for index, track := range a.plan.Metadata().Tracks {
		if track.ID == trackID {
			return index, track, nil
		}
	}
	return 0, Track{}, recordFail("UNKNOWN_TRACK", "plan has no track "+trackID)
}
func (a *Actions) MaterializeTrack(trackID string) (Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	trackIndex, track, err := a.trackIndex(trackID)
	if err != nil {
		return Receipt{}, err
	}
	snapshot, before, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	if !refExists(before.Release) {
		return Receipt{}, recordFail("REF_NOT_FOUND", "release ref is absent")
	}
	owner := before.Tracks[trackIndex]
	if refExists(owner) {
		if before.Release.Head != owner.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "track "+trackID+" is no longer at its exact materialization effect")
		}
		parents, err := a.repository.Parents(owner.Head)
		if err != nil || len(parents) != 1 {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "track marker has no exact baseline parent")
		}
		changes := make([]RepositoryChange, 0, len(track.Work))
		for _, work := range track.Work {
			selected, err := snapshot.SelectWork(work.ID)
			if err != nil {
				return Receipt{}, err
			}
			view := selected.Status.View()
			if selected.Source != "owner" || selected.Head != owner.Head || view.Materialization == nil ||
				projection(view) != "design/ready/implementer" || view.Outcome != "none" {
				return Receipt{}, recordFail("INVALID_MATERIALIZATION", "track "+trackID+" has advanced beyond its marker")
			}
			changes = append(changes, RepositoryChange{Path: WorkStatusPath(a.plan, work.ID), Bytes: selected.Status.Bytes()})
		}
		replay, err := a.prepareRecord(parents[0], "Materialize Baton track "+trackID, changes)
		if err != nil {
			return Receipt{}, err
		}
		if replay.Commit != owner.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "track marker is not the exact Baton effect")
		}
		materialization, _, _ := snapshot.MaterializationFor(trackID)
		return newReceipt("materializeTrack", map[string]any{
			"changed": false, "track_id": trackID, "base_commit": materialization.BaseCommit,
			"owner_head": owner.Head, "before": before.receiptValue(), "after": before.receiptValue(),
		})
	}
	materialization := Materialization{BaseCommit: before.Release.Head, Dependencies: []MaterializationDependency{}}
	for _, dependencyID := range track.DependsOn {
		dependencyIndex, dependency, err := a.trackIndex(dependencyID)
		if err != nil {
			return Receipt{}, err
		}
		dependencyHead := before.Tracks[dependencyIndex]
		if !refExists(dependencyHead) {
			return Receipt{}, recordFail("UNMET_TRACK_DEPENDENCY", "dependency "+dependencyID+" has no captured owner head")
		}
		contained, err := a.repository.IsAncestor(dependencyHead.Head, before.Release.Head)
		if err != nil {
			return Receipt{}, err
		}
		if !contained {
			return Receipt{}, recordFail("UNMET_TRACK_DEPENDENCY", "release does not contain dependency "+dependencyID)
		}
		for _, work := range dependency.Work {
			selected, err := snapshot.SelectWork(work.ID)
			if err != nil {
				return Receipt{}, err
			}
			view := selected.Status.View()
			if selected.Source != "composed" || view.Stage != "merge" || view.Status != "complete" ||
				view.Merge == nil || view.Merge.FrozenTrackHead == nil ||
				*view.Merge.FrozenTrackHead != dependencyHead.Head {
				return Receipt{}, recordFail("UNMET_TRACK_DEPENDENCY", "dependency "+dependencyID+"/"+work.ID+" lacks exact transfer")
			}
		}
		materialization.Dependencies = append(materialization.Dependencies, MaterializationDependency{
			TrackID: dependencyID, FrozenHead: dependencyHead.Head,
		})
	}
	var changes []RepositoryChange
	for _, work := range track.Work {
		selected, err := snapshot.SelectWork(work.ID)
		if err != nil {
			return Receipt{}, err
		}
		if selected.Source != "baseline" {
			return Receipt{}, recordFail("INVALID_MATERIALIZATION", "work "+work.ID+" is not one release baseline")
		}
		nextView := selected.Status.View()
		nextView.AuthorityRef = track.Ref
		copyMaterialization := materialization
		copyMaterialization.Dependencies = make([]MaterializationDependency, len(materialization.Dependencies))
		copy(copyMaterialization.Dependencies, materialization.Dependencies)
		nextView.Materialization = &copyMaterialization
		next, err := encodeStatus(nextView, StatusExpectation{PlanDigest: a.plan.Digest(), ApprovalRef: a.plan.Metadata().ApprovalRef})
		if err != nil {
			return Receipt{}, err
		}
		if err := ValidateTransition(selected.Status, next, Materialize); err != nil {
			return Receipt{}, err
		}
		previousAdmission, err := a.admit(selected.Status)
		if err != nil {
			return Receipt{}, err
		}
		nextAdmission, err := a.admit(next)
		if err != nil {
			return Receipt{}, err
		}
		if err := requireEvidenceAdmission(selected.Status, previousAdmission, a.profile); err != nil {
			return Receipt{}, err
		}
		if err := requireEvidenceAdmission(next, nextAdmission, a.profile); err != nil {
			return Receipt{}, err
		}
		changes = append(changes, RepositoryChange{Path: WorkStatusPath(a.plan, work.ID), Bytes: next.Bytes()})
	}
	prepared, err := a.prepareRecord(before.Release.Head, "Materialize Baton track "+trackID, changes)
	if err != nil {
		return Receipt{}, err
	}
	operations := []RefOperation{
		{Kind: "update", Ref: before.Release.Ref, NewHead: prepared.Commit, ExpectedHead: before.Release.Head},
		{Kind: "create", Ref: track.Ref, NewHead: prepared.Commit},
	}
	operations = append(operations, a.verifyOperations(before, map[string]bool{before.Release.Ref: true, track.Ref: true})...)
	if err := a.repository.AtomicUpdateRefs(snapshot.refs, operations); err != nil {
		return Receipt{}, err
	}
	_, after, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	if after.Release.Head != prepared.Commit || after.Tracks[trackIndex].Head != prepared.Commit {
		return Receipt{}, recordFail("ACTION_EFFECT_MISMATCH", "materialization did not atomically install both refs")
	}
	return newReceipt("materializeTrack", map[string]any{
		"changed": true, "track_id": trackID, "base_commit": materialization.BaseCommit,
		"owner_head": prepared.Commit, "before": before.receiptValue(), "after": after.receiptValue(),
	})
}
func mergedWorkStatus(plan Plan, previous Status, frozenHead, expectedTarget, resultCommit string) (Status, error) {
	next := previous.View()
	next.AuthorityRef = plan.Metadata().ReleaseRef
	next.Stage = "merge"
	next.Status = "complete"
	next.NextRole = "none"
	next.Outcome = "merged"
	frozen := frozenHead
	next.Merge = &MergeBinding{
		Scope: "track", PassedCandidate: next.Proof.CandidateCommit, FrozenTrackHead: &frozen,
		ExpectedTarget: expectedTarget, Outcome: "merged", ObservedTarget: expectedTarget,
		ResultCommit: resultCommit, PlanDigest: plan.Digest(),
		VerificationAttestationDigest: next.Verification.AttestationDigest,
	}
	return encodeStatus(next, StatusExpectation{PlanDigest: plan.Digest(), ApprovalRef: plan.Metadata().ApprovalRef})
}
func (a *Actions) prepareComposition(expected, candidate, targetRef string) (PreparedComposition, error) {
	metadata := a.plan.Metadata()
	for _, commit := range []string{expected, candidate} {
		if err := resolveInertness(a.inertness, InertnessRequest{
			Repository: a.repository.Root(), RecordRoot: metadata.RecordRoot, Commit: commit,
		}); err != nil {
			return PreparedComposition{}, err
		}
	}
	result, err := a.repository.PrepareComposition(PrepareCompositionRequest{
		Expected: expected, Candidate: candidate,
		Message: fmt.Sprintf("Baton exact composition of %s into %s\n", candidate, targetRef),
		Author:  mergeAuthor, Email: mergeEmail,
	})
	if err != nil {
		return PreparedComposition{}, err
	}
	if err := resolveInertness(a.inertness, InertnessRequest{
		Repository: a.repository.Root(), RecordRoot: metadata.RecordRoot, Commit: result.Commit,
	}); err != nil {
		return PreparedComposition{}, err
	}
	return result, nil
}
func (a *Actions) ComposeTrack(trackID string) (Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	trackIndex, track, err := a.trackIndex(trackID)
	if err != nil {
		return Receipt{}, err
	}
	snapshot, before, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	owner := before.Tracks[trackIndex]
	if !refExists(owner) {
		return Receipt{}, recordFail("AUTHORITATIVE_STATUS_MISSING", "track "+trackID+" is not materialized")
	}
	selections := make([]Selection, len(track.Work))
	composed := 0
	for index, work := range track.Work {
		selections[index], err = snapshot.SelectWork(work.ID)
		if err != nil {
			return Receipt{}, err
		}
		if selections[index].Source == "composed" {
			composed++
		}
	}
	if composed > 0 && composed != len(track.Work) {
		return Receipt{}, recordFail("PARTIAL_TRACK_TRANSFER", "track "+trackID+" is only partially transferred")
	}
	if composed == len(track.Work) {
		expected := selections[0].Status.View().Merge.ExpectedTarget
		composition, err := a.prepareComposition(expected, owner.Head, a.plan.Metadata().ReleaseRef)
		if err != nil {
			return Receipt{}, err
		}
		var changes []RepositoryChange
		for index, work := range track.Work {
			view := selections[index].Status.View()
			if view.Merge == nil || view.Merge.FrozenTrackHead == nil || *view.Merge.FrozenTrackHead != owner.Head ||
				view.Merge.ExpectedTarget != expected || view.Merge.ResultCommit != composition.Commit {
				return Receipt{}, recordFail("INVALID_AUTHORITY_TRANSFER", "track has divergent transfer bindings")
			}
			changes = append(changes, RepositoryChange{Path: WorkStatusPath(a.plan, work.ID), Bytes: selections[index].Status.Bytes()})
		}
		transfer, err := a.prepareRecord(composition.Commit, "Transfer composed Baton track "+trackID, changes)
		if err != nil {
			return Receipt{}, err
		}
		if transfer.Commit != before.Release.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "track transfer is not the exact Baton effect")
		}
		return newReceipt("composeTrack", map[string]any{
			"changed": false, "track_id": trackID, "frozen_track_head": owner.Head,
			"composition_commit": composition.Commit, "transfer_commit": transfer.Commit,
			"before": before.receiptValue(), "after": before.receiptValue(),
		})
	}
	ready, err := snapshot.TrackReadyForComposition(trackID)
	if err != nil {
		return Receipt{}, err
	}
	if !ready {
		return Receipt{}, recordFail("TRACK_NOT_READY", "track "+trackID+" has not passed on its owner")
	}
	for _, selected := range selections {
		if selected.Source != "owner" || selected.Head != owner.Head {
			return Receipt{}, recordFail("INVALID_AUTHORITY_TRANSFER", "work is not owned by frozen track "+trackID)
		}
		if _, err := a.admit(selected.Status); err != nil {
			return Receipt{}, err
		}
	}
	composition, err := a.prepareComposition(before.Release.Head, owner.Head, a.plan.Metadata().ReleaseRef)
	if err != nil {
		return Receipt{}, err
	}
	var changes []RepositoryChange
	nextStatuses := make([]Status, len(track.Work))
	for index, work := range track.Work {
		nextStatuses[index], err = mergedWorkStatus(a.plan, selections[index].Status, owner.Head, before.Release.Head, composition.Commit)
		if err != nil {
			return Receipt{}, err
		}
		if err := ValidateTransition(selections[index].Status, nextStatuses[index], Merged); err != nil {
			return Receipt{}, err
		}
		changes = append(changes, RepositoryChange{Path: WorkStatusPath(a.plan, work.ID), Bytes: nextStatuses[index].Bytes()})
	}
	transfer, err := a.prepareRecord(composition.Commit, "Transfer composed Baton track "+trackID, changes)
	if err != nil {
		return Receipt{}, err
	}
	operations := []RefOperation{{Kind: "update", Ref: before.Release.Ref, NewHead: transfer.Commit, ExpectedHead: before.Release.Head}}
	operations = append(operations, a.verifyOperations(before, map[string]bool{before.Release.Ref: true})...)
	if err := a.repository.AtomicUpdateRefs(snapshot.refs, operations); err != nil {
		return Receipt{}, err
	}
	_, after, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	if after.Release.Head != transfer.Commit || after.Tracks[trackIndex].Head != owner.Head {
		return Receipt{}, recordFail("ACTION_EFFECT_MISMATCH", "composition moved an unexpected ref")
	}
	return newReceipt("composeTrack", map[string]any{
		"changed": true, "track_id": trackID, "frozen_track_head": owner.Head,
		"composition_commit": composition.Commit, "transfer_commit": transfer.Commit,
		"before": before.receiptValue(), "after": after.receiptValue(),
	})
}

type PrepareAssemblyInput struct {
	ProofBytes         []byte
	ProducerInvocation string
}

func (a *Actions) PrepareAssembly(input PrepareAssemblyInput) (Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if input.ProofBytes == nil || !invocationPattern.MatchString(input.ProducerInvocation) {
		return Receipt{}, recordFail("INVALID_ACTION_INPUT", "prepareAssembly requires proof bytes and one invocation")
	}
	proofBytes := append([]byte(nil), input.ProofBytes...)
	snapshot, before, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	if selected, exists, err := snapshot.SelectAssembly(); err != nil {
		return Receipt{}, err
	} else if exists {
		view := selected.Status.View()
		if projection(view) != "verify/ready/verifier" || view.Outcome != "none" ||
			view.Proof.Digest != DigestBytes(proofBytes) ||
			view.Proof.ProducerInvocation != input.ProducerInvocation ||
			view.Proof.BaseCommit != view.Proof.CandidateCommit {
			return Receipt{}, recordFail("ASSEMBLY_ALREADY_PREPARED", "assembly exists with a different or advanced durable state")
		}
		parents, err := a.repository.Parents(selected.Head)
		if err != nil || len(parents) != 1 || parents[0] != view.Proof.CandidateCommit {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "assembly preparation has no exact candidate parent")
		}
		replay, err := a.prepareRecord(view.Proof.CandidateCommit, "Prepare Baton assembly "+a.plan.Metadata().Release, []RepositoryChange{
			{Path: AssemblyProofPath(a.plan), Bytes: proofBytes},
			{Path: AssemblyStatusPath(a.plan), Bytes: selected.Status.Bytes()},
		})
		if err != nil {
			return Receipt{}, err
		}
		if replay.Commit != selected.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "assembly preparation is not the exact Baton effect")
		}
		return newReceipt("prepareAssembly", map[string]any{
			"changed": false, "assembly_candidate": view.Proof.CandidateCommit,
			"preparation_commit": selected.Head, "before": before.receiptValue(), "after": before.receiptValue(),
		})
	}
	ready, err := snapshot.ReleaseReadyForAssembly()
	if err != nil {
		return Receipt{}, err
	}
	if !ready {
		return Receipt{}, recordFail("INCOMPLETE_ASSEMBLY", "not every track has an exact transfer")
	}
	metadata := a.plan.Metadata()
	firstWork := metadata.Tracks[0].Work[0].ID
	first, err := snapshot.SelectWork(firstWork)
	if err != nil {
		return Receipt{}, err
	}
	candidate := before.Release.Head
	if err := resolveInertness(a.inertness, InertnessRequest{Repository: a.repository.Root(), RecordRoot: metadata.RecordRoot, Commit: candidate}); err != nil {
		return Receipt{}, err
	}
	tree, err := a.repository.TreeOID(candidate)
	if err != nil {
		return Receipt{}, err
	}
	product, err := a.repository.ProductTreeIdentity(candidate)
	if err != nil {
		return Receipt{}, err
	}
	components := make([]ProofComponent, len(metadata.Tracks))
	for index, track := range metadata.Tracks {
		if !refExists(before.Tracks[index]) {
			return Receipt{}, recordFail("INCOMPLETE_ASSEMBLY", "track "+track.ID+" has no owner head")
		}
		components[index] = ProofComponent{TrackID: track.ID, Head: before.Tracks[index].Head}
	}
	status, err := encodeStatus(StatusView{
		Schema: StatusSchema, SchemaVersion: StatusVersion, Kind: "assembly", Release: metadata.Release,
		OwnerRef: metadata.ReleaseRef, AuthorityRef: metadata.ReleaseRef, TargetRef: metadata.TargetRef,
		Plan: first.Status.View().Plan, Stage: "verify", Status: "ready", NextRole: "verifier", Outcome: "none",
		Proof: &ProofBinding{
			Digest: DigestBytes(proofBytes), ProducerInvocation: input.ProducerInvocation,
			Repository: metadata.Repository, BaseCommit: candidate, CandidateCommit: candidate,
			CandidateTree: tree, ProductTree: product, PlanDigest: a.plan.Digest(),
			ApprovalDigest: first.Status.View().Plan.Approval.Digest, Components: components,
		},
	}, StatusExpectation{PlanDigest: a.plan.Digest(), ApprovalRef: metadata.ApprovalRef})
	if err != nil {
		return Receipt{}, err
	}
	if _, err := a.admit(status); err != nil {
		return Receipt{}, err
	}
	prepared, err := a.prepareRecord(candidate, "Prepare Baton assembly "+metadata.Release, []RepositoryChange{
		{Path: AssemblyProofPath(a.plan), Bytes: proofBytes},
		{Path: AssemblyStatusPath(a.plan), Bytes: status.Bytes()},
	})
	if err != nil {
		return Receipt{}, err
	}
	if err := a.validateAssembly(status, prepared.Commit, before, snapshot); err != nil {
		return Receipt{}, err
	}
	operations := []RefOperation{{Kind: "update", Ref: before.Release.Ref, NewHead: prepared.Commit, ExpectedHead: before.Release.Head}}
	operations = append(operations, a.verifyOperations(before, map[string]bool{before.Release.Ref: true})...)
	if err := a.repository.AtomicUpdateRefs(snapshot.refs, operations); err != nil {
		return Receipt{}, err
	}
	_, after, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	return newReceipt("prepareAssembly", map[string]any{
		"changed": true, "assembly_candidate": candidate, "preparation_commit": prepared.Commit,
		"before": before.receiptValue(), "after": after.receiptValue(),
	})
}
func mergedAssemblyStatus(plan Plan, previous Status, expectedTarget, resultCommit string) (Status, error) {
	next := previous.View()
	next.Stage = "merge"
	next.Status = "complete"
	next.NextRole = "none"
	next.Outcome = "merged"
	next.Merge = &MergeBinding{
		Scope: "release", PassedCandidate: next.Proof.CandidateCommit,
		ExpectedTarget: expectedTarget, Outcome: "merged", ObservedTarget: expectedTarget,
		ResultCommit: resultCommit, PlanDigest: plan.Digest(),
		VerificationAttestationDigest: next.Verification.AttestationDigest,
	}
	return encodeStatus(next, StatusExpectation{PlanDigest: plan.Digest(), ApprovalRef: plan.Metadata().ApprovalRef})
}
func (a *Actions) IntegrateRelease() (Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	snapshot, before, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	selected, exists, err := snapshot.SelectAssembly()
	if err != nil {
		return Receipt{}, err
	}
	if !exists {
		return Receipt{}, recordFail("AUTHORITATIVE_STATUS_MISSING", "assembly has not been prepared")
	}
	previous := selected.Status
	if projection(previous.View()) == "merge/complete/none" && previous.View().Outcome == "merged" {
		merge := previous.View().Merge
		if !refExists(before.Target) || before.Target.Head != merge.ResultCommit {
			return Receipt{}, recordFail("ACTION_EFFECT_MISMATCH", "terminal assembly does not match current target")
		}
		parents, err := a.repository.Parents(before.Release.Head)
		if err != nil || len(parents) != 1 {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "terminal assembly has no exact record predecessor")
		}
		prior, err := readStatusAt(a.repository, parents[0], AssemblyStatusPath(a.plan), a.plan)
		if err != nil {
			return Receipt{}, err
		}
		if err := ValidateTransition(prior, previous, Merged); err != nil {
			return Receipt{}, err
		}
		if _, err := a.admit(prior); err != nil {
			return Receipt{}, err
		}
		if err := a.validateAssembly(prior, parents[0], before, snapshot); err != nil {
			return Receipt{}, err
		}
		composition, err := a.prepareComposition(merge.ExpectedTarget, prior.View().Proof.CandidateCommit, a.plan.Metadata().TargetRef)
		if err != nil {
			return Receipt{}, err
		}
		if composition.Commit != merge.ResultCommit || composition.Commit != before.Target.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "release integration is not the exact Baton effect")
		}
		final, err := a.prepareRecord(parents[0], "Integrate Baton release "+a.plan.Metadata().Release, []RepositoryChange{
			{Path: AssemblyStatusPath(a.plan), Bytes: previous.Bytes()},
		})
		if err != nil {
			return Receipt{}, err
		}
		if final.Commit != before.Release.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "release status is not the exact Baton effect")
		}
		return newReceipt("integrateRelease", map[string]any{
			"changed": false, "assembly_candidate": merge.PassedCandidate,
			"integration_commit": merge.ResultCommit, "status_commit": before.Release.Head,
			"before": before.receiptValue(), "after": before.receiptValue(),
		})
	}
	if projection(previous.View()) != "merge/ready/merge" || previous.View().Outcome != "pass" {
		return Receipt{}, recordFail("UNVERIFIED_MERGE", "release integration requires exact assembly PASS")
	}
	if _, err := a.admit(previous); err != nil {
		return Receipt{}, err
	}
	if err := a.validateAssembly(previous, selected.Head, before, snapshot); err != nil {
		return Receipt{}, err
	}
	composition, err := a.prepareComposition(before.Target.Head, previous.View().Proof.CandidateCommit, a.plan.Metadata().TargetRef)
	if err != nil {
		return Receipt{}, err
	}
	next, err := mergedAssemblyStatus(a.plan, previous, before.Target.Head, composition.Commit)
	if err != nil {
		return Receipt{}, err
	}
	if err := ValidateTransition(previous, next, Merged); err != nil {
		return Receipt{}, err
	}
	final, err := a.prepareRecord(before.Release.Head, "Integrate Baton release "+a.plan.Metadata().Release, []RepositoryChange{
		{Path: AssemblyStatusPath(a.plan), Bytes: next.Bytes()},
	})
	if err != nil {
		return Receipt{}, err
	}
	operations := []RefOperation{
		{Kind: "update", Ref: before.Target.Ref, NewHead: composition.Commit, ExpectedHead: before.Target.Head},
		{Kind: "update", Ref: before.Release.Ref, NewHead: final.Commit, ExpectedHead: before.Release.Head},
	}
	operations = append(operations, a.verifyOperations(before, map[string]bool{before.Target.Ref: true, before.Release.Ref: true})...)
	if err := a.repository.AtomicUpdateRefs(snapshot.refs, operations); err != nil {
		return Receipt{}, err
	}
	_, after, err := a.captureSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	if after.Target.Head != composition.Commit || after.Release.Head != final.Commit {
		return Receipt{}, recordFail("ACTION_EFFECT_MISMATCH", "integration did not atomically install target and status")
	}
	return newReceipt("integrateRelease", map[string]any{
		"changed": true, "assembly_candidate": previous.View().Proof.CandidateCommit,
		"integration_commit": composition.Commit, "status_commit": final.Commit,
		"before": before.receiptValue(), "after": after.receiptValue(),
	})
}
func (a *Actions) validateAssembly(status Status, authorityHead string, heads actionHeads, snapshot Snapshot) error {
	view := status.View()
	if view.Kind != "assembly" || view.Proof == nil {
		return recordFail("INVALID_ASSEMBLY", "assembly validation requires an assembly proof")
	}
	metadata := a.plan.Metadata()
	for label, oid := range map[string]string{
		"base": view.Proof.BaseCommit, "candidate": view.Proof.CandidateCommit,
		"tree": view.Proof.CandidateTree, "authority": authorityHead,
	} {
		if err := validateRepositoryOID(a.repository, oid, label); err != nil {
			return err
		}
	}
	if view.Proof.BaseCommit != view.Proof.CandidateCommit {
		return recordFail("INVALID_ASSEMBLY", "assembly proof base and candidate must be identical")
	}
	reachable, err := a.repository.IsAncestor(view.Proof.CandidateCommit, authorityHead)
	if err != nil {
		return err
	}
	if !reachable {
		return recordFail("CANDIDATE_NOT_ON_AUTHORITY", "assembly candidate is not reachable from release authority")
	}
	tree, err := a.repository.TreeOID(view.Proof.CandidateCommit)
	if err != nil || tree != view.Proof.CandidateTree {
		return recordFail("TREE_MISMATCH", "assembly candidate tree does not match Git")
	}
	if err := resolveInertness(a.inertness, InertnessRequest{Repository: a.repository.Root(), RecordRoot: metadata.RecordRoot, Commit: view.Proof.CandidateCommit}); err != nil {
		return err
	}
	product, err := a.repository.ProductTreeIdentity(view.Proof.CandidateCommit)
	if err != nil {
		return err
	}
	if product != view.Proof.ProductTree {
		return recordFail("PRODUCT_TREE_MISMATCH", "assembly product tree does not match Git")
	}
	if len(view.Proof.Components) != len(metadata.Tracks) {
		return recordFail("INCOMPLETE_ASSEMBLY", "assembly does not bind every planned track")
	}
	for index, track := range metadata.Tracks {
		if !refExists(heads.Tracks[index]) || !reflect.DeepEqual(view.Proof.Components[index], ProofComponent{TrackID: track.ID, Head: heads.Tracks[index].Head}) {
			return recordFail("INCOMPLETE_ASSEMBLY", "assembly component heads are not exact")
		}
		for _, work := range track.Work {
			selected, err := snapshot.SelectWork(work.ID)
			if err != nil {
				return err
			}
			workView := selected.Status.View()
			if selected.Source != "composed" || workView.Merge == nil || workView.Merge.FrozenTrackHead == nil ||
				*workView.Merge.FrozenTrackHead != heads.Tracks[index].Head {
				return recordFail("INCOMPLETE_ASSEMBLY", "work "+work.ID+" lacks exact transfer")
			}
		}
	}
	return validateAssemblyRecordTail(a.plan, a.repository, view.Proof.CandidateCommit, authorityHead)
}
func validateAssemblyRecordTail(plan Plan, repository Repository, candidate, authorityHead string) error {
	if candidate == authorityHead {
		return nil
	}
	history, err := repository.FirstParentHistory(candidate, authorityHead)
	if err != nil {
		return err
	}
	allowed := map[string]bool{AssemblyProofPath(plan): true, AssemblyStatusPath(plan): true}
	previous := candidate
	for _, commit := range history {
		parents, err := repository.Parents(commit)
		if err != nil {
			return err
		}
		if len(parents) != 1 || parents[0] != previous {
			return recordFail("INVALID_ASSEMBLY", "assembly record tail is not direct")
		}
		paths, err := repository.ChangedPaths(previous, commit)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return recordFail("INVALID_ASSEMBLY", "assembly record tail contains an empty commit")
		}
		for _, path := range paths {
			if !allowed[path] {
				return recordFail("PRODUCT_AFTER_CANDIDATE", "assembly record tail changes "+path)
			}
		}
		if err := assertRegularRecords(repository, commit, paths); err != nil {
			return err
		}
		previous = commit
	}
	return nil
}
