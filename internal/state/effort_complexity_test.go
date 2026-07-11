package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestQuadrant pins the effort×complexity → quadrant mapping (ADR-0011 §3.7) at
// baton v0.10.0 (low/low=quick, high/high=beast; grind/puzzle unchanged) and the
// "" sentinel for off-enum axes.
func TestQuadrant(t *testing.T) {
	cases := []struct{ effort, complexity, want string }{
		{"low", "low", "quick"},
		{"high", "low", "grind"},
		{"low", "high", "puzzle"},
		{"high", "high", "beast"},
		{"low", "", ""},
		{"medium", "low", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := Quadrant(c.effort, c.complexity); got != c.want {
			t.Errorf("Quadrant(%q,%q)=%q want %q", c.effort, c.complexity, got, c.want)
		}
	}
	// Quadrant never emits the retired chore/epic names.
	for _, c := range cases {
		if got := Quadrant(c.effort, c.complexity); got == "chore" || got == "epic" {
			t.Errorf("Quadrant(%q,%q)=%q emitted a retired name", c.effort, c.complexity, got)
		}
	}
}

// TestEffortComplexityValidate proves the checksum accepts consistent canonical
// ratings and rejects both an off-enum axis and a quadrant that contradicts the
// axes — INCLUDING the retired chore/epic names, which Validate rejects
// (Validate is strictly quick/grind/puzzle/beast; the S11 read-path normalise
// shim that once mapped chore->quick / epic->beast was removed by S12, so there
// is no tolerance layer any more).
func TestEffortComplexityValidate(t *testing.T) {
	good := []EffortComplexity{
		{Effort: "low", Complexity: "low", Quadrant: "quick"},
		{Effort: "high", Complexity: "low", Quadrant: "grind"},
		{Effort: "low", Complexity: "high", Quadrant: "puzzle"},
		{Effort: "high", Complexity: "high", Quadrant: "beast"},
	}
	for _, ec := range good {
		if err := ec.Validate(); err != nil {
			t.Errorf("Validate(%+v) unexpected error: %v", ec, err)
		}
	}

	if err := (EffortComplexity{Effort: "medium", Complexity: "low", Quadrant: "quick"}).Validate(); err == nil {
		t.Error("off-enum effort axis accepted")
	}
	// low effort + high complexity is "puzzle"; "grind" is the checksum trap.
	if err := (EffortComplexity{Effort: "low", Complexity: "high", Quadrant: "grind"}).Validate(); err == nil {
		t.Error("inconsistent quadrant accepted — checksum not enforced")
	}
	// The retired names are rejected by Validate itself (strictly strict) — with
	// the S11 read-path shim gone (S12), nothing maps them, so they never load.
	if err := (EffortComplexity{Effort: "low", Complexity: "low", Quadrant: "chore"}).Validate(); err == nil {
		t.Error("retired 'chore' accepted by Validate — Validate must stay strict")
	}
	if err := (EffortComplexity{Effort: "high", Complexity: "high", Quadrant: "epic"}).Validate(); err == nil {
		t.Error("retired 'epic' accepted by Validate — Validate must stay strict")
	}
}

// TestStatusEffortComplexityRoundTrip proves the field parses off status.json and
// that Read/Write fail closed on an inconsistent rating (the integration point).
func TestStatusEffortComplexityRoundTrip(t *testing.T) {
	raw := `{"slice_id":"S01","release":"r1","state":"planned",` +
		`"verification":{"result":"pending"},` +
		`"effort_complexity":{"effort":"high","complexity":"high","quadrant":"beast",` +
		`"rationale":"novel concurrency + migration","confirmed_by_implementer":true}}`
	var s Status
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.EffortComplexity == nil {
		t.Fatal("effort_complexity not parsed")
	}
	if s.EffortComplexity.Quadrant != "beast" || !s.EffortComplexity.ConfirmedByImplementer {
		t.Errorf("round-trip lost fields: %+v", s.EffortComplexity)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")

	// Write + Read a consistent rating round-trips cleanly.
	if err := Write(path, &s); err != nil {
		t.Fatalf("Write consistent rating: %v", err)
	}
	back, err := Read(path)
	if err != nil {
		t.Fatalf("Read consistent rating: %v", err)
	}
	if back.EffortComplexity == nil || back.EffortComplexity.Quadrant != "beast" {
		t.Errorf("Read lost effort_complexity: %+v", back.EffortComplexity)
	}

	// Write fails closed on an inconsistent rating.
	bad := s
	bad.EffortComplexity = &EffortComplexity{Effort: "low", Complexity: "high", Quadrant: "quick"}
	if err := Write(filepath.Join(dir, "bad.json"), &bad); err == nil {
		t.Error("Write accepted an inconsistent effort_complexity rating")
	}

	// Read fails closed on a file with an inconsistent rating written out-of-band.
	badRaw := `{"slice_id":"S01","release":"r1","state":"planned",` +
		`"verification":{"result":"pending"},` +
		`"effort_complexity":{"effort":"low","complexity":"low","quadrant":"beast"}}`
	badPath := filepath.Join(dir, "badread.json")
	if err := os.WriteFile(badPath, []byte(badRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(badPath); err == nil {
		t.Error("Read accepted an inconsistent effort_complexity rating")
	}
}

// TestReadRejectsRetiredQuadrant proves the S11 read-path normalise shim is gone
// (removed wholesale by S12-record-migration, sworn#90): a status.json still
// carrying the retired chore/epic quadrant name no longer loads — Read fails
// closed on the strict quadrant<->axes checksum instead of silently mapping the
// name to quick/beast. This is the AC-04 tolerance-removal proof at the
// integration point (state.Read), the inverse of the shim behaviour it replaces.
func TestReadRejectsRetiredQuadrant(t *testing.T) {
	dir := t.TempDir()

	// A legacy epic rating no longer loads — the shim that mapped epic->beast on
	// read is gone, so the strict checksum (want=beast, got=epic) fails closed.
	epicPath := filepath.Join(dir, "epic.json")
	epicRaw := `{"slice_id":"S01","release":"r1","state":"planned",` +
		`"verification":{"result":"pending"},` +
		`"effort_complexity":{"effort":"high","complexity":"high","quadrant":"epic"}}`
	if err := os.WriteFile(epicPath, []byte(epicRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(epicPath); err == nil {
		t.Error("Read accepted a retired 'epic' quadrant — the S12 tolerance removal must make it fail closed")
	}

	// A legacy chore rating likewise fails closed (was low/low/chore -> quick).
	chorePath := filepath.Join(dir, "chore.json")
	choreRaw := `{"slice_id":"S02","release":"r1","state":"planned",` +
		`"verification":{"result":"pending"},` +
		`"effort_complexity":{"effort":"low","complexity":"low","quadrant":"chore"}}`
	if err := os.WriteFile(chorePath, []byte(choreRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(chorePath); err == nil {
		t.Error("Read accepted a retired 'chore' quadrant — the S12 tolerance removal must make it fail closed")
	}

	// Control: the canonical replacement rating still loads cleanly.
	okPath := filepath.Join(dir, "ok.json")
	okRaw := `{"slice_id":"S03","release":"r1","state":"planned",` +
		`"verification":{"result":"pending"},` +
		`"effort_complexity":{"effort":"low","complexity":"low","quadrant":"quick"}}`
	if err := os.WriteFile(okPath, []byte(okRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(okPath); err != nil {
		t.Errorf("Read rejected a canonical quick rating: %v", err)
	}
}
