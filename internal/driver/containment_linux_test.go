//go:build linux

package driver

import "testing"

// requireTrustedContainment skips a test whose assertions genuinely require a
// trusted containment binary: bwrap discovered on PATH or via SWORN_BWRAP,
// absolute and regular, without group/world write bits, owned by uid 0, and
// passing the full capability probe. A host without one (a read-only worker,
// a nix or homebrew layout, a container where bwrap is not uid 0) cannot
// exercise the isolation assertions, so the test is skipped there instead of
// failing on an unavailable prerequisite. The containment trust checks
// themselves (containment_binary_linux_test.go) are untouched: they still run
// on every host and still require the fail-closed ISOLATION_UNAVAILABLE paths
// for untrusted binaries.
func requireTrustedContainment(t *testing.T) {
	t.Helper()
	if _, err := trustedBubblewrap(); err != nil {
		t.Skipf("trusted containment unavailable: %v", err)
	}
}
