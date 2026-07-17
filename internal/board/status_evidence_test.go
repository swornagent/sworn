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
	planned := ev(t, "planned", "pending", "2026-01-01T00:00:00Z", "refs/heads/a", "committed")
	designReview := ev(t, "design_review", "pending", "2026-01-01T00:00:00Z", "refs/heads/b", "committed")
	inProgress := ev(t, "in_progress", "pending", "2026-01-01T00:00:00Z", "refs/heads/c", "committed")
	implemented := ev(t, "implemented", "pending", "2026-01-01T00:00:00Z", "refs/heads/a", "committed")
	verified := ev(t, "verified", "pending", "2026-01-01T00:00:00Z", "refs/heads/b", "committed")
	shipped := ev(t, "shipped", "pending", "2026-01-01T00:00:00Z", "refs/heads/c", "committed")
	if !better(designReview, planned) || !better(inProgress, designReview) || !better(implemented, inProgress) || !better(shipped, verified) {
		t.Fatal("normal lifecycle rank order regressed")
	}
	if !better(verified, implemented) {
		t.Fatal("verified must beat implemented")
	}
	blocked := ev(t, "implemented", "blocked", "2026-01-02T00:00:00Z", "refs/heads/c", "committed")
	if !better(blocked, verified) {
		t.Fatal("later attention must beat verified")
	}
	earlierBlocked := ev(t, "implemented", "blocked", "2025-12-31T23:59:59Z", "refs/heads/d", "committed")
	if better(earlierBlocked, verified) {
		t.Fatal("earlier attention must not beat later normal evidence")
	}
	missingTimestampBlocked := ev(t, "implemented", "blocked", "", "refs/heads/e", "committed")
	if !better(missingTimestampBlocked, verified) {
		t.Fatal("attention must win a missing-timestamp safety tie")
	}
}

func TestElectStateEvidenceCommittedTieBeatsWorkingTree(t *testing.T) {
	a := ev(t, "verified", "pending", "2026-01-01T00:00:00Z", "refs/heads/a", "committed")
	b := ev(t, "verified", "pending", "2026-01-01T00:00:00Z", "working-tree", "uncommitted")
	if !better(a, b) {
		t.Fatal("committed must win exact tie")
	}
}

func TestValidEvidenceRejectsMalformedUnknownAndMismatchedCandidates(t *testing.T) {
	cases := []string{
		`{`,
		`{"slice_id":"S","release":"R","track":"T","state":"unknown","verification":{"result":"pending"}}`,
		`{"slice_id":"other","release":"R","track":"T","state":"verified","verification":{"result":"pending"}}`,
		`{"slice_id":"S","release":"other","track":"T","state":"verified","verification":{"result":"pending"}}`,
		`{"slice_id":"S","release":"R","track":"other","state":"verified","verification":{"result":"pending"}}`,
	}
	for _, raw := range cases {
		if _, ok := validEvidence(raw, "refs/heads/candidate", "committed", "R", "S", "T"); ok {
			t.Fatalf("invalid candidate admitted: %s", raw)
		}
	}
}
