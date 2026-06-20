// Package specquality implements the deterministic, pre-code spec-quality
// first-pass: soundness + completeness metrics computed from a slice's
// acceptance examples alone, with no source code and no model call.
//
// Soundness measures whether the acceptance criteria accept every valid
// example output (no false rejection). Completeness uses mutation analysis:
// each example's expected output is mutated, and the fraction of mutations
// the criteria would reject is the completeness score. Below a configurable
// threshold, the gate fails closed.
//
// Stdlib only — zero runtime dependencies.
package specquality

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/state"
)

// DefaultThreshold is the minimum completeness score (0.0–1.0) required to
// pass the gate. Slices below this score are flagged as violations.
const DefaultThreshold = 0.5

// Example is one input → expected-output pair from a slice's spec.md.
// The structured format is parsed from the ## Acceptance examples section.
type Example struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

// SliceResult holds the computed metrics for one slice.
type SliceResult struct {
	SliceID      string      `json:"slice_id"`
	Soundness    float64     `json:"soundness"`    // 0.0–1.0
	Completeness float64     `json:"completeness"` // 0.0–1.0
	ExampleCount int         `json:"example_count"`
	Violations   []Violation `json:"violations,omitempty"`
}

// Violation records a spec-quality failure for one slice.
type Violation struct {
	SliceID string `json:"slice_id"`
	Reason  string `json:"reason"`
}

// Report aggregates spec-quality results across all slices in a release.
type Report struct {
	Results    []SliceResult `json:"results"`
	Threshold  float64       `json:"threshold"`
	Passed     bool          `json:"passed"`
	TotalScore float64       `json:"total_score,omitempty"`
}

// Run executes the spec-quality first-pass over a release directory.
// releaseDir is the path to docs/release/<name>.
// threshold is the minimum completeness score (0.0–1.0); use DefaultThreshold
// when the caller has no configured value.
func Run(releaseDir string, threshold float64) (Report, error) {
	report := Report{Threshold: threshold}

	if threshold <= 0 {
		threshold = DefaultThreshold
	}

	report.Passed = true

	// 1. Discover slice directories.
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return report, fmt.Errorf("specquality: reading release dir: %w", err)
	}

	var sliceDirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), "S") {
			continue
		}
		sliceDirs = append(sliceDirs, e.Name())
	}
	sort.Strings(sliceDirs)

	if len(sliceDirs) == 0 {
		return report, nil
	}

	// 2. Evaluate each slice.
	totalCompleteness := 0.0
	for _, sliceID := range sliceDirs {
		result := evaluateSlice(releaseDir, sliceID, threshold)
		report.Results = append(report.Results, result)
		totalCompleteness += result.Completeness
		if !resultPassed(result) {
			report.Passed = false
		}
	}

	if len(report.Results) > 0 {
		report.TotalScore = totalCompleteness / float64(len(report.Results))
	}

	return report, nil
}

// resultPassed returns true when the slice result has no violations.
func resultPassed(r SliceResult) bool {
	return len(r.Violations) == 0
}

// evaluateSlice computes soundness + completeness for one slice.
func evaluateSlice(releaseDir, sliceID string, threshold float64) SliceResult {
	result := SliceResult{SliceID: sliceID}

	specPath := filepath.Join(releaseDir, sliceID, "spec.md")
	examples := parseExamples(specPath)

	if len(examples) == 0 {
		result.Violations = append(result.Violations, Violation{
			SliceID: sliceID,
			Reason:  "no acceptance examples found — planner must add structured examples to the ## Acceptance examples section",
		})
		result.Soundness = 0
		result.Completeness = 0
		result.ExampleCount = 0
		return result
	}

	result.ExampleCount = len(examples)

	// Read the slice's status.json to get acceptance criteria text for
	// mutation analysis. The criteria are the ground truth we check against.
	statusPath := filepath.Join(releaseDir, sliceID, "status.json")
	status, err := state.Read(statusPath)
	if err != nil {
		result.Violations = append(result.Violations, Violation{
			SliceID: sliceID,
			Reason:  fmt.Sprintf("cannot read status.json: %v", err),
		})
		return result
	}

	// Parse acceptance criteria from spec.md.
	// We extract the AC text block from the spec for comparison.
	criteriaText := extractCriteriaText(specPath)

	// Soundness: every example's expected output must not contradict the criteria.
	// We implement a limited deterministic check: verify that the expected output
	// uses terminology consistent with the criteria and that no example's expected
	// output describes a behaviour the criteria would reject.
	soundnessViolations := computeSoundness(examples, criteriaText, status)
	if len(soundnessViolations) > 0 {
		result.Violations = append(result.Violations, soundnessViolations...)
		result.Soundness = float64(len(examples)-len(soundnessViolations)) / float64(len(examples))
	} else {
		result.Soundness = 1.0
	}

	// Completeness (mutation analysis): for each example, generate mutated
	// versions of the expected output and check what fraction the criteria
	// would reject.
	result.Completeness = computeCompleteness(examples, criteriaText)

	// Threshold gate.
	if result.Completeness < threshold {
		result.Violations = append(result.Violations, Violation{
			SliceID: sliceID,
			Reason: fmt.Sprintf(
				"completeness score %.0f%% is below threshold %.0f%% — acceptance examples do not catch enough output mutations",
				result.Completeness*100, threshold*100,
			),
		})
	}

	return result
}

// mutation operators: simple text mutations applied to expected outputs.
// Each operator returns a mutated version of the input string, or "" if
// this operator does not apply.
var mutationOperators = []struct {
	Name string
	Apply func(string) string
}{
	{"flip_exit_code", mutateFlipExitCode},
	{"negate_assertion", mutateNegateAssertion},
	{"remove_keyword", mutateRemoveKeyword},
	{"uppercase", mutateUppercase},
	{"lowercase", mutateLowercase},
	{"swap_zero_one", mutateSwapZeroOne},
}

// mutateFlipExitCode flips "exit 0" ↔ "exit 1" or "exit code 0" ↔ "exit code 1".
func mutateFlipExitCode(s string) string {
	if strings.Contains(s, "exit 0") {
		return strings.Replace(s, "exit 0", "exit 1", 1)
	}
	if strings.Contains(s, "exit 1") {
		return strings.Replace(s, "exit 1", "exit 0", 1)
	}
	if strings.Contains(s, "exits 0") {
		return strings.Replace(s, "exits 0", "exits 1", 1)
	}
	if strings.Contains(s, "exits 1") {
		return strings.Replace(s, "exits 1", "exits 0", 1)
	}
	if strings.Contains(s, "exit code 0") {
		return strings.Replace(s, "exit code 0", "exit code 1", 1)
	}
	if strings.Contains(s, "exit code 1") {
		return strings.Replace(s, "exit code 1", "exit code 0", 1)
	}
	return ""
}

// mutateNegateAssertion flips PASS ↔ FAIL, pass ↔ fail, passing ↔ failing.
func mutateNegateAssertion(s string) string {
	if strings.Contains(s, "PASS") {
		return strings.Replace(s, "PASS", "FAIL", 1)
	}
	if strings.Contains(s, "FAIL") {
		return strings.Replace(s, "FAIL", "PASS", 1)
	}
	if strings.Contains(s, " pass ") {
		return strings.Replace(s, " pass ", " fail ", 1)
	}
	if strings.Contains(s, " fail ") {
		return strings.Replace(s, " fail ", " pass ", 1)
	}
	return ""
}

// mutateRemoveKeyword removes a key phrase to simulate incomplete/inadequate output.
// It removes known strong signal words from the expected output.
func mutateRemoveKeyword(s string) string {
	keywords := []string{
		"all", "every", "each", "no", "never", "must",
		"violation", "error", "fail", "pass", "valid",
	}
	for _, kw := range keywords {
		repl := " " + kw + " "
		if strings.Contains(s, repl) {
			return strings.Replace(s, repl, " ", 1)
		}
	}
	// Also try keyword at start.
	for _, kw := range keywords {
		if strings.HasPrefix(s, kw+" ") {
			return strings.TrimPrefix(s, kw+" ")
		}
	}
	return ""
}

// mutateUppercase uppercases a section of the expected output.
func mutateUppercase(s string) string {
	if len(s) < 5 {
		return ""
	}
	// Uppercase the first 5-8 characters.
	end := 8
	if len(s) < end {
		end = len(s)
	}
	return strings.ToUpper(s[:end]) + s[end:]
}

// mutateLowercase lowercases a section of the expected output.
func mutateLowercase(s string) string {
	if len(s) < 5 {
		return ""
	}
	end := 8
	if len(s) < end {
		end = len(s)
	}
	return strings.ToLower(s[:end]) + s[end:]
}

// mutateSwapZeroOne swaps "0" ↔ "1" in the output.
func mutateSwapZeroOne(s string) string {
	if strings.Contains(s, " 0 ") {
		return strings.Replace(s, " 0 ", " 1 ", 1)
	}
	if strings.Contains(s, " 1 ") {
		return strings.Replace(s, " 1 ", " 0 ", 1)
	}
	return ""
}

// computeSoundness checks every example's expected output against the
// acceptance criteria text. Returns violations for any expected output
// that contradicts the criteria (deterministic checks only).
func computeSoundness(examples []Example, criteriaText string, status *state.Status) []Violation {
	var violations []Violation

	for _, ex := range examples {
		expected := strings.ToLower(strings.TrimSpace(ex.Expected))
		criteria := strings.ToLower(criteriaText)
		sliceID := status.SliceID

		// Check 1: expected output contains "violation" or "fail" but the
		// criteria only describe a pass case (no mention of failure conditions).
		if strings.Contains(expected, "violation") || strings.Contains(expected, "fail") {
			if !strings.Contains(criteria, "fail") && !strings.Contains(criteria, "violation") &&
				!strings.Contains(criteria, "non-zero") && !strings.Contains(criteria, "exit 1") {
				violations = append(violations, Violation{
					SliceID: sliceID,
					Reason: fmt.Sprintf(
						"example %q expects failure but criteria describe only pass case",
						ex.Name,
					),
				})
			}
		}

		// Check 2: expected output claims a different tool or command name than the
		// criteria reference. This catches copy-paste errors where the example
		// describes a different command than the slice implements.
		expectedCmds := extractCommandRefs(expected)
		criteriaCmds := extractCommandRefs(criteria)
		if len(expectedCmds) > 0 && len(criteriaCmds) > 0 {
			hasMatch := false
			for _, ec := range expectedCmds {
				for _, cc := range criteriaCmds {
					if ec == cc {
						hasMatch = true
						break
					}
				}
			}
			if !hasMatch {
				violations = append(violations, Violation{
					SliceID: sliceID,
					Reason: fmt.Sprintf(
						"example %q references command(s) %v not found in criteria %v",
						ex.Name, expectedCmds, criteriaCmds,
					),
				})
			}
		}
	}

	return violations
}

// extractCommandRefs extracts sworn command references (like "lint", "verify",
// "reqverify", etc.) from a string.
func extractCommandRefs(s string) []string {
	known := []string{
		"sworn lint", "sworn verify", "sworn run", "sworn init",
		"sworn bench", "sworn reqverify", "sworn reqvalidate",
		"sworn designfit", "sworn journeys", "sworn specquality",
		"sworn lint ac", "sworn lint trace",
	}
	var found []string
	for _, cmd := range known {
		if strings.Contains(s, cmd) {
			found = append(found, cmd)
		}
	}
	return found
}

// computeCompleteness performs mutation analysis on each example's expected
// output and returns the fraction of mutations the criteria would reject.
func computeCompleteness(examples []Example, criteriaText string) float64 {
	if len(examples) == 0 {
		return 0
	}

	totalMutations := 0
	caughtMutations := 0
	criteria := strings.ToLower(criteriaText)

	for _, ex := range examples {
		expected := strings.ToLower(strings.TrimSpace(ex.Expected))
		if expected == "" {
			continue
		}

		for _, op := range mutationOperators {
			mutated := op.Apply(expected)
			if mutated == "" || mutated == expected {
				continue
			}
			totalMutations++

			// Does the criteria text reject the mutated output?
			// We check if the mutated output contains behaviour the criteria flag.
			mutatedLower := strings.ToLower(mutated)
			if criteriaWouldReject(criteria, expected, mutatedLower) {
				caughtMutations++
			}
		}
	}

	if totalMutations == 0 {
		return 0
	}
	return float64(caughtMutations) / float64(totalMutations)
}

// criteriaWouldReject checks whether the criteria text would flag the
// mutated output as wrong. Uses deterministic heuristics:
//   - If mutated flips a pass→fail, criteria must mention failure paths
//   - If mutated negates an assertion, criteria must check both pass+ fail
//   - If mutated swaps a keyword, criteria must be specific enough to catch it
//
// original is the unmutated expected output for comparison.
// mutated is the mutated version.
// criteria is the criteria text (lowercased).
func criteriaWouldReject(criteria, original, mutated string) bool {
	// If the mutated string differs meaningfully from the original,
	// check whether the criteria mentions the specific signals in the
	// original that the mutation removed.

	// The criteria should mention both sides of any assertion flip.
	if strings.Contains(original, "pass") && strings.Contains(mutated, "fail") {
		if strings.Contains(criteria, "fail") || strings.Contains(criteria, "non-zero") {
			return true
		}
	}
	if strings.Contains(original, "fail") && strings.Contains(mutated, "pass") {
		if strings.Contains(criteria, "pass") || strings.Contains(criteria, "exit 0") {
			return true
		}
	}

	// Exit code flip — criteria should mention both states.
	if (strings.Contains(original, "exit 0") && strings.Contains(mutated, "exit 1")) ||
		(strings.Contains(original, "exits 0") && strings.Contains(mutated, "exits 1")) {
		if strings.Contains(criteria, "non-zero") || strings.Contains(criteria, "exit 1") {
			return true
		}
	}
	if (strings.Contains(original, "exit 1") && strings.Contains(mutated, "exit 0")) ||
		(strings.Contains(original, "exits 1") && strings.Contains(mutated, "exits 0")) {
		if strings.Contains(criteria, "exit 0") || strings.Contains(criteria, "all") {
			return true
		}
	}

	// Keyword removal — if the criteria mentions the removed keyword, it
	// would catch the omission.
	removedWords := findRemovedWords(original, mutated)
	for _, word := range removedWords {
		if strings.Contains(criteria, word) {
			return true
		}
	}

	// Case mutations should be caught if criteria cares about case.
	if strings.ToUpper(original) == mutated || strings.ToLower(original[:minInt(8, len(original))])+original[minInt(8, len(original)):] == mutated {
		// Case mutations are usually not caught by criteria unless they
		// mention specific case. Return false conservatively.
		return false
	}

	// 0↔1 swap in numbers.
	if strings.Contains(original, "0") && strings.Contains(mutated, "1") {
		if strings.Contains(criteria, "exit 0") || strings.Contains(criteria, "zero") {
			return false // criteria describes the pass case; doesn't catch a 0→1 swap
		}
		// If criteria describes the failure case, 0→1 means the criteria
		// wouldn't flag it because it's describing the failure.
		if strings.Contains(criteria, "non-zero") && strings.Contains(mutated, "exit 1") {
			return true // criteria catches that exit 1 should happen
		}
	}

	return false
}

// findRemovedWords returns words present in original but absent from mutated.
func findRemovedWords(original, mutated string) []string {
	origWords := strings.Fields(original)
	mutWords := make(map[string]bool)
	for _, w := range strings.Fields(mutated) {
		mutWords[w] = true
	}
	var removed []string
	seen := make(map[string]bool)
	for _, w := range origWords {
		if !mutWords[w] && !seen[w] {
			removed = append(removed, w)
			seen[w] = true
		}
	}
	return removed
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseExamples extracts acceptance examples from a spec.md file.
//
// The format is a ## Acceptance examples section containing a YAML-like or
// table-structured list of input -> expected-output pairs. Example:
//
//	## Acceptance examples
//
//	- name: "well-formed-ears"
//	  input: "release with all EARS-format ACs"
//	  expected: "sworn lint ac exits 0 and reports all ACs well-formed"
//
//	- name: "free-form-ac"
//	  input: "release with a free-form AC"
//	  expected: "sworn lint ac exits 1 naming the slice and line"
func parseExamples(specPath string) []Example {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil
	}
	content := string(data)

	// Find the ## Acceptance examples section.
	sectionStart := strings.Index(content, "## Acceptance examples")
	if sectionStart < 0 {
		return nil
	}

	// Find the next ## section or end of file.
	sectionContent := content[sectionStart:]
	nextSection := strings.Index(sectionContent[3:], "## ")
	if nextSection >= 0 {
		sectionContent = sectionContent[:nextSection+3]
	}

	// Parse list items.
	lines := strings.Split(sectionContent, "\n")
	var examples []Example
	var current Example
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// If we're inside a multi-line structured example, process field lines.
		if inList && current.Name != "" && !strings.HasPrefix(trimmed, "- ") {
			if trimmed == "" {
				// Empty line — could be end of multi-line block or whitespace.
				continue
			}
			body := trimmed
			if strings.HasPrefix(body, "input:") || strings.HasPrefix(body, "input :") {
				val := strings.TrimSpace(strings.TrimPrefix(body, "input:"))
				val = strings.TrimSpace(strings.TrimPrefix(val, "input :"))
				current.Input = strings.Trim(val, `"'`)
			} else if strings.HasPrefix(body, "expected:") || strings.HasPrefix(body, "expected :") {
				val := strings.TrimSpace(strings.TrimPrefix(body, "expected:"))
				val = strings.TrimSpace(strings.TrimPrefix(val, "expected :"))
				current.Expected = strings.Trim(val, `"'`)
			}
			continue
		}

		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}

		// Save previous example before starting a new list item.
		if current.Name != "" {
			examples = append(examples, current)
			current = Example{}
		}

		// Strip the "- " prefix for parsing.
		body := strings.TrimSpace(trimmed[2:])

		// Check for structured format: "- name: ..."
		if strings.HasPrefix(body, "name:") || strings.HasPrefix(body, "name :") {
			val := strings.TrimSpace(strings.TrimPrefix(body, "name:"))
			val = strings.TrimSpace(strings.TrimPrefix(val, "name :"))
			current.Name = strings.Trim(val, `"'`)
			inList = true
			continue
		}

		// Check for shorthand: "- <input> → <expected>"
		if strings.Contains(body, "→") {
			parts := strings.SplitN(body, "→", 2)
			if len(parts) == 2 {
				ex := Example{
					Name:     fmt.Sprintf("E%d", len(examples)+1),
					Input:    strings.TrimSpace(parts[0]),
					Expected: strings.TrimSpace(parts[1]),
				}
				examples = append(examples, ex)
				inList = false
				continue
			}
		}
	}	// Save last example.
	if current.Name != "" {
		examples = append(examples, current)
	}

	return examples
}

// extractCriteriaText extracts the acceptance criteria section from a spec.md
// file, returning the full text for comparison.
func extractCriteriaText(specPath string) string {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return ""
	}
	content := string(data)

	// Find the ## Acceptance checks section.
	sectionStart := strings.Index(content, "## Acceptance checks")
	if sectionStart < 0 {
		return ""
	}

	// Find the next ## section or end of file.
	sectionContent := content[sectionStart:]
	nextSection := strings.Index(sectionContent[3:], "## ")
	if nextSection >= 0 {
		sectionContent = sectionContent[:nextSection+3]
	}

	return sectionContent
}

// Print formats the report for human-readable output.
func Print(report Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Spec-quality first-pass report\n")
	fmt.Fprintf(&b, "==============================\n\n")
	fmt.Fprintf(&b, "Threshold: %.0f%% completeness\n\n", report.Threshold*100)

	if len(report.Results) == 0 {
		fmt.Fprintf(&b, "No slices to evaluate.\n")
		return b.String()
	}

	for _, r := range report.Results {
		fmt.Fprintf(&b, "Slice: %s\n", r.SliceID)
		fmt.Fprintf(&b, "  Examples: %d\n", r.ExampleCount)
		fmt.Fprintf(&b, "  Soundness:  %.0f%%\n", r.Soundness*100)
		fmt.Fprintf(&b, "  Completeness: %.0f%%\n", r.Completeness*100)

		if len(r.Violations) > 0 {
			fmt.Fprintf(&b, "  Violations:\n")
			for _, v := range r.Violations {
				fmt.Fprintf(&b, "    - %s\n", v.Reason)
			}
		} else {
			fmt.Fprintf(&b, "  Status: PASS\n")
		}
		fmt.Fprintln(&b)
	}

	if report.Passed {
		fmt.Fprintf(&b, "Overall: PASSED (average completeness: %.0f%%)\n", report.TotalScore*100)
	} else {
		fmt.Fprintf(&b, "Overall: FAILED (average completeness: %.0f%%)\n", report.TotalScore*100)
	}

	return b.String()
}

// PrintCompact formats a one-line summary for use by the CLI.
func PrintCompact(report Report) string {
	passCount := 0
	failCount := 0
	for _, r := range report.Results {
		if resultPassed(r) {
			passCount++
		} else {
			failCount++
		}
	}

	summary := fmt.Sprintf("specquality: %d slices — %d passed, %d failed (threshold %.0f%% completeness)",
		len(report.Results), passCount, failCount, report.Threshold*100)

	if report.Passed {
		summary += " — PASSED"
	} else {
		summary += " — FAILED"
	}
	return summary
}