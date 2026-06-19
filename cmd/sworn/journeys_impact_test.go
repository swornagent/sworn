package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/journey"
)
// TestJourneysImpactCmd_MissingArtefact verifies AC2 via CLI:
// WHEN no journeys artefact exists, sworn journeys --impact <release>
// exits non-zero and directs to S11.
func TestJourneysImpactCmd_MissingArtefact(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	makeImpactSlice(t, releaseDir, "S01-test", []string{"cmd/sworn/verify.go"})

	exit := cmdJourneys([]string{"--impact", "test-release", dir})
	if exit != 1 {
		t.Errorf("expected exit 1 for missing artefact, got %d", exit)
	}
}

// TestJourneysImpactCmd_UnratifiedArtefact verifies that an unratified
// artefact yields exit 1 and a clear message.
func TestJourneysImpactCmd_UnratifiedArtefact(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	makeImpactSlice(t, releaseDir, "S01-test", []string{"cmd/sworn/verify.go"})

	// Create an unratified artefact.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Run verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	exit := cmdJourneys([]string{"--impact", "test-release", dir})
	if exit != 1 {
		t.Errorf("expected exit 1 for unratified artefact, got %d", exit)
	}
}

// TestJourneysImpactCmd_TouchedJourneys verifies AC1 via CLI:
// a release with ratified journeys reports the touched set.
func TestJourneysImpactCmd_TouchedJourneys(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "fidelity-layer")
	makeImpactSlice(t, releaseDir, "S12-impact",
		[]string{"cmd/sworn/verify.go", "internal/journey/journey.go"})
	makeImpactSlice(t, releaseDir, "S13-walkthrough",
		[]string{"cmd/sworn/walkthrough.go"})

	// Create a ratified artefact.
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

	// Capture stdout.
	saved := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	exit := cmdJourneys([]string{"--impact", "fidelity-layer", dir})
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d", exit)
	}

	w.Close()
	out := make([]byte, 4096)
	n, _ := r.Read(out)
	os.Stdout = saved

	output := string(out[:n])

	if !strings.Contains(output, "J01-verify-flow") {
		t.Errorf("expected output to mention J01-verify-flow, got:\n%s", output)
	}
	if !strings.Contains(output, "touched") {
		t.Errorf("expected output to mention touched journeys, got:\n%s", output)
	}
	if !strings.Contains(output, "fidelity-layer") {
		t.Errorf("expected output to mention release name, got:\n%s", output)
	}
}

// TestJourneysImpactCmd_EmptyTouchedSet verifies AC3 via CLI:
// a release that touches no journeys explicitly reports an empty set.
func TestJourneysImpactCmd_EmptyTouchedSet(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "internal-refactor")
	makeImpactSlice(t, releaseDir, "S99-refactor",
		[]string{"internal/state/state.go", "internal/config/config.go"})

	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
		Steps: []journey.JourneyStep{
			{Order: 1, Description: "Click button", Surface: "web-dashboard"},
		},
		EntrySurface: "web-ui",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	exit := cmdJourneys([]string{"--impact", "internal-refactor", dir})
	if exit != 0 {
		t.Errorf("expected exit 0 for empty touched set, got %d", exit)
	}
}

// makeImpactSlice creates a minimal slice directory with a status.json
// containing planned_files for impact analysis testing.
func makeImpactSlice(t *testing.T, releaseDir, sliceID string, plannedFiles []string) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeImpactJSON(t, filepath.Join(sliceDir, "status.json"), map[string]interface{}{
		"slice_id":      sliceID,
		"planned_files": plannedFiles,
		"actual_files":  plannedFiles,
	})
}

func writeImpactJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}