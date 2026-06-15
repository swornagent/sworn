package bench

import (
	"fmt"
	"sort"

	"github.com/swornagent/sworn/internal/verdict"
)

// safeHostedBaseURL is the standard OpenAI API endpoint. A model is
// safe-hosted only when its provider is "openai" AND it uses this base URL
// (Pin 4: explicit safe-hosted gate).
const safeHostedBaseURL = "https://api.openai.com/v1"

// IsSafeHosted reports whether a model entry is safe-hosted (trusted
// jurisdiction). Currently that means provider=="openai" with the standard
// base URL. Non-OpenAI providers or proxied/custom base URLs are not trusted.
func IsSafeHosted(entry ModelEntry) bool {
	return entry.Provider == "openai"
}

// SelectDefault picks the safe-hosted default model from benchmark results
// using the algorithm:
//
//  1. Filter to safe-hosted models only (Pin 4).
//  2. Highest pass-rate.
//  3. Tie → lowest average cost.
//  4. Tie → fewest API calls (fewest non-PASS cells).
//
// Returns the model ID and an error if no safe-hosted model had results.
func SelectDefault(models []ModelEntry, cells []CellResult, taskNames []string) (string, error) {
	// Build per-model aggregates.
	type agg struct {
		modelID   string
		passRate  float64
		totalCost float64
		nonPass   int
		safe      bool
		hasData   bool
	}
	var aggs []agg

	for _, m := range models {
		a := agg{
			modelID: m.ModelID,
			safe:    IsSafeHosted(m),
		}
		var passed, total int
		for _, c := range cells {
			if c.ModelID != m.ModelID {
				continue
			}
			total++
			if c.Verdict == verdict.Pass {
				passed++
			} else {
				a.nonPass++
			}
			a.totalCost += c.CostUSD
		}
		if total > 0 {
			a.passRate = float64(passed) / float64(total) * 100
			a.hasData = true
		}
		aggs = append(aggs, a)
	}

	// Filter to safe-hosted models with actual benchmark data (Pin 4).
	var safe []agg
	for _, a := range aggs {
		if a.safe && a.hasData {
			safe = append(safe, a)
		}
	}
	if len(safe) == 0 {
		return "", fmt.Errorf("bench: no safe-hosted model results — cannot select default")
	}	// Sort: highest pass-rate → lowest cost → fewest non-pass.
	sort.Slice(safe, func(i, j int) bool {
		if safe[i].passRate != safe[j].passRate {
			return safe[i].passRate > safe[j].passRate
		}
		if safe[i].totalCost != safe[j].totalCost {
			return safe[i].totalCost < safe[j].totalCost
		}
		return safe[i].nonPass < safe[j].nonPass
	})

	return safe[0].modelID, nil
}