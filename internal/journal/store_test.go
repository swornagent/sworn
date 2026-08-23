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

func TestRecordCommandEffectIsAtomicAndConflictSafe(t *testing.T) {
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_100, 0).UTC()
	command := Command{
		RunID: run.ID, ReplayKey: "atomic-admission", Kind: "approval",
		Payload: []byte(`{"exact":true}`), CreatedAt: now,
	}
	effect := Effect{
		RunID: run.ID, ID: "atomic-admission", ReplayKey: command.ReplayKey,
		Kind: "approval.admit", BeforeDigest: digest(command.Payload),
		ExpectedDigest: digest([]byte("result")), UpdatedAt: now,
	}
	if err := store.RecordCommandEffect(ctx, command, effect); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundCommand, foundEffect := false, false
	for _, value := range snapshot.Commands {
		foundCommand = foundCommand || value.ReplayKey == command.ReplayKey
	}
	for _, value := range snapshot.Effects {
		foundEffect = foundEffect || value.ID == effect.ID
	}
	if !foundCommand || !foundEffect {
		t.Fatalf("atomic pair missing: command=%t effect=%t", foundCommand, foundEffect)
	}
	conflict := effect
	conflict.ExpectedDigest = digest([]byte("different"))
	if err := store.RecordCommandEffect(ctx, command, conflict); !IsCode(err, "EFFECT_CONFLICT") {
		t.Fatalf("effect conflict = %v", err)
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
	if err := os.Chmod(insecure, 0o644); err != nil {
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
	if _, err := db.Exec("PRAGMA user_version = 4"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); !IsCode(err, "IDENTITY_MISMATCH") {
		t.Fatalf("foreign schema identity = %v", err)
	}
}

func downgradeExactV3ToV2(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// ALTER TABLE DROP COLUMN restores the pre-S6 attempts text byte-for-
	// byte, so the recovered catalog fingerprints as the preserved v2 gate.
	statements := []string{
		"ALTER TABLE attempts DROP COLUMN observation_partial",
		"ALTER TABLE attempts DROP COLUMN observation_body",
		"PRAGMA user_version = 2",
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
	if got != schemaIdentityDigestV2 {
		t.Fatalf("recovered v2 fingerprint = %s, want %s", got, schemaIdentityDigestV2)
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
		// The live journal is v3: drop the S6 columns first so the attempts
		// catalog text is exactly the historical v1/v2 shape again.
		"ALTER TABLE attempts DROP COLUMN observation_partial",
		"ALTER TABLE attempts DROP COLUMN observation_body",
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
	migratedFingerprint, err := schemaFingerprint(ctx, migrated.conn)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := Open(ctx, filepath.Join(t.TempDir(), "fresh-v3.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	freshFingerprint, err := schemaFingerprint(ctx, fresh.conn)
	if err != nil {
		t.Fatal(err)
	}
	if migratedFingerprint != freshFingerprint ||
		freshFingerprint != schemaIdentityDigest {
		t.Fatalf(
			"fresh/migrated parity = fresh %s migrated %s want %s",
			freshFingerprint,
			migratedFingerprint,
			schemaIdentityDigest,
		)
	}
}

func TestOpenMigratesOnlyExactV2AndPreservesExistingFacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	path := store.Path()
	// Seed a historical failed attempt in the pre-S6 shape: an attempt row
	// with a digest and no observation body.
	now := run.CreatedAt.Add(time.Second)
	claim, err := store.Claim(ctx, run.ID, effect.ID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, Completion{
		RunID: run.ID, EffectID: effect.ID, Token: claim.Token,
		State: OperationalFailed, ErrorCode: "runner_error",
		Attempt: &Attempt{
			Number:            1,
			Responsibility:    "implementer_implementation",
			TransportStatus:   "runner_error",
			ObservationDigest: digest([]byte("observation")),
			Usage:             []byte(`{"token_status":"unavailable"}`),
		},
		EventKind: "dispatch_operational_failure",
		At:        now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeExactV3ToV2(t, path)

	readOnly, err := OpenReadOnly(ctx, path)
	if !IsCode(err, "IDENTITY_MISMATCH") || readOnly != nil {
		t.Fatalf("v2 read-only admission = %#v, %v", readOnly, err)
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
	// The historical row reads as distinguishably absent — Stored=false,
	// never CORRUPT_JOURNAL — and its pre-S6 facts are intact.
	observed, err := migrated.AttemptObservation(ctx, run.ID, effect.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Stored || observed.Partial || observed.Body != nil {
		t.Fatalf("historical attempt observation = %#v, want stored=false", observed)
	}
	if observed.Number != 1 ||
		observed.Responsibility != "implementer_implementation" ||
		observed.Transport != "runner_error" ||
		observed.Digest != digest([]byte("observation")) {
		t.Fatalf("historical attempt facts = %#v", observed)
	}
	if err := verifySchemaIdentity(ctx, migrated.conn); err != nil {
		t.Fatal(err)
	}
	migratedFingerprint, err := schemaFingerprint(ctx, migrated.conn)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := Open(ctx, filepath.Join(t.TempDir(), "fresh-v3.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	freshFingerprint, err := schemaFingerprint(ctx, fresh.conn)
	if err != nil {
		t.Fatal(err)
	}
	if migratedFingerprint != freshFingerprint ||
		freshFingerprint != schemaIdentityDigest {
		t.Fatalf(
			"fresh/migrated parity = fresh %s migrated %s want %s",
			freshFingerprint,
			migratedFingerprint,
			schemaIdentityDigest,
		)
	}
}

func TestV1MigrationNormalizesTheBoundedAttemptWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, run, _, effect := journalFixture(t)
	path := store.Path()
	usage := []byte(`{"token_status":"unavailable"}`)
	base := run.CreatedAt.Truncate(time.Second).Add(time.Second)
	olderBoundary := base.Add(100 * time.Millisecond)
	newerBoundary := base.Add(110 * time.Millisecond)
	later := base.Add(time.Second)
	if err := store.immediate(ctx, func(conn *sql.Conn) error {
		for attempt := 1; attempt <= MaxObservationAttempts+1; attempt++ {
			at := later
			switch attempt {
			case 1:
				at = olderBoundary
			case 2:
				at = newerBoundary
			}
			if _, err := conn.ExecContext(
				ctx,
				`INSERT INTO attempts(
				     run_id, effect_id, attempt, responsibility,
				     transport_status, observation_digest, usage_digest,
				     usage, created_at
				 ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				run.ID,
				effect.ID,
				attempt,
				"implementer_implementation",
				"completed",
				digest([]byte("observation")),
				digest(usage),
				usage,
				at.UTC().Format(time.RFC3339Nano),
			); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeExactV2ToV1(t, path, false)

	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	observation, err := migrated.ReadObservation(
		ctx,
		run.ID,
		MaxObservationAttempts,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.Attempts) != MaxObservationAttempts {
		t.Fatalf(
			"attempt window = %d, want %d",
			len(observation.Attempts),
			MaxObservationAttempts,
		)
	}
	var olderPresent, newerPresent bool
	for index, attempt := range observation.Attempts {
		if index > 0 &&
			attempt.CreatedAt.Before(observation.Attempts[index-1].CreatedAt) {
			t.Fatalf(
				"attempt window is not chronological at %d: %s before %s",
				index,
				attempt.CreatedAt,
				observation.Attempts[index-1].CreatedAt,
			)
		}
		switch attempt.Number {
		case 1:
			olderPresent = true
		case 2:
			newerPresent = true
		}
	}
	if olderPresent || !newerPresent {
		t.Fatalf(
			"migration selected boundary attempts: older=%t newer=%t",
			olderPresent,
			newerPresent,
		)
	}
	rows, err := migrated.conn.QueryContext(
		ctx,
		`SELECT created_at FROM attempts
		 WHERE run_id=? ORDER BY attempt`,
		run.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			t.Fatal(err)
		}
		parsed, err := parseTime(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if want := parsed.UTC().Format(
			"2006-01-02T15:04:05.000000000Z",
		); encoded != want {
			t.Fatalf("migrated timestamp = %q, want %q", encoded, want)
		}
	}
	if err := rows.Err(); err != nil {
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

func TestReleaseOwnerExpiredCurrentLeaseSucceeds(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	leaseDuration := 500 * time.Millisecond
	owner, err := store.AcquireOwner(ctx, run.ID, now, leaseDuration, false)
	if err != nil || owner.Generation != 1 {
		t.Fatalf("acquire owner = %#v, %v", owner, err)
	}
	observed, present, err := store.CurrentOwner(ctx, run.ID)
	if err != nil || !present || observed != owner {
		t.Fatalf("current owner = %#v, %t, %v", observed, present, err)
	}

	// Release past expiry when still current must succeed (A1).
	afterExpiry := owner.ExpiresAt.Add(time.Second)
	if err := store.ReleaseOwner(ctx, owner, afterExpiry); err != nil {
		t.Fatalf("release of expired current owner = %v, want success", err)
	}

	// Effect returns to pending; no active/claimed owner remains.
	observed, present, err = store.CurrentOwner(ctx, run.ID)
	if err != nil || present {
		t.Fatalf("current owner after release = %#v, present=%t, want false", observed, present)
	}

	// An ordinary non-takeover acquire succeeds next.
	second, err := store.AcquireOwner(ctx, run.ID, afterExpiry.Add(time.Second), time.Minute, false)
	if err != nil || second.Generation != 2 {
		t.Fatalf("ordinary acquire after expired release = %#v, %v", second, err)
	}
}

func TestReleaseOwnerAfterTakeoverIsFenced(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	first, err := store.AcquireOwner(ctx, run.ID, now, 500*time.Millisecond, false)
	if err != nil || first.Generation != 1 {
		t.Fatalf("first owner = %#v, %v", first, err)
	}

	takeoverAt := first.ExpiresAt.Add(time.Nanosecond)
	if _, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "takeover-fencing", Kind: Takeover,
	}, takeoverAt); err != nil {
		t.Fatal(err)
	}
	second, err := store.AcquireOwner(ctx, run.ID, takeoverAt, time.Minute, true)
	if err != nil || second.Generation != 2 {
		t.Fatalf("takeover owner = %#v, %v", second, err)
	}

	// Superseded first owner attempting release fails OWNER_FENCED (A2).
	if err := store.ReleaseOwner(ctx, first, takeoverAt.Add(time.Second)); !IsCode(err, "OWNER_FENCED") {
		t.Fatalf("superseded owner release = %v, want OWNER_FENCED", err)
	}

	// Second owner remains active current owner untouched.
	observed, present, err := store.CurrentOwner(ctx, run.ID)
	if err != nil || !present || observed != second {
		t.Fatalf("current owner after superseded release attempt = %#v, %t, %v", observed, present, err)
	}
}

func TestRenewOwnerExpiredLeaseRefused(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	owner, err := store.AcquireOwner(ctx, run.ID, now, 500*time.Millisecond, false)
	if err != nil || owner.Generation != 1 {
		t.Fatalf("acquire owner = %#v, %v", owner, err)
	}

	// Expired lease renewal must remain refused with OWNER_FENCED (A3).
	afterExpiry := owner.ExpiresAt.Add(time.Second)
	if _, err := store.RenewOwner(ctx, owner, afterExpiry, time.Minute); !IsCode(err, "OWNER_FENCED") {
		t.Fatalf("expired lease renewal = %v, want OWNER_FENCED", err)
	}

	// Current owner is still un-renewed expired lease.
	observed, present, err := store.CurrentOwner(ctx, run.ID)
	if err != nil || !present || observed != owner {
		t.Fatalf("current owner after refused renewal = %#v, %t, %v", observed, present, err)
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

func TestAnySucceededTryClosesRefusedPaymentBypass(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	work := digest([]byte("single-payment-work"))
	ensure := func(try int64) error {
		id := AttemptEffectID(work, 1, try)
		command := Command{RunID: run.ID, ReplayKey: id, Kind: "driver.dispatch",
			Payload: []byte("attempt"), CreatedAt: now}
		effect := Effect{RunID: run.ID, ID: id, ReplayKey: id, Kind: command.Kind,
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now}
		return store.EnsureAttempt(ctx, command, effect,
			EffectAttempt{WorkID: work, Epoch: 1, Try: try})
	}
	succeed := func(try int64) {
		id := AttemptEffectID(work, 1, try)
		claim, err := store.Claim(ctx, run.ID, id, now, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Complete(ctx, Completion{RunID: run.ID, EffectID: id,
			Token: claim.Token, State: Succeeded, Result: []byte("paid"),
			EventKind: "succeeded", At: now}); err != nil {
			t.Fatal(err)
		}
	}

	// t1 succeeds; the driver's recovery/replay path re-ensures the same
	// try, which must stay idempotent (the current effect is excluded from
	// the any-succeeded-try scan).
	if err := ensure(1); err != nil {
		t.Fatal(err)
	}
	succeed(1)
	if err := ensure(1); err != nil {
		t.Fatalf("replay of succeeded try = %v", err)
	}

	// A second try in the same epoch is refused by the any-succeeded-try
	// guard before it journals (and before the previous-try rule, which
	// would have reported PREVIOUS_ATTEMPT_NOT_RETRYABLE instead).
	if err := ensure(2); !IsCode(err, "WORK_ALREADY_SUCCEEDED") {
		t.Fatalf("try two after success = %v", err)
	}

	// The bypass the guard closes: t2 was refused before journaling any
	// effect row, so t3's previous attempt is absent and the
	// immediately-previous-try rule alone would retry it and re-pay; the
	// any-succeeded-try scan sees t1's success and refuses instead.
	if err := ensure(3); !IsCode(err, "WORK_ALREADY_SUCCEEDED") {
		t.Fatalf("try three after refused absent try = %v", err)
	}
}

func TestAnySucceededTryGuardAllowsNormalRetryAndDerivedEffects(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)
	ensure := func(work string, try int64) error {
		id := AttemptEffectID(work, 1, try)
		command := Command{RunID: run.ID, ReplayKey: id, Kind: "driver.dispatch",
			Payload: []byte("attempt"), CreatedAt: now}
		effect := Effect{RunID: run.ID, ID: id, ReplayKey: id, Kind: command.Kind,
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now}
		return store.EnsureAttempt(ctx, command, effect,
			EffectAttempt{WorkID: work, Epoch: 1, Try: try})
	}
	fail := func(work string, try int64) {
		id := AttemptEffectID(work, 1, try)
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

	// Operational failures never trip the guard: every later try within the
	// budget still retries normally.
	work := digest([]byte("retry-work"))
	for try := int64(1); try <= 3; try++ {
		if err := ensure(work, try); err != nil {
			t.Fatalf("try %d after operational failures = %v", try, err)
		}
		fail(work, try)
	}

	// A derived effect hanging off an attempt id as a longer path (for
	// example the human-park checkpoint) that succeeded must not block its
	// own cycle's later work: the guard counts try-level effects only.
	derivedWork := digest([]byte("derived-work"))
	childID := AttemptEffectID(derivedWork, 1, 1) + "/human-park"
	if err := store.RecordCommandEffect(ctx,
		Command{RunID: run.ID, ReplayKey: childID,
			Kind: "runtime.human_park", Payload: []byte("park"), CreatedAt: now},
		Effect{RunID: run.ID, ID: childID, ReplayKey: childID,
			Kind: "runtime.human_park", BeforeDigest: digest([]byte("parent")),
			ExpectedDigest: digest([]byte("park")), UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	childClaim, err := store.Claim(ctx, run.ID, childID, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, Completion{RunID: run.ID, EffectID: childID,
		Token: childClaim.Token, State: Succeeded, Result: []byte("park"),
		EventKind: "park_checkpointed", At: now}); err != nil {
		t.Fatal(err)
	}
	if err := ensure(derivedWork, 1); err != nil {
		t.Fatalf("try one with succeeded derived child = %v", err)
	}
	fail(derivedWork, 1)
	if err := ensure(derivedWork, 2); err != nil {
		t.Fatalf("try two with succeeded derived child = %v", err)
	}
}

func TestDerivedWorkInheritsParentCycleRetryEpoch(t *testing.T) {
	t.Parallel()
	store, run, _, _ := journalFixture(t)
	ctx := context.Background()
	now := run.CreatedAt.Add(time.Second)

	cycleWork := digest([]byte("seal-cycle-work"))
	dispatchWork := digest([]byte("derived-dispatch-work"))

	// Epoch 1: cycleWork and dispatchWork both run 3 tries and fail (OperationalFailed at try 3)
	for try := int64(1); try <= 3; try++ {
		// Cycle work
		cycleEffectID := AttemptEffectID(cycleWork, 1, try)
		cycleCmd := Command{RunID: run.ID, ReplayKey: cycleEffectID, Kind: "git.seal",
			Payload: []byte("cycle"), CreatedAt: now}
		cycleEff := Effect{RunID: run.ID, ID: cycleEffectID, ReplayKey: cycleEffectID, Kind: cycleCmd.Kind,
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")), UpdatedAt: now}
		if err := store.EnsureAttempt(ctx, cycleCmd, cycleEff,
			EffectAttempt{WorkID: cycleWork, Epoch: 1, Try: try}); err != nil {
			t.Fatalf("ensure cycle try %d: %v", try, err)
		}
		claim, err := store.Claim(ctx, run.ID, cycleEffectID, now, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Complete(ctx, Completion{RunID: run.ID, EffectID: cycleEffectID,
			Token: claim.Token, State: OperationalFailed, ErrorCode: "transport",
			EventKind: "failed", At: now}); err != nil {
			t.Fatal(err)
		}

		// Derived dispatch work
		dispatchEffectID := AttemptEffectID(dispatchWork, 1, try)
		dispatchCmd := Command{RunID: run.ID, ReplayKey: dispatchEffectID, Kind: "driver.dispatch",
			Payload: []byte("dispatch"), CreatedAt: now}
		dispatchEff := Effect{RunID: run.ID, ID: dispatchEffectID, ReplayKey: dispatchEffectID, Kind: dispatchCmd.Kind,
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")), UpdatedAt: now}
		if err := store.EnsureAttempt(ctx, dispatchCmd, dispatchEff,
			EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: try}); err != nil {
			t.Fatalf("ensure dispatch try %d: %v", try, err)
		}
		claim, err = store.Claim(ctx, run.ID, dispatchEffectID, now, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Complete(ctx, Completion{RunID: run.ID, EffectID: dispatchEffectID,
			Token: claim.Token, State: OperationalFailed, ErrorCode: "transport",
			EventKind: "failed", At: now}); err != nil {
			t.Fatal(err)
		}
	}

	// Retry ONLY the cycle work (ExpectedEpoch: 1)
	receipt, err := store.ApplyControl(ctx, ControlCommand{
		RunID: run.ID, ID: "retry-cycle-1", Kind: Retry, WorkID: cycleWork, ExpectedEpoch: 1,
	}, now)
	if err != nil || receipt.Epoch != 2 {
		t.Fatalf("retry cycle work = %#v, %v", receipt, err)
	}

	// Admit cycle work at epoch 2 try 1
	cycleEpoch2ID := AttemptEffectID(cycleWork, 2, 1)
	if err := store.EnsureAttempt(ctx,
		Command{RunID: run.ID, ReplayKey: cycleEpoch2ID, Kind: "git.seal",
			Payload: []byte("cycle-retry"), CreatedAt: now},
		Effect{RunID: run.ID, ID: cycleEpoch2ID, ReplayKey: cycleEpoch2ID, Kind: "git.seal",
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now},
		EffectAttempt{WorkID: cycleWork, Epoch: 2, Try: 1},
	); err != nil {
		t.Fatalf("ensure cycle at epoch 2: %v", err)
	}

	// Derived work at epoch 2 try 1 (was not directly retried via ApplyControl, so projection.RetryEpochs has no entry for it)
	// It should inherit epoch 2 and succeed with no STALE_RETRY_EPOCH
	dispatchEpoch2ID := AttemptEffectID(dispatchWork, 2, 1)
	if err := store.EnsureAttempt(ctx,
		Command{RunID: run.ID, ReplayKey: dispatchEpoch2ID, Kind: "driver.dispatch",
			Payload: []byte("dispatch-retry"), CreatedAt: now},
		Effect{RunID: run.ID, ID: dispatchEpoch2ID, ReplayKey: dispatchEpoch2ID, Kind: "driver.dispatch",
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now},
		EffectAttempt{WorkID: dispatchWork, Epoch: 2, Try: 1},
	); err != nil {
		t.Fatalf("ensure derived dispatch at epoch 2: %v", err)
	}

	// Read back from journal effects table
	eff, err := store.Effect(ctx, run.ID, dispatchEpoch2ID)
	if err != nil {
		t.Fatalf("read back derived dispatch effect at epoch 2: %v", err)
	}
	if eff.ID != dispatchEpoch2ID || eff.State != Pending {
		t.Fatalf("derived dispatch effect = %#v", eff)
	}

	// Verify superseded replay at epoch 1 still fails closed with STALE_RETRY_EPOCH
	dispatchOldID := AttemptEffectID(dispatchWork, 1, 1)
	if err := store.EnsureAttempt(ctx,
		Command{RunID: run.ID, ReplayKey: dispatchOldID, Kind: "driver.dispatch",
			Payload: []byte("dispatch-old"), CreatedAt: now},
		Effect{RunID: run.ID, ID: dispatchOldID, ReplayKey: dispatchOldID, Kind: "driver.dispatch",
			BeforeDigest: digest([]byte("before")), ExpectedDigest: digest([]byte("after")),
			UpdatedAt: now},
		EffectAttempt{WorkID: dispatchWork, Epoch: 1, Try: 1},
	); !IsCode(err, "STALE_RETRY_EPOCH") {
		t.Fatalf("stale derived dispatch replay = %v, want STALE_RETRY_EPOCH", err)
	}
}
