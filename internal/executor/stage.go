//go:build linux

package executor

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"
)

const workspaceManifestVersion = "sworn-workspace-manifest-v1"

// MeasureWorkspace returns the deterministic manifest digest used to pin a
// plain workspace before execution. Timestamps and ownership are excluded;
// relative paths, types, permissions, symlink targets, and regular bytes bind.
func MeasureWorkspace(ctx context.Context, source string, maximumBytes uint64) (string, uint64, error) {
	return walkWorkspace(ctx, source, "", maximumBytes)
}

func stageWorkspace(
	ctx context.Context,
	source, destination string,
	maximumBytes uint64,
) (string, uint64, error) {
	if err := os.Mkdir(destination, 0o700); err != nil {
		return "", 0, fmt.Errorf("create staged workspace: %w", err)
	}
	return walkWorkspace(ctx, source, destination, maximumBytes)
}

func walkWorkspace(
	ctx context.Context,
	source, destination string,
	maximumBytes uint64,
) (string, uint64, error) {
	if maximumBytes == 0 {
		return "", 0, errors.New("workspace byte ceiling is required")
	}
	sourceRoot, err := os.OpenRoot(source)
	if err != nil {
		return "", 0, fmt.Errorf("open workspace root: %w", err)
	}
	defer sourceRoot.Close() //nolint:errcheck
	var destinationRoot *os.Root
	if destination != "" {
		destinationRoot, err = os.OpenRoot(destination)
		if err != nil {
			return "", 0, fmt.Errorf("open staged workspace root: %w", err)
		}
		defer destinationRoot.Close() //nolint:errcheck
	}

	hasher := sha256.New()
	bindFrame(hasher, []byte(workspaceManifestVersion))
	var total uint64
	var entries uint64
	type stagedDirectory struct {
		path string
		mode os.FileMode
	}
	var directories []stagedDirectory
	err = fs.WalkDir(sourceRoot.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		entries++
		if entries > maximumWorkspaceEntries {
			return fmt.Errorf("workspace exceeds %d-entry ceiling", maximumWorkspaceEntries)
		}
		if !validStagedPath(path) {
			return fmt.Errorf("workspace contains invalid path %q", path)
		}
		info, err := sourceRoot.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect workspace path %q: %w", path, err)
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			bindFrame(hasher, []byte("directory"))
			bindFrame(hasher, []byte(path))
			bindMode(hasher, mode.Perm())
			if destinationRoot != nil {
				// Keep directories private and writable until their descendants
				// have been copied. Their observed source mode is restored below.
				if err := destinationRoot.Mkdir(path, 0o700); err != nil {
					return fmt.Errorf("create staged directory %q: %w", path, err)
				}
				directories = append(directories, stagedDirectory{path: path, mode: mode.Perm()})
			}
		case mode.IsRegular():
			if info.Size() < 0 {
				return fmt.Errorf("workspace file %q has a negative size", path)
			}
			size := uint64(info.Size())
			if size > maximumBytes-total {
				return fmt.Errorf("workspace exceeds %d-byte input ceiling", maximumBytes)
			}
			total += size
			bindFrame(hasher, []byte("regular"))
			bindFrame(hasher, []byte(path))
			bindMode(hasher, mode.Perm())
			bindUint64(hasher, size)
			if err := copyWorkspaceFile(ctx, sourceRoot, destinationRoot, path, info, size, hasher); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			target, err := sourceRoot.Readlink(path)
			if err != nil {
				return fmt.Errorf("read workspace symlink %q: %w", path, err)
			}
			if !utf8.ValidString(target) || strings.ContainsRune(target, '\x00') {
				return fmt.Errorf("workspace symlink %q has an invalid target", path)
			}
			size := uint64(len(target))
			if size > maximumBytes-total {
				return fmt.Errorf("workspace exceeds %d-byte input ceiling", maximumBytes)
			}
			total += size
			bindFrame(hasher, []byte("symlink"))
			bindFrame(hasher, []byte(path))
			bindFrame(hasher, []byte(target))
			if destinationRoot != nil {
				if err := destinationRoot.Symlink(target, path); err != nil {
					return fmt.Errorf("create staged symlink %q: %w", path, err)
				}
			}
		default:
			return fmt.Errorf("workspace contains unsupported special file %q", path)
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	if destinationRoot != nil {
		for index := len(directories) - 1; index >= 0; index-- {
			directory := directories[index]
			if err := destinationRoot.Chmod(directory.path, directory.mode); err != nil {
				return "", 0, fmt.Errorf("restore staged directory mode %q: %w", directory.path, err)
			}
		}
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), total, nil
}

func copyWorkspaceFile(
	ctx context.Context,
	sourceRoot, destinationRoot *os.Root,
	path string,
	sourceInfo fs.FileInfo,
	size uint64,
	hasher hash.Hash,
) error {
	source, err := sourceRoot.Open(path)
	if err != nil {
		return fmt.Errorf("open workspace file %q: %w", path, err)
	}
	defer source.Close() //nolint:errcheck
	openedInfo, err := source.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(openedInfo, sourceInfo) {
		return fmt.Errorf("workspace file %q changed while staging", path)
	}
	var destination *os.File
	if destinationRoot != nil {
		destination, err = destinationRoot.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create staged file %q: %w", path, err)
		}
		defer func() {
			if destination != nil {
				_ = destination.Close()
			}
		}()
	}
	writers := []io.Writer{hasher}
	if destination != nil {
		writers = append(writers, destination)
	}
	remaining := size
	buffer := make([]byte, 64<<10)
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := uint64(len(buffer))
		if chunk > remaining {
			chunk = remaining
		}
		read, err := io.ReadFull(source, buffer[:chunk])
		if err != nil {
			return fmt.Errorf("workspace file %q changed while staging: %w", path, err)
		}
		for _, writer := range writers {
			if _, err := writer.Write(buffer[:read]); err != nil {
				return fmt.Errorf("stage workspace file %q: %w", path, err)
			}
		}
		remaining -= uint64(read)
	}
	var extra [1]byte
	if read, err := source.Read(extra[:]); read != 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return fmt.Errorf("workspace file %q changed while staging", path)
	}
	if destination != nil {
		if err := destination.Close(); err != nil {
			return fmt.Errorf("close staged file %q: %w", path, err)
		}
		destination = nil
		if err := destinationRoot.Chmod(path, sourceInfo.Mode().Perm()); err != nil {
			return fmt.Errorf("restore staged file mode %q: %w", path, err)
		}
	}
	return nil
}

func stageInput(ctx context.Context, input Input, destination string, maximumBytes uint64) (BoundInput, error) {
	info, err := os.Lstat(input.Path)
	if err != nil {
		return BoundInput{}, fmt.Errorf("inspect input %q: %w", input.Name, err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 {
		return BoundInput{}, fmt.Errorf("input %q must be a regular file", input.Name)
	}
	size := uint64(info.Size())
	if size > maximumBytes {
		return BoundInput{}, fmt.Errorf("input %q exceeds remaining input ceiling", input.Name)
	}
	descriptor, err := syscall.Open(input.Path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return BoundInput{}, fmt.Errorf("open input %q: %w", input.Name, err)
	}
	source := os.NewFile(uintptr(descriptor), input.Path)
	if source == nil {
		_ = syscall.Close(descriptor)
		return BoundInput{}, fmt.Errorf("open input %q: invalid file descriptor", input.Name)
	}
	defer source.Close() //nolint:errcheck
	openedInfo, err := source.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return BoundInput{}, fmt.Errorf("input %q changed while staging", input.Name)
	}
	target, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		return BoundInput{}, fmt.Errorf("create staged input %q: %w", input.Name, err)
	}
	hasher := sha256.New()
	written, copyErr := copyWithContext(ctx, io.MultiWriter(target, hasher), source, size)
	closeErr := target.Close()
	if copyErr != nil {
		return BoundInput{}, fmt.Errorf("stage input %q: %w", input.Name, copyErr)
	}
	if closeErr != nil {
		return BoundInput{}, fmt.Errorf("close staged input %q: %w", input.Name, closeErr)
	}
	if written != size {
		return BoundInput{}, fmt.Errorf("input %q changed while staging", input.Name)
	}
	var extra [1]byte
	if read, err := source.Read(extra[:]); read != 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return BoundInput{}, fmt.Errorf("input %q changed while staging", input.Name)
	}
	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if digest != input.Digest {
		return BoundInput{}, fmt.Errorf("input %q digest mismatch: observed %s, want %s", input.Name, digest, input.Digest)
	}
	return BoundInput{Name: input.Name, Digest: digest, Size: size}, nil
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader, size uint64) (uint64, error) {
	var total uint64
	buffer := make([]byte, 64<<10)
	for total < size {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		chunk := uint64(len(buffer))
		if chunk > size-total {
			chunk = size - total
		}
		read, err := io.ReadFull(source, buffer[:chunk])
		if err != nil {
			return total, err
		}
		written, err := destination.Write(buffer[:read])
		total += uint64(written)
		if err != nil {
			return total, err
		}
		if written != read {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

func validStagedPath(path string) bool {
	if path == "" || path == "." || !utf8.ValidString(path) || strings.ContainsRune(path, '\x00') ||
		filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if segment == "" || segment == "." || segment == ".." || segment == ".git" {
			return false
		}
	}
	return true
}

func bindFrame(hasher hash.Hash, value []byte) {
	bindUint64(hasher, uint64(len(value)))
	_, _ = hasher.Write(value)
}

func bindUint64(hasher hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = hasher.Write(encoded[:])
}

func bindMode(hasher hash.Hash, mode os.FileMode) {
	bindUint64(hasher, uint64(mode.Perm()))
}

func removePrivateTree(path string) error {
	walkErr := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if err := os.Chmod(current, 0o700); err != nil {
				return err
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return walkErr
	}
	return os.RemoveAll(path)
}
