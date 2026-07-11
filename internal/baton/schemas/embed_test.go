package schemas

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAdvisorySchemasByteMatchPublished is the AC-10/AC-11 guard: the two NEW
// v0.10.0 advisory schemas embedded in the binary (contracts-v1, assembly-proof-v1)
// are BYTE-IDENTICAL to the published schema files at the pinned baton v0.10.0 tag
// (captured verbatim under testdata/published-v0.10.0). This fails closed if the
// embed is ever edited out-of-band — a fork of the shape under the same $id, the
// baton#55 divergence class the vendor discipline forbids.
func TestAdvisorySchemasByteMatchPublished(t *testing.T) {
	cases := []struct {
		name     string
		embedded []byte
		id       string
	}{
		{"contracts-v1", ContractsV1, "https://baton.sawy3r.net/schemas/contracts-v1.json"},
		{"assembly-proof-v1", AssemblyProofV1, "https://baton.sawy3r.net/schemas/assembly-proof-v1.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			published, err := os.ReadFile(filepath.Join("testdata", "published-v0.10.0", c.name+".json"))
			if err != nil {
				t.Fatalf("read published golden: %v", err)
			}
			if !bytes.Equal(c.embedded, published) {
				t.Errorf("%s embed is not byte-identical to the published v0.10.0 schema (out-of-band edit / fork under the same $id)", c.name)
			}
			// The embedded schema declares the canonical published $id — not a
			// sworn-local variant URI.
			var doc struct {
				ID string `json:"$id"`
			}
			if err := json.Unmarshal(c.embedded, &doc); err != nil {
				t.Fatalf("parse embedded %s: %v", c.name, err)
			}
			if doc.ID != c.id {
				t.Errorf("%s $id = %q, want %q", c.name, doc.ID, c.id)
			}
			// It is registered in SchemaMap (doctor-declarable / advisory-vendored).
			if _, ok := SchemaMap[c.name]; !ok {
				t.Errorf("%s not registered in SchemaMap", c.name)
			}
		})
	}
}
