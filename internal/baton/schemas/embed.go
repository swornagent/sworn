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

// BoardV1 is the canonical board-v1.json schema, embedded at build time.
// It validates every board.json written by the board package.
//
//go:embed board-v1.json
var BoardV1 []byte

// SpecV1 is the canonical spec-v1.json schema, embedded at build time.
// It validates every spec.json written by the implementer.
//
//go:embed spec-v1.json
var SpecV1 []byte

// ProofV1 is the canonical proof-v1.json schema, embedded at build time.
// It validates every proof.json written by the implementer.
//
//go:embed proof-v1.json
var ProofV1 []byte

// SchemaMap maps a short schema name (e.g. "slice-status-v1") to its
// embedded bytes. Callers use this to look up the schema by the name
// they store in the $schema field.
var SchemaMap = map[string][]byte{
	"slice-status-v1": SliceStatusV1,
	"board-v1":        BoardV1,
	"spec-v1":         SpecV1,
	"proof-v1":        ProofV1,
}