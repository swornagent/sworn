// Package ledger: routing.go — history-backed model recommendation.
// Consumes the verdict corpus to rank models by measured pass-rate per
// slice_kind, with a minimum-sample-size guard so thin evidence never
// silently changes the harness default. S56 adds cost-aware routing:
// OptimizeCost picks the cheapest model clearing a quality floor;
// OptimizeBalanced maximizes pass-rate per dollar.
package ledger

import (
	"sort"
)

// Objective selects the ranking strategy for RecommendModel.
type Objective int

const (
	// OptimizeQuality ranks by pass-rate (S54 behaviour, unchanged).
	OptimizeQuality Objective = iota
	// OptimizeCost picks the cheapest model whose pass-rate clears the floor.
	OptimizeCost
	// OptimizeBalanced maximises pass-rate per dollar.
	OptimizeBalanced
)

// String returns the lower-case name for CLI flag values.
func (o Objective) String() string {
	switch o {
	case OptimizeQuality:
		return "quality"
	case OptimizeCost:
		return "cost"
	case OptimizeBalanced:
		return "balanced"
	default:
		return "quality"
	}
}

// ParseObjective converts a string to an Objective. Unknown values
// default to OptimizeQuality.
func ParseObjective(s string) Objective {
	switch s {
	case "cost":
		return OptimizeCost
	case "balanced":
		return OptimizeBalanced
	default:
		return OptimizeQuality
	}
}

// modelStats is an intermediate aggregation for one model+kind combination.
type modelStats struct {
	model      string
	passRate   float64
	sample     int
	attempts   int     // best attempt that passed (or max int if none)
	meanCost   float64 // mean TotalCostUSD per record
	isUnpriced bool    // true when costRecords == 0 (all records have $0 cost)
}

// MinSampleSize is the minimum number of verdicts for a (model, slice_kind)
// before RecommendModel will return a confident recommendation. Below this
// threshold the engine refuses to route — it returns ok==false and the
// caller falls back to the existing precedence chain.
const MinSampleSize = 5

// DefaultPassRateFloor is the minimum pass-rate a model must clear to be
// eligible in cost-aware modes. Configurable via --floor flag.
const DefaultPassRateFloor = 0.8

// Recommendation is the ranked model pick for a slice kind and role.
type Recommendation struct {
	Model       string
	PassRate    float64 // 0.0–1.0, fraction of verdicts that are PASS
	Sample      int     // total verdicts (PASS + FAIL + BLOCKED) for this kind+model
	MeanCostUSD float64 // mean TotalCostUSD per record for this (model, kind)
	Objective   Objective
}

// RecommendModel ranks models for a (role, kind) by the given objective.
// Only PASS, FAIL, and BLOCKED verdicts are counted.
//
// OptimizeQuality: rank by pass-rate (desc), then attempts-to-pass (asc),
// then model name (deterministic tie-break). S54 behaviour unchanged.
//
// OptimizeCost: among models whose pass-rate ≥ floor and sample ≥
// MinSampleSize, pick the lowest mean TotalCostUSD; tie-break by pass-rate.
// Models where every record has TotalCostUSD == 0 (unpriced) are excluded
// from cost ranking — cost 0 means "no signal", never "free". If no model
// qualifies (all below floor or all unpriced), falls back to quality mode.
//
// OptimizeBalanced: among models clearing the sample guard, maximise
// pass-rate per dollar. Models with TotalCostUSD == 0 are excluded
// (division by zero → no signal).
//
// Returns (Recommendation, true) when a confident recommendation exists.
// Returns (Recommendation{}, false) when the corpus is absent, empty for
// the kind, or every candidate is below MinSampleSize.
func RecommendModel(records []Record, role string, kind string, obj Objective, floor float64) (Recommendation, bool) {
	if floor <= 0 {
		floor = DefaultPassRateFloor
	}

	// Aggregate pass/fail/blocked + best attempt + total cost for each model on
	// this kind.
	type accum struct {
		pass, fail, blocked int
		bestAttempt         int // smallest attempt that produced a PASS
		totalCost           float64
		costRecords         int // number of records with non-zero cost
	}
	m := make(map[string]*accum)
	for _, r := range records {
		if r.SliceKind != kind {
			continue
		}
		if r.Verdict != "pass" && r.Verdict != "fail" && r.Verdict != "blocked" {
			continue
		}
		a := m[r.Model]
		if a == nil {
			a = &accum{bestAttempt: -1}
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
		a.totalCost += r.TotalCostUSD
		if r.TotalCostUSD > 0 {
			a.costRecords++
		}
	}

	if len(m) == 0 {
		return Recommendation{}, false
	}

	// Build candidates.
	var candidates []modelStats
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
		meanCost := 0.0
		if sample > 0 {
			meanCost = a.totalCost / float64(sample)
		}
		candidates = append(candidates, modelStats{
			model:      model,
			passRate:   rate,
			sample:     sample,
			attempts:   attempts,
			meanCost:   meanCost,
			isUnpriced: a.costRecords == 0,
		})
	}

	// Sort according to the objective.
	sortCandidates(candidates, obj)

	// Select the best candidate that meets the mode's criteria.
	var best *modelStats
	switch obj {
	case OptimizeQuality:
		if candidates[0].sample >= MinSampleSize {
			best = &candidates[0]
		}
	case OptimizeCost:
		best = pickCost(candidates, floor)
	case OptimizeBalanced:
		best = pickBalanced(candidates)
	}

	if best == nil {
		return Recommendation{}, false
	}

	return Recommendation{
		Model:       best.model,
		PassRate:    best.passRate,
		Sample:      best.sample,
		MeanCostUSD: best.meanCost,
		Objective:   obj,
	}, true
}

// sortCandidates sorts candidates according to the objective.
func sortCandidates(candidates []modelStats, obj Objective) {
	sort.Slice(candidates, func(i, j int) bool {
		switch obj {
		case OptimizeQuality:
			return qualityLess(candidates[i], candidates[j])
		case OptimizeCost:
			return costLess(candidates[i], candidates[j])
		case OptimizeBalanced:
			return balancedLess(candidates[i], candidates[j])
		default:
			return qualityLess(candidates[i], candidates[j])
		}
	})
}

// qualityLess provides S54 ordering: higher pass-rate first, then fewer
// attempts, then deterministic model name tie-break.
func qualityLess(a, b modelStats) bool {
	if a.passRate != b.passRate {
		return a.passRate > b.passRate
	}
	if a.attempts != b.attempts {
		return a.attempts < b.attempts
	}
	return a.model < b.model
}

// costLess orders by mean cost ascending, then pass-rate descending, then
// model name (deterministic tie-break).
func costLess(a, b modelStats) bool {
	if a.meanCost != b.meanCost {
		return a.meanCost < b.meanCost
	}
	if a.passRate != b.passRate {
		return a.passRate > b.passRate
	}
	return a.model < b.model
}

// balancedLess orders by pass-rate per dollar descending (higher is better),
// then model name. Unpriced models sort last.
func balancedLess(a, b modelStats) bool {
	// Unpriced models (no cost signal) sort after priced models.
	if a.isUnpriced != b.isUnpriced {
		return !a.isUnpriced // priced before unpriced
	}
	// Both priced: compare pass-rate per dollar.
	if !a.isUnpriced && !b.isUnpriced {
		aRate := a.passRate / a.meanCost
		bRate := b.passRate / b.meanCost
		if aRate != bRate {
			return aRate > bRate
		}
	}
	// Both unpriced or equal rate/dollar: deterministic tie-break.
	return a.model < b.model
}

// pickCost selects the best candidate in OptimizeCost mode: first
// candidate (cheapest in cost-sorted order) that clears the sample
// floor, quality floor, and is priced. If none qualifies, falls back
// to quality mode.
func pickCost(candidates []modelStats, floor float64) *modelStats {
	// Already sorted by cost ascending, then pass-rate descending.
	for i := range candidates {
		c := &candidates[i]
		if c.sample < MinSampleSize {
			continue
		}
		if c.passRate < floor {
			continue
		}
		if c.isUnpriced {
			continue
		}
		return c
	}
	// Fall back to quality mode — pick the best pass-rate that clears
	// MinSampleSize (even if below the floor — best available).
	sort.Slice(candidates, func(i, j int) bool {
		return qualityLess(candidates[i], candidates[j])
	})
	for i := range candidates {
		if candidates[i].sample >= MinSampleSize {
			return &candidates[i]
		}
	}
	return nil
}

// pickBalanced selects the best candidate in OptimizeBalanced mode:
// first candidate (best pass-rate per dollar in balanced-sorted order)
// that clears the sample guard.
func pickBalanced(candidates []modelStats) *modelStats {
	for i := range candidates {
		c := &candidates[i]
		if c.sample < MinSampleSize {
			continue
		}
		if c.isUnpriced {
			continue
		}
		return c
	}
	// Fall back to quality mode.
	sort.Slice(candidates, func(i, j int) bool {
		return qualityLess(candidates[i], candidates[j])
	})
	for i := range candidates {
		if candidates[i].sample >= MinSampleSize {
			return &candidates[i]
		}
	}
	return nil
}
