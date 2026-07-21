package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
)

func validateBoundEffectResult(
	ctx context.Context,
	resolver journalResultResolver,
	effect Effect,
	result json.RawMessage,
) error {
	if err := engine.ValidateEffectResult(
		engine.EffectKind(effect.Kind), effect.ID, effect.Request, result,
	); err != nil {
		return fmt.Errorf("validate result for effect %q: %w", effect.ID, err)
	}
	if engine.EffectKind(effect.Kind) == engine.EffectLocalCheck {
		if err := validateLocalCheckResultClosure(ctx, resolver, effect, result); err != nil {
			return fmt.Errorf("validate result for effect %q: %w", effect.ID, err)
		}
	}
	if engine.EffectKind(effect.Kind) == engine.EffectVerifier {
		if err := validateVerifierResultClosure(ctx, resolver, effect, result); err != nil {
			return fmt.Errorf("validate result for effect %q: %w", effect.ID, err)
		}
	}
	return nil
}

func validateVerifierResultClosure(
	ctx context.Context,
	resolver journalResultResolver,
	effect Effect,
	encoded json.RawMessage,
) error {
	request, requestErr := engine.ParseVerifierEffectRequest(effect.Request)
	result, resultErr := engine.ParseVerifierEffectResult(encoded)
	if requestErr != nil || resultErr != nil || request.DeliveryRunID != effect.DeliveryRunID ||
		request.DispatchID != effect.ID || result.DispatchID != effect.ID ||
		request.VerificationEpoch != result.VerificationEpoch ||
		request.DispatchReceipt.Ref != request.DispatchReceipt.Digest ||
		result.Assessment.Ref != result.Assessment.Digest ||
		result.ExecutionReceipt.Ref != result.ExecutionReceipt.Digest {
		return errors.New("verifier result does not match its exact journal request")
	}
	identity, err := loadVerifierAttemptIdentity(ctx, resolver.query, effect)
	if err != nil {
		return err
	}
	dispatchBytes, err := protocol.ResolveArtifact(
		ctx, resolver, request.DispatchReceipt, protocol.MaximumControlReceiptBytes,
	)
	if err != nil {
		return fmt.Errorf("resolve verifier dispatch: %w", err)
	}
	dispatch, err := protocol.ParseVerifierDispatch(dispatchBytes)
	if err != nil || dispatch.DispatchID != effect.ID || dispatch.SubmissionDigest != request.SubmissionDigest ||
		dispatch.Candidate != request.Candidate {
		return errors.New("verifier dispatch does not match its effect request")
	}
	planKind, planBytes, err := loadRecord(ctx, resolver.query, request.PlanDigest)
	if err != nil || planKind != protocol.DeliveryPlanSchemaVersion {
		return errors.New("verifier execution lacks its exact delivery plan")
	}
	plan, err := protocol.ParseDeliveryPlan(planBytes)
	if err != nil || plan.Record().Digest != request.PlanDigest || plan.DeliveryID() != request.DeliveryID {
		return errors.New("verifier execution plan does not match its effect request")
	}
	submissionKind, submissionBytes, err := loadRecord(ctx, resolver.query, request.SubmissionDigest)
	if err != nil || submissionKind != protocol.SubmissionSchemaVersion {
		return errors.New("verifier execution lacks its exact submission")
	}
	submission, err := protocol.ParseSubmission(submissionBytes)
	if err != nil {
		return fmt.Errorf("parse verifier execution submission: %w", err)
	}
	submissionView := submission.View()
	if submission.Record().Digest != request.SubmissionDigest || submissionView.SubmissionID != request.SubmissionID ||
		submissionView.DeliveryID != request.DeliveryID || submissionView.WorkID != request.WorkID ||
		submissionView.Attempt != request.WorkAttempt || submissionView.Candidate != request.Candidate {
		return errors.New("verifier execution submission does not match its effect request")
	}
	review, err := protocol.ResolveExactVerifierReview(ctx, resolver, plan, submission)
	if err != nil {
		return fmt.Errorf("resolve exact verifier review: %w", err)
	}
	assessmentBytes, err := protocol.ResolveArtifact(
		ctx, resolver, result.Assessment, protocol.MaximumVerifierAssessmentBytes,
	)
	if err != nil {
		return fmt.Errorf("resolve verifier assessment: %w", err)
	}
	assessment, err := protocol.ParseVerifierAssessment(assessmentBytes)
	if err != nil {
		return fmt.Errorf("parse verifier assessment: %w", err)
	}
	kind, canonical, err := loadRecord(ctx, resolver.query, assessment.Record().Digest)
	if err != nil || kind != protocol.VerifierAssessmentSchemaVersion ||
		!bytes.Equal(canonical, assessment.Record().CanonicalJSON) {
		return errors.New("verifier assessment lacks its exact canonical record")
	}
	receiptBytes, err := protocol.ResolveArtifact(
		ctx, resolver, result.ExecutionReceipt, protocol.MaximumVerifierExecutionReceiptBytes,
	)
	if err != nil {
		return fmt.Errorf("resolve verifier execution receipt: %w", err)
	}
	receipt, err := protocol.ParseVerifierExecutionReceipt(receiptBytes)
	if err != nil {
		return fmt.Errorf("parse verifier execution receipt: %w", err)
	}
	profilePointer := protocol.Artifact{
		Ref: request.VerifierProfileDigest, MediaType: protocol.VerifierProfileMediaType,
		Digest: request.VerifierProfileDigest,
	}
	profileBytes, err := protocol.ResolveArtifact(
		ctx, resolver, profilePointer, protocol.MaximumVerifierProfileBytes,
	)
	if err != nil {
		return fmt.Errorf("resolve verifier profile: %w", err)
	}
	profile, err := protocol.ParseVerifierProfile(profileBytes)
	if err != nil {
		return fmt.Errorf("parse verifier profile: %w", err)
	}
	schemaBytes, err := protocol.VerifierAssessmentOutputSchema()
	if err != nil {
		return fmt.Errorf("derive verifier assessment schema: %w", err)
	}
	schemaPointer := protocol.Artifact{
		Ref: profile.OutputSchemaDigest, MediaType: protocol.VerifierAssessmentSchemaMediaType,
		Digest: profile.OutputSchemaDigest,
	}
	storedSchema, err := protocol.ResolveArtifact(
		ctx, resolver, schemaPointer, protocol.MaximumVerifierAssessmentBytes,
	)
	if err != nil || !bytes.Equal(storedSchema, schemaBytes) {
		return errors.New("verifier execution lacks its exact engine-owned assessment schema")
	}
	if err := validateVerifierReceiptBindings(
		request, result, identity, dispatchBytes, planBytes, submissionBytes,
		assessmentBytes, review, profile, schemaBytes, receipt,
	); err != nil {
		return err
	}
	var stdoutCapture []byte
	for label, capture := range map[string]protocol.CapturedArtifact{
		"stdout": receipt.Stdout,
		"stderr": receipt.Stderr,
	} {
		contents, err := protocol.ResolveArtifact(ctx, resolver, capture.Pointer(), uint64(capture.Size))
		if err != nil || int64(len(contents)) != capture.Size {
			return fmt.Errorf("resolve verifier execution %s capture: invalid exact capture", label)
		}
		if label == "stdout" {
			stdoutCapture = contents
		}
	}
	turn, err := protocol.ParseNativeCodexVerifierJSONL(stdoutCapture)
	if err != nil {
		return fmt.Errorf("parse verifier execution stdout capture: %w", err)
	}
	if !bytes.Equal(turn.Assessment, assessmentBytes) || turn.ThreadID != receipt.ThreadID {
		return errors.New("verifier execution stdout does not reproduce its exact assessment and thread")
	}
	journalStart := time.UnixMicro(effect.StartedAtUS).UTC().Format(time.RFC3339Nano)
	startOrder, startErr := protocol.CompareDateTimes(journalStart, result.StartedAt)
	dispatchOrder, dispatchErr := protocol.CompareDateTimes(dispatch.CreatedAt, result.StartedAt)
	if effect.StartedAtUS <= 0 || startErr != nil || dispatchErr != nil || startOrder > 0 || dispatchOrder > 0 {
		return errors.New("verifier result starts outside its dispatch and journal lease")
	}
	return nil
}

func validateVerifierReceiptBindings(
	request engine.VerifierEffectRequest,
	result engine.VerifierEffectResult,
	identity engine.VerifierAttemptIdentity,
	dispatchBytes, planBytes, submissionBytes, assessmentBytes []byte,
	review protocol.ExactVerifierReview,
	profile protocol.VerifierProfile,
	schemaBytes []byte,
	receipt protocol.VerifierExecutionReceipt,
) error {
	if receipt.EffectID != identity.EffectID || receipt.EffectAttempt != identity.EffectAttempt ||
		receipt.InvocationID != identity.InvocationID || receipt.DeliveryRunID != request.DeliveryRunID ||
		receipt.DeliveryID != request.DeliveryID || receipt.WorkID != request.WorkID ||
		receipt.WorkAttempt != request.WorkAttempt || receipt.PlanDigest != request.PlanDigest ||
		receipt.SubmissionID != request.SubmissionID || receipt.SubmissionDigest != request.SubmissionDigest ||
		receipt.Candidate != request.Candidate || receipt.DispatchID != request.DispatchID ||
		receipt.DispatchDigest != request.DispatchReceipt.Digest ||
		receipt.VerifierProfileDigest != request.VerifierProfileDigest || receipt.Agent != request.Agent ||
		receipt.VerificationEpoch != request.VerificationEpoch ||
		receipt.AssessmentDigest != result.Assessment.Digest || receipt.StartedAt != result.StartedAt ||
		receipt.CompletedAt != result.CompletedAt {
		return errors.New("verifier execution receipt does not match its journal request and result")
	}
	if profile.Agent != request.Agent || profile.RepositoryID != request.Candidate.Repository ||
		profile.ExecutorConfigurationDigest != receipt.ExecutorConfigurationDigest ||
		profile.ExecutableInput != receipt.ExecutableInput || profile.BinaryDigest != receipt.ExecutableDigest ||
		profile.Network != receipt.Network || profile.WorkspaceAccess != receipt.WorkspaceAccess ||
		profile.NestedSandbox != receipt.NestedSandbox || profile.CredentialAccess != receipt.CredentialAccess ||
		profile.ModelToolNetwork != receipt.ModelToolNetwork ||
		profile.ModelToolCredentialAccess != receipt.ModelToolCredentialAccess {
		return errors.New("verifier execution receipt does not match its exact profile")
	}
	expected := []protocol.VerifierExecutionInput{
		{Name: "assessment-schema", Digest: profile.OutputSchemaDigest, Size: uint64(len(schemaBytes))},
		{Name: profile.ExecutableInput, Digest: profile.BinaryDigest, Size: uint64(profile.BinarySize)},
		{Name: "dispatch", Digest: request.DispatchReceipt.Digest, Size: uint64(len(dispatchBytes))},
		{Name: "plan", Digest: request.PlanDigest, Size: uint64(len(planBytes))},
		{Name: "submission", Digest: request.SubmissionDigest, Size: uint64(len(submissionBytes))},
	}
	for _, input := range review.Inputs() {
		expected = append(expected, protocol.VerifierExecutionInput{
			Name: input.Name, Digest: input.Digest, Size: uint64(len(input.Contents)),
		})
	}
	slices.SortFunc(expected, func(left, right protocol.VerifierExecutionInput) int {
		return strings.Compare(left.Name, right.Name)
	})
	if !slices.Equal(receipt.Inputs, expected) {
		return errors.New("verifier execution receipt does not bind its exact review input closure")
	}
	if protocol.RawDigest(assessmentBytes) != receipt.AssessmentDigest {
		return errors.New("verifier execution receipt does not bind its raw assessment")
	}
	return nil
}

// validateVerifierCompletionWindow prevents a syntactically valid future
// review interval from becoming terminal journal truth. Binding may occur while
// an effect is still running, so the upper bound belongs at completion and
// bound-result recovery, where the exact durable completion time is known.
func validateVerifierCompletionWindow(effect Effect, completedAtUS int64) error {
	if engine.EffectKind(effect.Kind) != engine.EffectVerifier {
		return nil
	}
	result, err := engine.ParseVerifierEffectResult(effect.Result)
	if err != nil {
		return err
	}
	completed := cloneEffect(effect)
	completed.CompletedAtUS = completedAtUS
	if !journalContains(completed, result.StartedAt, result.CompletedAt) {
		return errors.New("verifier review timestamps fall outside its completed journal lease")
	}
	return nil
}

func validateLocalCheckResultClosure(
	ctx context.Context,
	resolver journalResultResolver,
	effect Effect,
	encoded json.RawMessage,
) error {
	request, _ := engine.ParseLocalCheckEffectRequest(effect.Request)
	result, _ := engine.ParseLocalCheckEffectResult(encoded)
	if request.DeliveryRunID != effect.DeliveryRunID || request.BuilderEffectID == effect.ID {
		return errors.New("local check request does not match its delivery journal")
	}
	builder, err := resolver.SucceededEffect(ctx, request.BuilderEffectID)
	if err != nil {
		return fmt.Errorf("resolve builder effect: %w", err)
	}
	if builder.Kind != engine.EffectBuild || builder.DeliveryRunID != effect.DeliveryRunID {
		return errors.New("local check builder belongs to a different journal")
	}
	if err := engine.ValidateEffectResult(builder.Kind, builder.ID, builder.Request, builder.Result); err != nil {
		return fmt.Errorf("validate builder effect: %w", err)
	}
	buildRequest, _ := engine.ParseBuildEffectRequest(builder.Request)
	buildResult, _ := engine.ParseBuildEffectResult(builder.Result)
	if buildRequest.DeliveryRunID != request.DeliveryRunID || buildRequest.DeliveryID != request.DeliveryID ||
		buildRequest.WorkID != request.WorkID || buildRequest.WorkAttempt != request.WorkAttempt {
		return errors.New("local check request does not match its builder attempt")
	}
	receiptBytes, err := protocol.ResolveArtifact(ctx, resolver, result.Receipt, protocol.MaximumLocalCheckReceiptBytes)
	if err != nil {
		return fmt.Errorf("resolve local check receipt: %w", err)
	}
	receipt, err := protocol.ParseLocalCheckReceipt(receiptBytes)
	if err != nil {
		return err
	}
	definition := protocol.Artifact{
		Ref: request.DefinitionDigest, MediaType: "application/json", Digest: request.DefinitionDigest,
	}
	if receipt.RunID != effect.ID || receipt.CheckID != request.CheckID || receipt.Definition != definition ||
		receipt.Candidate.Repository != buildResult.Candidate.RepositoryID ||
		receipt.Candidate.Commit != buildResult.Candidate.Commit || receipt.Candidate.Tree != buildResult.Candidate.Tree {
		return errors.New("local check receipt does not match its request and builder result")
	}
	definitionBytes, err := protocol.ResolveArtifact(ctx, resolver, definition, protocol.MaximumLocalCheckDefinitionBytes)
	if err != nil {
		return err
	}
	parsedDefinition, err := protocol.ParseLocalCheckDefinition(definitionBytes)
	if err != nil {
		return err
	}
	if receipt.WorkingDirectory != parsedDefinition.WorkingDirectory ||
		receipt.TimeoutSeconds != parsedDefinition.TimeoutSeconds || !slices.Equal(receipt.Argv, parsedDefinition.Argv) {
		return errors.New("local check receipt does not match its exact definition")
	}
	environmentBytes, err := protocol.ResolveArtifact(ctx, resolver, protocol.Artifact{
		Ref: receipt.Environment.Ref, MediaType: protocol.LocalEnvironmentMediaType, Digest: receipt.Environment.Ref,
	}, protocol.MaximumLocalEnvironmentBytes)
	if err != nil {
		return err
	}
	environment, err := protocol.ParseLocalEnvironment(environmentBytes)
	if err != nil {
		return err
	}
	if environment.SchemaVersion != protocol.ContentEnvironmentSchemaVersion ||
		environment.RuntimeManifestDigest != request.RuntimeManifestDigest {
		return errors.New("local check environment does not bind the requested content runtime")
	}
	if time.Duration(parsedDefinition.TimeoutSeconds)*time.Second > time.Duration(environment.Limits.RuntimeNanoseconds) {
		return errors.New("local check definition timeout exceeds its measured environment")
	}
	for _, observed := range []struct {
		name    string
		capture protocol.CapturedArtifact
		limit   int64
	}{
		{name: "stdout", capture: receipt.Stdout, limit: environment.Limits.StdoutBytes},
		{name: "stderr", capture: receipt.Stderr, limit: environment.Limits.StderrBytes},
	} {
		if observed.capture.Size > observed.limit {
			return fmt.Errorf("local check %s capture exceeds its measured environment", observed.name)
		}
		contents, err := protocol.ResolveArtifact(
			ctx, resolver, observed.capture.Pointer(), uint64(observed.capture.Size),
		)
		if err != nil {
			return fmt.Errorf("resolve local check %s capture: %w", observed.name, err)
		}
		if int64(len(contents)) != observed.capture.Size {
			return fmt.Errorf("local check %s capture size does not match its receipt", observed.name)
		}
	}
	expected := engine.LocalCheckOutcomeNotAdmitted
	if receipt.Outcome == "pass" {
		expected = engine.LocalCheckOutcomePass
	} else if receipt.Cancelled || receipt.TimedOut || receipt.OutputTruncated {
		expected = engine.LocalCheckOutcomeControlled
	}
	if result.Outcome != expected {
		return errors.New("local check semantic outcome does not match its receipt")
	}
	return nil
}

type journalResultResolver struct{ query rowQuerier }

func (resolver journalResultResolver) Artifact(
	ctx context.Context,
	artifactDigest string,
) (string, []byte, error) {
	return loadArtifact(ctx, resolver.query, artifactDigest)
}

func (resolver journalResultResolver) SucceededEffect(
	ctx context.Context,
	effectID string,
) (engine.JournalEffect, error) {
	effect, err := loadEffect(ctx, resolver.query, effectID)
	if err != nil {
		return engine.JournalEffect{}, err
	}
	if effect.State != EffectSucceeded {
		return engine.JournalEffect{}, fmt.Errorf("effect %q is %s, want succeeded", effectID, effect.State)
	}
	if len(effect.Result) == 0 {
		return engine.JournalEffect{}, fmt.Errorf("effect %q has an invalid durable result binding", effectID)
	}
	if err := validateBoundEffectResult(ctx, resolver, effect, effect.Result); err != nil {
		return engine.JournalEffect{}, err
	}
	return engine.JournalEffect{
		ID: effect.ID, DeliveryRunID: effect.DeliveryRunID, Kind: engine.EffectKind(effect.Kind),
		Attempt: effect.Attempt, Request: append(json.RawMessage(nil), effect.Request...),
		Result: append(json.RawMessage(nil), effect.Result...),
	}, nil
}
