//go:build linux && !amd64

package driver

import "os"

func unlinkedConfigFile([]byte) (*os.File, error) {
	return nil, fail("UNSUPPORTED_HOST")
}
