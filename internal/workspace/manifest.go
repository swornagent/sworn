// Package workspace owns the deterministic plain-tree manifest shared by Git
// materialization and contained executor staging.
package workspace

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
	"unicode/utf8"
)

const (
	manifestVersion = "sworn-workspace-manifest-v1"
	maximumEntries  = 100_000
)

// Measure returns the content, entry, type, and permission digest used to pin
// a plain workspace. Timestamps, ownership, and the root directory mode are
// deliberately excluded.
func Measure(ctx context.Context, source string, maximumBytes uint64) (string, uint64, error) {
	return walk(ctx, source, "", maximumBytes)
}

// StageInto copies a source into an existing empty destination while producing
// the exact same manifest as Measure.
func StageInto(
	ctx context.Context,
	source, destination string,
	maximumBytes uint64,
) (string, uint64, error) {
	if destination == "" {
		return "", 0, errors.New("workspace staging destination is required")
	}
	return walk(ctx, source, destination, maximumBytes)
}

func walk(
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
	bindFrame(hasher, []byte(manifestVersion))
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
		if entries > maximumEntries {
			return fmt.Errorf("workspace exceeds %d-entry ceiling", maximumEntries)
		}
		if !validPath(path) {
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
			if err := copyFile(ctx, sourceRoot, destinationRoot, path, info, size, hasher); err != nil {
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

func copyFile(
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

func validPath(path string) bool {
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
