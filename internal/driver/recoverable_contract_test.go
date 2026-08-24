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
	hook := func(_ context.Context, kind RecoveryStepKind) error {
		reserved = kind
		return nil
	}
	if err := reserveRecoveryStep(
		context.Background(), hook, RecoveryStepMalformedToolCall,
	); err != nil || reserved != RecoveryStepMalformedToolCall {
		t.Fatalf("reserve = %v, reserved = %s", err, reserved)
	}
	if err := reserveRecoveryStep(
		context.Background(), nil, RecoveryStepMalformedToolCall,
	); !IsCode(err, "RECOVERY_STEP_REFUSED") {
		t.Fatalf("expected RECOVERY_STEP_REFUSED without a hook, got: %v", err)
	}
	if err := reserveRecoveryStep(
		context.Background(), hook, RecoveryStepKind("unknown_kind"),
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
