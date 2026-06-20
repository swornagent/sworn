package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/swornagent/sworn/internal/journey"
)

// cmdShip implements `sworn ship <release> [project-root]`.
//
// sworn ship is the cutover gate: it fails closed unless every journey in
// the release's validation scope (computed by S12 impact analysis) carries a
// recorded human-walkthrough attestation with passing result, real-infra
// asserted, and mocks-off asserted.
//
// Returns exit codes:
//   0  — all touched journeys have complete, passing human attestations
//   1  — one or more journeys are un-walked, incomplete, or failed
//   2  — unrecoverable error (I/O or parse failure)
//   64 — usage error
func cmdShip(args []string) int {
	fs := flag.NewFlagSet("ship", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: sworn ship <release> [project-root]\n")
		return 64
	}

	releaseName := fs.Arg(0)
	projectRoot := "."
	if fs.NArg() > 1 {
		projectRoot = fs.Arg(1)
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn ship: resolve path: %v\n", err)
		return 2
	}

	releaseDir := filepath.Join(absRoot, "docs", "release", releaseName)

	// Check if the release directory exists.
	if _, err := os.Stat(releaseDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "sworn ship: release %q not found at %s\n", releaseName, releaseDir)
		return 2
	}

	// Run the ship gate.
	result, err := journey.CheckShipGate(absRoot, releaseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn ship: %v\n", err)
		return 2
	}

	if result.Pass {
		fmt.Printf("sworn ship: release %q passed the ship gate — all touched journeys have complete, ", releaseName)
		fmt.Printf("passing human-walkthrough attestations.\n")
		fmt.Println()
		fmt.Println("The release may proceed to cutover.")
		return 0
	}

	// Gate blocked — print the kill-list.
	fmt.Fprintf(os.Stderr, "FAIL: cutover blocked — release %q cannot ship.\n", releaseName)
	fmt.Fprintln(os.Stderr)

	unwalked := result.UnwalkedJourneys()
	if len(unwalked) > 0 {
		sort.Strings(unwalked)
		fmt.Fprintf(os.Stderr, "Journeys with NO human-walkthrough attestation (%d):\n", len(unwalked))
		for _, jid := range unwalked {
			fmt.Fprintf(os.Stderr, "  - %s\n", jid)
		}
		fmt.Fprintln(os.Stderr)
	}

	incomplete := result.IncompleteJourneys()
	if len(incomplete) > 0 {
		sort.Strings(incomplete)
		fmt.Fprintf(os.Stderr, "Journeys with INCOMPLETE or FAILED attestations (%d):\n", len(incomplete))
		for _, detail := range incomplete {
			fmt.Fprintf(os.Stderr, "  - %s\n", detail)
		}
		fmt.Fprintln(os.Stderr)
	}

	fmt.Fprintln(os.Stderr, "To ship, each touched journey needs a passing human-walkthrough attestation that:")
	fmt.Fprintln(os.Stderr, "  - records who walked it (human, not model)")
	fmt.Fprintln(os.Stderr, "  - asserts real infrastructure (real_infra: true)")
	fmt.Fprintln(os.Stderr, "  - asserts mocks are off (mocks_off: true)")
	fmt.Fprintln(os.Stderr, "  - records a passing result (status: walked-pass)")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Edit the attestation file at %s and add the missing records.\n",
		journey.AttestationArtefactPath(absRoot))
	return 1
}