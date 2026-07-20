//go:build !linux

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Non-Linux platforms retain and revalidate file identities so ordinary Store
// opens remain truthful, but controller ownership fails closed because Sworn
// has no proved crash-released locking implementation for those platforms.
type controlStoreIdentity struct {
	mu sync.RWMutex

	path       string
	parentPath string
	database   *os.File
	parent     *os.File
	closed     bool
}

type ControllerOwnership struct{}

func retainControlStoreIdentity(path string, readOnly bool) (*controlStoreIdentity, error) {
	parentPath := filepath.Dir(path)
	parent, err := os.Open(parentPath)
	if err != nil {
		return nil, fmt.Errorf("retain control store parent %q: %w", parentPath, err)
	}
	flags := os.O_RDONLY
	if !readOnly {
		flags = os.O_RDWR
	}
	database, err := os.OpenFile(path, flags, 0)
	if err != nil {
		_ = parent.Close()
		return nil, fmt.Errorf("retain control store %q: %w", path, err)
	}
	identity := &controlStoreIdentity{
		path: path, parentPath: parentPath, database: database, parent: parent,
	}
	if err := identity.validateExactPath(); err != nil {
		_ = identity.close(nil)
		return nil, err
	}
	return identity, nil
}

func (identity *controlStoreIdentity) validateExactPath() error {
	if identity == nil {
		return errors.New("control store identity is absent")
	}
	identity.mu.RLock()
	defer identity.mu.RUnlock()
	if identity.closed || identity.database == nil || identity.parent == nil {
		return errors.New("control store identity is closed")
	}
	parentOpened, err := identity.parent.Stat()
	if err != nil {
		return fmt.Errorf("inspect retained control store parent %q: %w", identity.parentPath, err)
	}
	parentCurrent, err := os.Lstat(identity.parentPath)
	if err != nil {
		return fmt.Errorf("inspect current control store parent %q: %w", identity.parentPath, err)
	}
	if parentCurrent.Mode()&os.ModeSymlink != 0 || !parentCurrent.IsDir() ||
		!parentOpened.IsDir() || !os.SameFile(parentOpened, parentCurrent) {
		return fmt.Errorf("control store parent %q was replaced", identity.parentPath)
	}
	databaseOpened, err := identity.database.Stat()
	if err != nil {
		return fmt.Errorf("inspect retained control store %q: %w", identity.path, err)
	}
	databaseCurrent, err := os.Lstat(identity.path)
	if err != nil {
		return fmt.Errorf("inspect current control store %q: %w", identity.path, err)
	}
	if databaseCurrent.Mode()&os.ModeSymlink != 0 || !databaseCurrent.Mode().IsRegular() ||
		!databaseOpened.Mode().IsRegular() || !os.SameFile(databaseOpened, databaseCurrent) {
		return fmt.Errorf("control store %q was replaced", identity.path)
	}
	return nil
}

func (identity *controlStoreIdentity) close(database *sql.DB) error {
	if identity == nil {
		if database != nil {
			return database.Close()
		}
		return nil
	}
	identity.mu.Lock()
	defer identity.mu.Unlock()
	if identity.closed {
		if database != nil {
			return database.Close()
		}
		return nil
	}
	identity.closed = true
	var databaseErr error
	if database != nil {
		databaseErr = database.Close()
	}
	retainedDatabaseErr := identity.database.Close()
	retainedParentErr := identity.parent.Close()
	identity.database = nil
	identity.parent = nil
	return errors.Join(databaseErr, retainedDatabaseErr, retainedParentErr)
}

func (s *Store) AcquireControllerOwnership(string) (*ControllerOwnership, error) {
	return nil, ErrControllerOwnershipUnsupported
}

func (*ControllerOwnership) ValidateRecovery(*Store, string) error {
	return ErrControllerOwnershipUnsupported
}

func (*ControllerOwnership) Activate(context.Context, *Store, string) error {
	return ErrControllerOwnershipUnsupported
}

func (*ControllerOwnership) ValidateActive(*Store, string) error {
	return ErrControllerOwnershipUnsupported
}

func (*ControllerOwnership) Close() error { return nil }
