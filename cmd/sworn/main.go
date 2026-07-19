package main

import (
	"fmt"
	"io"
	"os"

	"github.com/swornagent/sworn/internal/buildinfo"
)

const usage = `Sworn is a deterministic delivery engine.

Usage:
  sworn version [--json]
  sworn help

The delivery commands are not implemented in this walking-skeleton milestone.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
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
	default:
		fmt.Fprintf(stderr, "sworn: command %q is not implemented\n", args[0])
		return 2
	}
}
