package journal

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestObservationAndEventReplayAreBoundedContentFreeSnapshots(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	usage := []byte(
		`{"token_status":"reported","input_tokens":0,"output_tokens":0,` +
			`"cost_status":"unavailable","cost_micro_units":null,` +
			`"currency":null,"source":null}`,
	)
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "transport_error",
		Attempt: &Attempt{
			Number: 1, Responsibility: "work_verification",
			TransportStatus:   "failed",
			ObservationDigest: digest([]byte("observation")),
			Usage:             usage,
		},
		EventKind: "dispatch_operational_failure",
		EventBody: []byte("private body must not project"),
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		if err := store.AppendEvent(
			ctx,
			run.ID,
			"runtime_progress",
			[]byte("another private body"),
			now.Add(time.Duration(index+2)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AdvanceObserver(ctx, ObserverAdvance{
		RunID: run.ID, Observer: "webhook.primary",
		ExpectedOffset: 0,
		ThroughOffset:  4,
		Notifications: []NotificationDraft{{
			DestinationID:     "primary",
			SourceEventOffset: 4,
			MessageID:         "message-1",
			Body:              []byte(`{"private":"must stay internal"}`),
		}},
		At: now.Add(6 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	binding, err := store.RunBinding(ctx, run.ID)
	if err != nil || binding != run {
		t.Fatalf("binding = %#v, %v", binding, err)
	}
	observation, err := store.ReadObservation(ctx, run.ID, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Run != run || observation.EventOffset != 4 ||
		!observation.HasPrior || len(observation.Events) != 2 ||
		observation.Events[0].Offset != 3 ||
		observation.Events[1].Offset != 4 {
		t.Fatalf("observation = %#v", observation)
	}
	if len(observation.Attempts) != 1 ||
		observation.Attempts[0].EffectID != effect.ID ||
		string(observation.Attempts[0].Usage) != string(usage) {
		t.Fatalf("attempt facts = %#v", observation.Attempts)
	}
	if len(observation.Notifications) != 1 ||
		observation.Notifications[0].DestinationID != "primary" ||
		observation.Notifications[0].MessageID != "message-1" ||
		observation.Notifications[0].State != NotificationPending ||
		observation.NotificationsTruncated {
		t.Fatalf("notification facts = %#v", observation.Notifications)
	}

	first, err := store.EventsAfter(ctx, run.ID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.Through != 2 || first.EventOffset != 4 ||
		len(first.Events) != 2 {
		t.Fatalf("first replay = %#v", first)
	}
	second, err := store.EventsAfter(ctx, run.ID, first.Through, 2)
	if err != nil {
		t.Fatal(err)
	}
	if second.HasMore || second.Through != 4 || second.EventOffset != 4 ||
		len(second.Events) != 2 || second.Events[0].Offset != 3 {
		t.Fatalf("second replay = %#v", second)
	}
}

func TestObservationProjectsOnlyBoundedActiveAttentions(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(
		ctx,
		run.ID,
		now,
		time.Minute,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	recovery := testRecoveryBinding(
		"T3-runtime",
		"observation-cycle",
		"turn",
		"progress",
	)
	resolved := testAttentionBinding(recovery, 1)
	active := testAttentionBinding(recovery, 2)
	for _, attention := range []AttentionBinding{resolved, active} {
		if _, err := store.OpenAttention(
			ctx,
			owner,
			OpenAttentionCommand{
				RunID:              run.ID,
				Attention:          attention,
				ExpectedGeneration: 0,
				Question:           "A bounded public question",
			},
			now,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.AnswerAttention(
		ctx,
		AnswerAttentionCommand{
			RunID:              run.ID,
			Attention:          resolved,
			ExpectedGeneration: 1,
			Answer:             "A bounded public answer",
		},
		now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveAttention(
		ctx,
		owner,
		ResolveAttentionCommand{
			RunID:              run.ID,
			Attention:          resolved,
			ExpectedGeneration: 2,
		},
		now,
	); err != nil {
		t.Fatal(err)
	}
	observation, err := store.ReadObservation(ctx, run.ID, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	if observation.AttentionsTruncated ||
		len(observation.Attentions) != 1 ||
		observation.Attentions[0].Attention.ID != active.ID ||
		observation.Attentions[0].State != AttentionOpen {
		t.Fatalf("active attentions = %#v", observation.Attentions)
	}
}

func TestObservationRejectsTamperedUsageIntegrity(t *testing.T) {
	t.Parallel()

	const originalUsage = `{"token_status":"reported","input_tokens":0,` +
		`"output_tokens":0,"cost_status":"unavailable",` +
		`"cost_micro_units":null,"currency":null,"source":null}`
	const alternateUsage = `{"token_status":"reported","input_tokens":1,` +
		`"output_tokens":0,"cost_status":"unavailable",` +
		`"cost_micro_units":null,"currency":null,"source":null}`
	tests := []struct {
		name   string
		mutate func(context.Context, *sql.Conn, Run, Effect) error
	}{
		{
			name: "canonical body does not match stored digest",
			mutate: func(
				ctx context.Context,
				conn *sql.Conn,
				run Run,
				effect Effect,
			) error {
				_, err := conn.ExecContext(
					ctx,
					`UPDATE attempts SET usage=?
					 WHERE run_id=? AND effect_id=? AND attempt=1`,
					[]byte(alternateUsage),
					run.ID,
					effect.ID,
				)
				return err
			},
		},
		{
			name: "stored digest has invalid shape",
			mutate: func(
				ctx context.Context,
				conn *sql.Conn,
				run Run,
				effect Effect,
			) error {
				_, err := conn.ExecContext(
					ctx,
					`UPDATE attempts SET usage_digest=?
					 WHERE run_id=? AND effect_id=? AND attempt=1`,
					"not-a-digest",
					run.ID,
					effect.ID,
				)
				return err
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store, run, _, effect := journalFixture(t)
			ctx := context.Background()
			claim, err := store.Claim(
				ctx,
				run.ID,
				effect.ID,
				run.CreatedAt.Add(time.Second),
				time.Minute,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Complete(ctx, Completion{
				RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
				State: OperationalFailed, ErrorCode: "transport_error",
				Attempt: &Attempt{
					Number: 1, Responsibility: "work_verification",
					TransportStatus:   "failed",
					ObservationDigest: digest([]byte("observation")),
					Usage:             []byte(originalUsage),
				},
				EventKind: "dispatch_operational_failure",
				EventBody: []byte("private"),
				At:        run.CreatedAt.Add(2 * time.Second),
			}); err != nil {
				t.Fatal(err)
			}
			if err := store.immediate(ctx, func(conn *sql.Conn) error {
				return test.mutate(ctx, conn, run, effect)
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ReadObservation(
				ctx,
				run.ID,
				1,
				1,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf(
					"ReadObservation() error = %v, want CORRUPT_JOURNAL",
					err,
				)
			}
		})
	}
}
