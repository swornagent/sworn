package baton

import (
	"encoding/json"
	"fmt"
)

// Normalise is the D1 read-path transition shim (S11-baton-revendor). It maps a
// record written under a retired baton shape into canonical baton v0.10.0 form
// BEFORE validation, so an un-migrated on-disk record still parses while
// Validate / ValidateSchema and every schema's additionalProperties:false stay
// STRICTLY strict — this shim is the ONLY tolerance, it runs on read, and it is
// deleted wholesale by S12-record-migration once the on-disk data is migrated.
//
// The strip/map set is derived from the actual live-record-vs-strict-schema
// delta at baton v0.10.0 (design pin 4), covering every field the byte-identical
// v0.10.0 schemas forbid-but-legacy-records-carry:
//
//   - schema_version — retired across spec-v1 and board-v1 ($schema carries the
//     version); stripped wherever present.
//   - effort_complexity.quadrant — the retired enum names map to their v0.7.1
//     canonical names (chore->quick, epic->beast) on spec-v1 and slice-status-v1.
//     Only the NAME is mapped; the axes are untouched, so an already-inconsistent
//     rating (e.g. low/low/epic -> low/low/beast) still fails the strict checksum.
//   - board-v1 — a pure plan at v0.9.0/v0.10.0: the release-level
//     (release_worktree_path/release_worktree_branch) and track-level
//     (worktree_path/worktree_branch/state) fields are DERIVED from git refs, not
//     persisted, so they are stripped.
//   - spec-v1 acceptance_criteria items — additionalProperties:false at v0.10.0
//     ({id,text,ears_pattern,test_refs}); the retired spec.md scraper emitted
//     type/ears_keyword, which are stripped.
//
// Normalise is idempotent: a record already in canonical form round-trips
// unchanged. It fails closed only on non-JSON input; it never removes a field
// outside the enumerated retired set, leaving every other field for the strict
// validator to judge.
func Normalise(schemaName string, data []byte) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("normalise: invalid JSON: %w", err)
	}

	// schema_version is retired across spec-v1 and board-v1.
	delete(m, "schema_version")

	// Retired quadrant enum names map to their canonical replacements on any
	// record type carrying an effort_complexity rating (spec-v1, slice-status-v1).
	normaliseQuadrant(m)

	switch schemaName {
	case "board-v1":
		// board-v1 pure plan (invariant 5): worktree identity + state are derived.
		delete(m, "release_worktree_path")
		delete(m, "release_worktree_branch")
		if tracks, ok := m["tracks"].([]interface{}); ok {
			for _, t := range tracks {
				if tm, ok := t.(map[string]interface{}); ok {
					delete(tm, "worktree_path")
					delete(tm, "worktree_branch")
					delete(tm, "state")
				}
			}
		}
	case "spec-v1":
		// acceptance_criteria items are strict at v0.10.0; drop the retired
		// scraper fields.
		if acs, ok := m["acceptance_criteria"].([]interface{}); ok {
			for _, a := range acs {
				if am, ok := a.(map[string]interface{}); ok {
					delete(am, "type")
					delete(am, "ears_keyword")
				}
			}
		}
	}

	return json.Marshal(m)
}

// normaliseQuadrant maps the retired effort_complexity.quadrant enum names
// (chore->quick, epic->beast) in place. Only the enum NAME is rewritten; the
// effort/complexity axes are untouched, so the strict quadrant<->axes checksum
// still fails an inconsistent rating after normalisation.
func normaliseQuadrant(m map[string]interface{}) {
	ec, ok := m["effort_complexity"].(map[string]interface{})
	if !ok {
		return
	}
	q, ok := ec["quadrant"].(string)
	if !ok {
		return
	}
	switch q {
	case "chore":
		ec["quadrant"] = "quick"
	case "epic":
		ec["quadrant"] = "beast"
	}
}
