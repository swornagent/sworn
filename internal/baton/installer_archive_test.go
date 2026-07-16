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

func TestBatonV015CodexAndClaudeMirrorParity(t *testing.T) {
	bundle := extractInstallerBundle(t, adopt.BatonInstallerArchive())
	oracle := t.TempDir()
	oracleAgents := filepath.Join(oracle, "agents")
	oracleCodex := filepath.Join(oracle, "codex")
	oracleClaude := filepath.Join(oracle, "claude")
	oracleHome := filepath.Join(oracle, "home")

	runInstallerOracle(t, bundle, "install-codex.sh", []string{
		"HOME=" + oracleHome,
		"AGENTS_HOME=" + oracleAgents,
		"CODEX_HOME=" + oracleCodex,
	})
	runInstallerOracle(t, bundle, "install-claude.sh", []string{
		"HOME=" + oracleHome,
		"CLAUDE_HOME=" + oracleClaude,
	})

	native := t.TempDir()
	nativeAgents := filepath.Join(native, "agents")
	nativeCodex := filepath.Join(native, "codex")
	nativeClaude := filepath.Join(native, "claude")
	cmd := exec.Command("/bin/sh", "-c", `umask 0077; exec "$1" -test.run '^TestNativeInstallerHostileUmaskHelper$'`, "sh", os.Args[0])
	cmd.Env = append(os.Environ(),
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
