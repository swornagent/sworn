// Package testpath keeps strict path-admission tests independent of whether
// the host exposes its system temporary directory through a symlink.
package testpath

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Main canonicalises only the process-owned test temporary root before tests
// create fixtures. Product admission rules remain unchanged.
func Main(m *testing.M) {
	root, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil || !filepath.IsAbs(root) {
		fmt.Fprintln(os.Stderr, "canonical test temporary directory unavailable")
		os.Exit(1)
	}
	if err := os.Setenv("TMPDIR", root); err != nil {
		fmt.Fprintln(os.Stderr, "canonical test temporary directory unavailable")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
