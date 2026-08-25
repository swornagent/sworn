package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	// DegradationParkEventVersion is the schema version of the typed
	// degradation park event body.
	DegradationParkEventVersion = "sworn.degradation-park/v1"
	// ParkEventVersion is the schema version of the new-cause typed park
	// event bodies (economy budgets and identical failures). Legacy
	// degradation bodies keep DegradationParkEventVersion forever and
	// re-encode byte-identically.
	ParkEventVersion = "sworn.park-event/v1"
	// DegradationUnblockKnob is the manifest knob that unblocks a
	// degradation park.
	DegradationUnblockKnob = "limits.degradation_budget"
	// EconomyTurnsUnblockKnob is the manifest knob that unblocks an
	// economy turn-budget park.
	EconomyTurnsUnblockKnob = "limits.max_turns_per_work"
	// EconomyOutputTokensUnblockKnob is the manifest knob that unblocks an
	// economy output-token-budget park.
	EconomyOutputTokensUnblockKnob = "limits.max_output_tokens_per_work"
	// IdenticalFailureUnblockKnob is the manifest knob that unblocks an
	// identical-failure park.
	IdenticalFailureUnblockKnob = "limits.identical_failure_park_after"
	// ParkEventKind is the journal event kind every typed park event is
	// written under. The label is historical: it entered the journal
	// vocabulary for the degradation park, and the waived cockpit webhook
	// kind map keys exactly this string (park_updated), so the new park
	// causes ride the same kind rather than demoting to run_updated. The
	// cause field inside the typed body is the authority on every surface;
	// readers must never infer the park class from this label.
	ParkEventKind = "degradation_budget_parked"

	// ParkCauseDegradation is the park cause for a run that crossed its
	// degradation budget on real, counted context loss.
	ParkCauseDegradation = "degradation"
	// ParkCauseAttention is the park cause for a run stopped on an open
	// attention turn.
	ParkCauseAttention = "attention"
	// ParkCauseExhaustion is the park cause for a run with failed work.
	ParkCauseExhaustion = "exhaustion"
	// ParkCauseHumanAuthority is the park cause for a run held on required
	// human authority.
	ParkCauseHumanAuthority = "human_authority"
	// ParkCauseEconomyTurns is the park cause for a work whose dispatch
	// crossed its per-work turn budget.
	ParkCauseEconomyTurns = "economy_turns"
	// ParkCauseEconomyOutputTokens is the park cause for a work whose
	// dispatch crossed its per-work output-token budget.
	ParkCauseEconomyOutputTokens = "economy_output_tokens"
	// ParkCauseIdenticalFailure is the park cause for a work with N
	// consecutive identical operational failures.
	ParkCauseIdenticalFailure = "identical_failure"
)

// DegradationFallback is one counted fallback fact inside a degradation park
// event: the journal offset and the reason the event body carried.
type DegradationFallback struct {
	Offset int64  `json:"offset"`
	Reason string `json:"reason"`
}

// DegradationParkEvent is the versioned, typed park event body. It carries
// only engine-owned facts. For the degradation cause it carries the counted
// fallbacks, the effective budget, and the manifest knob that unblocks the
// park; for the new park causes it carries the economy spent-versus-budget
// figures or the identical-failure run, threshold, code, and durable refusal
// detail. Every new field is additive and omitted when absent, so legacy
// degradation bodies re-encode byte-identically.
type DegradationParkEvent struct {
	SchemaVersion string                `json:"schema_version"`
	RunID         string                `json:"run_id"`
	Cause         string                `json:"cause"`
	Count         int64                 `json:"count,omitempty"`
	Budget        int64                 `json:"budget,omitempty"`
	UnblockKnob   string                `json:"unblock_knob"`
	Fallbacks     []DegradationFallback `json:"fallbacks,omitempty"`
	// Spent is the engine-counted spend at an economy crossing: turns for
	// cause economy_turns, output tokens for cause economy_output_tokens.
	// Budget names the effective knob value that dispatch crossed.
	Spent int64 `json:"spent,omitempty"`
	// Consecutive is the number of consecutive operational failures sharing
	// FailureCode; Threshold is the effective
	// limits.identical_failure_park_after knob value.
	Consecutive   int64  `json:"consecutive,omitempty"`
	Threshold     int64  `json:"threshold,omitempty"`
	FailureCode   string `json:"failure_code,omitempty"`
	FailureDetail string `json:"failure_detail,omitempty"`
}

// degradationFallbacks returns the degradation-gated fallback list. Counting
// is gated on real loss: a structured fallback fact counts only when the
// dispatch actually had a retained continuation to lose (retained=true) and
// the adapter declares context retention; an adapter whose declared posture
// makes fresh rehydration its ordinary operation accumulates zero budget
// from transport churn. Legacy bare-string bodies keep today's conservative
// counting so journals recorded before this slice evaluate unchanged.
func degradationFallbacks(snapshot journal.Snapshot) []DegradationFallback {
	var fallbacks []DegradationFallback
	for _, event := range snapshot.Events {
		if !strings.Contains(event.Kind, ".continuation.fresh_rehydrate.") {
			continue
		}
		var fact continuationFallbackEvent
		if json.Unmarshal(event.Body, &fact) == nil &&
			fact.SchemaVersion == continuationFallbackEventVersion {
			// Structured fact: apply the gated semantics. An unknown posture
			// fails closed as context_retaining; only an explicit
			// fresh_by_design declaration skips counting.
			if fact.Posture == string(driver.ContinuationPostureFreshByDesign) {
				continue
			}
			if !fact.Retained {
				continue
			}
			reason := fact.Reason
			if reason == "" {
				reason = "absence"
			}
			fallbacks = append(fallbacks, DegradationFallback{
				Offset: event.Offset,
				Reason: reason,
			})
			continue
		}
		// Legacy bare-string body: count exactly as before.
		reason := string(event.Body)
		if reason == "" {
			reason = "absence"
		}
		fallbacks = append(fallbacks, DegradationFallback{
			Offset: event.Offset,
			Reason: reason,
		})
	}
	return fallbacks
}

// canonicalDegradationParkEvent validates the engine-owned facts of a typed
// park event and returns its canonical encoding. The rules are cause-driven:
// degradation keeps the legacy schema version and the exact rules it has
// always had (a legacy degradation body re-encodes byte-identically), while
// the new causes ride the new schema version with their own honest-facts
// rules. The cause is the authority on every surface.
func canonicalDegradationParkEvent(event DegradationParkEvent) ([]byte, error) {
	if !runtimeIdentityPattern.MatchString(event.RunID) {
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	switch {
	case event.SchemaVersion == DegradationParkEventVersion &&
		event.Cause == ParkCauseDegradation:
		if event.Count < 1 || event.Budget < 1 ||
			event.Count <= event.Budget ||
			event.UnblockKnob != DegradationUnblockKnob ||
			int64(len(event.Fallbacks)) != event.Count ||
			event.Spent != 0 || event.Consecutive != 0 ||
			event.Threshold != 0 || event.FailureCode != "" ||
			event.FailureDetail != "" {
			return nil, runtimeFail("INVALID_PARK_EVENT", nil)
		}
		for _, fallback := range event.Fallbacks {
			if fallback.Offset < 1 || fallback.Reason == "" {
				return nil, runtimeFail("INVALID_PARK_EVENT", nil)
			}
		}
	case event.SchemaVersion == ParkEventVersion &&
		(event.Cause == ParkCauseEconomyTurns ||
			event.Cause == ParkCauseEconomyOutputTokens):
		knob := EconomyTurnsUnblockKnob
		if event.Cause == ParkCauseEconomyOutputTokens {
			knob = EconomyOutputTokensUnblockKnob
		}
		// A crossing means the engine-counted spend reached the effective
		// budget at the loop-top boundary; an event must name both facts.
		if event.Spent < 1 || event.Budget < 1 ||
			event.Spent < event.Budget ||
			event.UnblockKnob != knob ||
			event.Count != 0 || len(event.Fallbacks) != 0 ||
			event.Consecutive != 0 || event.Threshold != 0 ||
			event.FailureCode != "" || event.FailureDetail != "" {
			return nil, runtimeFail("INVALID_PARK_EVENT", nil)
		}
	case event.SchemaVersion == ParkEventVersion &&
		event.Cause == ParkCauseIdenticalFailure:
		if event.Consecutive < 1 || event.Threshold < 1 ||
			event.Consecutive < event.Threshold ||
			event.UnblockKnob != IdenticalFailureUnblockKnob ||
			event.FailureCode == "" ||
			!runtimeIdentityPattern.MatchString(event.FailureCode) ||
			!validParkDetail(event.FailureDetail) ||
			event.Count != 0 || event.Budget != 0 ||
			len(event.Fallbacks) != 0 || event.Spent != 0 {
			return nil, runtimeFail("INVALID_PARK_EVENT", nil)
		}
	default:
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	body, err := json.Marshal(event)
	if err != nil {
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	return body, nil
}

// validParkDetail bounds the durable refusal detail a park event may carry.
// Empty is honest absence; everything else must be bounded, valid UTF-8, and
// free of control characters, exactly like the refusal detail itself was
// bounded at extraction.
func validParkDetail(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 2_048 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func degradationParkEvent(
	runID string,
	budget int64,
	fallbacks []DegradationFallback,
) ([]byte, error) {
	return canonicalDegradationParkEvent(DegradationParkEvent{
		SchemaVersion: DegradationParkEventVersion,
		RunID:         runID,
		Cause:         ParkCauseDegradation,
		Count:         int64(len(fallbacks)),
		Budget:        budget,
		UnblockKnob:   DegradationUnblockKnob,
		Fallbacks:     fallbacks,
	})
}

// ParseDegradationParkEvent validates a typed park event body strictly:
// canonical encoding, closed fields, and honest facts, under either the
// legacy degradation schema version or the new-cause park-event version. It
// is the egress-side gate the cockpit uses before any park fact crosses the
// webhook boundary; unparsable bodies are rejected so only validated typed
// fields are emitted.
func ParseDegradationParkEvent(body []byte) (*DegradationParkEvent, error) {
	var event DegradationParkEvent
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	canonical, err := canonicalDegradationParkEvent(event)
	if err != nil || !bytes.Equal(canonical, body) {
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	return &event, nil
}
