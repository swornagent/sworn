package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	gobuildinfo "debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestModuleHasOnlyTheAdmittedPackageSet(t *testing.T) {
	root := moduleRoot(t)
	command := exec.Command(
		"go",
		"list",
		"-f", "{{.ImportPath}}",
		"./cmd/sworn",
		"./internal/...",
		"./tools/...",
	)
	command.Dir = root
	command.Env = cleanEnvironment(map[string]string{
		"GOFLAGS":     "-buildvcs=false",
		"GOWORK":      "off",
		"GOTOOLCHAIN": "local",
	})
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v: %s", err, output)
	}
	got := strings.Fields(string(output))
	sort.Strings(got)
	want := []string{
		"github.com/swornagent/sworn/cmd/sworn",
		"github.com/swornagent/sworn/internal/baton",
		"github.com/swornagent/sworn/internal/driver",
		"github.com/swornagent/sworn/internal/gitx",
		"github.com/swornagent/sworn/internal/journal",
		"github.com/swornagent/sworn/internal/runtime",
		"github.com/swornagent/sworn/tools/batonassets",
		"github.com/swornagent/sworn/tools/batongolden",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("module packages = %v, want %v", got, want)
	}
}

func TestBuiltBinaryHasNoLegacySymbolsOrVCSSettings(t *testing.T) {
	binary := buildCurrentSworn(t)
	nm, err := exec.Command("go", "tool", "nm", binary).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool nm: %v: %s", err, nm)
	}
	for _, packagePath := range []string{
		"internal/adapter",
		"internal/app",
		"internal/board",
		"internal/buildinfo",
		"internal/config",
		"internal/control",
		"internal/effects",
		"internal/engine",
		"internal/executor",
		"internal/policy",
		"internal/producer",
		"internal/protocol",
		"internal/repo",
		"internal/store",
		"internal/workspace",
	} {
		if bytes.Contains(nm, []byte("github.com/swornagent/sworn/"+packagePath)) {
			t.Fatalf("official binary contains legacy package %q", packagePath)
		}
	}
	info, err := gobuildinfo.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range info.Settings {
		if strings.HasPrefix(setting.Key, "vcs") {
			t.Fatalf("official binary retained %q=%q", setting.Key, setting.Value)
		}
	}
}

func TestTwinProductBuildsIgnoreRecordOnlyHistory(t *testing.T) {
	root := moduleRoot(t)
	plain := copyProductTree(t, root)
	withRecord := copyProductTree(t, root)
	for _, repository := range []string{plain, withRecord} {
		initProductRepository(t, repository)
	}

	record := filepath.Join(withRecord, ".baton", "releases", "proof", "status.json")
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte("{\"status\":\"record-only\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, withRecord, "add", "--", ".baton/releases/proof/status.json")
	runGit(
		t,
		withRecord,
		"-c", "user.name=Sworn Admission Test",
		"-c", "user.email=sworn-admission@example.invalid",
		"commit", "--quiet", "-m", "record only",
	)

	first := filepath.Join(t.TempDir(), "sworn-first")
	second := filepath.Join(t.TempDir(), "sworn-second")
	buildOfficialSworn(t, plain, first, "./cmd/sworn")
	buildOfficialSworn(t, withRecord, second, "./cmd/sworn")
	firstBody, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBody, secondBody) {
		t.Fatalf(
			"twin binaries differ: first=%x second=%x",
			sha256.Sum256(firstBody),
			sha256.Sum256(secondBody),
		)
	}
	for _, root := range []string{plain, withRecord} {
		if bytes.Contains(firstBody, []byte(root)) {
			t.Fatalf("trimmed binary contains temporary product root %q", root)
		}
	}
}

func TestProductCopyAndArchiveExcludeBatonRecords(t *testing.T) {
	root := moduleRoot(t)
	repository := copyProductTree(t, root)
	initProductRepository(t, repository)

	record := filepath.Join(repository, ".baton", "releases", "proof", "status.json")
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte("{\"status\":\"record-only\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "--", ".baton/releases/proof/status.json")
	runGit(
		t,
		repository,
		"-c", "user.name=Sworn Admission Test",
		"-c", "user.email=sworn-admission@example.invalid",
		"commit", "--quiet", "-m", "record only",
	)
	for name := range archiveEntries(t, repository, "HEAD") {
		if name == ".baton/releases" || strings.HasPrefix(name, ".baton/releases/") {
			t.Fatalf("Git archive contains Baton authority path %q", name)
		}
	}

	copy := copyProductTree(t, repository)
	if _, err := os.Lstat(filepath.Join(copy, ".baton")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("product copy materialized Baton authority: %v", err)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func copyProductTree(t *testing.T, sourceRoot string) string {
	t.Helper()
	command := exec.Command(
		"git",
		"-C", sourceRoot,
		"ls-files", "-z",
		"--cached", "--others", "--exclude-standard",
		"--", ".",
		":(exclude,top).baton/releases",
		":(exclude,top).baton/releases/**",
	)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	targetRoot := t.TempDir()
	for _, raw := range bytes.Split(output, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		relative := string(raw)
		if relative == ".baton/releases" || strings.HasPrefix(relative, ".baton/releases/") {
			t.Fatalf("product file list contains Baton authority path %q", relative)
		}
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relative))
		targetPath := filepath.Join(targetRoot, filepath.FromSlash(relative))
		info, err := os.Lstat(sourcePath)
		if err != nil {
			t.Fatalf("inspect product path %q: %v", relative, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatal(err)
		}
		switch {
		case info.Mode().IsRegular():
			body, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(targetPath, body, info.Mode().Perm()); err != nil {
				t.Fatal(err)
			}
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(link, targetPath); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unsupported product path mode %s for %q", info.Mode(), relative)
		}
	}
	return targetRoot
}

func initProductRepository(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", "--all")
	runGit(
		t,
		root,
		"-c", "user.name=Sworn Admission Test",
		"-c", "user.email=sworn-admission@example.invalid",
		"commit", "--quiet", "-m", "product",
	)
}

func buildOfficialSworn(t *testing.T, moduleRoot, output, source string) {
	t.Helper()
	command := exec.Command(
		"go",
		"build",
		"-mod=readonly",
		"-buildvcs=false",
		"-trimpath",
		"-o", output,
		source,
	)
	command.Dir = moduleRoot
	command.Env = cleanEnvironment(map[string]string{
		"CGO_ENABLED": "0",
		"GOCACHE":     t.TempDir(),
		"GOMODCACHE":  t.TempDir(),
		"GOPATH":      t.TempDir(),
		"GOFLAGS":     "-buildvcs=false",
		"GOPROXY":     "off",
		"GOSUMDB":     "off",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
	outputBytes, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build official Sworn binary: %v: %s", err, outputBytes)
	}
}

func buildCurrentSworn(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "sworn")
	buildOfficialSworn(t, ".", binary, ".")
	return binary
}

func cleanEnvironment(overrides map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		environment = append(environment, entry)
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, key+"="+overrides[key])
	}
	return environment
}

func archiveEntries(t *testing.T, repository, treeish string) map[string]struct{} {
	t.Helper()
	output, err := exec.Command("git", "-C", repository, "archive", "--format=tar", treeish).Output()
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]struct{})
	reader := tar.NewReader(bytes.NewReader(output))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return entries
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = struct{}{}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			t.Fatal(err)
		}
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func assertProcessExit(t *testing.T, err error, want int) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != want {
		t.Fatalf("process error = %v, want exit status %d", err, want)
	}
}
