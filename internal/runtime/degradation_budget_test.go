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

// A2: Test that fresh_rehydrate fallback event bodies carry the reason
// additively beside the association fields, the retained fact, and the
// declared posture.
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
			if !strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback") {
				continue
			}
			found = true
			var body continuationFallbackEvent
			if err := json.Unmarshal(event.Body, &body); err != nil {
				t.Fatalf("event body is not structured: %v (raw: %s)", err, string(event.Body))
			}
			if body.SchemaVersion != continuationFallbackEventVersion {
				t.Fatalf("schema_version = %q, want %q", body.SchemaVersion, continuationFallbackEventVersion)
			}
			if body.Reason != siteLabel {
				t.Fatalf("reason = %q, want %q", body.Reason, siteLabel)
			}
			if !body.Retained {
				t.Fatalf("retained = false, want true: a stored handle was presented and rejected")
			}
			if body.Posture != string(driver.ContinuationPostureContextRetaining) {
				t.Fatalf("posture = %q, want %q", body.Posture, driver.ContinuationPostureContextRetaining)
			}
			// A2: the association fields ride alongside the reason.
			if body.EffectID == "" || body.WorkID == "" {
				t.Fatalf("fallback event lost its association: %#v", body)
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
			if !strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback_expired") &&
				!strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback") {
				continue
			}
			found = true
			var body continuationFallbackEvent
			if err := json.Unmarshal(event.Body, &body); err != nil {
				t.Fatalf("event body is not structured: %v (raw: %s)", err, string(event.Body))
			}
			if body.Reason != "expiry" {
				t.Fatalf("reason = %q, want 'expiry'", body.Reason)
			}
			if !body.Retained {
				t.Fatalf("retained = false, want true: a stored handle was presented and expired")
			}
			if body.Posture != string(driver.ContinuationPostureContextRetaining) {
				t.Fatalf("posture = %q, want %q", body.Posture, driver.ContinuationPostureContextRetaining)
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
			if !strings.Contains(event.Kind, ".continuation.fresh_rehydrate.fallback") {
				continue
			}
			found = true
			var body continuationFallbackEvent
			if err := json.Unmarshal(event.Body, &body); err != nil {
				t.Fatalf("event body is not structured: %v (raw: %s)", err, string(event.Body))
			}
			if body.Reason != "absence" {
				t.Fatalf("reason = %q, want 'absence'", body.Reason)
			}
			// Correction 2: engine-side non-reuse (no stored handle was
			// presented to the adapter) is retained=false.
			if body.Retained {
				t.Fatalf("retained = true, want false: no stored handle was presented to the adapter")
			}
			if body.Posture != string(driver.ContinuationPostureContextRetaining) {
				t.Fatalf("posture = %q, want %q", body.Posture, driver.ContinuationPostureContextRetaining)
			}
		}
		if !found {
			t.Fatalf("expected absence fallback event in snapshot, events = %#v", snapshot.Events)
		}
	})
}

// A1: the counting gate reads the structured facts: a fallback counts only
// when the dispatch actually had a retained continuation to lose and the
// adapter declares context retention.
func TestDegradationFallbacksGatedOnRetainedAndPosture(t *testing.T) {
	t.Parallel()

	structured := func(retained bool, posture string) []byte {
		body, err := json.Marshal(continuationFallbackEvent{
			SchemaVersion: continuationFallbackEventVersion,
			EventAssociation: EventAssociation{
				EffectID: "attempt/" + strings.Repeat("a", 64) + "/e1/t1",
				WorkID:   "sha256:" + strings.Repeat("a", 64),
				Slice:    "S1",
			},
			Reason:   "absence",
			Retained: retained,
			Posture:  posture,
		})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}

	snapshot := func(bodies ...[]byte) journal.Snapshot {
		events := make([]journal.Event, 0, len(bodies))
		for index, body := range bodies {
			events = append(events, journal.Event{
				Offset: int64(index + 1),
				Kind:   "dispatch_completed.continuation.fresh_rehydrate.fallback",
				Body:   body,
			})
		}
		return journal.Snapshot{Events: events}
	}

	for _, test := range []struct {
		name     string
		snapshot journal.Snapshot
		want     int
	}{
		{
			name: "retained context_retaining counts",
			snapshot: snapshot(
				structured(true, "context_retaining"),
				structured(true, "context_retaining"),
				structured(true, "context_retaining"),
				structured(true, "context_retaining"),
			),
			want: 4,
		},
		{
			name: "engine-side non-reuse does not count",
			snapshot: snapshot(
				structured(false, "context_retaining"),
				structured(false, "context_retaining"),
				structured(false, "context_retaining"),
				structured(false, "context_retaining"),
			),
			want: 0,
		},
		{
			name: "fresh_by_design churn does not count",
			snapshot: snapshot(
				structured(true, "fresh_by_design"),
				structured(true, "fresh_by_design"),
				structured(true, "fresh_by_design"),
				structured(true, "fresh_by_design"),
			),
			want: 0,
		},
		{
			name: "legacy bare-string bodies keep counting",
			snapshot: snapshot(
				[]byte("absence"),
				[]byte("continuation.ledger.step_budget_exhausted"),
				[]byte("expiry"),
				[]byte("absence"),
			),
			want: 4,
		},
		{
			name: "unknown posture fails closed as counting",
			snapshot: snapshot(
				structured(true, ""),
				structured(true, ""),
			),
			want: 2,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := degradationFallbacks(test.snapshot); len(got) != test.want {
				t.Fatalf("degradationFallbacks = %d items (%#v), want %d", len(got), got, test.want)
			}
		})
	}
}

// Correction 2: a structured retained=false fact is engine-side non-reuse,
// including the entry-present-but-mismatched case (the design receipt,
// binding, or selection digest changed). It is journaled and not counted.
func TestEntryPresentButMismatchedDispatchIsNotDegradation(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(continuationFallbackEvent{
		SchemaVersion: continuationFallbackEventVersion,
		EventAssociation: EventAssociation{
			EffectID: "attempt/" + strings.Repeat("b", 64) + "/e1/t1",
			WorkID:   "sha256:" + strings.Repeat("b", 64),
		},
		Reason:   "absence",
		Retained: false,
		Posture:  string(driver.ContinuationPostureContextRetaining),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := journal.Snapshot{Events: []journal.Event{{
		Offset: 1,
		Kind:   "dispatch_completed.continuation.fresh_rehydrate.fallback",
		Body:   body,
	}}}
	if got := degradationFallbacks(snapshot); len(got) != 0 {
		t.Fatalf("entry-present-but-mismatched dispatch counted: %#v", got)
	}
}

type degradationStatusFixture struct {
	ctx      context.Context
	manifest admittedManifest
	store    *journal.Store
	owner    journal.OwnerLease
	now      time.Time
	service  *Service
	engine   *engine
}

func newDegradationStatusFixture(t *testing.T) *degradationStatusFixture {
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
	return &degradationStatusFixture{
		ctx: ctx, manifest: manifest, store: store, owner: owner,
		now: now, service: service, engine: engine,
	}
}

func (f *degradationStatusFixture) appendStructuredFallback(
	t *testing.T,
	retained bool,
	posture string,
	index int,
) {
	t.Helper()
	body, err := json.Marshal(continuationFallbackEvent{
		SchemaVersion: continuationFallbackEventVersion,
		EventAssociation: EventAssociation{
			EffectID: "attempt/" + strings.Repeat("c", 64) + "/e1/t1",
			WorkID:   "sha256:" + strings.Repeat("c", 64),
		},
		Reason:   "absence",
		Retained: retained,
		Posture:  posture,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.AppendEvent(f.ctx, f.manifest.value.RunID,
		"dispatch_completed.continuation.fresh_rehydrate.fallback",
		body, f.now.Add(time.Duration(index)*time.Second)); err != nil {
		t.Fatal(err)
	}
}

// A1: the status count gate reads the journaled facts. An adapter whose
// declared posture makes fresh rehydration its ordinary operation, and a
// dispatch with nothing retained, accumulate zero budget.
func TestStatusParkCountsOnlyRealLoss(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		retained bool
		posture  string
		park     bool
	}{
		{
			name:     "context_retaining retained loss parks",
			retained: true,
			posture:  string(driver.ContinuationPostureContextRetaining),
			park:     true,
		},
		{
			name:     "fresh_by_design churn never parks",
			retained: true,
			posture:  string(driver.ContinuationPostureFreshByDesign),
			park:     false,
		},
		{
			name:     "engine-side non-reuse never parks",
			retained: false,
			posture:  string(driver.ContinuationPostureContextRetaining),
			park:     false,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newDegradationStatusFixture(t)
			// Default budget is 3; four events cross it when they count.
			for i := 1; i <= 4; i++ {
				fixture.appendStructuredFallback(
					t, test.retained, test.posture, i,
				)
			}
			status, err := fixture.service.Status(
				fixture.ctx,
				fixture.manifest.value.RunID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if status.State == "parked" != test.park {
				t.Fatalf("state = %q, want parked=%t (status = %#v)", status.State, test.park, status)
			}
			if !test.park {
				if status.Park != nil {
					t.Fatalf("non-counting churn produced a park status: %#v", status.Park)
				}
				return
			}
			if status.Park == nil ||
				status.Park.Cause != ParkCauseDegradation ||
				status.Park.FallbackCount != 4 ||
				status.Park.Budget != 3 ||
				status.Park.UnblockKnob != DegradationUnblockKnob {
				t.Fatalf("degradation park = %#v, want cause=%q count=4 budget=3 knob=%q",
					status.Park, ParkCauseDegradation, DegradationUnblockKnob)
			}
		})
	}
}

// A3: the park cause precedence mirrors the final park computation:
// human authority, attention, degradation, economy, identical failure,
// exhaustion.
func TestParkStatusForCausePrecedence(t *testing.T) {
	t.Parallel()

	manifest := admittedManifest{value: Manifest{}}

	human := parkStatusFor(manifest, parkFacts{
		humanAuthorityRequired: true, attentionParked: true,
		degradationBudgetExceeded: true, degradationCount: 7,
		economy:           &economyParkFacts{cause: ParkCauseEconomyTurns, spent: 201, budget: 200, knob: EconomyTurnsUnblockKnob},
		identicalFailure:  &identicalFailureFacts{code: "INVOCATION_TIMEOUT", consecutive: 2, threshold: 2},
		exhaustionApplies: true,
	})
	if human.Cause != ParkCauseHumanAuthority {
		t.Fatalf("human authority precedence = %#v", human)
	}

	attention := parkStatusFor(manifest, parkFacts{
		attentionParked: true, degradationBudgetExceeded: true,
		degradationCount:  7,
		economy:           &economyParkFacts{cause: ParkCauseEconomyTurns, spent: 201, budget: 200, knob: EconomyTurnsUnblockKnob},
		identicalFailure:  &identicalFailureFacts{code: "INVOCATION_TIMEOUT", consecutive: 2, threshold: 2},
		exhaustionApplies: true,
	})
	if attention.Cause != ParkCauseAttention {
		t.Fatalf("attention precedence = %#v", attention)
	}

	degradation := parkStatusFor(manifest, parkFacts{
		degradationBudgetExceeded: true, degradationCount: 7,
		economy:           &economyParkFacts{cause: ParkCauseEconomyTurns, spent: 201, budget: 200, knob: EconomyTurnsUnblockKnob},
		identicalFailure:  &identicalFailureFacts{code: "INVOCATION_TIMEOUT", consecutive: 2, threshold: 2},
		exhaustionApplies: true,
	})
	if degradation.Cause != ParkCauseDegradation ||
		degradation.FallbackCount != 7 ||
		degradation.Budget != 3 ||
		degradation.UnblockKnob != DegradationUnblockKnob {
		t.Fatalf("degradation park = %#v", degradation)
	}

	bootstrap := parkStatusFor(manifest, parkFacts{
		bootstrapAuthority: true, bootstrapReason: "reason text",
		economy:           &economyParkFacts{cause: ParkCauseEconomyTurns, spent: 201, budget: 200, knob: EconomyTurnsUnblockKnob},
		identicalFailure:  &identicalFailureFacts{code: "INVOCATION_TIMEOUT", consecutive: 2, threshold: 2},
		exhaustionApplies: true,
	})
	if bootstrap.Cause != ParkCauseBootstrapAuthority ||
		bootstrap.Reason != "reason text" {
		t.Fatalf("bootstrap authority park = %#v", bootstrap)
	}

	economy := parkStatusFor(manifest, parkFacts{
		economy:           &economyParkFacts{cause: ParkCauseEconomyTurns, spent: 201, budget: 200, knob: EconomyTurnsUnblockKnob},
		identicalFailure:  &identicalFailureFacts{code: "INVOCATION_TIMEOUT", consecutive: 2, threshold: 2},
		exhaustionApplies: true,
	})
	if economy.Cause != ParkCauseEconomyTurns ||
		economy.Spent != 201 ||
		economy.Budget != 200 ||
		economy.UnblockKnob != EconomyTurnsUnblockKnob {
		t.Fatalf("economy park = %#v", economy)
	}

	identical := parkStatusFor(manifest, parkFacts{
		identicalFailure:  &identicalFailureFacts{code: "INVOCATION_TIMEOUT", detail: "gateway timeout", consecutive: 2, threshold: 2},
		exhaustionApplies: true,
	})
	if identical.Cause != ParkCauseIdenticalFailure ||
		identical.Consecutive != 2 ||
		identical.Threshold != 2 ||
		identical.FailureCode != "INVOCATION_TIMEOUT" ||
		identical.FailureDetail != "gateway timeout" ||
		identical.UnblockKnob != IdenticalFailureUnblockKnob {
		t.Fatalf("identical-failure park = %#v", identical)
	}

	exhaustion := parkStatusFor(manifest, parkFacts{exhaustionApplies: true})
	if exhaustion.Cause != ParkCauseExhaustion {
		t.Fatalf("exhaustion park = %#v", exhaustion)
	}
	if exhaustion.FallbackCount != 0 ||
		exhaustion.Budget != 0 ||
		exhaustion.UnblockKnob != "" {
		t.Fatalf("non-degradation park carries degradation facts: %#v", exhaustion)
	}
	if exhaustion.FailureCode != "" || exhaustion.FailureDetail != "" {
		t.Fatalf("plain exhaustion carries diagnostic fields: %#v", exhaustion)
	}

	scopeExhaustion := parkStatusFor(manifest, parkFacts{
		exhaustionApplies: true,
		exhaustionCode:    "CANDIDATE_SCOPE_FAILED",
		exhaustionDetail:  "SLICE_OUTSIDE_SCOPE: outside.txt",
	})
	if scopeExhaustion.Cause != ParkCauseExhaustion ||
		scopeExhaustion.FailureCode != "CANDIDATE_SCOPE_FAILED" ||
		scopeExhaustion.FailureDetail != "SLICE_OUTSIDE_SCOPE: outside.txt" {
		t.Fatalf("scope-refused exhaustion park = %#v", scopeExhaustion)
	}
}

// A3: Test degradation budget exceeded parks the run at a safe boundary
func TestDegradationBudgetExceededParksRunAtSafeBoundary(t *testing.T) {
	t.Parallel()

	fixture := newDegradationStatusFixture(t)
	ctx := fixture.ctx
	manifest := fixture.manifest
	store := fixture.store
	service := fixture.service
	engine := fixture.engine
	owner := fixture.owner
	now := fixture.now

	// Budget is default 3. Record 3 legacy fallback events -> should NOT
	// park; a journal recorded before this slice keeps its evaluation.
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
	// A3: the status seam names the park: cause, count, budget, and knob.
	if status.Park == nil ||
		status.Park.Cause != ParkCauseDegradation ||
		status.Park.FallbackCount != 4 ||
		status.Park.Budget != 3 ||
		status.Park.UnblockKnob != DegradationUnblockKnob {
		t.Fatalf("degradation park = %#v, want cause=%q count=4 budget=3 knob=%q",
			status.Park, ParkCauseDegradation, DegradationUnblockKnob)
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

	// The typed park event carries cause, count, budget, knob, and the
	// counted fallback list, and round-trips the strict parser exactly.
	parsed, err := ParseDegradationParkEvent(parkEvent.Body)
	if err != nil {
		t.Fatalf("cannot parse degradation_budget_parked body: %v (raw: %s)", err, string(parkEvent.Body))
	}
	if parsed.RunID != manifest.value.RunID ||
		parsed.Cause != ParkCauseDegradation ||
		parsed.Count != 4 ||
		parsed.Budget != 3 ||
		parsed.UnblockKnob != DegradationUnblockKnob {
		t.Fatalf("typed park event = %#v", parsed)
	}
	if len(parsed.Fallbacks) != 4 {
		t.Fatalf("recorded fallbacks count = %d, want 4", len(parsed.Fallbacks))
	}
	if parsed.Fallbacks[3].Reason != "continuation.ledger.step_budget_exhausted" {
		t.Fatalf("recorded fallbacks[3].Reason = %q, want 'continuation.ledger.step_budget_exhausted'", parsed.Fallbacks[3].Reason)
	}
}

// TestParseDegradationParkEventValidation pins the strict egress-side gate:
// canonical encoding, closed fields, and honest facts.
func TestParseDegradationParkEventValidation(t *testing.T) {
	t.Parallel()

	valid := func() DegradationParkEvent {
		return DegradationParkEvent{
			SchemaVersion: DegradationParkEventVersion,
			RunID:         "run-degradation",
			Cause:         ParkCauseDegradation,
			Count:         4,
			Budget:        3,
			UnblockKnob:   DegradationUnblockKnob,
			Fallbacks: []DegradationFallback{
				{Offset: 1, Reason: "absence"},
				{Offset: 2, Reason: "absence"},
				{Offset: 3, Reason: "absence"},
				{Offset: 4, Reason: "expiry"},
			},
		}
	}

	body, err := json.Marshal(valid())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDegradationParkEvent(body)
	if err != nil || parsed.Count != 4 || parsed.RunID != "run-degradation" {
		t.Fatalf("valid park event rejected: %#v, %v", parsed, err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*DegradationParkEvent)
	}{
		{
			name:   "unknown schema version",
			mutate: func(value *DegradationParkEvent) { value.SchemaVersion = "sworn.degradation-park/v2" },
		},
		{
			name:   "wrong cause",
			mutate: func(value *DegradationParkEvent) { value.Cause = "attention" },
		},
		{
			name:   "count does not cross budget",
			mutate: func(value *DegradationParkEvent) { value.Count = 3 },
		},
		{
			name:   "count does not match fallbacks",
			mutate: func(value *DegradationParkEvent) { value.Count = 5 },
		},
		{
			name:   "wrong knob",
			mutate: func(value *DegradationParkEvent) { value.UnblockKnob = "limits.other" },
		},
		{
			name:   "empty fallback reason",
			mutate: func(value *DegradationParkEvent) { value.Fallbacks[0].Reason = "" },
		},
		{
			name:   "zero fallback offset",
			mutate: func(value *DegradationParkEvent) { value.Fallbacks[0].Offset = 0 },
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := valid()
			test.mutate(&value)
			body, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if parsed, err := ParseDegradationParkEvent(body); err == nil {
				t.Fatalf("mutated park event accepted: %#v", parsed)
			}
		})
	}

	t.Run("legacy array body is not a typed park event", func(t *testing.T) {
		t.Parallel()
		legacy := `[{"offset":1,"reason":"absence"}]`
		if parsed, err := ParseDegradationParkEvent([]byte(legacy)); err == nil {
			t.Fatalf("legacy body parsed as typed park event: %#v", parsed)
		}
	})

	t.Run("noncanonical bytes rejected", func(t *testing.T) {
		t.Parallel()
		noncanonical, err := json.MarshalIndent(valid(), "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if parsed, err := ParseDegradationParkEvent(noncanonical); err == nil {
			t.Fatalf("noncanonical park event accepted: %#v", parsed)
		}
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		t.Parallel()
		body, err := json.Marshal(valid())
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.Replace(body, []byte(`{"schema_version"`), []byte(`{"future_field":"x","schema_version"`), 1)
		if parsed, err := ParseDegradationParkEvent(body); err == nil {
			t.Fatalf("unknown field accepted: %#v", parsed)
		}
	})
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
