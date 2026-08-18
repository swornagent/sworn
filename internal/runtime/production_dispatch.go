package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
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
	productionCaptainEnvelopePath  = "captain/delegation.json"
	productionCaptainProposalPath  = "captain/proposal.md"
)

var productionOutputExpectation = sha256Digest(
	[]byte("sworn.dynamic-driver-output/v1\n"),
)

type dispatchCoordinates struct {
	Slice           string
	Responsibility  driver.Responsibility
	BatonAttempt    int64
	Epoch           int64
	Try             int64
	InvocationScope string
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
	// Base is the commit the candidate was built on, and Chain spells the
	// ancestry out so a verifier checks bindings instead of reconstructing
	// them: base -> candidate (the diff under verification) -> candidate
	// receipt (the receipted head this dispatch's prepared_base names).
	Base  string `json:"base,omitempty"`
	Chain string `json:"chain,omitempty"`
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

const (
	productionHostEvidencePath    = "baton/host-evidence.json"
	productionHostEvidenceVersion = "sworn.host-evidence/v1"
)

// productionHostEvidence projects the engine's recorded host-boundary check
// evidence into a verifier's work context for a host-check slice. It is the
// read-don't-rerun surface: the verifier reads these journaled results
// instead of executing the declared containment-requiring checks itself, and
// ManifestDigest proves the projected results are the exact bytes the
// candidate receipt's Checks digest covered. It is optional and additive; a
// slice without host checks never carries it, so persisted v2 work contexts
// stay byte-compatible.
type productionHostEvidence struct {
	SchemaVersion  string                      `json:"schema_version"`
	Slice          string                      `json:"slice"`
	Candidate      string                      `json:"candidate"`
	ContractDigest string                      `json:"contract_digest"`
	ManifestDigest string                      `json:"manifest_digest"`
	Results        []productionHostCheckResult `json:"results"`
	Input          driver.Input                `json:"input"`
	body           []byte
}

type productionHostCheckResult struct {
	Check        string `json:"check"`
	Outcome      string `json:"outcome"`
	ExitCode     int    `json:"exit_code"`
	OutputDigest string `json:"output_digest"`
	Output       string `json:"output"`
	Truncated    bool   `json:"truncated"`
	Diagnostic   string `json:"diagnostic,omitempty"`
	HostEffect   string `json:"host_effect"`
}

type productionCaptainPlanBinding struct {
	EnvelopeDigest    string       `json:"envelope_digest"`
	EnvelopeEpoch     int64        `json:"envelope_epoch"`
	EnvelopeInput     driver.Input `json:"envelope_input"`
	ProposalReplayKey string       `json:"proposal_replay_key"`
	ProposalDigest    string       `json:"proposal_digest"`
	ProposalByteCount int64        `json:"proposal_byte_count"`
	ProposalInput     driver.Input `json:"proposal_input"`
	DecisionClass     string       `json:"decision_class"`
	PredicateResults  []string     `json:"predicate_results"`
	envelopeBody      []byte
	proposalBody      []byte
}

type productionWorkContext struct {
	SchemaVersion      string                            `json:"schema_version"`
	ManifestDigest     string                            `json:"manifest_digest"`
	DriverConfigDigest string                            `json:"driver_config_digest"`
	RunID              string                            `json:"run_id"`
	Repository         string                            `json:"repository"`
	Release            string                            `json:"release"`
	Intent             string                            `json:"intent"`
	InvocationID       string                            `json:"invocation_id"`
	InvocationScope    string                            `json:"invocation_scope,omitempty"`
	PlannerAttempt     int64                             `json:"planner_attempt,omitempty"`
	ReplanDecision     string                            `json:"replan_decision,omitempty"`
	Role               driver.Role                       `json:"role"`
	Track              string                            `json:"track,omitempty"`
	Slice              string                            `json:"slice,omitempty"`
	Responsibility     driver.Responsibility             `json:"responsibility"`
	Attempt            int64                             `json:"attempt"`
	Epoch              int64                             `json:"epoch"`
	Try                int64                             `json:"try"`
	Before             string                            `json:"before"`
	WorkspaceAccess    driver.WorkspaceAccess            `json:"workspace_access"`
	Authority          productionAuthorityBinding        `json:"authority"`
	PreparedBase       string                            `json:"prepared_base,omitempty"`
	Plan               *productionPlanBinding            `json:"plan,omitempty"`
	Receipt            *productionReceiptBinding         `json:"receipt,omitempty"`
	DesignReceipt      *productionReceiptBinding         `json:"design_receipt,omitempty"`
	Candidate          *productionCandidateBinding       `json:"candidate,omitempty"`
	Evidence           []productionEvidenceBinding       `json:"evidence"`
	HostEvidence       *productionHostEvidence           `json:"host_evidence,omitempty"`
	CaptainPlan        *productionCaptainPlanBinding     `json:"captain_plan,omitempty"`
	Refusal            *productionRefusalBinding         `json:"refusal,omitempty"`
	PriorSubmission    *productionPriorSubmissionBinding `json:"prior_submission,omitempty"`
}

type productionPriorSubmissionBinding struct {
	Summary    string `json:"summary"`
	Detail     string `json:"detail"`
	Provenance string `json:"provenance"`
}

type productionRefusalBinding struct {
	Code       string   `json:"code"`
	Detail     string   `json:"detail,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	TotalPaths int      `json:"total_paths,omitempty"`
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
	case driver.CaptainReview, driver.CaptainPlanReview:
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
	if coordinates.InvocationScope != "" {
		work += "-" + coordinates.InvocationScope
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
	base := ""
	if entry.Receipt.Base != nil {
		base = *entry.Receipt.Base
	}
	binding := &productionCandidateBinding{
		Receipt:     entry.OID,
		Commit:      *entry.Receipt.Candidate,
		ProductTree: productTree,
		Base:        base,
	}
	if base != "" {
		binding.Chain = "base " + base +
			" -> candidate " + binding.Commit +
			" (the diff under verification) -> candidate receipt " +
			binding.Receipt +
			" (the receipted track head; this dispatch's prepared_base names this receipted head, not the build base)"
	}
	return binding, nil
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
		InvocationScope:    coordinates.InvocationScope,
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
	} else if coordinates.Responsibility == driver.CaptainPlanReview {
		if coordinates.Slice != "" {
			return productionWorkContext{}, nil, runtimeFail("INVALID_PRODUCTION_DISPATCH", nil)
		}
		if err := captureCaptainPlanReviewContext(ctx, engine, coordinates, before, &workContext); err != nil {
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

func captainReviewBefore(proposal admittedPlanProposal, delegation CaptainDelegationState) string {
	return sha256Digest(mustJSON(struct {
		SchemaVersion     string `json:"schema_version"`
		ProposalReplayKey string `json:"proposal_replay_key"`
		PlanDigest        string `json:"plan_digest"`
		EnvelopeDigest    string `json:"envelope_digest"`
		EnvelopeEpoch     int64  `json:"envelope_epoch"`
		TargetHead        string `json:"target_head"`
		ReleaseHead       string `json:"release_head"`
	}{"sworn.captain-plan-review-binding/v1", proposal.replayKey, proposal.plan.Digest(), delegation.Digest, delegation.Epoch, proposal.authority.TargetHead, proposal.authority.ReleaseHead}))
}

func captureCaptainPlanReviewContext(ctx context.Context, engine *engine, coordinates dispatchCoordinates, before string, workContext *productionWorkContext) error {
	snapshot, err := engineSnapshot(ctx, engine)
	if err != nil {
		return err
	}
	manifest, proposals, err := loadRunSnapshot(snapshot, engine.manifest.value.RunID)
	if err != nil || manifest.digest != engine.manifest.digest {
		return runtimeFail("RUN_BINDING_MISMATCH", err)
	}
	state, stateErr := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
	proposal, found, _, err := selectPlanProposal(engine, snapshot, proposals, state, stateErr)
	if err != nil || !found {
		return runtimeFail("STALE_DISPATCH", err)
	}
	delegation, err := currentCaptainDelegation(snapshot)
	if err != nil || !delegation.Active || captainReviewBefore(proposal, delegation) != before {
		return runtimeFail("STALE_DISPATCH", err)
	}
	class, err := approvalDecisionClass(proposal)
	if err != nil {
		return err
	}
	workContext.Authority = productionAuthorityBinding{ReleaseRef: proposal.authority.ReleaseRef, ReleaseHead: proposal.authority.ReleaseHead, TargetRef: proposal.authority.TargetRef, TargetHead: proposal.authority.TargetHead}
	workContext.CaptainPlan = &productionCaptainPlanBinding{EnvelopeDigest: delegation.Digest, EnvelopeEpoch: delegation.Epoch, EnvelopeInput: driver.Input{Name: "captain-delegation", Path: productionCaptainEnvelopePath, Digest: driver.Digest(delegation.EnvelopeBytes)}, ProposalReplayKey: proposal.replayKey, ProposalDigest: proposal.plan.Digest(), ProposalByteCount: int64(len(proposal.plan.Bytes())), ProposalInput: driver.Input{Name: "captain-proposal", Path: productionCaptainProposalPath, Digest: driver.Digest(proposal.plan.Bytes())}, DecisionClass: class, PredicateResults: []string{"authority_active", "bindings_exact", "limits_available", "policy_admitted", "proposal_unique"}, envelopeBody: append([]byte(nil), delegation.EnvelopeBytes...), proposalBody: proposal.plan.Bytes()}
	return nil
}

func engineSnapshot(ctx context.Context, engine *engine) (journal.Snapshot, error) {
	if engine == nil || engine.journal == nil {
		return journal.Snapshot{}, runtimeFail("JOURNAL_READ_FAILED", nil)
	}
	snapshot, err := engine.journal.Snapshot(ctx, engine.manifest.value.RunID)
	if err != nil {
		return journal.Snapshot{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	return snapshot, nil
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
		Release:        engine.manifest.value.Release,
		ReleaseRef:     release.Ref,
		TargetRef:      target.Ref,
		TargetHead:     target.Head.String(),
		PlannerAttempt: 1,
	}
	if engine.journal != nil {
		snapshot, snapshotErr := engine.journal.Snapshot(context.Background(), engine.manifest.value.RunID)
		if snapshotErr != nil {
			return runtimeFail("JOURNAL_READ_FAILED", snapshotErr)
		}
		for _, stored := range snapshot.Commands {
			if stored.Kind != "planner_continuation" {
				continue
			}
			var continuation CaptainPlannerContinuationCommand
			if json.Unmarshal(stored.Payload, &continuation) != nil {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if continuation.PlanRevision == coordinates.BatonAttempt && continuation.PlannerAttempt > authority.PlannerAttempt {
				authority.PlannerAttempt = continuation.PlannerAttempt
				authority.ReplanDecision = continuation.DecisionReplayKey
			}
		}
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
	workContext.PlannerAttempt = authority.PlannerAttempt
	workContext.ReplanDecision = authority.ReplanDecision
	if coordinates.Try > 1 || authority.PlannerAttempt > 1 || coordinates.BatonAttempt > 1 {
		coords := coordinates
		coords.Responsibility = driver.PlannerProposal
		priorSub, priorErr := capturePriorPlannerSubmission(engine, coords, authority)
		if priorErr != nil {
			return priorErr
		}
		workContext.PriorSubmission = priorSub
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
		if coordinates.Try > 1 {
			refusal, refusalErr := capturePriorRefusal(ctx, engine, coordinates, before)
			if refusalErr != nil {
				return refusalErr
			}
			workContext.Refusal = refusal
		}
	}
	workContext.Evidence = sliceEvidence(slice.ConsumedInputs)
	if coordinates.Responsibility == driver.WorkVerification {
		workContext.Candidate, err = candidateBinding(slice.Candidate)
		if err != nil {
			return err
		}
		if err := captureHostEvidence(
			ctx, engine, state, slice, workContext,
		); err != nil {
			return err
		}
	}
	if coordinates.Try > 1 || coordinates.BatonAttempt > 1 {
		priorSub, priorErr := capturePriorSubmission(ctx, engine, coordinates, before)
		if priorErr != nil {
			return priorErr
		}
		workContext.PriorSubmission = priorSub
	}
	return nil
}

// captureHostEvidence projects the recorded host-boundary evidence into a
// WorkVerification work context when the slice's approved contract declares
// containment-requiring checks. It reads the journaled check.host effects for
// the exact candidate (never re-running them in a worker), rebuilds the
// host-boundary portion of the receipt manifest, and proves the projected
// bytes are exactly what the candidate receipt's Checks digest covered. A
// declared host check whose journaled evidence is missing or not succeeded
// fails closed, so a verifier can never be handed incomplete host evidence.
func captureHostEvidence(
	ctx context.Context,
	engine *engine,
	state baton.State,
	slice *baton.SliceState,
	workContext *productionWorkContext,
) error {
	if engine == nil || slice == nil || workContext == nil ||
		slice.Candidate == nil || slice.Candidate.Receipt.Candidate == nil {
		return runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	if workContext.Plan == nil {
		return runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	plan, err := baton.ParsePlan(workContext.Plan.body)
	if err != nil || plan.Digest() != workContext.Plan.Digest {
		return runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	hostChecks, contractDigest, resolveErr := resolveSliceHostChecks(
		engine, plan, workContext.Slice, state.Refs.Target.Head)
	if resolveErr != nil {
		return resolveErr
	}
	if len(hostChecks) == 0 {
		return nil
	}
	candidate := *slice.Candidate.Receipt.Candidate
	results, readErr := readJournaledHostResults(
		ctx, engine, workContext.Slice, candidate, contractDigest, hostChecks)
	if readErr != nil {
		return readErr
	}
	roleDigest := ""
	if slice.Candidate.Receipt.Checks != nil {
		roleDigest = *slice.Candidate.Receipt.Checks
	}
	body := mustJSON(productionHostEvidence{
		SchemaVersion:  productionHostEvidenceVersion,
		Slice:          workContext.Slice,
		Candidate:      candidate,
		ContractDigest: contractDigest,
		ManifestDigest: roleDigest,
		Results:        productionHostResults(results),
	})
	workContext.HostEvidence = &productionHostEvidence{
		SchemaVersion:  productionHostEvidenceVersion,
		Slice:          workContext.Slice,
		Candidate:      candidate,
		ContractDigest: contractDigest,
		ManifestDigest: roleDigest,
		Results:        productionHostResults(results),
		Input: driver.Input{
			Name:   "host-evidence",
			Path:   productionHostEvidencePath,
			Digest: driver.Digest(body),
		},
		body: body,
	}
	return nil
}

func productionHostResults(
	results []hostCheckResult,
) []productionHostCheckResult {
	projected := make([]productionHostCheckResult, len(results))
	for index, result := range results {
		projected[index] = productionHostCheckResult{
			Check: result.Check, Outcome: result.Outcome,
			ExitCode: result.ExitCode, OutputDigest: result.OutputDigest,
			Output: result.Output, Truncated: result.Truncated,
			Diagnostic: result.Diagnostic, HostEffect: result.EffectID,
		}
	}
	return projected
}

// readJournaledHostResults reads the exactly-once journaled check.host
// effects for the declared host checks of one exact candidate. It never
// executes anything; a missing or non-succeeded effect fails closed.
func readJournaledHostResults(
	ctx context.Context,
	engine *engine,
	sliceID, candidate, contractDigest string,
	hostChecks []string,
) ([]hostCheckResult, error) {
	results := make([]hostCheckResult, 0, len(hostChecks))
	for _, check := range hostChecks {
		work := hostCheckWork(sliceID, candidate, contractDigest, check)
		effectID := hostCheckEffectID(work)
		effect, err := engine.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
		if err != nil {
			return nil, runtimeFail("JOURNAL_READ_FAILED", err)
		}
		if effect.Kind != "check.host" || effect.State != journal.Succeeded {
			return nil, runtimeFail("HOST_CHECK_EVIDENCE_MISSING", nil)
		}
		result, err := parseHostCheckResult(
			sliceID, candidate, contractDigest, check, effectID, effect.Result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
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

func capturePriorRefusal(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
) (*productionRefusalBinding, error) {
	if engine == nil || engine.journal == nil || coordinates.Try <= 1 {
		return nil, nil
	}
	priorTry := coordinates.Try - 1
	workID := workIdentity(before, "git.seal")

	// 1. Recovery mode dispatch effect
	dispatchWorkRec := workIdentity(workID, "driver.dispatch")
	dispatchEffectRec := journal.AttemptEffectID(dispatchWorkRec, coordinates.Epoch, priorTry)

	// 2. Non-recovery mode dispatch effect
	priorOuterID := journal.AttemptEffectID(workID, coordinates.Epoch, priorTry)
	dispatchWorkOuter := workIdentity(priorOuterID, "driver.dispatch")
	dispatchEffectOuter := journal.AttemptEffectID(dispatchWorkOuter, 1, 1)

	// 3. General driver dispatch work identity
	generalWork := driverWorkIdentity(
		engine.manifest.digest,
		coordinates.Slice,
		coordinates.Responsibility,
		coordinates.BatonAttempt,
		before,
	)
	generalDispatchEffect := journal.AttemptEffectID(generalWork, coordinates.Epoch, priorTry)

	candidateIDs := []string{dispatchEffectRec, dispatchEffectOuter, generalDispatchEffect, priorOuterID}
	for _, effectID := range candidateIDs {
		effect, err := engine.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
		if err != nil {
			continue
		}
		if len(effect.Result) > 0 {
			var refusal productionRefusalBinding
			if err := json.Unmarshal(effect.Result, &refusal); err == nil &&
				refusal.Code != "" && len(refusal.Paths) > 0 {
				return &refusal, nil
			}
		}
	}
	return nil, nil
}

func extractRefusal(err error) *productionRefusalBinding {
	if err == nil {
		return nil
	}
	var gitErr *gitx.Error
	if errors.As(err, &gitErr) && len(gitErr.Paths) > 0 {
		total := gitErr.TotalPaths
		if total < len(gitErr.Paths) {
			total = len(gitErr.Paths)
		}
		return &productionRefusalBinding{
			Code:       gitErr.Code,
			Paths:      append([]string(nil), gitErr.Paths...),
			TotalPaths: total,
		}
	}
	var recordErr *baton.RecordError
	if errors.As(err, &recordErr) && len(recordErr.Paths) > 0 {
		total := recordErr.TotalPaths
		if total < len(recordErr.Paths) {
			total = len(recordErr.Paths)
		}
		return &productionRefusalBinding{
			Code:       recordErr.Code,
			Paths:      append([]string(nil), recordErr.Paths...),
			TotalPaths: total,
		}
	}
	var contractErr *driver.ContractError
	if errors.As(err, &contractErr) && contractErr.Detail != "" {
		return &productionRefusalBinding{
			Code:   contractErr.Code,
			Detail: contractErr.Detail,
		}
	}
	return nil
}

func extractRefusalResult(err error) []byte {
	refusal := extractRefusal(err)
	if refusal == nil {
		return nil
	}
	return mustJSON(refusal)
}

func capturePriorSubmission(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
) (*productionPriorSubmissionBinding, error) {
	if engine == nil || engine.journal == nil ||
		(coordinates.Try <= 1 && coordinates.BatonAttempt <= 1) {
		return nil, nil
	}
	snapshot, err := engineSnapshot(ctx, engine)
	if err != nil {
		return nil, err
	}
	effectsByID := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, eff := range snapshot.Effects {
		effectsByID[eff.ID] = eff
	}

	type priorCandidate struct {
		attempt    int64
		try        int64
		submission driver.Submission
	}
	var best *priorCandidate

	for _, cmd := range snapshot.Commands {
		if cmd.Kind != "driver.dispatch" {
			continue
		}
		var command productionDispatchCommand
		if json.Unmarshal(cmd.Payload, &command) != nil {
			continue
		}
		if command.Context.Slice != coordinates.Slice ||
			command.Context.Responsibility != coordinates.Responsibility {
			continue
		}
		candAttempt := command.Context.Attempt
		candTry := command.Context.Try
		if candAttempt > coordinates.BatonAttempt {
			continue
		}
		if candAttempt == coordinates.BatonAttempt && candTry >= coordinates.Try {
			continue
		}
		eff, found := effectsByID[cmd.ReplayKey]
		if !found || len(eff.Result) == 0 {
			eff, err = engine.journal.Effect(ctx, engine.manifest.value.RunID, cmd.ReplayKey)
			if err != nil || len(eff.Result) == 0 {
				continue
			}
		}
		sub, decErr := driver.DecodeSubmission(eff.Result)
		if decErr != nil {
			continue
		}
		if best == nil || candAttempt > best.attempt ||
			(candAttempt == best.attempt && candTry > best.try) {
			best = &priorCandidate{
				attempt:    candAttempt,
				try:        candTry,
				submission: sub,
			}
		}
	}

	startAttempt := coordinates.BatonAttempt
	for att := startAttempt; att >= 1; att-- {
		startTry := int64(3)
		if att == coordinates.BatonAttempt {
			startTry = coordinates.Try - 1
		}
		for t := startTry; t >= 1; t-- {
			generalWork := driverWorkIdentity(
				engine.manifest.digest,
				coordinates.Slice,
				coordinates.Responsibility,
				att,
				before,
			)
			dispatchEffect := journal.AttemptEffectID(generalWork, coordinates.Epoch, t)
			workID := workIdentity(before, "git.seal")
			dispatchWorkRec := workIdentity(workID, "driver.dispatch")
			dispatchEffectRec := journal.AttemptEffectID(dispatchWorkRec, coordinates.Epoch, t)
			priorOuterID := journal.AttemptEffectID(workID, coordinates.Epoch, t)
			dispatchWorkOuter := workIdentity(priorOuterID, "driver.dispatch")
			dispatchEffectOuter := journal.AttemptEffectID(dispatchWorkOuter, 1, 1)

			candidates := []string{dispatchEffectRec, dispatchEffectOuter, dispatchEffect, priorOuterID}
			for _, effectID := range candidates {
				eff, found := effectsByID[effectID]
				if !found || len(eff.Result) == 0 {
					eff, err = engine.journal.Effect(ctx, engine.manifest.value.RunID, effectID)
					if err != nil || len(eff.Result) == 0 {
						continue
					}
				}
				sub, decErr := driver.DecodeSubmission(eff.Result)
				if decErr != nil {
					continue
				}
				if best == nil || att > best.attempt ||
					(att == best.attempt && t > best.try) {
					best = &priorCandidate{
						attempt:    att,
						try:        t,
						submission: sub,
					}
				}
			}
		}
	}

	if best == nil {
		return nil, nil
	}
	provenance := fmt.Sprintf("try %d", best.try)
	if best.attempt > 1 || coordinates.BatonAttempt > 1 {
		provenance = fmt.Sprintf("attempt %d, try %d", best.attempt, best.try)
	}
	return &productionPriorSubmissionBinding{
		Summary:    best.submission.Summary,
		Detail:     best.submission.Detail,
		Provenance: provenance,
	}, nil
}

func capturePriorPlannerSubmission(
	engine *engine,
	coordinates dispatchCoordinates,
	authority planProposalAuthority,
) (*productionPriorSubmissionBinding, error) {
	if engine == nil || engine.journal == nil ||
		(coordinates.Try <= 1 && authority.PlannerAttempt <= 1 && coordinates.BatonAttempt <= 1) {
		return nil, nil
	}
	snapshot, err := engine.journal.Snapshot(context.Background(), engine.manifest.value.RunID)
	if err != nil {
		return nil, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	effectsByID := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, eff := range snapshot.Effects {
		effectsByID[eff.ID] = eff
	}

	type priorCandidate struct {
		attempt        int64
		plannerAttempt int64
		try            int64
		submission     driver.Submission
	}
	var best *priorCandidate

	for _, cmd := range snapshot.Commands {
		if cmd.Kind != "driver.dispatch" {
			continue
		}
		var command productionDispatchCommand
		if json.Unmarshal(cmd.Payload, &command) != nil {
			continue
		}
		if command.Context.Responsibility != driver.PlannerProposal {
			continue
		}
		candAttempt := command.Context.Attempt
		candPlannerAttempt := command.Context.PlannerAttempt
		candTry := command.Context.Try
		isPrior := false
		if candAttempt < coordinates.BatonAttempt {
			isPrior = true
		} else if candAttempt == coordinates.BatonAttempt {
			if candPlannerAttempt < authority.PlannerAttempt {
				isPrior = true
			} else if candPlannerAttempt == authority.PlannerAttempt && candTry < coordinates.Try {
				isPrior = true
			}
		}
		if !isPrior {
			continue
		}
		eff, found := effectsByID[cmd.ReplayKey]
		if !found || len(eff.Result) == 0 {
			continue
		}
		sub, decErr := driver.DecodeSubmission(eff.Result)
		if decErr != nil {
			continue
		}
		if best == nil || candAttempt > best.attempt ||
			(candAttempt == best.attempt && candPlannerAttempt > best.plannerAttempt) ||
			(candAttempt == best.attempt && candPlannerAttempt == best.plannerAttempt && candTry > best.try) {
			best = &priorCandidate{
				attempt:        candAttempt,
				plannerAttempt: candPlannerAttempt,
				try:            candTry,
				submission:     sub,
			}
		}
	}

	if best == nil {
		return nil, nil
	}
	provenance := fmt.Sprintf("try %d", best.try)
	if best.plannerAttempt > 1 || authority.PlannerAttempt > 1 {
		provenance = fmt.Sprintf("planner_attempt %d, try %d", best.plannerAttempt, best.try)
	} else if best.attempt > 1 || coordinates.BatonAttempt > 1 {
		provenance = fmt.Sprintf("attempt %d, try %d", best.attempt, best.try)
	}
	return &productionPriorSubmissionBinding{
		Summary:    best.submission.Summary,
		Detail:     best.submission.Detail,
		Provenance: provenance,
	}, nil
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
				Slice:           workContext.Slice,
				Responsibility:  workContext.Responsibility,
				BatonAttempt:    workContext.Attempt,
				Epoch:           workContext.Epoch,
				Try:             workContext.Try,
				InvocationScope: workContext.InvocationScope,
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
	if workContext.InvocationScope != "" {
		workID := driverWorkIdentity(
			workContext.ManifestDigest,
			workContext.Slice,
			workContext.Responsibility,
			workContext.Attempt,
			workContext.Before,
		)
		expected := strings.TrimPrefix(workID, "sha256:")[:12]
		if workContext.InvocationScope != expected ||
			(workContext.Responsibility != driver.PlannerProposal &&
				workContext.Responsibility != driver.CaptainPlanReview) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	if workContext.SchemaVersion == productionWorkContextVersionV1 &&
		(workContext.Track != "" ||
			workContext.PreparedBase != "" ||
			workContext.DesignReceipt != nil ||
			workContext.HostEvidence != nil ||
			workContext.Refusal != nil ||
			workContext.PriorSubmission != nil) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if workContext.PriorSubmission != nil {
		if workContext.Try <= 1 && workContext.Attempt <= 1 && workContext.PlannerAttempt <= 1 {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		sub := workContext.PriorSubmission
		if !utf8.ValidString(sub.Summary) ||
			len([]byte(sub.Summary)) > driver.MaxSubmissionSummaryBytes ||
			strings.TrimSpace(sub.Summary) == "" {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if len([]byte(sub.Detail)) > driver.MaxSubmissionDetailBytes ||
			!utf8.ValidString(sub.Detail) ||
			strings.ContainsRune(sub.Detail, '\x00') ||
			strings.ContainsRune(sub.Detail, '\r') ||
			strings.Contains(sub.Detail, "Baton-Detail-Begin") ||
			strings.Contains(sub.Detail, "Baton-Detail-End") {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if !utf8.ValidString(sub.Provenance) ||
			strings.TrimSpace(sub.Provenance) == "" ||
			len([]byte(sub.Provenance)) > 1000 ||
			containsControlCharacter(sub.Provenance) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	}
	if workContext.Refusal != nil {
		if workContext.Try <= 1 {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		refusal := workContext.Refusal
		if !runtimeIdentityPattern.MatchString(refusal.Code) ||
			len(refusal.Paths) < 1 || len(refusal.Paths) > 20 ||
			refusal.TotalPaths < len(refusal.Paths) {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		for index, p := range refusal.Paths {
			if gitx.ValidatePath(p, false) != nil {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if index > 0 && refusal.Paths[index-1] >= p {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
		}
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
	if workContext.HostEvidence != nil {
		evidence := workContext.HostEvidence
		if evidence.SchemaVersion != productionHostEvidenceVersion ||
			evidence.Slice != workContext.Slice ||
			evidence.Slice == "" ||
			!validGitObjectID(evidence.Candidate) ||
			!runtimeDigestPattern.MatchString(evidence.ContractDigest) ||
			!runtimeDigestPattern.MatchString(evidence.ManifestDigest) ||
			evidence.Input.Name != "host-evidence" ||
			evidence.Input.Path != productionHostEvidencePath ||
			!runtimeDigestPattern.MatchString(evidence.Input.Digest) ||
			len(evidence.Results) == 0 ||
			workContext.Responsibility != driver.WorkVerification ||
			workContext.Candidate == nil ||
			workContext.Candidate.Commit != evidence.Candidate {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		seen := make(map[string]struct{}, len(evidence.Results))
		for _, result := range evidence.Results {
			if result.Check == "" || !runtimeIdentityPattern.MatchString(result.Outcome) ||
				!runtimeDigestPattern.MatchString(result.OutputDigest) ||
				result.HostEffect == "" {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			if _, duplicate := seen[result.Check]; duplicate {
				return runtimeFail("CORRUPT_JOURNAL", nil)
			}
			seen[result.Check] = struct{}{}
		}
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
			Release:        workContext.Release,
			PriorPlan:      priorPlan,
			ReleaseRef:     workContext.Authority.ReleaseRef,
			ReleaseHead:    workContext.Authority.ReleaseHead,
			TargetRef:      workContext.Authority.TargetRef,
			TargetHead:     workContext.Authority.TargetHead,
			PlannerAttempt: workContext.PlannerAttempt,
			ReplanDecision: workContext.ReplanDecision,
		}
		if authority.PlannerAttempt == 0 {
			authority.PlannerAttempt = 1
		}
		if authority.PlannerAttempt < 1 ||
			(authority.PlannerAttempt == 1 && authority.ReplanDecision != "") ||
			(authority.PlannerAttempt > 1 && authority.ReplanDecision == "") {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if plannerAuthorityBefore(authority) != workContext.Before {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
	} else if workContext.PlannerAttempt != 0 || workContext.ReplanDecision != "" {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	} else if workContext.Responsibility != driver.CaptainPlanReview && (workContext.Plan == nil || workContext.Receipt == nil) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	switch workContext.Responsibility {
	case driver.CaptainPlanReview:
		binding := workContext.CaptainPlan
		if workContext.Track != "" || workContext.Slice != "" || workContext.Plan != nil || workContext.Receipt != nil || workContext.DesignReceipt != nil || workContext.Candidate != nil || len(workContext.Evidence) != 0 || binding == nil ||
			!runtimeDigestPattern.MatchString(binding.EnvelopeDigest) || binding.EnvelopeEpoch < 1 || binding.EnvelopeInput.Name != "captain-delegation" || binding.EnvelopeInput.Path != productionCaptainEnvelopePath || !runtimeDigestPattern.MatchString(binding.EnvelopeInput.Digest) || binding.ProposalReplayKey == "" || !runtimeDigestPattern.MatchString(binding.ProposalDigest) || binding.ProposalByteCount < 1 || binding.ProposalInput.Name != "captain-proposal" || binding.ProposalInput.Path != productionCaptainProposalPath || !runtimeDigestPattern.MatchString(binding.ProposalInput.Digest) || (binding.DecisionClass != PlannerProposalClass && binding.DecisionClass != PlannerReplanClass) || len(binding.PredicateResults) == 0 || captainReviewBeforeFromBinding(binding, workContext.Authority) != workContext.Before {
			return runtimeFail("CORRUPT_JOURNAL", nil)
		}
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
	if workContext.Responsibility != driver.CaptainPlanReview && workContext.CaptainPlan != nil {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return nil
}

func captainReviewBeforeFromBinding(binding *productionCaptainPlanBinding, authority productionAuthorityBinding) string {
	return sha256Digest(mustJSON(struct {
		SchemaVersion     string `json:"schema_version"`
		ProposalReplayKey string `json:"proposal_replay_key"`
		PlanDigest        string `json:"plan_digest"`
		EnvelopeDigest    string `json:"envelope_digest"`
		EnvelopeEpoch     int64  `json:"envelope_epoch"`
		TargetHead        string `json:"target_head"`
		ReleaseHead       string `json:"release_head"`
	}{"sworn.captain-plan-review-binding/v1", binding.ProposalReplayKey, binding.ProposalDigest, binding.EnvelopeDigest, binding.EnvelopeEpoch, authority.TargetHead, authority.ReleaseHead}))
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
	workContext.HostEvidence = nil
	workContext.Refusal = nil
	workContext.PriorSubmission = nil
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
	if workContext.HostEvidence != nil {
		inputs = append(inputs, workContext.HostEvidence.Input)
	}
	if workContext.CaptainPlan != nil {
		inputs = append(inputs, workContext.CaptainPlan.EnvelopeInput, workContext.CaptainPlan.ProposalInput)
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
	if workContext.CaptainPlan != nil {
		binding := workContext.CaptainPlan
		if driver.Digest(binding.envelopeBody) != binding.EnvelopeInput.Digest || driver.Digest(binding.proposalBody) != binding.ProposalInput.Digest {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		contents = append(contents, driver.InputContent{Input: binding.EnvelopeInput, Bytes: append([]byte(nil), binding.envelopeBody...)}, driver.InputContent{Input: binding.ProposalInput, Bytes: append([]byte(nil), binding.proposalBody...)})
	}
	if workContext.HostEvidence != nil {
		evidence := workContext.HostEvidence
		if driver.Digest(evidence.body) != evidence.Input.Digest {
			return nil, runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		contents = append(contents, driver.InputContent{
			Input: evidence.Input,
			Bytes: append([]byte(nil), evidence.body...),
		})
	}
	return contents, nil
}
