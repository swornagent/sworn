package baton

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testVersionTag is a test-only Baton version tag used in fetch tests.
// It is not the real Baton protocol version.
var testVersionTag = "v9.8.7"

// makeTarball creates a gzipped tar archive in memory with the GitHub-style
// top-level prefix <repo>-<tag>/. Each entry in files is a relative path
// within the archive (e.g. "README.md" maps to <repo>-<tag>/README.md).
func makeTarball(repoName, tag string, files map[string]string) []byte {
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

	for name, content := range files {
		path := prefix + "/" + name
		// Ensure parent dirs exist in the tar.
		parent := filepath.Dir(path)
		if parent != "." && parent != prefix {
			_ = tw.WriteHeader(&tar.Header{
				Name:     parent + "/",
				Typeflag: tar.TypeDir,
				Mode:     0755,
			})
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

// ghTestServer creates an httptest.Server that serves both the GitHub API
// (api.github.com) and codeload (codeload.github.com) endpoints for the
// given repo/tag. Set baseURLForTest to ts.URL before calling FetchUpstream.
//
// statusOverride keys: "api" or "codeload" → HTTP status code.
func ghTestServer(owner, name, tag, commitSHA string, tarball []byte, statusOverride map[string]int) *httptest.Server {
	mux := http.NewServeMux()

	// API: /repos/{owner}/{name}/commits/{tag}
	apiPath := fmt.Sprintf("/repos/%s/%s/commits/%s", owner, name, tag)
	mux.HandleFunc(apiPath, func(w http.ResponseWriter, r *http.Request) {
		code := http.StatusOK
		if statusOverride != nil {
			if c, ok := statusOverride["api"]; ok {
				code = c
			}
		}
		if code != http.StatusOK {
			if code == http.StatusNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "error", code)
			return
		}
		resp := ghCommitResponse{SHA: commitSHA}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Codeload: /{owner}/{name}/tar.gz/refs/tags/{tag}
	codeloadPath := fmt.Sprintf("/%s/%s/tar.gz/refs/tags/%s", owner, name, tag)
	mux.HandleFunc(codeloadPath, func(w http.ResponseWriter, r *http.Request) {
		code := http.StatusOK
		if statusOverride != nil {
			if c, ok := statusOverride["codeload"]; ok {
				code = c
			}
		}
		if code != http.StatusOK {
			if code == http.StatusNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "error", code)
			return
		}
		w.Header().Set("Content-Type", "application/x-gzip")
		w.Write(tarball)
	})

	return httptest.NewServer(mux)
}

// tarballDigest returns the sha256:<hex> digest of the tarball bytes.
func tarballDigest(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// setTestPin records a pin for a specific tag. The tag matters: the SHA/digest
// guard only applies when the fetch requests the tag the pin describes.
func setTestPin(tag, sha, digest string) {
	upstreamPinForTest = &UpstreamPin{Tag: tag, SHA: sha, Digest: digest}
}

func clearTestPin() {
	upstreamPinForTest = nil
}

func TestFetchUpstream_Success(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123def456"
	files := map[string]string{
		"README.md":                  "# Baton",
		"baton/reachability-gate.md": "# Rule 1",
	}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)
	setTestPin(testVersionTag, commitSHA, digest)

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, nil)
	defer ts.Close()
	baseURLForTest = ts.URL

	result, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err != nil {
		t.Fatalf("FetchUpstream failed: %v", err)
	}
	defer result.Cleanup()

	if result.SHA != commitSHA {
		t.Errorf("SHA = %q, want %q", result.SHA, commitSHA)
	}
	if result.Digest != digest {
		t.Errorf("Digest = %q, want %q", result.Digest, digest)
	}

	// Verify extracted files.
	readmePath := filepath.Join(result.SourceDir, "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", readmePath, err)
	}
	if string(data) != "# Baton" {
		t.Errorf("README.md = %q, want %q", string(data), "# Baton")
	}

	// Verify prefix is stripped (no baton-v0.4.2/ prefix in extracted paths).
	prefixedPath := filepath.Join(result.SourceDir, "baton-v0.4.2")
	if _, err := os.Stat(prefixedPath); !os.IsNotExist(err) {
		t.Errorf("prefix dir %s exists — prefix was not stripped", prefixedPath)
	}
}

func TestFetchUpstream_SHAMismatch(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)

	// Pin a different SHA.
	setTestPin(testVersionTag, "xyz789", digest)

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, nil)
	defer ts.Close()
	baseURLForTest = ts.URL

	_, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err == nil {
		t.Fatal("expected SHA mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "SHA mismatch") {
		t.Errorf("error = %v, want SHA mismatch", err)
	}
}

func TestFetchUpstream_DigestMismatch(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)

	// Pin correct SHA but wrong digest.
	setTestPin(testVersionTag, commitSHA, "sha256:0000000000000000000000000000000000000000000000000000000000000000")

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, nil)
	defer ts.Close()
	baseURLForTest = ts.URL

	_, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err == nil {
		t.Fatal("expected digest mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Errorf("error = %v, want digest mismatch", err)
	}
}

// TestFetchUpstream_VersionBumpIsNotTampering is the regression guard for the
// bug that made `sworn baton vendor --upstream --tag <newer>` permanently
// unusable.
//
// The SHA/digest pin describes ONE tag. The guard compared it against whatever
// tag was requested, so bumping to a new tag — the entire purpose of --tag —
// aborted with "the tag may have been force-moved", because a different tag
// resolves to a different SHA by definition. Trust is per-tag: a new tag has no
// prior pin to verify against.
func TestFetchUpstream_VersionBumpIsNotTampering(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	const (
		pinnedTag = "v1.0.0"
		newTag    = "v1.1.0"
	)
	owner, name := "sawy3r", "baton"

	// The pin describes the OLD tag: its SHA and its digest.
	setTestPin(pinnedTag, "old-tag-sha", "sha256:"+strings.Repeat("a", 64))

	// We now fetch a DIFFERENT tag, which naturally resolves to a different SHA
	// and a different tarball. That is a version bump, not tampering.
	newSHA := "new-tag-sha"
	tarball := makeTarball(name, newTag, map[string]string{"README.md": "# Baton v1.1.0"})

	ts := ghTestServer(owner, name, newTag, newSHA, tarball, nil)
	defer ts.Close()
	baseURLForTest = ts.URL

	result, err := FetchUpstream(context.Background(), owner+"/"+name, newTag)
	if err != nil {
		t.Fatalf("fetching a NEW tag must not be treated as tampering, got: %v\n"+
			"the pin describes %s; %s has no prior pin to verify against", err, pinnedTag, newTag)
	}
	defer result.Cleanup()

	if result.SHA != newSHA {
		t.Errorf("SHA = %q, want %q", result.SHA, newSHA)
	}
	if result.Digest != tarballDigest(tarball) {
		t.Errorf("Digest = %q, want the new tag's digest", result.Digest)
	}
}

func TestFetchUpstream_NoDigestPinBootstrap(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)

	// Pin SHA but no digest — first fetch (bootstrap).
	setTestPin(testVersionTag, commitSHA, "")

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, nil)
	defer ts.Close()
	baseURLForTest = ts.URL

	result, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err != nil {
		t.Fatalf("FetchUpstream failed on bootstrap: %v", err)
	}
	defer result.Cleanup()

	if result.SHA != commitSHA {
		t.Errorf("SHA = %q, want %q", result.SHA, commitSHA)
	}
	if result.Digest != digest {
		t.Errorf("Digest = %q, want %q", result.Digest, digest)
	}
}

func TestFetchUpstream_NoSHAPinBootstrap(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)

	// No SHA pin, no digest pin — first ever fetch.
	setTestPin(testVersionTag, "", "")

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, nil)
	defer ts.Close()
	baseURLForTest = ts.URL

	result, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err != nil {
		t.Fatalf("FetchUpstream failed on bootstrap (no pins): %v", err)
	}
	defer result.Cleanup()

	if result.SHA != commitSHA {
		t.Errorf("SHA = %q, want %q", result.SHA, commitSHA)
	}
	if result.Digest != digest {
		t.Errorf("Digest = %q, want %q", result.Digest, digest)
	}
}

func TestFetchUpstream_APINotFound(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	tarball := makeTarball(name, tag, map[string]string{"README.md": "# Baton"})
	digest := tarballDigest(tarball)
	setTestPin(testVersionTag, commitSHA, digest)

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, map[string]int{"api": 404})
	defer ts.Close()
	baseURLForTest = ts.URL

	_, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err == nil {
		t.Fatal("expected 404 error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want not found", err)
	}
}

func TestFetchUpstream_CodeloadNotFound(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	tarball := makeTarball(name, tag, map[string]string{"README.md": "# Baton"})
	digest := tarballDigest(tarball)
	setTestPin(testVersionTag, commitSHA, digest)

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, map[string]int{"codeload": 404})
	defer ts.Close()
	baseURLForTest = ts.URL

	_, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err == nil {
		t.Fatal("expected 404 error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want not found (from codeload)", err)
	}
}

func TestFetchUpstream_ServerError(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	tarball := makeTarball(name, tag, map[string]string{"README.md": "# Baton"})
	digest := tarballDigest(tarball)
	setTestPin(testVersionTag, commitSHA, digest)

	ts := ghTestServer(owner, name, tag, commitSHA, tarball, map[string]int{"api": 500})
	defer ts.Close()
	baseURLForTest = ts.URL

	_, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err == nil {
		t.Fatal("expected 500 error, got nil")
	}
}

func TestFetchUpstream_BadGzip(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", testVersionTag
	commitSHA := "abc123"
	// Not real gzip — will fail on decompress.
	badTarball := []byte("this is not a gzip file")
	digest := tarballDigest(badTarball)
	setTestPin(testVersionTag, "", digest) // no SHA pin so SHA check passes; digest matches bad data

	ts := ghTestServer(owner, name, tag, commitSHA, badTarball, nil)
	defer ts.Close()
	baseURLForTest = ts.URL

	_, err := FetchUpstream(context.Background(), owner+"/"+name, tag)
	if err == nil {
		t.Fatal("expected gzip error, got nil")
	}
	if !strings.Contains(err.Error(), "gzip") {
		t.Errorf("error = %v, want gzip error", err)
	}
}

func TestFetchUpstream_RepoFormatValidation(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	// Set up a test server so "owner/name" doesn't hit real network.
	tb := makeTarball("baton", testVersionTag, map[string]string{"README.md": "# Baton"})
	ts := ghTestServer("sawy3r", "baton", testVersionTag, "abc123", tb, nil)
	defer ts.Close()
	baseURLForTest = ts.URL
	setTestPin(testVersionTag, "abc123", tarballDigest(tb))
	tests := []struct {
		repo    string
		wantErr bool
	}{
		{"sawy3r/baton", false},
		{"", true},
		{"no-slash", true},
		{"too/many/slashes", true},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			_, err := FetchUpstream(context.Background(), tt.repo, testVersionTag)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchUpstream(repo=%q) error = %v, wantErr = %v", tt.repo, err, tt.wantErr)
			}
		})
	}
}
func TestFetchUpstream_EmptyTag(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)

	_, err := FetchUpstream(context.Background(), "owner/name", "")
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}
