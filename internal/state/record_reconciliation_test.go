package state

// D6 record reconciliation (S01-d6-record-reconciliation). These tests pin the
// object-form carriers that a live consumer dogfood run crashed on: open_deferrals
// and verification.violations as arrays of OBJECTS, not []string. They cover
// AC-01 (read object form), AC-02 (loss-free byte-stable round trip), AC-03
// (typed structs preserving unknown keys), AC-07 (inconclusive result enum), and
// AC-10 (canonical strict-additive deferral round-trips through Write; a deferral
// missing acknowledgement or acknowledged_by fails closed against the schema).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// consumerShapedStatus carries a consumer's real PRE-MIGRATION object shapes verbatim:
// open_deferrals items as {id, description, why, tracking, acknowledged_by} (note:
// no top-level "item" and no acknowledgement — real coach data carries who
// (acknowledged_by) but not yet the canonical plain-text acknowledgement, which
// the AC-11 cutover migration adds) and verification.violations as {gate,
// description, evidence}. state.Read must tolerate this real data (Read unmarshals,
// it does not schema-validate); it is the exact shape that produced "cannot
// unmarshal object into Go struct field ... of type string" on the old []string
// carriers. The strict canonical schema is exercised separately (AC-10[B]).
const consumerShapedStatus = `{
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

// AC-01: state.Read unmarshals a consumer's real object shape without error.
func TestRead_ObjectFormDeferralsAndViolations_NoUnmarshalError(t *testing.T) {
	p := writeTemp(t, "status.json", consumerShapedStatus)
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
	p := writeTemp(t, "status.json", consumerShapedStatus)
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
	// acknowledged_by is now a named canonical field (AC-10), not an unknown key.
	if d.AcknowledgedBy != "Brad (Coach)" {
		t.Errorf("AC-03/AC-10: acknowledged_by not parsed into the named field, got %q", d.AcknowledgedBy)
	}
	// id/description are not named schema keys → preserved in Extra (no loss).
	for _, k := range []string{"id", "description"} {
		if _, ok := d.Extra[k]; !ok {
			t.Errorf("AC-03: unknown key %q dropped from Deferral.Extra", k)
		}
	}
	// acknowledged_by must NOT also linger in Extra (named field owns it now).
	if _, ok := d.Extra["acknowledged_by"]; ok {
		t.Error("AC-10: acknowledged_by must move to the named field, not stay in Extra")
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
	src := writeTemp(t, "status.json", consumerShapedStatus)
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

// AC-10 [A] (canonical round-trip through Write): a Status whose deferral carries
// the FULL canonical strict-additive shape (why + tracking + acknowledgement +
// acknowledged_by, plus optional acknowledged_at) must Write without error,
// validate against the schema, and read back with every canonical field populated
// in its named struct field (not lost to Extra). This is the post-migration shape
// the consumer loop runs on once the AC-11 cutover has added acknowledgement.
func TestWrite_CanonicalDeferral_RoundTrips(t *testing.T) {
	st := &Status{
		SliceID: "S01-x",
		Release: "2026-06-28-x",
		State:   InProgress,
		OpenDeferrals: []Deferral{{
			Item:            "year-snapshot backfill postponed",
			Why:             "depends on the schema migration landing first",
			Tracking:        "#123",
			Acknowledgement: "Coach told in plain text: defer to a later release",
			AcknowledgedBy:  "Brad (Coach)",
			AcknowledgedAt:  "2026-07-01T02:00:00Z",
		}},
		Verification: Verification{Result: "pending"},
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := Write(out, st); err != nil {
		t.Fatalf("AC-10[A]: write-back of canonical deferral must succeed, got: %v", err)
	}
	// The written bytes must validate against the strict canonical schema.
	b, _ := os.ReadFile(out)
	if err := baton.ValidateSchema("slice-status-v1", b); err != nil {
		t.Fatalf("AC-10[A]: canonical deferral must validate against slice-status-v1, got: %v", err)
	}
	// Read back: every canonical field populated in its named field.
	got, err := Read(out)
	if err != nil {
		t.Fatalf("AC-10[A]: read-back: %v", err)
	}
	d := got.OpenDeferrals[0]
	if d.Why == "" || d.Tracking == "" || d.Acknowledgement == "" || d.AcknowledgedBy == "" || d.AcknowledgedAt == "" {
		t.Errorf("AC-10[A]: canonical field lost on round-trip: %+v", d)
	}
}

// AC-10 [B] (schema fail-closed, strict additive): the open_deferrals required-set
// is the strict additive form [why, tracking, acknowledgement, acknowledged_by]
// (acknowledged_at optional) — NOT an anyOf either-or. The full canonical shape
// passes; a deferral missing ANY required key (acknowledgement OR acknowledged_by
// OR tracking OR why) fails closed. acknowledged_by alone no longer satisfies the
// set: a name is not Rule 2's plain-text "told" evidence.
func TestSchema_OpenDeferralStrictAdditive(t *testing.T) {
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
		{"full canonical passes", `{"why":"w","tracking":"#1","acknowledgement":"told in plain text","acknowledged_by":"Brad"}`, false},
		{"canonical with acknowledged_at passes", `{"why":"w","tracking":"#1","acknowledgement":"told","acknowledged_by":"Brad","acknowledged_at":"2026-07-01T02:00:00Z"}`, false},
		{"acknowledged_by alone fails closed (no acknowledgement)", `{"why":"w","tracking":"#1","acknowledged_by":"Brad"}`, true},
		{"acknowledgement alone fails closed (no acknowledged_by)", `{"why":"w","tracking":"#1","acknowledgement":"told"}`, true},
		{"missing tracking fails closed", `{"why":"w","acknowledgement":"told","acknowledged_by":"Brad"}`, true},
		{"missing why fails closed", `{"tracking":"#1","acknowledgement":"told","acknowledged_by":"Brad"}`, true},
		{"neither ack key fails closed", `{"why":"w","tracking":"#1"}`, true},
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
