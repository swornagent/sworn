// Package effects executes and reconciles typed external effects without
// owning delivery state or journal lifecycle.
package effects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/producer"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

// ResultResolver exposes only immutable facts needed to validate or execute an
// effect. Store leases and lifecycle transitions remain internal/store-owned.
type ResultResolver interface {
	SucceededEffect(context.Context, string) (engine.JournalEffect, error)
}

type LocalCheckControl interface {
	ResultResolver
	Artifact(context.Context, string) (mediaType string, contents []byte, err error)
	PutArtifact(context.Context, string, []byte) (string, error)
}

// LocalCheckWorker is the thin adapter from one journal request to the
// measurement-only producer. Runtime source identity and paths remain opaque.
type LocalCheckWorker struct {
	Control           LocalCheckControl
	Runner            producer.Runner
	Repository        *repo.Repository
	Runtime           executor.RuntimeTree
	WorkspaceRoot     string
	MaterializeLimits repo.MaterializeLimits
}

func (worker LocalCheckWorker) Run(
	ctx context.Context,
	effect engine.JournalEffect,
) (json.RawMessage, error) {
	if worker.Control == nil || worker.Runner == nil || worker.Repository == nil {
		return nil, errors.New("local check worker requires control, runner, and repository")
	}
	if effect.Kind != engine.EffectLocalCheck || !engine.ValidID(effect.ID) ||
		effect.Attempt < 1 || len(effect.Result) != 0 {
		return nil, errors.New("local check worker requires one unresolved claimed check effect")
	}
	request, err := engine.ParseLocalCheckEffectRequest(effect.Request)
	if err != nil {
		return nil, err
	}
	if request.DeliveryRunID != effect.DeliveryRunID || worker.Runtime.Digest() != request.RuntimeManifestDigest {
		return nil, errors.New("local check effect does not match its journal or configured runtime")
	}
	if err := validateWorkspaceRoot(worker.WorkspaceRoot); err != nil {
		return nil, err
	}
	_, build, err := resolveBuilder(ctx, worker.Control, effect, request)
	if err != nil {
		return nil, err
	}
	invocationRoot, err := os.MkdirTemp(worker.WorkspaceRoot, "sworn-check-")
	if err != nil {
		return nil, fmt.Errorf("create local check workspace root: %w", err)
	}
	invocationIdentity, err := os.Lstat(invocationRoot)
	if err != nil || !invocationIdentity.IsDir() {
		_ = os.Remove(invocationRoot)
		return nil, errors.New("local check workspace identity is invalid")
	}
	workspace, runErr := worker.Repository.MaterializeCandidate(
		ctx, build.Candidate, filepath.Join(invocationRoot, "candidate"), worker.MaterializeLimits,
	)
	var produced producer.Result
	if runErr == nil {
		produced, runErr = producer.RunLocalContentBound(ctx, worker.Runner, worker.Control, producer.Request{
			CheckID: request.CheckID,
			RunID:   effect.ID,
			Definition: protocol.Artifact{
				Ref: request.DefinitionDigest, MediaType: "application/json", Digest: request.DefinitionDigest,
			},
			Repository: worker.Repository,
			Candidate:  build.Candidate,
			Workspace:  workspace,
		}, worker.Runtime)
	}
	cleanupErr := removeWorkspaceRoot(invocationRoot, invocationIdentity)
	if cleanupErr != nil {
		return nil, fmt.Errorf("remove local check workspace: %w", cleanupErr)
	}
	outcome := engine.LocalCheckOutcomePass
	switch {
	case errors.Is(runErr, producer.ErrCheckNotAdmitted):
		outcome = engine.LocalCheckOutcomeNotAdmitted
	case errors.Is(runErr, producer.ErrCheckControlled):
		outcome = engine.LocalCheckOutcomeControlled
	case runErr != nil:
		return nil, runErr
	}
	encoded, err := engine.EncodeLocalCheckEffectResult(engine.LocalCheckEffectResult{
		SchemaVersion: engine.LocalCheckEffectResultSchemaVersion,
		Outcome:       outcome,
		Receipt:       produced.Receipt,
	})
	if err != nil {
		return nil, err
	}
	effect.Result = encoded
	if err := engine.ValidateEffectResult(effect.Kind, effect.ID, effect.Request, effect.Result); err != nil {
		return nil, fmt.Errorf("validate measured local check result shape: %w", err)
	}
	return encoded, nil
}

func validateWorkspaceRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("local check workspace root must be a clean absolute path")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return errors.New("local check workspace root contains a symbolic-link remap")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !workspaceRootOwnedByCurrentUser(info) {
		return errors.New("local check workspace root must be an existing private directory")
	}
	return nil
}

func removeWorkspaceRoot(root string, identity os.FileInfo) error {
	current, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("revalidate local check workspace identity: %w", err)
	}
	if !current.IsDir() || !os.SameFile(identity, current) {
		return errors.New("local check workspace identity changed before cleanup")
	}
	return os.RemoveAll(root)
}

func resolveBuilder(
	ctx context.Context,
	resolver ResultResolver,
	check engine.JournalEffect,
	request engine.LocalCheckEffectRequest,
) (engine.BuildEffectRequest, engine.BuildEffectResult, error) {
	if request.BuilderEffectID == check.ID {
		return engine.BuildEffectRequest{}, engine.BuildEffectResult{}, errors.New("local check cannot use itself as builder")
	}
	builder, err := resolver.SucceededEffect(ctx, request.BuilderEffectID)
	if err != nil {
		return engine.BuildEffectRequest{}, engine.BuildEffectResult{}, fmt.Errorf("resolve builder effect: %w", err)
	}
	if builder.Kind != engine.EffectBuild || builder.DeliveryRunID != check.DeliveryRunID {
		return engine.BuildEffectRequest{}, engine.BuildEffectResult{}, errors.New("local check builder belongs to a different journal")
	}
	if err := engine.ValidateEffectResult(builder.Kind, builder.ID, builder.Request, builder.Result); err != nil {
		return engine.BuildEffectRequest{}, engine.BuildEffectResult{}, fmt.Errorf("validate builder effect: %w", err)
	}
	buildRequest, _ := engine.ParseBuildEffectRequest(builder.Request)
	buildResult, _ := engine.ParseBuildEffectResult(builder.Result)
	if buildRequest.DeliveryRunID != request.DeliveryRunID || buildRequest.DeliveryID != request.DeliveryID ||
		buildRequest.WorkID != request.WorkID || buildRequest.WorkAttempt != request.WorkAttempt {
		return engine.BuildEffectRequest{}, engine.BuildEffectResult{}, errors.New("local check request does not match its builder attempt")
	}
	return buildRequest, buildResult, nil
}
