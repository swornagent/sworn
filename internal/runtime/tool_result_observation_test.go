package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

func toolResultRuntimeFixture(
	t *testing.T,
) (*Service, *journal.Store, journal.Run) {
	t.Helper()
	ctx := context.Background()
	store, err := journal.Open(
		ctx,
		filepath.Join(t.TempDir(), "tool-result.sqlite"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 8, 23, 4, 5, 6, 0, time.UTC)
	run := journal.Run{
		ID:             "run-tool-result-events",
		ManifestDigest: sha256Digest([]byte("tool-result-manifest")),
		Repository:     t.TempDir(),
		Release:        "2026-08-23-telemetry-foundations",
		TargetRef:      "refs/heads/release/v1.0.0",
		CreatedAt:      now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	service := &Service{
		journal: store,
		now:     func() time.Time { return now },
	}
	return service, store, run
}

func toolResultTestHook(
	t *testing.T,
	service *Service,
	runID string,
) driver.ToolResultHook {
	t.Helper()
	prepared := preparedDriverDispatch{
		request: driver.Request{Role: driver.RoleImplementer},
		productionContext: &productionWorkContext{
			Track: "T1-telemetry",
		},
	}
	coordinates := dispatchCoordinates{
		Slice:           "S8-tool-result-observation",
		Responsibility:  driver.ImplementerImplementation,
		ProtocolAttempt: 2,
		Epoch:           1,
		Try:             3,
	}
	attemptIdentity := journal.EffectAttempt{
		WorkID: "work-tool-result-events",
		Epoch:  1,
		Try:    3,
	}
	hook := service.toolResultObservationHook(
		journal.OwnerLease{RunID: runID},
		prepared,
		coordinates,
		attemptIdentity,
	)
	if hook == nil {
		t.Fatal("hook must exist for a live journal")
	}
	return hook
}

// TestLastInputTokensForSnapshotReadsLatestReportedValuePerEffect anchors
// A4: the latest turn's reported input-token count projects per effect ID
// regardless of the dispatch's current state (in flight or failed alike -
// this helper reads only the journaled events, never the effect state),
// a later part carrying none never erases an earlier part's value, and a
// later turn's report supersedes an earlier one.
func TestLastInputTokensForSnapshotReadsLatestReportedValuePerEffect(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	hook := toolResultTestHook(t, service, run.ID)
	ctx := context.Background()
	effectID := journal.AttemptEffectID("work-tool-result-events", 1, 3)

	first := int64(4_200)
	if err := hook(ctx, driver.ToolResultTurn{
		Turn:        1,
		InputTokens: &first,
		Results: []driver.ToolResultRecord{{
			Sequence: 1, ToolCallID: "call-1", Tool: "Read",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens := lastInputTokensForSnapshot(snapshot)
	if tokens[effectID] == nil || *tokens[effectID] != first {
		t.Fatalf("tokens[%s] = %#v, want *4200", effectID, tokens[effectID])
	}

	// A later part with no InputTokens (every part but the first of a
	// coalesced turn) never erases the value the first part carried.
	if err := hook(ctx, driver.ToolResultTurn{
		Turn: 1, Part: 2, Parts: 2,
		Results: []driver.ToolResultRecord{{
			Sequence: 2, ToolCallID: "call-2", Tool: "Read",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens = lastInputTokensForSnapshot(snapshot)
	if tokens[effectID] == nil || *tokens[effectID] != first {
		t.Fatalf("tokens[%s] after part 2 = %#v, want unchanged *4200", effectID, tokens[effectID])
	}

	// A later turn's own report supersedes the earlier one.
	second := int64(9_900)
	if err := hook(ctx, driver.ToolResultTurn{
		Turn:        3,
		InputTokens: &second,
		Results: []driver.ToolResultRecord{{
			Sequence: 1, ToolCallID: "call-3", Tool: "Read",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokens = lastInputTokensForSnapshot(snapshot)
	if tokens[effectID] == nil || *tokens[effectID] != second {
		t.Fatalf("tokens[%s] after turn 3 = %#v, want *9900", effectID, tokens[effectID])
	}
}

func TestToolResultObservationHookJournalsIdentityAndExactBytes(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	hook := toolResultTestHook(t, service, run.ID)
	ctx := context.Background()

	content := []byte("hello worker")
	turn := driver.ToolResultTurn{
		Turn:          7,
		Part:          2,
		Parts:         3,
		DroppedEvents: 4,
		Results: []driver.ToolResultRecord{{
			Sequence:   1,
			ToolCallID: "call-1",
			Tool:       "Read",
			TotalBytes: int64(len(content)),
			Head:       base64.StdEncoding.EncodeToString(content),
		}},
	}
	if err := hook(ctx, turn); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var eventBody []byte
	var eventOffset int64
	for _, event := range snapshot.Events {
		if event.Kind == "tool_result_observed" {
			eventBody = event.Body
			eventOffset = event.Offset
		}
	}
	if len(eventBody) == 0 {
		t.Fatalf("events = %#v, want a tool_result_observed event", snapshot.Events)
	}
	var body struct {
		SchemaVersion  string                    `json:"schema_version"`
		RunID          string                    `json:"run_id"`
		Track          string                    `json:"track"`
		Slice          string                    `json:"slice"`
		Role           driver.Role               `json:"role"`
		Responsibility driver.Responsibility     `json:"responsibility"`
		Attempt        int64                     `json:"attempt"`
		Epoch          int64                     `json:"epoch"`
		Try            int64                     `json:"try"`
		WorkID         string                    `json:"work_id"`
		EffectID       string                    `json:"effect_id"`
		Turn           int64                     `json:"turn"`
		Part           int64                     `json:"part"`
		Parts          int64                     `json:"parts"`
		DroppedEvents  int64                     `json:"dropped_events"`
		Encoding       string                    `json:"encoding"`
		Results        []driver.ToolResultRecord `json:"results"`
	}
	if err := json.Unmarshal(eventBody, &body); err != nil {
		t.Fatalf("event body does not decode: %v (%s)", err, eventBody)
	}
	if body.SchemaVersion != "sworn.tool-result-turn/v1" ||
		body.RunID != run.ID ||
		body.Track != "T1-telemetry" ||
		body.Slice != "S8-tool-result-observation" ||
		body.Role != driver.RoleImplementer ||
		body.Responsibility != driver.ImplementerImplementation ||
		body.Attempt != 2 || body.Epoch != 1 || body.Try != 3 ||
		body.WorkID != "work-tool-result-events" ||
		body.EffectID != "attempt/work-tool-result-events/e1/t3" ||
		body.Turn != 7 || body.Part != 2 || body.Parts != 3 ||
		body.DroppedEvents != 4 || body.Encoding != "base64" {
		t.Fatalf("identity = %s", eventBody)
	}
	if len(body.Results) != 1 {
		t.Fatalf("results = %#v", body.Results)
	}
	head, err := base64.StdEncoding.DecodeString(body.Results[0].Head)
	if err != nil || string(head) != string(content) {
		t.Fatalf("decoded head = %q, %v", head, err)
	}

	// The same fields the cockpit enrichment parses as EventAssociation
	// ride on the event verbatim.
	var association EventAssociation
	if err := json.Unmarshal(eventBody, &association); err != nil {
		t.Fatal(err)
	}
	if association.EffectID != "attempt/work-tool-result-events/e1/t3" ||
		association.WorkID != "work-tool-result-events" ||
		association.Track != "T1-telemetry" ||
		association.Slice != "S8-tool-result-observation" {
		t.Fatalf("association = %#v", association)
	}

	// Replay from the existing machinery yields the same stream.
	window, err := store.ReadWindow(ctx, run.ID, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	replayed := false
	for _, event := range window.Snapshot.Events {
		if event.Offset == eventOffset {
			replayed = true
			if event.Kind != "tool_result_observed" ||
				event.BodyDigest != driver.Digest(eventBody) ||
				string(event.Body) != string(eventBody) {
				t.Fatalf("replayed event = %#v", event)
			}
		}
	}
	if !replayed {
		t.Fatal("replay window lacks the tool_result_observed event")
	}
	facts, err := store.EventsAfter(ctx, run.ID, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range facts.Events {
		if fact.Offset == eventOffset && fact.Kind == "tool_result_observed" {
			found = true
		}
	}
	if !found {
		t.Fatal("EventsAfter lacks the tool_result_observed kind")
	}
}

func TestToolResultEventWorstCasePartStaysUnderJournalEventBytes(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	hook := toolResultTestHook(t, service, run.ID)
	ctx := context.Background()

	headBytes := []byte(strings.Repeat("m", driver.MaxToolResultHeadBytes))
	tailBytes := []byte(strings.Repeat("n", driver.MaxToolResultTailBytes))
	records := make([]driver.ToolResultRecord, 0, 21)
	for index := 0; index < 21; index++ {
		records = append(records, driver.ToolResultRecord{
			Sequence:   int64(index + 1),
			ToolCallID: strings.Repeat("i", 256),
			Tool:       "Bash",
			TotalBytes: int64(len(headBytes) + len(tailBytes)),
			Head:       base64.StdEncoding.EncodeToString(headBytes),
			Tail:       base64.StdEncoding.EncodeToString(tailBytes),
		})
	}
	if err := hook(ctx, driver.ToolResultTurn{Turn: 3, Results: records}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var worst []byte
	for _, event := range snapshot.Events {
		if event.Kind == "tool_result_observed" {
			worst = event.Body
		}
	}
	if len(worst) == 0 {
		t.Fatal("no tool_result_observed event")
	}
	if len(worst) >= journal.MaxEventBytes {
		t.Fatalf("worst-case part = %d bytes, journal bound %d",
			len(worst), journal.MaxEventBytes)
	}
}

func workerTurnTestHook(
	t *testing.T,
	service *Service,
	runID string,
) driver.WorkerTurnHook {
	t.Helper()
	prepared := preparedDriverDispatch{
		request: driver.Request{Role: driver.RoleImplementer},
		productionContext: &productionWorkContext{
			Track: "T1-worker-observability",
		},
	}
	coordinates := dispatchCoordinates{
		Slice:           "S1-native-turn-journal",
		Responsibility:  driver.ImplementerImplementation,
		ProtocolAttempt: 2,
		Epoch:           1,
		Try:             3,
	}
	attemptIdentity := journal.EffectAttempt{
		WorkID: "work-worker-turn-events",
		Epoch:  1,
		Try:    3,
	}
	hook := service.workerTurnObservationHook(
		journal.OwnerLease{RunID: runID},
		prepared,
		coordinates,
		attemptIdentity,
	)
	if hook == nil {
		t.Fatal("hook must exist for a live journal")
	}
	return hook
}

// TestWorkerTurnObservationHookJournalsIdentityAndExactBytes mirrors
// TestToolResultObservationHookJournalsIdentityAndExactBytes for the new
// worker_turn_observed kind (S1-native-turn-journal, A3): the identity
// envelope, base64 encoding and replay/EventsAfter machinery are identical,
// under a new, separately named kind and schema version.
func TestWorkerTurnObservationHookJournalsIdentityAndExactBytes(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	hook := workerTurnTestHook(t, service, run.ID)
	ctx := context.Background()

	content := []byte("hello worker")
	turn := driver.WorkerTurn{
		Turn:          7,
		Part:          2,
		Parts:         3,
		DroppedEvents: 4,
		Content: []driver.WorkerTurnPart{{
			Kind:       driver.WorkerTurnPartText,
			TotalBytes: int64(len(content)),
			Head:       base64.StdEncoding.EncodeToString(content),
		}},
	}
	if err := hook(ctx, turn); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var eventBody []byte
	var eventOffset int64
	for _, event := range snapshot.Events {
		if event.Kind == "worker_turn_observed" {
			eventBody = event.Body
			eventOffset = event.Offset
		}
	}
	if len(eventBody) == 0 {
		t.Fatalf("events = %#v, want a worker_turn_observed event", snapshot.Events)
	}
	var body struct {
		SchemaVersion  string                  `json:"schema_version"`
		RunID          string                  `json:"run_id"`
		Track          string                  `json:"track"`
		Slice          string                  `json:"slice"`
		Role           driver.Role             `json:"role"`
		Responsibility driver.Responsibility   `json:"responsibility"`
		Attempt        int64                   `json:"attempt"`
		Epoch          int64                   `json:"epoch"`
		Try            int64                   `json:"try"`
		WorkID         string                  `json:"work_id"`
		EffectID       string                  `json:"effect_id"`
		Turn           int64                   `json:"turn"`
		Part           int64                   `json:"part"`
		Parts          int64                   `json:"parts"`
		DroppedEvents  int64                   `json:"dropped_events"`
		Encoding       string                  `json:"encoding"`
		Content        []driver.WorkerTurnPart `json:"content"`
	}
	if err := json.Unmarshal(eventBody, &body); err != nil {
		t.Fatalf("event body does not decode: %v (%s)", err, eventBody)
	}
	if body.SchemaVersion != "sworn.worker-turn/v1" ||
		body.RunID != run.ID ||
		body.Track != "T1-worker-observability" ||
		body.Slice != "S1-native-turn-journal" ||
		body.Role != driver.RoleImplementer ||
		body.Responsibility != driver.ImplementerImplementation ||
		body.Attempt != 2 || body.Epoch != 1 || body.Try != 3 ||
		body.WorkID != "work-worker-turn-events" ||
		body.EffectID != "attempt/work-worker-turn-events/e1/t3" ||
		body.Turn != 7 || body.Part != 2 || body.Parts != 3 ||
		body.DroppedEvents != 4 || body.Encoding != "base64" {
		t.Fatalf("identity = %s", eventBody)
	}
	if len(body.Content) != 1 || body.Content[0].Kind != driver.WorkerTurnPartText {
		t.Fatalf("content = %#v", body.Content)
	}
	head, err := base64.StdEncoding.DecodeString(body.Content[0].Head)
	if err != nil || string(head) != string(content) {
		t.Fatalf("decoded head = %q, %v", head, err)
	}

	var association EventAssociation
	if err := json.Unmarshal(eventBody, &association); err != nil {
		t.Fatal(err)
	}
	if association.EffectID != "attempt/work-worker-turn-events/e1/t3" ||
		association.WorkID != "work-worker-turn-events" ||
		association.Track != "T1-worker-observability" ||
		association.Slice != "S1-native-turn-journal" {
		t.Fatalf("association = %#v", association)
	}

	window, err := store.ReadWindow(ctx, run.ID, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	replayed := false
	for _, event := range window.Snapshot.Events {
		if event.Offset == eventOffset {
			replayed = true
			if event.Kind != "worker_turn_observed" ||
				event.BodyDigest != driver.Digest(eventBody) ||
				string(event.Body) != string(eventBody) {
				t.Fatalf("replayed event = %#v", event)
			}
		}
	}
	if !replayed {
		t.Fatal("replay window lacks the worker_turn_observed event")
	}
	facts, err := store.EventsAfter(ctx, run.ID, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range facts.Events {
		if fact.Offset == eventOffset && fact.Kind == "worker_turn_observed" {
			found = true
		}
	}
	if !found {
		t.Fatal("EventsAfter lacks the worker_turn_observed kind")
	}
}

func TestWorkerTurnEventWorstCasePartStaysUnderJournalEventBytes(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	hook := workerTurnTestHook(t, service, run.ID)
	ctx := context.Background()

	headBytes := []byte(strings.Repeat("m", driver.MaxToolResultHeadBytes))
	tailBytes := []byte(strings.Repeat("n", driver.MaxToolResultTailBytes))
	parts := make([]driver.WorkerTurnPart, 0, 21)
	for index := 0; index < 21; index++ {
		parts = append(parts, driver.WorkerTurnPart{
			Kind:       driver.WorkerTurnPartToolCall,
			Tool:       "Bash",
			ToolCallID: strings.Repeat("i", 256),
			TotalBytes: int64(len(headBytes) + len(tailBytes)),
			Head:       base64.StdEncoding.EncodeToString(headBytes),
			Tail:       base64.StdEncoding.EncodeToString(tailBytes),
		})
	}
	if err := hook(ctx, driver.WorkerTurn{Turn: 3, Content: parts}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var worst []byte
	for _, event := range snapshot.Events {
		if event.Kind == "worker_turn_observed" {
			worst = event.Body
		}
	}
	if len(worst) == 0 {
		t.Fatal("no worker_turn_observed event")
	}
	if len(worst) >= journal.MaxEventBytes {
		t.Fatalf("worst-case part = %d bytes, journal bound %d",
			len(worst), journal.MaxEventBytes)
	}
}

func TestPreparedInvocationCarriesWorkerTurnHook(t *testing.T) {
	h := driver.WorkerTurnHook(func(
		context.Context, driver.WorkerTurn,
	) error {
		return nil
	})
	invocation := preparedInvocation(
		preparedDriverDispatch{workerTurnHook: h},
		nil,
		driver.Request{},
		driver.SubmissionPermission{},
		nil,
	)
	if invocation.WorkerTurnHook == nil {
		t.Fatal("preparedInvocation must copy the worker-turn hook")
	}
	plain := preparedInvocation(
		preparedDriverDispatch{},
		nil,
		driver.Request{},
		driver.SubmissionPermission{},
		nil,
	)
	if plain.WorkerTurnHook != nil {
		t.Fatal("a nil hook must stay nil")
	}
}

func TestPreparedInvocationCarriesToolResultHook(t *testing.T) {
	h := driver.ToolResultHook(func(
		context.Context, driver.ToolResultTurn,
	) error {
		return nil
	})
	invocation := preparedInvocation(
		preparedDriverDispatch{toolResultHook: h},
		nil,
		driver.Request{},
		driver.SubmissionPermission{},
		nil,
	)
	if invocation.ToolResultHook == nil {
		t.Fatal("preparedInvocation must copy the tool-result hook")
	}
	plain := preparedInvocation(
		preparedDriverDispatch{},
		nil,
		driver.Request{},
		driver.SubmissionPermission{},
		nil,
	)
	if plain.ToolResultHook != nil {
		t.Fatal("a nil hook must stay nil")
	}
}

type fakeActivityTap struct {
	events []ActivityTapEvent
	drops  []string
	panic  bool
}

func (f *fakeActivityTap) ObserveActivity(event ActivityTapEvent) {
	if f.panic {
		panic("tap panic must never fail the dispatch")
	}
	f.events = append(f.events, ActivityTapEvent{
		RunID: event.RunID, EffectID: event.EffectID, Offset: event.Offset,
		Kind: event.Kind, Body: append([]byte(nil), event.Body...), CreatedAt: event.CreatedAt,
	})
}

func (f *fakeActivityTap) DropActivityDispatch(effectID string) {
	if f.panic {
		panic("drop panic must never fail the dispatch")
	}
	f.drops = append(f.drops, effectID)
}

// A3: the observation hooks feed the live ring only after the durable
// append and carry the durable offset, so the ring and the journal share
// one cursor. The tap never blocks or fails the dispatch, holds only the
// already-bounded journaled bytes, and is dropped when the dispatch ends.
func TestToolResultObservationHookFeedsLiveRingWithDurableOffset(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	tap := &fakeActivityTap{}
	service.SetActivityTap(tap)
	hook := toolResultTestHook(t, service, run.ID)
	ctx := context.Background()
	turn := driver.ToolResultTurn{
		Turn: 3,
		Results: []driver.ToolResultRecord{{
			Sequence: 1, ToolCallID: "call-1", Tool: "Read",
			TotalBytes: 4, Head: base64.StdEncoding.EncodeToString([]byte("data")),
		}},
	}
	if err := hook(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if len(tap.events) != 1 {
		t.Fatalf("tap events = %d, want 1", len(tap.events))
	}
	event := tap.events[0]
	if event.RunID != run.ID || event.EffectID != "attempt/work-tool-result-events/e1/t3" || event.Kind != "tool_result_observed" || event.Offset <= 0 {
		t.Fatalf("tap event = %#v", event)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var journalOffset int64
	var journalBody []byte
	for _, journalEvent := range snapshot.Events {
		if journalEvent.Kind == "tool_result_observed" {
			journalOffset = journalEvent.Offset
			journalBody = journalEvent.Body
		}
	}
	if journalOffset == 0 || event.Offset != journalOffset || string(event.Body) != string(journalBody) {
		t.Fatalf("tap offset %d body %s vs journal offset %d body %s", event.Offset, event.Body, journalOffset, journalBody)
	}
	// A panicking tap never fails the dispatch: the hook still journals.
	tap.panic = true
	if err := hook(ctx, turn); err != nil {
		t.Fatalf("hook with panicking tap = %v, want nil (journaled, tap recovered)", err)
	}
	tap.panic = false
	// Dispatch end drops the per-dispatch ring.
	service.dropActivityDispatch("attempt/work-tool-result-events/e1/t1")
	service.dropActivityDispatch("attempt/work-tool-result-events/e1/t3")
	if len(tap.drops) != 2 || tap.drops[1] != "attempt/work-tool-result-events/e1/t3" {
		t.Fatalf("drops = %#v", tap.drops)
	}
	// A nil tap disables the ring; hooks still journal.
	service.SetActivityTap(nil)
	if err := hook(ctx, turn); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerTurnObservationHookFeedsLiveRingWithDurableOffset(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	tap := &fakeActivityTap{}
	service.SetActivityTap(tap)
	prepared := preparedDriverDispatch{
		request:           driver.Request{Role: driver.RoleImplementer},
		productionContext: &productionWorkContext{Track: "T1-telemetry"},
	}
	coordinates := dispatchCoordinates{
		Slice: "S8-tool-result-observation", Responsibility: driver.ImplementerImplementation,
		ProtocolAttempt: 2, Epoch: 1, Try: 3,
	}
	attemptIdentity := journal.EffectAttempt{WorkID: "work-tool-result-events", Epoch: 1, Try: 3}
	hook := service.workerTurnObservationHook(
		journal.OwnerLease{RunID: run.ID}, prepared, coordinates, attemptIdentity,
	)
	if hook == nil {
		t.Fatal("worker hook must exist for a live journal")
	}
	ctx := context.Background()
	turn := driver.WorkerTurn{
		Turn: 5,
		Content: []driver.WorkerTurnPart{{
			Kind: driver.WorkerTurnPartText, TotalBytes: 5,
			Head: base64.StdEncoding.EncodeToString([]byte("hello")),
		}},
	}
	if err := hook(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if len(tap.events) != 1 || tap.events[0].Kind != "worker_turn_observed" || tap.events[0].Offset <= 0 {
		t.Fatalf("tap events = %#v", tap.events)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, journalEvent := range snapshot.Events {
		if journalEvent.Kind == "worker_turn_observed" && journalEvent.Offset == tap.events[0].Offset {
			if string(journalEvent.Body) != string(tap.events[0].Body) {
				t.Fatalf("ring body differs from journal body")
			}
			return
		}
	}
	t.Fatal("journal lacks the worker-turn event at the tap offset")
}

// S3-failure-turn-context: the tail holds only durably journaled
// projections, fed after AppendEventWithOffset succeeds, as deep copies
// that a later mutation of the caller's Turn cannot change.
func TestFailureTailHoldsOnlyDurablyJournaledProjections(t *testing.T) {
	service, store, run := toolResultRuntimeFixture(t)
	hook := toolResultTestHook(t, service, run.ID)
	ctx := context.Background()
	effectID := "attempt/work-tool-result-events/e1/t3"
	service.initFailureTail(effectID)
	turn := driver.ToolResultTurn{
		Turn: 1,
		Results: []driver.ToolResultRecord{{
			Sequence: 1, ToolCallID: "call-1", Tool: "Read",
			TotalBytes: 4, Head: base64.StdEncoding.EncodeToString([]byte("data")),
		}},
	}
	if err := hook(ctx, turn); err != nil {
		t.Fatal(err)
	}
	// Mutating the caller's Turn after the hook must not change the tail.
	turn.Results[0].Tool = "Mutated"
	turn.Results[0].Head = base64.StdEncoding.EncodeToString([]byte("mutated"))
	stored := service.assembleFailureContextStored(effectID)
	if stored == nil || stored.Status != FailureTurnContextPresent || len(stored.Turns) != 1 {
		t.Fatalf("stored = %#v", stored)
	}
	if stored.Turns[0].Results[0].Tool != "Read" {
		t.Fatalf("tail was not a deep copy: %#v", stored.Turns[0].Results[0])
	}
	head, err := base64.StdEncoding.DecodeString(stored.Turns[0].Results[0].Head)
	if err != nil || string(head) != "data" {
		t.Fatalf("tail head = %q, %v", head, err)
	}
	// The journal holds the same turn the tail holds.
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range snapshot.Events {
		if event.Kind == "tool_result_observed" {
			found = true
		}
	}
	if !found {
		t.Fatal("journal lacks the tool-result event the tail holds")
	}
	service.dropFailureTail(effectID)
	if again := service.assembleFailureContextStored(effectID); again.Status != FailureTurnContextUnavailable || again.Reason != FailureTurnContextNoLiveTail {
		t.Fatalf("dropped tail = %#v, want unavailable/no_live_tail", again)
	}
}

// S3: the tail keeps the newest 16 turn-events with exact omitted counts;
// the context selects the newest 5, oldest-first, with tail evictions plus
// selection counted as omitted.
func TestFailureTailBoundsNewestWinsAndOmittedExact(t *testing.T) {
	service, _, _ := toolResultRuntimeFixture(t)
	effectID := "attempt/work-bounds/e1/t1"
	service.initFailureTail(effectID)
	for turn := int64(1); turn <= 20; turn++ {
		service.feedFailureTailTool(effectID, driver.ToolResultTurn{
			Turn: turn,
			Results: []driver.ToolResultRecord{{
				Sequence: 1, ToolCallID: "call-1", Tool: "Read",
				TotalBytes: 4, Head: base64.StdEncoding.EncodeToString([]byte("data")),
			}},
		})
	}
	stored := service.assembleFailureContextStored(effectID)
	if stored.Status != FailureTurnContextPresent {
		t.Fatalf("stored status = %q", stored.Status)
	}
	if len(stored.Turns) != FailureTurnContextMaxTurns {
		t.Fatalf("turns = %d, want %d", len(stored.Turns), FailureTurnContextMaxTurns)
	}
	// Newest 5 (16..20) emitted oldest-first (increasing).
	for i, want := range []int64{16, 17, 18, 19, 20} {
		if stored.Turns[i].Turn != want {
			t.Fatalf("turns = %v, want 16..20 increasing", stored.Turns)
		}
	}
	// Tail evicted 4 (20-16), context selection omitted 11 more (16-5).
	if stored.Omitted != 15 {
		t.Fatalf("omitted = %d, want 15 (4 tail + 11 selection)", stored.Omitted)
	}
}

// S3 C2: a single worker turn may legitimately approach the 240 KiB driver
// budget, exceeding the 48 KiB context bound. Truncating by whole
// turn-events would yield turns:[] with omitted>0 for a dispatch that did
// take turns, indistinguishable from explicit empty. The context instead
// admits the newest turn with whole parts dropped and counted, keeping
// S1's per-part geometry intact, so empty only ever means "no turns
// observed".
func TestFailureContextWorstCaseSingleTurnTruncatesByWholeParts(t *testing.T) {
	service, _, _ := toolResultRuntimeFixture(t)
	effectID := "attempt/work-worst/e1/t1"
	service.initFailureTail(effectID)
	headBytes := []byte(strings.Repeat("m", driver.MaxToolResultHeadBytes))
	tailBytes := []byte(strings.Repeat("n", driver.MaxToolResultTailBytes))
	records := make([]driver.ToolResultRecord, 0, 21)
	for index := 0; index < 21; index++ {
		records = append(records, driver.ToolResultRecord{
			Sequence:   int64(index + 1),
			ToolCallID: strings.Repeat("i", 256),
			Tool:       "Bash",
			TotalBytes: int64(len(headBytes) + len(tailBytes)),
			Head:       base64.StdEncoding.EncodeToString(headBytes),
			Tail:       base64.StdEncoding.EncodeToString(tailBytes),
		})
	}
	service.feedFailureTailTool(effectID, driver.ToolResultTurn{Turn: 3, Results: records})
	stored := service.assembleFailureContextStored(effectID)
	if stored.Status != FailureTurnContextPresent {
		t.Fatalf("stored status = %q, want present (never empty with turns observed)", stored.Status)
	}
	if len(stored.Turns) != 1 {
		t.Fatalf("turns = %d, want 1 truncated newest", len(stored.Turns))
	}
	turn := stored.Turns[0]
	if turn.OmittedParts <= 0 {
		t.Fatalf("omitted_parts = %d, want >0 (whole parts dropped and counted)", turn.OmittedParts)
	}
	if len(turn.Results) == 0 || len(turn.Results) >= len(records) {
		t.Fatalf("kept %d of %d records, want a strict truncated suffix", len(turn.Results), len(records))
	}
	// Latest kept (closest to the failure): sequences are a suffix.
	firstSeq := turn.Results[0].Sequence
	if firstSeq <= 1 || firstSeq+int64(len(turn.Results))-1 != int64(len(records)) {
		t.Fatalf("kept sequences start at %d, want a latest suffix of 1..%d", firstSeq, len(records))
	}
	// No head/tail span was ever split: every kept record still carries
	// full 2 KiB head and tail raw spans.
	for _, record := range turn.Results {
		head, err := base64.StdEncoding.DecodeString(record.Head)
		if err != nil || len(head) != driver.MaxToolResultHeadBytes {
			t.Fatalf("kept head = %d bytes, want %d", len(head), driver.MaxToolResultHeadBytes)
		}
		tail, err := base64.StdEncoding.DecodeString(record.Tail)
		if err != nil || len(tail) != driver.MaxToolResultTailBytes {
			t.Fatalf("kept tail = %d bytes, want %d", len(tail), driver.MaxToolResultTailBytes)
		}
	}
	body, err := json.Marshal(stored)
	if err != nil || len(body) > FailureTurnContextMaxBytes {
		t.Fatalf("worst-case context = %d bytes, bound %d", len(body), FailureTurnContextMaxBytes)
	}
	// The full event body (association ~1 KiB + context) stays far under
	// the journal bound with exact accounting.
	event := service.failureEventBodyFor(EventAssociation{
		EffectID: effectID, WorkID: "work-worst", Slice: "S3",
	}, nil, effectID)
	if len(event) >= journal.MaxEventBytes {
		t.Fatalf("worst-case event = %d bytes, journal bound %d", len(event), journal.MaxEventBytes)
	}
	if len(event) > 64*1024 {
		t.Fatalf("worst-case event = %d bytes, want <<256 KiB (48 KiB + ~1 KiB)", len(event))
	}
}

// S3: empty only ever means "no turns observed". A live tail with no turns
// assembles to explicit empty; a dispatch that took turns never assembles
// to empty, even when its newest turn alone exceeds the byte bound (pinned
// above).
func TestFailureContextEmptyOnlyWhenNoTurns(t *testing.T) {
	service, _, _ := toolResultRuntimeFixture(t)
	emptyID := "attempt/work-empty/e1/t1"
	service.initFailureTail(emptyID)
	empty := service.assembleFailureContextStored(emptyID)
	if empty.Status != FailureTurnContextEmpty || len(empty.Turns) != 0 || empty.Omitted != 0 {
		t.Fatalf("empty tail = %#v, want explicit empty with turns:[] omitted 0", empty)
	}
	body, err := json.Marshal(empty)
	if err != nil || !strings.Contains(string(body), `"status":"empty"`) || !strings.Contains(string(body), `"turns":[]`) {
		t.Fatalf("empty JSON = %s, want explicit status empty and turns []", body)
	}
	// No tail at all (a sweep reconcile that never held the dispatch, or a
	// prior-process path) is unavailable, never empty and never absent for
	// a new write.
	missing := service.assembleFailureContextStored("attempt/work-missing/e1/t1")
	if missing.Status != FailureTurnContextUnavailable || missing.Reason != FailureTurnContextNoLiveTail {
		t.Fatalf("missing tail = %#v, want unavailable/no_live_tail", missing)
	}
}

// S3: dropped counting is max-visible, exactly as honest as the journal.
// Each observer's count is monotonic, so the newest entry's value is the
// max; a dangling drop with no later success is invisible without new
// driver transport and is documented as max-visible, never an exact total.
func TestFailureContextDroppedMaxVisible(t *testing.T) {
	service, _, _ := toolResultRuntimeFixture(t)
	effectID := "attempt/work-dropped/e1/t1"
	service.initFailureTail(effectID)
	service.feedFailureTailTool(effectID, driver.ToolResultTurn{
		Turn: 1, DroppedEvents: 2,
		Results: []driver.ToolResultRecord{{Sequence: 1, ToolCallID: "c1", Tool: "Read", TotalBytes: 1}},
	})
	service.feedFailureTailWorker(effectID, driver.WorkerTurn{
		Turn: 2, DroppedEvents: 5,
		Content: []driver.WorkerTurnPart{{Kind: driver.WorkerTurnPartText, TotalBytes: 1}},
	})
	stored := service.assembleFailureContextStored(effectID)
	if stored.DroppedMaxVisible != 5 {
		t.Fatalf("dropped_max_visible = %d, want 5 (max across retained tail)", stored.DroppedMaxVisible)
	}
	if stored.Turns[0].DroppedEvents != 2 || stored.Turns[1].DroppedEvents != 5 {
		t.Fatalf("per-turn dropped = %d, %d", stored.Turns[0].DroppedEvents, stored.Turns[1].DroppedEvents)
	}
}
