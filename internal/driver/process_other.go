//go:build !linux

package driver

import (
	"context"
	"os"
)

func platformInvoke(context.Context, Invocation, ExecutableIdentity) (Observation, error) {
	return Observation{}, fail("UNSUPPORTED_HOST")
}

func openPinnedExecutable(ExecutableIdentity) (*os.File, error) {
	return nil, fail("UNSUPPORTED_HOST")
}
