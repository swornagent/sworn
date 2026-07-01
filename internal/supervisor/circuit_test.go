package supervisor

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// setupCircuitDB creates an in-memory SQLite DB with the circuit_failures
// table and returns the handle.
func setupCircuitDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS circuit_failures (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slice_id    TEXT NOT NULL,
		release     TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		recorded_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create circuit_failures: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestShouldBreak_ThreeConsecutiveSameFingerprint(t *testing.T) {
	// AC2: WHEN ShouldBreak is called 3 consecutive times for the same
	// sliceID+fingerprint with no intervening non-matching fingerprint,
	// THE SYSTEM SHALL return true.
	db := setupCircuitDB(t)
	release := "test-release"
	sliceID := "S03-crash-recovery"
	fp := Fingerprint(sliceID, "agent: turn cap (25) reached")

	// Record 3 failures with the same fingerprint.
	for i := 0; i < 3; i++ {
		if err := RecordFailure(db, release, sliceID, fp); err != nil {
			t.Fatalf("RecordFailure %d: %v", i, err)
		}
	}

	if !ShouldBreak(db, release, sliceID, fp) {
		t.Error("ShouldBreak should return true after 3 consecutive same-fingerprint failures")
	}
}

func TestShouldBreak_LessThanThree(t *testing.T) {
	// AC5: < 3 calls → returns false.
	db := setupCircuitDB(t)
	release := "test-release"
	sliceID := "S03-crash-recovery"
	fp := Fingerprint(sliceID, "agent: turn cap (25) reached")

	// Record 2 failures.
	for i := 0; i < 2; i++ {
		if err := RecordFailure(db, release, sliceID, fp); err != nil {
			t.Fatalf("RecordFailure %d: %v", i, err)
		}
	}

	if ShouldBreak(db, release, sliceID, fp) {
		t.Error("ShouldBreak should return false with only 2 failures")
	}
}

func TestShouldBreak_InterleavedDifferentFingerprint(t *testing.T) {
	// AC5: interleaved different fingerprint resets the counter.
	db := setupCircuitDB(t)
	release := "test-release"
	sliceID := "S03-crash-recovery"
	fp1 := Fingerprint(sliceID, "agent: turn cap (25) reached")
	fp2 := Fingerprint(sliceID, "implement: agent loop: some other error")

	// Record fp1, fp1, fp2 (different), fp1, fp1 — never reaches 3 consecutive fp1.
	for _, fp := range []string{fp1, fp1, fp2, fp1, fp1} {
		if err := RecordFailure(db, release, sliceID, fp); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	if ShouldBreak(db, release, sliceID, fp1) {
		t.Error("ShouldBreak should return false — interleaved different fingerprint breaks the streak")
	}
}

func TestShouldBreak_ResetAfterDifferentFingerprint(t *testing.T) {
	// AC5: the counter resets after a different fingerprint; a new run of 3
	// of the same fingerprint after the reset should trip the breaker.
	db := setupCircuitDB(t)
	release := "test-release"
	sliceID := "S03-crash-recovery"
	fp1 := Fingerprint(sliceID, "agent: turn cap (25) reached")
	fp2 := Fingerprint(sliceID, "implement: agent loop: timeout")

	// Record fp1, fp1, fp2 (resets), then fp1, fp1, fp1 (should trip).
	for _, fp := range []string{fp1, fp1, fp2, fp1, fp1, fp1} {
		if err := RecordFailure(db, release, sliceID, fp); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	if !ShouldBreak(db, release, sliceID, fp1) {
		t.Error("ShouldBreak should return true — 3 consecutive fp1 after reset by fp2")
	}
}

func TestShouldBreak_NilDB(t *testing.T) {
	// AC4: IF the supervisor DB is unavailable, THE SYSTEM SHALL default to
	// ShouldBreak returning false (fail open on circuit breaker).
	if ShouldBreak(nil, "release", "slice", "abc123") {
		t.Error("ShouldBreak should return false when DB is nil (fail-open)")
	}
}

func TestShouldBreak_EmptyDB(t *testing.T) {
	// Fresh DB with no failures → ShouldBreak returns false.
	db := setupCircuitDB(t)
	if ShouldBreak(db, "release", "slice", "abc123") {
		t.Error("ShouldBreak should return false on empty DB")
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	// Same inputs should produce the same fingerprint.
	fp1 := Fingerprint("S03", "agent: turn cap (25) reached")
	fp2 := Fingerprint("S03", "agent: turn cap (25) reached")
	if fp1 != fp2 {
		t.Errorf("Fingerprint should be deterministic: %q != %q", fp1, fp2)
	}
}

func TestFingerprint_DifferentSlice(t *testing.T) {
	// Different slice ID should produce different fingerprint.
	fp1 := Fingerprint("S03", "agent: turn cap (25) reached")
	fp2 := Fingerprint("S04", "agent: turn cap (25) reached")
	if fp1 == fp2 {
		t.Error("Fingerprint should differ for different slice IDs")
	}
}

func TestFingerprint_DifferentError(t *testing.T) {
	// Different error should produce different fingerprint.
	fp1 := Fingerprint("S03", "agent: turn cap (25) reached")
	fp2 := Fingerprint("S03", "implement: agent loop: timeout")
	if fp1 == fp2 {
		t.Error("Fingerprint should differ for different errors")
	}
}

func TestFingerprint_OnlyFirstLine(t *testing.T) {
	// Multi-line error: only the first line matters for fingerprinting.
	fp1 := Fingerprint("S03", "agent: turn cap (25) reached\nwith extra detail\nand more")
	fp2 := Fingerprint("S03", "agent: turn cap (25) reached")
	if fp1 != fp2 {
		t.Error("Fingerprint should use only the first line of the error")
	}
}

func TestShouldBreak_DifferentSliceDoesNotAffect(t *testing.T) {
	// Failures for a different slice should not count toward this slice.
	db := setupCircuitDB(t)
	release := "test-release"

	// Record 3 failures for S03, and 2 for S04.
	fpS03 := Fingerprint("S03", "agent: turn cap (25) reached")
	fpS04 := Fingerprint("S04", "agent: turn cap (25) reached")

	for i := 0; i < 2; i++ {
		_ = RecordFailure(db, release, "S03", fpS03)
		_ = RecordFailure(db, release, "S04", fpS04)
	}
	// S04 has 2 failures — no trip.
	if ShouldBreak(db, release, "S04", fpS04) {
		t.Error("ShouldBreak should not trip for S04 with only 2 failures")
	}
	// S03 has 2 failures — no trip.
	if ShouldBreak(db, release, "S03", fpS03) {
		t.Error("ShouldBreak should not trip for S03 with only 2 failures")
	}
	// Add one more for S03.
	_ = RecordFailure(db, release, "S03", fpS03)
	if !ShouldBreak(db, release, "S03", fpS03) {
		t.Error("ShouldBreak should trip for S03 after 3 failures")
	}
	// S04 still not tripped.
	if ShouldBreak(db, release, "S04", fpS04) {
		t.Error("ShouldBreak should not trip for S04 — only 2 failures")
	}
}