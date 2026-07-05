// Package db provides a SQLite-backed connection pool and schema management
// for the sworn run orchestration state. It is the single owner of the
// .sworn/sworn.db database.
//
// Stdlib + modernc.org/sqlite only. The sqlite dependency is documented in
// ADR-0003 as an exception to the project's stdlib-only policy.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DefaultDir is the default database directory relative to workspace root.
const DefaultDir = ".sworn"

// DefaultName is the default database filename.
const DefaultName = "sworn.db"

// Schema tracks SQLite schema version for migration detection. Current is 1.
const SchemaVersion = 1

// schema holds the DDL statements for the current schema version.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	)`,
	`CREATE TABLE IF NOT EXISTS tracks (
		id          TEXT NOT NULL,
		release     TEXT NOT NULL,
		pid         INTEGER NOT NULL DEFAULT 0,
		state       TEXT NOT NULL DEFAULT 'planned',
		current_slice TEXT NOT NULL DEFAULT '',
		started_at  TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (id, release)
	)`,
	`CREATE TABLE IF NOT EXISTS events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		track_id    TEXT NOT NULL,
		release     TEXT NOT NULL,
		event       TEXT NOT NULL,
		detail      TEXT NOT NULL DEFAULT '',
		ts          TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS decisions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slice_id    TEXT NOT NULL,
		release     TEXT NOT NULL,
		role        TEXT NOT NULL,
		action      TEXT NOT NULL,
		reason      TEXT NOT NULL DEFAULT '',
		recorded_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS circuit_failures (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slice_id    TEXT NOT NULL,
		release     TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		recorded_at TEXT NOT NULL
	)`,
}

// Open opens (or creates) the SQLite database at the given path and applies
// schema migrations. Returns the database handle.
func Open(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("db: create directory %s: %w", dir, err)
	}

	// Self-ignore sworn's own runtime dir so its churning binary DBs never
	// appear in the host repo's git status (and can't be accidentally
	// committed). Best-effort and idempotent — a failure here must never fail
	// the run, and an operator-customised .gitignore is left untouched. Gated
	// on the dir being sworn's .sworn/ so this generic opener never stamps a
	// stray ignore into an unrelated directory.
	if filepath.Base(dir) == DefaultDir {
		writeSelfIgnore(dir)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", dbPath, err)
	}

	// Limit to a single connection so SQLite's write serialisation is
	// managed at the Go pool level rather than via SQLITE_BUSY retries.
	// WAL mode still allows concurrent reads through the shared cache.
	conn.SetMaxOpenConns(1)

	// Enable WAL mode for better concurrent read/write behaviour.
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("db: enable WAL: %w", err)
	} // Enable foreign keys.
	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("db: enable foreign keys: %w", err)
	}

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("db: migrate: %w", err)
	}

	return conn, nil
}

// EnsureSelfIgnore best-effort stamps a self-ignoring .gitignore ("*") into dir
// when dir is sworn's runtime dir (basename == DefaultDir). It is the exported,
// gated entry point engine packages call before creating additional files under
// .sworn/ (e.g. logs/): the "*" ignore covers every child, so a caller that
// creates .sworn/logs/<release> can guarantee the ignore exists even on the
// (never-in-production) path where a log write races ahead of db.Open. Passing a
// non-.sworn dir is a no-op — the same gate db.Open applies — so this can never
// stamp a stray ignore into an unrelated directory. Idempotent and never errors.
func EnsureSelfIgnore(dir string) {
	if filepath.Base(dir) == DefaultDir {
		_ = writeSelfIgnore(dir)
	}
}

// writeSelfIgnore writes a .gitignore containing "*" into dir so git treats the
// whole directory — its DBs and the .gitignore itself — as ignored. It is
// best-effort and idempotent: O_EXCL makes the create fail (without touching
// the file) if a .gitignore already exists, preserving any operator
// customisation; any other write failure (e.g. an unwritable target) is
// likewise swallowed. The returned error is intentionally unused by Open — a
// repo-hygiene courtesy must never fail the DB-open path.
func writeSelfIgnore(dir string) error {
	f, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("*\n")
	return err
}

// DefaultPath returns the default database path under a workspace root.
func DefaultPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, DefaultDir, DefaultName)
}

// migrate applies schema migrations idempotently. On a fresh database it
// creates all tables and records the schema version. On subsequent opens,
// CREATE TABLE IF NOT EXISTS ensures no error.
func migrate(conn *sql.DB) error {
	for _, stmt := range schema {
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("db: schema exec: %w", err)
		}
	}

	// Record or verify schema version.
	_, err := conn.Exec(
		"INSERT OR IGNORE INTO schema_version (version) VALUES (?)",
		SchemaVersion,
	)
	if err != nil {
		return fmt.Errorf("db: record schema version: %w", err)
	}

	return nil
}
