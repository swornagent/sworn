package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRecord_ParsesSpecV1(t *testing.T) {
	dir := t.TempDir()
	// A v0.10.0-shaped record: the AC item is strict (id/text/ears_pattern/
	// test_refs) — the retired type/ears_keyword are gone (S12 migrated them to
	// the canonical ears_pattern) — and it carries the required in_scope/
	// out_of_scope arrays. The reader exposes ears_pattern for internal/ears.
	specJSON := `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S01-test",
  "release": "test-release",
  "user_outcome": "The user gets the thing.",
  "in_scope": ["Read the record", "Expose in_scope/out_of_scope"],
  "out_of_scope": ["Writing records"],
  "covers_needs": ["N-01", "N-02"],
  "acceptance_criteria": [
    {"id": "AC-1", "ears_pattern": "ubiquitous", "text": "THE SYSTEM SHALL do X."},
    {"id": "AC-2", "ears_pattern": "event-driven", "text": "WHEN Y THE SYSTEM SHALL do Z."}
  ],
  "references": [
    {"kind":"contract","contract_id":"C-01"},
    {"kind":"slice","slice_id":"S02-consumer"},
    {"kind":"file","path":"docs/public-contract.md"}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(specJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	rec, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil {
		t.Fatal("want record, got nil")
	}
	if rec.SliceID != "S01-test" {
		t.Errorf("want slice_id S01-test, got %q", rec.SliceID)
	}
	if len(rec.CoversNeeds) != 2 || rec.CoversNeeds[0] != "N-01" {
		t.Errorf("want covers_needs [N-01 N-02], got %v", rec.CoversNeeds)
	}
	if len(rec.InScope) != 2 || rec.InScope[0] != "Read the record" {
		t.Errorf("want in_scope exposed, got %v", rec.InScope)
	}
	if len(rec.OutOfScope) != 1 || rec.OutOfScope[0] != "Writing records" {
		t.Errorf("want out_of_scope [Writing records], got %v", rec.OutOfScope)
	}
	if len(rec.AcceptanceCriteria) != 2 {
		t.Fatalf("want 2 ACs, got %d", len(rec.AcceptanceCriteria))
	}
	if rec.AcceptanceCriteria[1].EARSPattern != "event-driven" {
		t.Errorf("want ears_pattern event-driven, got %q", rec.AcceptanceCriteria[1].EARSPattern)
	}
	if rec.AcceptanceCriteria[0].Text != "THE SYSTEM SHALL do X." {
		t.Errorf("unexpected AC text %q", rec.AcceptanceCriteria[0].Text)
	}
	if len(rec.References) != 3 || rec.References[0].ContractID != "C-01" || rec.References[1].SliceID != "S02-consumer" || rec.References[2].Path != "docs/public-contract.md" {
		t.Errorf("typed references were not retained: %+v", rec.References)
	}
}

func TestReadRecord_MissingReturnsNilNil(t *testing.T) {
	rec, err := ReadRecord(t.TempDir())
	if err != nil {
		t.Fatalf("want nil error for missing spec.json, got %v", err)
	}
	if rec != nil {
		t.Fatalf("want nil record for missing spec.json, got %+v", rec)
	}
}

func TestReadRecord_MalformedFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := ReadRecord(dir)
	if err == nil {
		t.Fatal("want parse error for malformed spec.json, got nil")
	}
	if rec != nil {
		t.Fatalf("want nil record on parse error, got %+v", rec)
	}
}
