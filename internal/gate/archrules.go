// Package gate provides lint gates for the SwornAgent CLI.
//
// archrules.go implements the architecture rule engine for `sworn lint design`.
// It reads docs/baton/architecture.json and runs four check types (grep,
// touchpoints, diff-size, external) against a slice's git diff, reporting
// violations with file:line detail.
//
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
	"strings"

	"github.com/swornagent/sworn/internal/style"
)

// --- data model ---

// ArchRulesReport holds the full structured result of RunArchRules.
type ArchRulesReport struct {
	Release    string           `json:"release"`
	Slice      string           `json:"slice"`
	Rules      int              `json:"rules_checked"`
	Violations []ArchViolation  `json:"violations"`
	Failed     int              `json:"failed"`
	Verdict    string           `json:"verdict"`
}


// ArchViolation is a single architecture rule violation.
type ArchViolation struct {
	RuleID      string `json:"rule_id"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Msg         string `json:"msg"`
}


// String returns a human-readable violation line.
func (v ArchViolation) String() string {
	s := fmt.Sprintf("[%s] %s: %s", v.Severity, v.RuleID, v.Msg)
	if v.File != "" {
		s += fmt.Sprintf("\n    in %s:%d", v.File, v.Line)
	}
	return s
}


// HasViolations returns true when the report contains violations.
func (r *ArchRulesReport) HasViolations() bool { return r.Failed > 0 }

// --- config types ---

// ArchConfig is the top-level structure of architecture.json.
type ArchConfig struct {
	Schema        string        `json:"$schema"`
	Description   string        `json:"_description"`
	CanonicalDocs CanonicalDocs `json:"canonical_docs"`
	Rules         []ArchRule    `json:"rules"`
}


// CanonicalDocs holds canonical architecture source-of-truth paths.
type CanonicalDocs struct {
	Description            string   `json:"_description"`
	DataModel              string   `json:"data_model"`
	APIContracts           []string `json:"api_contracts"`
	ComponentHierarchy     []string `json:"component_hierarchy"`
	ArchitecturalDecisions string   `json:"architectural_decisions"`
	DesignTokens           string   `json:"design_tokens"`
}


// ArchRule is a single architecture rule from architecture.json.
type ArchRule struct {
	ID            string `json:"id"`
	Description   string `json:"description"`
	Check         string `json:"check"`
	Pattern       string `json:"pattern,omitempty"`
	Files         string `json:"files,omitempty"`
	MaxLinesAdded int    `json:"max_lines_added,omitempty"`
	MaxFileLines  int    `json:"max_file_lines,omitempty"`
	Command       string `json:"command,omitempty"`
	Severity      string `json:"severity"`
	Note          string `json:"note"`
}


// --- allowlist ---

// DesignAllowlist holds per-slice exception rules from design-allowlist.json.
type DesignAllowlist struct {
	Schema      string              `json:"$schema"`
	Description string              `json:"_description"`
	Rules       []AllowlistEntry    `json:"rules"`
}


// AllowlistEntry is a single allowlist entry.
type AllowlistEntry struct {
	RuleID string `json:"rule_id"`
	File   string `json:"file,omitempty"`
	Reason string `json:"reason"`
}


// --- main entry point ---

// RunArchRules loads architecture.json from docs/baton/, reads the slice's git
// diff against baseRef, and runs every configured architecture rule.
//
// Parameters:
//
//	releaseDir — absolute path to docs/release/<release-name>/
//	sliceID    — e.g. "S67-lint-design"
//	baseRef    — git ref for the diff base (start_commit or "release-wt/<release>")
//
// Returns an error only for I/O / git / config failures; violations are in the report.
func RunArchRules(releaseDir, sliceID, baseRef string) (*ArchRulesReport, error) {
	// Resolve project root: walk up from releaseDir until we find .git.
	root := releaseDir
	for {
		if _, serr := os.Stat(filepath.Join(root, ".git")); serr == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return nil, fmt.Errorf("archrules: cannot find repo root from %s", releaseDir)
		}
		root = parent
	}
	releaseName := filepath.Base(releaseDir)
	sliceDir := filepath.Join(releaseDir, sliceID)

	// 1. Load architecture config.
	archPath := filepath.Join(root, "docs", "baton", "architecture.json")
	cfg, err := loadArchConfig(archPath)
	if err != nil {
		// No architecture.json is not an error — just nothing to check.
		cfg = &ArchConfig{Rules: nil}
	}

	// 2. Load per-slice allowlist.
	allowlistPath := filepath.Join(sliceDir, "design-allowlist.json")
	allowlist, _ := loadAllowlist(allowlistPath)
	if allowlist == nil {
		allowlist = &DesignAllowlist{}
	}

	// 3. Get changed files from the diff.
	changedFiles, err := diffChangedFiles(baseRef)
	if err != nil {
		return nil, fmt.Errorf("archrules: git diff: %w", err)
	}

	report := &ArchRulesReport{
		Release: releaseName,
		Slice:   sliceID,
		Rules:   len(cfg.Rules),
	}

	// 4. Run each rule.
	for _, rule := range cfg.Rules {
		viols := runRule(rule, root, changedFiles, baseRef, sliceDir, allowlist)
		for _, v := range viols {
			report.Violations = append(report.Violations, v)
			if v.Severity == "error" {
				report.Failed++
			}
		}
	}

	if report.Failed == 0 {
		report.Verdict = "PASS"
	} else {
		report.Verdict = "FAIL"
	}

	return report, nil
}


// --- config loading ---

func loadArchConfig(path string) (*ArchConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ArchConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse architecture.json: %w", err)
	}
	return &cfg, nil
}


func loadAllowlist(path string) (*DesignAllowlist, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var al DesignAllowlist
	if err := json.Unmarshal(data, &al); err != nil {
		return nil, fmt.Errorf("parse design-allowlist.json: %w", err)
	}
	return &al, nil
}


// isExempt checks whether a violation at file:line is suppressed by the allowlist.
func isExempt(allowlist *DesignAllowlist, ruleID, file string) bool {
	for _, e := range allowlist.Rules {
		if e.RuleID == ruleID {
			if e.File == "" || e.File == file {
				return true
			}
		}
	}
	return false
}


// --- git helpers ---

// diffChangedFiles returns the list of files changed between baseRef and HEAD.
func diffChangedFiles(baseRef string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", baseRef, "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}


// diffFileContent returns the full diff (added lines) for a specific file.
func diffFileContent(baseRef, file string) ([]string, error) {
	cmd := exec.Command("git", "diff", baseRef, "HEAD", "--", file)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %w", file, err)
	}
	return strings.Split(string(out), "\n"), nil
}


// diffAddedLines returns only the added lines (prefixed with + but not +++ ) from the diff.
func diffAddedLines(baseRef, file string) ([]lineInfo, error) {
	diffLines, err := diffFileContent(baseRef, file)
	if err != nil {
		return nil, err
	}
	var lines []lineInfo
	lineNo := 0
	inHunk := false
	hunkNewLine := 0
	for _, dl := range diffLines {
		if strings.HasPrefix(dl, "@@") {
			inHunk = true
			// Parse the new-file line number from "@@ -old,N +new,N @@"
			hunkNewLine = parseHunkNewStart(dl)
			lineNo = hunkNewLine
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(dl, "+") && !strings.HasPrefix(dl, "+++") {
			content := dl[1:]
			lines = append(lines, lineInfo{lineNo, content})
			lineNo++
		} else if !strings.HasPrefix(dl, "-") {
			lineNo++
		}
	}
	return lines, nil
}


// diffFileSize returns the number of lines in a file at HEAD.
func diffFileSize(root, file string) (int, error) {
	abs := filepath.Join(root, file)
	data, err := os.ReadFile(abs)
	if err != nil {
		return 0, err
	}
	return len(strings.Split(string(data), "\n")), nil
}


type lineInfo struct {
	Line int
	Text string
}


// parseHunkNewStart parses the new-file start line from a unified diff hunk header.
// Format: @@ -oldStart,oldCount +newStart,newCount @@
func parseHunkNewStart(line string) int {
	re := regexp.MustCompile(`\+(\d+)(?:,(\d+))?\s+@@`)
	m := re.FindStringSubmatch(line)
	if m != nil {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		return n
	}
	return 1
}


// --- rule execution ---

// runRule executes a single architecture rule against the diff.
func runRule(rule ArchRule, root string, changedFiles []string, baseRef, sliceDir string, allowlist *DesignAllowlist) []ArchViolation {
	switch rule.Check {
	case "grep":
		return runGrepRule(rule, root, changedFiles, baseRef, allowlist)
	case "touchpoints":
		return runTouchpointsRule(rule, changedFiles, sliceDir, allowlist)
	case "diff-size":
		return runDiffSizeRule(rule, root, changedFiles, baseRef, allowlist)
	case "external":
		return runExternalRule(rule, allowlist)
	default:
		return []ArchViolation{{
			RuleID:      rule.ID,
			Description: rule.Description,
			Severity:    "warning",
			Msg:         fmt.Sprintf("unknown check type %q — skipped", rule.Check),
		}}
	}
}


// --- grep check ---

// compileGlobToRegex compiles a glob-like pattern to a regex for file matching.
// Supports **, *, and {a,b,c} brace alternatives.
func compileGlobToRegex(pattern string) (*regexp.Regexp, error) {
	// Handle brace expansion before quoting: replace {a,b} with a sentinel,
	// quote everything else, then restore.
	braceMap := make(map[string]string)
	braceIdx := 0
	for {
		start := strings.Index(pattern, "{")
		if start < 0 {
			break
		}
		end := strings.Index(pattern[start:], "}")
		if end < 0 {
			break
		}
		end += start
		inner := pattern[start+1 : end]
		parts := strings.Split(inner, ",")
		for i, p := range parts {
			parts[i] = regexp.QuoteMeta(p)
		}
		replacement := "(" + strings.Join(parts, "|") + ")"
		key := fmt.Sprintf("\x00BRACE%d\x00", braceIdx)
		braceMap[key] = replacement
		pattern = pattern[:start] + key + pattern[end+1:]
		braceIdx++
	}

	// Escape regex metacharacters except * and **.
	s := regexp.QuoteMeta(pattern)
	// Unescape \*\* → .*
	s = strings.ReplaceAll(s, `\*\*`, `.*`)
	// Unescape \* → [^/]*
	s = strings.ReplaceAll(s, `\*`, `[^/]*`)
	// Restore brace expansions.
	for key, val := range braceMap {
		s = strings.ReplaceAll(s, key, val)
	}
	// Anchor to match full path.
	return regexp.Compile("^" + s + "$")
}
func matchGlob(file, pattern string) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	// Simple glob matching — supports ** and *
	re, err := compileGlobToRegex(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(file), nil
}


// skipTestFile returns true when the file path looks like a test file.
func skipTestFile(path string) bool {
	return isTestFilePath(path)
}


// isTestFilePath returns true for test/spec files (broader than isTestFile which
// is already in coverage.go; re-exported here to avoid import cycles — the func
// is duplicated intentionally as both files are in the same package).
func isTestFilePath(path string) bool {
	base := filepath.Base(path)
	// Go test files
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// TS test files
	for _, s := range []string{".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx", ".test.js", ".test.jsx", ".spec.js", ".spec.jsx"} {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	// Python test files
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return true
	}
	return false
}


func runGrepRule(rule ArchRule, root string, changedFiles []string, baseRef string, allowlist *DesignAllowlist) []ArchViolation {
	var violations []ArchViolation
	patternRe, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return []ArchViolation{{
			RuleID:      rule.ID,
			Description: rule.Description,
			Severity:    "warning",
			Msg:         fmt.Sprintf("invalid regex pattern: %v", err),
		}}
	}

	for _, file := range changedFiles {
		// Skip test files by default.
		if skipTestFile(file) {
			continue
		}

		// Check file glob if specified.
		if rule.Files != "" {
			match, err := matchGlob(file, rule.Files)
			if err != nil || !match {
				continue
			}
		}

		if isExempt(allowlist, rule.ID, file) {
			continue
		}

		added, err := diffAddedLines(baseRef, file)
		if err != nil {
			continue
		}

		for _, li := range added {
			if patternRe.MatchString(li.Text) {
				violations = append(violations, ArchViolation{
					RuleID:      rule.ID,
					Description: rule.Description,
					Severity:    rule.Severity,
					File:        file,
					Line:        li.Line,
					Msg:         fmt.Sprintf("pattern matched: %s", rule.Note),
				})
			}
		}
	}
	return violations
}


// --- touchpoints check ---

func runTouchpointsRule(rule ArchRule, changedFiles []string, sliceDir string, allowlist *DesignAllowlist) []ArchViolation {
	var violations []ArchViolation
	planned := readPlannedFiles(sliceDir)

	for _, file := range changedFiles {
		if skipTestFile(file) {
			continue
		}
		if isExempt(allowlist, rule.ID, file) {
			continue
		}
		if !planned[file] {
			violations = append(violations, ArchViolation{
				RuleID:      rule.ID,
				Description: rule.Description,
				Severity:    rule.Severity,
				File:        file,
				Msg:         fmt.Sprintf("file changed but not in planned touchpoints: %s", rule.Note),
			})
		}
	}
	return violations
}


// readPlannedFiles reads a slice's status.json and returns a set of planned file paths.
func readPlannedFiles(sliceDir string) map[string]bool {
	statusPath := filepath.Join(sliceDir, "status.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil
	}
	// Extract planned_files array via regex.
	re := regexp.MustCompile(`"([^"]+)"`)
	// Find the planned_files block.
	idx := strings.Index(string(data), `"planned_files"`)
	if idx < 0 {
		return nil
	}
	block := string(data)[idx:]
	// Find the closing ]
	end := strings.Index(block, "]")
	if end < 0 {
		return nil
	}
	block = block[:end+1]

	files := make(map[string]bool)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		f := m[1]
		if f != "planned_files" {
			files[f] = true
		}
	}
	return files
}


// --- diff-size check ---

func runDiffSizeRule(rule ArchRule, root string, changedFiles []string, baseRef string, allowlist *DesignAllowlist) []ArchViolation {
	var violations []ArchViolation

	for _, file := range changedFiles {
		if skipTestFile(file) {
			continue
		}
		if isExempt(allowlist, rule.ID, file) {
			continue
		}

		// Check growth limit (lines added).
		if rule.MaxLinesAdded > 0 {
			added, err := diffAddedLines(baseRef, file)
			if err != nil {
				continue
			}
			if len(added) > rule.MaxLinesAdded {
				violations = append(violations, ArchViolation{
					RuleID:      rule.ID,
					Description: rule.Description,
					Severity:    rule.Severity,
					File:        file,
					Msg:         fmt.Sprintf("added %d lines (limit %d): %s", len(added), rule.MaxLinesAdded, rule.Note),
				})
			}
		}

		// Check absolute size limit.
		if rule.MaxFileLines > 0 {
			size, err := diffFileSize(root, file)
			if err != nil {
				continue
			}
			if size > rule.MaxFileLines {
				violations = append(violations, ArchViolation{
					RuleID:      rule.ID,
					Description: rule.Description,
					Severity:    rule.Severity,
					File:        file,
					Msg:         fmt.Sprintf("file is %d lines (limit %d): %s", size, rule.MaxFileLines, rule.Note),
				})
			}
		}
	}
	return violations
}


// --- external check ---

func runExternalRule(rule ArchRule, allowlist *DesignAllowlist) []ArchViolation {
	if isExempt(allowlist, rule.ID, "") {
		return nil
	}

	if rule.Command == "" {
		return []ArchViolation{{
			RuleID:      rule.ID,
			Description: rule.Description,
			Severity:    "warning",
			Msg:         fmt.Sprintf("external check has no command — skipped: %s", rule.Note),
		}}
	}

	// Simple command execution: use /bin/sh -c
	cmd := exec.Command("sh", "-c", rule.Command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Non-zero exit = violation.
		return []ArchViolation{{
			RuleID:      rule.ID,
			Description: rule.Description,
			Severity:    rule.Severity,
			Msg:         fmt.Sprintf("external check failed: %s (output: %s)", rule.Note, strings.TrimSpace(string(out))),
		}}
	}
	_ = out // command succeeded, no violation
	return nil
}


// --- human-readable output ---

// PrintArchRules renders the ArchRulesReport as human-readable text.
func PrintArchRules(r *ArchRulesReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(style.Bold(fmt.Sprintf("ARCHITECTURE RULES — %s / %s", r.Release, r.Slice)))
	b.WriteString("\n\n")
	b.WriteString(style.Dim(fmt.Sprintf("Rules: %d checked  violations: %d\n", r.Rules, len(r.Violations))))
	b.WriteString("\n")

	for i, v := range r.Violations {
		severityStyle := style.Warn
		if v.Severity == "error" {
			severityStyle = style.Danger
		}
		b.WriteString(severityStyle(fmt.Sprintf("  %d. [%s] %s — %s\n", i+1, v.Severity, v.RuleID, v.Msg)))
		if v.File != "" {
			b.WriteString(style.Dim(fmt.Sprintf("     in %s:%d\n", v.File, v.Line)))
		}
	}
	b.WriteString("\n")

	if r.Verdict == "PASS" {
		b.WriteString(style.Success("PASS — no architecture rule violations\n"))
	} else {
		b.WriteString(style.Danger(fmt.Sprintf("FAIL — %d error violation(s)\n", r.Failed)))
	}
	b.WriteString("\n")

	return b.String()
}


// JSONArchRules returns the report as pretty-printed JSON.
func JSONArchRules(r *ArchRulesReport) string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}


// readFileLines reads a file and returns its lines.
func readFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}
