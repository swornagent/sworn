//go:build linux

package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/workspace"
)

func TestRuntimeTreeRequiresExactMeasuredNonHostSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := t.TempDir()
	writeRuntimeFile(t, filepath.Join(source, "bin", "tool"), []byte("tool"), 0o755)
	digest, size, err := workspace.Measure(ctx, source, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntimeTree(source, digest, 1<<20)
	if err != nil || runtime.Digest() != digest {
		t.Fatalf("runtime = %#v, %v", runtime, err)
	}
	if size == 0 {
		t.Fatal("runtime fixture has no bytes")
	}
	for _, hostSource := range []string{"/usr", "/"} {
		if _, err := NewRuntimeTree(hostSource, digest, 1<<20); err == nil || !strings.Contains(err.Error(), "host /usr") {
			t.Fatalf("host runtime source %q error = %v", hostSource, err)
		}
	}
	wrong, err := NewRuntimeTree(source, testDigest("a"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := stageRuntime(ctx, wrong, filepath.Join(t.TempDir(), "wrong"), 1<<20); err == nil ||
		!strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("wrong digest error = %v", err)
	}
	if _, _, err := stageRuntime(ctx, runtime, filepath.Join(t.TempDir(), "bounded"), 2); err == nil ||
		!strings.Contains(err.Error(), "2-byte") {
		t.Fatalf("executor runtime ceiling error = %v", err)
	}
	writeRuntimeFile(t, filepath.Join(source, "bin", "later"), []byte("changed"), 0o755)
	if _, _, err := stageRuntime(ctx, runtime, filepath.Join(t.TempDir(), "changed"), 1<<20); err == nil ||
		!strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("changed source error = %v", err)
	}
}

func TestRuntimeTreeRejectsSymlinksOutsideVirtualUsr(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	escaping := t.TempDir()
	if err := os.Mkdir(filepath.Join(escaping, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/workspace/tool", filepath.Join(escaping, "bin", "tool")); err != nil {
		t.Fatal(err)
	}
	digest, _, err := workspace.Measure(ctx, escaping, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	escapingRuntime, err := NewRuntimeTree(escaping, digest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := stageRuntime(ctx, escapingRuntime, filepath.Join(t.TempDir(), "escaping"), 1<<20); err == nil ||
		!strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escaping symlink stage error = %v", err)
	}

	safe := t.TempDir()
	writeRuntimeFile(t, filepath.Join(safe, "lib", "tool"), []byte("tool"), 0o755)
	if err := os.Mkdir(filepath.Join(safe, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../lib/tool", filepath.Join(safe, "bin", "tool")); err != nil {
		t.Fatal(err)
	}
	digest, _, err = workspace.Measure(ctx, safe, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntimeTree(safe, digest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(t.TempDir(), "staged")
	if observed, _, err := stageRuntime(ctx, runtime, staged, 1<<20); err != nil || observed != digest {
		t.Fatalf("stage safe runtime = %q, %v", observed, err)
	}
}

func TestRuntimeTreeRejectsSourceIdentitySwapBeforeTraversal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	parent := t.TempDir()
	source := filepath.Join(parent, "runtime")
	writeRuntimeFile(t, filepath.Join(source, "bin", "tool"), []byte("tool"), 0o755)
	digest, _, err := workspace.Measure(ctx, source, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntimeTree(source, digest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, filepath.Join(parent, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr", source); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stageRuntime(ctx, runtime, filepath.Join(parent, "staged"), 1<<20); err == nil ||
		!strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("source swap error = %v", err)
	}
}

func TestRuntimeSymlinkVirtualRootMapping(t *testing.T) {
	t.Parallel()
	for path, target := range map[string]string{
		"bin/a":    "../lib/a",
		"bin/b":    "/usr/lib/b",
		"bin/c":    "/lib/c",
		"bin/d":    "/lib64/d",
		"bin/e":    "/bin/e",
		"lib/link": "nested/value",
	} {
		if !runtimeSymlinkStaysInside(path, target) {
			t.Errorf("safe target %q -> %q was rejected", path, target)
		}
	}
	for _, target := range []string{"../../workspace/tool", "/workspace/tool", "/etc/passwd", "/usr/../../tmp/tool"} {
		if runtimeSymlinkStaysInside("bin/tool", target) {
			t.Errorf("escaping target %q was admitted", target)
		}
	}
}

func writeRuntimeFile(t *testing.T, path string, contents []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
}
