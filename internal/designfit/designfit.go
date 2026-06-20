// Package designfit implements the design-fit gate (Rule 9): stakes-calibrated,
// human-owned design-fit decisions. It reads each slice's design_decisions from
// status.json and fails closed when any Type-1 (high-stakes) choice lacks a
// recorded human decision.
//
// Stdlib only — zero runtime dependencies.
package designfit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/state"
)

// Violation records one design-fit violation for a single slice.
type Violation struct {
	SliceID     string
	ChoiceName  string
	StakeClass  state.StakeClass
	Description string
}

// String returns a human-readable violation line.
func (v Violation) String() string {
	if v.ChoiceName != "" {
		return fmt.Sprintf("%s: %s choice %q %s", v.SliceID, v.StakeClass, v.ChoiceName, v.Description)
	}
	return fmt.Sprintf("%s: %s", v.SliceID, v.Description)
}

// Report holds all design-fit violations for a release.
type Report struct {
	Release       string      `json:"release"`
	Violations    []Violation `json:"violations"`
	SlicesChecked int         `json:"slices_checked"`
}

// HasViolations returns true when the report contains at least one violation.
func (r *Report) HasViolations() bool {
	return len(r.Violations) > 0
}

// Print renders the report to a human-readable string.
func Print(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Design-fit gate report for release %q\n", r.Release)
	fmt.Fprintf(&b, "Slices checked: %d\n\n", r.SlicesChecked)

	if !r.HasViolations() {
		fmt.Fprintln(&b, "All design decisions have recorded human decisions where required — PASS.")
		return b.String()
	}

	fmt.Fprintf(&b, "%d design-fit violation(s) found:\n\n", len(r.Violations))
	for i, v := range r.Violations {
		fmt.Fprintf(&b, "%d. %s\n", i+1, v.String())
	}
	fmt.Fprintln(&b, "\nEach Type-1 (high-stakes) design choice requires a recorded human decision.")
	fmt.Fprintln(&b, "Run `/replan-release` or have the human record the decision before proceeding.")
	return b.String()
}

// PrintCompact prints a one-line summary suitable for stderr/CI parsing.
func PrintCompact(r *Report) string {
	if r.HasViolations() {
		return fmt.Sprintf("DESIGNFIT FAIL — %d violation(s) across %d slice(s)",
			len(r.Violations), r.SlicesChecked)
	}
	return fmt.Sprintf("DESIGNFIT PASS — %d slice(s) checked, all design-fit gates clear",
		r.SlicesChecked)
}

// Run checks every slice in the release for design-fit violations.
//
// For each slice, it reads the status.json from <releaseDir>/<slice-id>/
// and inspects the DesignDecisions field. A violation is recorded when:
//   - A Type-1 (high-stakes) decision has no human_decision
//   - A decision is marked architecturally_significant but not Type-1
//
// Returns one Report covering the entire release.
func Run(releaseDir string) (*Report, error) {
	// Extract release name from the directory path.
	releaseName := filepath.Base(releaseDir)

	report := &Report{
		Release: releaseName,
	}

	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, fmt.Errorf("designfit: read release dir %s: %w", releaseDir, err)
	}

	// Collect and sort slice directories by id for deterministic output.
	var sliceDirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		// Skip non-slice directories (e.g. screenshots/).
		if !strings.HasPrefix(id, "S") || !strings.Contains(id, "-") {
			continue
		}
		sliceDirs = append(sliceDirs, id)
	}
	sort.Strings(sliceDirs)

	for _, sliceID := range sliceDirs {
		statusPath := filepath.Join(releaseDir, sliceID, "status.json")

		st, err := state.Read(statusPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // slice without status.json is skipped
			}
			return nil, fmt.Errorf("designfit: read %s: %w", statusPath, err)
		}

		report.SlicesChecked++

		if len(st.DesignDecisions) == 0 {
			// No design decisions means no design-fit gate to enforce.
			continue
		}

		for _, dd := range st.DesignDecisions {
			// Check 1: architecturally-significant choices must be Type-1.
			if dd.ArchitecturallySignificant && dd.StakeClass == state.Type2 {
				report.Violations = append(report.Violations, Violation{
					SliceID:    sliceID,
					ChoiceName: dd.Choice,
					StakeClass: dd.StakeClass,
					Description: fmt.Sprintf(
						"is architecturally-significant but classified as Type-2 — must be Type-1",
					),
				})
				continue
			}

			// Check 2: Type-1 choices must have a human decision.
			if dd.StakeClass == state.Type1 && dd.HumanDecision == "" {
				report.Violations = append(report.Violations, Violation{
					SliceID:    sliceID,
					ChoiceName: dd.Choice,
					StakeClass: dd.StakeClass,
					Description: "has no recorded human decision — a Type-1 choice requires human judgement",
				})
			}
		}
	}

	return report, nil
}