package baton

import (
	"fmt"
	"strings"
	"testing"
)

func TestTransformStripsScriptRefs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "release-verify.sh → sworn verify",
			in:   "Run `scripts/release-verify.sh <slice-id>` from a terminal.",
			want: "Run `sworn verify <slice-id>` from a terminal.",
		},
		{
			name: "release-board-status.sh → sworn board",
			in:   "The board is rendered by `release-board-status.sh --json`.",
			want: "The board is rendered by `sworn board --json`.",
		},
		{
			name: "design-audit.sh → sworn designaudit",
			in:   "`bin/design-audit.sh <project-dir>` wraps `sworn designaudit`.",
			want: "`sworn designaudit <project-dir>` wraps `sworn designaudit`.",
		},
		{
			name: "port-deriver.sh → native port derivation", in: "These paths are consumed by `port-deriver.sh`.",
			want: "These paths are consumed by `native port derivation`.",
		},
		{
			name: "captain-memory-search.py → sworn memory search",
			in:   "Search is performed by `captain-memory-search.py`.",
			want: "Search is performed by `sworn memory search`.",
		},
		{
			name: "multiple replacements in one string",
			in:   "First run `release-verify.sh`, then check `release-board-status.sh`.",
			want: "First run `sworn verify`, then check `sworn board`.",
		},
		{
			name: "no script refs",
			in:   "sworn verify is the native command. No scripts here.",
			want: "sworn verify is the native command. No scripts here.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transform(tt.in)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Transform() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTransformAppliesToRulesAndPrompts(t *testing.T) {
	// The same transform must apply identically to rule content and role-prompt
	// content. Prove it with one fixture from each category.
	ruleFixture := `## Verification

Before claiming any slice as verified, run ` + "`release-verify.sh`" + ` from a fresh
terminal. The script does deterministic first-pass checks. If the script fails,
the slice never reaches the verifier.

The board is surfaced by ` + "`release-board-status.sh --json`" + ` for machine-readable
state.`

	promptFixture := `As the implementer, you must run $HOME/.claude/bin/release-verify.sh
before marking the slice implemented. Use $HOME/.claude/bin/release-board-status.sh
to read the board state from the track worktree.`

	ruleGot, err := Transform(ruleFixture)
	if err != nil {
		t.Fatalf("Transform(rule) error = %v", err)
	}
	promptGot, err := Transform(promptFixture)
	if err != nil {
		t.Fatalf("Transform(prompt) error = %v", err)
	}

	// Both should have had their script refs replaced.
	if strings.Contains(ruleGot, "release-verify.sh") {
		t.Error("rule fixture still contains release-verify.sh after Transform")
	}
	if strings.Contains(ruleGot, "release-board-status.sh") {
		t.Error("rule fixture still contains release-board-status.sh after Transform")
	}
	if strings.Contains(promptGot, "release-verify.sh") {
		t.Error("prompt fixture still contains release-verify.sh after Transform")
	}
	if strings.Contains(promptGot, "release-board-status.sh") {
		t.Error("prompt fixture still contains release-board-status.sh after Transform")
	}
	if !strings.Contains(ruleGot, "sworn verify") {
		t.Error("rule fixture missing 'sworn verify' after Transform")
	}
	if !strings.Contains(promptGot, "sworn verify") {
		t.Error("prompt fixture missing 'sworn verify' after Transform")
	}
}

func TestTransformFailsClosedOnUnmappedScript(t *testing.T) {
	// A fixture containing a known Baton script token that survives (because it
	// is NOT in the substitution map) must cause Transform to return an error.
	// This proves the fail-closed guard works. We test with the guard's regex
	// check by adding a script-like reference not in the map.
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{
			name:    "unknown .sh script",
			in:      "Run `some-new-tool.sh` to process things.",
			wantErr: true,
		},
		{
			name:    "unknown .py script",
			in:      "Use `captain-new-search.py` for searching.",
			wantErr: true,
		},
		{
			name:    "unknown .mjs script",
			in:      "Run `new-ui.mjs` to render the board.",
			wantErr: true,
		},
		{
			name:    "all known refs are fine",
			in:      "Use `sworn verify` and `sworn board` for everything.",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transform(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("Transform() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestTransformScriptReferenceLexicalBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantToken string
	}{
		{name: "prose member", input: "board.json.shared_touchpoints"},
		{name: "suffix inside prose", input: "example.pyish"},
		{name: "bare shell", input: "run example.sh", wantToken: "example.sh"},
		{name: "relative python", input: "run ./tools/example.py now", wantToken: "./tools/example.py"},
		{name: "scripts module", input: "`scripts/example.mjs`", wantToken: "scripts/example.mjs"},
		{name: "markdown link target", input: "[run](scripts/example.sh)", wantToken: "scripts/example.sh"},
		{name: "punctuation adjacent", input: "then example.py, next", wantToken: "example.py"},
		{name: "absolute looking", input: "/opt/tools/example.mjs", wantToken: "/opt/tools/example.mjs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transform(tt.input)
			if tt.wantToken == "" {
				if err != nil {
					t.Fatalf("Transform(%q) error = %v", tt.input, err)
				}
				if got != tt.input {
					t.Fatalf("Transform(%q) = %q, want unchanged", tt.input, got)
				}
				return
			}
			if err == nil {
				t.Fatalf("Transform(%q) error = nil, want exact token %q", tt.input, tt.wantToken)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", tt.wantToken)) {
				t.Fatalf("Transform(%q) error = %v, want exact token %q", tt.input, err, tt.wantToken)
			}
		})
	}
}

// TestTransformIdempotent proves that running Transform twice on the same
// content produces the same output, which is a prerequisite for Vendor
// idempotence.
func TestTransformIdempotent(t *testing.T) {
	in := "Run `release-verify.sh` and check `release-board-status.sh`."
	first, err := Transform(in)
	if err != nil {
		t.Fatalf("first Transform() error = %v", err)
	}
	second, err := Transform(first)
	if err != nil {
		t.Fatalf("second Transform() error = %v", err)
	}
	if first != second {
		t.Errorf("Transform not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
}

// TestReplacementsAndGuardDerivedFromSameTable proves that every entry in the
// replacements table appears in the guard's derivation — the single-table
// derive-both pattern (Design Decision §2.1).
func TestReplacementsAndGuardDerivedFromSameTable(t *testing.T) {
	// The guard is derived from the same replacements slice. Verify that
	// every replacement.Old is a string we would catch.
	for _, r := range replacements {
		// Inject the token into a plain sentence and verify Transform
		// removes it (i.e., the replacement works).
		in := "Use `" + r.token + "` for this step."
		out, err := Transform(in)
		if err != nil {
			t.Errorf("Transform(%q) unexpected error: %v", r.token, err)
			continue
		}
		if strings.Contains(out, r.token) {
			t.Errorf("Transform(%q): token %q still present in output: %q", r.token, r.token, out)
		}
		if !strings.Contains(out, r.new) {
			t.Errorf("Transform(%q): replacement %q not found in output: %q", r.token, r.new, out)
		}
	}
}
