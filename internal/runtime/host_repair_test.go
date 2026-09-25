package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

func TestHostCheckFailureRetriesWithRetainedWorkAndExactFeedback(t *testing.T) {
	t.Run("inline contract", func(t *testing.T) { testHostRepairRoundTrip(t, false) })
	t.Run("digest-addressed manifest contract", func(t *testing.T) { testHostRepairRoundTrip(t, true) })
}

func testHostRepairRoundTrip(t *testing.T, manifestPlan bool) {
	f := newHostCheckFixture(t, []string{"grep -q repaired one.txt || { echo 'one.txt needs repair'; exit 7; }"}, manifestPlan)
	calls := 0
	f.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		calls++
		var work productionWorkContext
		for _, input := range invocation.Inputs {
			if input.Input.Name == "work-context" {
				if err := json.Unmarshal(input.Bytes, &work); err != nil {
					t.Fatal(err)
				}
			}
		}
		path := filepath.Join(invocation.HostWorkspace, "one.txt")
		switch calls {
		case 1:
			if work.HostRepair != nil {
				t.Fatal("first try got repair context")
			}
			if err := os.WriteFile(path, []byte("valuable unfinished work\n"), 0600); err != nil {
				t.Fatal(err)
			}
		case 2:
			body, err := os.ReadFile(path)
			if err != nil || string(body) != "valuable unfinished work\n" {
				t.Fatalf("retry lost work: %q %v", body, err)
			}
			if work.HostRepair == nil {
				t.Fatal("retry lacks host repair context")
			}
			repair := work.HostRepair
			if repair.FailedCheck.ExitCode != 7 || repair.FailedCheck.Check != f.hostChecks[0] || !strings.Contains(repair.FailedCheck.Output, "one.txt needs repair") || repair.Submission.Summary != "Durable production candidate." {
				t.Fatalf("repair context = %#v", repair)
			}
			if work.Candidate != nil || work.HostEvidence != nil {
				t.Fatal("unverified repair became verified evidence")
			}
			priorInvocation := repair.Submission.InvocationID
			manualCoordinates := dispatchCoordinates{Slice: "S1", Responsibility: driver.ImplementerImplementation, ProtocolAttempt: work.Attempt, Epoch: 2, Try: 1}
			manualContext := work
			manualContext.Epoch, manualContext.Try = 2, 1
			manualContext.InvocationID = dispatchInvocationID(work.RunID, manualCoordinates)
			if err := validateProductionWorkContext(f.manifest, manualContext); err != nil {
				t.Fatalf("manual retry context refused: %v", err)
			}
			for name, mutate := range map[string]func(*productionHostRepair){
				"output":     func(r *productionHostRepair) { r.FailedCheck.Output += "altered" },
				"candidate":  func(r *productionHostRepair) { r.FailedCheck.Candidate = strings.Repeat("f", 40) },
				"slice":      func(r *productionHostRepair) { r.FailedCheck.Slice = "S2" },
				"submission": func(r *productionHostRepair) { r.Submission.InvocationID = invocation.Request.InvocationID },
				"pass":       func(r *productionHostRepair) { r.FailedCheck.Outcome = protocol.CheckOutcomePass },
				"schema":     func(r *productionHostRepair) { r.SchemaVersion = "future" },
			} {
				changed := *repair
				mutate(&changed)
				if err := validateHostRepair(changed, priorInvocation, "S1"); err == nil {
					t.Fatalf("accepted substituted %s", name)
				}
			}
			for name, mutate := range map[string]func(*productionWorkContext){
				"authority": func(w *productionWorkContext) { w.Before = driver.Digest([]byte("other")) },
				"base":      func(w *productionWorkContext) { w.Authority.TrackHead = strings.Repeat("f", 40) },
				"role":      func(w *productionWorkContext) { w.Responsibility = driver.WorkVerification },
			} {
				changed := work
				mutate(&changed)
				if err := validateProductionWorkContext(f.manifest, changed); err == nil {
					t.Fatalf("accepted stale repair %s", name)
				}
			}
			if err := os.WriteFile(path, append(body, []byte("repaired\n")...), 0600); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unexpected blind retry %d", calls)
		}
		return productionImplementationObservation(t, invocation), nil
	})
	slice, _ := f.state.Slice("S1")
	if err := f.service.implementSlice(f.ctx, f.engine, f.owner, f.state, slice); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("dispatches = %d", calls)
	}
	state, err := protocol.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := state.Slice("S1")
	if current.Candidate == nil || current.Pass != nil || current.NextRole != "verifier" {
		t.Fatalf("candidate bypassed independent verification: %#v", current)
	}
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "driver.dispatch" && effect.ErrorCode == "HOST_CHECK_FAILED" {
			failures++
			var repair productionHostRepair
			if effect.State != journal.OperationalFailed || json.Unmarshal(effect.Result, &repair) != nil || repair.SchemaVersion != hostRepairVersion {
				t.Fatal("failure did not durably retain repair input")
			}
		}
	}
	if failures != 1 {
		t.Fatalf("failed dispatches = %d", failures)
	}
}

func TestRepeatedHostFailuresPersistActionableParkExactlyOnce(t *testing.T) {
	f := newHostCheckFixture(t, []string{"echo 'still needs repair'; exit 7"})
	calls := 0
	f.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		calls++
		path := filepath.Join(invocation.HostWorkspace, "one.txt")
		if calls > 1 {
			if body, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(body), "retained") {
				t.Fatalf("lost prior candidate: %q %v", body, err)
			}
		}
		if err := os.WriteFile(path, []byte("retained"+strings.Repeat("x", calls)), 0600); err != nil {
			t.Fatal(err)
		}
		return productionImplementationObservation(t, invocation), nil
	})
	slice, _ := f.state.Slice("S1")
	if err := f.service.implementSlice(f.ctx, f.engine, f.owner, f.state, slice); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("repeat failure = %v", err)
	}
	if int64(calls) != f.manifest.value.EffectiveIdenticalFailureParkAfter() {
		t.Fatalf("dispatches = %d", calls)
	}
	state, err := protocol.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	work := workIdentity(sliceFingerprint(state, "S1"), "git.seal")
	current, _ := state.Slice("S1")
	manualCoordinates := dispatchCoordinates{Slice: "S1", Responsibility: driver.ImplementerImplementation, ProtocolAttempt: current.Attempt, Epoch: 2, Try: 1}
	manualContext, _, err := captureProductionWorkContext(f.ctx, f.engine, manualCoordinates, sliceFingerprint(state, "S1"), driver.ReadWrite)
	if err != nil || manualContext.HostRepair == nil || manualContext.HostRepair.SourceEpoch != 1 || manualContext.HostRepair.SourceTry != int64(calls) {
		t.Fatalf("manual retry forgot parked failure: %#v %v", manualContext.HostRepair, err)
	}
	for i := 0; i < 2; i++ {
		if parked, err := f.service.economyGuardsParked(f.ctx, f.manifest, f.owner.RunID, work); err != nil || !parked {
			t.Fatalf("park replay = %v %v", parked, err)
		}
	}
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	parks := 0
	for _, event := range snapshot.Events {
		if event.Kind == "degradation_budget_parked" {
			parks++
			park, err := ParseDegradationParkEvent(event.Body)
			if err != nil || park.FailureCode != "HOST_CHECK_FAILED" || !strings.Contains(park.FailureDetail, "retained unverified candidate") || park.Work != work {
				t.Fatalf("actionable park = %#v %v", park, err)
			}
		}
	}
	if parks != 1 {
		t.Fatalf("durable notification-source events = %d", parks)
	}
}

func TestMissingLegacyHostRepairCannotFallThroughOnLaterRetry(t *testing.T) {
	f := newHostCheckFixture(t, []string{"exit 7"})
	before := sliceFingerprint(f.state, "S1")
	work := workIdentity(before, "git.seal")
	writeFailure := func(workID string, retry int64, kind, code string) {
		t.Helper()
		id := journal.AttemptEffectID(workID, 1, retry)
		payload := mustJSON(map[string]string{"fixture": kind})
		now := f.service.now()
		if err := f.store.EnsureAttempt(f.ctx,
			journal.Command{RunID: f.owner.RunID, ReplayKey: id, Kind: kind, Payload: payload, CreatedAt: now},
			journal.Effect{RunID: f.owner.RunID, ID: id, ReplayKey: id, Kind: kind, BeforeDigest: workID, ExpectedDigest: driver.Digest(payload), UpdatedAt: now},
			journal.EffectAttempt{WorkID: workID, Epoch: 1, Try: retry}); err != nil {
			t.Fatal(err)
		}
		claim, err := f.store.ClaimOwned(f.ctx, f.owner, id, now, effectLease)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.CompleteOwned(f.ctx, f.owner, journal.Completion{RunID: f.owner.RunID, EffectID: id, Token: claim.Token, State: journal.OperationalFailed, ErrorCode: code, EventKind: "fixture_failure", At: now}); err != nil {
			t.Fatal(err)
		}
	}
	writeFailure(workIdentity(work, "driver.dispatch"), 1, "driver.dispatch", "HOST_CHECK_FAILED")
	coordinates := dispatchCoordinates{Slice: "S1", Responsibility: driver.ImplementerImplementation, ProtocolAttempt: 1, Epoch: 1, Try: 2}
	plan, err := currentPlanBinding(f.state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := captureHostRepair(f.ctx, f.engine, coordinates, before, plan); !IsCode(err, "HOST_REPAIR_UNAVAILABLE") {
		t.Fatalf("legacy failure = %v", err)
	}
	// Admission refused before driver.dispatch existed for try 2. Try 3 must
	// not interpret that missing dispatch as permission to restart blindly.
	writeFailure(work, 2, "git.seal", "HOST_REPAIR_UNAVAILABLE")
	coordinates.Try = 3
	if _, err := captureHostRepair(f.ctx, f.engine, coordinates, before, plan); !IsCode(err, "HOST_REPAIR_UNAVAILABLE") {
		t.Fatalf("later retry fell through: %v", err)
	}
	coordinates.Epoch, coordinates.Try = 2, 1
	if _, err := captureHostRepair(f.ctx, f.engine, coordinates, before, plan); !IsCode(err, "HOST_REPAIR_UNAVAILABLE") {
		t.Fatalf("new epoch lost refusal: %v", err)
	}
}

// anchorGatePlanBytes declares S1 with an "Anchor: README.md" clause -
// README.md already exists in productionRepository's base commit - and a
// scope wide enough to admit a candidate that touches either the anchor or
// an unrelated in-scope file, so a test can construct both an untouched-
// anchor refusal and an honest, anchor-touching candidate.
func anchorGatePlanBytes(t *testing.T, release, repository, target string) []byte {
	t.Helper()
	slice := protocol.Slice{
		ID: "S1", Outcome: "Deliver S1.",
		Scope:      protocol.Scope{Include: []string{"one.txt", "README.md"}, Exclude: []string{}},
		Acceptance: []protocol.Criterion{{ID: "A2", Text: "Anchor presence test. Anchor: README.md."}},
		Checks:     []string{"check S1"}, Constraints: []string{"deterministic"},
		DependsOn: []string{}, Consumes: []string{},
	}
	metadata := protocol.Metadata{
		SchemaVersion: protocol.PlanVersion,
		Release:       release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    repository,
		TargetRef:     target,
		ApprovalRef:   "operator://" + release + "/1",
		Tracks: []protocol.Track{
			{ID: "T1", DependsOn: []string{}, Slices: []protocol.Slice{slice}},
		},
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte("```protocol-plan-v2\n" + string(body) + "\n```\n\nAnchor gate fixture plan.\n")
}

// newAnchorGateImplementationFixture builds the same production seal-path
// fixture newProductionImplementationRecoveryFixture does, over
// anchorGatePlanBytes's Anchor-bearing S1 contract, so runProductionImplementationDispatch
// exercises claimPreparedImplementation's anchor-presence gate exactly as it
// exercises the scope gate in refusal_paths_test.go.
func newAnchorGateImplementationFixture(
	t *testing.T,
	dispatcher driver.Driver,
) *productionImplementationRecoveryFixture {
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
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, manifest.value.RunID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{
		journal: store, dispatcher: dispatcher, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now },
	}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	planBytes := anchorGatePlanBytes(
		t, manifest.value.Release, manifest.value.Authority.Project, manifest.value.TargetRef)
	if _, err := engine.actions.RecordPlanRevision(protocol.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install the exact anchor-gate fixture plan.",
		Detail:    []byte("Anchor-gate fixture."),
	}); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []protocol.AppendReceiptInput{
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "implementer", Result: "designed",
			Summary: "Design the anchor-gate fixture.",
			Detail:  []byte("Exact design."),
		},
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "lead", Result: "proceed",
			Summary: "Proceed with the anchor-gate fixture.",
			Detail:  []byte("Exact review."),
		},
	} {
		if _, err := engine.actions.AppendReceipt(receipt); err != nil {
			t.Fatal(err)
		}
	}
	state, err := protocol.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, sliceOK := state.Slice("S1")
	track, trackOK := state.Track("T1")
	if !sliceOK || !trackOK || slice.CurrentReceipt == nil ||
		slice.Stage != "implement" || slice.NextRole != "implementer" {
		t.Fatalf("implementation authority = %#v", state)
	}
	before := sliceFingerprint(state, "S1")
	outerWork := workIdentity(before, "git.seal")
	outerID := journal.AttemptEffectID(outerWork, 1, 1)
	cycle := implementationCycle{GitIdentity: runtimeTestGitIdentity,
		Release: state.Release, Slice: "S1",
		Binds: slice.CurrentReceipt.OID, Before: before,
		Plan: state.Plan.OID, ReleaseHead: state.Refs.Release.Head,
		TargetHead: state.Refs.Target.Head, Track: track.ID,
		TrackRef: track.Ref, TrackHead: track.Head,
		DispatchWork: workIdentity(outerWork, "driver.dispatch"),
		PreparedWork: workIdentity(outerWork, "git.seal.prepared"),
	}
	cycle.DispatchEffect = journal.AttemptEffectID(cycle.DispatchWork, 1, 1)
	cycle.PreparedEffect = journal.AttemptEffectID(cycle.PreparedWork, 1, 1)
	outerPayload := mustJSON(cycle)
	if err := store.EnsureAttempt(ctx,
		journal.Command{
			RunID: owner.RunID, ReplayKey: outerID,
			Kind: "git.seal", Payload: outerPayload, CreatedAt: now,
		},
		journal.Effect{
			RunID: owner.RunID, ID: outerID, ReplayKey: outerID,
			Kind: "git.seal", BeforeDigest: outerWork,
			ExpectedDigest: sha256Digest(outerPayload), UpdatedAt: now,
		},
		journal.EffectAttempt{WorkID: outerWork, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	outerClaim, err := store.ClaimOwned(ctx, owner, outerID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := engine.workspaces.OpenTrack(
		gitx.TrackKey{Release: state.Release, Track: track.ID},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &productionImplementationRecoveryFixture{
		ctx: ctx, repository: repository, config: config,
		manifest: manifest, store: store, owner: owner, now: now,
		service: service, engine: engine, state: state,
		slice: slice, track: track, cycle: cycle,
		outer: journal.Effect{
			RunID: owner.RunID, ID: outerID, Kind: "git.seal",
			State: journal.Claimed, CurrentClaim: outerClaim.Token,
		},
		workspace: workspace,
		coordinates: dispatchCoordinates{
			Slice: "S1", Responsibility: driver.ImplementerImplementation,
			ProtocolAttempt: slice.Attempt, Epoch: 1, Try: 1,
		},
	}
}

// TestAnchorGateRefusesCandidateThatTouchesNoAnchorFileBeforeAnyHostCheck
// pins A2's core promise: a candidate that touches none of a criterion's
// declared anchor files is refused ANCHOR_NOT_TOUCHED before a single
// check.host effect exists for it, and the refusal reaches the next
// same-authority implementer dispatch's work context so the worker can
// repair it in one further dispatch rather than a full evidence round.
func TestAnchorGateRefusesCandidateThatTouchesNoAnchorFileBeforeAnyHostCheck(t *testing.T) {
	dispatcher := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newAnchorGateImplementationFixture(t, dispatcher)

	// The candidate touches only one.txt, in scope but not the criterion's
	// declared anchor (README.md).
	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "one.txt"),
		[]byte("unrelated change\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace,
		fixture.cycle, fixture.coordinates,
	)
	if dispatchErr == nil {
		t.Fatal("expected dispatch to fail on untouched anchor, got nil")
	}
	if err := fixture.service.completeImplementationFailure(
		fixture.ctx, fixture.owner, fixture.outer.ID, fixture.outer.CurrentClaim,
		stableErrorCode(dispatchErr), extractRefusalResult(dispatchErr),
	); err != nil {
		t.Fatal(err)
	}

	sealEffect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, fixture.outer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sealEffect.ErrorCode != "ANCHOR_NOT_TOUCHED" {
		t.Fatalf("seal error code = %s, want ANCHOR_NOT_TOUCHED", sealEffect.ErrorCode)
	}
	var sealRefusal productionRefusalBinding
	if err := json.Unmarshal(sealEffect.Result, &sealRefusal); err != nil {
		t.Fatalf("cannot decode seal effect refusal: %v", err)
	}
	if sealRefusal.Code != "ANCHOR_NOT_TOUCHED" ||
		!strings.Contains(sealRefusal.Detail, "A2") ||
		!strings.Contains(sealRefusal.Detail, "anchor base") {
		t.Fatalf("unexpected seal refusal: %#v", sealRefusal)
	}
	if len(sealRefusal.Paths) == 0 || sealRefusal.Paths[0] != "README.md" {
		t.Fatalf("expected README.md named in refusal paths, got %v", sealRefusal.Paths)
	}

	// No check.host effect may exist for this refused candidate: the anchor
	// gate runs before resolveSliceHostChecks/runHostChecks are ever
	// reached in claimPreparedImplementation.
	dispatchEffect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, fixture.cycle.DispatchEffect)
	if err != nil {
		t.Fatal(err)
	}
	if dispatchEffect.ErrorCode != "ANCHOR_NOT_TOUCHED" {
		t.Fatalf("dispatch error code = %s, want ANCHOR_NOT_TOUCHED", dispatchEffect.ErrorCode)
	}

	// The refusal reaches the next same-authority implementer dispatch's
	// work context.
	retryCoords := fixture.coordinates
	retryCoords.Try = 2
	retryWorkContext, _, err := captureProductionWorkContext(
		fixture.ctx, fixture.engine, retryCoords, fixture.cycle.Before, driver.ReadWrite,
	)
	if err != nil {
		t.Fatalf("captureProductionWorkContext failed: %v", err)
	}
	if retryWorkContext.Refusal == nil || retryWorkContext.Refusal.Code != "ANCHOR_NOT_TOUCHED" {
		t.Fatalf("expected retry work context to carry the ANCHOR_NOT_TOUCHED refusal, got %#v", retryWorkContext.Refusal)
	}
	if !strings.Contains(retryWorkContext.Refusal.Detail, "A2") {
		t.Fatalf("expected the refusal detail to name the criterion, got %q", retryWorkContext.Refusal.Detail)
	}
}

// TestAnchorGateAdmitsCandidateThatTouchesTheDeclaredAnchor proves the
// converse: a candidate that does touch the criterion's declared anchor
// file clears the gate and reaches the checks-decoding step beyond it.
func TestAnchorGateAdmitsCandidateThatTouchesTheDeclaredAnchor(t *testing.T) {
	dispatcher := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newAnchorGateImplementationFixture(t, dispatcher)

	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "README.md"),
		[]byte("production fixture\ncovering the anchor\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace,
		fixture.cycle, fixture.coordinates,
	)
	if dispatchErr != nil {
		t.Fatalf("expected the anchor-touching candidate to clear the gate, got %v", dispatchErr)
	}
}

// S3-host-check-failure-facts A1 + lead correction 1: an in-flight next try
// (claimed, not terminal) must not clear the fact; a later terminal
// dispatch does, as A6 requires. The fact appears on EffectStatus even when
// the work is not pinned (lead correction 4).
func TestHostCheckFailureFactSurvivesInFlightNextTryAndClearsAfterPassing(t *testing.T) {
	failing := "echo 'needs repair'; exit 7"
	fixture := newHostCheckFixture(t, []string{failing})
	_, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	var failure *hostCheckFailure
	if err == nil {
		t.Fatal("expected HOST_CHECK_FAILED")
	}
	if !isHostCheckFailure(err, &failure) {
		t.Fatalf("expected hostCheckFailure, got %v", err)
	}
	recordFixtureManifestCommand(t, fixture)
	work := "sha256:" + strings.Repeat("c", 64)
	repair := buildRepairFromHostResult(t, fixture, failure.result, 1, 1)
	failedID := journalHostFailedDispatchForFact(t, fixture, work, 1, 1, repair)
	// In-flight next try: claimed, never terminal.
	nextID := journal.AttemptEffectID(work, 1, 2)
	payload := mustJSON(map[string]string{"work": work})
	if err := fixture.store.RecordCommandEffect(fixture.ctx, journal.Command{
		RunID: fixture.manifest.value.RunID, ReplayKey: nextID, Kind: "driver.dispatch",
		Payload: payload, CreatedAt: fixture.service.now().UTC(),
	}, journal.Effect{
		RunID: fixture.manifest.value.RunID, ID: nextID, ReplayKey: nextID,
		Kind: "driver.dispatch", State: journal.Pending,
		BeforeDigest: sha256Digest(payload), ExpectedDigest: "sha256:" + strings.Repeat("d", 64),
		UpdatedAt: fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Claim(fixture.ctx, fixture.manifest.value.RunID, nextID, fixture.service.now().UTC(), time.Minute); err != nil {
		t.Fatal(err)
	}
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]EffectStatus, len(status.Effects))
	for _, effect := range status.Effects {
		byID[effect.ID] = effect
	}
	failedStatus, ok := byID[failedID]
	if !ok || failedStatus.HostCheckFailure == nil {
		t.Fatalf("in-flight next try cleared the fact: %#v", failedStatus)
	}
	if failedStatus.HostCheckFailure.Check != failing || failedStatus.HostCheckFailure.ExitCode != 7 {
		t.Fatalf("fact = %#v", failedStatus.HostCheckFailure)
	}
	if nextStatus, ok := byID[nextID]; !ok || nextStatus.HostCheckFailure != nil {
		t.Fatalf("in-flight dispatch carries a fact: %#v", nextStatus)
	}
	if len(status.PinnedWork) != 0 {
		t.Fatalf("single failure pinned work: %#v", status.PinnedWork)
	}
	// A later terminal success for the same owner clears the fact everywhere
	// for that owner, as A6 requires (no stale fact remains).
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var claimed journal.Effect
	for _, effect := range snapshot.Effects {
		if effect.ID == nextID {
			claimed = effect
		}
	}
	if claimed.State != journal.Claimed {
		t.Fatalf("next try state = %q, want claimed", claimed.State)
	}
	if err := fixture.store.Complete(fixture.ctx, journal.Completion{
		RunID: fixture.manifest.value.RunID, EffectID: nextID, Token: claimed.CurrentClaim,
		State: journal.Succeeded, Result: []byte("{}"),
		EventKind: "dispatch_completed", EventBody: []byte("{}"), At: fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	status, err = fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, effect := range status.Effects {
		if effect.HostCheckFailure != nil {
			t.Fatalf("stale fact remains on %s after passing candidate: %#v", effect.ID, effect.HostCheckFailure)
		}
	}
	for _, pinned := range status.PinnedWork {
		if pinned.HostCheckFailure != nil {
			t.Fatalf("stale pinned fact remains: %#v", pinned)
		}
	}
}

func isHostCheckFailure(err error, failure **hostCheckFailure) bool {
	if err == nil {
		return false
	}
	type causer interface{ Unwrap() error }
	for current := err; current != nil; {
		if candidate, ok := current.(*hostCheckFailure); ok {
			*failure = candidate
			return true
		}
		unwrapped, ok := current.(causer)
		if !ok {
			return false
		}
		current = unwrapped.Unwrap()
	}
	return false
}
