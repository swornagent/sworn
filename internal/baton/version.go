package baton

import (
	"regexp"
	"strings"

	"github.com/swornagent/sworn/internal/adopt"
)

var versionForTest string

var semverTagRE = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

// Version returns the embedded Baton protocol version string (a semver tag
// like "v0.4.2") read from the adopt embed. If the embed is missing or the// baton-protocol line cannot be parsed, it returns "".
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