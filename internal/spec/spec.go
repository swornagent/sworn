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
//
// At baton v0.10.0 the AC item is strict (additionalProperties:false, allowing
// only id/text/ears_pattern/test_refs): the retired sworn-local type/ears_keyword
// fields are gone, replaced by the canonical ears_pattern (the EARS pattern class
// itself — "ubiquitous", "event-driven", "unwanted-behaviour", …). The
// internal/ears classifier reads EARSPattern directly (S12 record migration
// mapped every legacy type -> ears_pattern, sworn#95).
type AC struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	EARSPattern string `json:"ears_pattern,omitempty"`
}

// Record is the subset of a spec-v1 spec.json that read-side consumers use.
//
// in_scope/out_of_scope became required spec-v1 fields at baton v0.10.0
// (previously scraped `## In scope` / `## Out of scope` spec.md headings); the
// reader parses and exposes them so gates and verifiers read one boundary
// source instead of re-scraping markdown.
type Record struct {
	SliceID            string   `json:"slice_id"`
	Release            string   `json:"release"`
	UserOutcome        string   `json:"user_outcome"`
	InScope            []string `json:"in_scope"`
	OutOfScope         []string `json:"out_of_scope"`
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
