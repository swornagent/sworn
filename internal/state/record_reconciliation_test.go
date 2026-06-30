package state

// D6 record reconciliation (S01-d6-record-reconciliation). These tests pin the
// object-form carriers that the live fired dogfood run crashed on: open_deferrals
// and verification.violations as arrays of OBJECTS, not []string. They cover
// AC-01 (read object form), AC-02 (loss-free byte-stable round trip), AC-03
// (typed structs preserving unknown keys), AC-07 (inconclusive result enum), and
// AC-10 (write-back of real acknowledged_by-only deferrals + schema fail-closed).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// firedShapedStatus carries fired's real object shapes verbatim: open_deferrals
// items as {id, description, why, tracking, acknowledged_by} (note: no top-level
// "item" and no schema-required "acknowledgement" — acknowledged_by stands in for
// it) and verification.violations as {gate, description, evidence}. This is the
// exact shape that produced "cannot unmarshal object into Go struct field ... of
// type string" on the old []string carriers.
const firedShapedStatus = `{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S01-networth-hierarchy-remap",
  "release": "2026-06-28-yearSnapshot-schema-cleanup",
  "state": "in_progress",
  "open_deferrals": [
    {
      "id": "D-001",
      "description": "year-snapshot backfill postponed to a later release",
      "why": "depends on the schema migration landing first",
      "tracking": "#123",
      "acknowledged_by": "Brad (Coach)"
    }
  ],
  "verification": {
    "result": "blocked",
    "violations": [
      { "gate": "reachability", "description": "no e2e proof", "evidence": "proof.md absent" }
    ]
  }
}`

// writeTemp writes content to a fresh temp file and returns its path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

// AC-01: state.Read unmarshals fired's real object shape without error.
func TestRead_ObjectFormDeferralsAndViolations_NoUnmarshalError(t *testing.T) {
	p := writeTemp(t, "status.json", firedShapedStatus)
	st, err := Read(p)
	if err != nil {
		t.Fatalf("AC-01: Read of object-form status must not error, got: %v", err)
	}
	if len(st.OpenDeferrals) != 1 {
		t.Fatalf("AC-01: want 1 open_deferral, got %d", len(st.OpenDeferrals))
	}
	if len(st.Verification.Violations) != 1 {
		t.Fatalf("AC-01: want 1 violation, got %d", len(st.Verification.Violations))
	}
}

// AC-03: the carriers are typed structs whose named fields are populated and
// whose unknown keys (id, description, acknowledged_by) survive in Extra.
func TestRead_TypedStructsPreserveUnknownKeys(t *testing.T) {
	p := writeTemp(t, "status.json", firedShapedStatus)
	st, err := Read(p)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	d := st.OpenDeferrals[0]
	if d.Why != "depends on the schema migration landing first" {
		t.Errorf("AC-03: Why not parsed into the named field, got %q", d.Why)
	}
	if d.Tracking != "#123" {
		t.Errorf("AC-03: Tracking not parsed, got %q", d.Tracking)
	}
	// id/description/acknowledged_by are not named schema keys → preserved in Extra.
	for _, k := range []string{"id", "description", "acknowledged_by"} {
		if _, ok := d.Extra[k]; !ok {
			t.Errorf("AC-03: unknown key %q dropped from Deferral.Extra", k)
		}
	}
	v := st.Verification.Violations[0]
	if v.Gate != "reachability" || v.Description != "no e2e proof" || v.Evidence != "proof.md absent" {
		t.Errorf("AC-03: violation named fields not parsed: %+v", v)
	}
}

// AC-02: a read→write→read→write cycle preserves every original field (the
// unknown keys included) and is byte-stable on the write side — repeated writes
// produce identical bytes, so a no-op transition never churns the drift gate.
func TestRoundTrip_PreservesFieldsAndIsByteStable(t *testing.T) {
	src := writeTemp(t, "status.json", firedShapedStatus)
	st1, err := Read(src)
	if err != nil {
		t.Fatalf("Read 1: %v", err)
	}

	out1 := filepath.Join(t.TempDir(), "out1.json")
	if err := Write(out1, st1); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	b1, _ := os.ReadFile(out1)

	st2, err := Read(out1)
	if err != nil {
		t.Fatalf("Read 2: %v", err)
	}
	out2 := filepath.Join(t.TempDir(), "out2.json")
	if err := Write(out2, st2); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	b2, _ := os.ReadFile(out2)

	if !bytes.Equal(b1, b2) {
		t.Errorf("AC-02: write output not byte-stable across a round trip\n--- b1 ---\n%s\n--- b2 ---\n%s", b1, b2)
	}
	// Every original field must survive on the written bytes (no field dropped).
	for _, key := range []string{"\"id\"", "\"description\"", "\"acknowledged_by\"", "\"why\"", "\"tracking\"", "\"gate\"", "\"evidence\""} {
		if !strings.Contains(string(b1), key) {
			t.Errorf("AC-02: field %s dropped on write-back; output:\n%s", key, b1)
		}
	}
}

// AC-10 [A] (write path): state.Write of a Status whose deferral carries the real
// coach acknowledged_by but no schema-required acknowledgement must NOT be
// rejected — the live fired run round-trips a real coach status without dying on
// write-back.
func TestWrite_AcknowledgedByOnlyDeferral_NoError(t *testing.T) {
	src := writeTemp(t, "status.json", firedShapedStatus)
	st, err := Read(src)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, ok := st.OpenDeferrals[0].Extra["acknowledged_by"]; !ok {
		t.Fatal("fixture sanity: deferral should carry acknowledged_by")
	}
	if st.OpenDeferrals[0].Acknowledgement != "" {
		t.Fatal("fixture sanity: deferral should NOT carry acknowledgement")
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := Write(out, st); err != nil {
		t.Fatalf("AC-10[A]: write-back of acknowledged_by-only deferral must succeed, got: %v", err)
	}
}

// AC-10 [B] (schema fail-closed): the relaxed open_deferrals required-set is an
// anyOf over {why,tracking,acknowledgement} | {why,tracking,acknowledged_by}. A
// deferral with acknowledged_by passes; a deferral with neither ack key still
// fails closed, preserving Rule 2's must-be-acknowledged intent.
func TestSchema_OpenDeferralAcknowledgementAnyOf(t *testing.T) {
	base := func(deferral string) []byte {
		return []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S01-x",
  "release": "2026-06-28-x",
  "state": "in_progress",
  "open_deferrals": [` + deferral + `],
  "verification": { "result": "pending" }
}`)
	}
	cases := []struct {
		name     string
		deferral string
		wantErr  bool
	}{
		{"acknowledgement satisfies", `{"why":"w","tracking":"#1","acknowledgement":"Brad"}`, false},
		{"acknowledged_by satisfies", `{"why":"w","tracking":"#1","acknowledged_by":"Brad"}`, false},
		{"neither ack key fails closed", `{"why":"w","tracking":"#1"}`, true},
		{"missing tracking fails closed", `{"why":"w","acknowledged_by":"Brad"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := baton.ValidateSchema("slice-status-v1", base(tc.deferral))
			if tc.wantErr && err == nil {
				t.Errorf("AC-10[B]: expected schema validation error for %s, got nil", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("AC-10[B]: expected schema to accept %s, got: %v", tc.name, err)
			}
		})
	}
}

// AC-07: the verification.result enum includes "inconclusive" so an inconclusive
// verdict is representable and validates. The merge gate is unchanged — it keys
// on slice STATE, not result, so inconclusive is merely representable, never a pass.
func TestSchema_InconclusiveResultValidates(t *testing.T) {
	data := []byte(`{
  "$schema": "https://baton.sawy3r.net/schemas/slice-status-v1.json",
  "slice_id": "S01-x",
  "release": "2026-06-28-x",
  "state": "implemented",
  "verification": { "result": "inconclusive" }
}`)
	if err := baton.ValidateSchema("slice-status-v1", data); err != nil {
		t.Errorf("AC-07: result=inconclusive must validate against slice-status-v1, got: %v", err)
	}
}
