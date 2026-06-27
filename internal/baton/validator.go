// Package baton provides Baton-protocol primitives: schema embedding,
// record validation, and canonical-file fetching.
//
// Stdlib only — zero runtime dependencies.
package baton

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/baton/schemas")

// SchemaURI is the canonical $schema URI for slice-status-v1.json.
// Every Write() sets this value automatically.
const SchemaURI = "https://baton.sawy3r.net/schemas/slice-status-v1.json"

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

// Validate checks that data conforms to the embedded schema named by
// schemaName. It returns nil on success, or an error describing the
// first violation found.
//
// Validation is a structural required-fields check (option b per the S13
// spec Risks section): it verifies that required string fields are
// present and non-empty, that state is a recognised value, that the
// $schema field is the canonical URI, and that the verification.result
// object is present. Full JSON Schema validation is deferred to a
// follow-up ADR (ADR-0007).
func Validate(schemaName string, data []byte) error {
	// Confirm the schema exists at build time.
	if _, ok := schemas.SchemaMap[schemaName]; !ok {
		return fmt.Errorf("validator: unknown schema %q", schemaName)
	}

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