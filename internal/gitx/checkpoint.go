package gitx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	MaxCheckpointBytes          int64 = 64 * 1024 * 1024 // 64 MiB
	MaxCheckpointFiles          int   = 2048
	MaxCheckpointGenerations    int   = 3
	MaxAggregateCheckpointBytes int64 = 256 * 1024 * 1024 // 256 MiB
	CheckpointRefPrefix               = "refs/heads/checkpoints/"
	WorkspaceFenceFile                = ".fence"
)

type CheckpointAttempt struct {
	WorkID string
	Epoch  int64
	Try    int64
}

type CheckpointScope struct {
	Include []string
	Exclude []string
}

type CheckpointResult struct {
	Empty        bool
	Ref          string
	Commit       OID
	Tree         OID
	ProductTree  string
	StagedBytes  int64
	FileCount    int
	ChangedPaths []string
	Timestamp    int64
}

type WorkspaceFence struct {
	Token     string `json:"token"`
	Identity  string `json:"identity"`
	Release   string `json:"release"`
	Track     string `json:"track"`
	Slice     string `json:"slice,omitempty"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
	Path      string `json:"path,omitempty"`
}

func pathInCheckpointScope(scope CheckpointScope, changedPath string) bool {
	if len(scope.Include) == 0 {
		return true
	}
	included := false
	for _, inc := range scope.Include {
		if changedPath == inc || strings.HasPrefix(changedPath, inc+"/") {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, exc := range scope.Exclude {
		if changedPath == exc || strings.HasPrefix(changedPath, exc+"/") {
			return false
		}
	}
	return true
}

// Fence marks the workspace lease as fenced, writes .fence to the workspace root,
// writes an auxiliary fence marker, and ensures the workspace is not deleted when closed.
func (l *WorkspaceLease) Fence(reason, detail, slice string) error {
	if l == nil || l.owner == nil || l.closed {
		return fail("INVALID_WORKSPACE_LEASE", "fence workspace", nil)
	}
	l.fenced = true
	l.fenceReason = reason
	// A fence marker is now the durable retain-reason for this tree; the
	// attribution record (if any) has served its purpose and would otherwise
	// make reconciliation try to re-adopt an already-quarantined tree.
	_ = l.owner.ClearAttribution(l.token)
	return writeFenceRecord(l.path, l.owner.fencesRoot, l.token, l.owner.identity, l.key, slice, reason, detail)
}

// writeFenceRecord dual-writes the quarantine marker inside the workspace
// tree and to the owner's auxiliary fences directory, for both a live lease
// (WorkspaceLease.Fence) and a scan that found an abandoned tree with no live
// lease object yet (a corrupt or foreign attribution record).
func writeFenceRecord(
	treePath, fencesRoot, token, identity string,
	key TrackKey,
	slice, reason, detail string,
) error {
	fenceRecord := WorkspaceFence{
		Token:     token,
		Identity:  identity,
		Release:   key.Release,
		Track:     key.Track,
		Slice:     slice,
		Reason:    reason,
		Detail:    detail,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Path:      treePath,
	}
	body, err := json.MarshalIndent(fenceRecord, "", "  ")
	if err != nil {
		return fail("WORKSPACE_FENCE_FAILED", "marshal fence record", err)
	}
	fenceBody := append(body, '\n')

	var writeErrs []error
	fencePath := filepath.Join(treePath, WorkspaceFenceFile)
	if err := os.WriteFile(fencePath, fenceBody, 0o600); err != nil {
		writeErrs = append(writeErrs, err)
	}
	if fencesRoot != "" {
		_ = os.MkdirAll(fencesRoot, 0o700)
		auxPath := filepath.Join(fencesRoot, token)
		if err := os.WriteFile(auxPath, fenceBody, 0o600); err != nil {
			writeErrs = append(writeErrs, err)
		}
	}
	if len(writeErrs) == 2 {
		return fail("WORKSPACE_FENCE_FAILED", "write fence record", writeErrs[0])
	}
	return nil
}

func (l *WorkspaceLease) IsFenced() bool {
	if l == nil {
		return false
	}
	return l.fenced
}

func (l *WorkspaceLease) FenceReason() string {
	if l == nil {
		return ""
	}
	return l.fenceReason
}

func (l *WorkspaceLease) Token() string {
	if l == nil {
		return ""
	}
	return l.token
}

func (l *WorkspaceLease) Key() TrackKey {
	if l == nil {
		return TrackKey{}
	}
	return l.key
}

func (w *Workspaces) isFencedTree(treePath, token string) bool {
	fencePath := filepath.Join(treePath, WorkspaceFenceFile)
	if _, err := os.Lstat(fencePath); err == nil {
		data, readErr := os.ReadFile(fencePath)
		if readErr != nil {
			return true
		}
		var fence WorkspaceFence
		if unmarshalErr := json.Unmarshal(data, &fence); unmarshalErr != nil {
			return true
		}
		if fence.Token == token && fence.Identity == w.identity {
			return true
		}
	}
	if w.fencesRoot != "" {
		auxPath := filepath.Join(w.fencesRoot, token)
		if _, err := os.Lstat(auxPath); err == nil {
			data, readErr := os.ReadFile(auxPath)
			if readErr != nil {
				return true
			}
			var fence WorkspaceFence
			if unmarshalErr := json.Unmarshal(data, &fence); unmarshalErr != nil {
				return true
			}
			if fence.Token == token && fence.Identity == w.identity {
				return true
			}
		}
	}
	return false
}

func (w *Workspaces) hasAnyFenced() bool {
	entries, err := os.ReadDir(w.treesRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && workspaceTokenPattern.MatchString(entry.Name()) {
			if w.isFencedTree(filepath.Join(w.treesRoot, entry.Name()), entry.Name()) {
				return true
			}
		}
	}
	return false
}

// FencedWorkspaces scans treesRoot and returns metadata for all quarantined workspaces.
func (w *Workspaces) FencedWorkspaces() ([]WorkspaceFence, error) {
	if w == nil {
		return nil, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := os.ReadDir(w.treesRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var fenced []WorkspaceFence
	for _, entry := range entries {
		if !entry.IsDir() || !workspaceTokenPattern.MatchString(entry.Name()) {
			continue
		}
		treePath := filepath.Join(w.treesRoot, entry.Name())
		fencePath := filepath.Join(treePath, WorkspaceFenceFile)
		var auxPath string
		if w.fencesRoot != "" {
			auxPath = filepath.Join(w.fencesRoot, entry.Name())
		}
		var fence WorkspaceFence
		found := false
		if data, err := os.ReadFile(fencePath); err == nil {
			if err := json.Unmarshal(data, &fence); err == nil &&
				fence.Token == entry.Name() && fence.Identity == w.identity {
				found = true
			}
		}
		if !found && auxPath != "" {
			if data, err := os.ReadFile(auxPath); err == nil {
				if err := json.Unmarshal(data, &fence); err == nil &&
					fence.Token == entry.Name() && fence.Identity == w.identity {
					found = true
				}
			}
		}
		if !found {
			_, statPrimary := os.Lstat(fencePath)
			statAux := error(errors.New("no aux"))
			if auxPath != "" {
				_, statAux = os.Lstat(auxPath)
			}
			if statPrimary == nil || statAux == nil {
				fence = WorkspaceFence{
					Token:     entry.Name(),
					Identity:  w.identity,
					Reason:    "WORKSPACE_FENCE_CORRUPT",
					Detail:    "corrupt or truncated fence record",
					CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
				}
				found = true
			}
		}
		if found {
			if fence.Path == "" {
				fence.Path = treePath
			}
			fenced = append(fenced, fence)
		}
	}
	return fenced, nil
}

func (w *Workspaces) hasFencedWorkspace(key TrackKey) bool {
	fenced, err := w.FencedWorkspaces()
	if err != nil {
		return false
	}
	for _, f := range fenced {
		if f.Release == key.Release && f.Track == key.Track {
			return true
		}
	}
	return false
}

// FindFencedWorkspace returns information about a quarantined workspace for key if one exists.
func (w *Workspaces) FindFencedWorkspace(key TrackKey) (*WorkspaceFence, string, bool) {
	fenced, err := w.FencedWorkspaces()
	if err != nil {
		return nil, "", false
	}
	for _, f := range fenced {
		if f.Release == key.Release && f.Track == key.Track {
			treePath := filepath.Join(w.treesRoot, f.Token)
			fenceCopy := f
			return &fenceCopy, treePath, true
		}
	}
	return nil, "", false
}

// FencedWorkspacesForRun scans the workspace base for runID and returns all quarantined workspaces.
func FencedWorkspacesForRun(commonDir, runID string) ([]WorkspaceFence, error) {
	base, err := workspaceBase()
	if err != nil {
		return nil, err
	}
	identity := workspaceIdentity(commonDir, runID)
	treesRoot := filepath.Join(base, identity, "trees")
	fencesRoot := filepath.Join(base, identity, "fences")
	entries, err := os.ReadDir(treesRoot)
	if err != nil {
		return nil, nil
	}
	var fenced []WorkspaceFence
	for _, entry := range entries {
		if !entry.IsDir() || !workspaceTokenPattern.MatchString(entry.Name()) {
			continue
		}
		treePath := filepath.Join(treesRoot, entry.Name())
		fencePath := filepath.Join(treePath, WorkspaceFenceFile)
		auxPath := filepath.Join(fencesRoot, entry.Name())
		var fence WorkspaceFence
		found := false
		if data, err := os.ReadFile(fencePath); err == nil {
			if err := json.Unmarshal(data, &fence); err == nil &&
				fence.Token == entry.Name() && fence.Identity == identity {
				found = true
			}
		}
		if !found {
			if data, err := os.ReadFile(auxPath); err == nil {
				if err := json.Unmarshal(data, &fence); err == nil &&
					fence.Token == entry.Name() && fence.Identity == identity {
					found = true
				}
			}
		}
		if !found {
			_, statPrimary := os.Lstat(fencePath)
			_, statAux := os.Lstat(auxPath)
			if statPrimary == nil || statAux == nil {
				fence = WorkspaceFence{
					Token:     entry.Name(),
					Identity:  identity,
					Reason:    "WORKSPACE_FENCE_CORRUPT",
					Detail:    "corrupt or truncated fence record",
					CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
				}
				found = true
			}
		}
		if found {
			if fence.Path == "" {
				fence.Path = treePath
			}
			fenced = append(fenced, fence)
		}
	}
	return fenced, nil
}

// CaptureCheckpoint inspects the workspace, validates scope and entry types, stages changes,
// writes a checkpoint tree/commit, and records a reachability ref under refs/heads/checkpoints/...
func (w *Workspaces) CaptureCheckpoint(
	lease *WorkspaceLease,
	attempt CheckpointAttempt,
	slice string,
	scope CheckpointScope,
	existingStagedBytes int64,
	admission *ProductExclusionAdmission,
) (result CheckpointResult, err error) {
	if w == nil || lease == nil || lease.closed ||
		lease.access != WorkspaceReadWrite || lease.view != ImplementationView {
		return CheckpointResult{}, fail("INVALID_WORKSPACE_LEASE", "capture checkpoint", nil)
	}
	if err := validateTrackKey(lease.key); err != nil {
		return CheckpointResult{}, err
	}

	// Shelter tmp/ scratch before staging.
	// If capture fails and the workspace is fenced, restore tmp/ so diagnostics are preserved.
	tmpPath := filepath.Join(lease.path, "tmp")
	var scratchShelter string
	if _, statErr := os.Lstat(tmpPath); statErr == nil {
		tempRoot, tempErr := ResolveTempRoot()
		if tempErr == nil {
			shelter, err := os.MkdirTemp(tempRoot, "sworn-checkpoint-scratch-")
			if err == nil {
				shelterTmp := filepath.Join(shelter, "tmp")
				if renameErr := os.Rename(tmpPath, shelterTmp); renameErr == nil {
					scratchShelter = shelter
				} else {
					_ = os.RemoveAll(shelter)
				}
			}
		}
	}
	defer func() {
		if scratchShelter != "" {
			if lease.IsFenced() {
				// Restore scratch so the quarantined workspace retains its diagnostics.
				sheltered := filepath.Join(scratchShelter, "tmp")
				_ = os.Rename(sheltered, tmpPath)
			}
			_ = os.RemoveAll(scratchShelter)
		}
	}()

	// Stage all current workspace files.
	if _, err := w.repository.runAt(
		lease.path,
		nil,
		nil,
		"add",
		"--all",
		"--",
		".",
	); err != nil {
		if isENOSPC(err) {
			_ = lease.Fence("CHECKPOINT_ENOSPC", "git add failed with ENOSPC", slice)
			return CheckpointResult{}, fail("CHECKPOINT_ENOSPC", "git add failed", err)
		}
		_ = lease.Fence("CHECKPOINT_CORRUPT", "git add failed", slice)
		return CheckpointResult{}, fail("CHECKPOINT_CORRUPT", "git add failed", err)
	}

	// Inventory changed paths relative to lease head.
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
		_ = lease.Fence("CHECKPOINT_CORRUPT", "git diff --cached failed", slice)
		return CheckpointResult{}, fail("CHECKPOINT_CORRUPT", "diff cached failed", err)
	}

	var workspaceChanged []string
	var authorityPaths []string
	for _, raw := range bytes.Split(rawPaths, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		name := string(raw)
		if err := ValidatePath(name, false); err != nil {
			_ = lease.Fence("CHECKPOINT_UNSUPPORTED_ENTRY", "invalid path: "+name, slice)
			return CheckpointResult{}, err
		}
		if name == w.repository.recordRoot || strings.HasPrefix(name, w.repository.recordRoot+"/") ||
			name == w.repository.LegacyRecordRoot() || strings.HasPrefix(name, w.repository.LegacyRecordRoot()+"/") {
			authorityPaths = append(authorityPaths, name)
		}
		workspaceChanged = append(workspaceChanged, name)
	}

	if len(authorityPaths) > 0 {
		sort.Strings(authorityPaths)
		// Refuse authority modification.
		_ = lease.Fence("CHECKPOINT_SCOPE_VIOLATION", "authority path modified", slice)
		return CheckpointResult{}, fail("CHECKPOINT_SCOPE_VIOLATION", "authority paths modified", nil)
	}

	// Empty diff case: no failure, no fence, no checkpoint.
	if len(workspaceChanged) == 0 {
		return CheckpointResult{Empty: true}, nil
	}

	// Inspect each changed path for unsupported entries, symlink escapes, and scope.
	for _, name := range workspaceChanged {
		filePath := filepath.Join(lease.path, name)
		lstat, statErr := os.Lstat(filePath)
		if statErr == nil {
			if lstat.Mode()&fs.ModeSymlink != 0 {
				linkTarget, readErr := os.Readlink(filePath)
				if readErr != nil {
					_ = lease.Fence("CHECKPOINT_UNSUPPORTED_ENTRY", "readlink failed: "+name, slice)
					return CheckpointResult{}, fail("CHECKPOINT_UNSUPPORTED_ENTRY", "readlink failed", readErr)
				}
				resolved := linkTarget
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(filepath.Dir(filePath), linkTarget)
				}
				rel, relErr := filepath.Rel(lease.path, resolved)
				if relErr != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
					_ = lease.Fence("CHECKPOINT_UNSUPPORTED_ENTRY", "symlink escapes workspace: "+name, slice)
					return CheckpointResult{}, fail("CHECKPOINT_UNSUPPORTED_ENTRY", "escaping symlink", nil)
				}
			}
			mode := lstat.Mode()
			if mode&(fs.ModeNamedPipe|fs.ModeSocket|fs.ModeDevice|fs.ModeCharDevice) != 0 {
				_ = lease.Fence("CHECKPOINT_UNSUPPORTED_ENTRY", "unsupported device or fifo: "+name, slice)
				return CheckpointResult{}, fail("CHECKPOINT_UNSUPPORTED_ENTRY", "unsupported filesystem entry", nil)
			}
		}

		// Scope check: scope violations fence to retain work and prevent overwriting progress.
		if !pathInCheckpointScope(scope, name) {
			_ = lease.Fence("CHECKPOINT_SCOPE_VIOLATION", "path out of scope: "+name, slice)
			return CheckpointResult{}, fail("CHECKPOINT_SCOPE_VIOLATION", "path out of scope: "+name, nil)
		}
	}

	// Check file count bound.
	fileCount := len(workspaceChanged)
	if fileCount > MaxCheckpointFiles {
		_ = lease.Fence(
			"CHECKPOINT_TOO_MANY_FILES",
			fmt.Sprintf("file count %d exceeds limit %d", fileCount, MaxCheckpointFiles),
			slice,
		)
		return CheckpointResult{}, fail(
			"CHECKPOINT_TOO_MANY_FILES",
			fmt.Sprintf("checkpoint file count %d exceeds %d", fileCount, MaxCheckpointFiles),
			nil,
		)
	}

	// Calculate staged bytes accounting.
	var stagedBytes int64
	for _, name := range workspaceChanged {
		filePath := filepath.Join(lease.path, name)
		if lstat, statErr := os.Lstat(filePath); statErr == nil && !lstat.IsDir() {
			stagedBytes += lstat.Size()
		}
	}

	// Check byte bound.
	if stagedBytes > MaxCheckpointBytes {
		_ = lease.Fence(
			"CHECKPOINT_OVERSIZE",
			fmt.Sprintf("checkpoint bytes %d exceeds limit %d", stagedBytes, MaxCheckpointBytes),
			slice,
		)
		return CheckpointResult{}, fail(
			"CHECKPOINT_OVERSIZE",
			fmt.Sprintf("checkpoint size %d exceeds limit %d", stagedBytes, MaxCheckpointBytes),
			nil,
		)
	}

	// Aggregate capacity check (256 MiB aggregate cap).
	if existingStagedBytes+stagedBytes > MaxAggregateCheckpointBytes {
		_ = lease.Fence(
			"CHECKPOINT_CAPACITY_EXCEEDED",
			fmt.Sprintf("aggregate checkpoint bytes %d exceeds cap %d", existingStagedBytes+stagedBytes, MaxAggregateCheckpointBytes),
			slice,
		)
		return CheckpointResult{}, fail(
			"CHECKPOINT_CAPACITY_EXCEEDED",
			fmt.Sprintf("aggregate checkpoint bytes %d exceeds cap %d", existingStagedBytes+stagedBytes, MaxAggregateCheckpointBytes),
			nil,
		)
	}

	// Write tree.
	rawTree, writeErr := w.repository.runAt(
		lease.path,
		nil,
		nil,
		"write-tree",
	)
	if writeErr != nil {
		if isENOSPC(writeErr) {
			_ = lease.Fence("CHECKPOINT_ENOSPC", "write-tree failed with ENOSPC", slice)
			return CheckpointResult{}, fail("CHECKPOINT_ENOSPC", "write-tree failed with ENOSPC", writeErr)
		}
		_ = lease.Fence("CHECKPOINT_CORRUPT", "write-tree failed", slice)
		return CheckpointResult{}, fail("CHECKPOINT_CORRUPT", "write-tree failed", writeErr)
	}

	tree, err := w.repository.parseOID(string(rawTree))
	if err != nil {
		_ = lease.Fence("CHECKPOINT_CORRUPT", "parse tree failed", slice)
		return CheckpointResult{}, err
	}

	// The written tree has no ref yet and is unreachable from any commit: an
	// interrupt here leaves an object in the odb that nothing reports as a
	// completed checkpoint, and restart safely re-stages and re-binds.
	if testCrashAfterEffect == "checkpoint.prepare" {
		os.Exit(86)
	}

	timestamp, timestampErr := w.repository.CommitTimestamp(lease.head)
	if timestampErr != nil {
		timestamp = time.Now().Unix()
	}

	commitMessage := fmt.Sprintf(
		"sworn(checkpoint): %s/%s attempt %s-%d-%d\n",
		lease.key.Release,
		lease.key.Track,
		attempt.WorkID,
		attempt.Epoch,
		attempt.Try,
	)

	identity := w.commitIdentity
	rawCommit, commitErr := w.repository.run(
		[]byte(commitMessage),
		commitEnvironment(identity, timestamp+1),
		"commit-tree",
		tree.String(),
		"-p",
		lease.head.String(),
	)
	if commitErr != nil {
		if isENOSPC(commitErr) {
			_ = lease.Fence("CHECKPOINT_ENOSPC", "commit-tree failed with ENOSPC", slice)
			return CheckpointResult{}, fail("CHECKPOINT_ENOSPC", "commit-tree failed with ENOSPC", commitErr)
		}
		_ = lease.Fence("CHECKPOINT_CORRUPT", "commit-tree failed", slice)
		return CheckpointResult{}, fail("CHECKPOINT_CORRUPT", "commit-tree failed", commitErr)
	}

	commitOID, err := w.repository.parseOID(string(rawCommit))
	if err != nil {
		_ = lease.Fence("CHECKPOINT_CORRUPT", "parse commit failed", slice)
		return CheckpointResult{}, err
	}

	// Update ref under refs/heads/checkpoints/<release>/<track>/<work-id>-<epoch>-<try>.
	workName := strings.ReplaceAll(attempt.WorkID, ":", "-")
	refName := fmt.Sprintf(
		"%s%s/%s/%s-%d-%d",
		CheckpointRefPrefix,
		lease.key.Release,
		lease.key.Track,
		workName,
		attempt.Epoch,
		attempt.Try,
	)
	if err := ValidateHeadRef(refName); err != nil {
		_ = lease.Fence("CHECKPOINT_CORRUPT", "invalid ref name: "+refName, slice)
		return CheckpointResult{}, err
	}

	if _, err := w.repository.run(
		nil,
		nil,
		"update-ref",
		refName,
		commitOID.String(),
	); err != nil {
		_ = lease.Fence("CHECKPOINT_CORRUPT", "update-ref failed", slice)
		return CheckpointResult{}, fail("CHECKPOINT_CORRUPT", "update-ref failed", err)
	}

	productTree := ""
	if admission != nil {
		productIdentity, err := w.repository.ProductTreeIdentity(commitOID, admission)
		if err == nil {
			productTree = productIdentity.ProductTree
		}
	}
	if productTree == "" {
		entries, err := w.repository.ListTree(commitOID)
		if err != nil {
			_ = lease.Fence("CHECKPOINT_CORRUPT", "failed to compute product tree identity: "+err.Error(), slice)
			return CheckpointResult{}, fail("CHECKPOINT_CORRUPT", "product tree identity failed", err)
		}
		hasher := sha256.New()
		for _, entry := range entries {
			if w.repository.isReservedRecordPath(entry.Path) {
				continue
			}
			io.WriteString(hasher, entry.Path)
			hasher.Write([]byte{0})
			io.WriteString(hasher, entry.Mode)
			hasher.Write([]byte{0})
			io.WriteString(hasher, entry.Type)
			hasher.Write([]byte{0})
			io.WriteString(hasher, entry.OID.String())
			hasher.Write([]byte{'\n'})
		}
		productTree = "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	}

	sort.Strings(workspaceChanged)
	return CheckpointResult{
		Empty:        false,
		Ref:          refName,
		Commit:       commitOID,
		Tree:         tree,
		ProductTree:  productTree,
		StagedBytes:  stagedBytes,
		FileCount:    fileCount,
		ChangedPaths: workspaceChanged,
		Timestamp:    timestamp + 1,
	}, nil
}

func parseCheckpointRefAttempt(ref string) (int64, int64, bool) {
	lastSlash := strings.LastIndexByte(ref, '/')
	base := ref
	if lastSlash >= 0 {
		base = ref[lastSlash+1:]
	}
	parts := strings.Split(base, "-")
	if len(parts) < 3 {
		return 0, 0, false
	}
	epoch, err1 := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	try, err2 := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return epoch, try, true
}

// PruneCheckpointGenerations removes older checkpoint refs for workID on key,
// retaining up to maxGenerations newest generations. It sorts by numeric attempt
// coordinates and never evicts keepRef or the sole copy of work.
func (w *Workspaces) PruneCheckpointGenerations(
	key TrackKey,
	workID string,
	maxGenerations int,
	keepRef string,
) ([]string, error) {
	if w == nil || w.repository == nil {
		return nil, fail("INVALID_WORKSPACE_LEASE", "prune checkpoints", nil)
	}
	if maxGenerations <= 0 {
		maxGenerations = MaxCheckpointGenerations
	}
	workName := strings.ReplaceAll(workID, ":", "-")
	trackCheckpointPrefix := fmt.Sprintf(
		"%s%s/%s/",
		CheckpointRefPrefix,
		key.Release,
		key.Track,
	)
	workRefPrefix := trackCheckpointPrefix + workName + "-"
	existingRefs, err := w.repository.ListHeadRefsUnder(trackCheckpointPrefix)
	if err != nil {
		return nil, err
	}
	var workRefs []RefHead
	for _, r := range existingRefs {
		if strings.HasPrefix(r.Ref, workRefPrefix) {
			workRefs = append(workRefs, r)
		}
	}
	if len(workRefs) <= maxGenerations {
		return nil, nil
	}

	// Sort numerically by attempt coordinates (epoch, try) ascending (oldest first).
	sort.Slice(workRefs, func(i, j int) bool {
		epochI, tryI, okI := parseCheckpointRefAttempt(workRefs[i].Ref)
		epochJ, tryJ, okJ := parseCheckpointRefAttempt(workRefs[j].Ref)
		if okI && okJ {
			if epochI != epochJ {
				return epochI < epochJ
			}
			return tryI < tryJ
		}
		return workRefs[i].Ref < workRefs[j].Ref
	})

	var pruned []string
	excess := len(workRefs) - maxGenerations
	for i := 0; i < len(workRefs) && excess > 0; i++ {
		candidate := workRefs[i].Ref
		// Never evict keepRef or the newest generation
		if candidate == keepRef || i == len(workRefs)-1 {
			continue
		}
		if _, err := w.repository.run(nil, nil, "update-ref", "-d", candidate); err == nil {
			pruned = append(pruned, candidate)
			excess--
		}
	}
	return pruned, nil
}

// RestoreCheckpoint materializes the given tree OID into the writable workspace.
func (w *Workspaces) RestoreCheckpoint(lease *WorkspaceLease, tree OID) error {
	if w == nil || lease == nil || lease.closed || lease.access != WorkspaceReadWrite {
		return fail("INVALID_WORKSPACE_LEASE", "restore checkpoint", nil)
	}
	if err := w.repository.validateOID(tree); err != nil {
		return err
	}
	if _, err := w.repository.runAt(
		lease.path,
		nil,
		nil,
		"read-tree",
		"-u",
		"--reset",
		tree.String(),
	); err != nil {
		return fail("CHECKPOINT_RESTORE_FAILED", "restore checkpoint tree", err)
	}
	return nil
}

func isENOSPC(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no space left on device") || strings.Contains(msg, "enospc")
}
