package orchestrator_test

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/orchestrator"
)

// NOTE (ADR-0011 Step 3): the TestInterpreter_* table tests and their
// fakeClassifier were removed alongside the dead stateless prose classifier
// (orchestrator.Interpret). The INCONCLUSIVE→PAGE signalling contract below
// survives and stays covered.

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
