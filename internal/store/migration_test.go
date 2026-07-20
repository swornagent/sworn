package store

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
	recordDigest := digest(canonical)
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
