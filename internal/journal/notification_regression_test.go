package journal

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestCanonicalTimeIsFixedWidthAndLexicallySortable(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_700_000_000, 0).UTC()
	zero, err := canonicalTime(base)
	if err != nil {
		t.Fatal(err)
	}
	later, err := canonicalTime(base.Add(100 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if zero != "2023-11-14T22:13:20.000000000Z" ||
		later != "2023-11-14T22:13:20.100000000Z" ||
		zero >= later {
		t.Fatalf("canonical times = %q, %q", zero, later)
	}
}

func TestLatestNotificationWindowOrdersSameSecondExactly(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	const notifications = MaxObservationNotifications + 1
	second := run.CreatedAt.Add(time.Second).Truncate(time.Second)
	older, _ := canonicalTime(second)
	newer, _ := canonicalTime(second.Add(100 * time.Millisecond))
	if err := store.immediate(ctx, func(conn *sql.Conn) error {
		for index := 1; index <= notifications; index++ {
			body := []byte(fmt.Sprintf(`{"index":%d}`, index))
			eventBody := []byte("{}")
			messageID := fmt.Sprintf("message-%03d", index)
			updatedAt := older
			if index == 1 {
				updatedAt = newer
			}
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO events(
				     run_id, kind, body_digest, body, created_at
				 ) VALUES(?, 'runtime_progress', ?, ?, ?)`,
				run.ID,
				digest(eventBody),
				eventBody,
				older,
			); err != nil {
				return err
			}
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO notification_outbox(
				     run_id, destination_id, source_event_offset, sequence,
				     message_id, schema_version, body_digest, body, state,
				     attempts, available_at, created_at, updated_at
				 ) VALUES(?, 'primary', ?, ?, ?, ?, ?, ?, 'pending', 0, ?, ?, ?)`,
				run.ID,
				index,
				index,
				messageID,
				NotificationSchemaVersion,
				digest(body),
				body,
				older,
				older,
				updatedAt,
			); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	observation, err := store.ReadObservation(ctx, run.ID, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.NotificationsTruncated ||
		len(observation.Notifications) != MaxObservationNotifications {
		t.Fatalf(
			"notification window = %d, truncated=%t",
			len(observation.Notifications),
			observation.NotificationsTruncated,
		)
	}
	foundNewest := false
	for _, item := range observation.Notifications {
		foundNewest = foundNewest || item.MessageID == "message-001"
	}
	if !foundNewest {
		t.Fatal("newest same-second notification was truncated")
	}
}

func TestRedeliveryMovesBehindAnAlreadyClaimedSuccessor(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	for index := 0; index < 2; index++ {
		if err := store.AppendEvent(
			ctx,
			run.ID,
			"runtime_progress",
			nil,
			now.Add(time.Duration(index)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AdvanceObserver(ctx, ObserverAdvance{
		RunID:          run.ID,
		Observer:       "webhook.primary",
		ExpectedOffset: 0,
		ThroughOffset:  2,
		Notifications: []NotificationDraft{
			{
				DestinationID:     "primary",
				SourceEventOffset: 1,
				MessageID:         "message-1",
				Body:              []byte(`{"event":1}`),
			},
			{
				DestinationID:     "primary",
				SourceEventOffset: 2,
				MessageID:         "message-2",
				Body:              []byte(`{"event":2}`),
			},
		},
		At: now.Add(2 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	first, found, err := store.ClaimNotification(
		ctx,
		"primary",
		now.Add(3*time.Second),
		time.Minute,
	)
	if err != nil || !found || first.Notification.Sequence != 1 {
		t.Fatalf("first claim = %#v, %t, %v", first, found, err)
	}
	if err := store.FinishNotification(
		ctx,
		first,
		NotificationAbandon,
		time.Time{},
		"HTTP_400",
		now.Add(4*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	second, found, err := store.ClaimNotification(
		ctx,
		"primary",
		now.Add(5*time.Second),
		time.Minute,
	)
	if err != nil || !found || second.Notification.Sequence != 2 {
		t.Fatalf("second claim = %#v, %t, %v", second, found, err)
	}
	if err := store.RedeliverNotification(
		ctx,
		run.ID,
		"primary",
		"message-1",
		now.Add(6*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ClaimNotification(
		ctx,
		"primary",
		now.Add(7*time.Second),
		time.Minute,
	); err != nil || found {
		t.Fatalf("claimed redelivery around successor = %t, %v", found, err)
	}
	if err := store.FinishNotification(
		ctx,
		second,
		NotificationSucceeded,
		time.Time{},
		"",
		now.Add(8*time.Second),
	); err != nil {
		t.Fatalf("successor CAS = %v", err)
	}
	redelivered, found, err := store.ClaimNotification(
		ctx,
		"primary",
		now.Add(9*time.Second),
		time.Minute,
	)
	if err != nil || !found ||
		redelivered.Notification.Sequence != 3 ||
		redelivered.Notification.MessageID != "message-1" {
		t.Fatalf("redelivered tail = %#v, %t, %v", redelivered, found, err)
	}
}
