package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/journey"
)

// cmdJourneys implements `sworn journeys [--check] [project-path]`.
//
// Without --check: runs the elicitation loop — creates a draft journeys
// artefact from the project structure, presents it for human ratification.
//
// With --check: validates the artefact's presence + ratification status.
// Exits 0 when the artefact exists and is human-ratified, 1 otherwise.
//
// Returns exit codes:
//   0  — check passed (artefact exists and is ratified), or elicit+ratify succeeded
//   1  — check failed (missing or unratified artefact)
//   2  — unrecoverable error (parse failure, I/O error)
//   64 — usage error
func cmdJourneys(args []string) int {
	fs := flag.NewFlagSet("journeys", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "validate artefact presence + ratification (no draft)")
	_ = fs.Parse(args)

	projectRoot := "."
	if fs.NArg() > 0 {
		projectRoot = fs.Arg(0)
	}

	// Resolve the project root to an absolute path for stable output.
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys: resolve path: %v\n", err)
		return 2
	}

	if *checkOnly {
		return cmdJourneysCheck(absRoot)
	}

	return cmdJourneysElicit(absRoot)
}

// cmdJourneysCheck implements the --check path. It reads the artefact and
// checks presence + ratification. Exits 0 on pass, 1 on failure.
func cmdJourneysCheck(projectRoot string) int {
	result, artefact, err := journey.Check(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys --check: %v\n", err)
		return 2
	}

	switch result {
	case journey.CheckPass:
		fmt.Printf("Journeys artefact found and ratified by %s.\n", artefact.RatifiedBy)
		fmt.Println()
		for _, j := range journey.ListJourneys(artefact) {
			fmt.Println("  ", j)
		}
		return 0

	case journey.CheckMissing:
		fmt.Fprintf(os.Stderr, "FAIL: no journeys artefact found at %s.\n",
			journey.JourneyArtefactPath(projectRoot))
		fmt.Fprintln(os.Stderr, "Elicitation has not been run. Run 'sworn journeys <project>' to start.")
		return 1

	case journey.CheckUnratified:
		fmt.Fprintf(os.Stderr, "FAIL: journeys artefact exists but is NOT human-ratified.\n")
		fmt.Fprintln(os.Stderr, "Run 'sworn journeys <project>' to review and ratify the draft journeys.")
		return 1

	default:
		fmt.Fprintf(os.Stderr, "sworn journeys --check: unexpected result %v\n", result)
		return 2
	}
}

// cmdJourneysElicit implements the non--check path: elicitation loop.
// It drafts candidate journeys from the project structure, writes the
// artefact, and tells the user how to ratify.
func cmdJourneysElicit(projectRoot string) int {
	// Check if an artefact already exists.
	result, existingArtefact, err := journey.Check(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys: read existing artefact: %v\n", err)
		return 2
	}

	if result == journey.CheckPass {
		fmt.Printf("Journeys artefact already exists and is ratified (by %s).\n",
			existingArtefact.RatifiedBy)
		fmt.Println("Current journeys:")
		for _, j := range journey.ListJourneys(existingArtefact) {
			fmt.Println("  ", j)
		}
		fmt.Println()
		fmt.Println("To re-elicit, delete the artefact and run again.")
		return 0
	}

	artefactPath := journey.JourneyArtefactPath(projectRoot)

	if result == journey.CheckUnratified {
		// Artefact exists but is unratified — load it and present it.
		existing, loadErr := journey.LoadArtefact(projectRoot)
		if loadErr == nil {
			fmt.Printf("Using existing unratified artefact at %s.\n", artefactPath)
			fmt.Println()
			fmt.Println("Draft journeys:")
			for _, j := range journey.ListJourneys(existing) {
				fmt.Println("  ", j)
			}
			fmt.Println()
			fmt.Println("To ratify, edit the artefact at", artefactPath)
			fmt.Println("then run: sworn journeys --check", projectRoot)
			return 0
		}
		// If load fails, fall through to re-draft.
	}

	// Draft template journeys from the project structure.
	a, err := journey.DraftTemplate(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys: draft: %v\n", err)
		return 2
	}

	// Save the draft artefact.
	if err := journey.SaveArtefact(projectRoot, a); err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys: save artefact: %v\n", err)
		return 2
	}

	fmt.Printf("Journeys artefact drafted at %s.\n", artefactPath)
	fmt.Println()
	fmt.Println("Draft journeys:")
	for _, j := range journey.ListJourneys(a) {
		fmt.Println("  ", j)
	}
	fmt.Println()
	fmt.Println("To ratify:")
	fmt.Println("  1. Review and edit the draft at", artefactPath)
	fmt.Println("  2. Run: sworn journeys --check", projectRoot)
	fmt.Println("     (this will fail — the artefact needs human ratification)")
	fmt.Println("  3. Edit the artefact, set is_ratified=true, ratified_by=\"your-name\", ratified_at=<ISO 8601>")
	fmt.Println("  4. Re-run: sworn journeys --check", projectRoot)
	fmt.Println("     (should now pass)")
	fmt.Println()
	fmt.Println("Provisional: the exact ratification workflow will be refined")
	fmt.Println("via the live journey-validation hand-run (refined by /replan-release).")

	return 0
}