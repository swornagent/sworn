package journal

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestToolResultObservedEventsReplayAndObserverCursor pins the additive
// tool-result event kind through the existing replay and observer_cursors
// machinery: ReadWindow and EventsAfter yield the exact same stream, and an
// AdvanceObserver batch spanning the new kind persists its cursor and
// replays idempotently.
func TestToolResultObservedEventsReplayAndObserverCursor(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	at := run.CreatedAt.Add(time.Second)
	body1 := []byte(
		`{"schema_version":"sworn.tool-result-turn/v1",` +
			`"run_id":"run-1","turn":1,"encoding":"base64","results":[]}`,
	)
	body2 := []byte(
		`{"schema_version":"sworn.tool-result-turn/v1",` +
			`"run_id":"run-1","turn":2,"encoding":"base64","results":[]}`,
	)
	if err := store.AppendEvent(
		ctx, run.ID, "tool_result_observed", body1, at,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(
		ctx, run.ID, "tool_result_observed", body2, at.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}

	window, err := store.ReadWindow(ctx, run.ID, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	var offsets []int64
	var bodies [][]byte
	for _, event := range window.Snapshot.Events {
		if event.Kind != "tool_result_observed" {
			continue
		}
		offsets = append(offsets, event.Offset)
		bodies = append(bodies, event.Body)
	}
	if len(offsets) != 2 ||
		offsets[1] <= offsets[0] ||
		!bytes.Equal(bodies[0], body1) ||
		!bytes.Equal(bodies[1], body2) {
		t.Fatalf("ReadWindow replay = offsets %v", offsets)
	}
	for index, event := range window.Snapshot.Events {
		if event.Kind != "tool_result_observed" {
			continue
		}
		if event.BodyDigest != digest(event.Body) ||
			event.RunID != run.ID {
			t.Fatalf("event %d = %#v", index, event)
		}
	}

	facts, err := store.EventsAfter(ctx, run.ID, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	factKinds := make(map[int64]string)
	for _, fact := range facts.Events {
		factKinds[fact.Offset] = fact.Kind
	}
	for _, offset := range offsets {
		if factKinds[offset] != "tool_result_observed" {
			t.Fatalf("EventsAfter kinds = %#v", factKinds)
		}
	}

	advance := ObserverAdvance{
		RunID: run.ID, Observer: "tool-result-reader",
		ExpectedOffset: 0, ThroughOffset: offsets[1],
		Notifications: []NotificationDraft{{
			DestinationID:     "tool-result-feed",
			SourceEventOffset: offsets[1],
			MessageID:         "tool-result-message-1",
			Body:              []byte(`{"turn":2}`),
		}},
		At: at.Add(2 * time.Second),
	}
	if err := store.AdvanceObserver(ctx, advance); err != nil {
		t.Fatal(err)
	}
	cursor, err := store.ObserverCursor(ctx, run.ID, "tool-result-reader")
	if err != nil || cursor != offsets[1] {
		t.Fatalf("cursor = %d, %v; want %d", cursor, err, offsets[1])
	}
	// Replaying the exact committed batch is idempotent.
	if err := store.AdvanceObserver(ctx, advance); err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	// A different batch at the same cursor is a loud conflict, never a
	// silent overwrite.
	conflict := advance
	conflict.Notifications = []NotificationDraft{{
		DestinationID:     "tool-result-feed",
		SourceEventOffset: offsets[1],
		MessageID:         "tool-result-message-1",
		Body:              []byte(`{"turn":3}`),
	}}
	if err := store.AdvanceObserver(ctx, conflict); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("conflicting replay = %v, want REPLAY_CONFLICT", err)
	}
}
