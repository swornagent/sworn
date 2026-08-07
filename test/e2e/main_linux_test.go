//go:build linux

package e2e

import (
	"fmt"
	"os"
	"testing"
)

// TestMain owns two package-wide concerns.
//
// First, the shared build cache: every `go build` in this package is keyed by
// (source, ldflags) and produced once into a directory that outlives any single
// test, then linked into each caller's own workspace. Callers still execute a
// real binary built from this exact tree.
//
// Second, the Sworn conformance certification gate. Surface tests register the
// anchors they actually executed while running; after the run this function
// fails the package if any declared conformance case finished without a passed
// real-binary anchor. That makes certification executable: a case cannot be
// certified by a prose claim, a tool-list snapshot, a schema parse, a unit test
// or an exit status, because none of those register an anchor.
func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "sworn-e2e-binaries-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: binary cache: %v\n", err)
		os.Exit(1)
	}
	binaryCacheMutex.Lock()
	binaryCacheDir = directory
	binaryCacheMutex.Unlock()

	code := m.Run()
	os.RemoveAll(directory)

	if code == 0 {
		if err := certifySwornConformance(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}
