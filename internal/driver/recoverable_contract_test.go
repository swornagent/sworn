package driver

import (
	"context"
	"strings"
	"testing"
)

func TestReserveRecoveryStepAdmitsMalformedToolCallAndRefusesWithoutHook(
	t *testing.T,
) {
	t.Parallel()
	var reserved RecoveryStepKind
	var reservedRefusal *SubmitRefusal
	hook := func(_ context.Context, kind RecoveryStepKind, refusal *SubmitRefusal) error {
		reserved = kind
		reservedRefusal = refusal
		return nil
	}
	wantRefusal := &SubmitRefusal{Code: "TEST_CODE", Detail: "test detail"}
	if err := reserveRecoveryStep(
		context.Background(), hook, RecoveryStepMalformedToolCall, wantRefusal,
	); err != nil || reserved != RecoveryStepMalformedToolCall ||
		reservedRefusal != wantRefusal {
		t.Fatalf("reserve = %v, reserved = %s, refusal = %#v", err, reserved, reservedRefusal)
	}
	if err := reserveRecoveryStep(
		context.Background(), nil, RecoveryStepMalformedToolCall, nil,
	); !IsCode(err, "RECOVERY_STEP_REFUSED") {
		t.Fatalf("expected RECOVERY_STEP_REFUSED without a hook, got: %v", err)
	}
	if err := reserveRecoveryStep(
		context.Background(), hook, RecoveryStepKind("unknown_kind"), nil,
	); !IsCode(err, "RECOVERY_STEP_REFUSED") {
		t.Fatalf("expected RECOVERY_STEP_REFUSED for an unknown kind, got: %v", err)
	}
}

func TestRecoverableAnswerByteBoundary(t *testing.T) {
	for _, test := range []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{
			name:   "maximum",
			answer: strings.Repeat("a", MaxRecoverableInputBytes),
		},
		{
			name:    "over maximum",
			answer:  strings.Repeat("a", MaxRecoverableInputBytes+1),
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRecoverableTurnInput(RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        test.answer,
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateRecoverableTurnInput() = %v", err)
			}
		})
	}
}
