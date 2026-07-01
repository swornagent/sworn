// Package spec provides shared read access to slice spec artefacts.
//
// It is the single reader for spec.json (spec-v1) records so that gates and
// verifiers consume one parser instead of each growing a bespoke scanner
// (sworn#22). Writers live in internal/implement (spec_record.go); this
// package owns the read side only.
package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AC is one acceptance criterion in a spec-v1 record.
type AC struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	Type        string `json:"type,omitempty"`
	EARSKeyword string `json:"ears_keyword,omitempty"`
}

// Record is the subset of a spec-v1 spec.json that read-side consumers use.
type Record struct {
	SliceID            string   `json:"slice_id"`
	Release            string   `json:"release"`
	UserOutcome        string   `json:"user_outcome"`
	CoversNeeds        []string `json:"covers_needs"`
	AcceptanceCriteria []AC     `json:"acceptance_criteria"`
}

// ReadRecord loads spec.json from a slice directory.
//
// It returns (nil, nil) when spec.json does not exist, so callers can fall
// back to spec.md for legacy (markdown-era) releases. Any other read or
// parse failure is returned as an error — a malformed record must surface,
// not silently read as "no spec" (fail closed).
func ReadRecord(sliceDir string) (*Record, error) {
	path := filepath.Join(sliceDir, "spec.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("spec: read %s: %w", path, err)
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("spec: parse %s: %w", path, err)
	}
	return &r, nil
}
