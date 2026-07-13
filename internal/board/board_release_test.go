package board

import (
	"encoding/json"
	"strings"
	"testing"
)

// S05-board-canonical-emit: BoardRecord.Release reads ONLY the canonical baton
// object form ({name, vertical_trace, ...}) — a bare string fails closed (strict
// reader) — and round-trips the object verbatim (no field dropped). S04 first
// read both forms; S05 tightened the reader to object-only (no-wild-data).

func TestRelease_ObjectForm(t *testing.T) {
	// Canonical coach-produced board (a consumer's shape): release is an object.
	raw := []byte(`{
	  "schema_version": 1,
	  "release": {
	    "name": "2026-06-28-yearSnapshot-schema-cleanup",
	    "target_version": "v0.5.0",
	    "vertical_trace": {"benefit": "self-describing contract"}
	  },
	  "tracks": []
	}`)
	var br BoardRecord
	if err := json.Unmarshal(raw, &br); err != nil {
		t.Fatalf("unmarshal canonical object board: %v", err) // AC-01
	}
	if br.Release.Name != "2026-06-28-yearSnapshot-schema-cleanup" {
		t.Errorf("release name = %q, want the object's name", br.Release.Name)
	}
}

func TestRelease_StringForm_FailsClosed(t *testing.T) {
	// S05 strict reader (AC-03): a legacy bare-string release no longer parses —
	// it fails closed so a non-migrated operator board surfaces loudly instead of
	// lurking. Operator string boards are migrated to the object form at cutover
	// (AC-06), never read-tolerated.
	raw := []byte(`{"schema_version":1,"release":"legacy-release","tracks":[]}`)
	var br BoardRecord
	if err := json.Unmarshal(raw, &br); err == nil {
		t.Fatal("want error for bare-string release under the strict reader, got nil")
	}
}

func TestRelease_ObjectMissingName_FailsClosed(t *testing.T) {
	// An object release with no name is a defect — fail closed (AC-03).
	raw := []byte(`{"schema_version":1,"release":{"target_version":"v1"},"tracks":[]}`)
	var br BoardRecord
	if err := json.Unmarshal(raw, &br); err == nil {
		t.Fatal("want error for release object missing name, got nil")
	}
}

func TestRelease_RoundTripPreservesObjectFields(t *testing.T) {
	// AC-01: a release read from a canonical object must re-emit its full object
	// verbatim — a write-back must not drop vertical_trace / target_version.
	raw := []byte(`{"name":"r1","target_version":"v0.5.0","vertical_trace":{"benefit":"b"}}`)
	var rel Release
	if err := json.Unmarshal(raw, &rel); err != nil {
		t.Fatalf("unmarshal release object: %v", err)
	}
	out, err := json.Marshal(rel)
	if err != nil {
		t.Fatalf("marshal release: %v", err)
	}
	s := string(out)
	for _, want := range []string{`"name":"r1"`, `"target_version":"v0.5.0"`, `"vertical_trace"`, `"benefit":"b"`} {
		if !strings.Contains(s, want) {
			t.Errorf("round-trip dropped %s; got %s", want, s)
		}
	}
}

func TestRelease_BareStringRead_FailsClosed(t *testing.T) {
	// S05 strict reader (AC-03): a bare JSON string release fails closed at the
	// Release reader. There is no wild data — every string board is operator-owned
	// — so a stray string is a non-migrated artefact that must error, not be
	// silently accepted.
	var rel Release
	if err := json.Unmarshal([]byte(`"legacy"`), &rel); err == nil {
		t.Fatal("want error reading a bare-string release under the strict reader, got nil")
	}
}

func TestStringRelease_EmitsCanonicalObject(t *testing.T) {
	// AC-01: a name-only Release constructed in-process (StringRelease — the
	// index.md migration path) emits the canonical object form, so sworn never
	// writes the legacy bare-string form even for a release it only knows by name.
	rel := StringRelease("legacy")
	out, err := json.Marshal(rel)
	if err != nil {
		t.Fatalf("marshal StringRelease: %v", err)
	}
	if string(out) != `{"name":"legacy"}` {
		t.Errorf("StringRelease emit = %s, want canonical object {\"name\":\"legacy\"}", out)
	}
}
