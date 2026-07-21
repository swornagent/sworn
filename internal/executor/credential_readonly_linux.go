//go:build linux

package executor

import (
	"context"
	"errors"
)

// RunCredentialReadOnly executes one exact host-runtime input against a
// read-only workspace while making the configured CLI credential available to
// the trusted parent process. Admission requires nested-sandbox support; the
// caller must bind an inner policy which actually keeps model-directed tools
// away from that credential and from host network access.
func (executor *LinuxExecutor) RunCredentialReadOnly(
	ctx context.Context,
	invocation Invocation,
) (completion RawCompletion, resultErr error) {
	// Validate the narrow entry-point contract before contending for either
	// runtime ownership or the credential lease.
	if err := invocation.validateFor(executor.options, executionCredentialReadOnly); err != nil {
		return RawCompletion{}, err
	}
	executor.contentMutex.RLock()
	defer executor.contentMutex.RUnlock()
	ownership, err := executor.acquireContentBoundOwnership(ctx, false)
	if err != nil {
		return RawCompletion{}, err
	}
	defer func() {
		if err := releaseContentBoundOwnership(ownership); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	return executor.withCredentialFile(invocation, func(credential *credentialFileLease) (RawCompletion, error) {
		return executor.runInvocation(
			ctx, invocation, WorkspaceReadOnly, nil, credential, executionCredentialReadOnly,
		)
	})
}
