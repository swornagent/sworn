//go:build !linux

package driver

import "context"

func platformInvoke(context.Context, Invocation) (Observation, error) {
	return Observation{}, fail("UNSUPPORTED_HOST")
}
