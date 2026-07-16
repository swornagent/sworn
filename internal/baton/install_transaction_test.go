package baton

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/swornagent/sworn/internal/adopt"
)

func TestBatonSyncRollbackAndRecovery(t *testing.T) {
	trees, err := GenerateInstallerManagedTrees(adopt.BatonInstallerArchive())
	if err != nil {
		t.Fatal(err)
	}
	version, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("repair is complete and idempotent", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		writeInstallTestOriginals(t, opts.Roots)
		result, err := SyncBatonInstall(opts)
		if err != nil {
			t.Fatal(err)
		}
		if result.State != InstallRepaired || len(result.Changed) == 0 {
			t.Fatalf("first sync = %#v, want repaired with drift", result)
		}
		if drift, err := CheckBatonInstall(opts); err != nil || len(drift) != 0 {
			t.Fatalf("post-repair check = %v, %v", drift, err)
		}
		assertInstallTestOriginalsPreserved(t, opts.Roots)
		assertInstalledModes(t, opts.Roots, trees)
		result, err = SyncBatonInstall(opts)
		if err != nil || result.State != InstallAlreadyExact {
			t.Fatalf("second sync = %#v, %v, want already exact", result, err)
		}
	})

	t.Run("apply failure restores every complete root", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		writeInstallTestOriginals(t, opts.Roots)
		before := snapshotInstallTestRoots(t, opts.Roots)
		opts.Fault = func(point string) error {
			if point == "replace-after:agents_home" {
				return errors.New("injected apply failure")
			}
			return nil
		}
		result, err := SyncBatonInstall(opts)
		var installErr *InstallError
		if result != nil || !errors.As(err, &installErr) || installErr.Class != "repair-failed-restored" {
			t.Fatalf("failed repair = %#v, %v", result, err)
		}
		assertInstallTestSnapshots(t, opts.Roots, before)
		if _, err := os.Lstat(opts.Roots.RecoveryRoot); !os.IsNotExist(err) {
			t.Fatalf("complete rollback retained authority: %v", err)
		}
	})

	t.Run("durable sentinel forces recovery only", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		writeInstallTestOriginals(t, opts.Roots)
		before := snapshotInstallTestRoots(t, opts.Roots)
		opts.Fault = func(point string) error {
			if point == "publish-after" {
				return errors.New("simulated process stop after sentinel")
			}
			return nil
		}
		if _, err := SyncBatonInstall(opts); err == nil {
			t.Fatal("publish-after fault unexpectedly succeeded")
		}
		if info, err := os.Lstat(filepath.Join(opts.Roots.RecoveryRoot, installRecoverySentinel)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("durable sentinel = %v, %v", info, err)
		}
		opts.Fault = nil
		result, err := SyncBatonInstall(opts)
		if err != nil || result.State != InstallRecovered {
			t.Fatalf("recovery-only sync = %#v, %v", result, err)
		}
		assertInstallTestSnapshots(t, opts.Roots, before)
	})

	t.Run("incomplete rollback retains complete authority", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		writeInstallTestOriginals(t, opts.Roots)
		before := snapshotInstallTestRoots(t, opts.Roots)
		opts.Fault = func(point string) error {
			if point == "replace-after:agents_home" || point == "restore-before:agents_home" {
				return errors.New("injected rollback boundary failure")
			}
			return nil
		}
		_, err := SyncBatonInstall(opts)
		var installErr *InstallError
		if !errors.As(err, &installErr) || installErr.Class != "rollback-incomplete" || !strings.Contains(strings.Join(installErr.Paths, ","), "agents_home") {
			t.Fatalf("rollback error = %v", err)
		}
		opts.Fault = nil
		result, err := SyncBatonInstall(opts)
		if err != nil || result.State != InstallRecovered {
			t.Fatalf("retry recovery = %#v, %v", result, err)
		}
		assertInstallTestSnapshots(t, opts.Roots, before)
	})
}

func TestBatonSyncRejectsUnsafeTopologyBeforeMutation(t *testing.T) {
	trees, err := GenerateInstallerManagedTrees(adopt.BatonInstallerArchive())
	if err != nil {
		t.Fatal(err)
	}
	version, _ := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	base := t.TempDir()
	canary := filepath.Join(base, "canary")
	if err := os.WriteFile(canary, []byte("unchanged\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		roots InstallRoots
	}{
		{
			name: "nested roots",
			roots: InstallRoots{
				AgentsHome: filepath.Join(base, "agents"), CodexHome: filepath.Join(base, "agents", "codex"),
				ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "config", "recovery", "baton-sync"),
			},
		},
		{
			name: "recovery overlap",
			roots: InstallRoots{
				AgentsHome: filepath.Join(base, "agents2"), CodexHome: filepath.Join(base, "codex2"),
				ClaudeHome: filepath.Join(base, "claude2"), RecoveryRoot: filepath.Join(base, "codex2", "recovery"),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := SyncBatonInstall(InstallOpts{Roots: test.roots, Trees: trees, Version: version})
			var installErr *InstallError
			if !errors.As(err, &installErr) || installErr.Class != "unsafe-root-topology" {
				t.Fatalf("unsafe sync error = %v", err)
			}
			if contents, readErr := os.ReadFile(canary); readErr != nil || string(contents) != "unchanged\n" {
				t.Fatalf("preflight changed canary: %v %q", readErr, contents)
			}
		})
	}
}

func TestBatonSyncFaultMatrix(t *testing.T) {
	trees, err := GenerateInstallerManagedTrees(adopt.BatonInstallerArchive())
	if err != nil {
		t.Fatal(err)
	}
	version, _ := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	points := []string{
		"replace-before:agents_home", "replace-after:agents_home", "verify-after:agents_home",
		"replace-before:claude_home", "replace-after:claude_home", "verify-after:claude_home",
		"replace-before:codex_home", "replace-after:codex_home", "verify-after:codex_home",
	}
	for _, point := range points {
		t.Run(strings.ReplaceAll(point, ":", "-"), func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			before := snapshotInstallTestRoots(t, opts.Roots)
			opts.Fault = func(got string) error {
				if got == point {
					return errors.New("injected transaction fault")
				}
				return nil
			}
			if _, err := SyncBatonInstall(opts); err == nil {
				t.Fatal("fault unexpectedly succeeded")
			}
			assertInstallTestSnapshots(t, opts.Roots, before)
			if _, err := os.Lstat(opts.Roots.RecoveryRoot); !os.IsNotExist(err) {
				t.Fatalf("complete rollback retained recovery: %v", err)
			}
		})
	}

	for _, point := range []string{"publish-before", "publish-after", "retire-before", "retire-after"} {
		t.Run(point, func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			before := snapshotInstallTestRoots(t, opts.Roots)
			opts.Fault = func(got string) error {
				if got == point {
					return errors.New("injected durable-boundary fault")
				}
				return nil
			}
			if _, err := SyncBatonInstall(opts); err == nil {
				t.Fatal("durable-boundary fault unexpectedly succeeded")
			}
			opts.Fault = nil
			result, err := SyncBatonInstall(opts)
			if err != nil {
				t.Fatalf("restart after %s: %v", point, err)
			}
			if point == "publish-before" {
				if result.State != InstallRepaired {
					t.Fatalf("restart state = %s, want repaired", result.State)
				}
				return
			}
			if point == "retire-after" {
				if result.State != InstallAlreadyExact {
					t.Fatalf("restart state = %s, want already exact", result.State)
				}
				return
			}
			if result.State != InstallRecovered {
				t.Fatalf("restart state = %s, want recovered", result.State)
			}
			assertInstallTestSnapshots(t, opts.Roots, before)
		})
	}
}

func TestBatonSyncRejectsSymlinkSpecialAndTamperedRecovery(t *testing.T) {
	trees, err := GenerateInstallerManagedTrees(adopt.BatonInstallerArchive())
	if err != nil {
		t.Fatal(err)
	}
	version, _ := adopt.BatonDocsFS().ReadFile("baton/VERSION")

	t.Run("symlink beneath target", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		if err := os.MkdirAll(opts.Roots.AgentsHome, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), filepath.Join(opts.Roots.AgentsHome, "link")); err != nil {
			t.Fatal(err)
		}
		_, err := SyncBatonInstall(opts)
		var installErr *InstallError
		if !errors.As(err, &installErr) || installErr.Class != "unsafe-target" {
			t.Fatalf("symlink error = %v", err)
		}
	})

	t.Run("fifo beneath target", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		if err := os.MkdirAll(opts.Roots.ClaudeHome, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(filepath.Join(opts.Roots.ClaudeHome, "fifo"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := SyncBatonInstall(opts)
		var installErr *InstallError
		if !errors.As(err, &installErr) || installErr.Class != "unsafe-target" {
			t.Fatalf("fifo error = %v", err)
		}
	})

	t.Run("sentinel mode tamper", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		writeInstallTestOriginals(t, opts.Roots)
		before := snapshotInstallTestRoots(t, opts.Roots)
		opts.Fault = func(point string) error {
			if point == "publish-after" {
				return errors.New("stop after publication")
			}
			return nil
		}
		_, _ = SyncBatonInstall(opts)
		sentinel := filepath.Join(opts.Roots.RecoveryRoot, installRecoverySentinel)
		if err := os.Chmod(sentinel, 0o644); err != nil {
			t.Fatal(err)
		}
		opts.Fault = nil
		_, err := SyncBatonInstall(opts)
		var installErr *InstallError
		if !errors.As(err, &installErr) || installErr.Class != "recovery-invalid" {
			t.Fatalf("tampered recovery error = %v", err)
		}
		assertInstallTestSnapshots(t, opts.Roots, before)
		if !strings.Contains(err.Error(), "recovery-invalid") || strings.Contains(err.Error(), "private original") {
			t.Fatalf("diagnostic is not class/path-only: %v", err)
		}
	})
}

func newInstallTestOpts(t *testing.T, trees InstallerManagedTrees, version []byte) InstallOpts {
	t.Helper()
	base := t.TempDir()
	return InstallOpts{
		Roots: InstallRoots{
			AgentsHome: filepath.Join(base, "agents"), CodexHome: filepath.Join(base, "codex"),
			ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "config", "recovery", "baton-sync"),
		},
		Trees: trees, Version: append([]byte(nil), version...),
	}
}

func writeInstallTestOriginals(t *testing.T, roots InstallRoots) {
	t.Helper()
	for _, root := range []string{roots.AgentsHome, roots.CodexHome, roots.ClaudeHome} {
		if err := os.MkdirAll(filepath.Join(root, "unrelated"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, "unrelated"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "unrelated", "keep.txt"), []byte("private original\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, "unrelated", "keep.txt"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

func assertInstallTestOriginalsPreserved(t *testing.T, roots InstallRoots) {
	t.Helper()
	for _, root := range []string{roots.AgentsHome, roots.CodexHome, roots.ClaudeHome} {
		path := filepath.Join(root, "unrelated", "keep.txt")
		contents, err := os.ReadFile(path)
		info, statErr := os.Lstat(path)
		if err != nil || statErr != nil || string(contents) != "private original\n" || info.Mode().Perm() != 0o640 {
			t.Errorf("unrelated file changed at %s: read=%v stat=%v mode=%v", path, err, statErr, info)
		}
	}
}

func assertInstalledModes(t *testing.T, roots InstallRoots, trees InstallerManagedTrees) {
	t.Helper()
	for _, target := range []struct {
		root string
		tree ManagedTree
	}{{roots.AgentsHome, trees.AgentsHome}, {roots.CodexHome, trees.CodexHome}, {roots.ClaudeHome, trees.ClaudeHome}} {
		for _, entry := range target.tree.Entries {
			info, err := os.Lstat(filepath.Join(target.root, filepath.FromSlash(entry.Path)))
			if err != nil || info.Mode().Perm() != entry.Mode {
				t.Errorf("mode %s: info=%v err=%v want=%o", entry.Path, info, err, entry.Mode)
			}
		}
	}
}

func snapshotInstallTestRoots(t *testing.T, roots InstallRoots) map[string]map[string]filesystemSnapshotEntry {
	t.Helper()
	return map[string]map[string]filesystemSnapshotEntry{
		roots.AgentsHome: snapshotFilesystemTree(t, roots.AgentsHome),
		roots.CodexHome:  snapshotFilesystemTree(t, roots.CodexHome),
		roots.ClaudeHome: snapshotFilesystemTree(t, roots.ClaudeHome),
	}
}

func assertInstallTestSnapshots(t *testing.T, roots InstallRoots, want map[string]map[string]filesystemSnapshotEntry) {
	t.Helper()
	for _, root := range []string{roots.AgentsHome, roots.CodexHome, roots.ClaudeHome} {
		got := snapshotFilesystemTree(t, root)
		if len(got) != len(want[root]) {
			t.Errorf("%s entry count = %d, want %d", root, len(got), len(want[root]))
		}
		for name, wantEntry := range want[root] {
			gotEntry, ok := got[name]
			if !ok || gotEntry.mode != wantEntry.mode || !bytes.Equal(gotEntry.bytes, wantEntry.bytes) {
				t.Errorf("%s/%s differs after restoration", root, name)
			}
		}
	}
}
