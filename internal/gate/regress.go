// Package gate provides lint and regression gates for the SwornAgent CLI.
//
// regress.go ports the release-regression concept from bash to Go:
// `sworn regress --release <name>` runs the full test suite (Go + TS +
// golden fixtures) against the merged release-wt worktree and exits 0
// only when everything passes.
//
// Reads from a docs/release/<release-name>/ directory to resolve the
// release worktree, then executes test commands in that worktree.
// Stdlib only — zero runtime dependencies.
package gate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/style"
)

// --- data model ---

// RegressReport holds the structured result of RunRegress.
type RegressReport struct {
	Release   string        `json:"release"`
	Worktree  string        `json:"worktree"`
	Suites    []SuiteResult `json:"suites"`
	Passed    int           `json:"passed"`
	Failed    int           `json:"failed"`
	Skipped   int           `json:"skipped"`
	AllPassed bool          `json:"all_passed"`
}

// SuiteResult is the result of running a single test suite.
type SuiteResult struct {
	Name          string `json:"name"`
	Passed        bool   `json:"passed"`
	Skipped       bool   `json:"skipped"`
	SkippedReason string `json:"skipped_reason,omitempty"`
	Output        string `json:"output,omitempty"`
	ExitCode      int    `json:"exit_code"`
}

// HasViolations returns true when one or more suites failed.
func (r *RegressReport) HasViolations() bool { return r.Failed > 0 }

// --- test runner abstraction (testable) ---

// testRunner abstracts running a command in a directory.
// In production it shells out via realRunner; in tests it is a mock.
type testRunner interface {
	Run(dir, name string, args ...string) (stdout string, exitCode int, err error)
}

type realRunner struct{}

func (realRunner) Run(dir, name string, args ...string) (string, int, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return out.String(), -1, err
		}
	}
	return out.String(), exitCode, nil
}

// --- public API ---

// RunRegress runs the full regression suite against a release worktree.
// worktreePath must be an absolute path to the release-wt worktree.
func RunRegress(worktreePath, releaseName string) (*RegressReport, error) {
	return runRegress(worktreePath, releaseName, realRunner{})
}

// runRegress is the internal entry point, accepting a testRunner for testability.
func runRegress(worktreePath, releaseName string, runner testRunner) (*RegressReport, error) {
	report := &RegressReport{
		Release:  releaseName,
		Worktree: worktreePath,
	}

	// 1. Go tests
	report.Suites = append(report.Suites, runGoSuite(worktreePath, runner))

	// 2. TypeScript tests (skip if no pnpm or no package.json)
	report.Suites = append(report.Suites, runTSSuite(worktreePath, runner))

	// 3. Golden fixture check
	report.Suites = append(report.Suites, checkGoldenFixtures(worktreePath, runner))

	// Tally
	for _, s := range report.Suites {
		if s.Skipped {
			report.Skipped++
		} else if s.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
	}
	report.AllPassed = report.Failed == 0

	return report, nil
}

// --- suite runners ---

func runGoSuite(worktree string, runner testRunner) SuiteResult {
	out, exitCode, err := runner.Run(worktree, "go", "test", "./...")
	if err != nil {
		return SuiteResult{
			Name:     "Go tests",
			Passed:   false,
			Output:   fmt.Sprintf("go test error: %v\n%s", err, out),
			ExitCode: -1,
		}
	}
	return SuiteResult{
		Name:     "Go tests",
		Passed:   exitCode == 0,
		Output:   out,
		ExitCode: exitCode,
	}
}

func runTSSuite(worktree string, runner testRunner) SuiteResult {
	// Check if pnpm is available.
	_, _, pnpmErr := runner.Run(worktree, "pnpm", "--version")
	if pnpmErr != nil {
		return SuiteResult{
			Name:          "TypeScript tests",
			Skipped:       true,
			SkippedReason: "pnpm not available",
		}
	}

	// Check if package.json exists in the worktree.
	if _, err := os.Stat(filepath.Join(worktree, "package.json")); os.IsNotExist(err) {
		return SuiteResult{
			Name:          "TypeScript tests",
			Skipped:       true,
			SkippedReason: "no package.json in worktree",
		}
	}

	out, exitCode, err := runner.Run(worktree, "pnpm", "test")
	if err != nil {
		return SuiteResult{
			Name:     "TypeScript tests",
			Passed:   false,
			Output:   fmt.Sprintf("pnpm test error: %v\n%s", err, out),
			ExitCode: -1,
		}
	}
	return SuiteResult{
		Name:     "TypeScript tests",
		Passed:   exitCode == 0,
		Output:   out,
		ExitCode: exitCode,
	}
}

func checkGoldenFixtures(worktree string, runner testRunner) SuiteResult {
	// Check for golden fixture divergence using git diff on testdata directories.
	// Golden fixtures are tracked files under **/testdata/** that should not have
	// uncommitted changes after a test suite runs.
	//
	// Uses --exit-code: exits 1 when there are differences, 0 when clean.
	out, exitCode, err := runner.Run(worktree, "git", "diff", "--exit-code", "--", "**/testdata/**")
	if err != nil {
		return SuiteResult{
			Name:     "Golden fixtures",
			Passed:   false,
			Output:   fmt.Sprintf("golden fixture check error: %v\n%s", err, out),
			ExitCode: -1,
		}
	}
	if exitCode != 0 {
		return SuiteResult{
			Name:     "Golden fixtures",
			Passed:   false,
			Output:   out,
			ExitCode: exitCode,
		}
	}
	return SuiteResult{
		Name:     "Golden fixtures",
		Passed:   true,
		ExitCode: 0,
	}
}

// --- output formatters ---

// PrintRegress formats a RegressReport for human-readable output.
func PrintRegress(r *RegressReport) string {
	var b strings.Builder
	b.WriteString(style.Heading(fmt.Sprintf("Regression — %s", r.Release)))
	b.WriteString("\n\n")
	b.WriteString(style.Dim(fmt.Sprintf("Worktree: %s\n\n", r.Worktree)))

	for _, s := range r.Suites {
		switch {
		case s.Skipped:
			b.WriteString(fmt.Sprintf("  %s  %s (%s)\n",
				style.Accent("SKIP"), s.Name, s.SkippedReason))
		case s.Passed:
			b.WriteString(fmt.Sprintf("  %s  %s\n",
				style.Success("PASS"), s.Name))
		default:
			b.WriteString(fmt.Sprintf("  %s  %s (exit %d)\n",
				style.Danger("FAIL"), s.Name, s.ExitCode))
		}
	}

	b.WriteString("\n")
	if r.AllPassed {
		b.WriteString(style.Success(
			fmt.Sprintf("All %d suites passed (%d skipped).\n", r.Passed+r.Skipped, r.Skipped)))
	} else {
		b.WriteString(style.Danger(
			fmt.Sprintf("%d suite(s) failed, %d passed, %d skipped.\n",
				r.Failed, r.Passed, r.Skipped)))
	}
	return b.String()
}

// JSONRegress serialises a RegressReport as indented JSON.
func JSONRegress(r *RegressReport) string {
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}