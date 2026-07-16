package baton

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	vendorRecoveryRelativeRoot = "sworn/recovery/baton-vendor"
	vendorRecoverySentinelName = "rollback-incomplete.json"
	vendorRecoveryManifestName = "manifest.json"
)

type vendorRepository struct {
	root         string
	gitAdmin     string
	recoveryRoot string
}

type vendorError struct {
	class       string
	phase       string
	destination string
	recovery    string
	detail      string
	cause       error
}

func newVendorErrorWithDetail(class, phase, destination, recovery, detail string, cause error) error {
	err := newVendorError(class, phase, destination, recovery, cause).(*vendorError)
	err.detail = detail
	return err
}

func newVendorError(class, phase, destination, recovery string, cause error) error {
	return &vendorError{
		class:       class,
		phase:       phase,
		destination: filepath.ToSlash(destination),
		recovery:    recovery,
		cause:       cause,
	}
}

func (e *vendorError) Error() string {
	parts := []string{"baton vendor", "phase=" + e.phase, "class=" + e.class}
	if e.destination != "" {
		parts = append(parts, "destination="+e.destination)
	}
	if e.recovery != "" {
		parts = append(parts, "recovery="+e.recovery)
	}
	if e.detail != "" {
		parts = append(parts, "detail="+e.detail)
	}
	return strings.Join(parts, ": ")
}

func (e *vendorError) Unwrap() error { return e.cause }

type vendorFileOps struct {
	replace func(vendorRepository, string, []byte, os.FileMode) error
	restore func(vendorRepository, vendorCandidate) error
}

func defaultVendorFileOps() *vendorFileOps {
	return &vendorFileOps{
		replace: atomicReplaceVendorFile,
		restore: restoreVendorOriginal,
	}
}

type vendorRecovery struct {
	root           string
	sentinelPath   string
	transactionDir string
	manifest       vendorRecoveryManifest
	snapshots      map[string][]byte
}

type vendorRecoveryManifest struct {
	RecordVersion     int                         `json:"record_version"`
	RepositoryRoot    string                      `json:"repository_root"`
	GitAdminDirectory string                      `json:"git_admin_directory"`
	Destinations      []vendorRecoveryDestination `json:"destinations"`
}

type vendorRecoveryDestination struct {
	Path           string `json:"path"`
	OriginalExists bool   `json:"original_exists"`
	OriginalMode   string `json:"original_mode"`
	OriginalSHA256 string `json:"original_sha256"`
	SnapshotPath   string `json:"snapshot_path"`
}

type vendorRecoverySentinel struct {
	RecordVersion     int    `json:"record_version"`
	TransactionSHA256 string `json:"transaction_sha256"`
	RecoveryDirectory string `json:"recovery_directory"`
}

func loadVendorRecovery(repoRoot string) (vendorRepository, *vendorRecovery, error) {
	repo, err := resolveVendorRepository(repoRoot)
	if err != nil {
		return vendorRepository{}, nil, err
	}

	recoveryExists, err := validateVendorRecoveryPath(repo)
	if err != nil {
		return repo, nil, err
	}
	if !recoveryExists {
		cleaned, cleanupErr := cleanupVendorRecoveryDebris(repo)
		if cleanupErr != nil {
			return repo, nil, cleanupErr
		}
		if cleaned {
			return repo, nil, newVendorErrorWithDetail("recovery-debris-cleaned-rerun-required", "cleanup", "", repo.recoveryRoot, "stale transaction material removed; rerun vendor command", fmt.Errorf("stale transaction material removed"))
		}
		return repo, nil, nil
	}
	rootEntries, err := readVendorDirectoryEntries(repo, repo.recoveryRoot, 0o700)
	if err != nil {
		return repo, nil, newVendorError("recovery-unreadable", "recovery", "", repo.recoveryRoot, err)
	}
	if len(rootEntries) == 0 {
		return repo, nil, newVendorError("recovery-foreign-material", "recovery", "", repo.recoveryRoot, fmt.Errorf("recovery root is missing its sentinel and transaction"))
	}

	if vendorEntriesContain(rootEntries, vendorRecoverySentinelName) {
		return loadVendorRecoveryAuthority(repo, rootEntries)
	}
	return repo, nil, newVendorError("recovery-foreign-material", "recovery", "", repo.recoveryRoot, fmt.Errorf("recovery root has no recognised transaction authority"))
}

func loadVendorRecoveryAuthority(repo vendorRepository, rootEntries []os.DirEntry) (vendorRepository, *vendorRecovery, error) {
	sentinelPath := filepath.Join(repo.recoveryRoot, vendorRecoverySentinelName)
	sentinelBytes, err := readVendorControlFile(repo, sentinelPath, 0o600)
	if err != nil {
		return repo, nil, err
	}
	var sentinel vendorRecoverySentinel
	if err := decodeExactVendorJSON(sentinelBytes, &sentinel); err != nil {
		return repo, nil, newVendorError("recovery-sentinel-invalid", "recovery", "", sentinelPath, err)
	}
	canonicalSentinel, err := marshalVendorControlJSON(sentinel)
	if err != nil || !bytes.Equal(canonicalSentinel, sentinelBytes) {
		return repo, nil, newVendorError("recovery-sentinel-noncanonical", "recovery", "", sentinelPath, fmt.Errorf("sentinel bytes are not canonical"))
	}
	if sentinel.RecordVersion != 1 {
		return repo, nil, newVendorError("recovery-sentinel-invalid", "recovery", "", sentinelPath, fmt.Errorf("unsupported sentinel record version"))
	}
	transactionID, err := parseVendorDigest(sentinel.TransactionSHA256)
	if err != nil {
		return repo, nil, newVendorError("recovery-sentinel-invalid", "recovery", "", sentinelPath, err)
	}
	transactionDir := filepath.Join(repo.recoveryRoot, transactionID)
	if sentinel.RecoveryDirectory != transactionDir {
		return repo, nil, newVendorError("recovery-directory-mismatch", "recovery", "", sentinel.RecoveryDirectory, fmt.Errorf("sentinel does not name the current transaction directory"))
	}

	if len(rootEntries) != 2 || !vendorEntryNamesEqual(rootEntries, []string{vendorRecoverySentinelName, transactionID}) {
		return repo, nil, newVendorError("recovery-foreign-material", "recovery", "", repo.recoveryRoot, fmt.Errorf("recovery root must contain only the sentinel and named transaction"))
	}

	manifestPath := filepath.Join(transactionDir, vendorRecoveryManifestName)
	manifestBytes, err := readVendorControlFile(repo, manifestPath, 0o600)
	if err != nil {
		return repo, nil, err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	if hex.EncodeToString(manifestDigest[:]) != transactionID {
		return repo, nil, newVendorError("recovery-manifest-digest-mismatch", "recovery", "", manifestPath, fmt.Errorf("manifest digest does not match transaction identity"))
	}
	var manifest vendorRecoveryManifest
	if err := decodeExactVendorJSON(manifestBytes, &manifest); err != nil {
		return repo, nil, newVendorError("recovery-manifest-invalid", "recovery", "", manifestPath, err)
	}
	canonicalManifest, err := marshalVendorControlJSON(manifest)
	if err != nil || !bytes.Equal(canonicalManifest, manifestBytes) {
		return repo, nil, newVendorError("recovery-manifest-noncanonical", "recovery", "", manifestPath, fmt.Errorf("manifest bytes are not canonical"))
	}
	if manifest.RecordVersion != 1 || manifest.RepositoryRoot != repo.root || manifest.GitAdminDirectory != repo.gitAdmin {
		return repo, nil, newVendorError("recovery-identity-mismatch", "recovery", "", manifestPath, fmt.Errorf("repository or Git-admin identity does not match this invocation"))
	}

	snapshots, err := validateVendorRecoveryMaterial(repo, transactionDir, manifest)
	if err != nil {
		return repo, nil, err
	}
	return repo, &vendorRecovery{
		root:           repo.recoveryRoot,
		sentinelPath:   sentinelPath,
		transactionDir: transactionDir,
		manifest:       manifest,
		snapshots:      snapshots,
	}, nil
}

func resolveVendorRepository(repoRoot string) (vendorRepository, error) {
	repo, err := resolveVendorRepositoryRoot(repoRoot)
	if err != nil {
		return vendorRepository{}, err
	}
	physicalRoot := repo.root

	dotGit := filepath.Join(physicalRoot, ".git")
	dotGitInfo, err := os.Lstat(dotGit)
	if err != nil {
		return vendorRepository{}, newVendorError("git-admin-unavailable", "preflight", "", "", err)
	}
	if dotGitInfo.Mode()&os.ModeSymlink != 0 {
		return vendorRepository{}, newVendorError("git-admin-symlink", "preflight", "", "", fmt.Errorf(".git must not be a symlink"))
	}
	gitAdmin := dotGit
	switch {
	case dotGitInfo.IsDir():
	case dotGitInfo.Mode().IsRegular():
		data, err := os.ReadFile(dotGit)
		if err != nil {
			return vendorRepository{}, newVendorError("git-admin-unavailable", "preflight", "", "", err)
		}
		line := strings.TrimSpace(string(data))
		value, ok := strings.CutPrefix(line, "gitdir:")
		if !ok || strings.TrimSpace(value) == "" || strings.ContainsRune(value, '\x00') {
			return vendorRepository{}, newVendorError("git-admin-invalid", "preflight", "", "", fmt.Errorf("invalid .git file"))
		}
		gitAdmin = strings.TrimSpace(value)
		if !filepath.IsAbs(gitAdmin) {
			gitAdmin = filepath.Join(physicalRoot, gitAdmin)
		}
	default:
		return vendorRepository{}, newVendorError("git-admin-invalid", "preflight", "", "", fmt.Errorf(".git is neither a directory nor a gitdir file"))
	}
	gitAdmin, err = filepath.EvalSymlinks(gitAdmin)
	if err != nil {
		return vendorRepository{}, newVendorError("git-admin-unavailable", "preflight", "", "", err)
	}
	gitInfo, err := os.Stat(gitAdmin)
	if err != nil || !gitInfo.IsDir() {
		if err == nil {
			err = fmt.Errorf("Git administrative path is not a directory")
		}
		return vendorRepository{}, newVendorError("git-admin-invalid", "preflight", "", "", err)
	}

	repo.gitAdmin = filepath.Clean(gitAdmin)
	repo.recoveryRoot = filepath.Join(repo.gitAdmin, filepath.FromSlash(vendorRecoveryRelativeRoot))
	return repo, nil
}

func resolveVendorRepositoryRoot(repoRoot string) (vendorRepository, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return vendorRepository{}, newVendorError("repository-invalid", "preflight", "", "", err)
	}
	physicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return vendorRepository{}, newVendorError("repository-invalid", "preflight", "", "", err)
	}
	rootInfo, err := os.Stat(physicalRoot)
	if err != nil || !rootInfo.IsDir() {
		if err == nil {
			err = fmt.Errorf("repository root is not a directory")
		}
		return vendorRepository{}, newVendorError("repository-invalid", "preflight", "", "", err)
	}

	return vendorRepository{
		root: filepath.Clean(physicalRoot),
	}, nil
}

// validateVendorRecoveryPath walks every component below the already-physical
// current-worktree Git administrative directory without following symlinks.
// It returns false at the first absent component; a partially existing safe
// prefix is valid and may later be completed by recovery publication.
func validateVendorRecoveryPath(repo vendorRepository) (bool, error) {
	root, err := os.OpenRoot(repo.gitAdmin)
	if err != nil {
		return false, newVendorError("git-admin-unavailable", "recovery", "", repo.gitAdmin, err)
	}
	defer root.Close()
	currentRel := ""
	for _, segment := range strings.Split(vendorRecoveryRelativeRoot, "/") {
		currentRel = filepath.Join(currentRel, segment)
		current := filepath.Join(repo.gitAdmin, currentRel)
		info, err := root.Lstat(currentRel)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, newVendorError("recovery-path-unreadable", "recovery", "", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, newVendorError("recovery-path-invalid", "recovery", "", current, fmt.Errorf("recovery path component is not a real directory"))
		}
	}
	return true, nil
}

func ensureVendorRecoveryParentPath(repo vendorRepository) error {
	root, err := os.OpenRoot(repo.gitAdmin)
	if err != nil {
		return newVendorError("git-admin-unavailable", "preflight", "", repo.gitAdmin, err)
	}
	defer root.Close()
	currentRel := ""
	segments := strings.Split(vendorRecoveryRelativeRoot, "/")
	for _, segment := range segments[:len(segments)-1] {
		parentRel := currentRel
		currentRel = filepath.Join(currentRel, segment)
		current := filepath.Join(repo.gitAdmin, currentRel)
		info, err := root.Lstat(currentRel)
		if os.IsNotExist(err) {
			if err := root.Mkdir(currentRel, 0o700); err != nil {
				return newVendorError("recovery-path-create-failed", "preflight", "", current, err)
			}
			if err := root.Chmod(currentRel, 0o700); err != nil {
				return newVendorError("recovery-path-mode-failed", "preflight", "", current, err)
			}
			if err := syncVendorRootDirectory(root, parentRel); err != nil {
				return newVendorError("recovery-path-sync-failed", "preflight", "", filepath.Join(repo.gitAdmin, parentRel), err)
			}
			continue
		}
		if err != nil {
			return newVendorError("recovery-path-unreadable", "preflight", "", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return newVendorError("recovery-path-invalid", "preflight", "", current, fmt.Errorf("recovery path component is not a real directory"))
		}
	}
	return nil
}

func cleanupVendorRecoveryDebris(repo vendorRepository) (bool, error) {
	recoveryParent := filepath.Dir(repo.recoveryRoot)
	parentInfo, err := os.Lstat(recoveryParent)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, newVendorError("recovery-path-unreadable", "cleanup", "", repo.recoveryRoot, err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return false, newVendorError("recovery-path-invalid", "cleanup", "", repo.recoveryRoot, fmt.Errorf("recovery parent is not a real directory"))
	}
	root, err := os.OpenRoot(repo.gitAdmin)
	if err != nil {
		return false, newVendorError("git-admin-unavailable", "cleanup", "", repo.recoveryRoot, err)
	}
	defer root.Close()
	parentRel, err := filepath.Rel(repo.gitAdmin, recoveryParent)
	if err != nil {
		return false, newVendorError("recovery-path-invalid", "cleanup", "", repo.recoveryRoot, err)
	}
	directory, err := root.Open(parentRel)
	if err != nil {
		return false, newVendorError("recovery-path-unreadable", "cleanup", "", repo.recoveryRoot, err)
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return false, newVendorError("recovery-path-unreadable", "cleanup", "", repo.recoveryRoot, readErr)
	}
	if closeErr != nil {
		return false, newVendorError("recovery-path-unreadable", "cleanup", "", repo.recoveryRoot, closeErr)
	}

	cleaned := false
	for _, entry := range entries {
		if !isVendorRecoveryDebrisName(entry.Name()) {
			continue
		}
		entryRel := filepath.Join(parentRel, entry.Name())
		info, err := root.Lstat(entryRel)
		if err != nil {
			return cleaned, newVendorError("recovery-debris-unreadable", "cleanup", "", repo.recoveryRoot, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 {
			return cleaned, newVendorError("recovery-debris-invalid", "cleanup", "", repo.recoveryRoot, fmt.Errorf("stale transaction material has forbidden type or mode"))
		}
		if err := root.RemoveAll(entryRel); err != nil {
			return cleaned, newVendorError("recovery-debris-cleanup-failed", "cleanup", "", repo.recoveryRoot, err)
		}
		cleaned = true
	}
	if cleaned {
		if err := syncVendorRootDirectory(root, parentRel); err != nil {
			return true, newVendorError("recovery-debris-cleanup-failed", "cleanup", "", repo.recoveryRoot, err)
		}
	}
	return cleaned, nil
}

func isVendorRecoveryDebrisName(name string) bool {
	for _, prefix := range []string{".baton-vendor-staging-", ".baton-vendor-retired-"} {
		suffix, ok := strings.CutPrefix(name, prefix)
		if !ok || len(suffix) != 32 || strings.ToLower(suffix) != suffix {
			continue
		}
		decoded, err := hex.DecodeString(suffix)
		if err == nil && len(decoded) == 16 {
			return true
		}
	}
	return false
}

func validateVendorRelativePath(rel string) error {
	if rel == "" || !utf8.ValidString(rel) || strings.ContainsRune(rel, '\x00') || strings.Contains(rel, `\`) {
		return fmt.Errorf("destination path is empty, invalid UTF-8, or contains a forbidden byte")
	}
	if strings.HasPrefix(rel, "/") || strings.HasSuffix(rel, "/") || path.Clean(rel) != rel {
		return fmt.Errorf("destination path is not canonical repository-relative UTF-8")
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("destination path contains a forbidden segment")
		}
	}
	return nil
}

func vendorDestinationPath(repo vendorRepository, rel string) (string, error) {
	if err := validateVendorRelativePath(rel); err != nil {
		return "", err
	}
	abs := filepath.Join(repo.root, filepath.FromSlash(rel))
	relCheck, err := filepath.Rel(repo.root, abs)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("destination escapes repository")
	}
	return abs, nil
}

func snapshotVendorOriginal(repo vendorRepository, rel string) (vendorOriginal, error) {
	abs, err := vendorDestinationPath(repo, rel)
	if err != nil {
		return vendorOriginal{}, newVendorError("destination-invalid", "preflight", rel, "", err)
	}
	missingParents, err := inspectVendorParents(repo, rel)
	if err != nil {
		return vendorOriginal{}, err
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return vendorOriginal{missingParents: missingParents}, nil
	}
	if err != nil {
		return vendorOriginal{}, newVendorError("destination-unreadable", "preflight", rel, "", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return vendorOriginal{}, newVendorError("destination-non-regular", "preflight", rel, "", fmt.Errorf("destination must be a regular file or absent"))
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return vendorOriginal{}, newVendorError("destination-unreadable", "preflight", rel, "", err)
	}
	after, err := os.Lstat(abs)
	if err != nil || !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = fmt.Errorf("destination type changed while reading")
		}
		return vendorOriginal{}, newVendorError("destination-unstable", "preflight", rel, "", err)
	}
	return vendorOriginal{
		exists:         true,
		mode:           after.Mode().Perm(),
		bytes:          data,
		missingParents: missingParents,
	}, nil
}

func inspectVendorParents(repo vendorRepository, rel string) ([]string, error) {
	parts := strings.Split(rel, "/")
	missing := make([]string, 0)
	missingSeen := false
	for i := 1; i < len(parts); i++ {
		parentRel := strings.Join(parts[:i], "/")
		parentAbs := filepath.Join(repo.root, filepath.FromSlash(parentRel))
		if missingSeen {
			missing = append(missing, parentRel)
			continue
		}
		info, err := os.Lstat(parentAbs)
		if os.IsNotExist(err) {
			missingSeen = true
			missing = append(missing, parentRel)
			continue
		}
		if err != nil {
			return nil, newVendorError("destination-parent-unreadable", "preflight", rel, "", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, newVendorError("destination-parent-invalid", "preflight", rel, "", fmt.Errorf("parent %s is not a real directory", parentRel))
		}
	}
	return missing, nil
}

func applyVendorTransaction(repo vendorRepository, plan *vendorPlan) error {
	ops := plan.fileOps
	if ops == nil {
		ops = defaultVendorFileOps()
	}
	if ops.replace == nil || ops.restore == nil {
		return newVendorError("transaction-seam-invalid", "apply", "", "", fmt.Errorf("replace and restore operations are required"))
	}
	recovery, err := publishVendorRecovery(repo, plan)
	if err != nil {
		return newVendorError("recovery-publication-failed", "preflight", "", repo.recoveryRoot, err)
	}

	touched := make([]int, 0, len(plan.changed))
	for _, candidateIndex := range plan.changed {
		candidate := plan.candidates[candidateIndex]
		// Reject a concurrent edit made after materialisation instead of
		// overwriting it with a stale snapshot/candidate pair.
		if err := verifyVendorOriginalFile(repo, candidate); err != nil {
			return rollbackVendorTransaction(repo, plan, ops, recovery, touched, candidate.path, fmt.Errorf("original changed before replacement: %w", err))
		}
		touched = append(touched, candidateIndex)
		if err := ops.replace(repo, candidate.path, candidate.desired, candidate.desiredMode); err != nil {
			return rollbackVendorTransaction(repo, plan, ops, recovery, touched, candidate.path, err)
		}
	}
	changedSet := make(map[int]struct{}, len(plan.changed))
	for _, candidateIndex := range plan.changed {
		changedSet[candidateIndex] = struct{}{}
	}
	for candidateIndex, candidate := range plan.candidates {
		var verifyErr error
		if _, changed := changedSet[candidateIndex]; changed {
			verifyErr = verifyVendorDesired(repo, candidate)
		} else {
			verifyErr = verifyVendorOriginalFile(repo, candidate)
		}
		if verifyErr != nil {
			return rollbackVendorTransaction(repo, plan, ops, recovery, touched, candidate.path, verifyErr)
		}
	}
	if err := retireAndCleanupVendorRecovery(repo, recovery); err != nil {
		return err
	}
	return nil
}

func rollbackVendorTransaction(repo vendorRepository, plan *vendorPlan, ops *vendorFileOps, recovery *vendorRecovery, touched []int, failedPath string, applyErr error) error {
	unrestored := make(map[string]struct{})
	for i := len(touched) - 1; i >= 0; i-- {
		candidate := plan.candidates[touched[i]]
		if err := ops.restore(repo, candidate); err != nil {
			unrestored[candidate.path] = struct{}{}
		}
	}
	for _, candidate := range plan.candidates {
		if err := verifyVendorOriginal(repo, candidate); err != nil {
			unrestored[candidate.path] = struct{}{}
		}
	}
	if len(unrestored) == 0 {
		if err := retireAndCleanupVendorRecovery(repo, recovery); err != nil {
			return err
		}
		return newVendorError("apply-failed", "apply", failedPath, "", applyErr)
	}

	paths := make([]string, 0, len(unrestored))
	for _, candidate := range plan.candidates {
		if _, failed := unrestored[candidate.path]; failed {
			paths = append(paths, candidate.path)
		}
	}
	cause := fmt.Errorf("%v; unrestored=%s", applyErr, strings.Join(paths, ","))
	return newVendorErrorWithDetail("rollback-incomplete", "rollback", failedPath, recovery.transactionDir, "unrestored="+strings.Join(paths, ","), cause)
}

func atomicReplaceVendorFile(repo vendorRepository, rel string, content []byte, mode os.FileMode) error {
	if _, err := vendorDestinationPath(repo, rel); err != nil {
		return err
	}
	root, err := os.OpenRoot(repo.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := ensureVendorParentsInRoot(root, rel); err != nil {
		return err
	}
	if info, err := root.Lstat(filepath.FromSlash(rel)); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("destination is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	tmpRel := path.Join(path.Dir(rel), ".sworn-baton-vendor-"+hex.EncodeToString(random))
	tmp, err := root.OpenFile(filepath.FromSlash(tmpRel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = root.Remove(filepath.FromSlash(tmpRel))
	}
	if err := tmp.Chmod(mode.Perm()); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = root.Remove(filepath.FromSlash(tmpRel))
		return err
	}
	// Recheck the parent chain at the last point before the descriptor-relative
	// rename. os.Root confines the rename beneath the physical repository even
	// if another process races a symlink into the path.
	if _, err := inspectVendorParents(repo, rel); err != nil {
		_ = root.Remove(filepath.FromSlash(tmpRel))
		return err
	}
	if err := root.Rename(filepath.FromSlash(tmpRel), filepath.FromSlash(rel)); err != nil {
		_ = root.Remove(filepath.FromSlash(tmpRel))
		return err
	}
	if err := syncVendorRootDirectory(root, path.Dir(rel)); err != nil {
		return err
	}
	return nil
}

func ensureVendorParentsInRoot(root *os.Root, rel string) error {
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		parentRel := strings.Join(parts[:i], "/")
		info, err := root.Lstat(filepath.FromSlash(parentRel))
		if os.IsNotExist(err) {
			if err := root.Mkdir(filepath.FromSlash(parentRel), 0o755); err != nil {
				return fmt.Errorf("create parent %s: %w", parentRel, err)
			}
			if err := syncVendorRootDirectory(root, path.Dir(parentRel)); err != nil {
				return fmt.Errorf("sync parent for %s: %w", parentRel, err)
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("parent %s is not a real directory", parentRel)
		}
	}
	return nil
}

func restoreVendorOriginal(repo vendorRepository, candidate vendorCandidate) error {
	if candidate.original.exists {
		return atomicReplaceVendorFile(repo, candidate.path, candidate.original.bytes, candidate.original.mode)
	}
	if _, err := vendorDestinationPath(repo, candidate.path); err != nil {
		return err
	}
	if _, err := inspectVendorParents(repo, candidate.path); err != nil {
		return err
	}
	root, err := os.OpenRoot(repo.root)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(filepath.FromSlash(candidate.path))
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to remove non-regular destination")
		}
		if err := root.Remove(filepath.FromSlash(candidate.path)); err != nil {
			return err
		}
		if err := syncVendorRootDirectory(root, path.Dir(candidate.path)); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for i := len(candidate.original.missingParents) - 1; i >= 0; i-- {
		missingParent := candidate.original.missingParents[i]
		if err := root.Remove(filepath.FromSlash(missingParent)); err != nil && !os.IsNotExist(err) {
			// A non-empty directory is safe to retain; it may now contain a
			// different transaction member restored later in reverse order.
			if !isDirectoryNotEmpty(err) {
				return err
			}
		} else if err == nil {
			if err := syncVendorRootDirectory(root, path.Dir(missingParent)); err != nil {
				return err
			}
		}
	}
	return nil
}

func isDirectoryNotEmpty(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "directory not empty") || strings.Contains(strings.ToLower(err.Error()), "not empty")
}

func verifyVendorDesired(repo vendorRepository, candidate vendorCandidate) error {
	abs, err := vendorDestinationPath(repo, candidate.path)
	if err != nil {
		return err
	}
	if _, err := inspectVendorParents(repo, candidate.path); err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = fmt.Errorf("destination is not a regular file")
		}
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, candidate.desired) || info.Mode().Perm() != candidate.desiredMode.Perm() {
		return fmt.Errorf("destination bytes or mode do not match materialised candidate")
	}
	return nil
}

func verifyVendorOriginal(repo vendorRepository, candidate vendorCandidate) error {
	if err := verifyVendorOriginalFile(repo, candidate); err != nil {
		return err
	}
	if !candidate.original.exists {
		for _, missingParent := range candidate.original.missingParents {
			if _, parentErr := os.Lstat(filepath.Join(repo.root, filepath.FromSlash(missingParent))); parentErr == nil {
				return fmt.Errorf("originally absent parent %s still exists", missingParent)
			} else if !os.IsNotExist(parentErr) {
				return parentErr
			}
		}
	}
	return nil
}

// verifyVendorOriginalFile compares only the destination tuple. It is used
// immediately before each replacement because an earlier member in the same
// transaction may legitimately have created a shared parent directory.
func verifyVendorOriginalFile(repo vendorRepository, candidate vendorCandidate) error {
	abs, err := vendorDestinationPath(repo, candidate.path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if !candidate.original.exists {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("destination should be absent, found mode %s", info.Mode())
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = fmt.Errorf("destination is not a regular file")
		}
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, candidate.original.bytes) || info.Mode().Perm() != candidate.original.mode.Perm() {
		return fmt.Errorf("destination bytes or mode differ from original")
	}
	return nil
}

// publishVendorRecovery stages the complete restart authority outside the fixed
// recovery path, validates its durable bytes, and atomically publishes the
// whole root before apply may touch the primary worktree. The fixed root also
// serialises competing Sworn vendor writers.
func publishVendorRecovery(repo vendorRepository, plan *vendorPlan) (*vendorRecovery, error) {
	exists, err := validateVendorRecoveryPath(repo)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("recovery root already exists")
	}
	if err := ensureVendorRecoveryParentPath(repo); err != nil {
		return nil, err
	}
	recoveryParent := filepath.Dir(repo.recoveryRoot)
	gitRoot, err := os.OpenRoot(repo.gitAdmin)
	if err != nil {
		return nil, err
	}
	defer gitRoot.Close()
	recoveryParentRel, err := filepath.Rel(repo.gitAdmin, recoveryParent)
	if err != nil {
		return nil, err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	stagingName := ".baton-vendor-staging-" + hex.EncodeToString(random)
	stagingRel := filepath.Join(recoveryParentRel, stagingName)
	staging := filepath.Join(recoveryParent, stagingName)
	if err := gitRoot.Mkdir(stagingRel, 0o700); err != nil {
		return nil, err
	}
	published := false
	defer func() {
		if !published {
			_ = gitRoot.RemoveAll(stagingRel)
			_ = syncVendorRootDirectory(gitRoot, recoveryParentRel)
		}
	}()
	if err := gitRoot.Chmod(stagingRel, 0o700); err != nil {
		return nil, err
	}
	if err := syncVendorRootDirectory(gitRoot, recoveryParentRel); err != nil {
		return nil, err
	}
	stagingRoot, err := os.OpenRoot(staging)
	if err != nil {
		return nil, err
	}
	defer stagingRoot.Close()
	transactionStagingRel := "transaction-staging"
	if err := stagingRoot.Mkdir(transactionStagingRel, 0o700); err != nil {
		return nil, err
	}
	snapshotsRel := filepath.Join(transactionStagingRel, "snapshots")
	if err := stagingRoot.Mkdir(snapshotsRel, 0o700); err != nil {
		return nil, err
	}

	manifest := vendorRecoveryManifest{
		RecordVersion:     1,
		RepositoryRoot:    repo.root,
		GitAdminDirectory: repo.gitAdmin,
		Destinations:      make([]vendorRecoveryDestination, 0, len(plan.candidates)),
	}
	for i, candidate := range plan.candidates {
		record := vendorRecoveryDestination{
			Path:           candidate.path,
			OriginalExists: candidate.original.exists,
			OriginalMode:   "-",
			OriginalSHA256: "-",
			SnapshotPath:   "-",
		}
		if candidate.original.exists {
			snapshotRel := fmt.Sprintf("snapshots/%06d", i)
			snapshotStagingRel := filepath.Join(transactionStagingRel, filepath.FromSlash(snapshotRel))
			if err := writeVendorExclusiveRootFile(stagingRoot, snapshotStagingRel, candidate.original.bytes, 0o600); err != nil {
				return nil, err
			}
			digest := sha256.Sum256(candidate.original.bytes)
			record.OriginalMode = fmt.Sprintf("%04o", candidate.original.mode.Perm())
			record.OriginalSHA256 = "sha256:" + hex.EncodeToString(digest[:])
			record.SnapshotPath = snapshotRel
		}
		manifest.Destinations = append(manifest.Destinations, record)
	}
	manifestBytes, err := marshalVendorControlJSON(manifest)
	if err != nil {
		return nil, err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	transactionID := hex.EncodeToString(manifestDigest[:])
	if err := writeVendorExclusiveRootFile(stagingRoot, filepath.Join(transactionStagingRel, vendorRecoveryManifestName), manifestBytes, 0o600); err != nil {
		return nil, err
	}
	if err := syncVendorRootDirectory(stagingRoot, snapshotsRel); err != nil {
		return nil, err
	}
	if err := syncVendorRootDirectory(stagingRoot, transactionStagingRel); err != nil {
		return nil, err
	}
	if err := stagingRoot.Rename(transactionStagingRel, transactionID); err != nil {
		return nil, err
	}
	transactionDir := filepath.Join(repo.recoveryRoot, transactionID)
	sentinel := vendorRecoverySentinel{
		RecordVersion:     1,
		TransactionSHA256: "sha256:" + transactionID,
		RecoveryDirectory: transactionDir,
	}
	sentinelBytes, err := marshalVendorControlJSON(sentinel)
	if err != nil {
		return nil, err
	}
	if err := writeVendorExclusiveRootFile(stagingRoot, vendorRecoverySentinelName, sentinelBytes, 0o600); err != nil {
		return nil, err
	}
	if err := syncVendorRootDirectory(stagingRoot, "."); err != nil {
		return nil, err
	}
	if err := gitRoot.Rename(stagingRel, filepath.FromSlash(vendorRecoveryRelativeRoot)); err != nil {
		return nil, err
	}
	published = true
	if err := syncVendorRootDirectory(gitRoot, recoveryParentRel); err != nil {
		return nil, err
	}
	_, recovery, err := loadVendorRecovery(repo.root)
	if err != nil {
		return nil, err
	}
	if recovery == nil {
		return nil, fmt.Errorf("published recovery authority did not validate")
	}
	return recovery, nil
}

func validateVendorRecoveryMaterial(repo vendorRepository, transactionDir string, manifest vendorRecoveryManifest) (map[string][]byte, error) {
	if len(manifest.Destinations) != len(batonFileMappings)+1 {
		return nil, newVendorError("recovery-material-incomplete", "recovery", "", transactionDir, fmt.Errorf("unexpected destination count"))
	}
	expected := make(map[string]struct{}, len(batonFileMappings)+1)
	for _, mapping := range batonFileMappings {
		expected[filepath.ToSlash(mapping.Dest)] = struct{}{}
	}
	expected[upstreamVersionPath] = struct{}{}
	seen := make(map[string]struct{}, len(manifest.Destinations))
	referencedSnapshots := make(map[string]struct{})
	snapshots := make(map[string][]byte)
	lastPath := ""
	for i, destination := range manifest.Destinations {
		if err := validateVendorRelativePath(destination.Path); err != nil {
			return nil, newVendorError("recovery-destination-invalid", "recovery", destination.Path, transactionDir, err)
		}
		if i > 0 && destination.Path <= lastPath {
			return nil, newVendorError("recovery-destination-order-invalid", "recovery", destination.Path, transactionDir, fmt.Errorf("destinations are not unique byte-sorted paths"))
		}
		lastPath = destination.Path
		if _, ok := expected[destination.Path]; !ok {
			return nil, newVendorError("recovery-destination-foreign", "recovery", destination.Path, transactionDir, fmt.Errorf("destination is not vendor-owned"))
		}
		seen[destination.Path] = struct{}{}
		if _, err := vendorDestinationPath(repo, destination.Path); err != nil {
			return nil, newVendorError("recovery-destination-invalid", "recovery", destination.Path, transactionDir, err)
		}
		if _, err := inspectVendorParents(repo, destination.Path); err != nil {
			return nil, err
		}
		destinationAbs, _ := vendorDestinationPath(repo, destination.Path)
		if info, err := os.Lstat(destinationAbs); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, newVendorError("recovery-destination-node-invalid", "recovery", destination.Path, transactionDir, fmt.Errorf("destination is neither a regular file nor absent"))
			}
		} else if !os.IsNotExist(err) {
			return nil, newVendorError("recovery-destination-unreadable", "recovery", destination.Path, transactionDir, err)
		}
		if !destination.OriginalExists {
			if destination.OriginalMode != "-" || destination.OriginalSHA256 != "-" || destination.SnapshotPath != "-" {
				return nil, newVendorError("recovery-absent-tuple-invalid", "recovery", destination.Path, transactionDir, fmt.Errorf("absent original has mode, digest, or snapshot"))
			}
			continue
		}
		if _, err := parseVendorMode(destination.OriginalMode); err != nil {
			return nil, newVendorError("recovery-mode-invalid", "recovery", destination.Path, transactionDir, err)
		}
		if _, err := parseVendorDigest(destination.OriginalSHA256); err != nil {
			return nil, newVendorError("recovery-digest-invalid", "recovery", destination.Path, transactionDir, err)
		}
		expectedSnapshot := fmt.Sprintf("snapshots/%06d", i)
		if destination.SnapshotPath != expectedSnapshot || path.Clean(destination.SnapshotPath) != destination.SnapshotPath {
			return nil, newVendorError("recovery-snapshot-path-invalid", "recovery", destination.Path, transactionDir, fmt.Errorf("snapshot path is not canonical for its tuple"))
		}
		snapshotAbs := filepath.Join(transactionDir, filepath.FromSlash(destination.SnapshotPath))
		rel, err := filepath.Rel(transactionDir, snapshotAbs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, newVendorError("recovery-snapshot-escape", "recovery", destination.Path, transactionDir, fmt.Errorf("snapshot escapes transaction directory"))
		}
		data, err := readVendorControlFile(repo, snapshotAbs, 0o600)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		if "sha256:"+hex.EncodeToString(digest[:]) != destination.OriginalSHA256 {
			return nil, newVendorError("recovery-snapshot-digest-mismatch", "recovery", destination.Path, transactionDir, fmt.Errorf("snapshot bytes do not match manifest"))
		}
		referencedSnapshots[filepath.Base(snapshotAbs)] = struct{}{}
		snapshots[destination.Path] = data
	}
	for _, mapping := range batonFileMappings {
		if _, ok := seen[filepath.ToSlash(mapping.Dest)]; !ok {
			return nil, newVendorError("recovery-material-incomplete", "recovery", mapping.Dest, transactionDir, fmt.Errorf("mapped destination is missing"))
		}
	}
	if _, ok := seen[upstreamVersionPath]; !ok {
		return nil, newVendorError("recovery-material-incomplete", "recovery", upstreamVersionPath, transactionDir, fmt.Errorf("VERSION destination is missing"))
	}

	txEntries, err := readVendorDirectoryEntries(repo, transactionDir, 0o700)
	if err != nil || !vendorEntryNamesEqual(txEntries, []string{vendorRecoveryManifestName, "snapshots"}) {
		if err == nil {
			err = fmt.Errorf("transaction directory contains foreign material")
		}
		return nil, newVendorError("recovery-foreign-material", "recovery", "", transactionDir, err)
	}
	snapshotsDir := filepath.Join(transactionDir, "snapshots")
	snapshotEntries, err := readVendorDirectoryEntries(repo, snapshotsDir, 0o700)
	if err != nil || len(snapshotEntries) != len(referencedSnapshots) {
		if err == nil {
			err = fmt.Errorf("snapshot inventory does not match manifest")
		}
		return nil, newVendorError("recovery-snapshot-inventory-invalid", "recovery", "", snapshotsDir, err)
	}
	for _, entry := range snapshotEntries {
		if _, ok := referencedSnapshots[entry.Name()]; !ok {
			return nil, newVendorError("recovery-foreign-material", "recovery", "", filepath.Join(snapshotsDir, entry.Name()), fmt.Errorf("unreferenced snapshot"))
		}
	}
	return snapshots, nil
}

func recoverVendorTransaction(repo vendorRepository, recovery *vendorRecovery) error {
	unrestoredSet := make(map[string]struct{})
	for _, destination := range recovery.manifest.Destinations {
		original := vendorOriginal{exists: destination.OriginalExists}
		if destination.OriginalExists {
			mode, err := parseVendorMode(destination.OriginalMode)
			if err != nil {
				return newVendorError("recovery-mode-invalid", "recovery", destination.Path, recovery.transactionDir, err)
			}
			original.mode = mode
			original.bytes = recovery.snapshots[destination.Path]
		}
		candidate := vendorCandidate{path: destination.Path, original: original}
		if err := restoreVendorOriginal(repo, candidate); err != nil {
			unrestoredSet[destination.Path] = struct{}{}
		}
	}
	for _, destination := range recovery.manifest.Destinations {
		original := vendorOriginal{exists: destination.OriginalExists}
		if destination.OriginalExists {
			mode, _ := parseVendorMode(destination.OriginalMode)
			original.mode = mode
			original.bytes = recovery.snapshots[destination.Path]
		}
		if err := verifyVendorOriginal(repo, vendorCandidate{path: destination.Path, original: original}); err != nil {
			unrestoredSet[destination.Path] = struct{}{}
		}
	}
	if len(unrestoredSet) != 0 {
		unrestored := make([]string, 0, len(unrestoredSet))
		for _, destination := range recovery.manifest.Destinations {
			if _, failed := unrestoredSet[destination.Path]; failed {
				unrestored = append(unrestored, destination.Path)
			}
		}
		detail := "unrestored=" + strings.Join(unrestored, ",")
		return newVendorErrorWithDetail("rollback-incomplete", "recovery", "", recovery.transactionDir, detail, fmt.Errorf("%s", detail))
	}
	return retireAndCleanupVendorRecovery(repo, recovery)
}

func retireAndCleanupVendorRecovery(repo vendorRepository, recovery *vendorRecovery) error {
	recoveryParent := filepath.Dir(repo.recoveryRoot)
	gitRoot, err := os.OpenRoot(repo.gitAdmin)
	if err != nil {
		return newVendorError("recovery-retire-failed", "cleanup", "", recovery.root, err)
	}
	defer gitRoot.Close()
	recoveryParentRel, err := filepath.Rel(repo.gitAdmin, recoveryParent)
	if err != nil {
		return newVendorError("recovery-retire-failed", "cleanup", "", recovery.root, err)
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return newVendorError("recovery-retire-failed", "cleanup", "", recovery.root, err)
	}
	retiredName := ".baton-vendor-retired-" + hex.EncodeToString(random)
	retiredRel := filepath.Join(recoveryParentRel, retiredName)
	// Renaming the complete fixed root retires the sole normative sentinel and
	// its transaction in one directory operation. A crash before the rename
	// leaves recovery authoritative; a crash after it leaves no ambiguous or
	// non-normative material beneath the fixed recovery path.
	if err := gitRoot.Rename(filepath.FromSlash(vendorRecoveryRelativeRoot), retiredRel); err != nil {
		return newVendorError("recovery-retire-failed", "cleanup", "", recovery.root, err)
	}
	if err := syncVendorRootDirectory(gitRoot, recoveryParentRel); err != nil {
		return newVendorError("recovery-retire-failed", "cleanup", "", recoveryParent, err)
	}
	if err := gitRoot.RemoveAll(retiredRel); err != nil {
		return newVendorError("recovery-cleanup-failed", "cleanup", "", repo.recoveryRoot, err)
	}
	if err := syncVendorRootDirectory(gitRoot, recoveryParentRel); err != nil {
		return newVendorError("recovery-cleanup-failed", "cleanup", "", recoveryParent, err)
	}
	for parent := filepath.Dir(repo.recoveryRoot); parent != repo.gitAdmin; parent = filepath.Dir(parent) {
		parentRel, err := filepath.Rel(repo.gitAdmin, parent)
		if err != nil {
			return newVendorError("recovery-cleanup-failed", "cleanup", "", parent, err)
		}
		if err := gitRoot.Remove(parentRel); err != nil {
			if !os.IsNotExist(err) && !isDirectoryNotEmpty(err) {
				return newVendorError("recovery-cleanup-failed", "cleanup", "", parent, err)
			}
			continue
		}
		if err := syncVendorRootDirectory(gitRoot, filepath.Dir(parentRel)); err != nil {
			return newVendorError("recovery-cleanup-failed", "cleanup", "", filepath.Dir(parent), err)
		}
	}
	if err := syncVendorRootDirectory(gitRoot, "."); err != nil {
		return newVendorError("recovery-cleanup-failed", "cleanup", "", repo.gitAdmin, err)
	}
	return nil
}

func readVendorControlFile(repo vendorRepository, path string, mode os.FileMode) ([]byte, error) {
	root, rel, before, err := openVendorGitNode(repo, path, false, mode)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(rel)
	if err != nil {
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, err)
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, err)
	}
	if err := requireVendorControlNode(path, opened, false, mode); err != nil || !os.SameFile(before, opened) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, newVendorError("recovery-node-raced", "recovery", "", path, fmt.Errorf("control file identity changed while opening"))
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, readErr)
	}
	if closeErr != nil {
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, closeErr)
	}
	after, err := root.Lstat(rel)
	if err != nil {
		return nil, newVendorError("recovery-node-raced", "recovery", "", path, err)
	}
	if err := requireVendorControlNode(path, after, false, mode); err != nil || !os.SameFile(opened, after) {
		if err != nil {
			return nil, err
		}
		return nil, newVendorError("recovery-node-raced", "recovery", "", path, fmt.Errorf("control file identity changed while reading"))
	}
	return data, nil
}

func openVendorGitNode(repo vendorRepository, path string, directory bool, mode os.FileMode) (*os.Root, string, os.FileInfo, error) {
	rel, err := filepath.Rel(repo.gitAdmin, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, "", nil, newVendorError("recovery-path-escape", "recovery", "", path, fmt.Errorf("control path escapes Git-admin root"))
	}
	root, err := os.OpenRoot(repo.gitAdmin)
	if err != nil {
		return nil, "", nil, newVendorError("git-admin-unavailable", "recovery", "", repo.gitAdmin, err)
	}
	info, err := root.Lstat(rel)
	if err != nil {
		_ = root.Close()
		return nil, "", nil, newVendorError("recovery-material-missing", "recovery", "", path, err)
	}
	if err := requireVendorControlNode(path, info, directory, mode); err != nil {
		_ = root.Close()
		return nil, "", nil, err
	}
	return root, rel, info, nil
}

func requireVendorControlNode(path string, info os.FileInfo, directory bool, mode os.FileMode) error {
	if info.Mode()&os.ModeSymlink != 0 || (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return newVendorError("recovery-node-invalid", "recovery", "", path, fmt.Errorf("recovery node has forbidden type"))
	}
	if info.Mode().Perm() != mode.Perm() {
		return newVendorError("recovery-mode-drift", "recovery", "", path, fmt.Errorf("mode %04o, want %04o", info.Mode().Perm(), mode.Perm()))
	}
	return nil
}

func marshalVendorControlJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func decodeExactVendorJSON(data []byte, destination any) error {
	if err := rejectDuplicateVendorJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := ensureVendorJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func rejectDuplicateVendorJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walkValue func() error
	walkValue = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("JSON object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = struct{}{}
				if err := walkValue(); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return fmt.Errorf("invalid JSON object terminator")
			}
		case '[':
			for decoder.More() {
				if err := walkValue(); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return fmt.Errorf("invalid JSON array terminator")
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
		return nil
	}
	if err := walkValue(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func ensureVendorJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func parseVendorDigest(value string) (string, error) {
	hexDigest, ok := strings.CutPrefix(value, "sha256:")
	if !ok || len(hexDigest) != 64 || strings.ToLower(hexDigest) != hexDigest {
		return "", fmt.Errorf("digest must be lowercase sha256:<64 hex>")
	}
	decoded, err := hex.DecodeString(hexDigest)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("digest must be lowercase sha256:<64 hex>")
	}
	return hexDigest, nil
}

func parseVendorMode(value string) (os.FileMode, error) {
	if len(value) != 4 || value[0] != '0' {
		return 0, fmt.Errorf("mode must contain four octal digits")
	}
	parsed, err := strconv.ParseUint(value, 8, 12)
	if err != nil {
		return 0, fmt.Errorf("mode must contain four octal digits")
	}
	return os.FileMode(parsed), nil
}

func writeVendorExclusiveRootFile(root *os.Root, rel string, data []byte, mode os.FileMode) error {
	file, err := root.OpenFile(filepath.FromSlash(rel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	if err := file.Chmod(mode.Perm()); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func syncVendorDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func syncVendorRootDirectory(root *os.Root, rel string) error {
	if rel == "" {
		rel = "."
	}
	directory, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func readVendorDirectoryEntries(repo vendorRepository, path string, mode os.FileMode) ([]os.DirEntry, error) {
	root, rel, before, err := openVendorGitNode(repo, path, true, mode)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open(rel)
	if err != nil {
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, err)
	}
	opened, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, err)
	}
	if err := requireVendorControlNode(path, opened, true, mode); err != nil || !os.SameFile(before, opened) {
		_ = directory.Close()
		if err != nil {
			return nil, err
		}
		return nil, newVendorError("recovery-node-raced", "recovery", "", path, fmt.Errorf("control directory identity changed while opening"))
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, readErr)
	}
	if closeErr != nil {
		return nil, newVendorError("recovery-unreadable", "recovery", "", path, closeErr)
	}
	after, err := root.Lstat(rel)
	if err != nil {
		return nil, newVendorError("recovery-node-raced", "recovery", "", path, err)
	}
	if err := requireVendorControlNode(path, after, true, mode); err != nil || !os.SameFile(opened, after) {
		if err != nil {
			return nil, err
		}
		return nil, newVendorError("recovery-node-raced", "recovery", "", path, fmt.Errorf("control directory identity changed while reading"))
	}
	return entries, nil
}

func vendorEntriesContain(entries []os.DirEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name() == name {
			return true
		}
	}
	return false
}

func vendorEntryNamesEqual(entries []os.DirEntry, want []string) bool {
	if len(entries) != len(want) {
		return false
	}
	wanted := make(map[string]struct{}, len(want))
	for _, name := range want {
		if _, duplicate := wanted[name]; duplicate {
			return false
		}
		wanted[name] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := wanted[entry.Name()]; !ok {
			return false
		}
	}
	return true
}
