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

// SpecSchemaURI is the canonical $schema URI for spec-v1.json.
const SpecSchemaURI = "https://baton.sawy3r.net/schemas/spec-v1.json"

// ProofSchemaURI is the canonical $schema URI for proof-v1.json.
const ProofSchemaURI = "https://baton.sawy3r.net/schemas/proof-v1.json"

// JourneysSchemaURI is the canonical $schema URI for journeys-v1.json.
const JourneysSchemaURI = "https://baton.sawy3r.net/schemas/journeys-v1.json"

// AttestationsSchemaURI is the canonical $schema URI for attestations-v1.json.
const AttestationsSchemaURI = "https://baton.sawy3r.net/schemas/attestations-v1.json"
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
	case "spec-v1":
		return validateSpec(data)
	case "proof-v1":
		return validateProof(data)
	case "journeys-v1":
		return validateJourneys(data)
	case "attestations-v1":
		return validateAttestations(data)
	default:		return fmt.Errorf("validator: no validation rules for schema %q", schemaName)
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

// validateSpec validates data against the spec-v1 schema.
func validateSpec(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("validator: invalid JSON: %w", err)
	}

	if len(m) == 0 {
		return fmt.Errorf("validator: empty object — required fields missing: schema_version, slice_id, release")
	}

	// schema_version must be 1.
	sv, ok := m["schema_version"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"schema_version\"")
	}
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

	// slice_id must be present and non-empty.
	if err := checkNonEmptyString(m, "slice_id"); err != nil {
		return err
	}

	// release must be present and non-empty.
	if err := checkNonEmptyString(m, "release"); err != nil {
		return err
	}

	// $schema must be the canonical spec URI if present.
	if schema, ok := m["$schema"]; ok {
		if s, isStr := schema.(string); isStr && s != SpecSchemaURI && s != "" {
			return fmt.Errorf("validator: $schema must be %q, got %q", SpecSchemaURI, s)
		}
	}

	return nil
}

// validateProof validates data against the proof-v1 schema.
func validateProof(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("validator: invalid JSON: %w", err)
	}

	if len(m) == 0 {
		return fmt.Errorf("validator: empty object — required fields missing: schema_version, slice_id, release")
	}

	// schema_version must be 1.
	sv, ok := m["schema_version"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"schema_version\"")
	}
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

	// slice_id must be present and non-empty.
	if err := checkNonEmptyString(m, "slice_id"); err != nil {
		return err
	}

	// release must be present and non-empty.
	if err := checkNonEmptyString(m, "release"); err != nil {
		return err
	}

	// $schema must be the canonical proof URI if present.
	if schema, ok := m["$schema"]; ok {
		if s, isStr := schema.(string); isStr && s != ProofSchemaURI && s != "" {
			return fmt.Errorf("validator: $schema must be %q, got %q", ProofSchemaURI, s)
		}
	}

	return nil
}

// checkNonEmptyString verifies that m[f] is a non-empty string.
func checkNonEmptyString(m map[string]interface{}, f string) error {
	v, ok := m[f]
	if !ok {
		return fmt.Errorf("validator: missing required field %q", f)
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return fmt.Errorf("validator: field %q must be a non-empty string", f)
	}
	return nil
}
// validateJourneys validates data against the journeys-v1 schema.
func validateJourneys(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("validator: invalid JSON: %w", err)
	}

	if len(m) == 0 {
		return fmt.Errorf("validator: empty object — required fields missing: version, created_at, updated_at, ratification, journeys")
	}

	// $schema must be the canonical URI.
	if err := checkStringField(m, "$schema", JourneysSchemaURI); err != nil {
		return err
	}

	// version must be present.
	if _, ok := m["version"]; !ok {
		return fmt.Errorf("validator: missing required field \"version\"")
	}

	// created_at and updated_at must be non-empty strings.
	if err := checkNonEmptyString(m, "created_at"); err != nil {
		return err
	}
	if err := checkNonEmptyString(m, "updated_at"); err != nil {
		return err
	}

	// ratification must be an object with is_ratified.
	rat, ok := m["ratification"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"ratification\"")
	}
	ratMap, ok := rat.(map[string]interface{})
	if !ok {
		return fmt.Errorf("validator: ratification must be an object, got %T", rat)
	}
	if _, ok := ratMap["is_ratified"]; !ok {
		return fmt.Errorf("validator: ratification.is_ratified is required")
	}

	// by and at must be present (may be empty when unratified).
	// Only require non-empty when ratified.
	if isRat, _ := ratMap["is_ratified"].(bool); isRat {
		if err := checkNonEmptyString(ratMap, "by"); err != nil {
			return fmt.Errorf("validator: ratification.%w", err)
		}
		if err := checkNonEmptyString(ratMap, "at"); err != nil {
			return fmt.Errorf("validator: ratification.%w", err)
		}
	} else {
		// Ensure by and at keys exist even if empty.
		if _, ok := ratMap["by"]; !ok {
			return fmt.Errorf("validator: ratification.by is required")
		}
		if _, ok := ratMap["at"]; !ok {
			return fmt.Errorf("validator: ratification.at is required")
		}
	}

	// journeys must be present (may be empty array).
	if _, ok := m["journeys"]; !ok {
		return fmt.Errorf("validator: missing required field \"journeys\"")
	}

	return nil
}

// validateAttestations validates data against the attestations-v1 schema.
func validateAttestations(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("validator: invalid JSON: %w", err)
	}

	if len(m) == 0 {
		return fmt.Errorf("validator: empty object — required fields missing: version, ratification, boundary, attestations")
	}

	// $schema must be the canonical URI.
	if err := checkStringField(m, "$schema", AttestationsSchemaURI); err != nil {
		return err
	}

	// version must be present.
	if _, ok := m["version"]; !ok {
		return fmt.Errorf("validator: missing required field \"version\"")
	}

	// ratification must be an object with is_ratified.
	rat, ok := m["ratification"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"ratification\"")
	}
	ratMap, ok := rat.(map[string]interface{})
	if !ok {
		return fmt.Errorf("validator: ratification must be an object, got %T", rat)
	}
	if _, ok := ratMap["is_ratified"]; !ok {
		return fmt.Errorf("validator: ratification.is_ratified is required")
	}

	// by and at must be present (may be empty when unratified).
	if isRat, _ := ratMap["is_ratified"].(bool); isRat {
		if err := checkNonEmptyString(ratMap, "by"); err != nil {
			return fmt.Errorf("validator: ratification.%w", err)
		}
		if err := checkNonEmptyString(ratMap, "at"); err != nil {
			return fmt.Errorf("validator: ratification.%w", err)
		}
	} else {
		if _, ok := ratMap["by"]; !ok {
			return fmt.Errorf("validator: ratification.by is required")
		}
		if _, ok := ratMap["at"]; !ok {
			return fmt.Errorf("validator: ratification.at is required")
		}
	}
	// boundary must be an object with name, mock_banned, entitlement_boundary.
	bound, ok := m["boundary"]
	if !ok {
		return fmt.Errorf("validator: missing required field \"boundary\"")
	}
	boundMap, ok := bound.(map[string]interface{})
	if !ok {
		return fmt.Errorf("validator: boundary must be an object, got %T", bound)
	}
	if err := checkNonEmptyString(boundMap, "name"); err != nil {
		return fmt.Errorf("validator: boundary.%w", err)
	}
	if _, ok := boundMap["mock_banned"]; !ok {
		return fmt.Errorf("validator: boundary.mock_banned is required")
	}
	if err := checkNonEmptyString(boundMap, "entitlement_boundary"); err != nil {
		return fmt.Errorf("validator: boundary.%w", err)
	}

	// attestations must be present (may be empty array).
	if _, ok := m["attestations"]; !ok {
		return fmt.Errorf("validator: missing required field \"attestations\"")
	}

	return nil
}

// checkStringField verifies that m[f] is a string equal to expected.
func checkStringField(m map[string]interface{}, f string, expected string) error {
	v, ok := m[f]
	if !ok {
		return fmt.Errorf("validator: missing required field %q", f)
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("validator: field %q must be a string, got %T", f, v)
	}
	if s != expected {
		return fmt.Errorf("validator: field %q must be %q, got %q", f, expected, s)
	}
	return nil
}
