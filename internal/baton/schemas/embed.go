// Package schemas embeds canonical Baton JSON Schemas into the binary.
// Schema files are validated at build time via //go:embed — a missing
// or unreadable schema file is a compile error, not a runtime surprise.
//
// Stdlib only — zero runtime dependencies.
package schemas

import _ "embed"

// SliceStatusV1 is the canonical slice-status-v1.json schema, embedded
// at build time. It validates every status.json written by the Sworn
// orchestrator.
//
//go:embed slice-status-v1.json
var SliceStatusV1 []byte

// SchemaMap maps a short schema name (e.g. "slice-status-v1") to its
// embedded bytes. Callers use this to look up the schema by the name
// they store in the $schema field.
var SchemaMap = map[string][]byte{
	"slice-status-v1": SliceStatusV1,
}