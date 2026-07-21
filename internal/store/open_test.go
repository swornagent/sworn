package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/repo"
)

func TestOpenCreatesIdentifiedSchemaAndReadOnlyOpenDoesNotMigrate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	control := openTestStore(t, path)
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if control.ControlPath() != absolutePath {
		t.Fatalf("control path = %q, want %q", control.ControlPath(), absolutePath)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("store permissions = %04o, want 0600", info.Mode().Perm())
	}

	application, version, err := identity(ctx, control.db)
	if err != nil {
		t.Fatal(err)
	}
	if application != applicationID || version != len(migrationNames) {
		t.Fatalf("identity = (%d, %d), want (%d, %d)", application, version, applicationID, len(migrationNames))
	}
	var journalMode string
	var synchronous, foreignKeys int
	if err := control.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if err := control.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if err := control.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if journalMode != "delete" || synchronous != 2 || foreignKeys != 1 {
		t.Fatalf("pragmas = journal:%s synchronous:%d foreign_keys:%d", journalMode, synchronous, foreignKeys)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}

	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })
	if _, err := readOnly.PutArtifact(ctx, "text/plain", []byte("no")); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("read-only write error = %v", err)
	}
}

func TestOpenRejectsPublicOrSymlinkedStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "control.db")
	control := openTestStore(t, path)
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if control, err := OpenReadOnly(ctx, path); err == nil {
		_ = control.Close()
		t.Fatal("OpenReadOnly accepted publicly readable control state")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "linked.db")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if control, err := OpenReadOnly(ctx, link); err == nil {
		_ = control.Close()
		t.Fatal("OpenReadOnly accepted symlinked control state")
	}
}

func TestOpenConfiguredRequiresExactLocalCheckRuntime(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "control.db")
	if control, err := OpenConfigured(context.Background(), path, ControlConfiguration{}); err == nil {
		_ = control.Close()
		t.Fatal("OpenConfigured accepted an absent local-check runtime digest")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected configured open created a control store: %v", err)
	}
}

func TestOpenConfiguredRequiresRepositoryWithNativeBuilder(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "control.db")
	if control, err := OpenConfigured(context.Background(), path, ControlConfiguration{
		BuilderDispatchDigest: "sha256:" + strings.Repeat("b", 64),
	}); err == nil {
		_ = control.Close()
		t.Fatal("OpenConfigured accepted a native builder without its repository")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected native builder open created a control store: %v", err)
	}
}

func TestOpenConfiguredBindsRepositoryForStoreLifetime(t *testing.T) {
	t.Parallel()
	repository := &repo.Repository{}
	configuration := ControlConfiguration{
		LocalCheckRuntimeManifestDigest: "sha256:" + strings.Repeat("a", 64),
		Repository:                      repository,
	}
	control, err := OpenConfigured(
		context.Background(), filepath.Join(t.TempDir(), "control.db"), configuration,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	configuration.Repository = nil
	if control.repository != repository {
		t.Fatal("configured store did not retain its immutable repository binding")
	}
}

func TestOpenRejectsForeignApplicationAndNewerSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	foreignPath := filepath.Join(t.TempDir(), "foreign.db")
	foreign := rawDatabase(t, foreignPath)
	if _, err := foreign.Exec("PRAGMA application_id = 42"); err != nil {
		t.Fatal(err)
	}
	if err := foreign.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(foreignPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if control, err := Open(ctx, foreignPath); err == nil {
		_ = control.Close()
		t.Fatal("Open accepted foreign application_id")
	}

	newerPath := filepath.Join(t.TempDir(), "newer.db")
	control := openTestStore(t, newerPath)
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	newer := rawDatabase(t, newerPath)
	if _, err := newer.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := newer.Close(); err != nil {
		t.Fatal(err)
	}
	if control, err := Open(ctx, newerPath); err == nil {
		_ = control.Close()
		t.Fatal("Open accepted newer schema")
	}
}

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make test Store parent private: %v", err)
	}
	control, err := OpenConfigured(context.Background(), path, ControlConfiguration{
		LocalCheckRuntimeManifestDigest: "sha256:" + strings.Repeat("e", 64),
	})
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	return control
}

func rawDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open(driverName, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Ping(); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	return database
}
