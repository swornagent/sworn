package journal

import (
	"context"
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
