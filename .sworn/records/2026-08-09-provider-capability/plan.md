```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-09-provider-capability",
  "revision": 5,
  "previous_plan": "3b5d44f49a1909d7e5b10b42dfa18c55abe76cd1",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-09-provider-capability/5",
  "tracks": [
    {
      "id": "T1-provider",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-capability-axes",
          "outcome": "A driver profile declares the API flavour, reasoning effort and endpoint its model actually supports, so every OpenAI-compatible provider Sworn can reach is usable at its real capability without changing code.",
          "contract_path": "contracts/2026-08-09-provider-capability/rev5/S1-capability-axes.json",
          "digest": "sha256:a0e55d070df1e59f5e95ad4ec8f764b4a3f72db831639c68bd36ffb627271835",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver"
          ]
        },
        {
          "id": "S2-provider-presets",
          "outcome": "Providers are configuration, not code: the adapter union is keyed by wire protocol rather than by vendor, and a new OpenAI-compatible provider is added as a preset with no new Go type.",
          "contract_path": "contracts/2026-08-09-provider-capability/rev5/S2-provider-presets.json",
          "digest": "sha256:758ff36f52a64fe98b463a8e236b97a832c26c46f4fe16cc742bb52a72c336b5",
          "depends_on": [
            "S1-capability-axes"
          ],
          "consumes": [
            "S1-capability-axes"
          ],
          "touchpoints": [
            "internal/driver"
          ]
        },
        {
          "id": "S3-provider-observability",
          "outcome": "Every provider invocation reports what it actually cost and how it actually behaved - cache reuse, reasoning effort applied, and truncation - as normalized nullable facts that reach the operator surfaces and OpenTelemetry.",
          "contract_path": "contracts/2026-08-09-provider-capability/rev5/S3-provider-observability.json",
          "digest": "sha256:9b38bb75cbb730c36c8ea534092672c1eb55d4095cd2773bc8f89b2a289bd969",
          "depends_on": [
            "S2-provider-presets"
          ],
          "consumes": [
            "S2-provider-presets"
          ],
          "touchpoints": [
            "internal/driver",
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
`operator://2026-08-09-provider-capability/5`. Planning did not approve itself.

Revision 5 widens each slice's scope from an enumerated file list to the
owning package directory and adds an evidence-placement constraint. The
first real implementation attempts proved the enumerated scopes
unsatisfiable: acceptance requires changing behavior that existing tests
pin, so the tests are part of the deliverable, and the scope gate
rightly rejected candidates the contract had made impossible. Behavioral
exclusions are unchanged; the boundary moves from individual files to
the package that owns the promised behavior.

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
