// Package prompt embeds the Baton role prompts (planner, implementer, verifier,
// captain) and the full Baton protocol documents (rules, track-mode,
// session-discipline, brainstorm-patterns) into the sworn binary via go:embed.
// The prompts are vendored verbatim from the open Baton protocol
// (~/.claude/baton/role-prompts/). The baton/ subdirectory contains the
// canonical Baton protocol documents served via sworn://baton/* MCP resources.
//
// Baton protocol version is read from the canonical source (internal/adopt/baton/VERSION)
// via internal/baton.Version() — no separate VERSION.txt.
package prompt

import (
	"embed"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
)

//go:embed verifier.md implementer.md planner.md captain.md design-reviewer.md verify-stateless.md requirements-verifier.md design-tldr.md baton/*
var fs embed.FS
var (
	verifier             string
	implementer          string
	planner              string
	captain              string
	designReviewer       string
	verifyStateless      string
	requirementsVerifier string
	designTLDR           string
	trackMode            string
)

func init() {
	verifier = mustRead("verifier.md")
	implementer = mustRead("implementer.md")
	planner = mustRead("planner.md")
	captain = mustRead("captain.md")
	designReviewer = mustRead("design-reviewer.md")
	verifyStateless = mustRead("verify-stateless.md")
	requirementsVerifier = mustRead("requirements-verifier.md")
	designTLDR = mustRead("design-tldr.md")
	trackMode = mustRead("baton/track-mode.md")
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

// LLMCheck returns the vendored Baton system prompt for one of the six LLM check
// types (ac-satisfaction, spec-ambiguity, design-review, security-review,
// semantic-coverage, maintainability-review), with its YAML frontmatter stripped.
//
// The prompt body IS the contract — the same way a schema is (Baton v0.12.0,
// baton/llm-checks/). It is read verbatim from the vendored protocol rather than
// re-typed as a Go constant, so an upstream prompt change cannot silently diverge
// from what sworn actually runs. Before v0.12.0 these lived only as inline consts
// in internal/gate, which meant no other engine could implement the protocol and
// there was no by-hand fallback for an adopter.
func LLMCheck(name string) (string, error) {
	b, err := fs.ReadFile("baton/llm-checks/" + name + ".md")
	if err != nil {
		return "", fmt.Errorf("prompt: llm-check %q not vendored: %w", name, err)
	}
	return stripFrontmatter(string(b)), nil
}

// stripFrontmatter removes a leading YAML frontmatter block, returning the body.
// The frontmatter carries metadata (name, which roles run it, output schema); the
// body is the prompt.
func stripFrontmatter(s string) string {
	const fence = "---\n"
	if !strings.HasPrefix(s, fence) {
		return s
	}
	rest := s[len(fence):]
	end := strings.Index(rest, "\n"+fence)
	if end < 0 {
		return s // unterminated frontmatter — return as-is rather than truncate
	}
	return strings.TrimLeft(rest[end+len("\n"+fence):], "\n")
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

// DesignReviewer returns the embedded design-reviewer role prompt — the
// Captain in its design-review capacity only (S19-captain-split). The engine's
// design-review dispatch uses this, NOT Captain(): captain.md is vendored
// verbatim from upstream Baton and still conflates the release-orchestrator
// function, which the deterministic Sworn engine owns.
func DesignReviewer() string { return designReviewer }

// DesignTLDR returns the embedded design-TL;DR prompt (§1–§6) used by the
// design-generation step in sworn run (S45).
func DesignTLDR() string { return designTLDR }

// BatonVersion returns the vendored Baton protocol version string (e.g. "on Baton v0.4.2").
func BatonVersion() string { return "on Baton " + baton.Version() } // TrackMode returns the embedded Baton track-mode.md content.
func TrackMode() string    { return trackMode }

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
// "README.md".
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
