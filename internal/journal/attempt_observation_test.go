package journal

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"
)

const failedObservationBody = `{"transport_status":"runner_error","duration_ms":120000,` +
	`"usage":{"token_status":"unavailable","cost_status":"unavailable"},` +
	`"diagnostic":{"code":"adapter_failed","stderr_bytes":0,"truncated":false},` +
	`"events":[{"sequence":1,"kind":"result_completed"}]}`

func TestAttemptObservationRoundTripAndDigestBinding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(failedObservationBody)
	attempt := &Attempt{
		Number:            2,
		Responsibility:    "implementer_implementation",
		TransportStatus:   "runner_error",
		ObservationDigest: digest(body),
		ObservationBody:   body,
		Usage:             []byte(`{"token_status":"unavailable"}`),
	}
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "runner_error",
		Attempt:   attempt,
		EventKind: "dispatch_operational_failure",
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.AttemptObservation(ctx, run.ID, effect.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != run.ID || got.EffectID != effect.ID ||
		got.Number != 2 ||
		got.Responsibility != "implementer_implementation" ||
		got.Transport != "runner_error" {
		t.Fatalf("attempt facts = %#v", got)
	}
	if !got.Stored || got.Partial {
		t.Fatalf("stored/partial = %t/%t, want true/false", got.Stored, got.Partial)
	}
	if got.Digest != digest(body) {
		t.Fatalf("digest = %s, want %s", got.Digest, digest(body))
	}
	if !bytes.Equal(got.Body, body) {
		t.Fatalf("body = %q, want %q", got.Body, body)
	}
}

func TestAttemptObservationRejectsBodyDigestMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(failedObservationBody)
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "runner_error",
		Attempt: &Attempt{
			Number:            1,
			Responsibility:    "implementer_implementation",
			TransportStatus:   "runner_error",
			ObservationDigest: digest(body),
			// Deliberately the wrong pre-image for that digest.
			ObservationBody: []byte(`{"transport_status":"completed"}`),
			Usage:           []byte(`{"token_status":"unavailable"}`),
		},
		EventKind: "dispatch_operational_failure",
		At:        now.Add(time.Second),
	}); !IsCode(err, "OBSERVATION_DIGEST_MISMATCH") {
		t.Fatalf("digest-mismatched body = %v", err)
	}
	// The completion was refused as a whole: the effect stays claimed and
	// no attempt row exists.
	after, err := store.Effect(ctx, run.ID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != Claimed {
		t.Fatalf("effect state = %s, want claimed", after.State)
	}
	if _, err := store.AttemptObservation(ctx, run.ID, effect.ID, 1); !IsCode(err, "ATTEMPT_NOT_FOUND") {
		t.Fatalf("refused completion left an attempt row: %v", err)
	}
}

func TestAttemptObservationMarksOversizeBodyPartial(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	full := bytes.Repeat([]byte("x"), MaxPayloadBytes+1024)
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "runner_error",
		Attempt: &Attempt{
			Number:            1,
			Responsibility:    "implementer_implementation",
			TransportStatus:   "runner_error",
			ObservationDigest: digest(full),
			ObservationBody:   full,
			Usage:             []byte(`{"token_status":"unavailable"}`),
		},
		EventKind: "dispatch_operational_failure",
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.AttemptObservation(ctx, run.ID, effect.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stored || !got.Partial {
		t.Fatalf("stored/partial = %t/%t, want true/true", got.Stored, got.Partial)
	}
	if len(got.Body) != MaxPayloadBytes {
		t.Fatalf("stored body length = %d, want %d", len(got.Body), MaxPayloadBytes)
	}
	if !bytes.Equal(got.Body, full[:MaxPayloadBytes]) {
		t.Fatalf("stored body is not the explicit prefix")
	}
	// The digest remains over the full marshaled bytes, as promised.
	if got.Digest != digest(full) {
		t.Fatalf("digest = %s, want digest over full bytes %s", got.Digest, digest(full))
	}
}

func TestAttemptObservationAbsentBodyIsDistinguishable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// A successful attempt legitimately carries no observation body (its
	// sealed handoff is the durable record) and must not be treated as
	// corruption.
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: Succeeded, Result: []byte("result"),
		Attempt: &Attempt{
			Number:            1,
			Responsibility:    "implementer_implementation",
			TransportStatus:   "completed",
			ObservationDigest: digest([]byte("observation")),
			Usage:             []byte(`{"token_status":"unavailable"}`),
			HandoffDigest:     digest([]byte("handoff")),
		},
		EventKind: "dispatch_completed",
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.AttemptObservation(ctx, run.ID, effect.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stored || got.Partial || got.Body != nil {
		t.Fatalf("absent body read = %#v, want stored=false", got)
	}
	if got.Digest != digest([]byte("observation")) {
		t.Fatalf("digest = %s", got.Digest)
	}
}

func TestAttemptObservationNotFoundAndInvalidArguments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	if _, err := store.AttemptObservation(ctx, run.ID, effect.ID, 1); !IsCode(err, "ATTEMPT_NOT_FOUND") {
		t.Fatalf("missing attempt = %v", err)
	}
	if _, err := store.AttemptObservation(ctx, run.ID, effect.ID, 0); !IsCode(err, "INVALID_ATTEMPT") {
		t.Fatalf("attempt zero = %v", err)
	}
	if _, err := store.AttemptObservation(ctx, "bad run id!", effect.ID, 1); !IsCode(err, "INVALID_RUN") {
		t.Fatalf("invalid run identity = %v", err)
	}
	if _, err := store.AttemptObservation(ctx, run.ID, "bad effect id!", 1); !IsCode(err, "INVALID_EFFECT") {
		t.Fatalf("invalid effect identity = %v", err)
	}
}

func TestAttemptObservationDetectsTamperedBody(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(failedObservationBody)
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "runner_error",
		Attempt: &Attempt{
			Number:            1,
			Responsibility:    "implementer_implementation",
			TransportStatus:   "runner_error",
			ObservationDigest: digest(body),
			ObservationBody:   body,
			Usage:             []byte(`{"token_status":"unavailable"}`),
		},
		EventKind: "dispatch_operational_failure",
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.immediate(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(
			ctx,
			`UPDATE attempts SET observation_body = ? WHERE run_id = ? AND effect_id = ? AND attempt = ?`,
			[]byte(`{"transport_status":"tampered"}`),
			run.ID,
			effect.ID,
			1,
		)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttemptObservation(ctx, run.ID, effect.ID, 1); !IsCode(err, "CORRUPT_JOURNAL") {
		t.Fatalf("tampered body read = %v", err)
	}
}
