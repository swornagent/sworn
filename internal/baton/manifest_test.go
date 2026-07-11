package baton

import (
	"testing"

	"github.com/swornagent/sworn/internal/baton/schemas"
)

// TestSchemaManifestCoversAllVendoredSchemas is leaf-level coverage of
// SchemaManifest() against the real, unmodified schemas.SchemaMap — proof
// that the manifest's name/$id/version derivation and graded/advisory
// classification agree with live vendored ground truth. The CLI-affordance
// reachability proof (Rule 1) lives in cmd/sworn/doctor_test.go, driven
// through cmdDoctor, not here.
func TestSchemaManifestCoversAllVendoredSchemas(t *testing.T) {
	entries, err := SchemaManifest()
	if err != nil {
		t.Fatalf("SchemaManifest() error: %v", err)
	}

	if len(entries) != len(schemas.SchemaMap) {
		t.Fatalf("SchemaManifest() returned %d entries, want %d (len(schemas.SchemaMap))", len(entries), len(schemas.SchemaMap))
	}

	byName := make(map[string]SchemaManifestEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	graded := []string{
		"slice-status-v1", "board-v1", "spec-v1", "proof-v1",
		"journeys-v1", "attestations-v1", "verifier-verdict-v1",
	}
	for _, name := range graded {
		e, ok := byName[name]
		if !ok {
			t.Errorf("missing manifest entry for %q", name)
			continue
		}
		if e.Status != Graded {
			t.Errorf("%q: Status = %q, want %q", name, e.Status, Graded)
		}
		wantID := "https://baton.sawy3r.net/schemas/" + name + ".json"
		if e.ID != wantID {
			t.Errorf("%q: ID = %q, want %q", name, e.ID, wantID)
		}
		if e.Version != "v1" {
			t.Errorf("%q: Version = %q, want %q", name, e.Version, "v1")
		}
	}

	for _, name := range []string{"contracts-v1", "assembly-proof-v1"} {
		e, ok := byName[name]
		if !ok {
			t.Errorf("missing manifest entry for %q", name)
			continue
		}
		if e.Status != Advisory {
			t.Errorf("%q: Status = %q, want %q", name, e.Status, Advisory)
		}
	}
}

// TestSchemaSkewCleanOnRealSchemaMap asserts the real, unmodified
// classification table matches the real, unmodified vendored schema set —
// no skew — which is the state the release must always ship in.
func TestSchemaSkewCleanOnRealSchemaMap(t *testing.T) {
	if skew := SchemaSkew(); len(skew) != 0 {
		t.Errorf("SchemaSkew() on the real schema set = %v, want empty (no skew)", skew)
	}
}

// TestSchemaSkewFiresOnExtraUnclassifiedSchema injects a fixture schema map
// with one extra name absent from schemaGradeStatus and asserts SchemaSkew
// reports it, and that the corresponding SchemaManifest entry renders with
// an empty (unclassified) Status.
func TestSchemaSkewFiresOnExtraUnclassifiedSchema(t *testing.T) {
	fixture := make(map[string][]byte, len(schemas.SchemaMap)+1)
	for k, v := range schemas.SchemaMap {
		fixture[k] = v
	}
	fixture["made-up-v1"] = []byte(`{"$id":"https://baton.sawy3r.net/schemas/made-up-v1.json","type":"object"}`)

	SetSchemaMapForTest(fixture)
	defer ClearSchemaMapForTest()

	skew := SchemaSkew()
	if len(skew) != 1 {
		t.Fatalf("SchemaSkew() = %v, want exactly 1 line for the injected made-up-v1 schema", skew)
	}

	entries, err := SchemaManifest()
	if err != nil {
		t.Fatalf("SchemaManifest() error: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Name == "made-up-v1" {
			found = true
			if e.Status != "" {
				t.Errorf("made-up-v1: Status = %q, want empty (unclassified)", e.Status)
			}
		}
	}
	if !found {
		t.Fatalf("SchemaManifest() did not include the injected made-up-v1 fixture entry")
	}
}

// TestSchemaSkewFiresOnMissingVendoredSchema injects a fixture schema map
// with a classified name (contracts-v1) removed and asserts SchemaSkew
// reports the classification as stale.
func TestSchemaSkewFiresOnMissingVendoredSchema(t *testing.T) {
	fixture := make(map[string][]byte, len(schemas.SchemaMap)-1)
	for k, v := range schemas.SchemaMap {
		if k == "contracts-v1" {
			continue
		}
		fixture[k] = v
	}

	SetSchemaMapForTest(fixture)
	defer ClearSchemaMapForTest()

	skew := SchemaSkew()
	if len(skew) != 1 {
		t.Fatalf("SchemaSkew() = %v, want exactly 1 line for the removed contracts-v1 classification", skew)
	}
}
