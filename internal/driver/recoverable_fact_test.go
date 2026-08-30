package driver

import (
	"encoding/json"
	"testing"
)

func TestRecoverableTurnInputFactValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   RecoverableTurnInput
		wantErr bool
	}{
		{
			name: "byte-exact operator answer fact",
			input: RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        "Use the exact approved value.",
				Fact: &AutomationFact{
					Name:  FactOperatorAnswer,
					Value: "Use the exact approved value.",
				},
			},
		},
		{
			name: "no fact is still valid",
			input: RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        "Use the exact approved value.",
			},
		},
		{
			name: "fact value diverges from the carried answer",
			input: RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        "Use the exact approved value.",
				Fact: &AutomationFact{
					Name:  FactOperatorAnswer,
					Value: "Use a different value.",
				},
			},
			wantErr: true,
		},
		{
			name: "fact carries an unrelated name",
			input: RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        "Use the exact approved value.",
				Fact: &AutomationFact{
					Name:  FactCaptainAdvice,
					Value: "Use the exact approved value.",
				},
			},
			wantErr: true,
		},
		{
			name: "fact attached to a nudge",
			input: RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputNudge,
				Fact: &AutomationFact{
					Name:  FactOperatorAnswer,
					Value: "",
				},
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRecoverableTurnInput(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateRecoverableTurnInput() = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestPrepareRecoverableInvocationDeepCopiesFact(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.recoverableInput = nil
	original := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Use the pinned prepared base.",
		Fact: &AutomationFact{
			Name:  FactOperatorAnswer,
			Value: "Use the pinned prepared base.",
		},
	}
	if err := prepareRecoverableInvocation(&invocation, original); err != nil {
		t.Fatal(err)
	}
	if invocation.recoverableInput == nil || invocation.recoverableInput.Fact == nil {
		t.Fatalf("recoverableInput = %#v", invocation.recoverableInput)
	}
	if invocation.recoverableInput.Fact == original.Fact {
		t.Fatal("carried fact aliases the caller-owned pointer")
	}
	original.Fact.Value = "mutated after prepare"
	if invocation.recoverableInput.Fact.Value != "Use the pinned prepared base." {
		t.Fatalf(
			"carried fact mutated through caller alias: %q",
			invocation.recoverableInput.Fact.Value,
		)
	}
}

func TestModelPromptCarriesOperatorAnswerFactAndDistinguishesFromPlainRecovery(
	t *testing.T,
) {
	invocation, _, _ := memoryInvocationFixture(t)
	const answer = "Use the exact approved fixture value."

	attested := invocation
	attested.recoverableInput = &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        answer,
		Fact: &AutomationFact{
			Name:  FactOperatorAnswer,
			Value: answer,
		},
	}
	unattested := invocation
	unattested.recoverableInput = &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        answer,
	}

	type promptRecovery struct {
		Kind    RecoverableInputKind `json:"kind"`
		Content string               `json:"content"`
		Fact    *AutomationFact      `json:"fact"`
	}
	decode := func(invocation Invocation) promptRecovery {
		t.Helper()
		body, err := modelPrompt(invocation)
		if err != nil {
			t.Fatal(err)
		}
		var prompt struct {
			Recovery promptRecovery `json:"recovery"`
		}
		if err := json.Unmarshal(body, &prompt); err != nil {
			t.Fatal(err)
		}
		return prompt.Recovery
	}

	attestedRecovery := decode(attested)
	unattestedRecovery := decode(unattested)

	if attestedRecovery.Content != answer || unattestedRecovery.Content != answer {
		t.Fatalf(
			"content diverged from the operator's exact bytes: %#v, %#v",
			attestedRecovery, unattestedRecovery,
		)
	}
	if attestedRecovery.Fact == nil ||
		attestedRecovery.Fact.Name != FactOperatorAnswer ||
		attestedRecovery.Fact.Value != answer {
		t.Fatalf("attested recovery envelope missing fact: %#v", attestedRecovery)
	}
	if unattestedRecovery.Fact != nil {
		t.Fatalf(
			"unattested recovery envelope carries a fact: %#v",
			unattestedRecovery,
		)
	}
}
