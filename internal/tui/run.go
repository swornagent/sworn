package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// Run opens the project cockpit in the terminal alternate screen.
func Run(ctx context.Context, version string, backend Backend) error {
	if ctx == nil || backend == nil {
		return errors.New("tui unavailable")
	}
	program := tea.NewProgram(
		newModel(ctx, version, backend),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	return err
}
