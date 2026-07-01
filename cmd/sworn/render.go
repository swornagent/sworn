package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/command"
)

func init() {
	command.Register(command.Command{
		Name:    "render",
		Summary: "deterministically render a release's index.md from board.json + slice records",
		Run:     cmdRender,
	})
}

// cmdRender implements `sworn render <release> [project-root]`.
//
// It reads docs/release/<release>/board.json (canonical strict) plus each
// referenced slice's spec.json/status.json and writes docs/release/<release>/
// index.md — a deterministic view of the record, never hand-authored. Mirrors
// `sworn top` / `sworn ship`: positional release, optional project-root
// defaulting to ".".
//
// Fail closed: a missing/malformed/invalid board.json (or a referenced slice
// missing its records) exits non-zero and writes nothing. Exit codes: 0 ok,
// 2 render/IO error, 64 usage error.
func cmdRender(args []string) int {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn render: <release> is required")
		fmt.Fprintln(os.Stderr, "usage: sworn render <release> [project-root]")
		return 64
	}
	releaseName := fs.Arg(0)
	projectRoot := "."
	if fs.NArg() > 1 {
		projectRoot = fs.Arg(1)
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn render: resolve project path: %v\n", err)
		return 2
	}

	if err := board.RenderToFile(absRoot, releaseName); err != nil {
		fmt.Fprintf(os.Stderr, "sworn render: %v\n", err)
		return 2
	}

	indexPath := filepath.Join(absRoot, "docs", "release", releaseName, "index.md")
	fmt.Printf("rendered %s\n", indexPath)
	return 0
}
