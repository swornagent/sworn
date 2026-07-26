package gitx

import (
	"bytes"
	"context"
	"crypto/rand"
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

var workspaceIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type WorkspaceView string

const (
	PlannerView          WorkspaceView = "planner"
	DesignView           WorkspaceView = "design"
	CaptainView          WorkspaceView = "captain"
	ImplementationView   WorkspaceView = "implementation"
	WorkVerifierView     WorkspaceView = "work_verifier"
	AssemblyVerifierView WorkspaceView = "assembly_verifier"
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
	owner    *Workspaces
	path     string
	key      TrackKey
	view     WorkspaceView
	access   WorkspaceAccess
	head     OID
	closed   bool
	readOnly bool
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

type SealReconciliation string

const (
	SealAllOld    SealReconciliation = "all_old"
	SealAllNew    SealReconciliation = "all_new"
	SealAmbiguous SealReconciliation = "ambiguous"
)

type Workspaces struct {
	repository *Repository
	root       string
	mu         sync.Mutex
	leases     map[string]*WorkspaceLease
	closed     bool
}

// NewWorkspaces creates an engine-owned workspace root. No caller-provided
// path, ref, parent, Git command, or merge mode crosses this API.
func NewWorkspaces(repository *Repository) (*Workspaces, error) {
	if repository == nil {
		return nil, fail("INVALID_REPOSITORY", "create workspaces", nil)
	}
	root, err := os.MkdirTemp("", "sworn-workspaces-v1-")
	if err != nil {
		return nil, fail("WORKSPACE_CREATE_FAILED", "create workspace root", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return nil, fail("WORKSPACE_CREATE_FAILED", "protect workspace root", err)
	}
	return &Workspaces{
		repository: repository,
		root:       root,
		leases:     make(map[string]*WorkspaceLease),
	}, nil
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
		WorkVerifierView, AssemblyVerifierView:
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
) (*WorkspaceLease, error) {
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
	if view != PlannerView {
		if err := validateTrackKey(key); err != nil {
			return nil, err
		}
	}
	name, err := randomWorkspaceName()
	if err != nil {
		return nil, fail("WORKSPACE_CREATE_FAILED", "name workspace", err)
	}
	path := filepath.Join(w.root, name)
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil, fail("WORKSPACE_OWNER_CLOSED", "open workspace", nil)
	}
	w.mu.Unlock()
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
		return nil, fail("WORKSPACE_CREATE_FAILED", "materialize workspace", err)
	}
	lease := &WorkspaceLease{
		owner: w, path: path, key: key, view: view, access: access, head: head,
	}
	if access == WorkspaceReadOnly {
		if err := setWorkspaceReadOnly(path); err != nil {
			_ = makeWorkspaceRemovable(path)
			_, _ = w.repository.run(nil, nil, "worktree", "remove", "--force", "--", path)
			return nil, err
		}
		lease.readOnly = true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		_ = makeWorkspaceRemovable(path)
		_, _ = w.repository.run(nil, nil, "worktree", "remove", "--force", "--", path)
		return nil, fail("WORKSPACE_OWNER_CLOSED", "open workspace", nil)
	}
	w.leases[path] = lease
	return lease, nil
}

// OpenSnapshot admits a detached Planner view at one already admitted commit.
func (w *Workspaces) OpenSnapshot(head OID) (*WorkspaceLease, error) {
	return w.open(head, TrackKey{}, PlannerView, WorkspaceReadOnly)
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
	access := WorkspaceReadOnly
	if view == ImplementationView {
		access = WorkspaceReadWrite
	} else if view != DesignView && view != CaptainView {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "open track workspace", nil)
	}
	trackRef := trackHeadRef(key)
	refs := []string{trackRef}
	if view == DesignView {
		refs = append(refs, "refs/heads/release-wt/"+key.Release)
	}
	captured, err := w.repository.CaptureHeadRefs(refs)
	if err != nil {
		return nil, err
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
		return nil, fail("TRACK_NOT_MATERIALIZED", "open track workspace", nil)
	}
	return w.open(head.Head, key, view, access)
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
	return w.open(candidate, key, view, WorkspaceReadOnly)
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
	if w == nil || lease == nil || lease.owner != w || lease.closed ||
		lease.access != WorkspaceReadWrite || lease.view != ImplementationView {
		return SealedCandidate{}, fail("INVALID_WORKSPACE_LEASE", "seal track", nil)
	}
	if err := validateTrackKey(lease.key); err != nil {
		return SealedCandidate{}, err
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
	ref := trackHeadRef(lease.key)
	captured, err := w.repository.CaptureHeadRefs([]string{ref})
	if err != nil {
		return SealedCandidate{}, err
	}
	if len(captured) != 1 || captured[0].State != RefDirect ||
		captured[0].Head != lease.head {
		return SealedCandidate{}, fail("TRACK_MOVED", "seal candidate", nil)
	}
	if err := w.repository.ApplyRefTransaction(captured, []RefOperation{{
		Kind: UpdateRef, Ref: ref, NewHead: &candidate, Expected: &lease.head,
	}}); err != nil {
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
	if lease.closed {
		w.mu.Unlock()
		return nil
	}
	lease.closed = true
	delete(w.leases, lease.path)
	w.mu.Unlock()
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
	return nil
}

func (w *Workspaces) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	if w.closed {
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
	if err := os.RemoveAll(w.root); err != nil {
		joined = errors.Join(joined, fail("WORKSPACE_CLEANUP_FAILED", "remove workspace root", err))
	}
	return joined
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
