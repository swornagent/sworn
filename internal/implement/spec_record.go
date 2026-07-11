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
//
// The shape conforms to the strict vendored v0.10.0 spec-v1 schema
// (additionalProperties:false): schema_version is retired ($schema carries the
// version), and in_scope/out_of_scope are required arrays. They are never nil
// on the wire — an absent or empty section marshals as [] (not null), which the
// schema's "type": "array" requires.
type specRecord struct {
	Schema             string     `json:"$schema"`
	SliceID            string     `json:"slice_id"`
	Release            string     `json:"release"`
	UserOutcome        string     `json:"user_outcome"`
	InScope            []string   `json:"in_scope"`
	OutOfScope         []string   `json:"out_of_scope"`
	CoversNeeds        []string   `json:"covers_needs"`
	AcceptanceCriteria []acRecord `json:"acceptance_criteria"`
}

// acRecord is one acceptance criterion in spec.json. The v0.10.0 spec-v1 AC
// item is strict (additionalProperties:false, allowing only id/text/ears_pattern/
// test_refs), so the retired scraper fields type/ears_keyword are not emitted.
type acRecord struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

var reACLine = regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*(.+)`)

// reScopeBullet matches a plain markdown bullet ("- item" or "* item") under a
// "## In scope" / "## Out of scope" section (as distinct from the "- [ ]"
// acceptance-check bullets reACLine matches).
var reScopeBullet = regexp.MustCompile(`^\s*[-*]\s+(.+)`)

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
		Schema:      baton.SpecSchemaURI,
		SliceID:     st.SliceID,
		Release:     st.Release,
		UserOutcome: extractUserOutcome(specText),
		InScope:     parseScopeSection(specText, "In scope"),
		OutOfScope:  parseScopeSection(specText, "Out of scope"),
		CoversNeeds: st.CoversNeeds,
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
			acs = append(acs, acRecord{
				ID:   fmt.Sprintf("AC-%d", len(acs)+1),
				Text: text,
			})
		}
	}
	return acs
}

// parseScopeSection extracts the bullet items under a "## <heading>" section of
// spec.md — used for "In scope" and "Out of scope". It returns a NON-nil slice
// (an absent or empty section yields []string{}), so the emitted spec.json
// always carries in_scope/out_of_scope as arrays and never as JSON null, which
// the strict v0.10.0 spec-v1 schema requires (required + "type": "array").
func parseScopeSection(spec, heading string) []string {
	items := []string{}
	inSection := false
	for _, line := range strings.Split(spec, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			inSection = strings.EqualFold(name, heading)
			continue
		}
		if !inSection {
			continue
		}
		if m := reScopeBullet.FindStringSubmatch(line); m != nil {
			if text := strings.TrimSpace(m[1]); text != "" {
				items = append(items, text)
			}
		}
	}
	return items
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
