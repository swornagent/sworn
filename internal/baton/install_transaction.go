package baton

import (
	"bufio"
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
	"unicode/utf8"
)

const (
	installRecoverySentinel = "rollback-incomplete.json"
	installManifestName     = "manifest.bin"
	installManifestPrefix   = "sworn-baton-sync-rollback-v1"
	installVersionPath      = ".sworn-baton/VERSION"
)

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

type installSentinelTarget struct {
	LogicalRoot  string `json:"logical_root"`
	TargetPath   string `json:"target_path"`
	SnapshotPath string `json:"snapshot_path"`
}

type preparedInstall struct {
	roots       InstallRoots
	targets     []installTarget
	manifest    []installManifestEntry
	manifestRaw []byte
	transaction string
	fault       InstallFault
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
	return installDrift(prepared.targets, opts.Version)
}

// SyncBatonInstall repairs all three logical roots in one rollback-protected
// transaction. Sentinel presence always routes to recovery-only restoration.
func SyncBatonInstall(opts InstallOpts) (*InstallResult, error) {
	prepared, err := prepareInstall(opts, true)
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
	if err := cleanupIncompleteInstallRecovery(prepared.roots.RecoveryRoot); err != nil {
		return nil, err
	}

	drift, err := installDrift(prepared.targets, opts.Version)
	if err != nil {
		return nil, err
	}
	if len(drift) == 0 {
		if err := cleanupRetiredInstallRecovery(prepared.roots.RecoveryRoot); err != nil {
			return nil, err
		}
		return &InstallResult{State: InstallAlreadyExact}, nil
	}

	if err := stageDesiredInstall(prepared.targets, opts.Version); err != nil {
		cleanupInstallStages(prepared.targets)
		return nil, err
	}
	defer cleanupInstallStages(prepared.targets)

	if err := publishInstallRecovery(prepared); err != nil {
		return nil, err
	}

	var applyErr error
	for i := range prepared.targets {
		target := &prepared.targets[i]
		if err := callInstallFault(prepared.fault, "replace-before:"+target.logical); err != nil {
			applyErr = err
			break
		}
		if err := replaceInstallRoot(*target); err != nil {
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
		unrestored := restoreInstallTargets(prepared, prepared.manifest)
		if len(unrestored) != 0 {
			_ = updateInstallUnrestored(prepared.roots.RecoveryRoot, unrestored)
			return nil, newInstallError("rollback-incomplete", unrestored, prepared.roots.RecoveryRoot, applyErr)
		}
		if retireErr := retireInstallRecovery(prepared.roots.RecoveryRoot, prepared.fault); retireErr != nil {
			return nil, newInstallError("rollback-incomplete", []string{"recovery"}, prepared.roots.RecoveryRoot, retireErr)
		}
		return nil, newInstallError("repair-failed-restored", drift, "", applyErr)
	}

	if err := retireInstallRecovery(prepared.roots.RecoveryRoot, prepared.fault); err != nil {
		return nil, newInstallError("recovery-retire-failed", []string{"recovery"}, prepared.roots.RecoveryRoot, err)
	}
	return &InstallResult{State: InstallRepaired, Changed: drift}, nil
}

func prepareInstall(opts InstallOpts, needManifest bool) (*preparedInstall, error) {
	if len(opts.Version) == 0 {
		return nil, newInstallError("version-invalid", nil, "", errors.New("empty VERSION sentinel"))
	}
	roots, err := resolveInstallRoots(opts.Roots)
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
		if err := scanInstallTarget(targets[i].path); err != nil {
			return nil, err
		}
	}
	prepared := &preparedInstall{roots: roots, targets: targets, fault: opts.Fault}
	if !needManifest {
		return prepared, nil
	}
	manifest, err := snapshotManifestEntries(targets)
	if err != nil {
		return nil, err
	}
	prepared.manifest = manifest
	prepared.manifestRaw = marshalInstallManifest(manifest)
	digest := sha256.Sum256(prepared.manifestRaw)
	prepared.transaction = hex.EncodeToString(digest[:])
	transactionDir := filepath.Join(roots.RecoveryRoot, prepared.transaction)
	for i := range prepared.targets {
		prepared.targets[i].snapshot = filepath.Join(transactionDir, "snapshots", prepared.targets[i].logical)
		prepared.targets[i].stage = filepath.Join(filepath.Dir(prepared.targets[i].path), ".sworn-baton-stage-"+prepared.transaction[:16]+"-"+prepared.targets[i].logical)
	}
	return prepared, nil
}

func resolveInstallRoots(input InstallRoots) (InstallRoots, error) {
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
	for _, value := range values {
		path, info, err := resolvePhysicalNoSymlink(value.path)
		if err != nil {
			return InstallRoots{}, newInstallError("unsafe-root", []string{value.logical}, "", err)
		}
		resolved[value.logical] = path
		if info != nil {
			infos[value.logical] = info
		}
	}
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			a, b := values[i].logical, values[j].logical
			if pathsOverlap(resolved[a], resolved[b]) || (infos[a] != nil && infos[b] != nil && os.SameFile(infos[a], infos[b])) {
				return InstallRoots{}, newInstallError("unsafe-root-topology", []string{a, b}, "", errors.New("roots overlap or alias"))
			}
		}
	}
	return InstallRoots{
		AgentsHome: resolved["agents_home"], CodexHome: resolved["codex_home"],
		ClaudeHome: resolved["claude_home"], RecoveryRoot: resolved["recovery"],
	}, nil
}

func resolvePhysicalNoSymlink(name string) (string, fs.FileInfo, error) {
	if name == "" || !utf8.ValidString(name) {
		return "", nil, errors.New("path is empty or invalid UTF-8")
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", nil, err
	}
	abs = filepath.Clean(abs)
	volume := filepath.VolumeName(abs)
	rest := strings.TrimPrefix(abs, volume)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(rest, string(filepath.Separator)), string(filepath.Separator))
	var finalInfo fs.FileInfo
	for i, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			for _, missing := range parts[i+1:] {
				if missing != "" {
					current = filepath.Join(current, missing)
				}
			}
			return filepath.Clean(current), nil, nil
		}
		if statErr != nil {
			return "", nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("symlink component: %s", current)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", nil, fmt.Errorf("non-directory component: %s", current)
		}
		finalInfo = info
	}
	if finalInfo != nil && !finalInfo.IsDir() {
		return "", nil, fmt.Errorf("root is not a directory: %s", abs)
	}
	return abs, finalInfo, nil
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

func stageDesiredInstall(targets []installTarget, version []byte) error {
	for i := range targets {
		target := &targets[i]
		_ = os.RemoveAll(target.stage)
		if err := copyCompleteTree(target.path, target.stage, false, nil); err != nil {
			return newInstallError("stage-failed", []string{target.logical}, "", err)
		}
		rootMode := os.FileMode(0o755)
		if info, err := os.Lstat(target.path); err == nil {
			rootMode = info.Mode().Perm()
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
		stageTarget := *target
		stageTarget.path = target.stage
		if err := verifyInstalledTarget(stageTarget, version); err != nil {
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
	return out.Bytes()
}

func parseInstallManifest(raw []byte) ([]installManifestEntry, error) {
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
	if err := ensureOwnerOnlyDir(root); err != nil {
		return newInstallError("recovery-publication-failed", nil, root, err)
	}
	if err := callInstallFault(prepared.fault, "publish-before"); err != nil {
		return err
	}
	staging := filepath.Join(root, ".staging-"+prepared.transaction)
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(filepath.Join(staging, "snapshots"), 0o700); err != nil {
		return err
	}
	if err := chmodTreeOwnerOnly(staging); err != nil {
		return err
	}
	for _, target := range prepared.targets {
		dest := filepath.Join(staging, "snapshots", target.logical)
		if err := copyCompleteTree(target.path, dest, true, nil); err != nil {
			return err
		}
	}
	manifestPath := filepath.Join(staging, installManifestName)
	if err := os.WriteFile(manifestPath, prepared.manifestRaw, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(manifestPath, 0o600); err != nil {
		return err
	}
	if err := syncTree(staging); err != nil {
		return err
	}
	transactionDir := filepath.Join(root, prepared.transaction)
	if err := os.Rename(staging, transactionDir); err != nil {
		return err
	}
	if err := syncDir(root); err != nil {
		return err
	}
	sentinel := installSentinel{RecordVersion: 1, TransactionSHA256: "sha256:" + prepared.transaction, RecoveryDirectory: transactionDir}
	for _, target := range prepared.targets {
		sentinel.Targets = append(sentinel.Targets, installSentinelTarget{LogicalRoot: target.logical, TargetPath: target.path, SnapshotPath: filepath.Join(transactionDir, "snapshots", target.logical)})
	}
	raw, err := marshalInstallSentinel(sentinel)
	if err != nil {
		return err
	}
	if err := atomicWriteInstallControl(filepath.Join(root, installRecoverySentinel), raw); err != nil {
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

func loadInstallSentinel(root string) (installSentinel, []byte, error) {
	path := filepath.Join(root, installRecoverySentinel)
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
		_ = updateInstallUnrestored(prepared.roots.RecoveryRoot, unrestored)
		return newInstallError("rollback-incomplete", unrestored, prepared.roots.RecoveryRoot, errors.New("recovery incomplete"))
	}
	if err := retireInstallRecovery(prepared.roots.RecoveryRoot, prepared.fault); err != nil {
		return newInstallError("recovery-retire-failed", []string{"recovery"}, prepared.roots.RecoveryRoot, err)
	}
	return nil
}

func validateInstallRecovery(prepared *preparedInstall, sentinel installSentinel) ([]installManifestEntry, error) {
	if sentinel.RecordVersion != 1 || !strings.HasPrefix(sentinel.TransactionSHA256, "sha256:") {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("sentinel identity invalid"))
	}
	id := strings.TrimPrefix(sentinel.TransactionSHA256, "sha256:")
	if len(id) != 64 || filepath.Base(sentinel.RecoveryDirectory) != id || filepath.Dir(sentinel.RecoveryDirectory) != prepared.roots.RecoveryRoot {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("recovery directory invalid"))
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
	if hex.EncodeToString(digest[:]) != id {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, errors.New("manifest digest mismatch"))
	}
	manifest, err := parseInstallManifest(manifestRaw)
	if err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	if err := validateInstallManifestPaths(manifest, prepared.targets); err != nil {
		return nil, newInstallError("recovery-invalid", nil, prepared.roots.RecoveryRoot, err)
	}
	if err := validateRecoveryOwnerModes(prepared.roots.RecoveryRoot, sentinel.RecoveryDirectory); err != nil {
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
			if item.Kind == "file" {
				rel := strings.TrimPrefix(item.Path, target.logical+"/")
				contents, err := os.ReadFile(filepath.Join(target.snapshot, filepath.FromSlash(rel)))
				if err != nil {
					return err
				}
				digest := sha256.Sum256(contents)
				if "sha256:"+hex.EncodeToString(digest[:]) != item.Digest {
					return newInstallError("recovery-invalid", []string{item.Path}, "", errors.New("snapshot digest mismatch"))
				}
			}
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

func replaceInstallRoot(target installTarget) error {
	if err := os.RemoveAll(target.path); err != nil {
		return err
	}
	if err := os.Rename(target.stage, target.path); err != nil {
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

func updateInstallUnrestored(root string, paths []string) error {
	sentinel, _, err := loadInstallSentinel(root)
	if err != nil {
		return err
	}
	sentinel.UnrestoredPaths = uniqueSortedStrings(paths)
	raw, err := marshalInstallSentinel(sentinel)
	if err != nil {
		return err
	}
	return atomicWriteInstallControl(filepath.Join(root, installRecoverySentinel), raw)
}

func retireInstallRecovery(root string, fault InstallFault) error {
	if err := callInstallFault(fault, "retire-before"); err != nil {
		return err
	}
	parent := filepath.Dir(root)
	retired := filepath.Join(parent, ".baton-sync-retired-"+strings.Repeat("0", 16))
	_ = os.RemoveAll(retired)
	if err := os.Rename(root, retired); err != nil {
		return err
	}
	if err := syncDir(parent); err != nil {
		return err
	}
	if err := callInstallFault(fault, "retire-after"); err != nil {
		return err
	}
	if err := os.RemoveAll(retired); err != nil {
		return err
	}
	return syncDir(parent)
}

func cleanupRetiredInstallRecovery(root string) error {
	parent := filepath.Dir(root)
	entries, err := os.ReadDir(parent)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".baton-sync-retired-") {
			if err := os.RemoveAll(filepath.Join(parent, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanupIncompleteInstallRecovery(root string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return newInstallError("recovery-unreadable", nil, root, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		isStaging := strings.HasPrefix(name, ".staging-") && len(strings.TrimPrefix(name, ".staging-")) == 64
		isTransaction := len(name) == 64
		if isTransaction {
			_, decodeErr := hex.DecodeString(name)
			isTransaction = decodeErr == nil
		}
		if !isStaging && !isTransaction {
			return newInstallError("recovery-debris-invalid", []string{"recovery"}, root, errors.New("foreign recovery material"))
		}
	}
	if err := os.RemoveAll(root); err != nil {
		return newInstallError("recovery-debris-invalid", []string{"recovery"}, root, err)
	}
	return syncDir(filepath.Dir(root))
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

func atomicWriteInstallControl(name string, data []byte) error {
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	file, err := os.OpenFile(tmp, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		return err
	}
	return syncDir(filepath.Dir(name))
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

func cleanupInstallStages(targets []installTarget) {
	for _, target := range targets {
		_ = os.RemoveAll(target.stage)
	}
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
