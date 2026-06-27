// Package gate provides lint gates for the SwornAgent CLI.
//
// coverage.go ports bin/release-coverage.sh from bash to Go:
// mechanically maps every acceptance check in a slice's spec.md to a matching
// test function (file:line) in the slice's diff, flagging uncovered ACs.
//
// Recognises Go (func TestXxx), TypeScript (it/test/describe), and Python
// (def test_xxx) test function patterns.  Keyword-matches AC text against
// test function names and reports the best match per AC.
//
// Reads from a docs/release/<release-name>/ directory.
// Stdlib only — zero runtime dependencies.
package gate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/style"
)

// --- data model ---

// CoverageReport holds the full structured result of RunCoverage.
type CoverageReport struct {
	Slice     string          `json:"slice"`
	Release   string          `json:"release"`
	TotalACs  int             `json:"total_acs"`
	Covered   int             `json:"covered"`
	Uncovered int             `json:"uncovered"`
	Entries   []CoverageEntry `json:"entries"`
	Verdict   string          `json:"verdict"` // "PASS" or "FAIL"
}

// CoverageEntry is one AC mapped to its best-match test function (or empty).
type CoverageEntry struct {
	ACID        string   `json:"ac_id"`
	ACText      string   `json:"ac_text"`
	MatchedTest string   `json:"matched_test"`
	MatchFile   string   `json:"match_file"`
	MatchLine   int      `json:"match_line"`
	Candidates  []string `json:"candidates"`
}

// HasViolations returns true when one or more ACs are uncovered.
func (r *CoverageReport) HasViolations() bool { return r.Uncovered > 0 }

// --- regex patterns ---

var (
	// Go: func TestXxx(t *testing.T) { — or any receiver.
	reGoTest = regexp.MustCompile(`^\s*func\s+(?:\(\w+\s+\*?\w+\)\s+)?(Test\w+)\s*\(`)
	// Go: func BenchmarkXxx(b *testing.B)
	reGoBench = regexp.MustCompile(`^\s*func\s+(?:\(\w+\s+\*?\w+\)\s+)?(Benchmark\w+)\s*\(`)
	// TypeScript/Vitest/Jest: it('...', / test('...',
	reTSTest = regexp.MustCompile(`\b(it|test)\s*\(\s*['"\x60]([^'"\x60]+)['"\x60]`)	// Python: def test_xxx(  — plus pytest-style def test_xxx():
	rePyTest = regexp.MustCompile(`^\s*def\s+(test_\w+)\s*\(`)
)

// isTestFile returns true when the path looks like a test file.
// Recognises: *_test.go, *.test.ts(x), *.spec.ts(x), test_*.py.
func isTestFile(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	switch ext {
	case ".go":
		return strings.HasSuffix(base, "_test.go")
	case ".ts", ".tsx":
		return strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.tsx") ||
			strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, ".spec.tsx")
	case ".py":
		return strings.HasPrefix(base, "test_")
	}
	return false
}

// --- test function extraction ---

// testFunc holds a single test function found in a file.
type testFunc struct {
	Name string // e.g. "TestFoo", "renders the button", "test_calculate"
	File string // relative file path
	Line int    // 1-based line number
}

// extractTestFuncs scans a test file and returns every test function found.
func extractTestFuncs(filePath string) ([]testFunc, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []testFunc
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		ext := filepath.Ext(filePath)

		switch ext {
		case ".go":
			if m := reGoTest.FindStringSubmatch(line); m != nil {
				out = append(out, testFunc{Name: m[1], File: filePath, Line: lineNo})
			} else if m := reGoBench.FindStringSubmatch(line); m != nil {
				out = append(out, testFunc{Name: m[1], File: filePath, Line: lineNo})
			}
		case ".ts", ".tsx":
			for _, m := range reTSTest.FindAllStringSubmatch(line, -1) {
				out = append(out, testFunc{Name: m[2], File: filePath, Line: lineNo})
			}
		case ".py":
			if m := rePyTest.FindStringSubmatch(line); m != nil {
				out = append(out, testFunc{Name: m[1], File: filePath, Line: lineNo})
			}
		}
	}
	return out, scanner.Err()
}

// --- keyword matching ---

// tokenise splits text into lowercase alphanumeric tokens, discarding
// stop-words and short tokens.  CamelCase subwords are extracted before
// lowercasing (e.g. "TestValidateInputFields" yields "test", "validate",
// "input", "fields").
func tokenise(text string) map[string]bool {
	re := regexp.MustCompile(`[a-zA-Z0-9_]+`)
	tokens := make(map[string]bool)
	for _, tok := range re.FindAllString(text, -1) {
		// Split camelCase before lowercasing.
		subs := splitCamel(tok)
		for _, sw := range subs {
			sw = strings.ToLower(sw)
			if len(sw) < 3 || isStopWord(sw) {
				continue
			}
			tokens[sw] = true
		}
	}
	return tokens
}
// splitCamel splits a CamelCase identifier into lowercase subwords.
// e.g. "TestValidateInputFields" → ["test", "validate", "input", "fields"].
func splitCamel(s string) []string {
	if len(s) == 0 {
		return nil
	}
	var parts []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			parts = append(parts, strings.ToLower(s[start:i]))
			start = i
		}
	}
	parts = append(parts, strings.ToLower(s[start:]))
	return parts
}
// isStopWord returns true for common English words that carry no signal.
func isStopWord(w string) bool {
	switch w {
	case "the", "and", "for", "that", "this", "with", "from",
		"shall", "when", "while", "where", "then", "system",
		"have", "each", "not", "are", "its", "has", "but",
		"does", "into", "such", "more", "over", "under",
		"must", "will", "also", "than", "been", "can",
		"was", "were", "all", "only", "which", "what",
		"being", "able", "after", "their", "them", "these",
		"those", "there", "they", "other", "some", "any":
		return true
	}
	return false
}

// matchScore computes a simple token-overlap score between acText and a
// test function name.  Returns the count of tokens shared.
func matchScore(acText, testName string) int {
	acTokens := tokenise(acText)
	nameTokens := tokenise(testName)
	score := 0
	for t := range acTokens {
		// Also try camelCase/subword splitting for Go test names.
		if nameTokens[t] {
			score++
		}
	}
	// Bonus for multi-word overlap in TS/Python names.
	for nt := range nameTokens {
		if len(nt) >= 4 && strings.Contains(strings.ToLower(acText), nt) {
			score++
		}
	}
	return score
}

// --- git diff ---

// diffTestFiles returns test files changed between baseRef and HEAD.
func diffTestFiles(baseRef string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", baseRef, "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isTestFile(line) {
			files = append(files, line)
		}
	}
	return files, nil
}

// --- main entry point ---

// RunCoverage reads a slice's spec.md, extracts acceptance checks, scans
// the slice's diff for test functions, and produces a coverage map.
//
// Parameters:
//
//	releaseDir — absolute path to docs/release/<release-name>/
//	sliceID    — e.g. "S66-lint-coverage"
//	baseRef    — git ref for the diff base (e.g. start_commit or "release-wt/<release>")
//
// Returns an error only for I/O / git failures; coverage gaps are in the report.
func RunCoverage(releaseDir, sliceID, baseRef string) (*CoverageReport, error) {
	sliceDir := filepath.Join(releaseDir, sliceID)
	specPath := filepath.Join(sliceDir, "spec.md")

	specText, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("coverage: read spec.md: %w", err)
	}

	// 1. Extract ACs.
	acs := parseAcceptanceChecks(string(specText))
	if len(acs) == 0 {
		return nil, fmt.Errorf("coverage: no acceptance checks found in %s", specPath)
	}

	// 2. Get test files from the diff.
	testFiles, err := diffTestFiles(baseRef)
	if err != nil {
		return nil, fmt.Errorf("coverage: %w", err)
	}

	// 3. Scan for test functions.
	var allTests []testFunc
	for _, tf := range testFiles {
		tfs, err := extractTestFuncs(tf)
		if err != nil {
			return nil, fmt.Errorf("coverage: scan %s: %w", tf, err)
		}
		allTests = append(allTests, tfs...)
	}

	// 4. Build coverage map.
	r := &CoverageReport{
		Slice:   sliceID,
		Release: filepath.Base(releaseDir),
	}
	for i, ac := range acs {
		acID := fmt.Sprintf("AC-%02d", i+1)
		best, candidates := bestMatch(ac, allTests)
		entry := CoverageEntry{
			ACID:       acID,
			ACText:     ac,
			Candidates: candidates,
		}
		if best != nil {
			entry.MatchedTest = best.Name
			entry.MatchFile = best.File
			entry.MatchLine = best.Line
			r.Covered++
		} else {
			r.Uncovered++
		}
		r.Entries = append(r.Entries, entry)
	}
	r.TotalACs = len(acs)
	if r.Uncovered == 0 {
		r.Verdict = "PASS"
	} else {
		r.Verdict = "FAIL"
	}

	return r, nil
}

// bestMatch finds the test function with the highest token-overlap score
// against the AC text, and returns all candidates in descending score order.
func bestMatch(acText string, tests []testFunc) (*testFunc, []string) {
	type scored struct {
		tf    testFunc
		score int
	}
	var ranked []scored
	for _, tf := range tests {
		s := matchScore(acText, tf.Name)
		if s > 0 {
			ranked = append(ranked, scored{tf, s})
		}
	}
	if len(ranked) == 0 {
		return nil, nil
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	var cands []string
	for _, r := range ranked {
		cands = append(cands, fmt.Sprintf("%s (%s:%d score=%d)", r.tf.Name, r.tf.File, r.tf.Line, r.score))
	}
	return &ranked[0].tf, cands
}

// --- human-readable output ---

// PrintCoverage renders the CoverageReport as human-readable text.
func PrintCoverage(r *CoverageReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(style.Bold(fmt.Sprintf("COVERAGE — %s / %s", r.Release, r.Slice)))
	b.WriteString("\n\n")
	b.WriteString(style.Dim(fmt.Sprintf("ACs: %d checked  covered: %d  uncovered: %d\n", r.TotalACs, r.Covered, r.Uncovered)))
	b.WriteString("\n")

	for _, e := range r.Entries {
		if e.MatchedTest != "" {
			b.WriteString(style.Success(fmt.Sprintf("  %s ✓ %s → %s in %s:%d\n",
				e.ACID, truncate(e.ACText, 72), e.MatchedTest, e.MatchFile, e.MatchLine)))
		} else {
			b.WriteString(style.Danger(fmt.Sprintf("  %s ✗ %s — uncovered\n", e.ACID, truncate(e.ACText, 72))))
			if len(e.Candidates) > 0 {
				b.WriteString(style.Dim(fmt.Sprintf("      candidates: %s\n", strings.Join(e.Candidates, ", "))))
			}
		}
	}

	b.WriteString("\n")
	if r.Verdict == "PASS" {
		b.WriteString(style.Success("PASS — all acceptance checks have a matching test\n"))
	} else {
		b.WriteString(style.Danger(fmt.Sprintf("FAIL — %d acceptance check(s) uncovered\n", r.Uncovered)))
	}
	b.WriteString("\n")

	return b.String()
}

// JSONCoverage returns the report as pretty-printed JSON.
func JSONCoverage(r *CoverageReport) string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// --- CLI helper ---

// BaseRefForSlice resolves the git base ref for a slice: reads its
// status.json start_commit field, falls back to "release-wt/<release>".
func BaseRefForSlice(sliceDir, releaseName string) (string, error) {
	statusPath := filepath.Join(sliceDir, "status.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return "", fmt.Errorf("read status.json: %w", err)
	}
	// Extract start_commit via regex.
	re := regexp.MustCompile(`"start_commit"\s*:\s*"([^"]*)"`)
	m := re.FindStringSubmatch(string(data))
	if m != nil && m[1] != "" {
		return m[1], nil
	}
	return "release-wt/" + releaseName, nil
}