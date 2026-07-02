package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/journey"
	"github.com/swornagent/sworn/internal/style"
	"github.com/swornagent/sworn/internal/tui"
)

// cmdTop implements `sworn top <release> [project-path]`.
//
// With no release argument, it launches the TUI (same as `sworn` with no args).
// With a release argument, it renders a read-only evidence surface for the
// active release: each critical journey in scope with its walkthrough
// validation status, assembled into a green-board when all pass or a kill-list
// when any fail or are un-walked.
//
// This is strictly read-only — no state transitions, no artefact writes.
// Returns exit 0 on green-board, 1 on kill-list, 2 on unrecoverable error,
// 64 on usage error.
func cmdTop(args []string) int {
	fs := flag.NewFlagSet("top", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		// No release arg — launch TUI instead of rendering evidence surface.
		if err := tui.Run(version); err != nil {
			fmt.Fprintf(os.Stderr, "sworn top: %v\n", err)
			return 1
		}
		return 0
	}
	releaseName := fs.Arg(0)
	projectRoot := "."
	if fs.NArg() > 1 {
		projectRoot = fs.Arg(1)
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn top: resolve project path: %v\n", err)
		return 2
	}

	return renderEvidenceSurface(releaseName, absRoot)
}

// renderEvidenceSurface loads journeys and attestations, then renders
// the evidence surface to stdout. Returns 0 on green-board, 1 on kill-list.
func renderEvidenceSurface(releaseName string, projectRoot string) int {
	// Load journeys artefact.
	artefact, err := journey.LoadArtefact(projectRoot)
	if err != nil {
		// Missing artefact — render empty state.
		if isJourneyNotExist(err) {
			fmt.Println(style.Heading(fmt.Sprintf("Evidence surface for release %s", releaseName)))
			fmt.Println(style.Dim("────────────────────────────────────────────────────"))
			fmt.Println("No journeys artefact found.")
			fmt.Println()
			fmt.Printf("  Hint: run 'sworn journeys %s' to start journey elicitation.\n", projectRoot)
			fmt.Println()
			fmt.Println("(No evidence to display until journeys are elicited and ratified.)")
			return 0
		}
		fmt.Fprintf(os.Stderr, "sworn top: load journeys: %v\n", err)
		return 2
	}

	// Load attestations artefact (optional — defaults to all un-walked).
	attestations, err := journey.LoadAttestations(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn top: load attestations: %v\n", err)
		return 2
	}

	journeyList := artefact.Journeys

	// Render header.
	fmt.Println(style.Heading(fmt.Sprintf("Evidence surface for release %s", releaseName)))
	fmt.Printf("%d journey(s) in scope\n", len(journeyList))
	fmt.Println(style.Dim("────────────────────────────────────────────────────"))

	if len(journeyList) == 0 {
		fmt.Println()
		fmt.Println("No journeys in scope for this release.")
		fmt.Println("  Journeys artefact exists but contains no journeys.")
		return 0
	}

	// Walk each journey and collect status.
	type entry struct {
		id     string
		status journey.WalkStatus
	}
	entries := make([]entry, 0, len(journeyList))
	failCount := 0
	for _, j := range journeyList {
		status := journey.AttestationStatus(attestations, j.ID)
		entries = append(entries, entry{id: j.ID, status: status})
		if status != journey.WalkPass {
			failCount++
		}
	}

	// Render each journey.
	fmt.Println()
	for _, e := range entries {
		switch e.status {
		case journey.WalkPass:
			fmt.Printf("  ✓  %s: walked-pass\n", e.id)
		case journey.WalkFail:
			fmt.Printf("  ✗  %s: walked-fail\n", e.id)
		default:
			fmt.Printf("  ✗  %s: un-walked\n", e.id)
		}
	}

	// Render summary.
	fmt.Println()
	if failCount == 0 {
		// Green-board
		fmt.Println(style.Success(fmt.Sprintf("Green-board ✓  All %d journey(s) validated.", len(entries))))
		return 0
	}

	// Kill-list
	fmt.Println(style.Danger(fmt.Sprintf("Kill-list ✗  %d journey(s) need human walkthrough or re-validation:", failCount)))
	for _, e := range entries {
		if e.status != journey.WalkPass {
			switch e.status {
			case journey.WalkFail:
				fmt.Printf("    - %s (walked-fail — defect found)\n", e.id)
			default:
				fmt.Printf("    - %s (un-walked — no attestation recorded)\n", e.id)
			}
		}
	}
	fmt.Println()
	fmt.Println("  After S13: run walkthrough and record attestation via `sworn ship`.")
	return 1
}

// isJourneyNotExist reports whether err is the artefact-not-exist sentinel.
func isJourneyNotExist(err error) bool {
	if err == nil {
		return false
	}
	for e := err; e != nil; {
		if e == journey.ErrArtefactNotExist {
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
