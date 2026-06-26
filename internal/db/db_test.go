package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// tempDB creates a temporary database file, opens it, and returns the path,
// handle, and a cleanup function. The caller must call the cleanup function.
func tempDB(t *testing.T) (string, *sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("tempDB: Open(%s): %v", dbPath, err)
	}
	return dbPath, conn, func() {
		conn.Close()
		os.Remove(dbPath)
	}
}

func TestSchemaCreationIdempotent(t *testing.T) {
	// AC4: Subsequent runs do not re-run migrations or error on
	// an already-initialised schema.
	dbPath, conn, cleanup := tempDB(t)
	defer cleanup()

	// Verify tables exist after first open.
	var tableCount int
	row := conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('tracks','events','schema_version')")
	if err := row.Scan(&tableCount); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 3 {
		t.Fatalf("expected 3 tables, got %d", tableCount)
	}

	// Verify schema version.
	var version int
	row = conn.QueryRow("SELECT version FROM schema_version LIMIT 1")
	if err := row.Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", SchemaVersion, version)
	}

	conn.Close()

	// Re-open — should not error.
	conn2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer conn2.Close()

	// Same schema should still be present.
	var tableCount2 int
	row2 := conn2.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('tracks','events','schema_version')")
	if err := row2.Scan(&tableCount2); err != nil {
		t.Fatalf("count tables after re-open: %v", err)
	}
	if tableCount2 != 3 {
		t.Fatalf("expected 3 tables after re-open, got %d", tableCount2)
	}
}

func TestConcurrentWrites(t *testing.T) {
	// AC7 (extended): 8 goroutines insert rows concurrently; all succeed;
	// no corruption; final row count matches insertion count.
	_, conn, cleanup := tempDB(t)
	defer cleanup()

	const goroutines = 8
	const insertsPerGoroutine = 10

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < insertsPerGoroutine; i++ {
				trackID := "T" + itoa(gid*insertsPerGoroutine+i)
				_, err := conn.Exec(
					`INSERT INTO tracks (id, release, pid, state, current_slice, started_at)
					 VALUES (?, ?, ?, ?, ?, ?)`,
					trackID, "test-release", 0, "planned", "", "",
				)
				if err != nil {
					errs <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent insert error: %v", err)
	}

	var total int
	row := conn.QueryRow("SELECT COUNT(*) FROM tracks")
	if err := row.Scan(&total); err != nil {
		t.Fatalf("count rows: %v", err)
	}

	expected := goroutines * insertsPerGoroutine
	if total != expected {
		t.Fatalf("expected %d rows, got %d", expected, total)
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath("/tmp/workspace")
	expected := "/tmp/workspace/.sworn/sworn.db"
	if path != expected {
		t.Fatalf("DefaultPath: expected %q, got %q", expected, path)
	}
}

func TestOpenCreatesDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "sub", "test.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open with nested dirs: %v", err)
	}
	defer conn.Close()

	// Verify the file was created.
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}

// itoa is a simple int to string converter (no import needed).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
