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
	"github.com/swornagent/sworn/internal/store"
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

type contentBoundReconciler interface {
	ReconcileContentBound(context.Context, string) (executor.ContentBoundCleanup, error)
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

// ValidateConfiguration closes the restart-recovery boundary before a worker
// is admitted to a controller. A runner which can execute checks but cannot
// prove content-bound quiescence is not a production check worker.
func (worker LocalCheckWorker) ValidateConfiguration() error {
	if worker.Control == nil || worker.Runner == nil || worker.Repository == nil {
		return errors.New("local check worker requires control, runner, and repository")
	}
	if worker.Runtime.Digest() == "" {
		return errors.New("local check worker requires an exact content runtime")
	}
	if _, ok := worker.Runner.(contentBoundReconciler); !ok {
		return errors.New("local check worker runner cannot reconcile content-bound attempts")
	}
	if err := validateWorkspaceRoot(worker.WorkspaceRoot); err != nil {
		return err
	}
	if worker.MaterializeLimits.Bytes == 0 || worker.MaterializeLimits.Entries == 0 {
		return errors.New("local check worker requires materialization ceilings")
	}
	return nil
}

func (worker LocalCheckWorker) Run(
	ctx context.Context,
	capability store.PreparedAuthorizedCheckLease,
) (json.RawMessage, error) {
	result, err := capability.RunCheck(func(effect engine.JournalEffect) (json.RawMessage, error) {
		return worker.run(ctx, effect)
	})
	if err != nil {
		return nil, fmt.Errorf("run check capability: %w", err)
	}
	return result, nil
}

func (worker LocalCheckWorker) run(
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
	attempt, err := engine.CheckAttemptIdentityFor(effect.ID, effect.Attempt, request.RuntimeManifestDigest)
	if err != nil {
		return nil, err
	}
	if err := validateWorkspaceRoot(worker.WorkspaceRoot); err != nil {
		return nil, err
	}
	if _, ok := worker.Runner.(contentBoundReconciler); !ok {
		return nil, errors.New("local check worker runner cannot reconcile content-bound attempts")
	}
	if worker.MaterializeLimits.Bytes == 0 || worker.MaterializeLimits.Entries == 0 {
		return nil, errors.New("local check worker requires materialization ceilings")
	}
	_, build, err := resolveBuilder(ctx, worker.Control, effect, request)
	if err != nil {
		return nil, err
	}
	invocationRoot := localCheckWorkspacePath(worker.WorkspaceRoot, attempt.InvocationID)
	if err := os.Mkdir(invocationRoot, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("local check invocation %q has unreconciled workspace residue", attempt.InvocationID)
		}
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
			CheckID:      request.CheckID,
			RunID:        effect.ID,
			InvocationID: attempt.InvocationID,
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

// ReconcileUnbound proves the exact content-bound subprocess is quiescent,
// removes only its deterministic candidate materialization, and returns the
// executor's opaque cleanup proof. Store recovery must still bind this proof to
// its own exact unknown-attempt capability before requeueing anything.
func (worker LocalCheckWorker) ReconcileUnbound(
	ctx context.Context,
	capability store.CheckRecoveryLease,
) (store.CheckRetryProof, error) {
	proof, err := capability.ReconcileCheck(func(effect engine.JournalEffect) (executor.ContentBoundCleanup, error) {
		return worker.reconcileUnbound(ctx, effect)
	})
	if err != nil {
		return store.CheckRetryProof{}, fmt.Errorf("run check reconciliation capability: %w", err)
	}
	return proof, nil
}

func (worker LocalCheckWorker) reconcileUnbound(
	ctx context.Context,
	effect engine.JournalEffect,
) (executor.ContentBoundCleanup, error) {
	reconciler, ok := worker.Runner.(contentBoundReconciler)
	if !ok || worker.Runtime.Digest() == "" {
		return executor.ContentBoundCleanup{}, errors.New("local check recovery requires a content-bound reconciler and runtime")
	}
	if effect.Kind != engine.EffectLocalCheck || !engine.ValidID(effect.ID) ||
		effect.Attempt < 1 || len(effect.Result) != 0 {
		return executor.ContentBoundCleanup{}, errors.New("local check recovery requires one unbound interrupted check effect")
	}
	request, err := engine.ParseLocalCheckEffectRequest(effect.Request)
	if err != nil {
		return executor.ContentBoundCleanup{}, err
	}
	if request.DeliveryRunID != effect.DeliveryRunID || worker.Runtime.Digest() != request.RuntimeManifestDigest {
		return executor.ContentBoundCleanup{}, errors.New("local check recovery does not match its journal or configured runtime")
	}
	attempt, err := engine.CheckAttemptIdentityFor(effect.ID, effect.Attempt, request.RuntimeManifestDigest)
	if err != nil {
		return executor.ContentBoundCleanup{}, err
	}
	if err := validateWorkspaceRoot(worker.WorkspaceRoot); err != nil {
		return executor.ContentBoundCleanup{}, err
	}
	cleanup, err := reconciler.ReconcileContentBound(ctx, attempt.InvocationID)
	if err != nil {
		return executor.ContentBoundCleanup{}, fmt.Errorf("reconcile local check executor: %w", err)
	}
	if cleanup.InvocationID() != attempt.InvocationID {
		return executor.ContentBoundCleanup{}, errors.New("local check executor cleanup does not match its attempt")
	}
	invocationRoot := localCheckWorkspacePath(worker.WorkspaceRoot, attempt.InvocationID)
	identity, err := os.Lstat(invocationRoot)
	if err == nil {
		if !identity.IsDir() || identity.Mode()&os.ModeSymlink != 0 {
			return executor.ContentBoundCleanup{}, errors.New("local check recovery workspace identity is invalid")
		}
		if err := removeWorkspaceRoot(invocationRoot, identity); err != nil {
			return executor.ContentBoundCleanup{}, fmt.Errorf("remove interrupted local check workspace: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return executor.ContentBoundCleanup{}, fmt.Errorf("inspect interrupted local check workspace: %w", err)
	}
	if _, err := os.Lstat(invocationRoot); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return executor.ContentBoundCleanup{}, fmt.Errorf("recheck interrupted local check workspace: %w", err)
		}
		return executor.ContentBoundCleanup{}, errors.New("interrupted local check workspace remains after cleanup")
	}
	return cleanup, nil
}

func localCheckWorkspacePath(root, invocationID string) string {
	return filepath.Join(root, invocationID)
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
