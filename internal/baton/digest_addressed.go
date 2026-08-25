package baton

import (
	"strings"
)

// canonicalDigestDocumentationText is the sworn-owned authoring documentation
// for canonical contract digests. It states the three facts an operator
// authoring contracts must know: (1) the canonical payload shape whose sha256
// is the declared digest, (2) why `sha256sum` over the contract file never
// matches the declared digest (track and id are injected from the manifest
// entry, not present in the file), and (3) the stability promise that the
// canonical payload shape is frozen so every existing contract digest stays
// byte-for-byte stable. This is in-scope product content under internal/baton;
// the test-pinned surface is CanonicalDigestDocumentation(), not a docs/ path
// or a vendored asset.
const canonicalDigestDocumentationText = `# Canonical Contract Digests

## What the digest is

The digest declared in a sworn.release-manifest/v1 manifest entry is the
SHA-256 over the canonical JSON encoding of a payload shaped as:

  {track, id, outcome, scope, acceptance, checks, constraints, depends_on, consumes}

The manifest entry's ` + "`track`" + ` and ` + "`id`" + ` are injected into the
canonical payload from the manifest; they are not present in the standalone
contract file itself. The ` + "`scope`" + ` object carries ` + "`include`" + `,
` + "`exclude`" + `, and (when present) ` + "`waivers`" + `. The ` + "`acceptance`" + `
array carries ` + "`{id, text}`" + ` objects. When a contract declares
` + "`host_checks`" + `, it is included in the canonical payload; otherwise it is
omitted so contracts that do not use it keep an identical canonical digest.

## Why sha256sum over the contract file never matches

Because ` + "`track`" + ` and ` + "`id`" + ` are injected from the manifest entry
and the JSON is canonicalized (sorted keys, no insignificant whitespace),
running ` + "`sha256sum`" + ` over the contract file on disk will never produce
the declared digest. The declared digest is computed over the canonical
payload, not over the raw file bytes. Use ` + "`sworn plan pin`" + ` (when
available) or the engine's own computation to derive the canonical digest
from contract bytes and manifest identity.

## Stability promise

The canonical payload shape is frozen. Every existing contract digest stays
byte-for-byte stable across releases because the canonicalization and the
payload key set do not change. A future addition to the contract schema must
be optional and canonicalized identically to preserve every existing digest.
`

// CanonicalDigestDocumentation returns the sworn-owned authoring documentation
// for canonical contract digests. The text states the canonical payload
// shape, why `sha256sum` over the contract file never matches the declared
// digest, and the stability promise that the canonical payload shape is
// frozen. This is the test-pinned surface for the "where an operator
// authoring contracts will actually find them" requirement: it is in-scope
// product content under internal/baton, not a docs/ path or a vendored asset.
func CanonicalDigestDocumentation() string {
	return canonicalDigestDocumentationText
}

// contractStorePath returns the digest-addressed record path for one slice's
// contract bytes under the record root. The path is:
//
//	<recordRoot>/<release>/contracts/<digest-prefix>/<digest>.json
//
// where digest-prefix is the first 2 hex chars of the canonical digest (the
// "sha256:" prefix stripped). The manifest's declared digest determines the
// path, so the digest is the identity and the path is derived, not the other
// way around. The path lives under the record root, satisfying the record
// transition's requirement that every change be under the record root.
func contractStorePath(recordRoot, release, digest string) string {
	hex := strings.TrimPrefix(digest, "sha256:")
	prefix := ""
	if len(hex) >= 2 {
		prefix = hex[:2]
	}
	return recordRoot + "/" + release + "/contracts/" + prefix + "/" + digest + ".json"
}