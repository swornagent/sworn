package baton

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestNormaliseRealRecordsValidateStrict is the design-pin-4 proof: a REAL
// on-disk legacy record (the live release board.json and this slice's spec.json,
// copied verbatim into testdata) is REJECTED by strict v0.10.0 ValidateSchema as
// authored, but PASSES after Normalise — proving the shim's strip/map set is
// derived from the actual live-record-vs-strict-schema delta and is complete.
func TestNormaliseRealRecordsValidateStrict(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		path   string
	}{
		{"board", "board-v1", filepath.Join("testdata", "normalise", "board.json")},
		{"spec", "spec-v1", filepath.Join("testdata", "normalise", "spec.json")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := os.ReadFile(c.path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			// As authored, the legacy record must FAIL strict validation (proves
			// the fixture genuinely exercises the retired shape, not a subset that
			// happens to already conform).
			if err := ValidateSchema(c.schema, raw); err == nil {
				t.Fatalf("legacy %s validated strict as-authored — fixture no longer exercises the retired shape", c.name)
			}
			norm, err := Normalise(c.schema, raw)
			if err != nil {
				t.Fatalf("Normalise(%s): %v", c.schema, err)
			}
			if err := ValidateSchema(c.schema, norm); err != nil {
				t.Errorf("normalised %s does not conform to strict %s: %v", c.name, c.schema, err)
			}
		})
	}
}

// TestNormaliseStatusQuadrant proves a legacy slice-status record carrying the
// retired epic quadrant passes strict slice-status-v1 after Normalise maps it to
// beast (the effort_complexity block is additionalProperties:false with the
// quick/grind/puzzle/beast enum at v0.10.0).
func TestNormaliseStatusQuadrant(t *testing.T) {
	legacy := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01", "release": "r1", "state": "in_progress",
		"verification": {"result": "pending"},
		"effort_complexity": {"effort": "high", "complexity": "high", "quadrant": "epic", "confirmed_by_implementer": true}
	}`
	if err := ValidateSchema("slice-status-v1", []byte(legacy)); err == nil {
		t.Fatal("legacy epic status validated strict as-authored — enum not enforced")
	}
	norm, err := Normalise("slice-status-v1", []byte(legacy))
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	if err := ValidateSchema("slice-status-v1", norm); err != nil {
		t.Errorf("normalised status does not conform: %v", err)
	}
	// Confirm the name mapped to beast.
	var m map[string]interface{}
	_ = json.Unmarshal(norm, &m)
	ec, _ := m["effort_complexity"].(map[string]interface{})
	if ec["quadrant"] != "beast" {
		t.Errorf("quadrant = %v, want beast", ec["quadrant"])
	}
}

// TestNormaliseIdempotent proves a record already in canonical form round-trips
// through Normalise unchanged in meaning (schema-valid before and after).
func TestNormaliseIdempotent(t *testing.T) {
	canonical := `{
		"$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		"slice_id": "S01", "release": "r1", "state": "planned",
		"verification": {"result": "pending"}
	}`
	norm, err := Normalise("slice-status-v1", []byte(canonical))
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	if err := ValidateSchema("slice-status-v1", norm); err != nil {
		t.Errorf("idempotent normalise broke a canonical record: %v", err)
	}
}
