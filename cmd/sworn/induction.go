package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/command"
)

func init() {
	command.Register(command.Command{
		Name:    "induction",
		Summary: "one-time repo onboarding: discover design system, architecture patterns, and NFR stances",
		Run:     cmdInduction,
	})
}

// considerationsPath returns the canonical path to docs/considerations.md.
func considerationsPath() string { return "docs/considerations.md" }

// decisionsPath returns the canonical path to docs/decisions.md.
func decisionsPath() string { return "docs/decisions.md" }

// cmdInduction runs the sworn induction command.
func cmdInduction(args []string) int {
	fs := flag.NewFlagSet("induction", flag.ExitOnError)
	update := fs.Bool("update", false, "update existing catalog after a release")
	force := fs.Bool("force", false, "overwrite existing entries during update")
	designSystem := fs.Bool("design-system", false, "re-run design system discovery with --update")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: sworn induction [--update] [--force] [--design-system]

  induction       run the full onboarding flow (one-time per repo)
  induction --update  re-read dependency files and surface new patterns after a release
  induction --update --force  overwrite existing entries (default: append only)
  induction --update --design-system  re-run design system discovery
`)
	}
	_ = fs.Parse(args)

	updateMode := *update

	// Idempotent detection: if considerations.md already exists with non-empty
	// architecture.patterns, auto-enter --update mode (Pin 3 / AC5).
	if !updateMode {
		if patterns, _ := readPatternsFromCatalog(considerationsPath()); len(patterns) > 0 {
			fmt.Fprintln(os.Stderr, "notice: docs/considerations.md already has patterns — entering --update mode")
			updateMode = true
		}
	}

	// Ensure docs/templates/considerations.md is available as the base template.
	catalogPath := considerationsPath()
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		if err := initializeCatalog(catalogPath); err != nil {
			fmt.Fprintf(os.Stderr, "sworn induction: cannot create %s: %v\n", catalogPath, err)
			return 1
		}
	}

	// Phase 0 — Dependency file discovery (runs first, always silent).
	phase0DependencyDiscovery(catalogPath)

	// Phase 1 — Design system discovery.
	if !updateMode || *designSystem {
		phase1DesignSystem(catalogPath)
	}

	// Phase 2 — Architecture pattern discovery.
	phase2ArchitecturePatterns(catalogPath, updateMode, *force)

	// Phase 3 — NFR stance setup.
	phase3NFRStances(catalogPath)

	fmt.Fprintf(os.Stderr, "\nsworn induction complete — %s populated.\n", catalogPath)
	return 0
}

// ---------------------------------------------------------------------------
// Phase 0 — Dependency file discovery
// ---------------------------------------------------------------------------

func phase0DependencyDiscovery(catalogPath string) {
	var deps []depEntry

	if _, err := os.Stat("go.mod"); err == nil {
		deps = parseGoMod("go.mod")
	}

	if len(deps) == 0 {
		fmt.Fprintln(os.Stderr, "no dependency file detected")
		appendProjectPinned(catalogPath, nil)
		return
	}

	existing := readProjectPinned(catalogPath)
	existingSet := make(map[string]bool)
	for _, e := range existing {
		existingSet[e.module+"@"+e.version] = true
	}

	var newDeps []depEntry
	for _, d := range deps {
		if !existingSet[d.module+"@"+d.version] {
			newDeps = append(newDeps, d)
		}
	}

	if len(newDeps) > 0 {
		appendProjectPinned(catalogPath, newDeps)
	}

	src := "go.mod"
	pinned := readProjectPinned(catalogPath)
	fmt.Fprintf(os.Stderr, "Found %s — %d pinned dependencies recorded in %s [dependencies].\n", src, len(pinned), catalogPath)
}

// depEntry is a single pinned dependency.
type depEntry struct {
	module   string
	version  string
	pinnedBy string
}

// parseGoMod reads a go.mod file and extracts require entries line-by-line.
func parseGoMod(path string) []depEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var deps []depEntry
	inRequireBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inRequireBlock = true
			continue
		}
		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deps = append(deps, depEntry{module: parts[0], version: parts[1], pinnedBy: "go.mod"})
			}
			continue
		}
		if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				deps = append(deps, depEntry{module: parts[1], version: parts[2], pinnedBy: "go.mod"})
			}
		}
	}
	return deps
}

// readProjectPinned reads the [dependencies].project_pinned section from a catalog.
func readProjectPinned(catalogPath string) []depEntry {
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil
	}
	content := string(b)

	// Find the ## [dependencies] section, then project_pinned entries within it.
	idx := strings.Index(content, "## [dependencies]")
	if idx < 0 {
		return nil
	}
	rest := content[idx:]

	// Find the project_pinned: marker within the dependencies section.
	pinIdx := strings.Index(rest, "project_pinned:")
	if pinIdx < 0 {
		return nil
	}
	pinSection := rest[pinIdx:]

	// Read until we hit another ## heading or end of dependencies section.
	endIdx := strings.Index(pinSection, "\n## ")
	if endIdx > 0 {
		pinSection = pinSection[:endIdx]
	}

	var deps []depEntry
	lines := strings.Split(pinSection, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "- module:") {
			mod := strings.TrimSpace(strings.TrimPrefix(line, "- module:"))
			if strings.HasPrefix(line, "#") {
				continue
			}
			ver := ""
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextLine, "version:") {
					ver = strings.TrimSpace(strings.TrimPrefix(nextLine, "version:"))
				}
			}
			pb := ""
			if i+2 < len(lines) {
				pbLine := strings.TrimSpace(lines[i+2])
				if strings.HasPrefix(pbLine, "pinned_by:") {
					pb = strings.TrimSpace(strings.TrimPrefix(pbLine, "pinned_by:"))
				}
			}
			deps = append(deps, depEntry{module: mod, version: ver, pinnedBy: pb})
		}
	}
	return deps
}

// appendProjectPinned appends new dependencies to the catalog's project_pinned section.
func appendProjectPinned(catalogPath string, newDeps []depEntry) {
	if len(newDeps) == 0 {
		return
	}
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return
	}
	content := string(b)

	// Find the project_pinned: marker.
	idx := strings.Index(content, "project_pinned:")
	if idx < 0 {
		return
	}

	// Find the end of the project_pinned section — the next ## heading.
	rest := content[idx:]
	endIdx := strings.Index(rest, "\n## ")
	var before, after string
	if endIdx > 0 {
		before = content[:idx] + rest[:endIdx]
		after = rest[endIdx:]
	} else {
		before = content[:idx] + rest
		after = ""
	}

	// Build the new entries.
	var sb strings.Builder
	sb.WriteString(before)
	for _, d := range newDeps {
		sb.WriteString(fmt.Sprintf("\n  - module: %s\n    version: %s\n    pinned_by: %s\n", d.module, d.version, d.pinnedBy))
	}
	sb.WriteString("\n  " + after)

	_ = os.WriteFile(catalogPath, []byte(sb.String()), 0644)
}

// ---------------------------------------------------------------------------
// Catalog initialisation
// ---------------------------------------------------------------------------

func initializeCatalog(path string) error {
	// Copy from the template.
	tmplPath := filepath.Join("docs", "templates", "considerations.md")
	b, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("template %s not found: %w", tmplPath, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// ---------------------------------------------------------------------------
// Phase 1 — Design system discovery
// ---------------------------------------------------------------------------

func phase1DesignSystem(catalogPath string) {
	scanner := bufio.NewScanner(os.Stdin)
	reader := func(prompt, def string) string {
		if def != "" {
			fmt.Fprintf(os.Stderr, "%s [%s]: ", prompt, def)
		} else {
			fmt.Fprintf(os.Stderr, "%s: ", prompt)
		}
		if !scanner.Scan() {
			return def
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer == "" {
			return def
		}
		return answer
	}

	fmt.Fprintln(os.Stderr, "\n--- Phase 1: Design system ---")
	hasDS := reader("Do you have a design system? (y/n)", "y")
	if strings.ToLower(hasDS) == "n" || strings.ToLower(hasDS) == "no" {
		setFrontmatterField(catalogPath, "design_system:", "framework:", "none")
		return
	}

	framework := reader("What framework?", "shadcn")
	if framework == "" {
		framework = "shadcn"
	}
	location := reader("Where is it? (URL, path, or npm package)", "")
	compLib := reader("Component library package (e.g. @repo/ui, leave blank if none)", "")

	setFrontmatterField(catalogPath, "design_system:", "framework:", framework)
	setFrontmatterField(catalogPath, "design_system:", "location:", location)
	setFrontmatterField(catalogPath, "design_system:", "component_library:", compLib)
}

// setFrontmatterField sets a field in the YAML frontmatter of the catalog.
func setFrontmatterField(catalogPath, section, field, value string) {
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return
	}
	content := string(b)

	// Find the frontmatter (between first and second ---).
	first := strings.Index(content, "---")
	second := strings.Index(content[first+3:], "---")
	if first < 0 || second < 0 {
		return
	}
	second += first + 3
	fm := content[first : second+3]

	// Look for the field within the section.
	secIdx := strings.Index(fm, section)
	if secIdx < 0 {
		return
	}
	secRest := fm[secIdx:]

	// Find the field within this section, bounded by the next top-level key.
	fieldIdx := strings.Index(secRest, field)
	if fieldIdx < 0 {
		return
	}

	// Find the end of this line.
	lineStart := secIdx + fieldIdx
	lineEnd := strings.Index(content[lineStart:], "\n")
	if lineEnd < 0 {
		lineEnd = len(content) - lineStart
	}

	oldLine := content[lineStart : lineStart+lineEnd]
	quote := "'"
	if !strings.Contains(oldLine, "'") {
		quote = "''"
	}
	_ = quote
	newLine := fmt.Sprintf("%s '%s'", field, value)

	newContent := content[:lineStart] + newLine + content[lineStart+lineEnd:]
	_ = os.WriteFile(catalogPath, []byte(newContent), 0644)
}

// ---------------------------------------------------------------------------
// Phase 2 — Architecture pattern discovery
// ---------------------------------------------------------------------------

func phase2ArchitecturePatterns(catalogPath string, updateMode bool, force bool) {
	existingPatterns, _ := readPatternsFromCatalog(catalogPath)

	proposed := inferPatterns()
	if len(proposed) == 0 {
		fmt.Fprintln(os.Stderr, "\n--- Phase 2: Architecture patterns ---")
		fmt.Fprintln(os.Stderr, "No patterns could be inferred. You can add them manually later in the catalog.")
		return
	}

	var newPatterns []pattern
	existingMap := make(map[string]bool)
	for _, p := range existingPatterns {
		existingMap[p.Pattern] = true
	}
	for _, p := range proposed {
		if !existingMap[p.Pattern] || force {
			newPatterns = append(newPatterns, p)
		}
	}

	if len(newPatterns) == 0 {
		fmt.Fprintln(os.Stderr, "\n--- Phase 2: Architecture patterns ---")
		fmt.Fprintln(os.Stderr, "No new patterns detected since last induction.")
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprintln(os.Stderr, "\n--- Phase 2: Architecture patterns ---")
	fmt.Fprintln(os.Stderr, "I found these patterns in your codebase:")
	for i, p := range newPatterns {
		fmt.Fprintf(os.Stderr, "  [%d] %s — %s\n", i+1, p.Pattern, p.Location)
		fmt.Fprintf(os.Stderr, "      \"%s\"\n", p.Intent)
	}

	fmt.Fprint(os.Stderr, "\nAccept all? (y) / Edit individually (e) / Add more (a) / Skip (s): ")
	scanner.Scan()
	choice := strings.TrimSpace(strings.ToLower(scanner.Text()))

	switch choice {
	case "", "y", "yes":
		writePatterns(catalogPath, newPatterns)
	case "e":
		for _, p := range newPatterns {
			fmt.Fprintf(os.Stderr, "  Accept '%s'? (y/n): ", p.Pattern)
			scanner.Scan()
			if strings.ToLower(strings.TrimSpace(scanner.Text())) == "y" {
				writePatterns(catalogPath, []pattern{p})
			}
		}
	case "a":
		writePatterns(catalogPath, newPatterns)
		addMorePatterns(catalogPath, scanner)
	case "s":
		fmt.Fprintln(os.Stderr, "Skipping pattern detection. You can add patterns manually in the catalog.")
	}
}

type pattern struct {
	Pattern  string
	Location string
	Intent   string
}

func inferPatterns() []pattern {
	var patterns []pattern

	// Go: check for interface-first design in internal/model/
	if matches, loc := findInterfacePattern(); matches {
		patterns = append(patterns, pattern{
			Pattern:  "interface-first design",
			Location: loc,
			Intent:   "enables mock injection in verify/test contexts",
		})
	}

	// Go: check for stdlib HTTP (no framework dependency)
	if matches, loc := findStdlibHTTP(); matches {
		patterns = append(patterns, pattern{
			Pattern:  "stdlib HTTP",
			Location: loc,
			Intent:   "no framework dependency; cross-compiles cleanly",
		})
	}

	// Go: check for table-driven tests
	if matches, loc := findTableDrivenTests(); matches {
		patterns = append(patterns, pattern{
			Pattern:  "table-driven tests",
			Location: loc,
			Intent:   "readable failure output; easy to add cases",
		})
	}

	return patterns
}

func findInterfacePattern() (bool, string) {
	entries, err := os.ReadDir("internal/model")
	if err != nil {
		return false, ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join("internal/model", e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), "type ") && strings.Contains(string(b), "interface {") {
			return true, path
		}
	}
	return false, ""
}

func findStdlibHTTP() (bool, string) {
	entries, err := os.ReadDir("internal/model")
	if err != nil {
		return false, ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join("internal/model", e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(b)
		if strings.Contains(content, `"net/http"`) && !strings.Contains(content, "github.com/") {
			return true, path
		}
	}
	return false, ""
}

func findTableDrivenTests() (bool, string) {
	testDirs := []string{"internal/model", "internal/config", "internal/command", "internal/account", "internal/prompt"}
	for _, dir := range testDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := string(b)
			if strings.Contains(content, "tests := []struct") || strings.Contains(content, "tt := range") || strings.Contains(content, "tc := range") {
				return true, path
			}
		}
	}
	return false, ""
}

func readPatternsFromCatalog(catalogPath string) ([]pattern, string) {
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, ""
	}
	content := string(b)

	// Find patterns: in the frontmatter.
	first := strings.Index(content, "---")
	second := strings.Index(content[first+3:], "---")
	if first < 0 || second < 0 {
		return nil, content
	}
	second += first + 3
	fm := content[first : second+3]

	patIdx := strings.Index(fm, "patterns:")
	if patIdx < 0 {
		return nil, content
	}
	patSection := fm[patIdx:]

	// Find end of patterns list — next top-level key or end of frontmatter.
	endIdx := strings.Index(patSection, "\nenabled_dimensions:")
	if endIdx < 0 {
		endIdx = strings.Index(patSection, "\n---")
	}
	if endIdx > 0 {
		patSection = patSection[:endIdx]
	}

	var patterns []pattern
	lines := strings.Split(patSection, "\n")
	var current pattern
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- pattern:") {
			if current.Pattern != "" {
				patterns = append(patterns, current)
			}
			current = pattern{Pattern: strings.TrimSpace(strings.TrimPrefix(trimmed, "- pattern:"))}
		} else if strings.HasPrefix(trimmed, "location:") && current.Pattern != "" {
			current.Location = strings.TrimSpace(strings.TrimPrefix(trimmed, "location:"))
		} else if strings.HasPrefix(trimmed, "intent:") && current.Pattern != "" {
			current.Intent = strings.TrimSpace(strings.TrimPrefix(trimmed, "intent:"))
		}
	}
	if current.Pattern != "" {
		patterns = append(patterns, current)
	}
	return patterns, content
}

func writePatterns(catalogPath string, newPatterns []pattern) {
	_, content := readPatternsFromCatalog(catalogPath)

	first := strings.Index(content, "---")
	second := strings.Index(content[first+3:], "---")
	if first < 0 || second < 0 {
		return
	}
	second += first + 3

	// Find the patterns: line in the frontmatter.
	patIdx := strings.Index(content[:second], "patterns:")
	if patIdx < 0 {
		return
	}

	// Find the end of the patterns block by scanning forward line by line.
	afterPatterns := content[patIdx:]
	lines := strings.Split(afterPatterns, "\n")
	blockEnd := 0
	for i, line := range lines {
		if i == 0 {
			blockEnd += len(line) + 1
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			blockEnd += len(line) + 1
			continue
		}
		indent := len(line) - len(trimmed)
		if indent < 4 && !strings.HasPrefix(trimmed, "-") &&
			!strings.HasPrefix(trimmed, "location:") &&
			!strings.HasPrefix(trimmed, "intent:") &&
			!strings.HasPrefix(trimmed, "pattern:") {
			break
		}
		blockEnd += len(line) + 1
	}
	for blockEnd > 0 && (afterPatterns[blockEnd-1] == '\n' || afterPatterns[blockEnd-1] == '\r') {
		blockEnd--
	}

	existingPatterns, _ := readPatternsFromCatalog(catalogPath)
	allPatterns := existingPatterns
	for _, np := range newPatterns {
		dup := false
		for _, ep := range existingPatterns {
			if ep.Pattern == np.Pattern {
				dup = true
				break
			}
		}
		if !dup {
			allPatterns = append(allPatterns, np)
		}
	}

	var sb strings.Builder
	sb.WriteString("patterns:\n")
	for _, p := range allPatterns {
		sb.WriteString(fmt.Sprintf("  - pattern: %s\n    location: %s\n    intent: %s\n", p.Pattern, p.Location, p.Intent))
	}

	newContent := content[:patIdx] + strings.TrimRight(sb.String(), "\n") + "\n" + afterPatterns[blockEnd:]
	_ = os.WriteFile(catalogPath, []byte(newContent), 0644)
}

func addMorePatterns(catalogPath string, scanner *bufio.Scanner) {
	for {
		fmt.Fprint(os.Stderr, "\nAdd a pattern? (y/n): ")
		scanner.Scan()
		if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			break
		}
		fmt.Fprint(os.Stderr, "  Pattern name: ")
		scanner.Scan()
		pName := strings.TrimSpace(scanner.Text())
		fmt.Fprint(os.Stderr, "  Location (file path): ")
		scanner.Scan()
		pLoc := strings.TrimSpace(scanner.Text())
		fmt.Fprint(os.Stderr, "  Intent (one-line): ")
		scanner.Scan()
		pIntent := strings.TrimSpace(scanner.Text())

		if pName != "" {
			writePatterns(catalogPath, []pattern{{Pattern: pName, Location: pLoc, Intent: pIntent}})
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 3 — NFR stance setup
// ---------------------------------------------------------------------------

func phase3NFRStances(catalogPath string) {
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return
	}
	content := string(b)

	// Find enabled_dimensions in frontmatter.
	first := strings.Index(content, "---")
	second := strings.Index(content[first+3:], "---")
	if first < 0 || second < 0 {
		return
	}
	second += first + 3
	fm := content[first : second+3]

	dimIdx := strings.Index(fm, "enabled_dimensions:")
	if dimIdx < 0 {
		return
	}
	dimLine := fm[dimIdx:]
	dimLineEnd := strings.Index(dimLine, "\n")
	if dimLineEnd < 0 {
		return
	}
	dimLine = dimLine[:dimLineEnd]

	dimStr := strings.TrimPrefix(dimLine, "enabled_dimensions:")
	dimStr = strings.TrimSpace(dimStr)
	dimStr = strings.Trim(dimStr, "[]")
	dims := strings.Split(dimStr, ",")
	for i := range dims {
		dims[i] = strings.TrimSpace(dims[i])
	}

	fmt.Fprintln(os.Stderr, "\n--- Phase 3: NFR stances ---")
	scanner := bufio.NewScanner(os.Stdin)
	for _, dim := range dims {
		dim = strings.TrimSpace(dim)
		if dim == "" {
			continue
		}
		fmt.Fprintf(os.Stderr, "[%s] — required_for: all\n", dim)
		fmt.Fprintf(os.Stderr, "  Customise? Add project-specific notes? (press Enter to keep default, or type notes):\n  > ")
		scanner.Scan()
		notes := strings.TrimSpace(scanner.Text())
		if notes != "" {
			appendNFRNotes(catalogPath, dim, notes)
		}
	}
}

func appendNFRNotes(catalogPath, dim, notes string) {
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return
	}
	content := string(b)

	anchor := "\n## [" + dim + "]"
	idx := strings.Index(content, anchor)
	if idx < 0 {
		return
	}

	rest := content[idx+len(anchor):]
	nextHeading := strings.Index(rest, "\n## ")
	var sectionEnd int
	if nextHeading > 0 {
		sectionEnd = idx + len(anchor) + nextHeading
	} else {
		sectionEnd = len(content)
	}

	body := content[idx+len(anchor) : sectionEnd]
	insertAt := strings.LastIndex(body, "\n")
	if insertAt < 0 {
		return
	}

	notesLine := fmt.Sprintf("\nproject_notes: \"%s\"\n", notes)
	newContent := content[:idx+len(anchor)] + body[:insertAt] + notesLine + body[insertAt:]
	_ = os.WriteFile(catalogPath, []byte(newContent), 0644)
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func dedupStrings(s []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func sortStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}
