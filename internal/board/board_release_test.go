package board

import (
	"encoding/json"
	"strings"
	"testing"
)

// S04-board-record-reconciliation: BoardRecord.Release reads both the canonical
// baton object form ({name, vertical_trace, ...}) and the legacy string form,
// and round-trips the object verbatim (no field dropped).

func TestRelease_ObjectForm(t *testing.T) {
	// Canonical coach-produced board (fired's shape): release is an object.
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

func TestRelease_StringForm(t *testing.T) {
	// Legacy board: release is a bare string. Must still parse (AC-02).
	raw := []byte(`{"schema_version":1,"release":"legacy-release","tracks":[]}`)
	var br BoardRecord
	if err := json.Unmarshal(raw, &br); err != nil {
		t.Fatalf("unmarshal legacy string board: %v", err)
	}
	if br.Release.Name != "legacy-release" {
		t.Errorf("release name = %q, want legacy-release", br.Release.Name)
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
	// AC-07: a write-back must not drop vertical_trace / target_version.
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

func TestRelease_StringRoundTripsAsString(t *testing.T) {
	var rel Release
	if err := json.Unmarshal([]byte(`"legacy"`), &rel); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	out, _ := json.Marshal(rel)
	if string(out) != `"legacy"` {
		t.Errorf("string release round-trip = %s, want \"legacy\"", out)
	}
}
