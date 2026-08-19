package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

// A2: Test that fresh_rehydrate fallback event bodies name the reason
func TestFreshRehydrateFallbackEventBodyNamesReason(t *testing.T) {
	t.Parallel()

	t.Run("ledger_site_label", func(t *testing.T) {
		t.Parallel()
		const siteLabel = "continuation.ledger.step_budget_exhausted"

		continuationDriver := &testContinuationDriver{
			resumeFn: func(ctx context.Context, invocation driver.Invocation, binding driver.ContinuationBinding, cont *driver.Continuation) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
				return driver.Observation{}, nil, driver.ContinuationResult{
					Mode:   driver.ContinuationModeFreshRehydrate,
					Status: driver.ContinuationStatusMismatch,
					Reason: siteLabel,
				}, nil
			},
		}

		fixture := newProductionImplementationRecoveryFixture(t, continuationDriver)
		prepared, err := fixture.service.prepareDriverDispatch(
			fixture.ctx, fixture.engine, fixture.workspace,
			driver.RoleImplementer, fixture.coordinates, fixture.cycle.Before,
		)
		if err != nil {
			t.Fatal(err)
		}

		binding, selDigest, _ := continuationBindingForDispatch(prepared, fixture.coordinates)
		dummyCont := &driver.Continuation{}
		designOID := ""
		if prepared.productionContext.DesignReceipt != nil {
			designOID = prepared.productionContext.DesignReceipt.OID
		}
		fixture.service.storeContinuation(fixture.manifest.value.RunID, fixture.coordinates.Slice,
			&retainedContinuation{
				handle: dummyCont, binding: binding, selectionDigest: selDigest,
				before: fixture.cycle.Before, designReceipt: designOID,
			})

		_, _, _ = fixture.service.runProductionImplementationDispatch(
			fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, fixture.cycle, fixture.coordinates,
		)

		snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, event := range snapshot.Events {
			if strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback") {
				found = true
				if string(event.Body) != siteLabel {
					t.Fatalf("event body = %q, want %q (kind: %s)", string(event.Body), siteLabel, event.Kind)
				}
			}
		}
		if !found {
			t.Fatalf("expected fallback event in snapshot, events = %#v", snapshot.Events)
		}
	})

	t.Run("expiry", func(t *testing.T) {
		t.Parallel()

		continuationDriver := &testContinuationDriver{
			resumeFn: func(ctx context.Context, invocation driver.Invocation, binding driver.ContinuationBinding, cont *driver.Continuation) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
				return driver.Observation{}, nil, driver.ContinuationResult{
					Mode:   driver.ContinuationModeFreshRehydrate,
					Status: driver.ContinuationStatusExpired,
					Reason: "expiry",
				}, nil
			},
		}

		fixture := newProductionImplementationRecoveryFixture(t, continuationDriver)
		prepared, err := fixture.service.prepareDriverDispatch(
			fixture.ctx, fixture.engine, fixture.workspace,
			driver.RoleImplementer, fixture.coordinates, fixture.cycle.Before,
		)
		if err != nil {
			t.Fatal(err)
		}

		binding, selDigest, _ := continuationBindingForDispatch(prepared, fixture.coordinates)
		dummyCont := &driver.Continuation{}
		designOID := ""
		if prepared.productionContext.DesignReceipt != nil {
			designOID = prepared.productionContext.DesignReceipt.OID
		}
		fixture.service.storeContinuation(fixture.manifest.value.RunID, fixture.coordinates.Slice,
			&retainedContinuation{
				handle: dummyCont, binding: binding, selectionDigest: selDigest,
				before: fixture.cycle.Before, designReceipt: designOID,
			})

		_, _, _ = fixture.service.runProductionImplementationDispatch(
			fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, fixture.cycle, fixture.coordinates,
		)

		snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, event := range snapshot.Events {
			if strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback_expired") ||
				strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback") {
				found = true
				if string(event.Body) != "expiry" {
					t.Fatalf("event body = %q, want 'expiry' (kind: %s)", string(event.Body), event.Kind)
				}
			}
		}
		if !found {
			t.Fatalf("expected fallback_expired event in snapshot, events = %#v", snapshot.Events)
		}
	})

	t.Run("absence", func(t *testing.T) {
		t.Parallel()

		dispatcher := fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
			return productionImplementationObservation(t, invocation), nil
		})
		fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

		_, _, _ = fixture.service.runProductionImplementationDispatch(
			fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, fixture.cycle, fixture.coordinates,
		)

		snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, event := range snapshot.Events {
			if strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback") {
				found = true
				if string(event.Body) != "absence" {
					t.Fatalf("event body = %q, want 'absence' (kind: %s)", string(event.Body), event.Kind)
				}
			}
		}
		if !found {
			t.Fatalf("expected absence fallback event in snapshot, events = %#v", snapshot.Events)
		}
	})
}

// A3: Test degradation budget exceeded parks the run at a safe boundary
func TestDegradationBudgetExceededParksRunAtSafeBoundary(t *testing.T) {
	t.Parallel()

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

	// Budget is default 3. Record 3 fallback events -> should NOT park.
	for i := 1; i <= 3; i++ {
		if err := store.AppendEvent(ctx, manifest.value.RunID,
			"dispatch_completed.continuation.fresh_rehydrate.fallback",
			[]byte("absence"), now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	status, err := service.Status(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State == "parked" {
		t.Fatalf("expected state != parked with 3 fallbacks (budget=3), got: %s", status.State)
	}

	// Record 4th fallback event -> exceeds default budget of 3 -> MUST park.
	if err := store.AppendEvent(ctx, manifest.value.RunID,
		"dispatch_completed.continuation.fresh_rehydrate.fallback",
		[]byte("continuation.ledger.step_budget_exhausted"), now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}

	// Verify reducer policy in Status()
	status, err = service.Status(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "parked" {
		t.Fatalf("expected state = parked with 4 fallbacks (budget=3), got: %s", status.State)
	}

	// Verify driveLoop halts and records degradation_budget_parked event
	driveErr := service.driveLoop(ctx, engine, owner, false)
	if driveErr != nil {
		t.Fatalf("driveLoop returned error: %v", driveErr)
	}

	snapshot, err := store.Snapshot(ctx, manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}

	var parkEvent *journal.Event
	for i := range snapshot.Events {
		if snapshot.Events[i].Kind == "degradation_budget_parked" {
			parkEvent = &snapshot.Events[i]
			break
		}
	}
	if parkEvent == nil {
		t.Fatal("expected degradation_budget_parked event in journal")
	}

	var recordedFallbacks []degradationFallback
	if err := json.Unmarshal(parkEvent.Body, &recordedFallbacks); err != nil {
		t.Fatalf("cannot unmarshal degradation_budget_parked body: %v (raw: %s)", err, string(parkEvent.Body))
	}
	if len(recordedFallbacks) != 4 {
		t.Fatalf("recorded fallbacks count = %d, want 4", len(recordedFallbacks))
	}
	if recordedFallbacks[3].Reason != "continuation.ledger.step_budget_exhausted" {
		t.Fatalf("recorded fallbacks[3].Reason = %q, want 'continuation.ledger.step_budget_exhausted'", recordedFallbacks[3].Reason)
	}
}

// A4: Test manifest degradation budget admission, round-trip, and defaults
func TestManifestDegradationBudgetAdmissionAndRoundTrip(t *testing.T) {
	t.Parallel()

	baseManifest := Manifest{
		SchemaVersion: ManifestVersion,
		RunID:         "run-custom-budget",
		Repository:    "/tmp/repo",
		Release:       "rel-custom",
		TargetRef:     "refs/heads/main",
		GitIdentity: gitx.Identity{
			Name:  "sworn",
			Email: "sworn@example.com",
		},
		Intent:            "test custom degradation budget",
		MaxParallelTracks: 1,
		Authority: ProjectAuthority{
			Project:            "project",
			ExternalAuthorizer: "authorizer",
		},
		DriverConfigDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "default", Model: "model-p"},
			Implementer: driver.RoleSelection{Profile: "default", Model: "model-i"},
			Captain:     driver.RoleSelection{Profile: "default", Model: "model-c"},
			Verifier:    driver.RoleSelection{Profile: "default", Model: "model-v"},
		},
		Automation: &AutomationSelections{
			Recovery: driver.ModelSelection{Profile: "default", Model: "model-r"},
		},
		Limits: driver.Limits{
			TimeoutMillis: 60000,
			OutputBytes:   1048576,
		},
	}

	// 1. Existing manifest without degradation_budget admits cleanly with default (3)
	manifestNoField, err := canonicalManifest(baseManifest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseManifest(manifestNoField)
	if err != nil {
		t.Fatalf("ParseManifest failed on manifest without degradation_budget: %v", err)
	}
	if parsed.EffectiveDegradationBudget() != 3 {
		t.Fatalf("EffectiveDegradationBudget = %d, want 3", parsed.EffectiveDegradationBudget())
	}
	if parsed.Limits.DegradationBudget != 0 {
		t.Fatalf("Limits.DegradationBudget = %d, want 0", parsed.Limits.DegradationBudget)
	}

	// 2. Manifest declaring degradation_budget round-trips canonically
	baseWithBudget := baseManifest
	baseWithBudget.Limits.DegradationBudget = 5
	manifestWithBudget, err := canonicalManifest(baseWithBudget)
	if err != nil {
		t.Fatal(err)
	}
	parsed5, err := ParseManifest(manifestWithBudget)
	if err != nil {
		t.Fatalf("ParseManifest failed on manifest with degradation_budget: %v", err)
	}
	if parsed5.EffectiveDegradationBudget() != 5 {
		t.Fatalf("EffectiveDegradationBudget = %d, want 5", parsed5.EffectiveDegradationBudget())
	}
	if parsed5.Limits.DegradationBudget != 5 {
		t.Fatalf("Limits.DegradationBudget = %d, want 5", parsed5.Limits.DegradationBudget)
	}

	// 3. Manifest with explicit "degradation_budget": 0 fails canonical round-trip
	manifestZeroBudget := bytes.Replace(manifestWithBudget, []byte(`"degradation_budget":5`), []byte(`"degradation_budget":0`), 1)
	if _, err := ParseManifest(manifestZeroBudget); err == nil {
		t.Fatal("expected failure for manifest with explicit degradation_budget: 0")
	}

	// 4. Manifest with negative or >100 degradation_budget is rejected with INVALID_LIMITS
	manifestNegative := bytes.Replace(manifestWithBudget, []byte(`"degradation_budget":5`), []byte(`"degradation_budget":-1`), 1)
	if _, err := ParseManifest(manifestNegative); !IsCode(err, "INVALID_LIMITS") {
		t.Fatalf("expected INVALID_LIMITS for negative degradation_budget, got: %v", err)
	}

	manifestTooHigh := bytes.Replace(manifestWithBudget, []byte(`"degradation_budget":5`), []byte(`"degradation_budget":101`), 1)
	if _, err := ParseManifest(manifestTooHigh); !IsCode(err, "INVALID_LIMITS") {
		t.Fatalf("expected INVALID_LIMITS for degradation_budget > 100, got: %v", err)
	}
}

type testContinuationDriver struct {
	resumeFn func(ctx context.Context, invocation driver.Invocation, binding driver.ContinuationBinding, cont *driver.Continuation) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error)
}

func (d *testContinuationDriver) Invoke(ctx context.Context, invocation driver.Invocation) (driver.Observation, error) {
	return driver.Observation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
	}, nil
}

func (d *testContinuationDriver) InvokeTurn(ctx context.Context, invocation driver.Invocation, binding driver.ContinuationBinding, cont *driver.Continuation) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
	if cont != nil && d.resumeFn != nil {
		return d.resumeFn(ctx, invocation, binding, cont)
	}
	obs, err := d.Invoke(ctx, invocation)
	return obs, nil, driver.ContinuationResult{
		Mode:   driver.ContinuationModeFreshRehydrate,
		Status: driver.ContinuationStatusCompleted,
	}, err
}

func (d *testContinuationDriver) StartContinuation(ctx context.Context, invocation driver.Invocation, binding driver.ContinuationBinding) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
	obs, err := d.Invoke(ctx, invocation)
	return obs, &driver.Continuation{}, driver.ContinuationResult{
		Mode:   driver.ContinuationModeTranscriptReplay,
		Status: driver.ContinuationStatusSuspended,
	}, err
}

func (d *testContinuationDriver) ResumeContinuation(ctx context.Context, invocation driver.Invocation, binding driver.ContinuationBinding, cont *driver.Continuation) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
	if d.resumeFn != nil {
		return d.resumeFn(ctx, invocation, binding, cont)
	}
	obs, err := d.Invoke(ctx, invocation)
	return obs, nil, driver.ContinuationResult{
		Mode:   driver.ContinuationModeTranscriptReplay,
		Status: driver.ContinuationStatusCompleted,
	}, err
}
