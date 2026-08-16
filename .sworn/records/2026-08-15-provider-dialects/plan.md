```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-15-provider-dialects",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-15-provider-dialects/1",
  "tracks": [
    {
      "id": "T1-dialects",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-provider-dialect-extensions",
          "outcome": "Sworn's OpenAI-compatible driver admits Google and xAI as first-class providers by handling their wire dialects at named extension points: Google's per-message opaque thought signatures are retained and replayed byte-exact on every subsequent turn or the dispatch is refused, and xAI's vendor usage decorations are admitted without weakening strict decoding anywhere else, so gemini-3.7-flash and grok-4.6 certify and can carry roles.",
          "contract_path": "contracts/2026-08-15-provider-dialects/S1-provider-dialect-extensions.json",
          "digest": "sha256:a22164e7aa7f6ef1415bcb0e54fae1b9234f4b1cd5e77137cece08e9cd5b18f5",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver"
          ]
        }
      ]
    }
  ]
}
```

# Goal

Two frontier models the operator wants in the role rotation fail Sworn's
driver certification today, for the same underlying reason: each vendor
decorates the OpenAI-compatible wire shape, and Sworn's strict decoder
correctly refuses what it does not know.

gemini-3.7-flash returns each assistant message with an `extra_content`
container holding an opaque thought signature. The signature is the model's
reasoning continuity: a recorded two-turn probe shows that with the
signature replayed the model remembers its own prior turn, and without it
the model answers confidently and wrongly - no error, a silently broken
chain. grok-4.6 decorates its usage block with vendor accounting fields;
certification fails at usage decoding.

This release teaches the driver both dialects at named extension points.
It does not loosen strictness: unknown fields outside the extension points
keep failing exactly as they do today, and the extension points are an
explicit allowlist. Opaque state stays opaque - retained and replayed by
the continuation ledger, never parsed, never persisted into receipts.

The probe's silent-degradation finding sets the one non-negotiable design
point: replay is mandatory. If retained vendor state cannot be replayed,
the dispatch fails closed with a labelled error. A provider that degrades
silently is precisely the failure mode this engine exists to refuse.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-15-provider-dialects/1`. Planning did not approve
itself.

One track, one slice, scope `internal/driver` only - dialect handling,
fixtures and certification regression tests all land in that package, per
the scope-from-acceptance-evidence rule (issue #199). Recorded wire
fixtures from the live probes are the acceptance evidence: the Gemini
two-turn signature exchange and the Grok usage block.

Sequencing: approved now, run after 2026-08-12-configurable-paths merges,
since both bind `refs/heads/release/v1.0.0`. After this release merges, the
operator can route implementer to gemini-3.7-flash and verifier to
grok-4.6, preserving cross-vendor independence (Google implements, xAI
verifies, with DeepSeek and Qwen still certified as alternates).

This release does not alter trust rules, receipt schemas, role
independence, token budgets, streaming, continuation limits, or the
containment boundary. It renames nothing on the wire.
