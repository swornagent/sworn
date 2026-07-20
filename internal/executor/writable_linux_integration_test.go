//go:build linux

package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	repositoryPackage "github.com/swornagent/sworn/internal/repo"
)

func TestWritableRootRejectsNonTmpfs(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(root, &filesystem); err != nil {
		t.Fatal(err)
	}
	if filesystem.Type == tmpfsMagic {
		t.Skip("test temporary directory already uses tmpfs")
	}
	if err := ensureWritableRoot(root); err == nil || !strings.Contains(err.Error(), "finite tmpfs") {
		t.Fatalf("ordinary filesystem writable root error = %v", err)
	}
}

func TestLinuxExecutorExportsMeasuredWritableWorkspace(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "edit.txt"), []byte("before"), 0o640)
	writeTestFile(t, filepath.Join(source, "delete.txt"), []byte("delete"), 0o600)
	writeTestFile(t, filepath.Join(source, "rename.txt"), []byte("rename"), 0o644)
	external := filepath.Join(t.TempDir(), "external.txt")
	writeTestFile(t, external, []byte("external"), 0o644)
	if err := os.Link(external, filepath.Join(source, "linked-source.txt")); err != nil {
		t.Fatal(err)
	}
	digest, _, err := MeasureWorkspace(context.Background(), source, executor.options.Limits.InputBytes)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Join([]string{
		"import os, pathlib",
		"root = pathlib.Path('/workspace')",
		"root.joinpath('edit.txt').write_text('after')",
		"root.joinpath('delete.txt').unlink()",
		"root.joinpath('rename.txt').rename(root.joinpath('renamed.txt'))",
		"root.joinpath('new.txt').write_text('new')",
		"os.chmod(root.joinpath('new.txt'), 0o751)",
		"root.joinpath('link').symlink_to('new.txt')",
		"root.joinpath('linked-source.txt').write_text('contained')",
		"os.link(root.joinpath('new.txt'), root.joinpath('hardlink.txt'))",
		"print('built')",
	}, "\n")
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-export",
		Role:            "builder",
		Workspace:       source,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/python3", "-c", program},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err != nil {
		t.Fatalf("run writable: %v; stderr=%s", err, completion.Stderr)
	}
	if completion.ExitCode != 0 || string(completion.Stdout) != "built\n" || completion.Export == nil {
		t.Fatalf("writable completion = %#v", completion)
	}
	export := *completion.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, export) })
	if contents, err := os.ReadFile(filepath.Join(source, "edit.txt")); err != nil || string(contents) != "before" {
		t.Fatalf("source workspace changed: %q, %v", contents, err)
	}
	assertWorkspaceFile(t, filepath.Join(export.Path, "edit.txt"), "after", 0o640)
	assertWorkspaceFile(t, filepath.Join(export.Path, "renamed.txt"), "rename", 0o644)
	assertWorkspaceFile(t, filepath.Join(export.Path, "new.txt"), "new", 0o751)
	assertWorkspaceFile(t, filepath.Join(export.Path, "linked-source.txt"), "contained", 0o644)
	assertWorkspaceFile(t, filepath.Join(export.Path, "hardlink.txt"), "new", 0o751)
	assertWorkspaceFile(t, external, "external", 0o644)
	newInfo, err := os.Stat(filepath.Join(export.Path, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	hardlinkInfo, err := os.Stat(filepath.Join(export.Path, "hardlink.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(newInfo, hardlinkInfo) {
		t.Fatal("builder-created internal hardlink was not preserved in the raw export")
	}
	if _, err := os.Lstat(filepath.Join(export.Path, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted path remains: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(export.Path, "link")); err != nil || target != "new.txt" {
		t.Fatalf("exported symlink = %q, %v", target, err)
	}
	observedDigest, observedBytes, err := MeasureWorkspace(context.Background(), export.Path, executor.options.Limits.WorkspaceBytes)
	if err != nil || observedDigest != export.Digest || observedBytes != export.Bytes {
		t.Fatalf("export measurement = (%s, %d, %v), want (%s, %d)", observedDigest, observedBytes, err, export.Digest, export.Bytes)
	}
	if err := executor.ValidateExport(context.Background(), export); err != nil {
		t.Fatalf("validate export: %v", err)
	}
	if err := executor.DiscardExport(context.Background(), export); err != nil {
		t.Fatalf("discard export: %v", err)
	}
	if _, err := os.Lstat(export.Path); !os.IsNotExist(err) {
		t.Fatalf("released export remains: %v", err)
	}
}

func TestLinuxExecutorRejectsUnsafeWritableExports(t *testing.T) {
	tests := []struct {
		name    string
		program string
		want    string
	}{
		{"git metadata", "import pathlib; pathlib.Path('/workspace/.git').mkdir()", "invalid path"},
		{"special file", "import os; os.mkfifo('/workspace/pipe')", "special file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := requireWritableLinuxExecutor(t)
			workspace, digest := emptyTestWorkspace(t, executor)
			invocationID := "unsafe-" + strings.ReplaceAll(test.name, " ", "-")
			completion, err := executor.RunWritable(context.Background(), Invocation{
				SchemaVersion:   InvocationSchemaVersion,
				ID:              invocationID,
				Role:            "builder",
				Workspace:       workspace,
				WorkspaceDigest: digest,
				WorkspaceAccess: WorkspaceWritableExport,
				Argv:            []string{"/usr/bin/python3", "-c", test.program},
				Network:         NetworkNone,
				Timeout:         10 * time.Second,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) || completion.Export != nil {
				t.Fatalf("completion=%#v, error=%v; want %q", completion, err, test.want)
			}
			assertNoWritableWorkspace(t, executor, invocationID)
		})
	}
}

func TestLinuxExecutorRejectsOversizeWritableExport(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	executor.options.Limits.WorkspaceBytes = 1024
	workspace, digest := emptyTestWorkspace(t, executor)
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-oversize",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/python3", "-c", "open('/workspace/large','wb').write(b'x'*2048)"},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds 1024-byte") || completion.Export != nil {
		t.Fatalf("completion=%#v, error=%v", completion, err)
	}
	assertNoWritableWorkspace(t, executor, "writable-oversize")
}

func TestLinuxExecutorCancellationDoesNotExportWorkspace(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	completion, err := executor.RunWritable(ctx, Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-cancel",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/python3", "-c", "import time; open('/workspace/partial','w').write('x'); time.sleep(60)"},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err != nil || !completion.Cancelled || completion.Export != nil {
		t.Fatalf("completion=%#v, error=%v", completion, err)
	}
	assertNoWritableWorkspace(t, executor, "writable-cancel")
}

func TestLinuxExecutorDiscardsChangedExport(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-changed-after-measurement",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/python3", "-c", "open('/workspace/result','w').write('measured')"},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err != nil || completion.Export == nil {
		t.Fatalf("run writable: completion=%#v, error=%v", completion, err)
	}
	export := *completion.Export
	t.Cleanup(func() { _ = removePrivateTree(export.Path) })
	if err := os.WriteFile(filepath.Join(export.Path, "result"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := executor.ValidateExport(context.Background(), export); err == nil || !strings.Contains(err.Error(), "changed after measurement") {
		t.Fatalf("changed export validation error = %v", err)
	}
	if err := executor.DiscardExport(context.Background(), export); err != nil {
		t.Fatalf("discard changed export: %v", err)
	}
	if _, err := os.Lstat(export.Path); !os.IsNotExist(err) {
		t.Fatalf("discarded changed export remains: %v", err)
	}
}

func TestLinuxExecutorQuarantinesNonzeroWorkspaceResult(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-nonzero-result",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/python3", "-c", "open('/workspace/partial','w').write('inspectable'); raise SystemExit(7)"},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err != nil || completion.ExitCode != 7 || completion.Export == nil {
		t.Fatalf("nonzero completion=%#v, error=%v", completion, err)
	}
	export := *completion.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, export) })
	if err := executor.ValidateExport(context.Background(), export); err != nil {
		t.Fatalf("validate quarantined result: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(export.Path, "partial"), "inspectable", 0o600)
}

func TestLinuxExecutorRejectsUnreconciledWritableIdentityCollision(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	invocation := Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-one-shot-invocation-id",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/true"},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	}
	runtimeCollision := invocation
	runtimeCollision.ID = "writable-runtime-residue"
	runtimePath := executor.writableRuntimePath(runtimeCollision.ID)
	if err := os.Mkdir(runtimePath, 0o700); err != nil {
		t.Fatal(err)
	}
	completion, err := executor.RunWritable(context.Background(), runtimeCollision)
	if err == nil || !strings.Contains(err.Error(), "unreconciled runtime residue") || completion.Export != nil {
		t.Fatalf("runtime collision completion=%#v, error=%v", completion, err)
	}
	cleanup, err := executor.ReconcileWritable(context.Background(), runtimeCollision.ID)
	if err != nil || cleanup.InvocationID() != runtimeCollision.ID {
		t.Fatalf("runtime collision cleanup=%#v, error=%v", cleanup, err)
	}
	if _, err := os.Lstat(runtimePath); !os.IsNotExist(err) {
		t.Fatalf("reconciled runtime residue remains: %v", err)
	}

	first, err := executor.RunWritable(context.Background(), invocation)
	if err != nil || first.Export == nil {
		t.Fatalf("first completion=%#v, error=%v", first, err)
	}
	firstExport := *first.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, firstExport) })
	wantGeneration := writableWorkspaceGeneration(invocation.ID)
	wantPath := executor.writableWorkspacePath(invocation.ID, wantGeneration)
	if firstExport.Generation != wantGeneration || firstExport.Path != wantPath {
		t.Fatalf("export binding = (%q, %q), want (%q, %q)", firstExport.Generation, firstExport.Path, wantGeneration, wantPath)
	}
	second, err := executor.RunWritable(context.Background(), invocation)
	if err == nil || !strings.Contains(err.Error(), "unreconciled workspace residue") || second.Export != nil {
		t.Fatalf("identity collision completion=%#v, error=%v", second, err)
	}
	if err := executor.ValidateExport(context.Background(), firstExport); err != nil {
		t.Fatalf("identity collision changed first export: %v", err)
	}
	cleanup, err = executor.ReconcileWritable(context.Background(), invocation.ID)
	if err != nil || cleanup.InvocationID() != invocation.ID {
		t.Fatalf("workspace collision cleanup=%#v, error=%v", cleanup, err)
	}
	if _, err := os.Lstat(firstExport.Path); !os.IsNotExist(err) {
		t.Fatalf("reconciled workspace residue remains: %v", err)
	}
}

func TestLinuxExecutorWaitsForDetachedWorkspaceWriters(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	workspace, digest := emptyTestWorkspace(t, executor)
	program := strings.Join([]string{
		"import os, time",
		"child = os.fork()",
		"if child == 0:",
		" os.setsid()",
		" os.close(1)",
		" os.close(2)",
		" time.sleep(0.4)",
		" open('/workspace/result', 'w').write('late')",
		" time.sleep(60)",
		" os._exit(0)",
		"open('/workspace/result', 'w').write('stable')",
	}, "\n")
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-detached-writer",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/python3", "-c", program},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err != nil || completion.ExitCode != 0 || completion.Export == nil {
		t.Fatalf("detached-writer completion=%#v, error=%v", completion, err)
	}
	export := *completion.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, export) })
	assertWorkspaceFile(t, filepath.Join(export.Path, "result"), "stable", 0o600)
	if err := executor.ValidateExport(context.Background(), export); err != nil {
		t.Fatalf("validate quiescent export: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if err := executor.ValidateExport(context.Background(), export); err != nil {
		t.Fatalf("detached writer changed export after completion: %v", err)
	}
}

func TestLinuxExecutorDoesNotExportControlFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*LinuxExecutor, *Invocation)
	}{
		{"systemd-run", func(executor *LinuxExecutor, _ *Invocation) { executor.options.SystemdRunPath = "/usr/bin/false" }},
		{"shim", func(executor *LinuxExecutor, _ *Invocation) {
			executor.options.ShimArgv = []string{"/definitely/missing/sworn-shim"}
		}},
		{"bubblewrap", func(executor *LinuxExecutor, _ *Invocation) { executor.options.BubblewrapPath = "/usr/bin/false" }},
		{"target exec", func(_ *LinuxExecutor, invocation *Invocation) {
			invocation.Argv[0] = "/usr/bin/definitely-missing-sworn-target"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := requireWritableLinuxExecutor(t)
			workspace, digest := emptyTestWorkspace(t, executor)
			invocation := Invocation{
				SchemaVersion:   InvocationSchemaVersion,
				ID:              "writable-control-" + strings.ReplaceAll(test.name, " ", "-"),
				Role:            "builder",
				Workspace:       workspace,
				WorkspaceDigest: digest,
				WorkspaceAccess: WorkspaceWritableExport,
				Argv:            []string{"/usr/bin/true"},
				Network:         NetworkNone,
				Timeout:         10 * time.Second,
			}
			test.mutate(executor, &invocation)
			completion, err := executor.RunWritable(context.Background(), invocation)
			if err == nil || completion.Export != nil {
				t.Fatalf("control failure became export: completion=%#v, error=%v", completion, err)
			}
			assertNoWritableWorkspace(t, executor, invocation.ID)
		})
	}
}

func TestLinuxExecutorExportFeedsExactGitCapture(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	ctx := context.Background()
	source := t.TempDir()
	runWritableGit(t, "init", "--quiet", "--initial-branch=main", source)
	runWritableGit(t, "-C", source, "config", "user.name", "Sworn Test")
	runWritableGit(t, "-C", source, "config", "user.email", "sworn@example.invalid")
	if err := os.Mkdir(filepath.Join(source, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(source, "src", "base.txt"), []byte("base\n"), 0o644)
	runWritableGit(t, "-C", source, "add", "--all")
	runWritableGit(t, "-C", source, "commit", "--quiet", "-m", "base")

	binding, err := repositoryPackage.Discover(ctx, source, "writable-handoff")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repositoryPackage.Open(ctx, source, binding)
	if err != nil {
		t.Fatal(err)
	}
	target, err := repository.BindTarget(ctx, "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := repository.Materialize(ctx, target, filepath.Join(t.TempDir(), "builder-seed"))
	if err != nil {
		t.Fatal(err)
	}
	digest, _, err := MeasureWorkspace(ctx, workspace.Path, executor.options.Limits.InputBytes)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Join([]string{
		"import os, pathlib",
		"path = pathlib.Path('/workspace/src/base.txt')",
		"path.write_text('candidate\\n')",
		"os.link(path, '/workspace/src/hardlink.txt')",
	}, "\n")
	completion, err := executor.RunWritable(ctx, Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-repo-handoff",
		Role:            "builder",
		Workspace:       workspace.Path,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv:            []string{"/usr/bin/python3", "-c", program},
		Network:         NetworkNone,
		Timeout:         10 * time.Second,
	})
	if err != nil || completion.ExitCode != 0 || completion.Export == nil {
		t.Fatalf("builder completion=%#v, error=%v", completion, err)
	}
	export := *completion.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, export) })
	if err := executor.ValidateExport(ctx, export); err != nil {
		t.Fatalf("validate immediate repository handoff: %v", err)
	}
	workspace.Path = export.Path
	candidate, err := repository.Capture(ctx, workspace, repositoryPackage.CaptureOptions{
		Scope:     repositoryPackage.Scope{Include: []string{"src"}},
		Timestamp: time.Unix(1_784_442_400, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("capture measured export: %v", err)
	}
	wantPaths := []string{"src/base.txt", "src/hardlink.txt"}
	if !reflect.DeepEqual(candidate.ChangedPaths, wantPaths) {
		t.Fatalf("candidate changed paths = %q, want %q", candidate.ChangedPaths, wantPaths)
	}
	for _, path := range wantPaths {
		if got := runWritableGit(t, "-C", source, "show", candidate.Commit+":"+path); got != "candidate\n" {
			t.Fatalf("candidate %s = %q", path, got)
		}
	}
}

func TestLinuxExecutorCgroupBoundsWritableTmpfsGrowth(t *testing.T) {
	executor := requireWritableLinuxExecutor(t)
	executor.options.Limits.MemoryBytes = 96 << 20
	executor.options.Limits.FileBytes = 256 << 20
	executor.options.Limits.WorkspaceBytes = 256 << 20
	workspace, digest := emptyTestWorkspace(t, executor)
	completion, err := executor.RunWritable(context.Background(), Invocation{
		SchemaVersion:   InvocationSchemaVersion,
		ID:              "writable-memory-bound",
		Role:            "builder",
		Workspace:       workspace,
		WorkspaceDigest: digest,
		WorkspaceAccess: WorkspaceWritableExport,
		Argv: []string{
			"/usr/bin/python3",
			"-c",
			"import os\nf=open('/workspace/growth','wb',buffering=0)\nb=b'x'*(1024*1024)\nfor _ in range(256):\n f.write(b)\n os.fsync(f.fileno())",
		},
		Network: NetworkNone,
		Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("run memory-bound workspace: %v; completion=%#v", err, completion)
	}
	if completion.ExitCode == 0 || completion.Export == nil {
		t.Fatalf("workspace growth was not killed by its cgroup: %#v", completion)
	}
	export := *completion.Export
	t.Cleanup(func() { _ = releaseOrRemoveExport(executor, export) })
	info, err := os.Stat(filepath.Join(export.Path, "growth"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= 0 || info.Size() >= 200<<20 {
		t.Fatalf("bounded workspace file size = %d", info.Size())
	}
}

func requireWritableLinuxExecutor(t *testing.T) *LinuxExecutor {
	t.Helper()
	parent := os.Getenv("XDG_RUNTIME_DIR")
	if parent == "" {
		if os.Getenv(requireLinuxExecutorEnvironment) == "1" {
			t.Fatal("XDG_RUNTIME_DIR is required for writable executor integration")
		}
		t.Skip("XDG_RUNTIME_DIR is unavailable")
	}
	writableRoot, err := os.MkdirTemp(parent, "sworn-writable-test-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(writableRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = removePrivateTree(writableRoot) })
	executor, err := newTestExecutorWithWritableRoot(t.TempDir(), writableRoot)
	if err == nil {
		_, err = executor.Probe(context.Background())
	}
	if err != nil {
		if os.Getenv(requireLinuxExecutorEnvironment) == "1" {
			t.Fatalf("required writable Linux executor capability: %v", err)
		}
		t.Skipf("writable Linux executor capability unavailable: %v", err)
	}
	return executor
}

func assertWorkspaceFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != want {
		t.Fatalf("workspace file %s = %q, %v", path, contents, err)
	}
	assertMode(t, path, mode)
}

func releaseOrRemoveExport(executor *LinuxExecutor, export WorkspaceExport) error {
	if err := executor.DiscardExport(context.Background(), export); err == nil {
		return nil
	}
	return removePrivateTree(export.Path)
}

func assertNoWritableWorkspace(t *testing.T, executor *LinuxExecutor, invocationID string) {
	t.Helper()
	prefix := strings.TrimSuffix(executor.unitName(invocationID), ".service") + "."
	matches, err := filepath.Glob(filepath.Join(executor.options.WritableRoot, prefix+"*.workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("writable workspace residue = %q", matches)
	}
}

func runWritableGit(t *testing.T, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
