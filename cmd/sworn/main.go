package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/swornagent/sworn/internal/app"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/buildinfo"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/store"
)

const usage = `Sworn is a deterministic delivery engine.

Usage:
  sworn version [--json]
  sworn board [<run>] [--store <path>] [--json]
  sworn run <run> [<work>] --config <clean-absolute-path> [--json]
  sworn help
`

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__executor-shim" {
		os.Exit(executor.RunShim(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := runWithApplication(ctx, os.Args[1:], os.Stdout, os.Stderr, app.Run)
	stop()
	os.Exit(code)
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithApplication(context.Background(), args, stdout, stderr, app.Run)
}

func runWithApplication(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	application runApplication,
) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}

	switch args[0] {
	case "version":
		asJSON := false
		if len(args) == 2 && args[1] == "--json" {
			asJSON = true
		} else if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: sworn version [--json]")
			return 2
		}
		if err := buildinfo.Write(stdout, asJSON); err != nil {
			fmt.Fprintf(stderr, "sworn version: %v\n", err)
			return 1
		}
		return 0
	case "board":
		return runBoard(args[1:], stdout, stderr)
	case "run":
		return runDelivery(ctx, args[1:], stdout, stderr, application)
	default:
		fmt.Fprintf(stderr, "sworn: command %q is not implemented\n", args[0])
		return 2
	}
}

func runBoard(args []string, stdout, stderr io.Writer) int {
	storePath := ".sworn/sworn.db"
	asJSON := false
	runID := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			asJSON = true
		case "--store":
			index++
			if index >= len(args) || args[index] == "" {
				fmt.Fprintln(stderr, "sworn board: --store requires a path")
				return 2
			}
			storePath = args[index]
		default:
			if len(args[index]) > 0 && args[index][0] == '-' {
				fmt.Fprintf(stderr, "sworn board: unknown option %q\n", args[index])
				return 2
			}
			if runID != "" {
				fmt.Fprintln(stderr, "sworn board: at most one run may be selected")
				return 2
			}
			runID = args[index]
		}
	}

	control, err := store.OpenReadOnly(context.Background(), storePath)
	if err != nil {
		fmt.Fprintf(stderr, "sworn board: %v\n", err)
		return 1
	}
	state, err := selectState(context.Background(), control, runID)
	if err != nil {
		_ = control.Close()
		fmt.Fprintf(stderr, "sworn board: %v\n", err)
		return 1
	}
	projection, err := board.FromState(state)
	if err != nil {
		_ = control.Close()
		fmt.Fprintf(stderr, "sworn board: %v\n", err)
		return 1
	}
	if err := control.Close(); err != nil {
		fmt.Fprintf(stderr, "sworn board: close control store: %v\n", err)
		return 1
	}
	if asJSON {
		err = board.WriteJSON(stdout, projection)
	} else {
		err = board.WriteText(stdout, projection)
	}
	if err != nil {
		fmt.Fprintf(stderr, "sworn board: write output: %v\n", err)
		return 1
	}
	return 0
}
