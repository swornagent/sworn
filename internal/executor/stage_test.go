//go:build linux

package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStageWorkspaceIsDeterministicAndPreservesObservedTree(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(source, "nested", "script"), []byte("#!/bin/true\n"), 0o751)
	writeTestFile(t, filepath.Join(source, "data"), []byte("payload"), 0o640)
	if err := os.Symlink("nested/script", filepath.Join(source, "entry")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(source, "nested"), 0o550); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(source, "nested"), 0o700) })

	digest, size, err := MeasureWorkspace(context.Background(), source, 1<<20)
	if err != nil {
		t.Fatalf("measure workspace: %v", err)
	}
	if size != uint64(len("#!/bin/true\n")+len("payload")+len("nested/script")) {
		t.Fatalf("measured size = %d", size)
	}
	destination := filepath.Join(t.TempDir(), "stage")
	stagedDigest, stagedSize, err := stageWorkspace(context.Background(), source, destination, 1<<20)
	if err != nil {
		t.Fatalf("stage workspace: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(destination, "nested"), 0o700) })
	if stagedDigest != digest || stagedSize != size {
		t.Fatalf("staged measurement = (%s, %d), want (%s, %d)", stagedDigest, stagedSize, digest, size)
	}

	assertMode(t, filepath.Join(destination, "nested"), 0o550)
	assertMode(t, filepath.Join(destination, "nested", "script"), 0o751)
	assertMode(t, filepath.Join(destination, "data"), 0o640)
	if target, err := os.Readlink(filepath.Join(destination, "entry")); err != nil || target != "nested/script" {
		t.Fatalf("staged symlink = %q, %v", target, err)
	}
	contents, err := os.ReadFile(filepath.Join(destination, "data"))
	if err != nil || string(contents) != "payload" {
		t.Fatalf("staged data = %q, %v", contents, err)
	}

	if err := os.Chtimes(filepath.Join(source, "data"), testTime(), testTime()); err != nil {
		t.Fatal(err)
	}
	repeated, _, err := MeasureWorkspace(context.Background(), source, 1<<20)
	if err != nil || repeated != digest {
		t.Fatalf("timestamp-only digest = %q, %v; want %q", repeated, err, digest)
	}
	if err := os.Chmod(filepath.Join(source, "data"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, _, err := MeasureWorkspace(context.Background(), source, 1<<20)
	if err != nil || changed == digest {
		t.Fatalf("permission-changed digest = %q, %v", changed, err)
	}
}

func TestWorkspaceStagingRejectsUnboundedOrUnsafeTrees(t *testing.T) {
	t.Parallel()
	t.Run("oversize", func(t *testing.T) {
		source := t.TempDir()
		writeTestFile(t, filepath.Join(source, "large"), []byte("12345"), 0o600)
		if _, _, err := MeasureWorkspace(context.Background(), source, 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("measure error = %v", err)
		}
	})
	t.Run("git metadata", func(t *testing.T) {
		source := t.TempDir()
		if err := os.Mkdir(filepath.Join(source, ".git"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, _, err := MeasureWorkspace(context.Background(), source, 1<<20); err == nil || !strings.Contains(err.Error(), "invalid path") {
			t.Fatalf("measure error = %v", err)
		}
	})
	t.Run("special file", func(t *testing.T) {
		source := t.TempDir()
		if err := syscall.Mkfifo(filepath.Join(source, "pipe"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := MeasureWorkspace(context.Background(), source, 1<<20); err == nil || !strings.Contains(err.Error(), "special file") {
			t.Fatalf("measure error = %v", err)
		}
	})
}

func TestStageInputBindsExactRegularFile(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "input.json")
	contents := []byte(`{"task":"prove"}`)
	writeTestFile(t, source, contents, 0o600)
	digest := digestBytes(contents)
	destination := filepath.Join(t.TempDir(), "staged")
	bound, err := stageInput(context.Background(), Input{Name: "task", Path: source, Digest: digest}, destination, 1<<20)
	if err != nil {
		t.Fatalf("stage input: %v", err)
	}
	if bound.Name != "task" || bound.Digest != digest || bound.Size != uint64(len(contents)) {
		t.Fatalf("bound input = %#v", bound)
	}
	assertMode(t, destination, 0o400)

	wrong := filepath.Join(t.TempDir(), "wrong")
	if _, err := stageInput(context.Background(), Input{Name: "task", Path: source, Digest: testDigest("f")}, wrong, 1<<20); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("wrong-digest error = %v", err)
	}
	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(source, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := stageInput(context.Background(), Input{Name: "task", Path: symlink, Digest: digest}, filepath.Join(t.TempDir(), "linked"), 1<<20); err == nil {
		t.Fatal("stageInput admitted a symbolic link")
	}
}

func TestRemovePrivateTreeRecoversRestrictiveStagedModes(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "runtime")
	if err := os.MkdirAll(filepath.Join(root, "workspace", "locked"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "workspace", "locked"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "workspace"), 0o500); err != nil {
		t.Fatal(err)
	}
	if err := removePrivateTree(root); err != nil {
		t.Fatalf("remove private tree: %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("runtime root still exists: %v", err)
	}
}

func writeTestFile(t *testing.T, path string, contents []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

func digestBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func testTime() time.Time {
	return time.Unix(1_700_000_000, 0)
}
