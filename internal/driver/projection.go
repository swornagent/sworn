package driver

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	InputProjectionPrefix = ".sworn-inputs/v1/"
	MaxInputFileBytes     = 1_048_576
	MaxInputTotalBytes    = 8_388_608
)

type InputContent struct {
	Input Input
	Bytes []byte
}

type InputProjection struct {
	root string
}

func StageInputProjection(workspace string, requestInputs []Input, contents []InputContent) (*InputProjection, error) {
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
	reserved := filepath.Join(workspace, ".sworn-inputs")
	if _, err := os.Lstat(reserved); err == nil || !os.IsNotExist(err) {
		return nil, fail("INPUT_PROJECTION_CONFLICT")
	}
	if mountConflict(reserved) {
		return nil, fail("INPUT_PROJECTION_CONFLICT")
	}
	if len(requestInputs) > MaxInputs {
		return nil, fail("RESOURCE_LIMIT")
	}
	if len(requestInputs) != len(contents) {
		return nil, fail("INPUT_BINDING_MISMATCH")
	}
	root, err := os.MkdirTemp("", "sworn-inputs-v1-")
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
	var total int
	for index, expected := range requestInputs {
		content := contents[index]
		if content.Input != expected {
			return nil, fail("INPUT_BINDING_MISMATCH")
		}
		if !strings.HasPrefix(expected.Path, InputProjectionPrefix) {
			return nil, fail("INVALID_PRODUCTION_INPUT_PATH")
		}
		relative := strings.TrimPrefix(expected.Path, InputProjectionPrefix)
		if err := validateRepositoryPath(relative); err != nil {
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
		target := filepath.Join(root, filepath.FromSlash(relative))
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

func mountConflict(target string) bool {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return true
	}
	defer file.Close()
	cleanTarget := filepath.Clean(target)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			return true
		}
		mountpoint := unescapeMountField(fields[4])
		if mountpoint == cleanTarget || strings.HasPrefix(mountpoint, cleanTarget+string(filepath.Separator)) {
			return true
		}
	}
	return scanner.Err() != nil
}

func unescapeMountField(value string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(value)
}

type manifestEntry struct {
	Path   string
	Mode   string
	Size   int64
	Digest string
	Target string
}

type workspaceManifest []manifestEntry

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
