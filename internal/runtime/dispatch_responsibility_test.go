package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// knownDispatchResponsibility reports whether the given value is one of the
// driver's own dispatch responsibilities. The dispatch association body is
// content-free: only engine-owned coordinates may appear.
func knownDispatchResponsibility(value driver.Responsibility) bool {
	switch value {
	case driver.PlannerProposal,
		driver.ImplementerDesign,
		driver.ImplementerImplementation,
		driver.CaptainReview,
		driver.CaptainPlanReview,
		driver.WorkVerification,
		driver.AssemblyVerification:
		return true
	default:
		return false
	}
}

// assertEventBodyKeySet pins A3's content-free property: the body's JSON key
// set is exactly the declared engine-owned vocabulary, so no provider
// content, prompt, or submission text can ride in unnoticed.
func assertEventBodyKeySet(t *testing.T, body []byte, want ...string) {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		t.Fatalf("body is not a JSON object: %v (body %q)", err, body)
	}
	got := make([]string, 0, len(keys))
	for key := range keys {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("body key set = %v, want %v (body %s)", got, want, body)
	}
}

// assertEventBodyKeysWithin pins A3's content-free property when the emitted
// key set varies with dispatch coordinates: every key must come from the
// association vocabulary, and the required keys must be present.
func assertEventBodyKeysWithin(
	t *testing.T,
	body []byte,
	required, vocabulary []string,
) {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		t.Fatalf("body is not a JSON object: %v (body %q)", err, body)
	}
	allowed := make(map[string]bool, len(vocabulary))
	for _, key := range vocabulary {
		allowed[key] = true
	}
	for key := range keys {
		if !allowed[key] {
			t.Fatalf(
				"body key %q is outside the association vocabulary (body %s)",
				key, body,
			)
		}
	}
	for _, key := range required {
		if _, found := keys[key]; !found {
			t.Fatalf("body lacks required key %q (body %s)", key, body)
		}
	}
}

// singleDispatchEvent returns the one event whose kind carries the given
// dispatch prefix, failing on zero or ambiguous matches.
func singleDispatchEvent(
	t *testing.T,
	snapshot journal.Snapshot,
	prefix string,
) journal.Event {
	t.Helper()
	var found []journal.Event
	for _, event := range snapshot.Events {
		if strings.HasPrefix(event.Kind, prefix) {
			found = append(found, event)
		}
	}
	if len(found) != 1 {
		t.Fatalf(
			"events with prefix %q = %d, want exactly 1",
			prefix, len(found),
		)
	}
	return found[0]
}

// dispatchAssociationFromEvent decodes an event body into the association
// shape and pins A1's identity fields: the body names the exact effect,
// work, and slice the dispatch carried, and the dispatched responsibility
// alongside them.
func dispatchAssociationFromEvent(
	t *testing.T,
	fixture *productionImplementationRecoveryFixture,
	event journal.Event,
) EventAssociation {
	t.Helper()
	var assoc EventAssociation
	if err := json.Unmarshal(event.Body, &assoc); err != nil {
		t.Fatalf(
			"%s body is not an association: %v (body %q)",
			event.Kind, err, event.Body,
		)
	}
	if assoc.EffectID != fixture.cycle.DispatchEffect ||
		assoc.WorkID != fixture.cycle.DispatchWork ||
		assoc.Slice != "S1" ||
		assoc.Responsibility != fixture.coordinates.Responsibility ||
		!knownDispatchResponsibility(assoc.Responsibility) {
		t.Fatalf("%s association = %#v", event.Kind, assoc)
	}
	return assoc
}

// continuationFallbackBodyKeys is the exact engine-owned key vocabulary of a
// continuation-fallback event body: the four association fields the dispatch
// seam emits (track is omitempty and unset there) plus the fallback
// wrapper's own version, reason, retained, and posture facts.
var continuationFallbackBodyKeys = []string{
	"schema_version",
	"effect_id",
	"work_id",
	"slice",
	"responsibility",
	"reason",
	"retained",
	"posture",
}

// TestDispatchCompletedBodyCarriesResponsibility pins A1 and A3 on the
// successful production dispatch: the dispatch_completed event body carries
// the dispatched responsibility beside effect, work, and slice, and nothing
// else beyond the engine-owned fallback vocabulary.
func TestDispatchCompletedBodyCarriesResponsibility(t *testing.T) {
	t.Parallel()

	dispatcher := fixtureDriver(func(
		_ context.Context,
		invocation driver.Invocation,
	) (driver.Observation, error) {
		return productionImplementationObservation(t, invocation), nil
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	attempt := journal.EffectAttempt{
		WorkID: fixture.cycle.DispatchWork,
		Epoch:  fixture.coordinates.Epoch,
		Try:    1,
	}
	submission, err := fixture.service.runDriverEffectWithPreparation(
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
	if submission.Responsibility != driver.ImplementerImplementation {
		t.Fatalf("submission responsibility = %q", submission.Responsibility)
	}

	snapshot, err := fixture.store.Snapshot(
		fixture.ctx,
		fixture.manifest.value.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	event := singleDispatchEvent(t, snapshot, "dispatch_completed")
	if event.Kind !=
		"dispatch_completed.continuation.fresh_rehydrate.fallback" {
		t.Fatalf("dispatch_completed family kind = %q", event.Kind)
	}
	dispatchAssociationFromEvent(t, fixture, event)
	assertEventBodyKeySet(
		t,
		event.Body,
		continuationFallbackBodyKeys...,
	)
}

// TestOperationalFailureBodyCarriesResponsibility pins A1 and A3 on the
// failed production dispatch: the dispatch_operational_failure event body
// carries the dispatched responsibility beside effect, work, and slice, and
// nothing else beyond the engine-owned fallback vocabulary.
func TestOperationalFailureBodyCarriesResponsibility(t *testing.T) {
	t.Parallel()

	dispatcher := fixtureDriver(func(
		_ context.Context,
		_ driver.Invocation,
	) (driver.Observation, error) {
		return driver.Observation{}, &driver.ContractError{
			Code: "ADAPTER_UNAVAILABLE",
		}
	})
	fixture := newProductionImplementationRecoveryFixture(t, dispatcher)

	attempt := journal.EffectAttempt{
		WorkID: fixture.cycle.DispatchWork,
		Epoch:  fixture.coordinates.Epoch,
		Try:    1,
	}
	_, err := fixture.service.runDriverEffectWithPreparation(
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
	if err == nil || !IsCode(err, "DRIVER_OPERATIONAL_FAILURE") {
		t.Fatalf("dispatch error = %v, want DRIVER_OPERATIONAL_FAILURE", err)
	}

	snapshot, snapshotErr := fixture.store.Snapshot(
		fixture.ctx,
		fixture.manifest.value.RunID,
	)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	event := singleDispatchEvent(
		t,
		snapshot,
		"dispatch_operational_failure",
	)
	if event.Kind !=
		"dispatch_operational_failure.continuation.fresh_rehydrate.fallback" {
		t.Fatalf("dispatch_operational_failure family kind = %q", event.Kind)
	}
	dispatchAssociationFromEvent(t, fixture, event)
	assertEventBodyKeySet(
		t,
		event.Body,
		continuationFallbackBodyKeys...,
	)
}
