// Package spec provides shared read access to slice spec artefacts.
//
// It is the single reader for spec.json (spec-v1) records so that gates and
// verifiers consume one parser instead of each growing a bespoke scanner
// (sworn#22). Writers live in internal/implement (spec_record.go); this
// package owns the read side only.
package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AC is one acceptance criterion in a spec-v1 record.
//
// At baton v0.10.0 the AC item is strict (additionalProperties:false, allowing
// only id/text/ears_pattern/test_refs): the retired sworn-local type/ears_keyword
// fields are gone, replaced by the canonical ears_pattern (the EARS pattern class
// itself — "ubiquitous", "event-driven", "unwanted-behaviour", …). The
// internal/ears classifier reads EARSPattern directly (S12 record migration
// mapped every legacy type -> ears_pattern, sworn#95).
//
// TestRefs exposes the spec-v1 AC `test_refs` array (a real spec-v1 field, see
// the strict AC item above). It is the machine-readable AC->test link that the
// need->AC->test golden thread (internal/rtm) resolves against on a spec.json-only
// release, where there is no spec.md "Required tests" section to scrape
// (S01-spec-json-read-conformance AC-06, Coach-ratified Pin 1).
type AC struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	EARSPattern string   `json:"ears_pattern,omitempty"`
	TestRefs    []string `json:"test_refs,omitempty"`
}

// Reference is one typed, direct normative input to spec-ambiguity. Exactly
// one identifier field is populated according to Kind; spec-v1 validates the
// discriminated shape before the resolver reads it.
type Reference struct {
	Kind       string `json:"kind"`
	ContractID string `json:"contract_id,omitempty"`
	SliceID    string `json:"slice_id,omitempty"`
	Path       string `json:"path,omitempty"`
}

// Record is the subset of a spec-v1 spec.json that read-side consumers use.
//
// in_scope/out_of_scope became required spec-v1 fields at baton v0.10.0
// (previously scraped `## In scope` / `## Out of scope` spec.md headings); the
// reader parses and exposes them so gates and verifiers read one boundary
// source instead of re-scraping markdown.
type Record struct {
	SliceID            string      `json:"slice_id"`
	Release            string      `json:"release"`
	UserOutcome        string      `json:"user_outcome"`
	InScope            []string    `json:"in_scope"`
	OutOfScope         []string    `json:"out_of_scope"`
	CoversNeeds        []string    `json:"covers_needs"`
	AcceptanceCriteria []AC        `json:"acceptance_criteria"`
	References         []Reference `json:"references,omitempty"`
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
	if err := DecodeJSONNoDuplicate(data, &r); err != nil {
		return nil, fmt.Errorf("spec: parse %s: %w", path, err)
	}
	return &r, nil
}

// LoadSpec resolves a slice's machine contract with the single, canonical
// precedence used across every read site: spec.json is preferred over spec.md
// whenever spec.json exists and carries at least one acceptance criterion, and
// is authoritative on disagreement (the ADR-0009 / internal/ears rule). spec.md
// is used only as the legacy fallback for pre-spec-v1 (markdown-era) slices with
// no spec.json.
//
// Return contract:
//   - (rec, "", nil)     — spec.json present with ACs; rec is authoritative.
//   - (nil, mdText, nil) — spec.json absent (legacy); mdText is the spec.md body.
//
// It FAILS CLOSED: a malformed spec.json surfaces as an error (mirroring
// ReadRecord / ears.go), never a silent fall-through to spec.md; and when
// spec.json is absent, a spec.md read failure is returned as an error.
//
// This helper is the "spec.json-preferred, spec.md-legacy-fallback" precedence
// as one implementation so callers do not each re-inline the branch
// (S01-spec-json-read-conformance AC-04). Callers keep their own package-local
// spec.md parsers for the legacy branch only.
func LoadSpec(sliceDir string) (rec *Record, mdText string, err error) {
	rec, err = ReadRecord(sliceDir)
	if err != nil {
		// Malformed spec.json — fail closed, do not fall back to spec.md.
		return nil, "", err
	}
	if rec != nil && len(rec.AcceptanceCriteria) > 0 {
		return rec, "", nil
	}
	// Legacy fallback: read spec.md.
	mdPath := filepath.Join(sliceDir, "spec.md")
	data, mdErr := os.ReadFile(mdPath)
	if mdErr != nil {
		return nil, "", fmt.Errorf("spec: no spec.json with ACs and read %s: %w", mdPath, mdErr)
	}
	return nil, string(data), nil
}

// SpecFilePath returns the path to a slice's authoritative machine-contract
// file: spec.json when it exists, else the spec.md legacy path. It lets callers
// pass a truthful, dir-anchored spec path (the file may not exist in the spec.md
// case — that is the legacy-absent case the reader handles).
func SpecFilePath(sliceDir string) string {
	jsonPath := filepath.Join(sliceDir, "spec.json")
	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath
	}
	return filepath.Join(sliceDir, "spec.md")
}

// RenderMarkdown renders a spec-v1 Record as a readable markdown spec body:
// User outcome, Acceptance checks (as "- [ ] AC-NN: text" bullets), In scope,
// and Out of scope. It gives prose consumers (the implementer prompt, the
// design/captain/verify legs) one uniform text surface derived from spec.json,
// so they do not need a spec.md body to feed the model.
func RenderMarkdown(rec *Record) string {
	if rec == nil {
		return ""
	}
	var b strings.Builder
	title := rec.SliceID
	if title == "" {
		title = "Slice"
	}
	b.WriteString("# " + title + "\n\n")
	if rec.UserOutcome != "" {
		b.WriteString("## User outcome\n\n")
		b.WriteString(rec.UserOutcome + "\n\n")
	}
	b.WriteString("## Acceptance checks\n\n")
	if len(rec.AcceptanceCriteria) == 0 {
		b.WriteString("(none)\n\n")
	} else {
		for _, ac := range rec.AcceptanceCriteria {
			if ac.ID != "" {
				b.WriteString("- [ ] " + ac.ID + ": " + ac.Text + "\n")
			} else {
				b.WriteString("- [ ] " + ac.Text + "\n")
			}
		}
		b.WriteString("\n")
	}
	if len(rec.InScope) > 0 {
		b.WriteString("## In scope\n\n")
		for _, s := range rec.InScope {
			b.WriteString("- " + s + "\n")
		}
		b.WriteString("\n")
	}
	if len(rec.OutOfScope) > 0 {
		b.WriteString("## Out of scope\n\n")
		for _, s := range rec.OutOfScope {
			b.WriteString("- " + s + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}
