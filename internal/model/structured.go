package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Structured-output plumbing shared by the OAI and OpenAIResponses drivers
// (ADR-0011 authoring path). The two emission mechanisms are:
//
//   - native strict json_schema (response_format / responses text.format), which
//     requires the LENIENT canonical schema be projected to OpenAI's strict
//     profile at call time (D1: lenient canonical for storage, strict at call);
//   - a single forced function tool whose parameters ARE the schema — the
//     fallback for models without strict json_schema support (e.g. DeepSeek).
//
// The fixed function name used by the tool-call fallback path. tool_choice
// forces this function so the model must emit the object as its arguments.
const structuredToolName = "emit_structured_output"

// StructuredMode selects how an OAI driver constrains structured output in
// ChatStructured. The zero value (structuredUnsupported) means the driver does
// not advertise CapStructuredOutput.
type StructuredMode int

const (
	structuredUnsupported StructuredMode = iota
	// StructuredResponseFormat uses native OpenAI strict json_schema via
	// response_format; the lenient schema is strict-projected at call time.
	StructuredResponseFormat
	// StructuredToolCall uses a single forced function tool whose parameters
	// ARE the schema. The fallback for models without strict response_format
	// (e.g. DeepSeek — confirmed working). No projection needed: a tool's
	// parameters accept full JSON Schema (additionalProperties:true is fine).
	StructuredToolCall
)

// responseFormat is the /chat/completions response_format payload for strict
// structured output: {"type":"json_schema","json_schema":{...}}.
type responseFormat struct {
	Type       string          `json:"type"`
	JSONSchema *jsonSchemaSpec `json:"json_schema,omitempty"`
}

// jsonSchemaSpec is the inner json_schema object OpenAI strict mode requires:
// a name (^[a-zA-Z0-9_-]+$), the schema itself, and strict:true.
type jsonSchemaSpec struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

// strictProjection transforms a lenient canonical JSON Schema into the profile
// OpenAI strict structured outputs require, WITHOUT mutating the stored
// canonical schema (D1). For every object node it:
//   - sets additionalProperties:false,
//   - adds every property key to required, and
//   - widens any property that was NOT required in the lenient schema to be
//     nullable (its "type" gains "null"), so "all-required" is satisfiable.
//
// It recurses through properties, array items, $defs/definitions, and the
// anyOf/oneOf/allOf combinators. $ref strings are left intact (they resolve to
// transformed $defs). Other keywords pass through unchanged: structured-output
// TARGET schemas must stay within OpenAI's strict-supported keyword subset —
// a documented constraint on the schema author (see ADR-0011 §3 / D1), not a
// transform this function performs.
func strictProjection(lenient []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(lenient, &root); err != nil {
		return nil, fmt.Errorf("model: parse schema for strict projection: %w", err)
	}
	strictNode(root)
	out, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("model: marshal strict schema: %w", err)
	}
	return out, nil
}

// strictNode applies the strict transform to a single schema node in place,
// then recurses into nested schema positions.
func strictNode(node map[string]any) {
	if props, ok := node["properties"].(map[string]any); ok {
		// Snapshot the lenient required set BEFORE we overwrite it below.
		wasRequired := map[string]bool{}
		if reqList, ok := node["required"].([]any); ok {
			for _, r := range reqList {
				if s, ok := r.(string); ok {
					wasRequired[s] = true
				}
			}
		}
		keys := make([]string, 0, len(props))
		for key, raw := range props {
			keys = append(keys, key)
			if child, ok := raw.(map[string]any); ok {
				strictNode(child)
				if !wasRequired[key] {
					makeNullable(child)
				}
			}
		}
		// Deterministic required order (stable wire output / testable).
		sort.Strings(keys)
		required := make([]any, len(keys))
		for i, k := range keys {
			required[i] = k
		}
		node["required"] = required
		node["additionalProperties"] = false
	}

	if items, ok := node["items"].(map[string]any); ok {
		strictNode(items)
	}
	for _, defsKey := range []string{"$defs", "definitions"} {
		if defs, ok := node[defsKey].(map[string]any); ok {
			for _, raw := range defs {
				if d, ok := raw.(map[string]any); ok {
					strictNode(d)
				}
			}
		}
	}
	for _, comb := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := node[comb].([]any); ok {
			for _, raw := range arr {
				if c, ok := raw.(map[string]any); ok {
					strictNode(c)
				}
			}
		}
	}
}

// makeNullable widens a schema node's "type" to include "null" so an
// optional-in-lenient field can satisfy strict mode's all-required rule by
// being present-but-null. Nodes without an explicit string/array "type" (e.g.
// a bare $ref or enum-only field) are left unchanged — the schema author keeps
// such optionals required (documented limitation, ADR-0011 §3).
func makeNullable(node map[string]any) {
	switch t := node["type"].(type) {
	case string:
		if t != "null" {
			node["type"] = []any{t, "null"}
		}
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok && s == "null" {
				return // already nullable
			}
		}
		node["type"] = append(t, "null")
	}
}

// schemaName derives an OpenAI-API-compatible schema name (^[a-zA-Z0-9_-]+$)
// from the schema's $id basename or title, defaulting to "structured_output".
func schemaName(schema []byte) string {
	var meta struct {
		ID    string `json:"$id"`
		Title string `json:"title"`
	}
	_ = json.Unmarshal(schema, &meta)
	raw := meta.ID
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		raw = raw[i+1:]
	}
	raw = strings.TrimSuffix(raw, ".json")
	if raw == "" {
		raw = meta.Title
	}
	if name := sanitizeName(raw); name != "" {
		return name
	}
	return "structured_output"
}

// sanitizeName keeps only characters OpenAI's schema-name pattern allows,
// mapping spaces and dots to underscores.
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ', r == '.':
			b.WriteRune('_')
		}
	}
	return b.String()
}

// normaliseStructuredContent applies the wire-level fail-closed guard shared by
// every ChatStructured path: the emitted content must be non-empty and parse as
// a JSON object. Semantic validation against the canonical schema by name is the
// caller's job (baton.ValidateSchema).
func normaliseStructuredContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("model: structured output: empty content")
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(content), &probe); err != nil {
		return "", fmt.Errorf("model: structured output: content is not a JSON object: %w", err)
	}
	return content, nil
}
