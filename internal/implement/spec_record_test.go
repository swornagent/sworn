package implement

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
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
  "need_ids": ["N-04", "N-08"],
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
	if rec.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", rec.SchemaVersion)
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
	if len(rec.AcceptanceCriteria) != 2 {
		t.Fatalf("acceptance_criteria length = %d, want 2", len(rec.AcceptanceCriteria))
	}
	if rec.AcceptanceCriteria[0].ID != "AC-1" {
		t.Errorf("AC[0].id = %q, want AC-1", rec.AcceptanceCriteria[0].ID)
	}
	if !strings.Contains(rec.AcceptanceCriteria[0].Text, "write spec.json") {
		t.Errorf("AC[0].text = %q, want text about spec.json", rec.AcceptanceCriteria[0].Text)
	}
	if rec.AcceptanceCriteria[0].EARSKeyword != "When" {
		t.Errorf("AC[0].ears_keyword = %q, want When", rec.AcceptanceCriteria[0].EARSKeyword)
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