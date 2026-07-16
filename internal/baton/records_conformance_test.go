package baton_test

// Records-conformance sweep for the S12 record migration (sworn#48 data half).
//
// AC-03 / AC-06: every migrated spec.json and board.json across the five
// spec-v1-era releases validates against the vendored v0.10.0 spec-v1 / board-v1
// schema under full draft-2020-12 evaluation (baton.ValidateSchema, not the
// lenient hand-rolled baton.Validate). This doubles as durable CI regression —
// a future un-migrated record fails here — and Rule 1 reachability: the real
// committed records flow through the real strict validator, no fixture.
//
// AC-07 (sworn#95): after the type->ears_pattern migration + the ears.go reader
// repoint, the EARS classifier reads the migrated records and does NOT collapse
// every AC to Ubiquitous — the pre-migration all-Ubiquitous degradation is the
// regression this guards.
//
// One test file beyond S12's declared touchpoints (Captain acknowledged, review
// pin 3): no CLI surface runs ValidateSchema over on-disk records, so this Go
// test is the AC-03/AC-06 sweep mechanism.

import (
	"archive/tar"
	"bytes"
	"crypto/sha1" //nolint:gosec // Git's repository object format is SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/adopt"
	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/baton/schemas"
	"github.com/swornagent/sworn/internal/ears"
)

func TestBatonV015ExactParity(t *testing.T) {
	const (
		wantTag           = "v0.15.1"
		wantCommit        = "3fb4d275ae8a151f6287e7b9279d71628b12eea0"
		wantSourceDigest  = "sha256:8f0839ea897374eb10d6db2a789939714727739621babef1117d74cbf4488d2f"
		wantVersionBlob   = "5f1dd0af59642311ee04e018a0023562d4dde008"
		wantArchiveSHA256 = "27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15"
		wantArchiveBlob   = "39ae650dfe0282b0fa8bda14e1a01e7084077702"
		wantArchiveNodes  = 78
	)

	if baton.PinnedBatonTag != wantTag || baton.PinnedBatonCommit != wantCommit ||
		baton.PinnedBatonSourceDigest != wantSourceDigest || baton.PinnedBatonVersionBlobOID != wantVersionBlob ||
		baton.PinnedInstallerArchiveSHA256 != wantArchiveSHA256 || baton.PinnedInstallerArchiveBlobOID != wantArchiveBlob {
		t.Fatalf("compiled C-01 pin differs: tag=%q commit=%q digest=%q version_blob=%q archive_sha=%q archive_blob=%q",
			baton.PinnedBatonTag, baton.PinnedBatonCommit, baton.PinnedBatonSourceDigest, baton.PinnedBatonVersionBlobOID,
			baton.PinnedInstallerArchiveSHA256, baton.PinnedInstallerArchiveBlobOID)
	}

	upstreamVersion := []byte(wantTag + "\n")
	if got := gitBlobOIDForTest(upstreamVersion); got != wantVersionBlob {
		t.Fatalf("upstream VERSION blob = %s, want %s for exact %q bytes", got, wantVersionBlob, upstreamVersion)
	}

	archiveBytes := adopt.BatonInstallerArchive()
	digest := sha256.Sum256(archiveBytes)
	if got := hex.EncodeToString(digest[:]); got != wantArchiveSHA256 {
		t.Fatalf("embedded archive SHA-256 = %s, want %s", got, wantArchiveSHA256)
	}
	if got := gitBlobOIDForTest(archiveBytes); got != wantArchiveBlob {
		t.Fatalf("embedded archive Git blob = %s, want %s", got, wantArchiveBlob)
	}
	archive, err := baton.ValidateInstallerArchive(archiveBytes)
	if err != nil {
		t.Fatalf("ValidateInstallerArchive: %v", err)
	}
	if len(archive.Entries) != wantArchiveNodes {
		t.Fatalf("archive inventory = %d entries, want %d", len(archive.Entries), wantArchiveNodes)
	}
	assertExactArchiveInventory(t, archive)

	sourceRoot := extractEmbeddedInstallerBundle(t, archiveBytes)
	installPinnedGitShim(t, archiveBytes)
	repo := repoRoot(t)
	divergences, err := baton.Diff(baton.DiffOpts{SourceDir: sourceRoot, RepoRoot: repo})
	if err != nil {
		t.Fatalf("Diff(extracted embedded archive, repo): %v", err)
	}
	if len(divergences) != 0 {
		t.Fatalf("mapped repository parity differs from extracted embedded archive: %+v", divergences)
	}

	manifestPath := filepath.Join(repo, "internal", "adopt", "baton", "VERSION")
	committedManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	compiledManifest, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committedManifest, compiledManifest) {
		t.Fatal("committed adopting manifest differs from compiled binary manifest")
	}
	if baton.Version() != wantTag {
		t.Fatalf("Version() = %q, want %q", baton.Version(), wantTag)
	}
	pin, err := baton.ReadUpstreamPin()
	if err != nil {
		t.Fatal(err)
	}
	wantPin := baton.UpstreamPin{Tag: wantTag, SHA: wantCommit, Digest: wantSourceDigest}
	if pin != wantPin {
		t.Fatalf("adopting manifest pin = %#v, want %#v", pin, wantPin)
	}

	mappedSchemas := make(map[string]baton.FileMapping)
	seenDestinations := make(map[string]struct{}, len(baton.AllMappings()))
	for _, mapping := range baton.AllMappings() {
		if _, duplicate := seenDestinations[mapping.Dest]; duplicate {
			t.Fatalf("duplicate mapped destination %q", mapping.Dest)
		}
		seenDestinations[mapping.Dest] = struct{}{}
		if strings.HasPrefix(mapping.Source, "schemas/") {
			name := strings.TrimSuffix(filepath.Base(mapping.Source), ".json")
			mappedSchemas[name] = mapping
		}
	}
	if len(mappedSchemas) != len(schemas.SchemaMap) {
		t.Fatalf("mapped schema set = %d, embedded SchemaMap = %d", len(mappedSchemas), len(schemas.SchemaMap))
	}
	for name, embedded := range schemas.SchemaMap {
		mapping, ok := mappedSchemas[name]
		if !ok {
			t.Errorf("embedded schema %q has no live source mapping", name)
			continue
		}
		sourceBytes, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(mapping.Source)))
		if err != nil {
			t.Errorf("read archive schema %s: %v", mapping.Source, err)
			continue
		}
		repositoryBytes, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(mapping.Dest)))
		if err != nil {
			t.Errorf("read repository schema %s: %v", mapping.Dest, err)
			continue
		}
		if !bytes.Equal(sourceBytes, repositoryBytes) || !bytes.Equal(sourceBytes, embedded) {
			t.Errorf("normative schema %q is not byte-identical across archive, repository, and binary", name)
		}
	}
	if skew := baton.SchemaSkew(); len(skew) != 0 {
		t.Fatalf("v0.15 schema classification skew: %v", skew)
	}
}

// TestBatonV015CodexAndClaudeMirrorParity is the AC-03 record-sweep proof.
// installer_archive_test.go independently compares native materialisation with
// both tagged scripts; this named companion runs the same validated embedded
// authority through the scripts and asks the production read-only install
// checker to prove all three complete trees, modes, bytes, and VERSION sentinels.
func TestBatonV015CodexAndClaudeMirrorParity(t *testing.T) {
	archiveBytes := adopt.BatonInstallerArchive()
	bundle := extractEmbeddedInstallerBundle(t, archiveBytes)
	trees, err := baton.GenerateInstallerManagedTrees(archiveBytes)
	if err != nil {
		t.Fatal(err)
	}
	version, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		t.Fatal(err)
	}

	base := t.TempDir()
	home := filepath.Join(base, "home")
	roots := baton.InstallRoots{
		AgentsHome:   filepath.Join(base, "agents"),
		CodexHome:    filepath.Join(base, "codex"),
		ClaudeHome:   filepath.Join(base, "claude"),
		RecoveryRoot: filepath.Join(base, "recovery"),
	}
	runPinnedInstallerScript(t, bundle, "install-codex.sh", []string{
		"HOME=" + home,
		"AGENTS_HOME=" + roots.AgentsHome,
		"CODEX_HOME=" + roots.CodexHome,
	})
	runPinnedInstallerScript(t, bundle, "install-claude.sh", []string{
		"HOME=" + home,
		"CLAUDE_HOME=" + roots.ClaudeHome,
	})
	for _, root := range []string{roots.AgentsHome, roots.CodexHome, roots.ClaudeHome} {
		writeInstallVersionSentinel(t, root, version)
	}

	drift, err := baton.CheckBatonInstall(baton.InstallOpts{Roots: roots, Trees: trees, Version: version})
	if err != nil {
		t.Fatalf("CheckBatonInstall(script-created mirrors): %v", err)
	}
	if len(drift) != 0 {
		t.Fatalf("script-created mirrors differ from native canonical trees: %v", drift)
	}
}

func TestBatonV015ParityFailsClosedOnEveryLayer(t *testing.T) {
	t.Run("mapped repository byte", func(t *testing.T) {
		source, repository := newPinnedParityFixture(t)
		path := filepath.Join(repository, "internal", "adopt", "baton", "rules", "07-adversarial-verification.md")
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(original, []byte("\nmutation\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		divergences, err := baton.Diff(baton.DiffOpts{SourceDir: source, RepoRoot: repository})
		if err != nil || !hasDivergence(divergences, "internal/adopt/baton/rules/07-adversarial-verification.md") {
			t.Fatalf("mapped-byte mutation did not fail as deterministic drift: divs=%+v err=%v", divergences, err)
		}
	})

	t.Run("adopting manifest pin and binary identity", func(t *testing.T) {
		source, repository := newPinnedParityFixture(t)
		path := filepath.Join(repository, "internal", "adopt", "baton", "VERSION")
		manifest, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		mutated := bytes.Replace(manifest, []byte(baton.PinnedBatonSourceDigest), []byte("sha256:wrong"), 1)
		if bytes.Equal(mutated, manifest) {
			t.Fatal("manifest fixture did not contain pinned digest")
		}
		if err := os.WriteFile(path, mutated, 0o644); err != nil {
			t.Fatal(err)
		}
		divergences, err := baton.Diff(baton.DiffOpts{SourceDir: source, RepoRoot: repository})
		if err != nil || !hasDivergence(divergences, "internal/adopt/baton/VERSION") {
			t.Fatalf("manifest mutation did not fail against pin and compiled binary: divs=%+v err=%v", divergences, err)
		}
	})

	t.Run("upstream VERSION object", func(t *testing.T) {
		source, repository := newPinnedParityFixture(t)
		t.Setenv("SWORN_TEST_PINNED_VERSION_BYTES", "v0.15.1")
		if _, err := baton.Diff(baton.DiffOpts{SourceDir: source, RepoRoot: repository}); err == nil ||
			!strings.Contains(err.Error(), "VERSION bytes differ") {
			t.Fatalf("upstream VERSION mutation error = %v", err)
		}
	})

	t.Run("installer archive repository and compiled consumers", func(t *testing.T) {
		source, repository := newPinnedParityFixture(t)
		path := filepath.Join(repository, "internal", "adopt", "baton", "installer-input-v0.15.1.tar")
		mutated, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		mutated[len(mutated)/2] ^= 0x01
		if err := os.WriteFile(path, mutated, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := baton.Diff(baton.DiffOpts{SourceDir: source, RepoRoot: repository}); err == nil ||
			!strings.Contains(err.Error(), "malformed installer archive") {
			t.Fatalf("repository archive mutation error = %v", err)
		}
		if _, err := baton.ValidateInstallerArchive(mutated); err == nil {
			t.Fatal("ValidateInstallerArchive accepted mutated archive")
		}
		if _, err := baton.GenerateInstallerManagedTrees(mutated); err == nil {
			t.Fatal("native generator accepted mutated archive")
		}
	})

	t.Run("source inventory omission", func(t *testing.T) {
		source, repository := newPinnedParityFixture(t)
		if err := os.Remove(filepath.Join(source, "baton", "llm-checks", "spec-ambiguity.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := baton.Diff(baton.DiffOpts{SourceDir: source, RepoRoot: repository}); err == nil ||
			!strings.Contains(err.Error(), "source file missing") {
			t.Fatalf("missing source mutation error = %v", err)
		}
	})

	t.Run("schema classification", func(t *testing.T) {
		source, repository := newPinnedParityFixture(t)
		fixture := make(map[string][]byte, len(schemas.SchemaMap)+1)
		for name, raw := range schemas.SchemaMap {
			fixture[name] = raw
		}
		fixture["unclassified-v1"] = []byte(`{"$id":"https://baton.sawy3r.net/schemas/unclassified-v1.json","type":"object"}`)
		baton.SetSchemaMapForTest(fixture)
		defer baton.ClearSchemaMapForTest()
		divergences, err := baton.Diff(baton.DiffOpts{SourceDir: source, RepoRoot: repository})
		if err != nil || !hasDivergence(divergences, "internal/baton/schemas/embed.go") {
			t.Fatalf("schema classification mutation did not fail closed: divs=%+v err=%v", divergences, err)
		}
	})

	for _, test := range []struct {
		name     string
		logical  string
		root     func(baton.InstallRoots) string
		tree     func(baton.InstallerManagedTrees) baton.ManagedTree
		sentinel bool
	}{
		{"agents managed tree", "agents_home", func(r baton.InstallRoots) string { return r.AgentsHome }, func(t baton.InstallerManagedTrees) baton.ManagedTree { return t.AgentsHome }, false},
		{"codex managed tree", "codex_home", func(r baton.InstallRoots) string { return r.CodexHome }, func(t baton.InstallerManagedTrees) baton.ManagedTree { return t.CodexHome }, false},
		{"claude managed tree", "claude_home", func(r baton.InstallRoots) string { return r.ClaudeHome }, func(t baton.InstallerManagedTrees) baton.ManagedTree { return t.ClaudeHome }, false},
		{"agents VERSION sentinel", "agents_home", func(r baton.InstallRoots) string { return r.AgentsHome }, nil, true},
		{"codex VERSION sentinel", "codex_home", func(r baton.InstallRoots) string { return r.CodexHome }, nil, true},
		{"claude VERSION sentinel", "claude_home", func(r baton.InstallRoots) string { return r.ClaudeHome }, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts := newExactInstallFixture(t)
			root := test.root(opts.Roots)
			if test.sentinel {
				path := filepath.Join(root, ".sworn-baton", "VERSION")
				if err := os.WriteFile(path, []byte("wrong\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				entry := firstManagedFile(t, test.tree(opts.Trees))
				path := filepath.Join(root, filepath.FromSlash(entry.Path))
				if err := os.WriteFile(path, append(entry.Bytes, 0x00), entry.Mode); err != nil {
					t.Fatal(err)
				}
			}
			drift, err := baton.CheckBatonInstall(opts)
			if err != nil || !hasLogicalDrift(drift, test.logical) {
				t.Fatalf("%s mutation did not fail closed: drift=%v err=%v", test.logical, drift, err)
			}
		})
	}
}

func assertExactArchiveInventory(t *testing.T, archive *baton.InstallerArchive) {
	t.Helper()
	seen := make(map[string]struct{}, len(archive.Entries))
	paths := make([]string, 0, len(archive.Entries))
	for _, entry := range archive.Entries {
		if _, duplicate := seen[entry.Path]; duplicate {
			t.Fatalf("duplicate archive inventory path %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		paths = append(paths, entry.Path)
		if entry.IsDir {
			if entry.Mode != 0o775 || len(entry.Bytes) != 0 || entry.BlobOID != "" {
				t.Fatalf("archive directory %q identity = mode %04o bytes=%d blob=%q", entry.Path, entry.Mode, len(entry.Bytes), entry.BlobOID)
			}
			continue
		}
		if entry.Mode != 0o664 && entry.Mode != 0o775 {
			t.Fatalf("archive file %q mode = %04o", entry.Path, entry.Mode)
		}
		if entry.BlobOID == "" || entry.BlobOID != gitBlobOIDForTest(entry.Bytes) {
			t.Fatalf("archive file %q blob identity is absent or stale", entry.Path)
		}
	}
	if !sort.StringsAreSorted(paths) {
		t.Fatal("archive inventory is not byte-sorted")
	}
	for _, required := range []string{"install-codex.sh", "install-claude.sh", "baton", "commands", "schemas"} {
		if _, ok := seen[required]; !ok {
			t.Errorf("archive inventory omits required root %q", required)
		}
	}
	for _, command := range []string{
		"design-review.md", "implement-slice.md", "mark-shipped.md", "merge-release.md",
		"merge-track.md", "plan-release.md", "replan-release.md", "verify-slice.md",
	} {
		if _, ok := seen[filepath.ToSlash(filepath.Join("commands", command))]; !ok {
			t.Errorf("archive inventory omits command %q", command)
		}
	}
	for _, mapping := range baton.AllMappings() {
		if mapping.Source == "baton/rules.md" {
			continue
		}
		if _, ok := seen[mapping.Source]; !ok {
			t.Errorf("archive inventory omits mapped source %q", mapping.Source)
		}
	}
}

func gitBlobOIDForTest(contents []byte) string {
	header := fmt.Sprintf("blob %d%c", len(contents), 0)
	digest := sha1.Sum(append([]byte(header), contents...))
	return hex.EncodeToString(digest[:])
}

func extractEmbeddedInstallerBundle(t *testing.T, archiveBytes []byte) string {
	t.Helper()
	if _, err := baton.ValidateInstallerArchive(archiveBytes); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	reader := tar.NewReader(bytes.NewReader(archiveBytes))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		const prefix = "baton-v0.15.1/"
		if !strings.HasPrefix(header.Name, prefix) {
			t.Fatalf("validated archive path lacks %q prefix: %q", prefix, header.Name)
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(header.Name, prefix), "/")
		if rel == "" {
			continue
		}
		destination := filepath.Join(root, filepath.FromSlash(rel))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, header.FileInfo().Mode().Perm()); err != nil {
				t.Fatal(err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				t.Fatal(err)
			}
			contents, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(destination, contents, header.FileInfo().Mode().Perm()); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("validated archive exposed unsupported type %d", header.Typeflag)
		}
	}
	return root
}

func runPinnedInstallerScript(t *testing.T, bundle, script string, environment []string) {
	t.Helper()
	command := exec.Command("/bin/sh", "-c", `umask 0077; (umask 0022; exec /bin/bash "$1" -y)`, "sh", filepath.Join(bundle, script))
	command.Dir = bundle
	command.Env = append(os.Environ(), environment...)
	command.Env = append(command.Env, "BATON_ENGINE=sworn-proof-engine-not-present")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s isolated oracle: %v\n%s", script, err, output)
	}
}

func writeInstallVersionSentinel(t *testing.T, root string, version []byte) {
	t.Helper()
	directory := filepath.Join(root, ".sworn-baton")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "VERSION")
	if err := os.WriteFile(path, version, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
}

func installPinnedGitShim(t *testing.T, archiveBytes []byte) {
	t.Helper()
	shimRoot := t.TempDir()
	archivePath := filepath.Join(shimRoot, "installer-input.tar")
	if err := os.WriteFile(archivePath, archiveBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	gitPath := filepath.Join(shimRoot, "git")
	const shim = `#!/bin/sh
set -eu
if [ "$1" = "-C" ]; then
  shift 2
fi
case "$1:$2" in
  rev-parse:HEAD)
    printf '%s\n' "$SWORN_TEST_PINNED_COMMIT"
    ;;
  rev-parse:v0.15.1^{commit})
    printf '%s\n' "$SWORN_TEST_PINNED_COMMIT"
    ;;
  rev-parse:3fb4d275ae8a151f6287e7b9279d71628b12eea0:VERSION)
    printf '%s\n' "$SWORN_TEST_PINNED_VERSION_BLOB"
    ;;
  status:--porcelain=v1)
    ;;
  cat-file:blob)
    printf '%s' "$SWORN_TEST_PINNED_VERSION_BYTES"
    ;;
  archive:*)
    exec /bin/cat "$SWORN_TEST_PINNED_ARCHIVE"
    ;;
  *)
    printf 'unexpected git shim argv: %s\n' "$*" >&2
    exit 64
    ;;
esac
`
	if err := os.WriteFile(gitPath, []byte(shim), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimRoot+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SWORN_TEST_PINNED_COMMIT", baton.PinnedBatonCommit)
	t.Setenv("SWORN_TEST_PINNED_VERSION_BLOB", baton.PinnedBatonVersionBlobOID)
	t.Setenv("SWORN_TEST_PINNED_VERSION_BYTES", baton.PinnedBatonTag+"\n")
	t.Setenv("SWORN_TEST_PINNED_ARCHIVE", archivePath)
}

func newPinnedParityFixture(t *testing.T) (string, string) {
	t.Helper()
	archiveBytes := adopt.BatonInstallerArchive()
	source := extractEmbeddedInstallerBundle(t, archiveBytes)
	installPinnedGitShim(t, archiveBytes)
	repository := t.TempDir()
	live := repoRoot(t)
	paths := make(map[string]struct{}, len(baton.AllMappings())+2)
	for _, mapping := range baton.AllMappings() {
		paths[mapping.Dest] = struct{}{}
	}
	paths["internal/adopt/baton/VERSION"] = struct{}{}
	paths["internal/adopt/baton/installer-input-v0.15.1.tar"] = struct{}{}
	for name := range paths {
		copyParityFile(t, filepath.Join(live, filepath.FromSlash(name)), filepath.Join(repository, filepath.FromSlash(name)))
	}
	return source, repository
}

func copyParityFile(t *testing.T, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, contents, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
}

func hasDivergence(divergences []baton.Divergence, path string) bool {
	for _, divergence := range divergences {
		if divergence.File == path {
			return true
		}
	}
	return false
}

func newExactInstallFixture(t *testing.T) baton.InstallOpts {
	t.Helper()
	trees, err := baton.GenerateInstallerManagedTrees(adopt.BatonInstallerArchive())
	if err != nil {
		t.Fatal(err)
	}
	version, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	opts := baton.InstallOpts{
		Roots: baton.InstallRoots{
			AgentsHome: filepath.Join(base, "agents"), CodexHome: filepath.Join(base, "codex"),
			ClaudeHome: filepath.Join(base, "claude"), RecoveryRoot: filepath.Join(base, "recovery"),
		},
		Trees: trees, Version: version,
	}
	result, err := baton.SyncBatonInstall(opts)
	if err != nil || result.State != baton.InstallRepaired {
		t.Fatalf("create exact isolated install = %#v, %v", result, err)
	}
	if drift, err := baton.CheckBatonInstall(opts); err != nil || len(drift) != 0 {
		t.Fatalf("exact isolated install precondition = %v, %v", drift, err)
	}
	return opts
}

func firstManagedFile(t *testing.T, tree baton.ManagedTree) baton.ManagedTreeEntry {
	t.Helper()
	for _, entry := range tree.Entries {
		if !entry.IsDir {
			return entry
		}
	}
	t.Fatal("managed tree contains no regular file")
	return baton.ManagedTreeEntry{}
}

func hasLogicalDrift(drift []string, logical string) bool {
	for _, path := range drift {
		if path == logical || strings.HasPrefix(path, logical+"/") {
			return true
		}
	}
	return false
}

// specV1EraReleases are the five releases that carry spec.json records and are
// in scope for the S12 v0.10.0 migration. Pre-spec-v1 legacy releases
// (markdown-era, 0 spec.json) are excluded (Coach decision 2026-07-10).
var specV1EraReleases = []string{
	"2026-06-28-driver-contract",
	"2026-06-30-sworn-operational-readiness",
	"2026-07-01-loop-cli-ux",
	"2026-07-01-release-hygiene",
	"2026-07-01-render-drift-reconciliation",
}

// repoRoot walks up from this test file to the directory that contains
// docs/release — the repo (worktree) root — so the sweep reads the real
// committed records rather than a temp fixture.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 12; i++ {
		if fi, err := os.Stat(filepath.Join(dir, "docs", "release")); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root (docs/release) from %s", file)
	return ""
}

// TestRecordsConformance_SpecV1Era proves AC-03 / AC-06: every migrated spec.json
// and board.json across the five spec-v1-era releases conforms to the strict
// vendored v0.10.0 schema.
func TestRecordsConformance_SpecV1Era(t *testing.T) {
	root := repoRoot(t)
	specCount, boardCount := 0, 0
	for _, rel := range specV1EraReleases {
		relDir := filepath.Join(root, "docs", "release", rel)
		if fi, err := os.Stat(relDir); err != nil || !fi.IsDir() {
			t.Fatalf("spec-v1-era release dir missing: %s", relDir)
		}

		specs, err := filepath.Glob(filepath.Join(relDir, "S*", "spec.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(specs) == 0 {
			t.Errorf("release %s: no spec.json found (glob broken or release un-migrated)", rel)
		}
		for _, p := range specs {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := baton.ValidateSchema("spec-v1", data); err != nil {
				t.Errorf("spec-v1 conformance FAIL %s:\n  %v", p, err)
			}
			specCount++
		}

		boardPath := filepath.Join(relDir, "board.json")
		data, err := os.ReadFile(boardPath)
		if err != nil {
			t.Fatalf("read board.json %s: %v", boardPath, err)
		}
		if err := baton.ValidateSchema("board-v1", data); err != nil {
			t.Errorf("board-v1 conformance FAIL %s:\n  %v", boardPath, err)
		}
		boardCount++
	}

	// Fail closed: the sweep must actually have validated records. The five
	// releases carry 15/6/3/2/7 = 33 spec.json; a broken glob that validates
	// nothing must not read as PASS.
	if specCount < 33 {
		t.Fatalf("expected >=33 spec.json across the five spec-v1-era releases, validated %d — glob likely broken", specCount)
	}
	if boardCount != len(specV1EraReleases) {
		t.Fatalf("expected %d board.json, validated %d", len(specV1EraReleases), boardCount)
	}
	t.Logf("records-conformance PASS: %d spec.json + %d board.json validate against v0.10.0 spec-v1/board-v1", specCount, boardCount)
}

// TestRecordsConformance_EARSClassificationPreserved proves AC-07 on real
// migrated data (sworn#95): running the EARS classifier over the migrated
// driver-contract release does NOT collapse every AC to Ubiquitous — the
// event-driven and unwanted-behaviour ACs are still classified as such. The
// pre-fix stale reader (reading the now-absent ears_keyword) produced an
// all-Ubiquitous distribution; this test fails closed on that regression.
func TestRecordsConformance_EARSClassificationPreserved(t *testing.T) {
	root := repoRoot(t)
	relDir := filepath.Join(root, "docs", "release", "2026-06-28-driver-contract")

	report, err := ears.Validate(relDir)
	if err != nil {
		t.Fatalf("ears.Validate(%s): %v", relDir, err)
	}
	if report.HasViolations() {
		t.Fatalf("unexpected EARS violations in migrated release: %d", len(report.Violations))
	}
	if report.Dist[ears.PatternEventDriven] == 0 {
		t.Error("event-driven ACs classified as 0 — EARS classification degraded (sworn#95 regression)")
	}
	if report.Dist[ears.PatternUnwanted] == 0 {
		t.Error("unwanted-behaviour ACs classified as 0 — EARS classification degraded (sworn#95 regression)")
	}
	if report.TotalACs > 0 && report.Dist[ears.PatternUbiquitous] == report.TotalACs {
		t.Errorf("ALL %d ACs classified Ubiquitous — the sworn#95 all-Ubiquitous degradation", report.TotalACs)
	}
	t.Logf("EARS classification preserved on migrated data: ubiquitous=%d event-driven=%d state-driven=%d unwanted-behaviour=%d total=%d",
		report.Dist[ears.PatternUbiquitous], report.Dist[ears.PatternEventDriven],
		report.Dist[ears.PatternStateDriven], report.Dist[ears.PatternUnwanted], report.TotalACs)
}
