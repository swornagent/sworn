package journey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestImpactAnalysis_MissingArtefact verifies AC2: WHEN no ratified journeys
// artefact exists, impact analysis fails closed and directs the user to run
// elicitation (S11) first.
func TestImpactAnalysis_MissingArtefact(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S01-test", []string{"cmd/sworn/verify.go"}, nil)

	_, err := AnalyzeImpact(projectRoot, releaseDir)
	if err == nil {
		t.Fatal("expected error for missing artefact, got nil")
	}
	// Should be an ImpactError with CheckMissing.
	var impErr *ImpactError
	if !asImpactError(err, &impErr) {
		t.Fatalf("expected *ImpactError, got %T: %v", err, err)
	}
	if impErr.Result != CheckMissing {
		t.Errorf("expected CheckMissing, got %v", impErr.Result)
	}
	if !strings.Contains(impErr.Message, "elicit") {
		t.Errorf("expected message to mention elicit, got: %s", impErr.Message)
	}
}

// TestImpactAnalysis_UnratifiedArtefact verifies that an unratified artefact
// also fails closed — ratification is required before impact analysis.
func TestImpactAnalysis_UnratifiedArtefact(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test-release")
	makeSliceDir(t, releaseDir, "S01-test", []string{"cmd/sworn/verify.go"}, nil)

	// Create an unratified artefact.
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-test",
		UserType: "test_user",
		Outcome:  "Test journey",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	_, err := AnalyzeImpact(projectRoot, releaseDir)
	if err == nil {
		t.Fatal("expected error for unratified artefact, got nil")
	}
	var impErr *ImpactError
	if !asImpactError(err, &impErr) {
		t.Fatalf("expected *ImpactError, got %T: %v", err, err)
	}
	if impErr.Result != CheckUnratified {
		t.Errorf("expected CheckUnratified, got %v", impErr.Result)
	}
}

// TestImpactAnalysis_TouchedJourneys verifies AC1: a release with a ratified
// journeys artefact reports which journeys it touches. A journey step with
// surface "verify" should match a slice touching "cmd/sworn/verify.go" via
// token-level matching.
func TestImpactAnalysis_TouchedJourneys(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "fidelity-layer")
	makeSliceDir(t, releaseDir, "S12-impact",
		[]string{"cmd/sworn/verify.go", "internal/journey/journey.go"}, nil)
	makeSliceDir(t, releaseDir, "S13-walkthrough",
		[]string{"cmd/sworn/walkthrough.go"}, nil)

	// Create a ratified artefact with journeys matching the slice touchpoints.
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify-flow",
		UserType: "developer",
		Outcome:  "Verify a slice via sworn verify",
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
			{Order: 1, Description: "Install", Surface: "CLI"},
			{Order: 2, Description: "Run init", Surface: "sworn init"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeImpact(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}

	if !result.ArtefactFound {
		t.Error("expected artefact_found = true")
	}
	if !result.IsRatified {
		t.Error("expected is_ratified = true")
	}

	// J01-verify-flow should be touched (surface "verify" → token "verify"
	// → planned file "cmd/sworn/verify.go" token "verify").
	if !contains(result.JourneysTouched, "J01-verify-flow") {
		t.Errorf("expected J01-verify-flow in touched journeys, got %v", result.JourneysTouched)
	}

	// J02-init-setup should also be touched (surface "CLI" → conventional
	// mapping to "cmd/" directories in the slice files).
	if !contains(result.JourneysTouched, "J02-init-setup") {
		t.Errorf("expected J02-init-setup in touched journeys (CLI surface → cmd/), got %v", result.JourneysTouched)
	}

	// AllJourneyIDs should list both journeys.
	if !contains(result.AllJourneyIDs, "J01-verify-flow") || !contains(result.AllJourneyIDs, "J02-init-setup") {
		t.Errorf("expected both journeys in AllJourneyIDs, got %v", result.AllJourneyIDs)
	}

	if result.ReleaseName != "fidelity-layer" {
		t.Errorf("expected release_name 'fidelity-layer', got %q", result.ReleaseName)
	}
}

// TestImpactAnalysis_EmptyTouchedSet verifies AC3: WHEN a release touches no
// journeys (e.g. internal-only refactor touching paths that no journey surface
// matches), THE SYSTEM SHALL report an empty touched-set explicitly rather than
// failing.
func TestImpactAnalysis_EmptyTouchedSet(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "internal-refactor")
	// Internal-only paths with no journey-relevant surfaces.
	makeSliceDir(t, releaseDir, "S99-refactor",
		[]string{"internal/state/state.go", "internal/config/config.go"}, nil)

	// Create a ratified artefact with journeys that reference different surfaces.
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-verify",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Press verify button", Surface: "verify-ui"},
		},
		EntrySurface: "web-dashboard",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeImpact(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}

	if len(result.JourneysTouched) != 0 {
		t.Errorf("expected empty touched set for internal-only refactor, got %v", result.JourneysTouched)
	}

	// AllJourneyIDs should still list the journey.
	if len(result.AllJourneyIDs) != 1 || result.AllJourneyIDs[0] != "J01-verify" {
		t.Errorf("expected AllJourneyIDs = [J01-verify], got %v", result.AllJourneyIDs)
	}
}

// TestImpactAnalysis_DerivedFromTouchpoints verifies AC4: the touched set is
// derived from slice touchpoints, not a hand-maintained list. This is a
// structural test — if the function uses planned_files/actual_files from
// status.json (which it does), the derivation property holds by construction.
func TestImpactAnalysis_DerivedFromTouchpoints(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test")
	makeSliceDir(t, releaseDir, "S01-bench",
		[]string{"cmd/sworn/bench.go", "internal/bench/bench.go"}, nil)
	makeSliceDir(t, releaseDir, "S02-verify",
		[]string{"cmd/sworn/verify.go", "internal/verify/verify.go"}, nil)

	// Ratified artefact with journeys whose surfaces match planned_files.
	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-benchmark",
		UserType: "developer",
		Outcome:  "Run benchmarks on a model",
		Steps: []JourneyStep{
			{Order: 1, Description: "Invoke sworn bench", Surface: "bench"},
		},
		EntrySurface: "CLI",
	})
	a.AddJourney(Journey{
		ID:       "J02-verify",
		UserType: "developer",
		Outcome:  "Verify a slice",
		Steps: []JourneyStep{
			{Order: 1, Description: "Invoke sworn verify", Surface: "verify"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeImpact(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}

	// Both journeys should be touched — S01-bench has bench.go → "bench" matches,
	// S02-verify has verify.go → "verify" matches.
	if len(result.JourneysTouched) != 2 {
		t.Errorf("expected 2 touched journeys, got %v", result.JourneysTouched)
	}
	if !contains(result.JourneysTouched, "J01-benchmark") {
		t.Errorf("expected J01-benchmark in touched, got %v", result.JourneysTouched)
	}
	if !contains(result.JourneysTouched, "J02-verify") {
		t.Errorf("expected J02-verify in touched, got %v", result.JourneysTouched)
	}
}

// TestImpactAnalysis_WithActualFiles verifies that actual_files recorded by
// prior implementer sessions are also considered as touchpoints (not just
// planned_files).
func TestImpactAnalysis_WithActualFiles(t *testing.T) {
	projectRoot := t.TempDir()
	releaseDir := filepath.Join(projectRoot, "docs", "release", "test")
	// Only planned_files reference journeys.go; actual_files add impact.go.
	makeSliceDir(t, releaseDir, "S12-impact",
		[]string{"internal/journey/journey.go"},
		[]string{"internal/journey/impact.go", "internal/journey/journey.go"})

	a := NewArtefact()
	a.AddJourney(Journey{
		ID:       "J01-impact",
		UserType: "developer",
		Outcome:  "Analyse journey impact",
		Steps: []JourneyStep{
			{Order: 1, Description: "Run journeys impact", Surface: "impact"},
		},
		EntrySurface: "CLI",
	})
	if err := a.Ratify("test_human"); err != nil {
		t.Fatal(err)
	}
	if err := SaveArtefact(projectRoot, a); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeImpact(projectRoot, releaseDir)
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}

	// J01-impact should be touched: actual_files contains impact.go → "impact"
	// matches the journey surface "impact".
	if !contains(result.JourneysTouched, "J01-impact") {
		t.Errorf("expected J01-impact in touched (via actual_files), got %v", result.JourneysTouched)
	}
}

// TestSurfaceTouch verifies the surfacesTouch helper directly with known cases.
func TestSurfaceTouch(t *testing.T) {
	tests := []struct {
		file    string
		surface string
		want    bool
	}{
		// Token match: "verify" in surface matches "verify" in file path.
		{"cmd/sworn/verify.go", "verify", true},
		{"cmd/sworn/verify.go", "Verify", true},
		// CLI conventional mapping.
		{"cmd/sworn/journeys.go", "CLI", true},
		{"cmd/sworn/journeys.go", "cli", true},
		// Substring match.
		{"internal/journey/journey.go", "journey", true},
		{"internal/journey/journey.go", "Journey", true},
		// No match.
		{"internal/config/config.go", "verify", false},
		{"internal/state/state.go", "dashboard", false},
		// Token-level: "sworn init" → "init" token matches a file with "init" in path.
		{"cmd/sworn/init.go", "sworn init", true},
		// Broad surface: "journeys" matches files that contain "journey" as token.
		{"internal/journey/journey.go", "journeys", true}, // "journeys" → tokens [journeys]; "journey.go" → tokens [journey, go]; "journey" contained in "journeys"
	}

	for _, tt := range tests {
		got := surfacesTouch(tt.file, tt.surface)
		if got != tt.want {
			t.Errorf("surfacesTouch(%q, %q) = %v, want %v", tt.file, tt.surface, got, tt.want)
		}
	}
}

// TestTokenize verifies the tokenize helper.
func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"cmd/sworn/verify.go", []string{"cmd", "sworn", "verify", "go"}},
		{"CLI", []string{"CLI"}},
		{"sworn init", []string{"sworn", "init"}},
		{"CLI / sworn", []string{"CLI", "sworn"}}, {"verify-ui", []string{"verify", "ui"}},
		{"web-dashboard", []string{"web", "dashboard"}},
		{"", nil},
	}

	for _, tt := range tests {
		got := tokenize(tt.input)
		if !stringSliceEqual(got, tt.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- helpers ---

// makeSliceDir creates a slice directory under releaseDir with a status.json
// containing planned_files and (optionally) actual_files. If actualFiles is
// nil, only planned_files is set.
func makeSliceDir(t *testing.T, releaseDir, sliceID string, plannedFiles, actualFiles []string) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		t.Fatal(err)
	}

	type sliceStatus struct {
		SliceID      string   `json:"slice_id"`
		PlannedFiles []string `json:"planned_files"`
		ActualFiles  []string `json:"actual_files,omitempty"`
	}

	status := sliceStatus{
		SliceID:      sliceID,
		PlannedFiles: plannedFiles,
	}
	if actualFiles != nil {
		status.ActualFiles = actualFiles
	} else {
		// Default actual_files to match planned_files.
		status.ActualFiles = plannedFiles
	}

	writeJSON(t, filepath.Join(sliceDir, "status.json"), status)
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// asImpactError attempts to unwrap an error to *ImpactError.
func asImpactError(err error, target **ImpactError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*ImpactError); ok {
		*target = e
		return true
	}
	// Check for wrapped impact error.
	if e, ok := err.(interface{ Unwrap() error }); ok {
		return asImpactError(e.Unwrap(), target)
	}
	return false
}

// Ensure json import is used (it's used by writeJSON but the compiler may not
// count indirect usage). This import is required by the test file.
var _ = sort.Strings // ensure sort import is used
