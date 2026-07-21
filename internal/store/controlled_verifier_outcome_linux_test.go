//go:build linux

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

type verifierDispatchConvergenceAttempt struct {
	state    engine.State
	plan     protocol.ExactPlan
	request  policy.VerifierExecutionPermitRequest
	prepared PreparedVerifierDispatch
	command  engine.Command
	selector ControlledVerifierDispatchSelector
}

type verdictConvergenceAttempt struct {
	state    engine.State
	plan     protocol.ExactPlan
	request  policy.PASSAdmissionPermitRequest
	prepared PreparedVerdictAdmission
	command  engine.Command
	selector ControlledVerdictAdmissionSelector
}

func prepareVerifierDispatchConvergence(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	commandID string,
) verifierDispatchConvergenceAttempt {
	t.Helper()
	ctx := context.Background()
	state, plan, request, prepared, err := fixture.control.PrepareControlledVerifierDispatch(
		ctx, fixture.ownership, fixture.ownerID, fixture.runID, fixture.workID, commandID,
	)
	if err != nil {
		t.Fatal(err)
	}
	command := testCommand(
		t, commandID, engine.CommandDispatchVerifier, state.Revision,
		engine.DispatchVerifierPayload{WorkID: fixture.workID},
	)
	selector, err := prepared.ConvergenceSelector(fixture.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	return verifierDispatchConvergenceAttempt{
		state: state, plan: plan, request: request, prepared: prepared, command: command,
		selector: selector,
	}
}

func applyVerifierDispatchConvergence(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	attempt verifierDispatchConvergenceAttempt,
) ApplyResult {
	t.Helper()
	permit, err := fixture.authority.AuthorizeVerifierExecution(
		context.Background(), attempt.plan, attempt.request,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.control.ApplyControlledVerifierDispatch(
		context.Background(), fixture.ownership, fixture.authority, permit,
		attempt.request, attempt.prepared, attempt.command,
	)
	if err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("apply verifier convergence dispatch = %+v, %v", result, err)
	}
	return result
}

func prepareVerdictConvergence(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	commandID string,
) verdictConvergenceAttempt {
	t.Helper()
	state, plan, request, prepared, err := fixture.control.PrepareControlledVerdictAdmission(
		context.Background(), fixture.ownership, fixture.ownerID,
		fixture.runID, fixture.workID, commandID,
	)
	if err != nil {
		t.Fatal(err)
	}
	command := testCommand(
		t, commandID, engine.CommandAdmitVerdict, state.Revision,
		engine.AdmitVerdictPayload{WorkID: fixture.workID},
	)
	selector, err := prepared.ConvergenceSelector(fixture.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	return verdictConvergenceAttempt{
		state: state, plan: plan, request: request, prepared: prepared, command: command,
		selector: selector,
	}
}

func applyVerdictConvergence(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	attempt verdictConvergenceAttempt,
) ApplyResult {
	t.Helper()
	ctx := context.Background()
	var result ApplyResult
	var err error
	if attempt.prepared.outcome == engine.VerdictPass {
		permit, permitErr := fixture.authority.AuthorizePASSAdmission(ctx, attempt.plan, attempt.request)
		if permitErr != nil {
			t.Fatal(permitErr)
		}
		result, err = fixture.control.ApplyControlledPASSAdmission(
			ctx, fixture.ownership, fixture.authority, permit, attempt.request,
			attempt.prepared, attempt.command,
		)
	} else {
		result, err = fixture.control.ApplyControlledVerdictAdmission(
			ctx, fixture.ownership, fixture.ownerID, attempt.prepared, attempt.command,
		)
	}
	if err != nil || result.Outcome != OutcomeApplied {
		t.Fatalf("apply verifier convergence verdict = %+v, %v", result, err)
	}
	return result
}

func passAttentionSelector(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	attempt verdictConvergenceAttempt,
	attentionCommandID string,
) ControlledPASSAttentionSelector {
	t.Helper()
	selector, err := attempt.prepared.PASSAttentionConvergenceSelector(
		fixture.ownerID, attentionCommandID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return selector
}

func TestControlledVerifierConvergenceDistinguishesAbsenceFromExactReplay(t *testing.T) {
	t.Run("dispatch", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		attempt := prepareVerifierDispatchConvergence(t, fixture, "cmd-converge-dispatch")
		before := []int{
			tableCount(t, fixture.control, "commands"),
			tableCount(t, fixture.control, "events"),
			tableCount(t, fixture.control, "effects"),
			tableCount(t, fixture.control, "verifier_dispatch_records"),
		}
		result, found, err := fixture.control.ConvergeControlledVerifierDispatch(
			context.Background(), fixture.ownership, attempt.selector,
		)
		if err != nil || found || !reflect.DeepEqual(result, ApplyResult{}) {
			t.Fatalf("absent verifier dispatch = %+v, found=%t, error=%v", result, found, err)
		}
		if after := []int{
			tableCount(t, fixture.control, "commands"),
			tableCount(t, fixture.control, "events"),
			tableCount(t, fixture.control, "effects"),
			tableCount(t, fixture.control, "verifier_dispatch_records"),
		}; !reflect.DeepEqual(after, before) {
			t.Fatalf("absent verifier convergence mutated Store: got %v, want %v", after, before)
		}
		want := applyVerifierDispatchConvergence(t, fixture, attempt)
		result, found, err = fixture.control.ConvergeControlledVerifierDispatch(
			context.Background(), fixture.ownership, attempt.selector,
		)
		if err != nil || !found || !result.Replayed {
			t.Fatalf("durable verifier dispatch = %+v, found=%t, error=%v", result, found, err)
		}
		result.Replayed = false
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("replayed verifier dispatch = %+v, want %+v", result, want)
		}
	})

	t.Run("verdict", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		fixture.dispatch(t, "cmd-converge-verdict-dispatch")
		fixture.execute(t, engine.VerdictFail, false)
		attempt := prepareVerdictConvergence(t, fixture, "cmd-converge-verdict")
		result, found, err := fixture.control.ConvergeControlledVerdictAdmission(
			context.Background(), fixture.ownership, attempt.selector,
		)
		if err != nil || found || !reflect.DeepEqual(result, ApplyResult{}) {
			t.Fatalf("absent verdict = %+v, found=%t, error=%v", result, found, err)
		}
		want := applyVerdictConvergence(t, fixture, attempt)
		result, found, err = fixture.control.ConvergeControlledVerdictAdmission(
			context.Background(), fixture.ownership, attempt.selector,
		)
		if err != nil || !found || !result.Replayed {
			t.Fatalf("durable verdict = %+v, found=%t, error=%v", result, found, err)
		}
		result.Replayed = false
		want.EffectIDs = nil
		if !reflect.DeepEqual(result, want) {
			t.Fatalf("replayed verdict = %+v, want %+v", result, want)
		}
	})
}

func TestControlledVerdictConvergenceSeparatesRawAndCanonicalAssessmentDigests(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	fixture.dispatch(t, "cmd-noncanonical-assessment-dispatch")
	rawDigest, canonicalDigest := executeNoncanonicalVerifierAssessment(t, fixture, engine.VerdictFail)
	if rawDigest == canonicalDigest {
		t.Fatal("noncanonical assessment unexpectedly has its canonical record digest")
	}
	attempt := prepareVerdictConvergence(t, fixture, "cmd-noncanonical-assessment-verdict")
	if attempt.prepared.assessmentDigest != canonicalDigest {
		t.Fatalf("prepared assessment digest = %q, want canonical %q", attempt.prepared.assessmentDigest, canonicalDigest)
	}
	want := applyVerdictConvergence(t, fixture, attempt)
	result, found, err := fixture.control.ConvergeControlledVerdictAdmission(
		context.Background(), fixture.ownership, attempt.selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("noncanonical assessment convergence = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	want.EffectIDs = nil
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("noncanonical assessment replay = %+v, want %+v", result, want)
	}
}

func executeNoncanonicalVerifierAssessment(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	outcome engine.VerdictOutcome,
) (string, string) {
	t.Helper()
	ctx := context.Background()
	_, _, prepared := fixture.claimAndPrepare(t)
	assessment := fixture.assessment(t, outcome)
	canonical, err := protocol.EncodeCanonical(assessment)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(assessment, "", "  ")
	if err != nil || bytes.Equal(raw, canonical) {
		t.Fatalf("encode noncanonical assessment: equal=%t, error=%v", bytes.Equal(raw, canonical), err)
	}
	exact, err := protocol.ParseVerifierAssessment(raw)
	if err != nil {
		t.Fatal(err)
	}
	rawDigest, err := fixture.control.PutArtifact(ctx, "application/json", raw)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := protocol.EncodeCanonical(map[string]any{
		"schema_version": "sworn-verifier-execution-receipt-test-v1",
		"effect_id":      prepared.effect.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	receiptDigest, err := fixture.control.PutArtifact(ctx, verifierExecutionReceiptType, receipt)
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.ParseVerifierEffectRequest(prepared.effect.Request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := engine.EncodeVerifierEffectResult(engine.VerifierEffectResult{
		SchemaVersion:     engine.VerifierEffectResultSchemaVersion,
		Outcome:           engine.VerifierOutcomeAssessmentReady,
		DispatchID:        prepared.effect.ID,
		VerificationEpoch: request.VerificationEpoch,
		Assessment:        protocol.Artifact{Ref: rawDigest, MediaType: "application/json", Digest: rawDigest},
		ExecutionReceipt: protocol.Artifact{
			Ref: receiptDigest, MediaType: verifierExecutionReceiptType, Digest: receiptDigest,
		},
		StartedAt:   atomicAdmissionTime.Format(time.RFC3339Nano),
		CompletedAt: atomicAdmissionTime.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.RunVerifier(func(engine.JournalEffect) (json.RawMessage, error) {
		return encoded, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.BindAuthorizedVerifierResult(ctx, prepared, result); err != nil {
		t.Fatal(err)
	}
	if err := fixture.control.CompleteAuthorizedVerifier(ctx, prepared); err != nil {
		t.Fatal(err)
	}
	return rawDigest, exact.Record().Digest
}

func TestControlledVerifierConvergenceSurvivesRestartAndAdmittedPASSState(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	dispatchAttempt := prepareVerifierDispatchConvergence(t, fixture, "cmd-restart-verifier-dispatch")
	wantDispatch := applyVerifierDispatchConvergence(t, fixture, dispatchAttempt)
	fixture.execute(t, engine.VerdictPass, false)
	verdictAttempt := prepareVerdictConvergence(t, fixture, "cmd-restart-pass-verdict")
	wantVerdict := applyVerdictConvergence(t, fixture, verdictAttempt)
	state, err := fixture.control.State(context.Background(), fixture.runID)
	if err != nil || state.Work[0].State != engine.WorkVerified {
		t.Fatalf("admitted PASS state = %+v, %v", state.Work[0], err)
	}

	reopened, successor := restartVerifierConvergenceStore(t, fixture, "verifier-successor")
	dispatchAttempt.selector.ControllerID = "verifier-successor"
	result, found, err := reopened.ConvergeControlledVerifierDispatch(
		context.Background(), successor, dispatchAttempt.selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("restarted verifier dispatch = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	if !reflect.DeepEqual(result, wantDispatch) {
		t.Fatalf("restarted verifier dispatch = %+v, want %+v", result, wantDispatch)
	}
	verdictAttempt.selector.ControllerID = "verifier-successor"
	result, found, err = reopened.ConvergeControlledVerdictAdmission(
		context.Background(), successor, verdictAttempt.selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("restarted PASS verdict = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	wantVerdict.EffectIDs = nil
	if !reflect.DeepEqual(result, wantVerdict) {
		t.Fatalf("restarted PASS verdict = %+v, want %+v", result, wantVerdict)
	}
}

func TestControlledVerdictConvergenceSurvivesVerifierProfileRotation(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	dispatchAttempt := prepareVerifierDispatchConvergence(t, fixture, "cmd-profile-rotation-dispatch")
	wantDispatch := applyVerifierDispatchConvergence(t, fixture, dispatchAttempt)
	fixture.execute(t, engine.VerdictPass, false)
	attempt := prepareVerdictConvergence(t, fixture, "cmd-profile-rotation-verdict")
	want := applyVerdictConvergence(t, fixture, attempt)

	path := fixture.control.ControlPath()
	configuration := ControlConfiguration{
		LocalCheckRuntimeManifestDigest: fixture.control.localCheckRuntimeManifestDigest,
		BuilderDispatchDigest:           fixture.control.builderDispatchDigest,
		VerifierProfileDigest:           "sha256:" + strings.Repeat("a", 64),
		VerifierAgent:                   "codex-cli/rotated-verifier",
		Repository:                      fixture.control.repository,
	}
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
	if err := fixture.control.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenConfigured(context.Background(), path, configuration)
	if err != nil {
		t.Fatal(err)
	}
	successor, err := reopened.AcquireControllerOwnership("profile-rotation-successor")
	if err != nil {
		_ = reopened.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = successor.Close()
		_ = reopened.Close()
	})
	if err := successor.Activate(context.Background(), reopened, "profile-rotation-successor"); err != nil {
		t.Fatal(err)
	}
	dispatchAttempt.selector.ControllerID = "profile-rotation-successor"
	result, found, err := reopened.ConvergeControlledVerifierDispatch(
		context.Background(), successor, dispatchAttempt.selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("profile-rotated dispatch = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	if !reflect.DeepEqual(result, wantDispatch) {
		t.Fatalf("profile-rotated dispatch = %+v, want %+v", result, wantDispatch)
	}
	attempt.selector.ControllerID = "profile-rotation-successor"
	result, found, err = reopened.ConvergeControlledVerdictAdmission(
		context.Background(), successor, attempt.selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("profile-rotated verdict = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	want.EffectIDs = nil
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("profile-rotated verdict = %+v, want %+v", result, want)
	}
}

func TestControlledPASSAttentionConvergenceSurvivesLostResponseAndRestart(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	fixture.dispatch(t, "cmd-attention-converge-dispatch")
	fixture.execute(t, engine.VerdictPass, false)
	verdictAttempt := prepareVerdictConvergence(t, fixture, "cmd-attention-unadmitted-verdict")
	attentionCommandID := "cmd-converge-pass-attention"
	command, err := PASSAttentionCommand(
		attentionCommandID, fixture.runID, fixture.workID, verdictAttempt.state.Revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fixture.control.ApplyControlledPASSAttention(
		context.Background(), fixture.ownership, fixture.ownerID, verdictAttempt.prepared, command,
	)
	if err != nil || want.Outcome != OutcomeApplied {
		t.Fatalf("apply PASS attention = %+v, %v", want, err)
	}
	selector := passAttentionSelector(t, fixture, verdictAttempt, attentionCommandID)
	result, found, err := fixture.control.ConvergeControlledPASSAttention(
		context.Background(), fixture.ownership, selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("lost-response PASS attention = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	want.EffectIDs = nil
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("replayed PASS attention = %+v, want %+v", result, want)
	}
	admission := prepareVerdictConvergence(t, fixture, "cmd-admit-pass-after-attention")
	applyVerdictConvergence(t, fixture, admission)
	admitted, err := fixture.control.State(context.Background(), fixture.runID)
	if err != nil || admitted.Work[0].State != engine.WorkVerified || len(admitted.Attention) != 0 {
		t.Fatalf("PASS admission after attention = %+v, %v", admitted, err)
	}

	reopened, successor := restartVerifierConvergenceStore(t, fixture, "attention-successor")
	selector.ControllerID = "attention-successor"
	result, found, err = reopened.ConvergeControlledPASSAttention(
		context.Background(), successor, selector,
	)
	if err != nil || !found || !result.Replayed {
		t.Fatalf("restarted PASS attention = %+v, found=%t, error=%v", result, found, err)
	}
	result.Replayed = false
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("restarted PASS attention = %+v, want %+v", result, want)
	}
}

func TestControlledVerifierConvergenceRejectsOwnershipAndOccupiedIDs(t *testing.T) {
	fixture := newVerifierLifecycleFixture(t)
	dispatchAttempt := prepareVerifierDispatchConvergence(t, fixture, "cmd-ownership-dispatch")
	applyVerifierDispatchConvergence(t, fixture, dispatchAttempt)
	fixture.execute(t, engine.VerdictPass, false)
	verdictAttempt := prepareVerdictConvergence(t, fixture, "cmd-ownership-verdict")
	applyVerdictConvergence(t, fixture, verdictAttempt)

	if _, found, err := fixture.control.ConvergeControlledVerifierDispatch(
		context.Background(), nil, dispatchAttempt.selector,
	); !errors.Is(err, ErrInvalidControllerOwnership) || found {
		t.Fatalf("nil dispatch ownership = found=%t, error=%v", found, err)
	}
	if _, found, err := fixture.control.ConvergeControlledVerdictAdmission(
		context.Background(), nil, verdictAttempt.selector,
	); !errors.Is(err, ErrInvalidControllerOwnership) || found {
		t.Fatalf("nil verdict ownership = found=%t, error=%v", found, err)
	}

	dispatchCollision := dispatchAttempt.selector
	dispatchCollision.CommandID = "cmd-create"
	if _, found, err := fixture.control.ConvergeControlledVerifierDispatch(
		context.Background(), fixture.ownership, dispatchCollision,
	); !errors.Is(err, ErrIdempotencyConflict) || found {
		t.Fatalf("occupied dispatch command = found=%t, error=%v", found, err)
	}
	verdictCollision := verdictAttempt.selector
	verdictCollision.CommandID = "cmd-create"
	if _, found, err := fixture.control.ConvergeControlledVerdictAdmission(
		context.Background(), fixture.ownership, verdictCollision,
	); !errors.Is(err, ErrIdempotencyConflict) || found {
		t.Fatalf("occupied verdict command = found=%t, error=%v", found, err)
	}

	for name, mutate := range map[string]func(*ControlledVerdictAdmissionSelector){
		"submission": func(value *ControlledVerdictAdmissionSelector) {
			value.SubmissionID = "submission-foreign"
		},
		"epoch": func(value *ControlledVerdictAdmissionSelector) {
			value.VerificationEpoch++
		},
		"run": func(value *ControlledVerdictAdmissionSelector) {
			value.RunID = "run-foreign"
		},
		"work": func(value *ControlledVerdictAdmissionSelector) {
			value.WorkID = "work-foreign"
		},
	} {
		t.Run(name, func(t *testing.T) {
			selector := verdictAttempt.selector
			mutate(&selector)
			if _, found, err := fixture.control.ConvergeControlledVerdictAdmission(
				context.Background(), fixture.ownership, selector,
			); err == nil || found {
				t.Fatalf("drifted verdict selector = found=%t, error=%v", found, err)
			}
		})
	}
}

func TestControlledVerifierConvergenceRejectsIncompleteClosures(t *testing.T) {
	t.Run("missing dispatch identity", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		attempt := prepareVerifierDispatchConvergence(t, fixture, "cmd-corrupt-dispatch")
		applyVerifierDispatchConvergence(t, fixture, attempt)
		execVerifierConvergenceSQL(t, fixture.control, "DROP TRIGGER verifier_dispatch_records_no_delete")
		execVerifierConvergenceSQL(t, fixture.control,
			"DELETE FROM verifier_dispatch_records WHERE dispatch_id = ?", attempt.request.DispatchID)
		if _, found, err := fixture.control.ConvergeControlledVerifierDispatch(
			context.Background(), fixture.ownership, attempt.selector,
		); err == nil || found {
			t.Fatalf("missing dispatch identity = found=%t, error=%v", found, err)
		}
	})

	t.Run("missing verdict identity", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		fixture.dispatch(t, "cmd-corrupt-verdict-dispatch")
		fixture.execute(t, engine.VerdictFail, false)
		attempt := prepareVerdictConvergence(t, fixture, "cmd-corrupt-verdict")
		applyVerdictConvergence(t, fixture, attempt)
		execVerifierConvergenceSQL(t, fixture.control, "DROP TRIGGER verdict_records_no_delete")
		execVerifierConvergenceSQL(t, fixture.control,
			"DELETE FROM verdict_records WHERE verdict_id = ?", attempt.prepared.facts.VerdictID)
		if _, found, err := fixture.control.ConvergeControlledVerdictAdmission(
			context.Background(), fixture.ownership, attempt.selector,
		); err == nil || found {
			t.Fatalf("missing verdict identity = found=%t, error=%v", found, err)
		}
	})

	t.Run("corrupt verifier attempt witness", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		fixture.dispatch(t, "cmd-corrupt-witness-dispatch")
		fixture.execute(t, engine.VerdictFail, false)
		attempt := prepareVerdictConvergence(t, fixture, "cmd-corrupt-witness-verdict")
		applyVerdictConvergence(t, fixture, attempt)
		execVerifierConvergenceSQL(t, fixture.control, "DROP TRIGGER observations_no_update")
		execVerifierConvergenceSQL(t, fixture.control, `
			UPDATE effect_observations SET receipt_json = CAST('{}' AS BLOB)
			WHERE effect_id = ? AND kind = 'claimed'`, attempt.prepared.dispatchID)
		if _, found, err := fixture.control.ConvergeControlledVerdictAdmission(
			context.Background(), fixture.ownership, attempt.selector,
		); err == nil || found {
			t.Fatalf("corrupt verifier attempt witness = found=%t, error=%v", found, err)
		}
	})

	t.Run("corrupt assessment record", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		fixture.dispatch(t, "cmd-corrupt-assessment-dispatch")
		fixture.execute(t, engine.VerdictFail, false)
		attempt := prepareVerdictConvergence(t, fixture, "cmd-corrupt-assessment-verdict")
		applyVerdictConvergence(t, fixture, attempt)
		execVerifierConvergenceSQL(t, fixture.control, "DROP TRIGGER records_no_update")
		execVerifierConvergenceSQL(t, fixture.control,
			"UPDATE records SET canonical_json = CAST('{}' AS BLOB) WHERE digest = ?",
			attempt.prepared.assessmentDigest,
		)
		if _, found, err := fixture.control.ConvergeControlledVerdictAdmission(
			context.Background(), fixture.ownership, attempt.selector,
		); err == nil || found {
			t.Fatalf("corrupt assessment closure = found=%t, error=%v", found, err)
		}
	})

	t.Run("corrupt attention event", func(t *testing.T) {
		fixture := newVerifierLifecycleFixture(t)
		fixture.dispatch(t, "cmd-corrupt-attention-dispatch")
		fixture.execute(t, engine.VerdictPass, false)
		verdictAttempt := prepareVerdictConvergence(t, fixture, "cmd-corrupt-attention-verdict")
		commandID := "cmd-corrupt-pass-attention"
		command, err := PASSAttentionCommand(
			commandID, fixture.runID, fixture.workID, verdictAttempt.state.Revision,
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.control.ApplyControlledPASSAttention(
			context.Background(), fixture.ownership, fixture.ownerID, verdictAttempt.prepared, command,
		); err != nil {
			t.Fatal(err)
		}
		selector := passAttentionSelector(t, fixture, verdictAttempt, commandID)
		execVerifierConvergenceSQL(t, fixture.control, "DROP TRIGGER events_no_update")
		execVerifierConvergenceSQL(t, fixture.control,
			"UPDATE events SET data_json = CAST('{}' AS BLOB) WHERE command_id = ?", commandID)
		if _, found, err := fixture.control.ConvergeControlledPASSAttention(
			context.Background(), fixture.ownership, selector,
		); err == nil || found {
			t.Fatalf("corrupt attention closure = found=%t, error=%v", found, err)
		}
	})
}

func restartVerifierConvergenceStore(
	t *testing.T,
	fixture *verifierLifecycleFixture,
	successorID string,
) (*Store, *ControllerOwnership) {
	t.Helper()
	path := fixture.control.ControlPath()
	configuration := ControlConfiguration{
		LocalCheckRuntimeManifestDigest: fixture.control.localCheckRuntimeManifestDigest,
		BuilderDispatchDigest:           fixture.control.builderDispatchDigest,
		VerifierProfileDigest:           fixture.control.verifierProfileDigest,
		VerifierAgent:                   fixture.control.verifierAgent,
		Repository:                      fixture.control.repository,
	}
	if err := fixture.ownership.Close(); err != nil {
		t.Fatal(err)
	}
	fixture.ownership = nil
	if err := fixture.control.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenConfigured(context.Background(), path, configuration)
	if err != nil {
		t.Fatal(err)
	}
	successor, err := reopened.AcquireControllerOwnership(successorID)
	if err != nil {
		_ = reopened.Close()
		t.Fatal(err)
	}
	if err := successor.Activate(context.Background(), reopened, successorID); err != nil {
		_ = successor.Close()
		_ = reopened.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = successor.Close()
		_ = reopened.Close()
	})
	return reopened, successor
}

func execVerifierConvergenceSQL(t *testing.T, control *Store, query string, arguments ...any) {
	t.Helper()
	if _, err := control.db.Exec(query, arguments...); err != nil {
		t.Fatalf("execute verifier convergence SQL: %v", err)
	}
}
