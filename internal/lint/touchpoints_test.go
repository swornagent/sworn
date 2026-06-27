package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createTempRelease creates a temporary release directory structure with the
// given index.md content and slice directories, each with a spec.md and
// status.json. Returns the release dir.
func createTempRelease(t *testing.T, indexContent string, slices []struct {
	id           string
	track        string
	plannedFiles []string
	specContent  string
}) string {
	t.Helper()

	releaseDir := t.TempDir()

	// Write index.md.
	if err := os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0o644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}

	// Write each slice.
	for _, s := range slices {
		sliceDir := filepath.Join(releaseDir, s.id)
		if err := os.MkdirAll(sliceDir, 0o755); err != nil {
			t.Fatalf("mkdir slice dir: %v", err)
		}

		// Write spec.md.
		if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(s.specContent), 0o644); err != nil {
			t.Fatalf("write spec.md: %v", err)
		}

		// Write status.json.
		pfJSON := ""
		for i, pf := range s.plannedFiles {
			if i > 0 {
				pfJSON += ", "
			}
			pfJSON += `"` + pf + `"`
		}
		statusJSON := `{
  "slice_id": "` + s.id + `",
  "release": "test-release",
  "track": "` + s.track + `",
  "state": "in_progress",
  "planned_files": [` + pfJSON + `]
}`
		if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(statusJSON), 0o644); err != nil {
			t.Fatalf("write status.json: %v", err)
		}
	}

	return releaseDir
}

func TestTouchpointUndeclaredFails(t *testing.T) {
	indexContent := `# Test Release
### Touchpoint matrix
| File / surface | T1 |
|---|---|
| internal/foo/bar.go | ✓ |
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"internal/foo/bar.go"},
			specContent: `# S01-test
## In scope
- Implements ` + "`internal/foo/bar.go`" + ` and ` + "`internal/other/missing.go`" + `
## Planned touchpoints
- ` + "`internal/foo/bar.go`" + `
`,
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err == nil {
		t.Fatal("expected error for undeclared reference, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "internal/other/missing.go") {
		t.Fatalf("error should name internal/other/missing.go, got: %v", err)
	}
	if !strings.Contains(errStr, "undeclared") {
		t.Fatalf("error should mention 'undeclared', got: %v", err)
	}
}

func TestTouchpointCollisionFails(t *testing.T) {
	indexContent := `# Test Release
### Touchpoint matrix
| File / surface | T1 | T2 |
|---|---|---|
| internal/shared/file.go | ✓ | ✓ |
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"internal/shared/file.go"},
			specContent: `# S01-test
## In scope
- Uses ` + "`internal/shared/file.go`" + `
`,
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err == nil {
		t.Fatal("expected error for cross-slice collision, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "collision") {
		t.Fatalf("error should mention 'collision', got: %v", err)
	}
	if !strings.Contains(errStr, "internal/shared/file.go") {
		t.Fatalf("error should name the colliding file, got: %v", err)
	}
	if !strings.Contains(errStr, "T2") {
		t.Fatalf("error should name the other track T2, got: %v", err)
	}
}

func TestTouchpointDocumentedSharedIsNoteNotViolation(t *testing.T) {
	indexContent := `# Test Release
### Touchpoint matrix
| File / surface | T1 | T2 |
|---|---|---|
| cmd/sworn/main.go (DOCUMENTED SHARED — additive dispatch) | ✓ | ✓ |
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"cmd/sworn/main.go"},
			specContent: `# S01-test
## In scope
- Extends ` + "`cmd/sworn/main.go`" + ` with new case
## Planned touchpoints
- ` + "`cmd/sworn/main.go`" + `
`,
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err != nil {
		t.Fatalf("expected nil for DOCUMENTED SHARED file (informational note only), got: %v", err)
	}
}

func TestTouchpointCleanPasses(t *testing.T) {
	indexContent := `# Test Release
### Touchpoint matrix
| File / surface | T1 |
|---|---|
| internal/foo/bar.go | ✓ |
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"internal/foo/bar.go"},
			specContent: `# S01-test
## In scope
- Uses ` + "`internal/foo/bar.go`" + `
## Planned touchpoints
- ` + "`internal/foo/bar.go`" + `
`,
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err != nil {
		t.Fatalf("expected nil for clean slice, got: %v", err)
	}
}

func TestTouchpointSectionScopingExcludesRiskAndTests(t *testing.T) {
	// Verify that paths in Risk and Required-tests sections are NOT extracted.
	indexContent := `# Test Release
### Touchpoint matrix
| File / surface | T1 |
|---|---|
| internal/real.go | ✓ |
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"internal/real.go"},
			specContent: `# S01-test
## In scope
- Uses ` + "`internal/real.go`" + `
## Risks
- audit ` + "`internal/fake.go`" + ` for security
- verify ` + "`docs/release/test/spec.md`" + `
## Required tests
- ` + "`go test ./internal/...`" + `
`,
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err != nil {
		t.Fatalf("expected nil — section scoping should exclude Risk + Required-tests, got: %v", err)
	}
}

func TestTouchpointPackagePrefixMatch(t *testing.T) {
	// A package reference like `internal/lint` should match planned_files entries
	// like "internal/lint/touchpoints.go" via prefix matching.
	indexContent := `# Test Release
### Touchpoint matrix
| File / surface | T1 |
|---|---|
| internal/lint/touchpoints.go | ✓ |
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"internal/lint/touchpoints.go", "internal/lint/touchpoints_test.go"},
			specContent: `# S01-test
## In scope
- New ` + "`internal/lint`" + ` package for touchpoint checking
## Planned touchpoints
- ` + "`internal/lint/touchpoints.go`" + `
- ` + "`internal/lint/touchpoints_test.go`" + `
`,
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err != nil {
		t.Fatalf("expected nil — package ref `internal/lint` should prefix-match planned_files, got: %v", err)
	}
}

func TestMigrationCollisionFails(t *testing.T) {
	indexContent := `# Test Release
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"migrations/000012_create_foo.sql"},
			specContent:  "# S01-test\n",
		},
		{
			id:           "S02-test",
			track:        "T1",
			plannedFiles: []string{"migrations/000012_create_bar.sql"},
			specContent:  "# S02-test\n",
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err == nil {
		t.Fatal("expected error for duplicate migration number, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "migration") {
		t.Fatalf("error should mention 'migration', got: %v", err)
	}
	if !strings.Contains(errStr, "000012") {
		t.Fatalf("error should name migration 000012, got: %v", err)
	}
}

func TestNoTouchpointMatrixPasses(t *testing.T) {
	// Release without a touchpoint matrix should not error.
	indexContent := `# Test Release
## No matrix here
`
	slices := []struct {
		id           string
		track        string
		plannedFiles []string
		specContent  string
	}{
		{
			id:           "S01-test",
			track:        "T1",
			plannedFiles: []string{"internal/foo.go"},
			specContent: `# S01-test
## In scope
- Uses ` + "`internal/foo.go`" + `
`,
		},
	}

	releaseDir := createTempRelease(t, indexContent, slices)
	sliceDir := filepath.Join(releaseDir, "S01-test")

	err := CheckTouchpoints(sliceDir, releaseDir)
	if err != nil {
		t.Fatalf("expected nil for release without touchpoint matrix, got: %v", err)
	}
}