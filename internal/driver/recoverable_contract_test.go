package driver

import (
	"strings"
	"testing"
)

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
