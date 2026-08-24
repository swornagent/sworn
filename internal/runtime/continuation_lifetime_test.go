package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func TestManifestContinuationLifetimeAdmissionAndRoundTrip(t *testing.T) {
	t.Parallel()

	baseManifest := Manifest{
		SchemaVersion: ManifestVersion,
		RunID:         "run-continuation-lifetime",
		Repository:    "/tmp/repo",
		Release:       "rel-lifetime",
		TargetRef:     "refs/heads/main",
		GitIdentity: gitx.Identity{
			Name:  "sworn",
			Email: "sworn@example.com",
		},
		Intent:            "test continuation lifetime knob",
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
	if bytes.Contains(absent, []byte("max_continuation_lifetime_ms")) {
		t.Fatalf("absent knob must stay absent: %s", absent)
	}
	parsed, err := ParseManifest(absent)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.EffectiveContinuationLifetime() != 24*time.Hour {
		t.Fatalf(
			"default lifetime = %s, want 24h",
			parsed.EffectiveContinuationLifetime(),
		)
	}
	if parsed.Limits.MaxContinuationLifetimeMillis != 0 {
		t.Fatalf("absent knob decoded as %d", parsed.Limits.MaxContinuationLifetimeMillis)
	}

	declared := baseManifest
	declared.Limits.MaxContinuationLifetimeMillis = 3_600_000
	body, err := canonicalManifest(declared)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := ParseManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.EffectiveContinuationLifetime() != time.Hour {
		t.Fatalf("declared lifetime = %#v", roundTrip.Limits)
	}

	zeroLimits := driver.Limits{}
	if zeroLimits.EffectiveContinuationLifetime() != 24*time.Hour {
		t.Fatalf("explicit zero limits default = %s", zeroLimits.EffectiveContinuationLifetime())
	}

	for _, mutation := range []struct {
		from string
		to   string
	}{
		{`"max_continuation_lifetime_ms":3600000`, `"max_continuation_lifetime_ms":0`},
		{`"max_continuation_lifetime_ms":3600000`, `"max_continuation_lifetime_ms":-1`},
		{`"max_continuation_lifetime_ms":3600000`, `"max_continuation_lifetime_ms":2592000001`},
	} {
		mutated := bytes.Replace(body, []byte(mutation.from), []byte(mutation.to), 1)
		if _, parseErr := ParseManifest(mutated); parseErr == nil {
			t.Fatalf("mutation %s -> %s admitted", mutation.from, mutation.to)
		}
	}
}

func TestMergeContinuationFactsIsNonLossy(t *testing.T) {
	t.Parallel()
	expired := &continuationDispatchFact{
		mode:     driver.ContinuationModeFreshRehydrate,
		outcome:  continuationOutcomeFallbackExpired,
		reason:   "expiry",
		retained: true,
	}
	reuse := &continuationDispatchFact{
		mode:    driver.ContinuationModeTranscriptReplay,
		outcome: continuationOutcomeReuse,
	}
	fallback := &continuationDispatchFact{
		mode:     driver.ContinuationModeFreshRehydrate,
		outcome:  continuationOutcomeFallback,
		reason:   "absence",
		retained: false,
	}
	if got := mergeContinuationFacts(expired, nil); got != expired {
		t.Fatalf("nil incoming cleared existing: %#v", got)
	}
	if got := mergeContinuationFacts(nil, reuse); got != reuse {
		t.Fatalf("nil existing did not take incoming: %#v", got)
	}
	if got := mergeContinuationFacts(expired, reuse); got != expired {
		t.Fatalf("existing fallback_expired lost to reuse: %#v", got)
	}
	if got := mergeContinuationFacts(reuse, expired); got != expired {
		t.Fatalf("incoming fallback_expired lost to reuse: %#v", got)
	}
	if got := mergeContinuationFacts(reuse, fallback); got != fallback {
		t.Fatalf("non-expired merge did not take incoming: %#v", got)
	}
}

func recoverableLifetimeEntry(
	t *testing.T,
	prepared preparedDriverDispatch,
	cycle turnRecoveryCycle,
	before string,
) *retainedContinuation {
	t.Helper()
	binding, selectionDigest, err := recoverableContinuationBinding(
		prepared,
		cycle,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &retainedContinuation{
		handle:          &driver.Continuation{},
		binding:         binding,
		selectionDigest: selectionDigest,
		before:          before,
	}
}

func TestExpiredMidYieldResumeDegradesWithFallbackExpiredFact(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	resumes, starts := 0, 0
	dispatcher := &continuationFixtureDriver{
		invoke: func(context.Context, driver.Invocation) (driver.Observation, error) {
			t.Fatal("unexpected Invoke")
			return driver.Observation{}, nil
		},
		turn: func(
			context.Context,
			driver.Invocation,
			driver.ContinuationBinding,
			*driver.Continuation,
		) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
			t.Fatal("unexpected InvokeTurn")
			return driver.Observation{}, nil, driver.ContinuationResult{}, nil
		},
		recoverable: func(
			_ context.Context,
			invocation driver.Invocation,
			_ driver.ContinuationBinding,
			handle *driver.Continuation,
			_ *driver.RecoverableTurnInput,
		) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
			mu.Lock()
			defer mu.Unlock()
			if handle != nil {
				resumes++
				return driver.Observation{}, nil, driver.ContinuationResult{
					Mode:   driver.ContinuationModeFreshRehydrate,
					Status: driver.ContinuationStatusExpired,
					Reason: "expiry",
				}, nil
			}
			starts++
			return driver.Observation{
					TransportStatus: driver.Completed,
					Yield: &driver.Yield{
						SchemaVersion: driver.YieldSchemaVersion,
						InvocationID:  invocation.Request.InvocationID,
						Kind:          driver.YieldQuestion,
						Message:       "Rehydrated after expiry.",
					},
				}, &driver.Continuation{}, driver.ContinuationResult{
					Mode:   driver.ContinuationModeFreshRehydrate,
					Status: driver.ContinuationStatusSuspended,
				}, nil
		},
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()
	prepared, cycle := preparedTurnRecoveryFixture(t, fixture)
	entry := recoverableLifetimeEntry(
		t,
		prepared,
		cycle,
		fixture.cycle.Before,
	)
	observation, retained, fact, err := fixture.service.invokeRecoverableWorker(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		prepared,
		fixture.cycle.Before,
		fixture.owner,
		&cycle,
		entry,
		driver.RecoverableTurnInput{
			SchemaVersion: driver.RecoverableTurnInputSchemaVersion,
			Kind:          driver.RecoverableInputAnswer,
			Answer:        "Use the exact approved fixture value.",
		},
		true,
	)
	if err != nil {
		t.Fatalf("expired resume failed: %v", err)
	}
	if observation.Yield == nil || retained == nil || retained.handle == nil {
		t.Fatalf(
			"rehydrated yield not admitted: observation=%#v retained=%#v",
			observation,
			retained,
		)
	}
	if fact == nil ||
		fact.mode != driver.ContinuationModeFreshRehydrate ||
		fact.outcome != continuationOutcomeFallbackExpired ||
		fact.reason != "expiry" ||
		!fact.retained ||
		fact.posture != driver.ContinuationPostureContextRetaining {
		t.Fatalf("fallback fact = %#v", fact)
	}
	if retained.fallbackFact == nil ||
		retained.fallbackFact.outcome != continuationOutcomeFallbackExpired {
		t.Fatalf("retained handle dropped expiry fact: %#v", retained.fallbackFact)
	}
	mu.Lock()
	defer mu.Unlock()
	if resumes != 1 || starts != 1 {
		t.Fatalf("calls resume=%d start=%d", resumes, starts)
	}
}

func TestRehydratedFreshYieldIsAdmittedWithoutExpiryLabel(t *testing.T) {
	t.Parallel()
	dispatcher := &continuationFixtureDriver{
		invoke: func(context.Context, driver.Invocation) (driver.Observation, error) {
			t.Fatal("unexpected Invoke")
			return driver.Observation{}, nil
		},
		turn: func(
			context.Context,
			driver.Invocation,
			driver.ContinuationBinding,
			*driver.Continuation,
		) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
			t.Fatal("unexpected InvokeTurn")
			return driver.Observation{}, nil, driver.ContinuationResult{}, nil
		},
		recoverable: func(
			_ context.Context,
			invocation driver.Invocation,
			_ driver.ContinuationBinding,
			handle *driver.Continuation,
			_ *driver.RecoverableTurnInput,
		) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
			if handle != nil {
				t.Fatal("expected a fresh rehydrate, got a handle")
			}
			return driver.Observation{
					TransportStatus: driver.Completed,
					Yield: &driver.Yield{
						SchemaVersion: driver.YieldSchemaVersion,
						InvocationID:  invocation.Request.InvocationID,
						Kind:          driver.YieldQuestion,
						Message:       "Fresh start yielded.",
					},
				}, &driver.Continuation{}, driver.ContinuationResult{
					Mode:   driver.ContinuationModeFreshRehydrate,
					Status: driver.ContinuationStatusSuspended,
				}, nil
		},
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()
	prepared, cycle := preparedTurnRecoveryFixture(t, fixture)
	observation, retained, fact, err := fixture.service.invokeRecoverableWorker(
		fixture.ctx,
		fixture.engine,
		fixture.workspace,
		prepared,
		fixture.cycle.Before,
		fixture.owner,
		&cycle,
		nil,
		driver.RecoverableTurnInput{
			SchemaVersion: driver.RecoverableTurnInputSchemaVersion,
			Kind:          driver.RecoverableInputAnswer,
			Answer:        "Use the exact approved fixture value.",
		},
		true,
	)
	if err != nil || observation.Yield == nil || retained == nil ||
		retained.handle == nil || fact != nil {
		t.Fatalf(
			"unlabeled rehydrate yield err=%v observation=%#v retained=%#v fact=%#v",
			err,
			observation,
			retained,
			fact,
		)
	}
}

func TestYieldPathInvalidContinuationSitesCarryDistinctLabels(t *testing.T) {
	t.Parallel()

	const (
		resumeWithHandle = "turn_recovery.recoverable_resume.fresh_rehydrate_request_with_handle"
		yieldShape       = "turn_recovery.recoverable_yield.invalid_result_shape"
		promotionShape   = "turn_recovery.recoverable_promotion.invalid_result_shape"
		terminalShape    = "turn_recovery.recoverable_terminal.invalid_result_shape"
	)
	for _, label := range []string{
		resumeWithHandle, yieldShape, promotionShape, terminalShape,
	} {
		err := runtimeFailSite("INVALID_CONTINUATION", label, nil)
		if !IsCode(err, "INVALID_CONTINUATION") {
			t.Fatalf("label %s code = %v", label, err)
		}
		refusal := extractRefusalResult(err)
		if len(refusal) == 0 {
			t.Fatalf("label %s produced empty refusal", label)
		}
		var binding productionRefusalBinding
		if json.Unmarshal(refusal, &binding) != nil ||
			binding.Code != "INVALID_CONTINUATION" ||
			binding.Detail != label {
			t.Fatalf("label %s refusal = %#v (%s)", label, binding, refusal)
		}
	}

	type siteCase struct {
		name   string
		label  string
		entry  bool
		result func(driver.Invocation) (
			driver.Observation,
			*driver.Continuation,
			driver.ContinuationResult,
			error,
		)
	}
	cases := []siteCase{
		{
			name:  "fresh rehydrate request with handle",
			label: resumeWithHandle,
			entry: true,
			result: func(invocation driver.Invocation) (
				driver.Observation,
				*driver.Continuation,
				driver.ContinuationResult,
				error,
			) {
				return driver.Observation{}, &driver.Continuation{},
					driver.ContinuationResult{
						Mode:   driver.ContinuationModeFreshRehydrate,
						Status: driver.ContinuationStatusExpired,
						Reason: "expiry",
					}, nil
			},
		},
		{
			name:  "yield default",
			label: yieldShape,
			result: func(invocation driver.Invocation) (
				driver.Observation,
				*driver.Continuation,
				driver.ContinuationResult,
				error,
			) {
				return driver.Observation{
						Yield: &driver.Yield{
							SchemaVersion: driver.YieldSchemaVersion,
							InvocationID:  invocation.Request.InvocationID,
							Kind:          driver.YieldQuestion,
							Message:       "contradiction",
						},
					}, nil, driver.ContinuationResult{
						Mode:   driver.ContinuationModeTranscriptReplay,
						Status: driver.ContinuationStatusResumed,
					}, nil
			},
		},
		{
			name:  "terminal default",
			label: terminalShape,
			entry: true,
			result: func(invocation driver.Invocation) (
				driver.Observation,
				*driver.Continuation,
				driver.ContinuationResult,
				error,
			) {
				return continuationTestObservation(
						t,
						invocation,
						driver.ImplementerImplementation,
					), &driver.Continuation{}, driver.ContinuationResult{
						Mode:   driver.ContinuationModeTranscriptReplay,
						Status: driver.ContinuationStatusResumed,
					}, nil
			},
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dispatcher := &continuationFixtureDriver{
				invoke: func(context.Context, driver.Invocation) (driver.Observation, error) {
					t.Fatal("unexpected Invoke")
					return driver.Observation{}, nil
				},
				turn: func(
					context.Context,
					driver.Invocation,
					driver.ContinuationBinding,
					*driver.Continuation,
				) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
					t.Fatal("unexpected InvokeTurn")
					return driver.Observation{}, nil, driver.ContinuationResult{}, nil
				},
				recoverable: func(
					_ context.Context,
					invocation driver.Invocation,
					_ driver.ContinuationBinding,
					_ *driver.Continuation,
					_ *driver.RecoverableTurnInput,
				) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
					return test.result(invocation)
				},
			}
			fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
			defer fixture.workspace.Close()
			prepared, cycle := preparedTurnRecoveryFixture(t, fixture)
			var entry *retainedContinuation
			if test.entry {
				entry = recoverableLifetimeEntry(
					t,
					prepared,
					cycle,
					fixture.cycle.Before,
				)
			}
			_, _, _, err := fixture.service.invokeRecoverableWorker(
				fixture.ctx,
				fixture.engine,
				fixture.workspace,
				prepared,
				fixture.cycle.Before,
				fixture.owner,
				&cycle,
				entry,
				driver.RecoverableTurnInput{
					SchemaVersion: driver.RecoverableTurnInputSchemaVersion,
					Kind:          driver.RecoverableInputAnswer,
					Answer:        "Use the exact approved fixture value.",
				},
				true,
			)
			if !IsCode(err, "INVALID_CONTINUATION") {
				t.Fatalf("error = %v, want INVALID_CONTINUATION", err)
			}
			var binding productionRefusalBinding
			body := extractRefusalResult(err)
			if json.Unmarshal(body, &binding) != nil ||
				binding.Code != "INVALID_CONTINUATION" ||
				binding.Detail != test.label {
				t.Fatalf("refusal = %#v (%s), want %s", binding, body, test.label)
			}
		})
	}
}

func TestYieldPathInvalidContinuationLabelReachesAttemptWrite(t *testing.T) {
	t.Parallel()
	const wantLabel = "turn_recovery.recoverable_yield.invalid_result_shape"
	dispatcher := &continuationFixtureDriver{
		invoke: func(
			_ context.Context,
			invocation driver.Invocation,
		) (driver.Observation, error) {
			return driver.Observation{
				TransportStatus: driver.Completed,
				Yield: &driver.Yield{
					SchemaVersion: driver.YieldSchemaVersion,
					InvocationID:  invocation.Request.InvocationID,
					Kind:          driver.YieldQuestion,
					Message:       "Which exact approved value should I use?",
				},
			}, nil
		},
		turn: func(
			context.Context,
			driver.Invocation,
			driver.ContinuationBinding,
			*driver.Continuation,
		) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
			t.Fatal("unexpected InvokeTurn")
			return driver.Observation{}, nil, driver.ContinuationResult{}, nil
		},
		recoverable: func(
			_ context.Context,
			invocation driver.Invocation,
			_ driver.ContinuationBinding,
			_ *driver.Continuation,
			_ *driver.RecoverableTurnInput,
		) (driver.Observation, *driver.Continuation, driver.ContinuationResult, error) {
			return driver.Observation{
					Yield: &driver.Yield{
						SchemaVersion: driver.YieldSchemaVersion,
						InvocationID:  invocation.Request.InvocationID,
						Kind:          driver.YieldQuestion,
						Message:       "contradiction after automation",
					},
				}, nil, driver.ContinuationResult{
					Mode:   driver.ContinuationModeTranscriptReplay,
					Status: driver.ContinuationStatusResumed,
				}, nil
		},
		automation: func(
			_ context.Context,
			invocation driver.AutomationInvocation,
		) (driver.AutomationObservation, error) {
			if (invocation.Recovery == nil) == (invocation.Advisory == nil) {
				t.Fatalf("automation invocation = %#v", invocation)
			}
			observation := driver.AutomationObservation{
				TransportStatus: driver.Completed,
				Usage: driver.UsageReceipt{
					TokenStatus: driver.UsageUnavailable,
					CostStatus:  driver.UsageUnavailable,
				},
				Diagnostic: driver.Diagnostic{Code: "none"},
			}
			if invocation.Recovery != nil {
				observation.Recovery = &driver.RecoveryDecision{
					SchemaVersion: driver.RecoveryDecisionSchemaVersion,
					InvocationID:  invocation.Recovery.InvocationID,
					Action:        driver.RecoveryAskCaptain,
				}
			} else {
				answer := "Use the exact approved fixture value."
				observation.Advisory = &driver.AdvisoryResult{
					SchemaVersion: driver.AdvisoryResultSchemaVersion,
					InvocationID:  invocation.Advisory.InvocationID,
					Outcome:       driver.AdvisoryAnswer,
					Answer:        &answer,
				}
			}
			return observation, nil
		},
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()
	prepared, cycle := preparedTurnRecoveryFixture(t, fixture)
	prior := journal.AttentionBinding{
		Ordinal:  1,
		Recovery: cycle.binding,
	}
	prior.ID = journal.AttentionID(prior.Recovery, prior.Ordinal)
	if _, err := fixture.store.OpenAttention(
		fixture.ctx,
		fixture.owner,
		journal.OpenAttentionCommand{
			RunID:              fixture.owner.RunID,
			Attention:          prior,
			ExpectedGeneration: 0,
			Question:           "Existing attention so first-ask park is skipped.",
		},
		fixture.now,
	); err != nil {
		t.Fatal(err)
	}
	_, _, dispatchErr := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	)
	if dispatchErr == nil {
		t.Fatal("expected labelled INVALID_CONTINUATION dispatch failure")
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
		effect.ErrorCode != "INVALID_CONTINUATION" {
		t.Fatalf("dispatch effect = %#v", effect)
	}
	var refusal productionRefusalBinding
	if json.Unmarshal(effect.Result, &refusal) != nil ||
		refusal.Code != "INVALID_CONTINUATION" ||
		refusal.Detail != wantLabel {
		t.Fatalf("journaled refusal = %#v (%s)", refusal, effect.Result)
	}
	_ = prepared
}

type midYieldExpiryDriver struct {
	t                 *testing.T
	mu                sync.Mutex
	turnCalls         int
	recoverableCalls  int
	automationCalls   int
	freshYielded      bool
	expectedAnswer    string
	successorQuestion string
}

func (d *midYieldExpiryDriver) Invoke(
	_ context.Context,
	invocation driver.Invocation,
) (driver.Observation, error) {
	d.mu.Lock()
	d.turnCalls++
	d.mu.Unlock()
	return driver.Observation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
		Yield: &driver.Yield{
			SchemaVersion: driver.YieldSchemaVersion,
			InvocationID:  invocation.Request.InvocationID,
			Kind:          driver.YieldQuestion,
			Message:       "Which exact approved value should I use?",
		},
	}, nil
}

func (d *midYieldExpiryDriver) InvokeTurn(
	context.Context,
	driver.Invocation,
	driver.ContinuationBinding,
	*driver.Continuation,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	error,
) {
	d.t.Fatal("unexpected InvokeTurn")
	return driver.Observation{}, nil, driver.ContinuationResult{}, nil
}

func (d *midYieldExpiryDriver) InvokeRecoverableTurn(
	_ context.Context,
	invocation driver.Invocation,
	_ driver.ContinuationBinding,
	handle *driver.Continuation,
	input *driver.RecoverableTurnInput,
) (
	driver.Observation,
	*driver.Continuation,
	driver.ContinuationResult,
	error,
) {
	d.mu.Lock()
	d.recoverableCalls++
	d.mu.Unlock()
	if handle != nil {
		// Mid-yield expiry: the successor resume presents the handle
		// retained by the rehydrated start and it has aged out.
		return driver.Observation{}, nil, driver.ContinuationResult{
			Mode:   driver.ContinuationModeFreshRehydrate,
			Status: driver.ContinuationStatusExpired,
			Reason: "expiry",
		}, nil
	}
	d.mu.Lock()
	alreadyYielded := d.freshYielded
	d.freshYielded = true
	d.mu.Unlock()
	if !alreadyYielded {
		return driver.Observation{
				TransportStatus: driver.Completed,
				Usage: driver.UsageReceipt{
					TokenStatus: driver.UsageUnavailable,
					CostStatus:  driver.UsageUnavailable,
				},
				Diagnostic: driver.Diagnostic{Code: "none"},
				Yield: &driver.Yield{
					SchemaVersion: driver.YieldSchemaVersion,
					InvocationID:  invocation.Request.InvocationID,
					Kind:          driver.YieldQuestion,
					Message:       d.successorQuestion,
				},
			}, &driver.Continuation{}, driver.ContinuationResult{
				Mode:   driver.ContinuationModeFreshRehydrate,
				Status: driver.ContinuationStatusSuspended,
			}, nil
	}
	if input == nil || input.Answer != d.expectedAnswer {
		d.t.Fatalf("completion input = %#v", input)
	}
	if err := os.WriteFile(
		filepath.Join(invocation.HostWorkspace, "one.txt"),
		[]byte("expired mid-yield fixture\n"),
		0o600,
	); err != nil {
		return driver.Observation{}, nil,
			driver.ContinuationResult{}, err
	}
	return continuationTestObservation(
			d.t,
			invocation,
			driver.ImplementerImplementation,
		), nil, driver.ContinuationResult{
			Mode:   driver.ContinuationModeFreshRehydrate,
			Status: driver.ContinuationStatusCompleted,
		}, nil
}

func (d *midYieldExpiryDriver) InvokeAutomation(
	_ context.Context,
	invocation driver.AutomationInvocation,
) (driver.AutomationObservation, error) {
	d.mu.Lock()
	d.automationCalls++
	d.mu.Unlock()
	if (invocation.Recovery == nil) == (invocation.Advisory == nil) {
		d.t.Fatalf("automation invocation = %#v", invocation)
	}
	observation := driver.AutomationObservation{
		TransportStatus: driver.Completed,
		Usage: driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
		Diagnostic: driver.Diagnostic{Code: "none"},
	}
	if invocation.Recovery != nil {
		observation.Recovery = &driver.RecoveryDecision{
			SchemaVersion: driver.RecoveryDecisionSchemaVersion,
			InvocationID:  invocation.Recovery.InvocationID,
			Action:        driver.RecoveryAskCaptain,
		}
	} else {
		answer := d.expectedAnswer
		observation.Advisory = &driver.AdvisoryResult{
			SchemaVersion: driver.AdvisoryResultSchemaVersion,
			InvocationID:  invocation.Advisory.InvocationID,
			Outcome:       driver.AdvisoryAnswer,
			Answer:        &answer,
		}
	}
	return observation, nil
}

func TestExpiredMidYieldCompletesWithLabeledFallbackAndCounts(t *testing.T) {
	dispatcher := &midYieldExpiryDriver{
		t:                 t,
		expectedAnswer:    "Use the exact approved fixture value.",
		successorQuestion: "Which approved follow-up value should I use?",
	}
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)
	defer fixture.workspace.Close()
	recordManifestCommand(t, fixture)
	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("first-ask park = %v", err)
	}
	attentions, err := fixture.store.Attentions(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil || len(attentions) != 1 ||
		attentions[0].State != journal.AttentionOpen {
		t.Fatalf("first attention = %#v, %v", attentions, err)
	}
	if _, err := fixture.service.AnswerAttention(
		fixture.ctx,
		AnswerAttentionCommand{
			RunID:              fixture.owner.RunID,
			AttentionID:        attentions[0].Attention.ID,
			ExpectedGeneration: attentions[0].Generation,
			Answer:             dispatcher.expectedAnswer,
		},
	); err != nil {
		t.Fatalf("answer = %v", err)
	}
	if _, _, err := fixture.service.runProductionImplementationDispatch(
		fixture.ctx,
		fixture.engine,
		fixture.owner,
		fixture.workspace,
		fixture.cycle,
		fixture.coordinates,
	); err != nil {
		t.Fatalf("expired mid-yield completion = %v", err)
	}
	effect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.owner.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil || effect.State != journal.Succeeded {
		t.Fatalf("terminal dispatch = %#v, %v", effect, err)
	}
	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.owner.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantKind := turnRecoveryRecoveredEvent +
		".continuation.fresh_rehydrate.fallback_expired"
	var found *journal.Event
	for index := range snapshot.Events {
		event := &snapshot.Events[index]
		if event.Kind == wantKind {
			found = event
			break
		}
		if event.Kind == "INVALID_CONTINUATION" ||
			bytes.Contains([]byte(event.Kind), []byte("INVALID_CONTINUATION")) {
			t.Fatalf("unexpected INVALID_CONTINUATION event: %#v", event)
		}
	}
	if found == nil {
		t.Fatalf("missing labelled expiry event in %#v", snapshot.Events)
	}
	var body continuationFallbackEvent
	if json.Unmarshal(found.Body, &body) != nil ||
		body.SchemaVersion != continuationFallbackEventVersion ||
		body.Reason != "expiry" ||
		!body.Retained ||
		body.Posture != string(driver.ContinuationPostureContextRetaining) {
		t.Fatalf("fallback body = %#v (%s)", body, found.Body)
	}
	fallbacks := degradationFallbacks(snapshot)
	if len(fallbacks) != 1 || fallbacks[0].Reason != "expiry" {
		t.Fatalf("degradation fallbacks = %#v", fallbacks)
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if dispatcher.turnCalls != 1 ||
		dispatcher.recoverableCalls != 3 ||
		dispatcher.automationCalls != 2 {
		t.Fatalf(
			"calls turn=%d recoverable=%d automation=%d",
			dispatcher.turnCalls,
			dispatcher.recoverableCalls,
			dispatcher.automationCalls,
		)
	}
}
