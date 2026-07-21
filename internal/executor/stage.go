//go:build linux

package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/swornagent/sworn/internal/workspace"
)

func MeasureWorkspace(ctx context.Context, source string, maximumBytes uint64) (string, uint64, error) {
	return workspace.Measure(ctx, source, maximumBytes)
}

func stageWorkspace(ctx context.Context, source, destination string, maximumBytes uint64) (string, uint64, error) {
	if err := os.Mkdir(destination, 0o700); err != nil {
		return "", 0, fmt.Errorf("create staged workspace: %w", err)
	}
	return workspace.StageInto(ctx, source, destination, maximumBytes)
}

func stageRuntime(
	ctx context.Context,
	runtime RuntimeTree,
	destination string,
	executorMaximum uint64,
) (string, uint64, error) {
	if runtime.root == "" || runtime.maximumBytes == 0 || executorMaximum == 0 ||
		runtime.identity == nil || !validDigest(runtime.digest) {
		return "", 0, errors.New("content runtime capability is invalid")
	}
	resolved, err := filepath.EvalSymlinks(runtime.root)
	if err != nil || resolved != runtime.root || beneathPath("/usr", resolved) {
		return "", 0, errors.New("content runtime source identity changed")
	}
	info, err := os.Stat(resolved)
	if err != nil || !os.SameFile(runtime.identity, info) {
		return "", 0, errors.New("content runtime source identity changed")
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return "", 0, fmt.Errorf("create staged content runtime: %w", err)
	}
	maximumBytes := min(runtime.maximumBytes, executorMaximum)
	digest, size, err := workspace.StageInto(ctx, runtime.root, destination, maximumBytes)
	if err != nil {
		return "", 0, fmt.Errorf("stage content runtime: %w", err)
	}
	if digest != runtime.digest {
		return "", 0, fmt.Errorf("content runtime digest mismatch: observed %s, want %s", digest, runtime.digest)
	}
	remeasured, remeasuredSize, err := workspace.Measure(ctx, destination, maximumBytes)
	if err != nil {
		return "", 0, fmt.Errorf("remeasure staged content runtime: %w", err)
	}
	if remeasured != digest || remeasuredSize != size {
		return "", 0, errors.New("staged content runtime does not match its source measurement")
	}
	if err := validateRuntimeSymlinks(destination); err != nil {
		return "", 0, err
	}
	return digest, size, nil
}

func stageInput(
	ctx context.Context,
	input Input,
	destination string,
	maximumBytes uint64,
	executable bool,
) (BoundInput, error) {
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
	mode := os.FileMode(0o400)
	if executable {
		mode = 0o500
	}
	target, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
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
	if err := os.Chmod(destination, mode); err != nil {
		return BoundInput{}, fmt.Errorf("set staged input %q mode: %w", input.Name, err)
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
