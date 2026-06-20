// Package designaudit implements the design-system conformance audit (Rule 9,
// S09). It runs a deterministic first-pass against the declared design system
// (S08) — flagging hardcoded hex colours, off-scale spacing/border values, and
// recreated components — and enforces a recorded human cohesion verdict before
// allowing exit 0.
//
// Stdlib only — zero runtime dependencies.
package designaudit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/swornagent/sworn/internal/config"
)

// ViolationKind classifies the type of design drift found.
type ViolationKind string

const (
	// HardcodedColor flags a colour literal that bypasses the token system.
	HardcodedColor ViolationKind = "hardcoded-color"
	// OffScaleSpacing flags a spacing or border-width value not on the token scale.
	OffScaleSpacing ViolationKind = "off-scale-spacing"
	// RecreatedComponent flags a component definition that duplicates a library entry.
	RecreatedComponent ViolationKind = "recreated-component"
)

// AllowComment is the inline annotation that suppresses a violation on a single
// line. Append it to the line in question to declare a deliberate, sanctioned
// deviation: `color: #ff0000; /* sworn-design-allow */`.
const AllowComment = "sworn-design-allow"

// Violation records one design drift finding with its source location.
type Violation struct {
	File    string
	Line    int
	Kind    ViolationKind
	Value   string
	Message string
}

// String formats the violation for human-readable output.
func (v Violation) String() string {
	return fmt.Sprintf("%s:%d: [%s] %s", v.File, v.Line, v.Kind, v.Message)
}

// Report holds all findings from a design conformance audit run.
type Report struct {
	ProjectDir      string
	Violations      []Violation
	CohesionVerdict string // human-set; "" means not yet recorded
	Exempt          bool   // true when project is not ui_bearing
}

// HasViolations returns true when at least one machine-detectable drift was found.
func (r *Report) HasViolations() bool {
	return len(r.Violations) > 0
}

// NeedsCohesionVerdict returns true when the deterministic pass is clean but the
// human cohesion verdict has not been recorded. The audit cannot exit 0 until
// both conditions are satisfied.
func (r *Report) NeedsCohesionVerdict() bool {
	return !r.Exempt && !r.HasViolations() && r.CohesionVerdict == ""
}

// Passed returns true when the audit is fully complete: no machine violations
// and a human cohesion verdict is recorded.
func (r *Report) Passed() bool {
	return !r.Exempt && !r.HasViolations() && r.CohesionVerdict != ""
}

// Run performs the design conformance audit on projectDir using the config cfg.
// cohesionVerdict is the human-supplied judgement (e.g. "on-brand" or "off-brand"),
// or "" if not yet provided. Non-UI-bearing projects are exempt and return immediately.
func Run(projectDir string, cfg config.Config, cohesionVerdict string) (*Report, error) {
	report := &Report{
		ProjectDir:      projectDir,
		CohesionVerdict: cohesionVerdict,
	}

	if !cfg.UIBearing {
		report.Exempt = true
		return report, nil
	}

	// Fail closed: a UI-bearing project must have a declared design system.
	if cfg.DesignSystem == nil {
		return nil, fmt.Errorf("designaudit: ui_bearing project has no design_system declared — run 'sworn init' to configure")
	}

	if err := scanProject(projectDir, cfg.DesignSystem, report); err != nil {
		return nil, fmt.Errorf("designaudit: scanning %s: %w", projectDir, err)
	}

	return report, nil
}

// Print formats a Report for human-readable display on stdout.
func Print(r *Report) string {
	var b strings.Builder

	if r.Exempt {
		fmt.Fprintln(&b, "DESIGNAUDIT EXEMPT — project is not ui_bearing; design conformance does not apply.")
		return b.String()
	}

	fmt.Fprintf(&b, "Design conformance audit: %s\n\n", r.ProjectDir)

	if r.HasViolations() {
		fmt.Fprintf(&b, "%d violation(s) found:\n\n", len(r.Violations))
		for i, v := range r.Violations {
			fmt.Fprintf(&b, "%d. %s\n", i+1, v.String())
		}
		fmt.Fprintln(&b, "\nFix each violation or add `/* sworn-design-allow */` for sanctioned exceptions.")
		return b.String()
	}

	fmt.Fprintln(&b, "Deterministic checks: PASS — no machine-detectable drift.")

	if r.CohesionVerdict == "" {
		fmt.Fprintln(&b, "\nHuman cohesion verdict: REQUIRED — run with --cohesion=on-brand|off-brand")
		fmt.Fprintln(&b, "The cohesion judgement (\"does it feel on-brand\") must be human-set.")
	} else {
		fmt.Fprintf(&b, "Human cohesion verdict: %s\n", r.CohesionVerdict)
		fmt.Fprintln(&b, "\nAUDIT PASS")
	}
	return b.String()
}

// PrintCompact returns a single-line summary suitable for stderr / CI parsing.
func PrintCompact(r *Report) string {
	if r.Exempt {
		return "DESIGNAUDIT EXEMPT — not ui_bearing"
	}
	if r.HasViolations() {
		return fmt.Sprintf("DESIGNAUDIT FAIL — %d violation(s)", len(r.Violations))
	}
	if r.NeedsCohesionVerdict() {
		return "DESIGNAUDIT BLOCKED — deterministic pass clean; human cohesion verdict required (--cohesion=<verdict>)"
	}
	return fmt.Sprintf("DESIGNAUDIT PASS — cohesion: %s", r.CohesionVerdict)
}

// ---- source scanners --------------------------------------------------------

// sourceExts is the set of file extensions we scan for design drift.
var sourceExts = map[string]bool{
	".css": true, ".scss": true, ".sass": true, ".less": true,
	".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".vue": true, ".svelte": true,
}

// skipDirs is the set of directory names we never descend into.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	".next": true, ".nuxt": true, "out": true, "coverage": true,
}

func isSourceFile(ext string) bool {
	return sourceExts[strings.ToLower(ext)]
}

func scanProject(projectDir string, ds *config.DesignSystem, report *Report) error {
	// Collect component names from the declared component library.
	libComponents := collectLibraryComponents(filepath.Join(projectDir, ds.ComponentLibrary))

	return filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFile(filepath.Ext(path)) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		content := string(data)
		checkHardcodedColors(path, content, report)
		checkOffScaleSpacing(path, content, report)
		checkRecreatedComponents(path, content, projectDir, ds.ComponentLibrary, libComponents, report)
		return nil
	})
}

// collectLibraryComponents returns a set of component names declared in the
// component library directory. A component name is the filename stem of any
// source file in the library (e.g. "Button" from "Button.tsx").
func collectLibraryComponents(libraryDir string) map[string]bool {
	components := make(map[string]bool)
	entries, err := os.ReadDir(libraryDir)
	if err != nil {
		// If the library dir doesn't exist or can't be read, we can't detect
		// recreations — return empty map (no false positives).
		return components
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if !isSourceFile(ext) {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ext)
		// Only track PascalCase names — they are component conventions.
		if len(stem) > 0 && stem[0] >= 'A' && stem[0] <= 'Z' {
			components[stem] = true
		}
	}
	return components
}

// hexColorRe matches hardcoded hex colour values in CSS property assignments.
// Matches patterns like: `color: #ff0000`, `background: #abc`, `border-color: #AABBCC`.
var hexColorRe = regexp.MustCompile(`(?i)(?:^|;|\{)\s*[\w-]+\s*:\s*(#[0-9a-fA-F]{3,8})\b`)

func checkHardcodedColors(file, content string, report *Report) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, AllowComment) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		// Skip comment-only lines.
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		matches := hexColorRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			val := m[1]
			report.Violations = append(report.Violations, Violation{
				File:    file,
				Line:    i + 1,
				Kind:    HardcodedColor,
				Value:   val,
				Message: fmt.Sprintf("hardcoded colour %s — use a design token (e.g. var(--color-...))", val),
			})
		}
	}
}

// spacingPropRe matches CSS spacing/border properties with hardcoded px/rem values
// that are NOT CSS variable references (var(--...)).
var spacingPropRe = regexp.MustCompile(`(?i)(?:^|;|\{)\s*(?:margin|padding|gap|border-width|border-radius|border-top-width|border-right-width|border-bottom-width|border-left-width)\s*:\s*([^;{}\n]+)`)

// hardcodedDimRe matches a plain numeric px or rem value (not inside var(...)).
var hardcodedDimRe = regexp.MustCompile(`\b(\d+(?:\.\d+)?(?:px|rem))\b`)

func checkOffScaleSpacing(file, content string, report *Report) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, AllowComment) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		props := spacingPropRe.FindAllStringSubmatch(line, -1)
		for _, prop := range props {
			val := prop[1]
			// If the value contains var(--...) it's using a token — OK.
			if strings.Contains(val, "var(") {
				continue
			}
			// Check for hardcoded px or rem values.
			dims := hardcodedDimRe.FindAllString(val, -1)
			for _, dim := range dims {
				// 0px is a sentinel value universally allowed.
				if dim == "0px" || dim == "0rem" {
					continue
				}
				report.Violations = append(report.Violations, Violation{
					File:    file,
					Line:    i + 1,
					Kind:    OffScaleSpacing,
					Value:   dim,
					Message: fmt.Sprintf("hardcoded spacing %s — use a design token (e.g. var(--spacing-...))", dim),
				})
			}
		}
	}
}

// componentDefRe matches React function component or class component definitions.
// Captures PascalCase names: `function Button(`, `const Button =`, `class Button `.
var componentDefRe = regexp.MustCompile(`(?:^|\s)(?:function|const|class)\s+([A-Z][a-zA-Z0-9]*)\s*[=({\s]`)

func checkRecreatedComponents(file, content, projectDir, libraryPath string, libComponents map[string]bool, report *Report) {
	if len(libComponents) == 0 {
		return
	}

	// Don't check files inside the component library itself.
	absLibDir := filepath.Join(projectDir, libraryPath)
	absFile, err := filepath.Abs(file)
	if err == nil {
		if strings.HasPrefix(absFile, absLibDir+string(filepath.Separator)) || absFile == absLibDir {
			return
		}
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, AllowComment) {
			continue
		}
		matches := componentDefRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			name := m[1]
			if libComponents[name] {
				report.Violations = append(report.Violations, Violation{
					File:    file,
					Line:    i + 1,
					Kind:    RecreatedComponent,
					Value:   name,
					Message: fmt.Sprintf("component %q is defined here but already exists in the component library — reuse the library component instead", name),
				})
			}
		}
	}
}
