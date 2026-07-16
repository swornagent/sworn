// Package baton provides the vendor-down pipeline for the Baton protocol embed:
// resolve a pinned upstream source, transform bash/node script references into
// sworn-native commands, and write the result into the binary's go:embed trees.
//
// S48-baton-vendor — T14-baton-integration.
package baton

import (
	"fmt"
	"strings"
)

// replacement is a single substitution rule.
// re matches the Baton script reference (with optional path prefix).
// token is the bare script filename (e.g. "release-verify.sh"), used by the
// fail-closed guard to check that no known script token survives the transform.
// new is the sworn-native replacement string.
type replacement struct {
	token string
	new   string
}

type scriptReferenceError struct {
	token string
}

func (e *scriptReferenceError) Error() string {
	return fmt.Sprintf("baton: unknown script reference %q survives transform — add it to the substitution map or update upstream", e.token)
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
	{token: "port-deriver.sh", new: "native port derivation"}, {token: "captain-memory-search.py", new: "sworn memory search"},
	{token: "install.sh", new: "native binary installation"},
	{token: "server-start.sh", new: "sworn server start"},
	{token: "server-stop.sh", new: "sworn server stop"},
	{token: "install-codex.sh", new: "sworn codex"},
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
	var out strings.Builder
	out.Grow(len(content))

	for start := 0; start < len(content); {
		if !isScriptTokenByte(content[start]) {
			out.WriteByte(content[start])
			start++
			continue
		}

		end := start + 1
		for end < len(content) && isScriptTokenByte(content[end]) {
			end++
		}
		token := content[start:end]
		if !hasScriptSuffix(token) {
			out.WriteString(token)
			start = end
			continue
		}

		base := token
		if slash := strings.LastIndexByte(base, '/'); slash >= 0 {
			base = base[slash+1:]
		}
		replacementText := ""
		for _, replacement := range replacements {
			if base == replacement.token {
				replacementText = replacement.new
				break
			}
		}
		if replacementText == "" {
			return out.String() + token + content[end:], &scriptReferenceError{token: token}
		}
		out.WriteString(replacementText)
		start = end
	}

	return out.String(), nil
}

func isScriptTokenByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("_-.+/$~@", rune(b))
}

func hasScriptSuffix(token string) bool {
	return strings.HasSuffix(token, ".sh") || strings.HasSuffix(token, ".py") || strings.HasSuffix(token, ".mjs")
}
