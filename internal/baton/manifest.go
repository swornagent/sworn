package baton

// S15-baton-version-handshake: the graded-schema-version manifest.
//
// SchemaManifest declares, for each vendored Baton schema (S11's
// schemas.SchemaMap), whether sworn GRADES it (enforces via Validate()'s
// switch, or a direct ValidateSchema(...) call site on the authoring path)
// or carries it ADVISORY-only (vendored + version-declared, not yet
// enforced). `sworn doctor` renders this manifest (cmd/sworn/doctor.go,
// Group 1b) so a future vendoring bump that adds or removes a schema
// without updating the classification table WARNs instead of silently
// drifting — the scar this manages is baton#54/#55/#58 (vendored schemas
// at one version while the binary graded stale shapes, silently).

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
)

// GradeStatus classifies whether sworn enforces (Graded) or merely stores
// and version-declares (Advisory) a vendored schema.
type GradeStatus string

const (
	// Graded means sworn enforces this schema: either Validate()'s switch
	// (internal/baton/validator.go) has a case for it, or a
	// ValidateSchema(name, ...) call site grades it on the authoring path
	// (ADR-0011).
	Graded GradeStatus = "GRADED"
	// Advisory means the schema is vendored and version-declared but sworn
	// does not yet enforce it — a follow-on release owns the grader.
	Advisory GradeStatus = "ADVISORY"
)

// SchemaManifestEntry is one row of the graded-schema-version manifest: a
// vendored schema's short name, canonical $id, version, and grade status.
type SchemaManifestEntry struct {
	Name    string
	ID      string
	Version string
	// Status is "" when the schema is vendored but has no entry in
	// schemaGradeStatus — an unclassified/skewed schema. SchemaSkew()
	// reports this same condition as a dedicated skew line.
	Status GradeStatus
}

// schemaGradeStatus is the ONE hand-authored classification table in this
// package. Every other field a manifest entry carries (name, $id, version)
// is parsed straight out of the embedded schema bytes — never hand-listed
// here (S15 design decision D1; spec R-01's mitigation). Each entry below
// cites the call site that makes the classification true, so it stays
// auditable rather than asserted. SchemaSkew() checks this table's key set
// against the live schemas.SchemaMap key set, so a future vendoring bump
// that adds or removes a schema without updating this table WARNs instead
// of silently drifting.
var schemaGradeStatus = map[string]GradeStatus{
	// Graded: internal/baton/validator.go Validate()'s switch has a case
	// for each of these six schema names.
	"slice-status-v1": Graded,
	"board-v1":        Graded,
	"spec-v1":         Graded,
	"proof-v1":        Graded,
	"journeys-v1":     Graded,
	"attestations-v1": Graded,
	// Graded: the sole production ValidateSchema(...) call site —
	// internal/verify/verify.go's Rule-7 verifier verdict emission
	// (ADR-0011 keystone path).
	"verifier-verdict-v1": Graded,
	// Advisory: vendored at baton v0.10.0, stored + version-declared by
	// `sworn doctor`, not yet graded. The `sworn lint contracts` grader is
	// the follow-on contract-edge-gates release (see
	// internal/adopt/baton/VERSION's schemas-added line).
	"contracts-v1": Advisory,
	// Advisory: vendored at baton v0.10.0. The `sworn assemble` grader that
	// emits/reads this proof is the follow-on release.
	"assembly-proof-v1": Advisory,
}

// schemaVersionRE extracts a schema's trailing "-vN" version suffix from
// its short name (e.g. "spec-v1" -> "v1"). Every entry in schemas.SchemaMap
// follows this Baton naming convention.
var schemaVersionRE = regexp.MustCompile(`-(v[0-9]+)$`)

// schemaID parses the canonical $id straight out of a schema's embedded
// JSON bytes. Never hand-typed — the $id is the schema file's own
// declaration, so this can never drift from what the file actually says.
func schemaID(raw []byte) (string, error) {
	var doc struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("baton: parse schema $id: %w", err)
	}
	if doc.ID == "" {
		return "", fmt.Errorf("baton: schema has no $id")
	}
	return doc.ID, nil
}

// parseSchemaVersion derives the version (e.g. "v1") from a schema's short
// name (e.g. "spec-v1"), which by Baton convention always carries its
// version as a trailing "-vN" suffix.
func parseSchemaVersion(name string) (string, error) {
	m := schemaVersionRE.FindStringSubmatch(name)
	if m == nil {
		return "", fmt.Errorf("baton: schema name %q has no -vN version suffix", name)
	}
	return m[1], nil
}

// SchemaManifest returns one entry per vendored schema (schemaMapFn's live
// source — schemas.SchemaMap in production, an injected fixture in tests),
// sorted by name for deterministic rendering. Name/$id/version are always
// derived from the embedded bytes; Status comes from schemaGradeStatus and
// is "" (unclassified) when a vendored schema has no entry in the table —
// the condition SchemaSkew() also reports.
func SchemaManifest() ([]SchemaManifestEntry, error) {
	m := schemaMapFn()
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]SchemaManifestEntry, 0, len(names))
	for _, name := range names {
		id, err := schemaID(m[name])
		if err != nil {
			return nil, fmt.Errorf("baton: schema %q: %w", name, err)
		}
		version, err := parseSchemaVersion(name)
		if err != nil {
			return nil, fmt.Errorf("baton: schema %q: %w", name, err)
		}
		entries = append(entries, SchemaManifestEntry{
			Name:    name,
			ID:      id,
			Version: version,
			Status:  schemaGradeStatus[name], // "" if unclassified
		})
	}
	return entries, nil
}

// SchemaSkew compares schemaGradeStatus's key set against the live vendored
// schema set (schemaMapFn) and returns one line per disagreement: a
// vendored schema with no classification, or a classified name no longer
// vendored. An empty (nil) slice means the declared graded/advisory set
// matches the vendored set exactly — no skew.
func SchemaSkew() []string {
	vendored := schemaMapFn()
	var lines []string

	var unclassified []string
	for name := range vendored {
		if _, ok := schemaGradeStatus[name]; !ok {
			unclassified = append(unclassified, name)
		}
	}
	sort.Strings(unclassified)
	for _, name := range unclassified {
		lines = append(lines, fmt.Sprintf("vendored schema %q has no graded/advisory classification in schemaGradeStatus", name))
	}

	var stale []string
	for name := range schemaGradeStatus {
		if _, ok := vendored[name]; !ok {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		lines = append(lines, fmt.Sprintf("classified schema %q is no longer vendored (schemas.SchemaMap)", name))
	}

	return lines
}
