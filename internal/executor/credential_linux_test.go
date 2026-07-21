//go:build linux

package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCredentialFileLeaseAllowsInPlaceRefresh(t *testing.T) {
	path := newTestCredentialFile(t, "initial-credential")
	lease, err := acquireCredentialFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseCredentialFile(lease) })

	refreshed, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := refreshed.WriteString("refreshed-credential"); err != nil {
		_ = refreshed.Close()
		t.Fatal(err)
	}
	if err := refreshed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.validate(); err != nil {
		t.Fatalf("validate retained credential after refresh: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "refreshed-credential" {
		t.Fatalf("credential contents = %q, %v", contents, err)
	}
}

func TestCredentialFileLeaseRejectsContention(t *testing.T) {
	path := newTestCredentialFile(t, "credential")
	lease, err := acquireCredentialFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseCredentialFile(lease) })

	contending, err := acquireCredentialFile(path)
	if contending != nil {
		_ = releaseCredentialFile(contending)
	}
	if err == nil || !strings.Contains(err.Error(), "credential file is busy") {
		t.Fatalf("contending acquisition error = %v", err)
	}
}

func TestUnprovenQuiescenceLeavesInheritedCredentialLockHeld(t *testing.T) {
	path := newTestCredentialFile(t, `{"auth_mode":"chatgpt"}`)
	lease, err := acquireCredentialFile(path)
	if err != nil {
		t.Fatal(err)
	}
	inheritedFD, err := syscall.Dup(int(lease.file.Fd()))
	if err != nil {
		_ = releaseCredentialFile(lease)
		t.Fatal(err)
	}
	syscall.CloseOnExec(inheritedFD)
	inherited := os.NewFile(uintptr(inheritedFD), "inherited-credential")
	if inherited == nil {
		_ = syscall.Close(inheritedFD)
		_ = releaseCredentialFile(lease)
		t.Fatal("duplicate credential descriptor is invalid")
	}
	if err := finishCredentialFile(lease, errors.New("service quiescence is unproven")); err != nil {
		_ = inherited.Close()
		t.Fatal(err)
	}
	if contender, err := acquireCredentialFile(path); err == nil {
		_ = releaseCredentialFile(contender)
		_ = inherited.Close()
		t.Fatal("credential lock was released while an unproven child descriptor remained live")
	} else if !strings.Contains(err.Error(), "credential file is busy") {
		_ = inherited.Close()
		t.Fatalf("contending credential acquisition = %v, want busy", err)
	}
	if err := inherited.Close(); err != nil {
		t.Fatal(err)
	}
	reacquired, err := acquireCredentialFile(path)
	if err != nil {
		t.Fatalf("credential lock did not release after inherited descriptor closed: %v", err)
	}
	if err := releaseCredentialFile(reacquired); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialFileLeaseRejectsFileIdentitySwap(t *testing.T) {
	path := newTestCredentialFile(t, "credential")
	lease, err := acquireCredentialFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseCredentialFile(lease) })

	if err := os.Rename(path, path+".replaced"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.validate(); err == nil || !strings.Contains(err.Error(), "file path identity changed") {
		t.Fatalf("identity-swap validation error = %v", err)
	}
}

func TestCredentialFileLeaseRejectsParentIdentitySwap(t *testing.T) {
	path := newTestCredentialFile(t, "credential")
	lease, err := acquireCredentialFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseCredentialFile(lease) })

	parent := filepath.Dir(path)
	if err := os.Rename(parent, parent+".replaced"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.validate(); err == nil || !strings.Contains(err.Error(), "directory path identity changed") {
		t.Fatalf("parent identity-swap validation error = %v", err)
	}
}

func TestCredentialFileValidationRejectsUnsafeSources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string) string
		want   string
	}{
		{
			name: "non-private parent",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				if err := os.Chmod(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: "directory mode must be exactly 0700",
		},
		{
			name: "symbolic-link parent",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				alias := filepath.Join(filepath.Dir(filepath.Dir(path)), "codex-alias")
				if err := os.Symlink(filepath.Dir(path), alias); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(alias, filepath.Base(path))
			},
			want: "symbolic-link remap",
		},
		{
			name: "symbolic-link file",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				alias := filepath.Join(filepath.Dir(path), "auth-alias.json")
				if err := os.Symlink(path, alias); err != nil {
					t.Fatal(err)
				}
				return alias
			},
			want: "symbolic-link remap",
		},
		{
			name: "permissive file mode",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				if err := os.Chmod(path, 0o640); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: "file mode must be exactly 0600",
		},
		{
			name: "empty file",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				if err := os.Truncate(path, 0); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: "must contain 1 to",
		},
		{
			name: "oversized file",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				if err := os.Truncate(path, maximumCredentialFileBytes+1); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: "must contain 1 to",
		},
		{
			name: "directory instead of file",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: "non-symlink regular file",
		},
		{
			name: "multiple hard links",
			mutate: func(t *testing.T, path string) string {
				t.Helper()
				if err := os.Link(path, path+".alias"); err != nil {
					t.Fatal(err)
				}
				return path
			},
			want: "exactly one hard link",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.mutate(t, newTestCredentialFile(t, "credential"))
			lease, err := acquireCredentialFile(path)
			if lease != nil {
				_ = releaseCredentialFile(lease)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("credential validation error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestCredentialFileValidationRejectsForeignOwnership(t *testing.T) {
	path := newTestCredentialFile(t, "credential")
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	foreignUID := uint32(os.Geteuid()) + 1
	if foreignUID == uint32(os.Geteuid()) {
		foreignUID--
	}

	parentIdentity := *parentInfo.Sys().(*syscall.Stat_t)
	parentIdentity.Uid = foreignUID
	if err := validateCredentialParentInfo(identityOverrideFileInfo{
		FileInfo: parentInfo,
		identity: &parentIdentity,
	}); err == nil || !strings.Contains(err.Error(), "directory must be owned") {
		t.Fatalf("foreign parent ownership error = %v", err)
	}

	fileIdentity := *fileInfo.Sys().(*syscall.Stat_t)
	fileIdentity.Uid = foreignUID
	if err := validateCredentialFileInfo(identityOverrideFileInfo{
		FileInfo: fileInfo,
		identity: &fileIdentity,
	}); err == nil || !strings.Contains(err.Error(), "file must be owned") {
		t.Fatalf("foreign file ownership error = %v", err)
	}
}

func TestBubblewrapCredentialMountIsExplicitAndWritable(t *testing.T) {
	t.Parallel()
	executor := &LinuxExecutor{options: Options{Limits: DefaultLimits()}}
	invocation := Invocation{
		CredentialAccess: true,
		Argv:             []string{"/usr/bin/true"},
		Network:          NetworkNone,
	}
	arguments := executor.bubblewrapArgs(invocation, "/usr", "/workspace", "/inputs", true)
	if !containsArgumentSequence(arguments, []string{"--dir", CredentialHome}) {
		t.Fatalf("credential directory absent from arguments: %q", arguments)
	}
	if !containsArgumentSequence(arguments, []string{
		"--bind-fd", "4", CredentialFileTarget,
	}) {
		t.Fatalf("writable credential-file mount absent from arguments: %q", arguments)
	}
	if containsArgumentSequence(arguments, []string{
		"--ro-bind-fd", "4", CredentialFileTarget,
	}) {
		t.Fatalf("credential file was mounted read-only: %q", arguments)
	}
	if !containsArgumentSequence(arguments, []string{"--setenv", "CODEX_HOME", CredentialHome}) {
		t.Fatalf("fixed CODEX_HOME absent from arguments: %q", arguments)
	}

	blind := invocation
	blind.CredentialAccess = false
	blindArguments := executor.bubblewrapArgs(blind, "/usr", "/workspace", "/inputs", true)
	if containsString(blindArguments, CredentialFileTarget) ||
		containsString(blindArguments, CredentialHome) ||
		containsString(blindArguments, "CODEX_HOME") {
		t.Fatalf("blind invocation received credential state: %q", blindArguments)
	}
}

func TestContentBoundEntryPointRejectsCredentialAccess(t *testing.T) {
	t.Parallel()
	executor := &LinuxExecutor{options: Options{
		Limits:              DefaultLimits(),
		CredentialFile:      "/private/codex/auth.json",
		AllowCredentialFile: true,
	}}
	invocation := Invocation{
		SchemaVersion:    InvocationSchemaVersion,
		ID:               "credential-check-denial",
		Role:             "check",
		CredentialAccess: true,
		Workspace:        "/workspace/source",
		WorkspaceDigest:  testDigest("a"),
		WorkspaceAccess:  WorkspaceReadOnly,
		Argv:             []string{"/usr/bin/true"},
		Network:          NetworkNone,
		Timeout:          time.Second,
	}
	if _, err := executor.RunContentBound(context.Background(), invocation, RuntimeTree{}); err == nil ||
		!strings.Contains(err.Error(), "writable executor entry point") {
		t.Fatalf("content-bound credential-access error = %v", err)
	}
}

type identityOverrideFileInfo struct {
	os.FileInfo
	identity *syscall.Stat_t
}

func (info identityOverrideFileInfo) Sys() any { return info.identity }

func newTestCredentialFile(t *testing.T, contents string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "codex")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "auth.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
