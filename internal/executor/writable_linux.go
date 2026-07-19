//go:build linux

package executor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	tmpfsMagic               = 0x01021994
	mountFlagNoExec          = 0x8
	workspaceGenerationBytes = 16
)

func (executor *LinuxExecutor) RunWritable(ctx context.Context, invocation Invocation) (RawCompletion, error) {
	// The initial v1 kernel is serial. Keep staging and the shared-tmpfs
	// capacity decision in the same critical section so two callers cannot
	// both admit against the same free pages.
	executor.writableMutex.Lock()
	defer executor.writableMutex.Unlock()
	return executor.runInvocation(ctx, invocation, WorkspaceWritableExport)
}

func (executor *LinuxExecutor) ValidateExport(ctx context.Context, export WorkspaceExport) error {
	if err := executor.validateExportBinding(export, true); err != nil {
		return err
	}
	if err := executor.requireExportQuiescent(ctx, export); err != nil {
		return err
	}
	digest, size, err := MeasureWorkspace(ctx, export.Path, executor.options.Limits.WorkspaceBytes)
	if err != nil {
		return fmt.Errorf("revalidate writable workspace export: %w", err)
	}
	if digest != export.Digest || size != export.Bytes {
		return errors.New("writable workspace export changed after measurement")
	}
	return nil
}

// DiscardExport removes executor-owned workspace storage after the service has
// quiesced. Content is deliberately not revalidated: an unsafe or externally
// changed tree must remain cleanable, while its generation-bound path prevents
// a stale export handle from naming a later invocation's workspace.
func (executor *LinuxExecutor) DiscardExport(ctx context.Context, export WorkspaceExport) error {
	if err := executor.validateExportBinding(export, false); err != nil {
		return err
	}
	if err := executor.requireExportQuiescent(ctx, export); err != nil {
		return err
	}
	if err := removePrivateTree(export.Path); err != nil {
		return fmt.Errorf("discard writable workspace export: %w", err)
	}
	return nil
}

func (executor *LinuxExecutor) validateExportBinding(export WorkspaceExport, measure bool) error {
	if executor.options.WritableRoot == "" {
		return errors.New("writable executor root is not configured")
	}
	if export.SchemaVersion != WorkspaceExportSchemaVersion ||
		!idPattern.MatchString(export.InvocationID) ||
		!validWorkspaceGeneration(export.Generation) {
		return errors.New("invalid writable workspace export binding")
	}
	if measure && (!validDigest(export.BaseDigest) || !validDigest(export.Digest)) {
		return errors.New("invalid writable workspace export measurement")
	}
	if export.Path != executor.writableWorkspacePath(export.InvocationID, export.Generation) {
		return errors.New("writable workspace export path does not match invocation")
	}
	if measure && export.Bytes > executor.options.Limits.WorkspaceBytes {
		return errors.New("writable workspace export exceeds executor ceiling")
	}
	return nil
}

func (executor *LinuxExecutor) requireExportQuiescent(ctx context.Context, export WorkspaceExport) error {
	live, err := executor.unitLive(ctx, executor.unitName(export.InvocationID))
	if err != nil {
		return err
	}
	if live {
		return errors.New("cannot access a writable workspace while its service is live")
	}
	return nil
}

func (executor *LinuxExecutor) createWritableWorkspace(invocationID string) (string, string, error) {
	for attempts := 0; attempts < 4; attempts++ {
		contents := make([]byte, workspaceGenerationBytes)
		if _, err := rand.Read(contents); err != nil {
			return "", "", fmt.Errorf("generate writable workspace identity: %w", err)
		}
		generation := hex.EncodeToString(contents)
		path := executor.writableWorkspacePath(invocationID, generation)
		if err := os.Mkdir(path, 0o700); err == nil {
			return path, generation, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", "", fmt.Errorf("create writable workspace: %w", err)
		}
	}
	return "", "", errors.New("create writable workspace: generation collision")
}

func (executor *LinuxExecutor) writableWorkspacePath(invocationID, generation string) string {
	name := strings.TrimSuffix(executor.unitName(invocationID), ".service") + "." + generation + ".workspace"
	return filepath.Join(executor.options.WritableRoot, name)
}

func validWorkspaceGeneration(value string) bool {
	if len(value) != workspaceGenerationBytes*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == workspaceGenerationBytes && value == strings.ToLower(value)
}

func ensureWritableRoot(root string) error {
	if err := ensurePrivateRoot(root, "writable executor"); err != nil {
		return err
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(root, &filesystem); err != nil {
		return fmt.Errorf("inspect writable executor filesystem: %w", err)
	}
	if filesystem.Type != tmpfsMagic || filesystem.Blocks == 0 {
		return errors.New("writable executor root must use a finite tmpfs filesystem")
	}
	if filesystem.Flags&mountFlagNoExec != 0 {
		return errors.New("writable executor tmpfs must permit execution")
	}
	return nil
}

func ensureWritableCapacity(root string, limits Limits) error {
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(root, &filesystem); err != nil {
		return fmt.Errorf("inspect writable executor capacity: %w", err)
	}
	if filesystem.Bsize <= 0 || filesystem.Bavail > math.MaxUint64/uint64(filesystem.Bsize) {
		return errors.New("writable executor capacity is not representable")
	}
	available := filesystem.Bavail * uint64(filesystem.Bsize)
	required := limits.MemoryBytes
	if limits.SwapBytes > math.MaxUint64-required {
		return errors.New("writable executor memory and swap ceilings overflow")
	}
	required += limits.SwapBytes
	if available < required {
		return fmt.Errorf(
			"writable executor tmpfs has %d bytes available, below %d-byte live allocation ceiling",
			available, required,
		)
	}
	return nil
}

func (executor *LinuxExecutor) unitLive(ctx context.Context, unit string) (bool, error) {
	command := exec.CommandContext(ctx, executor.options.SystemctlPath, "--user", "is-active", unit)
	command.Env = controlEnvironment()
	output, err := command.CombinedOutput()
	state := strings.TrimSpace(string(output))
	switch state {
	case "active", "activating", "deactivating", "reloading", "refreshing":
		return true, nil
	case "inactive", "failed", "unknown":
		return false, nil
	default:
		if err != nil {
			return false, fmt.Errorf("resolve writable workspace service state: %w: %s", err, state)
		}
		return false, fmt.Errorf("unknown writable workspace service state %q", state)
	}
}

func (executor *LinuxExecutor) waitUnitQuiescent(ctx context.Context, unit string) error {
	var lastErr error
	for {
		live, err := executor.unitLive(ctx, unit)
		if err == nil && !live {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return errors.Join(ctx.Err(), lastErr)
			}
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func ensurePrivateRoot(root, label string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("%s root must be a clean absolute path", label)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create %s root: %w", label, err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve %s root: %w", label, err)
	}
	if filepath.Clean(resolved) != root {
		return fmt.Errorf("%s root contains a symbolic-link remap", label)
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect %s root: %w", label, err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s root must be private", label)
	}
	statistics, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(statistics.Uid) != os.Geteuid() {
		return fmt.Errorf("%s root must be owned by the current user", label)
	}
	return nil
}
