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

// setTestPin sets the upstream pin for testing.
func setTestPin(sha, digest string) {
	upstreamPinForTest = &UpstreamPin{SHA: sha, Digest: digest}
}

func clearTestPin() {
	upstreamPinForTest = nil
}

func TestFetchUpstream_Success(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123def456"
	files := map[string]string{
		"README.md":                         "# Baton",
		"claude/baton/reachability-gate.md": "# Rule 1",
	}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)
	setTestPin(commitSHA, digest)

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

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)

	// Pin a different SHA.
	setTestPin("xyz789", digest)

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

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)

	// Pin correct SHA but wrong digest.
	setTestPin(commitSHA, "sha256:0000000000000000000000000000000000000000000000000000000000000000")

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

func TestFetchUpstream_NoDigestPinBootstrap(t *testing.T) {
	clearTestPin()
	t.Cleanup(clearTestPin)
	t.Cleanup(func() { baseURLForTest = "" })

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)

	// Pin SHA but no digest — first fetch (bootstrap).
	setTestPin(commitSHA, "")

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

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	files := map[string]string{"README.md": "# Baton"}
	tarball := makeTarball(name, tag, files)
	digest := tarballDigest(tarball)

	// No SHA pin, no digest pin — first ever fetch.
	setTestPin("", "")

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

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	tarball := makeTarball(name, tag, map[string]string{"README.md": "# Baton"})
	digest := tarballDigest(tarball)
	setTestPin(commitSHA, digest)

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

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	tarball := makeTarball(name, tag, map[string]string{"README.md": "# Baton"})
	digest := tarballDigest(tarball)
	setTestPin(commitSHA, digest)

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

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	tarball := makeTarball(name, tag, map[string]string{"README.md": "# Baton"})
	digest := tarballDigest(tarball)
	setTestPin(commitSHA, digest)

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

	owner, name, tag := "sawy3r", "baton", "v0.4.2"
	commitSHA := "abc123"
	// Not real gzip — will fail on decompress.
	badTarball := []byte("this is not a gzip file")
	digest := tarballDigest(badTarball)
	setTestPin("", digest) // no SHA pin so SHA check passes; digest matches bad data

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
	tb := makeTarball("baton", "v0.4.2", map[string]string{"README.md": "# Baton"})
	ts := ghTestServer("sawy3r", "baton", "v0.4.2", "abc123", tb, nil)
	defer ts.Close()
	baseURLForTest = ts.URL
	setTestPin("abc123", tarballDigest(tb))
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
			_, err := FetchUpstream(context.Background(), tt.repo, "v0.4.2")
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
