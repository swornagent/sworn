// Package producer turns contained subprocess completions into measured,
// content-addressed protocol facts. It does not route work or mutate engine
// state.
package producer

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"time"

	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

const (
	LocalCheckDefinitionSchemaVersion = protocol.LocalCheckDefinitionSchemaVersion
	maximumDefinitionBytes            = protocol.MaximumLocalCheckDefinitionBytes
	maximumEnvironmentBytes           = protocol.MaximumLocalEnvironmentBytes
	maximumReceiptBytes               = protocol.MaximumLocalCheckReceiptBytes
	localReceiptMediaType             = "application/vnd.sworn.local-check-receipt+json"
)

var (
	ErrCheckNotAdmitted = errors.New("local check did not produce an admitted pass")
	ErrCheckControlled  = errors.New("local check ended through a control boundary")
)

type Runner interface {
	Probe(context.Context) (executor.ProbeReport, error)
	EffectiveLimits() executor.Limits
	RunContained(context.Context, executor.Invocation) (executor.RawCompletion, error)
}

type ArtifactStore interface {
	PutArtifact(context.Context, string, []byte) (string, error)
	Artifact(context.Context, string) (mediaType string, contents []byte, err error)
}

type EvidenceDefinition = protocol.LocalEvidenceDefinition
type LocalCheckDefinition = protocol.LocalCheckDefinition

type Request struct {
	CheckID    string
	RunID      string
	Definition protocol.Artifact
	Repository *repo.Repository
	Candidate  repo.Candidate
	Workspace  repo.CandidateWorkspace
}

type Result struct {
	Receipt  protocol.Artifact
	Check    *protocol.Check
	Evidence *protocol.Evidence
}

// RunLocal resolves one exact definition, runs it once over a fresh read-only
// candidate workspace, and stores immutable streams and a canonical receipt.
// Only an unambiguous exit-zero completion yields Baton check and evidence
// entries.
func RunLocal(
	ctx context.Context,
	runner Runner,
	artifacts ArtifactStore,
	request Request,
) (Result, error) {
	if runner == nil || artifacts == nil || request.Repository == nil {
		return Result{}, errors.New("local producer requires runner, artifact store, and repository")
	}
	if !protocol.ValidID(request.CheckID) || !protocol.ValidID(request.RunID) {
		return Result{}, errors.New("local producer requires valid check and run ids")
	}
	if request.Workspace.RepositoryID() != request.Candidate.RepositoryID || !sameCandidate(request.Workspace.Candidate(), request.Candidate) {
		return Result{}, errors.New("local producer workspace does not bind the exact candidate")
	}
	if err := request.Repository.VerifyCandidateWorkspace(ctx, request.Workspace); err != nil {
		return Result{}, fmt.Errorf("verify local check candidate workspace: %w", err)
	}
	definitionBytes, err := resolveArtifact(ctx, artifacts, request.Definition, maximumDefinitionBytes)
	if err != nil {
		return Result{}, fmt.Errorf("resolve local check definition: %w", err)
	}
	if request.Definition.MediaType != "application/json" {
		return Result{}, errors.New("local check definition must be application/json")
	}
	definition, err := parseDefinition(definitionBytes)
	if err != nil {
		return Result{}, err
	}
	effectiveLimits := runner.EffectiveLimits()
	if err := effectiveLimits.Validate(); err != nil {
		return Result{}, fmt.Errorf("validate local check executor limits: %w", err)
	}
	timeout := time.Duration(definition.TimeoutSeconds) * time.Second
	if timeout > effectiveLimits.Runtime {
		return Result{}, errors.New("local check timeout exceeds the effective executor limit")
	}
	workspaceDigest := request.Workspace.Manifest()
	probe, err := runner.Probe(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("probe local check executor: %w", err)
	}
	environment, err := storeEnvironment(ctx, artifacts, probe, effectiveLimits)
	if err != nil {
		return Result{}, err
	}
	invocation := executor.Invocation{
		SchemaVersion:   executor.InvocationSchemaVersion,
		ID:              request.RunID,
		Role:            "producer",
		Workspace:       request.Workspace.Path(),
		WorkspaceDigest: workspaceDigest,
		WorkspaceAccess: executor.WorkspaceReadOnly,
		Argv:            append([]string(nil), definition.Argv...),
		Network:         executor.NetworkNone,
		Timeout:         timeout,
	}
	completion, err := runner.RunContained(ctx, invocation)
	if err != nil {
		return Result{}, fmt.Errorf("run local check %q: %w", request.CheckID, err)
	}
	if err := validateCompletion(invocation, completion); err != nil {
		return Result{}, err
	}
	stdout, err := storeCapture(ctx, artifacts, completion.Stdout)
	if err != nil {
		return Result{}, fmt.Errorf("store local check stdout: %w", err)
	}
	stderr, err := storeCapture(ctx, artifacts, completion.Stderr)
	if err != nil {
		return Result{}, fmt.Errorf("store local check stderr: %w", err)
	}
	outcome := "pass"
	if completion.ExitCode != 0 || completion.Cancelled || completion.TimedOut || completion.OutputTruncated {
		outcome = "not_admitted"
	}
	receipt := protocol.LocalCheckReceipt{
		SchemaVersion: protocol.LocalCheckReceiptSchemaVersion,
		CheckID:       request.CheckID,
		RunID:         request.RunID,
		Definition:    request.Definition,
		Candidate: protocol.CandidatePoint{
			Repository: request.Candidate.RepositoryID,
			Commit:     request.Candidate.Commit,
			Tree:       request.Candidate.Tree,
		},
		WorkspaceDigest:  workspaceDigest,
		Environment:      environment,
		WorkspaceAccess:  string(executor.WorkspaceReadOnly),
		WorkingDirectory: definition.WorkingDirectory,
		Argv:             append([]string(nil), definition.Argv...),
		TimeoutSeconds:   definition.TimeoutSeconds,
		Network:          string(executor.NetworkNone),
		StartedAt:        formatTime(completion.StartedAt),
		CompletedAt:      formatTime(completion.CompletedAt),
		ExitCode:         completion.ExitCode,
		Cancelled:        completion.Cancelled,
		TimedOut:         completion.TimedOut,
		OutputTruncated:  completion.OutputTruncated,
		Outcome:          outcome,
		Stdout:           stdout,
		Stderr:           stderr,
	}
	encoded, err := protocol.EncodeLocalCheckReceipt(receipt)
	if err != nil {
		return Result{}, err
	}
	if len(encoded.CanonicalJSON) > maximumReceiptBytes {
		return Result{}, errors.New("local check receipt exceeds persistence ceiling")
	}
	receiptPointer, err := putVerifiedArtifact(ctx, artifacts, localReceiptMediaType, encoded.CanonicalJSON)
	if err != nil {
		return Result{}, fmt.Errorf("store local check receipt: %w", err)
	}
	result := Result{Receipt: receiptPointer}
	if outcome != "pass" {
		if completion.Cancelled || completion.TimedOut || completion.OutputTruncated {
			return result, ErrCheckControlled
		}
		return result, ErrCheckNotAdmitted
	}
	exitCode := completion.ExitCode
	check := protocol.Check{
		ID:            request.CheckID,
		Outcome:       "pass",
		RunID:         request.RunID,
		CandidateTree: request.Candidate.Tree,
		Environment:   environment,
		StartedAt:     receipt.StartedAt,
		CompletedAt:   receipt.CompletedAt,
		ExitCode:      &exitCode,
		Receipt:       receiptPointer,
	}
	evidence := protocol.Evidence{
		ID:            definition.Evidence.ID,
		AcceptanceIDs: append([]string(nil), definition.Evidence.AcceptanceIDs...),
		Kind:          "test",
		Boundary:      definition.Evidence.Boundary,
		Environment:   environment,
		UsesMocks:     definition.Evidence.UsesMocks,
		ProducerRunID: request.RunID,
		CandidateTree: request.Candidate.Tree,
		CapturedAt:    receipt.CompletedAt,
		Artifact:      receiptPointer,
		Observed:      definition.Evidence.Observed,
	}
	result.Check = &check
	result.Evidence = &evidence
	return result, nil
}

func parseDefinition(contents []byte) (protocol.LocalCheckDefinition, error) {
	if len(contents) > maximumDefinitionBytes {
		return protocol.LocalCheckDefinition{}, errors.New("local check definition exceeds byte ceiling")
	}
	return protocol.ParseLocalCheckDefinition(contents)
}

func storeEnvironment(
	ctx context.Context,
	artifacts ArtifactStore,
	probe executor.ProbeReport,
	limits executor.Limits,
) (protocol.Environment, error) {
	snapshotDigest, err := protocol.SnapshotDigest()
	if err != nil {
		return protocol.Environment{}, err
	}
	probe.Controllers = append([]string(nil), probe.Controllers...)
	slices.Sort(probe.Controllers)
	contents, err := protocol.EncodeCanonical(protocol.LocalEnvironment{
		SchemaVersion:          protocol.LocalEnvironmentSchemaVersion,
		ProtocolSnapshotDigest: "sha256:" + snapshotDigest,
		EngineRuntime:          runtime.Version(),
		OS:                     runtime.GOOS,
		Architecture:           runtime.GOARCH,
		Executor: protocol.LocalExecutorProbe{
			BubblewrapVersion: probe.BubblewrapVersion,
			SystemdVersion:    probe.SystemdVersion,
			CgroupV2:          probe.CgroupV2,
			UserManager:       probe.UserManager,
			Controllers:       probe.Controllers,
		},
		ExecutorPolicyVersion: executor.ContainmentPolicyVersion,
		Limits: protocol.LocalExecutionLimits{
			RuntimeNanoseconds: limits.Runtime.Nanoseconds(),
			MemoryBytes:        limits.MemoryBytes,
			SwapBytes:          limits.SwapBytes,
			Tasks:              limits.Tasks,
			CPUPercent:         limits.CPUPercent,
			FileBytes:          limits.FileBytes,
			TempBytes:          limits.TempBytes,
			HomeBytes:          limits.HomeBytes,
			InputBytes:         limits.InputBytes,
			WorkspaceBytes:     limits.WorkspaceBytes,
			StdoutBytes:        int64(limits.StdoutBytes),
			StderrBytes:        int64(limits.StderrBytes),
		},
		RuntimeTrustRoot:  "/usr",
		HermeticToolchain: false,
		WorkspaceAccess:   string(executor.WorkspaceReadOnly),
		Network:           string(executor.NetworkNone),
	})
	if err != nil {
		return protocol.Environment{}, err
	}
	if len(contents) > maximumEnvironmentBytes {
		return protocol.Environment{}, errors.New("local environment artifact exceeds byte ceiling")
	}
	pointer, err := putVerifiedArtifact(ctx, artifacts, protocol.LocalEnvironmentMediaType, contents)
	if err != nil {
		return protocol.Environment{}, fmt.Errorf("store local environment: %w", err)
	}
	return protocol.Environment{Kind: "local", Ref: pointer.Digest}, nil
}

func storeCapture(ctx context.Context, artifacts ArtifactStore, contents []byte) (protocol.CapturedArtifact, error) {
	pointer, err := putVerifiedArtifact(ctx, artifacts, "application/octet-stream", contents)
	if err != nil {
		return protocol.CapturedArtifact{}, err
	}
	return protocol.CapturedArtifact{
		Ref: pointer.Ref, MediaType: pointer.MediaType, Digest: pointer.Digest, Size: int64(len(contents)),
	}, nil
}

func putVerifiedArtifact(
	ctx context.Context,
	artifacts ArtifactStore,
	mediaType string,
	contents []byte,
) (protocol.Artifact, error) {
	digest, err := artifacts.PutArtifact(ctx, mediaType, contents)
	if err != nil {
		return protocol.Artifact{}, err
	}
	if digest != protocol.RawDigest(contents) {
		return protocol.Artifact{}, errors.New("artifact store returned the wrong digest")
	}
	pointer := protocol.Artifact{Ref: digest, MediaType: mediaType, Digest: digest}
	if _, err := resolveArtifact(ctx, artifacts, pointer, uint64(len(contents))); err != nil {
		return protocol.Artifact{}, err
	}
	return pointer, nil
}

func resolveArtifact(
	ctx context.Context,
	artifacts ArtifactStore,
	pointer protocol.Artifact,
	maximumBytes uint64,
) ([]byte, error) {
	mediaType, contents, err := artifacts.Artifact(ctx, pointer.Digest)
	if err != nil {
		return nil, err
	}
	if uint64(len(contents)) > maximumBytes {
		return nil, errors.New("artifact exceeds byte ceiling")
	}
	if mediaType != pointer.MediaType || protocol.RawDigest(contents) != pointer.Digest {
		return nil, errors.New("artifact does not match its pointer")
	}
	if err := protocol.ValidateArtifactContent(mediaType, contents); err != nil {
		return nil, err
	}
	return contents, nil
}

func validateCompletion(invocation executor.Invocation, completion executor.RawCompletion) error {
	if completion.InvocationID != invocation.ID || completion.WorkspaceDigest != invocation.WorkspaceDigest ||
		completion.WorkspaceAccess != executor.WorkspaceReadOnly || len(completion.Inputs) != 0 || completion.Export != nil ||
		completion.StartedAt.IsZero() || completion.CompletedAt.IsZero() || completion.CompletedAt.Before(completion.StartedAt) {
		return errors.New("local check completion does not match its invocation")
	}
	return nil
}

func sameCandidate(left, right repo.Candidate) bool {
	return left.RepositoryID == right.RepositoryID && left.TargetRef == right.TargetRef &&
		left.BaseCommit == right.BaseCommit && left.BaseTree == right.BaseTree &&
		left.Commit == right.Commit && left.Tree == right.Tree && left.Ref == right.Ref &&
		slices.Equal(left.ChangedPaths, right.ChangedPaths)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
