package journal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func journalFixture(t *testing.T) (*Store, Run, Command, Effect) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "run.sqlite")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Unix(1_700_000_000, 123).UTC()
	run := Run{
		ID: "run-1", ManifestDigest: digest([]byte("manifest")),
		Repository: "/repository", Release: "release-1",
		TargetRef: "refs/heads/main", CreatedAt: now,
	}
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	command := Command{
		RunID: run.ID, ReplayKey: "effect-1", Kind: "test",
		Payload: []byte("payload"), CreatedAt: now,
	}
	if err := store.RecordCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	effect := Effect{
		RunID: run.ID, ID: "effect-1", ReplayKey: command.ReplayKey,
		Kind: "test", BeforeDigest: digest([]byte("old")),
		ExpectedDigest: digest([]byte("new")), UpdatedAt: now,
	}
	if err := store.EnsureEffect(ctx, effect); err != nil {
		t.Fatal(err)
	}
	return store, run, command, effect
}

func TestSchemaFingerprintIsPinned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "schema.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, schema); err != nil {
		t.Fatal(err)
	}
	got, err := schemaFingerprint(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if got != schemaIdentityDigest {
		t.Fatalf("schema fingerprint = %s, want %s", got, schemaIdentityDigest)
	}
}

func TestPrivateOneWriterReplayClaimAndCASCompletion(t *testing.T) {
	t.Parallel()

	store, run, command, effect := journalFixture(t)
	ctx := context.Background()
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o, want 0600", info.Mode().Perm())
	}
	stats := store.db.Stats()
	if stats.MaxOpenConnections != 1 || stats.OpenConnections != 1 ||
		stats.InUse != 1 || stats.Idle != 0 {
		t.Fatalf("journal must pin exactly one connection: %#v", stats)
	}

	command.CreatedAt = command.CreatedAt.Add(time.Hour)
	if err := store.RecordCommand(ctx, command); err != nil {
		t.Fatalf("idempotent command replay: %v", err)
	}
	conflict := command
	conflict.Payload = []byte("different")
	if err := store.RecordCommand(ctx, conflict); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("conflicting replay = %v", err)
	}
	effect.UpdatedAt = effect.UpdatedAt.Add(time.Hour)
	if err := store.EnsureEffect(ctx, effect); err != nil {
		t.Fatalf("idempotent effect replay: %v", err)
	}

	now := time.Unix(1_700_000_010, 0).UTC()
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Claim is committed before execution. A separate write can proceed while
	// the caller owns the finite token, proving no SQL lock spans the effect.
	if err := store.AppendEvent(ctx, run.ID, "effect_executing", nil, now); err != nil {
		t.Fatalf("write after claim remained locked: %v", err)
	}
	usage := []byte(`{"token_status":"reported","input_tokens":0,"output_tokens":0}`)
	result := []byte(`{"result":"ok"}`)
	err = store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: Succeeded, Result: result,
		Attempt: &Attempt{
			Number: 1, Responsibility: "work_verification",
			TransportStatus:   "completed",
			ObservationDigest: digest([]byte("observation")),
			Usage:             usage, HandoffDigest: digest([]byte("handoff")),
		},
		Receipts:  []Receipt{{Kind: "sealed_handoff", Body: []byte("receipt")}},
		EventKind: "effect_completed", EventBody: []byte("ok"), At: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Effect(ctx, run.ID, effect.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Succeeded || !bytes.Equal(got.Result, result) ||
		got.ResultDigest != digest(result) || got.CurrentClaim != "" {
		t.Fatalf("completed effect = %#v", got)
	}
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: Succeeded, EventKind: "late", At: now.Add(2 * time.Second),
	}); !IsCode(err, "STALE_COMPLETION") {
		t.Fatalf("stale completion = %v", err)
	}
}

func TestReconcileManyIsAtomicAcrossRelatedClaims(t *testing.T) {
	t.Parallel()

	store, run, command, firstEffect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	firstClaim, err := store.Claim(
		ctx, run.ID, firstEffect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	secondCommand := command
	secondCommand.ReplayKey = "effect-2"
	if err := store.RecordCommand(ctx, secondCommand); err != nil {
		t.Fatal(err)
	}
	secondEffect := firstEffect
	secondEffect.ID = "effect-2"
	secondEffect.ReplayKey = secondCommand.ReplayKey
	if err := store.EnsureEffect(ctx, secondEffect); err != nil {
		t.Fatal(err)
	}
	secondClaim, err := store.Claim(
		ctx, run.ID, secondEffect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	completions := []Completion{
		{
			RunID: run.ID, EffectID: firstEffect.ID,
			Token: firstClaim.Token, EventKind: "group_uncertain", At: now,
		},
		{
			RunID: run.ID, EffectID: secondEffect.ID,
			Token: secondClaim.Token, EventKind: "group_uncertain", At: now,
		},
	}
	substituted := append([]Completion(nil), completions...)
	substituted[1].Token = strings.Repeat("0", 64)
	if err := store.ReconcileManyOwned(
		ctx,
		OwnerLease{},
		substituted,
		RecoveryAmbiguous,
	); !IsCode(err, "STALE_COMPLETION") {
		t.Fatalf("substituted group reconcile = %v", err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		byID[effect.ID] = effect
	}
	if byID[firstEffect.ID].State != Claimed ||
		byID[secondEffect.ID].State != Claimed {
		t.Fatalf(
			"partial reconcile escaped transaction: first=%#v second=%#v",
			byID[firstEffect.ID],
			byID[secondEffect.ID],
		)
	}
	if err := store.ReconcileManyOwned(
		ctx,
		OwnerLease{},
		completions,
		RecoveryAmbiguous,
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, effect := range snapshot.Effects {
		if (effect.ID == firstEffect.ID || effect.ID == secondEffect.ID) &&
			effect.State != Uncertain {
			t.Fatalf("group reconcile left %#v", effect)
		}
	}
}

func TestSnapshotRejectsCorruptCommandPayloadBinding(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
		value any
	}{
		{
			name:  "payload",
			query: `UPDATE commands SET payload = ? WHERE run_id = ? AND replay_key = ?`,
			value: []byte("mutated"),
		},
		{
			name:  "digest",
			query: `UPDATE commands SET payload_digest = ? WHERE run_id = ? AND replay_key = ?`,
			value: digest([]byte("not-the-payload")),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, run, command, _ := journalFixture(t)
			if _, err := store.conn.ExecContext(
				context.Background(),
				test.query,
				test.value,
				run.ID,
				command.ReplayKey,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Snapshot(
				context.Background(), run.ID,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("Snapshot() error = %v, want CORRUPT_JOURNAL", err)
			}
		})
	}
}

func TestCrashRecoveryAllOldAllNewAndAmbiguousAreExplicit(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	path := store.Path()
	now := time.Unix(1_700_000_100, 0).UTC()
	first, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	claimed, err := reopened.ClaimedEffects(ctx, run.ID)
	if err != nil || len(claimed) != 1 || claimed[0].CurrentClaim != first.Token {
		t.Fatalf("reopened claimed effects = %#v, err = %v", claimed, err)
	}
	if err := reopened.Reconcile(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: first.Token,
		EventKind: "reconciled_all_old", At: now.Add(time.Second),
	}, RecoveryAllOld); err != nil {
		t.Fatal(err)
	}
	pending, err := reopened.Effect(ctx, run.ID, effect.ID)
	if err != nil || pending.State != Pending || pending.CurrentClaim != "" {
		t.Fatalf("all-old effect = %#v, err = %v", pending, err)
	}
	second, err := reopened.Claim(
		ctx, run.ID, effect.ID, now.Add(2*time.Second), time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token {
		t.Fatal("reconciliation reused a finite CAS token")
	}
	if err := reopened.Reconcile(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: second.Token,
		State: Succeeded, Result: []byte("reconstructed"),
		Receipts:  []Receipt{{Kind: "reconstructed_receipt", Body: []byte("receipt-2")}},
		EventKind: "reconciled_all_new", At: now.Add(3 * time.Second),
	}, RecoveryAllNew); err != nil {
		t.Fatal(err)
	}
	succeeded, err := reopened.Effect(ctx, run.ID, effect.ID)
	if err != nil || succeeded.State != Succeeded {
		t.Fatalf("all-new effect = %#v, err = %v", succeeded, err)
	}

	secondStore, secondRun, _, secondEffect := journalFixture(t)
	ambiguousClaim, err := secondStore.Claim(
		ctx, secondRun.ID, secondEffect.ID, now, time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondStore.Reconcile(ctx, Completion{
		RunID: secondRun.ID, EffectID: secondEffect.ID, Token: ambiguousClaim.Token,
		EventKind: "reconciled_ambiguous", At: now.Add(time.Second),
	}, RecoveryAmbiguous); err != nil {
		t.Fatal(err)
	}
	ambiguous, err := secondStore.Effect(ctx, secondRun.ID, secondEffect.ID)
	if err != nil || ambiguous.State != Uncertain {
		t.Fatalf("ambiguous effect = %#v, err = %v", ambiguous, err)
	}
	if _, err := secondStore.Claim(
		ctx, secondRun.ID, secondEffect.ID, now.Add(time.Minute), time.Minute,
	); !IsCode(err, "EFFECT_NOT_CLAIMABLE") {
		t.Fatalf("uncertain effect claim = %v", err)
	}
}

func TestIdenticalReceiptBodiesRemainBoundToDistinctEffects(t *testing.T) {
	t.Parallel()

	store, run, _, firstEffect := journalFixture(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_150, 0).UTC()
	body := []byte("identical sealed handoff")

	firstClaim, err := store.Claim(ctx, run.ID, firstEffect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: firstEffect.ID, Token: firstClaim.Token,
		State: Succeeded, Result: body,
		Receipts:  []Receipt{{Kind: "sealed_handoff", Body: body}},
		EventKind: "first_completed", At: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	secondCommand := Command{
		RunID: run.ID, ReplayKey: "dispatch-2", Kind: "dispatch",
		Payload: []byte("input-2"), CreatedAt: run.CreatedAt,
	}
	secondEffect := Effect{
		RunID: run.ID, ID: "effect-2", ReplayKey: secondCommand.ReplayKey,
		Kind: "driver", BeforeDigest: digest([]byte("before-2")),
		ExpectedDigest: digest(body), UpdatedAt: run.CreatedAt,
	}
	if err := store.RecordCommand(ctx, secondCommand); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureEffect(ctx, secondEffect); err != nil {
		t.Fatal(err)
	}
	secondClaim, err := store.Claim(
		ctx, run.ID, secondEffect.ID, now.Add(2*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: secondEffect.ID, Token: secondClaim.Token,
		State: Succeeded, Result: body,
		Receipts:  []Receipt{{Kind: "sealed_handoff", Body: body}},
		EventKind: "second_completed", At: now.Add(3 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := store.conn.QueryRowContext(
		ctx,
		`SELECT count(*) FROM receipts
		  WHERE run_id = ? AND kind = ? AND body = ?`,
		run.ID, "sealed_handoff", body,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("bound identical receipt count = %d, want 2", count)
	}
}

func TestConcurrentClaimHasExactlyOneWinner(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_200, 0).UTC()
	const contenders = 8
	var (
		wait    sync.WaitGroup
		mu      sync.Mutex
		winners int
	)
	wait.Add(contenders)
	for range contenders {
		go func() {
			defer wait.Done()
			_, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				winners++
				return
			}
			if !IsCode(err, "EFFECT_NOT_CLAIMABLE") {
				t.Errorf("claim error = %v", err)
			}
		}()
	}
	wait.Wait()
	if winners != 1 {
		t.Fatalf("claim winners = %d, want 1", winners)
	}
}

func TestReadOnlyOpenDoesNotChangeJournalBytes(t *testing.T) {
	t.Parallel()

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := sha256.Sum256(before)
	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readOnly.Snapshot(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	var busyTimeout int
	if err := readOnly.conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil ||
		busyTimeout != 5000 {
		t.Fatalf("read-only busy timeout = %d, err = %v", busyTimeout, err)
	}
	if err := readOnly.AppendEvent(ctx, run.ID, "forbidden", nil, time.Now()); !IsCode(err, "READ_ONLY") {
		t.Fatalf("read-only write = %v", err)
	}
	if err := readOnly.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash != sha256.Sum256(after) {
		t.Fatal("read-only status changed journal bytes")
	}
}

func TestReadOnlyOpenRecoversHotRollbackJournal(t *testing.T) {
	const (
		helperEnv = "SWORN_TEST_HOT_ROLLBACK_HELPER"
		pathEnv   = "SWORN_TEST_HOT_ROLLBACK_PATH"
		runEnv    = "SWORN_TEST_HOT_ROLLBACK_RUN"
	)
	if os.Getenv(helperEnv) == "1" {
		ctx := context.Background()
		store, err := Open(ctx, os.Getenv(pathEnv))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.conn.ExecContext(
			ctx,
			"UPDATE runs SET target_ref = ? WHERE run_id = ?",
			"refs/heads/uncommitted",
			os.Getenv(runEnv),
		); err != nil {
			t.Fatal(err)
		}
		os.Exit(86)
	}

	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(
		os.Args[0],
		"-test.run=^TestReadOnlyOpenRecoversHotRollbackJournal$",
	)
	command.Env = append(
		os.Environ(),
		helperEnv+"=1",
		pathEnv+"="+path,
		runEnv+"="+run.ID,
	)
	output, err := command.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 86 {
		t.Fatalf("crash helper exit = %v, output = %s", err, output)
	}
	journalInfo, err := os.Stat(path + "-journal")
	if err != nil {
		t.Fatalf("crash helper did not leave a rollback journal: %v", err)
	}
	if journalInfo.Size() == 0 {
		t.Fatal("crash helper left an empty rollback journal")
	}

	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	snapshot, err := readOnly.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Run.TargetRef != run.TargetRef {
		t.Fatalf(
			"target ref after rollback = %q, want %q",
			snapshot.Run.TargetRef,
			run.TargetRef,
		)
	}
}

func TestOpenRejectsPermissionsAndSymlinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	insecure := filepath.Join(root, "insecure.sqlite")
	if err := os.WriteFile(insecure, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, insecure); !IsCode(err, "INSECURE_PERMISSIONS") {
		t.Fatalf("insecure journal = %v", err)
	}
	target := filepath.Join(root, "target.sqlite")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.sqlite")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, link); !IsCode(err, "INVALID_PATH") {
		t.Fatalf("symlink journal = %v", err)
	}
	symlinkParent := filepath.Join(root, "linked-parent")
	if err := os.Symlink(root, symlinkParent); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, filepath.Join(symlinkParent, "new.sqlite")); !IsCode(err, "INVALID_PATH") {
		t.Fatalf("symlink parent = %v", err)
	}
}

func TestOpenRejectsForeignApplicationAndSchemaIdentity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, _, _, _ := journalFixture(t)
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); !IsCode(err, "IDENTITY_MISMATCH") {
		t.Fatalf("foreign schema identity = %v", err)
	}
}

func downgradeExactV2ToV1(t *testing.T, path string, mutate bool) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		"DROP INDEX outbox_delivery_order",
		"DROP INDEX eval_records_by_run_offset",
		"DROP TABLE notification_outbox",
		"DROP TABLE eval_records",
		"DROP TABLE observer_cursors",
		"PRAGMA user_version = 1",
	}
	if mutate {
		statements = append(statements, "CREATE TABLE foreign_v1(value TEXT) STRICT")
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	got, err := schemaFingerprint(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if !mutate && got != legacySchemaIdentityDigest {
		t.Fatalf("legacy fingerprint = %s, want %s", got, legacySchemaIdentityDigest)
	}
}

func TestOpenMigratesOnlyExactV1AndPreservesExistingFacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, _ := journalFixture(t)
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeExactV2ToV1(t, path, false)

	readOnly, err := OpenReadOnly(ctx, path)
	if !IsCode(err, "IDENTITY_MISMATCH") || readOnly != nil {
		t.Fatalf("v1 read-only admission = %#v, %v", readOnly, err)
	}

	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	snapshot, err := migrated.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Run.ID != run.ID || len(snapshot.Commands) != 1 || len(snapshot.Effects) != 1 {
		t.Fatalf("migrated facts = %#v", snapshot)
	}
	if err := verifySchemaIdentity(ctx, migrated.conn); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsModifiedV1WithoutPartialMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, _, _, _ := journalFixture(t)
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeExactV2ToV1(t, path, true)

	if _, err := Open(ctx, path); !IsCode(err, "IDENTITY_MISMATCH") {
		t.Fatalf("modified v1 admission = %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, migratedTables int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(
		`SELECT count(*) FROM sqlite_schema
		 WHERE type='table' AND name IN
		       ('observer_cursors','eval_records','notification_outbox')`,
	).Scan(&migratedTables); err != nil {
		t.Fatal(err)
	}
	if version != 1 || migratedTables != 0 {
		t.Fatalf("modified v1 changed: version=%d tables=%d", version, migratedTables)
	}
}

func TestOpenRejectsMatchingPragmasWithForeignSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, _, _, _ := journalFixture(t)
	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE foreign_object(value TEXT) STRICT"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); !IsCode(err, "IDENTITY_MISMATCH") {
		t.Fatalf("foreign writable schema = %v", err)
	}
	if _, err := OpenReadOnly(ctx, path); !IsCode(err, "IDENTITY_MISMATCH") {
		t.Fatalf("foreign read-only schema = %v", err)
	}
}

func TestOperationalFailureCannotCarryVerdictReceiptOrAutoRetry(t *testing.T) {
	t.Parallel()

	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_300, 0).UTC()
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "transport_error",
		Receipts:  []Receipt{{Kind: "verdict", Body: []byte("pass")}},
		EventKind: "failed", At: now.Add(time.Second),
	})
	if !IsCode(err, "VERDICT_ON_OPERATIONAL_FAILURE") {
		t.Fatalf("failure with verdict = %v", err)
	}
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "transport_error",
		EventKind: "failed", At: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(
		ctx, run.ID, effect.ID, now.Add(2*time.Second), time.Minute,
	); !IsCode(err, "EFFECT_NOT_CLAIMABLE") {
		t.Fatalf("W3 automatically retried failed effect: %v", err)
	}
}

func TestTypedControlsAreGenerationCASIdempotentAndTerminal(t *testing.T) {
	t.Parallel()
	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	pause := ControlCommand{RunID: run.ID, ID: "pause-1", Kind: Pause}
	first, err := store.ApplyControl(ctx, pause, now)
	if err != nil || first.Generation != 1 {
		t.Fatalf("pause = %#v, %v", first, err)
	}
	if _, err := store.ClaimOwned(ctx, owner, effect.ID, now, time.Minute); !IsCode(err, "CONTROL_STOPPED") {
		t.Fatalf("claim after accepted pause = %v", err)
	}
	replay, err := store.ApplyControl(ctx, pause, now.Add(time.Hour))
	if err != nil || replay != first {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	conflict := pause
	conflict.Kind = Resume
	if _, err := store.ApplyControl(ctx, conflict, now); !IsCode(err, "REPLAY_CONFLICT") {
		t.Fatalf("conflict = %v", err)
	}
	resume := ControlCommand{RunID: run.ID, ID: "resume-1", Kind: Resume, ExpectedGeneration: 1}
	if _, err := store.ApplyControl(ctx, resume, now); err != nil {
		t.Fatal(err)
	}
	cancel := ControlCommand{RunID: run.ID, ID: "cancel-1", Kind: Cancel, ExpectedGeneration: 2}
	if _, err := store.ApplyControl(ctx, cancel, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "resume-2", Kind: Resume, ExpectedGeneration: 3,
	}, now); !IsCode(err, "STALE_CONTROL_GENERATION") {
		t.Fatalf("resume after cancel = %v", err)
	}
	projection, err := store.ControlProjection(ctx, run.ID)
	if err != nil || projection.Generation != 3 || projection.Desired != "cancelled" {
		t.Fatalf("projection = %#v, %v", projection, err)
	}
}

func TestOwnerLeaseRenewalTakeoverAndCompletionFencing(t *testing.T) {
	t.Parallel()
	store, run, _, effect := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	first, err := store.AcquireOwner(ctx, run.ID, now, time.Second, false)
	if err != nil || first.Generation != 1 {
		t.Fatalf("first owner = %#v, %v", first, err)
	}
	observed, present, err := store.CurrentOwner(ctx, run.ID)
	if err != nil || !present || observed != first {
		t.Fatalf("current first owner = %#v, %t, %v", observed, present, err)
	}
	first, err = store.RenewOwner(ctx, first, now.Add(200*time.Millisecond), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	observed, present, err = store.CurrentOwner(ctx, run.ID)
	if err != nil || !present || observed != first {
		t.Fatalf("renewed current owner = %#v, %t, %v", observed, present, err)
	}
	claim, err := store.ClaimOwned(ctx, first, effect.ID, now.Add(300*time.Millisecond), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "takeover-early", Kind: Takeover,
	}, now.Add(500*time.Millisecond)); !IsCode(err, "OWNER_ACTIVE") {
		t.Fatalf("early takeover = %v", err)
	}
	takeoverAt := first.ExpiresAt.Add(time.Nanosecond)
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "takeover-1", Kind: Takeover,
	}, takeoverAt); err != nil {
		t.Fatal(err)
	}
	second, err := store.AcquireOwner(ctx, run.ID, takeoverAt, time.Second, true)
	if err != nil || second.Generation != 2 {
		t.Fatalf("second owner = %#v, %v", second, err)
	}
	observed, present, err = store.CurrentOwner(ctx, run.ID)
	if err != nil || !present || observed != second {
		t.Fatalf("current second owner = %#v, %t, %v", observed, present, err)
	}
	completion := Completion{RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: Succeeded, Result: []byte("late"), EventKind: "late", At: takeoverAt}
	if err := store.CompleteOwned(ctx, first, completion); !IsCode(err, "OWNER_FENCED") {
		t.Fatalf("old completion = %v", err)
	}
	if err := store.ReconcileOwned(ctx, second, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		EventKind: "uncertain", At: takeoverAt,
	}, RecoveryAmbiguous); err != nil {
		t.Fatal(err)
	}
}

func TestRetryEpochRequiresExactThreeTryExhaustion(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	work := digest([]byte("bounded-work"))
	for try := int64(1); try <= 3; try++ {
		id := AttemptEffectID(work, 1, try)
		command := Command{RunID: run.ID, ReplayKey: id, Kind: "driver.dispatch",
			Payload: []byte("attempt"), CreatedAt: now}
		effect := Effect{RunID: run.ID, ID: id, ReplayKey: id, Kind: command.Kind,
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now}
		if err := store.EnsureAttempt(ctx, command, effect,
			EffectAttempt{WorkID: work, Epoch: 1, Try: try}); err != nil {
			t.Fatal(err)
		}
		claim, err := store.Claim(ctx, run.ID, id, now, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Complete(ctx, Completion{RunID: run.ID, EffectID: id,
			Token: claim.Token, State: OperationalFailed, ErrorCode: "transport",
			EventKind: "failed", At: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.EnsureAttempt(ctx, Command{}, Effect{},
		EffectAttempt{WorkID: work, Epoch: 1, Try: 4}); !IsCode(err, "INVALID_EFFECT_ATTEMPT") {
		t.Fatalf("try four = %v", err)
	}
	receipt, err := store.ApplyControl(ctx, ControlCommand{RunID: run.ID,
		ID: "retry-1", Kind: Retry, WorkID: work, ExpectedEpoch: 1}, now)
	if err != nil || receipt.Epoch != 2 {
		t.Fatalf("retry = %#v, %v", receipt, err)
	}
	oldID := AttemptEffectID(work, 1, 1)
	if err := store.EnsureAttempt(ctx, Command{RunID: run.ID, ReplayKey: oldID,
		Kind: "driver.dispatch", Payload: []byte("attempt"), CreatedAt: now},
		Effect{RunID: run.ID, ID: oldID, ReplayKey: oldID, Kind: "driver.dispatch",
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now}, EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); !IsCode(err, "STALE_RETRY_EPOCH") {
		t.Fatalf("stale epoch replay = %v", err)
	}
	id := AttemptEffectID(work, 2, 1)
	if err := store.EnsureAttempt(ctx, Command{RunID: run.ID, ReplayKey: id,
		Kind: "driver.dispatch", Payload: []byte("retry"), CreatedAt: now},
		Effect{RunID: run.ID, ID: id, ReplayKey: id, Kind: "driver.dispatch",
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now}, EffectAttempt{WorkID: work, Epoch: 2, Try: 1}); err != nil {
		t.Fatal(err)
	}
}
