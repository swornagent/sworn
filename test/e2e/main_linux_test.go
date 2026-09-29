//go:build linux

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// declaredCheckCommandDir holds one directory containing an executable
// literally named "check". S1-host-check-environment-failures' A4 resolves
// every plan-declared check's first word (Checks, not only HostChecks) on
// the host runner's own PATH before any dispatch; this package's fixtures
// declare "check <id>" as their Checks entries (a name journeyNamedCheckPattern
// and its refusal-driven re-run depend on literally, and which a
// Checks-only entry never runs for real - production only executes a
// slice's own HostChecks at the host boundary). cleanEnvironment puts this
// directory on every spawned Sworn process's PATH so A4 never parks a
// fixture run over a name that was always a placeholder, not a real host
// check.
var declaredCheckCommandDir string

// TestMain owns three package-wide concerns.
//
// First, the shared build cache: every `go build` in this package is keyed by
// (source, ldflags) and produced once into a directory that outlives any single
// test, then linked into each caller's own workspace. Callers still execute a
// real binary built from this exact tree.
//
// Second, declaredCheckCommandDir (see above).
//
// Third, the Sworn conformance certification gate. Surface tests register the
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

	checkDir, err := os.MkdirTemp("", "sworn-e2e-check-command-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: check command dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(
		filepath.Join(checkDir, "check"), []byte("#!/bin/sh\nexit 0\n"), 0o755,
	); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: check command: %v\n", err)
		os.Exit(1)
	}
	declaredCheckCommandDir = checkDir

	code := m.Run()
	os.RemoveAll(directory)
	os.RemoveAll(checkDir)

	if code == 0 {
		if err := certifySwornConformance(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}
