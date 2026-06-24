package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/git"
	"github.com/swornagent/sworn/internal/style"
)
func init() {
	command.Register(command.Command{
		Name:    "board",
		Summary: "read the release board from git refs (authoritative slice state)",
		Run:     cmdBoard,
	})
}

func cmdBoard(args []string) int {
	fs := flag.NewFlagSet("board", flag.ExitOnError)
	releaseName := fs.String("release", "", "release name (required)")
	asJSON := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	if *releaseName == "" {
		fmt.Fprintln(os.Stderr, "sworn board: --release is required")
		fmt.Fprintln(os.Stderr, "usage: sworn board --release <name> [--json]")
		return 64
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn board: get cwd: %v\n", err)
		return 2
	}

	repo := git.New(cwd)
	oracle := board.NewGitOracle(repo)

	// Resolve the release-wt ref. Try refs/heads/release-wt/<release> first,
	// then fall back to HEAD.
	releaseRef := "refs/heads/release-wt/" + *releaseName
	exists, err := repo.CatFileExists(releaseRef, "docs/release/"+*releaseName+"/index.md")
	if err != nil || !exists {
		// Try Fumadocs prefix.
		exists2, err2 := repo.CatFileExists(releaseRef, "apps/docs/content/docs/release/"+*releaseName+"/index.md")
		if err2 != nil || !exists2 {
			releaseRef = "HEAD"
		}
	}

	ctx := context.Background()
	boardState, err := oracle.ReadBoard(ctx, oracleReader{repo}, releaseRef, *releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn board: %v\n", err)
		return 2
	}

	if *asJSON {
		return printBoardJSON(boardState)
	}
	return printBoardText(boardState)
}

// oracleReader adapts *git.Repo to board.gitContentReader for ReadBoard.
type oracleReader struct {
	repo *git.Repo
}

func (r oracleReader) Show(ref, path string) (string, error) {
	return r.repo.Show(ref, path)
}

func (r oracleReader) CatFileExists(ref, path string) (bool, error) {
	return r.repo.CatFileExists(ref, path)
}

// boardSliceJSON is the per-slice JSON shape for --json output, matching the
// bash oracle (release-board-status.sh --json).
type boardSliceJSON struct {
	ID              string              `json:"id"`
	State           string              `json:"state"`
	Owner           string              `json:"owner,omitempty"`
	LastUpdated     string              `json:"lastUpdated,omitempty"`
	Track           string              `json:"track"`
	Actionable      bool                `json:"actionable"`
	DependsOnTracks []string            `json:"dependsOnTracks"`
	Blocked         bool                `json:"blocked"`
	BlockedReason   string              `json:"blocked_reason,omitempty"`
	BlockedOwner    string              `json:"blocked_owner,omitempty"`
}

func printBoardJSON(bs *board.BoardState) int {
	type trackJSON struct {
		ID     string           `json:"id"`
		State  string           `json:"state"`
		Slices []boardSliceJSON `json:"slices"`
	}
	type boardJSON struct {
		Release string      `json:"release"`
		Tracks  []trackJSON `json:"tracks"`
	}

	bj := boardJSON{Release: bs.Release}
	for _, t := range bs.Tracks {
		tj := trackJSON{ID: t.ID, State: t.State}
		for _, s := range t.Slices {
			tj.Slices = append(tj.Slices, boardSliceJSON{
				ID:              s.ID,
				State:           string(s.State),
				Owner:           s.Owner,
				LastUpdated:     s.LastUpdated,
				Track:           s.Track,
				Actionable:      s.Actionable,
				DependsOnTracks: s.DependsOnTracks,
				Blocked:         s.Blocked,
				BlockedReason:   s.BlockedReason,
				BlockedOwner:    string(s.BlockedOwner),
			})
		}
		bj.Tracks = append(bj.Tracks, tj)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(bj); err != nil {
		fmt.Fprintf(os.Stderr, "sworn board: json: %v\n", err)
		return 2
	}
	return 0
}

func printBoardText(bs *board.BoardState) int {
	fmt.Println(style.Heading(fmt.Sprintf("Release board: %s", bs.Release)))
	fmt.Println()

	// Sort tracks by ID for stable output.
	sorted := make([]board.TrackState, len(bs.Tracks))
	copy(sorted, bs.Tracks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	for _, t := range sorted {
		fmt.Println(style.Bold(fmt.Sprintf("Track %s — %s", t.ID, t.State)))
		if len(t.Slices) == 0 {
			fmt.Println("  (no slices)")
			fmt.Println()
			continue
		}

		// Sort slices by ID.
		slices := make([]board.SliceState, len(t.Slices))
		copy(slices, t.Slices)
		sort.Slice(slices, func(i, j int) bool { return slices[i].ID < slices[j].ID })

		for _, s := range slices {
			stateStr := string(s.State)
			if s.Blocked {
				// BLOCKED visibility: render distinctly.
				stateStr = style.Danger(fmt.Sprintf("BLOCKED → %s: %s", s.BlockedOwner, s.BlockedReason))
			} else if s.State == "verified" {
				stateStr = style.Accent(stateStr)
			} else if s.State == "failed_verification" {
				stateStr = style.Danger(stateStr)
			}

			ownerStr := ""
			if s.Owner != "" {
				ownerStr = fmt.Sprintf(" [%s]", s.Owner)
			}

			actionableMark := " "
			if s.Actionable {
				actionableMark = "*"
			}

			fmt.Printf("  %s %s — %s%s\n", actionableMark, style.Accent(s.ID), stateStr, ownerStr)
		}
		fmt.Println()
	}
	return 0
}

