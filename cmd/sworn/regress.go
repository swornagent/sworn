package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/gate"
	"github.com/swornagent/sworn/internal/git"
)

// cmdRegress implements `sworn regress --release <release-name>`.
//
// Runs the full test suite (Go + TS + golden fixtures) against the merged
// release-wt worktree. Resolves the worktree path from the release board
// (board.json when present, falling back to legacy index.md frontmatter via
// board.ReadBoard). Exits 0 when all suites pass, 1 on any failure.
//
// Usage:
//
//	sworn regress --release <release-name> [--json]
func cmdRegress(args []string) int {
	fs := flag.NewFlagSet("regress", flag.ExitOnError)
	releaseName := fs.String("release", "", "release name (e.g. 2026-06-19-safe-parallelism)")
	// --worktree overrides the index.md-resolved release worktree with an explicit
	// path. The merge-track gate (Step 2.5) uses it to run the affected-package
	// suite in a TRACK worktree on the merged base, before the track lands on
	// release-wt — per-slice test_commands only cover each slice's own package, so
	// a shared-file change can break a package no slice's command names. Pointing
	// regress at the track worktree catches that cross-package regression.
	worktreeOverride := fs.String("worktree", "", "run the suite in this worktree instead of the index.md-resolved release worktree")
	jsonOut := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	if *releaseName == "" {
		fmt.Fprintln(os.Stderr, "sworn regress: --release is required")
		fmt.Fprintln(os.Stderr, "usage: sworn regress --release <release-name> [--worktree <path>] [--json]")
		return 64
	}

	var worktreePath string
	if *worktreeOverride != "" {
		// Explicit worktree: skip board resolution entirely.
		worktreePath = *worktreeOverride
	} else {
		// Resolve the release directory (docs/release/<name> relative to CWD) —
		// kept ahead of the board read for a clearer "release directory not
		// found" error than a raw ReadBoard failure would give.
		if _, err := resolveReleaseDir(*releaseName); err != nil {
			fmt.Fprintf(os.Stderr, "sworn regress: %v\n", err)
			return 2
		}

		// Read release_worktree_path from board.json (preferred), falling back
		// to a lazy migration from index.md frontmatter for pre-ADR-0009
		// releases (AC-03) — same oracle S04 adopted for its repo==nil paths.
		br, err := board.ReadBoard(".", *releaseName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn regress: read board: %v\n", err)
			return 2
		}

		// board-v1 is a pure plan: the release worktree path is DERIVED as a
		// sibling of the primary repo (Pin 1 / sworn#80), not read from board.json.
		// Resolve the primary worktree root so the derivation holds even when
		// regress runs from a linked worktree; _ = br keeps the board read (which
		// still fails closed on a malformed/absent board).
		_ = br
		root, rerr := git.New(".").PrimaryWorktreeRoot()
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "sworn regress: cannot resolve repo root: %v\n", rerr)
			return 2
		}
		// Fail-closed target assertion (Rule 11): an empty path must never flow
		// into the worktree-stat guard below.
		worktreePath = board.ReleaseWorktreePathFrom(root, *releaseName)
		if worktreePath == "" {
			fmt.Fprintln(os.Stderr, "sworn regress: release worktree path not derivable (no repo root)")
			return 2
		}
	}

	// Ensure the worktree exists on disk. Fail-closed target assertion (Rule 11):
	// never run the suite against a missing or non-directory path.
	if info, err := os.Stat(worktreePath); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "sworn regress: worktree not found: %s\n", worktreePath)
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
