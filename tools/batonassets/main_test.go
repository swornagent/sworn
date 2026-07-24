package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSnapshotIsDeterministic(t *testing.T) {
	repo, commit := fixtureRepository(t)
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	second := filepath.Join(parent, "second")
	opts := options{
		repo: repo, commit: commit,
		paths: []string{"alpha.txt", "scripts/run.sh"},
	}
	opts.out = first
	if err := snapshot(opts, limits{asset: maxAssetBytes, total: maxTotalBytes}); err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	opts.out = second
	if err := snapshot(opts, limits{asset: maxAssetBytes, total: maxTotalBytes}); err != nil {
		t.Fatalf("second snapshot: %v", err)
	}

	firstTree := readOutputTree(t, first)
	secondTree := readOutputTree(t, second)
	if !reflect.DeepEqual(firstTree, secondTree) {
		t.Fatalf("repeated output differs:\nfirst: %#v\nsecond: %#v", firstTree, secondTree)
	}
	alphaDigest := sha256.Sum256([]byte("committed alpha\n"))
	scriptDigest := sha256.Sum256([]byte("#!/bin/sh\nexit 0\n"))
	wantManifest := `{"schema":"sworn.baton-assets/v1","commit":"` + commit +
		`","assets":[{"path":"alpha.txt","size":16,"sha256":"sha256:` + hex.EncodeToString(alphaDigest[:]) +
		`"},{"path":"scripts/run.sh","size":17,"sha256":"sha256:` + hex.EncodeToString(scriptDigest[:]) + `"}]}` + "\n"
	if got := string(firstTree["manifest.json"]); got != wantManifest {
		t.Fatalf("manifest mismatch:\ngot  %q\nwant %q", got, wantManifest)
	}
	var decoded manifest
	if err := json.Unmarshal(firstTree["manifest.json"], &decoded); err != nil {
		t.Fatal(err)
	}
	for _, entry := range decoded.Assets {
		if len(entry.SHA256) != 71 || !strings.HasPrefix(entry.SHA256, "sha256:") {
			t.Fatalf("asset digest is not canonical: %q", entry.SHA256)
		}
	}
	if got := firstTree["assets/alpha.txt"]; !bytes.Equal(got, []byte("committed alpha\n")) {
		t.Fatalf("alpha bytes = %q", got)
	}
	if got := firstTree["assets/scripts/run.sh"]; !bytes.Equal(got, []byte("#!/bin/sh\nexit 0\n")) {
		t.Fatalf("script bytes = %q", got)
	}
	info, err := os.Stat(filepath.Join(first, "assets", "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("executable mode = %o, want 755", info.Mode().Perm())
	}
	info, err = os.Stat(filepath.Join(first, "assets", "alpha.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("ordinary mode = %o, want 644", info.Mode().Perm())
	}
}

func TestSnapshotReadsCommitNotIndexWorktreeOrReplacement(t *testing.T) {
	repo, commit := fixtureRepository(t)
	originalOID := strings.TrimSpace(git(t, repo, "rev-parse", commit+":alpha.txt"))

	writeFile(t, filepath.Join(repo, "alpha.txt"), "staged mutation\n", 0o644)
	git(t, repo, "add", "alpha.txt")
	writeFile(t, filepath.Join(repo, "alpha.txt"), "dirty mutation\n", 0o644)
	replacementPath := filepath.Join(repo, "replacement")
	writeFile(t, replacementPath, "replacement mutation\n", 0o644)
	replacementOID := strings.TrimSpace(git(t, repo, "hash-object", "-w", replacementPath))
	git(t, repo, "replace", originalOID, replacementOID)

	out := filepath.Join(t.TempDir(), "snapshot")
	err := snapshot(options{
		repo: repo, commit: commit, paths: []string{"alpha.txt"}, out: out,
	}, limits{asset: maxAssetBytes, total: maxTotalBytes})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "assets", "alpha.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "committed alpha\n" {
		t.Fatalf("snapshot read mutable or replacement bytes: %q", got)
	}
}

func TestParsePathsRejectsMalformedAndTrailingJSON(t *testing.T) {
	cases := []string{
		``,
		`[]`,
		`null`,
		`{}`,
		`["alpha"] trailing`,
		`["alpha",]`,
		`[1]`,
		string([]byte{'[', '"', 0xff, '"', ']'}),
	}
	for _, input := range cases {
		if _, err := parsePaths(input); err == nil {
			t.Errorf("parsePaths(%q) unexpectedly succeeded", input)
		}
	}
}

func TestParsePathsRequiresCanonicalPOSIXPaths(t *testing.T) {
	invalid := []string{
		"", ".", "..", "/absolute", "./alpha", "alpha/", "alpha//beta",
		"alpha/./beta", "alpha/../beta", `alpha\beta`, "alpha\nbeta",
	}
	for _, value := range invalid {
		raw, err := json.Marshal([]string{value})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parsePaths(string(raw)); err == nil {
			t.Errorf("path %q unexpectedly succeeded", value)
		}
	}
	for _, raw := range []string{
		`["beta","alpha"]`,
		`["alpha","alpha"]`,
	} {
		if _, err := parsePaths(raw); err == nil {
			t.Errorf("ordering case %q unexpectedly succeeded", raw)
		}
	}
	got, err := parsePaths(`["alpha","nested/beta","日本語.txt"]`)
	if err != nil {
		t.Fatalf("valid paths: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("valid path count = %d", len(got))
	}
}

func TestSnapshotRejectsInvalidCommitIdentities(t *testing.T) {
	repo, commit := fixtureRepository(t)
	blob := strings.TrimSpace(git(t, repo, "rev-parse", commit+":alpha.txt"))
	git(t, repo, "tag", "-a", "annotated", "-m", "annotated")
	tag := strings.TrimSpace(git(t, repo, "rev-parse", "annotated^{tag}"))
	cases := []string{
		commit[:39],
		strings.ToUpper(commit),
		strings.Repeat("f", 40),
		blob,
		tag,
	}
	for index, candidate := range cases {
		err := snapshot(options{
			repo: repo, commit: candidate, paths: []string{"alpha.txt"},
			out: filepath.Join(t.TempDir(), "out"),
		}, limits{asset: maxAssetBytes, total: maxTotalBytes})
		if err == nil {
			t.Errorf("invalid commit case %d (%q) unexpectedly succeeded", index, candidate)
		}
	}
}

func TestSnapshotRejectsMissingAndNonBlobEntries(t *testing.T) {
	repo := newRepository(t)
	writeFile(t, filepath.Join(repo, "regular"), "regular", 0o644)
	writeFile(t, filepath.Join(repo, "directory", "file"), "nested", 0o644)
	if err := os.Symlink("regular", filepath.Join(repo, "link")); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "regular", "directory/file", "link")
	git(t, repo, "commit", "-q", "-m", "objects")
	parentCommit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+parentCommit+",gitlink")
	git(t, repo, "commit", "-q", "-m", "gitlink")
	commit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	for _, name := range []string{"absent", "directory", "link", "gitlink"} {
		err := snapshot(options{
			repo: repo, commit: commit, paths: []string{name},
			out: filepath.Join(t.TempDir(), "out"),
		}, limits{asset: maxAssetBytes, total: maxTotalBytes})
		if err == nil {
			t.Errorf("path %q unexpectedly succeeded", name)
		}
	}
}

func TestSnapshotEnforcesIndividualAndTotalByteCaps(t *testing.T) {
	repo := newRepository(t)
	writeFile(t, filepath.Join(repo, "a"), "abc", 0o644)
	writeFile(t, filepath.Join(repo, "b"), "de", 0o644)
	git(t, repo, "add", "a", "b")
	git(t, repo, "commit", "-q", "-m", "sizes")
	commit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	err := snapshot(options{
		repo: repo, commit: commit, paths: []string{"a"},
		out: filepath.Join(t.TempDir(), "individual"),
	}, limits{asset: 2, total: 4})
	if err == nil || !strings.Contains(err.Error(), "individual byte limit") {
		t.Fatalf("individual cap error = %v", err)
	}
	err = snapshot(options{
		repo: repo, commit: commit, paths: []string{"a", "b"},
		out: filepath.Join(t.TempDir(), "total"),
	}, limits{asset: 3, total: 4})
	if err == nil || !strings.Contains(err.Error(), "total byte limit") {
		t.Fatalf("total cap error = %v", err)
	}
}

func TestRunRejectsUnknownDuplicateAndMissingFlags(t *testing.T) {
	repo, commit := fixtureRepository(t)
	out := filepath.Join(t.TempDir(), "out")
	valid := []string{
		"snapshot", "--repo", repo, "--commit", commit,
		"--paths", `["alpha.txt"]`, "--out", out,
	}
	cases := [][]string{
		nil,
		{"other"},
		append(append([]string{}, valid...), "--unknown", "value"),
		append(append([]string{}, valid...), "--repo", repo),
		{"snapshot", "--repo", repo, "--commit", commit, "--paths", `["alpha.txt"]`},
		{"snapshot", "--repo"},
		{"snapshot", "--repo=" + repo, "--commit", commit, "--paths", `["alpha.txt"]`, "--out", out},
	}
	for index, args := range cases {
		if err := run(args); err == nil {
			t.Errorf("argument case %d unexpectedly succeeded: %#v", index, args)
		}
	}
}

func TestSnapshotRefusesExistingOutput(t *testing.T) {
	repo, commit := fixtureRepository(t)
	parent := t.TempDir()
	file := filepath.Join(parent, "file")
	writeFile(t, file, "keep", 0o644)
	directory := filepath.Join(parent, "directory")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{file, directory} {
		err := snapshot(options{
			repo: repo, commit: commit, paths: []string{"alpha.txt"}, out: out,
		}, limits{asset: maxAssetBytes, total: maxTotalBytes})
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("collision %q error = %v", out, err)
		}
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing file changed: %q, %v", got, err)
	}
}

func TestPublishCleansPrivateStageOnFailure(t *testing.T) {
	parent := t.TempDir()
	out := filepath.Join(parent, "snapshot")
	injected := errors.New("injected rename failure")
	err := publish(out, strings.Repeat("a", 40), []asset{{
		path: "alpha", data: []byte("alpha"), mode: 0o644,
	}}, func(_, _ string) error {
		return injected
	})
	if !errors.Is(err, injected) {
		t.Fatalf("publish error = %v", err)
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatalf("output exists after failure: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(parent, ".snapshot.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("private stage leaked: %v", matches)
	}
}

func TestRunCreatesSnapshot(t *testing.T) {
	repo, commit := fixtureRepository(t)
	out := filepath.Join(t.TempDir(), "snapshot")
	if err := run([]string{
		"snapshot", "--repo", repo, "--commit", commit,
		"--paths", `["alpha.txt"]`, "--out", out,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func fixtureRepository(t *testing.T) (string, string) {
	t.Helper()
	repo := newRepository(t)
	writeFile(t, filepath.Join(repo, "alpha.txt"), "committed alpha\n", 0o644)
	writeFile(t, filepath.Join(repo, "scripts", "run.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	git(t, repo, "add", "alpha.txt", "scripts/run.sh")
	git(t, repo, "commit", "-q", "-m", "fixture")
	return repo, strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

func newRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.name", "Baton Assets Test")
	git(t, repo, "config", "user.email", "baton-assets@example.invalid")
	return repo
}

func writeFile(t *testing.T, name, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, mode); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func readOutputTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	var names []string
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = body
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{
		"assets/alpha.txt", "assets/scripts/run.sh", "manifest.json",
	}) {
		t.Fatalf("output files = %v", names)
	}
	return files
}
