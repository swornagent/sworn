package db

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestSelfIgnoreWritten covers AC-01: opening a DB under .sworn/ writes
// .sworn/.gitignore containing "*", and both the run DB and the supervisor DB
// (which route through the same db.Open) yield the same ignore.
func TestSelfIgnoreWritten(t *testing.T) {
	dir := t.TempDir()
	swornDir := filepath.Join(dir, DefaultDir)
	gitignore := filepath.Join(swornDir, ".gitignore")

	// Run DB.
	conn, err := Open(filepath.Join(swornDir, DefaultName))
	if err != nil {
		t.Fatalf("Open run DB: %v", err)
	}
	conn.Close()

	got, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("read .sworn/.gitignore after run-DB open: %v", err)
	}
	if string(got) != "*\n" {
		t.Fatalf("run DB: expected .gitignore %q, got %q", "*\n", string(got))
	}

	// Supervisor DB routes through the same db.Open (see
	// internal/supervisor/supervisor.go) — its open must leave the same ignore.
	sup, err := Open(filepath.Join(swornDir, "supervisor-r.db"))
	if err != nil {
		t.Fatalf("Open supervisor DB: %v", err)
	}
	sup.Close()

	got2, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("read .sworn/.gitignore after supervisor-DB open: %v", err)
	}
	if string(got2) != "*\n" {
		t.Fatalf("supervisor DB: expected .gitignore %q, got %q", "*\n", string(got2))
	}
}

// TestSelfIgnoreNotOverwritten covers AC-02: a pre-existing .sworn/.gitignore is
// left byte-for-byte untouched (operator customisation respected via O_EXCL).
func TestSelfIgnoreNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	swornDir := filepath.Join(dir, DefaultDir)
	if err := os.MkdirAll(swornDir, 0o755); err != nil {
		t.Fatalf("mkdir .sworn: %v", err)
	}
	gitignore := filepath.Join(swornDir, ".gitignore")
	custom := "# operator-customised\nsworn.db\n!keep-me\n"
	if err := os.WriteFile(gitignore, []byte(custom), 0o644); err != nil {
		t.Fatalf("pre-write custom .gitignore: %v", err)
	}

	conn, err := Open(filepath.Join(swornDir, DefaultName))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	got, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(got) != custom {
		t.Fatalf("existing .gitignore was modified: expected %q, got %q", custom, string(got))
	}
}

// TestSelfIgnoreHidesSwornDir covers AC-03 (reachability): inside a freshly
// git-inited repo, opening the sworn DB leaves .sworn/ absent from
// `git status --porcelain`.
func TestSelfIgnoreHidesSwornDir(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH; skipping porcelain reachability check")
	}
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	conn, err := Open(filepath.Join(repo, DefaultDir, DefaultName))
	if err != nil {
		t.Fatalf("Open under git repo: %v", err)
	}
	defer conn.Close()

	cmd := exec.Command(git, "status", "--porcelain")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	if strings.Contains(string(out), DefaultDir) {
		t.Fatalf("expected .sworn/ absent from porcelain status, got:\n%s", out)
	}
}

// TestSelfIgnoreBestEffort covers AC-04: when the .gitignore write cannot
// succeed (here: a directory pre-exists at the .gitignore path, so O_EXCL
// create fails), Open must still return a working DB — the DB-open path never
// depends on the courtesy write. This is distinct from AC-02's existing-file
// case: here the target is unwritable, not a file to preserve.
func TestSelfIgnoreBestEffort(t *testing.T) {
	dir := t.TempDir()
	swornDir := filepath.Join(dir, DefaultDir)
	// Create .sworn/.gitignore as a *directory* — the write cannot succeed.
	if err := os.MkdirAll(filepath.Join(swornDir, ".gitignore"), 0o755); err != nil {
		t.Fatalf("pre-create .gitignore as directory: %v", err)
	}

	conn, err := Open(filepath.Join(swornDir, DefaultName))
	if err != nil {
		t.Fatalf("Open must succeed despite failed .gitignore write: %v", err)
	}
	defer conn.Close()

	// Prove the DB is genuinely usable, not just non-nil.
	var version int
	if err := conn.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("DB unusable after best-effort ignore failure: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", SchemaVersion, version)
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
