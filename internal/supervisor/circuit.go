package supervisor

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CircuitBreakerThreshold is the number of consecutive same-fingerprint
// failures before the circuit breaker fires. Hardcoded at 3 per the S03 spec
// (configurable threshold is out of scope).
const CircuitBreakerThreshold = 3

// Fingerprint computes a deterministic fingerprint for a failure: the SHA-256
// hex digest of sliceID + first line of the error (trimmed). This encodes
// neither session ID nor timestamp, so the same logical failure across runs
// produces the same fingerprint.
func Fingerprint(sliceID, errorLine string) string {
	data := sliceID + "\x00" + strings.TrimSpace(firstLine(errorLine))
	sum := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", sum)
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			return t
		}
	}
	return ""
}

// RecordFailure inserts a circuit-failure row for the given slice and
// fingerprint. The caller (the worker) computes the fingerprint from the
// error returned by RunSliceFn.
func RecordFailure(db *sql.DB, release, sliceID, fingerprint string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		`INSERT INTO circuit_failures (slice_id, release, fingerprint, recorded_at)
		 VALUES (?, ?, ?, ?)`,
		sliceID, release, fingerprint, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// ShouldBreak returns true if the circuit breaker should fire for this
// slice+fingerprint — i.e., the most recent CircuitBreakerThreshold failures
// all have exactly this fingerprint, with no intervening different
// fingerprint.
//
// If the DB is unavailable (nil or query error), ShouldBreak returns false
// (fail-open on circuit breaker — AC4: "IF the supervisor DB is unavailable,
// THE SYSTEM SHALL default to ShouldBreak returning false").
func ShouldBreak(db *sql.DB, release, sliceID, fingerprint string) bool {
	if db == nil {
		return false
	}

	// Fetch the most recent N failure rows for this slice+release.
	rows, err := db.Query(
		`SELECT fingerprint FROM circuit_failures
		 WHERE slice_id = ? AND release = ?
		 ORDER BY id DESC
		 LIMIT ?`,
		sliceID, release, CircuitBreakerThreshold,
	)
	if err != nil {
		return false // fail-open
	}
	defer rows.Close()

	var recent []string
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			return false
		}
		recent = append(recent, fp)
	}
	if err := rows.Err(); err != nil {
		return false
	}

	// Need exactly CircuitBreakerThreshold entries, all matching.
	if len(recent) < CircuitBreakerThreshold {
		return false
	}
	for _, fp := range recent {
		if fp != fingerprint {
			return false
		}
	}
	return true
}
