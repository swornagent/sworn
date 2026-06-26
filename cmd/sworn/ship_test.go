package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/journey"
)

// TestShipCmd_MissingReleaseArg verifies usage error when no release is given.
func TestShipCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdShip([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release, got %d", exit)
	}
}

// TestShipCmd_NoJourneys verifies the gate blocks with exit 2 when no journeys
// artefact exists.
func TestShipCmd_NoJourneys(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	makeShipSlice(t, releaseDir, "S13-walkthrough", []string{"cmd/sworn/verify.go"})

	exit := cmdShip([]string{"test-release", dir})
	if exit != 2 {
		t.Errorf("expected exit 2 for missing journeys, got %d", exit)
	}
}

// TestShipCmd_UnratifiedJourneys verifies the gate blocks when journeys
// artefact exists but is unratified.
func TestShipCmd_UnratifiedJourneys(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	makeShipSlice(t, releaseDir, "S13-walkthrough", []string{"cmd/sworn/verify.go"})

	// Create unratified journeys artefact.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-test",
		UserType: "developer",
		Outcome:  "Test",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Do it", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	exit := cmdShip([]string{"test-release", dir})
	if exit != 2 {
		t.Errorf("expected exit 2 for unratified journeys, got %d", exit)
	}
}

// TestShipCmd_AllTouchedAttested verifies the ship gate passes when all
// touched journeys have complete, passing attestations.
func TestShipCmd_AllTouchedAttested(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	makeShipSlice(t, releaseDir, "S13-walkthrough", []string{"cmd/sworn/verify.go"})

	// Create ratified journeys artefact.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	// Create passing attestation.
	att := &journey.AttestationArtefact{
		Version: 1,
		Attestations: []journey.Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    journey.WalkPass,
				WalkedBy:  "brad",
				WalkedAt:  "2026-06-26T10:00:00Z",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	saveShipAttestations(t, dir, att)

	exit := cmdShip([]string{"test-release", dir})
	if exit != 0 {
		t.Errorf("expected exit 0 when all journeys attested, got %d", exit)
	}
}

// TestShipCmd_UnwalkedJourneyBlocks verifies the ship gate blocks and names
// un-walked journeys.
func TestShipCmd_UnwalkedJourneyBlocks(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	// Two slices touching two different journeys.
	makeShipSlice(t, releaseDir, "S13-walkthrough", []string{"cmd/sworn/verify.go", "cmd/sworn/init.go"})

	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Run verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	a.AddJourney(journey.Journey{
		ID:       "J02-init-setup",
		UserType: "developer",
		Outcome:  "Init",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Run init", Surface: "init"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	// Attest J01 only.
	att := &journey.AttestationArtefact{
		Version: 1,
		Attestations: []journey.Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    journey.WalkPass,
				WalkedBy:  "brad",
				WalkedAt:  "2026-06-26T10:00:00Z",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	saveShipAttestations(t, dir, att)

	exit := cmdShip([]string{"test-release", dir})
	if exit != 1 {
		t.Errorf("expected exit 1 for un-walked journey, got %d", exit)
	}
}

// --- helpers ---

func makeShipSlice(t *testing.T, releaseDir, sliceID string, plannedFiles []string) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeShipJSON(t, filepath.Join(sliceDir, "status.json"), map[string]interface{}{
		"slice_id":      sliceID,
		"planned_files": plannedFiles,
		"actual_files":  plannedFiles,
	})
}

func writeShipJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func saveShipAttestations(t *testing.T, projectRoot string, a *journey.AttestationArtefact) {
	t.Helper()
	path := journey.AttestationArtefactPath(projectRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// Ensure imports are used.
var _ = strings.Contains
