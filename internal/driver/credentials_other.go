//go:build !linux

package driver

func acquireFileCredential(string, string, int64) (nativeCredentialLease, error) {
	return nil, fail("UNSUPPORTED_HOST")
}
