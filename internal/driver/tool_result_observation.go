package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Tool-result observation turns what a worker sees into durable events.
//
// Every tool result that crosses back into a model is projected at the
// canonical seams — the provider loop's appendResults crossings and the
// claude-native broker's writeBrokerResult crossing — into a bounded,
// versioned projection whose bytes are the exact model-facing bytes up to a
// declared head/tail bound. Emission rides a runtime-provided hook through a
// bounded driver-side queue with a pump goroutine, so an observer can never
// fail, stall, or alter the dispatch it observes: an enqueue never blocks
// the dispatch loop, hook failures and backpressure drops degrade
// observation only, and drain-at-close is bounded and loud.

const (
	// MaxToolResultHeadBytes and MaxToolResultTailBytes are the declared
	// projection bounds. OmittedBytes on every record names any remainder,
	// so bounding is never silent.
	MaxToolResultHeadBytes = 2_048
	MaxToolResultTailBytes = 2_048
	// toolResultEventBudget is the driver-local bound for one marshaled
	// turn event. It keeps a conservative margin under
	// journal.MaxEventBytes (256 KiB), which the driver cannot import; a
	// runtime test pins the worst case under the journal bound.
	toolResultEventBudget = 240 * 1024
	// toolResultObserverQueue bounds accepted-but-undelivered turn events.
	// A full queue drops loudly: the cumulative count rides on the next
	// accepted event as dropped_events.
	toolResultObserverQueue = 32
	// toolResultRedactedMarker replaces held credentials in the head/tail
	// spans. It is shorter than every admitted secret fragment (>= 16
	// bytes by the existing secret-guard minimum), so redacted spans can
	// never grow past the declared bound.
	toolResultRedactedMarker = "[redacted]"
	// toolResultCloseDrainAppend bounds one hook call during close-drain;
	// toolResultCloseDrainTotal bounds the whole drain. Both exist so a
	// wedged observer cannot stall dispatch completion.
	toolResultCloseDrainAppend = 2 * time.Second
	toolResultCloseDrainTotal  = 5 * time.Second
)

// ToolResultHook is the runtime-provided durable callback. The driver calls
// it from the observer pump goroutine, never from the dispatch loop; an
// error or a block degrades observation only.
type ToolResultHook func(context.Context, ToolResultTurn) error

// ToolResultRecord is the bounded, identity-carrying projection of one tool
// result. Head and Tail hold standard RFC 4648 padded base64 of the exact
// (post-redaction) head/tail byte spans; TotalBytes, OmittedBytes, and
// RedactedBytes count raw bytes, so redaction geometry stays honest:
// OmittedBytes is computed on the original spans and redaction is named
// separately.
type ToolResultRecord struct {
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

// ToolResultTurn is one coalesced turn of projected records. A pathological
// turn that would exceed the event byte budget is split into named
// part/parts events sharing the same turn identity; per-record sequences
// stay global to the turn across parts. DroppedEvents carries the
// observer's cumulative loud drop count at acceptance time.
type ToolResultTurn struct {
	Turn          int64              `json:"turn"`
	Part          int64              `json:"part,omitempty"`
	Parts         int64              `json:"parts,omitempty"`
	DroppedEvents int64              `json:"dropped_events,omitempty"`
	Results       []ToolResultRecord `json:"results"`
}

// projectToolResult builds the canonical bounded projection for one tool
// result. It is the single builder every seam consumes, so model-facing
// bytes and observed bytes cannot diverge up to the declared bound.
func projectToolResult(
	result providerToolResult,
	sequence int64,
	secrets [][]byte,
) ToolResultRecord {
	content := result.Content
	total := len(content)
	headLen := min(MaxToolResultHeadBytes, total)
	tailLen := min(MaxToolResultTailBytes, total-headLen)
	omitted := total - headLen - tailLen
	headBytes := content[:headLen]
	var tailBytes []byte
	if tailLen > 0 {
		tailBytes = content[total-tailLen:]
	}
	redactedHead, headRedacted := redactToolResultSpan(headBytes, secrets)
	redactedTail, tailRedacted := redactToolResultSpan(tailBytes, secrets)
	return ToolResultRecord{
		Sequence:      sequence,
		ToolCallID:    result.ID,
		Tool:          result.Name,
		Failed:        result.Failed,
		TotalBytes:    int64(total),
		OmittedBytes:  int64(omitted),
		RedactedBytes: headRedacted + tailRedacted,
		Head:          base64.StdEncoding.EncodeToString(redactedHead),
		Tail:          base64.StdEncoding.EncodeToString(redactedTail),
	}
}

// redactToolResultSpan replaces every held secret in a head/tail span with
// the fixed marker, longest-first so nested fragments cannot survive inside
// an already-replaced larger secret. It returns the redacted span and the
// count of raw bytes replaced.
func redactToolResultSpan(span []byte, secrets [][]byte) ([]byte, int64) {
	if len(span) == 0 || len(secrets) == 0 {
		return span, 0
	}
	ordered := append([][]byte(nil), secrets...)
	sort.Slice(ordered, func(i, j int) bool {
		return len(ordered[i]) > len(ordered[j])
	})
	out := append([]byte(nil), span...)
	var replaced int64
	marker := []byte(toolResultRedactedMarker)
	for _, secret := range ordered {
		if len(secret) < len(marker) {
			continue
		}
		occurrences := bytes.Count(out, secret)
		if occurrences == 0 {
			continue
		}
		out = bytes.ReplaceAll(out, secret, marker)
		replaced += int64(occurrences) * int64(len(secret))
	}
	return out, replaced
}

// splitToolResultParts keeps one coalesced turn under the event byte budget
// with exact compact-JSON accounting: the prefix plus "}" is the empty
// payload, and each record costs 1 (opening bracket or comma) plus its
// marshaled length. A turn over the budget becomes named parts; single-part
// turns carry no part geometry.
func splitToolResultParts(turn ToolResultTurn) []ToolResultTurn {
	if len(turn.Results) == 0 {
		return []ToolResultTurn{turn}
	}
	prefix := `{"turn":` + strconv.FormatInt(turn.Turn, 10) + `,"results":`
	size := len(prefix) + 1
	var parts []ToolResultTurn
	current := ToolResultTurn{Turn: turn.Turn}
	for _, record := range turn.Results {
		cost := 1 + len(mustToolResultRecordJSON(record))
		if len(current.Results) > 0 && size+cost > toolResultEventBudget {
			parts = append(parts, current)
			current = ToolResultTurn{Turn: turn.Turn}
			size = len(prefix) + 1
		}
		current.Results = append(current.Results, record)
		size += cost
	}
	parts = append(parts, current)
	if len(parts) > 1 {
		for index := range parts {
			parts[index].Part = int64(index + 1)
			parts[index].Parts = int64(len(parts))
		}
	}
	return parts
}

func mustToolResultRecordJSON(record ToolResultRecord) []byte {
	body, err := json.Marshal(record)
	if err != nil {
		// Marshal of this fixed shape cannot fail; a deliberately huge
		// defensive estimate keeps the budget honest if it ever does.
		return make(
			[]byte,
			2*MaxToolResultHeadBytes+2*MaxToolResultTailBytes+256,
		)
	}
	return body
}

// toolResultObserver is the bounded queue between the dispatch loop and the
// runtime hook. enqueue never blocks; a pump goroutine makes every hook
// call, so a synchronous journal append or a permanently blocked hook can
// neither fail, stall, nor alter the dispatch.
type toolResultObserver struct {
	hook     ToolResultHook
	queue    chan ToolResultTurn
	done     chan struct{}
	draining atomic.Bool
	// inFlight counts deliveries the pump has popped but not finished;
	// a close-drain timeout counts them as dropped so a wedged hook is
	// never a silent loss.
	inFlight atomic.Int64

	mu      sync.Mutex
	stopped bool
	dropped int64
}

func newToolResultObserver(hook ToolResultHook) *toolResultObserver {
	if hook == nil {
		return nil
	}
	observer := &toolResultObserver{
		hook:  hook,
		queue: make(chan ToolResultTurn, toolResultObserverQueue),
		done:  make(chan struct{}),
	}
	go observer.pump()
	return observer
}

func (observer *toolResultObserver) pump() {
	defer close(observer.done)
	for turn := range observer.queue {
		observer.inFlight.Add(1)
		observer.deliver(turn, observer.draining.Load())
		observer.inFlight.Add(-1)
	}
}

func (observer *toolResultObserver) deliver(turn ToolResultTurn, draining bool) {
	ctx := context.Background()
	cancel := func() {}
	if draining {
		ctx, cancel = context.WithTimeout(ctx, toolResultCloseDrainAppend)
	}
	defer cancel()
	if err := observer.hook(ctx, turn); err != nil {
		observer.noteDropped()
	}
}

// enqueue admits one turn event without ever blocking. The cumulative loud
// drop count rides on the accepted event; a full queue or a closed observer
// drops instead of admitting.
func (observer *toolResultObserver) enqueue(turn ToolResultTurn) {
	if observer == nil {
		return
	}
	observer.mu.Lock()
	if observer.stopped {
		observer.mu.Unlock()
		observer.noteDropped()
		return
	}
	turn.DroppedEvents = observer.dropped
	select {
	case observer.queue <- turn:
	default:
		observer.dropped++
	}
	observer.mu.Unlock()
}

func (observer *toolResultObserver) noteDropped() {
	observer.mu.Lock()
	observer.dropped++
	observer.mu.Unlock()
}

// close drains accepted events to the hook with a per-append timeout and an
// overall cap; undrained events are dropped and counted. It never fails and
// always returns, so dispatch completion is never stalled beyond the
// bounded drain.
func (observer *toolResultObserver) close() {
	if observer == nil {
		return
	}
	observer.mu.Lock()
	if observer.stopped {
		observer.mu.Unlock()
		return
	}
	observer.stopped = true
	observer.draining.Store(true)
	close(observer.queue)
	observer.mu.Unlock()
	timer := time.NewTimer(toolResultCloseDrainTotal)
	defer timer.Stop()
	select {
	case <-observer.done:
	case <-timer.C:
		// The pump is wedged past the drain bound: count every
		// not-yet-delivered event — queued and in-flight alike — as
		// dropped, loudly, and return. The mutex serializes against
		// enqueue, so the remaining length is exact and the closed
		// channel is never ranged (a closed channel is always ready).
		observer.mu.Lock()
		observer.dropped += observer.inFlight.Load()
		observer.dropped += int64(len(observer.queue))
		observer.mu.Unlock()
		return
	}
}

// observeToolResultTurn projects one turn of results that crossed into a
// model and enqueues the coalesced event (or its named parts). Sequences
// are per turn, in call order, global across parts.
func (session *toolSession) observeToolResultTurn(
	turn int64,
	results []providerToolResult,
) {
	if session == nil || len(results) == 0 {
		return
	}
	observer := session.observer
	if observer == nil {
		return
	}
	secrets := session.redactionSecrets()
	records := make([]ToolResultRecord, 0, len(results))
	for index, result := range results {
		records = append(
			records,
			projectToolResult(result, int64(index+1), secrets),
		)
	}
	for _, part := range splitToolResultParts(
		ToolResultTurn{Turn: turn, Results: records},
	) {
		observer.enqueue(part)
	}
}

// bindRedactionSecrets binds the credentials the engine actually holds —
// the broker capability, the capture bearer, and the launch credential
// snapshots — onto the tool session, exactly where runNative already
// materializes them. The provider loop has no broker, so its secret set is
// honestly empty; the provider bearer never reaches the worker's observable
// surface. Fragments shorter than the marker are ignored so a redacted span
// can never grow past the declared bound.
func (session *toolSession) bindRedactionSecrets(secrets [][]byte) {
	if session == nil || len(secrets) == 0 {
		return
	}
	marker := []byte(toolResultRedactedMarker)
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return
	}
	for _, secret := range secrets {
		if len(secret) < len(marker) {
			continue
		}
		duplicate := false
		for _, existing := range session.redaction {
			if bytes.Equal(existing, secret) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			session.redaction = append(
				session.redaction,
				append([]byte(nil), secret...),
			)
		}
	}
}

// redactionSecrets returns copies of the session's bound credentials.
func (session *toolSession) redactionSecrets() [][]byte {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	result := make([][]byte, 0, len(session.redaction))
	for _, secret := range session.redaction {
		result = append(result, append([]byte(nil), secret...))
	}
	return result
}

// redactionSecretSet copies and filters held credential values for
// binding; nil and short values are dropped by construction.
func redactionSecretSet(values ...[]byte) [][]byte {
	var result [][]byte
	for _, value := range values {
		if len(value) >= len(toolResultRedactedMarker) {
			result = append(result, append([]byte(nil), value...))
		}
	}
	return result
}
