package journal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func emptyCatalogStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func catalogRun(id string, createdAt time.Time) Run {
	return Run{
		ID:             id,
		ManifestDigest: digest([]byte("manifest-" + id)),
		Repository:     "/repository/" + id,
		Release:        "release-" + id,
		TargetRef:      "refs/heads/main",
		CreatedAt:      createdAt,
	}
}

func TestRunBindingsReturnsEmptyAndDeterministicallyOrderedCatalog(t *testing.T) {
	t.Parallel()

	store, _ := emptyCatalogStore(t)
	ctx := context.Background()
	empty, err := store.RunBindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty bindings = %#v", empty)
	}

	firstAt := time.Unix(1_700_000_000, 1).UTC()
	secondAt := firstAt.Add(time.Second)
	inserted := []Run{
		catalogRun("run-z", secondAt),
		catalogRun("run-b", firstAt),
		catalogRun("run-a", firstAt),
	}
	for _, run := range inserted {
		if err := store.RegisterRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.RunBindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Run{inserted[2], inserted[1], inserted[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings = %#v, want %#v", got, want)
	}
}

func TestRunBindingsReadOnlyDoesNotChangeJournal(t *testing.T) {
	t.Parallel()

	store, path := emptyCatalogStore(t)
	ctx := context.Background()
	run := catalogRun("run-read-only", time.Unix(1_700_000_000, 0).UTC())
	if err := store.RegisterRun(ctx, run); err != nil {
		t.Fatal(err)
	}
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
	got, err := readOnly.RunBindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], run) {
		t.Fatalf("read-only bindings = %#v, want %#v", got, run)
	}
	if err := readOnly.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash != sha256.Sum256(after) {
		t.Fatal("run catalog read changed journal bytes")
	}
}

func TestRunBindingsRejectsMalformedTimeAndOversizedCatalog(t *testing.T) {
	t.Run("malformed timestamp", func(t *testing.T) {
		t.Parallel()
		store, _ := emptyCatalogStore(t)
		ctx := context.Background()
		run := catalogRun("run-corrupt", time.Unix(1_700_000_000, 0).UTC())
		if err := store.RegisterRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		if err := store.immediate(ctx, func(conn *sql.Conn) error {
			_, err := conn.ExecContext(
				ctx,
				"UPDATE runs SET created_at=? WHERE run_id=?",
				"not-a-time",
				run.ID,
			)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RunBindings(ctx); !IsCode(err, "CORRUPT_JOURNAL") {
			t.Fatalf("malformed timestamp error = %v", err)
		}
	})

	t.Run("resource limit", func(t *testing.T) {
		t.Parallel()
		store, _ := emptyCatalogStore(t)
		ctx := context.Background()
		createdAt, err := canonicalTime(time.Unix(1_700_000_000, 0).UTC())
		if err != nil {
			t.Fatal(err)
		}
		if err := store.immediate(ctx, func(conn *sql.Conn) error {
			statement, err := conn.PrepareContext(
				ctx,
				`INSERT INTO runs(
					run_id, manifest_digest, repository, release_id,
					target_ref, created_at
				) VALUES(?, ?, ?, ?, ?, ?)`,
			)
			if err != nil {
				return err
			}
			defer statement.Close()
			for index := 0; index <= MaxRunBindings; index++ {
				id := fmt.Sprintf("run-%04d", index)
				if _, err := statement.ExecContext(
					ctx,
					id,
					digest([]byte(id)),
					"/repository",
					"release",
					"refs/heads/main",
					createdAt,
				); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.RunBindings(ctx); !IsCode(err, "RESOURCE_LIMIT") {
			t.Fatalf("oversized catalog error = %v", err)
		}
	})
}
