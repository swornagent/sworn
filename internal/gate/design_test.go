package gate

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- colour regex tests ---

func TestHexColorRe(t *testing.T) {
	tests := []struct {
		line  string
		match bool
		val   string
	}{
		{"color: #ff0000;", true, "#ff0000"},
		{"background: #abc;", true, "#abc"},
		{"border: #AABBCC;", true, "#AABBCC"},
		{"fill: #ffffff80;", true, "#ffffff80"},
		{"color: var(--primary);", false, ""},
		{"no colour here", false, ""},
		{"rgb(255, 0, 128)", false, ""}, // hex only
	}
	for _, tt := range tests {
		matches := hexColorRe.FindAllStringSubmatch(tt.line, -1)
		if tt.match {
			if len(matches) == 0 {
				t.Errorf("hexColorRe: expected match in %q", tt.line)
			} else {
				val := "#" + matches[0][1]
				if val != tt.val {
					t.Errorf("hexColorRe: %q → %q, want %q", tt.line, val, tt.val)
				}
			}
		} else {
			if len(matches) > 0 {
				t.Errorf("hexColorRe: unexpected match in %q → %q", tt.line, "#"+matches[0][1])
			}
		}
	}
}

func TestRgbColorRe(t *testing.T) {
	tests := []struct {
		line  string
		match bool
	}{
		{"color: rgb(255, 0, 128);", true},
		{"background: rgba(0,0,0,0.5);", true},
		{"border: rgb(255 128 0);", true},
		{"color: var(--primary);", false},
	}
	for _, tt := range tests {
		matches := rgbColorRe.FindAllString(tt.line, -1)
		hasMatch := len(matches) > 0
		if hasMatch != tt.match {
			t.Errorf("rgbColorRe: %q → match=%v, want %v", tt.line, hasMatch, tt.match)
		}
	}
}

func TestHslColorRe(t *testing.T) {
	tests := []struct {
		line  string
		match bool
	}{
		{"color: hsl(240, 100%, 50%);", true},
		{"background: hsla(120, 50%, 40%, 0.8);", true},
		{"color: var(--primary);", false},
	}
	for _, tt := range tests {
		matches := hslColorRe.FindAllString(tt.line, -1)
		hasMatch := len(matches) > 0
		if hasMatch != tt.match {
			t.Errorf("hslColorRe: %q → match=%v, want %v", tt.line, hasMatch, tt.match)
		}
	}
}

// --- isUIFile tests ---

func TestIsUIFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"src/Button.tsx", true},
		{"src/Button.ts", true},
		{"src/Button.jsx", true},
		{"src/styles.css", true},
		{"src/styles.scss", true},
		{"src/styles.less", true},
		{"src/App.vue", true},
		{"src/App.svelte", true},
		{"src/index.html", true},
		{"src/icon.svg", true},
		{"src/main.go", false},
		{"src/main.py", false},
		{"src/main.rs", false},
		{"README.md", false},
	}
	for _, tt := range tests {
		got := isUIFile(tt.path)
		if got != tt.want {
			t.Errorf("isUIFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// --- design-fidelity config tests ---

func TestLoadDesignFidelity(t *testing.T) {
	dir := fixture(t, map[string]string{
		"design-fidelity.json": `{
			"$schema": "https://baton.sawy3r.net/schemas/design-fidelity-v1.json",
			"ui_bearing": true,
			"design_system": {
				"token_source": "tokens.json",
				"component_library": "packages/ui"
			},
			"tokens": [
				{"name": "primary", "value": "#2563eb"},
				{"name": "secondary", "value": "#7c3aed"}
			]
		}`,
	})

	cfg := loadDesignFidelity(dir + "/design-fidelity.json")
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if !cfg.UIBearing {
		t.Error("expected ui_bearing: true")
	}
	if cfg.DesignSystem == nil {
		t.Fatal("expected design_system")
	}
	if cfg.DesignSystem.TokenSource != "tokens.json" {
		t.Errorf("expected token_source 'tokens.json', got %q", cfg.DesignSystem.TokenSource)
	}
	if len(cfg.Tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(cfg.Tokens))
	}
}

func TestLoadDesignFidelity_Missing(t *testing.T) {
	cfg := loadDesignFidelity("/nonexistent/design-fidelity.json")
	if cfg != nil {
		t.Error("expected nil for missing file")
	}
}

func TestDeclaredColorTokens(t *testing.T) {
	cfg := &DesignFidelityConfig{
		Tokens: []DesignToken{
			{Name: "primary", Value: "#2563eb"},
			{Name: "secondary", Value: "#7C3AED"},
			{Name: "no-value", Value: ""},
		},
	}
	tokens := declaredColorTokens(cfg)
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens (excluding empty value), got %d", len(tokens))
	}
	if !tokens["#2563eb"] {
		t.Error("expected #2563eb to be declared")
	}
	if !tokens["#7c3aed"] {
		t.Error("expected #7c3aed (lowercased) to be declared")
	}
	if tokens["#000000"] {
		t.Error("#000000 should not be declared")
	}
}

// --- design report tests ---

func TestDesignReport_HasViolations(t *testing.T) {
	r := &DesignReport{TotalViolations: 0}
	if r.HasViolations() {
		t.Error("empty report should not have violations")
	}
	r.TotalViolations = 1
	if !r.HasViolations() {
		t.Error("report with violations should report violations")
	}
}

func TestPrintDesign_Exempt(t *testing.T) {
	r := &DesignReport{
		Release: "test-release",
		Slice:   "S01-test",
		Exempt:  true,
		Verdict: "PASS",
	}
	out := PrintDesign(r)
	if !strings.Contains(out, "EXEMPT") {
		t.Error("expected EXEMPT in output")
	}
}

func TestPrintDesign_Pass(t *testing.T) {
	r := &DesignReport{
		Release: "test-release",
		Slice:   "S01-test",
		Verdict: "PASS",
		ArchRules: &ArchRulesReport{
			Release: "test-release",
			Slice:   "S01-test",
			Rules:   0,
			Verdict: "PASS",
		},
	}
	out := PrintDesign(r)
	if !strings.Contains(out, "PASS") {
		t.Error("expected PASS in output")
	}
}

func TestPrintDesign_Fail(t *testing.T) {
	r := &DesignReport{
		Release:         "test-release",
		Slice:           "S01-test",
		Verdict:         "FAIL",
		TotalViolations: 2,
		ColorViolations: []ColorViolation{
			{File: "src/Button.tsx", Line: 10, Kind: "hex", Value: "#ff0000"},
			{File: "src/Card.tsx", Line: 5, Kind: "rgb", Value: "rgb(0,0,0)"},
		},
		ArchRules: &ArchRulesReport{
			Release: "test-release",
			Slice:   "S01-test",
			Rules:   0,
			Verdict: "PASS",
		},
	}
	out := PrintDesign(r)
	if !strings.Contains(out, "FAIL") {
		t.Error("expected FAIL in output")
	}
	if !strings.Contains(out, "#ff0000") {
		t.Error("expected #ff0000 in output")
	}
}

func TestJSONDesign(t *testing.T) {
	r := &DesignReport{
		Release: "test-release",
		Slice:   "S01-test",
		Verdict: "PASS",
	}
	out := JSONDesign(r)
	var parsed DesignReport
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON output not valid: %v", err)
	}
	if parsed.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", parsed.Verdict)
	}
}

func TestColorViolation_String(t *testing.T) {
	v := ColorViolation{File: "src/Button.tsx", Line: 10, Kind: "hex", Value: "#ff0000"}
	s := v.String()
	if !strings.Contains(s, "src/Button.tsx:10") {
		t.Errorf("expected file:line in string, got %q", s)
	}
	if !strings.Contains(s, "[hex]") {
		t.Errorf("expected kind in string, got %q", s)
	}
	if !strings.Contains(s, "#ff0000") {
		t.Errorf("expected value in string, got %q", s)
	}
}

// --- findRepoRoot tests ---

func TestFindRepoRoot(t *testing.T) {
	// Create a mock repo structure.
	dir := fixture(t, map[string]string{
		".git/HEAD":                        "ref: refs/heads/main",
		"docs/release/r1/S01-test/spec.md": "test",
	})

	root, err := findRepoRoot(dir + "/docs/release/r1")
	if err != nil {
		t.Fatal(err)
	}
	if root != dir {
		t.Errorf("expected root %q, got %q", dir, root)
	}
}

func TestFindRepoRoot_FindsWorktree(t *testing.T) {
	dir := fixture(t, map[string]string{
		"docs/release/r1/spec.md": "test",
	})

	// findRepoRoot walks up from the given dir until it finds .git.
	// In the test environment, the actual worktree's .git will be found eventually.
	root, err := findRepoRoot(dir + "/docs/release/r1")
	if err != nil {
		t.Fatal(err)
	}
	if root == "" {
		t.Error("expected non-empty root")
	}
}
