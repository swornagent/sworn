package driver

import (
	"context"
	"strings"
	"testing"
)

func TestYieldCodecIsClosedBoundedAndBindsOneNonBatonTerminal(t *testing.T) {
	t.Parallel()
	value := Yield{
		SchemaVersion: YieldSchemaVersion,
		InvocationID:  "yield-1",
		Kind:          YieldBlocked,
		Message:       "The required external fact is unavailable.",
	}
	body, err := EncodeYield(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeYield(body)
	if err != nil || decoded != value {
		t.Fatalf("decoded = %#v, error = %v", decoded, err)
	}
	unknown := []byte(strings.Replace(
		string(body),
		`"message":`,
		`"responsibility":"captain_review","message":`,
		1,
	))
	if _, err := DecodeYield(unknown); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("authority-bearing yield error = %v", err)
	}
	value.Message = strings.Repeat("x", MaxYieldMessageBytes+1)
	if err := ValidateYield(value); !IsCode(err, "INVALID_YIELD_MESSAGE") {
		t.Fatalf("oversized yield error = %v", err)
	}
}

func TestYieldCodecAdmitsOnlyTheTwoTypedHumanTurnKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []YieldKind{
		YieldHumanChoice,
		YieldHumanConfirmation,
	} {
		value := Yield{
			SchemaVersion: YieldSchemaVersion,
			InvocationID:  "human-turn-1",
			Kind:          kind,
			Message:       "Confirm the exact intended meaning.",
		}
		body, err := EncodeYield(value)
		if err != nil {
			t.Fatalf("%s encode: %v", kind, err)
		}
		decoded, err := DecodeYield(body)
		if err != nil || decoded != value {
			t.Fatalf("%s decoded = %#v, error = %v", kind, decoded, err)
		}
	}
	invalid := Yield{
		SchemaVersion: YieldSchemaVersion,
		InvocationID:  "human-turn-1",
		Kind:          "human_approval",
		Message:       "Approve this plan.",
	}
	if err := ValidateYield(invalid); !IsCode(err, "INVALID_YIELD_KIND") {
		t.Fatalf("authority-shaped kind error = %v", err)
	}
}

func TestObservationRejectsSubmissionAndYieldTogether(t *testing.T) {
	t.Parallel()
	invocation, adapter, handoff := memoryInvocationFixture(t)
	adapter.observation = pointerTo(unavailableObservation())
	adapter.observation.Handoff = &handoff
	adapter.observation.Yield = &Yield{
		SchemaVersion: YieldSchemaVersion,
		InvocationID:  invocation.Request.InvocationID,
		Kind:          YieldQuestion,
		Message:       "This must not coexist with a submission.",
	}
	observation, err := (Dispatcher{}).Invoke(
		context.Background(),
		invocation,
	)
	if !IsCode(err, "INVALID_OBSERVATION") ||
		observation.Handoff != nil || observation.Yield != nil {
		t.Fatalf("observation = %#v, error = %v", observation, err)
	}
}
