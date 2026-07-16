package baton

import (
	"bufio"
	"bytes"
	"crypto/rand"
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
	"unicode/utf8"
)

const (
	installRecoverySentinel  = "rollback-incomplete.json"
	installManifestName      = "manifest.bin"
	installManifestPrefix    = "sworn-baton-sync-rollback-v1"
	installVersionPath       = ".sworn-baton/VERSION"
	installIdentityName      = "owner-identity.json"
	installStageIdentityName = ".sworn-baton-stage-owner.json"
)

var errInstallCrash = errors.New("simulated install process crash")

// InstallRoots are the three complete logical homes and the fixed recovery
// authority root used by doctor --sync-baton.
type InstallRoots struct {
	AgentsHome   string
	CodexHome    string
	ClaudeHome   string
	RecoveryRoot string
}

// InstallState describes the successful sync outcome.
type InstallState string

const (
	InstallAlreadyExact InstallState = "already-exact"
	InstallRepaired     InstallState = "repaired"
	InstallRecovered    InstallState = "recovered-rerun-required"
)

// InstallResult is path-only and safe to render publicly.
type InstallResult struct {
	State   InstallState
	Changed []string
}

// InstallFault is a test-only fault seam. Production passes nil.
type InstallFault func(point string) error

// InstallOpts is the complete immutable input for one sync invocation.
type InstallOpts struct {
	Roots   InstallRoots
	Trees   InstallerManagedTrees
	Version []byte
	Fault   InstallFault
	// OperationIDForTest makes pre-transaction debris fixtures deterministic.
	// Production callers must leave it empty.
	OperationIDForTest string
}

// InstallError omits payload and snapshot bytes by construction.
type InstallError struct {
	Class      string
	Paths      []string
	Recovery   string
	underlying error
}

func (e *InstallError) Error() string {
	parts := []string{"baton install: class=" + e.Class}
	if len(e.Paths) != 0 {
		parts = append(parts, "paths="+strings.Join(e.Paths, ","))
	}
	if e.Recovery != "" {
		parts = append(parts, "recovery="+e.Recovery)
	}
	return strings.Join(parts, ": ")
}

func (e *InstallError) Unwrap() error { return e.underlying }

func newInstallError(class string, paths []string, recovery string, err error) error {
	paths = uniqueSortedStrings(paths)
	return &InstallError{Class: class, Paths: paths, Recovery: recovery, underlying: err}
}

type installTarget struct {
	logical  string
	path     string
	tree     ManagedTree
	snapshot string
	stage    string
	absent   bool
}

type installPathIdentity struct {
	logical      string
	path         string
	ancestor     string
	ancestorInfo fs.FileInfo
	missing      []string
}

type installManifestEntry struct {
	Path   string
	Kind   string
	Mode   os.FileMode
	Digest string
}

type installSentinel struct {
	RecordVersion     int                     `json:"record_version"`
	TransactionSHA256 string                  `json:"transaction_sha256"`
	RecoveryDirectory string                  `json:"recovery_directory"`
	Targets           []installSentinelTarget `json:"targets"`
	UnrestoredPaths   []string                `json:"unrestored_paths"`
}

type installOwnerIdentity struct {
	RecordVersion int                     `json:"record_version"`
	OperationID   string                  `json:"operation_id"`
	RecoveryRoot  string                  `json:"recovery_root"`
	Targets       []installSentinelTarget `json:"targets"`
}

type installStageIdentity struct {
	RecordVersion     int    `json:"record_version"`
	TransactionSHA256 string `json:"transaction_sha256"`
	LogicalRoot       string `json:"logical_root"`
	TargetPath        string `json:"target_path"`
}

type installSentinelTarget struct {
	LogicalRoot  string `json:"logical_root"`
	TargetPath   string `json:"target_path"`
	SnapshotPath string `json:"snapshot_path"`
}

type preparedInstall struct {
	roots           InstallRoots
	targets         []installTarget
	manifest        []installManifestEntry
	manifestRaw     []byte
	operation       string
	transaction     string
	fault           InstallFault
	identities      map[string]installPathIdentity
	recoveryStaging string
	retiredPath     string
	sentinelTemp    string
	createdPaths    map[string]fs.FileInfo
}

// CheckBatonInstall validates topology, embedded source, sentinels, and every
// managed byte/mode without writing. A pending recovery sentinel is an error.
func CheckBatonInstall(opts InstallOpts) ([]string, error) {
	prepared, err := prepareInstall(opts, false)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(prepared.roots.RecoveryRoot, installRecoverySentinel)); err == nil {
		return nil, newInstallError("recovery-required", nil, prepared.roots.RecoveryRoot, errors.New("recovery sentinel present"))
	} else if !os.IsNotExist(err) {
		return nil, newInstallError("recovery-unreadable", nil, prepared.roots.RecoveryRoot, err)
	}
	if err := reassertInstallIdentities(prepared, true); err != nil {
		return nil, err
	}
	for _, target := range prepared.targets {
		if err := scanInstallTarget(target.path); err != nil {
			return nil, err
		}
	}
	return installDrift(prepared.targets, opts.Version)
}

// SyncBatonInstall repairs all three logical roots in one rollback-protected
// transaction. Sentinel presence always routes to recovery-only restoration.
func SyncBatonInstall(opts InstallOpts) (*InstallResult, error) {
	prepared, err := prepareInstall(opts, false)
	if err != nil {
		return nil, err
	}
	sentinelPath := filepath.Join(prepared.roots.RecoveryRoot, installRecoverySentinel)
	if _, statErr := os.Lstat(sentinelPath); statErr == nil {
		if err := recoverBatonInstall(prepared); err != nil {
			return nil, err
		}
		return &InstallResult{State: InstallRecovered}, nil
	} else if !os.IsNotExist(statErr) {
		return nil, newInstallError("recovery-unreadable", nil, prepared.roots.RecoveryRoot, statErr)
	}
	if err := cleanupInstallStageDebris(prepared); err != nil {
		return nil, err
	}
	if err := cleanupIncompleteInstallRecovery(prepared); err != nil {
		return nil, err
	}
	if err := reassertInstallIdentities(prepared, true); err != nil {
		return nil, err
	}
	for _, target := range prepared.targets {
		if err := scanInstallTarget(target.path); err != nil {
			return nil, err
		}
	}

	drift, err := installDrift(prepared.targets, opts.Version)
	if err != nil {
		return nil, err
	}
	retired, err := findRetiredInstallRecovery(prepared)
	if err != nil {
		return nil, err
	}
	if retired != "" && len(drift) != 0 {
		return nil, newInstallError("retired-recovery-pending", drift, retired, errors.New("installed roots differ while retired authority exists"))
	}
	if len(drift) == 0 {
		if retired != "" {
			if err := cleanupRetiredInstallRecovery(prepared, retired); err != nil {
				return nil, err
			}
		}
		if err := reassertInstallIdentities(prepared, false); err != nil {
			return nil, err
		}
		return &InstallResult{State: InstallAlreadyExact}, nil
	}

	if err := beginInstallTransaction(prepared, opts.OperationIDForTest); err != nil {
		return nil, err
	}
	cleanupOnReturn := true
	defer func() {
		if cleanupOnReturn {
			cleanupInstallStages(prepared)
		}
	}()
	phaseError := func(err error) error {
		if errors.Is(err, errInstallCrash) {
			cleanupOnReturn = false
			return newInstallError("process-crashed", nil, prepared.roots.RecoveryRoot, err)
		}
		return err
	}
	if err := captureInstallSources(prepared); err != nil {
		return nil, phaseError(err)
	}
	if err := stageDesiredInstall(prepared, opts.Version); err != nil {
		return nil, phaseError(err)
	}

	if err := publishInstallRecovery(prepared); err != nil {
		return nil, phaseError(err)
	}

	var applyErr error
	for i := range prepared.targets {
		target := &prepared.targets[i]
		if err := revalidateUnreplacedInstallTargets(prepared, i); err != nil {
			applyErr = err
			break
		}
		if err := callInstallFault(prepared.fault, "replace-before:"+target.logical); err != nil {
			applyErr = err
			break
		}
		if err := replaceInstallRoot(prepared, *target); err != nil {
			applyErr = err
			break
		}
		if err := callInstallFault(prepared.fault, "installed-sync-before:"+target.logical); err != nil {
			applyErr = err
			break
		}
		if err := syncTree(target.path); err != nil {
			applyErr = err
			break
		}
		if err := callInstallFault(prepared.fault, "installed-sync-after:"+target.logical); err != nil {
			applyErr = err
			break
		}
		if err := callInstallFault(prepared.fault, "replace-after:"+target.logical); err != nil {
			applyErr = err
			break
		}
		if err := verifyInstalledTarget(*target, opts.Version); err != nil {
			applyErr = err
			break
		}
		if err := callInstallFault(prepared.fault, "verify-after:"+target.logical); err != nil {
			applyErr = err
			break
		}
	}
	if applyErr != nil {
		if errors.Is(applyErr, errInstallCrash) {
			cleanupOnReturn = false
			return nil, newInstallError("process-crashed", nil, prepared.roots.RecoveryRoot, applyErr)
		}
		unrestored := restoreInstallTargets(prepared, prepared.manifest)
		if len(unrestored) != 0 {
			if updateErr := updateInstallUnrestored(prepared, unrestored); updateErr != nil {
				return nil, newInstallError("rollback-incomplete", append(unrestored, "recovery"), prepared.roots.RecoveryRoot, errors.Join(applyErr, updateErr))
			}
			return nil, newInstallError("rollback-incomplete", unrestored, prepared.roots.RecoveryRoot, applyErr)
		}
		if retireErr := retireInstallRecovery(prepared); retireErr != nil {
			return nil, newInstallError("rollback-incomplete", []string{"recovery"}, prepared.roots.RecoveryRoot, retireErr)
		}
		return nil, newInstallError("repair-failed-restored", drift, "", applyErr)
	}

	if err := retireInstallRecovery(prepared); err != nil {
		if errors.Is(err, errInstallCrash) {
			cleanupOnReturn = false
			return nil, newInstallError("process-crashed", nil, prepared.roots.RecoveryRoot, err)
		}
		return nil, newInstallError("recovery-retire-failed", []string{"recovery"}, prepared.roots.RecoveryRoot, err)
	}
	return &InstallResult{State: InstallRepaired, Changed: drift}, nil
}

func prepareInstall(opts InstallOpts, needManifest bool) (*preparedInstall, error) {
	_ = needManifest
	if len(opts.Version) == 0 {
		return nil, newInstallError("version-invalid", nil, "", errors.New("empty VERSION sentinel"))
	}
	roots, identities, err := resolveInstallRoots(opts.Roots)
	if err != nil {
		return nil, err
	}
	targets := []installTarget{
		{logical: "agents_home", path: roots.AgentsHome, tree: opts.Trees.AgentsHome},
		{logical: "claude_home", path: roots.ClaudeHome, tree: opts.Trees.ClaudeHome},
		{logical: "codex_home", path: roots.CodexHome, tree: opts.Trees.CodexHome},
	}
	for i := range targets {
		if len(targets[i].tree.Entries) == 0 {
			return nil, newInstallError("managed-tree-empty", []string{targets[i].logical}, "", errors.New("empty managed tree"))
		}
	}
	return &preparedInstall{roots: roots, targets: targets, fault: opts.Fault, identities: identities, createdPaths: make(map[string]fs.FileInfo)}, nil
}

func resolveInstallRoots(input InstallRoots) (InstallRoots, map[string]installPathIdentity, error) {
	values := []struct {
		logical string
		path    string
	}{
		{"agents_home", input.AgentsHome},
		{"claude_home", input.ClaudeHome},
		{"codex_home", input.CodexHome},
		{"recovery", input.RecoveryRoot},
	}
	resolved := make(map[string]string, len(values))
	infos := make(map[string]fs.FileInfo)
	identities := make(map[string]installPathIdentity, len(values))
	for _, value := range values {
		identity, info, err := resolvePhysicalNoSymlink(value.logical, value.path)
		if err != nil {
			return InstallRoots{}, nil, newInstallError("unsafe-root", []string{value.logical}, "", err)
		}
		resolved[value.logical] = identity.path
		identities[value.logical] = identity
		parentIdentity, _, parentErr := resolvePhysicalNoSymlink(value.logical+":parent", filepath.Dir(identity.path))
		if parentErr != nil {
			return InstallRoots{}, nil, newInstallError("unsafe-root", []string{value.logical}, "", parentErr)
		}
		identities[value.logical+":parent"] = parentIdentity
		if info != nil {
			infos[value.logical] = info
		}
	}
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			a, b := values[i].logical, values[j].logical
			if pathsOverlap(resolved[a], resolved[b]) || (infos[a] != nil && infos[b] != nil && os.SameFile(infos[a], infos[b])) || installIdentitySuffixesOverlap(identities[a], identities[b]) {
				return InstallRoots{}, nil, newInstallError("unsafe-root-topology", []string{a, b}, "", errors.New("roots overlap or alias"))
			}
		}
	}
	return InstallRoots{
		AgentsHome: resolved["agents_home"], CodexHome: resolved["codex_home"],
		ClaudeHome: resolved["claude_home"], RecoveryRoot: resolved["recovery"],
	}, identities, nil
}

func installIdentitySuffixesOverlap(a, b installPathIdentity) bool {
	if a.ancestorInfo == nil || b.ancestorInfo == nil || !os.SameFile(a.ancestorInfo, b.ancestorInfo) {
		return false
	}
	return pathPartsOverlap(a.missing, b.missing)
}

func pathPartsOverlap(a, b []string) bool {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func resolvePhysicalNoSymlink(logical, name string) (installPathIdentity, fs.FileInfo, error) {
	if name == "" || !utf8.ValidString(name) {
		return installPathIdentity{}, nil, errors.New("path is empty or invalid UTF-8")
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		return installPathIdentity{}, nil, err
	}
	abs = filepath.Clean(abs)
	volume := filepath.VolumeName(abs)
	rest := strings.TrimPrefix(abs, volume)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(rest, string(filepath.Separator)), string(filepath.Separator))
	var finalInfo fs.FileInfo
	var ancestorInfo fs.FileInfo
	ancestor := current
	var missing []string
	for i, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			missing = append(missing, part)
			for _, suffix := range parts[i+1:] {
				if suffix != "" {
					current = filepath.Join(current, suffix)
					missing = append(missing, suffix)
				}
			}
			return installPathIdentity{logical: logical, path: filepath.Clean(current), ancestor: ancestor, ancestorInfo: ancestorInfo, missing: missing}, nil, nil
		}
		if statErr != nil {
			return installPathIdentity{}, nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return installPathIdentity{}, nil, fmt.Errorf("symlink component: %s", current)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return installPathIdentity{}, nil, fmt.Errorf("non-directory component: %s", current)
		}
		finalInfo = info
		ancestor = current
		ancestorInfo = info
	}
	if finalInfo != nil && !finalInfo.IsDir() {
		return installPathIdentity{}, nil, fmt.Errorf("root is not a directory: %s", abs)
	}
	return installPathIdentity{logical: logical, path: abs, ancestor: ancestor, ancestorInfo: ancestorInfo}, finalInfo, nil
}

func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	relAB, errAB := filepath.Rel(a, b)
	relBA, errBA := filepath.Rel(b, a)
	return (errAB == nil && relAB != ".." && !strings.HasPrefix(relAB, ".."+string(filepath.Separator))) ||
		(errBA == nil && relBA != ".." && !strings.HasPrefix(relBA, ".."+string(filepath.Separator)))
}

func reassertInstallIdentities(prepared *preparedInstall, strict bool) error {
	for _, logical := range []string{"agents_home", "claude_home", "codex_home", "recovery"} {
		key := logical
		if !strict {
			key += ":parent"
		}
		identity, ok := prepared.identities[key]
		if !ok {
			return newInstallError("unsafe-root-identity", []string{logical}, "", errors.New("identity missing"))
		}
		if err := reassertInstallPathIdentity(identity, strict); err != nil {
			return newInstallError("unsafe-root-identity", []string{logical}, "", err)
		}
	}
	return nil
}

func reassertInstallPathIdentity(identity installPathIdentity, requireMissing bool) error {
	current, _, err := resolvePhysicalNoSymlink(identity.logical, identity.path)
	if err != nil {
		return err
	}
	if current.path != identity.path {
		return errors.New("physical path identity changed")
	}
	ancestorNow, err := os.Lstat(identity.ancestor)
	if err != nil || identity.ancestorInfo == nil || !os.SameFile(identity.ancestorInfo, ancestorNow) {
		return errors.New("nearest existing ancestor changed")
	}
	if requireMissing && (current.ancestor != identity.ancestor || !equalStrings(identity.missing, current.missing)) {
		return errors.New("missing suffix changed")
	}
	return nil
}

func beginInstallTransaction(prepared *preparedInstall, forced string) error {
	id := forced
	if id == "" {
		raw := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			return newInstallError("transaction-id-failed", nil, "", err)
		}
		id = hex.EncodeToString(raw)
	}
	if len(id) != 64 {
		return newInstallError("transaction-id-invalid", nil, "", errors.New("transaction id must be 32-byte lowercase hex"))
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || hex.EncodeToString(decoded) != id {
		return newInstallError("transaction-id-invalid", nil, "", errors.New("transaction id must be 32-byte lowercase hex"))
	}
	prepared.operation = id
	prepared.recoveryStaging = filepath.Join(prepared.roots.RecoveryRoot, ".staging-"+id)
	for i := range prepared.targets {
		target := &prepared.targets[i]
		target.snapshot = filepath.Join(prepared.recoveryStaging, "snapshots", target.logical)
	}
	if err := validateInstallOperationalTopology(prepared); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "paths-ready"); err != nil {
		return err
	}
	if err := reassertInstallIdentities(prepared, true); err != nil {
		return err
	}
	return validateInstallOperationalTopology(prepared)
}

func finalizeInstallTransaction(prepared *preparedInstall) error {
	digest := sha256.Sum256(prepared.manifestRaw)
	prepared.transaction = hex.EncodeToString(digest[:])
	prepared.retiredPath = filepath.Join(filepath.Dir(prepared.roots.RecoveryRoot), ".baton-sync-retired-"+prepared.transaction)
	prepared.sentinelTemp = filepath.Join(prepared.roots.RecoveryRoot, installRecoverySentinel+".tmp-"+prepared.transaction)
	for i := range prepared.targets {
		target := &prepared.targets[i]
		target.stage = filepath.Join(filepath.Dir(target.path), ".sworn-baton-stage-"+prepared.transaction+"-"+target.logical)
	}
	if err := validateInstallOperationalTopology(prepared); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "transaction-paths-ready"); err != nil {
		return err
	}
	return validateInstallOperationalTopology(prepared)
}

func validateInstallOperationalTopology(prepared *preparedInstall) error {
	targetPaths := []string{prepared.roots.AgentsHome, prepared.roots.ClaudeHome, prepared.roots.CodexHome}
	standalone := []struct {
		logical string
		path    string
	}{}
	if prepared.retiredPath != "" {
		standalone = append(standalone, struct {
			logical string
			path    string
		}{"retired", prepared.retiredPath})
	}
	for _, target := range prepared.targets {
		if target.stage == "" {
			continue
		}
		standalone = append(standalone, struct {
			logical string
			path    string
		}{"stage_" + target.logical, target.stage})
	}
	for i, candidate := range standalone {
		identity, _, err := resolvePhysicalNoSymlink(candidate.logical, candidate.path)
		if err != nil {
			return newInstallError("unsafe-operation-path", []string{candidate.logical}, "", err)
		}
		if _, err := os.Lstat(identity.path); err == nil || !os.IsNotExist(err) {
			return newInstallError("operation-path-collision", []string{candidate.logical}, identity.path, errors.New("derived path already exists"))
		}
		for _, root := range append(append([]string{}, targetPaths...), prepared.roots.RecoveryRoot) {
			if pathsOverlap(identity.path, root) {
				return newInstallError("unsafe-operation-topology", []string{candidate.logical}, identity.path, errors.New("derived path overlaps a logical root"))
			}
		}
		for j := 0; j < i; j++ {
			if pathsOverlap(identity.path, standalone[j].path) {
				return newInstallError("unsafe-operation-topology", []string{candidate.logical, standalone[j].logical}, "", errors.New("derived paths overlap"))
			}
		}
	}
	contained := []struct {
		logical string
		path    string
	}{{"recovery_staging", prepared.recoveryStaging}}
	if prepared.transaction != "" {
		contained = append(contained, struct {
			logical string
			path    string
		}{"transaction", filepath.Join(prepared.roots.RecoveryRoot, prepared.transaction)})
	}
	if prepared.sentinelTemp != "" {
		contained = append(contained, struct {
			logical string
			path    string
		}{"sentinel_temp", prepared.sentinelTemp})
	}
	for _, candidate := range contained {
		rel, err := filepath.Rel(prepared.roots.RecoveryRoot, candidate.path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return newInstallError("unsafe-operation-topology", []string{candidate.logical}, candidate.path, errors.New("control path escapes recovery root"))
		}
		if info, err := os.Lstat(candidate.path); err == nil {
			owned, ok := prepared.createdPaths[candidate.path]
			if candidate.path != prepared.recoveryStaging || !ok || validateOwnedInstallPath(candidate.path, owned, info.IsDir()) != nil {
				return newInstallError("operation-path-collision", []string{candidate.logical}, candidate.path, errors.New("derived path already exists"))
			}
		} else if !os.IsNotExist(err) {
			return newInstallError("operation-path-collision", []string{candidate.logical}, candidate.path, err)
		}
		for _, root := range targetPaths {
			if pathsOverlap(candidate.path, root) {
				return newInstallError("unsafe-operation-topology", []string{candidate.logical}, candidate.path, errors.New("control path overlaps target"))
			}
		}
	}
	return nil
}

func scanInstallTarget(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return newInstallError("target-unreadable", []string{root}, "", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return newInstallError("unsafe-target", []string{root}, "", errors.New("target is not a real directory"))
	}
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return newInstallError("target-unreadable", []string{root}, "", walkErr)
		}
		rel, relErr := filepath.Rel(root, name)
		if relErr != nil || !utf8.ValidString(filepath.ToSlash(rel)) {
			return newInstallError("unsafe-target", []string{root}, "", errors.New("invalid target path"))
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return newInstallError("target-unreadable", []string{root}, "", infoErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return newInstallError("unsafe-target", []string{root + "/" + filepath.ToSlash(rel)}, "", errors.New("unsupported node"))
		}
		return nil
	})
}

func ensureInstallDirectory(prepared *preparedInstall, name string, finalMode os.FileMode) error {
	identity, _, err := resolvePhysicalNoSymlink("operation_parent", name)
	if err != nil {
		return err
	}
	if len(identity.missing) == 0 {
		info, err := os.Lstat(identity.path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("operation parent is not a real directory")
		}
		if identity.path == prepared.roots.RecoveryRoot && info.Mode().Perm() != finalMode {
			return newInstallError("recovery-mode-invalid", nil, identity.path, errors.New("pre-existing recovery root mode differs"))
		}
		return nil
	}
	current := identity.ancestor
	for i, part := range identity.missing {
		current = filepath.Join(current, part)
		mode := os.FileMode(0o755)
		if i == len(identity.missing)-1 {
			mode = finalMode
		}
		if err := os.Mkdir(current, mode); err != nil {
			return err
		}
		if err := os.Chmod(current, mode); err != nil {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		prepared.createdPaths[current] = info
	}
	return syncDir(identity.ancestor)
}

func createOwnedInstallDir(prepared *preparedInstall, name string, mode os.FileMode) error {
	if err := os.Mkdir(name, mode); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	prepared.createdPaths[name] = info
	return nil
}

func writeExclusiveInstallFile(name string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	ok = true
	return syncDir(filepath.Dir(name))
}

func revalidateUnreplacedInstallTargets(prepared *preparedInstall, start int) error {
	if err := reassertInstallIdentities(prepared, false); err != nil {
		return err
	}
	want := manifestByRoot(prepared.manifest)
	for i := start; i < len(prepared.targets); i++ {
		target := prepared.targets[i]
		if err := scanInstallTarget(target.path); err != nil {
			return err
		}
		actual, err := snapshotManifestEntries([]installTarget{{logical: target.logical, path: target.path}})
		if err != nil {
			return err
		}
		if !bytes.Equal(marshalInstallManifest(actual), marshalInstallManifest(want[target.logical])) {
			return newInstallError("source-changed", []string{target.logical}, "", errors.New("live root changed after immutable capture"))
		}
	}
	return nil
}

func installDrift(targets []installTarget, version []byte) ([]string, error) {
	var drift []string
	for _, target := range targets {
		items, err := verifyTargetDrift(target, version)
		if err != nil {
			return nil, err
		}
		drift = append(drift, items...)
	}
	return uniqueSortedStrings(drift), nil
}

func verifyTargetDrift(target installTarget, version []byte) ([]string, error) {
	if _, err := os.Lstat(target.path); os.IsNotExist(err) {
		return []string{target.logical}, nil
	} else if err != nil {
		return nil, newInstallError("target-unreadable", []string{target.logical}, "", err)
	}
	var drift []string
	for _, entry := range target.tree.Entries {
		dest := filepath.Join(target.path, filepath.FromSlash(entry.Path))
		info, err := os.Lstat(dest)
		logicalPath := target.logical + "/" + entry.Path
		if err != nil {
			drift = append(drift, logicalPath)
			continue
		}
		if entry.IsDir {
			if !info.IsDir() || info.Mode().Perm() != entry.Mode {
				drift = append(drift, logicalPath)
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != entry.Mode {
			drift = append(drift, logicalPath)
			continue
		}
		contents, readErr := os.ReadFile(dest)
		if readErr != nil {
			return nil, newInstallError("target-unreadable", []string{logicalPath}, "", readErr)
		}
		if !bytes.Equal(contents, entry.Bytes) {
			drift = append(drift, logicalPath)
		}
	}
	sentinel := filepath.Join(target.path, filepath.FromSlash(installVersionPath))
	if contents, err := os.ReadFile(sentinel); err != nil || !bytes.Equal(contents, version) {
		drift = append(drift, target.logical+"/"+installVersionPath)
	} else if info, err := os.Lstat(sentinel); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o644 {
		drift = append(drift, target.logical+"/"+installVersionPath)
	}
	if extras, err := managedExtras(target); err != nil {
		return nil, err
	} else {
		drift = append(drift, extras...)
	}
	return uniqueSortedStrings(drift), nil
}

func managedExtras(target installTarget) ([]string, error) {
	expected := make(map[string]struct{}, len(target.tree.Entries))
	for _, entry := range target.tree.Entries {
		expected[entry.Path] = struct{}{}
	}
	var roots []string
	switch target.logical {
	case "agents_home":
		skills := filepath.Join(target.path, "skills")
		entries, err := os.ReadDir(skills)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		var extras []string
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "baton-") {
				rel := filepath.ToSlash(filepath.Join("skills", entry.Name()))
				if _, ok := expected[rel]; !ok {
					extras = append(extras, target.logical+"/"+rel)
				}
			}
		}
		return extras, nil
	case "codex_home":
		roots = []string{"baton"}
	case "claude_home":
		roots = []string{"baton"}
	}
	var extras []string
	for _, managedRoot := range roots {
		abs := filepath.Join(target.path, managedRoot)
		walkErr := filepath.WalkDir(abs, func(name string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(target.path, name)
			rel = filepath.ToSlash(rel)
			if _, ok := expected[rel]; !ok {
				extras = append(extras, target.logical+"/"+rel)
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return extras, nil
}

func captureInstallSources(prepared *preparedInstall) error {
	if err := ensureInstallDirectory(prepared, filepath.Dir(prepared.roots.RecoveryRoot), 0o755); err != nil {
		return err
	}
	if err := ensureInstallDirectory(prepared, prepared.roots.RecoveryRoot, 0o700); err != nil {
		return err
	}
	if err := createOwnedInstallDir(prepared, prepared.recoveryStaging, 0o700); err != nil {
		return newInstallError("recovery-publication-failed", nil, prepared.recoveryStaging, err)
	}
	if err := createOwnedInstallDir(prepared, filepath.Join(prepared.recoveryStaging, "snapshots"), 0o700); err != nil {
		return err
	}
	identity := installOwnerIdentity{RecordVersion: 1, OperationID: prepared.operation, RecoveryRoot: prepared.roots.RecoveryRoot}
	for _, target := range prepared.targets {
		identity.Targets = append(identity.Targets, installSentinelTarget{
			LogicalRoot: target.logical, TargetPath: target.path,
		})
	}
	identityRaw, err := marshalInstallOwnerIdentity(identity)
	if err != nil {
		return err
	}
	if err := writeExclusiveInstallFile(filepath.Join(prepared.recoveryStaging, installIdentityName), identityRaw, 0o600); err != nil {
		return err
	}

	var manifest []installManifestEntry
	for i := range prepared.targets {
		target := &prepared.targets[i]
		if err := callInstallFault(prepared.fault, "snapshot-before:"+target.logical); err != nil {
			return err
		}
		entries, absent, err := captureInstallTarget(*target)
		if err != nil {
			return newInstallError("snapshot-failed", []string{target.logical}, "", err)
		}
		target.absent = absent
		manifest = append(manifest, entries...)
		if err := callInstallFault(prepared.fault, "snapshot-after:"+target.logical); err != nil {
			return err
		}
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Path < manifest[j].Path })
	prepared.manifest = manifest
	prepared.manifestRaw = marshalInstallManifest(manifest)
	if err := finalizeInstallTransaction(prepared); err != nil {
		return err
	}
	if err := revalidateUnreplacedInstallTargets(prepared, 0); err != nil {
		return err
	}
	manifestPath := filepath.Join(prepared.recoveryStaging, installManifestName)
	if err := writeExclusiveInstallFile(manifestPath, prepared.manifestRaw, 0o600); err != nil {
		return err
	}
	if err := syncTree(prepared.recoveryStaging); err != nil {
		return newInstallError("recovery-publication-failed", nil, prepared.recoveryStaging, err)
	}
	return nil
}

func captureInstallTarget(target installTarget) ([]installManifestEntry, bool, error) {
	rootInfo, err := os.Lstat(target.path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(target.snapshot, 0o700); err != nil {
			return nil, false, err
		}
		if err := os.Chmod(target.snapshot, 0o700); err != nil {
			return nil, false, err
		}
		return []installManifestEntry{{Path: target.logical + "/", Kind: "absent"}}, true, nil
	}
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, false, errors.New("capture root is not a real directory")
	}
	if err := os.Mkdir(target.snapshot, 0o700); err != nil {
		return nil, false, err
	}
	if err := os.Chmod(target.snapshot, 0o700); err != nil {
		return nil, false, err
	}
	entries := []installManifestEntry{{Path: target.logical + "/", Kind: "directory", Mode: rootInfo.Mode().Perm()}}
	err = filepath.WalkDir(target.path, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == target.path {
			return nil
		}
		rel, err := filepath.Rel(target.path, name)
		if err != nil {
			return err
		}
		logicalPath := target.logical + "/" + filepath.ToSlash(rel)
		destination := filepath.Join(target.snapshot, rel)
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("capture entry invalid")
		}
		item := installManifestEntry{Path: logicalPath, Mode: info.Mode().Perm()}
		if info.IsDir() {
			item.Kind = "directory"
			if err := os.Mkdir(destination, 0o700); err != nil {
				return err
			}
			if err := os.Chmod(destination, 0o700); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			item.Kind = "file"
			contents, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(contents)
			item.Digest = "sha256:" + hex.EncodeToString(digest[:])
			if err := os.WriteFile(destination, contents, 0o600); err != nil {
				return err
			}
			if err := os.Chmod(destination, 0o600); err != nil {
				return err
			}
		} else {
			return errors.New("capture contains unsupported node")
		}
		entries = append(entries, item)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return entries, false, nil
}

func stageDesiredInstall(prepared *preparedInstall, version []byte) error {
	manifest := manifestByRoot(prepared.manifest)
	for i := range prepared.targets {
		target := &prepared.targets[i]
		if err := ensureInstallDirectory(prepared, filepath.Dir(target.stage), 0o755); err != nil {
			return err
		}
		if err := createOwnedInstallDir(prepared, target.stage, 0o700); err != nil {
			return newInstallError("stage-collision", []string{target.logical}, target.stage, err)
		}
		if err := copyCompleteTree(target.snapshot, target.stage, false, manifest[target.logical]); err != nil {
			return newInstallError("stage-failed", []string{target.logical}, "", err)
		}
		rootMode := os.FileMode(0o755)
		for _, entry := range prepared.manifest {
			if entry.Path == target.logical+"/" && entry.Kind == "directory" {
				rootMode = entry.Mode
				break
			}
		}
		if err := clearManagedInstall(target.logical, target.stage); err != nil {
			return newInstallError("stage-failed", []string{target.logical}, "", err)
		}
		if err := WriteManagedTree(target.stage, target.tree); err != nil {
			return newInstallError("stage-failed", []string{target.logical}, "", err)
		}
		versionPath := filepath.Join(target.stage, filepath.FromSlash(installVersionPath))
		if err := os.MkdirAll(filepath.Dir(versionPath), 0o755); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Dir(versionPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(versionPath, version, 0o644); err != nil {
			return err
		}
		if err := os.Chmod(versionPath, 0o644); err != nil {
			return err
		}
		if err := os.Chmod(target.stage, rootMode); err != nil {
			return err
		}
		stageIdentity := installStageIdentity{
			RecordVersion: 1, TransactionSHA256: "sha256:" + prepared.transaction,
			LogicalRoot: target.logical, TargetPath: target.path,
		}
		identityRaw, err := marshalInstallStageIdentity(stageIdentity)
		if err != nil {
			return err
		}
		if err := writeExclusiveInstallFile(filepath.Join(target.stage, installStageIdentityName), identityRaw, 0o600); err != nil {
			return newInstallError("stage-failed", []string{target.logical}, "", err)
		}
		stageTarget := *target
		stageTarget.path = target.stage
		if err := verifyInstalledTarget(stageTarget, version); err != nil {
			return err
		}
		if err := callInstallFault(prepared.fault, "stage-sync-before:"+target.logical); err != nil {
			return err
		}
		if err := syncTree(target.stage); err != nil {
			return newInstallError("stage-failed", []string{target.logical}, "", err)
		}
		if err := callInstallFault(prepared.fault, "stage-sync-after:"+target.logical); err != nil {
			return err
		}
	}
	return nil
}

func clearManagedInstall(logical, root string) error {
	switch logical {
	case "agents_home":
		entries, err := os.ReadDir(filepath.Join(root, "skills"))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "baton-") {
				if err := os.RemoveAll(filepath.Join(root, "skills", entry.Name())); err != nil {
					return err
				}
			}
		}
	case "codex_home":
		if err := os.RemoveAll(filepath.Join(root, "baton")); err != nil {
			return err
		}
	case "claude_home":
		if err := os.RemoveAll(filepath.Join(root, "baton")); err != nil {
			return err
		}
		for _, command := range pinnedCommandNames {
			if err := os.Remove(filepath.Join(root, "commands", command)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return os.RemoveAll(filepath.Join(root, filepath.FromSlash(installVersionPath)))
}

func snapshotManifestEntries(targets []installTarget) ([]installManifestEntry, error) {
	var entries []installManifestEntry
	for _, target := range targets {
		rootInfo, err := os.Lstat(target.path)
		if os.IsNotExist(err) {
			entries = append(entries, installManifestEntry{Path: target.logical + "/", Kind: "absent"})
			continue
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, installManifestEntry{Path: target.logical + "/", Kind: "directory", Mode: rootInfo.Mode().Perm()})
		err = filepath.WalkDir(target.path, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if name == target.path {
				return nil
			}
			rel, err := filepath.Rel(target.path, name)
			if err != nil {
				return err
			}
			logicalPath := target.logical + "/" + filepath.ToSlash(rel)
			info, err := entry.Info()
			if err != nil {
				return err
			}
			item := installManifestEntry{Path: logicalPath, Mode: info.Mode().Perm()}
			if info.IsDir() {
				item.Kind = "directory"
			} else if info.Mode().IsRegular() {
				item.Kind = "file"
				contents, err := os.ReadFile(name)
				if err != nil {
					return err
				}
				digest := sha256.Sum256(contents)
				item.Digest = "sha256:" + hex.EncodeToString(digest[:])
			} else {
				return errors.New("unsupported snapshot node")
			}
			entries = append(entries, item)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func marshalInstallManifest(entries []installManifestEntry) []byte {
	var out bytes.Buffer
	out.WriteString(installManifestPrefix)
	out.WriteByte(0)
	for _, entry := range entries {
		fmt.Fprintf(&out, "%d:%s", len([]byte(entry.Path)), entry.Path)
		out.WriteByte(0)
		out.WriteString(entry.Kind)
		out.WriteByte(0)
		if entry.Kind == "absent" {
			out.WriteString("-")
		} else {
			fmt.Fprintf(&out, "%04o", entry.Mode.Perm())
		}
		out.WriteByte(0)
		if entry.Kind == "file" {
			out.WriteString(entry.Digest)
		} else {
			out.WriteString("-")
		}
		out.WriteByte(0)
	}
	out.WriteByte('\n')
	return out.Bytes()
}

func parseInstallManifest(raw []byte) ([]installManifestEntry, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return nil, errors.New("manifest final LF missing")
	}
	raw = raw[:len(raw)-1]
	prefix := append([]byte(installManifestPrefix), 0)
	if !bytes.HasPrefix(raw, prefix) {
		return nil, errors.New("manifest prefix mismatch")
	}
	reader := bufio.NewReader(bytes.NewReader(raw[len(prefix):]))
	var entries []installManifestEntry
	for reader.Buffered() > 0 || reader.Size() > 0 {
		lengthText, err := reader.ReadString(':')
		if err == io.EOF && lengthText == "" {
			break
		}
		if err != nil {
			return nil, err
		}
		lengthText = strings.TrimSuffix(lengthText, ":")
		if lengthText == "" || (len(lengthText) > 1 && lengthText[0] == '0') {
			return nil, errors.New("invalid manifest path length")
		}
		length, err := strconv.Atoi(lengthText)
		if err != nil || length <= 0 {
			return nil, errors.New("invalid manifest path length")
		}
		pathBytes := make([]byte, length)
		if _, err := io.ReadFull(reader, pathBytes); err != nil {
			return nil, err
		}
		if nul, err := reader.ReadByte(); err != nil || nul != 0 {
			return nil, errors.New("invalid manifest path terminator")
		}
		readField := func() (string, error) {
			field, err := reader.ReadString(0)
			return strings.TrimSuffix(field, "\x00"), err
		}
		kind, err := readField()
		if err != nil {
			return nil, err
		}
		modeText, err := readField()
		if err != nil {
			return nil, err
		}
		digest, err := readField()
		if err != nil {
			return nil, err
		}
		entry := installManifestEntry{Path: string(pathBytes), Kind: kind, Digest: digest}
		if !utf8.Valid(pathBytes) || (kind != "file" && kind != "directory" && kind != "absent") {
			return nil, errors.New("invalid manifest entry")
		}
		if kind == "absent" {
			if modeText != "-" || digest != "-" {
				return nil, errors.New("invalid absent manifest entry")
			}
		} else {
			mode, err := strconv.ParseUint(modeText, 8, 12)
			if err != nil || len(modeText) != 4 {
				return nil, errors.New("invalid manifest mode")
			}
			entry.Mode = os.FileMode(mode)
			if kind == "file" && (!strings.HasPrefix(digest, "sha256:") || len(digest) != 71) {
				return nil, errors.New("invalid manifest digest")
			}
			if kind == "directory" && digest != "-" {
				return nil, errors.New("invalid directory digest")
			}
		}
		entries = append(entries, entry)
		if reader.Buffered() == 0 {
			if _, err := reader.Peek(1); err == io.EOF {
				break
			}
		}
	}
	for i := range entries {
		if i > 0 && entries[i-1].Path >= entries[i].Path {
			return nil, errors.New("manifest paths are not unique byte-sorted")
		}
	}
	return entries, nil
}

func publishInstallRecovery(prepared *preparedInstall) error {
	root := prepared.roots.RecoveryRoot
	if err := callInstallFault(prepared.fault, "publish-before"); err != nil {
		return err
	}
	if err := revalidateUnreplacedInstallTargets(prepared, 0); err != nil {
		return err
	}
	if err := syncTree(prepared.recoveryStaging); err != nil {
		return err
	}
	transactionDir := filepath.Join(root, prepared.transaction)
	if _, err := os.Lstat(transactionDir); err == nil || !os.IsNotExist(err) {
		return newInstallError("operation-path-collision", []string{"transaction"}, transactionDir, errors.New("transaction path appeared"))
	}
	stagingInfo, ok := prepared.createdPaths[prepared.recoveryStaging]
	if !ok {
		return newInstallError("recovery-publication-failed", nil, prepared.recoveryStaging, errors.New("staging ownership missing"))
	}
	if err := validateOwnedInstallPath(prepared.recoveryStaging, stagingInfo, true); err != nil {
		return err
	}
	if err := os.Rename(prepared.recoveryStaging, transactionDir); err != nil {
		return err
	}
	delete(prepared.createdPaths, prepared.recoveryStaging)
	prepared.createdPaths[transactionDir] = stagingInfo
	for i := range prepared.targets {
		prepared.targets[i].snapshot = filepath.Join(transactionDir, "snapshots", prepared.targets[i].logical)
	}
	if err := syncDir(root); err != nil {
		return err
	}
	if err := revalidateUnreplacedInstallTargets(prepared, 0); err != nil {
		return err
	}
	sentinel := installSentinel{
		RecordVersion: 1, TransactionSHA256: "sha256:" + prepared.transaction,
		RecoveryDirectory: transactionDir,
	}
	for _, target := range prepared.targets {
		sentinel.Targets = append(sentinel.Targets, installSentinelTarget{LogicalRoot: target.logical, TargetPath: target.path, SnapshotPath: filepath.Join(transactionDir, "snapshots", target.logical)})
	}
	raw, err := marshalInstallSentinel(sentinel)
	if err != nil {
		return err
	}
	if err := atomicWriteInstallControl(prepared, filepath.Join(root, installRecoverySentinel), raw); err != nil {
		return err
	}
	loaded, loadedRaw, err := loadInstallSentinel(root)
	if err != nil || !bytes.Equal(raw, loadedRaw) {
		return newInstallError("recovery-publication-failed", nil, root, errors.New("sentinel reread differs"))
	}
	if _, err := validateInstallRecovery(prepared, loaded); err != nil {
		return err
	}
	if err := revalidateUnreplacedInstallTargets(prepared, 0); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "publish-after"); err != nil {
		return err
	}
	return nil
}

func marshalInstallSentinel(s installSentinel) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(s); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func marshalInstallOwnerIdentity(identity installOwnerIdentity) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(identity); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func loadInstallOwnerIdentity(path string) (installOwnerIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return installOwnerIdentity{}, errors.New("owner identity type or mode invalid")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return installOwnerIdentity{}, err
	}
	var identity installOwnerIdentity
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return installOwnerIdentity{}, err
	}
	canonical, err := marshalInstallOwnerIdentity(identity)
	if err != nil || !bytes.Equal(raw, canonical) {
		return installOwnerIdentity{}, errors.New("owner identity is not canonical")
	}
	return identity, nil
}

func marshalInstallStageIdentity(identity installStageIdentity) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(identity); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func loadInstallStageIdentity(path string) (installStageIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return installStageIdentity{}, errors.New("stage identity type or mode invalid")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return installStageIdentity{}, err
	}
	var identity installStageIdentity
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return installStageIdentity{}, err
	}
	canonical, err := marshalInstallStageIdentity(identity)
	if err != nil || !bytes.Equal(raw, canonical) {
		return installStageIdentity{}, errors.New("stage identity is not canonical")
	}
	return identity, nil
}

func validateInstallStage(root, transaction string, target installTarget) error {
	identity, err := loadInstallStageIdentity(filepath.Join(root, installStageIdentityName))
	if err != nil || identity.RecordVersion != 1 || identity.TransactionSHA256 != "sha256:"+transaction || identity.LogicalRoot != target.logical || identity.TargetPath != target.path {
		return newInstallError("stage-identity-invalid", []string{target.logical}, root, errors.New("stage owner identity invalid"))
	}
	return nil
}

func loadInstallSentinel(root string) (installSentinel, []byte, error) {
	path := filepath.Join(root, installRecoverySentinel)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return installSentinel{}, nil, errors.New("sentinel type or mode invalid")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return installSentinel{}, nil, err
	}
	var sentinel installSentinel
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sentinel); err != nil {
		return installSentinel{}, nil, err
	}
	canonical, err := marshalInstallSentinel(sentinel)
	if err != nil || !bytes.Equal(raw, canonical) {
		return installSentinel{}, nil, errors.New("sentinel is not canonical")
	}
	return sentinel, raw, nil
}

func recoverBatonInstall(prepared *preparedInstall) error {
	sentinel, _, err := loadInstallSentinel(prepared.roots.RecoveryRoot)
	if err != nil {
		return newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	manifest, err := validateInstallRecovery(prepared, sentinel)
	if err != nil {
		return err
	}
	unrestored := restoreInstallTargets(prepared, manifest)
	if len(unrestored) != 0 {
		if updateErr := updateInstallUnrestored(prepared, unrestored); updateErr != nil {
			return newInstallError("rollback-incomplete", append(unrestored, "recovery"), prepared.roots.RecoveryRoot, updateErr)
		}
		return newInstallError("rollback-incomplete", unrestored, prepared.roots.RecoveryRoot, errors.New("recovery incomplete"))
	}
	if err := cleanupInstallStageDebris(prepared); err != nil {
		return newInstallError("recovery-stage-cleanup-failed", []string{"recovery"}, prepared.roots.RecoveryRoot, err)
	}
	if err := retireInstallRecovery(prepared); err != nil {
		return newInstallError("recovery-retire-failed", []string{"recovery"}, prepared.roots.RecoveryRoot, err)
	}
	return nil
}

func validateInstallRecovery(prepared *preparedInstall, sentinel installSentinel) ([]installManifestEntry, error) {
	if err := reassertInstallIdentities(prepared, false); err != nil {
		return nil, err
	}
	if sentinel.RecordVersion != 1 || !strings.HasPrefix(sentinel.TransactionSHA256, "sha256:") {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("sentinel identity invalid"))
	}
	id := strings.TrimPrefix(sentinel.TransactionSHA256, "sha256:")
	if len(id) != 64 || filepath.Base(sentinel.RecoveryDirectory) != id || filepath.Dir(sentinel.RecoveryDirectory) != prepared.roots.RecoveryRoot {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("recovery directory invalid"))
	}
	decodedID, decodeErr := hex.DecodeString(id)
	if decodeErr != nil || hex.EncodeToString(decodedID) != id {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("transaction id invalid"))
	}
	prepared.transaction = id
	prepared.sentinelTemp = filepath.Join(prepared.roots.RecoveryRoot, installRecoverySentinel+".tmp-"+id)
	prepared.retiredPath = filepath.Join(filepath.Dir(prepared.roots.RecoveryRoot), ".baton-sync-retired-"+id)
	if err := validateRecoveryOwnerModes(prepared.roots.RecoveryRoot, sentinel.RecoveryDirectory); err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	if err := validateRecoveryControlInventory(prepared.roots.RecoveryRoot, sentinel.RecoveryDirectory); err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	if len(sentinel.Targets) != len(prepared.targets) {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("target inventory invalid"))
	}
	if !equalStrings(sentinel.UnrestoredPaths, uniqueSortedStrings(sentinel.UnrestoredPaths)) {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("unrestored paths are not unique byte-sorted"))
	}
	for i, target := range prepared.targets {
		record := sentinel.Targets[i]
		wantSnapshot := filepath.Join(sentinel.RecoveryDirectory, "snapshots", target.logical)
		if record.LogicalRoot != target.logical || record.TargetPath != target.path || record.SnapshotPath != wantSnapshot {
			return nil, newInstallError("recovery-invalid", []string{target.logical}, prepared.roots.RecoveryRoot, errors.New("target identity invalid"))
		}
		prepared.targets[i].snapshot = record.SnapshotPath
	}
	manifestRaw, err := os.ReadFile(filepath.Join(sentinel.RecoveryDirectory, installManifestName))
	if err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	digest := sha256.Sum256(manifestRaw)
	if "sha256:"+hex.EncodeToString(digest[:]) != sentinel.TransactionSHA256 {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("manifest digest mismatch"))
	}
	manifest, err := parseInstallManifest(manifestRaw)
	if err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	if err := validateInstallManifestPaths(manifest, prepared.targets); err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	owner, err := loadInstallOwnerIdentity(filepath.Join(sentinel.RecoveryDirectory, installIdentityName))
	if err != nil || owner.RecordVersion != 1 || !validInstallTransactionID(owner.OperationID) || owner.RecoveryRoot != prepared.roots.RecoveryRoot {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("owner identity invalid"))
	}
	if err := validateOwnerTargets(prepared, owner.Targets); err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	if err := validateSnapshotMaterial(prepared.targets, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func validateSnapshotMaterial(targets []installTarget, manifest []installManifestEntry) error {
	byRoot := manifestByRoot(manifest)
	for _, target := range targets {
		entries := byRoot[target.logical]
		if len(entries) == 0 {
			return newInstallError("recovery-invalid", []string{target.logical}, filepath.Dir(filepath.Dir(target.snapshot)), errors.New("manifest target missing"))
		}
		root := entries[0]
		if root.Path != target.logical+"/" {
			return newInstallError("recovery-invalid", []string{target.logical}, "", errors.New("manifest root missing"))
		}
		if root.Kind == "absent" {
			children, err := os.ReadDir(target.snapshot)
			if err != nil || len(children) != 0 {
				return newInstallError("recovery-invalid", []string{target.logical}, "", errors.New("absent snapshot contains material"))
			}
			continue
		}
		seen := make(map[string]struct{})
		err := filepath.WalkDir(target.snapshot, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, _ := filepath.Rel(target.snapshot, name)
			logical := target.logical + "/"
			if rel != "." {
				logical += filepath.ToSlash(rel)
			}
			seen[logical] = struct{}{}
			info, err := entry.Info()
			if err != nil || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return errors.New("snapshot node invalid")
			}
			return nil
		})
		if err != nil {
			return newInstallError("recovery-invalid", []string{target.logical}, "", err)
		}
		for _, item := range entries {
			if _, ok := seen[item.Path]; !ok {
				return newInstallError("recovery-invalid", []string{item.Path}, "", errors.New("snapshot entry missing"))
			}
			rel := strings.TrimPrefix(item.Path, target.logical+"/")
			materialPath := target.snapshot
			if rel != "" {
				materialPath = filepath.Join(target.snapshot, filepath.FromSlash(rel))
			}
			info, err := os.Lstat(materialPath)
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				return newInstallError("recovery-invalid", []string{item.Path}, "", errors.New("snapshot entry invalid"))
			}
			if item.Kind == "directory" {
				if !info.IsDir() || info.Mode().Perm() != 0o700 {
					return newInstallError("recovery-invalid", []string{item.Path}, "", errors.New("snapshot directory kind or mode differs"))
				}
				continue
			}
			if item.Kind == "file" {
				if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
					return newInstallError("recovery-invalid", []string{item.Path}, "", errors.New("snapshot file kind or mode differs"))
				}
				contents, err := os.ReadFile(materialPath)
				if err != nil {
					return err
				}
				digest := sha256.Sum256(contents)
				if "sha256:"+hex.EncodeToString(digest[:]) != item.Digest {
					return newInstallError("recovery-invalid", []string{item.Path}, "", errors.New("snapshot digest mismatch"))
				}
				continue
			}
			return newInstallError("recovery-invalid", []string{item.Path}, "", errors.New("snapshot manifest kind invalid"))
		}
		if len(seen) != len(entries) {
			return newInstallError("recovery-invalid", []string{target.logical}, "", errors.New("foreign snapshot material"))
		}
	}
	return nil
}

func restoreInstallTargets(prepared *preparedInstall, manifest []installManifestEntry) []string {
	byRoot := manifestByRoot(manifest)
	var unrestored []string
	for _, target := range prepared.targets {
		if err := callInstallFault(prepared.fault, "restore-before:"+target.logical); err != nil {
			unrestored = append(unrestored, target.logical)
			continue
		}
		if err := restoreInstallTarget(target, byRoot[target.logical]); err != nil {
			unrestored = append(unrestored, target.logical)
			continue
		}
		if err := callInstallFault(prepared.fault, "restore-after:"+target.logical); err != nil {
			unrestored = append(unrestored, target.logical)
		}
	}
	return uniqueSortedStrings(unrestored)
}

func restoreInstallTarget(target installTarget, entries []installManifestEntry) error {
	if len(entries) == 0 || entries[0].Path != target.logical+"/" {
		return errors.New("manifest root missing")
	}
	if err := os.RemoveAll(target.path); err != nil {
		return err
	}
	if entries[0].Kind == "absent" {
		return nil
	}
	if err := copyCompleteTree(target.snapshot, target.path, false, entries); err != nil {
		return err
	}
	return verifyRestoredTarget(target, entries)
}

func copyCompleteTree(source, dest string, ownerOnly bool, manifest []installManifestEntry) error {
	info, err := os.Lstat(source)
	if os.IsNotExist(err) {
		mode := chooseMode(ownerOnly, 0o755, true)
		if err := os.MkdirAll(dest, mode); err != nil {
			return err
		}
		return os.Chmod(dest, mode)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("source root is not a directory")
	}
	if err := os.MkdirAll(dest, chooseMode(ownerOnly, info.Mode().Perm(), true)); err != nil {
		return err
	}
	if err := os.Chmod(dest, chooseMode(ownerOnly, info.Mode().Perm(), true)); err != nil {
		return err
	}
	err = filepath.WalkDir(source, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == source {
			return nil
		}
		rel, _ := filepath.Rel(source, name)
		targetPath := filepath.Join(dest, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			mode := chooseMode(ownerOnly, info.Mode().Perm(), true)
			if err := os.MkdirAll(targetPath, mode); err != nil {
				return err
			}
			return os.Chmod(targetPath, mode)
		}
		if !info.Mode().IsRegular() {
			return errors.New("unsupported copy node")
		}
		contents, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		mode := chooseMode(ownerOnly, info.Mode().Perm(), false)
		if err := os.WriteFile(targetPath, contents, mode); err != nil {
			return err
		}
		return os.Chmod(targetPath, mode)
	})
	if err != nil {
		return err
	}
	if !ownerOnly && manifest != nil {
		for i := len(manifest) - 1; i >= 0; i-- {
			entry := manifest[i]
			if entry.Kind == "absent" {
				continue
			}
			rel := strings.TrimPrefix(entry.Path, strings.Split(entry.Path, "/")[0]+"/")
			path := dest
			if rel != "" {
				path = filepath.Join(dest, filepath.FromSlash(rel))
			}
			if err := os.Chmod(path, entry.Mode); err != nil {
				return err
			}
		}
	}
	return nil
}

func chooseMode(ownerOnly bool, original os.FileMode, directory bool) os.FileMode {
	if !ownerOnly {
		return original
	}
	if directory {
		return 0o700
	}
	return 0o600
}

func verifyRestoredTarget(target installTarget, entries []installManifestEntry) error {
	actualTargets := []installTarget{{logical: target.logical, path: target.path}}
	actual, err := snapshotManifestEntries(actualTargets)
	if err != nil {
		return err
	}
	if !bytes.Equal(marshalInstallManifest(actual), marshalInstallManifest(entries)) {
		return errors.New("restored target differs from manifest")
	}
	return nil
}

func replaceInstallRoot(prepared *preparedInstall, target installTarget) error {
	if err := validateInstallStage(target.stage, prepared.transaction, target); err != nil {
		return err
	}
	if err := os.RemoveAll(target.path); err != nil {
		return err
	}
	if err := os.Rename(target.stage, target.path); err != nil {
		return err
	}
	if err := validateInstallStage(target.path, prepared.transaction, target); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(target.path, installStageIdentityName)); err != nil {
		return err
	}
	return syncDir(filepath.Dir(target.path))
}

func verifyInstalledTarget(target installTarget, version []byte) error {
	drift, err := verifyTargetDrift(target, version)
	if err != nil {
		return err
	}
	if len(drift) != 0 {
		return newInstallError("install-verify-failed", drift, "", errors.New("installed tree differs"))
	}
	return nil
}

func updateInstallUnrestored(prepared *preparedInstall, paths []string) error {
	root := prepared.roots.RecoveryRoot
	if err := callInstallFault(prepared.fault, "unrestored-update-before"); err != nil {
		return err
	}
	sentinel, _, err := loadInstallSentinel(root)
	if err != nil {
		return err
	}
	if _, err := validateInstallRecovery(prepared, sentinel); err != nil {
		return err
	}
	sentinel.UnrestoredPaths = uniqueSortedStrings(paths)
	raw, err := marshalInstallSentinel(sentinel)
	if err != nil {
		return err
	}
	if err := atomicWriteInstallControl(prepared, filepath.Join(root, installRecoverySentinel), raw); err != nil {
		return err
	}
	loaded, loadedRaw, err := loadInstallSentinel(root)
	if err != nil || !bytes.Equal(raw, loadedRaw) || !equalStrings(loaded.UnrestoredPaths, sentinel.UnrestoredPaths) {
		return errors.New("durable unrestored update differs")
	}
	if err := callInstallFault(prepared.fault, "unrestored-update-after"); err != nil {
		return err
	}
	return nil
}

func retireInstallRecovery(prepared *preparedInstall) error {
	root := prepared.roots.RecoveryRoot
	sentinel, _, err := loadInstallSentinel(root)
	if err != nil {
		return err
	}
	if _, err := validateInstallRecovery(prepared, sentinel); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "retire-before"); err != nil {
		return err
	}
	retired := prepared.retiredPath
	if retired == "" {
		retired = filepath.Join(filepath.Dir(root), ".baton-sync-retired-"+prepared.transaction)
	}
	if _, err := os.Lstat(retired); err == nil || !os.IsNotExist(err) {
		return newInstallError("operation-path-collision", []string{"retired"}, retired, errors.New("retired path exists"))
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode().Perm() != 0o700 {
		return errors.New("recovery root identity invalid before retirement")
	}
	if err := os.Rename(root, retired); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(root)); err != nil {
		return err
	}
	if err := validateRelocatedInstallRecovery(prepared, retired); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "retire-after"); err != nil {
		return err
	}
	if err := removeValidatedRetiredInstall(prepared, retired, rootInfo); err != nil {
		return err
	}
	return syncDir(filepath.Dir(root))
}

func findRetiredInstallRecovery(prepared *preparedInstall) (string, error) {
	parent := filepath.Dir(prepared.roots.RecoveryRoot)
	entries, err := os.ReadDir(parent)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var found string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".baton-sync-retired-") {
			continue
		}
		id := strings.TrimPrefix(entry.Name(), ".baton-sync-retired-")
		if !validInstallTransactionID(id) || found != "" {
			return "", newInstallError("retired-debris-invalid", []string{"recovery"}, parent, errors.New("unidentified or duplicate retired debris"))
		}
		found = filepath.Join(parent, entry.Name())
	}
	if found != "" {
		if err := validateRelocatedInstallRecovery(prepared, found); err != nil {
			return "", newInstallError("retired-debris-invalid", []string{"recovery"}, found, err)
		}
	}
	return found, nil
}

func cleanupRetiredInstallRecovery(prepared *preparedInstall, retired string) error {
	info, err := os.Lstat(retired)
	if err != nil {
		return err
	}
	if err := validateRelocatedInstallRecovery(prepared, retired); err != nil {
		return err
	}
	return removeValidatedRetiredInstall(prepared, retired, info)
}

func removeValidatedRetiredInstall(prepared *preparedInstall, retired string, info fs.FileInfo) error {
	if err := validateOwnedInstallPath(retired, info, true); err != nil {
		return err
	}
	if err := validateRelocatedInstallRecovery(prepared, retired); err != nil {
		return err
	}
	if err := os.RemoveAll(retired); err != nil {
		return err
	}
	return syncDir(filepath.Dir(retired))
}

func cleanupIncompleteInstallRecovery(prepared *preparedInstall) error {
	root := prepared.roots.RecoveryRoot
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return newInstallError("recovery-unreadable", nil, root, err)
	}
	rootInfo, statErr := os.Lstat(root)
	if statErr != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm() != 0o700 {
		return newInstallError("recovery-debris-invalid", []string{"recovery"}, root, errors.New("recovery root type or mode invalid"))
	}
	if len(entries) == 0 {
		return nil
	}
	if len(entries) != 1 {
		return newInstallError("recovery-debris-invalid", []string{"recovery"}, root, errors.New("unidentified recovery debris"))
	}
	name := entries[0].Name()
	id := name
	staging := strings.HasPrefix(name, ".staging-")
	if staging {
		id = strings.TrimPrefix(name, ".staging-")
	}
	if !validInstallTransactionID(id) {
		return newInstallError("recovery-debris-invalid", []string{"recovery"}, root, errors.New("unidentified recovery debris"))
	}
	path := filepath.Join(root, name)
	if err := validateIncompleteInstallBundle(prepared, path, id, staging); err != nil {
		return newInstallError("recovery-debris-invalid", []string{"recovery"}, root, err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return newInstallError("recovery-debris-invalid", []string{"recovery"}, root, errors.New("debris identity invalid"))
	}
	if err := validateOwnedInstallPath(path, info, true); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return syncDir(root)
}

func validInstallTransactionID(id string) bool {
	if len(id) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(id)
	return err == nil && hex.EncodeToString(decoded) == id
}

func validateIncompleteInstallBundle(prepared *preparedInstall, path, id string, staging bool) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return errors.New("incomplete bundle root invalid")
	}
	if err := validateOwnerOnlyTree(path); err != nil {
		return err
	}
	owner, err := loadInstallOwnerIdentity(filepath.Join(path, installIdentityName))
	if err != nil || owner.RecordVersion != 1 || !validInstallTransactionID(owner.OperationID) || owner.RecoveryRoot != prepared.roots.RecoveryRoot || (staging && owner.OperationID != id) {
		return errors.New("incomplete bundle owner identity invalid")
	}
	if err := validateOwnerTargets(prepared, owner.Targets); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	allowed := map[string]bool{installIdentityName: true, installManifestName: true, "snapshots": true}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !allowed[entry.Name()] || seen[entry.Name()] {
			return errors.New("foreign incomplete bundle material")
		}
		seen[entry.Name()] = true
	}
	if snapshots, ok := seen["snapshots"]; ok && snapshots {
		snapshotEntries, err := os.ReadDir(filepath.Join(path, "snapshots"))
		if err != nil {
			return err
		}
		allowedRoots := map[string]bool{"agents_home": true, "claude_home": true, "codex_home": true}
		for _, entry := range snapshotEntries {
			if !allowedRoots[entry.Name()] || !entry.IsDir() {
				return errors.New("foreign incomplete snapshot material")
			}
		}
	}
	if staging {
		return nil
	}
	if !seen[installManifestName] || !seen["snapshots"] {
		return errors.New("orphan transaction is incomplete")
	}
	manifestRaw, err := os.ReadFile(filepath.Join(path, installManifestName))
	if err != nil {
		return err
	}
	manifest, err := parseInstallManifest(manifestRaw)
	if err != nil || validateInstallManifestPaths(manifest, prepared.targets) != nil {
		return errors.New("orphan transaction manifest invalid")
	}
	targets := append([]installTarget(nil), prepared.targets...)
	for i := range targets {
		targets[i].snapshot = filepath.Join(path, "snapshots", targets[i].logical)
	}
	return validateSnapshotMaterial(targets, manifest)
}

func validateRelocatedInstallRecovery(prepared *preparedInstall, retired string) error {
	base := filepath.Base(retired)
	id := strings.TrimPrefix(base, ".baton-sync-retired-")
	if !strings.HasPrefix(base, ".baton-sync-retired-") || !validInstallTransactionID(id) {
		return errors.New("retired path identity invalid")
	}
	sentinel, _, err := loadInstallSentinel(retired)
	if err != nil || sentinel.RecordVersion != 1 || sentinel.TransactionSHA256 != "sha256:"+id {
		return errors.New("retired sentinel invalid")
	}
	originalTransaction := filepath.Join(prepared.roots.RecoveryRoot, id)
	if sentinel.RecoveryDirectory != originalTransaction {
		return errors.New("retired recovery identity invalid")
	}
	if err := validateOwnerOnlyTree(retired); err != nil {
		return err
	}
	relocatedTransaction := filepath.Join(retired, id)
	if err := validateRecoveryControlInventory(retired, relocatedTransaction); err != nil {
		return err
	}
	owner, err := loadInstallOwnerIdentity(filepath.Join(relocatedTransaction, installIdentityName))
	if err != nil || owner.RecordVersion != 1 || !validInstallTransactionID(owner.OperationID) || owner.RecoveryRoot != prepared.roots.RecoveryRoot {
		return errors.New("retired owner identity invalid")
	}
	if err := validateOwnerTargets(prepared, owner.Targets); err != nil {
		return err
	}
	if len(owner.Targets) != len(sentinel.Targets) {
		return errors.New("retired target inventory differs")
	}
	for i := range owner.Targets {
		if owner.Targets[i].LogicalRoot != sentinel.Targets[i].LogicalRoot || owner.Targets[i].TargetPath != sentinel.Targets[i].TargetPath {
			return errors.New("retired target identity differs")
		}
	}
	manifestRaw, err := os.ReadFile(filepath.Join(relocatedTransaction, installManifestName))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(manifestRaw)
	if sentinel.TransactionSHA256 != "sha256:"+hex.EncodeToString(digest[:]) {
		return errors.New("retired manifest digest differs")
	}
	manifest, err := parseInstallManifest(manifestRaw)
	if err != nil || validateInstallManifestPaths(manifest, prepared.targets) != nil {
		return errors.New("retired manifest invalid")
	}
	targets := append([]installTarget(nil), prepared.targets...)
	for i := range targets {
		targets[i].snapshot = filepath.Join(relocatedTransaction, "snapshots", targets[i].logical)
	}
	return validateSnapshotMaterial(targets, manifest)
}

func validateOwnerTargets(prepared *preparedInstall, records []installSentinelTarget) error {
	if len(records) != len(prepared.targets) {
		return errors.New("owner target inventory differs")
	}
	for i, target := range prepared.targets {
		want := installSentinelTarget{
			LogicalRoot: target.logical, TargetPath: target.path,
		}
		if records[i] != want {
			return errors.New("owner target identity differs")
		}
	}
	return nil
}

func validateOwnerOnlyTree(root string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("owner-only tree contains unsafe node")
		}
		if info.IsDir() {
			if info.Mode().Perm() != 0o700 {
				return errors.New("owner-only directory mode differs")
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return errors.New("owner-only file type or mode differs")
		}
		return nil
	})
}

func validateRecoveryOwnerModes(root, transactionDir string) error {
	allowedRoot := map[string]struct{}{
		installRecoverySentinel:       {},
		filepath.Base(transactionDir): {},
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, ok := allowedRoot[entry.Name()]; !ok {
			return errors.New("foreign recovery-root material")
		}
	}
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe recovery node")
		}
		if info.IsDir() {
			if info.Mode().Perm() != 0o700 {
				return errors.New("recovery directory mode differs from 0700")
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return errors.New("recovery file mode differs from 0600")
		}
		return nil
	})
}

func validateRecoveryControlInventory(root, transactionDir string) error {
	if err := requireExactDirectoryEntries(root, map[string]bool{
		installRecoverySentinel:       false,
		filepath.Base(transactionDir): true,
	}); err != nil {
		return err
	}
	if err := requireExactDirectoryEntries(transactionDir, map[string]bool{
		installIdentityName: false,
		installManifestName: false,
		"snapshots":         true,
	}); err != nil {
		return err
	}
	return requireExactDirectoryEntries(filepath.Join(transactionDir, "snapshots"), map[string]bool{
		"agents_home": true,
		"claude_home": true,
		"codex_home":  true,
	})
}

func requireExactDirectoryEntries(root string, expected map[string]bool) error {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != len(expected) {
		return errors.New("recovery inventory differs")
	}
	for _, entry := range entries {
		wantDir, ok := expected[entry.Name()]
		if !ok {
			return errors.New("foreign recovery material")
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != wantDir || (!wantDir && !info.Mode().IsRegular()) {
			return errors.New("recovery entry kind differs")
		}
	}
	return nil
}

func validateOwnedInstallPath(name string, want fs.FileInfo, directory bool) error {
	info, err := os.Lstat(name)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return errors.New("owned path type changed")
	}
	if want == nil || !os.SameFile(want, info) {
		return errors.New("owned path identity changed")
	}
	return nil
}

func validateInstallManifestPaths(entries []installManifestEntry, targets []installTarget) error {
	allowed := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		allowed[target.logical] = struct{}{}
	}
	roots := make(map[string]installManifestEntry)
	for _, entry := range entries {
		root, rel, ok := strings.Cut(entry.Path, "/")
		if !ok {
			return errors.New("manifest logical path has no root separator")
		}
		if _, ok := allowed[root]; !ok {
			return errors.New("manifest logical root is foreign")
		}
		if rel == "" {
			if _, duplicate := roots[root]; duplicate || (entry.Kind != "directory" && entry.Kind != "absent") {
				return errors.New("manifest root entry is invalid")
			}
			roots[root] = entry
			continue
		}
		if err := validateManagedRelativePath(rel); err != nil || entry.Kind == "absent" {
			return errors.New("manifest descendant path is invalid")
		}
	}
	if len(roots) != len(targets) {
		return errors.New("manifest root inventory is incomplete")
	}
	for root, entry := range roots {
		if entry.Kind != "absent" {
			continue
		}
		prefix := root + "/"
		for _, candidate := range entries {
			if candidate.Path != prefix && strings.HasPrefix(candidate.Path, prefix) {
				return errors.New("absent manifest root has descendants")
			}
		}
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ensureOwnerOnlyDir(name string) error {
	if err := os.MkdirAll(name, 0o700); err != nil {
		return err
	}
	return chmodTreeOwnerOnly(name)
}

func chmodTreeOwnerOnly(root string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe recovery node")
		}
		if info.IsDir() {
			return os.Chmod(name, 0o700)
		}
		if info.Mode().IsRegular() {
			return os.Chmod(name, 0o600)
		}
		return errors.New("unsafe recovery node")
	})
}

func atomicWriteInstallControl(prepared *preparedInstall, name string, data []byte) error {
	tmp := prepared.sentinelTemp
	if tmp == "" || filepath.Dir(tmp) != filepath.Dir(name) {
		return errors.New("control temporary path identity missing")
	}
	if _, err := os.Lstat(tmp); err == nil || !os.IsNotExist(err) {
		return newInstallError("operation-path-collision", []string{"sentinel_temp"}, tmp, errors.New("control temporary path exists"))
	}
	if err := callInstallFault(prepared.fault, "control-write-before"); err != nil {
		return err
	}
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	if statErr != nil {
		file.Close()
		return statErr
	}
	prepared.createdPaths[tmp] = info
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = removeOwnedInstallPath(prepared, tmp)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "control-sync-before"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "control-sync-after"); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "control-rename-before"); err != nil {
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		return err
	}
	delete(prepared.createdPaths, tmp)
	keep = true
	if err := syncDir(filepath.Dir(name)); err != nil {
		return err
	}
	if err := callInstallFault(prepared.fault, "control-rename-after"); err != nil {
		return err
	}
	contents, err := os.ReadFile(name)
	controlInfo, statErr := os.Lstat(name)
	if err != nil || statErr != nil || !controlInfo.Mode().IsRegular() || controlInfo.Mode().Perm() != 0o600 || !bytes.Equal(contents, data) {
		return errors.New("control file durable reread differs")
	}
	return nil
}

func syncTree(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, name)
			return nil
		}
		file, err := os.OpenFile(name, os.O_RDONLY, 0)
		if err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	})
	if err != nil {
		return err
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err := syncDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func syncDir(name string) error {
	dir, err := os.Open(name)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func callInstallFault(fault InstallFault, point string) error {
	if fault == nil {
		return nil
	}
	return fault(point)
}

func cleanupInstallStages(prepared *preparedInstall) {
	for _, target := range prepared.targets {
		_ = removeOwnedInstallPath(prepared, target.stage)
	}
	_ = removeOwnedInstallPath(prepared, prepared.sentinelTemp)
}

func cleanupInstallStageDebris(prepared *preparedInstall) error {
	seenParents := make(map[string]struct{})
	for _, target := range prepared.targets {
		parent := filepath.Dir(target.path)
		if _, ok := seenParents[parent]; ok {
			continue
		}
		seenParents[parent] = struct{}{}
		entries, err := os.ReadDir(parent)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".sworn-baton-stage-") {
				continue
			}
			stage := filepath.Join(parent, entry.Name())
			info, err := os.Lstat(stage)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return newInstallError("stage-debris-invalid", []string{"stage"}, stage, errors.New("unidentified external stage debris"))
			}
			identity, err := loadInstallStageIdentity(filepath.Join(stage, installStageIdentityName))
			if err != nil || identity.RecordVersion != 1 || !strings.HasPrefix(identity.TransactionSHA256, "sha256:") {
				return newInstallError("stage-debris-invalid", []string{"stage"}, stage, errors.New("external stage owner identity invalid"))
			}
			transaction := strings.TrimPrefix(identity.TransactionSHA256, "sha256:")
			if !validInstallTransactionID(transaction) {
				return newInstallError("stage-debris-invalid", []string{"stage"}, stage, errors.New("external stage transaction invalid"))
			}
			var matched *installTarget
			for i := range prepared.targets {
				candidate := &prepared.targets[i]
				if candidate.logical == identity.LogicalRoot && candidate.path == identity.TargetPath {
					matched = candidate
					break
				}
			}
			if matched == nil || entry.Name() != ".sworn-baton-stage-"+transaction+"-"+matched.logical {
				return newInstallError("stage-debris-invalid", []string{"stage"}, stage, errors.New("external stage path identity differs"))
			}
			if err := scanInstallTarget(stage); err != nil {
				return newInstallError("stage-debris-invalid", []string{"stage"}, stage, err)
			}
			if err := validateInstallStage(stage, transaction, *matched); err != nil {
				return err
			}
			if err := os.RemoveAll(stage); err != nil {
				return err
			}
			if err := syncDir(parent); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeOwnedInstallPath(prepared *preparedInstall, name string) error {
	if name == "" {
		return nil
	}
	want, ok := prepared.createdPaths[name]
	if !ok {
		return nil
	}
	info, err := os.Lstat(name)
	if os.IsNotExist(err) {
		delete(prepared.createdPaths, name)
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateOwnedInstallPath(name, want, info.IsDir()); err != nil {
		return err
	}
	if info.IsDir() {
		err = os.RemoveAll(name)
	} else {
		err = os.Remove(name)
	}
	if err == nil {
		delete(prepared.createdPaths, name)
	}
	return err
}

func manifestByRoot(entries []installManifestEntry) map[string][]installManifestEntry {
	result := make(map[string][]installManifestEntry)
	for _, entry := range entries {
		root, _, _ := strings.Cut(entry.Path, "/")
		result[root] = append(result[root], entry)
	}
	return result
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
