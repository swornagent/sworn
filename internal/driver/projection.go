package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/gitx"
)

const (
	MaxInputFileBytes  = 1_048_576
	MaxInputTotalBytes = 8_388_608
)

type InputContent struct {
	Input Input
	Bytes []byte
}
type InputProjection struct {
	root string
}

// StageInputProjection stages request input bytes read-only beneath a fresh
// temp root. The optional reserved argument is the engine-computed set of
// workspace-relative names that must never be admitted as an input path or
// escaped through a workspace symlink; absent, the fixed default names apply.
func StageInputProjection(
	workspace string,
	requestInputs []Input,
	contents []InputContent,
	reserved ...[]string,
) (*InputProjection, error) {
	if err := validateWorkspace(Workspace{Path: workspace, Access: ReadOnly}); err != nil {
		return nil, err
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return nil, fail("INVALID_WORKSPACE")
	}
	resolved, err := filepath.EvalSymlinks(workspace)
	if err != nil || resolved != workspace {
		return nil, fail("INVALID_WORKSPACE")
	}
	if err := validateWorkspaceBoundary(workspace, reserved...); err != nil {
		return nil, err
	}
	if len(requestInputs) > MaxInputs {
		return nil, fail("RESOURCE_LIMIT")
	}
	if len(requestInputs) != len(contents) {
		return nil, fail("INPUT_BINDING_MISMATCH")
	}
	temp, err := tempRoot()
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(temp, "sworn-inputs-v1-")
	if err != nil {
		return nil, fail("INPUT_STAGE_FAILED")
	}
	projection := &InputProjection{root: root}
	ok := false
	defer func() {
		if !ok {
			_ = projection.Close()
		}
	}()
	var reservedNames []string
	if len(reserved) != 0 {
		reservedNames = reserved[0]
	}
	var total int
	for index, expected := range requestInputs {
		content := contents[index]
		if content.Input != expected {
			return nil, fail("INPUT_BINDING_MISMATCH")
		}
		if err := validateRepositoryPath(expected.Path, reservedNames); err != nil {
			return nil, fail("INVALID_PRODUCTION_INPUT_PATH")
		}
		if len(content.Bytes) > MaxInputFileBytes {
			return nil, fail("RESOURCE_LIMIT")
		}
		total += len(content.Bytes)
		if total > MaxInputTotalBytes {
			return nil, fail("RESOURCE_LIMIT")
		}
		if Digest(content.Bytes) != expected.Digest {
			return nil, fail("INPUT_BINDING_MISMATCH")
		}
		target := filepath.Join(root, filepath.FromSlash(expected.Path))
		if !pathBeneath(root, target) {
			return nil, fail("INVALID_PRODUCTION_INPUT_PATH")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, fail("INPUT_STAGE_FAILED")
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
		if err != nil {
			return nil, fail("INPUT_STAGE_FAILED")
		}
		_, writeErr := file.Write(content.Bytes)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return nil, fail("INPUT_STAGE_FAILED")
		}
	}
	if err := projection.validate(); err != nil {
		return nil, err
	}
	ok = true
	return projection, nil
}
func (projection *InputProjection) Root() string {
	if projection == nil {
		return ""
	}
	return projection.root
}
func (projection *InputProjection) Close() error {
	if projection == nil || projection.root == "" {
		return nil
	}
	root := projection.root
	projection.root = ""
	if err := os.RemoveAll(root); err != nil {
		return fail("INPUT_CLEANUP_FAILED")
	}
	return nil
}
func (projection *InputProjection) validate() error {
	if projection == nil || projection.root == "" {
		return fail("INVALID_PROJECTION")
	}
	return filepath.WalkDir(projection.root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fail("INVALID_PROJECTION")
		}
		info, err := entry.Info()
		if err != nil {
			return fail("INVALID_PROJECTION")
		}
		if entry.IsDir() {
			if info.Mode().Perm()&0o077 != 0 {
				return fail("INVALID_PROJECTION")
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
			info.Mode().Perm()&0o222 != 0 {
			return fail("INVALID_PROJECTION")
		}
		return nil
	})
}
func pathBeneath(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func validateWorkspaceBoundary(root string, reserved ...[]string) error {
	reservedNames := gitx.ReservedNames(gitx.DefaultProjectConfig())
	if len(reserved) != 0 && len(reserved[0]) != 0 {
		reservedNames = reserved[0]
	}
	reservedSet := make(map[string]bool, len(reservedNames)+1)
	for _, name := range reservedNames {
		reservedSet[name] = true
	}
	reservedSet["sworn/inputs"] = true
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fail("WORKSPACE_INSPECTION_FAILED")
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(name)
		if err != nil || filepath.IsAbs(target) {
			return fail("UNSAFE_WORKSPACE_SYMLINK")
		}
		resolved, err := filepath.EvalSymlinks(name)
		if err != nil || !pathBeneath(root, resolved) {
			return fail("UNSAFE_WORKSPACE_SYMLINK")
		}
		relative, err := filepath.Rel(root, resolved)
		if err != nil {
			return fail("UNSAFE_WORKSPACE_SYMLINK")
		}
		first := strings.Split(filepath.ToSlash(relative), "/")[0]
		if reservedSet[first] {
			return fail("UNSAFE_WORKSPACE_SYMLINK")
		}
		return nil
	})
}

type manifestEntry struct {
	Path, Mode, Digest, Target string
	Size                       int64
}
type workspaceManifest []manifestEntry

// captureWorkspaceManifest is persistent-delta evidence, not a transient write
// audit: equal snapshots cannot disprove an unrelated host mutate/restore.
func captureWorkspaceManifest(root string) (workspaceManifest, error) {
	var entries workspaceManifest
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fail("WORKSPACE_INSPECTION_FAILED")
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return fail("WORKSPACE_INSPECTION_FAILED")
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fail("WORKSPACE_INSPECTION_FAILED")
		}
		item := manifestEntry{
			Path: filepath.ToSlash(relative),
			Mode: info.Mode().String(),
			Size: info.Size(),
		}
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			item.Target, err = os.Readlink(name)
			if err != nil {
				return fail("WORKSPACE_INSPECTION_FAILED")
			}
		case info.Mode().IsRegular():
			file, err := os.Open(name)
			if err != nil {
				return fail("WORKSPACE_INSPECTION_FAILED")
			}
			digest, digestErr := streamDigest(file)
			closeErr := file.Close()
			if digestErr != nil || closeErr != nil {
				return fail("WORKSPACE_INSPECTION_FAILED")
			}
			item.Digest = digest
		}
		entries = append(entries, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}
func equalManifest(left, right workspaceManifest) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
func executableDigest(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", fail("INVALID_EXECUTABLE")
	}
	defer file.Close()
	digest, err := streamDigest(file)
	if err != nil {
		return "", fail("INVALID_EXECUTABLE")
	}
	return digest, nil
}
func streamDigest(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
