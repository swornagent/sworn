package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttemptRetryTriggerRequiresExactMachineWitnessPair(t *testing.T) {
	t.Parallel()
	exact := []byte(`{"schema_version":"attempt-test-v1","attempt":"exact"}`)
	different := []byte(`{"schema_version":"attempt-test-v1","attempt":"different"}`)

	for _, test := range []struct {
		name       string
		effectKind string
		claimed    []byte
		reconciled []byte
		attempt    int
		want       bool
	}{
		{name: "absent retry observation", effectKind: "runner.build", claimed: exact},
		{name: "null claimed witness", effectKind: "runner.build", reconciled: exact, attempt: 1},
		{name: "null retry witness", effectKind: "runner.build", claimed: exact, attempt: 1},
		{name: "mismatched witness", effectKind: "runner.build", claimed: exact, reconciled: different, attempt: 1},
		{name: "wrong attempt", effectKind: "runner.build", claimed: exact, reconciled: exact, attempt: 2},
		{name: "uncontrolled effect", effectKind: "future.effect", claimed: exact, reconciled: exact, attempt: 1},
		{name: "exact check pair", effectKind: "check.local", claimed: exact, reconciled: exact, attempt: 1, want: true},
		{name: "exact pair", effectKind: "runner.build", claimed: exact, reconciled: exact, attempt: 1, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			control, effectID := retryTriggerFixture(t, test.effectKind, test.claimed)
			if test.attempt != 0 || test.reconciled != nil {
				if _, err := control.db.ExecContext(context.Background(), `
					INSERT INTO effect_observations (
						effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
					) VALUES (?, ?, 'not_applied', 'reconciler-1', ?, 4)`,
					effectID, test.attempt, nullableBytes(test.reconciled),
				); err != nil {
					t.Fatal(err)
				}
			}
			_, err := control.db.ExecContext(context.Background(), `
				UPDATE effects
				SET state = 'pending', owner_id = NULL, started_at_us = NULL,
				    completed_at_us = NULL, last_error = NULL
				WHERE effect_id = ?`, effectID)
			if test.want && err != nil {
				t.Fatalf("exact witness pair rejected: %v", err)
			}
			if !test.want && (err == nil || !strings.Contains(err.Error(), "invalid effect transition")) {
				t.Fatalf("inexact witness transition error = %v", err)
			}
		})
	}
}

func TestAttemptRetryObservationsAreUniquePerAttempt(t *testing.T) {
	t.Parallel()
	exact := []byte(`{"schema_version":"attempt-test-v1","attempt":"exact"}`)
	control, effectID := retryTriggerFixture(t, "runner.build", exact)
	ctx := context.Background()
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
		) VALUES (?, 1, 'claimed', 'worker-2', ?, 4)`, effectID, exact,
	); err == nil {
		t.Fatal("duplicate claimed observation was accepted")
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
		) VALUES (?, 1, 'not_applied', 'reconciler-1', ?, 4)`, effectID, exact,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
		) VALUES (?, 1, 'not_applied', 'reconciler-2', ?, 5)`, effectID, exact,
	); err == nil {
		t.Fatal("duplicate not-applied observation was accepted")
	}
}

func retryTriggerFixture(t *testing.T, effectKind string, claimed []byte) (*Store, string) {
	t.Helper()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	ctx := context.Background()
	effectID := "effect-trigger"
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO runs (
			run_id, delivery_id, repository_id, target_ref, plan_digest,
			revision, phase, terminal, state_json, created_at_us, updated_at_us
		) VALUES (
			'run-trigger', 'delivery-trigger', 'repo-trigger', 'refs/heads/main', 'plan-trigger',
			0, 'active', 0, CAST('{}' AS BLOB), 1, 1
		);
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES (
			'command-trigger', 'run-trigger', 'build.dispatch', 0, 'request-trigger',
			CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 1
		);
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state,
			attempt, owner_id, receipt_json, last_error, created_at_us,
			started_at_us, completed_at_us
		) VALUES (?, 'run-trigger', 'command-trigger', 0, ?, CAST('{}' AS BLOB), 'unknown',
			1, 'worker-1', NULL, 'interrupted', 1, 2, NULL)`, effectID, effectKind,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
		) VALUES (?, 1, 'claimed', 'worker-1', ?, 2)`, effectID, nullableBytes(claimed),
	); err != nil {
		t.Fatal(err)
	}
	return control, effectID
}

func nullableBytes(value []byte) any {
	if value == nil {
		return nil
	}
	return value
}
