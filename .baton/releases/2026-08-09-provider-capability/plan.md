```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-09-provider-capability",
  "revision": 4,
  "previous_plan": "7c304713c36ed4d68d7bed68aea8c3cfca9f8906",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-09-provider-capability/4",
  "tracks": [
    {
      "id": "T1-provider",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-capability-axes",
          "outcome": "A driver profile declares the API flavour, reasoning effort and endpoint its model actually supports, so every OpenAI-compatible provider Sworn can reach is usable at its real capability without changing code.",
          "contract_path": "contracts/2026-08-09-provider-capability/S1-capability-axes.json",
          "digest": "sha256:8396c802dd75f169b2596bafbae4a8ce665abe7992186967ece067d4848dc68f",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver/openai.go",
            "internal/driver/responses.go",
            "internal/driver/mantle.go",
            "internal/driver/http.go"
          ]
        },
        {
          "id": "S2-provider-presets",
          "outcome": "Providers are configuration, not code: the adapter union is keyed by wire protocol rather than by vendor, and a new OpenAI-compatible provider is added as a preset with no new Go type.",
          "contract_path": "contracts/2026-08-09-provider-capability/S2-provider-presets.json",
          "digest": "sha256:c5dd2947e690f4c02817c7c1be19768edc8359c858b4812550a30993c8ee0542",
          "depends_on": [
            "S1-capability-axes"
          ],
          "consumes": [
            "S1-capability-axes"
          ],
          "touchpoints": [
            "internal/driver/config.go",
            "internal/driver/factory.go",
            "internal/driver/bedrock.go",
            "internal/driver/aws_chain.go"
          ]
        },
        {
          "id": "S3-provider-observability",
          "outcome": "Every provider invocation reports what it actually cost and how it actually behaved - cache reuse, reasoning effort applied, and truncation - as normalized nullable facts that reach the operator surfaces and OpenTelemetry.",
          "contract_path": "contracts/2026-08-09-provider-capability/S3-provider-observability.json",
          "digest": "sha256:1f1c85b9a9a82f7025bcdd6db045d782d998bc910f4e0376bfca3e1c6c4a31fa",
          "depends_on": [
            "S2-provider-presets"
          ],
          "consumes": [
            "S2-provider-presets"
          ],
          "touchpoints": [
            "internal/driver/usage.go",
            "internal/driver/provider.go",
            "internal/observe"
          ]
        }
      ]
    }
  ]
}
```

# Goal

Sworn can already reach every OpenAI-compatible provider on the wire, but it
cannot be configured to use them at their real capability. Three verified
defects block it.

Reasoning effort is refused on the chat-completions flavour by
`OpenAIProfileConfig.valid()`, even though the request struct already serialises
`reasoning_effort` and providers accept it — so a live GLM-5.2 call at high
effort succeeds by hand and is unconfigurable in the product. Endpoint admission
requires a path ending `/v1/chat/completions`, which rejects DeepSeek (no `/v1`
segment) and Gemini (`/v1beta/openai/...`) outright. And the adapter union is
keyed by vendor rather than by wire protocol, so each new provider needs a Go
type even when it speaks a protocol already implemented.

The result is a system whose reach is limited by its configuration surface
rather than by what it can actually talk to. This release makes provider
capability a declared property of a profile, makes providers configuration
rather than code, and makes each invocation report what it actually cost and how
it actually behaved.

That last part matters beyond tidiness: cache accounting is parsed from provider
responses today and then discarded, so nothing can tell whether prefix caching
is working, and a silent regression to a zero hit rate would present only as a
larger bill. Recording effort applied and cache reuse turns provider choice into
something measurable instead of something asserted.

# Authority

Approved by the human operator against these exact bytes under
`operator://2026-08-09-provider-capability/4`. Planning did not approve itself.

Revision 4 changes no slice, contract, or dependency. It re-binds the same
approved work to the advanced target head, which now also removes the
recovery-budget expression caps (d72e0747): reasoning-size and
continuation-step limits that killed live worker turns, and per-type
nudge/correction allowances replaced by turn-budget-scale runaway guards
with durable per-step accounting. The adoption gate correctly refused
each moved target.

The three slices form one serial track because they share the same production
files; nothing here is eligible for concurrent execution. Each slice keeps the
existing five checks, with the end-to-end gate at 60 minutes — the value proven
necessary during 2026-08-07-sworn-native-delivery-kernel, where a 20-minute gate
could not execute its own acceptance.

Development targets `refs/heads/release/v1.0.0`, not `refs/heads/main`. Nothing
commits direct to main: the slice contracts land on the release branch as
ordinary product content, the release assembles and merges into it, and
promoting it to main is a separate human-gated step outside this release. The
branch is kept ahead-of-or-equal-to main and is never rebased, so that promotion
stays a fast-forward.

This release does not port any platform-gated file, does not touch native CLI
drivers or their certification, does not introduce streaming or provider-side
conversation state, and does not add a business telemetry projection. Those
remain separately approved work.
