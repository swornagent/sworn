// Package ears implements the EARS (Easy Approach to Requirements Syntax)
// acceptance-criteria notation for Baton releases.
//
// EARS is a lightweight, structured notation for writing requirements as
// single sentences with a fixed keyword shape. The six pattern classes are:
//
//   - Ubiquitous:       THE SYSTEM SHALL <action>
//   - Event-driven:     WHEN <trigger> THE SYSTEM SHALL <action>
//   - State-driven:     WHILE <state> THE SYSTEM SHALL <action>
//   - Optional-feature: WHERE <feature> THE SYSTEM SHALL <action>
//   - Unwanted:         IF <condition> THEN THE SYSTEM SHALL <action>
//   - Complex:          a combination of two or more of the above preconditions
//
// A line prefixed with "NOTE:" is a deliberate non-requirement note and is
// excluded from classification (the escape hatch).
//
// The package provides Classify (classify a single AC line) and Validate
// (classify every AC across a release and report violations). The validator
// fails closed: any AC that matches no EARS pattern is a violation.
//
// Stdlib only — zero runtime dependencies.
package ears

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/spec"
	"github.com/swornagent/sworn/internal/style"
)

// Pattern is the EARS pattern class assigned to an acceptance criterion.
type Pattern string

const (
	// PatternUbiquitous: "THE SYSTEM SHALL <action>"
	PatternUbiquitous Pattern = "ubiquitous"
	// PatternEventDriven: "WHEN <trigger> THE SYSTEM SHALL <action>"
	PatternEventDriven Pattern = "event-driven"
	// PatternStateDriven: "WHILE <state> THE SYSTEM SHALL <action>"
	PatternStateDriven Pattern = "state-driven"
	// PatternOptionalFeature: "WHERE <feature> THE SYSTEM SHALL <action>"
	PatternOptionalFeature Pattern = "optional-feature"
	// PatternUnwanted: "IF <condition> THEN THE SYSTEM SHALL <action>"
	PatternUnwanted Pattern = "unwanted-behaviour"
	// PatternComplex: a combination of two or more preconditions
	PatternComplex Pattern = "complex"
	// PatternNote: a deliberate non-requirement note (NOTE: prefix), excluded
	PatternNote Pattern = "note"
	// PatternNone: the AC matches no EARS pattern (free-form)
	PatternNone Pattern = "none"
)

// Result is the classification of a single acceptance criterion.
//
// Line is a 1-based line number within spec.md for a spec.md-sourced AC. For
// a spec.json-sourced AC there is no markdown body to point at, so Line is
// instead the 1-based ordinal position of the AC within acceptance_criteria[]
// (deterministic and non-zero, but not a physical line number).
type Result struct {
	SliceID string // the slice this AC belongs to
	Line    int
	Text    string  // the AC text (trimmed, without the checkbox marker)
	Pattern Pattern // the EARS pattern class, or PatternNone
}

// Violation is an AC that matches no EARS pattern (free-form).
type Violation struct {
	SliceID string
	Line    int
	Text    string
}

// String returns a human-readable violation line.
func (v Violation) String() string {
	return fmt.Sprintf("%s: line %d: %s", v.SliceID, v.Line, truncate(v.Text, 80))
}

// Distribution counts how many ACs matched each EARS pattern.
type Distribution map[Pattern]int

// Report is the full validation report for a release.
type Report struct {
	Results    []Result
	Violations []Violation
	Dist       Distribution
	TotalACs   int // excludes NOTE: lines
	TotalNotes int
}

// HasViolations returns true if any AC matched no EARS pattern.
func (r *Report) HasViolations() bool {
	return len(r.Violations) > 0
}

// --- regexes for EARS pattern matching ---
//
// All matchers are case-insensitive and whitespace-tolerant. The core of every
// EARS pattern is the "THE SYSTEM SHALL" clause. The precondition keywords
// (WHEN/WHILE/WHERE/IF) distinguish the non-ubiquitous patterns.

var (
	// reACLine matches a checkbox line: "- [ ] text" or "- [x] text"
	reACLine = regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*(.+)`)

	// reNote matches a NOTE: escape prefix (case-insensitive, after checkbox).
	reNote = regexp.MustCompile(`(?i)^\s*NOTE\s*:`)

	// reShall matches the core EARS clause: "THE SYSTEM SHALL" (case-insensitive,
	// whitespace-tolerant between words).
	reShall = regexp.MustCompile(`(?i)\bTHE\s+SYSTEM\s+SHALL\b`)

	// Precondition keywords — matched anywhere in the precondition part
	// (the text before the SHALL clause). Word-bounded, case-insensitive.
	reWhen  = regexp.MustCompile(`(?i)\bWHEN\b`)
	reWhile = regexp.MustCompile(`(?i)\bWHILE\b`)
	reWhere = regexp.MustCompile(`(?i)\bWHERE\b`)
	reIf    = regexp.MustCompile(`(?i)\bIF\b`)
	reThen  = regexp.MustCompile(`(?i)\bTHEN\b`)
)

// Classify determines the EARS pattern class of a single acceptance criterion
// text line (without the checkbox marker). Returns PatternNone if the text
// matches no EARS pattern.
func Classify(text string) Pattern {
	// NOTE: escape — excluded from classification.
	if reNote.MatchString(text) {
		return PatternNote
	}

	// Every EARS pattern requires a "THE SYSTEM SHALL" clause.
	shallIdx := reShall.FindStringIndex(text)
	if shallIdx == nil {
		return PatternNone
	}

	// Extract the precondition part: everything before the SHALL clause.
	// This is where precondition keywords (WHEN/WHILE/WHERE/IF/THEN) are
	// meaningful. Keywords after SHALL are part of the action, not preconditions.
	precond := text[:shallIdx[0]]

	hasWhen := reWhen.MatchString(precond)
	hasWhile := reWhile.MatchString(precond)
	hasWhere := reWhere.MatchString(precond)
	hasIf := reIf.MatchString(precond)
	hasThen := reThen.MatchString(precond)

	// IF without THEN is an incomplete unwanted-behaviour pattern.
	if hasIf && !hasThen {
		return PatternNone
	}
	// THEN without IF is a stray keyword, not a valid precondition.
	if hasThen && !hasIf {
		return PatternNone
	}

	// Count valid preconditions.
	count := 0
	if hasWhen {
		count++
	}
	if hasWhile {
		count++
	}
	if hasWhere {
		count++
	}
	if hasIf && hasThen {
		count++
	}

	if count >= 2 {
		return PatternComplex
	}
	if count == 1 {
		if hasWhen {
			return PatternEventDriven
		}
		if hasWhile {
			return PatternStateDriven
		}
		if hasWhere {
			return PatternOptionalFeature
		}
		// hasIf && hasThen
		return PatternUnwanted
	}

	// No preconditions + SHALL clause = ubiquitous.
	return PatternUbiquitous
}

// Validate reads a release directory and classifies every acceptance check
// in every slice. It returns a Report with per-AC results, any violations
// (free-form ACs), and the per-pattern distribution.
//
// For each slice, spec.json is preferred over spec.md whenever spec.json
// exists and carries at least one acceptance criterion (AC-02/AC-04): the
// already-computed ears_pattern field is read directly instead of
// re-classifying spec.md prose text, and this holds even when spec.md is
// also present — spec.json is authoritative on disagreement. spec.md is
// used only as a legacy fallback for slices with no spec.json (pre-ADR-0009
// releases). This source-of-truth order deliberately differs from
// internal/gate/trace.go's spec.md-first order: trace.go also runs
// text-level prose checks that need a markdown body, which ears.go does not.
//
// The releaseDir is the path to docs/release/<release-name>/, which must
// contain one subdirectory per slice. Non-slice directories (e.g.
// screenshots) are skipped.
//
// Validate returns an error for I/O failures and for a malformed spec.json
// (spec.ReadRecord fails closed on a parse error — that is distinct from an
// absent spec.json, which is the legacy-fallback case, not an error).
// Violations (free-form ACs) are reported in the Report, not the error.
func Validate(releaseDir string) (*Report, error) {
	report := &Report{Dist: Distribution{}}

	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, fmt.Errorf("ears: read release dir %s: %w", releaseDir, err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sliceID := e.Name()
		if !isSliceID(sliceID) {
			continue
		}
		sliceDir := filepath.Join(releaseDir, sliceID)

		var results []Result
		rec, recErr := spec.ReadRecord(sliceDir)
		if recErr != nil {
			// spec.ReadRecord fails closed on a malformed spec.json — a real
			// error, distinct from "spec.json absent" (nil, nil), which is
			// the legacy-fallback case below. Do not conflate the two.
			return nil, fmt.Errorf("ears: %w", recErr)
		}
		if rec != nil && len(rec.AcceptanceCriteria) > 0 {
			// spec.json wins whenever it exists and has ACs, even if
			// spec.md also exists (AC-04 — JSON authoritative on
			// disagreement).
			results = classifySpecJSON(sliceID, rec)
		} else {
			specPath := filepath.Join(sliceDir, "spec.md")
			specText, err := os.ReadFile(specPath)
			if err != nil {
				return nil, fmt.Errorf("ears: read %s: %w", specPath, err)
			}
			results = classifySpec(sliceID, string(specText))
		}
		for _, r := range results {
			report.Results = append(report.Results, r)
			if r.Pattern == PatternNote {
				report.TotalNotes++
				continue
			}
			report.TotalACs++
			report.Dist[r.Pattern]++
			if r.Pattern == PatternNone {
				report.Violations = append(report.Violations, Violation{
					SliceID: r.SliceID,
					Line:    r.Line,
					Text:    r.Text,
				})
			}
		}
	}

	// Sort results for deterministic output.
	sort.Slice(report.Results, func(i, j int) bool {
		if report.Results[i].SliceID != report.Results[j].SliceID {
			return report.Results[i].SliceID < report.Results[j].SliceID
		}
		return report.Results[i].Line < report.Results[j].Line
	})
	sort.Slice(report.Violations, func(i, j int) bool {
		if report.Violations[i].SliceID != report.Violations[j].SliceID {
			return report.Violations[i].SliceID < report.Violations[j].SliceID
		}
		return report.Violations[i].Line < report.Violations[j].Line
	})

	return report, nil
}

// classifySpec parses a spec.md text and classifies each acceptance check
// in the "Acceptance checks" section. Returns one Result per AC. An AC may
// span multiple lines: a checkbox line followed by indented continuation
// lines. The continuation lines are joined into the AC text before
// classification.
func classifySpec(sliceID, text string) []Result {
	var results []Result
	inSection := false
	lineNum := 0
	var curAC *Result
	var curLines []string

	flush := func() {
		if curAC == nil {
			return
		}
		curAC.Text = strings.TrimSpace(strings.Join(curLines, " "))
		curAC.Pattern = Classify(curAC.Text)
		results = append(results, *curAC)
		curAC = nil
		curLines = nil
	}

	for _, line := range strings.Split(text, "\n") {
		lineNum++
		// Detect section boundaries.
		if strings.HasPrefix(line, "## ") {
			flush()
			inSection = strings.Contains(strings.ToLower(line), "acceptance check")
			continue
		}
		if !inSection {
			continue
		}
		// A new checkbox line starts a new AC.
		if m := reACLine.FindStringSubmatch(line); m != nil {
			flush()
			curAC = &Result{SliceID: sliceID, Line: lineNum}
			curLines = []string{strings.TrimSpace(m[1])}
			continue
		}
		// A continuation line: indented (starts with whitespace) and not a
		// heading or checkbox. Join into the current AC.
		if curAC != nil && line != "" && (line[0] == ' ' || line[0] == '\t') {
			curLines = append(curLines, strings.TrimSpace(line))
			continue
		}
		// A blank line or a non-indented non-checkbox line ends the current AC.
		flush()
	}
	flush()
	return results
}

// classifySpecJSON classifies each acceptance criterion in a spec-v1 record.
// Unlike classifySpec, it does not re-derive the pattern from the AC text via
// Classify — it reads the already-computed ears_pattern field (the canonical
// v0.10.0 EARS pattern class the planner recorded, and that S12 migrated from
// the retired sworn-local type field) and maps it directly. Returns one Result
// per AC, in acceptance_criteria[] order; Line is the 1-based ordinal position
// (see Result's doc comment — there is no markdown line for a JSON record).
func classifySpecJSON(sliceID string, rec *spec.Record) []Result {
	results := make([]Result, 0, len(rec.AcceptanceCriteria))
	for i, ac := range rec.AcceptanceCriteria {
		text := strings.TrimSpace(ac.Text)
		pattern := patternFromEARSPattern(ac.EARSPattern)
		// Defensive NOTE: check — belt-and-braces. The current spec.json
		// writer already filters NOTE: lines out at write time, but this
		// matches the spec.md path's behaviour if a future writer changes.
		if reNote.MatchString(text) {
			pattern = PatternNote
		}
		results = append(results, Result{
			SliceID: sliceID,
			Line:    i + 1,
			Text:    text,
			Pattern: pattern,
		})
	}
	return results
}

// patternFromEARSPattern maps a spec-v1 acceptance-criterion's stored
// ears_pattern field (the canonical v0.10.0 EARS pattern class) to an
// ears.Pattern. The stored values are identical to the Pattern constants, so
// the mapping is a direct, case-insensitive match. An empty or unrecognised
// value defaults to PatternUbiquitous, mirroring the writer-side
// default-to-ubiquitous fallback (a plain "THE SYSTEM SHALL" AC carries no
// precondition). "none" is deliberately absent from the accepted set — the
// strict spec-v1 schema forbids it as an authored value, so an unknown value
// collapses to Ubiquitous here rather than surfacing as a spurious violation.
func patternFromEARSPattern(pat string) Pattern {
	p := Pattern(strings.ToLower(strings.TrimSpace(pat)))
	switch p {
	case PatternUbiquitous, PatternEventDriven, PatternStateDriven,
		PatternOptionalFeature, PatternUnwanted, PatternComplex, PatternNote:
		return p
	default:
		return PatternUbiquitous
	}
}

// Print renders the validation report as human-readable text.
func Print(r *Report) string {
	var b strings.Builder

	b.WriteString(style.Heading("EARS Acceptance-Criteria Validation") + "\n")
	b.WriteString(style.Dim(strings.Repeat("=", 60)) + "\n\n")

	// Per-pattern distribution.
	b.WriteString(style.Dim("Pattern distribution") + "\n")
	b.WriteString(style.Dim(strings.Repeat("-", 60)) + "\n")
	// Print patterns in a fixed order for deterministic output.
	order := []Pattern{
		PatternUbiquitous,
		PatternEventDriven,
		PatternStateDriven,
		PatternOptionalFeature,
		PatternUnwanted,
		PatternComplex,
		PatternNote,
		PatternNone,
	}
	for _, p := range order {
		count := r.Dist[p]
		// PatternNone and PatternNote are tracked separately; only print
		// them if they have counts (None is printed in violations, Note is
		// informational).
		if p == PatternNone || p == PatternNote {
			continue
		}
		b.WriteString(fmt.Sprintf("  %s %d\n", style.Accent(fmt.Sprintf("%-20s", string(p))), count))
	}
	if r.TotalNotes > 0 {
		b.WriteString(fmt.Sprintf("  %-20s %d (excluded from validation)\n", PatternNote, r.TotalNotes))
	}
	b.WriteString(style.Dim(fmt.Sprintf("  %-20s %d\n", "total", r.TotalACs)))
	b.WriteString("\n")

	// Per-slice breakdown.
	b.WriteString(style.Dim("Per-slice breakdown") + "\n")
	b.WriteString(style.Dim(strings.Repeat("-", 60)) + "\n")
	sliceACs := map[string]int{}
	slicePatterns := map[string]map[Pattern]int{}
	for _, res := range r.Results {
		if res.Pattern == PatternNote {
			continue
		}
		sliceACs[res.SliceID]++
		if slicePatterns[res.SliceID] == nil {
			slicePatterns[res.SliceID] = map[Pattern]int{}
		}
		slicePatterns[res.SliceID][res.Pattern]++
	}
	sliceIDs := make([]string, 0, len(sliceACs))
	for id := range sliceACs {
		sliceIDs = append(sliceIDs, id)
	}
	sort.Strings(sliceIDs)
	for _, id := range sliceIDs {
		b.WriteString(fmt.Sprintf("  %s: %d ACs\n", id, sliceACs[id]))
		for _, p := range order {
			if p == PatternNone || p == PatternNote {
				continue
			}
			if c := slicePatterns[id][p]; c > 0 {
				b.WriteString(fmt.Sprintf("    %-18s %d\n", p, c))
			}
		}
	}
	b.WriteString("\n")

	// Violations.
	if len(r.Violations) > 0 {
		b.WriteString(style.Danger(fmt.Sprintf("Violations (%d free-form ACs)", len(r.Violations))) + "\n")
		b.WriteString(strings.Repeat("-", 60) + "\n")
		for _, v := range r.Violations {
			b.WriteString(fmt.Sprintf("  %s\n", v.String()))
		}
	} else {
		b.WriteString("Violations: none\n")
	}

	return b.String()
}

// --- helpers ---

var reSliceID = regexp.MustCompile(`^S\d{2}-[a-z0-9-]+$`)

// isSliceID returns true if s matches the slice-id pattern S<NN>-<kebab-name>.
func isSliceID(s string) bool {
	return reSliceID.MatchString(s)
}

// truncate shortens s to at most n characters, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
