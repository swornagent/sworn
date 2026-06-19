// Package verify runs the SwornAgent verification protocol: a deterministic
// $0 first-pass, then an adversarial fresh-context model verification. It is
// provider-neutral and host-neutral — it operates only on the spec -> diff
// (-> proof) triple and a Verifier, never on a git host or a specific model.
package verify

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/verdict"
)

// systemPrompt is the sworn-authored stateless judge prompt, vendored at
// build time via go:embed (internal/prompt). It instructs the model to judge
// from SPEC+DIFF+PROOF only with a verdict-leading reply — no tools, no repo.
var systemPrompt = prompt.VerifyStateless()

// Input is everything a verification needs.
type Input struct {
	SpecPath      string
	DiffPath      string // "-" reads stdin
	ProofPath     string // optional in S1
	Model         string
	Verifier      model.Verifier // nil -> Unconfigured (fails closed)
	OpenDeferrals []string       // Rule-2 deferrals from status.json (S10 no-mock-boundary)
}
// Run executes the protocol and returns a fail-closed Result.
func Run(ctx context.Context, in Input) verdict.Result {
	// --- Deterministic first-pass ($0 gate) ---
	spec, err := readNonEmpty(in.SpecPath)
	if err != nil {
		return blocked("first_pass:spec", err.Error())
	}
	diff, err := readNonEmpty(in.DiffPath)
	if err != nil {
		return blocked("first_pass:diff", err.Error())
	}
	proof := ""
	if in.ProofPath != "" {
		proof, _ = readFile(in.ProofPath)
	}

	// --- Boundary-mock check (S10 first-pass gate) ---
	report := CheckBoundaryMocks(diff, in.OpenDeferrals)
	if len(report.UndeclaredMocks) > 0 {
		var b strings.Builder
		b.WriteString("Undeclared boundary mock(s) — fail closed per Rule 7/Rule 2:\n")
		for _, m := range report.UndeclaredMocks {
			b.WriteString(fmt.Sprintf("  - %s (boundary: %s) at %s:%d\n", m.MockType, m.Boundary, m.File, m.Line))
		}
		return verdict.Result{
			Verdict:    verdict.Fail,
			FailedGate: "boundary_mock",
			Rationale:  b.String(),
		}
	}
	if len(report.DeclaredMocks) > 0 {
		var b strings.Builder
		b.WriteString("Declared boundary mock(s) — allowed with known deferral:\n")
		for _, m := range report.DeclaredMocks {
			b.WriteString(fmt.Sprintf("  - %s (boundary: %s) at %s:%d\n", m.MockType, m.Boundary, m.File, m.Line))
		}
		// Append to diff so the model sees the deferral context.
		diff = diff + "\n\n" + b.String()
	}

	// --- Adversarial model verification ---
	v := in.Verifier
	if v == nil {
		v = model.Unconfigured{}
	}
	text, cost, err := v.Verify(ctx, systemPrompt, buildPayload(spec, diff, proof))
	if err != nil {
		return blocked("verifier_dispatch", err.Error())
	}
	result := parseVerdict(text, cost)

	// Surface declared boundary mocks in the result rationale so the caller
	// sees them as known deferrals (AC2 — no-mock-boundary).
	if len(report.DeclaredMocks) > 0 {
		var b strings.Builder
		b.WriteString("Declared boundary mock(s) — allowed with known deferral:\n")
		for _, m := range report.DeclaredMocks {
			b.WriteString(fmt.Sprintf("  - %s (boundary: %s) at %s:%d\n", m.MockType, m.Boundary, m.File, m.Line))
		}
		b.WriteString("\n")
		result.Rationale = b.String() + result.Rationale
	}

	return result}

func buildPayload(spec, diff, proof string) string {
	var b strings.Builder
	b.WriteString("## SPEC\n")
	b.WriteString(spec)
	b.WriteString("\n\n## DIFF\n")
	b.WriteString(diff)
	if proof != "" {
		b.WriteString("\n\n## PROOF\n")
		b.WriteString(proof)
	}
	return b.String()
}

// parseVerdict extracts the verdict from the model's reply. It tolerates common
// model output variations — leading blank lines, markdown emphasis, a leading
// code fence — while remaining fail-closed: only a leading PASS/FAIL/BLOCKED/
// INCONCLUSIVE token on the first substantive line passes; anything else blocks.
func parseVerdict(text string, cost float64) verdict.Result {
	line := firstVerdictLine(text)
	t := stripMarkdown(line)
	upper := strings.ToUpper(t)
	switch {
	case strings.HasPrefix(upper, "PASS"):
		return verdict.Result{Verdict: verdict.Pass, Rationale: text, CostUSD: cost}
	case strings.HasPrefix(upper, "FAIL"):
		return verdict.Result{Verdict: verdict.Fail, FailedGate: "adversarial", Rationale: text, CostUSD: cost}
	case strings.HasPrefix(upper, "BLOCKED"):
		return verdict.Result{Verdict: verdict.Blocked, FailedGate: "adversarial", Rationale: text, CostUSD: cost}
	case strings.HasPrefix(upper, "INCONCLUSIVE"):
		return verdict.Result{Verdict: verdict.Inconclusive, FailedGate: "adversarial", Rationale: text, CostUSD: cost}
	default:
		return verdict.Result{Verdict: verdict.Blocked, FailedGate: "unparseable_verdict",
			Rationale: "verifier reply did not start with PASS/FAIL/BLOCKED/INCONCLUSIVE", CostUSD: cost}
	}
}

// firstVerdictLine returns the first non-empty line that is not a bare code
// fence.  Leading blank lines are skipped; a line containing only ``` is treated
// as a fence and skipped so that ```\nPASS resolves to PASS.
func firstVerdictLine(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// A bare code fence line — skip it, the verdict follows.
		if t == "```" {
			continue
		}
		return t
	}
	return ""
}

// stripMarkdown removes surrounding markdown emphasis characters (*, _, `) and
// a leading code-fence marker (```) from a single line.  It trims space before
// and after so the result is ready for prefix matching.
func stripMarkdown(line string) string {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "```") {
		t = strings.TrimPrefix(t, "```")
	}
	t = strings.TrimSpace(t)
	// Strip surrounding emphasis — any run of *, _, ` on both sides.
	t = strings.TrimLeft(t, "*_`")
	t = strings.TrimRight(t, "*_`")
	return strings.TrimSpace(t)
}
func blocked(gate, why string) verdict.Result {	return verdict.Result{Verdict: verdict.Blocked, FailedGate: gate, Rationale: why}
}

func readNonEmpty(path string) (string, error) {
	s, err := readFile(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("%s is empty", display(path))
	}
	return s, nil
}

func readFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no path provided")
	}
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func display(path string) string {
	if path == "-" {
		return "stdin"
	}
	return path
}

// --- S10: Boundary-mock detection ---

// BoundaryMock records one detected mock at a validated boundary.
type BoundaryMock struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Boundary string `json:"boundary"` // "db", "auth", "entitlement"
	MockType string `json:"mock_type"`
	Declared bool   `json:"declared"`
	Deferral string `json:"deferral,omitempty"`
}

// BoundaryMockReport groups detected mocks by declaration status.
type BoundaryMockReport struct {
	UndeclaredMocks []BoundaryMock `json:"undeclared_mocks"`
	DeclaredMocks   []BoundaryMock `json:"declared_mocks"`
}

// boundaryPattern associates a keyword string with a validated boundary.
type boundaryPattern struct {
	Keyword  string // keyword to match in a line
	Boundary string // canonical boundary name
}

// knownBoundaryPatterns list patterns that, when combined with a mock/stub/fake
// construct, indicate a mock at a validated boundary.
var knownBoundaryPatterns = []boundaryPattern{
	{Keyword: "sql.", Boundary: "db"},
	{Keyword: "database/sql", Boundary: "db"},
	{Keyword: "*sql.DB", Boundary: "db"},
	{Keyword: "*sql.Tx", Boundary: "db"},
	{Keyword: "*sql.Conn", Boundary: "db"},
	{Keyword: "sql.DB", Boundary: "db"},
	{Keyword: "sql.Tx", Boundary: "db"},
	{Keyword: "sql.Conn", Boundary: "db"},
	{Keyword: "DB", Boundary: "db"},
	{Keyword: "auth", Boundary: "auth"},
	{Keyword: "Auth", Boundary: "auth"},
	{Keyword: "Authenticate", Boundary: "auth"},
	{Keyword: "Authorize", Boundary: "auth"},
	{Keyword: "entitle", Boundary: "entitlement"},
	{Keyword: "Entitle", Boundary: "entitlement"},
	{Keyword: "premium", Boundary: "entitlement"},
	{Keyword: "Premium", Boundary: "entitlement"},
	{Keyword: "subscription", Boundary: "entitlement"},
	{Keyword: "Subscription", Boundary: "entitlement"},
}

// mockMarkerPatterns are tokens on a line that suggest a mock/stub/fake/test
// double is being created or assigned.  At least one boundary pattern must also
// match for the line to be flagged.
var mockMarkerPatterns = []string{
	"mock", "Mock", "MOCK",
	"fake", "Fake", "FAKE",
	"stub", "Stub", "STUB",
	"testdouble", "TestDouble",
	"newMock", "NewMock",
	"newTest", "NewTest",
}

// CheckBoundaryMocks scans diff content for mocks/stubs at validated boundaries
// and cross-references against open deferrals.  Returns a report of undeclared
// (violations) and declared (known deferrals) boundary mocks.
//
// Detection is heuristic: a line must contain at least one boundary pattern AND
// at least one mock-marker pattern to be flagged.  If the mock description
// (boundary + mock type) matches any open deferral, it is treated as declared.
func CheckBoundaryMocks(diffContent string, openDeferrals []string) BoundaryMockReport {
	var report BoundaryMockReport
	lines := strings.Split(diffContent, "\n")
	for i, raw := range lines {
		line := i + 1 // 1-indexed
		t := strings.TrimSpace(raw)

		// Skip non-added lines (---) and context lines.
		if !strings.HasPrefix(t, "+") && !strings.HasPrefix(t, "-") {
			continue
		}
		content := strings.TrimPrefix(strings.TrimPrefix(t, "+"), "-")

		// Check for mock markers.
		hasMock := false
		for _, marker := range mockMarkerPatterns {
			if strings.Contains(content, marker) {
				hasMock = true
				break
			}
		}
		if !hasMock {
			continue
		}

		// Check for boundary patterns.
		matched := ""
		for _, bp := range knownBoundaryPatterns {
			if strings.Contains(content, bp.Keyword) {
				matched = bp.Boundary
				break
			}
		}
		if matched == "" {
			continue
		}

		// Extract a compact mock-type description.
		mockType := extractMockType(content)

		// Check against open deferrals.
		bm := BoundaryMock{
			File:     "diff",
			Line:     line,
			Boundary: matched,
			MockType: mockType,
		}
		if isDeclared(mockType, matched, openDeferrals) {
			bm.Declared = true
			report.DeclaredMocks = append(report.DeclaredMocks, bm)
		} else {
			report.UndeclaredMocks = append(report.UndeclaredMocks, bm)
		}
	}
	return report
}

// extractMockType extracts a compact description of the mock from a line.
// It returns the mock-marker token and surrounding context, trimmed to 80 chars.
func extractMockType(line string) string {
	lower := strings.ToLower(line)
	for _, marker := range mockMarkerPatterns {
		idx := strings.Index(line, marker)
		if idx >= 0 {
			start := idx - 15
			if start < 0 {
				start = 0
			}
			end := idx + len(marker) + 15
			if end > len(line) {
				end = len(line)
			}
			snippet := strings.TrimSpace(line[start:end])
			if len(snippet) > 80 {
				snippet = snippet[:77] + "..."
			}
			// Single occurrence per line is sufficient.
			if strings.Contains(lower, "mock") {
				return "mock: " + snippet
			}
			if strings.Contains(lower, "fake") {
				return "fake: " + snippet
			}
			if strings.Contains(lower, "stub") {
				return "stub: " + snippet
			}
			return "testdouble: " + snippet
		}
	}
	// Fallback — take first 60 chars.
	s := line
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

// isDeclared checks whether a mock at a given boundary matches any open deferral.
// Matching is case-insensitive substring: each deferral is checked for the
// boundary name AND a mock/fake/stub keyword.  A deferral like "db mock for
// integration tests" would match a db-boundary mock.
func isDeclared(mockType, boundary string, openDeferrals []string) bool {
	for _, d := range openDeferrals {
		dl := strings.ToLower(d)
		if strings.Contains(dl, strings.ToLower(boundary)) &&
			(strings.Contains(dl, "mock") || strings.Contains(dl, "fake") ||
				strings.Contains(dl, "stub") || strings.Contains(dl, "testdouble")) {
			return true
		}
	}
	return false
}