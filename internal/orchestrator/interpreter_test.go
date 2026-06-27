package orchestrator_test

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/orchestrator"
	"github.com/swornagent/sworn/internal/verdict"
)

// fakeClassifier implements model.Verifier for interpreter tests.
type fakeClassifier struct {
	text string
	err  error
}

func (f *fakeClassifier) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	return f.text, 0.001, f.err
}

func TestInterpreter_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		rawOutput      string
		classifierText string
		classifierErr  error
		classifierNil  bool
		wantVerdict    verdict.Verdict
		wantContains   string // substring expected in rationale
	}{
		{
			name:           "clean PASS response",
			rawOutput:      "the implementation looks good and all tests pass",
			classifierText: "PASS",
			wantVerdict:    verdict.Pass,
		},
		{
			name:           "clean FAIL response",
			rawOutput:      "acceptance check 3 is not satisfied: the button uses wrong label",
			classifierText: "FAIL",
			wantVerdict:    verdict.Fail,
		},
		{
			name:           "clean BLOCKED response",
			rawOutput:      "I cannot run the tests because the database is not running",
			classifierText: "BLOCKED",
			wantVerdict:    verdict.Blocked,
		},
		{
			name:           "ambiguous prose → INCONCLUSIVE",
			rawOutput:      "the diff looks good but I'm not 100% certain it meets every requirement",
			classifierText: "INCONCLUSIVE",
			wantVerdict:    verdict.Inconclusive,
		},
		{
			name:           "empty classifier response → INCONCLUSIVE",
			rawOutput:      "some model output that doesn't parse",
			classifierText: "",
			wantVerdict:    verdict.Inconclusive,
		},
		{
			name:          "nil model → INCONCLUSIVE",
			rawOutput:     "any output",
			classifierNil: true,
			wantVerdict:   verdict.Inconclusive,
			wantContains:  "no classifier model configured",
		},
		{
			name:           "classifier returns garbage → INCONCLUSIVE",
			rawOutput:      "some output",
			classifierText: "The implementation appears to satisfy most of the requirements but there might be some edge cases to consider",
			wantVerdict:    verdict.Inconclusive,
		},
		{
			name:           "classifier returns PASS with markdown wrapping",
			rawOutput:      "the implementation works",
			classifierText: "**PASS**",
			wantVerdict:    verdict.Pass,
		},
		{
			name:           "classifier returns FAIL with code fence",
			rawOutput:      "the implementation is wrong",
			classifierText: "```\nFAIL\n```",
			wantVerdict:    verdict.Fail,
		},
		{
			name:           "classifier returns BLOCKED with leading whitespace",
			rawOutput:      "cannot proceed",
			classifierText: "\n\n   BLOCKED",
			wantVerdict:    verdict.Blocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var classifier model.Verifier
			if !tt.classifierNil {
				classifier = &fakeClassifier{
					text: tt.classifierText,
					err:  tt.classifierErr,
				}
			}
			result := orchestrator.Interpret(context.Background(), tt.rawOutput, classifier)
			if result.Verdict != tt.wantVerdict {
				t.Errorf("verdict = %s, want %s (rationale: %s)", result.Verdict, tt.wantVerdict, result.Rationale)
			}
			if tt.wantContains != "" && !strings.Contains(result.Rationale, tt.wantContains) {
				t.Errorf("rationale missing %q: %s", tt.wantContains, result.Rationale)
			}
		})
	}
}

func TestInterpreter_ParsesVerdictCaseInsensitive(t *testing.T) {
	cases := []struct{ text, want string }{
		{"pass", "PASS"},
		{"PASS", "PASS"},
		{"Pass", "PASS"},
		{"fail", "FAIL"},
		{"FAIL", "FAIL"},
		{"Fail", "FAIL"},
		{"blocked", "BLOCKED"},
		{"BLOCKED", "BLOCKED"},
		{"Blocked", "BLOCKED"},
		{"inconclusive", "INCONCLUSIVE"},
		{"INCONCLUSIVE", "INCONCLUSIVE"},
		{"Inconclusive", "INCONCLUSIVE"},
	}
	for _, c := range cases {
		fc := &fakeClassifier{text: c.text}
		result := orchestrator.Interpret(context.Background(), "test input", fc)
		if string(result.Verdict) != c.want {
			t.Errorf("Interpret(%q) → %s, want %s", c.text, result.Verdict, c.want)
		}
	}
}

func TestInterpreterErrInterpretInconclusive(t *testing.T) {
	err := orchestrator.ErrInterpretInconclusive("S01-test", "ambiguous prose that does not parse to a verdict")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), orchestrator.InterpreterInconclusiveSentinel) {
		t.Errorf("error missing sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "S01-test") {
		t.Errorf("error missing slice ID: %v", err)
	}
}

func TestInterpreterErrInterpretInconclusive_TruncatesLongPreview(t *testing.T) {
	longRaw := strings.Repeat("x", 200)
	err := orchestrator.ErrInterpretInconclusive("S01-test", longRaw)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	// The preview should be truncated.
	if len(err.Error()) > len(orchestrator.InterpreterInconclusiveSentinel)+200 {
		t.Errorf("preview too long in error: %s", err.Error())
	}
	// Should include the truncation marker.
	if !strings.Contains(err.Error(), "...") {
		t.Errorf("missing truncation marker: %s", err.Error())
	}
}