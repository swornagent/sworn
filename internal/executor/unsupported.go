//go:build !linux

package executor

import (
	"context"
	"errors"
	"io"
)

var errLinuxRequired = errors.New("the contained executor requires Linux")

type LinuxExecutor struct{ options Options }

func NewLinux(Options) (*LinuxExecutor, error) {
	return nil, errLinuxRequired
}

func (*LinuxExecutor) Probe(context.Context) (ProbeReport, error) {
	return ProbeReport{}, errLinuxRequired
}

func (*LinuxExecutor) EffectiveLimits() Limits { return Limits{} }

func (executor *LinuxExecutor) ConfigurationDigest() string {
	return executorConfigurationDigest(executor.options)
}

func (*LinuxExecutor) RunContentBound(context.Context, Invocation, RuntimeTree) (RawCompletion, error) {
	return RawCompletion{}, errLinuxRequired
}

func (*LinuxExecutor) RunWritable(context.Context, Invocation) (RawCompletion, error) {
	return RawCompletion{}, errLinuxRequired
}

func (*LinuxExecutor) ValidateExport(context.Context, WorkspaceExport) error {
	return errLinuxRequired
}

func (*LinuxExecutor) DiscardExport(context.Context, WorkspaceExport) error {
	return errLinuxRequired
}

func (*LinuxExecutor) ReconcileWritable(context.Context, string) (WritableCleanup, error) {
	return WritableCleanup{}, errLinuxRequired
}

func MeasureWorkspace(context.Context, string, uint64) (string, uint64, error) {
	return "", 0, errLinuxRequired
}

func RunShim([]string, io.Reader, io.Writer, io.Writer) int {
	return 126
}
