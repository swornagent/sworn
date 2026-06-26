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
}

// Open opens (or creates) the SQLite database at the given path and applies
// schema migrations. Returns the database handle.
func Open(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("db: create directory %s: %w", filepath.Dir(dbPath), err)
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
