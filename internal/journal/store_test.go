package journal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
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
	if _, err := db.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); !IsCode(err, "IDENTITY_MISMATCH") {
		t.Fatalf("foreign schema identity = %v", err)
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
