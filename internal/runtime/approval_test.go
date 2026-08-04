package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func approvalFixture(t *testing.T) (admittedManifest, admittedPlanProposal, ApprovalCommand) {
	t.Helper()
	manifestValue, body, plan := fixtureManifest(t)
	manifestValue.Authority.BootstrapApprovedPlanDigest = nil
	body, err := canonicalManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	proposal := admittedPlanProposal{
		plan:      plan,
		replayKey: "plan-proposal/00000000000000000001/" + strings.Repeat("1", 64),
		authority: planProposalAuthority{
			Release:    manifest.value.Release,
			ReleaseRef: "refs/heads/release-wt/" + manifest.value.Release,
			TargetRef:  manifest.value.TargetRef,
			TargetHead: strings.Repeat("2", 40),
		},
	}
	command, err := approvalCommandForProposal(manifest, proposal)
	if err != nil {
		t.Fatal(err)
	}
	return manifest, proposal, command
}

func TestReplanDecisionClassComesOnlyFromExactLineage(t *testing.T) {
	manifest, initial, _ := approvalFixture(t)
	prior := strings.Repeat("9", 40)
	metadata := initial.plan.Metadata()
	metadata.Revision = 2
	metadata.PreviousPlan = &prior
	metadata.ApprovalRef = "operator://" + manifest.value.Release + "/2"
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := baton.ParsePlan([]byte(
		"```baton-plan-v2\n" + string(body) + "\n```\n\nReplan.\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	proposal := initial
	proposal.plan = plan
	proposal.authority.PriorPlan = prior
	proposal.authority.ReleaseHead = strings.Repeat("8", 40)
	command, err := approvalCommandForProposal(manifest, proposal)
	if err != nil || command.DecisionClass != PlannerReplanClass {
		t.Fatalf("derived replan class = %q, %v", command.DecisionClass, err)
	}
	command.DecisionClass = PlannerProposalClass
	if err := validateApprovalAuthority(manifest, proposal, command); err == nil {
		t.Fatal("caller-selected proposal class overrode replan lineage")
	}
	proposal.authority.PriorPlan = strings.Repeat("7", 40)
	if _, err := approvalDecisionClass(proposal); !IsCode(err, "APPROVAL_AMBIGUOUS") {
		t.Fatalf("broken lineage = %v", err)
	}
}

func TestApprovalCommandIsCanonicalAndEveryBindingFailsClosed(t *testing.T) {
	manifest, proposal, command := approvalFixture(t)
	canonical, err := CanonicalApprovalCommand(command)
	if err != nil || string(append(canonical, '\n')) != string(mustJSON(command)) {
		t.Fatalf("canonical command = %q, %v", canonical, err)
	}
	mutations := map[string]func(*ApprovalCommand){
		"schema":          func(v *ApprovalCommand) { v.SchemaVersion = "wrong" },
		"run":             func(v *ApprovalCommand) { v.RunID = "other" },
		"manifest":        func(v *ApprovalCommand) { v.ManifestDigest = "sha256:" + strings.Repeat("3", 64) },
		"project":         func(v *ApprovalCommand) { v.Project = "other" },
		"release":         func(v *ApprovalCommand) { v.Release = "other" },
		"release_ref":     func(v *ApprovalCommand) { v.ReleaseRef += "-other" },
		"release_head":    func(v *ApprovalCommand) { v.ReleaseHead = strings.Repeat("4", 40) },
		"proposal":        func(v *ApprovalCommand) { v.ProposalReplayKey += "-other" },
		"revision":        func(v *ApprovalCommand) { v.PlanRevision++ },
		"prior":           func(v *ApprovalCommand) { v.PriorPlan = strings.Repeat("5", 40) },
		"digest":          func(v *ApprovalCommand) { v.PlanDigest = "sha256:" + strings.Repeat("6", 64) },
		"target_ref":      func(v *ApprovalCommand) { v.TargetRef = "refs/heads/other" },
		"target_head":     func(v *ApprovalCommand) { v.TargetHead = strings.Repeat("7", 40) },
		"class":           func(v *ApprovalCommand) { v.DecisionClass = PlannerReplanClass },
		"decision":        func(v *ApprovalCommand) { v.Decision = "reject" },
		"actor_class":     func(v *ApprovalCommand) { v.ActorClass = "captain" },
		"actor_authority": func(v *ApprovalCommand) { v.ActorAuthority = "other" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := command
			mutate(&changed)
			if reflect.DeepEqual(changed, command) {
				t.Fatal("mutation did not change command")
			}
			if err := validateApprovalAuthority(manifest, proposal, changed); err == nil {
				t.Fatal("mutated binding was admitted")
			}
		})
	}
}

func TestSucceededExactAdmissionIsAuthorityAndHistoricalAdmissionIsInert(t *testing.T) {
	manifest, proposal, command := approvalFixture(t)
	replayKey, effectID, payload, err := approvalIdentity(command)
	if err != nil {
		t.Fatal(err)
	}
	_, resultBody, err := canonicalApprovalResult(command)
	if err != nil {
		t.Fatal(err)
	}
	exactCommand := journal.Command{
		RunID: command.RunID, ReplayKey: replayKey, Kind: "approval", Payload: payload,
	}
	exactEffect := journal.Effect{
		RunID: command.RunID, ID: effectID, ReplayKey: replayKey,
		Kind: approvalEffectKind, State: journal.Succeeded,
		ExpectedDigest: sha256Digest(resultBody), Result: resultBody,
	}
	if got, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{exactCommand}, Effects: []journal.Effect{exactEffect},
	}, &proposal); err != nil || got != command.PlanDigest {
		t.Fatalf("exact admission authority = %q, %v", got, err)
	}

	historicalProposal := proposal
	historicalProposal.authority.TargetHead = strings.Repeat("8", 40)
	historical, err := approvalCommandForProposal(manifest, historicalProposal)
	if err != nil {
		t.Fatal(err)
	}
	historicalKey, historicalID, historicalPayload, _ := approvalIdentity(historical)
	_, historicalResult, _ := canonicalApprovalResult(historical)
	if got, err := effectivePlanAuthority(manifest, journal.Snapshot{
		Commands: []journal.Command{{
			RunID: historical.RunID, ReplayKey: historicalKey,
			Kind: "approval", Payload: historicalPayload,
		}},
		Effects: []journal.Effect{{
			RunID: historical.RunID, ID: historicalID, ReplayKey: historicalKey,
			Kind: approvalEffectKind, State: journal.Succeeded,
			ExpectedDigest: sha256Digest(historicalResult), Result: historicalResult,
		}},
	}, &proposal); err != nil || got != "" {
		t.Fatalf("historical admission became authority = %q, %v", got, err)
	}
}

func TestApprovalResultIsStableAcrossExactReplay(t *testing.T) {
	_, _, command := approvalFixture(t)
	first, firstBody, err := canonicalApprovalResult(command)
	if err != nil {
		t.Fatal(err)
	}
	second, secondBody, err := canonicalApprovalResult(command)
	if err != nil || !reflect.DeepEqual(first, second) || string(firstBody) != string(secondBody) {
		t.Fatalf("replayed result drifted: %#v %#v %v", first, second, err)
	}
}

type approvalRecoveryFixture struct {
	path       string
	runID      string
	now        time.Time
	command    ApprovalCommand
	dispatcher fixtureDriver
}

func newApprovalRecoveryFixture(t *testing.T) approvalRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	repository := productionRepository(t)
	manifest, _, plan := fixtureManifest(t)
	manifest.Repository = repository
	manifest.Authority.BootstrapApprovedPlanDigest = nil

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
	for index := range manifest.Scripts {
		script := &manifest.Scripts[index]
		if script.Responsibility != driver.PlannerProposal {
			continue
		}
		encoded, err := base64.StdEncoding.DecodeString(script.Submission)
		if err != nil {
			t.Fatal(err)
		}
		submission, err := driver.DecodeSubmission(encoded)
		if err != nil {
			t.Fatal(err)
		}
		submission.Plan, err = driver.NewPlanBytes(plan.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		script.Submission = encodeSubmission(t, submission)
	}
	body, err := canonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	submissions := make(map[string][]byte, len(manifest.Scripts))
	for _, script := range manifest.Scripts {
		encoded, err := base64.StdEncoding.DecodeString(script.Submission)
		if err != nil {
			t.Fatal(err)
		}
		submissions[invocationID(manifest.RunID, script)] = encoded
	}
	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
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
	path := filepath.Join(t.TempDir(), "approval-recovery.sqlite")
	now := time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC)
	store, err := journal.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		journal: store, dispatcher: dispatcher,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	status, err := service.Start(ctx, body)
	if err != nil || status.State != "awaiting_approval" {
		t.Fatalf("start = %#v, %v", status, err)
	}
	offer, err := service.ApprovalOffer(ctx, manifest.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return approvalRecoveryFixture{
		path: path, runID: manifest.RunID, now: now, command: offer.Command,
		dispatcher: dispatcher,
	}
}

func (fixture approvalRecoveryFixture) openService(
	t *testing.T,
) (*Service, *journal.Store) {
	t.Helper()
	store, err := journal.Open(context.Background(), fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	return &Service{
		journal: store, dispatcher: fixture.dispatcher,
		gitExecutable: gitExecutable,
		now:           func() time.Time { return fixture.now },
	}, store
}

func (fixture approvalRecoveryFixture) seedAdmission(
	t *testing.T,
	state journal.EffectState,
) {
	t.Helper()
	ctx := context.Background()
	service, store := fixture.openService(t)
	replayKey, effectID, payload, err := approvalIdentity(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	_, resultBody, err := canonicalApprovalResult(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommandEffect(ctx, journal.Command{
		RunID: fixture.runID, ReplayKey: replayKey, Kind: "approval",
		Payload: payload, CreatedAt: fixture.now,
	}, journal.Effect{
		RunID: fixture.runID, ID: effectID, ReplayKey: replayKey,
		Kind: approvalEffectKind, BeforeDigest: sha256Digest(payload),
		ExpectedDigest: sha256Digest(resultBody), UpdatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}
	if state == journal.Pending {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	effect, err := store.Effect(ctx, fixture.runID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := store.Claim(
		ctx, fixture.runID, effectID, fixture.now, effectLease,
	)
	if err != nil {
		t.Fatal(err)
	}
	if state == journal.Succeeded {
		effect.State, effect.CurrentClaim = journal.Claimed, claim.Token
		if _, err := service.completeApprovalAdmission(
			ctx, fixture.command, effect,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertApprovalRecoveryCounts(
	t *testing.T,
	store *journal.Store,
	runID string,
) {
	t.Helper()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	events := make(map[string]int)
	for _, effect := range snapshot.Effects {
		counts[effect.Kind]++
		if effect.Kind == approvalEffectKind && effect.State != journal.Succeeded {
			t.Fatalf("approval effect state = %q", effect.State)
		}
	}
	for _, event := range snapshot.Events {
		events[event.Kind]++
	}
	if counts[approvalEffectKind] != 1 || counts["baton.install"] != 1 ||
		events["owner_acquired"] != 2 {
		t.Fatalf("recovery counts: effects=%#v events=%#v", counts, events)
	}
}

func TestReconcileApprovalsPendingAdmissionWakesOnce(t *testing.T) {
	fixture := newApprovalRecoveryFixture(t)
	fixture.seedAdmission(t, journal.Pending)
	service, store := fixture.openService(t)
	if err := service.ReconcileApprovals(
		context.Background(), fixture.runID,
	); err != nil {
		t.Fatal(err)
	}
	assertApprovalRecoveryCounts(t, store, fixture.runID)
}

func TestReconcileApprovalsClaimedAdmissionCompletesExactClaimAndWakesOnce(
	t *testing.T,
) {
	fixture := newApprovalRecoveryFixture(t)
	fixture.seedAdmission(t, journal.Claimed)
	service, store := fixture.openService(t)
	if err := service.ReconcileApprovals(
		context.Background(), fixture.runID,
	); err != nil {
		t.Fatal(err)
	}
	assertApprovalRecoveryCounts(t, store, fixture.runID)
}

func TestReconcileApprovalsSucceededBeforeInstallWakesOnce(t *testing.T) {
	fixture := newApprovalRecoveryFixture(t)
	fixture.seedAdmission(t, journal.Succeeded)
	service, store := fixture.openService(t)
	if err := service.ReconcileApprovals(
		context.Background(), fixture.runID,
	); err != nil {
		t.Fatal(err)
	}
	assertApprovalRecoveryCounts(t, store, fixture.runID)
}

func TestReconcileApprovalsRepeatedReconciliationIsIdempotent(t *testing.T) {
	fixture := newApprovalRecoveryFixture(t)
	fixture.seedAdmission(t, journal.Succeeded)
	service, store := fixture.openService(t)
	for range 2 {
		if err := service.ReconcileApprovals(
			context.Background(), fixture.runID,
		); err != nil {
			t.Fatal(err)
		}
	}
	assertApprovalRecoveryCounts(t, store, fixture.runID)
}
