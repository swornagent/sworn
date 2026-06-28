// Package board validates the structural integrity of release-board index.md
// files. This file (board.go) provides the board.json read/write layer —
// the oracle's source of truth for track metadata, replacing the YAML
// frontmatter extraction from index.md (ADR-0009).
//
// Pure stdlib — zero third-party dependencies.
package board

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
)

// BoardRecord is the on-disk representation of a board.json file.
// It mirrors the index.md YAML frontmatter but in typed JSON form.
type BoardRecord struct {
	SchemaVersion         int          `json:"schema_version"`
	Release               string       `json:"release"`
	ReleaseWorktreePath   string       `json:"release_worktree_path,omitempty"`
	ReleaseWorktreeBranch string       `json:"release_worktree_branch,omitempty"`
	Tracks                []BoardTrack `json:"tracks"`
}

// BoardTrack is one track entry in a BoardRecord.
type BoardTrack struct {
	ID             string   `json:"id"`
	Slices         []string `json:"slices"`
	DependsOn      []string `json:"depends_on,omitempty"`
	WorktreePath   string   `json:"worktree_path,omitempty"`
	WorktreeBranch string   `json:"worktree_branch"`
	State          string   `json:"state"`
}

// ReadBoard reads board.json from docs/release/<release>/board.json. If the
// file does not exist, it performs a lazy migration: reads the index.md
// frontmatter, builds a BoardRecord from it, and writes board.json so
// subsequent reads hit the JSON path.
func ReadBoard(repoRoot, release string) (*BoardRecord, error) {
	boardPath := filepath.Join(repoRoot, "docs", "release", release, "board.json")
	data, err := os.ReadFile(boardPath)
	if err == nil {
		var br BoardRecord
		if err := json.Unmarshal(data, &br); err != nil {
			return nil, fmt.Errorf("parse board.json: %w", err)
		}
		return &br, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read board.json: %w", err)
	}

	// Lazy migration: generate from index.md frontmatter.
	return migrateFromIndex(repoRoot, release)
}

// WriteBoard writes board.json to disk, validates it against the board-v1
// schema, then performs an advisory drift check against index.md. The drift
// guard is advisory only (warning); it does not block the write.
func WriteBoard(repoRoot, release string, br *BoardRecord) error {
	// Set schema metadata.
	br.SchemaVersion = 1
	if br.Tracks == nil {
		br.Tracks = []BoardTrack{}
	}

	data, err := json.MarshalIndent(br, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal board.json: %w", err)
	}

	boardPath := filepath.Join(repoRoot, "docs", "release", release, "board.json")
	if err := os.MkdirAll(filepath.Dir(boardPath), 0755); err != nil {
		return fmt.Errorf("mkdir for board.json: %w", err)
	}
	if err := os.WriteFile(boardPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write board.json: %w", err)
	}

	// Validate the written board.json.
	if err := baton.Validate("board-v1", data); err != nil {
		return fmt.Errorf("validate board.json: %w", err)
	}

	// Advisory drift guard: compare with index.md frontmatter.
	driftGuard(repoRoot, release, br)

	return nil
}

// migrateFromIndex reads the index.md frontmatter from the filesystem, parses
// it into a BoardRecord, and writes board.json. Called by ReadBoard when
// board.json does not exist.
func migrateFromIndex(repoRoot, release string) (*BoardRecord, error) {
	indexPath := filepath.Join(repoRoot, "docs", "release", release, "index.md")
	rawIndex, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("lazy migration: read index.md: %w", err)
	}

	fmBody := extractFrontmatterBody(string(rawIndex))
	trackInfos := ParseTracks(fmBody)

	// Extract release-level fields from frontmatter.
	releaseWTPath := ""
	releaseWTBranch := ""
	vt := ParseVerticalTrace(string(rawIndex)) // re-parse full text for vertical trace
	_ = vt                                      // vertical trace not stored in board.json

	// Extract release_worktree_path and release_worktree_branch from frontmatter.
	for _, line := range strings.Split(fmBody, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "release_worktree_path:") {
			releaseWTPath = strings.TrimSpace(strings.TrimPrefix(line, "release_worktree_path:"))
		}
		if strings.HasPrefix(line, "release_worktree_branch:") {
			releaseWTBranch = strings.TrimSpace(strings.TrimPrefix(line, "release_worktree_branch:"))
		}
	}

	br := &BoardRecord{
		SchemaVersion:         1,
		Release:               release,
		ReleaseWorktreePath:   releaseWTPath,
		ReleaseWorktreeBranch: releaseWTBranch,
		Tracks:                trackInfosToBoardTracks(trackInfos),
	}

	// Write the migrated board.json.
	if err := WriteBoard(repoRoot, release, br); err != nil {
		return nil, fmt.Errorf("lazy migration: write board.json: %w", err)
	}

	return br, nil
}

// driftGuard reads the current index.md frontmatter and compares its tracks
// section against the BoardRecord. If they differ, it logs a warning.
// This is advisory only — it does not return an error.
func driftGuard(repoRoot, release string, br *BoardRecord) {
	indexPath := filepath.Join(repoRoot, "docs", "release", release, "index.md")
	rawIndex, err := os.ReadFile(indexPath)
	if err != nil {
		log.Printf("board drift guard: cannot read index.md: %v", err)
		return
	}

	fmBody := extractFrontmatterBody(string(rawIndex))
	indexTracks := ParseTracks(fmBody)

	brTracks := boardTracksToTrackInfos(br.Tracks)

	if len(indexTracks) != len(brTracks) {
		log.Printf("board drift guard: index.md has %d tracks, board.json has %d — index.md frontmatter is stale; re-render it from board.json",
			len(indexTracks), len(brTracks))
		return
	}

	for i, it := range indexTracks {
		bt := brTracks[i]
		if it.ID != bt.ID {
			log.Printf("board drift guard: track %d id mismatch: index.md=%q, board.json=%q", i, it.ID, bt.ID)
			return
		}
		if it.State != bt.State {
			log.Printf("board drift guard: track %q state mismatch: index.md=%q, board.json=%q", it.ID, it.State, bt.State)
			return
		}
		if len(it.Slices) != len(bt.Slices) {
			log.Printf("board drift guard: track %q slice count mismatch: index.md=%d, board.json=%d", it.ID, len(it.Slices), len(bt.Slices))
			return
		}
	}
}

// trackInfosToBoardTracks converts internal TrackInfo structs to BoardTrack.
func trackInfosToBoardTracks(tis []TrackInfo) []BoardTrack {
	tracks := make([]BoardTrack, len(tis))
	for i, ti := range tis {
		tracks[i] = BoardTrack{
			ID:             ti.ID,
			Slices:         ti.Slices,
			DependsOn:      ti.DependsOn,
			WorktreePath:   ti.WorktreePath,
			WorktreeBranch: ti.WorktreeBranch,
			State:          ti.State,
		}
	}
	return tracks
}

// boardTracksToTrackInfos converts BoardTrack to internal TrackInfo structs.
func boardTracksToTrackInfos(tracks []BoardTrack) []TrackInfo {
	tis := make([]TrackInfo, len(tracks))
	for i, bt := range tracks {
		tis[i] = TrackInfo{
			ID:             bt.ID,
			Slices:         bt.Slices,
			DependsOn:      bt.DependsOn,
			WorktreePath:   bt.WorktreePath,
			WorktreeBranch: bt.WorktreeBranch,
			State:          bt.State,
		}
	}
	return tis
}