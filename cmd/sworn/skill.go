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
	explicitHome := options["--home"] != ""
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
	// The artefact home is Sworn's machine/user artefact location; the
	// default (unoverridden) install also places the skill there so the
	// artefact home carries Sworn's user-scoped artefact. An explicit
	// --home targets only the agent discovery roots the caller named.
	if !explicitHome {
		artefactPath, artefactErr := skill.InstallArtefact()
		if artefactErr != nil {
			writeKnownFailure(stdout, "skill install", artefactErr.Error(), "")
			return 1
		}
		fmt.Fprintf(stdout, "sworn skill install: installed %s\n", artefactPath)
	}
	return 0
}
