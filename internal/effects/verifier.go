package effects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
)

const verifierWorkspaceDescription = "fresh-read-only-materialization"

var verifierEngineInputNames = [...]string{
	"assessment-schema",
	"dispatch",
	"plan",
	"submission",
}

// VerifierControl exposes only immutable review truth and the CAS operations
// needed by one already-authorized verifier attempt. Claiming, retry, verdict,
// and journal lifecycle authority remain Store-owned.
type VerifierControl interface {
	ResultResolver
	Plan(context.Context, string) (protocol.ExactPlan, error)
	Record(context.Context, string) (kind string, canonicalJSON []byte, err error)
	Artifact(context.Context, string) (mediaType string, contents []byte, err error)
	PutArtifact(context.Context, string, []byte) (string, error)
}

// VerifierRunner is the process-neutral projection of the sole admitted
// credentialed read-only executor boundary. ReconcileContentBound is required
// at configuration time so interrupted runtime residue remains mechanically
// recoverable, but it is deliberately not converted into verifier retry proof.
type VerifierRunner interface {
	ConfigurationDigest() string
	EffectiveLimits() executor.Limits
	RunCredentialReadOnly(context.Context, executor.Invocation) (executor.RawCompletion, error)
	ReconcileContentBound(context.Context, string) (executor.ContentBoundCleanup, error)
}

// VerifierAdapter owns one exact process profile and its output grammar. It
// receives engine-selected inputs and may add only the executable named by its
// profile; it cannot choose review identity, candidate, or verdict authority.
type VerifierAdapter interface {
	Profile() protocol.VerifierProfile
	Invocation(engine.VerifierAttemptIdentity, repo.CandidateWorkspace, []executor.Input) (executor.Invocation, error)
	ParseCompletion(executor.RawCompletion) (VerifierAdapterCompletion, error)
}

// VerifierAdapterCompletion contains only model-owned assessment bytes and the
// process-observed fresh thread identity. Engine-owned envelope facts are
// stamped by VerifierWorker after the generic completion checks pass.
type VerifierAdapterCompletion struct {
	Assessment []byte
	ThreadID   string
}

// VerifierWorker performs one fresh independent review behind a one-shot Store
// capability. It cannot claim, retry, bind, complete, or turn an assessment
// into a Baton verdict.
type VerifierWorker struct {
	Control           VerifierControl
	Runner            VerifierRunner
	Adapter           VerifierAdapter
	Repository        *repo.Repository
	WorkspaceRoot     string
	MaterializeLimits repo.MaterializeLimits
}

type verifierConfiguration struct {
	profile protocol.VerifierProfile
	record  protocol.EncodedRecord
	limits  executor.Limits
}

// ValidateConfiguration closes the static process, repository, executor, and
// workspace boundary before the worker is admitted to a controller.
func (worker VerifierWorker) ValidateConfiguration() error {
	_, err := worker.configuration()
	return err
}

// ProfileDigest returns the canonical profile digest used to configure Store
// dispatch authorization. The same configuration path is repeated immediately
// before a prepared execution capability is consumed.
func (worker VerifierWorker) ProfileDigest() (string, error) {
	configuration, err := worker.configuration()
	if err != nil {
		return "", err
	}
	return configuration.record.Digest, nil
}

// Agent returns the exact immutable agent identity bound by ProfileDigest.
func (worker VerifierWorker) Agent() (string, error) {
	configuration, err := worker.configuration()
	if err != nil {
		return "", err
	}
	return configuration.profile.Agent, nil
}

func (worker VerifierWorker) configuration() (verifierConfiguration, error) {
	if worker.Control == nil || worker.Runner == nil || worker.Adapter == nil || worker.Repository == nil {
		return verifierConfiguration{}, errors.New("verifier worker requires control, runner, adapter, and repository")
	}
	if err := validateWorkspaceRoot(worker.WorkspaceRoot); err != nil {
		return verifierConfiguration{}, fmt.Errorf("validate verifier workspace root: %w", err)
	}
	if worker.MaterializeLimits.Bytes == 0 || worker.MaterializeLimits.Entries == 0 {
		return verifierConfiguration{}, errors.New("verifier worker requires materialization ceilings")
	}
	limits := worker.Runner.EffectiveLimits()
	if err := limits.Validate(); err != nil {
		return verifierConfiguration{}, fmt.Errorf("validate verifier executor limits: %w", err)
	}
	if worker.MaterializeLimits.Bytes > limits.WorkspaceBytes ||
		worker.MaterializeLimits.Bytes > limits.InputBytes {
		return verifierConfiguration{}, errors.New("verifier materialization ceiling exceeds the executor boundary")
	}
	executorDigest := worker.Runner.ConfigurationDigest()
	if !protocol.ValidDigest(executorDigest) {
		return verifierConfiguration{}, errors.New("verifier runner lacks an exact configuration digest")
	}
	binding := worker.Repository.Binding()
	if err := binding.Validate(); err != nil {
		return verifierConfiguration{}, fmt.Errorf("validate verifier repository binding: %w", err)
	}

	profile := worker.Adapter.Profile()
	if err := closeVerifierProfileField(
		&profile.ExecutorConfigurationDigest, executorDigest, "executor configuration digest",
	); err != nil {
		return verifierConfiguration{}, err
	}
	if err := closeVerifierProfileField(&profile.RepositoryID, binding.RepositoryID, "repository id"); err != nil {
		return verifierConfiguration{}, err
	}
	if err := closeVerifierProfileField(&profile.WorkspaceRoot, worker.WorkspaceRoot, "workspace root"); err != nil {
		return verifierConfiguration{}, err
	}
	if err := closeVerifierProfileUint64(
		&profile.MaterializeBytes, worker.MaterializeLimits.Bytes, "materialization byte ceiling",
	); err != nil {
		return verifierConfiguration{}, err
	}
	if err := closeVerifierProfileUint64(
		&profile.MaterializeEntries, worker.MaterializeLimits.Entries, "materialization entry ceiling",
	); err != nil {
		return verifierConfiguration{}, err
	}
	if profile.TimeoutNanoseconds <= 0 ||
		time.Duration(profile.TimeoutNanoseconds) > limits.Runtime {
		return verifierConfiguration{}, errors.New("verifier timeout is absent or exceeds the executor ceiling")
	}
	for _, reserved := range verifierEngineInputNames {
		if profile.ExecutableInput == reserved {
			return verifierConfiguration{}, errors.New("verifier executable collides with an engine input")
		}
	}
	record, err := protocol.EncodeVerifierProfile(profile)
	if err != nil {
		return verifierConfiguration{}, fmt.Errorf("encode exact verifier profile: %w", err)
	}
	parsed, err := protocol.ParseVerifierProfile(record.CanonicalJSON)
	if err != nil {
		return verifierConfiguration{}, fmt.Errorf("reparse exact verifier profile: %w", err)
	}
	return verifierConfiguration{profile: parsed, record: record, limits: limits}, nil
}

func closeVerifierProfileField(observed *string, exact string, label string) error {
	if *observed == "" {
		*observed = exact
		return nil
	}
	if *observed != exact {
		return fmt.Errorf("verifier profile %s does not match configured truth", label)
	}
	return nil
}

func closeVerifierProfileUint64(observed *uint64, exact uint64, label string) error {
	if *observed == 0 {
		*observed = exact
		return nil
	}
	if *observed != exact {
		return fmt.Errorf("verifier profile %s does not match configured truth", label)
	}
	return nil
}

// Run validates static configuration before consuming the Store's one-shot
// execution capability. Once RunVerifier enters its callback, every error is
// conservatively ambiguous to the caller: this worker grants no retry path.
func (worker VerifierWorker) Run(
	ctx context.Context,
	capability store.PreparedAuthorizedVerifierLease,
) (json.RawMessage, error) {
	configuration, err := worker.configuration()
	if err != nil {
		return nil, err
	}
	result, err := capability.RunVerifier(func(effect engine.JournalEffect) (json.RawMessage, error) {
		return worker.runConfigured(ctx, effect, configuration)
	})
	if err != nil {
		return nil, fmt.Errorf("run verifier capability: %w", err)
	}
	return result, nil
}

// run is the raw execution algorithm used by focused package tests. Production
// composition must enter through Run and its Store-issued capability.
func (worker VerifierWorker) run(
	ctx context.Context,
	effect engine.JournalEffect,
) (json.RawMessage, error) {
	configuration, err := worker.configuration()
	if err != nil {
		return nil, err
	}
	return worker.runConfigured(ctx, effect, configuration)
}

type resolvedVerifierReview struct {
	request         engine.VerifierEffectRequest
	identity        engine.VerifierAttemptIdentity
	plan            protocol.ExactPlan
	submission      protocol.ExactSubmission
	candidate       repo.Candidate
	dispatchBytes   []byte
	planBytes       []byte
	submissionBytes []byte
	reviewInputs    []protocol.VerifierReviewInput
}

func (worker VerifierWorker) runConfigured(
	ctx context.Context,
	effect engine.JournalEffect,
	configuration verifierConfiguration,
) (result json.RawMessage, resultErr error) {
	review, err := worker.resolveReview(ctx, effect, configuration)
	if err != nil {
		return nil, err
	}
	if _, err := verifierPutArtifact(
		ctx, worker.Control, protocol.VerifierProfileMediaType,
		configuration.record.CanonicalJSON, protocol.MaximumVerifierProfileBytes,
	); err != nil {
		return nil, fmt.Errorf("store exact verifier profile: %w", err)
	}
	assessmentSchema, err := protocol.VerifierAssessmentOutputSchema()
	if err != nil {
		return nil, fmt.Errorf("construct verifier assessment schema: %w", err)
	}
	schemaArtifact, err := verifierPutArtifact(
		ctx, worker.Control, protocol.VerifierAssessmentSchemaMediaType,
		assessmentSchema, protocol.MaximumVerifierAssessmentBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("store exact verifier assessment schema: %w", err)
	}
	if schemaArtifact.Digest != configuration.profile.OutputSchemaDigest {
		return nil, errors.New("verifier assessment schema does not match its exact profile")
	}

	attemptRoot, attemptIdentity, err := createVerifierAttemptRoot(
		worker.WorkspaceRoot, review.identity.InvocationID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanupErr := removeWorkspaceRoot(attemptRoot, attemptIdentity)
		if cleanupErr != nil {
			result = nil
			cleanupErr = fmt.Errorf("remove verifier attempt root: %w", cleanupErr)
			if resultErr == nil {
				resultErr = cleanupErr
			} else {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()

	workspace, err := worker.Repository.MaterializeCandidate(
		ctx, review.candidate, filepath.Join(attemptRoot, "candidate"), worker.MaterializeLimits,
	)
	if err != nil {
		return nil, fmt.Errorf("materialize exact verifier candidate: %w", err)
	}
	if _, err := os.Lstat(filepath.Join(workspace.Path(), ".git")); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return nil, fmt.Errorf("inspect verifier candidate Git metadata: %w", err)
		}
		return nil, errors.New("verifier candidate materialization contains Git metadata")
	}
	inputs, err := stageVerifierInputs(attemptRoot, review)
	if err != nil {
		return nil, err
	}
	invocation, err := worker.Adapter.Invocation(
		review.identity, workspace, slices.Clone(inputs),
	)
	if err != nil {
		return nil, fmt.Errorf("construct verifier invocation: %w", err)
	}
	if err := validateVerifierInvocation(
		invocation, review.identity, workspace, inputs, configuration,
	); err != nil {
		return nil, err
	}

	completion, runErr := worker.Runner.RunCredentialReadOnly(ctx, invocation)
	cleanup, cleanupErr := worker.Runner.ReconcileContentBound(
		context.WithoutCancel(ctx), invocation.ID,
	)
	if cleanupErr == nil && cleanup.InvocationID() != invocation.ID {
		cleanupErr = errors.New("verifier executor cleanup does not match its invocation")
	}
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("reconcile credentialed read-only verifier: %w", cleanupErr)
		if runErr != nil {
			return nil, errors.Join(fmt.Errorf("run credentialed read-only verifier: %w", runErr), cleanupErr)
		}
		return nil, cleanupErr
	}
	if runErr != nil {
		return nil, fmt.Errorf("run credentialed read-only verifier: %w", runErr)
	}
	if err := validateVerifierCompletion(completion, invocation, configuration); err != nil {
		return nil, err
	}
	adapterCompletion, err := worker.Adapter.ParseCompletion(cloneVerifierRawCompletion(completion))
	if err != nil {
		return nil, fmt.Errorf("parse verifier adapter completion: %w", err)
	}
	assessmentBytes := slices.Clone(adapterCompletion.Assessment)
	if _, err := protocol.ParseVerifierAssessment(assessmentBytes); err != nil {
		return nil, fmt.Errorf("parse strict verifier assessment: %w", err)
	}
	return worker.persistVerifierResult(
		ctx, effect, review, configuration, completion,
		assessmentBytes, adapterCompletion.ThreadID,
	)
}

func (worker VerifierWorker) resolveReview(
	ctx context.Context,
	effect engine.JournalEffect,
	configuration verifierConfiguration,
) (resolvedVerifierReview, error) {
	if effect.Kind != engine.EffectVerifier || !engine.ValidID(effect.ID) ||
		!protocol.ValidPositiveSafeInteger(effect.Attempt) || len(effect.Result) != 0 {
		return resolvedVerifierReview{}, errors.New("verifier worker requires one unresolved claimed verifier effect")
	}
	request, err := engine.ParseVerifierEffectRequest(effect.Request)
	if err != nil {
		return resolvedVerifierReview{}, err
	}
	if request.DeliveryRunID != effect.DeliveryRunID || request.DispatchID != effect.ID ||
		request.VerifierProfileDigest != configuration.record.Digest ||
		request.Agent != configuration.profile.Agent {
		return resolvedVerifierReview{}, errors.New("verifier effect does not match its journal or configured profile")
	}
	identity, err := engine.VerifierAttemptIdentityFor(
		effect.ID, effect.Attempt, request.DispatchID, request.DispatchReceipt.Digest,
		request.VerifierProfileDigest, request.Agent, request.VerificationEpoch,
	)
	if err != nil {
		return resolvedVerifierReview{}, err
	}

	plan, err := worker.Control.Plan(ctx, request.PlanDigest)
	if err != nil {
		return resolvedVerifierReview{}, fmt.Errorf("load exact verifier plan: %w", err)
	}
	planRecord := plan.Record()
	if planRecord.Kind != protocol.DeliveryPlanSchemaVersion ||
		planRecord.Digest != request.PlanDigest ||
		planRecord.Digest != protocol.RawDigest(planRecord.CanonicalJSON) ||
		plan.DeliveryID() != request.DeliveryID {
		return resolvedVerifierReview{}, errors.New("exact verifier plan does not match its effect request")
	}
	contract, exists := plan.Work(request.WorkID)
	if !exists {
		return resolvedVerifierReview{}, errors.New("verifier work is absent from the exact plan")
	}
	target := plan.Target()
	if target.Repository != worker.Repository.Binding().RepositoryID ||
		target.Repository != request.Candidate.Repository {
		return resolvedVerifierReview{}, errors.New("verifier plan target does not match the configured repository")
	}

	kind, submissionBytes, err := worker.Control.Record(ctx, request.SubmissionDigest)
	if err != nil {
		return resolvedVerifierReview{}, fmt.Errorf("load exact verifier submission: %w", err)
	}
	if kind != protocol.SubmissionSchemaVersion || protocol.RawDigest(submissionBytes) != request.SubmissionDigest {
		return resolvedVerifierReview{}, errors.New("verifier submission record does not match its request")
	}
	submission, err := protocol.ParseSubmission(submissionBytes)
	if err != nil {
		return resolvedVerifierReview{}, fmt.Errorf("parse exact verifier submission: %w", err)
	}
	submissionRecord, submissionView := submission.Record(), submission.View()
	if submissionRecord.Digest != request.SubmissionDigest ||
		!bytes.Equal(submissionRecord.CanonicalJSON, submissionBytes) ||
		submissionView.SubmissionID != request.SubmissionID ||
		submissionView.DeliveryID != request.DeliveryID ||
		submissionView.WorkID != request.WorkID || submissionView.Attempt != request.WorkAttempt ||
		submissionView.PlanDigest != request.PlanDigest ||
		submissionView.ContractDigest != contract.Digest() ||
		submissionView.Candidate != request.Candidate {
		return resolvedVerifierReview{}, errors.New("exact verifier submission does not match its effect and plan")
	}

	candidate, err := worker.resolveVerifierCandidate(ctx, effect, request, submissionView, contract)
	if err != nil {
		return resolvedVerifierReview{}, err
	}
	dispatchBytes, err := protocol.ResolveArtifact(
		ctx, worker.Control, request.DispatchReceipt, protocol.MaximumControlReceiptBytes,
	)
	if err != nil {
		return resolvedVerifierReview{}, fmt.Errorf("resolve exact verifier dispatch: %w", err)
	}
	dispatch, err := protocol.ParseVerifierDispatch(dispatchBytes)
	if err != nil {
		return resolvedVerifierReview{}, fmt.Errorf("parse exact verifier dispatch: %w", err)
	}
	if dispatch.DispatchID != request.DispatchID ||
		dispatch.SubmissionDigest != request.SubmissionDigest ||
		dispatch.Candidate != request.Candidate ||
		dispatch.Workspace != verifierWorkspaceDescription {
		return resolvedVerifierReview{}, errors.New("verifier dispatch does not match its exact effect closure")
	}
	exactReview, err := protocol.ResolveExactVerifierReview(ctx, worker.Control, plan, submission)
	if err != nil {
		return resolvedVerifierReview{}, fmt.Errorf("resolve exact verifier review closure: %w", err)
	}
	return resolvedVerifierReview{
		request: request, identity: identity, plan: plan, submission: submission,
		candidate: candidate, dispatchBytes: slices.Clone(dispatchBytes),
		planBytes: slices.Clone(planRecord.CanonicalJSON), submissionBytes: slices.Clone(submissionBytes),
		reviewInputs: exactReview.Inputs(),
	}, nil
}

func (worker VerifierWorker) resolveVerifierCandidate(
	ctx context.Context,
	effect engine.JournalEffect,
	request engine.VerifierEffectRequest,
	submission protocol.Submission,
	contract protocol.ExactWorkContract,
) (repo.Candidate, error) {
	if submission.Builder.RunID == effect.ID {
		return repo.Candidate{}, errors.New("verifier cannot use itself as the builder")
	}
	builder, err := worker.Control.SucceededEffect(ctx, submission.Builder.RunID)
	if err != nil {
		return repo.Candidate{}, fmt.Errorf("resolve verifier builder effect: %w", err)
	}
	if builder.Kind != engine.EffectBuild || builder.DeliveryRunID != effect.DeliveryRunID {
		return repo.Candidate{}, errors.New("verifier builder belongs to a different journal")
	}
	if err := engine.ValidateEffectResult(builder.Kind, builder.ID, builder.Request, builder.Result); err != nil {
		return repo.Candidate{}, fmt.Errorf("validate verifier builder effect: %w", err)
	}
	buildRequest, err := engine.ParseBuildEffectRequest(builder.Request)
	if err != nil {
		return repo.Candidate{}, err
	}
	buildResult, err := engine.ParseBuildEffectResult(builder.Result)
	if err != nil {
		return repo.Candidate{}, err
	}
	candidate := buildResult.Candidate
	if buildRequest.DeliveryRunID != request.DeliveryRunID ||
		buildRequest.DeliveryID != request.DeliveryID || buildRequest.WorkID != request.WorkID ||
		buildRequest.WorkAttempt != request.WorkAttempt || buildRequest.DispatchDigest != contract.Digest() ||
		buildResult.Builder != submission.Builder ||
		candidate.RepositoryID != submission.Candidate.Repository ||
		candidate.TargetRef != submission.Base.Ref || candidate.BaseCommit != submission.Base.Commit ||
		candidate.Commit != submission.Candidate.Commit || candidate.Tree != submission.Candidate.Tree ||
		!slices.Equal(candidate.ChangedPaths, submission.ChangedPaths) {
		return repo.Candidate{}, errors.New("verifier builder result does not match its exact submission")
	}
	if err := worker.Repository.VerifyCandidate(ctx, candidate, contract.View().Scope); err != nil {
		return repo.Candidate{}, fmt.Errorf("reverify verifier candidate scope: %w", err)
	}
	return candidate, nil
}

func stageVerifierInputs(
	attemptRoot string,
	review resolvedVerifierReview,
) ([]executor.Input, error) {
	inputsRoot := filepath.Join(attemptRoot, "inputs")
	if err := os.Mkdir(inputsRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create verifier input root: %w", err)
	}
	assessmentSchema, err := protocol.VerifierAssessmentOutputSchema()
	if err != nil {
		return nil, fmt.Errorf("construct verifier assessment schema: %w", err)
	}
	contents := map[string][]byte{
		"assessment-schema": assessmentSchema,
		"dispatch":          review.dispatchBytes,
		"plan":              review.planBytes,
		"submission":        review.submissionBytes,
	}
	inputs := make([]executor.Input, 0, len(verifierEngineInputNames)+len(review.reviewInputs))
	for _, name := range verifierEngineInputNames {
		path := filepath.Join(inputsRoot, name)
		if err := writeVerifierInput(path, contents[name]); err != nil {
			return nil, err
		}
		inputs = append(inputs, executor.Input{
			Name: name, Path: path, Digest: protocol.RawDigest(contents[name]),
		})
	}
	for _, reviewInput := range review.reviewInputs {
		if !strings.HasPrefix(reviewInput.Name, "review-") ||
			protocol.RawDigest(reviewInput.Contents) != reviewInput.Digest {
			return nil, errors.New("exact verifier review returned an invalid staged input")
		}
		path := filepath.Join(inputsRoot, reviewInput.Name)
		if err := writeVerifierInput(path, reviewInput.Contents); err != nil {
			return nil, err
		}
		inputs = append(inputs, executor.Input{
			Name: reviewInput.Name, Path: path, Digest: reviewInput.Digest,
		})
	}
	slices.SortFunc(inputs, func(left, right executor.Input) int {
		return strings.Compare(left.Name, right.Name)
	})
	return inputs, nil
}

func writeVerifierInput(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		return fmt.Errorf("create verifier input: %w", err)
	}
	written, writeErr := file.Write(contents)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write verifier input: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close verifier input: %w", closeErr)
	}
	if written != len(contents) {
		return errors.New("write verifier input: short write")
	}
	return nil
}

func createVerifierAttemptRoot(root, invocationID string) (string, os.FileInfo, error) {
	path := filepath.Join(root, invocationID)
	if err := os.Mkdir(path, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", nil, fmt.Errorf("verifier invocation %q has unreconciled workspace residue", invocationID)
		}
		return "", nil, fmt.Errorf("create verifier attempt root: %w", err)
	}
	identity, err := os.Lstat(path)
	if err != nil || !identity.IsDir() || identity.Mode()&os.ModeSymlink != 0 ||
		identity.Mode().Perm() != 0o700 || !workspaceRootOwnedByCurrentUser(identity) {
		_ = os.Remove(path)
		return "", nil, errors.New("verifier attempt workspace identity is invalid")
	}
	return path, identity, nil
}

func validateVerifierInvocation(
	invocation executor.Invocation,
	identity engine.VerifierAttemptIdentity,
	workspace repo.CandidateWorkspace,
	engineInputs []executor.Input,
	configuration verifierConfiguration,
) error {
	profile := configuration.profile
	if invocation.SchemaVersion != executor.InvocationSchemaVersion ||
		invocation.ID != identity.InvocationID || invocation.Role != "verifier" ||
		invocation.RuntimeDigest != "" || invocation.Workspace != workspace.Path() ||
		invocation.WorkspaceDigest != workspace.Manifest() ||
		invocation.WorkspaceAccess != executor.WorkspaceReadOnly ||
		invocation.ExecutableInput != profile.ExecutableInput ||
		invocation.Network != executor.NetworkHost || !invocation.NestedSandbox ||
		!invocation.CredentialAccess || invocation.Timeout != time.Duration(profile.TimeoutNanoseconds) ||
		!slices.Equal(invocation.Argv, profile.Argv) ||
		!slices.Equal(sortedEnvironmentNames(invocation.Environment), profile.EnvironmentNames) {
		return errors.New("verifier invocation does not match its exact attempt and profile")
	}
	wantInputs := append(slices.Clone(engineInputs), executor.Input{
		Name: profile.ExecutableInput, Path: profile.BinaryPath, Digest: profile.BinaryDigest,
	})
	slices.SortFunc(wantInputs, func(left, right executor.Input) int {
		return strings.Compare(left.Name, right.Name)
	})
	if !sameVerifierInputs(invocation.Inputs, wantInputs) {
		return errors.New("verifier invocation does not contain its exact sorted inputs")
	}
	var total uint64
	for _, input := range invocation.Inputs {
		info, err := os.Lstat(input.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
			return fmt.Errorf("verifier input %q is not a stable regular file", input.Name)
		}
		size := uint64(info.Size())
		if input.Name == profile.ExecutableInput && size != uint64(profile.BinarySize) {
			return errors.New("verifier executable size does not match its profile")
		}
		if size > configuration.limits.InputBytes ||
			total > configuration.limits.InputBytes-size {
			return errors.New("verifier inputs exceed the executor byte ceiling")
		}
		total += size
	}
	return nil
}

func sameVerifierInputs(left, right []executor.Input) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateVerifierCompletion(
	completion executor.RawCompletion,
	invocation executor.Invocation,
	configuration verifierConfiguration,
) error {
	if completion.InvocationID != invocation.ID || completion.RuntimeDigest != "" ||
		completion.WorkspaceDigest != invocation.WorkspaceDigest ||
		completion.WorkspaceAccess != executor.WorkspaceReadOnly ||
		completion.ExecutableInput != invocation.ExecutableInput ||
		!completion.CredentialAccess || completion.Export != nil {
		return errors.New("verifier completion does not match its exact invocation")
	}
	if !engine.ValidID(completion.Unit) {
		return errors.New("verifier completion lacks its exact service unit")
	}
	if completion.ExitCode != 0 || completion.Cancelled || completion.TimedOut || completion.OutputTruncated {
		return errors.New("verifier completion was nonzero, controlled, or truncated")
	}
	if completion.StartedAt.IsZero() || completion.CompletedAt.IsZero() ||
		completion.StartedAt.Location() != time.UTC || completion.CompletedAt.Location() != time.UTC ||
		completion.CompletedAt.Before(completion.StartedAt) {
		return errors.New("verifier completion has invalid engine timestamps")
	}
	if len(completion.Stdout) > configuration.limits.StdoutBytes ||
		len(completion.Stderr) > configuration.limits.StderrBytes {
		return errors.New("verifier completion output exceeds the executor ceiling")
	}
	want := make([]executor.BoundInput, len(invocation.Inputs))
	for index, input := range invocation.Inputs {
		info, err := os.Lstat(input.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
			return errors.New("verifier input changed before completion validation")
		}
		want[index] = executor.BoundInput{Name: input.Name, Digest: input.Digest, Size: uint64(info.Size())}
	}
	if !sameVerifierBoundInputs(completion.Inputs, want) {
		return errors.New("verifier completion does not bind its exact inputs")
	}
	return nil
}

func sameVerifierBoundInputs(left, right []executor.BoundInput) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneVerifierRawCompletion(completion executor.RawCompletion) executor.RawCompletion {
	completion.Inputs = slices.Clone(completion.Inputs)
	completion.Stdout = slices.Clone(completion.Stdout)
	completion.Stderr = slices.Clone(completion.Stderr)
	if completion.Export != nil {
		export := *completion.Export
		completion.Export = &export
	}
	return completion
}

func (worker VerifierWorker) persistVerifierResult(
	ctx context.Context,
	effect engine.JournalEffect,
	review resolvedVerifierReview,
	configuration verifierConfiguration,
	completion executor.RawCompletion,
	assessmentBytes []byte,
	threadID string,
) (json.RawMessage, error) {
	assessment, err := verifierPutArtifact(
		ctx, worker.Control, "application/json", assessmentBytes, protocol.MaximumVerifierAssessmentBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("store verifier assessment: %w", err)
	}
	stdout, err := verifierPutCapture(ctx, worker.Control, completion.Stdout)
	if err != nil {
		return nil, fmt.Errorf("store verifier stdout: %w", err)
	}
	stderr, err := verifierPutCapture(ctx, worker.Control, completion.Stderr)
	if err != nil {
		return nil, fmt.Errorf("store verifier stderr: %w", err)
	}
	inputs := make([]protocol.VerifierExecutionInput, len(completion.Inputs))
	for index, input := range completion.Inputs {
		inputs[index] = protocol.VerifierExecutionInput{
			Name: input.Name, Digest: input.Digest, Size: input.Size,
		}
	}
	receiptRecord, err := protocol.EncodeVerifierExecutionReceipt(protocol.VerifierExecutionReceipt{
		SchemaVersion: protocol.VerifierExecutionReceiptSchemaVersion,
		EffectID:      effect.ID, EffectAttempt: effect.Attempt,
		InvocationID:  review.identity.InvocationID,
		Unit:          completion.Unit,
		DeliveryRunID: review.request.DeliveryRunID, DeliveryID: review.request.DeliveryID,
		WorkID: review.request.WorkID, WorkAttempt: review.request.WorkAttempt,
		PlanDigest:   review.request.PlanDigest,
		SubmissionID: review.request.SubmissionID, SubmissionDigest: review.request.SubmissionDigest,
		Candidate:  review.request.Candidate,
		DispatchID: review.request.DispatchID, DispatchDigest: review.request.DispatchReceipt.Digest,
		VerifierProfileDigest: review.request.VerifierProfileDigest,
		Agent:                 review.request.Agent, VerificationEpoch: review.request.VerificationEpoch,
		ExecutorConfigurationDigest: configuration.profile.ExecutorConfigurationDigest,
		ExecutableInput:             configuration.profile.ExecutableInput,
		ExecutableDigest:            configuration.profile.BinaryDigest,
		WorkspaceDigest:             completion.WorkspaceDigest,
		WorkspaceAccess:             string(executor.WorkspaceReadOnly), Inputs: inputs,
		Network: string(executor.NetworkHost), NestedSandbox: true, CredentialAccess: true,
		ModelToolNetwork:          configuration.profile.ModelToolNetwork,
		ModelToolCredentialAccess: configuration.profile.ModelToolCredentialAccess,
		AssessmentDigest:          assessment.Digest,
		Stdout:                    stdout, Stderr: stderr, ThreadID: threadID,
		StartedAt:     completion.StartedAt.Format(time.RFC3339Nano),
		CompletedAt:   completion.CompletedAt.Format(time.RFC3339Nano),
		TargetStarted: true, ServiceQuiescent: true,
		ExitCode: completion.ExitCode, Cancelled: completion.Cancelled,
		TimedOut: completion.TimedOut, OutputTruncated: completion.OutputTruncated,
		ExportPresent: completion.Export != nil,
	})
	if err != nil {
		return nil, fmt.Errorf("encode verifier execution receipt: %w", err)
	}
	receipt, err := verifierPutArtifact(
		ctx, worker.Control, protocol.VerifierExecutionReceiptMediaType,
		receiptRecord.CanonicalJSON, protocol.MaximumVerifierExecutionReceiptBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("store verifier execution receipt: %w", err)
	}
	encoded, err := engine.EncodeVerifierEffectResult(engine.VerifierEffectResult{
		SchemaVersion:     engine.VerifierEffectResultSchemaVersion,
		Outcome:           engine.VerifierOutcomeAssessmentReady,
		DispatchID:        review.request.DispatchID,
		VerificationEpoch: review.request.VerificationEpoch,
		Assessment:        assessment, ExecutionReceipt: receipt,
		StartedAt:   completion.StartedAt.Format(time.RFC3339Nano),
		CompletedAt: completion.CompletedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}
	if err := engine.ValidateEffectResult(effect.Kind, effect.ID, effect.Request, encoded); err != nil {
		return nil, fmt.Errorf("validate engine-stamped verifier result: %w", err)
	}
	return encoded, nil
}

func verifierPutCapture(
	ctx context.Context,
	control VerifierControl,
	contents []byte,
) (protocol.CapturedArtifact, error) {
	pointer, err := verifierPutArtifact(
		ctx, control, "application/octet-stream", contents, uint64(len(contents)),
	)
	if err != nil {
		return protocol.CapturedArtifact{}, err
	}
	return protocol.CapturedArtifact{
		Ref: pointer.Ref, MediaType: pointer.MediaType, Digest: pointer.Digest, Size: int64(len(contents)),
	}, nil
}

func verifierPutArtifact(
	ctx context.Context,
	control VerifierControl,
	mediaType string,
	contents []byte,
	maximumBytes uint64,
) (protocol.Artifact, error) {
	if uint64(len(contents)) > maximumBytes {
		return protocol.Artifact{}, errors.New("verifier artifact exceeds its byte ceiling")
	}
	digest, err := control.PutArtifact(ctx, mediaType, contents)
	if err != nil {
		return protocol.Artifact{}, err
	}
	if digest != protocol.RawDigest(contents) {
		return protocol.Artifact{}, errors.New("verifier artifact store returned the wrong digest")
	}
	pointer := protocol.Artifact{Ref: digest, MediaType: mediaType, Digest: digest}
	resolved, err := protocol.ResolveArtifact(ctx, control, pointer, maximumBytes)
	if err != nil {
		return protocol.Artifact{}, err
	}
	if !bytes.Equal(resolved, contents) {
		return protocol.Artifact{}, errors.New("verifier artifact store changed exact bytes")
	}
	return pointer, nil
}
