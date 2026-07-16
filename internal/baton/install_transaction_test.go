package baton

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sort"
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

func TestBatonSyncImmutableCaptureRejectsConcurrentChange(t *testing.T) {
	trees, version := installTestAuthority(t)
	for _, point := range []string{"snapshot-after:agents_home", "publish-before"} {
		t.Run(strings.ReplaceAll(point, ":", "-"), func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			mutated := false
			opts.Fault = func(got string) error {
				if got == point && !mutated {
					mutated = true
					return os.WriteFile(filepath.Join(opts.Roots.AgentsHome, "unrelated", "concurrent.txt"), []byte("external\n"), 0o640)
				}
				return nil
			}
			_, err := SyncBatonInstall(opts)
			assertInstallErrorClass(t, err, "source-changed")
			if _, statErr := os.Lstat(filepath.Join(opts.Roots.RecoveryRoot, installRecoverySentinel)); !os.IsNotExist(statErr) {
				t.Fatalf("concurrent mutation published sentinel: %v", statErr)
			}
			if contents, readErr := os.ReadFile(filepath.Join(opts.Roots.AgentsHome, "unrelated", "concurrent.txt")); readErr != nil || string(contents) != "external\n" {
				t.Fatalf("external mutation was overwritten: %q %v", contents, readErr)
			}
		})
	}
}

func TestBatonSyncRejectsDerivedPathCollisionsWithoutDeletion(t *testing.T) {
	trees, version := installTestAuthority(t)
	id := strings.Repeat("a", 64)
	tests := []struct {
		name string
		path func(InstallRoots) string
		file bool
	}{
		{"stage", func(r InstallRoots) string {
			return filepath.Join(filepath.Dir(r.AgentsHome), ".sworn-baton-stage-"+id+"-agents_home")
		}, false},
		{"retired", func(r InstallRoots) string {
			return filepath.Join(filepath.Dir(r.RecoveryRoot), ".baton-sync-retired-"+id)
		}, false},
		{"recovery staging", func(r InstallRoots) string { return filepath.Join(r.RecoveryRoot, ".staging-"+id) }, false},
		{"transaction", func(r InstallRoots) string { return filepath.Join(r.RecoveryRoot, id) }, false},
		{"sentinel temp", func(r InstallRoots) string { return filepath.Join(r.RecoveryRoot, installRecoverySentinel+".tmp-"+id) }, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			opts.TransactionIDForTest = id
			writeInstallTestOriginals(t, opts.Roots)
			path := test.path(opts.Roots)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if filepath.Dir(path) == opts.Roots.RecoveryRoot {
				if err := os.Chmod(opts.Roots.RecoveryRoot, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			canary := filepath.Join(path, "canary")
			if test.file {
				if err := os.WriteFile(path, []byte("foreign\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				canary = path
			} else {
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(canary, []byte("foreign\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := SyncBatonInstall(opts); err == nil {
				t.Fatal("derived-path collision unexpectedly succeeded")
			}
			if contents, err := os.ReadFile(canary); err != nil || string(contents) != "foreign\n" {
				t.Fatalf("collision canary changed: %q %v", contents, err)
			}
		})
	}
}

func TestBatonSyncReassertsPhysicalAncestorIdentity(t *testing.T) {
	trees, version := installTestAuthority(t)
	opts := newInstallTestOpts(t, trees, version)
	writeInstallTestOriginals(t, opts.Roots)
	original := opts.Roots.AgentsHome
	moved := original + ".moved"
	opts.Fault = func(point string) error {
		if point != "paths-ready" {
			return nil
		}
		if err := os.Rename(original, moved); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(original, "unrelated"), 0o750); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(original, "unrelated", "keep.txt"), []byte("private original\n"), 0o640)
	}
	_, err := SyncBatonInstall(opts)
	assertInstallErrorClass(t, err, "unsafe-root-identity")
	if _, err := os.Lstat(filepath.Join(opts.Roots.RecoveryRoot, installRecoverySentinel)); !os.IsNotExist(err) {
		t.Fatalf("identity swap reached publication: %v", err)
	}
}

func TestBatonSyncCrashRecoveryMatrix(t *testing.T) {
	trees, version := installTestAuthority(t)
	points := []string{
		"replace-after:agents_home", "verify-after:agents_home",
		"replace-after:claude_home", "verify-after:claude_home",
		"replace-after:codex_home", "verify-after:codex_home",
		"retire-before",
	}
	for _, point := range points {
		t.Run(strings.ReplaceAll(point, ":", "-"), func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			before := snapshotInstallTestRoots(t, opts.Roots)
			opts.Fault = func(got string) error {
				if got == point {
					return errInstallCrash
				}
				return nil
			}
			if _, err := SyncBatonInstall(opts); err == nil {
				t.Fatal("crash fault unexpectedly succeeded")
			}
			if _, err := os.Lstat(filepath.Join(opts.Roots.RecoveryRoot, installRecoverySentinel)); err != nil {
				t.Fatalf("crash did not retain durable sentinel: %v", err)
			}
			opts.Fault = nil
			result, err := SyncBatonInstall(opts)
			if err != nil || result.State != InstallRecovered {
				t.Fatalf("fresh recovery = %#v, %v", result, err)
			}
			assertInstallTestSnapshots(t, opts.Roots, before)
		})
	}
}

func TestBatonSyncRecoversOriginallyAbsentRoots(t *testing.T) {
	trees, version := installTestAuthority(t)
	opts := newInstallTestOpts(t, trees, version)
	opts.Fault = func(point string) error {
		if point == "replace-after:agents_home" {
			return errInstallCrash
		}
		return nil
	}
	if _, err := SyncBatonInstall(opts); err == nil {
		t.Fatal("crash fault unexpectedly succeeded")
	}
	opts.Fault = nil
	result, err := SyncBatonInstall(opts)
	if err != nil || result.State != InstallRecovered {
		t.Fatalf("absent-root recovery = %#v, %v", result, err)
	}
	for _, root := range []string{opts.Roots.AgentsHome, opts.Roots.ClaudeHome, opts.Roots.CodexHome} {
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatalf("originally absent root restored as present: %s: %v", root, err)
		}
	}
}

func TestBatonSyncRecoveryTamperMatrixFailsBeforeTargets(t *testing.T) {
	trees, version := installTestAuthority(t)
	tests := []struct {
		name   string
		tamper func(*testing.T, InstallOpts, installSentinel)
	}{
		{"missing snapshot root", func(t *testing.T, _ InstallOpts, s installSentinel) {
			t.Helper()
			if err := os.RemoveAll(filepath.Join(s.RecoveryDirectory, "snapshots", "agents_home")); err != nil {
				t.Fatal(err)
			}
		}},
		{"foreign inventory", func(t *testing.T, _ InstallOpts, s installSentinel) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(s.RecoveryDirectory, "foreign"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"manifest kind", func(t *testing.T, _ InstallOpts, s installSentinel) {
			t.Helper()
			path := filepath.Join(s.RecoveryDirectory, installManifestName)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"manifest mode", func(t *testing.T, _ InstallOpts, s installSentinel) {
			t.Helper()
			if err := os.Chmod(filepath.Join(s.RecoveryDirectory, installManifestName), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"snapshot hash", func(t *testing.T, _ InstallOpts, s installSentinel) {
			t.Helper()
			path := filepath.Join(s.RecoveryDirectory, "snapshots", "agents_home", "unrelated", "keep.txt")
			if err := os.WriteFile(path, []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"snapshot symlink", func(t *testing.T, _ InstallOpts, s installSentinel) {
			t.Helper()
			path := filepath.Join(s.RecoveryDirectory, "snapshots", "agents_home", "unrelated", "keep.txt")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("elsewhere", path); err != nil {
				t.Fatal(err)
			}
		}},
		{"duplicate manifest", func(t *testing.T, opts InstallOpts, s installSentinel) {
			t.Helper()
			mutateInstallManifest(t, opts, s, func(entries []installManifestEntry) []installManifestEntry { return append(entries, entries[0]) })
		}},
		{"traversal manifest", func(t *testing.T, opts InstallOpts, s installSentinel) {
			t.Helper()
			mutateInstallManifest(t, opts, s, func(entries []installManifestEntry) []installManifestEntry {
				return append(entries, installManifestEntry{Path: "agents_home/../escape", Kind: "file", Mode: 0o600, Digest: "sha256:" + strings.Repeat("0", 64)})
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			before := snapshotInstallTestRoots(t, opts.Roots)
			opts.Fault = func(point string) error {
				if point == "publish-after" {
					return errInstallCrash
				}
				return nil
			}
			_, _ = SyncBatonInstall(opts)
			sentinel, _, err := loadInstallSentinel(opts.Roots.RecoveryRoot)
			if err != nil {
				t.Fatal(err)
			}
			test.tamper(t, opts, sentinel)
			opts.Fault = nil
			_, err = SyncBatonInstall(opts)
			assertInstallErrorClass(t, err, "recovery-invalid")
			assertInstallTestSnapshots(t, opts.Roots, before)
		})
	}
}

func TestBatonSyncBlocksUnidentifiedStagingAndRetiredDebris(t *testing.T) {
	trees, version := installTestAuthority(t)
	for _, retired := range []bool{false, true} {
		name := "staging"
		if retired {
			name = "retired"
		}
		t.Run(name, func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			var debris string
			if retired {
				debris = filepath.Join(filepath.Dir(opts.Roots.RecoveryRoot), ".baton-sync-retired-"+strings.Repeat("b", 64))
			} else {
				debris = filepath.Join(opts.Roots.RecoveryRoot, ".staging-"+strings.Repeat("b", 64))
			}
			if err := os.MkdirAll(debris, 0o700); err != nil {
				t.Fatal(err)
			}
			if !retired {
				if err := os.Chmod(opts.Roots.RecoveryRoot, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			canary := filepath.Join(debris, "foreign")
			if err := os.WriteFile(canary, []byte("keep\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := SyncBatonInstall(opts); err == nil {
				t.Fatal("foreign debris unexpectedly accepted")
			}
			if contents, err := os.ReadFile(canary); err != nil || string(contents) != "keep\n" {
				t.Fatalf("foreign debris changed: %q %v", contents, err)
			}
		})
	}
}

func TestBatonSyncFsyncAndUnrestoredUpdateFaultsFailClosed(t *testing.T) {
	trees, version := installTestAuthority(t)
	for _, point := range []string{
		"stage-sync-before:agents_home", "stage-sync-after:agents_home",
		"installed-sync-before:agents_home", "installed-sync-after:agents_home",
	} {
		t.Run(strings.ReplaceAll(point, ":", "-"), func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			before := snapshotInstallTestRoots(t, opts.Roots)
			opts.Fault = func(got string) error {
				if got == point {
					return errors.New("injected fsync boundary failure")
				}
				return nil
			}
			if _, err := SyncBatonInstall(opts); err == nil {
				t.Fatal("fsync boundary fault unexpectedly succeeded")
			}
			assertInstallTestSnapshots(t, opts.Roots, before)
		})
	}

	for _, point := range []string{"control-sync-before", "unrestored-update-after"} {
		t.Run(point, func(t *testing.T) {
			opts := newInstallTestOpts(t, trees, version)
			writeInstallTestOriginals(t, opts.Roots)
			before := snapshotInstallTestRoots(t, opts.Roots)
			restoring := false
			opts.Fault = func(got string) error {
				switch got {
				case "replace-after:agents_home":
					return errors.New("force rollback")
				case "restore-before:agents_home":
					restoring = true
					return errors.New("force incomplete rollback")
				case point:
					if restoring {
						return errors.New("force durable unrestored update failure")
					}
				}
				return nil
			}
			_, err := SyncBatonInstall(opts)
			assertInstallErrorClass(t, err, "rollback-incomplete")
			if _, err := os.Lstat(filepath.Join(opts.Roots.RecoveryRoot, installRecoverySentinel)); err != nil {
				t.Fatalf("update failure discarded recovery authority: %v", err)
			}
			opts.Fault = nil
			result, err := SyncBatonInstall(opts)
			if err != nil || result.State != InstallRecovered {
				t.Fatalf("recovery after update fault = %#v, %v", result, err)
			}
			assertInstallTestSnapshots(t, opts.Roots, before)
		})
	}
}

func TestBatonSyncRejectsCompleteUnsafePathMatrix(t *testing.T) {
	trees, version := installTestAuthority(t)
	t.Run("equal roots", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "same")
		opts := InstallOpts{Roots: InstallRoots{AgentsHome: root, CodexHome: root, ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "recovery")}, Trees: trees, Version: version}
		_, err := SyncBatonInstall(opts)
		assertInstallErrorClass(t, err, "unsafe-root-topology")
	})
	t.Run("reverse nesting", func(t *testing.T) {
		base := t.TempDir()
		opts := InstallOpts{Roots: InstallRoots{AgentsHome: filepath.Join(base, "agents", "child"), CodexHome: filepath.Join(base, "agents"), ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "recovery")}, Trees: trees, Version: version}
		_, err := SyncBatonInstall(opts)
		assertInstallErrorClass(t, err, "unsafe-root-topology")
	})
	t.Run("missing suffix lexical alias", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "missing", "home")
		opts := InstallOpts{Roots: InstallRoots{AgentsHome: root, CodexHome: filepath.Join(base, "missing", "other", "..", "home"), ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "recovery")}, Trees: trees, Version: version}
		_, err := SyncBatonInstall(opts)
		assertInstallErrorClass(t, err, "unsafe-root-topology")
	})
	t.Run("symlink path component", func(t *testing.T) {
		base := t.TempDir()
		real := filepath.Join(base, "real")
		if err := os.Mkdir(real, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(base, "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		opts := InstallOpts{Roots: InstallRoots{AgentsHome: filepath.Join(link, "agents"), CodexHome: filepath.Join(base, "codex"), ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "recovery")}, Trees: trees, Version: version}
		_, err := SyncBatonInstall(opts)
		assertInstallErrorClass(t, err, "unsafe-root")
	})
	t.Run("invalid utf8", func(t *testing.T) {
		base := t.TempDir()
		opts := InstallOpts{Roots: InstallRoots{AgentsHome: filepath.Join(base, string([]byte{0xff})), CodexHome: filepath.Join(base, "codex"), ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "recovery")}, Trees: trees, Version: version}
		_, err := SyncBatonInstall(opts)
		assertInstallErrorClass(t, err, "unsafe-root")
	})
	t.Run("device root", func(t *testing.T) {
		base := t.TempDir()
		opts := InstallOpts{Roots: InstallRoots{AgentsHome: "/dev/null", CodexHome: filepath.Join(base, "codex"), ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "recovery")}, Trees: trees, Version: version}
		_, err := SyncBatonInstall(opts)
		assertInstallErrorClass(t, err, "unsafe-root")
	})
	t.Run("socket beneath target", func(t *testing.T) {
		opts := newInstallTestOpts(t, trees, version)
		if err := os.MkdirAll(opts.Roots.AgentsHome, 0o755); err != nil {
			t.Fatal(err)
		}
		socket := filepath.Join(opts.Roots.AgentsHome, "socket")
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		_, err = SyncBatonInstall(opts)
		assertInstallErrorClass(t, err, "unsafe-target")
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

func installTestAuthority(t *testing.T) (InstallerManagedTrees, []byte) {
	t.Helper()
	trees, err := GenerateInstallerManagedTrees(adopt.BatonInstallerArchive())
	if err != nil {
		t.Fatal(err)
	}
	version, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		t.Fatal(err)
	}
	return trees, version
}

func assertInstallErrorClass(t *testing.T, err error, want string) {
	t.Helper()
	var installErr *InstallError
	if !errors.As(err, &installErr) || installErr.Class != want {
		t.Fatalf("install error = %v, want class %s", err, want)
	}
}

func mutateInstallManifest(t *testing.T, opts InstallOpts, sentinel installSentinel, mutate func([]installManifestEntry) []installManifestEntry) {
	t.Helper()
	path := filepath.Join(sentinel.RecoveryDirectory, installManifestName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := parseInstallManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	entries = mutate(entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	raw = marshalInstallManifest(entries)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	sentinel.ManifestSHA256 = "sha256:" + hex.EncodeToString(digest[:])
	sentinelRaw, err := marshalInstallSentinel(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.Roots.RecoveryRoot, installRecoverySentinel), sentinelRaw, 0o600); err != nil {
		t.Fatal(err)
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
