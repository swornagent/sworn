package tui

import (
	"context"
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run opens the project cockpit in the terminal alternate screen.
func Run(ctx context.Context, version string, backend Backend) error {
	if ctx == nil || backend == nil {
		return errors.New("tui unavailable")
	}
	if closer, ok := backend.(io.Closer); ok {
		defer closer.Close()
	}
	program := tea.NewProgram(
		newModel(ctx, version, backend),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	return err
}
