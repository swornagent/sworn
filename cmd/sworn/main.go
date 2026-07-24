package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/swornagent/sworn/internal/baton"
)

const usage = `Sworn runs autonomous delivery with the Baton protocol.

Available at the v0.3 admission checkpoint:
  sworn version [--json]
  sworn help

The delivery loop and board arrive in later v0.3 work.
`

const (
	swornVersion = "0.3.0-dev"
	swornState   = "baton-rc2-admitted"
)

type versionInfo struct {
	Version string         `json:"version"`
	State   string         `json:"state"`
	Baton   baton.Identity `json:"baton"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	if args[0] != "version" {
		fmt.Fprintf(stderr, "sworn: command %q is not implemented at the v0.3 admission checkpoint\n", args[0])
		return 2
	}

	asJSON := false
	if len(args) == 2 && args[1] == "--json" {
		asJSON = true
	} else if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sworn version [--json]")
		return 2
	}
	pkg, err := baton.Load()
	if err != nil {
		fmt.Fprintf(stderr, "sworn version: %v\n", err)
		return 1
	}
	batonIdentity, err := pkg.Identity()
	if err != nil {
		fmt.Fprintf(stderr, "sworn version: %v\n", err)
		return 1
	}
	if err := writeVersion(stdout, asJSON, batonIdentity); err != nil {
		fmt.Fprintf(stderr, "sworn version: %v\n", err)
		return 1
	}
	return 0
}

func writeVersion(out io.Writer, asJSON bool, batonIdentity baton.Identity) error {
	info := versionInfo{
		Version: swornVersion,
		State:   swornState,
		Baton:   batonIdentity,
	}
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	}
	_, err := fmt.Fprintf(
		out,
		"sworn %s\nstate %s\nbaton %s (%s)\n",
		info.Version,
		info.State,
		info.Baton.PackageVersion,
		info.Baton.Commit,
	)
	return err
}
