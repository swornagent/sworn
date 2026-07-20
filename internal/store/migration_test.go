package store

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/protocol"
)

func TestMigrationFivePurgesLegacySubmissionIdentitiesButRetainsRecords(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	for index, name := range migrationNames[:4] {
		contents, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply migration %d: %v", index+1, err)
		}
	}
	canonical := []byte(`{"schema_version":"submission-v1"}`)
	recordDigest := protocol.RawDigest(canonical)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us)
		VALUES (?, 'submission-v1', ?, ?, 1)`, recordDigest, canonical, len(canonical)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO submission_records (submission_id, delivery_id, work_id, attempt, digest)
		VALUES ('submission-legacy', 'delivery-legacy', 'work-legacy', 1, ?)`, recordDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 4"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	control, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	var identityCount int
	if err := control.db.QueryRowContext(ctx, "SELECT count(*) FROM submission_records").Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if identityCount != 0 {
		t.Fatalf("legacy submission identities retained = %d, want 0", identityCount)
	}
	var kind string
	var stored []byte
	if err := control.db.QueryRowContext(ctx,
		"SELECT kind, canonical_json FROM records WHERE digest = ?", recordDigest,
	).Scan(&kind, &stored); err != nil {
		t.Fatal(err)
	}
	if kind != "submission-v1" || string(stored) != string(canonical) {
		t.Fatalf("legacy canonical record = %q %s", kind, stored)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO submission_records (
			submission_id, delivery_id, work_id, attempt, digest, run_id, command_id
		) VALUES ('submission-new', 'delivery-new', 'work-new', 1, ?, 'run-missing', 'command-missing')`,
		recordDigest,
	); err == nil {
		t.Fatal("atomic submission identity accepted missing run and command bindings")
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO runs (
			run_id, delivery_id, repository_id, target_ref, plan_digest,
			revision, phase, terminal, state_json, created_at_us, updated_at_us
		) VALUES (
			'run-new', 'delivery-new', 'repo-new', 'refs/heads/main', ?,
			0, 'active', 0, ?, 1, 1
		)`, recordDigest, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES (
			'command-new', 'run-new', 'submission.admit', 0, ?,
			?, 'applied', ?, 1
		)`, recordDigest, []byte(`{}`), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO submission_records (
			submission_id, delivery_id, work_id, attempt, digest, run_id, command_id
		) VALUES ('submission-new', 'delivery-new', 'work-new', 1, ?, 'run-new', 'command-new')`,
		recordDigest,
	); err != nil {
		t.Fatalf("insert atomic submission identity: %v", err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES ('command-wrong-kind', 'run-new', 'checks.dispatch', 0, ?, ?, 'applied', ?, 1)`,
		recordDigest, []byte(`{}`), []byte(`{}`),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO submission_records (
			submission_id, delivery_id, work_id, attempt, digest, run_id, command_id
		) VALUES ('submission-wrong-kind', 'delivery-new', 'work-other', 2, ?, 'run-new', 'command-wrong-kind')`,
		recordDigest,
	); err == nil || !strings.Contains(err.Error(), "requires an applied admission command") {
		t.Fatalf("submission identity wrong-kind rejection = %v", err)
	}
	if _, err := control.db.ExecContext(ctx,
		"UPDATE submission_records SET work_id = 'changed' WHERE submission_id = 'submission-new'",
	); err == nil {
		t.Fatal("atomic submission identity was mutable")
	}
	if _, err := control.db.ExecContext(ctx,
		"DELETE FROM submission_records WHERE submission_id = 'submission-new'",
	); err == nil {
		t.Fatal("atomic submission identity was deletable")
	}
}

func TestMigrationSixPreservesUnknownHistoryAndRemovesManualTransitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	for index, name := range migrationNames[:5] {
		contents, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply migration %d: %v", index+1, err)
		}
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO runs (
			run_id, delivery_id, repository_id, target_ref, plan_digest,
			revision, phase, terminal, state_json, created_at_us, updated_at_us
		) VALUES ('run-1', 'delivery-1', 'repo-1', 'refs/heads/main', 'plan', 0, 'active', 0, CAST('{}' AS BLOB), 1, 1);
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES ('command-1', 'run-1', 'build.dispatch', 0, 'request', CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 1);
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state,
			attempt, owner_id, last_error, created_at_us, started_at_us
		) VALUES (
			'effect-1', 'run-1', 'command-1', 0, 'runner.build', CAST('{}' AS BLOB), 'unknown',
			1, 'worker-1', 'interrupted', 1, 2
		);
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, detail, recorded_at_us
		) VALUES ('effect-1', 1, 'unknown', 'worker-1', 'interrupted', 3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 5"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	control, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	unknown, err := listEffects(ctx, control, EffectUnknown)
	if err != nil || len(unknown) != 1 || unknown[0].ID != "effect-1" || unknown[0].Attempt != 1 {
		t.Fatalf("migrated unknown effect = %+v, %v", unknown, err)
	}
	assertCount(t, control, "effect_observations", 1)
	for _, update := range []string{
		`UPDATE effects SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		 last_error = NULL WHERE effect_id = 'effect-1'`,
		`UPDATE effects SET state = 'failed', completed_at_us = 4
		 WHERE effect_id = 'effect-1'`,
	} {
		if _, err := control.db.ExecContext(ctx, update); err == nil ||
			!strings.Contains(err.Error(), "invalid effect transition") {
			t.Fatalf("migration six retained manual transition: %v", err)
		}
	}
}

func TestMigrationSixRefusesPreviouslyManualRequeuedEffect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	for index, name := range migrationNames[:5] {
		contents, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply migration %d: %v", index+1, err)
		}
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO runs (
			run_id, delivery_id, repository_id, target_ref, plan_digest,
			revision, phase, terminal, state_json, created_at_us, updated_at_us
		) VALUES ('run-1', 'delivery-1', 'repo-1', 'refs/heads/main', 'plan', 0, 'active', 0, CAST('{}' AS BLOB), 1, 1);
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES ('command-1', 'run-1', 'build.dispatch', 0, 'request', CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 1);
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state,
			attempt, created_at_us
		) VALUES ('effect-1', 'run-1', 'command-1', 0, 'runner.build', CAST('{}' AS BLOB), 'pending', 1, 1);
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, detail, recorded_at_us
		) VALUES
			('effect-1', 1, 'claimed', 'worker-1', NULL, 2),
			('effect-1', 1, 'unknown', 'worker-1', 'interrupted', 3),
			('effect-1', 1, 'not_applied', 'reconciler-1', 'manual assertion', 4)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 5"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if control, err := Open(ctx, path); err == nil ||
		!strings.Contains(err.Error(), "previously manual-requeued effect") {
		if control != nil {
			_ = control.Close()
		}
		t.Fatalf("migration six manual-retry guard error = %v", err)
	}
	database = rawDatabase(t, path)
	t.Cleanup(func() { _ = database.Close() })
	var version, history int
	var state string
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT state FROM effects WHERE effect_id = 'effect-1'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM effect_observations WHERE effect_id = 'effect-1'").Scan(&history); err != nil {
		t.Fatal(err)
	}
	if version != 5 || state != "pending" || history != 3 {
		t.Fatalf("failed migration changed archaeology: version=%d state=%s history=%d", version, state, history)
	}
}
