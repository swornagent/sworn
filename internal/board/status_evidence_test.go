package board

import "testing"

func ev(t *testing.T, state, verification, stamp, source, durability string) evidence {
	t.Helper()
	raw := `{"slice_id":"S","release":"R","track":"T","state":"` + state + `","last_updated_at":"` + stamp + `","verification":{"result":"` + verification + `"}}`
	e, ok := validEvidence(raw, source, durability, "R", "S", "T")
	if !ok {
		t.Fatalf("invalid %s", raw)
	}
	return e
}

func TestElectStateEvidenceLifecycleAndAttention(t *testing.T) {
	implemented := ev(t, "implemented", "pending", "2026-01-01T00:00:00Z", "refs/heads/a", "committed")
	verified := ev(t, "verified", "pending", "2026-01-01T00:00:00Z", "refs/heads/b", "committed")
	if !better(verified, implemented) {
		t.Fatal("verified must beat implemented")
	}
	blocked := ev(t, "implemented", "blocked", "2026-01-02T00:00:00Z", "refs/heads/c", "committed")
	if !better(blocked, verified) {
		t.Fatal("later attention must beat verified")
	}
}

func TestElectStateEvidenceCommittedTieBeatsWorkingTree(t *testing.T) {
	a := ev(t, "verified", "pending", "2026-01-01T00:00:00Z", "refs/heads/a", "committed")
	b := ev(t, "verified", "pending", "2026-01-01T00:00:00Z", "working-tree", "uncommitted")
	if !better(a, b) {
		t.Fatal("committed must win exact tie")
	}
}
