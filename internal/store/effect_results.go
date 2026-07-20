package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
