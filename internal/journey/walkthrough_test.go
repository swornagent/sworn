package journey

import (
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
}

func TestLoadAttestations_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	ensureSwornDir(t, dir)
	path := AttestationArtefactPath(dir)
	data := `{
		"version": 1,
		"attestations": [
			{
				"journey_id": "J01-onboard-new-user",
				"status": "walked-pass",
				"walked_by": "brad",
				"real_infra": true,
				"mocks_off": true
			},
			{
				"journey_id": "J-develop-feature",
				"status": "walked-fail",
				"walked_by": "brad",
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
		Version: 1,
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
		Version: 1,
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

func ensureSwornDir(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, ".sworn")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
}
