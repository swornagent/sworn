package executor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeTree is an opaque, digest- and identity-bound source capability for
// the tree mounted at /usr. Its host path never enters a receipt or journal.
type RuntimeTree struct {
	root         string
	digest       string
	maximumBytes uint64
	identity     os.FileInfo
}

func (runtime RuntimeTree) Digest() string { return runtime.digest }

// NewRuntimeTree binds an explicit source to the digest and ceiling
// required by RunContentBound. Only the privately staged execution copy is
// measured as authoritative.
func NewRuntimeTree(source, digest string, maximumBytes uint64) (RuntimeTree, error) {
	if maximumBytes == 0 || !validDigest(digest) {
		return RuntimeTree{}, errors.New("content runtime requires a digest and byte ceiling")
	}
	if err := validateAbsoluteDirectory(source, "content runtime"); err != nil {
		return RuntimeTree{}, err
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return RuntimeTree{}, fmt.Errorf("resolve content runtime: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return RuntimeTree{}, errors.New("content runtime source must be a directory")
	}
	if beneathPath("/usr", resolved) || beneathPath(resolved, "/usr") {
		return RuntimeTree{}, errors.New("host /usr cannot become a content runtime source")
	}
	return RuntimeTree{root: resolved, digest: digest, maximumBytes: maximumBytes, identity: info}, nil
}

func beneathPath(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validateRuntimeSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.Type()&os.ModeSymlink == 0 {
			return walkErr
		}
		target, err := os.Readlink(path)
		if err != nil {
			return fmt.Errorf("read content runtime symlink: %w", err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || !runtimeSymlinkStaysInside(relative, target) {
			return fmt.Errorf("content runtime symlink %q escapes the mounted /usr tree", relative)
		}
		return nil
	})
}

func runtimeSymlinkStaysInside(path, target string) bool {
	if filepath.IsAbs(target) {
		for _, prefix := range []string{"/usr", "/bin", "/lib", "/lib64"} {
			if target == prefix || strings.HasPrefix(target, prefix+"/") {
				target = strings.TrimPrefix(target, prefix)
				target = strings.TrimPrefix(target, "/")
				return !strings.HasPrefix(filepath.Clean(target), "..")
			}
		}
		return false
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(path), target))
	return resolved != ".." && !strings.HasPrefix(resolved, ".."+string(filepath.Separator))
}
