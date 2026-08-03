package gitx

import (
	"errors"
	"fmt"
)

// MaxTrackBaseInputs matches Baton's bounded list cardinality. This transport
// must not introduce a narrower topology limit than the admitted plan.
const MaxTrackBaseInputs = 256

type TrackBaseAction string

const (
	TrackBaseNoop   TrackBaseAction = "noop"
	TrackBaseCreate TrackBaseAction = "create"
	TrackBaseUpdate TrackBaseAction = "update"
)

type TrackBaseInput struct {
	Slice            string
	Producer         TrackKey
	SourceHead       OID
	PassReceipt      OID
	CandidateReceipt OID
	Candidate        OID
	ProductTree      string
}

// PrepareTrackBaseRequest binds every authority ref and every consumed PASS
// chain used to prepare one consumer track. AuthoritySeed is immutable engine
// authority; ConsumerBefore is only the live consumer-ref CAS expectation.
type PrepareTrackBaseRequest struct {
	Release     string
	Plan        OID
	ReleaseHead OID
	TargetRef   string
	// TargetHead is the current live ref authority used for compare-and-set.
	TargetHead OID
	// ApprovedTarget is the immutable plan base used for track composition.
	ApprovedTarget     OID
	Consumer           TrackKey
	AuthoritySeed      OID
	ConsumerBefore     *OID
	Inputs             []TrackBaseInput
	ProductAdmission   *ProductExclusionAdmission
	ResolveProductBase func(TrackBaseInput) (OID, error)
}

type PrepareTrackBaseResult struct {
	Action         TrackBaseAction
	Changed        bool
	ConsumerRef    string
	ConsumerBefore *OID
	Seed           OID
	SeedTree       OID
	Base           OID
	BaseTree       OID
	Inputs         []TrackBaseInput
}

type TrackBaseReconciliation string

const (
	TrackBaseAllOld    TrackBaseReconciliation = "all_old"
	TrackBaseAllNew    TrackBaseReconciliation = "all_new"
	TrackBaseAdvanced  TrackBaseReconciliation = "advanced"
	TrackBaseAmbiguous TrackBaseReconciliation = "ambiguous"
)

func cloneOptionalOID(value *OID) *OID {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTrackBaseInputs(values []TrackBaseInput) []TrackBaseInput {
	if values == nil {
		return nil
	}
	result := make([]TrackBaseInput, len(values))
	copy(result, values)
	return result
}

func sameOptionalOID(left, right *OID) bool {
	return (left == nil && right == nil) ||
		(left != nil && right != nil && *left == *right)
}

func (w *Workspaces) validateTrackBaseRequest(
	request PrepareTrackBaseRequest,
) (string, OID, error) {
	if w == nil || w.repository == nil {
		return "", OID{}, fail(
			"INVALID_WORKSPACE_OWNER",
			"prepare track base",
			nil,
		)
	}
	if err := validateTrackKey(request.Consumer); err != nil {
		return "", OID{}, err
	}
	if request.Release != request.Consumer.Release {
		return "", OID{}, fail(
			"INVALID_TRACK_BASE",
			"prepare track base",
			errors.New("release and consumer identities differ"),
		)
	}
	if err := w.repository.validateOID(request.Plan); err != nil {
		return "", OID{}, err
	}
	if err := w.repository.validateOID(request.ReleaseHead); err != nil {
		return "", OID{}, err
	}
	if err := w.repository.validateOID(request.TargetHead); err != nil {
		return "", OID{}, err
	}
	if err := w.repository.validateOID(request.ApprovedTarget); err != nil {
		return "", OID{}, err
	}
	if err := w.repository.validateOID(request.AuthoritySeed); err != nil {
		return "", OID{}, err
	}
	if err := ValidateHeadRef(request.TargetRef); err != nil {
		return "", OID{}, err
	}
	releaseRef := "refs/heads/release-wt/" + request.Release
	consumerRef := trackHeadRef(request.Consumer)
	if request.TargetRef == releaseRef || request.TargetRef == consumerRef {
		return "", OID{}, fail(
			"INVALID_TRACK_BASE",
			"prepare track base",
			errors.New("target aliases protected authority"),
		)
	}
	if request.ConsumerBefore != nil {
		if err := w.repository.validateOID(*request.ConsumerBefore); err != nil {
			return "", OID{}, err
		}
	}
	if len(request.Inputs) > MaxTrackBaseInputs {
		return "", OID{}, fail(
			"RESOURCE_LIMIT",
			"prepare track base",
			fmt.Errorf("inputs exceed %d", MaxTrackBaseInputs),
		)
	}
	seenSlices := make(map[string]bool, len(request.Inputs))
	for _, input := range request.Inputs {
		if !workspaceIdentityPattern.MatchString(input.Slice) ||
			seenSlices[input.Slice] ||
			validateTrackKey(input.Producer) != nil ||
			input.Producer.Release != request.Release ||
			input.ProductTree == "" {
			return "", OID{}, fail(
				"INVALID_TRACK_BASE",
				"prepare track base",
				errors.New("invalid consumed input identity"),
			)
		}
		seenSlices[input.Slice] = true
		for _, oid := range []OID{
			input.SourceHead,
			input.PassReceipt,
			input.CandidateReceipt,
			input.Candidate,
		} {
			if err := w.repository.validateOID(oid); err != nil {
				return "", OID{}, err
			}
		}
	}
	return consumerRef, request.AuthoritySeed, nil
}

func (w *Workspaces) validatePlanAtRelease(
	request PrepareTrackBaseRequest,
) error {
	path := recordRoot + "/" + request.Release + "/plan.md"
	entries, err := w.repository.ListTree(request.ReleaseHead)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Path == path && entry.Type == "blob" &&
			entry.OID == request.Plan {
			return nil
		}
	}
	return fail(
		"PLAN_MOVED",
		"prepare track base",
		errors.New("release head does not contain the exact plan"),
	)
}

func (w *Workspaces) validateTrackBaseInput(
	input TrackBaseInput,
	seed OID,
	consumerRef string,
	previousByRef map[string]OID,
	request PrepareTrackBaseRequest,
) error {
	candidateParents, err := w.repository.Parents(input.CandidateReceipt)
	if err != nil {
		return err
	}
	passParents, err := w.repository.Parents(input.PassReceipt)
	if err != nil {
		return err
	}
	if len(candidateParents) != 1 || candidateParents[0] != input.Candidate ||
		len(passParents) != 1 || passParents[0] != input.CandidateReceipt {
		return fail(
			"INVALID_PASS_AUTHORITY",
			"prepare track base",
			errors.New("PASS authority is not the exact candidate receipt chain"),
		)
	}
	for _, commit := range []OID{
		input.Candidate,
		input.CandidateReceipt,
		input.PassReceipt,
	} {
		product, err := w.repository.ProductTreeIdentity(
			commit,
			request.ProductAdmission,
		)
		if err != nil {
			return err
		}
		if product.ProductTree != input.ProductTree {
			return fail(
				"INVALID_PASS_AUTHORITY",
				"prepare track base",
				errors.New("PASS authority changed product identity"),
			)
		}
	}
	contained, err := w.repository.IsAncestor(
		input.PassReceipt,
		input.SourceHead,
	)
	if err != nil {
		return err
	}
	if !contained {
		return fail(
			"INVALID_PASS_AUTHORITY",
			"prepare track base",
			errors.New("PASS receipt is absent from producer authority"),
		)
	}
	sourceRef := trackHeadRef(input.Producer)
	if previous, present := previousByRef[sourceRef]; present {
		serial, err := w.repository.IsAncestor(previous, input.PassReceipt)
		if err != nil {
			return err
		}
		if !serial {
			return fail(
				"INVALID_TRACK_TOPOLOGY",
				"prepare track base",
				errors.New("same-ref inputs are not plan-ordered ancestry"),
			)
		}
	}
	previousByRef[sourceRef] = input.PassReceipt
	if sourceRef == consumerRef {
		already, err := w.repository.IsAncestor(input.PassReceipt, seed)
		if err != nil {
			return err
		}
		if !already {
			return fail(
				"INVALID_TRACK_TOPOLOGY",
				"prepare track base",
				errors.New("serial same-track input is not already ancestral"),
			)
		}
	}
	return nil
}

func (w *Workspaces) expectedTrackBase(
	request PrepareTrackBaseRequest,
	consumerRef string,
	seed OID,
) (OID, error) {
	if err := w.validatePlanAtRelease(request); err != nil {
		return OID{}, err
	}
	approved, err := w.repository.PrepareApprovedTargetBase(
		CompositionRequest{
			Expected:         seed,
			Candidate:        request.ApprovedTarget,
			TargetRef:        consumerRef,
			ProductAdmission: request.ProductAdmission,
		},
	)
	if err != nil {
		return OID{}, err
	}
	base := approved.Commit
	previousByRef := make(map[string]OID)
	for _, input := range request.Inputs {
		if err := w.validateTrackBaseInput(
			input,
			seed,
			consumerRef,
			previousByRef,
			request,
		); err != nil {
			return OID{}, err
		}
		contained, err := w.repository.IsAncestor(input.PassReceipt, base)
		if err != nil {
			return OID{}, err
		}
		if contained {
			continue
		}
		prepared, err := w.repository.PrepareProductComposition(
			CompositionRequest{
				Expected:         base,
				Candidate:        input.PassReceipt,
				TargetRef:        consumerRef,
				ProductAdmission: request.ProductAdmission,
			},
			func() (OID, error) {
				if request.ResolveProductBase == nil {
					return OID{}, fail(
						"PRODUCT_BASE_RESOLVER_REQUIRED",
						"prepare track base",
						errors.New("conflicting PASS authority requires an engine-derived base"),
					)
				}
				return request.ResolveProductBase(input)
			},
		)
		if err != nil {
			return OID{}, err
		}
		base = prepared.Commit
	}
	return base, nil
}

func expectedTrackBaseRefs(
	request PrepareTrackBaseRequest,
	consumerRef string,
) (map[string]*OID, []string, error) {
	releaseRef := "refs/heads/release-wt/" + request.Release
	expected := map[string]*OID{
		releaseRef:        cloneOptionalOID(&request.ReleaseHead),
		request.TargetRef: cloneOptionalOID(&request.TargetHead),
		consumerRef:       cloneOptionalOID(request.ConsumerBefore),
	}
	for _, input := range request.Inputs {
		sourceRef := trackHeadRef(input.Producer)
		head := input.SourceHead
		if prior, present := expected[sourceRef]; present {
			if prior == nil || *prior != head {
				return nil, nil, fail(
					"INVALID_TRACK_BASE",
					"prepare track base",
					errors.New("one ref has competing expected heads"),
				)
			}
			continue
		}
		expected[sourceRef] = cloneOptionalOID(&head)
	}
	refs := make([]string, 0, len(expected))
	for ref := range expected {
		refs = append(refs, ref)
	}
	return expected, refs, nil
}

func capturedMatchExpected(
	captured []RefHead,
	expected map[string]*OID,
) bool {
	if len(captured) != len(expected) {
		return false
	}
	for _, observed := range captured {
		want, present := expected[observed.Ref]
		if !present {
			return false
		}
		if want == nil {
			if observed.State != RefAbsent {
				return false
			}
			continue
		}
		if observed.State != RefDirect || observed.Head != *want {
			return false
		}
	}
	return true
}

func (w *Workspaces) prepareTrackBaseWithClaim(
	request PrepareTrackBaseRequest,
	beforeUpdate func(PrepareTrackBaseResult) error,
) (result PrepareTrackBaseResult, resultErr error) {
	consumerRef, _, err := w.validateTrackBaseRequest(request)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	writer, err := acquireWorkspaceWriterLock(
		w.repository.commonDir,
		request.Consumer,
	)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	defer func() {
		resultErr = errors.Join(
			resultErr,
			releasePrivateLock(writer, "workspace writer"),
		)
	}()
	expected, refs, err := expectedTrackBaseRefs(request, consumerRef)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	captured, err := w.repository.CaptureHeadRefs(refs)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	if !capturedMatchExpected(captured, expected) {
		return PrepareTrackBaseResult{}, fail(
			"AUTHORITY_MOVED",
			"prepare track base",
			nil,
		)
	}
	result, err = w.ExpectedTrackBase(request)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	if beforeUpdate != nil {
		if err := beforeUpdate(result); err != nil {
			return PrepareTrackBaseResult{}, err
		}
	}
	operations := make([]RefOperation, 0, len(expected))
	for ref, head := range expected {
		if ref == consumerRef && result.Changed {
			continue
		}
		operations = append(operations, RefOperation{
			Kind:     VerifyRef,
			Ref:      ref,
			Expected: cloneOptionalOID(head),
		})
	}
	if result.Changed {
		if request.ConsumerBefore == nil {
			operations = append(operations, RefOperation{
				Kind:    CreateRef,
				Ref:     consumerRef,
				NewHead: cloneOptionalOID(&result.Base),
			})
		} else {
			operations = append(operations, RefOperation{
				Kind:     UpdateRef,
				Ref:      consumerRef,
				NewHead:  cloneOptionalOID(&result.Base),
				Expected: cloneOptionalOID(request.ConsumerBefore),
			})
		}
	}
	if err := w.repository.ApplyRefTransaction(captured, operations); err != nil {
		return PrepareTrackBaseResult{}, err
	}
	return result, nil
}

func (w *Workspaces) ExpectedTrackBase(
	request PrepareTrackBaseRequest,
) (PrepareTrackBaseResult, error) {
	consumerRef, seed, err := w.validateTrackBaseRequest(request)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	base, err := w.expectedTrackBase(request, consumerRef, seed)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	seedTree, err := w.repository.TreeOID(seed)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	baseTree, err := w.repository.TreeOID(base)
	if err != nil {
		return PrepareTrackBaseResult{}, err
	}
	result := PrepareTrackBaseResult{
		Action:         TrackBaseNoop,
		ConsumerRef:    consumerRef,
		ConsumerBefore: cloneOptionalOID(request.ConsumerBefore),
		Seed:           seed,
		SeedTree:       seedTree,
		Base:           base,
		BaseTree:       baseTree,
		Inputs:         cloneTrackBaseInputs(request.Inputs),
	}
	if request.ConsumerBefore == nil {
		result.Changed = true
		result.Action = TrackBaseCreate
	} else if base != *request.ConsumerBefore {
		fastForward, err := w.repository.IsAncestor(
			*request.ConsumerBefore,
			base,
		)
		if err != nil {
			return PrepareTrackBaseResult{}, err
		}
		if !fastForward {
			return PrepareTrackBaseResult{}, fail(
				"CHANGED_OWNER_HEAD",
				"prepare track base",
				errors.New(
					"consumer track moved beyond its authoritative base",
				),
			)
		}
		result.Changed = true
		result.Action = TrackBaseUpdate
	}
	return result, nil
}

func (w *Workspaces) PrepareTrackBase(
	request PrepareTrackBaseRequest,
) (PrepareTrackBaseResult, error) {
	return w.prepareTrackBaseWithClaim(request, nil)
}

func (w *Workspaces) PrepareTrackBaseWithClaim(
	request PrepareTrackBaseRequest,
	beforeUpdate func(PrepareTrackBaseResult) error,
) (PrepareTrackBaseResult, error) {
	return w.prepareTrackBaseWithClaim(request, beforeUpdate)
}

func (w *Workspaces) ReconcileTrackBase(
	request PrepareTrackBaseRequest,
	result PrepareTrackBaseResult,
) (TrackBaseReconciliation, error) {
	consumerRef, seed, err := w.validateTrackBaseRequest(request)
	if err != nil {
		return "", err
	}
	expectedBase, err := w.expectedTrackBase(request, consumerRef, seed)
	if err != nil {
		return "", err
	}
	if result.ConsumerRef != consumerRef ||
		!sameOptionalOID(result.ConsumerBefore, request.ConsumerBefore) ||
		result.Seed != seed ||
		result.Base != expectedBase {
		return TrackBaseAmbiguous, nil
	}
	expected, refs, err := expectedTrackBaseRefs(request, consumerRef)
	if err != nil {
		return "", err
	}
	captured, err := w.repository.CaptureHeadRefs(refs)
	if err != nil {
		return "", err
	}
	allOld := capturedMatchExpected(captured, expected)
	newExpected := make(map[string]*OID, len(expected))
	for ref, head := range expected {
		newExpected[ref] = cloneOptionalOID(head)
	}
	if result.Changed {
		newExpected[consumerRef] = cloneOptionalOID(&result.Base)
	}
	allNew := capturedMatchExpected(captured, newExpected)
	switch {
	case allOld:
		return TrackBaseAllOld, nil
	case allNew:
		return TrackBaseAllNew, nil
	}
	othersCurrent := true
	var consumerHead *OID
	for _, observed := range captured {
		if observed.Ref == consumerRef {
			if observed.State == RefDirect {
				value := observed.Head
				consumerHead = &value
			}
			continue
		}
		want := newExpected[observed.Ref]
		if want == nil || observed.State != RefDirect ||
			observed.Head != *want {
			othersCurrent = false
		}
	}
	if result.Changed && othersCurrent && consumerHead != nil {
		advanced, err := w.repository.IsAncestor(result.Base, *consumerHead)
		if err != nil {
			return "", err
		}
		if advanced {
			return TrackBaseAdvanced, nil
		}
	}
	return TrackBaseAmbiguous, nil
}
