// Package rtm builds and validates the 2-D requirements traceability matrix
// (RTM) for a Baton release. The RTM has two axes:
//
//   - Horizontal: need -> acceptance-criterion -> test -> proof. Needs are
//     enumerated with stable ids in intake.md; each spec.md acceptance check
//     cites the need id(s) it satisfies; required tests cite the acceptance
//     check; the proof bundle closes AC -> test -> proof.
//   - Vertical (golden thread): org objective -> release benefit -> slice.
//     Recorded in index.md (release-level benefit + optional objective) and
//     per-slice (the slice's link to the release benefit).
//
// The matrix is built from existing artefacts alone (intake.md / spec.md /
// status.json / index.md) — no separate datastore is introduced. The Build
// function reads a release directory and returns the matrix plus any
// violations (broken traces). A fully-traced release has zero violations.
//
// Stdlib only — zero runtime dependencies.
package rtm

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/style"
)

// Need is a requirements-level need enumerated in intake.md with a stable id.
type Need struct {
	ID          string // stable need id, e.g. "N-01"
	Description string // one-line description from intake
}

// AcceptanceCriterion is one spec acceptance check linked to one or more needs.
type AcceptanceCriterion struct {
	SliceID string   // the slice this AC belongs to
	Text    string   // the AC text (the checkbox line from spec.md)
	NeedIDs []string // need ids cited by this AC (parsed from the AC text)
}

// Test is a required test from a spec.md "Required tests" section.
type Test struct {
	SliceID string // the slice this test belongs to
	Text    string // the test description
}

// Slice is a release slice with its vertical-trace link.
type Slice struct {
	ID             string
	ReleaseGoal    string // the release goal (vertical floor: slice -> release goal)
	ReleaseBenefit string // the release benefit this slice links to (optional, above floor)
	OrgObjective   string // the org objective (optional, opt-in)
}

// Matrix is the full 2-D requirements traceability matrix for a release.
type Matrix struct {
	Release        string
	Needs          []Need
	ACs            []AcceptanceCriterion
	Tests          []Test
	Slices         []Slice
	ReleaseGoal    string
	ReleaseBenefit string
	OrgObjective   string // empty if no org objective declared (solo/small-team floor)
}

// Violation is a broken trace in the matrix. Kind names the failure category;
// Detail names the specific element.
type Violation struct {
	Kind   string // "orphaned_need", "orphaned_ac_no_need", "orphaned_ac_no_test", "slice_no_vertical"
	Detail string // human-readable detail
}

// String returns a human-readable violation line.
func (v Violation) String() string {
	return fmt.Sprintf("[%s] %s", v.Kind, v.Detail)
}

// Build reads a release directory and constructs the RTM. The releaseDir is
// the path to docs/release/<release-name>/, which must contain intake.md and
// index.md, plus one subdirectory per slice (each containing spec.md and
// status.json).
//
// The function returns the matrix and a list of violations. A fully-traced
// release has zero violations. Build itself never returns an error for
// trace violations — those are in the Violations slice. It returns an error
// only for I/O failures (unreadable files, missing directory).
func Build(releaseDir string) (*Matrix, []Violation, error) {
	m := &Matrix{Release: filepath.Base(releaseDir)}

	// 1. Parse intake.md for needs.
	intakePath := filepath.Join(releaseDir, "intake.md")
	intakeText, err := os.ReadFile(intakePath)
	if err != nil {
		return nil, nil, fmt.Errorf("rtm: read intake.md: %w", err)
	}
	m.Needs = parseNeeds(string(intakeText))
	m.ReleaseGoal = parseReleaseGoal(string(intakeText))

	// 2. Parse index.md for the vertical golden thread (release benefit,
	// org objective, and per-slice vertical links).
	indexPath := filepath.Join(releaseDir, "index.md")
	indexText, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, nil, fmt.Errorf("rtm: read index.md: %w", err)
	}
	m.ReleaseBenefit = parseReleaseBenefit(string(indexText))
	m.OrgObjective = parseOrgObjective(string(indexText))

	// 3. Parse each slice's spec.md and status.json.
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, nil, fmt.Errorf("rtm: read release dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sliceID := e.Name()
		// Skip non-slice directories (e.g. screenshots).
		if !isSliceID(sliceID) {
			continue
		}
		sliceDir := filepath.Join(releaseDir, sliceID)
		specPath := filepath.Join(sliceDir, "spec.md")
		statusPath := filepath.Join(sliceDir, "status.json")

		// Parse spec.md for acceptance checks and required tests.
		specText, err := os.ReadFile(specPath)
		if err != nil {
			return nil, nil, fmt.Errorf("rtm: read %s: %w", specPath, err)
		}
		acs := parseAcceptanceChecks(sliceID, string(specText))
		m.ACs = append(m.ACs, acs...)
		tests := parseRequiredTests(sliceID, string(specText))
		m.Tests = append(m.Tests, tests...)

		// Parse status.json for the vertical link.
		statusText, err := os.ReadFile(statusPath)
		if err != nil {
			return nil, nil, fmt.Errorf("rtm: read %s: %w", statusPath, err)
		}
		slice := Slice{ID: sliceID}
		slice.ReleaseGoal = m.ReleaseGoal
		slice.ReleaseBenefit = parseSliceReleaseBenefit(string(statusText))
		slice.OrgObjective = parseSliceOrgObjective(string(statusText))
		m.Slices = append(m.Slices, slice)
	}

	// Sort for deterministic output.
	sort.Slice(m.Needs, func(i, j int) bool { return m.Needs[i].ID < m.Needs[j].ID })
	sort.Slice(m.ACs, func(i, j int) bool {
		if m.ACs[i].SliceID != m.ACs[j].SliceID {
			return m.ACs[i].SliceID < m.ACs[j].SliceID
		}
		return m.ACs[i].Text < m.ACs[j].Text
	})
	sort.Slice(m.Tests, func(i, j int) bool {
		if m.Tests[i].SliceID != m.Tests[j].SliceID {
			return m.Tests[i].SliceID < m.Tests[j].SliceID
		}
		return m.Tests[i].Text < m.Tests[j].Text
	})
	sort.Slice(m.Slices, func(i, j int) bool { return m.Slices[i].ID < m.Slices[j].ID })

	// 4. Validate the matrix.
	violations := validate(m)

	return m, violations, nil
}

// validate checks the matrix for broken traces and returns all violations.
func validate(m *Matrix) []Violation {
	var vs []Violation

	needSet := map[string]bool{}
	for _, n := range m.Needs {
		needSet[n.ID] = true
	}

	// Build AC -> test lookup per slice.
	acsBySlice := map[string][]AcceptanceCriterion{}
	for _, ac := range m.ACs {
		acsBySlice[ac.SliceID] = append(acsBySlice[ac.SliceID], ac)
	}
	testsBySlice := map[string][]Test{}
	for _, t := range m.Tests {
		testsBySlice[t.SliceID] = append(testsBySlice[t.SliceID], t)
	}

	// Check: orphaned need (need with no linked AC).
	needHasAC := map[string]bool{}
	for _, ac := range m.ACs {
		for _, nid := range ac.NeedIDs {
			needHasAC[nid] = true
		}
	}
	for _, n := range m.Needs {
		if !needHasAC[n.ID] {
			vs = append(vs, Violation{
				Kind:   "orphaned_need",
				Detail: fmt.Sprintf("need %s (%s) has no linked acceptance criterion", n.ID, truncate(n.Description, 60)),
			})
		}
	}

	// Check: orphaned AC (cites no need, or cites a need but has no linked test).
	for _, ac := range m.ACs {
		if len(ac.NeedIDs) == 0 {
			vs = append(vs, Violation{
				Kind:   "orphaned_ac_no_need",
				Detail: fmt.Sprintf("acceptance criterion in %s cites no need id: %s", ac.SliceID, truncate(ac.Text, 60)),
			})
			continue
		}
		// Check that cited need ids exist.
		for _, nid := range ac.NeedIDs {
			if !needSet[nid] {
				vs = append(vs, Violation{
					Kind:   "orphaned_ac_no_need",
					Detail: fmt.Sprintf("acceptance criterion in %s cites need %s which does not exist in intake: %s", ac.SliceID, nid, truncate(ac.Text, 60)),
				})
			}
		}
		// Check that the slice has at least one test.
		if len(testsBySlice[ac.SliceID]) == 0 {
			vs = append(vs, Violation{
				Kind:   "orphaned_ac_no_test",
				Detail: fmt.Sprintf("acceptance criterion in %s has no linked test (slice has no required tests): %s", ac.SliceID, truncate(ac.Text, 60)),
			})
		}
	}

	// Check: slice with no vertical link.
	// The vertical floor is slice -> release goal. If the release has a
	// release goal (from intake), every slice satisfies the floor by
	// association. If there is no release goal, each slice must have an
	// explicit release benefit link.
	for _, s := range m.Slices {
		if m.ReleaseGoal == "" && s.ReleaseBenefit == "" && s.OrgObjective == "" {
			vs = append(vs, Violation{
				Kind:   "slice_no_vertical",
				Detail: fmt.Sprintf("slice %s has no vertical link (no release goal in intake and no release benefit or org objective on the slice)", s.ID),
			})
		}
	}

	return vs
}

// Print renders the matrix as a human-readable text table to stdout.
func Print(m *Matrix) string {
	var b strings.Builder

	b.WriteString(style.Heading(fmt.Sprintf("Requirements Traceability Matrix: %s", m.Release)) + "\n")
	b.WriteString(style.Dim(strings.Repeat("=", 60)) + "\n\n")

	// Horizontal trace: need -> AC -> test -> proof
	b.WriteString(style.Dim("Horizontal trace (need -> AC -> test -> proof)") + "\n")
	b.WriteString(style.Dim(strings.Repeat("-", 60)) + "\n")

	if len(m.Needs) == 0 {
		b.WriteString("  (no needs found in intake.md)\n\n")
	} else {
		for _, n := range m.Needs {
			b.WriteString(fmt.Sprintf("  Need %s: %s\n", style.Accent(n.ID), n.Description))
			// Find ACs linked to this need.
			linkedACs := 0
			for _, ac := range m.ACs {
				for _, nid := range ac.NeedIDs {
					if nid == n.ID {
						b.WriteString(fmt.Sprintf("    -> AC [%s]: %s\n", ac.SliceID, truncate(ac.Text, 70)))
						// Find tests for this slice.
						for _, t := range m.Tests {
							if t.SliceID == ac.SliceID {
								b.WriteString(fmt.Sprintf("       -> test: %s\n", truncate(t.Text, 70)))
							}
						}
						linkedACs++
					}
				}
			}
			if linkedACs == 0 {
				b.WriteString("    -> (no linked acceptance criterion)\n")
			}
		}
	}
	b.WriteString("\n")

	// Vertical trace: org objective -> release benefit -> slice
	b.WriteString(style.Dim("Vertical trace (objective -> release benefit -> slice)") + "\n")
	b.WriteString(style.Dim(strings.Repeat("-", 60)) + "\n")
	if m.OrgObjective != "" {
		b.WriteString(fmt.Sprintf("  Objective: %s\n", m.OrgObjective))
	} else {
		b.WriteString("  Objective: (none declared — solo/small-team floor)\n")
	}
	if m.ReleaseBenefit != "" {
		b.WriteString(fmt.Sprintf("  Release benefit: %s\n", m.ReleaseBenefit))
	}
	if m.ReleaseGoal != "" {
		b.WriteString(fmt.Sprintf("  Release goal: %s\n", truncate(m.ReleaseGoal, 70)))
	}
	b.WriteString("\n")
	for _, s := range m.Slices {
		b.WriteString(fmt.Sprintf("  Slice %s", style.Accent(s.ID)))
		if s.ReleaseBenefit != "" {
			b.WriteString(fmt.Sprintf(" -> benefit: %s", truncate(s.ReleaseBenefit, 50)))
		} else if m.ReleaseGoal != "" {
			b.WriteString(" -> (floor: release goal)")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// --- parsing helpers ---

var (
	// Need id pattern: N-NN (e.g. N-01, N-12). Stable, never reused.
	reNeedID = regexp.MustCompile(`\bN-\d{2}\b`)
	// Need declaration in intake.md: a line like "- N-01: description" or
	// "N-01: description" within a needs section.
	reNeedDecl = regexp.MustCompile(`(?m)^\s*[-*]?\s*(N-\d{2})\s*[:\-]\s*(.+)$`)
	// Slice id pattern: S<NN>-<kebab-name>
	reSliceID = regexp.MustCompile(`^S\d{2}-[a-z0-9-]+$`)
	// Acceptance check: a checkbox line starting with "- [ ]" in the
	// "Acceptance checks" section.
	reACLine = regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*(.+)`)
	// Need id reference within an AC text (cited as "N-01" etc.)
	reNeedRef = regexp.MustCompile(`\bN-\d{2}\b`)
)

// isSliceID returns true if s matches the slice-id pattern S<NN>-<kebab-name>.
func isSliceID(s string) bool {
	return reSliceID.MatchString(s)
}

// parseNeeds extracts need declarations from intake.md text. Needs are
// declared as lines like "- N-01: description" within the intake.
func parseNeeds(text string) []Need {
	var needs []Need
	seen := map[string]bool{}
	for _, m := range reNeedDecl.FindAllStringSubmatch(text, -1) {
		id := m[1]
		desc := strings.TrimSpace(m[2])
		if seen[id] {
			continue
		}
		seen[id] = true
		needs = append(needs, Need{ID: id, Description: desc})
	}
	return needs
}

// parseReleaseGoal extracts the release goal from intake.md. The release goal
// is the first non-empty paragraph after the "## Release goal" heading.
func parseReleaseGoal(text string) string {
	return parseFirstParagraphAfterHeading(text, "## Release goal")
}

// parseReleaseBenefit extracts the release benefit from index.md. The release
// benefit is the first non-empty paragraph after the "## Release benefit"
// heading, or the "Goal" line in the Release summary section.
func parseReleaseBenefit(text string) string {
	// Try "## Release benefit" heading first.
	if s := parseFirstParagraphAfterHeading(text, "## Release benefit"); s != "" {
		return s
	}
	// Fall back to the "Goal" line in the Release summary section.
	// Pattern: "- **Goal**: <text>"
	reGoal := regexp.MustCompile(`(?m)^\s*-\s*\*\*Goal\*\*\s*:\s*(.+)$`)
	if m := reGoal.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// parseOrgObjective extracts the org objective from index.md. The org
// objective is the first non-empty paragraph after the "## Org objective"
// heading. Returns empty string if no objective is declared (solo/small-team).
func parseOrgObjective(text string) string {
	return parseFirstParagraphAfterHeading(text, "## Org objective")
}

// parseAcceptanceChecks extracts acceptance criteria from a spec.md text.
// Acceptance checks are checkbox lines ("- [ ] ...") in the "Acceptance
// checks" section. Each AC is parsed for need id references (N-NN).
func parseAcceptanceChecks(sliceID, text string) []AcceptanceCriterion {
	var acs []AcceptanceCriterion
	inSection := false
	for _, line := range strings.Split(text, "\n") {
		// Detect section boundaries.
		if strings.HasPrefix(line, "## ") {
			inSection = strings.Contains(strings.ToLower(line), "acceptance check")
			continue
		}
		if !inSection {
			continue
		}
		if m := reACLine.FindStringSubmatch(line); m != nil {
			acText := strings.TrimSpace(m[1])
			needIDs := extractNeedRefs(acText)
			acs = append(acs, AcceptanceCriterion{
				SliceID: sliceID,
				Text:    acText,
				NeedIDs: needIDs,
			})
		}
	}
	return acs
}

// parseRequiredTests extracts required tests from a spec.md text. Required
// tests are bullet lines in the "Required tests" section.
func parseRequiredTests(sliceID, text string) []Test {
	var tests []Test
	inSection := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			inSection = strings.Contains(strings.ToLower(line), "required test")
			continue
		}
		if !inSection {
			continue
		}
		// Bullet line: "- **Unit**: ..." or "- ..." or "- **Integration**: ..."
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			testText := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			// Skip sub-bullets (indented further).
			if testText != "" {
				tests = append(tests, Test{SliceID: sliceID, Text: testText})
			}
		}
	}
	return tests
}

// parseSliceReleaseBenefit extracts the release benefit link from a
// status.json text. The field is "release_benefit" in the JSON.
func parseSliceReleaseBenefit(text string) string {
	// Simple regex extraction — avoids importing encoding/json for a single
	// field, and the status.json is already parsed by the state package
	// elsewhere. Here we just need the one field.
	re := regexp.MustCompile(`"release_benefit"\s*:\s*"([^"]*)"`)
	if m := re.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// parseSliceOrgObjective extracts the org objective link from a status.json
// text. The field is "org_objective" in the JSON.
func parseSliceOrgObjective(text string) string {
	re := regexp.MustCompile(`"org_objective"\s*:\s*"([^"]*)"`)
	if m := re.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// extractNeedRefs finds all need id references (N-NN) in a text.
func extractNeedRefs(text string) []string {
	matches := reNeedRef.FindAllString(text, -1)
	seen := map[string]bool{}
	var ids []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			ids = append(ids, m)
		}
	}
	return ids
}

// parseFirstParagraphAfterHeading returns the first non-empty paragraph after
// a markdown heading. A paragraph is text until the next blank line or
// heading.
func parseFirstParagraphAfterHeading(text, heading string) string {
	lines := strings.Split(text, "\n")
	inSection := false
	var para []string
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if inSection {
				break
			}
			if strings.HasPrefix(line, heading) {
				inSection = true
			}
			continue
		}
		if !inSection {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		// Skip sub-headings (### etc.) and metadata.
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ">") {
			continue
		}
		para = append(para, trimmed)
	}
	if len(para) == 0 {
		return ""
	}
	return strings.Join(para, " ")
}

// truncate shortens s to at most n characters, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
