// Package baton provides Baton-protocol primitives: schema embedding,
// record validation, and canonical-file fetching.
//
// Stdlib only — zero runtime dependencies.
package baton

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/baton/schemas"
)

// SchemaURI is the canonical $schema URI for slice-status-v1.json.
// Every Write() sets this value automatically.
const SchemaURI = "https://baton.sawy3r.net/schemas/slice-status-v1.json"

// BoardSchemaURI is the canonical $schema URI for board-v1.json.
const BoardSchemaURI = "https://baton.sawy3r.net/schemas/board-v1.json"

// requiredFields lists the top-level string fields that must be present
// and non-empty in every status.json payload.
var requiredFields = []string{"slice_id", "release", "track", "state"}

// validStates is the canonical set of valid slice states.
var validStates = map[string]bool{
	"planned":             true,
	"design_review":       true,
	"in_progress":         true,
	"implemented":         true,
	"verified":            true,
	"failed_verification": true,
	"blocked":             true,
	"deferred":            true,
	"shipped":             true,
}

// validTrackStates is the canonical set of valid track states.
var validTrackStates = map[string]bool{
	"planned":     true,
	"in_progress": true,
	"merged":      true,
}

// Validate checks that data conforms to the embedded schema named by
// schemaName. It returns nil on success, or an error describing the
// first violation found.
//
// Validation is a structural required-fields check (option b per the S13
// spec Risks section): it verifies that required string fields are
// present and non-empty, that state is a recognised value, that the
// $schema field is the canonical URI, and that the verification.result
// object is present (for slice-status-v1). Full JSON Schema validation
// is deferred to a follow-up ADR (ADR-0007).
func Validate(schemaName string, data []byte) error {
	// Confirm the schema exists at build time.
	if _, ok := schemas.SchemaMap[schemaName]; !ok {
		return fmt.Errorf("validator: unknown schema %q", schemaName)
	}

	switch schemaName {
	case "slice-status-v1":
		return validateSliceStatus(data)
	case "board-v1":
		return validateBoard(data)
	default:
		return fmt.Errorf("validator: no validation rules for schema %q", schemaName)
	}
}

func validateSliceStatus(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("validator: invalid JSON: %w", err)
	}

	// Empty object check.
	if len(m) == 0 {
		return fmt.Errorf("validator: empty object — required fields missing: %s", strings.Join(requiredFields, ", "))
	}

	// Required string fields.
	for _, f := range requiredFields {
		v, ok := m[f]
		if !ok {
			return fmt.Errorf("validator: missing required field %q", f)
		}
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("validator: field %q must be a string, got %T", f, v)
		}
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("validator: field %q must be non-empty", f)
		}
	}

	// State must be a recognised value.
	if state, ok := m["state"].(string); ok && !validStates[state] {
		return fmt.Errorf("validator: unknown state %q", state)
	}

	// $schema must be the canonical URI.
	if schema, ok := m["$schema"]; ok {
		if s, isStr := schema.(string); isStr && s != SchemaURI {
			return fmt.Errorf("validator: $schema must be %q, got %q", SchemaURI, s)
		}
	}

	// verification.result must be present.
	verif, ok := m["verification"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"verification\"")
	}
	verifMap, ok := verif.(map[string]interface{})
	if !ok {
		return fmt.Errorf("validator: verification must be an object, got %T", verif)
	}
	if _, ok := verifMap["result"]; !ok {
		return fmt.Errorf("validator: verification.result is required")
	}

	return nil
}

func validateBoard(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("validator: invalid JSON: %w", err)
	}

	if len(m) == 0 {
		return fmt.Errorf("validator: empty object — required fields missing: schema_version, release, tracks")
	}

	// schema_version must be 1.
	sv, ok := m["schema_version"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"schema_version\"")
	}
	// schema_version can be float64 from JSON unmarshal.
	var svInt int
	switch v := sv.(type) {
	case float64:
		svInt = int(v)
	case int:
		svInt = v
	default:
		return fmt.Errorf("validator: schema_version must be a number, got %T", sv)
	}
	if svInt != 1 {
		return fmt.Errorf("validator: schema_version must be 1, got %d", svInt)
	}

	// release must be present and non-empty.
	rel, ok := m["release"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"release\"")
	}
	relStr, ok := rel.(string)
	if !ok || strings.TrimSpace(relStr) == "" {
		return fmt.Errorf("validator: release must be a non-empty string")
	}

	// tracks must be present.
	tracksRaw, ok := m["tracks"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"tracks\"")
	}
	tracks, ok := tracksRaw.([]interface{})
	if !ok {
		return fmt.Errorf("validator: tracks must be an array, got %T", tracksRaw)
	}

	// Each track must have id, state, worktree_branch.
	for i, t := range tracks {
		tm, ok := t.(map[string]interface{})
		if !ok {
			return fmt.Errorf("validator: tracks[%d] must be an object, got %T", i, t)
		}
		// id
		id, ok := tm["id"]
		if !ok {
			return fmt.Errorf("validator: tracks[%d] missing required field \"id\"", i)
		}
		idStr, ok := id.(string)
		if !ok || strings.TrimSpace(idStr) == "" {
			return fmt.Errorf("validator: tracks[%d].id must be a non-empty string", i)
		}
		// state
		state, ok := tm["state"]
		if !ok {
			return fmt.Errorf("validator: tracks[%d] (%s) missing required field \"state\"", i, idStr)
		}
		stateStr, ok := state.(string)
		if !ok || !validTrackStates[stateStr] {
			return fmt.Errorf("validator: tracks[%d] (%s) invalid state %q", i, idStr, stateStr)
		}
		// worktree_branch
		wb, ok := tm["worktree_branch"]
		if !ok {
			return fmt.Errorf("validator: tracks[%d] (%s) missing required field \"worktree_branch\"", i, idStr)
		}
		wbStr, ok := wb.(string)
		if !ok || strings.TrimSpace(wbStr) == "" {
			return fmt.Errorf("validator: tracks[%d] (%s) worktree_branch must be a non-empty string", i, idStr)
		}
	}

	// $schema must be the canonical board URI if present.
	if schema, ok := m["$schema"]; ok {
		if s, isStr := schema.(string); isStr && s != BoardSchemaURI && s != "" {
			return fmt.Errorf("validator: $schema must be %q, got %q", BoardSchemaURI, s)
		}
	}

	return nil
}