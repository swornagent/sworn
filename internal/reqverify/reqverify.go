// Package reqverify implements the requirements-quality verification gate.
//
// It grades every acceptance criterion in a release against the ISO/IEC/IEEE
// 29148:2018 quality characteristics (singular, unambiguous, complete,
// consistent, feasible, verifiable, necessary) using a fresh-context model
// pass.  It judges well-formedness only — never intent-correctness (that is
// S05 / validation).
//
// Fail-closed: any characteristic breach on any AC yields a non-zero exit.
package reqverify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/style"
)

// Characteristic is a 29148 quality characteristic for requirements.
type Characteristic string

const (
	CharSingular    Characteristic = "singular"
	CharUnambiguous Characteristic = "unambiguous"
	CharAmbiguous   Characteristic = "ambiguous"
	CharComplete    Characteristic = "complete"
	CharConsistent  Characteristic = "consistent"
	CharFeasible    Characteristic = "feasible"
	CharVerifiable  Characteristic = "verifiable"
	CharNecessary   Characteristic = "necessary"
)

// AllCharacteristics lists the seven quality characteristics in definition order.
var AllCharacteristics = []Characteristic{
	CharSingular,
	CharUnambiguous,
	CharComplete,
	CharConsistent,
	CharFeasible,
	CharVerifiable,
	CharNecessary,
}

// Violation records a characteristic breach for one acceptance criterion.
type Violation struct {
	SliceID        string
	ACIndex        int // 1-based within the slice
	ACContent      string
	Characteristic Characteristic
	Reason         string
}

// Grade is the per-AC result after model grading.
type Grade struct {
	SliceID   string
	ACIndex   int
	ACContent string
	Passed    bool
	Violation *Violation // non-nil when Passed is false
}

// Report aggregates grades across all slices in the release.
type Report struct {
	Grades       []Grade
	Violations   []Violation
	TotalACs     int
	PassedACs    int
	FailedACs    int
	FreshContext bool // records that a fresh-context model pass was used
}

// HasViolations returns true when at least one characteristic breach exists.
func (r Report) HasViolations() bool { return len(r.Violations) > 0 }

// AC is an individual acceptance criterion extracted from a spec.md.
type AC struct {
	SliceID string
	Index   int    // 1-based within the slice
	Content string // the AC text without the checkbox marker
}

// Verifier is the model interface reqverify needs — a subset of model.Verifier
// so the package has no dependency on the model package.
type Verifier interface {
	Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, int64, int64, error)
}
// Run executes requirements verification over a release directory.
//
// It discovers every slice's spec.md, extracts all acceptance criteria, builds
// a payload, dispatches it to the model with the requirements-verifier prompt,
// parses the per-AC grades, and returns the aggregated Report.
//
// The releaseDir is the path to docs/release/<name>.
func Run(ctx context.Context, releaseDir string, verifier Verifier, systemPrompt string) (Report, error) {
	report := Report{FreshContext: true}

	// 1. Discover slices and extract ACs.
	acs, err := extractACs(releaseDir)
	if err != nil {
		return report, fmt.Errorf("reqverify: extracting ACs: %w", err)
	}
	report.TotalACs = len(acs)
	if len(acs) == 0 {
		// No ACs to verify — trivially passes.
		return report, nil
	}

	// 2. Build the model payload.
	payload := buildPayload(acs)

	// 3. Dispatch to model.
	reply, _, _, _, err := verifier.Verify(ctx, systemPrompt, payload)
	if err != nil {
		return report, fmt.Errorf("reqverify: model dispatch: %w", err)	}

	// 4. Parse per-AC grades from the model response.
	grades, err := parseGrades(reply, acs)
	if err != nil {
		return report, fmt.Errorf("reqverify: parsing model response: %w", err)
	}

	// 5. Aggregate.
	report.Grades = grades
	for _, g := range grades {
		if g.Passed {
			report.PassedACs++
		} else {
			report.FailedACs++
			if g.Violation != nil {
				report.Violations = append(report.Violations, *g.Violation)
			}
		}
	}

	return report, nil
}

// extractACs reads all spec.md files under the release directory and extracts
// acceptance criteria (checkbox lines under "## Acceptance checks").
func extractACs(releaseDir string) ([]AC, error) {
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, err
	}

	// Sort slice directories for deterministic order.
	var sliceDirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Accept only names starting with "S" (slice pattern).
		if !strings.HasPrefix(e.Name(), "S") {
			continue
		}
		sliceDirs = append(sliceDirs, e.Name())
	}
	sort.Strings(sliceDirs)

	var allACs []AC
	for _, sliceID := range sliceDirs {
		specPath := filepath.Join(releaseDir, sliceID, "spec.md")
		data, err := os.ReadFile(specPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // slice dir with no spec.md yet
			}
			return nil, fmt.Errorf("reading %s: %w", specPath, err)
		}
		acs := parseACs(string(data), sliceID)
		allACs = append(allACs, acs...)
	}

	return allACs, nil
}

// checkboxRe matches markdown checkbox lines: "- [ ] ..." or "- [x] ...".
var checkboxRe = regexp.MustCompile(`^- \[[ xX]\]\s+(.*)`)

// acceptanceChecksHeader matches the "## Acceptance checks" section header.
var acceptanceChecksHeader = regexp.MustCompile(`(?i)^##\s+acceptance\s+checks`)

// parseACs extracts checkbox lines from within the "## Acceptance checks"
// section of a spec.md.
func parseACs(spec string, sliceID string) []AC {
	lines := strings.Split(spec, "\n")
	var inSection bool
	var acs []AC
	idx := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section header.
		if acceptanceChecksHeader.MatchString(trimmed) {
			inSection = true
			continue
		}

		// Stop at the next top-level heading (## ...) that is NOT acceptance checks.
		if inSection && strings.HasPrefix(trimmed, "## ") && !acceptanceChecksHeader.MatchString(trimmed) {
			break
		}

		if !inSection {
			continue
		}

		// Match checkbox line.
		m := checkboxRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}

		idx++
		content := strings.TrimSpace(m[1])
		if content == "" {
			continue
		}

		acs = append(acs, AC{
			SliceID: sliceID,
			Index:   idx,
			Content: content,
		})
	}

	return acs
}

// buildPayload constructs the model payload from extracted ACs.
func buildPayload(acs []AC) string {
	var b strings.Builder

	// Group ACs by slice.
	type sliceGroup struct {
		SliceID string
		ACs     []AC
	}
	groupMap := make(map[string][]AC)
	var sliceOrder []string
	for _, ac := range acs {
		if _, ok := groupMap[ac.SliceID]; !ok {
			sliceOrder = append(sliceOrder, ac.SliceID)
		}
		groupMap[ac.SliceID] = append(groupMap[ac.SliceID], ac)
	}

	for _, sliceID := range sliceOrder {
		group := groupMap[sliceID]
		fmt.Fprintf(&b, "### Slice: %s\n\n", sliceID)
		for _, ac := range group {
			fmt.Fprintf(&b, "AC %d: %s\n", ac.Index, ac.Content)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// acResultRe parses a single model output line: AC <N> (<slice-id>): PASS|FAIL...
var acResultRe = regexp.MustCompile(`^AC\s+(\d+)\s+\(([^)]+)\):\s*(PASS|FAIL)`)

// parseGrades interprets the model's response and assigns a Grade per AC.
//
// The model is expected to return a "## RESULTS" section with lines in format:
//
//	AC <N> (<slice-id>): PASS
//	AC <N> (<slice-id>): FAIL — <characteristic> [<reason>]
//
// If the model response lacks a RESULTS section, we BLOCK. If an AC is missing
// from the results, we treat it as FAIL (fail-closed). If an AC has an
// unparseable result line, we treat it as FAIL.
func parseGrades(reply string, acs []AC) ([]Grade, error) {
	// Find the ## RESULTS section.
	resultsIdx := -1
	lines := strings.Split(reply, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "## RESULTS" {
			resultsIdx = i
			break
		}
	}

	if resultsIdx < 0 {
		// No RESULTS section — BLOCKED.
		return nil, fmt.Errorf("model response missing ## RESULTS section")
	}

	// Parse result lines after ## RESULTS.
	resultMap := make(map[string]bool)         // "sliceID:index" -> passed
	violationMap := make(map[string]Violation) // "sliceID:index" -> violation

	for i := resultsIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Stop at next top-level heading.
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "AC") {
			break
		}

		m := acResultRe.FindStringSubmatch(line)
		if m == nil {
			continue // skip non-matching lines
		}

		idx, _ := strconv.Atoi(m[1])
		sliceID := m[2]
		status := m[3]
		key := fmt.Sprintf("%s:%d", sliceID, idx)

		if status == "PASS" {
			resultMap[key] = true
		} else {
			// FAIL — extract characteristic and reason.
			resultMap[key] = false
			rest := line[len(m[0]):] // everything after "AC N (id): FAIL"
			rest = strings.TrimSpace(rest)
			var char Characteristic
			var reason string
			if strings.HasPrefix(rest, "—") || strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, "–") {
				rest = strings.TrimLeft(rest, "—–- ")
			}
			rest = strings.TrimSpace(rest)

			// The characteristic is the first word/token before any space or punctuation.
			if idx2 := strings.IndexAny(rest, " \t["); idx2 > 0 {
				char = Characteristic(rest[:idx2])
				reason = strings.TrimSpace(rest[idx2:])
			} else {
				char = Characteristic(rest)
			}

			violationMap[key] = Violation{
				SliceID:        sliceID,
				ACIndex:        idx,
				Characteristic: char,
				Reason:         reason,
			}
		}
	}

	// Map grades back to the ACs in order.
	var grades []Grade
	for _, ac := range acs {
		key := fmt.Sprintf("%s:%d", ac.SliceID, ac.Index)
		passed, ok := resultMap[key]
		if !ok {
			// AC not in model output — fail-closed.
			grades = append(grades, Grade{
				SliceID:   ac.SliceID,
				ACIndex:   ac.Index,
				ACContent: ac.Content,
				Passed:    false,
				Violation: &Violation{
					SliceID:        ac.SliceID,
					ACIndex:        ac.Index,
					ACContent:      ac.Content,
					Characteristic: "verifiable",
					Reason:         "AC missing from model response — fail-closed",
				},
			})
			continue
		}
		if passed {
			grades = append(grades, Grade{
				SliceID:   ac.SliceID,
				ACIndex:   ac.Index,
				ACContent: ac.Content,
				Passed:    true,
			})
		} else {
			v := violationMap[key]
			v.ACContent = ac.Content
			grades = append(grades, Grade{
				SliceID:   ac.SliceID,
				ACIndex:   ac.Index,
				ACContent: ac.Content,
				Passed:    false,
				Violation: &v,
			})
		}
	}

	return grades, nil
}

// Print formats the report for human-readable output.
func Print(report Report) string {
	var b strings.Builder

	fmt.Fprint(&b, style.Heading("Requirements verification report")+"\n")
	fmt.Fprint(&b, style.Dim("===============================")+"\n\n")

	if report.TotalACs == 0 {
		fmt.Fprintf(&b, "No acceptance criteria to verify.\n")
		return b.String()
	}

	fmt.Fprint(&b, style.Accent(fmt.Sprintf("Total ACs: %d | Passed: %d | Failed: %d",
		report.TotalACs, report.PassedACs, report.FailedACs))+"\n\n")

	if report.FreshContext {
		fmt.Fprintf(&b, "Verifier mode: fresh-context (requirements-verifier prompt)\n\n")
	}

	if report.HasViolations() {
		fmt.Fprint(&b, style.Danger("Violations:")+"\n")
		for _, v := range report.Violations {
			fmt.Fprintf(&b, "  AC %d (%s): %s — %s\n",
				v.ACIndex, v.SliceID, v.Characteristic, v.Reason)
		}
		fmt.Fprintln(&b)
	}

	// Per-AC grade table.
	fmt.Fprintf(&b, "Per-AC grades:\n")
	for _, g := range report.Grades {
		if g.Passed {
			fmt.Fprint(&b, style.Success(fmt.Sprintf("  AC %d (%s): PASS", g.ACIndex, g.SliceID))+"\n")
		} else if g.Violation != nil {
			fmt.Fprint(&b, style.Danger(fmt.Sprintf("  AC %d (%s): FAIL — %s", g.ACIndex, g.SliceID, g.Violation.Characteristic))+"\n")
		} else {
			fmt.Fprint(&b, style.Danger(fmt.Sprintf("  AC %d (%s): FAIL", g.ACIndex, g.SliceID))+"\n")
		}
	}

	return b.String()
}

// PrintCompact formats a one-line summary for use by the CLI.
func PrintCompact(report Report) string {
	if report.TotalACs == 0 {
		return "reqverify: no acceptance criteria to verify"
	}
	summary := fmt.Sprintf("reqverify: %d ACs — %d passed, %d failed",
		report.TotalACs, report.PassedACs, report.FailedACs)
	if report.HasViolations() {
		summary += " — FAILED"
	} else {
		summary += " — PASSED"
	}
	return summary
}
