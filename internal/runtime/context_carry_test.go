package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func TestRetryDispatchCarriesPriorSubmissionSummaryAndDetail(t *testing.T) {
	t.Parallel()

	const (
		expectedSummary = "Implementation try 1 candidate built"
		expectedDetail  = "Fixed all compilation errors and added regression tests."
	)

	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		obs := productionImplementationObservation(t, invocation)
		sub := driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocation.Request.InvocationID,
			Responsibility: driver.ImplementerImplementation,
			Summary:        expectedSummary,
			Detail:         expectedDetail,
			Checks: &driver.ExactBytes{
				ByteCount: 5,
				Digest:    driver.Digest([]byte("check")),
				Bytes:     "Y2hlY2s=",
			},
		}
		subBytes, err := driver.EncodeSubmission(sub)
		if err != nil {
			t.Fatal(err)
		}
		obs.Handoff.SubmissionBytes = subBytes
		obs.Handoff.SubmissionDigest = driver.Digest(subBytes)
		return obs, nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	// Dispatch Try 1
	attempt := journal.EffectAttempt{
		WorkID: fixture.cycle.DispatchWork,
		Epoch:  fixture.coordinates.Epoch,
		Try:    1,
	}
	sub, err := fixture.service.runDriverEffectWithPreparation(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		driver.RoleImplementer,
		fixture.coordinates,
		attempt,
		fixture.cycle.Before,
		fixture.owner,
		nil,
	)
	if err != nil {
		t.Fatalf("runDriverEffectWithPreparation failed: %v", err)
	}
	if sub.Summary != expectedSummary || sub.Detail != expectedDetail {
		t.Fatalf("unexpected submission: %#v", sub)
	}

	// Capture Try 2 work context
	retryCoords := fixture.coordinates
	retryCoords.Try = 2

	retryWorkContext, retryBytes, err := captureProductionWorkContext(
		fixture.ctx,
		fixture.engine,
		retryCoords,
		fixture.cycle.Before,
		driver.ReadWrite,
	)
	if err != nil {
		t.Fatalf("captureProductionWorkContext failed: %v", err)
	}

	// Assert PriorSubmission is carried in workContext
	if retryWorkContext.PriorSubmission == nil {
		t.Fatal("expected non-nil PriorSubmission on Try 2 dispatch")
	}
	if retryWorkContext.PriorSubmission.Summary != expectedSummary {
		t.Fatalf("PriorSubmission.Summary = %q, want %q", retryWorkContext.PriorSubmission.Summary, expectedSummary)
	}
	if retryWorkContext.PriorSubmission.Detail != expectedDetail {
		t.Fatalf("PriorSubmission.Detail = %q, want %q", retryWorkContext.PriorSubmission.Detail, expectedDetail)
	}
	if retryWorkContext.PriorSubmission.Provenance != "try 1" {
		t.Fatalf("PriorSubmission.Provenance = %q, want %q", retryWorkContext.PriorSubmission.Provenance, "try 1")
	}

	// Verify decoded JSON from prepared dispatch input bytes
	var decodedContext productionWorkContext
	if err := json.Unmarshal(retryBytes, &decodedContext); err != nil {
		t.Fatalf("cannot unmarshal work-context.json bytes: %v", err)
	}
	if decodedContext.PriorSubmission == nil {
		t.Fatal("expected PriorSubmission in serialized work-context.json")
	}
	if decodedContext.PriorSubmission.Summary != expectedSummary ||
		decodedContext.PriorSubmission.Detail != expectedDetail ||
		decodedContext.PriorSubmission.Provenance != "try 1" {
		t.Fatalf("serialized PriorSubmission mismatch: %#v", decodedContext.PriorSubmission)
	}
}

func TestRepairDispatchCarriesPriorAttemptSubmissionSummaryAndDetail(t *testing.T) {
	t.Parallel()

	const (
		expectedSummary = "Attempt 1 implementation summary"
		expectedDetail  = "Attempt 1 implementation detail."
	)

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

	dispatcher := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		return driver.Observation{}, nil
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

	planBytes, _ := runtimePlan(t, manifest.value.Release, manifest.value.Authority.Project, manifest.value.TargetRef, "approval-release-1-v1")
	if _, err := engine.actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install exact plan",
		Detail:    []byte("detail"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "implementer", Result: "designed",
		Summary: "designed S1", Detail: []byte("detail"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "captain", Result: "proceed",
		Summary: "approved", Detail: []byte("detail"),
	}); err != nil {
		t.Fatal(err)
	}

	state, err := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice1, ok := state.Slice("S1")
	if !ok {
		t.Fatal("slice S1 not found")
	}
	beforeAttempt1 := sliceFingerprint(state, "S1")

	// Simulate Attempt 1 driver dispatch completed with submission
	attempt1Coords := dispatchCoordinates{
		Slice:          "S1",
		Responsibility: driver.ImplementerImplementation,
		BatonAttempt:   1,
		Epoch:          1,
		Try:            1,
	}
	attempt1Work := driverWorkIdentity(
		manifest.digest,
		attempt1Coords.Slice,
		attempt1Coords.Responsibility,
		1,
		beforeAttempt1,
	)
	attempt1EffectID := journal.AttemptEffectID(attempt1Work, 1, 1)

	sub := driver.Submission{
		SchemaVersion:  driver.SubmissionSchemaVersion,
		InvocationID:   dispatchInvocationID(manifest.value.RunID, attempt1Coords),
		Responsibility: driver.ImplementerImplementation,
		Summary:        expectedSummary,
		Detail:         expectedDetail,
		Checks: &driver.ExactBytes{
			ByteCount: 5,
			Digest:    driver.Digest([]byte("check")),
			Bytes:     "Y2hlY2s=",
		},
	}
	subBytes, err := driver.EncodeSubmission(sub)
	if err != nil {
		t.Fatal(err)
	}

	dispatchCmd := productionDispatchCommand{
		SchemaVersion: productionDispatchVersion,
		RequestDigest: sha256Digest([]byte("req")),
		Context: productionWorkContext{
			SchemaVersion:      productionWorkContextVersion,
			ManifestDigest:     manifest.digest,
			DriverConfigDigest: manifest.value.DriverConfigDigest,
			RunID:              manifest.value.RunID,
			Repository:         manifest.value.Authority.Project,
			Release:            manifest.value.Release,
			Intent:             manifest.value.Intent,
			InvocationID:       dispatchInvocationID(manifest.value.RunID, attempt1Coords),
			Role:               driver.RoleImplementer,
			Track:              slice1.Location.Track.ID,
			Slice:              "S1",
			Responsibility:     driver.ImplementerImplementation,
			Attempt:            1,
			Epoch:              1,
			Try:                1,
			Before:             beforeAttempt1,
			WorkspaceAccess:    driver.ReadWrite,
			Authority: productionAuthorityBinding{
				ReleaseRef: "refs/heads/release-wt/" + manifest.value.Release,
				TargetRef:  manifest.value.TargetRef,
				TargetHead: state.Refs.Target.Head,
				TrackRef:   "refs/heads/track/" + manifest.value.Release + "/" + slice1.Location.Track.ID,
				TrackHead:  state.Refs.Release.Head,
			},
		},
	}
	cmdBytes, _ := json.Marshal(dispatchCmd)

	if err := store.EnsureAttempt(ctx,
		journal.Command{RunID: manifest.value.RunID, ReplayKey: attempt1EffectID, Kind: "driver.dispatch", Payload: cmdBytes, CreatedAt: now},
		journal.Effect{RunID: manifest.value.RunID, ID: attempt1EffectID, ReplayKey: attempt1EffectID, Kind: "driver.dispatch", BeforeDigest: sha256Digest([]byte(beforeAttempt1)), ExpectedDigest: sha256Digest([]byte("expected")), State: journal.Pending, UpdatedAt: now},
		journal.EffectAttempt{WorkID: attempt1Work, Epoch: 1, Try: 1},
	); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimOwned(ctx, owner, attempt1EffectID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteOwned(ctx, owner, journal.Completion{
		RunID: manifest.value.RunID, EffectID: attempt1EffectID, Token: claim.Token,
		State: journal.Succeeded, Result: subBytes, Attempt: &journal.Attempt{Number: 1, Responsibility: string(driver.ImplementerImplementation), TransportStatus: string(driver.Completed), ObservationDigest: sha256Digest([]byte("obs")), HandoffDigest: driver.Digest(subBytes), Usage: []byte("{}")},
		Receipts:  []journal.Receipt{{Kind: "sealed_driver_handoff", Body: subBytes}},
		EventKind: "dispatch_completed", EventBody: []byte(driver.ImplementerImplementation), At: now,
	}); err != nil {
		t.Fatal(err)
	}

	slice1, ok = state.Slice("S1")
	if !ok {
		t.Fatal("slice S1 not found")
	}
	state, slice1, err = service.prepareTrackBaseForSlice(ctx, engine, owner, state, slice1)
	if err != nil {
		t.Fatal(err)
	}

	workspace, err := engine.workspaces.OpenTrack(
		gitx.TrackKey{
			Release: manifest.value.Release,
			Track:   slice1.Location.Track.ID,
		},
		gitx.ImplementationView,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	if err := os.WriteFile(
		filepath.Join(workspace.Path(), "s1_candidate.txt"),
		[]byte("content\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	candidate, err := engine.workspaces.SealTrack(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "implementer", Result: "candidate",
		Candidate: candidate.Candidate.String(), CheckResults: []byte("checks\n"),
		Summary: "cand", Detail: []byte("detail"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.AppendReceipt(baton.AppendReceiptInput{
		Release: manifest.value.Release, Slice: "S1", Role: "verifier", Result: "fail",
		Candidate: candidate.Candidate.String(), CheckResults: []byte("fail checks\n"),
		Summary: "failed check", Detail: []byte("detail"),
	}); err != nil {
		t.Fatal(err)
	}

	state2, err := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice2, ok := state2.Slice("S1")
	if !ok || slice2.Attempt != 2 {
		t.Fatalf("expected slice attempt 2, got %d", slice2.Attempt)
	}
	beforeAttempt2 := sliceFingerprint(state2, "S1")

	// Capture Attempt 2 Try 1 work context
	attempt2Coords := dispatchCoordinates{
		Slice:          "S1",
		Responsibility: driver.ImplementerImplementation,
		BatonAttempt:   2,
		Epoch:          1,
		Try:            1,
	}
	repairWorkContext, repairBytes, err := captureProductionWorkContext(
		ctx,
		engine,
		attempt2Coords,
		beforeAttempt2,
		driver.ReadWrite,
	)
	if err != nil {
		t.Fatalf("captureProductionWorkContext for repair attempt failed: %v", err)
	}

	if repairWorkContext.PriorSubmission == nil {
		t.Fatal("expected non-nil PriorSubmission on repair dispatch (Attempt 2)")
	}
	if repairWorkContext.PriorSubmission.Summary != expectedSummary {
		t.Fatalf("PriorSubmission.Summary = %q, want %q", repairWorkContext.PriorSubmission.Summary, expectedSummary)
	}
	if repairWorkContext.PriorSubmission.Detail != expectedDetail {
		t.Fatalf("PriorSubmission.Detail = %q, want %q", repairWorkContext.PriorSubmission.Detail, expectedDetail)
	}
	if repairWorkContext.PriorSubmission.Provenance != "attempt 1, try 1" {
		t.Fatalf("PriorSubmission.Provenance = %q, want %q", repairWorkContext.PriorSubmission.Provenance, "attempt 1, try 1")
	}

	var decodedContext productionWorkContext
	if err := json.Unmarshal(repairBytes, &decodedContext); err != nil {
		t.Fatalf("cannot unmarshal work-context.json: %v", err)
	}
	if decodedContext.PriorSubmission == nil ||
		decodedContext.PriorSubmission.Summary != expectedSummary ||
		decodedContext.PriorSubmission.Provenance != "attempt 1, try 1" {
		t.Fatalf("serialized repair context PriorSubmission mismatch: %#v", decodedContext.PriorSubmission)
	}
}

func TestNoPriorSubmissionWhenPredecessorDidNotSubmit(t *testing.T) {
	t.Parallel()

	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return driver.Observation{}, &driver.ContractError{
			Code:   "CONTINUATION_INVALID",
			Detail: "continuation.ledger.step_budget_exhausted",
		}
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	// Run Try 1 which fails before submitting
	_, _, _ = fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	)

	// Capture Try 2 work context
	retryCoords := fixture.coordinates
	retryCoords.Try = 2

	retryWorkContext, _, err := captureProductionWorkContext(
		fixture.ctx,
		fixture.engine,
		retryCoords,
		fixture.cycle.Before,
		driver.ReadWrite,
	)
	if err != nil {
		t.Fatalf("captureProductionWorkContext failed: %v", err)
	}
	if retryWorkContext.PriorSubmission != nil {
		t.Fatalf("expected nil PriorSubmission when predecessor did not submit, got: %#v", retryWorkContext.PriorSubmission)
	}
}

func TestPriorSubmissionValidation(t *testing.T) {
	t.Parallel()

	dispatcher := fixtureDriver(func(_ context.Context, _ driver.Invocation) (driver.Observation, error) {
		return driver.Observation{}, nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	baseContext, _, err := captureProductionWorkContext(
		fixture.ctx,
		fixture.engine,
		fixture.coordinates,
		fixture.cycle.Before,
		driver.ReadWrite,
	)
	if err != nil {
		t.Fatalf("captureProductionWorkContext failed: %v", err)
	}

	// 1. Try 1 Attempt 1 with PriorSubmission is rejected
	try1WithPrior := baseContext
	try1WithPrior.PriorSubmission = &productionPriorSubmissionBinding{
		Summary:    "valid summary",
		Detail:     "valid detail",
		Provenance: "try 1",
	}
	if err := validateProductionWorkContext(fixture.manifest, try1WithPrior); err == nil {
		t.Fatal("expected validation failure for PriorSubmission on Try 1 Attempt 1")
	}

	// 2. Try 2 with valid PriorSubmission passes validation
	try2WithPrior := baseContext
	try2WithPrior.Try = 2
	try2WithPrior.InvocationID = dispatchInvocationID(fixture.manifest.value.RunID, dispatchCoordinates{
		Slice:          baseContext.Slice,
		Responsibility: baseContext.Responsibility,
		BatonAttempt:   baseContext.Attempt,
		Epoch:          baseContext.Epoch,
		Try:            2,
	})
	try2WithPrior.PriorSubmission = &productionPriorSubmissionBinding{
		Summary:    "valid summary",
		Detail:     "valid detail",
		Provenance: "try 1",
	}
	if err := validateProductionWorkContext(fixture.manifest, try2WithPrior); err != nil {
		t.Fatalf("expected valid Try 2 PriorSubmission to pass, got: %v", err)
	}

	// 3. Invalid Summary (empty) is rejected
	invalidSummary := try2WithPrior
	invalidSummary.PriorSubmission = &productionPriorSubmissionBinding{
		Summary:    "   ",
		Detail:     "valid detail",
		Provenance: "try 1",
	}
	if err := validateProductionWorkContext(fixture.manifest, invalidSummary); err == nil {
		t.Fatal("expected validation failure for empty summary")
	}

	// 4. Invalid Detail (containing Baton marker) is rejected
	invalidDetail := try2WithPrior
	invalidDetail.PriorSubmission = &productionPriorSubmissionBinding{
		Summary:    "valid summary",
		Detail:     "contains Baton-Detail-Begin marker",
		Provenance: "try 1",
	}
	if err := validateProductionWorkContext(fixture.manifest, invalidDetail); err == nil {
		t.Fatal("expected validation failure for detail with Baton marker")
	}

	// 5. Invalid Provenance (empty) is rejected
	invalidProv := try2WithPrior
	invalidProv.PriorSubmission = &productionPriorSubmissionBinding{
		Summary:    "valid summary",
		Detail:     "valid detail",
		Provenance: "",
	}
	if err := validateProductionWorkContext(fixture.manifest, invalidProv); err == nil {
		t.Fatal("expected validation failure for empty provenance")
	}

	// 6. productionWorkContextV1 clears PriorSubmission and rejects V1 with PriorSubmission
	v1, err := productionWorkContextV1(fixture.manifest, try2WithPrior)
	if err != nil {
		t.Fatalf("productionWorkContextV1 failed: %v", err)
	}
	if v1.PriorSubmission != nil {
		t.Fatalf("expected V1 PriorSubmission to be nil, got: %#v", v1.PriorSubmission)
	}
	v1.PriorSubmission = try2WithPrior.PriorSubmission
	if err := validateProductionWorkContext(fixture.manifest, v1); err == nil {
		t.Fatal("expected validation failure for V1 schema with PriorSubmission")
	}
}
