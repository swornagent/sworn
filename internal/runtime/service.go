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
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

const effectLease = 5 * time.Minute

// testCrashAfterEffect is empty in every official build. Real-binary crash-cut
// tests may replace it at link time to terminate after one named external
// effect and before its journal completion.
var testCrashAfterEffect string

type Service struct {
	journal       *journal.Store
	resolver      approvalResolver
	dispatcher    driver.Driver
	gitExecutable string
	now           func() time.Time
}

type RunStatus struct {
	SchemaVersion  string         `json:"schema_version"`
	RunID          string         `json:"run_id"`
	State          string         `json:"state"`
	ManifestDigest string         `json:"manifest_digest"`
	PlanDigest     string         `json:"plan_digest,omitempty"`
	TargetRef      string         `json:"target_ref"`
	TargetHead     string         `json:"target_head,omitempty"`
	ReleaseHead    string         `json:"release_head,omitempty"`
	Outcome        string         `json:"outcome,omitempty"`
	Effects        []EffectStatus `json:"effects"`
	EventOffset    int64          `json:"event_offset"`
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
	registry   driver.SelectionRegistry
	inertness  baton.InertnessResolver
}

type sealedRecord struct {
	Before       string   `json:"before"`
	Candidate    string   `json:"candidate"`
	Tree         string   `json:"tree"`
	ChangedPaths []string `json:"changed_paths"`
}

func OpenService(ctx context.Context, journalPath string) (*Service, error) {
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return nil, err
	}
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		return nil, runtimeFail("JOURNAL_UNAVAILABLE", err)
	}
	return &Service{
		journal: store,
		resolver: newProductionApprovalResolver(func() (string, error) {
			return os.Getenv("SWORN_GITHUB_TOKEN"), nil
		}),
		dispatcher:    driver.Dispatcher{},
		gitExecutable: gitExecutable,
		now:           time.Now,
	}, nil
}

func OpenStatusService(ctx context.Context, journalPath string) (*Service, error) {
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return nil, err
	}
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		return nil, runtimeFail("JOURNAL_UNAVAILABLE", err)
	}
	return &Service{
		journal: store, gitExecutable: gitExecutable, now: time.Now,
	}, nil
}

func resolveGitExecutable() (string, error) {
	gitExecutable, err := exec.LookPath("git")
	if err != nil {
		return "", runtimeFail("GIT_UNAVAILABLE", nil)
	}
	gitExecutable, err = filepath.Abs(gitExecutable)
	if err != nil {
		return "", runtimeFail("GIT_UNAVAILABLE", nil)
	}
	gitExecutable, err = filepath.EvalSymlinks(gitExecutable)
	if err != nil {
		return "", runtimeFail("GIT_UNAVAILABLE", nil)
	}
	return gitExecutable, nil
}

func newService(
	store *journal.Store,
	resolver approvalResolver,
	dispatcher driver.Driver,
	gitExecutable string,
	now func() time.Time,
) (*Service, error) {
	if store == nil || resolver == nil || dispatcher == nil ||
		!filepath.IsAbs(gitExecutable) || now == nil {
		return nil, runtimeFail("INVALID_SERVICE", nil)
	}
	return &Service{
		journal: store, resolver: resolver, dispatcher: dispatcher,
		gitExecutable: gitExecutable, now: now,
	}, nil
}

func (s *Service) Close() error {
	if s == nil || s.journal == nil {
		return nil
	}
	return s.journal.Close()
}

func (s *Service) openEngine(manifest admittedManifest) (*engine, error) {
	repository, err := gitx.Open(manifest.value.Repository, s.gitExecutable)
	if err != nil {
		return nil, runtimeFail("GIT_UNAVAILABLE", err)
	}
	if repository.Root() != manifest.value.Repository {
		return nil, runtimeFail("REPOSITORY_BINDING_MISMATCH", nil)
	}
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{
			Kind:       request.Kind,
			Repository: request.Repository,
			RecordRoot: request.RecordRoot,
			Commit:     request.Commit,
			Decision:   "inert",
		}, nil
	}
	gitRepository := baton.UseGitRepository(repository)
	actions, err := baton.NewActions(gitRepository, inertness)
	if err != nil {
		return nil, runtimeFail("BATON_UNAVAILABLE", err)
	}
	workspaces, err := gitx.NewWorkspaces(repository)
	if err != nil {
		return nil, runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	adapter, err := driver.NewProcessAdapter(
		manifest.value.Driver.AdapterKey,
		driver.FakeDriverID,
		driver.FakeDriverVersion,
		driver.ExecutableIdentity{
			Path: manifest.value.Driver.Executable, Digest: manifest.value.Driver.Digest,
		},
	)
	if err != nil {
		_ = workspaces.Close()
		return nil, runtimeFail("DRIVER_UNAVAILABLE", err)
	}
	registry, err := driver.NewSelectionRegistry(
		[]driver.ProfileConfig{{
			Key: manifest.value.Driver.Profile, Adapter: manifest.value.Driver.AdapterKey,
			Network: driver.NetworkNone,
		}},
		[]driver.Adapter{adapter},
	)
	if err != nil {
		_ = workspaces.Close()
		return nil, runtimeFail("DRIVER_UNAVAILABLE", err)
	}
	return &engine{
		manifest: manifest, repository: repository, git: gitRepository,
		actions: actions, installer: newAuthorityInstaller(actions),
		workspaces: workspaces, registry: registry, inertness: inertness,
	}, nil
}

func (e *engine) Close() error {
	if e == nil || e.workspaces == nil {
		return nil
	}
	return e.workspaces.Close()
}

func (s *Service) Start(ctx context.Context, manifestBytes []byte) (RunStatus, error) {
	if s == nil || s.journal == nil || ctx == nil {
		return RunStatus{}, runtimeFail("INVALID_SERVICE", nil)
	}
	manifest, err := admitManifest(manifestBytes)
	if err != nil {
		return RunStatus{}, err
	}
	now := s.now().UTC()
	if err := s.journal.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository, Release: manifest.value.Release,
		TargetRef: manifest.value.TargetRef, CreatedAt: now,
	}); err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	if err := s.journal.RecordCommand(ctx, journal.Command{
		RunID: manifest.value.RunID, ReplayKey: "manifest", Kind: "start",
		Payload: manifest.raw, CreatedAt: now,
	}); err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	engine, err := s.openEngine(manifest)
	if err != nil {
		return RunStatus{}, err
	}
	defer engine.Close()
	refNames := []string{
		manifest.value.TargetRef,
		"refs/heads/release-wt/" + manifest.value.Release,
		"refs/heads/track/" + manifest.value.Release + "/" + manifest.value.ActiveTrack,
	}
	before, err := engine.repository.CaptureHeadRefs(refNames)
	if err != nil || len(before) != len(refNames) {
		return RunStatus{}, runtimeFail("INVALID_AUTHORITY_STATE", err)
	}
	beforeByRef := refsByName(before)
	trackBefore := beforeByRef[refNames[2]]
	if trackBefore.State != gitx.RefAbsent && trackBefore.State != gitx.RefDirect {
		return RunStatus{}, runtimeFail("INVALID_AUTHORITY_STATE", nil)
	}
	targetBefore := beforeByRef[manifest.value.TargetRef]
	if targetBefore.State != gitx.RefDirect || targetBefore.Head.IsZero() {
		return RunStatus{}, runtimeFail("TARGET_NOT_FOUND", nil)
	}
	plannerWorkspace, err := engine.workspaces.OpenSnapshot(targetBefore.Head)
	if err != nil {
		return RunStatus{}, runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	submission, err := s.runDriverEffect(
		ctx, engine, plannerWorkspace, driver.RolePlanner, driver.PlannerProposal,
		manifest.digest,
	)
	closeErr := plannerWorkspace.Close()
	if err != nil {
		return RunStatus{}, err
	}
	if closeErr != nil {
		return RunStatus{}, runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	planBytes, err := exactBytes(submission.Plan)
	if err != nil {
		return RunStatus{}, err
	}
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		return RunStatus{}, runtimeFail("INVALID_PLAN", err)
	}
	if err := validatePlanBinding(manifest, plan); err != nil {
		return RunStatus{}, err
	}
	if err := s.journal.RecordCommand(ctx, journal.Command{
		RunID: manifest.value.RunID, ReplayKey: "plan-proposal",
		Kind: "planner_proposal", Payload: planBytes, CreatedAt: s.now().UTC(),
	}); err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	after, err := engine.repository.CaptureHeadRefs(refNames)
	if err != nil || !refVectorEqual(before, after) {
		return RunStatus{}, runtimeFail("PLANNER_MUTATED_AUTHORITY", err)
	}
	if err := s.journal.AppendEvent(
		ctx,
		manifest.value.RunID,
		"awaiting_approval",
		[]byte(plan.Digest()),
		s.now().UTC(),
	); err != nil {
		return RunStatus{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return s.Status(ctx, manifest.value.RunID)
}

func validatePlanBinding(manifest admittedManifest, plan baton.Plan) error {
	metadata := plan.Metadata()
	if metadata.Release != manifest.value.Release ||
		metadata.Repository != manifest.value.Approval.Repository ||
		metadata.TargetRef != manifest.value.TargetRef {
		return runtimeFail("PLAN_BINDING_MISMATCH", nil)
	}
	track, slice, ok := plan.FindSlice(manifest.value.ActiveSlice)
	if !ok || track.ID != manifest.value.ActiveTrack || slice.ID == "" {
		return runtimeFail("PLAN_BINDING_MISMATCH", nil)
	}
	expectedApproval := fmt.Sprintf(
		"github://%s/issues/%d#%s",
		manifest.value.Approval.Repository,
		manifest.value.Approval.Issue,
		manifest.value.Approval.Marker,
	)
	if metadata.ApprovalRef != expectedApproval {
		return runtimeFail("PLAN_BINDING_MISMATCH", nil)
	}
	return nil
}

func refVectorEqual(left, right []gitx.RefHead) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Ref != right[index].Ref ||
			left[index].State != right[index].State ||
			left[index].Head != right[index].Head ||
			left[index].Target != right[index].Target {
			return false
		}
	}
	return true
}

func refsByName(values []gitx.RefHead) map[string]gitx.RefHead {
	result := make(map[string]gitx.RefHead, len(values))
	for _, value := range values {
		result[value.Ref] = value
	}
	return result
}

func (s *Service) loadRun(
	ctx context.Context,
	runID string,
) (admittedManifest, baton.Plan, error) {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return admittedManifest{}, baton.Plan{}, runtimeFail("RUN_NOT_FOUND", err)
	}
	var manifestBytes, planBytes []byte
	for _, command := range snapshot.Commands {
		switch command.ReplayKey {
		case "manifest":
			manifestBytes = command.Payload
		case "plan-proposal":
			planBytes = command.Payload
		}
	}
	if len(manifestBytes) == 0 || len(planBytes) == 0 {
		return admittedManifest{}, baton.Plan{}, runtimeFail("RUN_NOT_READY", nil)
	}
	manifest, err := admitManifest(manifestBytes)
	if err != nil || manifest.value.RunID != runID ||
		manifest.digest != snapshot.Run.ManifestDigest {
		return admittedManifest{}, baton.Plan{}, runtimeFail("RUN_BINDING_MISMATCH", err)
	}
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		return admittedManifest{}, baton.Plan{}, runtimeFail("INVALID_PLAN", err)
	}
	if err := validatePlanBinding(manifest, plan); err != nil {
		return admittedManifest{}, baton.Plan{}, err
	}
	return manifest, plan, nil
}

func (s *Service) Resume(ctx context.Context, runID string) (RunStatus, error) {
	if s == nil || s.journal == nil || ctx == nil ||
		!runtimeIdentityPattern.MatchString(runID) {
		return RunStatus{}, runtimeFail("INVALID_RUN", nil)
	}
	manifest, plan, err := s.loadRun(ctx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	engine, err := s.openEngine(manifest)
	if err != nil {
		return RunStatus{}, err
	}
	defer engine.Close()
	admission, err := s.resolver.resolve(ctx, manifest, plan)
	if err != nil {
		return RunStatus{}, err
	}
	if _, err := s.runActionEffect(
		ctx,
		manifest,
		"baton.install",
		admission.evidence,
		func() (baton.ActionResult, error) {
			return engine.installer.install(admission)
		},
	); err != nil {
		return RunStatus{}, err
	}
	if err := s.runApprovedFlow(ctx, engine, plan); err != nil {
		return RunStatus{}, err
	}
	return s.Status(ctx, runID)
}

func (s *Service) runApprovedFlow(
	ctx context.Context,
	engine *engine,
	plan baton.Plan,
) error {
	manifest := engine.manifest
	key := gitx.TrackKey{
		Release: manifest.value.Release, Track: manifest.value.ActiveTrack,
	}
	designWorkspace, err := engine.workspaces.OpenTrack(key, gitx.DesignView)
	if err != nil {
		return runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	design, err := s.runDriverEffect(
		ctx, engine, designWorkspace, driver.RoleImplementer,
		driver.ImplementerDesign, plan.Digest(),
	)
	closeErr := designWorkspace.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	designInput := baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: manifest.value.ActiveSlice,
		Role: "implementer", Result: "designed",
		Summary: design.Summary, Detail: []byte(design.Detail),
	}
	if _, err := s.runActionEffect(
		ctx,
		manifest,
		"baton.design",
		mustJSON(designInput),
		func() (baton.ActionResult, error) {
			return engine.actions.AppendReceipt(designInput)
		},
	); err != nil {
		return err
	}

	captainWorkspace, err := engine.workspaces.OpenTrack(key, gitx.CaptainView)
	if err != nil {
		return runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	captain, err := s.runDriverEffect(
		ctx, engine, captainWorkspace, driver.RoleCaptain,
		driver.CaptainReview, design.Summary,
	)
	closeErr = captainWorkspace.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	captainResult := string(captain.Decision.Outcome)
	captainInput := baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: manifest.value.ActiveSlice,
		Role: "captain", Result: captainResult,
		Summary: captain.Summary, Detail: []byte(captain.Detail),
	}
	if _, err := s.runActionEffect(
		ctx,
		manifest,
		"baton.captain",
		mustJSON(captainInput),
		func() (baton.ActionResult, error) {
			return engine.actions.AppendReceipt(captainInput)
		},
	); err != nil {
		return err
	}
	if captain.Decision.Outcome != driver.DecisionProceed {
		return runtimeFail("CAPTAIN_STOPPED", nil)
	}

	sealed, implementation, foundSeal, err := s.loadOrRecoverSeal(ctx, engine, key)
	if err != nil {
		return err
	}
	if !foundSeal {
		implementationWorkspace, err := engine.workspaces.OpenTrack(
			key,
			gitx.ImplementationView,
		)
		if err != nil {
			return runtimeFail("WORKSPACE_UNAVAILABLE", err)
		}
		implementation, err = s.runDriverEffect(
			ctx, engine, implementationWorkspace, driver.RoleImplementer,
			driver.ImplementerImplementation, captain.Summary,
		)
		if err != nil {
			_ = implementationWorkspace.Close()
			return err
		}
		var sealClaim journal.Claim
		sealed, err = engine.workspaces.SealTrackWithClaim(
			implementationWorkspace,
			func(prepared gitx.SealedCandidate) error {
				if err := baton.ValidateSliceCandidateScope(
					engine.git,
					engine.inertness,
					plan,
					manifest.value.ActiveSlice,
					prepared.Before.String(),
					prepared.Candidate.String(),
				); err != nil {
					return runtimeFail("CANDIDATE_SCOPE_FAILED", err)
				}
				record := sealedRecordFromCandidate(prepared)
				payload := mustJSON(record)
				if err := s.ensureEffect(
					ctx,
					manifest,
					"git.seal",
					"git.seal",
					payload,
					sha256Digest([]byte(prepared.Before.String())),
					sha256Digest([]byte(prepared.Candidate.String())),
				); err != nil {
					return err
				}
				effect, err := s.journal.Effect(ctx, manifest.value.RunID, "git.seal")
				if err != nil {
					return runtimeFail("JOURNAL_READ_FAILED", err)
				}
				switch effect.State {
				case journal.Pending:
					sealClaim, err = s.journal.Claim(
						ctx,
						manifest.value.RunID,
						"git.seal",
						s.now().UTC(),
						effectLease,
					)
					if err != nil {
						return runtimeFail("EFFECT_CLAIM_FAILED", err)
					}
					return nil
				case journal.Claimed:
					return runtimeFail("RECOVERY_REQUIRED", nil)
				case journal.Succeeded:
					return runtimeFail("EFFECT_ALREADY_COMPLETE", nil)
				default:
					return runtimeFail("EFFECT_PARKED", nil)
				}
			},
		)
		closeErr = implementationWorkspace.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
		}
		sealResult := mustJSON(sealedRecordFromCandidate(sealed))
		if err := s.journal.Complete(ctx, journal.Completion{
			RunID: manifest.value.RunID, EffectID: "git.seal", Token: sealClaim.Token,
			State: journal.Succeeded, Result: sealResult,
			Receipts: []journal.Receipt{{
				Kind: "git_candidate", Body: sealResult,
			}},
			EventKind: "candidate_sealed", EventBody: sealResult, At: s.now().UTC(),
		}); err != nil {
			return runtimeFail("JOURNAL_WRITE_FAILED", err)
		}
	}
	implementationChecks, err := exactBytes(implementation.Checks)
	if err != nil {
		return err
	}
	candidateInput := baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: manifest.value.ActiveSlice,
		Role: "implementer", Result: "candidate",
		Summary: implementation.Summary, Detail: []byte(implementation.Detail),
		Candidate: sealed.Candidate.String(), CheckResults: implementationChecks,
	}
	if _, err := s.runActionEffect(
		ctx,
		manifest,
		"baton.candidate",
		mustJSON(candidateInput),
		func() (baton.ActionResult, error) {
			return engine.actions.AppendReceipt(candidateInput)
		},
	); err != nil {
		return err
	}

	workVerifier, err := engine.workspaces.OpenCandidate(
		key,
		gitx.WorkVerifierView,
		sealed.Candidate,
	)
	if err != nil {
		return runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	workVerdict, err := s.runDriverEffect(
		ctx, engine, workVerifier, driver.RoleVerifier,
		driver.WorkVerification, sealed.Candidate.String(),
	)
	closeErr = workVerifier.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	workChecks, err := exactBytes(workVerdict.Checks)
	if err != nil {
		return err
	}
	workInput := baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: manifest.value.ActiveSlice,
		Role: "verifier", Result: string(workVerdict.Decision.Outcome),
		Summary: workVerdict.Summary, Detail: []byte(workVerdict.Detail),
		Candidate: sealed.Candidate.String(), CheckResults: workChecks,
	}
	if _, err := s.runActionEffect(
		ctx,
		manifest,
		"baton.work_verdict",
		mustJSON(workInput),
		func() (baton.ActionResult, error) {
			return engine.actions.AppendReceipt(workInput)
		},
	); err != nil {
		return err
	}
	if workVerdict.Decision.Outcome != driver.DecisionPass {
		return runtimeFail("WORK_VERIFICATION_STOPPED", nil)
	}

	assemblyInput := baton.PrepareAssemblyInput{
		Release: manifest.value.Release,
		Summary: "Compose all exact passed track candidates.",
		Detail:  []byte("Deterministic engine-owned composition."),
	}
	assembly, err := s.runActionEffect(
		ctx,
		manifest,
		"baton.prepare_assembly",
		mustJSON(assemblyInput),
		func() (baton.ActionResult, error) {
			return engine.actions.PrepareAssembly(assemblyInput)
		},
	)
	if err != nil {
		return err
	}
	if assembly.Direct {
		return runtimeFail("DISTINCT_ASSEMBLY_VERIFICATION_REQUIRED", nil)
	}
	assemblyOID, err := gitx.ParseOID(
		engine.repository.ObjectFormat(),
		assembly.Candidate,
	)
	if err != nil {
		return runtimeFail("INVALID_ASSEMBLY_CANDIDATE", err)
	}
	assemblyVerifier, err := engine.workspaces.OpenCandidate(
		key,
		gitx.AssemblyVerifierView,
		assemblyOID,
	)
	if err != nil {
		return runtimeFail("WORKSPACE_UNAVAILABLE", err)
	}
	assemblyVerdict, err := s.runDriverEffect(
		ctx, engine, assemblyVerifier, driver.RoleVerifier,
		driver.AssemblyVerification, assembly.Candidate,
	)
	closeErr = assemblyVerifier.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return runtimeFail("WORKSPACE_CLEANUP_FAILED", closeErr)
	}
	assemblyChecks, err := exactBytes(assemblyVerdict.Checks)
	if err != nil {
		return err
	}
	assemblyVerdictInput := baton.AppendReceiptInput{
		Release: manifest.value.Release,
		Role:    "verifier", Result: string(assemblyVerdict.Decision.Outcome),
		Summary: assemblyVerdict.Summary, Detail: []byte(assemblyVerdict.Detail),
		Candidate: assembly.Candidate, CheckResults: assemblyChecks,
	}
	if _, err := s.runActionEffect(
		ctx,
		manifest,
		"baton.assembly_verdict",
		mustJSON(assemblyVerdictInput),
		func() (baton.ActionResult, error) {
			return engine.actions.AppendReceipt(assemblyVerdictInput)
		},
	); err != nil {
		return err
	}
	if assemblyVerdict.Decision.Outcome != driver.DecisionPass {
		return runtimeFail("ASSEMBLY_VERIFICATION_STOPPED", nil)
	}
	mergeInput := baton.MergePassedCandidateInput{
		Release: manifest.value.Release,
		Summary: "Merge the exact independently verified assembly candidate.",
		Detail:  []byte("Deterministic Merge; no model dispatch."),
	}
	merged, err := s.runActionEffectWithRecovery(
		ctx,
		manifest,
		"baton.merge",
		mustJSON(mergeInput),
		func() (baton.ActionResult, error) {
			return engine.actions.MergePassedCandidate(mergeInput)
		},
		func() (journal.RecoveryDisposition, baton.ActionResult, error) {
			state, err := baton.ReadState(
				engine.git,
				manifest.value.Release,
				engine.inertness,
			)
			if err != nil {
				return journal.RecoveryAmbiguous, baton.ActionResult{}, nil
			}
			if state.Assembly.Outcome == "merged" {
				result, err := engine.actions.MergePassedCandidate(mergeInput)
				return journal.RecoveryAllNew, result, err
			}
			if state.Assembly.Pass != nil && !state.Plan.TargetStale {
				return journal.RecoveryAllOld, baton.ActionResult{}, nil
			}
			return journal.RecoveryAmbiguous, baton.ActionResult{}, nil
		},
	)
	if err != nil {
		return err
	}
	target, err := engine.repository.CaptureHeadRefs([]string{manifest.value.TargetRef})
	if err != nil || len(target) != 1 || target[0].State != gitx.RefDirect ||
		target[0].Head.String() != merged.ResultCommit {
		return runtimeFail("MERGE_RECONCILIATION_FAILED", err)
	}
	return nil
}

func (s *Service) ensureEffect(
	ctx context.Context,
	manifest admittedManifest,
	replayKey, kind string,
	payload []byte,
	beforeDigest, expectedDigest string,
) error {
	now := s.now().UTC()
	if err := s.journal.RecordCommand(ctx, journal.Command{
		RunID: manifest.value.RunID, ReplayKey: replayKey,
		Kind: kind, Payload: payload, CreatedAt: now,
	}); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	if err := s.journal.EnsureEffect(ctx, journal.Effect{
		RunID: manifest.value.RunID, ID: replayKey, ReplayKey: replayKey,
		Kind: kind, BeforeDigest: beforeDigest, ExpectedDigest: expectedDigest,
		UpdatedAt: now,
	}); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

func sealedRecordFromCandidate(candidate gitx.SealedCandidate) sealedRecord {
	return sealedRecord{
		Before: candidate.Before.String(), Candidate: candidate.Candidate.String(),
		Tree: candidate.Tree.String(), ChangedPaths: append([]string(nil), candidate.ChangedPaths...),
	}
}

func sealedCandidateFromRecord(
	repository *gitx.Repository,
	record sealedRecord,
) (gitx.SealedCandidate, error) {
	before, err := gitx.ParseOID(repository.ObjectFormat(), record.Before)
	if err != nil {
		return gitx.SealedCandidate{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	candidate, err := gitx.ParseOID(repository.ObjectFormat(), record.Candidate)
	if err != nil {
		return gitx.SealedCandidate{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	tree, err := gitx.ParseOID(repository.ObjectFormat(), record.Tree)
	if err != nil {
		return gitx.SealedCandidate{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	return gitx.SealedCandidate{
		Before: before, Candidate: candidate, Tree: tree,
		ChangedPaths: append([]string(nil), record.ChangedPaths...),
	}, nil
}

func (s *Service) cachedSubmission(
	ctx context.Context,
	manifest admittedManifest,
	responsibility driver.Responsibility,
) (driver.Submission, error) {
	effect, err := s.journal.Effect(
		ctx,
		manifest.value.RunID,
		"dispatch."+string(responsibility),
	)
	if err != nil || effect.State != journal.Succeeded {
		return driver.Submission{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	submission, err := driver.DecodeSubmission(effect.Result)
	if err != nil || submission.Responsibility != responsibility {
		return driver.Submission{}, runtimeFail("CORRUPT_JOURNAL", err)
	}
	expected, err := base64.StdEncoding.Strict().DecodeString(
		manifest.value.Submissions.forResponsibility(responsibility),
	)
	if err != nil || !bytes.Equal(effect.Result, expected) {
		return driver.Submission{}, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return submission, nil
}

func (s *Service) loadOrRecoverSeal(
	ctx context.Context,
	engine *engine,
	key gitx.TrackKey,
) (gitx.SealedCandidate, driver.Submission, bool, error) {
	runID := engine.manifest.value.RunID
	effect, err := s.journal.Effect(ctx, runID, "git.seal")
	if journal.IsCode(err, "EFFECT_NOT_FOUND") {
		return gitx.SealedCandidate{}, driver.Submission{}, false, nil
	}
	if err != nil {
		return gitx.SealedCandidate{}, driver.Submission{}, false,
			runtimeFail("JOURNAL_READ_FAILED", err)
	}
	var body []byte
	switch effect.State {
	case journal.Succeeded:
		body = effect.Result
	case journal.Claimed:
		snapshot, err := s.journal.Snapshot(ctx, runID)
		if err != nil {
			return gitx.SealedCandidate{}, driver.Submission{}, false,
				runtimeFail("JOURNAL_READ_FAILED", err)
		}
		for _, command := range snapshot.Commands {
			if command.ReplayKey == "git.seal" {
				body = command.Payload
				break
			}
		}
		if len(body) == 0 {
			return gitx.SealedCandidate{}, driver.Submission{}, false,
				runtimeFail("CORRUPT_JOURNAL", nil)
		}
	default:
		if effect.State == journal.Pending {
			return gitx.SealedCandidate{}, driver.Submission{}, false, nil
		}
		return gitx.SealedCandidate{}, driver.Submission{}, false,
			runtimeFail("EFFECT_PARKED", nil)
	}
	var record sealedRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return gitx.SealedCandidate{}, driver.Submission{}, false,
			runtimeFail("CORRUPT_JOURNAL", nil)
	}
	sealed, err := sealedCandidateFromRecord(engine.repository, record)
	if err != nil {
		return gitx.SealedCandidate{}, driver.Submission{}, false, err
	}
	if effect.State == journal.Claimed {
		disposition, err := engine.workspaces.ReconcileSeal(
			key,
			sealed.Before,
			sealed.Candidate,
		)
		if err != nil {
			return gitx.SealedCandidate{}, driver.Submission{}, false,
				runtimeFail("RECOVERY_FAILED", err)
		}
		switch disposition {
		case gitx.SealAllNew:
			if err := s.journal.Reconcile(ctx, journal.Completion{
				RunID: runID, EffectID: "git.seal", Token: effect.CurrentClaim,
				State: journal.Succeeded, Result: body,
				Receipts:  []journal.Receipt{{Kind: "git_candidate", Body: body}},
				EventKind: "seal_reconciled_all_new", EventBody: body, At: s.now().UTC(),
			}, journal.RecoveryAllNew); err != nil {
				return gitx.SealedCandidate{}, driver.Submission{}, false,
					runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
		case gitx.SealAllOld:
			if err := s.journal.Reconcile(ctx, journal.Completion{
				RunID: runID, EffectID: "git.seal", Token: effect.CurrentClaim,
				EventKind: "seal_reconciled_all_old", EventBody: body, At: s.now().UTC(),
			}, journal.RecoveryAllOld); err != nil {
				return gitx.SealedCandidate{}, driver.Submission{}, false,
					runtimeFail("JOURNAL_WRITE_FAILED", err)
			}
			return gitx.SealedCandidate{}, driver.Submission{}, false,
				runtimeFail("RECOVERY_RECONCILED", nil)
		default:
			_ = s.journal.Reconcile(ctx, journal.Completion{
				RunID: runID, EffectID: "git.seal", Token: effect.CurrentClaim,
				EventKind: "seal_reconciled_ambiguous", EventBody: body, At: s.now().UTC(),
			}, journal.RecoveryAmbiguous)
			return gitx.SealedCandidate{}, driver.Submission{}, false,
				runtimeFail("RECOVERY_UNCERTAIN", nil)
		}
	}
	implementation, err := s.cachedSubmission(
		ctx,
		engine.manifest,
		driver.ImplementerImplementation,
	)
	if err != nil {
		return gitx.SealedCandidate{}, driver.Submission{}, false, err
	}
	return sealed, implementation, true, nil
}

func (s *Service) runActionEffect(
	ctx context.Context,
	manifest admittedManifest,
	kind string,
	payload []byte,
	action func() (baton.ActionResult, error),
) (baton.ActionResult, error) {
	return s.runActionEffectWithRecovery(
		ctx, manifest, kind, payload, action, nil,
	)
}

type actionRecovery func() (
	journal.RecoveryDisposition,
	baton.ActionResult,
	error,
)

func (s *Service) runActionEffectWithRecovery(
	ctx context.Context,
	manifest admittedManifest,
	kind string,
	payload []byte,
	action func() (baton.ActionResult, error),
	recover actionRecovery,
) (baton.ActionResult, error) {
	beforeDigest := sha256Digest([]byte(manifest.value.RunID + ":" + kind))
	expectedDigest := sha256Digest(payload)
	if err := s.ensureEffect(
		ctx, manifest, kind, kind, payload, beforeDigest, expectedDigest,
	); err != nil {
		return baton.ActionResult{}, err
	}
	effect, err := s.journal.Effect(ctx, manifest.value.RunID, kind)
	if err != nil {
		return baton.ActionResult{}, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if effect.State == journal.Succeeded {
		var cached baton.ActionResult
		if err := json.Unmarshal(effect.Result, &cached); err != nil {
			return baton.ActionResult{}, runtimeFail("CORRUPT_JOURNAL", nil)
		}
		return cached, nil
	}
	if effect.State == journal.Claimed {
		if recover != nil {
			disposition, recovered, recoverErr := recover()
			if recoverErr != nil {
				return baton.ActionResult{}, runtimeFail("RECOVERY_FAILED", recoverErr)
			}
			switch disposition {
			case journal.RecoveryAllNew:
				body := mustJSON(recovered)
				if err := s.journal.Reconcile(ctx, journal.Completion{
					RunID: manifest.value.RunID, EffectID: kind,
					Token: effect.CurrentClaim, State: journal.Succeeded,
					Result: body,
					Receipts: []journal.Receipt{{
						Kind: "baton_action_result", Body: body,
					}},
					EventKind: "effect_reconciled_all_new",
					EventBody: []byte(kind), At: s.now().UTC(),
				}, disposition); err != nil {
					return baton.ActionResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
				}
				return recovered, nil
			case journal.RecoveryAllOld:
				if err := s.journal.Reconcile(ctx, journal.Completion{
					RunID: manifest.value.RunID, EffectID: kind,
					Token:     effect.CurrentClaim,
					EventKind: "effect_reconciled_all_old",
					EventBody: []byte(kind), At: s.now().UTC(),
				}, disposition); err != nil {
					return baton.ActionResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
				}
				return baton.ActionResult{}, runtimeFail("RECOVERY_RECONCILED", nil)
			}
		}
		_ = s.journal.Reconcile(ctx, journal.Completion{
			RunID: manifest.value.RunID, EffectID: kind,
			Token: effect.CurrentClaim, EventKind: "effect_uncertain",
			EventBody: []byte(kind), At: s.now().UTC(),
		}, journal.RecoveryAmbiguous)
		return baton.ActionResult{}, runtimeFail("RECOVERY_UNCERTAIN", nil)
	}
	if effect.State != journal.Pending {
		return baton.ActionResult{}, runtimeFail("EFFECT_PARKED", nil)
	}
	claim, err := s.journal.Claim(
		ctx,
		manifest.value.RunID,
		kind,
		s.now().UTC(),
		effectLease,
	)
	if err != nil {
		return baton.ActionResult{}, runtimeFail("EFFECT_CLAIM_FAILED", err)
	}
	result, actionErr := action()
	if actionErr != nil {
		completeErr := s.journal.Complete(ctx, journal.Completion{
			RunID: manifest.value.RunID, EffectID: kind, Token: claim.Token,
			State: journal.OperationalFailed, ErrorCode: "baton_action_failed",
			EventKind: "effect_operational_failure", EventBody: []byte(kind),
			At: s.now().UTC(),
		})
		if completeErr != nil {
			return baton.ActionResult{}, runtimeFail("JOURNAL_WRITE_FAILED", completeErr)
		}
		return baton.ActionResult{}, runtimeFail("BATON_ACTION_FAILED", actionErr)
	}
	if testCrashAfterEffect == kind {
		os.Exit(86)
	}
	body := mustJSON(result)
	if err := s.journal.Complete(ctx, journal.Completion{
		RunID: manifest.value.RunID, EffectID: kind, Token: claim.Token,
		State: journal.Succeeded, Result: body,
		Receipts: []journal.Receipt{{
			Kind: "baton_action_result", Body: body,
		}},
		EventKind: "baton_action_completed", EventBody: []byte(kind),
		At: s.now().UTC(),
	}); err != nil {
		return baton.ActionResult{}, runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return result, nil
}

func exactBytes(value *driver.ExactBytes) ([]byte, error) {
	if value == nil {
		return nil, runtimeFail("MISSING_EXACT_BYTES", nil)
	}
	body, err := base64.StdEncoding.Strict().DecodeString(value.Bytes)
	if err != nil || int64(len(body)) != value.ByteCount ||
		driver.Digest(body) != value.Digest {
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
	if errors.As(err, &runtimeErr) &&
		runtimeIdentityPattern.MatchString(runtimeErr.Code) {
		return runtimeErr.Code
	}
	return "operational_failure"
}
