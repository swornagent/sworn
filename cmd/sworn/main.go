package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const usage = `Sworn v0.3 is in maintenance bootstrap.

Available:
  sworn version [--json]
  sworn help

Temporarily unavailable:
  sworn board [<run>] [--store <path>] [--json]
  sworn run <run> [<work>] --config <clean-absolute-path> [--json]
`

const (
	maintenanceVersion = "0.3.0-dev"
	maintenanceCommit  = "unknown"
	maintenanceState   = "maintenance-bootstrap"
)

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	State   string `json:"state"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__executor-shim" {
		os.Exit(writeCommandUnavailable("__executor-shim", os.Stderr))
	}
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
		if err := writeVersion(stdout, asJSON); err != nil {
			fmt.Fprintf(stderr, "sworn version: %v\n", err)
			return 1
		}
		return 0
	case "board":
		return writeCommandUnavailable("board", stderr)
	case "run":
		return writeCommandUnavailable("run", stderr)
	default:
		fmt.Fprintf(stderr, "sworn: command %q is not implemented\n", args[0])
		return 2
	}
}

func writeCommandUnavailable(command string, stderr io.Writer) int {
	_, _ = fmt.Fprintf(
		stderr,
		"sworn: %s is unavailable while v0.3 delivery is in maintenance bootstrap\n",
		command,
	)
	return 1
}

func writeVersion(out io.Writer, asJSON bool) error {
	info := versionInfo{
		Version: maintenanceVersion,
		Commit:  maintenanceCommit,
		State:   maintenanceState,
	}
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	}
	_, err := fmt.Fprintf(
		out,
		"sworn %s (%s)\nstate %s\n",
		info.Version,
		info.Commit,
		info.State,
	)
	return err
}
