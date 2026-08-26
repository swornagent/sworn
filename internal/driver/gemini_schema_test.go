package driver

import (
	"bytes"
	"encoding/json"
	"testing"
)

// The generateContent API validates functionDeclarations parameters against
// its Schema proto and rejects the whole request over unknown keywords.
// Every sworn tool pins additionalProperties, so declarations must render in
// the supported subset (confirmed live 2026-08-18: HTTP 400 `Unknown name
// "additionalProperties"` for the verbatim sworn_submit schema).
func TestGeminiParameterSchemaSubset(t *testing.T) {
	t.Run("strips additionalProperties at every schema node", func(t *testing.T) {
		rendered, err := geminiParameterSchema([]byte(swornSubmitInputSchema))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(rendered, []byte(`"additionalProperties"`)) {
			t.Fatalf("rendered schema still carries additionalProperties: %s", rendered)
		}
		var schema map[string]any
		if err := json.Unmarshal(rendered, &schema); err != nil {
			t.Fatal(err)
		}
		submission, ok := schema["properties"].(map[string]any)["submission"].(map[string]any)
		if !ok {
			t.Fatal("submission property lost in rendering")
		}
		if _, ok := submission["properties"].(map[string]any)["responsibility"]; !ok {
			t.Fatal("nested property lost in rendering")
		}
	})

	t.Run("keeps a property literally named additionalProperties", func(t *testing.T) {
		raw := []byte(`{"type":"object","properties":{"additionalProperties":{"type":"string"}},"additionalProperties":false}`)
		rendered, err := geminiParameterSchema(raw)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(rendered, &schema); err != nil {
			t.Fatal(err)
		}
		if _, ok := schema["properties"].(map[string]any)["additionalProperties"]; !ok {
			t.Fatal("property named additionalProperties was stripped")
		}
		if _, ok := schema["additionalProperties"]; ok {
			t.Fatal("schema keyword additionalProperties survived")
		}
	})

	t.Run("recurses through items", func(t *testing.T) {
		raw := []byte(`{"type":"object","properties":{"list":{"type":"array","items":{"type":"object","properties":{"value":{"type":"string"}},"additionalProperties":false}}},"additionalProperties":false}`)
		rendered, err := geminiParameterSchema(raw)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(rendered, []byte(`"additionalProperties"`)) {
			t.Fatalf("items schema still carries additionalProperties: %s", rendered)
		}
	})

	t.Run("Bash command alias survives rendering same as script", func(t *testing.T) {
		declarations, err := geminiDeclarations(toolDefinitions(ReadWrite))
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range declarations {
			if declaration.Name != "Bash" {
				continue
			}
			var schema map[string]any
			if err := json.Unmarshal(declaration.Parameters, &schema); err != nil {
				t.Fatal(err)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("Bash schema has no properties: %s", declaration.Parameters)
			}
			if _, ok := properties["script"]; !ok {
				t.Fatal("Bash schema lost script")
			}
			if _, ok := properties["command"]; !ok {
				t.Fatal("Bash schema lost command")
			}
			return
		}
		t.Fatal("Bash declaration not found")
	})

	t.Run("declarations render every sworn tool in the subset", func(t *testing.T) {
		declarations, err := geminiDeclarations(toolDefinitions(ReadWrite))
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range declarations {
			if bytes.Contains(declaration.Parameters, []byte(`"additionalProperties"`)) {
				t.Fatalf("tool %s still carries additionalProperties", declaration.Name)
			}
		}
	})
}
