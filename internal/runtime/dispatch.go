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
		prepared.commandPayload = mustJSON(productionDispatchCommand{
			SchemaVersion: productionDispatchVersion,
			RequestDigest: driver.Digest(requestBody),
			Context:       *prepared.productionContext,
		})
	}
	return prepared, nil
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
	prepareHandoff func(driver.Submission) error) (driver.Submission, error) {
	manifest := engine.manifest
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
	replayKey := journal.AttemptEffectID(
		attemptIdentity.WorkID,
		attemptIdentity.Epoch,
		attemptIdentity.Try,
	)
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
	fakeProfile := driver.FakeProfile("")
	if prepared.fake {
		fakeProfile = driver.FakeCompleted
	}
	observation, invokeErr := s.dispatcher.Invoke(ctx, driver.Invocation{
		Request:       prepared.request,
		HostWorkspace: workspace.Path(),
		Selected:      prepared.selected,
		Permission:    prepared.permission,
		Inputs:        prepared.inputs,
		FakeProfile:   fakeProfile,
	})
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
			EventKind: "dispatch_operational_failure",
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
			Attempt: attempt, EventKind: "dispatch_operational_failure",
			EventBody: []byte(coordinates.Responsibility), At: s.now().UTC(),
		}); completeErr != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
		}
		return driver.Submission{}, runtimeFail("INVALID_DRIVER_HANDOFF", err)
	}
	if !prepared.fake {
		_, currentBody, authorityErr := captureProductionWorkContext(
			completionCtx,
			engine,
			coordinates,
			before,
			prepared.request.Workspace.Access,
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
					EventKind: "dispatch_operational_failure",
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
						Token: claim.Token, EventKind: "dispatch_preparation_uncertain",
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
					EventKind: "dispatch_preparation_failed",
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
	if err := s.journal.CompleteOwned(completionCtx, owner, journal.Completion{
		RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
		State: journal.Succeeded, Result: observation.Handoff.SubmissionBytes,
		Attempt: attempt,
		Receipts: []journal.Receipt{{
			Kind: "sealed_driver_handoff", Body: observation.Handoff.SubmissionBytes,
		}},
		EventKind: "dispatch_completed",
		EventBody: []byte(coordinates.Responsibility), At: s.now().UTC(),
	}); err != nil {
		if prepareHandoff != nil {
			reconcileErr := s.journal.ReconcileOwned(
				completionCtx,
				owner,
				journal.Completion{
					RunID: manifest.value.RunID, EffectID: replayKey,
					Token: claim.Token, EventKind: "dispatch_completion_uncertain",
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
	return submission, nil
}
