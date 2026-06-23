package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/journey"
	"github.com/swornagent/sworn/internal/style"
)

// cmdJourneys implements `sworn journeys [--check] [--impact <release>] [--regen <release>] [project-path]`.
//
// Without flags: runs the elicitation loop — creates a draft journeys
// artefact from the project structure, presents it for human ratification.
//
// With --check: validates the artefact's presence + ratification status.
// Exits 0 when the artefact exists and is human-ratified, 1 otherwise.
//
// With --impact <release>: computes which critical journeys the release
// touches, derived from the release's slice touchpoints and journey surfaces.
// Exits 0 on success (even with an empty touched set), 1 when the journeys
// artefact is missing or unratified, 2 on I/O or parse errors.
//
// With --regen <release>: codifies every walked-pass journey into an automated
// regression test scaffold. Exits 0 when all walked journeys already had
// regression coverage at run start. Exits 1 when one or more walked journeys
// lacked coverage at run start — scaffolds are generated but the command
// exits 1 (fail-closed on pre-codification state, Option A). Exits 2 on I/O
// or parse errors.
//
// Returns exit codes:
//   0  — success (check passed, impact computed, or elicit+ratify succeeded)//   1  — check or impact failed (missing or unratified artefact)
//   2  — unrecoverable error (parse failure, I/O error)
//   64 — usage error
func cmdJourneys(args []string) int {
	fs := flag.NewFlagSet("journeys", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "validate artefact presence + ratification (no draft)")
	impactRelease := fs.String("impact", "", "analyse which journeys a release touches (release name)")
	regenRelease := fs.String("regen", "", "codify walked journeys into regression test scaffolds (release name)")
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

	if *impactRelease != "" {
		return cmdJourneysImpact(absRoot, *impactRelease)
	}

	if *regenRelease != "" {
		return cmdJourneysRegen(absRoot, *regenRelease)
	}

	return cmdJourneysElicit(absRoot)}
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
// cmdJourneysImpact implements the --impact path.
// It reads the journeys artefact from projectRoot and the release slices
// from docs/release/<release-name>/, computes the touched-journey set, and
// reports it.
func cmdJourneysImpact(projectRoot, releaseName string) int {
	releaseDir := filepath.Join(projectRoot, "docs", "release", releaseName)

	result, err := journey.AnalyzeImpact(projectRoot, releaseDir)
	if err != nil {
		// Structured error (missing/unratified artefact).
		var impErr *journey.ImpactError
		if asImpactError(err, &impErr) {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", impErr.Message)
			return 1
		}
		fmt.Fprintf(os.Stderr, "sworn journeys --impact: %v\n", err)
		return 2
	}

	fmt.Println(style.Accent(fmt.Sprintf("Release: %s", result.ReleaseName)))
	fmt.Printf("Journeys artefact: found and ratified\n")
	fmt.Println()
	fmt.Printf("Journeys touched by this release (%d):\n", len(result.JourneysTouched))
	if len(result.JourneysTouched) == 0 {
		fmt.Println("  (none — release touches no critical journeys)")
	} else {
		for _, j := range result.JourneysTouched {
			fmt.Println("  -", j)
		}
	}

	if len(result.AllJourneyIDs) > 0 {
		fmt.Println()
		fmt.Printf("All ratified journeys (%d):\n", len(result.AllJourneyIDs))
		for _, j := range result.AllJourneyIDs {
			mark := " "
			for _, t := range result.JourneysTouched {
				if j == t {
					mark = "*"
					break
				}
			}
			fmt.Printf("  %s %s\n", mark, j)
		}
	}

	return 0
}

// asImpactError unwraps a *journey.ImpactError (duplicate of test helper
// so the binary has no test dependencies).
func asImpactError(err error, target **journey.ImpactError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*journey.ImpactError); ok {
		*target = e
		return true
	}
	if e, ok := err.(interface{ Unwrap() error }); ok {
		return asImpactError(e.Unwrap(), target)
	}
	return false
}

// cmdJourneysRegen implements the --regen path.
// It reads the journeys artefact and attestations, then codifies each
// walked-pass journey as a regression test scaffold. Previously-codified
// journeys are preserved (accretive).
//
// Exit codes:
//
//	0 — success; all walked journeys already had regression coverage at run start
//	1 — one or more walked journeys lacked coverage at run start (scaffolds
//	    generated; exit 1 even if all gaps were filled during this run)
//	2 — unrecoverable error (I/O, parse failure, missing artefact)
func cmdJourneysRegen(projectRoot, releaseName string) int {	// 1. Load and verify the journeys artefact.
	checkResult, artefact, err := journey.Check(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys --regen: %v\n", err)
		return 2
	}
	switch checkResult {
	case journey.CheckMissing:
		fmt.Fprintf(os.Stderr, "FAIL: no journeys artefact at %s.\n",
			journey.JourneyArtefactPath(projectRoot))
		fmt.Fprintln(os.Stderr, "Run 'sworn journeys <project>' to elicit journeys first (S11).")
		return 1
	case journey.CheckUnratified:
		fmt.Fprintf(os.Stderr, "FAIL: journeys artefact exists but is NOT human-ratified.\n")
		fmt.Fprintln(os.Stderr, "Ratify the artefact before running regression codification.")
		return 1
	}

	// 2. Load attestations.
	attArtefact, err := journey.LoadAttestations(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys --regen: load attestations: %v\n", err)
		return 2
	}

	// 3. Check for coverage gaps BEFORE codification (so we can report). But
	// we still codify — the fail-closed check is a separate signal.
	gaps := journey.RegressionCoverageGaps(artefact, attArtefact, projectRoot)

	// 4. Codify walked-pass journeys.
	generated, err := journey.CodifyWalkedJourneys(artefact, attArtefact, "", projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys --regen: codify: %v\n", err)
		return 2
	}

	// 5. Save the updated artefact (HasRegression + RegressionTestPath).
	if err := journey.SaveArtefact(projectRoot, artefact); err != nil {
		fmt.Fprintf(os.Stderr, "sworn journeys --regen: save artefact: %v\n", err)
		return 2
	}

	// Report results.
	fmt.Printf("Release: %s\n", releaseName)
	fmt.Printf("Journeys artefact: found and ratified (by %s)\n", artefact.RatifiedBy)
	fmt.Println()

	if len(generated) > 0 {
		fmt.Printf("Generated %d regression test scaffold(s):\n", len(generated))
		for _, g := range generated {
			fmt.Println("  ", g)
		}
		fmt.Println()
	} else {
		fmt.Println("No new regression scaffolds needed (all walked journeys already covered).")
		fmt.Println()
	}

	if len(gaps) > 0 {
		// Gaps existed at run start — exit 1 even if all were filled during
		// this run per AC1 (Option A: fail-closed on pre-codification state).
		// The re-check distinguishes "some gaps remain" from "all filled".
		remaining := journey.RegressionCoverageGaps(artefact, attArtefact, projectRoot)
		if len(remaining) > 0 {
			fmt.Fprintf(os.Stderr, "FAIL: %d journey(s) flagged for regression with no committed test:\n", len(remaining))
			for _, id := range remaining {
				fmt.Fprintf(os.Stderr, "  - %s\n", id)
			}
			fmt.Fprintln(os.Stderr, "These journeys have passing walkthroughs but no regression test.")
			fmt.Fprintln(os.Stderr, "Add a committed test or mark as excluded.")
			return 1
		}
		// All gaps filled during this run — but gaps existed at start, so
		// still exit 1 per AC1 (fail-closed on pre-codification state).
		fmt.Fprintf(os.Stderr, "FAIL: %d coverage gap(s) existed at run start (all filled during this run).\n", len(gaps))
		for _, id := range gaps {
			fmt.Fprintf(os.Stderr, "  - %s\n", id)
		}
		fmt.Fprintln(os.Stderr, "Coverage gaps existed at run start — scaffolds were generated. Commit them and re-run.")
		return 1
	} else {
		fmt.Println("Coverage check: all walked journeys have regression coverage.")
	}
	return 0
}