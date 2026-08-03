package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

func initTestProject(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com",
			"commit", "-q", "--allow-empty", "-m", "base"},
	} {
		command := exec.Command(git, args...)
		command.Dir = resolved
		command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return resolved
}

// The project directory holds absolute host paths, binary digests, and the run
// journal. None of it belongs in a repository other people clone, so init must
// exclude the whole directory rather than trusting each writer to remember.
func TestInitExcludesTheProjectDirectoryFromGit(t *testing.T) {
	root := initTestProject(t)
	var stdout, stderr bytes.Buffer
	runInit([]string{"--project", root}, &stdout, &stderr)

	body, err := os.ReadFile(filepath.Join(root, ".sworn", ".gitignore"))
	if err != nil {
		t.Fatalf("no .gitignore written: %v", err)
	}
	if strings.TrimSpace(string(body)) != "*" {
		t.Fatalf("gitignore = %q, want everything excluded", body)
	}
	for _, directory := range []string{".sworn", ".sworn/runs"} {
		info, err := os.Stat(filepath.Join(root, directory))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s was not created as a directory: %v", directory, err)
		}
	}
}

// Run definitions record the exact fingerprint of the connection file, so
// silently rewriting it would invalidate them. Refusing is the safe default.
func TestInitRefusesToReplaceAnExistingConnectionFile(t *testing.T) {
	root := initTestProject(t)
	home := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "drivers.json")
	original := []byte(`{"schema_version":"kept"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root}, &stdout, &stderr); code == 0 {
		t.Fatal("init reported success while refusing to write")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatalf("existing connection file was modified: %q", current)
	}
	if !strings.Contains(stderr.String(), "--force") {
		t.Fatalf("refusal does not name the way forward: %s", stderr.String())
	}
	// The closing advice must not claim the file is missing when it is present.
	if strings.Contains(stdout.String(), "until an AI connection file exists") {
		t.Fatalf("refusal contradicts itself: %s", stdout.String())
	}
}

// init is the first command run in a project, so it must work from wherever the
// operator happens to be standing.
func TestInitResolvesTheProjectFromASubdirectory(t *testing.T) {
	root := initTestProject(t)
	nested := filepath.Join(root, "deep", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(working)
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	runInit(nil, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "Project: "+root) {
		t.Fatalf("did not resolve the project root: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".sworn", "runs")); err != nil {
		t.Fatalf("project was not prepared at the root: %v", err)
	}
}

func TestInitRefusesOutsideAGitProject(t *testing.T) {
	outside := t.TempDir()
	resolved, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", resolved}, &stdout, &stderr); code == 0 {
		t.Fatal("init reported success outside a Git project")
	}
	if _, err := os.Stat(filepath.Join(resolved, ".sworn")); !os.IsNotExist(err) {
		t.Fatal("init created a project directory outside a Git project")
	}
}

func TestInitRejectsRelativeProjectPaths(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", "relative/path"}, &stdout, &stderr); code == 0 {
		t.Fatal("init accepted a relative project path")
	}
}

// The launcher a package manager puts on PATH may be a script that executes a
// platform build elsewhere. Sworn must record the binary it will actually run.
func TestResolveCodexBinaryFindsThePlatformBuild(t *testing.T) {
	root := t.TempDir()
	launcher := filepath.Join(root, "bin", "codex.js")
	platform := filepath.Join(
		root, "node_modules", "@openai",
		"codex-linux-x64", "vendor", "x86_64-unknown-linux-musl", "bin",
	)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(platform, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(platform, "codex")
	if err := os.WriteFile(binary, []byte("ELF"), 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveCodexBinary(launcher)
	if err != nil {
		t.Fatalf("platform build not found: %v", err)
	}
	if resolved != binary {
		t.Fatalf("resolved = %q, want %q", resolved, binary)
	}
}

func TestAgentReportedVersionReadsEachFamilyFormat(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		family driver.ProfileFamily
		output string
		want   string
		ok     bool
	}{
		{"codex", driver.ProfileCodex, "codex-cli 0.146.0\n", "0.146.0", true},
		{"claude", driver.ProfileClaude, "2.1.220 (Claude Code)\n", "2.1.220", true},
		{"codex unexpected", driver.ProfileCodex, "something else\n", "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			value, ok := agentReportedVersion(testCase.family, testCase.output)
			if ok != testCase.ok || value != testCase.want {
				t.Fatalf("got (%q, %v), want (%q, %v)",
					value, ok, testCase.want, testCase.ok)
			}
		})
	}
}
