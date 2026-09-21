package runtime

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// Failure-turn context (S3-failure-turn-context): a bounded tail of the
// worker's last turns attached to every operational dispatch failure in the
// same journal transaction as the failure, and shown in the shared status
// projection beside the failure code and detail.
//
// The journal ride is the failure/uncertain event body, not the digested
// observation body and not a new table. CompleteOwned and ReconcileOwned
// both append their event atomically with the state transition in one
// SQLite immediate transaction, so attaching to EventBody satisfies "same
// journal transaction" for both dispatch_operational_failure and
// dispatch_uncertain with one envelope and no second event.
//
// The tail is runtime-owned, always on (not an optional tap), keyed by
// effectID (attempt/<work>/eN/tM, distinct per try so no stale leak). It
// holds copies of the already-bounded/redacted Turn structs the S1 hooks
// already received (no re-projection, no re-redaction, no raw bytes),
// fed after the durable AppendEventWithOffset succeeds, never before, so
// the tail holds only durably journaled projections.
//
// Status decodes head/tail once with U+FFFD replacement; counts stay
// authoritative; omitted bytes are never reassembled. The decoded shape is
// what the board, TUI, and future typed explain read share.
const (
	// FailureTurnContextSchemaVersion versions the failure-turn-context
	// shape, journaled and served.
	FailureTurnContextSchemaVersion = "sworn.failure-turn-context/v1"
	// FailureTurnContextMaxTurns bounds one context to the newest N
	// turn-events. Selection walks the tail newest-first; emission is
	// oldest-first (turn numbers increasing) so a reader sees the final
	// turns in order.
	FailureTurnContextMaxTurns = 5
	// FailureTurnContextMaxBytes bounds one context's compact JSON to
	// 48 KiB. Association overhead is ~1 KiB, so the full event body
	// stays far under journal.MaxEventBytes (256 KiB) with exact
	// accounting.
	FailureTurnContextMaxBytes = 48 * 1024
	// FailureTailMaxEvents bounds one live dispatch's in-memory tail in
	// turn-projection events. The tail is larger than the context bound
	// so context selection never loses beyond declared omission.
	FailureTailMaxEvents = 16
	// FailureTailMaxBytes bounds one live dispatch's tail in stored
	// JSON bytes. Like the S2 activity ring, the newest event is always
	// admitted whole even when it alone exceeds the byte bound (still
	// bounded by the driver's 240 KiB event budget); the bound is
	// enforced by evicting oldest while over.
	FailureTailMaxBytes = 128 * 1024
)

const (
	// FailureTurnKindToolResult names a tool-result turn-event in a
	// failure context. The journal kind is tool_result_observed; the
	// context kind is the short name.
	FailureTurnKindToolResult = "tool_result"
	// FailureTurnKindWorker names a native worker-turn turn-event.
	FailureTurnKindWorker = "worker_turn"
)

const (
	// FailureTurnContextPresent means the dispatch emitted turns and the
	// context carries the newest that fit the declared bounds.
	FailureTurnContextPresent = "present"
	// FailureTurnContextEmpty means the dispatch failed before any turn
	// was observed (failed before first turn, hooks disabled,
	// automation). It is explicit turns:[] with omitted 0, never an
	// absent field. Empty only ever means "no turns observed": a
	// dispatch that took turns always carries at least its newest turn,
	// truncated by whole parts when it alone exceeds the byte bound
	// (C2), never an empty list with omitted > 0.
	FailureTurnContextEmpty = "empty"
	// FailureTurnContextUnavailable means the tail could not be produced
	// but the failure is still journaled exactly as today, with the
	// reason named. Assembling context never turns one failure into
	// another.
	FailureTurnContextUnavailable = "unavailable"
)

const (
	// FailureTurnContextNoLiveTail names an unavailable context for a
	// write that structurally has no live tail: a prior-process or
	// ownerless recovery path that never held this dispatch's turns.
	FailureTurnContextNoLiveTail = "no_live_tail"
	// FailureTurnContextTailEncodeFailed names an infallible-marshal
	// failure, kept loud rather than silent. Unreachable today.
	FailureTurnContextTailEncodeFailed = "tail_encode_failed"
	// FailureTurnContextOverBudget names a defensive over-budget
	// assembly, unreachable today (48 KiB << 256 KiB with exact
	// accounting).
	FailureTurnContextOverBudget = "context_over_budget"
)

// FailureToolResult is one bounded, decoded tool result in a served
// failure context. Head and Tail are decoded display strings (base64
// strict-decoded with U+FFFD replacement for non-UTF-8 bytes); an
// undecodable span degrades to an empty string with its counts preserved,
// never to a dropped turn. TotalBytes, OmittedBytes and RedactedBytes stay
// the authoritative raw-byte counts.
type FailureToolResult struct {
	Sequence      int64  `json:"sequence"`
	ToolCallID    string `json:"tool_call_id"`
	Tool          string `json:"tool"`
	Failed        bool   `json:"failed"`
	TotalBytes    int64  `json:"total_bytes"`
	OmittedBytes  int64  `json:"omitted_bytes"`
	RedactedBytes int64  `json:"redacted_bytes"`
	Head          string `json:"head"`
	Tail          string `json:"tail"`
}

// FailureWorkerPart is one bounded, decoded content block of a worker turn
// in a served failure context, on the same display discipline as
// FailureToolResult.
type FailureWorkerPart struct {
	Kind          string `json:"kind"`
	Tool          string `json:"tool,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	TotalBytes    int64  `json:"total_bytes"`
	OmittedBytes  int64  `json:"omitted_bytes"`
	RedactedBytes int64  `json:"redacted_bytes"`
	Head          string `json:"head"`
	Tail          string `json:"tail"`
}

// FailureTurn is one turn-event in a served failure context: either a
// tool-result turn (Results set, Content empty) or a native worker turn
// (Content set, Results empty). Kind is tool_result or worker_turn. Part
// and Parts carry the driver's split geometry (0 means a single part).
// DroppedEvents is that turn's own observer count at acceptance time.
// OmittedParts counts whole records/parts dropped from this turn alone to
// fit the context byte bound (C2); it is 0 for every turn except possibly
// the newest admitted one, and whole-part drops keep S1's per-part
// geometry intact.
type FailureTurn struct {
	Kind          string              `json:"kind"`
	Turn          int64               `json:"turn"`
	Part          int64               `json:"part,omitempty"`
	Parts         int64               `json:"parts,omitempty"`
	DroppedEvents int64               `json:"dropped_events,omitempty"`
	Results       []FailureToolResult `json:"results,omitempty"`
	Content       []FailureWorkerPart `json:"content,omitempty"`
	OmittedParts  int64               `json:"omitted_parts,omitempty"`
}

// FailureTurnContext is the one bounded status shape reused on Effect,
// PinnedWork and Node (one shape, one JSON key, no new surface). Turns are
// oldest-first (turn numbers increasing). Omitted counts earlier
// journaled turn-events not carried due to bounds (tail evictions plus
// context selection); it is exact. DroppedMaxVisible is the maximum
// observer drop count visible in the retained tail (the newest entry's
// value, since each observer's count is monotonic); it is max-visible,
// not an exact total: a malformed/overflow drop immediately before
// failure with no later success never rides onto a journaled turn and is
// invisible without new driver transport (the S1 journal itself is
// exactly as honest). Reason names why an unavailable context has no
// turns.
type FailureTurnContext struct {
	SchemaVersion     string        `json:"schema_version"`
	Status            string        `json:"status"`
	Turns             []FailureTurn `json:"turns"`
	Omitted           int64         `json:"omitted,omitempty"`
	DroppedMaxVisible int64         `json:"dropped_max_visible,omitempty"`
	Reason            string        `json:"reason,omitempty"`
}

// failureTurnStored is the journaled turn-event: the same bounded
// redacted projection S1 journaled (base64 head/tail, raw-byte counts),
// with the context kind and the C2 within-turn truncation count.
type failureTurnStored struct {
	Kind          string                    `json:"kind"`
	Turn          int64                     `json:"turn"`
	Part          int64                     `json:"part,omitempty"`
	Parts         int64                     `json:"parts,omitempty"`
	DroppedEvents int64                     `json:"dropped_events,omitempty"`
	Results       []driver.ToolResultRecord `json:"results,omitempty"`
	Content       []driver.WorkerTurnPart   `json:"content,omitempty"`
	OmittedParts  int64                     `json:"omitted_parts,omitempty"`
}

// failureContextStored is the journaled failure-turn-context value carried
// on the failure/uncertain event body beside the association.
type failureContextStored struct {
	SchemaVersion     string              `json:"schema_version"`
	Status            string              `json:"status"`
	Turns             []failureTurnStored `json:"turns"`
	Omitted           int64               `json:"omitted,omitempty"`
	DroppedMaxVisible int64               `json:"dropped_max_visible,omitempty"`
	Reason            string              `json:"reason,omitempty"`
}

// failureEventBody is the versioned failure envelope for non-continuation
// kinds: the standard association fields stay present, with the context
// carried additively beside them. Old bodies without the new key decode
// with a nil context (absent, not corrupt).
type failureEventBody struct {
	EventAssociation
	FailureTurnContext *failureContextStored `json:"failure_turn_context,omitempty"`
}

// failureTailEntry holds one durably journaled turn-event's already
// bounded/redacted projection as deep copies of the Turn structs the S1
// hooks received. Exactly one of Tool and Worker is set. Size is the
// compact-JSON size of the stored turn as it would ride the context, used
// for exact byte accounting.
type failureTailEntry struct {
	Kind   string
	Tool   *driver.ToolResultTurn
	Worker *driver.WorkerTurn
	Size   int64
}

// failureTail is one live dispatch's bounded in-memory tail.
type failureTail struct {
	entries []failureTailEntry
	bytes   int64
	omitted int64
}

// initFailureTail eagerly creates one dispatch's empty tail at dispatch
// start, so a failure before any turn reads back as explicit empty rather
// than unavailable. It never fails the dispatch.
func (s *Service) initFailureTail(effectID string) {
	if s == nil || effectID == "" {
		return
	}
	defer func() { _ = recover() }()
	s.failureMu.Lock()
	defer s.failureMu.Unlock()
	if s.failureTails == nil {
		s.failureTails = make(map[string]*failureTail)
	}
	if _, exists := s.failureTails[effectID]; !exists {
		s.failureTails[effectID] = &failureTail{}
	}
}

// dropFailureTail forgets one dispatch's tail when the dispatch ends,
// whether it succeeds, fails, or parks. The journal already holds every
// projection the tail ever held. It never fails the dispatch.
func (s *Service) dropFailureTail(effectID string) {
	if s == nil || effectID == "" {
		return
	}
	defer func() { _ = recover() }()
	s.failureMu.Lock()
	defer s.failureMu.Unlock()
	delete(s.failureTails, effectID)
}

// feedFailureTailTool appends one durably journaled tool-result turn to
// the dispatch's tail. It runs only after AppendEventWithOffset succeeds,
// holds only deep copies of the already-bounded/redacted projection, and
// never blocks, fails, or alters the dispatch.
func (s *Service) feedFailureTailTool(effectID string, turn driver.ToolResultTurn) {
	if s == nil || effectID == "" {
		return
	}
	defer func() { _ = recover() }()
	copied := driver.ToolResultTurn{
		Turn:          turn.Turn,
		Part:          turn.Part,
		Parts:         turn.Parts,
		DroppedEvents: turn.DroppedEvents,
		Results:       append([]driver.ToolResultRecord(nil), turn.Results...),
	}
	stored := failureTurnStored{
		Kind:          FailureTurnKindToolResult,
		Turn:          copied.Turn,
		Part:          copied.Part,
		Parts:         copied.Parts,
		DroppedEvents: copied.DroppedEvents,
		Results:       copied.Results,
	}
	size := int64(len(mustJSON(stored)))
	s.failureMu.Lock()
	defer s.failureMu.Unlock()
	if s.failureTails == nil {
		s.failureTails = make(map[string]*failureTail)
	}
	tail := s.failureTails[effectID]
	if tail == nil {
		tail = &failureTail{}
		s.failureTails[effectID] = tail
	}
	// Newest is always admitted whole (ActivityRing precedent); the byte
	// bound is enforced by evicting oldest while over.
	for len(tail.entries) >= FailureTailMaxEvents ||
		(tail.bytes+size > FailureTailMaxBytes && len(tail.entries) > 0) {
		tail.bytes -= tail.entries[0].Size
		if tail.bytes < 0 {
			tail.bytes = 0
		}
		copy(tail.entries, tail.entries[1:])
		tail.entries[len(tail.entries)-1] = failureTailEntry{}
		tail.entries = tail.entries[:len(tail.entries)-1]
		tail.omitted++
	}
	tail.entries = append(tail.entries, failureTailEntry{
		Kind: FailureTurnKindToolResult, Tool: &copied, Size: size,
	})
	tail.bytes += size
}

// feedFailureTailWorker appends one durably journaled worker turn, on the
// identical discipline as feedFailureTailTool.
func (s *Service) feedFailureTailWorker(effectID string, turn driver.WorkerTurn) {
	if s == nil || effectID == "" {
		return
	}
	defer func() { _ = recover() }()
	copied := driver.WorkerTurn{
		Turn:          turn.Turn,
		Part:          turn.Part,
		Parts:         turn.Parts,
		DroppedEvents: turn.DroppedEvents,
		Content:       append([]driver.WorkerTurnPart(nil), turn.Content...),
	}
	stored := failureTurnStored{
		Kind:          FailureTurnKindWorker,
		Turn:          copied.Turn,
		Part:          copied.Part,
		Parts:         copied.Parts,
		DroppedEvents: copied.DroppedEvents,
		Content:       copied.Content,
	}
	size := int64(len(mustJSON(stored)))
	s.failureMu.Lock()
	defer s.failureMu.Unlock()
	if s.failureTails == nil {
		s.failureTails = make(map[string]*failureTail)
	}
	tail := s.failureTails[effectID]
	if tail == nil {
		tail = &failureTail{}
		s.failureTails[effectID] = tail
	}
	for len(tail.entries) >= FailureTailMaxEvents ||
		(tail.bytes+size > FailureTailMaxBytes && len(tail.entries) > 0) {
		tail.bytes -= tail.entries[0].Size
		if tail.bytes < 0 {
			tail.bytes = 0
		}
		copy(tail.entries, tail.entries[1:])
		tail.entries[len(tail.entries)-1] = failureTailEntry{}
		tail.entries = tail.entries[:len(tail.entries)-1]
		tail.omitted++
	}
	tail.entries = append(tail.entries, failureTailEntry{
		Kind: FailureTurnKindWorker, Worker: &copied, Size: size,
	})
	tail.bytes += size
}

// assembleFailureContextStored builds the journaled context for one
// dispatch from its live tail: present with the newest turns that fit,
// explicit empty when no turn was observed, unavailable with a named
// reason when no live tail exists or assembly itself fails. It never
// returns nil for a new write (absent is reserved for pre-S3 records and
// sweep reconciles that never held the dispatch), never fails, and never
// reads the provider stream or the journal.
func (s *Service) assembleFailureContextStored(effectID string) (stored *failureContextStored) {
	stored = &failureContextStored{
		SchemaVersion: FailureTurnContextSchemaVersion,
		Status:        FailureTurnContextUnavailable,
		Turns:         []failureTurnStored{},
		Reason:        FailureTurnContextNoLiveTail,
	}
	if s == nil || effectID == "" {
		return stored
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			stored = &failureContextStored{
				SchemaVersion: FailureTurnContextSchemaVersion,
				Status:        FailureTurnContextUnavailable,
				Turns:         []failureTurnStored{},
				Reason:        FailureTurnContextTailEncodeFailed,
			}
		}
	}()
	s.failureMu.Lock()
	tail := s.failureTails[effectID]
	var entries []failureTailEntry
	var tailOmitted int64
	var droppedMax int64
	if tail != nil {
		entries = append([]failureTailEntry(nil), tail.entries...)
		tailOmitted = tail.omitted
		for _, entry := range entries {
			var dropped int64
			if entry.Tool != nil {
				dropped = entry.Tool.DroppedEvents
			} else if entry.Worker != nil {
				dropped = entry.Worker.DroppedEvents
			}
			if dropped > droppedMax {
				droppedMax = dropped
			}
		}
	}
	s.failureMu.Unlock()
	if tail == nil {
		return stored
	}
	if len(entries) == 0 {
		return &failureContextStored{
			SchemaVersion:     FailureTurnContextSchemaVersion,
			Status:            FailureTurnContextEmpty,
			Turns:             []failureTurnStored{},
			DroppedMaxVisible: droppedMax,
		}
	}
	// Walk newest-first, accumulating exact compact-JSON sizes, stopping
	// before exceeding the context byte bound or the turn bound. The
	// newest turn is always admitted (C2): when it alone exceeds the
	// bound it is truncated by whole parts (keeping S1's per-part
	// geometry intact, latest parts kept as closest to the failure),
	// with the dropped count carried loudly on the turn. Empty therefore
	// only ever means "no turns observed".
	//
	// The envelope overhead reserves the omitted field at its widest
	// (10 digits), so the per-turn budget already leaves room for the
	// final omitted count: selection never admits a turn that the final
	// marshal would then push over budget. The final marshal below is
	// still the exact gate.
	overhead := int64(len(mustJSON(failureContextStored{
		SchemaVersion:     FailureTurnContextSchemaVersion,
		Status:            FailureTurnContextPresent,
		Turns:             []failureTurnStored{},
		Omitted:           9999999999,
		DroppedMaxVisible: droppedMax,
	})))
	// The turns array itself costs 2 (brackets); each additional turn
	// costs 1 (comma) plus its marshaled length. Overhead above already
	// includes the empty array; adjust per admitted turn below.
	const arrayBrackets = 2
	_ = arrayBrackets
	budget := int64(FailureTurnContextMaxBytes)
	// Reserve the envelope overhead; turns share the remainder.
	remaining := budget - overhead
	if remaining < 0 {
		remaining = 0
	}
	var selected []failureTurnStored
	var omittedTurns int64
	// Newest-first selection; emission below reverses to oldest-first.
	for index := len(entries) - 1; index >= 0; index-- {
		if int64(len(selected)) >= FailureTurnContextMaxTurns {
			omittedTurns += int64(index + 1)
			break
		}
		entry := entries[index]
		full := storedTurnForEntry(entry)
		cost := int64(1 + len(mustJSON(full)))
		if len(selected) == 0 {
			cost = int64(len(mustJSON(full)))
		}
		// Account for the comma before every turn except the first
		// admitted in emission order; newest-first accumulation uses
		// the same per-turn cost (1 for comma when not first).
		if len(selected) > 0 {
			// Already counted 1 above for full; keep.
		} else {
			// First admitted turn costs no leading comma.
			cost = int64(len(mustJSON(full)))
		}
		if cost <= remaining {
			selected = append(selected, full)
			remaining -= cost
			continue
		}
		if len(selected) == 0 {
			// The newest turn alone exceeds the bound: truncate it by
			// whole parts, latest kept, dropped counted (C2).
			truncated, droppedParts, ok := truncateStoredTurn(full, remaining)
			if !ok {
				return &failureContextStored{
					SchemaVersion: FailureTurnContextSchemaVersion,
					Status:        FailureTurnContextUnavailable,
					Turns:         []failureTurnStored{},
					Reason:        FailureTurnContextOverBudget,
				}
			}
			truncated.OmittedParts = droppedParts
			selected = append(selected, truncated)
			omittedTurns += int64(index)
			break
		}
		omittedTurns += int64(index + 1)
		break
	}
	// Reverse to oldest-first (turn numbers increasing) for emission.
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	omitted := tailOmitted + omittedTurns
	result := &failureContextStored{
		SchemaVersion:     FailureTurnContextSchemaVersion,
		Status:            FailureTurnContextPresent,
		Turns:             selected,
		Omitted:           omitted,
		DroppedMaxVisible: droppedMax,
	}
	// Defensive exact check: the marshaled context must fit the declared
	// bound, or the write stays unavailable rather than over-budget.
	body, err := json.Marshal(result)
	if err != nil {
		return &failureContextStored{
			SchemaVersion: FailureTurnContextSchemaVersion,
			Status:        FailureTurnContextUnavailable,
			Turns:         []failureTurnStored{},
			Reason:        FailureTurnContextTailEncodeFailed,
		}
	}
	if len(body) > FailureTurnContextMaxBytes {
		return &failureContextStored{
			SchemaVersion: FailureTurnContextSchemaVersion,
			Status:        FailureTurnContextUnavailable,
			Turns:         []failureTurnStored{},
			Reason:        FailureTurnContextOverBudget,
		}
	}
	return result
}

// storedTurnForEntry projects one tail entry to its stored turn shape.
func storedTurnForEntry(entry failureTailEntry) failureTurnStored {
	if entry.Tool != nil {
		turn := entry.Tool
		return failureTurnStored{
			Kind:          FailureTurnKindToolResult,
			Turn:          turn.Turn,
			Part:          turn.Part,
			Parts:         turn.Parts,
			DroppedEvents: turn.DroppedEvents,
			Results:       append([]driver.ToolResultRecord(nil), turn.Results...),
		}
	}
	if entry.Worker != nil {
		turn := entry.Worker
		return failureTurnStored{
			Kind:          FailureTurnKindWorker,
			Turn:          turn.Turn,
			Part:          turn.Part,
			Parts:         turn.Parts,
			DroppedEvents: turn.DroppedEvents,
			Content:       append([]driver.WorkerTurnPart(nil), turn.Content...),
		}
	}
	return failureTurnStored{Kind: entry.Kind}
}

// truncateStoredTurn drops whole records/parts from one oversize newest
// turn until it fits the remaining budget, keeping the latest (closest to
// the failure) and counting every dropped whole part. It keeps S1's
// per-part geometry intact: no head/tail span is ever split. Size checks
// include the carried omitted_parts count itself, so the admitted turn
// fits exactly as marshaled. It returns the truncated turn, the dropped
// count, and whether at least the turn shell (no records) fits.
func truncateStoredTurn(full failureTurnStored, remaining int64) (failureTurnStored, int64, bool) {
	shell := full
	shell.Results = nil
	shell.Content = nil
	shell.OmittedParts = 0
	if int64(len(mustJSON(shell))) > remaining {
		return failureTurnStored{}, 0, false
	}
	if len(full.Results) > 0 {
		original := len(full.Results)
		// Keep the latest suffix that fits, with its dropped count.
		for keep := original; keep > 0; keep-- {
			candidate := shell
			candidate.Results = append([]driver.ToolResultRecord(nil), full.Results[original-keep:]...)
			candidate.OmittedParts = int64(original - keep)
			if int64(len(mustJSON(candidate))) <= remaining {
				return candidate, int64(original - keep), true
			}
		}
		shell.OmittedParts = int64(original)
		if int64(len(mustJSON(shell))) > remaining {
			return failureTurnStored{}, 0, false
		}
		return shell, int64(original), true
	}
	if len(full.Content) > 0 {
		original := len(full.Content)
		for keep := original; keep > 0; keep-- {
			candidate := shell
			candidate.Content = append([]driver.WorkerTurnPart(nil), full.Content[original-keep:]...)
			candidate.OmittedParts = int64(original - keep)
			if int64(len(mustJSON(candidate))) <= remaining {
				return candidate, int64(original - keep), true
			}
		}
		shell.OmittedParts = int64(original)
		if int64(len(mustJSON(shell))) > remaining {
			return failureTurnStored{}, 0, false
		}
		return shell, int64(original), true
	}
	return shell, 0, true
}

// failureEventBodyFor assembles the event body for one failure/uncertain
// write: the standard association plus the bounded context, or the
// continuation-fallback body with the same additive context when the
// dispatch fell back. It never returns nil and never fails the dispatch:
// on any assembly or marshal error it carries unavailable with a named
// reason instead of failing.
func (s *Service) failureEventBodyFor(assoc EventAssociation, fact *continuationDispatchFact, effectID string) []byte {
	fallback := func() []byte {
		body, _ := json.Marshal(failureEventBody{
			EventAssociation: assoc,
			FailureTurnContext: &failureContextStored{
				SchemaVersion: FailureTurnContextSchemaVersion,
				Status:        FailureTurnContextUnavailable,
				Turns:         []failureTurnStored{},
				Reason:        FailureTurnContextTailEncodeFailed,
			},
		})
		if len(body) == 0 {
			return MarshalAssociation(assoc)
		}
		return body
	}
	defer func() {
		_ = recover()
	}()
	stored := s.assembleFailureContextStored(effectID)
	if stored == nil {
		return fallback()
	}
	if fact != nil &&
		fact.mode == driver.ContinuationModeFreshRehydrate &&
		(fact.outcome == continuationOutcomeFallback ||
			fact.outcome == continuationOutcomeFallbackExpired) {
		reason := fact.reason
		if reason == "" {
			reason = "absence"
		}
		body, err := json.Marshal(continuationFallbackEvent{
			SchemaVersion:      continuationFallbackEventVersion,
			EventAssociation:   assoc,
			Reason:             reason,
			Retained:           fact.retained,
			Posture:            string(fact.posture),
			FailureTurnContext: stored,
		})
		if err != nil || len(body) == 0 {
			return fallback()
		}
		return body
	}
	body, err := json.Marshal(failureEventBody{
		EventAssociation:   assoc,
		FailureTurnContext: stored,
	})
	if err != nil || len(body) == 0 {
		return fallback()
	}
	return body
}

// decodeFailureSpan decodes one base64 head/tail span for display with
// explicit U+FFFD replacement. An undecodable span degrades to an empty
// string with its counts preserved, never to a dropped turn.
func decodeFailureSpan(encoded string) string {
	if encoded == "" {
		return ""
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return ""
	}
	return strings.ToValidUTF8(string(raw), "\uFFFD")
}

// decodeFailureContextStored projects one journaled context to its served
// shape, decoding every head/tail once. Counts stay authoritative.
func decodeFailureContextStored(stored *failureContextStored) *FailureTurnContext {
	if stored == nil {
		return nil
	}
	if stored.SchemaVersion != FailureTurnContextSchemaVersion {
		return nil
	}
	switch stored.Status {
	case FailureTurnContextPresent, FailureTurnContextEmpty, FailureTurnContextUnavailable:
	default:
		return nil
	}
	result := &FailureTurnContext{
		SchemaVersion:     FailureTurnContextSchemaVersion,
		Status:            stored.Status,
		Turns:             make([]FailureTurn, 0, len(stored.Turns)),
		Omitted:           stored.Omitted,
		DroppedMaxVisible: stored.DroppedMaxVisible,
		Reason:            stored.Reason,
	}
	for _, turn := range stored.Turns {
		decoded := FailureTurn{
			Kind:          turn.Kind,
			Turn:          turn.Turn,
			Part:          turn.Part,
			Parts:         turn.Parts,
			DroppedEvents: turn.DroppedEvents,
			OmittedParts:  turn.OmittedParts,
		}
		for _, record := range turn.Results {
			decoded.Results = append(decoded.Results, FailureToolResult{
				Sequence:      record.Sequence,
				ToolCallID:    record.ToolCallID,
				Tool:          record.Tool,
				Failed:        record.Failed,
				TotalBytes:    record.TotalBytes,
				OmittedBytes:  record.OmittedBytes,
				RedactedBytes: record.RedactedBytes,
				Head:          decodeFailureSpan(record.Head),
				Tail:          decodeFailureSpan(record.Tail),
			})
		}
		for _, part := range turn.Content {
			decoded.Content = append(decoded.Content, FailureWorkerPart{
				Kind:          string(part.Kind),
				Tool:          part.Tool,
				ToolCallID:    part.ToolCallID,
				TotalBytes:    part.TotalBytes,
				OmittedBytes:  part.OmittedBytes,
				RedactedBytes: part.RedactedBytes,
				Head:          decodeFailureSpan(part.Head),
				Tail:          decodeFailureSpan(part.Tail),
			})
		}
		result.Turns = append(result.Turns, decoded)
	}
	if result.Turns == nil {
		result.Turns = []FailureTurn{}
	}
	return result
}

// parseFailureEventBody extracts the association and the stored context
// from one failure/uncertain event body. Old bodies without the new key
// decode as absent (nil context), not corrupt. Unknown shapes decode as
// absent, never as corrupt: the digest check in Snapshot/ReadWindow
// already guards tampering, and Status must never fail on a body it does
// not recognize.
func parseFailureEventBody(body []byte) (EventAssociation, *failureContextStored) {
	var assoc EventAssociation
	_ = json.Unmarshal(body, &assoc)
	if len(body) == 0 {
		return assoc, nil
	}
	var envelope struct {
		FailureTurnContext *failureContextStored `json:"failure_turn_context"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return assoc, nil
	}
	return assoc, envelope.FailureTurnContext
}

// isFailureTurnContextKind reports whether an event kind carries a
// failure-turn context: the operational-failure and uncertain outcomes for
// a driver.dispatch, including continuation suffixes and the preparation
// and completion uncertain outcomes that follow a real invoke.
func isFailureTurnContextKind(kind string) bool {
	switch {
	case strings.HasPrefix(kind, "dispatch_operational_failure"):
		return true
	case kind == "dispatch_uncertain" || strings.HasPrefix(kind, "dispatch_uncertain."):
		return true
	case strings.HasPrefix(kind, "dispatch_preparation_failed"):
		return true
	case strings.HasPrefix(kind, "dispatch_preparation_uncertain"):
		return true
	case strings.HasPrefix(kind, "dispatch_completion_uncertain"):
		return true
	default:
		return false
	}
}

// failureContextsForSnapshot builds the served failure-turn context for
// every driver.dispatch failure/uncertain event in one pass keyed by
// effect ID (never a scan per failed effect). The snapshot is already
// digest-checked by Snapshot/ReadWindow, so tampering surfaces as
// CORRUPT_JOURNAL before this projection runs. Old bodies without the new
// key yield no entry (absent, not corrupt); the latest failure event per
// effect wins.
func failureContextsForSnapshot(snapshot journal.Snapshot) map[string]*FailureTurnContext {
	result := make(map[string]*FailureTurnContext)
	for _, event := range snapshot.Events {
		if !isFailureTurnContextKind(event.Kind) {
			continue
		}
		assoc, stored := parseFailureEventBody(event.Body)
		if assoc.EffectID == "" || stored == nil {
			continue
		}
		decoded := decodeFailureContextStored(stored)
		if decoded == nil {
			continue
		}
		result[assoc.EffectID] = decoded
	}
	return result
}

// failureContextForPinnedWork finds the pinned work's latest failed
// dispatch context: for an economy pin the crossing's own dispatch-work
// identity directly, otherwise the dispatch whose owner is the pinned
// work. It reuses the already-decoded Effect contexts (same pointer, no
// re-decode) so Effects, PinnedWork and Node stay in parity. Nil means the
// pin has no failed dispatch to show.
func failureContextForPinnedWork(snapshot journal.Snapshot, effects []EffectStatus, pinned PinnedWork) *FailureTurnContext {
	byID := make(map[string]*FailureTurnContext, len(effects))
	for index := range effects {
		if effects[index].FailureTurnContext != nil {
			byID[effects[index].ID] = effects[index].FailureTurnContext
		}
	}
	var bestID string
	var bestEpoch, bestTry int64 = -1, -1
	for _, effect := range effects {
		if effect.Kind != "driver.dispatch" || effect.State != string(journal.OperationalFailed) {
			continue
		}
		work, epoch, try, err := attemptCoordinates(effect.ID)
		if err != nil {
			continue
		}
		matched := false
		if pinned.DispatchWorkID != "" {
			matched = work == pinned.DispatchWorkID
		} else {
			matched = ownerWorkForDispatch(snapshot, work) == pinned.WorkID
		}
		if !matched {
			continue
		}
		if epoch > bestEpoch || (epoch == bestEpoch && try > bestTry) {
			bestEpoch, bestTry, bestID = epoch, try, effect.ID
		}
	}
	if bestID == "" {
		return nil
	}
	return byID[bestID]
}
