package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/journey"
	"github.com/swornagent/sworn/internal/router"
	"github.com/swornagent/sworn/internal/state"
	"github.com/swornagent/sworn/internal/style"
)

func init() {
	command.Register(command.Command{
		Name:    "merge-track",
		Summary: "Merge a verified track into the release assembly branch",
		Run:     cmdMergeTrack,
	})
	command.Register(command.Command{
		Name:    "merge-release",
		Summary: "Merge all tracks and validate the journey gate for a release",
		Run:     cmdMergeRelease,
	})
}

// cmdMergeTrack implements `sworn merge-track <track-id> [--release <name>]`.
//
// Gates:
//  1. All track slices must be verified/deferred/shipped (via oracle).
//  2. Invariant-4 conflict classifier on the release worktree.
//  3. Working-tree cleanliness.
//
// Returns exit codes: 0 = merged, 1 = blocked, 2 = error.
func cmdMergeTrack(args []string) int {
	fs := flag.NewFlagSet("merge-track", flag.ExitOnError)
	releaseName := fs.String("release", "", "Release name (derived from branch if absent)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: sworn merge-track <track-id> [--release <name>]\n")
		return 64
	}

	trackID := fs.Arg(0)
	repo := git.New(".")
	projectRoot := "."

	// Resolve release name.
	rel := *releaseName
	if rel == "" {
		var err error
		rel, err = deriveReleaseFromBranch(repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn merge-track: %v (use --release to specify)\n", err)
			return 2
		}
	}

	releaseRef := "refs/heads/release-wt/" + rel

	// Read the release board via oracle.
	oracleAdapter, err := board.NewOracleReaderAdapterFromRepo(repo, rel, releaseRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: read board: %v\n", err)
		return 2
	}

	boardState, err := oracleAdapter.ReadBoard(context.Background(), rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: read board: %v\n", err)
		return 2
	}

	// Find the track.
	var targetTrack *board.TrackState
	for i := range boardState.Tracks {
		if boardState.Tracks[i].ID == trackID {
			targetTrack = &boardState.Tracks[i]
			break
		}
	}
	if targetTrack == nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: track %q not found in release %q\n", trackID, rel)
		return 2
	}

	trackBranch := targetTrack.WorktreeBranch
	if trackBranch == "" {
		fmt.Fprintf(os.Stderr, "sworn merge-track: track %q has no worktree_branch\n", trackID)
		return 2
	}

	// Gate 1 — all slices must be verified/deferred/shipped.
	var unverified []string
	for _, ss := range targetTrack.Slices {
		if ss.State != state.Verified && ss.State != state.Deferred {
			unverified = append(unverified, fmt.Sprintf("%s: state is %s", ss.ID, ss.State))
		}
	}
	if len(unverified) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", style.Danger(
			fmt.Sprintf("Cannot merge track %q — the following slices are not verified:", trackID)))
		for _, uv := range unverified {
			fmt.Fprintf(os.Stderr, "  - %s\n", uv)
		}
		return 1
	}

	// Gate 2 — invariant-4 classifier on the release worktree.
	releaseWorktreePath, err := oracleAdapter.ReadReleaseWorktreePath(rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: resolve release worktree: %v\n", err)
		return 2
	}
	releaseRepo := git.New(releaseWorktreePath)

	// Rule 11: assert the target worktree is on the expected branch.
	currentBranch, err := releaseRepo.CurrentBranch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: check release branch: %v\n", err)
		return 2
	}
	expectedRef := "release-wt/" + rel
	if currentBranch != expectedRef {
		fmt.Fprintf(os.Stderr, "sworn merge-track: BLOCK: release worktree is on branch %q, expected %q\n",
			currentBranch, expectedRef)
		return 1
	}

	// Gate 3 — working-tree cleanliness on the release worktree.
	porcelain, err := releaseRepo.StatusPorcelain()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: check working tree: %v\n", err)
		return 2
	}
	if strings.TrimSpace(porcelain) != "" {
		fmt.Fprintf(os.Stderr, "sworn merge-track: release worktree is not clean — commit or stash changes first\n")
		return 1
	}

	// Parse documented shared files for invariant-4 check.
	indexPath := filepath.Join(projectRoot, "docs", "release", rel, "index.md")
	docShared, err := router.ParseDocumentedShared(indexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: warning: touchpoint matrix parse: %v — using empty documented-shared set (all conflicts will block)\n", err)
		docShared = make(map[string]bool)
	}
	if err := router.Invariant4Check(releaseRepo, trackBranch, docShared); err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: BLOCK: %v\n", err)
		return 1
	}
	// Execute the merge.
	if err := releaseRepo.Merge(trackBranch); err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-track: merge failed: %v\n", err)
		return 2
	}

	fmt.Println(style.Success(fmt.Sprintf("Track %q merged successfully to release-wt.", trackID)))
	return 0
}

// cmdMergeRelease implements `sworn merge-release [--release <name>]`.
//
// Gates:
//  1. All slices across all tracks must be terminal (verified/deferred/shipped).
//  2. All track branches must be merged into release-wt.
//  3. Journey gate: journeys.json must exist and be ratified.
//
// Returns exit codes: 0 = merged, 1 = blocked, 2 = error.
func cmdMergeRelease(args []string) int {
	fs := flag.NewFlagSet("merge-release", flag.ExitOnError)
	releaseName := fs.String("release", "", "Release name (derived from branch if absent)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	repo := git.New(".")
	projectRoot := "."

	// Resolve release name.
	rel := *releaseName
	if rel == "" {
		var err error
		rel, err = deriveReleaseFromBranch(repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn merge-release: %v (use --release to specify)\n", err)
			return 2
		}
	}

	releaseRef := "refs/heads/release-wt/" + rel

	// Read the full release board via oracle.
	oracleAdapter, err := board.NewOracleReaderAdapterFromRepo(repo, rel, releaseRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-release: read board: %v\n", err)
		return 2
	}

	boardState, err := oracleAdapter.ReadBoard(context.Background(), rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-release: read board: %v\n", err)
		return 2
	}

	// Gate 1 — all slices across all tracks must be terminal.
	var undone []string
	for _, t := range boardState.Tracks {
		for _, ss := range t.Slices {
			switch string(ss.State) {
			case "verified", "shipped", "deferred":
				// terminal
			default:
				undone = append(undone, fmt.Sprintf("%s (%s): %s", ss.ID, t.ID, ss.State))
			}
		}
	}
	if len(undone) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", style.Danger("Cannot merge release — the following slices are not terminal:"))
		for _, u := range undone {
			fmt.Fprintf(os.Stderr, "  - %s\n", u)
		}
		return 1
	}

	// Gate 2 — all track branches must be ancestors of release-wt.
	releaseWorktreePath, err := oracleAdapter.ReadReleaseWorktreePath(rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-release: resolve release worktree: %v\n", err)
		return 2
	}
	releaseRepo := git.New(releaseWorktreePath)

	for _, t := range boardState.Tracks {
		if t.WorktreeBranch == "" {
			continue
		}
		ancestor, err := releaseRepo.IsAncestor(t.WorktreeBranch, "HEAD")
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn merge-release: check ancestor %s: %v\n", t.WorktreeBranch, err)
			return 2
		}
		if !ancestor {
			fmt.Fprintf(os.Stderr, "sworn merge-release: track %q is not merged into release-wt — run `sworn merge-track %s` first\n", t.ID, t.ID)
			return 1
		}
	}

	// Gate 3 — journey gate: journeys.json must exist and be ratified.
	result, _, err := journey.Check(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn merge-release: journey check: %v\n", err)
		return 2
	}
	switch result {
	case journey.CheckMissing:
		fmt.Fprintf(os.Stderr, "sworn merge-release: BLOCK: no ratified journeys.json — Rule 10 gate\n")
		return 1
	case journey.CheckUnratified:
		fmt.Fprintf(os.Stderr, "sworn merge-release: BLOCK: no ratified journeys.json — Rule 10 gate\n")
		return 1
	}

	fmt.Println(style.Success(fmt.Sprintf("Release %q passed all merge gates — ready to ship.", rel)))
	return 0
}

// deriveReleaseFromBranch parses the release name from the current branch.
// Expects the branch to be in the format track/<release>/<track-id> or
// release-wt/<release>.
func deriveReleaseFromBranch(repo *git.Repo) (string, error) {
	branch, err := repo.CurrentBranch()
	if err != nil {
		return "", fmt.Errorf("cannot determine current branch: %w", err)
	}
	// track/<release>/<track-id>
	if strings.HasPrefix(branch, "track/") {
		parts := strings.SplitN(branch, "/", 3)
		if len(parts) >= 2 {
			return parts[1], nil
		}
	}
	// release-wt/<release>
	if strings.HasPrefix(branch, "release-wt/") {
		return strings.TrimPrefix(branch, "release-wt/"), nil
	}
	return "", fmt.Errorf("cannot derive release from branch %q", branch)
}
