package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func TestV4ProjectAuthorityIsOptionalExactAndClosed(t *testing.T) {
	manifest, body, plan := fixtureManifest(t)
	if manifest.Authority.BootstrapApprovedPlanDigest == nil ||
		*manifest.Authority.BootstrapApprovedPlanDigest != plan.Digest() {
		t.Fatal("fixture bootstrap authority is not exact")
	}

	absent := manifest
	absent.Authority.BootstrapApprovedPlanDigest = nil
	absentBody, err := canonicalManifest(absent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admitManifest(absentBody); err != nil {
		t.Fatalf("absent optional authority = %v", err)
	}

	for name, digest := range map[string]string{
		"empty": "", "uppercase": "sha256:" + strings.Repeat("A", 64),
		"short": "sha256:abcd", "wrong algorithm": "sha512:" + strings.Repeat("a", 64),
	} {
		t.Run(name, func(t *testing.T) {
			value := manifest
			value.Authority.BootstrapApprovedPlanDigest = &digest
			raw, _ := json.Marshal(value)
			if _, err := admitManifest(append(raw, '\n')); !IsCode(err, "INVALID_AUTHORITY") {
				t.Fatalf("malformed digest = %v", err)
			}
		})
	}
	for name, authorizer := range map[string]string{
		"empty": "", "url": "https://operator", "account": "operator/user",
		"overlong": strings.Repeat("a", 129),
	} {
		t.Run("authorizer_"+name, func(t *testing.T) {
			value := manifest
			value.Authority.ExternalAuthorizer = authorizer
			raw, _ := json.Marshal(value)
			if _, err := admitManifest(append(raw, '\n')); !IsCode(err, "INVALID_AUTHORITY") {
				t.Fatalf("external authorizer = %v", err)
			}
		})
	}
	withGitHubField := strings.Replace(
		string(body), `"authority":{`,
		`"approval":{"repository":"host/project"},"authority":{`, 1)
	if _, err := admitManifest([]byte(withGitHubField)); !IsCode(err, "INVALID_MANIFEST") {
		t.Fatalf("former hosted approval field = %v", err)
	}
}

func TestEffectiveAuthorityRejectsEveryDistinctBootstrapJournalCombination(t *testing.T) {
	_, body, plan := fixtureManifest(t)
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	command := func(digest string) journal.Command {
		return journal.Command{
			ReplayKey: "plan-authority/" + strings.TrimPrefix(digest, "sha256:"),
			Kind:      "plan_authority",
			Payload: mustJSON(planAuthorityCommand{
				Version: planAuthorityVersion, PlanDigest: digest,
			}),
		}
	}
	if got, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{command(plan.Digest())},
	}); err != nil || got != plan.Digest() {
		t.Fatalf("identical bootstrap and journal authority = %q, %v", got, err)
	}
	other := "sha256:" + strings.Repeat("b", 64)
	if _, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{command(other)},
	}); !IsCode(err, "AUTHORITY_CONFLICT") {
		t.Fatalf("distinct bootstrap and journal authority = %v", err)
	}
	malformed := command(plan.Digest())
	malformed.ReplayKey = "plan-authority/wrong"
	if _, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{malformed},
	}); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("malformed journal authority = %v", err)
	}
}

func TestSavedPlanAdoptionRequiresIndependentExactAuthorityAndBatonApproval(t *testing.T) {
	repoPath := productionRepository(t)
	targetP := runRuntimeGit(t, repoPath, "rev-parse", "refs/heads/main")

	// Advance target with one additive commit D (descendant of P).
	if err := os.WriteFile(
		filepath.Join(repoPath, "forward.txt"),
		[]byte("forward target movement\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repoPath, "add", "--", "forward.txt")
	runRuntimeGit(
		t,
		repoPath,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit", "--quiet", "-m", "forward target movement",
	)
	targetD := runRuntimeGit(t, repoPath, "rev-parse", "refs/heads/main")

	// Create divergent commit X that does not contain P.
	tree := runRuntimeGit(t, repoPath, "rev-parse", targetP+"^{tree}")
	divergentX := runRuntimeGit(
		t,
		repoPath,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit-tree", tree, "-m", "divergent target history",
	)

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	repoView, err := gitx.Open(repoPath, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}

	manifestValue, _, plan := fixtureManifest(t)
	manifestValue.Repository = repoPath
	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}

	target := targetP
	planOID := strings.Repeat("2", 40)
	receipt := baton.Receipt{
		Version: baton.ReceiptVersion, Release: manifest.value.Release,
		Role: "planner", Result: "approved", Plan: planOID,
		Summary: "saved approval", Target: &target,
	}
	state := baton.State{
		Release:    manifest.value.Release,
		Repository: manifest.value.Authority.Project,
		Plan: baton.PlanState{
			OID: planOID, Digest: plan.Digest(), Metadata: plan.Metadata(),
			Approval: baton.ReceiptEntry{OID: strings.Repeat("3", 40), Receipt: receipt},
			History:  []baton.PlanHistory{{OID: planOID, Revision: 1, Plan: plan}},
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref:  "refs/heads/release-wt/" + manifest.value.Release,
				Head: strings.Repeat("4", 40),
			},
			Target: baton.CapturedRef{Ref: manifest.value.TargetRef, Head: targetD},
		},
	}
	eng := &engine{
		manifest:   manifest,
		repository: repoView,
		git:        baton.UseGitRepository(repoView),
	}

	// A1: Baton approval alone without authority digest fails.
	if adopted, err := validateSavedPlanAdoption(eng, state, ""); err != nil || adopted {
		t.Fatalf("Baton approval alone adopted = %t, %v", adopted, err)
	}

	// A1: Descendant target head (targetD) succeeds saved-plan adoption.
	if adopted, err := validateSavedPlanAdoption(eng, state, plan.Digest()); err != nil || !adopted {
		t.Fatalf("descendant target adoption = %t, %v", adopted, err)
	}

	// A1: Identical target head (targetP) succeeds saved-plan adoption.
	stateIdentical := state
	stateIdentical.Refs.Target.Head = targetP
	if adopted, err := validateSavedPlanAdoption(eng, stateIdentical, plan.Digest()); err != nil || !adopted {
		t.Fatalf("identical target adoption = %t, %v", adopted, err)
	}

	// A2: Refusals on divergence, stale lineage, or metadata substitution.
	for name, mutate := range map[string]func(*baton.State){
		"project":               func(value *baton.State) { value.Repository = "other" },
		"release":               func(value *baton.State) { value.Release = "other" },
		"divergent target head": func(value *baton.State) { value.Refs.Target.Head = divergentX },
		"approval target": func(value *baton.State) {
			other := divergentX
			value.Plan.Approval.Receipt.Target = &other
		},
		"digest":               func(value *baton.State) { value.Plan.Digest = "sha256:" + strings.Repeat("7", 64) },
		"bytes":                func(value *baton.State) { value.Plan.History = nil },
		"stale lineage":        func(value *baton.State) { value.Plan.TargetStale = true },
		"malformed target hex": func(value *baton.State) { value.Refs.Target.Head = "invalid-hex-oid" },
	} {
		t.Run(name, func(t *testing.T) {
			value := state
			value.Plan = state.Plan
			value.Plan.Approval = state.Plan.Approval.Clone()
			value.Plan.History = append([]baton.PlanHistory(nil), state.Plan.History...)
			mutate(&value)
			adopted, err := validateSavedPlanAdoption(eng, value, plan.Digest())
			if adopted || !IsCode(err, "INVALID_AUTHORITY") {
				t.Fatalf("substituted saved plan adopted=%t err=%v, want INVALID_AUTHORITY", adopted, err)
			}
		})
	}

	// Bounded correction 1: Nil engine / repository handle fails closed with INVALID_AUTHORITY.
	t.Run("nil repository", func(t *testing.T) {
		nilEngine := &engine{manifest: manifest}
		adopted, err := validateSavedPlanAdoption(nilEngine, state, plan.Digest())
		if adopted || !IsCode(err, "INVALID_AUTHORITY") {
			t.Fatalf("nil repository adoption adopted=%t err=%v, want INVALID_AUTHORITY", adopted, err)
		}
	})
}

func TestSavedPlanAdoptionAncestryProbeFailureSurfacesGitError(t *testing.T) {
	// A3: An ancestry probe that itself fails surfaces the git error code, never a silent INVALID_AUTHORITY.
	repoPath := productionRepository(t)
	targetP := runRuntimeGit(t, repoPath, "rev-parse", "refs/heads/main")

	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	repoView, err := gitx.Open(repoPath, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}

	manifestValue, _, plan := fixtureManifest(t)
	manifestValue.Repository = repoPath
	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}

	target := targetP
	planOID := strings.Repeat("2", 40)
	receipt := baton.Receipt{
		Version: baton.ReceiptVersion, Release: manifest.value.Release,
		Role: "planner", Result: "approved", Plan: planOID,
		Summary: "saved approval", Target: &target,
	}
	state := baton.State{
		Release:    manifest.value.Release,
		Repository: manifest.value.Authority.Project,
		Plan: baton.PlanState{
			OID: planOID, Digest: plan.Digest(), Metadata: plan.Metadata(),
			Approval: baton.ReceiptEntry{OID: strings.Repeat("3", 40), Receipt: receipt},
			History:  []baton.PlanHistory{{OID: planOID, Revision: 1, Plan: plan}},
		},
		Refs: baton.StateRefs{
			Release: baton.CapturedRef{
				Ref:  "refs/heads/release-wt/" + manifest.value.Release,
				Head: strings.Repeat("4", 40),
			},
			Target: baton.CapturedRef{Ref: manifest.value.TargetRef, Head: targetP},
		},
	}

	// Corrupt the git repository by removing its backing directory so git commands fail.
	if err := os.RemoveAll(repoPath); err != nil {
		t.Fatal(err)
	}

	engine := &engine{
		manifest:   manifest,
		repository: repoView,
		git:        baton.UseGitRepository(repoView),
	}

	adopted, adoptErr := validateSavedPlanAdoption(engine, state, plan.Digest())
	if adopted {
		t.Fatal("broken repository adopted saved plan")
	}
	if adoptErr == nil || IsCode(adoptErr, "INVALID_AUTHORITY") || !IsCode(adoptErr, "GIT_EXECUTION_FAILED") {
		t.Fatalf("broken repository error = %v, want GIT_EXECUTION_FAILED", adoptErr)
	}
}

func TestStatusReportsApprovedWhenTargetIsDescendantOfApprovedReceipt(t *testing.T) {
	// A4: sworn status over a journal whose approved target is an ancestor of the advanced head reports approved.
	ctx := context.Background()
	repoPath := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	manifestValue, _, plan := fixtureManifest(t)
	manifestValue.Repository = repoPath
	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}

	repoView, err := gitx.Open(repoPath, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}

	// Install plan revision 1 in baton.
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	actions, err := baton.NewActions(baton.UseGitRepository(repoView), inertness, manifest.value.GitIdentity)
	if err != nil {
		t.Fatal(err)
	}
	installer := newAuthorityInstaller(actions)
	targetP := runRuntimeGit(t, repoPath, "rev-parse", "refs/heads/main")
	admission := approvalAdmission{
		planBytes:  plan.Bytes(),
		planDigest: plan.Digest(),
		reference:  plan.Metadata().ApprovalRef,
	}
	if _, err := installer.install(admission, targetP); err != nil {
		t.Fatal(err)
	}

	// Register run in journal.
	path := filepath.Join(t.TempDir(), "status-adoption.sqlite")
	store, err := journal.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 19, 1, 0, 0, 0, time.UTC)
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: manifest.value.RunID, ReplayKey: "manifest",
		Kind: "start", Payload: manifest.raw, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	service := &Service{
		journal:       store,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	// Advance target on main with one additive commit D (descendant of P).
	if err := os.WriteFile(
		filepath.Join(repoPath, "additive.txt"),
		[]byte("additive commit past receipt target\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repoPath, "add", "--", "additive.txt")
	runRuntimeGit(
		t,
		repoPath,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit", "--quiet", "-m", "advance target additively",
	)
	targetD := runRuntimeGit(t, repoPath, "rev-parse", "refs/heads/main")

	status, err := service.Status(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatalf("status error = %v", err)
	}
	if status.AuthorityState != "approved" {
		t.Fatalf("status.AuthorityState = %q, want approved", status.AuthorityState)
	}
	if status.TargetHead != targetD {
		t.Fatalf("status.TargetHead = %q, want %q", status.TargetHead, targetD)
	}

	// Divergent target movement reports invalid_authority (A2).
	tree := runRuntimeGit(t, repoPath, "rev-parse", targetP+"^{tree}")
	divergentX := runRuntimeGit(
		t,
		repoPath,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit-tree", tree, "-m", "divergent target history",
	)
	runRuntimeGit(t, repoPath, "update-ref", "refs/heads/main", divergentX)

	divergentStatus, err := service.Status(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatalf("divergent status error = %v", err)
	}
	if divergentStatus.AuthorityState != "invalid_authority" {
		t.Fatalf("divergent status.AuthorityState = %q, want invalid_authority", divergentStatus.AuthorityState)
	}
}

func TestSavedPlanAdoptionDrivesRunWithDescendantTarget(t *testing.T) {
	// A1: With an approved plan receipt bound to commit P and the target head advanced to a descendant of P,
	// saved-plan adoption succeeds and the run drives.
	ctx := context.Background()
	repository := productionRepository(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}

	manifest, _, plan := fixtureManifest(t)
	manifest.Repository = repository
	metadata := plan.Metadata()
	metadata.Tracks = metadata.Tracks[:1]
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	planBytes := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err = baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	digest := plan.Digest()
	manifest.Authority.BootstrapApprovedPlanDigest = &digest
	manifest.Scripts = []ScriptedAttempt{
		{Responsibility: driver.AssemblyVerification, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.CaptainReview, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerDesign, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.ImplementerImplementation, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Responsibility: driver.PlannerProposal, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
		{Slice: "S1", Responsibility: driver.WorkVerification, BatonAttempt: 1, Epoch: 1, Try: 1, Behavior: "submit"},
	}
	submission := func(
		slice string,
		responsibility driver.Responsibility,
		batonAttempt int64,
	) driver.Submission {
		script := ScriptedAttempt{Slice: slice, Responsibility: responsibility,
			BatonAttempt: batonAttempt, Epoch: 1, Try: 1}
		return driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocationID(manifest.RunID, script),
			Responsibility: responsibility,
			Summary:        "Exact " + string(responsibility) + ".",
			Detail:         "Bounded fixture detail.",
		}
	}
	planner := submission("", driver.PlannerProposal, 1)
	planner.Plan, _ = driver.NewPlanBytes(planBytes)
	design := submission("S1", driver.ImplementerDesign, 1)
	captain := submission("S1", driver.CaptainReview, 1)
	captain.Decision, _ = driver.NewDecision(driver.DecisionProceed)
	implementation := submission("S1", driver.ImplementerImplementation, 1)
	implementation.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
	work := submission("S1", driver.WorkVerification, 1)
	work.Checks, _ = driver.NewCheckBytes([]byte("work checks\n"))
	work.Decision, _ = driver.NewDecision(driver.DecisionPass)
	assembly := submission("", driver.AssemblyVerification, 1)
	assembly.Checks, _ = driver.NewCheckBytes([]byte("assembly checks\n"))
	assembly.Decision, _ = driver.NewDecision(driver.DecisionPass)
	manifest.Scripts[0].Submission = encodeSubmission(t, assembly)
	manifest.Scripts[1].Submission = encodeSubmission(t, captain)
	manifest.Scripts[2].Submission = encodeSubmission(t, design)
	manifest.Scripts[3].Submission = encodeSubmission(t, implementation)
	manifest.Scripts[4].Submission = encodeSubmission(t, planner)
	manifest.Scripts[5].Submission = encodeSubmission(t, work)
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	repoView, err := gitx.Open(repository, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}

	// Pre-install the approved plan at commit P.
	targetP := runRuntimeGit(t, repository, "rev-parse", "refs/heads/main")
	inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{Kind: request.Kind, Repository: request.Repository,
			RecordRoot: request.RecordRoot, Commit: request.Commit, Decision: "inert"}, nil
	}
	actions, err := baton.NewActions(baton.UseGitRepository(repoView), inertness, manifest.GitIdentity)
	if err != nil {
		t.Fatal(err)
	}
	installer := newAuthorityInstaller(actions)
	admission := approvalAdmission{
		planBytes:  plan.Bytes(),
		planDigest: plan.Digest(),
		reference:  plan.Metadata().ApprovalRef,
	}
	if _, err := installer.install(admission, targetP); err != nil {
		t.Fatal(err)
	}

	// Advance target one additive commit past the receipt target P.
	if err := os.WriteFile(
		filepath.Join(repository, "forward.txt"),
		[]byte("forward movement\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runRuntimeGit(t, repository, "add", "--", "forward.txt")
	runRuntimeGit(
		t,
		repository,
		"-c", "user.name=Production Fixture",
		"-c", "user.email=production@example.invalid",
		"commit", "--quiet", "-m", "advance target additively",
	)

	// Set up driver submissions for the run.
	submissions := make(map[string][]byte, len(manifest.Scripts))
	for _, script := range manifest.Scripts {
		encoded, decodeErr := base64.StdEncoding.DecodeString(script.Submission)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		submissions[invocationID(manifest.RunID, script)] = encoded
	}
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		if invocation.Request.Role == driver.RoleImplementer &&
			invocation.Request.Workspace.Access == driver.ReadWrite {
			if err := os.WriteFile(
				filepath.Join(invocation.HostWorkspace, "one.txt"),
				[]byte("implemented one\n"),
				0o600,
			); err != nil {
				return driver.Observation{}, err
			}
		}
		submission := submissions[invocation.Request.InvocationID]
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage: driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
			Diagnostic: driver.Diagnostic{Code: "none"},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  submission,
				SubmissionDigest: driver.Digest(submission),
			},
		}, nil
	})

	path := filepath.Join(t.TempDir(), "drive-descendant.sqlite")
	store, err := journal.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 19, 2, 0, 0, 0, time.UTC)
	service := &Service{
		journal:       store,
		dispatcher:    dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return now },
	}

	status, err := service.Start(ctx, body)
	if err != nil {
		t.Fatalf("start error = %v", err)
	}
	if status.State != "complete" {
		t.Fatalf("status = %#v, want complete", status)
	}
	if status.AuthorityState != "approved" {
		t.Fatalf("status.AuthorityState = %q, want approved", status.AuthorityState)
	}
}

func TestEveryInstallEffectStateOutranksFreshAdoption(t *testing.T) {
	for _, state := range []journal.EffectState{
		journal.Pending, journal.Claimed, journal.Uncertain,
	} {
		validate, err := installEffectPrecedence(state)
		if validate || !IsCode(err, "INSTALL_RECOVERY_PENDING") {
			t.Fatalf("%s precedence = validate:%t err:%v", state, validate, err)
		}
	}
	validate, err := installEffectPrecedence(journal.OperationalFailed)
	if validate || !IsCode(err, "INSTALL_FAILED") {
		t.Fatalf("failed install precedence = validate:%t err:%v", validate, err)
	}
	validate, err = installEffectPrecedence(journal.Succeeded)
	if !validate || err != nil {
		t.Fatalf("succeeded install precedence = validate:%t err:%v", validate, err)
	}
}

func TestProposalActivationRequiresExactAppliedPlanAuthority(t *testing.T) {
	_, _, oldPlan := fixtureManifest(t)
	metadata := oldPlan.Metadata()
	previous := strings.Repeat("1", 40)
	metadata.Revision = 2
	metadata.PreviousPlan = &previous
	metadata.ApprovalRef = "operator://release-1/2"
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	newPlan, err := baton.ParsePlan([]byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nNewer fixture revision.\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	target := strings.Repeat("2", 40)
	proposal := admittedPlanProposal{
		plan: newPlan,
		authority: planProposalAuthority{
			TargetHead: target,
		},
	}
	state := baton.State{Plan: baton.PlanState{
		Digest: newPlan.Digest(), Metadata: newPlan.Metadata(),
		Approval: baton.ReceiptEntry{Receipt: baton.Receipt{Target: &target}},
	}}
	executed := journal.Snapshot{Effects: []journal.Effect{{
		Kind: "git.prepare_track_base", State: journal.Succeeded,
	}}}

	for _, test := range []struct {
		name      string
		found     bool
		installed bool
		authority string
		snapshot  journal.Snapshot
		state     baton.State
		want      bool
	}{
		{
			name:  "old authority and old execution cannot activate newer applied plan",
			found: true, authority: oldPlan.Digest(), snapshot: executed,
			state: state,
		},
		{
			name:  "exact same-plan restart resumes from durable execution",
			found: true, authority: newPlan.Digest(), snapshot: executed,
			state: state, want: true,
		},
		{
			name:  "exact installed proposal resumes after takeover",
			found: true, installed: true, authority: newPlan.Digest(),
			state: state, want: true,
		},
		{
			name:  "installed proposal cannot borrow old authority",
			found: true, installed: true, authority: oldPlan.Digest(),
			state: state,
		},
		{
			name:  "exact authority without durable activation remains waiting",
			found: true, authority: newPlan.Digest(), state: state,
		},
		{
			name:      "unselected proposal cannot activate",
			authority: newPlan.Digest(), snapshot: executed, state: state,
		},
		{
			name:  "different applied plan cannot activate",
			found: true, authority: newPlan.Digest(), snapshot: executed,
			state: baton.State{Plan: baton.PlanState{
				Digest: oldPlan.Digest(), Metadata: oldPlan.Metadata(),
				Approval: baton.ReceiptEntry{Receipt: baton.Receipt{Target: &target}},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := proposalActivationRecorded(
				proposal, test.found, test.installed, test.state, nil,
				test.authority, test.snapshot,
			); got != test.want {
				t.Fatalf("activation = %t, want %t", got, test.want)
			}
		})
	}
	if !proposalAwaitsExactAuthority(
		proposal, true, state, nil, oldPlan.Digest(),
	) {
		t.Fatal("newer applied proposal did not remain awaiting exact authority")
	}
	if proposalAwaitsExactAuthority(
		proposal, true, state, nil, newPlan.Digest(),
	) {
		t.Fatal("exact same-plan authority remained waiting")
	}
}

func TestLegacyV2V3V4AreReadOnlyBeforeEveryMutation(t *testing.T) {
	for _, version := range []string{ManifestVersionV2, ManifestVersionV3, ManifestVersionV4} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			legacy := []byte(`{"schema_version":"` + version + `","run_id":"legacy"}` + "\n")
			if got, err := ClassifyManifestVersion(legacy); err != nil || got != version {
				t.Fatalf("classification = %q, %v", got, err)
			}
			path := filepath.Join(t.TempDir(), "legacy.sqlite")
			store, err := journal.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
			service := &Service{journal: store, now: func() time.Time { return now }}
			if _, err := service.Start(ctx, legacy); !IsCode(err, "MIGRATION_REQUIRED") {
				t.Fatalf("legacy start = %v", err)
			}
			bindings, err := store.RunBindings(ctx)
			if err != nil || len(bindings) != 0 {
				t.Fatalf("legacy start registered a run: %#v, %v", bindings, err)
			}
			run := journal.Run{ID: "legacy", ManifestDigest: sha256Digest(legacy),
				Repository: "/legacy", Release: "legacy-release",
				TargetRef: "refs/heads/main", CreatedAt: now}
			if err := store.RegisterRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			if err := store.RecordCommand(ctx, journal.Command{RunID: run.ID,
				ReplayKey: "manifest", Kind: "start", Payload: legacy, CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			status, err := service.Status(ctx, run.ID)
			if err != nil || status.State != "migration_required" {
				t.Fatalf("legacy status = %#v, %v", status, err)
			}
			before, _ := store.ControlProjection(ctx, run.ID)
			for _, kind := range []journal.ControlKind{
				journal.Pause, journal.Resume, journal.Cancel,
				journal.Retry, journal.Takeover,
			} {
				if _, err := service.Control(ctx, journal.ControlCommand{
					RunID: run.ID, ID: "legacy-" + string(kind), Kind: kind,
					ExpectedGeneration: before.Generation,
					WorkID:             "sha256:" + strings.Repeat("a", 64), ExpectedEpoch: 1,
				}); !IsCode(err, "MIGRATION_REQUIRED") {
					t.Fatalf("legacy %s control = %v", kind, err)
				}
			}
			after, _ := store.ControlProjection(ctx, run.ID)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("legacy control wrote projection: before=%#v after=%#v", before, after)
			}
			if _, err := service.AnswerAttention(ctx, AnswerAttentionCommand{
				RunID: run.ID, AttentionID: "anything", ExpectedGeneration: 1,
				Answer: "continue",
			}); !IsCode(err, "MIGRATION_REQUIRED") {
				t.Fatalf("legacy attention mutation = %v", err)
			}
			_ = store.Close()
		})
	}
}
