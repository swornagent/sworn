package baton

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Reader captures one plan-bound, read-only authority projection.
type Reader struct {
	plan       Plan
	repository Repository
}

func NewReader(plan Plan, repository Repository) (Reader, error) {
	if _, err := plan.require(); err != nil {
		return Reader{}, err
	}
	if repository == nil || repository.Root() == "" ||
		(repository.ObjectFormat() != "sha1" && repository.ObjectFormat() != "sha256") {
		return Reader{}, recordFail("INVALID_REPOSITORY", "reader requires one admitted repository")
	}
	return Reader{plan: plan, repository: repository}, nil
}

type selectedRecord struct {
	source string
	ref    string
	head   string
	status Status
}

// Selection is a detached selected status bound to one captured ref/OID.
type Selection struct {
	Source string
	Ref    string
	Head   string
	Status Status
}

// Snapshot contains only immutable Baton domain values. It does not retain a
// repository callback or evidence admission.
type Snapshot struct {
	planDigest string
	metadata   Metadata
	refs       []CapturedRef
	selected   map[string]selectedRecord
	assembly   *selectedRecord
}

// Capture captures all plan refs once, then reads every projection at those
// exact OIDs. Ref movement after capture cannot alter the returned snapshot.
func (r Reader) Capture() (Snapshot, error) {
	metadata := r.plan.Metadata()
	refs := []string{metadata.TargetRef, metadata.ReleaseRef}
	for _, track := range metadata.Tracks {
		refs = append(refs, track.Ref)
	}
	captured, err := r.repository.CaptureHeadRefs(refs)
	if err != nil {
		return Snapshot{}, err
	}
	if len(captured) != len(refs) || captured[0].Ref != metadata.TargetRef || captured[1].Ref != metadata.ReleaseRef {
		return Snapshot{}, recordFail("INVALID_REF_SNAPSHOT", "repository did not return the exact ordered plan refs")
	}
	if !captured[0].Exists {
		return Snapshot{}, recordFail("REF_NOT_FOUND", "target ref is absent")
	}
	if !captured[1].Exists {
		return Snapshot{}, recordFail("REF_NOT_FOUND", "release ref is absent")
	}
	for index, expected := range refs {
		if captured[index].Ref != expected {
			return Snapshot{}, recordFail("INVALID_REF_SNAPSHOT", "captured ref order changed")
		}
	}
	releaseStatuses, assembly, err := r.readReleaseProjection(captured[1].Head)
	if err != nil {
		return Snapshot{}, err
	}
	selected := make(map[string]selectedRecord)
	for trackIndex, track := range metadata.Tracks {
		owner := captured[trackIndex+2]
		if !owner.Exists {
			for _, work := range track.Work {
				status := releaseStatuses[work.ID]
				if err := validatePristineBaseline(status, metadata.ReleaseRef, work.ID); err != nil {
					return Snapshot{}, err
				}
				selected[work.ID] = selectedRecord{source: "baseline", ref: captured[1].Ref, head: captured[1].Head, status: status}
			}
			continue
		}
		ownerStatuses, err := r.readTrackProjection(owner.Head, track)
		if err != nil {
			return Snapshot{}, err
		}
		var materialization *Materialization
		for _, work := range track.Work {
			status := ownerStatuses[work.ID]
			view := status.View()
			if view.AuthorityRef != track.Ref || view.Materialization == nil {
				return Snapshot{}, recordFail("INVALID_MATERIALIZATION", "materialised work "+work.ID+" must use owning-track authority")
			}
			if materialization == nil {
				copyValue := *view.Materialization
				copyValue.Dependencies = append([]MaterializationDependency(nil), view.Materialization.Dependencies...)
				materialization = &copyValue
			} else if !reflect.DeepEqual(*materialization, *view.Materialization) {
				return Snapshot{}, recordFail("INVALID_MATERIALIZATION", "track "+track.ID+" does not share one materialization")
			}
		}
		if materialization == nil {
			return Snapshot{}, recordFail("INVALID_MATERIALIZATION", "track "+track.ID+" has no materialization")
		}
		containedBase, err := r.repository.IsAncestor(materialization.BaseCommit, owner.Head)
		if err != nil {
			return Snapshot{}, err
		}
		if !containedBase {
			return Snapshot{}, recordFail("INVALID_MATERIALIZATION", "owner does not descend from its captured materialization base")
		}
		if err := r.validateMaterialization(track, *materialization); err != nil {
			return Snapshot{}, err
		}
		if err := r.validateOwnerMarker(track, *materialization, owner.Head, captured[1].Head); err != nil {
			return Snapshot{}, err
		}
		ownerContained, err := r.repository.IsAncestor(owner.Head, captured[1].Head)
		if err != nil {
			return Snapshot{}, err
		}
		for _, work := range track.Work {
			releaseStatus := releaseStatuses[work.ID]
			releaseView := releaseStatus.View()
			if releaseView.Stage == "merge" && releaseView.Status == "complete" &&
				releaseView.Merge != nil && releaseView.Merge.Scope == "track" &&
				releaseView.AuthorityRef == metadata.ReleaseRef {
				if releaseView.Merge.FrozenTrackHead == nil || *releaseView.Merge.FrozenTrackHead != owner.Head || !ownerContained {
					return Snapshot{}, recordFail("INVALID_AUTHORITY_TRANSFER", "release status for "+work.ID+" does not prove exact track composition")
				}
				selected[work.ID] = selectedRecord{source: "composed", ref: captured[1].Ref, head: captured[1].Head, status: releaseStatus}
			} else {
				selected[work.ID] = selectedRecord{source: "owner", ref: owner.Ref, head: owner.Head, status: ownerStatuses[work.ID]}
			}
		}
	}
	result := Snapshot{
		planDigest: r.plan.Digest(),
		metadata:   metadata,
		refs:       append([]CapturedRef(nil), captured...),
		selected:   selected,
	}
	if assembly.admission != nil {
		record := selectedRecord{source: "release", ref: captured[1].Ref, head: captured[1].Head, status: assembly}
		result.assembly = &record
	}
	return result, nil
}

func (r Reader) readReleaseProjection(head string) (map[string]Status, Status, error) {
	metadata := r.plan.Metadata()
	paths, err := r.regularTreePaths(head)
	if err != nil {
		return nil, Status{}, err
	}
	planPath := ReleasePlanPath(r.plan)
	if !paths[planPath] {
		return nil, Status{}, recordFail("PLAN_NOT_FOUND", "release projection lacks plan.md")
	}
	batchPaths := []string{planPath}
	for _, track := range metadata.Tracks {
		for _, work := range track.Work {
			statusPath := WorkStatusPath(r.plan, work.ID)
			if !paths[statusPath] {
				return nil, Status{}, recordFail("AUTHORITATIVE_STATUS_MISSING", "release snapshot lacks "+work.ID)
			}
			batchPaths = append(batchPaths, statusPath)
		}
	}
	assemblyPath := AssemblyStatusPath(r.plan)
	if paths[assemblyPath] {
		batchPaths = append(batchPaths, assemblyPath)
	}
	batch, err := r.repository.ReadBlobs(head, batchPaths)
	if err != nil {
		return nil, Status{}, err
	}
	rawPlan := batch[planPath]
	capturedPlan, err := ParsePlan(rawPlan)
	if err != nil {
		return nil, Status{}, err
	}
	if capturedPlan.Digest() != r.plan.Digest() {
		return nil, Status{}, recordFail("STALE_BINDING", "release projection contains a different plan")
	}
	statuses := make(map[string]Status)
	for _, track := range metadata.Tracks {
		for _, work := range track.Work {
			statusPath := WorkStatusPath(r.plan, work.ID)
			body := batch[statusPath]
			status, err := ParseStatus(body, StatusExpectation{PlanDigest: r.plan.Digest(), ApprovalRef: metadata.ApprovalRef})
			if err != nil {
				return nil, Status{}, err
			}
			if err := validateWorkIdentity(status, metadata, track, work); err != nil {
				return nil, Status{}, err
			}
			statuses[work.ID] = status
		}
	}
	var assembly Status
	if paths[assemblyPath] {
		body := batch[assemblyPath]
		assembly, err = ParseStatus(body, StatusExpectation{PlanDigest: r.plan.Digest(), ApprovalRef: metadata.ApprovalRef})
		if err != nil {
			return nil, Status{}, err
		}
		view := assembly.View()
		if view.Kind != "assembly" || view.Release != metadata.Release || view.OwnerRef != metadata.ReleaseRef ||
			view.TargetRef != metadata.TargetRef {
			return nil, Status{}, recordFail("STATUS_IDENTITY_MISMATCH", "assembly status does not match the plan")
		}
	}
	return statuses, assembly, nil
}

func (r Reader) readTrackProjection(head string, track Track) (map[string]Status, error) {
	paths, err := r.regularTreePaths(head)
	if err != nil {
		return nil, err
	}
	metadata := r.plan.Metadata()
	statuses := make(map[string]Status)
	statusPaths := make([]string, 0, len(track.Work))
	for _, work := range track.Work {
		statusPath := WorkStatusPath(r.plan, work.ID)
		if !paths[statusPath] {
			return nil, recordFail("AUTHORITATIVE_STATUS_MISSING", "owner snapshot lacks "+work.ID)
		}
		statusPaths = append(statusPaths, statusPath)
	}
	batch, err := r.repository.ReadBlobs(head, statusPaths)
	if err != nil {
		return nil, err
	}
	for _, work := range track.Work {
		statusPath := WorkStatusPath(r.plan, work.ID)
		body := batch[statusPath]
		status, err := ParseStatus(body, StatusExpectation{PlanDigest: r.plan.Digest(), ApprovalRef: metadata.ApprovalRef})
		if err != nil {
			return nil, err
		}
		if err := validateWorkIdentity(status, metadata, track, work); err != nil {
			return nil, err
		}
		statuses[work.ID] = status
	}
	return statuses, nil
}

func (r Reader) regularTreePaths(head string) (map[string]bool, error) {
	entries, err := r.repository.ListTree(head)
	if err != nil {
		return nil, err
	}
	metadata := r.plan.Metadata()
	components := strings.Split(metadata.RecordRoot, "/")
	prefix := ""
	for _, component := range components {
		if prefix == "" {
			prefix = component
		} else {
			prefix += "/" + component
		}
		for _, entry := range entries {
			if entry.Path == prefix && (entry.Mode != "040000" || entry.Type != "tree") {
				return nil, recordFail("NONCANONICAL_RECORD_ROOT", "record-root component is not a tree")
			}
		}
	}
	result := make(map[string]bool)
	for _, entry := range entries {
		if entry.Path == metadata.RecordRoot || strings.HasPrefix(entry.Path, metadata.RecordRoot+"/") {
			if entry.Mode != "100644" || entry.Type != "blob" {
				return nil, recordFail("NONREGULAR_RECORD", "Baton record "+entry.Path+" is not a regular file")
			}
			result[entry.Path] = true
		}
	}
	return result, nil
}

func (r Reader) validateMaterialization(track Track, materialization Materialization) error {
	if len(materialization.Dependencies) != len(track.DependsOn) {
		return recordFail("INVALID_MATERIALIZATION", "track "+track.ID+" does not bind every dependency exactly once")
	}
	for index, dependencyID := range track.DependsOn {
		dependency := materialization.Dependencies[index]
		if dependency.TrackID != dependencyID {
			return recordFail("INVALID_MATERIALIZATION", "materialization dependency order is not exact")
		}
		contained, err := r.repository.IsAncestor(dependency.FrozenHead, materialization.BaseCommit)
		if err != nil {
			return err
		}
		if !contained {
			return recordFail("UNMET_TRACK_DEPENDENCY", "materialization base does not contain dependency "+dependencyID)
		}
		dependencyTrack, ok := r.plan.FindTrack(dependencyID)
		if !ok {
			return recordFail("UNKNOWN_TRACK", "plan has no track "+dependencyID)
		}
		for _, work := range dependencyTrack.Work {
			status, err := readStatusAt(r.repository, materialization.BaseCommit, WorkStatusPath(r.plan, work.ID), r.plan)
			if err != nil {
				return err
			}
			view := status.View()
			if view.Stage != "merge" || view.Status != "complete" || view.AuthorityRef != r.plan.Metadata().ReleaseRef ||
				view.Merge == nil || view.Merge.FrozenTrackHead == nil || *view.Merge.FrozenTrackHead != dependency.FrozenHead {
				return recordFail("UNMET_TRACK_DEPENDENCY", "dependency "+dependencyID+"/"+work.ID+" lacks exact transfer")
			}
		}
	}
	return nil
}

func (r Reader) validateOwnerMarker(track Track, materialization Materialization, ownerHead, releaseHead string) error {
	history, err := r.repository.FirstParentHistory(materialization.BaseCommit, ownerHead)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		return recordFail("INVALID_MATERIALIZATION", "owner has no collective materialization marker")
	}
	marker := history[0]
	parents, err := r.repository.Parents(marker)
	if err != nil {
		return err
	}
	if len(parents) != 1 || parents[0] != materialization.BaseCommit {
		return recordFail("INVALID_MATERIALIZATION", "materialization marker has a foreign parent")
	}
	changed, err := r.repository.ChangedPaths(materialization.BaseCommit, marker)
	if err != nil {
		return err
	}
	expected := make([]string, len(track.Work))
	for index, work := range track.Work {
		expected[index] = WorkStatusPath(r.plan, work.ID)
	}
	sort.Strings(changed)
	sort.Strings(expected)
	if !reflect.DeepEqual(changed, expected) {
		return recordFail("INVALID_MATERIALIZATION", "materialization marker is not one collective status-only commit")
	}
	for _, work := range track.Work {
		before, err := readStatusAt(r.repository, materialization.BaseCommit, WorkStatusPath(r.plan, work.ID), r.plan)
		if err != nil {
			return err
		}
		after, err := readStatusAt(r.repository, marker, WorkStatusPath(r.plan, work.ID), r.plan)
		if err != nil {
			return err
		}
		if err := ValidateTransition(before, after, Materialize); err != nil {
			return err
		}
	}
	retained, err := r.repository.IsAncestor(marker, releaseHead)
	if err != nil {
		return err
	}
	if !retained {
		return recordFail("ERASED_MATERIALIZATION", "release history no longer retains the owner marker")
	}
	return nil
}

func validateWorkIdentity(status Status, metadata Metadata, track Track, work Work) error {
	view := status.View()
	if view.Kind != "work" || view.WorkID == nil || *view.WorkID != work.ID ||
		view.TrackID == nil || *view.TrackID != track.ID || view.Release != metadata.Release ||
		view.OwnerRef != track.Ref || view.TargetRef != metadata.TargetRef {
		return recordFail("STATUS_IDENTITY_MISMATCH", "status identity does not match planned work "+work.ID)
	}
	return nil
}

func validatePristineBaseline(status Status, releaseRef, workID string) error {
	view := status.View()
	if status.admission == nil || view.AuthorityRef != releaseRef || projection(view) != "design/ready/implementer" ||
		view.Outcome != "none" || view.Materialization != nil || view.Blocker != nil || view.Design != nil ||
		view.Captain != nil || view.Proof != nil || view.Verification != nil || view.Merge != nil {
		return recordFail("INVALID_BASELINE", "work "+workID+" without a captured owner must have one pristine release baseline")
	}
	return nil
}

func (s Snapshot) valid() bool {
	return s.planDigest != "" && s.metadata.Release != "" && s.selected != nil
}

func (s Snapshot) SelectWork(workID string) (Selection, error) {
	if !s.valid() {
		return Selection{}, recordFail("INVALID_RECORD_SNAPSHOT", "selection requires one captured structural-authority snapshot")
	}
	value, ok := s.selected[workID]
	if !ok {
		return Selection{}, recordFail("UNKNOWN_WORK", "plan has no work "+workID)
	}
	return Selection{Source: value.source, Ref: value.ref, Head: value.head, Status: value.status}, nil
}

func (s Snapshot) SelectAssembly() (Selection, bool, error) {
	if !s.valid() {
		return Selection{}, false, recordFail("INVALID_RECORD_SNAPSHOT", "selection requires one captured structural-authority snapshot")
	}
	if s.assembly == nil {
		return Selection{}, false, nil
	}
	return Selection{Source: s.assembly.source, Ref: s.assembly.ref, Head: s.assembly.head, Status: s.assembly.status}, true, nil
}

func (s Snapshot) statusesForTrack(trackID string) (Track, map[string]Status, error) {
	if !s.valid() {
		return Track{}, nil, recordFail("INVALID_RECORD_SNAPSHOT", "snapshot is not admitted")
	}
	var selectedTrack Track
	found := false
	for _, track := range s.metadata.Tracks {
		if track.ID == trackID {
			selectedTrack = track
			found = true
			break
		}
	}
	if !found {
		return Track{}, nil, recordFail("UNKNOWN_TRACK", "plan has no track "+trackID)
	}
	statuses := make(map[string]Status)
	for _, work := range selectedTrack.Work {
		statuses[work.ID] = s.selected[work.ID].status
	}
	return selectedTrack, statuses, nil
}

func (s Snapshot) NextWorkForTrack(trackID string) (string, bool, error) {
	track, statuses, err := s.statusesForTrack(trackID)
	if err != nil {
		return "", false, err
	}
	completed := 0
	for _, work := range track.Work {
		view := statuses[work.ID].View()
		if view.Stage == "merge" && view.Status == "complete" {
			completed++
		}
	}
	if completed > 0 && completed != len(track.Work) {
		return "", false, recordFail("PARTIAL_TRACK_TRANSFER", "track "+trackID+" has only some work transferred to release-wt")
	}
	for _, work := range track.Work {
		view := statuses[work.ID].View()
		passed := view.Stage == "merge" && view.Status == "ready" && view.NextRole == "merge" &&
			view.Outcome == "pass" && view.AuthorityRef == track.Ref
		transferred := view.Stage == "merge" && view.Status == "complete"
		if !passed && !transferred {
			return work.ID, true, nil
		}
	}
	return "", false, nil
}

func (s Snapshot) MayAdvanceWork(workID string) (bool, error) {
	var owner Track
	var work Work
	found := false
	for _, track := range s.metadata.Tracks {
		for _, candidate := range track.Work {
			if candidate.ID == workID {
				owner, work, found = track, candidate, true
			}
		}
	}
	if !found {
		return false, recordFail("UNKNOWN_WORK", "plan has no work "+workID)
	}
	next, exists, err := s.NextWorkForTrack(owner.ID)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, recordFail("TRACK_WORK_COMPLETE", "track "+owner.ID+" has no remaining work to implement")
	}
	if next != workID {
		return false, recordFail("OUT_OF_ORDER_WORK", "track "+owner.ID+" must advance "+next+" before "+workID)
	}
	for _, dependency := range work.DependsOn {
		status := s.selected[dependency].status.View()
		if status.Stage != "merge" || status.Status != "complete" {
			return false, recordFail("UNMET_WORK_DEPENDENCY", "work "+workID+" depends on incomplete "+dependency)
		}
	}
	return true, nil
}

func (s Snapshot) MaterializationFor(trackID string) (Materialization, bool, error) {
	track, statuses, err := s.statusesForTrack(trackID)
	if err != nil {
		return Materialization{}, false, err
	}
	var result *Materialization
	for _, work := range track.Work {
		materialization := statuses[work.ID].View().Materialization
		if materialization == nil {
			if result != nil {
				return Materialization{}, false, recordFail("INVALID_MATERIALIZATION", "track has a partial materialization")
			}
			continue
		}
		if result == nil {
			copyValue := *materialization
			copyValue.Dependencies = append([]MaterializationDependency(nil), materialization.Dependencies...)
			result = &copyValue
		} else if !reflect.DeepEqual(*result, *materialization) {
			return Materialization{}, false, recordFail("INVALID_MATERIALIZATION", "track does not share one materialization")
		}
	}
	if result == nil {
		return Materialization{}, false, nil
	}
	return *result, true, nil
}

func (s Snapshot) TrackReadyForComposition(trackID string) (bool, error) {
	track, statuses, err := s.statusesForTrack(trackID)
	if err != nil {
		return false, err
	}
	for _, work := range track.Work {
		view := statuses[work.ID].View()
		if view.Stage != "merge" || view.Status != "ready" || view.NextRole != "merge" ||
			view.Outcome != "pass" || view.AuthorityRef != track.Ref {
			return false, nil
		}
	}
	return true, nil
}

func (s Snapshot) ReleaseReadyForAssembly() (bool, error) {
	if !s.valid() {
		return false, recordFail("INVALID_RECORD_SNAPSHOT", "snapshot is not admitted")
	}
	for _, track := range s.metadata.Tracks {
		for _, work := range track.Work {
			selected := s.selected[work.ID]
			view := selected.status.View()
			if selected.source != "composed" || view.Stage != "merge" || view.Status != "complete" ||
				view.AuthorityRef != s.metadata.ReleaseRef || view.Merge == nil ||
				view.Merge.FrozenTrackHead == nil {
				return false, nil
			}
		}
	}
	return true, nil
}

// CapturedRefs returns a detached ordered copy for diagnostics only.
func (s Snapshot) CapturedRefs() []CapturedRef {
	result := append([]CapturedRef(nil), s.refs...)
	sort.SliceStable(result, func(i, j int) bool { return i < j })
	return result
}

func (s Snapshot) String() string {
	if !s.valid() {
		return "baton snapshot <invalid>"
	}
	return fmt.Sprintf("baton snapshot %s at %d refs", s.metadata.Release, len(s.refs))
}
