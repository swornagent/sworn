// Package gate provides lint gates for the SwornAgent CLI.
//
// mock.go implements `sworn lint mock`: Rule 10 no-mock boundary enforcement.
// It scans test files in a slice's diff for mock/stub/fixture/seed usage,
// detects real-infra references alongside undeclared mocks, and checks for
// boundary declarations (@mock-boundary comments, open_deferrals entries,
// architecture-overrides.json suppressions).
//
// Stdlib only — zero runtime dependencies.
package gate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"github.com/swornagent/sworn/internal/style"
)
// --- data model ---

// MockReport holds the full structured result of RunMock.
type MockReport struct {
	Slice           string          `json:"slice"`
	Release         string          `json:"release"`
	MockUsages      []MockUsage     `json:"mock_usages"`
	Violations      []MockViolation `json:"violations"`
	TotalViolations int             `json:"total_violations"`
	Verdict         string          `json:"verdict"` // "PASS" or "FAIL"
}

// MockViolation is a single undeclared mock boundary: a test file that uses
// mocks/stubs/fixtures AND real-infra references without a boundary declaration.
type MockViolation struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	MockKind  string `json:"mock_kind"`  // e.g. "mock.New", "httptest"
	MockValue string `json:"mock_value"` // the matched mock text
	InfraKind string `json:"infra_kind"` // e.g. "localhost", "AUTH0_DOMAIN"
	InfraLine int    `json:"infra_line"`
	InfraRef  string `json:"infra_ref"` // the matched infra text
	Msg       string `json:"msg"`
}

// MockUsage records a mock/stub/fixture/seed usage found in a test file.
// Informational — only becomes a violation when paired with real-infra refs
// and no boundary declaration.
type MockUsage struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// String returns a human-readable violation line.
func (v MockViolation) String() string {
	return fmt.Sprintf("%s:%d mock=%q line:%d infra=%q — %s",
		v.File, v.Line, v.MockValue, v.InfraLine, v.InfraRef, v.Msg)
}

// HasViolations returns true when the report contains boundary violations.
func (r *MockReport) HasViolations() bool { return r.TotalViolations > 0 }

// --- mock detection regex ---

var (
	// mockPatterns matches common mock/stub/fixture/seed patterns in test code.
	// Ordered by specificity: longer/more-specific patterns first so they
	// don't get shadowed by shorter/greedier patterns.
	mockPatterns = []struct {
		re   *regexp.Regexp
		kind string
	}{
		// Go patterns — specific before general.
		{regexp.MustCompile(`\bNewMock\w+`), "NewMock"},
		{regexp.MustCompile(`\.Mock\w+\(`), ".Mock"},
		{regexp.MustCompile(`\bMock\w+\s*\{`), "Mock"},
		{regexp.MustCompile(`\bMock\w+\(`), "Mock"},
		{regexp.MustCompile(`\bmock\.New`), "mock.New"},
		{regexp.MustCompile(`\bstub\.New`), "stub.New"},		{regexp.MustCompile(`\bStub\w+\s*\{`), "Stub"},
		{regexp.MustCompile(`\bFake\w+\(`), "Fake"},
		{regexp.MustCompile(`\bhttptest\.`), "httptest"},
		{regexp.MustCompile(`\bgomock\.`), "gomock"},
		{regexp.MustCompile(`testify/mock`), "testify/mock"},

		// TypeScript/JS patterns.
		{regexp.MustCompile(`\bvi\.(fn|mock|spyOn)`), "vitest-mock"},
		{regexp.MustCompile(`\bjest\.(fn|mock|spyOn)`), "jest-mock"},
		{regexp.MustCompile(`\bsinon\.`), "sinon"},
		{regexp.MustCompile(`\bnock\(`), "nock"},
		{regexp.MustCompile(`\bmsw\b`), "msw"},
		{regexp.MustCompile(`@ts-expect-error.*mock`), "ts-expect-mock"},

		// Python patterns.
		{regexp.MustCompile(`\bunittest\.mock\b`), "unittest.mock"},
		{regexp.MustCompile(`\bMagicMock\b`), "MagicMock"},
		{regexp.MustCompile(`\bpatch\(`), "patch"},
		{regexp.MustCompile(`\bmonkeypatch\b`), "pytest-monkeypatch"},
		{regexp.MustCompile(`\bresponses\b`), "responses-lib"},
		{regexp.MustCompile(`\bVCR\.`), "vcrpy"},

		// General patterns (checked last, after specific patterns).
		{regexp.MustCompile(`\bmock\.`), "mock."},
		{regexp.MustCompile(`\bstub\.`), "stub."},
		{regexp.MustCompile(`\bfake\.`), "fake."},
		{regexp.MustCompile(`\bfixture\.`), "fixture."},
		{regexp.MustCompile(`\bseed\.`), "seed."},
		{regexp.MustCompile(`\btestdata/`), "testdata/"},
		{regexp.MustCompile(`\bfixtures/`), "fixtures/"},
		{regexp.MustCompile(`json\.Unmarshal\(\s*\[\]byte`), "inline-json-stub"},
	}
	// realInfraPatterns matches patterns that suggest real infrastructure access.
	// Ordered by specificity: named env vars first, then connection URIs, generic
	// patterns later, localhost last.
	realInfraPatterns = []struct {
		re   *regexp.Regexp
		kind string
	}{
		// Specific env var names (checked before URIs to avoid shadowing).
		{regexp.MustCompile(`\bDATABASE_URL\b`), "DATABASE_URL"},
		{regexp.MustCompile(`\bDB_URL\b`), "DB_URL"},
		{regexp.MustCompile(`\bPOSTGRES_`), "POSTGRES"},
		{regexp.MustCompile(`\bMYSQL_`), "MYSQL"},
		{regexp.MustCompile(`\bREDIS_`), "REDIS"},
		{regexp.MustCompile(`\bMONGODB_`), "MONGODB"},
		{regexp.MustCompile(`\bDATABASE_`), "DATABASE"},

		// Auth & secrets — specific names before generic patterns.
		{regexp.MustCompile(`\bAUTH0_DOMAIN\b`), "AUTH0_DOMAIN"},
		{regexp.MustCompile(`\bAUTH0_CLIENT_ID\b`), "AUTH0_CLIENT_ID"},
		{regexp.MustCompile(`\bAUTH0_CLIENT_SECRET\b`), "AUTH0_CLIENT_SECRET"},
		{regexp.MustCompile(`\bSTRIPE_KEY\b`), "STRIPE_KEY"},
		{regexp.MustCompile(`\bSTRIPE_SECRET\b`), "STRIPE_SECRET"},
		{regexp.MustCompile(`\bSENDGRID_API_KEY\b`), "SENDGRID_API_KEY"},
		{regexp.MustCompile(`\bRESEND_API_KEY\b`), "RESEND_API_KEY"},
		{regexp.MustCompile(`\bAPI_KEY\b`), "API_KEY"},
		{regexp.MustCompile(`\bSECRET_KEY\b`), "SECRET_KEY"},

		// Connection strings (checked after named env vars, before localhost).
		{regexp.MustCompile(`postgres://`), "postgres-uri"},
		{regexp.MustCompile(`mysql://`), "mysql-uri"},
		{regexp.MustCompile(`mongodb://`), "mongodb-uri"},
		{regexp.MustCompile(`redis://`), "redis-uri"},
		{regexp.MustCompile(`sqlite://`), "sqlite-uri"},
		// .env file references.
		{regexp.MustCompile(`\bprocess\.env\.`), "process.env"},
		{regexp.MustCompile(`os\.Getenv\(`), "os.Getenv"},
		{regexp.MustCompile(`os\.environ\b`), "os.environ"},
		{regexp.MustCompile(`dotenv\b`), "dotenv"},

		// Real HTTP calls (not to localhost).
		{regexp.MustCompile(`http\.Get\(`), "http.Get"},
		{regexp.MustCompile(`http\.Post\(`), "http.Post"},
		{regexp.MustCompile(`http\.Do\(`), "http.Do"},
		{regexp.MustCompile(`fetch\(`), "fetch"},
		{regexp.MustCompile(`axios\.`), "axios"},
		{regexp.MustCompile(`\brequests\.(get|post|put|delete|patch)\b`), "requests"},
		{regexp.MustCompile(`\burllib\.request\b`), "urllib.request"},

		// Cloud SDK / provider references.
		{regexp.MustCompile(`\baws\.`), "aws-sdk"},
		{regexp.MustCompile(`\bgcp\.`), "gcp-sdk"},
		{regexp.MustCompile(`\bazure\.`), "azure-sdk"},

		// Real filesystem beyond testdata.
		{regexp.MustCompile(`os\.Open\(`), "os.Open"},
		{regexp.MustCompile(`os\.Create\(`), "os.Create"},
		{regexp.MustCompile(`ioutil\.ReadFile\(`), "ioutil.ReadFile"},
		{regexp.MustCompile(`os\.ReadFile\(`), "os.ReadFile"},

		// Localhost / loopback connections — checked LAST.
		{regexp.MustCompile(`localhost:\d+`), "localhost"},
		{regexp.MustCompile(`127\.0\.0\.1:\d+`), "127.0.0.1"},
		{regexp.MustCompile(`0\.0\.0\.0:\d+`), "0.0.0.0"},
	}
	// boundaryCommentRe matches boundary declaration comments.
	// Supports: // @mock-boundary ...  or /* @mock-boundary ... */
	boundaryCommentRe = regexp.MustCompile(`@mock-boundary\b`)
)

// --- main entry point ---

// RunMock runs the mock lint gate for a slice. It scans test files in the
// slice's diff for mock/stub/fixture usage, detects real-infra references
// alongside undeclared mocks, and checks boundary declarations.
//
// Parameters:
//
//	releaseDir — absolute path to docs/release/<release-name>/
//	sliceID    — e.g. "S68-lint-mock"
//	baseRef    — git ref for the diff base (start_commit or "release-wt/<release>")
//
// Returns an error only for I/O / git failures; violations are in the report.
func RunMock(releaseDir, sliceID, baseRef string) (*MockReport, error) {
	releaseName := filepath.Base(releaseDir)
	sliceDir := filepath.Join(releaseDir, sliceID)

	// 1. Get changed test files from the diff.
	changedFiles, err := diffChangedFiles(baseRef)
	if err != nil {
		return nil, fmt.Errorf("mock: git diff: %w", err)
	}
	var testFiles []string
	for _, f := range changedFiles {
		if isTestFile(f) || isSpecFile(f) {
			testFiles = append(testFiles, f)
		}
	}

	// 2. Load boundary overrides from architecture-overrides.json.
	overrides := loadMockOverrides(sliceDir)

	// 3. Load open_deferrals from status.json.
	deferrals := loadMockDeferrals(sliceDir)

	report := &MockReport{
		Slice:   sliceID,
		Release: releaseName,
	}

	// 4. Scan each test file.
	for _, file := range testFiles {
		usages, viols := scanFileForMocks(file, baseRef, overrides, deferrals)
		report.MockUsages = append(report.MockUsages, usages...)
		report.Violations = append(report.Violations, viols...)
	}

	// 5. Deduplicate mock usages (keep first occurrence per file+kind).
	report.MockUsages = dedupeMockUsages(report.MockUsages)

	report.TotalViolations = len(report.Violations)
	if report.TotalViolations == 0 {
		report.Verdict = "PASS"
	} else {
		report.Verdict = "FAIL"
	}

	return report, nil
}

// isSpecFile returns true for test specification files (e.g. .spec.ts, .test.ts).
func isSpecFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, ".spec.tsx") ||
		strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.js") || strings.HasSuffix(base, ".spec.jsx") ||
		strings.HasSuffix(base, ".test.js") || strings.HasSuffix(base, ".test.jsx")
}

// --- file scanning ---

// scanFileForMocks scans a single test file for mock usage and real-infra references.
// Returns informational mock usages and boundary violations.
func scanFileForMocks(file, baseRef string, overrides *MockOverrides, deferrals *MockDeferrals) ([]MockUsage, []MockViolation) {
	// Check isExempt first (per-file override).
	if isMockExempt(overrides, file) {
		return nil, nil
	}

	// Get the full file content.
	fullLines, err := readFileLines(file)
	if err != nil {
		return nil, nil
	}

	// Build the set of diff-added line numbers for scoping (informational).
	addedLines, err := diffAddedLines(baseRef, file)
	if err != nil {
		return nil, nil
	}
	addedSet := make(map[int]bool)
	for _, li := range addedLines {
		addedSet[li.Line] = true
	}

	// Phase 1: Find all mock usages.
	mockUsages, mockLineNos := findMocksInFile(file, fullLines, addedSet)

	// Phase 2: Find all real-infra references.
	infraRefs, _ := findInfraInFile(file, fullLines, addedSet)

	// If no mocks found, nothing to report (even if infra refs exist).
	if len(mockUsages) == 0 {
		return nil, nil
	}

	// Check for boundary declarations in the file.
	hasBoundaryComment := hasMockBoundaryInFile(fullLines)
	hasDeferral := deferrals.HasMockBoundary()

	// If boundary declared, no violations — just report usages for info.
	if hasBoundaryComment || hasDeferral {
		return mockUsages, nil
	}

	// If mocks AND infra refs found without boundary, each intersection is a violation.
	if len(infraRefs) == 0 {
		// Mocks without infra — informational only, no violation.
		return mockUsages, nil
	}

	// Build violations: each infra ref paired with the nearest mock.
	var violations []MockViolation
	seen := make(map[string]bool) // dedupe key: mockValue+infraRef
	for _, inf := range infraRefs {
		bestMock := closestMock(mockLineNos, inf.Line)
		key := fmt.Sprintf("%s|%s", bestMock.Value, inf.Value)
		if seen[key] {
			continue
		}
		seen[key] = true
		violations = append(violations, MockViolation{
			File:      file,
			Line:      bestMock.Line,
			MockKind:  bestMock.Kind,
			MockValue: bestMock.Value,
			InfraKind: inf.Kind,
			InfraLine: inf.Line,
			InfraRef:  inf.Value,
			Msg:       fmt.Sprintf("undeclared mock boundary: %s in test file alongside %s reference", bestMock.Kind, inf.Kind),
		})
	}

	return mockUsages, violations
}
// findMocksInFile scans a file for mock/stub/fixture/seed patterns.
func findMocksInFile(file string, lines []string, addedSet map[int]bool) ([]MockUsage, map[int]MockUsage) {
	var usages []MockUsage
	lineNos := make(map[int]MockUsage)
	for i, line := range lines {
		lineNo := i + 1
		for _, pat := range mockPatterns {
			if m := pat.re.FindString(line); m != "" {
				u := MockUsage{File: file, Line: lineNo, Kind: pat.kind, Value: m}
				usages = append(usages, u)
				if _, exists := lineNos[lineNo]; !exists {
					lineNos[lineNo] = u
				}
				break // one match per line is sufficient
			}
		}
	}
	return usages, lineNos
}

// findInfraInFile scans a file for real-infra reference patterns.
func findInfraInFile(file string, lines []string, addedSet map[int]bool) ([]MockUsage, map[int]MockUsage) {
	var refs []MockUsage
	lineNos := make(map[int]MockUsage)
	for i, line := range lines {
		lineNo := i + 1
		for _, pat := range realInfraPatterns {
			if m := pat.re.FindString(line); m != "" {
				u := MockUsage{File: file, Line: lineNo, Kind: pat.kind, Value: m}
				refs = append(refs, u)
				if _, exists := lineNos[lineNo]; !exists {
					lineNos[lineNo] = u
				}
				break
			}
		}
	}
	return refs, lineNos
}

// closestMock returns the mock usage closest to a given line number.
func closestMock(mockLineNos map[int]MockUsage, targetLine int) MockUsage {
	bestDist := int(^uint(0) >> 1) // max int
	var best MockUsage
	for line, u := range mockLineNos {
		dist := line - targetLine
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			best = u
		}
	}
	return best
}

// hasMockBoundaryInFile checks if the file contains a @mock-boundary comment.
func hasMockBoundaryInFile(lines []string) bool {
	for _, line := range lines {
		if boundaryCommentRe.MatchString(line) {
			return true
		}
	}
	return false
}

// --- override / deferral loading ---
// MockOverrides holds per-slice mock boundary overrides from architecture-overrides.json.
type MockOverrides struct {
	Schema        string             `json:"$schema"`
	Description   string             `json:"_description,omitempty"`
	MockOverrides []MockOverrideRule `json:"mock_overrides,omitempty"`
}

// MockOverrideRule is a single mock override rule.
type MockOverrideRule struct {
	RuleID string `json:"rule_id"`
	File   string `json:"file,omitempty"`
	Reason string `json:"reason"`
}

// MockDeferrals holds parsed open_deferrals from status.json.
type MockDeferrals struct {
	Entries []MockDeferralEntry `json:"entries"`
}

// MockDeferralEntry is a single deferral entry.
type MockDeferralEntry struct {
	What    string `json:"what"`
	Why     string `json:"why"`
	Issue   string `json:"issue"`
	Ack     string `json:"ack"`
}

// HasMockBoundary returns true when any deferral entry relates to mocks.
func (d *MockDeferrals) HasMockBoundary() bool {
	for _, e := range d.Entries {
		lower := strings.ToLower(e.What + e.Why)
		if strings.Contains(lower, "mock") ||
			strings.Contains(lower, "boundary") ||
			strings.Contains(lower, "stub") ||
			strings.Contains(lower, "fixture") ||
			strings.Contains(lower, "seed") {
			return true
		}
	}
	return false
}

// loadMockOverrides reads architecture-overrides.json from the slice directory.
func loadMockOverrides(sliceDir string) *MockOverrides {
	path := filepath.Join(sliceDir, "architecture-overrides.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var o MockOverrides
	if err := json.Unmarshal(data, &o); err != nil {
		return nil
	}
	return &o
}

// loadMockDeferrals reads open_deferrals from the slice's status.json.
func loadMockDeferrals(sliceDir string) *MockDeferrals {
	statusPath := filepath.Join(sliceDir, "status.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil
	}
	// Extract the open_deferrals array.
	idx := strings.Index(string(data), `"open_deferrals"`)
	if idx < 0 {
		return &MockDeferrals{}
	}
	block := string(data)[idx:]
	end := strings.Index(block, "]")
	if end < 0 {
		return &MockDeferrals{}
	}
	block = block[:end+1]

	var d MockDeferrals
	if err := json.Unmarshal([]byte(`{"entries":`+block[strings.Index(block, "[")+1:]), &d.Entries); err != nil {
		return &MockDeferrals{}
	}
	return &d
}

// isMockExempt checks if a file is exempted by the mock overrides.
func isMockExempt(overrides *MockOverrides, file string) bool {
	if overrides == nil {
		return false
	}
	for _, r := range overrides.MockOverrides {
		if r.RuleID == "mock-boundary" || r.RuleID == "no-mock-boundary" {
			if r.File == "" || r.File == file {
				return true
			}
		}
	}
	return false
}

// --- deduplication ---

// dedupeMockUsages removes duplicate mock usages keeping the first per file+kind.
func dedupeMockUsages(usages []MockUsage) []MockUsage {
	seen := make(map[string]bool)
	var result []MockUsage
	for _, u := range usages {
		key := u.File + "|" + u.Kind
		if !seen[key] {
			seen[key] = true
			result = append(result, u)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].File != result[j].File {
			return result[i].File < result[j].File
		}
		return result[i].Line < result[j].Line
	})
	return result
}

// --- human-readable output ---

// PrintMock renders the MockReport as human-readable text.
func PrintMock(r *MockReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(style.Bold(fmt.Sprintf("MOCK LINT — %s / %s", r.Release, r.Slice)))
	b.WriteString("\n\n")

	// Mock usage summary.
	b.WriteString(style.Dim(fmt.Sprintf("Mock/stub/fixture usages found: %d\n", len(r.MockUsages))))
	for i, u := range r.MockUsages {
		b.WriteString(style.Dim(fmt.Sprintf("  %d. [%s] %s:%d — %s\n", i+1, u.Kind, u.File, u.Line, u.Value)))
	}
	b.WriteString("\n")

	// Violations.
	if len(r.Violations) == 0 {
		b.WriteString(style.Success("No undeclared mock boundaries.\n"))
	} else {
		b.WriteString(style.Danger(fmt.Sprintf("Undeclared mock boundaries: %d\n", len(r.Violations))))
		for i, v := range r.Violations {
			b.WriteString(style.Danger(fmt.Sprintf("  %d. %s:%d mock=%q\n", i+1, v.File, v.Line, v.MockValue)))
			b.WriteString(style.Dim(fmt.Sprintf("     infra ref: %s (line %d) — %s\n", v.InfraRef, v.InfraLine, v.InfraKind)))
		}
	}

	// Verdict.
	b.WriteString("\n")
	if r.Verdict == "PASS" {
		b.WriteString(style.Success("PASS — mock lint clean\n"))
	} else {
		b.WriteString(style.Danger(fmt.Sprintf("FAIL — %d violation(s)\n", r.TotalViolations)))
	}
	b.WriteString("\n")

	return b.String()
}

// JSONMock returns the report as pretty-printed JSON.
func JSONMock(r *MockReport) string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}
