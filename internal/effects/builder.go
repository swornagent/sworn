package effects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
	"github.com/swornagent/sworn/internal/workspace"
)

const (
	builderDispatchSchemaVersion = "sworn-builder-dispatch-v1"
	builderProfileSchemaVersion  = "sworn-builder-profile-v2"
)

// BuilderControl exposes only the durable state and exact plan needed to
// execute one already-claimed builder effect.
type BuilderControl interface {
	State(context.Context, string) (engine.State, error)
	Plan(context.Context, string) (protocol.ExactPlan, error)
}

// BuilderRunner is the narrow contained-executor projection used by the
// builder worker. ReconcileWritable returns only after the runner has validated
// its own opaque cleanup proof for the requested invocation.
type BuilderRunner interface {
	ConfigurationDigest() string
	EffectiveLimits() executor.Limits
	RunWritable(context.Context, executor.Invocation) (executor.RawCompletion, error)
	ValidateExport(context.Context, executor.WorkspaceExport) error
	DiscardExport(context.Context, executor.WorkspaceExport) error
	ReconcileWritable(context.Context, string) (executor.WritableCleanup, error)
}

// BuilderCompletionPolicy is the adapter-owned semantic completion boundary.
// Its digest is part of the immutable builder profile; the policy may inspect
// bounded raw process output but cannot replace measured Git candidate truth.
type BuilderCompletionPolicy interface {
	BuilderProfileDigest() string
	ValidateBuilderCompletion(executor.RawCompletion) error
}

// BuilderWorker executes one Store-authorized builder operation. It never
// claims an effect, binds a result, publishes a Git ref, or completes journal
// state. Every effectful entry point consumes one narrow Store-issued
// capability before it can reach an executor, Git, or attempt workspace.
type BuilderWorker struct {
	Control          BuilderControl
	Runner           BuilderRunner
	Repository       *repo.Repository
	WorkspaceRoot    string
	Agent            string
	Argv             []string
	Environment      map[string]string
	Timeout          time.Duration
	ExecutableInput  *executor.Input
	Network          executor.NetworkMode
	NestedSandbox    bool
	CompletionPolicy BuilderCompletionPolicy
}

type builderExecutableInput struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type builderProfile struct {
	SchemaVersion               string                   `json:"schema_version"`
	ExecutorConfigurationDigest string                   `json:"executor_configuration_digest"`
	WorkspaceRoot               string                   `json:"workspace_root"`
	Repository                  repo.Binding             `json:"repository"`
	Agent                       string                   `json:"agent"`
	Argv                        []string                 `json:"argv"`
	EnvironmentNames            []string                 `json:"environment_names"`
	TimeoutNanoseconds          int64                    `json:"timeout_nanoseconds"`
	Network                     executor.NetworkMode     `json:"network"`
	WorkspaceAccess             executor.WorkspaceAccess `json:"workspace_access"`
	NestedSandbox               bool                     `json:"nested_sandbox"`
	ExecutableInput             *builderExecutableInput  `json:"executable_input,omitempty"`
	CompletionPolicyDigest      string                   `json:"completion_policy_digest,omitempty"`
}

type buildConfiguration struct {
	digest           string
	argv             []string
	environment      map[string]string
	limits           executor.Limits
	executableInput  *executor.Input
	network          executor.NetworkMode
	nestedSandbox    bool
	completionPolicy BuilderCompletionPolicy
}

// DispatchDigest binds every process-local choice which can change builder
// execution. The effect request must carry this digest exactly.
func (worker BuilderWorker) DispatchDigest() (string, error) {
	configuration, err := worker.configuration()
	if err != nil {
		return "", err
	}
	return configuration.digest, nil
}

func (worker BuilderWorker) configuration() (buildConfiguration, error) {
	if worker.Runner == nil || worker.Repository == nil {
		return buildConfiguration{}, errors.New("builder worker requires a runner and repository")
	}
	if err := validateWorkspaceRoot(worker.WorkspaceRoot); err != nil {
		return buildConfiguration{}, fmt.Errorf("validate builder workspace root: %w", err)
	}
	if !protocol.ValidNonEmpty(worker.Agent) || len(worker.Agent) > 512 {
		return buildConfiguration{}, errors.New("builder worker requires a bounded agent identity")
	}
	var executableInput *executor.Input
	if worker.ExecutableInput == nil {
		if err := executor.ValidateArgv(worker.Argv); err != nil {
			return buildConfiguration{}, fmt.Errorf("validate builder argv: %w", err)
		}
	} else {
		selected := *worker.ExecutableInput
		if selected.Name == "dispatch" || selected.Name == "plan" {
			return buildConfiguration{}, errors.New("builder executable input collides with an engine input")
		}
		if !filepath.IsAbs(selected.Path) || filepath.Clean(selected.Path) != selected.Path ||
			!engine.ValidDigest(selected.Digest) {
			return buildConfiguration{}, errors.New("builder executable input is not exact")
		}
		if err := executor.ValidateExecutableArgv(selected.Name, worker.Argv); err != nil {
			return buildConfiguration{}, fmt.Errorf("validate builder argv: %w", err)
		}
		executableInput = &selected
	}
	limits := worker.Runner.EffectiveLimits()
	if err := limits.Validate(); err != nil {
		return buildConfiguration{}, fmt.Errorf("validate builder executor limits: %w", err)
	}
	if worker.Timeout <= 0 || worker.Timeout > limits.Runtime {
		return buildConfiguration{}, errors.New("builder timeout is absent or exceeds the executor ceiling")
	}
	executorDigest := worker.Runner.ConfigurationDigest()
	if !engine.ValidDigest(executorDigest) {
		return buildConfiguration{}, errors.New("builder runner lacks an exact configuration digest")
	}
	binding := worker.Repository.Binding()
	if err := binding.Validate(); err != nil {
		return buildConfiguration{}, fmt.Errorf("validate builder repository binding: %w", err)
	}
	network := worker.Network
	if network == "" {
		network = executor.NetworkNone
	}
	if network != executor.NetworkNone && network != executor.NetworkHost {
		return buildConfiguration{}, errors.New("builder network mode is invalid")
	}
	completionPolicyDigest := ""
	if worker.CompletionPolicy != nil {
		completionPolicyDigest = worker.CompletionPolicy.BuilderProfileDigest()
		if !engine.ValidDigest(completionPolicyDigest) {
			return buildConfiguration{}, errors.New("builder completion policy lacks an exact profile digest")
		}
	}
	profile := builderProfile{
		SchemaVersion:               builderProfileSchemaVersion,
		ExecutorConfigurationDigest: executorDigest,
		WorkspaceRoot:               worker.WorkspaceRoot,
		Repository:                  binding,
		Agent:                       worker.Agent,
		Argv:                        slices.Clone(worker.Argv),
		EnvironmentNames:            sortedEnvironmentNames(worker.Environment),
		TimeoutNanoseconds:          worker.Timeout.Nanoseconds(),
		Network:                     network,
		WorkspaceAccess:             executor.WorkspaceWritableExport,
		NestedSandbox:               worker.NestedSandbox,
		CompletionPolicyDigest:      completionPolicyDigest,
	}
	if executableInput != nil {
		profile.ExecutableInput = &builderExecutableInput{
			Name: executableInput.Name, Path: executableInput.Path, Digest: executableInput.Digest,
		}
	}
	canonical, err := protocol.EncodeCanonical(profile)
	if err != nil {
		return buildConfiguration{}, fmt.Errorf("encode builder profile: %w", err)
	}
	return buildConfiguration{
		digest: protocol.RawDigest(canonical), argv: profile.Argv,
		environment: cloneEnvironment(worker.Environment), limits: limits,
		executableInput: executableInput, network: network,
		nestedSandbox: worker.NestedSandbox, completionPolicy: worker.CompletionPolicy,
	}, nil
}

type resolvedBuild struct {
	request  engine.BuildEffectRequest
	identity engine.BuildAttemptIdentity
	state    engine.State
	plan     protocol.ExactPlan
	work     protocol.ExactWorkContract
	target   repo.Target
}

func (worker BuilderWorker) resolveBuild(
	ctx context.Context,
	effect engine.JournalEffect,
	configuration buildConfiguration,
	requireActive bool,
) (resolvedBuild, error) {
	if worker.Control == nil {
		return resolvedBuild{}, errors.New("builder worker requires an exact control Store")
	}
	if effect.Kind != engine.EffectBuild || !engine.ValidID(effect.ID) ||
		!protocol.ValidPositiveSafeInteger(effect.Attempt) {
		return resolvedBuild{}, errors.New("builder worker requires one claimed build effect")
	}
	request, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil {
		return resolvedBuild{}, err
	}
	if request.SchemaVersion != engine.BuildEffectRequestSchemaVersion ||
		request.DeliveryRunID != effect.DeliveryRunID ||
		request.BuilderDispatchDigest != configuration.digest {
		return resolvedBuild{}, errors.New("build effect does not match its journal or configured dispatch")
	}
	identity, err := engine.BuildAttemptIdentityFor(
		effect.ID, effect.Attempt, request.BuilderDispatchDigest,
	)
	if err != nil {
		return resolvedBuild{}, err
	}
	state, err := worker.Control.State(ctx, effect.DeliveryRunID)
	if err != nil {
		return resolvedBuild{}, fmt.Errorf("load build delivery state: %w", err)
	}
	if state.RunID != effect.DeliveryRunID || state.DeliveryID != request.DeliveryID ||
		state.PlanDigest == "" || state.Repository != worker.Repository.Binding().RepositoryID {
		return resolvedBuild{}, errors.New("build delivery state does not match its request or repository")
	}
	plan, err := worker.Control.Plan(ctx, state.PlanDigest)
	if err != nil {
		return resolvedBuild{}, fmt.Errorf("load exact build plan: %w", err)
	}
	planRecord, targetView := plan.Record(), plan.Target()
	if planRecord.Kind != protocol.DeliveryPlanSchemaVersion || planRecord.Digest != state.PlanDigest ||
		plan.DeliveryID() != state.DeliveryID || targetView.Repository != state.Repository ||
		targetView.Ref != state.TargetRef {
		return resolvedBuild{}, errors.New("exact build plan does not match delivery state")
	}
	work, exists := plan.Work(request.WorkID)
	if !exists || work.Digest() != request.DispatchDigest {
		return resolvedBuild{}, errors.New("build work is absent from the exact plan")
	}
	workIDs := plan.WorkIDs()
	if len(workIDs) != len(state.Work) {
		return resolvedBuild{}, errors.New("build state work does not match the exact plan")
	}
	matched := false
	for index, workID := range workIDs {
		if state.Work[index].ID != workID {
			return resolvedBuild{}, errors.New("build state work order does not match the exact plan")
		}
		if workID == request.WorkID {
			matched = state.Work[index].Attempt == request.WorkAttempt
			if requireActive {
				matched = matched && state.Work[index].State == engine.WorkActive
			}
		}
	}
	if !matched {
		return resolvedBuild{}, errors.New("build request does not match the current work attempt")
	}
	target, err := worker.Repository.BindTarget(ctx, state.TargetRef)
	if err != nil {
		return resolvedBuild{}, fmt.Errorf("bind exact build target: %w", err)
	}
	return resolvedBuild{
		request: request, identity: identity, state: state, plan: plan, work: work, target: target,
	}, nil
}

type builderDispatch struct {
	SchemaVersion         string `json:"schema_version"`
	BuilderRunID          string `json:"builder_run_id"`
	InvocationID          string `json:"invocation_id"`
	DeliveryRunID         string `json:"delivery_run_id"`
	DeliveryID            string `json:"delivery_id"`
	PlanDigest            string `json:"plan_digest"`
	WorkID                string `json:"work_id"`
	WorkAttempt           int64  `json:"work_attempt"`
	ContractDigest        string `json:"contract_digest"`
	BuilderDispatchDigest string `json:"builder_dispatch_digest"`
	RepositoryID          string `json:"repository_id"`
	TargetRef             string `json:"target_ref"`
	BaseCommit            string `json:"base_commit"`
	BaseTree              string `json:"base_tree"`
}

// Run consumes one prepared execution capability, then executes and measures
// its exact build without publishing Git or mutating the effect journal. All
// executor and local attempt resources are removed before a result is returned.
func (worker BuilderWorker) Run(
	ctx context.Context,
	capability store.PreparedAuthorizedBuildLease,
) (json.RawMessage, error) {
	result, err := capability.RunBuilder(func(effect engine.JournalEffect) (json.RawMessage, error) {
		return worker.run(ctx, effect)
	})
	if err != nil {
		return nil, fmt.Errorf("run builder capability: %w", err)
	}
	return result, nil
}

// run contains the raw execution algorithm behind Run's one-shot Store gate.
func (worker BuilderWorker) run(
	ctx context.Context,
	effect engine.JournalEffect,
) (result json.RawMessage, resultErr error) {
	configuration, err := worker.configuration()
	if err != nil {
		return nil, err
	}
	if len(effect.Result) != 0 {
		return nil, errors.New("builder worker requires an unresolved build effect")
	}
	build, err := worker.resolveBuild(ctx, effect, configuration, true)
	if err != nil {
		return nil, err
	}
	attemptRoot, attemptIdentity, err := createBuildAttemptRoot(worker.WorkspaceRoot, build.identity.InvocationID)
	if err != nil {
		return nil, err
	}
	invoked := false
	var export *executor.WorkspaceExport
	defer func() {
		cleanupErr := worker.cleanupRun(ctx, build.identity.InvocationID, invoked, export, attemptRoot, attemptIdentity)
		if cleanupErr != nil {
			result = nil
			if resultErr == nil {
				resultErr = cleanupErr
			} else {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()

	base, err := worker.Repository.Materialize(ctx, build.target, filepath.Join(attemptRoot, "base"))
	if err != nil {
		return nil, fmt.Errorf("materialize exact builder base: %w", err)
	}
	manifest, _, err := workspace.Measure(ctx, base.Path, configuration.limits.InputBytes)
	if err != nil {
		return nil, fmt.Errorf("measure exact builder base: %w", err)
	}
	inputsRoot := filepath.Join(attemptRoot, "inputs")
	if err := os.Mkdir(inputsRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create builder input root: %w", err)
	}
	planRecord := build.plan.Record()
	dispatchBytes, err := protocol.EncodeCanonical(builderDispatch{
		SchemaVersion: builderDispatchSchemaVersion,
		BuilderRunID:  effect.ID, InvocationID: build.identity.InvocationID,
		DeliveryRunID: effect.DeliveryRunID, DeliveryID: build.state.DeliveryID,
		PlanDigest: planRecord.Digest, WorkID: build.request.WorkID,
		WorkAttempt: build.request.WorkAttempt, ContractDigest: build.request.DispatchDigest,
		BuilderDispatchDigest: build.request.BuilderDispatchDigest,
		RepositoryID:          build.target.RepositoryID, TargetRef: build.target.Ref,
		BaseCommit: build.target.Commit, BaseTree: build.target.Tree,
	})
	if err != nil {
		return nil, fmt.Errorf("encode canonical builder dispatch: %w", err)
	}
	planPath := filepath.Join(inputsRoot, "plan")
	dispatchPath := filepath.Join(inputsRoot, "dispatch")
	if err := writeBuilderInput(planPath, planRecord.CanonicalJSON); err != nil {
		return nil, err
	}
	if err := writeBuilderInput(dispatchPath, dispatchBytes); err != nil {
		return nil, err
	}
	inputs := []executor.Input{
		{Name: "dispatch", Path: dispatchPath, Digest: protocol.RawDigest(dispatchBytes)},
		{Name: "plan", Path: planPath, Digest: protocol.RawDigest(planRecord.CanonicalJSON)},
	}
	executableInput := ""
	if configuration.executableInput != nil {
		inputs = append(inputs, *configuration.executableInput)
		executableInput = configuration.executableInput.Name
	}
	slices.SortFunc(inputs, func(left, right executor.Input) int {
		return strings.Compare(left.Name, right.Name)
	})
	invocation := executor.Invocation{
		SchemaVersion: executor.InvocationSchemaVersion,
		ID:            build.identity.InvocationID, Role: "builder",
		Workspace: base.Path, WorkspaceDigest: manifest,
		WorkspaceAccess: executor.WorkspaceWritableExport,
		Inputs:          inputs, ExecutableInput: executableInput,
		Argv:        slices.Clone(configuration.argv),
		Environment: cloneEnvironment(configuration.environment),
		Network:     configuration.network, NestedSandbox: configuration.nestedSandbox,
		Timeout: worker.Timeout,
	}
	invoked = true
	completion, runErr := worker.Runner.RunWritable(ctx, invocation)
	if completion.Export != nil {
		cloned := *completion.Export
		export = &cloned
	}
	if runErr != nil {
		return nil, fmt.Errorf("run contained builder: %w", runErr)
	}
	if err := validateBuilderCompletion(completion, invocation, inputs); err != nil {
		return nil, err
	}
	if configuration.completionPolicy != nil {
		if err := configuration.completionPolicy.ValidateBuilderCompletion(completion); err != nil {
			return nil, fmt.Errorf("validate adapter builder completion: %w", err)
		}
	}
	if err := worker.Runner.ValidateExport(ctx, *completion.Export); err != nil {
		return nil, fmt.Errorf("validate builder workspace export: %w", err)
	}
	exported := base
	exported.Path = completion.Export.Path
	candidate, err := worker.Repository.PrepareCandidate(ctx, exported, repo.CaptureOptions{
		Scope: build.work.View().Scope, Timestamp: completion.CompletedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("prepare exact builder candidate: %w", err)
	}
	encoded, err := engine.EncodeBuildEffectResult(engine.BuildEffectResult{
		SchemaVersion: engine.BuildEffectResultSchemaVersion,
		Outcome:       engine.BuildOutcomeCandidateReady,
		Builder: protocol.BuilderRun{
			RunID: effect.ID, Agent: worker.Agent,
			StartedAt:   completion.StartedAt.UTC().Format(time.RFC3339Nano),
			CompletedAt: completion.CompletedAt.UTC().Format(time.RFC3339Nano),
		},
		Candidate: candidate,
	})
	if err != nil {
		return nil, err
	}
	if err := engine.ValidateEffectResult(effect.Kind, effect.ID, effect.Request, encoded); err != nil {
		return nil, fmt.Errorf("validate engine-stamped builder result: %w", err)
	}
	return encoded, nil
}

func validateBuilderCompletion(
	completion executor.RawCompletion,
	invocation executor.Invocation,
	inputs []executor.Input,
) error {
	if completion.InvocationID != invocation.ID || completion.RuntimeDigest != "" ||
		completion.WorkspaceDigest != invocation.WorkspaceDigest ||
		completion.WorkspaceAccess != executor.WorkspaceWritableExport ||
		completion.ExecutableInput != invocation.ExecutableInput {
		return errors.New("builder completion does not match its exact invocation")
	}
	if completion.Cancelled || completion.TimedOut || completion.OutputTruncated || completion.ExitCode != 0 {
		return errors.New("builder completion was nonzero, controlled, or truncated")
	}
	if completion.StartedAt.IsZero() || completion.CompletedAt.IsZero() ||
		completion.StartedAt.Location() != time.UTC || completion.CompletedAt.Location() != time.UTC ||
		completion.CompletedAt.Before(completion.StartedAt) {
		return errors.New("builder completion has invalid engine timestamps")
	}
	if completion.Export == nil || completion.Export.InvocationID != invocation.ID ||
		completion.Export.BaseDigest != invocation.WorkspaceDigest {
		return errors.New("builder completion lacks its exact writable export")
	}
	wantInputs := make([]executor.BoundInput, len(inputs))
	for index, input := range inputs {
		info, err := os.Lstat(input.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
			return errors.New("builder input changed before completion validation")
		}
		wantInputs[index] = executor.BoundInput{Name: input.Name, Digest: input.Digest, Size: uint64(info.Size())}
	}
	if !reflect.DeepEqual(completion.Inputs, wantInputs) {
		return errors.New("builder completion does not bind its exact inputs")
	}
	return nil
}

func writeBuilderInput(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		return fmt.Errorf("create builder input: %w", err)
	}
	written, writeErr := file.Write(contents)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write builder input: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close builder input: %w", closeErr)
	}
	if written != len(contents) {
		return errors.New("write builder input: short write")
	}
	return nil
}

func (worker BuilderWorker) cleanupRun(
	ctx context.Context,
	invocationID string,
	invoked bool,
	export *executor.WorkspaceExport,
	attemptRoot string,
	attemptIdentity os.FileInfo,
) error {
	if invoked {
		var err error
		if export != nil {
			err = worker.Runner.DiscardExport(ctx, *export)
		} else {
			var cleanup executor.WritableCleanup
			cleanup, err = worker.Runner.ReconcileWritable(ctx, invocationID)
			if err == nil && cleanup.InvocationID() != invocationID {
				err = errors.New("writable cleanup does not match the builder invocation")
			}
		}
		if err != nil {
			return fmt.Errorf("clean builder writable workspace: %w", err)
		}
	}
	if err := removeWorkspaceRoot(attemptRoot, attemptIdentity); err != nil {
		return fmt.Errorf("remove builder attempt root: %w", err)
	}
	return nil
}

// Cleanup consumes one bound-result cleanup capability before removing its
// attempt-owned executor and local workspace residue.
func (worker BuilderWorker) Cleanup(ctx context.Context, capability store.BoundBuildCleanupLease) error {
	if err := capability.RunBuilderCleanup(func(effect engine.JournalEffect) error {
		return worker.cleanup(ctx, effect)
	}); err != nil {
		return fmt.Errorf("run builder cleanup capability: %w", err)
	}
	return nil
}

// cleanup contains the raw cleanup algorithm behind Cleanup's Store gate.
func (worker BuilderWorker) cleanup(ctx context.Context, effect engine.JournalEffect) error {
	configuration, err := worker.configuration()
	if err != nil {
		return err
	}
	request, identity, err := buildIdentity(effect, configuration.digest)
	if err != nil {
		return err
	}
	_ = request
	cleanup, err := worker.Runner.ReconcileWritable(ctx, identity.InvocationID)
	if err != nil {
		return fmt.Errorf("reconcile builder writable workspace: %w", err)
	}
	if cleanup.InvocationID() != identity.InvocationID {
		return errors.New("writable cleanup does not match the builder invocation")
	}
	if err := removeBuildAttemptRoot(worker.WorkspaceRoot, identity.InvocationID); err != nil {
		return fmt.Errorf("remove builder attempt root: %w", err)
	}
	return nil
}

// ReconcileUnbound consumes one recovery capability, proves absence of Git
// publication, reconciles every attempt-owned workspace, and asks the same
// capability to seal those lower-level proofs into a Store retry proof.
func (worker BuilderWorker) ReconcileUnbound(
	ctx context.Context,
	capability store.BuildRecoveryLease,
) (store.BuildRetryProof, error) {
	proof, err := capability.ReconcileBuilder(
		func(effect engine.JournalEffect) (repo.AttemptUnpublishedProof, executor.WritableCleanup, error) {
			return worker.reconcileUnbound(ctx, effect)
		},
	)
	if err != nil {
		return store.BuildRetryProof{}, fmt.Errorf("run builder reconciliation capability: %w", err)
	}
	return proof, nil
}

// reconcileUnbound contains the raw proof and cleanup algorithm behind
// ReconcileUnbound's Store gate.
func (worker BuilderWorker) reconcileUnbound(
	ctx context.Context,
	effect engine.JournalEffect,
) (repo.AttemptUnpublishedProof, executor.WritableCleanup, error) {
	configuration, err := worker.configuration()
	if err != nil {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{}, err
	}
	if len(effect.Result) != 0 {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{},
			errors.New("unbound build reconciliation refuses an effect result")
	}
	build, err := worker.resolveBuild(ctx, effect, configuration, true)
	if err != nil {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{}, err
	}
	unpublished, err := worker.Repository.ProveAttemptUnpublished(ctx, build.identity.InvocationID)
	if err != nil {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{},
			fmt.Errorf("prove builder attempt unpublished: %w", err)
	}
	if unpublished.RepositoryID() != build.state.Repository ||
		unpublished.AttemptID() != build.identity.InvocationID {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{},
			errors.New("unpublished proof does not match the exact build attempt")
	}
	writable, err := worker.Runner.ReconcileWritable(ctx, build.identity.InvocationID)
	if err != nil {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{},
			fmt.Errorf("reconcile unpublished builder workspace: %w", err)
	}
	if writable.InvocationID() != build.identity.InvocationID {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{},
			errors.New("writable cleanup does not match the exact build attempt")
	}
	if err := removeBuildAttemptRoot(worker.WorkspaceRoot, build.identity.InvocationID); err != nil {
		return repo.AttemptUnpublishedProof{}, executor.WritableCleanup{},
			fmt.Errorf("remove unpublished builder attempt root: %w", err)
	}
	return unpublished, writable, nil
}

func buildIdentity(
	effect engine.JournalEffect,
	dispatchDigest string,
) (engine.BuildEffectRequest, engine.BuildAttemptIdentity, error) {
	if effect.Kind != engine.EffectBuild || !engine.ValidID(effect.ID) ||
		!protocol.ValidPositiveSafeInteger(effect.Attempt) {
		return engine.BuildEffectRequest{}, engine.BuildAttemptIdentity{},
			errors.New("builder operation requires one claimed build effect")
	}
	request, err := engine.ParseBuildEffectRequest(effect.Request)
	if err != nil {
		return engine.BuildEffectRequest{}, engine.BuildAttemptIdentity{}, err
	}
	if request.SchemaVersion != engine.BuildEffectRequestSchemaVersion ||
		request.DeliveryRunID != effect.DeliveryRunID ||
		request.BuilderDispatchDigest != dispatchDigest {
		return engine.BuildEffectRequest{}, engine.BuildAttemptIdentity{},
			errors.New("build effect does not match its journal or configured dispatch")
	}
	identity, err := engine.BuildAttemptIdentityFor(
		effect.ID, effect.Attempt, request.BuilderDispatchDigest,
	)
	return request, identity, err
}

func createBuildAttemptRoot(root, invocationID string) (string, os.FileInfo, error) {
	path, err := buildAttemptPath(root, invocationID)
	if err != nil {
		return "", nil, err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return "", nil, fmt.Errorf("create builder attempt root: %w", err)
	}
	identity, err := os.Lstat(path)
	if err != nil || !identity.IsDir() || identity.Mode().Perm() != 0o700 ||
		!workspaceRootOwnedByCurrentUser(identity) {
		_ = os.Remove(path)
		return "", nil, errors.New("builder attempt root identity is invalid")
	}
	return path, identity, nil
}

func removeBuildAttemptRoot(root, invocationID string) error {
	path, err := buildAttemptPath(root, invocationID)
	if err != nil {
		return err
	}
	identity, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !identity.IsDir() || identity.Mode().Perm()&0o077 != 0 ||
		!workspaceRootOwnedByCurrentUser(identity) {
		return errors.New("builder attempt root residue has an invalid identity")
	}
	return removeWorkspaceRoot(path, identity)
}

func buildAttemptPath(root, invocationID string) (string, error) {
	if err := validateWorkspaceRoot(root); err != nil {
		return "", fmt.Errorf("validate builder workspace root: %w", err)
	}
	if !engine.ValidID(invocationID) {
		return "", errors.New("builder invocation id is invalid")
	}
	path := filepath.Join(root, invocationID)
	if filepath.Dir(path) != root {
		return "", errors.New("builder attempt path escapes its configured root")
	}
	return path, nil
}

func cloneEnvironment(environment map[string]string) map[string]string {
	cloned := make(map[string]string, len(environment))
	for name, value := range environment {
		cloned[name] = value
	}
	return cloned
}

func sortedEnvironmentNames(environment map[string]string) []string {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
