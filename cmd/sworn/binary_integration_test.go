//go:build linux

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/swornagent/sworn/internal/app"
	"github.com/swornagent/sworn/internal/repo"
)

func TestBuiltSwornBinaryEntersProductionRunComposition(t *testing.T) {
	binary := buildSwornForProcessTest(t)

	t.Run("dispatches through the production application", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent-run-config.json")
		command := exec.Command(binary, "run", "run-1", "--config", missing, "--json")
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		assertProcessExit(t, err, 1)
		if stdout.Len() != 0 {
			t.Fatalf("built binary stdout = %q, want no result on composition failure", stdout.String())
		}
		if !strings.Contains(stderr.String(), "sworn run: resolve run config:") {
			t.Fatalf("built binary stderr = %q, want production config-loader failure", stderr.String())
		}
	})

	t.Run("reaches and enforces the exact Codex pin without executing it", func(t *testing.T) {
		configuration, credentialName, credential, marker := completeRejectedCodexConfig(t)
		encoded, err := json.Marshal(configuration)
		if err != nil {
			t.Fatal(err)
		}
		configPath := filepath.Join(t.TempDir(), "run.json")
		if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
			t.Fatal(err)
		}

		command := exec.Command(
			binary, "run", "run-1", "work-1", "--config", configPath, "--json",
		)
		command.Env = append(os.Environ(), credentialName+"="+credential)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err = command.Run()
		assertProcessExit(t, err, 1)
		if stdout.Len() != 0 {
			t.Fatalf("built binary stdout = %q, want no result on rejected pin", stdout.String())
		}
		if !strings.Contains(stderr.String(), "configure pinned Codex builder: Codex binary size") ||
			!strings.Contains(stderr.String(), "is not pinned profile") {
			t.Fatalf("built binary stderr = %q, want exact Codex pin rejection", stderr.String())
		}
		if strings.Contains(stderr.String(), credential) {
			t.Fatal("built binary disclosed the configured credential")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("rejected Codex executable ran; marker stat error = %v", err)
		}
	})
}

func buildSwornForProcessTest(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "sworn")
	command := exec.Command("go", "build", "-o", binary, ".")
	command.Dir = "."
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build sworn process boundary: %v: %s", err, output)
	}
	return binary
}

func completeRejectedCodexConfig(t *testing.T) (app.Config, string, string, string) {
	t.Helper()
	root := t.TempDir()
	repositoryRoot := filepath.Join(root, "repository")
	if output, err := exec.Command("git", "init", "--quiet", repositoryRoot).CombinedOutput(); err != nil {
		t.Fatalf("initialize exact repository: %v: %s", err, output)
	}
	binding, err := repo.Discover(t.Context(), repositoryRoot, "repo-1")
	if err != nil {
		t.Fatalf("discover exact repository binding: %v", err)
	}

	writableRoot := executableTmpfsDirectory(t)
	privateDirectories := map[string]string{
		"runtime": filepath.Join(root, "executor-runtime"),
		"builder": filepath.Join(root, "builder-workspaces"),
		"checks":  filepath.Join(root, "check-workspaces"),
		"content": filepath.Join(root, "content-runtime"),
	}
	for label, path := range privateDirectories {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create %s directory: %v", label, err)
		}
	}
	controlDatabase := filepath.Join(root, "control.db")
	if err := os.WriteFile(controlDatabase, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(root, "rejected-codex-ran")
	codex := filepath.Join(root, "codex")
	program := fmt.Sprintf("#!/bin/sh\nprintf ran > %q\n", marker)
	if err := os.WriteFile(codex, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}

	credentialName := "SWORN_BINARY_TEST_CODEX_KEY"
	credential := "token-that-must-not-leak-from-built-binary"
	configuration := app.Config{
		SchemaVersion:   app.RunConfigSchemaVersion,
		ControlDatabase: controlDatabase,
		Repository: app.RepositoryConfig{
			Root: repositoryRoot, Binding: binding,
		},
		Authority: app.AuthorityConfig{Sources: []app.AuthoritySource{{
			SourceRef: "authority-source-1", AuthorizerRef: "authority-key-1",
			PublicKey:       base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)),
			BundleDirectory: filepath.Join(root, "authority-bundles"),
		}}},
		Executor: app.ExecutorConfig{
			RuntimeRoot: privateDirectories["runtime"], WritableRoot: writableRoot,
			Bubblewrap: exactExecutable(t, "bwrap"), SystemdRun: exactExecutable(t, "systemd-run"),
			Systemctl: exactExecutable(t, "systemctl"),
		},
		ContentRuntime: app.ContentRuntime{
			Source: privateDirectories["content"],
			Digest: "sha256:" + strings.Repeat("a", 64), MaximumBytes: 1 << 20,
		},
		Workspaces: app.WorkspaceConfig{
			BuilderRoot: privateDirectories["builder"], CheckRoot: privateDirectories["checks"],
		},
		Codex: app.CodexConfig{
			Binary: codex, Model: "gpt-5.4", TimeoutSeconds: 60,
			CredentialEnvironment: credentialName,
		},
	}
	return configuration, credentialName, credential, marker
}

func executableTmpfsDirectory(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.Getenv("XDG_RUNTIME_DIR"), fmt.Sprintf("/run/user/%d", os.Getuid()), "/dev/shm",
	}
	const (
		tmpfsMagic = 0x01021994
		noExecFlag = 0x8
	)
	for _, parent := range candidates {
		if parent == "" {
			continue
		}
		var filesystem syscall.Statfs_t
		if err := syscall.Statfs(parent, &filesystem); err != nil ||
			filesystem.Type != tmpfsMagic || filesystem.Blocks == 0 ||
			filesystem.Flags&noExecFlag != 0 {
			continue
		}
		root, err := os.MkdirTemp(parent, "sworn-binary-test-")
		if err != nil {
			continue
		}
		if err := os.Chmod(root, 0o700); err != nil {
			_ = os.RemoveAll(root)
			continue
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) })
		return root
	}
	t.Skip("exact Codex pin process proof requires an executable finite tmpfs")
	return ""
}

func exactExecutable(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is unavailable: %v", name, err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func assertProcessExit(t *testing.T, err error, want int) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != want {
		t.Fatalf("process error = %v, want exit status %d", err, want)
	}
}
