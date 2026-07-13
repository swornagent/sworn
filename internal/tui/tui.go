// Package tui provides the Bubble Tea TUI for `sworn` (no args) and
// `sworn top` (no release arg). It shows a releases list in the left pane
// and a board view (tracks + slice states) in the right pane.
//
// Exported extension points (for S04b/S04c):
//   - Model.Releases — ReleasesList component (upgrade for live data)
//   - Model.Board — BoardView component (add TL;DR overlay)
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Run launches the Bubble Tea TUI program. It is the entry point called
// from cmd/sworn/main.go and cmd/sworn/top.go (when no release arg given).
// version is the sworn binary version (the value `sworn --version` reports),
// shown in the TUI header (S03, AC-03). Returns an error only if the TUI
// cannot start (not on user quit).
func Run(version string) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("finding repo root: %w", err)
	}

	m := &Model{
		state:    viewReleases,
		repoRoot: repoRoot,
		Version:  version,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}

	// Load releases before starting.
	if err := m.Releases.LoadReleases(repoRoot); err != nil {
		m.errMsg = err.Error()
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// findRepoRoot runs git rev-parse --show-toplevel to find the repo root.
func findRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		// Fallback to CWD.
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return cwd, nil
	}
	return strings.TrimSpace(string(out)), nil
}
