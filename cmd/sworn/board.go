package main

import (
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
	releaseName := fs.String("release", "", "release name (optional filter)")
	asJSON := fs.Bool("json", false, "output as JSON")
	_ = fs.Parse(args)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn board: get cwd: %v\n", err)
		return 2
	}

	repo := git.New(cwd)
	catalog, err := board.DiscoverCatalog(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn board: %v\n", err)
		return 2
	}
	if *releaseName == "" {
		if *asJSON {
			return printCatalogJSON(catalog)
		}
		for _, record := range catalog {
			fmt.Printf("Source ref: %s\n", record.SourceRef)
			if code := printBoardText(record.Board); code != 0 {
				return code
			}
		}
		return 0
	}
	var boardState *board.BoardState
	for _, record := range catalog {
		if record.Release == *releaseName {
			boardState = record.Board
			break
		}
	}
	if boardState == nil {
		fmt.Fprintf(os.Stderr, "sworn board: release %q not found\n", *releaseName)
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
	ID              string   `json:"id"`
	State           string   `json:"state"`
	Owner           string   `json:"owner,omitempty"`
	LastUpdated     string   `json:"lastUpdated,omitempty"`
	Track           string   `json:"track"`
	Actionable      bool     `json:"actionable"`
	DependsOnTracks []string `json:"dependsOnTracks"`
	Blocked         bool     `json:"blocked"`
	BlockedReason   string   `json:"blocked_reason,omitempty"`
	BlockedOwner    string   `json:"blocked_owner,omitempty"`
	StateSource     string   `json:"stateSource"`
	StateDurability string   `json:"stateDurability"`
}

type trackJSON struct {
	ID     string           `json:"id"`
	State  string           `json:"state"`
	Slices []boardSliceJSON `json:"slices"`
}

func projectTracks(bs *board.BoardState) []trackJSON {
	var tracks []trackJSON
	for _, t := range bs.Tracks {
		tj := trackJSON{ID: t.ID, State: t.State}
		for _, s := range t.Slices {
			tj.Slices = append(tj.Slices, boardSliceJSON{ID: s.ID, State: string(s.State), Owner: s.Owner, LastUpdated: s.LastUpdated, Track: s.Track, Actionable: s.Actionable, DependsOnTracks: s.DependsOnTracks, Blocked: s.Blocked, BlockedReason: s.BlockedReason, BlockedOwner: string(s.BlockedOwner), StateSource: s.StateSource, StateDurability: s.StateDurability})
		}
		tracks = append(tracks, tj)
	}
	return tracks
}

func printCatalogJSON(catalog []board.CatalogRecord) int {
	type entry struct {
		Release   string      `json:"release"`
		SourceRef string      `json:"sourceRef"`
		Tracks    []trackJSON `json:"tracks"`
	}
	out := struct {
		Releases map[string]entry `json:"releases"`
	}{Releases: map[string]entry{}}
	for _, r := range catalog {
		out.Releases[r.Release] = entry{Release: r.Release, SourceRef: r.SourceRef, Tracks: projectTracks(r.Board)}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "sworn board: json: %v\n", err)
		return 2
	}
	return 0
}

func printBoardJSON(bs *board.BoardState) int {
	type boardJSON struct {
		Release string      `json:"release"`
		Tracks  []trackJSON `json:"tracks"`
	}

	bj := boardJSON{Release: bs.Release}
	bj.Tracks = projectTracks(bs)

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
			if s.StateDurability == "uncommitted" {
				stateStr += " [uncommitted]"
			}
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
