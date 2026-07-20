//go:build linux

package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ReconcileContentBound proves one exact invocation's systemd unit is
// quiescent, removes its deterministic executor-owned runtime, and verifies
// both facts again before minting a cleanup proof.
func (executor *LinuxExecutor) ReconcileContentBound(
	ctx context.Context,
	invocationID string,
) (cleanup ContentBoundCleanup, resultErr error) {
	if !idPattern.MatchString(invocationID) {
		return ContentBoundCleanup{}, errors.New("valid content-bound invocation id is required")
	}
	executor.contentMutex.Lock()
	defer executor.contentMutex.Unlock()
	ownership, err := executor.acquireContentBoundOwnership(ctx, true)
	if err != nil {
		return ContentBoundCleanup{}, err
	}
	defer func() {
		if err := releaseContentBoundOwnership(ownership); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	unit := executor.unitName(invocationID)
	live, err := executor.unitLive(ctx, unit)
	if err != nil {
		return ContentBoundCleanup{}, err
	}
	if live {
		return ContentBoundCleanup{}, fmt.Errorf("content-bound invocation %q is still live", invocationID)
	}
	path := executor.contentBoundRuntimePath(invocationID)
	if err := removePrivateTree(path); err != nil {
		return ContentBoundCleanup{}, fmt.Errorf("remove content-bound invocation residue %q: %w", path, err)
	}
	live, err = executor.unitLive(ctx, unit)
	if err != nil {
		return ContentBoundCleanup{}, err
	}
	if live {
		return ContentBoundCleanup{}, fmt.Errorf("content-bound invocation %q became live during reconciliation", invocationID)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return ContentBoundCleanup{}, fmt.Errorf("recheck content-bound invocation residue %q: %w", path, err)
		}
		return ContentBoundCleanup{}, fmt.Errorf("content-bound invocation residue %q remains", path)
	}
	return ContentBoundCleanup{
		invocationID: invocationID, proof: &contentBoundCleanupProof{},
	}, nil
}

func (executor *LinuxExecutor) contentBoundRuntimePath(invocationID string) string {
	name := strings.TrimSuffix(executor.unitName(invocationID), ".service") + ".content.runtime"
	return filepath.Join(executor.options.RuntimeRoot, name)
}

func (executor *LinuxExecutor) acquireContentBoundOwnership(ctx context.Context, exclusive bool) (*os.File, error) {
	root, err := os.Open(executor.options.RuntimeRoot)
	if err != nil {
		return nil, fmt.Errorf("open content-bound executor root for ownership: %w", err)
	}
	operation := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		operation = syscall.LOCK_EX | syscall.LOCK_NB
	}
	for {
		err = syscall.Flock(int(root.Fd()), operation)
		if err == nil {
			return root, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = root.Close()
			return nil, fmt.Errorf("lock content-bound executor root: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = root.Close()
			return nil, fmt.Errorf("acquire content-bound executor ownership: %w", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func releaseContentBoundOwnership(root *os.File) error {
	unlockErr := syscall.Flock(int(root.Fd()), syscall.LOCK_UN)
	closeErr := root.Close()
	if err := errors.Join(unlockErr, closeErr); err != nil {
		return fmt.Errorf("release content-bound executor ownership: %w", err)
	}
	return nil
}
