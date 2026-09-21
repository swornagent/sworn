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
