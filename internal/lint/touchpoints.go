package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/state"
)

// touchpointViolation represents a single touchpoint check failure.
type touchpointViolation struct {
	Kind     string // "undeclared", "collision", "migration"
	Detail   string // human-readable description
}

// Violations is a collection of touchpoint violations, implementing error.
type touchpointViolations []touchpointViolation

func (vs touchpointViolations) Error() string {
	var parts []string
	for _, v := range vs {
		parts = append(parts, fmt.Sprintf("%s: %s", v.Kind, v.Detail))
	}
	return strings.Join(parts, "\n")
}

// touchpointNote is an informational note (not a violation).
type touchpointNote struct {
	Detail string
}

// touchpointResult holds the output of a touchpoint check.
type touchpointResult struct {
	Violations touchpointViolations
	Notes      []touchpointNote
}

// HasViolations reports whether any fail-closed violations were found.
func (r *touchpointResult) HasViolations() bool {
	return len(r.Violations) > 0
}

// Error returns all violations as a newline-separated string, or nil if none.
func (r *touchpointResult) Error() error {
	if !r.HasViolations() {
		return nil
	}
	return r.Violations
}

// backtickPathRe matches back-ticked tokens that look like file paths or
// package references: they contain a '/' (package ref like internal/lint)
// or end in a known extension (.go, .ts, .tsx, .md).
var backtickPathRe = regexp.MustCompile("`([^`]+)`")

// knownExtensions lists file extensions we recognise as source/artefact paths.
var knownExtensions = []string{".go", ".ts", ".tsx", ".md"}

// isFilePath returns true if the token looks like a file or package path.
// It filters out Go package patterns ("internal/..."), template placeholders
// ("docs/<release>/..."), and bare extension descriptors (".go", ".ts").
func isFilePath(token string) bool {
	// Bare extension descriptors like ".go", ".ts", ".md".
	if strings.HasPrefix(token, ".") && len(token) <= 4 {
		for _, ext := range knownExtensions {
			if token == ext {
				return false
			}
		}
	}
	// Go package patterns with ellipsis.
	if strings.Contains(token, "...") {
		return false
	}
	// Template placeholders.
	if strings.Contains(token, "<") || strings.Contains(token, ">") {
		return false
	}
	if strings.Contains(token, "/") {
		return true
	}
	for _, ext := range knownExtensions {
		if strings.HasSuffix(token, ext) {
			return true
		}
	}
	return false
}
// migrationPrefixRe matches a 6-digit numeric prefix followed by an underscore
// (e.g. "000012_" in "000012_create_users.sql").
var migrationPrefixRe = regexp.MustCompile(`\b(\d{6})_`)

// CheckTouchpoints verifies that a slice's spec references only files/packages
// declared in its planned_files, that no cross-slice file collision exists in
// the release touchpoint matrix, and that no two slices share a migration number.
//
// Arguments:
//   - sliceDir: path to the slice directory (contains spec.md + status.json)
//   - releaseDir: path to the release directory (contains index.md + slice dirs)
//
// Returns nil on pass, error naming violations on fail (fail-closed).
func CheckTouchpoints(sliceDir, releaseDir string) error {
	st, err := state.Read(filepath.Join(sliceDir, "status.json"))
	if err != nil {
		return fmt.Errorf("lint touchpoints: reading status.json: %w", err)
	}

	result := &touchpointResult{}

	// 1. Extract file/package references from spec.md sections.
	specPath := filepath.Join(sliceDir, "spec.md")
	refs, err := extractSectionRefs(specPath)
	if err != nil {
		return fmt.Errorf("lint touchpoints: reading spec: %w", err)
	}

	// Build planned_files set for quick lookup.
	plannedSet := make(map[string]bool, len(st.PlannedFiles))
	for _, p := range st.PlannedFiles {
		plannedSet[p] = true
	}

	// Check each reference against planned_files.
	for _, ref := range refs {
		if !plannedSet[ref] && !plannedFilesContainPrefix(plannedSet, ref) {
			result.Violations = append(result.Violations, touchpointViolation{
				Kind:   "undeclared",
				Detail: fmt.Sprintf("%s referenced in spec but not in planned_files", ref),
			})
		}
	}

	// 2. Parse touchpoint matrix for cross-slice collisions.
	indexPath := filepath.Join(releaseDir, "index.md")
	collisions, sharedFiles, err := parseTouchpointMatrix(indexPath, st.Track, st.PlannedFiles)
	if err != nil {
		return fmt.Errorf("lint touchpoints: parsing touchpoint matrix: %w", err)
	}
	result.Violations = append(result.Violations, collisions...)

	// Report DOCUMENTED SHARED files as informational notes.
	for _, sf := range sharedFiles {
		result.Notes = append(result.Notes, touchpointNote{
			Detail: fmt.Sprintf("DOCUMENTED SHARED file %s appears in planned_files — verify additive-only change (non-additive detection is a Rule 2 deferral, Coach must decide ownership)", sf),
		})
	}

	// 3. Detect duplicate migration numbers.
	migDups, err := detectDuplicateMigrations(releaseDir)
	if err != nil {
		return fmt.Errorf("lint touchpoints: checking migrations: %w", err)
	}
	result.Violations = append(result.Violations, migDups...)

	if result.HasViolations() {
		return result.Violations
	}
	return nil
}

// plannedFilesContainPrefix returns true if any planned file starts with the
// given prefix (e.g. "internal/lint" matches "internal/lint/touchpoints.go").
// For tokens without a "/", also checks suffix matching (e.g. "touchpoints.go"
// matches "internal/lint/touchpoints.go").
func plannedFilesContainPrefix(plannedSet map[string]bool, prefix string) bool {
	for p := range plannedSet {
		if strings.HasPrefix(p, prefix) || strings.HasPrefix(p, prefix+"/") {
			return true
		}
		// Suffix match for bare filenames (tokens without "/").
		if !strings.Contains(prefix, "/") {
			if p == prefix || strings.HasSuffix(p, "/"+prefix) {
				return true
			}
		}
	}
	return false
}
// extractSectionRefs extracts back-ticked file/package references from the
// "## In scope" and "## Planned touchpoints" sections of a spec.md file.
// Only back-ticked tokens that look like paths (contain '/' or a known extension)
// are returned.
func extractSectionRefs(specPath string) ([]string, error) {
	f, err := os.Open(specPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sections, err := extractSections(f)
	if err != nil {
		return nil, err
	}

	targetSections := []string{"In scope", "Planned touchpoints"}
	var refs []string
	seen := make(map[string]bool)

	for _, sec := range targetSections {
		body, ok := sections[sec]
		if !ok {
			continue
		}
		for _, match := range backtickPathRe.FindAllStringSubmatch(body, -1) {
			token := match[1]
			if isFilePath(token) && !seen[token] {
				refs = append(refs, token)
				seen[token] = true
			}
		}
	}

	return refs, nil
}

// extractSections reads a markdown file and extracts sections keyed by heading
// text. Sections are delimited by "## " headings.
func extractSections(f *os.File) (map[string]string, error) {
	sections := make(map[string]string)
	scanner := bufio.NewScanner(f)

	var currentSection string
	var currentBody strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			// Save previous section.
			if currentSection != "" {
				sections[currentSection] = currentBody.String()
			}
			currentSection = strings.TrimPrefix(line, "## ")
			currentBody.Reset()
			continue
		}
		// Skip headings of other levels (###, #, etc).
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "## ") {
			if currentSection != "" {
				// End of current ## section — save and reset.
				sections[currentSection] = currentBody.String()
				currentSection = ""
				currentBody.Reset()
			}
			continue
		}
		if currentSection != "" {
			currentBody.WriteString(line)
			currentBody.WriteString("\n")
		}
	}
	if currentSection != "" {
		sections[currentSection] = currentBody.String()
	}

	return sections, scanner.Err()
}

// parseTouchpointMatrix reads the release index.md and extracts cross-slice
// file collisions from the "### Touchpoint matrix" table.
//
// Returns:
//   - violations: files claimed by multiple tracks without DOCUMENTED SHARED annotation
//   - sharedFiles: DOCUMENTED SHARED files that appear in plannedFiles
//   - error: on parse errors
func parseTouchpointMatrix(indexPath, thisTrack string, plannedFiles []string) (touchpointViolations, []string, error) {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, nil, err
	}

	// Find the "### Touchpoint matrix" section.
	lines := strings.Split(string(content), "\n")
	matrixStart := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "### Touchpoint matrix") {
			matrixStart = i
			break
		}
	}
	if matrixStart < 0 {
		// No touchpoint matrix — not an error.
		return nil, nil, nil
	}

	// Find the table within the matrix section. Tables start with a pipe.
	headerLine := -1
	for i := matrixStart + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "|") && strings.Contains(line, "File") {
			headerLine = i
			break
		}
		// Stop at next heading level.
		if strings.HasPrefix(line, "#") {
			break
		}
	}
	if headerLine < 0 {
		return nil, nil, nil
	}

	// Parse the header to map column index → track id.
	headerCells := splitTableRow(lines[headerLine])
	trackCols := make(map[int]string) // column index → track id
	for i, cell := range headerCells {
		cell = strings.TrimSpace(cell)
		if strings.HasPrefix(cell, "T") && len(cell) >= 2 && cell[1] >= '0' && cell[1] <= '9' {
			trackCols[i] = cell
		}
	}

	// The separator line: skip it.
	separatorLine := headerLine + 1

	// Parse data rows.
	var rowData []struct {
		file          string
		isDocShared   bool
		trackMarks    map[string]bool // track id → has ✓
	}

	for i := separatorLine + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "|") {
			// End of table.
			break
		}
		cells := splitTableRow(line)
		if len(cells) < 2 {
			continue
		}

		file := strings.TrimSpace(cells[0])
		// Remove backticks from file.
		file = strings.Trim(file, "`")

		isDocShared := strings.Contains(cells[0], "DOCUMENTED SHARED")

		trackMarks := make(map[string]bool)
		for colIdx, trackID := range trackCols {
			if colIdx < len(cells) && strings.Contains(cells[colIdx], "✓") {
				trackMarks[trackID] = true
			}
		}

		rowData = append(rowData, struct {
			file          string
			isDocShared   bool
			trackMarks    map[string]bool
		}{file, isDocShared, trackMarks})
	}

	// Now check for collisions and shared files.
	var violations touchpointViolations
	var sharedFiles []string

	// Build a set of this track's planned files for quick lookup.
	plannedSet := make(map[string]bool, len(plannedFiles))
	for _, p := range plannedFiles {
		plannedSet[p] = true
	}

	for _, row := range rowData {
		// Count tracks with ✓ marks.
		var markedTracks []string
		for t := range row.trackMarks {
			markedTracks = append(markedTracks, t)
		}
		sort.Strings(markedTracks)

		if row.isDocShared {
			// DOCUMENTED SHARED: check if the file is in this slice's planned_files.
			if plannedSet[row.file] {
				sharedFiles = append(sharedFiles, row.file)
			}
			continue
		}

		// Check if this track is one of the marked tracks and there are others.
		if len(markedTracks) > 1 && row.trackMarks[thisTrack] {
			others := make([]string, 0, len(markedTracks)-1)
			for _, t := range markedTracks {
				if t != thisTrack {
					others = append(others, t)
				}
			}
			violations = append(violations, touchpointViolation{
				Kind:   "collision",
				Detail: fmt.Sprintf("file %s claimed by multiple tracks: %s (this track) + %s", row.file, thisTrack, strings.Join(others, ", ")),
			})
		}
	}

	return violations, sharedFiles, nil
}

// splitTableRow splits a markdown table row into cells by the pipe character,
// trimming whitespace.
func splitTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// detectDuplicateMigrations scans all slices in releaseDir for migration
// number collisions. Returns violations when two slices share the same
// 6-digit prefix in any planned_file.
func detectDuplicateMigrations(releaseDir string) (touchpointViolations, error) {
	// Read all status.json files in slice subdirectories.
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, err
	}

	// migrationOwners: map migNumber → sliceID
	migrationOwners := make(map[string]string)
	var violations touchpointViolations

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sliceID := entry.Name()
		statusPath := filepath.Join(releaseDir, sliceID, "status.json")
		st, err := state.Read(statusPath)
		if err != nil {
			// Skip slices without valid status.json (may be uninitialised).
			continue
		}

		for _, pf := range st.PlannedFiles {
			match := migrationPrefixRe.FindStringSubmatch(pf)
			if match == nil {
				continue
			}
			migNum := match[1]
			if owner, exists := migrationOwners[migNum]; exists {
				violations = append(violations, touchpointViolation{
					Kind:   "migration",
					Detail: fmt.Sprintf("migration number %s shared by %s and %s", migNum, owner, sliceID),
				})
			} else {
				migrationOwners[migNum] = sliceID
			}
		}
	}

	return violations, nil
}