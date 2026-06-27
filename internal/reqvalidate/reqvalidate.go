// Package reqvalidate implements the requirements validation gate.
//
// It checks that every slice in a release carries a human-ratified validation
// record with positive + negative scenarios and a benefit/alignment hypothesis.
// Validation is "are we building the *right* requirements?" — the spec makes
// sense and serves the need. This gate is human-owned: the model drafts
// scenarios + a benefit hypothesis; the human ratifies. Per current research,
// spec validation has no oracle but the user, so this gate is never LLM
// self-certified — it reads status.json directly, with no model dispatch.
//
// Fail-closed: any slice without a complete, human-ratified record yields a
// non-zero exit.
package reqvalidate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/style"
)

// Violation records a validation failure for one slice.
type Violation struct {
	SliceID string `json:"slice_id"`
	Reason  string `json:"reason"`
}

// Report aggregates validation results across all slices in the release.
type Report struct {
	Violations      []Violation `json:"violations,omitempty"`
	TotalSlices     int         `json:"total_slices"`
	ValidatedSlices int         `json:"validated_slices"`
	FailedSlices    int         `json:"failed_slices"`
}

// HasViolations returns true when at least one validation failure exists.
func (r Report) HasViolations() bool { return len(r.Violations) > 0 }

// Run executes requirements validation over a release directory.
//
// It discovers every slice's status.json, checks the validation record, and
// returns an aggregated Report. releaseDir is the path to docs/release/<name>.
func Run(releaseDir string) (Report, error) {
	report := Report{}

	// 1. Discover slice directories.
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return report, fmt.Errorf("reqvalidate: reading release dir: %w", err)
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

	report.TotalSlices = len(sliceDirs)
	if len(sliceDirs) == 0 {
		return report, nil
	}

	// 2. Validate each slice.
	for _, sliceID := range sliceDirs {
		violations := validateSlice(releaseDir, sliceID)
		if len(violations) == 0 {
			report.ValidatedSlices++
		} else {
			report.FailedSlices++
			report.Violations = append(report.Violations, violations...)
		}
	}

	return report, nil
}

// validateSlice checks one slice's validation record and returns any violations.
func validateSlice(releaseDir, sliceID string) []Violation {
	var violations []Violation

	statusPath := filepath.Join(releaseDir, sliceID, "status.json")
	status, err := state.Read(statusPath)
	if err != nil {
		violations = append(violations, Violation{
			SliceID: sliceID,
			Reason:  fmt.Sprintf("cannot read status.json: %v", err),
		})
		return violations
	}

	v := status.Validation

	// Check 1: human ratification is present.
	if !v.HumanRatified {
		violations = append(violations, Violation{
			SliceID: sliceID,
			Reason:  "validation record missing human ratification (human_ratified is false or absent)",
		})
	}

	// Check 2: at least one positive scenario.
	if len(v.PositiveScenarios) == 0 {
		violations = append(violations, Violation{
			SliceID: sliceID,
			Reason:  "validation record has no positive scenarios",
		})
	}

	// Check 3: at least one negative/exception scenario.
	if len(v.NegativeScenarios) == 0 {
		violations = append(violations, Violation{
			SliceID: sliceID,
			Reason:  "validation record has no negative/exception scenarios",
		})
	}

	// Check 4: benefit/alignment hypothesis is present.
	if strings.TrimSpace(v.BenefitHypothesis) == "" {
		violations = append(violations, Violation{
			SliceID: sliceID,
			Reason:  "validation record missing benefit/alignment hypothesis",
		})
	}

	return violations
}

// Print formats the report for human-readable output.
func Print(report Report) string {
	var b strings.Builder

	fmt.Fprint(&b, style.Heading("Requirements validation report")+"\n")
	fmt.Fprint(&b, style.Dim("==============================")+"\n\n")

	if report.TotalSlices == 0 {
		fmt.Fprintf(&b, "No slices to validate.\n")
		return b.String()
	}

	fmt.Fprint(&b, style.Accent(fmt.Sprintf("Total slices: %d | Validated: %d | Failed: %d", report.TotalSlices, report.ValidatedSlices, report.FailedSlices))+"\n\n")

	if report.HasViolations() {
		fmt.Fprint(&b, style.Danger("Violations:")+"\n")
		for _, v := range report.Violations {
			fmt.Fprintf(&b, "  %s: %s\n", v.SliceID, v.Reason)
		}
		fmt.Fprintln(&b)
	}

	// Per-slice results.
	fmt.Fprintf(&b, "Per-slice results:\n")
	// Build a set of failing slices.
	failed := make(map[string]bool)
	for _, v := range report.Violations {
		failed[v.SliceID] = true
	}
	for sliceID := range failed {
		reasons := []string{}
		for _, v := range report.Violations {
			if v.SliceID == sliceID {
				reasons = append(reasons, v.Reason)
			}
		}
		fmt.Fprint(&b, style.Danger(fmt.Sprintf("  %s: FAIL — %s", sliceID, strings.Join(reasons, "; ")))+"\n")
	}
	passed := report.TotalSlices - len(failed)
	if passed > 0 {
		fmt.Fprint(&b, "\n"+style.Success(fmt.Sprintf("%d slice(s) fully validated.", passed))+"\n")
	}

	return b.String()
}

// PrintCompact formats a one-line summary for use by the CLI.
func PrintCompact(report Report) string {
	if report.TotalSlices == 0 {
		return "reqvalidate: no slices to validate"
	}
	summary := fmt.Sprintf("reqvalidate: %d slices — %d validated, %d failed",
		report.TotalSlices, report.ValidatedSlices, report.FailedSlices)
	if report.HasViolations() {
		summary += " — FAILED"
	} else {
		summary += " — PASSED"
	}
	return summary
}
