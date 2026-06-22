// Package prompt embeds the Baton role prompts (planner, implementer, verifier,
// captain) and the full Baton protocol documents (rules, track-mode,
// session-discipline, brainstorm-patterns) into the sworn binary via go:embed.
// The prompts are vendored verbatim from the open Baton protocol
// (~/.claude/baton/role-prompts/). The baton/ subdirectory contains the
// canonical Baton protocol documents served via sworn://baton/* MCP resources.
//
// Vendored Baton protocol version is recorded in VERSION.txt and surfaced by
// `sworn version`.
package prompt

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed verifier.md implementer.md planner.md captain.md verify-stateless.md requirements-verifier.md VERSION.txt baton/*
var fs embed.FS

var (
	verifier             string
	implementer          string
	planner              string
	captain              string
	verifyStateless      string
	requirementsVerifier string
	batonVer             string
	trackMode            string
)

func init() {
	verifier = mustRead("verifier.md")
	implementer = mustRead("implementer.md")
	planner = mustRead("planner.md")
	captain = mustRead("captain.md")
	verifyStateless = mustRead("verify-stateless.md")
	requirementsVerifier = mustRead("requirements-verifier.md")
	trackMode = mustRead("baton/track-mode.md")
	batonVer = strings.TrimSpace(mustRead("VERSION.txt")) // Strip the comment line(s) — version is the last non-empty line.
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

// Baton returns the content of a single Baton protocol document from the
// embedded baton/ subdirectory. name is the filename relative to baton/
// (e.g. "rules.md", "track-mode.md"). Returns an error if the file does
// not exist.
func Baton(name string) (string, error) {
	b, err := fs.ReadFile("baton/" + name)
	if err != nil {
		return "", fmt.Errorf("baton: %s not found in embed: %w", name, err)
	}
	return string(b), nil
}

// BatonAll returns all embedded Baton protocol documents keyed by filename.
// The map includes every file in the baton/ subdirectory: "rules.md",
// "track-mode.md", "session-discipline.md", "brainstorm-patterns.md",
// "README.md", "VERSION.txt".
func BatonAll() map[string]string {
	entries, err := fs.ReadDir("baton")
	if err != nil {
		// The baton/ directory must exist in the embed — this is a
		// build-time invariant enforced by the go:embed directive.
		panic("prompt: baton/ directory not found in embed: " + err.Error())
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := fs.ReadFile("baton/" + e.Name())
		if err != nil {
			// Skip unreadable files rather than panicking — a
			// corrupted embed is better surfaced as a missing
			// key than as a binary crash.
			continue
		}
		out[e.Name()] = string(b)
	}
	return out
}