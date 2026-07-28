package journal

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestVisitEvaluationStreamsExactFactsBeyondCockpitWindow(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	const attempts = 300
	const usage = `{"token_status":"reported","input_tokens":0,"output_tokens":0,` +
		`"cost_status":"unavailable","cost_micro_units":null,` +
		`"currency":null,"source":null}`
	if err := store.immediate(ctx, func(conn *sql.Conn) error {
		for index := 0; index < attempts; index++ {
			id := fmt.Sprintf("eval-effect-%03d", index)
			startedAt := run.CreatedAt.Add(time.Duration(index+1) * time.Second)
			finishedAt := startedAt.Add(250 * time.Millisecond)
			started, _ := canonicalTime(startedAt)
			finished, _ := canonicalTime(finishedAt)
			payload := []byte("private command body")
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO commands(
				     run_id, replay_key, kind, payload_digest, payload, created_at
				 ) VALUES(?, ?, 'driver.dispatch', ?, ?, ?)`,
				run.ID,
				id,
				digest(payload),
				payload,
				started,
			); err != nil {
				return err
			}
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO effects(
				     run_id, effect_id, replay_key, kind, state,
				     before_digest, expected_digest, updated_at
				 ) VALUES(?, ?, ?, 'driver.dispatch', 'succeeded', ?, ?, ?)`,
				run.ID,
				id,
				id,
				digest([]byte("before")),
				digest([]byte("after")),
				finished,
			); err != nil {
				return err
			}
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO attempts(
				     run_id, effect_id, attempt, responsibility,
				     transport_status, observation_digest, usage_digest,
				     usage, created_at
				 ) VALUES(?, ?, ?, 'implementer', 'completed', ?, ?, ?, ?)`,
				run.ID,
				id,
				index%3+1,
				digest([]byte("private observation")),
				digest([]byte(usage)),
				[]byte(usage),
				finished,
			); err != nil {
				return err
			}
			eventBody := []byte("private event body")
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO events(
				     run_id, kind, body_digest, body, created_at
				 ) VALUES(?, 'dispatch_completed', ?, ?, ?)`,
				run.ID,
				digest(eventBody),
				eventBody,
				finished,
			); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var eventCount, attemptCount int
	var firstAttempt EvaluationFact
	window, err := store.VisitEvaluation(
		ctx,
		run.ID,
		func(fact EvaluationFact) {
			switch fact.Kind {
			case EvaluationEvent:
				eventCount++
			case EvaluationAttempt:
				attemptCount++
				if attemptCount == 1 {
					firstAttempt = fact
				}
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if eventCount != attempts || attemptCount != attempts ||
		window.ThroughOffset != attempts ||
		!window.ObservedAt.Equal(
			run.CreatedAt.Add(attempts*time.Second+250*time.Millisecond),
		) {
		t.Fatalf(
			"window=%#v events=%d attempts=%d",
			window,
			eventCount,
			attemptCount,
		)
	}
	if firstAttempt.EffectKind != "driver.dispatch" ||
		firstAttempt.EffectState != Succeeded ||
		firstAttempt.Responsibility != "implementer" ||
		firstAttempt.Transport != "completed" ||
		firstAttempt.Attempt != 1 ||
		!firstAttempt.StartedAt.Equal(run.CreatedAt.Add(time.Second)) ||
		!firstAttempt.FinishedAt.Equal(
			run.CreatedAt.Add(time.Second+250*time.Millisecond),
		) ||
		string(firstAttempt.Usage) != usage {
		t.Fatalf("first attempt = %#v", firstAttempt)
	}
}

func TestVisitEvaluationIsOneHighWaterSnapshotAndRejectsCorruption(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	usage := []byte(
		`{"token_status":"unavailable","input_tokens":null,` +
			`"output_tokens":null,"cost_status":"unavailable",` +
			`"cost_micro_units":null,"currency":null,"source":null}`,
	)
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: Succeeded,
		Attempt: &Attempt{
			Number: 1, Responsibility: "verifier",
			TransportStatus:   "completed",
			ObservationDigest: digest([]byte("private")),
			Usage:             usage,
		},
		EventKind: "dispatch_completed",
		EventBody: []byte("private body"),
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	var facts []EvaluationFact
	window, err := store.VisitEvaluation(
		ctx,
		run.ID,
		func(fact EvaluationFact) { facts = append(facts, fact) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if window.ThroughOffset != 1 || len(facts) != 2 ||
		facts[0].Kind != EvaluationEvent ||
		facts[1].Kind != EvaluationAttempt {
		t.Fatalf("window=%#v facts=%#v", window, facts)
	}
	if err := store.AppendEvent(
		ctx,
		run.ID,
		"runtime_progress",
		nil,
		now.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if window.ThroughOffset != 1 || len(facts) != 2 {
		t.Fatalf("completed snapshot changed: %#v %#v", window, facts)
	}

	if err := store.immediate(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(
			ctx,
			"UPDATE attempts SET usage=? WHERE run_id=? AND effect_id=?",
			[]byte(`{"tampered":true}`),
			run.ID,
			effect.ID,
		)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.VisitEvaluation(
		ctx,
		run.ID,
		func(EvaluationFact) {},
	); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("tampered usage = %v", err)
	}
}

func TestVisitEvaluationRejectsNilVisitor(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	if _, err := store.VisitEvaluation(
		context.Background(),
		run.ID,
		nil,
	); !IsCode(err, "INVALID_EVALUATION_VISITOR") {
		t.Fatalf("nil visitor = %v", err)
	}
}
