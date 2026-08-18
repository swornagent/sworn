```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-17-google-native-driver",
  "revision": 3,
  "previous_plan": "c6035af4e06e56422ec79e702d1ecd9bb077454b",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-17-google-native-driver/3",
  "tracks": [
    {
      "id": "T1-native",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-gemini-native-adapter",
          "outcome": "Gemini becomes economically real through Google's front door: a native generateContent adapter family whose implicit caching is visible and accounted (77% observed on recorded fixtures), whose thinking level is an operator-chosen profile knob, and whose thought signatures ride as the first-class fields they are - so the roster's fastest implementer stops paying full freight on every resent byte.",
          "contract_path": "contracts/2026-08-17-google-native-driver/rev3/S1-gemini-native-adapter.json",
          "digest": "sha256:d3eed71ebc14b81804c368bfedf0799a398c947f7af002058df2a755eda9273f",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/driver", "internal/observe", "test/e2e"]
        }
      ]
    }
  ]
}
```

# Goal

Gemini 3.7 Flash entered the roster through Google's OpenAI-compatibility
side door, and the meter told the story: ~2.2M tokens per minute against a
3M ceiling, every turn re-paying the full context, two provider-limit
parks in one evening. The measurements are unambiguous - identical
realistic bodies show no cache reporting at all on the compat surface and
a 77% reported implicit-cache hit on the native surface. The model was
never the problem; the door was.

This release builds the native generateContent adapter family: visible
caching in the usage receipts, thinking level as the operator's knob
(ruled HIGH for implementation work), thought signatures as the
first-class fields the native surface makes them, tools and multi-call
turns in the native shape, and the same closed-allowlist strictness as
every other surface. The compat adapter stays certified as the fallback.

The paused loop-economics release resumes on this driver: the fast
implementer returns paying cache prices, and S4's batching then cuts the
remaining bill.

# Revision 2

Mid-design, the implementer was observed weighing how to encode Sworn's
string-shaped tool results into functionResponse.response, a JSON value.
The seductive answer - parse the string and embed it structured when it
happens to be valid JSON - would make the wire shape depend on content:
receipts would bind bytes the model never saw, and Gemini would diverge
from every other provider on identical tool results. The operator chose
to intervene before a candidate could choose it.

This revision changes one acceptance criterion and adds one constraint:
results ride a fixed single-key envelope {"result": "<the exact string>"}
regardless of content, arguments re-serialise through the engine's
canonical encoder so digested bytes are deterministic, and adversarial
fixture n6 - a tool result whose text is itself valid JSON, recorded
passing through the envelope as text - makes the shortcut mechanically
detectable forever. Scope, checks and every other criterion are
byte-identical to revision 1.

# Revision 3

A4 binds "thoughtsTokenCount reported as reasoning", but nothing downstream
can carry it: Gemini's reasoning count is admitted into the native decoder's
closed allowlist and then dropped, and the eval records' token gap is already
tracked as sworn#209 because it cannot be backfilled after the fact. The
"no receipt schema changes" sentence in the Authority section predates A4's
specific, deliberately approved day-1 requirement (A4 was approved as-is at
revision 2); where the boilerplate and A4 conflict, A4 wins.

This revision therefore adds exactly one additive, optional,
backward-compatible field - `reasoning_tokens` on `driver.Usage` and
`driver.UsageReceipt`, both `omitempty` - propagated on the same path
`cache_read_tokens` already takes: Gemini's `thoughtsTokenCount` into the
driver result, through `NormalizeUsage` into the journal's usage receipt,
into `internal/observe` eval aggregation and telemetry, and into the
production journey's receipt assertion. The Authority sentence is revised
to authorise precisely this one field and nothing else; no other receipt
schema, journal, or wire-vocabulary change is made.

Because loop-economics resumes mid-S1 and re-records its receipts after this
release moves the target head, the additive field must leave every receipt
written before it byte-identical when re-encoded. That is an explicit
acceptance criterion (A7), verified by a test that re-encodes existing
receipt shapes byte-for-byte - not an assumption.

Scope is widened to `internal/observe` and `test/e2e` alongside the existing
`internal/driver`, per the configurable-paths rev4 precedent: the
propagation's acceptance evidence lands across `eval.go`, `telemetry.go`,
`production_journey_linux_test.go`, and their supporting record, payload and
test files, so the candidate must be able to touch those paths legally.
Fixtures n1-n6, the pinned content-independent {"result": ...} envelope,
canonical arguments re-serialisation, the thinking-level knob,
thought-signature replay and the closed allowlists stay exactly as revision 2
promised.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-17-google-native-driver/3`. Planning did not approve
itself. One slice, scope internal/driver, internal/observe and test/e2e per
the scope-from-acceptance-evidence rule (sworn#199): adapter, dialect,
fixtures, certification tests and the reasoning-token propagation all land in
those packages.

Roles: DeepSeek implements and grok-4.6 verifies - the subject vendor
holds neither role, which is the independence discipline at its cleanest.

Sequencing: loop-economics is paused mid-S1 with receipts safe on its
refs; this release merges first, moves the target head, and economics
re-records at resume - the known, cheap dance. This release does not alter
trust rules, budgets, or any other adapter, and it changes the receipt
schema in exactly one way: the additive, optional `reasoning_tokens` field
this revision authorises, which leaves every existing receipt byte-identical
when re-encoded (A7 verifies this, rather than assuming it). It renames
nothing on the wire.

