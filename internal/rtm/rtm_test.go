package rtm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixture creates a minimal release directory tree for testing.
// The fixture is a fully-traced release by default; individual tests
// introduce breaks by modifying the fixture.
func writeFixture(t *testing.T, mods ...func(dir string)) string {
	t.Helper()
	dir := t.TempDir()

	// intake.md with needs and a release goal.
	intake := `---
title: Test intake
---

# Release Intake: test-release

## Release goal

The release goal text for testing.

## Needs

- N-01: First need for testing
- N-02: Second need for testing

## Other section

Some content.
`
	os.WriteFile(filepath.Join(dir, "intake.md"), []byte(intake), 0644)

	// index.md with release benefit and slices.
	index := `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
    worktree_branch: track/test/T1-test
---

# Board

## Release summary

- **Goal**: the release goal from index
- **Target version / integration branch**: release/v0.1.0

## Release benefit

The release delivers value to users.

## Slices

| ID | Track | User outcome | State | Owner | Spec | Proof |
|---|---|---|---|---|---|---|
| S01-test-slice | T1 | test outcome | planned | human | [spec](./S01-test-slice/spec.md) | — |
`
	os.WriteFile(filepath.Join(dir, "index.md"), []byte(index), 0644)

	// S01-test-slice with spec.md and status.json.
	sliceDir := filepath.Join(dir, "S01-test-slice")
	os.MkdirAll(sliceDir, 0755)

	spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-01.
- [ ] WHEN a test runs, THE SYSTEM SHALL verify N-02.

## Required tests

- **Unit**: internal/rtm/rtm_test.go — basic tests
- **Integration**: exercise the command end-to-end
`
	os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0644)

	status := `{
  "slice_id": "S01-test-slice",
  "state": "planned",
  "release_benefit": "The release delivers value to users."
}`
	os.WriteFile(filepath.Join(sliceDir, "status.json"), []byte(status), 0644)

	// Apply any modifications.
	for _, mod := range mods {
		mod(dir)
	}

	return dir
}

func TestBuild_FullyTraced(t *testing.T) {
	dir := writeFixture(t)
	m, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected 0 violations for fully-traced release, got %d:\n%s", len(vs), violationsString(vs))
	}
	if len(m.Needs) != 2 {
		t.Errorf("expected 2 needs, got %d", len(m.Needs))
	}
	if len(m.ACs) != 2 {
		t.Errorf("expected 2 ACs, got %d", len(m.ACs))
	}
	if len(m.Tests) != 2 {
		t.Errorf("expected 2 tests, got %d", len(m.Tests))
	}
	if len(m.Slices) != 1 {
		t.Errorf("expected 1 slice, got %d", len(m.Slices))
	}
}

// TestBuild_SpecJSONOnly_GoldenThread exercises S01 AC-06: on a spec.json-only
// slice (no spec.md "Required tests" section) the rtm reader must expose each
// AC's test_refs so the need->AC->test golden thread resolves. Without exposing
// test_refs the slice would show zero required tests and every AC would be
// flagged orphaned_ac_no_test — a trace break on exactly the releases this
// slice makes work.
func TestBuild_SpecJSONOnly_GoldenThread(t *testing.T) {
	dir := writeFixture(t, func(dir string) {
		sliceDir := filepath.Join(dir, "S01-test-slice")
		// spec.json-ONLY: remove the legacy spec.md, author spec.json.
		os.Remove(filepath.Join(sliceDir, "spec.md"))
		specJSON := `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "S01-test-slice",
  "release": "test-release",
  "user_outcome": "Test outcome.",
  "covers_needs": ["N-01", "N-02"],
  "in_scope": ["do the thing"],
  "out_of_scope": ["not the other thing"],
  "acceptance_criteria": [
    {"id": "AC-01", "text": "WHEN a release has a need, THE SYSTEM SHALL link it to N-01.", "ears_pattern": "event-driven", "test_refs": ["internal/rtm/rtm_test.go:TestBuild_SpecJSONOnly_GoldenThread"]},
    {"id": "AC-02", "text": "WHEN a test runs, THE SYSTEM SHALL verify N-02.", "ears_pattern": "event-driven", "test_refs": ["internal/rtm/rtm_test.go:TestBuild_SpecJSONOnly_GoldenThread"]}
  ]
}`
		os.WriteFile(filepath.Join(sliceDir, "spec.json"), []byte(specJSON), 0644)
	})

	m, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// ACs sourced from spec.json.
	if len(m.ACs) != 2 {
		t.Fatalf("expected 2 ACs from spec.json, got %d", len(m.ACs))
	}
	// Required tests sourced from each AC's test_refs — the golden-thread link.
	if len(m.Tests) == 0 {
		t.Fatal("expected required tests sourced from AC test_refs on a spec.json-only slice, got 0")
	}
	for _, v := range vs {
		if v.Kind == "orphaned_ac_no_test" {
			t.Fatalf("golden thread broke on a spec.json-only slice: %s\nall: %s", v, violationsString(vs))
		}
	}
}

// TestBuild_VerticalTraceFromBoardJSON exercises S06 AC-04: when a release has
// a committed board.json, Build reads release.vertical_trace.benefit /
// .org_objective from it (the ADR-0009 source of truth) rather than scraping
// index.md markdown headings. The board.json benefit is deliberately DIFFERENT
// from the index.md benefit so a pass proves board.json won, not a coincidence.
// It also round-trips a real board-shaped document through readBoardVerticalTrace
// (Captain pin 3 drift insurance).
func TestBuild_VerticalTraceFromBoardJSON(t *testing.T) {
	const boardBenefit = "board.json benefit is the canonical source of truth"
	dir := writeFixture(t, func(dir string) {
		board := `{
  "$schema": "https://baton.sawy3r.net/schemas/board-v1.json",
  "schema_version": 1,
  "release": {
    "name": "test-release",
    "vertical_trace": {
      "benefit": "` + boardBenefit + `"
    }
  },
  "tracks": [
    {"id": "T1-test", "slices": ["S01-test-slice"], "worktree_branch": "track/test/T1-test", "state": "planned"}
  ]
}`
		os.WriteFile(filepath.Join(dir, "board.json"), []byte(board), 0644)
	})

	m, _, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.ReleaseBenefit != boardBenefit {
		t.Errorf("ReleaseBenefit: got %q, want board.json value %q (index.md must not win when board.json exists)", m.ReleaseBenefit, boardBenefit)
	}
	// board.json carries no org_objective key — the field is genuinely unauthored,
	// so OrgObjective must be empty (AC-04), NOT the empty-because-wrong-file case.
	if m.OrgObjective != "" {
		t.Errorf("OrgObjective: got %q, want empty (no org_objective in board.json)", m.OrgObjective)
	}
}

// TestBuild_VerticalTraceLegacyFallback proves the fallback branch: a release
// with NO board.json (pre-ADR-0009) still traces via the markdown-heading parse
// of index.md, so legacy releases keep working unchanged.
func TestBuild_VerticalTraceLegacyFallback(t *testing.T) {
	dir := writeFixture(t) // no board.json written
	m, _, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.ReleaseBenefit != "The release delivers value to users." {
		t.Errorf("ReleaseBenefit (legacy fallback): got %q, want the index.md heading value", m.ReleaseBenefit)
	}
}

func TestBuild_OrphanedNeed(t *testing.T) {
	// Add a need with no linked AC.
	dir := writeFixture(t, func(dir string) {
		intake := `---
title: Test intake
---

# Release Intake: test-release

## Release goal

The release goal text for testing.

## Needs

- N-01: First need for testing
- N-02: Second need for testing
- N-03: Orphaned need with no AC
`
		os.WriteFile(filepath.Join(dir, "intake.md"), []byte(intake), 0644)
	})
	_, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, v := range vs {
		if v.Kind == "orphaned_need" && strings.Contains(v.Detail, "N-03") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected orphaned_need violation for N-03, got:\n%s", violationsString(vs))
	}
}

func TestBuild_OrphanedAC_NoNeed(t *testing.T) {
	// An AC that cites no need id.
	dir := writeFixture(t, func(dir string) {
		spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN something happens, THE SYSTEM SHALL do a thing with no need ref.

## Required tests

- **Unit**: some test
`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "spec.md"), []byte(spec), 0644)
	})
	_, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, v := range vs {
		if v.Kind == "orphaned_ac_no_need" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected orphaned_ac_no_need violation, got:\n%s", violationsString(vs))
	}
}

func TestBuild_OrphanedAC_NoTest(t *testing.T) {
	// A slice with an AC that cites a need but has no required tests.
	dir := writeFixture(t, func(dir string) {
		spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-01.

## Required tests

(none)
`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "spec.md"), []byte(spec), 0644)
	})
	_, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, v := range vs {
		if v.Kind == "orphaned_ac_no_test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected orphaned_ac_no_test violation, got:\n%s", violationsString(vs))
	}
}

func TestBuild_SliceNoVertical(t *testing.T) {
	// A slice with no release goal in intake and no release benefit on the slice.
	dir := writeFixture(t, func(dir string) {
		// Remove the release goal from intake.
		intake := `---
title: Test intake
---

# Release Intake: test-release

## Needs

- N-01: First need for testing
- N-02: Second need for testing
`
		os.WriteFile(filepath.Join(dir, "intake.md"), []byte(intake), 0644)
		// Remove the release benefit from status.json.
		status := `{
  "slice_id": "S01-test-slice",
  "state": "planned"
}`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "status.json"), []byte(status), 0644)
	})
	_, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, v := range vs {
		if v.Kind == "slice_no_vertical" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected slice_no_vertical violation, got:\n%s", violationsString(vs))
	}
}

func TestBuild_SoloFloor_NoObjective(t *testing.T) {
	// Solo/small-team floor: no org objective, but release goal present.
	// Every slice should pass on slice -> release goal.
	dir := writeFixture(t, func(dir string) {
		// No org objective in index.md (already the default).
		// No release benefit on the slice — but release goal is in intake.
		status := `{
  "slice_id": "S01-test-slice",
  "state": "planned"
}`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "status.json"), []byte(status), 0644)
	})
	_, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Should pass — release goal is the vertical floor.
	for _, v := range vs {
		if v.Kind == "slice_no_vertical" {
			t.Fatalf("solo floor should pass on slice -> release goal, but got: %s", v)
		}
	}
}

func TestBuild_AC_CitesNonExistentNeed(t *testing.T) {
	// An AC that cites a need id not in intake.
	dir := writeFixture(t, func(dir string) {
		spec := `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-99.

## Required tests

- **Unit**: some test
`
		os.WriteFile(filepath.Join(dir, "S01-test-slice", "spec.md"), []byte(spec), 0644)
	})
	_, vs, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, v := range vs {
		if v.Kind == "orphaned_ac_no_need" && strings.Contains(v.Detail, "N-99") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected orphaned_ac_no_need violation for N-99, got:\n%s", violationsString(vs))
	}
}

func TestPrint_NonEmpty(t *testing.T) {
	dir := writeFixture(t)
	m, _, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out := Print(m)
	if !strings.Contains(out, "Requirements Traceability Matrix") {
		t.Error("Print output missing title")
	}
	if !strings.Contains(out, "Horizontal trace") {
		t.Error("Print output missing horizontal trace section")
	}
	if !strings.Contains(out, "Vertical trace") {
		t.Error("Print output missing vertical trace section")
	}
	if !strings.Contains(out, "N-01") {
		t.Error("Print output missing need N-01")
	}
}

func TestParseNeeds(t *testing.T) {
	text := `## Needs

- N-01: First need
- N-02: Second need
- N-03: Third need with longer description
`
	needs := parseNeeds(text)
	if len(needs) != 3 {
		t.Fatalf("expected 3 needs, got %d", len(needs))
	}
	if needs[0].ID != "N-01" || needs[0].Description != "First need" {
		t.Errorf("first need: %+v", needs[0])
	}
}

func TestParseAcceptanceChecks(t *testing.T) {
	spec := `## Acceptance checks

- [ ] WHEN a thing, THE SYSTEM SHALL do N-01.
- [ ] IF another thing, THE SYSTEM SHALL verify N-02.
- [ ] WHERE no objective, THE SYSTEM SHALL accept N-01 and N-02.

## Other section

Not an AC.
`
	acs := parseAcceptanceChecks("S01-test", spec)
	if len(acs) != 3 {
		t.Fatalf("expected 3 ACs, got %d", len(acs))
	}
	if len(acs[0].NeedIDs) != 1 || acs[0].NeedIDs[0] != "N-01" {
		t.Errorf("AC 0 need ids: %v", acs[0].NeedIDs)
	}
	if len(acs[2].NeedIDs) != 2 {
		t.Errorf("AC 2 need ids: %v (expected 2)", acs[2].NeedIDs)
	}
}

func TestParseRequiredTests(t *testing.T) {
	spec := `## Required tests

- **Unit**: internal/rtm/rtm_test.go — basic tests
- **Integration**: exercise the command end-to-end
- **Reachability artefact**: smoke step description
`
	tests := parseRequiredTests("S01-test", spec)
	if len(tests) != 3 {
		t.Fatalf("expected 3 tests, got %d", len(tests))
	}
	if !strings.Contains(tests[0].Text, "Unit") {
		t.Errorf("test 0: %s", tests[0].Text)
	}
}

func TestIsSliceID(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"S01-rtm-spine", true},
		{"S02-ears-ac-format", true},
		{"S10-no-mock-boundary", true},
		{"screenshots", false},
		{".git", false},
		{"intake.md", false},
		{"T1-fidelity-core", false},
	}
	for _, tc := range tests {
		if got := isSliceID(tc.s); got != tc.want {
			t.Errorf("isSliceID(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate: got %q", got)
	}
	if got := truncate("hello world foo bar", 10); got != "hello w..." {
		t.Errorf("truncate: got %q", got)
	}
}

// violationsString renders violations for test error messages.
func violationsString(vs []Violation) string {
	var b []byte
	for _, v := range vs {
		b = append(b, v.String()...)
		b = append(b, '\n')
	}
	return string(b)
}
