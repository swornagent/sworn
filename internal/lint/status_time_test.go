package lint

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixedClock returns a Clock pinned to the given time.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// baseTime is the fixed "now" for all table tests.
var baseTime = time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

// TestCheckStatusTimestamps_Table is the central table-driven test for
// CheckStatusTimestamps. Each case creates a fixture release directory
// with one slice's status.json and validates the output.
func TestCheckStatusTimestamps_Table(t *testing.T) {
	tests := []struct {
		name          string
		lastUpdatedAt string // written into status.json
		verdictAt     string // written into verification.verifier_verdict_at, omit if empty
		wantViolation bool
		wantField     string // expected field name in violation, empty if none
	}{
		{
			name:          "valid past timestamp",
			lastUpdatedAt: "2026-06-24T12:00:00Z",
			wantViolation: false,
		},
		{
			name:          "valid timestamp within skew",
			lastUpdatedAt: "2026-06-25T12:04:00Z",
			wantViolation: false,
		},
		{
			name:          "valid timestamp exactly at skew boundary",
			lastUpdatedAt: "2026-06-25T12:05:00Z",
			wantViolation: false,
		},
		{
			name:          "future timestamp beyond skew",
			lastUpdatedAt: "2026-06-25T12:05:01Z",
			wantViolation: true,
			wantField:     "last_updated_at",
		},
		{
			name:          "far future timestamp",
			lastUpdatedAt: "2026-07-15T00:00:00Z",
			wantViolation: true,
			wantField:     "last_updated_at",
		},
		{
			name:          "malformed timestamp",
			lastUpdatedAt: "not-a-timestamp",
			wantViolation: true,
			wantField:     "last_updated_at",
		},
		{
			name:          "missing last_updated_at",
			lastUpdatedAt: "",
			wantViolation: false,
		},
		{
			name:          "valid verifier_verdict_at",
			lastUpdatedAt: "2026-06-24T12:00:00Z",
			verdictAt:     "2026-06-24T12:00:00Z",
			wantViolation: false,
		},
		{
			name:          "future verifier_verdict_at",
			lastUpdatedAt: "2026-06-24T12:00:00Z",
			verdictAt:     "2026-07-15T00:00:00Z",
			wantViolation: true,
			wantField:     "verification.verifier_verdict_at",
		},
		{
			name:          "malformed verifier_verdict_at",
			lastUpdatedAt: "2026-06-24T12:00:00Z",
			verdictAt:     "garbage",
			wantViolation: true,
			wantField:     "verification.verifier_verdict_at",
		},
		{
			name:          "both future",
			lastUpdatedAt: "2026-07-15T00:00:00Z",
			verdictAt:     "2026-07-16T00:00:00Z",
			wantViolation: true,
			wantField:     "last_updated_at", // first violation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			releaseDir := filepath.Join(dir, "docs", "release", "test-release")
			sliceDir := filepath.Join(releaseDir, "S01-test-slice")
			os.MkdirAll(sliceDir, 0755)

			// Build status.json.
			json := buildStatusJSON(tt.lastUpdatedAt, tt.verdictAt)
			os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(json), 0644)

			clock := fixedClock{t: baseTime}
			violations := CheckStatusTimestamps(releaseDir, clock)

			if tt.wantViolation {
				if len(violations) == 0 {
					t.Fatalf("expected violation, got none")
				}
				found := false
				for _, v := range violations {
					if v.Field == tt.wantField {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected violation on field %q, got: %v", tt.wantField, violations)
				}
			} else {
				if len(violations) > 0 {
					t.Errorf("expected no violations, got: %v", violations)
				}
			}
		})
	}
}

// TestCheckStatusTimestamps_MultipleSlices verifies that a release with
// multiple slices reports violations for each offending slice.
func TestCheckStatusTimestamps_MultipleSlices(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")

	for _, id := range []string{"S01-ok", "S02-future", "S03-malformed"} {
		sliceDir := filepath.Join(releaseDir, id)
		os.MkdirAll(sliceDir, 0755)

		var json string
		switch id {
		case "S01-ok":
			json = buildStatusJSON("2026-06-24T12:00:00Z", "")
		case "S02-future":
			json = buildStatusJSON("2026-07-15T00:00:00Z", "")
		case "S03-malformed":
			json = buildStatusJSON("not-a-timestamp", "")
		}
		os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(json), 0644)
	}

	clock := fixedClock{t: baseTime}
	violations := CheckStatusTimestamps(releaseDir, clock)

	if len(violations) != 2 {
		t.Fatalf("expected 2 violations (future + malformed), got %d: %v", len(violations), violations)
	}

	// Verify each slice id appears.
	slices := map[string]bool{}
	for _, v := range violations {
		slices[v.SliceID] = true
	}
	if !slices["S02-future"] {
		t.Error("missing S02-future violation")
	}
	if !slices["S03-malformed"] {
		t.Error("missing S03-malformed violation")
	}
	if slices["S01-ok"] {
		t.Error("S01-ok should have no violation")
	}
}

// TestCheckStatusTimestamps_NoReleases verifies that an empty directory
// produces no violations and no errors.
func TestCheckStatusTimestamps_NoReleases(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	clock := fixedClock{t: baseTime}
	violations := CheckStatusTimestamps(releaseDir, clock)

	if len(violations) != 0 {
		t.Errorf("expected no violations in empty release, got: %v", violations)
	}
}

// TestCheckStatusTimestamps_SkewEdgeCases verifies boundary behaviour around
// the 5-minute skew allowance.
func TestCheckStatusTimestamps_SkewEdgeCases(t *testing.T) {
	// 4m59s after baseTime — passes.
	t.Run("4m59s future passes", func(t *testing.T) {
		dir := t.TempDir()
		releaseDir := filepath.Join(dir, "docs", "release", "test-release")
		sliceDir := filepath.Join(releaseDir, "S01-edge")
		os.MkdirAll(sliceDir, 0755)

		ts := baseTime.Add(4*time.Minute + 59*time.Second).Format(time.RFC3339)
		json := buildStatusJSON(ts, "")
		os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(json), 0644)

		clock := fixedClock{t: baseTime}
		violations := CheckStatusTimestamps(releaseDir, clock)
		if len(violations) != 0 {
			t.Errorf("expected 4m59s to pass, got violations: %v", violations)
		}
	})

	// 5m1s after baseTime — fails.
	t.Run("5m1s future fails", func(t *testing.T) {
		dir := t.TempDir()
		releaseDir := filepath.Join(dir, "docs", "release", "test-release")
		sliceDir := filepath.Join(releaseDir, "S01-edge")
		os.MkdirAll(sliceDir, 0755)

		ts := baseTime.Add(5*time.Minute + 1*time.Second).Format(time.RFC3339)
		json := buildStatusJSON(ts, "")
		os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(json), 0644)

		clock := fixedClock{t: baseTime}
		violations := CheckStatusTimestamps(releaseDir, clock)
		if len(violations) == 0 {
			t.Error("expected 5m1s to fail, got no violations")
		}
	})
}

// TestCheckStatusTimestamps_NilClock uses DefaultClock — not table-driven
// because it depends on the real wall clock. We only verify it doesn't panic
// and the violations slice is valid.
func TestCheckStatusTimestamps_NilClock(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	sliceDir := filepath.Join(releaseDir, "S01-real")
	os.MkdirAll(sliceDir, 0755)

	// Write a status.json with a far-past timestamp — should always pass
	// against the real clock.
	json := buildStatusJSON("2020-01-01T00:00:00Z", "")
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(json), 0644)

	violations := CheckStatusTimestamps(releaseDir, nil)
	if len(violations) != 0 {
		t.Errorf("expected far-past timestamp to pass against real clock, got: %v", violations)
	}
}

// TestExtractJSONField tests the lightweight JSON field extraction.
func TestExtractJSONField(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		field string
		want  string
	}{
		{
			name:  "simple top-level field",
			data:  `{"last_updated_at": "2026-06-24T12:00:00Z"}`,
			field: "last_updated_at",
			want:  "2026-06-24T12:00:00Z",
		},
		{
			name:  "nested field",
			data:  `{"verification": {"verifier_verdict_at": "2026-06-24T12:00:00Z"}}`,
			field: "verification.verifier_verdict_at",
			want:  "2026-06-24T12:00:00Z",
		},
		{
			name:  "nested field with other keys",
			data:  `{"verification": {"result": "pending", "verifier_verdict_at": "2026-06-24T12:00:00Z"}}`,
			field: "verification.verifier_verdict_at",
			want:  "2026-06-24T12:00:00Z",
		},
		{
			name:  "absent field",
			data:  `{"slice_id": "S01"}`,
			field: "last_updated_at",
			want:  "",
		},
		{
			name:  "absent nested field",
			data:  `{"verification": {"result": "pending"}}`,
			field: "verification.verifier_verdict_at",
			want:  "",
		},
		{
			name:  "escaped quotes in value",
			data:  `{"note": "hello \"world\""}`,
			field: "note",
			want:  `hello \"world\"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSONField([]byte(tt.data), tt.field)
			if got != tt.want {
				t.Errorf("extractJSONField(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

// buildStatusJSON creates a minimal status.json with the given timestamp fields.
func buildStatusJSON(lastUpdatedAt, verdictAt string) string {
	s := `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S01-test-slice",
  "release": "test-release",
  "track": "T1-test",
  "state": "planned"`
	if lastUpdatedAt != "" {
		s += `,
  "last_updated_at": "` + lastUpdatedAt + `"`
	}
	s += `,
  "verification": {
    "result": "pending"`
	if verdictAt != "" {
		s += `,
    "verifier_verdict_at": "` + verdictAt + `"`
	}
	s += `
  }
}
`
	return s
}
