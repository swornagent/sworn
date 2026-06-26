package baton

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/swornagent/sworn/internal/adopt"
)

var versionForTest string
var upstreamPinForTest *UpstreamPin

var semverTagRE = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

// Version returns the embedded Baton protocol version string (a semver tag
// like "v0.4.2") read from the adopt embed. If the embed is missing or the
// baton-protocol line cannot be parsed, it returns "".
func Version() string {
	if versionForTest != "" {
		return versionForTest
	}
	data, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "baton-protocol:"); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// IsSemverTag reports whether s is a strict vMAJOR.MINOR.PATCH semver tag.
// Pre-release/build suffixes are rejected.
func IsSemverTag(s string) bool {
	return semverTagRE.MatchString(s)
}

// UpstreamPin holds the recorded upstream commit SHA and content digest.
type UpstreamPin struct {
	SHA    string // upstream-sha from VERSION (empty if not recorded)
	Digest string // upstream-digest from VERSION (empty on first fetch)
}

// ReadUpstreamPin reads the embedded VERSION and extracts the upstream-sha
// and upstream-digest values. Returns zero-valued UpstreamPin on missing
// or unparseable embed — the caller treats empty SHA/digest as a first-fetch
// bootstrap.
func ReadUpstreamPin() (UpstreamPin, error) {
	if upstreamPinForTest != nil {
		return *upstreamPinForTest, nil
	}
	data, err := adopt.BatonDocsFS().ReadFile("baton/VERSION")
	if err != nil {
		// No VERSION embed — first fetch, no pins.
		return UpstreamPin{}, nil
	}
	return parseUpstreamPin(string(data)), nil
}

// WriteUpstreamPin writes the resolved SHA and computed digest into the
// VERSION file at repoRoot/internal/adopt/baton/VERSION. Existing
// upstream-sha / upstream-digest lines are updated; if absent, they are
// appended. Other lines (baton-protocol, vendored, etc.) are preserved.
func WriteUpstreamPin(repoRoot, sha, digest string) error {
	path := filepath.Join(repoRoot, "internal", "adopt", "baton", "VERSION")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read VERSION: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	hasSHA := false
	hasDigest := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "upstream-sha:") {
			out = append(out, fmt.Sprintf("upstream-sha: %s", sha))
			hasSHA = true
		} else if strings.HasPrefix(trimmed, "upstream-digest:") {
			out = append(out, fmt.Sprintf("upstream-digest: %s", digest))
			hasDigest = true
		} else {
			out = append(out, line)
		}
	}

	if !hasSHA && sha != "" {
		out = append(out, fmt.Sprintf("upstream-sha: %s", sha))
	}
	if !hasDigest && digest != "" {
		out = append(out, fmt.Sprintf("upstream-digest: %s", digest))
	}

	content := strings.Join(out, "\n")
	// Normalise trailing newline.
	content = strings.TrimRight(content, "\n") + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write VERSION: %w", err)
	}
	return nil
}

// parseUpstreamPin extracts UpstreamPin from the raw VERSION content.
func parseUpstreamPin(content string) UpstreamPin {
	var pin UpstreamPin
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "upstream-sha:"); ok {
			pin.SHA = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "upstream-digest:"); ok {
			pin.Digest = strings.TrimSpace(after)
		}
	}
	return pin
}
