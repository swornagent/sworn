package baton

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/adopt"
)

var versionForTest string
var upstreamPinForTest *UpstreamPin

var semverTagRE = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

// Version returns the embedded Baton protocol version string (a semver tag
// like "vX.Y.Z") read from the adopt embed. If the embed is missing or the
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

// UpstreamPin holds the recorded upstream tag, commit SHA and content digest.
//
// The pin is self-describing: Tag names WHICH upstream the SHA and digest belong
// to. A SHA without its tag cannot be checked — comparing it against a different
// tag's resolved SHA is meaningless, since on an intentional version bump they
// differ by definition.
type UpstreamPin struct {
	Tag    string // baton-protocol from VERSION — the tag this pin describes
	SHA    string // upstream-sha from VERSION (empty if not recorded)
	Digest string // upstream-digest from VERSION (empty on first fetch)
}

// ReadUpstreamPin reads the embedded VERSION and extracts the pinned tag,
// upstream-sha and upstream-digest values. Returns zero-valued UpstreamPin on
// missing or unparseable embed — the caller treats empty SHA/digest as a
// first-fetch bootstrap.
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

// WriteUpstreamPin writes the resolved tag, SHA and digest — plus the vendoring
// date — into the VERSION file at repoRoot/internal/adopt/baton/VERSION.
// Existing lines are updated in place; absent ones are appended.
//
// The tag, SHA and digest are ONE FACT — "this is the upstream we vendored" — so
// they are written together. This previously wrote the SHA and digest while
// deliberately preserving the baton-protocol line, which left the pin claiming
// one version while carrying another's content: `sworn doctor` reported the wrong
// protocol version, and a later fetch of the tag the pin *claimed* compared that
// tag's real SHA against a pin secretly holding a different tag's, aborting with a
// bogus "force-moved" error. An empty tag leaves baton-protocol untouched.
func WriteUpstreamPin(repoRoot, tag, sha, digest string) error {
	path := filepath.Join(repoRoot, "internal", "adopt", "baton", "VERSION")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read VERSION: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	hasTag := false
	hasDate := false
	hasSHA := false
	hasDigest := false

	vendoredOn := time.Now().UTC().Format("2006-01-02")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case tag != "" && strings.HasPrefix(trimmed, "baton-protocol:"):
			out = append(out, fmt.Sprintf("baton-protocol: %s", tag))
			hasTag = true
		case tag != "" && strings.HasPrefix(trimmed, "vendored:"):
			out = append(out, fmt.Sprintf("vendored: %s", vendoredOn))
			hasDate = true
		case strings.HasPrefix(trimmed, "upstream-sha:"):
			out = append(out, fmt.Sprintf("upstream-sha: %s", sha))
			hasSHA = true
		case strings.HasPrefix(trimmed, "upstream-digest:"):
			out = append(out, fmt.Sprintf("upstream-digest: %s", digest))
			hasDigest = true
		default:
			out = append(out, line)
		}
	}

	if !hasTag && tag != "" {
		out = append(out, fmt.Sprintf("baton-protocol: %s", tag))
	}
	if !hasDate && tag != "" {
		out = append(out, fmt.Sprintf("vendored: %s", vendoredOn))
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
		if after, ok := strings.CutPrefix(line, "baton-protocol:"); ok {
			pin.Tag = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "upstream-sha:"); ok {
			pin.SHA = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "upstream-digest:"); ok {
			pin.Digest = strings.TrimSpace(after)
		}
	}
	return pin
}
