// Package lint implements the `sworn lint` sub-targets that perform
// mechanical, pre-verification checks on release slices.
//
// Status-time checks validate that Baton status.json metadata timestamps
// (last_updated_at, verification.verifier_verdict_at) are not in the
// future beyond a small clock-skew allowance, ensuring the board never
// silently renders impossible dates.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Clock abstracts time.Now() for testability.
type Clock interface {
	Now() time.Time
}

// realClock delegates to time.Now.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// DefaultClock is the real wall clock. Tests inject a fixed clock.
var DefaultClock Clock = realClock{}

// maxFutureSkew is the allowed future offset for timestamps.
// A timestamp up to 5 minutes in the future is treated as clock skew
// and passes; anything further out fails closed.
const maxFutureSkew = 5 * time.Minute

// StatusTimeViolation describes one future/malformed timestamp in a
// status.json file.
type StatusTimeViolation struct {
	SliceID   string // e.g. "S64-status-timestamp-sanity"
	Release   string // the release directory name
	Field     string // "last_updated_at" or "verification.verifier_verdict_at"
	Value     string // the raw string from JSON
	AllowedAt string // RFC 3339 of now+5m, for error messages
}

// statusTimeViolations is a sortable collection.
type statusTimeViolations []StatusTimeViolation

func (vs statusTimeViolations) sort() {
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].SliceID != vs[j].SliceID {
			return vs[i].SliceID < vs[j].SliceID
		}
		return vs[i].Field < vs[j].Field
	})
}

// Error returns all violations as human-readable lines.
func (vs statusTimeViolations) Error() string {
	var b strings.Builder
	for i, v := range vs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(v.String())
	}
	return b.String()
}

func (v StatusTimeViolation) String() string {
	return fmt.Sprintf("slice %s (%s/%s): %s is %q — exceeds allowed maximum %s",
		v.SliceID, v.Release, v.SliceID, v.Field, v.Value, v.AllowedAt)
}

// CheckStatusTimestamps walks the release directory tree, reads every
// status.json, and validates last_updated_at and verification.verifier_verdict_at
// against clock.Now() + maxFutureSkew. Unparsable timestamps fail closed.
//
// A nil clock uses DefaultClock (real wall clock).
func CheckStatusTimestamps(releaseDir string, clock Clock) []StatusTimeViolation {
	if clock == nil {
		clock = DefaultClock
	}
	now := clock.Now()
	allowed := now.Add(maxFutureSkew)
	allowedStr := allowed.Format(time.RFC3339)

	var violations []StatusTimeViolation

	// Derive the release name from the directory basename.
	releaseName := filepath.Base(releaseDir)

	// Walk the release directory for status.json files.
	filepath.Walk(releaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if info.Name() != "status.json" {
			return nil
		}
		// Determine slice id from parent directory.
		sliceDir := filepath.Dir(path)
		sliceID := filepath.Base(sliceDir)

		// Read the file raw — we need the raw timestamp strings, not
		// the parsed struct, because we want to surface unparsable values.
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable
		}

		// Check last_updated_at.
		violations = append(violations, checkTimestampField(data, sliceID, releaseName, "last_updated_at", now, allowed, allowedStr)...)

		// Check verification.verifier_verdict_at (optional — only when present).
		violations = append(violations, checkTimestampField(data, sliceID, releaseName, "verification.verifier_verdict_at", now, allowed, allowedStr)...)

		return nil
	})

	statusTimeViolations(violations).sort()
	return violations
}

// checkTimestampField extracts and validates a single timestamp field from
// status.json raw bytes. It handles both top-level fields
// ("last_updated_at") and nested fields ("verification.verifier_verdict_at").
func checkTimestampField(data []byte, sliceID, releaseName, fieldPath string, now, allowed time.Time, allowedStr string) []StatusTimeViolation {
	// Extract the raw JSON string value for this field.
	value := extractJSONField(data, fieldPath)
	if value == "" {
		return nil
	}

	// Parse as RFC 3339.
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return []StatusTimeViolation{{
			SliceID:   sliceID,
			Release:   releaseName,
			Field:     fieldPath,
			Value:     value,
			AllowedAt: allowedStr,
		}}
	}

	// Reject future timestamps beyond skew allowance.
	if t.After(allowed) {
		return []StatusTimeViolation{{
			SliceID:   sliceID,
			Release:   releaseName,
			Field:     fieldPath,
			Value:     value,
			AllowedAt: allowedStr,
		}}
	}

	return nil
}

// extractJSONField does a simple string-scan extract of the first occurrence
// of "field": "value" in the JSON blob. It handles dotted paths for nested
// objects (e.g. "verification.verifier_verdict_at" looks inside the
// "verification" block). This avoids depending on json.Unmarshal for raw
// string extraction and lets us surface unparsable values exactly as written.
func extractJSONField(data []byte, fieldPath string) string {
	parts := strings.Split(fieldPath, ".")
	search := data

	for i, part := range parts {
		isLast := i == len(parts)-1
		if isLast {
			return extractJSONStringValue(search, part)
		}
		// Navigate into nested object.
		search = findJSONObject(search, part)
		if search == nil {
			return ""
		}
	}
	return ""
}

// findJSONObject finds the JSON object following "key": { and returns its raw bytes.
func findJSONObject(data []byte, key string) []byte {
	// Build the key-with-quotes pattern.
	needle := `"` + key + `"`
	idx := indexAfter(data, needle)
	if idx < 0 {
		return nil
	}
	// Skip whitespace and colon.
	rest := data[idx:]
	colonIdx := -1
	for i, b := range rest {
		if b == ':' {
			colonIdx = i
			break
		}
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return nil // unexpected
		}
	}
	if colonIdx < 0 {
		return nil
	}
	rest = rest[colonIdx+1:]

	// Skip whitespace to find opening brace.
	braceIdx := -1
	for i, b := range rest {
		if b == '{' {
			braceIdx = i
			break
		}
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return nil
		}
	}
	if braceIdx < 0 {
		return nil
	}
	rest = rest[braceIdx:]

	// Find matching closing brace.
	depth := 0
	closeIdx := -1
	for i, b := range rest {
		if b == '{' {
			depth++
		} else if b == '}' {
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx < 0 {
		return nil
	}
	return rest[:closeIdx+1]
}

// extractJSONStringValue extracts the string value of key from a flat JSON
// object blob. Returns "" if key is absent or value is not a quoted string.
func extractJSONStringValue(data []byte, key string) string {
	needle := `"` + key + `"`
	idx := indexAfter(data, needle)
	if idx < 0 {
		return ""
	}
	rest := data[idx:]

	// Skip whitespace and colon.
	colonIdx := -1
	for i, b := range rest {
		if b == ':' {
			colonIdx = i
			break
		}
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return ""
		}
	}
	if colonIdx < 0 {
		return ""
	}
	rest = rest[colonIdx+1:]

	// Skip whitespace to find opening quote.
	quoteIdx := -1
	for i, b := range rest {
		if b == '"' {
			quoteIdx = i
			break
		}
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return ""
		}
	}
	if quoteIdx < 0 {
		return ""
	}
	rest = rest[quoteIdx+1:]

	// Find closing unescaped quote.
	closeIdx := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if rest[i] == '"' {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return ""
	}
	return string(rest[:closeIdx])
}

// indexAfter returns the index immediately after the first occurrence of
// needle in data, or -1 if not found.
func indexAfter(data []byte, needle string) int {
	idx := strings.Index(string(data), needle)
	if idx < 0 {
		return -1
	}
	return idx + len(needle)
}