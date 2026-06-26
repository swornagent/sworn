package designfit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/state"
)

// writeFixture creates a release directory with a single slice's status.json.
// Returns the release directory path.
func writeFixture(t *testing.T, dir, sliceID string, status *state.Status) string {
	t.Helper()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	sliceDir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.Write(filepath.Join(sliceDir, "status.json"), status); err != nil {
		t.Fatal(err)
	}
	return releaseDir
}

// writeReleaseSlice creates a minimal status.json for a slice in a release
// directory with just the given design decisions.
func writeReleaseSlice(t *testing.T, releaseDir, sliceID string, decisions []state.DesignDecision) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	st := &state.Status{
		SliceID:         sliceID,
		DesignDecisions: decisions,
	}
	if err := state.Write(filepath.Join(sliceDir, "status.json"), st); err != nil {
		t.Fatal(err)
	}
}

// TestDesignfit_Type1WithoutDecision verifies AC1: Type-1 without human_decision fails.
func TestDesignfit_Type1WithoutDecision(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S01-test", &state.Status{
		SliceID: "S01-test",
		DesignDecisions: []state.DesignDecision{
			{
				Choice:     "database-engine",
				StakeClass: state.Type1,
				Options:    []string{"PostgreSQL", "SQLite"},
				Rationale:  "migrations matter",
				// No HumanDecision — this should fail
			},
		},
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if !report.HasViolations() {
		t.Fatal("expected violations for Type-1 decision without human_decision, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(report.Violations))
	}
	v := report.Violations[0]
	if v.SliceID != "S01-test" {
		t.Errorf("expected slice S01-test, got %s", v.SliceID)
	}
	if v.ChoiceName != "database-engine" {
		t.Errorf("expected choice 'database-engine', got %s", v.ChoiceName)
	}
}

// TestDesignfit_Type2WithNotedDefault verifies AC2: Type-2 proceeds with noted default.
func TestDesignfit_Type2WithNotedDefault(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S01-test", &state.Status{
		SliceID: "S01-test",
		DesignDecisions: []state.DesignDecision{
			{
				Choice:        "button-color",
				StakeClass:    state.Type2,
				HumanDecision: "default noted — use primary-600",
				Rationale:     "matches existing palette, low blast-radius",
			},
		},
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if report.HasViolations() {
		t.Fatalf("expected no violations for Type-2 with noted default, got %v", report.Violations)
	}
}

// TestDesignfit_Type1WithHumanDecision verifies AC3: all Type-1 with decisions passes.
func TestDesignfit_Type1WithHumanDecision(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S01-test", &state.Status{
		SliceID: "S01-test",
		DesignDecisions: []state.DesignDecision{
			{
				Choice:        "database-engine",
				StakeClass:    state.Type1,
				Options:       []string{"PostgreSQL", "SQLite"},
				HumanDecision: "PostgreSQL",
				Rationale:     "migrations matter and we already have the infra",
			},
			{
				Choice:        "auth-provider",
				StakeClass:    state.Type1,
				Options:       []string{"Auth0", "Clerk"},
				HumanDecision: "Auth0 — existing integration",
				Rationale:     "already in use, no migration cost",
			},
		},
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if report.HasViolations() {
		t.Fatalf("expected no violations when all Type-1 have human decisions, got %v", report.Violations)
	}
}

// TestDesignfit_Type2WithoutDecision passes — Type-2 does not require a decision.
func TestDesignfit_Type2WithoutDecision(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S01-test", &state.Status{
		SliceID: "S01-test",
		DesignDecisions: []state.DesignDecision{
			{
				Choice:     "toast-position",
				StakeClass: state.Type2,
				Options:    []string{"top-right", "bottom-center"},
				// No HumanDecision — Type-2 allows this with noted default
				Rationale: "top-right is the convention",
			},
		},
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if report.HasViolations() {
		t.Fatalf("expected no violations for Type-2 without decision, got %v", report.Violations)
	}
}

// TestDesignfit_ArchitecturallySignificantMustBeType1 verifies AC4:
// architecturally-significant choices must be Type-1.
func TestDesignfit_ArchitecturallySignificantMustBeType1(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S01-test", &state.Status{
		SliceID: "S01-test",
		DesignDecisions: []state.DesignDecision{
			{
				Choice:                     "state-management",
				StakeClass:                 state.Type2, // incorrectly Type-2
				ArchitecturallySignificant: true,
				Rationale:                  "need to pick a state lib",
			},
		},
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if !report.HasViolations() {
		t.Fatal("expected violation for architecturally-significant but Type-2, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(report.Violations))
	}
	v := report.Violations[0]
	if v.ChoiceName != "state-management" {
		t.Errorf("expected choice 'state-management', got %s", v.ChoiceName)
	}
}

// TestDesignfit_ArchitecturallySignificantType1Passes verifies that an
// architecturally-significant choice classified as Type-1 with a human
// decision passes.
func TestDesignfit_ArchitecturallySignificantType1Passes(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S01-test", &state.Status{
		SliceID: "S01-test",
		DesignDecisions: []state.DesignDecision{
			{
				Choice:                     "routing-framework",
				StakeClass:                 state.Type1,
				ArchitecturallySignificant: true,
				Options:                    []string{"react-router", "tanstack-router"},
				HumanDecision:              "tanstack-router — type-safe routes",
				Rationale:                  "type-safe routing reduces runtime errors",
			},
		},
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if report.HasViolations() {
		t.Fatalf("expected no violations for architecturally-significant Type-1 with decision, got %v", report.Violations)
	}
}

// TestDesignfit_MultipleSlices checks across-slice aggregation.
func TestDesignfit_MultipleSlices(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// S01: one Type-1 WITHOUT decision -> violation
	writeReleaseSlice(t, releaseDir, "S01-pass", []state.DesignDecision{
		{
			Choice:        "cache-strategy",
			StakeClass:    state.Type1,
			Options:       []string{"redis", "memcached"},
			HumanDecision: "redis",
			Rationale:     "already running redis for queues",
		},
	})

	// S02: one Type-1 WITHOUT decision -> violation
	writeReleaseSlice(t, releaseDir, "S02-fail", []state.DesignDecision{
		{
			Choice:     "queue-provider",
			StakeClass: state.Type1,
			Options:    []string{"rabbitmq", "sqs", "nats"},
			// No HumanDecision
		},
	})

	// S03: no design decisions -> no violation
	writeReleaseSlice(t, releaseDir, "S03-no-decisions", nil)

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if !report.HasViolations() {
		t.Fatal("expected violations across multiple slices, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation (S02 only), got %d", len(report.Violations))
	}
	if report.Violations[0].SliceID != "S02-fail" {
		t.Errorf("expected violation from S02-fail, got %s", report.Violations[0].SliceID)
	}
}

// TestDesignfit_Print_RoundTrip verifies that Print works on both pass and fail reports.
func TestDesignfit_Print_RoundTrip(t *testing.T) {
	// PASS report
	passReport := &Report{
		Release:       "test-release",
		SlicesChecked: 3,
	}
	passOut := Print(passReport)
	if passOut == "" {
		t.Fatal("Print returned empty string for pass report")
	}

	// FAIL report
	failReport := &Report{
		Release:       "test-release",
		SlicesChecked: 2,
		Violations: []Violation{
			{SliceID: "S01-test", ChoiceName: "db", StakeClass: state.Type1, Description: "no decision"},
		},
	}
	failOut := Print(failReport)
	if failOut == "" {
		t.Fatal("Print returned empty string for fail report")
	}

	// PrintCompact
	compactPass := PrintCompact(passReport)
	if compactPass == "" {
		t.Fatal("PrintCompact returned empty for pass")
	}
	compactFail := PrintCompact(failReport)
	if compactFail == "" {
		t.Fatal("PrintCompact returned empty for fail")
	}
}

// TestDesignfit_EmptyRelease verifies that a release with no slices produces
// a clean report.
func TestDesignfit_EmptyRelease(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if report.HasViolations() {
		t.Fatalf("expected no violations for empty release, got %v", report.Violations)
	}
	if report.SlicesChecked != 0 {
		t.Errorf("expected 0 slices checked, got %d", report.SlicesChecked)
	}
}

// TestType1ImpliedEmptyDecisionsFails verifies AC1: a slice whose planned_files
// touch an architecturally-significant prefix (cmd/sworn/) but whose
// design_decisions is empty records a violation — the gate fails closed.
func TestType1ImpliedEmptyDecisionsFails(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S23-memory-config", &state.Status{
		SliceID: "S23-memory-config",
		// PlannedFiles touch cmd/sworn/ → Type-1 implied
		PlannedFiles: []string{
			"cmd/sworn/memory.go",
			"internal/memory/engine.go",
		},
		// DesignDecisions empty — the bypass being fixed
		DesignDecisions: nil,
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if !report.HasViolations() {
		t.Fatal("expected violation for Type-1-implied slice with empty design_decisions, got none")
	}
	if len(report.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(report.Violations))
	}
	v := report.Violations[0]
	if v.SliceID != "S23-memory-config" {
		t.Errorf("expected slice S23-memory-config, got %s", v.SliceID)
	}
	// ChoiceName is empty for this violation type (the decision array is empty).
	if v.ChoiceName != "" {
		t.Errorf("expected empty ChoiceName for empty-decisions violation, got %q", v.ChoiceName)
	}
}

// TestNoType1EmptyDecisionsPasses verifies AC2: a slice with no Type-1-implied
// work (planned_files only in non-architectural packages) and empty
// design_decisions records no violation — the benign empty case still passes.
func TestNoType1EmptyDecisionsPasses(t *testing.T) {
	dir := t.TempDir()
	releaseDir := writeFixture(t, dir, "S99-utility", &state.Status{
		SliceID: "S99-utility",
		// PlannedFiles only touch non-architectural packages
		PlannedFiles: []string{
			"internal/lint/deps.go",
			"docs/release/test/spec.md",
		},
		// DesignDecisions empty — benign, no Type-1 work implied
		DesignDecisions: nil,
	})

	report, err := Run(releaseDir)
	if err != nil {
		t.Fatal(err)
	}

	if report.HasViolations() {
		t.Fatalf("expected no violations for benign empty design_decisions, got %v", report.Violations)
	}
}
