package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/swornagent/sworn/internal/gitx"
)

// migrationEngineIdentity is the explicit engine identity the one-time
// operator-gated records migration commits with. It is attribution only.
var migrationEngineIdentity = gitx.Identity{
	Name:  "Sworn Records Engine",
	Email: "records@" + engineIdentityDomain,
}

// runMigrateRecords is the operator-gated one-time relocation of the
// reserved records root from the historical .baton/releases location to the
// configured .sworn/records root. It is never a silent side effect of
// ordinary model-directed work: it requires --confirm, refuses a dirty tree
// or index, refuses when nothing remains to migrate, refuses to overwrite an
// already-relocated record, and is idempotent.
func runMigrateRecords(args []string, stdout, stderr io.Writer) int {
	options, ok := parseOptionsWithOptionalValues(
		args,
		[]string{"--project"},
		nil,
		nil,
		[]string{"--confirm"},
	)
	if !ok {
		fmt.Fprintln(stderr, "usage: sworn migrate-records --project ABS [--confirm]")
		return 2
	}
	if options["--confirm"] != "true" {
		writeKnownFailure(
			stderr,
			"migrate-records",
			"The reserved records root move is one-time and operator-gated. Pass --confirm to run it.",
			"CONFIRMATION_REQUIRED",
		)
		return 1
	}
	project := options["--project"]
	if project == "" || !filepath.IsAbs(project) || filepath.Clean(project) != project {
		writeKnownFailure(
			stderr,
			"migrate-records",
			"--project must be an absolute path to the Git project.",
			"INVALID_REPOSITORY",
		)
		return 1
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		writeKnownFailure(stderr, "migrate-records", "Could not find Git.", "GIT_UNAVAILABLE")
		return 1
	}
	repository, err := gitx.Open(project, gitExecutable)
	if err != nil {
		writeKnownFailure(
			stderr,
			"migrate-records",
			"Could not open the Git project at the given path.",
			commandErrorCode(err),
		)
		return 1
	}
	migration, err := repository.MigrateLegacyRecords(gitx.MigrateRecordsRequest{
		Confirmed: true,
		Identity:  migrationEngineIdentity,
	})
	if err != nil {
		writeKnownFailure(
			stderr,
			"migrate-records",
			"The records root could not be migrated.",
			commandErrorCode(err),
		)
		return 1
	}
	fmt.Fprintf(stdout, "Migrated %d release records from .baton/releases to %s.\n",
		len(migration.Releases), repository.RecordRoot())
	for _, release := range migration.Releases {
		fmt.Fprintf(stdout, "  %s\n", release)
	}
	fmt.Fprintf(stdout, "Marker commit: %s\n", migration.Commit.String())
	return 0
}
