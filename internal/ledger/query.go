// Package ledger: query.go — load the verdict corpus and produce aggregates
// (pass-rate by model×slice_kind, attempts-to-pass distribution, gate-failure
// histogram) plus a plain-text Report renderer. Stdlib only; text/tabwriter.
package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

// ── Load ────────────────────────────────────────────────────────────────

// Load reads the JSONL verdict corpus at path and returns all records.
// Blank lines and unparseable lines are skipped silently (the corpus is
// append-only; a corrupted line from a crash is not a fatal error).
func Load(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no corpus yet — empty is valid
		}
		return nil, fmt.Errorf("ledger: open %s: %w", path, err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue // skip malformed line
		}
		records = append(records, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ledger: scan %s: %w", path, err)
	}
	return records, nil
}

// ── Aggregation types ───────────────────────────────────────────────────

// PassRateBucket is one row in the pass-rate-by-model×slice_kind table.
type PassRateBucket struct {
	Model     string
	SliceKind string
	Pass      int
	Fail      int
	Blocked   int
	Total     int
	PassRate  float64 // 0.0–1.0
}

// AttemptBucket is one row in the attempts-to-pass distribution.
type AttemptBucket struct {
	Attempts int // e.g. 1, 2, 3
	Count    int // how many PASS verdicts took this many attempts
}

// GateBucket is one row in the gate-failure histogram.
type GateBucket struct {
	Violation string
	Count     int
}

// ── Aggregators ─────────────────────────────────────────────────────────

// PassRateByModelKind returns pass-rate buckets grouped by (model, slice_kind).
// Only PASS, FAIL, and BLOCKED verdicts are counted. Pending records (from a
// partial corpus) are excluded from the total.
func PassRateByModelKind(records []Record) []PassRateBucket {
	type key struct{ model, kind string }
	type accum struct {
		pass, fail, blocked int
	}
	m := make(map[key]*accum)
	for _, r := range records {
		if r.Verdict != "pass" && r.Verdict != "fail" && r.Verdict != "blocked" {
			continue
		}
		k := key{r.Model, r.SliceKind}
		a := m[k]
		if a == nil {
			a = &accum{}
			m[k] = a
		}
		switch r.Verdict {
		case "pass":
			a.pass++
		case "fail":
			a.fail++
		case "blocked":
			a.blocked++
		}
	}
	var buckets []PassRateBucket
	for k, a := range m {
		total := a.pass + a.fail + a.blocked
		rate := 0.0
		if total > 0 {
			rate = float64(a.pass) / float64(total)
		}
		buckets = append(buckets, PassRateBucket{
			Model:     k.model,
			SliceKind: k.kind,
			Pass:      a.pass,
			Fail:      a.fail,
			Blocked:   a.blocked,
			Total:     total,
			PassRate:  rate,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Model != buckets[j].Model {
			return buckets[i].Model < buckets[j].Model
		}
		return buckets[i].SliceKind < buckets[j].SliceKind
	})
	return buckets
}

// AttemptsToPass returns the distribution of attempt numbers at which slices
// first reached a PASS verdict. Only PASS records are considered; an attempt
// of 0 (unrecorded) is skipped.
func AttemptsToPass(records []Record) []AttemptBucket {
	m := make(map[int]int)
	for _, r := range records {
		if r.Verdict != "pass" || r.Attempt <= 0 {
			continue
		}
		m[r.Attempt]++
	}
	var buckets []AttemptBucket
	for attempt, count := range m {
		buckets = append(buckets, AttemptBucket{Attempts: attempt, Count: count})
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Attempts < buckets[j].Attempts })
	return buckets
}

// GateFailureHistogram returns a frequency list of violation strings across
// all FAIL verdict records. Each unique violation is one bucket.
func GateFailureHistogram(records []Record) []GateBucket {
	m := make(map[string]int)
	for _, r := range records {
		if r.Verdict != "fail" {
			continue
		}
		for _, v := range r.Violations {
			m[v]++
		}
	}
	var buckets []GateBucket
	for v, count := range m {
		buckets = append(buckets, GateBucket{Violation: v, Count: count})
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Count != buckets[j].Count {
			return buckets[i].Count > buckets[j].Count // descending frequency
		}
		return buckets[i].Violation < buckets[j].Violation
	})
	return buckets
}

// ── Report renderer ─────────────────────────────────────────────────────

// Report renders the three aggregate tables in plain text using text/tabwriter.
type Report struct{}

// Render writes the report to w.
func (Report) Render(w io.Writer, records []Record) {
	if len(records) == 0 {
		fmt.Fprintln(w, "No verdict records — run 'sworn ledger sync' first.")
		return
	}

	// 1. Pass-rate by model × slice_kind
	fmt.Fprintln(w, "Pass-rate by model × slice_kind")
	fmt.Fprintln(w, "")
	buckets := PassRateByModelKind(records)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "MODEL\tSLICE_KIND\tPASS\tFAIL\tBLOCKED\tTOTAL\tRATE\n")
	for _, b := range buckets {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%.0f%%\n",
			b.Model, b.SliceKind, b.Pass, b.Fail, b.Blocked, b.Total, b.PassRate*100)
	}
	tw.Flush()
	fmt.Fprintln(w, "")

	// 2. Attempts-to-pass distribution
	fmt.Fprintln(w, "Attempts to pass")
	fmt.Fprintln(w, "")
	attempts := AttemptsToPass(records)
	if len(attempts) == 0 {
		fmt.Fprintln(w, "  (no PASS verdicts recorded)")
	} else {
		tw2 := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw2, "ATTEMPTS\tCOUNT\n")
		for _, a := range attempts {
			fmt.Fprintf(tw2, "%d\t%d\n", a.Attempts, a.Count)
		}
		tw2.Flush()
	}
	fmt.Fprintln(w, "")

	// 3. Gate-failure histogram
	fmt.Fprintln(w, "Gate-failure histogram")
	fmt.Fprintln(w, "")
	gates := GateFailureHistogram(records)
	if len(gates) == 0 {
		fmt.Fprintln(w, "  (no FAIL verdicts with violations recorded)")
	} else {
		tw3 := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw3, "VIOLATION\tCOUNT\n")
		for _, g := range gates {
			fmt.Fprintf(tw3, "%s\t%d\n", g.Violation, g.Count)
		}
		tw3.Flush()
	}
	fmt.Fprintln(w, "")

	// Summary line
	total := 0
	pass := 0
	fail := 0
	blocked := 0
	for _, r := range records {
		total++
		switch r.Verdict {
		case "pass":
			pass++
		case "fail":
			fail++
		case "blocked":
			blocked++
		}
	}
	fmt.Fprintf(w, "%d records: %d pass, %d fail, %d blocked\n", total, pass, fail, blocked)
}