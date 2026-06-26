// Package gate provides lint gates for the SwornAgent CLI.
//
// design.go implements `sworn lint design`: hardcoded colour detection plus
// architecture rule enforcement. It reads docs/baton/architecture.json for rule
// configuration, docs/baton/design-fidelity.json for design system config (token
// exemptions), and the per-slice design-allowlist.json for escape-hatch
// suppression.
//
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

// --- data model ---

// DesignReport holds the full structured result of RunDesign.
type DesignReport struct {
	Release         string           `json:"release"`
	Slice           string           `json:"slice"`
	ColorViolations []ColorViolation `json:"color_violations"`
	ArchRules       *ArchRulesReport `json:"arch_rules"`
	TotalViolations int              `json:"total_violations"`
	Verdict         string           `json:"verdict"`
	Exempt          bool             `json:"exempt,omitempty"`
}

// ColorViolation is a single hardcoded colour finding.
type ColorViolation struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Kind  string `json:"kind"` // "hex", "rgb", "hsl"
	Value string `json:"value"`
}

// String returns a human-readable violation line.
func (v ColorViolation) String() string {
	return fmt.Sprintf("%s:%d [%s] %s", v.File, v.Line, v.Kind, v.Value)
}

// HasViolations returns true when the report contains any violations.
func (r *DesignReport) HasViolations() bool {
	return r.TotalViolations > 0
}

// --- design-fidelity config ---

// DesignFidelityConfig is the optional design system config from docs/baton/design-fidelity.json.
type DesignFidelityConfig struct {
	Schema       string        `json:"$schema"`
	UIBearing    bool          `json:"ui_bearing"`
	DesignSystem *DesignSystem `json:"design_system,omitempty"`
	TokenSource  string        `json:"token_source,omitempty"`
	Tokens       []DesignToken `json:"tokens,omitempty"`
}

// DesignSystem holds the design system declaration.
type DesignSystem struct {
	TokenSource      string `json:"token_source"`
	ComponentLibrary string `json:"component_library"`
}

// DesignToken is a declared design token (colour).
type DesignToken struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// --- colour detection regex ---

var (
	// hexColorRe matches hex colour literals: #fff, #ffffff, #ffffffff, #FFF, #FFFFFF.
	hexColorRe = regexp.MustCompile(`#([0-9a-fA-F]{3,8})\b`)

	// rgbColorRe: rgb(255, 0, 128), rgb(255 0 128), rgba(255,0,128,0.5)
	rgbColorRe = regexp.MustCompile(`rgba?\(\s*\d+\s*[, ]\s*\d+\s*[, ]\s*\d+`)

	// hslColorRe: hsl(240, 100%, 50%), hsla(240, 100%, 50%, 0.5)
	hslColorRe = regexp.MustCompile(`hsla?\(\s*\d+`)

	// UI file extensions that may contain colours.
	uiExts = map[string]bool{
		".css": true, ".scss": true, ".sass": true, ".less": true,
		".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".vue": true, ".svelte": true, ".html": true, ".htm": true,
		".svg": true,
	}
)

func isUIFile(path string) bool {
	ext := filepath.Ext(path)
	return uiExts[strings.ToLower(ext)]
}

// --- main entry point ---

// RunDesign runs the design lint gate for a slice. It detects hardcoded colours
// in UI files from the diff, then runs architecture rules.
//
// Parameters:
//
//	releaseDir — absolute path to docs/release/<release-name>/
//	sliceID    — e.g. "S67-lint-design"
//	baseRef    — git ref for the diff base (start_commit or "release-wt/<release>")
//
// Returns an error only for I/O / git / config failures; violations are in the report.
func RunDesign(releaseDir, sliceID, baseRef string) (*DesignReport, error) {
	// Resolve project root.
	root, err := findRepoRoot(releaseDir)
	if err != nil {
		return nil, fmt.Errorf("design: %w", err)
	}

	releaseName := filepath.Base(releaseDir)
	sliceDir := filepath.Join(releaseDir, sliceID)

	report := &DesignReport{
		Release: releaseName,
		Slice:   sliceID,
	}

	// 1. Check if project is UI-bearing (design-fidelity.json).
	dfPath := filepath.Join(root, "docs", "baton", "design-fidelity.json")
	dfCfg := loadDesignFidelity(dfPath)

	// If project is not UI-bearing, the colour check is exempt.
	if dfCfg != nil && !dfCfg.UIBearing {
		report.Exempt = true
	}

	// 2. Load declared token values for exemption matching.
	declaredTokens := declaredColorTokens(dfCfg)

	// 3. Detect hardcoded colours in UI files from the diff.
	if !report.Exempt {
		colorViols, err := detectHardcodedColors(baseRef, declaredTokens, sliceDir)
		if err != nil {
			return nil, fmt.Errorf("design: colour detection: %w", err)
		}
		report.ColorViolations = colorViols
	}

	// 4. Run architecture rules (always — grep rules cover all projects).
	archReport, err := RunArchRules(releaseDir, sliceID, baseRef)
	if err != nil {
		return nil, fmt.Errorf("design: arch rules: %w", err)
	}
	report.ArchRules = archReport

	// 5. Compute totals.
	report.TotalViolations = len(report.ColorViolations) + archReport.Failed
	if report.TotalViolations == 0 {
		report.Verdict = "PASS"
	} else {
		report.Verdict = "FAIL"
	}

	return report, nil
}

// findRepoRoot walks up from releaseDir until it finds .git or go.mod.
func findRepoRoot(releaseDir string) (string, error) {
	dir := releaseDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find repo root from %s", releaseDir)
		}
		dir = parent
	}
}

// loadDesignFidelity reads and parses docs/baton/design-fidelity.json.
func loadDesignFidelity(path string) *DesignFidelityConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg DesignFidelityConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

// declaredColorTokens extracts known colour values from the design system config.
func declaredColorTokens(cfg *DesignFidelityConfig) map[string]bool {
	tokens := make(map[string]bool)
	if cfg == nil {
		return tokens
	}
	for _, t := range cfg.Tokens {
		if t.Value != "" {
			tokens[strings.ToLower(t.Value)] = true
		}
	}
	return tokens
}

// --- colour detection ---

// detectHardcodedColors scans UI files in the diff for hardcoded hex, rgb, hsl values.
// Colours matching declared design tokens are exempt.
// Files in the per-slice design-allowlist.json are exempt.
func detectHardcodedColors(baseRef string, declaredTokens map[string]bool, sliceDir string) ([]ColorViolation, error) {
	changedFiles, err := diffChangedFiles(baseRef)
	if err != nil {
		return nil, err
	}

	// Load allowlist for per-file exemptions.
	allowlistPath := filepath.Join(sliceDir, "design-allowlist.json")
	allowlist, _ := loadAllowlist(allowlistPath)
	if allowlist == nil {
		allowlist = &DesignAllowlist{}
	}

	var violations []ColorViolation
	for _, file := range changedFiles {
		if !isUIFile(file) {
			continue
		}
		if skipTestFile(file) {
			continue
		}
		if isExempt(allowlist, "hardcoded-color", file) {
			continue
		}

		added, err := diffAddedLines(baseRef, file)
		if err != nil {
			continue
		}

		for _, li := range added {
			// Check hex colours.
			hexMatches := hexColorRe.FindAllStringSubmatch(li.Text, -1)
			for _, m := range hexMatches {
				val := "#" + m[1]
				if declaredTokens[strings.ToLower(val)] {
					continue
				}
				violations = append(violations, ColorViolation{
					File:  file,
					Line:  li.Line,
					Kind:  "hex",
					Value: val,
				})
			}

			// Check rgb/rgba.
			rgbMatches := rgbColorRe.FindAllString(li.Text, -1)
			for _, val := range rgbMatches {
				violations = append(violations, ColorViolation{
					File:  file,
					Line:  li.Line,
					Kind:  "rgb",
					Value: val,
				})
			}

			// Check hsl/hsla.
			hslMatches := hslColorRe.FindAllString(li.Text, -1)
			for _, val := range hslMatches {
				violations = append(violations, ColorViolation{
					File:  file,
					Line:  li.Line,
					Kind:  "hsl",
					Value: val,
				})
			}
		}
	}
	return violations, nil
}

// --- human-readable output ---

// PrintDesign renders the DesignReport as human-readable text.
func PrintDesign(r *DesignReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(style.Bold(fmt.Sprintf("DESIGN LINT — %s / %s", r.Release, r.Slice)))
	b.WriteString("\n\n")

	if r.Exempt {
		b.WriteString(style.Warn("DESIGN EXEMPT — project is not ui_bearing; colour checks do not apply."))
		b.WriteString("\n")
	}

	// Colour violations.
	if !r.Exempt {
		b.WriteString(style.Dim(fmt.Sprintf("Colour violations: %d\n", len(r.ColorViolations))))
		for i, v := range r.ColorViolations {
			b.WriteString(style.Danger(fmt.Sprintf("  %d. [%s] %s — %s\n", i+1, v.Kind, v.Value, v.File+":"+fmt.Sprint(v.Line))))
		}
		if len(r.ColorViolations) == 0 {
			b.WriteString(style.Success("  No hardcoded colours detected.\n"))
		}
		b.WriteString("\n")
	}

	// Architecture rules.
	if r.ArchRules != nil {
		b.WriteString(PrintArchRules(r.ArchRules))
	}

	// Verdict.
	b.WriteString("\n")
	if r.Verdict == "PASS" {
		b.WriteString(style.Success("PASS — design lint clean\n"))
	} else {
		b.WriteString(style.Danger(fmt.Sprintf("FAIL — %d violation(s)\n", r.TotalViolations)))
	}
	b.WriteString("\n")

	return b.String()
}

// JSONDesign returns the report as pretty-printed JSON.
func JSONDesign(r *DesignReport) string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}
