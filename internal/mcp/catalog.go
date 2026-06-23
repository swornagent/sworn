package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RegisterCatalogTools registers the catalog management, decision registry, and
// unified release planning tools (S20). All eight tools operate on files under
// repoRoot — docs/considerations.md, docs/decisions.md, and docs/release/<name>/.
//
// Stdlib is sufficient for this slice's text-file ops; no new dependency, no ADR
// required.
func RegisterCatalogTools(s *Server, repoRoot string) {
	// ---- 1. plan_release ----
	s.RegisterTool("plan_release", json.RawMessage(`{
		"type": "object",
		"properties": {
			"name":        {"type": "string"},
			"goal":        {"type": "string"},
			"tracking_issue": {"type": "string"}
		},
		"required": ["name"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Name          string `json:"name"`
			Goal          string `json:"goal"`
			TrackingIssue string `json:"tracking_issue"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		releaseDir := filepath.Join(repoRoot, "docs", "release", p.Name)
		if _, err := os.Stat(releaseDir); err == nil {
			// Existing release: read index.md and return state summary.
			indexPath := filepath.Join(releaseDir, "index.md")
			indexData, err := os.ReadFile(indexPath)
			if err != nil {
				return nil, fmt.Errorf("release exists but cannot read index.md: %w", err)
			}
			summary := releaseStateSummary(string(indexData))
			result := map[string]any{
				"exists":        true,
				"slice_count":   summary["slice_count"],
				"state_summary": summary,
			}
			b, _ := json.Marshal(result)
			return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
		}

		// New release: delegate to CreateRelease helper from tools_plan.go.
		goal := p.Goal
		if goal == "" {
			goal = "(no goal provided)"
		}
		tracking := p.TrackingIssue
		if tracking == "" {
			tracking = "(no tracking issue)"
		}
		created, err := CreateRelease(repoRoot, p.Name, goal, tracking)
		if err != nil {
			return nil, fmt.Errorf("plan_release: %w", err)
		}
		paths := make([]string, 0, len(created))
		for _, v := range created {
			paths = append(paths, v)
		}
		result := map[string]any{
			"exists":        false,
			"created_paths": paths,
		}
		b, _ := json.Marshal(result)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 2. get_induction_status ----
	s.RegisterTool("get_induction_status", json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		considerationsPath := filepath.Join(repoRoot, "docs", "considerations.md")
		data, err := os.ReadFile(considerationsPath)
		if err != nil {
			result, _ := json.Marshal(map[string]any{"catalog_exists": false})
			return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
		}
		content := string(data)

		dsConfigured := hasSectionWithContent(content, "design_system", "location")
		patternsCount := countSectionEntries(content, "architecture.patterns", "- ")
		dimensions := enabledDimensions(content)
		decisionsPath := filepath.Join(repoRoot, "docs", "decisions.md")
		decisionsCount := 0
		if dData, dErr := os.ReadFile(decisionsPath); dErr == nil {
			decisionsCount = countSectionEntries(string(dData), "", "### ")
		}

		result, _ := json.Marshal(map[string]any{
			"catalog_exists":              true,
			"design_system_configured":    dsConfigured,
			"architecture_patterns_count": patternsCount,
			"enabled_dimensions":          dimensions,
			"decisions_count":             decisionsCount,
		})
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
	})

	// ---- 3. get_considerations ----
	s.RegisterTool("get_considerations", json.RawMessage(`{
		"type": "object",
		"properties": {
			"slice_type": {"type": "string"}
		}
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			SliceType string `json:"slice_type"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if p.SliceType == "" {
			p.SliceType = "all"
		}

		considerationsPath := filepath.Join(repoRoot, "docs", "considerations.md")
		data, err := os.ReadFile(considerationsPath)
		if err != nil {
			result, _ := json.Marshal(map[string]any{"catalog_missing": true})
			return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
		}
		content := string(data)

		sections := extractConsiderations(content, p.SliceType)
		b, _ := json.Marshal(sections)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 4. search_decisions ----
	s.RegisterTool("search_decisions", json.RawMessage(`{
		"type": "object",
		"properties": {
			"keywords": {"type": "string"}
		},
		"required": ["keywords"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Keywords string `json:"keywords"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		decisionsPath := filepath.Join(repoRoot, "docs", "decisions.md")
		data, err := os.ReadFile(decisionsPath)
		if err != nil {
			// No decisions.md → empty array, no error.
			return &ToolResult{Content: []ContentItem{{Type: "text", Text: "[]"}}}, nil
		}

		entries := searchDecisions(string(data), p.Keywords)
		b, _ := json.Marshal(entries)
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(b)}}}, nil
	})

	// ---- 5. record_decision ----
	s.RegisterTool("record_decision", json.RawMessage(`{
		"type": "object",
		"properties": {
			"type":        {"type": "string"},
			"title":       {"type": "string"},
			"decision":    {"type": "string"},
			"rationale":   {"type": "string"},
			"applies_to":  {"type": "string"},
			"release":     {"type": "string"},
			"slice":       {"type": "string"},
			"overrides":   {"type": "string"}
		},
		"required": ["type", "title", "decision", "rationale", "applies_to"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Type      string `json:"type"`
			Title     string `json:"title"`
			Decision  string `json:"decision"`
			Rationale string `json:"rationale"`
			AppliesTo string `json:"applies_to"`
			Release   string `json:"release,omitempty"`
			Slice     string `json:"slice,omitempty"`
			Overrides string `json:"overrides,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		decisionsPath := filepath.Join(repoRoot, "docs", "decisions.md")
		entry := formatDecisionEntry(p.Type, p.Title, p.Decision, p.Rationale, p.AppliesTo, p.Release, p.Slice, p.Overrides)

		// Create docs/decisions.md if it does not exist.
		if _, err := os.Stat(decisionsPath); os.IsNotExist(err) {
			header := "# Decisions Registry\n\n> Append-only decision log. Each entry records one design, architecture, data,\n> flow, deviation, or resolution decision. Overrides are recorded as new entries\n> with `Overrides: <prior-decision-date>` rather than editing the original.\n\n"
			if err := os.WriteFile(decisionsPath, []byte(header+entry+"\n"), 0644); err != nil {
				return nil, fmt.Errorf("record_decision: %w", err)
			}
		} else {
			f, err := os.OpenFile(decisionsPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return nil, fmt.Errorf("record_decision: %w", err)
			}
			defer f.Close()
			if _, err := f.WriteString("\n" + entry + "\n"); err != nil {
				return nil, fmt.Errorf("record_decision: %w", err)
			}
		}

		return &ToolResult{Content: []ContentItem{{Type: "text", Text: entry}}}, nil
	})

	// ---- 6. check_design_system ----
	s.RegisterTool("check_design_system", json.RawMessage(`{
		"type": "object",
		"properties": {
			"component_description": {"type": "string"}
		},
		"required": ["component_description"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			ComponentDescription string `json:"component_description"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		considerationsPath := filepath.Join(repoRoot, "docs", "considerations.md")
		data, err := os.ReadFile(considerationsPath)
		if err != nil {
			// No catalog → unconfigured.
			result, _ := json.Marshal(map[string]any{"status": "unconfigured"})
			return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
		}

		content := string(data)
		location := extractSectionField(content, "design_system", "location")
		if location == "" {
			result, _ := json.Marshal(map[string]any{"status": "unconfigured"})
			return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
		}

		// For now, return a scaffold. The AI enriches conversationally.
		result, _ := json.Marshal(map[string]any{
			"status":            "exists",
			"matched_component": extractSectionField(content, "design_system", "component_library"),
			"location":          location, "options": []map[string]string{
				{"label": "Reuse as-is", "description": "Use the existing component without modification."},
				{"label": "Extend with variant", "description": "Create a variant of the existing component for this use case."},
				{"label": "Build new", "description": "Build a new component from scratch."},
			},
			"recommendation": "Extend with variant",
		})
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
	})

	// ---- 7. update_design_system ----
	s.RegisterTool("update_design_system", json.RawMessage(`{
		"type": "object",
		"properties": {
			"location":          {"type": "string"},
			"framework":         {"type": "string"},
			"version":           {"type": "string"},
			"component_library": {"type": "string"}
		},
		"required": ["location", "framework"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Location         string `json:"location"`
			Framework        string `json:"framework"`
			Version          string `json:"version,omitempty"`
			ComponentLibrary string `json:"component_library,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		considerationsPath := filepath.Join(repoRoot, "docs", "considerations.md")
		if err := upsertSection(considerationsPath, "design_system", map[string]string{
			"location":          p.Location,
			"framework":         p.Framework,
			"version":           p.Version,
			"component_library": p.ComponentLibrary,
		}); err != nil {
			return nil, fmt.Errorf("update_design_system: %w", err)
		}

		result, _ := json.Marshal(map[string]any{
			"updated": true,
			"section": "design_system",
		})
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
	})

	// ---- 8. record_architecture_pattern ----
	s.RegisterTool("record_architecture_pattern", json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern":  {"type": "string"},
			"location": {"type": "string"},
			"intent":   {"type": "string"}
		},
		"required": ["pattern", "location", "intent"]
	}`), func(ctx context.Context, params json.RawMessage) (*ToolResult, error) {
		var p struct {
			Pattern  string `json:"pattern"`
			Location string `json:"location"`
			Intent   string `json:"intent"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		considerationsPath := filepath.Join(repoRoot, "docs", "considerations.md")

		// Idempotent: check if pattern already exists.
		if existing, err := os.ReadFile(considerationsPath); err == nil {
			if strings.Contains(string(existing), "- "+p.Pattern) {
				result, _ := json.Marshal(map[string]any{
					"added":   false,
					"pattern": p.Pattern,
					"reason":  "already exists",
				})
				return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
			}
		}

		entry := fmt.Sprintf("- %s (%s): %s", p.Pattern, p.Location, p.Intent)
		if err := appendToSection(considerationsPath, "architecture.patterns", entry); err != nil {
			return nil, fmt.Errorf("record_architecture_pattern: %w", err)
		}

		result, _ := json.Marshal(map[string]any{
			"added":   true,
			"pattern": p.Pattern,
		})
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: string(result)}}}, nil
	})
}

// ---- helper functions ----

// releaseStateSummary parses an index.md body to count slices by state.
// It does a simple grep for state: <value> lines.
func releaseStateSummary(indexContent string) map[string]int {
	summary := map[string]int{
		"planned":     0,
		"in_progress": 0,
		"implemented": 0,
		"verified":    0,
	}
	lines := strings.Split(indexContent, "\n")
	states := []string{"planned", "in_progress", "implemented", "verified"}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, s := range states {
			if strings.HasPrefix(trimmed, "state:") && strings.Contains(trimmed, s) {
				summary[s]++
			}
		}
	}
	// Count slice entries from the slices table.
	summary["slice_count"] = countSliceTableRows(indexContent)
	return summary
}

// countSliceTableRows counts the number of data rows in the Slices table.
func countSliceTableRows(content string) int {
	lines := strings.Split(content, "\n")
	inTable := false
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "| ID | Track | User outcome |") {
			inTable = true
			continue
		}
		if inTable {
			if strings.HasPrefix(trimmed, "|") && strings.Contains(trimmed, "| S") {
				count++
			} else if trimmed == "" || strings.HasPrefix(trimmed, "###") || strings.HasPrefix(trimmed, "##") {
				inTable = false
			}
		}
	}
	return count
}

// hasSectionWithContent checks whether a named section has a non-empty value for
// the given field. Sections are delimited by `## section_name`.
func hasSectionWithContent(content, section, field string) bool {
	lines := strings.Split(content, "\n")
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "## "+section) {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "## ") {
			inSection = false
			continue
		}
		if inSection && strings.HasPrefix(trimmed, field+":") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, field+":"))
			if val != "" {
				return true
			}
		}
	}
	return false
}

// countSectionEntries counts lines starting with prefix within a named section.
// If section is "", counts across the whole content.
func countSectionEntries(content, section, prefix string) int {
	lines := strings.Split(content, "\n")
	inSection := section == ""
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inSection && strings.EqualFold(trimmed, "## "+section) {
			inSection = true
			continue
		}
		if inSection && section != "" && strings.HasPrefix(trimmed, "## ") {
			inSection = false
			continue
		}
		if inSection && strings.HasPrefix(trimmed, prefix) {
			count++
		}
	}
	return count
}

// enabledDimensions returns the list of dimension sections found in the catalog.
func enabledDimensions(content string) []string {
	var dims []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## [") && strings.Contains(trimmed, "]") {
			dim := strings.TrimPrefix(trimmed, "## ")
			dims = append(dims, dim)
		}
	}
	return dims
}

// extractConsiderations returns the applicable sections for a slice_type.
func extractConsiderations(content, sliceType string) map[string]any {
	result := map[string]any{}

	// Always include design_system block.
	if ds := extractSection(content, "design_system"); ds != "" {
		result["design_system"] = ds
	}

	// Always include architecture.patterns block.
	if ap := extractSection(content, "architecture.patterns"); ap != "" {
		result["architecture.patterns"] = ap
	}

	// Determine which dimension sections to include.
	wanted := map[string]bool{}
	switch sliceType {
	case "ui":
		wanted["[ui]"] = true
		wanted["[security]"] = true
	case "api":
		wanted["[api]"] = true
		wanted["[security]"] = true
	case "data":
		wanted["[data]"] = true
		wanted["[security]"] = true
	default: // "all"
		wanted["[ui]"] = true
		wanted["[api]"] = true
		wanted["[data]"] = true
		wanted["[security]"] = true
		wanted["[observability]"] = true
	}

	for dim := range wanted {
		if s := extractDimensionSection(content, dim); s != "" {
			result[dim] = s
		}
	}

	return result
}

// extractSection returns the full body of a named ## section.
func extractSection(content, section string) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "## "+section) {
			inSection = true
			sb.WriteString(line + "\n")
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if inSection {
			sb.WriteString(line + "\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// extractDimensionSection returns the body of a dimension section like `## [ui]`.
func extractDimensionSection(content, dim string) string {
	return extractSection(content, dim)
}

// extractSectionField extracts a `field: value` line from within a named section.
func extractSectionField(content, section, field string) string {
	lines := strings.Split(content, "\n")
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "## "+section) {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "## ") {
			inSection = false
			continue
		}
		if inSection && strings.HasPrefix(trimmed, field+":") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, field+":"))
		}
	}
	return ""
}

// searchDecisions performs case-insensitive keyword search against decision entries.
// Each entry is a block starting with `### <TYPE>: <title>`.
func searchDecisions(content, keywords string) []map[string]string {
	keyword := strings.ToLower(strings.TrimSpace(keywords))
	// Split into entries on ### boundaries.
	entries := splitDecisionEntries(content)
	var results []map[string]string
	for _, e := range entries {
		lower := strings.ToLower(e)
		if strings.Contains(lower, keyword) {
			results = append(results, map[string]string{"entry": strings.TrimSpace(e)})
		}
	}
	return results
}

// splitDecisionEntries splits decisions.md content on `### ` boundaries.
func splitDecisionEntries(content string) []string {
	var entries []string
	var current strings.Builder
	inEntry := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			if inEntry && current.Len() > 0 {
				entries = append(entries, current.String())
			}
			current.Reset()
			current.WriteString(line + "\n")
			inEntry = true
			continue
		}
		if inEntry {
			current.WriteString(line + "\n")
		}
	}
	if inEntry && current.Len() > 0 {
		entries = append(entries, current.String())
	}
	return entries
}

// formatDecisionEntry formats a single decision entry for docs/decisions.md.
func formatDecisionEntry(typ, title, decision, rationale, appliesTo, release, slice, overrides string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s: %s\n", strings.ToUpper(typ), title))
	sb.WriteString(fmt.Sprintf("- **Date**: %s\n", now))
	sb.WriteString(fmt.Sprintf("- **Decision**: %s\n", decision))
	sb.WriteString(fmt.Sprintf("- **Rationale**: %s\n", rationale))
	sb.WriteString(fmt.Sprintf("- **Applies to**: %s\n", appliesTo))
	if release != "" {
		sb.WriteString(fmt.Sprintf("- **Release**: %s\n", release))
	}
	if slice != "" {
		sb.WriteString(fmt.Sprintf("- **Slice**: %s\n", slice))
	}
	if overrides != "" {
		sb.WriteString(fmt.Sprintf("- **Overrides**: %s\n", overrides))
	}
	return strings.TrimSpace(sb.String())
}

// upsertSection creates or replaces a named section in a Markdown file.
func upsertSection(path, section string, fields map[string]string) error {
	// Build the section block.
	var sb strings.Builder
	sb.WriteString("## " + section + "\n\n")
	for k, v := range fields {
		if v != "" {
			sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create file with header + section.
		header := "# Considerations Catalog\n\n> Design system, architecture patterns, and dimension-specific stances.\n> Generated by the AI during conversational induction via SwornAgent MCP tools.\n\n"
		if err := os.WriteFile(path, []byte(header+sb.String()+"\n"), 0644); err != nil {
			return err
		}
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	newContent := replaceOrAppendSection(string(content), section, sb.String())
	return os.WriteFile(path, []byte(newContent), 0644)
}

// replaceOrAppendSection replaces an existing ## section or appends it at end.
func replaceOrAppendSection(content, section, newBlock string) string {
	lines := strings.Split(content, "\n")
	marker := "## " + section

	var result []string
	inSection := false
	replaced := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, marker) {
			inSection = true
			replaced = true
			result = append(result, newBlock)
			// Skip until next ## or EOF.
			for i+1 < len(lines) {
				i++
				next := strings.TrimSpace(lines[i])
				if strings.HasPrefix(next, "## ") {
					result = append(result, lines[i])
					break
				}
			}
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "## ") {
			inSection = false
		}
		result = append(result, line)
	}
	if !replaced {
		result = append(result, "", newBlock)
	}
	return strings.Join(result, "\n")
}

// appendToSection appends a line to a named ## section, creating it if absent.
func appendToSection(path, section, line string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := "# Considerations Catalog\n\n> Design system, architecture patterns, and dimension-specific stances.\n\n"
		content := header + "## " + section + "\n\n" + line + "\n"
		return os.WriteFile(path, []byte(content), 0644)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(content)
	marker := "## " + section

	// Check if section exists.
	if strings.Contains(strings.ToLower(text), strings.ToLower(marker)) {
		// Append to existing section.
		lines := strings.Split(text, "\n")
		var result []string
		inSection := false
		appended := false
		for i := 0; i < len(lines); i++ {
			l := lines[i]
			trimmed := strings.TrimSpace(l)
			if strings.EqualFold(trimmed, marker) {
				inSection = true
				result = append(result, l)
				continue
			}
			if inSection && strings.HasPrefix(trimmed, "## ") {
				// End of section — insert before this heading.
				if !appended {
					result = append(result, line)
					appended = true
				}
				inSection = false
			}
			if inSection && i == len(lines)-1 {
				// Last line, still in section — no next heading.
				result = append(result, l)
				if !appended {
					result = append(result, line)
					appended = true
				}
				continue
			}
			result = append(result, l)
		}
		if inSection && !appended {
			result = append(result, line)
		}
		return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0644)
	}

	// Section doesn't exist — append at end.
	newContent := text + "\n## " + section + "\n\n" + line + "\n"
	return os.WriteFile(path, []byte(newContent), 0644)
}
