```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-16-provider-dialects-2",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-16-provider-dialects-2/1",
  "tracks": [
    {
      "id": "T1-google-toolcalls",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-google-toolcall-dialect",
          "outcome": "The Google chat dialect handles the turns agentic work is made of: assistant messages that carry only tool calls decode without a content field, each tool call's own opaque thought-signature container is retained and replayed byte-exact in its per-call position or the dispatch is refused, and live certification of gemini-3.7-flash passes end to end.",
          "contract_path": "contracts/2026-08-16-provider-dialects-2/S1-google-toolcall-dialect.json",
          "digest": "sha256:018ec81a953d387f9ee8b8d8bbad31296fbe9aa997a052b150b9178297daaec2",
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

The provider-dialects release taught the driver Google's message-level
thought signature and xAI's usage decorations, and grok-4.6 certified
through it on the first try. gemini-3.7-flash did not, because the
recorded fixtures the work verified against never sampled a tool-call
turn - and live probing shows tool-call turns differ twice over: the
assistant message omits content entirely, and the thought signature rides
inside each tool call rather than at message level.

Both are one dialect's wire truth, now recorded as fixtures g4 (the
tool-call message) and g5 (the request/response pair proving the provider
accepts per-call replay and continues coherently). This release closes
exactly those two positions and makes live Gemini certification the
regression bar.

The evidence-sampling lesson is the durable part: a verifier can only
defend the wire shapes its fixtures contain. Fixtures for a dialect must
sample every structural position the dialect decorates - above all the
tool-call turn, which is the turn agentic delivery is made of.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-16-provider-dialects-2/1`. Planning did not approve
itself. One slice, scope internal/driver, per the
scope-from-acceptance-evidence rule (sworn#199). Roles: DeepSeek
implements; grok-4.6 verifies - its first verification, on the release
completing the dialect family that certified it.

This release does not alter trust rules, receipt schemas, token budgets,
or any dialect other than Google chat. It renames nothing on the wire.
