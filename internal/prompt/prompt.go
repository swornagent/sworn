// Package prompt embeds the Baton role prompts (planner, implementer, verifier,
// captain) into the sworn binary via go:embed. The prompts are vendored verbatim
// from the open Baton protocol (~/.claude/baton/role-prompts/).
//
// Vendored Baton protocol version is recorded in VERSION.txt and surfaced by
// `sworn version`.
package prompt

import (
	"embed"
	"strings"
)

//go:embed verifier.md implementer.md planner.md captain.md verify-stateless.md requirements-verifier.md VERSION.txt baton/track-mode.md
var fs embed.FS

var (
	verifier              string
	implementer           string
	planner               string
	captain               string
	verifyStateless       string
	requirementsVerifier  string
	batonVer              string
	trackMode             string
)

func init() {
	verifier = mustRead("verifier.md")
	implementer = mustRead("implementer.md")
	planner = mustRead("planner.md")
	captain = mustRead("captain.md")
	verifyStateless = mustRead("verify-stateless.md")
	requirementsVerifier = mustRead("requirements-verifier.md")
	trackMode = mustRead("baton/track-mode.md")
	batonVer = strings.TrimSpace(mustRead("VERSION.txt"))	// Strip the comment line(s) — version is the last non-empty line.
	if lines := strings.Split(batonVer, "\n"); len(lines) > 0 {
		for i := len(lines) - 1; i >= 0; i-- {
			if ln := strings.TrimSpace(lines[i]); ln != "" && !strings.HasPrefix(ln, "#") {
				batonVer = ln
				break
			}
		}
	}
}

func mustRead(name string) string {
	b, err := fs.ReadFile(name)
	if err != nil {
		// A missing file at init() is a build-time failure — the binary
		// should not start in a degraded state.
		panic("prompt: embedded file " + name + " not found: " + err.Error())
	}
	return string(b)
}

// Verifier returns the embedded Baton verifier role prompt.
func Verifier() string { return verifier }

// VerifyStateless returns the embedded sworn-authored stateless judge prompt
// for the verify gate — SPEC+DIFF only, no tools, verdict-leading reply.
func VerifyStateless() string { return verifyStateless }

// RequirementsVerifier returns the embedded requirements-quality verifier prompt
// for the reqverify gate — grades acceptance criteria against 29148 quality
// characteristics, fail-closed on any violation.
func RequirementsVerifier() string { return requirementsVerifier }

// Implementer returns the embedded Baton implementer role prompt.
func Implementer() string { return implementer }

// Planner returns the embedded Baton planner role prompt.
func Planner() string { return planner }

// Captain returns the embedded Baton captain role prompt.
func Captain() string { return captain }

// BatonVersion returns the vendored Baton protocol version string (e.g. "v1.0.0").
func BatonVersion() string { return batonVer }

// TrackMode returns the embedded Baton track-mode.md content.
func TrackMode() string { return trackMode }
