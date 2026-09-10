package gitx

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const workspaceAttributionSchemaVersion = "sworn.workspace-attribution/v1"

// WorkspaceAttribution durably records which run owns an implementation
// dispatch's writable workspace, and the exact authority it was opened
// against, before any driver dispatch can begin. It exists so a replacement
// owner can prove, after a hard process exit, that an abandoned worktree is
// attributable production work rather than debris, and what to compare it
// against before treating it as current.
type WorkspaceAttribution struct {
	SchemaVersion  string   `json:"schema_version"`
	Token          string   `json:"token"`
	Identity       string   `json:"identity"`
	CommonDir      string   `json:"common_dir"`
	RunID          string   `json:"run_id"`
	Release        string   `json:"release"`
	Track          string   `json:"track"`
	Slice          string   `json:"slice"`
	PlanOID        string   `json:"plan_oid"`
	PlanDigest     string   `json:"plan_digest"`
	ContractPath   string   `json:"contract_path,omitempty"`
	ContractDigest string   `json:"contract_digest"`
	PreparedBase   string   `json:"prepared_base"`
	DispatchWork   string   `json:"dispatch_work"`
	Epoch          int64    `json:"epoch"`
	Try            int64    `json:"try"`
	ScopeInclude   []string `json:"scope_include,omitempty"`
	ScopeExclude   []string `json:"scope_exclude,omitempty"`
	CreatedAt      string   `json:"created_at"`
}

// AttributeWorkspace durably records lease as owned production implementation
// work, before any driver dispatch begins. It must be called at most once per
// lease token; a second call for the same token fails closed rather than
// silently overwriting the original attribution.
func (w *Workspaces) AttributeWorkspace(
	lease *WorkspaceLease,
	attribution WorkspaceAttribution,
) error {
	if w == nil || lease == nil || lease.owner != w || lease.closed ||
		lease.access != WorkspaceReadWrite || lease.view != ImplementationView {
		return fail("INVALID_WORKSPACE_LEASE", "attribute workspace", nil)
	}
	if attribution.Release != lease.key.Release || attribution.Track != lease.key.Track {
		return fail("INVALID_WORKSPACE_REQUEST", "attribute workspace", nil)
	}
	attribution.SchemaVersion = workspaceAttributionSchemaVersion
	attribution.Token = lease.token
	attribution.Identity = w.identity
	if attribution.CreatedAt == "" {
		attribution.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	body, err := json.Marshal(attribution)
	if err != nil {
		return fail("WORKSPACE_ATTRIBUTION_FAILED", "marshal workspace attribution", err)
	}
	if err := ensurePrivateDirectory(w.attributionsRoot); err != nil {
		return err
	}
	path := filepath.Join(w.attributionsRoot, lease.token)
	if err := writeExclusiveMarker(path, body); err != nil {
		return fail("WORKSPACE_ATTRIBUTION_FAILED", "write workspace attribution", err)
	}
	return nil
}

// ClearAttribution removes a workspace's durable attribution record, if any.
// It is called once a lease's fate (restored, salvaged, or discarded empty)
// is durably decided, or when the lease that wrote it closes or fences
// normally.
func (w *Workspaces) ClearAttribution(token string) error {
	if w == nil || !workspaceTokenPattern.MatchString(token) {
		return fail("INVALID_WORKSPACE_REQUEST", "clear workspace attribution", nil)
	}
	path := filepath.Join(w.attributionsRoot, token)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail("WORKSPACE_CLEANUP_FAILED", "remove workspace attribution", err)
	}
	return nil
}

// readAttribution loads and validates the attribution record for token. On
// validation failure it still returns whatever was parseable (which may be
// the zero value) alongside the error, so a caller can fence the tree under
// its best-known track key rather than an empty one.
func (w *Workspaces) readAttribution(token string) (WorkspaceAttribution, error) {
	path := filepath.Join(w.attributionsRoot, token)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return WorkspaceAttribution{}, fail("WORKSPACE_OWNERSHIP_MISMATCH", "inspect workspace attribution", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return WorkspaceAttribution{}, fail("WORKSPACE_OWNERSHIP_MISMATCH", "inspect workspace attribution owner", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceAttribution{}, fail("WORKSPACE_OWNERSHIP_MISMATCH", "read workspace attribution", err)
	}
	var attribution WorkspaceAttribution
	if err := json.Unmarshal(data, &attribution); err != nil {
		return WorkspaceAttribution{}, fail("CHECKPOINT_CORRUPT", "parse workspace attribution", err)
	}
	if attribution.SchemaVersion != workspaceAttributionSchemaVersion ||
		attribution.Token != token || attribution.Identity != w.identity ||
		!workspaceIdentityPattern.MatchString(attribution.Release) ||
		!workspaceIdentityPattern.MatchString(attribution.Track) {
		return attribution, fail("CHECKPOINT_CORRUPT", "validate workspace attribution", nil)
	}
	return attribution, nil
}

// hasAttributionMarker reports whether an attribution record file exists for
// token, regardless of whether its content is valid. recoverAbandoned uses
// exactly this test (existence, not validity) to decide whether to defer
// deletion: a present-but-corrupt record must still be preserved for
// reconciliation to quarantine explicitly, never silently discarded by the
// same pass that would otherwise delete ordinary debris.
func (w *Workspaces) hasAttributionMarker(token string) bool {
	_, err := os.Lstat(filepath.Join(w.attributionsRoot, token))
	return err == nil
}

// hasAnyAttributed reports whether any durable attribution record remains,
// so Close can defer root teardown exactly as it already does for fenced
// workspaces: a retained attributed tree still occupies treesRoot/leasesRoot,
// and must survive an ordinary approval-path Close that never runs
// reconciliation.
func (w *Workspaces) hasAnyAttributed() bool {
	entries, err := os.ReadDir(w.attributionsRoot)
	if err != nil {
		return false
	}
	return len(entries) != 0
}

// ListAttributions scans every durable attribution record for this workspace
// owner. A record that fails to parse or validate is treated as a corrupt
// recovery binding: it is quarantined under a fence immediately, using
// whatever release/track/slice its bytes could still yield, and excluded from
// the returned slice so the caller never mistakes it for adoptable work.
func (w *Workspaces) ListAttributions() ([]WorkspaceAttribution, error) {
	if w == nil {
		return nil, nil
	}
	entries, err := os.ReadDir(w.attributionsRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fail("WORKSPACE_CLEANUP_FAILED", "read workspace attributions", err)
	}
	var result []WorkspaceAttribution
	for _, entry := range entries {
		if entry.IsDir() || !workspaceTokenPattern.MatchString(entry.Name()) {
			continue
		}
		attribution, err := w.readAttribution(entry.Name())
		if err != nil {
			key := TrackKey{Release: attribution.Release, Track: attribution.Track}
			_ = w.FenceAbandonedWorkspace(
				entry.Name(), key, attribution.Slice,
				"CHECKPOINT_CORRUPT", "corrupt or invalid workspace attribution record: "+err.Error(),
			)
			_ = w.ClearAttribution(entry.Name())
			continue
		}
		result = append(result, attribution)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Token < result[j].Token })
	return result, nil
}

// FenceAbandonedWorkspace quarantines an abandoned tree that has no live
// lease object (a corrupt or foreign attribution, or a recovery binding this
// run could not safely adopt), using the same fence representation a live
// lease's Fence writes. It does not require or take the track writer lock:
// the worker that opened this tree is already gone, and quarantine is a pure
// filesystem marker read by isFencedTree/hasFencedWorkspace.
func (w *Workspaces) FenceAbandonedWorkspace(
	token string,
	key TrackKey,
	slice, reason, detail string,
) error {
	if !workspaceTokenPattern.MatchString(token) {
		return fail("INVALID_WORKSPACE_REQUEST", "fence abandoned workspace", nil)
	}
	treePath := filepath.Join(w.treesRoot, token)
	return writeFenceRecord(treePath, w.fencesRoot, token, w.identity, key, slice, reason, detail)
}

// AdoptAbandonedWorkspace reconstructs a writable implementation lease bound
// to an existing attributed abandoned tree and lease marker, admitting the
// track's sole writer before any state is read or changed - the same
// ordering invariant OpenTrack enforces for a freshly materialized
// workspace. It never creates a new worktree; the tree must already exist,
// registered, from the interrupted dispatch that attributed it.
func (w *Workspaces) AdoptAbandonedWorkspace(
	token string,
	key TrackKey,
	head OID,
) (lease *WorkspaceLease, resultErr error) {
	if w == nil || w.repository == nil {
		return nil, fail("INVALID_WORKSPACE_OWNER", "adopt workspace", nil)
	}
	if !workspaceTokenPattern.MatchString(token) {
		return nil, fail("INVALID_WORKSPACE_REQUEST", "adopt workspace", nil)
	}
	if err := validateTrackKey(key); err != nil {
		return nil, err
	}
	if err := w.repository.validateOID(head); err != nil {
		return nil, err
	}
	if w.hasFencedWorkspace(key) {
		return nil, fail("WORKSPACE_FENCED", "adopt workspace", errors.New("track has a quarantined fenced workspace"))
	}
	// Admit the one writer before reading or trusting anything else about
	// this tree, mirroring OpenTrack: a live prior worker refuses adoption
	// here rather than racing it.
	writerLock, err := acquireWorkspaceWriterLock(w.repository.commonDir, key)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, releasePrivateLock(writerLock, "workspace writer"))
		}
	}()
	marker := filepath.Join(w.leasesRoot, token)
	if err := validateMarker(marker, leaseMarker(w.identity, token)); err != nil {
		return nil, err
	}
	path := filepath.Join(w.treesRoot, token)
	if info, statErr := os.Lstat(path); statErr != nil || !info.IsDir() {
		return nil, fail("WORKSPACE_OWNERSHIP_MISMATCH", "inspect adopted workspace tree", statErr)
	}
	registered, err := registeredWorktreePaths(w.repository)
	if err != nil {
		return nil, err
	}
	found := false
	for _, candidate := range registered {
		if candidate == path {
			found = true
			break
		}
	}
	if !found {
		return nil, fail("WORKSPACE_OWNERSHIP_MISMATCH", "validate adopted workspace registration", nil)
	}
	rawHead, err := w.repository.runAt(path, nil, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, fail("CHECKPOINT_CORRUPT", "read adopted workspace head", err)
	}
	actualHead, err := w.repository.parseOID(strings.TrimSpace(string(rawHead)))
	if err != nil {
		return nil, fail("CHECKPOINT_CORRUPT", "parse adopted workspace head", err)
	}
	if actualHead != head {
		return nil, fail(
			"CHECKPOINT_CORRUPT",
			"adopted workspace head does not match its attributed prepared base",
			nil,
		)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, fail("WORKSPACE_OWNER_CLOSED", "adopt workspace", nil)
	}
	lease = &WorkspaceLease{
		owner: w, path: path, token: token, key: key,
		view: ImplementationView, access: WorkspaceReadWrite, head: head,
		writerLock: writerLock,
	}
	w.leases[path] = lease
	writerLock = nil
	return lease, nil
}
