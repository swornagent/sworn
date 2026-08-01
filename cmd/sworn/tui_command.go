package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	term "github.com/charmbracelet/x/term"
	"github.com/swornagent/sworn/internal/tui"
)

type tuiOptions struct {
	projectPath string
	journalPath string
	configPath  string
	manifestDir string
}

func runTUI(args []string, stdout, stderr io.Writer) int {
	options, ok := parseTUIOptions(args)
	if !ok {
		fmt.Fprintln(
			stderr,
			"usage: sworn tui [--project ABS] [--journal ABS] "+
				"[--config ABS] [--manifest-dir ABS]",
		)
		return 2
	}
	if !terminalIsInteractive(os.Stdin, os.Stdout) {
		fmt.Fprintln(
			stderr,
			"sworn tui: An interactive terminal is required. Run \"sworn help\" for commands that work in scripts.",
		)
		return 1
	}
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	backend := newProjectTUIBackend(
		options.projectPath,
		options.journalPath,
		options.configPath,
		options.manifestDir,
	)
	if err := tui.Run(ctx, swornVersion, backend); err != nil {
		fmt.Fprintln(
			stderr,
			"sworn tui: Could not open the project board in this terminal.",
		)
		return 1
	}
	return 0
}

func parseTUIOptions(args []string) (tuiOptions, bool) {
	values, ok := parseOptionsWithOptionalValues(
		args,
		nil,
		[]string{"--project", "--journal", "--config", "--manifest-dir"},
		nil,
		nil,
	)
	if !ok {
		return tuiOptions{}, false
	}
	options := tuiOptions{}
	for name, destination := range map[string]*string{
		"--project":      &options.projectPath,
		"--journal":      &options.journalPath,
		"--config":       &options.configPath,
		"--manifest-dir": &options.manifestDir,
	} {
		value := values[name]
		if value == "" {
			continue
		}
		if strings.ContainsRune(value, 0) || !filepath.IsAbs(value) ||
			filepath.Clean(value) != value {
			return tuiOptions{}, false
		}
		*destination = value
	}
	return options, true
}

func terminalIsInteractive(stdin, stdout *os.File) bool {
	if stdin == nil || stdout == nil {
		return false
	}
	return fileIsTerminal(stdin) && fileIsTerminal(stdout)
}

func fileIsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	return term.IsTerminal(file.Fd())
}
