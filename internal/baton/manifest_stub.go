package baton

import "github.com/swornagent/sworn/internal/baton/schemas"

// schemaMapFn is the injectable source SchemaManifest and SchemaSkew read
// from. Production code never bypasses it — this is the seam a test uses
// to inject a skew fixture without touching the real embedded schema
// files, mirroring the versionForTest / SetVersionForTest pattern in
// version.go / version_stub.go.
var schemaMapFn = func() map[string][]byte { return schemas.SchemaMap }

// SetSchemaMapForTest overrides the vendored schema set SchemaManifest and
// SchemaSkew read from. Used to prove doctor's skew check fires on a
// deliberately-skewed fixture (S15 AC-02) without mutating real embedded
// schemas. Callers must defer ClearSchemaMapForTest.
func SetSchemaMapForTest(m map[string][]byte) {
	schemaMapFn = func() map[string][]byte { return m }
}

// ClearSchemaMapForTest restores the default (embedded) schema set.
func ClearSchemaMapForTest() {
	schemaMapFn = func() map[string][]byte { return schemas.SchemaMap }
}
