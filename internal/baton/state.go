package baton

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/gitx"
)

const maxPlanRevisions = 256
const maxCandidateLineage = 4096

type PlanHistory struct {
	OID         string
	Revision    int64
	Approval    ReceiptEntry
	Plan        Plan
	InstallHead string
	Retirements []RetirementResult
}

type PlanState struct {
	OID            string
	Digest         string
	Metadata       Metadata
	Approval       ReceiptEntry
	ApprovalOID    string
	TargetStale    bool
	LegacyFallback bool
	History        []PlanHistory
}

type SliceLocation struct {
	Track Track
	Slice Slice
}

type SliceHistory struct {
	Entries        []ReceiptEntry
	MaximumAttempt int64
}

type SliceHistoryState struct {
	Slice   string
	Track   string
	Ref     string
	History SliceHistory
}

type AttemptNumbers struct {
	Design    int64
	Candidate int64
}

// ConsumedInput is the exact current producer authority that a consuming
// slice must materialize before model work can proceed. The slice order in the
// plan is preserved by SliceState.ConsumedInputs.
type ConsumedInput struct {
	Slice            string
	PassReceipt      string
	CandidateReceipt string
	Candidate        string
	ProductTree      string
	SourceRef        string
	SourceHead       string
}

type SliceState struct {
	Location              SliceLocation
	History               SliceHistory
	InputPins             map[string]string
	ReviewedPins          map[string]string
	ReviewedBase          string
	PreparationSeed       string
	PreparedBase          string
	ConsumedInputs        []ConsumedInput
	NextAttempts          AttemptNumbers
	StaleReason           string
	Stage                 string
	Status                string
	NextRole              string
	Outcome               string
	Attempt               int64
	CurrentReceipt        *ReceiptEntry
	Candidate             *ReceiptEntry
	Pass                  *ReceiptEntry
	Retained              bool
	preparedAuthorityBase string
}

type TrackState struct {
	ID            string
	DependsOn     []string
	Ref           string
	Head          string
	AuthorityHead string
	Slices        []*SliceState
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
	Release        string
	Repository     string
	Plan           PlanState
	Refs           StateRefs
	Tracks         []TrackState
	Slices         []*SliceState
	SliceHistories []SliceHistoryState
	Assembly       AssemblyState
	Diagnostics    []Diagnostic
	productBases   *productBaseEvidence
}

// productBaseEvidence is engine-only state reconstructed from exact protected
// PASS chains. It is intentionally absent from the public State projection:
// callers may identify a consumed PASS, but cannot choose a Git merge base.
type productBaseEvidence struct {
	pass  func(sliceID, passOID string) (string, error)
	track func(trackID string) (string, error)
}

// BindTrackBaseProductResolver returns an opaque resolver bound to this exact
// state snapshot. Runtime may hand the closure to gitx, but neither a journal
// record nor an external caller can provide a raw merge base.
func (s State) BindTrackBaseProductResolver(
	format gitx.ObjectFormat,
) (func(gitx.TrackBaseInput) (gitx.OID, error), error) {
	if s.productBases == nil || s.productBases.pass == nil {
		return nil, recordFail(
			"PRODUCT_BASE_RESOLVER_REQUIRED",
			"exact Baton state has no product-base evidence",
		)
	}
	return func(input gitx.TrackBaseInput) (gitx.OID, error) {
		value, err := s.productBases.pass(
			input.Slice,
			input.PassReceipt.String(),
		)
		if err != nil {
			return gitx.OID{}, err
		}
		oid, err := gitx.ParseOID(format, value)
		if err != nil {
			return gitx.OID{}, translateGitError(
				"parse product-base evidence",
				err,
			)
		}
		return oid, nil
	}, nil
}

type planEntry struct {
	OID            string
	Parsed         Plan
	LegacyFallback bool
}

type receiptHistory struct {
	Boundary string
	Rows     []historyRow
	Receipts []ReceiptEntry
	ByOID    map[string]ReceiptEntry
}

type trackHistory struct {
	ref   CapturedRef
	owned []ReceiptEntry
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
	releaseHistory, err := readReleaseReceiptHistory(
		repository,
		release,
		initial.Head,
	)
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
	if err := validateRetirements(chain, approvals, releaseHistory.Receipts); err != nil {
		return State{}, err
	}
	historicalTracks := make(map[string]Track)
	var historicalTrackOrder []string
	historicalLocations := make(map[string]SliceLocation)
	var sliceHistoryOrder []string
	seenTracks := make(map[string]bool)
	seenSlices := make(map[string]bool)
	for _, entry := range chain {
		for _, track := range entry.Parsed.Metadata().Tracks {
			if !seenTracks[track.ID] {
				seenTracks[track.ID] = true
				historicalTrackOrder = append(historicalTrackOrder, track.ID)
			}
			historicalTracks[track.ID] = track
			for _, slice := range track.Slices {
				if !seenSlices[slice.ID] {
					seenSlices[slice.ID] = true
					sliceHistoryOrder = append(sliceHistoryOrder, slice.ID)
				}
				historicalLocations[slice.ID] = SliceLocation{
					Track: track,
					Slice: slice,
				}
			}
		}
	}
	names := []string{releaseName, metadata.TargetRef}
	for _, trackID := range historicalTrackOrder {
		names = append(names, trackRef(release, trackID))
	}
	captured, err := repository.capture(unique(names))
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
	// The current admitted manifest's declared contracts are resolved by
	// canonical digest from the digest-addressed store in the record root at
	// the release head (the record commit lineage, which always contains the
	// record root), falling back to path-keyed resolution from the target head
	// for releases recorded before digest addressing. Rereading and
	// cross-validating them here, on every read, keeps repository discovery
	// fail-closed against a contract that was substituted or disappeared after
	// the manifest was recorded; legacy baton.plan/v2 plans have no contract
	// paths and are unaffected.
	if err := resolveManifestContracts(repository, current.Parsed, targetCapture.Head, releaseCapture.Head, nil); err != nil {
		return State{}, err
	}
	for _, trackID := range historicalTrackOrder {
		value := byRef[trackRef(release, trackID)]
		if !directCommit(value) && !absentRef(value) {
			return State{}, recordFail("INVALID_HEAD_OBJECT", "track "+trackID+" ref is not absent or one direct commit")
		}
	}
	planByOID := make(map[string]planEntry, len(chain))
	for _, entry := range chain {
		planByOID[entry.OID] = entry
	}
	topologies := assemblyTopologies(chain)

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
	trackHistories := make(map[string]trackHistory)
	claimed := make(map[string]string)
	for _, trackID := range historicalTrackOrder {
		track := historicalTracks[trackID]
		ref := byRef[trackRef(release, trackID)]
		history, err := historyAt(
			repository,
			ref.Head,
			releaseHistory.Boundary,
		)
		if err != nil {
			return State{}, err
		}
		var owned []ReceiptEntry
		for _, entry := range history.Receipts {
			receipt := entry.Receipt
			if receipt.Slice == nil || plannerOnRelease[entry.OID] ||
				priorOwners[*receipt.Slice] != track.ID {
				continue
			}
			plan, knownPlan := planByOID[receipt.Plan]
			location, planned := locations(plan.Parsed)[receipt.SliceID()]
			if !knownPlan || !planned || location.Track.ID != track.ID ||
				receipt.Release != release ||
				(claimed[entry.OID] != "" && claimed[entry.OID] != track.ID) {
				return State{}, recordFail(
					"AMBIGUOUS_AUTHORITY",
					"track "+track.ID+" contains a foreign receipt",
				)
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
	sliceHistories := make([]SliceHistoryState, 0, len(sliceHistoryOrder))
	for _, id := range sliceHistoryOrder {
		location := historicalLocations[id]
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
		sliceHistories = append(sliceHistories, SliceHistoryState{
			Slice:   id,
			Track:   location.Track.ID,
			Ref:     trackHistories[location.Track.ID].ref.Ref,
			History: history,
		})
	}
	productBaseResolver := newPassProductBaseResolver(
		repository,
		release,
		histories,
		planByOID,
		approvals,
		productCache,
	)
	if err := validateConsumedHistories(
		repository,
		release,
		histories,
		trackHistories,
		planByOID,
		approvals,
		releaseHistory.Receipts,
		productBaseResolver.baselineFor,
		productCache,
	); err != nil {
		return State{}, err
	}
	states, err := deriveSlices(
		repository,
		current,
		histories,
		approvals,
		planByOID,
		productCache,
	)
	if err != nil {
		return State{}, err
	}
	tracks := make([]TrackState, 0, len(metadata.Tracks))
	flatSlices := make([]*SliceState, 0, len(states))
	trackRefs := make([]TrackRefState, 0, len(metadata.Tracks))
	for _, track := range metadata.Tracks {
		ref := byRef[trackRef(release, track.ID)]
		authorityHead := releaseCapture.Head
		if owned := trackHistories[track.ID].owned; len(owned) > 0 {
			authorityHead = owned[len(owned)-1].OID
		}
		trackState := TrackState{
			ID: track.ID, DependsOn: append([]string(nil), track.DependsOn...),
			Ref: ref.Ref, Head: ref.Head, AuthorityHead: authorityHead,
		}
		trackRefs = append(trackRefs, TrackRefState{ID: track.ID, CapturedRef: ref})
		for _, plannedSlice := range track.Slices {
			state := states[plannedSlice.ID]
			for index := range state.ConsumedInputs {
				input := &state.ConsumedInputs[index]
				producer := states[input.Slice]
				source := byRef[trackRef(release, producer.Location.Track.ID)]
				input.SourceRef, input.SourceHead = source.Ref, source.Head
				if !directCommit(source) {
					return State{}, recordFail(
						"AMBIGUOUS_AUTHORITY",
						"consumed slice "+input.Slice+" has no direct producer authority",
					)
				}
				contained, err := repository.isAncestor(input.PassReceipt, source.Head)
				if err != nil {
					return State{}, err
				}
				if !contained {
					return State{}, recordFail(
						"AMBIGUOUS_AUTHORITY",
						"consumed PASS "+input.PassReceipt+" is absent from producer authority",
					)
				}
			}
			trackState.Slices = append(trackState.Slices, state)
			flatSlices = append(flatSlices, state)
		}
		tracks = append(tracks, trackState)
	}
	trackProductBaseFor := func(trackID string) (string, error) {
		for _, track := range metadata.Tracks {
			if track.ID != trackID {
				continue
			}
			if len(track.Slices) == 0 {
				break
			}
			first := states[track.Slices[0].ID]
			if first == nil || first.Pass == nil {
				return "", recordFail(
					"AMBIGUOUS_AUTHORITY",
					"track "+trackID+" has no first-slice PASS",
				)
			}
			return productBaseResolver.baselineFor(
				track.Slices[0].ID,
				first.Pass.OID,
			)
		}
		return "", recordFail(
			"AMBIGUOUS_AUTHORITY",
			"track "+trackID+" is absent",
		)
	}
	for _, track := range tracks {
		var incomplete *SliceState
		for _, slice := range track.Slices {
			if slice.Pass == nil {
				incomplete = slice
				break
			}
		}
		var active *SliceState
		if incomplete != nil {
			if incomplete.NextRole == "verifier" ||
				incomplete.NextRole == "merge" {
				active = incomplete
			}
		} else if len(track.Slices) > 0 {
			active = track.Slices[len(track.Slices)-1]
		}
		// Only product work beyond the last accepted receipt is a head refresh.
		if track.Head != "" &&
			track.Head != track.AuthorityHead &&
			active != nil &&
			active.Candidate != nil &&
			(active.CurrentReceipt == nil ||
				track.Head != active.CurrentReceipt.OID) {
			awaitingCandidateVerdict := active.CurrentReceipt != nil &&
				active.Stage == "verify" &&
				active.NextRole == "verifier" &&
				active.CurrentReceipt.OID == active.Candidate.OID
			if !awaitingCandidateVerdict {
				return State{}, recordFail(
					"CHANGED_CANDIDATE",
					"track "+track.ID+" moved after its current candidate",
				)
			}
			linear, err := linearOneParentAncestry(
				repository,
				active.Candidate.OID,
				track.Head,
			)
			if err != nil {
				return State{}, err
			}
			if !linear {
				return State{}, recordFail(
					"CHANGED_CANDIDATE",
					"track "+track.ID+" moved after its current candidate",
				)
			}
			refreshHistory, err := historyAt(
				repository,
				track.Head,
				active.Candidate.OID,
			)
			if err != nil {
				return State{}, err
			}
			if len(refreshHistory.Receipts) > 0 {
				return State{}, recordFail(
					"CHANGED_CANDIDATE",
					"track "+track.ID+" moved after its current candidate",
				)
			}
			if err := repository.assertCandidateRecordRootUnchanged(
				active.Candidate.OID,
				track.Head,
			); err != nil {
				return State{}, err
			}
			active.Stage = "implement"
			active.Status = "ready"
			active.NextRole = "implementer"
			active.Outcome = "stale"
			active.Attempt = active.History.MaximumAttempt + 1
			active.Retained = false
			active.StaleReason =
				"track head changed before verification was recorded"
		}
		if incomplete != nil {
			currentApproval := approvals[current.OID]
			if currentApproval.Receipt.Target == nil {
				return State{}, recordFail(
					"APPROVAL_MISSING",
					"current plan approval has no target",
				)
			}
			preparedBase, err := projectedConsumedTrackBase(
				repository,
				track.Ref,
				track.Head,
				track.AuthorityHead,
				*currentApproval.Receipt.Target,
				incomplete.ConsumedInputs,
				productBaseResolver.baselineFor,
			)
			if err != nil {
				return State{}, err
			}
			incomplete.preparedAuthorityBase = preparedBase
			if len(incomplete.ConsumedInputs) > 0 {
				incomplete.PreparationSeed = track.AuthorityHead
				incomplete.PreparedBase = preparedBase
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
		topologies, allSliceEntries, productCache,
		current, tracks, releaseHistory.Receipts,
		trackProductBaseFor,
	)
	if err != nil {
		return State{}, err
	}
	approval := approvals[current.OID]
	assembly, err := deriveAssembly(
		repository, current, assemblyHistory, approval, tracks,
		topologies[current.OID], targetCapture.Head, releaseCapture.Head,
		trackProductBaseFor,
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
	targetStale := false
	if assembly.Status != "complete" && approval.Receipt.Target != nil {
		contained, err := repository.isAncestor(
			*approval.Receipt.Target,
			targetCapture.Head,
		)
		if err != nil {
			return State{}, err
		}
		targetStale = !contained
	}

	var diagnostics []Diagnostic
	if targetStale {
		diagnostics = append(diagnostics, Diagnostic{
			Code: "TARGET_DIVERGED", Release: release,
			Message: "the target no longer contains the approved starting point; reconcile its history",
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
		approval := approvals[entry.OID]
		installHead, retirements, err := planInstallResult(
			entry.OID, approval, releaseHistory.Receipts)
		if err != nil {
			return State{}, err
		}
		planHistory[index] = PlanHistory{
			OID: entry.OID, Revision: entry.Parsed.Metadata().Revision,
			Approval: approval.Clone(), Plan: entry.Parsed,
			InstallHead: installHead, Retirements: retirements,
		}
	}
	return State{
		Release: release, Repository: metadata.Repository,
		Plan: PlanState{
			OID: current.OID, Digest: current.Parsed.Digest(), Metadata: metadata,
			Approval: approval.Clone(), ApprovalOID: approval.OID,
			TargetStale: targetStale, LegacyFallback: current.LegacyFallback, History: planHistory,
		},
		Refs: StateRefs{
			Release: releaseCapture, Target: targetCapture, Tracks: trackRefs,
		},
		Tracks: tracks, Slices: flatSlices, SliceHistories: sliceHistories,
		Assembly:    assembly,
		Diagnostics: diagnostics,
		productBases: &productBaseEvidence{
			pass:  productBaseResolver.baselineFor,
			track: trackProductBaseFor,
		},
	}, nil
}

func planInstallResult(
	planOID string,
	approval ReceiptEntry,
	receipts []ReceiptEntry,
) (string, []RetirementResult, error) {
	head := approval.OID
	var retirements []RetirementResult
	for _, entry := range receipts {
		receipt := entry.Receipt
		if receipt.Role != "planner" ||
			receipt.Result != "retired" ||
			receipt.Plan != planOID ||
			receipt.Binds != approval.OID {
			continue
		}
		if receipt.Slice == nil || entry.Parent != head {
			return "", nil, recordFail(
				"INVALID_RETIREMENT",
				"retirements for plan "+planOID+
					" do not form the exact post-approval chain",
			)
		}
		retirements = append(retirements, RetirementResult{
			Slice:         *receipt.Slice,
			ReceiptCommit: entry.OID,
			Receipt:       receipt.Clone(),
		})
		head = entry.OID
	}
	return head, retirements, nil
}

// planFileAt resolves the exact plan file for one release at one commit: the
// configured records root first, falling back to the historical legacy root
// only when the configured root holds no record there. A release present
// under both roots resolves to the configured root — one authority, never
// two — so releases recorded before the relocation stay readable.
func planFileAt(repository *repository, commit, release string) (repositoryFile, bool, error) {
	file, err := repository.file(commit, planPath(repository.recordRoot(), release))
	if err != nil {
		return repositoryFile{}, false, err
	}
	if file.Present {
		return file, false, nil
	}
	legacyFile, err := repository.file(commit, planPath(LegacyRecordRoot, release))
	if err != nil {
		return repositoryFile{}, false, err
	}
	return legacyFile, legacyFile.Present, nil
}

func planAt(repository *repository, release, commit string) (planEntry, error) {
	file, legacyFallback, err := planFileAt(repository, commit, release)
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
	return planEntry{OID: file.Object, Parsed: parsed, LegacyFallback: legacyFallback}, nil
}

func historyBoundaryFailure(rows []historyRow, label string) error {
	if len(rows) == maxCandidateLineage {
		return recordFail(
			"RESOURCE_LIMIT",
			label+" exceeds the bounded first-parent history limit",
		)
	}
	return recordFail(
		"HISTORY_BOUNDARY_MISSING",
		label+" is absent from the exact first-parent history",
	)
}

func historyEntryAt(rows []historyRow, index int) (ReceiptEntry, error) {
	row := rows[index]
	if index+1 >= len(rows) ||
		len(row.Parents) != 1 ||
		row.Parents[0] != rows[index+1].OID {
		return ReceiptEntry{}, recordFail(
			"HISTORY_LIMIT",
			"cannot establish the parent tree for receipt "+row.OID,
		)
	}
	entry, err := ParseReceiptHistoryEntry(HistoryEnvelope{
		OID: row.OID, Parents: row.Parents, Tree: row.Tree,
		ParentTree: rows[index+1].Tree, Message: row.Message,
	})
	if err != nil {
		return ReceiptEntry{}, recordWrap(
			ErrorCode(err),
			"invalid receipt "+row.OID,
			err,
		)
	}
	return entry, nil
}

func historyAt(
	repository *repository,
	head string,
	exclusiveBoundary string,
) (receiptHistory, error) {
	if head == "" {
		return receiptHistory{
			Boundary: exclusiveBoundary,
			ByOID:    make(map[string]ReceiptEntry),
		}, nil
	}
	rows, err := repository.history(head)
	if err != nil {
		return receiptHistory{}, err
	}
	var receipts []ReceiptEntry
	boundaryIndex := len(rows)
	for index, row := range rows {
		if row.OID == exclusiveBoundary && exclusiveBoundary != "" {
			boundaryIndex = index
			break
		}
		parentEstablished := index+1 < len(rows) &&
			len(row.Parents) > 0 &&
			row.Parents[0] == rows[index+1].OID
		if !parentEstablished && exclusiveBoundary != "" {
			return receiptHistory{}, historyBoundaryFailure(
				rows,
				"history boundary "+exclusiveBoundary,
			)
		}
		if !bytes.Contains(row.Message, []byte("\n"+ReceiptTrailer)) {
			continue
		}
		entry, err := historyEntryAt(rows, index)
		if err != nil {
			return receiptHistory{}, err
		}
		receipts = append(receipts, entry)
	}
	if exclusiveBoundary != "" && boundaryIndex == len(rows) {
		return receiptHistory{}, historyBoundaryFailure(
			rows,
			"history boundary "+exclusiveBoundary,
		)
	}
	rows = append([]historyRow(nil), rows[:boundaryIndex]...)
	reverseRows(rows)
	reverseReceipts(receipts)
	byOID := make(map[string]ReceiptEntry, len(receipts))
	for _, entry := range receipts {
		byOID[entry.OID] = entry
	}
	return receiptHistory{
		Boundary: exclusiveBoundary,
		Rows:     rows,
		Receipts: receipts,
		ByOID:    byOID,
	}, nil
}

func assertPlanPredecessor(current, prior planEntry) error {
	nextMetadata := current.Parsed.Metadata()
	priorMetadata := prior.Parsed.Metadata()
	if priorMetadata.Revision != nextMetadata.Revision-1 ||
		priorMetadata.Release != nextMetadata.Release ||
		priorMetadata.Repository != nextMetadata.Repository ||
		priorMetadata.TargetRef != nextMetadata.TargetRef ||
		priorMetadata.ApprovalRef == nextMetadata.ApprovalRef {
		return recordFail(
			"INVALID_PLAN_HISTORY",
			fmt.Sprintf(
				"plan revision %d has a broken predecessor",
				nextMetadata.Revision,
			),
		)
	}
	return nil
}

// readReleaseReceiptHistory scopes authority to the current release epoch.
// The revision-1 approved target is the exclusive floor: inherited receipts
// below it are archaeology, while every receipt above it is parsed before
// ownership filtering.
func readReleaseReceiptHistory(
	repository *repository,
	release string,
	head string,
) (receiptHistory, error) {
	if head == "" {
		return receiptHistory{ByOID: make(map[string]ReceiptEntry)}, nil
	}
	current, err := planAt(repository, release, head)
	if err != nil {
		return receiptHistory{}, err
	}
	rows, err := repository.history(head)
	if err != nil {
		return receiptHistory{}, err
	}
	var receipts []ReceiptEntry
	var lineage []planEntry
	expectedOID := current.OID
	expectedRevision := current.Parsed.Metadata().Revision
	boundary := ""
	boundaryIndex := len(rows)
	for index, row := range rows {
		if boundary != "" && row.OID == boundary {
			boundaryIndex = index
			break
		}
		parentEstablished := index+1 < len(rows) &&
			len(row.Parents) > 0 &&
			row.Parents[0] == rows[index+1].OID
		if !parentEstablished {
			label := "revision-1 approval for " + release
			if boundary != "" {
				label = "release epoch boundary " + boundary
			}
			return receiptHistory{}, historyBoundaryFailure(rows, label)
		}
		if !bytes.Contains(row.Message, []byte("\n"+ReceiptTrailer)) {
			continue
		}
		entry, err := historyEntryAt(rows, index)
		if err != nil {
			return receiptHistory{}, err
		}
		receipts = append(receipts, entry)
		receipt := entry.Receipt
		if boundary != "" ||
			receipt.Role != "planner" ||
			receipt.Result != "approved" ||
			receipt.Slice != nil ||
			receipt.Plan != expectedOID ||
			receipt.Release != release {
			continue
		}
		file, _, err := planFileAt(repository, entry.OID, release)
		if err != nil {
			return receiptHistory{}, err
		}
		if receipt.Binds != entry.Parent ||
			!file.Present ||
			file.Object != expectedOID {
			return receiptHistory{}, recordFail(
				"STALE_BINDING",
				"approval "+entry.OID+" does not bind its plan commit",
			)
		}
		parsed, err := ParsePlan(file.Bytes)
		if err != nil {
			return receiptHistory{}, err
		}
		metadata := parsed.Metadata()
		if metadata.Release != release ||
			metadata.Revision != expectedRevision {
			return receiptHistory{}, recordFail(
				"INVALID_PLAN_HISTORY",
				"approval "+entry.OID+" has stale plan topology",
			)
		}
		approved := planEntry{OID: file.Object, Parsed: parsed}
		lineage = append(lineage, approved)
		if metadata.PreviousPlan != nil {
			for _, prior := range receipts {
				candidate := prior.Receipt
				if candidate.Role == "planner" &&
					candidate.Result == "approved" &&
					candidate.Slice == nil &&
					candidate.Plan == *metadata.PreviousPlan &&
					candidate.Release == release {
					return receiptHistory{}, recordFail(
						"INVALID_PLAN_HISTORY",
						"approval for previous plan "+
							*metadata.PreviousPlan+
							" is out of order",
					)
				}
			}
			expectedOID = *metadata.PreviousPlan
			expectedRevision = metadata.Revision - 1
			continue
		}
		if metadata.Revision != 1 {
			return receiptHistory{}, recordFail(
				"INVALID_PLAN_HISTORY",
				"plan history does not terminate at revision 1",
			)
		}
		if receipt.Target == nil {
			return receiptHistory{}, recordFail(
				"INVALID_PLAN_HISTORY",
				"revision-1 approval "+entry.OID+
					" does not install directly above its target",
			)
		}
		planParents, err := repository.parents(entry.Parent)
		if err != nil {
			return receiptHistory{}, err
		}
		if len(planParents) != 1 || planParents[0] != *receipt.Target {
			return receiptHistory{}, recordFail(
				"INVALID_PLAN_HISTORY",
				"revision-1 approval "+entry.OID+
					" does not install directly above its target",
			)
		}
		configuredFloor, err := repository.file(*receipt.Target, planPath(repository.recordRoot(), release))
		if err != nil {
			return receiptHistory{}, err
		}
		legacyFloor, err := repository.file(*receipt.Target, planPath(LegacyRecordRoot, release))
		if err != nil {
			return receiptHistory{}, err
		}
		if configuredFloor.Present || legacyFloor.Present {
			return receiptHistory{}, recordFail(
				"INVALID_PLAN_HISTORY",
				"revision-1 target "+*receipt.Target+
					" already contains release "+release,
			)
		}
		configuredChange, err := repository.firstParentPathChange(
			*receipt.Target,
			planPath(repository.recordRoot(), release),
		)
		if err != nil {
			return receiptHistory{}, err
		}
		legacyChange, err := repository.firstParentPathChange(
			*receipt.Target,
			planPath(LegacyRecordRoot, release),
		)
		if err != nil {
			return receiptHistory{}, err
		}
		if configuredChange != "" || legacyChange != "" {
			priorChange := configuredChange
			if priorChange == "" {
				priorChange = legacyChange
			}
			return receiptHistory{}, recordFail(
				"INVALID_PLAN_HISTORY",
				"revision-1 plan path was already introduced at "+
					priorChange,
			)
		}
		boundary = *receipt.Target
	}
	if boundary == "" || boundaryIndex == len(rows) {
		label := "revision-1 approval for " + release
		if boundary != "" {
			label = "release epoch boundary " + boundary
		}
		return receiptHistory{}, historyBoundaryFailure(rows, label)
	}
	for index := 0; index+1 < len(lineage); index++ {
		if err := assertPlanPredecessor(
			lineage[index],
			lineage[index+1],
		); err != nil {
			return receiptHistory{}, err
		}
	}
	lineageOIDs := make(map[string]bool, len(lineage))
	for _, plan := range lineage {
		lineageOIDs[plan.OID] = true
	}
	for planOID := range lineageOIDs {
		matches := 0
		for _, entry := range receipts {
			receipt := entry.Receipt
			if receipt.Role == "planner" &&
				receipt.Result == "approved" &&
				receipt.Slice == nil &&
				receipt.Plan == planOID {
				matches++
			}
		}
		if matches != 1 {
			code := "APPROVAL_MISSING"
			if matches > 1 {
				code = "AMBIGUOUS_APPROVAL"
			}
			return receiptHistory{}, recordFail(
				code,
				fmt.Sprintf(
					"plan %s has %d approvals inside its release epoch",
					planOID,
					matches,
				),
			)
		}
	}
	for _, entry := range receipts {
		receipt := entry.Receipt
		if receipt.Release != release {
			return receiptHistory{}, recordFail(
				"RELEASE_RECEIPT_MISMATCH",
				"receipt "+entry.OID+" names another release",
			)
		}
		if receipt.Role == "planner" &&
			receipt.Result == "approved" &&
			!lineageOIDs[receipt.Plan] {
			return receiptHistory{}, recordFail(
				"AMBIGUOUS_APPROVAL",
				"approval "+entry.OID+
					" is outside the current plan lineage",
			)
		}
	}
	rows = append([]historyRow(nil), rows[:boundaryIndex]...)
	reverseRows(rows)
	reverseReceipts(receipts)
	byOID := make(map[string]ReceiptEntry, len(receipts))
	for _, entry := range receipts {
		byOID[entry.OID] = entry
	}
	return receiptHistory{
		Boundary: boundary,
		Rows:     rows,
		Receipts: receipts,
		ByOID:    byOID,
	}, nil
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

func predecessorIDs(location SliceLocation) []string {
	position := -1
	for index := range location.Track.Slices {
		if location.Track.Slices[index].ID == location.Slice.ID {
			position = index
			break
		}
	}
	if position <= 0 {
		return []string{}
	}
	result := make([]string, position)
	for index := range location.Track.Slices[:position] {
		result[index] = location.Track.Slices[index].ID
	}
	return result
}

func sameIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// slicePlanLineage returns the contiguous plan suffix in which a slice has the
// same authority-bearing identity: ID, track, contract, and ordered serial
// predecessors. A later return to an older shape cannot bridge a changed plan.
func slicePlanLineage(
	planByOID map[string]planEntry,
	current planEntry,
	sliceID string,
) map[string]bool {
	currentLocation, ok := locations(current.Parsed)[sliceID]
	if !ok {
		return map[string]bool{}
	}
	contract := current.Parsed.Metadata().Contracts[sliceID]
	trackID := currentLocation.Track.ID
	predecessors := predecessorIDs(currentLocation)
	lineage := make(map[string]bool)
	cursor := current
	for {
		location, present := locations(cursor.Parsed)[sliceID]
		if !present || location.Track.ID != trackID ||
			cursor.Parsed.Metadata().Contracts[sliceID] != contract ||
			!sameIDs(predecessorIDs(location), predecessors) {
			break
		}
		lineage[cursor.OID] = true
		previous := cursor.Parsed.Metadata().PreviousPlan
		if previous == nil {
			break
		}
		prior, present := planByOID[*previous]
		if !present {
			break
		}
		cursor = prior
	}
	return lineage
}

type assemblyTopology struct {
	DirectSlice string
}

func topologyForPlan(plan Plan) assemblyTopology {
	tracks := plan.Metadata().Tracks
	if len(tracks) != 1 || len(tracks[0].Slices) != 1 {
		return assemblyTopology{}
	}
	return assemblyTopology{
		DirectSlice: tracks[0].Slices[0].ID,
	}
}

func assemblyTopologies(chain []planEntry) map[string]assemblyTopology {
	result := make(map[string]assemblyTopology, len(chain))
	for _, entry := range chain {
		result[entry.OID] = topologyForPlan(entry.Parsed)
	}
	return result
}

func topologyFromPlanHistory(history []PlanHistory) assemblyTopology {
	if len(history) == 0 {
		return assemblyTopology{}
	}
	return topologyForPlan(history[len(history)-1].Plan)
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
	file, _, err := planFileAt(repository, approval.OID, release)
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
		file, _, err := planFileAt(repository, approval.OID, release)
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
				return nil, nil, recordFail(
					"REPLACED_SLICE_AUTHORITY",
					"slice "+id+" moved between tracks",
				)
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
	chain []planEntry,
	approvals map[string]ReceiptEntry,
	receipts []ReceiptEntry,
) error {
	var retired []ReceiptEntry
	for _, entry := range receipts {
		if entry.Receipt.Role == "planner" && entry.Receipt.Result == "retired" {
			retired = append(retired, entry)
		}
	}
	matched := make(map[string]bool)
	retiredIDs := make(map[string]bool)
	for index := 1; index < len(chain); index++ {
		prior, next := chain[index-1], chain[index]
		priorLocations, nextLocations := locations(prior.Parsed), locations(next.Parsed)
		for id := range nextLocations {
			if retiredIDs[id] {
				return recordFail("INVALID_RETIREMENT", "retired slice "+id+" cannot be re-added")
			}
		}
		for id := range priorLocations {
			if _, present := nextLocations[id]; present {
				continue
			}
			var matches []ReceiptEntry
			for _, entry := range retired {
				receipt := entry.Receipt
				if receipt.Slice != nil && *receipt.Slice == id &&
					receipt.Plan == next.OID &&
					receipt.Binds == approvals[next.OID].OID &&
					receipt.Contract != nil &&
					*receipt.Contract == prior.Parsed.Metadata().Contracts[id] {
					matches = append(matches, entry)
				}
			}
			if len(matches) == 0 {
				return recordFail("RETIREMENT_MISSING", "removed slice "+id+" requires one retirement at its first absent revision")
			}
			if len(matches) != 1 {
				return recordFail("INVALID_RETIREMENT", "removed slice "+id+" has duplicate retirements")
			}
			matched[matches[0].OID] = true
			retiredIDs[id] = true
		}
	}
	for _, entry := range retired {
		if !matched[entry.OID] {
			return recordFail("INVALID_RETIREMENT", "retirement "+entry.OID+" does not bind one first-removal transition")
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

func applicablePriorPass(
	entries []ReceiptEntry,
	plan planEntry,
	sliceID string,
	planByOID map[string]planEntry,
) *ReceiptEntry {
	contract := plan.Parsed.Metadata().Contracts[sliceID]
	lineage := slicePlanLineage(planByOID, plan, sliceID)
	var matching []ReceiptEntry
	for _, entry := range entries {
		if entry.Receipt.Slice != nil && *entry.Receipt.Slice == sliceID &&
			entry.Receipt.Contract != nil && *entry.Receipt.Contract == contract &&
			lineage[entry.Receipt.Plan] {
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
			if applicablePriorPass(priorEntries, plan, prior.ID, planByOID) == nil {
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
		if !ok || !plannedOK {
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
		sameLineage := sameSlice &&
			slicePlanLineage(planByOID, plan, receipt.SliceID())[bound.Receipt.Plan]
		switch {
		case receipt.Role == "implementer" && receipt.Result == "designed":
			approved := boundOK && bound.Receipt.Role == "planner" &&
				bound.Receipt.Result == "approved" && samePlan
			retry := sameLineage && bound.Receipt.Attempt != nil &&
				*bound.Receipt.Attempt == attempt-1 &&
				((bound.Receipt.Role == "captain" && bound.Receipt.Result == "revise") ||
					(bound.Receipt.Role == "verifier" && bound.Receipt.Result == "fail"))
			staleReviewRetry := sameLineage && bound.Receipt.Attempt != nil &&
				*bound.Receipt.Attempt == attempt-1 &&
				((bound.Receipt.Role == "implementer" &&
					(bound.Receipt.Result == "designed" ||
						bound.Receipt.Result == "candidate")) ||
					(bound.Receipt.Role == "captain" &&
						bound.Receipt.Result == "proceed") ||
					(bound.Receipt.Role == "verifier" &&
						bound.Receipt.Result == "pass"))
			if !approved && !retry && !staleReviewRetry {
				return SliceHistory{}, recordFail("STALE_BINDING", "design "+entry.OID+" has no predecessor")
			}
			if receipt.Inputs != nil {
				if len(planned.Slice.Consumes) == 0 {
					return SliceHistory{}, recordFail(
						"STALE_BINDING",
						"design "+entry.OID+" records inputs for a non-consuming slice",
					)
				}
				if err := exactInputs(
					receipt,
					planned.Slice.Consumes,
					"design "+entry.OID,
				); err != nil {
					return SliceHistory{}, err
				}
			}
		case receipt.Role == "captain":
			if !sameLineage || bound.Receipt.Role != "implementer" ||
				bound.Receipt.Result != "designed" || *bound.Receipt.Attempt != attempt {
				return SliceHistory{}, recordFail("STALE_BINDING", "Captain "+entry.OID+" does not bind its design")
			}
		case receipt.Role == "implementer" && receipt.Result == "candidate":
			proceeded := sameLineage && bound.Receipt.Role == "captain" &&
				bound.Receipt.Result == "proceed" && *bound.Receipt.Attempt == attempt
			retry := sameLineage && bound.Receipt.Role == "verifier" &&
				bound.Receipt.Result == "fail" && *bound.Receipt.Attempt == attempt-1
			candidateRefresh := false
			if sameLineage &&
				bound.Receipt.Role == "implementer" &&
				bound.Receipt.Result == "candidate" &&
				bound.Receipt.Attempt != nil &&
				*bound.Receipt.Attempt == attempt-1 &&
				receipt.Candidate != nil &&
				inputsEqual(receipt.Inputs, bound.Receipt.Inputs) {
				linear, err := linearOneParentAncestry(
					repository,
					receipt.Binds,
					*receipt.Candidate,
				)
				if err != nil {
					return SliceHistory{}, err
				}
				if linear {
					refreshHistory, err := historyAt(
						repository,
						*receipt.Candidate,
						receipt.Binds,
					)
					if err != nil {
						return SliceHistory{}, err
					}
					candidateRefresh =
						len(refreshHistory.Receipts) == 0
				}
			}
			staleRetry := sameLineage && bound.Receipt.Attempt != nil &&
				*bound.Receipt.Attempt == attempt-1 &&
				((bound.Receipt.Role == "implementer" && bound.Receipt.Result == "candidate") ||
					(bound.Receipt.Role == "verifier" &&
						(bound.Receipt.Result == "pass" || bound.Receipt.Result == "fail"))) &&
				!inputsEqual(receipt.Inputs, bound.Receipt.Inputs)
			if !proceeded && !retry && !candidateRefresh && !staleRetry {
				return SliceHistory{}, recordFail("STALE_BINDING", "candidate "+entry.OID+" lacks PROCEED")
			}
			if err := exactInputs(receipt, planned.Slice.Consumes, "candidate "+entry.OID); err != nil {
				return SliceHistory{}, err
			}
			if len(planned.Slice.Consumes) == 0 && receipt.Base != nil {
				return SliceHistory{}, recordFail(
					"STALE_BINDING",
					"candidate "+entry.OID+" records a base for a non-consuming slice",
				)
			}
			if receipt.Candidate == nil || receipt.ProductTree == nil ||
				entry.Parent != *receipt.Candidate || *receipt.Candidate == receipt.Binds {
				return SliceHistory{}, recordFail("CHANGED_CANDIDATE", "candidate "+entry.OID+" has invalid Git evidence")
			}
			deferConsumingEvidence := len(planned.Slice.Consumes) > 0 &&
				receipt.Base != nil
			implementationBase := receipt.Binds
			if len(planned.Slice.Consumes) == 0 {
				preparedBase, err := preparePlanBoundBase(
					repository,
					receipt.Release,
					plan,
					planned,
					receipt.Binds,
					nil,
					approvals,
					nil,
					receipt.Binds,
				)
				if err != nil {
					return SliceHistory{}, err
				}
				preparedAncestor, err := repository.isAncestor(
					preparedBase,
					*receipt.Candidate,
				)
				if err != nil {
					return SliceHistory{}, err
				}
				if !preparedAncestor {
					return SliceHistory{}, recordFail(
						"CHANGED_CANDIDATE",
						"candidate "+entry.OID+" omits its exact prepared base",
					)
				}
				implementationBase = preparedBase
			}
			if !deferConsumingEvidence {
				if err := repository.assertCandidateRecordRootUnchanged(
					implementationBase,
					*receipt.Candidate,
				); err != nil {
					return SliceHistory{}, err
				}
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
			if !ancestor || !baseAncestor {
				return SliceHistory{}, recordFail("CHANGED_CANDIDATE", "candidate "+entry.OID+" has invalid Git evidence")
			}
			if !deferConsumingEvidence {
				product, err := productTreeFor(repository, *receipt.Candidate, productCache)
				if err != nil {
					return SliceHistory{}, err
				}
				if product != *receipt.ProductTree {
					return SliceHistory{}, recordFail("CHANGED_CANDIDATE", "candidate "+entry.OID+" has invalid Git evidence")
				}
			}
		case receipt.Role == "verifier":
			if !sameLineage || bound.Receipt.Role != "implementer" ||
				bound.Receipt.Result != "candidate" || *bound.Receipt.Attempt != attempt ||
				entry.Parent != bound.OID || !sameCandidate(receipt, bound.Receipt) {
				return SliceHistory{}, recordFail("STALE_BINDING", "Verifier "+entry.OID+" does not bind its candidate")
			}
		default:
			return SliceHistory{}, recordFail("INVALID_RECEIPT", "slice "+receipt.SliceID()+" has an unsupported receipt")
		}
		byOID[entry.OID] = entry
	}
	return SliceHistory{Entries: entries, MaximumAttempt: maximum}, nil
}

func governingDesign(history SliceHistory, current *ReceiptEntry) *ReceiptEntry {
	if current == nil {
		return nil
	}
	byOID := make(map[string]ReceiptEntry, len(history.Entries))
	for _, entry := range history.Entries {
		byOID[entry.OID] = entry
	}
	cursor := current
	for steps := 0; cursor != nil && steps <= len(history.Entries); steps++ {
		if cursor.Receipt.Role == "implementer" &&
			cursor.Receipt.Result == "designed" {
			value := cursor.Clone()
			return &value
		}
		next, present := byOID[cursor.Receipt.Binds]
		if !present {
			return nil
		}
		value := next
		cursor = &value
	}
	return nil
}

func consumedInputForPass(
	repository *repository,
	sliceID string,
	history SliceHistory,
	pass ReceiptEntry,
	productCache map[string]string,
) (ConsumedInput, error) {
	if pass.Receipt.Role != "verifier" || pass.Receipt.Result != "pass" ||
		pass.Receipt.SliceID() != sliceID || pass.Receipt.Candidate == nil ||
		pass.Receipt.ProductTree == nil {
		return ConsumedInput{}, recordFail(
			"STALE_BINDING",
			"consumed slice "+sliceID+" has invalid PASS authority",
		)
	}
	candidate := findEntry(history.Entries, pass.Receipt.Binds)
	if candidate == nil || candidate.Receipt.Role != "implementer" ||
		candidate.Receipt.Result != "candidate" ||
		candidate.Receipt.Candidate == nil ||
		candidate.Receipt.ProductTree == nil ||
		pass.Parent != candidate.OID ||
		candidate.Parent != *candidate.Receipt.Candidate ||
		!sameCandidate(pass.Receipt, candidate.Receipt) {
		return ConsumedInput{}, recordFail(
			"STALE_BINDING",
			"consumed PASS "+pass.OID+" has no exact candidate chain",
		)
	}
	for _, commit := range []string{
		*candidate.Receipt.Candidate,
		candidate.OID,
		pass.OID,
	} {
		product, err := productTreeFor(repository, commit, productCache)
		if err != nil {
			return ConsumedInput{}, err
		}
		if product != *pass.Receipt.ProductTree {
			return ConsumedInput{}, recordFail(
				"CHANGED_CANDIDATE",
				"consumed PASS "+pass.OID+" changed product identity",
			)
		}
	}
	return ConsumedInput{
		Slice:            sliceID,
		PassReceipt:      pass.OID,
		CandidateReceipt: candidate.OID,
		Candidate:        *candidate.Receipt.Candidate,
		ProductTree:      *pass.Receipt.ProductTree,
	}, nil
}

func consumedInputsAtBase(
	repository *repository,
	plan planEntry,
	base string,
	consumes []string,
	histories map[string]SliceHistory,
	planByOID map[string]planEntry,
	productCache map[string]string,
) ([]ConsumedInput, bool, error) {
	result := make([]ConsumedInput, 0, len(consumes))
	for _, dependency := range consumes {
		lineage := slicePlanLineage(planByOID, plan, dependency)
		contract := plan.Parsed.Metadata().Contracts[dependency]
		var selected *ReceiptEntry
		for _, entry := range histories[dependency].Entries {
			receipt := entry.Receipt
			if receipt.Role != "verifier" || receipt.Result != "pass" ||
				receipt.Contract == nil || *receipt.Contract != contract ||
				!lineage[receipt.Plan] {
				continue
			}
			ancestor, err := repository.isAncestor(entry.OID, base)
			if err != nil {
				return nil, false, err
			}
			if !ancestor {
				continue
			}
			if selected != nil {
				ordered, err := repository.isAncestor(selected.OID, entry.OID)
				if err != nil {
					return nil, false, err
				}
				if !ordered {
					return nil, false, recordFail(
						"AMBIGUOUS_AUTHORITY",
						"consumed slice "+dependency+" has incomparable PASS authority",
					)
				}
			}
			value := entry
			selected = &value
		}
		if selected == nil {
			return nil, false, nil
		}
		input, err := consumedInputForPass(
			repository, dependency, histories[dependency], *selected, productCache,
		)
		if err != nil {
			return nil, false, err
		}
		result = append(result, input)
	}
	return result, true, nil
}

func legacyConsumedInputs(
	repository *repository,
	plan planEntry,
	candidate ReceiptEntry,
	consumes []string,
	histories map[string]SliceHistory,
	planByOID map[string]planEntry,
	productCache map[string]string,
) ([]ConsumedInput, error) {
	result := make([]ConsumedInput, 0, len(consumes))
	for _, dependency := range consumes {
		lineage := slicePlanLineage(planByOID, plan, dependency)
		contract := plan.Parsed.Metadata().Contracts[dependency]
		product := candidate.Receipt.Inputs[dependency]
		var matches []ReceiptEntry
		for _, entry := range histories[dependency].Entries {
			receipt := entry.Receipt
			if receipt.Role == "verifier" &&
				receipt.Result == "pass" &&
				receipt.Contract != nil &&
				*receipt.Contract == contract &&
				lineage[receipt.Plan] &&
				receipt.ProductTree != nil &&
				*receipt.ProductTree == product {
				matches = append(matches, entry)
			}
		}
		if len(matches) == 0 {
			return nil, recordFail(
				"AMBIGUOUS_AUTHORITY",
				"legacy candidate "+candidate.OID+
					" has no exact "+dependency+" PASS authority",
			)
		}
		var selected *ReceiptEntry
		for index := range matches {
			protected, err := repository.isAncestor(
				matches[index].OID,
				*candidate.Receipt.Candidate,
			)
			if err != nil {
				return nil, err
			}
			if !protected {
				continue
			}
			if selected == nil {
				value := matches[index]
				selected = &value
				continue
			}
			after, err := repository.isAncestor(
				selected.OID,
				matches[index].OID,
			)
			if err != nil {
				return nil, err
			}
			if after {
				value := matches[index]
				selected = &value
				continue
			}
			before, err := repository.isAncestor(
				matches[index].OID,
				selected.OID,
			)
			if err != nil {
				return nil, err
			}
			if !before {
				selected = nil
				break
			}
		}
		if selected == nil && len(matches) == 1 {
			value := matches[0]
			selected = &value
		}
		if selected == nil {
			return nil, recordFail(
				"AMBIGUOUS_AUTHORITY",
				"legacy candidate "+candidate.OID+
					" has ambiguous "+dependency+" PASS authorities",
			)
		}
		input, err := consumedInputForPass(
			repository,
			dependency,
			histories[dependency],
			*selected,
			productCache,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, input)
	}
	return result, nil
}

type passProductBaseResolver struct {
	repository   *repository
	release      string
	histories    map[string]SliceHistory
	planByOID    map[string]planEntry
	approvals    map[string]ReceiptEntry
	productCache map[string]string
	memo         map[string]string
	pending      map[string]bool
}

func newPassProductBaseResolver(
	repository *repository,
	release string,
	histories map[string]SliceHistory,
	planByOID map[string]planEntry,
	approvals map[string]ReceiptEntry,
	productCache map[string]string,
) *passProductBaseResolver {
	return &passProductBaseResolver{
		repository: repository, release: release,
		histories: histories, planByOID: planByOID,
		approvals: approvals, productCache: productCache,
		memo: make(map[string]string), pending: make(map[string]bool),
	}
}

func (r *passProductBaseResolver) passEntry(
	sliceID string,
	passOID string,
) (ReceiptEntry, error) {
	for _, entry := range r.histories[sliceID].Entries {
		if entry.OID == passOID {
			return entry, nil
		}
	}
	return ReceiptEntry{}, recordFail(
		"AMBIGUOUS_AUTHORITY",
		sliceID+" PASS "+passOID+" is absent",
	)
}

func (r *passProductBaseResolver) baselineFor(
	sliceID string,
	passOID string,
) (string, error) {
	key := sliceID + ":" + passOID
	if value, present := r.memo[key]; present {
		return value, nil
	}
	if r.pending[key] {
		return "", recordFail(
			"DEPENDENCY_CYCLE",
			"product-base dependency cycle reaches "+sliceID,
		)
	}
	r.pending[key] = true
	defer delete(r.pending, key)

	pass, err := r.passEntry(sliceID, passOID)
	if err != nil {
		return "", err
	}
	plan, planPresent := r.planByOID[pass.Receipt.Plan]
	location, locationPresent := locations(plan.Parsed)[sliceID]
	approval, approvalPresent := r.approvals[plan.OID]
	if !planPresent || !locationPresent || !approvalPresent ||
		approval.Receipt.Target == nil {
		return "", recordFail(
			"AMBIGUOUS_AUTHORITY",
			sliceID+" PASS "+pass.OID+" has no approved plan",
		)
	}
	candidate := findEntry(
		r.histories[sliceID].Entries,
		pass.Receipt.Binds,
	)
	if candidate == nil ||
		candidate.Receipt.Role != "implementer" ||
		candidate.Receipt.Result != "candidate" ||
		candidate.Receipt.Candidate == nil {
		return "", recordFail(
			"AMBIGUOUS_AUTHORITY",
			sliceID+" PASS "+pass.OID+" has no exact candidate",
		)
	}
	priorInputs, priorComplete, err := consumedInputsAtBase(
		r.repository,
		plan,
		pass.OID,
		predecessorIDs(location),
		r.histories,
		r.planByOID,
		r.productCache,
	)
	if err != nil {
		return "", err
	}
	if !priorComplete {
		return "", recordFail(
			"AMBIGUOUS_AUTHORITY",
			sliceID+" PASS "+pass.OID+" omits prior slice authority",
		)
	}
	var consumedInputs []ConsumedInput
	if len(location.Slice.Consumes) > 0 {
		if candidate.Receipt.Base != nil {
			var complete bool
			consumedInputs, complete, err = consumedInputsAtBase(
				r.repository,
				plan,
				*candidate.Receipt.Base,
				location.Slice.Consumes,
				r.histories,
				r.planByOID,
				r.productCache,
			)
			if err != nil {
				return "", err
			}
			if !complete {
				consumedInputs = nil
			}
		} else {
			consumedInputs, err = legacyConsumedInputs(
				r.repository,
				plan,
				*candidate,
				location.Slice.Consumes,
				r.histories,
				r.planByOID,
				r.productCache,
			)
			if err != nil {
				return "", err
			}
		}
		if consumedInputs == nil ||
			!inputsEqual(
				candidate.Receipt.Inputs,
				pinsForConsumedInputs(consumedInputs),
			) {
			return "", recordFail(
				"AMBIGUOUS_AUTHORITY",
				sliceID+" candidate "+candidate.OID+
					" has no exact consumed PASS bindings",
			)
		}
	}
	baseline := *approval.Receipt.Target
	inputs := append(
		append([]ConsumedInput(nil), priorInputs...),
		consumedInputs...,
	)
	for _, input := range inputs {
		dependencyPass, err := r.passEntry(
			input.Slice,
			input.PassReceipt,
		)
		if err != nil {
			return "", err
		}
		contained, err := r.repository.isAncestor(
			input.Candidate,
			baseline,
		)
		if err != nil {
			return "", err
		}
		if input.Candidate == baseline || contained {
			continue
		}
		prepared, err := r.repository.prepareProductComposition(
			trackRef(r.release, location.Track.ID),
			baseline,
			input.Candidate,
			func() (string, error) {
				return r.baselineFor(
					input.Slice,
					dependencyPass.OID,
				)
			},
		)
		if err != nil {
			return "", err
		}
		baseline = prepared.Result
	}
	r.memo[key] = baseline
	return baseline, nil
}

func reviewedConsumedInputs(
	repository *repository,
	design ReceiptEntry,
	consumes []string,
	histories map[string]SliceHistory,
	planByOID map[string]planEntry,
	productCache map[string]string,
) ([]ConsumedInput, bool, error) {
	plan, present := planByOID[design.Receipt.Plan]
	if !present || design.Parent == "" {
		return nil, false, recordFail(
			"STALE_BINDING",
			"design "+design.OID+" has no reviewed base",
		)
	}
	return consumedInputsAtBase(
		repository,
		plan,
		design.Parent,
		consumes,
		histories,
		planByOID,
		productCache,
	)
}

func pinsForConsumedInputs(inputs []ConsumedInput) map[string]string {
	result := make(map[string]string, len(inputs))
	for _, input := range inputs {
		result[input.Slice] = input.ProductTree
	}
	return result
}

func linearOneParentAncestry(
	repository *repository,
	base string,
	candidate string,
) (bool, error) {
	cursor := candidate
	for steps := 0; steps < maxCandidateLineage; steps++ {
		if cursor == base {
			return true, nil
		}
		parents, err := repository.parents(cursor)
		if err != nil {
			return false, err
		}
		if len(parents) != 1 {
			return false, nil
		}
		cursor = parents[0]
	}
	return false, recordFail(
		"RESOURCE_LIMIT",
		"candidate lineage exceeds the bounded history limit",
	)
}

type preparedDesignInputsCache struct {
	values  map[string][]ConsumedInput
	present map[string]bool
}

func newPreparedDesignInputsCache() *preparedDesignInputsCache {
	return &preparedDesignInputsCache{
		values:  make(map[string][]ConsumedInput),
		present: make(map[string]bool),
	}
}

func exactPreparedDesignInputs(
	repository *repository,
	release string,
	design ReceiptEntry,
	histories map[string]SliceHistory,
	trackHistories map[string]trackHistory,
	planByOID map[string]planEntry,
	approvals map[string]ReceiptEntry,
	releaseReceipts []ReceiptEntry,
	resolveProductBase func(sliceID, passOID string) (string, error),
	productCache map[string]string,
	cache *preparedDesignInputsCache,
) ([]ConsumedInput, error) {
	cacheKey := design.OID + ":" + design.Parent
	if cache.present[cacheKey] {
		return append([]ConsumedInput(nil), cache.values[cacheKey]...), nil
	}
	remember := func(value []ConsumedInput) []ConsumedInput {
		cache.present[cacheKey] = true
		if value == nil {
			cache.values[cacheKey] = nil
			return nil
		}
		cache.values[cacheKey] = append([]ConsumedInput{}, value...)
		return append([]ConsumedInput{}, value...)
	}
	plan, planPresent := planByOID[design.Receipt.Plan]
	location, locationPresent := locations(plan.Parsed)[design.Receipt.SliceID()]
	if !planPresent || !locationPresent {
		return nil, recordFail(
			"STALE_BINDING",
			"design "+design.OID+" has invalid reviewed-input evidence",
		)
	}
	owned := trackHistories[location.Track.ID].owned
	designIndex := -1
	for index := range owned {
		if owned[index].OID == design.OID {
			designIndex = index
			break
		}
	}
	if designIndex < 0 {
		return nil, recordFail(
			"AMBIGUOUS_AUTHORITY",
			"design "+design.OID+" has no owning track authority",
		)
	}
	approval, approvalPresent := approvals[plan.OID]
	if !approvalPresent || approval.Receipt.Target == nil {
		return nil, recordFail(
			"APPROVAL_MISSING",
			"design "+design.OID+" has no plan approval",
		)
	}
	seed := ""
	var err error
	if designIndex == 0 {
		seed, _, err = planInstallResult(
			plan.OID,
			approval,
			releaseReceipts,
		)
		if err != nil {
			return nil, err
		}
	} else {
		seed = owned[designIndex-1].OID
	}
	targetBase, err := preparePlanBoundBase(
		repository,
		release,
		plan,
		location,
		seed,
		nil,
		approvals,
		resolveProductBase,
		design.Parent,
	)
	if err != nil {
		return nil, err
	}
	if len(location.Slice.Consumes) == 0 {
		if targetBase != design.Parent {
			return nil, recordFail(
				"STALE_BINDING",
				"design "+design.OID+
					" has an inexact approved-target base",
			)
		}
		return remember([]ConsumedInput{}), nil
	}
	if design.Receipt.Base == nil {
		targetPresent, err := repository.isAncestor(
			*approval.Receipt.Target,
			design.Parent,
		)
		if err != nil {
			return nil, err
		}
		if !targetPresent {
			return nil, recordFail(
				"STALE_BINDING",
				"design "+design.OID+" omits its approved target",
			)
		}
		return remember(nil), nil
	}
	if *design.Receipt.Base != seed {
		return nil, recordFail(
			"STALE_BINDING",
			"design "+design.OID+" has the wrong prior track authority",
		)
	}
	inputs, complete, err := consumedInputsAtBase(
		repository,
		plan,
		design.Parent,
		location.Slice.Consumes,
		histories,
		planByOID,
		productCache,
	)
	if err != nil {
		return nil, err
	}
	if !complete ||
		!inputsEqual(
			design.Receipt.Inputs,
			pinsForConsumedInputs(inputs),
		) {
		return nil, recordFail(
			"STALE_BINDING",
			"design "+design.OID+" has stale reviewed-input pins",
		)
	}
	expected, err := preparePlanBoundBase(
		repository,
		release,
		plan,
		location,
		seed,
		inputs,
		approvals,
		resolveProductBase,
		design.Parent,
	)
	if err != nil {
		return nil, err
	}
	if expected != design.Parent {
		return nil, recordFail(
			"STALE_BINDING",
			"design "+design.OID+" has an inexact reviewed base",
		)
	}
	return remember(inputs), nil
}

func validateConsumedHistories(
	repository *repository,
	release string,
	histories map[string]SliceHistory,
	trackHistories map[string]trackHistory,
	planByOID map[string]planEntry,
	approvals map[string]ReceiptEntry,
	releaseReceipts []ReceiptEntry,
	resolveProductBase func(sliceID, passOID string) (string, error),
	productCache map[string]string,
) error {
	preparedDesignCache := newPreparedDesignInputsCache()
	for sliceID, history := range histories {
		byOID := make(map[string]ReceiptEntry, len(history.Entries)+len(approvals))
		for _, approval := range approvals {
			byOID[approval.OID] = approval
		}
		for _, entry := range history.Entries {
			byOID[entry.OID] = entry
		}
		for _, entry := range history.Entries {
			receipt := entry.Receipt
			plan, present := planByOID[receipt.Plan]
			if !present {
				continue
			}
			location, planned := locations(plan.Parsed)[sliceID]
			if !planned {
				continue
			}
			if receipt.Role == "implementer" && receipt.Result == "designed" {
				bound, boundPresent := byOID[receipt.Binds]
				if !boundPresent {
					continue
				}
				currentInputs, err := exactPreparedDesignInputs(
					repository,
					release,
					entry,
					histories,
					trackHistories,
					planByOID,
					approvals,
					releaseReceipts,
					resolveProductBase,
					productCache,
					preparedDesignCache,
				)
				if err != nil {
					return err
				}
				if len(location.Slice.Consumes) == 0 ||
					currentInputs == nil {
					continue
				}
				staleRetry := (bound.Receipt.Role == "implementer" &&
					(bound.Receipt.Result == "designed" ||
						bound.Receipt.Result == "candidate")) ||
					(bound.Receipt.Role == "captain" &&
						bound.Receipt.Result == "proceed") ||
					(bound.Receipt.Role == "verifier" &&
						bound.Receipt.Result == "pass")
				if staleRetry {
					priorDesign := governingDesign(history, &bound)
					if priorDesign == nil {
						return recordFail(
							"STALE_BINDING",
							"design "+entry.OID+" has no stale review chain",
						)
					}
					priorInputs, priorComplete, err := reviewedConsumedInputs(
						repository,
						*priorDesign,
						location.Slice.Consumes,
						histories,
						planByOID,
						productCache,
					)
					if err != nil {
						return err
					}
					if priorComplete &&
						inputsEqual(
							pinsForConsumedInputs(priorInputs),
							pinsForConsumedInputs(currentInputs),
						) {
						return recordFail(
							"STALE_BINDING",
							"design "+entry.OID+" retries an unchanged review",
						)
					}
				}
			}
			if len(location.Slice.Consumes) == 0 {
				continue
			}
			if receipt.Role != "implementer" ||
				receipt.Result != "candidate" {
				continue
			}
			design := governingDesign(history, &entry)
			strictDesign := design != nil && design.Receipt.Base != nil
			var reviewed []ConsumedInput
			if strictDesign {
				var err error
				reviewed, err = exactPreparedDesignInputs(
					repository,
					release,
					*design,
					histories,
					trackHistories,
					planByOID,
					approvals,
					releaseReceipts,
					resolveProductBase,
					productCache,
					preparedDesignCache,
				)
				if err != nil {
					return err
				}
			} else if design != nil {
				var complete bool
				var err error
				reviewed, complete, err = reviewedConsumedInputs(
					repository,
					*design,
					location.Slice.Consumes,
					histories,
					planByOID,
					productCache,
				)
				if err != nil {
					return err
				}
				if !complete {
					reviewed = nil
				}
			}
			if receipt.Base == nil {
				if strictDesign {
					return recordFail(
						"STALE_BINDING",
						"candidate "+entry.OID+" has no consumed-input base",
					)
				}
				// Marker-free designs and candidates remain readable legacy
				// history. Every newly appended consuming candidate has Base.
				continue
			}
			if reviewed != nil &&
				!inputsEqual(
					receipt.Inputs,
					pinsForConsumedInputs(reviewed),
				) {
				return recordFail(
					"STALE_BINDING",
					"candidate "+entry.OID+" differs from its reviewed inputs",
				)
			}
			if receipt.Base == nil || receipt.Candidate == nil {
				return recordFail(
					"CHANGED_CANDIDATE",
					"candidate "+entry.OID+" omits its prepared base",
				)
			}
			linear, err := linearOneParentAncestry(
				repository,
				*receipt.Base,
				*receipt.Candidate,
			)
			if err != nil {
				return err
			}
			if !linear {
				return recordFail(
					"CHANGED_CANDIDATE",
					"candidate "+entry.OID+" is not linear one-parent work from its base",
				)
			}
			inputs, complete, err := consumedInputsAtBase(
				repository,
				plan,
				*receipt.Base,
				location.Slice.Consumes,
				histories,
				planByOID,
				productCache,
			)
			if err != nil {
				return err
			}
			if !complete ||
				!inputsEqual(receipt.Inputs, pinsForConsumedInputs(inputs)) {
				return recordFail(
					"STALE_BINDING",
					"candidate "+entry.OID+" has stale consumed pins",
				)
			}
			expected, err := preparePlanBoundBase(
				repository,
				release,
				plan,
				location,
				receipt.Binds,
				inputs,
				approvals,
				resolveProductBase,
				*receipt.Base,
			)
			if err != nil {
				return err
			}
			if err := repository.assertCandidateRecordRootUnchanged(
				expected,
				*receipt.Candidate,
			); err != nil {
				return err
			}
			if expected != *receipt.Base {
				return recordFail(
					"CHANGED_CANDIDATE",
					"candidate "+entry.OID+" has an inexact prepared base",
				)
			}
			product, err := productTreeFor(
				repository,
				*receipt.Candidate,
				productCache,
			)
			if err != nil {
				return err
			}
			if product != *receipt.ProductTree {
				return recordFail(
					"CHANGED_CANDIDATE",
					"candidate "+entry.OID+" has invalid Git evidence",
				)
			}
			for _, input := range inputs {
				for _, ancestor := range []string{
					input.Candidate,
					input.CandidateReceipt,
					input.PassReceipt,
				} {
					contained, err := repository.isAncestor(
						ancestor,
						*receipt.Candidate,
					)
					if err != nil {
						return err
					}
					if !contained {
						return recordFail(
							"CHANGED_CANDIDATE",
							"candidate "+entry.OID+" omits consumed authority",
						)
					}
				}
			}
		}
	}
	return nil
}

func deriveSlice(
	location SliceLocation,
	history SliceHistory,
	current planEntry,
	approval ReceiptEntry,
	planByOID map[string]planEntry,
) *SliceState {
	contract := current.Parsed.Metadata().Contracts[location.Slice.ID]
	lineage := slicePlanLineage(planByOID, current, location.Slice.ID)
	var matching []ReceiptEntry
	for _, entry := range history.Entries {
		if entry.Receipt.Contract != nil && *entry.Receipt.Contract == contract &&
			lineage[entry.Receipt.Plan] {
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
	currentReceipt := latest(matching, func(ReceiptEntry) bool { return true })
	if currentReceipt != nil && currentReceipt.Receipt.Plan != current.OID {
		receipt := currentReceipt.Receipt
		nonRetainableBlocker :=
			(receipt.Role == "captain" && receipt.Result == "escalate") ||
				(receipt.Role == "verifier" && receipt.Result == "blocked")
		if nonRetainableBlocker {
			currentReceipt = nil
			passCurrent = false
		}
	}
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
	state.Retained = receipt.Plan != current.OID
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
	repository *repository,
	current planEntry,
	histories map[string]SliceHistory,
	approvals map[string]ReceiptEntry,
	planByOID map[string]planEntry,
	productCache map[string]string,
) (map[string]*SliceState, error) {
	states := make(map[string]*SliceState)
	for id, location := range locations(current.Parsed) {
		states[id] = deriveSlice(
			location, histories[id], current, approvals[current.OID], planByOID,
		)
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
		for _, dependency := range required {
			if err := resolve(dependency); err != nil {
				return err
			}
		}
		consumesReady := true
		pins := make(map[string]string, len(slice.Consumes))
		inputs := make([]ConsumedInput, 0, len(slice.Consumes))
		for _, dependency := range slice.Consumes {
			if states[dependency].Pass != nil && states[dependency].Pass.Receipt.ProductTree != nil {
				pins[dependency] = *states[dependency].Pass.Receipt.ProductTree
				input, err := consumedInputForPass(
					repository,
					dependency,
					histories[dependency],
					*states[dependency].Pass,
					productCache,
				)
				if err != nil {
					return err
				}
				inputs = append(inputs, input)
			} else {
				consumesReady = false
			}
		}
		if consumesReady {
			state.InputPins = pins
			state.ConsumedInputs = inputs
		}
		design := governingDesign(state.History, state.CurrentReceipt)
		reviewRerouted := false
		externallyBlocked := state.Status == "blocked" &&
			state.NextRole == "planner"
		if design != nil && len(slice.Consumes) > 0 {
			state.ReviewedBase = design.Parent
			reviewed, reviewedCurrent, err := reviewedConsumedInputs(
				repository,
				*design,
				slice.Consumes,
				histories,
				planByOID,
				productCache,
			)
			if err != nil {
				return err
			}
			if reviewedCurrent {
				state.ReviewedPins = pinsForConsumedInputs(reviewed)
			}
			reviewChanged := !reviewedCurrent ||
				!consumesReady ||
				!inputsEqual(state.ReviewedPins, pins)
			current := state.CurrentReceipt
			beforeCandidate := current != nil &&
				((current.Receipt.Role == "implementer" &&
					current.Receipt.Result == "designed") ||
					current.Receipt.Role == "captain")
			if !externallyBlocked && reviewChanged &&
				(beforeCandidate || reviewedCurrent) {
				state.Stage, state.Status, state.NextRole, state.Outcome =
					"design", "ready", "implementer", "stale"
				state.Attempt = state.History.MaximumAttempt + 1
				state.Pass, state.Candidate, state.Retained = nil, nil, false
				state.StaleReason = "reviewed consumed input product changed or is absent"
				reviewRerouted = true
			}
		}
		if !externallyBlocked && !reviewRerouted &&
			state.Candidate != nil && consumesReady &&
			!inputsEqual(state.Candidate.Receipt.Inputs, pins) {
			state.Stage, state.Status, state.NextRole, state.Outcome = "implement", "ready", "implementer", "stale"
			state.Attempt = state.History.MaximumAttempt + 1
			state.Pass, state.Retained = nil, false
			state.StaleReason = "consumed input lineage or product changed"
		} else if !externallyBlocked && !reviewRerouted &&
			state.Candidate != nil && !consumesReady {
			state.Stage, state.Status, state.NextRole, state.Outcome =
				"implement", "ready", "implementer", "stale"
			state.Attempt = state.History.MaximumAttempt + 1
			state.Pass, state.Retained = nil, false
			state.StaleReason = "consumed input product is absent"
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

type sliceAssemblyEvidence struct {
	Pass      *ReceiptEntry
	Candidate *ReceiptEntry
}

type assemblyTrackCandidate struct {
	ID          string
	Candidate   string
	Authority   string
	ProductTree string
}

type assemblyClassification struct {
	Direct          bool
	DirectPass      *ReceiptEntry
	Inputs          map[string]string
	TrackCandidates []assemblyTrackCandidate
}

// classifyAssembly is the shared topology predicate used by projection,
// receipt validation, preparation, and Merge. Callers must supply fresh,
// applicable evidence for every slice in the plan.
func classifyAssembly(
	repository *repository,
	plan Plan,
	topology assemblyTopology,
	evidence map[string]sliceAssemblyEvidence,
) (assemblyClassification, error) {
	metadata := plan.Metadata()
	if topology != topologyForPlan(plan) {
		return assemblyClassification{}, recordFail("INVALID_TRACK_TOPOLOGY", "release slice topology is inconsistent")
	}
	classification := assemblyClassification{
		Inputs: make(map[string]string, len(metadata.Tracks)),
	}
	for _, track := range metadata.Tracks {
		var candidates []string
		var final sliceAssemblyEvidence
		for _, plannedSlice := range track.Slices {
			item := evidence[plannedSlice.ID]
			contract := metadata.Contracts[plannedSlice.ID]
			if item.Pass == nil || item.Candidate == nil ||
				item.Pass.Receipt.Role != "verifier" ||
				item.Pass.Receipt.Result != "pass" ||
				item.Pass.Receipt.SliceID() != plannedSlice.ID ||
				item.Pass.Receipt.Contract == nil ||
				*item.Pass.Receipt.Contract != contract ||
				item.Candidate.Receipt.Role != "implementer" ||
				item.Candidate.Receipt.Result != "candidate" ||
				item.Candidate.Receipt.SliceID() != plannedSlice.ID ||
				item.Candidate.Receipt.Contract == nil ||
				*item.Candidate.Receipt.Contract != contract ||
				item.Pass.Receipt.Binds != item.Candidate.OID ||
				!sameCandidate(item.Pass.Receipt, item.Candidate.Receipt) ||
				item.Candidate.Receipt.Candidate == nil ||
				item.Candidate.Receipt.ProductTree == nil {
				return assemblyClassification{}, recordFail(
					"SLICE_PASS_REQUIRED",
					plannedSlice.ID+" has no exact applicable PASS",
				)
			}
			candidates = append(candidates, *item.Candidate.Receipt.Candidate)
			final = item
		}
		for _, candidate := range candidates {
			contained, err := repository.isAncestor(
				candidate, *final.Candidate.Receipt.Candidate,
			)
			if err != nil {
				return assemblyClassification{}, err
			}
			if !contained {
				return assemblyClassification{}, recordFail(
					"INVALID_TRACK_TOPOLOGY",
					"track "+track.ID+" candidates are not one serial lineage",
				)
			}
		}
		classification.Inputs[track.ID] = *final.Candidate.Receipt.ProductTree
		classification.TrackCandidates = append(
			classification.TrackCandidates,
			assemblyTrackCandidate{
				ID:          track.ID,
				Candidate:   *final.Candidate.Receipt.Candidate,
				Authority:   final.Pass.OID,
				ProductTree: *final.Candidate.Receipt.ProductTree,
			},
		)
	}
	return classification, nil
}

func assemblyEvidenceFromTracks(tracks []TrackState) map[string]sliceAssemblyEvidence {
	result := make(map[string]sliceAssemblyEvidence)
	for _, track := range tracks {
		for _, slice := range track.Slices {
			result[slice.Location.Slice.ID] = sliceAssemblyEvidence{
				Pass: slice.Pass, Candidate: slice.Candidate,
			}
		}
	}
	return result
}

func prepareClassifiedAssembly(
	repository *repository,
	targetRef, target, releaseHead string,
	classification assemblyClassification,
	resolvers ...func(trackID string) (string, error),
) (string, error) {
	var resolveTrackProductBase func(trackID string) (string, error)
	if len(resolvers) > 0 {
		resolveTrackProductBase = resolvers[0]
	}
	approved, err := repository.prepareApprovedTargetBase(
		targetRef,
		releaseHead,
		target,
	)
	if err != nil {
		return "", err
	}
	candidate := approved.Result
	for _, component := range classification.TrackCandidates {
		authority := component.Authority
		if authority == "" {
			authority = component.Candidate
		}
		if authority == candidate {
			continue
		}
		contained, err := repository.isAncestor(authority, candidate)
		if err != nil {
			return "", err
		}
		if contained {
			continue
		}
		var prepared preparedComposition
		if resolveTrackProductBase == nil {
			prepared, err = repository.prepareComposition(
				targetRef,
				candidate,
				authority,
			)
		} else {
			current := component
			prepared, err = repository.prepareProductComposition(
				targetRef,
				candidate,
				authority,
				func() (string, error) {
					return resolveTrackProductBase(current.ID)
				},
			)
		}
		if err != nil {
			return "", err
		}
		candidate = prepared.Result
	}
	return candidate, nil
}

func prepareConsumedTrackBase(
	repository *repository,
	consumerRef string,
	seed string,
	inputs []ConsumedInput,
	resolvers ...func(sliceID, passOID string) (string, error),
) (string, error) {
	var resolveProductBase func(sliceID, passOID string) (string, error)
	if len(resolvers) > 0 {
		resolveProductBase = resolvers[0]
	}
	candidate := seed
	for _, input := range inputs {
		if input.PassReceipt == candidate {
			continue
		}
		contained, err := repository.isAncestor(input.PassReceipt, candidate)
		if err != nil {
			return "", err
		}
		if contained {
			continue
		}
		var prepared preparedComposition
		if resolveProductBase == nil {
			prepared, err = repository.prepareComposition(
				consumerRef,
				candidate,
				input.PassReceipt,
			)
		} else {
			current := input
			prepared, err = repository.prepareProductComposition(
				consumerRef,
				candidate,
				input.PassReceipt,
				func() (string, error) {
					return resolveProductBase(
						current.Slice,
						current.PassReceipt,
					)
				},
			)
		}
		if err != nil {
			return "", err
		}
		candidate = prepared.Result
	}
	return candidate, nil
}

func preparePlanBoundBase(
	repository *repository,
	release string,
	plan planEntry,
	location SliceLocation,
	authority string,
	inputs []ConsumedInput,
	approvals map[string]ReceiptEntry,
	resolveProductBase func(sliceID, passOID string) (string, error),
	historicalResults ...string,
) (string, error) {
	if len(historicalResults) > 1 {
		return "", recordFail("INVALID_GIT_IDENTITY", "only one historical composition result is accepted")
	}
	if len(historicalResults) == 1 {
		var err error
		repository, err = repository.withHistoricalIdentity(historicalResults[0])
		if err != nil {
			return "", err
		}
	}
	approval, present := approvals[plan.OID]
	if !present || approval.Receipt.Target == nil {
		return "", recordFail(
			"APPROVAL_MISSING",
			"plan "+plan.OID+" has no approval",
		)
	}
	ref := trackRef(release, location.Track.ID)
	targetBase, err := repository.prepareApprovedTargetBase(
		ref,
		authority,
		*approval.Receipt.Target,
	)
	if err != nil {
		return "", err
	}
	return prepareConsumedTrackBase(
		repository,
		ref,
		targetBase.Result,
		inputs,
		resolveProductBase,
	)
}

func preparedStateTrackBase(
	repository *repository,
	state State,
	slice *SliceState,
) (string, error) {
	if slice == nil || state.productBases == nil ||
		state.productBases.pass == nil {
		return "", recordFail(
			"PRODUCT_BASE_RESOLVER_REQUIRED",
			"track preparation requires exact state evidence",
		)
	}
	track, present := state.Track(slice.Location.Track.ID)
	if !present {
		return "", recordFail(
			"INVALID_TRACK_TOPOLOGY",
			"track "+slice.Location.Track.ID+" is absent",
		)
	}
	approval := state.Plan.Approval
	if approval.Receipt.Target == nil {
		return "", recordFail(
			"APPROVAL_MISSING",
			"current plan approval has no target",
		)
	}
	targetBase, err := repository.prepareApprovedTargetBase(
		track.Ref,
		track.AuthorityHead,
		*approval.Receipt.Target,
	)
	if err != nil {
		return "", err
	}
	return prepareConsumedTrackBase(
		repository,
		track.Ref,
		targetBase.Result,
		slice.ConsumedInputs,
		state.productBases.pass,
	)
}

// projectedConsumedTrackBase keeps record projection read-only with respect to
// future composition. An absent consumer, or one still at its authority seed
// with an uncontained input, is merely unprepared: the action boundary owns
// merge preparation and any local product conflict. Once the consumer has
// advanced, recomputing the deterministic base remains an integrity check over
// the authority that was actually prepared.
func projectedConsumedTrackBase(
	repository *repository,
	consumerRef string,
	consumerHead string,
	authority string,
	approvedTarget string,
	inputs []ConsumedInput,
	resolveProductBase func(sliceID, passOID string) (string, error),
) (string, error) {
	if consumerHead == "" {
		return "", nil
	}
	var err error
	repository, err = repository.withHistoricalIdentity(authority)
	if err != nil {
		return "", err
	}
	targetBase, err := repository.prepareApprovedTargetBase(
		consumerRef,
		authority,
		approvedTarget,
	)
	if err != nil {
		return "", err
	}
	seed := targetBase.Result
	if consumerHead == authority && seed != authority {
		return "", nil
	}
	if consumerHead == seed {
		for _, input := range inputs {
			contained, err := repository.isAncestor(
				input.PassReceipt,
				seed,
			)
			if err != nil {
				return "", err
			}
			if !contained {
				return "", nil
			}
		}
		return seed, nil
	}
	return prepareConsumedTrackBase(
		repository,
		consumerRef,
		seed,
		inputs,
		resolveProductBase,
	)
}

func exactAssemblyComposition(
	repository *repository,
	targetRef, target, releaseHead, candidate, productTree string,
	classification assemblyClassification,
	resolvers ...func(trackID string) (string, error),
) (bool, error) {
	var err error
	repository, err = repository.withHistoricalIdentity(candidate)
	if err != nil {
		return false, err
	}
	expected, err := prepareClassifiedAssembly(
		repository, targetRef, target, releaseHead, classification,
		resolvers...,
	)
	if err != nil {
		return false, err
	}
	if expected != candidate {
		return false, nil
	}
	product, err := repository.productTree(expected)
	if err != nil {
		return false, err
	}
	return product == productTree, nil
}

func withDirectAssemblyReuse(
	repository *repository,
	_ Plan,
	topology assemblyTopology,
	evidence map[string]sliceAssemblyEvidence,
	target, _ string,
	classification assemblyClassification,
	_ ...func(trackID string) (string, error),
) (assemblyClassification, error) {
	if topology.DirectSlice == "" {
		return classification, nil
	}
	item := evidence[topology.DirectSlice]
	if item.Pass == nil || item.Candidate == nil ||
		item.Candidate.Receipt.Candidate == nil ||
		item.Candidate.Receipt.ProductTree == nil {
		return assemblyClassification{}, recordFail(
			"SLICE_PASS_REQUIRED",
			topology.DirectSlice+" has no exact applicable PASS",
		)
	}
	direct, err := repository.isAncestor(
		target,
		*item.Candidate.Receipt.Candidate,
	)
	if err != nil {
		return assemblyClassification{}, err
	}
	if direct {
		classification.Direct = true
		classification.DirectPass = item.Pass
	}
	return classification, nil
}

func directPassMatchesComposition(
	repository *repository,
	_ Plan,
	topology assemblyTopology,
	pass ReceiptEntry,
	target, _ string,
	_ ...func(trackID string) (string, error),
) (bool, error) {
	if topology.DirectSlice == "" ||
		pass.Receipt.SliceID() != topology.DirectSlice ||
		pass.Receipt.Candidate == nil ||
		pass.Receipt.ProductTree == nil {
		return false, nil
	}
	return repository.isAncestor(target, *pass.Receipt.Candidate)
}

func validateAssemblyHistory(
	repository *repository,
	entries []ReceiptEntry,
	planByOID map[string]planEntry,
	approvals map[string]ReceiptEntry,
	topologies map[string]assemblyTopology,
	sliceEntries []ReceiptEntry,
	productCache map[string]string,
	current planEntry,
	tracks []TrackState,
	releaseEntries []ReceiptEntry,
	resolveTrackProductBase func(trackID string) (string, error),
) (receiptHistory, error) {
	byOID := make(map[string]ReceiptEntry)
	for _, entry := range releaseEntries {
		byOID[entry.OID] = entry
	}
	for _, entry := range approvals {
		byOID[entry.OID] = entry
	}
	for _, entry := range sliceEntries {
		byOID[entry.OID] = entry
	}
	var currentClassification *assemblyClassification
	currentEvidence := assemblyEvidenceFromTracks(tracks)
	allCurrentPassed := true
	for _, track := range tracks {
		for _, slice := range track.Slices {
			allCurrentPassed = allCurrentPassed && slice.Pass != nil
		}
	}
	if allCurrentPassed {
		classification, err := classifyAssembly(
			repository, current.Parsed, topologies[current.OID],
			currentEvidence,
		)
		if err != nil {
			return receiptHistory{}, err
		}
		currentClassification = &classification
	}
	releasePredecessor := make(map[string]string)
	lastReleaseReceipt := ""
	for _, entry := range releaseEntries {
		if entry.Receipt.Role == "planner" || entry.Receipt.Slice == nil {
			releasePredecessor[entry.OID] = lastReleaseReceipt
			lastReleaseReceipt = entry.OID
		}
	}
	for index := range entries {
		entry := entries[index]
		receipt := entry.Receipt
		plan, ok := planByOID[receipt.Plan]
		if !ok {
			return receiptHistory{}, recordFail("STALE_BINDING", "assembly receipt "+entry.OID+" has an unknown plan")
		}
		approval := approvals[receipt.Plan]
		topology := topologies[receipt.Plan]
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
			if !exists || bound.OID != releasePredecessor[entry.OID] ||
				receipt.Candidate == nil || receipt.Base == nil ||
				receipt.Target == nil || receipt.ProductTree == nil ||
				entry.Parent != *receipt.Candidate ||
				*receipt.Base != *receipt.Target {
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
			if approval.Receipt.Target == nil {
				return receiptHistory{}, recordFail("STALE_BINDING", "assembly candidate "+entry.OID+" has invalid evidence")
			}
			approvedAncestor, err := repository.isAncestor(
				*approval.Receipt.Target,
				*receipt.Target,
			)
			if err != nil {
				return receiptHistory{}, err
			}
			if !baseAncestor || !bindAncestor || !approvedAncestor ||
				product != *receipt.ProductTree {
				return receiptHistory{}, recordFail("STALE_BINDING", "assembly candidate "+entry.OID+" has invalid evidence")
			}
			if receipt.Plan == current.OID && currentClassification != nil &&
				inputsEqual(receipt.Inputs, currentClassification.Inputs) {
				historicalRepository, err := repository.withHistoricalIdentity(*receipt.Candidate)
				if err != nil {
					return receiptHistory{}, err
				}
				expected, err := prepareClassifiedAssembly(
					historicalRepository, plan.Parsed.Metadata().TargetRef,
					*receipt.Target, releasePredecessor[entry.OID],
					*currentClassification,
					resolveTrackProductBase,
				)
				if err != nil {
					return receiptHistory{}, err
				}
				if expected != *receipt.Candidate {
					return receiptHistory{}, recordFail(
						"INVALID_TRACK_TOPOLOGY",
						"assembly candidate "+entry.OID+" does not exactly compose the applicable topology",
					)
				}
			}
		case "verifier":
			bound, exists := byOID[receipt.Binds]
			if !exists || bound.Receipt.Role != "implementer" || bound.Receipt.Slice != nil ||
				bound.Receipt.Plan != receipt.Plan ||
				!sameCandidate(receipt, bound.Receipt) {
				return receiptHistory{}, recordFail("STALE_BINDING", "assembly Verifier "+entry.OID+" has no exact candidate")
			}
		case "merge":
			bound, exists := byOID[receipt.Binds]
			assemblyPass := exists && bound.Receipt.Role == "verifier" &&
				bound.Receipt.Result == "pass" && bound.Receipt.Slice == nil &&
				bound.Receipt.Plan == receipt.Plan
			directPass := topology.DirectSlice != "" && exists &&
				bound.Receipt.Role == "verifier" &&
				bound.Receipt.Result == "pass" &&
				slicePlanLineage(
					planByOID, plan, topology.DirectSlice,
				)[bound.Receipt.Plan] &&
				bound.Receipt.SliceID() == topology.DirectSlice
			if directPass && receipt.Target != nil {
				exact, err := directPassMatchesComposition(
					repository, plan.Parsed, topology, bound,
					*receipt.Target, entry.Parent,
					resolveTrackProductBase,
				)
				if err != nil {
					return receiptHistory{}, err
				}
				directPass = exact
			}
			candidateEntry := &bound
			if assemblyPass {
				candidateEntry = assemblyCandidate(byOID, &bound)
			}
			if receipt.Target == nil || approval.Receipt.Target == nil {
				return receiptHistory{}, recordFail("STALE_BINDING", "Merge "+entry.OID+" has no applicable PASS")
			}
			approvedAncestor, err := repository.isAncestor(
				*approval.Receipt.Target,
				*receipt.Target,
			)
			if err != nil {
				return receiptHistory{}, err
			}
			if (!assemblyPass && !directPass) || !approvedAncestor ||
				receipt.Candidate == nil || candidateEntry == nil ||
				candidateEntry.Receipt.Candidate == nil ||
				*receipt.Candidate != *candidateEntry.Receipt.Candidate ||
				receipt.ProductTree == nil || candidateEntry.Receipt.ProductTree == nil ||
				*receipt.ProductTree != *candidateEntry.Receipt.ProductTree ||
				receipt.ResultCommit == nil {
				return receiptHistory{}, recordFail("STALE_BINDING", "Merge "+entry.OID+" has no applicable PASS")
			}
			if receipt.Plan == current.OID {
				if currentClassification == nil {
					return receiptHistory{}, recordFail(
						"SLICE_PASS_REQUIRED",
						"Merge "+entry.OID+" has no current slice topology",
					)
				}
				switch {
				case directPass:
					direct := currentEvidence[topology.DirectSlice]
					if direct.Pass == nil || direct.Pass.OID != bound.OID {
						return receiptHistory{}, recordFail(
							"INVALID_TRACK_TOPOLOGY",
							"Merge "+entry.OID+" bypasses required assembly verification",
						)
					}
				case assemblyPass:
					if candidateEntry == nil ||
						!inputsEqual(
							candidateEntry.Receipt.Inputs,
							currentClassification.Inputs,
						) {
						return receiptHistory{}, recordFail(
							"INVALID_TRACK_TOPOLOGY",
							"Merge "+entry.OID+" binds stale assembly inputs",
						)
					}
					historicalRepository, err := repository.withHistoricalIdentity(*candidateEntry.Receipt.Candidate)
					if err != nil {
						return receiptHistory{}, err
					}
					expected, err := prepareClassifiedAssembly(
						historicalRepository, plan.Parsed.Metadata().TargetRef,
						*receipt.Target,
						releasePredecessor[candidateEntry.OID],
						*currentClassification,
						resolveTrackProductBase,
					)
					if err != nil {
						return receiptHistory{}, err
					}
					if candidateEntry.Receipt.Candidate == nil ||
						expected != *candidateEntry.Receipt.Candidate {
						return receiptHistory{}, recordFail(
							"INVALID_TRACK_TOPOLOGY",
							"Merge "+entry.OID+" binds an inexact assembly candidate",
						)
					}
				}
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
	}
	return receiptHistory{Receipts: entries, ByOID: byOID}, nil
}

func deriveAssembly(
	repository *repository,
	current planEntry,
	history receiptHistory,
	approval ReceiptEntry,
	tracks []TrackState,
	topology assemblyTopology,
	target, releaseHead string,
	resolveTrackProductBase func(trackID string) (string, error),
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
	classification, err := classifyAssembly(
		repository, current.Parsed, topology,
		assemblyEvidenceFromTracks(tracks),
	)
	if err != nil {
		return AssemblyState{}, err
	}
	directReleaseHead := releaseHead
	if latestEntry != nil && latestEntry.Receipt.Role == "merge" {
		if bound, present := history.ByOID[latestEntry.Receipt.Binds]; present &&
			bound.Receipt.Slice != nil {
			directReleaseHead = latestEntry.Parent
		}
	}
	classification, err = withDirectAssemblyReuse(
		repository, current.Parsed, topology,
		assemblyEvidenceFromTracks(tracks),
		target, directReleaseHead, classification,
		resolveTrackProductBase,
	)
	if err != nil {
		return AssemblyState{}, err
	}
	pins = make(map[string]*string, len(classification.Inputs))
	for track, product := range classification.Inputs {
		value := product
		pins[track] = &value
	}
	common.InputPins = pins
	if latestEntry == nil {
		if classification.Direct {
			common.Stage, common.Status, common.NextRole, common.Outcome = "merge", "ready", "merge", "pass"
			common.CurrentReceipt, common.Pass =
				classification.DirectPass, classification.DirectPass
			common.Candidate = findEntry(
				historyEntriesForTracks(tracks),
				classification.DirectPass.Receipt.Binds,
			)
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

func historyEntriesForTracks(tracks []TrackState) []ReceiptEntry {
	var result []ReceiptEntry
	for _, track := range tracks {
		for _, slice := range track.Slices {
			result = append(result, slice.History.Entries...)
		}
	}
	return result
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

func (s State) HistoryForSlice(id string) (*SliceHistoryState, bool) {
	for index := range s.SliceHistories {
		if s.SliceHistories[index].Slice == id {
			return &s.SliceHistories[index], true
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
