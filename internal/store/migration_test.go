package store

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

	database = rawDatabase(t, path)
	contents, err := migrationFiles.ReadFile(migrationNames[5])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, string(contents)); err != nil {
		t.Fatalf("apply migration 6: %v", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 6"); err != nil {
		t.Fatal(err)
	}
	control := &Store{db: database, now: time.Now, leaseIssuer: &leaseIssuer{}}
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
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	if control, err := Open(ctx, path); err == nil ||
		!strings.Contains(err.Error(), "refuses legacy builder recovery authority") {
		if control != nil {
			_ = control.Close()
		}
		t.Fatalf("migration seven legacy unknown guard error = %v", err)
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
	if version != 6 || state != "unknown" || history != 1 {
		t.Fatalf("failed migration seven changed archaeology: version=%d state=%s history=%d", version, state, history)
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

func TestMigrationSevenRefusesPreexistingRecoveryReceiptsAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	for index, name := range migrationNames[:6] {
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
			attempt, owner_id, last_error, created_at_us, started_at_us, completed_at_us
		) VALUES (
			'effect-1', 'run-1', 'command-1', 0, 'runner.build', CAST('{}' AS BLOB), 'failed',
			1, 'worker-1', 'legacy failure', 1, 2, 3
		);
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
		) VALUES ('effect-1', 1, 'claimed', 'worker-1', CAST('{"forged":true}' AS BLOB), 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 6"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if control, err := Open(ctx, path); err == nil ||
		!strings.Contains(err.Error(), "refuses legacy builder recovery authority") {
		if control != nil {
			_ = control.Close()
		}
		t.Fatalf("migration seven hostile receipt guard error = %v", err)
	}
	database = rawDatabase(t, path)
	t.Cleanup(func() { _ = database.Close() })
	var version, history int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM effect_observations").Scan(&history); err != nil {
		t.Fatal(err)
	}
	if version != 6 || history != 1 {
		t.Fatalf("failed migration changed hostile archaeology: version=%d history=%d", version, history)
	}
}

func TestMigrationEightRefusesUnwitnessedUnknownLocalCheckAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	for index, name := range migrationNames[:6] {
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
		) VALUES ('command-1', 'run-1', 'checks.dispatch', 0, 'request', CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 1);
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state,
			attempt, owner_id, last_error, created_at_us, started_at_us
		) VALUES (
			'effect-check-1', 'run-1', 'command-1', 0, 'check.local', CAST('{}' AS BLOB), 'unknown',
			1, 'legacy-check-worker', 'legacy process stopped', 1, 2
		);
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, detail, recorded_at_us
		) VALUES ('effect-check-1', 1, 'claimed', 'legacy-check-worker', NULL, NULL, 2)`); err != nil {
		t.Fatal(err)
	}
	contents, err := migrationFiles.ReadFile(migrationNames[6])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, string(contents)); err != nil {
		t.Fatalf("apply migration 7: %v", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 7"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if control, err := Open(ctx, path); err == nil ||
		!strings.Contains(err.Error(), "refuses legacy local-check recovery authority") {
		if control != nil {
			_ = control.Close()
		}
		t.Fatalf("migration eight legacy check guard error = %v", err)
	}
	database = rawDatabase(t, path)
	t.Cleanup(func() { _ = database.Close() })
	var version, history int
	var state string
	if err := database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx,
		"SELECT state FROM effects WHERE effect_id = 'effect-check-1'",
	).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx,
		"SELECT count(*) FROM effect_observations WHERE effect_id = 'effect-check-1'",
	).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if version != 7 || state != "unknown" || history != 1 {
		t.Fatalf("failed migration eight changed archaeology: version=%d state=%s history=%d", version, state, history)
	}
}

func TestMigrationEightPreservesPendingCheckAndRequiresMatchedRetryWitness(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	for index, name := range migrationNames[:7] {
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
		) VALUES ('command-1', 'run-1', 'checks.dispatch', 0, 'request', CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 1);
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state,
			attempt, created_at_us
		) VALUES ('effect-check-1', 'run-1', 'command-1', 0, 'check.local', CAST('{}' AS BLOB), 'pending', 0, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 7"); err != nil {
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
	var version int
	if err := control.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != len(migrationNames) {
		t.Fatalf("migrated version = %d, %v", version, err)
	}
	if _, err := control.db.ExecContext(ctx, `
		UPDATE effects SET state = 'running', attempt = 1,
		       owner_id = 'controller-1', started_at_us = 2
		WHERE effect_id = 'effect-check-1';
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
		) VALUES ('effect-check-1', 1, 'claimed', 'controller-1', CAST('{"attempt":1}' AS BLOB), 2);
		UPDATE effects SET state = 'unknown', last_error = 'interrupted'
		WHERE effect_id = 'effect-check-1'`); err != nil {
		t.Fatalf("construct witnessed migrated attempt: %v", err)
	}
	if _, err := control.db.ExecContext(ctx, `
		UPDATE effects SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		       completed_at_us = NULL, last_error = NULL
		WHERE effect_id = 'effect-check-1'`); err == nil ||
		!strings.Contains(err.Error(), "invalid effect transition") {
		t.Fatalf("unwitnessed migrated check retry error = %v", err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO effect_observations (
			effect_id, attempt, kind, owner_id, receipt_json, recorded_at_us
		) VALUES ('effect-check-1', 1, 'not_applied', 'controller-1', CAST('{"attempt":1}' AS BLOB), 3);
		UPDATE effects SET state = 'pending', owner_id = NULL, started_at_us = NULL,
		       completed_at_us = NULL, last_error = NULL
		WHERE effect_id = 'effect-check-1'`); err != nil {
		t.Fatalf("matched migrated check retry witness: %v", err)
	}
	var state string
	var attempt int64
	if err := control.db.QueryRowContext(ctx,
		"SELECT state, attempt FROM effects WHERE effect_id = 'effect-check-1'",
	).Scan(&state, &attempt); err != nil || state != "pending" || attempt != 1 {
		t.Fatalf("migrated check retry = state %q attempt %d, %v", state, attempt, err)
	}
}

func TestMigrationNineCreatesStrictVerifierHistoryAtSchemaNine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	database := rawDatabase(t, path)
	if len(migrationNames) != 9 {
		t.Fatalf("migration count = %d, want 9", len(migrationNames))
	}
	for index, name := range migrationNames[:8] {
		contents, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply migration %d: %v", index+1, err)
		}
	}
	var before int
	if err := database.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE type = 'table' AND name IN ('verifier_dispatch_records', 'verdict_records')`,
	).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatalf("schema eight already contains verifier history tables = %d", before)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA user_version = 8"); err != nil {
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
	var version int
	if err := control.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 9 {
		t.Fatalf("migrated schema version = %d, %v", version, err)
	}
	for _, table := range []string{"verifier_dispatch_records", "verdict_records"} {
		var strict int
		if err := control.db.QueryRowContext(ctx,
			"SELECT strict FROM pragma_table_list WHERE name = ?", table,
		).Scan(&strict); err != nil {
			t.Fatalf("inspect %s: %v", table, err)
		}
		if strict != 1 {
			t.Fatalf("%s strict = %d, want 1", table, strict)
		}
	}
	var triggerCount int
	if err := control.db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE type = 'trigger' AND name IN (
			'verifier_dispatch_records_require_dispatch',
			'verifier_dispatch_records_no_update',
			'verifier_dispatch_records_no_delete',
			'verdict_records_require_admission',
			'verdict_records_no_update',
			'verdict_records_no_delete'
		)`,
	).Scan(&triggerCount); err != nil {
		t.Fatal(err)
	}
	if triggerCount != 6 {
		t.Fatalf("migration nine trigger count = %d, want 6", triggerCount)
	}
}

func TestMigrationNineVerifierHistoryIsClosedAndImmutable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openMigrationNineTestStore(t)
	seedMigrationNineBase(t, control)
	seedMigrationNineDispatchPrerequisites(t, control, "verifier-1", "dispatch-digest-1", "dispatch-command-1")
	insertMigrationNineDispatch(t, control,
		"verifier-1", "dispatch-digest-1", "dispatch-command-1", "submission-digest-1", 1,
	)
	seedMigrationNineVerdictPrerequisites(
		t, control, "verdict-digest-1", "verdict-command-1", "verdict-event-1", "verdict.admitted", 4,
	)
	insertMigrationNineVerdict(t, control,
		"verdict-1", "verdict-digest-1", "verdict-command-1", "verdict-event-1",
		"verifier-1", "assessment-digest-1", 4, 1,
	)

	for name, statement := range map[string]string{
		"dispatch update": `UPDATE verifier_dispatch_records
			SET profile_digest = 'changed' WHERE dispatch_id = 'verifier-1'`,
		"dispatch delete": `DELETE FROM verifier_dispatch_records WHERE dispatch_id = 'verifier-1'`,
		"verdict update":  `UPDATE verdict_records SET outcome = 'FAIL' WHERE verdict_id = 'verdict-1'`,
		"verdict delete":  `DELETE FROM verdict_records WHERE verdict_id = 'verdict-1'`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := control.db.ExecContext(ctx, statement); err == nil ||
				!strings.Contains(err.Error(), "immutable") {
				t.Fatalf("immutable history error = %v", err)
			}
		})
	}
	rows, err := control.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("valid verifier history contains a foreign-key violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationNineRejectsBrokenClosureAndIdentityCollisions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openMigrationNineTestStore(t)
	seedMigrationNineBase(t, control)
	seedMigrationNineDispatchPrerequisites(t, control, "verifier-1", "dispatch-digest-1", "dispatch-command-1")
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us)
		VALUES ('submission-digest-other', 'submission-v1', CAST('{"other":true}' AS BLOB), 14, 20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO verifier_dispatch_records (
			dispatch_id, digest, artifact_digest, submission_id, submission_digest,
			effect_id, profile_digest, run_id, command_id, review_epoch, created_at_us
		) VALUES (
			'verifier-1', 'dispatch-digest-1', 'dispatch-digest-1', 'submission-1', 'submission-digest-other',
			'verifier-1', 'profile-digest-1', 'run-1', 'dispatch-command-1', 1, 30
		)`); err == nil || !strings.Contains(err.Error(), "exact applied command, effect, and submission") {
		t.Fatalf("mismatched dispatch closure error = %v", err)
	}
	insertMigrationNineDispatch(t, control,
		"verifier-1", "dispatch-digest-1", "dispatch-command-1", "submission-digest-1", 1,
	)

	seedMigrationNineDispatchPrerequisites(
		t, control, "verifier-epoch-4", "dispatch-digest-epoch-4", "dispatch-command-epoch-4",
	)
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO verifier_dispatch_records (
			dispatch_id, digest, artifact_digest, submission_id, submission_digest,
			effect_id, profile_digest, run_id, command_id, review_epoch, created_at_us
		) VALUES (
			'verifier-epoch-4', 'dispatch-digest-epoch-4', 'dispatch-digest-epoch-4',
			'submission-1', 'submission-digest-1', 'verifier-epoch-4',
			'profile-digest-1', 'run-1', 'dispatch-command-epoch-4', 4, 31
		)`); err == nil || !strings.Contains(strings.ToLower(err.Error()), "check constraint") {
		t.Fatalf("dispatch epoch four error = %v", err)
	}

	seedMigrationNineDispatchPrerequisites(
		t, control, "verifier-duplicate-epoch", "dispatch-digest-duplicate-epoch",
		"dispatch-command-duplicate-epoch",
	)
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO verifier_dispatch_records (
			dispatch_id, digest, artifact_digest, submission_id, submission_digest,
			effect_id, profile_digest, run_id, command_id, review_epoch, created_at_us
		) VALUES (
			'verifier-duplicate-epoch', 'dispatch-digest-duplicate-epoch',
			'dispatch-digest-duplicate-epoch', 'submission-1', 'submission-digest-1',
			'verifier-duplicate-epoch', 'profile-digest-1', 'run-1',
			'dispatch-command-duplicate-epoch', 1, 31
		)`); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("duplicate submission review epoch error = %v", err)
	}

	seedMigrationNineDispatchPrerequisites(t, control, "verifier-2", "dispatch-digest-2", "dispatch-command-2")
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO verifier_dispatch_records (
			dispatch_id, digest, artifact_digest, submission_id, submission_digest,
			effect_id, profile_digest, run_id, command_id, review_epoch, created_at_us
		) VALUES (
			'verifier-2', 'dispatch-digest-1', 'dispatch-digest-1', 'submission-1', 'submission-digest-1',
			'verifier-2', 'profile-digest-1', 'run-1', 'dispatch-command-2', 2, 31
		)`); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("dispatch digest collision error = %v", err)
	}
	insertMigrationNineDispatch(t, control,
		"verifier-2", "dispatch-digest-2", "dispatch-command-2", "submission-digest-1", 2,
	)

	seedMigrationNineVerdictPrerequisites(
		t, control, "verdict-digest-bad-event", "verdict-command-bad-event", "verdict-event-bad",
		"verdict.rejected", 5,
	)
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO verdict_records (
			verdict_id, digest, submission_id, submission_digest, dispatch_id,
			verifier_effect_id, assessment_digest, outcome, run_id, command_id,
			event_id, event_revision, review_epoch, created_at_us
		) VALUES (
			'verdict-bad-event', 'verdict-digest-bad-event', 'submission-1', 'submission-digest-1', 'verifier-2',
			'verifier-2', 'assessment-digest-1', 'PASS', 'run-1', 'verdict-command-bad-event',
			'verdict-event-bad', 5, 2, 40
		)`); err == nil || !strings.Contains(err.Error(), "exact applied admission event and dispatch") {
		t.Fatalf("wrong-event verdict closure error = %v", err)
	}

	seedMigrationNineVerdictPrerequisites(
		t, control, "verdict-digest-2", "verdict-command-2", "verdict-event-2", "verdict.admitted", 6,
	)
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO verdict_records (
			verdict_id, digest, submission_id, submission_digest, dispatch_id,
			verifier_effect_id, assessment_digest, outcome, run_id, command_id,
			event_id, event_revision, review_epoch, created_at_us
		) VALUES (
			'verdict-2', 'verdict-digest-2', 'submission-1', 'submission-digest-1', 'verifier-2',
			'verifier-2', 'assessment-digest-missing', 'PASS', 'run-1', 'verdict-command-2',
			'verdict-event-2', 6, 2, 41
		)`); err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("missing assessment foreign-key error = %v", err)
	}
	insertMigrationNineVerdict(t, control,
		"verdict-2", "verdict-digest-2", "verdict-command-2", "verdict-event-2",
		"verifier-2", "assessment-digest-1", 6, 2,
	)

	seedMigrationNineVerdictPrerequisites(
		t, control, "verdict-digest-3", "verdict-command-3", "verdict-event-3", "verdict.admitted", 7,
	)
	if _, err := control.db.ExecContext(ctx, `
		INSERT INTO verdict_records (
			verdict_id, digest, submission_id, submission_digest, dispatch_id,
			verifier_effect_id, assessment_digest, outcome, run_id, command_id,
			event_id, event_revision, review_epoch, created_at_us
		) VALUES (
			'verdict-3', 'verdict-digest-3', 'submission-1', 'submission-digest-1', 'verifier-2',
			'verifier-2', 'assessment-digest-1', 'PASS', 'run-1', 'verdict-command-3',
			'verdict-event-3', 7, 2, 42
		)`); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("second verdict for one dispatch error = %v", err)
	}
}

func openMigrationNineTestStore(t *testing.T) *Store {
	t.Helper()
	control, err := Open(context.Background(), filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	return control
}

func seedMigrationNineBase(t *testing.T, control *Store) {
	t.Helper()
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us) VALUES
			('submission-digest-1', 'submission-v1', CAST('{"submission":1}' AS BLOB), 16, 1),
			('assessment-digest-1', 'sworn-verifier-assessment-v1', CAST('{"assessment":1}' AS BLOB), 16, 2);
		INSERT INTO runs (
			run_id, delivery_id, repository_id, target_ref, plan_digest,
			revision, phase, terminal, state_json, created_at_us, updated_at_us
		) VALUES (
			'run-1', 'delivery-1', 'repo-1', 'refs/heads/main', 'plan-digest-1',
			0, 'active', 0, CAST('{}' AS BLOB), 1, 1
		);
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES (
			'submission-command-1', 'run-1', 'submission.admit', 0, 'request-submission-1',
			CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 2
		);
		INSERT INTO submission_records (
			submission_id, delivery_id, work_id, attempt, digest, run_id, command_id
		) VALUES (
			'submission-1', 'delivery-1', 'work-1', 1, 'submission-digest-1', 'run-1', 'submission-command-1'
		)`,
	); err != nil {
		t.Fatalf("seed migration nine base: %v", err)
	}
}

func seedMigrationNineDispatchPrerequisites(
	t *testing.T,
	control *Store,
	dispatchID, digest, commandID string,
) {
	t.Helper()
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us)
		VALUES (?, 'control-receipt-v1', CAST('{"dispatch":true}' AS BLOB), 17, 10)`, digest,
	); err != nil {
		t.Fatalf("seed dispatch %s record: %v", dispatchID, err)
	}
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO artifacts (digest, media_type, content, size, created_at_us)
		VALUES (?, 'application/json', CAST('{"dispatch":true}' AS BLOB), 17, 10)`, digest,
	); err != nil {
		t.Fatalf("seed dispatch %s artifact: %v", dispatchID, err)
	}
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES (?, 'run-1', 'verifier.dispatch', 1, ?, CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 10)`,
		commandID, "request-"+commandID,
	); err != nil {
		t.Fatalf("seed dispatch %s command: %v", dispatchID, err)
	}
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO effects (
			effect_id, run_id, command_id, ordinal, kind, request_json, state, attempt, created_at_us
		) VALUES (?, 'run-1', ?, 0, 'runner.verifier', CAST('{}' AS BLOB), 'pending', 0, 10)`,
		dispatchID, commandID,
	); err != nil {
		t.Fatalf("seed dispatch %s effect: %v", dispatchID, err)
	}
}

func insertMigrationNineDispatch(
	t *testing.T,
	control *Store,
	dispatchID, digest, commandID, submissionDigest string,
	reviewEpoch int64,
) {
	t.Helper()
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO verifier_dispatch_records (
			dispatch_id, digest, artifact_digest, submission_id, submission_digest,
			effect_id, profile_digest, run_id, command_id, review_epoch, created_at_us
		) VALUES (?, ?, ?, 'submission-1', ?, ?, 'profile-digest-1', 'run-1', ?, ?, 30)`,
		dispatchID, digest, digest, submissionDigest, dispatchID, commandID, reviewEpoch,
	); err != nil {
		t.Fatalf("insert dispatch %s: %v", dispatchID, err)
	}
}

func seedMigrationNineVerdictPrerequisites(
	t *testing.T,
	control *Store,
	digest, commandID, eventID, eventKind string,
	eventRevision int64,
) {
	t.Helper()
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us)
		VALUES (?, 'delivery-verdict-v1', CAST('{"verdict":true}' AS BLOB), 16, 40)`, digest,
	); err != nil {
		t.Fatalf("seed verdict %s record: %v", digest, err)
	}
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO commands (
			command_id, run_id, kind, expected_revision, request_digest,
			request_json, outcome, result_json, recorded_at_us
		) VALUES (?, 'run-1', 'verdict.admit', 2, ?, CAST('{}' AS BLOB), 'applied', CAST('{}' AS BLOB), 40)`,
		commandID, "request-"+commandID,
	); err != nil {
		t.Fatalf("seed verdict %s command: %v", digest, err)
	}
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO events (
			event_id, run_id, command_id, revision, ordinal, kind, data_json, recorded_at_us
		) VALUES (?, 'run-1', ?, ?, 0, ?, CAST('{}' AS BLOB), 40)`,
		eventID, commandID, eventRevision, eventKind,
	); err != nil {
		t.Fatalf("seed verdict %s event: %v", digest, err)
	}
}

func insertMigrationNineVerdict(
	t *testing.T,
	control *Store,
	verdictID, digest, commandID, eventID, dispatchID, assessmentDigest string,
	eventRevision, reviewEpoch int64,
) {
	t.Helper()
	if _, err := control.db.ExecContext(context.Background(), `
		INSERT INTO verdict_records (
			verdict_id, digest, submission_id, submission_digest, dispatch_id,
			verifier_effect_id, assessment_digest, outcome, run_id, command_id,
			event_id, event_revision, review_epoch, created_at_us
		) VALUES (
			?, ?, 'submission-1', 'submission-digest-1', ?,
			?, ?, 'PASS', 'run-1', ?, ?, ?, ?, 40
		)`,
		verdictID, digest, dispatchID, dispatchID, assessmentDigest,
		commandID, eventID, eventRevision, reviewEpoch,
	); err != nil {
		t.Fatalf("insert verdict %s: %v", verdictID, err)
	}
}
