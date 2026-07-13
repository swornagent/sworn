package journey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAttestations_MissingFile(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadAttestations(dir)
	if err != nil {
		t.Fatalf("LoadAttestations on empty dir: %v", err)
	}
	if a.Version != 1 {
		t.Errorf("expected version 1, got %d", a.Version)
	}
	if len(a.Attestations) != 0 {
		t.Errorf("expected empty attestations, got %d", len(a.Attestations))
	}
	if a.Schema == "" {
		t.Error("expected non-empty $schema")
	}
}

func TestLoadAttestations_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	ensureSwornDir(t, dir)
	path := AttestationArtefactPath(dir)
	data := `{
		"$schema": "https://baton.sawy3r.net/schemas/attestations-v1.json",
		"version": 1,
		"ratification": {
			"by": "brad",
			"at": "2026-01-01T00:00:00Z",
			"is_ratified": true
		},
		"boundary": {
			"name": "production",
			"mock_banned": true,
			"entitlement_boundary": "full"
		},
		"attestations": [
			{
				"journey_id": "J01-onboard-new-user",
				"status": "walked-pass",
				"walked_by": "brad",
				"walked_at": "2026-01-01T00:00:00Z",
				"real_infra": true,
				"mocks_off": true
			},
			{
				"journey_id": "J-develop-feature",
				"status": "walked-fail",
				"walked_by": "brad",
				"walked_at": "2026-01-01T00:00:00Z",
				"real_infra": true,
				"mocks_off": true,
				"notes": "Found a regression in the output format"
			}
		]
	}`

	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	a, err := LoadAttestations(dir)
	if err != nil {
		t.Fatalf("LoadAttestations: %v", err)
	}
	if a.Version != 1 {
		t.Errorf("expected version 1, got %d", a.Version)
	}
	if len(a.Attestations) != 2 {
		t.Fatalf("expected 2 attestations, got %d", len(a.Attestations))
	}

	// Verify first attestation.
	a0 := a.Attestations[0]
	if a0.JourneyID != "J01-onboard-new-user" {
		t.Errorf("expected J01-onboard-new-user, got %s", a0.JourneyID)
	}
	if a0.Status != WalkPass {
		t.Errorf("expected WalkPass, got %s", a0.Status)
	}
	if a0.WalkedBy != "brad" {
		t.Errorf("expected brad, got %s", a0.WalkedBy)
	}
	if !a0.RealInfra {
		t.Error("expected RealInfra true")
	}
	if !a0.MocksOff {
		t.Error("expected MocksOff true")
	}

	// Verify second attestation.
	a1 := a.Attestations[1]
	if a1.JourneyID != "J-develop-feature" {
		t.Errorf("expected J-develop-feature, got %s", a1.JourneyID)
	}
	if a1.Status != WalkFail {
		t.Errorf("expected WalkFail, got %s", a1.Status)
	}
}

func TestLoadAttestations_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	ensureSwornDir(t, dir)
	path := AttestationArtefactPath(dir)
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAttestations(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestAttestationStatus_NoArtefact(t *testing.T) {
	status := AttestationStatus(nil, "J01-onboard-new-user")
	if status != WalkUnwalked {
		t.Errorf("expected WalkUnwalked, got %s", status)
	}
}

func TestAttestationStatus_NoMatch(t *testing.T) {
	a := &AttestationArtefact{
		Schema:  "https://baton.sawy3r.net/schemas/attestations-v1.json",
		Version: 1,
		Ratification: Ratification{
			By:         "",
			At:         "",
			IsRatified: false,
		},
		Boundary: Boundary{
			Name:                "production",
			MockBanned:          true,
			EntitlementBoundary: "full",
		},
		Attestations: []Attestation{
			{JourneyID: "J01-onboard-new-user", Status: WalkPass},
		},
	}
	status := AttestationStatus(a, "J-nonexistent")
	if status != WalkUnwalked {
		t.Errorf("expected WalkUnwalked for unknown journey, got %s", status)
	}
}

func TestAttestationStatus_Match(t *testing.T) {
	a := &AttestationArtefact{
		Schema:  "https://baton.sawy3r.net/schemas/attestations-v1.json",
		Version: 1,
		Ratification: Ratification{
			By:         "",
			At:         "",
			IsRatified: false,
		},
		Boundary: Boundary{
			Name:                "production",
			MockBanned:          true,
			EntitlementBoundary: "full",
		},
		Attestations: []Attestation{
			{JourneyID: "J01-onboard-new-user", Status: WalkPass},
			{JourneyID: "J-develop-feature", Status: WalkFail},
		},
	}

	if got := AttestationStatus(a, "J01-onboard-new-user"); got != WalkPass {
		t.Errorf("expected WalkPass, got %s", got)
	}
	if got := AttestationStatus(a, "J-develop-feature"); got != WalkFail {
		t.Errorf("expected WalkFail, got %s", got)
	}
}

func TestAttestationArtefactPath(t *testing.T) {
	path := AttestationArtefactPath("/tmp/myproject")
	expected := filepath.Join("/tmp/myproject", ".sworn", "attestations.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestSaveAttestations_NestedShapeRoundtrip(t *testing.T) {
	dir := t.TempDir()

	a := &AttestationArtefact{
		Schema:  "https://baton.sawy3r.net/schemas/attestations-v1.json",
		Version: 1,
		Ratification: Ratification{
			By:         "maintainer@sworn.sh",
			At:         "2026-01-01T00:00:00Z",
			IsRatified: true,
		},
		Boundary: Boundary{
			Name:                "production",
			MockBanned:          true,
			EntitlementBoundary: "full",
		},
		Attestations: []Attestation{
			{
				JourneyID: "J01-onboard-new-user",
				Status:    WalkPass,
				WalkedBy:  "brad",
				WalkedAt:  "2026-01-01T00:00:00Z",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	if err := SaveAttestations(dir, a); err != nil {
		t.Fatalf("SaveAttestations: %v", err)
	}

	// Read back the raw JSON to verify nested shape.
	data, err := os.ReadFile(AttestationArtefactPath(dir))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify $schema.
	schema, ok := raw["$schema"].(string)
	if !ok || schema == "" {
		t.Error("expected non-empty $schema field")
	}

	// Verify ratification is nested.
	rat, ok := raw["ratification"].(map[string]interface{})
	if !ok {
		t.Fatal("ratification must be a nested object")
	}
	if rat["by"] != "maintainer@sworn.sh" {
		t.Errorf("expected ratification.by 'maintainer@sworn.sh', got %q", rat["by"])
	}

	// Verify boundary is nested.
	bound, ok := raw["boundary"].(map[string]interface{})
	if !ok {
		t.Fatal("boundary must be a nested object")
	}
	if bound["name"] != "production" {
		t.Errorf("expected boundary.name 'production', got %q", bound["name"])
	}

	// Verify round-trip load.
	loaded, err := LoadAttestations(dir)
	if err != nil {
		t.Fatalf("LoadAttestations round-trip: %v", err)
	}
	if loaded.Version != 1 {
		t.Errorf("expected version 1, got %d", loaded.Version)
	}
	if len(loaded.Attestations) != 1 {
		t.Fatalf("expected 1 attestation, got %d", len(loaded.Attestations))
	}
	if loaded.Attestations[0].JourneyID != "J01-onboard-new-user" {
		t.Errorf("expected J01-onboard-new-user, got %s", loaded.Attestations[0].JourneyID)
	}
}

func TestSaveAttestations_ValidateOnWrite(t *testing.T) {
	dir := t.TempDir()

	// Create an artefact with missing $schema — SaveAttestations should
	// auto-populate it and succeed.
	a := &AttestationArtefact{
		Schema:  "",
		Version: 1,
		Ratification: Ratification{
			By:         "brad",
			At:         "2026-01-01T00:00:00Z",
			IsRatified: true,
		},
		Boundary: Boundary{
			Name:                "production",
			MockBanned:          true,
			EntitlementBoundary: "full",
		},
		Attestations: []Attestation{
			{
				JourneyID: "J01-test",
				Status:    WalkPass,
				WalkedBy:  "brad",
				WalkedAt:  "2026-01-01T00:00:00Z",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	// Missing $schema: SaveAttestations should auto-populate and succeed.
	if err := SaveAttestations(dir, a); err != nil {
		t.Fatalf("SaveAttestations should succeed with auto-populated $schema: %v", err)
	}

	// Verify the file was written and has $schema.
	data, err := os.ReadFile(AttestationArtefactPath(dir))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if schema, ok := raw["$schema"].(string); !ok || schema == "" {
		t.Error("expected non-empty $schema after SaveAttestations")
	}
}

func ensureSwornDir(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, ".sworn")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
}
