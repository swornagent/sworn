// Package ledger: query.go — load the verdict corpus and produce aggregates
// (pass-rate by model×slice_kind, attempts-to-pass distribution, gate-failure
// histogram, cost-per-pass, per-role quality) plus a plain-text Report renderer.
// Stdlib only; text/tabwriter.
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

// CostPerPassBucket is one row in the cost-per-passing-slice table.
type CostPerPassBucket struct {
	Model     string
	SliceKind string
	PassCount int
	TotalCost float64
	MeanCost  float64 // TotalCost / sample count
}

// PerRoleQuality holds derived quality signals for a single role.
type PerRoleQuality struct {
	Role         string
	Sample       int     // records with this role in Dispatches
	MissRate     float64 // fraction of captain-passed slices that later FAIL/BLOCKED
	OverturnRate float64 // fraction of verifier verdicts later overturned
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

// CostPerPassingSlice returns cost-per-passing-slice buckets grouped by
// (model, slice_kind). Only PASS verdicts are counted (cost of a failing
// slice is still spent, but the "per passing slice" metric is what the
// spec calls for). MeanCost is TotalCost / record count for that bucket.
func CostPerPassingSlice(records []Record) []CostPerPassBucket {
	type key struct{ model, kind string }
	type accum struct {
		passCount int
		totalCost float64
		sample    int // total records for this bucket
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
		a.sample++
		a.totalCost += r.TotalCostUSD
		if r.Verdict == "pass" {
			a.passCount++
		}
	}
	var buckets []CostPerPassBucket
	for k, a := range m {
		mean := 0.0
		if a.sample > 0 {
			mean = a.totalCost / float64(a.sample)
		}
		buckets = append(buckets, CostPerPassBucket{
			Model:     k.model,
			SliceKind: k.kind,
			PassCount: a.passCount,
			TotalCost: a.totalCost,
			MeanCost:  mean,
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

// CaptainMissRate computes the share of slices where the captain dispatched
// (has a dispatch entry with role="captain") and the implementer verdict was
// FAIL or BLOCKED. This is a derived quality signal: captain approved a
// design that later failed verification.
//
// Returns 0 when there are no captain dispatches in the corpus.
func CaptainMissRate(records []Record) float64 {
	total := 0
	misses := 0
	for _, r := range records {
		hasCaptain := false
		for _, d := range r.Dispatches {
			if d.Role == "captain" {
				hasCaptain = true
				break
			}
		}
		if !hasCaptain {
			continue
		}
		total++
		if r.Verdict == "fail" || r.Verdict == "blocked" {
			misses++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(misses) / float64(total)
}

// VerifierOverturnRate computes the share of verifier verdicts that were
// later overturned. An overturn is detected when multiple records exist for
// the same SliceID with different verdicts, indicating a re-verification
// flipped the outcome.
//
// For v:2 corpus (one terminal verdict per slice), this will typically
// return 0 — overturn tracking requires multi-verdict-per-slice data.
func VerifierOverturnRate(records []Record) float64 {
	// Group records by SliceID.
	type verdicts struct {
		first string
		last  string
		count int
	}
	m := make(map[string]*verdicts)
	for _, r := range records {
		v := m[r.SliceID]
		if v == nil {
			v = &verdicts{first: r.Verdict, last: r.Verdict, count: 1}
			m[r.SliceID] = v
		} else {
			v.last = r.Verdict
			v.count++
		}
	}

	// A slice with >=2 records and different first/last verdict was overturned.
	total := 0
	overturns := 0
	for _, v := range m {
		if v.count >= 2 {
			total++
			if v.first != v.last {
				overturns++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(overturns) / float64(total)
}

// PerRoleQualityAll returns per-role quality signals for every role that
// appears in the Dispatches across the corpus.
func PerRoleQualityAll(records []Record) []PerRoleQuality {
	// Collect the set of roles that appear in dispatches.
	roleSet := make(map[string]bool)
	for _, r := range records {
		for _, d := range r.Dispatches {
			roleSet[d.Role] = true
		}
	}
	// Always include "captain" and "verifier" for report consistency even
	// if no dispatches exist yet.
	if len(roleSet) == 0 {
		roleSet["captain"] = true
		roleSet["verifier"] = true
	}

	var out []PerRoleQuality
	for role := range roleSet {
		sample := 0
		for _, r := range records {
			for _, d := range r.Dispatches {
				if d.Role == role {
					sample++
					break
				}
			}
		}

		pq := PerRoleQuality{
			Role:         role,
			Sample:       sample,
			MissRate:     0,
			OverturnRate: 0,
		}
		if role == "captain" {
			pq.MissRate = CaptainMissRate(records)
		}
		if role == "verifier" {
			pq.OverturnRate = VerifierOverturnRate(records)
		}
		out = append(out, pq)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out
}

// ── Report renderer ─────────────────────────────────────────────────────

// Report renders the aggregate tables in plain text using text/tabwriter.
type Report struct{}

// Render writes the report to w.
func (Report) Render(w io.Writer, records []Record) {
	if len(records) == 0 {
		fmt.Fprintln(w, "No verdict records — run 'sworn ledger sync' first.")
		return
	}

	// 1. Pass-rate by model × slice_kind (with cost columns)
	fmt.Fprintln(w, "Pass-rate by model × slice_kind")
	fmt.Fprintln(w, "")
	buckets := PassRateByModelKind(records)
	costBuckets := CostPerPassingSlice(records)
	costMap := make(map[string]float64) // "model|kind" -> mean cost
	for _, c := range costBuckets {
		costMap[c.Model+"|"+c.SliceKind] = c.MeanCost
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "MODEL\tSLICE_KIND\tPASS\tFAIL\tBLOCKED\tTOTAL\tRATE\tCOST/EA\n")
	for _, b := range buckets {
		key := b.Model + "|" + b.SliceKind
		cost := costMap[key]
		costStr := "—"
		if cost > 0 {
			costStr = fmt.Sprintf("$%.4f", cost)
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%.0f%%\t%s\n",
			b.Model, b.SliceKind, b.Pass, b.Fail, b.Blocked, b.Total, b.PassRate*100, costStr)
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

	// 4. Per-role quality
	fmt.Fprintln(w, "Per-role quality")
	fmt.Fprintln(w, "")
	pqs := PerRoleQualityAll(records)
	tw4 := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw4, "ROLE\tSAMPLE\tMISS_RATE\tOVERTURN_RATE\n")
	for _, pq := range pqs {
		fmt.Fprintf(tw4, "%s\t%d\t%.1f%%\t%.1f%%\n",
			pq.Role, pq.Sample, pq.MissRate*100, pq.OverturnRate*100)
	}
	tw4.Flush()
	fmt.Fprintln(w, "")

	// Summary line
	total := 0
	pass := 0
	fail := 0
	blocked := 0
	var totalCost float64
	for _, r := range records {
		total++
		totalCost += r.TotalCostUSD
		switch r.Verdict {
		case "pass":
			pass++
		case "fail":
			fail++
		case "blocked":
			blocked++
		}
	}
	fmt.Fprintf(w, "%d records: %d pass, %d fail, %d blocked, $%.4f total cost\n",
		total, pass, fail, blocked, totalCost)
}
