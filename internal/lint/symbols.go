// Package lint implements the `sworn lint` sub-targets that perform
// mechanical, pre-verification checks on release slices. Each target is
// fail-closed: exit 0 only when the check passes, non-zero on any violation.
//
// The symbols target extracts backtick-quoted identifiers from a slice's
// design.md, greps each against the live codebase, and returns advisory
// warnings for unresolved symbols.
package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// symbolPatterns is the set of regex patterns that match code-symbol-shaped
// identifiers inside backtick quotes. We only extract backtick-quoted tokens
// that match at least one of these — plain prose backticks are skipped.
//
//   - CamelCase / dotted: `CalculateFIRE`, `state.Read`, `rtm.Build`
//   - snake_case: `start_commit`, `planned_files`, `check_deps`
//
// Single-word lowercase identifiers (e.g. `todo`, `error`) are intentionally
// excluded — they are too common in prose to distinguish from code symbols.
var symbolPatterns = []*regexp.Regexp{
	// CamelCase or dotted: uppercase-starting words optionally chained with dots.
	regexp.MustCompile(`\b[A-Z][a-zA-Z0-9]*(\.[A-Z][a-zA-Z0-9]*)+\b`),
	regexp.MustCompile(`\b[A-Z][a-zA-Z0-9]*\b`),
	// snake_case: multi-underscore only — single-word lowercase is intentionally excluded.
	regexp.MustCompile(`\b[a-z]+(_[a-z0-9]+)+\b`),
}

// backtickPattern extracts all backtick-quoted strings from text.
var backtickPattern = regexp.MustCompile("`([^`]+)`")

// CheckSymbols reads the slice's design.md, extracts every backtick-quoted
// identifier that matches a code-symbol shape (CamelCase, dotted, snake_case),
// and greps each against repoRoot (excluding docs/). Returns nil if all
// identifiers resolve. Returns an error naming the unresolved identifiers
// otherwise — this is advisory (the caller chooses the exit code), not a
// hard-fail contract violation.
func CheckSymbols(sliceDir, repoRoot string) error {
	designPath := filepath.Join(sliceDir, "design.md")
	data, err := os.ReadFile(designPath)
	if err != nil {
		return fmt.Errorf("lint symbols: reading design.md: %w", err)
	}

	symbols := extractSymbols(string(data))
	if len(symbols) == 0 {
		return nil
	}

	unresolved := grepSymbols(symbols, repoRoot)
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		return fmt.Errorf("unresolved symbol(s): %s", strings.Join(unresolved, ", "))
	}
	return nil
}

// extractSymbols finds every code-symbol-shaped identifier inside backtick
// quotes in text. Duplicates are deduplicated.
func extractSymbols(text string) []string {
	seen := make(map[string]bool)
	var result []string

	matches := backtickPattern.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		token := strings.TrimSpace(m[1])
		if token == "" {
			continue
		}
		if seen[token] {
			continue
		}
		if !isSymbol(token) {
			continue
		}
		seen[token] = true
		result = append(result, token)
	}
	return result
}

// isSymbol reports whether token matches at least one code-symbol shape.
func isSymbol(token string) bool {
	for _, pat := range symbolPatterns {
		if pat.MatchString(token) {
			return true
		}
	}
	return false
}

// grepSymbols greps each symbol against the repoRoot (excluding docs/) and
// returns the subset that are not found anywhere in the live codebase.
func grepSymbols(symbols []string, repoRoot string) []string {
	var unresolved []string
	for _, sym := range symbols {
		if !grepOne(sym, repoRoot) {
			unresolved = append(unresolved, sym)
		}
	}
	return unresolved
}

// grepOne runs grep -r --include='*.go' (plus other code extensions) for a
// single symbol against repoRoot, excluding docs/. Returns true if any match
// is found.
func grepOne(symbol, repoRoot string) bool {
	// Use grep -r with include patterns for code files. We search across Go,
	// TypeScript, Python, YAML, JSON, Markdown (non-docs), and plain text files
	// because a symbol might appear in config, templates, or migration files
	// rather than just Go.
	//
	// Exclude docs/ to avoid matching the design document's own mentions.
	cmd := exec.Command("grep",
		"-r", "--include=*.go", "--include=*.ts", "--include=*.tsx",
		"--include=*.py", "--include=*.yaml", "--include=*.yml",
		"--include=*.json", "--include=*.md", "--include=*.txt",
		"--include=*.sql", "--include=*.html", "--include=*.css",
		"--exclude-dir=docs",
		"-l", "-F", symbol,
		repoRoot,
	)
	out, err := cmd.Output()
	if err != nil {
		// grep exits 1 when no match — that's our "unresolved" signal.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false
		}
		// Other errors (grep not found, permission denied, etc.) — treat as
		// unresolved rather than crashing, since this is advisory.
		return false
	}
	return len(out) > 0
}
