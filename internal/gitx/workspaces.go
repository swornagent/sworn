package gitx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	workspaceRootSchema   = "sworn.workspace-root/v2"
	workspaceLeaseSchema  = "sworn.workspace-lease/v2"
	workspaceWriterSchema = "sworn.workspace-writer/v1"
	workspaceBaseName     = "sworn-workspaces-v2"
	workspaceWriterBase   = "sworn-workspace-writers-v1"
)

var (
	workspaceIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	workspaceTokenPattern    = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type WorkspaceView string

const (
	PlannerView          WorkspaceView = "planner"
	DesignView           WorkspaceView = "design"
	CaptainView          WorkspaceView = "captain"
	ImplementationView   WorkspaceView = "implementation"
	WorkVerifierView     WorkspaceView = "work_verifier"
	AssemblyVerifierView WorkspaceView = "assembly_verifier"
	ReleaseAssemblyView  WorkspaceView = "release_assembly"
)

type WorkspaceAccess string

const (
	WorkspaceReadOnly  WorkspaceAccess = "read_only"
	WorkspaceReadWrite WorkspaceAccess = "read_write"
)

type TrackKey struct {
	Release string
	Track   string
}

type WorkspaceLease struct {
	owner      *Workspaces
	path       string
	token      string
	key        TrackKey
	view       WorkspaceView
	access     WorkspaceAccess
	head       OID
	closed     bool
	readOnly   bool
	writerLock *os.File
}

// ReleaseAssemblyLease deliberately exposes no workspace path. It is an
// engine-only ownership token for deterministic assembly actions, never a
// model workspace.
type ReleaseAssemblyLease struct {
	workspace *WorkspaceLease
}

func (l *ReleaseAssemblyLease) Close() error {
	if l == nil || l.workspace == nil {
		return nil
	}
	workspace := l.workspace
	l.workspace = nil
	return workspace.Close()
}

func (l *WorkspaceLease) Path() string {
	if l == nil || l.closed {
		return ""
	}
	return l.path
}

func (l *WorkspaceLease) Head() OID {
	if l == nil || l.closed {
		return OID{}
	}
	return l.head
}

func (l *WorkspaceLease) Access() WorkspaceAccess {
	if l == nil || l.closed {
		return ""
	}
	return l.access
}

type SealedCandidate struct {
	Before       OID
	Candidate    OID
	Tree         OID
	ChangedPaths []string
}

// SealAuthority is the exact non-track Git authority that must remain current
// while an implementation candidate is published. The release ref is derived
// from the lease's typed TrackKey; callers cannot substitute another owner.
type SealAuthority struct {
	ReleaseHead OID
	TargetRef   string
	TargetHead  OID
}

type SealReconciliation string

const (
	SealAllOld    SealReconciliation = "all_old"
	SealAllNew    SealReconciliation = "all_new"
	SealAmbiguous SealReconciliation = "ambiguous"
)

type Workspaces struct {
	repository *Repository
	identity   string
	root       string
	treesRoot  string
	leasesRoot string
	lock       *os.File
	mu         sync.Mutex
	leases     map[string]*WorkspaceLease
	closed     bool
}

// NewWorkspaces creates an ephemeral engine-owned workspace root for callers
// that do not have a durable run identity. Runtime delivery must use
// NewRunWorkspaces so a replacement owner can recover worktrees after a hard
// process exit.
func NewWorkspaces(repository *Repository) (*Workspaces, error) {
	token, err := randomWorkspaceName()
	if err != nil {
		return nil, fail("WORKSPACE_CREATE_FAILED", "name workspace owner", err)
	}
	return newWorkspaces(repository, "ephemeral-"+token)
}

// NewRunWorkspaces creates the one stable workspace owner for a canonical
// repository and admitted run. Its root is derived from SHA-256 over the Git
// common directory and run identity. A replacement runtime owner therefore
// finds and removes only abandoned worktrees carrying this exact ownership
// marker; other repositories and runs have different roots.
func NewRunWorkspaces(repository *Repository, runID string) (*Workspaces, error) {
	if !workspaceIdentityPattern.MatchString(runID) {
		return nil, fail("INVALID_WORKSPACE_KEY", "validate workspace run", nil)
	}
	return newWorkspaces(repository, runID)
}

func newWorkspaces(repository *Repository, runID string) (*Workspaces, error) {
	if repository == nil {
		return nil, fail("INVALID_REPOSITORY", "create workspaces", nil)
	}
	base, err := workspaceBase()
	if err != nil {
		return nil, err
	}
	if pathsOverlap(base, repository.Root()) ||
		pathsOverlap(base, repository.commonDir) {
		return nil, fail(
			"INVALID_REPOSITORY", "separate repository from workspace roots", nil)
	}
	if err := ensurePrivateDirectory(base); err != nil {
		return nil, err
	}
	identity := workspaceIdentity(repository.commonDir, runID)
	root := filepath.Join(base, identity)
	lock, err := acquireWorkspaceLock(base, identity)
	if err != nil {
		return nil, err
	}
	workspaces := &Workspaces{
		repository: repository,
		identity:   identity,
		root:       root,
		treesRoot:  filepath.Join(root, "trees"),
		leasesRoot: filepath.Join(root, "leases"),
		lock:       lock,
		leases:     make(map[string]*WorkspaceLease),
	}
	if err := workspaces.prepareRoot(); err != nil {
		_ = workspaces.releaseLock()
		return nil, err
	}
	return workspaces, nil
}

func workspaceIdentity(commonDir, runID string) string {
	digest := sha256.Sum256([]byte(
		workspaceRootSchema + "\x00" + commonDir + "\x00" + runID,
	))
	return hex.EncodeToString(digest[:])
}

func workspaceWriterIdentity(commonDir string, key TrackKey) string {
	digest := sha256.Sum256([]byte(
		workspaceWriterSchema + "\x00" + commonDir + "\x00" +
			key.Release + "\x00" + key.Track,
	))
	return "writer-" + hex.EncodeToString(digest[:])
}

func workspaceBase() (string, error) {
	temp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil || !filepath.IsAbs(temp) || filepath.Clean(temp) == string(filepath.Separator) {
		return "", fail("WORKSPACE_CREATE_FAILED", "resolve workspace base", err)
	}
	base := filepath.Join(filepath.Clean(temp), fmt.Sprintf("%s-%d", workspaceBaseName, os.Geteuid()))
	return base, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fail("WORKSPACE_CREATE_FAILED", "create workspace directory", err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm()&0o077 != 0 {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "inspect workspace directory", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "inspect workspace directory owner", nil)
	}
	return nil
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func pathsOverlap(first, second string) bool {
	return first == second || pathWithin(first, second) || pathWithin(second, first)
}

func acquireWorkspaceLock(base, identity string) (*os.File, error) {
	return acquirePrivateLock(
		base,
		identity,
		rootMarker(identity),
		"workspace owner",
	)
}

func acquireWorkspaceWriterLock(
	commonDir string,
	key TrackKey,
) (*os.File, error) {
	// The canonical Git common directory is shared by the primary checkout,
	// linked worktrees, and runtimes with different temporary directories.
	// Keep lock files permanently: unlinking a locked path would let a second
	// process create a new inode and acquire a competing lock.
	base := filepath.Join(commonDir, workspaceWriterBase)
	if err := ensurePrivateDirectory(base); err != nil {
		return nil, err
	}
	identity := workspaceWriterIdentity(commonDir, key)
	return acquirePrivateLock(
		base,
		identity,
		writerMarker(identity),
		"workspace writer",
	)
}

func validateWorkspaceWriterLock(
	file *os.File,
	commonDir string,
	key TrackKey,
) error {
	if file == nil {
		return fail("INVALID_WORKSPACE_REQUEST", "admit writable workspace", nil)
	}
	identity := workspaceWriterIdentity(commonDir, key)
	path := filepath.Join(commonDir, workspaceWriterBase, identity+".lock")
	if file.Name() != path {
		return fail(
			"WORKSPACE_OWNERSHIP_MISMATCH",
			"validate workspace writer lock path",
			nil,
		)
	}
	openInfo, err := file.Stat()
	if err != nil {
		return fail(
			"WORKSPACE_OWNERSHIP_MISMATCH",
			"inspect open workspace writer lock",
			err,
		)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !os.SameFile(openInfo, pathInfo) {
		return fail(
			"WORKSPACE_OWNERSHIP_MISMATCH",
			"validate workspace writer lock file",
			err,
		)
	}
	return validateMarker(path, writerMarker(identity))
}

func acquirePrivateLock(
	base, identity string,
	expected []byte,
	kind string,
) (*os.File, error) {
	path := filepath.Join(base, identity+".lock")
	fd, err := syscall.Open(
		path,
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, fail("WORKSPACE_CREATE_FAILED", "open "+kind+" lock", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, fail("WORKSPACE_CREATE_FAILED", "open "+kind+" lock", nil)
	}
	failLock := func(code, operation string, err error) (*os.File, error) {
		_ = file.Close()
		return nil, fail(code, operation, err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return failLock(
			"WORKSPACE_OWNERSHIP_MISMATCH", "inspect "+kind+" lock", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return failLock(
			"WORKSPACE_OWNERSHIP_MISMATCH", "inspect "+kind+" lock owner", nil)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return failLock("WORKSPACE_OWNER_ACTIVE", "acquire "+kind+" lock", nil)
		}
		return failLock("WORKSPACE_CREATE_FAILED", "acquire "+kind+" lock", err)
	}
	switch {
	case info.Size() == 0:
		if _, err := file.WriteAt(expected, 0); err != nil {
			_ = syscall.Flock(fd, syscall.LOCK_UN)
			return failLock("WORKSPACE_CREATE_FAILED", "write "+kind+" lock", err)
		}
		if err := file.Sync(); err != nil {
			_ = syscall.Flock(fd, syscall.LOCK_UN)
			return failLock("WORKSPACE_CREATE_FAILED", "sync "+kind+" lock", err)
		}
	case info.Size() != int64(len(expected)):
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		return failLock(
			"WORKSPACE_OWNERSHIP_MISMATCH", "validate "+kind+" lock", nil)
	default:
		observed := make([]byte, len(expected))
		if _, err := file.ReadAt(observed, 0); err != nil ||
			!bytes.Equal(observed, expected) {
			_ = syscall.Flock(fd, syscall.LOCK_UN)
			return failLock(
				"WORKSPACE_OWNERSHIP_MISMATCH", "validate "+kind+" lock", err)
		}
	}
	return file, nil
}

func (w *Workspaces) releaseLock() error {
	if w == nil || w.lock == nil {
		return nil
	}
	file := w.lock
	w.lock = nil
	return releasePrivateLock(file, "workspace owner")
}

func releasePrivateLock(file *os.File, kind string) error {
	if file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	if err := errors.Join(unlockErr, closeErr); err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "release "+kind+" lock", err)
	}
	return nil
}

func (w *Workspaces) removeLockFile() error {
	if w == nil || w.lock == nil {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace owner lock", nil)
	}
	path := filepath.Join(filepath.Dir(w.root), w.identity+".lock")
	if w.lock.Name() != path {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace owner lock path", nil)
	}
	openInfo, err := w.lock.Stat()
	if err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "inspect open workspace owner lock", err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !os.SameFile(openInfo, pathInfo) {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace owner lock file", err)
	}
	expected := rootMarker(w.identity)
	if openInfo.Size() != int64(len(expected)) {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace owner lock", nil)
	}
	observed := make([]byte, len(expected))
	if _, err := w.lock.ReadAt(observed, 0); err != nil ||
		!bytes.Equal(observed, expected) {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace owner lock", err)
	}
	if err := os.Remove(path); err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "remove workspace owner lock", err)
	}
	return nil
}

func rootMarker(identity string) []byte {
	return []byte(workspaceRootSchema + "\n" + identity + "\n")
}

func writerMarker(identity string) []byte {
	return []byte(workspaceWriterSchema + "\n" + identity + "\n")
}

func leaseMarker(identity, token string) []byte {
	return []byte(workspaceLeaseSchema + "\n" + identity + "\n" + token + "\n")
}

func validateMarker(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() != int64(len(expected)) {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "inspect workspace marker", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "inspect workspace marker owner", nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "open workspace marker", err)
	}
	defer file.Close()
	openInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openInfo) ||
		openInfo.Size() != int64(len(expected)) {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace marker file", err)
	}
	observed := make([]byte, len(expected))
	if _, err := file.ReadAt(observed, 0); err != nil ||
		!bytes.Equal(observed, expected) {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace marker", err)
	}
	return nil
}

func writeExclusiveMarker(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	closeErr := file.Close()
	joined := errors.Join(writeErr, closeErr)
	if joined != nil {
		_ = os.Remove(path)
	}
	return joined
}

func (w *Workspaces) prepareRoot() error {
	if err := ensurePrivateDirectory(w.root); err != nil {
		return err
	}
	ownerPath := filepath.Join(w.root, "owner")
	if _, err := os.Lstat(ownerPath); errors.Is(err, fs.ErrNotExist) {
		entries, readErr := os.ReadDir(w.root)
		if readErr != nil {
			return fail("WORKSPACE_CREATE_FAILED", "inspect new workspace root", readErr)
		}
		if len(entries) != 0 {
			return fail("WORKSPACE_OWNERSHIP_MISMATCH", "admit workspace root", nil)
		}
		if err := writeExclusiveMarker(ownerPath, rootMarker(w.identity)); err != nil {
			return fail("WORKSPACE_CREATE_FAILED", "write workspace owner", err)
		}
	}
	if err := validateMarker(ownerPath, rootMarker(w.identity)); err != nil {
		return err
	}
	if err := ensurePrivateDirectory(w.treesRoot); err != nil {
		return err
	}
	if err := ensurePrivateDirectory(w.leasesRoot); err != nil {
		return err
	}
	entries, err := os.ReadDir(w.root)
	if err != nil {
		return fail("WORKSPACE_CREATE_FAILED", "inspect workspace root layout", err)
	}
	expected := map[string]bool{"owner": false, "trees": false, "leases": false}
	for _, entry := range entries {
		if _, ok := expected[entry.Name()]; !ok {
			return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace root layout", nil)
		}
		expected[entry.Name()] = true
	}
	for _, present := range expected {
		if !present {
			return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace root layout", nil)
		}
	}
	return w.recoverAbandoned()
}

func registeredWorktreePaths(repository *Repository) ([]string, error) {
	raw, err := repository.run(nil, nil, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, fail("WORKSPACE_CLEANUP_FAILED", "list registered worktrees", err)
	}
	var paths []string
	seen := make(map[string]struct{})
	for _, field := range bytes.Split(raw, []byte{0}) {
		if !bytes.HasPrefix(field, []byte("worktree ")) {
			continue
		}
		path := filepath.Clean(string(bytes.TrimPrefix(field, []byte("worktree "))))
		if !filepath.IsAbs(path) {
			return nil, fail(
				"WORKSPACE_OWNERSHIP_MISMATCH", "validate registered worktree", nil)
		}
		if _, duplicate := seen[path]; duplicate {
			return nil, fail(
				"WORKSPACE_OWNERSHIP_MISMATCH", "validate registered worktree", nil)
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func (w *Workspaces) recoverAbandoned() error {
	leaseEntries, err := os.ReadDir(w.leasesRoot)
	if err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "read workspace leases", err)
	}
	markers := make(map[string]string, len(leaseEntries))
	for _, entry := range leaseEntries {
		token := entry.Name()
		path := filepath.Join(w.leasesRoot, token)
		if entry.IsDir() || !workspaceTokenPattern.MatchString(token) ||
			validateMarker(path, leaseMarker(w.identity, token)) != nil {
			return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace lease", nil)
		}
		markers[token] = path
	}
	treeEntries, err := os.ReadDir(w.treesRoot)
	if err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "read workspace trees", err)
	}
	for _, entry := range treeEntries {
		if !entry.IsDir() || !workspaceTokenPattern.MatchString(entry.Name()) {
			return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace tree", nil)
		}
		if _, ok := markers[entry.Name()]; !ok {
			return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace tree owner", nil)
		}
	}
	registered, err := registeredWorktreePaths(w.repository)
	if err != nil {
		return err
	}
	registeredHere := make(map[string]struct{})
	for _, path := range registered {
		if path == w.root {
			return fail(
				"WORKSPACE_OWNERSHIP_MISMATCH", "validate registered workspace path", nil)
		}
		if !pathWithin(w.root, path) {
			continue
		}
		relative, err := filepath.Rel(w.treesRoot, path)
		if err != nil || filepath.Dir(relative) != "." ||
			!workspaceTokenPattern.MatchString(relative) {
			return fail(
				"WORKSPACE_OWNERSHIP_MISMATCH", "validate registered workspace path", nil)
		}
		if _, ok := markers[relative]; !ok {
			return fail(
				"WORKSPACE_OWNERSHIP_MISMATCH", "validate registered workspace owner", nil)
		}
		registeredHere[path] = struct{}{}
	}
	tokens := make([]string, 0, len(markers))
	for token := range markers {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	for _, token := range tokens {
		tree := filepath.Join(w.treesRoot, token)
		if _, ok := registeredHere[tree]; ok {
			if _, err := os.Lstat(tree); err == nil {
				if err := makeWorkspaceRemovable(tree); err != nil {
					return fail(
						"WORKSPACE_CLEANUP_FAILED",
						"restore abandoned workspace permissions",
						err,
					)
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return fail("WORKSPACE_CLEANUP_FAILED", "inspect abandoned worktree", err)
			}
			if _, err := w.repository.run(
				nil, nil, "worktree", "remove", "--force", "--", tree,
			); err != nil {
				return fail("WORKSPACE_CLEANUP_FAILED", "remove abandoned worktree", err)
			}
		} else if _, err := os.Lstat(tree); err == nil {
			if err := os.RemoveAll(tree); err != nil {
				return fail("WORKSPACE_CLEANUP_FAILED", "remove abandoned workspace tree", err)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fail("WORKSPACE_CLEANUP_FAILED", "inspect abandoned workspace tree", err)
		}
		if err := os.Remove(markers[token]); err != nil {
			return fail("WORKSPACE_CLEANUP_FAILED", "remove abandoned workspace lease", err)
		}
	}
	return nil
}

func validateTrackKey(key TrackKey) error {
	if !workspaceIdentityPattern.MatchString(key.Release) ||
		!workspaceIdentityPattern.MatchString(key.Track) {
		return fail("INVALID_WORKSPACE_KEY", "validate track key", nil)
	}
	return nil
}

func trackHeadRef(key TrackKey) string {
	return "refs/heads/track/" + key.Release + "/" + key.Track
}

func validView(view WorkspaceView) bool {
	switch view {
	case PlannerView, DesignView, CaptainView, ImplementationView,
		WorkVerifierView, AssemblyVerifierView, ReleaseAssemblyView:
		return true
	default:
		return false
	}
}

func randomWorkspaceName() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (w *Workspaces) open(
	head OID,
	key TrackKey,
	view WorkspaceView,
	access WorkspaceAccess,
	writerLock *os.File,
) (lease *WorkspaceLease, resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(
				resultErr,
				releasePrivateLock(writerLock, "workspace writer"),
			)
		}
	}()
	if w == nil || w.repository == nil {
		return nil, fail("INVALID_WORKSPACE_OWNER", "open workspace", nil)
	}
	if err := w.repository.validateOID(head); err != nil {
		return nil, err
	}
	if !validView(view) ||
		(access != WorkspaceReadOnly && access != WorkspaceReadWrite) {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "open workspace", nil)
	}
	if access == WorkspaceReadWrite && view != ImplementationView {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "open writable workspace", nil)
	}
	if access == WorkspaceReadOnly && view == ImplementationView {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "open implementation workspace", nil)
	}
	if access == WorkspaceReadWrite {
		if err := validateWorkspaceWriterLock(
			writerLock,
			w.repository.commonDir,
			key,
		); err != nil {
			return nil, err
		}
	} else if writerLock != nil {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "open read-only workspace", nil)
	}
	switch view {
	case PlannerView:
	case ReleaseAssemblyView:
		if !workspaceIdentityPattern.MatchString(key.Release) ||
			key.Track != "" {
			return nil, fail(
				"INVALID_WORKSPACE_KEY",
				"validate release assembly key",
				nil,
			)
		}
	default:
		if err := validateTrackKey(key); err != nil {
			return nil, err
		}
	}
	name, err := randomWorkspaceName()
	if err != nil {
		return nil, fail("WORKSPACE_CREATE_FAILED", "name workspace", err)
	}
	path := filepath.Join(w.treesRoot, name)
	marker := filepath.Join(w.leasesRoot, name)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, fail("WORKSPACE_OWNER_CLOSED", "open workspace", nil)
	}
	if err := writeExclusiveMarker(marker, leaseMarker(w.identity, name)); err != nil {
		return nil, fail("WORKSPACE_CREATE_FAILED", "claim workspace path", err)
	}
	if _, err := w.repository.run(
		nil,
		nil,
		"worktree",
		"add",
		"--quiet",
		"--detach",
		"--",
		path,
		head.String(),
	); err != nil {
		_ = os.Remove(marker)
		return nil, fail("WORKSPACE_CREATE_FAILED", "materialize workspace", err)
	}
	lease = &WorkspaceLease{
		owner: w, path: path, token: name, key: key, view: view, access: access, head: head,
		writerLock: writerLock,
	}
	if access == WorkspaceReadOnly {
		if err := setWorkspaceReadOnly(path); err != nil {
			_ = makeWorkspaceRemovable(path)
			_, _ = w.repository.run(nil, nil, "worktree", "remove", "--force", "--", path)
			_ = os.Remove(marker)
			return nil, err
		}
		lease.readOnly = true
	}
	w.leases[path] = lease
	writerLock = nil
	return lease, nil
}

// OpenSnapshot admits a detached Planner view at one already admitted commit.
func (w *Workspaces) OpenSnapshot(head OID) (*WorkspaceLease, error) {
	return w.open(head, TrackKey{}, PlannerView, WorkspaceReadOnly, nil)
}

// OpenReleaseAssembly binds one opaque, read-only engine lease to the exact
// current release authority. Baton's compare-and-set action remains the
// mutation authority; this lease only establishes the Coach-style topology.
func (w *Workspaces) OpenReleaseAssembly(
	release string,
	expected OID,
) (*ReleaseAssemblyLease, error) {
	if !workspaceIdentityPattern.MatchString(release) ||
		w == nil || w.repository == nil {
		return nil, fail(
			"INVALID_WORKSPACE_KEY",
			"open release assembly workspace",
			nil,
		)
	}
	if err := w.repository.validateOID(expected); err != nil {
		return nil, err
	}
	releaseRef := "refs/heads/release-wt/" + release
	exact := func() error {
		captured, err := w.repository.CaptureHeadRefs([]string{releaseRef})
		if err != nil {
			return err
		}
		if len(captured) != 1 ||
			captured[0].State != RefDirect ||
			captured[0].Head != expected {
			return fail(
				"AUTHORITY_MOVED",
				"open release assembly workspace",
				nil,
			)
		}
		return nil
	}
	if err := exact(); err != nil {
		return nil, err
	}
	workspace, err := w.open(
		expected,
		TrackKey{Release: release},
		ReleaseAssemblyView,
		WorkspaceReadOnly,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if err := exact(); err != nil {
		return nil, errors.Join(err, workspace.Close())
	}
	return &ReleaseAssemblyLease{workspace: workspace}, nil
}

// OpenTrack resolves the engine-derived owner ref and materializes its exact
// current head.
func (w *Workspaces) OpenTrack(
	key TrackKey,
	view WorkspaceView,
) (*WorkspaceLease, error) {
	if err := validateTrackKey(key); err != nil {
		return nil, err
	}
	if w == nil || w.repository == nil {
		return nil, fail("INVALID_WORKSPACE_OWNER", "open track workspace", nil)
	}
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return nil, fail("WORKSPACE_OWNER_CLOSED", "open track workspace", nil)
	}
	access := WorkspaceReadOnly
	if view == ImplementationView {
		access = WorkspaceReadWrite
	} else if view != DesignView && view != CaptainView {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "open track workspace", nil)
	}
	var writerLock *os.File
	if access == WorkspaceReadWrite {
		var err error
		// Admit the one writer before reading the track head. Capturing first
		// would allow a predecessor to publish and release between these steps,
		// materializing this workspace from stale authority.
		writerLock, err = acquireWorkspaceWriterLock(w.repository.commonDir, key)
		if err != nil {
			return nil, err
		}
	}
	releaseWriter := func(cause error) error {
		return errors.Join(
			cause,
			releasePrivateLock(writerLock, "workspace writer"),
		)
	}
	trackRef := trackHeadRef(key)
	refs := []string{trackRef}
	if view == DesignView {
		refs = append(refs, "refs/heads/release-wt/"+key.Release)
	}
	captured, err := w.repository.CaptureHeadRefs(refs)
	if err != nil {
		return nil, releaseWriter(err)
	}
	byRef := make(map[string]RefHead, len(captured))
	for _, value := range captured {
		byRef[value.Ref] = value
	}
	head := byRef[trackRef]
	if head.State == RefAbsent && view == DesignView {
		head = byRef["refs/heads/release-wt/"+key.Release]
	}
	if head.State != RefDirect || head.Head.IsZero() {
		return nil, releaseWriter(
			fail("TRACK_NOT_MATERIALIZED", "open track workspace", nil),
		)
	}
	return w.open(head.Head, key, view, access, writerLock)
}

// OpenCandidate creates a fresh, detached, physically read-only verifier
// workspace. Work and assembly verification are intentionally separate views.
func (w *Workspaces) OpenCandidate(
	key TrackKey,
	view WorkspaceView,
	candidate OID,
) (*WorkspaceLease, error) {
	if view != WorkVerifierView && view != AssemblyVerifierView {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "open verifier workspace", nil)
	}
	return w.open(candidate, key, view, WorkspaceReadOnly, nil)
}

func setWorkspaceReadOnly(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		mode := os.FileMode(0o400)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o500
		}
		return os.Chmod(path, mode)
	})
	if err != nil {
		return fail("WORKSPACE_PROTECTION_FAILED", "protect verifier workspace", err)
	}
	sort.Slice(directories, func(i, j int) bool {
		return len(directories[i]) > len(directories[j])
	})
	for _, directory := range directories {
		if err := os.Chmod(directory, 0o500); err != nil {
			return fail("WORKSPACE_PROTECTION_FAILED", "protect verifier workspace", err)
		}
	}
	return nil
}

func makeWorkspaceRemovable(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o700
		}
		return os.Chmod(path, mode)
	})
}

func (w *Workspaces) SealTrack(lease *WorkspaceLease) (SealedCandidate, error) {
	return w.SealTrackWithClaim(lease, nil)
}

// SealTrackWithClaim prepares the deterministic candidate object, then calls
// beforeUpdate before the owner ref can move. The runtime uses this seam to
// durably commit a finite effect claim containing both the old and new object
// identities. The callback must only persist that claim.
func (w *Workspaces) SealTrackWithClaim(
	lease *WorkspaceLease,
	beforeUpdate func(SealedCandidate) error,
) (SealedCandidate, error) {
	return w.sealTrackWithClaim(lease, nil, beforeUpdate)
}

// SealTrackGuardedWithClaim is SealTrackWithClaim plus one atomic release,
// target, and track ref transaction. A target or plan supersession therefore
// loses before the candidate can become the track head.
func (w *Workspaces) SealTrackGuardedWithClaim(
	lease *WorkspaceLease,
	authority SealAuthority,
	beforeUpdate func(SealedCandidate) error,
) (SealedCandidate, error) {
	return w.sealTrackWithClaim(lease, &authority, beforeUpdate)
}

func (w *Workspaces) sealTrackWithClaim(
	lease *WorkspaceLease,
	authority *SealAuthority,
	beforeUpdate func(SealedCandidate) error,
) (SealedCandidate, error) {
	if w == nil || lease == nil || lease.owner != w || lease.closed ||
		lease.access != WorkspaceReadWrite || lease.view != ImplementationView {
		return SealedCandidate{}, fail("INVALID_WORKSPACE_LEASE", "seal track", nil)
	}
	if err := validateTrackKey(lease.key); err != nil {
		return SealedCandidate{}, err
	}
	releaseRef := "refs/heads/release-wt/" + lease.key.Release
	trackRef := trackHeadRef(lease.key)
	if authority != nil {
		if w.repository.validateOID(authority.ReleaseHead) != nil ||
			w.repository.validateOID(authority.TargetHead) != nil {
			return SealedCandidate{}, fail(
				"OBJECT_FORMAT_MISMATCH", "guard seal authority", nil)
		}
		if err := ValidateHeadRef(authority.TargetRef); err != nil {
			return SealedCandidate{}, err
		}
		if authority.TargetRef == releaseRef || authority.TargetRef == trackRef {
			return SealedCandidate{}, fail(
				"INVALID_REF_TRANSACTION", "guard seal authority", nil)
		}
	}
	if _, err := w.repository.runAt(
		lease.path,
		nil,
		nil,
		"add",
		"--all",
		"--",
		".",
	); err != nil {
		return SealedCandidate{}, fail("CANDIDATE_SEAL_FAILED", "stage workspace", err)
	}
	rawPaths, err := w.repository.runAt(
		lease.path,
		nil,
		nil,
		"diff",
		"--cached",
		"--name-only",
		"-z",
		lease.head.String(),
		"--",
	)
	if err != nil {
		return SealedCandidate{}, fail("CANDIDATE_SEAL_FAILED", "inventory candidate", err)
	}
	var changed []string
	for _, raw := range bytes.Split(rawPaths, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		name := string(raw)
		if err := ValidatePath(name, false); err != nil {
			return SealedCandidate{}, err
		}
		if name == recordRoot || strings.HasPrefix(name, recordRoot+"/") {
			return SealedCandidate{}, fail("AUTHORITY_PATH_CHANGED", "seal candidate", nil)
		}
		changed = append(changed, name)
	}
	if len(changed) == 0 {
		return SealedCandidate{}, fail("EMPTY_CANDIDATE", "seal candidate", nil)
	}
	sort.Strings(changed)
	rawTree, err := w.repository.runAt(lease.path, nil, nil, "write-tree")
	if err != nil {
		return SealedCandidate{}, fail("CANDIDATE_SEAL_FAILED", "write candidate tree", err)
	}
	tree, err := w.repository.parseOID(string(rawTree))
	if err != nil {
		return SealedCandidate{}, err
	}
	timestamp, err := w.repository.CommitTimestamp(lease.head)
	if err != nil {
		return SealedCandidate{}, err
	}
	message := fmt.Sprintf(
		"sworn(%s/%s): implementation candidate\n",
		lease.key.Release,
		lease.key.Track,
	)
	rawCommit, err := w.repository.run(
		[]byte(message),
		commitEnvironment(
			Identity{Name: "Sworn Runtime", Email: "runtime@sworn.invalid"},
			timestamp+1,
		),
		"commit-tree",
		tree.String(),
		"-p",
		lease.head.String(),
	)
	if err != nil {
		return SealedCandidate{}, fail("CANDIDATE_SEAL_FAILED", "write candidate commit", err)
	}
	candidate, err := w.repository.parseOID(string(rawCommit))
	if err != nil {
		return SealedCandidate{}, err
	}
	sealed := SealedCandidate{
		Before:       lease.head,
		Candidate:    candidate,
		Tree:         tree,
		ChangedPaths: append([]string(nil), changed...),
	}
	if beforeUpdate != nil {
		if err := beforeUpdate(sealed); err != nil {
			return SealedCandidate{}, err
		}
	}
	refs := []string{trackRef}
	operations := []RefOperation{{
		Kind: UpdateRef, Ref: trackRef, NewHead: &candidate, Expected: &lease.head,
	}}
	if authority != nil {
		refs = append(refs, releaseRef, authority.TargetRef)
		operations = append(operations,
			RefOperation{Kind: VerifyRef, Ref: releaseRef, Expected: &authority.ReleaseHead},
			RefOperation{Kind: VerifyRef, Ref: authority.TargetRef, Expected: &authority.TargetHead},
		)
	}
	captured, err := w.repository.CaptureHeadRefs(refs)
	if err != nil {
		return SealedCandidate{}, err
	}
	expected := map[string]OID{trackRef: lease.head}
	if authority != nil {
		expected[releaseRef] = authority.ReleaseHead
		expected[authority.TargetRef] = authority.TargetHead
	}
	if len(captured) != len(expected) {
		return SealedCandidate{}, fail("AUTHORITY_MOVED", "seal candidate", nil)
	}
	for _, observed := range captured {
		if observed.State != RefDirect || observed.Head != expected[observed.Ref] {
			code := "AUTHORITY_MOVED"
			if observed.Ref == trackRef {
				code = "TRACK_MOVED"
			}
			return SealedCandidate{}, fail(code, "seal candidate", nil)
		}
	}
	if err := w.repository.ApplyRefTransaction(captured, operations); err != nil {
		return SealedCandidate{}, err
	}
	return sealed, nil
}

func (w *Workspaces) ReconcileSeal(
	key TrackKey,
	before, candidate OID,
) (SealReconciliation, error) {
	if err := validateTrackKey(key); err != nil {
		return "", err
	}
	if err := w.repository.validateOID(before); err != nil {
		return "", err
	}
	if err := w.repository.validateOID(candidate); err != nil {
		return "", err
	}
	captured, err := w.repository.CaptureHeadRefs([]string{trackHeadRef(key)})
	if err != nil {
		return "", err
	}
	if len(captured) != 1 || captured[0].State != RefDirect {
		return SealAmbiguous, nil
	}
	switch captured[0].Head {
	case before:
		return SealAllOld, nil
	case candidate:
		return SealAllNew, nil
	default:
		return SealAmbiguous, nil
	}
}

func (l *WorkspaceLease) Close() error {
	if l == nil || l.closed {
		return nil
	}
	return l.owner.remove(l)
}

func (w *Workspaces) remove(lease *WorkspaceLease) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if lease.closed {
		return nil
	}
	if !workspaceTokenPattern.MatchString(lease.token) ||
		lease.path != filepath.Join(w.treesRoot, lease.token) {
		return fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate workspace lease path", nil)
	}
	marker := filepath.Join(w.leasesRoot, lease.token)
	if err := validateMarker(marker, leaseMarker(w.identity, lease.token)); err != nil {
		return err
	}
	if lease.readOnly {
		if err := makeWorkspaceRemovable(lease.path); err != nil {
			return fail("WORKSPACE_CLEANUP_FAILED", "restore workspace permissions", err)
		}
		raw, err := w.repository.runAt(
			lease.path,
			nil,
			nil,
			"status",
			"--porcelain=v1",
			"--untracked-files=all",
		)
		if err != nil {
			return fail("WORKSPACE_CLEANUP_FAILED", "inspect verifier workspace", err)
		}
		if len(raw) != 0 {
			return fail("WORKSPACE_MUTATED", "inspect verifier workspace", nil)
		}
	}
	if _, err := w.repository.run(
		nil,
		nil,
		"worktree",
		"remove",
		"--force",
		"--",
		lease.path,
	); err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "remove workspace", err)
	}
	if err := os.Remove(marker); err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "remove workspace lease", err)
	}
	lease.closed = true
	delete(w.leases, lease.path)
	writerLock := lease.writerLock
	lease.writerLock = nil
	return releasePrivateLock(writerLock, "workspace writer")
}

func (w *Workspaces) Close() (result error) {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	if w.closed && w.lock == nil {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	leases := make([]*WorkspaceLease, 0, len(w.leases))
	for _, lease := range w.leases {
		leases = append(leases, lease)
	}
	w.mu.Unlock()
	var joined error
	for _, lease := range leases {
		joined = errors.Join(joined, w.remove(lease))
	}
	if joined != nil {
		return joined
	}
	defer func() {
		result = errors.Join(result, w.releaseLock())
	}()
	if err := validateMarker(
		filepath.Join(w.root, "owner"),
		rootMarker(w.identity),
	); err != nil {
		return err
	}
	for _, directory := range []string{w.treesRoot, w.leasesRoot} {
		if err := os.Remove(directory); err != nil {
			return fail("WORKSPACE_CLEANUP_FAILED", "remove workspace directory", err)
		}
	}
	if err := os.Remove(filepath.Join(w.root, "owner")); err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "remove workspace owner", err)
	}
	if err := os.Remove(w.root); err != nil {
		return fail("WORKSPACE_CLEANUP_FAILED", "remove workspace root", err)
	}
	if err := w.removeLockFile(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) runAt(
	directory string,
	stdin []byte,
	extraEnv []string,
	args ...string,
) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || !filepath.IsAbs(resolved) || filepath.Clean(resolved) != directory {
		return nil, fail("INVALID_WORKSPACE", "run Git in workspace", nil)
	}
	home, err := os.MkdirTemp("", "sworn-git-home-*")
	if err != nil {
		return nil, fail("GIT_EXECUTION_FAILED", "create Git home", err)
	}
	defer os.RemoveAll(home)
	if err := os.MkdirAll(filepath.Join(home, "hooks"), 0o700); err != nil {
		return nil, fail("GIT_EXECUTION_FAILED", "create hooks directory", err)
	}
	commandArgs := []string{
		"-C", directory,
		"-c", "core.hooksPath=" + filepath.Join(home, "hooks"),
		"-c", "core.fsmonitor=false",
		"-c", "core.quotePath=false",
	}
	commandArgs = append(commandArgs, args...)
	command := exec.Command(r.git, commandArgs...)
	command.Env = literalEnvironment(home, extraEnv, "/dev/null")
	command.Stdin = bytes.NewReader(stdin)
	var stdout, stderr boundedBuffer
	stdout.limit, stderr.limit = MaxCommandOutput, MaxDiagnostic
	command.Stdout, command.Stderr = &stdout, &stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fail("GIT_EXECUTION_FAILED", "start Git workspace command", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	timer := time.NewTimer(CommandTimeout)
	defer timer.Stop()
	select {
	case err = <-wait:
	case <-timer.C:
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		err = <-wait
		return nil, fail("GIT_TIMEOUT", "run Git workspace command", err)
	}
	quietContext, cancel := context.WithTimeout(context.Background(), time.Second)
	quiet := processGroupQuiet(quietContext, command.Process.Pid)
	cancel()
	if err != nil || !quiet || stdout.overflow || stderr.overflow {
		diagnostic := strings.TrimSpace(stderr.String())
		if len(diagnostic) > 256 {
			diagnostic = diagnostic[:256]
		}
		if diagnostic == "" {
			diagnostic = "Git workspace command failed"
		}
		return nil, fail("GIT_EXECUTION_FAILED", "run Git workspace command", errors.New(diagnostic))
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}
