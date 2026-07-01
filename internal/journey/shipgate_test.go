package journey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestShipGate_MissingJourneysArtefact verifies the ship gate fails closed when
// no journeys artefact exists.
func TestShipGate_MissingJourneysArtefact(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S14-test", []string{"cmd/sworn/verify.go"}, nil)

	result, err := CheckShipGate(projectRoot, releaseDir)
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if err == nil {
		t.Fatal("expected error for missing journeys artefact, got nil")
	}
}

// TestShipGate_UnratifiedJourneysArtefact verifies the ship gate fails closed
// when the journeys artefact exists but is unratified.
func TestShipGate_UnratifiedJourneysArtefact(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S14-test", []string{"cmd/sworn/verify.go"}, nil)

	// Create an unratified artefact.
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run test", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	result, err := CheckShipGate(projectRoot, releaseDir)
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	if err == nil {
		t.Fatal("expected error for unratified artefact, got nil")
	}
}

// TestShipGate_AllTouchedJourneysAttested verifies AC3: WHEN every touched
// journey has a passing human attestation asserting real-infra + mocks-off,
// THE SYSTEM SHALL allow verified -> shipped.
func TestShipGate_AllTouchedJourneysAttested(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S13-walkthrough",
		[]string{"cmd/sworn/verify.go", "internal/journey/journey.go"}, nil)

	// Create and ratify journeys artefact.
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	// Create a passing attestation for the only touched journey.
	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkPass,
				WalkedBy:  "brad",
				WalkedAt:  "2026-06-26T10:00:00Z",
				RealInfra: true,
				MocksOff:  true,
				Notes:     "Walked against staging, all good.",
			},
		},
	}
	if err := saveAttestationsArtefact(projectRoot, attArtefact); err != nil {
		t.Fatal(err)
	}

	result, err := CheckShipGate(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Pass {
		t.Errorf("expected Pass=true, got false. Blocked: %v", result.BlockedReasons)
	}
	if len(result.BlockedReasons) != 0 {
		t.Errorf("expected no blocked reasons, got %v", result.BlockedReasons)
	}
}

// TestShipGate_UnwalkedJourneyBlocks verifies AC1: WHEN a journey in the
// release's validation scope has no human-walkthrough attestation, THE SYSTEM
// SHALL block the ship gate and name the un-walked journey.
func TestShipGate_UnwalkedJourneyBlocks(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	// Two slices touching two different journey surfaces.
	makeSliceDir(t, releaseDir, "S13-walkthrough",
		[]string{"cmd/sworn/verify.go", "cmd/sworn/init.go"}, nil)

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	a.AddJourney(Journey{
		ID:       "J02-init-setup",
		UserType: "new_user",
		Outcome:  "Set up the tool for the first time",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run init", Surface: "init"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	// Only attest J01, not J02.
	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkPass,
				WalkedBy:  "brad",
				WalkedAt:  "2026-06-26T10:00:00Z",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	if err := saveAttestationsArtefact(projectRoot, attArtefact); err != nil {
		t.Fatal(err)
	}

	result, err := CheckShipGate(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass {
		t.Fatal("expected Pass=false for un-walked journey, got true")
	}

	unwalked := result.UnwalkedJourneys()
	if len(unwalked) != 1 {
		t.Fatalf("expected 1 un-walked journey, got %v", unwalked)
	}
	if unwalked[0] != "J02-init-setup" {
		t.Errorf("expected J02-init-setup as un-walked, got %v", unwalked)
	}
}

// TestShipGate_FailedWalkthroughBlocks verifies AC2: WHEN a touched journey's
// attestation records a failed walkthrough, THE SYSTEM SHALL block cutover and
// name it in the kill-list.
func TestShipGate_FailedWalkthroughBlocks(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S13-walkthrough",
		[]string{"cmd/sworn/verify.go"}, nil)

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	// Attestation records a FAILED walkthrough.
	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkFail,
				WalkedBy:  "brad",
				WalkedAt:  "2026-06-26T10:00:00Z",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	if err := saveAttestationsArtefact(projectRoot, attArtefact); err != nil {
		t.Fatal(err)
	}

	result, err := CheckShipGate(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass {
		t.Fatal("expected Pass=false for failed walkthrough, got true")
	}

	incomplete := result.IncompleteJourneys()
	if len(incomplete) != 1 {
		t.Fatalf("expected 1 incomplete journey, got %v", incomplete)
	}
}

// TestShipGate_ModelCannotAuthorAttestation verifies AC4: THE SYSTEM SHALL NOT
// permit the model to author a walkthrough attestation; the walked-by-human
// field is mandatory and human-set.
func TestShipGate_ModelCannotAuthorAttestation(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S13-walkthrough",
		[]string{"cmd/sworn/verify.go"}, nil)

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	// Attestation with empty WalkedBy — model-authored (rejected).
	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkPass,
				WalkedBy:  "", // empty — model-authored
				WalkedAt:  "2026-06-26T10:00:00Z",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	if err := saveAttestationsArtefact(projectRoot, attArtefact); err != nil {
		t.Fatal(err)
	}

	result, err := CheckShipGate(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pass {
		t.Fatal("expected Pass=false for model-authored attestation, got true")
	}

	incomplete := result.IncompleteJourneys()
	if len(incomplete) != 1 {
		t.Fatalf("expected 1 incomplete journey, got %v", incomplete)
	}
}

// TestShipGate_MissingAssertionsBlocks verifies AC5: THE SYSTEM SHALL require
// both the real-infra and mocks-off assertions on each attestation; an
// attestation missing either is incomplete and blocks cutover.
func TestShipGate_MissingAssertionsBlocks(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S13-walkthrough",
		[]string{"cmd/sworn/verify.go"}, nil)

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		realInfra bool
		mocksOff  bool
	}{
		{"missing real_infra", false, true},
		{"missing mocks_off", true, false},
		{"missing both", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attArtefact := &AttestationArtefact{
				Version: 1,
				Attestations: []Attestation{
					{
						JourneyID: "J01-verify-flow",
						Status:    WalkPass,
						WalkedBy:  "brad",
						WalkedAt:  "2026-06-26T10:00:00Z",
						RealInfra: tt.realInfra,
						MocksOff:  tt.mocksOff,
					},
				},
			}
			if err := saveAttestationsArtefact(projectRoot, attArtefact); err != nil {
				t.Fatal(err)
			}

			result, err := CheckShipGate(projectRoot, releaseDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Pass {
				t.Fatal("expected Pass=false for missing assertions, got true")
			}
		})
	}
}

// TestShipGate_EmptyTouchedSetPasses verifies that a release touching no
// journeys passes the ship gate (nothing to attest).
func TestShipGate_EmptyTouchedSetPasses(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "internal-refactor")
	// Internal-only paths with no journey surfaces.
	makeSliceDir(t, releaseDir, "S99-refactor",
		[]string{"internal/config/config.go"}, nil)

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
		Steps: []JourneyStep{
			{Order: 1, Description: "Do thing", Surface: "web-dashboard"},
		},
		EntrySurface: "web-ui",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	result, err := CheckShipGate(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Pass {
		t.Errorf("expected Pass=true for empty touched set, got false")
	}
}

// --- helpers ---

// saveAttestationsArtefact writes an AttestationArtefact to the attestations
// path under the given project root.
func saveAttestationsArtefact(projectRoot string, a *AttestationArtefact) error {
	path := AttestationArtefactPath(projectRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
