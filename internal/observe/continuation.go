package observe

import (
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/driver"
)

const continuationEventSegment = "continuation"

const (
	continuationOutcomeReuse           = "reuse"
	continuationOutcomeFallback        = "fallback"
	continuationOutcomeFallbackExpired = "fallback_expired"
)

// ContinuationCount is one closed, content-free continuation observation.
// Neither the event base nor any adapter-owned continuation data is retained.
type ContinuationCount struct {
	Mode    string `json:"mode"`
	Outcome string `json:"outcome"`
	Count   int64  `json:"count"`
}

// ContinuationSummary is cumulative at the evaluation high-water mark.
// Fallback includes fallback_expired; Expired counts only fallback_expired.
type ContinuationSummary struct {
	Reused   int64               `json:"reused"`
	Fallback int64               `json:"fallback"`
	Expired  int64               `json:"expired"`
	Counts   []ContinuationCount `json:"counts"`
}

type continuationKey struct {
	mode    string
	outcome string
}

type continuationAggregate struct {
	reused   int64
	fallback int64
	expired  int64
	counts   map[continuationKey]int64
}

func (a *continuationAggregate) add(kind string) error {
	key, marked, err := parseContinuationEvent(kind)
	if err != nil || !marked {
		return err
	}
	if a.counts == nil {
		a.counts = make(map[continuationKey]int64)
	}
	count, err := safeAdd(a.counts[key], 1)
	if err != nil {
		return err
	}
	switch key.outcome {
	case continuationOutcomeReuse:
		a.reused, err = safeAdd(a.reused, 1)
	case continuationOutcomeFallback:
		a.fallback, err = safeAdd(a.fallback, 1)
	case continuationOutcomeFallbackExpired:
		a.fallback, err = safeAdd(a.fallback, 1)
		if err == nil {
			a.expired, err = safeAdd(a.expired, 1)
		}
	}
	if err != nil {
		return err
	}
	a.counts[key] = count
	return nil
}

func (a continuationAggregate) summary() ContinuationSummary {
	result := ContinuationSummary{
		Reused:   a.reused,
		Fallback: a.fallback,
		Expired:  a.expired,
	}
	if len(a.counts) == 0 {
		return result
	}
	result.Counts = make([]ContinuationCount, 0, len(a.counts))
	for key, count := range a.counts {
		result.Counts = append(result.Counts, ContinuationCount{
			Mode:    key.mode,
			Outcome: key.outcome,
			Count:   count,
		})
	}
	sort.Slice(result.Counts, func(left, right int) bool {
		l, r := result.Counts[left], result.Counts[right]
		if l.Mode != r.Mode {
			return l.Mode < r.Mode
		}
		return l.Outcome < r.Outcome
	})
	return result
}

func parseContinuationEvent(
	kind string,
) (continuationKey, bool, error) {
	parts := strings.Split(kind, ".")
	marker := -1
	for index, part := range parts {
		if part != continuationEventSegment {
			continue
		}
		if marker != -1 {
			return continuationKey{}, true,
				fail("INVALID_EVALUATION_FACT")
		}
		marker = index
	}
	if marker == -1 {
		return continuationKey{}, false, nil
	}
	if marker == 0 || marker+3 != len(parts) {
		return continuationKey{}, true, fail("INVALID_EVALUATION_FACT")
	}
	for _, part := range parts[:marker] {
		if part == "" {
			return continuationKey{}, true,
				fail("INVALID_EVALUATION_FACT")
		}
	}
	mode, outcome := parts[marker+1], parts[marker+2]
	if !validContinuationMode(mode) ||
		!validContinuationOutcome(outcome) {
		return continuationKey{}, true, fail("INVALID_EVALUATION_FACT")
	}
	return continuationKey{mode: mode, outcome: outcome}, true, nil
}

func validContinuationMode(value string) bool {
	switch driver.ContinuationMode(value) {
	case driver.ContinuationModeFreshRehydrate,
		driver.ContinuationModeTranscriptReplay,
		driver.ContinuationModeOpaqueReplay,
		driver.ContinuationModeProviderCursor,
		driver.ContinuationModeNativeSession,
		driver.ContinuationModeCompacted:
		return true
	default:
		return false
	}
}

func validContinuationOutcome(value string) bool {
	switch value {
	case continuationOutcomeReuse, continuationOutcomeFallback,
		continuationOutcomeFallbackExpired:
		return true
	default:
		return false
	}
}

func validContinuationSummary(
	value ContinuationSummary,
	events int64,
) bool {
	if value.Reused < 0 || value.Fallback < 0 || value.Expired < 0 ||
		value.Expired > value.Fallback || len(value.Counts) > 18 {
		return false
	}
	var reused, fallback, expired, observed int64
	previous := continuationKey{}
	for index, count := range value.Counts {
		key := continuationKey{mode: count.Mode, outcome: count.Outcome}
		if !validContinuationMode(key.mode) ||
			!validContinuationOutcome(key.outcome) || count.Count < 1 ||
			(index != 0 && !continuationKeyLess(previous, key)) {
			return false
		}
		var err error
		observed, err = telemetryAdd(observed, count.Count)
		if err != nil {
			return false
		}
		switch key.outcome {
		case continuationOutcomeReuse:
			reused, err = telemetryAdd(reused, count.Count)
		case continuationOutcomeFallback:
			fallback, err = telemetryAdd(fallback, count.Count)
		case continuationOutcomeFallbackExpired:
			fallback, err = telemetryAdd(fallback, count.Count)
			if err == nil {
				expired, err = telemetryAdd(expired, count.Count)
			}
		}
		if err != nil {
			return false
		}
		previous = key
	}
	return observed <= events &&
		reused == value.Reused &&
		fallback == value.Fallback &&
		expired == value.Expired
}

func continuationKeyLess(left, right continuationKey) bool {
	if left.mode != right.mode {
		return left.mode < right.mode
	}
	return left.outcome < right.outcome
}
