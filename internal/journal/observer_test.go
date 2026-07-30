package journal

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestReadWindowPagesOneConsistentRuntimeObservation(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	for index, body := range [][]byte{
		[]byte(`{"step":1}`),
		[]byte(`{"step":2}`),
		[]byte(`{"step":3}`),
	} {
		if err := store.AppendEvent(
			ctx,
			run.ID,
			"runtime_progress",
			body,
			now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID,
		ID:    "pause-observation",
		Kind:  Pause,
	}, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(
		ctx,
		run.ID,
		now.Add(5*time.Second),
		time.Minute,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.ReadWindow(ctx, run.ID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMoreEvents || len(first.Snapshot.Events) != 2 ||
		first.ThroughEventOffset != first.Snapshot.Events[1].Offset {
		t.Fatalf("first window = %#v", first)
	}
	if first.Control.Generation != 1 || first.Control.Desired != "paused" {
		t.Fatalf("first control = %#v", first.Control)
	}
	if !first.OwnerPresent || first.Owner != owner {
		t.Fatalf("first owner = %#v, present=%t", first.Owner, first.OwnerPresent)
	}

	seen := append([]Event(nil), first.Snapshot.Events...)
	current := first
	for current.HasMoreEvents {
		next, err := store.ReadWindow(
			ctx,
			run.ID,
			current.ThroughEventOffset,
			2,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(next.Snapshot.Events) == 0 ||
			next.Snapshot.Events[0].Offset <= current.ThroughEventOffset {
			t.Fatalf("next window repeated or skipped progress: %#v", next)
		}
		if next.Control.Generation != first.Control.Generation ||
			next.Control.Desired != first.Control.Desired ||
			!next.OwnerPresent || next.Owner != first.Owner {
			t.Fatalf("runtime overlay changed across idle pages: %#v", next)
		}
		seen = append(seen, next.Snapshot.Events...)
		current = next
	}
	if len(seen) < 3 ||
		current.ThroughEventOffset != seen[len(seen)-1].Offset {
		t.Fatalf("complete observation = events=%#v, window=%#v", seen, current)
	}
}

func TestObserverAdvanceIsAtomicReplaySafeAndOrdered(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	for index := 0; index < 3; index++ {
		if err := store.AppendEvent(
			ctx,
			run.ID,
			"runtime_progress",
			[]byte{byte('1' + index)},
			now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Events) != 3 {
		t.Fatalf("events = %#v", snapshot.Events)
	}
	firstOffset := snapshot.Events[0].Offset
	secondOffset := snapshot.Events[1].Offset
	thirdOffset := snapshot.Events[2].Offset

	first := ObserverAdvance{
		RunID:          run.ID,
		Observer:       "operator-projection",
		ExpectedOffset: 0,
		ThroughOffset:  secondOffset,
		Eval: []EvalDraft{{
			SourceEventOffset: firstOffset,
			ID:                "eval-1",
			Body:              []byte(`{"outcome":"progress"}`),
		}},
		Notifications: []NotificationDraft{{
			DestinationID:     "webhook-primary",
			SourceEventOffset: secondOffset,
			MessageID:         "message-1",
			Body:              []byte(`{"event":"handoff"}`),
		}},
		At: now.Add(4 * time.Second),
	}
	if err := store.AdvanceObserver(ctx, first); err != nil {
		t.Fatal(err)
	}
	replay := first
	replay.At = replay.At.Add(time.Hour)
	if err := store.AdvanceObserver(ctx, replay); err != nil {
		t.Fatalf("exact replay = %v", err)
	}
	conflict := replay
	conflict.Eval = append([]EvalDraft(nil), conflict.Eval...)
	conflict.Eval[0].Body = []byte(`{"outcome":"different"}`)
	if err := store.AdvanceObserver(ctx, conflict); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("conflicting replay = %v", err)
	}
	omitted := replay
	omitted.Eval = nil
	if err := store.AdvanceObserver(ctx, omitted); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("incomplete replay = %v", err)
	}

	cursor, err := store.ObserverCursor(ctx, run.ID, first.Observer)
	if err != nil || cursor != secondOffset {
		t.Fatalf("cursor = %d, %v", cursor, err)
	}
	records, err := store.EvalRecords(
		ctx,
		run.ID,
		first.Observer,
		0,
		MaxObserverItems,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "eval-1" ||
		!bytes.Equal(records[0].Body, first.Eval[0].Body) {
		t.Fatalf("eval records = %#v", records)
	}
	notifications, err := store.Notifications(
		ctx,
		run.ID,
		"webhook-primary",
		MaxObserverItems,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 || notifications[0].Sequence != 1 ||
		notifications[0].State != NotificationPending {
		t.Fatalf("notifications = %#v", notifications)
	}

	missing := ObserverAdvance{
		RunID:          run.ID,
		Observer:       first.Observer,
		ExpectedOffset: secondOffset,
		ThroughOffset:  thirdOffset + 100,
		Eval: []EvalDraft{{
			SourceEventOffset: thirdOffset + 100,
			ID:                "eval-missing",
			Body:              []byte(`{"outcome":"missing"}`),
		}},
		At: now.Add(5 * time.Second),
	}
	if err := store.AdvanceObserver(ctx, missing); !IsCode(err, "EVENT_NOT_FOUND") {
		t.Fatalf("missing source event = %v", err)
	}
	cursor, err = store.ObserverCursor(ctx, run.ID, first.Observer)
	if err != nil || cursor != secondOffset {
		t.Fatalf("cursor after rollback = %d, %v", cursor, err)
	}
	records, err = store.EvalRecords(
		ctx,
		run.ID,
		first.Observer,
		0,
		MaxObserverItems,
	)
	if err != nil || len(records) != 1 {
		t.Fatalf("records after rollback = %#v, %v", records, err)
	}
	missing.Eval = nil
	if err := store.AdvanceObserver(ctx, missing); !IsCode(err, "EVENT_NOT_FOUND") {
		t.Fatalf("empty batch missing through event = %v", err)
	}

	second := ObserverAdvance{
		RunID:          run.ID,
		Observer:       first.Observer,
		ExpectedOffset: secondOffset,
		ThroughOffset:  thirdOffset,
		Notifications: []NotificationDraft{{
			DestinationID:     "webhook-primary",
			SourceEventOffset: thirdOffset,
			MessageID:         "message-2",
			Body:              []byte(`{"event":"verified"}`),
		}},
		At: now.Add(6 * time.Second),
	}
	if err := store.AdvanceObserver(ctx, second); err != nil {
		t.Fatal(err)
	}
	notifications, err = store.Notifications(
		ctx,
		run.ID,
		"webhook-primary",
		MaxObserverItems,
	)
	if err != nil || len(notifications) != 2 ||
		notifications[1].Sequence != 2 {
		t.Fatalf("ordered notifications = %#v, %v", notifications, err)
	}
}

func TestNotificationDeliveryUsesLeasesAndBoundedRedelivery(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	for index := 0; index < 2; index++ {
		if err := store.AppendEvent(
			ctx,
			run.ID,
			"runtime_progress",
			[]byte{byte('1' + index)},
			now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AdvanceObserver(ctx, ObserverAdvance{
		RunID:          run.ID,
		Observer:       "webhook-projector",
		ExpectedOffset: 0,
		ThroughOffset:  snapshot.Events[1].Offset,
		Notifications: []NotificationDraft{
			{
				DestinationID:     "webhook-primary",
				SourceEventOffset: snapshot.Events[1].Offset,
				MessageID:         "message-2",
				Body:              []byte(`{"event":"two"}`),
			},
			{
				DestinationID:     "webhook-primary",
				SourceEventOffset: snapshot.Events[0].Offset,
				MessageID:         "message-1",
				Body:              []byte(`{"event":"one"}`),
			},
		},
		At: now.Add(3 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	claimAt := now.Add(4 * time.Second)
	first, found, err := store.ClaimNotification(
		ctx,
		"webhook-primary",
		claimAt,
		time.Second,
	)
	if err != nil || !found || first.Notification.Sequence != 1 ||
		first.Notification.MessageID != "message-1" ||
		first.Notification.Attempts != 1 {
		t.Fatalf("first claim = %#v, found=%t, %v", first, found, err)
	}
	if _, found, err := store.ClaimNotification(
		ctx,
		"webhook-primary",
		claimAt.Add(500*time.Millisecond),
		time.Second,
	); err != nil || found {
		t.Fatalf("active lease claim = found=%t, %v", found, err)
	}
	retryAt := claimAt.Add(2 * time.Second)
	if err := store.FinishNotification(
		ctx,
		first,
		NotificationRetry,
		retryAt,
		"HTTP_503",
		claimAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ClaimNotification(
		ctx,
		"webhook-primary",
		retryAt.Add(-time.Nanosecond),
		time.Second,
	); err != nil || found {
		t.Fatalf("early retry claim = found=%t, %v", found, err)
	}
	first, found, err = store.ClaimNotification(
		ctx,
		"webhook-primary",
		retryAt,
		time.Second,
	)
	if err != nil || !found || first.Notification.Attempts != 2 {
		t.Fatalf("retry claim = %#v, found=%t, %v", first, found, err)
	}
	if err := store.FinishNotification(
		ctx,
		first,
		NotificationSucceeded,
		time.Time{},
		"",
		retryAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}

	second, found, err := store.ClaimNotification(
		ctx,
		"webhook-primary",
		retryAt.Add(2*time.Second),
		time.Second,
	)
	if err != nil || !found || second.Notification.Sequence != 2 ||
		second.Notification.MessageID != "message-2" ||
		second.Notification.Attempts != 1 {
		t.Fatalf("second claim = %#v, found=%t, %v", second, found, err)
	}
	for attempt := int64(2); attempt <= MaxNotificationAttempts; attempt++ {
		second, found, err = store.ClaimNotification(
			ctx,
			"webhook-primary",
			second.ExpiresAt,
			time.Second,
		)
		if err != nil || !found || second.Notification.Attempts != attempt {
			t.Fatalf(
				"expired claim %d = %#v, found=%t, %v",
				attempt,
				second,
				found,
				err,
			)
		}
	}
	if _, found, err := store.ClaimNotification(
		ctx,
		"webhook-primary",
		second.ExpiresAt,
		time.Second,
	); err != nil || found {
		t.Fatalf("exhausted claim = found=%t, %v", found, err)
	}
	notifications, err := store.Notifications(
		ctx,
		run.ID,
		"webhook-primary",
		MaxObserverItems,
	)
	if err != nil {
		t.Fatal(err)
	}
	if notifications[0].State != NotificationDelivered ||
		notifications[1].State != NotificationDead ||
		notifications[1].Attempts != MaxNotificationAttempts {
		t.Fatalf("terminal notifications = %#v", notifications)
	}

	redeliverAt := second.ExpiresAt.Add(time.Second)
	if err := store.RedeliverNotification(
		ctx,
		run.ID,
		"webhook-primary",
		"message-2",
		redeliverAt,
	); err != nil {
		t.Fatal(err)
	}
	second, found, err = store.ClaimNotification(
		ctx,
		"webhook-primary",
		redeliverAt,
		time.Second,
	)
	if err != nil || !found || second.Notification.Attempts != 1 {
		t.Fatalf("redelivered claim = %#v, found=%t, %v", second, found, err)
	}
	if err := store.FinishNotification(
		ctx,
		second,
		NotificationSucceeded,
		time.Time{},
		"",
		redeliverAt.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	notifications, err = store.Notifications(
		ctx,
		run.ID,
		"webhook-primary",
		MaxObserverItems,
	)
	if err != nil || notifications[1].State != NotificationDelivered ||
		notifications[1].Attempts != 1 {
		t.Fatalf("redelivered notifications = %#v, %v", notifications, err)
	}
}

func TestNotificationClaimSerializesOneDestinationAcrossRuns(t *testing.T) {
	t.Parallel()

	store, firstRun, _, _ := journalFixture(t)
	ctx := context.Background()
	secondRun := firstRun
	secondRun.ID = "run-2"
	secondRun.ManifestDigest = digest([]byte("manifest-2"))
	secondRun.Release = "release-2"
	secondRun.CreatedAt = firstRun.CreatedAt.Add(time.Second)
	if err := store.RegisterRun(ctx, secondRun); err != nil {
		t.Fatal(err)
	}
	for index, run := range []Run{firstRun, secondRun} {
		at := secondRun.CreatedAt.Add(time.Duration(index+1) * time.Second)
		if err := store.AppendEvent(
			ctx,
			run.ID,
			"runtime_progress",
			[]byte(run.ID),
			at,
		); err != nil {
			t.Fatal(err)
		}
		window, err := store.EventsAfter(ctx, run.ID, 0, 1)
		if err != nil || len(window.Events) != 1 {
			t.Fatalf("%s events = %#v, %v", run.ID, window, err)
		}
		if err := store.AdvanceObserver(ctx, ObserverAdvance{
			RunID: run.ID, Observer: "webhook.shared",
			ExpectedOffset: 0,
			ThroughOffset:  window.Through,
			Notifications: []NotificationDraft{{
				DestinationID:     "shared",
				SourceEventOffset: window.Through,
				MessageID:         "message-" + run.ID,
				Body:              []byte(`{"safe":true}`),
			}},
			At: at.Add(time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	firstItems, err := store.Notifications(
		ctx,
		firstRun.ID,
		"shared",
		1,
	)
	if err != nil || len(firstItems) != 1 || firstItems[0].Sequence != 1 {
		t.Fatalf("first run outbox = %#v, %v", firstItems, err)
	}
	secondItems, err := store.Notifications(
		ctx,
		secondRun.ID,
		"shared",
		1,
	)
	if err != nil || len(secondItems) != 1 || secondItems[0].Sequence != 2 {
		t.Fatalf("second run outbox = %#v, %v", secondItems, err)
	}

	now := secondRun.CreatedAt.Add(10 * time.Second)
	first, found, err := store.ClaimNotification(
		ctx,
		"shared",
		now,
		time.Minute,
	)
	if err != nil || !found ||
		first.Notification.RunID != firstRun.ID ||
		first.Notification.Sequence != 1 {
		t.Fatalf("first global claim = %#v, found=%t, %v", first, found, err)
	}
	if _, found, err := store.ClaimNotification(
		ctx,
		"shared",
		now.Add(time.Second),
		time.Minute,
	); err != nil || found {
		t.Fatalf("concurrent later-run claim = found=%t, %v", found, err)
	}
	if err := store.FinishNotification(
		ctx,
		first,
		NotificationSucceeded,
		time.Time{},
		"",
		now.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	second, found, err := store.ClaimNotification(
		ctx,
		"shared",
		now.Add(3*time.Second),
		time.Minute,
	)
	if err != nil || !found ||
		second.Notification.RunID != secondRun.ID ||
		second.Notification.Sequence != 2 {
		t.Fatalf("second global claim = %#v, found=%t, %v", second, found, err)
	}
}

func TestEvalPagingIsObserverScopedAtSharedSourceOffsets(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	for index := 0; index < 2; index++ {
		if err := store.AppendEvent(
			ctx,
			run.ID,
			"runtime_progress",
			[]byte{byte('1' + index)},
			now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, observer := range []string{"eval-primary", "eval-secondary"} {
		if err := store.AdvanceObserver(ctx, ObserverAdvance{
			RunID: run.ID, Observer: observer,
			ExpectedOffset: 0,
			ThroughOffset:  snapshot.Events[1].Offset,
			Eval: []EvalDraft{
				{
					SourceEventOffset: snapshot.Events[1].Offset,
					ID:                observer + "-2",
					Body:              []byte(`{"kind":"outcome"}`),
				},
				{
					SourceEventOffset: snapshot.Events[0].Offset,
					ID:                observer + "-1",
					Body:              []byte(`{"kind":"attempt"}`),
				},
			},
			At: now.Add(3 * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.EvalRecords(
		ctx,
		run.ID,
		"eval-primary",
		0,
		1,
	)
	if err != nil || len(first) != 1 ||
		first[0].SourceEventOffset != snapshot.Events[0].Offset {
		t.Fatalf("first eval page = %#v, %v", first, err)
	}
	second, err := store.EvalRecords(
		ctx,
		run.ID,
		"eval-primary",
		first[0].SourceEventOffset,
		1,
	)
	if err != nil || len(second) != 1 ||
		second[0].SourceEventOffset != snapshot.Events[1].Offset {
		t.Fatalf("second eval page = %#v, %v", second, err)
	}
	other, err := store.EvalRecords(
		ctx,
		run.ID,
		"eval-secondary",
		0,
		MaxObserverItems,
	)
	if err != nil || len(other) != 2 ||
		other[0].Observer != "eval-secondary" {
		t.Fatalf("other observer page = %#v, %v", other, err)
	}
}

func TestObserverAndOutboxReadsRejectMissingRun(t *testing.T) {
	t.Parallel()

	store, _, _, _ := journalFixture(t)
	ctx := context.Background()
	for name, operation := range map[string]func() error{
		"cursor": func() error {
			_, err := store.ObserverCursor(ctx, "missing-run", "eval-primary")
			return err
		},
		"eval": func() error {
			_, err := store.EvalRecords(
				ctx,
				"missing-run",
				"eval-primary",
				0,
				1,
			)
			return err
		},
		"notifications": func() error {
			_, err := store.Notifications(
				ctx,
				"missing-run",
				"webhook-primary",
				1,
			)
			return err
		},
	} {
		if err := operation(); !IsCode(err, "RUN_NOT_FOUND") {
			t.Errorf("%s missing run = %v", name, err)
		}
	}
	if _, found, err := store.ClaimNotification(
		ctx,
		"webhook-primary",
		time.Unix(1_700_000_000, 0).UTC(),
		time.Second,
	); err != nil || found {
		t.Fatalf("empty destination claim = found=%t, %v", found, err)
	}
}
