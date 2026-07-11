package implement

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/spec"
)

func TestWriteSpecRecord_ParsesSpecAndWritesJSON(t *testing.T) {
	dir := t.TempDir()

	// Create a spec.md with known content.
	spec := `---
title: Test slice
---

# Slice: S15-test-slice

## User outcome

The system writes spec.json with ACs and covers_needs.

## In scope

- Write spec.json

## Acceptance checks

- [ ] WHEN Run() completes, THE SYSTEM SHALL write spec.json (N-04)
- [x] spec.json covers_needs array contains at least one element (N-08)

## Required tests

- **Unit**: go test ./...

## Out of scope

- N/A
`
	specPath := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a minimal status.json with covers_needs.
	status := `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S15-test-slice",
  "release": "2026-06-27-test",
  "track": "T4-records-as-json",
  "state": "in_progress",
  "covers_needs": ["N-04", "N-08"],
  "verification": {"result": "pending"}
}`
	statusPath := filepath.Join(dir, "status.json")
	if err := os.WriteFile(statusPath, []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}

	sliceDir := dir
	if err := WriteSpecRecord(specPath, statusPath, sliceDir); err != nil {
		t.Fatalf("WriteSpecRecord: %v", err)
	}

	// Verify spec.json exists and has correct content.
	specJSONPath := filepath.Join(dir, "spec.json")
	data, err := os.ReadFile(specJSONPath)
	if err != nil {
		t.Fatalf("spec.json not created: %v", err)
	}

	var rec specRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("spec.json not valid JSON: %v", err)
	}

	if rec.Schema != baton.SpecSchemaURI {
		t.Errorf("$schema = %q, want %q", rec.Schema, baton.SpecSchemaURI)
	}
	if rec.SliceID != "S15-test-slice" {
		t.Errorf("slice_id = %q, want S15-test-slice", rec.SliceID)
	}
	if rec.Release != "2026-06-27-test" {
		t.Errorf("release = %q, want 2026-06-27-test", rec.Release)
	}
	if !strings.Contains(rec.UserOutcome, "writes spec.json") {
		t.Errorf("user_outcome = %q, want text about spec.json", rec.UserOutcome)
	}
	// in_scope/out_of_scope are scraped from the spec.md sections and must be
	// present (v0.10.0 spec-v1 requires them as arrays).
	if len(rec.InScope) != 1 || rec.InScope[0] != "Write spec.json" {
		t.Errorf("in_scope = %v, want [Write spec.json]", rec.InScope)
	}
	if len(rec.OutOfScope) != 1 || rec.OutOfScope[0] != "N/A" {
		t.Errorf("out_of_scope = %v, want [N/A]", rec.OutOfScope)
	}
	if len(rec.AcceptanceCriteria) != 2 {
		t.Fatalf("acceptance_criteria length = %d, want 2", len(rec.AcceptanceCriteria))
	}
	if rec.AcceptanceCriteria[0].ID != "AC-1" {
		t.Errorf("AC[0].id = %q, want AC-1", rec.AcceptanceCriteria[0].ID)
	}
	if !strings.Contains(rec.AcceptanceCriteria[0].Text, "write spec.json") {
		t.Errorf("AC[0].text = %q, want text about spec.json", rec.AcceptanceCriteria[0].Text)
	}
	if !strings.Contains(rec.AcceptanceCriteria[1].Text, "covers_needs") {
		t.Errorf("AC[1].text = %q, want text about covers_needs", rec.AcceptanceCriteria[1].Text)
	}
	if len(rec.CoversNeeds) != 2 {
		t.Errorf("covers_needs length = %d, want 2", len(rec.CoversNeeds))
	}
	if rec.CoversNeeds[0] != "N-04" || rec.CoversNeeds[1] != "N-08" {
		t.Errorf("covers_needs = %v, want [N-04 N-08]", rec.CoversNeeds)
	}
}

// TestWriteSpecRecord_RoundTripValidatesStrictSchema is the AC-03 writer→reader
// round trip: WriteSpecRecord's output must conform to the strict vendored
// v0.10.0 spec-v1 schema (draft-2020-12, additionalProperties:false), and the
// read side (internal/spec.ReadRecord) must parse and expose the in_scope /
// out_of_scope boundary the writer emitted. A conformant record carries NO
// schema_version and NO AC type/ears_keyword (all forbidden by the strict
// schema) and DOES carry in_scope/out_of_scope as arrays. This is the round
// trip the prior read-path-only normalise test never exercised.
func TestWriteSpecRecord_RoundTripValidatesStrictSchema(t *testing.T) {
	dir := t.TempDir()

	specMD := `# Slice: S21-round-trip

## User outcome

The writer emits a spec.json that conforms to strict v0.10.0 spec-v1.

## In scope

- Emit in_scope and out_of_scope
- Drop schema_version and AC type/ears_keyword

## Acceptance checks

- [ ] WHEN WriteSpecRecord runs, the record SHALL validate against spec-v1 (N-12)

## Out of scope

- Migrating live records
`
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(specMD), 0o644); err != nil {
		t.Fatal(err)
	}

	status := `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S21-round-trip",
  "release": "2026-06-28-driver-contract",
  "state": "in_progress",
  "covers_needs": ["N-12"],
  "verification": {"result": "pending"}
}`
	if err := os.WriteFile(filepath.Join(dir, "status.json"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteSpecRecord(filepath.Join(dir, "spec.md"), filepath.Join(dir, "status.json"), dir); err != nil {
		t.Fatalf("WriteSpecRecord: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "spec.json"))
	if err != nil {
		t.Fatalf("read written spec.json: %v", err)
	}

	// The written record must conform to the STRICT vendored v0.10.0 spec-v1
	// schema — no normalise shim on the write path, so the writer's own output
	// is the thing under test (unlike the read-path normalise fixture test).
	if err := baton.ValidateSchema("spec-v1", written); err != nil {
		t.Fatalf("written spec.json does not conform to strict spec-v1: %v\npayload: %s", err, written)
	}

	// The retired fields must be absent by key (additionalProperties:false makes
	// their presence a strict-validation failure; assert directly for a clear
	// signal on the flagged fields).
	var raw map[string]interface{}
	if err := json.Unmarshal(written, &raw); err != nil {
		t.Fatalf("re-parse written spec.json: %v", err)
	}
	if _, ok := raw["schema_version"]; ok {
		t.Error("written spec.json still carries schema_version")
	}
	if acs, ok := raw["acceptance_criteria"].([]interface{}); ok {
		for i, a := range acs {
			am, _ := a.(map[string]interface{})
			if _, ok := am["type"]; ok {
				t.Errorf("acceptance_criteria[%d] still carries type", i)
			}
			if _, ok := am["ears_keyword"]; ok {
				t.Errorf("acceptance_criteria[%d] still carries ears_keyword", i)
			}
		}
	}

	// Writer → reader round trip: the read side parses and EXPOSES the boundary.
	rec, err := spec.ReadRecord(dir)
	if err != nil {
		t.Fatalf("spec.ReadRecord: %v", err)
	}
	if rec == nil {
		t.Fatal("spec.ReadRecord returned nil record")
	}
	if len(rec.InScope) != 2 {
		t.Errorf("reader in_scope = %v, want 2 items", rec.InScope)
	}
	if len(rec.OutOfScope) != 1 || rec.OutOfScope[0] != "Migrating live records" {
		t.Errorf("reader out_of_scope = %v, want [Migrating live records]", rec.OutOfScope)
	}
}

func TestParseAcceptanceCriteria_ExcludesNotes(t *testing.T) {
	spec := `## Acceptance checks

- [ ] WHEN Run() completes, THE SYSTEM SHALL write spec.json
- [ ] NOTE: This is an informational note, not an AC
- [ ] Another real AC
`
	acs := parseAcceptanceCriteria(spec)
	if len(acs) != 2 {
		t.Fatalf("got %d ACs, want 2", len(acs))
	}
	if acs[0].ID != "AC-1" {
		t.Errorf("AC[0].id = %q, want AC-1", acs[0].ID)
	}
	if acs[1].ID != "AC-2" {
		t.Errorf("AC[1].id = %q, want AC-2", acs[1].ID)
	}
}

func TestExtractUserOutcome(t *testing.T) {
	spec := `## User outcome

The system does X.

## In scope`
	outcome := extractUserOutcome(spec)
	if outcome != "The system does X." {
		t.Errorf("user_outcome = %q, want 'The system does X.'", outcome)
	}
}

func TestClassifyEARSKeyword(t *testing.T) {
	tests := []struct {
		ac   string
		want string
	}{
		{"WHEN Run() completes, THE SYSTEM SHALL write spec.json", "When"},
		{"WHILE the system is running, it shall log events", "While"},
		{"WHERE the user is authenticated, the system shall allow access", "Where"},
		{"IF the user is premium THEN the system shall unlock the feature", "If"},
		{"The system shall validate input", "Ubiquitous"},
	}
	for _, tt := range tests {
		got := classifyEARSKeyword(tt.ac)
		if got != tt.want {
			t.Errorf("classifyEARSKeyword(%q) = %q, want %q", tt.ac, got, tt.want)
		}
	}
}
