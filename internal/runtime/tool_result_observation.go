package runtime

import (
	"context"
	"encoding/json"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	// toolResultEventKind names the durable event every emitted
	// tool-result projection lands under. The events are additive: the
	// eval observer and the cockpit classify by kind and ignore kinds
	// they do not own.
	toolResultEventKind = "tool_result_observed"
	// toolResultTurnSchemaVersion versions the event body. Head and tail
	// are standard RFC 4648 padded base64 of the exact post-redaction
	// byte spans; all counts are on raw bytes.
	toolResultTurnSchemaVersion = "sworn.tool-result-turn/v1"
)

// toolResultEventBody is the versioned, identity-carrying envelope the
// runtime writes around the driver's coalesced turn projection. Every event
// names its run, track, slice, role, responsibility, attempt, epoch, try,
// work, effect, and turn, plus its part geometry and the observer's loud
// drop count — sufficient to derive a per-track stream and to bind a cursor
// to a position through the existing events and observer_cursors machinery.
type toolResultEventBody struct {
	SchemaVersion  string                    `json:"schema_version"`
	RunID          string                    `json:"run_id"`
	Track          string                    `json:"track,omitempty"`
	Slice          string                    `json:"slice,omitempty"`
	Role           driver.Role               `json:"role"`
	Responsibility driver.Responsibility     `json:"responsibility"`
	Attempt        int64                     `json:"attempt"`
	Epoch          int64                     `json:"epoch"`
	Try            int64                     `json:"try"`
	WorkID         string                    `json:"work_id,omitempty"`
	EffectID       string                    `json:"effect_id,omitempty"`
	Turn           int64                     `json:"turn"`
	Part           int64                     `json:"part,omitempty"`
	Parts          int64                     `json:"parts,omitempty"`
	DroppedEvents  int64                     `json:"dropped_events,omitempty"`
	Encoding       string                    `json:"encoding"`
	Results        []driver.ToolResultRecord `json:"results"`
}

// toolResultObservationHook builds the durable hook for one dispatch. It
// captures the dispatch's identity once — run, track, slice, role,
// responsibility, attempt, epoch, try, work, and effect — and writes each
// projected turn through the existing journal event machinery. The track is
// honestly empty when the dispatch carries no production context (e.g.
// planner-level dispatches). A nil journal disables observation.
func (s *Service) toolResultObservationHook(
	owner journal.OwnerLease,
	prepared preparedDriverDispatch,
	coordinates dispatchCoordinates,
	attemptIdentity journal.EffectAttempt,
) driver.ToolResultHook {
	if s == nil || s.journal == nil || owner.RunID == "" {
		return nil
	}
	track := ""
	if prepared.productionContext != nil {
		track = prepared.productionContext.Track
	}
	return func(ctx context.Context, turn driver.ToolResultTurn) error {
		if ctx == nil {
			ctx = context.Background()
		}
		body := toolResultEventBody{
			SchemaVersion:  toolResultTurnSchemaVersion,
			RunID:          owner.RunID,
			Track:          track,
			Slice:          coordinates.Slice,
			Role:           prepared.request.Role,
			Responsibility: coordinates.Responsibility,
			Attempt:        coordinates.BatonAttempt,
			Epoch:          coordinates.Epoch,
			Try:            coordinates.Try,
			WorkID:         attemptIdentity.WorkID,
			EffectID: journal.AttemptEffectID(
				attemptIdentity.WorkID,
				attemptIdentity.Epoch,
				attemptIdentity.Try,
			),
			Turn:          turn.Turn,
			Part:          turn.Part,
			Parts:         turn.Parts,
			DroppedEvents: turn.DroppedEvents,
			Encoding:      "base64",
			Results:       turn.Results,
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			// Marshal of this fixed shape cannot fail today; keep the
			// failure loud on the observation side instead of silent.
			return runtimeFail("TOOL_RESULT_OBSERVATION_FAILED", err)
		}
		return s.journal.AppendEvent(
			ctx,
			owner.RunID,
			toolResultEventKind,
			encoded,
			s.now().UTC(),
		)
	}
}
