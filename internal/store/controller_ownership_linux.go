//go:build linux

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/swornagent/sworn/internal/engine"
)

type controlFileKey struct {
	device uint64
	inode  uint64
}

// controlStoreIdentity retains the exact directory and database objects seen
// before SQLite first connects. Ownership locks these descriptors directly;
// it never reopens a mutable pathname and accidentally locks a replacement.
type controlStoreIdentity struct {
	mu sync.RWMutex

	path       string
	parentPath string
	database   *os.File
	parent     *os.File
	databaseID controlFileKey
	parentID   controlFileKey
	closed     bool
}

var controllerOwnershipRegistry = struct {
	sync.Mutex
	owners map[controlFileKey]*ControllerOwnership
}{owners: make(map[controlFileKey]*ControllerOwnership)}

type controllerOwnershipPhase uint8

const (
	controllerOwnershipRecovery controllerOwnershipPhase = iota + 1
	controllerOwnershipActive
)

// ControllerOwnership is an opaque process-lifetime capability for one exact
// retained Store identity. It must not be copied; callers may share its
// pointer. The kernel releases both locks on abrupt process termination.
type ControllerOwnership struct {
	state *controllerOwnershipState
}

type controllerOwnershipState struct {
	sync.RWMutex
	control  *Store
	identity *controlStoreIdentity
	ownerID  string
	phase    controllerOwnershipPhase
	live     bool
}

func retainControlStoreIdentity(path string, readOnly bool) (*controlStoreIdentity, error) {
	parentPath := filepath.Dir(path)
	parentFD, err := syscall.Open(
		parentPath,
		syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("retain control store parent %q: %w", parentPath, err)
	}
	parent := os.NewFile(uintptr(parentFD), parentPath)
	if parent == nil {
		_ = syscall.Close(parentFD)
		return nil, fmt.Errorf("retain control store parent %q: invalid file descriptor", parentPath)
	}

	flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	if !readOnly {
		flags = syscall.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	}
	databaseFD, err := syscall.Open(path, flags, 0)
	if err != nil {
		_ = parent.Close()
		return nil, fmt.Errorf("retain control store %q: %w", path, err)
	}
	database := os.NewFile(uintptr(databaseFD), path)
	if database == nil {
		_ = syscall.Close(databaseFD)
		_ = parent.Close()
		return nil, fmt.Errorf("retain control store %q: invalid file descriptor", path)
	}

	databaseInfo, err := database.Stat()
	if err != nil {
		_ = database.Close()
		_ = parent.Close()
		return nil, fmt.Errorf("inspect retained control store %q: %w", path, err)
	}
	parentInfo, err := parent.Stat()
	if err != nil {
		_ = database.Close()
		_ = parent.Close()
		return nil, fmt.Errorf("inspect retained control store parent %q: %w", parentPath, err)
	}
	databaseID, err := controlFileIdentity(databaseInfo)
	if err != nil {
		_ = database.Close()
		_ = parent.Close()
		return nil, fmt.Errorf("identify retained control store %q: %w", path, err)
	}
	parentID, err := controlFileIdentity(parentInfo)
	if err != nil {
		_ = database.Close()
		_ = parent.Close()
		return nil, fmt.Errorf("identify retained control store parent %q: %w", parentPath, err)
	}
	identity := &controlStoreIdentity{
		path: path, parentPath: parentPath, database: database, parent: parent,
		databaseID: databaseID, parentID: parentID,
	}
	if err := identity.validateExactPath(); err != nil {
		_ = identity.close(nil)
		return nil, err
	}
	return identity, nil
}

func controlFileIdentity(info os.FileInfo) (controlFileKey, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Dev == 0 || stat.Ino == 0 {
		return controlFileKey{}, errors.New("file lacks a stable device and inode identity")
	}
	return controlFileKey{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, nil
}

func (identity *controlStoreIdentity) validateExactPath() error {
	if identity == nil {
		return errors.New("control store identity is absent")
	}
	identity.mu.RLock()
	defer identity.mu.RUnlock()
	return identity.validateExactPathLocked()
}

func (identity *controlStoreIdentity) validateExactPathLocked() error {
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
	parentID, err := controlFileIdentity(parentOpened)
	if err != nil || parentID != identity.parentID {
		return fmt.Errorf("control store parent %q changed identity", identity.parentPath)
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
	databaseID, err := controlFileIdentity(databaseOpened)
	if err != nil || databaseID != identity.databaseID {
		return fmt.Errorf("control store %q changed identity", identity.path)
	}
	return nil
}

func (identity *controlStoreIdentity) validatePrivateOwnershipLocked() error {
	if err := identity.validateExactPathLocked(); err != nil {
		return err
	}
	databaseInfo, err := identity.database.Stat()
	if err != nil {
		return fmt.Errorf("inspect owned control store %q: %w", identity.path, err)
	}
	databaseStat, ok := databaseInfo.Sys().(*syscall.Stat_t)
	if !ok || databaseStat == nil {
		return fmt.Errorf("control store %q lacks Linux ownership facts", identity.path)
	}
	if databaseInfo.Mode().Perm() != 0o600 {
		return fmt.Errorf(
			"control store %q permissions %04o are unsafe for ownership; want 0600",
			identity.path, databaseInfo.Mode().Perm(),
		)
	}
	if databaseStat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("control store %q is owned by uid %d, want %d", identity.path, databaseStat.Uid, os.Geteuid())
	}
	if databaseStat.Nlink != 1 {
		return fmt.Errorf("control store %q has %d hard links; want exactly one", identity.path, databaseStat.Nlink)
	}

	parentInfo, err := identity.parent.Stat()
	if err != nil {
		return fmt.Errorf("inspect owned control store parent %q: %w", identity.parentPath, err)
	}
	parentStat, ok := parentInfo.Sys().(*syscall.Stat_t)
	if !ok || parentStat == nil {
		return fmt.Errorf("control store parent %q lacks Linux ownership facts", identity.parentPath)
	}
	if parentInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf(
			"control store parent %q permissions %04o are unsafe for ownership; group and world write bits must be absent",
			identity.parentPath, parentInfo.Mode().Perm(),
		)
	}
	if parentStat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf(
			"control store parent %q is owned by uid %d, want %d",
			identity.parentPath, parentStat.Uid, os.Geteuid(),
		)
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
	controllerOwnershipRegistry.Lock()
	identity.mu.Lock()
	if identity.closed {
		identity.mu.Unlock()
		controllerOwnershipRegistry.Unlock()
		if database != nil {
			return database.Close()
		}
		return nil
	}
	if registered := controllerOwnershipRegistry.owners[identity.databaseID]; registered != nil && registered.state != nil && registered.state.identity == identity {
		identity.mu.Unlock()
		controllerOwnershipRegistry.Unlock()
		return errors.New("close control store with active controller ownership")
	}
	identity.closed = true
	controllerOwnershipRegistry.Unlock()

	var databaseErr error
	if database != nil {
		databaseErr = database.Close()
	}
	retainedDatabaseErr := identity.database.Close()
	retainedParentErr := identity.parent.Close()
	identity.database = nil
	identity.parent = nil
	identity.mu.Unlock()
	return errors.Join(databaseErr, retainedDatabaseErr, retainedParentErr)
}

// AcquireControllerOwnership nonblockingly locks the exact parent namespace
// and database objects retained by this Store before SQLite connected.
func (s *Store) AcquireControllerOwnership(ownerID string) (*ControllerOwnership, error) {
	if s == nil || s.readOnly || s.controlIdentity == nil {
		return nil, errors.New("controller ownership requires a writable control Store")
	}
	if !engine.ValidID(ownerID) {
		return nil, fmt.Errorf("controller ownership requires a valid owner id %q", ownerID)
	}
	identity := s.controlIdentity
	controllerOwnershipRegistry.Lock()
	defer controllerOwnershipRegistry.Unlock()
	identity.mu.Lock()
	defer identity.mu.Unlock()
	if err := identity.validatePrivateOwnershipLocked(); err != nil {
		return nil, fmt.Errorf("acquire controller ownership: %w", err)
	}
	if controllerOwnershipRegistry.owners[identity.databaseID] != nil {
		return nil, fmt.Errorf("acquire controller ownership for %q: %w", identity.path, ErrControllerOwnershipUnavailable)
	}
	if err := flockNonblockingExclusive(identity.parent); err != nil {
		return nil, ownershipLockError("parent", identity.parentPath, err)
	}
	parentLocked := true
	defer func() {
		if parentLocked {
			_ = flockUnlock(identity.parent)
		}
	}()
	if err := flockNonblockingExclusive(identity.database); err != nil {
		return nil, ownershipLockError("store", identity.path, err)
	}
	databaseLocked := true
	defer func() {
		if databaseLocked {
			_ = flockUnlock(identity.database)
		}
	}()
	if err := identity.validatePrivateOwnershipLocked(); err != nil {
		return nil, fmt.Errorf("validate acquired controller ownership: %w", err)
	}
	ownership := &ControllerOwnership{state: &controllerOwnershipState{
		control: s, identity: identity, ownerID: ownerID,
		phase: controllerOwnershipRecovery, live: true,
	}}
	controllerOwnershipRegistry.owners[identity.databaseID] = ownership
	parentLocked = false
	databaseLocked = false
	return ownership, nil
}

func ownershipLockError(kind, path string, err error) error {
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return fmt.Errorf("lock control %s %q: %w", kind, path, ErrControllerOwnershipUnavailable)
	}
	return fmt.Errorf("lock control %s %q: %w", kind, path, err)
}

func flockNonblockingExclusive(file *os.File) error {
	return flock(file, syscall.LOCK_EX|syscall.LOCK_NB)
}

func flockUnlock(file *os.File) error { return flock(file, syscall.LOCK_UN) }

func flock(file *os.File, operation int) error {
	if file == nil {
		return errors.New("lock file is absent")
	}
	for {
		err := syscall.Flock(int(file.Fd()), operation)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

// ValidateRecovery proves this exact live handle remains in its startup
// recovery phase for the supplied Store and owner.
func (ownership *ControllerOwnership) ValidateRecovery(control *Store, ownerID string) error {
	return ownership.validate(control, ownerID, controllerOwnershipRecovery)
}

// ValidateActive proves this exact live handle completed recovery and remains
// the active owner for the supplied Store and owner.
func (ownership *ControllerOwnership) ValidateActive(control *Store, ownerID string) error {
	return ownership.validate(control, ownerID, controllerOwnershipActive)
}

func (ownership *ControllerOwnership) validate(
	control *Store,
	ownerID string,
	want controllerOwnershipPhase,
) error {
	if ownership == nil || ownership.state == nil || control == nil || !engine.ValidID(ownerID) {
		return ErrInvalidControllerOwnership
	}
	state := ownership.state
	state.RLock()
	defer state.RUnlock()
	return ownership.validateStateLocked(control, ownerID, want)
}

func (ownership *ControllerOwnership) validateStateLocked(
	control *Store,
	ownerID string,
	want controllerOwnershipPhase,
) error {
	state := ownership.state
	if !state.live || state.control != control || state.identity == nil ||
		control.controlIdentity != state.identity || state.ownerID != ownerID || state.phase != want {
		return ErrInvalidControllerOwnership
	}
	identity := state.identity
	controllerOwnershipRegistry.Lock()
	defer controllerOwnershipRegistry.Unlock()
	identity.mu.RLock()
	defer identity.mu.RUnlock()
	if controllerOwnershipRegistry.owners[identity.databaseID] != ownership {
		return ErrInvalidControllerOwnership
	}
	if err := identity.validatePrivateOwnershipLocked(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidControllerOwnership, err)
	}
	return nil
}

// Activate advances an exclusively owned Store from recovery to active only
// after durable truth proves that no running or unknown external effect remains.
func (ownership *ControllerOwnership) Activate(
	ctx context.Context,
	control *Store,
	ownerID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ownership == nil || ownership.state == nil || control == nil || !engine.ValidID(ownerID) {
		return ErrInvalidControllerOwnership
	}
	state := ownership.state
	state.Lock()
	defer state.Unlock()
	if state.phase == controllerOwnershipActive {
		return ownership.validateStateLocked(control, ownerID, controllerOwnershipActive)
	}
	if err := ownership.validateStateLocked(control, ownerID, controllerOwnershipRecovery); err != nil {
		return err
	}
	transaction, err := control.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  true,
	})
	if err != nil {
		return fmt.Errorf("begin controller recovery proof: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	var unresolved int
	if err := transaction.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM effects WHERE state IN ('running', 'unknown')`,
	).Scan(&unresolved); err != nil {
		return fmt.Errorf("prove controller recovery completion: %w", err)
	}
	if unresolved != 0 {
		return fmt.Errorf("controller recovery has %d unresolved effects", unresolved)
	}
	if err := ownership.validateStateLocked(control, ownerID, controllerOwnershipRecovery); err != nil {
		return err
	}
	// Flip the in-memory capability while SQLite still holds the read snapshot.
	// In rollback-journal mode, no peer writer can commit a new running effect
	// between the durable zero proof and this phase transition.
	state.phase = controllerOwnershipActive
	if err := transaction.Commit(); err != nil {
		state.phase = controllerOwnershipRecovery
		return fmt.Errorf("finish controller recovery proof: %w", err)
	}
	return nil
}

// Close releases database and namespace ownership. It is idempotent for the
// exact handle; copied or stale handles cannot release a successor's locks.
func (ownership *ControllerOwnership) Close() error {
	if ownership == nil || ownership.state == nil {
		return nil
	}
	state := ownership.state
	state.Lock()
	defer state.Unlock()
	if !state.live {
		return nil
	}
	identity := state.identity
	if identity == nil {
		return ErrInvalidControllerOwnership
	}
	controllerOwnershipRegistry.Lock()
	defer controllerOwnershipRegistry.Unlock()
	identity.mu.Lock()
	defer identity.mu.Unlock()
	if controllerOwnershipRegistry.owners[identity.databaseID] != ownership {
		return ErrInvalidControllerOwnership
	}
	databaseErr := flockUnlock(identity.database)
	parentErr := flockUnlock(identity.parent)
	delete(controllerOwnershipRegistry.owners, identity.databaseID)
	state.live = false
	if err := errors.Join(databaseErr, parentErr); err != nil {
		return fmt.Errorf("release controller ownership for %q: %w", identity.path, err)
	}
	return nil
}
