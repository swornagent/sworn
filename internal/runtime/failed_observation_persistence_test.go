package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// TestMarshalObservationBodyLoudOnFailure pins A3's marshal-failure branch:
// the single observation marshal seam is error-aware, and a failing marshal
// yields the contentless marker body (whose digest is self-consistent, never
// sha256 of empty bytes) plus the marshal error for the caller to join.
func TestMarshalObservationBodyLoudOnFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("marshal exploded")
	marker := []byte(`{"observation_marshal_error":true}`)
	body, err := marshalObservationBody(
		func(any) ([]byte, error) { return nil, sentinel },
		driver.Observation{},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("marshal error = %v, want %v", err, sentinel)
	}
	if !bytes.Equal(body, marker) {
		t.Fatalf("marker body = %q, want %q", body, marker)
	}
	if len(body) == 0 {
		t.Fatal("marker body must not be the digested-nothing")
	}
	if sha256Digest(body) != driver.Digest(body) {
		t.Fatalf("marker digest is not self-consistent")
	}

	expected := []byte(`{"transport_status":"runner_error"}`)
	body, err = marshalObservationBody(
		func(any) ([]byte, error) { return expected, nil },
		driver.Observation{},
	)
	if err != nil || !bytes.Equal(body, expected) {
		t.Fatalf("pass-through = %q, %v", body, err)
	}
}

// TestRunnerErrorDispatchPersistsObservationBody pins A1 on the
// DRIVER_OPERATIONAL_FAILURE / runner_error branch: the failed attempt's
// marshaled observation is durably readable under the attempt whose
// observation_digest it matches.
func TestRunnerErrorDispatchPersistsObservationBody(t *testing.T) {
	t.Parallel()

	expected := driver.Observation{
		TransportStatus: driver.RunnerError,
		DurationMillis:  120000,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "adapter_failed"},
		Events:     []driver.TerminalEvent{{Sequence: 1, Kind: "result_completed"}},
	}
	dispatcher := fixtureDriver(func(
		_ context.Context,
		_ driver.Invocation,
	) (driver.Observation, error) {
		return expected, &driver.ContractError{
			Code: "ADAPTER_UNAVAILABLE",
		}
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	)
	if dispatchErr == nil {
		t.Fatal("expected dispatch to fail with a runner error, got nil")
	}
	if !IsCode(dispatchErr, "DRIVER_OPERATIONAL_FAILURE") {
		t.Fatalf("dispatch error = %v", dispatchErr)
	}

	observed := readPersistedAttemptObservation(t, fixture)
	var decoded driver.Observation
	if err := json.Unmarshal(observed.Body, &decoded); err != nil {
		t.Fatalf("persisted body is not a driver observation: %v", err)
	}
	if !reflect.DeepEqual(decoded, expected) {
		t.Fatalf("persisted observation = %#v, want %#v", decoded, expected)
	}
	if observed.Transport != string(driver.RunnerError) {
		t.Fatalf("persisted transport = %s, want %s", observed.Transport, driver.RunnerError)
	}
}

// TestInvalidDriverHandoffDispatchPersistsObservationBody pins A1 on the
// invalid_driver_handoff branch: the digest's pre-image — including the
// invalid submission bytes the engine already digests — is durably readable.
func TestInvalidDriverHandoffDispatchPersistsObservationBody(t *testing.T) {
	t.Parallel()

	expected := driver.Observation{
		TransportStatus: driver.Completed,
		DurationMillis:  7,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
		Handoff: &driver.SealedHandoff{
			SubmissionBytes:  []byte("not a sealed submission"),
			SubmissionDigest: driver.Digest([]byte("not a sealed submission")),
			SealBytes:        []byte("not a seal"),
			SealDigest:       driver.Digest([]byte("not a seal")),
		},
		Events: []driver.TerminalEvent{{Sequence: 1, Kind: "result_completed"}},
	}
	dispatcher := fixtureDriver(func(
		_ context.Context,
		_ driver.Invocation,
	) (driver.Observation, error) {
		return expected, nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	)
	if dispatchErr == nil {
		t.Fatal("expected dispatch to fail with an invalid handoff, got nil")
	}
	if !IsCode(dispatchErr, "INVALID_DRIVER_HANDOFF") {
		t.Fatalf("dispatch error = %v", dispatchErr)
	}
	effect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.OperationalFailed ||
		effect.ErrorCode != "invalid_driver_handoff" {
		t.Fatalf("dispatch effect = %#v", effect)
	}

	observed := readPersistedAttemptObservation(t, fixture)
	var decoded driver.Observation
	if err := json.Unmarshal(observed.Body, &decoded); err != nil {
		t.Fatalf("persisted body is not a driver observation: %v", err)
	}
	if !reflect.DeepEqual(decoded, expected) {
		t.Fatalf("persisted observation = %#v, want %#v", decoded, expected)
	}
	if observed.Transport != string(driver.Completed) {
		t.Fatalf("persisted transport = %s, want %s", observed.Transport, driver.Completed)
	}
}

// TestSuccessfulDispatchPersistsNoObservationBody pins the scope exclusion:
// the sealed handoff remains the successful attempt's durable record, and
// the success path stores no observation body.
func TestSuccessfulDispatchPersistsNoObservationBody(t *testing.T) {
	t.Parallel()

	dispatcher := fixtureDriver(func(
		ctx context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		_ = ctx
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	// A successful dispatch seals the prepared candidate; the empty fixture
	// workspace has nothing to seal, so plant the one candidate file first.
	if err := os.WriteFile(
		filepath.Join(fixture.workspace.Path(), "one.txt"),
		[]byte("content\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); err != nil {
		t.Fatalf("expected dispatch to succeed, got %v", err)
	}

	observed, err := fixture.store.AttemptObservation(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.cycle.DispatchEffect,
		fixture.coordinates.Try,
	)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Stored || observed.Partial || observed.Body != nil {
		t.Fatalf("success attempt observation = %#v, want stored=false", observed)
	}
}

func readPersistedAttemptObservation(
	t *testing.T,
	fixture *productionImplementationRecoveryFixture,
) journal.AttemptObservation {
	t.Helper()
	observed, err := fixture.store.AttemptObservation(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.cycle.DispatchEffect,
		fixture.coordinates.Try,
	)
	if err != nil {
		t.Fatalf("failed to read persisted observation: %v", err)
	}
	if !observed.Stored || observed.Partial {
		t.Fatalf(
			"persisted observation stored/partial = %t/%t, want true/false",
			observed.Stored,
			observed.Partial,
		)
	}
	if observed.Number != fixture.coordinates.Try {
		t.Fatalf(
			"persisted attempt number = %d, want %d",
			observed.Number,
			fixture.coordinates.Try,
		)
	}
	if observed.Responsibility != string(driver.ImplementerImplementation) {
		t.Fatalf(
			"persisted responsibility = %s, want %s",
			observed.Responsibility,
			driver.ImplementerImplementation,
		)
	}
	if observed.Digest != sha256Digest(observed.Body) {
		t.Fatalf(
			"stored body digest = %s, want %s",
			sha256Digest(observed.Body),
			observed.Digest,
		)
	}
	return observed
}

// S3-failure-turn-context A1: a driver-error operational failure carries the
// last turns of that attempt, newest that fit, oldest-first, with omitted
// counts, in the same journal transaction as the failure.
func TestFailureTurnContextDriverErrorCarriesBoundedTail(t *testing.T) {
	t.Parallel()
	dispatcher := fixtureDriver(func(ctx context.Context, inv driver.Invocation) (driver.Observation, error) {
		if inv.ToolResultHook != nil {
			for turn := int64(1); turn <= 3; turn++ {
				_ = inv.ToolResultHook(ctx, driver.ToolResultTurn{
					Turn: turn,
					Results: []driver.ToolResultRecord{{
						Sequence: 1, ToolCallID: "call-1", Tool: "Read",
						TotalBytes: 4, Head: "ZGF0YQ==",
					}},
				})
			}
		}
		if inv.WorkerTurnHook != nil {
			_ = inv.WorkerTurnHook(ctx, driver.WorkerTurn{
				Turn: 2,
				Content: []driver.WorkerTurnPart{{
					Kind: driver.WorkerTurnPartText, TotalBytes: 5, Head: "aGVsbG8=",
				}},
			})
		}
		return driver.Observation{
			TransportStatus: driver.RunnerError,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Diagnostic:      driver.Diagnostic{Code: "adapter_failed"},
		}, &driver.ContractError{Code: "ADAPTER_UNAVAILABLE"}
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, fixture.cycle, fixture.coordinates,
	)
	if dispatchErr == nil || !IsCode(dispatchErr, "DRIVER_OPERATIONAL_FAILURE") {
		t.Fatalf("dispatch error = %v", dispatchErr)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var failureBody []byte
	var failureKind string
	for _, event := range snapshot.Events {
		if isFailureTurnContextKind(event.Kind) {
			assoc, stored := parseFailureEventBody(event.Body)
			if assoc.EffectID == fixture.cycle.DispatchEffect && stored != nil {
				failureBody = event.Body
				failureKind = event.Kind
			}
		}
	}
	if len(failureBody) == 0 {
		t.Fatalf("no failure-turn-context event for %s (kinds: %v)", fixture.cycle.DispatchEffect, snapshot.Events)
	}
	if failureKind != "dispatch_operational_failure" && !strings.HasPrefix(failureKind, "dispatch_operational_failure.") {
		t.Fatalf("failure kind = %q", failureKind)
	}
	_, stored := parseFailureEventBody(failureBody)
	if stored.Status != FailureTurnContextPresent {
		t.Fatalf("stored status = %q, want present", stored.Status)
	}
	if len(stored.Turns) != 4 {
		t.Fatalf("turns = %d, want 4 (3 tool + 1 worker, all fit 5/48KiB)", len(stored.Turns))
	}
	// Same-transaction atomicity: the failed effect and its context are
	// visible together; a failure without its context is not reachable.
	effect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, fixture.cycle.DispatchEffect)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.OperationalFailed {
		t.Fatalf("effect state = %s", effect.State)
	}
	if len(failureBody) >= journal.MaxEventBytes {
		t.Fatalf("failure event = %d bytes, journal bound %d", len(failureBody), journal.MaxEventBytes)
	}
	// The status projection exposes the same tail beside the failure code,
	// decoded once, through the one digest-checked pass (tested without a
	// full Status, whose manifest the recovery fixture does not record).
	contexts := failureContextsForSnapshot(snapshot)
	decoded := contexts[fixture.cycle.DispatchEffect]
	if decoded == nil || decoded.Status != FailureTurnContextPresent || len(decoded.Turns) != 4 {
		t.Fatalf("decoded = %#v", decoded)
	}
	if effect.ErrorCode == "" {
		t.Fatal("effect lost the failure code beside its context")
	}
	// Decoded display strings, counts authoritative.
	turn := decoded.Turns[0]
	if len(turn.Results) == 0 || turn.Results[0].Head != "data" {
		t.Fatalf("decoded head = %#v", turn.Results)
	}
}

// S3 A1: an invalid driver handoff carries the same bounded tail through
// the one helper, once for all failure sites.
func TestFailureTurnContextInvalidHandoffCarriesTail(t *testing.T) {
	t.Parallel()
	dispatcher := fixtureDriver(func(ctx context.Context, inv driver.Invocation) (driver.Observation, error) {
		if inv.ToolResultHook != nil {
			_ = inv.ToolResultHook(ctx, driver.ToolResultTurn{
				Turn: 1,
				Results: []driver.ToolResultRecord{{
					Sequence: 1, ToolCallID: "c1", Tool: "Bash", TotalBytes: 2, Head: "b2s=",
				}},
			})
		}
		return driver.Observation{
			TransportStatus: driver.Completed,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
			Handoff: &driver.SealedHandoff{
				SubmissionBytes:  []byte("not a sealed submission"),
				SubmissionDigest: driver.Digest([]byte("not a sealed submission")),
				SealBytes:        []byte("not a seal"),
				SealDigest:       driver.Digest([]byte("not a seal")),
			},
		}, nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, fixture.cycle, fixture.coordinates,
	)
	if dispatchErr == nil || !IsCode(dispatchErr, "INVALID_DRIVER_HANDOFF") {
		t.Fatalf("dispatch error = %v", dispatchErr)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, fixture.cycle.DispatchEffect)
	if err != nil {
		t.Fatal(err)
	}
	if effect.ErrorCode != "invalid_driver_handoff" {
		t.Fatalf("handoff code = %q", effect.ErrorCode)
	}
	decoded := failureContextsForSnapshot(snapshot)[fixture.cycle.DispatchEffect]
	if decoded == nil || decoded.Status != FailureTurnContextPresent || len(decoded.Turns) != 1 || decoded.Turns[0].Turn != 1 {
		t.Fatalf("handoff context = %#v", decoded)
	}
}

// S3 A1: a dispatch that failed before any turn carries explicit empty,
// not an absent field. Old bodies without the key decode as absent, not
// corrupt.
func TestFailureTurnContextEmptyVsAbsent(t *testing.T) {
	t.Parallel()
	dispatcher := fixtureDriver(func(context.Context, driver.Invocation) (driver.Observation, error) {
		return driver.Observation{
			TransportStatus: driver.RunnerError,
			Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
		}, &driver.ContractError{Code: "ADAPTER_UNAVAILABLE"}
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, fixture.cycle, fixture.coordinates,
	)
	if dispatchErr == nil {
		t.Fatal("expected dispatch to fail")
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	decoded := failureContextsForSnapshot(snapshot)[fixture.cycle.DispatchEffect]
	if decoded == nil || decoded.Status != FailureTurnContextEmpty || len(decoded.Turns) != 0 {
		t.Fatalf("empty context = %#v, want explicit empty with turns:[]", decoded)
	}
	// Explicit empty marshals with the key present; absent (nil) omits it.
	effectStatus := EffectStatus{ID: fixture.cycle.DispatchEffect, FailureTurnContext: decoded}
	body, _ := json.Marshal(effectStatus)
	if !bytes.Contains(body, []byte(`"failure_turn_context"`)) || !bytes.Contains(body, []byte(`"status":"empty"`)) {
		t.Fatalf("empty JSON = %s, want explicit key", body)
	}
	absent := EffectStatus{ID: fixture.cycle.DispatchEffect}
	absentBody, _ := json.Marshal(absent)
	if bytes.Contains(absentBody, []byte(`"failure_turn_context"`)) {
		t.Fatalf("absent JSON = %s, want no key", absentBody)
	}
	// A pre-S3 body (plain association, no key) decodes as absent, not
	// corrupt, and Status reports nil rather than empty.
	assocBody := MarshalAssociation(EventAssociation{EffectID: "attempt/old/e1/t1", WorkID: "old"})
	_, stored := parseFailureEventBody(assocBody)
	if stored != nil {
		t.Fatalf("old body decoded as %#v, want absent (nil)", stored)
	}
	if decoded := decodeFailureContextStored(stored); decoded != nil {
		t.Fatalf("old body decoded to %#v, want nil", decoded)
	}
}

// S3 A2 + C1: the uncertain writes that follow a real invoke
// (preparation and completion uncertain) route through the same helper and
// carry the live tail; the prior-process/ownerless uncertain writes with
// no live tail carry unavailable with no_live_tail, and the failure is
// still journaled exactly as today.
func TestFailureTurnContextUncertainLiveVsNoLiveTail(t *testing.T) {
	t.Parallel()
	service, store, run := toolResultRuntimeFixture(t)
	_ = context.Background()
	// Live tail: feed then assemble for a preparation-uncertain write.
	liveID := "attempt/work-live/e1/t1"
	service.initFailureTail(liveID)
	service.feedFailureTailTool(liveID, driver.ToolResultTurn{
		Turn: 1, Results: []driver.ToolResultRecord{{Sequence: 1, ToolCallID: "c1", Tool: "Read", TotalBytes: 1}},
	})
	liveBody := service.failureEventBodyFor(EventAssociation{EffectID: liveID, WorkID: "work-live"}, nil, liveID)
	_, liveStored := parseFailureEventBody(liveBody)
	if liveStored == nil || liveStored.Status != FailureTurnContextPresent {
		t.Fatalf("live uncertain = %#v, want present", liveStored)
	}
	// No live tail: no init, no feed.
	deadID := "attempt/work-dead/e1/t1"
	deadBody := service.failureEventBodyFor(EventAssociation{EffectID: deadID, WorkID: "work-dead"}, nil, deadID)
	_, deadStored := parseFailureEventBody(deadBody)
	if deadStored == nil || deadStored.Status != FailureTurnContextUnavailable || deadStored.Reason != FailureTurnContextNoLiveTail {
		t.Fatalf("dead uncertain = %#v, want unavailable/no_live_tail", deadStored)
	}
	// Association passes through unchanged beside the context.
	var assoc EventAssociation
	_ = json.Unmarshal(deadBody, &assoc)
	if assoc.EffectID != deadID || assoc.WorkID != "work-dead" {
		t.Fatalf("assoc = %#v", assoc)
	}
	_ = store
	_ = run
}

// S3 A2: assembling context never alters the failure code, the refusal
// result, the observation digest, or the try accounting. The same error
// and observation journal identically with turns observed (present) and
// without (empty), differing only in the context status.
func TestFailureTurnContextNeverAltersFailureFacts(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, emit bool) (journal.Effect, journal.AttemptObservation, []byte) {
		t.Helper()
		dispatcher := fixtureDriver(func(ctx context.Context, inv driver.Invocation) (driver.Observation, error) {
			if emit && inv.ToolResultHook != nil {
				_ = inv.ToolResultHook(ctx, driver.ToolResultTurn{
					Turn: 1, Results: []driver.ToolResultRecord{{Sequence: 1, ToolCallID: "c1", Tool: "Read", TotalBytes: 1}},
				})
			}
			return driver.Observation{
				TransportStatus: driver.RunnerError,
				Usage:           driver.UsageReceipt{TokenStatus: driver.UsageUnavailable, CostStatus: driver.UsageUnavailable},
				Diagnostic:      driver.Diagnostic{Code: "adapter_failed"},
			}, &driver.ContractError{Code: "ADAPTER_UNAVAILABLE"}
		})
		fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
		_, _, err := fixture.service.runProductionImplementationDispatch(
			fixture.ctx, fixture.engine, fixture.owner, fixture.workspace, fixture.cycle, fixture.coordinates,
		)
		if err == nil {
			t.Fatal("expected dispatch to fail")
		}
		effect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, fixture.cycle.DispatchEffect)
		if err != nil {
			t.Fatal(err)
		}
		observed, err := fixture.store.AttemptObservation(fixture.ctx, fixture.manifest.value.RunID, fixture.cycle.DispatchEffect, fixture.coordinates.Try)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
		if err != nil {
			t.Fatal(err)
		}
		var failureBody []byte
		for _, event := range snapshot.Events {
			if isFailureTurnContextKind(event.Kind) {
				if assoc, _ := parseFailureEventBody(event.Body); assoc.EffectID == fixture.cycle.DispatchEffect {
					failureBody = event.Body
				}
			}
		}
		return effect, observed, failureBody
	}
	withEffect, withObserved, withBody := run(t, true)
	withoutEffect, withoutObserved, withoutBody := run(t, false)
	if withEffect.ErrorCode != withoutEffect.ErrorCode || withEffect.ErrorCode == "" {
		t.Fatalf("codes = %q vs %q", withEffect.ErrorCode, withoutEffect.ErrorCode)
	}
	if !bytes.Equal(withEffect.Result, withoutEffect.Result) {
		t.Fatalf("refusal results differ")
	}
	if withObserved.Digest != withoutObserved.Digest || withObserved.Number != withoutObserved.Number {
		t.Fatalf("observation digest/try = %s/%d vs %s/%d", withObserved.Digest, withObserved.Number, withoutObserved.Digest, withoutObserved.Number)
	}
	_, withStored := parseFailureEventBody(withBody)
	_, withoutStored := parseFailureEventBody(withoutBody)
	if withStored.Status != FailureTurnContextPresent || withoutStored.Status != FailureTurnContextEmpty {
		t.Fatalf("statuses = %q vs %q, want present vs empty", withStored.Status, withoutStored.Status)
	}
}

// S3 C1: pin the complete failure/uncertain site inventory. The five
// dispatch_operational_failure completions, dispatch_preparation_failed,
// and the uncertain outcomes that follow a real invoke all route through
// the one helper (isFailureTurnContextKind true, including continuation
// suffixes). Scheduler cycle sweeps that never held the dispatch stay
// absent by construction (false), and absent keeps one meaning: pre-S3
// record or sweep reconcile.
func TestFailureTurnContextSiteInventory(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{
		"dispatch_operational_failure",
		"dispatch_operational_failure.continuation.fresh_rehydrate.fallback",
		"dispatch_uncertain",
		"dispatch_uncertain.continuation.fresh_rehydrate.fallback_expired",
		"dispatch_preparation_failed",
		"dispatch_preparation_failed.continuation.fresh_rehydrate.fallback",
		"dispatch_preparation_uncertain",
		"dispatch_completion_uncertain",
	} {
		if !isFailureTurnContextKind(kind) {
			t.Fatalf("kind %q not covered, want helper coverage", kind)
		}
	}
	for _, kind := range []string{
		"dispatch_completed",
		"dispatch_claim_cleared",
		"dispatch_completed.continuation.fresh_rehydrate.fallback",
		"implementation_uncertain",
		"implementation_preparation_uncertain",
		"implementation_dispatch_uncertain",
		"implementation_reconciled_all_old",
		"candidate_sealed",
		"tool_result_observed",
		"worker_turn_observed",
	} {
		if isFailureTurnContextKind(kind) {
			t.Fatalf("kind %q covered, want absent (success, clear, or sweep that never held the dispatch)", kind)
		}
	}
}
