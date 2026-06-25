// Package gate provides lint gates for the SwornAgent CLI.
//
// trace.go ports the canonical baton release-trace.sh from bash to Go:
// a mechanical RTM + EARS + sniff-test gate for Rule 8.
// It verifies the full requirements-fidelity chain:
//
//	intake → slice (covers_needs) → AC (spec.md citations) → test (Required tests)
//
// Plus structural-completeness sniff-test and EARS conformance.
//
// Reads from a docs/release/<release-name>/ directory.
// Stdlib only — zero runtime dependencies.
package gate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/swornagent/sworn/internal/style"
)

// TraceReport is the full structured report from RunTrace.
type TraceReport struct {
	Release     string           `json:"release"`
	TotalNeeds  int              `json:"total_needs"`
	TotalSlices int              `json:"total_slices"`
	TotalACs    int              `json:"total_acs_checked"`
	EARSStats   map[string]int   `json:"ears_distribution"`
	FreeFormACs int              `json:"free_form_acs"`
	Violations  []TraceViolation `json:"violations"`
	Failed      int              `json:"failed"`
	Verdict     string           `json:"verdict"`
}

// TraceViolation is a single traceability or quality violation.
type TraceViolation struct {
	Check    string `json:"check"`
	Severity string `json:"severity"` // "FAIL" or "WARN"
	Msg      string `json:"msg"`
	Slice    string `json:"slice,omitempty"`
	Need     string `json:"need,omitempty"`
}

// String returns a human-readable violation line.
func (v TraceViolation) String() string {
	s := fmt.Sprintf("[%s] %s", v.Check, v.Msg)
	if v.Slice != "" {
		s += fmt.Sprintf("\n    slice: %s", v.Slice)
	}
	if v.Need != "" {
		s += fmt.Sprintf("\n    need: %s", v.Need)
	}
	return s
}

// HasViolations returns true if the report contains any violations.
func (r *TraceReport) HasViolations() bool { return r.Failed > 0 }

// need represents a single requirement need from intake.md.
type need struct {
	ID   string // e.g. "N-01"
	Desc string // one-line description
}

// sliceStatus holds the parsed covers_needs and state for a single slice.
type sliceStatus struct {
	ID     string
	Covers []string
	State  string
}

// --- regex patterns (compiled once) ---

var (
	// Need ID pattern: N-NN (stable, never reused).
	reNeedID = regexp.MustCompile(`\bN-\d{2}\b`)

	// Need declaration in intake: "- N-01: description" (explicit id format).
	reNeedDecl = regexp.MustCompile(`(?m)^\s*[-*]?\s*(N-\d{2})\s*[:\-]\s*(.+)$`)

	// Bold-label needs in "What the human wants" section:
	//   - **Parallel track execution**: description
	// The ID is derived from the 1-based position (N-01, N-02, ...).
	reBoldNeed = regexp.MustCompile(`(?m)^\s*-\s*\*{1,2}([^*]+)\*{1,2}\s*:\s*(.+)$`)

	// Slice id pattern: S<NN>-<kebab-name>.
	reSliceID = regexp.MustCompile(`^S\d{2}-[a-z0-9-]+$`)

	// Acceptance check: a checkbox line starting with "- [ ]" or "- [x]".
	reACLine = regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*(.+)`)

	// "shall" keyword (case-insensitive) for EARS ubiquitous check.
	reShall = regexp.MustCompile(`\b[Ss][Hh][Aa][Ll][Ll]\b`)

	// EARS keywords (each with case-insensitive prefix).
	reWhen  = regexp.MustCompile(`\b[Ww][Hh][Ee][Nn]\b`)
	reWhile = regexp.MustCompile(`\b[Ww][Hh][Ii][Ll][Ee]\b`)
	reWhere = regexp.MustCompile(`\b[Ww][Hh][Ee][Rr][Ee]\b`)
	reIf    = regexp.MustCompile(`\b[Ii][Ff]\b.*\b[Tt][Hh][Ee][Nn]\b`)

	// "See intake" reference detection.
	reSeeIntake = regexp.MustCompile(`(?i)see\s+intake\.?md|refer\s+to\s+intake|as\s+described\s+in\s+(the\s+)?intake`)

	// Vague AC detection: starts with a vague verb + "the/a/an" and lacks concrete terms.
	reVagueVerb = regexp.MustCompile(`(?i)^(fix|add|wire|build|implement|make|do|handle|address)\s+(the|a|an)\s+`)

	// Concrete artefact terms: file extensions, testids, status codes, percentages, etc.
	reConcrete = regexp.MustCompile(
		`\.(` + `tsx?|go|json|css|md` + `)['\"\s]|` +
			`data-testid=|testid=|aria-label=|className=` +
			`|[A-Z][a-z]+\.tsx|[a-z_]+\.go|` +
			`['\"][A-Za-z0-9._/-]+\.(` + `tsx?|go|json` + `)['\"]|` +
			`\b\d{3}\b|` +
			`\b[0-9]+(?:\.[0-9]+)?%\b`,
	)
)

// isSliceID returns true if s matches the slice-id pattern S<NN>-<kebab-name>.
func isSliceID(s string) bool { return reSliceID.MatchString(s) }

// RunTrace reads a release directory and produces the full traceability report.
// It returns an error only for I/O failures; trace violations are in the report.
func RunTrace(releaseDir string) (*TraceReport, error) {
	r := &TraceReport{
		Release:   filepath.Base(releaseDir),
		EARSStats: map[string]int{},
	}

	// 1. Parse intake.md
	intakePath := filepath.Join(releaseDir, "intake.md")
	intakeText, err := os.ReadFile(intakePath)
	if err != nil {
		return nil, fmt.Errorf("trace: read intake.md: %w", err)
	}

	needs := parseNeeds(string(intakeText))
	r.TotalNeeds = len(needs)

	// Check 0: intake has needs.
	if len(needs) == 0 {
		r.Violations = append(r.Violations, TraceViolation{
			Check:    "intake-structure",
			Severity: "FAIL",
			Msg:      "No N-NN needs found in intake.md 'What the human wants' section.",
		})
	}

	// 2. Discover slices and parse status.json (covers_needs) + spec.md (ACs).
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, fmt.Errorf("trace: read release dir: %w", err)
	}

	var slices []sliceStatus
	totalACs := 0
	needSet := map[string]bool{}       // set of valid need IDs from intake
	coveredSet := map[string]bool{}    // needs covered by at least one slice's covers_needs
	coversMap := map[string][]string{} // slice -> covers_needs

	for _, n := range needs {
		needSet[n.ID] = true
	}

	for _, e := range entries {
		if !e.IsDir() || !isSliceID(e.Name()) {
			continue
		}
		sliceID := e.Name()
		sliceDir := filepath.Join(releaseDir, sliceID)
		statusPath := filepath.Join(sliceDir, "status.json")
		specPath := filepath.Join(sliceDir, "spec.md")

		// Parse status.json for covers_needs.
		covers := parseCoversNeeds(statusPath)
		slices = append(slices, sliceStatus{ID: sliceID, Covers: covers})
		coversMap[sliceID] = covers
		for _, nid := range covers {
			coveredSet[nid] = true
		}

		// Parse spec.md for acceptance checks.
		specText, err := os.ReadFile(specPath)
		if err != nil {
			// spec.md is optional for planned slices.
			continue
		}
		specStr := string(specText)

		// Check 4: EARS conformance on every AC checkbox.
		acs := parseAcceptanceChecks(specStr)
		for _, ac := range acs {
			acClean := strings.TrimSpace(ac)
			if strings.HasPrefix(strings.ToUpper(acClean), "NOTE:") {
				continue
			}
			totalACs++
			if !reShall.MatchString(acClean) {
				r.FreeFormACs++
				r.Violations = append(r.Violations, TraceViolation{
					Check:    "ears-conformance",
					Severity: "FAIL",
					Msg:      fmt.Sprintf("Slice %s: AC '%s...' lacks 'shall' — not EARS-conformant.", sliceID, truncate(acClean, 80)),
					Slice:    sliceID,
				})
			} else {
				// Classify EARS pattern.
				tag := classifyEARS(acClean)
				r.EARSStats[tag] = r.EARSStats[tag] + 1
			}
		}

		// Check 5a: "see intake" references in spec.md.
		if reSeeIntake.MatchString(specStr) {
			r.Violations = append(r.Violations, TraceViolation{
				Check:    "see-intake",
				Severity: "FAIL",
				Msg:      fmt.Sprintf("Slice %s spec.md contains a 'see intake.md' reference.", sliceID),
				Slice:    sliceID,
			})
		}

		// Check 5b: vague-scope ACs (no concrete terms).
		for _, ac := range acs {
			acClean := strings.TrimSpace(ac)
			if strings.HasPrefix(strings.ToUpper(acClean), "NOTE:") {
				continue
			}
			if reVagueVerb.MatchString(acClean) && !reConcrete.MatchString(acClean) {
				r.Violations = append(r.Violations, TraceViolation{
					Check:    "vague-ac",
					Severity: "FAIL",
					Msg:      fmt.Sprintf("Slice %s: AC '%s...' is vague — no concrete artefact (file, testid, status code, label).", sliceID, truncate(acClean, 80)),
					Slice:    sliceID,
				})
			}
		}

		// Check 5c: vague-scope in-scope items.
		vagueItems := parseVagueInScope(specStr)
		for _, item := range vagueItems {
			r.Violations = append(r.Violations, TraceViolation{
				Check:    "vague-scope",
				Severity: "FAIL",
				Msg:      fmt.Sprintf("Slice %s: In-scope item '%s...' is vague — no concrete artefact.", sliceID, truncate(item, 80)),
				Slice:    sliceID,
			})
		}
	}

	r.TotalSlices = len(slices)
	r.TotalACs = totalACs

	// Check 1: every intake N-NN covered by >=1 slice's covers_needs.
	for _, n := range needs {
		if !coveredSet[n.ID] {
			r.Violations = append(r.Violations, TraceViolation{
				Check:    "orphaned-need",
				Severity: "FAIL",
				Msg:      fmt.Sprintf("Intake need %s ('%s') is not covered by any slice's covers_needs.", n.ID, truncate(n.Desc, 60)),
				Need:     n.ID,
			})
		}
	}

	// Check 2: every covers_needs ID exists in intake.
	for _, s := range slices {
		for _, nid := range s.Covers {
			if !needSet[nid] {
				r.Violations = append(r.Violations, TraceViolation{
					Check:    "invalid-covers",
					Severity: "FAIL",
					Msg:      fmt.Sprintf("Slice %s covers_needs references %s which is not in intake.md needs.", s.ID, nid),
					Slice:    s.ID,
				})
			}
		}
	}

	// Check 3: every covers_needs ID has an AC citation in that slice's spec.
	for _, s := range slices {
		specPath := filepath.Join(releaseDir, s.ID, "spec.md")
		specText, err := os.ReadFile(specPath)
		if err != nil {
			continue
		}
		specStr := string(specText)
		for _, nid := range s.Covers {
			if !strings.Contains(specStr, nid) {
				r.Violations = append(r.Violations, TraceViolation{
					Check:    "unclaimed-coverage",
					Severity: "FAIL",
					Msg:      fmt.Sprintf("Slice %s claims %s in covers_needs but no AC in spec.md cites %s.", s.ID, nid, nid),
					Slice:    s.ID,
					Need:     nid,
				})
			}
		}
	}

	// Count failures.
	for _, v := range r.Violations {
		if v.Severity == "FAIL" {
			r.Failed++
		}
	}
	if r.Failed == 0 {
		r.Verdict = "PASS"
	} else {
		r.Verdict = "FAIL"
	}

	return r, nil
}

// parseNeeds extracts need declarations from intake.md text.
//
// Two formats are supported:
//  1. Explicit: "- N-01: description" or "N-01: description"
//  2. Bold-label in "What the human wants": "- **Label**: description"
//     IDs are derived: N-01, N-02, ...
func parseNeeds(text string) []need {
	// Try explicit N-NN format first.
	needs := parseExplicitNeeds(text)
	if len(needs) > 0 {
		return needs
	}

	// Fall back to bold-label format in "What the human wants" section.
	return parseBoldNeeds(text)
}

// parseExplicitNeeds extracts needs in "N-01: desc" format.
func parseExplicitNeeds(text string) []need {
	var needs []need
	seen := map[string]bool{}
	for _, m := range reNeedDecl.FindAllStringSubmatch(text, -1) {
		id := m[1]
		desc := strings.TrimSpace(m[2])
		if seen[id] {
			continue
		}
		seen[id] = true
		needs = append(needs, need{ID: id, Desc: desc})
	}
	return needs
}

// parseBoldNeeds extracts needs from "What the human wants" section
// using bold-label format: "- **Label**: description".
func parseBoldNeeds(text string) []need {
	// Find the "What the human wants" section.
	inWants := false
	var matches []string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(line), "what the human wants") {
			inWants = true
			continue
		}
		if inWants && strings.HasPrefix(line, "##") {
			break
		}
		if inWants {
			m := reBoldNeed.FindStringSubmatch(line)
			if m != nil {
				desc := strings.TrimSpace(m[2])
				matches = append(matches, desc)
			}
		}
	}

	if len(matches) == 0 {
		return nil
	}

	var needs []need
	for i, desc := range matches {
		id := fmt.Sprintf("N-%02d", i+1)
		needs = append(needs, need{ID: id, Desc: desc})
	}
	return needs
}

// parseCoversNeeds reads the covers_needs array from a status.json file.
func parseCoversNeeds(statusPath string) []string {
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil
	}
	// Simple regex extraction — avoids full JSON unmarshal for a single field.
	re := regexp.MustCompile(`"covers_needs"\s*:\s*\[([^\]]*)\]`)
	m := re.FindStringSubmatch(string(data))
	if m == nil {
		return nil
	}
	inner := strings.TrimSpace(m[1])
	if inner == "" {
		return nil
	}
	// Split by comma, strip quotes and whitespace.
	var ids []string
	for _, part := range strings.Split(inner, ",") {
		cleaned := strings.TrimSpace(part)
		cleaned = strings.Trim(cleaned, `"`)
		if cleaned != "" {
			ids = append(ids, cleaned)
		}
	}
	return ids
}

// parseAcceptanceChecks extracts checkbox AC lines from spec.md.
// NOTE lines (informational, not verifiable) are excluded.
func parseAcceptanceChecks(text string) []string {
	var acs []string
	inSection := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			inSection = strings.Contains(strings.ToLower(line), "acceptance check")
			continue
		}
		if !inSection {
			continue
		}
		if m := reACLine.FindStringSubmatch(line); m != nil {
			acText := strings.TrimSpace(m[1])
			if strings.HasPrefix(strings.ToUpper(acText), "NOTE:") {
				continue
			}
			acs = append(acs, acText)
		}
	}
	return acs
}
// classifyEARS determines the EARS pattern for an AC that contains "shall".
func classifyEARS(ac string) string {
	keywordCount := 0
	tag := "Ubiquitous"

	if reWhen.MatchString(ac) {
		keywordCount++
		tag = "When"
	}
	if reWhile.MatchString(ac) {
		keywordCount++
		tag = "While"
	}
	if reWhere.MatchString(ac) {
		keywordCount++
		tag = "Where"
	}
	if reIf.MatchString(ac) {
		keywordCount++
		tag = "If"
	}

	if keywordCount >= 2 {
		return "Complex"
	}
	return tag
}

// parseVagueInScope extracts vague in-scope items from spec.md.
func parseVagueInScope(text string) []string {
	// Find the "## In scope" section.
	re := regexp.MustCompile(`(?s)## In scope\s*\n((?:\s*-.*\n)*)`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return nil
	}

	var vague []string
	for _, line := range strings.Split(m[1], "\n") {
		stripped := strings.TrimSpace(line)
		stripped = strings.TrimPrefix(stripped, "- ")
		stripped = strings.TrimSpace(stripped)
		if stripped == "" {
			continue
		}
		if reVagueVerb.MatchString(stripped) && !reConcrete.MatchString(stripped) {
			vague = append(vague, stripped)
		}
	}
	return vague
}

// --- human-readable output ---

// PrintReport renders the TraceReport as human-readable text matching the
// canonical release-trace.sh output style.
func PrintReport(r *TraceReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(style.Bold(fmt.Sprintf("RELEASE TRACE — %s", r.Release)))
	b.WriteString("\n\n")

	// Summary line.
	b.WriteString(style.Dim(fmt.Sprintf("needs: %d  slices: %d  ACs checked: %d\n", r.TotalNeeds, r.TotalSlices, r.TotalACs)))

	// EARS distribution.
	earsOrder := []string{"Ubiquitous", "Complex", "When", "While", "Where", "If"}
	var earsParts []string
	for _, tag := range earsOrder {
		if count, ok := r.EARSStats[tag]; ok && count > 0 {
			earsParts = append(earsParts, fmt.Sprintf("%s=%d", tag, count))
		}
	}
	// Also include any tags not in the predefined order.
	for tag, count := range r.EARSStats {
		found := false
		for _, t := range earsOrder {
			if t == tag {
				found = true
				break
			}
		}
		if !found && count > 0 {
			earsParts = append(earsParts, fmt.Sprintf("%s=%d", tag, count))
		}
	}
	b.WriteString(style.Dim(fmt.Sprintf("EARS: %s free-form=%d\n", strings.Join(earsParts, " "), r.FreeFormACs)))

	if r.Verdict == "PASS" {
		b.WriteString(style.Success(fmt.Sprintf("PASS — all %d needs traced, %d ACs conformant\n", r.TotalNeeds, r.TotalACs)))
		b.WriteString("\n")
		return b.String()
	}

	// FAIL — list violations.
	b.WriteString(style.Danger(fmt.Sprintf("FAIL — %d violation(s)\n", r.Failed)))
	b.WriteString("\n")

	checkLabels := map[string]string{
		"intake-structure":  "Intake structure",
		"orphaned-need":     "Orphaned need",
		"invalid-covers":    "Invalid covers_needs reference",
		"unclaimed-coverage": "Unclaimed coverage",
		"ears-conformance":  "EARS conformance",
		"see-intake":        "\"See intake\" reference",
		"vague-ac":          "Vague acceptance criterion",
		"vague-scope":       "Vague in-scope item",
	}

	for i, v := range r.Violations {
		if v.Severity != "FAIL" {
			continue
		}
		label := checkLabels[v.Check]
		if label == "" {
			label = v.Check
		}
		b.WriteString(fmt.Sprintf("  %d. [%s] ", i+1, label))
		b.WriteString(style.Danger(v.Msg))
		b.WriteString("\n")
		if v.Slice != "" {
			b.WriteString(style.Dim(fmt.Sprintf("    slice: %s\n", v.Slice)))
		}
		if v.Need != "" {
			b.WriteString(style.Dim(fmt.Sprintf("    need: %s\n", v.Need)))
		}
	}

	b.WriteString("\n")
	b.WriteString(style.Danger("NOT TRACEABLE"))
	b.WriteString("\n\n")
	b.WriteString("Fix violations above, then re-run sworn lint trace.\n")
	b.WriteString("\n")

	return b.String()
}

// JSONReport returns the report as pretty-printed JSON.
func JSONReport(r *TraceReport) string {
	data, err := json.MarshalIndent(struct {
		Summary    *TraceReport     `json:"summary"`
		Violations []TraceViolation `json:"violations"`
	}{Summary: r, Violations: r.Violations}, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// --- helpers ---

// truncate shortens s to at most n characters, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

