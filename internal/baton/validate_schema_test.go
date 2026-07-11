package baton

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton/schemas"
)

// TestValidateSchema_Compiles confirms every embedded schema compiles under a
// real draft-2020-12 evaluator (the legacy hand-rolled validator never did).
func TestValidateSchema_Compiles(t *testing.T) {
	for name := range schemas.SchemaMap {
		if _, err := CompiledSchema(name); err != nil {
			t.Errorf("schema %q failed to compile: %v", name, err)
		}
	}
}

// TestValidateSchema_GoodAndBad proves real validation accepts a conformant
// slice-status payload and rejects a malformed one (missing required field).
func TestValidateSchema_GoodAndBad(t *testing.T) {
	good := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01-x", "release": "r1", "state": "planned",
		"verification": {"result": "pending"}
	}`
	if err := ValidateSchema("slice-status-v1", []byte(good)); err != nil {
		t.Errorf("good payload rejected: %v", err)
	}

	bad := `{"slice_id": "S01-x"}` // missing required release/state/verification
	if err := ValidateSchema("slice-status-v1", []byte(bad)); err == nil {
		t.Error("malformed payload accepted — real validation not enforcing required fields")
	}

	if err := ValidateSchema("no-such-schema", []byte(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "unknown schema") {
		t.Errorf("unknown schema should error, got %v", err)
	}
}

// TestValidateSchema_VerifierVerdict proves the ADR-0011 keystone schema enforces
// its core contract: the verdict enum is real, and a FAIL/BLOCKED verdict MUST
// carry at least one violation (the allOf conditional that structurally prevents
// a verifier from failing a slice without citing why).
func TestValidateSchema_VerifierVerdict(t *testing.T) {
	pass := `{"schema_version": 1, "verdict": "PASS", "rationale": "all checks satisfied"}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(pass)); err != nil {
		t.Errorf("valid PASS verdict rejected: %v", err)
	}

	failWithViolations := `{"schema_version": 1, "verdict": "FAIL", "rationale": "AC3 unmet",
		"violations": [{"gate": "adversarial", "description": "AC3 not satisfied"}]}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(failWithViolations)); err != nil {
		t.Errorf("valid FAIL+violations verdict rejected: %v", err)
	}

	failNoViolations := `{"schema_version": 1, "verdict": "FAIL", "rationale": "vague"}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(failNoViolations)); err == nil {
		t.Error("FAIL without violations accepted — allOf conditional not enforced")
	}

	badEnum := `{"schema_version": 1, "verdict": "MAYBE", "rationale": "x"}`
	if err := ValidateSchema("verifier-verdict-v1", []byte(badEnum)); err == nil {
		t.Error("out-of-enum verdict accepted — verdict enum not enforced")
	}
}

// TestValidateSchema_EffortComplexity proves the #36 effort_complexity field is
// enforced by real draft-2020-12 evaluation on BOTH spec-v1 (planner-canonical)
// and slice-status-v1 (implementer mirror): a conformant rating validates, an
// off-enum axis is rejected, and a rating missing a required axis is rejected.
func TestValidateSchema_EffortComplexity(t *testing.T) {
	// v0.10.0 spec-v1: additionalProperties:false, schema_version retired, and
	// slice_id/user_outcome/covers_needs/acceptance_criteria/in_scope/out_of_scope
	// all required.
	specGood := `{
		"$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
		"slice_id": "S01", "release": "r1",
		"user_outcome": "the thing works", "covers_needs": ["N-01"],
		"in_scope": ["do the thing"], "out_of_scope": ["not the other thing"],
		"acceptance_criteria": [{"id": "AC-01", "text": "the thing shall work"}],
		"effort_complexity": {"effort": "high", "complexity": "low", "quadrant": "grind"}
	}`
	if err := ValidateSchema("spec-v1", []byte(specGood)); err != nil {
		t.Errorf("good spec rating rejected: %v", err)
	}

	specBadEnum := `{
		"$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
		"slice_id": "S01", "release": "r1",
		"user_outcome": "the thing works", "covers_needs": ["N-01"],
		"in_scope": ["do the thing"], "out_of_scope": ["not the other thing"],
		"acceptance_criteria": [{"id": "AC-01", "text": "the thing shall work"}],
		"effort_complexity": {"effort": "medium", "complexity": "low", "quadrant": "grind"}
	}`
	if err := ValidateSchema("spec-v1", []byte(specBadEnum)); err == nil {
		t.Error("off-enum effort accepted — schema enum not enforced")
	}

	statusGood := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01", "release": "r1", "state": "planned",
		"verification": {"result": "pending"},
		"effort_complexity": {"effort": "low", "complexity": "high", "quadrant": "puzzle", "confirmed_by_implementer": true}
	}`
	if err := ValidateSchema("slice-status-v1", []byte(statusGood)); err != nil {
		t.Errorf("good status rating rejected: %v", err)
	}

	statusBadMissing := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01", "release": "r1", "state": "planned",
		"verification": {"result": "pending"},
		"effort_complexity": {"effort": "low"}
	}`
	if err := ValidateSchema("slice-status-v1", []byte(statusBadMissing)); err == nil {
		t.Error("rating missing required complexity/quadrant accepted")
	}
}
