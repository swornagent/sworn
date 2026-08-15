//go:build linux

package driver

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

// TestTrustedBubblewrapResolvesOverrideAndKeepsTrustRequirements is the A3
// proof that the containment binary resolves from configuration (SWORN_BWRAP)
// while its trust requirements are unchanged: every unsafe override is
// refused with ISOLATION_UNAVAILABLE before any capability probe.
func TestTrustedBubblewrapResolvesOverrideAndKeepsTrustRequirements(t *testing.T) {
	t.Setenv(gitx.EnvBubblewrap, "")

	t.Run("relative override refused", func(t *testing.T) {
		t.Setenv(gitx.EnvBubblewrap, "relative/bwrap")
		if _, err := trustedBubblewrap(); err == nil || !IsCode(err, "ISOLATION_UNAVAILABLE") {
			t.Fatalf("relative override error = %v, want ISOLATION_UNAVAILABLE", err)
		}
	})

	t.Run("world-writable override refused", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bwrap")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o666); err != nil {
			t.Fatal(err)
		}
		t.Setenv(gitx.EnvBubblewrap, path)
		if _, err := trustedBubblewrap(); err == nil || !IsCode(err, "ISOLATION_UNAVAILABLE") {
			t.Fatalf("world-writable override error = %v, want ISOLATION_UNAVAILABLE", err)
		}
	})

	t.Run("non-root-owned override refused", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bwrap")
		// A regular executable owned by the current (non-root) user must be
		// refused: the containment binary must be owned by uid 0.
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(gitx.EnvBubblewrap, path)
		if _, err := trustedBubblewrap(); err == nil || !IsCode(err, "ISOLATION_UNAVAILABLE") {
			t.Fatalf("non-root override error = %v, want ISOLATION_UNAVAILABLE", err)
		}
	})

	t.Run("nonexistent override refused", func(t *testing.T) {
		t.Setenv(gitx.EnvBubblewrap, filepath.Join(t.TempDir(), "missing"))
		if _, err := trustedBubblewrap(); err == nil || !IsCode(err, "ISOLATION_UNAVAILABLE") {
			t.Fatalf("missing override error = %v, want ISOLATION_UNAVAILABLE", err)
		}
	})

	t.Run("unset override uses discovery", func(t *testing.T) {
		t.Setenv(gitx.EnvBubblewrap, "")
		path, err := trustedBubblewrap()
		if err != nil {
			// A host without a trusted bwrap (or a sandbox where the probe
			// cannot complete) reports ISOLATION_UNAVAILABLE; the resolution
			// default itself is discovery, never an absolute distribution
			// literal.
			if !IsCode(err, "ISOLATION_UNAVAILABLE") {
				t.Fatalf("default bwrap error = %v", err)
			}
			return
		}
		discovered, lookErr := exec.LookPath("bwrap")
		if lookErr != nil {
			t.Fatalf("discovered bwrap unavailable while default resolved: %v", lookErr)
		}
		if path != discovered {
			t.Fatalf("default bwrap path = %q, want discovery %q", path, discovered)
		}
	})
}

// TestContainmentBinaryRefusedFromProjectScope proves A3's project-scope
// refusal: the project config schema carries no containment-binary field, so
// a docs/sworn/sworn.json naming one is refused at parse time with a named
// error instead of being silently honoured.
func TestContainmentBinaryRefusedFromProjectScope(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "sworn"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"containment_binary": "/usr/bin/bwrap"}`
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(gitx.ProjectConfigPath)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gitx.LoadProjectConfig(root); err == nil {
		t.Fatal("project config naming the containment binary was admitted")
	}
}
