package journey

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestArtefact creates a minimal ratified artefact in a temp directory
// for use as a fixture. Returns the project root.
func newTestArtefact(t *testing.T, ratified bool) string {
	t.Helper()
	root := t.TempDir()

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-test-journey",
		UserType: "test_user",
		Outcome:  "Complete a test action",
		Steps: []JourneyStep{
			{Order: 1, Description: "Do step one", Surface: "test"},
			{Order: 2, Description: "Do step two", Surface: "test"},
		},
		EntrySurface: "test",
	})

	if ratified {
		if err := a.Ratify("test_human"); err != nil {
			t.Fatal(err)
		}
	}

	if err := SaveArtefact(root, a); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestCheck_MissingArtefact verifies AC1: WHEN no journeys artefact exists for
// a project, THE SYSTEM SHALL exit non-zero from sworn journeys --check and
// state that elicitation has not been run.
func TestCheck_MissingArtefact(t *testing.T) {
	root := t.TempDir() // no artefact written

	result, artefact, err := Check(root)
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if result != CheckMissing {
		t.Errorf("expected CheckMissing, got %v", result)
	}
	if artefact != nil {
		t.Errorf("expected nil artefact for missing check, got non-nil")
	}
}

// TestCheck_UnratifiedArtefact verifies AC2: WHEN a journeys artefact exists
// but is unratified by a human, THE SYSTEM SHALL fail and name it as
// unratified.
func TestCheck_UnratifiedArtefact(t *testing.T) {
	root := newTestArtefact(t, false)

	result, artefact, err := Check(root)
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if result != CheckUnratified {
		t.Errorf("expected CheckUnratified, got %v", result)
	}
	if artefact == nil {
		t.Fatal("expected non-nil artefact for unratified check")
	}
	if artefact.IsRatified {
		t.Error("expected artefact.IsRatified to be false")
	}
}

// TestCheck_RatifiedArtefact verifies AC4: WHEN the artefact exists and is
// human-ratified, THE SYSTEM SHALL exit 0 from sworn journeys --check and
// list the journeys.
func TestCheck_RatifiedArtefact(t *testing.T) {
	root := newTestArtefact(t, true)

	result, artefact, err := Check(root)
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if result != CheckPass {
		t.Errorf("expected CheckPass, got %v", result)
	}
	if artefact == nil {
		t.Fatal("expected non-nil artefact for passing check")
	}
	if !artefact.IsRatified {
		t.Error("expected artefact.IsRatified to be true")
	}
}

// TestListJourneys verifies that ListJourneys returns sorted journey
// descriptions from a ratified artefact.
func TestListJourneys(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{ID: "J02-second", UserType: "user_b", Outcome: "Outcome B"})
	a.AddJourney(Journey{ID: "J01-first", UserType: "user_a", Outcome: "Outcome A"})

	list := ListJourneys(a)
	if len(list) != 2 {
		t.Fatalf("expected 2 journeys, got %d", len(list))
	}

	// Should be sorted by ID.
	if list[0] != "J01-first: user_a — Outcome A" {
		t.Errorf("expected first to be J01-first, got %q", list[0])
	}
	if list[1] != "J02-second: user_b — Outcome B" {
		t.Errorf("expected second to be J02-second, got %q", list[1])
	}
}

// TestListJourneys_NilArtefact verifies ListJourneys returns nil for a nil
// artefact.
func TestListJourneys_NilArtefact(t *testing.T) {
	list := ListJourneys(nil)
	if list != nil {
		t.Errorf("expected nil for nil artefact, got %v", list)
	}
}

// TestListJourneys_EmptyArtefact verifies ListJourneys returns nil for an
// artefact with no journeys.
func TestListJourneys_EmptyArtefact(t *testing.T) {
	a := NewArtefact()
	list := ListJourneys(a)
	if list != nil {
		t.Errorf("expected nil for empty artefact, got %v", list)
	}
}

// TestDraftTemplate verifies that DraftTemplate creates a non-nil artefact
// with at least one journey.
func TestDraftTemplate(t *testing.T) {
	root := t.TempDir()

	// Create a minimal project structure to influence template generation.
	if err := os.MkdirAll(filepath.Join(root, "cmd", "verify"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "verify"), 0755); err != nil {
		t.Fatal(err)
	}

	a, err := DraftTemplate(root)
	if err != nil {
		t.Fatalf("DraftTemplate returned error: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil artefact from DraftTemplate")
	}
	if len(a.Journeys) == 0 {
		t.Error("expected at least one journey from DraftTemplate")
	}
	if a.IsRatified {
		t.Error("expected draft template to NOT be ratified")
	}

	// Must be saveable.
	if err := SaveArtefact(root, a); err != nil {
		t.Fatalf("SaveArtefact after DraftTemplate: %v", err)
	}

	// Must be loadable back.
	loaded, err := LoadArtefact(root)
	if err != nil {
		t.Fatalf("LoadArtefact after save: %v", err)
	}
	if len(loaded.Journeys) == 0 {
		t.Error("expected loaded artefact to have journeys")
	}
}

// TestRatify_EmptyArtefact verifies that Ratifying an artefact with no
// journeys fails.
func TestRatify_EmptyArtefact(t *testing.T) {
	a := NewArtefact()
	err := a.Ratify("test_human")
	if err == nil {
		t.Error("expected error when ratifying artefact with no journeys")
	}
}

// TestRatify_MissingName verifies that Ratify requires a non-empty name.
func TestRatify_MissingName(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{ID: "J01", UserType: "test", Outcome: "test"})
	err := a.Ratify("")
	if err == nil {
		t.Error("expected error when ratifying with empty name")
	}
}

// TestRatify_Success verifies successful ratification.
func TestRatify_Success(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{ID: "J01", UserType: "test", Outcome: "test"})

	if err := a.Ratify("brad"); err != nil {
		t.Fatalf("Ratify failed: %v", err)
	}
	if !a.IsRatified {
		t.Error("expected IsRatified to be true after Ratify")
	}
	if a.RatifiedBy != "brad" {
		t.Errorf("expected RatifiedBy 'brad', got %q", a.RatifiedBy)
	}
	if a.RatifiedAt == "" {
		t.Error("expected RatifiedAt to be set")
	}
}

// TestAddJourney_InvalidatesRatification verifies that adding a journey after
// ratification resets the ratification state.
func TestAddJourney_InvalidatesRatification(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{ID: "J01", UserType: "test", Outcome: "test"})
	if err := a.Ratify("brad"); err != nil {
		t.Fatal(err)
	}

	// Adding a new journey should invalidate ratification.
	a.AddJourney(Journey{ID: "J02", UserType: "test", Outcome: "test2"})
	if a.IsRatified {
		t.Error("expected IsRatified to be false after adding a journey")
	}
}

// TestSaveAndLoadArtefact verifies round-trip save and load.
func TestSaveAndLoadArtefact(t *testing.T) {
	root := t.TempDir()
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-save-load",
		UserType: "test",
		Outcome:  "Test round-trip",
		Steps: []JourneyStep{
			{Order: 1, Description: "Step one", Surface: "cli"},
		},
		EntrySurface: "cli",
	})

	if err := SaveArtefact(root, a); err != nil {
		t.Fatalf("SaveArtefact: %v", err)
	}

	loaded, err := LoadArtefact(root)
	if err != nil {
		t.Fatalf("LoadArtefact: %v", err)
	}
	if loaded.Version != 1 {
		t.Errorf("expected Version 1, got %d", loaded.Version)
	}
	if len(loaded.Journeys) != 1 {
		t.Errorf("expected 1 journey, got %d", len(loaded.Journeys))
	}
	if loaded.Journeys[0].ID != "J01-save-load" {
		t.Errorf("expected ID J01-save-load, got %q", loaded.Journeys[0].ID)
	}
}

// TestLoadArtefact_NotExist verifies that LoadArtefact returns the sentinel
// error when the artefact file does not exist.
func TestLoadArtefact_NotExist(t *testing.T) {
	root := t.TempDir()
	_, err := LoadArtefact(root)
	if err == nil {
		t.Fatal("expected error for non-existent artefact")
	}
	if !isArtefactNotExist(err) {
		t.Errorf("expected ErrArtefactNotExist, got %v", err)
	}
}

// TestJourneyArtefactPath verifies the artefact path is correct.
func TestJourneyArtefactPath(t *testing.T) {
	path := JourneyArtefactPath("/tmp/project")
	expected := "/tmp/project/.sworn/journeys.json"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}
