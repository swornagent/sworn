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

type economyGuardFixture struct {
	ctx      context.Context
	manifest admittedManifest
	store    *journal.Store
	owner    journal.OwnerLease
	now      time.Time
	service  *Service
	engine   *engine
}

// newEconomyGuardFixture builds the same production-shaped service and
// journal the degradation park tests use, with operator-chosen economy
// limits.
func newEconomyGuardFixture(
	t *testing.T,
	limits driver.Limits,
) *economyGuardFixture {
	t.Helper()
	ctx := context.Background()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	manifest.value.Limits = limits
	body, err := canonicalManifest(manifest.value)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := admitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	manifest = admitted
	production, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 30, 5, 6, 7, 0, time.UTC)
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
		RunID: manifest.value.RunID, ReplayKey: "manifest", Kind: "start",
		Payload: manifest.raw, CreatedAt: now,
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
	return &economyGuardFixture{
		ctx: ctx, manifest: manifest, store: store, owner: owner,
		now: now, service: service, engine: engine,
	}
}

func testWork() string {
	return "sha256:" + strings.Repeat("b", 64)
}

// failedDispatchAttempt journals one completed operational failure on a
// driver.dispatch effect, with a durable attempt observation carrying the
// given usage receipt and an optional refusal detail.
func (f *economyGuardFixture) failedDispatchAttempt(
	t *testing.T,
	work string,
	epoch, try int64,
	code string,
	detail string,
	usage driver.UsageReceipt,
) {
	t.Helper()
	effectID := journal.AttemptEffectID(work, epoch, try)
	payload, err := json.Marshal(map[string]string{"work": work})
	if err != nil {
		t.Fatal(err)
	}
	before := sha256Digest(payload)
	expected := "sha256:" + strings.Repeat("d", 64)
	if err := f.store.RecordCommandEffect(f.ctx, journal.Command{
		RunID:     f.manifest.value.RunID,
		ReplayKey: effectID,
		Kind:      "driver.dispatch",
		Payload:   payload,
		CreatedAt: f.now,
	}, journal.Effect{
		RunID:          f.manifest.value.RunID,
		ID:             effectID,
		ReplayKey:      effectID,
		Kind:           "driver.dispatch",
		State:          journal.Pending,
		BeforeDigest:   before,
		ExpectedDigest: expected,
		UpdatedAt:      f.now,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.Claim(f.ctx, f.manifest.value.RunID, effectID, f.now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	usageBody, err := driver.EncodeUsageReceipt(usage)
	if err != nil {
		t.Fatal(err)
	}
	observationBody, err := json.Marshal(driver.Observation{
		TransportStatus: driver.RunnerError,
		Usage:           usage,
		Diagnostic:      driver.Diagnostic{Code: "economy_turn_budget"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var result []byte
	if detail != "" {
		result = mustJSON(productionRefusalBinding{Code: code, Detail: detail})
	}
	if err := f.store.Complete(f.ctx, journal.Completion{
		RunID:     f.manifest.value.RunID,
		EffectID:  effectID,
		Token:     claim.Token,
		State:     journal.OperationalFailed,
		ErrorCode: code,
		Result:    result,
		Attempt: &journal.Attempt{
			Number:            try,
			Responsibility:    string(driver.ImplementerImplementation),
			TransportStatus:   string(driver.RunnerError),
			ObservationDigest: sha256Digest(observationBody),
			Usage:             usageBody,
			ObservationBody:   observationBody,
		},
		EventKind: "dispatch_operational_failure",
		EventBody: []byte("{}"),
		At:        f.now,
	}); err != nil {
		t.Fatal(err)
	}
}

// economyUsageReceipt builds the exact receipt shape a budget-crossing
// dispatch persists: a v2 receipt with the accumulated tokens and the
// engine-counted turn economics.
func economyUsageReceipt(
	t *testing.T,
	surface string,
	inputTokens, outputTokens, turns, toolCalls int64,
) driver.UsageReceipt {
	t.Helper()
	usage, err := driver.NormalizeUsage(
		&driver.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
		nil,
		surface,
	)
	if err != nil {
		t.Fatal(err)
	}
	turnsValue := turns
	toolCallsValue := toolCalls
	usage.Turns = &turnsValue
	usage.ToolCalls = &toolCallsValue
	return usage
}

func TestEconomyTurnBudgetCrossingParksRunWithSpentAndBudget(t *testing.T) {
	t.Parallel()
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	// Default turn budget is 200; the dispatch crossed at 201 turns.
	fixture.failedDispatchAttempt(
		t,
		testWork(),
		1,
		1,
		"ECONOMY_TURN_BUDGET_EXCEEDED",
		"",
		economyUsageReceipt(t, "sworn.openai", 10_000, 20_000, 201, 201),
	)

	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "parked" {
		t.Fatalf("state = %q, want parked", status.State)
	}
	if status.Park == nil ||
		status.Park.Cause != ParkCauseEconomyTurns ||
		status.Park.Spent != 201 ||
		status.Park.Budget != driver.DefaultMaxTurnsPerWork ||
		status.Park.UnblockKnob != EconomyTurnsUnblockKnob {
		t.Fatalf("economy park = %#v", status.Park)
	}
	// A parked economy crossing never emits a retry recommendation the
	// control verbs refuse: RunStatus.Recovery stays empty for parked runs.
	if status.Recovery != nil {
		t.Fatalf("parked run carries recovery guidance = %#v", status.Recovery)
	}

	// driveLoop writes the typed park event exactly once, parks, and starts
	// no new work.
	driveErr := fixture.service.driveLoop(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		false,
	)
	if driveErr != nil {
		t.Fatalf("driveLoop error = %v", driveErr)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var parkEvents []journal.Event
	for _, event := range snapshot.Events {
		if event.Kind == ParkEventKind {
			parkEvents = append(parkEvents, event)
		}
	}
	if len(parkEvents) != 1 {
		t.Fatalf("park events = %d, want 1", len(parkEvents))
	}
	parsed, err := ParseDegradationParkEvent(parkEvents[0].Body)
	if err != nil {
		t.Fatalf("park event unparsable: %v", err)
	}
	if parsed.SchemaVersion != ParkEventVersion ||
		parsed.Cause != ParkCauseEconomyTurns ||
		parsed.Spent != 201 ||
		parsed.Budget != driver.DefaultMaxTurnsPerWork ||
		parsed.UnblockKnob != EconomyTurnsUnblockKnob {
		t.Fatalf("typed park event = %#v", parsed)
	}
	if len(snapshot.Commands) == 0 {
		t.Fatal("no commands journaled")
	}
	// A fresh service over the same journal re-parks with the same figures.
	service2 := &Service{
		journal: fixture.store, dispatcher: fixture.service.dispatcher,
		production:    fixture.service.production,
		gitExecutable: fixture.service.gitExecutable,
		now:           fixture.service.now,
	}
	status2, err := service2.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status2.State != "parked" ||
		status2.Park == nil ||
		status2.Park.Cause != ParkCauseEconomyTurns ||
		status2.Park.Spent != 201 ||
		status2.Park.Budget != driver.DefaultMaxTurnsPerWork {
		t.Fatalf("re-drive status = %#v", status2)
	}
	// A second driveLoop pass must not duplicate the park event.
	if err := fixture.service.driveLoop(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		false,
	); err != nil {
		t.Fatalf("second driveLoop error = %v", err)
	}
	snapshot, err = fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	parkCount := 0
	for _, event := range snapshot.Events {
		if event.Kind == ParkEventKind {
			parkCount++
		}
	}
	if parkCount != 1 {
		t.Fatalf("park events after re-drive = %d, want 1", parkCount)
	}
}

func TestEconomyOutputTokenBudgetCrossingParksRun(t *testing.T) {
	t.Parallel()
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	fixture.failedDispatchAttempt(
		t,
		testWork(),
		1,
		1,
		"ECONOMY_OUTPUT_BUDGET_EXCEEDED",
		"",
		economyUsageReceipt(t, "sworn.openai", 40_000, 300_000, 30, 30),
	)
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "parked" || status.Park == nil {
		t.Fatalf("status = %#v", status)
	}
	if status.Park.Cause != ParkCauseEconomyOutputTokens ||
		status.Park.Spent != 300_000 ||
		status.Park.Budget != driver.DefaultMaxOutputTokensPerWork ||
		status.Park.UnblockKnob != EconomyOutputTokensUnblockKnob {
		t.Fatalf("economy output park = %#v", status.Park)
	}
}

func TestEconomyKnobOverrideNamesOperatorBudget(t *testing.T) {
	t.Parallel()
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis:   30_000,
		OutputBytes:     65_536,
		MaxTurnsPerWork: 50,
	})
	fixture.failedDispatchAttempt(
		t,
		testWork(),
		1,
		1,
		"ECONOMY_TURN_BUDGET_EXCEEDED",
		"",
		economyUsageReceipt(t, "sworn.openai", 2_000, 4_000, 60, 60),
	)
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "parked" || status.Park == nil ||
		status.Park.Cause != ParkCauseEconomyTurns ||
		status.Park.Spent != 60 ||
		status.Park.Budget != 50 {
		t.Fatalf("knob-override park = %#v", status.Park)
	}
}

func TestUnrelatedFailureCodesDoNotPark(t *testing.T) {
	t.Parallel()
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	fixture.failedDispatchAttempt(
		t,
		testWork(),
		1,
		1,
		"INVOCATION_TIMEOUT",
		"",
		economyUsageReceipt(t, "sworn.openai", 1_000, 2_000, 3, 3),
	)
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State == "parked" && status.Park != nil &&
		(status.Park.Cause == ParkCauseEconomyTurns ||
			status.Park.Cause == ParkCauseEconomyOutputTokens) {
		t.Fatalf("unrelated failure parked as economy: %#v", status.Park)
	}
}

func TestIdenticalFailureParkBeforeTryExhaustion(t *testing.T) {
	t.Parallel()
	const code = "PROVIDER_LIMITED"
	const detail = "Monthly spending limit reached."

	for _, test := range []struct {
		name       string
		codes      []string
		details    []string
		threshold  int64
		park       bool
		claimTry3  bool
		wantCode   string
		wantDetail string
	}{
		{
			name:       "two identical failures park at default threshold",
			codes:      []string{code, code},
			details:    []string{detail, detail},
			park:       true,
			wantCode:   code,
			wantDetail: detail,
		},
		{
			name:    "differing codes never park",
			codes:   []string{code, "INVOCATION_TIMEOUT"},
			details: []string{detail, ""},
			park:    false,
		},
		{
			name:    "single failure never parks",
			codes:   []string{code},
			details: []string{detail},
			park:    false,
		},
		{
			name:      "threshold three admits two identical failures",
			codes:     []string{code, code},
			details:   []string{detail, detail},
			threshold: 3,
			park:      false,
		},
		{
			name:       "second and third tries identical park",
			codes:      []string{"INVOCATION_TIMEOUT", code, code},
			details:    []string{"", detail, detail},
			park:       true,
			wantCode:   code,
			wantDetail: detail,
		},
		{
			name:      "claimed third try breaks the run",
			codes:     []string{code, code},
			details:   []string{detail, detail},
			park:      false,
			claimTry3: true,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			limits := driver.Limits{TimeoutMillis: 30_000, OutputBytes: 65_536}
			if test.threshold != 0 {
				limits.IdenticalFailureParkAfter = test.threshold
			}
			fixture := newEconomyGuardFixture(t, limits)
			for index, tryCode := range test.codes {
				usage := economyUsageReceipt(t, "sworn.openai", 1_000, 2_000, 2, 2)
				fixture.failedDispatchAttempt(
					t,
					testWork(),
					1,
					int64(index+1),
					tryCode,
					test.details[index],
					usage,
				)
			}
			if test.claimTry3 {
				// A claimed, still-in-flight third try breaks the
				// consecutive suffix: the work is making progress.
				effectID := journal.AttemptEffectID(testWork(), 1, 3)
				payload, _ := json.Marshal(map[string]string{"work": testWork()})
				if err := fixture.store.RecordCommandEffect(
					fixture.ctx,
					journal.Command{
						RunID: fixture.manifest.value.RunID, ReplayKey: effectID,
						Kind: "driver.dispatch", Payload: payload,
						CreatedAt: fixture.now,
					},
					journal.Effect{
						RunID:          fixture.manifest.value.RunID,
						ID:             effectID,
						ReplayKey:      effectID,
						Kind:           "driver.dispatch",
						State:          journal.Pending,
						BeforeDigest:   sha256Digest(payload),
						ExpectedDigest: "sha256:" + strings.Repeat("d", 64),
						UpdatedAt:      fixture.now,
					},
				); err != nil {
					t.Fatal(err)
				}
				if _, err := fixture.store.Claim(
					fixture.ctx,
					fixture.manifest.value.RunID,
					effectID,
					fixture.now,
					time.Minute,
				); err != nil {
					t.Fatal(err)
				}
			}
			status, err := fixture.service.Status(
				fixture.ctx,
				fixture.manifest.value.RunID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !test.park {
				if status.State == "parked" && status.Park != nil &&
					status.Park.Cause == ParkCauseIdenticalFailure {
					t.Fatalf("unexpected identical-failure park: %#v", status.Park)
				}
				return
			}
			if status.State != "parked" || status.Park == nil {
				t.Fatalf("status = %#v, want parked", status)
			}
			threshold := driver.DefaultIdenticalFailureParkAfter
			if test.threshold != 0 {
				threshold = test.threshold
			}
			if status.Park.Cause != ParkCauseIdenticalFailure ||
				status.Park.FailureCode != test.wantCode ||
				status.Park.FailureDetail != test.wantDetail ||
				status.Park.Threshold != threshold ||
				status.Park.UnblockKnob != IdenticalFailureUnblockKnob {
				t.Fatalf("identical park = %#v", status.Park)
			}
			if status.Recovery != nil {
				t.Fatalf("parked run carries recovery guidance = %#v", status.Recovery)
			}
		})
	}
}

// TestIdenticalFailureLiveDispatchAdmissionParksBeforeThirdTry is the
// production dispatch round trip for A2: two real implementation dispatches
// failing with the same error code park the run at the admission boundary
// the try chains evaluate before a third try, and Status names the park with
// the shared code and run length.
func TestIdenticalFailureLiveDispatchAdmissionParksBeforeThirdTry(t *testing.T) {
	t.Parallel()
	dispatcher := fixtureDriver(func(
		_ context.Context,
		_ driver.Invocation,
	) (driver.Observation, error) {
		return driver.Observation{}, &driver.ContractError{
			Code: "INVOCATION_TIMEOUT",
		}
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	runID := fixture.manifest.value.RunID
	// The production fixture does not journal the manifest command (its
	// tests never project Status); record it so Status and driveLoop can
	// evaluate the park over this journal.
	if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
		RunID: runID, ReplayKey: "manifest", Kind: "start",
		Payload: fixture.manifest.raw, CreatedAt: fixture.now,
	}); err != nil {
		t.Fatal(err)
	}
	for try := int64(1); try <= 2; try++ {
		coordinates := fixture.coordinates
		coordinates.Try = try
		_, dispatchErr := fixture.service.runDriverEffectWithPreparation(
			fixture.ctx,
			fixture.engine,
			fixture.workspace,
			driver.RoleImplementer,
			coordinates,
			journal.EffectAttempt{
				WorkID: fixture.cycle.DispatchWork,
				Epoch:  fixture.coordinates.Epoch,
				Try:    try,
			},
			fixture.cycle.Before,
			fixture.owner,
			nil,
			true,
		)
		if !IsCode(dispatchErr, "DRIVER_OPERATIONAL_FAILURE") {
			t.Fatalf("try %d dispatch error = %v", try, dispatchErr)
		}
	}

	// The fixture's pre-claimed outer seal effect keeps the run active
	// while it is claimed; complete it the way the implementation cycle
	// does after a failed dispatch so the park can project.
	if err := fixture.store.CompleteOwned(
		fixture.ctx,
		fixture.owner,
		journal.Completion{
			RunID: runID, EffectID: fixture.outer.ID,
			Token:     fixture.outer.CurrentClaim,
			State:     journal.OperationalFailed,
			ErrorCode: "DRIVER_OPERATIONAL_FAILURE",
			EventKind: "implementation_operational_failure",
			EventBody: []byte("{}"),
			At:        fixture.now,
		},
	); err != nil {
		t.Fatal(err)
	}

	// The between-tries admission check the try chains evaluate before
	// admitting a third try sees the crossed guard.
	parked, err := fixture.service.economyGuardsParked(
		fixture.ctx,
		fixture.manifest,
		runID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !parked {
		t.Fatal("admission check did not park two identical dispatch failures")
	}

	// Status projects the park over the real production dispatch effects.
	status, err := fixture.service.Status(fixture.ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "parked" || status.Park == nil {
		t.Fatalf("status = %#v", status)
	}
	if status.Park.Cause != ParkCauseIdenticalFailure ||
		status.Park.FailureCode != "INVOCATION_TIMEOUT" ||
		status.Park.Consecutive != 2 ||
		status.Park.Threshold != driver.DefaultIdenticalFailureParkAfter {
		t.Fatalf("identical park = %#v", status.Park)
	}

	// The drive loop writes the typed park event and starts no new work.
	if err := fixture.service.driveLoop(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		false,
	); err != nil {
		t.Fatalf("driveLoop error = %v", err)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, runID)
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
		t.Fatal("no park event recorded")
	}
	parsed, err := ParseDegradationParkEvent(parkEvent.Body)
	if err != nil {
		t.Fatalf("park event unparsable: %v", err)
	}
	if parsed.Cause != ParkCauseIdenticalFailure ||
		parsed.FailureCode != "INVOCATION_TIMEOUT" ||
		parsed.Consecutive != 2 ||
		parsed.Threshold != driver.DefaultIdenticalFailureParkAfter {
		t.Fatalf("park event = %#v", parsed)
	}
}

func TestIdenticalFailureDriveLoopWritesParkEventAndReParks(t *testing.T) {
	t.Parallel()
	const code = "INVOCATION_TIMEOUT"
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	fixture.failedDispatchAttempt(
		t, testWork(), 1, 1, code, "",
		economyUsageReceipt(t, "sworn.openai", 1_000, 2_000, 3, 3),
	)
	fixture.failedDispatchAttempt(
		t, testWork(), 1, 2, code, "",
		economyUsageReceipt(t, "sworn.openai", 1_000, 2_000, 3, 3),
	)
	if err := fixture.service.driveLoop(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		false,
	); err != nil {
		t.Fatalf("driveLoop error = %v", err)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var parkEvents []journal.Event
	for _, event := range snapshot.Events {
		if event.Kind == ParkEventKind {
			parkEvents = append(parkEvents, event)
		}
	}
	if len(parkEvents) != 1 {
		t.Fatalf("park events = %d, want 1", len(parkEvents))
	}
	parsed, err := ParseDegradationParkEvent(parkEvents[0].Body)
	if err != nil {
		t.Fatalf("park event unparsable: %v", err)
	}
	if parsed.SchemaVersion != ParkEventVersion ||
		parsed.Cause != ParkCauseIdenticalFailure ||
		parsed.FailureCode != code ||
		parsed.Consecutive != 2 ||
		parsed.Threshold != driver.DefaultIdenticalFailureParkAfter ||
		parsed.UnblockKnob != IdenticalFailureUnblockKnob {
		t.Fatalf("typed park event = %#v", parsed)
	}
	// A fresh service over the same journal re-parks with the same facts.
	service2 := &Service{
		journal: fixture.store, dispatcher: fixture.service.dispatcher,
		production:    fixture.service.production,
		gitExecutable: fixture.service.gitExecutable,
		now:           fixture.service.now,
	}
	status, err := service2.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "parked" || status.Park == nil ||
		status.Park.Cause != ParkCauseIdenticalFailure ||
		status.Park.FailureCode != code ||
		status.Park.Consecutive != 2 ||
		status.Park.Threshold != driver.DefaultIdenticalFailureParkAfter {
		t.Fatalf("re-drive identical park = %#v", status.Park)
	}
}

// TestManifestEconomyKnobAdmissionAndRoundTrip pins the validated-bounds
// pattern the contract names: absent knobs get the documented defaults and
// keep legacy manifests byte-identical; declared knobs round-trip
// canonically; out-of-bounds and explicit-zero declarations are rejected.
func TestManifestEconomyKnobAdmissionAndRoundTrip(t *testing.T) {
	t.Parallel()

	baseManifest := Manifest{
		SchemaVersion: ManifestVersion,
		RunID:         "run-economy-knobs",
		Repository:    "/tmp/repo",
		Release:       "rel-economy",
		TargetRef:     "refs/heads/main",
		GitIdentity: gitx.Identity{
			Name:  "sworn",
			Email: "sworn@example.com",
		},
		Intent:            "test economy knobs",
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
			TimeoutMillis: 60_000,
			OutputBytes:   1_048_576,
		},
	}

	absent, err := canonicalManifest(baseManifest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(absent, []byte("max_turns_per_work")) ||
		bytes.Contains(absent, []byte("max_output_tokens_per_work")) ||
		bytes.Contains(absent, []byte("identical_failure_park_after")) {
		t.Fatalf("absent knobs must stay absent: %s", absent)
	}
	parsed, err := ParseManifest(absent)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.EffectiveMaxTurnsPerWork() != driver.DefaultMaxTurnsPerWork ||
		parsed.EffectiveMaxOutputTokensPerWork() != driver.DefaultMaxOutputTokensPerWork ||
		parsed.EffectiveIdenticalFailureParkAfter() != driver.DefaultIdenticalFailureParkAfter {
		t.Fatalf("defaults = turns %d tokens %d identical %d",
			parsed.EffectiveMaxTurnsPerWork(),
			parsed.EffectiveMaxOutputTokensPerWork(),
			parsed.EffectiveIdenticalFailureParkAfter())
	}

	declared := baseManifest
	declared.Limits.MaxTurnsPerWork = 50
	declared.Limits.MaxOutputTokensPerWork = 300_000
	declared.Limits.IdenticalFailureParkAfter = 3
	body, err := canonicalManifest(declared)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := ParseManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.EffectiveMaxTurnsPerWork() != 50 ||
		roundTrip.EffectiveMaxOutputTokensPerWork() != 300_000 ||
		roundTrip.EffectiveIdenticalFailureParkAfter() != 3 {
		t.Fatalf("declared knobs = %#v", roundTrip.Limits)
	}

	for _, mutation := range []struct {
		from string
		to   string
	}{
		{`"max_turns_per_work":50`, `"max_turns_per_work":0`},
		{`"max_turns_per_work":50`, `"max_turns_per_work":1001`},
		{`"max_output_tokens_per_work":300000`, `"max_output_tokens_per_work":0`},
		{`"max_output_tokens_per_work":300000`, `"max_output_tokens_per_work":4194305`},
		{`"identical_failure_park_after":3`, `"identical_failure_park_after":0`},
		{`"identical_failure_park_after":3`, `"identical_failure_park_after":4`},
	} {
		mutated := bytes.Replace(body, []byte(mutation.from), []byte(mutation.to), 1)
		if _, parseErr := ParseManifest(mutated); parseErr == nil {
			t.Fatalf("mutation %s -> %s admitted", mutation.from, mutation.to)
		}
	}
}

// TestParkEventCauseScopedIdempotence pins C1 both directions over the
// shared journal kind.
func TestParkEventCauseScopedIdempotence(t *testing.T) {
	t.Parallel()
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	appendFallback := func(index int) {
		t.Helper()
		body, _ := json.Marshal(continuationFallbackEvent{
			SchemaVersion: continuationFallbackEventVersion,
			EventAssociation: EventAssociation{
				EffectID: "attempt/" + strings.Repeat("c", 64) + "/e1/t1",
				WorkID:   "sha256:" + strings.Repeat("c", 64),
			},
			Reason:   "absence",
			Retained: true,
			Posture:  string(driver.ContinuationPostureContextRetaining),
		})
		if err := fixture.store.AppendEvent(
			fixture.ctx,
			fixture.manifest.value.RunID,
			"dispatch_completed.continuation.fresh_rehydrate.fallback",
			body,
			fixture.now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	// First: the economy park event of the shared kind exists (degradation
	// not yet crossed, so the degradation gate lets the loop continue).
	fixture.failedDispatchAttempt(
		t,
		testWork(),
		1,
		1,
		"ECONOMY_TURN_BUDGET_EXCEEDED",
		"",
		economyUsageReceipt(t, "sworn.openai", 10_000, 20_000, 201, 201),
	)
	if err := fixture.service.driveLoop(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		false,
	); err != nil {
		t.Fatalf("economy driveLoop error = %v", err)
	}

	// Then the degradation budget crosses on the same journal. The old
	// kind-only hasParkEvent scan would see the economy event and silently
	// drop the degradation park; the cause-aware scan must still admit it.
	for i := 1; i <= 4; i++ {
		appendFallback(i)
	}
	if err := fixture.service.driveLoop(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		false,
	); err != nil {
		t.Fatalf("degradation driveLoop error = %v", err)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	causes := make(map[string]int)
	for _, event := range snapshot.Events {
		if event.Kind != ParkEventKind {
			continue
		}
		parsed, err := ParseDegradationParkEvent(event.Body)
		if err != nil {
			t.Fatalf("park event unparsable: %v", err)
		}
		causes[parsed.Cause]++
	}
	if causes[ParkCauseDegradation] != 1 || causes[ParkCauseEconomyTurns] != 1 {
		t.Fatalf("park events by cause = %#v, want one each", causes)
	}
}

// TestDegradationParkEventNeverSuppressesEconomyPark pins the converse C1
// direction: an existing degradation event of the shared kind must not stop
// the economy gate from admitting its own park event.
func TestDegradationParkEventNeverSuppressesEconomyPark(t *testing.T) {
	t.Parallel()
	fixture := newEconomyGuardFixture(t, driver.Limits{
		TimeoutMillis: 30_000,
		OutputBytes:   65_536,
	})
	// A pre-existing degradation park event of the shared kind.
	legacy, err := degradationParkEvent(
		fixture.manifest.value.RunID,
		3,
		[]DegradationFallback{
			{Offset: 1, Reason: "absence"},
			{Offset: 2, Reason: "absence"},
			{Offset: 3, Reason: "absence"},
			{Offset: 4, Reason: "expiry"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.AppendEvent(
		fixture.ctx,
		fixture.manifest.value.RunID,
		ParkEventKind,
		legacy,
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	// An economy crossing with no degradation crossing: the economy gate is
	// reachable and must admit its event despite the degradation event.
	fixture.failedDispatchAttempt(
		t,
		testWork(),
		1,
		1,
		"ECONOMY_TURN_BUDGET_EXCEEDED",
		"",
		economyUsageReceipt(t, "sworn.openai", 10_000, 20_000, 201, 201),
	)
	if err := fixture.service.driveLoop(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		false,
	); err != nil {
		t.Fatalf("economy driveLoop error = %v", err)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	causes := make(map[string]int)
	for _, event := range snapshot.Events {
		if event.Kind != ParkEventKind {
			continue
		}
		parsed, err := ParseDegradationParkEvent(event.Body)
		if err != nil {
			t.Fatalf("park event unparsable: %v", err)
		}
		causes[parsed.Cause]++
	}
	if causes[ParkCauseDegradation] != 1 || causes[ParkCauseEconomyTurns] != 1 {
		t.Fatalf("park events by cause = %#v, want one each", causes)
	}
}

// TestParseParkEventNewCausesAndLegacyByteStability pins the egress gate: the
// new-cause bodies validate strictly, and a legacy degradation body
// re-encodes byte-identically.
func TestParseParkEventNewCausesAndLegacyByteStability(t *testing.T) {
	t.Parallel()

	legacy := DegradationParkEvent{
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
	legacyBytes, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDegradationParkEvent(legacyBytes)
	if err != nil {
		t.Fatal(err)
	}
	reEncoded, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyBytes, reEncoded) {
		t.Fatalf("legacy degradation body re-encodes differently: %s vs %s", legacyBytes, reEncoded)
	}

	economy, err := economyParkEventBody("run-economy", economyParkFacts{
		cause:  ParkCauseEconomyTurns,
		spent:  201,
		budget: 200,
		knob:   EconomyTurnsUnblockKnob,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsedEconomy, err := ParseDegradationParkEvent(economy)
	if err != nil {
		t.Fatal(err)
	}
	if parsedEconomy.SchemaVersion != ParkEventVersion ||
		parsedEconomy.Cause != ParkCauseEconomyTurns ||
		parsedEconomy.Spent != 201 || parsedEconomy.Budget != 200 {
		t.Fatalf("economy park event = %#v", parsedEconomy)
	}

	identical, err := identicalFailureParkEventBody("run-identical", identicalFailureFacts{
		code:        "INVOCATION_TIMEOUT",
		consecutive: 2,
		threshold:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsedIdentical, err := ParseDegradationParkEvent(identical)
	if err != nil {
		t.Fatal(err)
	}
	if parsedIdentical.Cause != ParkCauseIdenticalFailure ||
		parsedIdentical.FailureCode != "INVOCATION_TIMEOUT" ||
		parsedIdentical.Consecutive != 2 ||
		parsedIdentical.Threshold != 2 {
		t.Fatalf("identical park event = %#v", parsedIdentical)
	}

	for _, invalid := range []DegradationParkEvent{
		// New version may not ride the degradation cause.
		{
			SchemaVersion: ParkEventVersion, RunID: "run-x",
			Cause: ParkCauseDegradation, Count: 4, Budget: 3,
			UnblockKnob: DegradationUnblockKnob,
			Fallbacks: []DegradationFallback{
				{Offset: 1, Reason: "absence"},
				{Offset: 2, Reason: "absence"},
				{Offset: 3, Reason: "absence"},
				{Offset: 4, Reason: "expiry"},
			},
		},
		// Legacy version may not ride a new cause.
		{
			SchemaVersion: DegradationParkEventVersion, RunID: "run-y",
			Cause: ParkCauseEconomyTurns, Spent: 201, Budget: 200,
			UnblockKnob: EconomyTurnsUnblockKnob,
		},
		// An economy event must name a real crossing.
		{
			SchemaVersion: ParkEventVersion, RunID: "run-z",
			Cause: ParkCauseEconomyOutputTokens, Spent: 10, Budget: 200,
			UnblockKnob: EconomyOutputTokensUnblockKnob,
		},
		// An identical-failure event must reach its threshold.
		{
			SchemaVersion: ParkEventVersion, RunID: "run-w",
			Cause: ParkCauseIdenticalFailure, Consecutive: 1, Threshold: 2,
			FailureCode: "INVOCATION_TIMEOUT",
			UnblockKnob: IdenticalFailureUnblockKnob,
		},
	} {
		body, marshalErr := json.Marshal(invalid)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, parseErr := ParseDegradationParkEvent(body); parseErr == nil {
			t.Fatalf("invalid park event admitted: %s", body)
		}
	}
}
