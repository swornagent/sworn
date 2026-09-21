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
	// workerTurnEventKind names the durable event a native-lane worker
	// turn (S1-native-turn-journal) lands under: what the worker said and
	// which tools it called, one bounded record per turn. It is a new,
	// separately named kind; tool_result_observed and
	// sworn.tool-result-turn/v1 stay byte-compatible for existing readers.
	workerTurnEventKind = "worker_turn_observed"
	// workerTurnSchemaVersion versions the worker-turn event body, on the
	// identical base64/raw-byte-count discipline as the tool-result body.
	workerTurnSchemaVersion = "sworn.worker-turn/v1"
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

// workerTurnEventBody is the versioned, identity-carrying envelope the
// runtime writes around the driver's coalesced worker-turn projection, on
// the identical association-field shape as toolResultEventBody so a reader
// can join a worker turn to its tool results by identity and turn.
type workerTurnEventBody struct {
	SchemaVersion  string                  `json:"schema_version"`
	RunID          string                  `json:"run_id"`
	Track          string                  `json:"track,omitempty"`
	Slice          string                  `json:"slice,omitempty"`
	Role           driver.Role             `json:"role"`
	Responsibility driver.Responsibility   `json:"responsibility"`
	Attempt        int64                   `json:"attempt"`
	Epoch          int64                   `json:"epoch"`
	Try            int64                   `json:"try"`
	WorkID         string                  `json:"work_id,omitempty"`
	EffectID       string                  `json:"effect_id,omitempty"`
	Turn           int64                   `json:"turn"`
	Part           int64                   `json:"part,omitempty"`
	Parts          int64                   `json:"parts,omitempty"`
	DroppedEvents  int64                   `json:"dropped_events,omitempty"`
	Encoding       string                  `json:"encoding"`
	Content        []driver.WorkerTurnPart `json:"content"`
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
			Attempt:        coordinates.ProtocolAttempt,
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
		now := s.now().UTC()
		offset, err := s.journal.AppendEventWithOffset(ctx, owner.RunID, toolResultEventKind, encoded, now)
		if err != nil {
			return err
		}
		// S2 live ring: feed only after the durable append and carry the
		// durable offset, so the ring and the journal share one cursor.
		s.observeActivityTap(ActivityTapEvent{RunID: owner.RunID, EffectID: body.EffectID, Offset: offset, Kind: toolResultEventKind, Body: encoded, CreatedAt: now})
		// S3 failure tail: feed only after the durable append, so the
		// tail holds only durably journaled projections. It never fails
		// the hook: the journal already holds the turn.
		s.feedFailureTailTool(body.EffectID, turn)
		return nil
	}
}

// workerTurnObservationHook builds the durable hook for one dispatch's
// worker-turn projection, on the identical construction as
// toolResultObservationHook: the dispatch's identity is captured once and
// each projected turn is written through the same generic events(kind,
// body) journal machinery - no schema migration, no second write path.
func (s *Service) workerTurnObservationHook(
	owner journal.OwnerLease,
	prepared preparedDriverDispatch,
	coordinates dispatchCoordinates,
	attemptIdentity journal.EffectAttempt,
) driver.WorkerTurnHook {
	if s == nil || s.journal == nil || owner.RunID == "" {
		return nil
	}
	track := ""
	if prepared.productionContext != nil {
		track = prepared.productionContext.Track
	}
	return func(ctx context.Context, turn driver.WorkerTurn) error {
		if ctx == nil {
			ctx = context.Background()
		}
		body := workerTurnEventBody{
			SchemaVersion:  workerTurnSchemaVersion,
			RunID:          owner.RunID,
			Track:          track,
			Slice:          coordinates.Slice,
			Role:           prepared.request.Role,
			Responsibility: coordinates.Responsibility,
			Attempt:        coordinates.ProtocolAttempt,
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
			Content:       turn.Content,
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			// Marshal of this fixed shape cannot fail today; keep the
			// failure loud on the observation side instead of silent.
			return runtimeFail("WORKER_TURN_OBSERVATION_FAILED", err)
		}
		now := s.now().UTC()
		offset, err := s.journal.AppendEventWithOffset(ctx, owner.RunID, workerTurnEventKind, encoded, now)
		if err != nil {
			return err
		}
		// S2 live ring: feed only after the durable append and carry the
		// durable offset, so the ring and the journal share one cursor.
		s.observeActivityTap(ActivityTapEvent{RunID: owner.RunID, EffectID: body.EffectID, Offset: offset, Kind: workerTurnEventKind, Body: encoded, CreatedAt: now})
		// S3 failure tail: feed only after the durable append, on the
		// identical discipline as the tool-result hook.
		s.feedFailureTailWorker(body.EffectID, turn)
		return nil
	}
}
