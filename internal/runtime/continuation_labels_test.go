package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func TestLabelledContinuationRefusalRecordedInJournalEffectResult(t *testing.T) {
	t.Parallel()

	const expectedLabel = "continuation.toolcall_decode.correlate_reuse"

	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return driver.Observation{}, &driver.ContractError{
			Code:   "CONTINUATION_INVALID",
			Detail: expectedLabel,
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
		t.Fatal("expected dispatch to fail with continuation error, got nil")
	}

	// 1. Verify driver.dispatch effect in journal
	dispatchEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.cycle.DispatchEffect,
	)
	if err != nil {
		t.Fatalf("failed to read dispatch effect: %v", err)
	}

	if dispatchEffect.State != journal.OperationalFailed {
		t.Fatalf("dispatch effect state = %s, want %s", dispatchEffect.State, journal.OperationalFailed)
	}
	if dispatchEffect.ErrorCode != "CONTINUATION_INVALID" {
		t.Fatalf("dispatch error code = %s, want CONTINUATION_INVALID", dispatchEffect.ErrorCode)
	}
	if len(dispatchEffect.Result) == 0 {
		t.Fatal("expected non-empty Result on dispatch effect for labelled refusal")
	}

	var dispatchRefusal productionRefusalBinding
	if err := json.Unmarshal(dispatchEffect.Result, &dispatchRefusal); err != nil {
		t.Fatalf("cannot decode dispatch effect result: %v (raw: %s)", err, string(dispatchEffect.Result))
	}
	if dispatchRefusal.Code != "CONTINUATION_INVALID" {
		t.Fatalf("refusal code = %s, want CONTINUATION_INVALID", dispatchRefusal.Code)
	}
	if dispatchRefusal.Detail != expectedLabel {
		t.Fatalf("refusal detail = %s, want %s", dispatchRefusal.Detail, expectedLabel)
	}
	if len(dispatchRefusal.Paths) != 0 {
		t.Fatalf("refusal paths = %v, want empty", dispatchRefusal.Paths)
	}

	// Complete outer failure
	if err := fixture.service.completeImplementationFailure(
		fixture.ctx,
		fixture.owner,
		fixture.outer.ID,
		fixture.outer.CurrentClaim,
		stableErrorCode(dispatchErr),
		extractRefusalResult(dispatchErr),
	); err != nil {
		t.Fatal(err)
	}

	// 2. Verify git.seal effect in journal
	sealEffect, err := fixture.store.Effect(
		fixture.ctx,
		fixture.manifest.value.RunID,
		fixture.outer.ID,
	)
	if err != nil {
		t.Fatalf("failed to read seal effect: %v", err)
	}
	if sealEffect.State != journal.OperationalFailed {
		t.Fatalf("seal effect state = %s, want %s", sealEffect.State, journal.OperationalFailed)
	}
	if sealEffect.ErrorCode != "DRIVER_OPERATIONAL_FAILURE" {
		t.Fatalf("seal error code = %s, want DRIVER_OPERATIONAL_FAILURE", sealEffect.ErrorCode)
	}
	var sealRefusal productionRefusalBinding
	if err := json.Unmarshal(sealEffect.Result, &sealRefusal); err != nil {
		t.Fatalf("cannot decode seal effect refusal: %v", err)
	}
	if sealRefusal.Code != "CONTINUATION_INVALID" || sealRefusal.Detail != expectedLabel {
		t.Fatalf("unexpected seal refusal: %#v", sealRefusal)
	}

	// 3. Verify Try 2 retry does not carry path refusal for detail-only continuation exit
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
	if retryWorkContext.Refusal != nil {
		t.Fatalf("expected nil Refusal for continuation failure without path violations, got: %#v", retryWorkContext.Refusal)
	}
}

func TestDifferentLabelledContinuationRefusalsTravelToJournal(t *testing.T) {
	t.Parallel()

	labels := []string{
		"continuation.ledger.step_budget_exhausted",
		"continuation.provider.resume_state_invalid",
		"continuation.toolcall_decode.arguments_json_invalid",
		"continuation.responses.tool_count_out_of_bounds",
	}

	for _, label := range labels {
		label := label
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			dispatcher := fixtureDriver(func(
				_ context.Context,
				invocation driver.Invocation,
			) (driver.Observation, error) {
				return driver.Observation{}, &driver.ContractError{
					Code:   "CONTINUATION_INVALID",
					Detail: label,
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
				t.Fatal("expected dispatch error, got nil")
			}

			dispatchEffect, err := fixture.store.Effect(
				fixture.ctx,
				fixture.manifest.value.RunID,
				fixture.cycle.DispatchEffect,
			)
			if err != nil {
				t.Fatal(err)
			}
			if dispatchEffect.State != journal.OperationalFailed {
				t.Fatalf("state = %s, want %s", dispatchEffect.State, journal.OperationalFailed)
			}
			if dispatchEffect.ErrorCode != "CONTINUATION_INVALID" {
				t.Fatalf("error_code = %s, want CONTINUATION_INVALID", dispatchEffect.ErrorCode)
			}

			var refusal productionRefusalBinding
			if err := json.Unmarshal(dispatchEffect.Result, &refusal); err != nil {
				t.Fatalf("unmarshal result = %v", err)
			}
			if refusal.Code != "CONTINUATION_INVALID" || refusal.Detail != label {
				t.Fatalf("refusal = %#v, want Code: CONTINUATION_INVALID, Detail: %s", refusal, label)
			}
		})
	}
}
