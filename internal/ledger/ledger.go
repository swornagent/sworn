// Package ledger provides the append-only verdict ledger: a git-tracked
// docs/ledger/verdicts.jsonl corpus that Project-s from every slice's
// status.json verification block. One line per verdict — every PASS / FAIL
// / BLOCKED the board has ever recorded, queryable in one place.
//
// Stdlib only — zero runtime dependencies.
package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/state"
)

// Record is one line in the verdict ledger. It captures the implementer model
// and attempt number at the verdict-record site so downstream consumers (S54
// history-backed routing) can answer model-vs-outcome questions.
type Record struct {
	V    int    `json:"v"`
	Ts   string `json:"ts"`
	Release  string `json:"release"`
	Track    string `json:"track"`
	SliceID  string `json:"slice_id"`
	// SliceKind is a rubric dimension derived from the track id
	// (e.g. T5-providers→provider, T16-verdict-ledger→ledger).
	SliceKind string `json:"slice_kind"`
	// Role is the agent role whose model is recorded (always "implementer").
	Role    string `json:"role"`
	Model   string `json:"model,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Verdict string `json:"verdict"`
	State   string `json:"state"`
	// FreshContext records whether the verifier session was fresh.
	FreshContext *bool `json:"fresh_context,omitempty"`
	// VerifierSessionID is the session identifier from the verifier.
	VerifierSessionID string `json:"verifier_session_id,omitempty"`
	// Violations are the verifier's listed violations (or empty).
	Violations []string `json:"violations,omitempty"`
	// GateCount is the number of acceptance checks ( - [ ] lines) in the slice spec.
	GateCount int `json:"gate_count"`
	// ViolationCount is len(Violations).
	ViolationCount int `json:"violation_count"`
	// SwornVersion is the baton protocol version the binary embeds.
	SwornVersion string `json:"sworn_version"`

	// Dispatches records per-role model + cost for each dispatch during the
	// slice run (v:2 field). Omitted from JSON when empty for back-compat
	// with v:1 lines.
	Dispatches []state.Dispatch `json:"dispatches,omitempty"`
	// TotalCostUSD is the sum of all Dispatch.CostUSD values (v:2 field).
	// Convenience field for reporting; derived from Dispatches.
	TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
}

// SliceKind derives a rubric dimension from a track id. It strips the
// "T<number>-" prefix, then splits the remainder on "-" and takes the
// first segment, de-pluralising where it looks like a common plural.
// The examples from the spec:
//
//	T5-providers        → provider
//	T12-harness-hardening → harness
//	T8-memory           → memory
//	T3-commercial       → commercial
//	T16-verdict-ledger  → verdict (first segment; spec note: spec example
//	                      says "ledger" — kept first-segment rule for consistency
//	                      across all tracks; any non-mechanical mapping is the
//	                      planner's domain)
//
// Returns "other" for tracks that don't match the expected T<n>-... pattern.
func SliceKind(track string) string {
	// Must match "T<number>-..." pattern.
	if len(track) < 3 || track[0] != 'T' {
		return "other"
	}
	idx := strings.IndexByte(track, '-')
	if idx < 1 {
		return "other"
	}
	// The part between T and the first dash must be numeric.
	for _, c := range track[1:idx] {
		if c < '0' || c > '9' {
			return "other"
		}
	}
	rest := track[idx+1:]

	// Take the first segment before any subsequent "-".
	if dash := strings.IndexByte(rest, '-'); dash >= 0 {
		rest = rest[:dash]
	}

	// De-pluralise: remove trailing "s" from words that end in 's' but not 'ss'
	// (so "providers" → "provider" but "harness" → "harness").
	if len(rest) >= 2 && rest[len(rest)-1] == 's' && rest[len(rest)-2] != 's' {
		rest = rest[:len(rest)-1]
	}

	if rest == "" {
		return "other"
	}
	return rest
}
// CountLines returns the number of non-empty lines in the ledger file.
// Returns 0 if the file does not exist.
func CountLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	n := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			n++
		}
	}
	return n
}

// Key returns a deduplication key for a Record.// Two records with the same SliceID + Verdict + Ts are the same entry.
func Key(r Record) string {
	return fmt.Sprintf("%s|%s|%s", r.SliceID, r.Verdict, r.Ts)
}

// Project creates a Record from a slice's Status. Returns (Record, true) when
// the slice has a terminal verdict (verification.result is non-empty and not
// "pending"). Returns (Record{}, false) for slices with no verdict to record.
func Project(st *state.Status, gateCount int) (Record, bool) {
	v := st.Verification
	if v.Result == "" || v.Result == "pending" {
		return Record{}, false
	}

	r := Record{
		V:                 2,
		Ts:                time.Now().UTC().Format(time.RFC3339),
		Release:           st.Release,
		Track:             st.Track,
		SliceID:           st.SliceID,
		SliceKind:         SliceKind(st.Track),
		Role:              "implementer",
		Model:             v.Model,
		Attempt:           v.Attempt,
		Verdict:           v.Result,
		State:             string(st.State),
		FreshContext:      v.VerifierWasFreshContext,
		Violations:        v.Violations,
		GateCount:         gateCount,
		ViolationCount:    len(v.Violations),
		Dispatches:         v.Dispatches,
		SwornVersion:       baton.Version(),
	}
	var totalCost float64
	for _, d := range v.Dispatches {
		totalCost += d.CostUSD
	}
	r.TotalCostUSD = totalCost
	if v.VerifierSessionID != nil {
		r.VerifierSessionID = *v.VerifierSessionID
	}
	return r, true
}

// Append writes a Record as one JSON line to the verdicts.jsonl ledger file.
// Creates docs/ledger/ and the file if absent. Before appending, it scans
// the file for an existing record with the same Key (idempotent re-sync);
// if found, the write is a no-op. Returns nil on success.
func Append(path string, r Record) error {
	// Ensure the parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ledger: create dir %s: %w", dir, err)
	}

	targetKey := Key(r)

	// Idempotency guard: if the key already exists, skip.
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("ledger: open %s: %w", path, err)
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var existing Record
		if err := json.Unmarshal([]byte(line), &existing); err != nil {
			continue // skip malformed lines
		}
		if Key(existing) == targetKey {
			f.Close()
			return nil // already recorded
		}
	}
	f.Close()
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ledger: scan %s: %w", path, err)
	}

	// Append the new record.
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("ledger: marshal record: %w", err)
	}
	data = append(data, '\n')

	af, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("ledger: open %s for append: %w", path, err)
	}
	defer af.Close()

	if _, err := af.Write(data); err != nil {
		return fmt.Errorf("ledger: write %s: %w", path, err)
	}
	return nil
}