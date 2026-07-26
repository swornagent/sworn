package journal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	MaxPayloadBytes = 2 * 1024 * 1024
	MaxEventBytes   = 256 * 1024
	MaxLease        = 15 * time.Minute
	ApplicationID   = 1_398_230_866
	SchemaVersion   = 1

	schemaIdentityDigest = "sha256:bb78cc011e12981e7a7d82ac3198936b0a04c9ce8516f062d4d2017957f3cd3e"
)

//go:embed schema.sql
var schema string

var (
	identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)
	digestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// Error is intentionally byte-poor: journal failures never echo payloads,
// credentials, SQL, or driver output.
type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return "journal: " + e.Code
	}
	return "journal: " + e.Code + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

func fail(code string, err error) error { return &Error{Code: code, Err: err} }

func IsCode(err error, code string) bool {
	var journalErr *Error
	return errors.As(err, &journalErr) && journalErr.Code == code
}

type Store struct {
	path     string
	db       *sql.DB
	conn     *sql.Conn
	readOnly bool
	mu       sync.Mutex
}

// Open creates or admits one private, non-symlink SQLite journal.
func Open(ctx context.Context, path string) (*Store, error) {
	if ctx == nil {
		return nil, fail("INVALID_CONTEXT", nil)
	}
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return nil, fail("INVALID_PATH", errors.New("journal path must be absolute"))
	}
	clean := filepath.Clean(path)
	parent := filepath.Dir(clean)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, fail("INVALID_PATH", errors.New("journal parent is unavailable"))
	}
	if filepath.Clean(resolvedParent) != parent {
		return nil, fail("INVALID_PATH", errors.New("journal parent must not traverse symlinks"))
	}
	if info, err := os.Lstat(clean); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fail("INVALID_PATH", errors.New("journal is not a regular file"))
		}
		if info.Mode().Perm() != 0o600 {
			return nil, fail("INSECURE_PERMISSIONS", errors.New("journal mode must be 0600"))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fail("INVALID_PATH", errors.New("journal cannot be inspected"))
	} else {
		file, createErr := os.OpenFile(clean, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if createErr != nil {
			return nil, fail("OPEN_FAILED", errors.New("journal cannot be created"))
		}
		if closeErr := file.Close(); closeErr != nil {
			return nil, fail("OPEN_FAILED", errors.New("journal cannot be closed"))
		}
	}

	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "trusted_schema(0)")
	query.Add("_pragma", "synchronous(FULL)")
	dsn := (&url.URL{Scheme: "file", Path: clean, RawQuery: query.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fail("OPEN_FAILED", errors.New("SQLite open failed"))
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fail("OPEN_FAILED", errors.New("SQLite connection unavailable"))
	}
	store := &Store{path: clean, db: db, conn: conn}
	if err := store.initialize(ctx); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// OpenReadOnly admits an existing journal without creating files, migrating
// schema, acquiring a write transaction, or changing application data.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	if ctx == nil {
		return nil, fail("INVALID_CONTEXT", nil)
	}
	clean, err := admitExistingPath(path)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "trusted_schema(0)")
	query.Add("_pragma", "foreign_keys(1)")
	dsn := (&url.URL{Scheme: "file", Path: clean, RawQuery: query.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fail("OPEN_FAILED", errors.New("SQLite open failed"))
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fail("OPEN_FAILED", errors.New("SQLite connection unavailable"))
	}
	store := &Store{path: clean, db: db, conn: conn, readOnly: true}
	var count int
	if err := conn.QueryRowContext(
		ctx,
		`SELECT count(*) FROM sqlite_schema
		 WHERE type = 'table' AND name IN (
			'runs', 'commands', 'effects', 'claims', 'attempts', 'receipts', 'events'
		 )`,
	).Scan(&count); err != nil || count != 7 {
		_ = conn.Close()
		_ = db.Close()
		return nil, fail("SCHEMA_FAILED", errors.New("journal schema is unavailable"))
	}
	var applicationID, userVersion int
	if err := conn.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil ||
		applicationID != ApplicationID {
		_ = conn.Close()
		_ = db.Close()
		return nil, fail("IDENTITY_MISMATCH", errors.New("journal application identity is unavailable"))
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); err != nil ||
		userVersion != SchemaVersion {
		_ = conn.Close()
		_ = db.Close()
		return nil, fail("IDENTITY_MISMATCH", errors.New("journal schema identity is unavailable"))
	}
	if err := verifySchemaIdentity(ctx, conn); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func admitExistingPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return "", fail("INVALID_PATH", errors.New("journal path must be absolute"))
	}
	clean := filepath.Clean(path)
	parent := filepath.Dir(clean)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || filepath.Clean(resolvedParent) != parent {
		return "", fail("INVALID_PATH", errors.New("journal parent must not traverse symlinks"))
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fail("INVALID_PATH", errors.New("journal is not a regular file"))
	}
	if info.Mode().Perm() != 0o600 {
		return "", fail("INSECURE_PERMISSIONS", errors.New("journal mode must be 0600"))
	}
	return clean, nil
}

func (s *Store) initialize(ctx context.Context) error {
	if s == nil || s.db == nil || s.conn == nil {
		return fail("CLOSED", nil)
	}
	var priorApplicationID, priorUserVersion, priorObjects int
	if err := s.conn.QueryRowContext(ctx, "PRAGMA application_id").Scan(&priorApplicationID); err != nil {
		return fail("IDENTITY_MISMATCH", errors.New("journal application identity is unavailable"))
	}
	if err := s.conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&priorUserVersion); err != nil {
		return fail("IDENTITY_MISMATCH", errors.New("journal schema identity is unavailable"))
	}
	if err := s.conn.QueryRowContext(
		ctx,
		"SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'",
	).Scan(&priorObjects); err != nil {
		return fail("IDENTITY_MISMATCH", errors.New("journal catalog identity is unavailable"))
	}
	empty := priorObjects == 0 && priorApplicationID == 0 && priorUserVersion == 0
	if !empty {
		if priorApplicationID != ApplicationID || priorUserVersion != SchemaVersion {
			return fail("IDENTITY_MISMATCH", errors.New("journal identity does not match Sworn"))
		}
	}
	if empty {
		if _, err := s.conn.ExecContext(ctx, schema); err != nil {
			return fail("SCHEMA_FAILED", errors.New("journal schema was rejected"))
		}
	}
	var journalMode string
	if err := s.conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil ||
		strings.ToLower(journalMode) != "delete" {
		return fail("PRAGMA_FAILED", errors.New("rollback journal mode is required"))
	}
	var busyTimeout, foreignKeys, trustedSchema, synchronous int
	if err := s.conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil ||
		busyTimeout != 5000 {
		return fail("PRAGMA_FAILED", errors.New("bounded busy timeout is required"))
	}
	if err := s.conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil ||
		foreignKeys != 1 {
		return fail("PRAGMA_FAILED", errors.New("foreign keys are required"))
	}
	if err := s.conn.QueryRowContext(ctx, "PRAGMA trusted_schema").Scan(&trustedSchema); err != nil ||
		trustedSchema != 0 {
		return fail("PRAGMA_FAILED", errors.New("trusted schema must be disabled"))
	}
	if err := s.conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil ||
		synchronous < 2 {
		return fail("PRAGMA_FAILED", errors.New("FULL synchronous mode is required"))
	}
	var applicationID, userVersion int
	if err := s.conn.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil ||
		applicationID != ApplicationID {
		return fail("IDENTITY_MISMATCH", errors.New("journal application identity is unavailable"))
	}
	if err := s.conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); err != nil ||
		userVersion != SchemaVersion {
		return fail("IDENTITY_MISMATCH", errors.New("journal schema identity is unavailable"))
	}
	return verifySchemaIdentity(ctx, s.conn)
}

func schemaFingerprint(ctx context.Context, conn *sql.Conn) (string, error) {
	rows, err := conn.QueryContext(
		ctx,
		`SELECT type, name, tbl_name, COALESCE(sql, '')
		 FROM sqlite_schema
		 WHERE name NOT LIKE 'sqlite_%'
		 ORDER BY type, name`,
	)
	if err != nil {
		return "", fail("IDENTITY_MISMATCH", errors.New("journal catalog is unavailable"))
	}
	defer rows.Close()
	hash := sha256.New()
	var length [8]byte
	for rows.Next() {
		var objectType, name, table, statement string
		if err := rows.Scan(&objectType, &name, &table, &statement); err != nil {
			return "", fail("IDENTITY_MISMATCH", errors.New("journal catalog is unavailable"))
		}
		for _, value := range []string{objectType, name, table, statement} {
			binary.BigEndian.PutUint64(length[:], uint64(len(value)))
			_, _ = hash.Write(length[:])
			_, _ = hash.Write([]byte(value))
		}
	}
	if err := rows.Err(); err != nil {
		return "", fail("IDENTITY_MISMATCH", errors.New("journal catalog is unavailable"))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func verifySchemaIdentity(ctx context.Context, conn *sql.Conn) error {
	observed, err := schemaFingerprint(ctx, conn)
	if err != nil {
		return err
	}
	if observed != schemaIdentityDigest {
		return fail("IDENTITY_MISMATCH", errors.New("journal schema fingerprint does not match Sworn"))
	}
	return nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	var connErr error
	if s.conn != nil {
		connErr = s.conn.Close()
		s.conn = nil
	}
	dbErr := s.db.Close()
	s.db = nil
	if connErr != nil {
		return connErr
	}
	return dbErr
}

// immediate uses a connection-scoped literal BEGIN IMMEDIATE. Callers must
// return before any external effect is performed.
func (s *Store) immediate(ctx context.Context, fn func(*sql.Conn) error) (err error) {
	if s == nil {
		return fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil || s.conn == nil {
		return fail("CLOSED", nil)
	}
	if s.readOnly {
		return fail("READ_ONLY", nil)
	}
	conn := s.conn
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fail("DATABASE_BUSY", errors.New("write claim unavailable"))
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if err = fn(conn); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fail("COMMIT_FAILED", errors.New("journal commit acknowledgement unavailable"))
	}
	committed = true
	return nil
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validateIdentity(value, label string) error {
	if !identityPattern.MatchString(value) {
		return fail("INVALID_"+strings.ToUpper(label), nil)
	}
	return nil
}

func validateDigest(value string) error {
	if !digestPattern.MatchString(value) {
		return fail("INVALID_DIGEST", nil)
	}
	return nil
}

func canonicalTime(value time.Time) (string, error) {
	if value.IsZero() {
		return "", fail("INVALID_TIME", nil)
	}
	return value.UTC().Format(time.RFC3339Nano), nil
}

func randomToken() (string, error) {
	var body [32]byte
	if _, err := rand.Read(body[:]); err != nil {
		return "", fail("TOKEN_FAILED", errors.New("claim token unavailable"))
	}
	return hex.EncodeToString(body[:]), nil
}

func requireRows(result sql.Result, code string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fail("DATABASE_FAILED", errors.New("write result unavailable"))
	}
	if count != 1 {
		return fail(code, nil)
	}
	return nil
}

func dbError(err error) error {
	if err == nil {
		return nil
	}
	return fail("DATABASE_FAILED", fmt.Errorf("%T", err))
}
