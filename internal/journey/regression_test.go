package journey

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegressionCoverageGaps_WalkedJourneyNoTest verifies AC1:
// WHEN a journey is ratified + walked but flagged for regression with no
// committed test, THE SYSTEM SHALL list it as a coverage gap.
func TestRegressionCoverageGaps_WalkedJourneyNoTest(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice via sworn verify",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})

	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	projectRoot := t.TempDir()

	gaps := RegressionCoverageGaps(a, attArtefact, projectRoot)
	if len(gaps) != 1 {
		t.Fatalf("expected 1 coverage gap, got %v", gaps)
	}
	if gaps[0] != "J01-verify-flow" {
		t.Errorf("expected J01-verify-flow as gap, got %s", gaps[0])
	}
}

// TestRegressionCoverageGaps_WalkedJourneyWithTest verifies that a journey
// with HasRegression=true is not a coverage gap.
func TestRegressionCoverageGaps_WalkedJourneyWithTest(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:                "J01-verify-flow",
		UserType:          "developer",
		Outcome:           "Verify a slice",
		HasRegression:     true,
		RegressionTestPath: "tests/e2e/journeys/journey_j01_verify_flow_test.go",
	})

	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	projectRoot := t.TempDir()
	gaps := RegressionCoverageGaps(a, attArtefact, projectRoot)
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps for journey with HasRegression, got %v", gaps)
	}
}

// TestRegressionCoverageGaps_FileOnDiskButNotFlagged verifies that a journey
// whose RegressionTestPath points to an existing file on disk is treated as
// covered even if HasRegression is not set.
func TestRegressionCoverageGaps_FileOnDiskButNotFlagged(t *testing.T) {
	projectRoot := t.TempDir()

	// Create the scaffold file on disk.
	testPath := filepath.Join("tests", "e2e", "journeys", "journey_j01_test.go")
	absPath := filepath.Join(projectRoot, testPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte("// existing"), 0644); err != nil {
		t.Fatal(err)
	}

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:                 "J01-verify-flow",
		UserType:           "developer",
		Outcome:            "Verify a slice",
		RegressionTestPath: testPath,
		// HasRegression deliberately false — coverage is from disk.
	})

	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	gaps := RegressionCoverageGaps(a, attArtefact, projectRoot)
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps (file exists on disk), got %v", gaps)
	}
}

// TestRegressionCoverageGaps_UnwalkedJourneyNotFlagged verifies AC4:
// THE SYSTEM SHALL only codify journeys that have a passing walkthrough — an
// un-walked journey is not flagged as a gap.
func TestRegressionCoverageGaps_UnwalkedJourneyNotFlagged(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Verify", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	a.AddJourney(Journey{
		ID:       "J02-init-setup",
		UserType: "new_user",
		Outcome:  "Set up",
		Steps: []JourneyStep{
			{Order: 1, Description: "Init", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})

	// Only J01 has a passing attestation.
	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-verify-flow",
				Status:    WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	projectRoot := t.TempDir()
	gaps := RegressionCoverageGaps(a, attArtefact, projectRoot)

	// Only J01 should be a gap (walked but no regression test).
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap (J01 walked, J02 un-walked), got %v", gaps)
	}
	if gaps[0] != "J01-verify-flow" {
		t.Errorf("expected J01-verify-flow as gap, got %s", gaps[0])
	}
}

// TestRegressionCoverageGaps_FailedWalkthroughNotFlagged verifies that a
// journey with a failed walkthrough is NOT flagged as a coverage gap.
func TestRegressionCoverageGaps_FailedWalkthroughNotFlagged(t *testing.T) {
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-failed-journey",
		UserType: "developer",
		Outcome:  "Test journey that failed walkthrough",
	})

	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-failed-journey",
				Status:    WalkFail,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	projectRoot := t.TempDir()
	gaps := RegressionCoverageGaps(a, attArtefact, projectRoot)
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps for failed walkthrough, got %v", gaps)
	}
}

// TestCodifyJourney_GeneratesScaffold verifies AC2:
// WHEN sworn journeys --regen runs for a walked journey, THE SYSTEM SHALL
// emit a regression test scaffold whose steps mirror the journey's steps.
func TestCodifyJourney_GeneratesScaffold(t *testing.T) {
	projectRoot := t.TempDir()

	j := &Journey{
		ID:       "J02-init-setup",
		UserType: "new_user",
		Outcome:  "Set up the tool for the first time",
		Steps: []JourneyStep{
			{Order: 1, Description: "Install the tool", Surface: "CLI"},
			{Order: 2, Description: "Run init to bootstrap configuration", Surface: "sworn init"},
			{Order: 3, Description: "Verify the setup with a smoke test", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	}

	relPath, err := CodifyJourney(j, "", projectRoot)
	if err != nil {
		t.Fatalf("CodifyJourney: %v", err)
	}

	// Verify the file exists and contains expected content.
	absPath := filepath.Join(projectRoot, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("read generated scaffold: %v", err)
	}

	content := string(data)

	// Must reference the journey ID.
	if !strings.Contains(content, "J02-init-setup") {
		t.Errorf("expected scaffold to mention journey ID J02-init-setup")
	}

	// Must contain all journey steps.
	for _, step := range j.Steps {
		if !strings.Contains(content, step.Description) {
			t.Errorf("expected scaffold to include step description %q", step.Description)
		}
	}

	// Must be a valid Go test file structure.
	if !strings.Contains(content, "package journey_test") {
		t.Errorf("expected package journey_test in scaffold")
	}
	if !strings.Contains(content, "import \"testing\"") {
		t.Errorf("expected testing import in scaffold")
	}
	if !strings.Contains(content, "t.Skip") {
		t.Errorf("expected t.Skip in scaffold (scaffold, not full test)")
	}

	// Must end with a closing brace.
	if !strings.HasSuffix(strings.TrimSpace(content), "}") {
		t.Errorf("expected scaffold to end with closing brace")
	}
}

// TestCodifyJourney_Idempotent verifies AC3:
// WHEN a journey already has regression coverage, THE SYSTEM SHALL preserve it
// (accretive, not regenerated-from-scratch).
func TestCodifyJourney_Idempotent(t *testing.T) {
	projectRoot := t.TempDir()

	j := &Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run sworn verify", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	}

	// First call should generate the scaffold.
	relPath1, err := CodifyJourney(j, "", projectRoot)
	if err != nil {
		t.Fatalf("first CodifyJourney: %v", err)
	}

	// Second call should fail because the file already exists (accretive).
	_, err = CodifyJourney(j, "", projectRoot)
	if err == nil {
		t.Fatal("expected error on second call (file exists), got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}

	// Verify the original file was not overwritten.
	absPath := filepath.Join(projectRoot, relPath1)
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("read scaffold: %v", err)
	}
	if !strings.Contains(string(data), "Run sworn verify") {
		t.Error("expected scaffold content to be preserved after second call")
	}
}

// TestCodifyWalkedJourneys_Accretive verifies AC3 end-to-end:
// walked journeys are codified, previously-codified journeys are preserved.
func TestCodifyWalkedJourneys_Accretive(t *testing.T) {
	projectRoot := t.TempDir()

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-first",
		UserType: "developer",
		Outcome:  "First journey",
		Steps: []JourneyStep{
			{Order: 1, Description: "Step one", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	a.AddJourney(Journey{
		ID:       "J02-second",
		UserType: "developer",
		Outcome:  "Second journey",
		Steps: []JourneyStep{
			{Order: 1, Description: "Step two", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})

	// Both journeys have passing attestations.
	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-first",
				Status:    WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
			{
				JourneyID: "J02-second",
				Status:    WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	// First pass: both journeys should be codified.
	generated, err := CodifyWalkedJourneys(a, attArtefact, "", projectRoot)
	if err != nil {
		t.Fatalf("first CodifyWalkedJourneys: %v", err)
	}
	if len(generated) != 2 {
		t.Fatalf("expected 2 generated files, got %d: %v", len(generated), generated)
	}

	// Verify both journeys are marked as having regression coverage.
	if !a.Journeys[0].HasRegression {
		t.Errorf("expected J01-first.HasRegression=true after codification")
	}
	if !a.Journeys[1].HasRegression {
		t.Errorf("expected J02-second.HasRegression=true after codification")
	}

	// Second pass: no new files should be generated (accretive).
	generated2, err := CodifyWalkedJourneys(a, attArtefact, "", projectRoot)
	if err != nil {
		t.Fatalf("second CodifyWalkedJourneys: %v", err)
	}
	if len(generated2) != 0 {
		t.Errorf("expected 0 generated files on second pass (accretive), got %d: %v", len(generated2), generated2)
	}
}

// TestCodifyWalkedJourneys_UnwalkedNotCodified verifies AC4:
// an un-walked journey is not auto-codified.
func TestCodifyWalkedJourneys_UnwalkedNotCodified(t *testing.T) {
	projectRoot := t.TempDir()

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-walked",
		UserType: "developer",
		Outcome:  "Walked journey",
		Steps: []JourneyStep{
			{Order: 1, Description: "Step", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})
	a.AddJourney(Journey{
		ID:       "J02-unwalked",
		UserType: "developer",
		Outcome:  "Un-walked journey",
	})

	// Only J01 has a passing attestation.
	attArtefact := &AttestationArtefact{
		Version: 1,
		Attestations: []Attestation{
			{
				JourneyID: "J01-walked",
				Status:    WalkPass,
				WalkedBy:  "brad",
				RealInfra: true,
				MocksOff:  true,
			},
		},
	}

	generated, err := CodifyWalkedJourneys(a, attArtefact, "", projectRoot)
	if err != nil {
		t.Fatalf("CodifyWalkedJourneys: %v", err)
	}
	if len(generated) != 1 {
		t.Fatalf("expected 1 generated file (only walked journey), got %d: %v", len(generated), generated)
	}

	// Verify the walked journey is marked.
	if !a.Journeys[0].HasRegression {
		t.Errorf("expected J01-walked.HasRegression=true")
	}
	// Verify the un-walked journey is NOT marked.
	if a.Journeys[1].HasRegression {
		t.Errorf("expected J02-unwalked.HasRegression=false")
	}
}

// TestSanitiseID verifies the ID sanitisation helper.
func TestSanitiseID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"J01-verify-flow", "j01_verify_flow"},
		{"J01", "j01"},
		{"J-develop-feature", "j_develop_feature"},
		{"J02.init.setup", "j02_init_setup"},
		{"ABC", "abc"},
		{"", ""},
	}

	for _, tt := range tests {
		got := sanitiseID(tt.input)
		if got != tt.want {
			t.Errorf("sanitiseID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestRegressionCoverageGaps_NilArtefacts verifies nil-safety.
func TestRegressionCoverageGaps_NilArtefacts(t *testing.T) {
	projectRoot := t.TempDir()

	if gaps := RegressionCoverageGaps(nil, nil, projectRoot); gaps != nil {
		t.Errorf("expected nil for nil artefacts, got %v", gaps)
	}
}