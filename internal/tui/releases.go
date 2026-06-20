package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/state"
	"gopkg.in/yaml.v3"
)

// ReleaseInfo holds metadata about one release for the list view.
type ReleaseInfo struct {
	ID          string // directory name, e.g. "2026-06-19-safe-parallelism"
	Name        string `yaml:"title"` // display name from frontmatter
	TrackCount  int
	SliceStates map[string]int // state -> count, for aggregation
}
// ReleasesList is a Bubble Tea component embedded in the root model.
// It holds all discovered releases and a cursor for navigation.
type ReleasesList struct {
	Releases []ReleaseInfo
	Cursor   int
}

// ErrNoReleases indicates no releases were found.
var ErrNoReleases = fmt.Errorf("no releases found under docs/release/")

// LoadReleases scans docs/release/*/index.md and populates the ReleasesList.
// repoRoot is the path to the git repo root (from git rev-parse --show-toplevel).
func (r *ReleasesList) LoadReleases(repoRoot string) error {
	pattern := filepath.Join(repoRoot, "docs", "release", "*", "index.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("scanning releases: %w", err)
	}
	if len(matches) == 0 {
		return ErrNoReleases
	}

	var releases []ReleaseInfo
	for _, path := range matches {
		rel, err := parseReleaseIndex(path)
		if err != nil {
			// Skip unparseable releases — log but don't block.
			continue
		}
		releases = append(releases, rel)
	}
	if len(releases) == 0 {
		return ErrNoReleases
	}

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Name < releases[j].Name
	})

	r.Releases = releases
	if r.Cursor >= len(r.Releases) {
		r.Cursor = len(r.Releases) - 1
	}
	return nil
}

// parseReleaseIndex reads an index.md file and returns ReleaseInfo.
// It extracts the frontmatter title and walks slices/status.json files
// for track count and state aggregation.
func parseReleaseIndex(path string) (ReleaseInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReleaseInfo{}, err
	}

	// Extract YAML frontmatter (between --- markers).
	var info ReleaseInfo
	// Set ID from the directory name (parent of index.md).
	info.ID = filepath.Base(filepath.Dir(path))
	frontmatter, _, _ := strings.Cut(string(data), "---")	// Walk past opening ---.
	if trimmed := strings.TrimSpace(frontmatter); trimmed == "" {
		// First --- at start: find second ---
		rest := string(data)
		if strings.HasPrefix(rest, "---\n") || strings.HasPrefix(rest, "---\r\n") {
			rest = rest[4:]
		}
		parts := strings.SplitN(rest, "---", 2)
		if len(parts) >= 1 {
			frontmatter = parts[0]
		}
	}

	if err := yaml.Unmarshal([]byte(frontmatter), &info); err != nil {
		// Fallback: use directory name as release name.
		info.Name = filepath.Base(filepath.Dir(path))
	}

	// Count tracks: index.md has a `tracks:` list in frontmatter.
	// But the real source is the directory structure.
	releaseDir := filepath.Dir(path)
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return info, nil // return partial info
	}

	info.SliceStates = map[string]int{}
	trackIDs := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "S") {
			// This is a slice directory — read its status.json.
			statusPath := filepath.Join(releaseDir, entry.Name(), "status.json")
			st, errR := state.Read(statusPath)
			if errR != nil {
				continue
			}
			stateStr := string(st.State)
			info.SliceStates[stateStr]++
			if st.Track != "" {
				trackIDs[st.Track] = true
			}
		}
	}
	info.TrackCount = len(trackIDs)

	return info, nil
}

// AggregatedState returns the dominant state across all slices.
func (r ReleaseInfo) AggregatedState() string {
	// Priority order for display: blocked > failed > in_progress > design_review > verified > planned
	priority := []string{"blocked", "failed_verification", "in_progress", "design_review", "verified", "planned"}
	for _, s := range priority {
		if count, ok := r.SliceStates[s]; ok && count > 0 {
			return s
		}
	}
	return "planned"
}

// View renders the releases list pane.
func (r *ReleasesList) View() string {
	if len(r.Releases) == 0 {
		return ReleaseListTitle.Render("Releases") + "\n" +
			EmptyMessage.Render("No releases found")
	}

	var b strings.Builder
	b.WriteString(ReleaseListTitle.Render("Releases"))
	b.WriteString("\n")

	for i, rel := range r.Releases {
		stateStr := rel.AggregatedState()
		label := fmt.Sprintf("%s  %s (%d tracks, %s)",
			rel.Name,
			Divider,
			rel.TrackCount,
			stateStr,
		)
		if i == r.Cursor {
			b.WriteString(ReleaseItemSelected.Render("▸ " + label))
		} else {
			b.WriteString(ReleaseItem.Render("  " + label))
		}
		b.WriteString("\n")
	}
	return b.String()
}