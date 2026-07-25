package baton

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

const (
	recordsAuthor = "Baton Records"
	recordsEmail  = "records@baton.invalid"
	mergeAuthor   = "Baton Merge"
	mergeEmail    = "merge@baton.invalid"
)

// Receipt is an opaque immutable action receipt.
type Receipt struct {
	raw []byte
}

func newReceipt(action string, details map[string]any) (Receipt, error) {
	value := make(map[string]any, len(details)+2)
	value["kind"] = "baton.action-receipt/v1"
	value["action"] = action
	for key, item := range details {
		value[key] = item
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return Receipt{}, recordWrap("INVALID_RECEIPT", "encode action receipt", err)
	}
	return Receipt{raw: raw}, nil
}

func (r Receipt) MarshalJSON() ([]byte, error) {
	if len(r.raw) == 0 {
		return nil, recordFail("INVALID_RECEIPT", "zero receipt is not admitted")
	}
	return append([]byte(nil), r.raw...), nil
}

func (r Receipt) Data() map[string]any {
	if len(r.raw) == 0 {
		return nil
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(r.raw))
	decoder.UseNumber()
	_ = decoder.Decode(&result)
	return result
}

// Actions is the sole plan-bound mutation facade.
type Actions struct {
	plan       Plan
	profile    Profile
	repository Repository
	evidence   *evidenceCache
	inertness  InertnessResolver
	mu         sync.Mutex
}

func NewActions(
	plan Plan,
	profile Profile,
	repository Repository,
	resolveEvidence EvidenceResolver,
	resolveBehavioralInertness InertnessResolver,
) (*Actions, error) {
	if _, err := plan.require(); err != nil {
		return nil, err
	}
	if profile != Guided && profile != Autonomous {
		return nil, recordFail("INVALID_PROFILE", "NewActions requires guided or autonomous profile")
	}
	if repository == nil {
		return nil, recordFail("INVALID_REPOSITORY", "NewActions requires an admitted repository")
	}
	if resolveEvidence == nil {
		return nil, recordFail("EVIDENCE_RESOLVER_REQUIRED", "NewActions requires a trusted evidence resolver")
	}
	if resolveBehavioralInertness == nil {
		return nil, recordFail("INERTNESS_RESOLVER_REQUIRED", "NewActions requires a behavioral-inertness resolver")
	}
	cache := newEvidenceCache(resolveEvidence)
	return &Actions{
		plan: plan, profile: profile, repository: repository,
		evidence: cache, inertness: resolveBehavioralInertness,
	}, nil
}

func (a *Actions) admit(status Status) (*evidenceAdmission, error) {
	return resolveStatusEvidence(status, a.profile, a.evidence.resolve)
}

type actionHeads struct {
	Target   CapturedRef
	Release  CapturedRef
	Tracks   []CapturedRef
	TrackIDs []string
}

func (h actionHeads) receiptValue() map[string]any {
	trackValues := make([]any, len(h.Tracks))
	for index, track := range h.Tracks {
		value := refReceipt(track)
		if index < len(h.TrackIDs) {
			value["id"] = h.TrackIDs[index]
		}
		trackValues[index] = value
	}
	return map[string]any{
		"target": refReceipt(h.Target), "release": refReceipt(h.Release), "tracks": trackValues,
	}
}

func refReceipt(value CapturedRef) map[string]any {
	var head any
	if value.Exists {
		head = value.Head
	}
	return map[string]any{"ref": value.Ref, "head": head}
}

func (a *Actions) captureHeads() (actionHeads, error) {
	metadata := a.plan.Metadata()
	refs := []string{metadata.TargetRef, metadata.ReleaseRef}
	trackIDs := make([]string, len(metadata.Tracks))
	for index, track := range metadata.Tracks {
		refs = append(refs, track.Ref)
		trackIDs[index] = track.ID
	}
	values, err := a.repository.CaptureHeadRefs(refs)
	if err != nil {
		return actionHeads{}, err
	}
	if len(values) != len(refs) {
		return actionHeads{}, recordFail("INVALID_REF_SNAPSHOT", "repository returned an incomplete plan snapshot")
	}
	for index, ref := range refs {
		if values[index].Ref != ref {
			return actionHeads{}, recordFail("INVALID_REF_SNAPSHOT", "repository changed captured ref order")
		}
	}
	return actionHeads{
		Target: values[0], Release: values[1], Tracks: append([]CapturedRef(nil), values[2:]...),
		TrackIDs: trackIDs,
	}, nil
}

func (a *Actions) verifyOperations(before actionHeads, except map[string]bool) []RefOperation {
	var result []RefOperation
	all := append([]CapturedRef{before.Target, before.Release}, before.Tracks...)
	for _, ref := range all {
		if except[ref.Ref] {
			continue
		}
		result = append(result, RefOperation{
			Kind: "verify", Ref: ref.Ref, ExpectedHead: ref.Head, ExpectAbsent: !ref.Exists,
		})
	}
	return result
}

func (a *Actions) prepareRecord(parent, message string, changes []RepositoryChange) (PreparedRecord, error) {
	metadata := a.plan.Metadata()
	copied := make([]RepositoryChange, len(changes))
	for index, change := range changes {
		if change.Path != metadata.RecordRoot && !pathContains(metadata.RecordRoot, change.Path) {
			return PreparedRecord{}, recordFail("INVALID_RECORD_PATH", "action attempted to write outside the fixed record root")
		}
		copied[index] = RepositoryChange{Path: change.Path, Bytes: append([]byte(nil), change.Bytes...), Delete: change.Delete}
	}
	timestamp, err := a.repository.CommitTimestamp(parent)
	if err != nil {
		return PreparedRecord{}, err
	}
	prepared, err := a.repository.PrepareRecord(PrepareRecordRequest{
		Parent: parent, Changes: copied, Message: message + "\n",
		Author: recordsAuthor, Email: recordsEmail, Timestamp: timestamp + 1,
	})
	if err != nil {
		return PreparedRecord{}, err
	}
	for _, commit := range []string{parent, prepared.Commit} {
		if err := resolveInertness(a.inertness, InertnessRequest{
			Repository: a.repository.Root(), RecordRoot: metadata.RecordRoot, Commit: commit,
		}); err != nil {
			return PreparedRecord{}, err
		}
	}
	beforeProduct, err := a.repository.ProductTreeIdentity(parent, metadata.RecordRoot)
	if err != nil {
		return PreparedRecord{}, err
	}
	afterProduct, err := a.repository.ProductTreeIdentity(prepared.Commit, metadata.RecordRoot)
	if err != nil {
		return PreparedRecord{}, err
	}
	if beforeProduct != afterProduct {
		return PreparedRecord{}, recordFail("RECORD_CHANGED_PRODUCT", "record transition changed product identity")
	}
	return prepared, nil
}

func baselineStatus(plan Plan, track Track, work Work, approvalDigest string) (Status, error) {
	workID, trackID := work.ID, track.ID
	metadata := plan.Metadata()
	return encodeStatus(StatusView{
		Schema: StatusSchema, SchemaVersion: StatusVersion, Kind: "work",
		Release: metadata.Release, WorkID: &workID, TrackID: &trackID,
		OwnerRef: track.Ref, AuthorityRef: metadata.ReleaseRef, TargetRef: metadata.TargetRef,
		Plan:  PlanBinding{Digest: plan.Digest(), Approval: ApprovalBinding{Ref: metadata.ApprovalRef, Digest: approvalDigest}},
		Stage: "design", Status: "ready", NextRole: "implementer", Outcome: "none",
	}, StatusExpectation{PlanDigest: plan.Digest(), ApprovalRef: metadata.ApprovalRef, ApprovalDigest: approvalDigest})
}

func (a *Actions) baselineStatuses(approvalDigest string) (map[string]Status, error) {
	if !digestPattern.MatchString(approvalDigest) {
		return nil, recordFail("INVALID_ACTION_INPUT", "approval digest must be one SHA-256 digest")
	}
	result := make(map[string]Status)
	for _, track := range a.plan.Metadata().Tracks {
		for _, work := range track.Work {
			status, err := baselineStatus(a.plan, track, work, approvalDigest)
			if err != nil {
				return nil, err
			}
			if _, err := a.admit(status); err != nil {
				return nil, err
			}
			result[work.ID] = status
		}
	}
	return result, nil
}

func (a *Actions) baselineChanges(statuses map[string]Status) []RepositoryChange {
	changes := []RepositoryChange{{Path: ReleasePlanPath(a.plan), Bytes: a.plan.Bytes()}}
	for _, track := range a.plan.Metadata().Tracks {
		for _, work := range track.Work {
			changes = append(changes, RepositoryChange{Path: WorkStatusPath(a.plan, work.ID), Bytes: statuses[work.ID].Bytes()})
		}
	}
	return changes
}

func (a *Actions) assertRecordRootAbsent(commit string) error {
	metadata := a.plan.Metadata()
	entries, err := a.repository.ListTree(commit)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Path == metadata.RecordRoot || pathContains(metadata.RecordRoot, entry.Path) {
			return recordFail("RECORD_NAMESPACE_EXISTS", "target already contains the Baton release namespace")
		}
	}
	return nil
}

// InstallApprovedPlan installs exact plan bytes and pristine baselines.
func (a *Actions) InstallApprovedPlan(approvalDigest string) (Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	before, err := a.captureHeads()
	if err != nil {
		return Receipt{}, err
	}
	metadata := a.plan.Metadata()
	if !before.Target.Exists {
		return Receipt{}, recordFail("REF_NOT_FOUND", "target "+metadata.TargetRef+" does not exist")
	}
	for _, track := range before.Tracks {
		if track.Exists {
			return Receipt{}, recordFail("EXTERNAL_AUTHORITY_REQUIRED", "approved plan installation requires every owner ref to be absent")
		}
	}
	statuses, err := a.baselineStatuses(approvalDigest)
	if err != nil {
		return Receipt{}, err
	}
	if !before.Release.Exists {
		if err := a.assertRecordRootAbsent(before.Target.Head); err != nil {
			return Receipt{}, err
		}
		prepared, err := a.prepareRecord(before.Target.Head, "Install approved Baton plan "+metadata.Release, a.baselineChanges(statuses))
		if err != nil {
			return Receipt{}, err
		}
		operations := []RefOperation{{Kind: "verify", Ref: before.Target.Ref, ExpectedHead: before.Target.Head}, {
			Kind: "create", Ref: before.Release.Ref, NewHead: prepared.Commit,
		}}
		for _, track := range before.Tracks {
			operations = append(operations, RefOperation{Kind: "verify", Ref: track.Ref, ExpectAbsent: true})
		}
		if err := a.repository.AtomicUpdateRefs(operations); err != nil {
			return Receipt{}, err
		}
		after, err := a.captureHeads()
		if err != nil {
			return Receipt{}, err
		}
		if !after.Release.Exists || after.Release.Head != prepared.Commit {
			return Receipt{}, recordFail("ACTION_EFFECT_MISMATCH", "installed release head does not match prepared commit")
		}
		return newReceipt("installApprovedPlan", map[string]any{
			"changed": true, "release_head": prepared.Commit,
			"before": before.receiptValue(), "after": after.receiptValue(),
		})
	}
	parents, err := a.repository.Parents(before.Release.Head)
	if err != nil || len(parents) != 1 {
		return Receipt{}, recordFail("INVALID_RECONCILIATION", "installed release has no exact target parent")
	}
	replay, err := a.prepareRecord(parents[0], "Install approved Baton plan "+metadata.Release, a.baselineChanges(statuses))
	if err != nil {
		return Receipt{}, err
	}
	if replay.Commit != before.Release.Head {
		return Receipt{}, recordFail("EXTERNAL_AUTHORITY_REQUIRED", "release ref already contains a different installation")
	}
	return newReceipt("installApprovedPlan", map[string]any{
		"changed": false, "release_head": before.Release.Head,
		"before": before.receiptValue(), "after": before.receiptValue(),
	})
}

func sameTopology(left, right Metadata) bool {
	if left.Release != right.Release || left.Repository != right.Repository ||
		left.ReleaseRef != right.ReleaseRef || left.RecordRoot != right.RecordRoot ||
		len(left.Tracks) != len(right.Tracks) {
		return false
	}
	for index := range left.Tracks {
		if left.Tracks[index].ID != right.Tracks[index].ID || left.Tracks[index].Ref != right.Tracks[index].Ref ||
			!reflect.DeepEqual(left.Tracks[index].DependsOn, right.Tracks[index].DependsOn) ||
			len(left.Tracks[index].Work) != len(right.Tracks[index].Work) {
			return false
		}
		for workIndex := range left.Tracks[index].Work {
			if left.Tracks[index].Work[workIndex].ID != right.Tracks[index].Work[workIndex].ID {
				return false
			}
		}
	}
	return true
}

// ReboundPristinePlan changes bindings only while the release remains wholly
// pristine and unmaterialized.
func (a *Actions) ReboundPristinePlan(previousPlan Plan, approvalDigest string) (Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := previousPlan.require(); err != nil {
		return Receipt{}, err
	}
	if !sameTopology(previousPlan.Metadata(), a.plan.Metadata()) {
		return Receipt{}, recordFail("EXTERNAL_AUTHORITY_REQUIRED", "plan rebound requires identical release ownership topology")
	}
	before, err := a.captureHeads()
	if err != nil {
		return Receipt{}, err
	}
	if !before.Release.Exists {
		return Receipt{}, recordFail("REF_NOT_FOUND", "previous release is absent")
	}
	for _, track := range before.Tracks {
		if track.Exists {
			return Receipt{}, recordFail("EXTERNAL_AUTHORITY_REQUIRED", "materialized plans require a new release identity")
		}
	}
	installedBytes, err := a.repository.ReadBlob(before.Release.Head, ReleasePlanPath(a.plan))
	if err != nil {
		return Receipt{}, err
	}
	installedPlan, err := ParsePlan(installedBytes)
	if err != nil {
		return Receipt{}, err
	}
	statuses, err := a.baselineStatuses(approvalDigest)
	if err != nil {
		return Receipt{}, err
	}
	if installedPlan.Digest() == a.plan.Digest() {
		parents, err := a.repository.Parents(before.Release.Head)
		if err != nil || len(parents) != 1 {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "rebound retry has no exact previous-plan parent")
		}
		parentPlanBytes, err := a.repository.ReadBlob(parents[0], ReleasePlanPath(previousPlan))
		if err != nil {
			return Receipt{}, err
		}
		parentPlan, err := ParsePlan(parentPlanBytes)
		if err != nil || parentPlan.Digest() != previousPlan.Digest() {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "rebound retry parent is not the supplied previous plan")
		}
		currentReader, err := NewReader(a.plan, a.repository)
		if err != nil {
			return Receipt{}, err
		}
		current, err := currentReader.Capture()
		if err != nil {
			return Receipt{}, err
		}
		for _, track := range a.plan.Metadata().Tracks {
			for _, work := range track.Work {
				selected, err := current.SelectWork(work.ID)
				if err != nil || selected.Source != "baseline" || !statusesEqual(selected.Status, statuses[work.ID]) {
					return Receipt{}, recordFail("EXTERNAL_AUTHORITY_REQUIRED", "rebound plan no longer has exact pristine baselines")
				}
			}
		}
		replay, err := a.prepareRecord(parents[0], "Rebound pristine Baton plan "+a.plan.Metadata().Release, a.baselineChanges(statuses))
		if err != nil {
			return Receipt{}, err
		}
		if replay.Commit != before.Release.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "rebound retry does not match the exact Baton effect")
		}
		return newReceipt("reboundPristinePlan", map[string]any{
			"changed": false, "previous_plan_digest": previousPlan.Digest(), "plan_digest": a.plan.Digest(),
			"release_head": before.Release.Head, "before": before.receiptValue(), "after": before.receiptValue(),
		})
	}
	if installedPlan.Digest() != previousPlan.Digest() {
		return Receipt{}, recordFail("STALE_BINDING", "release does not contain the exact previous plan")
	}
	reader, err := NewReader(previousPlan, a.repository)
	if err != nil {
		return Receipt{}, err
	}
	snapshot, err := reader.Capture()
	if err != nil {
		return Receipt{}, err
	}
	for _, track := range previousPlan.Metadata().Tracks {
		for _, work := range track.Work {
			selected, err := snapshot.SelectWork(work.ID)
			if err != nil || selected.Source != "baseline" {
				return Receipt{}, recordFail("EXTERNAL_AUTHORITY_REQUIRED", "previous plan is no longer pristine")
			}
		}
	}
	for _, track := range previousPlan.Metadata().Tracks {
		for _, work := range track.Work {
			selected, err := snapshot.SelectWork(work.ID)
			if err != nil {
				return Receipt{}, err
			}
			next := statuses[work.ID]
			if err := ValidateTransition(selected.Status, next, Rebound); err != nil {
				return Receipt{}, err
			}
			if _, err := a.admit(selected.Status); err != nil {
				return Receipt{}, err
			}
			if _, err := a.admit(next); err != nil {
				return Receipt{}, err
			}
		}
	}
	prepared, err := a.prepareRecord(before.Release.Head, "Rebound pristine Baton plan "+a.plan.Metadata().Release, a.baselineChanges(statuses))
	if err != nil {
		return Receipt{}, err
	}
	operations := []RefOperation{{Kind: "update", Ref: before.Release.Ref, NewHead: prepared.Commit, ExpectedHead: before.Release.Head}}
	operations = append(operations, a.verifyOperations(before, map[string]bool{before.Release.Ref: true})...)
	if err := a.repository.AtomicUpdateRefs(operations); err != nil {
		return Receipt{}, err
	}
	after, err := a.captureHeads()
	if err != nil {
		return Receipt{}, err
	}
	return newReceipt("reboundPristinePlan", map[string]any{
		"changed": true, "previous_plan_digest": previousPlan.Digest(), "plan_digest": a.plan.Digest(),
		"release_head": prepared.Commit, "before": before.receiptValue(), "after": after.receiptValue(),
	})
}

// Handoffs contains exact immutable bytes introduced by one transition.
type Handoffs struct {
	Design []byte
	Proof  []byte
}

type RecordTransitionInput struct {
	Scope    string
	WorkID   string
	Result   TransitionResult
	Next     Status
	Handoffs Handoffs
}

func resultMatchesStatus(result TransitionResult, status StatusView) bool {
	projection := projection(status)
	switch result {
	case NoVerdict:
		return projection == "verify/ready/verifier" && status.Outcome == "none"
	case DesignWritten:
		return projection == "design/ready/captain" && status.Outcome == "none"
	case Proceed:
		return projection == "implement/ready/implementer" && status.Outcome == "proceed"
	case Revise:
		return projection == "design/ready/implementer" && status.Outcome == "revise"
	case Escalate:
		return projection == "design/blocked/planner" && status.Outcome == "escalate"
	case Implemented:
		return projection == "verify/ready/verifier" && status.Outcome == "none"
	case Pass:
		return projection == "merge/ready/merge" && status.Outcome == "pass"
	case Blocked:
		return projection == "verify/blocked/planner" && status.Outcome == "blocked"
	case Fail:
		return (status.Kind == "assembly" && projection == "verify/ready/planner" && status.Outcome == "fail") ||
			(status.Kind == "work" && projection == "implement/ready/implementer" && status.Outcome == "fail")
	default:
		return false
	}
}

func (a *Actions) transitionChanges(previous, next Status, handoffs Handoffs, scope, workID string) ([]RepositoryChange, error) {
	before, after := previous.View(), next.View()
	var result []RepositoryChange
	if after.Design != nil && (before.Design == nil || before.Design.Digest != after.Design.Digest) {
		if scope != "work" || handoffs.Design == nil || DigestBytes(handoffs.Design) != after.Design.Digest {
			return nil, recordFail("INVALID_HANDOFF", "changed design requires exact digest-bound bytes")
		}
		result = append(result, RepositoryChange{Path: WorkDesignPath(a.plan, workID), Bytes: append([]byte(nil), handoffs.Design...)})
	} else if handoffs.Design != nil {
		return nil, recordFail("INVALID_HANDOFF", "unchanged transition cannot introduce design bytes")
	}
	if after.Proof != nil && (before.Proof == nil || before.Proof.Digest != after.Proof.Digest) {
		if handoffs.Proof == nil || DigestBytes(handoffs.Proof) != after.Proof.Digest {
			return nil, recordFail("INVALID_HANDOFF", "changed proof requires exact digest-bound bytes")
		}
		proofPath := AssemblyProofPath(a.plan)
		if scope == "work" {
			proofPath = WorkProofPath(a.plan, workID)
		}
		result = append(result, RepositoryChange{Path: proofPath, Bytes: append([]byte(nil), handoffs.Proof...)})
	} else if handoffs.Proof != nil {
		return nil, recordFail("INVALID_HANDOFF", "unchanged transition cannot introduce proof bytes")
	}
	statusPath := AssemblyStatusPath(a.plan)
	if scope == "work" {
		statusPath = WorkStatusPath(a.plan, workID)
	}
	result = append(result, RepositoryChange{Path: statusPath, Bytes: next.Bytes()})
	return result, nil
}

// RecordTransition records one ordinary work or assembly lifecycle result.
func (a *Actions) RecordTransition(input RecordTransitionInput) (Receipt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if input.Scope != "work" && input.Scope != "assembly" {
		return Receipt{}, recordFail("INVALID_ACTION_INPUT", "record transition scope must be work or assembly")
	}
	if input.Scope == "work" && input.WorkID == "" || input.Scope == "assembly" && input.WorkID != "" ||
		input.Result == Materialize || input.Result == Rebound || input.Result == Merged || !validTransitionResult(input.Result) {
		return Receipt{}, recordFail("INVALID_ACTION_INPUT", "recordTransition accepts only ordinary work/assembly results")
	}
	if _, err := input.Next.require(); err != nil {
		return Receipt{}, err
	}
	next := Status{admission: &statusAdmission{raw: input.Next.Bytes(), view: input.Next.View()}}
	handoffs := Handoffs{Design: append([]byte(nil), input.Handoffs.Design...), Proof: append([]byte(nil), input.Handoffs.Proof...)}
	reader, err := NewReader(a.plan, a.repository)
	if err != nil {
		return Receipt{}, err
	}
	snapshot, err := reader.Capture()
	if err != nil {
		return Receipt{}, err
	}
	before, err := a.captureHeads()
	if err != nil {
		return Receipt{}, err
	}
	var selected Selection
	if input.Scope == "work" {
		if _, _, found := a.plan.FindWork(input.WorkID); !found {
			return Receipt{}, recordFail("UNKNOWN_WORK", "plan has no work "+input.WorkID)
		}
		selected, err = snapshot.SelectWork(input.WorkID)
		if err != nil {
			return Receipt{}, err
		}
	} else {
		var exists bool
		selected, exists, err = snapshot.SelectAssembly()
		if err != nil {
			return Receipt{}, err
		}
		if !exists {
			return Receipt{}, recordFail("AUTHORITATIVE_STATUS_MISSING", "assembly has not been prepared")
		}
	}
	previous := selected.Status
	if input.Scope == "assembly" {
		if err := a.validateAssembly(previous, selected.Head, before, snapshot); err != nil {
			return Receipt{}, err
		}
	}
	if statusesEqual(previous, next) {
		if !resultMatchesStatus(input.Result, previous.View()) {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "result does not match durable post-state")
		}
		if input.Result == NoVerdict {
			previousAdmission, err := a.admit(previous)
			if err != nil {
				return Receipt{}, err
			}
			if err := requireEvidenceAdmission(previous, previousAdmission, a.profile); err != nil {
				return Receipt{}, err
			}
			return newReceipt("recordTransition", map[string]any{
				"changed": false, "scope": input.Scope, "work_id": nullableWorkID(input),
				"result": string(input.Result), "authority_ref": selected.Ref, "commit": selected.Head,
				"before": before.receiptValue(), "after": before.receiptValue(),
			})
		}
		parents, err := a.repository.Parents(selected.Head)
		if err != nil || len(parents) != 1 {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "durable result has no exact record predecessor")
		}
		statusPath := AssemblyStatusPath(a.plan)
		if input.Scope == "work" {
			statusPath = WorkStatusPath(a.plan, input.WorkID)
		}
		predecessor, err := readStatusAt(a.repository, parents[0], statusPath, a.plan)
		if err != nil {
			return Receipt{}, err
		}
		if err := ValidateTransition(predecessor, previous, input.Result); err != nil {
			return Receipt{}, err
		}
		replayChanges, err := a.transitionChanges(predecessor, previous, handoffs, input.Scope, input.WorkID)
		if err != nil {
			// Retry callers may omit handoffs; read exact durable bytes.
			replayChanges, err = a.transitionChangesFromRepository(predecessor, previous, input.Scope, input.WorkID, selected.Head)
			if err != nil {
				return Receipt{}, err
			}
		}
		replay, err := a.prepareRecord(parents[0], fmt.Sprintf("Record %s %s %s", input.Scope, transitionIdentity(input, a.plan), input.Result), replayChanges)
		if err != nil {
			return Receipt{}, err
		}
		if replay.Commit != selected.Head {
			return Receipt{}, recordFail("INVALID_RECONCILIATION", "result does not match the exact Baton effect")
		}
		return newReceipt("recordTransition", map[string]any{
			"changed": false, "scope": input.Scope, "work_id": nullableWorkID(input),
			"result": string(input.Result), "authority_ref": selected.Ref, "commit": selected.Head,
			"before": before.receiptValue(), "after": before.receiptValue(),
		})
	}
	if input.Scope == "work" {
		if _, err := snapshot.MayAdvanceWork(input.WorkID); err != nil {
			return Receipt{}, err
		}
	}
	if err := ValidateTransition(previous, next, input.Result); err != nil {
		return Receipt{}, err
	}
	previousEvidence, err := a.admit(previous)
	if err != nil {
		return Receipt{}, err
	}
	nextEvidence, err := a.admit(next)
	if err != nil {
		return Receipt{}, err
	}
	if err := requireEvidenceAdmission(previous, previousEvidence, a.profile); err != nil {
		return Receipt{}, err
	}
	if err := requireEvidenceAdmission(next, nextEvidence, a.profile); err != nil {
		return Receipt{}, err
	}
	changes, err := a.transitionChanges(previous, next, handoffs, input.Scope, input.WorkID)
	if err != nil {
		return Receipt{}, err
	}
	prepared, err := a.prepareRecord(selected.Head, fmt.Sprintf("Record %s %s %s", input.Scope, transitionIdentity(input, a.plan), input.Result), changes)
	if err != nil {
		return Receipt{}, err
	}
	if err := validateStatusHandoffs(a.repository, a.plan, prepared.Commit, next); err != nil {
		return Receipt{}, err
	}
	if input.Scope == "work" && next.View().Proof != nil {
		if _, err := ValidateWorkCandidate(a.plan, a.repository, input.WorkID, next, prepared.Commit, a.inertness); err != nil {
			return Receipt{}, err
		}
	} else if input.Scope == "assembly" {
		if err := a.validateAssembly(next, prepared.Commit, before, snapshot); err != nil {
			return Receipt{}, err
		}
	}
	operations := []RefOperation{{Kind: "update", Ref: selected.Ref, NewHead: prepared.Commit, ExpectedHead: selected.Head}}
	operations = append(operations, a.verifyOperations(before, map[string]bool{selected.Ref: true})...)
	if err := a.repository.AtomicUpdateRefs(operations); err != nil {
		return Receipt{}, err
	}
	after, err := a.captureHeads()
	if err != nil {
		return Receipt{}, err
	}
	if ref := findCaptured(after, selected.Ref); !ref.Exists || ref.Head != prepared.Commit {
		return Receipt{}, recordFail("ACTION_EFFECT_MISMATCH", "record transition did not install its exact commit")
	}
	return newReceipt("recordTransition", map[string]any{
		"changed": true, "scope": input.Scope, "work_id": nullableWorkID(input),
		"result": string(input.Result), "authority_ref": selected.Ref, "commit": prepared.Commit,
		"before": before.receiptValue(), "after": after.receiptValue(),
	})
}

func (a *Actions) transitionChangesFromRepository(previous, next Status, scope, workID, head string) ([]RepositoryChange, error) {
	before, after := previous.View(), next.View()
	handoffs := Handoffs{}
	if after.Design != nil && (before.Design == nil || before.Design.Digest != after.Design.Digest) {
		body, err := a.repository.ReadBlob(head, WorkDesignPath(a.plan, workID))
		if err != nil {
			return nil, err
		}
		handoffs.Design = body
	}
	if after.Proof != nil && (before.Proof == nil || before.Proof.Digest != after.Proof.Digest) {
		proofPath := AssemblyProofPath(a.plan)
		if scope == "work" {
			proofPath = WorkProofPath(a.plan, workID)
		}
		body, err := a.repository.ReadBlob(head, proofPath)
		if err != nil {
			return nil, err
		}
		handoffs.Proof = body
	}
	return a.transitionChanges(previous, next, handoffs, scope, workID)
}

func transitionIdentity(input RecordTransitionInput, plan Plan) string {
	if input.Scope == "work" {
		return input.WorkID
	}
	return plan.Metadata().Release
}

func nullableWorkID(input RecordTransitionInput) any {
	if input.Scope == "work" {
		return input.WorkID
	}
	return nil
}

func findCaptured(heads actionHeads, ref string) CapturedRef {
	for _, value := range append([]CapturedRef{heads.Target, heads.Release}, heads.Tracks...) {
		if value.Ref == ref {
			return value
		}
	}
	return CapturedRef{}
}
