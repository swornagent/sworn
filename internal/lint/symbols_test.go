package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupSymbolTest creates a temp fixture tree with:
//   - a Go file containing a real symbol (CalculateFIRE)
//   - a design.md referencing both the real symbol and a fake one
//
// Returns the slice directory and repo root.
func setupSymbolTest(t *testing.T, designContent string) (sliceDir, repoRoot string) {
	t.Helper()

	repoRoot = t.TempDir()
	sliceDir = filepath.Join(repoRoot, "docs", "release", "test-release", "S01-test")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatalf("mkdir slice dir: %v", err)
	}

	// Write a real Go file with a symbol.
	goFile := filepath.Join(repoRoot, "calculator.go")
	goContent := "package main\n\nfunc CalculateFIRE() int { return 0 }\n"
	if err := os.WriteFile(goFile, []byte(goContent), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}

	// Write design.md.
	if err := os.WriteFile(filepath.Join(sliceDir, "design.md"), []byte(designContent), 0o644); err != nil {
		t.Fatalf("write design.md: %v", err)
	}

	return sliceDir, repoRoot
}

func TestSymbolsUnresolvedWarns(t *testing.T) {
	design := "# Design\n\nCall `CalculateFIRE` and `NonExistentFunc` to compute.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	err := CheckSymbols(sliceDir, repoRoot)
	if err == nil {
		t.Fatal("expected error for unresolved symbol, got nil")
	}
	if !strings.Contains(err.Error(), "NonExistentFunc") {
		t.Fatalf("error should name NonExistentFunc, got: %v", err)
	}
	if strings.Contains(err.Error(), "CalculateFIRE") {
		t.Fatalf("error should NOT name CalculateFIRE (it resolves), got: %v", err)
	}
}

func TestSymbolsResolvedQuiet(t *testing.T) {
	design := "# Design\n\nUses `CalculateFIRE` for computation.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	err := CheckSymbols(sliceDir, repoRoot)
	if err != nil {
		t.Fatalf("expected nil when all symbols resolve, got: %v", err)
	}
}

func TestSymbolsAllResolvedExitZero(t *testing.T) {
	design := "# Design\n\n- `CalculateFIRE` is the main function.\n- `main` is the package.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	// Write main.go so `main` resolves as well.
	mainGo := "package main\n\nfunc main() { CalculateFIRE() }\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	err := CheckSymbols(sliceDir, repoRoot)
	if err != nil {
		t.Fatalf("expected nil when all symbols resolve, got: %v", err)
	}
}

func TestSymbolsSnakeCaseResolves(t *testing.T) {
	design := "# Design\n\nUses `start_commit` and `planned_files` from status.json.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	// Write a file with snake_case identifiers.
	schemaGo := "package state\n\nvar start_commit string\nvar planned_files []string\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "schema.go"), []byte(schemaGo), 0o644); err != nil {
		t.Fatalf("write schema.go: %v", err)
	}

	err := CheckSymbols(sliceDir, repoRoot)
	if err != nil {
		t.Fatalf("expected nil when snake_case symbols resolve, got: %v", err)
	}
}

func TestSymbolsSingleWordLowercaseSkips(t *testing.T) {
	// "error" and "todo" are single-word lowercase — should be excluded
	// by the snake_case regex (requires at least one underscore).
	design := "# Design\n\nHandle `error` and mark `todo`.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	err := CheckSymbols(sliceDir, repoRoot)
	if err != nil {
		t.Fatalf("expected nil (single-word lowercase skipped), got: %v", err)
	}
}

func TestSymbolsDottedResolves(t *testing.T) {
	design := "# Design\n\nUses `state.Read` and `rtm.Build`.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	// Write files containing the dotted identifiers as literal substrings
	// (grep matches the verbatim token, not the package-qualified definition).
	stateGo := "package state\n\nfunc Read(path string) {} // called as state.Read\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "state.go"), []byte(stateGo), 0o644); err != nil {
		t.Fatalf("write state.go: %v", err)
	}
	rtmGo := "package rtm\n\nfunc Build(dir string) {} // called as rtm.Build\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "rtm.go"), []byte(rtmGo), 0o644); err != nil {
		t.Fatalf("write rtm.go: %v", err)
	}
	err := CheckSymbols(sliceDir, repoRoot)
	if err != nil {
		t.Fatalf("expected nil when dotted symbols resolve, got: %v", err)
	}
}

func TestSymbolsNoBackticks(t *testing.T) {
	design := "# Design\n\nNo backticks here, just plain prose.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	err := CheckSymbols(sliceDir, repoRoot)
	if err != nil {
		t.Fatalf("expected nil when no backticks present, got: %v", err)
	}
}

func TestSymbolsDeduplicates(t *testing.T) {
	// Same symbol referenced twice — should only grep once and not double-report.
	design := "# Design\n\nUses `CalculateFIRE` and also `CalculateFIRE`.\n"
	sliceDir, repoRoot := setupSymbolTest(t, design)

	err := CheckSymbols(sliceDir, repoRoot)
	if err != nil {
		t.Fatalf("expected nil when deduplicated symbol resolves, got: %v", err)
	}
}

func TestExtractSymbols(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "camelCase",
			text: "Call `CalculateFIRE` and `ParseJSON`.",
			want: []string{"CalculateFIRE", "ParseJSON"},
		},
		{
			name: "snake_case",
			text: "Uses `start_commit` and `planned_files`.",
			want: []string{"start_commit", "planned_files"},
		},
		{
			name: "dotted",
			text: "Calls `state.Read` and `rtm.Build`.",
			want: []string{"state.Read", "rtm.Build"},
		},
		{
			name: "mixed with prose",
			text: "The `CalculateFIRE` function, plus a `todo` note.",
			want: []string{"CalculateFIRE"},
		},
		{
			name: "single word lowercase excluded",
			text: "Mark as `error`, `todo`, `defer`.",
			want: nil,
		},
		{
			name: "cli flag excluded",
			text: "Run with `--verbose` or `--dry-run`.",
			want: nil,
		},
		{
			name: "empty backticks",
			text: "Empty `` backtick.",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSymbols(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("extractSymbols = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("extractSymbols = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
