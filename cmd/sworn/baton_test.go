package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

// testBatonTag is a test-only Baton version tag.
var testBatonTag = "v9.8.7"

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

// -- upstream vendor integration tests (Rule 1: through the command) ---------

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
		files[m.Source] = fmt.Sprintf("# %s\n\nMinimal fixture content for integration test.\n", filepath.Base(m.Source))
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
	// Create VERSION file for WriteUpstreamPin.
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

func TestBatonVendorUpstream_RefreshesProjectContextSchema(t *testing.T) {
	owner, repo, tag := "sawy3r", "baton", testBatonTag
	commitSHA := "abc123def4567890123456789012345678abcdef"
	expected := "{\n  \"$id\": \"https://baton.sawy3r.net/schemas/project-context-v1.json\",\n  \"description\": \"The literal install.sh token proves schemas are copied byte-for-byte.\",\n  \"examples\": [{\"ratification\": {\"by\": \"sam\"}}]\n}\n"

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

	if exit := cmdBatonVendor([]string{"--upstream", "--repo", owner + "/" + repo, "--tag", tag}); exit != 0 {
		t.Fatalf("cmdBatonVendor --upstream exit = %d, want 0", exit)
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
