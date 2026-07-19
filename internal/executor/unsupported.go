//go:build !linux

package executor

import (
	"context"
	"errors"
	"io"
)

var errLinuxRequired = errors.New("the contained executor requires Linux")

type LinuxExecutor struct{}

func NewLinux(Options) (*LinuxExecutor, error) {
	return nil, errLinuxRequired
}

func (*LinuxExecutor) Probe(context.Context) (ProbeReport, error) {
	return ProbeReport{}, errLinuxRequired
}

func (*LinuxExecutor) RunContained(context.Context, Invocation) (RawCompletion, error) {
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

func RunShim([]string, io.Reader, io.Writer, io.Writer) int {
	return 126
}
