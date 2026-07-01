// Package journey — Impact analysis: given a ratified journeys artefact and
// a release, compute which critical journeys the release touches (derived from
// the release's slice planned/actual files and the journeys' step surfaces).
//
// The mapping is heuristic and biased toward over-inclusion: a journey is in
// scope if any of its step surfaces or entry surface textually matches any
// slice's planned or actual files. This errs safe — the walkthrough scope is
// wider than necessary rather than narrower.
package journey

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ImpactResult holds the result of impact analysis for a release.
type ImpactResult struct {
	// ArtefactFound is true when the journeys artefact exists at the project root.
	ArtefactFound bool `json:"artefact_found"`

	// IsRatified is true when the journeys artefact is human-ratified.
	IsRatified bool `json:"is_ratified"`

	// JourneysTouched is the set of journey IDs that the release touches,
	// sorted alphabetically.
	JourneysTouched []string `json:"journeys_touched"`

	// AllJourneyIDs is the full set of journey IDs from the artefact,
	// sorted alphabetically.
	AllJourneyIDs []string `json:"all_journey_ids"`

	// ReleaseName is the name of the release analysed (the base name of
	// the release directory).
	ReleaseName string `json:"release_name"`
}

// ImpactError is a structured error returned when impact analysis cannot
// proceed due to a missing or unratified journeys artefact. Callers can
// type-assert to distinguish from I/O or parse errors.
type ImpactError struct {
	Result  CheckResult
	Message string
}

func (e *ImpactError) Error() string { return e.Message }

// AnalyzeImpact computes which critical journeys a release touches.
//
// projectRoot is the root of the project (to find .sworn/journeys.json).
// releaseDir is the absolute path to docs/release/<release-name>/.
//
// Returns an ImpactResult on success, *ImpactError if the journeys artefact
// is missing or unratified, or a generic error for I/O / parse failures.
func AnalyzeImpact(projectRoot, releaseDir string) (*ImpactResult, error) {
	// 1. Check journeys artefact.
	result, artefact, err := Check(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("journeys check: %w", err)
	}

	switch result {
	case CheckMissing:
		return nil, &ImpactError{
			Result: CheckMissing,
			Message: "no journeys artefact found at " + JourneyArtefactPath(projectRoot) +
				" — run 'sworn journeys <project>' to elicit journeys first (S11)",
		}
	case CheckUnratified:
		return nil, &ImpactError{
			Result: CheckUnratified,
			Message: "journeys artefact exists but is NOT human-ratified at " +
				JourneyArtefactPath(projectRoot) +
				" — ratify before running impact analysis",
		}
	}

	// 2. Collect slice touchpoints from the release directory.
	touchpoints, err := collectSliceTouchpoints(releaseDir)
	if err != nil {
		return nil, fmt.Errorf("collecting slice touchpoints: %w", err)
	}

	// 3. Match journeys to touchpoints using heuristic surface matching.
	touched := matchJourneysToTouchpoints(artefact.Journeys, touchpoints)

	// 4. Build the full sorted list of journey IDs.
	allIDs := make([]string, 0, len(artefact.Journeys))
	for _, j := range artefact.Journeys {
		allIDs = append(allIDs, j.ID)
	}
	sort.Strings(allIDs)

	// 5. Sort touched IDs for deterministic output.
	sort.Strings(touched)

	releaseName := filepath.Base(releaseDir)

	return &ImpactResult{
		ArtefactFound:   true,
		IsRatified:      artefact.Ratification.IsRatified,
		JourneysTouched: touched,
		AllJourneyIDs:   allIDs,
		ReleaseName:     releaseName,
	}, nil
}

// SliceTouchpoint holds a slice's surface signatures derived from its
// planned_files + actual_files in the release board.
type SliceTouchpoint struct {
	// SliceID is the slice identifier (e.g. "S12-journey-impact-analysis").
	SliceID string `json:"slice_id"`

	// Files is the union of planned_files and actual_files from the slice's
	// status.json.
	Files []string `json:"files"`
}

// collectSliceTouchpoints reads all slice directories under the release
// directory and collects their planned + actual file lists.
func collectSliceTouchpoints(releaseDir string) ([]SliceTouchpoint, error) {
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("release directory not found: %s", releaseDir)
		}
		return nil, fmt.Errorf("reading release directory %s: %w", releaseDir, err)
	}

	var touchpoints []SliceTouchpoint
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), "S") {
			continue
		}

		statusPath := filepath.Join(releaseDir, e.Name(), "status.json")
		data, readErr := os.ReadFile(statusPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue // slice dir exists but no status.json yet — skip
			}
			return nil, fmt.Errorf("reading %s: %w", statusPath, readErr)
		}

		var status struct {
			SliceID      string   `json:"slice_id"`
			PlannedFiles []string `json:"planned_files"`
			ActualFiles  []string `json:"actual_files"`
		}
		if parseErr := json.Unmarshal(data, &status); parseErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", statusPath, parseErr)
		}

		// Merge planned and actual files (deduplicate).
		seen := make(map[string]bool)
		var files []string
		for _, f := range status.PlannedFiles {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
		for _, f := range status.ActualFiles {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}

		if len(files) > 0 {
			touchpoints = append(touchpoints, SliceTouchpoint{
				SliceID: status.SliceID,
				Files:   files,
			})
		}
	}

	// Sort by slice ID for deterministic output.
	sort.Slice(touchpoints, func(i, j int) bool {
		return touchpoints[i].SliceID < touchpoints[j].SliceID
	})

	return touchpoints, nil
}

// matchJourneysToTouchpoints determines which journeys are touched by the
// release's slices. A journey is "touched" when any of its step surfaces or
// entry surface heuristically matches any slice's file touchpoints.
//
// The heuristic errs toward over-inclusion (spec Risk mitigation): a journey
// is in scope if any of its surfaces textually overlaps with any touchpoint
// file. This means the walkthrough scope is wider than necessary rather than
// narrower.
func matchJourneysToTouchpoints(journeys []Journey, touchpoints []SliceTouchpoint) []string {
	touched := make(map[string]bool) // set of journey IDs

	for _, j := range journeys {
		// Collect all surfaces from this journey: entry + every step.
		surfaces := make([]string, 0, 1+len(j.Steps))
		if j.EntrySurface != "" {
			surfaces = append(surfaces, j.EntrySurface)
		}
		for _, step := range j.Steps {
			if step.Surface != "" {
				surfaces = append(surfaces, step.Surface)
			}
		}

		if len(surfaces) == 0 {
			continue // a journey with no surfaces cannot be matched
		}

		// Check if any touchpoint file matches any surface.
	journeyLoop:
		for _, tp := range touchpoints {
			for _, file := range tp.Files {
				for _, surface := range surfaces {
					if surfacesTouch(file, surface) {
						touched[j.ID] = true
						break journeyLoop
					}
				}
			}
		}
	}

	result := make([]string, 0, len(touched))
	for id := range touched {
		result = append(result, id)
	}
	return result
}

// surfacesTouch reports whether a file path heuristically touches a journey
// surface. Uses multi-level matching biased toward over-inclusion:
//
//  1. Direct substring match (either direction) — normalised to lowercase.
//  2. Token-level equality or containment: split both path and surface into
//     alphanumeric tokens and check for matches.
//  3. Conventional mapping: "cli" (from surface "CLI") maps to files in "cmd/".
//
// Examples:
//
//	("cmd/sworn/journeys.go", "sworn init")	→ true  (token "sworn" matches)
//	("cmd/sworn/journeys.go", "CLI")	→ true  (conventional: cli ↔ cmd)
//	("internal/verify/verify.go", "verify")	→ true  (direct substring)
//	("docs/release/...", "sworn top")	→ false (no overlap)
func surfacesTouch(filePath, surface string) bool {
	f := strings.ToLower(filePath)
	s := strings.ToLower(surface)

	// Level 1: Direct substring match (either direction).
	if strings.Contains(f, s) || strings.Contains(s, f) {
		return true
	}

	// Level 2: Token-level matching.
	fTokens := tokenize(f)
	sTokens := tokenize(s)

	for _, ft := range fTokens {
		for _, st := range sTokens {
			if ft == st || strings.Contains(ft, st) || strings.Contains(st, ft) {
				return true
			}
		}
	}

	// Level 3: Conventional mapping — "cli" surface -> "cmd/" directory.
	for _, st := range sTokens {
		if st == "cli" {
			for _, ft := range fTokens {
				if ft == "cmd" {
					return true
				}
			}
		}
	}

	return false
}

// tokenize splits a string into contiguous alphanumeric tokens.
// Non-alphanumeric characters (slashes, dots, spaces, hyphens, etc.) act as
// delimiters. Both uppercase and lowercase letters are recognised.
func tokenize(s string) []string {
	var tokens []string
	start := -1
	for i, r := range s {
		if isAlphaNum(r) {
			if start < 0 {
				start = i
			}
		} else {
			if start >= 0 {
				tokens = append(tokens, s[start:i])
				start = -1
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, s[start:])
	}
	return tokens
}

// isAlphaNum reports whether r is an ASCII letter or digit.
func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
