package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture creates a temp release directory with the given files.
// Each entry in files is a pair: relative path (from release dir) and content.
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}

// --- fully-traced release (PASS) ---

func TestRunTrace_FullyTraced(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

# Release Intake

## Release goal

Test release goal.

## What the human wants

- N-01: First need for testing
- N-02: Second need for testing
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

# Board

## Release summary

- **Goal**: test goal

## Release benefit

The release benefit text.
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01", "N-02"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN a release has a need, THE SYSTEM SHALL link it to N-01.
- [ ] WHEN a test runs, THE SYSTEM SHALL verify N-02.

## Required tests

- **Unit**: some test
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	if r.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", r.Verdict)
		for _, v := range r.Violations {
			t.Logf("  violation: %s", v.Msg)
		}
	}
	if r.TotalNeeds != 2 {
		t.Errorf("expected 2 needs, got %d", r.TotalNeeds)
	}
	if r.TotalACs != 2 {
		t.Errorf("expected 2 ACs, got %d", r.TotalACs)
	}
}

// --- bold-label intake format (N-01 derived from position) ---

func TestRunTrace_BoldLabelIntake(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

# Release Intake: test

## Release goal

Goal text.

## What the human wants

- **Parallel execution**: Run tracks concurrently
- **Safety guarantee**: Process ownership is safe
- **Verification gate**: Verify under concurrency

## Market context

Some context here.
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

# Board

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01", "N-02", "N-03"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

# Slice: S01-test-slice

## User outcome

Test outcome.

## Acceptance checks

- [ ] WHEN parallel execution is needed, THE SYSTEM SHALL run tracks concurrently. (N-01) (N-02) (N-03)

## Required tests

- **Unit**: some test
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	if r.TotalNeeds != 3 {
		t.Errorf("expected 3 bold-label needs, got %d", r.TotalNeeds)
	}
	if r.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", r.Verdict)
		for _, v := range r.Violations {
			t.Logf("  violation: %s", v.Msg)
		}
	}
}

// --- orphaned need ---

func TestRunTrace_OrphanedNeed(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: First need
- N-02: Orphaned need with no cover
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] WHEN something happens, THE SYSTEM SHALL respond. (N-01)
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	if r.Verdict != "FAIL" {
		t.Errorf("expected FAIL for orphaned need, got %s", r.Verdict)
	}
	found := false
	for _, v := range r.Violations {
		if v.Check == "orphaned-need" && v.Need == "N-02" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphaned-need violation for N-02")
	}
}

// --- invalid covers_needs ---

func TestRunTrace_InvalidCovers(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: Only need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01", "N-99"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] WHEN something happens, THE SYSTEM SHALL respond. (N-01)
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	found := false
	for _, v := range r.Violations {
		if v.Check == "invalid-covers" && strings.Contains(v.Msg, "N-99") {
			found = true
		}
	}
	if !found {
		t.Error("expected invalid-covers violation for N-99")
	}
}

// --- unclaimed coverage ---

func TestRunTrace_UnclaimedCoverage(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: A need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] WHEN something happens, THE SYSTEM SHALL respond.
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	found := false
	for _, v := range r.Violations {
		if v.Check == "unclaimed-coverage" {
			found = true
		}
	}
	if !found {
		t.Error("expected unclaimed-coverage violation")
	}
}

// --- EARS conformance (free-form AC) ---

func TestRunTrace_FreeFormAC(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: A need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] Fix the reported bug in production.
`})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	found := false
	for _, v := range r.Violations {
		if v.Check == "ears-conformance" {
			found = true
		}
	}
	if !found {
		t.Error("expected ears-conformance violation for free-form AC")
	}
	if r.FreeFormACs != 1 {
		t.Errorf("expected 1 free-form AC, got %d", r.FreeFormACs)
	}
}

// --- EARS classification ---

func TestRunTrace_EARSClassification(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: Need one
- N-02: Need two
- N-03: Need three
- N-04: Need four
- N-05: Need five
- N-06: Need six
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-all-ears]
---

## Release summary

- **Goal**: test
`,
		"S01-all-ears/status.json": `{
  "slice_id": "S01-all-ears",
  "covers_needs": ["N-01", "N-02", "N-03", "N-04", "N-05", "N-06"],
  "state": "verified"
}`,
		"S01-all-ears/spec.md": `---
title: S01-all-ears
---

## Acceptance checks

- [ ] THE SYSTEM SHALL respond to ubiquitous events. (N-01)
- [ ] WHEN a trigger occurs, THE SYSTEM SHALL process it. (N-02)
- [ ] WHILE in a state, THE SYSTEM SHALL maintain invariants. (N-03)
- [ ] WHERE a feature is enabled, THE SYSTEM SHALL provide access. (N-04)
- [ ] IF a condition holds, THEN THE SYSTEM SHALL take action. (N-05)
- [ ] WHEN a trigger fires and WHILE in a state, THE SYSTEM SHALL respond with a complex pattern. (N-06)
`})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	if r.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", r.Verdict)
		for _, v := range r.Violations {
			t.Logf("  violation: %s", v.Msg)
		}
	}
	if r.EARSStats["Ubiquitous"] != 1 {
		t.Errorf("expected Ubiquitous=1, got %d", r.EARSStats["Ubiquitous"])
	}
	if r.EARSStats["When"] != 1 {
		t.Errorf("expected When=1, got %d", r.EARSStats["When"])
	}
	if r.EARSStats["While"] != 1 {
		t.Errorf("expected While=1, got %d", r.EARSStats["While"])
	}
	if r.EARSStats["Where"] != 1 {
		t.Errorf("expected Where=1, got %d", r.EARSStats["Where"])
	}
	if r.EARSStats["If"] != 1 {
		t.Errorf("expected If=1, got %d", r.EARSStats["If"])
	}
	if r.EARSStats["Complex"] != 1 {
		t.Errorf("expected Complex=1, got %d", r.EARSStats["Complex"])
	}
}

// --- "see intake" reference ---

func TestRunTrace_SeeIntake(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: A need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] WHEN something happens, THE SYSTEM SHALL respond.

See intake.md for more details.
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	found := false
	for _, v := range r.Violations {
		if v.Check == "see-intake" {
			found = true
		}
	}
	if !found {
		t.Error("expected see-intake violation")
	}
}

// --- vague AC ---

func TestRunTrace_VagueAC(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: A need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] fix the reported error in the component
`})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	// The AC has "shall" so it's EARS-conformant, but it's vague (no concrete terms).
	found := false
	for _, v := range r.Violations {
		if v.Check == "vague-ac" {
			found = true
		}
	}
	if !found {
		t.Error("expected vague-ac violation")
	}
}

// --- vague in-scope item ---

func TestRunTrace_VagueInScope(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

- N-01: A need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## In scope

- fix the thing without concrete terms
- Build a component for user management

## Acceptance checks

- [ ] WHEN something happens, THE SYSTEM SHALL respond using component.tsx.
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	count := 0
	for _, v := range r.Violations {
		if v.Check == "vague-scope" {
			count++
		}
	}
	if count < 1 {
		t.Errorf("expected at least 1 vague-scope violation, got %d", count)
	}
}

// --- empty intake ---

func TestRunTrace_EmptyIntake(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal text.

## What the human wants

(no needs here)
`,
		"index.md": `---
title: Test board
---

## Release summary

- **Goal**: test
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	found := false
	for _, v := range r.Violations {
		if v.Check == "intake-structure" {
			found = true
		}
	}
	if !found {
		t.Error("expected intake-structure violation")
	}
}

// --- PrintReport / JSONReport ---

func TestPrintReport_Pass(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal.

## What the human wants

- N-01: Need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": ["N-01"],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] WHEN X happens, THE SYSTEM SHALL respond. (N-01)
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}

	out := PrintReport(r)
	if !strings.Contains(out, "PASS") {
		t.Errorf("PrintReport expected PASS, got: %s", out)
	}
	if !strings.Contains(out, "RELEASE TRACE") {
		t.Error("PrintReport missing banner")
	}

	jsonOut := JSONReport(r)
	if !strings.Contains(jsonOut, `"verdict"`) {
		t.Error("JSONReport missing verdict")
	}
}

func TestPrintReport_Fail(t *testing.T) {
	dir := fixture(t, map[string]string{
		"intake.md": `---
title: Test
---

## Release goal

Goal.

## What the human wants

- N-01: Need
`,
		"index.md": `---
title: Test board
tracks:
  - id: T1-test
    slices: [S01-test-slice]
---

## Release summary

- **Goal**: test
`,
		"S01-test-slice/status.json": `{
  "slice_id": "S01-test-slice",
  "covers_needs": [],
  "state": "verified"
}`,
		"S01-test-slice/spec.md": `---
title: S01-test-slice
---

## Acceptance checks

- [ ] free-form ac without shall
`,
	})

	r, err := RunTrace(dir)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}

	if r.Verdict != "FAIL" {
		t.Fatalf("expected FAIL, got %s", r.Verdict)
	}

	out := PrintReport(r)
	if !strings.Contains(out, "FAIL") {
		t.Errorf("PrintReport expected FAIL, got: %s", out)
	}
	if !strings.Contains(out, "NOT TRACEABLE") {
		t.Error("PrintReport missing NOT TRACEABLE")
	}
}

// --- helpers ---

func TestTruncate(t *testing.T) {
	if s := truncate("hello", 10); s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}
	if s := truncate("hello world this is long", 15); !strings.HasSuffix(s, "...") {
		t.Errorf("expected truncated with '...', got %q", s)
	}
	if len(truncate("hello world this is long", 15)) != 15 {
		t.Errorf("expected length 15 after truncation")
	}
}

func TestParseNeeds_Explicit(t *testing.T) {
	needs := parseNeeds("# Intake\n\n## What the human wants\n\n- N-01: First\n- N-02: Second\n")
	if len(needs) != 2 {
		t.Fatalf("expected 2 needs, got %d", len(needs))
	}
	if needs[0].ID != "N-01" || needs[0].Desc != "First" {
		t.Errorf("need[0] = %s/%s, want N-01/First", needs[0].ID, needs[0].Desc)
	}
	if needs[1].ID != "N-02" || needs[1].Desc != "Second" {
		t.Errorf("need[1] = %s/%s, want N-02/Second", needs[1].ID, needs[1].Desc)
	}
}

func TestParseNeeds_BoldLabel(t *testing.T) {
	needs := parseNeeds("# Intake\n\n## What the human wants\n\n- **Parallel execution**: Run tracks concurrently\n- **Safety**: Ensure safety\n")
	if len(needs) != 2 {
		t.Fatalf("expected 2 needs, got %d", len(needs))
	}
	if needs[0].ID != "N-01" || needs[0].Desc != "Run tracks concurrently" {
		t.Errorf("need[0] = %s/%s, want N-01/Run tracks concurrently", needs[0].ID, needs[0].Desc)
	}
	if needs[1].ID != "N-02" || needs[1].Desc != "Ensure safety" {
		t.Errorf("need[1] = %s/%s, want N-02/Ensure safety", needs[1].ID, needs[1].Desc)
	}
}

func TestParseNeeds_ExplicitTakesPrecedence(t *testing.T) {
	// Explicit N-NN format takes priority over bold-label.
	needs := parseNeeds("# Intake\n\n## What the human wants\n\n- N-01: Explicit need\n- **Bold label**: Ignored\n")
	if len(needs) != 1 {
		t.Fatalf("expected 1 need (explicit), got %d", len(needs))
	}
	if needs[0].ID != "N-01" || needs[0].Desc != "Explicit need" {
		t.Errorf("need[0] = %s/%s, want N-01/Explicit need", needs[0].ID, needs[0].Desc)
	}
}

func TestParseCoversNeeds(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []string
	}{
		{"empty array", `{"covers_needs": []}`, nil},
		{"single", `{"covers_needs": ["N-01"]}`, []string{"N-01"}},
		{"multiple", `{"covers_needs": ["N-01", "N-02", "N-03"]}`, []string{"N-01", "N-02", "N-03"}},
		{"none", `{"slice_id": "S01"}`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), "status.json")
			os.WriteFile(tmp, []byte(tt.json), 0644)
			got := parseCoversNeeds(tmp)
			if len(got) != len(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d]=%q, want[%d]=%q", i, got[i], i, tt.want[i])
				}
			}
		})
	}
}

func TestClassifyEARS(t *testing.T) {
	tests := []struct {
		ac   string
		want string
	}{
		{"THE SYSTEM SHALL respond", "Ubiquitous"},
		{"WHEN triggered, THE SYSTEM SHALL act", "When"},
		{"while active, THE SYSTEM SHALL maintain", "While"},
		{"WHERE enabled, THE SYSTEM SHALL show", "Where"},
		{"IF condition, THEN THE SYSTEM SHALL respond", "If"},
		{"WHEN triggered and WHILE active, THE SYSTEM SHALL respond", "Complex"},
	}
	for _, tt := range tests {
		got := classifyEARS(tt.ac)
		if got != tt.want {
			t.Errorf("classifyEARS(%q) = %q, want %q", tt.ac, got, tt.want)
		}
	}
}

func TestHasViolations(t *testing.T) {
	r := &TraceReport{Verdict: "PASS", Failed: 0}
	if r.HasViolations() {
		t.Error("expected no violations")
	}
	r.Failed = 1
	if !r.HasViolations() {
		t.Error("expected violations")
	}
}

func TestViolationString(t *testing.T) {
	v := TraceViolation{
		Check:    "orphaned-need",
		Severity: "FAIL",
		Msg:      "Need N-02 is orphaned",
		Slice:    "S01-test",
		Need:     "N-02",
	}
	s := v.String()
	if !strings.Contains(s, "orphaned-need") {
		t.Error("String missing check")
	}
	if !strings.Contains(s, "Need N-02 is orphaned") {
		t.Error("String missing msg")
	}
	if !strings.Contains(s, "S01-test") {
		t.Error("String missing slice")
	}
	if !strings.Contains(s, "N-02") {
		t.Error("String missing need")
	}
}

func TestParseAcceptanceChecks(t *testing.T) {
	spec := `## Acceptance checks

- [ ] First AC with shall
- [x] Second AC done
- [ ] NOTE: This is an informational note

## Required tests
`
	acs := parseAcceptanceChecks(spec)
	if len(acs) != 2 {
		t.Fatalf("expected 2 ACs, got %d: %v", len(acs), acs)
	}
	if acs[0] != "First AC with shall" {
		t.Errorf("acs[0] = %q", acs[0])
	}
	if acs[1] != "Second AC done" {
		t.Errorf("acs[1] = %q", acs[1])
	}
}

func TestParseVagueInScope(t *testing.T) {
	spec := `## In scope

- fix the broken thing
- Build a component for the feature
- Parse intake.md for needs
- Write internal/gate/trace_test.go

## Acceptance checks

- [ ] WHEN called, THE SYSTEM SHALL parse.
`
	items := parseVagueInScope(spec)
	// "fix the broken thing" is vague (no concrete terms)
	// "Build a component for the feature" is vague
	// "Parse intake.md for needs" has "intake.md" — concrete
	// "Write internal/gate/trace_test.go" has ".go" — concrete
	if len(items) != 2 {
		t.Errorf("expected 2 vague items, got %d: %v", len(items), items)
	}
}
