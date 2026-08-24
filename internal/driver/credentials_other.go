//go:build !linux

package driver

import "os"

func acquireFileCredential(string, string, int64) (nativeCredentialLease, error) {
	return nil, fail("UNSUPPORTED_HOST")
}

// openCredentialPreflight is unsupported off Linux: native dispatch is
// UNSUPPORTED_HOST there, so the preflight probe passes unchanged and the
// lease never exists.
func openCredentialPreflight(string) (*os.File, error) {
	return nil, fail("UNSUPPORTED_HOST")
}
