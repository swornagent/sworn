package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// TestProviderLimitRefusalRecordedInJournalEffectResult pins the durable
// half of A2: a limit-failed dispatch journals the provider's own bounded
// words into the dispatch effect's Result through the existing
// refusal-result path, instead of recording a bare PROVIDER_LIMITED. This
// fixture dispatcher returns the ContractError straight from the stub, so
// this test intentionally does not exercise normalizeAdapterError — the
// dispatcher boundary is pinned by TestNormalizeAdapterErrorPreservesBoundedProviderDetail
// in the driver package.
func TestProviderLimitRefusalRecordedInJournalEffectResult(t *testing.T) {
	t.Parallel()

	const expectedDetail = "Monthly spending limit reached. Contact billing to raise the cap."

	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return driver.Observation{}, &driver.ContractError{
			Code:   "PROVIDER_LIMITED",
			Detail: expectedDetail,
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
		t.Fatal("expected dispatch to fail with provider limit error, got nil")
	}

	// 1. Verify driver.dispatch effect in journal carries the provider words.
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
	if dispatchEffect.ErrorCode != "PROVIDER_LIMITED" {
		t.Fatalf("dispatch error code = %s, want PROVIDER_LIMITED", dispatchEffect.ErrorCode)
	}
	if len(dispatchEffect.Result) == 0 {
		t.Fatal("expected non-empty Result on dispatch effect for provider limit refusal")
	}
	var dispatchRefusal productionRefusalBinding
	if err := json.Unmarshal(dispatchEffect.Result, &dispatchRefusal); err != nil {
		t.Fatalf("cannot decode dispatch effect result: %v (raw: %s)", err, string(dispatchEffect.Result))
	}
	if dispatchRefusal.Code != "PROVIDER_LIMITED" {
		t.Fatalf("refusal code = %s, want PROVIDER_LIMITED", dispatchRefusal.Code)
	}
	if dispatchRefusal.Detail != expectedDetail {
		t.Fatalf("refusal detail = %s, want %s", dispatchRefusal.Detail, expectedDetail)
	}
	if len(dispatchRefusal.Paths) != 0 {
		t.Fatalf("refusal paths = %v, want empty", dispatchRefusal.Paths)
	}

	// 2. The outer failure completion journals the same provider words.
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
	if sealRefusal.Code != "PROVIDER_LIMITED" || sealRefusal.Detail != expectedDetail {
		t.Fatalf("unexpected seal refusal: %#v", sealRefusal)
	}
}
