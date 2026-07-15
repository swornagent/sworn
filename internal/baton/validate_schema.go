package baton

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/swornagent/sworn/internal/baton/schemas"
)

const v015BoardPathPattern = `^(?!/)(?!.*(?:^|/)\.\.?($|/)).+$`

type v015BoardPathRegexp struct{}

func (v015BoardPathRegexp) MatchString(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func (v015BoardPathRegexp) String() string { return v015BoardPathPattern }

func compileCompatibleRegexp(expression string) (jsonschema.Regexp, error) {
	if expression == v015BoardPathPattern {
		return v015BoardPathRegexp{}, nil
	}
	return regexp.Compile(expression)
}

// Real draft-2020-12 schema validation (ADR-0011 keystone, step 1).
//
// The legacy Validate() dispatches to hand-rolled per-type field checks that
// verify a handful of top-level fields and stop — nested shapes, item types,
// enums, patterns, and minLengths are never evaluated, so the embedded schemas
// are decorative (ADR-0011 §2, finding 1). ValidateSchema replaces that with an
// actual draft-2020-12 evaluator over the embedded schema. The rewire of
// Validate() to call this (and the D6 field-drift reconciliation it surfaces)
// lands as a deliberate follow-up so a single commit does not flip every
// record's enforcement at once.

var (
	schemaCacheMu sync.Mutex
	schemaCache   = map[string]*jsonschema.Schema{}
)

// schemaURL is the canonical $id used to register an embedded schema with the
// compiler. Matches the $id the schema files declare.
func schemaURL(name string) string {
	return "https://baton.sawy3r.net/schemas/" + name + ".json"
}

// CompiledSchema returns the compiled, cached draft-2020-12 schema for a short
// name (e.g. "slice-status-v1"). Compilation is done once per name.
func CompiledSchema(name string) (*jsonschema.Schema, error) {
	schemaCacheMu.Lock()
	defer schemaCacheMu.Unlock()
	if s, ok := schemaCache[name]; ok {
		return s, nil
	}
	raw, ok := schemas.SchemaMap[name]
	if !ok {
		return nil, fmt.Errorf("validator: unknown schema %q", name)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("validator: parse schema %q: %w", name, err)
	}
	c := jsonschema.NewCompiler()
	c.UseRegexpEngine(compileCompatibleRegexp)
	url := schemaURL(name)
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("validator: add schema %q: %w", name, err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("validator: compile schema %q: %w", name, err)
	}
	schemaCache[name] = s
	return s, nil
}

func compileSchemaBytes(name string, raw []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("validator: parse schema %q: %w", name, err)
	}
	c := jsonschema.NewCompiler()
	c.UseRegexpEngine(compileCompatibleRegexp)
	url := schemaURL(name)
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("validator: add schema %q: %w", name, err)
	}
	schema, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("validator: compile schema %q: %w", name, err)
	}
	return schema, nil
}

// ValidateSchema validates a JSON payload against the embedded JSON Schema by
// short name, using full draft-2020-12 evaluation. Returns nil on conformance,
// a descriptive error otherwise. This is the enforcement the authoring-path
// structured-output call (ADR-0009) round-trips through: emit → ValidateSchema
// → accept, fail-closed on error.
func ValidateSchema(name string, data []byte) error {
	s, err := CompiledSchema(name)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("validator: parse %q payload: %w", name, err)
	}
	if err := s.Validate(inst); err != nil {
		return fmt.Errorf("validator: %q does not conform: %w", name, err)
	}
	return nil
}
