package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

type fakeScript struct {
	SchemaVersion string `json:"schema_version"`
	Behavior      string `json:"behavior"`
	Submission    string `json:"submission"`
}

func (s *Service) runDriverEffect(
	ctx context.Context,
	engine *engine,
	workspace *gitx.WorkspaceLease,
	role driver.Role,
	responsibility driver.Responsibility,
	before string,
) (driver.Submission, error) {
	if workspace == nil || workspace.Path() == "" {
		return driver.Submission{}, runtimeFail("INVALID_WORKSPACE", nil)
	}
	manifest := engine.manifest
	submissionEncoding := manifest.value.Submissions.forResponsibility(responsibility)
	submissionBody, err := base64.StdEncoding.Strict().DecodeString(submissionEncoding)
	if err != nil {
		return driver.Submission{}, runtimeFail("INVALID_SCRIPTED_SUBMISSION", nil)
	}
	scriptBody := mustJSON(fakeScript{
		SchemaVersion: "sworn.fake-script/v1",
		Behavior:      "submit",
		Submission:    submissionEncoding,
	})
	input := driver.Input{
		Name: "fake-script", Path: "runtime/fake-script.json",
		Digest: driver.Digest(scriptBody),
	}
	selected, err := engine.registry.Resolve(manifest.value.Roles, role)
	if err != nil {
		return driver.Submission{}, runtimeFail("DRIVER_SELECTION_FAILED", err)
	}
	access := driver.ReadOnly
	containment := driver.ContainmentReadOnly
	if workspace.Access() == gitx.WorkspaceReadWrite {
		access = driver.ReadWrite
		containment = driver.ContainmentReadWrite
	}
	request, err := driver.NewRequest(
		invocationID(manifest.value.RunID, responsibility),
		role,
		selected.Profile.Key,
		selected.Model,
		driver.Workspace{Path: driver.GuestWorkspacePath, Access: access},
		[]driver.Input{input},
		true,
		manifest.value.Limits,
	)
	if err != nil {
		return driver.Submission{}, runtimeFail("DRIVER_REQUEST_FAILED", err)
	}
	permission, err := driver.NewSubmissionPermission(
		request,
		selected,
		containment,
		responsibility,
	)
	if err != nil {
		return driver.Submission{}, runtimeFail("DRIVER_REQUEST_FAILED", err)
	}
	replayKey := "dispatch." + string(responsibility)
	if err := s.ensureEffect(
		ctx,
		manifest,
		replayKey,
		"driver.dispatch",
		scriptBody,
		sha256Digest([]byte(before)),
		sha256Digest(submissionBody),
	); err != nil {
		return driver.Submission{}, err
	}
	effect, err := s.journal.Effect(ctx, manifest.value.RunID, replayKey)
	if err != nil {
		return driver.Submission{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		cached, err := driver.DecodeSubmission(effect.Result)
		if err != nil || cached.Responsibility != responsibility ||
			!bytes.Equal(effect.Result, submissionBody) {
			return driver.Submission{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return cached, nil
	}
	if effect.State == journal.Claimed {
		_ = s.journal.Reconcile(ctx, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey,
			Token: effect.CurrentClaim, EventKind: "dispatch_uncertain",
			EventBody: []byte(responsibility), At: s.now().UTC(),
		}, journal.RecoveryAmbiguous)
		return driver.Submission{}, runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	if effect.State != journal.Pending {
		return driver.Submission{}, runtimeFail("EFFECT_PARKED", nil)
	}
	claim, err := s.journal.Claim(
		ctx,
		manifest.value.RunID,
		replayKey,
		s.now().UTC(),
		effectLease,
	)
	if err != nil {
		return driver.Submission{}, runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	observation, invokeErr := s.dispatcher.Invoke(ctx, driver.Invocation{
		Request:       request,
		HostWorkspace: workspace.Path(),
		Selected:      selected,
		Permission:    permission,
		Inputs: []driver.InputContent{{
			Input: input, Bytes: scriptBody,
		}},
		FakeProfile: driver.FakeCompleted,
	})
	usageBody, usageErr := driver.EncodeUsageReceipt(observation.Usage)
	if usageErr != nil {
		usageBody = []byte(`{"token_status":"unavailable","input_tokens":null,"output_tokens":null,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null}`)
	}
	observationBody, _ := json.Marshal(observation)
	attempt := &journal.Attempt{
		Number: 1, Responsibility: string(responsibility),
		TransportStatus:   string(observation.TransportStatus),
		ObservationDigest: sha256Digest(observationBody),
		Usage:             usageBody,
	}
	if observation.Handoff != nil {
		attempt.HandoffDigest = observation.Handoff.SubmissionDigest
	}
	if invokeErr != nil || observation.Handoff == nil {
		code := stableErrorCode(invokeErr)
		if err := s.journal.Complete(ctx, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: code, Attempt: attempt,
			EventKind: "dispatch_operational_failure",
			EventBody: []byte(responsibility), At: s.now().UTC(),
		}); err != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
		return driver.Submission{}, runtimeFail("DRIVER_OPERATIONAL_FAILURE", invokeErr)
	}
	submission, err := driver.DecodeSubmission(observation.Handoff.SubmissionBytes)
	if err != nil || submission.Responsibility != responsibility ||
		!bytes.Equal(observation.Handoff.SubmissionBytes, submissionBody) {
		if completeErr := s.journal.Complete(ctx, journal.Completion{
			RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: "invalid_driver_handoff",
			Attempt: attempt, EventKind: "dispatch_operational_failure",
			EventBody: []byte(responsibility), At: s.now().UTC(),
		}); completeErr != nil {
			return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
		}
		return driver.Submission{}, runtimeFail("INVALID_DRIVER_HANDOFF", err)
	}
	if err := s.journal.Complete(ctx, journal.Completion{
		RunID: manifest.value.RunID, EffectID: replayKey, Token: claim.Token,
		State: journal.Succeeded, Result: observation.Handoff.SubmissionBytes,
		Attempt: attempt,
		Receipts: []journal.Receipt{{
			Kind: "sealed_driver_handoff", Body: observation.Handoff.SubmissionBytes,
		}},
		EventKind: "dispatch_completed",
		EventBody: []byte(responsibility), At: s.now().UTC(),
	}); err != nil {
		return driver.Submission{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return submission, nil
}
