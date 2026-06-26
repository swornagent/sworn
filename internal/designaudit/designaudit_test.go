package designaudit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/config"
)

// uiBearingCfg returns a Config for a UI-bearing project with the given design system.
func uiBearingCfg(tokenSource, componentLibrary string) config.Config {
	return config.Config{
		Version:   1,
		UIBearing: true,
		DesignSystem: &config.DesignSystem{
			TokenSource:      tokenSource,
			ComponentLibrary: componentLibrary,
		},
	}
}

// writeFile creates a file with the given content under dir.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDesignAudit_HardcodedHex verifies AC1: hardcoded hex in UI source exits
// non-zero and names the file + line.
func TestDesignAudit_HardcodedHex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeFile(t, dir, "src/app/page.css", "body {\n  color: #ff0000;\n  background: var(--color-surface);\n}\n")

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !report.HasViolations() {
		t.Fatal("expected HardcodedColor violation, got none")
	}
	found := false
	for _, v := range report.Violations {
		if v.Kind == HardcodedColor && strings.Contains(v.Value, "#ff0000") {
			found = true
			if v.Line == 0 {
				t.Error("violation line should be non-zero")
			}
			if v.File == "" {
				t.Error("violation file should be non-empty")
			}
		}
	}
	if !found {
		t.Errorf("expected violation with Value=#ff0000, got %+v", report.Violations)
	}
}

// TestDesignAudit_OffScaleSpacing verifies AC2: hardcoded spacing value flags file+line.
func TestDesignAudit_OffScaleSpacing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/ui/Btn.tsx", "export function Btn() { return null; }\n")
	writeFile(t, dir, "src/styles/layout.css",
		"body {\n  margin: 17px;\n  padding: var(--spacing-4);\n}\n")

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !report.HasViolations() {
		t.Fatal("expected OffScaleSpacing violation, got none")
	}
	found := false
	for _, v := range report.Violations {
		if v.Kind == OffScaleSpacing && v.Value == "17px" {
			found = true
			if v.Line == 0 {
				t.Error("violation line should be non-zero")
			}
		}
	}
	if !found {
		t.Errorf("expected OffScaleSpacing violation for 17px, got %+v", report.Violations)
	}
}

// TestDesignAudit_RecreatedComponent verifies AC3: component duplicating a library
// entry is flagged.
func TestDesignAudit_RecreatedComponent(t *testing.T) {
	dir := t.TempDir()
	// Component library declares Button and Input.
	writeFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeFile(t, dir, "src/ui/Input.tsx", "export function Input() { return null; }\n")
	// App defines its own Button — recreation violation.
	writeFile(t, dir, "src/app/MyPage.tsx", "function Button() { return <button>click</button>; }\n")

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	found := false
	for _, v := range report.Violations {
		if v.Kind == RecreatedComponent && v.Value == "Button" {
			found = true
			if !strings.Contains(v.File, "MyPage.tsx") {
				t.Errorf("expected violation in MyPage.tsx, got file %s", v.File)
			}
		}
	}
	if !found {
		t.Errorf("expected RecreatedComponent violation for Button, got %+v", report.Violations)
	}
}

// TestDesignAudit_LibraryFilesNotFlagged verifies component definitions in the
// library directory itself are not flagged as recreations.
func TestDesignAudit_LibraryFilesNotFlagged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeFile(t, dir, "src/ui/Input.tsx", "export function Input() { return null; }\n")
	// No app-level recreations.

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, v := range report.Violations {
		if v.Kind == RecreatedComponent {
			t.Errorf("library file itself should not be flagged, got violation %s", v.String())
		}
	}
}

// TestDesignAudit_CleanSourceWithCohesionVerdict verifies AC4: clean source +
// human cohesion verdict → Passed() true and NeedsCohesionVerdict() false.
func TestDesignAudit_CleanSourceWithCohesionVerdict(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeFile(t, dir, "src/app/page.css",
		"body {\n  color: var(--color-primary);\n  margin: var(--spacing-4);\n}\n")

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "on-brand")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.HasViolations() {
		t.Errorf("expected no violations, got %+v", report.Violations)
	}
	if report.NeedsCohesionVerdict() {
		t.Error("expected NeedsCohesionVerdict() false when verdict is provided")
	}
	if !report.Passed() {
		t.Error("expected Passed() true for clean source + cohesion verdict")
	}
}

// TestDesignAudit_MissingCohesionVerdict verifies AC5: clean source without a
// human cohesion verdict does NOT pass — the system requires the verdict to be set.
func TestDesignAudit_MissingCohesionVerdict(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/ui/Button.tsx", "export function Button() { return null; }\n")
	writeFile(t, dir, "src/app/page.css",
		"body {\n  color: var(--color-primary);\n}\n")

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "") // no cohesion verdict
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.HasViolations() {
		t.Errorf("expected no machine violations, got %+v", report.Violations)
	}
	if !report.NeedsCohesionVerdict() {
		t.Error("expected NeedsCohesionVerdict() true when cohesion is empty")
	}
	if report.Passed() {
		t.Error("expected Passed() false when cohesion verdict is missing — the system must not auto-pass")
	}
}

// TestDesignAudit_AllowComment verifies that lines marked with the allow comment
// are not flagged, providing the sanctioned-exception mechanism.
func TestDesignAudit_AllowComment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/ui/Btn.tsx", "export function Btn() { return null; }\n")
	writeFile(t, dir, "src/app/legacy.css",
		"body { color: #ff0000; /* sworn-design-allow */ }\n")

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "on-brand")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, v := range report.Violations {
		if v.Kind == HardcodedColor {
			t.Errorf("allow-commented line should not produce violation, got %s", v.String())
		}
	}
}

// TestDesignAudit_NotUIBearing verifies that a CLI project (ui_bearing: false)
// is exempt from the audit.
func TestDesignAudit_NotUIBearing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	cfg := config.Config{Version: 1, UIBearing: false}
	report, err := Run(dir, cfg, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !report.Exempt {
		t.Error("expected report.Exempt for non-UI-bearing project")
	}
	if report.HasViolations() {
		t.Errorf("exempt project should have no violations, got %+v", report.Violations)
	}
	if report.Passed() {
		t.Error("exempt project Passed() should be false (exempt != passed)")
	}
}

// TestDesignAudit_NoDesignSystemFails verifies that a UI-bearing project without
// a design_system declaration fails closed (error, not silent pass).
func TestDesignAudit_NoDesignSystemFails(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{Version: 1, UIBearing: true, DesignSystem: nil}
	_, err := Run(dir, cfg, "")
	if err == nil {
		t.Fatal("expected error for ui_bearing without design_system, got nil")
	}
	if !strings.Contains(err.Error(), "design_system") {
		t.Errorf("expected error to mention design_system, got: %v", err)
	}
}

// TestDesignAudit_ZeroPxAllowed verifies that 0px is not flagged as off-scale
// (it's a universally valid sentinel value).
func TestDesignAudit_ZeroPxAllowed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/ui/Btn.tsx", "export function Btn() { return null; }\n")
	writeFile(t, dir, "src/app/reset.css", "* { margin: 0px; padding: 0px; }\n")

	cfg := uiBearingCfg("tokens.json", "src/ui")
	report, err := Run(dir, cfg, "on-brand")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, v := range report.Violations {
		if v.Kind == OffScaleSpacing {
			t.Errorf("0px should not be flagged, got violation %s", v.String())
		}
	}
}

// TestDesignAudit_Print covers Print and PrintCompact for all report states.
func TestDesignAudit_Print(t *testing.T) {
	tests := []struct {
		name          string
		report        *Report
		wantIn        string // substring expected in Print output
		wantCompactIn string
	}{
		{
			name:          "exempt",
			report:        &Report{Exempt: true},
			wantIn:        "EXEMPT",
			wantCompactIn: "EXEMPT",
		},
		{
			name: "violation",
			report: &Report{
				Violations: []Violation{{File: "a.css", Line: 1, Kind: HardcodedColor, Value: "#f00", Message: "bad"}},
			},
			wantIn:        "1 violation",
			wantCompactIn: "FAIL",
		},
		{
			name:          "needs cohesion",
			report:        &Report{},
			wantIn:        "REQUIRED",
			wantCompactIn: "BLOCKED",
		},
		{
			name:          "passed",
			report:        &Report{CohesionVerdict: "on-brand"},
			wantIn:        "PASS",
			wantCompactIn: "PASS",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := Print(tc.report)
			if !strings.Contains(out, tc.wantIn) {
				t.Errorf("Print: expected %q in output, got:\n%s", tc.wantIn, out)
			}
			compact := PrintCompact(tc.report)
			if !strings.Contains(compact, tc.wantCompactIn) {
				t.Errorf("PrintCompact: expected %q in output, got: %s", tc.wantCompactIn, compact)
			}
		})
	}
}
