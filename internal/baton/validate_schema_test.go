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
