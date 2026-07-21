package protocol

import "fmt"

// VerifierAssessmentOutputSchema returns the canonical engine-owned JSON Schema
// supplied to a verifier model. It deliberately describes only assessment
// content; verdict identity, freshness, agent, dispatch, and timestamps remain
// absent because the engine owns those facts.
func VerifierAssessmentOutputSchema() ([]byte, error) {
	id := map[string]any{
		"type":      "string",
		"maxLength": 128,
		"pattern":   `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`,
	}
	packID := map[string]any{
		"type":      "string",
		"maxLength": 97,
		"pattern":   `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}@[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`,
	}
	evidenceIDs := map[string]any{
		"type": "array", "items": id, "maxItems": maximumVerifierAssessmentReferenceItems,
	}
	acceptanceResult := strictVerifierSchemaObject(
		[]string{"acceptance_id", "outcome", "evidence_ids", "summary"},
		map[string]any{
			"acceptance_id": id,
			"outcome":       verifierSchemaEnum("pass", "fail", "inconclusive"),
			"evidence_ids":  evidenceIDs,
			"summary":       verifierSchemaSummary(maximumVerifierResultSummaryCodePoints),
		},
	)
	assuranceResult := strictVerifierSchemaObject(
		[]string{"pack", "outcome", "evidence_ids", "summary"},
		map[string]any{
			"pack":         packID,
			"outcome":      verifierSchemaEnum("pass", "fail", "inconclusive"),
			"evidence_ids": evidenceIDs,
			"summary":      verifierSchemaSummary(maximumVerifierResultSummaryCodePoints),
		},
	)
	finding := strictVerifierSchemaObject(
		[]string{"id", "kind", "principle", "severity", "summary", "acceptance_ids", "evidence_ids"},
		map[string]any{
			"id":             id,
			"kind":           verifierSchemaEnum("authority", "contract", "implementation", "evidence", "environment", "composition"),
			"principle":      verifierSchemaEnum("B1", "B2", "B3", "B4", "B5"),
			"severity":       verifierSchemaEnum("blocking", "non_blocking"),
			"summary":        verifierSchemaSummary(maximumVerifierResultSummaryCodePoints),
			"acceptance_ids": map[string]any{"type": "array", "items": id, "maxItems": maximumVerifierAssessmentReferenceItems},
			"evidence_ids":   evidenceIDs,
		},
	)
	schema := strictVerifierSchemaObject(
		[]string{"schema_version", "outcome", "summary", "acceptance_results", "assurance_results", "findings"},
		map[string]any{
			"schema_version": verifierSchemaEnum(VerifierAssessmentSchemaVersion),
			"outcome":        verifierSchemaEnum("PASS", "FAIL", "SPEC_BLOCK", "INCONCLUSIVE"),
			"summary":        verifierSchemaSummary(maximumVerifierAssessmentSummaryCodePoints),
			"acceptance_results": map[string]any{
				"type": "array", "minItems": 1, "maxItems": maximumVerifierAssessmentCollectionItems, "items": acceptanceResult,
			},
			"assurance_results": map[string]any{"type": "array", "maxItems": maximumVerifierAssessmentCollectionItems, "items": assuranceResult},
			"findings":          map[string]any{"type": "array", "maxItems": maximumVerifierAssessmentCollectionItems, "items": finding},
		},
	)
	canonical, err := EncodeCanonical(schema)
	if err != nil {
		return nil, fmt.Errorf("canonicalize verifier assessment output schema: %w", err)
	}
	if len(canonical) > MaximumVerifierAssessmentBytes {
		return nil, fmt.Errorf("verifier assessment output schema exceeds its byte ceiling")
	}
	return canonical, nil
}

// VerifierAssessmentOutputSchemaDigest returns the raw digest of the canonical
// schema bytes staged for the verifier process.
func VerifierAssessmentOutputSchemaDigest() (string, error) {
	contents, err := VerifierAssessmentOutputSchema()
	if err != nil {
		return "", err
	}
	return RawDigest(contents), nil
}

func strictVerifierSchemaObject(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": required, "properties": properties,
	}
}

func verifierSchemaEnum(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func verifierSchemaSummary(maximum int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": maximum}
}
