package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
)

const (
	productionWorkContextVersionV1 = "sworn.work-context/v1"
	productionWorkContextVersion   = "sworn.work-context/v2"
	productionDispatchVersionV1    = "sworn.production-dispatch/v1"
	productionDispatchVersion      = "sworn.production-dispatch/v2"
	productionWorkContextPath      = "work-context.json"
	productionPlanPath             = "baton/plan.md"
	productionReceiptPath          = "baton/current-receipt.json"
	productionReceiptDetailPath    = "baton/current-receipt-detail.md"
	productionDesignReceiptPath    = "baton/design-receipt.json"
	productionDesignDetailPath     = "baton/design-receipt-detail.md"
)

var productionOutputExpectation = sha256Digest(
	[]byte("sworn.dynamic-driver-output/v1\n"),
)

type dispatchCoordinates struct {
	Slice          string
	Responsibility driver.Responsibility
	BatonAttempt   int64
	Epoch          int64
	Try            int64
}

type productionAuthorityBinding struct {
	ReleaseRef  string `json:"release_ref"`
	ReleaseHead string `json:"release_head,omitempty"`
	TargetRef   string `json:"target_ref"`
	TargetHead  string `json:"target_head"`
	TrackRef    string `json:"track_ref,omitempty"`
	TrackHead   string `json:"track_head,omitempty"`
}

type productionPlanBinding struct {
	OID      string       `json:"oid"`
	Digest   string       `json:"digest"`
	Revision int64        `json:"revision"`
	Input    driver.Input `json:"input"`
	body     []byte
}

type productionReceiptBinding struct {
	OID         string       `json:"oid"`
	BodyInput   driver.Input `json:"body_input"`
	DetailInput driver.Input `json:"detail_input"`
	body        []byte
	detail      []byte
}

type productionCandidateBinding struct {
	Receipt     string `json:"receipt"`
	Commit      string `json:"commit"`
	ProductTree string `json:"product_tree,omitempty"`
}

type productionEvidenceBinding struct {
	Slice            string `json:"slice"`
	PassReceipt      string `json:"pass_receipt"`
	CandidateReceipt string `json:"candidate_receipt"`
	Candidate        string `json:"candidate"`
	ProductTree      string `json:"product_tree"`
	SourceRef        string `json:"source_ref"`
	SourceHead       string `json:"source_head"`
}

type productionWorkContext struct {
	SchemaVersion      string                      `json:"schema_version"`
	ManifestDigest     string                      `json:"manifest_digest"`
	DriverConfigDigest string                      `json:"driver_config_digest"`
	RunID              string                      `json:"run_id"`
	Repository         string                      `json:"repository"`
	Release            string                      `json:"release"`
	Intent             string                      `json:"intent"`
	InvocationID       string                      `json:"invocation_id"`
	Role               driver.Role                 `json:"role"`
	Track              string                      `json:"track,omitempty"`
	Slice              string                      `json:"slice,omitempty"`
	Responsibility     driver.Responsibility       `json:"responsibility"`
	Attempt            int64                       `json:"attempt"`
	Epoch              int64                       `json:"epoch"`
	Try                int64                       `json:"try"`
	Before             string                      `json:"before"`
	WorkspaceAccess    driver.WorkspaceAccess      `json:"workspace_access"`
	Authority          productionAuthorityBinding  `json:"authority"`
	PreparedBase       string                      `json:"prepared_base,omitempty"`
	Plan               *productionPlanBinding      `json:"plan,omitempty"`
	Receipt            *productionReceiptBinding   `json:"receipt,omitempty"`
	DesignReceipt      *productionReceiptBinding   `json:"design_receipt,omitempty"`
	Candidate          *productionCandidateBinding `json:"candidate,omitempty"`
	Evidence           []productionEvidenceBinding `json:"evidence"`
}

type productionDispatchCommand struct {
	SchemaVersion       string                `json:"schema_version"`
	RequestDigest       string                `json:"request_digest"`
	ResumeRequestDigest string                `json:"resume_request_digest,omitempty"`
	Context             productionWorkContext `json:"context"`
}

func hasContinuationResumeRequest(work productionWorkContext) bool {
	return work.Responsibility == driver.WorkVerification ||
		(work.Responsibility == driver.ImplementerImplementation &&
			work.DesignReceipt != nil)
}

func roleForResponsibility(
	responsibility driver.Responsibility,
) (driver.Role, bool) {
	switch responsibility {
	case driver.PlannerProposal:
		return driver.RolePlanner, true
	case driver.ImplementerDesign, driver.ImplementerImplementation:
		return driver.RoleImplementer, true
	case driver.CaptainReview:
		return driver.RoleCaptain, true
	case driver.WorkVerification, driver.AssemblyVerification:
		return driver.RoleVerifier, true
	default:
		return "", false
	}
}

func dispatchInvocationID(
	runID string,
	coordinates dispatchCoordinates,
) string {
	work := coordinates.Slice
	if work == "" {
		work = "release"
	}
	return invocationIdentity(
		runID,
		work,
		coordinates.Responsibility,
		coordinates.BatonAttempt,
		coordinates.Epoch,
		coordinates.Try,
	)
}

func invocationIdentity(
	runID string,
	work string,
	responsibility driver.Responsibility,
	attempt int64,
	epoch int64,
	try int64,
) string {
	return runID + "/" + work + "/" + string(responsibility) + "/" +
		strconv.FormatInt(attempt, 10) + "/" +
		strconv.FormatInt(epoch, 10) + "/" +
		strconv.FormatInt(try, 10)
}

func currentPlanBinding(state baton.State) (*productionPlanBinding, error) {
	for _, history := range state.Plan.History {
		if history.OID != state.Plan.OID {
			continue
		}
		body := history.Plan.Bytes()
		if history.Plan.Digest() != state.Plan.Digest ||
			history.Revision != state.Plan.Metadata.Revision {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		return &productionPlanBinding{
			OID:      history.OID,
			Digest:   history.Plan.Digest(),
			Revision: history.Revision,
			Input: driver.Input{
				Name:   "plan",
				Path:   productionPlanPath,
				Digest: driver.Digest(body),
			},
			body: body,
		}, nil
	}
	return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
}

func receiptBinding(
	entry *baton.ReceiptEntry,
) (*productionReceiptBinding, error) {
	return namedReceiptBinding(
		entry,
		"receipt",
		productionReceiptPath,
		"receipt-detail",
		productionReceiptDetailPath,
	)
}

func designReceiptBinding(
	entry *baton.ReceiptEntry,
) (*productionReceiptBinding, error) {
	return namedReceiptBinding(
		entry,
		"design-receipt",
		productionDesignReceiptPath,
		"design-receipt-detail",
		productionDesignDetailPath,
	)
}

func namedReceiptBinding(
	entry *baton.ReceiptEntry,
	bodyName string,
	bodyPath string,
	detailName string,
	detailPath string,
) (*productionReceiptBinding, error) {
	if entry == nil {
		return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	body, err := entry.Receipt.CanonicalBytes()
	if err != nil || entry.Receipt.Detail != baton.DigestBytes(entry.Detail) {
		return nil, runtimeFail("INVALID_AUTHORITY_STATE", err)
	}
	return &productionReceiptBinding{
		OID: entry.OID,
		BodyInput: driver.Input{
			Name:   bodyName,
			Path:   bodyPath,
			Digest: driver.Digest(body),
		},
		DetailInput: driver.Input{
			Name:   detailName,
			Path:   detailPath,
			Digest: driver.Digest(entry.Detail),
		},
		body:   body,
		detail: append([]byte(nil), entry.Detail...),
	}, nil
}

func candidateBinding(
	entry *baton.ReceiptEntry,
) (*productionCandidateBinding, error) {
	if entry == nil || entry.Receipt.Candidate == nil {
		return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	productTree := ""
	if entry.Receipt.ProductTree != nil {
		productTree = *entry.Receipt.ProductTree
	}
	return &productionCandidateBinding{
		Receipt:     entry.OID,
		Commit:      *entry.Receipt.Candidate,
		ProductTree: productTree,
	}, nil
}

func sliceEvidence(
	inputs []baton.ConsumedInput,
) []productionEvidenceBinding {
	result := make([]productionEvidenceBinding, len(inputs))
	for index, input := range inputs {
		result[index] = productionEvidenceBinding{
			Slice:            input.Slice,
			PassReceipt:      input.PassReceipt,
			CandidateReceipt: input.CandidateReceipt,
			Candidate:        input.Candidate,
			ProductTree:      input.ProductTree,
			SourceRef:        input.SourceRef,
			SourceHead:       input.SourceHead,
		}
	}
	return result
}

func assemblyEvidence(
	state baton.State,
) ([]productionEvidenceBinding, error) {
	if len(state.Tracks) == 0 ||
		len(state.Assembly.InputPins) != len(state.Tracks) {
		return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	result := make([]productionEvidenceBinding, 0, len(state.Assembly.InputPins))
	seenTracks := make(map[string]struct{}, len(state.Tracks))
	seenSlices := make(map[string]struct{}, len(state.Slices))
	for index := range state.Tracks {
		track := &state.Tracks[index]
		productTree, pinned := state.Assembly.InputPins[track.ID]
		if track.ID == "" || !pinned || productTree == nil ||
			len(track.Slices) == 0 ||
			track.Ref != "refs/heads/track/"+state.Release+"/"+track.ID ||
			!validGitObjectID(track.Head) ||
			track.Head != track.AuthorityHead {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		if _, duplicate := seenTracks[track.ID]; duplicate {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		seenTracks[track.ID] = struct{}{}
		for _, slice := range track.Slices {
			if !exactPassedSlice(slice, track.ID) {
				return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
			}
			sliceID := slice.Location.Slice.ID
			if _, duplicate := seenSlices[sliceID]; duplicate {
				return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
			}
			seenSlices[sliceID] = struct{}{}
		}
		slice := track.Slices[len(track.Slices)-1]
		if *slice.Candidate.Receipt.ProductTree != *productTree {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		result = append(result, productionEvidenceBinding{
			Slice:            slice.Location.Slice.ID,
			PassReceipt:      slice.Pass.OID,
			CandidateReceipt: slice.Candidate.OID,
			Candidate:        *slice.Candidate.Receipt.Candidate,
			ProductTree:      *slice.Candidate.Receipt.ProductTree,
			SourceRef:        track.Ref,
			SourceHead:       track.Head,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Slice < result[j].Slice
	})
	return result, nil
}

func exactPassedSlice(slice *baton.SliceState, trackID string) bool {
	if slice == nil || slice.Location.Track.ID != trackID ||
		slice.Location.Slice.ID == "" ||
		slice.Candidate == nil || slice.Candidate.OID == "" ||
		slice.Candidate.Receipt.Role != "implementer" ||
		slice.Candidate.Receipt.Result != "candidate" ||
		slice.Candidate.Receipt.SliceID() != slice.Location.Slice.ID ||
		slice.Candidate.Receipt.Candidate == nil ||
		slice.Candidate.Receipt.ProductTree == nil ||
		!runtimeDigestPattern.MatchString(
			*slice.Candidate.Receipt.ProductTree,
		) ||
		slice.Pass == nil || slice.Pass.OID == "" ||
		slice.Pass.Receipt.Role != "verifier" ||
		slice.Pass.Receipt.Result != "pass" ||
		slice.Pass.Receipt.SliceID() != slice.Location.Slice.ID ||
		slice.Pass.Receipt.Binds != slice.Candidate.OID ||
		slice.Pass.Receipt.Candidate == nil ||
		*slice.Pass.Receipt.Candidate !=
			*slice.Candidate.Receipt.Candidate ||
		slice.Pass.Receipt.ProductTree == nil ||
		*slice.Pass.Receipt.ProductTree !=
			*slice.Candidate.Receipt.ProductTree {
		return false
	}
	return true
}

func validGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' ||
			character > '9' && character < 'a' ||
			character > 'f' {
			return false
		}
	}
	return true
}

func captureProductionWorkContext(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	access driver.WorkspaceAccess,
) (productionWorkContext, []byte, error) {
	role, ok := roleForResponsibility(coordinates.Responsibility)
	if engine == nil || !ok || !engine.manifest.value.production() ||
		coordinates.BatonAttempt < 1 || coordinates.Epoch < 1 ||
		coordinates.Try < 1 || coordinates.Try > 3 ||
		!runtimeDigestPattern.MatchString(before) {
		return productionWorkContext{}, nil,
			runtimeFail("INVALID_PRODUCTION_DISPATCH", nil)
	}
	workContext := productionWorkContext{
		SchemaVersion:      productionWorkContextVersion,
		ManifestDigest:     engine.manifest.digest,
		DriverConfigDigest: engine.manifest.value.DriverConfigDigest,
		RunID:              engine.manifest.value.RunID,
		Repository:         engine.manifest.value.Authority.Project,
		Release:            engine.manifest.value.Release,
		Intent:             engine.manifest.value.Intent,
		InvocationID:       dispatchInvocationID(engine.manifest.value.RunID, coordinates),
		Role:               role,
		Track:              "",
		Slice:              coordinates.Slice,
		Responsibility:     coordinates.Responsibility,
		Attempt:            coordinates.BatonAttempt,
		Epoch:              coordinates.Epoch,
		Try:                coordinates.Try,
		Before:             before,
		WorkspaceAccess:    access,
		Evidence:           make([]productionEvidenceBinding, 0),
	}
	if coordinates.Responsibility == driver.PlannerProposal {
		if coordinates.Slice != "" {
			return productionWorkContext{}, nil,
				runtimeFail("INVALID_PRODUCTION_DISPATCH", nil)
		}
		if err := capturePlannerWorkContext(engine, coordinates, before, &workContext); err != nil {
			return productionWorkContext{}, nil, err
		}
	} else {
		if err := captureBatonWorkContext(
			ctx,
			engine,
			coordinates,
			before,
			&workContext,
		); err != nil {
			return productionWorkContext{}, nil, err
		}
	}
	body := mustJSON(workContext)
	if len(body) > driver.MaxInputFileBytes {
		return productionWorkContext{}, nil,
			runtimeFail("WORK_CONTEXT_TOO_LARGE", nil)
	}
	if err := validateProductionWorkContext(engine.manifest, workContext); err != nil {
		return productionWorkContext{}, nil, err
	}
	return workContext, body, nil
}

func capturePlannerWorkContext(
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	workContext *productionWorkContext,
) error {
	release, target, err := captureProposalRefs(
		engine.repository,
		engine.manifest,
	)
	if err != nil {
		return err
	}
	authority := planProposalAuthority{
		Release:    engine.manifest.value.Release,
		ReleaseRef: release.Ref,
		TargetRef:  target.Ref,
		TargetHead: target.Head.String(),
	}
	if release.Head.String() == "" {
		if coordinates.BatonAttempt != 1 {
			return runtimeFail("STALE_DISPATCH", nil)
		}
	} else {
		state, readErr := baton.ReadState(
			engine.git,
			engine.manifest.value.Release,
			engine.inertness,
		)
		if readErr != nil ||
			coordinates.BatonAttempt != state.Plan.Metadata.Revision+1 ||
			state.Refs.Release.Head != release.Head.String() ||
			state.Refs.Target.Head != target.Head.String() {
			return runtimeFail("STALE_DISPATCH", readErr)
		}
		authority.PriorPlan = state.Plan.OID
		authority.ReleaseHead = release.Head.String()
		workContext.Plan, err = currentPlanBinding(state)
		if err != nil {
			return err
		}
		workContext.Receipt, err = receiptBinding(&state.Plan.Approval)
		if err != nil {
			return err
		}
	}
	authority.Before = plannerAuthorityBefore(authority)
	if authority.Before != before {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	workContext.Authority = productionAuthorityBinding{
		ReleaseRef:  authority.ReleaseRef,
		ReleaseHead: authority.ReleaseHead,
		TargetRef:   authority.TargetRef,
		TargetHead:  authority.TargetHead,
	}
	return nil
}

func captureBatonWorkContext(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	workContext *productionWorkContext,
) error {
	state, err := baton.ReadState(
		engine.git,
		engine.manifest.value.Release,
		engine.inertness,
	)
	if err != nil {
		return runtimeFail("BATON_UNAVAILABLE", err)
	}
	if !dispatchAuthorityCurrent(
		state,
		coordinates.Slice,
		coordinates.Responsibility,
		before,
	) {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	workContext.Plan, err = currentPlanBinding(state)
	if err != nil {
		return err
	}
	workContext.Authority = productionAuthorityBinding{
		ReleaseRef:  state.Refs.Release.Ref,
		ReleaseHead: state.Refs.Release.Head,
		TargetRef:   state.Refs.Target.Ref,
		TargetHead:  state.Refs.Target.Head,
	}
	if coordinates.Responsibility == driver.AssemblyVerification {
		if coordinates.Slice != "" ||
			coordinates.BatonAttempt != state.Plan.Metadata.Revision ||
			state.Assembly.NextRole != "verifier" ||
			state.Assembly.Candidate == nil {
			return runtimeFail("STALE_DISPATCH", nil)
		}
		workContext.Receipt, err = receiptBinding(state.Assembly.CurrentReceipt)
		if err != nil {
			return err
		}
		workContext.Candidate, err = candidateBinding(state.Assembly.Candidate)
		if err != nil {
			return err
		}
		workContext.Evidence, err = assemblyEvidence(state)
		return err
	}
	slice, ok := state.Slice(coordinates.Slice)
	if !ok || slice.Attempt != coordinates.BatonAttempt ||
		slice.CurrentReceipt == nil {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	switch coordinates.Responsibility {
	case driver.ImplementerDesign:
		ok = slice.Stage == "design" && slice.NextRole == "implementer"
	case driver.CaptainReview:
		ok = slice.NextRole == "captain"
	case driver.ImplementerImplementation:
		ok = slice.Stage == "implement" && slice.NextRole == "implementer"
	case driver.WorkVerification:
		ok = slice.NextRole == "verifier" && slice.Candidate != nil
	default:
		ok = false
	}
	if !ok {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	track, ok := state.Track(slice.Location.Track.ID)
	if !ok {
		return runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	workContext.Authority.TrackRef = track.Ref
	workContext.Authority.TrackHead = track.Head
	workContext.Track = slice.Location.Track.ID
	workContext.PreparedBase = slice.PreparedBase
	workContext.Receipt, err = receiptBinding(slice.CurrentReceipt)
	if err != nil {
		return err
	}
	if coordinates.Responsibility == driver.ImplementerImplementation {
		design, designErr := currentImplementationDesignReceipt(
			state,
			slice,
			track,
		)
		if designErr != nil {
			return designErr
		}
		if design != nil {
			workContext.DesignReceipt, err =
				designReceiptBinding(design)
			if err != nil {
				return err
			}
		}
	}
	workContext.Evidence = sliceEvidence(slice.ConsumedInputs)
	if coordinates.Responsibility == driver.WorkVerification {
		workContext.Candidate, err = candidateBinding(slice.Candidate)
		if err != nil {
			return err
		}
	}
	return nil
}

func currentImplementationDesignReceipt(
	state baton.State,
	slice *baton.SliceState,
	track *baton.TrackState,
) (*baton.ReceiptEntry, error) {
	if slice == nil || track == nil || slice.CurrentReceipt == nil ||
		slice.CurrentReceipt.Receipt.SliceID() != slice.Location.Slice.ID {
		return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	if slice.CurrentReceipt.Receipt.Role != "captain" ||
		slice.CurrentReceipt.Receipt.Result != "proceed" {
		return nil, nil
	}
	if track.Head != slice.CurrentReceipt.OID ||
		slice.CurrentReceipt.Receipt.Attempt == nil ||
		*slice.CurrentReceipt.Receipt.Attempt != slice.Attempt ||
		slice.CurrentReceipt.Receipt.Plan != state.Plan.OID ||
		slice.CurrentReceipt.Parent == "" {
		return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	designOID := slice.CurrentReceipt.Receipt.Binds
	var design *baton.ReceiptEntry
	for index := range slice.History.Entries {
		entry := &slice.History.Entries[index]
		if entry.OID != designOID {
			continue
		}
		if design != nil {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		design = entry
	}
	if design == nil ||
		slice.CurrentReceipt.Parent != design.OID ||
		design.Receipt.Role != "implementer" ||
		design.Receipt.Result != "designed" ||
		design.Receipt.Attempt == nil ||
		*design.Receipt.Attempt != slice.Attempt ||
		design.Receipt.Plan != state.Plan.OID ||
		design.Receipt.SliceID() != slice.Location.Slice.ID {
		return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	return design, nil
}

func validateProductionWorkContext(
	manifest admittedManifest,
	workContext productionWorkContext,
) error {
	role, ok := roleForResponsibility(workContext.Responsibility)
	if !ok ||
		(workContext.SchemaVersion != productionWorkContextVersion &&
			workContext.SchemaVersion != productionWorkContextVersionV1) ||
		workContext.ManifestDigest != manifest.digest ||
		workContext.DriverConfigDigest != manifest.value.DriverConfigDigest ||
		workContext.RunID != manifest.value.RunID ||
		workContext.Repository != manifest.value.Authority.Project ||
		workContext.Release != manifest.value.Release ||
		workContext.Intent != manifest.value.Intent ||
		workContext.Role != role ||
		workContext.InvocationID != dispatchInvocationID(
			workContext.RunID,
			dispatchCoordinates{
				Slice:          workContext.Slice,
				Responsibility: workContext.Responsibility,
				BatonAttempt:   workContext.Attempt,
				Epoch:          workContext.Epoch,
				Try:            workContext.Try,
			},
		) ||
		workContext.Attempt < 1 || workContext.Epoch < 1 ||
		workContext.Try < 1 || workContext.Try > 3 ||
		!runtimeDigestPattern.MatchString(workContext.Before) ||
		(workContext.WorkspaceAccess != driver.ReadOnly &&
			workContext.WorkspaceAccess != driver.ReadWrite) ||
		workContext.Authority.ReleaseRef !=
			"refs/heads/release-wt/"+manifest.value.Release ||
		workContext.Authority.TargetRef != manifest.value.TargetRef ||
		workContext.Authority.TargetHead == "" {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if workContext.SchemaVersion == productionWorkContextVersionV1 &&
		(workContext.Track != "" ||
			workContext.PreparedBase != "" ||
			workContext.DesignReceipt != nil) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	expectedAccess := driver.ReadOnly
	if workContext.Responsibility == driver.ImplementerImplementation {
		expectedAccess = driver.ReadWrite
	}
	if workContext.WorkspaceAccess != expectedAccess {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if workContext.Plan != nil {
		if workContext.Plan.OID == "" ||
			!runtimeDigestPattern.MatchString(workContext.Plan.Digest) ||
			workContext.Plan.Revision < 1 ||
			workContext.Plan.Input.Name != "plan" ||
			workContext.Plan.Input.Path != productionPlanPath ||
			workContext.Plan.Input.Digest != workContext.Plan.Digest {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	if workContext.Receipt != nil {
		if !validProductionReceiptBinding(
			workContext.Receipt,
			"receipt",
			productionReceiptPath,
			"receipt-detail",
			productionReceiptDetailPath,
		) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	if workContext.DesignReceipt != nil &&
		!validProductionReceiptBinding(
			workContext.DesignReceipt,
			"design-receipt",
			productionDesignReceiptPath,
			"design-receipt-detail",
			productionDesignDetailPath,
		) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	seenEvidence := make(map[string]struct{}, len(workContext.Evidence))
	for _, evidence := range workContext.Evidence {
		if evidence.Slice == "" || evidence.PassReceipt == "" ||
			evidence.CandidateReceipt == "" || evidence.Candidate == "" ||
			!runtimeDigestPattern.MatchString(evidence.ProductTree) ||
			evidence.SourceRef == "" || evidence.SourceHead == "" {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if _, duplicate := seenEvidence[evidence.Slice]; duplicate {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		seenEvidence[evidence.Slice] = struct{}{}
	}
	if workContext.Candidate != nil &&
		(workContext.Candidate.Receipt == "" ||
			workContext.Candidate.Commit == "" ||
			(workContext.Candidate.ProductTree != "" &&
				!runtimeDigestPattern.MatchString(
					workContext.Candidate.ProductTree,
				))) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if workContext.Responsibility == driver.PlannerProposal {
		if workContext.Track != "" ||
			workContext.Slice != "" || workContext.Candidate != nil ||
			workContext.PreparedBase != "" ||
			workContext.DesignReceipt != nil ||
			len(workContext.Evidence) != 0 {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		priorPlan := ""
		if workContext.Authority.ReleaseHead == "" {
			if workContext.Attempt != 1 ||
				workContext.Plan != nil ||
				workContext.Receipt != nil {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
		} else {
			if workContext.Plan == nil ||
				workContext.Receipt == nil ||
				workContext.Attempt != workContext.Plan.Revision+1 {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			priorPlan = workContext.Plan.OID
		}
		authority := planProposalAuthority{
			Release:     workContext.Release,
			PriorPlan:   priorPlan,
			ReleaseRef:  workContext.Authority.ReleaseRef,
			ReleaseHead: workContext.Authority.ReleaseHead,
			TargetRef:   workContext.Authority.TargetRef,
			TargetHead:  workContext.Authority.TargetHead,
		}
		if plannerAuthorityBefore(authority) != workContext.Before {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	} else if workContext.Plan == nil || workContext.Receipt == nil {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	switch workContext.Responsibility {
	case driver.ImplementerDesign,
		driver.CaptainReview:
		if (workContext.SchemaVersion == productionWorkContextVersion &&
			(workContext.Track == "" ||
				workContext.Authority.TrackRef !=
					"refs/heads/track/"+workContext.Release+"/"+
						workContext.Track)) ||
			workContext.Slice == "" ||
			workContext.Authority.TrackRef == "" ||
			workContext.Authority.TrackHead == "" ||
			workContext.DesignReceipt != nil ||
			workContext.Candidate != nil {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	case driver.ImplementerImplementation:
		if (workContext.SchemaVersion == productionWorkContextVersion &&
			(workContext.Track == "" ||
				workContext.Authority.TrackRef !=
					"refs/heads/track/"+workContext.Release+"/"+
						workContext.Track)) ||
			workContext.Slice == "" ||
			workContext.Authority.TrackRef == "" ||
			workContext.Authority.TrackHead == "" ||
			workContext.Candidate != nil {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	case driver.WorkVerification:
		if (workContext.SchemaVersion == productionWorkContextVersion &&
			(workContext.Track == "" ||
				workContext.Authority.TrackRef !=
					"refs/heads/track/"+workContext.Release+"/"+
						workContext.Track)) ||
			workContext.Slice == "" ||
			workContext.Authority.TrackRef == "" ||
			workContext.Authority.TrackHead == "" ||
			workContext.DesignReceipt != nil ||
			workContext.Candidate == nil ||
			!runtimeDigestPattern.MatchString(
				workContext.Candidate.ProductTree,
			) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	case driver.AssemblyVerification:
		if workContext.Track != "" ||
			workContext.Slice != "" ||
			workContext.Authority.TrackRef != "" ||
			workContext.Authority.TrackHead != "" ||
			workContext.PreparedBase != "" ||
			workContext.DesignReceipt != nil ||
			workContext.Candidate == nil ||
			!runtimeDigestPattern.MatchString(
				workContext.Candidate.ProductTree,
			) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	return nil
}

func productionWorkContextV1(
	manifest admittedManifest,
	workContext productionWorkContext,
) (productionWorkContext, error) {
	if workContext.SchemaVersion != productionWorkContextVersion {
		return productionWorkContext{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	workContext.SchemaVersion = productionWorkContextVersionV1
	workContext.Track = ""
	workContext.PreparedBase = ""
	workContext.DesignReceipt = nil
	if err := validateProductionWorkContext(
		manifest,
		workContext,
	); err != nil {
		return productionWorkContext{}, err
	}
	return workContext, nil
}

func validProductionReceiptBinding(
	binding *productionReceiptBinding,
	bodyName string,
	bodyPath string,
	detailName string,
	detailPath string,
) bool {
	return binding != nil &&
		binding.OID != "" &&
		binding.BodyInput.Name == bodyName &&
		binding.BodyInput.Path == bodyPath &&
		runtimeDigestPattern.MatchString(binding.BodyInput.Digest) &&
		binding.DetailInput.Name == detailName &&
		binding.DetailInput.Path == detailPath &&
		runtimeDigestPattern.MatchString(binding.DetailInput.Digest)
}

func parseProductionDispatchCommand(
	manifest admittedManifest,
	body []byte,
) (productionDispatchCommand, error) {
	var command productionDispatchCommand
	if json.Unmarshal(body, &command) != nil ||
		!bytes.Equal(body, mustJSON(command)) ||
		(command.SchemaVersion != productionDispatchVersion &&
			command.SchemaVersion != productionDispatchVersionV1) ||
		(command.SchemaVersion == productionDispatchVersion &&
			command.Context.SchemaVersion !=
				productionWorkContextVersion) ||
		(command.SchemaVersion == productionDispatchVersionV1 &&
			command.Context.SchemaVersion !=
				productionWorkContextVersionV1) ||
		!runtimeDigestPattern.MatchString(command.RequestDigest) {
		return productionDispatchCommand{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := validateProductionWorkContext(manifest, command.Context); err != nil {
		return productionDispatchCommand{}, err
	}
	request, err := productionRequestForContext(manifest, command.Context)
	if err != nil {
		return productionDispatchCommand{}, err
	}
	requestBody, err := driver.EncodeRequest(request)
	if err != nil || driver.Digest(requestBody) != command.RequestDigest {
		return productionDispatchCommand{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	if command.SchemaVersion == productionDispatchVersionV1 {
		if command.ResumeRequestDigest != "" {
			return productionDispatchCommand{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return command, nil
	}
	if hasContinuationResumeRequest(command.Context) {
		resume, err := productionRequestForContextFreshness(
			manifest,
			command.Context,
			false,
		)
		if err != nil {
			return productionDispatchCommand{}, err
		}
		resumeBody, err := driver.EncodeRequest(resume)
		if err != nil ||
			driver.Digest(resumeBody) != command.ResumeRequestDigest {
			return productionDispatchCommand{},
				runtimeFail("CORRUPT_JOURNAL", err)
		}
	} else if command.ResumeRequestDigest != "" {
		return productionDispatchCommand{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return command, nil
}

func selectionForRole(
	selections driver.RoleSelections,
	role driver.Role,
) (driver.RoleSelection, bool) {
	switch role {
	case driver.RolePlanner:
		return selections.Planner, true
	case driver.RoleImplementer:
		return selections.Implementer, true
	case driver.RoleCaptain:
		return selections.Captain, true
	case driver.RoleVerifier:
		return selections.Verifier, true
	default:
		return driver.RoleSelection{}, false
	}
}

func productionRequestForContext(
	manifest admittedManifest,
	workContext productionWorkContext,
) (driver.Request, error) {
	return productionRequestForContextFreshness(
		manifest,
		workContext,
		true,
	)
}

func productionRequestForContextFreshness(
	manifest admittedManifest,
	workContext productionWorkContext,
	fresh bool,
) (driver.Request, error) {
	selection, ok := selectionForRole(
		manifest.value.Roles,
		workContext.Role,
	)
	if !ok {
		return driver.Request{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	contextBody := mustJSON(workContext)
	if len(contextBody) > driver.MaxInputFileBytes {
		return driver.Request{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	inputs := []driver.Input{{
		Name:   "work-context",
		Path:   productionWorkContextPath,
		Digest: driver.Digest(contextBody),
	}}
	if workContext.Plan != nil {
		inputs = append(inputs, workContext.Plan.Input)
	}
	if workContext.Receipt != nil {
		inputs = append(
			inputs,
			workContext.Receipt.BodyInput,
			workContext.Receipt.DetailInput,
		)
	}
	if workContext.DesignReceipt != nil {
		inputs = append(
			inputs,
			workContext.DesignReceipt.BodyInput,
			workContext.DesignReceipt.DetailInput,
		)
	}
	request, err := driver.NewRequest(
		workContext.InvocationID,
		workContext.Role,
		selection.Profile,
		selection.Model,
		driver.Workspace{
			Path:   driver.GuestWorkspacePath,
			Access: workContext.WorkspaceAccess,
		},
		inputs,
		fresh,
		manifest.value.Limits,
	)
	if err != nil {
		return driver.Request{},
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	return request, nil
}

func productionInputContents(
	workContext productionWorkContext,
	contextBody []byte,
) ([]driver.InputContent, error) {
	contents := []driver.InputContent{{
		Input: driver.Input{
			Name:   "work-context",
			Path:   productionWorkContextPath,
			Digest: driver.Digest(contextBody),
		},
		Bytes: contextBody,
	}}
	if workContext.Plan != nil {
		if driver.Digest(workContext.Plan.body) !=
			workContext.Plan.Input.Digest {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		contents = append(contents, driver.InputContent{
			Input: workContext.Plan.Input,
			Bytes: append([]byte(nil), workContext.Plan.body...),
		})
	}
	if workContext.Receipt != nil {
		if driver.Digest(workContext.Receipt.body) !=
			workContext.Receipt.BodyInput.Digest ||
			driver.Digest(workContext.Receipt.detail) !=
				workContext.Receipt.DetailInput.Digest {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		contents = append(
			contents,
			driver.InputContent{
				Input: workContext.Receipt.BodyInput,
				Bytes: append([]byte(nil), workContext.Receipt.body...),
			},
			driver.InputContent{
				Input: workContext.Receipt.DetailInput,
				Bytes: append([]byte(nil), workContext.Receipt.detail...),
			},
		)
	}
	if workContext.DesignReceipt != nil {
		if driver.Digest(workContext.DesignReceipt.body) !=
			workContext.DesignReceipt.BodyInput.Digest ||
			driver.Digest(workContext.DesignReceipt.detail) !=
				workContext.DesignReceipt.DetailInput.Digest {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		contents = append(
			contents,
			driver.InputContent{
				Input: workContext.DesignReceipt.BodyInput,
				Bytes: append(
					[]byte(nil),
					workContext.DesignReceipt.body...,
				),
			},
			driver.InputContent{
				Input: workContext.DesignReceipt.DetailInput,
				Bytes: append(
					[]byte(nil),
					workContext.DesignReceipt.detail...,
				),
			},
		)
	}
	return contents, nil
}
