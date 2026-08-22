package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	// DegradationParkEventVersion is the schema version of the typed
	// degradation park event body.
	DegradationParkEventVersion = "sworn.degradation-park/v1"
	// DegradationUnblockKnob is the manifest knob that unblocks a
	// degradation park.
	DegradationUnblockKnob = "limits.degradation_budget"

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
)

// DegradationFallback is one counted fallback fact inside a degradation park
// event: the journal offset and the reason the event body carried.
type DegradationFallback struct {
	Offset int64  `json:"offset"`
	Reason string `json:"reason"`
}

// DegradationParkEvent is the versioned, typed park event body. It carries
// only engine-owned facts: the counted fallbacks, the effective budget, and
// the manifest knob that unblocks the park.
type DegradationParkEvent struct {
	SchemaVersion string                `json:"schema_version"`
	RunID         string                `json:"run_id"`
	Cause         string                `json:"cause"`
	Count         int64                 `json:"count"`
	Budget        int64                 `json:"budget"`
	UnblockKnob   string                `json:"unblock_knob"`
	Fallbacks     []DegradationFallback `json:"fallbacks"`
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

// canonicalDegradationParkEvent validates the engine-owned facts of a
// degradation park event and returns its canonical encoding. A park event is
// only written when the gated count crossed the effective budget, so both
// facts must hold.
func canonicalDegradationParkEvent(event DegradationParkEvent) ([]byte, error) {
	if event.SchemaVersion != DegradationParkEventVersion ||
		!runtimeIdentityPattern.MatchString(event.RunID) ||
		event.Cause != ParkCauseDegradation ||
		event.Count < 1 || event.Budget < 1 ||
		event.Count <= event.Budget ||
		event.UnblockKnob != DegradationUnblockKnob ||
		int64(len(event.Fallbacks)) != event.Count {
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	for _, fallback := range event.Fallbacks {
		if fallback.Offset < 1 || fallback.Reason == "" {
			return nil, runtimeFail("INVALID_PARK_EVENT", nil)
		}
	}
	body, err := json.Marshal(event)
	if err != nil {
		return nil, runtimeFail("INVALID_PARK_EVENT", nil)
	}
	return body, nil
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

// ParseDegradationParkEvent validates a degradation park event body strictly:
// canonical encoding, closed fields, and honest facts. It is the egress-side
// gate the cockpit uses before any park fact crosses the webhook boundary;
// unparsable bodies are rejected so only validated typed fields are emitted.
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
