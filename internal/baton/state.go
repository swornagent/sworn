package baton

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

const maxPlanRevisions = 256

type PlanHistory struct {
	OID      string
	Revision int64
	Approval ReceiptEntry
	Plan     Plan
}

type PlanState struct {
	OID         string
	Digest      string
	Metadata    Metadata
	Approval    ReceiptEntry
	ApprovalOID string
	TargetStale bool
	History     []PlanHistory
}

type SliceLocation struct {
	Track Track
	Slice Slice
}

type SliceHistory struct {
	Entries        []ReceiptEntry
	MaximumAttempt int64
}

type AttemptNumbers struct {
	Design    int64
	Candidate int64
}

type SliceState struct {
	Location       SliceLocation
	History        SliceHistory
	InputPins      map[string]string
	NextAttempts   AttemptNumbers
	StaleReason    string
	Stage          string
	Status         string
	NextRole       string
	Outcome        string
	Attempt        int64
	CurrentReceipt *ReceiptEntry
	Candidate      *ReceiptEntry
	Pass           *ReceiptEntry
	Retained       bool
}

type TrackState struct {
	ID        string
	DependsOn []string
	Ref       string
	Head      string
	Slices    []*SliceState
}

type AssemblyState struct {
	History        []ReceiptEntry
	InputPins      map[string]*string
	StaleReason    string
	ResultCommit   string
	Stage          string
	Status         string
	NextRole       string
	Outcome        string
	CurrentReceipt *ReceiptEntry
	Candidate      *ReceiptEntry
	Pass           *ReceiptEntry
}

type Diagnostic struct {
	Code    string
	Release string
	Track   string
	Work    string
	Message string
}

type StateRefs struct {
	Release CapturedRef
	Target  CapturedRef
	Tracks  []TrackRefState
}

type TrackRefState struct {
	ID string
	CapturedRef
}

// State is a derived, immutable-by-convention projection of exact Git facts.
// It is never action authority; each action rescans before mutating.
type State struct {
	Release     string
	Repository  string
	Plan        PlanState
	Refs        StateRefs
	Tracks      []TrackState
	Slices      []*SliceState
	Assembly    AssemblyState
	Diagnostics []Diagnostic
}

type planEntry struct {
	OID    string
	Parsed Plan
}

type receiptHistory struct {
	Rows     []historyRow
	Receipts []ReceiptEntry
	ByOID    map[string]ReceiptEntry
}

func ReadState(gitRepository GitRepository, release string, resolver InertnessResolver) (State, error) {
	repository, err := newRepository(gitRepository.repository(), resolver)
	if err != nil {
		return State{}, err
	}
	if _, err := identity(release, "release"); err != nil {
		return State{}, err
	}
	return readState(repository, release, "")
}

func readState(repository *repository, release, expectedReleaseHead string) (State, error) {
	releaseName := releaseRef(release)
	initialValues, err := repository.capture([]string{releaseName})
	if err != nil {
		return State{}, err
	}
	initial := initialValues[0]
	if !directCommit(initial) {
		if absentRef(initial) {
			return State{}, recordFail("REF_NOT_FOUND", "release ref "+releaseName+" does not exist")
		}
		return State{}, recordFail("INVALID_HEAD_OBJECT", "release ref is not one direct commit")
	}
	if expectedReleaseHead != "" && initial.Head != expectedReleaseHead {
		return State{}, recordFail("REF_SNAPSHOT_UNSTABLE", "release ref moved before capture")
	}
	current, err := planAt(repository, release, initial.Head)
	if err != nil {
		return State{}, err
	}
	metadata := current.Parsed.Metadata()
	names := []string{releaseName, metadata.TargetRef}
	for _, track := range metadata.Tracks {
		names = append(names, trackRef(release, track.ID))
	}
	captured, err := repository.capture(names)
	if err != nil {
		return State{}, err
	}
	byRef := make(map[string]CapturedRef, len(captured))
	for _, value := range captured {
		byRef[value.Ref] = value
	}
	releaseCapture := byRef[releaseName]
	targetCapture := byRef[metadata.TargetRef]
	if !directCommit(releaseCapture) || releaseCapture.Head != initial.Head {
		return State{}, recordFail("REF_SNAPSHOT_UNSTABLE", "release ref moved during capture")
	}
	if !directCommit(targetCapture) {
		if absentRef(targetCapture) {
			return State{}, recordFail("REF_NOT_FOUND", "target ref "+metadata.TargetRef+" does not exist")
		}
		return State{}, recordFail("INVALID_HEAD_OBJECT", "target ref is not one direct commit")
	}
	for _, track := range metadata.Tracks {
		value := byRef[trackRef(release, track.ID)]
		if !directCommit(value) && !absentRef(value) {
			return State{}, recordFail("INVALID_HEAD_OBJECT", "track "+track.ID+" ref is not absent or one direct commit")
		}
	}

	releaseHistory, err := historyAt(repository, releaseCapture.Head)
	if err != nil {
		return State{}, err
	}
	for _, entry := range releaseHistory.Receipts {
		if entry.Receipt.Release != release {
			return State{}, recordFail("RELEASE_RECEIPT_MISMATCH", "receipt "+entry.OID+" names another release")
		}
	}
	chain, approvals, err := planChain(repository, release, current, releaseHistory.Receipts)
	if err != nil {
		return State{}, err
	}
	if err := validateRetirements(current, chain, approvals, releaseHistory.Receipts); err != nil {
		return State{}, err
	}
	planByOID := make(map[string]planEntry, len(chain))
	for _, entry := range chain {
		planByOID[entry.OID] = entry
	}

	plannerOnRelease := make(map[string]bool)
	for _, entry := range releaseHistory.Receipts {
		if entry.Receipt.Role == "planner" {
			plannerOnRelease[entry.OID] = true
		}
	}
	priorOwners := make(map[string]string)
	for _, entry := range chain {
		for id, location := range locations(entry.Parsed) {
			priorOwners[id] = location.Track.ID
		}
	}
	type trackHistory struct {
		ref   CapturedRef
		owned []ReceiptEntry
	}
	trackHistories := make(map[string]trackHistory)
	claimed := make(map[string]string)
	for _, track := range metadata.Tracks {
		ref := byRef[trackRef(release, track.ID)]
		history, err := historyAt(repository, ref.Head)
		if err != nil {
			return State{}, err
		}
		var owned []ReceiptEntry
		for _, entry := range history.Receipts {
			receipt := entry.Receipt
			if receipt.Slice == nil || plannerOnRelease[entry.OID] || priorOwners[*receipt.Slice] != track.ID {
				continue
			}
			if receipt.Release != release || (claimed[entry.OID] != "" && claimed[entry.OID] != track.ID) {
				return State{}, recordFail("AMBIGUOUS_AUTHORITY", "track "+track.ID+" contains a foreign receipt")
			}
			claimed[entry.OID] = track.ID
			owned = append(owned, entry)
		}
		if err := validateSerialSliceOrder(track, owned, planByOID); err != nil {
			return State{}, err
		}
		trackHistories[track.ID] = trackHistory{ref: ref, owned: owned}
	}

	productCache := make(map[string]string)
	histories := make(map[string]SliceHistory)
	allLocations := locations(current.Parsed)
	for id, location := range allLocations {
		var entries []ReceiptEntry
		for _, entry := range trackHistories[location.Track.ID].owned {
			if entry.Receipt.Slice != nil && *entry.Receipt.Slice == id {
				entries = append(entries, entry)
			}
		}
		history, err := validateSliceHistory(
			repository, location, entries, planByOID, approvals, productCache,
		)
		if err != nil {
			return State{}, err
		}
		histories[id] = history
	}
	states, err := deriveSlices(current, histories, approvals)
	if err != nil {
		return State{}, err
	}
	tracks := make([]TrackState, 0, len(metadata.Tracks))
	flatSlices := make([]*SliceState, 0, len(states))
	trackRefs := make([]TrackRefState, 0, len(metadata.Tracks))
	for _, track := range metadata.Tracks {
		ref := byRef[trackRef(release, track.ID)]
		trackState := TrackState{
			ID: track.ID, DependsOn: append([]string(nil), track.DependsOn...),
			Ref: ref.Ref, Head: ref.Head,
		}
		trackRefs = append(trackRefs, TrackRefState{ID: track.ID, CapturedRef: ref})
		for _, plannedSlice := range track.Slices {
			state := states[plannedSlice.ID]
			trackState.Slices = append(trackState.Slices, state)
			flatSlices = append(flatSlices, state)
		}
		tracks = append(tracks, trackState)
	}
	for _, track := range tracks {
		var active *SliceState
		for _, slice := range track.Slices {
			if slice.Pass != nil || slice.NextRole == "verifier" || slice.NextRole == "merge" {
				active = slice
				break
			}
		}
		if track.Head != "" && active != nil && active.Candidate != nil {
			observed, err := productTreeFor(repository, track.Head, productCache)
			if err != nil {
				return State{}, err
			}
			if active.Candidate.Receipt.ProductTree == nil ||
				observed != *active.Candidate.Receipt.ProductTree {
				return State{}, recordFail("CHANGED_CANDIDATE", "track "+track.ID+" moved after its current candidate")
			}
		}
	}

	var assemblyEntries []ReceiptEntry
	for _, entry := range releaseHistory.Receipts {
		receipt := entry.Receipt
		isAssembly := (receipt.Role == "implementer" && receipt.Result == "candidate" && receipt.Slice == nil) ||
			(receipt.Role == "verifier" && receipt.Slice == nil) || receipt.Role == "merge"
		if isAssembly {
			assemblyEntries = append(assemblyEntries, entry)
			continue
		}
		if receipt.Role != "planner" && claimed[entry.OID] == "" {
			return State{}, recordFail("AMBIGUOUS_AUTHORITY", "receipt "+entry.OID+" is on the wrong authority")
		}
	}
	var allSliceEntries []ReceiptEntry
	for _, history := range histories {
		allSliceEntries = append(allSliceEntries, history.Entries...)
	}
	assemblyHistory, err := validateAssemblyHistory(
		repository, assemblyEntries, planByOID, approvals,
		allSliceEntries, productCache,
	)
	if err != nil {
		return State{}, err
	}
	approval := approvals[current.OID]
	assembly, err := deriveAssembly(
		repository, current, assemblyHistory, approval, tracks, targetCapture.Head,
	)
	if err != nil {
		return State{}, err
	}
	if assembly.Status == "complete" {
		contained, err := repository.isAncestor(assembly.ResultCommit, targetCapture.Head)
		if err != nil {
			return State{}, err
		}
		if !contained {
			return State{}, recordFail("MOVED_TARGET", "target no longer contains the recorded merge")
		}
	}
	targetStale := assembly.Status != "complete" &&
		approval.Receipt.Target != nil && *approval.Receipt.Target != targetCapture.Head

	var diagnostics []Diagnostic
	if targetStale {
		diagnostics = append(diagnostics, Diagnostic{
			Code: "TARGET_MOVED", Release: release,
			Message: "the approved target moved; record a new plan revision",
		})
	}
	for _, track := range tracks {
		if track.Head == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "TRACK_REF_ABSENT", Release: release, Track: track.ID,
				Message: "track " + track.ID + " may be materialized from approved facts",
			})
		}
		for _, slice := range track.Slices {
			if slice.StaleReason != "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code: "STALE_INPUTS", Release: release, Track: track.ID,
					Work:    slice.Location.Slice.ID,
					Message: slice.Location.Slice.ID + " is recoverable: " + slice.StaleReason,
				})
			}
		}
	}
	if assembly.StaleReason != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Code: "STALE_ASSEMBLY", Release: release, Message: assembly.StaleReason,
		})
	}

	planHistory := make([]PlanHistory, len(chain))
	for index, entry := range chain {
		planHistory[index] = PlanHistory{
			OID: entry.OID, Revision: entry.Parsed.Metadata().Revision,
			Approval: approvals[entry.OID].Clone(), Plan: entry.Parsed,
		}
	}
	return State{
		Release: release, Repository: metadata.Repository,
		Plan: PlanState{
			OID: current.OID, Digest: current.Parsed.Digest(), Metadata: metadata,
			Approval: approval.Clone(), ApprovalOID: approval.OID,
			TargetStale: targetStale, History: planHistory,
		},
		Refs: StateRefs{
			Release: releaseCapture, Target: targetCapture, Tracks: trackRefs,
		},
		Tracks: tracks, Slices: flatSlices, Assembly: assembly,
		Diagnostics: diagnostics,
	}, nil
}

func planAt(repository *repository, release, commit string) (planEntry, error) {
	file, err := repository.file(commit, planPath(release))
	if err != nil {
		return planEntry{}, err
	}
	if !file.Present || file.Object == "" {
		return planEntry{}, recordFail("PLAN_NOT_FOUND", "release "+release+" has no plan")
	}
	parsed, err := ParsePlan(file.Bytes)
	if err != nil {
		return planEntry{}, err
	}
	if parsed.Metadata().Release != release {
		return planEntry{}, recordFail("RELEASE_PLAN_MISMATCH", "plan release does not match "+release)
	}
	return planEntry{OID: file.Object, Parsed: parsed}, nil
}

func historyAt(repository *repository, head string) (receiptHistory, error) {
	if head == "" {
		return receiptHistory{ByOID: make(map[string]ReceiptEntry)}, nil
	}
	rows, err := repository.history(head)
	if err != nil {
		return receiptHistory{}, err
	}
	var receipts []ReceiptEntry
	for index, row := range rows {
		if !bytes.Contains(row.Message, []byte("\n"+ReceiptTrailer)) {
			continue
		}
		if index+1 >= len(rows) || len(row.Parents) != 1 || row.Parents[0] != rows[index+1].OID {
			return receiptHistory{}, recordFail("HISTORY_LIMIT", "cannot establish the parent tree for receipt "+row.OID)
		}
		entry, err := ParseReceiptHistoryEntry(HistoryEnvelope{
			OID: row.OID, Parents: row.Parents, Tree: row.Tree,
			ParentTree: rows[index+1].Tree, Message: row.Message,
		})
		if err != nil {
			return receiptHistory{}, recordWrap(ErrorCode(err), "invalid receipt "+row.OID, err)
		}
		receipts = append(receipts, entry)
	}
	reverseRows(rows)
	reverseReceipts(receipts)
	byOID := make(map[string]ReceiptEntry, len(receipts))
	for _, entry := range receipts {
		byOID[entry.OID] = entry
	}
	return receiptHistory{Rows: rows, Receipts: receipts, ByOID: byOID}, nil
}

func reverseRows(values []historyRow) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseReceipts(values []ReceiptEntry) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func locations(plan Plan) map[string]SliceLocation {
	result := make(map[string]SliceLocation)
	for _, track := range plan.Metadata().Tracks {
		for _, slice := range track.Slices {
			result[slice.ID] = SliceLocation{Track: track, Slice: slice}
		}
	}
	return result
}

func matchingApproval(
	repository *repository,
	release string,
	entry planEntry,
	receipts []ReceiptEntry,
) (ReceiptEntry, error) {
	var matches []ReceiptEntry
	for _, candidate := range receipts {
		receipt := candidate.Receipt
		if receipt.Role == "planner" && receipt.Result == "approved" &&
			receipt.Plan == entry.OID && receipt.Slice == nil {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		code := "APPROVAL_MISSING"
		if len(matches) > 1 {
			code = "AMBIGUOUS_APPROVAL"
		}
		return ReceiptEntry{}, recordFail(code, fmt.Sprintf(
			"plan revision %d has %d approvals", entry.Parsed.Metadata().Revision, len(matches),
		))
	}
	approval := matches[0]
	file, err := repository.file(approval.OID, planPath(release))
	if err != nil {
		return ReceiptEntry{}, err
	}
	if approval.Receipt.Binds != approval.Parent || file.Object != entry.OID {
		return ReceiptEntry{}, recordFail("STALE_BINDING", "approval "+approval.OID+" does not bind its plan commit")
	}
	return approval, nil
}

func planChain(
	repository *repository,
	release string,
	current planEntry,
	receipts []ReceiptEntry,
) ([]planEntry, map[string]ReceiptEntry, error) {
	var reverse []planEntry
	seen := make(map[string]bool)
	cursor := current
	for {
		if seen[cursor.OID] {
			return nil, nil, recordFail("INVALID_PLAN_HISTORY", "plan history contains a cycle")
		}
		seen[cursor.OID] = true
		reverse = append(reverse, cursor)
		previous := cursor.Parsed.Metadata().PreviousPlan
		if previous == nil {
			break
		}
		if len(reverse) >= maxPlanRevisions {
			return nil, nil, recordFail("RESOURCE_LIMIT", fmt.Sprintf("plan history exceeds %d revisions", maxPlanRevisions))
		}
		var approval *ReceiptEntry
		for index := range receipts {
			receipt := receipts[index].Receipt
			if receipt.Role == "planner" && receipt.Result == "approved" &&
				receipt.Plan == *previous && receipt.Slice == nil {
				value := receipts[index]
				approval = &value
				break
			}
		}
		if approval == nil {
			return nil, nil, recordFail("INVALID_PLAN_HISTORY", "previous plan "+*previous+" has no approval")
		}
		file, err := repository.file(approval.OID, planPath(release))
		if err != nil {
			return nil, nil, err
		}
		if !file.Present || file.Object != *previous {
			return nil, nil, recordFail("INVALID_PLAN_HISTORY", "approval "+approval.OID+" does not contain "+*previous)
		}
		parsed, err := ParsePlan(file.Bytes)
		if err != nil {
			return nil, nil, err
		}
		nextMetadata, priorMetadata := cursor.Parsed.Metadata(), parsed.Metadata()
		if priorMetadata.Revision != nextMetadata.Revision-1 ||
			priorMetadata.Release != current.Parsed.Metadata().Release ||
			priorMetadata.Repository != current.Parsed.Metadata().Repository ||
			priorMetadata.TargetRef != current.Parsed.Metadata().TargetRef ||
			priorMetadata.ApprovalRef == nextMetadata.ApprovalRef {
			return nil, nil, recordFail("INVALID_PLAN_HISTORY", fmt.Sprintf(
				"plan revision %d has a broken predecessor", nextMetadata.Revision,
			))
		}
		oldLocations := locations(parsed)
		for id, location := range locations(cursor.Parsed) {
			if old, ok := oldLocations[id]; ok && old.Track.ID != location.Track.ID {
				return nil, nil, recordFail("AMBIGUOUS_AUTHORITY", "slice "+id+" moved between tracks")
			}
		}
		cursor = planEntry{OID: *previous, Parsed: parsed}
	}
	for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
		reverse[left], reverse[right] = reverse[right], reverse[left]
	}
	if reverse[0].Parsed.Metadata().Revision != 1 {
		return nil, nil, recordFail("INVALID_PLAN_HISTORY", "plan history does not terminate at revision 1")
	}
	approvals := make(map[string]ReceiptEntry, len(reverse))
	for _, entry := range reverse {
		approval, err := matchingApproval(repository, release, entry, receipts)
		if err != nil {
			return nil, nil, err
		}
		approvals[entry.OID] = approval
	}
	return reverse, approvals, nil
}

func validateRetirements(
	current planEntry,
	chain []planEntry,
	approvals map[string]ReceiptEntry,
	receipts []ReceiptEntry,
) error {
	active := locations(current.Parsed)
	type priorLocation struct {
		entry    planEntry
		location SliceLocation
	}
	prior := make(map[string]priorLocation)
	for _, entry := range chain[:len(chain)-1] {
		for id, location := range locations(entry.Parsed) {
			prior[id] = priorLocation{entry: entry, location: location}
		}
	}
	var retired []ReceiptEntry
	for _, entry := range receipts {
		if entry.Receipt.Role == "planner" && entry.Receipt.Result == "retired" {
			retired = append(retired, entry)
			if entry.Receipt.Slice != nil && active[*entry.Receipt.Slice].Slice.ID != "" {
				return recordFail("INVALID_RETIREMENT", "active slice "+*entry.Receipt.Slice+" is retired")
			}
		}
	}
	for id, old := range prior {
		if _, ok := active[id]; ok {
			continue
		}
		matches := 0
		for _, entry := range retired {
			receipt := entry.Receipt
			if receipt.Slice != nil && *receipt.Slice == id &&
				receipt.Plan == current.OID &&
				receipt.Binds == approvals[current.OID].OID &&
				receipt.Contract != nil &&
				*receipt.Contract == old.entry.Parsed.Metadata().Contracts[id] {
				matches++
			}
		}
		if matches != 1 {
			return recordFail("RETIREMENT_MISSING", "removed slice "+id+" requires one current retirement")
		}
	}
	return nil
}

func latest(entries []ReceiptEntry, predicate func(ReceiptEntry) bool) *ReceiptEntry {
	for index := len(entries) - 1; index >= 0; index-- {
		if predicate(entries[index]) {
			value := entries[index]
			return &value
		}
	}
	return nil
}

func applicablePriorPass(entries []ReceiptEntry, plan planEntry, sliceID string) *ReceiptEntry {
	contract := plan.Parsed.Metadata().Contracts[sliceID]
	var matching []ReceiptEntry
	for _, entry := range entries {
		if entry.Receipt.Slice != nil && *entry.Receipt.Slice == sliceID &&
			entry.Receipt.Contract != nil && *entry.Receipt.Contract == contract {
			matching = append(matching, entry)
		}
	}
	pass := latest(matching, func(entry ReceiptEntry) bool {
		return entry.Receipt.Role == "verifier" && entry.Receipt.Result == "pass"
	})
	if pass == nil {
		return nil
	}
	for _, entry := range matching {
		if entry.Receipt.Attempt != nil && *entry.Receipt.Attempt > *pass.Receipt.Attempt {
			return nil
		}
	}
	return pass
}

func validateSerialSliceOrder(
	track Track,
	entries []ReceiptEntry,
	planByOID map[string]planEntry,
) error {
	var priorEntries []ReceiptEntry
	for _, entry := range entries {
		plan, ok := planByOID[entry.Receipt.Plan]
		if !ok {
			return recordFail("AMBIGUOUS_AUTHORITY", "receipt "+entry.OID+" has an unknown plan")
		}
		var plannedTrack *Track
		for index := range plan.Parsed.Metadata().Tracks {
			if plan.Parsed.Metadata().Tracks[index].ID == track.ID {
				value := plan.Parsed.Metadata().Tracks[index]
				plannedTrack = &value
				break
			}
		}
		position := -1
		if plannedTrack != nil && entry.Receipt.Slice != nil {
			for index, slice := range plannedTrack.Slices {
				if slice.ID == *entry.Receipt.Slice {
					position = index
					break
				}
			}
		}
		if position < 0 {
			return recordFail("AMBIGUOUS_AUTHORITY", "receipt "+entry.OID+" uses the wrong track")
		}
		for _, prior := range plannedTrack.Slices[:position] {
			if applicablePriorPass(priorEntries, plan, prior.ID) == nil {
				return recordFail("DEPENDENCIES_NOT_READY", *entry.Receipt.Slice+" advanced before "+prior.ID+" PASS")
			}
		}
		priorEntries = append(priorEntries, entry)
	}
	return nil
}

func validateSliceHistory(
	repository *repository,
	location SliceLocation,
	entries []ReceiptEntry,
	planByOID map[string]planEntry,
	approvals map[string]ReceiptEntry,
	productCache map[string]string,
) (SliceHistory, error) {
	byOID := make(map[string]ReceiptEntry)
	for _, entry := range approvals {
		byOID[entry.OID] = entry
	}
	seen := make(map[string]bool)
	var maximum int64
	for _, entry := range entries {
		receipt := entry.Receipt
		plan, ok := planByOID[receipt.Plan]
		planned, plannedOK := locations(plan.Parsed)[receipt.SliceID()]
		if !ok || !plannedOK || planned.Track.ID != location.Track.ID {
			return SliceHistory{}, recordFail("AMBIGUOUS_AUTHORITY", "receipt "+entry.OID+" uses the wrong track")
		}
		if receipt.Contract == nil ||
			*receipt.Contract != plan.Parsed.Metadata().Contracts[receipt.SliceID()] ||
			(receipt.Role != "implementer" && receipt.Role != "captain" && receipt.Role != "verifier") {
			return SliceHistory{}, recordFail("STALE_BINDING", "receipt "+entry.OID+" has stale slice bindings")
		}
		attempt := *receipt.Attempt
		if attempt < maximum || attempt > maximum+1 {
			return SliceHistory{}, recordFail("INVALID_ATTEMPT", "receipt "+entry.OID+" has non-monotonic attempt")
		}
		if attempt > maximum {
			maximum = attempt
		}
		roleKey := "decision"
		if receipt.Role == "implementer" {
			roleKey = receipt.Result
		}
		identity := fmt.Sprintf("%d:%s:%s", attempt, receipt.Role, roleKey)
		if seen[identity] {
			return SliceHistory{}, recordFail("AMBIGUOUS_RECEIPT", receipt.SliceID()+" repeats "+identity)
		}
		seen[identity] = true
		bound, boundOK := byOID[receipt.Binds]
		sameSlice := boundOK && bound.Receipt.Slice != nil &&
			*bound.Receipt.Slice == receipt.SliceID()
		samePlan := boundOK && bound.Receipt.Plan == receipt.Plan
		switch {
		case receipt.Role == "implementer" && receipt.Result == "designed":
			approved := boundOK && bound.Receipt.Role == "planner" &&
				bound.Receipt.Result == "approved" && samePlan
			retry := sameSlice && bound.Receipt.Attempt != nil &&
				*bound.Receipt.Attempt == attempt-1 &&
				((bound.Receipt.Role == "captain" && bound.Receipt.Result == "revise") ||
					(bound.Receipt.Role == "verifier" && bound.Receipt.Result == "fail"))
			if !approved && !retry {
				return SliceHistory{}, recordFail("STALE_BINDING", "design "+entry.OID+" has no predecessor")
			}
		case receipt.Role == "captain":
			if !sameSlice || !samePlan || bound.Receipt.Role != "implementer" ||
				bound.Receipt.Result != "designed" || *bound.Receipt.Attempt != attempt {
				return SliceHistory{}, recordFail("STALE_BINDING", "Captain "+entry.OID+" does not bind its design")
			}
		case receipt.Role == "implementer" && receipt.Result == "candidate":
			proceeded := sameSlice && samePlan && bound.Receipt.Role == "captain" &&
				bound.Receipt.Result == "proceed" && *bound.Receipt.Attempt == attempt
			retry := sameSlice && samePlan && bound.Receipt.Role == "verifier" &&
				bound.Receipt.Result == "fail" && *bound.Receipt.Attempt == attempt-1
			if !proceeded && !retry {
				return SliceHistory{}, recordFail("STALE_BINDING", "candidate "+entry.OID+" lacks PROCEED")
			}
			if err := exactInputs(receipt, planned.Slice.Consumes, "candidate "+entry.OID); err != nil {
				return SliceHistory{}, err
			}
			if receipt.Candidate == nil || receipt.ProductTree == nil ||
				entry.Parent != *receipt.Candidate || *receipt.Candidate == receipt.Binds {
				return SliceHistory{}, recordFail("CHANGED_CANDIDATE", "candidate "+entry.OID+" has invalid Git evidence")
			}
			ancestor, err := repository.isAncestor(receipt.Binds, *receipt.Candidate)
			if err != nil {
				return SliceHistory{}, err
			}
			baseAncestor := true
			if receipt.Base != nil {
				baseAncestor, err = repository.isAncestor(*receipt.Base, *receipt.Candidate)
				if err != nil {
					return SliceHistory{}, err
				}
			}
			product, err := productTreeFor(repository, *receipt.Candidate, productCache)
			if err != nil {
				return SliceHistory{}, err
			}
			if !ancestor || !baseAncestor || product != *receipt.ProductTree {
				return SliceHistory{}, recordFail("CHANGED_CANDIDATE", "candidate "+entry.OID+" has invalid Git evidence")
			}
		case receipt.Role == "verifier":
			if !sameSlice || !samePlan || bound.Receipt.Role != "implementer" ||
				bound.Receipt.Result != "candidate" || *bound.Receipt.Attempt != attempt ||
				!sameCandidate(receipt, bound.Receipt) {
				return SliceHistory{}, recordFail("STALE_BINDING", "Verifier "+entry.OID+" does not bind its candidate")
			}
		default:
			return SliceHistory{}, recordFail("INVALID_RECEIPT", "slice "+receipt.SliceID()+" has an unsupported receipt")
		}
		byOID[entry.OID] = entry
	}
	return SliceHistory{Entries: entries, MaximumAttempt: maximum}, nil
}

func deriveSlice(
	location SliceLocation,
	history SliceHistory,
	current planEntry,
	approval ReceiptEntry,
) *SliceState {
	contract := current.Parsed.Metadata().Contracts[location.Slice.ID]
	var matching []ReceiptEntry
	for _, entry := range history.Entries {
		if entry.Receipt.Contract != nil && *entry.Receipt.Contract == contract {
			matching = append(matching, entry)
		}
	}
	pass := latest(matching, func(entry ReceiptEntry) bool {
		return entry.Receipt.Role == "verifier" && entry.Receipt.Result == "pass"
	})
	passCurrent := pass != nil
	if pass != nil {
		for _, entry := range matching {
			if *entry.Receipt.Attempt > *pass.Receipt.Attempt {
				passCurrent = false
			}
		}
	}
	currentReceipt := latest(matching, func(entry ReceiptEntry) bool {
		return entry.Receipt.Plan == current.OID
	})
	next := history.MaximumAttempt + 1
	state := &SliceState{
		Location: location, History: history,
		NextAttempts: AttemptNumbers{Design: next, Candidate: next},
	}
	if passCurrent {
		candidate := findEntry(history.Entries, pass.Receipt.Binds)
		state.Stage, state.Status, state.NextRole, state.Outcome = "merge", "ready", "merge", "pass"
		state.Attempt, state.CurrentReceipt, state.Candidate, state.Pass = *pass.Receipt.Attempt, pass, candidate, pass
		state.Retained = pass.Receipt.Plan != current.OID
		return state
	}
	if currentReceipt == nil {
		approvalCopy := approval.Clone()
		state.Stage, state.Status, state.NextRole, state.Outcome = "design", "ready", "implementer", "none"
		state.Attempt, state.CurrentReceipt = next, &approvalCopy
		return state
	}
	receipt := currentReceipt.Receipt
	var candidate *ReceiptEntry
	if receipt.Role == "verifier" {
		candidate = findEntry(history.Entries, receipt.Binds)
	} else if receipt.Result == "candidate" {
		candidate = currentReceipt
	}
	state.Attempt, state.CurrentReceipt, state.Candidate = *receipt.Attempt, currentReceipt, candidate
	switch receipt.Role + "/" + receipt.Result {
	case "implementer/designed":
		state.Stage, state.Status, state.NextRole, state.Outcome = "design", "ready", "captain", "none"
	case "implementer/candidate":
		state.Stage, state.Status, state.NextRole, state.Outcome = "verify", "ready", "verifier", "none"
	case "captain/proceed":
		state.Stage, state.Status, state.NextRole, state.Outcome = "implement", "ready", "implementer", "proceed"
	case "captain/revise":
		state.Stage, state.Status, state.NextRole, state.Outcome = "design", "ready", "implementer", "revise"
		state.Attempt++
	case "captain/escalate":
		state.Stage, state.Status, state.NextRole, state.Outcome = "design", "blocked", "planner", "escalate"
	case "verifier/pass":
		state.Stage, state.Status, state.NextRole, state.Outcome = "merge", "ready", "merge", "pass"
		state.Pass = currentReceipt
	case "verifier/fail":
		state.Stage, state.Status, state.NextRole, state.Outcome = "implement", "ready", "implementer", "fail"
		state.Attempt++
		state.NextAttempts = AttemptNumbers{Design: next, Candidate: *receipt.Attempt + 1}
	case "verifier/blocked":
		state.Stage, state.Status, state.NextRole, state.Outcome = "verify", "blocked", "planner", "blocked"
	default:
		state.Stage, state.Status, state.NextRole, state.Outcome = "invalid", "blocked", "none", "invalid"
	}
	return state
}

func deriveSlices(
	current planEntry,
	histories map[string]SliceHistory,
	approvals map[string]ReceiptEntry,
) (map[string]*SliceState, error) {
	states := make(map[string]*SliceState)
	for id, location := range locations(current.Parsed) {
		states[id] = deriveSlice(location, histories[id], current, approvals[current.OID])
	}
	pending, done := make(map[string]bool), make(map[string]bool)
	var resolve func(string) error
	resolve = func(id string) error {
		if done[id] {
			return nil
		}
		if pending[id] {
			return recordFail("DEPENDENCY_CYCLE", "slice dependency cycle reaches "+id)
		}
		pending[id] = true
		state := states[id]
		slice := state.Location.Slice
		required := unique(append(append([]string(nil), slice.DependsOn...), slice.Consumes...))
		ready := true
		for _, dependency := range required {
			if err := resolve(dependency); err != nil {
				return err
			}
			ready = ready && states[dependency].Pass != nil
		}
		pins := make(map[string]string, len(slice.Consumes))
		for _, dependency := range slice.Consumes {
			if states[dependency].Pass != nil && states[dependency].Pass.Receipt.ProductTree != nil {
				pins[dependency] = *states[dependency].Pass.Receipt.ProductTree
			} else {
				pins[dependency] = ""
			}
		}
		if ready {
			state.InputPins = pins
		}
		if state.Candidate != nil &&
			(!ready || !inputsEqual(state.Candidate.Receipt.Inputs, pins)) {
			state.Stage, state.Status, state.NextRole, state.Outcome = "implement", "ready", "implementer", "stale"
			state.Attempt = state.History.MaximumAttempt + 1
			state.Pass, state.Retained = nil, false
			state.StaleReason = "dependency eligibility or consumed inputs changed"
		}
		delete(pending, id)
		done[id] = true
		return nil
	}
	for id := range states {
		if err := resolve(id); err != nil {
			return nil, err
		}
	}
	metadata := current.Parsed.Metadata()
	trackReady := make(map[string]bool)
	for _, track := range metadata.Tracks {
		ready := true
		for _, slice := range track.Slices {
			ready = ready && states[slice.ID].Pass != nil
		}
		trackReady[track.ID] = ready
	}
	for _, track := range metadata.Tracks {
		dependenciesReady := true
		for _, dependency := range track.DependsOn {
			dependenciesReady = dependenciesReady && trackReady[dependency]
		}
		priorReady := true
		for _, slice := range track.Slices {
			state := states[slice.ID]
			explicitReady := true
			for _, dependency := range unique(append(append([]string(nil), slice.DependsOn...), slice.Consumes...)) {
				explicitReady = explicitReady && states[dependency].Pass != nil
			}
			if state.Pass == nil && state.Status != "blocked" &&
				!(dependenciesReady && priorReady && explicitReady) {
				state.Status, state.NextRole = "waiting", "none"
			}
			priorReady = priorReady && state.Pass != nil
		}
	}
	return states, nil
}

func findEntry(entries []ReceiptEntry, oid string) *ReceiptEntry {
	for index := range entries {
		if entries[index].OID == oid {
			value := entries[index]
			return &value
		}
	}
	return nil
}

func assemblyCandidate(byOID map[string]ReceiptEntry, entry *ReceiptEntry) *ReceiptEntry {
	cursor := entry
	for cursor != nil && cursor.Receipt.Role != "implementer" {
		next, ok := byOID[cursor.Receipt.Binds]
		if !ok {
			return nil
		}
		value := next
		cursor = &value
	}
	return cursor
}

func validateAssemblyHistory(
	repository *repository,
	entries []ReceiptEntry,
	planByOID map[string]planEntry,
	approvals map[string]ReceiptEntry,
	sliceEntries []ReceiptEntry,
	productCache map[string]string,
) (receiptHistory, error) {
	byOID := make(map[string]ReceiptEntry)
	for _, entry := range approvals {
		byOID[entry.OID] = entry
	}
	for _, entry := range sliceEntries {
		byOID[entry.OID] = entry
	}
	var previous *ReceiptEntry
	for index := range entries {
		entry := entries[index]
		receipt := entry.Receipt
		plan, ok := planByOID[receipt.Plan]
		if !ok {
			return receiptHistory{}, recordFail("STALE_BINDING", "assembly receipt "+entry.OID+" has an unknown plan")
		}
		approval := approvals[receipt.Plan]
		switch receipt.Role {
		case "implementer":
			var trackIDs []string
			for _, track := range plan.Parsed.Metadata().Tracks {
				trackIDs = append(trackIDs, track.ID)
			}
			if err := exactInputs(receipt, trackIDs, "assembly candidate "+entry.OID); err != nil {
				return receiptHistory{}, err
			}
			bound, exists := byOID[receipt.Binds]
			first := exists && bound.OID == approval.OID
			retry := previous != nil && exists && bound.OID == previous.OID
			if (!first && !retry) || receipt.Candidate == nil || receipt.Base == nil ||
				receipt.Target == nil || receipt.ProductTree == nil ||
				entry.Parent != *receipt.Candidate {
				return receiptHistory{}, recordFail("STALE_BINDING", "assembly candidate "+entry.OID+" has invalid evidence")
			}
			baseAncestor, err := repository.isAncestor(*receipt.Base, *receipt.Candidate)
			if err != nil {
				return receiptHistory{}, err
			}
			bindAncestor, err := repository.isAncestor(receipt.Binds, *receipt.Candidate)
			if err != nil {
				return receiptHistory{}, err
			}
			product, err := productTreeFor(repository, *receipt.Candidate, productCache)
			if err != nil {
				return receiptHistory{}, err
			}
			if !baseAncestor || !bindAncestor || approval.Receipt.Target == nil ||
				*receipt.Target != *approval.Receipt.Target || product != *receipt.ProductTree {
				return receiptHistory{}, recordFail("STALE_BINDING", "assembly candidate "+entry.OID+" has invalid evidence")
			}
		case "verifier":
			bound, exists := byOID[receipt.Binds]
			if !exists || bound.Receipt.Role != "implementer" || bound.Receipt.Slice != nil ||
				!sameCandidate(receipt, bound.Receipt) {
				return receiptHistory{}, recordFail("STALE_BINDING", "assembly Verifier "+entry.OID+" has no exact candidate")
			}
		case "merge":
			bound, exists := byOID[receipt.Binds]
			assemblyPass := exists && bound.Receipt.Role == "verifier" &&
				bound.Receipt.Result == "pass" && bound.Receipt.Slice == nil
			oneTrack := len(plan.Parsed.Metadata().Tracks) == 1
			lastSlice := ""
			if oneTrack {
				slices := plan.Parsed.Metadata().Tracks[0].Slices
				lastSlice = slices[len(slices)-1].ID
			}
			directPass := oneTrack && exists && bound.Receipt.Role == "verifier" &&
				bound.Receipt.Result == "pass" && bound.Receipt.SliceID() == lastSlice
			candidateEntry := &bound
			if assemblyPass {
				candidateEntry = assemblyCandidate(byOID, &bound)
			}
			if (!assemblyPass && !directPass) || receipt.Target == nil ||
				approval.Receipt.Target == nil || *receipt.Target != *approval.Receipt.Target ||
				receipt.Candidate == nil || candidateEntry == nil ||
				candidateEntry.Receipt.Candidate == nil ||
				*receipt.Candidate != *candidateEntry.Receipt.Candidate ||
				receipt.ProductTree == nil || candidateEntry.Receipt.ProductTree == nil ||
				*receipt.ProductTree != *candidateEntry.Receipt.ProductTree ||
				receipt.ResultCommit == nil {
				return receiptHistory{}, recordFail("STALE_BINDING", "Merge "+entry.OID+" has no applicable PASS")
			}
			if err := verifyReleaseIntegration(
				repository, *receipt.Target, *receipt.Candidate, *receipt.ResultCommit,
			); err != nil {
				return receiptHistory{}, recordWrap(ErrorCode(err), "Merge "+entry.OID+" is not exact", err)
			}
		default:
			return receiptHistory{}, recordFail("AMBIGUOUS_AUTHORITY", "release receipt "+entry.OID+" has an invalid role")
		}
		byOID[entry.OID] = entry
		value := entry
		previous = &value
	}
	return receiptHistory{Receipts: entries, ByOID: byOID}, nil
}

func deriveAssembly(
	repository *repository,
	current planEntry,
	history receiptHistory,
	approval ReceiptEntry,
	tracks []TrackState,
	target string,
) (AssemblyState, error) {
	var entries []ReceiptEntry
	for _, entry := range history.Receipts {
		if entry.Receipt.Plan == current.OID {
			entries = append(entries, entry)
		}
	}
	var latestEntry *ReceiptEntry
	if len(entries) > 0 {
		value := entries[len(entries)-1]
		latestEntry = &value
	}
	allPassed := true
	pins := make(map[string]*string, len(tracks))
	for _, track := range tracks {
		for _, slice := range track.Slices {
			allPassed = allPassed && slice.Pass != nil
		}
		final := track.Slices[len(track.Slices)-1]
		if final.Pass != nil && final.Pass.Receipt.ProductTree != nil {
			value := *final.Pass.Receipt.ProductTree
			pins[track.ID] = &value
		} else {
			pins[track.ID] = nil
		}
	}
	common := AssemblyState{History: history.Receipts, InputPins: pins}
	if !allPassed {
		common.Stage, common.Status, common.NextRole, common.Outcome = "verify", "waiting", "none", "none"
		return common, nil
	}
	if latestEntry == nil {
		var direct *SliceState
		if len(tracks) == 1 {
			direct = tracks[0].Slices[len(tracks[0].Slices)-1]
		}
		isDirect := false
		if direct != nil && direct.Pass != nil && approval.Receipt.Target != nil &&
			direct.Pass.Receipt.Candidate != nil {
			var err error
			isDirect, err = repository.isAncestor(*approval.Receipt.Target, *direct.Pass.Receipt.Candidate)
			if err != nil {
				return AssemblyState{}, err
			}
		}
		if isDirect {
			common.Stage, common.Status, common.NextRole, common.Outcome = "merge", "ready", "merge", "pass"
			common.CurrentReceipt, common.Candidate, common.Pass = direct.Pass, direct.Candidate, direct.Pass
		} else {
			approvalCopy := approval.Clone()
			common.Stage, common.Status, common.NextRole, common.Outcome = "verify", "ready", "merge", "none"
			common.CurrentReceipt = &approvalCopy
		}
		return common, nil
	}
	candidate := assemblyCandidate(history.ByOID, latestEntry)
	if latestEntry.Receipt.Role == "merge" {
		common.Stage, common.Status, common.NextRole, common.Outcome = "merge", "complete", "none", "merged"
		common.CurrentReceipt, common.Candidate = latestEntry, candidate
		pass := history.ByOID[latestEntry.Receipt.Binds]
		common.Pass = &pass
		if latestEntry.Receipt.ResultCommit != nil {
			common.ResultCommit = *latestEntry.Receipt.ResultCommit
		}
		return common, nil
	}
	stale := candidate != nil && candidate.Receipt.Target != nil &&
		(*candidate.Receipt.Target != target ||
			!inputsEqual(candidate.Receipt.Inputs, concreteAssemblyPins(pins)))
	if stale {
		common.Stage, common.Status, common.NextRole, common.Outcome = "verify", "ready", "merge", "stale"
		common.CurrentReceipt, common.Candidate = latestEntry, candidate
		common.StaleReason = "target or track inputs changed"
		return common, nil
	}
	if latestEntry.Receipt.Role == "implementer" {
		common.Stage, common.Status, common.NextRole, common.Outcome = "verify", "ready", "verifier", "none"
		common.CurrentReceipt, common.Candidate = latestEntry, candidate
		return common, nil
	}
	if latestEntry.Receipt.Role == "verifier" {
		result := latestEntry.Receipt.Result
		common.Stage, common.Status, common.NextRole, common.Outcome = "verify", "ready", "merge", result
		if result == "pass" {
			common.Stage, common.NextRole, common.Pass = "merge", "merge", latestEntry
		}
		if result == "blocked" {
			common.Status, common.NextRole = "blocked", "planner"
		}
		common.CurrentReceipt, common.Candidate = latestEntry, candidate
		return common, nil
	}
	return AssemblyState{}, recordFail("INVALID_RECEIPT", "assembly history ends at unsupported "+latestEntry.Receipt.Role)
}

func concreteAssemblyPins(value map[string]*string) map[string]string {
	result := make(map[string]string, len(value))
	for key, item := range value {
		if item != nil {
			result[key] = *item
		}
	}
	return result
}

func (s State) Slice(id string) (*SliceState, bool) {
	for _, slice := range s.Slices {
		if slice.Location.Slice.ID == id {
			return slice, true
		}
	}
	return nil, false
}

func (s State) Track(id string) (*TrackState, bool) {
	for index := range s.Tracks {
		if s.Tracks[index].ID == id {
			return &s.Tracks[index], true
		}
	}
	return nil, false
}

func orderedInputMap(values map[string]string) string {
	keys := sortedInputKeys(values)
	var result strings.Builder
	for _, key := range keys {
		result.WriteString(key)
		result.WriteByte(0)
		result.WriteString(values[key])
		result.WriteByte('\n')
	}
	return result.String()
}

func inputMapsEqual(left, right map[string]string) bool {
	return orderedInputMap(left) == orderedInputMap(right)
}

func sortReceipts(values []ReceiptEntry) {
	sort.Slice(values, func(i, j int) bool {
		return bytes.Compare([]byte(values[i].OID), []byte(values[j].OID)) < 0
	})
}
