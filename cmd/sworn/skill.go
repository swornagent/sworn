package main

import (
	"fmt"
	"io"
	"os"

	"github.com/swornagent/sworn/internal/skill"
)

func runSkill(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "install" {
		fmt.Fprintln(stderr, "usage: sworn skill install [--home ABS]")
		return 2
	}
	options, ok := parseOptionsWithOptionalValues(args[1:], nil, []string{"--home"}, nil, nil)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn skill install [--home ABS]")
		return 2
	}
	home := options["--home"]
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			writeKnownFailure(stdout, "skill install", "Could not find the current user's home directory.", "")
			return 1
		}
		home = resolved
	}
	report, err := skill.Install(home)
	if err != nil {
		writeKnownFailure(
			stdout,
			"skill install",
			err.Error(),
			"",
		)
		return 1
	}
	for _, path := range report.MigratedStubs {
		fmt.Fprintf(stdout, "sworn skill install: migrated %s\n", path)
	}
	for _, path := range report.InstalledPaths {
		fmt.Fprintf(stdout, "sworn skill install: installed %s\n", path)
	}
	if len(report.MigratedStubs) == 0 && len(report.InstalledPaths) == 0 {
		fmt.Fprintln(stdout, "sworn skill install: nothing to do")
	}
	return 0
}
