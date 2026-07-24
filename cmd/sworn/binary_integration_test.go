//go:build linux

package main

import (
	"archive/tar"
	"bytes"
	"context"
	gobuildinfo "debug/buildinfo"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBuiltSwornBinaryRefusesRunAndShimInMaintenanceBootstrap(t *testing.T) {
	binary := buildSwornForProcessTest(t)

	t.Run("run command refuses", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "run-config.fifo")
		if err := syscall.Mkfifo(configPath, 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, "run", "run-1", "--config", configPath, "--json")
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatal("built run blocked while consuming the configuration path")
		}
		assertProcessExit(t, err, 1)
		if stdout.Len() != 0 {
			t.Fatalf("built run stdout = %q, want no JSON result", stdout.String())
		}
		if !strings.Contains(stderr.String(), "run is unavailable while v0.3 delivery is in maintenance bootstrap") {
			t.Fatalf("built run stderr = %q, want maintenance bootstrap refusal", stderr.String())
		}
		if strings.Contains(stderr.String(), "resolve run config") {
			t.Fatalf("built run reached production configuration path: %q", stderr.String())
		}
	})

	t.Run("shim entry point refuses", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "sworn.marker")
		command := exec.Command(binary, "__executor-shim", "--sworn-start-marker", marker)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		assertProcessExit(t, err, 1)
		if stdout.Len() != 0 {
			t.Fatalf("built shim stdout = %q, want no invocation output", stdout.String())
		}
		if !strings.Contains(stderr.String(), "__executor-shim is unavailable while v0.3 delivery is in maintenance bootstrap") {
			t.Fatalf("built shim stderr = %q, want maintenance bootstrap refusal", stderr.String())
		}
		if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("built shim touched start marker: %v", err)
		}
	})

	t.Run("board refuses before opening Baton record store", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), ".baton", "releases", "demo", "status.json")
		if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(storePath, 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, "board", "--store", storePath, "--json")
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatal("built board blocked while opening the Baton record store")
		}
		assertProcessExit(t, err, 1)
		if stdout.Len() != 0 {
			t.Fatalf("built board stdout = %q, want no projection", stdout.String())
		}
		if got := stderr.String(); got != "sworn: board is unavailable while v0.3 delivery is in maintenance bootstrap\n" {
			t.Fatalf("built board stderr = %q, want maintenance refusal", got)
		}
	})
}

func TestBuiltSwornBinaryExcludesLegacyOperationalPackages(t *testing.T) {
	binary := buildSwornForProcessTest(t)
	output, err := exec.Command("go", "tool", "nm", binary).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect built symbols: %v: %s", err, output)
	}
	for _, packagePath := range []string{
		"internal/app",
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
		if bytes.Contains(output, []byte("github.com/swornagent/sworn/"+packagePath)) {
			t.Fatalf("official binary retains legacy operational package %q", packagePath)
		}
	}
}

func TestBuiltSwornBinaryHasNoVCSSettings(t *testing.T) {
	binary := buildSwornForProcessTest(t)
	info, err := gobuildinfo.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range info.Settings {
		if strings.HasPrefix(setting.Key, "vcs") {
			t.Fatalf("official binary retained VCS setting %q=%q", setting.Key, setting.Value)
		}
	}
}

func TestBuiltSwornBinaryTwinBuildReproducibilityIgnoresBatonRecordRoot(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	plain := copyCandidateModuleToPlainTree(t, root)
	withRecord := copyCandidateModuleToPlainTree(t, root)
	for _, repository := range []string{plain, withRecord} {
		runGitForCandidateCopy(t, repository, "init", "--quiet")
		runGitForCandidateCopy(t, repository, "add", "--all")
		runGitForCandidateCopy(
			t,
			repository,
			"-c", "user.name=Sworn Bootstrap Test",
			"-c", "user.email=sworn-bootstrap@example.invalid",
			"commit", "--quiet", "-m", "product",
		)
	}

	first := filepath.Join(t.TempDir(), "sworn-first")
	buildOfficialSwornBinary(t, plain, first, "./cmd/sworn")

	recordRoot := filepath.Join(withRecord, ".baton", "releases", "maintenance-bootstrap", "status.json")
	if err := os.MkdirAll(filepath.Dir(recordRoot), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordRoot, []byte(`{"status":"maintenance-bootstrap"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForCandidateCopy(t, withRecord, "add", "--", ".baton/releases/maintenance-bootstrap/status.json")
	runGitForCandidateCopy(
		t,
		withRecord,
		"-c", "user.name=Sworn Bootstrap Test",
		"-c", "user.email=sworn-bootstrap@example.invalid",
		"commit", "--quiet", "-m", "record only",
	)

	second := filepath.Join(t.TempDir(), "sworn-second")
	buildOfficialSwornBinary(t, withRecord, second, "./cmd/sworn")

	firstBinary, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBinary, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBinary, secondBinary) {
		t.Fatal("rebuilding after adding .baton/releases status did not preserve binary bytes")
	}
}

func TestCandidateModuleCopyExcludesBatonRecordRoot(t *testing.T) {
	source := t.TempDir()
	runGitForCandidateCopy(t, source, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(source, "product.txt"), []byte("product\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(source, ".baton", "releases", "demo", "status.json")
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte(`{"status":"ready"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForCandidateCopy(t, source, "add", "--", "product.txt", ".baton/releases/demo/status.json")

	plain := copyCandidateModuleToPlainTree(t, source)
	if data, err := os.ReadFile(filepath.Join(plain, "product.txt")); err != nil || string(data) != "product\n" {
		t.Fatalf("copied product file = %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(plain, ".baton")); !os.IsNotExist(err) {
		t.Fatalf("candidate copy materialized Baton record root: %v", err)
	}
}

func TestGitArchiveExcludesBatonRecordRootWithoutChangingProductEntries(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	repository := copyCandidateModuleToPlainTree(t, root)
	runGitForCandidateCopy(t, repository, "init", "--quiet")
	runGitForCandidateCopy(t, repository, "add", "--all")
	runGitForCandidateCopy(
		t,
		repository,
		"-c", "user.name=Sworn Bootstrap Test",
		"-c", "user.email=sworn-bootstrap@example.invalid",
		"commit", "--quiet", "-m", "product",
	)
	product := archiveProductEntries(t, repository, "HEAD")

	record := filepath.Join(repository, ".baton", "releases", "demo", "status.json")
	if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte(`{"status":"ready"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForCandidateCopy(t, repository, "add", "--", ".baton/releases/demo/status.json")
	runGitForCandidateCopy(
		t,
		repository,
		"-c", "user.name=Sworn Bootstrap Test",
		"-c", "user.email=sworn-bootstrap@example.invalid",
		"commit", "--quiet", "-m", "records",
	)
	withRecord := archiveProductEntries(t, repository, "HEAD")

	if len(product) != len(withRecord) {
		t.Fatalf("record-only commit changed archive entry count from %d to %d", len(product), len(withRecord))
	}
	for name, want := range product {
		if got, ok := withRecord[name]; !ok || got != want {
			t.Fatalf("record-only commit changed archive entry %q", name)
		}
	}
}

type archiveEntry struct {
	mode     int64
	typeflag byte
	linkname string
	content  string
}

func archiveProductEntries(t *testing.T, repository string, treeish string) map[string]archiveEntry {
	t.Helper()
	command := exec.Command("git", "-C", repository, "archive", "--format=tar", treeish)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]archiveEntry)
	reader := tar.NewReader(bytes.NewReader(output))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(header.Name, ".baton/releases/") {
			t.Fatalf("Git archive exposed Baton record path %q", header.Name)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = archiveEntry{
			mode: header.Mode, typeflag: header.Typeflag,
			linkname: header.Linkname, content: string(content),
		}
	}
	return entries
}

func copyCandidateModuleToPlainTree(t *testing.T, sourceRoot string) string {
	t.Helper()

	candidateOutput, err := exec.Command(
		"git", "-C", sourceRoot, "ls-files", "-z",
		"--cached", "--others", "--exclude-standard",
		"--", ".", ":(exclude,top).baton/releases",
		":(exclude,top).baton/releases/**",
	).Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(candidateOutput) == 0 {
		t.Fatal("candidate module copy has no files")
	}

	plainRoot := t.TempDir()
	for _, raw := range bytes.Split(candidateOutput, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		relative := filepath.FromSlash(string(raw))
		sourcePath := filepath.Join(sourceRoot, relative)
		targetPath := filepath.Join(plainRoot, relative)
		info, err := os.Lstat(sourcePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("candidate source file %q: %v", sourcePath, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatal(err)
		}
		switch {
		case info.Mode().IsRegular():
			data, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("read candidate file %q: %v", sourcePath, err)
			}
			if err := os.WriteFile(targetPath, data, info.Mode().Perm()); err != nil {
				t.Fatalf("write candidate file %q: %v", targetPath, err)
			}
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(sourcePath)
			if err != nil {
				t.Fatalf("read candidate symlink %q: %v", sourcePath, err)
			}
			if err := os.Symlink(link, targetPath); err != nil {
				t.Fatalf("write candidate symlink %q: %v", targetPath, err)
			}
		default:
			t.Fatalf("unsupported candidate mode %s for %q", info.Mode(), relative)
		}
	}

	return plainRoot
}

func runGitForCandidateCopy(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func buildOfficialSwornBinary(t *testing.T, moduleRoot, output, source string) {
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
	command.Env = append(
		os.Environ(),
		"GOFLAGS=-buildvcs=false",
		"CGO_ENABLED=0",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOCACHE="+t.TempDir(),
	)
	outputBytes, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build sworn official: %v: %s", err, outputBytes)
	}
}

func buildSwornForProcessTest(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "sworn")
	buildOfficialSwornBinary(t, ".", binary, ".")
	return binary
}

func assertProcessExit(t *testing.T, err error, want int) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != want {
		t.Fatalf("process error = %v, want exit status %d", err, want)
	}
}
