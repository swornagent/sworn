package cockpit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

// ActivitySchemaVersion versions the activity projection wire shape. It is
// distinct from SnapshotSchemaVersion: the activity route serves turn
// content, while the events route serves invalidate ticks only.
const ActivitySchemaVersion = "sworn.activity/v1"

const (
	// ActivityRingMaxTurnsPerDispatch bounds one live dispatch's ring in
	// turn-projection events (each event is one part of one turn). A ring
	// that is full drops its oldest event and counts the drop; it never
	// blocks the dispatch.
	ActivityRingMaxTurnsPerDispatch = 64
	// ActivityRingMaxBytesPerDispatch bounds one live dispatch's ring in
	// journaled body bytes. Bodies are already bounded and redacted for
	// journaling; the ring holds only those bytes, never raw worker
	// output.
	ActivityRingMaxBytesPerDispatch = 256 * 1024
)

const (
	activityWorkerTurnKind   = "worker_turn_observed"
	activityToolResultKind   = "tool_result_observed"
	activityWorkerTurnSchema = "sworn.worker-turn/v1"
	activityToolResultSchema = "sworn.tool-result-turn/v1"
)

// ActivityFilter narrows one activity page. Empty fields match everything;
// an empty Track or Slice also matches run-scoped turns with no track or
// slice, mirroring the events route's track filter. EffectID and WorkID
// match exactly when set.
type ActivityFilter struct {
	Track    string
	Slice    string
	EffectID string
	WorkID   string
}

// ActivityPage is one ordered, paged list of worker turns. Turns are
// ordered by Offset ascending; Offset is the max durable event offset of
// the turn's contributing events and governs the route cursor: a client
// resumes with after=ThroughOffset (or Last-Event-ID) and receives only
// turns with Offset greater than it, with no gap and no duplicate.
//
// ThroughOffset is the cursor for the next page; EventOffset is the
// journal high watermark at read time (or the max ring offset when the
// ring raced ahead of the high read). HasMore means more turns are already
// durable beyond ThroughOffset. Live is true when the serving projector
// has a ring (the serve host drives the run in-process) and false when it
// serves from the journal alone; the journal-only path says nothing false
// about liveness. RingDropped is the serving ring's cumulative
// oldest-drop count for this run.
type ActivityPage struct {
	SchemaVersion string         `json:"schema_version"`
	RunID         string         `json:"run_id"`
	Turns         []ActivityTurn `json:"turns"`
	ThroughOffset int64          `json:"through_offset"`
	EventOffset   int64          `json:"event_offset"`
	HasMore       bool           `json:"has_more"`
	Live          bool           `json:"live"`
	RingDropped   int64          `json:"ring_dropped,omitempty"`
}

// ActivityTurn is one worker turn with its tool results keyed to it. It is
// built from the journaled worker-turn and tool-result events that share
// one (effect_id, turn) identity, merged in part order. A turn exists
// without a worker-turn event (an HTTP-lane turn built from tool results
// alone) and without tool results (a native turn that called no tools).
// Parts is the max parts geometry seen (0 means a single part);
// DroppedEvents is the max dropped_events count across contributing
// events, surfaced rather than hidden.
type ActivityTurn struct {
	Offset         int64  `json:"offset"`
	EffectID       string `json:"effect_id,omitempty"`
	WorkID         string `json:"work_id,omitempty"`
	Track          string `json:"track,omitempty"`
	Slice          string `json:"slice,omitempty"`
	Role           string `json:"role,omitempty"`
	Responsibility string `json:"responsibility,omitempty"`
	Attempt        int64  `json:"attempt"`
	Epoch          int64  `json:"epoch"`
	Try            int64  `json:"try"`
	Turn           int64  `json:"turn"`
	Parts          int64  `json:"parts,omitempty"`
	DroppedEvents  int64  `json:"dropped_events,omitempty"`
	// InputTokens is the latest turn's reported input-token count
	// (S6-context-window-clamp A4), nil when absent.
	InputTokens *int64           `json:"input_tokens,omitempty"`
	Content     []ActivityPart   `json:"content,omitempty"`
	Results     []ActivityResult `json:"results,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
}

// ActivityPart is one bounded, decoded content block of a worker turn.
// Head and Tail are decoded display strings: the base64 head/tail spans
// decoded explicitly with U+FFFD replacement for non-UTF-8 bytes.
// TotalBytes, OmittedBytes and RedactedBytes stay the authoritative
// raw-byte counts shown to the operator; omitted bytes are never
// reassembled, guessed or fetched.
type ActivityPart struct {
	Kind          string `json:"kind"`
	Tool          string `json:"tool,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	TotalBytes    int64  `json:"total_bytes"`
	OmittedBytes  int64  `json:"omitted_bytes"`
	RedactedBytes int64  `json:"redacted_bytes"`
	Head          string `json:"head"`
	Tail          string `json:"tail"`
}

// ActivityResult is one bounded, decoded tool result keyed to its turn,
// on the same display discipline as ActivityPart.
type ActivityResult struct {
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

// ActivityAPI is the optional activity projection seam. The HTTP handler
// type-asserts its SnapshotAPI to this interface so the project-wide host
// and existing doubles are not forced to fabricate liveness: a projector
// without a ring serves the same content from the journal alone with
// Live=false.
type ActivityAPI interface {
	Activity(context.Context, string, int64, int, ActivityFilter) (ActivityPage, error)
}

// activityEnvelope is the shared association-field shape the runtime
// journals for both worker-turn and tool-result events. Only the fields
// the activity projection reads are named; unknown fields are ignored so
// the projection fails closed on kind and schema version, never on
// forward-compatible additions.
type activityEnvelope struct {
	SchemaVersion  string `json:"schema_version"`
	RunID          string `json:"run_id"`
	Track          string `json:"track"`
	Slice          string `json:"slice"`
	Role           string `json:"role"`
	Responsibility string `json:"responsibility"`
	Attempt        int64  `json:"attempt"`
	Epoch          int64  `json:"epoch"`
	Try            int64  `json:"try"`
	WorkID         string `json:"work_id"`
	EffectID       string `json:"effect_id"`
	Turn           int64  `json:"turn"`
	Part           int64  `json:"part"`
	Parts          int64  `json:"parts"`
	DroppedEvents  int64  `json:"dropped_events"`
	Encoding       string `json:"encoding"`
	// InputTokens carries the latest turn's reported input-token count
	// through unchanged (S6-context-window-clamp A4); nil when absent or
	// on a worker-turn envelope, which never carries it.
	InputTokens *int64                    `json:"input_tokens,omitempty"`
	Results     []driver.ToolResultRecord `json:"results"`
	Content     []driver.WorkerTurnPart   `json:"content"`
}

// activityEvent is one journaled or ring-held event considered for one
// activity page.
type activityEvent struct {
	Offset    int64
	Kind      string
	Body      []byte
	CreatedAt time.Time
}

type envelopeWithMeta struct {
	envelope  activityEnvelope
	offset    int64
	createdAt time.Time
}

type activityBuilder struct {
	effectID       string
	workID         string
	track          string
	slice          string
	role           string
	responsibility string
	attempt        int64
	epoch          int64
	try            int64
	turn           int64
	parts          int64
	dropped        int64
	minOffset      int64
	maxOffset      int64
	createdAt      time.Time
	workerEvents   []envelopeWithMeta
	toolEvents     []envelopeWithMeta
}

func isActivityKind(kind string) bool {
	return kind == activityWorkerTurnKind || kind == activityToolResultKind
}

// parseActivityEnvelope fails closed: only the two journaled worker-turn
// and tool-result kinds with their exact expected schema versions and
// base64 encoding are ever placed on the activity route or in the ring.
// Any other kind, schema, encoding, or undecodable body is skipped, never
// rendered.
func parseActivityEnvelope(kind string, body []byte) (activityEnvelope, bool) {
	var envelope activityEnvelope
	if !isActivityKind(kind) || len(body) == 0 || len(body) > 256*1024 {
		return activityEnvelope{}, false
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return activityEnvelope{}, false
	}
	switch kind {
	case activityWorkerTurnKind:
		if envelope.SchemaVersion != activityWorkerTurnSchema {
			return activityEnvelope{}, false
		}
	case activityToolResultKind:
		if envelope.SchemaVersion != activityToolResultSchema {
			return activityEnvelope{}, false
		}
	default:
		return activityEnvelope{}, false
	}
	if envelope.Encoding != "" && envelope.Encoding != "base64" {
		return activityEnvelope{}, false
	}
	if envelope.EffectID == "" {
		return activityEnvelope{}, false
	}
	return envelope, true
}

func activityFilterMatch(envelope activityEnvelope, filter ActivityFilter) bool {
	if filter.Track != "" && envelope.Track != "" && envelope.Track != filter.Track {
		return false
	}
	if filter.Slice != "" && envelope.Slice != "" && envelope.Slice != filter.Slice {
		return false
	}
	if filter.EffectID != "" && envelope.EffectID != filter.EffectID {
		return false
	}
	if filter.WorkID != "" && envelope.WorkID != filter.WorkID {
		return false
	}
	return true
}

// decodeActivitySpan decodes one base64 head/tail span for display with
// explicit U+FFFD replacement for non-UTF-8 bytes. Head and tail are
// arbitrary byte spans cut at a byte bound post-redaction; decoding never
// reassembles omitted bytes and the raw-byte counts stay authoritative.
// An undecodable span degrades to an empty display string with its counts
// preserved, never to a dropped turn.
func decodeActivitySpan(encoded string) string {
	if encoded == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return strings.ToValidUTF8(string(raw), "\uFFFD")
}

func projectActivityPart(part driver.WorkerTurnPart) ActivityPart {
	return ActivityPart{
		Kind:          string(part.Kind),
		Tool:          part.Tool,
		ToolCallID:    part.ToolCallID,
		TotalBytes:    part.TotalBytes,
		OmittedBytes:  part.OmittedBytes,
		RedactedBytes: part.RedactedBytes,
		Head:          decodeActivitySpan(part.Head),
		Tail:          decodeActivitySpan(part.Tail),
	}
}

func projectActivityResult(record driver.ToolResultRecord) ActivityResult {
	return ActivityResult{
		Sequence:      record.Sequence,
		ToolCallID:    record.ToolCallID,
		Tool:          record.Tool,
		Failed:        record.Failed,
		TotalBytes:    record.TotalBytes,
		OmittedBytes:  record.OmittedBytes,
		RedactedBytes: record.RedactedBytes,
		Head:          decodeActivitySpan(record.Head),
		Tail:          decodeActivitySpan(record.Tail),
	}
}

func partOrder(part int64) int64 {
	if part <= 0 {
		return 1
	}
	return part
}

// groupActivityEvents joins worker-turn and tool-result events by
// (effect_id, turn), merges part/parts events sharing one identity in part
// order, and emits one turn row per turn instance ordered by max offset. A
// turn instance is one logical worker turn: one worker side (one event or
// one split group) joined to one tool side. A turn built from tool results
// alone (no worker-turn event, the HTTP lane) is emitted, as is a turn
// built from worker content alone (a native turn that called no tools);
// part geometry and dropped counts are merged per instance (max parts, max
// dropped) rather than hidden. Events failing the kind/schema filter or
// the run/filter match are skipped.
//
// A dispatch that parks on a human turn and resumes reuses its provider
// turn numbers: the resumed invocation journals new events with the same
// (effect_id, turn) as the pre-park invocation (the e2e implementer
// journals Turn 1 sworn_yield before the park and Turn 1 Write after it).
// Those are two distinct worker turns sharing one number, not two parts
// of one turn. Merging them into one row makes the row's content grow
// after it was already served (the live stream's Turn 1 at offset 39 with
// one result versus the journal's Turn 1 at offset 39 with two), which
// breaks A2's no-gap-no-duplicate resume and A6's live-equals-journal
// check. The projection therefore splits same-number events into
// instances: only events carrying split geometry (parts > 1) merge; each
// single-part event is its own instance, and worker/tool sides pair by
// shared tool-call identity with offset proximity breaking ties. Each
// instance's offset is then stable once journaled, so resuming from it
// never re-emits the same identity with new content.
//
// Cursor discipline: parts of one split turn are consecutive journal
// events (the driver enqueues a split turn's parts back-to-back and the
// observer pump journals them back-to-back), so a split turn's parts never
// straddle an activity page boundary except for a microsecond live-tail
// race between two back-to-back journal appends. The projection emits
// whatever parts are durable in the examined window; the race resolves on
// the next poll.
func groupActivityEvents(events []activityEvent, runID string, filter ActivityFilter) []ActivityTurn {
	builders := make(map[string]*activityBuilder)
	for _, event := range events {
		envelope, ok := parseActivityEnvelope(event.Kind, event.Body)
		if !ok {
			continue
		}
		if envelope.RunID != "" && envelope.RunID != runID {
			continue
		}
		if !activityFilterMatch(envelope, filter) {
			continue
		}
		// Stable, collision-free join key on (effect_id, turn).
		key := envelope.EffectID + "\x00" + itoaTurn(envelope.Turn)
		builder, found := builders[key]
		if !found {
			builder = &activityBuilder{
				effectID:       envelope.EffectID,
				workID:         envelope.WorkID,
				track:          envelope.Track,
				slice:          envelope.Slice,
				role:           envelope.Role,
				responsibility: envelope.Responsibility,
				attempt:        envelope.Attempt,
				epoch:          envelope.Epoch,
				try:            envelope.Try,
				turn:           envelope.Turn,
				minOffset:      event.Offset,
				maxOffset:      event.Offset,
				createdAt:      event.CreatedAt,
			}
			builders[key] = builder
		}
		if event.Offset < builder.minOffset {
			builder.minOffset = event.Offset
		}
		if event.Offset > builder.maxOffset {
			builder.maxOffset = event.Offset
		}
		if event.CreatedAt.After(builder.createdAt) {
			builder.createdAt = event.CreatedAt
		}
		if envelope.Parts > builder.parts {
			builder.parts = envelope.Parts
		}
		if envelope.DroppedEvents > builder.dropped {
			builder.dropped = envelope.DroppedEvents
		}
		meta := envelopeWithMeta{envelope: envelope, offset: event.Offset, createdAt: event.CreatedAt}
		if event.Kind == activityWorkerTurnKind {
			builder.workerEvents = append(builder.workerEvents, meta)
		} else {
			builder.toolEvents = append(builder.toolEvents, meta)
		}
	}
	turns := make([]ActivityTurn, 0, len(builders))
	for _, builder := range builders {
		turns = append(turns, buildActivityInstances(builder)...)
	}
	sort.Slice(turns, func(i, j int) bool {
		if turns[i].Offset != turns[j].Offset {
			return turns[i].Offset < turns[j].Offset
		}
		if turns[i].EffectID != turns[j].EffectID {
			return turns[i].EffectID < turns[j].EffectID
		}
		return turns[i].Turn < turns[j].Turn
	})
	return turns
}

// buildActivityInstances splits one (effect_id, turn) builder into one or
// more stable turn instances. Split geometry (parts > 1) merges; distinct
// single-part events for the same number (a human-resumed dispatch
// reusing turn numbers) stay separate so no served row ever grows.
func buildActivityInstances(builder *activityBuilder) []ActivityTurn {
	workerGroups := partitionActivityGroups(builder.workerEvents)
	toolGroups := partitionActivityGroups(builder.toolEvents)
	if len(workerGroups) == 0 {
		turns := make([]ActivityTurn, 0, len(toolGroups))
		for _, group := range toolGroups {
			turns = append(turns, newActivityTurn(builder, nil, group))
		}
		return turns
	}
	if len(toolGroups) == 0 {
		turns := make([]ActivityTurn, 0, len(workerGroups))
		for _, group := range workerGroups {
			turns = append(turns, newActivityTurn(builder, group, nil))
		}
		return turns
	}
	if len(workerGroups) == 1 && len(toolGroups) == 1 {
		return []ActivityTurn{newActivityTurn(builder, workerGroups[0], toolGroups[0])}
	}
	pairedTool := make([]bool, len(toolGroups))
	workerIDs := make([]map[string]struct{}, len(workerGroups))
	for i, group := range workerGroups {
		workerIDs[i] = activityGroupToolCallIDs(group, true)
	}
	toolIDs := make([]map[string]struct{}, len(toolGroups))
	toolMin := make([]int64, len(toolGroups))
	for i, group := range toolGroups {
		toolIDs[i] = activityGroupToolCallIDs(group, false)
		toolMin[i] = group[0].offset
		for _, meta := range group[1:] {
			if meta.offset < toolMin[i] {
				toolMin[i] = meta.offset
			}
		}
	}
	turns := make([]ActivityTurn, 0, len(workerGroups)+len(toolGroups))
	for i, group := range workerGroups {
		workerMin := group[0].offset
		for _, meta := range group[1:] {
			if meta.offset < workerMin {
				workerMin = meta.offset
			}
		}
		best := -1
		var bestDist int64
		for j := range toolGroups {
			if pairedTool[j] || !activityIDsOverlap(workerIDs[i], toolIDs[j]) {
				continue
			}
			dist := workerMin - toolMin[j]
			if dist < 0 {
				dist = -dist
			}
			if best < 0 || dist < bestDist {
				best, bestDist = j, dist
			}
		}
		if best < 0 {
			turns = append(turns, newActivityTurn(builder, group, nil))
			continue
		}
		pairedTool[best] = true
		turns = append(turns, newActivityTurn(builder, group, toolGroups[best]))
	}
	for j, group := range toolGroups {
		if !pairedTool[j] {
			turns = append(turns, newActivityTurn(builder, nil, group))
		}
	}
	return turns
}

// partitionActivityGroups splits one side's events (already sharing one
// (effect_id, turn)) into logical turns. Each single-part event
// (parts <= 1) is its own group; consecutive split parts (parts > 1) with
// the same parts count accumulate into one group of that size. Groups
// preserve offset order; events inside a split group are ordered by part
// then offset when projected.
func partitionActivityGroups(events []envelopeWithMeta) [][]envelopeWithMeta {
	if len(events) == 0 {
		return nil
	}
	ordered := append([]envelopeWithMeta(nil), events...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].offset < ordered[j].offset })
	var groups [][]envelopeWithMeta
	var cur []envelopeWithMeta
	var curParts int64
	flush := func() {
		if len(cur) > 0 {
			groups = append(groups, cur)
			cur, curParts = nil, 0
		}
	}
	for _, meta := range ordered {
		parts := meta.envelope.Parts
		if parts <= 1 {
			flush()
			groups = append(groups, []envelopeWithMeta{meta})
			continue
		}
		if len(cur) == 0 {
			cur, curParts = []envelopeWithMeta{meta}, parts
		} else if curParts != parts {
			flush()
			cur, curParts = []envelopeWithMeta{meta}, parts
		} else {
			cur = append(cur, meta)
		}
		if int64(len(cur)) >= curParts {
			flush()
		}
	}
	flush()
	return groups
}

// activityGroupToolCallIDs collects the tool-call identities one logical
// group carries: worker sides from content parts, tool sides from results.
// Pairing joins worker and tool sides sharing an identity.
func activityGroupToolCallIDs(group []envelopeWithMeta, worker bool) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, meta := range group {
		if worker {
			for _, part := range meta.envelope.Content {
				if part.ToolCallID != "" {
					ids[part.ToolCallID] = struct{}{}
				}
			}
			continue
		}
		for _, record := range meta.envelope.Results {
			if record.ToolCallID != "" {
				ids[record.ToolCallID] = struct{}{}
			}
		}
	}
	return ids
}

func activityIDsOverlap(left, right map[string]struct{}) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	for id := range left {
		if _, found := right[id]; found {
			return true
		}
	}
	return false
}

// newActivityTurn projects one logical turn instance: at most one worker
// side and at most one tool side. Either side may be nil (a tool-only
// HTTP turn or a worker-only native turn). Split sides merge in part
// order; counts are the instance's max, never the whole number's.
func newActivityTurn(builder *activityBuilder, worker, tool []envelopeWithMeta) ActivityTurn {
	orderedWorker := append([]envelopeWithMeta(nil), worker...)
	sort.Slice(orderedWorker, func(i, j int) bool {
		pi, pj := partOrder(orderedWorker[i].envelope.Part), partOrder(orderedWorker[j].envelope.Part)
		if pi != pj {
			return pi < pj
		}
		return orderedWorker[i].offset < orderedWorker[j].offset
	})
	orderedTool := append([]envelopeWithMeta(nil), tool...)
	sort.Slice(orderedTool, func(i, j int) bool {
		pi, pj := partOrder(orderedTool[i].envelope.Part), partOrder(orderedTool[j].envelope.Part)
		if pi != pj {
			return pi < pj
		}
		return orderedTool[i].offset < orderedTool[j].offset
	})
	var maxOffset int64
	var parts, dropped int64
	var inputTokens *int64
	var createdAt time.Time
	first := true
	consider := func(meta envelopeWithMeta) {
		if first || meta.offset > maxOffset {
			maxOffset = meta.offset
		}
		if meta.envelope.Parts > parts {
			parts = meta.envelope.Parts
		}
		if meta.envelope.DroppedEvents > dropped {
			dropped = meta.envelope.DroppedEvents
		}
		if meta.envelope.InputTokens != nil {
			value := *meta.envelope.InputTokens
			inputTokens = &value
		}
		if first || meta.createdAt.After(createdAt) {
			createdAt = meta.createdAt
		}
		first = false
	}
	for _, meta := range orderedWorker {
		consider(meta)
	}
	for _, meta := range orderedTool {
		consider(meta)
	}
	turn := ActivityTurn{
		Offset:         maxOffset,
		EffectID:       builder.effectID,
		WorkID:         builder.workID,
		Track:          builder.track,
		Slice:          builder.slice,
		Role:           builder.role,
		Responsibility: builder.responsibility,
		Attempt:        builder.attempt,
		Epoch:          builder.epoch,
		Try:            builder.try,
		Turn:           builder.turn,
		Parts:          parts,
		DroppedEvents:  dropped,
		InputTokens:    inputTokens,
		CreatedAt:      createdAt,
	}
	for _, meta := range orderedWorker {
		for _, part := range meta.envelope.Content {
			turn.Content = append(turn.Content, projectActivityPart(part))
		}
	}
	for _, meta := range orderedTool {
		for _, record := range meta.envelope.Results {
			turn.Results = append(turn.Results, projectActivityResult(record))
		}
	}
	return turn
}

func itoaTurn(turn int64) string {
	if turn == 0 {
		return "0"
	}
	negative := turn < 0
	if negative {
		turn = -turn
	}
	var digits [32]byte
	pos := len(digits)
	for turn > 0 {
		pos--
		digits[pos] = byte('0' + turn%10)
		turn /= 10
	}
	if negative {
		pos--
		digits[pos] = '-'
	}
	return string(digits[pos:])
}

// Activity implements ActivityAPI over the journaled worker-turn and
// tool-result events S1 and the existing tool-result observation journaled.
// It reads only those bodies via ReadWindow, joins by (effect_id, turn),
// and merges the in-memory ring ahead of the journal when one is wired.
//
// Single-cursor discipline: the ring is fed only after the durable append
// and carries the durable offset (journal.Store.AppendEventWithOffset), so
// the ring and the journal share one cursor space. The route merges by
// offset (journal preferred on duplicates, which are byte-identical) and
// resumes from that one cursor with no gap and no duplicate.
func (p *Projector) Activity(
	ctx context.Context,
	runID string,
	after int64,
	limit int,
	filter ActivityFilter,
) (ActivityPage, error) {
	if p == nil || ctx == nil || runID == "" {
		return ActivityPage{}, fail("INVALID_REQUEST")
	}
	if after < 0 || limit < 1 || limit > 256 {
		return ActivityPage{}, fail("INVALID_REQUEST")
	}
	wr, ok := p.journal.(windowReader)
	if !ok {
		return ActivityPage{}, fail("JOURNAL_UNAVAILABLE")
	}
	highWindow, err := p.journal.EventsAfter(ctx, runID, after, 1)
	if err != nil {
		return ActivityPage{}, fail("JOURNAL_UNAVAILABLE")
	}
	high := highWindow.EventOffset
	live := false
	var ringEvents []activityEvent
	var ringDropped int64
	if p.activityRing != nil {
		live = true
		ringEvents, ringDropped = p.activityRing.eventsAfter(runID, after)
		for _, re := range ringEvents {
			if re.Offset > high {
				high = re.Offset
			}
		}
	}
	var collected []activityEvent
	cursor := after
	for cursor < high {
		chunk := 256
		if remaining := high - cursor; remaining < int64(chunk) {
			chunk = int(remaining)
			if chunk < 1 {
				chunk = 1
			}
		}
		if chunk > 1024 {
			chunk = 1024
		}
		window, err := wr.ReadWindow(ctx, runID, cursor, chunk)
		if err != nil {
			return ActivityPage{}, fail("JOURNAL_UNAVAILABLE")
		}
		if len(window.Snapshot.Events) == 0 {
			if window.ThroughEventOffset > high {
				high = window.ThroughEventOffset
			}
			if window.ThroughEventOffset <= cursor {
				break
			}
			cursor = window.ThroughEventOffset
			if !window.HasMoreEvents {
				break
			}
			continue
		}
		for _, event := range window.Snapshot.Events {
			collected = append(collected, activityEvent{
				Offset:    event.Offset,
				Kind:      event.Kind,
				Body:      event.Body,
				CreatedAt: event.CreatedAt,
			})
		}
		prev := cursor
		cursor = window.ThroughEventOffset
		if cursor > high {
			high = cursor
		}
		if cursor <= prev {
			break
		}
		// Stop early once the examined window already holds a full page
		// of turns; the next page resumes from ThroughOffset.
		merged := mergeActivityEvents(collected, ringEvents)
		if len(groupActivityEvents(merged, runID, filter)) >= limit {
			break
		}
		if !window.HasMoreEvents {
			break
		}
	}
	merged := mergeActivityEvents(collected, ringEvents)
	allTurns := groupActivityEvents(merged, runID, filter)
	// Turns are already ordered by Offset; filter to the requested cursor.
	filtered := allTurns[:0]
	for _, turn := range allTurns {
		if turn.Offset > after {
			filtered = append(filtered, turn)
		}
	}
	allTurns = filtered
	hasMore := false
	emitted := allTurns
	if len(allTurns) > limit {
		emitted = allTurns[:limit]
		hasMore = true
	} else if cursor < high {
		// The examined window was cut short by the early-stop above but
		// held fewer than limit turns only because non-activity events
		// filled it; more journal remains that might hold activity.
		// Re-examine without the early stop is unnecessary: the next page
		// resumes from ThroughOffset and will find it.
		hasMore = true
	}
	through := high
	if hasMore {
		if len(emitted) > 0 {
			through = emitted[len(emitted)-1].Offset
		} else {
			through = after
		}
	} else {
		through = high
		if len(emitted) > 0 && emitted[len(emitted)-1].Offset > through {
			through = emitted[len(emitted)-1].Offset
		}
	}
	if emitted == nil {
		emitted = []ActivityTurn{}
	}
	return ActivityPage{
		SchemaVersion: ActivitySchemaVersion,
		RunID:         runID,
		Turns:         emitted,
		ThroughOffset: through,
		EventOffset:   high,
		HasMore:       hasMore,
		Live:          live,
		RingDropped:   ringDropped,
	}, nil
}

// mergeActivityEvents unions journal and ring events by durable offset.
// The ring is fed only after the durable append, so duplicates are the
// same durable event byte for byte; the journal copy wins.
func mergeActivityEvents(journalEvents, ringEvents []activityEvent) []activityEvent {
	if len(ringEvents) == 0 {
		return journalEvents
	}
	seen := make(map[int64]struct{}, len(journalEvents)+len(ringEvents))
	merged := make([]activityEvent, 0, len(journalEvents)+len(ringEvents))
	for _, event := range journalEvents {
		if _, dup := seen[event.Offset]; dup {
			continue
		}
		seen[event.Offset] = struct{}{}
		merged = append(merged, event)
	}
	for _, event := range ringEvents {
		if _, dup := seen[event.Offset]; dup {
			continue
		}
		seen[event.Offset] = struct{}{}
		merged = append(merged, event)
	}
	return merged
}

// ActivityRing is the bounded, ephemeral, in-memory latency shortening for
// the activity projection. One ring serves one serve host's driven run;
// entries are keyed per live dispatch (run, effect) and dropped when the
// dispatch ends. The ring holds only projections that were already bounded
// and redacted for journaling, is never persisted, never blocks the
// dispatch, and drops its oldest event with a loud count when full.
type ActivityRing struct {
	mu      sync.Mutex
	runs    map[string]*activityRunRing
	dropped int64
}

type activityRunRing struct {
	dispatches map[string]*activityDispatchRing
	dropped    int64
}

type activityDispatchRing struct {
	events []activityEvent
	bytes  int64
}

// NewActivityRing builds the empty ring the serve host wires into both the
// runtime Service (as its ActivityTap) and the cockpit Projector.
func NewActivityRing() *ActivityRing {
	return &ActivityRing{runs: make(map[string]*activityRunRing)}
}

// ObserveActivity implements runtime.ActivityTap. It stores only the two
// journaled activity kinds with their exact expected schema versions;
// anything else is skipped, never placed in the ring. It never blocks:
// a full dispatch ring drops its oldest event and counts the drop.
func (r *ActivityRing) ObserveActivity(event runtimepkg.ActivityTapEvent) {
	if r == nil || event.RunID == "" || event.EffectID == "" || event.Offset <= 0 {
		return
	}
	if event.Kind != activityWorkerTurnKind && event.Kind != activityToolResultKind {
		return
	}
	if _, ok := parseActivityEnvelope(event.Kind, event.Body); !ok {
		return
	}
	body := append([]byte(nil), event.Body...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs == nil {
		r.runs = make(map[string]*activityRunRing)
	}
	run, found := r.runs[event.RunID]
	if !found {
		run = &activityRunRing{dispatches: make(map[string]*activityDispatchRing)}
		r.runs[event.RunID] = run
	}
	dispatch, found := run.dispatches[event.EffectID]
	if !found {
		dispatch = &activityDispatchRing{}
		run.dispatches[event.EffectID] = dispatch
	}
	entry := activityEvent{Offset: event.Offset, Kind: event.Kind, Body: body, CreatedAt: event.CreatedAt}
	for len(dispatch.events) >= ActivityRingMaxTurnsPerDispatch ||
		dispatch.bytes+int64(len(body)) > ActivityRingMaxBytesPerDispatch && len(dispatch.events) > 0 {
		oldest := dispatch.events[0]
		copy(dispatch.events, dispatch.events[1:])
		dispatch.events[len(dispatch.events)-1] = activityEvent{}
		dispatch.events = dispatch.events[:len(dispatch.events)-1]
		dispatch.bytes -= int64(len(oldest.Body))
		if dispatch.bytes < 0 {
			dispatch.bytes = 0
		}
		run.dropped++
		r.dropped++
	}
	dispatch.events = append(dispatch.events, entry)
	dispatch.bytes += int64(len(body))
}

// DropActivityDispatch implements runtime.ActivityTap: it forgets one
// dispatch's ring entries when the dispatch ends. The journal already
// holds every projection the ring ever held.
func (r *ActivityRing) DropActivityDispatch(effectID string) {
	if r == nil || effectID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		delete(run.dispatches, effectID)
	}
}

// eventsAfter returns the ring's events for one run with offset greater
// than after, ordered by offset, plus the run's cumulative drop count.
func (r *ActivityRing) eventsAfter(runID string, after int64) ([]activityEvent, int64) {
	if r == nil || runID == "" {
		return nil, 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run, found := r.runs[runID]
	if !found {
		return nil, 0
	}
	var result []activityEvent
	for _, dispatch := range run.dispatches {
		for _, event := range dispatch.events {
			if event.Offset > after {
				result = append(result, activityEvent{
					Offset:    event.Offset,
					Kind:      event.Kind,
					Body:      append([]byte(nil), event.Body...),
					CreatedAt: event.CreatedAt,
				})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Offset < result[j].Offset })
	return result, run.dropped
}
