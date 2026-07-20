// Package store owns Sworn's single transactional SQLite control truth.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/repo"

	_ "modernc.org/sqlite"
)

const (
	applicationID = 0x53574f52 // "SWOR"
	driverName    = "sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var migrationNames = []string{
	"migrations/001_initial.sql",
	"migrations/002_submission_identity.sql",
	"migrations/003_plan_authority.sql",
	"migrations/004_typed_effect_results.sql",
	"migrations/005_atomic_admission.sql",
	"migrations/006_bound_result_recovery.sql",
	"migrations/007_attempt_bound_retry.sql",
}

type Store struct {
	db                              *sql.DB
	path                            string
	controlIdentity                 *controlStoreIdentity
	readOnly                        bool
	now                             func() time.Time
	leaseIssuer                     *leaseIssuer
	localCheckRuntimeManifestDigest string
	builderDispatchDigest           string
	repository                      *repo.Repository
}

// ControlConfiguration contains immutable process configuration used by
// mutating command gates. Values are fixed for the lifetime of an opened
// Store; command payloads cannot replace them.
type ControlConfiguration struct {
	LocalCheckRuntimeManifestDigest string
	BuilderDispatchDigest           string
	Repository                      *repo.Repository
}

func Open(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, ControlConfiguration{})
}

// OpenConfigured opens the mutating control store with the exact local-check
// runtime selected by the process composition root. The ordinary Open remains
// useful for control operations that do not dispatch local checks, which fail
// closed while this configuration is absent.
func OpenConfigured(ctx context.Context, path string, configuration ControlConfiguration) (*Store, error) {
	if configuration.LocalCheckRuntimeManifestDigest != "" &&
		!engine.ValidDigest(configuration.LocalCheckRuntimeManifestDigest) {
		return nil, errors.New("configured control store has an invalid local-check runtime manifest digest")
	}
	if configuration.BuilderDispatchDigest != "" && !engine.ValidDigest(configuration.BuilderDispatchDigest) {
		return nil, errors.New("configured control store has an invalid builder dispatch digest")
	}
	if configuration.BuilderDispatchDigest != "" {
		if configuration.Repository == nil {
			return nil, errors.New("configured native builder requires an immutable repository")
		}
		if err := configuration.Repository.Binding().Validate(); err != nil {
			return nil, fmt.Errorf("configured native builder repository: %w", err)
		}
	}
	if configuration.LocalCheckRuntimeManifestDigest == "" && configuration.BuilderDispatchDigest == "" {
		return nil, errors.New("configured control store requires an execution digest")
	}
	return open(ctx, path, configuration)
}

func open(ctx context.Context, path string, configuration ControlConfiguration) (*Store, error) {
	database, absolutePath, controlIdentity, err := openDatabase(ctx, path, false)
	if err != nil {
		return nil, err
	}
	store := &Store{
		db: database, path: absolutePath, controlIdentity: controlIdentity,
		now: time.Now, leaseIssuer: &leaseIssuer{},
		localCheckRuntimeManifestDigest: configuration.LocalCheckRuntimeManifestDigest,
		builderDispatchDigest:           configuration.BuilderDispatchDigest,
		repository:                      configuration.Repository,
	}
	if err := store.migrate(ctx); err != nil {
		_ = controlIdentity.close(database)
		return nil, err
	}
	if err := controlIdentity.validateExactPath(); err != nil {
		_ = controlIdentity.close(database)
		return nil, fmt.Errorf("validate migrated control store identity: %w", err)
	}
	return store, nil
}

// OpenReadOnly never creates or migrates a database. It accepts only the exact
// schema version understood by this binary.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	database, absolutePath, controlIdentity, err := openDatabase(ctx, path, true)
	if err != nil {
		return nil, err
	}
	store := &Store{
		db: database, path: absolutePath, controlIdentity: controlIdentity, readOnly: true,
		now: time.Now, leaseIssuer: &leaseIssuer{},
	}
	if err := store.verifyIdentity(ctx, true); err != nil {
		_ = controlIdentity.close(database)
		return nil, err
	}
	if err := controlIdentity.validateExactPath(); err != nil {
		_ = controlIdentity.close(database)
		return nil, fmt.Errorf("validate read-only control store identity: %w", err)
	}
	return store, nil
}

func openDatabase(
	ctx context.Context,
	path string,
	readOnly bool,
) (*sql.DB, string, *controlStoreIdentity, error) {
	if path == "" {
		return nil, "", nil, errors.New("store path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolve store path: %w", err)
	}
	if err := prepareDatabasePath(absolute, readOnly); err != nil {
		return nil, "", nil, err
	}
	controlIdentity, err := retainControlStoreIdentity(absolute, readOnly)
	if err != nil {
		return nil, "", nil, err
	}
	parameters := url.Values{}
	parameters.Set("mode", map[bool]string{true: "ro", false: "rwc"}[readOnly])
	parameters.Set("_dqs", "false")
	parameters.Set("_error_rc", "true")
	parameters.Add("_pragma", "foreign_keys(1)")
	parameters.Add("_pragma", "busy_timeout(5000)")
	parameters.Add("_pragma", "trusted_schema(OFF)")
	if readOnly {
		parameters.Add("_pragma", "query_only(ON)")
	} else {
		parameters.Add("_pragma", "journal_mode(DELETE)")
		parameters.Add("_pragma", "synchronous(FULL)")
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute, RawQuery: parameters.Encode()}).String()
	database, err := sql.Open(driverName, dsn)
	if err != nil {
		_ = controlIdentity.close(nil)
		return nil, "", nil, fmt.Errorf("open SQLite driver: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(0)
	if err := database.PingContext(ctx); err != nil {
		_ = controlIdentity.close(database)
		return nil, "", nil, fmt.Errorf("connect to control store: %w", err)
	}
	if err := controlIdentity.validateExactPath(); err != nil {
		_ = controlIdentity.close(database)
		return nil, "", nil, fmt.Errorf("validate opened control store identity: %w", err)
	}
	return database, absolute, controlIdentity, nil
}

func prepareDatabasePath(path string, readOnly bool) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if readOnly {
			return fmt.Errorf("control store %q does not exist", path)
		}
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if createErr != nil {
			return fmt.Errorf("create private control store %q: %w", path, createErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("close new control store %q: %w", path, closeErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect control store %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("control store %q is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("control store %q permissions %04o expose private state; want 0600", path, info.Mode().Perm())
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if s.controlIdentity == nil {
		if s.db == nil {
			return nil
		}
		return s.db.Close()
	}
	return s.controlIdentity.close(s.db)
}

// ControlPath returns the absolute diagnostic path opened by this Store.
// Controller ownership never reopens this mutable name: Store retains and
// locks the exact parent and database identities observed before SQLite Ping.
func (s *Store) ControlPath() string {
	if s == nil {
		return ""
	}
	return s.path
}

// RequireBuilderConfiguration closes the composition boundary before a native
// worker can receive a lease. Structural Store use without a builder remains
// possible, but cannot construct the autonomous builder service.
func (s *Store) RequireBuilderConfiguration(dispatchDigest string, binding repo.Binding) error {
	if s.readOnly || !engine.ValidDigest(dispatchDigest) || s.builderDispatchDigest != dispatchDigest {
		return errors.New("control store does not match the native builder dispatch")
	}
	if s.repository == nil || s.repository.Binding() != binding {
		return errors.New("control store does not match the native builder repository")
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin store migration: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck

	application, version, err := identity(ctx, transaction)
	if err != nil {
		return err
	}
	if application != 0 && application != applicationID {
		return fmt.Errorf("control store application_id is %d, want %d", application, applicationID)
	}
	if version > len(migrationNames) {
		return fmt.Errorf("control store schema %d is newer than supported schema %d", version, len(migrationNames))
	}
	if application == 0 {
		if _, err := transaction.ExecContext(ctx, "PRAGMA application_id = "+strconv.Itoa(applicationID)); err != nil {
			return fmt.Errorf("set control store application_id: %w", err)
		}
	}
	for next := version + 1; next <= len(migrationNames); next++ {
		contents, err := migrationFiles.ReadFile(migrationNames[next-1])
		if err != nil {
			return fmt.Errorf("read migration %d: %w", next, err)
		}
		if _, err := transaction.ExecContext(ctx, string(contents)); err != nil {
			return fmt.Errorf("apply migration %d: %w", next, err)
		}
		if _, err := transaction.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(next)); err != nil {
			return fmt.Errorf("record migration %d: %w", next, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit store migration: %w", err)
	}
	return s.verifyIdentity(ctx, false)
}

func (s *Store) verifyIdentity(ctx context.Context, readOnly bool) error {
	application, version, err := identity(ctx, s.db)
	if err != nil {
		return err
	}
	if application != applicationID {
		return fmt.Errorf("control store application_id is %d, want %d", application, applicationID)
	}
	if version != len(migrationNames) {
		if readOnly && version < len(migrationNames) {
			return fmt.Errorf("control store schema %d requires migration to %d", version, len(migrationNames))
		}
		return fmt.Errorf("control store schema is %d, want %d", version, len(migrationNames))
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if foreignKeys != 1 {
		return errors.New("SQLite foreign keys are disabled")
	}
	return nil
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func identity(ctx context.Context, query rowQuerier) (application int, version int, err error) {
	if err := query.QueryRowContext(ctx, "PRAGMA application_id").Scan(&application); err != nil {
		return 0, 0, fmt.Errorf("read control store application_id: %w", err)
	}
	if err := query.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, 0, fmt.Errorf("read control store user_version: %w", err)
	}
	return application, version, nil
}
