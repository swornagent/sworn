package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type fakeScript struct {
	SchemaVersion string `json:"schema_version"`
	Behavior      string `json:"behavior"`
	Submission    string `json:"submission,omitempty"`
}

type preparedDriverDispatch struct {
	request            driver.Request
	selected           driver.SelectedProfile
	permission         driver.SubmissionPermission
	resumeRequest      *driver.Request
	resumePermission   *driver.SubmissionPermission
	inputs             []driver.InputContent
	inputBody          []byte
	commandPayload     []byte
	productionContext  *productionWorkContext
	expectedDigest     string
	expectedSubmission []byte
	fake               bool
}

type uncertainHandoffPreparationError struct {
	err error
}

const (
	continuationOutcomeReuse           = "reuse"
	continuationOutcomeFallback        = "fallback"
	continuationOutcomeFallbackExpired = "fallback_expired"
)

type continuationDispatchFact struct {
	mode    driver.ContinuationMode
	outcome string
	reason  string
}

type continuationPlanAuthority struct {
	OID      string `json:"oid"`
	Digest   string `json:"digest"`
	Revision int64  `json:"revision"`
}

type continuationTargetAuthority struct {
	TargetRef    string                      `json:"target_ref"`
	TargetHead   string                      `json:"target_head"`
	Track        string                      `json:"track"`
	TrackRef     string                      `json:"track_ref"`
	PreparedBase string                      `json:"prepared_base,omitempty"`
	Evidence     []productionEvidenceBinding `json:"evidence"`
}

type continuationToolAuthority struct {
	SchemaVersion string                `json:"schema_version"`
	Operation     driver.Operation      `json:"operation"`
	Source        continuationToolStage `json:"source"`
	Resume        continuationToolStage `json:"resume"`
}

type continuationToolStage struct {
	Responsibility driver.Responsibility  `json:"responsibility"`
	Workspace      driver.WorkspaceAccess `json:"workspace"`
	FreshContext   bool                   `json:"fresh_context"`
}

type continuationSelectionAuthority struct {
	Profile driver.ProfileConfig   `json:"profile"`
	Adapter driver.AdapterIdentity `json:"adapter"`
	Model   string                 `json:"model"`
}

func continuationPlanDigest(oid, digest string, revision int64) string {
	return driver.Digest(mustJSON(continuationPlanAuthority{
		OID: oid, Digest: digest, Revision: revision,
	}))
}

func continuationTargetDigest(
	targetRef, targetHead, track, trackRef, preparedBase string,
	evidence []productionEvidenceBinding,
) string {
	return driver.Digest(mustJSON(continuationTargetAuthority{
		TargetRef: targetRef, TargetHead: targetHead,
		Track: track, TrackRef: trackRef, PreparedBase: preparedBase,
		Evidence: append([]productionEvidenceBinding(nil), evidence...),
	}))
}

func continuationBindingForDispatch(
	prepared preparedDriverDispatch,
	coordinates dispatchCoordinates,
) (driver.ContinuationBinding, string, error) {
	if prepared.productionContext == nil ||
		prepared.productionContext.Plan == nil ||
		prepared.productionContext.Receipt == nil ||
		coordinates.Slice == "" ||
		coordinates.BatonAttempt < 1 {
		return driver.ContinuationBinding{}, "",
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	expectedRole := driver.RoleImplementer
	source := continuationToolStage{
		Responsibility: driver.ImplementerDesign,
		Workspace:      driver.ReadOnly, FreshContext: true,
	}
	resume := continuationToolStage{
		Responsibility: driver.ImplementerImplementation,
		Workspace:      driver.ReadWrite, FreshContext: false,
	}
	preparedBase := prepared.productionContext.PreparedBase
	if coordinates.Responsibility == driver.WorkVerification {
		expectedRole = driver.RoleVerifier
		preparedBase = ""
		source = continuationToolStage{
			Responsibility: driver.WorkVerification,
			Workspace:      driver.ReadOnly, FreshContext: true,
		}
		resume = continuationToolStage{
			Responsibility: driver.WorkVerification,
			Workspace:      driver.ReadOnly, FreshContext: false,
		}
	} else if coordinates.Responsibility != driver.ImplementerDesign &&
		coordinates.Responsibility != driver.ImplementerImplementation {
		return driver.ContinuationBinding{}, "",
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	if prepared.request.Role != expectedRole {
		return driver.ContinuationBinding{}, "",
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	workContext := prepared.productionContext
	planDigest := continuationPlanDigest(
		workContext.Plan.OID,
		workContext.Plan.Digest,
		workContext.Plan.Revision,
	)
	targetDigest := continuationTargetDigest(
		workContext.Authority.TargetRef,
		workContext.Authority.TargetHead,
		workContext.Track,
		workContext.Authority.TrackRef,
		preparedBase,
		workContext.Evidence,
	)
	toolDigest := driver.Digest(mustJSON(continuationToolAuthority{
		SchemaVersion: "sworn.continuation-tool-transition/v1",
		Operation:     prepared.request.Operation,
		Source:        source,
		Resume:        resume,
	}))
	selectionDigest := driver.Digest(mustJSON(
		continuationSelectionAuthority{
			Profile: prepared.selected.Profile,
			Adapter: prepared.selected.Adapter,
			Model:   prepared.selected.Model,
		},
	))
	return driver.ContinuationBinding{
		RunID:                 workContext.RunID,
		Release:               workContext.Release,
		Slice:                 coordinates.Slice,
		Attempt:               coordinates.BatonAttempt,
		PlanAuthorityDigest:   planDigest,
		TargetAuthorityDigest: targetDigest,
		ToolContractDigest:    toolDigest,
	}, selectionDigest, nil
}

func sameStableContinuationAuthority(
	entry *retainedContinuation,
	binding driver.ContinuationBinding,
	selectionDigest string,
) bool {
	if entry == nil || entry.handle == nil {
		return false
	}
	binding.Attempt = entry.binding.Attempt
	return entry.binding == binding && entry.selectionDigest == selectionDigest
}

func retainedDispatchContinuation(
	handle *driver.Continuation,
	binding driver.ContinuationBinding,
	selectionDigest string,
	before string,
	sourceReceipt string,
) *retainedContinuation {
	return &retainedContinuation{
		handle: handle, binding: binding, before: before,
		selectionDigest: selectionDigest, sourceReceipt: sourceReceipt,
	}
}

func verifierRepairContinuationMatches(
	entry *retainedContinuation,
	freshBinding driver.ContinuationBinding,
	selectionDigest string,
	work *productionWorkContext,
	history baton.SliceHistory,
) bool {
	if entry == nil || entry.handle == nil || entry.verifierFailReceipt == "" ||
		work == nil || work.Receipt == nil || work.Candidate == nil ||
		work.Plan == nil ||
		work.Responsibility != driver.WorkVerification ||
		work.Receipt.OID != work.Candidate.Receipt ||
		work.Authority.TrackHead != work.Receipt.OID ||
		!sameStableContinuationAuthority(
			entry,
			freshBinding,
			selectionDigest,
		) {
		return false
	}
	entries := make(map[string]*baton.ReceiptEntry, len(history.Entries))
	for index := range history.Entries {
		candidate := &history.Entries[index]
		if candidate.OID == "" || entries[candidate.OID] != nil {
			return false
		}
		entries[candidate.OID] = candidate
	}
	fail := entries[entry.verifierFailReceipt]
	current := entries[work.Receipt.OID]
	if fail == nil || current == nil ||
		fail.Receipt.Role != "verifier" ||
		fail.Receipt.Result != "fail" ||
		fail.Receipt.Attempt == nil ||
		*fail.Receipt.Attempt != entry.binding.Attempt ||
		fail.Receipt.Plan != work.Plan.OID ||
		fail.Receipt.SliceID() != work.Slice ||
		current.Receipt.Candidate == nil ||
		*current.Receipt.Candidate != work.Candidate.Commit {
		return false
	}
	expectedAttempt := work.Attempt
	seen := make(map[string]struct{}, len(history.Entries))
	for steps := 0; steps < len(history.Entries); steps++ {
		if current == nil ||
			current.Receipt.Role != "implementer" ||
			current.Receipt.Result != "candidate" ||
			current.Receipt.Attempt == nil ||
			*current.Receipt.Attempt != expectedAttempt ||
			current.Receipt.Plan != work.Plan.OID ||
			current.Receipt.SliceID() != work.Slice {
			return false
		}
		if _, duplicate := seen[current.OID]; duplicate {
			return false
		}
		seen[current.OID] = struct{}{}
		if current.Receipt.Binds == fail.OID {
			return expectedAttempt == *fail.Receipt.Attempt+1
		}
		expectedAttempt--
		if expectedAttempt <= *fail.Receipt.Attempt {
			return false
		}
		current = entries[current.Receipt.Binds]
	}
	return false
}

func continuationEventKind(
	base string,
	fact *continuationDispatchFact,
) (string, error) {
	if fact == nil {
		return base, nil
	}
	if !validContinuationMode(fact.mode) ||
		!validContinuationOutcome(fact.outcome) {
		return "", runtimeFail("INVALID_CONTINUATION", nil)
	}
	return base + ".continuation." + string(fact.mode) + "." +
		fact.outcome, nil
}

func validContinuationMode(mode driver.ContinuationMode) bool {
	switch mode {
	case driver.ContinuationModeFreshRehydrate,
		driver.ContinuationModeTranscriptReplay,
		driver.ContinuationModeOpaqueReplay,
		driver.ContinuationModeProviderCursor,
		driver.ContinuationModeNativeSession,
		driver.ContinuationModeCompacted:
		return true
	default:
		return false
	}
}

func validRetainedContinuationMode(mode driver.ContinuationMode) bool {
	return mode != driver.ContinuationModeFreshRehydrate &&
		validContinuationMode(mode)
}

func validContinuationOutcome(outcome string) bool {
	switch outcome {
	case continuationOutcomeReuse,
		continuationOutcomeFallback,
		continuationOutcomeFallbackExpired:
		return true
	default:
		return false
	}
}

func zeroDriverObservation(observation driver.Observation) bool {
	return observation.TransportStatus == "" &&
		observation.DurationMillis == 0 &&
		observation.Usage == (driver.UsageReceipt{}) &&
		observation.Diagnostic == (driver.Diagnostic{}) &&
		observation.Handoff == nil &&
		len(observation.Events) == 0
}

func requestsFreshRehydrate(
	observation driver.Observation,
	result driver.ContinuationResult,
	err error,
) bool {
	if err != nil ||
		result.Mode != driver.ContinuationModeFreshRehydrate ||
		!zeroDriverObservation(observation) {
		return false
	}
	switch result.Status {
	case driver.ContinuationStatusUnsupported,
		driver.ContinuationStatusClosed,
		driver.ContinuationStatusMismatch,
		driver.ContinuationStatusExpired,
		driver.ContinuationStatusOverflow:
		return true
	default:
		return false
	}
}

func validFreshContinuationStart(
	handle *driver.Continuation,
	result driver.ContinuationResult,
) bool {
	if handle != nil ||
		result.Mode != driver.ContinuationModeFreshRehydrate {
		return false
	}
	switch result.Status {
	case driver.ContinuationStatusUnsupported,
		driver.ContinuationStatusCompleted,
		driver.ContinuationStatusCancelled,
		driver.ContinuationStatusOverflow:
		return true
	default:
		return false
	}
}

func validRetainedContinuationResume(
	handle *driver.Continuation,
	result driver.ContinuationResult,
) bool {
	if handle != nil || !validRetainedContinuationMode(result.Mode) {
		return false
	}
	switch result.Status {
	case driver.ContinuationStatusResumed,
		driver.ContinuationStatusCancelled:
		return true
	default:
		return false
	}
}

func runtimeContinuationError(err error) error {
	if driver.IsCode(err, "CONTINUATION_CLEANUP_FAILED") {
		return runtimeFail("CONTINUATION_CLEANUP_FAILED", err)
	}
	return err
}

func preparedInvocation(
	prepared preparedDriverDispatch,
	workspace *gitx.WorkspaceLease,
	request driver.Request,
	permission driver.SubmissionPermission,
	maskNames []string,
) driver.Invocation {
	fakeProfile := driver.FakeProfile("")
	if prepared.fake {
		fakeProfile = driver.FakeCompleted
	}
	return driver.Invocation{
		Request:       request,
		HostWorkspace: workspace.Path(),
		Selected:      prepared.selected,
		Permission:    permission,
		Inputs:        prepared.inputs,
		FakeProfile:   fakeProfile,
		MaskNames:     append([]string(nil), maskNames...),
	}
}

func preparedResumeInvocation(
	prepared preparedDriverDispatch,
	workspace *gitx.WorkspaceLease,
	hook driver.RecoveryStepHook,
	maskNames []string,
) *driver.Invocation {
	if prepared.resumeRequest == nil || prepared.resumePermission == nil {
		return nil
	}
	invocation := preparedInvocation(
		prepared, workspace, *prepared.resumeRequest,
		*prepared.resumePermission, maskNames,
	)
	invocation.RecoveryStepHook = hook
	return &invocation
}

func currentPreparedProductionBody(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	prepared preparedDriverDispatch,
) ([]byte, error) {
	if prepared.productionContext == nil {
		return nil, runtimeFail("INVALID_CONTINUATION", nil)
	}
	current, _, err := captureProductionWorkContext(
		ctx,
		engine,
		coordinates,
		before,
		prepared.request.Workspace.Access,
	)
	if err != nil {
		return nil, err
	}
	if prepared.productionContext.SchemaVersion ==
		productionWorkContextVersionV1 {
		current, err = productionWorkContextV1(
			engine.manifest,
			current,
		)
		if err != nil {
			return nil, err
		}
	}
	if current.Slice != "" {
		// Release metadata is shared by every track. Progress on another
		// track must not invalidate this lane's otherwise exact authority.
		current.Authority.ReleaseHead =
			prepared.productionContext.Authority.ReleaseHead
	}
	return mustJSON(current), nil
}

func revalidatePreparedProductionDispatch(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	prepared preparedDriverDispatch,
) error {
	currentBody, err := currentPreparedProductionBody(
		ctx,
		engine,
		coordinates,
		before,
		prepared,
	)
	if err != nil {
		return err
	}
	if !bytes.Equal(currentBody, prepared.inputBody) {
		return runtimeFail("STALE_DISPATCH", nil)
	}
	return nil
}

func (s *Service) invokeFreshRecoverableDriver(
	ctx context.Context,
	invocation driver.Invocation,
	prepared preparedDriverDispatch,
	before string,
	recovery turnRecoveryCycle,
) (
	driver.Observation,
	*retainedContinuation,
	error,
) {
	recoverable, supported := s.dispatcher.(driver.RecoverableTurnDriver)
	if !supported {
		observation, err := s.dispatcher.Invoke(ctx, invocation)
		return observation, nil, err
	}
	binding, selectionDigest, err :=
		recoverableContinuationBinding(prepared, recovery)
	if err != nil {
		return driver.Observation{}, nil, err
	}
	observation, handle, result, invokeErr :=
		recoverable.InvokeRecoverableTurn(
			ctx,
			invocation,
			binding,
			nil,
			nil,
		)
	invokeErr = runtimeContinuationError(invokeErr)
	if invokeErr != nil {
		if cleanupErr := closeRetainedContinuation(
			&retainedContinuation{handle: handle},
		); cleanupErr != nil {
			return driver.Observation{}, nil, cleanupErr
		}
		return observation, nil, invokeErr
	}
	if observation.Yield != nil {
		switch {
		case handle != nil &&
			result.Status == driver.ContinuationStatusSuspended &&
			validRetainedContinuationMode(result.Mode):
			return observation, retainedRecoverableContinuation(
				handle,
				binding,
				selectionDigest,
				before,
			), nil
		case handle == nil && validFreshContinuationStart(handle, result):
			return observation, nil, nil
		default:
			if cleanupErr := closeRetainedContinuation(
				&retainedContinuation{handle: handle},
			); cleanupErr != nil {
				return driver.Observation{}, nil, cleanupErr
			}
			return driver.Observation{}, nil,
				runtimeFail("INVALID_CONTINUATION", nil)
		}
	}
	if observation.Handoff == nil ||
		!validFreshContinuationStart(handle, result) {
		if cleanupErr := closeRetainedContinuation(
			&retainedContinuation{handle: handle},
		); cleanupErr != nil {
			return driver.Observation{}, nil, cleanupErr
		}
		return driver.Observation{}, nil,
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	return observation, nil, nil
}

func (s *Service) startPreparedContinuation(
	ctx context.Context,
	invocation driver.Invocation,
	binding driver.ContinuationBinding,
	continuationDriver driver.ContinuationDriver,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	error,
) {
	observation, handle, result, invokeErr :=
		continuationDriver.InvokeTurn(ctx, invocation, binding, nil)
	invokeErr = runtimeContinuationError(invokeErr)
	valid := handle == nil &&
		validFreshContinuationStart(handle, result)
	valid = valid || (handle != nil && invokeErr == nil &&
		result.Status == driver.ContinuationStatusSuspended &&
		validRetainedContinuationMode(result.Mode))
	if valid {
		return observation, handle, result, invokeErr
	}
	if cleanupErr := closeRetainedContinuation(
		&retainedContinuation{handle: handle},
	); cleanupErr != nil {
		return driver.Observation{}, nil, result, cleanupErr
	}
	return observation, nil, result, errors.Join(
		invokeErr, runtimeFail("INVALID_CONTINUATION", nil),
	)
}

func resumePreparedContinuation(
	ctx context.Context,
	continuationDriver driver.ContinuationDriver,
	invocation driver.Invocation,
	binding driver.ContinuationBinding,
	entry *retainedContinuation,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	bool,
	error,
) {
	if entry == nil || entry.handle == nil {
		return driver.Observation{}, nil, driver.ContinuationResult{},
			false, runtimeFail("INVALID_CONTINUATION", nil)
	}
	source := entry.handle
	entry.handle = nil
	observation, next, result, invokeErr :=
		continuationDriver.InvokeTurn(
			ctx,
			invocation,
			binding,
			source,
		)
	sourceCloseErr := source.Close()
	invokeErr = runtimeContinuationError(invokeErr)
	if sourceCloseErr != nil {
		if next != nil {
			sourceCloseErr = errors.Join(sourceCloseErr, next.Close())
		}
		return driver.Observation{}, nil, result, false,
			runtimeFail(
				"CONTINUATION_CLEANUP_FAILED",
				sourceCloseErr,
			)
	}
	if requestsFreshRehydrate(observation, result, invokeErr) {
		if next != nil {
			if cleanupErr := next.Close(); cleanupErr != nil {
				return driver.Observation{}, nil, result, false,
					runtimeFail(
						"CONTINUATION_CLEANUP_FAILED",
						cleanupErr,
					)
			}
			return driver.Observation{}, nil, result, false,
				runtimeFail("INVALID_CONTINUATION", nil)
		}
		return observation, nil, result, true, nil
	}
	if invokeErr != nil {
		if cleanupErr := closeRetainedContinuation(
			&retainedContinuation{handle: next},
		); cleanupErr != nil {
			return driver.Observation{}, nil, result, false, cleanupErr
		}
		return observation, nil, result, false, invokeErr
	}
	if next != nil {
		if result.Status != driver.ContinuationStatusSuspended ||
			!validRetainedContinuationMode(result.Mode) {
			if cleanupErr := closeRetainedContinuation(
				&retainedContinuation{handle: next},
			); cleanupErr != nil {
				return driver.Observation{}, nil, result, false, cleanupErr
			}
			return driver.Observation{}, nil, result, false,
				runtimeFail("INVALID_CONTINUATION", nil)
		}
	} else if !validRetainedContinuationResume(next, result) {
		return driver.Observation{}, nil, result, false,
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	return observation, next, result, false, nil
}

type continuationTurnPolicy struct {
	retainFresh       bool
	missingIsFallback bool
}

func (s *Service) invokeContinuationTurn(
	ctx context.Context,
	continuationDriver driver.ContinuationDriver,
	supported bool,
	freshInvocation driver.Invocation,
	resumeInvocation *driver.Invocation,
	binding driver.ContinuationBinding,
	entry *retainedContinuation,
	matches bool,
	revalidate func() error,
	policy continuationTurnPolicy,
) (
	driver.Observation,
	*driver.Continuation,
	*continuationDispatchFact,
	error,
) {
	start := func() (
		driver.Observation,
		*driver.Continuation,
		driver.ContinuationResult,
		error,
	) {
		if policy.retainFresh && supported {
			return s.startPreparedContinuation(
				ctx, freshInvocation, binding,
				continuationDriver,
			)
		}
		observation, err := s.dispatcher.Invoke(ctx, freshInvocation)
		return observation, nil, driver.ContinuationResult{
			Mode:   driver.ContinuationModeFreshRehydrate,
			Status: driver.ContinuationStatusCompleted,
		}, err
	}
	fallback := entry != nil || policy.missingIsFallback
	if !matches || !supported || resumeInvocation == nil {
		if err := closeRetainedContinuation(entry); err != nil {
			return driver.Observation{}, nil, nil, err
		}
		observation, next, _, err := start()
		if !fallback {
			return observation, next, nil, err
		}
		return observation, next, &continuationDispatchFact{
			mode:    driver.ContinuationModeFreshRehydrate,
			outcome: continuationOutcomeFallback,
			reason:  "absence",
		}, err
	}
	observation, next, result, wantsFresh, err :=
		resumePreparedContinuation(
			ctx, continuationDriver, *resumeInvocation,
			binding, entry,
		)
	if err != nil || !wantsFresh {
		if err != nil {
			return observation, nil, nil, err
		}
		return observation, next, &continuationDispatchFact{
			mode: result.Mode, outcome: continuationOutcomeReuse,
		}, nil
	}
	if err := revalidate(); err != nil {
		return driver.Observation{}, nil, nil, err
	}
	outcome := continuationOutcomeFallback
	reason := result.Reason
	if result.Status == driver.ContinuationStatusExpired {
		outcome = continuationOutcomeFallbackExpired
		if reason == "" {
			reason = "expiry"
		}
	}
	if reason == "" {
		reason = "absence"
	}
	observation, next, _, err = start()
	return observation, next, &continuationDispatchFact{
		mode:    driver.ContinuationModeFreshRehydrate,
		outcome: outcome,
		reason:  reason,
	}, err
}

func (s *Service) invokePreparedDriver(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	coordinates dispatchCoordinates,
	before string,
	prepared preparedDriverDispatch,
	owner journal.OwnerLease,
	recovery *turnRecoveryCycle,
) (
	driver.Observation,
	*retainedContinuation,
	*continuationDispatchFact,
	error,
) {
	invocation := preparedInvocation(
		prepared,
		workspace,
		prepared.request,
		prepared.permission,
		engine.repository.ReservedNames(),
	)
	if recovery != nil {
		invocation.RecoveryStepHook =
			s.turnRecoveryStepHook(owner, recovery)
	}
	if (prepared.fake || prepared.productionContext == nil) &&
		recovery != nil {
		observation, pending, err := s.invokeFreshRecoverableDriver(
			ctx,
			invocation,
			prepared,
			before,
			*recovery,
		)
		return observation, pending, nil, err
	}
	if prepared.fake || prepared.productionContext == nil {
		observation, err := s.dispatcher.Invoke(ctx, invocation)
		return observation, nil, nil, err
	}
	continuationDriver, supported :=
		s.dispatcher.(driver.ContinuationDriver)
	switch coordinates.Responsibility {
	case driver.ImplementerDesign:
		if !supported ||
			prepared.productionContext.SchemaVersion !=
				productionWorkContextVersion {
			observation, err := s.dispatcher.Invoke(ctx, invocation)
			return observation, nil, nil, err
		}
		binding, selectionDigest, err :=
			continuationBindingForDispatch(prepared, coordinates)
		if err != nil {
			if cleanupErr := s.discardContinuation(
				prepared.productionContext.RunID,
				coordinates.Slice,
			); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			return driver.Observation{}, nil, nil, err
		}
		observation, handle, _, invokeErr :=
			s.startPreparedContinuation(
				ctx,
				invocation,
				binding,
				continuationDriver,
			)
		if handle == nil {
			return observation, nil, nil, invokeErr
		}
		workContext := prepared.productionContext
		return observation, retainedDispatchContinuation(
			handle, binding, selectionDigest, before,
			workContext.Receipt.OID,
		), nil, nil
	case driver.ImplementerImplementation:
		if err := revalidatePreparedProductionDispatch(
			ctx,
			engine,
			coordinates,
			before,
			prepared,
		); err != nil {
			if cleanupErr := s.discardContinuation(
				prepared.productionContext.RunID,
				coordinates.Slice,
			); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			return driver.Observation{}, nil, nil, err
		}
		if prepared.productionContext.SchemaVersion ==
			productionWorkContextVersion &&
			prepared.productionContext.DesignReceipt == nil {
			if cleanupErr := s.discardContinuation(
				prepared.productionContext.RunID,
				coordinates.Slice,
			); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			observation, invokeErr := s.dispatcher.Invoke(
				ctx,
				invocation,
			)
			return observation, nil, nil, invokeErr
		}
		binding, selectionDigest, err :=
			continuationBindingForDispatch(prepared, coordinates)
		if err != nil {
			if cleanupErr := s.discardContinuation(
				prepared.productionContext.RunID,
				coordinates.Slice,
			); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			return driver.Observation{}, nil, nil, err
		}
		entry := s.takeContinuation(
			prepared.productionContext.RunID,
			coordinates.Slice,
		)
		matches := entry != nil &&
			entry.handle != nil &&
			entry.designReceipt != "" &&
			prepared.productionContext.DesignReceipt != nil &&
			entry.designReceipt ==
				prepared.productionContext.DesignReceipt.OID &&
			entry.binding == binding &&
			entry.selectionDigest == selectionDigest
		resumeInvocation := preparedResumeInvocation(
			prepared, workspace, invocation.RecoveryStepHook,
			engine.repository.ReservedNames(),
		)
		observation, next, fact, invokeErr :=
			s.invokeContinuationTurn(
				ctx, continuationDriver, supported,
				invocation, resumeInvocation, binding, entry,
				matches,
				func() error {
					return revalidatePreparedProductionDispatch(
						ctx, engine, coordinates, before, prepared,
					)
				},
				continuationTurnPolicy{missingIsFallback: true},
			)
		if invokeErr != nil {
			return observation, nil, fact, invokeErr
		}
		if observation.Yield != nil {
			if fact == nil ||
				fact.outcome != continuationOutcomeReuse {
				return observation, nil, fact, nil
			}
			if next == nil {
				return observation, nil, nil,
					runtimeFail("INVALID_CONTINUATION", nil)
			}
			return observation, retainedRecoverableContinuation(
				next,
				binding,
				selectionDigest,
				before,
			), fact, nil
		}
		if next != nil {
			if cleanupErr := closeRetainedContinuation(
				&retainedContinuation{handle: next},
			); cleanupErr != nil {
				return observation, nil, nil, cleanupErr
			}
			return observation, nil, nil,
				runtimeFail("INVALID_CONTINUATION", nil)
		}
		return observation, nil, fact, nil
	case driver.WorkVerification:
		if err := revalidatePreparedProductionDispatch(
			ctx,
			engine,
			coordinates,
			before,
			prepared,
		); err != nil {
			if cleanupErr := s.discardRetainedContinuation(
				prepared.productionContext.RunID,
				continuationVerifier,
				coordinates.Slice,
			); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			return driver.Observation{}, nil, nil, err
		}
		freshBinding, selectionDigest, err :=
			continuationBindingForDispatch(prepared, coordinates)
		if err != nil {
			if cleanupErr := s.discardRetainedContinuation(
				prepared.productionContext.RunID,
				continuationVerifier,
				coordinates.Slice,
			); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			return driver.Observation{}, nil, nil, err
		}
		entry := s.takeRetainedContinuation(
			prepared.productionContext.RunID,
			continuationVerifier,
			coordinates.Slice,
		)
		var history baton.SliceHistory
		if entry != nil && entry.verifierFailReceipt != "" {
			state, stateErr := baton.ReadState(
				engine.git,
				engine.manifest.value.Release,
				engine.inertness,
			)
			current, found := state.Slice(coordinates.Slice)
			if stateErr != nil || !found {
				restoreErr := s.storeRetainedContinuation(
					prepared.productionContext.RunID,
					continuationVerifier,
					coordinates.Slice,
					entry,
				)
				if stateErr == nil {
					stateErr = runtimeFail("STALE_DISPATCH", nil)
				} else {
					stateErr = runtimeFail("BATON_UNAVAILABLE", stateErr)
				}
				return driver.Observation{}, nil, nil,
					errors.Join(stateErr, restoreErr)
			}
			history = current.History
		}
		matches := verifierRepairContinuationMatches(
			entry,
			freshBinding,
			selectionDigest,
			prepared.productionContext,
			history,
		)
		resumeInvocation := preparedResumeInvocation(
			prepared, workspace, invocation.RecoveryStepHook,
			engine.repository.ReservedNames(),
		)
		observation, next, fact, invokeErr :=
			s.invokeContinuationTurn(
				ctx, continuationDriver, supported,
				invocation, resumeInvocation, freshBinding, entry,
				matches,
				func() error {
					return revalidatePreparedProductionDispatch(
						ctx, engine, coordinates, before, prepared,
					)
				},
				continuationTurnPolicy{
					retainFresh: true, missingIsFallback: prepared.productionContext.Attempt > 1,
				},
			)
		if invokeErr != nil || next == nil {
			return observation, nil, fact, invokeErr
		}
		return observation, retainedDispatchContinuation(
			next, freshBinding, selectionDigest, before,
			prepared.productionContext.Receipt.OID,
		), fact, nil
	default:
		if recovery != nil {
			observation, pending, err :=
				s.invokeFreshRecoverableDriver(
					ctx,
					invocation,
					prepared,
					before,
					*recovery,
				)
			return observation, pending, nil, err
		}
		observation, err := s.dispatcher.Invoke(ctx, invocation)
		return observation, nil, nil, err
	}
}

func (e *uncertainHandoffPreparationError) Error() string {
	return e.err.Error()
}

func (e *uncertainHandoffPreparationError) Unwrap() error {
	return e.err
}

func uncertainHandoffPreparation(err error) error {
	if err == nil {
		return nil
	}
	return &uncertainHandoffPreparationError{err: err}
}

func (s *Service) prepareDriverDispatch(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	role driver.Role,
	coordinates dispatchCoordinates,
	before string,
) (preparedDriverDispatch, error) {
	if workspace == nil || workspace.Path() == "" {
		return preparedDriverDispatch{}, runtimeFail("INVALID_WORKSPACE", nil)
	}
	expectedRole, ok := roleForResponsibility(coordinates.Responsibility)
	if !ok || expectedRole != role {
		return preparedDriverDispatch{},
			runtimeFail("INVALID_PRODUCTION_DISPATCH", nil)
	}
	selected, err := engine.registry.Resolve(engine.manifest.value.Roles, role)
	if err != nil {
		return preparedDriverDispatch{},
			runtimeFail("DRIVER_SELECTION_FAILED", err)
	}
	if err := engine.configured.validateSelected(selected); err != nil {
		return preparedDriverDispatch{}, err
	}
	access := driver.ReadOnly
	containment := driver.ContainmentReadOnly
	if workspace.Access() == gitx.WorkspaceReadWrite {
		access = driver.ReadWrite
		containment = driver.ContainmentReadWrite
	}
	prepared := preparedDriverDispatch{
		selected: selected,
		fake:     !engine.manifest.value.production(),
	}
	if prepared.fake {
		script, found := engine.manifest.value.script(
			coordinates.Slice,
			coordinates.Responsibility,
			coordinates.BatonAttempt,
			coordinates.Epoch,
			coordinates.Try,
		)
		if !found {
			return preparedDriverDispatch{},
				runtimeFail("SCRIPT_NOT_FOUND", nil)
		}
		if script.Submission != "" {
			prepared.expectedSubmission, err =
				base64.StdEncoding.Strict().DecodeString(script.Submission)
			if err != nil {
				return preparedDriverDispatch{},
					runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
			}
		}
		prepared.inputBody = mustJSON(fakeScript{
			SchemaVersion: "sworn.fake-script/v1",
			Behavior:      script.Behavior,
			Submission:    script.Submission,
		})
		input := driver.Input{
			Name: "fake-script", Path: "runtime/fake-script.json",
			Digest: driver.Digest(prepared.inputBody),
		}
		prepared.inputs = []driver.InputContent{{
			Input: input,
			Bytes: prepared.inputBody,
		}}
		prepared.commandPayload = prepared.inputBody
		prepared.expectedDigest =
			sha256Digest(prepared.expectedSubmission)
	} else {
		workContext, body, captureErr := captureProductionWorkContext(
			ctx,
			engine,
			coordinates,
			before,
			access,
		)
		if captureErr != nil {
			return preparedDriverDispatch{}, captureErr
		}
		prepared.inputBody = body
		prepared.inputs, err = productionInputContents(workContext, body)
		if err != nil {
			return preparedDriverDispatch{}, err
		}
		prepared.productionContext = &workContext
		prepared.expectedDigest = productionOutputExpectation
	}
	if prepared.fake {
		requestInputs := make([]driver.Input, len(prepared.inputs))
		for index := range prepared.inputs {
			requestInputs[index] = prepared.inputs[index].Input
		}
		prepared.request, err = driver.NewRequest(
			dispatchInvocationID(engine.manifest.value.RunID, coordinates),
			role,
			selected.Profile.Key,
			selected.Model,
			driver.Workspace{Path: driver.GuestWorkspacePath, Access: access},
			requestInputs,
			true,
			engine.manifest.value.Limits,
		)
		if err != nil {
			return preparedDriverDispatch{},
				runtimeFail("DRIVER_REQUEST_FAILED", err)
		}
	} else {
		prepared.request, err = productionRequestForContext(
			engine.manifest,
			*prepared.productionContext,
		)
		if err != nil {
			return preparedDriverDispatch{}, err
		}
	}
	prepared.permission, err = driver.NewSubmissionPermission(
		prepared.request,
		selected,
		containment,
		coordinates.Responsibility,
	)
	if err != nil {
		return preparedDriverDispatch{},
			runtimeFail("DRIVER_REQUEST_FAILED", err)
	}
	if !prepared.fake {
		requestBody, encodeErr := driver.EncodeRequest(prepared.request)
		if encodeErr != nil {
			return preparedDriverDispatch{},
				runtimeFail("DRIVER_REQUEST_FAILED", encodeErr)
		}
		command := productionDispatchCommand{
			SchemaVersion: productionDispatchVersion,
			RequestDigest: driver.Digest(requestBody),
			Context:       *prepared.productionContext,
		}
		if hasContinuationResumeRequest(*prepared.productionContext) {
			resumeRequest, requestErr :=
				productionRequestForContextFreshness(
					engine.manifest,
					*prepared.productionContext,
					false,
				)
			if requestErr != nil {
				return preparedDriverDispatch{}, requestErr
			}
			resumeContainment := driver.ContainmentReadOnly
			if resumeRequest.Workspace.Access == driver.ReadWrite {
				resumeContainment = driver.ContainmentReadWrite
			}
			resumePermission, permissionErr :=
				driver.NewSubmissionPermission(
					resumeRequest,
					selected,
					resumeContainment,
					coordinates.Responsibility,
				)
			if permissionErr != nil {
				return preparedDriverDispatch{},
					runtimeFail(
						"DRIVER_REQUEST_FAILED",
						permissionErr,
					)
			}
			resumeBody, resumeErr :=
				driver.EncodeRequest(resumeRequest)
			if resumeErr != nil {
				return preparedDriverDispatch{},
					runtimeFail(
						"DRIVER_REQUEST_FAILED",
						resumeErr,
					)
			}
			prepared.resumeRequest = &resumeRequest
			prepared.resumePermission = &resumePermission
			command.ResumeRequestDigest = driver.Digest(resumeBody)
		}
		prepared.commandPayload = mustJSON(command)
	}
	return prepared, nil
}

func downgradePreparedProductionDispatchV1(
	manifest admittedManifest,
	prepared preparedDriverDispatch,
) (preparedDriverDispatch, error) {
	if prepared.fake || prepared.productionContext == nil {
		return preparedDriverDispatch{},
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	workContext, err := productionWorkContextV1(
		manifest,
		*prepared.productionContext,
	)
	if err != nil {
		return preparedDriverDispatch{}, err
	}
	body := mustJSON(workContext)
	inputs, err := productionInputContents(workContext, body)
	if err != nil {
		return preparedDriverDispatch{}, err
	}
	request, err := productionRequestForContext(manifest, workContext)
	if err != nil {
		return preparedDriverDispatch{}, err
	}
	containment := driver.ContainmentReadOnly
	if request.Workspace.Access == driver.ReadWrite {
		containment = driver.ContainmentReadWrite
	}
	permission, err := driver.NewSubmissionPermission(
		request,
		prepared.selected,
		containment,
		workContext.Responsibility,
	)
	if err != nil {
		return preparedDriverDispatch{},
			runtimeFail("DRIVER_REQUEST_FAILED", err)
	}
	requestBody, err := driver.EncodeRequest(request)
	if err != nil {
		return preparedDriverDispatch{},
			runtimeFail("DRIVER_REQUEST_FAILED", err)
	}
	prepared.request = request
	prepared.permission = permission
	prepared.resumeRequest = nil
	prepared.resumePermission = nil
	prepared.inputs = inputs
	prepared.inputBody = body
	prepared.productionContext = &workContext
	prepared.commandPayload = mustJSON(productionDispatchCommand{
		SchemaVersion: productionDispatchVersionV1,
		RequestDigest: driver.Digest(requestBody),
		Context:       workContext,
	})
	return prepared, nil
}

func restorePreparedProductionDispatch(
	manifest admittedManifest,
	prepared preparedDriverDispatch,
	command productionDispatchCommand,
) (preparedDriverDispatch, error) {
	if prepared.productionContext == nil {
		return preparedDriverDispatch{},
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	workContext, err := rehydrateProductionContextInputs(
		command.Context,
		*prepared.productionContext,
	)
	if err != nil {
		return preparedDriverDispatch{}, err
	}
	body := mustJSON(workContext)
	inputs, err := productionInputContents(workContext, body)
	if err != nil {
		return preparedDriverDispatch{}, err
	}
	request, err := productionRequestForContext(manifest, workContext)
	if err != nil {
		return preparedDriverDispatch{}, err
	}
	containment := driver.ContainmentReadOnly
	if request.Workspace.Access == driver.ReadWrite {
		containment = driver.ContainmentReadWrite
	}
	permission, err := driver.NewSubmissionPermission(
		request,
		prepared.selected,
		containment,
		workContext.Responsibility,
	)
	if err != nil {
		return preparedDriverDispatch{},
			runtimeFail("DRIVER_REQUEST_FAILED", err)
	}
	prepared.request = request
	prepared.permission = permission
	prepared.resumeRequest = nil
	prepared.resumePermission = nil
	prepared.inputs = inputs
	prepared.inputBody = body
	prepared.productionContext = &workContext
	prepared.commandPayload = mustJSON(command)
	if command.ResumeRequestDigest != "" {
		resume, requestErr := productionRequestForContextFreshness(
			manifest,
			workContext,
			false,
		)
		if requestErr != nil {
			return preparedDriverDispatch{}, requestErr
		}
		resumeContainment := driver.ContainmentReadOnly
		if resume.Workspace.Access == driver.ReadWrite {
			resumeContainment = driver.ContainmentReadWrite
		}
		resumePermission, permissionErr := driver.NewSubmissionPermission(
			resume,
			prepared.selected,
			resumeContainment,
			workContext.Responsibility,
		)
		if permissionErr != nil {
			return preparedDriverDispatch{},
				runtimeFail("DRIVER_REQUEST_FAILED", permissionErr)
		}
		prepared.resumeRequest = &resume
		prepared.resumePermission = &resumePermission
	}
	return prepared, nil
}

func rehydrateProductionContextInputs(
	persisted productionWorkContext,
	current productionWorkContext,
) (productionWorkContext, error) {
	if persisted.Plan != nil {
		if current.Plan == nil ||
			current.Plan.Input != persisted.Plan.Input {
			return productionWorkContext{},
				runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		binding := *persisted.Plan
		binding.body = append([]byte(nil), current.Plan.body...)
		persisted.Plan = &binding
	}
	if persisted.Receipt != nil {
		if current.Receipt == nil ||
			current.Receipt.BodyInput != persisted.Receipt.BodyInput ||
			current.Receipt.DetailInput != persisted.Receipt.DetailInput {
			return productionWorkContext{},
				runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		binding := *persisted.Receipt
		binding.body = append([]byte(nil), current.Receipt.body...)
		binding.detail = append([]byte(nil), current.Receipt.detail...)
		persisted.Receipt = &binding
	}
	if persisted.DesignReceipt != nil {
		if current.DesignReceipt == nil ||
			current.DesignReceipt.BodyInput !=
				persisted.DesignReceipt.BodyInput ||
			current.DesignReceipt.DetailInput !=
				persisted.DesignReceipt.DetailInput {
			return productionWorkContext{},
				runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		binding := *persisted.DesignReceipt
		binding.body = append(
			[]byte(nil),
			current.DesignReceipt.body...,
		)
		binding.detail = append(
			[]byte(nil),
			current.DesignReceipt.detail...,
		)
		persisted.DesignReceipt = &binding
	}
	if persisted.HostEvidence != nil {
		if current.HostEvidence == nil ||
			current.HostEvidence.Input != persisted.HostEvidence.Input {
			return productionWorkContext{},
				runtimeFail("INVALID_AUTHORITY_STATE", nil)
		}
		binding := *persisted.HostEvidence
		binding.body = append(
			[]byte(nil),
			current.HostEvidence.body...,
		)
		persisted.HostEvidence = &binding
	}
	return persisted, nil
}

func (s *Service) persistedDriverCommand(
	ctx context.Context,
	runID string,
	replayKey string,
) (journal.Command, bool, error) {
	if _, err := s.journal.Effect(ctx, runID, replayKey); err != nil {
		if journal.IsCode(err, "EFFECT_NOT_FOUND") {
			return journal.Command{}, false, nil
		}
		return journal.Command{}, false,
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return journal.Command{}, false,
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	var found *journal.Command
	for index := range snapshot.Commands {
		command := &snapshot.Commands[index]
		if command.ReplayKey != replayKey {
			continue
		}
		if found != nil {
			return journal.Command{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		found = command
	}
	if found == nil || found.Kind != "driver.dispatch" {
		return journal.Command{}, false,
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return *found, true, nil
}

const humanHandoffCheckpointVersion = "sworn.human-handoff-checkpoint/v1"

type humanHandoffCheckpoint struct {
	SchemaVersion       string                   `json:"schema_version"`
	ParentEffect        string                   `json:"parent_effect"`
	AttentionID         string                   `json:"attention_id"`
	AttentionGeneration int64                    `json:"attention_generation"`
	AnswerDigest        string                   `json:"answer_digest"`
	HumanTurn           journal.HumanTurnBinding `json:"human_turn"`
	Observation         driver.Observation       `json:"observation"`
}

func humanHandoffCheckpointID(parentEffect string) string {
	return parentEffect + "/human-handoff"
}

func expectedHumanHandoffCheckpoint(
	parentEffect string,
	attention journal.AttentionProjection,
	observation driver.Observation,
) (humanHandoffCheckpoint, error) {
	if attention.Attention.HumanTurn == nil ||
		attention.State != journal.AttentionAnswered ||
		attention.Generation != 2 || observation.Handoff == nil ||
		observation.Yield != nil ||
		driver.Digest(observation.Handoff.SubmissionBytes) !=
			observation.Handoff.SubmissionDigest {
		return humanHandoffCheckpoint{},
			runtimeFail("INVALID_HUMAN_HANDOFF", nil)
	}
	return humanHandoffCheckpoint{
		SchemaVersion:       humanHandoffCheckpointVersion,
		ParentEffect:        parentEffect,
		AttentionID:         attention.Attention.ID,
		AttentionGeneration: attention.Generation,
		AnswerDigest:        driver.Digest([]byte(attention.Answer)),
		HumanTurn:           *attention.Attention.HumanTurn,
		Observation:         observation,
	}, nil
}

func (s *Service) loadHumanHandoffCheckpoint(
	ctx context.Context,
	runID string,
	parentEffect string,
	attention journal.AttentionProjection,
) (driver.Observation, bool, error) {
	id := humanHandoffCheckpointID(parentEffect)
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return driver.Observation{}, false,
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	var command *journal.Command
	for index := range snapshot.Commands {
		candidate := &snapshot.Commands[index]
		if candidate.ReplayKey != id {
			continue
		}
		if command != nil {
			return driver.Observation{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		command = candidate
	}
	var effect *journal.Effect
	for index := range snapshot.Effects {
		candidate := &snapshot.Effects[index]
		if candidate.ID != id {
			continue
		}
		if effect != nil {
			return driver.Observation{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effect = candidate
	}
	if command == nil && effect == nil {
		return driver.Observation{}, false, nil
	}
	if command == nil || effect == nil ||
		command.RunID != runID || command.Kind != "driver.handoff" ||
		effect.RunID != runID || effect.ReplayKey != id ||
		effect.Kind != command.Kind || effect.State != journal.Succeeded ||
		effect.ResultDigest != sha256Digest(effect.Result) ||
		!bytes.Equal(command.Payload, effect.Result) {
		return driver.Observation{}, false,
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var checkpoint humanHandoffCheckpoint
	if json.Unmarshal(effect.Result, &checkpoint) != nil ||
		!bytes.Equal(effect.Result, mustJSON(checkpoint)) {
		return driver.Observation{}, false,
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	expected, err := expectedHumanHandoffCheckpoint(
		parentEffect,
		attention,
		checkpoint.Observation,
	)
	if err != nil ||
		!bytes.Equal(mustJSON(checkpoint), mustJSON(expected)) {
		return driver.Observation{}, false,
			runtimeFail("CORRUPT_JOURNAL", err)
	}
	return checkpoint.Observation, true, nil
}

func (s *Service) persistHumanHandoffCheckpoint(
	ctx context.Context,
	owner journal.OwnerLease,
	parentEffect string,
	attention journal.AttentionProjection,
	observation driver.Observation,
) error {
	checkpoint, err := expectedHumanHandoffCheckpoint(
		parentEffect,
		attention,
		observation,
	)
	if err != nil {
		return err
	}
	body := mustJSON(checkpoint)
	id := humanHandoffCheckpointID(parentEffect)
	now := s.now().UTC()
	if err := s.journal.RecordCommandEffect(
		ctx,
		journal.Command{
			RunID: owner.RunID, ReplayKey: id,
			Kind: "driver.handoff", Payload: body, CreatedAt: now,
		},
		journal.Effect{
			RunID: owner.RunID, ID: id, ReplayKey: id,
			Kind: "driver.handoff",
			BeforeDigest: driver.Digest(
				mustJSON(checkpoint.HumanTurn),
			),
			ExpectedDigest: sha256Digest(body), UpdatedAt: now,
		},
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	effect, err := s.journal.Effect(ctx, owner.RunID, id)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		_, found, loadErr := s.loadHumanHandoffCheckpoint(
			ctx,
			owner.RunID,
			parentEffect,
			attention,
		)
		if loadErr != nil || !found {
			return runtimeFail("CORRUPT_JOURNAL", loadErr)
		}
		return nil
	}
	if effect.State != journal.Pending {
		return runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	claim, err := s.journal.ClaimOwned(
		ctx,
		owner,
		id,
		now,
		effectLease,
	)
	if err != nil {
		return runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	if err := s.journal.CompleteOwned(
		context.WithoutCancel(ctx),
		owner,
		journal.Completion{
			RunID: owner.RunID, EffectID: id, Token: claim.Token,
			State: journal.Succeeded, Result: body,
			EventKind: "human_turn.handoff_checkpointed",
			EventBody: MarshalAssociation(EventAssociation{
				EffectID: id,
				WorkID:   checkpoint.HumanTurn.WorkIdentity,
				Track:    checkpoint.HumanTurn.Track,
				Slice:    checkpoint.HumanTurn.Slice,
			}), At: now,
		},
	); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func (s *Service) runDriverEffect(ctx context.Context, engine *engine,
	workspace *gitx.WorkspaceLease, role driver.Role, coordinates dispatchCoordinates,
	attemptIdentity journal.EffectAttempt, before string,
	owner journal.OwnerLease) (driver.Submission, error) {
	return s.runDriverEffectWithPreparation(
		ctx,
		engine,
		workspace,
		role,
		coordinates,
		attemptIdentity,
		before,
		owner,
		nil,
	)
}

func (s *Service) runDriverEffectWithPreparation(ctx context.Context, engine *engine,
	workspace *gitx.WorkspaceLease, role driver.Role, coordinates dispatchCoordinates,
	attemptIdentity journal.EffectAttempt, before string,
	owner journal.OwnerLease,
	prepareHandoff func(driver.Submission) error) (
	submissionResult driver.Submission,
	resultErr error,
) {
	manifest := engine.manifest
	replayKey := journal.AttemptEffectID(
		attemptIdentity.WorkID,
		attemptIdentity.Epoch,
		attemptIdentity.Try,
	)
	persistedCommand, persisted, err := s.persistedDriverCommand(
		ctx,
		manifest.value.RunID,
		replayKey,
	)
	if err != nil {
		return driver.Submission{}, err
	}
	prepared, err := s.prepareDriverDispatch(
		ctx,
		engine,
		workspace,
		role,
		coordinates,
		before,
	)
	if err != nil {
		return driver.Submission{}, err
	}
	if persisted && manifest.value.production() {
		prior, parseErr := parseProductionDispatchCommand(
			manifest,
			persistedCommand.Payload,
		)
		if parseErr != nil {
			return driver.Submission{}, parseErr
		}
		currentPrepared := prepared
		if prior.SchemaVersion == productionDispatchVersionV1 {
			currentPrepared, err = downgradePreparedProductionDispatchV1(
				manifest,
				currentPrepared,
			)
			if err != nil {
				return driver.Submission{}, err
			}
		}
		currentBody, currentErr := currentPreparedProductionBody(
			ctx,
			engine,
			coordinates,
			before,
			currentPrepared,
		)
		if currentErr != nil ||
			!bytes.Equal(currentBody, mustJSON(prior.Context)) {
			if currentErr != nil {
				return driver.Submission{}, currentErr
			}
			return driver.Submission{}, runtimeFail("STALE_DISPATCH", nil)
		}
		prepared, err = restorePreparedProductionDispatch(
			manifest,
			prepared,
			prior,
		)
		if err != nil {
			return driver.Submission{}, err
		}
	}
	var recovery *turnRecoveryCycle
	if _, enabled := manifest.value.recoverySelection(); enabled {
		cycle, cycleErr := turnRecoveryCycleForDispatch(
			manifest,
			prepared,
			coordinates,
			attemptIdentity.WorkID,
			before,
		)
		if cycleErr != nil {
			return driver.Submission{}, cycleErr
		}
		recovery = &cycle
	}
	now := s.now().UTC()
	command := journal.Command{RunID: manifest.value.RunID, ReplayKey: replayKey,
		Kind: "driver.dispatch", Payload: prepared.commandPayload, CreatedAt: now}
	effectInput := journal.Effect{RunID: manifest.value.RunID, ID: replayKey,
		ReplayKey: replayKey, Kind: "driver.dispatch",
		BeforeDigest:   sha256Digest([]byte(before)),
		ExpectedDigest: prepared.expectedDigest, UpdatedAt: now}
	if err := s.journal.EnsureAttempt(ctx, command, effectInput, attemptIdentity); err != nil {
		return driver.Submission{}, err
	}
	effect, err := s.journal.Effect(ctx, manifest.value.RunID, replayKey)
	if err != nil {
		return driver.Submission{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		cached, _, err := validateSucceededDriverResult(
			manifest,
			command,
			effect,
		)
		if err != nil ||
			cached.Responsibility != coordinates.Responsibility ||
			cached.InvocationID != prepared.request.InvocationID {
			return driver.Submission{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		if coordinates.Responsibility ==
			driver.ImplementerImplementation {
			if cleanupErr := s.discardContinuation(
				manifest.value.RunID,
				coordinates.Slice,
			); cleanupErr != nil {
				return driver.Submission{}, cleanupErr
			}
		}
		if recovery != nil {
			attention, found, attentionErr := s.attentionForWork(
				ctx,
				manifest.value.RunID,
				attemptIdentity.WorkID,
			)
			if attentionErr != nil {
				return driver.Submission{}, attentionErr
			}
			if found {
				if attention.Attention.Recovery.CycleID !=
					recovery.binding.CycleID ||
					attention.Attention.Recovery.LaneID !=
						recovery.binding.LaneID ||
					attention.State != journal.AttentionAnswered {
					return driver.Submission{},
						runtimeFail("CORRUPT_JOURNAL", nil)
				}
				if err := s.resolveAnsweredAttention(
					ctx,
					owner,
					attention,
				); err != nil {
					return driver.Submission{}, err
				}
			}
		}
		return cached, nil
	}
	var answered *journal.AttentionProjection
	var claim journal.Claim
	switch effect.State {
	case journal.Claimed:
		if recovery != nil {
			attention, found, attentionErr := s.attentionForWork(
				ctx,
				manifest.value.RunID,
				attemptIdentity.WorkID,
			)
			if attentionErr != nil {
				return driver.Submission{}, attentionErr
			}
			if found {
				if attention.Attention.Recovery.CycleID !=
					recovery.binding.CycleID ||
					attention.Attention.Recovery.LaneID !=
						recovery.binding.LaneID {
					return driver.Submission{},
						runtimeFail("CORRUPT_JOURNAL", nil)
				}
				if attention.State == journal.AttentionOpen {
					return driver.Submission{},
						runtimeFail("EFFECT_PARKED", nil)
				}
				answered = &attention
				claim = journal.Claim{
					RunID:    manifest.value.RunID,
					EffectID: replayKey,
					Token:    effect.CurrentClaim,
				}
				break
			}
		}
		_ = s.journal.ReconcileOwned(ctx, owner, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey,
			Token: effect.CurrentClaim, EventKind: "dispatch_uncertain",
			EventBody: MarshalAssociation(EventAssociation{
				EffectID: replayKey,
				WorkID:   attemptIdentity.WorkID,
				Slice:    coordinates.Slice,
			}), At: s.now().UTC(),
		}, journal.RecoveryAmbiguous)
		return driver.Submission{}, runtimeFail("RECOVERY_UNCERTAIN", nil)
	case journal.Pending:
		claim, err = s.journal.ClaimOwned(
			ctx,
			owner,
			replayKey,
			s.now().UTC(),
			effectLease,
		)
		if err != nil {
			return driver.Submission{},
				runtimeFail("EFFECT_CLAIM_FAILED", err)
		}
	default:
		return driver.Submission{}, runtimeFail("EFFECT_PARKED", nil)
	}
	var (
		observation         driver.Observation
		pendingContinuation *retainedContinuation
		continuationFact    *continuationDispatchFact
		invokeErr           error
		recovered           bool
		checkpointed        bool
	)
	if answered != nil && answered.Attention.HumanTurn != nil {
		observation, checkpointed, invokeErr =
			s.loadHumanHandoffCheckpoint(
				ctx,
				manifest.value.RunID,
				replayKey,
				*answered,
			)
		if checkpointed {
			recovered = true
		}
	}
	if checkpointed {
		// The exact sealed handoff was already validated and checkpointed
		// before a prior process died. Continue from those immutable bytes.
	} else if answered != nil {
		observation, pendingContinuation, recovered, invokeErr =
			s.resumeAnsweredWorker(
				ctx,
				engine,
				workspace,
				prepared,
				before,
				owner,
				recovery,
				replayKey,
				*answered,
			)
	} else {
		observation, pendingContinuation, continuationFact, invokeErr =
			s.invokePreparedDriver(
				ctx,
				engine,
				workspace,
				coordinates,
				before,
				prepared,
				owner,
				recovery,
			)
		if recovery != nil &&
			driver.IsCode(invokeErr, "RECOVERY_STEP_REFUSED") {
			parkErr := s.parkTurnRecovery(
				ctx,
				owner,
				recovery,
				replayKey,
				"Automatic recovery reached its bound. What should the worker do next?",
				pendingContinuation,
			)
			pendingContinuation = nil
			return driver.Submission{}, errors.Join(parkErr, invokeErr)
		}
		if recovery != nil && observation.Yield != nil {
			observation, pendingContinuation, recovered, invokeErr =
				s.continueYieldedWorker(
					ctx,
					engine,
					workspace,
					prepared,
					before,
					owner,
					recovery,
					replayKey,
					observation,
					pendingContinuation,
				)
		}
	}
	if IsCode(invokeErr, "EFFECT_PARKED") ||
		IsCode(invokeErr, "RECOVERY_UNCERTAIN") {
		return driver.Submission{}, invokeErr
	}
	pendingStored := false
	pendingCommitted := false
	pendingKind := continuationDesign
	if coordinates.Responsibility == driver.WorkVerification {
		pendingKind = continuationVerifier
	}
	defer func() {
		if pendingContinuation == nil || pendingCommitted {
			return
		}
		var cleanupErr error
		if pendingStored {
			cleanupErr = s.discardRetainedContinuation(
				manifest.value.RunID,
				pendingKind,
				coordinates.Slice,
			)
		} else {
			cleanupErr = closeRetainedContinuation(
				pendingContinuation,
			)
		}
		if cleanupErr != nil {
			submissionResult = driver.Submission{}
			resultErr = cleanupErr
		}
	}()
	preserveAnswered := func(cause error) error {
		if answered == nil {
			return cause
		}
		if pendingContinuation != nil &&
			pendingContinuation.handle != nil {
			if err := s.storeRecoverableContinuation(
				owner.RunID,
				replayKey,
				pendingContinuation,
			); err != nil {
				return runtimeFail(
					"RECOVERY_UNCERTAIN",
					errors.Join(cause, err),
				)
			}
		}
		pendingCommitted = true
		return runtimeFail("RECOVERY_UNCERTAIN", cause)
	}
	defaultAssocBody := MarshalAssociation(EventAssociation{
		EffectID: replayKey,
		WorkID:   attemptIdentity.WorkID,
		Slice:    coordinates.Slice,
	})
	eventKind := func(base string) string {
		kind, kindErr := continuationEventKind(
			base,
			continuationFact,
		)
		if kindErr != nil {
			return base
		}
		return kind
	}
	eventBody := func(defaultBody []byte) []byte {
		if continuationFact != nil &&
			continuationFact.mode == driver.ContinuationModeFreshRehydrate &&
			(continuationFact.outcome == continuationOutcomeFallback ||
				continuationFact.outcome == continuationOutcomeFallbackExpired) {
			reason := continuationFact.reason
			if reason == "" {
				reason = "absence"
			}
			return []byte(reason)
		}
		return defaultBody
	}
	if testCrashAfterEffect == "driver.dispatch" {
		os.Exit(86)
	}
	completionCtx := context.WithoutCancel(ctx)
	usageBody, usageErr := driver.EncodeUsageReceipt(observation.Usage)
	if usageErr != nil {
		usageBody = []byte(`{"token_status":"unavailable","input_tokens":null,"output_tokens":null,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null}`)
	}
	observationBody, _ := json.Marshal(observation)
	transport := observation.TransportStatus
	if transport == "" && invokeErr != nil {
		transport = driver.RunnerError
	}
	attempt := &journal.Attempt{
		Number: coordinates.Try, Responsibility: string(coordinates.Responsibility),
		TransportStatus:   string(transport),
		ObservationDigest: sha256Digest(observationBody),
		Usage:             usageBody,
	}
	if observation.Handoff != nil {
		attempt.HandoffDigest = observation.Handoff.SubmissionDigest
	}
	if invokeErr != nil || observation.Handoff == nil {
		if answered != nil {
			return driver.Submission{}, preserveAnswered(invokeErr)
		}
		code := stableErrorCode(invokeErr)
		resultBytes := extractRefusalResult(invokeErr)
		if err := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: code, Attempt: attempt,
			Result:    resultBytes,
			EventKind: eventKind("dispatch_operational_failure"),
			EventBody: eventBody(defaultAssocBody), At: s.now().UTC(),
		}); err != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return driver.Submission{}, runtimeFail("DRIVER_OPERATIONAL_FAILURE", invokeErr)
	}
	submission, err := driver.DecodeSubmission(observation.Handoff.SubmissionBytes)
	if err != nil ||
		submission.Responsibility != coordinates.Responsibility ||
		submission.InvocationID != prepared.request.InvocationID ||
		(prepared.fake &&
			!bytes.Equal(
				observation.Handoff.SubmissionBytes,
				prepared.expectedSubmission,
			)) {
		if answered != nil {
			return driver.Submission{},
				preserveAnswered(
					runtimeFail("INVALID_DRIVER_HANDOFF", err),
				)
		}
		if completeErr := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: "invalid_driver_handoff",
			Attempt:   attempt,
			EventKind: eventKind("dispatch_operational_failure"),
			EventBody: eventBody(defaultAssocBody), At: s.now().UTC(),
		}); completeErr != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
		}
		return driver.Submission{}, runtimeFail("INVALID_DRIVER_HANDOFF", err)
	}
	if planErr := s.validateHumanConfirmedPlannerHandoff(
		completionCtx,
		manifest,
		coordinates,
		replayKey,
		submission,
		answered,
	); planErr != nil {
		if answered != nil {
			return driver.Submission{}, preserveAnswered(planErr)
		}
		if completeErr := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: "invalid_human_turn",
			Attempt:   attempt,
			EventKind: eventKind("dispatch_operational_failure"),
			EventBody: eventBody(defaultAssocBody), At: s.now().UTC(),
		}); completeErr != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
		}
		return driver.Submission{}, planErr
	}
	if !prepared.fake {
		currentBody, authorityErr := currentPreparedProductionBody(
			completionCtx,
			engine,
			coordinates,
			before,
			prepared,
		)
		if authorityErr != nil || !bytes.Equal(currentBody, prepared.inputBody) {
			if answered != nil {
				if authorityErr == nil {
					authorityErr = runtimeFail("STALE_DISPATCH", nil)
				}
				return driver.Submission{},
					preserveAnswered(authorityErr)
			}
			code := stableErrorCode(authorityErr)
			if authorityErr == nil {
				code = "stale_authority"
			}
			if completeErr := s.journal.CompleteOwned(
				completionCtx,
				owner,
				journal.Completion{
					RunID: manifest.value.RunID, EffectID: replayKey,
					Token: claim.Token, State: journal.OperationalFailed,
					ErrorCode: code, Attempt: attempt,
					EventKind: eventKind(
						"dispatch_operational_failure",
					),
					EventBody: eventBody(defaultAssocBody),
					At:        s.now().UTC(),
				},
			); completeErr != nil {
				return driver.Submission{},
					runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
			}
			if authorityErr != nil {
				return driver.Submission{}, authorityErr
			}
			return driver.Submission{}, runtimeFail("STALE_DISPATCH", nil)
		}
	}
	if answered != nil && answered.Attention.HumanTurn != nil {
		if err := s.persistHumanHandoffCheckpoint(
			completionCtx,
			owner,
			replayKey,
			*answered,
			observation,
		); err != nil {
			return driver.Submission{}, preserveAnswered(err)
		}
	}
	if prepareHandoff != nil {
		if err := prepareHandoff(submission); err != nil {
			if answered != nil {
				return driver.Submission{}, preserveAnswered(err)
			}
			var uncertain *uncertainHandoffPreparationError
			if errors.As(err, &uncertain) {
				reconcileErr := s.journal.ReconcileOwned(
					completionCtx,
					owner,
					journal.Completion{
						RunID: manifest.value.RunID, EffectID: replayKey,
						Token: claim.Token,
						EventKind: eventKind(
							"dispatch_preparation_uncertain",
						),
						EventBody: eventBody(defaultAssocBody),
						At:        s.now().UTC(),
					},
					journal.RecoveryAmbiguous,
				)
				return driver.Submission{}, runtimeFail(
					"RECOVERY_UNCERTAIN",
					errors.Join(err, reconcileErr),
				)
			}
			resultBytes := extractRefusalResult(err)
			if completeErr := s.journal.CompleteOwned(
				completionCtx,
				owner,
				journal.Completion{
					RunID: manifest.value.RunID, EffectID: replayKey,
					Token: claim.Token, State: journal.OperationalFailed,
					ErrorCode: stableErrorCode(err), Attempt: attempt,
					Result: resultBytes,
					EventKind: eventKind(
						"dispatch_preparation_failed",
					),
					EventBody: eventBody(defaultAssocBody),
					At:        s.now().UTC(),
				},
			); completeErr != nil {
				return driver.Submission{},
					runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
			}
			return driver.Submission{}, err
		}
	}
	if answered != nil && answered.Attention.HumanTurn != nil {
		crashHumanTurnBarrier("after_terminal_handoff")
	}
	if pendingContinuation != nil &&
		coordinates.Responsibility == driver.WorkVerification &&
		(submission.Decision == nil ||
			submission.Decision.Outcome != driver.DecisionFail) {
		if err := closeRetainedContinuation(
			pendingContinuation,
		); err != nil {
			return driver.Submission{}, err
		}
		pendingContinuation = nil
	}
	if pendingContinuation != nil {
		if err := s.storeRetainedContinuation(
			manifest.value.RunID,
			pendingKind,
			coordinates.Slice,
			pendingContinuation,
		); err != nil {
			if answered != nil {
				return driver.Submission{}, preserveAnswered(err)
			}
			if completeErr := s.journal.CompleteOwned(
				completionCtx,
				owner,
				journal.Completion{
					RunID: manifest.value.RunID, EffectID: replayKey,
					Token: claim.Token, State: journal.OperationalFailed,
					ErrorCode: stableErrorCode(err), Attempt: attempt,
					EventKind: eventKind(
						"dispatch_operational_failure",
					),
					EventBody: eventBody(defaultAssocBody),
					At:        s.now().UTC(),
				},
			); completeErr != nil {
				return driver.Submission{},
					runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
			}
			return driver.Submission{}, err
		}
		pendingStored = true
	}
	if recovery != nil && !recovered {
		if budget, budgetErr := s.journal.RecoveryBudget(
			completionCtx,
			manifest.value.RunID,
			recovery.binding,
		); budgetErr == nil {
			recovered = budget.AutomaticActions > 0
		}
	}
	completionEvent := "dispatch_completed"
	if recovered {
		// Bind the recovery outcome to the same durable transaction as the
		// successful dispatch. A restart can neither lose nor duplicate it.
		completionEvent = turnRecoveryRecoveredEvent
	}
	if err := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
		RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
		State: journal.Succeeded, Result: observation.Handoff.SubmissionBytes,
		Attempt: attempt,
		Receipts: []journal.Receipt{{
			Kind: "sealed_driver_handoff", Body: observation.Handoff.SubmissionBytes,
		}},
		EventKind: eventKind(completionEvent),
		EventBody: eventBody(defaultAssocBody), At: s.now().UTC(),
	}); err != nil {
		if prepareHandoff != nil {
			reconcileErr := s.journal.ReconcileOwned(
				completionCtx,
				owner,
				journal.Completion{
					RunID: manifest.value.RunID, EffectID: replayKey,
					Token: claim.Token,
					EventKind: eventKind(
						"dispatch_completion_uncertain",
					),
					EventBody: eventBody(defaultAssocBody),
					At:        s.now().UTC(),
				},
				journal.RecoveryAmbiguous,
			)
			return driver.Submission{}, runtimeFail(
				"RECOVERY_UNCERTAIN",
				errors.Join(err, reconcileErr),
			)
		}
		return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	pendingCommitted = true
	if answered != nil && answered.Attention.HumanTurn != nil {
		crashHumanTurnBarrier("after_terminal_completion")
	}
	if answered != nil {
		if err := s.resolveAnsweredAttention(
			completionCtx,
			owner,
			*answered,
		); err != nil {
			return driver.Submission{}, err
		}
	}
	return submission, nil
}
