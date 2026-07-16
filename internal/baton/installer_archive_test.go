package baton

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/adopt"
)

func TestPinnedInstallerArchiveGlobalMetadata(t *testing.T) {
	archiveBytes := adopt.BatonInstallerArchive()
	if _, err := ValidateInstallerArchive(archiveBytes); err != nil {
		t.Fatalf("ValidateInstallerArchive() error = %v", err)
	}
	r := tar.NewReader(bytes.NewReader(archiveBytes))
	h, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if h.Typeflag != tar.TypeXGlobalHeader || h.Name != "pax_global_header" {
		t.Fatalf("first header = (%q, %d), want exact global PAX header", h.Name, h.Typeflag)
	}
	if len(h.PAXRecords) != 1 || h.PAXRecords["comment"] != PinnedBatonCommit {
		t.Fatalf("global PAX metadata = %#v, want only pinned commit comment", h.PAXRecords)
	}
}

// TestBatonV015GitBundleFixtureIdentity authenticates the committed clean-CI
// source fixture before any test treats a temporary checkout as Baton v0.15.1.
// The fixture is deliberately test-only: production installer authority remains
// the embedded installer-input tar.
func TestBatonV015GitBundleFixtureIdentity(t *testing.T) {
	const (
		bundleSize   = 2505826
		bundleSHA256 = "cba3796ed382623f35abc568183e3a5a0d4a82335cebd4589989d0ae41b43ad5"
		bundleBlob   = "77e5b4cc7210a41ce8779bc352a1f487101fb80e"
		tagObject    = "3ba5f70435ff1ef3ea819def7b06c126fdb269d8"
		commit       = "3fb4d275ae8a151f6287e7b9279d71628b12eea0"
		versionBlob  = "5f1dd0af59642311ee04e018a0023562d4dde008"
	)
	bundlePath := filepath.Join(installerArchiveTestRepoRoot(t), "internal", "baton", "testdata", "fixture", "baton-v0.15.1.bundle")
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle) != bundleSize {
		t.Fatalf("bundle size = %d, want %d", len(bundle), bundleSize)
	}
	digest := sha256.Sum256(bundle)
	if got := fmt.Sprintf("%x", digest); got != bundleSHA256 {
		t.Fatalf("bundle SHA-256 = %s, want %s", got, bundleSHA256)
	}
	if got := gitBlobOID(bundle); got != bundleBlob {
		t.Fatalf("bundle Git blob = %s, want %s", got, bundleBlob)
	}
	const header = "# v2 git bundle\n3ba5f70435ff1ef3ea819def7b06c126fdb269d8 refs/tags/v0.15.1\n\n"
	if len(bundle) < len(header) || string(bundle[:len(header)]) != header {
		t.Fatal("bundle header differs from the exact v0.15.1 authority")
	}
	verify := exec.Command("git", "bundle", "verify", bundlePath)
	verifyOutput, err := verify.CombinedOutput()
	if err != nil || !strings.Contains(string(verifyOutput), "complete history") {
		t.Fatalf("verify bundle: %v\n%s", err, verifyOutput)
	}

	clone := filepath.Join(t.TempDir(), "baton-v0.15.1")
	if output, err := exec.Command("git", "clone", "--no-checkout", bundlePath, clone).CombinedOutput(); err != nil {
		t.Fatalf("clone bundle: %v\n%s", err, output)
	}
	installerArchiveRunGit(t, clone, "fsck", "--full", "--no-reflogs")
	installerArchiveRunGit(t, clone, "checkout", "--detach", "v0.15.1^{commit}")
	if got := installerArchiveGitOutput(t, clone, "rev-parse", "v0.15.1^{tag}"); got != tagObject {
		t.Fatalf("annotated tag = %s, want %s", got, tagObject)
	}
	if got := installerArchiveGitOutput(t, clone, "rev-parse", "v0.15.1^{commit}"); got != commit {
		t.Fatalf("peeled tag = %s, want %s", got, commit)
	}
	if got := installerArchiveGitOutput(t, clone, "rev-parse", "HEAD"); got != commit {
		t.Fatalf("HEAD = %s, want %s", got, commit)
	}
	if got := installerArchiveGitOutput(t, clone, "rev-parse", commit+":VERSION"); got != versionBlob {
		t.Fatalf("VERSION blob = %s, want %s", got, versionBlob)
	}
	version, err := os.ReadFile(filepath.Join(clone, "VERSION"))
	if err != nil || string(version) != "v0.15.1\n" {
		t.Fatalf("VERSION bytes = %q err=%v", version, err)
	}
	args := []string{"-C", clone, "archive", "--format=tar", "--prefix=baton-v0.15.1/", commit,
		"install-codex.sh", "install-claude.sh", "baton", "commands", "schemas"}
	expectedArchive, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatalf("construct archive from verified clone: %v", err)
	}
	if !bytes.Equal(expectedArchive, adopt.BatonInstallerArchive()) {
		t.Fatal("embedded installer archive differs from the verified clone archive")
	}
	if status := installerArchiveGitOutput(t, clone, "status", "--porcelain=v1", "--untracked-files=all"); status != "" {
		t.Fatalf("verified clone is dirty: %q", status)
	}
}

func installerArchiveTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if parent := filepath.Dir(dir); parent == dir {
			t.Fatal("cannot locate repository root")
		}
	}
}

func installerArchiveRunGit(t *testing.T, clone string, args ...string) {
	t.Helper()
	if output, err := exec.Command("git", append([]string{"-C", clone}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func installerArchiveGitOutput(t *testing.T, clone string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", clone}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestBatonV015CodexAndClaudeMirrorParity(t *testing.T) {
	bundle := extractInstallerBundle(t, adopt.BatonInstallerArchive())
	oracle := t.TempDir()
	oracleAgents := filepath.Join(oracle, "agents")
	oracleCodex := filepath.Join(oracle, "codex")
	oracleClaude := filepath.Join(oracle, "claude")
	oracleHome := filepath.Join(oracle, "home")
	oracleRecovery := filepath.Join(oracle, "sworn-config")
	assertContainedDisjointArchiveProofRoots(t, oracle, oracleHome, oracleAgents, oracleCodex, oracleClaude, oracleRecovery)
	oracleEnv := installerArchiveProofEnvironment(oracle, oracleHome, oracleAgents, oracleCodex, oracleClaude)

	runInstallerOracle(t, bundle, "install-codex.sh", oracleEnv)
	runInstallerOracle(t, bundle, "install-claude.sh", oracleEnv)

	native := t.TempDir()
	nativeAgents := filepath.Join(native, "agents")
	nativeCodex := filepath.Join(native, "codex")
	nativeClaude := filepath.Join(native, "claude")
	nativeHome := filepath.Join(native, "home")
	nativeRecovery := filepath.Join(native, "sworn-config")
	assertContainedDisjointArchiveProofRoots(t, native, nativeHome, nativeAgents, nativeCodex, nativeClaude, nativeRecovery)
	cmd := exec.Command("/bin/sh", "-c", `umask 0077; exec "$1" -test.run '^TestNativeInstallerHostileUmaskHelper$'`, "sh", os.Args[0])
	cmd.Env = append(installerArchiveProofEnvironment(native, nativeHome, nativeAgents, nativeCodex, nativeClaude),
		"SWORN_NATIVE_INSTALLER_HELPER=1",
		"SWORN_NATIVE_AGENTS_HOME="+nativeAgents,
		"SWORN_NATIVE_CODEX_HOME="+nativeCodex,
		"SWORN_NATIVE_CLAUDE_HOME="+nativeClaude,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native hostile-umask helper: %v\n%s", err, output)
	}

	compareFilesystemTrees(t, oracleAgents, nativeAgents)
	compareFilesystemTrees(t, oracleCodex, nativeCodex)
	compareFilesystemTrees(t, oracleClaude, nativeClaude)

	for _, command := range pinnedCommandNames {
		if _, err := os.Stat(filepath.Join(oracleClaude, "commands", command)); err != nil {
			t.Errorf("Claude oracle omitted %s: %v", command, err)
		}
		skill := "baton-" + strings.TrimSuffix(command, ".md")
		if _, err := os.Stat(filepath.Join(oracleAgents, "skills", skill, "SKILL.md")); err != nil {
			t.Errorf("Codex oracle omitted %s: %v", skill, err)
		}
	}
}

func TestNativeInstallerHostileUmaskHelper(t *testing.T) {
	if os.Getenv("SWORN_NATIVE_INSTALLER_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	trees, err := GenerateInstallerManagedTrees(adopt.BatonInstallerArchive())
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		root string
		tree ManagedTree
	}{
		{os.Getenv("SWORN_NATIVE_AGENTS_HOME"), trees.AgentsHome},
		{os.Getenv("SWORN_NATIVE_CODEX_HOME"), trees.CodexHome},
		{os.Getenv("SWORN_NATIVE_CLAUDE_HOME"), trees.ClaudeHome},
	} {
		if err := WriteManagedTree(target.root, target.tree); err != nil {
			t.Fatal(err)
		}
	}
}

func extractInstallerBundle(t *testing.T, archiveBytes []byte) string {
	t.Helper()
	if _, err := ValidateInstallerArchive(archiveBytes); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	r := tar.NewReader(bytes.NewReader(archiveBytes))
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		rel := strings.TrimPrefix(h.Name, installerArchivePrefix)
		rel = strings.TrimSuffix(rel, "/")
		if rel == "" {
			continue
		}
		dest := filepath.Join(root, filepath.FromSlash(rel))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dest, h.FileInfo().Mode().Perm()); err != nil {
				t.Fatal(err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			contents, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dest, contents, h.FileInfo().Mode().Perm()); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dest, h.FileInfo().Mode().Perm()); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unexpected node %q type %d", h.Name, h.Typeflag)
		}
	}
	return root
}

func runInstallerOracle(t *testing.T, bundle, script string, env []string) {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", `umask 0077; (umask 0022; exec /bin/bash "$1" -y)`, "sh", filepath.Join(bundle, script))
	cmd.Dir = bundle
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "BATON_ENGINE=sworn-proof-engine-not-present")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s oracle: %v\n%s", script, err, output)
	}
}

func installerArchiveProofEnvironment(base, home, agents, codex, claude string) []string {
	blocked := map[string]bool{
		"HOME": true, "AGENTS_HOME": true, "CODEX_HOME": true, "CLAUDE_HOME": true,
		"SWORN_HOME": true, "XDG_CONFIG_HOME": true,
	}
	env := make([]string, 0, len(os.Environ())+7)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !blocked[key] {
			env = append(env, item)
		}
	}
	return append(env,
		"HOME="+home,
		"AGENTS_HOME="+agents,
		"CODEX_HOME="+codex,
		"CLAUDE_HOME="+claude,
		"SWORN_HOME="+filepath.Join(base, "sworn-config"),
		"XDG_CONFIG_HOME="+filepath.Join(base, "xdg-config"),
	)
}

func assertContainedDisjointArchiveProofRoots(t *testing.T, base string, roots ...string) {
	t.Helper()
	base, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for i, root := range roots {
		root, err = filepath.Abs(root)
		if err != nil {
			t.Fatal(err)
		}
		if !archiveProofPathContained(base, root) {
			t.Fatalf("archive proof root escapes test root: %s", root)
		}
		if archiveProofPathsOverlap(realHome, root) {
			t.Fatalf("archive proof selected real home: %s", root)
		}
		for _, name := range []string{".agents", ".codex", ".claude"} {
			if archiveProofPathsOverlap(filepath.Join(realHome, name), root) {
				t.Fatalf("archive proof selected real %s home: %s", name, root)
			}
		}
		for _, other := range roots[:i] {
			if archiveProofPathsOverlap(root, other) {
				t.Fatalf("archive proof roots overlap: %s and %s", root, other)
			}
		}
	}
}

func archiveProofPathContained(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func archiveProofPathsOverlap(left, right string) bool {
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	return archiveProofPathContained(left, right) || archiveProofPathContained(right, left)
}

type filesystemSnapshotEntry struct {
	mode  fs.FileMode
	bytes []byte
}

func snapshotFilesystemTree(t *testing.T, root string) map[string]filesystemSnapshotEntry {
	t.Helper()
	result := make(map[string]filesystemSnapshotEntry)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := filesystemSnapshotEntry{mode: info.Mode()}
		if info.Mode().IsRegular() {
			item.bytes, err = os.ReadFile(name)
			if err != nil {
				return err
			}
		}
		result[rel] = item
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func compareFilesystemTrees(t *testing.T, oracleRoot, nativeRoot string) {
	t.Helper()
	oracle := snapshotFilesystemTree(t, oracleRoot)
	native := snapshotFilesystemTree(t, nativeRoot)
	paths := make([]string, 0, len(oracle)+len(native))
	seen := make(map[string]struct{})
	for name := range oracle {
		seen[name] = struct{}{}
		paths = append(paths, name)
	}
	for name := range native {
		if _, ok := seen[name]; !ok {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)
	for _, name := range paths {
		want, wantOK := oracle[name]
		got, gotOK := native[name]
		if wantOK != gotOK {
			t.Errorf("%s presence: oracle=%t native=%t", name, wantOK, gotOK)
			continue
		}
		if want.mode != got.mode {
			t.Errorf("%s mode: oracle=%s native=%s", name, want.mode, got.mode)
		}
		if !bytes.Equal(want.bytes, got.bytes) {
			t.Errorf("%s bytes differ: oracle_sha=%s native_sha=%s", name, shortSHA(want.bytes), shortSHA(got.bytes))
		}
	}
}

func shortSHA(contents []byte) string {
	digest := sha256.Sum256(contents)
	return fmt.Sprintf("%x", digest[:8])
}
