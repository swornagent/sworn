package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
