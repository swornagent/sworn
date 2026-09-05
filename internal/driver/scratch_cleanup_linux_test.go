//go:build linux

package driver

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveScratchTreeClearsReadOnlySubtreesWithoutFollowingLinks pins the
// mechanism behind sworn#285: a scratch root holding a copy of a ReadOnly
// workspace (0500 directories, 0400 files) is removed completely, while a
// symbolic link inside the scratch that points outside it is removed as a
// link and its target's mode is left exactly as it was.
func TestRemoveScratchTreeClearsReadOnlySubtreesWithoutFollowingLinks(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	outsideDir := filepath.Join(outside, "kept")
	if err := os.Mkdir(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "f"), []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outsideDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(outsideDir, 0o700) })

	root, err := os.MkdirTemp(t.TempDir(), "sworn-invocation-scratch-")
	if err != nil {
		t.Fatal(err)
	}
	copyDir := filepath.Join(root, "tmp", "swornmod", "cmd")
	if err := os.MkdirAll(copyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, "main.go"), []byte("package main\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "tmp", "link")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{copyDir, filepath.Dir(copyDir)} {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(root); err == nil {
		t.Fatal("os.RemoveAll removed a 0500 subtree; the fixture no longer reproduces sworn#285")
	}

	if err := removeScratchTree(root); err != nil {
		t.Fatalf("removeScratchTree = %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("scratch root survived: %v", err)
	}
	info, err := os.Stat(outsideDir)
	if err != nil {
		t.Fatalf("link target lost: %v", err)
	}
	if info.Mode().Perm() != 0o500 {
		t.Fatalf("link target mode changed to %o", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "f")); err != nil {
		t.Fatalf("link target content lost: %v", err)
	}
}

// TestToolSessionCloseSurvivesReadOnlyScratchCopy drives the live shape of
// sworn#285 through the sandbox: a worker copies read-only material into its
// /tmp surface, and closing the session must still remove the scratch and
// report no error - the turn's yield or submission must never be lost to the
// cleanup.
func TestToolSessionCloseSurvivesReadOnlyScratchCopy(t *testing.T) {
	requireTrustedContainment(t)
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	scratch := session.scratch
	if scratch == "" {
		t.Fatal("session has no scratch root")
	}
	result := executeToolJSON(t, session, "bash-ro-copy", "Bash", map[string]any{
		"script": `mkdir -p /tmp/swornmod/cmd &&
printf 'package main\n' > /tmp/swornmod/cmd/main.go &&
chmod 0400 /tmp/swornmod/cmd/main.go &&
chmod 0500 /tmp/swornmod/cmd /tmp/swornmod &&
ls -ld /tmp/swornmod`,
	})
	if result.Failed {
		t.Fatalf("read-only copy setup failed: %s", string(result.Content))
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close with a read-only scratch copy = %v", err)
	}
	if _, err := os.Lstat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch root survived Close: %v", err)
	}
}
