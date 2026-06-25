// Package ledger: routing.go — history-backed model recommendation.
// Consumes the verdict corpus to rank models by measured pass-rate per
// slice_kind, with a minimum-sample-size guard so thin evidence never
// silently changes the harness default.
package ledger

import (
	"sort"
)

// MinSampleSize is the minimum number of verdicts for a (model, slice_kind)
// before RecommendModel will return a confident recommendation. Below this
// threshold the engine refuses to route — it returns ok==false and the
// caller falls back to the existing precedence chain.
const MinSampleSize = 5

// Recommendation is the ranked model pick for a slice kind.
type Recommendation struct {
	Model    string
	PassRate float64 // 0.0–1.0, fraction of verdicts that are PASS
	Sample   int     // total verdicts (PASS + FAIL + BLOCKED) for this kind+model
}

// RecommendModel ranks models for kind by pass-rate (desc), then
// attempts-to-pass (asc, fewer is better), then model name (deterministic
// tie-break). Only PASS, FAIL, and BLOCKED verdicts are counted.
//
// Returns (Recommendation, true) when the top-ranked model clears the
// minimum-sample threshold. Returns (Recommendation{}, false) when there
// is no confident recommendation — the corpus is absent, empty for the
// kind, or every candidate is below MinSampleSize.
func RecommendModel(records []Record, kind string) (Recommendation, bool) {
	// Aggregate pass/fail/blocked + best attempt for each model on this kind.
	type acc struct {
		pass, fail, blocked int
		bestAttempt         int // smallest attempt that produced a PASS
	}
	m := make(map[string]*acc)
	for _, r := range records {
		if r.SliceKind != kind {
			continue
		}
		if r.Verdict != "pass" && r.Verdict != "fail" && r.Verdict != "blocked" {
			continue
		}
		a := m[r.Model]
		if a == nil {
			a = &acc{bestAttempt: -1}
			m[r.Model] = a
		}
		switch r.Verdict {
		case "pass":
			a.pass++
			if a.bestAttempt < 0 || r.Attempt < a.bestAttempt {
				a.bestAttempt = r.Attempt
			}
		case "fail":
			a.fail++
		case "blocked":
			a.blocked++
		}
	}

	if len(m) == 0 {
		return Recommendation{}, false
	}

	// Build candidates.
	type candidate struct {
		model      string
		passRate   float64
		sample     int
		attempts   int // best attempt that passed (or max int if none)
	}
	var candidates []candidate
	for model, a := range m {
		sample := a.pass + a.fail + a.blocked
		rate := 0.0
		if sample > 0 {
			rate = float64(a.pass) / float64(sample)
		}
		attempts := a.bestAttempt
		if attempts < 0 {
			attempts = 1<<31 - 1 // no PASS verdict — sort last
		}
		candidates = append(candidates, candidate{
			model:    model,
			passRate: rate,
			sample:   sample,
			attempts: attempts,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		// 1. Higher pass-rate first
		if candidates[i].passRate != candidates[j].passRate {
			return candidates[i].passRate > candidates[j].passRate
		}
		// 2. Fewer attempts-to-pass (fewer is better)
		if candidates[i].attempts != candidates[j].attempts {
			return candidates[i].attempts < candidates[j].attempts
		}
		// 3. Deterministic tie-break
		return candidates[i].model < candidates[j].model
	})

	best := candidates[0]
	if best.sample < MinSampleSize {
		return Recommendation{}, false
	}

	return Recommendation{
		Model:    best.model,
		PassRate: best.passRate,
		Sample:   best.sample,
	}, true
}