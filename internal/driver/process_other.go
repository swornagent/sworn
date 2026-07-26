//go:build !linux

package driver

import "context"

func platformInvoke(context.Context, Invocation, ExecutableIdentity) (Observation, error) {
	return Observation{}, fail("UNSUPPORTED_HOST")
}
