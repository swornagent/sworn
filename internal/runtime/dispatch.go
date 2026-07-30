package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"

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

func continuationBindingForDispatch(
	prepared preparedDriverDispatch,
	coordinates dispatchCoordinates,
) (driver.ContinuationBinding, string, error) {
	if prepared.productionContext == nil ||
		prepared.productionContext.Plan == nil ||
		prepared.productionContext.Receipt == nil ||
		coordinates.Slice == "" ||
		coordinates.BatonAttempt < 1 ||
		prepared.request.Role != driver.RoleImplementer {
		return driver.ContinuationBinding{}, "",
			runtimeFail("INVALID_CONTINUATION", nil)
	}
	workContext := prepared.productionContext
	planDigest := driver.Digest(mustJSON(continuationPlanAuthority{
		OID:      workContext.Plan.OID,
		Digest:   workContext.Plan.Digest,
		Revision: workContext.Plan.Revision,
	}))
	targetDigest := driver.Digest(mustJSON(continuationTargetAuthority{
		TargetRef:    workContext.Authority.TargetRef,
		TargetHead:   workContext.Authority.TargetHead,
		Track:        workContext.Track,
		TrackRef:     workContext.Authority.TrackRef,
		PreparedBase: workContext.PreparedBase,
		Evidence: append(
			make(
				[]productionEvidenceBinding,
				0,
				len(workContext.Evidence),
			),
			workContext.Evidence...,
		),
	}))
	toolDigest := driver.Digest(mustJSON(continuationToolAuthority{
		SchemaVersion: "sworn.continuation-tool-transition/v1",
		Operation:     prepared.request.Operation,
		Source: continuationToolStage{
			Responsibility: driver.ImplementerDesign,
			Workspace:      driver.ReadOnly,
			FreshContext:   true,
		},
		Resume: continuationToolStage{
			Responsibility: driver.ImplementerImplementation,
			Workspace:      driver.ReadWrite,
			FreshContext:   false,
		},
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
	}
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
	current, currentBody, err := captureProductionWorkContext(
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
		currentBody = mustJSON(current)
	}
	return currentBody, nil
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

func (s *Service) invokePreparedDriver(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	coordinates dispatchCoordinates,
	before string,
	prepared preparedDriverDispatch,
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
	)
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
		observation, handle, result, invokeErr :=
			continuationDriver.InvokeTurn(
				ctx,
				invocation,
				binding,
				nil,
			)
		invokeErr = runtimeContinuationError(invokeErr)
		if handle == nil {
			if !validFreshContinuationStart(handle, result) {
				return observation, nil, nil,
					errors.Join(
						invokeErr,
						runtimeFail("INVALID_CONTINUATION", nil),
					)
			}
			return observation, nil, nil, invokeErr
		}
		if invokeErr != nil ||
			result.Status != driver.ContinuationStatusSuspended ||
			!validRetainedContinuationMode(result.Mode) {
			closeErr := closeRetainedContinuation(
				&retainedContinuation{handle: handle},
			)
			if closeErr != nil {
				return observation, nil, nil, closeErr
			}
			return observation, nil, nil,
				errors.Join(
					invokeErr,
					runtimeFail("INVALID_CONTINUATION", nil),
				)
		}
		workContext := prepared.productionContext
		return observation, &retainedContinuation{
			handle:          handle,
			binding:         binding,
			selectionDigest: selectionDigest,
			before:          before,
			sourceReceipt:   workContext.Receipt.OID,
			sourceTrackHead: workContext.Authority.TrackHead,
		}, nil, nil
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
		if !matches {
			if cleanupErr := closeRetainedContinuation(entry); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			observation, invokeErr := s.dispatcher.Invoke(ctx, invocation)
			return observation, nil, &continuationDispatchFact{
				mode:    driver.ContinuationModeFreshRehydrate,
				outcome: continuationOutcomeFallback,
			}, invokeErr
		}
		if !supported ||
			prepared.resumeRequest == nil ||
			prepared.resumePermission == nil {
			if cleanupErr := closeRetainedContinuation(entry); cleanupErr != nil {
				return driver.Observation{}, nil, nil, cleanupErr
			}
			observation, invokeErr := s.dispatcher.Invoke(ctx, invocation)
			return observation, nil, &continuationDispatchFact{
				mode:    driver.ContinuationModeFreshRehydrate,
				outcome: continuationOutcomeFallback,
			}, invokeErr
		}
		resumeInvocation := preparedInvocation(
			prepared,
			workspace,
			*prepared.resumeRequest,
			*prepared.resumePermission,
		)
		sourceHandle := entry.handle
		observation, next, result, invokeErr :=
			continuationDriver.InvokeTurn(
				ctx,
				resumeInvocation,
				binding,
				sourceHandle,
			)
		entry.handle = nil
		if cleanupErr := sourceHandle.Close(); cleanupErr != nil {
			if next != nil {
				cleanupErr = errors.Join(
					cleanupErr,
					next.Close(),
				)
			}
			return driver.Observation{}, nil, nil,
				runtimeFail(
					"CONTINUATION_CLEANUP_FAILED",
					cleanupErr,
				)
		}
		invokeErr = runtimeContinuationError(invokeErr)
		if requestsFreshRehydrate(observation, result, invokeErr) {
			if next != nil {
				if cleanupErr := closeRetainedContinuation(
					&retainedContinuation{handle: next},
				); cleanupErr != nil {
					return driver.Observation{}, nil, nil, cleanupErr
				}
				return driver.Observation{}, nil, nil,
					runtimeFail("INVALID_CONTINUATION", nil)
			}
			if err := revalidatePreparedProductionDispatch(
				ctx,
				engine,
				coordinates,
				before,
				prepared,
			); err != nil {
				return driver.Observation{}, nil, nil, err
			}
			outcome := continuationOutcomeFallback
			if result.Status == driver.ContinuationStatusExpired {
				outcome = continuationOutcomeFallbackExpired
			}
			observation, invokeErr = s.dispatcher.Invoke(ctx, invocation)
			return observation, nil, &continuationDispatchFact{
				mode:    driver.ContinuationModeFreshRehydrate,
				outcome: outcome,
			}, invokeErr
		}
		if !validRetainedContinuationResume(next, result) {
			if cleanupErr := closeRetainedContinuation(
				&retainedContinuation{handle: next},
			); cleanupErr != nil {
				return observation, nil, nil, cleanupErr
			}
			return observation, nil, nil,
				errors.Join(
					invokeErr,
					runtimeFail("INVALID_CONTINUATION", nil),
				)
		}
		return observation, nil, &continuationDispatchFact{
			mode:    result.Mode,
			outcome: continuationOutcomeReuse,
		}, invokeErr
	default:
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
	if err := engine.configured.certifySelected(ctx, selected); err != nil {
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
		if coordinates.Responsibility ==
			driver.ImplementerImplementation &&
			prepared.productionContext.DesignReceipt != nil {
			resumeRequest, requestErr :=
				productionRequestForContextFreshness(
					engine.manifest,
					*prepared.productionContext,
					false,
				)
			if requestErr != nil {
				return preparedDriverDispatch{}, requestErr
			}
			resumePermission, permissionErr :=
				driver.NewSubmissionPermission(
					resumeRequest,
					selected,
					driver.ContainmentReadWrite,
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
		if prior.SchemaVersion == productionDispatchVersionV1 {
			prepared, err = downgradePreparedProductionDispatchV1(
				manifest,
				prepared,
			)
			if err != nil {
				return driver.Submission{}, err
			}
		}
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
		return cached, nil
	}
	if effect.State == journal.Claimed {
		_ = s.journal.ReconcileOwned(ctx, owner, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey,
			Token: effect.CurrentClaim, EventKind: "dispatch_uncertain",
			EventBody: []byte(coordinates.Responsibility), At: s.now().UTC(),
		}, journal.RecoveryAmbiguous)
		return driver.Submission{}, runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	if effect.State != journal.Pending {
		return driver.Submission{}, runtimeFail("EFFECT_PARKED", nil)
	}
	claim, err := s.journal.ClaimOwned(
		ctx, owner,
		replayKey,
		s.now().UTC(),
		effectLease,
	)
	if err != nil {
		return driver.Submission{}, runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	observation, pendingContinuation, continuationFact, invokeErr :=
		s.invokePreparedDriver(
			ctx,
			engine,
			workspace,
			coordinates,
			before,
			prepared,
		)
	pendingStored := false
	pendingCommitted := false
	defer func() {
		if pendingContinuation == nil || pendingCommitted {
			return
		}
		var cleanupErr error
		if pendingStored {
			cleanupErr = s.discardContinuation(
				manifest.value.RunID,
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
	if testCrashAfterEffect == "driver.dispatch" {
		os.Exit(86)
	}
	completionCtx := context.WithoutCancel(ctx)
	usageBody, usageErr := driver.EncodeUsageReceipt(observation.Usage)
	if usageErr != nil {
		usageBody = []byte(`{"token_status":"unavailable","input_tokens":null,"output_tokens":null,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null}`)
	}
	observationBody, _ := json.Marshal(observation)
	attempt := &journal.Attempt{
		Number: coordinates.Try, Responsibility: string(coordinates.Responsibility),
		TransportStatus:   string(observation.TransportStatus),
		ObservationDigest: sha256Digest(observationBody),
		Usage:             usageBody,
	}
	if observation.Handoff != nil {
		attempt.HandoffDigest = observation.Handoff.SubmissionDigest
	}
	if invokeErr != nil || observation.Handoff == nil {
		code := stableErrorCode(invokeErr)
		if err := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: code, Attempt: attempt,
			EventKind: eventKind("dispatch_operational_failure"),
			EventBody: []byte(coordinates.Responsibility), At: s.now().UTC(),
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
		if completeErr := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: "invalid_driver_handoff",
			Attempt:   attempt,
			EventKind: eventKind("dispatch_operational_failure"),
			EventBody: []byte(coordinates.Responsibility), At: s.now().UTC(),
		}); completeErr != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
		}
		return driver.Submission{}, runtimeFail("INVALID_DRIVER_HANDOFF", err)
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
					EventBody: []byte(coordinates.Responsibility),
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
	if prepareHandoff != nil {
		if err := prepareHandoff(submission); err != nil {
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
						EventBody: []byte(coordinates.Responsibility),
						At:        s.now().UTC(),
					},
					journal.RecoveryAmbiguous,
				)
				return driver.Submission{}, runtimeFail(
					"RECOVERY_UNCERTAIN",
					errors.Join(err, reconcileErr),
				)
			}
			if completeErr := s.journal.CompleteOwned(
				completionCtx,
				owner,
				journal.Completion{
					RunID: manifest.value.RunID, EffectID: replayKey,
					Token: claim.Token, State: journal.OperationalFailed,
					ErrorCode: stableErrorCode(err), Attempt: attempt,
					EventKind: eventKind(
						"dispatch_preparation_failed",
					),
					EventBody: []byte(coordinates.Responsibility),
					At:        s.now().UTC(),
				},
			); completeErr != nil {
				return driver.Submission{},
					runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
			}
			return driver.Submission{}, err
		}
	}
	if pendingContinuation != nil {
		if err := s.storeContinuation(
			manifest.value.RunID,
			coordinates.Slice,
			pendingContinuation,
		); err != nil {
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
					EventBody: []byte(coordinates.Responsibility),
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
	if err := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
		RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
		State: journal.Succeeded, Result: observation.Handoff.SubmissionBytes,
		Attempt: attempt,
		Receipts: []journal.Receipt{{
			Kind: "sealed_driver_handoff", Body: observation.Handoff.SubmissionBytes,
		}},
		EventKind: eventKind("dispatch_completed"),
		EventBody: []byte(coordinates.Responsibility), At: s.now().UTC(),
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
					EventBody: []byte(coordinates.Responsibility),
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
	return submission, nil
}
