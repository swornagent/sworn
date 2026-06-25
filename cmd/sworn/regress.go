package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/gate"
)

// cmdRegress implements `sworn regress --release <release-name>`.
//
// Runs the full test suite (Go + TS + golden fixtures) against the merged
// release-wt worktree. Resolves the worktree path from the release board's
// index.md frontmatter.  Exits 0 when all suites pass, 1 on any failure.
//
// Usage:
//
//	sworn regress --release <release-name> [--json]
func cmdRegress(args []string) int {
	fs := flag.NewFlagSet("regress", flag.ExitOnError)
	releaseName := fs.String("release", "", "release name (e.g. 2026-06-19-safe-parallelism)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	if *releaseName == "" {
		fmt.Fprintln(os.Stderr, "sworn regress: --release is required")
		fmt.Fprintln(os.Stderr, "usage: sworn regress --release <release-name> [--json]")
		return 64
	}

	// Resolve the release directory (docs/release/<name> relative to CWD).
	releaseDir, err := resolveReleaseDir(*releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn regress: %v\n", err)
		return 2
	}

	// Read index.md to extract the release worktree path from frontmatter.
	indexPath := filepath.Join(releaseDir, "index.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn regress: read index.md: %v\n", err)
		return 2
	}

	worktreePath := extractReleaseWorktreePath(string(indexData))
	if worktreePath == "" {
		fmt.Fprintln(os.Stderr, "sworn regress: release_worktree_path not set in index.md frontmatter")
		return 2
	}

	// Ensure the worktree exists on disk.
	if info, err := os.Stat(worktreePath); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "sworn regress: release worktree not found: %s\n", worktreePath)
		return 2
	}

	report, err := gate.RunRegress(worktreePath, *releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn regress: %v\n", err)
		return 2
	}

	if *jsonOut {
		fmt.Print(gate.JSONRegress(report))
	} else {
		fmt.Print(gate.PrintRegress(report))
	}

	if report.HasViolations() {
		return 1
	}
	return 0
}

// extractReleaseWorktreePath extracts release_worktree_path from index.md
// YAML frontmatter. Returns "" when not found or frontmatter is absent.
func extractReleaseWorktreePath(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if strings.HasPrefix(trimmed, "release_worktree_path:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "release_worktree_path:"))
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}