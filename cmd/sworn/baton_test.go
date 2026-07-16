package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
)

// testBatonTag is a test-only Baton version tag.
var testBatonTag = "v9.8.7"

const vendorPayloadCanary = "SWORN_VENDOR_SECRET_PAYLOAD_CANARY_7f31d64c"

func TestBatonDiffExitsNonZeroOnDivergence(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "..", "internal", "baton", "testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	// Create a .git directory so RepoRoot discovery succeeds.
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create the embed directory structure.
	for _, m := range baton.AllMappings() {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Vendor to populate the embed.
	_, err = baton.Vendor(baton.VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	// Sanity: diff is clean before mutation.
	t.Run("clean_before_mutation", func(t *testing.T) {
		oldDir, _ := os.Getwd()
		if err := os.Chdir(tmpRepo); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(oldDir)

		exit := cmdBatonDiff([]string{fixture})
		if exit != 0 {
			t.Errorf("cmdBatonDiff clean exit = %d, want 0", exit)
		}
	})

	// Hand-edit an embed file.
	ruleFile := filepath.Join(tmpRepo, "internal/adopt/baton/rules/01-reachability-gate.md")
	orig, err := os.ReadFile(ruleFile)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(orig), "sworn verify", "sworn verify (FORKED)", 1)
	if err := os.WriteFile(ruleFile, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	// Diff should exit non-zero and name the file.
	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	exit := cmdBatonDiff([]string{fixture})
	if exit == 0 {
		t.Fatal("cmdBatonDiff after hand-edit exit = 0, want non-zero")
	}

	// Capture output: the command prints to stdout.
	code, out := captureStdout(t, func() int {
		return cmdBatonDiff([]string{fixture})
	})
	if code == 0 {
		t.Fatal("cmdBatonDiff after hand-edit exit = 0, want non-zero")
	}
	if !strings.Contains(out, "internal/adopt/baton/rules/01-reachability-gate.md") {
		t.Errorf("output missing divergent file path, got:\n%s", out)
	}
	if !strings.Contains(out, "content differs") {
		t.Errorf("output missing reason, got:\n%s", out)
	}
}

func TestBatonDiffExitsZeroWhenInSync(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "..", "internal", "baton", "testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, m := range baton.AllMappings() {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	_, err = baton.Vendor(baton.VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	exit := cmdBatonDiff([]string{fixture})
	if exit != 0 {
		t.Errorf("cmdBatonDiff clean exit = %d, want 0", exit)
	}
}

// TestBatonDiffV015BinaryReachability is S02's Rule-1 red. It builds and
// drives the registered public command against the exact repository-owned
// offline input instead of importing a leaf parity helper directly.
func TestBatonDiffV015BinaryReachability(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(repoRoot, "internal", "adopt", "baton", "installer-input-v0.15.1.tar")
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read exact offline installer input: %v", err)
	}

	sourceRoot := extractBatonArchiveForReachability(t, archiveBytes)
	bin := buildSworn(t)
	cmd := exec.Command(bin, "baton", "diff", sourceRoot)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sworn baton diff exact v0.15.1 source: %v\n%s", err, output)
	}
}

func extractBatonArchiveForReachability(t *testing.T, archiveBytes []byte) string {
	t.Helper()
	destination := t.TempDir()
	reader := tar.NewReader(bytes.NewReader(archiveBytes))
	const prefix = "baton-v0.15.1/"
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read installer archive: %v", err)
		}
		if !strings.HasPrefix(header.Name, prefix) {
			t.Fatalf("installer archive entry %q lacks %q prefix", header.Name, prefix)
		}
		relative := strings.TrimPrefix(header.Name, prefix)
		if relative == "" {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatalf("create archive directory: %v", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create archive parent: %v", err)
			}
			contents, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("read archive file: %v", err)
			}
			if err := os.WriteFile(target, contents, 0o644); err != nil {
				t.Fatalf("write archive file: %v", err)
			}
		default:
			t.Fatalf("unexpected archive type %d for %q", header.Typeflag, header.Name)
		}
	}
	return destination
}

// -- upstream vendor integration tests (Rule 1: through the command) ---------

func TestBatonVendorAtomicPreflightReachability(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "..", "internal", "baton", "testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	canaryFixture := t.TempDir()
	if err := os.CopyFS(canaryFixture, os.DirFS(fixture)); err != nil {
		t.Fatalf("copy vendor fixture: %v", err)
	}
	canarySource := filepath.Join(canaryFixture, filepath.FromSlash(baton.AllMappings()[0].Source))
	canaryContent, err := os.ReadFile(canarySource)
	if err != nil {
		t.Fatalf("read canary source: %v", err)
	}
	canaryContent = append(canaryContent, []byte("\n"+vendorPayloadCanary+"\n")...)
	if err := os.WriteFile(canarySource, canaryContent, 0o644); err != nil {
		t.Fatalf("write canary source: %v", err)
	}
	fixture = canaryFixture

	newRepo := func(t *testing.T) string {
		t.Helper()
		repo := t.TempDir()
		if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		return repo
	}

	t.Run("valid drift is exit 1 and check is mutation-free", func(t *testing.T) {
		repo := newRepo(t)
		canaryDest := filepath.Join(repo, filepath.FromSlash(baton.AllMappings()[0].Dest))
		if err := os.MkdirAll(filepath.Dir(canaryDest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(canaryDest, []byte(vendorPayloadCanary), 0o640); err != nil {
			t.Fatal(err)
		}
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", "--check", fixture})
		if exit != 1 {
			t.Fatalf("sworn baton vendor --check drift exit = %d, want 1; output=%s", exit, output)
		}
		got, err := os.ReadFile(canaryDest)
		if err != nil {
			t.Fatalf("read pre-existing destination after check: %v", err)
		}
		if string(got) != vendorPayloadCanary {
			t.Fatalf("check mode changed pre-existing payload: got %q", got)
		}
		info, err := os.Stat(canaryDest)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Fatalf("check mode changed pre-existing mode to %04o", info.Mode().Perm())
		}
		if _, err := os.Stat(filepath.Join(repo, ".git", "sworn")); !os.IsNotExist(err) {
			t.Fatalf("check mode created Git-admin recovery state: %v", err)
		}
		assertBatonPathOnlyOutput(t, output)
	})

	t.Run("positional source followed by check is honored and mutation-free", func(t *testing.T) {
		repo := newRepo(t)
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", fixture, "--check"})
		if exit != 1 {
			t.Fatalf("sworn baton vendor SOURCE --check exit = %d, want 1; output=%s", exit, output)
		}
		if _, err := os.Stat(filepath.Join(repo, "internal")); !os.IsNotExist(err) {
			t.Fatalf("SOURCE --check mutated repository: stat internal error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(repo, ".git", "sworn")); !os.IsNotExist(err) {
			t.Fatalf("SOURCE --check created Git-admin recovery state: %v", err)
		}
		assertBatonPathOnlyOutput(t, output)
	})

	t.Run("successful write and byte-identical check are exit 0", func(t *testing.T) {
		repo := newRepo(t)
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", fixture})
		if exit != 0 {
			t.Fatalf("sworn baton vendor write exit = %d, want 0; output=%s", exit, output)
		}
		assertBatonPathOnlyOutput(t, output)
		exit, output = captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", "--check", fixture})
		if exit != 0 {
			t.Fatalf("sworn baton vendor clean check exit = %d, want 0; output=%s", exit, output)
		}
		if !strings.Contains(output, "No changes") {
			t.Fatalf("clean check output = %q, want no-changes guidance", output)
		}
		modeProbe := filepath.Join(repo, filepath.FromSlash(baton.AllMappings()[0].Dest))
		if err := os.Chmod(modeProbe, 0o600); err != nil {
			t.Fatal(err)
		}
		exit, output = captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", "--check", fixture})
		if exit != 0 || !strings.Contains(output, "No changes") {
			t.Fatalf("byte-identical mode-only check = exit %d output %q, want clean exit 0", exit, output)
		}
		if info, err := os.Stat(modeProbe); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("byte-identical check changed existing mode: info=%v err=%v", info, err)
		}
	})

	t.Run("invalid source is exit 2 without mutation", func(t *testing.T) {
		repo := newRepo(t)
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", "--check", t.TempDir()})
		if exit != 2 {
			t.Fatalf("invalid source exit = %d, want 2; output=%s", exit, output)
		}
		if _, err := os.Stat(filepath.Join(repo, "internal")); !os.IsNotExist(err) {
			t.Fatalf("invalid preflight mutated repository: %v", err)
		}
		assertBatonPathOnlyOutput(t, output)
	})

	t.Run("invalid schema error is path-only and payload-redacted", func(t *testing.T) {
		repo := newRepo(t)
		invalidFixture := t.TempDir()
		if err := os.CopyFS(invalidFixture, os.DirFS(fixture)); err != nil {
			t.Fatal(err)
		}
		invalidSchema := []byte(`{"type":"string","pattern":"` + vendorPayloadCanary + `["}`)
		if err := os.WriteFile(filepath.Join(invalidFixture, "schemas", "board-v1.json"), invalidSchema, 0o644); err != nil {
			t.Fatal(err)
		}
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", "--check", invalidFixture})
		if exit != 2 {
			t.Fatalf("invalid-schema exit = %d, want 2; output=%s", exit, output)
		}
		if !strings.Contains(output, "class=schema-invalid") || !strings.Contains(output, "destination=internal/baton/schemas/board-v1.json") {
			t.Fatalf("invalid-schema output omitted deterministic class/path: %s", output)
		}
		assertBatonPathOnlyOutput(t, output)
		if _, err := os.Stat(filepath.Join(repo, ".git", "sworn")); !os.IsNotExist(err) {
			t.Fatalf("invalid schema created Git-admin state: %v", err)
		}
	})

	t.Run("non-regular destination is operational exit 2 not drift", func(t *testing.T) {
		repo := newRepo(t)
		first := baton.AllMappings()[0]
		if err := os.MkdirAll(filepath.Join(repo, filepath.FromSlash(first.Dest)), 0o755); err != nil {
			t.Fatal(err)
		}
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", "--check", fixture})
		if exit != 2 {
			t.Fatalf("non-regular destination exit = %d, want 2; output=%s", exit, output)
		}
		if info, err := os.Stat(filepath.Join(repo, filepath.FromSlash(first.Dest))); err != nil || !info.IsDir() {
			t.Fatalf("operational failure changed destination directory: info=%v err=%v", info, err)
		}
		assertBatonPathOnlyOutput(t, output)
	})

	t.Run("apply failure rolls back and is public exit 2", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("requires POSIX directory write permissions")
		}
		repo := newRepo(t)
		blockedParent := filepath.Join(repo, "internal", "baton", "schemas")
		if err := os.MkdirAll(blockedParent, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(blockedParent, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(blockedParent, 0o755) })
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", fixture})
		if exit != 2 {
			t.Fatalf("apply failure exit = %d, want 2; output=%s", exit, output)
		}
		if !strings.Contains(output, "class=apply-failed") || !strings.Contains(output, "phase=apply") {
			t.Fatalf("apply failure output omitted deterministic classification: %s", output)
		}
		firstDest := filepath.Join(repo, filepath.FromSlash(baton.AllMappings()[0].Dest))
		if _, err := os.Stat(firstDest); !os.IsNotExist(err) {
			t.Fatalf("successful rollback retained an earlier apply destination: %v", err)
		}
		if _, err := os.Stat(filepath.Join(repo, ".git", "sworn")); !os.IsNotExist(err) {
			t.Fatalf("successful rollback retained recovery authority: %v", err)
		}
		assertBatonPathOnlyOutput(t, output)
	})

	t.Run("incomplete rollback publishes public recovery-only authority", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("requires POSIX directory write permissions")
		}
		repo := newRepo(t)
		blockedApplyParent := filepath.Join(repo, "internal", "prompt")
		if err := os.MkdirAll(blockedApplyParent, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(blockedApplyParent, 0o555); err != nil {
			t.Fatal(err)
		}
		rollbackParent := filepath.Join(repo, "internal", "adopt", "baton")
		watchPath := filepath.Join(repo, "internal", "baton", "schemas", "assembly-proof-v1.json")
		watchDone := make(chan error, 1)
		stopWatch := make(chan struct{})
		defer close(stopWatch)
		go func() {
			for {
				select {
				case <-stopWatch:
					return
				default:
				}
				if _, err := os.Stat(watchPath); err == nil {
					watchDone <- os.Chmod(rollbackParent, 0o555)
					return
				} else if !os.IsNotExist(err) {
					watchDone <- err
					return
				}
				runtime.Gosched()
			}
		}()
		t.Cleanup(func() {
			_ = os.Chmod(blockedApplyParent, 0o755)
			_ = os.Chmod(rollbackParent, 0o755)
		})

		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", fixture})
		select {
		case err := <-watchDone:
			if err != nil {
				t.Fatalf("arm rollback fault: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out arming rollback fault")
		}
		if exit != 2 || !strings.Contains(output, "class=rollback-incomplete") {
			t.Fatalf("incomplete rollback = exit %d output %s, want exit 2 rollback-incomplete", exit, output)
		}
		sentinel := filepath.Join(repo, ".git", "sworn", "recovery", "baton-vendor", "rollback-incomplete.json")
		if _, err := os.Stat(sentinel); err != nil {
			t.Fatalf("incomplete rollback omitted fixed recovery authority: %v", err)
		}
		assertBatonPathOnlyOutput(t, output)

		if err := os.Chmod(blockedApplyParent, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(rollbackParent, 0o755); err != nil {
			t.Fatal(err)
		}
		exit, output = captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", fixture})
		if exit != 2 || !strings.Contains(output, "class=recovered-rerun-required") {
			t.Fatalf("valid recovery-only invocation = exit %d output %s, want exit 2 rerun", exit, output)
		}
		firstDest := filepath.Join(repo, filepath.FromSlash(baton.AllMappings()[0].Dest))
		if _, err := os.Stat(firstDest); !os.IsNotExist(err) {
			t.Fatalf("recovery-only invocation combined restoration with ordinary vendor write: %v", err)
		}
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Fatalf("valid recovery retained fixed authority: %v", err)
		}
		assertBatonPathOnlyOutput(t, output)

		exit, output = captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", fixture})
		if exit != 0 {
			t.Fatalf("post-recovery rerun exit = %d, want 0; output=%s", exit, output)
		}
	})

	t.Run("invalid invocations are exit 2 without mutation", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
		}{
			{name: "missing source", args: []string{"sworn", "baton", "vendor"}},
			{name: "extra local operand", args: []string{"sworn", "baton", "vendor", fixture, fixture}},
			{name: "upstream source operand", args: []string{"sworn", "baton", "vendor", "--upstream", fixture}},
			{name: "unknown flag", args: []string{"sworn", "baton", "vendor", "--unknown"}},
			{name: "missing tag value", args: []string{"sworn", "baton", "vendor", "--upstream", "--tag"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				repo := newRepo(t)
				exit, output := captureBatonDispatch(t, repo, tt.args)
				if exit != 2 {
					t.Fatalf("invalid vendor invocation exit = %d, want 2; output=%s", exit, output)
				}
				if _, err := os.Stat(filepath.Join(repo, ".git", "sworn")); !os.IsNotExist(err) {
					t.Fatalf("invalid invocation created Git-admin recovery state: %v", err)
				}
			})
		}
	})

	t.Run("help is non-success exit 2", func(t *testing.T) {
		repo := newRepo(t)
		exit, output := captureBatonDispatch(t, repo, []string{"sworn", "baton", "vendor", "--help"})
		if exit != 2 {
			t.Fatalf("vendor --help exit = %d, want 2; output=%s", exit, output)
		}
		if !strings.Contains(output, "usage: sworn baton vendor") {
			t.Fatalf("vendor --help output missing usage: %s", output)
		}
	})
}

func captureBatonDispatch(t *testing.T, repo string, args []string) (int, string) {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	}()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer
	exit := dispatch(args)
	_ = writer.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return exit, string(output)
}

func assertBatonPathOnlyOutput(t *testing.T, output string) {
	t.Helper()
	for _, payloadMarker := range []string{"--- a/", "+++ b/", "# Rule:", `"properties":`, "sworn verify", vendorPayloadCanary} {
		if strings.Contains(output, payloadMarker) {
			t.Fatalf("vendor output exposed mapped payload marker %q: %s", payloadMarker, output)
		}
	}
}

func TestBatonVendorUpstreamRecoveryPrecedesNetwork(t *testing.T) {
	repo := t.TempDir()
	recoveryRoot := filepath.Join(repo, ".git", "sworn", "recovery", "baton-vendor")
	if err := os.MkdirAll(recoveryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(recoveryRoot, "rollback-incomplete.json")
	if err := os.WriteFile(sentinel, []byte("{not-valid-recovery-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "network must not be reached", http.StatusInternalServerError)
	}))
	defer server.Close()
	baton.SetBaseURLForTest(server.URL)
	t.Cleanup(baton.ClearBaseURLForTest)

	exit, output := captureBatonDispatch(t, repo, []string{
		"sworn", "baton", "vendor", "--upstream",
		"--repo", "example/baton", "--tag", testBatonTag,
	})
	if exit != 2 {
		t.Fatalf("upstream pending-recovery exit = %d, want 2; output=%s", exit, output)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("upstream fetch started before recovery was handled: requests=%d", got)
	}
	if !strings.Contains(output, "recovery") {
		t.Fatalf("upstream recovery failure output missing recovery classification: %s", output)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("invalid recovery authority was not preserved: %v", err)
	}
}

// makeUpstreamTarball creates a gzipped tar archive in memory with the
// GitHub-style top-level prefix <repo>-<tag>/. Each entry in files is a
// relative path within the archive.
func makeUpstreamTarball(repoName, tag string, files map[string]string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// GitHub strips the leading v from semver tags in tarball paths.
	cleanTag := strings.TrimPrefix(tag, "v")
	prefix := fmt.Sprintf("%s-%s", repoName, cleanTag)

	// Add the top-level directory entry first.
	_ = tw.WriteHeader(&tar.Header{
		Name:     prefix + "/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})

	// Track written dirs to avoid duplicates.
	writtenDirs := map[string]bool{}

	for name, content := range files {
		path := prefix + "/" + name
		// Ensure parent dirs exist in the tar.
		parent := filepath.Dir(path)
		if parent != "." && parent != prefix && !writtenDirs[parent] {
			_ = tw.WriteHeader(&tar.Header{
				Name:     parent + "/",
				Typeflag: tar.TypeDir,
				Mode:     0755,
			})
			writtenDirs[parent] = true
		}

		data := []byte(content)
		_ = tw.WriteHeader(&tar.Header{
			Name:     path,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
			Mode:     0644,
		})
		tw.Write(data)
	}

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

// sha256HexDigest returns the sha256:<hex> digest of data.
func sha256HexDigest(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// upstreamTestServer creates an httptest.Server that serves both the GitHub API
// (api.github.com) and codeload (codeload.github.com) endpoints.
func upstreamTestServer(owner, name, tag, commitSHA string, tarball []byte) *httptest.Server {
	mux := http.NewServeMux()

	// API: /repos/{owner}/{name}/commits/{tag}
	apiPath := fmt.Sprintf("/repos/%s/%s/commits/%s", owner, name, tag)
	mux.HandleFunc(apiPath, func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			SHA string `json:"sha"`
		}{SHA: commitSHA}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Codeload: /{owner}/{name}/tar.gz/refs/tags/{tag}
	codeloadPath := fmt.Sprintf("/%s/%s/tar.gz/refs/tags/%s", owner, name, tag)
	mux.HandleFunc(codeloadPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		w.Write(tarball)
	})

	return httptest.NewServer(mux)
}

// vendorFixtureFiles returns a minimal set of fixture source files that
// satisfies the baton file mappings (all non-sentinel entries).
func vendorFixtureFiles() map[string]string {
	files := make(map[string]string)
	for _, m := range baton.AllMappings() {
		if m.Source == "baton/rules.md" {
			continue // sentinel — concatenated by Vendor, not a source file
		}
		// Deduplicate: same source mapped to multiple destinations.
		if _, ok := files[m.Source]; ok {
			continue
		}
		if strings.HasSuffix(m.Source, ".json") {
			// Candidate schemas now compile during the shared preflight. An
			// empty JSON Schema is valid and keeps these network tests focused
			// on fetch/transaction reachability rather than schema semantics.
			files[m.Source] = "{}\n"
		} else {
			files[m.Source] = fmt.Sprintf("# %s\n\nMinimal fixture content for integration test.\n", filepath.Base(m.Source))
		}
	}
	return files
}

func TestBatonVendorUpstream_Success(t *testing.T) {
	owner, repo, tag := "sawy3r", "baton", testBatonTag
	commitSHA := "abc123def4567890123456789012345678abcdef"

	files := vendorFixtureFiles()
	tarball := makeUpstreamTarball(repo, tag, files)
	digest := sha256HexDigest(tarball)

	// Set up test pin.
	baton.SetUpstreamPinForTest(&baton.UpstreamPin{SHA: commitSHA, Digest: digest})
	t.Cleanup(baton.ClearUpstreamPinForTest)

	// Create httptest server.
	ts := upstreamTestServer(owner, repo, tag, commitSHA, tarball)
	defer ts.Close()
	baton.SetBaseURLForTest(ts.URL)
	t.Cleanup(baton.ClearBaseURLForTest)

	// Create temp repo with .git for findRepoRoot.
	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, m := range baton.AllMappings() {
		if err := os.MkdirAll(filepath.Join(tmpRepo, filepath.Dir(m.Dest)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Create the existing VERSION transaction member.
	versionDir := filepath.Join(tmpRepo, "internal", "adopt", "baton")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "VERSION"), []byte("baton-protocol: v0.4.2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Chdir to temp repo so findRepoRoot discovers it.
	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	// Run the command through the CLI entry point (Rule 1).
	exit := cmdBatonVendor([]string{"--upstream", "--repo", owner + "/" + repo, "--tag", tag})
	if exit != 0 {
		t.Fatalf("cmdBatonVendor --upstream exit = %d, want 0", exit)
	}

	// Verify: embed files were written.
	for _, m := range baton.AllMappings() {
		destPath := filepath.Join(tmpRepo, m.Dest)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("dest file not written: %s", m.Dest)
		}
	}

	// Verify: VERSION was updated with pin.
	versionContent, err := os.ReadFile(filepath.Join(versionDir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(versionContent), "upstream-sha: "+commitSHA) {
		t.Error("VERSION missing upstream-sha")
	}
	if !strings.Contains(string(versionContent), "upstream-digest: sha256:") {
		t.Error("VERSION missing upstream-digest")
	}

	// Verify: spot-check a rule file has been transformed.
	ruleFile := filepath.Join(tmpRepo, "internal/adopt/baton/rules/01-reachability-gate.md")
	content, err := os.ReadFile(ruleFile)
	if err != nil {
		t.Fatalf("cannot read %s: %v", ruleFile, err)
	}
	if len(content) == 0 {
		t.Error("rule file is empty after vendor")
	}
}

func TestBatonVendorUpstreamCheckIncludesVersionWithoutMutation(t *testing.T) {
	owner, repo, tag := "sawy3r", "baton", testBatonTag
	commitSHA := "abc123def4567890123456789012345678abcdef"
	files := vendorFixtureFiles()
	tarball := makeUpstreamTarball(repo, tag, files)
	digest := sha256HexDigest(tarball)
	baton.SetUpstreamPinForTest(&baton.UpstreamPin{SHA: commitSHA, Digest: digest})
	t.Cleanup(baton.ClearUpstreamPinForTest)
	ts := upstreamTestServer(owner, repo, tag, commitSHA, tarball)
	defer ts.Close()
	baton.SetBaseURLForTest(ts.URL)
	t.Cleanup(baton.ClearBaseURLForTest)

	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	versionPath := filepath.Join(tmpRepo, "internal", "adopt", "baton", "VERSION")
	if err := os.MkdirAll(filepath.Dir(versionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	originalVersion := []byte("baton-protocol: v0.4.2\nvendored: 2026-07-01\nupstream-sha: old\nupstream-digest: sha256:old\n")
	if err := os.WriteFile(versionPath, originalVersion, 0o640); err != nil {
		t.Fatal(err)
	}

	exit, output := captureBatonDispatch(t, tmpRepo, []string{
		"sworn", "baton", "vendor", "--upstream", "--check",
		"--repo", owner + "/" + repo, "--tag", tag,
	})
	if exit != 1 {
		t.Fatalf("upstream check drift exit = %d, want 1; output=%s", exit, output)
	}
	if !strings.Contains(output, "changed: internal/adopt/baton/VERSION") {
		t.Fatalf("upstream check omitted VERSION drift: %s", output)
	}
	got, err := os.ReadFile(versionPath)
	if err != nil || !bytes.Equal(got, originalVersion) {
		t.Fatalf("upstream check mutated VERSION: read=%v got=%q", err, got)
	}
	if info, err := os.Stat(versionPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("upstream check mutated VERSION mode: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(tmpRepo, ".git", "sworn")); !os.IsNotExist(err) {
		t.Fatalf("upstream check created Git-admin state: %v", err)
	}
	firstDest := filepath.Join(tmpRepo, filepath.FromSlash(baton.AllMappings()[0].Dest))
	if _, err := os.Stat(firstDest); !os.IsNotExist(err) {
		t.Fatalf("upstream check created mapped destination: %v", err)
	}
	assertBatonPathOnlyOutput(t, output)
}

func TestBatonVendorUpstream_RefreshesProjectContextSchema(t *testing.T) {
	owner, repo, tag := "sawy3r", "baton", testBatonTag
	commitSHA := "abc123def4567890123456789012345678abcdef"
	// Deliberately omit a trailing newline and include a transform token: a
	// normative schema must survive the public vendor command byte-for-byte.
	expected := "{\n  \"$id\": \"https://baton.sawy3r.net/schemas/project-context-v1.json\",\n  \"description\": \"The literal install.sh token proves schemas are copied byte-for-byte.\",\n  \"examples\": [{\"ratification\": {\"by\": \"sam\"}}]\n}"

	files := vendorFixtureFiles()
	files["schemas/project-context-v1.json"] = expected
	tarball := makeUpstreamTarball(repo, tag, files)
	digest := sha256HexDigest(tarball)

	baton.SetUpstreamPinForTest(&baton.UpstreamPin{SHA: commitSHA, Digest: digest})
	t.Cleanup(baton.ClearUpstreamPinForTest)

	ts := upstreamTestServer(owner, repo, tag, commitSHA, tarball)
	defer ts.Close()
	baton.SetBaseURLForTest(ts.URL)
	t.Cleanup(baton.ClearBaseURLForTest)

	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	versionDir := filepath.Join(tmpRepo, "internal", "adopt", "baton")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "VERSION"), []byte("baton-protocol: v0.13.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	if exit := dispatch([]string{"sworn", "baton", "vendor", "--upstream", "--repo", owner + "/" + repo, "--tag", tag}); exit != 0 {
		t.Fatalf("dispatch sworn baton vendor --upstream exit = %d, want 0", exit)
	}

	dest := filepath.Join(tmpRepo, "internal", "baton", "schemas", "project-context-v1.json")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read vendored project context schema: %v", err)
	}
	if string(got) != expected {
		t.Fatalf("vendored project context schema differs from tagged source\ngot:\n%s\nwant:\n%s", got, expected)
	}
}

func TestBatonVendorUpstream_DigestMismatch(t *testing.T) {
	owner, repo, tag := "sawy3r", "baton", testBatonTag
	commitSHA := "abc123def4567890123456789012345678abcdef"

	files := vendorFixtureFiles()
	tarball := makeUpstreamTarball(repo, tag, files)

	// Pin a different digest for THIS tag — simulates a tampered tarball.
	// Tag matters: the SHA/digest guard only applies to the tag the pin describes,
	// because comparing a pin against a different tag's content is meaningless
	// (an intentional version bump changes both by definition).
	baton.SetUpstreamPinForTest(&baton.UpstreamPin{
		Tag:    tag,
		SHA:    commitSHA,
		Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	})
	t.Cleanup(baton.ClearUpstreamPinForTest)

	ts := upstreamTestServer(owner, repo, tag, commitSHA, tarball)
	defer ts.Close()
	baton.SetBaseURLForTest(ts.URL)
	t.Cleanup(baton.ClearBaseURLForTest)

	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, m := range baton.AllMappings() {
		os.MkdirAll(filepath.Join(tmpRepo, filepath.Dir(m.Dest)), 0755)
	}
	versionDir := filepath.Join(tmpRepo, "internal", "adopt", "baton")
	os.MkdirAll(versionDir, 0755)
	os.WriteFile(filepath.Join(versionDir, "VERSION"), []byte("baton-protocol: v0.4.2\n"), 0644)

	oldDir, _ := os.Getwd()
	os.Chdir(tmpRepo)
	defer os.Chdir(oldDir)

	// Should fail closed — non-zero exit, no files written.
	exit := cmdBatonVendor([]string{"--upstream", "--repo", owner + "/" + repo, "--tag", tag})
	if exit == 0 {
		t.Fatal("cmdBatonVendor --upstream with digest mismatch exit = 0, want non-zero")
	}

	// Verify: no embed files were written (the first dest file should not exist).
	firstMapping := baton.AllMappings()[0]
	if firstMapping.Dest != "" {
		destPath := filepath.Join(tmpRepo, firstMapping.Dest)
		if _, err := os.Stat(destPath); err == nil {
			t.Errorf("dest file %s was written despite fetch failure", firstMapping.Dest)
		}
	}
}

func TestBatonVendorUpstream_NoTagUsesPinned(t *testing.T) {
	owner, repo, tag := "sawy3r", "baton", testBatonTag
	commitSHA := "abc123def4567890123456789012345678abcdef"

	// Set the pinned version so Version() returns this tag when --tag is empty.
	baton.SetVersionForTest(tag)
	t.Cleanup(func() { baton.SetVersionForTest("") })

	files := vendorFixtureFiles()
	tarball := makeUpstreamTarball(repo, tag, files)
	digest := sha256HexDigest(tarball)

	baton.SetUpstreamPinForTest(&baton.UpstreamPin{SHA: commitSHA, Digest: digest})
	t.Cleanup(baton.ClearUpstreamPinForTest)

	// Custom test server that captures the codeload URL path for assertion.
	var codeloadURLPath string
	mux := http.NewServeMux()

	// API: /repos/{owner}/{repo}/commits/{tag}
	apiPath := fmt.Sprintf("/repos/%s/%s/commits/%s", owner, repo, tag)
	mux.HandleFunc(apiPath, func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			SHA string `json:"sha"`
		}{SHA: commitSHA}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Codeload: capture path for assertion; serve tarball.
	codeloadPrefix := fmt.Sprintf("/%s/%s/tar.gz/refs/tags/", owner, repo)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, codeloadPrefix) {
			codeloadURLPath = r.URL.Path
			w.Header().Set("Content-Type", "application/x-gzip")
			w.Write(tarball)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	baton.SetBaseURLForTest(ts.URL)
	t.Cleanup(baton.ClearBaseURLForTest)

	// Set up temp repo.
	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, m := range baton.AllMappings() {
		os.MkdirAll(filepath.Join(tmpRepo, filepath.Dir(m.Dest)), 0755)
	}
	versionDir := filepath.Join(tmpRepo, "internal", "adopt", "baton")
	os.MkdirAll(versionDir, 0755)
	os.WriteFile(filepath.Join(versionDir, "VERSION"), []byte("baton-protocol: v0.4.2\n"), 0644)

	oldDir, _ := os.Getwd()
	os.Chdir(tmpRepo)
	defer os.Chdir(oldDir)

	// Run with --upstream but NO --tag. Should fall back to baton.Version().
	exit := cmdBatonVendor([]string{"--upstream", "--repo", owner + "/" + repo})
	if exit != 0 {
		t.Fatalf("cmdBatonVendor --upstream (no --tag) exit = %d, want 0", exit)
	}

	// Assert: codeload URL contains the pinned semver tag, not "latest" or "HEAD".
	if !strings.Contains(codeloadURLPath, tag) {
		t.Errorf("codeload URL path %q does not contain pinned tag %q", codeloadURLPath, tag)
	}
	if strings.Contains(strings.ToLower(codeloadURLPath), "latest") {
		t.Errorf("codeload URL path %q contains 'latest' — should use pinned tag %q", codeloadURLPath, tag)
	}
	if strings.Contains(strings.ToLower(codeloadURLPath), "head") {
		t.Errorf("codeload URL path %q contains 'head' — should use pinned tag %q", codeloadURLPath, tag)
	}

	// Verify files were written (same as success test).
	for _, m := range baton.AllMappings() {
		destPath := filepath.Join(tmpRepo, m.Dest)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("dest file not written: %s", m.Dest)
		}
	}
}

func TestBatonVendorUpstream_LocalBackCompat(t *testing.T) { // Without --upstream, the command should use the local-dir path (S48 back-compat).
	fixture, err := filepath.Abs(filepath.Join("..", "..", "internal", "baton", "testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, m := range baton.AllMappings() {
		if err := os.MkdirAll(filepath.Join(tmpRepo, filepath.Dir(m.Dest)), 0755); err != nil {
			t.Fatal(err)
		}
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	// Local vendor (no --upstream flag).
	exit := cmdBatonVendor([]string{fixture})
	if exit != 0 {
		t.Fatalf("cmdBatonVendor (local) exit = %d, want 0", exit)
	}

	// Verify files were written.
	for _, m := range baton.AllMappings() {
		destPath := filepath.Join(tmpRepo, m.Dest)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("dest file not written: %s", m.Dest)
		}
	}
}
