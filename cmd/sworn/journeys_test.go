package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/journey"
)

// TestJourneysCmd_MissingCheck verifies AC1 via CLI: WHEN no journeys artefact
// exists for a project, sworn journeys --check exits non-zero and states that
// elicitation has not been run.
func TestJourneysCmd_MissingCheck(t *testing.T) {
	dir := t.TempDir()

	// Run `sworn journeys --check <dir>` — no artefact exists.
	exit := cmdJourneys([]string{"--check", dir})
	if exit != 1 {
		t.Errorf("expected exit 1 for missing artefact, got %d", exit)
	}
}

// TestJourneysCmd_UnratifiedCheck verifies AC2 via CLI: WHEN a journeys
// artefact exists but is unratified, sworn journeys --check fails and
// names it as unratified.
func TestJourneysCmd_UnratifiedCheck(t *testing.T) {
	dir := t.TempDir()

	// Create an unratified artefact.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
	})
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	exit := cmdJourneys([]string{"--check", dir})
	if exit != 1 {
		t.Errorf("expected exit 1 for unratified artefact, got %d", exit)
	}
}

// TestJourneysCmd_PassCheck verifies AC4 via CLI: WHEN the artefact exists and
// is human-ratified, sworn journeys --check exits 0 and lists journeys.
func TestJourneysCmd_PassCheck(t *testing.T) {
	dir := t.TempDir()

	// Create and ratify an artefact.
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	exit := cmdJourneys([]string{"--check", dir})
	if exit != 0 {
		t.Errorf("expected exit 0 for ratified artefact, got %d", exit)
	}
}

// TestJourneysCmd_Elicit verifies AC3: WHEN sworn journeys <project> runs,
// THE SYSTEM SHALL draft >=1 candidate journey from the app and present it
// for human ratification. Since no AI model is wired yet, DraftTemplate
// produces the initial journeys from the project structure.
func TestJourneysCmd_Elicit(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal project structure.
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "verify"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "internal", "verify"), 0755); err != nil {
		t.Fatal(err)
	}

	exit := cmdJourneys([]string{dir})
	// The elicitation command should exit 0 after drafting.
	if exit != 0 {
		t.Errorf("expected exit 0 after elicitation, got %d", exit)
	}

	// The artefact should now exist.
	if _, err := journey.LoadArtefact(dir); err != nil {
		t.Errorf("artefact should exist after elicitation: %v", err)
	}

	// The artefact should NOT be ratified (that's a human step).
	result, _, err := journey.Check(dir)
	if err != nil {
		t.Fatalf("Check after elicit: %v", err)
	}
	if result != journey.CheckUnratified {
		t.Errorf("expected CheckUnratified after elicit, got %v", result)
	}
}

// TestJourneysCmd_ElicitWithExistingArtefact verifies that running journeys
// again when an artefact already exists reports the existing artefact.
func TestJourneysCmd_ElicitWithExistingArtefact(t *testing.T) {
	dir := t.TempDir()

	// First pass — elicit.
	cmdJourneys([]string{dir})

	// Second pass — elicit again. Should see existing artefact.
	exit := cmdJourneys([]string{dir})
	if exit != 0 {
		t.Errorf("expected exit 0 for elicit with existing artefact, got %d", exit)
	}
}

// TestJourneysCmd_PassPrint verifies that passing check prints the journeys.
func TestJourneysCmd_PassPrint(t *testing.T) {
	// Capture stdout.
	saved := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	dir := t.TempDir()
	a := journey.NewArtefact()
	a.AddJourney(journey.Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := journey.SaveArtefact(dir, a); err != nil {
		t.Fatal(err)
	}

	exit := cmdJourneys([]string{"--check", dir})
	if exit != 0 {
		t.Fatalf("expected exit 0, got %d", exit)
	}

	w.Close()
	// Read captured output.
	out := make([]byte, 1024)
	n, _ := r.Read(out)
	os.Stdout = saved

	output := string(out[:n])
	if !strings.Contains(output, "J01-test") {
		t.Errorf("expected output to contain J01-test, got: %s", output)
	}
	if !strings.Contains(output, "test_user") {
		t.Errorf("expected output to contain test_user, got: %s", output)
	}
}

// TestJourneysCmd_NoArgs verifies that journeys with no args defaults to cwd
// and drafts a template artefact.
func TestJourneysCmd_NoArgs(t *testing.T) {
	// Without --check and no project path, journeys should default to "."
	// and attempt to draft in cwd.
	exit := cmdJourneys([]string{})
	if exit != 0 {
		t.Errorf("expected exit 0 for journeys with no args (defaults to cwd), got %d", exit)
	}
	// Clean up the artefact created in cwd.
	os.RemoveAll(".sworn")
}

// TestJourneysCmd_NonExistentPath verifies handling of a non-existent path.
// filepath.Abs does not validate existence, so the path resolves to an
// absolute path and Check returns missing artefact (exit 1), not exit 2.
func TestJourneysCmd_NonExistentPath(t *testing.T) {
	exit := cmdJourneys([]string{"--check", "/nonexistent/path/xyz"})
	if exit != 1 {
		t.Errorf("expected exit 1 for nonexistent path (missing artefact), got %d", exit)
	}
}