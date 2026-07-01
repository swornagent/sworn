package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/journey"
)

// TestJourneysRegenCmd_CoverageGapFilled verifies AC1 via CLI:
// WHEN a journey is ratified + walked but flagged for regression with no
// committed test, sworn journeys --regen generates the scaffold but exits 1
// because gaps existed at run start (Option A: fail-closed on pre-codification
// state). The message reports the gap was filled but exit is still 1.
func TestJourneysRegenCmd_CoverageGapFilled(t *testing.T) {
	dir := t.TempDir()

	// Create a ratified journeys artefact with one walked-pass journey
	// that has no regression test.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice via sworn verify",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	// Create a matching attestation with WalkPass.
	attArtefact := &journey.AttestationArtefact{
		Version: 1,
		Attestations: []journey.Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    journey.WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	saveAttestations(t, dir, attArtefact)

	// Capture stdout.
	savedStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	exit := cmdJourneys([]string{"--regen", "test-release", dir})

	w.Close()
	os.Stdout = savedStdout

	stdout := readAllPipe(t, r)

	// The journey had no test — gaps existed at run start, so exit 1
	// even though regen fills the gaps (Option A).
	if exit != 1 {
		t.Errorf("expected exit 1 (gaps at run start), got %d", exit)
	}
	if !strings.Contains(stdout, "Generated 1 regression test scaffold") {
		t.Errorf("expected stdout to indicate scaffold generation, got:\n%s", stdout)
	}
	// Verify the scaffold was created.
	expectedPath := filepath.Join(dir, "tests", "e2e", "journeys", "journey_j01_verify_flow_test.go")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected scaffold at %s, but it doesn't exist", expectedPath)
	}
}

// TestJourneysRegenCmd_FullCoverage verifies AC1 + AC3 via CLI:
// WHEN all walked journeys have regression coverage, sworn journeys --regen
// exits 0 and reports full coverage.
func TestJourneysRegenCmd_FullCoverage(t *testing.T) {
	dir := t.TempDir()

	// Create a ratified journeys artefact with a walked-pass journey
	// that already has HasRegression=true.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:                 "J01-verify-flow",
		UserType:           "developer",
		Outcome:            "Verify a slice",
		HasRegression:      true,
		RegressionTestPath: "tests/e2e/journeys/journey_j01_verify_flow_test.go",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	// Create a matching attestation with WalkPass.
	attArtefact := &journey.AttestationArtefact{
		Version: 1,
		Attestations: []journey.Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    journey.WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	saveAttestations(t, dir, attArtefact)

	// Capture output.
	savedStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	exit := cmdJourneys([]string{"--regen", "test-release", dir})

	w.Close()
	os.Stdout = savedStdout

	stdout := readAllPipe(t, r)

	if exit != 0 {
		t.Errorf("expected exit 0 for full coverage, got %d", exit)
	}
	if !strings.Contains(stdout, "No new regression scaffolds") {
		t.Errorf("expected stdout to indicate full coverage, got:\n%s", stdout)
	}
}

// TestJourneysRegenCmd_ScaffoldEmission verifies AC2 via CLI:
// WHEN sworn journeys --regen runs for a walked journey without coverage,
// THE SYSTEM SHALL emit a regression test scaffold at the expected path.
func TestJourneysRegenCmd_ScaffoldEmission(t *testing.T) {
	dir := t.TempDir()

	// Create a ratified journeys artefact with one walked-pass journey
	// that has no regression test.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J02-init-setup",
		UserType: "new_user",
		Outcome:  "Set up the tool for the first time",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Install the tool", Surface: "CLI"},
			{Order: 2, Description: "Run init to bootstrap configuration", Surface: "sworn init"},
			{Order: 3, Description: "Verify the setup with a smoke test", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	// Create a matching attestation with WalkPass.
	attArtefact := &journey.AttestationArtefact{
		Version: 1,
		Attestations: []journey.Attestation{
			{
				JourneyID: "J02-init-setup",
				Status:    journey.WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	saveAttestations(t, dir, attArtefact)

	exit := cmdJourneys([]string{"--regen", "test-release", dir})
	if exit != 1 {
		t.Errorf("expected exit 1 (gaps at run start), got %d", exit)
	}
	// The scaffold file should exist at the expected path.
	expectedPath := filepath.Join(dir, "tests", "e2e", "journeys", "journey_j02_init_setup_test.go")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected scaffold file at %s, but it does not exist", expectedPath)
	}

	// The artefact should have been updated with HasRegression + RegressionTestPath.
	updated, err := journey.LoadArtefact(dir)
	if err != nil {
		t.Fatalf("load updated artefact: %v", err)
	}
	if len(updated.Journeys) != 1 {
		t.Fatalf("expected 1 journey in artefact, got %d", len(updated.Journeys))
	}
	if !updated.Journeys[0].HasRegression {
		t.Error("expected HasRegression=true after codification")
	}
	if updated.Journeys[0].RegressionTestPath == "" {
		t.Error("expected RegressionTestPath to be set after codification")
	}

	// Verify the scaffold content includes journey steps.
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "J02-init-setup") {
		t.Errorf("expected scaffold to reference journey ID")
	}
	if !strings.Contains(content, "Install the tool") {
		t.Errorf("expected scaffold to include journey steps")
	}
}

// TestJourneysRegenCmd_UnwalkedJourneyNotCodified verifies AC4 via CLI:
// THE SYSTEM SHALL only codify journeys with a passing walkthrough — an
// un-walked journey is not auto-codified.
func TestJourneysRegenCmd_UnwalkedJourneyNotCodified(t *testing.T) {
	dir := t.TempDir()

	// Create a ratified journeys artefact with two journeys — one walked,
	// one un-walked.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-walked",
		UserType: "developer",
		Outcome:  "Walked journey",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Step one", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	a.AddJourney(journey.Journey{
		ID:       "J02-unwalked",
		UserType: "developer",
		Outcome:  "Un-walked journey",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Step two", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	// Only J01 has a WalkPass attestation.
	attArtefact := &journey.AttestationArtefact{
		Version: 1,
		Attestations: []journey.Attestation{
			{
				JourneyID: "J01-walked",
				Status:    journey.WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}
	saveAttestations(t, dir, attArtefact)

	exit := cmdJourneys([]string{"--regen", "test-release", dir})
	if exit != 1 {
		t.Errorf("expected exit 1 (gaps at run start from walked-but-uncovered J01), got %d", exit)
	}
	// Only the walked journey should get a scaffold.
	walkedPath := filepath.Join(dir, "tests", "e2e", "journeys", "journey_j01_walked_test.go")
	unwalkedPath := filepath.Join(dir, "tests", "e2e", "journeys", "journey_j02_unwalked_test.go")

	if _, err := os.Stat(walkedPath); os.IsNotExist(err) {
		t.Errorf("expected scaffold for walked journey J01-walked at %s", walkedPath)
	}
	if _, err := os.Stat(unwalkedPath); !os.IsNotExist(err) {
		t.Errorf("expected NO scaffold for un-walked journey J02-unwalked at %s", unwalkedPath)
	}

	// Verify artefact status.
	updated, err := journey.LoadArtefact(dir)
	if err != nil {
		t.Fatalf("load updated artefact: %v", err)
	}
	for _, j := range updated.Journeys {
		switch j.ID {
		case "J01-walked":
			if !j.HasRegression {
				t.Error("expected J01-walked.HasRegression=true")
			}
		case "J02-unwalked":
			if j.HasRegression {
				t.Error("expected J02-unwalked.HasRegression=false (un-walked)")
			}
		}
	}
}

// readAllPipe reads all bytes from a pipe until EOF.
func readAllPipe(t *testing.T, r *os.File) string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(data)
}

// saveAttestations saves an attestation artefact to the project root.
func saveAttestations(t *testing.T, projectRoot string, a *journey.AttestationArtefact) {
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
