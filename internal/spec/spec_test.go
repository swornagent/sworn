package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRecord_ParsesSpecV1(t *testing.T) {
	dir := t.TempDir()
	specJSON := `{
  "schema_version": 1,
  "slice_id": "S01-test",
  "release": "test-release",
  "user_outcome": "The user gets the thing.",
  "covers_needs": ["N-01", "N-02"],
  "acceptance_criteria": [
    {"id": "AC-1", "type": "ubiquitous", "text": "THE SYSTEM SHALL do X."},
    {"id": "AC-2", "type": "event-driven", "ears_keyword": "When", "text": "WHEN Y THE SYSTEM SHALL do Z."}
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
	if len(rec.AcceptanceCriteria) != 2 {
		t.Fatalf("want 2 ACs, got %d", len(rec.AcceptanceCriteria))
	}
	if rec.AcceptanceCriteria[1].EARSKeyword != "When" {
		t.Errorf("want ears_keyword When, got %q", rec.AcceptanceCriteria[1].EARSKeyword)
	}
	if rec.AcceptanceCriteria[0].Text != "THE SYSTEM SHALL do X." {
		t.Errorf("unexpected AC text %q", rec.AcceptanceCriteria[0].Text)
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
