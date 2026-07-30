package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

const effectLease = 5 * time.Minute

var (
	testCrashBeforeEffect string
	testCrashAfterEffect  string
	testOwnerLeaseMillis  string
)

type Service struct {
	journal       *journal.Store
	resolver      approvalResolver
	dispatcher    driver.Driver
	production    *productionDriverRuntime
	gitExecutable string
	now           func() time.Time
}

type RunStatus struct {
	SchemaVersion     string         `json:"schema_version"`
	RunID             string         `json:"run_id"`
	State             string         `json:"state"`
	DesiredState      string         `json:"desired_state"`
	ControlGeneration int64          `json:"control_generation"`
	ManifestDigest    string         `json:"manifest_digest"`
	PlanDigest        string         `json:"plan_digest,omitempty"`
	TargetRef         string         `json:"target_ref"`
	TargetHead        string         `json:"target_head,omitempty"`
	ReleaseHead       string         `json:"release_head,omitempty"`
	Outcome           string         `json:"outcome,omitempty"`
	Effects           []EffectStatus `json:"effects"`
	EventOffset       int64          `json:"event_offset"`
}

type EffectStatus struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code,omitempty"`
}

type engine struct {
	manifest   admittedManifest
	repository *gitx.Repository
	git        baton.GitRepository
	actions    *baton.Actions
	installer  *authorityInstaller
	workspaces *gitx.Workspaces
	product    *gitx.ProductExclusionAdmission
	registry   driver.SelectionRegistry
	configured *configuredRuntimeRegistry
	inertness  baton.InertnessResolver
	actionMu   sync.Mutex
}

type sealedRecord struct {
	Slice        string                   `json:"slice"`
	Binds        string                   `json:"binds"`
	Before       string                   `json:"before"`
	Candidate    string                   `json:"candidate"`
	Tree         string                   `json:"tree"`
	ProductTree  string                   `json:"product_tree"`
	ChangedPaths []string                 `json:"changed_paths"`
	Receipt      baton.AppendReceiptInput `json:"receipt"`
}

type implementationCycle struct {
	Release        string `json:"release"`
	Slice          string `json:"slice"`
	Binds          string `json:"binds"`
	Before         string `json:"before"`
	Plan           string `json:"plan"`
	ReleaseHead    string `json:"release_head"`
	TargetHead     string `json:"target_head"`
	Track          string `json:"track"`
	TrackRef       string `json:"track_ref"`
	TrackHead      string `json:"track_head"`
	Base           string `json:"base,omitempty"`
	DispatchWork   string `json:"dispatch_work"`
	DispatchEffect string `json:"dispatch_effect"`
	PreparedWork   string `json:"prepared_work"`
	PreparedEffect string `json:"prepared_effect"`
}

const planProposalVersion = "sworn.plan-proposal/v1"

type planProposalAuthority struct {
	Release      string `json:"release"`
	PriorPlan    string `json:"prior_plan,omitempty"`
	ReleaseRef   string `json:"release_ref"`
	ReleaseHead  string `json:"release_head,omitempty"`
	TargetRef    string `json:"target_ref"`
	TargetHead   string `json:"target_head"`
	Before       string `json:"before"`
	SourceWork   string `json:"source_work"`
	SourceEffect string `json:"source_effect"`
}

type planProposalCommand struct {
	Version    string                `json:"version"`
	Authority  planProposalAuthority `json:"authority"`
	PlanBytes  []byte                `json:"plan_bytes"`
	PlanDigest string                `json:"plan_digest"`
}

type admittedPlanProposal struct {
	plan      baton.Plan
	authority planProposalAuthority
	replayKey string
}

func OpenService(ctx context.Context, path string) (*Service, error) {
	return openService(ctx, path, nil)
}

func OpenServiceWithDriverConfig(
	ctx context.Context,
	path string,
	config driver.LoadedDriverConfig,
	options driver.DriverFactoryOptions,
) (*Service, error) {
	production, err := newProductionDriverRuntime(config, options)
	if err != nil {
		return nil, err
	}
	return openService(ctx, path, production)
}

func openService(
	ctx context.Context,
	path string,
	production *productionDriverRuntime,
) (*Service, error) {
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return nil, err
	}
	store, err := journal.Open(ctx, path)
	if err != nil {
		return nil, runtimeFail("JOURNAL_UNAVAILABLE", err)
	}
	return &Service{journal: store, resolver: newProductionApprovalResolver(func() (string, error) {
		return os.Getenv("SWORN_GITHUB_TOKEN"), nil
	}), dispatcher: driver.Dispatcher{}, production: production,
		gitExecutable: gitExecutable, now: time.Now}, nil
}

func OpenStatusService(ctx context.Context, path string) (*Service, error) {
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return nil, err
	}
	store, err := journal.OpenReadOnly(ctx, path)
	if err != nil {
		return nil, runtimeFail("JOURNAL_UNAVAILABLE", err)
	}
	return &Service{journal: store, gitExecutable: gitExecutable, now: time.Now}, nil
}

func resolveGitExecutable() (string, error) {
	value, err := exec.LookPath("git")
	if err == nil {
		value, err = filepath.Abs(value)
	}
	if err == nil {
		value, err = filepath.EvalSymlinks(value)
	}
	if err != nil {
		return "", runtimeFail("GIT_UNAVAILABLE", nil)
	}
	return value, nil
}

func newService(store *journal.Store, resolver approvalResolver, dispatcher driver.Driver, gitExecutable string, now func() time.Time) (*Service, error) {
	if store == nil || resolver == nil || dispatcher == nil || !filepath.IsAbs(gitExecutable) || now == nil {
		return nil, runtimeFail("INVALID_SERVICE", nil)
	}
	return &Service{journal: store, resolver: resolver, dispatcher: dispatcher,
		gitExecutable: gitExecutable, now: now}, nil
}

func (s *Service) Close() error {
	if s == nil || s.journal == nil {
		return nil
	}
	return s.journal.Close()
}

func (s *Service) openEngine(manifest admittedManifest) (*engine, error) {
	if err := s.validateDriverConfigMode(manifest); err != nil {
		return nil, err
	}
	var registry driver.SelectionRegistry
	var configured *configuredRuntimeRegistry
	if manifest.value.production() {
		var err error
		configured, err = s.production.registryFor(manifest)
		if err != nil {
			return nil, err
		}
		registry = configured.registry.SelectionRegistry
	}
	repository, err := gitx.Open(manifest.value.Repository, s.gitExecutable)
	if err != nil || repository.Root() != manifest.value.Repository {
		return nil, runtimeFail("REPOSITORY_BINDING_MISMATCH", err)
	}
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	gitRepository := baton.UseGitRepository(repository)
	recordAdmission, err := repository.ResolveRecordPathAdmission()
	if err != nil {
		return nil, runtimeFail("BATON_UNAVAILABLE", err)
	}
	productAdmission, err := repository.ResolveProductExclusion(
		recordAdmission,
		inertness,
	)
	if err != nil {
		return nil, runtimeFail("BATON_UNAVAILABLE", err)
	}
	actions, err := baton.NewActions(gitRepository, inertness)
	if err != nil {
		return nil, runtimeFail("BATON_UNAVAILABLE", err)
	}
	workspaces, err := gitx.NewRunWorkspaces(repository, manifest.value.RunID)
	if err != nil {
		return nil, runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	if !manifest.value.production() {
		adapter, err := driver.NewProcessAdapter(manifest.value.Driver.AdapterKey,
			driver.FakeDriverID, driver.FakeDriverVersion, driver.ExecutableIdentity{
				Path: manifest.value.Driver.Executable, Digest: manifest.value.Driver.Digest})
		if err != nil {
			_ = workspaces.Close()
			return nil, runtimeFail("DRIVER_UNAVAILABLE", err)
		}
		registry, err = driver.NewSelectionRegistry([]driver.ProfileConfig{{
			Key: manifest.value.Driver.Profile, Adapter: manifest.value.Driver.AdapterKey,
			Network: driver.NetworkNone}}, []driver.Adapter{adapter})
		if err != nil {
			_ = workspaces.Close()
			return nil, runtimeFail("DRIVER_UNAVAILABLE", err)
		}
	}
	return &engine{manifest: manifest, repository: repository, git: gitRepository,
		actions: actions, installer: newAuthorityInstaller(actions), workspaces: workspaces,
		product: productAdmission, registry: registry, configured: configured,
		inertness: inertness}, nil
}

func (s *Service) validateDriverConfigMode(
	manifest admittedManifest,
) error {
	if manifest.value.production() {
		if s == nil || s.production == nil {
			return runtimeFail("DRIVER_CONFIG_UNAVAILABLE", nil)
		}
		return nil
	}
	if s != nil && s.production != nil {
		return runtimeFail("DRIVER_CONFIG_UNEXPECTED", nil)
	}
	return nil
}

func (e *engine) Close() error {
	if e == nil || e.workspaces == nil {
		return nil
	}
	return e.workspaces.Close()
}

func ownerDuration() time.Duration {
	if testOwnerLeaseMillis != "" {
		if value, err := strconv.ParseInt(testOwnerLeaseMillis, 10, 64); err == nil && value >= 300 {
			return time.Duration(value) * time.Millisecond
		}
	}
	return 30 * time.Second
}

func (s *Service) Start(ctx context.Context, manifestBytes []byte) (RunStatus, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return RunStatus{}, runtimeFail("INVALID_SERVICE", nil)
	}
	manifest, err := admitManifest(manifestBytes)
	if err != nil {
		return RunStatus{}, err
	}
	if err := s.validateDriverConfigMode(manifest); err != nil {
		return RunStatus{}, err
	}
	now := s.now().UTC()
	if err := s.journal.RegisterRun(ctx, journal.Run{ID: manifest.value.RunID,
		ManifestDigest: manifest.digest, Repository: manifest.value.Repository,
		Release: manifest.value.Release, TargetRef: manifest.value.TargetRef, CreatedAt: now}); err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	if err := s.journal.RecordCommand(ctx, journal.Command{RunID: manifest.value.RunID,
		ReplayKey: "manifest", Kind: "start", Payload: manifest.raw, CreatedAt: now}); err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	owner, err := s.journal.AcquireOwner(ctx, manifest.value.RunID, now, ownerDuration(), false)
	if err != nil {
		return RunStatus{}, runtimeFail("OWNER_UNAVAILABLE", err)
	}
	return s.driveOwned(ctx, manifest.value.RunID, owner)
}

func proposalReplayKey(revision int64, sourceWork string) string {
	return fmt.Sprintf(
		"plan-proposal/%020d/%s",
		revision,
		strings.TrimPrefix(sourceWork, "sha256:"),
	)
}

func proposalSourceEffect(
	snapshot journal.Snapshot,
	sourceWork string,
	planBytes []byte,
) (string, error) {
	prefix := "attempt/" + strings.TrimPrefix(sourceWork, "sha256:") + "/"
	found := ""
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" ||
			effect.State != journal.Succeeded ||
			!strings.HasPrefix(effect.ID, prefix) {
			continue
		}
		submission, err := driver.DecodeSubmission(effect.Result)
		if err != nil || submission.Responsibility != driver.PlannerProposal {
			return "", runtimeFail("CORRUPT_JOURNAL", err)
		}
		body, err := exactBytes(submission.Plan)
		if err != nil || !bytes.Equal(body, planBytes) || found != "" {
			return "", runtimeFail("CORRUPT_JOURNAL", err)
		}
		found = effect.ID
	}
	if found == "" {
		return "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return found, nil
}

func (s *Service) recordProposal(
	ctx context.Context,
	runID string,
	plan baton.Plan,
	authority planProposalAuthority,
) error {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return runtimeFail("JOURNAL_READ_FAILED", err)
	}
	authority.SourceEffect, err = proposalSourceEffect(
		snapshot, authority.SourceWork, plan.Bytes())
	if err != nil {
		return err
	}
	command := planProposalCommand{
		Version: planProposalVersion, Authority: authority,
		PlanBytes: plan.Bytes(), PlanDigest: plan.Digest(),
	}
	payload := mustJSON(command)
	key := proposalReplayKey(plan.Metadata().Revision, authority.SourceWork)
	now := s.now().UTC()
	if err := s.journal.RecordCommand(ctx, journal.Command{RunID: runID, ReplayKey: key,
		Kind: "planner_proposal", Payload: payload, CreatedAt: now}); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	if err := s.journal.AppendEvent(ctx, runID, "awaiting_approval",
		[]byte(plan.Digest()), now); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func validatePlanBinding(manifest admittedManifest, plan baton.Plan, current *baton.State) error {
	metadata := plan.Metadata()
	if metadata.Release != manifest.value.Release ||
		metadata.Repository != manifest.value.Approval.Repository ||
		metadata.TargetRef != manifest.value.TargetRef {
		return runtimeFail("PLAN_BINDING_MISMATCH", nil)
	}
	if _, err := approvalMarker(manifest.value.Approval, metadata.ApprovalRef); err != nil {
		return err
	}
	if current == nil {
		if metadata.Revision != 1 || metadata.PreviousPlan != nil {
			return runtimeFail("PLAN_BINDING_MISMATCH", nil)
		}
		return nil
	}
	if metadata.Revision != current.Plan.Metadata.Revision+1 ||
		metadata.PreviousPlan == nil || *metadata.PreviousPlan != current.Plan.OID ||
		metadata.ApprovalRef == current.Plan.Metadata.ApprovalRef {
		return runtimeFail("PLAN_REVISION_MISMATCH", nil)
	}
	return nil
}

func admitPlanProposal(
	manifest admittedManifest,
	command journal.Command,
	commands map[string]journal.Command,
	effects map[string]journal.Effect,
) (admittedPlanProposal, error) {
	var wire planProposalCommand
	if json.Unmarshal(command.Payload, &wire) != nil ||
		!bytes.Equal(command.Payload, mustJSON(wire)) ||
		wire.Version != planProposalVersion ||
		!runtimeDigestPattern.MatchString(wire.Authority.Before) ||
		!runtimeDigestPattern.MatchString(wire.Authority.SourceWork) ||
		wire.Authority.Release != manifest.value.Release ||
		wire.Authority.ReleaseRef !=
			"refs/heads/release-wt/"+manifest.value.Release ||
		wire.Authority.TargetRef != manifest.value.TargetRef ||
		wire.Authority.TargetHead == "" ||
		wire.Authority.SourceEffect == "" {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	plan, err := baton.ParsePlan(wire.PlanBytes)
	if err != nil || wire.PlanDigest != plan.Digest() {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	metadata := plan.Metadata()
	if metadata.Release != manifest.value.Release ||
		metadata.Repository != manifest.value.Approval.Repository ||
		metadata.TargetRef != manifest.value.TargetRef {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if _, err := approvalMarker(
		manifest.value.Approval, metadata.ApprovalRef); err != nil {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if metadata.Revision == 1 {
		if metadata.PreviousPlan != nil ||
			wire.Authority.PriorPlan != "" ||
			wire.Authority.ReleaseHead != "" {
			return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
	} else if metadata.Revision < 2 ||
		metadata.PreviousPlan == nil ||
		*metadata.PreviousPlan != wire.Authority.PriorPlan ||
		wire.Authority.PriorPlan == "" ||
		wire.Authority.ReleaseHead == "" {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if wire.Authority.Before != plannerAuthorityBefore(wire.Authority) {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if driverWorkIdentity(
		manifest.digest, "", driver.PlannerProposal,
		metadata.Revision, wire.Authority.Before,
	) != wire.Authority.SourceWork ||
		command.ReplayKey != proposalReplayKey(
			metadata.Revision, wire.Authority.SourceWork) {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	effect, ok := effects[wire.Authority.SourceEffect]
	if !ok || effect.Kind != "driver.dispatch" ||
		effect.State != journal.Succeeded ||
		effect.BeforeDigest != sha256Digest([]byte(wire.Authority.Before)) ||
		!strings.HasPrefix(
			effect.ID,
			"attempt/"+strings.TrimPrefix(wire.Authority.SourceWork, "sha256:")+"/",
		) {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	sourceCommand, ok := commands[effect.ReplayKey]
	if !ok {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var submission driver.Submission
	if manifest.value.production() {
		submission, _, err = validateSucceededDriverResult(
			manifest,
			sourceCommand,
			effect,
		)
	} else {
		if effect.ExpectedDigest != sha256Digest(effect.Result) ||
			validateRecoveryCommand(sourceCommand, effect, false) != nil {
			return admittedPlanProposal{},
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		submission, err = driver.DecodeSubmission(effect.Result)
	}
	if err != nil ||
		submission.Responsibility != driver.PlannerProposal {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	sourcePlan, err := exactBytes(submission.Plan)
	if err != nil || !bytes.Equal(sourcePlan, wire.PlanBytes) {
		return admittedPlanProposal{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return admittedPlanProposal{
		plan: plan, authority: wire.Authority,
		replayKey: command.ReplayKey,
	}, nil
}

func loadRunSnapshot(
	snapshot journal.Snapshot,
	runID string,
) (admittedManifest, []admittedPlanProposal, error) {
	var manifestBytes []byte
	for _, command := range snapshot.Commands {
		if command.ReplayKey == "manifest" {
			if manifestBytes != nil {
				return admittedManifest{}, nil,
					runtimeFail("CORRUPT_JOURNAL", nil)
			}
			manifestBytes = command.Payload
		}
	}
	manifest, err := admitManifest(manifestBytes)
	if err != nil || manifest.value.RunID != runID || manifest.digest != snapshot.Run.ManifestDigest ||
		snapshot.Run.ID != runID {
		return admittedManifest{}, nil,
			runtimeFail("RUN_BINDING_MISMATCH", err)
	}
	effects := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		if _, duplicate := effects[effect.ID]; duplicate {
			return admittedManifest{}, nil,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		effects[effect.ID] = effect
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		if _, duplicate := commands[command.ReplayKey]; duplicate {
			return admittedManifest{}, nil,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
		commands[command.ReplayKey] = command
	}
	proposals := make([]admittedPlanProposal, 0)
	for _, command := range snapshot.Commands {
		if command.Kind != "planner_proposal" {
			continue
		}
		proposal, err := admitPlanProposal(
			manifest, command, commands, effects)
		if err != nil {
			return admittedManifest{}, nil, err
		}
		proposals = append(proposals, proposal)
	}
	return manifest, proposals, nil
}

func (s *Service) loadRun(
	ctx context.Context,
	runID string,
) (admittedManifest, []admittedPlanProposal, error) {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return admittedManifest{}, nil,
			runtimeFail("RUN_NOT_FOUND", err)
	}
	return loadRunSnapshot(snapshot, runID)
}

type ControlCommand = journal.ControlCommand
type ControlReceipt = journal.ControlReceipt

func (s *Service) Control(ctx context.Context, command ControlCommand) (RunStatus, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return RunStatus{}, runtimeFail("INVALID_SERVICE", nil)
	}
	if _, err := s.journal.ApplyControl(
		ctx,
		command,
		s.now().UTC(),
	); err != nil {
		return RunStatus{}, runtimeFail("CONTROL_REJECTED", err)
	}
	if command.Kind != journal.Resume && command.Kind != journal.Takeover {
		return s.Status(ctx, command.RunID)
	}
	projection, err := s.journal.ControlProjection(ctx, command.RunID)
	if err != nil {
		return RunStatus{}, runtimeFail("CONTROL_REJECTED", err)
	}
	if projection.Desired != "running" {
		// An exact replay of an older resume/takeover is still idempotent, but
		// it cannot override a later pause or cancellation.
		return s.Status(ctx, command.RunID)
	}
	owner, err := s.acquireControlOwner(ctx, command, s.now().UTC())
	if journal.IsCode(err, "OWNER_ACTIVE") {
		if command.Kind == journal.Resume {
			// The resume command is durable, but the pausing owner has not
			// released its lease yet. Report the transition explicitly so an
			// exact replay can acquire ownership instead of falsely claiming
			// that delivery has resumed.
			return RunStatus{}, runtimeFail("OWNER_TRANSITION_PENDING", err)
		}
		return s.Status(ctx, command.RunID)
	}
	if err != nil {
		return RunStatus{}, runtimeFail("OWNER_UNAVAILABLE", err)
	}
	return s.driveOwned(ctx, command.RunID, owner)
}

func (s *Service) acquireControlOwner(
	ctx context.Context,
	command ControlCommand,
	now time.Time,
) (journal.OwnerLease, error) {
	takeover := false
	if command.Kind == journal.Takeover {
		// The durable control command is replayable after the owner it created
		// has finished. Only request takeover semantics while an expired
		// claimed owner actually remains; once that owner has released, the
		// exact command replay acquires the pending owner normally.
		_, present, currentErr := s.journal.CurrentOwner(ctx, command.RunID)
		if currentErr != nil {
			return journal.OwnerLease{},
				runtimeFail("OWNER_UNAVAILABLE", currentErr)
		}
		takeover = present
	}
	return s.journal.AcquireOwner(
		ctx,
		command.RunID,
		now,
		ownerDuration(),
		takeover,
	)
}

func exactBytes(value *driver.ExactBytes) ([]byte, error) {
	if value == nil {
		return nil, runtimeFail("MISSING_EXACT_BYTES", nil)
	}
	body, err := base64.StdEncoding.Strict().DecodeString(value.Bytes)
	if err != nil || int64(len(body)) != value.ByteCount || driver.Digest(body) != value.Digest {
		return nil, runtimeFail("INVALID_EXACT_BYTES", nil)
	}
	return body, nil
}

func mustJSON(value any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return append(body, '\n')
}

func stableErrorCode(err error) string {
	var runtimeErr *Error
	if errors.As(err, &runtimeErr) && runtimeIdentityPattern.MatchString(runtimeErr.Code) {
		return runtimeErr.Code
	}
	var gitErr *gitx.Error
	if errors.As(err, &gitErr) &&
		runtimeIdentityPattern.MatchString(gitErr.Code) {
		return gitErr.Code
	}
	return "operational_failure"
}

func refVectorEqual(left, right []gitx.RefHead) bool {
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
