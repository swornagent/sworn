package implement

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/state"
)

// specRecord is the JSON shape written to spec.json.
type specRecord struct {
	Schema             string        `json:"$schema"`
	SchemaVersion      int           `json:"schema_version"`
	SliceID            string        `json:"slice_id"`
	Release            string        `json:"release"`
	UserOutcome        string        `json:"user_outcome"`
	AcceptanceCriteria []acRecord    `json:"acceptance_criteria"`
	CoversNeeds        []string      `json:"covers_needs"`
}

// acRecord is one acceptance criterion in spec.json.
type acRecord struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	Type        string `json:"type,omitempty"`
	EARSKeyword string `json:"ears_keyword,omitempty"`
}

var reACLine = regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*(.+)`)

// WriteSpecRecord parses spec.md, extracts the user outcome and acceptance
// criteria, reads covers_needs from status.json, and writes spec.json
// (spec-v1 schema) to the slice directory.
func WriteSpecRecord(specPath, statusPath, sliceDir string) error {
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("spec_record: read spec: %w", err)
	}
	specText := string(specBytes)

	// Read status.json for covers_needs and slice metadata.
	st, err := state.Read(statusPath)
	if err != nil {
		return fmt.Errorf("spec_record: read status: %w", err)
	}

	rec := specRecord{
		Schema:        baton.SpecSchemaURI,
		SchemaVersion: 1,
		SliceID:       st.SliceID,
		Release:       st.Release,
		UserOutcome:   extractUserOutcome(specText),
		CoversNeeds:   st.NeedIDs,
	}

	// Parse acceptance criteria.
	rec.AcceptanceCriteria = parseAcceptanceCriteria(specText)

	// Marshal and validate.
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("spec_record: marshal: %w", err)
	}
	if err := baton.Validate("spec-v1", data); err != nil {
		return fmt.Errorf("spec_record: validation failed: %w", err)
	}

	specJSONPath := filepath.Join(sliceDir, "spec.json")
	if err := os.WriteFile(specJSONPath, data, 0o644); err != nil {
		return fmt.Errorf("spec_record: write: %w", err)
	}
	return nil
}

// extractUserOutcome returns the user outcome text from a spec.md body.
// It returns the first non-empty, non-heading line after "## User outcome".
func extractUserOutcome(spec string) string {
	lines := strings.Split(spec, "\n")
	inOutcome := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## User outcome") {
			inOutcome = true
			continue
		}
		if inOutcome && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return trimmed
		}
	}
	return ""
}

// parseAcceptanceCriteria extracts all checkbox AC lines from spec.md.
// Each line "- [ ] text" or "- [x] text" becomes one acRecord.
func parseAcceptanceCriteria(spec string) []acRecord {
	var acs []acRecord
	inSection := false
	for _, line := range strings.Split(spec, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = strings.Contains(strings.ToLower(trimmed), "acceptance check")
			continue
		}
		if !inSection {
			continue
		}
		if m := reACLine.FindStringSubmatch(line); m != nil {
			text := strings.TrimSpace(m[1])
			if strings.HasPrefix(strings.ToUpper(text), "NOTE:") {
				continue
			}
			checked := strings.Contains(line, "[x]") || strings.Contains(line, "[X]")
			acType := "unchecked"
			if checked {
				acType = "checked"
			}
			earsKw := classifyEARSKeyword(text)
			acs = append(acs, acRecord{
				ID:          fmt.Sprintf("AC-%d", len(acs)+1),
				Text:        text,
				Type:        acType,
				EARSKeyword: earsKw,
			})
		}
	}
	return acs
}

// classifyEARSKeyword determines the EARS pattern keyword for an AC.
func classifyEARSKeyword(ac string) string {
	upper := strings.ToUpper(ac)
	if strings.Contains(upper, "WHEN") {
		return "When"
	}
	if strings.Contains(upper, "WHILE") {
		return "While"
	}
	if strings.Contains(upper, "WHERE") {
		return "Where"
	}
	if strings.Contains(upper, "IF") && strings.Contains(upper, "THEN") {
		return "If"
	}
	return "Ubiquitous"
}