// Package baton provides the vendor-down pipeline for the Baton protocol embed:
// resolve a pinned upstream source, transform bash/node script references into
// sworn-native commands, and write the result into the binary's go:embed trees.
//
// S48-baton-vendor — T14-baton-integration.
package baton

import (
	"fmt"
	"regexp"
	"strings"
)

// replacement is a single substitution rule.
// re matches the Baton script reference (with optional path prefix).
// token is the bare script filename (e.g. "release-verify.sh"), used by the
// fail-closed guard to check that no known script token survives the transform.
// new is the sworn-native replacement string.
type replacement struct {
	re    *regexp.Regexp
	token string
	new   string
}

// replacements is the ordered, table-driven substitution map from ADR-0006.
// Every Baton bash/node script reference is replaced with its sworn-native
// equivalent. The regex handles common path prefixes (scripts/, bin/,
// $HOME/.claude/bin/) so that e.g. `scripts/release-verify.sh` becomes
// `sworn verify` rather than `scripts/sworn verify`.
//
// The fail-closed guard derives its token list from this table — the single-table
// derive-both pattern guarantees they can't drift apart (Design Decision §2.1).
var replacements = []replacement{
	{token: "release-trace.sh", new: "sworn trace"},
	{token: "release-verify.sh", new: "sworn verify"},
	{token: "release-board-status.sh", new: "sworn board"},
	{token: "release-audit-design.sh", new: "sworn designaudit"},
	{token: "release-coverage.sh", new: "sworn coverage"},
	{token: "release-llm-check.sh", new: "sworn llmcheck"},
	{token: "release-mock-check.sh", new: "sworn mockcheck"},
	{token: "release-regression.sh", new: "sworn regression"},
	{token: "design-audit.sh", new: "sworn designaudit"},
	{token: "captain-route.sh", new: "the sworn internal router"},
	{token: "port-deriver.sh", new: "native port derivation"},
	{token: "captain-memory-search.py", new: "sworn memory search"},
	{token: "install.sh", new: "native binary installation"},
	{token: "server-start.sh", new: "sworn server start"},
	{token: "server-stop.sh", new: "sworn server stop"},
	{token: "install-codex.sh", new: "sworn codex"},
}

func init() {
	// Compile regex patterns for each replacement.
	// Pattern matches: optional path prefix (scripts/|bin/|...), then the token.
	for i := range replacements {
		tok := regexp.QuoteMeta(replacements[i].token)
		// Match common path prefixes: scripts/, bin/, $HOME/.claude/bin/
		// Also match the bare token (no prefix).
		pattern := `(?:scripts/|bin/|\$HOME/\.claude/bin/)?` + tok
		replacements[i].re = regexp.MustCompile(pattern)
	}
}

// Transform applies every substitution in the replacements table to content.
// It is file-format agnostic — it operates on the plain string, not on parsed
// markdown — so it won't break on upstream format changes.
//
// Transform returns the transformed content and a non-nil error if any known
// Baton script token (from the replacements table) survives the transform.
// This is the fail-closed guard: a new script reference added upstream that is
// not in the map cannot slip through unmapped.
func Transform(content string) (string, error) {
	out := content
	for _, r := range replacements {
		out = r.re.ReplaceAllString(out, r.new)
	}

	// Fail-closed guard: check that no known Baton script token survives.
	// The guard list is derived from the same table, so they can't drift apart.
	for _, r := range replacements {
		if strings.Contains(out, r.token) {
			return out, fmt.Errorf("baton: unmapped script reference %q survives transform — update the substitution map", r.token)
		}
	}

	// Additional guard: scan for any Baton script-like reference that isn't in
	// the map. This catches new script names added upstream (e.g. "new-tool.sh").
	// Match bare script names (lowercase + hyphens + .sh/.py/.mjs), excluding
	// known tokens that have already been replaced.
	scriptRef := regexp.MustCompile(`[a-z][a-z0-9-]*\.(?:sh|py|mjs)`)
	for _, m := range scriptRef.FindAllString(out, -1) {
		known := false
		for _, r := range replacements {
			if m == r.token {
				known = true
				break
			}
		}
		if !known {
			return out, fmt.Errorf("baton: unknown script reference %q survives transform — add it to the substitution map or update upstream", m)
		}
	}

	return out, nil
}