package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestQuadrant pins the effort×complexity → quadrant mapping (ADR-0011 §3.7) and
// the "" sentinel for off-enum axes.
func TestQuadrant(t *testing.T) {
	cases := []struct{ effort, complexity, want string }{
		{"low", "low", "chore"},
		{"high", "low", "grind"},
		{"low", "high", "puzzle"},
		{"high", "high", "epic"},
		{"low", "", ""},
		{"medium", "low", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := Quadrant(c.effort, c.complexity); got != c.want {
			t.Errorf("Quadrant(%q,%q)=%q want %q", c.effort, c.complexity, got, c.want)
		}
	}
}

// TestEffortComplexityValidate proves the checksum accepts consistent ratings and
// rejects both an off-enum axis and a quadrant that contradicts the axes.
func TestEffortComplexityValidate(t *testing.T) {
	good := []EffortComplexity{
		{Effort: "low", Complexity: "low", Quadrant: "chore"},
		{Effort: "high", Complexity: "low", Quadrant: "grind"},
		{Effort: "low", Complexity: "high", Quadrant: "puzzle"},
		{Effort: "high", Complexity: "high", Quadrant: "epic"},
	}
	for _, ec := range good {
		if err := ec.Validate(); err != nil {
			t.Errorf("Validate(%+v) unexpected error: %v", ec, err)
		}
	}

	if err := (EffortComplexity{Effort: "medium", Complexity: "low", Quadrant: "chore"}).Validate(); err == nil {
		t.Error("off-enum effort axis accepted")
	}
	// low effort + high complexity is "puzzle"; "grind" is the checksum trap.
	if err := (EffortComplexity{Effort: "low", Complexity: "high", Quadrant: "grind"}).Validate(); err == nil {
		t.Error("inconsistent quadrant accepted — checksum not enforced")
	}
}

// TestStatusEffortComplexityRoundTrip proves the field parses off status.json and
// that Read/Write fail closed on an inconsistent rating (the integration point).
func TestStatusEffortComplexityRoundTrip(t *testing.T) {
	raw := `{"slice_id":"S01","release":"r1","state":"planned",` +
		`"verification":{"result":"pending"},` +
		`"effort_complexity":{"effort":"high","complexity":"high","quadrant":"epic",` +
		`"rationale":"novel concurrency + migration","confirmed_by_implementer":true}}`
	var s Status
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.EffortComplexity == nil {
		t.Fatal("effort_complexity not parsed")
	}
	if s.EffortComplexity.Quadrant != "epic" || !s.EffortComplexity.ConfirmedByImplementer {
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
	if back.EffortComplexity == nil || back.EffortComplexity.Quadrant != "epic" {
		t.Errorf("Read lost effort_complexity: %+v", back.EffortComplexity)
	}

	// Write fails closed on an inconsistent rating.
	bad := s
	bad.EffortComplexity = &EffortComplexity{Effort: "low", Complexity: "high", Quadrant: "chore"}
	if err := Write(filepath.Join(dir, "bad.json"), &bad); err == nil {
		t.Error("Write accepted an inconsistent effort_complexity rating")
	}

	// Read fails closed on a file with an inconsistent rating written out-of-band.
	badRaw := `{"slice_id":"S01","release":"r1","state":"planned",` +
		`"verification":{"result":"pending"},` +
		`"effort_complexity":{"effort":"low","complexity":"low","quadrant":"epic"}}`
	badPath := filepath.Join(dir, "badread.json")
	if err := os.WriteFile(badPath, []byte(badRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(badPath); err == nil {
		t.Error("Read accepted an inconsistent effort_complexity rating")
	}
}
