package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func runtimePlanSingleSlice(t *testing.T, release, repository, target, marker string) ([]byte, baton.Plan) {
	t.Helper()
	slice := baton.Slice{
		ID: "S1", Outcome: "Deliver S1.",
		Scope:      baton.Scope{Include: []string{"one.txt"}, Exclude: []string{}},
		Acceptance: []baton.Criterion{{ID: "A-S1", Text: "S1 is exact."}},
		Checks:     []string{"check S1"}, Constraints: []string{"deterministic"},
		DependsOn: []string{}, Consumes: []string{},
	}
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    repository,
		TargetRef:     target,
		ApprovalRef:   "operator://" + release + "/1",
		Tracks: []baton.Track{
			{ID: "T1", DependsOn: []string{}, Slices: []baton.Slice{slice}},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nFixture plan.\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func newFixtureSingleSlice(t *testing.T, bootstrapAuthority bool) *degradationStatusFixture {
	t.Helper()
	ctx := context.Background()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	production, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 5, 6, 7, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	planBytes, plan := runtimePlanSingleSlice(t, manifest.value.Release, manifest.value.Authority.Project, manifest.value.TargetRef, "approval-release-1-v1")
	if bootstrapAuthority {
		planDigest := plan.Digest()
		manifest.value.Authority.BootstrapApprovedPlanDigest = &planDigest
	} else {
		manifest.value.Authority.BootstrapApprovedPlanDigest = nil
	}
	body, err := canonicalManifest(manifest.value)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(ctx, journal.Command{
		RunID: manifest.value.RunID, ReplayKey: "manifest", Kind: "start", Payload: manifest.raw, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, manifest.value.RunID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
		}, nil
	})
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	if _, err := engine.actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install exact plan",
		Detail:    []byte("detail"),
	}); err != nil {
		t.Fatal(err)
	}
	return &degradationStatusFixture{
		ctx: ctx, manifest: manifest, store: store, owner: owner,
		now: now, service: service, engine: engine,
	}
}

func newBootstrapFixtureSingleSlice(t *testing.T) *degradationStatusFixture {
	return newFixtureSingleSlice(t, true)
}

// escalateSlice advances a slice in design stage to captain/escalate.
func escalateSlice(t *testing.T, fixture *degradationStatusFixture, sliceID, summary string) {
	t.Helper()
	release := fixture.manifest.value.Release
	_, err := fixture.engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: release,
		Slice:   sliceID,
		Role:    "implementer",
		Result:  "designed",
		Summary: "Design " + sliceID + ".",
		Detail:  []byte("design detail"),
	})
	if err != nil {
		t.Fatalf("AppendReceipt designed failed: %v", err)
	}
	_, err = fixture.engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: release,
		Slice:   sliceID,
		Role:    "captain",
		Result:  "escalate",
		Summary: summary,
		Detail:  []byte("escalate detail"),
	})
	if err != nil {
		t.Fatalf("AppendReceipt escalate failed: %v", err)
	}
}

// passSlice advances a slice through design, review, candidate, and pass verification.
func passSlice(t *testing.T, fixture *degradationStatusFixture, trackID, sliceID string) {
	t.Helper()
	release := fixture.manifest.value.Release
	_, err := fixture.engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: release,
		Slice:   sliceID,
		Role:    "implementer",
		Result:  "designed",
		Summary: "Design " + sliceID + ".",
		Detail:  []byte("design detail"),
	})
	if err != nil {
		t.Fatalf("AppendReceipt designed failed: %v", err)
	}
	_, err = fixture.engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: release,
		Slice:   sliceID,
		Role:    "captain",
		Result:  "proceed",
		Summary: "Proceed " + sliceID + ".",
		Detail:  []byte("proceed detail"),
	})
	if err != nil {
		t.Fatalf("AppendReceipt proceed failed: %v", err)
	}
	ref := "refs/heads/track/" + release + "/" + trackID
	cmd := exec.Command(fixture.service.gitExecutable, "-C", fixture.manifest.value.Repository, "rev-parse", "--verify", ref)
	parentBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse track ref failed: %v", err)
	}
	parent := strings.TrimSpace(string(parentBytes))

	parentTreeCmd := exec.Command(fixture.service.gitExecutable, "-C", fixture.manifest.value.Repository, "rev-parse", parent+"^{tree}")
	treeBytes, err := parentTreeCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse parent tree failed: %v", err)
	}
	tree := strings.TrimSpace(string(treeBytes))

	// Commit candidate product
	commitCmd := exec.Command(fixture.service.gitExecutable, "-C", fixture.manifest.value.Repository, "commit-tree", tree, "-p", parent, "-m", "candidate "+sliceID)
	commitCmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	candidateBytes, err := commitCmd.Output()
	if err != nil {
		t.Fatalf("commit-tree failed: %v", err)
	}
	candidate := strings.TrimSpace(string(candidateBytes))
	updateRefCmd := exec.Command(fixture.service.gitExecutable, "-C", fixture.manifest.value.Repository, "update-ref", ref, candidate, parent)
	if err := updateRefCmd.Run(); err != nil {
		t.Fatalf("update-ref failed: %v", err)
	}

	_, err = fixture.engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release:      release,
		Slice:        sliceID,
		Role:         "implementer",
		Result:       "candidate",
		Summary:      "Candidate " + sliceID + ".",
		Detail:       []byte("candidate detail"),
		Candidate:    candidate,
		CheckResults: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("AppendReceipt candidate failed: %v", err)
	}

	_, err = fixture.engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release:      release,
		Slice:        sliceID,
		Role:         "verifier",
		Result:       "pass",
		Summary:      "Pass " + sliceID + ".",
		Detail:       []byte("pass detail"),
		Candidate:    candidate,
		CheckResults: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("AppendReceipt pass failed: %v", err)
	}
}

// passAllSlicesAndBlockAssembly advances S1 and S2 to pass, prepares assembly, and blocks assembly with verifier.
func passAllSlicesAndBlockAssembly(t *testing.T, fixture *degradationStatusFixture, summary string) {
	t.Helper()
	passSlice(t, fixture, "T1", "S1")
	passSlice(t, fixture, "T2", "S2")

	state, err := baton.ReadState(fixture.engine.git, fixture.manifest.value.Release, fixture.engine.inertness)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if err := fixture.service.prepareAssembly(fixture.ctx, fixture.engine, fixture.owner, state); err != nil {
		t.Fatalf("prepareAssembly failed: %v", err)
	}

	state, err = baton.ReadState(fixture.engine.git, fixture.manifest.value.Release, fixture.engine.inertness)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}
	if state.Assembly.NextRole != "verifier" {
		t.Fatalf("Assembly.NextRole = %q, want verifier", state.Assembly.NextRole)
	}

	_, err = fixture.engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release:      fixture.manifest.value.Release,
		Role:         "verifier",
		Result:       "blocked",
		Summary:      summary,
		Detail:       []byte("assembly blocked detail"),
		Candidate:    *state.Assembly.Candidate.Receipt.Candidate,
		CheckResults: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("AppendReceipt assembly blocked failed: %v", err)
	}
}

// A1(1): Captain/escalate slice under pure bootstrap authority stops driveLoop with zero RolePlanner invocations.
func TestDriveLoopStopsPlannerDispatchUnderBootstrapAuthority_CaptainEscalate(t *testing.T) {
	t.Parallel()
	fixture := newBootstrapFixtureSingleSlice(t)

	var plannerCalls int64
	fixture.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		if invocation.Request.Role == driver.RolePlanner {
			atomic.AddInt64(&plannerCalls, 1)
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
		}, nil
	})

	escalateSlice(t, fixture, "S1", "Captain escalated S1 due to scope conflict.")

	if err := fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false); err != nil {
		t.Fatalf("driveLoop returned error: %v", err)
	}

	if calls := atomic.LoadInt64(&plannerCalls); calls != 0 {
		t.Fatalf("driver.RolePlanner invocations = %d, want 0", calls)
	}

	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasParkEventForCause(snapshot, ParkCauseBootstrapAuthority, "") {
		t.Fatal("expected ParkCauseBootstrapAuthority event in journal")
	}
}

// A1(2): All slices passed and assembly in verifier/blocked records park reason deriving from state.Assembly.CurrentReceipt with zero RolePlanner invocations.
func TestDriveLoopStopsPlannerDispatchUnderBootstrapAuthority_AssemblyVerifierBlocked(t *testing.T) {
	t.Parallel()
	fixture := newDegradationStatusFixture(t)

	var plannerCalls int64
	fixture.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		if invocation.Request.Role == driver.RolePlanner {
			atomic.AddInt64(&plannerCalls, 1)
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
		}, nil
	})

	assemblySummary := "Assembly verification failed: integration check failed."
	passAllSlicesAndBlockAssembly(t, fixture, assemblySummary)

	if err := fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false); err != nil {
		t.Fatalf("driveLoop error: %v", err)
	}

	if calls := atomic.LoadInt64(&plannerCalls); calls != 0 {
		t.Fatalf("driver.RolePlanner invocations = %d, want 0", calls)
	}

	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}

	var parkEvent *journal.Event
	for i := range snapshot.Events {
		if snapshot.Events[i].Kind == ParkEventKind {
			parkEvent = &snapshot.Events[i]
			break
		}
	}
	if parkEvent == nil {
		t.Fatal("expected park event in journal")
	}

	parsed, err := ParseDegradationParkEvent(parkEvent.Body)
	if err != nil {
		t.Fatalf("ParseDegradationParkEvent failed: %v", err)
	}
	if parsed.Cause != ParkCauseBootstrapAuthority {
		t.Fatalf("cause = %q, want %q", parsed.Cause, ParkCauseBootstrapAuthority)
	}
	wantReason := assemblySummary + "\n\n" + bootstrapParkUnblockDirective
	if parsed.Reason != wantReason {
		t.Fatalf("reason = %q, want %q", parsed.Reason, wantReason)
	}
}

// A1(3): Ready slice whose advanceSlice returns EFFECT_PARKED concurrently with captain/escalate slice
// asserts driveLoop reaches :5690-5695 branch and records zero driver.RolePlanner invocations.
func TestDriveLoopStopsPlannerDispatchUnderBootstrapAuthority_ConcurrentReadyWorkEffectParked(t *testing.T) {
	t.Parallel()
	fixture := newDegradationStatusFixture(t)

	var plannerCalls int64
	// S1 is escalated, S2 is ready. When advanceSlice is called for S2, yield question so advanceSlice returns EFFECT_PARKED.
	fixture.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		if invocation.Request.Role == driver.RolePlanner {
			atomic.AddInt64(&plannerCalls, 1)
		}
		if strings.Contains(invocation.Request.InvocationID, "/S2/") {
			return driver.Observation{
				TransportStatus: driver.Completed,
				Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
				Diagnostic:      driver.Diagnostic{Code: "none"},
				Yield: &driver.Yield{
					SchemaVersion: driver.YieldSchemaVersion,
					InvocationID:  invocation.Request.InvocationID,
					Kind:          driver.YieldQuestion,
					Message:       "Clarification on S2 required",
				},
			}, nil
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
		}, nil
	})

	escalateSlice(t, fixture, "S1", "Captain escalated S1.")

	// Run driveLoop: S2 is ready, its advanceSlice returns EFFECT_PARKED (because of Yield).
	// Then driveLoop reaches the :5690-5695 branch (ready existed but no progress, plannerNeeded=true).
	if err := fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false); err != nil {
		t.Fatalf("driveLoop error: %v", err)
	}

	if calls := atomic.LoadInt64(&plannerCalls); calls != 0 {
		t.Fatalf("driver.RolePlanner invocations = %d, want 0", calls)
	}

	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasParkEventForCause(snapshot, ParkCauseBootstrapAuthority, "") {
		t.Fatal("expected ParkCauseBootstrapAuthority event in journal")
	}
}

// A2(1) & A2(2): Reason verbatim preserves newlines, and re-entering driveLoop appends no duplicate event.
func TestBootstrapParkReasonMultilineSummaryAndIdempotence(t *testing.T) {
	t.Parallel()
	fixture := newBootstrapFixtureSingleSlice(t)

	multilineSummary := "Line 1: Refusal reason\nLine 2: Further details with control char \t tab\nLine 3: End of explanation."
	escalateSlice(t, fixture, "S1", multilineSummary)

	if err := fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false); err != nil {
		t.Fatalf("first driveLoop error: %v", err)
	}

	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}

	eventCount := 0
	var parkBody []byte
	for _, event := range snapshot.Events {
		if event.Kind == ParkEventKind {
			parsed, parseErr := ParseDegradationParkEvent(event.Body)
			if parseErr == nil && parsed.Cause == ParkCauseBootstrapAuthority {
				eventCount++
				parkBody = event.Body
			}
		}
	}
	if eventCount != 1 {
		t.Fatalf("park events count = %d, want 1", eventCount)
	}

	parsed, err := ParseDegradationParkEvent(parkBody)
	if err != nil {
		t.Fatalf("ParseDegradationParkEvent failed: %v", err)
	}

	wantReason := multilineSummary + "\n\n" + bootstrapParkUnblockDirective
	if parsed.Reason != wantReason {
		t.Fatalf("park event reason =\n%q\nwant:\n%q", parsed.Reason, wantReason)
	}

	// Re-enter driveLoop: must append no duplicate event
	if err := fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false); err != nil {
		t.Fatalf("second driveLoop error: %v", err)
	}

	snapshot2, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}

	eventCount2 := 0
	for _, event := range snapshot2.Events {
		if event.Kind == ParkEventKind {
			parsed, parseErr := ParseDegradationParkEvent(event.Body)
			if parseErr == nil && parsed.Cause == ParkCauseBootstrapAuthority {
				eventCount2++
			}
		}
	}
	if eventCount2 != 1 {
		t.Fatalf("after re-entry, park events count = %d, want 1", eventCount2)
	}
}

// A2(3): RunStatus reports State=="parked" and Park.Cause==ParkCauseBootstrapAuthority with .Reason set on no-proposal path.
func TestBootstrapParkStatusReportsStateParkedAndReason(t *testing.T) {
	t.Parallel()
	fixture := newBootstrapFixtureSingleSlice(t)

	summary := "Captain escalated slice S1."
	escalateSlice(t, fixture, "S1", summary)

	// Before driveLoop runs, Status() should already report parked from Baton state and authority
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status.State != "parked" {
		t.Fatalf("status.State = %q, want %q", status.State, "parked")
	}
	if status.Park == nil {
		t.Fatal("status.Park is nil")
	}
	if status.Park.Cause != ParkCauseBootstrapAuthority {
		t.Fatalf("status.Park.Cause = %q, want %q", status.Park.Cause, ParkCauseBootstrapAuthority)
	}
	wantReason := summary + "\n\n" + bootstrapParkUnblockDirective
	if status.Park.Reason != wantReason {
		t.Fatalf("status.Park.Reason = %q, want %q", status.Park.Reason, wantReason)
	}

	// Now run driveLoop and verify Status() still reports parked
	if err := fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false); err != nil {
		t.Fatalf("driveLoop failed: %v", err)
	}

	statusAfter, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatalf("Status after driveLoop failed: %v", err)
	}
	if statusAfter.State != "parked" {
		t.Fatalf("statusAfter.State = %q, want %q", statusAfter.State, "parked")
	}
	if statusAfter.Park == nil || statusAfter.Park.Cause != ParkCauseBootstrapAuthority || statusAfter.Park.Reason != wantReason {
		t.Fatalf("statusAfter.Park = %#v, want cause %q and reason %q", statusAfter.Park, ParkCauseBootstrapAuthority, wantReason)
	}
}

// A3(1): Journaled plan_authority command preserves normal planner dispatch with captain/escalate slice.
func TestDriveLoopDispatchesPlannerWithJournaledPlanAuthority_CaptainEscalate(t *testing.T) {
	t.Parallel()
	fixture := newBootstrapFixtureSingleSlice(t)

	var plannerCalls int64
	fixture.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		if invocation.Request.Role == driver.RolePlanner {
			atomic.AddInt64(&plannerCalls, 1)
			planBytes, _ := runtimePlan(t, fixture.manifest.value.Release, fixture.manifest.value.Authority.Project, fixture.manifest.value.TargetRef, "approval-release-1-v2")
			planValue, _ := driver.NewPlanBytes(planBytes)
			sub := driver.Submission{
				SchemaVersion:  driver.SubmissionSchemaVersion,
				InvocationID:   invocation.Request.InvocationID,
				Responsibility: driver.PlannerProposal,
				Summary:        "Planner proposal after escalation.",
				Plan:           planValue,
			}
			encoded, _ := driver.EncodeSubmission(sub)
			return driver.Observation{
				TransportStatus: driver.Completed,
				Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
				Diagnostic:      driver.Diagnostic{Code: "none"},
				Handoff: &driver.SealedHandoff{
					SubmissionBytes:  encoded,
					SubmissionDigest: driver.Digest(encoded),
				},
			}, nil
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
		}, nil
	})

	escalateSlice(t, fixture, "S1", "Captain escalated S1.")

	// Record plan_authority command
	planDigest := *fixture.manifest.value.Authority.BootstrapApprovedPlanDigest
	if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
		RunID:     fixture.manifest.value.RunID,
		ReplayKey: "plan-authority/" + strings.TrimPrefix(planDigest, "sha256:"),
		Kind:      "plan_authority",
		Payload: mustJSON(planAuthorityCommand{
			Version:    planAuthorityVersion,
			PlanDigest: planDigest,
		}),
		CreatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}

	// driveLoop should now dispatch RolePlanner instead of parking
	_ = fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false)

	if calls := atomic.LoadInt64(&plannerCalls); calls == 0 {
		t.Fatal("expected driver.RolePlanner to be dispatched when plan_authority command is recorded")
	}
}

// A3(2): Journaled plan_authority command preserves normal planner dispatch with assembly in verifier/blocked.
func TestDriveLoopDispatchesPlannerWithJournaledPlanAuthority_AssemblyVerifierBlocked(t *testing.T) {
	t.Parallel()
	fixture := newDegradationStatusFixture(t)

	var plannerCalls int64
	fixture.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		if invocation.Request.Role == driver.RolePlanner {
			atomic.AddInt64(&plannerCalls, 1)
			planBytes, _ := runtimePlan(t, fixture.manifest.value.Release, fixture.manifest.value.Authority.Project, fixture.manifest.value.TargetRef, "approval-release-1-v2")
			planValue, _ := driver.NewPlanBytes(planBytes)
			sub := driver.Submission{
				SchemaVersion:  driver.SubmissionSchemaVersion,
				InvocationID:   invocation.Request.InvocationID,
				Responsibility: driver.PlannerProposal,
				Summary:        "Planner proposal after assembly blocked.",
				Plan:           planValue,
			}
			encoded, _ := driver.EncodeSubmission(sub)
			return driver.Observation{
				TransportStatus: driver.Completed,
				Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
				Diagnostic:      driver.Diagnostic{Code: "none"},
				Handoff: &driver.SealedHandoff{
					SubmissionBytes:  encoded,
					SubmissionDigest: driver.Digest(encoded),
				},
			}, nil
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
		}, nil
	})

	passAllSlicesAndBlockAssembly(t, fixture, "Assembly verifier blocked.")

	// Record plan_authority command
	planDigest := *fixture.manifest.value.Authority.BootstrapApprovedPlanDigest
	if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
		RunID:     fixture.manifest.value.RunID,
		ReplayKey: "plan-authority/" + strings.TrimPrefix(planDigest, "sha256:"),
		Kind:      "plan_authority",
		Payload: mustJSON(planAuthorityCommand{
			Version:    planAuthorityVersion,
			PlanDigest: planDigest,
		}),
		CreatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}

	// driveLoop should now dispatch RolePlanner
	_ = fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false)

	if calls := atomic.LoadInt64(&plannerCalls); calls == 0 {
		t.Fatal("expected driver.RolePlanner to be dispatched for assembly verifier/blocked when plan_authority is recorded")
	}
}

// A4(1) & A4(2): Manifest with no BootstrapApprovedPlanDigest dispatches planner on plannerNeeded,
// and RunStatus never reports ParkCauseBootstrapAuthority.
func TestOrdinaryExternalAuthorityDispatchesPlannerAndNeverBootstrapParks(t *testing.T) {
	t.Parallel()
	fixture := newFixtureSingleSlice(t, false)

	escalateSlice(t, fixture, "S1", "Captain escalated S1.")

	// Check status: must NOT be ParkCauseBootstrapAuthority
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Park != nil && status.Park.Cause == ParkCauseBootstrapAuthority {
		t.Fatalf("status.Park has ParkCauseBootstrapAuthority under external authority: %#v", status.Park)
	}

	var plannerCalls int64
	fixture.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		if invocation.Request.Role == driver.RolePlanner {
			atomic.AddInt64(&plannerCalls, 1)
			planBytes, _ := runtimePlan(t, fixture.manifest.value.Release, fixture.manifest.value.Authority.Project, fixture.manifest.value.TargetRef, "approval-release-1-v2")
			planValue, _ := driver.NewPlanBytes(planBytes)
			sub := driver.Submission{
				SchemaVersion:  driver.SubmissionSchemaVersion,
				InvocationID:   invocation.Request.InvocationID,
				Responsibility: driver.PlannerProposal,
				Summary:        "External authority planner proposal.",
				Plan:           planValue,
			}
			encoded, _ := driver.EncodeSubmission(sub)
			return driver.Observation{
				TransportStatus: driver.Completed,
				Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
				Diagnostic:      driver.Diagnostic{Code: "none"},
				Handoff: &driver.SealedHandoff{
					SubmissionBytes:  encoded,
					SubmissionDigest: driver.Digest(encoded),
				},
			}, nil
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "none"},
		}, nil
	})

	_ = fixture.service.driveLoop(fixture.ctx, fixture.engine, fixture.owner, false)

	if calls := atomic.LoadInt64(&plannerCalls); calls == 0 {
		t.Fatal("expected driver.RolePlanner to be dispatched when BootstrapApprovedPlanDigest is nil")
	}
}

// Captain Correction 2: A bootstrap-only run with no planner-needed slice reads "running" with Park == nil.
func TestStatusBootstrapAuthorityNotParkedWhenNoPlannerNeeded(t *testing.T) {
	t.Parallel()
	fixture := newBootstrapFixtureSingleSlice(t)

	// S1 is ready in design stage.
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status.State != "running" {
		t.Fatalf("status.State = %q, want %q", status.State, "running")
	}
	if status.Park != nil {
		t.Fatalf("status.Park = %#v, want nil", status.Park)
	}
}

// Captain Correction 3: canonicalDegradationParkEvent validation and field bounds.
func TestCanonicalDegradationParkEventBootstrapAuthority(t *testing.T) {
	t.Parallel()

	validEvent := DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         "run-1",
		Cause:         ParkCauseBootstrapAuthority,
		UnblockKnob:   "",
		Reason:        "Summary\n\n" + bootstrapParkUnblockDirective,
	}

	// Valid roundtrip
	canonical, err := canonicalDegradationParkEvent(validEvent)
	if err != nil {
		t.Fatalf("canonicalDegradationParkEvent failed: %v", err)
	}
	parsed, err := ParseDegradationParkEvent(canonical)
	if err != nil {
		t.Fatalf("ParseDegradationParkEvent failed: %v", err)
	}
	if parsed.Reason != validEvent.Reason {
		t.Fatalf("parsed.Reason = %q, want %q", parsed.Reason, validEvent.Reason)
	}

	// Empty reason rejected
	emptyReason := validEvent
	emptyReason.Reason = ""
	if _, err := canonicalDegradationParkEvent(emptyReason); err == nil {
		t.Fatal("expected empty Reason to be rejected")
	}

	// Overlong reason rejected
	overlongReason := validEvent
	overlongReason.Reason = strings.Repeat("a", maxParkReasonBytes+1)
	if _, err := canonicalDegradationParkEvent(overlongReason); err == nil {
		t.Fatal("expected overlong Reason to be rejected")
	}

	// Non-empty UnblockKnob rejected
	withKnob := validEvent
	withKnob.UnblockKnob = "limits.degradation_fallback_budget"
	if _, err := canonicalDegradationParkEvent(withKnob); err == nil {
		t.Fatal("expected non-empty UnblockKnob to be rejected for bootstrap_authority")
	}

	// Reason on degradation cause rejected
	degradationWithReason := DegradationParkEvent{
		SchemaVersion: DegradationParkEventVersion,
		RunID:         "run-1",
		Cause:         ParkCauseDegradation,
		Count:         4,
		Budget:        3,
		UnblockKnob:   DegradationUnblockKnob,
		Fallbacks: []DegradationFallback{
			{Offset: 1, Reason: "r1"},
			{Offset: 2, Reason: "r2"},
			{Offset: 3, Reason: "r3"},
			{Offset: 4, Reason: "r4"},
		},
		Reason: "forbidden reason",
	}
	if _, err := canonicalDegradationParkEvent(degradationWithReason); err == nil {
		t.Fatal("expected Reason to be forbidden on ParkCauseDegradation")
	}

	// Reason on economy cause rejected
	economyWithReason := DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         "run-1",
		Cause:         ParkCauseEconomyTurns,
		Spent:         10,
		Budget:        5,
		UnblockKnob:   EconomyTurnsUnblockKnob,
		Work:          "sha256:" + strings.Repeat("a", 64),
		Reason:        "forbidden reason",
	}
	if _, err := canonicalDegradationParkEvent(economyWithReason); err == nil {
		t.Fatal("expected Reason to be forbidden on ParkCauseEconomyTurns")
	}

	// Reason on identical failure cause rejected
	identicalWithReason := DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         "run-1",
		Cause:         ParkCauseIdenticalFailure,
		Consecutive:   2,
		Threshold:     2,
		FailureCode:   "TIMEOUT",
		UnblockKnob:   IdenticalFailureUnblockKnob,
		Work:          "sha256:" + strings.Repeat("a", 64),
		Reason:        "forbidden reason",
	}
	if _, err := canonicalDegradationParkEvent(identicalWithReason); err == nil {
		t.Fatal("expected Reason to be forbidden on ParkCauseIdenticalFailure")
	}
}

// Captain Correction 4: Nil pointer and empty Summary safety in triggering receipt selection.
func TestTriggeringPlannerReceiptNilAndEmptySummarySafety(t *testing.T) {
	t.Parallel()

	// Empty state: no slices, no assembly
	emptyState := baton.State{}
	reason := bootstrapParkReasonForState(emptyState)
	wantDefault := "Planner revision required.\n\n" + bootstrapParkUnblockDirective
	if reason != wantDefault {
		t.Fatalf("reason for empty state = %q, want %q", reason, wantDefault)
	}

	// Slice with nil CurrentReceipt
	sliceNilReceipt := baton.State{
		Slices: []*baton.SliceState{
			{NextRole: "planner", CurrentReceipt: nil},
		},
	}
	reason = bootstrapParkReasonForState(sliceNilReceipt)
	if reason != wantDefault {
		t.Fatalf("reason for nil CurrentReceipt = %q, want %q", reason, wantDefault)
	}

	// Slice with empty Summary
	sliceEmptySummary := baton.State{
		Slices: []*baton.SliceState{
			{NextRole: "planner", CurrentReceipt: &baton.ReceiptEntry{
				Receipt: baton.Receipt{Summary: ""},
			}},
		},
	}
	reason = bootstrapParkReasonForState(sliceEmptySummary)
	if reason != wantDefault {
		t.Fatalf("reason for empty Summary = %q, want %q", reason, wantDefault)
	}

	// Assembly with nil CurrentReceipt when no slice carries planner
	assemblyNilReceipt := baton.State{
		Assembly: baton.AssemblyState{NextRole: "planner", CurrentReceipt: nil},
	}
	reason = bootstrapParkReasonForState(assemblyNilReceipt)
	if reason != wantDefault {
		t.Fatalf("reason for assembly nil CurrentReceipt = %q, want %q", reason, wantDefault)
	}
}
