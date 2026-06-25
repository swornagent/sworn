package baton

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)
// FetchUpstream downloads and verifies a Baton release tarball from the public
// GitHub repo at the pinned semver tag, then extracts it into a temp source
// directory — stdlib only, no git binary, no module dependency.
//
// Two network calls:
//  1. api.github.com/repos/{owner}/{repo}/commits/{tag} — resolves the commit
//     SHA (handles annotated tags correctly) to guard against force-moved tags.
//  2. codeload.github.com/{owner}/{repo}/tar.gz/refs/tags/{tag} — fetches the
//     release tarball for extraction.
//
// The resolved commit SHA is verified against the upstream-sha pin, and the
// content digest (SHA-256 of the raw tarball bytes) is verified against the
// upstream-digest pin. On first fetch (no upstream-digest pin), digest
// verification is skipped — the SHA still catches force-moved tags. The
// computed digest is returned so the caller can persist it after a successful
// Vendor.
//
// Tarballs wrap content in a top-level <repo>-<ref>/ directory; the extractor
// strips it so every mapped path resolves correctly.
//
// repo must be "owner/name" format.
// tag must be a semver tag (e.g. "v0.4.2").
//
// On any error (network failure, non-2xx, tag not found, SHA mismatch, digest
// mismatch, bad gzip, tar error), the returned error is non-nil and no files
// are written to disk (the temp dir is cleaned up before return).
func FetchUpstream(ctx context.Context, repo, tag string) (*FetchResult, error) {
	if !strings.Contains(repo, "/") || strings.Count(repo, "/") != 1 {
		return nil, fmt.Errorf("baton: repo must be owner/name format, got %q", repo)
	}
	if tag == "" {
		return nil, fmt.Errorf("baton: tag is required; use the pinned semver tag (baton-protocol from VERSION)")
	}

	parts := strings.SplitN(repo, "/", 2)
	owner, name := parts[0], parts[1]

	// 1. Resolve commit SHA via GitHub API.
	resolvedSHA, err := resolveCommitSHA(ctx, owner, name, tag)
	if err != nil {
		return nil, fmt.Errorf("baton: commit resolution: %w", err)
	}

	// 2. Fetch tarball via codeload.
	tarballBytes, err := fetchTarball(ctx, owner, name, tag)
	if err != nil {
		return nil, fmt.Errorf("baton: tarball fetch: %w", err)
	}

	// 3. Compute content digest (SHA-256 of raw tarball).
	h := sha256.New()
	h.Write(tarballBytes)
	computedDigest := fmt.Sprintf("sha256:%x", h.Sum(nil))

	// 4. Verify against recorded pins.
	pin, err := ReadUpstreamPin()
	if err != nil {
		return nil, fmt.Errorf("baton: cannot read pin: %w", err)
	}

	if pin.SHA != "" && pin.SHA != resolvedSHA {
		return nil, fmt.Errorf("baton: upstream SHA mismatch: pinned %s, resolved %s — the tag may have been force-moved; aborting", pin.SHA, resolvedSHA)
	}

	if pin.Digest != "" {
		// Digest pin exists — verify. (First-fetch bootstrap: Digest is empty,
		// so this branch is skipped; SHA still catches force-moved tags.)
		if pin.Digest != computedDigest {
			return nil, fmt.Errorf("baton: upstream digest mismatch: pinned %s, computed %s — tarball content changed; aborting", pin.Digest, computedDigest)
		}
	}

	// 5. Extract tarball to temp dir, stripping GitHub's prefix.
	tmpDir, err := os.MkdirTemp("", "baton-upstream-")
	if err != nil {
		return nil, fmt.Errorf("baton: temp dir: %w", err)
	}

	if err := extractTarball(tarballBytes, tmpDir, name, tag); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("baton: extract: %w", err)
	}

	return &FetchResult{
		SourceDir: tmpDir,
		SHA:       resolvedSHA,
		Digest:    computedDigest,
	}, nil
}

// FetchResult is the output of a successful FetchUpstream call.
type FetchResult struct {
	// SourceDir is the path to the extracted tarball content (a temp directory).
	// The caller is responsible for removing it after Vendor.
	SourceDir string

	// SHA is the resolved upstream commit SHA.
	SHA string

	// Digest is the SHA-256 content digest of the raw tarball (sha256:<hex>).
	Digest string
}

// Cleanup removes the temp source directory. Safe to call even if SourceDir is
// empty. Always call this after Vendor to avoid tempdir leaks.
func (r *FetchResult) Cleanup() {
	if r.SourceDir != "" {
		os.RemoveAll(r.SourceDir)
	}
}

// -- GitHub API helpers --------------------------------------------------------

// ghCommitResponse is the subset of GitHub's commit API response we need.
type ghCommitResponse struct {
	SHA string `json:"sha"`
}

var baseURLForTest string

// SetBaseURLForTest overrides the base URL for test HTTP servers.
// Call ClearBaseURLForTest to restore default behaviour.
func SetBaseURLForTest(url string) { baseURLForTest = url }

// ClearBaseURLForTest restores the default (real) base URLs.
func ClearBaseURLForTest() { baseURLForTest = "" }
func resolveCommitSHA(ctx context.Context, owner, name, tag string) (string, error) {
	base := "https://api.github.com"
	if baseURLForTest != "" {
		base = baseURLForTest
	}
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", base, owner, name, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// Add User-Agent to satisfy GitHub's API requirement.
	req.Header.Set("User-Agent", "sworn-baton-vendor/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("tag %q not found on %s/%s (404)", tag, owner, name)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var cr ghCommitResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode commit response: %w", err)
	}
	if cr.SHA == "" {
		return "", fmt.Errorf("empty SHA in commit response for tag %q — tag may reference a non-commit object", tag)
	}
	return cr.SHA, nil
}

func fetchTarball(ctx context.Context, owner, name, tag string) ([]byte, error) {
	base := "https://codeload.github.com"
	if baseURLForTest != "" {
		base = baseURLForTest
	}
	url := fmt.Sprintf("%s/%s/%s/tar.gz/refs/tags/%s", base, owner, name, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sworn-baton-vendor/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codeload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("tag %q not found — codeload returned 404; the tag may not exist upstream", tag)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("codeload returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tarball: %w", err)
	}
	return body, nil
}

// -- Tarball extraction -------------------------------------------------------

func extractTarball(data []byte, destDir, repoName, tag string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	// GitHub strips the leading v from semver tags in tarball paths
	// (e.g. tag "v0.5.0" -> archive prefix "baton-0.5.0/").
	cleanTag := strings.TrimPrefix(tag, "v")
	prefix := fmt.Sprintf("%s-%s/", repoName, cleanTag)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		// Strip the GitHub top-level prefix.
		name := hdr.Name
		if !strings.HasPrefix(name, prefix) {
			// File outside the expected prefix tree — skip it safely.
			continue
		}
		relPath := strings.TrimPrefix(name, prefix)
		if relPath == "" || strings.HasSuffix(relPath, "/") {
			continue // top-level dir itself or a trailing dir entry
		}

		destPath := filepath.Join(destDir, relPath)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", relPath, err)
			}
		case tar.TypeReg:
			// Ensure parent dir exists (tar entries for files may precede dir entries).
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("mkdir parent %s: %w", relPath, err)
			}
			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("create %s: %w", relPath, err)
			}
			if _, err := io.CopyN(f, tr, hdr.Size); err != nil {
				f.Close()
				return fmt.Errorf("write %s: %w", relPath, err)
			}
			f.Close()
		case tar.TypeSymlink:
			// Symlinks in tarballs are uncommon for Baton; skip safely.
		default:
			// Skip unknown entries safely.
		}
	}

	return nil
}